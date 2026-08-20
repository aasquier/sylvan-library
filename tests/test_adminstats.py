"""The admin dashboard's numbers: what the four stats views actually report.

`tests/test_isolation.py` owns *who may reach* these routes — they are
classified `ADMIN` there, so the 403-for-non-admins sweep covers them without
this file repeating it. This file owns what they say once reached, and the
properties are mostly about honesty on a fresh box:

- an absent store is `null`, never `0` — a fresh instance has no pool, and
  "present and empty" is a different fact from "not there yet";
- the Claude view's caveat rides in the payload, because the token totals
  are a floor on the bill and any renderer of the numbers needs the sentence
  next to them;
- the activity view is counts all the way down — nothing in it names a deck,
  a question, or another person's job.
"""

import logging
import sqlite3
import sys
import tempfile
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("fastapi")
pytest.importorskip("httpx")
pytest.importorskip("argon2")

from fastapi.testclient import TestClient

from mtglab import config
from mtglab.api import adminstats, jobs
from mtglab.api.app import create_app
from mtglab.auth import db, users
from mtglab.claude import ledger

PASSWORD = "correct-horse-battery-staple"
OTHER = "a-different-long-passphrase"


@pytest.fixture
def client(tmp_path, monkeypatch):
    """Auth on, logged in as an admin, with a second ordinary account."""
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    jobs.clear()
    # `decks_dir` is scoped as well as `data_dir`, and the first run of this
    # file is why: left at the default, the storage endpoint counted the nine
    # real deck directories of the machine the suite ran on (ADR 30 — decks
    # are live app data, and a checkout has none).
    with config.use_paths(data_dir=tmp_path / "data",
                          decks_dir=tmp_path / "decks"):
        connection = db.connect()
        try:
            users.create(connection, "root", password=PASSWORD,
                         email="root@example.com", is_admin=True)
            users.create(connection, "friend", password=OTHER,
                         email="friend@example.com")
        finally:
            connection.close()
        app = create_app(require_auth=True, secure_cookies=False)
        with TestClient(app) as test_client:
            assert test_client.post(
                "/api/auth/login",
                json={"username": "root", "password": PASSWORD}
            ).status_code == 200
            yield test_client


def test_system_reports_the_process_and_the_volume(client):
    body = client.get("/api/admin/stats/system").json()

    assert body["process"]["bytes"] > 0
    # The label the page needs to render the number honestly: /proc gives a
    # current figure, the getrusage fallback a peak.
    assert body["process"]["kind"] in ("current", "peak")
    assert body["disk"]["total_bytes"] > 0
    assert body["disk"]["used_bytes"] + body["disk"]["free_bytes"] <= \
        body["disk"]["total_bytes"]
    # The volume the number describes is named, so the page never has to
    # guess which mount it is looking at.
    assert body["disk"]["path"] == str(config.DATA_DIR)


def test_system_reports_the_schema_version_the_file_actually_reached(client):
    """Both halves, because a lone version number cannot be wrong.

    ADR 23 is the reason this is on a panel at all: a merge deploys itself
    and the migration runs on boot with nobody watching, so `applied` is
    read off the database and `expected` off the code that is running.
    """
    body = client.get("/api/admin/stats/system").json()

    # The scratch app.db is migrated by the fixture that logs in, so a
    # healthy pair is what this box should report.
    assert body["schema"]["expected"] == db.SCHEMA_VERSION
    assert body["schema"]["applied"] == db.SCHEMA_VERSION


def test_reading_the_schema_version_never_migrates_the_file(tmp_path):
    """The panel may not change the thing it reports on.

    `db.connect` applies migrations, which is right everywhere except here:
    an admin refreshing a stats tab would silently upgrade the volume. The
    check is a database deliberately left at an older version -- if the read
    path migrates, `user_version` moves and this fails.
    """
    stale = tmp_path / "app.db"
    con = sqlite3.connect(stale)
    con.execute("PRAGMA user_version = 3")
    con.commit()
    con.close()

    with config.use_paths(data_dir=tmp_path):
        reported = adminstats._schema()

    after = sqlite3.connect(stale)
    try:
        left_at = after.execute("PRAGMA user_version").fetchone()[0]
    finally:
        after.close()

    assert reported["applied"] == 3
    assert reported["expected"] == db.SCHEMA_VERSION
    assert left_at == 3, "reading the version migrated the database"


