"""The command line, exercised end to end against a scratch deck directory.

`cli.py` was the worst-covered module in the project at 27%, not because it is
hard to test but because `DECKS_DIR` and `DB_PATH` were module constants with
nowhere to point them. `config.use_paths()` fixed that, so these run the real
`main()` with real argv.

Exit codes matter here as much as output: `decks validate` is used as a shell
gate, and `decks build` must *refuse* to emit artifacts for an invalid deck.
Nothing here needs DuckDB -- without a corpus the gate degrades to structural
checks, which is exactly the fresh-clone path worth covering.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab import config
from mtglab.cli import main
from mtglab.decks.model import Deck

# A real 99, because the gate checks deck size and there is no flag to relax
# it -- nor should there be. Basics carry qty and are exempt from the
# singleton rule, so 98 Swamps plus one Sol Ring is the smallest honest deck.
GOOD_DECK = """\
slug: mini
name: Mini Deck
commander:
  - Gyome, Master Chef
bracket: 4
strategy: A minimal but legally sized deck used by the CLI tests.
cards:
  - name: Swamp
    category: land
    qty: 98
    why: Black mana.
  - name: Sol Ring
    category: ramp
    why: Two mana for one.
"""

# Same deck, but one card has no `why` -- the gate's oldest rule.
BAD_DECK = GOOD_DECK.replace("    why: Two mana for one.\n", "")


@pytest.fixture
def decks(tmp_path):
    """A scratch deck directory, with no corpus alongside it."""
    root = tmp_path / "decks"
    (root / "mini").mkdir(parents=True)
    (root / "mini" / "deck.yaml").write_text(GOOD_DECK, encoding="utf-8")
    (root / "_template").mkdir()
    (root / "_template" / "deck.yaml").write_text(GOOD_DECK, encoding="utf-8")
    with config.use_paths(decks_dir=root, data_dir=tmp_path / "data"):
        yield root


def tmp_listing(decks: Path, text: str) -> Path:
    """A decklist on disk, next to the scratch deck directory."""
    path = decks.parent / "listing.txt"
    path.write_text(text, encoding="utf-8")
    return path


def run(argv) -> tuple[int, str]:
    """Run the CLI, returning `(exit_code, exit_message)`.

    `sys.exit("some message")` carries the message *as* the exit code and only
    reaches stderr when it propagates to the interpreter, so catching it here
    would otherwise swallow exactly the text worth asserting on.
    """
    try:
        main(argv)
    except SystemExit as exc:
        code = exc.code
        if code is None:
            return 0, ""
        if isinstance(code, int):
            return code, ""
        return 1, str(code)
    return 0, ""


# --------------------------------------------------------------- decks list

def test_decks_list_shows_the_deck_and_hides_the_template(decks, capsys):
    code, _ = run(["decks", "list"])
    assert code == 0
    out = capsys.readouterr().out
    assert "mini" in out
    assert "_template" not in out, "scaffolding must not appear in the library"


def test_decks_list_without_a_decks_directory_exits_cleanly(tmp_path, capsys):
    with config.use_paths(decks_dir=tmp_path / "absent"):
        assert run(["decks", "list"])[0] == 1


# ----------------------------------------------------------- decks validate

def test_validate_exits_zero_for_a_structurally_sound_deck(decks, capsys):
    """No corpus here, so this is the structural path -- which must still be a
    clean exit, because a fresh clone has no data/mtg.duckdb."""
    code, _ = run(["decks", "validate", "mini"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "0 error(s)" in out
    assert "NOT checked" in out, "a skipped corpus check must say so"


def test_validate_exits_one_when_the_gate_finds_an_error(decks, capsys):
    (decks / "mini" / "deck.yaml").write_text(BAD_DECK, encoding="utf-8")
    code, _ = run(["decks", "validate", "mini"])
    out = capsys.readouterr().out
    assert code == 1, "a failing gate must be a nonzero exit for shell use"
    assert "missing-rationale" in out


def test_validate_on_an_unknown_slug_says_where_it_looked(decks, capsys):
    code, msg = run(["decks", "validate", "nope"])
    assert code == 1
    assert "no deck at" in msg


# -------------------------------------------------------------- decks build

def test_build_refuses_to_emit_artifacts_for_an_invalid_deck(decks, capsys):
    """The core promise: no generated document ever describes a deck that does
    not validate."""
    (decks / "mini" / "deck.yaml").write_text(BAD_DECK, encoding="utf-8")
    code, msg = run(["decks", "build", "mini"])
    capsys.readouterr()
    assert code == 1
    assert "refusing to generate" in msg
    assert not (decks / "mini" / "artifacts").exists(), "wrote artifacts anyway"


def test_build_force_overrides_the_refusal(decks, capsys):
    (decks / "mini" / "deck.yaml").write_text(BAD_DECK, encoding="utf-8")
    assert run(["decks", "build", "mini", "--force"])[0] == 0
    assert (decks / "mini" / "artifacts" / "moxfield.txt").exists()


def test_build_writes_the_deliverables(decks, capsys):
    assert run(["decks", "build", "mini"])[0] == 0
    out = decks / "mini" / "artifacts"
    written = {p.name for p in out.iterdir()}
    assert {"primer-quick.md", "primer-advanced.md",
            "decklist-annotated.md", "moxfield.txt"} <= written


def test_build_moxfield_export_carries_the_commander(decks):
    run(["decks", "build", "mini"])
    text = (decks / "mini" / "artifacts" / "moxfield.txt").read_text()
    assert "98 Swamp" in text, "qty must survive the export"
    assert "Gyome, Master Chef" in text


def test_build_refuses_a_draft_and_force_does_not_help(decks, capsys):
    """ADR 13: the artifacts are the shareable surface, and a primer for a deck
    nobody has reasoned about looks exactly like one for a deck somebody did.

    `--force` overrides the gate's *errors* -- things the deck got wrong. A
    draft is not wrong, it is unfinished and says so in the file, so the way
    out is to write the rationales rather than to pass a flag."""
    draft = GOOD_DECK.replace("name: Mini Deck", "name: Mini Deck\nstage: draft")
    (decks / "mini" / "deck.yaml").write_text(draft, encoding="utf-8")

    for argv in (["decks", "build", "mini"], ["decks", "build", "mini", "--force"]):
        code, msg = run(argv)
        capsys.readouterr()
        assert code == 1, argv
        assert "is a draft" in msg, argv
        assert not (decks / "mini" / "artifacts").exists(), argv


def test_build_accepts_a_draft_once_it_is_promoted(decks):
    """Promotion is mechanical, not a claim: the deck already justifies every
    card, so setting the stage is all that is left."""
    promoted = GOOD_DECK.replace("name: Mini Deck", "name: Mini Deck\nstage: curated")
    (decks / "mini" / "deck.yaml").write_text(promoted, encoding="utf-8")
    assert run(["decks", "build", "mini"])[0] == 0
    assert (decks / "mini" / "artifacts" / "moxfield.txt").exists()


def test_the_gate_will_not_accept_a_promotion_with_a_blank_rationale(decks, capsys):
    """`stage: curated` is not something you can declare your way into."""
    claimed = BAD_DECK.replace("name: Mini Deck", "name: Mini Deck\nstage: curated")
    (decks / "mini" / "deck.yaml").write_text(claimed, encoding="utf-8")
    code, _ = run(["decks", "validate", "mini"])
    assert code == 1
    assert "missing-rationale" in capsys.readouterr().out


def test_a_draft_reports_one_counted_warning_not_a_wall(decks, capsys):
    """The count is the point (ADR 13). Ninety-nine identical warnings would
    bury the one error that matters, which ADR 8 forbids."""
    draft = BAD_DECK.replace("name: Mini Deck", "name: Mini Deck\nstage: draft")
    (decks / "mini" / "deck.yaml").write_text(draft, encoding="utf-8")
    code, _ = run(["decks", "validate", "mini"])
    out = capsys.readouterr().out
    assert code == 0, "a draft's missing rationale warns; it does not block"
    assert out.count("missing-rationale") == 0
    assert "draft-incomplete" in out
    assert "1 of 2 cards still need a `why`" in out


# ------------------------------------------------------------- decks import

def test_import_without_a_corpus_refuses_rather_than_guessing(decks, capsys):
    """Every name would be unknown and no land would be filed, so the deck's
    facts would never be checked -- the one thing the gate exists to do."""
    listing = tmp_listing(decks, "1 Sol Ring\n1 Swamp\n")
    code, msg = run(["decks", "import", "new-deck", "--from", str(listing),
                     "--commander", "Gyome, Master Chef"])
    capsys.readouterr()
    assert code == 1
    assert "needs the card corpus" in msg
    assert not (decks / "new-deck").exists()


def test_import_rejects_a_slug_that_is_not_a_directory_name(decks, capsys):
    """The slug becomes a path component, so it is checked rather than
    sanitised after the fact."""
    listing = tmp_listing(decks, "1 Sol Ring\n")
    for slug in ("../escape", "Has Spaces", "trailing-"):
        code, msg = run(["decks", "import", slug, "--from", str(listing),
                         "--commander", "Gyome, Master Chef"])
        capsys.readouterr()
        assert code == 1, slug
        assert "not a usable slug" in msg, slug


def test_import_refuses_to_overwrite_an_existing_deck(decks, capsys):
    listing = tmp_listing(decks, "1 Sol Ring\n")
    code, msg = run(["decks", "import", "mini", "--from", str(listing),
                     "--commander", "Gyome, Master Chef"])
    capsys.readouterr()
    assert code == 1
    assert "already exists" in msg
    # And the deck it refused to touch is untouched.
    assert (decks / "mini" / "deck.yaml").read_text() == GOOD_DECK


def test_import_writes_a_draft_and_the_gate_catches_the_banned_card(decks, capsys):
    """The whole bargain, end to end: the facts are checked on day one, the
    thinking is counted rather than invented, and nothing is guessed."""
    pytest.importorskip("duckdb")
    import tiny_corpus

    tiny_corpus.build(config.DB_PATH)
    listing = tmp_listing(decks, tiny_corpus.DECKLIST)
    code, msg = run(["decks", "import", "gyome-x", "--from", str(listing),
                     "--name", "Gyome imported", "--bracket", "4"])
    out = capsys.readouterr().out
    assert code == 0, msg

    deck = Deck.load(decks / "gyome-x" / "deck.yaml")
    assert deck.stage == "draft"
    assert deck.status == "theoretical", "never silently claimed as owned"
    assert deck.commander == ["Gyome, Master Chef"]
    assert deck.total_cards == 99
    assert deck.land_count == 97, "only lands are categorised, and by the corpus"
    assert [c.why for c in deck.cards] == ["", "", ""], "no rationale is invented"

    assert "banned" in out and "Primeval Titan" in out
    assert "3 card(s) still need a `why`" in out


def test_import_dry_run_writes_nothing(decks, capsys):
    pytest.importorskip("duckdb")
    import tiny_corpus

    tiny_corpus.build(config.DB_PATH)
    listing = tmp_listing(decks, tiny_corpus.DECKLIST)
    code, msg = run(["decks", "import", "gyome-x", "--from", str(listing),
                     "--dry-run"])
    out = capsys.readouterr().out
    assert code == 0, msg
    assert "dry run" in out
    assert "banned" in out, "the preview runs the real gate, not an estimate"
    assert not (decks / "gyome-x").exists()


# ---------------------------------------------------------------- sim paths

# ------------------------------------------------------------- decks editing
#
# These run without a corpus, which is exactly the point for three of the four:
# removing a card, setting a field and writing a note are facts about the deck
# file rather than about Magic. Adding a card needs the corpus and says so.

def test_remove_takes_a_card_out_without_a_corpus(decks, capsys):
    code, msg = run(["decks", "remove", "mini", "--card", "Sol Ring"])
    assert code == 0, msg
    assert "Sol Ring" not in (decks / "mini" / "deck.yaml").read_text()
    assert "- Sol Ring" in capsys.readouterr().out


def test_set_writes_the_rationale_it_was_given(decks, capsys):
    """The gap import opened, closed at the terminal: a `why` can be written
    without opening an editor. The text is the argument's, verbatim."""
    code, msg = run(["decks", "set", "mini", "--card", "Sol Ring",
                     "--why", "Two mana for one, and it always has been."])
    assert code == 0, msg
    text = (decks / "mini" / "deck.yaml").read_text()
    assert "Two mana for one, and it always has been." in text


