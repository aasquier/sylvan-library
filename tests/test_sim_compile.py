"""Oracle-text heuristics used to compile a deck into SimCards.

Both of these were silently wrong and both skewed the land-count
recommendation, which is the headline output of a Tier 1 sweep. The texts
below are quoted from the real corpus.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.cli import enters_tapped, fetches_lands

# ------------------------------------------------------------- enters tapped

def test_current_scryfall_wording_is_detected():
    """The bug: the old check looked for 'enters the battlefield tapped',
    but every modern printing says 'enters tapped'."""
    assert enters_tapped("This land enters tapped.")


def test_legacy_wording_still_detected():
    assert enters_tapped("Karoo enters the battlefield tapped.")


def test_self_named_wording_is_detected():
    assert enters_tapped("The Shire enters tapped unless you control a legendary creature.") is False
    assert enters_tapped("The Shire enters tapped.")


def test_conditional_taplands_count_as_untapped():
    """Tier 1 cannot evaluate the condition. These resolve untapped in most
    real games, so treating them as tapped would slow every deck."""
    assert not enters_tapped("This land enters tapped unless you control a Forest or a Plains.")
    assert not enters_tapped("This land enters tapped unless you control two or more other lands.")
    assert not enters_tapped("This land enters tapped unless you have two or more opponents.")


def test_shockland_is_untapped():
    assert not enters_tapped(
        "As this land enters, you may pay 2 life. If you don't, it enters tapped.")


def test_plain_land_is_untapped():
    assert not enters_tapped("({T}: Add {G}.)")
    assert not enters_tapped("")


# -------------------------------------------------------------- land fetching

def test_single_land_fetch():
    assert fetches_lands(
        "Search your library for a Forest card, put that card onto the "
        "battlefield, then shuffle.") == 1


def test_up_to_two_lands():
    assert fetches_lands(
        "Search your library for up to two Forest cards, put them onto the "
        "battlefield, then shuffle.") == 2


def test_sacrifice_ramp_creature_counts():
    assert fetches_lands(
        "Sacrifice this creature: Search your library for a basic land card, "
        "put that card onto the battlefield tapped, then shuffle.") == 1


def test_tutor_that_is_not_land_ramp_scores_zero():
    """Demonic Tutor must not read as ramp."""
    assert fetches_lands(
        "Search your library for a card, put that card into your hand, then "
        "shuffle.") == 0


def test_land_search_to_hand_is_not_battlefield_ramp():
    """Many Partings puts a basic in HAND -- that is not acceleration."""
    assert fetches_lands(
        "Search your library for a basic land card, reveal it, put it into "
        "your hand, then shuffle. Create a Food token.") == 0


def test_blank_text_scores_zero():
    assert fetches_lands("") == 0


# ------------------------------------------------------------ qty expansion

class _Rec:
    """Minimal stand-in for a CardRecord, so this needs no DuckDB."""

    def __init__(self, name, type_line="Creature — Troll", mana_cost="{1}{G}",
                 oracle_text="", produced_mana=()):
        self.name = name
        self.type_line = type_line
        self.mana_cost = mana_cost
        self.oracle_text = oracle_text
        self.produced_mana = produced_mana

    @property
    def is_land(self):
        return "Land" in self.type_line


def test_qty_is_expanded_into_the_library():
    """The bug: basics carry qty 8-16 and compile_one ran once per ENTRY, so a
    99-card deck simulated as ~83 cards with far too few lands."""
    from mtglab.cli import _sim_cards
    from mtglab.decks.model import CardEntry, Deck

    deck = Deck(slug="t", name="T", commander=["Cmd"], cards=[
        CardEntry(name="Forest", category="land", qty=8, why="x"),
        CardEntry(name="Swamp", category="land", qty=8, why="x"),
        CardEntry(name="Sol Ring", category="ramp", why="x"),
    ])
    corpus = {
        "Cmd": _Rec("Cmd"),
        "Forest": _Rec("Forest", "Basic Land — Forest", None, "", ("G",)),
        "Swamp": _Rec("Swamp", "Basic Land — Swamp", None, "", ("B",)),
        "Sol Ring": _Rec("Sol Ring", "Artifact", "{1}", "", ("C",)),
    }
    library, commander = _sim_cards(deck, corpus)
    assert len(library) == 17, len(library)
    assert sum(1 for c in library if c.is_land) == 16
    assert commander is not None


def test_instants_do_not_become_permanent_mana_sources():
    """Scryfall reports produced_mana for Treasure-makers like Deadly Dispute.
    Casting one must not leave a mana source on the battlefield."""
    from mtglab.cli import _sim_cards
    from mtglab.decks.model import CardEntry, Deck

    deck = Deck(slug="t", name="T", commander=["Cmd"], cards=[
        CardEntry(name="Deadly Dispute", category="card-advantage", why="x"),
    ])
    corpus = {
        "Cmd": _Rec("Cmd"),
        "Deadly Dispute": _Rec("Deadly Dispute", "Instant", "{1}{B}", "",
                               ("W", "U", "B", "R", "G")),
    }
    library, _ = _sim_cards(deck, corpus)
    assert library[0].produces == ()
    assert not library[0].is_ramp


if __name__ == "__main__":
    failures = 0
    for name, fn in sorted(globals().items()):
        if name.startswith("test_") and callable(fn):
            try:
                fn()
                print(f"  PASS  {name}")
            except AssertionError as exc:
                failures += 1
                print(f"  FAIL  {name}: {exc}")
    print(f"\n{failures} failure(s)")
    sys.exit(1 if failures else 0)
