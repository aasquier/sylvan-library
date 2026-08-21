"""The closed form and the policy search, shaped for the UI.

Two surfaces of very different weight, which is why they are shaped
differently -- and this module is where that reasoning is written down,
because CLAUDE.md's standing lesson is that **a duration measured for one
surface is a question to ask of every sibling surface**, and the answer here
came out different for the two.

*The shelf is a plain route.* `karsten.shelf` is arithmetic over a compiled
deck and was measured at 0.03-0.04s on every deck in the library on
2026-08-21. Making it a background job would add a submit, a poll and a job
row to a call that finishes before the response is serialised. Every other
Claude-and-simulation surface in this project is a job for a measured reason;
this one is not, for a measured reason.

*The policy search is a job.* It is thirty-three seeded Tier 1 runs, and at
its default sample it takes about fifty seconds -- past any sensible request
timeout and squarely in the territory `api/simruns.py` was built for. It goes
in the CPU pool, not NET: it is pure Python and GIL-bound, exactly like the
Tier 1 runs it is made of, and putting it in NET would let it starve the
socket-bound work that pool exists to protect.

Both take the same `key` treatment as their siblings: the mulligan sweep is
deduplicated in flight, because two clicks inside fifty seconds are one
question asked twice.
"""

from __future__ import annotations

from typing import Any

from mtglab.api.jobs import Plan, Progress
from mtglab.api.simruns import (
    DEFAULT_SEED,
    _compile_checked,
    _deferred_failure,
)
from mtglab.decks.model import Deck
from mtglab.decks.source import DeckSource
from mtglab.sim import cache, curve, karsten, mulligan
from mtglab.sim.tier1.engine import KeepRule, SimCard

SHELF_CAVEAT = (
    "The closed form asks whether the mana would be there, assuming the card "
    "is in your hand. It does not ask whether you drew it, and it cannot see "
    "ramp. Read it beside the simulation, not instead of it."
)

CURVE_CAVEAT = (
    "These odds assume you keep your opening seven. Mulliganing digs for "
    "lands, and against Tier 1 it is worth about six points at turn four on "
    "these decks — so read a slot count as the pessimistic end."
)

POLICY_CAVEAT = (
    "Policies are judged on spells deployed through turn 8, the same measure "
    "the land sweep uses: mulligan rate alone recommends keeping everything, "
    "and hand quality alone recommends mulliganing forever."
)


def _color_payload(req: karsten.ColorRequirement) -> dict[str, Any]:
    return {
        "color": req.color,
        "have": req.have,
        "have_lands": req.have_lands,
        "met": req.met,
        "shortfall": req.shortfall,
        "tiers": [
            {
                "pips": tier.pips,
                "turn": tier.turn,
                "need": tier.need,
                "have": tier.have,
                "met": tier.met,
                "shortfall": tier.shortfall,
                "odds_now": round(tier.odds_now, 4),
                # Capped: a single-pip rung in a mono-coloured deck names
                # thirty cards, and a tooltip is not a decklist. The count
                # rides alongside so the client can say "and 27 more".
                "cards": list(tier.cards[:6]),
                "card_count": len(tier.cards),
            }
            for tier in req.tiers
        ],
    }


def shelf_result(slug: str, payload: dict[str, Any], *,
                 source: DeckSource | None = None) -> dict[str, Any]:
    """The whole closed form for one deck, computed in the request.

    Unlike `plan_mana` there is no cache and no job. A cache would be keyed on
    the compiled deck and cost a hash plus a SELECT to save forty
    milliseconds of `math.comb`, which is a cache that loses -- and
    `caches.py` exists in this project precisely because a cache that is never
    worth its lookup is worse than none.
    """
    on_the_play = bool(payload.get("on_the_play", True))
    target = float(payload.get("target", karsten.TARGET))
    target = max(0.5, min(target, 0.99))
    target_turn = int(payload.get("target_turn", curve.DEFAULT_TARGET_TURN))
    raw_mana = payload.get("target_mana")
    target_mana = None if raw_mana in (None, "") else int(raw_mana)

    deck, report, check = _compile_checked(slug, source=source)
    computed = karsten.shelf(report.library, report.commander, target=target,
                             on_the_play=on_the_play)
    # The curve rides on the shelf rather than on a route of its own: it is
    # the same arithmetic over the same compiled deck, it costs about as much
    # as the shelf does, and a second round trip for a second closed form
    # would be two spinners where the page needs none.
    mana = curve.curve(report.library, target_turn=target_turn,
                       target_mana=target_mana, target=target,
                       on_the_play=on_the_play)
    return dict(_shelf_payload(slug, deck, computed),
                deck_check=check, mana_curve=_curve_payload(mana))


