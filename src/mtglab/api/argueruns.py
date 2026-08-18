"""The slot argument (ADR 25), swept across a selection of slots, as one job.

The single argue endpoint is synchronous on a measured argument -- it shares
the interview's ~4,900-token brief and sits in the seconds class. This module
exists because a *sweep* multiplies that by the selection: one Claude call per
card, so even at seconds each, a few dozen slots is minutes and a full 99 is
tens of minutes. That is `themeruns`/`dossierruns` territory, and this is the
fourth module of their shape.

**Checking happens in the request; calling happens in the job.** Everything
refusable is refused before anything is queued, each with its own status code:
an empty selection or a card the deck does not hold (422, and the missing
cards are named), a malformed stance (422), no key or SDK (503), no such deck
(404). Carried into a worker they would all arrive as a job in state `error`
-- one string for four cases and a status code for none.

**One job for the whole sweep, sequential inside it.** The `NET` lane is two
workers wide and shared with the theme proposal and the dossier; a sweep
submitted as N jobs would occupy the whole lane for its duration and starve
every sibling surface. One job argues one card at a time and reports
`progress(done, total)`, which is also what makes the progress bar honest.

**A failed card is recorded and the sweep continues.** Partial results are
the point of paying for a sweep; a single flaky call must not cost the other
forty answers. The one exception is the credential vanishing mid-run
(`ClaudeUnavailable`) -- every remaining card would fail the same way, so the
rest are marked unattempted and the sweep stops rather than burning the
selection on a dead key.

**A stance of `off` is a job born finished** -- the sweep's answer is already
known and is one report, not N copies of "no call was made".

**Deduplicated in flight on the selection itself.** `key` is the slug plus a
fingerprint of the selected names, so a double-click joins the run already in
flight (per owner, per ADR 5), while a genuinely different selection runs as
its own job. Nothing is cached across runs: like the theme proposal, two runs
over the same slots may legitimately differ, and the client keeps the job id
for reattaching instead.
"""

from __future__ import annotations

import hashlib
from typing import Any

from mtglab.api.jobs import NET, Plan, Progress
from mtglab.api.service import ClaudeFailed
from mtglab.decks.source import DeckSource

#: What `/api/jobs` calls one of these. Namespaced like `claude.dossier` so a
#: job list reads as what produced it.
KIND = "claude.argue.deck"


def plan_review(*, slug: str, cards: Any = None, requested: Any = None,
                source: DeckSource | None = None,
                tier: str | None = None) -> Plan:
    """Refuse now if it is refusable; otherwise hand back the sweep.

    Raises `ValueError` (bad selection or bad stance -- `CardNotInDeck` is a
    `ValueError` and names the cards), `DeckNotFound` and `ClaudeUnavailable`
    to the caller, which is what keeps their 422 / 404 / 503 rather than
    flattening them into a job error.
    """
    from mtglab.api import service
    from mtglab.claude import client as claude_client
    from mtglab.claude import stance as stance_mod
    from mtglab.claude.argue import CardNotInDeck

    if not isinstance(cards, list) or not all(isinstance(c, str) for c in cards):
        raise ValueError("cards must be a list of card names")
    names = [c.strip() for c in cards if c and c.strip()]
    if not names:
        raise ValueError("no cards selected")

    deck = service.get_deck(slug, source=source)
    in_deck = {c["name"].casefold(): c["name"] for c in deck["cards"]}
    missing = [n for n in names if n.casefold() not in in_deck]
    if missing:
        # Named, not counted: "which card" is the actionable part, and the
        # deck page sent these so a mismatch means its list is stale.
        raise CardNotInDeck(f"not in this deck: {', '.join(missing)}")

    # The pool's spelling, de-duplicated with order kept -- the order is the
    # user's selection order, and it is the order the reports come back in.
    seen: set[str] = set()
    ordered: list[str] = []
    for n in names:
        proper = in_deck[n.casefold()]
        if proper.casefold() not in seen:
            seen.add(proper.casefold())
            ordered.append(proper)

    # Resolved here only to answer "would any call be made at all"; each
    # per-card report still carries the stance `argue.ask` resolved for it,
    # from the same inputs.
    deck_obj = type("D", (), {"status": deck["status"]})()
    effective = stance_mod.resolve(requested, deck=deck_obj)

    label = (f"argue: {len(ordered)} slot{'' if len(ordered) == 1 else 's'} "
             f"of {slug}")

    if not effective.allows_calls:
        answer: dict[str, Any] = {
            "slug": slug, "asked": False,
            "reason": ("The stance is off, so no calls were made. "
                       "Everything else about this deck still works."),
            "total": len(ordered), "reports": [], "errors": {},
        }
        return Plan(KIND, label, answer, lambda _progress: answer, lane=NET)

    # Raised here rather than minutes into a sweep that was never going to
    # work, which preserves the 503 the UI already handles.
    claude_client.require()

    def run(progress: Progress) -> dict[str, Any]:
        reports: list[dict[str, Any]] = []
        errors: dict[str, str] = {}
        total = len(ordered)
        progress(0, total)
        for done, name in enumerate(ordered, start=1):
            try:
                reports.append(service.claude_argue(
                    slug=slug, card=name, requested=requested, source=source,
                    tier=tier))
            except claude_client.ClaudeUnavailable as exc:
                # The key vanished mid-sweep. Every remaining card fails the
                # same way, so say so once each and stop spending time on it.
                errors[name] = claude_client.explain(exc)
                for rest in ordered[done:]:
                    errors[rest] = "not attempted: the credential went away"
                progress(total, total)
                break
            except ClaudeFailed as exc:
                # One bad call must not cost the rest of the sweep.
                errors[name] = str(exc)
            except Exception as exc:  # noqa: BLE001 -- same treatment as its siblings
                errors[name] = claude_client.explain(exc)
            progress(done, total)
        return {"slug": slug, "asked": True, "reason": "",
                "total": total, "reports": reports, "errors": errors}

    # The slug plus the selection: a double-click joins the sweep in flight,
    # a different selection is different work. Sorted and casefolded so the
    # same slots picked in a different order are still the same sweep.
    fingerprint = hashlib.sha256(
        "\x00".join(sorted(c.casefold() for c in ordered)).encode()
    ).hexdigest()[:16]
    return Plan(KIND, label, None, run, lane=NET, key=f"{slug}:{fingerprint}")
