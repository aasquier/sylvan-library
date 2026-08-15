"""Surgical deck.yaml edits.

`deck.yaml` is the source of truth and `swaps.md` is a git diff of it, so the
*size* of an edit is part of its correctness: a one-card swap that rewrites the
file produces a swap record nobody can read. These tests pin that, and pin the
refusals -- the failure this module must not have is silently corrupting the
one file the whole project is built on.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest
import yaml

from mtglab.decks.edit import (
    EditFailed,
    add_card,
    entomb_card,
    exile_card,
    remove_card,
    replace_card,
    return_card,
    set_card_field,
    set_deck_field,
    set_note,
)

DECK = """\
# A deck file with comments that must survive.
slug: mini
name: Mini Deck
commander:
  - Gyome, Master Chef
bracket: 4

notes:
  gameplan: >
    Ramp on one through three, land the commander, then assemble any outlet
    plus any payoff.
  mulligan: Keep two to five lands.

# The 99 follows.
cards:
  - name: Swamp
    category: land
    qty: 98
    why: Black mana.
  - name: Primeval Titan
    category: ramp
    why: >
      6/6 trample fetching two lands on ETB and on attack.
      Ramp and threat in one card.
  - name: Sol Ring
    category: ramp
    why: Two mana for one.

swap_board:
  - name: Reliquary Tower
    category: land
    why: No maximum hand size.
"""

# The shape the real files use and the synthetic one above does not: cards
# grouped under banner comments, with a blank line above each banner. This is
# what `_split_tail` exists for, and editing near a banner is where a naive
# line-range edit does its damage.
BANNERED = """\
slug: bannered
name: Bannered Deck
commander:
  - Goreclaw, Terror of Qal Sisma

cards:
  # ------------------------------------------------------------------ LANDS
  - name: Forest
    category: land
    qty: 30
    why: Green mana.
  - name: Dryad Arbor
    category: land
    why: >
      A Forest that is also a creature, fetchable off Nature's Lore.

  # ------------------------------------------------------------------- RAMP
  - name: Sol Ring
    category: ramp
    why: Two mana for one.
"""


def changed(before: str, after: str) -> list[str]:
    import difflib
    return [
        line for line in difflib.unified_diff(before.splitlines(), after.splitlines(),
                                              lineterm="", n=0)
        if line[:1] in "+-" and line[:3] not in ("---", "+++")
    ]


def changed_lines(before: str, after: str) -> int:
    return len(changed(before, after))


# ------------------------------------------------------------- minimal diff

def test_a_one_card_swap_is_a_one_card_diff():
    out = replace_card(DECK, old_name="Primeval Titan",
                       new_name="Cultivator Colossus", why="Lands into a body.")
    # Three lines out (name, and a two-line folded why), two in.
    assert changed_lines(DECK, out) <= 6, out


def test_comments_and_blank_lines_survive():
    out = replace_card(DECK, old_name="Sol Ring", new_name="Arcane Signet",
                       why="Two mana rock, on colour.")
    assert "# A deck file with comments that must survive." in out
    assert "# The 99 follows." in out
    assert "\n  mulligan: Keep two to five lands.\n\n# The 99 follows." in out


def test_untouched_cards_are_byte_identical():
    out = replace_card(DECK, old_name="Primeval Titan", new_name="Regal Force",
                       why="Draws a fistful in a green board.")
    for line in ("  - name: Swamp", "    qty: 98", "    why: Black mana.",
                 "  - name: Sol Ring", "    why: Two mana for one."):
        assert line in out, line


# ------------------------------------------------------------- the why block

def test_a_folded_why_is_replaced_whole():
    """The old rationale's continuation lines must go with it -- otherwise two
    cards' reasoning ends up stacked under one name."""
    out = replace_card(DECK, old_name="Primeval Titan", new_name="Regal Force",
                       why="Short reason.")
    assert "6/6 trample fetching" not in out
    assert "Ramp and threat in one card." not in out
    parsed = yaml.safe_load(out)
    assert parsed["cards"][1] == {"name": "Regal Force", "category": "ramp",
                                  "why": "Short reason."}


def test_a_single_line_why_can_become_a_long_one():
    long_why = ("A rationale long enough that it has to wrap onto more than one "
                "line, which is the case a naive single-line writer gets wrong "
                "and produces a file that no longer parses.")
    out = replace_card(DECK, old_name="Sol Ring", new_name="Mana Crypt",
                       why=long_why)
    assert yaml.safe_load(out)["cards"][2]["why"] == long_why


def test_a_why_containing_yaml_punctuation_still_parses():
    """`why: text: with a colon` is not valid YAML. Quoting is delegated to the
    dumper rather than hand-rolled, and this is what proves it."""
    for why in ("Ratio: two lands per cast.", "> not a folded block",
                "#1 in the deck", "ends with a space "):
        out = replace_card(DECK, old_name="Sol Ring", new_name="Mox Diamond",
                           why=why)
        assert yaml.safe_load(out)["cards"][2]["why"] == why


