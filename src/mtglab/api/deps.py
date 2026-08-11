"""Request scope: what the caller of this request is allowed to see.

Nothing in the app models "who is asking", because today there is exactly one
asker. That is fine right up until it isn't, and the cost of fixing it later is
proportional to how many handlers have to change.

So there is one dependency, and it returns a `DeckSource`. Right now it always
returns the curated file-backed library. When auth arrives, it reads the session
and returns a source that unions the curated decks with that user's -- and no
handler changes. `docs/HOSTING.md` §1 requires every user-scoped query to go
through a single accessor, on the grounds that isolation fails when the filter
is sprinkled across handlers and one is forgotten. This is that accessor, built
before it has anything to isolate, which is the only time it is cheap.

Deliberately not here: anything about users. There is no session, no user table
and no auth, and inventing a `UserScope` with one field would be guessing at a
shape rather than preparing for it.
"""

from __future__ import annotations

from mtglab.decks.source import DeckSource, FileDeckSource


def deck_source() -> DeckSource:
    """The decks this request may see."""
    return FileDeckSource()
