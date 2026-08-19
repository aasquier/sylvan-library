"""What is worth timing, declared once so two runs measure the same thing.

A benchmark whose targets are chosen fresh each session is a benchmark with no
history, and history is the only thing that turns a millisecond into a
finding. So the suite is a list here rather than an argument at the prompt.

Two kinds, deliberately both. **Endpoints** are what a person waits on, and
they are the number that belongs in the ledger. **Library calls** are where
the time actually goes, and they are what moves when somebody optimises: an
endpoint that got slower with every library call unchanged is a serialisation
or a payload problem, which is a different investigation entirely.

Every target says what it needs. A run with no card pool skips most of this
suite and **says so, per target** -- a bench that quietly measures nothing is
the exact shape of failure this whole pass keeps finding, and a row reading
`skipped: no pool` is worth more than a table that silently shrank.
"""

from __future__ import annotations

from collections.abc import Callable, Iterator
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from mtglab import config

#: How slow a target has to be before `bench run` profiles it unasked.
#: Not a service-level objective -- a curiosity threshold. The 200ms deck
#: shelf sat in the ledger as a recorded datum for three days because nothing
#: said that a number this size is a question.
PROFILE_OVER_MS = 25.0


@dataclass(frozen=True)
class Target:
    """One thing to time, and what it needs in order to be timed at all."""

    name: str
    kind: str                      #: "endpoint" or "library"
    call: Callable[[], Any]
    note: str = ""
    #: Filled in when the target cannot run here; the run reports it verbatim
    #: rather than dropping the row.
    unavailable: str = ""


@dataclass
class _Bench:
    """The shared fixtures a suite needs, built at most once."""

    client: Any = None
    #: `(owner, slug)` pairs as the shelf itself reports them.
    slugs: list[tuple[str, str]] = field(default_factory=list)
    reason: str = ""


def _pool_present() -> bool:
    return Path(config.DB_PATH).exists()


def _build(owner: str = "local") -> _Bench:
    """A TestClient over the real app, or the reason there is not one.

    In-process on purpose. A bench that shells out to a running server
    measures the server *and* the socket *and* whatever else that process is
    doing, and -- the part that matters here -- it cannot empty a cache
    between samples, so it can never produce an honest cold number.
    """
    out = _Bench()
    try:
        from fastapi.testclient import TestClient

        from mtglab.api.app import create_app
    except Exception as exc:                                        # noqa: BLE001
        out.reason = f"no API extra ({type(exc).__name__})"
        return out
    try:
        out.client = TestClient(create_app())
        listed = out.client.get("/api/decks")
        if listed.status_code == 200:
            out.slugs = [(d.get("owner") or owner, d["slug"])
                         for d in listed.json()]
    except Exception as exc:                                        # noqa: BLE001
        out.reason = f"app would not start ({type(exc).__name__}: {exc})"
    if out.client is not None and not out.slugs:
        out.reason = out.reason or "no decks in the library"
    return out


def suite(*, owner: str = "local") -> list[Target]:
    """The declared suite, resolved against what this machine actually has."""
    return list(_endpoints(owner)) + list(_library())