def test_set_refuses_to_blank_a_rationale_on_a_curated_deck(decks):
    code, msg = run(["decks", "set", "mini", "--card", "Sol Ring", "--why", "  "])
    assert code == 1
    assert "needs a `why`" in msg


def test_set_takes_exactly_one_field(decks):
    code, msg = run(["decks", "set", "mini", "--card", "Sol Ring"])
    assert (code, "exactly one" in msg) == (1, True)
    code, msg = run(["decks", "set", "mini", "--card", "Sol Ring",
                     "--why", "x", "--qty", "2"])
    assert (code, "exactly one" in msg) == (1, True)


def test_note_sets_deck_level_prose(decks, capsys):
    code, msg = run(["decks", "note", "mini", "--key", "mulligan",
                     "--value", "Keep any two-lander with a rock."])
    assert code == 0, msg
    text = (decks / "mini" / "deck.yaml").read_text()
    assert "mulligan" in text and "Keep any two-lander with a rock." in text


def test_note_can_read_long_prose_from_a_file(decks, tmp_path):
    """Long prose is folded across lines the way the deck files write it, so
    the value is checked after parsing rather than by searching the text."""
    import yaml

    text = ("Ramp on one through three, land the commander on four, then "
            "assemble any outlet plus any payoff.")
    prose = tmp_path / "note.txt"
    prose.write_text(text, encoding="utf-8")
    code, msg = run(["decks", "note", "mini", "--key", "gameplan",
                     "--from-file", str(prose)])
    assert code == 0, msg
    written = yaml.safe_load((decks / "mini" / "deck.yaml").read_text())
    assert written["notes"]["gameplan"] == text


