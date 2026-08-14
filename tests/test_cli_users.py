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

from mtglab import config
from mtglab.auth import db, sessions, tokens, users
from mtglab.cli import main

PASSWORD = "a-perfectly-fine-passphrase"


@pytest.fixture
def app_db(tmp_path, monkeypatch):
    # No key and no auth, so `users invite` resolves the console sender and
    # prints the link instead of sending it. That is the development path, and
    # it is also the only one a test may take -- no test sends mail (ADR 16).
    monkeypatch.delenv("RESEND_API_KEY", raising=False)
    monkeypatch.delenv("MTGLAB_REQUIRE_AUTH", raising=False)
    # Every `users` command reconciles the maintainer first (ADR 17). Unset
    # here, so a test that wants that behaviour opts into it and the rest do
    # not silently gain an extra account.
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
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


# -------------------------------------------------------------- users invite

def test_invite_creates_an_unclaimed_account_and_prints_a_link(app_db, capsys):
    """ADR 16's headline: the account exists, cannot log in, and the only way
    it gets a password is a link its own holder follows."""
    assert run(["users", "invite", "ada@example.com"]) == (0, "")

    ada = account()
    assert ada is not None and ada.email == "ada@example.com"
    with db.connection() as con:
        assert not users.has_password(con, ada.id)
        assert users.authenticate(con, "ada", PASSWORD) is None
        assert tokens.outstanding(con, ada.id, tokens.Purpose.INVITE)

    printed = capsys.readouterr()
    assert "invited ada" in printed.out
    # The console sender writes to stderr, and the link is the thing you need.
    assert "/auth/claim#token=" in printed.err
    assert "NOT sent" in printed.err


def test_the_link_it_prints_is_the_one_that_works(app_db, capsys):
    run(["users", "invite", "ada@example.com"])
    link = next(w for w in capsys.readouterr().err.split() if "#token=" in w)
    token = link.partition("#token=")[2]

    with db.connection() as con:
        claimed = tokens.redeem(con, token, PASSWORD)
        assert claimed.username == "ada"
        assert users.authenticate(con, "ada", PASSWORD) is not None


def test_the_username_defaults_to_the_local_part(app_db):
    run(["users", "invite", "ada.lovelace@example.com"])
    assert account("ada.lovelace") is not None


def test_the_username_can_be_given(app_db):
    assert run(["users", "invite", "ada@example.com",
                "--username", "countess"])[0] == 0
    assert account("countess").email == "ada@example.com"


def test_an_admin_can_be_invited(app_db):
    run(["users", "invite", "ada@example.com", "--admin"])
    assert account().is_admin


def test_a_local_part_that_cannot_be_a_username_asks_for_one(app_db):
    code, message = run(["users", "invite", "a+tag@example.com"])
    assert code == 1
    assert "--username" in message
    assert account("a+tag") is None


def test_a_taken_username_asks_for_another(app_db, typed):
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    code, message = run(["users", "invite", "ada@example.com"])
    assert code == 1
    assert "--username" in message


def test_re_inviting_an_unclaimed_account_issues_a_fresh_link(app_db, capsys):
    """The resend path. The first link stops working, which is the point --
    somebody who thinks a message went astray should not leave one live."""
    run(["users", "invite", "ada@example.com"])
    first = next(w for w in capsys.readouterr().err.split() if "#token=" in w)

    assert run(["users", "invite", "ada@example.com"])[0] == 0
    second = next(w for w in capsys.readouterr().err.split() if "#token=" in w)
    assert first != second

    with db.connection() as con:
        with pytest.raises(tokens.InvalidToken):
            tokens.lookup(con, first.partition("#token=")[2])
        assert tokens.lookup(con, second.partition("#token=")[2])
        assert len(users.all_users(con)) == 1, "and no second account"


def test_re_inviting_a_claimed_account_is_refused(app_db, typed):
    """They have a password. What they need is the reset flow, not a second
    account and not somebody else setting one for them."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada", "--email", "ada@example.com"])
    code, message = run(["users", "invite", "ada@example.com"])
    assert code == 1
    assert "already claimed" in message and "reset" in message


def test_an_address_that_is_not_one_is_refused(app_db):
    code, message = run(["users", "invite", "not-an-address"])
    assert code == 1
    assert "email address" in message


def test_invite_is_refused_when_a_deployment_has_no_mail(app_db, monkeypatch):
    """With auth on, the console fallback would print recipients into the
    platform's log, which ADR 16 forbids. Fail loudly instead."""
    monkeypatch.setenv("MTGLAB_REQUIRE_AUTH", "1")
    code, message = run(["users", "invite", "ada@example.com"])
    assert code == 1
    assert "RESEND_API_KEY" in message
    assert account() is None, "and no half-made account is left behind"


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


def test_list_distinguishes_an_invited_account_from_a_stranded_one(app_db,
                                                                   capsys):
    """"Invited" and "no password" are different problems: one is waiting on
    somebody's inbox, the other needs a link sending."""
    run(["users", "invite", "ada@example.com"])
    run(["users", "add", "grace", "--no-password"])
    capsys.readouterr()

    assert run(["users", "list"])[0] == 0
    lines = {line.split()[0]: line
             for line in capsys.readouterr().out.splitlines() if line.strip()}
    assert "invited" in lines["ada"]
    assert "no password" in lines["grace"]


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


# ------------------------------------------------------- promote and demote

