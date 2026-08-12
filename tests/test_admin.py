"""Admin authorization: the last-admin refusal, the bootstrap, and the routes.

[ADR 17](../docs/adr/0017-the-maintainer-is-named-in-the-environment.md) in
three parts, and the first two are the ones with teeth:

- **`users.set_admin` and `users.set_disabled` refuse to remove the last admin
  who can sign in.** In the core, so the CLI and the routes inherit it. The
  interesting cases are all about what "can sign in" means — an unclaimed invite
  with the admin flag is not an administrator of anything, and a guard that
  counted it would report success while the lockout happened.
- **`bootstrap.ensure_maintainer` reconciles the configured address every
  start**, which is the difference between a guarantee and a one-time event.
- The five routes, which are mostly `mtglab users` with HTTP status codes.

`tests/test_isolation.py` owns *who may reach* the routes; this file owns what
they do once reached. No test here sends mail — the app is built with a recorder
passed to `create_app(email_sender=...)`, the same seam ADR 16 put there.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("fastapi")
pytest.importorskip("httpx")
pytest.importorskip("argon2")

from fastapi.testclient import TestClient  # noqa: E402

from mtglab import config  # noqa: E402
from mtglab.api import jobs  # noqa: E402
from mtglab.api.app import create_app  # noqa: E402
from mtglab.auth import bootstrap, db, mail, sessions, tokens, users  # noqa: E402

PASSWORD = "correct-horse-battery-staple"
OTHER = "a-different-long-passphrase"


class Recorder:
    """An `EmailSender` that keeps what it was given, and sends nothing."""

    def __init__(self) -> None:
        self.sent: list[mail.Message] = []

    def send(self, message: mail.Message) -> None:
        self.sent.append(message)


@pytest.fixture
def con(tmp_path):
    """A scratch `app.db`, with no accounts in it."""
    with config.use_paths(data_dir=tmp_path / "data"):
        connection = db.connect()
        try:
            yield connection
        finally:
            connection.close()


# ------------------------------------------------ the last-admin refusal

def test_the_only_admin_cannot_be_demoted(con):
    """The refusal itself, and the reason `set_admin` needed a guard at all."""
    root = users.create(con, "root", password=PASSWORD, is_admin=True)
    users.create(con, "friend", password=OTHER)

    with pytest.raises(users.LastAdmin):
        users.set_admin(con, root.id, False)

    assert users.get(con, "root").is_admin


def test_the_only_admin_cannot_be_disabled(con):
    """The other door to the same lockout, and the easier one to walk through.

    Disabling is how an account is revoked, so it is the operation somebody
    reaches for in a hurry -- and `disable root` is one keystroke from an
    instance nobody can administer.
    """
    root = users.create(con, "root", password=PASSWORD, is_admin=True)

    with pytest.raises(users.LastAdmin):
        users.set_disabled(con, root.id, True)

    assert not users.get(con, "root").disabled


def test_a_second_admin_makes_the_first_demotable(con):
    """The intended way to hand an instance over: promote, then demote.

    The ordering is forced rather than advised, which is the point of the guard.
    """
    root = users.create(con, "root", password=PASSWORD, is_admin=True)
    heir = users.create(con, "heir", password=OTHER)

    users.set_admin(con, heir.id, True)
    users.set_admin(con, root.id, False)

    assert not users.get(con, "root").is_admin
    assert users.usable_admin_ids(con) == {heir.id}


def test_an_unclaimed_admin_does_not_count(con):
    """An invite with the admin flag cannot administer anything yet.

    This is the case a naive `WHERE is_admin = 1` gets wrong, and it gets it
    wrong in the worst direction: it reports two admins, permits the demotion,
    and leaves an instance whose only administrator has no password to log in
    with. ADR 17 says "can actually sign in" for exactly this.
    """
    root = users.create(con, "root", password=PASSWORD, is_admin=True)
    invited = users.create(con, "invited", email="a@b.com", is_admin=True)

    assert users.usable_admin_ids(con) == {root.id}
    assert invited.is_admin

    with pytest.raises(users.LastAdmin):
        users.set_admin(con, root.id, False)


def test_a_disabled_admin_does_not_count_either(con):
    """Same predicate, the other clause. Disabled is not administering."""
    root = users.create(con, "root", password=PASSWORD, is_admin=True)
    other = users.create(con, "other", password=OTHER, is_admin=True)
    users.set_disabled(con, other.id, True)

    assert users.usable_admin_ids(con) == {root.id}
    with pytest.raises(users.LastAdmin):
        users.set_disabled(con, root.id, True)


def test_operations_on_everybody_else_are_untouched(con):
    """The guard must not become a tax on ordinary account management.

    Only the *last* usable admin is protected. A non-admin, a second admin, and
    an account that is already disabled are all unaffected -- and re-enabling
    is never refused, because it cannot reduce the count.
    """
    users.create(con, "root", password=PASSWORD, is_admin=True)
    friend = users.create(con, "friend", password=OTHER)

    assert users.set_disabled(con, friend.id, True) == 0
    users.set_disabled(con, friend.id, False)
    users.set_admin(con, friend.id, True)
    users.set_admin(con, friend.id, False)
    assert not users.get(con, "friend").disabled


def test_an_instance_with_no_usable_admin_is_not_frozen(con):
    """Zero admins must not mean nothing can be disabled.

    The guard refuses to take the count from one to zero. It has no opinion
    about an instance that is already at zero -- refusing there would mean a
    misconfigured deployment could not even revoke an account while somebody
    sorted the admin situation out.
    """
    users.create(con, "nobody", password=PASSWORD)
    friend = users.create(con, "friend", password=OTHER)

    assert users.usable_admin_ids(con) == set()
    users.set_disabled(con, friend.id, True)
    assert users.get(con, "friend").disabled


def test_a_refused_change_leaves_no_half_written_row(con):
    """The refusal happens inside the transaction, so nothing is committed.

    `set_disabled` writes two statements -- the flag, then the session sweep.
    A guard placed after the first would revoke the sessions of an account it
    then declined to disable.
    """
    root = users.create(con, "root", password=PASSWORD, is_admin=True)
    sessions.create(con, root.id)

    with pytest.raises(users.LastAdmin):
        users.set_disabled(con, root.id, True)

    assert sessions.count_for_user(con, root.id) == 1
    assert not users.get(con, "root").disabled


def test_demotion_does_not_sign_anybody_out(con):
    """Deliberate, and the opposite of what `set_password` does.

    Admin is read fresh from the account on every request, so a demotion takes
    effect on the next call without ending a session the person is still
    entitled to have.
    """
    root = users.create(con, "root", password=PASSWORD, is_admin=True)
    heir = users.create(con, "heir", password=OTHER, is_admin=True)
    sessions.create(con, root.id)

    users.set_admin(con, root.id, False)
    assert sessions.count_for_user(con, root.id) == 1
    assert users.usable_admin_ids(con) == {heir.id}


# --------------------------------------------------------- the bootstrap

def test_unset_does_nothing(con, monkeypatch):
    """A laptop must not acquire an account as a side effect of starting."""
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    assert bootstrap.ensure_maintainer(con) is None
    assert users.count(con) == 0


def test_it_creates_an_unclaimed_admin(con, monkeypatch):
    """A fresh volume gets an admin account nobody has a password for.

    Unclaimed rather than disabled, which is ADR 16's shape: the account exists,
    cannot log in yet, and becomes usable through a link its owner redeems. No
    password is invented and none is printed.
    """
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "Maintainer@Example.COM")

    made = bootstrap.ensure_maintainer(con)

    assert made is not None
    assert made.username == "maintainer"
    assert made.email == "maintainer@example.com"
    assert made.is_admin
    assert not made.disabled
    assert not users.has_password(con, made.id)


def test_it_is_idempotent(con, monkeypatch):
    """Every boot after the first finds the account already correct."""
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")

    first = bootstrap.ensure_maintainer(con)
    second = bootstrap.ensure_maintainer(con)

    assert first is not None and second is not None
    assert first.id == second.id
    assert users.count(con) == 1


def test_it_promotes_an_existing_account(con, monkeypatch):
    """The reconciliation half, and the reason this beat first-account-wins.

    A demotion is repaired by a restart, with no shell involved. First-account-
    wins could not do this: it says something about the moment an instance was
    created and nothing about the instance a week later.
    """
    existing = users.create(con, "aaron", password=PASSWORD,
                            email="maintainer@example.com")
    assert not existing.is_admin
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")

    reconciled = bootstrap.ensure_maintainer(con)

    assert reconciled is not None
    assert reconciled.is_admin
    assert reconciled.username == "aaron"        # the account, not a new one
    assert users.count(con) == 1


def test_it_re_enables_a_disabled_maintainer(con, monkeypatch):
    """The break-glass half. Whoever sets the variable can already deploy code."""
    existing = users.create(con, "aaron", password=PASSWORD,
                            email="maintainer@example.com", is_admin=True)
    other = users.create(con, "other", password=OTHER, is_admin=True)
    users.set_disabled(con, existing.id, True)
    assert users.usable_admin_ids(con) == {other.id}

    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")
    reconciled = bootstrap.ensure_maintainer(con)

    assert reconciled is not None
    assert not reconciled.disabled
    assert reconciled.is_admin


def test_a_taken_username_does_not_rename_anybody(con, monkeypatch):
    """A friend already holding the obvious handle keeps it."""
    users.create(con, "aaron", password=PASSWORD, email="friend@example.com")
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "aaron@example.com")

    made = bootstrap.ensure_maintainer(con)

    assert made is not None
    assert made.username == "aaron2"
    assert users.get(con, "aaron").email == "friend@example.com"
    assert not users.get(con, "aaron").is_admin


@pytest.mark.parametrize("address,expected", [
    ("ada.lovelace@example.com", "ada.lovelace"),
    ("ada+mtg@example.com", "adamtg"),          # `+` is not a username character
    ("a@example.com", "admin"),                 # too short to be one at all
    ("...@example.com", "admin"),               # nothing usable left
])
def test_the_username_is_derived_and_never_refused(address, expected):
    """Unattended, so it mangles rather than asking.

    `mtglab users invite` answers this differently on purpose -- it refuses and
    asks for `--username`, because an invited person has to be told the handle
    they were given. Here there is nobody to ask, and refusing would mean an
    instance with no admin because an address had a `+` in it.
    """
    assert bootstrap.username_for(address) == expected


def test_a_malformed_address_is_loud_and_not_fatal(con, monkeypatch, caplog):
    """A typo in one variable must not be an instance that serves nothing."""
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "not-an-address")

    with caplog.at_level("ERROR"):
        assert bootstrap.ensure_maintainer(con) is None

    assert users.count(con) == 0
    assert "MTGLAB_ADMIN_EMAIL" in caplog.text


def test_the_app_reconciles_when_it_starts_serving(tmp_path, monkeypatch):
    """The wiring, not just the function — and *when* it fires.

    On the lifespan rather than in `create_app`, because this module builds an
    `app` at import for uvicorn. Building one has to stay free of side effects:
    otherwise importing `mtglab.api.app`, which the CLI and every API test do,
    would create an `app.db` wherever the environment happened to point.
    """
    monkeypatch.setenv("MTGLAB_ADMIN_EMAIL", "maintainer@example.com")
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        app = create_app(require_auth=True, secure_cookies=False)
        assert not (tmp_path / "data" / "app.db").exists(), \
            "building the app touched the database"

        with TestClient(app):
            pass

        with db.connection() as connection:
            account = users.get_by_email(connection, "maintainer@example.com")
            assert account is not None and account.is_admin


# ------------------------------------------------------------ the routes

@pytest.fixture
def mailbox():
    return Recorder()


@pytest.fixture
def client(tmp_path, mailbox, monkeypatch):
    """Auth on, logged in as an admin, with a second ordinary account."""
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        connection = db.connect()
        try:
            users.create(connection, "root", password=PASSWORD,
                         email="root@example.com", is_admin=True)
            users.create(connection, "friend", password=OTHER,
                         email="friend@example.com")
        finally:
            connection.close()
        app = create_app(require_auth=True, secure_cookies=False,
                         email_sender=mailbox)
        with TestClient(app) as test_client:
            assert test_client.post(
                "/api/auth/login",
                json={"username": "root", "password": PASSWORD}
            ).status_code == 200
            yield test_client


def test_the_list_carries_addresses_states_and_the_admin_count(client):
    """What the page needs, and the one field ADR 17 decided to expose.

    `admins` is the count of admins who can sign in, so the page can grey out
    the last demote button rather than offering a click that returns 409.
    """
    body = client.get("/api/admin/users").json()

    assert body["admins"] == 1
    by_name = {u["username"]: u for u in body["users"]}
    assert by_name["root"]["email"] == "root@example.com"
    assert by_name["root"]["is_admin"] is True
    assert by_name["root"]["state"] == "active"
    assert by_name["root"]["sessions"] == 1
    assert by_name["friend"]["state"] == "active"
    assert by_name["friend"]["sessions"] == 0


def test_inviting_creates_an_unclaimed_account_and_mails_a_link(client, mailbox):
    response = client.post("/api/admin/users",
                           json={"email": "New.Person@example.com"})

    assert response.status_code == 201
    assert response.json()["username"] == "new.person"
    assert response.json()["state"] == "invited"
    assert len(mailbox.sent) == 1
    assert mailbox.sent[0].to == "new.person@example.com"
    assert "#token=" in mailbox.sent[0].body


def test_inviting_a_claimed_address_points_at_the_reset_flow(client, mailbox):
    """Not a way to re-issue somebody's credentials. ADR 16."""
    response = client.post("/api/admin/users",
                           json={"email": "friend@example.com"})

    assert response.status_code == 409
    assert "reset" in response.json()["detail"]
    assert mailbox.sent == []