# ---------------------------------------------------------------- refusals

def test_an_unknown_card_is_refused():
    with pytest.raises(EditFailed, match="no card entry"):
        replace_card(DECK, old_name="Black Lotus", new_name="Sol Ring", why="x")


def test_an_empty_why_is_refused():
    """Rule 4, enforced where the edit happens. A machine-written rationale is
    exactly the empty justification the rule exists to prevent, so the tool
    declines to invent one rather than writing a placeholder."""
    with pytest.raises(EditFailed, match="needs a `why`"):
        replace_card(DECK, old_name="Sol Ring", new_name="Mana Crypt", why="   ")


def test_the_file_is_unchanged_when_an_edit_is_refused():
    with pytest.raises(EditFailed):
        replace_card(DECK, old_name="Nonexistent", new_name="X", why="y")
    # `replace_card` is pure -- it returns text rather than writing -- so the
    # caller's copy cannot have been touched. Pinned because the write path
    # depends on it.
    assert "Primeval Titan" in DECK


# ------------------------------------------------------------ card identity

def test_overrides_belonging_to_the_old_card_are_dropped():
    """`mana_cost` and `scryfall_id` are overrides for a specific card. Leaving
    them attached to a different one is how a deck ends up simulating the wrong
    cost."""
    deck = DECK.replace(
        "  - name: Sol Ring\n    category: ramp\n    why: Two mana for one.\n",
        "  - name: Sol Ring\n    category: ramp\n    mana_cost: '{1}'\n"
        "    scryfall_id: abc-123\n    why: Two mana for one.\n")
    out = replace_card(deck, old_name="Sol Ring", new_name="Arcane Signet",
                       why="Two mana rock.")
    entry = yaml.safe_load(out)["cards"][2]
    assert entry == {"name": "Arcane Signet", "category": "ramp",
                     "why": "Two mana rock."}


def test_category_is_kept_by_default_and_can_be_moved():
    kept = replace_card(DECK, old_name="Primeval Titan", new_name="Regal Force",
                        why="Card draw.")
    assert yaml.safe_load(kept)["cards"][1]["category"] == "ramp"

    moved = replace_card(DECK, old_name="Primeval Titan", new_name="Regal Force",
                         why="Card draw.", category="card-advantage")
    assert yaml.safe_load(moved)["cards"][1]["category"] == "card-advantage"


def test_the_swap_board_is_editable_too():
    out = replace_card(DECK, old_name="Reliquary Tower",
                       new_name="Thought Vessel", why="Rock with the same text.")
    assert yaml.safe_load(out)["swap_board"][0]["name"] == "Thought Vessel"
    assert yaml.safe_load(out)["cards"][0]["name"] == "Swamp"


def test_quantities_survive_a_swap():
    out = replace_card(DECK, old_name="Swamp", new_name="Mountain",
                       why="Red mana instead.")
    assert yaml.safe_load(out)["cards"][0]["qty"] == 98


# ---------------------------------------------------------------- add_card

def test_adding_a_card_is_a_pure_insertion():
    out = add_card(DECK, name="Arcane Signet", category="ramp",
                   why="Two mana rock, on colour.")
    lines = list(changed(DECK, out))
    assert all(line.startswith("+") for line in lines), lines
    assert len(lines) == 3, lines
    assert yaml.safe_load(out)["cards"][3] == {
        "name": "Arcane Signet", "category": "ramp",
        "why": "Two mana rock, on colour."}


def test_a_card_lands_beside_the_cards_it_belongs_with():
    """Not at the end of the list. The deck files are grouped by category under
    section banners, so appending a land to the bottom files it under whatever
    the last banner happens to say."""
    out = add_card(DECK, name="Bojuka Bog", category="land", why="Yard hate.")
    names = [c["name"] for c in yaml.safe_load(out)["cards"]]
    assert names == ["Swamp", "Bojuka Bog", "Primeval Titan", "Sol Ring"]


def test_a_new_category_goes_to_the_end():
    out = add_card(DECK, name="Swords to Plowshares", category="interaction",
                   why="One mana, exiles anything.")
    names = [c["name"] for c in yaml.safe_load(out)["cards"]]
    assert names[-1] == "Swords to Plowshares"


def test_a_card_added_under_a_banner_stays_under_it():
    out = add_card(BANNERED, name="Ancient Tomb", category="land",
                   why="Two colourless, two life.")
    body = out.split("cards:")[1]
    lands, ramp = body.split("RAMP")
    assert "Ancient Tomb" in lands and "Ancient Tomb" not in ramp
    # And the banner still has its blank line above it.
    assert "\n\n  # ---" in body


