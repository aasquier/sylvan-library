"""FastAPI application.

Routes are deliberately thin: parse, delegate to `service.py`, serialise. The
logic they call is the same code the CLI uses, so the app and the terminal can
never drift into disagreeing about a deck.
"""

from __future__ import annotations

import mimetypes
from collections.abc import AsyncIterator, Awaitable, Callable
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Annotated, Any

from fastapi import Depends, FastAPI, HTTPException, Query, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.middleware.gzip import GZipMiddleware
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles

import mtglab
from mtglab import config
from mtglab.api import admin, adminstats, auth, jobs, service, traffic
from mtglab.api.deps import Scope, UserScope, deck_source, library
from mtglab.auth import bootstrap
from mtglab.auth.mail import EmailSender
from mtglab.decks import log
from mtglab.decks.library import Library
from mtglab.decks.source import DeckNotFound, DeckSource, ReadOnlySource

# The request scope, as one annotation. Every deck-facing route takes it, so
# when auth arrives the change is to `deps.deck_source` and nowhere else.
#
# `Decks` is now only for routes that are about the instance rather than about
# one person's decks -- `/api/health` counts decks and has no owner to name.
# Anything owner-addressed takes `Lib` and resolves the path's owner segment
# through it, which is ADR 22 and `decks/library.py`.
Decks = Annotated[DeckSource, Depends(deck_source)]
Lib = Annotated[Library, Depends(library)]

WEB_DIST = Path(__file__).resolve().parent.parent / "web_dist"

#: The 78 tarot pictures, shipped in the package rather than the bundle.
#: `assets/tarot/PROVENANCE.md` says where they came from and why they may be
#: here. Kept out of `web_dist` because that directory belongs to Vite, and
#: putting them through `web/public` would store 4.6MB twice in git.
TAROT = Path(__file__).resolve().parent.parent / "assets" / "tarot"

# Name the type ourselves rather than asking the host what a `.webp` is.
#
# Starlette guesses a static file's content type through `mimetypes`, which
# reads the *operating system's* database — `/etc/mime.types` and friends. The
# slim image has none, so `mimetypes.guess_type("x.webp")` answers `None`
# there and all 78 tarot pictures went out as `application/octet-stream`. This
# laptop knows `.webp` and so does CI's ubuntu, which is precisely why nothing
# caught it: the fault is a property of the environment the code is installed
# into, not of the code. That is the same shape as the two bugs of 2026-08-12
# — a faked mail `Transport`, a stubbed SDK — and the third instance of the
# lesson: **ask what the container has, not what the tests pass with.**
#
# Browsers sniff the bytes and render them anyway, so this was invisible in
# production too. That is not a reason to leave it. One
# `X-Content-Type-Options: nosniff`, one strict proxy or one CDN image rule
# and every card in the deck fails at once, in production only, with a green
# suite behind it. `.js`, `.css` and `.html` are in Python's own built-in
# table and need no help; `.webp` is not, in 3.12.
mimetypes.add_type("image/webp", ".webp")

#: Revalidate before reuse, every time.
#:
#: **`no-cache` does not mean "do not store".** It means "do not reuse without
#: asking", which is exactly what a committed bundle with stable filenames
#: needs. `web/vite.config.ts` emits `assets/app.js` rather than
#: `assets/app.<hash>.js` deliberately — the bundle is in git so that `mtglab
#: ui` needs no Node, and hashed names would add two files to the repository on
#: every rebuild — and its comment says freshness comes from the etag and
#: last-modified Starlette already sends.
#:
#: It does, but **only if the browser asks**, and with no `Cache-Control` at all
#: a browser may assign its own heuristic freshness lifetime (RFC 9111 §4.2.2)
#: and skip the question entirely. Safari did. After the deploy of 2026-08-13 a
#: reload re-fetched `app.js` and never requested `DeckDetail.js`, so an old
#: dossier panel met a new server contract, was handed a job where it expected
#: a report, and the page went black.
#:
#: The cost of getting this wrong is not one bad reload: it is **a returning
#: visitor running two halves of two different versions**, silently, after any
#: deploy that changes the contract between them. What it trades for that is
#: one conditional request per asset per load, answered `304` with no body.
NO_CACHE = {"Cache-Control": "no-cache"}


class Revalidated(StaticFiles):
    """`StaticFiles` that asks the browser to check first. See `NO_CACHE`.

    `setdefault` rather than assignment so a future explicit header on a
    particular file wins over this blanket one.
    """

    async def get_response(self, path: str, scope: Any) -> Any:
        response = await super().get_response(path, scope)
        response.headers.setdefault("cache-control", NO_CACHE["Cache-Control"])
        return response


def _job_for(plan: jobs.Plan, caller: UserScope) -> jobs.Job:
    """A finished job when the answer was already known, a queued one otherwise.

    Every producer of a `Plan` comes through here -- `api/simruns.py` for
    Tier 1, `api/themeruns.py` for the theme proposal, `api/dossierruns.py`
    for the dossier -- and no route decides which pool the work belongs in.
    The plan carries its own `lane` because that is a property of the work
    rather than of the route, and a route is exactly the place somebody would
    eventually forget to pass it. `key` rides along for the same reason: what
    counts as "the same work" is the planner's to know.
    """
    if plan.result is not None:
        return jobs.completed(plan.kind, result=plan.result,
                              label=plan.label, owner=caller.user_id)
    return jobs.submit(plan.kind, plan.run, label=plan.label,
                       owner=caller.user_id, lane=plan.lane, key=plan.key)


