"""Where decks come from.

Today there is exactly one answer -- `decks/<slug>/deck.yaml` in git, which is
the source of truth (ADR 1). Tomorrow there are two: `docs/HOSTING.md` keeps the
curated decks file-backed *permanently* and adds other people's decks from
SQLite on the volume (ADR 4). So this protocol is not scaffolding for a
hypothetical; it is the shape the system ends up with, and introducing it while
there is one implementation means adding `SqlDeckSource` later is additive
rather than a change to every endpoint.

It pays for itself before then. The API previously reached for the filesystem in
three places, which made "what does the library endpoint do with an empty
library" a question you could only answer by creating directories. With a source
it is a two-line test.

**A DeckSource is a locator, not a connection.** It must stay valid after the
request that produced it has finished, because a background simulation job
captures one and outlives its request by minutes. A SQL-backed source should
therefore open and close a connection per call rather than holding one open.
"""

from __future__ import annotations

import shutil
from collections.abc import Iterable
from datetime import UTC, datetime
from pathlib import Path
from typing import Protocol, runtime_checkable

from mtglab import config
from mtglab.decks.model import Deck


class DeckNotFound(Exception):
    """Raised so the route layer can turn it into a 404 without guessing."""


@runtime_checkable
class DeckSource(Protocol):
    """The three questions the API actually asks about decks."""

    def slugs(self) -> list[str]:
        """Every deck's slug, without parsing any of them.

        Separate from `all()` because `/api/health` only wants a count, and
        parsing six YAML files to produce the number 6 is silly now and worse
        when the number is 600.
        """

    def get(self, slug: str) -> Deck:
        """One deck. Raises `DeckNotFound` if there is no such deck."""

    def all(self) -> list[Deck]:
        """Every deck, parsed, in a stable order."""

    def read_text(self, slug: str) -> str:
        """The deck's YAML, verbatim.

        Text rather than a parsed `Deck` because edits are surgical: rewriting
        a deck from its parsed form destroys comments and reflows every folded
        block, which turns a one-card swap into an unreadable diff. See
        `decks/edit.py`.
        """

    def write_text(self, slug: str, text: str) -> None:
        """Replace the deck's YAML. Raises `ReadOnlySource` if not permitted."""

    def create(self, slug: str, text: str) -> None:
        """Add a deck that does not exist yet.

        Separate from `write_text` rather than folded into it, because the two
        have opposite safety requirements: an update to a deck that has
        vanished is a bug, and a create over a deck that already exists
        destroys somebody's work. Raises `DeckExists` or `ReadOnlySource`.
        """

    def delete(self, slug: str) -> str:
        """Remove a deck, and say where it went.

        **Recoverably.** The return value is a location, not a boolean, because
        an implementation that cannot say where the deck went has destroyed it
        rather than removed it. The file-backed source moves the directory
        aside; a future SQL source would mark a row.

        This is the only operation here that can lose work nothing else
        recorded. A curated deck lives in git and `git checkout` is its undo,
        but a deck imported an hour ago and never committed has no other copy —
        which is exactly the deck somebody is most likely to delete by mistake.

        Raises `DeckNotFound` or `ReadOnlySource`.
        """

    @property
    def writable(self) -> bool:
        """Whether this caller may edit these decks.

        Always true today, with one local user. `docs/HOSTING.md` keeps the
        curated decks read-only for everyone but the maintainer, and this is
        the flag that will say so -- checked in one place rather than
        rediscovered per endpoint.
        """


class ReadOnlySource(Exception):
    """Raised when a deck source will not accept an edit."""


class DeckExists(Exception):
    """Raised rather than overwriting a deck that is already there."""


