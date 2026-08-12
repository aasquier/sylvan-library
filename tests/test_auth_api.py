"""Login, logout, the cookie, and the two ways the app can be configured.

`tests/test_auth.py` covers the rules; this covers the HTTP that carries them.
The split matters because most of what could go wrong here is not in the auth
package at all — it is a cookie flag, a status code, or a middleware that
resolves the caller after routing instead of before.

Both configurations are exercised. **Auth off is the default and the way the
app has always run**, so it gets tests of its own: `mtglab ui` on a laptop must
not have grown a login, and a regression there would be invisible to every
other file in this suite.
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
from mtglab.api.auth import COOKIE_NAME  # noqa: E402
from mtglab.auth import db, ratelimit, sessions, users  # noqa: E402

PASSWORD = "a-perfectly-fine-passphrase"


@pytest.fixture
def secured(tmp_path):
    """The deployed configuration: auth required, one account, scratch db."""
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"):
        con = db.connect()
        try:
            users.create(con, "ada", password=PASSWORD, email="ada@example.com")
        finally:
            con.close()
        with TestClient(create_app(require_auth=True,
                                   secure_cookies=False)) as client:
            yield client


@pytest.fixture
def local(tmp_path, monkeypatch):
    """The default configuration: no auth, one person, a laptop.

    `MTGLAB_ADMIN_EMAIL` is cleared because the app's lifespan reconciles it
    (ADR 17) — with it set in a developer's `.env`, starting a client would
    create the `app.db` that `test_the_local_app_touches_no_database` exists to
    say it does not. The configuration under test is a laptop with nothing set.
    """
    monkeypatch.delenv("MTGLAB_ADMIN_EMAIL", raising=False)
    jobs.clear()
    with config.use_paths(data_dir=tmp_path / "data"), \
            TestClient(create_app()) as client:
        yield client


def login(client, username: str = "ada", password: str = PASSWORD):
    return client.post("/api/auth/login",
                       json={"username": username, "password": password})


# ------------------------------------------------------------ the happy path

def test_login_sets_a_session_cookie(secured):
    response = login(secured)
    assert response.status_code == 200
    assert response.json()["user"]["username"] == "ada"
    assert secured.cookies.get(COOKIE_NAME)


def test_the_cookie_carries_the_flags_that_make_it_safe(tmp_path):
    """`HttpOnly` and `SameSite=Lax` are the XSS and CSRF halves; `Secure` is
    the one that has to be *off* locally or the browser never sends it back."""
    with config.use_paths(data_dir=tmp_path / "data"):
        con = db.connect()
        users.create(con, "ada", password=PASSWORD)
        con.close()
        with TestClient(create_app(require_auth=True,
                                   secure_cookies=True)) as client:
            header = login(client).headers["set-cookie"]

    assert "HttpOnly" in header
    assert "SameSite=lax" in header.replace("samesite", "SameSite")
    assert "Secure" in header
    assert "Path=/" in header
    assert "Max-Age=" in header


def test_the_session_token_is_not_in_the_response_body(secured):
    """Only the cookie carries it. A token in a JSON body is a token in a log."""
    body = login(secured).text
    token = secured.cookies.get(COOKIE_NAME)
    assert token not in body


def test_me_reports_the_logged_in_account(secured):
    login(secured)
    body = secured.get("/api/auth/me").json()
    assert body == {"auth_required": True, "authenticated": True,
                    "is_admin": False,
                    "user": {"id": 1, "username": "ada", "is_admin": False}}


def test_me_does_not_hand_out_the_email(secured):
    """First personal data the project stores (ADR 16). It goes where it is
    needed and nowhere else, and `/me` does not need it."""
    login(secured)
    assert "example.com" not in secured.get("/api/auth/me").text
    assert "example.com" not in login(secured).text


def test_a_session_survives_the_next_request(secured):
    login(secured)
    assert secured.get("/api/decks").status_code == 200


def test_logout_ends_the_session(secured):
    login(secured)
    token = secured.cookies.get(COOKIE_NAME)
    assert secured.post("/api/auth/logout").status_code == 200
    assert secured.get("/api/decks").status_code == 401

    with db.connection() as con:
        assert sessions.lookup(con, token) is None


def test_logout_without_a_session_is_not_an_error(secured):
    """The honest answer to logging out of nothing is that you are logged out."""
    assert secured.post("/api/auth/logout").status_code == 200


# ----------------------------------------------------------- the refusals

def test_a_wrong_password_is_401_with_a_generic_message(secured):
    response = login(secured, password="not-the-right-passphrase")
    assert response.status_code == 401
    assert response.json()["detail"] == "invalid username or password"


def test_an_unknown_user_gets_the_identical_message(secured):
    """Byte-identical, not merely similar. Any difference is the answer to
    "does this account exist"."""
    unknown = login(secured, username="nobody")
    wrong = login(secured, password="not-the-right-passphrase")
    assert unknown.status_code == wrong.status_code
    assert unknown.json() == wrong.json()


def test_a_disabled_account_gets_the_identical_message_too(secured):
    with db.connection() as con:
        ada = users.get(con, "ada")
        users.set_disabled(con, ada.id, True)
    disabled = login(secured)
    unknown = login(secured, username="nobody")
    assert disabled.status_code == 401
    assert disabled.json() == unknown.json()


def test_a_missing_field_is_422_not_401(secured):
    """A malformed request is a different thing from a refused one, and
    collapsing them makes the client's error message wrong."""
    assert secured.post("/api/auth/login", json={"username": "ada"})\
        .status_code == 422


