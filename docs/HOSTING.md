# Hosting sylvan-library

A maintainer's guide: what has to change in the app, how login and per-user
isolation should work, what it costs to run, and the step-by-step infra setup.

Written for **you as the sole maintainer**, hosting one instance that friends
log into — not a forked repo per person. That choice is right: a fork gives
every friend a 500 MB Scryfall download, a Python toolchain, and their own
stale copy of the code. One instance means one upgrade path.

---

## 0. The architectural decision you have to make first

Everything else depends on this, so it goes first.

**sylvan-library is local-first by design.** `decks/<slug>/deck.yaml` is the
source of truth. It lived in git when this section was first written; ADR 30
has since made decks **live app data** — `decks/` locally, `/data/decks` on
the volume, gitignored everywhere — with the activity log (ADR 28) as the
edit record and the build snapshot as `swaps.md`'s baseline. What follows kept
its shape through that change: there are still two tiers, because friends'
decks still need per-user storage.

### Recommended: two tiers, not one

| Tier | Storage | Who writes it | Keeps |
| --- | --- | --- | --- |
| **Curated decks** (your six) | `deck.yaml` files under `MTGLAB_DECKS_DIR` | You, via CLI or UI | The file-based model: the gate, the five artifacts, `swaps.md` |
| **User decks** | SQLite on the volume, one row per deck | Logged-in users, via UI | Per-user isolation |

Your six live on the volume and everyone can view them — that is the
showcase, and it is what makes the site worth logging into. Users get a
UI-backed deck store.

**Why not give each user a git repo on the volume?** Tempting — it would
preserve `swaps.md` for everyone. But it means running git operations per
request, handling concurrent writes to repos, and a much larger failure
surface, for a feature friends have not asked for. Revisit if they do.

### Prerequisite changes in the app

These are needed before any deployment works at all:

1. ~~**Paths are hardcoded and relative.**~~ **Done.** `config.py` reads
   `MTGLAB_DATA_DIR` and `MTGLAB_DECKS_DIR`, defaulting to `data/` and
   `decks/` so local use is unchanged. The `fly.toml` below sets both, and the
   same change is what made the CLI testable against a scratch directory.
2. **A user/session/deck store.** SQLite, on the volume. Details in §2.
3. **Auth middleware and a user-scoped query layer.** Details in §1.

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
using, which the Accounts page deliberately refuses to do.

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

## 4. Infrastructure setup guide

**Recommendation: Fly.io.** Persistent volumes, scale-to-zero with fast wake,
a single deploy command, and no server to patch. Roughly **$6–8/month**, less
with scale-to-zero. If a simulator later needs real cores, move to a Hetzner
CX22 (2 vCPU / 4 GB, ~€4/mo) — more CPU per euro, at the cost of owning OS
updates and TLS.

**Two things could change this sizing, both open decisions in `ROADMAP.md`.**
Server-side Forge means a JVM plus a card database in the image, which is a
different class of container than anything costed here — **measured on
2026-08-11, and the numbers are in §7 below; the short version is that it does
not fit on the 1 GB instance this section prices.** And a Claude surface
(ADR 14) adds a per-request cost that is *not* CPU — it is somebody's API bill,
and on a shared instance it is the maintainer's. The numbers above cover the
app without either.

On the Claude half there is now at least an estimate. A rationale interview
turn is roughly 12K tokens in and 800 out, and the deck sits in a cached prefix
that reads at a tenth of input price — so interviewing a whole 99-card draft
lands near **$1–1.50 on Sonnet 5** and **$2.50–3 on Opus 5**. That is small
enough not to need a funding model at friends scale, and it is a separate
account from the app's hosting bill either way. **Research is the mode that is
not estimated** — web search and long context are where the cost actually is,
and it is the one plausibly worth gating per user. ADR 15's stance dial doubles
as the control: off means no API calls at all, which is a defensible hosted
default.

### Prerequisites

**Not `brew install flyctl` on this machine.** The only development machine
this project has is macOS 12 on Intel, where Homebrew is too stale to build
anything from source — so the package-manager instruction every Fly guide opens
with is a dead end here, and it is worth saying that in the guide rather than
rediscovering it on deploy day. Fly ships a shell installer that needs no
package manager and no compiler:

```bash
curl -L https://fly.io/install.sh | sh
fly auth login
```

It installs a static binary to `~/.fly/bin` and appends a `PATH` line to your
shell rc — `~/.bashrc` here, which `~/.bash_profile` sources, so it resolves in
a login shell. Verified on macOS 12.7.6 / x86_64 with flyctl v0.4.82: the
binary runs, which was the real risk on an OS this old.

**`flyctl` is not optional, and the browser is not a substitute.** Fly's web
dashboard can create the app, set secrets and deploy — but steps 6 and 8 below
are commands *inside* the machine, and there is no browser path to either. An
instance built entirely from the dashboard has no card pool, no decks, and no way
to give the maintainer a password.

You will also need a credit card on file; the machine sizes below are inside
Fly's paid tier.

### Step 1 — Dockerfile

**Both files now exist in the repository** — [`Dockerfile`](../Dockerfile),
[`docker-entrypoint.sh`](../docker-entrypoint.sh),
[`.dockerignore`](../.dockerignore) and [`fly.toml`](../fly.toml), landed
2026-08-12. They carry their reasoning inline; what follows is what the drafts
this section used to hold got wrong, so the reasoning is not lost with them.

The pool is **not** in the image. It is ~63 MB built from ~98 MB of Scryfall
bulk, Scryfall asks that bulk data not be redistributed, and it belongs on the
volume where it survives deploys. `.dockerignore` keeps `data/` out of the
build context so a local pool cannot reach a layer by accident, and the
`image` job in CI greps the built image for card pool files and fails on a hit —
the tracked-file check is about the repository, this one is about the artifact.
The frontend bundle *is* committed to `src/mtglab/web_dist`, so the image needs
no Node toolchain.

