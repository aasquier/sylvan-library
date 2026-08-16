"""The auth core: `app.db`, passwords, accounts, sessions, the limiter.

No HTTP here — `tests/test_auth_api.py` covers the routes and
`tests/test_isolation.py` covers the part that matters most. This file is the
rules underneath, and it is where the properties that are easy to state and
easy to lose get pinned: an unknown user costs a hash, a password change ends
every session, a token is never stored, a disabled account is refused the same
way a wrong password is.
"""

import sqlite3
import sys
from datetime import UTC, datetime, timedelta
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("argon2")

from mtglab import config
from mtglab.auth import db, passwords, ratelimit, sessions, users

GOOD = "a-perfectly-fine-passphrase"
ALSO_GOOD = "another-perfectly-fine-one"


@pytest.fixture
def con(tmp_path):
    with config.use_paths(data_dir=tmp_path / "data"):
        connection = db.connect()
        try:
            yield connection
        finally:
            connection.close()


# ------------------------------------------------------------------ the file

def test_connect_creates_the_file_and_the_schema(tmp_path):
    with config.use_paths(data_dir=tmp_path / "data"):
        assert not config.APP_DB_PATH.exists()
        connection = db.connect()
        try:
            tables = {r["name"] for r in connection.execute(
                "SELECT name FROM sqlite_master WHERE type = 'table'")}
        finally:
            connection.close()
        assert config.APP_DB_PATH.exists()
    assert {"users", "sessions", "login_attempts"} <= tables


def test_app_db_sits_beside_the_pool_and_moves_with_it(tmp_path):
    """`use_paths` has to move both, or a test writes into the real data dir."""
    with config.use_paths(data_dir=tmp_path / "elsewhere"):
        assert config.APP_DB_PATH.name == "app.db"
        assert config.APP_DB_PATH.parent == tmp_path / "elsewhere"
        assert config.APP_DB_PATH.parent == config.DB_PATH.parent
    assert config.APP_DB_PATH.parent == Path("data")


def test_wal_and_foreign_keys_are_on(con):
    """Both are per-connection defaults SQLite gets wrong for this use.

    `foreign_keys` especially: it is **off** by default, and with it off the
    `REFERENCES` clause on `sessions` is a comment rather than a constraint.
    """
    assert con.execute("PRAGMA journal_mode").fetchone()[0].lower() == "wal"
    assert con.execute("PRAGMA foreign_keys").fetchone()[0] == 1


def test_migrations_are_idempotent(tmp_path):
    """Opening an existing database must not re-run version 1."""
    with config.use_paths(data_dir=tmp_path / "data"):
        first = db.connect()
        users.create(first, "ada", password=GOOD)
        first.close()

        second = db.connect()                     # would throw on a re-run DDL
        try:
            assert users.count(second) == 1
            assert second.execute("PRAGMA user_version").fetchone()[0] == \
                db.SCHEMA_VERSION
        finally:
            second.close()


def test_deleting_a_user_cascades_to_their_sessions(con):
    ada = users.create(con, "ada", password=GOOD)
    sessions.create(con, ada.id)
    with con:
        con.execute("DELETE FROM users WHERE id = ?", (ada.id,))
    assert con.execute("SELECT count(*) FROM sessions").fetchone()[0] == 0


# ----------------------------------------------------------------- passwords

def test_the_profile_is_the_owasp_minimum():
    """Not a default to tune. 19 MiB is what keeps a 1 GB box alive under a
    login burst, and the number is the decision in ADR 5."""
    assert passwords.MEMORY_COST_KIB == 19_456           # 19 MiB, in KiB
    assert passwords.TIME_COST == 2
    assert passwords.PARALLELISM == 1


def test_a_hash_is_argon2id_and_carries_those_parameters():
    encoded = passwords.hash_password(GOOD)
    assert encoded.startswith("$argon2id$")
    assert "m=19456,t=2,p=1" in encoded


def test_the_same_password_hashes_differently_every_time():
    """Salted. Two accounts with the same password must not look alike."""
    assert passwords.hash_password(GOOD) != passwords.hash_password(GOOD)


def test_verify_round_trips_and_rejects():
    encoded = passwords.hash_password(GOOD)
    assert passwords.verify(encoded, GOOD)
    assert not passwords.verify(encoded, ALSO_GOOD)


