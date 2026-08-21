# History

How sylvan-library was designed and built, kept for the reasoning rather than
for the record. Split out of `ROADMAP.md` and `docs/HOSTING.md` on 2026-08-21,
on Aaron's ruling, because both files had grown the same shape: a live head
sitting on top of a long landed narrative, where a fresh session had to read
past the second to find the first.

**This is not a changelog.** Git holds what changed. What is here survives
only because it carries the *why* a diff does not — the option that was
rejected, the measurement that killed a design, the bug that bought a rule.
When this file disagrees with a live one, the live one is right.

Where the live heads went:

| Question | File |
| --- | --- |
| What is being built next, and what the goals are | [`ROADMAP.md`](../ROADMAP.md) |
| How to deploy, run, back up and roll back the instance | [`docs/HOSTING.md`](HOSTING.md) §§0, 4, 5 |
| What is open right now, re-checked against the tree | [`docs/polish/LEDGER.md`](polish/LEDGER.md) |
| Why a decision was made, immutably | [`docs/adr/`](adr/) |

Two conventions carried across the split. **Section numbers travel with the
content**: the five sections below that came from HOSTING keep the numbers
they were cited by, so a docstring or an ADR saying "`docs/HOSTING.md` §1"
resolves here, at a section still called §1 — the ADRs are immutable and were
not edited to follow the prose. And **nothing was rewritten in the move.**
The sections below are verbatim; where one narrates a plan in the future
tense, that is the era it was written in, and the tense is the evidence.

---

## The deployment, as it was designed and built

The five sections that follow were `docs/HOSTING.md` §§1, 2, 3, 6 and 7. The
first three are the design — auth and isolation, the data model, and the cost
analysis — settled before deploy day. The last two are the build order and the
readiness list for a deploy that happened on 2026-08-13. HOSTING keeps §0 (the
architectural decision everything still rests on) and §§4–5 (the live runbook
CLAUDE.md points into), and carries a stub at each number pointing here.

---

## 1. Login and per-user isolation

> **Built, as of 2026-08-12** — the whole of this section's server side.
> `src/mtglab/auth/` is `app.db`, Argon2id, accounts, sessions, the login rate
> limiter and (step 5b) the token machinery, the `EmailSender` seam and
> `mtglab users invite`; `src/mtglab/api/auth.py` is the middleware and the five
> routes; `api/deps.py` is the scoped accessor. It is **off unless
> `MTGLAB_REQUIRE_AUTH` is set**, because the local single-user app is how this
> runs today and putting a login in front of it would be a regression.
>
> **The browser side landed 2026-08-12 too**, and with it the last code-side
> blocker on deploying with auth on. `web/src/routes/Login.tsx` and
> `Claim.tsx` are the two logged-out screens; `App.tsx` is the gate that decides
> whether either is ever shown, and it reads `auth_required` and `authenticated`
> as the two separate flags this section's endpoint was careful to report. With
> auth off nothing about the app changes — no login, no gate, no sign-in
> affordance — which is the property that made a login safe to build at all.
> §6 steps 5c and 5d, and §7 tracks both. What remains is infrastructure: a
> `Dockerfile`, a `fly.toml`, a `RESEND_API_KEY` and a verified domain.

### Use Argon2id, sessions, and no self-signup

You asked for username and password. Here is the shape that is both correct and
cheap to run:

**Password storage — Argon2id**, via `argon2-cffi`. It is the Password Hashing
Competition winner and the current OWASP first choice. bcrypt is acceptable;
anything else (SHA-family, PBKDF2 with low iterations, home-grown) is not.

One tuning note that matters on a small box: Argon2 is deliberately
*memory-hard*. The common `m=64MB` setting means each concurrent login
allocates 64 MB. On a 1 GB instance, a handful of simultaneous logins plus
DuckDB plus a running sim is how you get OOM-killed. Use the **OWASP minimum
profile — `m=19MiB, t=2, p=1`** — which is still far beyond brute-force
economics for a private site with a handful of accounts, and cannot exhaust
your RAM.

**Sessions, not JWTs.** An opaque random token (`secrets.token_urlsafe(32)`),
stored server-side in SQLite, sent as a cookie:

```
Set-Cookie: sid=<token>; HttpOnly; Secure; SameSite=Lax; Path=/; Max-Age=1209600
```

JWTs are the wrong tool here: they cannot be revoked without a server-side
denylist, which is the very state they exist to avoid, and this app has no
stateless-scaling problem to solve. A session row you can `DELETE` is simpler
and strictly more controllable. Store a hash of the token, not the token, so a
database read does not hand over live sessions.

**Invite-only, and the account holder owns the password.** Changed
2026-08-12; see [ADR 16](adr/0016-accounts-are-invited-and-passwords-are-self-served.md),
which supersedes this section's original "no self-signup, you run
`mtglab users passwd`" plan. That plan deleted a great deal of infrastructure
and was right about the cost — what it got wrong is that an admin-set password
is a password that has existed in plaintext in a chat window, and that a
forgotten password being "one command" is only cheap for the person running it.

```bash
mtglab users invite ada@example.com   # disabled account + single-use setup link
mtglab users list
mtglab users disable ada
```

What exists today is the half that needs no email. `add` prompts twice at the
terminal and **there is no `--password` flag**, because a password passed as an
argument is a password in the shell history and in the process table:

```bash
mtglab users add aaron --email aaron@example.com --admin   # prompts
mtglab users add ada --no-password    # the state an invite leaves behind
mtglab users passwd ada               # prompts; ends every session
mtglab users list                     # who exists, and who can log in
mtglab users disable|enable ada
mtglab users delete ada               # irreversible; type the name back
```

`disable` is almost always the one you want: it revokes every session and can
be undone. `delete` exists for the one thing disabling cannot do — a disabled
row still holds its `username` and `email`, both `UNIQUE`, so **an address
cannot be invited again until the account is gone.** It asks for the username
to be typed back (`--yes` for scripts), and it will delete the account you are
using, which the Admin page deliberately refuses to do.

The invitee follows the link and sets their own password. Password reset is the
same token machinery behind a second entry point — one implementation, because
a bespoke second path is how one of the two ends up weaker. Tokens are stored
hashed, single-use, and short-lived (an hour for a reset; longer for an invite,
which grants nothing until used). **Changing a password invalidates every
session for that user** — a reset is usually somebody suspecting compromise,
and one that leaves the attacker logged in has answered the wrong question.

**The reset endpoint answers identically whether or not the address exists**,
and is rate-limited per address and per IP. The login rules below already forbid
leaking account existence through timing; a reset form that says "no such user"
gives the same thing away through the front door.

Mail goes through **Resend**, behind an `EmailSender` protocol with a console
implementation for development. **No test sends mail**, the same rule that keeps
the Claude tests off the network. This is a real new dependency: an API key in
`fly secrets`, a verified sending domain, and deliverability as something that
can break.

What is *not* built is open signup. Sim jobs and the pool are expensive per
user, and an open door needs bot defence and an abuse story to protect something
with no revenue behind it. Opening it later is a flag and a rate limiter, not a
rewrite — the token machinery and the users table are the same either way.

**The rest of the checklist**, none of it optional:

- Rate-limit login by account *and* by IP. A fixed window in SQLite is enough;
  you do not need Redis.
- One generic failure message for both unknown-user and wrong-password. Never
  reveal which.
- Do not skip the hash on unknown users — verify against a dummy hash so
  response time does not leak account existence.
- CSRF: `SameSite=Lax` blocks the cross-site form post. Add a double-submit
  token for state-changing requests if you ever relax that.
- Regenerate the session ID on login (prevents fixation).
- HTTPS only. `Secure` on the cookie, HSTS on the response, and no plaintext
  listener.
- Log auth failures with timestamp and IP; do not log passwords or tokens.

### The lower-effort alternative, stated honestly

**Cloudflare Access** (free for up to 50 users) puts an identity gate in front
of the whole app and passes the authenticated email in a header. You would
store *no passwords at all*, write no login code, and have no credential-breach
surface — friends sign in with a one-time PIN emailed to them, or Google/GitHub.

It is genuinely less work and less risk than anything in this section, and you
still get per-user isolation because you key user rows on the verified email.
The cost is a Cloudflare dependency and a login flow you do not control.

You asked for username and password, so the guide builds that. But if the goal
is "friends can log in and see their own stuff," Access gets you there sooner
and safer. Worth ten minutes of consideration before you write auth code.

### Isolation — enforce it in one place, and test it

Per-user isolation fails when the `user_id` filter is sprinkled across handlers
and one is forgotten. Two rules:

1. **A single scoped accessor.** Every user-data query goes through one
   function that takes the session's `user_id`. No handler builds its own
   `SELECT` against user tables. A FastAPI dependency that yields a
   `UserScope(user_id)` object is the natural shape.
2. **A test that tries to breach it.** For every user-scoped endpoint, log in
   as user B and request user A's resource; assert 404 (not 403 — do not
   confirm the resource exists). Parametrise it over the route table so a new
   endpoint added without scoping fails the suite. This is the single highest
   value test in the whole auth story.

Return 404 rather than 403 for another user's object, so IDs cannot be probed.

**Both are built** (`api/deps.py`, `tests/test_isolation.py`), and two things
about how turned out to matter more than expected.

The first is that **the accessor is enforced by middleware, not by a
dependency**. A dependency has to be remembered on each new route, and the
route somebody adds in a year is exactly the one that will not have it — and it
will look entirely normal in review. Middleware runs before routing, so an
endpoint nobody protected is refused because nobody listed it. The allowlist is
`api/auth.py:PUBLIC_PATHS`, four entries long, and the test reads that same
constant so the two cannot drift.

The second is that the isolation test needed **something to isolate**, and the
user deck tier at step 6 does not exist yet. The answer was already in the app:
a background simulation job belongs to whoever submitted it, and its label
names the deck they are working on. So `jobs` is owner-scoped, and the
adversarial test is real rather than a placeholder waiting for step 6. The
generated part is what carries it forward — every `/api` route must be
classified public, shared or user-scoped, and an unclassified one fails the
suite with instructions.

---

## 2. Data model

Two embedded databases, zero managed services:

| Store | Engine | Contents | Access |
| --- | --- | --- | --- |
| `/data/mtg.duckdb` | DuckDB, 63 MB | Scryfall pool, prices | Read-mostly, rebuilt by `data refresh` |
| `/data/app.db` | SQLite | users, sessions, user decks, cached sim results | Read-write |

Keep them separate. DuckDB is an analytics engine holding regenerable public
data; SQLite is transactional state you must back up. Different lifecycles,
different backup rules — do not merge them for tidiness.

SQLite is the right call over Postgres here: it is a file on the volume you
already pay for, needs no second service, no connection pooling and no separate
`$7/mo`, and it will not break a sweat at this concurrency. Turn on WAL mode
(`PRAGMA journal_mode=WAL`) so readers do not block the writer.

Rough shape:

```sql
CREATE TABLE users (
  id INTEGER PRIMARY KEY, username TEXT UNIQUE NOT NULL COLLATE NOCASE,
  password_hash TEXT NOT NULL, is_admin INTEGER NOT NULL DEFAULT 0,
  disabled_at TEXT, created_at TEXT NOT NULL);

CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL, last_seen_at TEXT);

CREATE TABLE user_decks (
  id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id),
  slug TEXT NOT NULL, yaml TEXT NOT NULL, updated_at TEXT NOT NULL,
  UNIQUE(user_id, slug));

-- Built 2026-08-12; `auth/db.py` migration 3 is the real one. The sketch below
-- said "a pure function of deck content + parameters", which is close enough
-- to be dangerous: card facts come from the pool, so the key is a hash of
-- the *compiled* deck rather than of deck.yaml. See ADR 18.
CREATE TABLE sim_cache (
  key TEXT PRIMARY KEY, kind TEXT NOT NULL, result_json TEXT NOT NULL,
  created_at TEXT NOT NULL, last_used_at TEXT NOT NULL);
```

`sim_cache` is the one derived table in `app.db` and the only one it is safe to
drop: every row can be recomputed, and `mtglab sim cache --clear` does exactly
that. It lives here rather than in a third store because the rows are keyed to
decks on the same volume and have to survive a deploy the same way they do.

The `yaml` column of `user_decks` — a column, not a file — stores the same
YAML the file-backed decks use, so
`Deck.load` logic, the gate, and the artifact generator all work unchanged on
both tiers. One parser, one validator, two sources.

**A privacy rule that carries over from CLAUDE.md rule 5:** the reason
collection, wishlist and purchase data must never be committed is that a public
inventory of expensive cards tied to a real identity is a targeting list. That
reasoning does not stop at `git`. If those features ever ship, they are
per-user, behind auth, never in a public view, and never in a backup you put
somewhere shared.

---

## 3. Cost and compute — measured, not guessed

I profiled the Tier 1 engine on the Arahbo list before writing this section.

**Baseline throughput** (this machine, single core):

| Workload | Time |
| --- | --- |
| One game | 0.89 ms |
| 20,000 games (`sim mana`) | ~18 s |
| 11-count sweep at 25,000 games (`sim lands`) | ~4 min |

**Where the time goes.** `_consume` — the castability solver in
`sim/tier1/engine.py` — is ~21% of runtime by itself and roughly **50%
including callees**. It runs ~95 times per game. Below it, `expand_units` is
called 350k times and `ManaSource.units()` over 1M times per 2,000 games.

That last number looks like an obvious memoisation win. **It isn't** — I
tested it. `ManaSource` is a frozen, hashable dataclass, so I memoised
`units()` and measured again: **1.00x, no change at all.** The allocations are
cheap; the real cost is the adjacency-list construction and bipartite matching
inside `_consume`. Worth recording so nobody else spends an afternoon on it.

**Parallelism works, modestly.** Games are independent, so a process pool
scales:

| Workers | Time for 8,000 games | Speedup |
| --- | --- | --- |
| 1 | 7.13 s | — |
| 2 | 4.33 s | 1.65x |
| 4 | 2.95 s | 2.42x |
| 8 | 3.17 s | 2.25x (worse — startup and pickling dominate) |

On a 2-vCPU cloud box expect ~1.6x; on 4 vCPU, ~2.4x. Real, free, about twenty
lines of code. **Caveat discovered while measuring:** DuckDB takes an exclusive
lock *per process*, so pool workers must receive already-compiled `SimCard`
objects and must never open the pool themselves.

### The DuckDB locking rule, because it will bite you

The write lock is held by the **process**, not the connection, and DuckDB
allows exactly one writer. Two things follow, and an earlier draft of this
document got the second one wrong.

**Reads are fine, including across processes.** The API opens the pool with
`read_only=True` in `service._connect()`, and read-only handles share happily:
verified with four separate processes querying the pool simultaneously, all
succeeding. Within one process it is fine too — 12 concurrent requests against
`/api/decks` all returned 200, because single-worker uvicorn serves sync
endpoints from a threadpool. **`uvicorn --workers 2+` is therefore fine for
serving**, contrary to what this section previously claimed.

**Writes are exclusive, and that is the real constraint.** `db.connect()` —
the CLI path — executes schema DDL, so it opens read-write and takes the
exclusive lock. While `mtglab data refresh` is running, nothing else can open
the pool read-write, and `service._connect()` deliberately swallows the
failure and returns `None` so the app degrades to "no card pool" rather than
500ing every request. Expect a refresh to make card lookups briefly
unavailable; that is by design, not a bug to fix.

The rule that does still bind: **sim pool workers must receive already-compiled
`SimCard` objects** rather than opening the pool themselves, since each new
process would otherwise contend for a handle it does not need.

### Should you rewrite the simulator in Rust or Go?

**No. Not now, and not for cost reasons.** The measured case against it:

1. **Idle dominates the bill.** A `shared-cpu-1x` machine costs the same
   whether it simulates or sits there. With a handful of friends, it is idle
   ~99% of the time. A 20,000-game sim is **18 CPU-seconds**; a hundred of them
   a month is half an hour of CPU. You cannot save money you are not spending.
2. **Caching removes most of the work outright.** A sim result is a pure
   function of `(deck content hash, parameters, seed)`. Deck files change
   rarely and everyone views the same numbers repeatedly. Cache by that key in
   `sim_cache` and the common path costs *zero* CPU — a far bigger win than
   making the cold path 30x faster.
3. **A rewrite splits the codebase** into two languages and two toolchains for
   a workload that is not the bottleneck.
4. **ROADMAP already reached this conclusion** and it still holds: do not port
   Tier 1 pre-emptively; **a heavy simulator is where this decides itself.**
   That was written about Tier 2 — four seats making real decisions over more
   turns, plausibly 50-100x the work per game. Since 2026-08-11 Tier 2 waits
   behind a Forge feasibility spike
   ([ADR 14](adr/0014-python-decides-claude-advises.md)), so the trigger waits
   on whichever simulator gets built. Either way the measurement that should
   trigger a port does not exist yet.

**Do these instead, in order of value over effort:**

1. ~~**Cache sim results by deck-content hash.** Biggest win, smallest change,
   and it makes the hosted UI feel instant.~~ **Built 2026-08-12** —
   `sim/cache.py`, the `sim_cache` table in §2, and
   [ADR 18](adr/0018-a-cached-simulation-is-keyed-on-its-compiled-input.md).
   **"By deck-content hash" was the wrong key and the ADR is why.** Card facts
   come from the pool, so `deck.yaml` can sit byte-identical while a
   `data refresh` changes what a card does — Scryfall's "enters the battlefield
   tapped" retemplating is the documented case, and it moves the numbers for
   every deck. The key is a hash of the **compiled** deck instead: the SimCards
   the engine is handed, plus the clamped parameters, the seed, and a
   fingerprint of `engine.py` and `mana.py`'s own source. A hit costs a deck
   parse and one indexed `get_cards`, so it is milliseconds rather than truly
   zero — and in exchange a rationale edit does not throw the numbers away and
   a card pool refresh that matters does.
2. **Precompute the standard sims when a deck is saved**, so the numbers are
   already warm when anyone opens the deck. Cheap now: the cache exists, and
   this is one call to `plan_mana` on the write path. Not built, because
   warming a cache for a deck nobody may open is speculative work and the cold
   path is already only eighteen seconds.
3. **Scale to zero.** Paying only when someone is actually using it beats every
   micro-optimisation on this list combined.
4. **Process pool for sweeps** — 2.4x on 4 vCPU, remembering the DuckDB rule.

**When a rewrite does become right**, the boundary already exists and CLAUDE.md
put it there on purpose: `mana.py` and `sim/tier1/` are deliberately
stdlib-plus-numpy so they can move. Choose **Rust via PyO3/maturin** — it keeps
one deployable and one process, with no IPC or serialisation between the API
and the engine. Go would mean either cgo or a separate service; both are worse
fits for a library-shaped hot loop.