Five things differ from the draft, each for a reason:

- **Two stages, and still no Node.** `docs/ENGINEERING.md` §3 asks both that
  the no-Node property be kept and that the build prove the bundle rebuilds
  from source. Those pull opposite ways, and the second is already satisfied by
  the `frontend` job in CI, which runs the real `npm run build` and fails on any
  diff against the committed bundle. So CI proves the bundle is current and the
  image ships it; the builder stage exists to keep pip and any future compiler
  out of the runtime image, not to touch the frontend.
- **`MTGLAB_DECKS_DIR` is `/data/decks`, not `/app/decks`.** The draft would
  have lost data. `deck.yaml` is the source of truth and every editing route
  writes it, so decks inside the image meant a rationale written in the UI
  vanished at the next deploy — silently, with nothing to notice. See the
  deck-drift note in §5.
- **A non-root app process, reached through an entrypoint.** Fly attaches the
  volume owned by `root:root` and the mount shadows whatever the image had at
  that path, so a `chown` in the Dockerfile is invisible by the time it matters.
  PID 1 starts as root, fixes ownership, and `exec`s the app as `mtglab` via
  `setpriv`. A bare `USER mtglab` would look stricter and leave the app unable
  to write its own volume.
- **A `HEALTHCHECK` on `/api/health`**, using stdlib `urllib` rather than
  adding `curl` — a package installed for one HTTP request is a package to
  patch forever. That path is on `PUBLIC_PATHS`, so it answers with auth on.
- **No `README.md` in the build context.** `pyproject.toml` declares no
  `readme`, so the build backend never reads it.

> **Single worker on purpose — but not for the reason this section used to
> give.** The old note pointed at the DuckDB locking rule, and §3 has since
> corrected itself: read-only handles are safe across processes, so serving on
> two workers would be fine. What actually binds is `api/jobs.py`, whose
> registry is a module-level dict in one process. A sim submitted to worker A
> is invisible to worker B, and `get()` reports what it cannot see as absent —
> which the route turns into a 404 (ADR 5, never a 403). The symptom would be a
> running simulation reported as gone, at random, half the time. Sessions and
> the login rate limiter live in `app.db` and would have been fine.
>
> **That got sharper on 2026-08-13**, when the theme proposal became a job too
> (`api/themeruns.py`). It is a *four-minute* job that costs a real Anthropic
> call, so on two workers the failure would not be a lost simulation somebody
> resubmits for free — it would be a proposal that vanishes halfway through and
> has to be paid for twice. One worker, and the reason is now in two places
> rather than one.

### Step 2 — fly.toml

[`fly.toml`](../fly.toml) is in the repository. Two things to know before the
first deploy:

- **Four `[env]` values are placeholders** and are marked as such in the file:
  `MTGLAB_ADMIN_EMAIL`, `MTGLAB_ADMIN_USERNAME`, `MTGLAB_BASE_URL` and
  `MTGLAB_EMAIL_FROM`. The address ones are placeholders on purpose — this
  repository is public, and an email address is the one piece of personal data
  the project handles. Either edit the file or, to keep the address out of git
  entirely, `fly secrets set MTGLAB_ADMIN_EMAIL=...`; a secret is injected as an
  environment variable and takes precedence over `[env]`. Neither is a
  credential, but the second is a private place to put configuration.
- **`MTGLAB_REQUIRE_AUTH = "1"` is set**, and the local default stays off. One
  person on a laptop does not need a login; a shared instance is what §1 was
  written for.

Nothing secret is in that file and nothing secret may go in it — CI scans every
tracked file's contents for an API key, the committed frontend bundle included.

### Step 3 — create the app and volume

```bash
fly launch --no-deploy --name sylvan-library
```

```bash
fly volumes create mtglab_data --size 3 --region iad
```

3 GB leaves room for the 63 MB pool, the raw Scryfall download during a
refresh, `app.db`, the decks, and backups. It costs about $0.45/month.
`fly.toml` also carries `initial_size = "3gb"`, so a volume Fly creates for you
is the same size as one you create here.

That sizing was only half true until 2026-08-12: `cards.db.download_bulk`
defaulted to a *relative* `data/scryfall` and `cmd_data_refresh` passed nothing,
so the pool went to the volume and the ~98 MB of JSON it is built from went to
the container's working directory — an ephemeral layer, and not the thing this
3 GB was sized for. `config.SCRYFALL_DIR` is derived from `MTGLAB_DATA_DIR` now,
with the same "derived, never set independently" rule as `DB_PATH`.

### Step 4 — secrets

```bash
fly secrets set RESEND_API_KEY="paste-it-here"
fly secrets set ANTHROPIC_API_KEY="paste-it-here"   # four modes exist; see below
```

**If you set them in the dashboard instead, they are `Staged` and not live.**
Found the hard way on 2026-08-13. Fly's web UI stages a secret and waits for an
explicit `fly secrets deploy`; `fly secrets list` says so in a column nobody
reads until something is wrong. Until that runs, the app is up, healthy, and
running on `fly.toml`'s **placeholders** — which means `MTGLAB_ADMIN_EMAIL` is
`you@example.com` and the admin account that exists is called `you`. The
symptom is `mtglab users passwd <your-handle>` reporting no such user, which
points at everything except the cause.

```bash
fly secrets list          # STATUS must read Deployed, not Staged
fly secrets deploy
```

**And `fly secrets deploy` does not pick up `fly.toml` changes.** It restarts
the machine with new secrets against the *image and `[env]` of the last real
`fly deploy`, which is a half-state worth naming: new credentials, old
configuration. On the first deploy that showed up as a correct
`RESEND_API_KEY` next to a placeholder `MTGLAB_EMAIL_FROM` on an unverified
domain — so mail was configured, authorised, and refused by Resend on every
send. Run `fly deploy` after any `fly.toml` edit, and treat a `[env]` change
and a secret change as two different operations.

