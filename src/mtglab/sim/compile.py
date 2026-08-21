"""Turn a `deck.yaml` plus pool records into SimCards for Tier 1.

This lived in `cli.py`, which meant the HTTP layer imported the command-line
layer to run a simulation (`from mtglab.cli import _sim_cards`). The compiler
belongs with the simulator it feeds.

It stays dependency-light in the sense CLAUDE.md means: `cards` arrives as a
plain mapping of name -> record, so nothing here imports DuckDB. The pool
boundary stays inside `cards/db.py`.

Three of the four "confidently wrong for every deck" bugs in ROADMAP were in
this function, so the comments explaining them are load-bearing.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

from mtglab.decks.model import Deck

if TYPE_CHECKING:
    # Import-time only: `engine` is imported inside the function so
    # that Tier 1 stays loadable without this module, and vice versa.
    from mtglab.sim.tier1.engine import SimCard


class NothingToSimulate(RuntimeError):
    """Raised when a deck compiles to no cards at all.

    Not the same failure as `PoolRequired`, which is "this machine has no card
    pool". This is "there is nothing here to measure", and it is the one deck
    state where a simulation must be refused rather than reported.

    It exists because an empty deck simulated perfectly happily and answered
    with confident nonsense: a 100% mulligan rate, zero spells through turn 8,
    and a shelf demanding coloured sources for a commander's two colours
    against a library of nought. Every one of those numbers is arithmetically
    correct and none of them is about anything. `adrix-and-nev-twincasters`
    was in exactly that state on the deployed instance.
    """


class PoolRequired(RuntimeError):
    """Raised when a simulation is asked for without the card pool.

    A plain exception rather than `sys.exit`, which is what this used to do --
    fine for the CLI, but it was also reachable from an API worker thread,
    where raising SystemExit would have been a confusing way to fail.
    """


def enters_tapped(oracle_text: str) -> bool:
    """Whether a land unconditionally enters tapped.

    Scryfall retemplated this: current oracle text reads "This land enters
    tapped", not "enters the battlefield tapped". Matching only the old
    wording silently treated every modern tapland as untapped, which
    overstates early mana for every deck.

    Conditional lands are deliberately treated as untapped. Tier 1 cannot
    evaluate "unless you control a Forest" or a shock land's "you may pay 2
    life", and in practice those resolve untapped in most real games; calling
    them tapped would systematically slow every deck instead.
    """
    text = (oracle_text or "").lower()
    if "enters tapped" not in text and "enters the battlefield tapped" not in text:
        return False
    return not ("unless" in text or "you may pay" in text)


#: A mana symbol inside an activation cost or an "Add" clause. `{T}` and `{Q}`
#: are not mana and must never be charged as though they were -- a Signet's
#: `{1}, {T}:` costs one mana, not two.
_SYMBOL = re.compile(r"\{([^}]+)\}")

#: How many mana a written-out amount means. Magic spells these out whenever
#: the colour is chosen rather than fixed ("Add three mana of any one color"),
#: so a symbol count alone reads every one of them as zero.
_AMOUNT_WORDS = {"one": 1, "two": 2, "three": 3, "four": 4, "five": 5}


def _mana_symbols(text: str) -> int:
    """Mana in a run of symbols. `{2}` is two, `{G}` is one, `{T}` is none."""
    total = 0
    for raw in _SYMBOL.findall(text):
        sym = raw.upper().strip()
        if sym.isdigit():
            total += int(sym)
        elif any(ch in "WUBRGC" for ch in sym.split("/")):
            total += 1
    return total


def mana_produced(oracle_text: str) -> int:
    """How many mana one activation of this permanent actually nets.

    Scryfall's `produced_mana` says *which colours* a card can make and never
    **how many**, so every mana source in this project compiled to exactly one
    mana until 2026-08-21 -- which meant **Sol Ring produced one mana**, and
    Mana Vault one, and Gilded Lotus one. Tier 1 and the closed form both
    understated every deck's acceleration, and a deck built on fast mana
    understated it most.

    The amount is read off the oracle text, conservatively: anything this
    cannot parse stays at one. Five things it has to get right, each of which
    is a real card:

    - **an alternative is not a sum.** Talisman of Progress reads "Add {W} or
      {U}" and makes one mana, not two.
    - **alternatives are separated by commas as well as by "or".** A triome
      reads "Add {R}, {G}, or {W}" and Wooded Bastion "Add {G}{G}, {G}{W}, or
      {W}{W}"; splitting on "or" alone reads those as two and four. A run is
      never comma-separated -- it is "{C}{C}", never "{C}, {C}" -- so the
      comma is unambiguous.
    - **the cost comes off the same ability only.** Arcane Signet's "{1}, {T}:
      Add {W}{U}" nets one; Grim Monolith's "{4}: Untap this artifact" sits on
      its own line and must not be charged against its "Add {C}{C}{C}".
    - **amounts are often words.** Gilded Lotus adds "three mana of any one
      color" and contains no mana symbol at all. The first number word after
      "add" is the amount -- "any *one* color" comes later in that same
      sentence and is not it.
    - **"add" is not always mana.** A card that adds a +1/+1 counter says so,
      which is why the clause must mention mana or carry a mana symbol.

    Nykthos is the shape that correctly falls through to one: "Add an amount
    of mana of that color equal to your devotion" is not a number this can
    know, and guessing would be worse than the floor.

    **Non-mana costs are not priced**, which is this module's existing
    position rather than a new one: `enters_tapped` already reads "you may pay
    2 life" as untapped. So Ashnod's Altar counts as two and Phyrexian Tower's
    sacrifice ability as two, because those abilities really do add that much
    -- what Tier 1 cannot model is the supply of creatures to feed them. Read
    a sacrifice-driven deck's acceleration as an upper bound.
    """
    best = 0
    for line in (oracle_text or "").lower().split("\n"):
        if "add" not in line:
            continue
        head, _, tail = line.partition(":")
        # An ability with no colon is a static or triggered one; its whole
        # text is the clause and nothing in it is an activation cost.
        cost_text, body = (head, tail) if tail else ("", line)
        start = body.find("add")
        if start < 0:
            continue
        clause = body[start:]
        stop = clause.find(".")
        if stop > 0:
            clause = clause[:stop]
        if "mana" not in clause and not _SYMBOL.search(clause):
            continue

        # Alternatives are maxed; a run is summed. Magic separates
        # alternatives with commas *and* "or" -- "Add {R}, {G}, or {W}" is
        # three choices of one, and Wooded Bastion's "Add {G}{G}, {G}{W}, or
        # {W}{W}" is three choices of two. Splitting on " or " alone reads
        # those as 2 and 4. A run never carries a comma ({C}{C}, never
        # "{C}, {C}"), so the comma is unambiguous here.
        added = max(_mana_symbols(part) for part in re.split(r",| or ", clause))
        if added == 0:
            after = clause[len("add"):].split()
            added = _AMOUNT_WORDS.get(after[0], 0) if after else 0
        if added == 0:
            continue
        best = max(best, added - _mana_symbols(cost_text))
    # One is the floor, not a default to fall back on lightly: a source that
    # nets nothing is still a source for *colour*, and Tier 1 has always
    # treated it as one mana.
    return max(1, best)


def fetches_lands(oracle_text: str) -> int:
    """How many lands a spell puts onto the battlefield from the library.

    Nature's Lore, Three Visits, Skyshroud Claim and Sakura-Tribe Elder are
    ramp that produces no mana of its own. Without this they compile to blank
    cards, which understates the deck's acceleration and skews the land-count
    recommendation.
    """
    text = (oracle_text or "").lower()
    if "search your library" not in text or "onto the battlefield" not in text:
        return 0
    if not any(w in text for w in ("land", "forest", "swamp", "island",
                                   "mountain", "plains")):
        return 0
    return 2 if ("two" in text or "up to two" in text) else 1


@dataclass(frozen=True)
class CompileReport:
    """What compiling produced, and what it could not.

    `unresolved` is the field this exists for. `compile_one` returns `None`
    for a name the pool does not know and the caller used to drop it on the
    floor, so a 99-card deck whose pool was missing six cards **simulated as a
    93-card deck with no signal at all** -- and for the closed form that is not
    a rounding error, it is the population every hypergeometric is computed
    over. The commander went the same way: unresolved, silently `None`, and
    every "never cast the commander" figure quietly about nothing.
    """

    library: list[SimCard]
    commander: SimCard | None
    #: Names in the deck that the pool did not know, in deck order.
    unresolved: tuple[str, ...]
    #: What the deck file says it holds, counting `qty`.
    declared_size: int
    #: True when the commander itself did not resolve.
    commander_unresolved: bool

    @property
    def simulated_size(self) -> int:
        return len(self.library)

    @property
    def complete(self) -> bool:
        return not self.unresolved and not self.commander_unresolved


def compile_report(deck: Deck, cards: dict[str, Any] | None) -> CompileReport:
    """Compile a deck and say what happened, including what went missing.

    Raises `NothingToSimulate` when the result is empty. That refusal lives
    here rather than in each caller because it is not a policy question --
    there is no number to report about no cards, and a caller that forgot to
    check would publish the confident nonsense described on that exception.
    """
    library, commander = _compile_cards(deck, cards)
    known = cards or {}
    unresolved = tuple(
        entry.name for entry in deck.cards if entry.name not in known)
    commander_unresolved = bool(deck.commander) and commander is None
    declared = sum(entry.qty for entry in deck.cards)

    if not library:
        raise NothingToSimulate(
            f"{deck.slug} compiles to no cards, so there is nothing to "
            f"simulate (the deck file declares {declared})")

    return CompileReport(
        library=library,
        commander=commander,
        unresolved=unresolved,
        declared_size=declared,
        commander_unresolved=commander_unresolved,
    )


def compile_deck(
    deck: Deck, cards: dict[str, Any] | None,
) -> tuple[list[SimCard], SimCard | None]:
    """Compile a deck into `(library, commander)` SimCards.

    The long-standing shape, kept because most callers only want the cards.
    Anything that will *report a number to somebody* should use
    `compile_report` instead, which also says what the pool could not resolve
    -- silently dropping a card changes every probability in the answer.
    """
    return _compile_cards(deck, cards)


def _compile_cards(
    deck: Deck, cards: dict[str, Any] | None,
) -> tuple[list[SimCard], SimCard | None]:
    """The compilation itself.

    Raises `PoolRequired` when `cards` is missing: mana production cannot be
    inferred from a deck file alone, and guessing would produce numbers that
    look authoritative and are not.
    """
    from mtglab.mana import ManaSource, parse_mana_cost
    from mtglab.sim.tier1.engine import SimCard

    if not cards:
        raise PoolRequired(
            "simulation needs the card pool -- run `mtglab data refresh` first")

    def compile_one(name: str) -> SimCard | None:
        rec = cards.get(name)
        if rec is None:
            return None
        # Only permanents stay on the battlefield making mana. Scryfall reports
        # produced_mana for Treasure-makers like Deadly Dispute too, and
        # without this guard an instant compiles into a permanent mana source.
        front = rec.type_line.split(" // ")[0]
        is_permanent = not ("Instant" in front or "Sorcery" in front)
        produced = frozenset(p for p in rec.produced_mana if p in "WUBRGC")
        # The amount comes from the oracle text; `produced_mana` only ever
        # said which colours. See `mana_produced` for why that mattered.
        amount = mana_produced(rec.oracle_text) if produced else 1
        produces = ((ManaSource(produced, amount),)
                    if (produced and is_permanent) else ())
        is_creature = "Creature" in rec.type_line
        # A fetchland sacrifices itself, so it is net-zero lands and must not
        # count here -- only spells that add a land to the board do.
        fetch = 0 if rec.is_land else fetches_lands(rec.oracle_text)
        return SimCard(
            name=rec.name,
            cost=parse_mana_cost(rec.mana_cost),
            is_land=rec.is_land,
            enters_tapped=rec.is_land and enters_tapped(rec.oracle_text),
            produces=produces,
            produce_delay=1 if (produces and is_creature and not rec.is_land) else 0,
            fetches_lands=fetch,
        )

    # Expand by qty. Basics carry qty 8-16, so ignoring it simulated a deck of
    # ~83 cards with ~20 lands instead of 99 with 34 -- which made every
    # mulligan rate and land-count recommendation wrong.
    library = []
    for entry in deck.cards:
        compiled = compile_one(entry.name)
        if compiled is not None:
            library.extend([compiled] * entry.qty)
    commander = compile_one(deck.commander[0]) if deck.commander else None
    return library, commander