def test_re_inviting_an_unclaimed_account_is_the_resend_path(client, mailbox):
    client.post("/api/admin/users", json={"email": "new@example.com"})
    again = client.post("/api/admin/users", json={"email": "new@example.com"})

    assert again.status_code == 201
    assert len(mailbox.sent) == 2
    with db.connection() as connection:
        account = users.get_by_email(connection, "new@example.com")
        assert account is not None
        # One live link per purpose: issuing the second drops the first, so the
        # message somebody worries went astray stops working when they ask again.
        assert tokens.outstanding(connection, account.id,
                                  tokens.Purpose.INVITE) == 1


def test_an_unusable_address_is_refused_before_anything_is_written(client):
    response = client.post("/api/admin/users", json={"email": "not-an-address"})

    assert response.status_code == 422
    assert client.get("/api/admin/users").json()["admins"] == 1


def test_promoting_and_disabling_go_through_one_route(client):
    promoted = client.patch("/api/admin/users/friend", json={"is_admin": True})
    assert promoted.status_code == 200
    assert promoted.json()["is_admin"] is True
    assert client.get("/api/admin/users").json()["admins"] == 2

    disabled = client.patch("/api/admin/users/friend", json={"disabled": True})
    assert disabled.status_code == 200
    assert disabled.json()["state"] == "disabled"


