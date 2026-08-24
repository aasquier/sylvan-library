"""The card pool, read-only: the slice of `cards/db.py` the motion tier reads.

The app's pool module owned the whole lifecycle -- download, ingest, schema,
migration, price history. None of that survives into this toolbox, and that
is a decision rather than an omission: the pool is built and migrated by the
deployed app's own tooling, and a picture pipeline that could *create* an
empty `mtg.duckdb` would turn every path typo into a plausible-looking pool
with no cards in it. So `connect` here opens read-only, refuses a file that
is not there, and never runs a schema statement -- the same posture the app's
own serving tier took (`connect_readonly`), for the same stated reason: a
reader must degrade under a concurrent refresh, never lock it out.

Three symbols cross with it because `cardmotion/build.py` calls them, each
copied with its argument intact: the `Connection` alias, `printing_columns`
(how the derivation asks whether this pool predates the painter column), and
`art_crop_from` (the URL derivation that spares a 500MB re-ingest).
"""

from __future__ import annotations

import sys
from importlib import util
from pathlib import Path
from typing import Any, TypeAlias

#: A DuckDB connection.
#:
#: `Any`, and deliberately so: duckdb ships no type stubs, and it is imported
#: lazily inside `connect()` so importing this package costs nothing until a
#: build actually reaches for card facts. An alias rather than a bare `Any`
#: at each call site, so the intent reads as "the checker cannot see into
#: this library" rather than "nobody got round to annotating this" -- and so
#: there is one place to narrow if duckdb ever publishes stubs.
Connection: TypeAlias = Any


def _duckdb() -> Any:
    """`import duckdb`, with the pandas probe defused first.

    DuckDB's Python client asks `import pandas` **twice per bound parameter**
    on its way in, to decide whether a value is a DataFrame. Pandas is not a
    dependency of this project and never will be -- so every one of those
    asks is an `ImportError`, and an `ImportError` is not free: it walks the
    whole of `sys.path` and stats every entry before giving up. The app
    measured the cost at 162ms of a 200ms request; this toolbox binds fewer
    parameters, but the fix costs three lines and the mechanism is identical.

    A `None` in `sys.modules` is the documented way to say "this module is
    not here, stop looking" (CPython raises `ImportError` on the sentinel
    without touching the path). The probe still fails, so DuckDB takes
    exactly the same branch it always took -- it just stops paying for the
    answer. `find_spec` asks whether pandas is installed without importing
    it, so a machine that *does* have it is left completely alone, and the
    guard on `sys.modules` means a real import that got in first is never
    overwritten.
    """
    if "pandas" not in sys.modules and util.find_spec("pandas") is None:
        sys.modules["pandas"] = None  # type: ignore[assignment]

    import duckdb

    return duckdb


def connect(db_path: str | Path) -> Connection:
    """Open an existing pool read-only. A missing pool raises, loudly.

    Read-only is the whole contract -- see the module docstring. Raising on
    an absent file rather than creating one is the other half: the callers
    in `cardmotion/cli.py` already gate on the path existing and tell the
    user to refresh the pool, so a raise here is the belt for anybody
    calling the library directly.
    """
    duckdb = _duckdb()  # lazy, so importing the package stays free

    path = Path(db_path)
    if not path.exists():
        raise FileNotFoundError(
            f"no card pool at {path} -- this toolbox reads the pool and "
            "never creates one; build it with the app's own `data refresh`")
    return duckdb.connect(str(path), read_only=True)


def printing_columns(con: Connection) -> set[str]:
    """The columns `printings` has here.

    Needed because this toolbox never migrates a pool: `printings` gained its
    `artist` column on 2026-08-19, and a derivation naming it against a pool
    built the week before would not degrade, it would fail to bind.
    `resolve_subject` asks this instead and falls back to Scryfall for the
    credit when the answer is no.

    The app cached this per pool file because its serving tier asked on
    every card query; a cardmotion build asks once per card it derives, so
    the catalogue scan is paid where it is cheap and the cache stayed home.
    """
    rows = con.execute(
        "SELECT column_name FROM information_schema.columns "
        "WHERE table_name = ?", ["printings"]).fetchall()
    return {r[0] for r in rows}


def art_crop_from(image_normal: str | None) -> str | None:
    """The `art_crop` URL for a printing whose `normal` URL we have.

    The `printings` table stores `image_normal` and no crop, so a deck showing
    a chosen printing would have nothing to put in the hero band, which is a
    crop. Rather than adding a column and requiring a 500MB re-ingest before
    the feature works at all, the crop is derived: Scryfall's image URLs differ
    only in the size segment, which is checkable rather than assumed --
    `oracle_cards` stores both for the same printing id and they are identical
    but for `normal` / `art_crop`.

    Anything not matching that shape returns None rather than a guess, and the
    caller falls back to the full card image. A wrong URL renders as a broken
    image; None renders as the card.
    """
    if not image_normal or "/normal/" not in image_normal:
        return None
    return image_normal.replace("/normal/", "/art_crop/", 1)