def test_promote_grants_admin_after_the_fact(app_db, typed, capsys):
    """`users.set_admin` had no caller until ADR 17; `--admin` at creation was
    the only way to make one, so promoting somebody meant an `UPDATE` by hand."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])

    assert run(["users", "promote", "ada"])[0] == 0
    out = capsys.readouterr().out
    assert "ada is now an admin" in out
    assert account("ada").is_admin


def test_demote_refuses_to_leave_the_instance_without_an_admin(app_db, typed):
    """The guard reaching the terminal. It lives in `auth/users.py`, so this is
    checking that the command reports it rather than that it exists."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "root", "--admin"])

    code, message = run(["users", "demote", "root"])
    assert code == 1
    assert "only admin" in message
    assert account("root").is_admin


def test_disable_refuses_the_last_admin_too(app_db, typed):
    """The likelier accident of the two, and the same refusal."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "root", "--admin"])

    code, message = run(["users", "disable", "root"])
    assert code == 1
    assert "only admin" in message
    assert not account("root").disabled


def test_promoting_the_successor_first_unblocks_the_demotion(app_db, typed,
                                                             capsys):
    """The documented way to hand an instance over."""
    typed.extend([PASSWORD, PASSWORD, PASSWORD, PASSWORD])
    run(["users", "add", "root", "--admin"])
    run(["users", "add", "heir"])

    assert run(["users", "promote", "heir"])[0] == 0
    assert run(["users", "demote", "root"])[0] == 0
    assert not account("root").is_admin
    assert account("heir").is_admin


def test_promoting_an_unclaimed_account_says_it_cannot_sign_in(app_db, capsys):
    """The case where the command works and the instance still has no admin.

    An invited account with the flag counts for nothing until it has a password,
    which is what stops this being a way around the last-admin guard.
    """
    run(["users", "add", "invited", "--no-password"])
    capsys.readouterr()

    assert run(["users", "promote", "invited"])[0] == 0
    out = capsys.readouterr().out
    assert "cannot sign in" in out
    assert "0 admin(s) can sign in" in out


def test_promote_is_idempotent_and_says_so(app_db, typed, capsys):
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "root", "--admin"])
    capsys.readouterr()

    assert run(["users", "promote", "root"])[0] == 0
    assert "already an admin" in capsys.readouterr().out


def test_promote_on_an_unknown_account_is_refused(app_db):
    code, message = run(["users", "promote", "nobody"])
    assert code == 1
    assert "no account" in message


# ------------------------------------------------------- the maintainer bootstrap

def test_a_users_command_reconciles_the_maintainer(app_db, monkeypatch, capsys):
    """ADR 17 runs before every `users` command, not only at app startup.

    Otherwise the CLI and the app would disagree about who administers the
    instance depending on which one ran last -- and the CLI is the path that
    exists precisely for when the app is what is broken.
    """
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")

    assert run(["users", "list"])[0] == 0
    out = capsys.readouterr().out
    assert "maintainer@example.com" in out
    assert account("maintainer").is_admin


def test_a_demoted_maintainer_is_restored_by_the_next_command(app_db,
                                                              monkeypatch):
    """Reconciliation is the guarantee; the last-admin guard is not.

    Worth pinning together because the interaction is not the obvious one. A
    freshly bootstrapped maintainer has no password, so it is not yet an admin
    who *can sign in* and the guard does not protect it — the demotion here is
    allowed. What makes the requirement hold anyway is that the next start puts
    it back. Two mechanisms, and this is the seam between them.
    """
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")
    run(["users", "list"])

    assert run(["users", "demote", "maintainer"])[0] == 0
    assert not account("maintainer").is_admin

    run(["users", "list"])
    assert account("maintainer").is_admin


def test_a_claimed_maintainer_is_protected_by_the_guard_as_well(app_db, typed,
                                                                monkeypatch):
    """Once it has a password, both mechanisms hold it: refused, not repaired."""
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")
    run(["users", "list"])
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "passwd", "maintainer"])

    code, message = run(["users", "demote", "maintainer"])
    assert code == 1
    assert "only admin" in message


# ---------------------------------------------------------------- users delete

def test_delete_wants_the_username_typed_back(app_db, typed, monkeypatch):
    """`decks delete`'s shape: a word produced, not a prompt clicked past."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    monkeypatch.setattr("builtins.input", lambda prompt="": "adam")

    code, message = run(["users", "delete", "ada"])

    assert code == 1
    assert "not the username" in message
    assert account("ada") is not None


def test_delete_removes_the_account_when_confirmed(app_db, typed, monkeypatch):
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    monkeypatch.setattr("builtins.input", lambda prompt="": "ada")

    assert run(["users", "delete", "ada"]) == (0, "")
    assert account("ada") is None


def test_delete_yes_skips_the_prompt(app_db, typed):
    """For scripts. The name is still on the command line, so nothing is
    deleted that the caller did not type out."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])

    assert run(["users", "delete", "ada", "--yes"]) == (0, "")
    assert account("ada") is None


def test_delete_refuses_the_last_admin(app_db, typed, monkeypatch):
    """The guard lives in `auth/users.py`, so the CLI inherits it rather than
    implementing a second opinion about it (ADR 17)."""
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")
    run(["users", "list"])
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "passwd", "maintainer"])

    code, message = run(["users", "delete", "maintainer", "--yes"])

    assert code == 1
    assert "only admin" in message
    assert account("maintainer") is not None


def test_delete_refuses_somebody_who_is_not_there(app_db):
    code, message = run(["users", "delete", "nobody", "--yes"])
    assert code == 1
    assert "no account" in message


# ------------------------------------------------------------------- the file

def test_the_commands_write_where_config_points(app_db, typed, capsys):
    """`MTGLAB_DATA_DIR` has to move `app.db` with the pool, or a deployment
    puts the irreplaceable database somewhere that is not the volume."""
    typed.extend([PASSWORD, PASSWORD])
    run(["users", "add", "ada"])
    assert app_db.exists()
    assert str(app_db) in capsys.readouterr().out


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
