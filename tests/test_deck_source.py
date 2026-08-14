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
    DeckExists,
    DeckNotFound,
    DeckSource,
    FileDeckSource,
    MemoryDeckSource,
    ReadOnlySource,
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



# ------------------------------------------------------------- raw text
#
# Edits are surgical, so a source has to hand back the bytes rather than a
# parsed deck. See decks/edit.py for why.

def test_file_source_round_trips_raw_text(decks_root):
    source = FileDeckSource(decks_root)
    text = source.read_text("mini")
    assert text.startswith("slug: mini")

    source.write_text("mini", text.replace("Mini Deck", "Renamed Deck"))
    assert source.get("mini").name == "Renamed Deck"
    assert "Renamed Deck" in (decks_root / "mini" / "deck.yaml").read_text()


def test_file_source_text_operations_need_a_real_deck(decks_root):
    source = FileDeckSource(decks_root)
    with pytest.raises(DeckNotFound):
        source.read_text("absent")
    with pytest.raises(DeckNotFound):
        source.write_text("absent", "slug: absent\n")


def test_memory_source_round_trips_raw_text(decks_root):
    deck = Deck.load(decks_root / "mini" / "deck.yaml")
    source = MemoryDeckSource([deck])
    text = source.read_text("mini")
    assert "slug: mini" in text

    source.write_text("mini", text.replace("Mini Deck", "Renamed"))
    assert source.get("mini").name == "Renamed"
    assert "Renamed" in source.read_text("mini")


def test_memory_source_will_not_write_an_unknown_deck(decks_root):
    with pytest.raises(DeckNotFound):
        MemoryDeckSource().write_text("mini", "slug: mini\n")


def test_a_read_only_source_refuses_writes(decks_root):
    """What the hosted model needs: curated decks stay read-only for anyone
    but the maintainer, refused in one place rather than per endpoint."""
    deck = Deck.load(decks_root / "mini" / "deck.yaml")
    source = MemoryDeckSource([deck], writable=False)
    assert source.writable is False
    assert FileDeckSource(decks_root).writable is True
    with pytest.raises(ReadOnlySource):
        source.write_text("mini", "slug: mini\n")
    with pytest.raises(ReadOnlySource):
        source.create("new", "slug: new\n")


def test_a_read_only_file_source_refuses_all_three_writes(decks_root):
    """The same contract, on the implementation that has a filesystem.

    `MemoryDeckSource` has always honoured `writable`; `FileDeckSource`
    hardcoded `True` and so the flag was, for the only source the API actually
    served, decorative. Every one of these three raised nothing before.
    """
    source = FileDeckSource(decks_root, writable=False)
    assert source.writable is False
    with pytest.raises(ReadOnlySource):
        source.write_text("mini", "slug: mini\n")
    with pytest.raises(ReadOnlySource):
        source.create("new", "slug: new\n")
    with pytest.raises(ReadOnlySource):
        source.delete("mini")


def test_a_read_only_file_source_leaves_the_disk_alone(decks_root):
    """The assertion that matters: refusing is not the same as not writing.

    An implementation that raised after `shutil.move` would pass the test
    above and still have destroyed the deck.
    """
    before = (decks_root / "mini" / "deck.yaml").read_text(encoding="utf-8")
    source = FileDeckSource(decks_root, writable=False)
    for attempt in (lambda: source.write_text("mini", "wrecked: true\n"),
                    lambda: source.delete("mini"),
                    lambda: source.create("fresh", "slug: fresh\n")):
        with pytest.raises(ReadOnlySource):
            attempt()
    assert (decks_root / "mini" / "deck.yaml").read_text(
        encoding="utf-8") == before
    assert sorted(source.slugs()) == ["mini", "other"]
    assert not (decks_root / ".trash").exists()
    assert not (decks_root / "fresh").exists()


def test_read_only_refusal_does_not_depend_on_the_deck_existing(decks_root):
    """A refused write says the same thing whether or not the deck is there.

    Otherwise the pair (403, 404) is a membership oracle for the library. It
    is a readable library, so this leaks nothing today -- but the ordering is
    the part that survives into the per-user tier, where it will.
    """
    source = FileDeckSource(decks_root, writable=False)
    with pytest.raises(ReadOnlySource):
        source.write_text("mini", "slug: mini\n")
    with pytest.raises(ReadOnlySource):
        source.write_text("no-such-deck", "slug: no-such-deck\n")
    with pytest.raises(ReadOnlySource):
        source.delete("no-such-deck")
    # And a *writable* source still tells the difference, so the ordering
    # above is the read-only path's doing rather than a lost 404.
    with pytest.raises(DeckNotFound):
        FileDeckSource(decks_root).write_text("no-such-deck", "x: 1\n")