**There is no `SESSION_SECRET`.** This step used to open with one; the code was
then written and did not need it. Sessions are opaque random tokens stored as
their SHA-256, so there is nothing to sign and no key to hold — one fewer
secret to rotate. §7 records the same correction rather than quietly dropping
the line, because a checklist that loses entries is a checklist nobody trusts.

`RESEND_API_KEY` is the one that is genuinely required: with
`MTGLAB_REQUIRE_AUTH` on, `sender_from_env()` refuses to fall back to the
console sender, because that fallback would print recipients' email addresses
into Fly's logs and ADR 16 forbids it. It is read at call time rather than at
import, so the app still *starts* without it — what fails is the first invite or
password reset, which is to say the first thing you will try to do.

**If a send is refused with a bare `HTTP 403`, suspect the request before you
suspect the account.** `api.resend.com` is behind Cloudflare, and the first
real invite ever sent from this instance (2026-08-13) was refused with 403 and
Cloudflare's error code 1010 — the banned-browser-signature page — because
`ResendSender` was sending Python's default `Python-urllib/3.12` User-Agent.
The domain was verified, the key was valid, and the same request from
`http.client`, which sets no User-Agent at all, was answered 200. `mail.py`
now sends its own agent and a test pins it.

What is worth carrying forward is the diagnostic: **a 403 whose body is not
Resend's JSON did not come from Resend.** Their refusals carry
`{"name": "...", "message": "..."}`; a WAF's do not, so the error message now
says which kind it got. The two want completely different fixes, and an hour
went into the domain and the API key — both healthy — before that distinction
existed.

`MTGLAB_ADMIN_EMAIL` is **not** a secret and belongs in `[env]` — it is an
address, not a credential. (It may still be set with `fly secrets` to keep it
out of a public repository, as step 2 notes; that is a privacy choice, not a
security one.) It is worth guarding either way: whoever can change it is an
admin on the next boot (ADR 17). That is already true of anybody who can run
`fly secrets` or `fly deploy`, so it grants nothing new, and every change the
reconciliation makes is written to the log.

Never put either of the secrets in `fly.toml` or the repo. Fly stores them
encrypted, injects
them as environment variables at runtime, and setting one triggers a redeploy —
so they are never in the image either. Rotating `SESSION_SECRET` logs everyone
out, which is the correct emergency response to a suspected session leak.

`ANTHROPIC_API_KEY` is read by the Anthropic SDK directly; the app never binds
it to a variable, and asks only whether it is set. Locally the same variable
comes from a gitignored `.env` (see `.env.example`) rather than from Fly.

**The key alone is not enough, and the failure is silent.** The image must also
carry the SDK, which is the `claude` extra. On the first deployment it did not:
the Dockerfile installed `.[api]` on the then-true grounds that no ADR 15 mode
was built, so the instance had the secret set and nothing that could read it.
Nothing looked broken from outside — the app was healthy, the UI rendered the
dossier and theme-interview controls, and every one of them was a 503.
`mtglab claude check` on the machine is what says so:

```bash
fly ssh console -C "mtglab claude check"     # status: available
```

`tests/test_packaging.py` now pins the image's extras against the surfaces that
need them, because no other test can see this: they all stub the SDK, so they
pass whether or not it is installed.

#### Why a static key here, and when to stop using one

Anthropic offers three authentication methods, and their documented fit is
worth quoting rather than guessing at, because the answer for this app is not
the answer for every app:

| Method | Documented best for |
| --- | --- |
| **API key** | "Local development, prototyping, scripts, and **single-tenant servers where you control secret storage**" |
| **Workload Identity Federation** | "Production workloads on cloud platforms (AWS, Google Cloud, Azure), CI/CD pipelines, and Kubernetes, **where you want to eliminate static secrets**" |
| App Attest | iOS/macOS apps calling the API directly with no backend — not us |

A single-tenant Fly app with secrets in `fly secrets` is the first row
verbatim, so the key is the right instrument here and not a shortcut. The
guidance for moving is conditional rather than aspirational: adopt federation
"when your workload already has a platform-issued identity you can federate."

**WIF exchanges an OIDC JWT from an identity provider you already trust for a
short-lived token the SDK refreshes itself — there is no `sk-ant-api...` string
to mint, distribute, or rotate.** Setup is three Console resources (a service
account, a federation issuer, and a federation rule) plus an IdP that issues
OIDC tokens; the named ones are AWS IAM, Google Cloud, Azure/Entra, Kubernetes
service accounts, GitHub Actions, SPIFFE, and Okta. Two consequences for us:

- **Fly is not on that list.** Whether Fly's machine OIDC tokens can back a
  federation rule is a question for Fly's own docs before betting on it, and
  the Hetzner alternative in §3 has no platform identity at all. Until one of
  those resolves, a key in `fly secrets` is the endpoint, not a waypoint.
- **CI is the better first candidate.** GitHub Actions is an OIDC issuer
  Anthropic supports, and the reviewer workflow in `docs/ENGINEERING.md`
  already requests `id-token: write`. That is the place where a static
  repository secret could actually be removed.

Federation is not a free upgrade either — Anthropic's own caveat is that it
"does not, on its own, guarantee end-to-end security: the trust chain is only
as strong as your identity provider's configuration, and a long-lived secret
one hop upstream ... can still undermine it."

#### Key expiration, chosen once

A key's expiration is set **at creation and cannot be changed afterwards** —
3 hours, 1 day, 7 days, 30 days, a custom duration, or **Never**. "Never" is
the documented choice "for keys you store in a secrets manager and rotate
yourself", which is what `fly secrets` is. A short-lived key is the better
choice while the key lives mainly on a laptop, where the blast radius of a leak
is a stolen file rather than a breached host.