@pytest.mark.parametrize("stored", [None, "", "not-a-hash", "$argon2id$broken"])
def test_verify_never_raises_on_a_bad_stored_hash(stored):
    """Every failure is one answer. A login that reported "your stored hash is
    corrupt" would be reporting on the database to whoever typed a password."""
    assert passwords.verify(stored, GOOD) is False


def test_short_passwords_are_refused_before_storage():
    with pytest.raises(passwords.WeakPassword):
        passwords.hash_password("short")


def test_absurdly_long_passwords_are_refused():
    """An unbounded field plus a memory-hard hash is a denial of service."""
    with pytest.raises(passwords.WeakPassword):
        passwords.hash_password("x" * (passwords.MAX_PASSWORD_BYTES + 1))


def test_needs_rehash_is_false_for_a_current_hash():
    assert not passwords.needs_rehash(passwords.hash_password(GOOD))


# ------------------------------------------------------------------ accounts

def test_create_and_fetch(con):
    ada = users.create(con, "ada", password=GOOD, email="Ada@Example.COM")
    assert ada.username == "ada"
    assert ada.email == "ada@example.com"            # normalised on the way in
    assert not ada.is_admin
    assert not ada.disabled
    assert users.get(con, "ADA") == ada              # COLLATE NOCASE
    assert users.get_by_email(con, "ADA@example.com") == ada


def test_the_record_does_not_carry_the_hash(con):
    """A hash that is never loaded into the record cannot leak out of it."""
    ada = users.create(con, "ada", password=GOOD)
    assert "password_hash" not in ada.as_dict(include_email=True)
    assert not hasattr(ada, "password_hash")


def test_email_is_left_out_of_the_default_serialisation(con):
    """Opt-in, because the rule is easy to state and easy to forget."""
    ada = users.create(con, "ada", password=GOOD, email="ada@example.com")
    assert "email" not in ada.as_dict()
    assert ada.as_dict(include_email=True)["email"] == "ada@example.com"


def test_duplicate_username_is_refused_case_insensitively(con):
    users.create(con, "ada", password=GOOD)
    with pytest.raises(users.UserExists):
        users.create(con, "ADA", password=ALSO_GOOD)


def test_duplicate_email_is_refused(con):
    users.create(con, "ada", password=GOOD, email="ada@example.com")
    with pytest.raises(users.UserExists, match="email"):
        users.create(con, "grace", password=ALSO_GOOD, email="ADA@example.com")


def test_several_accounts_may_have_no_email(con):
    """SQLite's UNIQUE permits many NULLs, which is the wanted behaviour --
    the maintainer's bootstrap account does not need an address."""
    users.create(con, "ada", password=GOOD)
    users.create(con, "grace", password=ALSO_GOOD)
    assert users.count(con) == 2


@pytest.mark.parametrize("bad", ["", "a", "-nope", "has space", "x" * 33,
                                 "emoji🙂"])
def test_bad_usernames_are_refused(con, bad):
    with pytest.raises(users.InvalidUsername):
        users.create(con, bad, password=GOOD)


@pytest.mark.parametrize("bad", ["nope", "a@b", "two@@example.com",
                                 "spaces @example.com"])
def test_bad_emails_are_refused(con, bad):
    with pytest.raises(users.InvalidEmail):
        users.create(con, "ada", password=GOOD, email=bad)


def test_an_address_at_the_rfc_limit_is_still_accepted(con):
    """The bound is RFC 5321's 254 octets, so it refuses nothing that was ever
    deliverable -- which is only true if the limit itself gets in."""
    local = "a" * (users.MAX_EMAIL - len("@example.com"))
    assert users.normalise_email(f"{local}@example.com") is not None


def test_an_oversized_address_is_refused_before_the_pattern(con):
    """`EMAIL_RE` is polynomial in what it is handed and `normalise_email` sits
    on the unauthenticated claim path -- the one place a stranger picks the
    input. The length check runs first for that reason."""
    with pytest.raises(users.InvalidEmail):
        users.normalise_email("a" * users.MAX_EMAIL + "@example.com")


def test_the_refusal_does_not_echo_the_whole_address(con):
    """An oversized address is exactly what reaches here, so quoting it whole
    would put the caller's own megabyte into an exception, a log and a
    response."""
    with pytest.raises(users.InvalidEmail) as raised:
        users.normalise_email("a" * 100_000 + "@example.com")
    assert len(str(raised.value)) < users.MAX_EMAIL * 2


