"""Every golden record, named in one place.

Two kinds. `READS` are stateless requests -- one call, one record -- that
`test_contract_golden.py` runs parametrised. `SEQUENCES` are the records the
stateful scenarios in `test_contract_sequences.py` produce, in order: a deck
created, edited, built and deleted; a job submitted and polled; a session
opened and closed; the admin surface. Their names are declared here rather
than discovered by running, so the ghost check in `test_contract_golden.py`
can compare `golden/*.json` against this file without running anything, and
so a Go implementer can read the whole contract's table of contents in one
screen.

What is deliberately **not** here, and why:

- `/api/sets/upcoming` asks Scryfall at request time; its shape depends on
  the network the runner has. It is tested in `tests/test_api.py` with the
  fetch stubbed.
- `/api/symbols/{code}.svg` and `/api/ocr/{name}` fill a cache from the
  network on a first ask for a *known* code; only their refusal of an
  unknown one is pinned, because that is the only answer every machine
  gives the same.
- `/openapi.json`, `/docs` and `/redoc` are FastAPI's own surface, unread by
  the frontend. Pinning them would oblige the Go front door to generate an
  OpenAPI document; whether to serve, proxy or retire them is a decision
  for the port, flagged in docs/go-migration/PLAN.md, not a shape to freeze.
- Every Claude route is pinned at its *refusal* -- the 503 an absent key
  produces, the 422 a malformed request produces -- because the harness
  runs with the key blanked on purpose. The successful shapes are the
  response schemas' business (`tests/test_claude_*.py`) and a real
  conversation on the deployed pair is Phase 6's gate.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

from contract.harness import DECK_OWNER, DECK_SLUG

DECK = f"/api/decks/{DECK_OWNER}/{DECK_SLUG}"

#: The fields of a job whose kind cannot be pinned at the moment it is read:
#: an in-process client may see a 20-game run already finished where a
#: socket sees it queued. Presence is still required.
JOB_VOLATILE = ("status", "done", "total", "percent", "partial", "result",
                "error")

#: A nonsense oracle id, for the art routes: no derivative exists for it,
#: which is the answer being pinned.
NO_ORACLE = "0aae2e33-0000-4000-8000-000000000000"


def _first_bundle_asset() -> str:
    """One file of the committed bundle, so the static mount is pinned by a
    path that exists -- the hashed filename changes with every rebuild, the
    record's name does not."""
    from mtglab.api.app import WEB_DIST
    assets = sorted(p.name for p in (WEB_DIST / "assets").glob("*.js"))
    return assets[0] if assets else "index.js"


@dataclass(frozen=True)
class Case:
    family: str
    name: str
    method: str
    path: str
    user: str | None = "alice"          # None: no session
    json: Any = None
    content: bytes | None = None        # a raw body, for the not-JSON case
    params: dict[str, Any] | None = None
    expect: tuple[int, ...] = (200,)    # refuse to record anything else
    volatile: tuple[str, ...] = ()

    @property
    def id(self) -> str:
        return f"{self.family}/{self.name}"


