# 5. Sessions over JWTs, and no self-signup

**Status:** Proposed — no auth exists yet · **Recorded:** 2026-08-10

## Context

The app has no authentication today, which is fine while it serves public card
data and the maintainer's own decks. It stops being fine the moment there are
other people's decks, and it stops being fine *hard* if collection data ever
ships — CLAUDE.md rule 5 exists because a public inventory of expensive cards
tied to a real identity is a targeting list, and that reasoning does not stop at
`git`.

Expected scale: fewer than a dozen known people, all personally invited.

## Options considered

**JWTs.** Rejected. They cannot be revoked without a server-side denylist, which
is the very state they exist to avoid, and this app has no stateless-scaling
problem to solve — it is one machine. A session row you can `DELETE` is simpler
and strictly more controllable.

**Self-signup with email verification.** Rejected. It requires a signup flow, an
SMTP provider, verification tokens, password-reset tokens, bot defences and
probably a CAPTCHA — a large amount of infrastructure and recurring cost, to
solve a problem (strangers wanting accounts) that does not exist here.

**Cloudflare Access instead of writing auth at all.** Free for up to 50 users,
puts an identity gate in front of the whole app, passes a verified email in a
header. **It is genuinely less work and less risk than anything below**: no
passwords stored, no login code, no credential-breach surface. The costs are a
Cloudflare dependency and a login flow the maintainer does not control. Recorded
as the honest alternative, and as the recommended first step if the goal is
simply "friends can log in".

## Decision

If auth is built in-app:

- **Argon2id** via `argon2-cffi`, at the **OWASP minimum profile —
  `m=19MiB, t=2, p=1`**. Argon2 is deliberately memory-hard, and the common
  `m=64MB` setting means each concurrent login allocates 64 MB; on a 1 GB
  instance, a few simultaneous logins plus DuckDB plus a running sim is how you
  get OOM-killed. 19 MiB is still far beyond brute-force economics for a private
  site.
- **Opaque server-side sessions.** `secrets.token_urlsafe(32)`, stored hashed in
  SQLite, sent as `HttpOnly; Secure; SameSite=Lax`. Store the hash, not the
  token, so reading the database does not hand over live sessions.
- **No self-signup.** Accounts are provisioned: `mtglab users add|passwd|list|disable`.
- Rate-limit by account *and* IP; one generic failure message for both unknown
  user and wrong password; verify against a dummy hash on unknown users so
  response time does not leak account existence; regenerate the session ID on
  login; HTTPS only.

**Isolation is enforced in one place and tested adversarially.** Every
user-scoped query goes through a single accessor taking the session's `user_id`
— no handler writes its own `SELECT` against user tables. For every user-scoped
endpoint, a test logs in as user B, requests user A's resource and asserts
**404, not 403**, so IDs cannot be probed. Parametrised over the route table, so
an endpoint added without scoping fails the suite.

## Consequences

- "No self-signup" deletes an enormous amount of infrastructure and is why the
  monthly bill can stay near zero. A forgotten password is the maintainer
  running one command.
- The single scoped accessor is worth introducing *before* it has anything to
  isolate, as a FastAPI dependency returning the file-backed library for now.
  Auth then swaps one implementation instead of rewriting handlers.
- The isolation test is the highest-value test in the whole auth story, and it
  only works if it is generated from the route table rather than written per
  endpoint. A hand-maintained list of endpoints to check will miss the one
  someone adds next year.
- Nothing here ships until someone actually wants an account. Do not build auth
  speculatively; `docs/HOSTING.md` §6 deliberately puts a read-only deploy
  behind Cloudflare Access at step 2 and auth at step 5.