class FileDeckSource:
    """Decks as `<root>/<slug>/deck.yaml`."""

    def __init__(self, root: Path | str | None = None) -> None:
        # None means "ask config at call time", so `config.use_paths()` in a
        # test actually takes effect. Passing a root pins it explicitly.
        self._root = Path(root) if root is not None else None

    @property
    def root(self) -> Path:
        return self._root if self._root is not None else config.DECKS_DIR

    def slugs(self) -> list[str]:
        return [p.parent.name for p in config.deck_paths(self._root)]

    def get(self, slug: str) -> Deck:
        path = self.root / slug / "deck.yaml"
        if not path.exists():
            raise DeckNotFound(slug)
        return Deck.load(path)

    def all(self) -> list[Deck]:
        return [Deck.load(p) for p in config.deck_paths(self._root)]

    def _path(self, slug: str) -> Path:
        path = self.root / slug / "deck.yaml"
        if not path.exists():
            raise DeckNotFound(slug)
        return path

    def read_text(self, slug: str) -> str:
        return self._path(slug).read_text(encoding="utf-8")

    def write_text(self, slug: str, text: str) -> None:
        # Written whole rather than in place: a partial write would leave the
        # source of truth truncated, and the caller has already verified the
        # text parses.
        self._path(slug).write_text(text, encoding="utf-8")

    def create(self, slug: str, text: str) -> None:
        path = self.root / slug / "deck.yaml"
        if path.exists():
            raise DeckExists(slug)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def delete(self, slug: str) -> str:
        """Move the deck's whole directory into `.trash/`, and say where.

        A move rather than an unlink, for one reason: the deck this is most
        likely to be pointed at by mistake is a draft imported ten minutes ago
        and never committed, and for that deck there is no `git checkout` to
        undo with. Moving costs one rename and makes the mistake survivable.

        The directory goes whole — `artifacts/` with it — because the artifacts
        are generated from the deck file and a folder of primers for a deck
        that no longer exists is worse than no folder at all.

        `.trash` is dot-prefixed so `config.deck_paths` cannot see it: that
        glob is `*/deck.yaml`, and a trashed deck sits one level deeper. It is
        gitignored, so a deleted deck leaves the working tree without leaving a
        staged deletion nobody asked for.
        """
        path = self._path(slug)                      # raises DeckNotFound
        stamp = datetime.now(UTC).strftime("%Y%m%dT%H%M%SZ")
        trash = self.root / ".trash" / f"{slug}-{stamp}"
        trash.parent.mkdir(parents=True, exist_ok=True)
        # Import, delete, re-import, delete again inside one second is a real
        # sequence, and the stamp is only second-resolution. This matters more
        # than it looks: `shutil.move` onto an existing directory moves the
        # source *inside* it rather than failing, so a collision would bury the
        # earlier deletion instead of reporting anything.
        if trash.exists():
            n = 2
            while (candidate := trash.with_name(f"{trash.name}-{n}")).exists():
                n += 1
            trash = candidate
        # `path.parent`, not `path`: the deck is a directory, and leaving the
        # artifacts behind would strand them beside a deck that is gone.
        shutil.move(str(path.parent), str(trash))
        return str(trash)

    @property
    def writable(self) -> bool:
        return True

    def __repr__(self) -> str:
        return f"FileDeckSource({self.root})"


class MemoryDeckSource:
    """Decks held in a dict.

    Exists for tests, and as the proof that the protocol is implementable by
    something that is not a filesystem -- which is the whole claim being made
    about a future SQL source.
    """

    def __init__(self, decks: Iterable[Deck] = (), *, writable: bool = True) -> None:
        self._decks = {d.slug: d for d in decks}
        self._text: dict[str, str] = {}
        self._trash: dict[str, str] = {}
        self._writable = writable

    def slugs(self) -> list[str]:
        return sorted(self._decks)

    def get(self, slug: str) -> Deck:
        try:
            return self._decks[slug]
        except KeyError:
            raise DeckNotFound(slug) from None

    def all(self) -> list[Deck]:
        return [self._decks[slug] for slug in self.slugs()]

    def read_text(self, slug: str) -> str:
        if slug in self._text:
            return self._text[slug]
        return self.get(slug).dump()

    def write_text(self, slug: str, text: str) -> None:
        if not self._writable:
            raise ReadOnlySource(slug)
        self.get(slug)          # raises DeckNotFound for an unknown deck
        self._text[slug] = text
        self._decks[slug] = Deck.from_text(text, slug=slug)

    def create(self, slug: str, text: str) -> None:
        if not self._writable:
            raise ReadOnlySource(slug)
        if slug in self._decks:
            raise DeckExists(slug)
        self._text[slug] = text
        self._decks[slug] = Deck.from_text(text, slug=slug)

    def delete(self, slug: str) -> str:
        if not self._writable:
            raise ReadOnlySource(slug)
        self.get(slug)          # raises DeckNotFound for an unknown deck
        self._trash[slug] = self.read_text(slug)
        del self._decks[slug]
        self._text.pop(slug, None)
        # Kept rather than dropped, so this source models the same promise the
        # file-backed one makes: a delete is recoverable, and the return value
        # names where from.
        return f"memory:.trash/{slug}"

    @property
    def writable(self) -> bool:
        return self._writable

    def __repr__(self) -> str:
        return f"MemoryDeckSource({len(self._decks)} decks)"
