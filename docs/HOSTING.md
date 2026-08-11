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
source of truth, it lives in git, and *deck history is git history* —
`git log -p decks/gyome-food/deck.yaml` is the swap record, and `swaps.md` is
literally a git diff. That is a real feature, and it is the thing that breaks
the moment other people have decks.

Friends' decks cannot live in your git repo. So you need a second storage path,
and the question is how much of the local-first model to keep.

### Recommended: two tiers, not one

| Tier | Storage | Who writes it | Keeps |
| --- | --- | --- | --- |
| **Curated decks** (your six) | `decks/*.yaml` in git, as today | You, locally, via CLI | Full local-first model, git-diff `swaps.md`, artifacts in the repo |
| **User decks** | SQLite on the volume, one row per deck | Logged-in users, via UI | Per-user isolation, no git |

Your six ship read-only inside the image and everyone can view them — that is
the showcase, and it is what makes the site worth logging into. Nothing about
your workflow changes: you still edit YAML locally, still run
`mtglab decks build`, still commit. Users get a UI-backed deck store that never
touches git.

**Why not put everything in the database?** You would lose the git swap record,
which is one of the few genuinely novel things this project does. Don't trade
it away to save one code path.

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

**No self-signup.** You provision accounts:

```bash
mtglab users add ada
mtglab users passwd ada
mtglab users list
mtglab users disable ada
```

This one decision deletes an enormous amount of infrastructure: no signup flow,
no email verification, no SMTP provider, no password-reset tokens, no bot
abuse, no CAPTCHA, no per-email cost. A forgotten password is you running
`mtglab users passwd`. For a site with fewer than a dozen known people, this is
the correct trade, and it is why the monthly bill can stay near zero.

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

---

## 2. Data model

Two embedded databases, zero managed services:

| Store | Engine | Contents | Access |
| --- | --- | --- | --- |
| `/data/mtg.duckdb` | DuckDB, 63 MB | Scryfall corpus, prices | Read-mostly, rebuilt by `data refresh` |
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

-- Sim results are a pure function of deck content + parameters. See §3.
CREATE TABLE sim_cache (
  key TEXT PRIMARY KEY, result_json TEXT NOT NULL, created_at TEXT NOT NULL);
```

`user_decks.yaml` stores the same YAML the file-backed decks use, so
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
objects and must never open the corpus themselves.

### The DuckDB locking rule, because it will bite you

The write lock is held by the **process**, not the connection, and DuckDB
allows exactly one writer. Two things follow, and an earlier draft of this
document got the second one wrong.

**Reads are fine, including across processes.** The API opens the corpus with
`read_only=True` in `service._connect()`, and read-only handles share happily:
verified with four separate processes querying the corpus simultaneously, all
succeeding. Within one process it is fine too — 12 concurrent requests against
`/api/decks` all returned 200, because single-worker uvicorn serves sync
endpoints from a threadpool. **`uvicorn --workers 2+` is therefore fine for
serving**, contrary to what this section previously claimed.

**Writes are exclusive, and that is the real constraint.** `db.connect()` —
the CLI path — executes schema DDL, so it opens read-write and takes the
exclusive lock. While `mtglab data refresh` is running, nothing else can open
the corpus read-write, and `service._connect()` deliberately swallows the
failure and returns `None` so the app degrades to "no corpus" rather than
500ing every request. Expect a refresh to make card lookups briefly
unavailable; that is by design, not a bug to fix.

The rule that does still bind: **sim pool workers must receive already-compiled
`SimCard` objects** rather than opening the corpus themselves, since each new
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

1. **Cache sim results by deck-content hash.** Biggest win, smallest change,
   and it makes the hosted UI feel instant.
2. **Precompute the standard sims when a deck is saved**, so the numbers are
   already warm when anyone opens the deck.
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
different class of container than anything costed here. And a Claude surface
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

```bash
brew install flyctl
fly auth signup     # or: fly auth login
```

You will also need a credit card on file; the machine sizes below are inside
Fly's paid tier.

### Step 1 — Dockerfile

The corpus is **not** in the image. It is ~63 MB built from ~98 MB of Scryfall
bulk, Scryfall asks that bulk data not be redistributed, and it belongs on the
volume where it survives deploys. The frontend bundle *is* committed to
`src/mtglab/web_dist`, so the image needs no Node toolchain.

```dockerfile
FROM python:3.12-slim

ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    MTGLAB_DATA_DIR=/data \
    MTGLAB_DECKS_DIR=/app/decks

