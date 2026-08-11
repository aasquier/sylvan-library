"""Importing a decklist, and what an imported deck is allowed to claim.

The importer is the first thing in this project that *writes* a deck rather
than reading one, so the tests that matter most are the ones pinning what it
refuses to invent: no rationale, no category beyond land, no name it could not
find in the corpus. ADR 13 is the argument; these are the teeth.

No DuckDB here. `build_deck` takes a name -> CardRecord mapping, so a handful
of fakes covers the resolution rules exactly.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab.cards.db import CardRecord
from mtglab.decks.decklist import parse
from mtglab.decks.importer import ImportRefused, build_deck, names_in
from mtglab.decks.model import Deck
from mtglab.decks.validate import validate


def card(name, *, type_line="Creature — Cat", identity=("G", "W"),
         layout="normal", text="", legal=True) -> CardRecord:
    return CardRecord(
        name=name, mana_cost="{2}{G}", cmc=3.0, type_line=type_line,
        oracle_text=text, color_identity=frozenset(identity),
        produced_mana=(), legal_commander=legal, reserved=False,
        edhrec_rank=None, image_normal=None, layout=layout,
    )


class Corpus(dict):
    """A stand-in for `db.get_cards`'s result, which resolves case-insensitively.

    Worth being faithful about: the real lookup keys its result by the name the
    caller asked for, so `cards["sol ring"]` hits. A plain dict here would make
    the casing tests pass or fail for the wrong reason.
    """

    def __contains__(self, key) -> bool:
        return super().__contains__(str(key).lower())

    def get(self, key, default=None):
        return super().get(str(key).lower(), default)

    def __getitem__(self, key):
        return super().__getitem__(str(key).lower())


CORPUS = Corpus({rec.name.lower(): rec for rec in [
    card("Arahbo, Roar of the World", type_line="Legendary Creature — Cat Avatar"),
    card("Sol Ring", type_line="Artifact", identity=()),
    card("Forest", type_line="Basic Land — Forest", identity=()),
    card("Branchloft Pathway // Boulderloft Pathway",
         type_line="Land // Land", identity=(), layout="modal_dfc"),
    card("Growing Rites of Itlimoc",
         # The trap `CardRecord.is_land` exists for: the *back* is a land, but
         # you cast the front, so it is not a land drop.
         type_line="Legendary Enchantment // Legendary Land",
         layout="transform", identity=("G",)),
    card("Primeval Titan", type_line="Creature — Giant", identity=("G",),
         legal=False),
]})
# `db.get_cards` resolves a double-faced card by either face, and so must the
# fake -- the whole point of the canonicalisation rule is what happens then.
CORPUS["branchloft pathway"] = CORPUS["Branchloft Pathway // Boulderloft Pathway"]


def build(text, **kwargs):
    parsed = parse(text)
    kwargs.setdefault("slug", "imported")
    return build_deck(parsed, CORPUS, **kwargs)


# ------------------------------------------------------- what is not invented

def test_every_card_arrives_with_an_empty_why():
    """Rule 4's whole point. A rationale written by the tool is exactly the
    empty justification the rule exists to prevent, so import writes none."""
    report = build("Commander\n1 Arahbo, Roar of the World\n\nDeck\n1 Sol Ring\n")
    assert [c.why for c in report.deck.cards] == [""]
    assert report.needs_rationale == 1
    assert "why: ''" in report.yaml_text


def test_the_deck_is_a_draft():
    report = build("Commander\n1 Arahbo, Roar of the World\n\nDeck\n1 Sol Ring\n")
    assert report.deck.stage == "draft"
    assert "stage: draft" in report.yaml_text
    # Never silently claimed as owned -- same reason `status` defaults that way.
    assert report.deck.status == "theoretical"


def test_only_lands_get_a_category():
    report = build("Commander\n1 Arahbo, Roar of the World\n"
                   "Deck\n1 Sol Ring\n1 Forest\n1 Growing Rites of Itlimoc\n")
    filed = {c.name: c.category for c in report.deck.cards}
    assert filed == {
        "Sol Ring": "utility",
        "Forest": "land",
        # Its back face is a land and its type line says so, but you cast the
        # front. Filing it under `land` would put a phantom land in the sim.
        "Growing Rites of Itlimoc": "utility",
    }


def test_an_unknown_name_is_kept_verbatim_and_reported():
    """Dropping it would hand back a deck one card short and say nothing.
    Guessing at the intended card would break rule 1."""
    report = build("Commander\n1 Arahbo, Roar of the World\nDeck\n1 Sol Rng\n")
    assert report.unknown == ["Sol Rng"]
    assert [c.name for c in report.deck.cards] == ["Sol Rng"]
    # And the gate says so on its own, without the importer having to.
    assert any(i.code == "unknown-card" for i in validate(report.deck, CORPUS).errors)


def test_no_commander_is_a_refusal_not_a_guess():
    with pytest.raises(ImportRefused) as exc:
        build("1 Sol Ring\n1 Forest\n")
    assert "no commander" in str(exc.value)


def test_the_refusal_points_at_the_sideboard_moxfield_hides_it_in():
    """Our own moxfield.txt artifact writes the commander under `SIDEBOARD:`,
    so a re-import lands it there. Naming the candidates is help; picking one
    would be a guess between two legendary creatures."""
    with pytest.raises(ImportRefused) as exc:
        build("1 Sol Ring\n\nSIDEBOARD:\n1 Arahbo, Roar of the World\n")
    assert "Arahbo, Roar of the World" in str(exc.value)


# ------------------------------------------------------------ what it resolves

def test_casing_is_corrected_but_a_face_name_stays_a_face_name():
    """Every curated deck writes "Branchloft Pathway". Expanding it to the
    combined name on import would make the library inconsistent for no gain --
    `db.get_cards` already resolves both."""
    report = build("Commander\n1 arahbo, roar of the world\n"
                   "Deck\n1 branchloft pathway\n")
    assert report.deck.commander == ["Arahbo, Roar of the World"]
    assert [c.name for c in report.deck.cards] == ["Branchloft Pathway"]


def test_the_combined_name_survives_when_that_is_what_was_written():
    report = build("Commander\n1 Arahbo, Roar of the World\n"
                   "Deck\n1 Branchloft Pathway // Boulderloft Pathway\n")
    assert [c.name for c in report.deck.cards] == [
        "Branchloft Pathway // Boulderloft Pathway"]


def test_repeated_lines_merge_into_one_entry():
    report = build("Commander\n1 Arahbo, Roar of the World\n"
                   "Deck\n20 Forest\n16 Forest\n")
    assert [(c.name, c.qty) for c in report.deck.cards] == [("Forest", 36)]
    assert any("merged" in n for n in report.notes)


def test_a_commander_marked_inline_does_not_land_in_the_99():
    """Moxfield's plain export is 100 lines with the commander marked in
    place, so leaving it in the deck would guarantee a `commander-in-99`
    error for a purely mechanical reason."""
    report = build("1 Arahbo, Roar of the World *CMDR*\n1 Sol Ring\n1 Forest\n")
    assert report.deck.commander == ["Arahbo, Roar of the World"]
    assert [c.name for c in report.deck.cards] == ["Sol Ring", "Forest"]


def test_a_commander_repeated_in_the_deck_section_is_lifted_out_of_it():
    report = build("Commander\n1 Arahbo, Roar of the World\n"
                   "Deck\n1 Arahbo, Roar of the World\n1 Sol Ring\n")
    assert [c.name for c in report.deck.cards] == ["Sol Ring"]
    assert any("outside the 99" in n for n in report.notes)


def test_an_explicit_commander_overrides_the_lists_own():
    """And the card the list nominated does not evaporate. Dropping it because
    of the section header it happened to be under would hand back a deck one
    card short and say nothing about it."""
    report = build("1 Sol Ring *CMDR*\nDeck\n1 Forest\n",
                   commander=["Arahbo, Roar of the World"])
    assert report.deck.commander == ["Arahbo, Roar of the World"]
    assert sorted(c.name for c in report.deck.cards) == ["Forest", "Sol Ring"]
    assert any("were not chosen" in n for n in report.notes)


def test_more_than_two_commanders_is_refused():
    with pytest.raises(ImportRefused) as exc:
        build("Deck\n1 Forest\n", commander=["Arahbo, Roar of the World",
                                             "Sol Ring", "Forest"])
    assert "at most two" in str(exc.value)


def test_a_sideboard_becomes_the_swap_board():
    """Commander has no sideboard, and a list of cards you are considering is
    what the deck file's swap board already holds."""
    report = build("Commander\n1 Arahbo, Roar of the World\n"
                   "Deck\n1 Forest\n\nMaybeboard\n1 Sol Ring\n")
    assert [c.name for c in report.deck.swap_board] == ["Sol Ring"]
    assert [c.name for c in report.deck.cards] == ["Forest"]