Anthropic emails the key's creator before expiry — 7 days ahead for a key with
a lifetime of at least 14 days — and an **expired key returns `401` with no way
to reactivate it**. Rotating means creating a new key and replacing it
everywhere it lives.

**This project runs a single environment**, so there is one key rather than one
per stage. That is the right call at this size — two keys is two things to keep
in sync for a benefit that only appears once other people have accounts — but
it has two consequences worth stating plainly:

- Once deployed, one key lives in **two places**: the gitignored `.env` on the
  maintainer's machine and `fly secrets` on the host. A rotation is not done
  until both are updated, and the second one is the easy one to forget.
- There is no staging key to fail first. The expiry date is therefore an
  operational date, not a background detail, and a short-lived key wants either
  a calendar reminder or a switch to **Never** once `fly secrets` is holding it.

**Make the failure legible in code.** When the Claude surface lands, a `401`
from the API should say *the key was rejected and may have expired* rather than
surfacing as a generic error — that message is worth writing on the day the
integration is built, because it will be read a month later by someone who has
forgotten the key had a lifetime at all.

Three ways this leaks that are worth naming, because two of them are specific
to this app:

- **Never with a `VITE_` prefix, and never in `web/.env`.** Vite bakes those
  into the bundle, and `src/mtglab/web_dist/` is committed to a public
  repository *and* served to every browser. Every Claude call goes through
  FastAPI; the frontend must never hold the key. CI scans the committed bundle
  for exactly this.
- **Never in a prompt or a message.** Session history persists, so a key placed
  there is durably stored and readable back for the life of the session.
- **A spend limit on the API workspace is the backstop**, because storage
  hygiene eventually fails and a cap bounds what that costs. Rotating the key
  is a console click.

### Step 5 — first deploy

```bash
fly deploy
```

This is the **first** deploy, and the manual path generally. Routine deploys
are automatic as of 2026-08-14: a push to `main` whose four checks are green
deploys itself, and there is a manual button in the Actions tab that runs the
whole workflow and then deploys. See
[ADR 23](adr/0023-a-green-main-deploys-itself.md) and §5 below for the runbook.
`fly deploy` from a laptop still works and is the rollback path.

The app will start with **no card pool and no decks**. Both are expected, and both
are fixed by step 6 — `/api/health` reports pool state rather than crashing,
which is exactly the fresh-clone case the API tests already cover, and an empty
`MTGLAB_DECKS_DIR` yields an empty library rather than an error. The CI `image`
job pins that: it starts this image with an empty volume and requires
`/api/health` to answer 200 with `"pool": false`.

You should be able to sign in at this point, before seeding anything.

### Step 6 — seed the volume

Two things live on the volume and neither arrives on its own. **This is a
documented run, not a build step and not a boot step** — the pool half needs
several minutes and a ~500 MB download, and with scale-to-zero putting boot on
the request path, doing it at startup would turn a visit into an outage.

**The decks**, first, because it is instant. The image carries none (ADR 30:
decks are live app data, not repository content), so a fresh instance's
library fills the way the pool does — from outside. Either restore a backup
pulled off a previous instance:

```bash
fly ssh sftp put ./backups/decks-<date>/<slug>/deck.yaml /data/decks/<slug>/deck.yaml
```

or push your local working decks up, one `deck.yaml` per deck, or simply
import through the app once you can sign in. A brand-new instance with zero
decks is a legitimate state, not a broken one.

Files put over sftp arrive owned by root, so hand them back afterwards — the
entrypoint does this at every boot, and a restart would fix it, but not
before the first write fails:

```bash
fly ssh console -C "chown -R mtglab:mtglab /data"
```

**The pool**, second, and this is the slow one:

```bash
fly ssh console -C "mtglab data refresh"
```

Both halves of that download land on the volume, which was not true before
2026-08-12 — see step 3. If the machine's connection or your shell drops
part-way, re-run it: `download_bulk` writes to a `.part` file and renames only
on completion, so an interrupted download is never mistaken for a finished one.

Verify, and note this checks both halves at once — `validate` needs the pool
to check card facts and a deck to have something to check (use whichever slug
you restored or imported):

```bash
fly ssh console -C "mtglab decks validate gyome-food"
```

On the maintainer's own library, expect **Goreclaw and Atla Palani to fail
the gate on one banned card each**. That is a known, deliberate state
recorded in CLAUDE.md, not a bad deploy.

#### Reading the mail DNS, and the mistake worth not repeating

Resend's records span three names and it is easy to declare one missing by
querying the wrong one. For a sending domain of `send.sylvan-libraries.com`:

| Name | Record | What it is |
| --- | --- | --- |
| `send.sylvan-libraries.com` | `MX inbound-smtp...` | receiving |
| `send.sylvan-libraries.com` | DKIM at `resend._domainkey.` | signs outbound mail |
| `send.send.sylvan-libraries.com` | `TXT v=spf1 include:amazonses.com` | the custom MAIL FROM |
| `send.send.sylvan-libraries.com` | `MX feedback-smtp...` | bounces |

**SPF is evaluated against the envelope sender, not the `From` header.** SES
sets a custom MAIL FROM on a further subdomain, so the SPF record correctly
lives at `send.send.` and its absence from `send.` is not a gap. On
2026-08-13 this was reported as a missing SPF record on exactly that reasoning;
the tell that it was wrong was already in the output and read past —
`inbound-smtp` is a *receiving* endpoint, so the sending records were always
going to be somewhere else.

Both alignments hold: DKIM matches the `From` domain exactly, and SPF matches
under relaxed alignment through the shared organisational domain.

**Porkbun ships two ALIAS records on a new domain** — one at the apex and a
`*` wildcard — both pointing at a parking page. Both have to go: the apex one
conflicts with the `A` record outright, and the wildcard otherwise answers for
every name that has no record of its own, including the `_acme-challenge` and
`_fly-ownership` names Fly falls back to. Deleting them does not disturb the
mail records, because a wildcard never synthesises for a name that exists —
verified before and after on this instance rather than assumed.

