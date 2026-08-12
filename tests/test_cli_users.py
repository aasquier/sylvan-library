"""`mtglab users`, run for real against a scratch `app.db`.

The rule this file exists to hold in place is ADR 16's: **no password is ever
chosen by one person for another.** In code that means there is no way to put a
password on the command line — no flag, no positional, no environment variable
— so `test_there_is_no_way_to_pass_a_password_as_an_argument` reads the parser
rather than trusting the absence of a flag to stay absent.

The prompt is `getpass`, which reads the terminal directly, so the tests
substitute it. That is the same substitution a person makes by typing.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("argon2")

from mtglab import config  # noqa: E402
from mtglab.auth import db, sessions, users  # noqa: E402
from mtglab.cli import main  # noqa: E402

PASSWORD = "a-perfectly-fine-passphrase"


@pytest.fixture
def app_db(tmp_path):
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tmp_path / "data" / "app.db"


@pytest.fixture
def typed(monkeypatch):
    """Stand in for the terminal. Returns a list to load with answers."""
    answers: list[str] = []
    monkeypatch.setattr("getpass.getpass",
                        lambda prompt="": answers.pop(0) if answers else "")
    return answers


def run(argv) -> tuple[int, str]:
    """Run the CLI, returning `(exit_code, exit_message)`."""
    try:
        main(argv)
    except SystemExit as exc:
        code = exc.code
        if code is None:
            return 0, ""
        if isinstance(code, int):
            return code, ""
        return 1, str(code)
    return 0, ""


def account(username: str = "ada"):
    with db.connection() as con:
        return users.get(con, username)


# ----------------------------------------------------------------- users add

def test_add_creates_an_account_from_a_typed_password(app_db, typed, capsys):
    typed.extend([PASSWORD, PASSWORD])
    assert run(["users", "add", "ada"]) == (0, "")

    ada = account()
    assert ada is not None and ada.username == "ada"
    with db.connection() as con:
        assert users.authenticate(con, "ada", PASSWORD) == ada
    assert "created ada" in capsys.readouterr().out


def test_add_refuses_when_the_two_entries_differ(app_db, typed):
    typed.extend([PASSWORD, "something-else-entirely"])
    code, message = run(["users", "add", "ada"])
    assert code == 1
    assert "did not match" in message
    assert account() is None, "a mistyped confirmation must create nothing"


def test_add_refuses_a_short_password(app_db, typed):
    typed.extend(["short", "short"])
    code, message = run(["users", "add", "ada"])
    assert code == 1
    assert "at least" in message
    assert account() is None


def test_add_takes_an_email_and_an_admin_flag(app_db, typed):
    typed.extend([PASSWORD, PASSWORD])
    assert run(["users", "add", "ada", "--email", "Ada@Example.com",
                "--admin"])[0] == 0
    ada = account()
    assert ada.is_admin
    assert ada.email == "ada@example.com"


def test_add_refuses_a_duplicate(app_db, typed, capsys):
    typed.extend([PASSWORD, PASSWORD, PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    code, message = run(["users", "add", "ADA"])
    assert code == 1
    assert "already registered" in message


def test_add_refuses_a_username_that_could_not_be_one(app_db, typed):
    typed.extend([PASSWORD, PASSWORD])
    code, message = run(["users", "add", "not a username"])
    assert code == 1
    assert "usable username" in message


def test_no_password_creates_an_account_that_cannot_log_in(app_db, capsys):
    """The state an invite leaves behind, available before invites exist."""
    assert run(["users", "add", "ada", "--no-password"])[0] == 0
    out = capsys.readouterr().out
    assert "cannot log in yet" in out

    with db.connection() as con:
        ada = users.get(con, "ada")
        assert not users.has_password(con, ada.id)
        assert users.authenticate(con, "ada", PASSWORD) is None


def test_there_is_no_way_to_pass_a_password_as_an_argument(app_db, typed):
    """ADR 16 in the parser rather than in a docstring.

    A password on the command line lands in shell history and in the process
    table, and a flag that lets one person set another's password is the exact
    thing the invite flow exists to remove. Asserting the flag is *rejected*
    means adding one back has to break this test on purpose.
    """
    for spelling in (["users", "add", "ada", "--password", PASSWORD],
                     ["users", "add", "ada", "-p", PASSWORD],
                     ["users", "passwd", "ada", "--password", PASSWORD]):
        with pytest.raises(SystemExit) as caught:
            main(spelling)
        assert caught.value.code == 2, f"{spelling} was accepted"


# ---------------------------------------------------------------- users list

def test_list_says_so_when_there_is_nobody(app_db, capsys):
    assert run(["users", "list"])[0] == 0
    out = capsys.readouterr().out
    assert "no accounts" in out
    assert "mtglab users add" in out


def test_list_shows_state_and_live_sessions(app_db, typed, capsys):
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada", "--email", "ada@example.com", "--admin"])
    run(["users", "add", "grace", "--no-password"])
    with db.connection() as con:
        sessions.create(con, users.get(con, "ada").id)

    assert run(["users", "list"])[0] == 0
    out = capsys.readouterr().out
    assert "ada" in out and "grace" in out
    assert "no password" in out, "an unclaimed account must be visible as one"
    assert "ada@example.com" in out, "the maintainer's own terminal may see it"
    assert "$argon2" not in out, "a hash must never be printed"


# -------------------------------------------------------------- users passwd

def test_passwd_sets_a_password_and_ends_every_session(app_db, typed, capsys):
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    with db.connection() as con:
        token = sessions.create(con, users.get(con, "ada").id)
        sessions.create(con, users.get(con, "ada").id)

    typed.extend(["a-brand-new-passphrase", "a-brand-new-passphrase"])
    assert run(["users", "passwd", "ada"])[0] == 0
    out = capsys.readouterr().out
    assert "2 session(s) ended" in out

    with db.connection() as con:
        assert sessions.lookup(con, token) is None
        assert users.authenticate(con, "ada", "a-brand-new-passphrase")
        assert users.authenticate(con, "ada", PASSWORD) is None


def test_passwd_on_an_unknown_account_is_refused(app_db, typed):
    code, message = run(["users", "passwd", "nobody"])
    assert code == 1
    assert "no account" in message


def test_passwd_claims_an_unclaimed_account(app_db, typed):
    """The break-glass path until the emailed invite exists: an account with
    no password gets one, typed at the terminal by whoever holds the box."""
    run(["users", "add", "ada", "--no-password"])
    typed.extend([PASSWORD, PASSWORD])
    assert run(["users", "passwd", "ada"])[0] == 0
    with db.connection() as con:
        assert users.authenticate(con, "ada", PASSWORD) is not None


# ------------------------------------------------------- disable and enable

def test_disable_ends_sessions_and_blocks_login(app_db, typed, capsys):
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    with db.connection() as con:
        token = sessions.create(con, users.get(con, "ada").id)

    assert run(["users", "disable", "ada"])[0] == 0
    out = capsys.readouterr().out
    assert "is now disabled" in out and "1 session(s) ended" in out

    with db.connection() as con:
        assert sessions.lookup(con, token) is None
        assert users.authenticate(con, "ada", PASSWORD) is None


def test_enable_puts_the_account_back(app_db, typed, capsys):
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    run(["users", "disable", "ada"])
    assert run(["users", "enable", "ada"])[0] == 0
    assert "is now enabled" in capsys.readouterr().out
    with db.connection() as con:
        assert users.authenticate(con, "ada", PASSWORD) is not None


def test_disable_on_an_unknown_account_is_refused(app_db):
    code, message = run(["users", "disable", "nobody"])
    assert code == 1
    assert "no account" in message


# ------------------------------------------------------------------- the file

def test_the_commands_write_where_config_points(app_db, typed, capsys):
    """`MTGLAB_DATA_DIR` has to move `app.db` with the corpus, or a deployment
    puts the irreplaceable database somewhere that is not the volume."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    assert app_db.exists()
    assert str(app_db) in capsys.readouterr().out


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
