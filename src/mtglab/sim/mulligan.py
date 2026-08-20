"""Searching for a deck's mulligan policy, instead of assuming one.

`KeepRule`'s docstring has said "tuned per deck -- this is a real lever, not a
default" since it was written, and then every caller in the project used the
default anyway. This module is that sentence made executable: run the same deck
under a grid of keep rules and report which one deploys the most.

**The objective is spells deployed through turn 8**, and it is the same
objective the land sweep uses, for the same reason. Judging a mulligan policy
by mulligan rate recommends keeping everything; judging it by opening-hand
quality recommends mulliganing forever. Deployment prices both -- a hand thrown
away costs a card off the top, and a hand kept on two lands costs the turns it
spends stuck -- so the policy that deploys most is the policy that was right.

Two honesty rules carried over from `api/simruns.py`, both bought there:

*A flat sweep is not a peak.* Sixty-four rules whose deployment differs by a
tenth of a spell have no winner, and reporting the arithmetic maximum of that
is reading noise aloud. `PolicySweep.flat` says so, and the surface says it in
words.

*Every rule runs on the same seed.* The comparison is between policies, not
between shuffles, so the shuffles must be held still. A grid where each cell
drew its own sample would rank the rules by luck at these sample sizes.
"""

from __future__ import annotations

from collections.abc import Callable, Iterable
from dataclasses import dataclass

from mtglab.sim.tier1.engine import KeepRule, SimCard, run

#: The turn deployment is counted through. Eight, matching the land sweep and
#: `stat.spells_through_t8` -- one horizon across the whole simulator, because
#: two decks compared at different horizons are not compared at all.
THROUGH = 8

#: The grid. Small on purpose: these are the three fields the app's own
#: mulligan controls expose, so every rule the search can recommend is a rule
#: somebody can then dial in by hand and re-run. A search whose answer had no
#: control would be a curiosity.
MIN_LANDS = (1, 2, 3)
MAX_LANDS = (4, 5, 6)
MIN_PIECES = (2, 3, 4, 5)

#: Below this spread, in spells through T8, the grid is reported as flat and
#: the recommendation is withheld. A quarter of a spell is the same bar
#: `_land_summary` draws, and it is drawn from the same place: two runs of the
#: same rule at different seeds move by about this much.
FLAT = 0.25


@dataclass(frozen=True)
class PolicyRow:
    """One keep rule, and what it did.

    `spells_through_t8` is the objective; everything else is here so a person
    can see *why* a rule won rather than being told that it did. A rule that
    wins by mulliganing less will show it in `mulligan_rate`, and one that wins
    by finding better hands will show it in `median_commander_turn`.
    """

    min_lands: int
    max_lands: int
    min_pieces: int
    spells_through_t8: float
    mulligan_rate: float
    avg_mulligans: float
    median_commander_turn: float | None
    color_screw_rate: float
    stalled_turns: float
    describe: str


@dataclass(frozen=True)
class PolicySweep:
    games: int
    turns: int
    seed: int
    rows: tuple[PolicyRow, ...]
    best: PolicyRow
    #: The rule the simulator uses when nobody chooses one, so the answer can
    #: be read as a change from where the deck stands rather than in a vacuum.
    baseline: PolicyRow
    #: Best minus worst across the whole grid. Context, not the verdict --
    #: see `flat`.
    spread: float

    @property
    def gain(self) -> float:
        """Spells the best rule deploys over the default. The whole verdict."""
        return round(self.best.spells_through_t8
                     - self.baseline.spells_through_t8, 2)

    @property
    def flat(self) -> bool:
        """Is the winner meaningfully better than the rule you already have?

        Measured against the **default**, not against the grid's range, and
        that distinction is the difference between an honest answer and a
        confident one. The land sweep can read flatness off its spread because
        every count in its range is a real candidate. This grid is not like
        that: it deliberately contains rules nobody would play -- "keep only
        hands with five mana pieces" mulligans 95% of the time -- so its spread
        is always wide, and a `flat` keyed on it would never fire.

        Goreclaw is the case that found this. Its grid spans 1.62 spells, so
        by spread the sweep looks decisive; but its best rule beats its default
        by 0.01, which is noise, and the true answer is "the default is already
        right for this deck". Reporting a 0.01 improvement as a recommendation
        would send somebody off to change a setting for nothing.
        """
        return self.gain < FLAT

    @property
    def gentlest(self) -> PolicyRow:
        """Of the rules that deploy as well as the best, the one that keeps most.

        The answer for a deck whose sweep is flat, and for most decks that is
        most decks -- four of the six here. "Your mulligan rule is already
        right" is true and useless; "these two rules deploy the same and this
        one throws away half as many hands" is the same fact with something to
        do about it. Arahbo is the case: 8.49 spells either way, at a 21%
        mulligan rate instead of 40%.

        Deployment is the objective and this never overrides it -- only rules
        within `FLAT` of the winner are eligible, which is exactly the band the
        objective cannot tell apart anyway.
        """
        best = self.best.spells_through_t8
        tied = [r for r in self.rows if best - r.spells_through_t8 < FLAT]
        return min(tied, key=lambda r: (r.mulligan_rate, -r.spells_through_t8))


