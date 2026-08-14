"""Which decks a caller may see, across both tiers (ADR 22).

`DeckSource` is **one person's library** — slug-keyed, and it has been since it
was written. That did not have to change when decks acquired owners, and the
reason is worth stating: a `Library` resolves the owner segment of
`/api/decks/{owner}/{slug}` to a *source*, so the nineteen handlers keep asking
a source for a slug and never learn that ownership exists.

**The visibility rule is enforced by which source you get, not by branching in
a handler.** That is the same bet `api/deps.py` made and #80 collected on: a
check written once cannot be forgotten by the route somebody adds in a year.

    caller is the owner        -> their tier, writable
    caller is somebody else    -> their tier, shared decks only, read-only
    no such account            -> nothing at all

The middle line is what produces ADR 22's two answers, and the ordering is the
whole subtlety. A private deck is **absent** from the source, so every verb
against it raises `DeckNotFound` -> 404, writes included, before writability is
ever consulted. A shared deck is present but read-only, so a write raises
`ReadOnlySource` -> 403. Hence the rule:

> **403 is only ever an answer about a deck the caller can already read.**

`ReadOnlySource` -> 403 and `DeckNotFound` -> 404 are already wired in
`api/app.py`, so neither status code is written here. This module only decides
which exception the source will raise.
"""

from __future__ import annotations

from dataclasses import dataclass

from mtglab.auth import db, users
from mtglab.decks.model import Deck
from mtglab.decks.source import DeckNotFound, DeckSource, FileDeckSource
from mtglab.decks.sqlsource import SqlDeckSource, shared_decks

# The owner segment for a laptop with auth off. There is one person, they own
# everything they can see, and the URL still has to say so. Not a username in
# `users` -- with auth off there is no `app.db` account at all, which is the
# point of that mode.
LOCAL_OWNER = "local"


@dataclass(frozen=True)
class DeckRef:
    """A deck's full address: whose it is, and which one."""

    owner: str
    slug: str