@pytest.mark.parametrize("dodge", ["/api/decks/", "/api//decks",
                                   "///api/decks"])
def test_a_respelt_path_is_still_refused(secured, dodge):
    """A protected handler must not be reachable by spelling its path oddly."""
    assert secured.get(dodge).status_code != 200, f"{dodge} was served"


def test_the_allowlist_is_at_least_as_strict_as_the_router():
    """The rule the middleware has to satisfy, checked on the function.

    Not over HTTP, because httpx resolves `.`, `..` and a leading `//` before
    the request leaves the client -- `//api/decks` becomes a request to the
    host `api` for `/decks`, which never reaches the app at all. A raw client
    is under no such obligation, so the check belongs on the function that
    would see it.

    `//api/decks` is the case that motivated normalising: a plain
    `startswith("/api")` reads it as being outside the API, and so public.
    """
    from mtglab.api.auth import is_public

    for public in ("/api/health", "/api/health/", "/assets/index.js", "/"):
        assert is_public(public), f"{public} should be public"
    for protected in ("/api/decks", "//api/decks", "/api//decks",
                      "/api/decks/", "/api/health/../decks",
                      "/api/auth/../decks", "/api/jobs/abc123"):
        assert not is_public(protected), f"{protected} should need a session"


def test_a_forged_cookie_is_simply_not_a_session(secured):
    secured.cookies.set(COOKIE_NAME, "made-up-token")
    assert secured.get("/api/decks").status_code == 401


def test_a_session_stops_working_when_the_account_is_disabled(secured):
    """Disabling revokes sessions, so the cookie in hand dies with the account
    rather than outliving it."""
    login(secured)
    with db.connection() as con:
        users.set_disabled(con, users.get(con, "ada").id, True)
    assert secured.get("/api/decks").status_code == 401


def test_a_session_stops_working_when_the_password_changes(secured):
    login(secured)
    with db.connection() as con:
        users.set_password(con, users.get(con, "ada").id, "a-brand-new-phrase")
    assert secured.get("/api/decks").status_code == 401


def test_login_regenerates_the_session_id(secured):
    """Fixation: the token that arrives is destroyed, and the one that leaves
    is new. A token planted before the login is not valid after it."""
    login(secured)
    first = secured.cookies.get(COOKIE_NAME)
    login(secured)
    second = secured.cookies.get(COOKIE_NAME)

    assert first != second
    with db.connection() as con:
        assert sessions.lookup(con, first) is None
        assert sessions.lookup(con, second) is not None


# ------------------------------------------------------------ the rate limit

def test_the_account_budget_runs_out(secured):
    for _ in range(ratelimit.PER_ACCOUNT.failures):
        assert login(secured, password="wrong-but-long-enough").status_code == 401
    throttled = login(secured, password="wrong-but-long-enough")
    assert throttled.status_code == 429
    assert int(throttled.headers["Retry-After"]) > 0


def test_the_throttle_applies_to_the_right_password_too(secured):
    """Otherwise it is not a throttle -- it is a hint that you were close."""
    for _ in range(ratelimit.PER_ACCOUNT.failures):
        login(secured, password="wrong-but-long-enough")
    assert login(secured).status_code == 429


def test_a_success_restores_the_budget(secured):
    for _ in range(ratelimit.PER_ACCOUNT.failures - 1):
        login(secured, password="wrong-but-long-enough")
    assert login(secured).status_code == 200
    assert login(secured, password="wrong-but-long-enough").status_code == 401


def test_the_throttle_message_says_nothing_about_the_account(secured):
    for _ in range(ratelimit.PER_ACCOUNT.failures):
        login(secured, password="wrong-but-long-enough")
    assert "ada" not in login(secured).text


def test_a_failed_login_is_logged_with_the_address(secured, caplog):
    """The maintainer needs to see an attack. `docs/HOSTING.md` §1 asks for the
    time and the address, and for no secrets."""
    with caplog.at_level("WARNING", logger="mtglab.auth"):
        login(secured, password="wrong-but-long-enough")
    logged = caplog.text
    assert "failed login" in logged and "ada" in logged
    assert "wrong-but-long-enough" not in logged