### Step 7 — domain and TLS

The domain is `sylvan-libraries.com`, registered 2026-08-13, and the app
answers on the **root**:

```bash
fly certs add sylvan-libraries.com
```

Add the records Fly prints, then confirm:

```bash
fly certs show sylvan-libraries.com
```

An apex is A/AAAA rather than a CNAME, which Fly prints for you — there is no
extra work, but it is the one place the instructions differ from a subdomain.
`force_https` in `fly.toml` handles the redirect. TLS is automatic and free.

**The `A` record points at Fly's *shared* IPv4 and that is correct.** It looks
wrong next to a dedicated IPv6 and it is not: Fly routes shared-IPv4 traffic by
the hostname in the TLS handshake, so a dedicated IPv4 buys nothing here and
costs ~$2/month. Validation runs against the `AAAA` — Fly needs one of an AAAA
pointing at the app, an `_acme-challenge` CNAME, or a `_fly-ownership` TXT, and
the dedicated IPv6 satisfies the first without any extra record.

**Done 2026-08-13.** Issued by Let's Encrypt about three minutes after the
records landed; `https://sylvan-libraries.com` serves the app, plain HTTP 301s
to it, and `sylvan-library.fly.dev` keeps working alongside.

**`send.sylvan-libraries.com` is the mail subdomain and is not this.** It is
verified with Resend and carries the sending records; nothing is served from
it. `MTGLAB_EMAIL_FROM` is on `send.`, `MTGLAB_BASE_URL` is the root, and
mixing them up produces mail Resend refuses or links that go nowhere.

### Step 8 — create your account

**Usually you do not.** `MTGLAB_ADMIN_EMAIL` from step 4 has already created it:
the app reconciles that address to an enabled admin every time it starts
(ADR 17), so a deployed instance comes up with your account in place and no
password on it. Claim it from the sign-in page's "forgot password" link — an
unclaimed account gets a reset link deliberately — and you are in without ever
opening a shell.

The shell path is the break-glass one, for a misconfigured mail provider or a
`MTGLAB_ADMIN_EMAIL` you got wrong. **It has to be an interactive console**,
and this is the correction worth reading before you need it: the password is
read with `getpass`, which needs a TTY, so `fly ssh console -C "..."` — which
runs one command with no terminal attached — cannot work. Open the shell
first:

```bash
fly ssh console
```

then, **before anything else**, put the venv on `PATH`:

```bash
export PATH="/opt/venv/bin:$PATH"
```

This is the second half of the same correction and it is the combination that
actually bites. The image sets `ENV PATH="/opt/venv/bin:$PATH"`, and
`fly ssh console -C "mtglab ..."` inherits it — every command in step 6 works
unmodified. An *interactive* console does not: it starts a login shell, and
Debian's `/etc/profile` overwrites `PATH` for root. So the one form that needs
a TTY is the one form where `mtglab` is not found, and the error says
`command not found` rather than anything about `PATH`. `/opt/venv/bin/mtglab`
in full works just as well for a single command.

Then, whichever of these applies:

```bash
mtglab users passwd gyome                       # the bootstrapped account exists
mtglab users add aaron --email you@example.com --admin   # it does not
```

Running as root is fine here, and deliberately so: `docker-entrypoint.sh`
does a recursive `chown` of the volume at every boot *because* this step and
step 6 both arrive over `fly ssh console` as root. Anything you leave
root-owned is repaired at the next restart.

`users passwd` is usually the one you want. `MTGLAB_ADMIN_EMAIL` has already
created the account and left it unclaimed, so there is nothing to *add* — what
is missing is a password, and that is the command that sets one. It ends every
session on that account as a side effect, which is the right behaviour for a
credential reset.

Do this from a terminal you trust. Both prompt twice; there is no way to pass a
password as an argument, because command-line arguments land in shell history
and in the process table.

**`RESEND_API_KEY` is not optional for the email path**, and it is worth being
explicit because the fallback people expect is deliberately absent: locally, a
missing key makes `mtglab users invite` print the link it would have sent, but
with `MTGLAB_REQUIRE_AUTH` on that fallback is *refused* rather than used —
printing recipients into Fly's logs is what ADR 16 forbids. So on a deployed
instance there is no "just read the link off the console" escape hatch. Either
Resend works, or you use the shell.

**Clean up the placeholder account if one was created.** If the instance ever
booted with `MTGLAB_ADMIN_EMAIL` unset or still reading `you@example.com`, an
admin account exists for it. Disabling is the right lever here and `delete`
would be the wrong one: the placeholder holds no address anybody wants back,
and a row that stays is a record of what the bootstrap did. Do it only *after*
your real account has a password, or the last-admin guard refuses:

```bash
mtglab users disable you
```

(`mtglab users delete` does exist, and sessions and tokens are not the obstacle
they once looked like — both cascade. It is for releasing a `username` or an
`email` so it can be invited again, which is the one thing disabling cannot do.)

Everyone else gets an invite rather than an account you made a password for
(ADR 16) — `mtglab users invite <email>`, or the Accounts page once you are in.
Note that neither the CLI nor that page will demote or disable the last admin
who can sign in; to hand the instance over, promote the successor first.

**If somebody says the link "didn't work" and dropped them on the sign-in
screen, this is almost certainly it: their mail app cut the link short.** The
token travels in the URL fragment so it stays out of every access log, and the
cost of that is that some clients drop it when you click — the address they can
*see* is whole, and the one the browser opens is not. Nothing about it reaches
the server, so there is no log line to find, and re-sending does not help
because the next link fails identically.

The claim screen takes a pasted address for exactly this, and both messages say
so. Tell them to copy the whole address out of the email — including the part
after the `#` — and paste it into the box on that page. Their link is
untouched: a stripped click spends nothing.

