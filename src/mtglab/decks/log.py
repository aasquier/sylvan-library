"""What happened to a deck, and who did it (ADR 28).

`deck.yaml` is the source of truth and git is its history -- for the curated
six. For every other deck there is no history at all: ADR 22's tier is a row in
`app.db`, edited through the app, and the only record that a card was ever
entombed is that it is now in the graveyard. This module is the missing half.

ADR 15 lists it as a prerequisite rather than a nicety, and the sentence it
uses is the whole specification: *"what did it change while I was not
looking"*. Nothing autonomous writes a deck today, and the `actor` column is
here anyway, because a log that cannot say who is a log that has to be
migrated before it can answer the question it was built for.

Modelled on `claude/ledger.py`, which solved the same problems for the same
file, and the three properties carry over unchanged:

* **`record` never raises.** A history that can fail an edit is worse than no
  history: the deck write has already happened by the time this is called, so
  an exception here would report a failure for work that succeeded. A failed
  write is a logged warning.
* **`mtglab.auth.db` is imported for its connection helper, not for auth** --
  that module owns `app.db`'s pragmas and migration ladder, and a second ladder
  for the same file would be worse than the import.
* **One call site.** `service._commit` is the single place every deck write
  already passes through, so "an edit nobody logged" is not something a new
  route can produce by forgetting.

Two things are deliberately *not* like the ledger, and both follow from this
being per-deck rather than aggregate.

**It is addressed, so ADR 17's who-may-read-what question does have to be
answered.** It is answered by where the route is mounted rather than by a check
of its own: `GET /api/decks/{owner}/{slug}/log` resolves its source through
`Library`, so a deck you cannot read 404s before the log is consulted, and a
deck you *can* read shows you a history whose every actor is its owner --
whose name you already had, from the URL you typed to get there.

**No rationale text ever lands here.** `describe` builds its sentence out of
card names, categories and field *names*; where `swap_card` hands `_commit` the
`why` the user typed, it is dropped. The log records that a rationale changed
and never what it says. Rule 4's text lives in `deck.yaml`; a second copy in a
table nobody edits would go stale, and would be a place a rationale could be
read back out of by something that is not allowed to write one.
"""

from __future__ import annotations

import logging
import sqlite3
from collections.abc import Mapping
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

_LOG = logging.getLogger("mtglab.decks.log")

#: How many entries a reader gets when it does not say. A deck's history is
#: read as a panel beside the deck rather than as an archive, and fifty is
#: several months of a deck somebody is actively working on.
DEFAULT_LIMIT = 50

#: The field names `set_card_field` and `set_deck_field` accept, in the words
#: a sentence wants. Anything absent falls through as itself, so a field added
#: to the editor reads acceptably here before anybody remembers this table.
_FIELD_WORDS = {
    "why": "rationale",
    "qty": "quantity",
    "commander_art": "commander art",
}


def _now() -> str:
    return datetime.now(UTC).isoformat()


def _plural(n: int, word: str) -> str:
    return f"{n} {word}" if n == 1 else f"{n} {word}s"