def test_adding_a_card_that_is_already_there_is_refused():
    for name in ("Sol Ring", "sol ring", "Reliquary Tower"):
        with pytest.raises(EditFailed, match="already in"):
            add_card(DECK, name=name, category="ramp", why="A reason.")


def test_adding_the_commander_to_the_99_is_refused():
    with pytest.raises(EditFailed, match="is the commander"):
        add_card(DECK, name="Gyome, Master Chef", category="engine",
                 why="A reason.")


def test_a_card_can_be_added_to_the_swap_board():
    out = add_card(DECK, name="Thought Vessel", category="ramp",
                   why="On the bubble.", list_key="swap_board")
    parsed = yaml.safe_load(out)
    assert [c["name"] for c in parsed["swap_board"]] == \
        ["Reliquary Tower", "Thought Vessel"]
    assert len(parsed["cards"]) == 3


def test_a_quantity_is_written_only_when_it_is_not_one():
    plain = add_card(DECK, name="Bojuka Bog", category="land", why="Yard hate.")
    assert "qty" not in yaml.safe_load(plain)["cards"][1]
    many = add_card(DECK, name="Wastes", category="land", why="Colourless.", qty=4)
    assert yaml.safe_load(many)["cards"][1]["qty"] == 4


# --------------------------------------------------------- add_card and rule 4

def test_a_curated_deck_will_not_take_a_card_with_no_rationale():
    """Rule 4 at the point a card enters the deck. The tool declines to invent
    one rather than writing a placeholder -- ADR 12 rule 3."""
    with pytest.raises(EditFailed, match="needs a `why`"):
        add_card(DECK, name="Bojuka Bog", category="land", why="   ")


def test_a_draft_may_take_a_card_that_still_owes_its_rationale():
    """The one bend, and it does not bend the rule: a draft is honestly
    incomplete and counts what it owes (ADR 13). The blank `why:` is written
    into the file, which is where the outstanding work has to be visible."""
    draft = DECK.replace("bracket: 4", "bracket: 4\nstage: draft")
    out = add_card(draft, name="Bojuka Bog", category="land")
    assert yaml.safe_load(out)["cards"][1] == {
        "name": "Bojuka Bog", "category": "land", "why": ""}
    assert "    why: ''" in out


# ------------------------------------------------------------- remove_card

def test_removing_a_card_takes_only_its_own_lines():
    out = remove_card(DECK, name="Primeval Titan")
    assert "Primeval Titan" not in out
    assert "6/6 trample fetching" not in out
    assert [c["name"] for c in yaml.safe_load(out)["cards"]] == ["Swamp", "Sol Ring"]
    # Its neighbours, byte for byte.
    assert "  - name: Swamp\n    category: land\n    qty: 98\n" in out
    assert "  - name: Sol Ring\n    category: ramp\n" in out


def test_removing_a_card_leaves_the_next_section_banner_alone():
    """Dryad Arbor is the last land, and the RAMP banner sits in its line span
    purely because that is where the next entry starts. Taking the banner with
    the card is exactly the collateral damage ADR 12 forbids."""
    out = remove_card(BANNERED, name="Dryad Arbor")
    assert "Dryad Arbor" not in out
    assert "RAMP" in out
    assert "\n\n  # ---" in out, "the blank line above the banner went with it"
    assert yaml.safe_load(out)["cards"] == [
        {"name": "Forest", "category": "land", "qty": 30, "why": "Green mana."},
        {"name": "Sol Ring", "category": "ramp", "why": "Two mana for one."},
    ]


def test_removing_a_card_keeps_the_spacing_even():
    """An entry owns the blank line after it, so removing one does not leave a
    double gap behind."""
    out = remove_card(DECK, name="Sol Ring")
    assert "\n\n\n" not in out


def test_comments_survive_a_removal():
    out = remove_card(DECK, name="Swamp")
    assert "# A deck file with comments that must survive." in out
    assert "# The 99 follows." in out


def test_removing_an_unknown_card_is_refused():
    with pytest.raises(EditFailed, match="no card entry"):
        remove_card(DECK, name="Black Lotus")


def test_a_swap_board_card_can_be_removed():
    out = remove_card(DECK, name="Reliquary Tower")
    assert yaml.safe_load(out)["swap_board"] == []
    assert len(yaml.safe_load(out)["cards"]) == 3


def test_emptying_a_list_leaves_something_a_deck_can_still_load():
    """A block key with nothing under it parses to None rather than to an empty
    list, and `Deck.from_text` iterates it. Caught by the verification, which is
    what it is for."""
    from mtglab.decks.model import Deck

    out = remove_card(DECK, name="Reliquary Tower")
    assert "swap_board: []" in out
    assert Deck.from_text(out).swap_board == []