WORKDIR /app
COPY pyproject.toml README.md ./
COPY src ./src
COPY decks ./decks
RUN pip install --no-cache-dir ".[api]" argon2-cffi

# Never bake the corpus in: Scryfall asks that bulk data not be
# redistributed, and it must persist across deploys on the volume.
EXPOSE 8080
CMD ["mtglab", "ui", "--no-open", "--host", "0.0.0.0", "--port", "8080"]
```

> Single worker on purpose — see the DuckDB locking rule in §3.

### Step 2 — fly.toml

```toml
app = "sylvan-library"
primary_region = "iad"          # pick the one nearest you

[build]

[env]
  MTGLAB_DATA_DIR = "/data"
  MTGLAB_DECKS_DIR = "/app/decks"

[mounts]
  source = "mtglab_data"
  destination = "/data"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "suspend"   # scale to zero between visits
  auto_start_machines = true
  min_machines_running = 0

[[vm]]
  size = "shared-cpu-1x"
  memory = "1gb"                   # 512mb is too tight: DuckDB + a sweep + Argon2
```

### Step 3 — create the app and volume

```bash
fly launch --no-deploy --name sylvan-library
```

```bash
fly volumes create mtglab_data --size 3 --region iad
```

3 GB leaves room for the 63 MB corpus, the raw Scryfall download during a
refresh, `app.db`, and backups. It costs about $0.45/month.

### Step 4 — secrets

```bash
fly secrets set SESSION_SECRET="$(openssl rand -base64 32)"
fly secrets set ANTHROPIC_API_KEY="paste-it-here"   # only once ADR 15's modes exist
```

Never put either in `fly.toml` or the repo. Fly stores them encrypted, injects
them as environment variables at runtime, and setting one triggers a redeploy —
so they are never in the image either. Rotating `SESSION_SECRET` logs everyone
out, which is the correct emergency response to a suspected session leak.

`ANTHROPIC_API_KEY` is read by the Anthropic SDK directly; the app never binds
it to a variable, and asks only whether it is set. Locally the same variable
comes from a gitignored `.env` (see `.env.example`) rather than from Fly.

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

The app will start with no corpus. That is expected — `/api/health` reports
corpus state rather than crashing, which is exactly the fresh-clone case the
API tests already cover.

### Step 6 — seed the corpus on the volume

This is the slow one: ~500 MB of downloads and several minutes. Run it as a
one-off against the running machine, **never** as a build or release step,
where it would blow the timeout.

```bash
fly ssh console -C "mtglab data refresh"
```

Verify:

```bash
fly ssh console -C "mtglab decks validate gyome-food"
```

### Step 7 — domain and TLS

```bash
fly certs add mtg.yourdomain.com
```

Add the CNAME Fly prints, then confirm:

```bash
fly certs show mtg.yourdomain.com
```

`force_https` in `fly.toml` handles the redirect. TLS is automatic and free.

### Step 8 — create your account

```bash
fly ssh console -C "mtglab users add aaron --admin"
```

Do this from a terminal you trust, and set the password when prompted rather
than passing it as an argument — command-line arguments land in shell history
and in the process table.

---

## 5. Running it

### Refreshing the corpus

Scryfall publishes daily; deck tooling does not need day-fresh data. Monthly is
plenty unless you are watching prices.

```bash
fly ssh console -C "mtglab data refresh"
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

### Backups

The corpus needs no backup — `data refresh` rebuilds it in one command. That is
the whole reason it is gitignored.

`app.db` is the irreplaceable part: users, sessions and every deck your friends
have written. Back it up with SQLite's online backup, which is safe to run
against a live database:

```bash
fly ssh console -C "sqlite3 /data/app.db \".backup /data/app-backup.db\""
fly ssh sftp get /data/app-backup.db ./backups/app-$(date +%F).db
```

Do not simply `cp` a live SQLite file — with WAL enabled you can capture a torn
copy. Keep these backups private: they contain password hashes.

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
3. **Sim result caching.** Biggest performance win, and it is pure
   infrastructure — no user-facing change.
4. **`app.db`, users table, `mtglab users` CLI.** No web login yet.
5. **Sessions, login, the scoped accessor, and the isolation test** from §1.
6. **User decks** in `user_decks`, reusing the existing YAML parser, gate and
   artifact generator.
7. **Process pool for sweeps** once anyone actually complains about the wait.

Stopping after step 2 is a perfectly good outcome if the multi-user part turns
out not to matter. Do not build auth until someone wants an account.
