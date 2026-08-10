import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.artifacts.generate import (annotated_decklist, moxfield_txt,
                                       quick_primer, swap_list, write_all)
from mtglab.decks.model import CardEntry, Deck
from mtglab.decks.validate import validate


@dataclass
class FakeCard:
    """Stands in for a CardRecord so the gate is testable without DuckDB."""
    name: str
    color_identity: frozenset
    type_line: str = "Creature — Human"
    oracle_text: str = ""
    mana_cost: str = "{1}{G}"
    legal_commander: bool = True
    reserved: bool = False

    @property
    def is_land(self):
        return "Land" in self.type_line


G, B, BG = frozenset("G"), frozenset("B"), frozenset("BG")


def make_deck(n=99, **kw):
    cards = [CardEntry(name=f"Card {i}", category="threat", why="does a thing")
             for i in range(n)]
    return Deck(slug="t", name="Test Deck", commander=["Gyome, Master Chef"],
                cards=cards, **kw)


def corpus_for(deck, identity=BG, **overrides):
    out = {"Gyome, Master Chef": FakeCard("Gyome, Master Chef", identity,
                                          "Legendary Creature — Troll Chef")}
    for c in deck.cards + deck.swap_board:
        out[c.name] = FakeCard(c.name, identity)
    out.update(overrides)
    return out


# ------------------------------------------------------------------ model

def test_counts_and_categories():
    deck = make_deck(99)
    deck.cards[0].category = "land"
    deck.cards[1].category = "land"
    assert deck.total_cards == 99
    assert deck.land_count == 2
    assert deck.category_counts["threat"] == 97


def test_yaml_round_trip():
    deck = make_deck(5, bracket=4, strategy="Do the thing.")
    deck.notes["gameplan"] = "Ramp, engine, drain."
    with tempfile.TemporaryDirectory() as tmp:
        deck.dump(Path(tmp) / "deck.yaml")
        back = Deck.load(Path(tmp) / "deck.yaml")
    assert back.total_cards == 5
    assert back.bracket == 4
    assert back.notes["gameplan"] == "Ramp, engine, drain."
    assert back.cards[0].why == "does a thing"


# -------------------------------------------------------------- validation

def test_valid_deck_passes():
    deck = make_deck(99)
    assert validate(deck, corpus_for(deck)).ok


def test_wrong_size_fails():
    deck = make_deck(98)
    rep = validate(deck, corpus_for(deck))
    assert not rep.ok
    assert any(i.code == "deck-size" for i in rep.errors)


def test_missing_rationale_blocks_generation():
    deck = make_deck(99)
    deck.cards[3].why = ""
    rep = validate(deck, corpus_for(deck))
    assert any(i.code == "missing-rationale" for i in rep.errors)


def test_offidentity_card_is_caught():
    """The Ajani, Nacatl Pariah case: a card whose identity leaks a colour."""
    deck = make_deck(99)
    corpus = corpus_for(deck)
    corpus["Card 7"] = FakeCard("Card 7", frozenset("RW"))
    rep = validate(deck, corpus)
    bad = [i for i in rep.errors if i.code == "color-identity"]
    assert bad and bad[0].card == "Card 7"
    assert "R" in bad[0].message and "W" in bad[0].message


def test_banned_card_is_caught():
    deck = make_deck(99)
    corpus = corpus_for(deck)
    corpus["Card 2"] = FakeCard("Card 2", G, legal_commander=False)
    assert any(i.code == "banned" for i in validate(deck, corpus).errors)


def test_duplicate_nonbasic_is_caught():
    deck = make_deck(98)
    deck.cards.append(CardEntry(name="Card 0", category="threat", why="dupe"))
    assert any(i.code == "singleton" for i in validate(deck, corpus_for(deck)).errors)


def test_basics_may_repeat():
    deck = make_deck(95)
    deck.cards.append(CardEntry(name="Forest", category="land", qty=4, why="basic"))
    rep = validate(deck, corpus_for(deck))
    assert not any(i.code == "singleton" for i in rep.errors)


def test_unknown_card_is_an_error_not_a_shrug():
    deck = make_deck(99)
    corpus = corpus_for(deck)
    del corpus["Card 5"]
    assert any(i.code == "unknown-card" for i in validate(deck, corpus).errors)


def test_no_corpus_warns_loudly():
    rep = validate(make_deck(99), None)
    assert any(i.code == "unverified" for i in rep.warnings)


# --------------------------------------------------------------- artifacts

def test_moxfield_txt_shape():
    deck = make_deck(3)
    deck.companion = "Kaheera, the Orphanguard"
    txt = moxfield_txt(deck)
    lines = txt.strip().splitlines()
    assert lines[0].startswith("1 Card")
    assert "SIDEBOARD:" in lines
    tail = lines[lines.index("SIDEBOARD:") + 1:]
    assert "1 Gyome, Master Chef" in tail
    assert "1 Kaheera, the Orphanguard" in tail


def test_annotated_list_includes_every_why():
    deck = make_deck(10)
    deck.cards[0].why = "unique-marker-string"
    out = annotated_decklist(deck, corpus_for(deck))
    assert "unique-marker-string" in out
    for c in deck.cards:
        assert c.name in out


def test_annotated_list_flags_an_unjustified_card():
    deck = make_deck(3)
    deck.cards[1].why = ""
    assert "should not ship" in annotated_decklist(deck)


def test_swap_list_diffs_two_versions():
    old = make_deck(99)
    new = make_deck(99)
    new.cards[0] = CardEntry(name="Pitiless Plunderer", category="ramp",
                             why="Treasure per death; fixes and accelerates.")
    out = swap_list(new, old, prices={"Pitiless Plunderer": 3.49})
    assert "1 out / 1 in" in out
    assert "Card 0" in out and "Pitiless Plunderer" in out
    assert "3.49" in out
    assert "massentry" in out


def test_write_all_produces_five_files():
    deck = make_deck(99)
    prev = make_deck(99)
    prev.cards[0] = CardEntry(name="Old Card", category="threat", why="was here")
    with tempfile.TemporaryDirectory() as tmp:
        written = write_all(deck, tmp, cards=corpus_for(deck), previous=prev,
                            prices={"Card 0": 1.0})
        names = sorted(p.name for p in written)
    assert names == ["decklist-annotated.md", "moxfield.txt", "primer-advanced.md",
                     "primer-quick.md", "swaps.md"]


def test_write_all_omits_swaps_when_nothing_changed():
    deck = make_deck(99)
    with tempfile.TemporaryDirectory() as tmp:
        written = write_all(deck, tmp, cards=corpus_for(deck))
    assert len(written) == 4


def test_quick_primer_surfaces_category_table():
    deck = make_deck(99)
    deck.cards[0].category = "land"
    out = quick_primer(deck, stats={"Commander by T5": "74%"})
    assert "| Lands | 1 |" in out
    assert "74%" in out


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
            except Exception as exc:
                failures += 1
                print(f"  ERROR {name}: {type(exc).__name__}: {exc}")
    print(f"\n{failures} failure(s)")
    sys.exit(1 if failures else 0)
