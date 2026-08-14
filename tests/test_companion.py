"""Companion deckbuilding restrictions.

The gate used to accept `companion: Kaheera, the Orphanguard` after confirming
only that the card had a Companion ability. The restriction itself -- the
entire reason a companion costs you anything -- went unchecked, so a deck with
a non-Cat creature would have been declared legal in writing.

The most important test in here is `test_an_unknown_companion_warns_...`: a
restriction we cannot evaluate must never be reported as satisfied.
"""

import sys
from dataclasses import dataclass, field
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab.decks import companion
from mtglab.decks.model import CardEntry, Deck
from mtglab.decks.validate import validate

# Real oracle sentences, copied from the pool. The checkers parse these, so
# paraphrasing them here would test the wrong thing.
CONDITIONS = {
    "Gyruda, Doom of Depths":
        "Companion — Your starting deck contains only cards with even mana values.",
    "Obosh, the Preypiercer":
        "Companion — Your starting deck contains only cards with odd mana values "
        "and land cards.",
    "Keruga, the Macrosage":
        "Companion — Your starting deck contains only cards with mana value 3 or "
        "greater and land cards.",
    "Lurrus of the Dream-Den":
        "Companion — Each permanent card in your starting deck has mana value 2 or less.",
    "Kaheera, the Orphanguard":
        "Companion — Each creature card in your starting deck is a Cat, Elemental, "
        "Nightmare, Dinosaur, or Beast card.",
    "Jegantha, the Wellspring":
        "Companion — No card in your starting deck has more than one of the same "
        "mana symbol in its mana cost.",
    "Umori, the Collector":
        "Companion — Each nonland card in your starting deck shares a card type.",
    "Zirda, the Dawnwaker":
        "Companion — Each permanent card in your starting deck has an activated ability.",
    "Yorion, Sky Nomad":
        "Companion — Your starting deck contains at least twenty cards more than "
        "the minimum deck size.",
}

GW = frozenset("GW")


@dataclass
class Fake:
    name: str
    type_line: str = "Creature — Cat"
    cmc: float = 2.0
    mana_cost: str = "{1}{G}"
    oracle_text: str = ""
    color_identity: frozenset = field(default_factory=lambda: GW)
    legal_commander: bool = True
    reserved: bool = False
    layout: str = "normal"

    @property
    def is_land(self):
        return "Land" in self.type_line


def comp(name: str, **kw) -> Fake:
    """A companion card carrying its real oracle condition."""
    return Fake(name=name, type_line="Legendary Creature — Cat Beast",
                oracle_text=CONDITIONS.get(name, "") + " (If this card is your "
                            "chosen companion, you may put it into your hand "
                            "from outside the game for {3} as a sorcery.)", **kw)


def run(name, cards):
    """Check `name` against a starting deck given as {card_name: Fake}."""
    pool = dict(cards)
    pool[name] = comp(name)
    return companion.check(name, list(cards.items()), pool)


# ------------------------------------------------------- condition parsing

def test_condition_is_read_off_the_card_with_reminder_text_stripped():
    text = companion.condition_text(comp("Kaheera, the Orphanguard"))
    assert text.startswith("Companion — Each creature card")
    assert "outside the game" not in text, "reminder text should be stripped"


def test_is_companion_needs_the_ability_marker_not_just_the_word():
    """`'companion' in oracle_text` matched flavour and rules text. The
    marker is what identifies the ability."""
    assert companion.is_companion(comp("Kaheera, the Orphanguard"))
    assert not companion.is_companion(
        Fake("Bear", oracle_text="This creature attacks with its companion."))


def test_kaheera_types_come_from_her_own_oracle_text():
    """Not a hardcoded list here -- an errata to the card changes the check."""
    res = run("Kaheera, the Orphanguard", {
        "Ocelot": Fake("Ocelot", "Creature — Cat"),
        "Sprite": Fake("Sprite", "Creature — Elemental"),
        "Rex": Fake("Rex", "Creature — Dinosaur"),
    })
    assert res.ok, res.violations


# ------------------------------------------------------------ the checkers

def test_kaheera_catches_a_creature_of_the_wrong_type():
    res = run("Kaheera, the Orphanguard", {
        "Ocelot": Fake("Ocelot", "Creature — Cat"),
        "Llanowar Elves": Fake("Llanowar Elves", "Creature — Elf Druid"),
    })
    assert res.violations == ["Llanowar Elves"]


def test_kaheera_ignores_noncreature_cards():
    res = run("Kaheera, the Orphanguard", {
        "Ocelot": Fake("Ocelot", "Creature — Cat"),
        "Sol Ring": Fake("Sol Ring", "Artifact"),
        "Plains": Fake("Plains", "Basic Land — Plains"),
    })
    assert res.ok


