"""FastAPI application.

Routes are deliberately thin: parse, delegate to `service.py`, serialise. The
logic they call is the same code the CLI uses, so the app and the terminal can
never drift into disagreeing about a deck.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from pathlib import Path
from typing import Annotated, Any

from fastapi import Depends, FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles

import mtglab
from mtglab import config
from mtglab.api import admin, auth, jobs, service
from mtglab.api.deps import Scope, UserScope, deck_source
from mtglab.auth import bootstrap
from mtglab.auth.mail import EmailSender
from mtglab.decks.source import DeckNotFound, DeckSource

# The request scope, as one annotation. Every deck-facing route takes it, so
# when auth arrives the change is to `deps.deck_source` and nowhere else.
Decks = Annotated[DeckSource, Depends(deck_source)]

WEB_DIST = Path(__file__).resolve().parent.parent / "web_dist"

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

    Both producers of a `Plan` come through here -- `api/simruns.py` for Tier 1
    and `api/themeruns.py` for the theme proposal -- and neither route decides
    which pool the work belongs in. The plan carries its own `lane` because
    that is a property of the work rather than of the route, and a route is
    exactly the place somebody would eventually forget to pass it.
    """
    if plan.result is not None:
        return jobs.completed(plan.kind, result=plan.result,
                              label=plan.label, owner=caller.user_id)
    return jobs.submit(plan.kind, plan.run, label=plan.label,
                       owner=caller.user_id, lane=plan.lane)


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

    app = FastAPI(title="sylvan-library", version=mtglab.__version__,
                  description="Local Commander deckbuilding and simulation.",
                  lifespan=startup)

    # First, and before any route is declared: the middleware runs ahead of
    # routing, so what it protects is every path the app will ever serve rather
    # than the ones remembered at review time.
    auth.install(app, require=required, secure_cookies=secure,
                 email_sender=email_sender)
    admin.install(app, email_sender=email_sender)

    if dev:
        # Vite dev server runs on another port, so the browser needs CORS. Only
        # in dev -- the built app is same-origin and needs none of this.
        app.add_middleware(
            CORSMiddleware,
            allow_origins=["http://localhost:5173", "http://127.0.0.1:5173"],
            allow_methods=["*"], allow_headers=["*"],
        )

    @app.exception_handler(DeckNotFound)
    async def _deck_missing(_request, exc: DeckNotFound):
        return JSONResponse(status_code=404,
                            content={"detail": f"no deck '{exc}'"})

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
    def list_decks(decks: Decks) -> list[dict[str, Any]]:
        return service.list_decks(source=decks)

    @app.post("/api/decks/import")
    def import_deck(payload: dict[str, Any], decks: Decks) -> dict[str, Any]:
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
                text=str(payload.get("text", "")),
                slug=str(payload.get("slug", "")),
                name=str(payload.get("name", "")),
                commander=[str(c) for c in commander],
                companion=str(payload.get("companion") or ""),
                bracket=int(bracket) if bracket not in (None, "") else None,
                status=str(payload.get("status") or "theoretical"),
                dry_run=bool(payload.get("dry_run")),
                source=decks,
            )
        except (ValueError, TypeError) as exc:
            raise HTTPException(status_code=422,
                                detail=f"bracket must be a number: {exc}") from exc
        except service.ImportRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.post("/api/decks")
    def create_deck(payload: dict[str, Any], decks: Decks) -> dict[str, Any]:
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
                slug=str(payload.get("slug", "")),
                name=str(payload.get("name", "")),
                commander=[str(c) for c in commander],
                companion=str(payload.get("companion") or ""),
                bracket=int(bracket) if bracket not in (None, "") else None,
                status=str(payload.get("status") or "theoretical"),
                source=decks,
            )
        except (ValueError, TypeError) as exc:
            raise HTTPException(status_code=422,
                                detail=f"bracket must be a number: {exc}") from exc
        except service.CreateRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/decks/{slug}")
    def get_deck(slug: str, decks: Decks) -> dict[str, Any]:
        return service.get_deck(slug, source=decks)

    @app.delete("/api/decks/{slug}")
    def delete_deck(slug: str, decks: Decks,
                    confirm: str = Query("", description="must equal the slug"),
                    ) -> dict[str, Any]:
        """Remove a deck from the library, recoverably.

        `confirm` is a query parameter and must equal the slug. A DELETE with
        no body is the conventional shape, and making the confirmation a value
        only somebody looking at the right deck can produce is what stops a
        mis-aimed request from being indistinguishable from an intended one.
        """
        try:
            return service.delete_deck(slug=slug, confirm=confirm, source=decks)
        except service.DeleteRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/decks/{slug}/validate")
    def validate_deck(slug: str, decks: Decks) -> dict[str, Any]:
        return service.validate_deck(slug, source=decks)

    @app.get("/api/decks/{slug}/stats")
    def deck_stats(slug: str, decks: Decks) -> dict[str, Any]:
        return service.stats_for(slug, source=decks)

    @app.post("/api/decks/{slug}/swap")
    def swap_card(slug: str, payload: dict[str, Any], decks: Decks) -> dict[str, Any]:
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
                source=decks,
            )
        except service.SwapRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    # The rest of the edit operations (ADR 12). Each is one narrow write that
    # re-runs the gate and reports it, so the app can never leave a deck
    # changed and unchecked. None of them may author a rationale: `why` is
    # whatever the caller typed, and an empty one on a curated deck is a 422
    # rather than a blank the tool fills in.

    @app.post("/api/decks/{slug}/cards")
    def add_card(slug: str, payload: dict[str, Any], decks: Decks) -> dict[str, Any]:
        try:
            return service.add_card(
                slug,
                name=str(payload.get("name", "")),
                category=str(payload.get("category", "")),
                why=str(payload.get("why") or ""),
                qty=int(payload.get("qty") or 1),
                to=str(payload.get("to") or "cards"),
                source=decks,
            )
        except (TypeError, ValueError) as exc:
            raise HTTPException(status_code=422,
                                detail=f"qty must be a number: {exc}") from exc
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.delete("/api/decks/{slug}/cards/{name}")
    def remove_card(slug: str, name: str, decks: Decks) -> dict[str, Any]:
        try:
            return service.remove_card(slug, name=name, source=decks)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.patch("/api/decks/{slug}/cards/{name}")
    def set_card_field(slug: str, name: str, payload: dict[str, Any],
                       decks: Decks) -> dict[str, Any]:
        """Change one field of one card: its category, quantity or rationale.

        The rationale editor's write path. A PATCH of one field rather than a
        PUT of the card, because a card is mostly corpus facts and the deck
        file only carries the handful of things a person decided.
        """
        field = str(payload.get("field", ""))
        if "value" not in payload:
            raise HTTPException(status_code=422, detail="value is required")
        try:
            return service.set_card_field(slug, name=name, field=field,
                                          value=payload["value"], source=decks)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.patch("/api/decks/{slug}")
    def set_deck_field(slug: str, payload: dict[str, Any],
                       decks: Decks) -> dict[str, Any]:
        """Change one of the deck's own fields: stage, status or bracket.

        `stage` to `curated` is promotion, the last step of an import. It is
        refused while any card still lacks a rationale, so the deck is never
        written into a state the gate would immediately reject.
        """
        if "value" not in payload:
            raise HTTPException(status_code=422, detail="value is required")
        try:
            return service.set_deck_field(slug, field=str(payload.get("field", "")),
                                          value=payload["value"], source=decks)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.put("/api/decks/{slug}/notes/{key}")
    def set_note(slug: str, key: str, payload: dict[str, Any],
                 decks: Decks) -> dict[str, Any]:
        try:
            return service.set_note(slug, key=key,
                                    value=str(payload.get("value", "")),
                                    source=decks)
        except service.EditRejected as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/decks/{slug}/suggestions")
    def deck_suggestions(slug: str, decks: Decks,
                         limit: int = Query(5, ge=1, le=20)) -> dict[str, Any]:
        return service.suggestions_for(slug, source=decks, limit=limit)

    @app.get("/api/decks/{slug}/commander")
    def deck_commander(slug: str, decks: Decks) -> dict[str, Any]:
        """Who leads this deck, and what the corpus knows about them.

        Its own route rather than more fields on `GET /api/decks/{slug}`,
        for two reasons. It runs several extra queries — a count per subtype,
        a name search, a printing lookup — to fill a panel that is decorative,
        and the deck page should not wait on any of that to render its 99.
        And it answers with `card: null` rather than a 404 when there is no
        corpus, which is a different contract from the deck itself.
        """
        return service.commander_dossier(slug, source=decks)

    @app.get("/api/decks/{slug}/printings")
    def deck_printings(slug: str, decks: Decks) -> dict[str, Any]:
        """Every non-digital printing of this deck's commander, newest first.

        Its own route rather than fields on the deck: Goreclaw has twelve and
        most decks never open the picker, so this is a query the deck page
        should not pay for on every load.
        """
        return service.commander_printings(slug, source=decks)

    # ------------------------------------------------------------ cards

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
    def claude_status(decks: Decks, stance: str = "",
                      slug: str = "") -> dict[str, Any]:
        """Is the Claude surface installed, configured, and switched on?

        Three separate answers — a UI that collapses them tells someone their
        key is missing when actually they turned it off. Reaches no network:
        the stance is deterministic and availability is a fact about the
        environment.
        """
        try:
            return service.claude_status(
                requested=stance or None, slug=slug or None, source=decks)
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.post("/api/claude/theme")
    def claude_theme(payload: dict[str, Any]) -> dict[str, Any]:
        """One turn of the theme interview (ADR 20).

        Takes no deck and no `Decks` dependency, and that absence is the
        feature: this surface exists to help somebody *start* a deck, and a
        mode that cannot reach a deck cannot critique one.

        The transcript is the client's — ADR 20 keeps conversation state off
        the server — so this endpoint is the door. It takes plain
        `{role, text}` turns and never Anthropic message blocks; an endpoint
        that accepted those would be a free proxy for somebody else's spend.
        `check_transcript` refuses everything else as a 422.
        """
        from mtglab.claude.client import ClaudeUnavailable
        from mtglab.claude.theme import TranscriptRejected

        try:
            return service.claude_theme_ask(
                transcript=payload.get("transcript"),
                slots=payload.get("slots"),
                requested=payload.get("stance") or None)
        except (TranscriptRejected, ValueError) as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        except service.ClaudeFailed as exc:
            raise HTTPException(status_code=502, detail=str(exc)) from exc

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
                avoid=str(payload.get("avoid") or ""))
        except NotReady as exc:
            raise HTTPException(status_code=409, detail=str(exc)) from exc
        except (TranscriptRejected, ValueError) as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        except ClaudeUnavailable as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc
        return _job_for(plan, caller).as_dict()

    @app.post("/api/decks/{slug}/interview")
    def claude_interview(slug: str, payload: dict[str, Any],
                         decks: Decks) -> dict[str, Any]:
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
                source=decks)
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

    @app.get("/api/decks/{slug}/dossier")
    def claude_dossier_cached(slug: str, decks: Decks) -> dict[str, Any]:
        """A stored commander dossier, or an empty one. Never calls Anthropic.

        A GET on purpose, and a *different function* from the POST below rather
        than the same one with a flag: this one is free and idempotent, so the
        deck page can ask for it on every load, and no amount of refreshing can
        turn it into spend.
        """
        return service.claude_dossier_cached(slug=slug, source=decks)

    @app.post("/api/decks/{slug}/dossier")
    def claude_dossier(slug: str, payload: dict[str, Any],
                       decks: Decks, caller: Scope) -> dict[str, Any]:
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
        request. 422 when the deck has no commander the corpus can find, which
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
                source=decks)
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

        No corpus, no deck source, no network -- so this is the one deck-facing
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
        against the corpus. The teaching depth behind a slot."""
        try:
            return service.combination_detail(key)
        except KeyError as exc:
            raise HTTPException(
                status_code=404,
                detail=f"no colour combination {key!r}") from exc

    @app.get("/api/glossary")
    def glossary() -> dict[str, Any]:
        """The vocabulary. Reference data, like `/api/colors` -- no corpus, no
        deck source, no network."""
        return service.glossary()

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
    # parse and one indexed corpus query -- and asks the cache whether these
    # exact numbers already exist; if they do the job is born finished and the
    # response is the same shape it always was, just with `status: "done"` on
    # the first read. The cache is global rather than per-user on purpose: it
    # is keyed on a hash of the compiled deck, so two callers share an entry
    # only when they are asking for the identical simulation, and the answer
    # carries nothing about whose deck produced it.

    @app.post("/api/sim/mana")
    def sim_mana(payload: dict[str, Any], decks: Decks,
                 caller: Scope) -> dict[str, Any]:
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        from mtglab.api.simruns import plan_mana
        return _job_for(plan_mana(slug, payload, source=decks), caller).as_dict()

    @app.post("/api/sim/lands")
    def sim_lands(payload: dict[str, Any], decks: Decks,
                  caller: Scope) -> dict[str, Any]:
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        from mtglab.api.simruns import plan_lands
        return _job_for(plan_lands(slug, payload, source=decks), caller).as_dict()

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

    if WEB_DIST.is_dir():
        assets = WEB_DIST / "assets"
        if assets.is_dir():
            app.mount("/assets", Revalidated(directory=assets), name="assets")

        @app.get("/{full_path:path}")
        def spa(full_path: str):
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
            candidate = WEB_DIST / full_path
            if full_path and candidate.is_file():
                return FileResponse(candidate, headers=NO_CACHE)
            # The shell above all: it is what names the asset files, so a
            # stale one pins every other stale thing in place.
            return FileResponse(WEB_DIST / "index.html", headers=NO_CACHE)

    return app


app = create_app()
