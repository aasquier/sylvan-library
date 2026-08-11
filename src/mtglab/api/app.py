"""FastAPI application.

Routes are deliberately thin: parse, delegate to `service.py`, serialise. The
logic they call is the same code the CLI uses, so the app and the terminal can
never drift into disagreeing about a deck.
"""

from __future__ import annotations

from pathlib import Path
from typing import Annotated, Any

from fastapi import Depends, FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles

from mtglab.api import jobs, service
from mtglab.api.deps import deck_source
from mtglab.decks.source import DeckNotFound, DeckSource

# The request scope, as one annotation. Every deck-facing route takes it, so
# when auth arrives the change is to `deps.deck_source` and nowhere else.
Decks = Annotated[DeckSource, Depends(deck_source)]

WEB_DIST = Path(__file__).resolve().parent.parent / "web_dist"


def create_app(*, dev: bool = False) -> FastAPI:
    app = FastAPI(title="sylvan-library", version="0.1.0",
                  description="Local Commander deckbuilding and simulation.")

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

    @app.get("/api/decks/{slug}")
    def get_deck(slug: str, decks: Decks) -> dict[str, Any]:
        return service.get_deck(slug, source=decks)

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

    @app.get("/api/decks/{slug}/suggestions")
    def deck_suggestions(slug: str, decks: Decks,
                         limit: int = Query(5, ge=1, le=20)) -> dict[str, Any]:
        return service.suggestions_for(slug, source=decks, limit=limit)

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
    ) -> dict[str, Any]:
        return service.search_cards(q=q, identity=identity, type_line=type_line,
                                    cmc_max=cmc_max, price_max=price_max,
                                    sort=sort, limit=limit)

    # -------------------------------------------------------------- sim

    # The job closures capture `decks` and run after the response has been
    # sent. That is safe because a DeckSource is a locator rather than a
    # connection -- see the note in `decks/source.py`, which is a constraint on
    # future implementations, not an accident of this one.

    @app.post("/api/sim/mana")
    def sim_mana(payload: dict[str, Any], decks: Decks) -> dict[str, Any]:
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        from mtglab.api.simruns import run_mana
        games = int(payload.get("games", 20_000))
        job = jobs.submit(
            "sim.mana",
            lambda progress: run_mana(slug, payload, progress, source=decks),
            label=f"{slug}: mana, {games:,} games")
        return job.as_dict()

    @app.post("/api/sim/lands")
    def sim_lands(payload: dict[str, Any], decks: Decks) -> dict[str, Any]:
        slug = payload.get("slug")
        if not slug:
            raise HTTPException(status_code=422, detail="slug is required")
        from mtglab.api.simruns import run_lands
        low = int(payload.get("low", 30))
        high = int(payload.get("high", 40))
        job = jobs.submit(
            "sim.lands",
            lambda progress: run_lands(slug, payload, progress, source=decks),
            label=f"{slug}: land sweep {low}-{high}")
        return job.as_dict()

    @app.get("/api/jobs")
    def list_jobs() -> list[dict[str, Any]]:
        return [j.as_dict() for j in jobs.all_jobs()]

    @app.get("/api/jobs/{job_id}")
    def get_job(job_id: str) -> dict[str, Any]:
        job = jobs.get(job_id)
        if job is None:
            raise HTTPException(status_code=404, detail=f"no job '{job_id}'")
        return job.as_dict()

    # ----------------------------------------------------------- static

    if WEB_DIST.is_dir():
        assets = WEB_DIST / "assets"
        if assets.is_dir():
            app.mount("/assets", StaticFiles(directory=assets), name="assets")

        @app.get("/{full_path:path}")
        def spa(full_path: str):
            """Serve the built app, letting the client router own real paths.

            Anything under /api has already matched above, so a miss here is a
            frontend route, not a missing endpoint.
            """
            candidate = WEB_DIST / full_path
            if full_path and candidate.is_file():
                return FileResponse(candidate)
            return FileResponse(WEB_DIST / "index.html")

    return app


app = create_app()
