"""Two-commander pairings: Partner, Backgrounds, Doctors.

The gate assumed one commander, which made it wrong about legal decks in two
ways worth pinning forever:

* A Background is a Legendary Enchantment whose text never says it can be your
  commander, so `not-a-commander` rejected Jaheira + Raised by Giants.
* Ten `Partner with` cards from Battlebond are not Legendary at all. CR
  702.124f grants commander eligibility through the ability rather than
  printing it, so a type-line test rejects them.

Plus the arithmetic: two commanders means a 98-card deck.

Oracle text below is copied from the corpus. The detector parses these exact
shapes, so paraphrasing would test the wrong thing.
"""

import sys
from dataclasses import dataclass, field
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.decks import partners
from mtglab.decks.model import CardEntry, Deck
from mtglab.decks.validate import validate

PARTNER = "Partner (You can have two commanders if both have partner.)"
FRIENDS = ("Partner—Friends forever (You can have two commanders if both have "
           "this ability.)")
SURVIVORS = ("Partner—Survivors (You can have two commanders if both have this "
             "ability.)")
BACKGROUND_CHOOSER = ("Choose a Background (You can have a Background as a "
                      "second commander.)")
DOCTORS_COMPANION = ("Doctor's companion (You can have two commanders if the "
                     "other is the Doctor.)")

G = frozenset("G")


@dataclass
class Fake:
    name: str
    type_line: str = "Legendary Creature — Human"
    oracle_text: str = ""
    color_identity: frozenset = field(default_factory=lambda: G)
    cmc: float = 3.0
    mana_cost: str = "{2}{G}"
    legal_commander: bool = True
    reserved: bool = False
    layout: str = "normal"

    @property
    def is_land(self):
        return "Land" in self.type_line


# ------------------------------------------------------------- detection

def test_detects_plain_partner():
    p = partners.pairing(Fake("Ghost", oracle_text="Flying\n" + PARTNER))
    assert p.kind == partners.PARTNER


def test_detects_partner_with_and_extracts_the_named_card():
    p = partners.pairing(Fake("Lore Weaver", oracle_text=(
        "Partner with Ley Weaver (When this creature enters, target player "
        "may put Ley Weaver into their hand from their library, then shuffle.)")))
    assert p.kind == partners.PARTNER_WITH
    assert p.partner_name == "Ley Weaver"


def test_partner_with_parses_without_reminder_text():
    p = partners.pairing(Fake("A", oracle_text="Partner with Ley Weaver"))
    assert p.partner_name == "Ley Weaver"


def test_detects_labeled_partner_and_keeps_the_label():
    """`Partner—<label>` is a generalised template; the corpus already has
    four labels, so matching the label beats hardcoding one."""
    p = partners.pairing(Fake("Sophina", oracle_text="Menace\n" + FRIENDS))
    assert p.kind == partners.LABELED
    assert p.label == "Friends forever"


def test_partner_with_is_not_mistaken_for_plain_partner():
    """Both lines start with 'Partner', so ordering in the detector matters."""
    assert partners.pairing(
        Fake("A", oracle_text="Partner with Ley Weaver")).kind == partners.PARTNER_WITH
    assert partners.pairing(
        Fake("A", oracle_text=FRIENDS)).kind == partners.LABELED


def test_detects_background_chooser_and_background():
    assert partners.pairing(
        Fake("Jaheira", oracle_text=BACKGROUND_CHOOSER)).kind == partners.BACKGROUND_CHOOSER
    assert partners.is_background(
        Fake("Raised by Giants", "Legendary Enchantment — Background"))


def test_detects_doctor_and_companion():
    assert partners.pairing(
        Fake("Nyssa", oracle_text=DOCTORS_COMPANION)).kind == partners.DOCTORS_COMPANION
    assert partners.is_doctor(
        Fake("The Eighth Doctor", "Legendary Creature — Time Lord Doctor"))


