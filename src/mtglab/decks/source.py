"""Where decks come from.

Today there is exactly one answer -- `decks/<slug>/deck.yaml` on disk, which is
the source of truth (ADR 1) and, since ADR 30, the app's live data rather than
anything git tracks. Tomorrow there are two: `docs/HOSTING.md` keeps the
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
from collections.abc import Iterable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Protocol, runtime_checkable

from mtglab import config
from mtglab.artifacts.generate import DELIVERABLES, SNAPSHOT, store
from mtglab.decks import edit
from mtglab.decks.model import Deck


class DeckNotFound(Exception):
    """Raised so the route layer can turn it into a 404 without guessing."""


#: Stands in for "built, but this source never recorded when" -- only
#: reachable if an implementation stores artifacts without a timestamp, which
#: neither of the two here does.
_EPOCH = datetime(1970, 1, 1, tzinfo=UTC)


class ArtifactNotFound(Exception):
    """Raised for a deliverable this deck has not got.

    Distinct from `DeckNotFound` because the two are different 404s to a
    reader: the deck may be perfectly real and simply never built, or built
    before it had anything to diff, which is the ordinary state of a first
    build and the reason `swaps.md` is so often absent.
    """


@dataclass(frozen=True)
class Artifact:
    """One generated deliverable, as the library holds it.

    `built_at` is the store's own timestamp rather than anything inside the
    file, so it answers the question a reader actually has -- *is this older
    than the deck?* On 2026-08-21 every artifact on the volume was eight days
    older than its `deck.yaml`, and nothing in the app could say so.
    """

    name: str
    size: int
    built_at: datetime


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
        recorded, and since ADR 30 that is true of *every* deck: none of them
        is in git, so none of them has a checkout to fall back on. The
        graveyard undoes an entombed card (ADR 27); nothing undoes a deleted
        deck but the directory this moves aside.

        Raises `DeckNotFound` or `ReadOnlySource`.
        """

    def set_shared(self, slug: str, shared: bool) -> None:
        """Put the deck on display to other accounts, or take it off (ADR 22).

        Its own operation rather than a `set_deck_field(field="shared")`,
        because the two tiers keep this fact in different places -- the file
        tier in `deck.yaml`, the SQL tier in a column it treats as the truth --
        and a caller should not have to know which. Raises `DeckNotFound` or
        `ReadOnlySource`.
        """

    def artifacts(self, slug: str) -> list[Artifact]:
        """Which deliverables this deck has, in `DELIVERABLES` order.

        Absence is normal and is not an error: a deck that has never been
        built has none, and one built with nothing to diff has no `swaps.md`.
        Raises `DeckNotFound` for a deck that is not there at all.
        """

    def read_artifact(self, slug: str, name: str) -> str:
        """One deliverable's text.

        `name` is whatever arrived in the URL, so an implementation must
        refuse anything outside `DELIVERABLES` *before* it resolves the name
        against storage -- which for the file tier is the whole path-traversal
        story, and for a SQL tier is a `WHERE` clause it would want anyway.
        Raises `ArtifactNotFound`, or `DeckNotFound` for an unknown deck.
        """

    def read_baseline(self, slug: str) -> str | None:
        """The last build's snapshot, or None if there has never been one.

        Its own method rather than `read_artifact(slug, SNAPSHOT)` because the
        baseline is not a deliverable and `DELIVERABLES` refuses it -- which is
        the point of that tuple. This is the build's own bookkeeping being read
        back by the next build, so it is internal on both ends.

        None is ordinary, not exceptional: a deck built for the first time has
        no baseline, and so gets no `swaps.md`. Decks built before the
        snapshot mechanism existed (ADR 30) are in the same position.
        """

    def write_artifacts(self, slug: str, files: Mapping[str, str]) -> list[str]:
        """Store a build's output, and say what was stored.

        Takes rendered text rather than a deck, because generating and storing
        are different jobs and only one of them is the source's (see
        `artifacts.generate.render_all`). The mapping is stored whole,
        including the snapshot -- `DELIVERABLES` governs what may be *read*,
        not what a build may write, and the baseline is part of the build.

        Names are returned rather than paths: a SQL tier has no paths, and no
        caller needs one. Raises `DeckNotFound` or `ReadOnlySource`.
        """

    @property
    def writable(self) -> bool:
        """Whether this caller may edit these decks.

        ~~Always true today, with one local user.~~ No longer: the moment a
        second account existed, "the curated library is the same six for
        everyone" stopped meaning "and everyone may edit them". `api/deps.py`
        derives this from the caller, and it is checked in one place rather
        than rediscovered per endpoint -- which is the whole reason it was put
        on the protocol before anything needed it.

        An implementation that reports `False` here must also *refuse*, by
        raising `ReadOnlySource` from the write operations. The flag is for
        callers that want to ask ahead (the UI, so it can hide a control);
        it is not the enforcement, because a flag nobody reads enforces
        nothing.
        """