---

## 5. Running it

### Deploying

**A push to `main` whose four checks are green deploys itself**, since
2026-08-14 — [ADR 23](adr/0023-a-green-main-deploys-itself.md). The `deploy`
job in `ci.yml` `needs` all four checks, so it cannot start unless they passed,
and it runs only for a push to `main` or an explicit `workflow_dispatch`.
Expect it about ten minutes after a merge; most of that is the `image` job.

**To deploy without merging anything** — a redeploy after a `fly secrets`
change, say — use the manual button: Actions → *tests* → **Run workflow**. It
runs the whole suite and then deploys, deliberately: a button that skips the
suite is a button that eventually ships something red.

#### The deploy token

CI authenticates with a `FLY_API_TOKEN` repository secret. Fly is opinionated
about this and worth following: use **the token with the narrowest access that
will work**, which for deploying one app is an app-scoped deploy token rather
than the org-wide auth token.

```bash
fly tokens create deploy -a sylvan-library -n github-actions-deploy -x 8760h
```

```bash
gh secret set FLY_API_TOKEN --repo aasquier/sylvan-library
```

Both flags matter and neither is the default:

- **`-x 8760h` — one year.** `fly tokens create deploy` issues a **20-year**
  token when you omit this (175200h), and flyctl's own help recommends against
  it. A credential that outlives the project is how one ends up live in an old
  fork.
- **`-n github-actions-deploy`** — the default name is `flyctl deploy token`,
  which tells you nothing in `fly tokens list` once there is more than one.

**This token expires 2027-08-14.** When it does, merges will keep landing and
the deploy job will go red, which means `main` and the instance diverge — the
exact state [ADR 23](adr/0023-a-green-main-deploys-itself.md) exists to
prevent, just with a red check beside it. The deploy job prints the rotation
command on failure for that reason.

There is no way to avoid a stored secret here: Fly's OIDC support is
**outbound** — Machines authenticating to AWS, GCP and so on — and there is no
inbound trust from GitHub Actions to Fly. Checked 2026-08-14.

```bash
fly tokens list                            # what exists, and when it lapses
fly tokens revoke <id>                     # after rotating
```

**The deploy is not done when `flyctl` exits.** The job then requires the live
instance to answer `/api/health` with 200, `"pool": true` and a non-zero deck
count. The last two are the ones worth having — a fresh or unmounted volume
answers `"pool": false`, and a deploy that lost the volume loses deck edits
that exist in no repository.

**A failed smoke test does not roll back.** It fails loudly and prints
`fly releases`. That is deliberate: the schema ladder is forward-only, so an
automatic revert can leave things worse than the state it reverted from, and
an unmounted volume is not fixed by redeploying the previous image.

To roll back by hand:

```bash
fly releases --app sylvan-library          # find the last good version
fly deploy --image <image-ref-from-above>  # or: fly releases rollback
```

**A schema change deserves the treatment every deploy used to get.** Migrations
in `auth/db.py` run on boot and do not run backwards, so rolling the code back
does not roll the schema back. Land a schema change on its own branch, merge
it when you can watch it, and take a backup first (see *Backups* below).

One machine and one volume means Fly cannot roll — a deploy stops the instance
and starts the replacement, so every deploy is a few seconds of downtime. That
is inherent to the shape rather than something automation introduced; it simply
happens more often now.

### Refreshing the pool

Scryfall publishes daily; deck tooling does not need day-fresh data. Monthly is
plenty unless you are watching prices.

```bash
fly ssh console -C "mtglab data refresh"
```

**One line, and it does not fit on the machine that runs the app.** Measured on
the first real run, 2026-08-13. The job is five phases and only the first
prints before it starts:

1. download `oracle_cards` (~24 MB gzipped)
2. `load_oracle` — 35,390 rows, **silent until it finishes**
3. print `loaded N oracle cards`
4. download `default_cards` — several times larger
5. `load_printings` — 107,338 rows, the longest phase

On `shared-cpu-1x` phase 2 alone took **over thirty minutes** and phase 5 is
three times the rows. So scale up for the run and back down after:

```bash
fly scale vm performance-1x                       # ~7x faster, measured
fly ssh console -C "mtglab data refresh"
fly scale vm shared-cpu-1x --vm-memory 1024       # or let the next deploy do it
```

**`performance-1x` rather than `shared-cpu-4x`, despite a quarter the cores.**
The load is a single-threaded Python loop — `json.loads` per line, then batched
`executemany` — so more cores buy nothing and an unthrottled one buys
everything. Measured on the volume: ~0.65 MB/min of WAL growth on the shared
slice against ~4.5 MB/min on the dedicated core.

`fly.toml` pins `shared-cpu-1x` in `[[vm]]`, so **the next `fly deploy` scales
you back down whether or not you remember to.** That is a safety net, and it is
also why a permanently larger machine has to be a change to the file rather
than a `fly scale` command.

#### Watch the filesystem, not stdout

Two things about a long job over `fly ssh console` that cost an hour of
confusion on the first run, and that generalise to every command in step 6:

**The output lags badly.** `fly ssh` delivers stdout in chunks, so the terminal
sat on `downloading oracle_cards ...` while the work was demonstrably three
phases further on. Progress is on the volume:

```bash
fly ssh console -C "sh -c 'date -u +%T; ls -l /data/mtg.duckdb /data/mtg.duckdb.wal'"
```

A growing `.wal` is work in progress; a `.wal` that collapses while the
database jumps is a commit landing — that is how you see a phase boundary go
by without a line being printed.

**And the session dying does not mean the job died.** `auto_stop_machines` is
`suspend`, which snapshots memory rather than killing processes: when the
machine suspended mid-refresh, the SSH session broke with
`remote command exited without exit status or exit signal` — a message about
the transport that reads like a message about the job — and the refresh
*resumed from memory* on the next start and ran to completion headless. The
apparent failure was reported, believed, and wrong. **Check
`/api/health` before concluding anything about a job that appeared to die.**

