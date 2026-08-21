"""One person's decks, as rows in `app.db` (ADR 4's second tier, ADR 22).

The curated six are file-backed permanently (on disk, not in git -- ADR 30)
and are not in here. This is
where everybody else's decks live, and the only reason it can exist at all is
the property ADR 4 bought: `deck.yaml`'s text is what a row stores, so
`Deck.from_text` parses both tiers and the gate, the compiler and the artifact
generator never learn there are two.

**An instance of this class is one owner's library**, not the table. That is
what keeps it a `DeckSource` — slug-keyed, exactly like `FileDeckSource` — and
it is why the owner segment in ADR 22's paths resolves to a *source* rather
than to an argument threaded through nineteen handlers. `Library` does the
resolving.

Like every source this is **a locator, not a connection** (`source.py`): a
background job captures one and outlives the request that made it by minutes,
so every method opens and closes its own connection rather than holding one.

`auth/db.py` is imported for `connect()` and for nothing to do with auth — it
is the `app.db` connection helper and its migration ladder. The same import
`sim/cache.py` makes, for the same reason: a second ladder over one file would
be worse than the dependency.
"""

from __future__ import annotations

import sqlite3
from collections.abc import Mapping
from datetime import UTC, datetime
from typing import cast

from mtglab.artifacts.generate import DELIVERABLES, SNAPSHOT
from mtglab.auth import db
from mtglab.decks.model import Deck
from mtglab.decks.source import (
    Artifact,
    ArtifactNotFound,
    DeckExists,
    DeckNotFound,
    ReadOnlySource,
)


def _now() -> str:
    return datetime.now(UTC).isoformat(timespec="seconds")