def test_a_card_with_no_pairing_ability_returns_none():
    assert partners.pairing(Fake("Bear", oracle_text="Flying")) is None


# ----------------------------------------------------------- legal pairs

def test_two_plain_partners_pair():
    a = Fake("Ghost", oracle_text=PARTNER)
    b = Fake("Silas", oracle_text=PARTNER)
    assert partners.check_pair(a, b) is None


def test_partner_with_pairs_only_with_its_named_card():
    a = Fake("Lore Weaver", "Creature — Human Wizard",
             oracle_text="Partner with Ley Weaver")
    b = Fake("Ley Weaver", "Creature — Elf Druid",
             oracle_text="Partner with Lore Weaver")
    assert partners.check_pair(a, b) is None
    assert partners.check_pair(b, a) is None, "order must not matter"

    wrong = Fake("Silas", oracle_text=PARTNER)
    problem = partners.check_pair(a, wrong)
    assert problem and "only pair with that card" in problem


def test_same_label_pairs_and_different_labels_do_not():
    a = Fake("Sophina", oracle_text=FRIENDS)
    b = Fake("Othelm", oracle_text=FRIENDS)
    assert partners.check_pair(a, b) is None

    c = Fake("Ellie", oracle_text=SURVIVORS)
    problem = partners.check_pair(a, c)
    assert problem and "Friends forever" in problem and "Survivors" in problem


def test_background_chooser_pairs_with_a_background_either_way_round():
    a = Fake("Jaheira", oracle_text=BACKGROUND_CHOOSER)
    b = Fake("Raised by Giants", "Legendary Enchantment — Background")
    assert partners.check_pair(a, b) is None
    assert partners.check_pair(b, a) is None


def test_background_chooser_rejects_a_non_background():
    a = Fake("Jaheira", oracle_text=BACKGROUND_CHOOSER)
    b = Fake("Ghost", oracle_text=PARTNER)
    problem = partners.check_pair(a, b)
    assert problem and "is not a Background" in problem


def test_doctors_companion_pairs_with_a_doctor_only():
    a = Fake("Nyssa", oracle_text=DOCTORS_COMPANION)
    doc = Fake("The Eighth Doctor", "Legendary Creature — Time Lord Doctor")
    assert partners.check_pair(a, doc) is None

    problem = partners.check_pair(a, Fake("Silas", oracle_text=PARTNER))
    assert problem and "not a Doctor" in problem


def test_two_cards_with_no_pairing_ability_are_rejected():
    problem = partners.check_pair(Fake("A"), Fake("B"))
    assert problem and "neither card has a pairing ability" in problem


def test_one_sided_partner_is_rejected():
    problem = partners.check_pair(Fake("Ghost", oracle_text=PARTNER), Fake("B"))
    assert problem and "only" in problem


# ------------------------------------------------- commander eligibility

def test_a_legendary_creature_is_always_eligible():
    rec = Fake("Gyome", "Legendary Creature — Troll Chef")
    assert partners.can_be_commander(rec, paired=False)
    assert partners.can_be_commander(rec, paired=True)


def test_a_background_is_eligible_only_as_one_of_two():
    rec = Fake("Raised by Giants", "Legendary Enchantment — Background")
    assert not partners.can_be_commander(rec, paired=False)
    assert partners.can_be_commander(rec, paired=True)


def test_a_non_legendary_partner_with_creature_is_eligible_when_paired():
    """CR 702.124f grants this through the ability, not the type line -- the
    Battlebond partners are not Legendary."""
    rec = Fake("Lore Weaver", "Creature — Human Wizard",
               oracle_text="Partner with Ley Weaver")
    assert not partners.can_be_commander(rec, paired=False)
    assert partners.can_be_commander(rec, paired=True)


def test_explicit_can_be_your_commander_text_is_honoured():
    rec = Fake("Rograkh", "Legendary Artifact",
               oracle_text="Anything can be your commander.")
    assert partners.can_be_commander(rec, paired=False)