def _endpoints(owner: str) -> Iterator[Target]:
    bench = _build(owner)
    if bench.client is None:
        reason = bench.reason or "no API"
        for name in ("GET /api/health", "GET /api/decks", "GET /api/lore",
                     "GET /api/colors", "GET /api/glossary",
                     "GET /api/cards/search?q=goblin"):
            yield Target(name, "endpoint", _never, unavailable=reason)
        return

    client = bench.client

    def get(path: str) -> Callable[[], Any]:
        return lambda: client.get(path)

    yield Target("GET /api/health", "endpoint", get("/api/health"),
                 "the cheapest route there is; anything here is overhead")
    yield Target("GET /api/decks", "endpoint", get("/api/decks"),
                 "the library shelf -- every deck parsed and every card "
                 "resolved, which is why it was the worst offender")
    yield Target("GET /api/lore", "endpoint", get("/api/lore"),
                 "checked-in prose plus ~120 card lookups")
    yield Target("GET /api/colors", "endpoint", get("/api/colors"),
                 "pure Python, no pool -- the control in this experiment")
    yield Target("GET /api/glossary", "endpoint", get("/api/glossary"),
                 "pure Python, no pool")
    yield Target("GET /api/cards/search?q=goblin", "endpoint",
                 get("/api/cards/search?q=goblin"),
                 "a text scan of oracle_text; never goes through get_cards, "
                 "so no memo can help it")

    if bench.slugs:
        deck_owner, slug = bench.slugs[0]
        yield Target(f"GET /api/decks/../{slug}", "endpoint",
                     get(f"/api/decks/{deck_owner}/{slug}"),
                     "one deck's detail payload")
        yield Target(f"GET /api/decks/../{slug}/validate", "endpoint",
                     get(f"/api/decks/{deck_owner}/{slug}/validate"),
                     "the gate over one deck")
    else:
        why = bench.reason or "no decks in the library"
        yield Target("GET /api/decks/../{slug}", "endpoint", _never,
                     unavailable=why)
        yield Target("GET /api/decks/../{slug}/validate", "endpoint",
                     _never, unavailable=why)


def _library() -> Iterator[Target]:
    """The calls underneath the endpoints, timed without the HTTP layer."""
    if not _pool_present():
        for name in ("db.get_cards (441 names)", "db.oracle_columns",
                     "db.connect_readonly", "db.search (goblin)"):
            yield Target(name, "library", _never,
                         unavailable=f"no card pool at {config.DB_PATH}")
    else:
        yield from _pool_targets()

    decks = [p for p in sorted(Path(config.DECKS_DIR).glob("*/deck.yaml"))
             if not p.parent.name.startswith("_")]
    if not decks:
        yield Target("Deck.load", "library", _never,
                     unavailable=f"no decks under {config.DECKS_DIR}")
        return

    from mtglab.decks.model import Deck
    path = decks[0]
    yield Target("Deck.load", "library", lambda: Deck.load(path),
                 "YAML parse plus the deepcopy the cache pays on every hit")


def _pool_targets() -> Iterator[Target]:
    from mtglab.cards import db

    names = _bench_names()
    yield Target(f"db.get_cards ({len(names)} names)", "library",
                 lambda: _with_con(lambda con: db.get_cards(con, names)),
                 "the shelf's own question, memoised on the pool stamp")
    yield Target("db.oracle_columns", "library",
                 lambda: _with_con(db.oracle_columns),
                 "a catalogue scan, cached per pool file")
    yield Target("db.connect_readonly", "library",
                 lambda: _with_con(lambda con: None),
                 "open and close; 17.5ms without the keeper, 0.7ms with it")


def _with_con(fn: Callable[[Any], Any]) -> Any:
    from mtglab.cards import db
    con = db.connect_readonly(config.DB_PATH)
    try:
        return fn(con)
    finally:
        con.close()


def _bench_names() -> list[str]:
    """The card names to look up: the biggest real deck's, or a stub.

    Real names rather than a generated list because the memo is keyed on the
    exact name tuple, so a synthetic list measures a key no page ever poses --
    a warm number for a question nobody asks. Biggest rather than first
    because `_template` is a deck too, and benchmarking the empty one would
    report the library as free.
    """
    best: list[str] = []
    for path in sorted(Path(config.DECKS_DIR).glob("*/deck.yaml")):
        try:
            from mtglab.decks.model import Deck
            deck = Deck.load(path)
        except Exception:                                           # noqa: BLE001
            continue
        names = [c.name for c in deck.cards] + list(deck.commander)
        if len(names) > len(best):
            best = names
    return best or ["Sol Ring", "Command Tower", "Arcane Signet"]


def _never() -> Any:
    """Stands in for a target that cannot run. Never called by the runner."""
    raise RuntimeError("this target is unavailable and must not be called")