def test_an_emptied_list_can_be_filled_again():
    emptied = remove_card(DECK, name="Reliquary Tower")
    out = add_card(emptied, name="Thought Vessel", category="ramp",
                   why="Back on the bubble.", list_key="swap_board")
    parsed = yaml.safe_load(out)
    assert [c["name"] for c in parsed["swap_board"]] == ["Thought Vessel"]
    assert len(parsed["cards"]) == 3


# ---------------------------------------------------------- set_card_field

def test_a_rationale_can_be_written_without_replacing_the_card():
    """The write path behind the rationale editor. Import produces 99 cards
    with no `why`, and before this the only way to fill one in was a text
    editor."""
    out = set_card_field(DECK, name="Sol Ring", field="why",
                         value="Two mana for one, and it always has been.")
    parsed = yaml.safe_load(out)["cards"][2]
    assert parsed == {"name": "Sol Ring", "category": "ramp",
                      "why": "Two mana for one, and it always has been."}
    assert changed_lines(DECK, out) == 2


def test_replacing_a_folded_rationale_takes_the_whole_block():
    out = set_card_field(DECK, name="Primeval Titan", field="why",
                         value="Short reason.")
    assert "6/6 trample fetching" not in out
    assert "Ramp and threat in one card." not in out
    assert yaml.safe_load(out)["cards"][1]["why"] == "Short reason."


def test_a_category_and_a_quantity_can_be_changed():
    moved = set_card_field(DECK, name="Primeval Titan", field="category",
                           value="threat")
    assert yaml.safe_load(moved)["cards"][1]["category"] == "threat"
    assert yaml.safe_load(moved)["cards"][1]["why"].startswith("6/6 trample")

    fewer = set_card_field(DECK, name="Swamp", field="qty", value=30)
    assert yaml.safe_load(fewer)["cards"][0]["qty"] == 30


def test_a_field_the_card_does_not_have_yet_is_appended():
    deck = DECK.replace("  - name: Sol Ring\n    category: ramp\n"
                        "    why: Two mana for one.\n",
                        "  - name: Sol Ring\n    category: ramp\n")
    out = set_card_field(deck, name="Sol Ring", field="why", value="Fast mana.")
    assert yaml.safe_load(out)["cards"][2]["why"] == "Fast mana."


def test_only_a_short_list_of_fields_can_be_set():
    """`name` belongs to `replace_card`, which also drops the overrides that
    identify the outgoing card. Setting it here would leave them attached."""
    for field in ("name", "scryfall_id", "mana_cost", "tags", "slug"):
        with pytest.raises(EditFailed, match="not settable"):
            set_card_field(DECK, name="Sol Ring", field=field, value="x")


def test_a_rationale_cannot_be_blanked_on_a_curated_deck():
    with pytest.raises(EditFailed, match="needs a `why`"):
        set_card_field(DECK, name="Sol Ring", field="why", value="  ")


def test_a_draft_may_blank_a_rationale():
    draft = DECK.replace("bracket: 4", "bracket: 4\nstage: draft")
    out = set_card_field(draft, name="Sol Ring", field="why", value="")
    assert yaml.safe_load(out)["cards"][2]["why"] == ""


def test_a_quantity_below_one_is_refused():
    for qty in (0, -3):
        with pytest.raises(EditFailed, match="at least 1"):
            set_card_field(DECK, name="Swamp", field="qty", value=qty)
    with pytest.raises(EditFailed, match="whole number"):
        set_card_field(DECK, name="Swamp", field="qty", value="lots")


# ---------------------------------------------------------- set_deck_field
#
# Promotion is the operation that closes the import lifecycle, and it is the
# only edit whose refusal depends on the state of every card rather than one.

def test_a_draft_can_be_promoted_once_every_card_is_justified():
    draft = DECK.replace("bracket: 4", "bracket: 4\nstage: draft")
    out = set_deck_field(draft, field="stage", value="curated")
    assert yaml.safe_load(out)["stage"] == "curated"
    assert changed_lines(draft, out) == 2


def test_promotion_is_refused_while_any_card_is_blank():
    """The gate would catch this anyway -- a curated deck reports one
    `missing-rationale` per card. Refusing here means the deck is never written
    into a state its author has to undo."""
    draft = DECK.replace("bracket: 4", "bracket: 4\nstage: draft")
    draft = set_card_field(draft, name="Sol Ring", field="why", value="")
    with pytest.raises(EditFailed, match="still have no `why`") as caught:
        set_deck_field(draft, field="stage", value="curated")
    # And it names them, so the refusal is a to-do list rather than a wall.
    assert "Sol Ring" in str(caught.value)


def test_demoting_to_draft_is_never_blocked():
    """Only promotion has a precondition. Going the other way is admitting the
    deck is not finished, which is always allowed."""
    out = set_deck_field(DECK, field="stage", value="draft")
    assert yaml.safe_load(out)["stage"] == "draft"