def test_the_schema_version_is_null_rather_than_conjured(tmp_path, caplog):
    """A missing `app.db` is a fresh laptop, not an error.

    Two guards, checked apart because they are not the same guard and a
    mutation run caught this docstring crediting the wrong one. `mode=ro` is
    what refuses to create the file. The `exists()` check earns its line
    separately: without it every poll of a timer-refreshed panel logs a
    warning about a database that is *correctly* absent.
    """
    with (config.use_paths(data_dir=tmp_path),
          caplog.at_level(logging.WARNING, logger="mtglab.api.adminstats")):
        reported = adminstats._schema()

    assert reported["applied"] is None
    assert reported["expected"] == db.SCHEMA_VERSION
    assert not (tmp_path / "app.db").exists(), "asking created a database"
    assert not caplog.records, "an absent database is not worth a warning"


def test_the_expected_version_is_read_from_the_code_not_restated(
        monkeypatch, tmp_path):
    """It must report the constant, not a copy of today's value of it.

    Asserting `expected == db.SCHEMA_VERSION` cannot tell a lookup from a
    literal while the literal happens to be right -- the restated-claim shape
    this cycle found five times. Moving the constant is what separates them.

    `use_paths` is not decoration here, and the first draft omitted it. This
    test moves `SCHEMA_VERSION` to a number no ladder will ever reach, and
    `_schema` resolves its database from `config` -- so pointed at the
    ambient data directory it reads the maintainer's own `app.db` while a
    fake version is installed. Read-only, so the test itself was harmless;
    the *mutation* run that checks this guard was not. Replacing the
    read-only connection with `db.connect` -- exactly the wrongness the
    guard exists to catch -- migrated the real database to version 4242,
    where `_apply_migrations` would have returned early forever and silently
    skipped every future migration. A sandbox is what makes a destructive
    mutation survivable.
    """
    monkeypatch.setattr(db, "SCHEMA_VERSION", 4242)
    with config.use_paths(data_dir=tmp_path):
        assert adminstats._schema()["expected"] == 4242


def test_storage_says_null_for_absent_and_sized_for_present(client):
    """The fresh-box distinction this endpoint exists to make."""
    body = client.get("/api/admin/stats/storage").json()

    # The scratch app.db genuinely exists — logging in wrote to it.
    assert body["app_db_bytes"] > 0
    # There is no card pool in a scratch data dir, and that is `null`, not
    # zero: a zero would claim an empty pool file exists.
    assert body["pool_bytes"] is None
    assert body["scryfall_bulk_bytes"] is None
    assert body["decks"]["count"] == 0
    assert body["decks"]["trashed"] == 0


def test_the_cache_breakdown_cannot_hide_a_shelf_nobody_named(client):
    """The failure this remainder was added for, reproduced.

    The tile shipped naming `cardmotion` and `symbols` while the reading
    engine (`ocr.py`, ~5.8MB) sat beside them unnamed — 38% of the deployed
    cache, present in the total and absent from the breakdown, so the line
    read as an itemisation and was not one. A fixed list of tenants can only
    ever miss the next one; a remainder cannot. `surprise` below is that next
    one, standing in for whatever it turns out to be.
    """
    cache = config.DATA_DIR / "cache"
    sizes = {"symbols": 10, "cardmotion": 200, "ocr": 3000, "surprise": 40000}
    for name, size in sizes.items():
        (cache / name).mkdir(parents=True)
        (cache / name / "blob").write_bytes(b"x" * size)

    body = client.get("/api/admin/stats/storage").json()["cache"]

    assert body["symbols_bytes"] == sizes["symbols"]
    assert body["cardmotion_bytes"] == sizes["cardmotion"]
    assert body["ocr_bytes"] == sizes["ocr"]
    # The whole point: the shelf this endpoint has never heard of is still
    # counted, and it is counted somewhere a reader can see it.
    assert body["other_bytes"] == sizes["surprise"]


def test_an_unfilled_cache_reports_nothing_rather_than_zero(client):
    """Absent is not empty, the same distinction the pool makes above."""
    body = client.get("/api/admin/stats/storage").json()

    assert body["cache_bytes"] is None
    assert body["cache"] == {"symbols_bytes": None, "cardmotion_bytes": None,
                             "ocr_bytes": None, "other_bytes": None}


