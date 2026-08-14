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

from typing import TYPE_CHECKING, Any

from mtglab.decks.model import Deck

if TYPE_CHECKING:
    # Import-time only: `engine` is imported inside the function so
    # that Tier 1 stays loadable without this module, and vice versa.
    from mtglab.sim.tier1.engine import SimCard


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


def compile_deck(
    deck: Deck, cards: dict[str, Any] | None,
) -> tuple[list[SimCard], SimCard | None]:
    """Compile a deck into `(library, commander)` SimCards.

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
        produces = (ManaSource(produced),) if (produced and is_permanent) else ()
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