One more finding while profiling: **numpy is a declared dependency that the
Tier 1 engine barely uses.** The hot loop is pure Python lists and sets. That is
not a bug, but it means the "stdlib plus numpy" framing oversells the
vectorisation that is actually present — and it means a future rewrite has less
numpy to replace than you might assume.

---

## 6. Build order

Roughly ascending risk, each step independently useful:

1. **Environment-configurable paths** (`MTGLAB_DATA_DIR`, `MTGLAB_DECKS_DIR`).
   Nothing deploys without this, and it changes no local behaviour.
2. **Dockerfile + fly.toml, deploy read-only, no auth.** Just your six curated
   decks, behind Cloudflare Access or a single password, so you can follow
   along remotely. This alone satisfies the original goal.
3. ~~**Sim result caching.** Biggest performance win, and it is pure
   infrastructure — no user-facing change.~~ **Done 2026-08-12**, and the
   second half of that sentence turned out to be wrong. See §3 below and
   [ADR 18](adr/0018-a-cached-simulation-is-keyed-on-its-compiled-input.md):
   memoising a *sample* means deciding which sample, and the app was sending no
   seed at all — so the numbers on a deck were a fresh draw every time, and
   nothing was cacheable until that was fixed. The user-facing changes are a
   seed field, a **New sample** button, and every result now saying whether it
   was computed just now or read back.
4. ~~**`app.db`, users table, `mtglab users` CLI.**~~ **Done 2026-08-12.**
5. ~~**Sessions, login, the scoped accessor, and the isolation test** from §1.~~
   **Done 2026-08-12.** Steps 4 and 5 together were "auth core", and the claim
   that **all of it is testable locally with no deployment** held: 154 tests,
   no network, no container. What genuinely needs a deploy is as narrow as
   predicted — `Secure` cookies over real TLS, HSTS, proxy headers, and email
   deliverability.
5b. ~~**Invite, verify and reset over email** (ADR 16).~~ **Done 2026-08-12.**
   Schema version 2 in `auth/db.py`, `auth/tokens.py` serving both entry
   points, the `EmailSender` seam in `auth/mail.py` with a console
   implementation, `mtglab users invite`, and `POST /api/auth/reset` plus
   `POST /api/auth/claim`. Every rule it needed from step 5 was already there
   and tested, which is why it was a small build: `password_hash` is nullable
   so an unclaimed account is a real state, and `users.get_by_email` was
   written with a note that a miss must answer identically to a hit.
   What it cannot prove locally is the half it was split out for —
   deliverability. No test sends mail.
5c. ~~**A login screen**, and the claim page behind the emailed link.~~
   **Done 2026-08-12.** `routes/Login.tsx`, `routes/Claim.tsx`, and the gate in
   `App.tsx`. Four decisions worth finding here rather than in a diff:
   - *A gate, not a route.* When auth is on, `App` renders the login screen in
     place of the header, the nav and the router entirely — the same shape the
     server has, where the middleware refuses everything outside
     `PUBLIC_PATHS` before routing. There is no half-logged-in view for a nav
     bar to be useful in, so there is none to render. The one thing that
     renders *ahead* of the gate is `/auth/claim`, whose whole audience is
     people the gate would otherwise stop.
   - *One interceptor, not eleven catch blocks.* A 401 from any request
     announces a lost session from `lib/api.ts`, and the shell re-asks
     `/api/auth/me` rather than assuming what it meant. Same argument as the
     middleware: a per-screen check is a check the twelfth screen will not
     have. `login` and `me` are the two carve-outs, and a 401 from `login` is
     an answer about a password rather than a session that ended.
   - *The claim page reads `location.hash`.* Never the query string — see
     `auth/invites.py`. The token is held in component state, posted in a JSON
     body, and stripped from the address bar **on success only**, because a
     422 for a short password leaves the link intact and the retry needs it.
   - *The reset answer is rendered verbatim*, in a note that is neither green
     nor a confirmation. The endpoint says the same thing for an address with
     an account and one without; a UI that added "check your inbox!" would
     give away from the client exactly what ADR 16 built the server not to say.
     `routes/Login.test.tsx` pins it by asserting the rendered node's text
     *equals* the server's sentence.

   Auth off is untouched and pinned by `App.test.tsx`: no login, no gate, no
   sign-out button, because `auth_required` and `authenticated` are separate
   and only the first of them turns any of this on.
5d. ~~**The admin surface — an admin UI, and the authorization that gives it
   teeth.**~~ **Done 2026-08-12**, and decided in
   [ADR 17](adr/0017-the-maintainer-is-named-in-the-environment.md). `is_admin`
   had been on the account, on `UserScope` and in `/api/auth/me` since the auth
   core, with nothing reading it. Both halves landed together:
   - *Enforcement.* Admin routes live under `/api/admin`, and `api/auth.py`'s
     middleware refuses the whole prefix to a non-admin **before routing** —
     the same mechanism as `PUBLIC_PATHS`, so a route is protected by where it
     is mounted rather than by what its author remembered. `deps.Admin` is the
     second check on the handlers themselves, which is what an admin route
     mounted outside the prefix would have. `tests/test_isolation.py` gained an
     `admin` classification alongside public/shared/user-scoped: it is checked
     against the prefix in both directions, and the generated sweep logs in as
     a non-admin and requires **403**. 403 rather than ADR 5's 404 is
     deliberate — an admin route's existence is published in a public
     repository, so there is nothing for a 404 to hide.
   - *A surface.* `api/admin.py` — list, invite, promote/demote,
     disable/enable, send a reset link, revoke sessions — and the **Accounts**
     page behind it. No password field, and there will not be one: ADR 16 is
     unconditional that nobody chooses a password for anybody else.

   The maintainer requirement is met by `MTGLAB_ADMIN_EMAIL`; see §7. The CLI
   stays regardless: it is the bootstrap path (the first account on a fresh
   deployment predates anyone who could log in to create it) and the
   break-glass path when mail is misconfigured. `mtglab users promote|demote`
   landed with this, because `users.set_admin` had had no caller at all.
6. **User decks** in `user_decks`, reusing the existing YAML parser, gate and
   artifact generator.
7. **Process pool for sweeps** once anyone actually complains about the wait.

Stopping after step 2 is a perfectly good outcome if the multi-user part turns
out not to matter. "Do not build auth until someone wants an account" was the
rule here until 2026-08-12; somebody wants an account, so it is being built.

---

## 7. Deployment readiness — the running list

Started 2026-08-11, when hosting stopped being hypothetical. Everything above
is *how* to deploy; this is what was missing, kept as a live list so deploy day
was an afternoon rather than a discovery exercise. Tick things off here rather
than rewriting the sections above.

**Deploy day happened on 2026-08-13**, and the instance has been serving since.
The list is kept rather than deleted: what remains unticked is real work that
the deployment is running without — a written refresh runbook, the Forge
deployment shape, Cloudflare Access, and a second home for the API key. An
unticked box below is an open question, not a blocker that was ignored.

### Already true — do not re-solve these

- [x] **Paths are environment-configurable.** `MTGLAB_DATA_DIR`,
      `MTGLAB_DECKS_DIR`, and now `MTGLAB_FORGE_HOME` / `MTGLAB_FORGE_PROFILE`
      / `MTGLAB_JAVA`. This was build-order step 1 and it is done.
- [x] **Long simulations are already off the request path** — `api/jobs.py`
      and `api/simruns.py` run them in the background and the UI polls.
- [x] **The frontend is prebuilt and committed**, so the image needs no Node.
- [x] **Static files are served with a content type the app names itself.**
      Found on the instance 2026-08-13, hours after the tarot art shipped:
      every one of the 78 cards was going out as `application/octet-stream`.
      Starlette resolves a static file's type through `mimetypes`, which reads
      the **host's** database — and the slim image has no `/etc/mime.types`,
      so `.webp` answered `None` there. macOS knows the type and so does CI's
      ubuntu, so nothing local could see it; browsers sniff the bytes and
      render them, so nothing *remote* could see it either. It was one
      `X-Content-Type-Options: nosniff`, one strict proxy or one CDN image
      rule away from every picture in the app failing at once, in production
      only, behind a green suite. `api/app.py` registers `image/webp`
      explicitly and the `image` CI job asks the container the same question.

      **The fourth deployment-only fault, and the fourth of one shape.** A
      faked mail `Transport`, a stubbed Anthropic SDK, a mail client's
      linkifier, and now the operating system's mime database: every one is a
      property of the environment the code runs *in* rather than of the code,
      which is exactly what a test seam replaces. The habit worth keeping is
      the one that caught this — **ask the container, not the suite**, and
      check the response headers rather than the rendered page, because the
      browser was quietly compensating.
- [x] **CI refuses to let the pool or a key be committed** — by filename and
      by scanning every tracked file's contents, the built bundle included.
- [x] **Read-only DuckDB is safe across processes and threads**, verified in
      §3, so `uvicorn --workers 2+` is fine for serving.
- [x] **The auth core** — `app.db`, Argon2id, accounts, sessions, the login
      rate limiter, `mtglab users`, the scoped accessor and the adversarial
      isolation test. Build-order steps 4 and 5, landed 2026-08-12, **off
      unless `MTGLAB_REQUIRE_AUTH` is set**. Turn it on in `fly.toml`.
- [x] **No `SESSION_SECRET` is needed.** It was on the pre-deploy list below
      until the code was written; sessions are opaque random tokens stored as
      their SHA-256, so there is nothing to sign and no key to hold. One fewer
      secret to rotate, and the item is struck rather than silently dropped
      because a checklist that loses entries is a checklist nobody trusts.
- [x] **The email half of auth** — build-order step 5b, landed 2026-08-12.
      `mtglab users invite`, `POST /api/auth/reset`, `POST /api/auth/claim`,
      tokens stored hashed and single-use, and the `EmailSender` seam. What
      this does **not** tick is the provider itself: the code is done, the
      account and the verified domain are still deploy-day items below.

### Does not exist yet — this is the actual build list

- [x] **`Dockerfile`** — landed 2026-08-12, with `docker-entrypoint.sh` and
      `.dockerignore`. Two stages and still no Node; non-root app process;
      `HEALTHCHECK` on `/api/health`; no card pool, enforced against the built
      image and not just the tree. §4 step 1 records what the draft it replaced
      got wrong, including the single-worker argument, which was pointing at
      the wrong cause.
- [x] **`fly.toml`** — likewise, with four `[env]` placeholders to fill in and
      no secrets. **`MTGLAB_DECKS_DIR` is `/data/decks`, not `/app/decks`**:
      the draft would have thrown away every deck edit made in the app at each
      deploy. See the deck-drift note in §5, which is the question that choice
      creates rather than answers.
- [x] **A documented pool-seeding run** against the volume — §4 step 6, which
      also says where decks come from (a backup or an import — the image
      carries none since ADR 30, and since 2026-08-21 no laptop keeps a
      standing copy either). Not a build step and not a boot
      step: it needs
      several minutes and a ~500 MB download, and with scale-to-zero putting
      boot on the request path it would turn a wake into an outage.
      **This one needed a code fix to be true rather than only written down.**
      `download_bulk` defaulted to a relative `data/scryfall`, so the pool
      landed on the volume and the ~98 MB it is built from landed on the
      container's ephemeral layer; `config.SCRYFALL_DIR` is derived from
      `MTGLAB_DATA_DIR` now. `service.health()` had the same bug, which mattered
      more than it looks — it is the platform's health-check target, so a fully
      seeded instance reported no bulk files at all.
- [x] **The image is built and exercised in CI.** New, and load-bearing: the
      maintainer's machine is macOS 12 on Intel, where Docker Desktop will not
      install and Homebrew is too stale to build Colima, so **no container can
      be built locally at all.** The `image` job is the only place the
      Dockerfile is ever built. It builds `linux/amd64` and `linux/arm64`
      (the dev machine is Intel; anything deployed to is arm64), runs the
      amd64 image with auth on, and requires: `/api/health` answers 200 with
      `"pool": false`, `/api/decks` answers 401, PID 1 runs as `mtglab`,
      `/data` is writable by it, and no card pool file and no `deck.yaml`
      exists anywhere in the image (ADR 30). Trivy
      fails the build on HIGH/CRITICAL. **`image` is a required check on `main`
      as of 2026-08-12**, which it was not on the day it shipped — a check that
      is not required does not gate, and it sat passing-but-decorative for a
      day. `docs/ENGINEERING.md` §5 keeps the list; the settings themselves are
      the authority, so read them back rather than trusting either document.
- [ ] **A refresh procedure.** Cron does not work — Fly volumes attach to
      exactly one machine, so a scheduled second Machine cannot mount the
      pool. Monthly and by hand is the plan; write it down as a runbook.
- [x] **A login screen in the app**, and the claim page behind the emailed
      link. Build-order step 5c, landed 2026-08-12, and with it the last
      code-side blocker on deploying with auth on. The gate lives in `App.tsx`
      and is built on `GET /api/auth/me` answering `auth_required` and
      `authenticated` separately: with auth off neither screen exists and the
      app is byte-for-byte what it was, which is what `App.test.tsx` asserts
      first. A 401 from anywhere is handled once, in `lib/api.ts`, and brings
      the gate back rather than leaving a screen with an unexplained error.
      The claim page reads the token from `location.hash` (never the query
      string — see `auth/invites.py` for why), posts it to `/api/auth/claim`,
      and clears it from the address bar once it is spent; claiming sets no
      session, so it hands the username to the login form and stops there.
      Verified end to end against a real server with `MTGLAB_REQUIRE_AUTH=1`
      — invite, claim, sign in, reload, sign out, reset — not only under
      Vitest. `.claude/launch.json` has an `mtglab-ui-auth` entry that runs
      the app that way against a scratch `app.db`.
- [x] **An admin UI, and admin authorization that means something.**
      Build-order step 5d, landed 2026-08-12 under
      [ADR 17](adr/0017-the-maintainer-is-named-in-the-environment.md). Admin
      routes live under `/api/admin` and the middleware refuses the prefix to a
      non-admin before routing; `tests/test_isolation.py` has a fourth
      classification that is checked against that prefix in both directions and
      sweeps every admin route with a logged-in non-admin, expecting **403**.
      The Admin page does what `mtglab users` does, and as of the login
      screen above it is reachable with auth on — which it was not on the day
      it shipped.
- [x] **The maintainer is always an admin, on every instance.** All four parts,
      2026-08-12:
      - **A bootstrap admin.** `MTGLAB_ADMIN_EMAIL`, decided over
        first-account-wins because the requirement is a standing invariant
        rather than a moment: `auth/bootstrap.py` reconciles the named address
        to admin-and-enabled at **every start** of the app and of every
        `mtglab users` command, creating it unclaimed if absent —
        `MTGLAB_ADMIN_USERNAME` names its handle, or one is derived from the
        address. So a demotion,
        an accidental disable, or a backup restored from before the account was
        an admin are all repaired by a restart, and none of it depends on the
        volume surviving. **No mail is sent at boot** — the account is claimed
        from the sign-in page's reset link, which already serves unclaimed
        accounts, or over `mtglab users invite`.
      - **No last-admin lockout.** `users.set_admin` and `set_disabled` raise
        `LastAdmin` rather than removing the last admin *who can sign in* —
        enabled and holding a password, because an instance whose only admin is
        an unclaimed invite is locked out just as thoroughly. In the auth core
        and inside a `BEGIN IMMEDIATE` transaction, so the CLI, the routes and
        anything written later inherit it and two concurrent callers cannot
        walk through it together.
      - **A way to grant it after the fact.** `mtglab users promote|demote`,
        and the Promote/Demote buttons on the Admin page. `set_admin` had
        had no caller at all.
      - **Recovery that does not depend on the app.** `mtglab users` over
        `fly ssh console` still, and now a second path that needs no shell: fix
        `MTGLAB_ADMIN_EMAIL` and restart.

      One consequence to know before deploy day: **`MTGLAB_ADMIN_EMAIL` is
      credential-adjacent.** Whoever can set it is an admin on the next boot.
      That is already true of anybody who can run `fly secrets` or deploy, so
      it grants nothing new — but it belongs in the same care as the rest of
      the deployment config, and every change the reconciliation makes is
      logged.
- [x] **A transactional email provider.** New with ADR 16. The *code* landed
      2026-08-12; the account and the DNS followed, and `RESEND_API_KEY` is in
      `fly secrets` (verified 2026-08-14). The sending domain
      `send.sylvan-libraries.com` is verified with Resend, `MTGLAB_EMAIL_FROM`
      is an address on it, and `MTGLAB_BASE_URL` is set — without that last one
      every emailed link would point at `127.0.0.1`. All three are in
      `.env.example`. Note that `sender_from_env()` **refuses to start without
      the key when auth is on**, deliberately: the console fallback would print
      recipients into the platform's log, which ADR 16 forbids.
- [x] **Sim result caching** — build-order step 3, the biggest performance win,
      landed 2026-08-12 under
      [ADR 18](adr/0018-a-cached-simulation-is-keyed-on-its-compiled-input.md).
      `sim/cache.py` plus schema version 3 in `auth/db.py`; a hit is answered
      inside the request as a job that is already `done`, so no client changed
      shape. Two things it turned up that are worth knowing before deploy day:
      **the app was sending no seed**, so every view of a deck was a different
      sample and nothing could have been cached until that was fixed; and the
      obvious key — a hash of `deck.yaml` — would have served pre-refresh
      numbers forever, because card facts come from the pool and not from the
      deck file. `mtglab sim cache` reports what is stored and `--clear` empties
      it, which is a `fly ssh console` away if it is ever needed.

### Have these in hand before deploy day

Ticked items are done as of 2026-08-13.

- [x] ~~Fly account with a card on file~~ — done, billing set up.
- [x] ~~A domain~~ — `sylvan-libraries.com`, with `send.sylvan-libraries.com`
      verified with Resend and its DNS in place. The Fly TLS step (§4 step 7)
      is still to run.
- [x] ~~`MTGLAB_EMAIL_FROM` and `MTGLAB_BASE_URL`~~ — both are real values in
      `fly.toml` now rather than placeholders, since neither is personal data
      and a placeholder here is only discovered once a reset link has already
      gone out wrong.
- [x] ~~`SESSION_SECRET` generated (`openssl rand -base64 32`).~~ **Not
      needed** — see above. Sessions are opaque tokens, not signed ones.
- [x] ~~**`MTGLAB_ADMIN_EMAIL` and `MTGLAB_ADMIN_USERNAME` via `fly secrets`.**~~
      Both set and deployed (verified 2026-08-14). These two stay placeholders
      in the tracked file — the address is the one piece of personal data the
      project handles — so this is the step nothing in the repository can do
      for you, and **a deploy without it comes up looking healthy with nobody
      behind the sign-in page who is you.** `MTGLAB_REQUIRE_AUTH=1` is set in
      the committed file.
- [x] ~~`RESEND_API_KEY` via `fly secrets`, and one real invite sent~~ — the
      key is deployed (verified 2026-08-14) and an invite has been sent,
      claimed and signed in with. Deliverability is the one part of the email
      half that no test covers, by design. Note there is no console fallback on
      a deployed instance (§4 step 8): with auth on, a missing key refuses
      rather than prints.