def create_app(*, dev: bool = False, require_auth: bool | None = None,
               secure_cookies: bool | None = None,
               email_sender: EmailSender | None = None) -> FastAPI:
    """Build the app.

    `require_auth` defaults to `MTGLAB_REQUIRE_AUTH`, which is off — see
    `config.require_auth` for why the local tool is not put behind a login. The
    argument exists so a test can build the deployed configuration without
    setting environment variables, which is what makes the whole auth core
    checkable on a laptop (`docs/HOSTING.md` §6 step 5).

    `email_sender` is the same idea one layer further out (ADR 16): `None`
    resolves from the environment when a message is sent, and a test passes a
    recorder, which is how "no test sends mail" is a property of the wiring
    rather than a promise.
    """
    required = config.require_auth() if require_auth is None else require_auth
    secure = (config.secure_cookies() if secure_cookies is None
              else secure_cookies)

    @asynccontextmanager
    async def startup(_app: FastAPI) -> AsyncIterator[None]:
        """Reconcile the maintainer to admin, once, as the server comes up.

        ADR 17, and a no-op unless `MTGLAB_ADMIN_EMAIL` is set. It belongs here
        rather than in the body of `create_app` because this module builds an
        `app` at import for uvicorn: doing it there would mean that merely
        *importing* `mtglab.api.app` -- which the CLI and every API test do --
        creates an `app.db` wherever the environment happens to be pointing.
        Starting to serve is the event this is actually about.
        """
        bootstrap.ensure_maintainer()
        yield
        # The visitor ledger's last write: whatever the request counter still
        # buffers goes to disk as the server stops, so a quiet instance's
        # final minute is not the minute that never happened.
        traffic.flush()

    app = FastAPI(title="sylvan-library", version=mtglab.__version__,
                  description="Local Commander deckbuilding and simulation.",
                  lifespan=startup)

    # Compression the app owns. The premise this landed under — "Fly's proxy
    # passes bodies through as sent" — was wrong, and a skipped deploy is what
    # proved it: the old code, still live, was already answering
    # `Content-Encoding: gzip`, so Fly's edge compresses on its own. What this
    # buys anyway, measured on the instance on 2026-08-14: level-9 gzip sends
    # the bundle at 84.5 kB where the edge's compressor sent 119.6 kB, the
    # behaviour is the app's own on any host rather than one proxy's
    # undocumented habit, and `mtglab ui` on a laptop has no edge at all.
    #
    # Registered FIRST, which makes it the INNERMOST layer, and that placement
    # is load-bearing rather than stylistic: `minimum_size` reads the response's
    # Content-Length, and the decorator-style middlewares below re-wrap every
    # response as a stream without one — registered outermost, this compressed
    # two-byte job polls. Innermost it sees the real response, so the floor
    # keeps 304s and small JSON whole. Known waste, accepted: a tarot WebP over
    # the floor is gzipped for no byte saved — on first load only, since
    # `Revalidated` turns every later request into a bodyless 304. Card art
    # never passes through here at all; it hotlinks from Scryfall's CDN.
    app.add_middleware(GZipMiddleware, minimum_size=1024)

    # Before any route is declared: the middleware runs ahead of routing, so
    # what it protects is every path the app will ever serve rather than the
    # ones remembered at review time. (The gzip registration above is not an
    # exception — `add_middleware` mounts a wrapper, and the auth middleware
    # still sees every request before any route does.)
    auth.install(app, require=required, secure_cookies=secure,
                 email_sender=email_sender)
    admin.install(app, email_sender=email_sender)
    adminstats.install(app)

    # Registered *after* auth.install on purpose: `add_middleware` prepends, so
    # the later registration is the outer layer and these headers land on the
    # auth middleware's own refusals (the 401 and the admin 403) as well as on
    # every routed response.
    #
    # What is deliberately absent is a Content-Security-Policy. The app inlines
    # styles (React style props throughout) and loads card art from Scryfall's
    # CDN, so a real policy needs `style-src 'unsafe-inline'` and an img-src
    # allowlist -- and a policy got slightly wrong fails only in production,
    # only in the browser, with a green suite behind it, which is this
    # project's most-repeated deployment bug shape. The headers below are the
    # ones with no such failure mode.
    #
    # `nosniff` is safe *because* `mimetypes.add_type` above registered
    # `.webp`: before that fix, sniffing was what kept the tarot art rendering,
    # and this header would have broken all 78 cards at once.
    @app.middleware("http")
    async def security_headers(
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        response = await call_next(request)
        headers = response.headers
        # `setdefault`, like `Revalidated`: an explicit header on a particular
        # response wins over the blanket one.
        headers.setdefault("X-Content-Type-Options", "nosniff")
        headers.setdefault("X-Frame-Options", "DENY")
        # `same-origin` rather than the browser default: outbound links and
        # the Scryfall image fetches need no referrer, and the one URL that
        # must never leak -- the claim link's token -- rides in the fragment,
        # which no Referer header ever carries. This is belt and braces.
        headers.setdefault("Referrer-Policy", "same-origin")
        # One sensor is wanted, by one door: the camera import (ADR 34) calls
        # `getUserMedia` from this origin. `self` is the narrowest value that
        # allows it -- an embedded frame still cannot ask -- and the two the
        # app has no use for stay denied outright, so an XSS that got this far
        # cannot reach a microphone or a position.
        #
        # This read `camera=()` until 2026-08-19. It was written when "nothing
        # in the app wants a sensor" was true and was not revisited when the
        # camera door landed, so the feature shipped unable to open a camera
        # at all: the policy denies the origin itself, `getUserMedia` rejects,
        # and the door reports a refusal nobody made. A sentence in a comment
        # asserting what the app does not do is a claim to re-check against
        # the app, which is the same rule CLAUDE.md draws around `.[dev]`.
        headers.setdefault("Permissions-Policy",
                           "camera=(self), microphone=(), geolocation=()")
        if secure:
            # Only when TLS fronts the app (the same condition as the cookie's
            # `Secure` flag). A year, no preload: preload is a public,
            # hard-to-undo registration and this instance's domain choices
            # should not be made by a default.
            headers.setdefault("Strict-Transport-Security",
                               "max-age=31536000")
        return response

    # Registered after the headers middleware, so it is the outer layer and
    # counts everything — the auth middleware's own refusals included, which
    # never reach routing and land in the ledger's one shared bucket.
    traffic.install(app)

    if dev:
        # Vite dev server runs on another port, so the browser needs CORS. Only
        # in dev -- the built app is same-origin and needs none of this.
        app.add_middleware(
            CORSMiddleware,
            allow_origins=["http://localhost:5173", "http://127.0.0.1:5173"],
            allow_methods=["*"], allow_headers=["*"],
        )

    @app.exception_handler(DeckNotFound)
    async def _deck_missing(_request: Request, exc: DeckNotFound) -> JSONResponse:
        return JSONResponse(status_code=404,
                            content={"detail": f"no deck '{exc}'"})

    @app.exception_handler(ReadOnlySource)
    async def _deck_read_only(_request: Request, exc: ReadOnlySource) -> JSONResponse:
        """A deck this caller may read but not change.

        Registered here rather than caught in each write route, for the reason
        the middleware exists: there are nine routes that write a deck and the
        tenth is the one somebody adds in a year. A handler on the exception
        cannot be forgotten by a route that raises it, because raising it *is*
        how a source refuses.

        **403 and not ADR 5's 404**, which is the same exception ADR 17 makes
        for `/api/admin` and for the same reason. ADR 5 hides resources whose
        *existence* is the secret; this deck's existence is not — it is in a
        public repository, `GET /api/decks` lists it to every account, and the
        caller has very likely just been reading it. A 404 would hide nothing
        and would tell somebody their deck had vanished.
        """
        return JSONResponse(
            status_code=403,
            content={"detail": f"read-only: {exc} is not yours to change"})

    # ------------------------------------------------------------- meta

    @app.get("/api/health")
    def health(decks: Decks) -> dict[str, Any]:
        return service.health(source=decks)

    @app.get("/api/sets/upcoming")
    def upcoming_sets() -> dict[str, Any]:
        try:
            return service.upcoming_sets()
        except OSError as exc:
            # The only route that needs the network. Say so plainly instead of
            # returning a 500 that looks like a bug in the app.
            raise HTTPException(
                status_code=503,
                detail=f"could not reach Scryfall: {exc}") from exc

    # ------------------------------------------------------------ decks

    @app.get("/api/decks")
    def list_decks(lib: Lib) -> list[dict[str, Any]]:
        """Every deck this caller may see, their own first (ADR 22).

        Spans owners now, and each tile says which one — so this is the route
        the browse tab groups, and the only place a client learns the owner
        segment it needs to build any other deck URL.

        A private deck belonging to somebody else is simply not here. That is
        the same fact its 404 states, arrived at the same way: `Library` never
        hands out a source that can see it.
        """
        return service.list_library(lib)

    @app.post("/api/decks/import")
    def import_deck(payload: dict[str, Any], lib: Lib) -> dict[str, Any]:
        """Turn a pasted decklist into a draft deck.

        Declared before `/api/decks/{slug}` so the literal path wins the match
        -- and it is a POST while that is a GET, so the two could not collide
        even if the order changed.

        `dry_run` runs the identical code path and writes nothing, which is
        what the app's preview uses: the user approves the actual result rather
        than a description of it.
        """
        commander = payload.get("commander") or []
        if isinstance(commander, str):
            commander = [commander]
        bracket = payload.get("bracket")
        try:
            return service.import_deck(
                owner=lib.my_owner,
                text=str(payload.get("text", "")),
                slug=str(payload.get("slug", "")),
                name=str(payload.get("name", "")),
                commander=[str(c) for c in commander],
                companion=str(payload.get("companion") or ""),
                bracket=int(bracket) if bracket not in (None, "") else None,
                status=str(payload.get("status") or "theoretical"),
                dry_run=bool(payload.get("dry_run")),
                source=lib.mine(),
            )
        except (ValueError, TypeError) as exc:
            raise HTTPException(status_code=422,
                                detail=f"bracket must be a number: {exc}") from exc
        except service.ImportRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.post("/api/decks")
    def create_deck(payload: dict[str, Any], lib: Lib) -> dict[str, Any]:
        """Start a new deck from a commander and nothing else.

        The last gap in the deck lifecycle. Like the import route it is
        declared before `/api/decks/{slug}`, and like that route it is a POST
        against a GET, so the two cannot collide.

        There is no `color_identity` field on purpose: identity is derived from
        the commander (rule 2), and accepting one here would be a second source
        of truth for the one fact this project will not guess at.
        """
        commander = payload.get("commander") or []
        if isinstance(commander, str):
            commander = [commander]
        bracket = payload.get("bracket")
        try:
            return service.create_deck(
                owner=lib.my_owner,
                slug=str(payload.get("slug", "")),
                name=str(payload.get("name", "")),
                commander=[str(c) for c in commander],
                companion=str(payload.get("companion") or ""),
                bracket=int(bracket) if bracket not in (None, "") else None,
                status=str(payload.get("status") or "theoretical"),
                source=lib.mine(),
            )
        except (ValueError, TypeError) as exc:
            raise HTTPException(status_code=422,
                                detail=f"bracket must be a number: {exc}") from exc
        except service.CreateRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/decks/{owner}/{slug}")
    def get_deck(owner: str, slug: str, lib: Lib) -> dict[str, Any]:
        return service.get_deck(slug, source=lib.source_for(owner), owner=owner)

    @app.delete("/api/decks/{owner}/{slug}")
    def delete_deck(owner: str, slug: str, lib: Lib,
                    confirm: str = Query("", description="must equal the slug"),
                    ) -> dict[str, Any]:
        """Remove a deck from the library, recoverably.

        `confirm` is a query parameter and must equal the slug. A DELETE with
        no body is the conventional shape, and making the confirmation a value
        only somebody looking at the right deck can produce is what stops a
        mis-aimed request from being indistinguishable from an intended one.
        """
        try:
            return service.delete_deck(slug=slug, confirm=confirm, source=lib.source_for(owner))
        except service.DeleteRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/decks/{owner}/{slug}/validate")
    def validate_deck(owner: str, slug: str, lib: Lib) -> dict[str, Any]:
        return service.validate_deck(slug, source=lib.source_for(owner))

    @app.get("/api/decks/{owner}/{slug}/stats")
    def deck_stats(owner: str, slug: str, lib: Lib) -> dict[str, Any]:
        return service.stats_for(slug, source=lib.source_for(owner))

    @app.get("/api/decks/{owner}/{slug}/log")
    def deck_log(owner: str, slug: str, lib: Lib,
                 limit: int = Query(log.DEFAULT_LIMIT, ge=1, le=500),
                 ) -> dict[str, Any]:
        """What has been done to this deck, newest first (ADR 28).

        A GET beside `validate` and `stats`, and user-scoped by exactly the
        same mechanism: `lib.source_for(owner)` is what decides whether this
        deck exists for this caller, so a history is unreachable in precisely
        the cases the deck is. There is no separate rule about who may read
        one, which is the point — a second rule is a second thing to get wrong.
        """
        return service.history_for(slug, source=lib.source_for(owner),
                                   limit=limit)

    @app.post("/api/decks/{owner}/{slug}/swap")
    def swap_card(owner: str, slug: str, payload: dict[str, Any], lib: Lib) -> dict[str, Any]:
        """Carry out a swap the caller has already decided on.

        A write endpoint on an otherwise read-only API, so it is narrow on
        purpose: one card out, one card in, a rationale the caller supplies,
        and a re-run of the gate in the response. It cannot add, delete or
        reorder anything.
        """
        try:
            return service.swap_card(
                slug,
                out=str(payload.get("out", "")),
                into=str(payload.get("into", "")),
                why=str(payload.get("why", "")),
                source=lib.source_for(owner), actor=lib.actor,
            )
        except service.SwapRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    # The rest of the edit operations (ADR 12). Each is one narrow write that
    # re-runs the gate and reports it, so the app can never leave a deck
    # changed and unchecked. None of them may author a rationale: `why` is
    # whatever the caller typed, and an empty one on a curated deck is a 422
    # rather than a blank the tool fills in.

    @app.post("/api/decks/{owner}/{slug}/cards")
    def add_card(owner: str, slug: str, payload: dict[str, Any], lib: Lib) -> dict[str, Any]:
        try:
            return service.add_card(
                slug,
                name=str(payload.get("name", "")),
                category=str(payload.get("category", "")),
                why=str(payload.get("why") or ""),
                qty=int(payload.get("qty") or 1),
                to=str(payload.get("to") or "cards"),
                source=lib.source_for(owner), actor=lib.actor,
            )
        except (TypeError, ValueError) as exc:
            raise HTTPException(status_code=422,
                                detail=f"qty must be a number: {exc}") from exc
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.delete("/api/decks/{owner}/{slug}/cards/{name}")
    def remove_card(owner: str, slug: str, name: str, lib: Lib) -> dict[str, Any]:
        """Entomb a 99-card (ADR 27); a swap-board card is removed outright."""
        try:
            return service.remove_card(slug, name=name,
                                       source=lib.source_for(owner),
                                       actor=lib.actor)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.post("/api/decks/{owner}/{slug}/entomb")
    def entomb_cards(owner: str, slug: str, payload: dict[str, Any],
                     lib: Lib) -> dict[str, Any]:
        """The bulk entombment: several 99-cards to the graveyard in one write.

        All or nothing -- a name not in the 99 refuses the batch with nothing
        written, so the deck state after a sweep is always one somebody chose.
        """
        names = payload.get("names")
        if not isinstance(names, list):
            raise HTTPException(status_code=422, detail="names must be a list")
        try:
            return service.entomb_cards(slug, names=[str(n) for n in names],
                                        source=lib.source_for(owner),
                                        actor=lib.actor)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.post("/api/decks/{owner}/{slug}/graveyard/{name}/return")
    def return_card(owner: str, slug: str, name: str, lib: Lib) -> dict[str, Any]:
        """Bring an entombed card back to the 99, its rationale intact."""
        try:
            return service.return_card(slug, name=name,
                                       source=lib.source_for(owner),
                                       actor=lib.actor)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.delete("/api/decks/{owner}/{slug}/graveyard/{name}")
    def exile_card(owner: str, slug: str, name: str, lib: Lib) -> dict[str, Any]:
        """Remove an entombed card permanently -- the only hard delete left."""
        try:
            return service.exile_card(slug, name=name,
                                      source=lib.source_for(owner),
                                      actor=lib.actor)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.patch("/api/decks/{owner}/{slug}/cards/{name}")
    def set_card_field(owner: str, slug: str, name: str, payload: dict[str, Any],
                       lib: Lib) -> dict[str, Any]:
        """Change one field of one card: its category, quantity or rationale.

        The rationale editor's write path. A PATCH of one field rather than a
        PUT of the card, because a card is mostly pool facts and the deck
        file only carries the handful of things a person decided.
        """
        field = str(payload.get("field", ""))
        if "value" not in payload:
            raise HTTPException(status_code=422, detail="value is required")
        try:
            return service.set_card_field(slug, name=name, field=field,
                                          value=payload["value"],
                                          source=lib.source_for(owner),
                                          actor=lib.actor)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.patch("/api/decks/{owner}/{slug}")
    def set_deck_field(owner: str, slug: str, payload: dict[str, Any],
                       lib: Lib) -> dict[str, Any]:
        """Change one of the deck's own fields: stage, status or bracket.

        `stage` to `curated` is promotion, the last step of an import. It is
        refused while any card still lacks a rationale, so the deck is never
        written into a state the gate would immediately reject.
        """
        if "value" not in payload:
            raise HTTPException(status_code=422, detail="value is required")
        try:
            return service.set_deck_field(slug, field=str(payload.get("field", "")),
                                          value=payload["value"],
                                          source=lib.source_for(owner),
                                          actor=lib.actor)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.put("/api/decks/{owner}/{slug}/notes/{key}")
    def set_note(owner: str, slug: str, key: str, payload: dict[str, Any],
                 lib: Lib) -> dict[str, Any]:
        try:
            return service.set_note(slug, key=key,
                                    value=str(payload.get("value", "")),
                                    source=lib.source_for(owner),
                                    actor=lib.actor)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.put("/api/decks/{owner}/{slug}/shared")
    def set_deck_shared(owner: str, slug: str, payload: dict[str, Any],
                        lib: Lib) -> dict[str, Any]:
        """Put a deck on display to other accounts, or take it off (ADR 22).

        Its own route rather than a `field` on the PATCH beside it, because
        the two tiers hold this fact in different places and the source is
        what knows which — `deck.yaml` for the curated six, a column for
        everybody else.

        Refusals are the ordinary two and neither is written here: somebody
        else's shared deck raises `ReadOnlySource` (403), and their private
        one was never in the source at all, so it raises `DeckNotFound` (404).
        """
        if "shared" not in payload:
            raise HTTPException(status_code=422, detail="shared is required")
        source = lib.source_for(owner)
        source.set_shared(slug, bool(payload["shared"]))
        return service.get_deck(slug, source=source, owner=owner)

    @app.post("/api/decks/{owner}/{slug}/wheel")
    def deck_wheel(owner: str, slug: str, lib: Lib,
                   body: dict[str, Any] | None = None) -> dict[str, Any]:
        """One turn of the Wheel of Fortune (punch list item 9).

        A POST because each spin is a fresh draw, but read-only with respect
        to the deck -- readers may spin a shared deck's wheel, exactly as
        they may read its stats. `seed` replays a spin; absent, the server
        rolls one and reports it.
        """
        seed = (body or {}).get("seed")
        return service.wheel_spin(
            slug, source=lib.source_for(owner),
            seed=int(seed) if seed is not None else None)

    @app.get("/api/decks/{owner}/{slug}/suggestions")
    def deck_suggestions(owner: str, slug: str, lib: Lib,
                         limit: int = Query(5, ge=1, le=20)) -> dict[str, Any]:
        return service.suggestions_for(slug, source=lib.source_for(owner), limit=limit)

    @app.get("/api/decks/{owner}/{slug}/commander")
    def deck_commander(owner: str, slug: str, lib: Lib) -> dict[str, Any]:
        """Who leads this deck, and what the pool knows about them.

        Its own route rather than more fields on `GET /api/decks/{slug}`,
        for two reasons. It runs several extra queries — a count per subtype,
        a name search, a printing lookup — to fill a panel that is decorative,
        and the deck page should not wait on any of that to render its 99.
        And it answers with `card: null` rather than a 404 when there is no
        pool, which is a different contract from the deck itself.
        """
        return service.commander_dossier(slug, source=lib.source_for(owner))

    @app.get("/api/decks/{owner}/{slug}/printings")
    def deck_printings(owner: str, slug: str, lib: Lib,
                       card: str | None = Query(None)) -> dict[str, Any]:
        """Every non-digital printing of this deck's commander, newest first.

        Its own route rather than fields on the deck: Goreclaw has twelve and
        most decks never open the picker, so this is a query the deck page
        should not pay for on every load.

        `?card=` asks about any card the deck holds instead -- the 99, the
        swap board or the graveyard -- so alternate art is not a privilege of
        the command zone. A card the deck does not hold is a 422, not an
        empty list, because the only caller is a picker about to write.
        """
        try:
            return service.commander_printings(slug, card=card,
                                               source=lib.source_for(owner))
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    # ------------------------------------------------------------ cards

    @app.post("/api/cards/identify")
    def identify_cards(payload: dict[str, Any]) -> dict[str, Any]:
        """Read what a camera thought it saw against the pool.

        A POST because a fanned spread is forty sightings and a title is
        free text, neither of which belongs in a query string -- and because
        what somebody is holding is not a thing to leave in an access log.

        No image is ever sent here. The body is a list of
        `{set, number, title}`, all optional, and the answer is a card per
        sighting or a shortlist per sighting. Which of the two, and why it is
        never both, is argued in `cards/identify.py`.
        """
        raw = payload.get("sightings")
        if not isinstance(raw, list):
            raise HTTPException(
                status_code=422,
                detail="sightings must be a list of {set, number, title}")
        sightings = [s for s in raw if isinstance(s, dict)]
        return service.identify_cards(sightings)

    @app.get("/api/cards/search")
    def search_cards(
        q: str = "",
        identity: str = "",
        type_line: str = "",
        cmc_max: float | None = None,
        price_max: float | None = None,
        sort: str = "edhrec",
        limit: int = Query(60, ge=1, le=200),
        identity_exact: bool = False,
        commanders_only: bool = False,
    ) -> dict[str, Any]:
        return service.search_cards(q=q, identity=identity, type_line=type_line,
                                    cmc_max=cmc_max, price_max=price_max,
                                    sort=sort, limit=limit,
                                    identity_exact=identity_exact,
                                    commanders_only=commanders_only)

    # ----------------------------------------------------------- claude

    @app.get("/api/claude")
    def claude_status(lib: Lib, stance: str = "", slug: str = "",
                      owner: str = "", surface: str = "") -> dict[str, Any]:
        """Is the Claude surface installed, configured, and switched on?

        Three separate answers — a UI that collapses them tells someone their
        key is missing when actually they turned it off. Reaches no network:
        the stance is deterministic and availability is a fact about the
        environment.

        `surface` names which mode is asking, and exists because the answer is
        not the same for all of them. A deck-facing mode derives its default
        from the deck, so `slug` is enough; the theme interview runs before a
        deck exists and defaults to what a theoretical deck would get. Without
        this, the dial beside the create flow reported `off` while the
        conversation it governs was about to run at `second-opinion`.
        """
        try:
            return service.claude_status(
                requested=stance or None, slug=slug or None,
                surface=surface or None,
                source=lib.source_for(owner or lib.my_owner))
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/claude/personas")
    def claude_personas() -> dict[str, Any]:
        """The voices the theme interview can adopt, for the door to render.

        Free, deterministic and reaching nothing: this is a checked-in table,
        the same class of thing as `/api/colors`. It answers with no key set
        and no card pool, which matters because the door renders before anybody
        has committed to spending anything.

        `voice` is deliberately not in the payload. Not because a prompt in a
        public repository is a secret, but because a client that received one
        would eventually send one back, and "the persona is one of a fixed set"
        is worth keeping structural rather than polite.
        """
        from mtglab.claude import persona as persona_mod
        return {"personas": persona_mod.as_dicts(),
                "default": persona_mod.DEFAULT}

    @app.get("/api/tarot/reading")
    def tarot_reading(seed: int | None = None) -> dict[str, Any]:
        """Deal three cards. No model, no card pool, no network, no cost.

        **Python decides** (ADR 14): a shuffle has a right answer and belongs
        here, while what a spread means has none and belongs to the reader.
        Seeded and returning its seed, so the client can carry one integer and
        get the same three cards for the whole conversation — the same
        stateless trick the transcript uses, and the reason a reading needs no
        table either.

        A `seed` may be supplied to re-deal an existing reading, which is what
        a reload does.
        """
        from mtglab import tarot
        return tarot.deal(seed).as_dict()

    @app.post("/api/claude/research")
    def claude_research(payload: dict[str, Any],
                        caller: Scope) -> dict[str, Any]:
        """Answer a question about Magic (ADR 26). Returns a **job**.

        **Takes no deck, no owner and no `Decks` dependency, and that absence
        is the feature.** This surface exists to answer what the pool cannot —
        the meta, a ruling in practice, a card spoiled ahead of the next bulk
        refresh — and a mode that cannot reach a deck cannot critique one. It
        is also what keeps *deck conversation*, ADR 15's third mode and
        deliberately unbuilt, from being built here by accident: the way to
        make this route into that one is to add a dependency on purpose, in a
        diff, superseding ADR 26.

        A job from the first commit rather than after a deployed failure.
        Research searches more than the dossier, which broke at 236 seconds,
        and ADR 20's lesson — a duration measured for one surface is a question
        to ask of every sibling — has now cost three incidents. What stays
        here is every refusal: 422 for an empty question or one long enough to
        be a pasted decklist, 503 for no key. A call that comes back unusable
        is a job in state `error`, which is where it belongs once the response
        has been sent.

        Two identical questions in flight from one account are one job — see
        `api/researchruns.py` on why that is the opposite call from the theme
        conversation's.
        """
        from mtglab.api.researchruns import plan_research
        from mtglab.claude.client import ClaudeUnavailable
        from mtglab.claude.research import QuestionRejected

        try:
            plan = plan_research(
                question=payload.get("question"),
                requested=payload.get("stance") or None,
                tier=caller.model_tier)
        except (QuestionRejected, ValueError) as exc:
            # `QuestionRejected` is a ValueError, which is why it is named
            # first; the second catch is a malformed stance.
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        return _job_for(plan, caller).as_dict()

    @app.post("/api/claude/scan")
    def claude_scan(payload: dict[str, Any],
                    caller: Scope) -> dict[str, Any]:
        """Read a photographed card with Claude (ADR 34). Returns a **job**.

        The fallback tier of the camera door, for the cards the browser's own
        reader cannot do — chiefly **anything printed before mid-2015, which
        carries no collector number on its face at all** and is most of the
        deep cuts this library is full of.

        **This is the one route in the app that receives a photograph**, and
        it never receives one by accident: the local tier sends nothing but
        two short strings, and a capture arrives here only because somebody
        pressed a button on that specific card. The image is passed to
        Anthropic, is not written to disk, and is not logged.

        What comes back is not a card. The mode transcribes what is printed
        and `identify` decides what it is, so a corner resolves only against
        the pool's real set codes and a title still only ever offers a
        shortlist — the same scrutiny the WebAssembly reader's output gets,
        which is what keeps ADR 14 intact with a model in the loop.

        A job from the first commit, and its duration is **unmeasured** — the
        reason it is a job rather than an argument that it needs to be one.
        Refusals stay here: 422 for a capture that is not an image this reads,
        is empty, or is over the size cap; 503 for no key. Two presses on one
        photograph are one paid call.
        """
        from mtglab.api.scanruns import plan_scan
        from mtglab.claude.client import ClaudeUnavailable
        from mtglab.claude.scan import ScanRefused

        try:
            plan = plan_scan(
                image=payload.get("image") or b"",
                media_type=str(payload.get("media_type") or "image/jpeg"),
                requested=payload.get("stance") or None,
                tier=caller.model_tier)
        except (ScanRefused, ValueError) as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        return _job_for(plan, caller).as_dict()

    @app.post("/api/claude/theme")
    def claude_theme(payload: dict[str, Any],
                     caller: Scope) -> dict[str, Any]:
        """One turn of the theme interview (ADR 20). Returns a **job**.

        Takes no deck and no `Decks` dependency, and that absence is the
        feature: this surface exists to help somebody *start* a deck, and a
        mode that cannot reach a deck cannot critique one.

        The transcript is the client's — ADR 20 keeps conversation state off
        the server — so this endpoint is the door. It takes plain
        `{role, text}` turns and never Anthropic message blocks; an endpoint
        that accepted those would be a free proxy for somebody else's spend.
        `check_transcript` refuses everything else as a 422.

        This was a synchronous POST until it was measured: 4.3–37.7 seconds
        across eleven turns on the instance, and one at 133.8s. The docstring
        that justified keeping it synchronous said "it is a few seconds", which
        is word for word what left the dossier synchronous until it broke
        deployed at 236s — and 236s is the *only* thing known about the
        transport ceiling, as an upper bound nobody has narrowed. What is no
        longer here is the 502: a call that came back unusable is now a job in
        state `error`, which is where it belongs once the response has been
        sent. 422 and 503 are still decided here, by `plan_ask`.

        A turn that reaches nobody — stance `off`, or a conversation past its
        exchange ceiling — comes back as a job already `done`, so the common
        cheap case still costs exactly one request.
        """
        from mtglab.api.themeruns import plan_ask
        from mtglab.claude.client import ClaudeUnavailable
        from mtglab.claude.theme import TranscriptRejected

        try:
            plan = plan_ask(
                transcript=payload.get("transcript"),
                slots=payload.get("slots"),
                requested=payload.get("stance") or None,
                # An unknown persona is an `UnknownPersona`, which is a
                # `ValueError`, which the handler below already answers 422.
                persona=payload.get("persona") or None,
                seed=payload.get("seed"),
                # The facts already shown, client-held like the transcript,
                # so "never give the same fact twice" is enforceable rather
                # than aspirational. `check_told` refuses a malformed list
                # as a 422 like everything else about the request.
                facts=payload.get("facts"),
                tier=caller.model_tier)
        except (TranscriptRejected, ValueError) as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        return _job_for(plan, caller).as_dict()

    @app.post("/api/claude/theme/proposal")
    def claude_theme_proposal(payload: dict[str, Any],
                              caller: Scope) -> dict[str, Any]:
        """Submit the proposal. Returns a **job**, not a proposal.

        Measured at 226 seconds, which no hosted proxy will hold a POST open
        for, so this queues the work and the client polls `/api/jobs/{id}` --
        the same contract the two sim routes have had all along. The
        conversational half above stays synchronous; it is a few seconds.

        What did *not* move into the job is every refusal, and that is the
        point of `themeruns.plan_proposal`. 409 when the floor has not been
        reached, which is its own status on purpose: nothing is malformed and
        nothing failed, there simply is not enough yet — a 422 would read as
        "you sent something wrong" to a client that sent exactly the right
        thing too early. 503 when there is no key. Both are still decided here,
        because a job in state `error` carrying one of those sentences is three
        answers flattened into one string.
        """
        from mtglab.api.themeruns import plan_proposal
        from mtglab.claude.client import ClaudeUnavailable
        from mtglab.claude.theme import NotReady, TranscriptRejected

        budget = payload.get("budget")
        try:
            plan = plan_proposal(
                transcript=payload.get("transcript"),
                slots=payload.get("slots"),
                requested=payload.get("stance") or None,
                budget=float(budget) if budget else None,
                avoid=str(payload.get("avoid") or ""),
                persona=payload.get("persona") or None,
                seed=payload.get("seed"),
                tier=caller.model_tier)
        except NotReady as exc:
            raise HTTPException(status_code=409, detail=str(exc)) from exc
        except (TranscriptRejected, ValueError) as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        return _job_for(plan, caller).as_dict()

    @app.post("/api/decks/{owner}/{slug}/interview")
    def claude_interview(owner: str, slug: str, payload: dict[str, Any],
                         lib: Lib, caller: Scope) -> dict[str, Any]:
        """Ask the rationale interview about one card. Returns questions.

        A POST because it costs money and makes a network call, not because it
        writes anything — it cannot. It is filed under the deck rather than
        under `/api/claude` because that is what it is about, and because a
        second mode will want the same shape.

        The failure modes are kept apart deliberately. 503 means no call was
        possible (no SDK, no key); 502 means a call was made and came back
        unusable; 422 means the question was wrong. Collapsing them tells
        someone their key is missing when the model was merely rate limited.
        """
        from mtglab.claude.client import ClaudeUnavailable
        from mtglab.claude.interview import CardNotInDeck

        card = str(payload.get("card", "")).strip()
        if not card:
            raise HTTPException(status_code=422, detail="card is required")
        try:
            return service.claude_interview(
                slug=slug, card=card,
                requested=payload.get("stance") or None,
                focus=str(payload.get("focus") or ""),
                source=lib.source_for(owner),
                tier=caller.model_tier)
        except CardNotInDeck as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ValueError as exc:
            # A malformed stance. `CardNotInDeck` is a ValueError too, which is
            # why it is caught above this rather than below it.
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        except service.ClaudeFailed as exc:
            raise HTTPException(status_code=502, detail=str(exc)) from exc

    @app.post("/api/decks/{owner}/{slug}/argue")
    def claude_argue(owner: str, slug: str, payload: dict[str, Any],
                     lib: Lib, caller: Scope) -> dict[str, Any]:
        """The case against one card's slot (ADR 25). Returns charges.

        Deliberately the interview's twin, down to the status codes: the two
        per-card modes differ in what they answer, not in how they are asked,
        and a client driving one should not have to learn a second shape.

        Synchronous, and that is a measured claim rather than an assumption
        this time. The interview costs ~4,900 input tokens and makes no tool
        calls because `brief()` hands the facts over; this mode shares that
        brief and adds a tool set it uses only when it goes shopping, so it
        sits in the same seconds-scale class rather than the theme proposal's
        minutes. **If that ever stops being true, this is the endpoint to
        move** -- ADR 20's lesson is that a duration measured for one surface
        is a question to ask of every sibling, and the docstring that says "it
        is a few seconds" without a number is the one that broke the dossier.
        """
        from mtglab.claude.argue import CardNotInDeck
        from mtglab.claude.client import ClaudeUnavailable

        card = str(payload.get("card", "")).strip()
        if not card:
            raise HTTPException(status_code=422, detail="card is required")
        try:
            return service.claude_argue(
                slug=slug, card=card,
                requested=payload.get("stance") or None,
                focus=str(payload.get("focus") or ""),
                source=lib.source_for(owner),
                tier=caller.model_tier)
        except CardNotInDeck as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ValueError as exc:
            # A malformed stance. `CardNotInDeck` is a ValueError too, which is
            # why it is caught above this rather than below it.
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        except service.ClaudeFailed as exc:
            raise HTTPException(status_code=502, detail=str(exc)) from exc

    @app.post("/api/decks/{owner}/{slug}/argue/deck")
    def claude_argue_deck(owner: str, slug: str, payload: dict[str, Any],
                          lib: Lib, caller: Scope) -> dict[str, Any]:
        """The slot argument, swept over a selection. Returns a **job**.

        One Claude call per selected card, so this is minutes the moment the
        selection is more than a handful -- the single-card endpoint above
        stays synchronous on its measured seconds, and this is the sibling
        that was never going to fit under the transport ceiling. See
        `api/argueruns.py` for the sweep's shape: one job, sequential, with
        progress, partial results kept, and an in-flight dedupe on the
        selection so a double-click joins the run rather than paying twice.

        The refusals stay here, per the planning-in-the-request rule: 422 for
        an empty selection, a card the deck does not hold (named), or a
        malformed stance; 404 for a deck this caller cannot see; 503 when
        there is no key. A stance of `off` comes back as a job born finished.
        """
        from mtglab.api.argueruns import plan_review
        from mtglab.claude.client import ClaudeUnavailable

        try:
            plan = plan_review(
                slug=slug, cards=payload.get("cards"),
                requested=payload.get("stance") or None,
                source=lib.source_for(owner),
                tier=caller.model_tier)
        except ValueError as exc:
            # `CardNotInDeck` is a ValueError and names the missing cards.
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        return _job_for(plan, caller).as_dict()

    @app.get("/api/decks/{owner}/{slug}/dossier")
    def claude_dossier_cached(owner: str, slug: str, lib: Lib) -> dict[str, Any]:
        """A stored commander dossier, or an empty one. Never calls Anthropic.

        A GET on purpose, and a *different function* from the POST below rather
        than the same one with a flag: this one is free and idempotent, so the
        deck page can ask for it on every load, and no amount of refreshing can
        turn it into spend.
        """
        return service.claude_dossier_cached(slug=slug, source=lib.source_for(owner))

    @app.post("/api/decks/{owner}/{slug}/dossier")
    def claude_dossier(owner: str, slug: str, payload: dict[str, Any],
                       lib: Lib, caller: Scope) -> dict[str, Any]:
        """Write the commander dossier (ADR 19). Returns a **job**, not a dossier.

        Measured at 236 seconds on the deployed instance — longer than the
        theme proposal above, which has been a job since #60. This one was left
        synchronous because nobody re-measured it, and what that looked like in
        a browser was a spinner and then `Load failed`: a transport error, with
        no line in the access log, because uvicorn writes one when a response
        completes and this one never did. The work itself was fine — it was
        sitting in `dossier_cache` while the page showed a failure. See
        `api/dossierruns.py`.

        The refusals stay here, which is the whole point of planning in the
        request. 422 when the deck has no commander the pool can find, which
        is a fact about the deck rather than a failure of the model and a poor
        thing to wait four minutes to be told; 503 when there is no key. What
        is *no longer* here is the 502: a call that came back unusable is now a
        job in state `error`, which is the right place for it, because by then
        the response has long since been sent.

        A stored dossier still answers instantly, as a job born finished — the
        `GET` beside this remains the free way to ask.
        """
        from mtglab.api.dossierruns import plan_dossier
        from mtglab.claude.client import ClaudeUnavailable
        from mtglab.claude.dossier import NoCommander

        try:
            plan = plan_dossier(
                slug=slug,
                requested=payload.get("stance") or None,
                refresh=bool(payload.get("refresh")),
                source=lib.source_for(owner),
                tier=caller.model_tier)
        except NoCommander as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        return _job_for(plan, caller).as_dict()

    # ---------------------------------------------------------- colours

    @app.get("/api/colors")
    def color_taxonomy() -> dict[str, Any]:
        """The 32 combinations, the five colours and the three eras.

        No card pool, no deck source, no network -- so this is the one deck-facing
        page that works on a fresh clone before `data refresh` has ever run.
        """
        return service.color_taxonomy()

    @app.get("/api/colors/progress")
    def challenge_progress(decks: Decks) -> dict[str, Any]:
        """Which of the 32 slots the library has filled. The 32 Deck
        Challenge, scored against the same table."""
        return service.challenge_progress(source=decks)

    # Declared after `/progress`, and it has to stay that way: FastAPI matches
    # in declaration order, so a `{key}` route above it would swallow the
    # literal path.
    @app.get("/api/colors/{key}")
    def combination_detail(key: str) -> dict[str, Any]:
        """One combination, with its champions and signature cards resolved
        against the pool. The teaching depth behind a slot."""
        try:
            return service.combination_detail(key)
        except KeyError as exc:
            raise HTTPException(
                status_code=404,
                detail=f"no colour combination {key!r}") from exc

    @app.get("/api/glossary")
    def glossary() -> dict[str, Any]:
        """The vocabulary. Reference data, like `/api/colors` -- no card pool, no
        deck source, no network."""
        return service.glossary()

    @app.get("/api/lore")
    def lore_shelves() -> dict[str, Any]:
        """The fact volumes (second 2026-08-15 punch list, item 7). Reference
        prose like the glossary, plus pool-resolved cards like
        `/api/colors/{key}` -- and like that route it answers without a pool,
        with the cards absent and counted for."""
        return service.lore_shelves()

    # ------------------------------------------------- card-art motion
    #
    # ADR 32's serving half, and deliberately the *only* half the app has:
    # a derivative either sits in `data/cache/cardmotion/` (put there by a
    # dev-machine `mtglab cardmotion build`, pushed over sftp deployed) or
    # it does not exist. No request triggers generation, so these routes
    # need no job pool, no dedup key, and no model anywhere near them.

    @app.get("/api/art/motion/{oracle_id}/{effect}")
    def art_motion_status(oracle_id: str, effect: str,
                          art: str | None = None) -> dict[str, Any]:
        """Is there a motion derivative for this painting? `ready: false` is
        a complete, correct answer -- the client shows the still it already
        has, which is the current page, not an error. `art` is the crop the
        page is showing: a deck that picked a printing must not be handed a
        loop derived from a different painting, so a mismatch is `ready:
        false`, exactly as if nothing had been built."""
        from urllib.parse import quote

        from mtglab.cardmotion import cache as cardmotion_cache
        from mtglab.cardmotion.effects import EFFECTS

        chosen = EFFECTS.get(effect)
        if chosen is None:
            raise HTTPException(status_code=404,
                                detail=f"no effect {effect!r}")
        hit = cardmotion_cache.find_ready(oracle_id, chosen, art)
        if hit is None:
            return {"ready": False, "effect": effect}
        meta = hit.attribution()
        stamp = meta.get("fingerprint", "")
        base = f"/api/art/motion/{oracle_id}/{effect}"
        # The art rides on the file URLs too: two printings of one commander
        # are two derivatives under one oracle_id, and the file route must
        # land on the same one the status answer described.
        suffix = f"&art={quote(art, safe='')}" if art else ""
        keys = {"loop.webm": "webm", "loop.mp4": "mp4",
                "poster.webp": "poster", "depth.png": "depth"}
        urls = {keys[name]: f"{base}/{name}?v={stamp}{suffix}"
                for name in sorted(cardmotion_cache.SERVABLE)
                if hit.file(name).exists()}
        return {"ready": True, "effect": effect, "fingerprint": stamp,
                "urls": urls, "attribution": meta}

    _ART_MOTION_TYPES = {"loop.webm": "video/webm", "loop.mp4": "video/mp4",
                         "poster.webp": "image/webp",
                         "depth.png": "image/png"}

    @app.get("/api/art/motion/{oracle_id}/{effect}/{filename}")
    def art_motion_file(oracle_id: str, effect: str, filename: str,
                        art: str | None = None) -> FileResponse:
        """One derivative file. Long-lived caching is safe because the
        status payload's URLs carry the fingerprint as a version stamp -- a
        regenerated derivative is a different URL, never a stale hit. The
        media type is explicit because the container has no /etc/mime.types
        (the tarot lesson)."""
        from mtglab.cardmotion import cache as cardmotion_cache
        from mtglab.cardmotion.effects import EFFECTS

        chosen = EFFECTS.get(effect)
        media_type = _ART_MOTION_TYPES.get(filename)
        # The allowlist is the path-traversal guard: `filename` never
        # reaches the filesystem unless it is one of four fixed names.
        if chosen is None or media_type is None:
            raise HTTPException(status_code=404, detail="no such derivative")
        hit = cardmotion_cache.find_ready(oracle_id, chosen, art)
        if hit is None or not hit.file(filename).exists():
            raise HTTPException(status_code=404, detail="no such derivative")
        return FileResponse(
            hit.file(filename), media_type=media_type,
            headers={"Cache-Control": "public, max-age=31536000, immutable"})

    # ----------------------------------------------------- mana symbols
    #
    # ADR 33: the official symbols, filled into a runtime cache on first
    # ask and served first-party ever after. The client's drawn glyphs are
    # the fallback, so a 404 here is a complete answer, not an error the
    # user sees — same contract as the motion routes above.

    @app.get("/api/symbols/{code}.svg")
    def symbol_svg(code: str) -> FileResponse:
        """One official mana-symbol SVG. `code` is the braced symbol with
        its punctuation dropped ({W/U} -> WU); the module's shape check is
        the path-traversal guard, so nothing unvetted reaches the
        filesystem. A week of caching rather than immutable: there is no
        version stamp to bust with, and the set of symbols moves a few
        times a year."""
        from mtglab import symbols as symbolcache

        path = symbolcache.ensure(code.upper())
        if path is None:
            raise HTTPException(status_code=404, detail="no such symbol")
        return FileResponse(
            path, media_type="image/svg+xml",
            headers={"Cache-Control": "public, max-age=604800"})

    @app.get("/api/ocr/{name}")
    def ocr_asset(name: str) -> FileResponse:
        """One file of the reading engine, off the shelf `ocr.py` fills.

        Six megabytes of WebAssembly and trained data that git never holds
        and no page ever hotlinks -- the mana-symbol arrangement (ADR 33)
        applied to somebody else's compiler output. The name must be a key
        of `ocr.ASSETS`, which is the whole path-traversal story: there is
        no pattern to get wrong, only a table to be absent from.

        Immutable, unlike the symbols beside it, and that is the digest's
        doing: the cache path carries the pinned versions, so these bytes
        can never mean anything else.
        """
        from mtglab import ocr as ocrshelf

        path = ocrshelf.ensure(name)
        if path is None:
            raise HTTPException(status_code=404, detail="no such reader file")
        asset = ocrshelf.ASSETS[name]
        return FileResponse(
            path, media_type=asset.media_type,
            headers={"Cache-Control": "public, max-age=31536000, immutable"})

    # -------------------------------------------------------------- sim

    # The job closures capture `decks` and run after the response has been
    # sent. That is safe because a DeckSource is a locator rather than a
    # connection -- see the note in `decks/source.py`, which is a constraint on
    # future implementations, not an accident of this one.

    # A job belongs to whoever submitted it. The four routes below are the only
    # user-scoped resource in the app today, and they take the scope rather
    # than reading the registry directly -- `jobs.get` does the filtering, so
    # "whose is this" is answered in one place (ADR 5).

    # Both sim routes plan before they queue. A plan compiles the deck -- a
    # parse and one indexed pool query -- and asks the cache whether these
    # exact numbers already exist; if they do the job is born finished and the
    # response is the same shape it always was, just with `status: "done"` on
    # the first read. The cache is global rather than per-user on purpose: it
    # is keyed on a hash of the compiled deck, so two callers share an entry
    # only when they are asking for the identical simulation, and the answer
    # carries nothing about whose deck produced it.

    @app.post("/api/sim/mana")
    def sim_mana(payload: dict[str, Any], lib: Lib,
                 caller: Scope) -> dict[str, Any]:
        """Queue a Tier 1 mana run against one deck.

        The slug rides in the payload rather than the path, which is why this
        needs `owner` in the payload too (ADR 22). Resolving a bare slug would
        reach a deck by name with nobody asked whose it is — and somebody
        else's private deck must answer 404 here exactly as it does on `GET`.
        Absent, `owner` means the caller's own library, which is what every
        existing client sends.
        """
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        owner = str(payload.get("owner") or lib.my_owner)
        from mtglab.api.simruns import plan_mana
        return _job_for(plan_mana(slug, payload, source=lib.source_for(owner)),
                        caller).as_dict()

    @app.post("/api/sim/lands")
    def sim_lands(payload: dict[str, Any], lib: Lib,
                  caller: Scope) -> dict[str, Any]:
        """Queue a Tier 1 land sweep. `owner` as for `/api/sim/mana` above."""
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        owner = str(payload.get("owner") or lib.my_owner)
        from mtglab.api.simruns import plan_lands
        return _job_for(plan_lands(slug, payload, source=lib.source_for(owner)),
                        caller).as_dict()

    @app.post("/api/sim/shelf")
    def sim_shelf(payload: dict[str, Any], lib: Lib) -> dict[str, Any]:
        """The closed form for one deck (Tier 1.5), computed in the request.

        The one simulation route that is **not** a job, and the reason is a
        measurement rather than a preference: `karsten.shelf` is arithmetic
        over an already-compiled deck and runs in 0.03-0.04s, where a Tier 1
        run is eighteen seconds. `api/shelfruns.py`'s docstring carries the
        numbers. `owner` resolves as it does for `/api/sim/mana`.
        """
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        owner = str(payload.get("owner") or lib.my_owner)
        from mtglab.api.shelfruns import shelf_result
        try:
            return shelf_result(slug, payload, source=lib.source_for(owner))
        except FileNotFoundError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc
        except RuntimeError as exc:
            # No pool, or a deck that cannot compile. 422 rather than 500:
            # this is a fact about the request's deck, not a broken server.
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.post("/api/sim/policy")
    def sim_policy(payload: dict[str, Any], lib: Lib,
                   caller: Scope) -> dict[str, Any]:
        """Queue a mulligan policy search. `owner` as for `/api/sim/mana`.

        A job, unlike the shelf above, because it is thirty-three seeded Tier 1
        runs and takes about fifty seconds.
        """
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        owner = str(payload.get("owner") or lib.my_owner)
        from mtglab.api.shelfruns import plan_policy
        return _job_for(plan_policy(slug, payload, source=lib.source_for(owner)),
                        caller).as_dict()

    @app.get("/api/forge")
    def forge_status() -> dict[str, Any]:
        """Is Tier 3 reachable from this process? (ADR 35.)

        The gate the Simulator asks before it offers real games — the same
        contract as `/api/claude`: a fact about the environment, reaching no
        network and booting no JVM. Where the answer is no, the mode is
        honestly absent (the Ask Claude rule), and `why` is maintainer-facing
        prose the client must not render.
        """
        from mtglab.api.forgeruns import status
        return status()

    @app.post("/api/sim/forge")
    def sim_forge(payload: dict[str, Any], lib: Lib,
                  caller: Scope) -> dict[str, Any]:
        """Queue one heads-up Forge match (ADR 35). Returns a **job**.

        Everything refusable is refused here, not in the job (the
        `themeruns` division): decks that do not resolve are 404 via the
        `DeckNotFound` handler, an uninstalled Forge is 503, and a deck with
        cards Forge does not implement is a 422 that names them — because a
        Forge game *plays on* without them and reports a winner, which is
        the one failure this surface exists to never serve.
        """
        from mtglab.api.forgeruns import plan_forge, status
        from mtglab.sim.tier3 import worker
        from mtglab.sim.tier3.coverage import ForgeNotInstalled
        from mtglab.sim.tier3.run import CoverageFailed, check_coverage

        pairs = []
        for side in ("a", "b"):
            slug = payload.get(f"{side}_slug")
            if not slug:
                raise HTTPException(status_code=422,
                                    detail=f"{side}_slug is required")
            owner = str(payload.get(f"{side}_owner") or lib.my_owner)
            pairs.append((owner, str(slug)))

        gate = status()
        if not gate["available"]:
            raise HTTPException(status_code=503, detail=gate["why"])

        decks = [lib.source_for(owner).get(slug) for owner, slug in pairs]
        addresses = [f"{owner}/{slug}" for owner, slug in pairs]
        # For the match ledger: the same ownership key the activity log uses
        # (an owner_id, NULL for the file tier — never the URL's owner
        # segment, which is not stable across configurations).
        owner_ids = [service.owner_id_of(lib.source_for(owner))
                     for owner, _slug in pairs]
        try:
            # The pre-flight runs where the card scripts live: against the
            # local zip, or on the worker machine (which this wakes — the
            # one request-thread cost the hosted shape adds, bounded by
            # `worker.BOOT_SECONDS` so a machine that will not come up is a
            # 503 rather than a hang).
            if worker.configured():
                worker.check_coverage(decks)
            else:
                check_coverage(decks)
        except CoverageFailed as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ForgeNotInstalled as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc

        return _job_for(plan_forge(decks, addresses, payload, owner_ids),
                        caller).as_dict()

    @app.get("/api/jobs")
    def list_jobs(caller: Scope) -> list[dict[str, Any]]:
        return [j.as_dict() for j in jobs.all_jobs(owner=caller.user_id)]

    @app.get("/api/jobs/{job_id}")
    def get_job(job_id: str, caller: Scope) -> dict[str, Any]:
        # 404 rather than 403 for somebody else's job, so an id cannot be
        # probed for existence. `jobs.get` has already collapsed the two cases.
        job = jobs.get(job_id, owner=caller.user_id)
        if job is None:
            raise HTTPException(status_code=404, detail=f"no job '{job_id}'")
        return job.as_dict()

    # ----------------------------------------------------------- static

    # Mounted before the SPA catch-all below, which would otherwise hand a
    # missing card the HTML shell with a 200 -- an `<img>` that "loads" and
    # shows nothing. Same reason `/assets` sits here.
    #
    # `Revalidated` for consistency rather than necessity: unlike the bundle
    # these files genuinely never change, so a long immutable cache would be
    # defensible. It is not worth a second caching policy to argue about, and
    # only the three dealt cards are ever fetched.
    if TAROT.is_dir():
        app.mount("/tarot", Revalidated(directory=TAROT), name="tarot")

    if WEB_DIST.is_dir():
        assets = WEB_DIST / "assets"
        if assets.is_dir():
            app.mount("/assets", Revalidated(directory=assets), name="assets")

        # The bundle's own root files (index.html, and any favicon / manifest /
        # robots.txt a build adds), mapped name -> path once from the trusted
        # directory. The catch-all serves one only by looking the request up as
        # a key here -- subdirectories are served by their own mounts above --
        # so the path handed to FileResponse comes from this listing, never
        # built out of the request. That is the containment (an exact key match
        # cannot be walked out of the tree with `..`) and the reason the lookup
        # no longer trips the path-injection scanner: the served path carries no
        # user input at all, where the earlier `WEB_DIST / full_path` did.
        root_files = {p.name: p for p in WEB_DIST.iterdir() if p.is_file()}

        @app.get("/{full_path:path}")
        def spa(full_path: str) -> FileResponse:
            """Serve the built app, letting the client router own real paths.

            A miss under /api lands here too -- FastAPI falls through to the
            catch-all rather than 404ing on the prefix -- and it must be
            refused as JSON, not served the shell. A client that mistypes an
            endpoint should get an error it can read, never a 200 carrying a
            web page.

            Checked against the *normalised* path, through the same helper the
            auth middleware uses, because a naive prefix test is more
            permissive than the router: `//api/decks` and `/api/./decks` do
            not start with `api/` and would be served the shell.
            """
            normalised = auth.normalise_path(full_path)
            if normalised == "/api" or normalised.startswith("/api/"):
                raise HTTPException(status_code=404,
                                    detail=f"no such endpoint: {normalised}")
            # Serve a bundle root file only by exact key match. A raw traversal
            # path (`../../../etc/hosts`) reaches this handler un-normalised --
            # the same way `//api` does above -- and simply is not one of the
            # known names, so the lookup misses and it falls through to the
            # shell. The served path is the trusted listing's value, not the
            # request.
            target = root_files.get(full_path)
            if target is not None:
                return FileResponse(target, headers=NO_CACHE)
            # The shell above all: it is what names the asset files, so a
            # stale one pins every other stale thing in place.
            return FileResponse(WEB_DIST / "index.html", headers=NO_CACHE)

    return app


app = create_app()