def _shelf_payload(slug: str, deck: Deck,
                   computed: karsten.Shelf) -> dict[str, Any]:
    estimate = computed.land_estimate
    return {
        "slug": slug,
        "deck_name": deck.name,
        "deck_size": computed.deck_size,
        "lands": computed.lands,
        "target": computed.target,
        "on_the_play": computed.on_the_play,
        "horizon": karsten.HORIZON,
        "colors": [_color_payload(c) for c in computed.colors],
        "lands_estimate": {
            "lands_now": estimate.lands_now,
            "recommended": estimate.recommended,
            "delta": estimate.delta,
            "average_mana_value": estimate.average_mana_value,
            "cheap_accelerants": estimate.cheap_accelerants,
            "caveats": list(estimate.caveats),
        },
        # The heatmap. Sorted by lateness in the engine -- worst first -- so a
        # client rendering the head of the list is already showing the rows
        # worth reading, exactly as `card_timings` does for Tier 1.
        "cards": [
            {
                "name": o.name,
                "mv": o.mv,
                "on_curve": None if o.on_curve is None else round(o.on_curve, 4),
                "reliable_turn": o.reliable_turn,
                "lag": o.lag,
                "by_turn": [round(x, 4) for x in o.by_turn],
            }
            for o in computed.odds
        ],
        "approximated": list(computed.approximated),
        "caveat": SHELF_CAVEAT,
    }


def _curve_payload(mc: curve.ManaCurve) -> dict[str, Any]:
    """The mana curve, shaped for a screen.

    `advice.recommend` is the server's verdict and the client must not
    re-derive it. The rule behind it -- lands up to the curve, ramp past it --
    is arithmetic the client has no business re-implementing, and a second
    copy in TypeScript would be a second chance to get its *direction* wrong,
    which is the one error nobody would spot from a screenshot.
    """
    a = mc.advice
    return {
        "deck_size": mc.deck_size,
        "lands": mc.lands,
        "accelerants": mc.accelerants,
        "target_turn": mc.target_turn,
        "target_mana": mc.target_mana,
        "target": mc.target,
        "turns": [
            {
                "turn": t.turn,
                "from_lands": round(t.from_lands, 2),
                "from_ramp": round(t.from_ramp, 2),
                "expected_mana": round(t.expected_mana, 2),
                "land_drop_odds": round(t.land_drop_odds, 4),
                "odds": round(t.odds, 4),
            }
            for t in mc.turns
        ],
        "advice": {
            "target_turn": a.target_turn,
            "target_mana": a.target_mana,
            "odds": a.odds,
            "odds_per_land": a.odds_per_land,
            "odds_per_ramp": a.odds_per_ramp,
            "recommend": a.recommend,
            "slots": a.slots,
            "ramp_is_generic": a.ramp_is_generic,
            "beyond_the_curve": a.beyond_the_curve,
            "lands_for_every_drop": a.lands_for_every_drop,
        },
        "caveat": CURVE_CAVEAT,
    }


# ------------------------------------------------------------ the policy

