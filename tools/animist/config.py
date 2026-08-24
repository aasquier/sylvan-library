"""Where the toolbox's data and decks live on disk.

This is the slice of the app's old `mtglab/config.py` that the media toolbox
actually reads, extracted when the Python app retired and only the local
picture tooling survived. The same two environment variables steer it, with
the same defaults, so the toolbox and the deployed app's own tooling point at
the same places without a word of coordination:

    MTGLAB_DATA_DIR    default "data"    pool, caches (animist originals,
                                         cardmotion derivatives, model weights)
    MTGLAB_DECKS_DIR   default "decks"   deck.yaml files, for `cardmotion sync`

Resolved once at import. Tests that need a different location should use
`use_paths()` rather than reassigning the module globals, so the derived
`DB_PATH` cannot drift out of step with `DATA_DIR`.

A `.env` file is loaded first, if one is present and python-dotenv is
installed. Kept from the original for the original's reason, one launcher
fewer: a maintainer whose `.env` points `MTGLAB_DATA_DIR` at a volume mount
would otherwise find this tool quietly writing a second cache tree beside the
real one. A file the process reads itself works regardless of how it was
started.

What was deliberately left behind: the app database, the Scryfall staging
directory, Forge's home, and every auth and mail setting -- this toolbox
fetches pictures and reads the card pool, and a config surface wider than
what it does is where the next stale claim comes from.
"""

from __future__ import annotations

import os
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path


def _load_dotenv() -> None:
    """Populate `os.environ` from a `.env`, without overriding what is set.

    Optional: python-dotenv rides with the `dev` extra, and a base install has
    no `.env` to read. The real environment wins over the file, so an exported
    variable is never silently shadowed by a stale one on disk.
    """
    try:
        from dotenv import find_dotenv, load_dotenv
    except ImportError:
        return
    # Search upward from the working directory, not from this file: installed
    # in site-packages, the package's own parents are the wrong tree.
    found = find_dotenv(usecwd=True)
    if found:
        load_dotenv(found, override=False)


_load_dotenv()

DATA_DIR: Path
DECKS_DIR: Path
DB_PATH: Path


def _read_env() -> tuple[Path, Path]:
    return (Path(os.environ.get("MTGLAB_DATA_DIR", "data")),
            Path(os.environ.get("MTGLAB_DECKS_DIR", "decks")))


def _apply(data_dir: Path, decks_dir: Path) -> None:
    global DATA_DIR, DECKS_DIR, DB_PATH
    DATA_DIR = data_dir
    DECKS_DIR = decks_dir
    # Derived, never set independently -- that is the whole reason this lives
    # behind a function instead of assignable globals.
    DB_PATH = data_dir / "mtg.duckdb"


_apply(*_read_env())


def reload_from_env() -> None:
    """Re-read the environment. For tests that set the variables directly."""
    _apply(*_read_env())


@contextmanager
def use_paths(*, data_dir: Path | str | None = None,
              decks_dir: Path | str | None = None) -> Iterator[None]:
    """Temporarily point the toolbox at other directories.

    Used by the tests to run real commands against a scratch tree without
    touching the repository's own data.
    """
    previous = (DATA_DIR, DECKS_DIR)
    _apply(Path(data_dir) if data_dir is not None else DATA_DIR,
           Path(decks_dir) if decks_dir is not None else DECKS_DIR)
    try:
        yield
    finally:
        _apply(*previous)


def deck_paths(decks_dir: Path | None = None) -> list[Path]:
    """Every real deck file. `_template` and any other underscore-prefixed
    directory is scaffolding, not a deck.

    Reads `DECKS_DIR` at call time rather than binding it as a default
    argument, so `use_paths()` actually takes effect.
    """
    root = Path(decks_dir) if decks_dir is not None else DECKS_DIR
    if not root.exists():
        return []
    return [p for p in sorted(root.glob("*/deck.yaml"))
            if not p.parent.name.startswith("_")]
