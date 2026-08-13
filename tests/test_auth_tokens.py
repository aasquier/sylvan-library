"""Invite and reset tokens, and the `EmailSender` seam they travel through.

The rules being pinned are ADR 16's, and they are the ones that make a link in
somebody's inbox safe to be a credential: stored hashed, single use,
short-lived, and redeeming one ends every session for that account. Each has a
test that fails if the property is removed, rather than a comment saying it
holds.

**No test here sends mail.** The seam is the whole point — `ConsoleSender`
writes to a stream a test owns, and `ResendSender` is exercised through an
injected transport, so the request it builds is checked without a network and
without an account. A test that reached `api.resend.com` would be a test that
fails on an aeroplane and charges money on a runner.
"""

import json
import sys
from datetime import UTC, datetime, timedelta
from io import StringIO
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

pytest.importorskip("argon2")

from mtglab import config  # noqa: E402
from mtglab.auth import db, invites, mail, sessions, tokens, users  # noqa: E402

PASSWORD = "a-perfectly-fine-passphrase"


@pytest.fixture
def con(tmp_path):
    with config.use_paths(data_dir=tmp_path / "data"):
        connection = db.connect()
        try:
            yield connection
        finally:
            connection.close()


@pytest.fixture
def ada(con):
    return users.create(con, "ada", email="ada@example.com")


class Recorder:
    """An `EmailSender` that keeps what it was given. The test double."""

    def __init__(self) -> None:
        self.sent: list[mail.Message] = []

    def send(self, message: mail.Message) -> None:
        self.sent.append(message)


# --------------------------------------------------------------- the schema

def test_a_fresh_database_is_at_the_current_version(con):
    """Pinned to the constant rather than to a literal.

    It was `== 3` and had to be edited when the dossier cache added a fourth
    migration, which is churn that proves nothing: what matters is that a
    freshly created file has run every migration in the ladder, not what the
    ladder's length happens to be this week. The length itself is asserted
    once, below, where adding a migration is the thing being checked.
    """
    version = con.execute("PRAGMA user_version").fetchone()[0]
    assert version == db.SCHEMA_VERSION
    assert len(db._MIGRATIONS) == db.SCHEMA_VERSION, (
        "SCHEMA_VERSION and the migration ladder have drifted apart; a "
        "migration added without bumping the version never runs on an "
        "existing app.db")


def test_a_version_one_database_migrates_in_place(tmp_path):
    """The migration path is the reason `_MIGRATIONS` is a tuple of versions
    rather than one string somebody edits. There was already an `app.db` on a
    laptop with an account in it when this landed."""
    from mtglab.auth import db as auth_db

    path = tmp_path / "old.db"
    old = auth_db.connect(path)
    users.create(old, "ada", password=PASSWORD)
    # Wind it back to what version 1 left behind.
    old.executescript("DROP TABLE auth_tokens; DROP TABLE sim_cache; "
                      "DROP TABLE dossier_cache; PRAGMA user_version = 1;")
    old.commit()
    old.close()

    migrated = auth_db.connect(path)
    try:
        assert migrated.execute(
            "PRAGMA user_version").fetchone()[0] == auth_db.SCHEMA_VERSION
        assert users.get(migrated, "ada") is not None, "the account survived"
        assert tokens.issue(migrated, users.get(migrated, "ada").id,
                            tokens.Purpose.INVITE)
        assert migrated.execute("SELECT count(*) FROM sim_cache").fetchone()[0] == 0
    finally:
        migrated.close()


def test_a_version_two_database_gains_the_sim_cache(tmp_path):
    """The upgrade that actually happens on the maintainer's laptop.

    Every `app.db` in existence when the simulation cache landed was at version
    two, holding real accounts. The tail of the ladder has to run against one
    without touching what is already there -- which is the case a fresh-database
    test cannot see, because a fresh database runs every migration in order and
    would pass even if the tail assumed an empty file.
    """
    from mtglab.auth import db as auth_db

    path = tmp_path / "v2.db"
    old = auth_db.connect(path)
    users.create(old, "ada", password=PASSWORD)
    old.executescript("DROP TABLE sim_cache; DROP TABLE dossier_cache; "
                      "PRAGMA user_version = 2;")
    old.commit()
    old.close()

    migrated = auth_db.connect(path)
    try:
        assert migrated.execute(
            "PRAGMA user_version").fetchone()[0] == auth_db.SCHEMA_VERSION
        assert users.get(migrated, "ada") is not None, "the account survived"
        assert migrated.execute("SELECT count(*) FROM sim_cache").fetchone()[0] == 0
    finally:
        migrated.close()


