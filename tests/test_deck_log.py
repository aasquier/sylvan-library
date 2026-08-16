"""The deck activity log (ADR 28): what it records, and what it refuses to.

Three things are being pinned, in rising order of how quietly they would break.

**The sentence.** `describe` is the only renderer -- the CLI and the deck panel
both show what it produced -- so a change to it is a change to every entry ever
shown, and it is worth having the cases written out.

**The rationale never lands here.** `swap_card` hands `_commit` the `why` the
user typed, and this is where it stops. Checked by scanning the whole table for
the text rather than by inspecting one column, because the failure mode is a
future operation passing prose under a key nobody thought about.

**The file tier is `owner_id IS NULL`, and NULL is not `=`.** An equality test
would return an empty history for the curated six forever, without erroring --
the shape of bug that ships. Two libraries are written here and each is asked
for its own.

Everything runs against a scratch `app.db` named by path, or under
`config.use_paths`. Never the config-derived default, which in a test process
is the developer's real data directory; `conftest.py`'s `_no_deck_log` is what
protects the rest of the suite, and one test below removes it deliberately.
"""

import logging
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

import tiny_pool
from mtglab import config
from mtglab.decks import log
from mtglab.decks.model import Deck
from mtglab.decks.source import MemoryDeckSource

# --------------------------------------------------------------- the sentence


@pytest.mark.parametrize("extra,action,summary", [
    ({"added": "Sol Ring", "category": "ramp", "into": "cards"},
     "add", "added Sol Ring as ramp"),
    ({"added": "Sol Ring", "category": "ramp", "into": "swap_board"},
     "add", "added Sol Ring as ramp on the swap board"),
    ({"entombed": "Sol Ring"}, "entomb", "entombed Sol Ring"),
    ({"entombed": ["Sol Ring", "Forest"]}, "entomb",
     "entombed Sol Ring, Forest"),
    ({"removed": "Sol Ring"}, "remove",
     "removed Sol Ring from the swap board"),
    ({"returned": "Sol Ring"}, "return",
     "returned Sol Ring from the graveyard"),
    ({"exiled": "Sol Ring"}, "exile", "exiled Sol Ring from the graveyard"),
    ({"swapped_out": "Sol Ring", "swapped_in": "Arcane Signet", "why": "x"},
     "swap", "swapped Sol Ring out for Arcane Signet"),
    ({"note": "mulligan"}, "note", "changed the mulligan note"),
    ({"card": "Sol Ring", "field": "why"}, "set-card",
     "changed the rationale for Sol Ring"),
    ({"card": "Sol Ring", "field": "qty"}, "set-card",
     "changed the quantity for Sol Ring"),
    ({"card": "Sol Ring", "field": "category"}, "set-card",
     "changed the category for Sol Ring"),
    ({"field": "status", "value": "built"}, "set-deck", "set status to built"),
    ({"field": "stage", "value": "curated"}, "set-deck",
     "set stage to curated"),
])
def test_describe_renders_each_operation(extra, action, summary):
    assert log.describe(extra) == (action, summary)


def test_a_bulk_entombment_names_a_handful_and_counts_the_rest():
    """The entry somebody opens the panel to expand, so the names are in it --
    up to the point where the sentence stops being a sentence."""
    _, summary = log.describe({"entombed": [f"Card {n}" for n in range(9)]})
    assert summary.startswith("entombed 9 cards: Card 0, ")
    assert summary.endswith("Card 5, and 3 more")


def test_an_art_id_is_not_printed_and_clearing_one_is_not_a_change():
    """A Scryfall UUID would be the longest entry in the panel saying the
    least. Clearing it back to the default printing is a real edit and must
    not be reported as a change to some new picture nobody chose."""
    assert log.describe({"field": "commander_art",
                         "value": "8a1b7c3d-0000-4000-8000-abcdefabcdef"}) == (
        "set-deck", "changed the commander art")
    assert log.describe({"field": "commander_art", "value": ""}) == (
        "set-deck", "cleared the commander art")


def test_an_unrecognised_operation_is_still_recorded():
    """The load-bearing fallback. An operation whose shape this has never seen
    is one somebody added, and a history that silently omits it is the single
    failure mode a history cannot have."""
    assert log.describe({"teleported": "Sol Ring"}) == ("edit", "edited the deck")


def test_describe_never_carries_a_rationale():
    """`swap_card` is the one operation that passes `why` through `_commit`,
    and this is where it stops. Rule 4's text lives in `deck.yaml`."""
    _, summary = log.describe({
        "swapped_out": "Sol Ring", "swapped_in": "Arcane Signet",
        "why": "a secret plan nobody should read back out of a table"})
    assert "secret plan" not in summary