def test_names_in_covers_every_lookup_the_build_will_need():
    parsed = parse("Deck\n1 Sol Ring\n1 Forest\n")
    assert names_in(parsed, commander=["Arahbo, Roar of the World"],
                    companion="Kaheera, the Orphanguard") == [
        "Arahbo, Roar of the World", "Forest", "Kaheera, the Orphanguard",
        "Sol Ring"]


# ---------------------------------------------------------- the file it writes

def test_the_yaml_round_trips_through_the_parser():
    report = build("Commander\n1 Arahbo, Roar of the World\n"
                   "Deck\n1 Sol Ring\n36 Forest\n", name="Cats", bracket=4)
    reloaded = Deck.from_text(report.yaml_text)
    assert reloaded.slug == "imported"
    assert reloaded.name == "Cats"
    assert reloaded.bracket == 4
    assert reloaded.stage == "draft"
    assert reloaded.commander == ["Arahbo, Roar of the World"]
    assert reloaded.total_cards == 37
    assert reloaded.land_count == 36


def test_the_file_explains_what_a_draft_is_and_how_to_leave_it():
    """The deck file is the source of truth and the thing the user opens next.
    Telling them there how to promote it is cheaper than any documentation."""
    report = build("Commander\n1 Arahbo, Roar of the World\nDeck\n1 Sol Ring\n")
    header = report.yaml_text.split("slug:")[0]
    assert "stage: curated" in header
    assert "draft" in header.lower()


def test_the_facts_are_checked_on_day_one_even_though_the_thinking_is_not():
    """The whole bargain of ADR 13: a banned card is an error immediately, and
    99 missing rationales are a counted warning rather than 99 errors."""
    report = build("Commander\n1 Arahbo, Roar of the World\n"
                   "Deck\n1 Primeval Titan\n")
    rep = validate(report.deck, CORPUS)
    assert [i.code for i in rep.errors] == ["deck-size", "banned"]
    assert [i.code for i in rep.warnings] == ["draft-incomplete"]