def candidates() -> list[KeepRule]:
    """Every rule in the grid worth running.

    A rule whose minimum exceeds its maximum keeps nothing and would mulligan
    to `max_mulligans` every game -- not a policy, an error -- so those cells
    are dropped rather than run and reported as terrible.
    """
    return [
        KeepRule(min_lands=lo, max_lands=hi, min_mana_pieces=pieces)
        for lo in MIN_LANDS
        for hi in MAX_LANDS
        for pieces in MIN_PIECES
        if lo <= hi and pieces >= lo
    ]


def _row(rule: KeepRule, library: list[SimCard], commander: SimCard | None, *,
         games: int, turns: int, seed: int) -> PolicyRow:
    summary = run(library, commander, games=games, turns=turns,
                  keep_rule=rule, seed=seed)
    return PolicyRow(
        min_lands=rule.min_lands,
        max_lands=rule.max_lands,
        min_pieces=rule.min_mana_pieces,
        spells_through_t8=round(summary.spells_through(THROUGH), 2),
        mulligan_rate=round(summary.mulligan_rate, 4),
        avg_mulligans=round(summary.avg_mulligans, 3),
        median_commander_turn=summary.median_commander_turn,
        color_screw_rate=round(summary.color_screw_rate, 3),
        stalled_turns=round(summary.avg_stalled_turns, 3),
        describe=rule.describe(),
    )


def search(library: list[SimCard], commander: SimCard | None, *,
           games: int = 2_000, turns: int = 10, seed: int = 7,
           rules: Iterable[KeepRule] | None = None,
           progress: Callable[[int, int], None] | None = None) -> PolicySweep:
    """Run the grid and report the best rule, or that there is no best rule.

    `games` defaults low against Tier 1's usual twenty thousand, and that is a
    real trade rather than an oversight: this runs thirty-odd simulations where
    the mana screen runs one. Two thousand games puts the sampling error on
    deployment at well under `FLAT`, which is the resolution the answer is
    reported at anyway -- a finer sample would buy precision the verdict throws
    away.
    """
    grid = list(rules) if rules is not None else candidates()
    if not grid:
        raise ValueError("no keep rules to search")

    rows: list[PolicyRow] = []
    for index, rule in enumerate(grid):
        if progress is not None:
            progress(index, len(grid))
        rows.append(_row(rule, library, commander, games=games, turns=turns,
                         seed=seed))
    if progress is not None:
        progress(len(grid), len(grid))

    baseline_rule = KeepRule()
    baseline = next(
        (r for r in rows
         if (r.min_lands, r.max_lands, r.min_pieces)
         == (baseline_rule.min_lands, baseline_rule.max_lands,
             baseline_rule.min_mana_pieces)),
        None,
    )
    if baseline is None:
        baseline = _row(baseline_rule, library, commander, games=games,
                        turns=turns, seed=seed)

    spread = (max(r.spells_through_t8 for r in rows)
              - min(r.spells_through_t8 for r in rows))
    # Ties broken toward the rule that mulligans less. Two policies that
    # deploy identically are not equally good to play: the one that keeps more
    # hands is the one that spends less of the game having already lost cards.
    best = max(rows, key=lambda r: (r.spells_through_t8, -r.mulligan_rate))

    # Sorted best-first, which is the order the table renders in and the order
    # the CLI prints. The rows are a ranking, not a curve, so unlike the land
    # sweep there is no reading order to preserve.
    rows.sort(key=lambda r: (-r.spells_through_t8, r.mulligan_rate))

    return PolicySweep(
        games=games,
        turns=turns,
        seed=seed,
        rows=tuple(rows),
        best=best,
        baseline=baseline,
        spread=round(spread, 2),
    )
