"""Which printing's art a deck shows for its commander.

A **deck** property, not a viewer preference: `deck.yaml` is the source of
truth (ADR 1) and the choice travels with the deck through git the way every
other decision about it does. That is the design decision these tests pin —
along with the two checks that keep it honest, which sit in different layers
on purpose.

`edit.py` checks the *shape* of the value, because it is text surgery over YAML
and has never had a database. `service.py` checks that the printing is one of
**this commander's**, because only a query can answer that. Splitting them is
what lets a deck edit stay pure while still refusing to point a deck at some
other card's painting.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import tiny_pool
from mtglab import config
from mtglab.api import service
from mtglab.decks import edit
from mtglab.decks.model import Deck
from mtglab.decks.source import MemoryDeckSource

DECK_YAML = """\
slug: mini
name: Mini Deck
status: theoretical
stage: curated
commander:
  - Gyome, Master Chef
bracket: 4
cards:
  - name: Swamp
    category: land
    qty: 99
    why: Black mana.
"""

A_PRINTING = "de8a64e5-9986-4692-b173-43475f4b5005"


@pytest.fixture
def source(tmp_path):
    path = tmp_path / "deck.yaml"
    path.write_text(DECK_YAML, encoding="utf-8")
    return MemoryDeckSource([Deck.load(path)])


@pytest.fixture
def pool(tmp_path):
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_pool.build(config.DB_PATH)


# ----------------------------------------------------------------- the file

def test_the_field_round_trips_through_the_deck_file():
    deck = Deck.from_text(DECK_YAML)
    assert deck.commander_art == ""
    deck.commander_art = A_PRINTING
    assert Deck.from_text(deck.dump()).commander_art == A_PRINTING


def test_a_deck_that_never_picked_one_is_byte_identical_after_a_round_trip():
    """The six curated decks predate this field. A round trip must not start
    writing `commander_art: ''` into all of them, which would put a line in
    every diff that means nothing."""
    assert "commander_art" not in Deck.from_text(DECK_YAML).dump()


def test_setting_it_writes_one_line_in_the_right_place():
    """ADR 12 rule 1: an edit touches only what it changes. The key is placed
    where `Deck.dump` would put it rather than appended to the end."""
    updated = edit.set_deck_field(DECK_YAML, field="commander_art",
                                  value=A_PRINTING)
    assert f"commander_art: {A_PRINTING}" in updated
    assert Deck.from_text(updated).commander_art == A_PRINTING
    # Everything else survived, including the card and its rationale.
    assert Deck.from_text(updated).cards[0].why == "Black mana."


def test_a_typo_is_refused_by_shape_before_any_query():
    with pytest.raises(edit.EditFailed, match="Scryfall printing id"):
        edit.set_deck_field(DECK_YAML, field="commander_art", value="blc")


def test_clearing_it_is_a_real_operation():
    """Emptying the field means 'back to the default printing', which somebody
    will want after trying one. It is allowed through rather than refused as a
    blank value."""
    picked = edit.set_deck_field(DECK_YAML, field="commander_art",
                                 value=A_PRINTING)
    cleared = edit.set_deck_field(picked, field="commander_art", value="")
    assert Deck.from_text(cleared).commander_art == ""


def test_the_case_of_the_id_is_preserved():
    """The one settable deck field that must not be lowercased: it is an opaque
    identifier in somebody else's system, and `stage`/`status` being enums is
    why the shared path lowers them."""
    mixed = "DE8A64E5-9986-4692-b173-43475f4b5005"
    updated = edit.set_deck_field(DECK_YAML, field="commander_art", value=mixed)
    assert Deck.from_text(updated).commander_art == mixed


# ------------------------------------------------------------- the printings

def test_printings_are_listed_newest_first(pool, source):
    listing = service.commander_printings("mini", source=source)
    assert listing["commander"] == "Gyome, Master Chef"
    dates = [p["released_at"] for p in listing["printings"] if p["released_at"]]
    assert dates == sorted(dates, reverse=True)


def test_every_listed_printing_can_actually_be_shown(pool, source):
    """A printing with no image is a row the picker would render as a blank
    tile, so it is filtered out rather than offered."""
    for printing in service.commander_printings("mini", source=source)["printings"]:
        assert printing["image"]


def test_the_selected_printing_is_marked(pool, tmp_path):
    yaml = DECK_YAML.replace("bracket: 4",
                             f"commander_art: {A_PRINTING}\nbracket: 4")
    path = tmp_path / "picked" / "deck.yaml"
    path.parent.mkdir()
    path.write_text(yaml, encoding="utf-8")
    src = MemoryDeckSource([Deck.load(path)])

    listing = service.commander_printings("mini", source=src)
    assert listing["selected"] == A_PRINTING
    chosen = [p for p in listing["printings"] if p["selected"]]
    # The tiny pool carries no printings table, so this asserts the contract
    # rather than a row: whatever is listed, at most the picked one is marked.
    assert len(chosen) <= 1


def test_a_deck_with_no_commander_lists_nothing_rather_than_failing(tmp_path,
                                                                    pool):
    yaml = DECK_YAML.replace("commander:\n  - Gyome, Master Chef\n",
                             "commander: []\n")
    path = tmp_path / "headless" / "deck.yaml"
    path.parent.mkdir()
    path.write_text(yaml, encoding="utf-8")
    src = MemoryDeckSource([Deck.load(path)])
    assert service.commander_printings("mini", source=src)["printings"] == []


def test_printings_survive_a_pool_that_is_not_there(tmp_path, source):
    """A fresh clone shows a deck with its default art and an empty picker."""
    with config.use_paths(data_dir=tmp_path / "empty"):
        assert service.commander_printings("mini", source=source)["printings"] == []


# ------------------------------------------------------- deriving the crop

def test_the_crop_url_differs_from_the_full_one_only_in_its_size_segment():
    """Checkable rather than assumed: `oracle_cards` stores both URLs for the
    same printing id, and they are identical but for this segment. That is why
    deriving it is safe, and why `printings` not having a crop column does not
    block the feature on a 500MB re-ingest."""
    normal = "https://cards.scryfall.io/normal/front/e/e/ee47f23a.jpg?178"
    assert service.art_crop_from(normal) == (
        "https://cards.scryfall.io/art_crop/front/e/e/ee47f23a.jpg?178")


def test_an_unexpected_url_shape_gives_up_rather_than_guessing():
    """A wrong URL renders as a broken image; None renders as the card."""
    assert service.art_crop_from("https://example.com/some/other/thing.jpg") is None
    assert service.art_crop_from(None) is None


def test_only_the_first_size_segment_is_replaced():
    """A card whose id happens to contain the word would otherwise be mangled
    twice — belt and braces on a string replacement over somebody's URLs."""
    normal = "https://cards.scryfall.io/normal/front/n/o/normal-ish.jpg"
    assert service.art_crop_from(normal).count("art_crop") == 1


