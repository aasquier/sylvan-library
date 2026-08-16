import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.artifacts.generate import (
    annotated_decklist,
    moxfield_txt,
    quick_primer,
    swap_list,
    write_all,
)
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


def pool_for(deck, identity=BG, **overrides):
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
    assert validate(deck, pool_for(deck)).ok


def test_wrong_size_fails():
    deck = make_deck(98)
    rep = validate(deck, pool_for(deck))
    assert not rep.ok
    assert any(i.code == "deck-size" for i in rep.errors)


def test_missing_rationale_blocks_generation():
    deck = make_deck(99)
    deck.cards[3].why = ""
    rep = validate(deck, pool_for(deck))
    assert any(i.code == "missing-rationale" for i in rep.errors)


def test_offidentity_card_is_caught():
    """The Ajani, Nacatl Pariah case: a card whose identity leaks a colour."""
    deck = make_deck(99)
    pool = pool_for(deck)
    pool["Card 7"] = FakeCard("Card 7", frozenset("RW"))
    rep = validate(deck, pool)
    bad = [i for i in rep.errors if i.code == "color-identity"]
    assert bad and bad[0].card == "Card 7"
    assert "R" in bad[0].message and "W" in bad[0].message


def test_banned_card_is_caught():
    deck = make_deck(99)
    pool = pool_for(deck)
    pool["Card 2"] = FakeCard("Card 2", G, legal_commander=False)
    assert any(i.code == "banned" for i in validate(deck, pool).errors)


def test_duplicate_nonbasic_is_caught():
    deck = make_deck(98)
    deck.cards.append(CardEntry(name="Card 0", category="threat", why="dupe"))
    assert any(i.code == "singleton" for i in validate(deck, pool_for(deck)).errors)


def test_basics_may_repeat():
    deck = make_deck(95)
    deck.cards.append(CardEntry(name="Forest", category="land", qty=4, why="basic"))
    rep = validate(deck, pool_for(deck))
    assert not any(i.code == "singleton" for i in rep.errors)


def test_unknown_card_is_an_error_not_a_shrug():
    deck = make_deck(99)
    pool = pool_for(deck)
    del pool["Card 5"]
    assert any(i.code == "unknown-card" for i in validate(deck, pool).errors)


def test_no_pool_warns_loudly():
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
    out = annotated_decklist(deck, pool_for(deck))
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


def test_write_all_produces_five_files_and_the_snapshot():
    deck = make_deck(99)
    prev = make_deck(99)
    prev.cards[0] = CardEntry(name="Old Card", category="threat", why="was here")
    with tempfile.TemporaryDirectory() as tmp:
        written = write_all(deck, tmp, cards=pool_for(deck), previous=prev,
                            prices={"Card 0": 1.0})
        names = sorted(p.name for p in written)
    # The sixth file is the build's own baseline for the next swaps.md
    # (ADR 30), not a deliverable.
    assert names == ["deck.last-built.yaml", "decklist-annotated.md",
                     "moxfield.txt", "primer-advanced.md", "primer-quick.md",
                     "swaps.md"]


def test_write_all_omits_swaps_when_nothing_changed():
    deck = make_deck(99)
    with tempfile.TemporaryDirectory() as tmp:
        written = write_all(deck, tmp, cards=pool_for(deck))
    assert "swaps.md" not in {p.name for p in written}
    assert len(written) == 5


def test_the_snapshot_round_trips_as_the_next_baseline():
    """What `deck.last-built.yaml` is for: `Deck.load` of it must equal the
    deck that was built, or the next bare build's swaps.md lies."""
    deck = make_deck(9)
    with tempfile.TemporaryDirectory() as tmp:
        write_all(deck, tmp, cards=pool_for(deck))
        again = Deck.load(Path(tmp) / "deck.last-built.yaml")
    assert [c.name for c in again.cards] == [c.name for c in deck.cards]
    assert [c.why for c in again.cards] == [c.why for c in deck.cards]


def test_quick_primer_surfaces_category_table():
    deck = make_deck(99)
    deck.cards[0].category = "land"
    out = quick_primer(deck, stats={"Commander by T5": "74%"})
    assert "| Lands | 1 |" in out
    assert "74%" in out



# --------------------------------------------------------------- deck status

def test_status_defaults_to_theoretical_when_absent():
    """Absent must never mean built. A wrong "built" sends someone to a shelf
    that has no deck on it; a wrong "theoretical" costs nothing."""
    deck = Deck.from_text("slug: x\nname: X\ncards: []\n")
    assert deck.status == "theoretical"


def test_status_is_read_and_normalised():
    assert Deck.from_text("slug: x\nname: X\nstatus: BUILT\n").status == "built"
    assert Deck.from_text("slug: x\nname: X\nstatus: ' theoretical '\n").status \
        == "theoretical"


