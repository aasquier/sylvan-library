"""The command line, exercised end to end against a scratch deck directory.

`cli.py` was the worst-covered module in the project at 27%, not because it is
hard to test but because `DECKS_DIR` and `DB_PATH` were module constants with
nowhere to point them. `config.use_paths()` fixed that, so these run the real
`main()` with real argv.

Exit codes matter here as much as output: `decks validate` is used as a shell
gate, and `decks build` must *refuse* to emit artifacts for an invalid deck.
Nothing here needs DuckDB -- without a card pool the gate degrades to structural
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
    """A scratch deck directory, with no card pool alongside it."""
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
    """No card pool here, so this is the structural path -- which must still be a
    clean exit, because a fresh clone has no data/mtg.duckdb."""
    code, _ = run(["decks", "validate", "mini"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "0 error(s)" in out
    assert "NOT checked" in out, "a skipped pool check must say so"


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

def test_import_without_a_pool_refuses_rather_than_guessing(decks, capsys):
    """Every name would be unknown and no land would be filed, so the deck's
    facts would never be checked -- the one thing the gate exists to do."""
    listing = tmp_listing(decks, "1 Sol Ring\n1 Swamp\n")
    code, msg = run(["decks", "import", "new-deck", "--from", str(listing),
                     "--commander", "Gyome, Master Chef"])
    capsys.readouterr()
    assert code == 1
    assert "needs the card pool" in msg
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
    import tiny_pool

    tiny_pool.build(config.DB_PATH)
    listing = tmp_listing(decks, tiny_pool.DECKLIST)
    code, msg = run(["decks", "import", "gyome-x", "--from", str(listing),
                     "--name", "Gyome imported", "--bracket", "4"])
    out = capsys.readouterr().out
    assert code == 0, msg

    deck = Deck.load(decks / "gyome-x" / "deck.yaml")
    assert deck.stage == "draft"
    assert deck.status == "theoretical", "never silently claimed as owned"
    assert deck.commander == ["Gyome, Master Chef"]
    assert deck.total_cards == 99
    assert deck.land_count == 97, "only lands are categorised, and by the pool"
    assert [c.why for c in deck.cards] == ["", "", ""], "no rationale is invented"

    assert "banned" in out and "Primeval Titan" in out
    assert "3 card(s) still need a `why`" in out


def test_import_dry_run_writes_nothing(decks, capsys):
    import tiny_pool

    tiny_pool.build(config.DB_PATH)
    listing = tmp_listing(decks, tiny_pool.DECKLIST)
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
# These run without a card pool, which is exactly the point for three of the four:
# removing a card, setting a field and writing a note are facts about the deck
# file rather than about Magic. Adding a card needs the pool and says so.

def test_remove_takes_a_card_out_without_a_pool(decks, capsys):
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


def test_add_without_a_pool_refuses_rather_than_guessing(decks):
    """Rule 1 applied to a write: a card nobody looked up is a card whose
    legality and colour identity are a guess."""
    code, msg = run(["decks", "add", "mini", "--card", "Llanowar Elves",
                     "--category", "ramp", "--why", "One mana dork."])
    assert code == 1
    assert "pool" in msg


def test_an_unknown_category_is_refused_before_the_pool_is_needed(decks):
    code, msg = run(["decks", "add", "mini", "--card", "Llanowar Elves",
                     "--category", "rampp", "--why", "typo"])
    assert code == 1
    assert "is not a category" in msg


def test_promote_closes_the_import_lifecycle(decks, capsys):
    """`decks import` writes a draft; this is how it stops being one, without
    anybody opening a text editor."""
    import yaml

    path = decks / "mini" / "deck.yaml"
    path.write_text(GOOD_DECK.replace("bracket: 4", "bracket: 4\nstage: draft"),
                    encoding="utf-8")
    code, msg = run(["decks", "promote", "mini"])
    assert code == 0, msg
    assert yaml.safe_load(path.read_text())["stage"] == "curated"


def test_promote_is_refused_while_a_card_is_blank_and_names_it(decks):
    import yaml

    path = decks / "mini" / "deck.yaml"
    path.write_text(
        GOOD_DECK.replace("bracket: 4", "bracket: 4\nstage: draft")
                 .replace("    why: Two mana for one.\n", "    why: ''\n"),
        encoding="utf-8")
    code, msg = run(["decks", "promote", "mini"])
    assert code == 1
    assert "Sol Ring" in msg
    assert yaml.safe_load(path.read_text())["stage"] == "draft"


def test_set_changes_a_deck_field_when_no_card_is_named(decks):
    import yaml

    code, msg = run(["decks", "set", "mini", "--status", "built"])
    assert code == 0, msg
    assert yaml.safe_load((decks / "mini" / "deck.yaml").read_text())["status"] == "built"


def test_set_will_not_mix_a_card_field_with_a_deck_field(decks):
    code, msg = run(["decks", "set", "mini", "--status", "built",
                     "--card", "Sol Ring"])
    assert (code, "not a card's" in msg) == (1, True)
    code, msg = run(["decks", "set", "mini", "--why", "A reason."])
    assert (code, "name it with --card" in msg) == (1, True)


def test_a_refused_edit_leaves_the_file_untouched(decks):
    before = (decks / "mini" / "deck.yaml").read_text()
    run(["decks", "remove", "mini", "--card", "Black Lotus"])
    run(["decks", "set", "mini", "--card", "Sol Ring", "--why", ""])
    run(["decks", "note", "mini", "--key", "x", "--value", ""])
    assert (decks / "mini" / "deck.yaml").read_text() == before


def test_sim_without_a_pool_exits_with_advice_not_a_traceback(decks, capsys):
    """`compile_deck` raises PoolRequired; the CLI turns that into a clean
    message naming the command that fixes it."""
    code, msg = run(["sim", "mana", "mini", "--games", "10"])
    assert code == 1
    assert "data refresh" in msg


def test_sim_lands_without_a_pool_also_exits_cleanly(decks, capsys):
    code, msg = run(["sim", "lands", "mini", "30", "31", "--games", "10"])
    assert code == 1
    assert "data refresh" in msg


def test_sim_cache_reports_an_empty_store_without_creating_noise(decks, capsys):
    """The break-glass window on the memoised results.

    Reporting `enabled` separately matters: "no rows" and "caching is off
    because the engine could not be fingerprinted" look identical from a count
    and want opposite responses.
    """
    main(["sim", "cache"])
    out = capsys.readouterr().out
    assert "rows:    0" in out
    assert "enabled: yes" in out


def test_sim_cache_clear_empties_the_store(decks, capsys):
    from mtglab.sim import cache
    cache.put("a-key", "sim.mana", {"games": 1})
    assert cache.stats()["rows"] == 1

    main(["sim", "cache", "--clear"])
    assert "cleared 1 cached result" in capsys.readouterr().out
    assert cache.stats()["rows"] == 0


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


# -------------------------------------------------------------- deleting

def test_delete_moves_the_deck_aside_and_says_so(decks, capsys):
    """`--yes` is for scripts, and it still has to name the slug on the command
    line — there is no spelling of this that deletes a deck nobody typed."""
    main(["decks", "delete", "mini", "--yes"])

    assert not (decks / "mini").exists()
    out = capsys.readouterr().out
    assert "deleted mini" in out
    assert ".trash" in out, "it must say where the deck went"
    # And the deck really is recoverable, not merely described as such.
    trashed = list((decks / ".trash").glob("mini-*/deck.yaml"))
    assert len(trashed) == 1
    assert "Mini Deck" in trashed[0].read_text(encoding="utf-8")


def test_delete_prompts_for_the_slug_when_not_given_yes(decks, monkeypatch, capsys):
    monkeypatch.setattr("builtins.input", lambda _: "mini")
    main(["decks", "delete", "mini"])
    assert not (decks / "mini").exists()


def test_delete_takes_the_magic_word_too(decks, monkeypatch, capsys):
    """The shell and the app ask for the same word. A confirmation that is
    only spelled one way in one surface is one people learn twice."""
    monkeypatch.setattr("builtins.input", lambda _: "bury")
    main(["decks", "delete", "mini"])
    assert not (decks / "mini").exists()


def test_a_mistyped_confirmation_deletes_nothing(decks, monkeypatch, capsys):
    monkeypatch.setattr("builtins.input", lambda _: "")
    with pytest.raises(SystemExit) as exc:
        main(["decks", "delete", "mini"])
    assert exc.value.code != 0
    assert (decks / "mini" / "deck.yaml").exists(), "the deck is untouched"


def test_deleting_an_unknown_deck_exits_rather_than_tracebacks(decks):
    with pytest.raises(SystemExit) as exc:
        main(["decks", "delete", "no-such-deck", "--yes"])
    assert exc.value.code != 0


def test_load_all_decks_reads_every_deck_in_the_configured_directory(decks):
    from mtglab.cli import load_all_decks
    loaded = load_all_decks()
    assert [d.slug for d in loaded] == ["mini"]
    assert loaded[0].total_cards == 99, "qty must be counted, not entries"


# -------------------------------------------------------------- sim forge

def test_forge_without_the_distribution_exits_with_advice(decks, monkeypatch):
    """A missing 467 MB download is a setup problem, not a traceback."""
    monkeypatch.setenv("MTGLAB_FORGE_HOME", str(decks / "nowhere"))
    code, msg = run(["sim", "forge", "mini", "mini", "--check-only"])
    assert code == 1
    assert "MTGLAB_FORGE_HOME" in msg


def test_forge_check_only_names_the_cards_forge_lacks(decks, monkeypatch):
    """The pre-flight has to fail loudly and say which card, because the
    alternative -- Forge dropping it and playing on -- is silent."""
    from mtglab.sim.tier3 import run as forge
    monkeypatch.setattr(forge, "implemented_names",
                        lambda *a, **k: frozenset({"Swamp", "Gyome, Master Chef"}))
    code, msg = run(["sim", "forge", "mini", "mini", "--check-only"])
    assert code == 1
    assert "Sol Ring" in msg


# ------------------------------------------------- the commands that need a card pool
#
# `cli.py` sat at 62% because roughly a third of it -- suggest, the simulators,
# card lookup, price, the Claude surface -- opens `config.DB_PATH` on the first
# line and exits if it is missing. On a machine with no card pool those handlers
# were unreachable, so CI ran the argument parsing and none of the work.
#
# `tiny_pool` removes that. The scratch deck below is built from cards the
# fixture knows, so the same `main()` runs the same code path it runs for real.

@pytest.fixture
def pool_decks(tmp_path):
    """A scratch deck directory *with* a card pool beside it."""
    import tiny_pool

    root = tmp_path / "decks"
    (root / "mini").mkdir(parents=True)
    (root / "mini" / "deck.yaml").write_text(GOOD_DECK, encoding="utf-8")
    with config.use_paths(decks_dir=root, data_dir=tmp_path / "data"):
        tiny_pool.build(config.DB_PATH)
        yield root


def write_deck(root: Path, slug: str, deck) -> None:
    """Put a `Deck` object on disk under the scratch directory."""
    (root / slug).mkdir(parents=True, exist_ok=True)
    deck.slug = slug
    deck.dump(root / slug / "deck.yaml")


def test_validate_with_a_pool_checks_card_facts_not_just_structure(pool_decks,
                                                                     capsys):
    """Without a card pool the gate degrades to structural checks. With one it
    resolves every name, which is the half that was never exercised in CI."""
    code, _ = run(["decks", "validate", "mini"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "0 error(s)" in out


def test_validate_reports_a_banned_card_by_name(pool_decks, capsys):
    import tiny_pool
    write_deck(pool_decks, "banned", tiny_pool.mono_green_deck())
    code, _ = run(["decks", "validate", "banned"])
    out = capsys.readouterr().out
    assert code == 1, out
    assert "Primeval Titan" in out


def test_suggest_shortlists_replacements_for_what_the_gate_flagged(pool_decks,
                                                                   capsys):
    import tiny_pool
    write_deck(pool_decks, "banned", tiny_pool.mono_green_deck())
    code, _ = run(["decks", "suggest", "banned"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "Primeval Titan" in out
    # A shortlist with nothing on it is the failure mode worth catching.
    assert "Cultivator Colossus" in out or "Craterhoof" in out


def test_suggest_on_a_clean_deck_says_so_rather_than_upselling(pool_decks,
                                                              capsys):
    import tiny_pool
    write_deck(pool_decks, "clean", tiny_pool.mono_green_deck(clean=True))
    code, _ = run(["decks", "suggest", "clean"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "Primeval Titan" not in out


def test_sim_mana_runs_and_prints_its_caveat(pool_decks, capsys):
    """The command runs end to end and reports a row per turn."""
    code, _ = run(["sim", "mana", "mini", "--games", "50", "--turns", "4",
                   "--seed", "1"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "games=50" in out and "turns=4" in out
    # One row per simulated turn, which is the whole result.
    for turn in ("1", "2", "3", "4"):
        assert turn in out


def test_sim_mana_is_seeded_and_repeatable(pool_decks, capsys):
    """Same seed, same numbers -- otherwise a sweep is not a comparison."""
    run(["sim", "mana", "mini", "--games", "50", "--turns", "4", "--seed", "7"])
    first = capsys.readouterr().out
    run(["sim", "mana", "mini", "--games", "50", "--turns", "4", "--seed", "7"])
    assert capsys.readouterr().out == first


def test_sim_lands_sweeps_the_requested_range(pool_decks, capsys):
    code, _ = run(["sim", "lands", "mini", "34", "36", "--games", "40",
                   "--seed", "1"])
    out = capsys.readouterr().out
    assert code == 0, out
    for count in ("34", "35", "36"):
        assert count in out


def test_sim_needs_a_pool_and_says_which_command_fixes_it(decks):
    """The fresh-clone path: refuse rather than simulate a deck of unknowns."""
    code, msg = run(["sim", "mana", "mini", "--games", "10"])
    assert code == 1
    assert "data refresh" in msg


def test_price_deck_reports_rather_than_crashing_without_printings(pool_decks,
                                                                  capsys):
    """`tiny_pool` loads oracle rows but no printings, which is exactly the
    state a card pool refreshed with --oracle-only is in. Pricing must degrade to
    "no price" rather than raising."""
    code, _ = run(["price", "deck", "mini"])
    out = capsys.readouterr().out
    assert code in (0, 1), out


# ------------------------------------------------------ the Claude commands
#
# Both call out to Anthropic on their first line, so on any machine without a
# key they exited before doing anything and CI covered neither. The service
# layer is stubbed here: what is worth pinning is the terminal output, and in
# particular ADR 14's third boundary -- Claude's answer has to be *labelled* as
# Claude's, because the gate's output is reproducible and this is not.

def test_claude_check_reports_a_working_key(decks, monkeypatch, capsys):
    from mtglab.claude import client as claude
    monkeypatch.setattr(claude, "check", lambda: {
        "ok": True, "model": "claude-sonnet-5", "served_by": "claude-sonnet-5",
        "text": "ok", "input_tokens": 12, "output_tokens": 3})
    code, _ = run(["claude", "check"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "claude-sonnet-5" in out
    assert "12 in / 3 out" in out


def test_claude_check_exits_nonzero_when_the_key_is_dead(decks, monkeypatch,
                                                        capsys):
    """The command exists to answer "is the key live?", so the exit code has to
    carry the answer -- it gets used as a shell gate."""
    from mtglab.claude import client as claude
    monkeypatch.setattr(claude, "check", lambda: {
        "ok": False, "model": "claude-sonnet-5",
        "error": "authentication_error: invalid x-api-key"})
    code, _ = run(["claude", "check"])
    out = capsys.readouterr().out
    assert code == 1
    assert "unavailable" in out
    assert "invalid x-api-key" in out


def test_claude_check_can_list_the_tools_a_surface_may_call(decks, monkeypatch,
                                                            capsys):
    from mtglab.claude import client as claude
    monkeypatch.setattr(claude, "check", lambda: {
        "ok": True, "model": "m", "served_by": "m", "text": "ok",
        "input_tokens": 1, "output_tokens": 1})
    run(["claude", "check", "--tools"])
    out = capsys.readouterr().out
    assert "all read-only" in out
    assert "get_cards" in out and "validate_deck" in out


def _interview_report(**overrides):
    report = {
        "slug": "mini", "card": "Sol Ring", "model": "claude-sonnet-5",
        "asked": True, "reason": "",
        "stance": {"preset": "consultant",
                   "axes": [{"level": "on-request"}, {"level": "flagged"},
                            {"level": "none"}]},
        "tool_calls": [{"tool": "get_cards"}, {"tool": "get_cards"},
                       {"tool": "validate_deck"}],
        "questions": [
            {"angle": "cost", "question": "What does the second one do?",
             "fact": "mana value 1"},
            {"angle": "slot", "question": "What comes out for it?", "fact": ""},
        ],
        "questions_dropped": 0,
        "never": "Claude never writes a rationale.",
        "usage": {"input_tokens": 900, "output_tokens": 120},
    }
    report.update(overrides)
    return report


def test_claude_interview_prints_questions_and_says_whose_they_are(decks,
                                                                   monkeypatch,
                                                                   capsys):
    """Rule 4 and ADR 14's third boundary in one command: questions, no
    rationale, and a label saying this is not the gate."""
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_interview",
                        lambda **kw: _interview_report())
    code, _ = run(["claude", "interview", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "not the gate" in out
    assert "1. [cost] What does the second one do?" in out
    assert "(mana value 1)" in out
    assert "900 in / 120 out" in out


def test_the_interview_tells_you_to_write_the_why_yourself(decks, monkeypatch,
                                                           capsys):
    """The command's last word is the user's next command, and it is a `set`
    with the rationale left as `...` -- there is nothing to paste."""
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_interview",
                        lambda **kw: _interview_report())
    run(["claude", "interview", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert "mtglab decks set mini --card 'Sol Ring' --why '...'" in out
    assert "Claude never writes a rationale." in out


def test_the_interview_names_what_it_looked_up(decks, monkeypatch, capsys):
    """Rule 1's receipt: an opinion assembled from pool lookups should say
    which ones, deduplicated rather than one line per call."""
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_interview",
                        lambda **kw: _interview_report())
    run(["claude", "interview", "mini", "--card", "Sol Ring"])
    assert "looked up: get_cards, validate_deck" in capsys.readouterr().out


def test_an_interview_that_was_not_asked_says_why(decks, monkeypatch, capsys):
    """Stance `off` means no call was made. That has to read differently from
    a call that came back with nothing."""
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_interview", lambda **kw: _interview_report(
        asked=False, reason="stance is off, so no call was made",
        questions=[], tool_calls=[]))
    code, _ = run(["claude", "interview", "mini", "--card", "Sol Ring",
                   "--stance", "off"])
    out = capsys.readouterr().out
    assert code == 0, out
    assert "no call was made" in out
    assert "not the gate" not in out


def test_an_empty_answer_is_reported_rather_than_printed_as_success(decks,
                                                                    monkeypatch,
                                                                    capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_interview", lambda **kw: _interview_report(
        questions=[], reason="the model returned prose, not questions"))
    run(["claude", "interview", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert "nothing usable came back" in out


def test_dropped_answers_are_counted_out_loud(decks, monkeypatch, capsys):
    """A filtered response that silently shrank would look like the model had
    less to say, rather than like the filter doing its job."""
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_interview",
                        lambda **kw: _interview_report(questions_dropped=2))
    run(["claude", "interview", "mini", "--card", "Sol Ring"])
    assert "2 answer(s) dropped" in capsys.readouterr().out


def test_asking_about_a_card_that_is_not_in_the_deck_exits_nonzero(decks,
                                                                   monkeypatch,
                                                                   capsys):
    from mtglab.api import service
    from mtglab.claude.interview import CardNotInDeck

    def boom(**kw):
        raise CardNotInDeck("'Black Lotus' is not in mini.")
    monkeypatch.setattr(service, "claude_interview", boom)
    code, _ = run(["claude", "interview", "mini", "--card", "Black Lotus"])
    out = capsys.readouterr().out
    assert code == 1
    assert "not in mini" in out


def test_an_unavailable_claude_is_a_message_not_a_traceback(decks, monkeypatch,
                                                            capsys):
    from mtglab.api import service
    from mtglab.claude.client import ClaudeUnavailable

    def boom(**kw):
        raise ClaudeUnavailable("no ANTHROPIC_API_KEY")
    monkeypatch.setattr(service, "claude_interview", boom)
    code, _ = run(["claude", "interview", "mini", "--card", "Sol Ring"])
    assert code == 1
    assert "no ANTHROPIC_API_KEY" in capsys.readouterr().out


# ------------------------------------------------- claude: the renderers

def _argue_report(**over):
    """A faithful `service.claude_argue` payload, small enough to read.

    The keys mirror `argue._report`; a renderer test that invents its own
    shape tests the invention, so anything asserted below should be checked
    against that function when it changes.
    """
    report = {
        "slug": "mini", "card": "Sol Ring", "asked": True, "reason": "",
        "model": "claude-sonnet-5",
        "stance": {"preset": "consultant",
                   "axes": [{"level": "asks"}, {"level": "card"},
                            {"level": "none"}]},
        "tool_calls": [{"tool": "get_cards"}, {"tool": "search_cards"}],
        "charges": [
            {"strength": "strong", "ground": "deck",
             "claim": "Every other rock here costs less.",
             "fact": "why: Two mana for one."},
            {"strength": "weak", "ground": "pool",
             "claim": "Colourless ramp ignores the commander.",
             "fact": "Gyome, Master Chef: {3}{B}{G}"},
        ],
        "charges_dropped": 1,
        "alternatives": [
            {"name": "Arcane Signet", "mana_cost": "{2}",
             "type_line": "Artifact"},
        ],
        "alternatives_dropped": {"not_in_pool": ["Sol Talisman"],
                                 "off_identity": [], "banned": []},
        "never": "Claude argues one direction and never writes a why.",
        "usage": {"input_tokens": 900, "output_tokens": 210},
    }
    report.update(over)
    return report


def test_claude_argue_renders_the_case_and_both_dropped_counts(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_argue",
                        lambda **kw: _argue_report())
    code, _ = run(["claude", "argue", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert code == 0
    assert "Sol Ring — mini" in out
    assert "asked as: consultant (scope: card)" in out
    assert "looked up: get_cards, search_cards" in out
    # The header says whose voice this is -- ADR 14 boundary 3 in a terminal.
    assert "not the gate" in out
    assert "[strong/deck]" in out and "[weak/pool]" in out
    assert "Every other rock here costs less." in out
    assert "(why: Two mana for one.)" in out
    assert "Arcane Signet" in out
    # Both kinds of dropped are said out loud, each under its own reason.
    assert "1 charge(s) dropped: nothing cited." in out
    assert "dropped (not in pool): Sol Talisman" in out
    # And the empty reasons stay silent rather than printing empty lists.
    assert "off identity" not in out
    assert "mtglab decks remove mini" in out
    assert "tokens: 900 in / 210 out" in out


def test_claude_argue_at_stance_off_prints_the_reason_and_stops(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_argue", lambda **kw: _argue_report(
        asked=False, reason="stance is off: no call was made.",
        charges=[], tool_calls=[]))
    code, _ = run(["claude", "argue", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert code == 0
    assert "stance is off" in out
    assert "The case against" not in out


def test_claude_argue_says_when_nothing_usable_came_back(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_argue", lambda **kw: _argue_report(
        charges=[], charges_dropped=3, alternatives=[],
        reason="every charge came back citing nothing."))
    code, _ = run(["claude", "argue", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert code == 0
    assert "nothing usable came back" in out
    assert "3 charge(s) dropped" in out


def test_claude_argue_failure_is_a_message_not_a_traceback(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    from mtglab.claude.client import ClaudeUnavailable

    def boom(**kw):
        raise ClaudeUnavailable("no ANTHROPIC_API_KEY")
    monkeypatch.setattr(service, "claude_argue", boom)
    code, _ = run(["claude", "argue", "mini", "--card", "Sol Ring"])
    assert code == 1
    assert "no ANTHROPIC_API_KEY" in capsys.readouterr().out


def _dossier_report(**over):
    """A faithful `service.claude_dossier` payload; keys mirror the mode's."""
    section = {"prose": "A chef who feeds the table to win it.",
               "source_ids": [1]}
    report = {
        "slug": "mini", "commander": "Gyome, Master Chef",
        "cached": False, "generated_at": "2026-08-14T00:00:00+00:00",
        "model": "claude-sonnet-5", "reason": "",
        "dossier": {
            "who": dict(section),
            "archetype": {**section, "name": "Food value engine"},
            "competitors": [{"name": "Trostani Discordant",
                             "mana_cost": "{3}{G}{W}",
                             "prose": "The other token general at the table.",
                             "source_ids": [2]}],
            "rivals": {"prose": "The lore gives him no nemesis; the kitchen "
                                "is the story.",
                       "source_ids": [1]},
            "standing": dict(section),
            "sources": [
                {"id": 1, "title": "Gyome, Master Chef — EDHREC",
                 "url": "https://example.com/gyome"},
                {"id": 2, "title": "Commander tier history",
                 "url": "https://example.com/tiers"},
            ],
            "sources_dropped": 1, "competitors_dropped": 2, "searched": 54,
        },
        "never": "Card facts come from the pool; the web supplied the history.",
        "usage": {"input_tokens": 800, "output_tokens": 2100,
                  "cache_read_tokens": 57000},
    }
    report.update(over)
    return report