def test_claude_totals_carry_their_caveat(client):
    """The numbers and the sentence travel together, Tier 1's rule."""
    empty = client.get("/api/admin/stats/claude").json()
    assert empty["windows"]["week"]["by_mode"] == []
    assert empty["windows"]["all"]["by_model"] == []
    assert empty["windows"]["all"]["estimated_usd"]["usd"] == 0
    assert "floor" in empty["caveat"]
    # The date a person last read the pricing page rides with the figure. A
    # number whose age is invisible is the wrong invoice the old no-price-table
    # comment warned about; one a reader can discount is not.
    assert empty["prices"]["checked"]

    ledger.record(mode="dossier", model="claude-sonnet-5",
                  stop_reason="end_turn", requests=3,
                  input_tokens=1000, output_tokens=200,
                  cache_read_tokens=400)

    body = client.get("/api/admin/stats/claude").json()
    week = {row["mode"]: row for row in body["windows"]["week"]["by_mode"]}
    assert week["dossier"]["conversations"] == 1
    assert week["dossier"]["input_tokens"] == 1000
    # A row recorded now is inside every window.
    assert {row["mode"] for row in body["windows"]["all"]["by_mode"]} == {"dossier"}


def test_the_two_axes_sum_to_the_same_totals(client):
    """Each is its own `GROUP BY`, not a pivot of the other — so a disagreement
    here means one of the two queries lost rows."""
    for mode in ("dossier", "research"):
        ledger.record(mode=mode, model="claude-sonnet-5",
                      stop_reason="end_turn", requests=2,
                      input_tokens=500, output_tokens=100,
                      cache_read_tokens=50)

    pane = client.get("/api/admin/stats/claude").json()["windows"]["all"]
    for field in ("conversations", "requests", "input_tokens",
                  "output_tokens", "cache_read_tokens"):
        assert (sum(r[field] for r in pane["by_mode"])
                == sum(r[field] for r in pane["by_model"])), field


def test_a_per_mode_row_says_various_rather_than_naming_one_model(client):
    """Grouping by mode aggregates the model, so the column cannot name one
    truthfully. `(various)` is the honest answer; whichever id SQLite happened
    to keep would look like a fact and be an accident."""
    for model in ("claude-sonnet-5", "claude-opus-5"):
        ledger.record(mode="dossier", model=model, stop_reason="end_turn",
                      requests=1, input_tokens=10, output_tokens=10,
                      cache_read_tokens=0)

    pane = client.get("/api/admin/stats/claude").json()["windows"]["all"]
    assert [r["model"] for r in pane["by_mode"]] == ["(various)"]
    assert {r["model"] for r in pane["by_model"]} == {"claude-sonnet-5",
                                                     "claude-opus-5"}
    assert {r["mode"] for r in pane["by_model"]} == {"(various)"}


def test_a_model_with_no_rate_is_counted_rather_than_priced_at_zero(client):
    """The ledger stores the served-by id, which can be one this build has
    never heard of. Charging it nothing without saying so would make the total
    read low and reassuring — which is worse than refusing to price it."""
    ledger.record(mode="dossier", model="claude-archaeopteryx-9",
                  stop_reason="end_turn", requests=1,
                  input_tokens=9_000_000, output_tokens=9_000_000,
                  cache_read_tokens=0)

    spend = (client.get("/api/admin/stats/claude").json()
             ["windows"]["all"]["estimated_usd"])
    assert spend["usd"] == 0
    assert spend["unpriced"] == 1
    assert spend["unpriced_models"] == ["claude-archaeopteryx-9"]
    assert spend["complete"] is False


def test_activity_counts_accounts_sessions_and_the_registry(client):
    body = client.get("/api/admin/stats/activity").json()

    # Two claimed accounts, both able to sign in.
    assert body["accounts"] == {"active": 2}
    # Exactly one session exists (root's own), and it has been seen today —
    # this very request authenticated with it.
    assert body["sessions"]["total"] == 1
    assert body["sessions"]["seen_day"] == 1
    assert body["sessions"]["seen_week"] >= body["sessions"]["seen_day"]
    assert body["deck_edits_by_day"] == []
    assert body["sim_cache_rows"] == 0
    assert body["jobs"] == {}


def test_activity_sees_the_job_registry_as_counts_only(client):
    """The census is statuses and integers — no label, no owner, no result.

    `jobs.get`/`all_jobs` scope to an owner because labels can name another
    person's deck; the dashboard's cross-owner view must stay too coarse to
    leak. This pins the shape, so a helpful field added later fails a test
    instead of an audit.
    """
    jobs.completed("sim", result={"answer": 42}, label="somebody's deck",
                   owner=99)
    try:
        body = client.get("/api/admin/stats/activity").json()
        assert body["jobs"] == {"done": 1}
        assert "somebody's deck" not in str(body)
    finally:
        jobs.clear()