def test_an_account_can_exist_without_a_password(con):
    """The state an invite leaves behind (ADR 16): it exists, it cannot log in."""
    ada = users.create(con, "ada")
    assert not users.has_password(con, ada.id)
    assert users.authenticate(con, "ada", GOOD) is None


# ------------------------------------------------------------ authentication

def test_authenticate_accepts_the_right_password(con):
    ada = users.create(con, "ada", password=GOOD)
    assert users.authenticate(con, "ada", GOOD) == ada
    assert users.authenticate(con, "ADA", GOOD) == ada


def test_authenticate_refuses_the_wrong_one(con):
    users.create(con, "ada", password=GOOD)
    assert users.authenticate(con, "ada", ALSO_GOOD) is None


def test_authenticate_refuses_a_disabled_account(con):
    ada = users.create(con, "ada", password=GOOD)
    users.set_disabled(con, ada.id, True)
    assert users.authenticate(con, "ada", GOOD) is None


def test_an_unknown_user_still_costs_a_hash(con, monkeypatch):
    """The timing leak, pinned by observing the work rather than the clock.

    A wall-clock assertion would be flaky on a shared runner, so this counts
    verifications instead: an unknown username must do exactly as much hashing
    as a known one, which is what makes the two indistinguishable.
    """
    users.create(con, "ada", password=GOOD)
    calls = []
    real = passwords.verify_dummy
    monkeypatch.setattr(passwords, "verify_dummy",
                        lambda pw: (calls.append(pw), real(pw))[1])

    assert users.authenticate(con, "nobody-at-all", GOOD) is None
    assert calls == [GOOD], "an unknown user returned without hashing anything"


def test_an_unclaimed_account_costs_a_hash_too(con, monkeypatch):
    """An invited-but-unset account must not be identifiable by how fast it
    is refused, which a bare `if hash is None: return` would make it."""
    users.create(con, "ada")                            # no password
    calls = []
    real = passwords.verify_dummy
    monkeypatch.setattr(passwords, "verify_dummy",
                        lambda pw: (calls.append(pw), real(pw))[1])

    assert users.authenticate(con, "ada", GOOD) is None
    assert calls == [GOOD]


def test_a_disabled_account_is_refused_after_the_hash_not_before(con,
                                                                monkeypatch):
    """Order matters: checking `disabled` first would make a disabled account
    answer faster than a wrong password, which says the account exists."""
    ada = users.create(con, "ada", password=GOOD)
    users.set_disabled(con, ada.id, True)
    verified = []
    real = passwords.verify
    monkeypatch.setattr(passwords, "verify",
                        lambda h, pw: (verified.append(pw), real(h, pw))[1])

    assert users.authenticate(con, "ada", GOOD) is None
    assert verified == [GOOD]


def test_an_outdated_hash_is_upgraded_on_a_successful_login(con, monkeypatch):
    """The only moment a hash can be re-made for free: the plaintext is in hand
    and has just been proven correct."""
    ada = users.create(con, "ada", password=GOOD)
    before = con.execute("SELECT password_hash FROM users WHERE id = ?",
                         (ada.id,)).fetchone()[0]
    monkeypatch.setattr(passwords, "needs_rehash", lambda h: True)

    assert users.authenticate(con, "ada", GOOD) == ada
    after = con.execute("SELECT password_hash FROM users WHERE id = ?",
                        (ada.id,)).fetchone()[0]
    assert after != before
    assert passwords.verify(after, GOOD)


# ------------------------------------------------------------------ sessions

def test_a_session_round_trips(con):
    ada = users.create(con, "ada", password=GOOD)
    token = sessions.create(con, ada.id)
    found = sessions.lookup(con, token)
    assert found is not None
    assert found.user_id == ada.id


def test_the_token_itself_is_never_stored(con):
    """Reading `app.db` must not hand over live sessions (ADR 5)."""
    ada = users.create(con, "ada", password=GOOD)
    token = sessions.create(con, ada.id)
    stored = con.execute("SELECT token_hash FROM sessions").fetchone()[0]
    assert token not in stored
    assert stored != token
    assert len(stored) == 64                                  # sha256, hex


def test_every_token_is_distinct(con):
    ada = users.create(con, "ada", password=GOOD)
    minted = {sessions.create(con, ada.id) for _ in range(25)}
    assert len(minted) == 25