- [x] ~~The first sign-in~~ — done 2026-08-13 via
      `mtglab users passwd gyome` over an **interactive** `fly ssh console`,
      which is the path that needs no working mail. There is no
      `MTGLAB_ADMIN_PASSWORD` and there will not be one (ADR 16) — the
      bootstrapped account is unclaimed, so what it is missing is a password
      rather than an account.
- [x] ~~Seed the volume~~ — done 2026-08-13. 35,390 oracle cards and 107,338
      printings, and the resulting `mtg.duckdb` is byte-for-byte the same size
      as the one built on the maintainer's laptop. The gate then passed
      gyome-food, arahbo-cats and trostani-tokens clean and failed
      goreclaw-stompy on Primeval Titan, which is the documented state and the
      strongest single proof that the pool is real card data rather than an
      empty table.
- [ ] Cloudflare Access configured, if the instance is not to be public.
- [ ] **A second home for `ANTHROPIC_API_KEY`.** One key, one environment, two
      places once deployed: the gitignored `.env` here and `fly secrets` there.
      A rotation is not done until both are updated. **The current key is
      30-day and expires in early September 2026** — a `401` after that is the
      expiry, not a bug, and there is no staging key to fail first. Consider
      switching to a **Never**-expiring key once `fly secrets` is holding it,
      which is the documented choice for a secrets manager you rotate yourself.
- [ ] A spend limit set on the API workspace, as the backstop.

### Decisions that must be made before, not during

- [x] ~~**How the maintainer becomes admin on a fresh instance** — first
      account wins, or `MTGLAB_ADMIN_EMAIL`.~~ **Decided 2026-08-12:
      `MTGLAB_ADMIN_EMAIL`**, and implemented the same day. First-account-wins
      was rejected for being a one-time event rather than a standing property,
      and for a plausible order of operations that gives a friend the flag
      silently: `mtglab users invite friend@example.com` on a fresh volume,
      before the maintainer's own account exists. The full argument is
      [ADR 17](adr/0017-the-maintainer-is-named-in-the-environment.md).
- [x] **The Forge deployment shape** — decided 2026-08-20, an on-demand
      worker machine ([ADR 35](adr/0035-the-forge-joins-the-simulator-and-a-worker-runs-it-hosted.md)).
      The section below records what was built and how to provision it.
- [x] **Fly or Hetzner.** Resolved by the same decision: the Hetzner case was
      "Forge on the app box", and ADR 35 put Forge on its own Fly machine
      instead — dedicated CPU while it runs, nothing while it sleeps. The app
      machine's sizing is untouched.
- [ ] **Whether Claude research is gated per user.** The interview is cheap
      (~$1–1.50 a draft on Sonnet 5); research is the unestimated half. ADR 15's
      stance dial doubles as the control, and *off* is a defensible default.

### The Forge worker, as built (ADR 35's second half)

The spike's costs, for the record — measured 2026-08-11 on the 2015 MBP, and
the numbers that sized the worker:

