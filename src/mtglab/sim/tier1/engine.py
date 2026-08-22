"""Tier 1: stochastic goldfish simulation.

Answers the questions that are genuinely answerable by sampling: how often does
a hand keep, how many lands is correct, what turn does the commander land, does
the mana base actually produce the colors the curve demands.

It does NOT model opponents, interaction, or card-specific text beyond mana
production and ramp. That is Tier 2's job. Keeping the boundary sharp is what
makes these numbers trustworthy -- every result here follows from shuffling,
drawing, and paying costs, with no hidden heuristics about "good" play.
"""

from __future__ import annotations

import random
from collections import Counter
from collections.abc import Callable, Iterable, Sequence
from dataclasses import dataclass, field
from math import fsum
from statistics import median

from mtglab.mana import ManaCost, ManaSource, can_pay, expand_units


@dataclass(frozen=True)
class SimCard:
    """A card reduced to what Tier 1 can reason about."""

    name: str
    cost: ManaCost = field(default_factory=ManaCost)
    category: str = "utility"
    is_land: bool = False
    enters_tapped: bool = False
    # Mana this permanent produces once it is on the battlefield and able.
    produces: tuple[ManaSource, ...] = ()
    # Turns of delay before `produces` is usable (0 for rocks and lands,
    # 1 for creatures with summoning sickness).
    produce_delay: int = 0
    # Land-fetch ramp (Cultivate, Rampant Growth): lands moved to battlefield.
    fetches_lands: int = 0

    @property
    def mv(self) -> int:
        return self.cost.mana_value

    @property
    def is_ramp(self) -> bool:
        return bool(self.produces) or self.fetches_lands > 0


@dataclass
class KeepRule:
    """Mulligan policy. Tuned per deck -- this is a real lever, not a default.

    `min_lands`/`max_lands` bound the land count in the opener.
    `min_mana_pieces` counts lands plus cheap ramp (mv <= cheap_ramp_mv); the
    rule most often worth tuning, since ramp-dense decks keep 2-landers that
    land-light decks cannot.
    """

    min_lands: int = 2
    max_lands: int = 5
    min_mana_pieces: int = 3
    cheap_ramp_mv: int = 2
    max_mulligans: int = 3

    def keeps(self, hand: Sequence[SimCard]) -> bool:
        lands = sum(1 for c in hand if c.is_land)
        if not (self.min_lands <= lands <= self.max_lands):
            return False
        cheap_ramp = sum(
            1 for c in hand if (not c.is_land) and c.is_ramp and c.mv <= self.cheap_ramp_mv
        )
        return lands + cheap_ramp >= self.min_mana_pieces

    def describe(self) -> str:
        return (
            f"keep {self.min_lands}-{self.max_lands} lands AND "
            f"lands + ramp(mv<={self.cheap_ramp_mv}) >= {self.min_mana_pieces}"
        )


@dataclass
class GameResult:
    commander_turn: int | None
    lands_by_turn: list[int]
    mana_by_turn: list[int]
    spells_by_turn: list[int]
    # Mana available but never spent, per turn. Without this the model has no
    # cost for flooding and will always recommend more lands.
    unused_by_turn: list[int]
    mulligans: int
    color_screwed_turns: int
    # --- the per-game texture (second 2026-08-15 punch list, item 11) ----
    # The turn each named nonland spell was first cast, when it was. Keyed by
    # name: the 99 is singleton apart from basics, and basics are lands.
    first_cast: dict[str, int]
    # True for each turn where no land was played while none was in hand --
    # a missed drop, as opposed to a chosen discard of tempo.
    missed_drop_by_turn: list[bool]
    # The first turn any nonland spell resolved, or None for a game that
    # never got moving inside the horizon.
    first_spell_turn: int | None
    # Turns that ended with zero spells cast while at least one nonland
    # spell sat in hand -- the "couldn't act" count, blocked on quantity or
    # colour alike (color_screwed_turns is the colour-only subset).
    stalled_turns: int


