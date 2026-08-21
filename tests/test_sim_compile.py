"""Oracle-text heuristics used to compile a deck into SimCards.

Both of these were silently wrong and both skewed the land-count
recommendation, which is the headline output of a Tier 1 sweep. The texts
below are quoted from the real pool.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.sim.compile import (
    compile_deck,
    enters_tapped,
    fetches_lands,
    mana_produced,
)

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


def test_a_fetchland_compiles_to_no_ramp_at_all():
    """`compile_one` zeroes the count for anything that *is* a land, and the
    reason is in its comment: a fetchland sacrifices itself, so it is net-zero
    lands. Nothing pinned it. Mutating that `0` to `1` -- every land in the
    deck fetching another -- left `test_sim_compile`, `test_sim_tier1`,
    `test_determinism` and `test_sim_cache` all green, while the engine
    counted every land as ramp (`SimCard.is_ramp`) and drew an extra land for
    each one played. Found by `mtglab mutate run --sample 25 --seed 1`,
    2026-08-19; the sentence above is exactly the shape of the survivor the
    previous run found in `decks/validate.py`.

    The heuristic itself is checked directly, one section up. What this pins
    is the *land* branch, which the oracle text alone can never reach.
    """
    from mtglab.decks.model import CardEntry, Deck

    text = ("{T}, Pay 1 life, Sacrifice this land: Search your library for a "
            "Forest or Plains card, put it onto the battlefield, then shuffle.")
    assert fetches_lands(text) == 1, "the text really does read as a fetch"

    deck = Deck(slug="t", name="T", commander=["Cmd"], cards=[
        CardEntry(name="Windswept Heath", category="land", why="x"),
    ])
    pool = {
        "Cmd": _Rec("Cmd"),
        "Windswept Heath": _Rec("Windswept Heath", "Land", None, text),
    }
    library, _commander = compile_deck(deck, pool)
    fetchland, = [c for c in library if c.name == "Windswept Heath"]
    assert fetchland.fetches_lands == 0
    assert not fetchland.is_ramp


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
    from mtglab.decks.model import CardEntry, Deck

    deck = Deck(slug="t", name="T", commander=["Cmd"], cards=[
        CardEntry(name="Forest", category="land", qty=8, why="x"),
        CardEntry(name="Swamp", category="land", qty=8, why="x"),
        CardEntry(name="Sol Ring", category="ramp", why="x"),
    ])
    pool = {
        "Cmd": _Rec("Cmd"),
        "Forest": _Rec("Forest", "Basic Land — Forest", None, "", ("G",)),
        "Swamp": _Rec("Swamp", "Basic Land — Swamp", None, "", ("B",)),
        "Sol Ring": _Rec("Sol Ring", "Artifact", "{1}", "", ("C",)),
    }
    library, commander = compile_deck(deck, pool)
    assert len(library) == 17, len(library)
    assert sum(1 for c in library if c.is_land) == 16
    assert commander is not None


def test_instants_do_not_become_permanent_mana_sources():
    """Scryfall reports produced_mana for Treasure-makers like Deadly Dispute.
    Casting one must not leave a mana source on the battlefield."""
    from mtglab.decks.model import CardEntry, Deck

    deck = Deck(slug="t", name="T", commander=["Cmd"], cards=[
        CardEntry(name="Deadly Dispute", category="card-advantage", why="x"),
    ])
    pool = {
        "Cmd": _Rec("Cmd"),
        "Deadly Dispute": _Rec("Deadly Dispute", "Instant", "{1}{B}", "",
                               ("W", "U", "B", "R", "G")),
    }
    library, _ = compile_deck(deck, pool)
    assert library[0].produces == ()
    assert not library[0].is_ramp


# --------------------------------------------------------- mana produced
#
# Every oracle text below is quoted from the real pool, checked on 2026-08-21
# rather than recalled -- which is rule 1 applied to the tests as well as to
# the code. The bug these defend against is that Scryfall's `produced_mana`
# names colours and never amounts, so **Sol Ring produced one mana** until
# this function existed, and every deck's acceleration was understated.


def test_sol_ring_makes_two():
    """The card the whole function is named after in the docstring."""
    assert mana_produced("{T}: Add {C}{C}.") == 2


def test_a_plain_source_still_makes_one():
    assert mana_produced("{T}: Add {G}.") == 1
    assert mana_produced("({T}: Add {G}.)") == 1
    assert mana_produced("") == 1
    assert mana_produced("Flying\n{T}: Add one mana of any color.") == 1


def test_a_bigger_rock_makes_what_it_says():
    assert mana_produced("{T}: Add {C}{C}{C}.") == 3
    assert mana_produced(
        "You may spend mana as though it were mana of any color.\n"
        "{T}: Add {C}{C}{C}{C}{C}.\n"
        "{5}, {T}: Draw a card for each color among permanents you control."
    ) == 5, "the {5} draw ability must not be charged against the mana ability"


def test_an_alternative_is_not_a_sum():
    """Talisman of Progress adds one mana, not two.

    Summing an "or" would report every filter land and every Talisman as
    double what it makes, which is the wrong direction: it would tell a deck
    its mana base is better than it is.
    """
    assert mana_produced(
        "{T}: Add {C}.\n"
        "{T}: Add {W} or {U}. This artifact deals 1 damage to you."
    ) == 1


def test_comma_separated_alternatives_are_alternatives_too():
    """A triome reads "Add {R}, {G}, or {W}" and makes one.

    Splitting on "or" alone reads that as two, and Wooded Bastion's
    "Add {G}{G}, {G}{W}, or {W}{W}" as four. Both were wrong in the first
    draft of this function and both are real cards in Aaron's decks.
    """
    assert mana_produced("This land enters tapped.\n{T}: Add {R}, {G}, or {W}.") == 1
    assert mana_produced(
        "{T}: Add {C}.\n{G/W}, {T}: Add {G}{G}, {G}{W}, or {W}{W}."
    ) == 1, "two mana for a cost of one is a net of one"


def test_an_activation_cost_is_subtracted_but_only_its_own_ability():
    """Arcane Signet nets one; Grim Monolith's untap cost is not a mana cost.

    The second half is the subtle one: `{4}: Untap this artifact` lives on its
    own line and has nothing to do with the ability that adds three.
    """
    assert mana_produced("{1}, {T}: Add {W}{U}.") == 1
    assert mana_produced(
        "This artifact doesn't untap during your untap step.\n"
        "{T}: Add {C}{C}{C}.\n"
        "{4}: Untap this artifact."
    ) == 3


def test_a_written_out_amount_is_read():
    """Gilded Lotus contains no mana symbol at all.

    And "any one color" later in the same sentence is not the amount -- only
    the first number word after "add" is.
    """
    assert mana_produced("{T}: Add three mana of any one color.") == 3
    assert mana_produced(
        "{T}: Add one mana of any color in your commander's color identity."
    ) == 1


def test_an_amount_this_cannot_know_falls_through_to_one():
    """Nykthos. Guessing a devotion count would be worse than the floor."""
    assert mana_produced(
        "{T}: Add {C}.\n"
        "{2}, {T}: Choose a color. Add an amount of mana of that color equal "
        "to your devotion to that color."
    ) == 1


def test_adding_something_that_is_not_mana_is_ignored():
    """"Add" is not a mana word on its own, and a counter is not mana."""
    assert mana_produced("Whenever this creature attacks, add a +1/+1 counter "
                         "on it.") == 1


def test_the_amount_reaches_the_compiled_card():
    """The wiring, not just the parser.

    A function that returns 2 and a compiler that ignores it is the bug
    unfixed, and nothing above would catch that.
    """
    from mtglab.decks.model import CardEntry, Deck

    deck = Deck(slug="d", name="D", commander=[],
                cards=[CardEntry(name="Sol Ring", qty=1, category="ramp",
                                 why="fast mana")])
    pool = {"Sol Ring": _Rec("Sol Ring", "Artifact", "{1}", "{T}: Add {C}{C}.",
                             ("C",))}
    library, _ = compile_deck(deck, pool)
    assert [s.amount for s in library[0].produces] == [2]


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
