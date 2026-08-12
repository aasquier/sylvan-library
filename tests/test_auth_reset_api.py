"""`POST /api/auth/reset` and `POST /api/auth/claim` over HTTP.

`tests/test_auth_tokens.py` covers the rules; this covers the two routes that
carry them, and the properties that only exist at the HTTP layer.

The one that matters most is at the top of the file: **the reset endpoint
answers identically whether or not the address exists** (ADR 16). Identical is
asserted byte for byte, on the status and the body, because "similar" is how
that kind of leak survives review — and the send happens in a background task
so that the *timing* carries no signal either.

No test here sends mail. The app is built with a recorder passed to
`create_app(email_sender=...)`, which is the seam reaching the edge.
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
from mtglab.api.auth import COOKIE_NAME, RESET_ANSWER  # noqa: E402
from mtglab.auth import db, mail, ratelimit, sessions, tokens, users  # noqa: E402

PASSWORD = "a-perfectly-fine-passphrase"
NEW_PASSWORD = "a-completely-different-passphrase"


class Recorder:
    """An `EmailSender` that keeps what it was given, and sends nothing."""

    def __init__(self) -> None:
        self.sent: list[mail.Message] = []

    def send(self, message: mail.Message) -> None:
        self.sent.append(message)

    @property
    def links(self) -> list[str]:
        return [word for message in self.sent for word in message.body.split()
                if word.startswith("http")]

    @property
    def tokens(self) -> list[str]:
        return [link.partition("#token=")[2] for link in self.links]


@pytest.fixture
def mailbox():
    return Recorder()


@pytest.fixture
def secured(tmp_path, mailbox):
    """Auth on, one claimed account and one still unclaimed."""
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        con = db.connect()
        try:
            users.create(con, "ada", password=PASSWORD,
                         email="ada@example.com")
            users.create(con, "grace", email="grace@example.com")
        finally:
            con.close()
        with TestClient(create_app(require_auth=True, secure_cookies=False,
                                   email_sender=mailbox)) as client:
            yield client


def reset(client, email: str):
    return client.post("/api/auth/reset", json={"email": email})


def claim(client, token: str, password: str = NEW_PASSWORD):
    return client.post("/api/auth/claim",
                       json={"token": token, "password": password})


# ------------------------------------------------------- the identical answer

def test_a_reset_for_a_real_address_is_accepted(secured, mailbox):
    response = reset(secured, "ada@example.com")
    assert response.status_code == 202
    assert response.json() == {"detail": RESET_ANSWER}
    assert len(mailbox.sent) == 1


def test_an_unknown_address_gets_a_byte_identical_answer(secured, mailbox):
    """The whole endpoint, in one assertion. Any difference at all -- status,
    body, a stray field -- answers "does this account exist"."""
    real = reset(secured, "ada@example.com")
    unknown = reset(secured, "nobody@example.com")

    assert real.status_code == unknown.status_code == 202
    assert real.json() == unknown.json()
    assert len(mailbox.sent) == 1, "and only one of them sent anything"


def test_a_disabled_account_gets_the_identical_answer_too(secured, mailbox):
    with db.connection() as con:
        users.set_disabled(con, users.get(con, "ada").id, True)
    disabled = reset(secured, "ada@example.com")
    unknown = reset(secured, "nobody@example.com")

    assert disabled.status_code == unknown.status_code
    assert disabled.json() == unknown.json()
    assert mailbox.sent == []


def test_something_that_is_not_an_address_gets_it_as_well(secured, mailbox):
    """A shape check that answered 422 would be a cheaper existence oracle
    than the endpoint it guards."""
    malformed = reset(secured, "not-an-address")
    assert malformed.status_code == 202
    assert malformed.json() == reset(secured, "nobody@example.com").json()
    assert mailbox.sent == []


def test_a_missing_field_is_422(secured):
    """A malformed *request* is a different thing from a refused one, and this
    one says nothing about any address because it carries none."""
    assert secured.post("/api/auth/reset", json={}).status_code == 422


def test_the_reset_endpoint_needs_no_session(secured):
    """It is on the public list, which is the only way somebody locked out can
    reach it. `tests/test_isolation.py` reads that list."""
    from mtglab.api.auth import PUBLIC_PATHS
    assert "/api/auth/reset" in PUBLIC_PATHS
    assert "/api/auth/claim" in PUBLIC_PATHS


def test_the_response_carries_no_address_and_no_token(secured, mailbox):
    body = reset(secured, "ada@example.com").text
    assert "ada@example.com" not in body
    assert mailbox.tokens[0] not in body


def test_no_address_reaches_the_log(secured, caplog):
    """ADR 16 is unconditional about this, and the reset form is a field
    somebody types their address into by design."""
    with caplog.at_level("INFO", logger="mtglab.auth"):
        reset(secured, "ada@example.com")
        reset(secured, "nobody@example.com")
    assert "ada@example.com" not in caplog.text
    assert "example.com" not in caplog.text
    assert "password reset requested" in caplog.text


def test_a_mail_failure_is_logged_and_not_returned(tmp_path, caplog):
    """A provider outage must not become a 500 that says "this address exists,
    and something went wrong sending to it"."""
    class Broken:
        def send(self, message):
            raise mail.EmailNotSent("the mail provider refused: HTTP 500")

    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        con = db.connect()
        users.create(con, "ada", password=PASSWORD, email="ada@example.com")
        con.close()
        with TestClient(create_app(require_auth=True, secure_cookies=False,
                                   email_sender=Broken())) as client, \
                caplog.at_level("ERROR", logger="mtglab.auth"):
            response = reset(client, "ada@example.com")

    assert response.status_code == 202
    assert response.json() == {"detail": RESET_ANSWER}
    assert "could not be delivered" in caplog.text
    assert "ada@example.com" not in caplog.text


