"""The deck source seam.

`decks/source.py` exists because there will be two places decks live -- the
curated six in git, and other people's in SQLite (ADR 4) -- and introducing the
abstraction while there is one implementation is what makes the second one
additive. These tests pin the contract both implementations have to meet, and
the call-time path resolution that is easy to get wrong.

Nothing here touches HTTP or DuckDB. The API's use of the seam is tested in
tests/test_api.py, against an in-memory source.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import pytest

from mtglab import config
from mtglab.decks.model import Deck
from mtglab.decks.source import (
    DeckNotFound,
    DeckSource,
    FileDeckSource,
    MemoryDeckSource,
)

DECK_YAML = """\
slug: mini
name: Mini Deck
commander:
  - Gyome, Master Chef
bracket: 4
strategy: A minimal but legally sized deck used by the source tests.
cards:
  - name: Swamp
    category: land
    qty: 98
    why: Black mana.
  - name: Sol Ring
    category: ramp
    why: Two mana for one.
"""


@pytest.fixture
def decks_root(tmp_path):
    """Two decks and one underscore-prefixed scaffolding directory."""
    root = tmp_path / "decks"
    for slug in ("mini", "other", "_template"):
        (root / slug).mkdir(parents=True)
        (root / slug / "deck.yaml").write_text(
            DECK_YAML.replace("slug: mini", f"slug: {slug}"), encoding="utf-8")
    return root


# ------------------------------------------------------------ file-backed

def test_file_source_lists_real_decks_and_skips_scaffolding(decks_root):
    source = FileDeckSource(decks_root)
    assert source.slugs() == ["mini", "other"]
    assert [d.slug for d in source.all()] == ["mini", "other"]


def test_file_source_loads_one_deck(decks_root):
    deck = FileDeckSource(decks_root).get("mini")
    assert deck.name == "Mini Deck"
    assert deck.total_cards == 99


def test_file_source_raises_deck_not_found(decks_root):
    with pytest.raises(DeckNotFound):
        FileDeckSource(decks_root).get("no-such-deck")


def test_file_source_with_no_root_follows_config_at_call_time(decks_root):
    """The bug this shape prevents: binding `DECKS_DIR` at construction, so a
    source built at import time keeps pointing at the old directory after
    `use_paths()` moves it. `deck_paths()` already had this problem and solved
    it the same way."""
    source = FileDeckSource()
    with config.use_paths(decks_dir=decks_root):
        assert source.slugs() == ["mini", "other"]
    with config.use_paths(decks_dir=decks_root.parent / "absent"):
        assert source.slugs() == []


def test_an_explicit_root_ignores_config(decks_root):
    source = FileDeckSource(decks_root)
    with config.use_paths(decks_dir=decks_root.parent / "absent"):
        assert source.slugs() == ["mini", "other"]


def test_slugs_does_not_parse_the_deck_files(decks_root):
    """`/api/health` only wants a count. One unreadable deck file must not take
    the health endpoint down with it -- and it cannot, because listing slugs
    never opens a file."""
    (decks_root / "broken").mkdir()
    (decks_root / "broken" / "deck.yaml").write_text(
        "this: [is not: valid yaml", encoding="utf-8")
    source = FileDeckSource(decks_root)
    assert source.slugs() == ["broken", "mini", "other"]
    with pytest.raises(Exception):                                  # noqa: B017
        source.get("broken")


def test_a_missing_decks_directory_is_empty_not_an_error(tmp_path):
    assert FileDeckSource(tmp_path / "absent").slugs() == []
    assert FileDeckSource(tmp_path / "absent").all() == []


# ---------------------------------------------------------------- in-memory

def test_memory_source_round_trips(decks_root):
    deck = Deck.load(decks_root / "mini" / "deck.yaml")
    source = MemoryDeckSource([deck])
    assert source.slugs() == ["mini"]
    assert source.get("mini") is deck
    assert source.all() == [deck]


def test_memory_source_raises_the_same_error(decks_root):
    with pytest.raises(DeckNotFound):
        MemoryDeckSource().get("mini")


def test_an_empty_source_is_a_valid_source():
    source = MemoryDeckSource()
    assert source.slugs() == []
    assert source.all() == []


# ----------------------------------------------------------- the protocol

@pytest.mark.parametrize("source", [FileDeckSource(), MemoryDeckSource()])
def test_both_implementations_satisfy_the_protocol(source):
    """Structural, so a future `SqlDeckSource` needs no inheritance -- and so a
    test double is a source without pretending to be one."""
    assert isinstance(source, DeckSource)


if __name__ == "__main__":
    sys.exit(pytest.main([__file__, "-q"]))