class SqlDeckSource:
    """The decks belonging to one account.

    `writable` is the caller's relationship to that account, decided in
    `Library` and never here: a source does not know who is asking, only what
    it was told they may do. That is the split `FileDeckSource` already had and
    #80 established.
    """

    def __init__(self, owner_id: int, *, writable: bool = True,
                 shared_only: bool = False) -> None:
        self._owner_id = owner_id
        self._writable = writable
        # Set when the caller is somebody else: their private decks are not
        # merely unwritable, they are not there at all. Applied in the WHERE
        # clause rather than by filtering afterwards, so a private deck cannot
        # be leaked by a code path that forgot to filter -- the row never
        # arrives. ADR 22's 404 is this line.
        self._shared_only = shared_only

    @property
    def owner_id(self) -> int:
        """Whose decks these are, as a `users.id`.

        Exposed for the activity log (ADR 28), which keys an entry on the
        library rather than on the owner segment out of the URL -- that segment
        is `local` on a laptop and a username on a deployment, and a history
        keyed on it would split in two the day `MTGLAB_ADMIN_EMAIL` was set.
        The file tier has no id and answers `None` by not having this at all,
        which is exactly what it means: there is one of it per instance.
        """
        return self._owner_id

    # ---- reading ---------------------------------------------------------

    def _where(self) -> tuple[str, list[object]]:
        sql = "owner_id = ? AND deleted_at IS NULL"
        args: list[object] = [self._owner_id]
        if self._shared_only:
            sql += " AND shared = 1"
        return sql, args

    def slugs(self) -> list[str]:
        where, args = self._where()
        with db.connection() as con:
            rows = con.execute(
                f"SELECT slug FROM user_decks WHERE {where} ORDER BY slug",
                args).fetchall()
        return [r[0] for r in rows]

    def _row(self, con: sqlite3.Connection, slug: str) -> sqlite3.Row:
        where, args = self._where()
        row = con.execute(
            f"SELECT * FROM user_decks WHERE {where} AND slug = ?",
            [*args, slug]).fetchone()
        if row is None:
            raise DeckNotFound(slug)
        # `fetchone` is typed `Any`; `connect()` sets `row_factory` to
        # `sqlite3.Row`, which is what makes the string indexing below valid.
        return cast(sqlite3.Row, row)

    def get(self, slug: str) -> Deck:
        with db.connection() as con:
            row = self._row(con, slug)
        return self._parse(row)

    def all(self) -> list[Deck]:
        where, args = self._where()
        with db.connection() as con:
            rows = con.execute(
                f"SELECT * FROM user_decks WHERE {where} ORDER BY slug",
                args).fetchall()
        return [self._parse(r) for r in rows]

    def read_text(self, slug: str) -> str:
        with db.connection() as con:
            return str(self._row(con, slug)["yaml"])

    @staticmethod
    def _parse(row: sqlite3.Row) -> Deck:
        # `slug=` is the row's, not the YAML's, for the same reason
        # `Deck.load` passes the directory name: the location is the identity,
        # and a YAML body whose `slug:` disagrees must not be able to rename
        # somebody's deck by being saved.
        deck = Deck.from_text(str(row["yaml"]), slug=str(row["slug"]))
        # **The column is the truth for this tier**, and the field on the
        # parsed deck is overwritten from it rather than the other way round.
        # One truth each: `deck.yaml` carries `shared` for the file tier, this
        # column carries it here. That is what lets the two tiers hold
        # opposite defaults (ADR 22) without a deck ever disagreeing with
        # itself -- a row created private stays private no matter what YAML is
        # later written over it by a card edit.
        deck.shared = bool(row["shared"])
        return deck

    # ---- writing ---------------------------------------------------------

    @property
    def hides_decks(self) -> bool:
        """Whether this source conceals some of what it holds.

        True for the `shared_only` view, and it is what tells `service.py` not
        to answer 403 before a slug has been resolved — for a deck this view
        hides, the answer has to be 404. See `_writable_or_raise`.
        """
        return self._shared_only

    def _writable_or_raise(self, slug: str) -> None:
        """Refuse a write, in the order ADR 22 requires.

        **Visibility first when this view hides things.** For a `shared_only`
        source a deck the caller cannot see must answer 404, so `_row` runs
        before the writability check and raises `DeckNotFound` — otherwise a
        403 here would confirm somebody's private deck exists, which is the
        leak ADR 5 exists to prevent and the reason this is not simply #80's
        ordering.

        When the source hides nothing, #80's ordering stands: refuse before
        touching the database, so no sequence of refused edits maps out a
        library and a delete cannot fail after the row has moved.
        """
        if self._shared_only:
            with db.connection() as con:
                self._row(con, slug)         # raises DeckNotFound if hidden
        if not self._writable:
            raise ReadOnlySource(slug)

    def _write(self, con: sqlite3.Connection, slug: str, text: str,
               *, creating: bool) -> None:
        """Store the text and re-derive the column that summarises it.

        `name` is a denormalised copy of what the YAML says, so the library
        list does not parse every row to render a title. Re-derived here on
        every write rather than accepted from a caller, which is what stops it
        drifting from the text it describes.

        **`shared` is deliberately not touched on an update.** It is this
        tier's own truth (see `_parse`) and it is changed by `set_shared` and
        nothing else — otherwise editing a card's rationale would silently
        republish, or silently hide, the deck it belongs to.
        """
        deck = Deck.from_text(text, slug=slug)
        now = _now()
        if creating:
            # Private. ADR 22's default for this tier, and the argument is
            # `decks import`: it writes a draft with an empty `why` on all 99
            # cards, and publishing that the instant it exists is nobody's
            # intent. `set_shared` is how it goes on display.
            con.execute(
                "INSERT INTO user_decks "
                "(owner_id, slug, name, yaml, shared, created_at, updated_at) "
                "VALUES (?, ?, ?, ?, 0, ?, ?)",
                (self._owner_id, slug, deck.name, text, now, now))
        else:
            con.execute(
                "UPDATE user_decks SET yaml = ?, name = ?, updated_at = ? "
                "WHERE owner_id = ? AND slug = ? AND deleted_at IS NULL",
                (text, deck.name, now, self._owner_id, slug))

    # ------------------------------------------------------------ artifacts
    #
    # The file tier's `<slug>/artifacts/` directory, as rows. Everything about
    # what a deliverable *is* stays in `artifacts/generate.py`; this only
    # stores and returns text, which is the same division `yaml` already has.

    def artifacts(self, slug: str) -> list[Artifact]:
        with db.connection() as con:
            row = self._row(con, slug)      # raises DeckNotFound
            held = {
                str(r["name"]): r for r in con.execute(
                    "SELECT name, LENGTH(CAST(body AS BLOB)) AS size, built_at "
                    "FROM user_deck_artifacts WHERE deck_id = ?", (row["id"],))
            }
        # Ordered here rather than in SQL: `DELIVERABLES` is the reader's
        # order and the database has no reason to know it.
        return [Artifact(name=n, size=int(held[n]["size"]),
                         built_at=datetime.fromisoformat(str(held[n]["built_at"])))
                for n in DELIVERABLES if n in held]

    def read_artifact(self, slug: str, name: str) -> str:
        # Membership before the query, exactly as the file tier checks it
        # before building a path: one whitelist, enforced per tier at the same
        # point in the call.
        if name not in DELIVERABLES:
            raise ArtifactNotFound(name)
        return self._body(slug, name)

    def read_baseline(self, slug: str) -> str | None:
        try:
            return self._body(slug, SNAPSHOT)
        except ArtifactNotFound:
            return None

    def _body(self, slug: str, name: str) -> str:
        with db.connection() as con:
            row = self._row(con, slug)      # raises DeckNotFound
            got = con.execute(
                "SELECT body FROM user_deck_artifacts "
                "WHERE deck_id = ? AND name = ?", (row["id"], name)).fetchone()
        if got is None:
            raise ArtifactNotFound(name)
        return str(got["body"])

    def write_artifacts(self, slug: str, files: Mapping[str, str]) -> list[str]:
        self._writable_or_raise(slug)
        now = _now()
        with db.connection() as con:
            row = self._row(con, slug)      # raises DeckNotFound
            # Deleted then inserted, in one transaction, so a rebuild leaves
            # exactly what this build produced -- the same pruning
            # `generate.store` does to the directory, and for the same reason:
            # a `swaps.md` from an older build describes a diff that no longer
            # exists. Only `DELIVERABLES` are swept, so the snapshot survives
            # a build that writes no swap list, as it must.
            con.executemany(
                "DELETE FROM user_deck_artifacts WHERE deck_id = ? AND name = ?",
                [(row["id"], n) for n in DELIVERABLES if n not in files])
            con.executemany(
                "INSERT INTO user_deck_artifacts (deck_id, name, body, built_at) "
                "VALUES (?, ?, ?, ?) ON CONFLICT(deck_id, name) DO UPDATE SET "
                "body = excluded.body, built_at = excluded.built_at",
                [(row["id"], n, body, now) for n, body in files.items()])
            # `db.connection()` closes but does not commit, so every write in
            # this module says so itself. Omitting it does not fail -- it
            # discards, which reads exactly like a build that produced nothing.
            con.commit()
        return list(files)

    def set_shared(self, slug: str, shared: bool) -> None:
        """Put the deck on display, or take it off. The owner's call alone."""
        self._writable_or_raise(slug)
        with db.connection() as con:
            row = self._row(con, slug)      # raises DeckNotFound
            con.execute(
                "UPDATE user_decks SET shared = ?, updated_at = ? WHERE id = ?",
                (1 if shared else 0, _now(), row["id"]))
            con.commit()

    def write_text(self, slug: str, text: str) -> None:
        self._writable_or_raise(slug)
        with db.connection() as con:
            self._row(con, slug)            # raises DeckNotFound
            self._write(con, slug, text, creating=False)
            con.commit()

    def create(self, slug: str, text: str) -> None:
        self._writable_or_raise(slug)
        with db.connection() as con:
            existing = con.execute(
                "SELECT 1 FROM user_decks "
                "WHERE owner_id = ? AND slug = ? AND deleted_at IS NULL",
                (self._owner_id, slug)).fetchone()
            if existing is not None:
                raise DeckExists(slug)
            try:
                self._write(con, slug, text, creating=True)
            except sqlite3.IntegrityError as exc:
                # The partial unique index is the real guard; the SELECT above
                # is the friendly one. Two creates racing for the same slug
                # would both pass the SELECT and one would land here.
                raise DeckExists(slug) from exc
            con.commit()

    def delete(self, slug: str) -> str:
        """Mark the row and say where it went.

        A mark rather than a `DELETE`, which is the protocol's requirement and
        not a preference: `delete` returns a location because an implementation
        that cannot say where the deck went has destroyed it rather than
        removed it. No deck in either tier has a revision behind it (ADR 30),
        so the mark is the only thing standing between a misclick and the work.
        """
        self._writable_or_raise(slug)
        with db.connection() as con:
            row = self._row(con, slug)      # raises DeckNotFound
            con.execute("UPDATE user_decks SET deleted_at = ? WHERE id = ?",
                        (_now(), row["id"]))
            con.commit()
        return f"user_decks:{row['id']}"

    @property
    def writable(self) -> bool:
        return self._writable

    def __repr__(self) -> str:
        mode = "rw" if self._writable else "ro"
        view = ", shared-only" if self._shared_only else ""
        return f"SqlDeckSource(owner={self._owner_id}, {mode}{view})"


def shared_decks() -> list[tuple[str, str, str]]:
    """Every shared deck in the SQL tier as `(username, slug, name)`.

    The browse tab's query, and it is here rather than on a source because it
    deliberately crosses owners -- a `DeckSource` is one person's library and
    this is the list of everybody's. Ordered by owner so the grouping the tab
    renders is the order it arrives in.

    The curated six are not in this table and are added by `Library`, which is
    the only place that knows the file tier belongs to the maintainer.

    **Only ever assembles what is already shared**, so no caller can turn this
    into a listing of private decks by forgetting a filter.
    """
    with db.connection() as con:
        rows = con.execute(
            "SELECT u.username, d.slug, d.name FROM user_decks d "
            "JOIN users u ON u.id = d.owner_id "
            "WHERE d.shared = 1 AND d.deleted_at IS NULL "
            "ORDER BY u.username COLLATE NOCASE, d.slug").fetchall()
    return [(str(r[0]), str(r[1]), str(r[2])) for r in rows]