def _consume(cost: ManaCost, sources: list[ManaSource]) -> list[ManaSource] | None:
    """Pay `cost` from `sources`, returning what remains, or None if unpayable.

    Colored pips are assigned by exact matching. Generic is then paid from the
    leftovers, spending the least flexible sources first so that the survivors
    stay as useful as possible for the next spell this turn.
    """
    units = expand_units(sources)
    # Not `cost.mana_value` -- Phyrexian symbols are paid with life, so they
    # add to a card's mana value without ever costing mana here.
    need = cost.generic + len(cost.pips)
    if len(units) < need:
        return None

    used: set[int] = set()
    if cost.pips:
        adj = [[j for j, u in enumerate(units) if u & pip] for pip in cost.pips]
        match_unit: list[int] = [-1] * len(units)

        def assign(pip_idx: int, seen: list[bool]) -> bool:
            # `seen` must persist across the recursion within one augmenting
            # search; resetting it here would revisit units and loop forever.
            for j in adj[pip_idx]:
                if seen[j]:
                    continue
                seen[j] = True
                if match_unit[j] == -1 or assign(match_unit[j], seen):
                    match_unit[j] = pip_idx
                    return True
            return False

        for i in range(len(cost.pips)):
            if not assign(i, [False] * len(units)):
                return None
        used = {j for j, p in enumerate(match_unit) if p != -1}

    # Pay generic with the most restrictive remaining units.
    leftovers = sorted(
        (j for j in range(len(units)) if j not in used),
        key=lambda j: len(units[j]),
    )
    if len(leftovers) < cost.generic:
        return None
    used |= set(leftovers[: cost.generic])

    return [ManaSource(units[j], 1) for j in range(len(units)) if j not in used]


def _pick_land(hand: list[SimCard], pool: list[ManaSource],
               castables: list[SimCard]) -> SimCard | None:
    """Choose which land to play: the one that maximises this turn's options.

    Ranks by (mana value of the best spell it lets us cast now, new colors it
    adds, untapped). This reproduces the standard human heuristic -- untapped
    when it enables a play, otherwise fix colors.
    """
    lands = [c for c in hand if c.is_land]
    if not lands:
        return None

    have_colors: set[str] = set()
    for src in pool:
        have_colors |= set(src.colors)

    def score(land: SimCard) -> tuple[int, int, int]:
        added = [] if land.enters_tapped else list(land.produces)
        trial = pool + added
        best_mv = 0
        for spell in castables:
            if can_pay(spell.cost, trial) and spell.mv > best_mv:
                best_mv = spell.mv
        new_colors = len({c for s in land.produces for c in s.colors} - have_colors)
        return (best_mv, new_colors, 0 if land.enters_tapped else 1)

    return max(lands, key=score)