def test_gyruda_wants_even_mana_values():
    res = run("Gyruda, Doom of Depths", {
        "Two": Fake("Two", cmc=2), "Four": Fake("Four", cmc=4),
        "Three": Fake("Three", cmc=3),
    })
    assert res.violations == ["Three"]


def test_obosh_wants_odd_but_exempts_lands():
    """Lands are mana value 0 -- even -- so without the exemption every land
    in the deck would be reported."""
    res = run("Obosh, the Preypiercer", {
        "One": Fake("One", cmc=1),
        "Mountain": Fake("Mountain", "Basic Land — Mountain", cmc=0),
        "Two": Fake("Two", cmc=2),
    })
    assert res.violations == ["Two"]


def test_keruga_exempts_lands_too():
    res = run("Keruga, the Macrosage", {
        "Big": Fake("Big", cmc=5),
        "Island": Fake("Island", "Basic Land — Island", cmc=0),
        "Small": Fake("Small", cmc=2),
    })
    assert res.violations == ["Small"]


def test_lurrus_limits_permanents_only():
    """A five-mana instant is fine; a three-mana creature is not."""
    res = run("Lurrus of the Dream-Den", {
        "Cheap": Fake("Cheap", "Creature — Cat", cmc=2),
        "Big Instant": Fake("Big Instant", "Instant", cmc=5),
        "Big Creature": Fake("Big Creature", "Creature — Cat", cmc=3),
    })
    assert res.violations == ["Big Creature"]


def test_jegantha_catches_a_repeated_mana_symbol():
    res = run("Jegantha, the Wellspring", {
        "Fine": Fake("Fine", mana_cost="{1}{G}{W}"),
        "Double": Fake("Double", mana_cost="{2}{G}{G}"),
    })
    assert res.violations == ["Double"]


def test_jegantha_does_not_count_generic_mana():
    """{3} is one generic symbol, not three of a kind."""
    res = run("Jegantha, the Wellspring", {"Big": Fake("Big", mana_cost="{7}{G}")})
    assert res.ok


def test_umori_wants_one_shared_nonland_type():
    res = run("Umori, the Collector", {
        "A": Fake("A", "Creature — Cat"), "B": Fake("B", "Creature — Beast"),
        "Forest": Fake("Forest", "Basic Land — Forest"),
    })
    assert res.ok
    res = run("Umori, the Collector", {
        "A": Fake("A", "Creature — Cat"), "Bolt": Fake("Bolt", "Instant"),
    })
    assert res.violations


def test_zirda_is_heuristic_and_reports_inexact():
    """A colon means an activated ability, and so does a bare `Equip {2}`.
    Because that is a heuristic, the caller must downgrade it to a warning."""
    res = run("Zirda, the Dawnwaker", {
        "Clamp": Fake("Clamp", "Artifact — Equipment", oracle_text="Equip {1}"),
        "Sink": Fake("Sink", "Creature — Cat", oracle_text="{2}: Draw a card."),
        "Vanilla": Fake("Vanilla", "Creature — Cat", oracle_text="Flying"),
    })
    assert res.violations == ["Vanilla"]
    assert res.exact is False, "a heuristic must not be reported as exact"


# ------------------------------------------- unknown and uncheckable rules

def test_an_unknown_companion_warns_rather_than_passing_silently():
    """The single most important behaviour here. Reporting 'no violations'
    for a rule that was never evaluated is worse than reporting nothing."""
    pool = {"Mystery": comp("Mystery")}
    pool["Mystery"].oracle_text = "Companion — Some rule we do not implement."
    res = companion.check("Mystery", [], pool)
    assert not res.ok
    assert res.unsupported
    assert res.violations == []


def test_conditions_about_printings_report_that_they_cannot_be_checked():
    """Expansion symbols and retro frames are per-printing, not oracle data."""
    pool = {"Treizeci, Sun of Serra": Fake(
        "Treizeci, Sun of Serra",
        oracle_text="Companion — Your starting deck contains only nostalgic cards.")}
    res = companion.check("Treizeci, Sun of Serra", [], pool)
    assert "per-printing" in res.unsupported


def test_a_card_with_no_companion_ability_is_reported():
    pool = {"Bear": Fake("Bear", oracle_text="Flying")}
    res = companion.check("Bear", [], pool)
    assert "no Companion ability" in res.unsupported


# --------------------------------------------------------- gate integration

def deck_with(companion_name, cards, commander="Arahbo, Roar of the World"):
    entries = [CardEntry(name=n, category="threat", why="x") for n in cards]
    return Deck(slug="t", name="T", commander=[commander],
                companion=companion_name, cards=entries)