def test_the_route_inherits_the_last_admin_refusal(client):
    """A 409, and the rule that produced it is not written in the handler."""
    response = client.patch("/api/admin/users/root", json={"is_admin": False})

    assert response.status_code == 409
    assert "only admin" in response.json()["detail"]
    assert client.get("/api/admin/users").json()["admins"] == 1


def test_an_admin_cannot_disable_themselves_out_of_the_instance(client):
    """The same refusal by the other door, and the likelier accident."""
    response = client.patch("/api/admin/users/root", json={"disabled": True})

    assert response.status_code == 409
    assert client.post("/api/auth/logout").status_code == 200


def test_patching_nothing_is_refused_rather_than_reported_as_success(client):
    assert client.patch("/api/admin/users/friend", json={}).status_code == 422


def test_an_unknown_account_is_a_404(client):
    """404 means "no such username" here, and says so.

    ADR 5's 404-not-403 rule is about hiding a resource's existence from
    somebody it does not belong to. An admin listing accounts is the one caller
    for whom every account is in scope, so there is nothing to hide and the
    status code carries its ordinary meaning.
    """
    assert client.patch("/api/admin/users/nobody",
                        json={"disabled": True}).status_code == 404
    assert client.post("/api/admin/users/nobody/reset").status_code == 404