**Whatever ran over `fly ssh` ran as root**, so hand the volume back
afterwards. A restart does it, since that is what the entrypoint's
`chown -R` is for:

```bash
fly machine restart <machine-id>
```

**Why there is no cron here.** Fly volumes attach to exactly one machine, so a
scheduled second machine cannot mount the same volume — the obvious approach
does not work. Your options, in order of how much I would recommend them:

1. **Run it by hand monthly.** One command, zero code, and you will notice if
   it breaks. At this cadence this is genuinely the right answer.
2. An authenticated admin endpoint that kicks off a refresh as a background
   job, called by GitHub Actions on a schedule.
3. An in-process timer thread in the app, guarded by a lock file.

Start with (1). Only build (2) if you find yourself forgetting.

### Decks on the volume, and the laptop

This section used to be about keeping `/data/decks` in step with a copy
tracked in git, and it opened with the admission that neither was
automatically authoritative. ADR 30 resolved that by removal: decks are live
app data, git holds none, and **each instance's `MTGLAB_DECKS_DIR` is the
only copy that instance has.** The laptop's `decks/` and the volume's
`/data/decks` are two different libraries that happen to share some slugs,
the same way two laptops would be. Copy between them deliberately, in
whichever direction you mean:

```bash
fly ssh sftp get /data/decks/<slug>/deck.yaml ./decks/<slug>/deck.yaml
```

`sftp put` silently overwrites whatever was written on the instance, so do it
only when you know nothing was — the History tab (ADR 28) is how you know.

### Card-art motion derivatives (ADR 32)

Generated on the dev machine (`mtglab cardmotion build --deck <slug>
--effect depth-drift`), never in git and never in the image; the instance
only serves what sits in its cache. Push a finished derivative up the same
way everything else reaches the volume:

```bash
fly ssh sftp put -r data/cache/cardmotion /data/cache/cardmotion
```

Files arrive root-owned; the entrypoint re-chowns `/data` at the next boot,
or run the chown line from §4 step 6 to fix it immediately. Nothing here is
irreplaceable — a lost cache regenerates from the same seeds and pool — so
`backups/` need not carry it; the deck pages simply show stills until the
push is redone, which is the app as it was before the tier existed.

### Backups

The pool needs no backup — `data refresh` rebuilds it in one command. That is
the whole reason it is gitignored.

Two things on the volume *are* irreplaceable. **`app.db`** holds users,
sessions and password hashes. **`/data/decks`** holds the instance's whole
library — since ADR 30 there is no git copy behind it — including the
rationales your friends wrote, which by rule 4 nobody may regenerate on
their behalf.

Back `app.db` up with SQLite's online backup, which is safe to run against a
live database. **There is no `sqlite3` binary in the image** — the same class of
absence as `curl`, and for the same reason: nothing in the runtime needs one.
Python's `sqlite3` module has the identical online-backup API and is present by
definition, so that is what the procedure uses:

```bash
fly ssh console -C "python3 -c \"import sqlite3; s = sqlite3.connect('/data/app.db'); d = sqlite3.connect('/data/app-backup.db'); s.backup(d); d.close(); s.close()\""
fly ssh sftp get /data/app-backup.db ./backups/app-$(date +%F).db
fly ssh console -C "rm /data/app-backup.db"
```

`python3 -m sqlite3` looks like a drop-in for the missing binary and is not
one: Python 3.12 does ship a `sqlite3` CLI, but it only executes SQL. It has no
`.backup` dot-command and answers `near ".": syntax error`.

**The third line is not tidiness.** Left behind, `/data/app-backup.db` is a
second complete copy of every password hash and every email address, sitting on
the volume indefinitely. Take the backup, pull it down, remove it.

`fly ssh sftp get` works. It is `put` that Fly's permission classifier refuses
— worth knowing when you want a script *on* the machine, where inline
`python3 -c` is the way in.

Do not simply `cp` a live SQLite file — with WAL enabled you can capture a torn
copy. Keep these backups private: they contain password hashes **and email
addresses**, which is the same reason `app.db` is gitignored (ADR 16). A
backup directory that ends up in git is the leak this whole rule exists to
prevent, so keep `backups/` out of the repository.

This procedure was executed against the live instance on 2026-08-13 and
verified end to end: online backup, `integrity_check: ok`, pulled down with
`sftp get`, restored into a scratch `MTGLAB_DATA_DIR`, and opened by the app's
own `auth/db.connect()` — with `foreign_keys` on, so ADR 16's
`ON DELETE CASCADE` survives a restore.

**Take one before a deploy that carries a schema migration.** `app.db` migrates
itself on the first connection after a deploy, which is usually invisible and
occasionally not: schema version 5 *rebuilds the `users` table* to add
`AUTOINCREMENT`, because SQLite cannot `ALTER` a column into it. The rebuild is
guarded — `foreign_keys` off around the ladder, `PRAGMA foreign_key_check`
before it is given back, and a refusal to serve from a file that fails it — but
"the migration is careful" and "there is a copy of the file from before it ran"
are not the same assurance, and only one of them is yours. `PRAGMA user_version`
tells you where a file is:

```bash
fly ssh console -C "python3 -c \"import sqlite3; print(sqlite3.connect('/data/app.db').execute('PRAGMA user_version').fetchone()[0])\""
```

The decks need no such ceremony — they are plain YAML — but they do need
copying, and since ADR 30 this pull is the instance's whole recovery story
alongside Fly's own snapshots:

```bash
fly ssh sftp get /data/decks ./backups/decks-$(date +%F)
```

#### The snapshots Fly takes on its own, and what is not yet known about them