def simulate_game(
    library: Sequence[SimCard],
    commander: SimCard | None,
    *,
    turns: int = 12,
    keep_rule: KeepRule | None = None,
    rng: random.Random | None = None,
    on_the_play: bool = False,
) -> GameResult:
    rng = rng or random.Random()
    keep_rule = keep_rule or KeepRule()

    # --- London mulligan -------------------------------------------------
    deck = list(library)
    mulligans = 0
    while True:
        rng.shuffle(deck)
        hand = deck[:7]
        if keep_rule.keeps(hand) or mulligans >= keep_rule.max_mulligans:
            break
        mulligans += 1
    library_left = deck[7:]
    hand = list(hand)
    # Bottom `mulligans` cards -- put back the least useful (excess lands first,
    # then the highest-cost spells).
    for _ in range(min(mulligans, len(hand) - 1)):
        lands_in_hand = [c for c in hand if c.is_land]
        if len(lands_in_hand) > keep_rule.min_lands:
            hand.remove(lands_in_hand[-1])
        else:
            hand.remove(max(hand, key=lambda c: c.mv))

    # --- game state ------------------------------------------------------
    battlefield: list[tuple[SimCard, int]] = []  # (card, turn it entered)
    commander_turn: int | None = None
    lands_by_turn: list[int] = []
    mana_by_turn: list[int] = []
    spells_by_turn: list[int] = []
    unused_by_turn: list[int] = []
    color_screwed = 0
    first_cast: dict[str, int] = {}
    missed_drop_by_turn: list[bool] = []
    first_spell_turn: int | None = None
    stalled_turns = 0

    for turn in range(1, turns + 1):
        if not (turn == 1 and on_the_play):
            if library_left:
                hand.append(library_left.pop(0))

        # Available mana this turn: every permanent past its delay.
        pool: list[ManaSource] = []
        for card, entered in battlefield:
            if turn - entered >= card.produce_delay:
                pool.extend(card.produces)

        spells_in_hand = [c for c in hand if not c.is_land]
        land = _pick_land(hand, pool, spells_in_hand)
        if land is not None:
            hand.remove(land)
            battlefield.append((land, turn))
            if not land.enters_tapped:
                pool.extend(land.produces)
        # A missed drop is having none to play, not choosing not to --
        # `_pick_land` always plays one when the hand holds one.
        missed_drop_by_turn.append(
            land is None and not any(c.is_land for c in hand))

        # --- casting ------------------------------------------------------
        cast_count = 0
        while True:
            options = [c for c in hand if not c.is_land and _consume(c.cost, pool) is not None]
            if commander is not None and commander_turn is None:
                if _consume(commander.cost, pool) is not None:
                    options.append(commander)

            if not options:
                # Was anything blocked purely on color rather than quantity?
                total = len(expand_units(pool))
                if any(c.mv <= total for c in hand if not c.is_land):
                    color_screwed += 1
                break

            # Priority: cheap ramp first (it compounds), then the commander,
            # then the biggest thing we can deploy.
            def priority(c: SimCard) -> tuple[int, int]:
                if c.is_ramp and commander is not None and c.mv < commander.mv:
                    return (0, -c.mv)
                if c is commander:
                    return (1, 0)
                return (2, -c.mv)

            chosen = min(options, key=priority)
            remaining = _consume(chosen.cost, pool)
            assert remaining is not None
            pool = remaining

            if first_spell_turn is None:
                first_spell_turn = turn
            first_cast.setdefault(chosen.name, turn)
            if chosen is commander:
                commander_turn = turn
                battlefield.append((chosen, turn))
            else:
                hand.remove(chosen)
                if chosen.produces:
                    battlefield.append((chosen, turn))
                    if chosen.produce_delay == 0:
                        pool.extend(chosen.produces)
                elif chosen.fetches_lands:
                    for _ in range(chosen.fetches_lands):
                        found = next((c for c in library_left if c.is_land), None)
                        if found is None:
                            break
                        library_left.remove(found)
                        battlefield.append((found, turn))
            cast_count += 1

        lands_by_turn.append(sum(1 for c, _ in battlefield if c.is_land))
        mana_by_turn.append(
            len(expand_units([s for c, e in battlefield
                              if turn - e >= c.produce_delay for s in c.produces]))
        )
        spells_by_turn.append(cast_count)
        unused_by_turn.append(len(expand_units(pool)))
        if cast_count == 0 and any(not c.is_land for c in hand):
            stalled_turns += 1

    return GameResult(
        commander_turn=commander_turn,
        lands_by_turn=lands_by_turn,
        mana_by_turn=mana_by_turn,
        spells_by_turn=spells_by_turn,
        unused_by_turn=unused_by_turn,
        mulligans=mulligans,
        color_screwed_turns=color_screwed,
        first_cast=first_cast,
        missed_drop_by_turn=missed_drop_by_turn,
        first_spell_turn=first_spell_turn,
        stalled_turns=stalled_turns,
    )


@dataclass(frozen=True)
class CardTiming:
    """When one card actually comes online, over many games.

    The gap between `mv` and `median_turn` is the fun number: a four-drop
    with a median first cast of seven is telling you something about the
    mana base (or about how much else competes for the same turn) that no
    curve histogram shows.
    """

    name: str
    mv: int
    #: Fraction of games in which it was cast at all inside the horizon.
    cast_rate: float
    #: Median turn of the first cast, over the games that cast it.
    median_turn: float | None
    #: Fraction of games in which it was down by end of turn 8.
    by_t8: float