def test_add_without_a_corpus_refuses_rather_than_guessing(decks):
    """Rule 1 applied to a write: a card nobody looked up is a card whose
    legality and colour identity are a guess."""
    code, msg = run(["decks", "add", "mini", "--card", "Llanowar Elves",
                     "--category", "ramp", "--why", "One mana dork."])
    assert code == 1
    assert "corpus" in msg


def test_an_unknown_category_is_refused_before_the_corpus_is_needed(decks):
    code, msg = run(["decks", "add", "mini", "--card", "Llanowar Elves",
                     "--category", "rampp", "--why", "typo"])
    assert code == 1
    assert "is not a category" in msg


def test_a_refused_edit_leaves_the_file_untouched(decks):
    before = (decks / "mini" / "deck.yaml").read_text()
    run(["decks", "remove", "mini", "--card", "Black Lotus"])
    run(["decks", "set", "mini", "--card", "Sol Ring", "--why", ""])
    run(["decks", "note", "mini", "--key", "x", "--value", ""])
    assert (decks / "mini" / "deck.yaml").read_text() == before


def test_sim_without_a_corpus_exits_with_advice_not_a_traceback(decks, capsys):
    """`compile_deck` raises CorpusRequired; the CLI turns that into a clean
    message naming the command that fixes it."""
    code, msg = run(["sim", "mana", "mini", "--games", "10"])
    assert code == 1
    assert "data refresh" in msg


def test_sim_lands_without_a_corpus_also_exits_cleanly(decks, capsys):
    code, msg = run(["sim", "lands", "mini", "30", "31", "--games", "10"])
    assert code == 1
    assert "data refresh" in msg


# ------------------------------------------------------------ argv handling

def test_no_subcommand_is_an_error_not_a_crash(capsys):
    with pytest.raises(SystemExit):
        main([])


def test_unknown_subcommand_is_rejected(capsys):
    with pytest.raises(SystemExit):
        main(["nonsense"])


@pytest.mark.parametrize("argv", [
    ["decks", "--help"], ["sim", "--help"], ["data", "--help"], ["--help"],
])
def test_help_is_available_at_every_level(argv, capsys):
    """argparse exits 0 for --help; a crash here means a broken parser tree."""
    with pytest.raises(SystemExit) as exc:
        main(argv)
    assert exc.value.code == 0


def test_load_all_decks_reads_every_deck_in_the_configured_directory(decks):
    from mtglab.cli import load_all_decks
    loaded = load_all_decks()
    assert [d.slug for d in loaded] == ["mini"]
    assert loaded[0].total_cards == 99, "qty must be counted, not entries"
