"""The decklist grammar.

There is no decklist standard, so the parser's contract is "the union of what
the exports people actually paste", and these pin each dialect against a real
sample of it. The module is pure text -> structure, which is why every one of
these runs without a card pool, a filesystem or a database.

The refusals matter as much as the successes: a line the parser cannot read has
to come back with its line number, because the alternative is a deck that is
silently three cards short.
"""

import sys
import time
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.decks.decklist import MAX_LINE, parse

MOXFIELD = """\
1 Arahbo, Roar of the World (C17) 27 *CMDR*
1 Kaheera, the Orphanguard (IKO) 197
1 Sol Ring (LTC) 284
1 Branchloft Pathway // Boulderloft Pathway (ZNR) 258
36 Forest (UNF) 235 *F*
"""

ARCHIDEKT = """\
1x Atla Palani, Nest Tender (M20) 191 [Commander{top}]
1x Sol Ring (C21) 263 [Ramp]
1x Cultivate (M21) 177 [Ramp{noDeck}]
"""

ARENA = """\
Deck
1 Gyome, Master Chef (CLB) 265
30 Swamp (UNF) 239

Sideboard
1 Reliquary Tower (M19) 254
"""

TAPPEDOUT = """\
Creatures (2)
1x Goreclaw, Terror of Qal Sisma
1x Craterhoof Behemoth

Lands (1)
1x Ancient Tomb
"""


# -------------------------------------------------------------- the dialects

def test_moxfield_export():
    result = parse(MOXFIELD)
    assert result.commander == ["Arahbo, Roar of the World"]
    names = [(c.name, c.qty) for c in result.section("deck")]
    assert names == [
        ("Kaheera, the Orphanguard", 1),
        ("Sol Ring", 1),
        # Double-faced names keep their separator: the `//` only means "comment"
        # at the start of a line.
        ("Branchloft Pathway // Boulderloft Pathway", 1),
        ("Forest", 36),
    ]
    assert not result.unreadable


def test_archidekt_categories_are_dropped_but_its_commander_label_is_not():
    """ADR 13 leaves every category but land to a human, so `[Ramp]` is
    discarded rather than trusted. `[Commander{top}]` is not a category; it is
    the exporter stating which card is the commander."""
    result = parse(ARCHIDEKT)
    assert result.commander == ["Atla Palani, Nest Tender"]
    assert [c.name for c in result.section("deck")] == ["Sol Ring", "Cultivate"]


def test_arena_sections_and_a_sideboard_that_becomes_the_swap_board():
    result = parse(ARENA)
    assert [(c.name, c.qty) for c in result.section("deck")] == [
        ("Gyome, Master Chef", 1), ("Swamp", 30)]
    assert [c.name for c in result.section("swap_board")] == ["Reliquary Tower"]


def test_type_headings_are_headings_not_cards():
    """A `Lands (1)` heading groups the list; it does not file the cards. The
    land category comes from the pool, which is right about the double-faced
    cards a heading is wrong about."""
    result = parse(TAPPEDOUT)
    assert [c.name for c in result.section("deck")] == [
        "Goreclaw, Terror of Qal Sisma", "Craterhoof Behemoth", "Ancient Tomb"]
    assert not result.unreadable


def test_headers_tolerate_colons_counts_and_case():
    for header in ("Commander", "Commander:", "COMMANDER (1)", "Commanders"):
        result = parse(f"{header}\n1 Gyome, Master Chef\n")
        assert result.commander == ["Gyome, Master Chef"], header


def test_deckstats_comment_headers_and_leading_set_codes():
    """Deckstats writes its headers as comments and its set codes in front. If
    `//Commander` were taken for an ordinary comment the commander would land
    in the 99 -- a wrong deck rather than an obviously incomplete one."""
    result = parse("//Commander\n1 [C17] Arahbo, Roar of the World\n"
                   "//Main\n1 [LTC] Sol Ring\n")
    assert result.commander == ["Arahbo, Roar of the World"]
    assert [c.name for c in result.section("deck")] == ["Sol Ring"]


# ------------------------------------------------------------- the line shape

def test_quantity_forms():
    text = "1 Sol Ring\n1x Arcane Signet\n4 Forest\n12X Island\nMana Crypt\n"
    assert [(c.name, c.qty) for c in parse(text).cards] == [
        ("Sol Ring", 1), ("Arcane Signet", 1), ("Forest", 4),
        ("Island", 12), ("Mana Crypt", 1)]


def test_comments_and_blank_lines_are_skipped():
    text = "# a note\n\n// MTGO writes comments like this\n1 Sol Ring\n"
    result = parse(text)
    assert [c.name for c in result.cards] == ["Sol Ring"]
    assert not result.unreadable


def test_printing_annotations_are_stripped_but_parenthesised_names_survive():
    text = ("1 Sol Ring (2X2) 297 *F*\n"
            "1 Lim-Dul's Vault (ALL) 25\n"
            "1 Erase (Not the Urza's Legacy One)\n")
    assert [c.name for c in parse(text).cards] == [
        "Sol Ring", "Lim-Dul's Vault", "Erase (Not the Urza's Legacy One)"]