def test_a_version_three_database_gains_the_dossier_cache(tmp_path):
    """The upgrade that happens on the maintainer's laptop this time.

    Same shape as the one above and worth having as its own test for the same
    reason: every `app.db` in existence when ADR 19 landed is at version three
    with real accounts and real cached simulations in it, and the new
    migration has to run against that without disturbing either.
    """
    from mtglab.auth import db as auth_db

    path = tmp_path / "v3.db"
    old = auth_db.connect(path)
    users.create(old, "ada", password=PASSWORD)
    old.execute("INSERT INTO sim_cache (key, kind, result_json, created_at, "
                "last_used_at) VALUES ('k', 'sim.mana', '{}', 'then', 'then')")
    old.executescript("DROP TABLE dossier_cache; PRAGMA user_version = 3;")
    old.commit()
    old.close()

    migrated = auth_db.connect(path)
    try:
        assert migrated.execute(
            "PRAGMA user_version").fetchone()[0] == auth_db.SCHEMA_VERSION
        assert users.get(migrated, "ada") is not None, "the account survived"
        assert migrated.execute(
            "SELECT count(*) FROM sim_cache").fetchone()[0] == 1, \
            "cached simulations survived the upgrade"
        assert migrated.execute(
            "SELECT count(*) FROM dossier_cache").fetchone()[0] == 0
    finally:
        migrated.close()


def test_deleting_a_user_cascades_to_their_tokens(con, ada):
    tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    with con:
        con.execute("DELETE FROM users WHERE id = ?", (ada.id,))
    assert con.execute("SELECT count(*) AS n FROM auth_tokens")\
        .fetchone()["n"] == 0


# ---------------------------------------------------------------- the token

def test_a_token_round_trips(con, ada):
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    resolved = tokens.lookup(con, token)
    assert resolved.user_id == ada.id
    assert resolved.purpose is tokens.Purpose.INVITE


def test_the_token_itself_is_never_stored(con, ada):
    """Same rule as a session token, and for the same reason: reading `app.db`
    must not hand over a live credential."""
    token = tokens.issue(con, ada.id, tokens.Purpose.RESET)
    stored = con.execute("SELECT token_hash FROM auth_tokens").fetchone()
    assert token not in stored["token_hash"]
    assert len(stored["token_hash"]) == 64, "sha256, hex"


def test_every_token_is_distinct(con, ada):
    minted = {tokens.issue(con, ada.id, tokens.Purpose.RESET)
              for _ in range(20)}
    assert len(minted) == 20


@pytest.mark.parametrize("bad", ["", "not-a-token", "x" * 43])
def test_an_unknown_token_is_invalid(con, bad):
    with pytest.raises(tokens.InvalidToken):
        tokens.lookup(con, bad)


def test_a_reset_lasts_an_hour_and_an_invite_lasts_longer(con, ada):
    """ADR 16 names the hour. The invite is longer because it grants nothing
    until it is used, and expiring it strands somebody who was on holiday."""
    assert tokens.LIFETIMES[tokens.Purpose.RESET] == timedelta(hours=1)
    assert tokens.LIFETIMES[tokens.Purpose.INVITE] > timedelta(hours=1)

    token = tokens.issue(con, ada.id, tokens.Purpose.RESET)
    resolved = tokens.lookup(con, token)
    life = resolved.expires_at - resolved.created_at
    assert timedelta(minutes=59) <= life <= timedelta(minutes=61)


def test_an_expired_token_is_refused(con, ada):
    token = tokens.issue(con, ada.id, tokens.Purpose.RESET,
                         lifetime=timedelta(seconds=-1))
    with pytest.raises(tokens.ExpiredToken):
        tokens.lookup(con, token)