def describe(extra: Mapping[str, Any]) -> tuple[str, str]:
    """Turn `_commit`'s per-operation keywords into a verb and a sentence.

    `_commit(**extra)` has always assembled exactly the description an entry
    wants -- `added=..., category=..., into=...` for one operation,
    `swapped_out=..., swapped_in=...` for another -- and then put it in a
    response and forgotten it. This is that description, kept.

    Two rules. The sentence is rendered **here**, once, rather than at read
    time: the CLI and the deck panel would otherwise be two renderers of the
    same row in two languages, and they would drift. And the verb is returned
    beside it so the row stays queryable without parsing prose.

    The fallback is `('edit', 'edited the deck')` rather than nothing, which is
    the load-bearing case: an operation whose shape this function has never
    seen is an operation somebody added, and the log must say it happened even
    when it cannot say what it was. Silence would be the one failure mode a
    history cannot have.
    """
    if "added" in extra:
        where = (" on the swap board" if extra.get("into") == "swap_board"
                 else "")
        category = extra.get("category")
        of = f" as {category}" if category else ""
        return "add", f"added {extra['added']}{of}{where}"

    if "entombed" in extra:
        names = extra["entombed"]
        if isinstance(names, list):
            # The bulk sweep. Named in full up to a handful, because "entombed
            # 12 cards" is the entry somebody comes to this panel to expand.
            if len(names) > 6:
                head = ", ".join(str(n) for n in names[:6])
                return "entomb", (f"entombed {_plural(len(names), 'card')}: "
                                  f"{head}, and {len(names) - 6} more")
            return "entomb", f"entombed {', '.join(str(n) for n in names)}"
        return "entomb", f"entombed {names}"

    if "removed" in extra:
        return "remove", f"removed {extra['removed']} from the swap board"

    if "returned" in extra:
        return "return", f"returned {extra['returned']} from the graveyard"

    if "exiled" in extra:
        return "exile", f"exiled {extra['exiled']} from the graveyard"

    if "swapped_out" in extra:
        # `why` is in this operation's keywords and is the one thing not
        # carried across. See the module docstring.
        return "swap", (f"swapped {extra['swapped_out']} out "
                        f"for {extra.get('swapped_in', 'another card')}")

    if "note" in extra:
        return "note", f"changed the {extra['note']} note"

    # `card` before `field`: both operations pass a `field`, and only the
    # card-level one names a card.
    if "card" in extra:
        field = str(extra.get("field", ""))
        word = _FIELD_WORDS.get(field, field or "entry")
        return "set-card", f"changed the {word} for {extra['card']}"

    if "field" in extra:
        field = str(extra["field"])
        word = _FIELD_WORDS.get(field, field)
        value = extra.get("value")
        if isinstance(value, (list, tuple)):
            # `themes` is a list, and `str(list)` would put brackets and
            # quotes in a sentence people read.
            text = ", ".join(str(v) for v in value)
        else:
            text = "" if value is None else str(value).strip()
        # Emptiness is checked first, and that ordering is the whole of this
        # branch: clearing the art back to the default printing is a real
        # edit, and the art rule below would have reported it as a change to
        # some new picture nobody chose.
        if not text:
            return "set-deck", f"cleared the {word}"
        # An art id is a Scryfall UUID. Nobody reads one, and printing it
        # would be the longest entry in the panel saying the least.
        if field in ("commander_art", "art") or len(text) > 40:
            return "set-deck", f"changed the {word}"
        return "set-deck", f"set {word} to {text}"

    return "edit", "edited the deck"


def record(*, slug: str, action: str, summary: str,
           owner_id: int | None = None, actor: str | None = None,
           path: Path | str | None = None) -> None:
    """Write one entry. Never raises.

    `owner_id` is `None` for the file-backed curated library, of which there is
    one per instance -- see the migration in `auth/db.py` for why that is not
    the owner segment out of the URL.

    `actor` is a username or `None` for whoever is at this machine: the CLI,
    and the app with auth off. Never an email address, which must not reach a
    log line at all (`CLAUDE.md`, rule 5).

    On a laptop with auth off this is the first thing to *create* `app.db`, and
    that is accepted rather than worked around -- `sim/cache.py` already does
    it the first time somebody runs a simulation. The rule it must not break is
    the narrower one `test_the_local_app_touches_no_database` pins: *reading*
    must not acquire a database. An edit is not a read.
    """
    try:
        from mtglab.auth import db
        with db.connection(path) as con, con:
            con.execute(
                "INSERT INTO deck_log (created_at, owner_id, slug, actor,"
                " action, summary) VALUES (?, ?, ?, ?, ?, ?)",
                (_now(), owner_id, slug, actor, action, summary))
    except (sqlite3.Error, OSError) as exc:
        _LOG.warning("deck log record failed for %s (%s: %s)",
                     slug, type(exc).__name__, exc)


def entries(slug: str, *, owner_id: int | None = None,
            limit: int = DEFAULT_LIMIT,
            path: Path | str | None = None) -> list[dict[str, Any]]:
    """This deck's history, newest first.

    Raises on a broken database: unlike `record`, this runs because somebody
    asked a question, and a wrong silent answer -- an empty history for a deck
    that has one -- would be worse than an error.

    `owner_id IS ?` rather than `= ?`, because the file tier's owner is NULL
    and `NULL = NULL` is not true in SQL. That is the whole reason this reads
    the way it does, and it is the bug this query would otherwise have: an
    equality test would return nothing at all for the curated six, forever,
    without erroring.
    """
    from mtglab.auth import db
    with db.connection(path) as con:
        rows = con.execute(
            "SELECT id, created_at, actor, action, summary FROM deck_log"
            " WHERE owner_id IS ? AND slug = ?"
            " ORDER BY id DESC LIMIT ?",
            (owner_id, slug, max(1, int(limit)))).fetchall()
    return [dict(row) for row in rows]
