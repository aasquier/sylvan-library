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

from collections.abc import Iterable
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

    @property
    def writable(self) -> bool:
        return self._writable

    def __repr__(self) -> str:
        return f"MemoryDeckSource({len(self._decks)} decks)"