def test_a_token_is_not_redeemable_as_the_other_purpose(con, ada):
    """An invite lives a week. Accepting one where a reset is expected would
    quietly hand a reset that week."""
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    with pytest.raises(tokens.InvalidToken):
        tokens.lookup(con, token, tokens.Purpose.RESET)
    assert tokens.lookup(con, token, tokens.Purpose.INVITE)


def test_the_wrong_purpose_is_indistinguishable_from_nonsense(con, ada):
    """Same message, so poking the other endpoint with a real token teaches
    nothing about what the token is for."""
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    with pytest.raises(tokens.InvalidToken) as mismatched:
        tokens.lookup(con, token, tokens.Purpose.RESET)
    with pytest.raises(tokens.InvalidToken) as unknown:
        tokens.lookup(con, "made-up", tokens.Purpose.RESET)
    assert str(mismatched.value) == str(unknown.value)


def test_issuing_supersedes_the_previous_link_of_that_purpose(con, ada):
    """"It never arrived, send another" has to make the first one dead --
    otherwise a message that went astray stays live for its full hour."""
    first = tokens.issue(con, ada.id, tokens.Purpose.RESET)
    second = tokens.issue(con, ada.id, tokens.Purpose.RESET)
    with pytest.raises(tokens.InvalidToken):
        tokens.lookup(con, first)
    assert tokens.lookup(con, second)


def test_issuing_leaves_the_other_purpose_alone(con, ada):
    invite = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    tokens.issue(con, ada.id, tokens.Purpose.RESET)
    assert tokens.lookup(con, invite, tokens.Purpose.INVITE)


def test_one_accounts_token_does_not_touch_another(con, ada):
    grace = users.create(con, "grace", email="grace@example.com")
    hers = tokens.issue(con, grace.id, tokens.Purpose.INVITE)
    tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    assert tokens.lookup(con, hers).user_id == grace.id


# --------------------------------------------------------------- redemption

def test_redeeming_sets_the_password(con, ada):
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    assert users.authenticate(con, "ada", PASSWORD) is None

    redeemed = tokens.redeem(con, token, PASSWORD)
    assert redeemed.id == ada.id
    assert users.authenticate(con, "ada", PASSWORD) is not None


def test_a_token_works_exactly_once(con, ada):
    """The rule ADR 16 states plainly. A link forwarded to somebody else after
    it has been used must do nothing."""
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    tokens.redeem(con, token, PASSWORD)
    with pytest.raises(tokens.UsedToken):
        tokens.redeem(con, token, "another-perfectly-fine-phrase")
    assert users.authenticate(con, "ada", PASSWORD) is not None, \
        "the second attempt must not have changed anything"


def test_a_used_token_says_so_rather_than_saying_nothing(con, ada):
    """Whoever is holding it already knows it was real, so "already used" is
    an answer that helps and leaks nothing."""
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    tokens.redeem(con, token, PASSWORD)
    with pytest.raises(tokens.UsedToken, match="already been used"):
        tokens.lookup(con, token)


def test_redeeming_ends_every_session_for_that_account(con, ada):
    """ADR 16: a reset is usually somebody who suspects compromise, and one
    that leaves the attacker logged in has answered the wrong question."""
    users.set_password(con, ada.id, PASSWORD)
    first = sessions.create(con, ada.id)
    second = sessions.create(con, ada.id)

    token = tokens.issue(con, ada.id, tokens.Purpose.RESET)
    tokens.redeem(con, token, "a-brand-new-passphrase")

    assert sessions.lookup(con, first) is None
    assert sessions.lookup(con, second) is None


def test_redeeming_leaves_another_accounts_sessions_alone(con, ada):
    grace = users.create(con, "grace", password=PASSWORD)
    hers = sessions.create(con, grace.id)
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    tokens.redeem(con, token, PASSWORD)
    assert sessions.lookup(con, hers) is not None


def test_a_weak_password_is_refused_without_spending_the_token(con, ada):
    """The sensible next move is a longer password, not a fresh link."""
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    from mtglab.auth import passwords

    with pytest.raises(passwords.WeakPassword):
        tokens.redeem(con, token, "short")
    assert tokens.lookup(con, token), "the link must still work"
    tokens.redeem(con, token, PASSWORD)


