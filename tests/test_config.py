"""Where mtglab looks for decks and the corpus.

These paths used to be module-level constants in `cli.py`. That had two costs:
the API had to import the command line to find them, and there was no way to
point a test at a scratch directory -- which is most of why `cli.py` sat at
27% coverage.

`DB_PATH` is derived from `DATA_DIR`, so the tests here care mainly that the
two cannot drift apart.
"""

import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab import config


@pytest.fixture
def clean_env(monkeypatch):
    monkeypatch.delenv("MTGLAB_DATA_DIR", raising=False)
    monkeypatch.delenv("MTGLAB_DECKS_DIR", raising=False)
    yield
    config.reload_from_env()


def test_defaults_match_the_historical_constants(clean_env):
    """Local use must be unchanged by making these configurable."""
    config.reload_from_env()
    # Bound to locals because ruff's SIM300 reads ALL_CAPS attributes as
    # literals and wants the comparison flipped, which reads worse.
    data_dir, decks_dir, db_path = config.DATA_DIR, config.DECKS_DIR, config.DB_PATH
    assert data_dir == Path("data")
    assert decks_dir == Path("decks")
    assert db_path == Path("data/mtg.duckdb")


def test_environment_overrides_both_directories(clean_env, monkeypatch):
    monkeypatch.setenv("MTGLAB_DATA_DIR", "/srv/mtg/data")
    monkeypatch.setenv("MTGLAB_DECKS_DIR", "/srv/mtg/decks")
    config.reload_from_env()
    data_dir, decks_dir = config.DATA_DIR, config.DECKS_DIR
    assert data_dir == Path("/srv/mtg/data")
    assert decks_dir == Path("/srv/mtg/decks")


def test_db_path_follows_data_dir(clean_env, monkeypatch):
    """The reason DB_PATH is derived rather than separately assignable: a
    container that mounts a volume at /data must not keep reading the corpus
    from ./data."""
    monkeypatch.setenv("MTGLAB_DATA_DIR", "/data")
    config.reload_from_env()
    db_path = config.DB_PATH
    assert db_path == Path("/data/mtg.duckdb")


def test_use_paths_restores_afterwards(tmp_path):
    before = (config.DATA_DIR, config.DECKS_DIR, config.DB_PATH)
    with config.use_paths(data_dir=tmp_path / "d", decks_dir=tmp_path / "k"):
        inside = (config.DECKS_DIR, config.DB_PATH)
        assert inside == (tmp_path / "k", tmp_path / "d" / "mtg.duckdb")
    after = (config.DATA_DIR, config.DECKS_DIR, config.DB_PATH)
    assert after == before


def test_use_paths_restores_even_when_the_body_raises(tmp_path):
    before = config.DECKS_DIR
    with pytest.raises(ValueError), config.use_paths(decks_dir=tmp_path):
        raise ValueError("boom")
    after = config.DECKS_DIR
    assert after == before


# ------------------------------------------------------------- deck_paths

def make_deck_dir(root: Path, *names: str) -> Path:
    for n in names:
        (root / n).mkdir(parents=True, exist_ok=True)
        (root / n / "deck.yaml").write_text("slug: x\n", encoding="utf-8")
    return root


def test_deck_paths_skips_underscore_scaffolding(tmp_path):
    """`_template` is a starting point, not a deck. It must never appear in
    the library or the API's deck list."""
    root = make_deck_dir(tmp_path, "real-deck", "_template", "_scratch")
    found = {p.parent.name for p in config.deck_paths(root)}
    assert found == {"real-deck"}


def test_deck_paths_is_sorted(tmp_path):
    root = make_deck_dir(tmp_path, "zed", "alpha", "mid")
    assert [p.parent.name for p in config.deck_paths(root)] == ["alpha", "mid", "zed"]


def test_deck_paths_on_a_missing_directory_is_empty_not_an_error(tmp_path):
    """A fresh clone with no decks/ must not explode."""
    assert config.deck_paths(tmp_path / "nope") == []


def test_deck_paths_reads_the_current_setting_not_an_import_time_default(tmp_path):
    """The bug this guards: binding DECKS_DIR as a default argument would make
    use_paths() silently ineffective."""
    root = make_deck_dir(tmp_path, "scratch-deck")
    with config.use_paths(decks_dir=root):
        found = [p.parent.name for p in config.deck_paths()]
    assert found == ["scratch-deck"]


def test_the_service_resolves_the_corpus_path_at_call_time(tmp_path):
    """The same bug as `deck_paths`, found in `service._connect`, which did
    `from mtglab.config import DB_PATH` and so bound the default `data/`
    forever. It worked in production, where the environment is set before the
    process starts, and made it impossible to point a test at a scratch corpus
    -- so the import path could only be tested against the real 500MB one."""
    from mtglab.api import service

    with config.use_paths(data_dir=tmp_path / "empty"):
        assert service._connect() is None

    pytest.importorskip("duckdb")
    import tiny_corpus

    with config.use_paths(data_dir=tmp_path / "corpus"):
        tiny_corpus.build(config.DB_PATH)
        con = service._connect()
        assert con is not None
        try:
            # The fixture's own size, not a number to remember here -- the
            # point of this test is which *path* was opened.
            assert (con.execute("SELECT count(*) FROM oracle_cards").fetchone()[0]
                    == len(tiny_corpus.CARDS))
        finally:
            con.close()