# ------------------------------------------------------- recording and reading


def test_record_then_entries_roundtrip_newest_first(tmp_path):
    db = tmp_path / "app.db"
    for n in range(3):
        log.record(slug="green", action="add", summary=f"added Card {n}",
                   actor="ada", path=db)

    rows = log.entries("green", path=db)
    assert [r["summary"] for r in rows] == [
        "added Card 2", "added Card 1", "added Card 0"]
    assert {r["actor"] for r in rows} == {"ada"}
    assert rows[0]["created_at"] >= rows[-1]["created_at"]


def test_the_limit_takes_the_newest(tmp_path):
    db = tmp_path / "app.db"
    for n in range(5):
        log.record(slug="green", action="add", summary=f"#{n}", path=db)
    assert [r["summary"] for r in log.entries("green", limit=2, path=db)] == [
        "#4", "#3"]


def _an_account(db, user_id=7, username="bob"):
    """A row in `users`, written directly rather than through `users.create`.

    `deck_log.owner_id` references it, so a log row for an account that does
    not exist is refused by the foreign key -- which is the behaviour, not a
    detail to work around. Inserted as SQL so this file needs no argon2: what
    the constraint wants is an id, and hashing a password to get one would
    make every test here skip on a base install.
    """
    from mtglab.auth import db as auth_db

    with auth_db.connection(db) as con, con:
        con.execute("INSERT INTO users (id, username, created_at)"
                    " VALUES (?, ?, '2026-01-01T00:00:00+00:00')",
                    (user_id, username))


def test_two_libraries_do_not_share_a_history(tmp_path):
    """The file tier is NULL and everybody else is an id, and `NULL = NULL` is
    not true in SQL -- so `entries` asks with `IS`.

    Written with both tiers present because the bug this guards is silent in
    one direction: an equality test returns an *empty* history for the curated
    six rather than an error, and an empty history is what a new deck has.

    Both decks are called `green` on purpose. Two accounts may each have a
    deck by that slug -- ADR 22's namespace is per owner -- and a history keyed
    on the slug alone would show each of them the other's edits.
    """
    db = tmp_path / "app.db"
    _an_account(db)
    log.record(slug="green", action="add", summary="the curated one", path=db)
    log.record(slug="green", action="add", summary="somebody else's",
               owner_id=7, actor="bob", path=db)

    assert [r["summary"] for r in log.entries("green", path=db)] == [
        "the curated one"]
    assert [r["summary"] for r in log.entries("green", owner_id=7, path=db)] == [
        "somebody else's"]


def test_deleting_an_account_takes_its_history(tmp_path):
    """The consequence the CASCADE is there for, and it matches `user_decks`:
    deleting an account takes its decks, so a record of edits to decks that no
    longer exist, attributed to somebody with no account, must not outlive it.

    The file tier's NULL is exempt from the constraint, which is what lets one
    column mean "the curated library" without a second table -- so the curated
    entry is still here afterwards.
    """
    from mtglab.auth import db as auth_db

    db = tmp_path / "app.db"
    _an_account(db)
    log.record(slug="green", action="add", summary="mine", path=db)
    log.record(slug="green", action="add", summary="bob's", owner_id=7,
               actor="bob", path=db)

    with auth_db.connection(db) as con, con:
        con.execute("DELETE FROM users WHERE id = 7")

    assert log.entries("green", owner_id=7, path=db) == []
    assert [r["summary"] for r in log.entries("green", path=db)] == ["mine"]


def test_a_log_row_for_an_account_that_does_not_exist_is_refused(tmp_path,
                                                                caplog):
    """Not silently accepted, and not raised either: `record` never raises, so
    an impossible owner is a warning and no row. Reaching this needs a
    `SqlDeckSource` for an account `Library` could not have resolved, which is
    why it is a guard rather than a case anyone hits."""
    db = tmp_path / "app.db"
    with caplog.at_level(logging.WARNING):
        log.record(slug="green", action="add", summary="nobody's",
                   owner_id=999, path=db)
    assert "deck log record failed" in caplog.text
    assert log.entries("green", owner_id=999, path=db) == []


def test_a_deck_with_no_history_is_an_empty_list(tmp_path):
    assert log.entries("never-touched", path=tmp_path / "app.db") == []


def test_record_never_raises(tmp_path, caplog):
    """Accounting that can fail an edit is worse than no accounting: the deck
    write has already happened by the time this runs, so an exception here
    would report a failure for work that succeeded."""
    broken = tmp_path / "not-a-database"
    broken.write_text("this is not SQLite", encoding="utf-8")
    with caplog.at_level(logging.WARNING):
        log.record(slug="green", action="add", summary="x", path=broken)
    assert "deck log record failed" in caplog.text