def test_an_email_typed_into_the_username_box_is_not_logged(secured, caplog):
    """ADR 16 is unconditional: an address must never reach a log line, and the
    login form is the one place a user will type theirs into a field that was
    not asking for it."""
    with caplog.at_level("WARNING", logger="mtglab.auth"):
        login(secured, username="ada@example.com",
              password="wrong-but-long-enough")
    assert "ada@example.com" not in caplog.text
    assert "<redacted>@example.com" in caplog.text


# -------------------------------------------------- auth off: the default

def test_the_local_app_has_no_login(local):
    """The way the app has always run, and the way it runs for anyone who
    clones it. A regression here is the one this whole file is guarding."""
    assert local.get("/api/decks").status_code == 200
    assert local.get("/api/health").status_code == 200


def test_me_says_no_login_is_required(local):
    """Three separate fields: a frontend must tell "logged out" from "there is
    nothing to log in to", or the local app renders a sign-in form.

    `is_admin` is true here while `user` is null, and that pair is the whole
    reason it sits at the top level (ADR 17). Auth off means the caller is
    `LOCAL` — an admin, because there is nobody else for it to be true relative
    to, and authenticated as nobody. A client reading `user?.is_admin` would
    hide the admin page on the only configuration the app has ever run in.
    """
    assert local.get("/api/auth/me").json() == {
        "auth_required": False, "authenticated": False, "is_admin": True,
        "user": None}


def test_the_local_app_touches_no_database(local, tmp_path):
    """Auth off means the middleware returns before opening anything. A local
    run should not leave an `app.db` behind for a feature it does not use."""
    local.get("/api/decks")
    local.get("/api/auth/me")
    assert not (tmp_path / "data" / "app.db").exists()


def test_the_local_caller_owns_their_jobs(local):
    """`owner=None` is a real owner, not a hole: with auth off there is one
    person and every job is theirs."""
    job = jobs.submit("test", lambda progress: 1, label="local")
    assert local.get(f"/api/jobs/{job.id}").status_code == 200
    assert len(local.get("/api/jobs").json()) == 1


# ------------------------------------------------------------ configuration

def test_require_auth_defaults_to_the_environment(monkeypatch, tmp_path):
    monkeypatch.setenv("MTGLAB_REQUIRE_AUTH", "1")
    with config.use_paths(data_dir=tmp_path / "data"), \
            TestClient(create_app()) as client:
        assert client.get("/api/decks").status_code == 401


@pytest.mark.parametrize("value,expected",
                         [("1", True), ("true", True), ("YES", True),
                          ("on", True), ("0", False), ("", False),
                          ("no", False), ("maybe", False)])
def test_the_flag_reads_the_obvious_spellings(monkeypatch, value, expected):
    monkeypatch.setenv("MTGLAB_REQUIRE_AUTH", value)
    assert config.require_auth() is expected


def test_secure_cookies_follow_require_auth_unless_told_otherwise(monkeypatch):
    """They travel together: auth on means deployed means TLS. A `Secure`
    cookie on `http://127.0.0.1` is never sent back, which looks like a login
    that succeeds and then does not work."""
    monkeypatch.setenv("MTGLAB_REQUIRE_AUTH", "1")
    monkeypatch.delenv("MTGLAB_SECURE_COOKIES", raising=False)
    assert config.secure_cookies() is True

    monkeypatch.setenv("MTGLAB_REQUIRE_AUTH", "0")
    assert config.secure_cookies() is False

    monkeypatch.setenv("MTGLAB_SECURE_COOKIES", "1")
    assert config.secure_cookies() is True


def test_no_proxy_header_is_trusted_by_default(monkeypatch, secured):
    """A rate limit keyed on a header any client can send is a rate limit an
    attacker opts out of by typing a different number."""
    monkeypatch.delenv("MTGLAB_CLIENT_IP_HEADER", raising=False)
    assert config.client_ip_header() is None

    for i in range(ratelimit.PER_ADDRESS.failures + 1):
        secured.post("/api/auth/login",
                     json={"username": f"nobody{i}", "password": "long-enough-x"},
                     headers={"X-Forwarded-For": f"10.0.0.{i}"})
    # Every one of those came from the same real address, so the per-address
    # budget is spent regardless of what the header claimed.
    assert login(secured).status_code == 429


def test_a_named_proxy_header_is_read(monkeypatch, secured):
    monkeypatch.setenv("MTGLAB_CLIENT_IP_HEADER", "Fly-Client-IP")
    assert config.client_ip_header() == "Fly-Client-IP"
    # Distinct claimed addresses now get distinct budgets, which is the point
    # of trusting a header a proxy you control rewrites.
    for i in range(ratelimit.PER_ADDRESS.failures + 1):
        secured.post("/api/auth/login",
                     json={"username": "nobody", "password": "long-enough-x"},
                     headers={"Fly-Client-IP": f"10.0.0.{i}"})
    with db.connection() as con:
        assert not ratelimit.exhausted(con, ratelimit.address_key("10.0.0.1"),
                                       ratelimit.PER_ADDRESS)


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