@dataclass
class SimSummary:
    games: int
    turns: int
    keep_rule: str
    mulligan_rate: float
    avg_mulligans: float
    commander_by_turn: dict[int, float]
    median_commander_turn: float | None
    never_cast_commander: float
    avg_lands_by_turn: list[float]
    avg_mana_by_turn: list[float]
    avg_unused_by_turn: list[float]
    avg_spells_by_turn: list[float]
    color_screw_rate: float
    # --- the texture (second 2026-08-15 punch list, item 11) -------------
    #: Per nonland spell, when it comes online. Sorted latest median first,
    #: because the interesting rows are the ones running late.
    card_timings: list[CardTiming]
    #: P(no land to play on turn t), cumulative misses being the reader's own
    #: sum -- the per-turn number is the honest unit.
    missed_drop_by_turn: list[float]
    #: Median turn of the first nonland spell, over games that cast one.
    median_first_spell_turn: float | None
    #: Turns per game that ended castless with a spell in hand.
    avg_stalled_turns: float

    def spells_through(self, turn: int) -> float:
        """Total spells deployed by end of `turn` -- the flood-aware measure
        of whether extra lands are actually buying anything.

        `fsum`, not `sum`, for the reason `sim/curve.py`'s module docstring
        sets out at length: **CPython 3.12 gave `sum()` compensated
        (Neumaier) accumulation and 3.11 adds left to right**, so a float sum
        answers differently on the two interpreters this project supports.
        Measured: `sum([0.1] * 10)` is 1.0 under 3.12.13 and
        0.9999999999999999 under 3.11.15.

        The exposure here is smaller than the curve's -- every caller wraps
        this in `round(x, 2)`, so the difference is a last-ulp one that no
        screen shows -- but `sim/mulligan.py` *ranks* policies on
        `spells_through_t8` and reports a spread against `FLAT`, and a
        ranking that depends on the interpreter is the same bug however
        rarely it fires. Fixing beats pinning either answer, which is the
        call the closed forms made first and this follows.

        One visible consequence, and it is an improvement: `sum([])` is the
        **int** 0 and `fsum([])` is `0.0`, so a zero-turn total now matches
        the annotation.
        """
        return fsum(self.avg_spells_by_turn[:turn])

    def wasted_through(self, turn: int) -> float:
        return fsum(self.avg_unused_by_turn[:turn])

    def report(self) -> str:
        lines = [
            f"games={self.games}  turns={self.turns}",
            f"mulligan policy: {self.keep_rule}",
            f"mulligan rate: {self.mulligan_rate:.1%}  (avg {self.avg_mulligans:.2f})",
            f"median commander turn: {self.median_commander_turn}",
            f"commander never cast by T{self.turns}: {self.never_cast_commander:.1%}",
            f"turns with a color-only block: {self.color_screw_rate:.2f} per game",
            f"median first spell: T{self.median_first_spell_turn}",
            (f"stalled turns (castless with a spell in hand): "
             f"{self.avg_stalled_turns:.2f} per game"),
            "",
            "  turn   lands   mana   unused   spells   P(commander down)",
        ]
        for t in range(1, self.turns + 1):
            lines.append(
                f"  {t:>4}   {self.avg_lands_by_turn[t-1]:>5.2f}  "
                f"{self.avg_mana_by_turn[t-1]:>5.2f}  "
                f"{self.avg_unused_by_turn[t-1]:>6.2f}  "
                f"{self.avg_spells_by_turn[t-1]:>6.2f}   "
                f"{self.commander_by_turn.get(t, 0.0):>6.1%}"
            )
        return "\n".join(lines)