def test_a_file_source_is_writable_by_default(decks_root):
    """Every caller that is not the API -- the CLI, the artifact generator,
    the tests above -- constructs one positionally and must be unaffected."""
    assert FileDeckSource(decks_root).writable is True
    assert FileDeckSource().writable is True


# ---------------------------------------------------------------- creating

def test_create_makes_a_new_deck_and_its_directory(decks_root):
    source = FileDeckSource(decks_root)
    source.create("fresh", DECK_YAML.replace("slug: mini", "slug: fresh"))
    assert (decks_root / "fresh" / "deck.yaml").exists()
    assert source.get("fresh").total_cards == 99
    assert "fresh" in source.slugs()


def test_create_refuses_to_overwrite_an_existing_deck(decks_root):
    """`write_text` and `create` have opposite safety requirements, which is
    why they are separate calls: updating a deck that vanished is a bug, and
    creating over a deck that exists destroys somebody's work."""
    source = FileDeckSource(decks_root)
    with pytest.raises(DeckExists):
        source.create("mini", "slug: mini\n")
    # And the deck it refused to touch is unchanged.
    assert source.get("mini").name == "Mini Deck"


def test_memory_source_creates_and_refuses_duplicates(decks_root):
    source = MemoryDeckSource()
    source.create("mini", DECK_YAML)
    assert source.slugs() == ["mini"]
    assert source.get("mini").name == "Mini Deck"
    with pytest.raises(DeckExists):
        source.create("mini", DECK_YAML)


# ---------------------------------------------------------------- deleting

def test_delete_moves_the_deck_aside_rather_than_erasing_it(decks_root):
    """The promise the return value makes: it says *where*, not whether.

    A deck committed to git has `git checkout` as its undo. The deck this is
    most likely to be aimed at by mistake is a draft imported ten minutes ago,
    which has no other copy — so the delete has to leave one.
    """
    source = FileDeckSource(decks_root)
    (decks_root / "mini" / "artifacts").mkdir()
    (decks_root / "mini" / "artifacts" / "primer-quick.md").write_text("x")

    moved_to = source.delete("mini")

    assert not (decks_root / "mini").exists()
    assert "mini" not in source.slugs()
    landed = Path(moved_to)
    assert landed.is_dir()
    assert (landed / "deck.yaml").exists()
    # The artifacts go with it. A folder of primers for a deck that no longer
    # exists is worse than no folder at all.
    assert (landed / "artifacts" / "primer-quick.md").exists()


def test_a_trashed_deck_is_invisible_to_the_library(decks_root):
    """`.trash` is dot-prefixed so `config.deck_paths` cannot see it — the
    glob is `*/deck.yaml` and a trashed deck sits a level deeper. A deleted
    deck reappearing in the library would be the whole feature backwards."""
    source = FileDeckSource(decks_root)
    source.delete("mini")
    assert "mini" not in source.slugs()
    assert "mini" not in {d.slug for d in source.all()}
    # And nothing under `.trash` is mistaken for a deck, however it is named.
    assert not any(".trash" in p.parts for p in config.deck_paths(decks_root))


def test_deleting_twice_is_a_not_found_rather_than_a_second_move(decks_root):
    source = FileDeckSource(decks_root)
    source.delete("mini")
    with pytest.raises(DeckNotFound):
        source.delete("mini")


def test_delete_refuses_an_unknown_deck(decks_root):
    with pytest.raises(DeckNotFound):
        FileDeckSource(decks_root).delete("no-such-deck")


def test_two_deletions_of_the_same_slug_can_coexist_in_the_trash(decks_root):
    """Import, delete, re-import, delete again. The second must not overwrite
    the first — which is the one case a bare `slug` directory name would."""
    source = FileDeckSource(decks_root)
    first = source.delete("mini")
    source.create("mini", DECK_YAML)
    second = source.delete("mini")
    assert first != second
    assert Path(first).exists() and Path(second).exists()


def test_memory_source_deletes(decks_root):
    deck = Deck.load(decks_root / "mini" / "deck.yaml")
    source = MemoryDeckSource([deck])
    assert source.delete("mini")
    assert source.slugs() == []
    with pytest.raises(DeckNotFound):
        source.get("mini")


def test_a_read_only_source_refuses_a_delete(decks_root):
    deck = Deck.load(decks_root / "mini" / "deck.yaml")
    source = MemoryDeckSource([deck], writable=False)
    with pytest.raises(ReadOnlySource):
        source.delete("mini")
    assert source.slugs() == ["mini"], "and the deck is still there"

if __name__ == "__main__":
    sys.exit(pytest.main([__file__, "-q"]))
