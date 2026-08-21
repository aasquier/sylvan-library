# The plan: a Go backend for the sylvan library

*Flesh to metal, deliberately and with the digests to prove nothing else
changed.*

**Status: RATIFIED, 2026-08-21.** The seven decisions in §11 were ruled
with Aaron the same day the plan was drafted; each ruling is recorded
inline there, and [ADR 38](../adr/0038-the-served-backend-is-rewritten-in-go.md)
made them formal on the Phase 1 branch the same day (Appendix A was its
draft). **Phases 0, 1 and 2 are done and Phase 3 is in progress**; the port
board in §10 is the frontier. Written by Claude from a
measured read of the tree (see [BASELINE.md](BASELINE.md)); the judgment
calls were argued, then ruled.

---

## 1. What is being decided, and what was decided before

`docs/ENGINEERING.md` §1 deferred "a compiled backend" with a written
trigger, and its deferred-list row for Go reads: *"Only if the engine ever
becomes a standalone service rather than a library in-process. Not the
current shape."* That decision answered a narrow question — *should the Tier 1
hot loop be ported for speed?* — and its answer (no; and if ever, Rust-as-
embedded-library, not Go) was right and remains right. The measurements have
not moved: an 18-second cached batch job did not need a 30× speedup then and
does not now.

**This plan answers the other question: should the whole served backend be a
compiled service?** That is exactly the shape ENGINEERING's own table says Go
is for. And the honest framing, stated up front because this repo's
engineering doc is written for reviewers who ask *why did you do it that
way*: **the §1 trigger never fired. This migration is not demanded by a
profile.** It is the owner's architectural call — Aaron's, made 2026-08-21 —
bought for durability, static typing enforced by a compiler rather than by
mypy's discipline, true parallelism, a single static binary, and an
operational footprint the current 1GB machine strains against. The
measurements' role here is different: they are the **equivalence evidence** —
the digests, oracles and contract suite that let a full rewrite land without
the numbers changing out from under anyone. ENGINEERING §1 called
differential testing "the single highest-value item" a port could carry, and
built the 13,944-case mana oracle to be *"usable as the differential case set
for a port… in any language, forever."* This plan is that sentence cashed in.

What Go buys, concretely, on this app's own recorded facts:

- **The GIL concession goes away.** `api/jobs.py` runs one CPU worker because
  Tier 1 is GIL-bound (CLAUDE.md says so in as many words), and `fly.toml`
  documents why a second process is unsafe. In Go the CPU pool is a
  semaphore over goroutines; N sims use N cores of whatever machine exists.
- **The 1GB machine stops being tight.** `fly.toml` records 512MB as "too
  tight: DuckDB, numpy, an Argon2id hash… all want headroom at once." A Go
  binary + DuckDB comfortably fits where CPython + numpy + uvicorn did not.
- **Deploys shrink.** One static binary in the image instead of CPython plus
  a venv; boot-to-healthy (currently a per-merge downtime window, ADR 23)
  drops to process-start time. The Forge worker shim — deliberately stdlib —
  becomes a static binary too.
- **Dependency churn drops.** Weekly Dependabot groups carrying majors are a
  Python-ecosystem tax; Go's stdlib carries far more of this app.
- **The compiler holds what tests currently hold.** Strict mypy took months
  to reach an empty exception list; in Go that floor is the language. The
  race detector and native fuzzing are new instruments the suite has never
  had.

And the cost, stated with the same plainness: **~38,000 lines of Python and
the meaning of ~36,000 lines of tests, ported under active development,
across working days that are therefore not building the simulator roadmap.**
§7 prices those days against the repo's measured velocity; §10 and §11 are
where the trade gets ruled on.

## 2. Scope — and the answer to "does Python gain us anything?"

Aaron's carve-out: migrate *unless Python genuinely gains us something we
can't replicate in Go*. Measured against the tree, the answer splits clean
along a boundary the repo already enforces:

**The served app migrates — nothing in it is irreplaceable.** Everything the
container runs: `api/`, `auth/`, `decks/`, `sim/`, `cards/`, `claude/`,
`artifacts/`, the reference-prose modules, `mana.py`, `tarot.py`,
`symbols.py`, `ocr.py`, `caches.py`, `config.py`, and the runbook half of
`cli.py`. Every dependency has a Go answer: DuckDB (go-duckdb, CGO),
SQLite, Argon2id (PHC-format verify, so **existing password hashes keep
working**), the official `anthropic-sdk-go` (verified 2026-08-21 against the
claude-api skill: tool use, response schemas, `web_search_20260209`, prompt
caching, and the same manual `pause_turn` resume our `converse` already
does). Tier 1 needs no numpy — measured today, `import numpy` appears only
under `animist/`; the engine is stdlib `random.Random`, which is what makes
§5's exact-equivalence plan possible.

**The dev bench stays Python — this is the genuine carve-out.** `animist/`
(Pillow + imageio-ffmpeg), `cardmotion` build (torch behind the `depth`
extra), `bench/`, `mutate/` — ~4,400 lines that never ship in the image, a
boundary ADR 29/32 and `test_packaging.py` already hold. Torch has no Go
path at all, Pillow has no Go peer worth chasing, and porting offline asset
tooling buys zero user value. These stay exactly where they are, in a Python
package that outlives the served app's port as dev tooling. (`mutate/` is
Python-source-specific; its Go counterpart is the race detector, fuzzing,
and optionally a Go mutation tool — §8.)

**Also out of scope, permanently:** `web/` (untouched — a stated invariant,
§3), Forge itself (a JVM either way), and the deployed data model (`app.db`
schema, `deck.yaml` format, the volume layout — bytes on disk do not change).

## 3. Invariants — what "the same app" means

The port is done only if all of these hold. Each becomes a test, not a
sentence:

1. **Zero changes under `web/src`.** The committed bundle keeps working
   against the Go backend unmodified. That pins wire shapes exactly — field
   names, job-lifecycle states, `auth_required`/`authenticated`, and the
   error envelope: `lib/api.ts` reads `detail` off every non-2xx body
   (`api.ts:1130`), so Go speaks FastAPI's envelope, 422 validation shape
   included, forever or until the frontend is deliberately changed later.