# --------------------------------------------------------- gate wiring

def build(commanders, body, corpus_extra=None):
    deck = Deck(slug="t", name="T", commander=commanders,
                cards=[CardEntry(name=n, category="ramp", why="x") for n in body])
    corpus = {n: Fake(n, "Artifact") for n in body}
    corpus.update(corpus_extra or {})
    return deck, corpus


def test_two_commanders_reduce_the_expected_deck_size():
    deck, corpus = build(["Ghost", "Silas"], ["Sol Ring"], {
        "Ghost": Fake("Ghost", oracle_text=PARTNER),
        "Silas": Fake("Silas", oracle_text=PARTNER)})
    rep = validate(deck, corpus, expected_size=2)     # 2 - 1 == 1
    assert rep.ok, rep.render()


def test_the_size_error_explains_the_two_commander_adjustment():
    deck, corpus = build(["Ghost", "Silas"], ["Sol Ring", "Mox"], {
        "Ghost": Fake("Ghost", oracle_text=PARTNER),
        "Silas": Fake("Silas", oracle_text=PARTNER)})
    rep = validate(deck, corpus, expected_size=2)
    msg = next(i.message for i in rep.errors if i.code == "deck-size")
    assert "expected 1" in msg and "2 commanders" in msg


def test_three_commanders_are_rejected():
    deck, corpus = build(["A", "B", "C"], ["Sol Ring"], {
        n: Fake(n, oracle_text=PARTNER) for n in "ABC"})
    rep = validate(deck, corpus, expected_size=3)
    assert any(i.code == "too-many-commanders" for i in rep.errors)


def test_a_background_alone_is_rejected_with_a_useful_message():
    deck, corpus = build(["Raised by Giants"], ["Sol Ring"], {
        "Raised by Giants": Fake("Raised by Giants",
                                 "Legendary Enchantment — Background")})
    rep = validate(deck, corpus, expected_size=1)
    msg = next(i.message for i in rep.errors if i.code == "not-a-commander")
    assert "one of two commanders" in msg


def test_a_legal_background_pairing_passes_the_gate():
    """The regression this whole module exists for."""
    deck, corpus = build(["Jaheira", "Raised by Giants"], ["Sol Ring"], {
        "Jaheira": Fake("Jaheira", oracle_text=BACKGROUND_CHOOSER),
        "Raised by Giants": Fake("Raised by Giants",
                                 "Legendary Enchantment — Background")})
    rep = validate(deck, corpus, expected_size=2)
    assert rep.ok, rep.render()


def test_an_illegal_pairing_is_reported():
    deck, corpus = build(["Jaheira", "Ghost"], ["Sol Ring"], {
        "Jaheira": Fake("Jaheira", oracle_text=BACKGROUND_CHOOSER),
        "Ghost": Fake("Ghost", oracle_text=PARTNER)})
    rep = validate(deck, corpus, expected_size=2)
    assert any(i.code == "illegal-pairing" for i in rep.errors)


def test_colour_identity_is_the_union_of_both_commanders():
    deck, corpus = build(["Ghost", "Silas"], ["Blue Card"], {
        "Ghost": Fake("Ghost", oracle_text=PARTNER, color_identity=frozenset("B")),
        "Silas": Fake("Silas", oracle_text=PARTNER, color_identity=frozenset("U")),
        "Blue Card": Fake("Blue Card", "Artifact", color_identity=frozenset("U"))})
    rep = validate(deck, corpus, expected_size=2)
    assert not [i for i in rep.errors if i.code == "color-identity"], rep.render()


def test_a_single_commander_deck_is_unaffected():
    deck, corpus = build(["Gyome"], ["Sol Ring"], {
        "Gyome": Fake("Gyome", "Legendary Creature — Troll Chef")})
    rep = validate(deck, corpus, expected_size=1)
    assert rep.ok, rep.render()
    assert not [i for i in rep.issues
                if i.code in ("illegal-pairing", "too-many-commanders")]
