"""Accounts, passwords and sessions — the auth core.

[ADR 5](../../../docs/adr/0005-sessions-over-jwts-and-no-self-signup.md) decided
the shape and
[ADR 16](../../../docs/adr/0016-accounts-are-invited-and-passwords-are-self-served.md)
changed one paragraph of it. What is here is `docs/HOSTING.md` §6 steps 4, 5 and
5b: `app.db`, the users table, Argon2id, sessions, the rate limiter guarding the
endpoints that accept a password, and the invite and reset machinery.

Eight modules and one job each:

- `db.py`         the SQLite file, its schema, and its migration path
- `passwords.py`  Argon2id at the OWASP minimum profile, and nothing else
- `users.py`      the account record and the only path that verifies one
- `sessions.py`   opaque tokens, stored hashed, revocable by `DELETE`
- `ratelimit.py`  a fixed window, in the same database, for attempts
- `tokens.py`     single-use invite and reset links, also stored hashed
- `mail.py`       the `EmailSender` seam, a console sender and a Resend one
- `invites.py`    issue a link and deliver it — the one path both doors take

`mail.py` is the only module here that touches a network, and it is behind a
protocol precisely so that nothing above it can tell. **No test in this project
sends mail**, the same rule that keeps the Claude tests off the network.

Nothing here imports FastAPI and nothing here knows what a cookie is. HTTP
lives in `api/auth.py`, and the separation is what lets `mtglab users invite`
create an account and mail its link from a box with no web server running --
and what lets the tests exercise every rule in this package without a client.

Submodules are imported by name (`from mtglab.auth import users`) rather than
re-exported here. `users.create`, `sessions.create` and `db.connect` are three
different verbs that would collide in one namespace, and the module prefix is
the thing that makes a call site say which database row it is about.
"""
