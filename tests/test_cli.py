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


# ---------------------------------------------------------------- sim paths

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