def test_entries_does_raise(tmp_path):
    """Unlike `record`: this runs because somebody asked a question, and an
    empty history for a deck that has one would be worse than an error."""
    broken = tmp_path / "not-a-database"
    broken.write_text("this is not SQLite", encoding="utf-8")
    import sqlite3
    with pytest.raises(sqlite3.DatabaseError):
        log.entries("green", path=broken)


# ------------------------------------------------------------------- the seam


@pytest.fixture
def library(tmp_path, monkeypatch):
    """A real deck, a real pool and a real (scratch) `app.db`.

    `_no_deck_log` from conftest.py is removed here on purpose. That fixture
    keeps the rest of the suite out of the developer's history; a stub nothing
    ever takes back is how a broken seam stays green, so the tests that are
    *about* the seam put the real module back.
    """
    from mtglab.api import service

    monkeypatch.setattr(service, "log", log)
    root = tmp_path / "lib"
    with config.use_paths(data_dir=root / "data", decks_dir=root / "decks"):
        tiny_pool.build(config.DB_PATH)
        deck = tiny_pool.mono_green_deck()
        folder = config.DECKS_DIR / "mono-green"
        folder.mkdir(parents=True)
        (folder / "deck.yaml").write_text(deck.dump(), encoding="utf-8")
        yield service


def test_every_edit_operation_writes_one_entry(library):
    """The single call site is the point: `_commit` is where every deck write
    already passes through, so a route that forgets to log is not a route
    somebody can write."""
    service = library
    service.remove_card("mono-green", name="Sol Ring")
    service.return_card("mono-green", name="Sol Ring")
    service.set_card_field("mono-green", name="Sol Ring", field="why", value="ramp")
    service.set_deck_field("mono-green", field="status", value="built")
    service.set_note("mono-green", key="mulligan", value="two lands and a one-drop")

    rows = log.entries("mono-green")
    assert [r["action"] for r in rows] == [
        "note", "set-deck", "set-card", "return", "entomb"]
    # The CLI is not an account. `None` is whoever is at this machine, which
    # is the same person the app-with-auth-off is.
    assert {r["actor"] for r in rows} == {None}


def test_the_actor_is_recorded_when_there_is_one(library):
    library.set_deck_field("mono-green", field="status", value="built", actor="ada")
    assert log.entries("mono-green")[0]["actor"] == "ada"


def test_a_swap_records_the_cards_and_not_the_rationale(library):
    """Scanned over the whole table rather than one column, because the way
    this breaks in future is an operation passing prose under a key nobody
    thought about."""
    secret = "an argument I would rather not have mined back out of a table"
    library.swap_card("mono-green", out="Primeval Titan", into="Craterhoof Behemoth",
                      why=secret, actor="ada")

    row = log.entries("mono-green")[0]
    assert row["action"] == "swap"
    assert row["summary"] == (
        "swapped Primeval Titan out for Craterhoof Behemoth")

    from mtglab.auth import db as auth_db
    with auth_db.connection() as con:
        everything = str(con.execute("SELECT * FROM deck_log").fetchall())
    assert secret not in everything


def test_a_refused_edit_records_nothing(library):
    """Nothing is written before the gate, so nothing is logged before it
    either -- `_commit` is only reached once the deck file has changed."""
    with pytest.raises(library.EditRejected):
        library.remove_card("mono-green", name="A Card That Is Not In This Deck")
    assert log.entries("mono-green") == []


def test_history_for_404s_before_it_reads_a_row(library):
    """The whole authorisation check, and deliberately not a second rule: a
    deck this caller cannot see is absent from their source."""
    from mtglab.decks.source import DeckNotFound

    with pytest.raises(DeckNotFound):
        library.history_for("not-a-deck")


def test_history_for_reads_the_source_it_was_given(library):
    """A `MemoryDeckSource` has no `owner_id`, so it reads the file tier's
    rows -- which is what `_owner_id_of` answering `None` means."""
    library.set_note("mono-green", key="mulligan", value="two lands")
    green = Deck.from_text(
        (config.DECKS_DIR / "mono-green" / "deck.yaml").read_text(encoding="utf-8"),
        slug="mono-green")
    body = library.history_for("mono-green", source=MemoryDeckSource([green]))
    assert body["slug"] == "mono-green"
    assert [e["summary"] for e in body["entries"]] == [
        "changed the mulligan note"]