def test_an_expired_token_sets_no_password(con, ada):
    token = tokens.issue(con, ada.id, tokens.Purpose.RESET,
                         lifetime=timedelta(seconds=-1))
    with pytest.raises(tokens.ExpiredToken):
        tokens.redeem(con, token, PASSWORD)
    assert users.authenticate(con, "ada", PASSWORD) is None


def test_a_disabled_account_cannot_be_reset_back_into_service(con, ada):
    """Disabling is the maintainer's revocation lever. One the disabled party
    can undo from their own inbox is not a lever."""
    token = tokens.issue(con, ada.id, tokens.Purpose.RESET)
    users.set_disabled(con, ada.id, True)

    with pytest.raises(tokens.InvalidToken):
        tokens.redeem(con, token, PASSWORD)
    assert users.get_by_id(con, ada.id).disabled
    assert users.authenticate(con, "ada", PASSWORD) is None


# ------------------------------------------------------------ housekeeping

def test_outstanding_reports_a_live_invite(con, ada):
    assert not tokens.outstanding(con, ada.id, tokens.Purpose.INVITE)
    token = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    assert tokens.outstanding(con, ada.id, tokens.Purpose.INVITE)
    assert not tokens.outstanding(con, ada.id, tokens.Purpose.RESET)
    tokens.redeem(con, token, PASSWORD)
    assert not tokens.outstanding(con, ada.id, tokens.Purpose.INVITE)


def test_purge_drops_expired_links_and_keeps_recent_used_ones(con, ada):
    tokens.issue(con, ada.id, tokens.Purpose.RESET,
                 lifetime=timedelta(seconds=-1))
    used = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    tokens.redeem(con, used, PASSWORD)

    assert tokens.purge_expired(con) == 1
    with pytest.raises(tokens.UsedToken):
        tokens.lookup(con, used)


def test_purge_eventually_forgets_a_used_link(con, ada):
    used = tokens.issue(con, ada.id, tokens.Purpose.INVITE)
    tokens.redeem(con, used, PASSWORD)
    assert tokens.purge_expired(con, keep_used_for=timedelta(seconds=-1)) == 1
    with pytest.raises(tokens.InvalidToken):
        tokens.lookup(con, used)


# ------------------------------------------------------------- the messages

def test_an_invite_carries_a_link_with_the_token_in_the_fragment(con, ada):
    """The `#` is load-bearing: a fragment is never sent to the server, so the
    token stays out of access logs and `Referer` headers."""
    recorder = Recorder()
    invites.send_invite(con, ada, sender=recorder,
                        base_url="https://mtglab.example.com")

    (message,) = recorder.sent
    assert message.to == "ada@example.com"
    link = [w for w in message.body.split() if w.startswith("https://")][0]
    base, _, token = link.partition("#token=")
    assert base == "https://mtglab.example.com/auth/claim"
    assert "?" not in link, "a token in a query string lands in an access log"
    assert tokens.lookup(con, token, tokens.Purpose.INVITE).user_id == ada.id


def test_a_reset_resolves_the_address_and_sends(con, ada):
    users.set_password(con, ada.id, PASSWORD)
    recorder = Recorder()
    invites.send_reset(con, "Ada@Example.com", sender=recorder)

    (message,) = recorder.sent
    assert message.to == "ada@example.com", "normalised, not as typed"
    link = [w for w in message.body.split() if w.startswith("http")][0]
    token = link.partition("#token=")[2]
    assert tokens.lookup(con, token, tokens.Purpose.RESET).user_id == ada.id


@pytest.mark.parametrize("address", ["nobody@example.com", "not-an-address",
                                     ""])
def test_a_reset_for_an_address_with_no_account_sends_nothing_and_says_nothing(
        con, address):
    """The signature is the enforcement: a caller that gets `None` either way
    cannot branch on whether the lookup hit."""
    recorder = Recorder()
    assert invites.send_reset(con, address, sender=recorder) is None
    assert recorder.sent == []