# ------------------------------------------- the two spellings of one ladder

def test_the_accounts_table_and_the_dashboard_agree_about_every_state():
    """`api/admin.py:_state` and `adminstats.py:_user_state` are the same four
    rules written twice, and the second one's docstring says the duplication is
    deliberate. This is that claim made checkable.

    Both spellings are called over the same four accounts on the same
    connection. The comment argues that a disagreement would show up as the
    accounts table and the dashboard tile contradicting each other on one page;
    without this, the two could drift for a release before anybody noticed,
    which is precisely what the shared import was rejected for avoiding
    *later* rather than never.
    """
    from mtglab.api import admin as admin_routes
    from mtglab.auth import invites

    with config.use_paths(data_dir=Path(tempfile.mkdtemp()) / "data"):
        con = db.connect()
        try:
            signed_in = users.create(con, "active", password=PASSWORD)
            invited = users.create(con, "invited", email="i@example.com")
            invites.send_invite(con, invited, sender=_Silent())
            # Created on the machine and never given one: `mtglab users add`
            # without a password, and no invite to claim either.
            stranded = users.create(con, "stranded")
            off = users.create(con, "off", password=OTHER)
            users.set_disabled(con, off.id, True)

            expected = {"active": "active", "invited": "invited",
                        "stranded": "no password", "off": "disabled"}
            for user in (signed_in, invited, stranded, off):
                fresh = users.get(con, user.username)
                table = admin_routes._state(con, fresh)
                tile = adminstats._user_state(con, fresh)
                assert table == tile == expected[fresh.username], (
                    f"{fresh.username}: accounts table says {table!r}, "
                    f"dashboard says {tile!r}")
        finally:
            con.close()


class _Silent:
    """An `EmailSender` that sends nothing. No test here sends mail."""

    def send(self, message) -> None:
        del message


# ----------------------------------------- the platform the container is on

class _FakeProc:
    """Enough of `Path` to stand in for the two files only Linux has.

    The deployed instance is the audience for `_rss` and `_machine_memory`,
    and the dev Mac takes the fallback branch in both -- so the code that
    actually runs in production is the code no test on this machine reaches.
    Mapping the two `/proc` paths onto real files is the cheapest way to run
    the Linux half here; everything else delegates, so nothing else changes.
    """

    def __init__(self, mapping: dict[str, Path]) -> None:
        self._mapping = mapping

    def __call__(self, raw: str) -> Path:
        return self._mapping.get(raw, Path(raw))


def test_resident_memory_is_read_from_proc_where_there_is_one(tmp_path,
                                                              monkeypatch):
    """Linux reports the *current* RSS in kilobytes; the fallback reports a
    peak. `kind` is what lets the page label the number honestly, so it is
    asserted alongside the value rather than treated as decoration."""
    status = tmp_path / "status"
    status.write_text("Name:\tpython\nVmRSS:\t  123456 kB\nThreads:\t9\n")
    monkeypatch.setattr(adminstats, "Path",
                        _FakeProc({"/proc/self/status": status}))

    assert adminstats._rss() == {"bytes": 123456 * 1024, "kind": "current"}


def test_an_unreadable_proc_falls_back_rather_than_failing(tmp_path,
                                                           monkeypatch,
                                                           caplog):
    """A stats panel may not be the thing that takes the page down. The file
    exists and its contents are nonsense, which is the shape a container
    runtime change would produce."""
    status = tmp_path / "status"
    status.write_text("VmRSS:\tnot-a-number kB\n")
    monkeypatch.setattr(adminstats, "Path",
                        _FakeProc({"/proc/self/status": status}))

    with caplog.at_level(logging.WARNING, logger="mtglab.api.adminstats"):
        answer = adminstats._rss()

    assert answer["kind"] == "peak", "fell back rather than raising"
    assert caplog.records, "a broken /proc is worth saying out loud"