class Library:
    """Every deck this caller may reach, in both tiers.

    Built per request from the scope (`api/deps.library`). Holds no connection
    — every source it hands out opens its own, because a background job
    captures one and outlives the request (`decks/source.py`).
    """

    def __init__(self, *, username: str | None, user_id: int | None,
                 maintainer: str | None, authenticated: bool,
                 is_admin: bool = False) -> None:
        self._username = username
        self._user_id = user_id
        # Whose the file-backed curated six are. ADR 22 makes this a rule
        # rather than an `owner:` field in six deck files: that field would put
        # one instance's account model into the portable half of the project
        # and would mean nothing on a laptop.
        self._maintainer = maintainer
        self._authenticated = authenticated
        self._is_admin = is_admin

    @property
    def _file_owner(self) -> str:
        """The owner segment the curated six sit under.

        `MTGLAB_ADMIN_EMAIL` names the maintainer and that is the answer
        whenever it is set — which it is on any real deployment. Unset, the
        fallback is `local` and **the six stay visible**, because the
        alternative is that an instance with auth on and no admin address
        configured silently serves an empty library: nobody would own the
        showcase, so nobody could see it. Writability then falls back to
        `is_admin`, which is exactly what #80 did before owners existed.
        """
        return self._maintainer or LOCAL_OWNER

    # ---- resolving -------------------------------------------------------

    def _is_me(self, owner: str) -> bool:
        if self._username is None:
            return False
        return owner.casefold() == self._username.casefold()

    def _is_maintainer(self, owner: str) -> bool:
        if self._maintainer is None:
            return False
        return owner.casefold() == self._maintainer.casefold()

    def source_for(self, owner: str) -> DeckSource:
        """The decks of `owner`, as this caller may see them.

        Raises `DeckNotFound` for an account that does not exist — **not a
        distinct error**, and deliberately. If an unknown owner answered
        differently from a known one with nothing shared, the owner segment
        would enumerate the account list, which is exactly the leak ADR 5
        exists to prevent. One answer for "no such person" and "nothing of
        theirs for you": 404.
        """
        mine = self._is_me(owner)

        # The file tier, under whichever segment owns it here.
        if owner.casefold() == self._file_owner.casefold():
            # Writable by its owner; by an admin when no maintainer is
            # configured and the six are therefore nobody's in particular.
            if mine or not self._authenticated or (
                    self._maintainer is None and self._is_admin):
                return FileDeckSource(writable=True)
            return _SharedOnly(FileDeckSource(writable=False))

        # The SQL tier: somebody's own decks.
        owner_id = self._owner_id(owner)
        if owner_id is None:
            raise DeckNotFound(owner)
        if mine:
            return SqlDeckSource(owner_id, writable=True)
        return SqlDeckSource(owner_id, writable=False, shared_only=True)

    @staticmethod
    def _owner_id(owner: str) -> int | None:
        with db.connection() as con:
            account = users.get(con, owner)
        return account.id if account is not None else None

    def mine(self) -> DeckSource:
        """The caller's own decks, whichever tier they are in.

        What the create and import routes write into, and what "my decks"
        renders. A caller with no account at all (auth off) gets the file tier,
        which is the library on their laptop.
        """
        if not self._authenticated:
            return FileDeckSource(writable=True)
        if self._username is not None and self._is_maintainer(self._username):
            return FileDeckSource(writable=True)
        if self._user_id is None:              # pragma: no cover - defensive
            raise DeckNotFound("no account")
        return SqlDeckSource(self._user_id, writable=True)

    @property
    def my_owner(self) -> str:
        """The owner segment the caller's own decks live under."""
        if not self._authenticated:
            return LOCAL_OWNER
        return self._username or LOCAL_OWNER

    @property
    def file_owner(self) -> str:
        """Who the curated six are filed under. See `_file_owner`."""
        return self._file_owner

    # ---- browsing --------------------------------------------------------

    def visible(self) -> list[tuple[str, DeckSource]]:
        """Every library this caller may read, as `(owner, source)`.

        Owner-first because that is how it is shown: ADR 22's browse tab groups
        other people's decks by username, and the path shape already names the
        owner, so the grouping is the order this returns.

        The caller's own library comes first and is writable; the maintainer's
        is next when it is not already theirs, because it is the showcase; then
        everybody else's shared decks, alphabetically.
        """
        out: list[tuple[str, DeckSource]] = []
        seen: set[str] = set()

        def add(owner: str, source: DeckSource) -> None:
            key = owner.casefold()
            if key not in seen:
                seen.add(key)
                out.append((owner, source))

        add(self.my_owner, self.mine())
        # The curated six, always — they are the showcase and ADR 22 makes
        # them shared by default. `_file_owner` falls back to `local`, so an
        # instance with no maintainer configured still shows its library
        # rather than an empty shelf.
        add(self._file_owner, self.source_for(self._file_owner))

        # **Nothing below this line may run with auth off.** There are no
        # accounts to share with, and `shared_decks()` would open `app.db` --
        # which would create it. `mtglab ui` on a laptop has no accounts at
        # all and must not acquire a database as a side effect of listing
        # decks, which is the same rule `auth/bootstrap.py` states and
        # `test_the_local_app_touches_no_database` pins.
        if not self._authenticated:
            return out

        for username, _slug, _name in shared_decks():
            if username.casefold() in seen:
                continue
            try:
                add(username, self.source_for(username))
            except DeckNotFound:                # pragma: no cover - defensive
                continue
        return out


class _SharedOnly:
    """A read-only view hiding whatever its inner source does not share.

    Wraps the *file* tier only. `SqlDeckSource` filters in its own `WHERE`
    clause, which is strictly better — the row never arrives — but the file
    tier has no query to filter, so hiding happens here.

    Never writable, and it does not merely report so: every write raises
    through the inner source, which was constructed read-only. A flag nobody
    reads enforces nothing (`source.py`).
    """

    def __init__(self, inner: DeckSource) -> None:
        self._inner = inner

    @property
    def hides_decks(self) -> bool:
        """This view is the definition of one. See `service._for_writing`."""
        return True

    def _visible(self, slug: str) -> None:
        """Raise `DeckNotFound` for a deck this view is hiding.

        The same exception an absent deck raises, which is the point: to this
        caller the two are the same fact.
        """
        if not self._inner.get(slug).shared:
            raise DeckNotFound(slug)

    def slugs(self) -> list[str]:
        return [d.slug for d in self.all()]

    def all(self) -> list[Deck]:
        return [d for d in self._inner.all() if d.shared]

    def get(self, slug: str) -> Deck:
        self._visible(slug)
        return self._inner.get(slug)

    def read_text(self, slug: str) -> str:
        self._visible(slug)
        return self._inner.read_text(slug)

    def write_text(self, slug: str, text: str) -> None:
        self._visible(slug)
        self._inner.write_text(slug, text)

    def create(self, slug: str, text: str) -> None:
        self._inner.create(slug, text)

    def delete(self, slug: str) -> str:
        self._visible(slug)
        return self._inner.delete(slug)

    def set_shared(self, slug: str, shared: bool) -> None:
        self._visible(slug)
        self._inner.set_shared(slug, shared)

    @property
    def writable(self) -> bool:
        return False

    def __repr__(self) -> str:
        return f"_SharedOnly({self._inner!r})"
