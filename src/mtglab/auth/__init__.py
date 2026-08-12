"""Accounts, passwords and sessions — the auth core.

[ADR 5](../../../docs/adr/0005-sessions-over-jwts-and-no-self-signup.md) decided
the shape and
[ADR 16](../../../docs/adr/0016-accounts-are-invited-and-passwords-are-self-served.md)
changed one paragraph of it. What is here is `docs/HOSTING.md` §6 steps 4 and 5:
`app.db`, the users table, Argon2id, sessions, and the rate limiter guarding the
one endpoint that accepts a password. **The email half — invite links and
password resets — is a separate build**, and nothing here reaches a network or
sends a message.

Five modules and one job each:

- `db.py`         the SQLite file, its schema, and its one migration path
- `passwords.py`  Argon2id at the OWASP minimum profile, and nothing else
- `users.py`      the account record and the only path that verifies one
- `sessions.py`   opaque tokens, stored hashed, revocable by `DELETE`
- `ratelimit.py`  a fixed window, in the same database, for login attempts

Nothing here imports FastAPI and nothing here knows what a cookie is. HTTP
lives in `api/auth.py`, and the separation is what lets `mtglab users add`
create an account on a box with no web server running -- and what lets the
tests exercise every rule in this package without a client.

Submodules are imported by name (`from mtglab.auth import users`) rather than
re-exported here. `users.create`, `sessions.create` and `db.connect` are three
different verbs that would collide in one namespace, and the module prefix is
the thing that makes a call site say which database row it is about.
"""