# ---------------------------------------------------- refusing the wrong card

GYOME_PRINTING = "11111111-1111-4111-8111-111111111111"
SOL_RING_PRINTING = "22222222-2222-4222-8222-222222222222"


@pytest.fixture
def printed(pool):
    """Two real printing rows: one of the commander, one of another card.

    `tiny_pool` loads oracle rows only, so without this the printings table
    is empty and every id is refused — which would make the test below pass
    while proving nothing about *whose* card the printing is.
    """
    from mtglab.cards import db

    con = db.connect(pool)
    try:
        for card, pid in (("Gyome, Master Chef", GYOME_PRINTING),
                          ("Sol Ring", SOL_RING_PRINTING)):
            oracle_id = con.execute(
                "SELECT oracle_id FROM oracle_cards WHERE name = ?",
                [card]).fetchone()[0]
            con.execute(
                "INSERT INTO printings (id, oracle_id, name, set_code, "
                "set_name, collector_number, rarity, released_at, digital, "
                "promo, image_normal) VALUES (?, ?, ?, 'tst', 'Test Set', "
                "'1', 'rare', DATE '2024-01-01', FALSE, FALSE, "
                "'https://cards.scryfall.io/normal/front/a/b/x.jpg')",
                [pid, oracle_id, card])
    finally:
        con.close()
    yield


def test_a_printing_of_another_card_is_refused(printed, source):
    """The check that needs a database, and the reason it lives in `service`.

    Every shape check passes here — it is a well-formed printing id that
    genuinely exists. Only a query can say it belongs to Sol Ring.
    """
    deck = Deck.from_text(DECK_YAML)
    with pytest.raises(service.EditRejected, match="not a printing of"):
        service._check_printing(deck, SOL_RING_PRINTING)


def test_a_printing_of_this_commander_is_accepted(printed, source):
    deck = Deck.from_text(DECK_YAML)
    service._check_printing(deck, GYOME_PRINTING)      # does not raise


def test_the_chosen_printing_replaces_the_art_and_nothing_else(printed, tmp_path):
    """A cosmetic choice must not look like it changed what the commander does,
    so oracle text, cost and type line stay the card's."""
    yaml = DECK_YAML.replace("bracket: 4",
                             f"commander_art: {GYOME_PRINTING}\nbracket: 4")
    path = tmp_path / "picked" / "deck.yaml"
    path.parent.mkdir()
    path.write_text(yaml, encoding="utf-8")
    src = MemoryDeckSource([Deck.load(path)])

    plain = service.get_deck("mini", source=source_for(DECK_YAML, tmp_path))
    picked = service.get_deck("mini", source=src)

    assert picked["commander_card"]["image"].endswith("/a/b/x.jpg")
    assert picked["commander_card"]["printing"]["set_code"] == "TST"
    assert picked["commander_card"]["oracle_text"] == \
        plain["commander_card"]["oracle_text"]
    assert picked["commander_card"]["mana_cost"] == \
        plain["commander_card"]["mana_cost"]


def source_for(yaml_text, tmp_path):
    path = tmp_path / "plain" / "deck.yaml"
    path.parent.mkdir(exist_ok=True)
    path.write_text(yaml_text, encoding="utf-8")
    return MemoryDeckSource([Deck.load(path)])


def test_a_stale_art_id_falls_back_to_the_default_printing(printed, tmp_path):
    """A deck pointing at a printing the pool no longer has shows its default
    art, not a hole where the commander should be."""
    gone = "33333333-3333-4333-8333-333333333333"
    yaml = DECK_YAML.replace("bracket: 4", f"commander_art: {gone}\nbracket: 4")
    path = tmp_path / "stale" / "deck.yaml"
    path.parent.mkdir()
    path.write_text(yaml, encoding="utf-8")
    src = MemoryDeckSource([Deck.load(path)])

    card = service.get_deck("mini", source=src)["commander_card"]
    assert card is not None and card["image"]
    assert "printing" not in card


def test_no_pool_means_the_choice_is_accepted_rather_than_blocked(tmp_path,
                                                                    source):
    """A machine without the 500MB download cannot check, and refusing every
    art change there would be a worse answer than accepting one that renders
    as the default."""
    deck = Deck.from_text(DECK_YAML)
    with config.use_paths(data_dir=tmp_path / "empty"):
        service._check_printing(deck, A_PRINTING)      # does not raise