def _policy_params(payload: dict[str, Any]) -> tuple[int, int, int]:
    """Clamped before the key is built, for `_mana_params`' reason.

    `games` is clamped harder than Tier 1's own runs because this multiplies
    it by the size of the grid: the ceiling here is thirty-three times the
    number requested.
    """
    games = max(200, min(int(payload.get("games", 2_000)), 10_000))
    turns = max(8, min(int(payload.get("turns", 10)), 16))
    raw = payload.get("seed")
    seed = DEFAULT_SEED if raw in (None, "") else int(raw)
    return games, turns, seed


def _row_payload(row: mulligan.PolicyRow) -> dict[str, Any]:
    return {
        "min_lands": row.min_lands,
        "max_lands": row.max_lands,
        "min_pieces": row.min_pieces,
        "describe": row.describe,
        "spells_through_t8": row.spells_through_t8,
        "mulligan_rate": row.mulligan_rate,
        "avg_mulligans": row.avg_mulligans,
        "median_commander_turn": row.median_commander_turn,
        "color_screw_rate": row.color_screw_rate,
        "stalled_turns": row.stalled_turns,
    }


def _policy_result(slug: str, deck: Deck, library: list[SimCard],
                   commander: SimCard | None, games: int, turns: int,
                   seed: int, progress: Progress) -> dict[str, Any]:
    sweep = mulligan.search(
        library, commander, games=games, turns=turns, seed=seed,
        progress=lambda done, total: progress(done, total))
    return {
        "slug": slug,
        "deck_name": deck.name,
        "games": games,
        "turns": turns,
        "seed": seed,
        "rows": [_row_payload(r) for r in sweep.rows],
        "best": _row_payload(sweep.best),
        "baseline": _row_payload(sweep.baseline),
        "gentlest": _row_payload(sweep.gentlest),
        "spread": sweep.spread,
        "gain": sweep.gain,
        # The verdict, and the client renders words off it rather than
        # deciding for itself: `flat` is measured against the default, not
        # against the grid's range, and a second implementation of that rule
        # in TypeScript would be a second chance to get it wrong.
        "flat": sweep.flat,
        "caveat": POLICY_CAVEAT,
    }


def plan_policy(slug: str, payload: dict[str, Any],
                *, source: DeckSource | None = None) -> Plan:
    """Decide whether this policy search is already answered."""
    games, turns, seed = _policy_params(payload)
    label = f"{slug}: mulligan policies, {games:,} games each"

    try:
        deck, report, check = _compile_checked(slug, source=source)
        library, commander = report.library, report.commander
    except Exception as exc:                                        # noqa: BLE001
        # Same contract as `plan_mana`: planning is an optimisation, and a
        # deck that cannot compile must fail as a job in state `error` rather
        # than as an exception out of the route.
        return Plan("sim.policy", label, None, _deferred_failure(exc))

    # The grid rides in `extra` and it rides in full, not as a count. Which
    # rules were tried decides the answer, and `len(candidates())` is the
    # fingerprint that does not change when somebody swaps a 6 for a 7 in
    # `MAX_LANDS` -- the exact shape of stale-cache bug this project keys on
    # compiled input to avoid. `keep_rule` is the baseline the gain is
    # measured against, which is a real input to the verdict.
    grid = sorted((r.min_lands, r.max_lands, r.min_mana_pieces)
                  for r in mulligan.candidates())
    key = cache.key("sim.policy", library=library, commander=commander,
                    games=games, turns=turns, keep_rule=KeepRule(), seed=seed,
                    extra={"grid": grid, "through": mulligan.THROUGH,
                           "flat": mulligan.FLAT})
    hit = cache.get(key)
    if hit is not None:
        # Attached after the cache, for the reason `plan_mana` records: the
        # numbers are keyed on the compiled deck, and the verdict is not.
        answer = dict(hit.result, cached=True, computed_at=hit.created_at,
                      deck_check=check)
        return Plan("sim.policy", label, answer, lambda _progress: answer)

    def compute(progress: Progress) -> dict[str, Any]:
        result = _policy_result(slug, deck, library, commander, games, turns,
                                seed, progress)
        cache.put(key, "sim.policy", result)
        return dict(result, cached=False, computed_at=None, deck_check=check)

    return Plan("sim.policy", label, None, compute)