def _reads() -> tuple[Case, ...]:
    r = "reads"
    return (
        # meta, and the two method answers a router must reproduce
        Case(r, "health", "GET", "/api/health", user=None),
        Case(r, "health-head", "HEAD", "/api/health", user=None, expect=(405,)),
        Case(r, "health-delete", "DELETE", "/api/health", user=None,
             expect=(405,)),
        Case(r, "me-anonymous", "GET", "/api/auth/me", user=None),
        # the library, file tier
        Case(r, "decks", "GET", "/api/decks"),
        Case(r, "deck", "GET", DECK),
        Case(r, "deck-missing", "GET", f"/api/decks/{DECK_OWNER}/no-such-deck",
             expect=(404,)),
        Case(r, "owner-missing", "GET", "/api/decks/nobody-here/whatever",
             expect=(404,)),
        Case(r, "validate", "GET", f"{DECK}/validate"),
        Case(r, "stats", "GET", f"{DECK}/stats"),
        Case(r, "log", "GET", f"{DECK}/log"),
        Case(r, "log-bad-limit", "GET", f"{DECK}/log", params={"limit": "abc"},
             expect=(422,)),
        Case(r, "artifacts-unbuilt", "GET", f"{DECK}/artifacts"),
        Case(r, "artifact-unbuilt", "GET", f"{DECK}/artifacts/primer-quick.md",
             expect=(404,)),
        Case(r, "suggestions", "GET", f"{DECK}/suggestions"),
        Case(r, "commander", "GET", f"{DECK}/commander"),
        Case(r, "printings", "GET", f"{DECK}/printings"),
        Case(r, "wheel", "POST", f"{DECK}/wheel", json={"seed": 7}),
        # the pool
        Case(r, "search", "GET", "/api/cards/search", params={"q": "sol"}),
        Case(r, "search-empty", "GET", "/api/cards/search",
             params={"q": "zzzz-no-such-card"}),
        Case(r, "identify", "POST", "/api/cards/identify",
             json={"sightings": [{"set": "LTC", "number": "284/281"}]}),
        Case(r, "identify-refused", "POST", "/api/cards/identify", json={},
             expect=(422,)),
        # reference prose
        Case(r, "colors", "GET", "/api/colors"),
        Case(r, "colors-progress", "GET", "/api/colors/progress"),
        Case(r, "colors-key", "GET", "/api/colors/G"),
        Case(r, "colors-missing", "GET", "/api/colors/nope", expect=(404,)),
        Case(r, "glossary", "GET", "/api/glossary"),
        Case(r, "lore", "GET", "/api/lore"),
        Case(r, "themes", "GET", "/api/themes"),
        # the Claude surface's configuration, and the tarot table
        Case(r, "claude", "GET", "/api/claude"),
        Case(r, "claude-for-deck", "GET", "/api/claude",
             params={"slug": DECK_SLUG, "owner": DECK_OWNER}),
        Case(r, "personas", "GET", "/api/claude/personas"),
        Case(r, "tarot", "GET", "/api/tarot/reading", params={"seed": 7}),
        Case(r, "tarot-bad-seed", "GET", "/api/tarot/reading",
             params={"seed": "abc"}, expect=(422,)),
        # the Forge gate, and the three runtime shelves at their refusals
        Case(r, "forge", "GET", "/api/forge"),
        Case(r, "symbol-missing", "GET", "/api/symbols/NOT-A-SYMBOL.svg",
             expect=(404,)),
        Case(r, "ocr-missing", "GET", "/api/ocr/no-such-file", expect=(404,)),
        Case(r, "art-status", "GET", f"/api/art/motion/{NO_ORACLE}/depth-drift"),
        Case(r, "art-bad-effect", "GET", f"/api/art/motion/{NO_ORACLE}/nope",
             expect=(404,)),
        Case(r, "art-file-missing", "GET",
             f"/api/art/motion/{NO_ORACLE}/depth-drift/loop.webm",
             expect=(404,)),
        # the shell and the static mounts, which the front door serves first
        Case(r, "spa-root", "GET", "/", user=None),
        Case(r, "spa-client-route", "GET", f"/decks/{DECK_OWNER}/{DECK_SLUG}",
             user=None),
        Case(r, "spa-asset", "GET", f"/assets/{_first_bundle_asset()}",
             user=None),
        Case(r, "tarot-asset", "GET", "/tarot/00-fool.webp", user=None),
        Case(r, "api-unknown-anonymous", "GET", "/api/no-such-route",
             user=None, expect=(401,)),
        Case(r, "api-unknown", "GET", "/api/no-such-route", expect=(404,)),
        # the one 422 FastAPI itself writes: a body that is not JSON at all
        Case(r, "create-not-json", "POST", "/api/decks", content=b"{",
             expect=(422,)),
    )