Everything above is a backup *you* run. Fly is separately snapshotting the
volume daily without being asked, and this runbook said nothing about them
until 2026-08-16, which is the kind of silence that reads as "there are none"
at exactly the wrong moment. Observed that day:

```bash
fly volumes list --app sylvan-library
fly volumes snapshots list vol_vwnqxewn1y00oy9v
```

Four snapshots, one a day, the newest six hours old, **five-day retention**,
286 MiB stored against a 3 GB volume. Five days is Fly's default and it is the
number to know: a corruption nobody notices within five days is a corruption
with no snapshot behind it, and `app.db` is the file where that matters,
because a bad row can sit unnoticed far longer than a missing one.

**A deploy does not take one, and the timing is exactly backwards.** Four
deploys landed on 2026-08-16 — machine v61 through v65 — and the newest
snapshot still predated all four, because Fly snapshots on a daily clock that
knows nothing about when the volume is at risk. The moment it is *most* at
risk is the boot after a merge: ADR 23 means merging deploys, and `auth/db.py`
migrates forward-only on the first connection afterwards with nobody watching.
So the deploy that would most want a rollback point is the one guaranteed not
to have a fresh one. That is what the manual `app.db` backup above is for, and
why "take one before a deploy that carries a schema migration" is written as an
instruction rather than left to the schedule.

**The restore path has never been exercised, and this file will not pretend
otherwise.** Fly restores a snapshot by creating a *new* volume from it
(`fly volume fork` / `fly volumes create --snapshot-id`) rather than by
rewinding the one in place, so a real restore also means detaching the running
machine from the current volume and attaching the new one — which is downtime,
a machine edit, and a step nobody has walked here. Contrast the `app.db`
procedure above, which was executed end to end on 2026-08-13 and is written
down because it was. Until somebody does the same for a snapshot, treat these
as a safety net of unmeasured strength and keep taking the manual `app.db`
backup, which is the one with a proven restore.

The two are not redundant, either: the snapshot holds the whole volume
including the pool, and the manual backup holds the two irreplaceable things at
a moment you chose — which is the one you want before a schema migration.

### Watching it

```bash
fly logs
fly status
fly machine list
```

Memory is the number to watch. If you see OOM kills, the likely causes in order
are: a 25,000-game sweep, Argon2 memory during a login burst, or `data refresh`
holding the bulk file. Bump to 2 GB (~$11/mo) before you start optimising.

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
      also says where decks come from (a backup, your laptop, or an import —
      the image carries none since ADR 30). Not a build step and not a boot
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
      The Accounts page does what `mtglab users` does, and as of the login
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
        and the Promote/Demote buttons on the Accounts page. `set_admin` had
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
- [ ] **The Forge deployment shape** — local only, server-side, or a worker.
      Open in `ROADMAP.md`, and the section below is what it costs.
- [ ] **Fly or Hetzner.** §4 recommends Fly for the app alone. **If Forge goes
      server-side that recommendation flips**: 1 GB RAM is already the number
      to watch for DuckDB plus numpy, and Forge wants gigabytes of its own.
- [ ] **Whether Claude research is gated per user.** The interview is cheap
      (~$1–1.50 a draft on Sonnet 5); research is the unestimated half. ADR 15's
      stance dial doubles as the control, and *off* is a defensible default.

### If Forge goes server-side, this is what it adds

Measured during the feasibility spike, 2026-08-11. See
[FORGE.md](FORGE.md) for the mechanics.

| | Cost |
| --- | --- |
| Forge distribution in the image | ~470 MB unpacked |
| A JRE 17+ | ~190 MB more |
| JVM boot + card database | **~9s per `sim` invocation**, flat, amortises over `-n` |
| A heads-up game | median 4.6–6.8s across every archetype |
| The tail | **one Trostani game took 134s** |
| Four-player pods | **not yet measured, and heavier** — a pod was still running after ~4 min for 5 games |
| Heap used in the spike | `-Xmx4096m` |

Four things that are code changes, not just sizing:

- [ ] **`forge.profile.properties` must be baked at image build.** `run.py`
      writes it into the Forge install directory, because that is the only
      place Forge reads it from. A read-only mount decided later would break
      `ensure_profile` at runtime.
- [ ] **Generated `.dck` files are named for the deck slug in one shared
      directory.** Two concurrent runs race. Needs a per-run directory before
      more than one person can press the button.
- [ ] **Forge must run with its own directory as the working directory**, which
      constrains how the process is launched in a container.
- [x] **The licensing question is answered — and it is not a blocker.**
      Researched 2026-08-11; the reasoning is in [NOTICE.md](../NOTICE.md).
      **Forge is GPL-3.0, not AGPL-3.0**, so *running* it as a network service
      is not distribution and a hosted instance owes nobody source. This
      project stays MIT because `run.py` starts a separate process rather than
      linking. And being noncommercial is irrelevant to the GPL either way —
      its terms are identical sold or free. The one action that would trigger
      obligations is **publishing a container image containing Forge**, so:
- [ ] **Put Forge on the volume, not in the image.** The pool already lives
      there for a licensing reason of its own (Scryfall asks that bulk data not
      be redistributed), and Forge fits the same slot for the same shape of
      reason: an image that does not contain it cannot redistribute it. This
      also keeps the image small and makes a Forge upgrade a volume operation
      rather than a rebuild. Fold the download into the same manual runbook as
      `data refresh`. **If Forge ever does go into a published image instead**,
      pin the exact version and ship the corresponding source alongside it —
      do not rely on a written offer, which is poorly suited to registry
      distribution because there is no reliable way to present it to whoever
      pulls the image.

**The honest read right now:** local-only is unblocked and costs nothing.
Server-side is affordable on a Hetzner box and not on the 1 GB Fly instance
§3 prices. A worker is what the 134-second tail actually argues for, and
`api/jobs.py` already has the shape — but it is the most moving parts for a
feature nobody has asked for yet. **Measure the pods before choosing between
the last two.**