def run(
    library: Sequence[SimCard],
    commander: SimCard | None,
    *,
    games: int = 20_000,
    turns: int = 12,
    keep_rule: KeepRule | None = None,
    seed: int | None = None,
    progress: Callable[[int, int], None] | None = None,
) -> SimSummary:
    """Run `games` independent goldfish games and summarise.

    `progress(done, total)` is called roughly 100 times over the run, so a UI
    can show a bar. A 20,000-game sweep takes ~30s, which is far too long to
    leave a caller with no signal.
    """
    rng = random.Random(seed)
    keep_rule = keep_rule or KeepRule()
    # Report ~1% at a time: frequent enough to animate, rare enough that the
    # callback never shows up in a profile.
    report_every = max(1, games // 100)

    commander_turns: list[int] = []
    never = 0
    mull_total = 0
    mull_any = 0
    lands_acc = [0.0] * turns
    mana_acc = [0.0] * turns
    unused_acc = [0.0] * turns
    spells_acc = [0.0] * turns
    screw_acc = 0
    cast_turns: dict[str, list[int]] = {}
    missed_acc = [0] * turns
    first_spells: list[int] = []
    stalled_total = 0

    for game_index in range(games):
        if progress is not None and game_index % report_every == 0:
            progress(game_index, games)
        res = simulate_game(library, commander, turns=turns,
                            keep_rule=keep_rule, rng=rng)
        if res.commander_turn is None:
            never += 1
        else:
            commander_turns.append(res.commander_turn)
        mull_total += res.mulligans
        mull_any += 1 if res.mulligans else 0
        screw_acc += res.color_screwed_turns
        stalled_total += res.stalled_turns
        if res.first_spell_turn is not None:
            first_spells.append(res.first_spell_turn)
        for name, cast_on in res.first_cast.items():
            cast_turns.setdefault(name, []).append(cast_on)
        for i in range(turns):
            lands_acc[i] += res.lands_by_turn[i]
            mana_acc[i] += res.mana_by_turn[i]
            unused_acc[i] += res.unused_by_turn[i]
            spells_acc[i] += res.spells_by_turn[i]
            if res.missed_drop_by_turn[i]:
                missed_acc[i] += 1

    if progress is not None:
        progress(games, games)

    cum = Counter(commander_turns)
    by_turn: dict[int, float] = {}
    running = 0
    for t in range(1, turns + 1):
        running += cum.get(t, 0)
        by_turn[t] = running / games

    # Every distinct nonland spell in the deck, whether or not any game cast
    # it -- a card the simulation *never* reached is the most interesting row
    # of all, and dropping it would hide exactly that. Duplicates collapse by
    # name, which in a singleton format is only the handful of spells a deck
    # runs twice by special dispensation.
    mv_by_name: dict[str, int] = {}
    for c in library:
        if not c.is_land:
            mv_by_name.setdefault(c.name, c.mv)
    if commander is not None:
        mv_by_name.setdefault(commander.name, commander.mv)
    timings = []
    for name, mv in mv_by_name.items():
        casts = cast_turns.get(name, [])
        timings.append(CardTiming(
            name=name,
            mv=mv,
            cast_rate=len(casts) / games,
            median_turn=float(median(casts)) if casts else None,
            by_t8=sum(1 for t in casts if t <= 8) / games,
        ))
    # Latest median first; the never-cast rows (median None) lead outright.
    timings.sort(key=lambda t: (t.median_turn is not None,
                                -(t.median_turn or 0), t.name))

    return SimSummary(
        games=games,
        turns=turns,
        keep_rule=keep_rule.describe(),
        mulligan_rate=mull_any / games,
        avg_mulligans=mull_total / games,
        commander_by_turn=by_turn,
        median_commander_turn=median(commander_turns) if commander_turns else None,
        never_cast_commander=never / games,
        avg_lands_by_turn=[x / games for x in lands_acc],
        avg_mana_by_turn=[x / games for x in mana_acc],
        avg_unused_by_turn=[x / games for x in unused_acc],
        avg_spells_by_turn=[x / games for x in spells_acc],
        color_screw_rate=screw_acc / games,
        card_timings=timings,
        missed_drop_by_turn=[x / games for x in missed_acc],
        median_first_spell_turn=float(median(first_spells)) if first_spells else None,
        avg_stalled_turns=stalled_total / games,
    )


def sweep_land_counts(
    build: Callable[[int], tuple[list[SimCard], SimCard | None]],
    counts: Iterable[int],
    *,
    games: int = 5_000,
    turns: int = 12,
    keep_rule: KeepRule | None = None,
    seed: int | None = 7,
) -> list[tuple[int, SimSummary]]:
    """Find the knee of the curve: run the same deck at several land counts.

    `build(n)` must return (library, commander) for a deck with n lands.
    """
    out = []
    for n in counts:
        library, commander = build(n)
        out.append((n, run(library, commander, games=games, turns=turns,
                           keep_rule=keep_rule, seed=seed)))
    return out