def test_claude_dossier_renders_passages_competitors_and_sources(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_dossier",
                        lambda **kw: _dossier_report())
    code, _ = run(["claude", "dossier", "mini"])
    out = capsys.readouterr().out
    assert code == 0
    assert "Gyome, Master Chef — mini" in out
    assert "written by claude-sonnet-5" in out
    assert "WHO" in out and "STANDING" in out
    assert "ARCHETYPE — Food value engine" in out
    assert "COMPETITORS" in out
    assert "Trostani Discordant {3}{G}{W}" in out
    # The story rivals render as their own passage, and only when there is one.
    assert "RIVALS" in out and "no nemesis" in out
    assert "[1]" in out and "https://example.com/gyome" in out
    # The counters that climb when a prompt starts inventing.
    assert "dropped: 1 cited page(s)" in out and "2 competitor(s)" in out
    assert "54 pages searched, 2 cited." in out
    assert "tokens: 800 in / 2100 out (57000 cached)" in out


def test_claude_dossier_quotes_a_stored_one_as_stored(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_dossier",
                        lambda **kw: _dossier_report(cached=True))
    run(["claude", "dossier", "mini"])
    out = capsys.readouterr().out
    # A cached dossier is dated, not re-attributed, and costs nothing.
    assert "stored 2026-08-14" in out
    assert "no tokens: served from the store" in out
    assert "tokens: 800 in" not in out