def test_a_missing_key_is_inserted_where_the_dumper_would_put_it():
    """`stage` is absent from every deck written before ADR 13. Appending it to
    the bottom of the file would be legal YAML and unlike every deck here."""
    assert "stage:" not in DECK
    out = set_deck_field(DECK, field="stage", value="draft")
    assert yaml.safe_load(out)["stage"] == "draft"
    assert out.index("stage:") < out.index("commander:")
    assert changed_lines(DECK, out) == 1


def test_a_trailing_comment_on_the_line_survives():
    """`status: built  # built: the cards are sleeved up` -- the comment is the
    author's note about the vocabulary, not about the value."""
    deck = DECK.replace("name: Mini Deck",
                        "name: Mini Deck\nstatus: built  # the cards are sleeved up")
    out = set_deck_field(deck, field="status", value="theoretical")
    assert "status: theoretical  # the cards are sleeved up" in out
    assert yaml.safe_load(out)["status"] == "theoretical"


def test_only_the_decks_own_scalars_are_settable():
    for field in ("name", "slug", "commander", "strategy", "notes", "cards"):
        with pytest.raises(EditFailed, match="not a settable deck field"):
            set_deck_field(DECK, field=field, value="x")


def test_a_stage_or_status_outside_the_vocabulary_is_refused():
    with pytest.raises(EditFailed, match="stage must be one of"):
        set_deck_field(DECK, field="stage", value="drafted")
    with pytest.raises(EditFailed, match="status must be one of"):
        set_deck_field(DECK, field="status", value="sleeved")


def test_a_bracket_is_a_number_in_range():
    assert yaml.safe_load(set_deck_field(DECK, field="bracket", value=5))["bracket"] == 5
    with pytest.raises(EditFailed, match="runs from 1 to 5"):
        set_deck_field(DECK, field="bracket", value=9)
    with pytest.raises(EditFailed, match="must be a number"):
        set_deck_field(DECK, field="bracket", value="four")


def test_promotion_leaves_every_card_and_note_alone():
    draft = DECK.replace("bracket: 4", "bracket: 4\nstage: draft")
    out = set_deck_field(draft, field="stage", value="curated")
    before, after = yaml.safe_load(draft), yaml.safe_load(out)
    assert before["cards"] == after["cards"]
    assert before["notes"] == after["notes"]
    assert "# A deck file with comments that must survive." in out


# ---------------------------------------------------------------- set_note

def test_an_existing_note_is_replaced_in_place():
    out = set_note(DECK, key="mulligan", value="Keep any two-lander with ramp.")
    parsed = yaml.safe_load(out)
    assert parsed["notes"]["mulligan"] == "Keep any two-lander with ramp."
    # The other note is untouched, folding and all.
    assert parsed["notes"]["gameplan"].startswith("Ramp on one through three")
    assert "  gameplan: >\n" in out


def test_a_new_note_joins_the_existing_block():
    out = set_note(DECK, key="pitfalls", value="Ygra turns your creatures into "
                   "artifacts, so your own Vandalblast becomes a one-sided wrath.")
    parsed = yaml.safe_load(out)
    assert set(parsed["notes"]) == {"gameplan", "mulligan", "pitfalls"}
    assert "# The 99 follows." in out


def test_a_notes_block_is_created_when_the_deck_has_none():
    bare = DECK.replace(
        "notes:\n  gameplan: >\n    Ramp on one through three, land the commander,"
        " then assemble any outlet\n    plus any payoff.\n"
        "  mulligan: Keep two to five lands.\n\n", "")
    assert "notes:" not in bare
    out = set_note(bare, key="gameplan", value="Ramp, commander, payoff.")
    assert yaml.safe_load(out)["notes"] == {"gameplan": "Ramp, commander, payoff."}
    # Where `Deck.dump` would put it: above the cards, not appended to the file.
    assert out.index("notes:") < out.index("cards:")
    assert len(yaml.safe_load(out)["cards"]) == 3


def test_long_note_prose_is_folded_the_way_the_decks_are_written():
    prose = ("Keep two to five lands. That is the whole rule, derived from a "
             "Tier 1 sweep at twenty thousand games per policy: demanding a "
             "third mana piece on top of the land count pushes the mulligan "
             "rate up and buys almost nothing.")
    out = set_note(DECK, key="mulligan", value=prose)
    assert yaml.safe_load(out)["notes"]["mulligan"] == prose
    assert "  mulligan: >-\n" in out
    assert max(len(line) for line in out.split("\n")) <= 82


def test_an_empty_note_is_refused():
    with pytest.raises(EditFailed, match="needs text"):
        set_note(DECK, key="mulligan", value="   ")
    with pytest.raises(EditFailed, match="needs a key"):
        set_note(DECK, key="  ", value="Something.")


# ------------------------------------------------------- the real deck files