# ------------------------------------------------------------ the rate limit

def test_the_mailbox_budget_runs_out(secured):
    for _ in range(ratelimit.RESET_PER_MAILBOX.failures):
        assert reset(secured, "ada@example.com").status_code == 202
    throttled = reset(secured, "ada@example.com")
    assert throttled.status_code == 429
    assert int(throttled.headers["Retry-After"]) > 0


def test_the_budget_is_spent_by_addresses_that_do_not_exist(secured):
    """Every request counts, hit or miss -- a reset has no success to clear the
    counter with. It also means being throttled says nothing about whether an
    account is behind the address."""
    for _ in range(ratelimit.RESET_PER_MAILBOX.failures):
        reset(secured, "nobody@example.com")
    assert reset(secured, "nobody@example.com").status_code == 429
    assert reset(secured, "ada@example.com").status_code == 202


def test_the_client_budget_runs_out_across_mailboxes(secured):
    """Otherwise one client mail-bombs a list of addresses three at a time."""
    for i in range(ratelimit.RESET_PER_ADDRESS.failures):
        assert reset(secured, f"person{i}@example.com").status_code == 202
    assert reset(secured, "yet-another@example.com").status_code == 429


def test_the_mailbox_is_not_stored_in_the_clear(secured):
    """A limiter keyed on plaintext would accumulate the addresses of people
    who do not even have accounts."""
    reset(secured, "nobody@example.com")
    with db.connection() as con:
        keys = [r["key"] for r in con.execute("SELECT key FROM login_attempts")]
    assert keys, "the attempt was recorded"
    assert not any("nobody@example.com" in key for key in keys)
    assert ratelimit.email_key("nobody@example.com") in keys


def test_a_reset_budget_does_not_spend_the_login_budget(secured):
    """Scoped IP keys: being throttled on one unauthenticated endpoint must not
    lock somebody out of the others."""
    for i in range(ratelimit.RESET_PER_ADDRESS.failures + 1):
        reset(secured, f"person{i}@example.com")
    assert secured.post("/api/auth/login",
                        json={"username": "ada",
                              "password": PASSWORD}).status_code == 200


# ------------------------------------------------------------------- claim

def test_an_invited_account_claims_itself_end_to_end(secured, mailbox):
    """The whole of ADR 16 in one test: a link arrives, its holder chooses a
    password nobody else has ever seen, and then they can log in."""
    from mtglab.auth import invites

    with db.connection() as con:
        invites.send_invite(con, users.get(con, "grace"), sender=mailbox)

    assert secured.post("/api/auth/login",
                        json={"username": "grace",
                              "password": NEW_PASSWORD}).status_code == 401

    claimed = claim(secured, mailbox.tokens[0])
    assert claimed.status_code == 200
    assert claimed.json()["username"] == "grace"

    assert secured.post("/api/auth/login",
                        json={"username": "grace",
                              "password": NEW_PASSWORD}).status_code == 200