def test_a_four_digit_leading_number_is_part_of_the_name():
    """`1996 World Champion` is a card. 1,996 copies of `World Champion` is
    not a thing anyone has ever pasted."""
    assert [(c.name, c.qty) for c in parse("1996 World Champion\n").cards] == [
        ("1996 World Champion", 1)]


def test_a_bare_leading_number_is_read_as_a_quantity():
    """The one ambiguity the parser cannot resolve without a card pool, pinned so
    the behaviour is deliberate rather than incidental. Written the way every
    real export writes it -- `1 3 Steps Ahead` -- it is unambiguous, and the
    importer reports the unresolved name rather than guessing."""
    assert [(c.name, c.qty) for c in parse("3 Steps Ahead\n").cards] == [
        ("Steps Ahead", 3)]
    assert [(c.name, c.qty) for c in parse("1 3 Steps Ahead\n").cards] == [
        ("3 Steps Ahead", 1)]


# ------------------------------------------------------------------ refusals

def test_a_line_that_leaves_no_name_is_reported_with_its_number():
    result = parse("1 Sol Ring\n(LTC) 284\n1 Arcane Signet\n")
    assert [c.name for c in result.cards] == ["Sol Ring", "Arcane Signet"]
    assert result.unreadable == [(2, "(LTC) 284")]


def test_token_sections_are_skipped_rather_than_resolved():
    """A token is not a card in the 99. Reporting it as an unknown card would
    be true and useless."""
    result = parse("1 Sol Ring\n\nTokens\n1 Cat // Insect\n")
    assert [c.name for c in result.cards] == ["Sol Ring"]
    assert result.skipped == [(4, "Cat // Insect")]


def test_an_empty_list_parses_to_nothing():
    result = parse("")
    assert result.cards == [] and result.unreadable == []
    assert result.commander == [] and result.companion is None


# ------------------------------------------------- bounded before the patterns

def test_an_overlong_line_costs_only_itself():
    """`parse` never raises, and a pasted list is somebody's whole deck -- one
    absurd line must not cost them the other ninety-eight."""
    result = parse(f"1 Sol Ring\n1 {'a' * (MAX_LINE * 4)}\n1 Arcane Signet\n")
    assert [c.name for c in result.cards] == ["Sol Ring", "Arcane Signet"]
    assert [n for n, _ in result.unreadable] == [2]


def test_the_reported_text_is_sliced_to_the_bound():
    """Reporting the line back whole would hand the input straight to an API
    response and a log line, which is most of what the bound is for."""
    (_, reported), = parse("1 " + "a" * (MAX_LINE * 4) + "\n").unreadable
    assert len(reported) == MAX_LINE


def test_a_line_at_the_bound_still_parses():
    """The boundary itself. `>` and `>=` differ by exactly one real decklist,
    and only this test can tell them apart."""
    name = "a" * (MAX_LINE - len("1 "))
    assert [c.name for c in parse(f"1 {name}\n").cards] == [name]


def test_a_header_that_cannot_match_returns_in_no_time_at_all():
    """The one that was not merely polynomial.

    `_HEADER` used to let a lazy `[A-Za-z ]*?`, three `\\s*` runs and `\\s*$`
    compete for the same spaces, so a line that could not match explored every
    division of them -- about ten times worse per doubling, and **26 seconds
    measured on one 512-character line**, which is inside any bound a real
    decklist needs. Taken apart, it is linear.

    The budget is a hundred-thousand-fold margin over the observed time, which
    is what keeps a wall-clock assertion from being a flaky one; the failure
    this guards against is measured in seconds, not milliseconds.
    """
    line = "a" + " " * (MAX_LINE - 3) + "!"
    assert len(line) <= MAX_LINE
    start = time.perf_counter()
    parse(line + "\n")
    assert time.perf_counter() - start < 1.0


@pytest.mark.parametrize("pattern, line", [
    # Each is the shape that makes that pattern's own backtracking bite: a long
    # run the quantifier must give back one character at a time, and no way to
    # match at the end. CodeQL flagged all five, at their use sites.
    ("_MARKER", " " * (MAX_LINE * 2) + "*x"),
    ("_BRACKET", " " * (MAX_LINE * 2) + "[x"),
    ("_PRINTING", " " * (MAX_LINE * 2) + "(ab) x!"),
    # Each ends on a non-space: `rstrip` runs before the guard, so a trailing
    # whitespace run never reaches a pattern in the first place.
    ("_QTY", "1" + " " * (MAX_LINE * 2) + "x"),
    ("_HEADER", "a" + " " * (MAX_LINE * 2) + "!"),
])
def test_no_pattern_is_handed_more_than_the_bound(pattern, line):
    """One guard covers all five, which is the point of putting it in the loop
    rather than anchoring five patterns on a parser six decks depend on."""
    assert len(line) > MAX_LINE, pattern
    result = parse(line + "\n")
    assert result.cards == [], pattern
    assert [n for n, _ in result.unreadable] == [1], pattern