def test_an_unknown_or_empty_token_resolves_to_nothing(con):
    assert sessions.lookup(con, "") is None
    assert sessions.lookup(con, "not-a-real-token") is None


def test_an_expired_session_is_refused_and_removed(con):
    ada = users.create(con, "ada", password=GOOD)
    token = sessions.create(con, ada.id, lifetime=timedelta(seconds=-1))
    assert sessions.lookup(con, token) is None
    assert con.execute("SELECT count(*) FROM sessions").fetchone()[0] == 0


def test_logout_ends_exactly_one_session(con):
    ada = users.create(con, "ada", password=GOOD)
    laptop, phone = sessions.create(con, ada.id), sessions.create(con, ada.id)
    sessions.delete(con, laptop)
    assert sessions.lookup(con, laptop) is None
    assert sessions.lookup(con, phone) is not None


def test_changing_a_password_ends_every_session(con):
    """ADR 16, and the reason it is a rule: a reset is usually somebody who
    suspects compromise, and one that leaves the attacker logged in has
    answered the wrong question."""
    ada = users.create(con, "ada", password=GOOD)
    tokens = [sessions.create(con, ada.id) for _ in range(3)]

    assert users.set_password(con, ada.id, ALSO_GOOD) == 3
    assert all(sessions.lookup(con, t) is None for t in tokens)
    assert users.authenticate(con, "ada", ALSO_GOOD) == ada


def test_disabling_an_account_ends_every_session(con):
    """An account that cannot log in but whose cookie still works has not been
    disabled in any sense the person doing it meant."""
    ada = users.create(con, "ada", password=GOOD)
    token = sessions.create(con, ada.id)
    assert users.set_disabled(con, ada.id, True) == 1
    assert sessions.lookup(con, token) is None


def test_one_users_password_change_leaves_another_alone(con):
    ada = users.create(con, "ada", password=GOOD)
    grace = users.create(con, "grace", password=ALSO_GOOD)
    mine, theirs = sessions.create(con, ada.id), sessions.create(con, grace.id)

    users.set_password(con, ada.id, "a-third-long-passphrase")
    assert sessions.lookup(con, mine) is None
    assert sessions.lookup(con, theirs) is not None


def test_purge_drops_only_what_has_expired(con):
    ada = users.create(con, "ada", password=GOOD)
    live = sessions.create(con, ada.id)
    sessions.create(con, ada.id, lifetime=timedelta(seconds=-1))
    assert sessions.purge_expired(con) == 1
    assert sessions.lookup(con, live) is not None


def test_count_for_user_ignores_expired_ones(con):
    ada = users.create(con, "ada", password=GOOD)
    sessions.create(con, ada.id)
    sessions.create(con, ada.id, lifetime=timedelta(seconds=-1))
    assert sessions.count_for_user(con, ada.id) == 1


def test_last_seen_is_written_but_not_on_every_request(con, monkeypatch):
    """A write per authenticated request is cheap on WAL but not free, and the
    column is only ever read by a human wondering when an account was used."""
    ada = users.create(con, "ada", password=GOOD)
    token = sessions.create(con, ada.id)

    def stamp() -> str:
        return con.execute("SELECT last_seen_at FROM sessions").fetchone()[0]

    first = stamp()
    sessions.lookup(con, token)
    assert stamp() == first, "touched again inside the interval"

    monkeypatch.setattr(sessions, "_now",
                        lambda: datetime.now(UTC) + timedelta(minutes=10))
    sessions.lookup(con, token)
    assert stamp() != first


# ---------------------------------------------------------------- the limiter

def test_a_fresh_key_has_its_full_budget(con):
    key = ratelimit.account_key("ada")
    assert not ratelimit.exhausted(con, key, ratelimit.PER_ACCOUNT)


def test_failures_accumulate_until_the_budget_is_spent(con):
    key = ratelimit.account_key("ada")
    limit = ratelimit.PER_ACCOUNT
    for _ in range(limit.failures):
        assert not ratelimit.exhausted(con, key, limit)
        ratelimit.record_failure(con, key, limit)
    assert ratelimit.exhausted(con, key, limit)


def test_a_success_clears_the_count(con):
    key = ratelimit.account_key("ada")
    limit = ratelimit.PER_ACCOUNT
    for _ in range(limit.failures):
        ratelimit.record_failure(con, key, limit)
    ratelimit.clear(con, key)
    assert not ratelimit.exhausted(con, key, limit)