def pool_with(deck, cards, companion_name, **kw):
    out = dict(cards)
    out[deck.commander[0]] = Fake(deck.commander[0],
                                  "Legendary Creature — Cat Avatar")
    out[companion_name] = comp(companion_name, **kw)
    return out


def test_gate_reports_a_companion_restriction_as_an_error():
    cards = {"Ocelot": Fake("Ocelot", "Creature — Cat"),
             "Elves": Fake("Elves", "Creature — Elf")}
    deck = deck_with("Kaheera, the Orphanguard", cards)
    rep = validate(deck, pool_with(deck, cards, "Kaheera, the Orphanguard"),
                   expected_size=2)
    codes = [i.code for i in rep.errors]
    assert "companion-restriction" in codes
    assert "Elves" in next(i.message for i in rep.errors
                           if i.code == "companion-restriction")


def test_gate_counts_the_commander_as_part_of_the_starting_deck():
    """'Your starting deck' includes your commander, so a non-Cat commander
    breaks Kaheera even when all 99 are Cats."""
    cards = {"Ocelot": Fake("Ocelot", "Creature — Cat")}
    deck = deck_with("Kaheera, the Orphanguard", cards, commander="Gyome")
    pool = dict(cards)
    pool["Gyome"] = Fake("Gyome", "Legendary Creature — Troll Chef")
    pool["Kaheera, the Orphanguard"] = comp("Kaheera, the Orphanguard")
    rep = validate(deck, pool, expected_size=1)
    assert any(i.code == "companion-restriction" and "Gyome" in i.message
               for i in rep.errors)


def test_gate_downgrades_a_heuristic_check_to_a_warning():
    cards = {"Vanilla": Fake("Vanilla", "Creature — Cat", oracle_text="Flying")}
    deck = deck_with("Zirda, the Dawnwaker", cards)
    rep = validate(deck, pool_with(deck, cards, "Zirda, the Dawnwaker"),
                   expected_size=1)
    assert not any(i.code == "companion-restriction" for i in rep.errors)
    assert any(i.code == "companion-restriction" for i in rep.warnings)


def test_gate_rejects_a_companion_outside_the_commanders_identity():
    cards = {"Ocelot": Fake("Ocelot", "Creature — Cat")}
    deck = deck_with("Kaheera, the Orphanguard", cards)
    pool = pool_with(deck, cards, "Kaheera, the Orphanguard",
                         color_identity=frozenset("UB"))
    rep = validate(deck, pool, expected_size=1)
    assert any(i.code == "companion-color-identity" for i in rep.errors)


def test_gate_rejects_a_companion_that_is_banned():
    cards = {"Ocelot": Fake("Ocelot", "Creature — Cat")}
    deck = deck_with("Kaheera, the Orphanguard", cards)
    pool = pool_with(deck, cards, "Kaheera, the Orphanguard",
                         legal_commander=False)
    rep = validate(deck, pool, expected_size=1)
    assert any(i.code == "companion-banned" for i in rep.errors)


def test_gate_rejects_a_companion_also_present_in_the_99():
    cards = {"Kaheera, the Orphanguard": comp("Kaheera, the Orphanguard")}
    deck = deck_with("Kaheera, the Orphanguard", cards)
    rep = validate(deck, pool_with(deck, cards, "Kaheera, the Orphanguard"),
                   expected_size=1)
    assert any(i.code == "companion-in-99" for i in rep.errors)


def test_yorion_raises_the_expected_deck_size_by_twenty():
    cards = {"Ocelot": Fake("Ocelot", "Creature — Cat")}
    deck = deck_with("Yorion, Sky Nomad", cards)
    rep = validate(deck, pool_with(deck, cards, "Yorion, Sky Nomad"),
                   expected_size=1)
    size = [i for i in rep.errors if i.code == "deck-size"]
    assert size, "a 1-card Yorion deck should fail the size check"
    assert "expected 21" in size[0].message
    assert "Yorion" in size[0].message


def test_a_deck_with_no_companion_is_untouched():
    cards = {"Ocelot": Fake("Ocelot", "Creature — Cat")}
    deck = deck_with(None, cards)
    pool = dict(cards)
    pool[deck.commander[0]] = Fake(deck.commander[0],
                                     "Legendary Creature — Cat Avatar")
    rep = validate(deck, pool, expected_size=1)
    assert not [i for i in rep.issues if i.code.startswith("companion")]


@pytest.mark.parametrize("name", sorted(CONDITIONS))
def test_every_condition_in_this_file_has_a_checker(name):
    """Guards against adding a companion to the pool expectations without
    teaching the gate how to check it."""
    res = companion.check(name, [], {name: comp(name)})
    assert not res.unsupported, f"{name}: {res.unsupported}"