def _claude() -> tuple[Case, ...]:
    c = "claude"
    return (
        Case(c, "research", "POST", "/api/claude/research",
             json={"question": "what is a mulligan"}, expect=(503,)),
        Case(c, "research-refused", "POST", "/api/claude/research",
             json={"question": "   "}, expect=(422,)),
        Case(c, "scan-refused", "POST", "/api/claude/scan", json={},
             expect=(422,)),
        Case(c, "theme", "POST", "/api/claude/theme",
             json={"transcript": [], "slots": []}, expect=(200, 503)),
        Case(c, "theme-refused", "POST", "/api/claude/theme",
             json={"transcript": "not a list"}, expect=(422,)),
        Case(c, "proposal-floor-unmet", "POST", "/api/claude/theme/proposal",
             json={"transcript": [], "slots": []}, expect=(409, 503)),
        Case(c, "interview-refused", "POST", f"{DECK}/interview", json={},
             expect=(422,)),
        Case(c, "interview", "POST", f"{DECK}/interview",
             json={"card": "Sol Ring"}, expect=(503,)),
        Case(c, "argue-refused", "POST", f"{DECK}/argue", json={},
             expect=(422,)),
        Case(c, "argue", "POST", f"{DECK}/argue", json={"card": "Sol Ring"},
             expect=(503,)),
        Case(c, "argue-deck-refused", "POST", f"{DECK}/argue/deck", json={},
             expect=(422,)),
        Case(c, "argue-deck", "POST", f"{DECK}/argue/deck",
             json={"cards": ["Sol Ring"]}, expect=(503,)),
        # Not a 404: a dossier nobody has asked for yet is an empty answer
        # with `cached: false`, which the page renders as the button.
        Case(c, "dossier-get", "GET", f"{DECK}/dossier"),
        Case(c, "dossier-post", "POST", f"{DECK}/dossier", json={},
             expect=(503,)),
    )


READS: tuple[Case, ...] = _reads() + _claude()

#: The records the sequence tests produce, per family, in order.
SEQUENCES: dict[str, tuple[str, ...]] = {
    "auth": (
        "login-unknown-user", "login-malformed", "login", "me", "logout",
        "me-after-logout", "reset", "reset-malformed", "claim-bad-token",
        "claim-preview-bad-token", "login-throttled",
    ),
    "jobs": (
        "sim-mana-refused", "sim-mana", "sim-mana-done",
        "sim-mana-missing-deck", "sim-mana-missing-deck-done",
        "sim-lands", "sim-lands-done", "sim-policy", "sim-policy-done",
        "sim-shelf", "sim-shelf-refused", "sim-shelf-missing",
        "sim-forge-refused", "sim-forge-unavailable",
        "jobs-list", "job-missing",
    ),
    "edits": (
        "create", "create-duplicate", "create-refused", "get",
        "add-card", "add-card-refused", "add-second-card",
        "patch-card", "patch-card-refused",
        "patch-deck", "patch-deck-refused", "notes", "shared", "shared-refused",
        "swap", "swap-refused", "entomb", "entomb-refused",
        "graveyard-return", "graveyard-return-refused", "graveyard-exile",
        "promote", "artifacts-build", "artifacts-build-forced",
        "artifacts-shelf", "artifact", "artifact-missing", "log",
        "import-dry-run", "import", "import-refused",
        "delete-refused", "delete",
    ),
    "admin": (
        "users", "invite-unsendable", "invite-refused",
        "patch-user", "patch-user-missing", "patch-last-admin",
        "reset-unsendable", "sessions-revoked",
        "delete-user-refused", "delete-user",
        "stats-system", "stats-storage", "stats-claude", "stats-activity",
        "stats-traffic", "stats-fly",
    ),
}


def expected_records() -> dict[str, set[str]]:
    """Every golden record that should exist, by family."""
    table: dict[str, set[str]] = {}
    for case in READS:
        table.setdefault(case.family, set()).add(case.name)
    for family, names in SEQUENCES.items():
        table.setdefault(family, set()).update(names)
    return table


def golden_files() -> set[Path]:
    from contract.goldens import GOLDEN_DIR
    return set(GOLDEN_DIR.glob("*.json"))