2. **The middleware properties survive re-proof.** Deny-before-route outside
   `PUBLIC_PATHS`; `/api/admin` refused 403 to non-admins before routing
   (ADR 17); everything owned-by-another-person is **404, never 403**
   (ADR 5); rate limits answer 429 with `Retry-After`. The
   `test_isolation.py` classification becomes shared data both languages'
   suites consume (§8), so there is exactly one table.
3. **The claude/ boundary is re-enforced structurally, not re-promised.** No
   path under the Go claude package may name a deck-write function
   (`test_claude_boundary.py`'s AST walk becomes a `go/analysis` pass over
   typed call graphs — stronger than name-matching); response schemas still
   have no field for a rationale, a defence, or a card name where ADRs
   15/25/34 forbid one.
4. **Rule 4's edge holds:** no code path passes a model response into a
   card's `why`. Same enforcement class as (3).
5. **The gate's verdicts are bit-identical** on the oracle case sets, and
   Tier 1 reproduces the pinned `REFERENCE_DIGEST` (§5) — a seeded run means
   the same games after the port as before it.
6. **Surgical edits stay surgical** (ADR 12): a one-card swap is a one-card
   diff, proven the same way — text surgery, verified against a
   parse-mutate-dump oracle, refused on mismatch.
7. **Sessions and passwords survive the cutover.** Argon2id PHC hashes
   verify as-is; the session-token scheme is matched so nobody is signed
   out. (If the token scheme resists matching, the fallback is a one-time
   global sign-out, taken deliberately — §11.)
8. **Commandment 10 holds in the new language:** no technology name reaches
   a user. "Go" appears in docs and CI, never in copy — the port must not
   change a single rendered word.

## 4. The mechanism — a strangler, because main deploys itself

Two ways to run a rewrite. A **big-bang branch** holds the Go app aside until
it passes everything, then cuts over once. On this repo it fights every
recorded practice: main auto-deploys on merge (ADR 23), 215 PRs have landed
at a pace that would fork any long-lived branch, and another Claude is
actively landing work. Rejected.

**Chosen: the Go binary becomes the front door early, and eats the app one
route family at a time.**

```
                    ┌──────────────────────────────────────────┐
   :8080 ──────────▶│  Go front door (one binary)              │
                    │  auth middleware · static web_dist       │
                    │  ├── ported families ──▶ Go handlers     │
                    │  └── everything else ──▶ reverse proxy   │
                    └───────────────┬──────────────────────────┘
                                    ▼ loopback
                            uvicorn (FastAPI app, shrinking)
```

- The first Go PR ships a binary that serves `/api/health` (a second,
  Go-specific health path during coexistence) and proxies everything else to
  uvicorn on loopback. From that day, every merge deploys a working pair.
- **The auth middleware ports first** and runs in the front door for *all*
  traffic — deny-before-route in front of both runtimes, one enforcement
  point, no route accidentally reachable during the interregnum. Python's
  middleware stays on as a second wall until retirement (defense in depth,
  costing microseconds).
- Route families flip by moving their prefix off the proxy list. **A family
  is atomic**: a job-shaped feature's submit and poll flip together, because
  `api/jobs.py`'s registry is in-process and a job born in one runtime is a
  404 in the other (`fly.toml` documents this exact property). The CPU/NET
  pools may exist per-runtime during coexistence (they are throttles, not
  correctness); the **FORGE lane may not** — one lane, one runtime, at every
  moment, so two submitters can never race the worker or the `.dck`
  directory.
- Both runtimes share the volume: same `app.db` (SQLite in WAL mode with a
  busy timeout — the coexistence cost is two writer processes), same
  `/data/decks`, same DuckDB pool opened read-only per-connection as today.
- The image temporarily carries both runtimes (a third Dockerfile stage;
  entrypoint starts the Go binary, which supervises uvicorn as a child).
  **The image gets bigger before it gets smaller** — stated now so nobody
  reads the interim size as the outcome.
- Retirement (Phase 8) deletes the proxy, the Python stage, and the served
  Python — and the repo keeps a `python/`-side dev package for the bench.

## 5. The equivalence strategy — how a rewrite proves itself

The project's standing lesson is that a green suite is a claim about what the
suite read. The port gets four instruments, cheapest first:

1. **The contract suite** (Phase 1). The existing HTTP tests grow a mode that
   targets a live base URL instead of an in-process TestClient — isolation
   matrix, auth flows, route shapes, error envelopes. Runs three ways: against
   Python alone (proves the harness), against the coexistence pair on every
   flip, and against Go alone (the retirement gate). Seeding uses the same
   Python helpers pointed at the shared scratch `MTGLAB_DATA_DIR`, so the
   harness needs no Go-side fixtures. This phase is pure Python work that
   hardens the current app even if the port were later abandoned — it is
   deliberately the first thing built.
2. **The enumerated oracles, ported as data.** The mana oracle's 13,944
   (cost, pool) rows dump to JSON Lines today; the Go solver must match the
   pinned `POOL_ANSWER_DIGEST` answer set case-for-case. `test_mana.py`'s
   hand-pinned traps port as table-driven Go tests. Hypothesis's generative
   role is covered on the Go side by native fuzzing against the same
   brute-force/Hall's-theorem oracles, re-implemented once in Go.
3. **`pyrand`: CPython's `random.Random`, bit-exact, as a small Go package.**
   Tier 1, the wheel, and the tarot deal all draw from seeded
   `random.Random` — Mersenne Twister plus documented, stable `shuffle`/
   `_randbelow`/`sample` algorithms, all reimplementable exactly (MT19937 is
   specified; CPython's shuffle is Fisher–Yates over `_randbelow`). Done
   right, **the Go engine reproduces `REFERENCE_DIGEST` byte-for-byte**, the
   same seeds replay the same games, a client-held tarot seed deals the same
   three cards across the cutover, and the ADR 18 sim cache stays *coherent*
   (its keys carry an engine-source fingerprint, which changes — cached rows
   re-key honestly — but a re-run under the old seed still answers
   identically, which is what "seeded" promised users). This turns the
   scariest port into a checked one. If exactness stalls, the fallback is
   statistical equivalence (same decks, N=50k, probabilities within CI
   bounds) plus re-pinned digests — but exactness is the plan, and it is
   ENGINEERING §1's differential-testing dream with a better punchline than
   Rust-vs-Python ever offered.
4. **The closed forms match to float tolerance.** `sim/karsten.py` and
   `sim/curve.py` are `math.comb` hypergeometrics and expectations — no
   sampling; Go must agree to within an epsilon pinned per function.

Plus commandments 14 and 16 unchanged: every flip is walked on the deployed
pair before it is called done, and anything Aaron's eye can see changed (there
should be nothing) goes before his eye first.