def test_the_window_lapses(con):
    """Fixed window: once it has passed, the budget is whole again.

    The lapse is *backdated* rather than slept through. The first version
    used a 40ms window and a real `time.sleep`, and on a loaded CI runner
    those 40ms could elapse between recording the failures and the first
    assertion -- a flake, and one with teeth: it failed the push run for
    #105 on `main`, and a red push run is a deploy that silently never
    happens (the #94 lesson, again). Rewriting `window_start` tests the
    same lapse arithmetic with no clock in the race at all.
    """
    key = ratelimit.account_key("ada")
    limit = ratelimit.Limit(failures=2, window=timedelta(minutes=15))
    ratelimit.record_failure(con, key, limit)
    ratelimit.record_failure(con, key, limit)
    assert ratelimit.exhausted(con, key, limit)

    lapsed = datetime.now(UTC) - limit.window - timedelta(seconds=1)
    con.execute("UPDATE login_attempts SET window_start = ? WHERE key = ?",
                (lapsed.isoformat(), key))
    con.commit()
    assert not ratelimit.exhausted(con, key, limit)


def test_the_two_keys_are_independent(con):
    """Which is the whole reason there are two. Spraying one password across
    every username must not be free, and neither must guessing one account
    from many addresses."""
    account, address = ratelimit.account_key("ada"), ratelimit.address_key("::1")
    for _ in range(ratelimit.PER_ACCOUNT.failures):
        ratelimit.record_failure(con, account, ratelimit.PER_ACCOUNT)
    assert ratelimit.exhausted(con, account, ratelimit.PER_ACCOUNT)
    assert not ratelimit.exhausted(con, address, ratelimit.PER_ADDRESS)


def test_account_keys_are_case_insensitive(con):
    """Or `ADA` gets a fresh budget against the same account as `ada`."""
    assert ratelimit.account_key("ADA ") == ratelimit.account_key("ada")


def test_retry_after_is_a_positive_number_of_seconds(con):
    key = ratelimit.account_key("ada")
    ratelimit.record_failure(con, key, ratelimit.PER_ACCOUNT)
    wait = ratelimit.retry_after(con, key, ratelimit.PER_ACCOUNT)
    assert 0 < wait <= ratelimit.PER_ACCOUNT.window.total_seconds()


def test_purge_stale_clears_old_windows(con):
    key = ratelimit.account_key("ada")
    ratelimit.record_failure(con, key, ratelimit.PER_ACCOUNT)
    assert ratelimit.purge_stale(con, older_than=timedelta(seconds=-1)) == 1
    assert not ratelimit.exhausted(con, key, ratelimit.PER_ACCOUNT)


# ------------------------------------------------------------------- misuse

def test_setting_a_password_on_a_missing_account_raises(con):
    with pytest.raises(users.NoSuchUser):
        users.set_password(con, 999, GOOD)


def test_disabling_a_missing_account_raises(con):
    with pytest.raises(users.NoSuchUser):
        users.set_disabled(con, 999, True)


def test_promoting_a_missing_account_raises(con):
    with pytest.raises(users.NoSuchUser):
        users.set_admin(con, 999, True)


def test_has_password_on_a_missing_account_raises(con):
    with pytest.raises(users.NoSuchUser):
        users.has_password(con, 999)


def test_admin_is_a_flag_that_can_be_taken_away(con):
    """Two admins, because ADR 17 refuses to leave an instance with none.

    The second account is not decoration: `set_admin(..., False)` on the only
    admin who can sign in now raises `LastAdmin`. That refusal and its edges
    live in `tests/test_admin.py`; this test is still about the flag itself.
    """
    ada = users.create(con, "ada", password=GOOD, is_admin=True)
    users.create(con, "grace", password=GOOD, is_admin=True)
    assert users.get(con, "ada").is_admin
    users.set_admin(con, ada.id, False)
    assert not users.get(con, "ada").is_admin


def test_users_are_listed_in_a_stable_order(con):
    for name in ("grace", "Ada", "linus"):
        users.create(con, name, password=GOOD)
    assert [u.username for u in users.all_users(con)] == ["Ada", "grace", "linus"]


def test_a_session_for_a_nonexistent_user_is_refused_by_the_database(con):
    """The `REFERENCES` clause, doing its job -- which it only does because
    `connect()` turns foreign keys on."""
    with pytest.raises(sqlite3.IntegrityError):
        sessions.create(con, 4242)


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