| | Cost |
| --- | --- |
| Forge distribution in the image | ~470 MB unpacked |
| A JRE (21, headless) | ~190 MB more |
| JVM boot + card database | **~9s per `sim` invocation**, flat, amortises over `-n` |
| A heads-up game | median 4.6–6.8s across every archetype |
| The tail | **one Trostani game took 134s** |
| Four-player pods | 40% hit the clock — why the app surface is heads-up only |
| Heap ceiling | `-Xmx3072m` on the worker (the shim's default), on a 4 GB machine |

**The shape.** A second machine named `forge-worker` in the same Fly app,
built from `Dockerfile.forge` (JRE 21 + the Forge 2.0.14 distribution,
SHA-256-pinned + `sim/tier3/shim.py` in front of `run_games`). Dedicated CPU
(`performance-2x`), because ADR 35's core argument is that shared-core
throttling stretches the measured tail into the clock and corrupts results
into fake draws. The machine is **stopped** between matches: the app starts
it per job through the Machines API, talks to the shim over the private
network (`<machine-id>.vm.<app>.internal`), and the shim exits after
`MTGLAB_FORGE_IDLE_SECONDS` of quiet — its `restart: no` policy turns that
exit into `stopped`, which bills nothing but rootfs. A ten-game match plus
the idle window costs on the order of a cent.

**Lifecycle.** The deploy workflow owns the machine: it builds and pushes the
worker image to the app's private registry, creates or updates `forge-worker`
with it, wakes it once so the host pulls the image, and stops it. The machine
carries no `fly_process_group` metadata, so `flyctl deploy` never touches it.
The app never creates infrastructure from a request thread — a missing
machine is a 503, and the fix is running a deploy.

**Provisioning, once per instance** (the two secrets the workflow cannot set):

```bash
fly tokens create deploy -a sylvan-library    # then:
fly secrets set MTGLAB_FLY_API_TOKEN=<that token> \
                MTGLAB_FORGE_SHIM_TOKEN=$(openssl rand -hex 32)
```

`MTGLAB_FORGE_WORKER=1` is already in `fly.toml`; the gate at `/api/forge`
answers yes only once the token secret exists, so a deploy before this step
is honest rather than broken. Setting secrets restarts the app machine.
Rollback is the same lever in reverse: `fly secrets unset MTGLAB_FLY_API_TOKEN`
hides the mode again, and `fly machine destroy forge-worker` reclaims the
rootfs; the next deploy recreates it.

The spike's four code-change warnings all landed inside the build rather
than as runbook steps: the profile is baked at image build (`Dockerfile.forge`
writes exactly what `ensure_profile` would), the `.dck` race is closed by the
one-worker `FORGE` lane plus the shim's own match lock, and the
working-directory constraint is the image's `WORKDIR`. The licensing answer
moved with the shape: the image *does* contain Forge now, and stays compliant
because it is pushed only to the app's private registry — deployment, not
distribution. [NOTICE.md](../NOTICE.md) §Forge carries the full argument;
the one thing that must never happen is that image reaching a public
registry.

**The honest read right now:** local-only is unblocked and costs nothing.
Server-side is affordable on a Hetzner box and not on the 1 GB Fly instance
§3 prices. A worker is what the 134-second tail actually argues for, and
`api/jobs.py` already has the shape — but it is the most moving parts for a
feature nobody has asked for yet. **Measure the pods before choosing between
the last two.**

---

## The phase-by-phase account

Sixteen phases, 2026-08-12 to 2026-08-18, in the order they were worked. This
was the back half of `ROADMAP.md`; the summary that used to introduce it is
still there, pointing here. Fifteen landed. The two that did not close
cleanly are called out where they sit: item 3's UI/UX punchlist is Aaron's
list and closes only when he says so, and item 5's remainder — driving the
deployed instance by hand — is still open. Item 16 names its own next branch,
region-scoped motion, which has not been built.

1. ~~**The best-practices and cleanup pass.**~~ Landed 2026-08-12: the API
   catch-all refuses `/api` misses as JSON, dead code out, mode tool sets
   enforced at dispatch, four-colour names in, `api/service.py` and
   `api/simruns.py` strict under mypy, route-level code splitting (262 kB
   entry, Recharts lazy), actions SHA-pinned with dependency review and
   Dependabot, the mode prefix prompt-cached, version 0.2.0. Still open from
   that pass, deliberately deferred: mutation testing, golden artifact
   snapshots, Playwright/axe, SBOM and image signing, and making
   `dependency review` a required check.
2. ~~**Manual UI testing**~~ — done 2026-08-12, a person driving the app end
   to end rather than a suite. Before it, all seven screens and the catch-all
   were smoke-tested clean (every lazy route mounts, no stuck Suspense
   spinner, no console errors), so what the tour found is UI/UX rather than
   breakage. The auth-on configuration (`MTGLAB_REQUIRE_AUTH=1`, the
   `mtglab-ui-auth` launch entry) is still worth a pass of its own, since it
   is the configuration Fly actually runs.
3. **UI/UX polish** — the punchlist from that tour, and the phase that now
   stands between here and deploy. The list came from the maintainer on
   2026-08-12 and is written out below so it survives a fresh session; it is
   his list, so **an item is done when he says it is, not when it compiles.**
   Five branches, each green before the next starts. Branch 2 was not on the
   original list — it came out of reviewing branch 1's deck page and displaced
   the rest by one.

   **The order is now 1, 2, 5, 3, 4**, changed 2026-08-12 after the branch 2
   review; **all five have landed as of 2026-08-13**, and what stands between
   here and deploy is item 4 below. The numbers are identities, not positions, so nothing renumbers.
   What moved 5 to the front is worth recording because it is a finding rather
   than a preference: asked to test branch 2, the maintainer went looking for
   an *interactive* Claude assist in the deck builder and found none. He was
   right — "Start a deck" has no Claude in it at all, and the one interactive
   surface that does exist, the rationale interview, is reachable only by
   opening a deck, clicking *Edit why* on a card, and then *Ask for questions*.
   Nothing announces it. So the four modes built or planned so far are a read
   surface and a hidden one, and the thing that would make the app feel like it
   has an assistant in it is branch 5. Visual identity and teaching are worth
   doing and neither of them changes that.

   **1 — Bugs and quick wins.** Landed 2026-08-12 in
   [#55](https://github.com/aasquier/sylvan-library/pull/55).
   - *Delete was unusable.* The confirm label was styled `uppercase` while the
     check was case-sensitive against the lowercase slug, so typing what was
     on screen left the button disabled and said nothing about why. The
     confirmation is now the word `bury` (`service.DELETE_WORD`, and the slug
     still works), matched case-insensitively, with the reason shown when it
     does not match. Regression-tested on both sides.
   - *Deck notes read like source code.* 17 `TODO —` markers across six decks
     became prose or a bare `—`. Artifacts regenerated for the four decks the
     gate lets through; Atla and Goreclaw still refuse on their banned card.
   - *The commander was cropped out of its own page.* `art_crop` is 1.37:1 and
     the hero band was 4.6:1, so it kept a third of the painting's height from
     the middle. The band is atmosphere now and the commander is the whole
     card, uncropped, with the usual hover. Library tiles went to `art_crop`'s
     own ratio for the same reason.
   - *Dead carousel controls* on Colourless and All five, which have one
     member each.
   - *Tier context lived inside the first combination of each tier* — Bant
     explained shards, Abzan explained wedges, Artifice explained four-colour
     identities — so it was invisible to anyone who arrowed past them.
     `colors.TIER_BLURBS` is where it lives now, one per tier including the
     four that never had one. A blurb is mechanical and never names a plane;
     an era is a setting and never restates the mechanics.

   **2 — The commander dossier, and alternative arts.** Landed 2026-08-12 in
   [#57](https://github.com/aasquier/sylvan-library/pull/57), with
   [ADR 19](adr/0019-the-dossier-cites-three-sources.md) written first. Branch 1 answered "what does this card do" with pool counts; this
   is the *interesting* half — who this character is, what archetype they
   define and where it came from, their rivals, where they sit in Magic's
   history. The second Claude mode, and the first whose facts do not all come
   from the pool.

   What the build settled, beyond the decisions below:

   - **Web search and structured outputs coexist, and that is what makes the
     source check load-bearing.** A response schema *suppresses* the API's own
     citations, so a URL in the payload is a string the model typed and nothing
     more. `dossier.keep_sources()` intersects the cited pages with the ones
     `Turn.searched` recorded and drops the rest, counting what went — the
     answer to "how do you know it read that" is now a set intersection rather
     than a promise. Measured on Gyome: 54 pages read, 4 cited, 0 dropped.
   - **Two bugs that only appear with a server tool *and* a second turn**, both
     found by running the mode rather than reading the request shapes. The
     dated search filters its results inside a code-execution container, so a
     follow-up request carrying that turn's blocks is a 400 unless the
     container id comes with it. And a server-side tool loop that hits its own
     limit stops with `pause_turn` carrying text that reads finished — the same
     shape as a Forge game that plays on with 96 cards, and now resumed rather
     than returned.
   - **A rule-1 leak in the one sentence most likely to have one.** A first run
     described Trostani Discordant as making Food tokens; she makes 1/1
     Soldiers. Comparing two commanders is exactly the sentence that wants a
     half-remembered ability, so the mode now must call `get_cards` on every
     rival before writing about it, and the rival's real pool text rides in
     the payload so the card sits next to the sentence. The second half is what
     does not depend on the model complying.
   - **Cost:** about 800 uncached input tokens and 2,100 out per commander,
     with ~57k served from the prompt cache. Once per commander, ever, because
     the key is the `oracle_id`.

   Decided with the maintainer on 2026-08-12, so a session does not re-open
   these:

   - **Three sources, and a rule about which may support which claim.** Card
     facts — cost, type, text, legality, identity — come from the pool,
     always; never from web search and never from recall. The meta, archetype
     history and "where does this sit in Magic" come from **server-side web
     search with its sources shown** (`web_search_20260209`; Anthropic-hosted,
     so the no-crawler rule is intact). Claude supplies voice and framing and
     carries no factual weight. The UI shows the seams: branch 1's counted
     strip stays, Claude's prose sits below it labelled, web claims keep their
     link. That is ADR 14 boundary 3 made visible.
   - **It writes nothing to `deck.yaml`.** `may_write` stays empty and ADR 15's
     invariant is untouched. The result is cached like Tier 1 results, keyed on
     the commander's `oracle_id`, so it is generated once and shared by every
     deck that commander leads — including across users on a hosted instance.
   - **Generated automatically for new and imported decks**, on a button for
     the existing six. At stance `off` the button does not appear, because off
     means no calls (ADR 15).
   - **Any card the model names is validated against the pool**, the way the
     interview drops anything that is not a question.
   - **Alternative arts**: a `commander_art` field on `deck.yaml` holding a
     printing id, a picker showing every **non-digital** printing newest first
     (Goreclaw has 12, including a Secret Lair; Gyome has 3), and
     `mtglab decks set <slug> --art <set>`. A deck property rather than a
     per-viewer preference — `deck.yaml` is the source of truth and the choice
     should travel with the deck through git. Note `printings` has
     `image_normal` but no `image_art_crop`, so the hero band needs one or the
     other resolved.

     *Built.* The crop is **derived** rather than stored: Scryfall's image URLs
     differ only in the size segment, which `oracle_cards` proves by carrying
     both for the same printing id, so `service.art_crop_from` swaps `normal`
     for `art_crop` and returns None on any other shape. That avoids blocking a
     UI branch on a 500MB re-ingest, and a column can still be added later
     without changing a caller. Two checks in two layers: `edit.py` refuses a
     value that is not a UUID (it is text surgery and has no database), and
     `service.py` refuses a printing that is not **this commander's** (only a
     query can know). A set code with several printings — `MUL` has four
     Goreclaws — lists them and refuses rather than picking one.

   **3 — Visual identity.** Built 2026-08-13. No ADR: the one decision that
   needed making had already been made — card art stays a Scryfall hotlink and
   everything else is **drawn in SVG/CSS**, no new binary assets, no licensing
   question — and nothing in the build moved it. The scope was the splash and
   the Sylvan Library art (rendered only on an *empty* library, so the
   maintainer had never seen it), an interactive colour pentagram for the mono
   tier, and the builder's tier headers, which were plain grey panels.

   What the build settled:

   - **The five mana symbols are drawn now**, which was not on the list — the
     maintainer raised it mid-branch, and it is the change that touches the
     most screens because `Pip` is shared by every cost, every prose pip and
     every identity ring. A lettered disc was a placeholder that had stopped
     looking like one. Drawn rather than hotlinked from Scryfall, and the
     reason is not licensing: **a pip is inline in a sentence**. The deck files
     carry 174 across `why`, `strategy` and `notes`, the gate's own errors are
     the densest of them, and `/api/colors` is the one page that works with no
     pool and no network. Card art is decorative and lazy; prose is neither.
     Checking Scryfall's own path data in would also be redistributing their
     asset rather than hotlinking it, which is the line rule 5 draws. ~2 kB,
     no requests, works offline.
   - **Only the five colours get an icon.** A numeral is a numeral, `{X}` is a
     letter, a hybrid is two colours no single glyph states — so the branch is
     `hasGlyph` and the text path is untouched. `{C}` is the one that is merely
     not done rather than argued against.
   - **A drawn pip had to be given a name.** A lettered one reached the
     accessibility tree as the character "G"; a drawing contributes nothing, so
     `role="img"` and the colour's name are explicit. Caught by an existing
     test that read the letters out of `textContent` — the failure was the
     feature working.
   - **The pentagram's geometry is derived, not tabulated.** Five vertices in
     WUBRG order; adjacent is allied, two apart is enemy. That yields exactly
     the ten guilds, once each, and the chords are what draw the star. Every
     name comes from the taxonomy the page already fetched, so there is no
     second copy of `colors.py` — pinned from both sides, by
     `tests/test_colors.py` against the table and by a frontend test that
     renames a guild in the data and watches the diagram rename it.
   - **The tier badges are the same wheel at 26px**, each with its own shape
     lit, which makes the two tiers people confuse self-explaining: a shard is
     an arc and mostly solid, a wedge is a span and mostly dashed. Same three
     dots, opposite texture.
   - **The `art_crop` ratio lesson has now been learned three times.** The
     hero band on the empty library is 3.08:1 over a 1.36:1 painting, so it
     kept **44% of the height from the middle** — bare wall, with the sky, the
     path and the three figures that give the canyon scale all cropped away.
     Branch 1 fixed exactly this on the deck hero and the library tiles and
     could not have caught this one, because the only screen that renders it is
     one an instance with decks on it never shows. The nameplate therefore
     shows the painting **whole, beside the title**, which is branch 1's own
     answer; the empty-library band keeps its crop but is anchored low, where
     the subject is.
   - **A dark-mode filter tuned on the wrong sample.** `brightness(1.75)`
     assumes card paintings are dark and vanish against a near-black panel.
     *Sylvan Library* is a bright yellow-green forest and it bleached — the
     app's own art was the one image the rule was never checked against. 1.3
     now, and the nameplate's copy gets 1.12 because it is the picture rather
     than a wash behind text.
   - **Moving the title into the nameplate took the empty library's `h1` with
     it**, found by running the app rather than by the suite. Fixed and pinned:
     exactly one `h1` and exactly one copy of the painting in both states.
   - **Cost:** entry chunk 262 kB → 266 kB (84 kB gzipped). The pentagram rides
     in the lazy `NewDeck` chunk; only the glyphs are in the entry.

   **4 — Teaching.** Built 2026-08-13. A vocabulary section for beginners;
   hover help in the simulator, whose parameters are words and numbers
   divorced from meaning; and real depth behind the guilds, shards, clans and
   colours — champions, plot lines, classic cards. No ADR: the one decision
   that needed making is recorded here and in `colors.py`'s own docstring,
   and nothing in the build moved it.

   The decision, made before any code: **the depth is checked-in reference
   prose, not a Claude surface.** A guild is exactly the kind of question
   ADR 19's dossier answers well, so it is worth writing down why not.
   `/api/colors` is the one page in the app that works with no card pool and no
   network — a stated property, pinned in `tests/test_isolation.py` — and a
   model call would spend it on the screen a brand-new player meets first. The
   set is finite: ten guilds, five shards, five wedges, written once, ever, so
   a per-view call pays repeatedly for content with no variance in it. ADR 20
   had already classed `colors.py` as a **fourth source** alongside the
   dossier's three — checked in, carrying `verified_by`, and free. And the
   complaint that produced the work was that the prose was *bland*; bland is
   fixed by editing, and only checked-in text can be edited. What Claude
   answers is the unbounded, per-deck question about a **commander**, which is
   ADR 19 and stays there.

   What the build settled:

   - **The teaching content moved out of a wizard.** The whole colour taxonomy
     rendered only inside "Start a deck" — a screen you pass through on the
     way to something else. Reference material reachable only mid-task is
     reference material nobody reads twice. `/learn` is a sixth nav item and
     the home of all three pieces; the create flow keeps a **short version**
     (the champions named, and a link across) so it stays a chooser.
   - **A champion is a character; the card is the evidence.** `Champion(card,
     role)` holds a card *name* and one sentence about who they are in the
     story — the only thing the card cannot say about itself. The page
     resolves the name through `get_cards` and renders the real card's cost,
     type and oracle text directly beneath the sentence, so a role that
     drifted from the card is visible next to what would disprove it. A name
     that does not resolve is **dropped and counted**, which is ADR 19's
     rivals instrument pointed at reference data.
   - **`signature` carries no prose at all, and that is the design.** Three or
     four cards per combination whose colour identity is **exactly** that
     combination — so the list asserts a checkable property rather than an
     opinion, and there is no sentence in it for a card fact to be wrong in.
     "Exactly these colours" is also the most direct available answer to what
     a combination is *for*.
   - **`exact_total` is counted live and teaches more than the paragraph does.**
     Exactly **two** cards in the pool have the Artifice identity, and the
     page says so. No four-colour blurb about refusing green lands as hard.
   - **144 card names, every one verified against the pool before it landed**
     — 51 champions and 93 signature slots. Two rules, and they are not the
     same rule: signature and `verified_by` must be *exactly* the
     combination's identity, a champion need only be a *subset*, because a
     faction is a story and the story owes the colour pie no exact match.
     Folded into the existing `needs_full_pool` test rather than added as a
     second one, so CI's skip gate stays pinned at two.
   - **The vocabulary is one table with two kinds of entry**, deliberately:
     Magic words (commander, colour identity, ramp, goldfish) and *this
     tool's own* controls and measures (`sim.min_pieces`,
     `stat.deployment_spread`). Both are words and numbers divorced from
     meaning to a newcomer. Keeping the simulator's half in `glossary.py` next
     to the `KeepRule` that defines it is what makes it checkable —
     `SIMULATOR_KEYS` in `tests/test_glossary.py` fails if a control on the
     screen has no entry, which TypeScript cannot do against a Python table.
   - **Twenty of the 32 get a story and twelve do not.** Mono-Red is not from
     anywhere. The test asserts that in both directions, because a non-faction
     with lore is a paragraph invented to fill a field.
   - **Two bugs only the running app showed**, which is now six branches for
     six. The colour wheel captions whatever `selected` it is handed, and on
     Learn that is any of the 32 — a four-colour key is neither a vertex nor a
     chord, so it found no edge, fell through, and described **Artifice as an
     "enemy pair — opposite on the wheel"** with a button offering to cross to
     the guilds. And the help popover is a descendant of the field label,
     which on the simulator is `uppercase tracking-wide`, so every sentence of
     help arrived SHOUTED AND LETTER-SPACED. Both are pinned now.
   - **Cost:** entry chunk unchanged at 266 kB (84 kB gzipped). `Learn` is
     lazy at 10.9 kB and the pentagram became a shared chunk, since two lazy
     routes now draw it.

   **5 — Claude in the builder.** Built 2026-08-12, with
   [ADR 20](adr/0020-the-theme-interview-reads-a-person.md) written first.
   Moved ahead of 3 and 4 for the reason recorded above. A guided, adaptive
   interview that helps pick a theme and a commander, plus the discoverability
   fix below. The refactor pass stayed out and remains goal 10. Rule 4 is
   untouched: no mode writes a `why`.

   What the build settled, beyond the decisions below:

   - **The questions are not about Magic, and that is the whole feature.** The
     first draft asked "when you picture yourself winning, what is on the
     table" — a Magic question wearing a friendly hat, unanswerable by exactly
     the person this is for. It asks about a film, a period, a star sign, how
     somebody is at game night, and translates. That works because the colour
     pie is a personality taxonomy before it is a set of mechanics, which is
     also why `colors.py` is a **fourth source** alongside ADR 19's three:
     checked in, carrying `verified_by`, and free.
   - **Readiness is a grounded-slot count.** Every reading the mode takes
     carries a quote, Python checks the quote against the user's own turns, and
     three surviving kinds opens the proposal. Third instrument after
     `only_questions()` and `keep_sources()`, pointed at a model reporting back
     a preference nobody expressed.
   - **Three bugs that only appear when you run it**, none visible from reading
     the shapes. The interviewer speaks first, so the transcript starts with an
     assistant turn and the request needs a synthetic user frame or every
     answer is a 400. Alternation was enforced on a false premise — the API
     combines consecutive same-role turns — and enforcing it wedged any
     conversation where a turn came back without a usable question. And the
     proposal ran **zero searches** on its first outing, resting archetype
     claims on nothing, until the prompt was made prescriptive about when to
     call the tool.
   - **A dropped commander can cost a whole suggestion.** A legend of a
     *subset* identity is legal in those colours and does not make a deck that
     fills that slot, so it is dropped — and when all three go, the combination
     goes with them. Observed live: one run returned two combinations and the
     next returned one. Counted and surfaced now rather than silently thin.
   - **Cost and time:** a conversation turn is heavily prompt-cached (~48k
     cached tokens by turn three). It was described here as "a few seconds"
     until somebody measured it: **4.3–37.7s across eleven turns on the
     instance, with one at 133.8s**, and 27.7s for an ordinary turn driven
     locally. That sentence is why 5c exists. The proposal is the expensive
     half — **measured at 226 seconds** end to end with `max_uses: 4`, ~79k
     input / 8k output, since it reads a dozen-odd pages and checks every
     legend. Trimmed to three searches, and the UI says it takes a few minutes.
     That was **the deploy blocker**, and it is fixed — see 5b below.

   **5b — The proposal is a background job.** Landed 2026-08-13, on the branch
   that also carries this paragraph. No ADR: nothing ADR 20 settled moved. The
   transcript is still client-held and resent, the server still stores no
   conversation, readiness is still recomputed rather than carried, and the
   wire format is still the mode's own. What changed is only how the answer is
   delivered.

   - **Checking happens in the request; calling happens in the job.** The same
     division `plan_mana` makes, and for a sharper reason: three things refuse a
     proposal without a network call — a malformed transcript (422), a floor not
     yet reached (409), no key (503) — and each is a distinct answer the UI acts
     on. Carried into a worker they would all arrive as *a job in state `error`*,
     which is one string for three cases and a status code for none. So
     `theme.check_proposal` runs in the route and `api/themeruns.py` queues only
     what needs Anthropic. A stance of `off` is a job born finished, the shape
     `jobs.completed` already existed for.
   - **There are two job pools now, and the split is about what the work waits
     on.** Tier 1 is CPU-bound pure Python and keeps its single worker, because
     a second thread would contend on the GIL. A Claude call is a socket wait
     that releases it for minutes, so sharing one queue would stall a
     thirty-second sweep behind four minutes of somebody else's conversation.
     `jobs.CPU` and `jobs.NET`; the lane rides on the `Plan` because it is a
     property of the work rather than of the route.
   - **Nothing is cached, deliberately.** ADR 18 caches a simulation because it
     is reproducible; a proposal is not, and the dossier is cached because its
     subject is a character that outlives any conversation. Caching here would
     mean the one moment somebody wants a different answer — clicking again on
     an unchanged transcript — is the moment they cannot have one. The client
     keeps the job *id* instead, so a reload reattaches to the run in flight
     rather than paying for a second.
   - **Two things only the live run showed**, which is now the fourth branch
     running where that has been true. A four-minute job reporting nothing is
     indistinguishable from a wedged one, so `converse` gained an `on_turn`
     hook and the job reports turn *n* of 8 (a ceiling it usually does not
     reach, so the UI shows seconds rather than a bar that would sit at 38% and
     jump). And the first reattach after a reload showed **0s against a job
     already 70 seconds old** — the clock now reads the job's own `created_at`,
     which is the run's age rather than this tab's.
   - **Measured on the real surface, twice:** 15 pages read, 4 cited, 0 sources
     dropped, 0 commanders dropped, ~72k in / 5.8k out with 53k served from the
     prompt cache. A reload mid-run reattached to the same job both times.

   **5c — The conversation turn is a background job too.** Landed 2026-08-13,
   the third surface to make the same move and the one that had the weakest
   case on its own numbers. No ADR: as with 5b, nothing ADR 20 settled moved.

   - **The reason is not the outlier.** One turn in eleven at 133.8s did not
     reproduce, and restructuring a chat box on a single data point would be
     wrong. The reason is that `api/app.py` justified keeping it synchronous
     with *"it is a few seconds"* — **word for word the sentence that left the
     dossier synchronous until it broke deployed at 236s.** A duration measured
     for one surface is a question to ask of every sibling surface, and this
     was the sibling nobody asked.
   - **The ceiling is unknown, and that is the argument.** All anybody knows is
     that it is *at or below* 236s, because that is where the dossier failed.
     133.8s sits inside the unmeasured region below it. Measuring it properly
     would take a throwaway endpoint that holds a response open, a deploy to
     put it there, a binary search, and a deploy to remove it — for a number
     that is multi-hop (Fly's proxy, then Safari, then whatever network), that
     Fly can change without telling anybody, and that would not change the
     decision unless it came back above ~240s. Considered and rejected on
     2026-08-13; the fix costs less than the measurement.
   - **The failure being avoided is the bad kind.** A transport error carries
     no status code, writes no access-log line — uvicorn logs a response when
     it *completes* — and discards work that finished fine. That is exactly
     what the dossier looked like from a browser: a spinner, then `Load failed`.
   - **The cheap case stays one request.** A turn that reaches nobody — stance
     `off`, or a conversation past `MAX_EXCHANGES` — comes back as a job
     already `done`, and the client hands it straight to `followJob` as
     `initial`, which resolves without a single poll. Only a turn that actually
     calls Anthropic pays the 400ms poll.
   - **`key=None`, and that is the opposite of the dossier's call.**
     `jobs.submit(key=…)` collapses concurrent duplicates, which is right when
     two clicks inside four minutes are one question asked twice. A transcript
     is client-held, so two turns in flight are two *conversations*, and
     joining them would hand one of them the other's question.
   - **The route had no tests, which is how the dossier shipped and was very
     nearly how this did.** The proposal route had five; `/api/claude/theme`
     had zero, and all 259 tests matching "theme" passed against a module. Nine
     now cover the HTTP surface. Worth recording separately: the first draft of
     the born-finished test **passed against a mutation that removed the
     short-circuit**, because it asserted `status == "done"` on the response
     and a queued job satisfies that whenever the worker wins the race — which
     it always does when the work makes no call. The honest seam is
     `jobs.submit` never being reached at all, and the `no_worker` fixture is
     that. The identical weakness in the *proposal's* equivalent test was
     inherited and is fixed with it.

   Three things are already settled and should not be re-opened:

   - **It proposes; the user creates.** Nothing under `src/mtglab/claude/` can
     reach a write path, and `create_deck` is on the write surface
     `tests/test_claude_boundary.py` forbids naming. So the interview's output
     is a *proposal* — colours, then commanders — and the existing create flow
     is what makes a deck. That is the same shape the rationale interview has,
     arrived at from the other direction, and it is a feature: the deck is
     made by the person whose deck it is.
   - **Every commander it names comes from the pool.** The theme half is
     opinion and is exactly what Claude is for; the moment it starts naming
     cards, rule 1 binds. `search_cards` with `commanders_only` and an identity
     filter is the tool, and a name that does not resolve gets dropped and
     counted — the instrument the dossier's rivals already use.
   - **A theme is not a `why`.** Asking what historical period somebody relates
     to engages no part of rule 4, and the mode still may not pre-fill a
     rationale for any card it suggests.

   **Conversational, not one-shot** — decided 2026-08-12. A multi-turn
   interview that adapts to the answers, rather than a form of fixed questions
   followed by a proposal. It is the more expensive thing to build and it is
   the reason the feature is interesting: a form could have been a form
   without a language model in it.

   That choice is what makes this mode genuinely new rather than the rationale
   interview with different words, and it is the part the ADR has to think
   about hardest. Three consequences that are not obvious:

   - **The interview holds state across turns, and `converse` currently does
     not.** Every mode so far is one question and one answer; this one is a
     conversation whose history has to survive between HTTP requests. Where
     that history lives — client-held and resent, or server-side and keyed —
     is a real decision with a cost either way, and it is the first thing to
     settle.
   - **A multi-turn mode has no natural stopping point**, which is exactly
     where `MAX_TOOL_TURNS` came from for the single-shot ones. It needs a
     ceiling that is about the *conversation*, not the tool loop, and a way
     to say "I have enough to propose now" that is checkable in Python rather
     than trusted from the model.
   - **The proposal is a schema, the conversation is prose.** Those want
     different response shapes, so a mode that does both is either two modes
     or one mode with a mode switch — and ADR 15 says a mode is a prompt, a
     tool set and a capability declaration, so two is probably the honest
     answer.

   **The refactor pass stays out**, confirmed 2026-08-12, and the reason is now
   a design one rather than only sequencing: it is a *critique* surface over an
   existing deck, and this branch's whole argument is that the deckbuilding
   surface must not reach a deck. Shipping both together would blur the
   boundary on the branch that draws it. It also still inherits the rationale
   interview's answer to who writes the `why`, and the pod measurement still
   decides whether Forge can contribute to it honestly. Goal 10.

   **Also in scope, and cheap:** the rationale interview was undiscoverable —
   it worked, and nothing on the deck page said so. *Done:* every card carries
   an **Ask Claude** control beside *Write why*, which opens the editor already
   asking rather than revealing a second button that asks; the cards tab says
   the feature exists and states rule 4 in the same breath; and all of it is
   honestly absent when the surface is off, unconfigured or uninstalled.

   Two things from the cleanup pass are worth knowing while working through
   it: the four-colour names come from `colors.py`'s taxonomy (Artifice,
   Chaos, Aggression, Altruism, Growth) and any new copy of that table has to
   agree with it, and the six non-landing routes are lazy, so a new screen
   wants a `React.lazy` line rather than a top-level import.

   **Added 2026-08-13, from play-testing branch 5.**
   Eleven more from the maintainer, unprompted, after he drove the theme
   interview. **None of it is started, none of it is scheduled**, and it does
   not displace branches 3 and 4 — it is written here rather than left in a
   conversation because that is the whole point of this file. Three of them
   already have a home elsewhere and say so.

   *Deckbuilding surface:*

   - **An opening-hand randomiser/visualiser for a built deck** — "pretty
     standard and a fun addition to help people get a feel for opening hands."
     Bonus: randomise which printing's art each card shows. Further bonus: a
     **mulligan-confidence suggestion**, which is the part to be careful with —
     confidence about a keep is a claim, and Tier 1 is the only thing here that
     could back one. Either put a real simulation behind it or do not call it
     confidence.
   - **Two-sided cards show one face.** Scryfall renders a small flip control;
     this app does not, anywhere. The most concrete item in the list — the
     pool already carries both faces, and `CardRecord.front_type_line` exists
     because the commander dossier already had to care.
   - ~~**"Entomb" as the delete button's label for commanders.**~~ **Done
     2026-08-15, and it grew into
     [ADR 27](adr/0027-entomb-is-the-delete-and-the-graveyard-is-the-undo.md)**
     after a live drive showed the card-level delete firing on one unconfirmed
     click — a handful of Gyome's cards died in an afternoon, unrecoverable on
     the instance where deck edits have no git history. Every delete label is
     Entomb now, red, and armed (first click cocks the button, second acts,
     four seconds disarms); a removed 99-card goes to a `graveyard:` list in
     `deck.yaml` with its `why` intact, with **Return** and **Exile** as the
     two ways out and a bulk sweep that is one all-or-nothing write. The typed
     confirmation for a whole deck **stays `bury`**, as he confirmed. The
     card-row buttons also stopped looking disabled (`card-action` classes —
     hover states, which inline styles could never say).

   *Content depth:*

   - ~~**The guild, clan and shard descriptions are bland**, at the macro level
     too — "the guilds of Ravnica are pretty famous. We can do better."~~
     **Done in branch 4**, which is what it turned out to be the brief for:
     every faction has what happened to it, two or three named champions with
     their real cards, and the cards that are exactly its colours.
   - ~~**Lore rivals on the commander dossier.**~~ **Done 2026-08-15**, after
     the first Gyome dossier showed why it mattered: the single "rivals" list
     answered the deckbuilding question while wearing the story's name, and
     the `who` section — the only one whose prompt gave no guidance at all —
     regressed to a mechanical description. The split: **Competitors** is the
     old list (pool-resolved cards, `get_cards` or dropped, unchanged
     machinery), **Rivals** is the story's — cited prose like `who` and
     `standing`, because a plot line is not a pool row, with the prompt
     explicit that a minor character honestly has none rather than an invented
     feud. `who` got its brief back: character first, mechanics belong to
     archetype. `DOSSIER_VERSION` bumped to 2, and the prompt fingerprint in
     the cache key means every stored dossier regenerates on next request —
     one call per commander, which is the price of the fix reaching Gyome.
   - **Searchable infinite combos**, linked to a deck's wincons or its
     breakdown — "that is good info to know and I think there are websites
     devoted to it." There are, and **the no-crawler rule is what shapes this**:
     hosted web search per question, or a small hand-curated set of our own.
     Not an ingest of somebody's combo database.

   *Storage:*

   - **"Are we ready for multi-user deck storage? Seems like we just throw
     things in `/decks`."** Not yet, and the answer is already designed: that
     is `user_decks`, [docs/HOSTING.md](HOSTING.md) §6 step 6, and
     `decks/source.py`'s `DeckSource` protocol plus `api/deps.py` exist so it
     is one dependency to swap rather than thirteen handlers to edit.

   *The theme interview, which is where most of his thinking went:*

   - ~~**Personas instead of a fixed question battery.**~~ **Started
     2026-08-13, [ADR 21](adr/0021-a-persona-is-a-voice-and-the-spread-is-the-slots.md).**
     The complaint turned out to be half right and the half that was wrong is
     the useful part: `SLOT_KINDS` is a taxonomy handed to the model, not a
     script, so every sentence anybody reads was already generated. What was
     fixed was the *register*. So a **persona** is a voice and explicitly not a
     fourth stance axis — stance is how much the model does, persona is who it
     sounds like — and the voice is *appended* to the interview's instructions
     rather than replacing them, so its rules stay out of a persona's reach.
     `plain` and `fortune-teller` were built first; **the costumed five
     followed on 2026-08-15** — therapist, scientist, chef, storyteller,
     barkeep — and ADR 21's "a `Persona` and a prompt with nothing else to
     move" held exactly: the client changes for that PR were tiles and art,
     not plumbing. The same change merged the theme and tarot doors into one
     persona-grid door ("Help me decide"), so the reader picker is now the
     first thing the create flow's Claude door shows and the fortune-teller
     tile is where "Read my cards" went.
   - ~~**A tarot reading as a door of its own.**~~ **Built 2026-08-13** —
     backend first, then the door: a fourth entry on "Start a deck" that picks
     a reader from the roster, shuffles, deals three cards face down, and turns
     them over. The decision that made it possible rather
     than a rewrite: **the spread's three positions are `SLOT_KINDS[:3]`**, so
     a card is dealt *for* a slot and ADR 20's grounded-quote readiness works
     untouched — a card is not something the querent said, and the cards colour
     the questions rather than replacing the evidence. `tarot.py` holds all 78
     cards and no card's meaning; Python shuffles, the reader reads.

     **The licence was checked rather than assumed, and it matters.** The
     original 1909 Rider "Roses & Lilies" printing is public domain in both the
     US and the UK — but **US Games Systems' 1971 recolouring, which is the
     deck everybody pictures, is not.** All 78 files were verified per file
     through the Commons API; `src/mtglab/assets/tarot/PROVENANCE.md` is the
     argument. 4.6MB of WebP, shipped as package-data rather than through the
     committed bundle, which is why the `image` CI job now counts the cards.

     The door's own decisions, all small and all load-bearing. **The reader
     roster is fetched, never written in the client** — `/api/claude/personas`
     is free and needs no key, so the first screen of the most expensive door
     costs nothing, and the three unbuilt voices will appear there with no
     frontend change (a test pins that by putting a reader in the mock that
     exists nowhere in the app's source). **The card back is drawn**, for the
     reason the mana symbols are: 78 faces have a provenance file behind them
     and a back lifted off the internet would be a 79th image with no argument
     attached. **Persona is fixed for a conversation**, enforced by remounting
     the interview on its key rather than by a warning nobody reads, and a
     stash left by a different reader is discarded rather than adopted.
     **A reversal is a rotation on the `<img>`, never on the face** — the face
     is already rotated 180° about Y to hide behind the back, so putting both
     on one element makes a reversed card spend the flip un-reversing itself
     and land upright, which looks exactly like nothing going wrong. And the
     reveal gets a beat before the spread folds away, because the first version
     shrank the cards 840ms into a 1.6s stagger and resized the climax.

     Driven in a browser and screenshotted rather than described, per the
     habit that has caught eight bugs in eight branches. What it turned up:
     Vite proxies `/api` and nothing else, so package-data art 404s in dev and
     only in dev; a card back needs a hairline of light in dark mode, where a
     drop shadow against `#0d0d0d` says nothing; and "Your spread" was labelling
     an empty table for the reader who deals no cards.
   - **An interview for somebody who already has a theme.** The current one
     discovers a theme; this one would take a given theme and follow it. "That
     would be a fun alternative interview style."
   - **Claude's suggestions should go past interpretation.** Today it reads you
     and describes; he wants it advising specific commanders and **tied into
     the rest of the analysis** the tool already does. That is the item most
     likely to collide with ADR 20's "it proposes, you create" and with rule 4,
     so it wants reading against both before it is designed.

   **The register is the requirement, not decoration.** The theme interview
   exists because he rejected a first draft that asked Magic questions in a
   friendly voice; the tarot and persona ideas are that same instinct. A
   version of these that arrives sensible and dull has missed them.
4. ~~**Deploy**~~ — **done 2026-08-13. Live at https://sylvan-libraries.com.**
   [docs/HOSTING.md](HOSTING.md) is the guide and was corrected the same
   day against what actually happened (#65).
5. **Testing the live instance** — started 2026-08-13, **not finished.** Email
   is proven end to end: a real invite, a new sending domain, Gmail, the claim
   link, a sign-in. Getting there turned up two bugs that only a deployment
   could show, both fixed and deployed in #66 — Cloudflare in front of
   `api.resend.com` refusing Python's default User-Agent, and the image not
   carrying the Anthropic SDK while the instance held the key.

   **Both lived below a test seam**, which is the durable lesson: mail is faked
   through `Transport` and the SDK is stubbed in every Claude test, so the
   whole suite passed while neither worked. `tests/test_packaging.py` is the
   first check that reads the *image* rather than the code.

   What is left: driving the app itself — the Learn page, the theme interview,
   the dossier, a deck edit surviving a restart. **The Claude surfaces became
   reachable on 2026-08-13** (`mtglab claude check` answers `pipe open` on the
   machine). Since 2026-08-16 the driving no longer stops at the login page
   either: the `claude` account is a live-testing seat Aaron signs in and
   Claude rides, so an authenticated flow on the deployed instance is now a
   thing a session can do rather than a thing it can only ask for. See
   `docs/ENGINEERING.md`, "Mobile engines, and the live seat".

   Two punchlist items came out of the first real claim, both now built:
   choosing your own username at sign-up (#67), and **deleting an account**,
   which the first one turned up rather than predicted. `disable` was the whole
   revocation story and it does not release anything: `username` and `email` are
   `UNIQUE`, so a disabled row keeps both and an address cannot be invited twice.
   `users.delete` is the third door to a lockout and carries the same
   `LastAdmin` guard as `disable` and `demote`, with nothing to walk it back.

   The find worth keeping is not the feature. `users.id` is `INTEGER PRIMARY
   KEY` **without `AUTOINCREMENT`, so SQLite reissues a deleted account's rowid**
   — and jobs are held in memory keyed on exactly that integer. Delete the
   newest account, invite a replacement, and the new holder of the id inherits
   the dead account's jobs. `jobs.forget_owner` is the fix, and the class is
   worth naming: an isolation filter that is written correctly and defeated by
   arithmetic underneath it. Anything future keyed on a user id inherits the
   same trap; the alternative considered and not taken was a migration to
   `AUTOINCREMENT`.

   **The reset path is now proven end to end too** — a real reset mail, Gmail,
   the link, a new password, sessions revoked — but proving it turned up a third
   deployment-only failure. **A mail app can drop the URL fragment when you
   click.** The message left the server whole and the *visible* URL was whole;
   the click arrived at `/auth/claim` with an empty hash, which the server
   cannot see, because keeping the token out of every access log is exactly what
   the fragment is for (ADR 16).

   That made the failure terminal rather than annoying: a stripped link is
   indistinguishable from no link, so **"ask for a new one" produces one that
   fails identically, forever.** The fix is a paste field on the claim screen
   and a sentence in both messages pointing at it. The token still never rides
   in a URL — a paste is read on the client and posted in the body, which is
   the same rule the fragment was serving rather than an exception to it.

   The pattern across all three is worth naming: **every one lived below a seam
   the suite cannot reach** — a faked mail `Transport`, a stubbed SDK, and now a
   mail client's linkifier, which no test anywhere can exercise. What each
   needed was somebody using the thing for real.

   Also of note: the instance's host went unreachable for several minutes that
   day (machine suspended, volume on the same host, `no snapshots available`)
   and came back intact. **One machine, one volume, no snapshot** is the shape
   that was exposed — worth a decision before it matters.

   **The evening of 2026-08-13 put the first Claude output on the instance.**
   A dossier ran there — `claude-sonnet-5`, 77 pages searched, 5 cited, ~236
   seconds — and finding that out cost two more deployment-only bugs, merged as
   [#71](https://github.com/aasquier/sylvan-library/pull/71) and
   [#72](https://github.com/aasquier/sylvan-library/pull/72): a synchronous POST
   nothing had re-measured since ADR 20, and a bundle a browser cached
   heuristically and then black-screened on. Six dossiers are cached now;
   the reattach-after-reload contract is proven in Safari against the access
   log, which is the instrument for it — what you are looking for is one job id
   continuing across a document request, and a screenshot cannot show that.

   **A fourth deployment-only fault, found the same way, 2026-08-13.** Hours
   after the tarot art shipped, every one of the 78 pictures was being served
   as `application/octet-stream` — on the instance and only there. Starlette
   asks `mimetypes`, `mimetypes` asks the operating system, and the slim image
   has no `/etc/mime.types`; macOS and CI's ubuntu both know `.webp`, so no
   local check could see it, and browsers sniff and render anyway, so no remote
   *page* could either. It took reading the response headers. `api/app.py`
   names the type itself now and the `image` job asks the container.

   The shape is the point and it is now four for four: a faked mail
   `Transport`, a stubbed SDK, a mail client's linkifier, and the host's mime
   database. **Every one is a fact about the environment rather than about the
   code**, which is precisely what a test seam stands in for.

   ### The punchlist, 2026-08-13

   Five items, written down here rather than left in a session's memory. The
   first is fixed on the branch that carries this paragraph; the rest are open.

   1. ~~**No in-flight dedupe on the dossier.**~~ **Fixed 2026-08-13.**
      `plan_dossier` answered a *stored* dossier as a job born done, but nothing
      checked for a run already going for that `oracle_id` — so a second click
      inside the four-minute window started a second paid job with a second web
      search, and two ran concurrently on the instance that day. `Plan.key` and
      `jobs.submit(key=…)` are the fix: the lookup and the insert are one locked
      step, matching is per owner as well as per key (ADR 5 — two accounts
      sharing an id would give the second a 404 for a job it had just been
      handed), and only a *live* job is joined, because a finished one is what
      the cache is for and a failed one has to stay retryable. **This is the
      robust half of the reattach story** — the localStorage id only ever
      covered one tab; this covers a reload, a second tab, another device and a
      cleared cache, because the server is the thing that knows.
   2. ~~**Learn/Vocabulary renders `long` only.**~~ **Fixed 2026-08-13**, by
      rendering `short` as a lead line — the maintainer's call between that and
      rewriting the two offending paragraphs.

      **The count in the original note was wrong, and it is what decided it.**
      "35 of the 37 longs stand alone" does not survive reading all 37: around a
      third open as sentence *two*. The entire `stat.*` block does it as a house
      style — "The tail the median hides", "The cost of flooding, made into a
      number", "The sweep's answer, and only as good as the spread it sits in" —
      each commenting on a measure that only `short` ever names. **Commander
      tax** and **Mana base** were not two exceptions; they were the two the
      maintainer happened to open. So rewriting them would have fixed two
      symptoms and left a dozen, and left the next entry free to acquire the
      same defect.

      The rendering fix also closes a smaller hole: the search box at `:395` has
      always matched `short`, so it was possible to find an entry by text the
      page then refused to display. `glossary.py`'s docstring now states the
      contract the data always had — definition in `short`, argument in `long`,
      the page renders both in that order — so a new entry inherits it.
   3. **iOS Safari private tab lost the dossier reattach.** Unexplained — zero
      polls after the reload, while the theme key survived a reload in that same
      tab and planting a job id locally proved the read half works. P3, and (1)
      covers it in practice.
   4. **Fly volume snapshots still unconfirmed.** Volume created 13 Aug 14:11
      UTC with `snapshot_retention: 5` and `Scheduled snapshots: true`; the list
      was empty at 19:45Z. **Re-check any time after 14:11 UTC on 2026-08-14** —
      before that an empty list proves nothing, which is the same trap as the
      morning of the 13th.
   5. ~~**`users.id` → `AUTOINCREMENT`** migration still owed.~~ **Done
      2026-08-13**, as schema version 5. #68 fixed the instance;
      `jobs.forget_owner` covers the one caller that exists today, and this
      closes the class, so the next thing keyed on a user id does not have to
      rediscover it.

      AUTOINCREMENT cannot be added by `ALTER TABLE`, so it is SQLite's
      documented table rebuild — and **the rebuild is more dangerous than the
      thing it fixes.** `sessions` and `auth_tokens` reference `users`
      `ON DELETE CASCADE`, and `connect()` turns `foreign_keys` on *before* the
      ladder runs, so the obvious migration signs every account out and voids
      every unspent invite. Silently: a cascade is not an error. On the instance
      as it stands that is five live sessions and one outstanding invite.

      So `_apply_migrations` now turns the pragma off around the whole ladder,
      runs `PRAGMA foreign_key_check` before giving it back, and raises
      `MigrationFailed` rather than serving requests on a file that did not pass
      — the pragma is a no-op inside a transaction, which is why it cannot live
      in the migration script that needs it. The migration carries its own
      `BEGIN`/`COMMIT` for a second reason: `executescript` performs no
      transaction control, so a failure between the `DROP` and the `RENAME`
      would leave a half-built table and a version still at 4, and the next
      start would fail on a table that already exists — an app that never boots.

      Worth recording that the first draft of the *test* destroyed the rows it
      was written to protect, in its own setup, for exactly this reason.

      What it does not do is repair ids already handed out: the high-water mark
      is `max(id)` over rows that exist, and a deleted id is not one of them. It
      stops the next reissue, not the last one.

   6. **The curated library was writable by every invited account.** Found
      2026-08-14, minutes after the first non-admin claimed their invite and
      became a real second person on the instance. `deps.deck_source` handed
      the same `FileDeckSource()` to everybody and `FileDeckSource.writable`
      was hardcoded `True`, so `mitch` could swap cards in, or delete, any of
      the curated six. Recoverable — a delete moves the directory to `.trash/`
      and the decks are in git — but an edit to `/data/decks` is the live
      source of truth and nothing else records it.

      **Nothing was wrong with the classification; it was answering a
      different question.** `tests/test_isolation.py` files every deck route as
      *shared*, with reasons like "edits a shared deck", and that is correct
      about **reading**. Before there was a second account, "everyone sees the
      same decks" and "everyone may edit them" were indistinguishable
      statements. An invite made them different and nothing in the suite was
      watching the seam: the read-only path *existed* in `service.py` and was
      dead code, because no source ever reported `writable=False`.

      Fixed by deriving writability from the caller in the one place that
      already decides what a request may see, and by collapsing four bespoke
      refusals — `EditRejected`, `CreateRejected`, `DeleteRejected`,
      `ImportRejected`, each chosen to match whatever its own route caught —
      into `ReadOnlySource`, handled once as a **403**. It answered 422 before,
      which was defensible while read-only was a property of the *source* and
      is not once it is a property of the *caller*: nothing is wrong with the
      request except the person making it. No client had ever seen the 422,
      since the path was unreachable.

      Two things worth carrying. **The test that would have caught it is the
      one nobody writes**: `test_api.py` covered read-only sources through
      `dependency_overrides`, proving what a route does *when handed* one, and
      never that anybody is handed one. Same shape as the dossier's missing
      HTTP tests. And **`/api/decks` is no longer byte-identical per caller** —
      it carries `writable`, which is about the viewer — so the "shared really
      is shared" assertion had to get sharper rather than looser: every field
      but that one is identical, and that one must actually differ.

      **The consequence is deliberate and is not the end state.** There is
      nowhere for a non-admin's decks to live, so the app is read-only for
      them, the three "start a deck" doors included. That is the correct
      interim answer — the alternative is their decks landing in somebody
      else's library — and it is the argument for doing the ownership tier
      next rather than eventually.

   7. **Deck ownership and sharing — built, both halves
      ([ADR 22](adr/0022-decks-have-owners-and-sharing-is-a-flag.md)), and
      not yet exercised on the instance.** Asked for 2026-08-14: people should be able to
      show each other their decks, the maintainer's should always be visible,
      it should be a tab somebody opts into rather than something in the way,
      and other players' decks should be organised **by username**. Leaderboards
      and macro deck stats are named as later work on top of it.

      **Decided, on the branch `deck-ownership-and-sharing`:**

      - **Paths are owner-qualified — `/api/decks/{owner}/{slug}`.** Slugs are
        unique *per owner*, never globally, which is what stops "is this slug
        free" from being a question about everybody's private decks at once. A
        global namespace was rejected for exactly that leak; an opaque deck id
        was rejected for breaking the slug/directory correspondence ADR 1 keeps
        permanently.
      - **Sharing is a per-deck flag.** Curated six shared by default (absent
        means shared, so they are never silently hidden); a deck in the SQL
        tier is **private** by default, because `decks import` writes 99 empty
        `why` fields and publishing that instantly is nobody's intent.
      - **The 403/404 split resolves as one sentence: *403 is only ever an
        answer about a deck the caller can already read.*** A private deck is
        absent from the source, so every verb answers 404 — writes included,
        because a 403 there confirms it exists. A shared deck answers 403 to a
        write, which is item 6's answer unchanged.
      - **The file tier's owner is a rule, not a column.** `MTGLAB_ADMIN_EMAIL`
        names them; unset, the six fall back to `local` and stay visible, since
        the alternative is an instance whose showcase nobody owns and therefore
        nobody sees.

      **What that cost, and what it caught.** Two bugs the sweep found rather
      than the design: the sim routes take their slug in the *payload*, so they
      resolved a deck by name with nobody asked whose it was; and
      `_for_writing` refused on writability before resolving the deck, so a
      write to somebody's private deck answered 403 and confirmed it. Both are
      the same shape as item 6 — a check that was correct while there was one
      library. `tests/test_isolation.py` files every per-deck route as
      **user-scoped** now, with ten new adversarial tests.

      **The browser half, and the two things it needed that the server half did
      not have.** Every deck call takes a `DeckRef` — `{owner, slug}` as an
      object rather than two positional strings, because transposing two
      strings is a runtime 404 against somebody else's library and named fields
      make it a compile error. `deckUrl` is the single place an in-app deck link
      is built, and `lib/api.test.ts` asserts the **URL shape** directly:
      a screen mocking `api` passes its tests while the real client asks for a
      route that no longer exists, which is precisely the failure this half
      exists to prevent.

      - **`GET /api/decks` gained one field, `showcase`.** The browse tab needs
        three groups out of one flat list — yours, the showcase, everybody
        else's — and could only work out two. `writable` identifies the
        caller's own decks; *nothing* identified the maintainer's, because the
        client is never told who that is. Inferring it from the response's
        order was the alternative, and ordering is not a contract.
      - **`/decks/:slug` survives as a resolver, not as a deck route.** That
        was every deck's address for the life of the app and the instance has
        been driven for days, so a bookmark or a link sent to a friend still
        works: it looks the slug up and redirects, first match winning, which
        is your own deck before the showcase before a stranger's because that
        is the order the library is listed in.
      - **The authoring doors are no longer gated on `is_admin`.** That gate
        said it would disappear rather than move when decks got owners, and it
        did — everybody has a library to put a deck in now.
      - **The sharing toggle is the deck page's, owner-only.** Without it a
        SQL-tier deck is private forever and the browse tab can never have
        anything in it.

      **Owed: exercising it on the instance.** A non-admin account driving the
      write gate, ADR 5's 404 and ADR 17's 403 against the deployed app, which
      is where every fault in this project has actually lived.

   Still owed from the test list itself: **the theme interview on the
   instance** (both modes, and now both readers) — the deployed React half,
   specifically; the environment itself is proven.

   **Done since:** a deck edit surviving a machine restart (2026-08-13), and
   **delete → re-invite → claim with a chosen username**, completed
   2026-08-14T00:25Z. The claim is worth a sentence because it was open for
   three sessions on a wrong theory: a stripped URL fragment was suspected, and
   `POST /api/auth/claim/preview` answering 200 disproved it — that call reads
   the token and spends nothing, so its success means the fragment had been
   arriving the whole time and **the claim had simply never been attempted.**
   An absent request is the proof of a stripped link *and* of a thing nobody
   did; the log cannot tell them apart, and only a live tail during a real
   attempt could.
6. ~~**A second quality pass.**~~ Landed 2026-08-14. Not a feature branch —
   vocabulary, documentation, lint reach and workflow hygiene, done while the
   instance was already live.

   - **"Corpus" is now "the card pool"**, across 1,016 occurrences in 106
     files: prose, comments, identifiers, UI strings, the pytest marker, and
     the `/api/health` and deck-response **wire fields**. The word came from
     linguistics rather than from Magic, and "card pool" is a term the game
     already uses for the set of cards you may build from. **ADRs 2 and 7 keep
     the old word** — they are records of what was decided and how it was said,
     and `docs/adr/README.md` carries a note saying both names mean one thing.
     One place resisted the rename and was more interesting than the rest:
     `tests/mana_oracle.py` used "corpus" for its enumerated set of
     differential test cases, a different sense entirely, and the file already
     used `pool` for a pool of mana sources — a blind sweep produced
     `pool_pools()`. Those are `all_cases()` and a **case set** now.
   - **The README is a README**, not a tutorial: 297 lines to 180, leading with
     the idea rather than with `pip install`. The setup path, the full command
     reference and the deck workflow moved to `CONTRIBUTING.md`, which is where
     somebody who has decided to use the thing actually looks.
   - **Stale claims across the docs**, nearly all of one kind: they were
     written before the deploy and still said so. The ADR index stopped at 20
     with 21 and 22 on disk, described 14 and 15's modes and stances as
     unbuilt, and called 4 and 5 decisions about "a deployment that does not
     exist". HOSTING §7 framed itself as a pre-deploy checklist. Three
     documents disagreed about the size of the mypy exemption list; two said
     "ten", `pyproject.toml` said eight, and eight was right.
   - **Ruff's rule set widened** to add C4, RET, PTH, TID, PIE and RUF, chosen
     by measuring each against the tree rather than by taste — three reported
     nothing at all. RUF earns its place on RUF100 alone: **71 `# noqa`
     directives were suppressing rules that no longer fire**, and a dead
     suppression reads exactly like a live one.
   - **Workflow hygiene**: `permissions: contents: read`, a `concurrency` group,
     `timeout-minutes` on all four jobs, and pip caching. See ENGINEERING §5.
   - **Five new tests** on the three background-job error translations, which
     had none. That is the code deciding whether an expired key reads as "your
     key may have expired" or as a stack trace in a job's error field — and the
     key has a fixed lifetime, so it is a question of when. Verified by
     mutation, not by going green: stripping `explain()` fails all three.
   - **Deferred, with the measurement written down:** `noUncheckedIndexedAccess`
     (51 errors across 15 files, ENGINEERING §4) and `ruff format` (101 of 111
     files, ~15,000 lines, and it would fight the deliberate argparse table).
     Both are their own change for the same reason `strict` was.

     **Both decided 2026-08-14.** `ruff format` is a **no**, recorded as
     [ADR 24](adr/0024-no-python-autoformatter.md) — the first rejection in
     the directory, because "why is there no formatter?" is a question that will
     be asked again and an answer nobody wrote down gets relitigated. The
     deciding measurement was not the diff size but the line-length one: 117
     lines of 39,823 over 88 characters, and 60 of the 61 over 100 are oracle
     text in `tests/tiny_pool.py` that a formatter cannot split. The discipline
     it would impose is already there. `noUncheckedIndexedAccess` is a **yes**,
     on its own branch — re-measuring found the 51 errors cluster into ~15
     distinct sites, several of whose fixes are strictly better code.

     **Both landed 2026-08-14.** The flag is on, all 51 are fixed, and no
     non-null assertion was added under `web/src` outside test files. The one
     finding worth carrying: **a tuple type does not satisfy the flag** — only
     a literal index escapes it, so `WUBRG[(i + 1) % 5]` is `string | undefined`
     whether `WUBRG` is an array or a five-element tuple. The pentagram's edge
     list is built by walking a rotated copy in lockstep instead.

7. **After that, next build work in order:** ~~re-price automated PR review~~
   (done 2026-08-14 — **still parked**, and now for a measured reason: 87 PRs
   in five days is 17.4 a day, and a Sonnet 5 review of a median PR costs $0.50,
   so **$262/month against the $10/month Copilot Pro already rejected on
   price**. Not close, and not fixable by model choice. ENGINEERING §5 has the
   table.), the stance dial UI, then the remaining Claude modes ADR 15 names and
   branch 5 does not build (argue a slot, deck conversation, research).

   The re-price's **useful** finding was incidental to it: `web_dist/` is **75%
   of the median review's input and 88% of the worst** — PR #87 is 305,735
   tokens whole and 15,216 without the bundle. That is a bill being paid today,
   because `/code-review ultra` is billed and does get run, and #81's 865,448
   tokens is close enough to the 1M window to lose the diff. Exclude the bundle
   from any review diff.

   **The stance dial landed 2026-08-14** and was a prerequisite rather than a
   peer, which only became clear once the code was read: ADR 15 gives **deck
   conversation** its reversible edits *"at the top stance only"*, and until
   this no client could ask for a stance at all — every surface sent none and
   took the deck-derived default, which caps at `SECOND_OPINION` (`write:
   none`). That mode's defining capability was unreachable. It is reachable
   now.

   Building it found a bug forty-two tests had missed, and the shape of the
   miss is the transferable part. The create flow has no deck, so
   `/api/claude` resolved through `stance.resolve(None, None)` and answered
   `off` — while `theme.stance_for` was about to run that conversation at
   `second-opinion`. Every test of that endpoint passed, because every one of
   them named a deck; the case with no deck was the case nobody wrote.
   **Rendering a value is what audits it.** The number had been served since
   ADR 20 and nothing had ever had cause to look at it, and it took putting it
   on screen next to the thing it describes. `/api/claude` takes a `surface`
   now, and each surface's default is asked of the module that owns it.

   **Argue a slot landed 2026-08-14**, and the ordering was argued rather than
   taken in table order — the reasoning is worth keeping because it corrects
   the paragraph above. The dial made `write: proposes` selectable; ADR 15's
   phrase for deck conversation is *"the reversible edits, at the top stance
   only"*, and `stance.py`'s own table maps *reversible edits* to
   `write: applies`, which **is not a preset**. `COLLABORATOR` is the top
   preset and it is `proposes`; `lib/stance.ts` pins a preset *name* and
   nothing else, so no client can express an axis. Below that sit four locks
   that each say in their own words that moving them needs a superseding ADR:
   no write tool in `tools.READ_ONLY`, `Mode.__post_init__` refusing a
   non-empty `may_write`, `test_claude_boundary.py` forbidding the *mention*
   of a write function anywhere under `src/mtglab/claude/` — including
   `remove_card` and `set_card_field`, the two ADR 15 says are
   autonomous-safe. Plus a sim-results tool that `service.py` does not have.
   **Deck conversation is the largest of the three, not the unblocked one.**

   The fifth item on that list — the activity log — landed 2026-08-16 as
   ADR 28, which leaves the four locks and the missing tool. Each lock names
   the ADR that would have to supersede it, so none of them is work; they are
   arguments somebody has to make.

   So argue a slot went first, and not only because it was cheapest. It is
   the mode nearest the boundary while the stakes are lowest: its whole output
   is declarative prose about a card's merit, which is exactly what
   `only_questions()` exists to delete from the interview, so it forced the
   question of what guards that. The answer is
   [ADR 25](adr/0025-argue-a-slot-argues-one-direction.md) — **it argues
   one direction**, and the schema has no field for the case in favour. A
   balanced version would return a finished `why` grounded in the user's own
   deck, and a UI that declined to render it would not be a guard, because the
   CLI renders the same payload and the endpoint is public.

   Three things the build added that the ADR did not need to name. The
   alternatives it offers are **bare names judged by Python** — resolved
   through the pool and dropped if invented, banned, or outside the colour
   identity, counted separately in each case — which makes the *Ajani, Nacatl
   Pariah* error in CLAUDE.md an assertion that runs on every PR rather than a
   story. Writing that filter found a real bug: a double-faced card comes back
   under its full `A // B` name, so an index keyed on the pool's spelling
   dropped every DFC named by its front face, **silently**, which is the one
   thing the function exists not to do. And the UI test for "this is the case
   against" passed against a heading relabelled *Assessment*, because it was
   matching the `never` sentence in the payload — **a test asserting the
   server's own text back at itself is not testing the renderer**, and it was
   only caught by mutating the code rather than by going green.

   **Research landed 2026-08-14** — `src/mtglab/claude/research.py`,
   `mtglab claude research "<question>"`, `POST /api/claude/research`, and a
   `/research` page of its own in the nav, with
   [ADR 26](adr/0026-research-answers-about-magic-not-about-your-deck.md)
   written first. Sixth mode, fourth feature, ADR 15's fourth table row.

   The unsolved problem this entry used to name — *it has no narrow contract to
   check against* — has an answer, and the answer turned out not to be about
   facts at all. **The contract is that the mode cannot reach a deck.** No
   `DeckSource`, no slug, no deck tool, and the route sits at
   `/api/claude/research` rather than under `/api/decks/{owner}/{slug}`. That
   does two jobs with one absence: rule 4 is out of reach because there is no
   rationale to read and no 99 to be asked what to cut from, and **deck
   conversation cannot be built by accident**, which was the real risk — the
   question "should I cut X from my deck" is what somebody types on day one,
   and answering it well is deck conversation under another name, with none of
   the five things ADR 15 still owes it settled.

   Three things the build settled that the ADR did not have to name:

   - **A card the pool lacks is labelled, not dropped, and that is the one
     place research must differ from the dossier.** ADR 19 drops an unresolved
     rival because a rival that does not exist is an error; a card *spoiled
     since the last `data refresh`* does not exist either and is one of the
     three things this surface is for. So both are kept and marked
     `in_pool: false`, counted separately from the dropped counts because that
     number is not a fault. Applying the dossier's instrument here would have
     made the mode silently worst at its own best use.
   - **A finding whose citations all failed the check is dropped, not
     narrowed.** One step past what `dossier._section` does, and the reason is
     that a dossier passage may rest on the brief it was handed while research
     has no brief — its subject is whatever was asked, so `get_cards` is the
     only pool door rather than a second one.
   - **265 seconds, measured on the first real question.** Longer than the
     dossier's 236s, which is the duration that broke deployed. It was a
     background job from its first commit rather than after an incident —
     the first Claude surface here of which that is true — and the route had
     tests before it had a deploy, which is the other half of that lesson.

   What is left of item 7 is one mode: **deck conversation**, and it is now
   *harder* to build rather than closer, deliberately. Anything that wants a
   deck inside a Claude surface has to supersede ADR 26 and say what it does
   about the five things listed under the stance dial above.

8. **An efficiency pass against a stated load** — landed 2026-08-14. The
   target was named rather than assumed: 100 accounts, 10 concurrent, one
   `shared-cpu-1x` machine. Measured first, on the real pool and the real six
   decks, and the findings were not where intuition pointed:

   - **PyYAML was the shelf.** `yaml.safe_load` takes the pure-Python path
     even with libyaml compiled in — the C loader is opt-in per call — so each
     deck file cost ~36ms to parse and the shelf spent more time in YAML than
     in DuckDB. `model.load_yaml` is the one entry point now, `edit.py`
     included: 36ms → 7ms per deck, the shelf 430ms → 245ms, the deck page
     228ms → 124ms, with a pure-Python fallback where libyaml is absent.
   - **"Nothing on the wire compressed" — half wrong, and a skipped deploy
     proved it.** The premise was that Fly's proxy passes bodies through; a
     flaky frontend test skipped the deploy, which left the old code live long
     enough to catch it answering `Content-Encoding: gzip` already — Fly's
     edge compresses on its own, undocumented. `GZipMiddleware` stays for what
     was measured once the deploy landed: the app's level-9 gzip puts the
     bundle at 84.5 kB where the edge's compressor sent 119.6 kB, the
     behaviour is owned rather than inherited from one host, and `mtglab ui`
     on a laptop has no edge. Registered *innermost* because `minimum_size`
     reads Content-Length and the decorator-style middlewares re-wrap every
     response as a stream without one — registered outermost it compressed
     two-byte job polls. Two tests pin both sides, verified by mutation. The
     flaky test read the tarot stash the instant 'The Root' rendered, before
     the writing effect flushed, and waits now.
   - **The session lookup could block the event loop.** `sessions.lookup`
     writes — the five-minute `last_seen_at` touch, the delete of an expired
     row — and a write that finds the file locked waits up to `busy_timeout`,
     five seconds, on the loop, stalling every request in flight rather than
     this one. It runs in the threadpool now. The docstring that kept it
     inline priced the hop against the read alone, which is the "it is a few
     seconds" shape at a smaller scale.
   - **`jobs.MAX_JOBS` was sized for a laptop.** Fifty global slots shared by
     100 accounts evicts a finished job somebody's tab is still polling —
     cache hits are born finished, so ordinary use fills the registry with
     exactly the jobs eviction takes first. 200 now.
   - **Measured and deliberately left alone:** the per-request DuckDB connect
     (~15ms, but holding one open would lock `mtglab data refresh` out of the
     volume for the life of the process — the transient handle is what keeps
     that workflow possible), and `get_cards`' query shape (an array-bind
     variant saved nothing; the ~200ms shelf union is the scan itself, and
     the 2026-08-14 CTE rewrite already took the cheap half).

9. **Coverage to ~96%, floor to 95** — landed 2026-08-14. The suite stood at
   90% (both CI and local; the old two-point gap between them is gone) and the
   floor had been 90 since 2026-08-12. A deliberate pass took it to ~96 — the
   ground gained is itemised in `pyproject.toml`'s `fail_under` comment, and
   the largest single piece was the CLI's three Claude renderers, which had
   *no* output tests at all: the argue, dossier and research printing is where
   ADR 14's "say which system answered" lives in a terminal, and none of it
   was pinned. Also new: the theme modes' full call path faked at `Turn`, the
   Forge run faked at `subprocess` (seat mapping, the dropped-card refusal,
   the no-games refusal), the Scryfall ingest against fake bulk files, and
   ADR 22's `SqlDeckSource` exercised directly — create/read/update/delete,
   the freed slug, and the private-is-absent rule, which had only ever been
   tested through the routes. The floor sits a point under the suite so a
   change that costs a full point is loud and ordinary churn is not.

10. **A shore-up pass** — landed 2026-08-15. A full audit first (every screen
    driven in the browser, desktop and phone, both themes; suites, ruff, mypy
    all green; no console errors anywhere), then the gaps it found:

    - **The page nameplates.** The library's masthead — a whole painting at
      its own ratio beside the title, credited — was the app's best screen
      and four screens were plain grey next to it. `PageMasthead` in
      `components/ui.tsx` is that layout made shared (the library uses it
      too now), and Card search, Simulator, Research and Import each carry a
      painting from the **Strixhaven Mystical Archive** cycle — an archive
      of the game's definitive spells, for an app named after a library:
      *Demonic Tutor* (search your library for a card), *Strategic Planning*
      (look at the top three, keep what the plan needs), *Compulsive
      Research* (draw three, keep what survives scrutiny), *Cultivate*
      (search for two, keep both). All hotlinked and credited, chosen by
      printing id resolved through the pool, none committed — the branch-3
      decision unchanged, just applied to more screens. Each attribution
      clause was checked against the card's oracle text, because rule 1
      does not stop applying when the card fact is in a caption.
    - **CodeQL** (`codeql.yml`) — see ENGINEERING §5. The one scanner that
      reads the source; free on a public repo; not a required check until
      its signal-to-noise has been watched for a few weeks.
    - **A preconnect to `cards.scryfall.io`** in `index.html` — every screen
      hotlinks card art from that one host, and the handshake now happens
      while the bundle parses instead of in front of the first painting.
    - **Stale doc claims**: ENGINEERING §5 still said the coverage floor was
      90 (it is 95, item 9) and the mypy exemption list was eight (it is
      two, #90); CONTRIBUTING said five required checks (six) and described
      a local-vs-CI coverage gap that item 9 closed.
    - **`web/README.md`** — the frontend conventions map for a fresh
      session: what serves what, the load-bearing conventions (lazy routes,
      `DeckRef`, the stance readout, `hasGlyph`, glossary keys, the
      masthead rules, both-themes), and the testing habits with a history
      behind them.

11. **A Claude cost pass** — landed 2026-08-15, from an audit of the client
    against current API guidance. The audit's headline was that the
    integration was already clean (no deprecated parameters, `pause_turn`
    resumed, refusals handled, the system-block cache breakpoint in place);
    what it found were the two things below, built together:

    - **The tool loop now caches its own history.** `converse` kept one
      breakpoint, on the system block — so turn six of a dossier re-bought
      turns one through five, search results included, at full input price.
      A second, *moving* marker now rides the newest tool-result block each
      turn (moved, not accumulated: the API allows four markers and the
      theme flow already spends one inside `messages`). Cache reads bill at
      ~a tenth of the input rate, so this is the searching modes' largest
      single saving.
    - **A usage ledger** (`claude/ledger.py`, `claude_usage` in app.db,
      schema v7). Every mode counted its tokens and the CLI printed them,
      but the hosted instance — where the spending happens — discarded them
      with the job payload, so "what did this month's dossiers cost" had no
      answer. `converse` now records every conversation on every way out
      (answer, refusal, and the turn-ceiling exception, whose burned tokens
      are exactly the ones worth seeing); `mtglab claude usage` is the
      roll-up, per mode, most expensive first. Deliberately aggregate —
      counters, a mode name, a model id; no user id and no question text —
      so ADR 17's who-may-read-what argument never has to be made for it.
      Tokens and not dollars, because prices move (Sonnet 5's introductory
      rate ends 2026-08-31) and a stale price table is a wrong invoice.

    Deferred until the ledger has numbers to argue from: an effort A/B
    (`medium` on the brief-fed modes), a per-mode model field (Haiku on the
    mechanical modes via the `MTGLAB_CLAUDE_MODEL` machinery), and a
    Batch-API warm command for post-`DOSSIER_VERSION`-bump regeneration at
    half price. Rejected outright: doing our own web searches and pasting
    results into prompts — it would dismantle the `keep_sources` check
    (ADRs 19/26 verify citations against pages the search *actually
    returned*, in-band) and it is the crawler CLAUDE.md already bans, for
    savings measured in cents.

12. **The photo-real pass** — started 2026-08-15, from the maintainer's second
    punch list of that day, whose through-line was "no more clip art." The
    tooling question ("do we need third-party software for animation and
    photo-real imaging?") was asked three times in that list and is answered
    once, here, so no session re-litigates it:

    - **No third-party animation software.** Lottie and friends produce
      vector output — the exact aesthetic being evicted. The material is
      **real images**: Magic's own paintings (hotlinked from Scryfall with
      credit, the posture the app has always had) and **CC0 photography
      committed as assets** when a thing must be ours — found via Openverse
      with a licence filter, checked per file, recorded in a `PROVENANCE.md`
      per asset directory (the tarot deck's rule).
    - **The pipeline is scripted Pillow** (a dev-only dependency): fetch,
      matte, tile, measure, encode WebP. Measurements against a painting
      (the wheel's circle, the Lab's glassware) are fitted numerically and
      recorded in the component that uses them.
    - **Motion is transforms and particles over real images** — CSS masks
      and rotations of the artwork itself (the Wheel of Fortune spins the
      painted wheel), Ken Burns drift on gallery lanes, and a canvas
      particle layer (`components/vapor.tsx`) for volumetric steam/smoke.
      Video loops stay on the table for later, but nothing so far has needed
      one; if one ever does, it is a committed asset with a PROVENANCE
      entry like any other.

    Landed so far under this heading: the painted wheel spin (#116), and the
    photo ivy canopy + Experimental Lab bench + gallery lanes (this branch).

    **The room was invisible from #118 until 2026-08-16, and the reason is
    worth keeping.** Driving the deployed instance, the maintainer reported
    every backdrop as "just a bland background… not present in the least."
    Three faults, stacked, none of which any test could see:

    - **The gallery lanes never rendered at any width.** `.scene-lane` is an
      `<img>` — a *replaced* element — given `top: 0; bottom: 0` and no
      explicit height. `height: auto` resolves from the intrinsic aspect
      ratio, the box is over-constrained, and `bottom` is the declaration
      dropped. Measured at 250x145 where 250x1000 was intended: painted,
      present, and far too small to read as anything.
    - **Every fixed backdrop was trapped for the first 300ms of each page
      view.** The routed page is wrapped in `.page-enter`, which animates a
      `transform`, and a transformed ancestor becomes the containing block
      for `position: fixed` descendants. `SceneBackdrop` portals to
      `document.body` now; anything fixed added under that wrapper needs the
      same treatment.
    - **The wash was tuned below visibility** — 0.13 opacity behind an 18px
      blur behind a mask fully transparent for its top 26%, an effective peak
      near 0.09. The sunbeam was worse and for a different reason: pale
      yellow on a near-white page is light-on-light, so more alpha could
      never have fixed it. It is amber now; the warmth is the signal.

    Two pages had no room at all (`/learn`, `/new`) and now wear mastheads
    like their four siblings. The lesson generalises past this feature: **a
    test that asserts an element renders has not asserted that it has a
    size**, and jsdom cannot close that gap — only a browser measuring a real
    box can.

    **The pipeline is real now — `mtglab animist`, 2026-08-16
    ([ADR 29](adr/0029-an-asset-is-committed-only-with-a-recipe.md)).**
    "Scripted Pillow" had been a description of scripts that were never
    committed; it is now a package (`src/mtglab/animist/`, Pillow behind the
    `animist` extra) driven by a `*.recipe.yaml` beside each asset directory:
    fetch from Openverse/Commons, **licence gate per file through the
    provider's API with no override**, transform (matte, feather, tile,
    resize), encode WebP with metadata stripped, write the PROVENANCE entry,
    and `verify` holds every committed asset to its recipe's `expect` block
    in the test suite. Both founding pipelines (ivy, tarot) are reconstructed
    as committed recipes. Wizards' art stays runtime-animated only — the
    pipeline deliberately has no provider that takes a Scryfall URL.

    The first two later phases landed 2026-08-16 as **ADR 31**: procedural
    motion (a seeded, loop-perfect `spectral_noise` generator plus `advect`,
    `color_ramp` and `ken_burns`, with a `procedural` source whose
    declaration — its seed — is the source) and the animated formats
    (`awebp`/`apng` through Pillow, `webm`/`mp4` through `imageio-ffmpeg`,
    crf-controlled, dual-shipped for the Safari floor, never in the image).
    `verify` reads video through the same bundled ffmpeg that wrote it, and
    `measure` sweeps crf where a video output is the subject.

    The whole chain is live as of the same day. The first procedural asset
    shipped — `mist.recipe.yaml`, seed 6161, the forest mist every room's
    floor now carries through `SceneBackdrop`, budgeted at `measure`'s crf
    knees (webm 40, mp4 30) — and **2.5D depth parallax runs for real**:
    ADR 32's runtime tier derived depth-drift loops for all six commanders
    with Depth-Anything-V2-Small on this machine, the browser plays them
    through `CommanderMotion` (WebGL tilt where a depth map ships, the
    baked loop elsewhere, the still as the floor), and the derivatives
    travel to the instance over sftp, never through git.

    One trap worth the paragraph: torch 2.2.2 is the **last** torch with
    macOS x86_64 wheels and it predates numpy 2, so the `depth` extra pins
    `numpy<2` and lives in **its own venv** on this machine
    (`.venv-depth`); the main venv never downgrades. The extra's comment in
    `pyproject.toml` records the whole argument.

    The committed painting landed 2026-08-16, chosen with Aaron over the
    alternatives (Rembrandt's Philosopher, Böcklin's Isle, Friedrich's
    Wanderer): **Spitzweg's *Der Bücherwurm*** (bookworm.recipe.yaml — a
    wikimedia source through the gate, resize, then a `ken_burns` breath
    with `bounce`), hanging framed at the foot of the Learn page. Two more
    procedural loops joined the mist the same day — **mana wisps** (seed
    1909, the fortune-teller's table and room; `color_ramp` grew a `gamma`
    for them, because a wisp is mostly the dark around it) and **candlelight
    embers** (seed 1666, the Laboratory) — behind a `mood` prop on
    `SceneBackdrop`, and the mist itself was softened (falloff 2.7, advect
    5) after its chewed edges read as moss. Sprite sheets and the runtime
    sprite-sheet player remain open; a committed depth map for the painting
    (true parallax rather than the breath) is the natural next step and
    needs the where-does-a-depth-map-live story from ADR 31.

    From Aaron's 2026-08-16 eye pass: **motion derivatives are a
    dev-machine artifact, and the deployed instance has no way to grow
    one.** `mtglab cardmotion sync` (2026-08-16) is the dev-side answer —
    every deck's commander, from the printing the deck shows, art swaps and
    imports both — but a deck imported *on the instance* shows a still
    until somebody runs the sync here and pushes.

    **Decided 2026-08-16 (at Aaron's direction, in session): the still is
    the intake-time story.** A deck imported on the instance shows its
    commander's still — the browser ladder's designed floor, not a
    degraded state — until the next dev-machine `cardmotion sync` + push,
    which joins the end-of-session ritual rather than a schedule. The
    other two options lost on their own terms: in-container `slow-pan`
    needs ffmpeg in the image, which ADR 31 deliberately keeps out
    ("dev and CI only, never the image"), and spends shared-machine CPU
    upgrading a decorative layer a few hours earlier; a scheduled
    unattended sweep is automation on a laptop that sleeps, and a
    schedule that silently doesn't run is worse than a ritual that
    visibly does. Three named triggers reopen this, and the shape it
    would take is a NET job (never the request, per ADR 32): another
    pilot's instance-imported deck visibly living on a still long enough
    for a human to mind; a second Fly machine appearing; or ffmpeg
    entering the image for any other reason.

13. **The tarot overhaul** — started 2026-08-16, under the new commandment
    15: the reading is a gift for Aaron's sister, and it gets the best of
    everything. Aaron's brief is eight items; the phase runs them as five
    PRs, each verified by eye through the Playwright WebKit rig before it
    lands (the pane screenshots black on this machine; headless WebKit
    does not).

    **Landed in the first PR (this branch):** the Magic crossovers wear a
    drawn 1909 Rider frame (ivory ground, numeral band, Fell-set caption,
    per-card cover focus — and the `cqw` unit is banned from the flipped
    card face, where WebKit 17.4 resolves it against the viewport); the
    fun facts can no longer repeat (the told list rides the wire like the
    transcript, is quoted back in the closing instruction, and a repeat is
    dropped and counted — the rule had been enforced by nothing); the
    fortune-teller writes on aged parchment (the Fell Types, OFL, with
    per-directory provenance; a quill-tracked word-by-word ink reveal,
    reduced-motion safe); and the deck grew an **echoes tier** (three deep
    dives, 2026-08-16): real cards whose name, art and rules carry a tarot
    card — every one of the 22 trumps answered (Gelon's Alpha Wheel of
    Fortune above all, the same painting the site's Wheel spins) and the
    minors opened — on top of the three printed tarot cards, whose cycle a
    Scryfall sweep confirmed complete. Two disciplines hold the tier:
    original imagery outranks every other classifier, and every Magic card
    carries a `note` justifying its slot in checkable pool facts (Flubs
    has 0 power, Homer is a 0/9, Apatzec's rules text says 4 four times)
    or it is cut — rendered under the turned card as the fun fact and
    handed to the reader.

    **Aaron judged the roster on 2026-08-17** and it is now thirty-eight
    echoes, 119 cards. Eight trumps changed hands: The High Priestess to
    Willow Priestess, The Lovers to True Love's Kiss, The Hanged Man to
    Suspension Field, Temperance to Chalice of Life // Chalice of Death,
    The Devil to Asmodeus the Archfiend (type line: Devil God, 6/6 for
    six, ability called Binding Contract), The Tower to Command Tower
    (Evan Shipard's struck-and-burning painting, on the land written for
    this format), The Star to Ephara, God of the Polis, and Death to
    Murderous Rider // Swift End. Nine minors opened: the Two, Five, Page
    and Knight of Wands; the Three, Ten and Queen of Cups; the Three, Nine
    and Ten of Swords. Two rules came out of that session. The rubric
    **widened past art alone** — a slot can be won on name, rules text or
    the card's place in the game, so long as the `note`'s facts are still
    checked against the pool. And the **printing is a choice**: the pool's
    default is not always the right painting, so Command Tower, Young
    Pyromancer, Thassa and Murder each name theirs. The search method that
    found them is worth keeping: Scryfall's Tagger vocabulary (`art:` /
    `otag:`) over the API, then Pillow contact sheets of every candidate
    beside the 1909 scan, judged by eye. A Magic card landing is called an omen with
    precedence in the frame message, and an original landing beside its
    Magic answer (trumps and minors both) is named as the stars aligning —
    all Python-detected facts, never prompt hopes.

    **Round two of the minors, and three rulings, the same day.** Aaron
    settled the three questions round one left open: **Temperance keeps
    Chalice of Life** over Angel of Serenity (the better fact, and the
    monochrome suits 1909 stock — the earlier "unreadable grey blob"
    report was a screenshot of the folded 96px strip, not of the card);
    **suit colour is a tiebreaker and not a law**; and **the pale horse
    stays, as the Knight of Swords** — Pale Rider of Trostad, cut from
    Death in round one, takes the charging knight instead, which is the
    better slot for it, since the Knight's horse gallops and Death's
    walks. Seventeen more minors then filled in the same method
    (tag-search, contact sheets beside the 1909 scans, ruled card by
    card): the Six, Seven, Eight, Ten and Queen of Wands; the Two, Eight,
    Nine and King of Cups; the Four, Six and Knight of Swords; and the
    Ace, Seven, Eight, Nine and Ten of Pentacles. **Fifty-five echoes,
    136 cards**, thirty-six of the fifty-six minors now answered.
    `ECHO_WEIGHT` came down 0.2 → 0.14, and that number was **measured
    rather than reasoned**: 38 echoes at 0.20 put a Magic card in 35.5%
    of spreads and 55 at 0.14 put one in 35.7%, over 40,000 seeded deals.
    The landing rate is the constant; the weight is what moves.

    The colour tiebreaker is worth recording as **a rule that did almost
    no work**: round two seated four white cards in the fire suit and no
    white card at all in the suit of air, because in every one of those
    slots the painting and the rules text pointed where the colour pie
    did not. It breaks a genuine tie and nothing more — if colour is the
    best argument a candidate has, it is not a good enough candidate.

    **Murderous Rider changed printing** in the same pass, and it is the
    clearest case the printing rule has: the pool's default (Josh Hass)
    is a colour painting of a green-faced Zombie Knight, while Ravenna
    Tran's Eldraine showcase is pen and ink and, under the ageing filter,
    reads as though it came off the same press as the 1909 plate next to
    it. Five echoes now name their printing.

    **PR 2 landed the photo-real crystal ball.** The old one was inline
    SVG and it was good SVG — fresnel ring, caustic pool, a hand-turned
    brass cradle — and it still read as a rendering, because what a real
    sphere has that geometry does not is *dirt*: veils, fractures, internal
    cloud that no gradient stack proposes. The glass is now a CC0
    photograph of a smoky-quartz sphere (Ervins Strauhmanis, via
    Openverse), fetched, licence-gated, matted and committed through the
    animist rather than hand-placed, and the room behind it is two ADR 31
    procedural loops of our own — smoke at seed 1848, the year the Fox
    sisters started the spiritualist craze, and candlelight at 1909, for
    the printing.

    Three things are worth keeping from building it. **The composite is
    four layers because the card has to be *inside* the ball** — candle and
    smoke behind, the depths, the vision, then the photograph twice on top
    (once at `soft-light` to lay the glass over the card, once crushed to
    its highlights and screened so the specular arc sits above everything).
    Flatten any of it and the card is in front of a ball instead of in one.
    **The brass stays drawn**, because a cradle is turned geometry — a
    stack of profiles each catching its own light — which is what SVG is
    good at and what a photograph would have bolted a stranger's taste
    onto. And **moving it to the centre deleted a problem rather than
    managing one**: pinned to the felt's right edge it closed on the
    centred spread as the page narrowed, sat across the third card below
    about a thousand pixels, and had to be shrunk and then hidden below
    `lg` to cope. Standing it above the cards means nothing is beside it,
    so it shows at every width and can be the size the thing deserves.

    The animist grew one op for it, `mask_circle`, and the reason it is
    geometric rather than perceptual is the point: `matte_green` keys on
    colour because foliage has no edge a number can name, while a glass
    sphere is the one subject a chroma matte *cannot* cut out, since the
    whole subject is the background seen through it.

    **PR 3 is [#151](https://github.com/aasquier/sylvan-library/pull/151), and
    it grew past its brief into the whole table.** Aaron's verdict on the
    drawn brass was blunt and correct — asked whether it looked photo-real,
    the answer was no, and the argument for it ("a cradle is turned
    geometry, which is what SVG is good at") was a rationalisation for not
    doing the harder half. A photographed sphere on a vector stand is worse
    than an all-drawn ball, because the sphere sets a standard of realism
    the stand cannot meet and the eye goes to the seam.

    So the stand is now **the Met's own crystal ball on a bronze stand in
    the shape of a fish** — a carp leaping through waves with the sphere
    held in a spray of foam, museum open access, CC0. Aaron picked it off a
    board of six and chose the **sepia** grade off a board of four. The
    decisive property is that sphere and stand come from ONE photograph:
    one light, one shadow, one set of material responses, and no
    compositing mismatch to manage. It also deletes the problem three
    passes went into — the ball does not need to be made to *sit in* the
    ring, because in the photograph it already does.

    Two animist ops were built for it, both tested and committed.
    **`matte_backdrop`** cuts a subject off a studio ground by flooding
    inward from the frame edges, because on these plates the bronze is
    darker than the ground while the crystal is brighter and no single
    threshold keeps both; what separates subject from ground is
    connectivity, not brightness. Its `soft` ramp exists because what
    survived a hard cut was not backdrop but the object's own **cast
    shadow** (143-167 against a ground of 192), and `enclosed: drop` runs
    the flood first and only then removes unreachable pockets — the studio
    grey framed inside the arch of the wave, which on a table reads as a
    hole cut in the felt. **`duotone`** maps luminance onto a three-stop
    ramp, the still sibling of `color_ramp`; a monochrome museum plate has
    no hue to preserve, and bronze is close to the ideal subject, being
    genuinely one hue with luminance variation.

    The assets are built and verified: `crystal-fish.webp` and
    `crystal-shell-sepia.webp` (the same quartz through the fish's own
    ramp — two photographs on one table have to share a plate, or the glass
    reads as pasted onto the bronze, which it visibly did). **The geometry
    is recorded in the recipe** because no image can carry it: in the
    700x950 asset the foam closes at y=290 and its centre is x=529, and the
    ball is drawn at r=265 with its centre **0.88 radii above the claw
    line** — Aaron's number, off a board of four, because at 0.62 the claws
    crossed the ball's belly and it read as impaled.

    The plate is the **Met's own**, not the aggregator's, and that is an
    animist provider rather than a URL: Openverse's record for this object
    points at rawpixel's `editor_1024` derivative, 763x1024, while the
    museum serves 2982x4000. Irrelevant while the stand was an ornament and
    decisive the moment it became the largest thing in the room. The `met`
    provider's gate reads a **boolean** rather than a licence string —
    `isPublicDomain`, mapped onto CC0 — so the check is `is True` and not
    truthiness, and a test pins that a string `"true"` is refused; one hop to
    the institution that owns the object is also the better provenance.

    **Aaron's two nits were one bug and one bug's cousin.** The "dark navy
    fresnel rim" was never a colour: the shell photograph's sphere is
    `mask_circle: {r: 0.482}` of its file, so the glass ends at 96.4% of the
    orb's box while `border-radius: 50%` clips at 100%, and that 3.6%
    annulus of bare `.crystal-depths` was hard-cut by the radius — a drawn
    stroke by any other name. Warming the palette only made it a warmer
    stroke; ending the depths where the glass ends removes it. Its cousin:
    `filter: drop-shadow` follows an element's **clip**, so the orb's perfect
    circle cast a perfect ring of shadow in the same place. A third of the
    family sat next door — the vision's mask ramp reached zero *at* the box
    edge, and under a permanent `scale` animation WebKit squares it off
    there. **A mask that ends at the edge is one you are trusting the
    compositor to round off.**

    **Then the table became a room**, which is Aaron's composition and not a
    decoration on the old one: black above, green below, one horizontal line
    between them, and that line is the back edge of the table. A rack of
    CC0 church candles burns along it on either side — one crop mirrored, so
    it reads as one rack seen from both ends rather than two racks to
    compare — and the carp stands in the gap with its base bridging them,
    which is what puts the mirror's seam behind a foot of bronze. The
    horizon needs no number: the stage is exactly as tall as the ball and
    the stand occupies 17.98%–100% of that box, so `--seance-horizon` is
    measured up from the carp's own foot and used by the dark and both
    racks. The candles are **screened, not matted** — their ground is black
    and black is already transparent under `screen`, so the flames' halos
    survive as photographed; `matte_backdrop` is for the opposite case, the
    bronze, whose studio grey is *lighter* than its subject.

    The cloth is printed, too, which is the item's "card positions laid out
    around the ball": three named places stamped into the felt on an arc
    that radiates from the ball, with the cards landing in them. The place
    carries `--arc-rot` and the card carries `--arc-rot` *plus*
    `--settle-rot`, so every card lies a degree or two off its own place —
    the cloth was printed square and the deal was a hand. And turned cards
    zoom under the pointer (1.35x on the felt, 2.1x in the 96px strip),
    gated on `(hover: hover)` because on a touch screen `:hover` latches.

    **Three rig facts came out of it, and each looked like a product bug.**
    An **element** screenshot is not what a person sees — the first scale
    board cropped to `.crystal-ball` and would have shipped a ball whose
    tail is cut off by the viewport. A **narrow viewport is not a phone**:
    headless WebKit at 390px reports `hover: hover`, so the latched-hover bug
    rendered as correct behaviour until the context got `hasTouch: true,
    isMobile: true`. And headless WebKit **ignores `backface-visibility:
    hidden`** (proved in isolation, not inferred), so a face-down card paints
    its own face, mirrored — the card back cannot be photographed through
    this rig at all. Same ledger as the black Browser pane and the missing
    media codecs: the rig is not the browser, and where it differs it
    differs silently.

    **The two researched fronts and Aaron's ten-item polish landed
    2026-08-17 evening** (PRs #153 and #154, both ruled live off boards).
    The parchment is a photograph now — the Met's Qur'an folio 448369
    through `parchment.recipe.yaml`, its own hue kept (a duotone
    monochromes skin; the new `levels` op floors the shadows at a stated
    colour instead, 6.4:1 against the ink), a seeded deckle mask for a
    silhouette, candlelight breathing on the sheet. The hand writes in
    **Parisienne** at 52ms/char with pen-lift and punctuation pauses, no
    cap, click-to-dry — and the trap worth remembering: per-character
    spans break OpenType shaping in a joined face, so the timing is per
    character while the markup stays per word. The ceremony **fits one
    screen**: the page's chrome steps aside (`onCeremony`), the ball
    sizes against the viewport's height, the controls ride the room's
    dark corners, and the spread pulls up with its outer cards on the
    wings of the trimmed, widened racks. The table **lingers** once all
    three are up until the querent knocks twice on the glass (or takes
    the visible button). The card backs wear Magic's colour wheel, drawn.
    **The dripping-wax feature is scrapped** — five prototype mechanisms
    (falling beads, screened runs, curved SVG paths, photographed
    self-clones) all failed Aaron's eye; the photographed frozen runs
    stand, and the motion budget went to the flicker instead.

    **All of the above about the room was superseded on 2026-08-18**, and
    the paragraphs stay because the reasoning is worth keeping, not because
    the code is. Aaron generated a photo-real séance table (Seedance 2.0,
    his own machine, second take against a written brief: 16:9, eight
    seconds, clear felt in the near third, dark upper corners for the
    controls) and it replaced **the whole composite** — the Met's bronze
    carp and its quartz sphere, the mirrored candle plate and its
    seventeen measured flames, the three smoke loops, the drawn gradient
    room, the light-spill. Roughly 690 lines of CSS and 400 of TSX went
    with them, plus six assets and the `crystal.recipe.yaml` that built
    them.

    The argument is that **every one of those numbers existed to hide a
    seam between two things never photographed together** — the rack's
    mirror line, the sphere against its stand, the horizon where black met
    felt — and one photograph of one table has no such seams to hide. What
    is left is four rules and four measured numbers: where the sphere sits
    in the frame, as percentages of the room's own 16:9 box, so nothing is
    cropped and nothing drifts as the window resizes.

    The turned card now surfaces **inside the footage's own glass**: a dark
    disc multiplies the interior down, the card is screened over it, and a
    seeded turbulence filter ripples its edges. That order is forced —
    the sphere is lit from within, and a picture screened onto a near-white
    ball adds nothing an eye can find. The card backs are Magic's now, a
    plate Aaron painted, and the cards themselves are small: in a
    photographed room they are objects lying on a table rather than a
    composition competing with a drawn one.

    Three things came out of it worth keeping. **A duration measured for
    one surface is a question to ask of every sibling** — the phone was
    broken by this change and nothing said so, because the controls' "dark
    upper corners with nothing in them" are a fact about a wide window,
    not about the design; at 375px the room is 184px tall and three rows
    of wrapped buttons covered the candles. **`mtglab animist verify` is
    enforced by nothing**: it sweeps all eleven recipes and CI never runs
    it, while `tests/test_animist_recipes_repo.py` pins only two of them —
    so the nine broken outputs this change created passed the full suite
    and were caught by hand. And **the three new assets sat outside the
    gate**, because `sources.py` has no kind for a file the maintainer
    authored; `web/src/assets/seance/PROVENANCE.md` says so in as many
    words and names the `authored` source kind that is owed.

    **Still to come, in order:** the
    reader-as-artist proposal (the spread, flavor text and art crops as
    real evidence in the commander pick, pool-resolved as ever); an ADR
    and the 99 ceremony (category by category, a card dealt per category,
    ending in a draft deck through the import path — rule 4 intact); and
    the Wheel of Fortune's wildcard slots in that ceremony plus its
    visual love.

14. **The six-feature batch** — Aaron's list of 2026-08-18, planned as seven
    PRs (the full carve-up lives in the session plan and the PRs
    themselves). Landed so far, each walked by eye before commit: **the
    official mana symbols** served first-party from a runtime cache with
    the drawn five as offline fallback (ADR 33 — supersedes PR #61's
    drawn-only stance; tap, untap, colourless, hybrids, Phyrexian and
    numerals all draw now, every pip named for a reader); **the button
    overhaul** (commandment 17, Aaron's own words: thou shalt not make a
    simple button — one `.btn` family plus chip/tab/menu/felt cuts, every
    control answering hover, focus and press, the felt warming to brass
    rather than vine) with the replay and hand-fan glyphs on the rerun and
    reshuffle verbs; and **the 99 rollup** (each category folds behind a
    header wearing a drawn glyph, folds remembered per deck, plus the
    CardHover fix — the ~200 capture-phase scroll listeners the cards tab
    used to register are now at most one); and **the About Claude page**
    (`/claude`, after Learn in the nav — the librarian's own bio in the
    Keeper's structural pattern, with the tarot grid's spark serialized as
    its masthead and room, Simic argued as the site's own name said
    quietly, Kwain and Tatyova as live pool-fetched exhibit cases, and a
    four-painting gallery — Poole, Avon, Guay, McKinnon — every credit
    looked up at writing time and pinned by test, never recalled); and
    **the Admin tab** (Accounts renamed, wearing *Teferi's Protection* —
    the steward's spell — plus an on-box dashboard: four read-only
    `/api/admin/stats/*` views classified ADMIN in the isolation suite,
    all facts the box can report without an external API or a new secret.
    The ledger's tokens are labelled a floor on the bill in the payload
    itself — the Anthropic dollar widget was dropped with Aaron
    2026-08-18, his Console account is individual and the Usage & Cost
    Admin API does not exist for those; the Console links out, a person
    adds money. The job registry's view is `jobs.census()`, counts only,
    because labels can name another person's deck).

    Sixth: **the visitor ledger** (schema v9 — `request_log(day, route,
    status_class, count)`, four columns and a test that fails on a
    helpful fifth. Route *templates* only, never the concrete path a
    person typed; no IP, no user agent, no username, nothing finer than
    the UTC day. `api/traffic.py` buffers in memory and flushes
    opportunistically — no per-request write, and the flush never
    raises; requests refused before routing share one `(unrouted)`
    bucket because recording which door was tried is recording the
    path. `GET /api/admin/stats/traffic` + the dashboard's stacked
    chart and top-routes list. Landed on its own watched branch, the
    rule for anything that migrates on boot).

    **Still to come — the seventh and last of the batch: Fly metrics**
    (`FLY_METRICS_TOKEN` → managed Prometheus for machine + edge stats,
    Grafana link-out for alerting). The org slug is `personal` and the app
    is `sylvan-library`, so the query URL is already determined; the one
    thing code cannot conjure is the read-only token, which Aaron mints
    (`fly tokens create readonly`) because a credential is a human act.
    Consumers read `os.environ` directly (the `RESEND_API_KEY` pattern —
    a public config name may not contain KEY/TOKEN/SECRET, which
    `tests/test_config.py` pins), stdlib `urllib` behind a seam, a
    five-minute TTL cache, and the widget hides itself when unconfigured.

    **And one more thing, parked here so it is not lost: photographing a
    deck into the library.** Researched 2026-08-18 at Aaron's ask,
    alongside the batch rather than inside it. The gap is real: every
    import path we have is **text**, and a deck that exists only as a
    stack of cards on a table — the newcomer's deck, the one commandment
    2 is about — has nowhere to be typed from.

    *What already works, with no code from us.* The free scanner apps
    export the very format `decks/decklist.py` already parses (`1 Sol
    Ring (LTC) 284` — quantity, set code, collector number):
    **Dragon Shield MTG Scanner** (free, any language, text/CSV out) is
    the cleanest, **ManaBox** exports the Arena format (its free tier
    caps decks stored *in their app*, which scan-then-export does not
    touch), and **Delver Lens** leads on accuracy on Android but caps
    free exports at 100 cards a session — one Commander deck exactly. So
    the whole feature, in its cheapest form, is **one sentence on the
    Import page** telling somebody with a paper-only deck to scan it and
    paste the export; that sentence rides along with the next branch
    that touches Import.

    *If we build our own* — a camera door on the Import page, one card
    at a time (the rhythm those apps use; a 99 goes quickly).
    `getUserMedia` viewfinder with a card-shaped guide, and the capture
    crops the **bottom-left corner rather than the title**: reading the
    **collector number and set code** is the load-bearing trick, learnt
    from `GrimbiXcode/mtgscan` — a tiny alphabet in a fixed position,
    language-independent where title OCR is neither. **Tesseract.js**
    client-side, lazy-loaded only when the camera opens so the entry
    chunk pays nothing, and **Apache-2.0**, which is why it is that and
    not the GPL-3.0 prior art we may read and must not vendor into an
    MIT repo. **No image ever leaves the browser** — what crosses the
    wire is a short string, resolved against our own pool (set code plus
    collector number is in `printings`), with the existing fuzzy name
    search as the fallback a misread lands in. Confirmed cards feed the
    **existing import path**, so it becomes a draft with counted
    warnings and rule 4 stays intact (ADR 13 does the rest); nothing new
    is owed to the gate.

    Policy is clean on every count: Apache-2.0 tooling, the user
    photographs cards they own (Fan Content Policy), nothing of Wizards'
    is redistributed, no scraping, and no cloud OCR bill ever — which is
    what rules out `fortierq/mtgscan`, MIT but Azure-only. The honest
    risk is OCR accuracy on foils and old frames, which is what the
    corner trick is for, and why the fallback is a feature we already
    ship. **It wants its own session rather than a ride-along**: a
    viewfinder is exactly the surface commandment 16 exists for, and it
    needs Aaron's eye on real cards in real light rather than a green
    suite's word.

15. **The punch list** — Aaron's second list of 2026-08-18, five PRs, all
    merged and deployed the same day. **Four of the six items are done.**

    **Landed:** the deck page's defaults turned over (#167) — the 99 opens
    folded, the Wheel never remembers being open, the ivy withdraws the
    moment the page scrolls, and the commander's painting takes the crop's
    own 1.37:1 rather than a 4:1 stripe; **Admin became five wards** (#168)
    — Accounts, Claude, Machine, Storage, Activity, each fetching only what
    it shows, which cut the page from six polled endpoints to one or two;
    **per-account model tiers** (#169, schema v10) — a named seat may be
    answered by Opus or Fable, the column holds a tier *name* and never a
    model id, an unknown tier reads as the default while writing one is
    refused, and the dossier cache fragments per tier because the model id
    was already in its fingerprint; **the bill, estimated and dated**
    (#170) — `claude/prices.py` reverses the written-down "no price table"
    decision deliberately, answering each clause of the original objection
    (the rate that moves is modelled on both sides of 2026-08-31, rates
    carry the date a person read them, an unpriceable model is counted
    rather than charged zero), the ledger gained a per-model axis, and
    "Answered by" names the Claude rather than the model id; and **the
    glass cleared, then showed something** (#171, #172) — two separate Fly
    bugs, the second only findable once the first was fixed. A Fly token
    carries its own scheme, so `Bearer FlyV1 …` had 401'd every request the
    panel ever made; and with that fixed it reported success and five
    em-dashes, because `status` is the full code rather than a class
    (`status="2xx"` matched nothing) and `fly_instance_memory_resident` has
    never existed (it is `mem_total` − `mem_available`). Nothing had
    misbehaved: an empty vector reads as `None` reads as an em-dash, correct
    at every step — three correct behaviours hiding two wrong strings, and
    neither visible until the credential worked. Verified live: 244 MB of
    962 MB, 3,581 edge 2xx, 20 4xx, 5xx honestly absent.

    **Item 5 landed 2026-08-18** as a Blue + Red polish run — API hygiene
    and dev-cycle relics being Blue, alerts and instability signals being
    Red. The API surface came back **clean**: every one of the 60 routes is
    called or deliberately unclient-facing, every deck sub-route is reached
    from the app, no unimported frontend module, no dead Python module, and
    not a single TODO/FIXME/legacy marker in `src/mtglab` or `web/src`. The
    relics were not in the code but in what the code *says* about itself.
    **ADR 30 took decks out of git and eighteen comments never heard** —
    `deck.yaml` "tracked in git", `git log -p` offered as the swap record,
    `git checkout` named as a deck's undo four times, and one line printed
    to the terminal by `mtglab decks log`. The banked animist finding was
    real and is fixed: `verify` now runs over all twelve recipes in the
    suite instead of two. And Red found the sharpest one — **"Edge failed"
    rendered an em-dash both for a day with no 5xx and for a query that had
    never worked**, which is the punch list's own trap one turn on: a
    readout that cannot succeed differently from failing is not a readout.
    Both fixes carry mutation evidence, and the drift tripwire nearly
    shipped as decoration (it matched line-by-line, and this codebase wraps
    its comments).

    **Item 6 is done** — `tarotlore.py`, the fourth reference-prose module.
    Aaron picked the well from four (2026-08-18): **Pamela Colman Smith and
    the 1909 deck** — who drew it, what she was paid, whose photographs of a
    fifteenth-century deck she had seen two years earlier, and what is
    actually in the picture the querent is looking at. Not what a card
    portends, which has no right answer and is the reader's half.

    It extends `theme.py`'s fact mechanism rather than rebuilding it, and
    tightens it in one place. A fact is cited **by id** (`tarot:pixie-fee`)
    rather than by sentence, and `keep_fact` renders **the corpus's own
    words**; a reader that paraphrases or embellishes has the embellishment
    discarded. That is deliberately stricter than the `'taxonomy'` source
    beside it, because a fun fact invented at a fortune-teller's table is the
    one thing at that table that would be a lie — ADR 14 in miniature, and
    commandment 15's table getting the careful version.

    **Two tiers, and the deck tier is why no reading is ever empty.**
    18 facts are about the deck and its makers and are true
    of every spread — which matters because the sampler can deal three
    minors. 359 more are per-card, and **all 78 cards are
    covered**: the 22 trumps, and every minor at Aaron's asked-for five
    apiece or better. 377 facts in all.

    **None of the picture facts was written from memory.** Every one was
    checked against the committed 1909 plate in `assets/tarot/` — rule 1's
    habit applied to a deck instead of a pool — and the looking is what
    produced the corpus's best material. The Aces and the sixteen court cards
    carry printed title banners while the pips two to ten carry only a Roman
    numeral. The Seven of Wands is wearing two different shoes. The Eight of
    Wands and the Three of Swords have no person on them at all. There is
    exactly one cat in the deck and one rabbit, both at a Queen's feet, and
    the cat is the only animal looking straight out of a card. Each suit gave
    its four royals a creature: salamanders to Wands, fish to Cups, birds and
    butterflies to Swords, bulls to Pentacles. The Ten of Pentacles hides the
    Tree of Life in a picture of a family in a courtyard.

    One more finding came out of the wiring: `FactNote` captioned every
    source-less fact "From this tool's own colour reference data", so a fact
    about Pixie Smith would have been credited to a table of Magic colours.

16. **About Claude became Claude's room, and the room came alive** —
    2026-08-18, one branch. The masthead is a generated moonlit library
    (Aaron rendered Claude's numbered scene brief with Seedance; a measured
    ping-pong makes it loop seamlessly), Syr Gwyn got her exhibit case
    under the bio (commandment 4 shown a face, fetched live from the pool),
    **commandment 18** gave the page to its keeper, and the motion tier
    grew **`breath`** — a phase-warped swell that dwells at rest, the
    no-depth floor for paintings a pan would mistreat. The gallery and the
    heart wear `CommanderMotion` with per-piece effect ladders, and the
    cover crop learned a `center` anchor after Avon's portrait Island
    traded its subject for its sky under the hero band's `top`.

    **Next branch out of this one: region-scoped motion.** Aaron's asks
    from the first walk, in order: a **flame** for Syr Gwyn's torch, and
    **livelier cranes** on Farewell — both the same machinery, a masked
    region of a painting carrying its own displacement field (the
    animist's `spectral_noise` + `advect` are the primitives, a heat-haze
    warp scoped to a region). Two design decisions gate it, ADR-worthy:
    (a) a per-card mask must ride the cache fingerprint, which breaks the
    fixed `Effect` roster — per-card effect *instances* are the load-
    bearing change; (b) the vocabulary line: displacement-only shimmer
    reads as presentation and stays inside Scryfall's guidelines. A
    **person animator** (Poole's readers walking) was considered and
    **counselled against**: animating painted figures is synthesizing new
    content from Wizards' art rather than presenting it, and the Fan
    Content Policy is a hard boundary (commandment 9). The readers keep
    reading.