def test_an_unrecognised_status_fails_the_gate():
    from mtglab.decks.validate import validate
    deck = Deck.from_text("slug: x\nname: X\nstatus: someday\ncards: []\n")
    codes = {i.code for i in validate(deck, None).errors}
    assert "deck-status" in codes


# The two tests that used to live here reading the maintainer's real decks —
# their statuses and stages — left with ADR 30: decks are live app data, so a
# checkout has none to read. The facts they pinned are prose in CLAUDE.md's
# "The decks" section, where a wrong claim is an edit rather than a red suite.


# ---------------------------------------------------------------- deck stage
#
# ADR 13. `stage` says whether a deck has been *reasoned about*; `status` says
# whether it physically exists. They are orthogonal, which is the argument for
# two fields rather than one, and they default in opposite directions.

def test_stage_defaults_to_curated_when_absent():
    """The opposite default from `status`, and for the same reason: the six
    existing decks justify every card, and defaulting to draft would silently
    demote them into decks the gate stops blocking."""
    assert Deck.from_text("slug: x\nname: X\ncards: []\n").stage == "curated"


def test_stage_is_read_and_normalised():
    assert Deck.from_text("slug: x\nname: X\nstage: DRAFT\n").stage == "draft"
    assert Deck.from_text("slug: x\nname: X\nstage: ' draft '\n").stage == "draft"


def test_an_unrecognised_stage_fails_the_gate():
    """Rather than falling back to curated, which would make a typo turn every
    missing rationale into a blocking error the author never asked for."""
    deck = Deck.from_text("slug: x\nname: X\nstage: drafted\ncards: []\n")
    assert "deck-stage" in {i.code for i in validate(deck, None).errors}


def test_a_draft_warns_about_a_missing_why_and_a_curated_deck_blocks():
    for stage, level in (("draft", "warn"), ("curated", "error")):
        deck = make_deck(99, stage=stage)
        deck.cards[3].why = ""
        rep = validate(deck, pool_for(deck))
        codes = {i.code for i in getattr(rep, "errors" if level == "error"
                                        else "warnings")}
        assert ("missing-rationale" in codes) == (level == "error"), stage
        assert ("draft-incomplete" in codes) == (level == "warn"), stage
        assert rep.ok == (stage == "draft"), stage


def test_a_draft_reports_one_counted_issue_rather_than_ninety_nine():
    """ADR 13's actual argument: a number is a better prompt than a wall, and
    ADR 8 needs warnings rare enough that the one that matters is readable."""
    deck = make_deck(99, stage="draft")
    for card in deck.cards:
        card.why = ""
    rep = validate(deck, pool_for(deck))
    assert len(rep.warnings) == 1
    assert rep.warnings[0].code == "draft-incomplete"
    assert "99 of 99 cards still need a `why`" in rep.warnings[0].message


def test_a_draft_still_blocks_on_every_card_fact():
    """Legality and colour identity are facts about cards. They do not become
    negotiable because a deck is new."""
    deck = make_deck(99, stage="draft")
    for card in deck.cards:
        card.why = ""
    pool = pool_for(deck)
    pool["Card 0"] = FakeCard("Card 0", G, legal_commander=False)
    pool["Card 1"] = FakeCard("Card 1", frozenset("U"))
    assert {i.code for i in validate(deck, pool).errors} == \
        {"banned", "color-identity"}


def test_promotion_is_refused_while_any_card_is_blank():
    """`stage: curated` is not something a deck can declare its way into."""
    deck = make_deck(99, stage="curated")
    deck.cards[0].why = ""
    assert not validate(deck, pool_for(deck)).ok


def test_write_all_refuses_a_draft_and_names_what_is_owed():
    """The artifacts are the shareable surface, and a primer for a deck nobody
    has reasoned about looks exactly like one for a deck somebody did."""
    from mtglab.artifacts.generate import DraftDeck

    deck = make_deck(99, stage="draft")
    deck.cards[0].why = ""
    with tempfile.TemporaryDirectory() as tmp:
        try:
            write_all(deck, tmp)
        except DraftDeck as exc:
            assert "Card 0" in str(exc)
        else:
            raise AssertionError("generated artifacts for a draft")
        assert not list(Path(tmp).iterdir()), "wrote something anyway"


def test_write_all_accepts_the_same_deck_once_promoted():
    deck = make_deck(99, stage="curated")
    with tempfile.TemporaryDirectory() as tmp:
        assert len(write_all(deck, tmp)) == 5  # four deliverables + snapshot


def test_a_dumped_draft_carries_a_blank_why_for_every_card():
    """The to-do list, written into the file where the work has to be done."""
    draft = Deck.from_text(Deck(slug="x", name="X", stage="draft",
                                cards=[CardEntry("Sol Ring", "ramp")]).dump())
    assert draft.stage == "draft"
    assert "why: ''" in draft.dump()
    # A curated deck must not get one pre-typed: there it is a blocking error.
    curated = Deck(slug="x", name="X", cards=[CardEntry("Sol Ring", "ramp")])
    assert "why" not in curated.dump()


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