def test_reset_mails_the_account_holder_rather_than_setting_a_password(
        client, mailbox):
    """The whole of what an admin can do about a forgotten password (ADR 16)."""
    response = client.post("/api/admin/users/friend/reset")

    assert response.status_code == 202
    assert len(mailbox.sent) == 1
    assert mailbox.sent[0].to == "friend@example.com"
    assert "reset" in mailbox.sent[0].subject.lower()


def test_a_disabled_account_gets_no_reset_link(client, mailbox):
    """A lever the disabled party can undo from their own inbox is not a lever.

    `invites.send_reset` declines this silently, which is right for the public
    endpoint and wrong as an answer to an admin who pressed a button -- so the
    route says it out loud instead of reporting a send that did not happen.
    """
    client.patch("/api/admin/users/friend", json={"disabled": True})
    response = client.post("/api/admin/users/friend/reset")

    assert response.status_code == 409
    assert mailbox.sent == []


def test_revoking_sessions_signs_an_account_out_without_disabling_it(client):
    """The lighter revocation: the account is still good, the cookies are not."""
    with db.connection() as connection:
        friend = users.get(connection, "friend")
        assert friend is not None
        sessions.create(connection, friend.id)
        sessions.create(connection, friend.id)

    response = client.delete("/api/admin/users/friend/sessions")

    assert response.status_code == 200
    assert response.json()["revoked"] == 2
    body = client.get("/api/admin/users").json()
    friend_row = next(u for u in body["users"] if u["username"] == "friend")
    assert friend_row["sessions"] == 0
    assert friend_row["state"] == "active"       # still able to log back in


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