class ReadOnlySource(Exception):
    """Raised when a deck source will not accept an edit."""


class DeckExists(Exception):
    """Raised rather than overwriting a deck that is already there."""


class FileDeckSource:
    """Decks as `<root>/<slug>/deck.yaml`.

    `writable` is a constructor argument rather than a hardcoded `True` because
    this library is now read by people who do not own it. Until the per-user
    tier exists there is exactly one library and it is the maintainer's, so
    "may this caller edit decks" and "is this caller the maintainer" are the
    same question -- but they are the same question only by accident of there
    being one library, and the flag is where they come apart later. See
    `api/deps.deck_source`, which is the only place that decides it.
    """

    def __init__(self, root: Path | str | None = None, *,
                 writable: bool = True) -> None:
        # None means "ask config at call time", so `config.use_paths()` in a
        # test actually takes effect. Passing a root pins it explicitly.
        self._root = Path(root) if root is not None else None
        # Defaults to True so every existing caller -- the CLI, the artifact
        # generator, the tests that build a source directly -- is unchanged.
        # The API is the one caller with somebody to be read-only *to*.
        self._writable = writable

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

    def _writable_or_raise(self, slug: str) -> None:
        """Refuse an edit before looking at the filesystem.

        Checked *first*, ahead of `DeckNotFound` and `DeckExists`, for two
        reasons. It is the cheaper answer, and more importantly it is the one
        that does not depend on state: a caller who may not write gets the same
        answer whether or not the deck is there, so no sequence of refused
        edits maps out the library.

        That mapping is not itself a secret here -- this library is deliberately
        readable by everyone, and `GET /api/decks` lists it -- but an
        authorisation check whose answer varies with the resource is the shape
        of a leak, and it costs nothing to not write one.
        """
        if not self._writable:
            raise ReadOnlySource(slug)

    def write_text(self, slug: str, text: str) -> None:
        self._writable_or_raise(slug)
        # Written whole rather than in place: a partial write would leave the
        # source of truth truncated, and the caller has already verified the
        # text parses.
        self._path(slug).write_text(text, encoding="utf-8")

    def create(self, slug: str, text: str) -> None:
        self._writable_or_raise(slug)
        path = self.root / slug / "deck.yaml"
        if path.exists():
            raise DeckExists(slug)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")

    def delete(self, slug: str) -> str:
        """Move the deck's whole directory into `.trash/`, and say where.

        A move rather than an unlink, for one reason: since ADR 30 no deck is
        in git, so no deck has a revision to restore from -- the draft imported
        ten minutes ago and the curated deck of a year's thinking are equally
        gone. Moving costs one rename and makes the mistake survivable.

        The directory goes whole — `artifacts/` with it — because the artifacts
        are generated from the deck file and a folder of primers for a deck
        that no longer exists is worse than no folder at all.

        `.trash` is dot-prefixed so `config.deck_paths` cannot see it: that
        glob is `*/deck.yaml`, and a trashed deck sits one level deeper. The
        whole tree is the app's data directory rather than anything tracked
        (ADR 30), so this is the only record that the deck was ever here.

        Refused outright on a read-only source, before anything moves. This is
        the operation the flag exists for: a delete here takes the artifacts
        with it, and nothing outside `.trash/` remembers the deck existed.
        """
        self._writable_or_raise(slug)
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

    def _artifact_dir(self, slug: str) -> Path:
        """`<slug>/artifacts/`, once the deck itself is known to exist.

        `_path` first, so a missing deck is `DeckNotFound` rather than an
        empty artifact list -- "this deck has never been built" and "there is
        no such deck" are answers a caller must be able to tell apart.
        """
        return self._path(slug).parent / "artifacts"

    def artifacts(self, slug: str) -> list[Artifact]:
        out = self._artifact_dir(slug)
        found = []
        for name in DELIVERABLES:
            path = out / name
            if not path.is_file():
                continue
            stat = path.stat()
            found.append(Artifact(
                name=name, size=stat.st_size,
                built_at=datetime.fromtimestamp(stat.st_mtime, UTC)))
        return found

    def read_artifact(self, slug: str, name: str) -> str:
        # Membership first, and it is the only check that matters: `name`
        # comes from a URL, and a name that is not one of the five never
        # becomes a path at all. No normalising, no `..` stripping, no
        # resolve-and-compare against the root -- all of which are ways of
        # being careful with a path this never constructs.
        if name not in DELIVERABLES:
            raise ArtifactNotFound(name)
        path = self._artifact_dir(slug) / name       # raises DeckNotFound
        if not path.is_file():
            raise ArtifactNotFound(name)
        return path.read_text(encoding="utf-8")

    def read_baseline(self, slug: str) -> str | None:
        path = self._artifact_dir(slug) / SNAPSHOT   # raises DeckNotFound
        if not path.is_file():
            return None
        return path.read_text(encoding="utf-8")

    def write_artifacts(self, slug: str, files: Mapping[str, str]) -> list[str]:
        self._writable_or_raise(slug)
        out = self._artifact_dir(slug)               # raises DeckNotFound
        # `generate.store` rather than a loop here, so this tier and
        # `mtglab decks build` cannot disagree about what a rebuild leaves
        # behind -- which they did until 2026-08-21, over a stale `swaps.md`.
        return [p.name for p in store(files, out)]

    def set_shared(self, slug: str, shared: bool) -> None:
        """Write `shared:` into the deck file. The YAML is this tier's truth.

        A surgical edit like every other write (ADR 12), through
        `edit.set_shared`. It was a `Deck.load` / `Deck.dump` round trip until
        2026-08-22, on the argument that `shared` is a single boolean with no
        prose attached -- true of the field, and irrelevant, because `dump`
        rewrites the whole file. One press of the deck page's share toggle
        took a hand-written deck's section banners, its trailing comments, its
        folded blocks and its `swap_board: []` with it. Found by the Go port,
        which had to reproduce the bytes and asked what they were.

        Unchanged: `dump` omits `shared` when true, so putting a deck back on
        display removes the key rather than asserting the default.
        """
        self._writable_or_raise(slug)
        path = self._path(slug)                      # raises DeckNotFound
        if Deck.load(path).shared == shared:
            return
        path.write_text(
            edit.set_shared(path.read_text(encoding="utf-8"), shared=shared),
            encoding="utf-8")

    @property
    def writable(self) -> bool:
        return self._writable

    def __repr__(self) -> str:
        mode = "rw" if self._writable else "ro"
        return f"FileDeckSource({self.root}, {mode})"


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
        self._artifacts: dict[str, dict[str, str]] = {}
        self._built: dict[str, datetime] = {}
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
        # The file tier moves the directory whole, artifacts included; this
        # tier has to say so itself or the two would disagree about what a
        # deleted deck leaves behind.
        self._artifacts.pop(slug, None)
        self._built.pop(slug, None)
        # Kept rather than dropped, so this source models the same promise the
        # file-backed one makes: a delete is recoverable, and the return value
        # names where from.
        return f"memory:.trash/{slug}"

    def artifacts(self, slug: str) -> list[Artifact]:
        self.get(slug)          # raises DeckNotFound for an unknown deck
        held = self._artifacts.get(slug, {})
        return [Artifact(name=n, size=len(held[n].encode("utf-8")),
                         built_at=self._built.get(slug, _EPOCH))
                for n in DELIVERABLES if n in held]

    def read_artifact(self, slug: str, name: str) -> str:
        if name not in DELIVERABLES:
            raise ArtifactNotFound(name)
        self.get(slug)          # raises DeckNotFound for an unknown deck
        try:
            return self._artifacts[slug][name]
        except KeyError:
            raise ArtifactNotFound(name) from None

    def read_baseline(self, slug: str) -> str | None:
        self.get(slug)          # raises DeckNotFound for an unknown deck
        return self._artifacts.get(slug, {}).get(SNAPSHOT)

    def write_artifacts(self, slug: str, files: Mapping[str, str]) -> list[str]:
        if not self._writable:
            raise ReadOnlySource(slug)
        self.get(slug)          # raises DeckNotFound for an unknown deck
        # Replaced rather than merged, which is what a rebuild does on disk
        # too: a `swaps.md` from an older build is not part of this one.
        self._artifacts[slug] = dict(files)
        self._built[slug] = datetime.now(UTC)
        return list(files)

    def set_shared(self, slug: str, shared: bool) -> None:
        if not self._writable:
            raise ReadOnlySource(slug)
        self.get(slug).shared = shared

    @property
    def writable(self) -> bool:
        return self._writable

    def __repr__(self) -> str:
        return f"MemoryDeckSource({len(self._decks)} decks)"