def real_decks() -> list[Path]:
    root = Path(__file__).resolve().parents[1] / "decks"
    # Decks with no cards are skipped, not an oversight. `decks/` stopped being
    # a fixed set of hand-written files when the app could create a deck, and a
    # brand-new deck is legitimately empty (ADR 13) -- there is no "first card"
    # to replace and no "last card" to remove, so these operations have nothing
    # to be small about. The point of this module is the *hand-written* files
    # with their folded scalars, and an empty deck has none.
    return [p for p in sorted(root.glob("*/deck.yaml"))
            if not p.parent.name.startswith("_")
            and (yaml.safe_load(p.read_text(encoding="utf-8")) or {}).get("cards")]


def test_every_curated_deck_can_be_edited_without_collateral_damage():
    """The synthetic fixture above is tidy. The real files are hand-written,
    with folded scalars wrapped at several widths -- which is the reason this
    module exists instead of a load-and-dump."""
    for path in real_decks():
        text = path.read_text(encoding="utf-8")
        first = yaml.safe_load(text)["cards"][0]["name"]
        out = replace_card(text, old_name=first, new_name="Island",
                           why="A replacement, for test purposes only.")
        assert changed_lines(text, out) <= 12, path.parent.name
        assert yaml.safe_load(out)["cards"][0]["name"] == "Island"
        # And the file on disk is untouched, because this returns text.
        assert path.read_text(encoding="utf-8") == text


def test_every_operation_is_small_on_every_real_deck():
    """Each operation, on each hand-written file, against the diff budget that
    makes `swaps.md` readable. Adding a card is a pure insertion; removing one
    is a pure deletion; setting a field touches that field's lines and no
    others."""
    for path in real_decks():
        text = path.read_text(encoding="utf-8")
        slug = path.parent.name
        parsed = yaml.safe_load(text)
        victim = parsed["cards"][-1]["name"]

        added = add_card(text, name="Karakas", category="land",
                         why="A test insertion.")
        lines = changed(text, added)
        assert all(line.startswith("+") for line in lines), slug
        assert len(lines) == 3, slug

        removed = remove_card(text, name=victim)
        lines = changed(text, removed)
        assert all(line.startswith("-") for line in lines), slug

        field = set_card_field(text, name=victim, field="category",
                               value="utility")
        assert changed_lines(text, field) == 2, slug

        note = set_note(text, key="test_note", value="A test note.")
        lines = changed(text, note)
        assert all(line.startswith("+") for line in lines), slug

        assert path.read_text(encoding="utf-8") == text


def test_operations_compose_in_memory():
    """ADR 12 rule 4: operations are pure, so a refactor of several cards is
    several calls and one write. There is no partially-applied edit to recover
    from because nothing is applied until the caller writes."""
    text = DECK
    text = add_card(text, name="Bojuka Bog", category="land", why="Yard hate.")
    text = remove_card(text, name="Primeval Titan")
    text = set_card_field(text, name="Sol Ring", field="category", value="utility")
    text = set_note(text, key="mulligan", value="Keep two lands and a rock.")

    parsed = yaml.safe_load(text)
    assert [c["name"] for c in parsed["cards"]] == ["Swamp", "Bojuka Bog", "Sol Ring"]
    assert parsed["cards"][2]["category"] == "utility"
    assert parsed["notes"]["mulligan"] == "Keep two lands and a rock."
    assert parsed["notes"]["gameplan"].startswith("Ramp on one through three")
    # The original is untouched: these functions never wrote anything.
    assert "Primeval Titan" in DECK


def test_a_deck_the_editor_cannot_read_is_refused_rather_than_guessed():
    """ADR 12's last consequence: text surgery can be defeated by YAML this
    project does not use. When the parse and the text disagree about how many
    cards there are, no span can be trusted, so nothing is written."""
    flow = DECK.replace("  - name: Sol Ring\n    category: ramp\n"
                        "    why: Two mana for one.\n",
                        "  - {name: Sol Ring, category: ramp, why: Fast mana.}\n")
    assert len(yaml.safe_load(flow)["cards"]) == 3
    with pytest.raises(EditFailed, match="cannot edit safely"):
        remove_card(flow, name="Swamp")


if __name__ == "__main__":
    sys.exit(pytest.main([__file__, "-q"]))


def test_an_empty_deck_cannot_promote_itself_to_curated():
    """"Every card is justified" is vacuously true of no cards.

    The blank-`why` guard passes an empty deck, so without this it promotes
    cleanly and claims the thinking is done about a deck that has none. It was
    unreachable until the create flow could make an empty deck; it is reachable
    now.
    """
    text = (
        "slug: brand-new\n"
        "name: Brand New\n"
        "status: theoretical\n"
        "stage: draft\n"
        "commander:\n"
        "  - Gyome, Master Chef\n"
        "cards: []\n"
    )
    with pytest.raises(EditFailed, match="no cards yet"):
        set_deck_field(text, field="stage", value="curated")


