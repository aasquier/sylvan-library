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

from mtglab.decks.edit import EditFailed, replace_card

DECK = """\
# A deck file with comments that must survive.
slug: mini
name: Mini Deck
commander:
  - Gyome, Master Chef
bracket: 4

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


def changed_lines(before: str, after: str) -> int:
    import difflib
    return sum(
        1 for line in difflib.unified_diff(before.splitlines(), after.splitlines(),
                                           lineterm="", n=0)
        if line[:1] in "+-" and line[:3] not in ("---", "+++")
    )


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
    assert "\nbracket: 4\n\n# The 99 follows." in out


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


# ------------------------------------------------------- the real deck files

def test_every_curated_deck_can_be_edited_without_collateral_damage():
    """The synthetic fixture above is tidy. The real files are hand-written,
    with folded scalars wrapped at several widths -- which is the reason this
    module exists instead of a load-and-dump."""
    root = Path(__file__).resolve().parents[1] / "decks"
    for path in sorted(root.glob("*/deck.yaml")):
        if path.parent.name.startswith("_"):
            continue
        text = path.read_text(encoding="utf-8")
        first = yaml.safe_load(text)["cards"][0]["name"]
        out = replace_card(text, old_name=first, new_name="Island",
                           why="A replacement, for test purposes only.")
        assert changed_lines(text, out) <= 12, path.parent.name
        assert yaml.safe_load(out)["cards"][0]["name"] == "Island"
        # And the file on disk is untouched, because this returns text.
        assert path.read_text(encoding="utf-8") == text


if __name__ == "__main__":
    sys.exit(pytest.main([__file__, "-q"]))