def test_a_reset_link_changes_the_password_and_ends_every_session(secured,
                                                                 mailbox):
    with db.connection() as con:
        ada = users.get(con, "ada")
        token_of_a_live_session = sessions.create(con, ada.id)

    reset(secured, "ada@example.com")
    assert claim(secured, mailbox.tokens[0]).status_code == 200

    with db.connection() as con:
        assert sessions.lookup(con, token_of_a_live_session) is None
        assert users.authenticate(con, "ada", NEW_PASSWORD) is not None
        assert users.authenticate(con, "ada", PASSWORD) is None


def test_claiming_does_not_log_you_in(secured, mailbox):
    """A cookie here would make an emailed link a session-minting endpoint, to
    save one trip through a login form that has to work anyway."""
    reset(secured, "ada@example.com")
    response = claim(secured, mailbox.tokens[0])
    assert not response.cookies.get(COOKIE_NAME)
    assert secured.get("/api/decks").status_code == 401


def test_a_forged_token_is_refused(secured):
    for forgery in ("made-up-token", "x" * 43):
        response = claim(secured, forgery)
        assert response.status_code == 400
        assert response.json()["detail"] == "that link is not valid"


def test_an_empty_token_is_a_malformed_request(secured):
    """422 rather than 400: nothing was submitted to be refused."""
    assert claim(secured, "").status_code == 422


def test_a_used_link_is_refused_the_second_time(secured, mailbox):
    reset(secured, "ada@example.com")
    token = mailbox.tokens[0]
    assert claim(secured, token).status_code == 200

    again = claim(secured, token, "a-third-perfectly-fine-phrase")
    assert again.status_code == 400
    assert "already been used" in again.json()["detail"]
    with db.connection() as con:
        assert users.authenticate(con, "ada", NEW_PASSWORD) is not None


def test_an_expired_link_says_so(secured):
    from datetime import timedelta
    with db.connection() as con:
        token = tokens.issue(con, users.get(con, "ada").id,
                             tokens.Purpose.RESET,
                             lifetime=timedelta(seconds=-1))
    response = claim(secured, token)
    assert response.status_code == 400
    assert "expired" in response.json()["detail"]


def test_a_weak_password_is_422_and_leaves_the_link_alive(secured, mailbox):
    reset(secured, "ada@example.com")
    token = mailbox.tokens[0]

    refused = claim(secured, token, "short")
    assert refused.status_code == 422
    assert "at least" in refused.json()["detail"]

    assert claim(secured, token).status_code == 200, "the link still works"


def test_a_missing_field_is_422_on_claim(secured):
    assert secured.post("/api/auth/claim",
                        json={"token": "x"}).status_code == 422


def test_repeated_bad_tokens_run_out_of_budget(secured):
    """`claim` is unauthenticated and hashes with Argon2, so it gets a ceiling
    even though the token in front of that hash is unguessable."""
    for _ in range(ratelimit.CLAIM_PER_ADDRESS.failures):
        assert claim(secured, "made-up-token").status_code == 400
    assert claim(secured, "made-up-token").status_code == 429


def test_a_successful_claim_restores_the_budget(secured, mailbox):
    for _ in range(ratelimit.CLAIM_PER_ADDRESS.failures - 1):
        claim(secured, "made-up-token")
    reset(secured, "ada@example.com")
    assert claim(secured, mailbox.tokens[0]).status_code == 200
    assert claim(secured, "made-up-token").status_code == 400


def test_claiming_is_logged_by_username_and_not_by_address(secured, mailbox,
                                                           caplog):
    reset(secured, "ada@example.com")
    with caplog.at_level("INFO", logger="mtglab.auth"):
        claim(secured, mailbox.tokens[0])
    assert "password set" in caplog.text
    assert "ada" in caplog.text
    assert "example.com" not in caplog.text
    assert NEW_PASSWORD not in caplog.text


# -------------------------------------------------- the local configuration

def test_the_routes_exist_with_auth_off(tmp_path, mailbox):
    """The local app has no login, but it still has the flow -- which is how it
    gets exercised against a real browser on a laptop."""
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        con = db.connect()
        users.create(con, "ada", email="ada@example.com")
        con.close()
        with TestClient(create_app(email_sender=mailbox)) as client:
            assert reset(client, "ada@example.com").status_code == 202
            assert claim(client, mailbox.tokens[0]).status_code == 200


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