# ------------------------------------------------- the refusals and the diff

def test_a_file_that_does_not_parse_is_refused_with_the_parser_error():
    with pytest.raises(EditFailed, match="does not parse"):
        remove_card("cards:\n  - name: [unclosed", name="x")


def test_a_file_that_is_not_a_mapping_is_refused():
    with pytest.raises(EditFailed, match="not a mapping"):
        remove_card("- just\n- a\n- list\n", name="x")


def test_add_refuses_the_malformed_before_touching_the_text():
    with pytest.raises(EditFailed, match="cards live in"):
        add_card(DECK, name="Sol Ring", category="ramp", why="x",
                 list_key="sideboard")
    with pytest.raises(EditFailed, match="needs a name"):
        add_card(DECK, name="  ", category="ramp", why="x")
    with pytest.raises(EditFailed, match="needs a category"):
        add_card(DECK, name="Sol Ring", category=" ", why="x")
    with pytest.raises(EditFailed, match="at least 1"):
        add_card(DECK, name="Sol Ring", category="ramp", why="x", qty=0)


def test_set_category_to_blank_is_refused():
    with pytest.raises(EditFailed, match="needs a category"):
        set_card_field(DECK, name="Sol Ring", field="category", value="  ")


def test_a_note_key_must_be_a_word():
    with pytest.raises(EditFailed, match="needs a key"):
        set_note(DECK, key=" ", value="something")
    with pytest.raises(EditFailed, match="not a usable note key"):
        set_note(DECK, key="bad key!", value="something")


def test_a_notes_block_that_is_not_a_mapping_is_refused():
    broken = DECK + "\nnotes:\n  - a list entry\n"
    with pytest.raises(EditFailed, match="`notes:` is not a mapping"):
        set_note(broken, key="mulligan", value="keep lands")


def test_removing_from_a_list_that_ends_the_file():
    """`_block_span`'s run-to-EOF branch: every curated deck has keys after
    `cards`, so only a file that ends mid-list exercises it."""
    text = ("slug: tail\nname: Tail\ncommander:\n  - Gyome, Master Chef\n"
            "cards:\n"
            "  - name: Sol Ring\n    category: ramp\n    why: Rocks.\n"
            "  - name: Swamp\n    category: land\n    why: Mana.\n")
    updated = remove_card(text, name="Sol Ring")
    assert "Sol Ring" not in updated
    assert "Swamp" in updated


def test_first_difference_names_the_earliest_divergence():
    """The sentence a refused self-check shows -- each shape of drift has its
    own wording, and the wording is the debugging surface."""
    from mtglab.decks.edit import _first_difference as diff

    assert diff({"a": 1}, {"a": 1, "b": 2}) == ".b appeared"
    assert diff({"a": 1, "b": 2}, {"a": 1}) == ".b disappeared"
    assert diff({"a": {"b": 1}}, {"a": {"b": 2}}) == \
        ".a.b is 2, expected 1"
    assert diff([1, 2], [1]) == "the list has 1 entries, expected 2"
    assert diff([1, 2], [1, 3]) == "[1] is 3, expected 2"
    assert diff("x", "y") == "the document is 'y', expected 'x'"


def test_unquote_survives_text_yaml_cannot_read():
    from mtglab.decks.edit import _unquote

    assert _unquote("plain words") == "plain words"
    # A scalar YAML refuses to parse falls back to the text as typed.
    assert _unquote("[unclosed") == "[unclosed"
    # A scalar that parses to a non-string keeps the typed spelling too:
    # `why: 42` means the text "42", not the number.
    assert _unquote("42") == "42"


def test_a_fold_that_would_not_read_back_falls_back_to_plain():
    """`_render(fold=True)` folds long prose -- but folding collapses single
    newlines, so a value it would damage is rendered unfolded instead."""
    from mtglab.decks.edit import _render

    value = "line one\nline two"
    lines = _render("why", value, 4, width=40, fold=True)
    rendered = "\n".join(line[4:] if line[:4] == "    " else line
                         for line in lines)
    assert yaml.safe_load(rendered) == {"why": value}


# ------------------------------------------------- the graveyard (ADR 27)

def test_entomb_moves_the_entry_verbatim():
    """The whole entry -- category, qty, folded why -- moves to the graveyard.

    Verbatim matters because the `why` is the user's own words: a return has
    to restore exactly what left, or the graveyard is a lossy delete wearing
    an undo's name.
    """
    after = entomb_card(DECK, name="Primeval Titan")
    doc = yaml.safe_load(after)
    assert [c["name"] for c in doc["cards"]] == ["Swamp", "Sol Ring"]
    assert len(doc["graveyard"]) == 1
    dead = doc["graveyard"][0]
    assert dead["name"] == "Primeval Titan"
    assert dead["category"] == "ramp"
    assert "fetching two lands" in dead["why"]
    # The folded scalar's own lines moved untouched -- text surgery, not a
    # re-render.
    assert "6/6 trample fetching two lands on ETB and on attack." in after