def test_env_vars_are_read_at_import(monkeypatch, tmp_path):
    """A container sets these before the process starts; nothing should have
    to call reload_from_env() for them to take effect."""
    monkeypatch.setenv("MTGLAB_DECKS_DIR", str(tmp_path))
    assert os.environ["MTGLAB_DECKS_DIR"] == str(tmp_path)
    config.reload_from_env()
    decks_dir = config.DECKS_DIR
    assert decks_dir == tmp_path
    monkeypatch.delenv("MTGLAB_DECKS_DIR")
    config.reload_from_env()


# ------------------------------------------------------------------ .env
#
# The API key reaches the app through the environment, and the app is started
# several ways -- `mtglab ui`, uvicorn, the launch configuration in `.claude/`
# -- only some of which inherit a login shell. A `.env` the process reads
# itself is what makes that uniform, so its two rules are worth pinning.

def test_a_dotenv_populates_the_environment(tmp_path, monkeypatch):
    pytest.importorskip("dotenv")
    monkeypatch.delenv("MTGLAB_SCRATCH_PROBE", raising=False)
    (tmp_path / ".env").write_text("MTGLAB_SCRATCH_PROBE=from-the-file\n",
                                   encoding="utf-8")
    monkeypatch.chdir(tmp_path)

    config._load_dotenv()
    assert os.environ["MTGLAB_SCRATCH_PROBE"] == "from-the-file"


def test_the_real_environment_wins_over_the_file(tmp_path, monkeypatch):
    """A stale `.env` must never silently shadow an exported value.

    This is the rule that matters for the API key specifically: using the wrong
    key is confusing rather than loud, so the precedence has to be the obvious
    one.
    """
    pytest.importorskip("dotenv")
    monkeypatch.setenv("MTGLAB_SCRATCH_PROBE", "from-the-environment")
    (tmp_path / ".env").write_text("MTGLAB_SCRATCH_PROBE=from-the-file\n",
                                   encoding="utf-8")
    monkeypatch.chdir(tmp_path)

    config._load_dotenv()
    assert os.environ["MTGLAB_SCRATCH_PROBE"] == "from-the-environment"


def test_no_dotenv_is_not_an_error(tmp_path, monkeypatch):
    """A base install has no `.env` and no python-dotenv. Importing the package
    must not depend on either."""
    monkeypatch.chdir(tmp_path)
    config._load_dotenv()


def test_the_api_key_is_never_bound_to_a_name(tmp_path, monkeypatch):
    """`config` loads the file and stops there.

    The key is read by the Anthropic SDK directly. A value this project never
    holds is one it cannot log, serialise into an error response, or leak into
    a prompt -- so the absence of an attribute here is the feature.
    """
    pytest.importorskip("dotenv")
    (tmp_path / ".env").write_text("ANTHROPIC_API_KEY=not-a-real-key\n",
                                   encoding="utf-8")
    monkeypatch.chdir(tmp_path)
    config._load_dotenv()

    exported = [n for n in dir(config) if not n.startswith("_")]
    assert not any("KEY" in n.upper() or "TOKEN" in n.upper() or
                   "SECRET" in n.upper() for n in exported), exported


def test_a_blank_credential_is_dropped_rather_than_passed_along(monkeypatch):
    """Empty is worse than absent, per Anthropic's credential precedence.

    `ANTHROPIC_API_KEY=""` selects the API-key path with an empty key and fails
    as a 401, instead of falling through or reporting that nothing is
    configured. A half-filled `.env` is the obvious way to produce one by
    accident, so it is deleted here and the failure becomes honest.
    """
    for blank in ("", "   "):
        monkeypatch.setenv("ANTHROPIC_API_KEY", blank)
        monkeypatch.setenv("ANTHROPIC_AUTH_TOKEN", blank)
        config._drop_blank_credentials()
        assert "ANTHROPIC_API_KEY" not in os.environ
        assert "ANTHROPIC_AUTH_TOKEN" not in os.environ


def test_a_real_credential_is_left_alone(monkeypatch):
    monkeypatch.setenv("ANTHROPIC_API_KEY", "not-a-real-key")
    config._drop_blank_credentials()
    assert os.environ["ANTHROPIC_API_KEY"] == "not-a-real-key"


def test_the_example_env_does_not_ship_a_blank_key():
    """`.env.example` is meant to be copied. If it set the key to nothing, the
    copy would be the failure mode above rather than a clean unset."""
    example = (Path(__file__).resolve().parents[1] / ".env.example").read_text()
    live = [ln for ln in example.splitlines()
            if ln.strip() and not ln.lstrip().startswith("#")]
    assert not any(ln.startswith("ANTHROPIC_API_KEY=") for ln in live), live