def test_a_disabled_account_gets_no_reset_link(con, ada):
    users.set_disabled(con, ada.id, True)
    recorder = Recorder()
    invites.send_reset(con, "ada@example.com", sender=recorder)
    assert recorder.sent == []


def test_an_unclaimed_account_may_still_reset(con, ada):
    """Somebody whose invite expired asking for a reset is the same request in
    different words, and there is nothing else for them to do."""
    recorder = Recorder()
    invites.send_reset(con, "ada@example.com", sender=recorder)
    assert len(recorder.sent) == 1


def test_an_invite_without_an_address_is_a_programming_error(con):
    nameless = users.create(con, "grace")
    with pytest.raises(ValueError, match="address"):
        invites.send_invite(con, nameless, sender=Recorder())


def test_the_link_uses_the_configured_base_url(monkeypatch, con, ada):
    monkeypatch.setenv("MTGLAB_BASE_URL", "https://decks.example.com/")
    recorder = Recorder()
    invites.send_invite(con, ada, sender=recorder)
    assert "https://decks.example.com/auth/claim#token=" in recorder.sent[0].body


def test_the_default_base_url_is_the_local_app(monkeypatch):
    """Wrong-by-default rather than absent: a deployment that forgets this
    sends links to localhost, which is visibly broken rather than pointing
    somewhere an attacker chose."""
    monkeypatch.delenv("MTGLAB_BASE_URL", raising=False)
    assert config.base_url() == "http://127.0.0.1:8765"
    assert invites.claim_link("abc").startswith("http://127.0.0.1:8765/")


# ----------------------------------------------------------- the mail seam

def test_the_console_sender_prints_the_message_and_sends_nothing():
    stream = StringIO()
    mail.ConsoleSender(stream).send(
        mail.Message(to="ada@example.com", subject="Hello",
                     body="a link\nand a second line"))
    printed = stream.getvalue()
    assert "ada@example.com" in printed
    assert "Hello" in printed
    assert "and a second line" in printed
    assert "NOT sent" in printed, "it must not look like delivery happened"


def test_the_console_sender_satisfies_the_protocol():
    assert isinstance(mail.ConsoleSender(), mail.EmailSender)
    assert isinstance(mail.ResendSender("re_key", "a@b.c"), mail.EmailSender)


def test_the_resend_sender_builds_the_request_the_api_documents():
    seen: dict[str, object] = {}

    def transport(url, headers, body):
        seen.update(url=url, headers=headers, body=json.loads(body))
        return 200, b'{"id": "abc"}'

    mail.ResendSender("re_secret", "mtglab <no-reply@example.com>",
                      transport=transport).send(
        mail.Message(to="ada@example.com", subject="Hi", body="a link"))

    assert seen["url"] == mail.RESEND_ENDPOINT
    assert seen["headers"]["Authorization"] == "Bearer re_secret"
    assert seen["body"] == {"from": "mtglab <no-reply@example.com>",
                            "to": ["ada@example.com"],
                            "subject": "Hi", "text": "a link"}


def test_a_refusal_becomes_an_error_without_the_recipient_in_it():
    """Resend's error bodies quote the address back. ADR 16 is unconditional
    that one must never reach a log line, and this exception is logged."""
    def transport(url, headers, body):
        return 422, json.dumps({
            "statusCode": 422, "name": "validation_error",
            "message": "ada@example.com is not a valid recipient"}).encode()

    with pytest.raises(mail.EmailNotSent) as caught:
        mail.ResendSender("re_secret", "a@b.c", transport=transport).send(
            mail.Message(to="ada@example.com", subject="Hi", body="x"))

    assert "ada@example.com" not in str(caught.value)
    assert "422" in str(caught.value)
    assert "validation_error" in str(caught.value), "enough to search their logs"


def test_an_unreachable_provider_is_an_error_too():
    def transport(url, headers, body):
        raise TimeoutError("timed out")

    with pytest.raises(mail.EmailNotSent, match="could not reach"):
        mail.ResendSender("re_secret", "a@b.c", transport=transport).send(
            mail.Message(to="ada@example.com", subject="Hi", body="x"))