def test_claude_dossier_refused_prints_the_reason(decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "claude_dossier", lambda **kw: _dossier_report(
        dossier=None, reason="no source survived the check; refused."))
    code, _ = run(["claude", "dossier", "mini"])
    assert code == 0
    assert "no source survived" in capsys.readouterr().out


def test_claude_dossier_list_and_clear_and_the_missing_slug(
        decks, monkeypatch, capsys):
    from mtglab.claude import dossier as dossier_mod

    monkeypatch.setattr(dossier_mod, "stored", list)
    run(["claude", "dossier", "--list"])
    assert "no dossiers stored yet" in capsys.readouterr().out

    monkeypatch.setattr(dossier_mod, "stored", lambda: [
        {"commander": "Gyome, Master Chef",
         "created_at": "2026-08-14T00:00:00+00:00"}])
    run(["claude", "dossier", "--list"])
    out = capsys.readouterr().out
    assert "1 stored:" in out
    assert "Gyome, Master Chef" in out and "2026-08-14" in out

    monkeypatch.setattr(dossier_mod, "clear", lambda: 6)
    run(["claude", "dossier", "--clear"])
    assert "cleared 6 dossier(s)" in capsys.readouterr().out

    code, _ = run(["claude", "dossier"])
    assert code == 1
    assert "a deck slug is required" in capsys.readouterr().out