## 6. Toolchain choices (proposed, Aaron ratifies)

| Concern | Choice | Why |
| --- | --- | --- |
| Go version | current stable at Phase 2 | **Spike caveat:** this Mac is at its OS ceiling (macOS 12, Intel). Go's minimum macOS has been climbing (1.23 required 11+; the floor has since moved again). Verify current Go runs on this Mac in Phase 0; if the floor has passed 12, pin the newest Go that supports it and note the runway. CI is Linux either way. **Answered 2026-08-21:** the floor has passed 12 — Go 1.27 (August 2026) requires macOS 13, and the 1.26 release notes say 1.26 is the last that runs on macOS 12 (both read from go.dev the same day). So the module pins **`go 1.26`**; `go1.26.7` was installed here via the official per-version installer (`~/sdk/go1.26.7`, the stock `/usr/local/go` 1.20.7 untouched) and runs. The runway is roughly until Go 1.28 ships (~February 2027), after which 1.26 leaves support and this Mac's Go work moves to the CI/container loop or a newer machine. |
| Module layout | `go/` at repo root | Mirrors `web/`: a source directory beside the Python package, building an artifact the image ships. One module, packages inside mirroring today's map (`go/internal/mana`, `go/internal/sim`, …). **Built 2026-08-21:** module path `github.com/aasquier/sylvan-library/go`; `cmd/mtglab` (cobra root, one command: `ui`, the front door), `internal/door` (the door), `internal/auth` (app.db read-only, sessions, argon2id), `internal/routes` (the reader of `tests/contract/routes.json`), `internal/pool` and `internal/deckyaml` (the two spikes, as packages). |
| HTTP | stdlib `net/http` (1.22+ mux) | The dependency-light ethos, applied. The deny-before-route middleware is a handler wrapper; no framework earns its place here. |
| DuckDB | `github.com/marcboeker/go-duckdb` | The community-standard driver; CGO with bundled libduckdb. Spike in Phase 2 for macOS-12 dev + CI arm64. Its Appender is the bulk path the 16-minute `load_printings` wants anyway — the refresh fix rides along. **Spiked 2026-08-21, and the driver has moved:** it is `github.com/duckdb/duckdb-go/v2` now (the same project, under the DuckDB organisation; marcboeker's path last published October 2025). v2.10505.0 bundles libduckdb **1.5.5, the same version the venv's `duckdb` is**, links on this Mac (Apple clang 12 prints weak-symbol warnings against the prebuilt static library — harmless, the tests pass), reads a pool Python wrote (`MTGLAB_TEST_POOL=… go test ./internal/pool`), and builds on both CI architectures in the `go` job. The front door does not import it yet and is built CGO-free (the Dockerfile's door stage says why); the `go` job proves that build stays possible. |
| SQLite | decide at spike: `modernc.org/sqlite` (pure Go) vs `mattn/go-sqlite3` (CGO) | CGO is already required by DuckDB, so either works; modernc keeps DB-free unit tests CGO-free. Whichever is chosen, WAL + busy_timeout from day one (two processes share `app.db` during coexistence). **Decided at the spike, 2026-08-21: modernc.org/sqlite** (v1.57.0). It makes the front door a static binary with no C toolchain in its build, keeps the auth tests race-detected without one, and the cost — modernc is slower than mattn on heavy work — is nothing at a primary-key session lookup. The door opens `app.db` **read-only** (`mode=ro`, busy_timeout 5000): Python's middleware behind it still does the session touch and the expired-row delete, so during coexistence there is still one writer and risk 6 is deferred rather than taken. |
| YAML | `github.com/goccy/go-yaml` for the *oracle parse only* | The load-bearing discovery: `decks/edit.py` is hand-rolled **text surgery**, ruamel was measured and rejected in its own docstring, and text surgery ports language-neutrally. Go needs YAML only to parse (validate/oracle-check), never to serialize a deck. Spike a golden-deck round-trip against PyYAML's parse early anyway — parser equivalence (folded scalars, anchors) is the residual risk. **Spiked 2026-08-21, and it agrees:** `tests/go_fixtures.py` writes a deck built to carry every shape `Deck.dump` can emit (single-quoted scalars folded at width 100 with doubled apostrophes, plain multi-line scalars with braces in them, quoted look-alikes of `yes`/`null`/`12`, a newline inside a quoted scalar, unicode), plus PyYAML's reading of it as JSON; `go/internal/deckyaml` parses the text with goccy (v1.19.2) and matches value for value. `tests/test_go_fixtures.py` holds the committed pair current against the dumper. |
| Argon2id | `alexedwards/argon2id` (PHC parse + verify over `x/crypto`) | Existing hashes carry the OWASP parameters in-string; verification is compatibility, not migration. **Proven 2026-08-21** (`go/internal/auth`): hashes argon2-cffi wrote verify, hashes Go writes are in the PHC form Python reads, the token hash is the same SHA-256 hex, `isoformat` timestamps parse, and — the strongest form — a session `mtglab.auth.sessions.create` minted resolves in Go (`MTGLAB_PYTHON=.venv/bin/python go test ./internal/auth`), which the `contract` job then proves over the wire on every pull request. |
| Anthropic | official `anthropic-sdk-go` | Verified current: tool use, structured outputs, `web_search_20260209`, caching, manual `pause_turn` loop. Model stays `claude-sonnet-5` with per-account tiers — the port changes no model decision. Load the claude-api skill before writing the integration, same rule as ever. |
| CLI | `spf13/cobra` | `mtglab` has ~40 subcommands; stdlib flag-wrangling at that scale is its own framework, worse. The one heavyweight dependency argued for, not defaulted to. **Ratified 2026-08-21 as Aaron's explicit call: cobra for any CLI work, full stop.** |
| Tests | stdlib `testing` + `google/go-cmp`; native fuzzing | Ethos again. Table-driven ports of the pinned cases. |
| Lint/CI | `golangci-lint` (errcheck, staticcheck, govet, revive) + `go test -race` + `go vet`, as **required checks** | And the recorded lesson applies verbatim: *adding a CI job is two steps* — writing it and requiring it in branch protection, which has no artifact in the repo (ENGINEERING §5). Phase 2's exit gate includes reading the protection list back. **Done 2026-08-21:** three jobs — `go (amd64)` and `go (arm64)` (vet, the CGO-free door build, race-detected tests, a tidy check, on native runners) and `go-lint` (golangci-lint v2.13.1, config in `go/.golangci.yml`) — written and required; ENGINEERING §5's table is the read-back. One trap for this Mac: `go install` of golangci-lint fails at link with CGO on (Apple clang 12 against Go 1.26's cgo), and `CGO_ENABLED=0 go install …@v2.13.1` works. |

## 7. Phases

Effort is priced against this repo's **measured velocity, never a
conventional team's** — a correction Aaron made to this plan's first draft
on 2026-08-21, and the measurement that grounds it: the entire project —
42k lines of backend, 39k of frontend, ~37k of tests, 37 ADRs, the deploy
and the ledger — was built from an empty directory in **twelve days**
(first commit 2026-08-10; 220 squash merges on main, a sustained ~18–19 a
day, consistent with ENGINEERING §5's 17.4/day measured on day five). A
port of ~38k of those lines, with every design decision already made and
the equivalence instruments already built, prices **at or below one
build-cycle of the surface it replaces** — the from-scratch version cost
roughly half of those twelve days once the frontend, research and art are
subtracted, and a port trades design work away for verification work.

Running total: **~4–7 working days at the demonstrated cadence as the main
line** — about one build-week — and two to three times that calendar
interleaved with feature work; §11 decision 2 is which. Two things about
that band, both learned correcting this paragraph (twice, both times by
Aaron holding it to the history, 2026-08-21). First, the calibration and
the total must agree: the from-scratch backend share of the twelve days was
roughly five or six of them, design included, so a port with the design
done and the oracles in hand prices at or under that — a band above it was
this plan contradicting itself. Second, CI waits and Aaron-walked releases
are **already inside** the measured ~19 merges/day — the back half of those
twelve days ran at full pace with commandments 14 and 16 in force — so they
widen nothing. What is genuinely new to a port, and the whole width of the
band, is the spike tail: CGO on this Mac, the `pyrand` digest chase, the
YAML oracle, each an hours-to-a-day item with a named fallback. The
estimate meets reality at the end of Phase 2 (~1–1½ days in): the spikes
are resolved and the first flips have landed, and the remaining phases get
re-priced against those actuals rather than argued further. Per-phase
figures below share this calibration; each phase names its **exit gate**,
and no phase starts before the previous gate is green.

**Re-priced 2026-08-21, after Phase 2.** Phases 0–2 took about one working
day together — the plan and baseline in one session, the contract harness
in one, and Phase 2 in **~75 minutes** against its ½–1 day (the clock, not
a feeling; the paragraph under Phase 2 has the timestamps). That phase was
a risk budget and the risk did not arrive, which argues for the low end of
every band without licensing anything lower: the rest of the port is priced
by lines, not by spikes. So Phases 3–8 are re-stated at the low end of each
figure below — **~3¾ working days** — and the whole port reads **~4–5 days**
rather than 4–7. Phase 3 started at 23:10 UTC the same day (branch
`go-read-spine-prose`), noted here so the next re-pricing can subtract
rather than remember.

**Phase 0 — Baseline and ratification** *(done 2026-08-21)*. BASELINE.md
captured, and its quiet-window checklist closed in a second dated block the
same day (image 121MB compressed / ~325MB unpacked; idle RSS 127MB, peak
215MB; machine-update-to-healthy ≈23s; clean dev install 79s; one item —
the bench re-measure — deliberately not taken on a busy machine, per the
ledger's own rule); Go-on-macOS-12 verified (§6: 1.26 is the last, and it
runs here); this plan argued over with Aaron, amended, and ADR 38 landed on
the Phase 1 branch. *Gate: Aaron's yes, in writing, in the ADR — met.*

**Phase 1 — The contract harness** *(done 2026-08-21, in one session;
pure Python, no Go).* Built as `tests/contract/` (its README is the map):
a **contract suite** that runs three ways — in-process `TestClient`
(inside the ordinary `pytest`), `--live` (the harness seeds a scratch,
starts `mtglab ui` on it as the container's `CMD` does, and drives it
over TCP), and `--base-url` against a server somebody else started on a
directory the same seeder fills, which is the coexistence mode Phase 2
needs; the **route classification extracted to `tests/contract/routes.json`**,
read by `tests/test_isolation.py` (which now also holds it equal to
`api/auth.py`'s allowlist), by the contract suite, and — from Phase 2 —
by the Go module; **golden wire shapes** (status, content type, pinned
headers, body *shape* — keys and kinds, never data) for 168 checks across
six families, success and refusal paths, recorded with `--update-golden`;
and a **`contract` CI job** running the live mode. *Gate: met and proven
by mutation* — `tests/test_contract_harness.py` runs every assertion
against an app broken in one of eight ways (a renamed `detail`, an
allowlist that grew, a 403 turned 404, a 404 turned 403, a dropped field,
a drifted status, a missing header, a 429 without `Retry-After`) and shows
it raise, then runs the whole suite once, as wired, against the envelope
mutation and shows it go red across the protected sweep. One honest
narrowing: the plan said "the existing HTTP tests grow a mode"; what was
built is a *suite* that can run against a live server, because the other
thirteen HTTP test files reach into process state (`jobs.submit`,
`MemoryDeckSource`, monkeypatched writers) and could not. They keep doing
what they do; the contract suite is the live-capable set, and the
adversarial matrix is written in both. **Owed:** requiring the `contract`
check on `main` (a repository setting, Aaron's — ENGINEERING §5).

**Phase 2 — Skeleton and front door** *(~½–1 day; the spike day — if the
plan snags on CGO or the toolchain, it snags here, cheaply, and the
whole-plan estimate is re-priced on this phase's actuals).* `go/` module;
CGO spikes (duckdb, sqlite, yaml-parse) pass on this Mac and both CI arches;
the binary serves its health path, serves `web_dist` static, proxies the
rest; **auth middleware ported with the shared classification table**, plus
sessions/argon2id verify compat proven against fixtures Python wrote; new CI
jobs written *and required*; third Dockerfile stage; deploy of the pair.
*Gate: contract suite green through the front door on the deployed instance;
a session minted by Python authenticates a Go-served request.*

*Done 2026-08-21, in one session (branch `go-front-door`).* The three
spikes passed on this Mac in the first hour (§6 records each, with the one
surprise: the DuckDB driver had moved house). The door is `go/internal/door`
and the binary is `go/cmd/mtglab ui`: the ported middleware (deny before
route, 401/403, the same path normalisation, `PublicPaths` held equal to
`routes.json` by a Go test the way `test_isolation.py` holds Python's), the
shell and the two static mounts served with the container's content types
and Python's exact refusals (a JSON `Not Found`, a 405 on a `HEAD /`, the
shell for `/assets` without its slash), a reverse proxy that preserves
`Host` and the raw query and **drops client-supplied `X-Forwarded-*`**
(uvicorn trusts loopback, and the door is loopback now), `/door/health`
for the door's own liveness (outside `/api`, because a Go-only `/api` route
would be a ghost to `test_isolation.py`; `/api/health` stays the *pair's*
health and stays proxied), and a supervisor that runs the Python server as a
child and exits with it. The **contract suite runs through the door**: 169
of 169 locally, and the `contract` CI job now builds the door and runs the
suite `--base-url` against it on every pull request — which is where the one
real bug of the phase was found, on the first run: the door pre-set the
security headers and the proxy appended Python's copies, so the wire said
`nosniff, nosniff`. Fixed by applying them at `WriteHeader` time; the Go
test now asserts single values rather than `Get`'s first. The image gained
its third stage (`golang:1.26-trixie`, CGO off, the binary at
`/opt/door/mtglab` — off PATH, so `fly ssh console -C "mtglab …"` still
finds Python's for the runbook) and its CMD runs the door with the Python
server after `--`; `tests/test_packaging.py` pins the CMD's shape, the
`go 1.26` pin, and that the Go reader points at the one route table.
**Actuals against the estimate, measured rather than felt:** the estimate
was ½–1 day; the clock says **about 75 minutes of one session** — the first
read of the tree ~21:25 UTC, PR #220 merged 22:27, release v156 live 22:33,
walked on the instance by 22:39 — from first read to deployed-and-walked.
(This sentence said "about three hours" until the Phase 3 session read the
timestamps back on 2026-08-21; that figure was a guess written before the
clock was, and it was wrong by ~2.5×.) What the number says about the rest
is narrower than it looks: Phase 2's estimate was a **risk budget** — the
spike day — and the spikes passed in the first hour, so most of the estimate
was reserved for trouble that did not arrive. The remaining phases are
priced by *lines ported*, which that pace does not scale; the re-pricing in
the paragraph above the phase list takes the low end of each phase's band
and says so, and Phase 3 is the first line-driven phase, so its actual is
the next calibration point.

**Phase 3 — The read spine** *(~¾–1 day; re-priced ¾).* `config`, `cards/db` reads,
`DeckSource` (file + SQL tiers), the gate (`validate`, companion, partners),
`analyze`, `suggest`, search, glossary/colors/lore/tarotlore served from
**generated JSON both runtimes share** (generator + drift check in CI, the
`web_dist` pattern; the JSON becomes authoritative at retirement), symbols
and OCR shelf serving with SHA-256 pins, cardmotion serving routes. Flip the
read-only families as they finish. *Gate: per family — contract green on the
pair, plus the ported unit tables green; validate agrees with Python on
tiny_pool's deck and the template decks case-for-case.*

*In progress from 2026-08-21 23:10 UTC.* **The flip mechanism and the first
family landed first**, deliberately the family with no pool behind it so the
cycle — table, handlers, contract run through the door, deploy, walk — was
proven at the lowest stake: `go/internal/api` is `src/mtglab/api` one
family at a time, `go/internal/wire` writes FastAPI's envelope (compact
separators, unescaped HTML, the 422 validation list), and the door's
`routes.go` answers a ported route only for a **canonical** request — the
raw path already normalised, no escape that moves a segment, the method
matching — and proxies everything else as it arrived, so a doubled slash,
a trailing slash or a `POST` to a Go-served `GET` still gets Python's own
answer (the router's own 404/405 are Phase 8's to write). The prose moved as
**generated JSON**: `mtglab.reference` renders exactly what `service.
color_taxonomy`, `service.glossary` and `/api/themes` serve (a test pins each
equality), `tests/go_fixtures.py` writes the five files into
`go/internal/reference/data/`, `tests/test_go_fixtures.py` holds the
committed bytes to a fresh render, and `go/internal/reference` embeds them
and compacts them once at start — so `/api/colors`, `/api/glossary` and
`/api/themes` are **byte-identical** from either door, checked on the
laptop pair, not only shape-identical under the contract suite. The
`lore.json` and `tarotlore.json` carry names for the pool-backed routes
that follow. The door test `TestEveryPortedRouteIsInTheSharedTable` is the
ghost guard for this direction: Go may serve only a path `routes.json`
already classifies.

**The pool came second, and the door became a CGO build.** `go/internal/pool`
is `cards/db.py`'s read side over `github.com/duckdb/duckdb-go`: the file
opened read-only **on a lease** — opened at the first ask, handed back after
ten idle seconds (`service._KEEPER_IDLE`, shorter than the platform health
check's cadence for the reason `api/service.py:_pin` argues: a held read-only
handle is a held shared lock, and a door that kept the pool open forever
would refuse every `data refresh` on the instance), re-opened when the file's
stamp moves, with `get_cards` memoised per open the way Python memoises it
per stamp. `GetCards` keeps Python's precedence (exact full name over a face
name; Ajani, Nacatl Pariah by its white front still reports {R}{W}), and the
schema Python runs is embedded verbatim (`pool.Schema`, written by
`tests/go_fixtures.py` beside the 21-card `tiny_pool` as rows, so the Go
tests build a real pool in CI where there is no Python). `go/internal/gate`
opened with `partners.py` (search's `commanders_only` decides with the same
`CanBeCommander` the gate will), `go/internal/cards` is the camera reader
(`identify.py`: a corner resolves, a title only offers), and the second
family flipped: `/api/cards/search`, `/api/cards/identify`,
`/api/colors/{key}` and `/api/lore`. Two things the flip surfaced. The route
table learned that a literal Python still owns can sit beside a template Go
answers — FastAPI declares `/api/colors/progress` before `/api/colors/{key}`
— so the table matches the most specific pattern and the API **reserves**
the literals it has not ported (`api.Proxied`), both held to `routes.json`
by the door's test. And on this Mac the CGO door needs
`CGO_LDFLAGS="-Wl,-U,_SecTrustCopyCertificateChain"` to link at all: Go's
`crypto/x509` references a macOS 12 Security API the Xcode 12 SDK here does
not declare, which the CGO-free door never asked clang about (the same
missing symbol that made golangci-lint a `CGO_ENABLED=0` install in Phase
2). Linux — CI and the image — never sees it; CLAUDE.md's toolchain
paragraph carries the flag.

**Phase 4 — Writes and the log** *(~¾–1 day).* The edit operations
(text surgery + oracle verification, ADR 12's five rules re-proven),
`_commit` + the activity log (ADR 28: one call site, `record` never raises,
no rationale text), create/import/promote/delete, notes, labels (ADR 37's
worst-piloted reading — server-side only, as the deck page already assumes),
admin routes, invites/resets (EmailSender seam; no test sends mail),
rate limiting. *Gate: golden-deck edit equivalence — every operation applied
by Go over fixture decks yields byte-output Python's operation also yields;
ADR 5/16/17 matrices green via the contract suite.*

**Phase 5 — Jobs and the simulator** *(~1–1½ days; the digest chase
carries the tail risk).* The registry
(pools → semaphores; `key=` dedupe per owner in one locked step;
born-finished jobs; the FORGE single-lane rule), then `pyrand`, then Tier 1
against `REFERENCE_DIGEST`, karsten + curve to tolerance, the mulligan grid,
land sweeps, the sim cache (ADR 18 keys with a Go-source fingerprint;
`deck_check` attached after the cache exactly as today), `NothingToSimulate`,
and the job families flipped. **The GIL dividend lands here and gets
measured** — same machine, N-core Tier 1 scaling into Appendix B. *Gate:
`REFERENCE_DIGEST` reproduced (or the statistical fallback consciously taken
and re-pinned — an ADR-worthy divergence, not a shrug); the mana oracle's
13,944 cases match the pinned digest; contract green.*

**Phase 6 — The Claude surfaces** *(~¾–1 day; claude-api skill loaded
first).* The pipe (`converse`: caching breakpoints, container-id ride-along,
`pause_turn` resumed-never-returned), stance + `/api/claude` per-surface
defaults, personas, all seven modes with their schemas' deliberate absences
intact, source-checking (`keep_sources`, drop-and-count), the tarot table's
server half (seeded deal via `pyrand` — same seed, same spread, across the
cutover), the usage ledger, and the six job-shaped routes flipped. *Gate:
boundary analysis pass in CI (invariant 3); every mode's structural tests
ported; a real conversation driven on the deployed pair for each mode —
rendering a value audits it, per the recorded lesson.*

**Phase 7 — Forge, scan, and the ledger** *(~¼–½ day).* `sim/tier3` wire
+ worker client (Machines API; creation stays in the deploy workflow), the
gate/refusal split, per-game progress streaming, the match ledger (ADR 36:
snapshot labels at match time, record from exactly two places), scanruns
(ADR 34: schema with no name field), coverage checked twice, medians-and-
tails reporting. The shim itself re-ships as a static Go binary on the
worker (its idle self-stop behavior preserved). *Gate: a real hosted match
on the instance, bar ticking, ledger row written — photographed, commandment
14.*

**Phase 8 — Retirement and the comparison** *(~¼–½ day).* Python leaves
the image and the request path; entrypoint runs the binary alone; `mtglab`
(Go) covers the runbook surface (`users`, `decks`, `sim`, `data refresh` —
now via Appender, closing the ledger's queued 28-minute item, measured
before/after); docs swept (CLAUDE.md architecture map, HOSTING, ENGINEERING
§1 annotated, memory updated); the dev-only Python package renamed/trimmed
so nothing can quietly import it server-side; **Appendix B filled in** —
LoC, tests, perf warm/cold, RSS, image size, boot, CI wall, N-core scaling —
beside BASELINE.md's numbers. *Gate: contract suite green against Go alone
on the deployed instance; the comparison table published; a full
walk-through of every surface by Aaron.*

## 8. Enforcement parity — the immune system ports too

This repo's recurring lesson (found four times: *a rule enforced by nothing
drifts*) means behavior-porting is half the job; the guards are the other
half. Inventory, with each guard's Go answer:

| Python guard | What it holds | Go answer |
| --- | --- | --- |
| `test_isolation.py` route classification | every route declared public/user/admin/shared; sweep refuses unclassified | classification extracted to shared data (Phase 1); Go test generates the same sweep from the same file; middleware built from code, tested from data |
| `test_claude_boundary.py` (AST walk) | nothing under claude/ names a deck write | `go/analysis` pass over the typed call graph, in CI — stronger (types, not names) |
| interview/argue/scan schema absences | no field for a `why`, a defence, a card name | struct types without the field + `additionalProperties:false` schemas; table test asserts marshaled schemas lack the forbidden keys |
| `test_packaging.py` extras/doc pins | dev covers the suite; depth stays out; docs name every extra | Python side keeps it (the extras remain, dev-only); Go side: `go.mod` tidy check + image-content greps unchanged |
| `caches.py` register sweep | no memo without a counter; a dead cache is visible | port the register; sweep by convention + a lint pass flagging package-level maps in served packages that never register (weaker than Python's introspection — accepted, noted) |
| skip-count gate | the suite cannot quietly shrink | `go test` has no skips by default; keep the Python gate for the remaining Python suite; contract suite counts its own tests against the classification table |
| coverage floor 95 | large regressions are loud | `go test -cover` with a floor set from Phase 3's measured baseline, raised deliberately, never drifted (same words as pyproject's comment) |
| mutation testing (`mtglab mutate`) | does the suite notice wrongness | race detector + fuzzing (new instruments) now; a Go mutation harness (e.g. gremlins) evaluated in Phase 5 when the sim lands — the module where it earns most |
| `SIMULATOR_KEYS` glossary seam | renamed key fails a test, not a tooltip | keys move to Go; a Go test reads the TS file the way the Python test does today |
| secrets/card-data CI scans | nothing sensitive tracked or baked | unchanged — language-independent |
| deck-location drift tripwire | no prose claims decks live in git | unchanged; sweep extended over `go/` |

## 9. Risk register

Ordered by expected pain, each with the mitigation already in the plan:

1. **Porting under active development** — the moving target. *Mitigation:*
   strangler on main (never a long fork), family-atomic flips, the freeze
   protocol in §10, and the contract suite as referee. This is the risk that
   kills rewrites; it gets the process, not a hope.
2. **YAML parser divergence** under the edit oracle (PyYAML vs goccy on
   folded scalars, width, anchors). *Mitigation:* Phase 2 spike with golden
   decks; the surgery itself never serializes; worst case the oracle parse
   gets a strictness shim, and the refusal path (edit refused, nothing
   written) already fails safe.
3. **`pyrand` exactness stalls** (an undocumented CPython corner).
   *Mitigation:* MT19937 + shuffle + `_randbelow` are specified and stable
   across CPython 3.11/3.12 (the digest already proves cross-version
   stability); fallback to statistical equivalence is pre-declared with its
   own gate so it cannot happen silently.
4. **CGO friction** — go-duckdb on macOS 12 Intel dev, arm64 CI, image size.
   *Mitigation:* earliest spike in the plan; native runners already build
   both arches; if this Mac cannot build, dev falls back to the container/CI
   loop for db-touching packages and the plan says so out loud.
5. **`anthropic-sdk-go` beta-surface lag** (server-tool details, container
   reuse). *Mitigation:* verified current today; wire-level escape hatch
   exists; Phase 6 budgeted with slack; behavior pinned by our own tests,
   not by SDK trust.
6. **Two writers on `app.db`** during coexistence. *Mitigation:* WAL +
   busy_timeout from Phase 2; write-owning families flip atomically so no
   table has two owners; the ladder (forward-only, applies on boot) migrates
   from exactly one runtime — the Python one until Phase 8, a rule not a
   habit.
7. **Session/auth continuity turns out fiddly.** *Mitigation:* compat proven
   in Phase 2 against fixtures; the deliberate fallback (one global
   sign-out) is cheap for this instance's population but is Aaron's call in
   advance, not an incident response.
8. **Error-shape drift the frontend feels.** *Mitigation:* invariant 1 +
   golden envelopes in the contract suite, recorded from Python before any
   Go handler exists.
9. **The interim gets normalized** — the pair runs fine and retirement
   slips forever, leaving two runtimes to maintain. *Mitigation:* Phase 8 is
   a phase with a gate and a published comparison, not a someday; ROADMAP
   carries it as the phase's end condition.
10. **Scope creep wearing a helpful face** — "while we're porting X, improve
    it." *Mitigation:* the port changes behavior only where a gate says so;
    improvements are queued to the ledger like any other finding. (Sole
    blessed exception: `load_printings` via Appender in Phase 8, because the
    ledger already queued it and the driver hands it to us.)

## 10. Working beside the other sessions

There is more than one Claude in this house now. Rules for the duration:

- **The port board is the table below**: per module — `python | porting |
  go`. A module marked `porting` is frozen for feature work; a change that
  cannot wait lands on the Python side **and** is flagged in the PR body as
  owed to the port, which does not merge its family until the change is
  mirrored.

  | Module / surface | State | Where, and what moved |
  | --- | --- | --- |
  | the listening port, static `web_dist` and `/tarot`, the auth middleware (deny-before-route, 401/403, the admin prefix) | **go** | `go/internal/door`, 2026-08-21 — in front of *both* runtimes; Python's middleware stays on behind it as the second wall |
  | `auth/sessions.py`, `auth/passwords.py` (read side: resolve a token, verify a hash) | **go** | `go/internal/auth`; read-only, Python still owns every write and the schema ladder |
  | the route classification (`tests/contract/routes.json`) | shared | read by `tests/test_isolation.py`, `tests/contract/`, and `go/internal/routes` |
  | `GET /api/colors`, `GET /api/glossary`, `GET /api/themes` — the reference prose with no pool behind it | **go** | `go/internal/api` over `go/internal/reference`, 2026-08-21 — the first family flipped, and the flip mechanism (`go/internal/door/routes.go`) with it |
  | `cards/db.py` (read side: open read-only, `get_cards`, `search`, the column fill, `art_crop_from`), `config.py` (paths and flags), `decks/partners.py`, `cards/identify.py` | **go** | `go/internal/pool` (leased, stamp-checked; `pool.Schema` is `SCHEMA` verbatim), `go/internal/config`, `go/internal/gate` (partners first), `go/internal/cards`, 2026-08-21 — the door is a CGO build from here |
  | `GET /api/cards/search`, `POST /api/cards/identify`, `GET /api/colors/{key}`, `GET /api/lore` — the pool behind the prose and the pool's own two doors | **go** | `go/internal/api`, 2026-08-21 — the second family; `/api/colors/progress` stays Python's and is reserved (`api.Proxied`) until the deck family moves |
  | `colors.py`, `glossary.py`, `lore.py`, `tarotlore.py`, `decks/model.py:THEMES` — the prose itself | shared | authored in Python, rendered by `mtglab.reference` into `go/internal/reference/data/` (written by `tests/go_fixtures.py`, held current by `tests/test_go_fixtures.py`), embedded and served by Go; the JSON becomes authoritative at Phase 8 |
  | everything else under `/api` — `api/`, `decks/` (model, gate, analyze, suggest, the sources, the log), `sim/`, `claude/`, `artifacts/`, `mana.py`, `tarot.py`, `symbols.py`, `ocr.py`, `cli.py` | python | proxied to uvicorn on loopback; the read spine continues with the deck reads, then the shelves |
  | `animist/`, `cardmotion` build, `bench/`, `mutate/` | python, permanently | ADR 38 decision 1 |
- **Flips are single PRs** with the contract run attached, deployed and
  walked before the next flip starts (main deploys itself; every flip is a
  release).
- **Schema changes keep their standing rule** — own branch, merged while
  watched — and during coexistence they land in the runtime that owns the
  ladder (Python until Phase 8).
- Session-end rituals unchanged (commandments 12/13); the roadmap artifact
  carries the port board's state so a fresh session knows the frontier.

## 11. Decisions Aaron owns (the "go over together" list)

**All seven ruled with Aaron, 2026-08-21, interactively.** The
recommendation carried in each case, with one sharpened rather than merely
accepted: cobra is a requirement for all CLI work, not a proposal that
survived. Rulings recorded per item below.

1. **Ratify the scope split** (§2): served app → Go; animist/cardmotion-
   build/bench/mutate stay Python dev tooling. (Claude's recommendation:
   yes — the alternative ports torch, which cannot be done, or abandons the
   asset pipeline, which commandments 5/6 forbid.) **Ruled 2026-08-21:
   ratified.**
2. **Sequencing:** is the port the main line (pausing simulator rungs 4–5
   for about one build-week, per §7's measured calibration) or the
   background line (two to three times that calendar, with the freeze
   protocol carrying more weight)? This is the single biggest schedule
   lever and it is entirely a priorities call. **Ruled 2026-08-21: main
   line — rungs 4–5 pause for the port's build-week.**
3. **Strangler vs big-bang** (§4). Recommendation: strangler, strongly —
   ADR 23 and the second active session make a long branch a fork factory.
   **Ruled 2026-08-21: strangler.**
4. **Session continuity** (invariant 7): match the token scheme, or accept a
   one-time sign-out at auth cutover? Recommendation: match it; fall back
   only if the spike finds real cost, and then deliberately. **Ruled
   2026-08-21: match the scheme.**
5. **`pyrand` exactness as a gate** (§5): hold Phase 5 to byte-identical
   `REFERENCE_DIGEST`, with statistical equivalence as a declared fallback —
   or accept statistical from the start? Recommendation: hold the gate; it
   is cheap to attempt and the payoff (seeds, spreads, and cache coherence
   surviving the cutover) is user-visible honesty. **Ruled 2026-08-21: the
   gate holds.**
6. **Toolchain ratification** (§6) — notably cobra as the one heavy
   dependency, and the go-duckdb/CGO posture if the Mac spike disappoints.
   **Ruled 2026-08-21: ratified as proposed — and cobra is Aaron's explicit
   requirement for any CLI work, not a default open to revisiting.**
7. **The dev-tooling package's name** post-retirement (keep `mtglab` for the
   Go CLI and rename the Python remnant, e.g. `mtglab-bench`?). Naming is
   cheap now and confusing later; flagged early on purpose. **Ruled
   2026-08-21: `mtglab` stays the Go binary; the Python remnant becomes
   `mtglab-bench`.**

---

## Appendix A — ADR 38 draft (landed 2026-08-21 as
[`docs/adr/0038`](../adr/0038-the-served-backend-is-rewritten-in-go.md);
kept as the record of the draft)

> **Title:** The served backend is rewritten in Go, and the bench stays
> Python
> **Status:** proposed (draft riding in docs/go-migration until ratified)
> **Supersedes:** ADR 3's frame (Tier 1 stays Python) for the served app;
> **amends** the ENGINEERING §1 deferred table's Go row by meeting its own
> condition (the backend *becomes* the standalone service).
> **Context:** the owner's durability/typing/parallelism call, made with the
> §1 trigger explicitly un-fired; the measured concessions to CPython (one
> GIL-bound CPU worker; a 1GB floor; per-merge boot windows); the existence
> of language-independent equivalence instruments (mana oracle digest,
> REFERENCE_DIGEST, the contract-ready HTTP suite).
> **Decision:** strangler port behind a Go front door, family-atomic flips,
> equivalence gates per §5/§7 of the plan; animist/cardmotion-build/bench/
> mutate remain a Python dev package indefinitely.
> **Consequences:** two runtimes in the image until Phase 8; enforcement
> mechanisms re-implemented per §8 or the port is judged incomplete; the
> comparison table (Appendix B) is published or the migration is not called
> done; ADRs 5, 8, 11–37 carry forward as constraints on the Go
> implementation unchanged — a rewrite of the code is not a rewrite of the
> decisions.

## Appendix B — the comparison, to be filled at Phase 8

| Metric | Python (BASELINE.md, 2026-08-21) | Go (Phase 8) |
| --- | --- | --- |
| Served-app source lines | ~38,000 | |
| Test lines / count (served app) | 36,589 / 2,668 | |
| Suite wall clock | 352s @ 2,470 (re-measure) | |
| CI wall clock (full pipeline) | 9 m 37 s on the last main run (test job 6 m 16 s; image job 2 m 27 s, 45 s of it the cached amd64 build) | |
| `GET /api/decks` warm p50 / p95 | 16.5 / 18.0 ms | |
| `GET /api/decks` cold p50 | 134.2 ms | |
| `/api/health` warm p50 | 7.3 ms | |
| card search warm p50 (DuckDB-bound) | 43.8 ms (37.9 in DB) | |
| Tier 1: one 20k-game run, 1 core | ~18 s (ENGINEERING §1) | |
| Tier 1: same run, all cores | n/a (GIL; single worker by design) | |
| `data refresh` (`load_printings`) | ~16 min / 107,355 rows | |
| Idle RSS on the instance | 127 MB (peak 215 MB, 8 threads; 2026-08-21, 2h39m after deploy) | |
| Image size (compressed) | 121.3 MB, 10 layers (registry manifest); ~325 MB unpacked, 219 MB of it the venv | |
| Boot → healthy | ≈23 s machine update → health passing (deploy log, bounded below by the check cadence); merge → instance healthy 7 m 44 s; whole pipeline 9 m 37 s | |
| Direct dependencies (served app) | 8 Python (75 installed dists) | |
| `REFERENCE_DIGEST` | pinned | must match (or superseding pin + ADR note) |
| Mana oracle 13,944 cases | pinned digest | must match |