def test_available_memory_is_read_where_the_kernel_offers_it(tmp_path,
                                                             monkeypatch):
    """`MemAvailable` is Linux's own answer to how much could be allocated
    before swapping. There is no portable equivalent, so off Linux the field
    is absent rather than approximated -- which is why it needs a Linux."""
    meminfo = tmp_path / "meminfo"
    meminfo.write_text("MemTotal:       2048000 kB\n"
                       "MemFree:          10240 kB\n"
                       "MemAvailable:   1024000 kB\n")
    monkeypatch.setattr(adminstats, "Path",
                        _FakeProc({"/proc/meminfo": meminfo}))

    assert adminstats._machine_memory()["available_bytes"] == 1024000 * 1024


def test_a_platform_that_cannot_be_asked_reports_no_total(monkeypatch):
    """`os.sysconf` is the portable half and it is not portable everywhere.
    None rather than a guess: a memory tile showing an invented total is
    worse than one showing a gap."""
    monkeypatch.setattr(adminstats.os, "sysconf",
                        lambda name: (_ for _ in ()).throw(ValueError(name)))

    assert adminstats._machine_memory()["total_bytes"] is None


def test_a_garbled_meminfo_leaves_the_field_absent(tmp_path, monkeypatch):
    """Absent, not zero -- the same rule the storage view follows, because
    zero free memory and no answer are very different things to show."""
    meminfo = tmp_path / "meminfo"
    meminfo.write_text("MemAvailable:\n")
    monkeypatch.setattr(adminstats, "Path",
                        _FakeProc({"/proc/meminfo": meminfo}))

    assert adminstats._machine_memory()["available_bytes"] is None


def test_a_machine_with_no_load_average_reports_none_rather_than_500ing(
        client, monkeypatch):
    """`os.getloadavg` raises where the platform cannot answer. An empty list
    is a widget that hides; an exception is an admin page that does not load."""
    monkeypatch.setattr(adminstats.os, "getloadavg",
                        lambda: (_ for _ in ()).throw(OSError("no such thing")))

    assert client.get("/api/admin/stats/system").json()["load"] == []


# -------------------------------------------------- when the disk says no

def test_a_store_that_cannot_be_sized_is_absent_rather_than_zero(tmp_path,
                                                                 monkeypatch,
                                                                 caplog):
    """The third answer this helper can give. `None` already means "not there
    yet"; a directory that refuses to be walked is also not a size, and
    reporting `0` would put a full volume on the page as an empty one."""
    directory = tmp_path / "cache"
    directory.mkdir()
    monkeypatch.setattr(Path, "rglob",
                        lambda self, pattern: (_ for _ in ()).throw(
                            OSError("permission denied")))

    with caplog.at_level(logging.WARNING, logger="mtglab.api.adminstats"):
        assert adminstats._size_of(directory) is None

    assert caplog.records, "a refused read is worth saying out loud"


def test_counting_directories_survives_a_refused_listing(tmp_path,
                                                         monkeypatch):
    """Zero here is right rather than a lie: the count is "how many decks",
    and a listing that cannot be read is a count nobody can act on."""
    directory = tmp_path / "decks"
    directory.mkdir()
    monkeypatch.setattr(Path, "iterdir",
                        lambda self: (_ for _ in ()).throw(OSError("nope")))

    assert adminstats._count_dirs(directory) == 0


def test_a_corrupt_app_db_reports_no_version_and_still_reports_the_expected_one(
        tmp_path, caplog):
    """The other half of the pair. `applied` is a fact about the file and can
    go missing; `expected` is a fact about the code and cannot -- so a
    database that is not a database must still leave the tile able to say
    what this build was written against."""
    (tmp_path / "app.db").write_text("this is not a database")

    with (config.use_paths(data_dir=tmp_path),
          caplog.at_level(logging.WARNING, logger="mtglab.api.adminstats")):
        reported = adminstats._schema()

    assert reported == {"applied": None, "expected": db.SCHEMA_VERSION}
    assert caplog.records, "a corrupt app.db is worth a warning"


# --------------------------------------------------- the one view off the box

def test_fly_metrics_report_unconfigured_rather_than_broken(client,
                                                            monkeypatch):
    """`FLY_METRICS_TOKEN` is the maintainer's to mint and unset is an
    ordinary state, so the widget hides rather than showing a dashboard that
    looks broken. The route is `async` and hands the blocking fetch to a
    threadpool; going through the client is what exercises that."""
    from mtglab.api import flymetrics

    monkeypatch.delenv("FLY_METRICS_TOKEN", raising=False)
    flymetrics.reset()

    body = client.get("/api/admin/stats/fly").json()

    assert body == {"configured": False, "ok": False, "values": {}}
