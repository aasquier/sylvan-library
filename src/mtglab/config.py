"""Where things live on disk.

These were module-level constants in `cli.py`, which caused two problems.

The web API had to reach into the CLI to find them (`from mtglab.cli import
DB_PATH, DECKS_DIR`), so the HTTP layer depended on the command-line layer --
backwards, and the reason `cli.py` sat at 27% test coverage: there was no way
to point a test at a temporary deck directory.

And they were *relative* paths resolved against the process working directory,
so a container could not put the corpus on a mounted volume. `docs/HOSTING.md`
lists this as the first prerequisite for deploying anything.

Both are read from the environment now, defaulting to exactly what they were,
so local use is unchanged:

    MTGLAB_DATA_DIR    default "data"    corpus, price history
    MTGLAB_DECKS_DIR   default "decks"   deck.yaml source of truth

Resolved once at import. Tests that need a different location should use
`use_paths()` rather than reassigning the module globals, so the derived
`DB_PATH` cannot drift out of step with `DATA_DIR`.
"""

from __future__ import annotations

import os
from collections.abc import Iterator
from contextlib import contextmanager
from pathlib import Path

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
    # behind a function instead of three assignable globals.
    DB_PATH = data_dir / "mtg.duckdb"


_apply(*_read_env())


def reload_from_env() -> None:
    """Re-read the environment. For tests that set the variables directly."""
    _apply(*_read_env())


@contextmanager
def use_paths(*, data_dir: Path | str | None = None,
              decks_dir: Path | str | None = None) -> Iterator[None]:
    """Temporarily point mtglab at other directories.

    Used by the CLI tests to run real commands against a scratch deck
    directory without touching the repository's own decks.
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