def test_claude_dossier_failure_is_a_message_not_a_traceback(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    from mtglab.claude import dossier as dossier_mod

    def boom(**kw):
        raise dossier_mod.NoCommander("mini has no commander to write about")
    monkeypatch.setattr(service, "claude_dossier", boom)
    code, _ = run(["claude", "dossier", "mini"])
    assert code == 1
    assert "no commander" in capsys.readouterr().out


def _research_report(**over):
    """A faithful `research.ask` payload; keys mirror `research._report`."""
    report = {
        "question": "Is Nadu banned in Commander?",
        "model": "claude-sonnet-5", "reason": "",
        "stance": {"preset": "second-opinion",
                   "axes": [{"level": "asks"}, {"level": "question"},
                            {"level": "none"}]},
        "research": {
            "confidence": "settled",
            "answer": "Yes -- banned in the September 2024 update.",
            "findings": [
                {"claim": "The ban announcement names combo speed.",
                 "source_ids": [1]},
            ],
            "cards": [
                {"name": "Nadu, Winged Wisdom", "in_pool": True,
                 "mana_cost": "{1}{G}{U}",
                 "type_line": "Legendary Creature — Bird Wizard"},
                {"name": "Spoiled Newcomer", "in_pool": False,
                 "mana_cost": None, "type_line": None},
            ],
            "sources": [{"id": 1, "title": "Commander bans, September 2024",
                         "url": "https://example.com/bans"}],
            "sources_dropped": 0, "findings_dropped": 1,
            "cards_unresolved": 1, "searched": 12,
        },
        "never": "Research reads pages; it cannot see a deck.",
        "usage": {"input_tokens": 700, "output_tokens": 1500,
                  "cache_read_tokens": 40000},
    }
    report.update(over)
    return report


def test_claude_research_renders_the_two_kinds_of_card_differently(
        decks, monkeypatch, capsys):
    from mtglab.claude import research as research_mod
    monkeypatch.setattr(research_mod, "ask",
                        lambda question, requested: _research_report())
    code, _ = run(["claude", "research", "Is", "Nadu", "banned?"])
    out = capsys.readouterr().out
    assert code == 0
    assert "ANSWER  [settled]" in out
    assert "banned in the September 2024 update" in out
    assert "FINDINGS" in out and "[1]" in out
    # The ADR 26 seam: a pool card prints its text, a spoiler prints a label.
    assert "Nadu, Winged Wisdom  {1}{G}{U}" in out
    assert "Spoiled Newcomer  — not in the pool" in out
    assert "1 named card(s) are not in the pool — expected" in out
    assert "1 finding(s) left citing nothing" in out
    assert "12 pages searched, 1 cited." in out
    assert "tokens: 700 in / 1500 out (40000 cached)" in out


def test_claude_research_refusal_prints_the_reason(decks, monkeypatch, capsys):
    from mtglab.claude import research as research_mod
    monkeypatch.setattr(research_mod, "ask", lambda question, requested:
                        _research_report(research=None,
                                         reason="no source survived; refused."))
    code, _ = run(["claude", "research", "anything"])
    assert code == 0
    assert "no source survived" in capsys.readouterr().out


def test_claude_research_rejection_is_a_message_not_a_traceback(
        decks, monkeypatch, capsys):
    from mtglab.claude import research as research_mod

    def boom(question, requested):
        raise research_mod.QuestionRejected(
            "research cannot see a deck, so it cannot say what to cut")
    monkeypatch.setattr(research_mod, "ask", boom)
    code, _ = run(["claude", "research", "what", "should", "I", "cut?"])
    assert code == 1
    assert "cannot see a deck" in capsys.readouterr().out


# ------------------------------------------------- data, art, ui, forge

def test_data_refresh_loads_both_bulks_and_oracle_only_skips_one(
        decks, monkeypatch, capsys):
    from types import SimpleNamespace

    from mtglab.cards import db as cards_db

    calls = []
    monkeypatch.setattr(cards_db, "connect",
                        lambda path: SimpleNamespace(close=lambda: None))
    monkeypatch.setattr(cards_db, "download_bulk",
                        lambda kind: f"data/scryfall/{kind}.jsonl.gz")
    monkeypatch.setattr(cards_db, "load_oracle",
                        lambda con, p: calls.append("oracle") or 35390)
    monkeypatch.setattr(cards_db, "load_printings",
                        lambda con, p: calls.append("printings") or 107338)

    code, _ = run(["data", "refresh"])
    out = capsys.readouterr().out
    assert code == 0
    assert "loaded 35,390 oracle cards" in out
    assert "loaded 107,338 printings" in out
    assert calls == ["oracle", "printings"]

    calls.clear()
    run(["data", "refresh", "--oracle-only"])
    assert calls == ["oracle"], "--oracle-only must not touch default_cards"


def test_data_snapshot_reports_the_count(decks, monkeypatch, capsys):
    from types import SimpleNamespace

    from mtglab.cards import db as cards_db
    monkeypatch.setattr(cards_db, "connect",
                        lambda path: SimpleNamespace(close=lambda: None))
    monkeypatch.setattr(cards_db, "snapshot_prices", lambda con: 1234)
    code, _ = run(["data", "snapshot"])
    assert code == 0
    assert "snapshotted 1,234 prices" in capsys.readouterr().out


def _gate_clean():
    """The edit-result keys `_report_edit` reads, with a clean gate."""
    return {"errors": [], "warnings": []}


def test_import_prints_what_it_kept_and_what_it_could_not_read(
        decks, monkeypatch, capsys):
    """The unknown/unreadable/skipped accounting, which is the import's whole
    honesty story: nothing is guessed, so everything set aside is listed."""
    from mtglab.api import service
    monkeypatch.setattr(service, "import_deck", lambda **kw: {
        "name": "Mini Deck", "slug": "mini2", "commander": ["Gyome, Master Chef"],
        "companion": "Kaheera, the Orphanguard",
        "total_cards": 99, "land_count": 36, "swap_board": [],
        "notes": ["bracket defaulted to 4"],
        "unknown": ["Definitely Not A Card"],
        "unreadable": [{"line": 7, "text": "4x ???"}],
        "skipped": ["Food"], "needs_rationale": 99,
        **_gate_clean(),
    })
    listing = tmp_listing(decks, "1 Sol Ring\n")
    code, _ = run(["decks", "import", "mini2", "--from", str(listing),
                   "--commander", "Gyome, Master Chef"])
    out = capsys.readouterr().out
    assert code == 0
    assert "companion: Kaheera" in out
    assert "note: bracket defaulted to 4" in out
    assert "1 name(s) the pool does not know" in out
    assert "Definitely Not A Card" in out
    assert "line 7: 4x ???" in out
    assert "1 token line(s) skipped" in out


def test_swap_prints_the_exchange_and_reruns_the_gate(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "swap_card", lambda *a, **kw: {
        "swapped_out": "Sol Ring", "swapped_in": "Arcane Signet",
        **_gate_clean(),
    })
    code, _ = run(["decks", "swap", "mini", "--out", "Sol Ring",
                   "--in", "Arcane Signet", "--why", "Coloured mana."])
    out = capsys.readouterr().out
    assert code == 0
    assert "Sol Ring  ->  Arcane Signet" in out
    assert "gate: 0 error(s), 0 warning(s)" in out


def test_a_refused_swap_is_an_exit_message(decks, monkeypatch, capsys):
    from mtglab.api import service

    def boom(*a, **kw):
        raise service.SwapRejected("'Sol Ring' is not in mini")
    monkeypatch.setattr(service, "swap_card", boom)
    code, msg = run(["decks", "swap", "mini", "--out", "Sol Ring",
                     "--in", "Arcane Signet", "--why", "x"])
    assert code == 1
    assert "refused: 'Sol Ring' is not in mini" in msg


def test_add_prints_destination_quantity_and_a_draft_reminder(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "add_card", lambda *a, **kw: {
        "added": "Swamp", "category": "land",
        "stage": "draft", "needs_rationale": 12,
        **_gate_clean(),
    })
    code, _ = run(["decks", "add", "mini", "--card", "Swamp",
                   "--category", "land", "--qty", "2", "--to", "swap_board"])
    out = capsys.readouterr().out
    assert code == 0
    assert "+ 2x Swamp  (land) -> the swap board" in out
    assert "draft: 12 card(s) still owe a `why`." in out


def _printings(*entries):
    return {"commander": "Gyome, Master Chef",
            "printings": [
                {"id": e[0], "set_code": e[1], "collector_number": e[2],
                 "rarity": "rare", "set_name": e[3]} for e in entries]}


def test_set_art_resolves_a_set_code_to_its_one_printing(
        decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "commander_printings",
                        lambda slug: _printings(("abc-123", "C21", "1",
                                                 "Commander 2021")))
    seen = {}

    def set_field(slug, *, field, value):
        seen.update(field=field, value=value)
        return _gate_clean()
    monkeypatch.setattr(service, "set_deck_field", set_field)
    code, _ = run(["decks", "set", "mini", "--art", "c21"])
    assert code == 0
    assert seen == {"field": "commander_art", "value": "abc-123"}
    assert "commander_art -> abc-123" in capsys.readouterr().out


def test_set_art_with_several_printings_lists_them_and_refuses(
        decks, monkeypatch, capsys):
    """`MUL` has four Goreclaws; picking one silently would be wrong about
    which painting the user meant, which is the whole reason ids exist."""
    from mtglab.api import service
    monkeypatch.setattr(service, "commander_printings",
                        lambda slug: _printings(
                            ("id-1", "MUL", "42", "Multiverse Legends"),
                            ("id-2", "MUL", "107", "Multiverse Legends")))
    code, msg = run(["decks", "set", "mini", "--art", "MUL"])
    out = capsys.readouterr().out
    assert code == 1
    assert "re-run with --art <id>" in msg
    assert "MUL has 2 printings" in out
    assert "id-1" in out and "#42" in out


def test_set_art_names_the_sets_it_is_actually_in(decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "commander_printings",
                        lambda slug: _printings(("id-1", "C21", "1",
                                                 "Commander 2021")))
    code, msg = run(["decks", "set", "mini", "--art", "ZNR"])
    assert code == 1
    assert "no printing in 'ZNR'" in msg
    assert "It is in: C21" in msg


def test_set_art_without_a_pool_says_to_load_one(decks, monkeypatch):
    from mtglab.api import service
    monkeypatch.setattr(service, "commander_printings",
                        lambda slug: {"commander": "", "printings": []})
    code, msg = run(["decks", "set", "mini", "--art", "C21"])
    assert code == 1
    assert "is the pool loaded?" in msg


def test_ui_hands_the_app_to_uvicorn_on_the_asked_address(
        decks, monkeypatch, capsys):
    import sys as _sys
    from types import SimpleNamespace

    served = {}

    def fake_run(app, *, host, port, log_level):
        served.update(host=host, port=port)
    monkeypatch.setitem(_sys.modules, "uvicorn",
                        SimpleNamespace(run=fake_run))
    code, _ = run(["ui", "--no-open", "--host", "127.0.0.1", "--port", "8123"])
    assert code == 0
    assert served == {"host": "127.0.0.1", "port": 8123}
    assert "http://127.0.0.1:8123" in capsys.readouterr().out


def test_ui_warns_when_the_frontend_was_never_built(decks, monkeypatch, capsys):
    import sys as _sys
    from types import SimpleNamespace

    from mtglab.api import app as app_mod
    monkeypatch.setitem(_sys.modules, "uvicorn",
                        SimpleNamespace(run=lambda *a, **kw: None))
    monkeypatch.setattr(app_mod, "WEB_DIST",
                        app_mod.WEB_DIST / "nowhere")
    run(["ui", "--no-open"])
    out = capsys.readouterr().out
    assert "no built frontend" in out
    assert "npm --prefix web run build" in out


def _forge_result(games, *, wall=12.0, startup=6.0):
    from types import SimpleNamespace
    return SimpleNamespace(
        games=[SimpleNamespace(milliseconds=ms, timed_out=t, winner=w)
               for ms, t, w in games],
        wall_seconds=wall, startup_seconds=startup,
        winner_slug=lambda game: game.winner)


def test_sim_forge_reports_wins_draws_and_clockouts_separately(
        decks, monkeypatch, capsys):
    """A clock-out is the measurement giving up, not the game ending -- it is
    reported apart from the draws it was scored as (docs/FORGE.md)."""
    from mtglab.sim.tier3 import run as forge
    monkeypatch.setattr(forge, "run_games", lambda *a, **kw: _forge_result([
        (5000, False, "mini"), (7000, False, None), (300000, True, None)]))
    code, _ = run(["sim", "forge", "mini", "mini"])
    out = capsys.readouterr().out
    assert code == 0
    assert "3 games in 12.0s (6.0s of it JVM + card database)" in out
    assert "per game: 5.0s min" in out
    assert "draw" in out
    assert "(1 hit the 300s clock and were called draws)" in out
    assert "Read these per archetype, not as one ranking." in out


def test_sim_forge_check_only_prints_coverage_and_runs_nothing(
        decks, monkeypatch, capsys):
    from types import SimpleNamespace

    from mtglab.sim.tier3 import run as forge
    monkeypatch.setattr(forge, "check_coverage", lambda decks: [
        SimpleNamespace(summary=lambda: "mini: 99/99 implemented")])
    monkeypatch.setattr(forge, "run_games",
                        lambda *a, **kw: pytest.fail("must not run games"))
    code, _ = run(["sim", "forge", "mini", "--check-only"])
    assert code == 0
    assert "mini: 99/99 implemented" in capsys.readouterr().out


def test_sim_forge_refuses_when_coverage_fails(decks, monkeypatch):
    from mtglab.sim.tier3 import run as forge

    def boom(*a, **kw):
        raise forge.CoverageFailed("mini: 3 card(s) Forge does not implement")
    monkeypatch.setattr(forge, "run_games", boom)
    code, msg = run(["sim", "forge", "mini", "mini"])
    assert code == 1
    assert "3 card(s)" in msg


def test_sim_lands_refuses_a_deck_with_no_lands(decks, monkeypatch):
    import mtglab.cli as cli
    monkeypatch.setattr(cli, "_pool", lambda deck: {})
    monkeypatch.setattr(cli, "_sim_cards", lambda deck, cards: ([], None))
    code, msg = run(["sim", "lands", "mini", "30", "32"])
    assert code == 1
    assert "no lands to sweep" in msg


def test_sim_cache_prints_kinds_and_the_age_span(decks, monkeypatch, capsys):
    from mtglab.sim import cache
    monkeypatch.setattr(cache, "stats", lambda: {
        "enabled": True, "rows": 3, "bytes": 2048,
        "by_kind": {"sim.mana": 2, "sim.lands.count": 1},
        "oldest": "2026-08-13T10:00:00+00:00",
        "newest": "2026-08-14T09:00:00+00:00"})
    code, _ = run(["sim", "cache"])
    out = capsys.readouterr().out
    assert code == 0
    assert "rows:    3 (2.0 kB)" in out
    assert "sim.mana" in out and "sim.lands.count" in out
    assert "computed between 2026-08-13T10:00:00 and 2026-08-14T09:00:00" in out


def test_price_without_a_pool_exits_with_advice(decks):
    code, msg = run(["price", "deck", "mini"])
    assert code == 1
    assert "data refresh" in msg


def test_suggest_for_a_named_card_says_when_the_pool_cannot_help(
        decks, monkeypatch, capsys):
    from types import SimpleNamespace

    from mtglab.cards import db as cards_db
    from mtglab.decks import suggest as suggest_mod

    (decks.parent / "data").mkdir(exist_ok=True)
    (decks.parent / "data" / "mtg.duckdb").touch()
    monkeypatch.setattr(cards_db, "connect",
                        lambda path: SimpleNamespace(close=lambda: None))
    monkeypatch.setattr(cards_db, "get_cards", lambda con, names: {})
    monkeypatch.setattr(suggest_mod, "replacements_for",
                        lambda *a, **kw: [])
    code, _ = run(["decks", "suggest", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert code == 0
    assert "Sol Ring (asked for)" in out
    assert "no candidates -- the pool does not know this card." in out


def test_suggest_ranks_candidates_with_their_reasons(
        decks, monkeypatch, capsys):
    from types import SimpleNamespace

    from mtglab.cards import db as cards_db
    from mtglab.decks import suggest as suggest_mod

    (decks.parent / "data").mkdir(exist_ok=True)
    (decks.parent / "data" / "mtg.duckdb").touch()
    monkeypatch.setattr(cards_db, "connect",
                        lambda path: SimpleNamespace(close=lambda: None))
    monkeypatch.setattr(cards_db, "get_cards", lambda con, names: {})
    monkeypatch.setattr(suggest_mod, "replacements_for", lambda *a, **kw: [
        SimpleNamespace(name="Arcane Signet", score=0.91,
                        reasons=["artifact", "ramp"],
                        record=SimpleNamespace(mana_cost="{2}"))])
    code, _ = run(["decks", "suggest", "mini", "--card", "Sol Ring"])
    out = capsys.readouterr().out
    assert code == 0
    assert "1. Arcane Signet" in out and "0.91" in out
    assert "artifact · ramp" in out


def test_set_art_takes_a_printing_id_verbatim(decks, monkeypatch, capsys):
    from mtglab.api import service
    monkeypatch.setattr(service, "commander_printings",
                        lambda slug: _printings(("abc-123", "C21", "1",
                                                 "Commander 2021")))
    monkeypatch.setattr(service, "set_deck_field",
                        lambda slug, *, field, value: _gate_clean())
    code, _ = run(["decks", "set", "mini", "--art", "abc-123"])
    assert code == 0
    assert "commander_art -> abc-123" in capsys.readouterr().out


def test_build_against_a_previous_revision_diffs_the_swaps(decks, capsys):
    """`--against` is why `swaps.md` exists: commit first, then diff."""
    previous = decks.parent / "previous.yaml"
    previous.write_text(GOOD_DECK.replace("Sol Ring", "Arcane Signet"),
                        encoding="utf-8")
    code, _ = run(["decks", "build", "mini", "--against", str(previous)])
    assert code == 0
    swaps = (decks / "mini" / "artifacts" / "swaps.md").read_text("utf-8")
    assert "Sol Ring" in swaps and "Arcane Signet" in swaps