def test_a_junk_error_body_still_produces_a_usable_message():
    def transport(url, headers, body):
        return 500, b"<html>gateway</html>"

    with pytest.raises(mail.EmailNotSent, match="500"):
        mail.ResendSender("re_secret", "a@b.c", transport=transport).send(
            mail.Message(to="ada@example.com", subject="Hi", body="x"))


def test_a_body_that_is_not_the_providers_json_says_so():
    """The 403 that broke the first real invite was a Cloudflare block page,
    not a Resend refusal, and a bare status sent the diagnosis at the API key
    and the domain instead. The two failures need different fixes, so the
    message has to tell them apart."""
    def transport(url, headers, body):
        return 403, b"error code: 1010"

    with pytest.raises(mail.EmailNotSent) as caught:
        mail.ResendSender("re_secret", "a@b.c", transport=transport).send(
            mail.Message(to="ada@example.com", subject="Hi", body="x"))

    assert "403" in str(caught.value)
    assert "not the provider's JSON" in str(caught.value)
    assert "1010" not in str(caught.value), "the body itself is never quoted"


def test_the_resend_sender_identifies_itself():
    """Cloudflare sits in front of `api.resend.com` and answers the default
    `Python-urllib/x.y` with 403 error 1010, so an unset User-Agent means no
    mail is ever delivered. Measured against the live API 2026-08-13; this is
    the one property of the request that a working invite depends on and that
    nothing else in the suite would notice."""
    seen: dict[str, object] = {}

    def transport(url, headers, body):
        seen.update(headers)
        return 200, b'{"id": "abc"}'

    mail.ResendSender("re_secret", "a@b.c", transport=transport).send(
        mail.Message(to="ada@example.com", subject="Hi", body="x"))

    agent = str(seen.get("User-Agent", ""))
    assert agent, "an absent User-Agent is a 403 from the WAF, not from Resend"
    assert "urllib" not in agent, "the default is precisely what is blocked"


def test_an_empty_key_is_refused_at_construction():
    with pytest.raises(mail.EmailNotConfigured):
        mail.ResendSender("   ", "a@b.c")


def test_a_key_selects_the_real_sender(monkeypatch):
    monkeypatch.setenv("RESEND_API_KEY", "re_secret")
    assert isinstance(mail.sender_from_env(), mail.ResendSender)


def test_no_key_and_no_auth_is_the_console(monkeypatch):
    monkeypatch.delenv("RESEND_API_KEY", raising=False)
    monkeypatch.setenv("MTGLAB_REQUIRE_AUTH", "0")
    assert isinstance(mail.sender_from_env(), mail.ConsoleSender)


def test_no_key_with_auth_on_is_refused(monkeypatch):
    """A deployment is the one place the console fallback is wrong: "the
    console" there is the platform's log aggregator, and the messages contain
    email addresses."""
    monkeypatch.delenv("RESEND_API_KEY", raising=False)
    monkeypatch.setenv("MTGLAB_REQUIRE_AUTH", "1")
    with pytest.raises(mail.EmailNotConfigured, match="RESEND_API_KEY"):
        mail.sender_from_env()


def test_nothing_in_the_seam_reaches_the_network(monkeypatch):
    """The rule, asserted rather than trusted: with `urlopen` booby-trapped,
    every sender in this file still works."""
    def explode(*args, **kwargs):                          # pragma: no cover
        raise AssertionError("a test tried to send mail")

    monkeypatch.setattr("urllib.request.urlopen", explode)
    mail.ConsoleSender(StringIO()).send(
        mail.Message(to="a@b.c", subject="s", body="b"))
    mail.ResendSender("re_x", "a@b.c",
                      transport=lambda u, h, b: (200, b"{}")).send(
        mail.Message(to="a@b.c", subject="s", body="b"))


def test_the_stamps_are_timezone_aware(con, ada):
    """`DTZ` is on in ruff for a reason; a naive stamp compared against an
    aware one raises at the moment somebody's link is being checked."""
    token = tokens.issue(con, ada.id, tokens.Purpose.RESET)
    resolved = tokens.lookup(con, token)
    assert resolved.expires_at.tzinfo is not None
    assert resolved.expires_at > datetime.now(UTC)


if __name__ == "__main__":                                    # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-v"]))