def test_the_graveyard_is_newest_first():
    once = entomb_card(DECK, name="Primeval Titan")
    twice = entomb_card(once, name="Sol Ring")
    doc = yaml.safe_load(twice)
    assert [c["name"] for c in doc["graveyard"]] == \
        ["Sol Ring", "Primeval Titan"]


def test_entomb_is_a_small_edit():
    after = entomb_card(DECK, name="Sol Ring")
    # The entry's lines plus the new `graveyard:` header, and nothing else.
    assert changed_lines(DECK, after) <= 8


def test_entomb_near_a_banner_leaves_the_banner():
    after = entomb_card(BANNERED, name="Dryad Arbor")
    assert "# ------------------------------------------------------------------- RAMP" in after
    doc = yaml.safe_load(after)
    assert [c["name"] for c in doc["cards"]] == ["Forest", "Sol Ring"]
    assert doc["graveyard"][0]["name"] == "Dryad Arbor"


def test_a_swap_board_card_has_no_graveyard():
    with pytest.raises(EditFailed, match="swap board"):
        entomb_card(DECK, name="Reliquary Tower")


def test_return_restores_the_entry_next_to_its_category():
    buried = entomb_card(DECK, name="Sol Ring")
    back = return_card(buried, name="Sol Ring")
    doc = yaml.safe_load(back)
    # Anchored after the last ramp card, exactly where `add_card` files one.
    assert [c["name"] for c in doc["cards"]] == \
        ["Swamp", "Primeval Titan", "Sol Ring"]
    assert doc["cards"][2]["why"] == "Two mana for one."
    # The emptied graveyard is gone, not left as `graveyard: []`.
    assert "graveyard" not in doc
    assert "graveyard" not in back


def test_a_round_trip_is_byte_identical():
    """Entomb then return puts the file back exactly as it was.

    Stronger than the doc-level checks above: if either half re-rendered,
    reflowed or reordered anything, this is where it shows.
    """
    buried = entomb_card(DECK, name="Sol Ring")
    assert return_card(buried, name="Sol Ring") == DECK


def test_return_refuses_a_card_that_is_not_buried():
    with pytest.raises(EditFailed, match="not in the graveyard"):
        return_card(DECK, name="Sol Ring")


def test_return_refuses_a_duplicate_in_the_99():
    buried = entomb_card(DECK, name="Sol Ring")
    # Somebody re-added the card while its ghost sat in the graveyard.
    readded = buried.replace("graveyard:", "cards2_placeholder:")  # keep ghost
    readded = add_card(readded, name="Sol Ring", category="ramp",
                       why="Fresh copy.")
    readded = readded.replace("cards2_placeholder:", "graveyard:")
    with pytest.raises(EditFailed, match="already in the deck"):
        return_card(readded, name="Sol Ring")


def test_exile_is_the_permanent_half():
    buried = entomb_card(DECK, name="Sol Ring")
    gone = exile_card(buried, name="Sol Ring")
    doc = yaml.safe_load(gone)
    assert "graveyard" not in doc
    assert all(c["name"] != "Sol Ring" for c in doc["cards"])
    with pytest.raises(EditFailed, match="not in the graveyard"):
        exile_card(DECK, name="Sol Ring")


def test_exile_keeps_the_rest_of_the_graveyard():
    once = entomb_card(DECK, name="Primeval Titan")
    twice = entomb_card(once, name="Sol Ring")
    after = exile_card(twice, name="Sol Ring")
    doc = yaml.safe_load(after)
    assert [c["name"] for c in doc["graveyard"]] == ["Primeval Titan"]


def test_add_refuses_a_card_waiting_in_the_graveyard():
    """One copy, one place. A fresh add beside a graveyard ghost would show
    the card in two places with two rationales."""
    buried = entomb_card(DECK, name="Sol Ring")
    with pytest.raises(EditFailed, match="graveyard"):
        add_card(buried, name="Sol Ring", category="ramp", why="Again.")


def test_entombing_the_last_card_leaves_a_readable_file():
    text = DECK
    for name in ("Swamp", "Primeval Titan", "Sol Ring"):
        text = entomb_card(text, name=name)
    doc = yaml.safe_load(text)
    assert doc["cards"] == []
    assert [c["name"] for c in doc["graveyard"]] == \
        ["Sol Ring", "Primeval Titan", "Swamp"]
    # And the way back works from the emptied state.
    back = return_card(text, name="Swamp")
    assert yaml.safe_load(back)["cards"][0]["name"] == "Swamp"
