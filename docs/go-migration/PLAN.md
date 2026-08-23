# The plan: a Go backend for the sylvan library

*Flesh to metal, deliberately and with the digests to prove nothing else
changed.*

**Status: RATIFIED, 2026-08-21.** The seven decisions in §11 were ruled
with Aaron the same day the plan was drafted; each ruling is recorded
inline there, and [ADR 38](../adr/0038-the-served-backend-is-rewritten-in-go.md)
made them formal on the Phase 1 branch the same day (Appendix A was its
draft). **Phases 0 to 4 are done** — the edit engine, the nine editing routes,
the four lifecycle routes, the renderer, the artifacts rebuild and the accounts
have all flipped, and every `/api/decks` route and eleven of the twelve
account registrations are the door's. The port board in §10 is the frontier,
and it now also records what is **not** Phase 4's despite living under
`/api/admin`: the stats six and `DELETE /api/admin/users/{username}`, both
coupled to the in-memory jobs registry, both Phase 5's.
**Phase 5's tail risk was pulled forward and closed on 2026-08-22**:
`pyrand` is bit-exact (§5 item 3), so the one item this plan said *"does not
price at all"* now has an actual beside it. **Phase 5 itself began the same
day with the registry** (`go/internal/jobs`) — the engine only, no route
flipped, and the gate everything else in the phase waits behind: the CPU pool
is a semaphore over goroutines at last, which is the one thing §1 names as
what Go buys — **banked rather than realised, since the instance has one
core** (`nproc` answers 1, measured 2026-08-22). The two *generic* job routes
were examined the same day and deliberately left with Python: they own no
state, being the view over a registry the eight job-submitting families still
write from the uvicorn process, so **a view flips last, not first**.
Written by Claude from a
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
  **Built 2026-08-22, and on today's machine N is 1** — `nproc` on the
  instance answers 1 and `shared-cpu-1x` is a 1-vCPU microVM, so the lane is
  currently exactly as wide as Python's single worker. What this buys is
  therefore **banked, not realised**: the width follows the machine, so
  scaling the instance collects it with no code change and no second process
  to keep a registry in step. Quoting it as a throughput win today would be
  quoting a measurement nobody has taken — see Appendix B's `all cores` row,
  which cannot be filled from `shared-cpu-1x`.
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
   pinned answer set case-for-case. `test_mana.py`'s
   hand-pinned traps port as table-driven Go tests. Hypothesis's generative
   role is covered on the Go side by native fuzzing against the same
   brute-force/Hall's-theorem oracles, re-implemented once in Go.

   **Landed 2026-08-22, and every case matches.** `go/internal/mana`'s
   `solver.go` is `can_pay` and Kuhn's matching; `castability.json` beside it
   is the case set. Three corrections and one finding came out of doing it.

   The constant this item named does not exist and never did: it is
   **`CASES_ANSWER_DIGEST`**, in `tests/test_mana_properties.py`, not
   `POOL_ANSWER_DIGEST`. Named here because the wrong name was carried through
   two drafts and a session was told to go and find it.

   The corpus ships the **enumeration**, not the rows. Go rebuilds the 13,944
   cases from the same alphabets and limits, in the same order, and only the
   answers are read — because `mana_oracle.py`'s claim is that its cases come
   out the same "in any language, on any machine, forever", and a port that
   replayed a dump would have proved its solver and left that claim untested.
   Two digests rather than one, the draw corpus's lesson reused: the case
   *names* alone, then the names with their answers. A failure on the first is
   an enumeration bug and means the solver was never tested; a failure on only
   the second is castability and nothing else.

   And the fixture generator **compares the golden rather than writing it** —
   it refuses to render if the answers it computes hash to anything but
   `CASES_ANSWER_DIGEST`. Regenerating a corpus must not be a way to move a
   pin, which is the one direction in which a differential test can fail
   silently and stay green forever.

   **The finding: the enumerated case set has a structural blind spot, and it
   is exactly the shape a wrong matching hides in.** `case_costs` draws pips
   with `itertools.combinations_with_replacement`, whose tuples are
   non-decreasing in alphabet index, and `CASE_PIPS` puts its only hybrid
   last — so **no cost among the 13,944 ever presents a wider pip before a
   narrower one.** Deleting the `seen` reset between pips in Kuhn's (one line,
   and the classic way to get that algorithm wrong) passes the entire case set,
   both oracles across the entire case set, and every hand-pinned trap. It is
   wrong on `{W/U}{W} <- [W U]`, which is two pips and two lands.

   Nothing is broken: `mana.py` is correct, and Hypothesis has always covered
   this on the Python side — `test_pip_order_does_not_change_the_answer` states
   the property that fails, in as many words. What was missing was anybody
   *saying* so, in a document that describes this set as "the differential case
   set for a compiled port". So the limit is now pinned by a test that names it
   (`tests/test_go_fixtures.py`), the case is a permanent trap in the Go table,
   and the Go fuzz target found it **from a seed the enumeration can reach**,
   by the order-reversal property rather than by an oracle. That is the general
   lesson and it is worth more than the bug: **an enumeration covers what its
   generator can say, and the shape of the generator is a claim nobody
   restates.** Read it beside item 4's epsilons before trusting a case count.
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

   **Landed 2026-08-22, ahead of its phase, and exact.** `go/internal/pyrand`
   is CPython's `random.Random` bit for bit — the seeding path included, which
   is the half that is easy to get plausibly wrong: `random.Random(n)` runs
   `abs(n)` through `init_by_array` over little-endian 32-bit words, so it is
   not `init_genrand` and the key grows a word at 2**32 and again at 2**64.
   It was pulled forward precisely because it was the item this document said
   *"does not price at all"*; §7's Phase 5 paragraph and §11's risk 3 are
   rewritten accordingly, and §10 carries its row.

   **The fallback was not taken and is no longer needed.** What is proved
   here is the generator, not yet Tier 1: the engine was still Phase 5's own
   work when this was written, so `REFERENCE_DIGEST` was not reproduced in
   that change. **It was reproduced later the same day**, by
   `go/internal/sim/tier1` — see Phase 5 below, and note which half of the
   work each instrument did: the engine matched on its first run because the
   draw stream was already a checked fact, and the one bug the port did have
   was found by a *second* corpus, not by the digest. What replaces it is
   stronger
   than a spot check and was possible because of a fact worth recording —
   **Tier 1 consumes randomness through exactly one call**, `rng.shuffle(deck)`
   in `simulate_game`, and through nothing else. So a run's whole entropy
   budget is a sequence of shuffles of a known length from one seeded
   generator, and `tests/go_fixtures.py` reads that sequence off a *real*
   reference run by instrumentation that delegates to CPython (and re-checks
   `REFERENCE_DIGEST` while instrumented, refusing to write a corpus if it
   moved). The Go test replays it: **all 99,274 draws of the reference run,
   in order, through the real `Shuffle`.** When Phase 5 ports the engine, the
   randomness under it is already a checked fact rather than a hope.

   The corpus is `go/internal/pyrand/testdata/draws.json` — 20 seeds
   (including 0, negatives, and both sides of 2**32 and 2**64), the raw
   `genrand_uint32` stream recorded separately from every method that consumes
   it, `random()` compared as `Float64bits` rather than to a tolerance, and
   `getrandbits` at every width from 1 to 64. It is **byte-identical under
   CPython 3.11.15 and 3.12.13** — verified by rendering it under both, and
   held that way continuously, since nothing in the file records which
   interpreter wrote it and CI runs the drift test on each leg of the matrix.
   `sample()`, which this item names, turned out to have **no caller at all**
   and was not written.
4. **The closed forms match to float tolerance.** `sim/karsten.py` and
   `sim/curve.py` are `math.comb` hypergeometrics and expectations — no
   sampling; Go must agree to within an epsilon pinned per function.

   **Done 2026-08-22, and every pinned epsilon is zero.** The tolerance
   this item allowed for turned out not to be needed: `math/big` gives
   the binomials exactly where Python has `math.comb`, one `big.Rat`
   division is correctly rounded to the same definition CPython's
   int/int division uses, and CPython's `math.fsum` and both `round`s
   are reproduced in `go/internal/sim` rather than approximated. So the
   corpora compare `Float64bits` and the per-function pins each record
   *why* exact was affordable there — which is what a future drift will
   name.

   **Exactness is load-bearing here rather than tidy**, which is the
   part worth carrying forward: every integer these two modules produce
   comes out of a `>=` against a float. `required_sources` scans until
   the odds clear the target, `CardOdds.reliable_turn` scans against
   0.90 and feeds the shelf's sort key, `_slots_to_target` scans until
   `on_curve_odds` clears, and `curve`'s advice branches on
   `abs(per_land - per_ramp) < TOO_CLOSE`. A tolerance wide enough to
   absorb a rounding difference is wide enough to hide a different land
   count.

   **Two divergences the port found rather than inherited**, both fixed
   in both runtimes on the same branch, in the pattern the share toggle
   set. *The arm64 fused multiply-add*: Go's spec lets an implementation
   fuse `t += a*b` into one operation and the arm64 backend does, which
   rounds once where CPython rounds twice — one ulp, on the architecture
   the image ships, in exactly the accumulations these modules are made
   of. The spec names one cure, an explicit conversion, and
   `sim.Rounded` is it; the disassembly was read to confirm the guard
   survives inlining. *And `sum()` is not the same function on every
   interpreter*: CPython 3.12 gave `sum()` over floats compensated
   (Neumaier) accumulation where 3.11 adds left to right, so
   `curve.expected_lands_in_play` and `curve.on_curve_odds` answered
   differently depending on the Python underneath them — on a project
   whose CI tests both and whose container runs 3.12. Both are `fsum`
   now, which is correctly rounded and therefore the same everywhere;
   the corpus was rendered under 3.11 and 3.12 and diffed to prove it.

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

**Re-priced again 2026-08-22, after Phase 3 — and this is the calibration
point that matters**, because Phase 3 was the first phase priced by *lines
ported* rather than by spike risk, which is what the paragraph above said
to wait for. Measured the same way, off the clock rather than the feeling:
first read of the tree 2026-08-21 23:10 UTC, the fourth flip merged 00:49
and live-and-walked between 00:55 and 01:00 — **about 1 hour 45 minutes**,
four pull requests (#224 the prose, #225 the pool, #226 the deck reads,
#227 the shelves), four releases, four walks on the instance. The estimate
was ¾ of a day. What moved inside it, counted rather than felt — the Go
module went from **1,541 lines of hand-written Go and 1,172 of tests** at
Phase 2's merge to **9,236 and 4,051** at Phase 3's, so the phase itself
added **~7,700 lines of Go and ~2,900 of tests** across thirteen new
packages, beside ~3,000 lines of generated differential fixtures and
~230KB of generated JSON, against a Python read surface of the same order.
The rate is real, it is roughly **3½–4× the ¾-day figure**, and it was
measured on the phase this plan nominated in advance as the place to
measure it.

(That paragraph said "~9,000 lines of hand-written Go" until it was checked
against `git ls-tree` rather than against the working tree. Nine thousand
is what the module *stood at* after Phase 3, Phase 2's door and spikes
included — a cumulative figure quoted as a delta, which is the same class
of error as the completeness claims CLAUDE.md has had to correct three
times. The numbers above are both endpoints, so the subtraction is the
reader's to check.)

Two things stop that from licensing a smaller number for everything left.
First, **the unit that held is the per-PR shape, not the hour** — a
family-atomic flip with its own contract run, deploy and walk is what a
phase actually costs, so a phase with more families costs more flips
however fast the Go goes. Second, **the phases left are not all
line-driven.** Phase 5 carries the `pyrand` digest chase, a research task
with a named fallback and no line count; Phase 6 needs a real conversation
driven through all seven modes on the deployed pair; Phase 7 needs a hosted
Forge match photographed. Those are wall-clock, and a fast compiler does
not shorten them. So Phases 4–8 are re-stated per phase below — the
line-driven work at Phase 3's measured rate, the rest left where it was —
totalling **~1¾–2½ working days remaining**, which puts the whole port at
**~3–3¾ days** rather than 4–5. The next calibration point is Phase 4's own
actual, for exactly the reason this one was: it is the second line-driven
phase, and two points make a rate where one makes an anecdote.

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
adversarial matrix is written in both. **Owed for three sessions, and closed
2026-08-22:** `contract` is now a required check on `main`, the eleventh, read
back from the API with `strict`, linear history and admin enforcement
unchanged in the same call. Until then it gated `deploy` through `needs` — so
a red contract run stopped a *release* — but could not stop a *merge*, and
that gap widened with every flip: from Phase 4 on, the suite is the only thing
standing between a mis-ported write and somebody's `deck.yaml`.

**Why it took three sessions is the part worth keeping.** This paragraph said
the setting was "Aaron's", and two other documents said the same; each session
inherited it as a fact. It never was one — the `gh` CLI is his own and the
change is one API call. What was actually missing was a *confirmation*, which
is a question to ask in the moment, not a line to write into a plan. A note
saying somebody else must do a thing reads like a permission boundary and
outlives the moment it was true.

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

**Phase 3 — The read spine** *(~¾–1 day; re-priced ¾; **done 2026-08-22
in ~1h45m**).* `config`, `cards/db` reads,
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
paragraph carries the flag. *Deployed as v158 and walked the same way: the
signed-in probes answered from the door (search, identify, `/api/colors/G`,
`/api/colors/nope`, `/api/lore`, the 422 for `limit=abc`) and uvicorn
logged only `/api/colors/progress`; BASELINE has the block.*

**The deck reads came third — the family the phase is named for.**
`go/internal/deck` is `decks/model.py` parsed and only parsed (a `Payload`
projection stands in for `dump()` where the baseline comparison needs one,
so Go still never serialises a deck); `go/internal/gate` is the whole gate
now — `validate`, `companion`, `partners` — and **agrees with Python case
for case** on eight fixture decks: `tests/go_fixtures.py` writes each
deck's text beside Python's own report (with the 21-card pool and without),
its `stats_for` answer and, where the gate found something a different
card would fix, its `suggestions_for` answer, into
`go/internal/gate/testdata/`; `tests/test_go_fixtures.py` holds those
files current and requires the cases to emit every code the gate has; and
the Go tests parse the same text, build the same pool from `pooltest`, and
must produce the same issues in the same order with the same sentences —
the same stats document, the same candidates with the same scores and
reasons. `go/internal/analyze`, `go/internal/suggest`, `go/internal/mana`
(the parser; the solver is Phase 5's, held to the 13,944-case oracle then),
`go/internal/library` (the file tier, the SQL tier read-only over the door's
`app.db` handle, the shared-only view and `Library` — ADR 22's owner
resolution, `visible()`'s order, the maintainer resolved through the
address and never rendered) and `go/internal/decklog` (ADR 28's read side)
carry the rest; the deck model's spoken vocabulary — `CATEGORIES`, the
statuses and stages, the singleton exemptions, the category targets and
the Game Changer limits — joined the generated JSON as `model.json`, so the
Go gate's sentences are Python's words. The door puts the caller's scope on
the request context (`auth.WithScope`) for the ported routes; the third
family flipped is `GET /api/decks`, the deck, `validate`, `stats`,
`suggestions`, `commander`, `printings`, `log`, `artifacts` (the shelf and
one deliverable) and `/api/colors/progress`, which left the reservation
list the day it moved. *Deployed as v159 and walked through the signed-in
seat against the real library: seven tiles under the maintainer, the
gate's counts on the shelf exactly CLAUDE.md's record (adrix 1, goreclaw 1
— the banned Titan — the rest 0), Goreclaw's 75 known cards and the Titan's
five suggested replacements, Tivit's and Goreclaw's commander panels; and
uvicorn's access log silent for every one of them. The door's RSS read
103 MB after the walk; the v159 image 147.7 MB compressed.*

**The shelves came last, and closed the read spine.** `go/internal/shelves`
is `symbols.py`, `ocr.py` and `cardmotion/cache.py`'s serving half over
`data/cache/`: a mana symbol fetched from Scryfall's CDN on the first ask
(size-capped, SVG-checked, written atomically, a 404 remembered), a
reading-engine file fetched once and **refused unless its SHA-256 matches
the pin** (loudly and stickily, as Python refuses it), and a card-art
derivative found by scanning the small cache for an `attribution.json`
naming the oracle id, the effect's fingerprint and — when the page says
which painting it is showing — that painting's URL stem. The configuration
moved as generated JSON too (`shelves.json`: the CDN and the code shape,
the pinned files with their digests and the versioned cache stamp, the
effects table **with the fingerprints Python computed**), because matching
a derivative means matching the sixteen hex digits the dev-machine build
wrote, and re-deriving Python's `json.dumps` byte for byte in a second
language is the kind of cleverness a drift check exists to make
unnecessary. The files serve with explicit media types (`text/javascript;
charset=utf-8` as Starlette adds it, `image/svg+xml`, the four motion
types), the caching policy `api/app.py` gives each, and Range requests a
video element needs. The route table learned a parameter with a literal
suffix (`/api/symbols/{code}.svg`, FastAPI's `[^/]+` before the dot). The
fourth family flipped: `/api/symbols/{code}.svg`, `/api/ocr/{name}`,
`/api/art/motion/{oracle_id}/{effect}` and its `/{filename}` — and with it
**every read-only route under `/api` is the door's**; what still goes to
Python is the writes, the jobs, the Claude surfaces, `/api/health`, the
upcoming sets and the wheel (seeded `random.Random`, Phase 5's `pyrand`).

*Phase 3 closed 2026-08-22, in one session.* **Actuals:** first read of the
tree 2026-08-21 23:10 UTC; #224 merged 23:38, #225 23:59, #226 00:29, #227
00:49; the fourth release live and walked on the instance between 00:55 and
01:00 — **about 1 hour 45 minutes** for four family-atomic flips against an
estimate of ¾ of a day. The gate was met per family, and in one respect
beyond its own terms: it asked that `validate` agree with Python on
`tiny_pool`'s deck and the template decks, and what was built agrees **case
for case on eight fixture decks chosen to emit every code the gate has**,
with `stats_for` and `suggestions_for` held to Python's answers in the same
comparison. Three things this phase taught that the phases after it
inherit. A **generated JSON shared by both runtimes** is cheaper and safer
than re-deriving prose or configuration in a second language, and it is now
the pattern for anything Python authors and Go serves — five files already
(the prose, the deck model's spoken vocabulary, the shelves' configuration
carrying Python's own fingerprints), and the writes will want a sixth. A
**differential fixture written by the Python side** (`tests/go_fixtures.py`
→ `go/internal/gate/testdata/`) turns "the gate agrees" from a claim into a
test that fails in CI, where there is no Python to ask — which is exactly
the shape Phase 4's gate calls for and the reason it is cheap to build. And
**the route table has to describe FastAPI's routing, not merely its
paths**: the most-specific-pattern rule and `api.Proxied`'s reservation
list both came out of one real collision (`/api/colors/progress` declared
before `/api/colors/{key}`), and a write family flipping beside a literal
Python still owns will meet it again.

**Phase 4 — Writes and the log** *(~¾–1 day; **re-priced 2026-08-22:
~2–3 hours** — line-driven, at Phase 3's rate, plus the extra care the
writes earn: a wrong byte in a read is a wrong answer that refreshes away,
and a wrong byte here lands in the file somebody's deck lives in).* The edit operations
(text surgery + oracle verification, ADR 12's five rules re-proven),
`_commit` + the activity log (ADR 28: one call site, `record` never raises,
no rationale text), create/import/promote/delete, notes, labels (ADR 37's
worst-piloted reading — server-side only, as the deck page already assumes),
the **account** routes, invites/resets (EmailSender seam; no test sends mail),
rate limiting. *Gate: golden-deck edit equivalence — every operation applied
by Go over fixture decks yields byte-output Python's operation also yields;
ADR 5/16/17 matrices green via the contract suite.*

**What "admin routes" turned out to mean, recorded 2026-08-22 when the
accounts were built.** This phase said "admin routes" without qualification
and that sentence was false in both directions, so it now says *accounts*.
The **stats** six under `/api/admin` are not Phase 4's: `adminstats.py` reads
`api/jobs.py`'s in-memory registry and the process's own RSS, and
`claude/prices.py` and `claude/tiers.py` besides — two families that have not
moved. And one *account* route is not Phase 4's either: `DELETE
/api/admin/users/{username}` calls `jobs.forget_owner` on that same in-memory
registry, and it cannot be skipped, because `users.id` is re-issued by SQLite
and jobs left keyed on a freed id would be handed to the next account
created. Both flip behind the jobs registry, which is Phase 5. §10's board
carries the detail; the lesson is the general one this plan keeps
re-learning — **a prefix is not a family.**

**Phase 5 — Jobs and the simulator** *(~1–1½ days; re-priced ~½–1 day, and
**re-priced again 2026-08-22 to ~½ day** — the band's whole width was the
`pyrand` digest chase, which said it did not price at all; it has now been
run and it priced. Measured off the clock: first read of the plan and the
consumers 05:44 UTC, three green Go gates 06:10 — **about 25 minutes** for
the package, its 444KB differential corpus, the fuzz target and the
99,274-draw replay of the reference run's stream. Two things about that
number. It is **not** evidence the rest of Phase 5 is fast: the chase was
the risk, and what is left is line-driven work that prices at Phase 3's
rate. And it was 25 minutes **because the research was cheap, not because
the reproduction was easy** — the seeding path, `getrandbits`'s word order
and `_randbelow`'s rejection are each a place a plausible implementation is
wrong, and all three were caught by having CPython's own answers on hand
rather than by being careful. The corpus is the whole story; a session that
had written the same package without one would still be looking for the
first of them.).* The registry
(pools → semaphores; `key=` dedupe per owner in one locked step;
born-finished jobs; the FORGE single-lane rule) — **landed 2026-08-22 as
`go/internal/jobs`, and it flipped nothing**, in the engine-then-routes
rhythm #228/#229 and #234/#238 established. Two things it found are worth
carrying into the rest of the phase. **A ported result may not be a
`map[string]any`**: encoding/json sorts map keys and a Python dict keeps its
insertion order, so every job result still to cross owes itself a struct with
the fields in Python's order. And **the arithmetic on the way out is not
neutral** — `percent` rounds half to even, which `math.Round` does not, and
one job in eight lands on a tie.

**Its sibling hazard, and the one that has now cost three lanes in a day:
`sum()` over floats is not the same function on every interpreter.** CPython
3.12 gave it compensated (Neumaier) accumulation where 3.11 adds left to
right — `sum([0.1] * 10)` is `1.0` under 3.12.13 and `0.9999999999999999`
under 3.11.15 — and this project supports both, tests both in CI, and runs
3.12 in the container. So the obvious Go transcription, `for … { total += x }`,
reproduces **3.11**, which is the Python the image is not running. Read it as
the exact counterpart of the rounding rule above: `math.Round` is not
CPython's `round`, and a `+=` loop is not CPython's `sum`. Both are cases
where Go's plainest spelling is a *different function*, not a worse one.

Three things about it are worth carrying rather than rediscovering. **Fix it
in Python rather than reproducing it in Go** — `math.fsum` is correctly
rounded, so it is not either interpreter's dialect, and `pyfloat.Fsum` is
already CPython's own algorithm on this side; a port that faithfully
reproduced `sum` would have to pick an interpreter and be wrong on the other
leg of the matrix. **The corpora will not find it for you**: when the sweep
ran on 2026-08-22, three byte-exact oracles were green against naive Go —
`artifacts.json` priced only exact halves and quarters, and all eight gate
decks sat at land counts where the two arithmetics happen to agree — so each
one needed a fixture *cut for the edge* (`last-bit`, `half-cent`, a new scorer
corpus) plus a test asserting the corpus still separates `Fsum` from a running
total, in the idiom `pyfloat_test.go` already had. And **the proof is a
diff**: render the corpora under 3.11 and under 3.12 and compare bytes. With
the fix reverted, exactly three files differ; with it in place, none does, and
CI re-checks that forever because nothing in a corpus names an interpreter.

`pyfloat` moved out of `internal/sim` in the same change, and the reason
generalises: **the CPython-reproduction packages are not owned by whichever
family needed them first.** `artifacts`, `analyze` and `suggest` all need
`Fsum` and none of them is the simulator, so it sits beside `pyrand` and
`pyyaml` as `go/internal/pyfloat`. A fourth family reaching for one of these
should find a package, not a dependency on somebody else's tier.

Then `pyrand`, then Tier 1
against `REFERENCE_DIGEST`, karsten + curve to tolerance (**done 2026-08-22, and
the tolerance came out at zero** — see §10's row and the note in §5 item
4), the mulligan grid,
land sweeps, the sim cache (ADR 18 keys with a Go-source fingerprint;
`deck_check` attached after the cache exactly as today), `NothingToSimulate`,
and the job families flipped. **Everything on that list up to the flips is
done, as of 2026-08-22**: the compiler, the mulligan grid and the sim cache
crossed in the phase's closing engine PR, and land sweeps came across with
Tier 1 (`SweepLandCounts`). The fingerprint question was answered
deliberately rather than by default — Go's rows and Python's **sit apart**,
which is ADR 18's second consequence applied to the most extreme engine
change there is; §10's `sim/cache.py` row carries the argument. So **what is
left in Phase 5 is entirely the flips**, and they landed on 2026-08-22: the
sim family (`/api/sim/mana`, `/lands`, `/shelf`, `/policy`) answers from the
door, and the two generic job routes went with it as the **hybrid**. The
choice between the two shapes this paragraph used to leave open was made by
the dependency graph rather than by preference -- five of the eight
job-submitting families need `claude/` and one needs `sim/tier3`, so "all
eight in one change" was not reachable and the hybrid was the only shape
available. **`/api/sim/forge` stays Python's** (Phase 7's engine), which is
what keeps the proxy branch live. **Phase 5 is complete.** **The GIL dividend is banked here, and cannot
be measured on the instance as it stands** — that sentence read *"lands here
and gets measured — same machine, N-core Tier 1 scaling into Appendix B"*
until 2026-08-22, when somebody asked the machine. `fly ssh console -C nproc`
answers **1**: `shared-cpu-1x` is a 1-vCPU Firecracker microVM, not a
cgroup-limited slice of a bigger host, so the deployed CPU lane is exactly as
wide as Python's single worker and an N-core scaling curve has no N to sweep.
The capability is real and follows the machine — scaling the instance
collects it with no code change — but Appendix B's `all cores` row needs a
bigger machine, or it is filled from this Mac and labelled as such. Deciding
which is Aaron's, and it is a question about what the comparison is *for*,
not a blocker on the phase. *Gate:
`REFERENCE_DIGEST` reproduced (or the statistical fallback consciously taken
and re-pinned — an ADR-worthy divergence, not a shrug); the mana oracle's
13,944 cases match the pinned digest; contract green.*

**The digest is reproduced, 2026-08-22.** `go/internal/sim/tier1` computes
`c3e278e3…22d4` — the same sha256, over the same text, from a simulator
written in another language. The fallback was not taken and no number moved.
Three things about how it went are worth the next session's time.

**It came out right on the first run, and that is `pyrand`'s doing rather
than luck.** The engine was written against the Python line by line and the
digest matched before any debugging, because the one thing that could not
have been debugged from the outside — the draw stream — had already been
proved. This is what pulling a tail risk forward buys: not a faster port, a
port whose failures are all in the half you can read.

**Reproducing the numbers is not reproducing the digest.** The gate hashes
`repr()`, so CPython's float formatting is inside it: `100.0` is not `100`,
`1e-05` is not `0.00001`, and `median_commander_turn` is an *int* for an
odd-length list because `statistics.median` returns one — its `float | None`
annotation is simply wrong, and the digest is not. So the package renders
Python's `repr` (`repr.go`), held to CPython by 527 recorded floats.

**And the digest is one run, which is exactly its weakness.** It covers a
deck whose 99 cards all have distinct names and a policy that mulligans at
most three times — so it is blind to `list.remove` taking out an equal card
rather than the one it was handed, and blind to the hand-size cap on
bottoming. A second corpus was written for that (a deck of repeated cards,
policies that mulligan to nine), and it **caught a real bug the digest could
not see**: Go re-evaluates a `for` condition every pass where Python's
`range(min(mulligans, len(hand) - 1))` is computed once, so at four or more
mulligans the port bottomed one card too few. Nine of ten deliberate
mutations die against the corpus; the tenth is an equivalent mutant and is
argued as one in the code.

**Phase 6 — The Claude surfaces** *(~¾–1 day; **re-priced ~½ day** — a
couple of hours of porting, plus a real conversation driven through all
seven modes on the deployed pair, which is wall-clock the compiler does not
shorten; claude-api skill loaded first).* The pipe (`converse`: caching breakpoints, container-id ride-along,
`pause_turn` resumed-never-returned), stance + `/api/claude` per-surface
defaults, personas, all seven modes with their schemas' deliberate absences
intact, source-checking (`keep_sources`, drop-and-count), the tarot table's
server half (seeded deal via `pyrand` — same seed, same spread, across the
cutover), the usage ledger, and the six job-shaped routes flipped. *Gate:
boundary analysis pass in CI (invariant 3); every mode's structural tests
ported; a real conversation driven on the deployed pair for each mode —
rendering a value audits it, per the recorded lesson.*

**Phase 7 — Forge, scan, and the ledger** *(~¼–½ day; **re-priced ~2–3
hours**, and the hosted match is most of it).* `sim/tier3` wire
+ worker client (Machines API; creation stays in the deploy workflow), the
gate/refusal split, per-game progress streaming, the match ledger (ADR 36:
snapshot labels at match time, record from exactly two places), scanruns
(ADR 34: schema with no name field), coverage checked twice, medians-and-
tails reporting. The shim itself re-ships as a static Go binary on the
worker (its idle self-stop behavior preserved). *Gate: a real hosted match
on the instance, bar ticking, ledger row written — photographed, commandment
14.*

**Phase 8 — Retirement and the comparison** *(~¼–½ day; **re-priced ~2–3
hours** — the retirement itself is small, and Appendix B's measurements are
bench runs and image builds, which take the time they take).* Python leaves
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
| `test_claude_boundary.py` (AST walk) | nothing under claude/ names a deck write | **BUILT 2026-08-22** — `go/internal/claude/boundary_test.go`, over the typed package graph (`go/packages`), and deliberately written **before** the modes, which is stance.py's own argument reused: retrofitting a gate around modes that already exist is how the gate ends up with holes. Two checks rather than one, and both are stronger than the Python guard in a way that was *proven by mutation* rather than claimed. The **graph** check bans `internal/deckedit` across the whole transitive import graph, so an intermediary package is not a way around the rule — a `claude -> helper -> deckedit` chain passes Python's per-file name walk completely, and this names the full route in the failure. The **typed** check resolves every identifier through the type checker rather than matching text, so an aliased import (`import notanedit "…/deckedit"`) is caught where a string match misses it, an interface method (`library.Writer.WriteText`) is caught where a package-function match misses it, and a local variable that merely shares a name is correctly *not* flagged. All five of those shapes were driven as deliberate violations before the guard was trusted. Two anti-rot tests ride along: the named write surface must still exist (a rename otherwise leaves a green suite guarding nothing), and the guard must cover a non-empty set of packages that actually parsed |
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
3. ~~**`pyrand` exactness stalls** (an undocumented CPython corner).~~
   **Closed 2026-08-22 — the risk did not arrive, and it was pulled forward
   on purpose so that sentence could be written early.** `go/internal/pyrand`
   reproduces `random.Random` bit for bit, held to 20 seeds of CPython's own
   answers and to the reference run's full 99,274-draw stream; the
   statistical fallback was never reached and is withdrawn. There was no
   undocumented corner: every difficulty was documented and *easy to skip* —
   the `abs()`-then-`init_by_array` seeding, `getrandbits`'s least-
   significant-word-first fill, `_randbelow`'s `n.bit_length()` rejection
   (not `(n-1)`), and `shuffle` walking downwards. Each is a place where a
   reasonable implementation is wrong and still looks random, which is the
   general lesson: **the hazard here was never obscurity, it was
   plausibility**, and the only instrument that finds plausibility is a
   differential corpus.
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
  | `GET /api/cards/search`, `POST /api/cards/identify`, `GET /api/colors/{key}`, `GET /api/lore` — the pool behind the prose and the pool's own two doors | **go** | `go/internal/api`, 2026-08-21 — the second family; deployed as v158 |
  | `decks/model.py` (parse), `decks/validate.py`, `decks/companion.py`, `decks/analyze.py`, `decks/suggest.py`, `mana.py` (the parser), `decks/source.py` + `decks/sqlsource.py` + `decks/library.py` (reads), `decks/log.py` (reads), `auth/bootstrap.py:maintainer_username` | **go** | `go/internal/deck`, `gate`, `analyze`, `suggest`, `mana`, `library`, `decklog`, 2026-08-22 — held to Python by the differential cases in `go/internal/gate/testdata/` (written by `tests/go_fixtures.py`); Python still owns every write and the schema ladder |
  | `GET /api/decks`, `GET /api/decks/{owner}/{slug}` and its `validate`, `stats`, `suggestions`, `commander`, `printings`, `log`, `artifacts`, `artifacts/{name}`; `GET /api/colors/progress` | **go** | `go/internal/api`, 2026-08-22 — the third family, the deck reads |
  | `symbols.py`, `ocr.py`, `cardmotion/cache.py` (serving) — the runtime shelves under `data/cache/` | **go** | `go/internal/shelves`, 2026-08-22; configured by the generated `shelves.json` (the pins, the CDN, the effects' fingerprints as Python computes them) |
  | `GET /api/symbols/{code}.svg`, `GET /api/ocr/{name}`, `GET /api/art/motion/{oracle_id}/{effect}`, `GET .../{filename}` | **go** | `go/internal/api`, 2026-08-22 — the fourth family; the read spine is whole |
  | `colors.py`, `glossary.py`, `lore.py`, `tarotlore.py`, `decks/model.py:THEMES` — the prose itself | shared | authored in Python, rendered by `mtglab.reference` into `go/internal/reference/data/` (written by `tests/go_fixtures.py`, held current by `tests/test_go_fixtures.py`), embedded and served by Go; the JSON becomes authoritative at Phase 8 |
  | `decks/edit.py` (the nine operations and their text surgery), and PyYAML's emitter under it | **go** | `go/internal/deckedit` over `go/internal/pyyaml`, 2026-08-22 — the engine only; **no route has flipped**, so Python still serves every write. Held to Python by two generated oracles: 2,051 `_render` cases byte for byte, and 514 operation steps over eleven fixture decks (`tests/go_fixtures.py` writes both) |
  | `service._commit`, the pool checks above the editor (`_check_category`, `_identity_of`, `_check_printing_of`), `decks/source.py`'s and `decks/sqlsource.py`'s `write_text`, `decks/log.py`'s write side | **go** | `go/internal/api/edits.go`, `go/internal/library/write.go`, `go/internal/decklog`, 2026-08-22 — `create` and `delete` deliberately **not** ported yet, since an update and a create have opposite safety requirements and a method that exists but refuses is a method somebody wires up |
  | `POST .../swap`, `POST .../cards`, `DELETE .../cards/{name}`, `POST .../entomb`, `POST .../graveyard/{name}/return`, `DELETE .../graveyard/{name}`, `PATCH .../cards/{name}`, `PATCH /api/decks/{owner}/{slug}`, `PUT .../notes/{key}` | **go** | `go/internal/api`, 2026-08-22 — the fifth family, and the first that *writes*. Every one goes out through one `commit`, so the gate's verdict and ADR 28's entry are inherited rather than remembered |
  | `decks/decklist.py`, `decks/importer.py`, `decks/model.py:Deck.dump`, `decks/source.py`'s and `decks/sqlsource.py`'s `create` / `delete` / `set_shared`, and `decks/edit.py`'s **tenth** operation | **go** | `go/internal/decklist`, `go/internal/deckimport`, `go/internal/deck/dump.go` over `go/internal/pyyaml`'s whole-document emitter, `go/internal/library/write.go`, `go/internal/deckedit`, 2026-08-22 — held to Python by two more generated oracles (15 decklist pastes, 12 imports resolved against the 21-card pool, 13 dumps). `Dump` **refused a deck carrying notes** until the artifacts flip ordered them, which is the expiry that refusal named for itself |
  | `POST /api/decks`, `POST /api/decks/import`, `DELETE /api/decks/{owner}/{slug}`, `PUT .../shared` | **go** | `go/internal/api/lifecycle.go`, 2026-08-22 — the sixth family: the moments a deck begins and ends. **None of them goes through `commit`**, because creation, deletion and sharing are outside `service._commit` in Python and therefore outside ADR 28's log; keeping them outside is the decision, since adding one means a second call site and one call site is the log's whole design |
  | `artifacts/generate.py` — the renderer, `store`, and the notes' file order under both | **go** | `go/internal/artifacts` over `go/internal/deckyaml`'s `ParseOrdered`, 2026-08-22 — the engine only; the route followed in the row below. Held to Python by an eighth generated oracle: 18 fixture decks, every deliverable byte for byte and in order, the draft refusal's own sentence included, with the date pinned so the corpus does not expire at midnight. Ordering the notes **fixed a live regression**: `GET .../{slug}` had served them alphabetised since the deck reads flipped (#226, v159), and the deck page's Notes tab renders the payload's order unsorted |
  | `POST /api/decks/{owner}/{slug}/artifacts` and `decks/source.py`'s `write_artifacts` | **go** | `go/internal/api/artifacts.go`, `go/internal/library/write.go`, 2026-08-22 — the flip that finishes Phase 4's **deck** side, every route under `/api/decks` now the door's. Deliberately not numbered among the families: the accounts row below claims a number too, and which of the two lands first is a fact about a night's merge order rather than about the port. A **plain route, and that was measured** (70–83ms warm across four real decks on the instance), so it stays one; and it is deliberately **outside ADR 28's log**, which is why it does not go through `commit` even though it writes. Two refusals and only one is forceable: the gate's errors yield to `force`, a draft never does (ADR 13) |
  | `auth/users.py`, `auth/tokens.py`, `auth/invites.py`, `auth/ratelimit.py`, `auth/mail.py`, and the write sides of `auth/sessions.py` and `auth/passwords.py`; `claude/tiers.py`'s table | **go** | `go/internal/auth` (one Go package for one Python package, file for file) and `go/internal/tiers`, 2026-08-22 — the engine only; **no route has flipped**, so Python still serves every account request. The write handle is `mode=rw`, never `rwc`, and an absent `app.db` is **read as an empty one** — measured against Python, which creates the file on the first login and then answers 401 against it, because a reader cannot tell empty from absent. Held to Python by `go/internal/auth/testdata/crypto.json`: the exact PHC string argon2-cffi writes for a password **and a fixed salt**, which the Go encoder must reproduce byte for byte — a round trip in each direction would pass even if the two encoders disagreed about base64 padding for some salts and not others |
  | `POST /api/auth/login`, `logout`, `reset`, `claim`, `claim/preview`, `GET /api/auth/me`; `GET` and `POST /api/admin/users`, `PATCH /api/admin/users/{username}`, `POST .../reset`, `DELETE .../sessions` | **go** | `go/internal/api/accounts.go` and `admin.go`, 2026-08-22 — the seventh family: eleven of the twelve registrations under `/api/auth` and `/api/admin/users`. The public six are the ones with no session behind them, so each is rate limited and each is written so a refusal tells the caller nothing it did not already know; the admin five are protected twice, by the prefix the door refuses before routing and by `requireAdmin` on the handler. `POST /api/auth/reset` is the only route in the whole port with work *after* the response — Starlette's `BackgroundTasks`, and the reason is the timing: a hit costs a database read and an HTTPS round trip, a miss costs neither, and neither happens while anybody is waiting. **The twelfth registration, `DELETE /api/admin/users/{username}`, is not here** — it is the row below, and it is still Python's. This row is checkable: `grep -c 'api/auth' go/internal/api/api.go` is non-zero from the commit that landed the routes, and was **zero** for the day the engine's PR carried this row ahead of the code |
  | `DELETE /api/admin/users/{username}` | python | **blocked behind the jobs registry, not deferred by choice.** Deleting an account also calls `jobs.forget_owner`, and `api/jobs.py` holds its jobs **in memory in the uvicorn process** — which the door cannot reach. The reason it must not be skipped is arithmetic rather than tidiness: `users.id` is `INTEGER PRIMARY KEY` without `AUTOINCREMENT`, so SQLite re-issues a deleted account's rowid, and jobs left keyed on that integer would be handed to whoever is created next. The response says `jobs_dropped`, so a Go handler could not even report an honest number. It flips with the jobs family — **and the registry it waits on landed 2026-08-22** (`jobs.ForgetOwner` included, guarded so owner zero, which is the local user, can never be mistaken for a deleted account), so what is left here is the route, not the engine. `go/internal/api/admin_test.go`'s `TestDeletingAnAccountIsStillPythons` is the tripwire and is meant to fail when somebody flips it on purpose |
  | `/api/admin/stats/*` (the six), `api/traffic.py`, `api/flymetrics.py` | python | **not part of Phase 4, despite the prefix.** `adminstats.py` is coupled to two families that have not moved: `stats/system` reads the same in-memory job registry *and* `_rss()`, the process's own resident size — which is the worse of the two, because a Go handler would answer it *successfully* with the door's RSS and the number would keep rendering while quietly changing meaning; and `stats/claude` reads `claude/prices.py` and `claude/tiers.py`, so porting it now would drag slices of `claude/` across ahead of its family, which §7's "a route family moves whole" forbids. It flips behind the jobs registry and the Claude family |
  | CPython's `random.Random` — MT19937, `init_by_array` seeding, `random()`, `getrandbits`, `_randbelow`, `randrange`, `shuffle`, `choice` | **go** | `go/internal/pyrand`, 2026-08-22 — **Phase 5's named tail risk, pulled forward and closed** (§5 item 3, §11 risk 3). A library, not a route: nothing calls it yet and nothing flipped. Held to CPython by `testdata/draws.json` (20 seeds, the raw word stream recorded apart from every consumer, `random()` compared as bits) plus a replay of the reference run's full 99,274-draw stream, so Tier 1's randomness is a checked fact before the engine that consumes it exists. `sample()` had no caller and was not written |
  | `mana.py`'s castability solver — `can_pay`, `expand_units` and Kuhn's augmenting-path matching under them | **go** | `go/internal/mana/solver.go`, 2026-08-22 — the parser's package, extended rather than a second one. **A library, not a route: nothing flipped.** It had no caller at all for a few hours — Tier 1's engine carried its own private `canPay` over `sim.Source`, exactly as `engine._consume` re-solves this in Python — and this row said so, adding that `tier1.go`'s comment promised the collapse and that until then **the two Go solvers were each held to Python and neither to the other**. **The collapse landed the same day** (the sim-engine lane): `tier1.canPay` is now a call to `CanPay`, which is the *faithful* port rather than a tidy-up, since Python's `_pick_land` calls `mana.can_pay`. `REFERENCE_DIGEST` did not move, and the seam was **measured** rather than assumed — a 2,000-game run is 155–196ms through `CanPay` against 169–216ms through the local answer, because `CanPay` refuses cheaply on `unitCount` without materialising the units. `consume` stays, because it has to return the leftovers, so there are still two solvers; what changed is that they are now held to **each other** by `TestConsumeAgreesWithCanPay`, which is `test_consume_agrees_with_can_pay` in Go. `mana.Source` is deliberately field-for-field `sim.Source`, in order, so the conversion at the seam is free. They stay separate types because the layering runs the other way: `mana` sits below `sim` and must not import it, which is the same split `sim.Cost` beside `mana.Cost` already makes. Held to Python by `testdata/castability.json`, which carries the **enumeration** and the answers rather than 13,944 dumped rows — Go rebuilds the case set from the same alphabets in the same order, because `mana_oracle.py` claims that set is reproducible "in any language, on any machine, forever" and a replayed dump would leave the claim untested. Two digests, so a failure says which half broke: the case *names* alone, then `CASES_ANSWER_DIGEST` — the project's own golden, compared and never re-pinned. Both reference implementations are re-written in Go from their theorems (§5 item 2) and keep their own unit expansion and colour comparison, so `ExpandUnits` and the solver's six-bit packing are then judged *against them* — over the amounts 0, 2 and negative, which the case set **cannot reach at all**, every pool in it being a single-mana source. The fuzz target that plays all three against each other **earned its place on day one**: see §5 item 2. Checkable: `grep -c 'func CanPay' go/internal/mana/solver.go` |
  | `api/jobs.py` — the registry: two pools, the `key` dedupe, born-finished jobs, the bound, `forget_owner` | **go** | `go/internal/jobs`, 2026-08-22 — **Phase 5's first half, and the engine only: no route has flipped**, so Python still owns every job a request creates. The CPU lane is the semaphore over goroutines ADR 38 promised, sized from `GOMAXPROCS` (8 here, and 1 on a `shared-cpu-1x` — a `Config` knob, not a constant); `NET` stays at two and `FORGE` at one, because neither of those bounds is a fact about the machine. Two differences are Go's rather than the design's and both are argued in the package comment: the mutable half of a `Job` is **guarded**, where Python's worker writes `status` and `done` with no lock and the GIL absorbs it, and a **panicking worker is a failed job** rather than a dead process. Held to Python by a ninth generated oracle (`jobs/testdata/jobs.json`), which caught three divergences a careful implementation would have shipped: `percent` **rounds half to even** (1 of 8 is 12.5, and Python answers 12 where `math.Round` answers 13), `created_at` **drops its fraction entirely** when the microsecond is zero and is spelled `+00:00` rather than `Z` — and it is the *sort key*, as text — and the lane refusal quotes with `repr`, which prefers single quotes. The concurrency is proven by the race detector, six killed mutations and a fuzz target over submit/dedupe (126k executions under `-race`); the mutation that dropped the owner from the match **found a weak test** rather than a weak implementation, because the fixture's keys and owners had been varied together. One constraint falls out of it for every family still to flip: **a ported job result must be a struct with its fields in Python's order, never a `map[string]any`**, since encoding/json sorts map keys and a dict does not. The lane's width was **measured on the instance 2026-08-22 and the code comment explaining it was wrong**: `nproc` answers 1 and `/sys/fs/cgroup/cpu.max` does not exist, because a Fly machine is a 1-vCPU Firecracker microVM on cgroup **v1** with every controller at the root — so Go 1.25's quota reader never fires, GOMAXPROCS falls back to `NumCPU`, and the two agree. `NumCPU` would not "count cores nobody gave us" here; it would answer the same. The call stands, the reason given for it did not, and §1's dividend is **banked rather than realised** until the machine is scaled |
  | `sim/karsten.py` and `sim/curve.py` — Tier 1.5, the closed forms | **go** | `go/internal/sim/karsten` and `go/internal/sim/curve` over the shared `go/internal/sim`, 2026-08-22 — **the engine only; no route has flipped**, so `/api/decks/{owner}/{slug}/shelf` and its siblings are still Python's. Checkable, per the rule below: `grep -rn 'sim/karsten\|sim/curve' go/internal/api go/internal/door go/cmd` prints **nothing** at this commit — the engine has no caller in the served path — and stays that way until the jobs family moves. (The obvious `grep -c 'shelf' go/internal/api/api.go` is *not* that command and was written first: it answers **3**, all of them comments about the runtime shelves and the artifacts shelf. A row's command has to be run, which is the rule below, and running it is what caught this.) §5 item 4 asked for agreement "to within an epsilon pinned per function"; every epsilon is pinned at **zero** and the corpora compare `Float64bits`, because exact was affordable — `math/big` binomials where Python has `math.comb`, and CPython's own `math.fsum` and `round` reproduced in `go/internal/sim`. Exactness is not decoration: `required_sources`, `reliable_turn` and `_slots_to_target` all scan a float against `>=`, so one ulp is a different land count or a different recommendation. Two findings the port made rather than inherited, both fixed in **both** runtimes: arm64 fuses `t += a*b` into one `FMADDD` and rounds once where CPython rounds twice (guarded by `sim.Rounded`, an explicit conversion the Go spec blesses for exactly this), and **`sim/curve.py` answered differently on Python 3.11 and 3.12** because CPython 3.12 gave `sum()` over floats compensated accumulation — two lines, now `fsum`, and the corpus is byte-identical under both interpreters, verified |
  | `sim/tier1/engine.py` — the goldfish itself: `KeepRule`, `simulate_game`, `run`, `sweep_land_counts`, `_consume` | **go** | `go/internal/sim/tier1`, 2026-08-22 — **the engine only; no route has flipped**, so Python still serves every simulation. **`REFERENCE_DIGEST` is reproduced** — `go test -run TestTheReferenceRunReproducesThePinnedDigest ./internal/sim/tier1`, and the digest is a literal in `tier1_test.go` so a regenerated fixture cannot re-pin the gate. Reproducing the numbers was not enough: the gate hashes `repr()`, so `repr.go` is CPython's float and string rendering, and `median_commander_turn` is an **int** for an odd-length list because `statistics.median` returns one — its `float | None` annotation was wrong and the digest was not. (**Narrowed to `int | float | None` on 2026-08-22**, with a Python test that pins the runtime type at each parity; the value was deliberately *not* coerced, since `repr(4)` and `repr(4.0)` differ and a `float(...)` there would be a change to Tier 1's output dressed as a type fix.) It matched on the first run, which is `pyrand`'s doing rather than luck. But **the digest is one deck and one seed**: `build_golgari`'s 99 names are all distinct, so it never exercises `list.remove` taking out an *equal* card, and its policy mulligans three times at most. The second corpus that covers both (`testdata/tier1.json`: 18 games, 7 runs, 390 castability cases, 527 floats) **caught the port's one real bug** — a `for` condition Go re-reads where `range(min(mulligans, len(hand) - 1))` is computed once, invisible below four mulligans. Nine of ten deliberate mutations die against it; the tenth is an equivalent mutant and is argued as one in the code. One thing it deliberately does not carry: `SimSummary.report()`, which is `cli.py`'s text table and no route reads. **This row named a second and it was already false when written** — it said `spells_through`'s value was left out because `sum` over floats is compensated from 3.12 and naive before it. The Python became `math.fsum` in the same change, so the value went into the corpus with everything else: 42 totals across seven runs as `repr` text, and 258 more as `Float64bits` once `sim/mulligan` crossed. Byte-identical under 3.11 and 3.12, verified by rendering under both and diffing. And **`canPay` is now a call to `mana.CanPay`** (the sim-engine lane, same day): `consume` keeps its own matching because it returns the leftovers, so the two Go solvers still exist, but `TestConsumeAgreesWithCanPay` holds them to each other — Python's Hypothesis pair, in Go. The digest did not move |
  | `GET /api/jobs`, `GET /api/jobs/{job_id}` — the caller's own jobs, and one by id | **go (hybrid)** | `go/internal/api/jobruns.go`, 2026-08-22 — **flipped with the sim family, as the hybrid this row argued for.** The row below records what it used to say; the short version is that these two could not flip while every job-submitting family was Python's, because a registry is per-process and a Go handler would have answered from one nothing wrote to. Two shapes were possible: all eight families and both routes in one change, or the hybrid built *with the first family flip*, where both branches are finally live. **The first turned out not to be reachable** — five of the eight families need `claude/` (Phase 6) and one needs `sim/tier3` (Phase 7), and neither engine has crossed — so the second was not a preference but the only shape available. The rules: **one by id is ours if we own it, otherwise proxied**, which settles ADR 5 without a second rule (a Go job belonging to somebody else misses the owner-scoped lookup, gets proxied, and Python 404s an id it has never seen — a 404 arrived at rather than asserted); and **the list is the union**, because a caller's jobs are spread across two registries and a list showing one of them is wrong in the way nobody can see. The door's proxy reaches `api` as a plain `http.Handler` on `Config.Upstream`, which is how the dependency inversion this row worried about is avoided: the door builds its proxy first and hands it over, and `api` never learns what an upstream is. **Python's rows are held as `json.RawMessage` and written back byte for byte**, and that is the load-bearing decision rather than an optimisation: decoding into a struct puts each job's `result` into a `map[string]any`, and `encoding/json` sorts a map's keys where a Python dict keeps insertion order — every simulation result, dossier and theme proposal would come back alphabetised, which is the regression that shipped on the deck page's Notes tab from v159 to v166. Exactly one field is parsed out of each row, the one the sort needs. `internal/door`'s `TestTheJobListIsTheUnion` and `TestAJobIdTheDoorDoesNotHoldIsProxied` pin both branches; `internal/api`'s `TestTheGenericJobRoutesAreTheHybrid` replaced the old tripwire and asserts neither route answers without consulting the upstream |
  | `sim/compile.py` — `deck.yaml` + pool → `SimCard`s, `CompileReport`, `NothingToSimulate`, `PoolRequired` | **go** | `go/internal/sim/compile`, 2026-08-22 — **the engine only; no route has flipped**, and the package `go/internal/sim` was written around: that package's comment already said "when it is ported it lands here." The two behaviours that are contract rather than detail both cross intact — a dropped card **shrinks the deck silently** unless `Unresolved` says so (the population every Tier 1.5 hypergeometric is computed over), and `ManaProduced` reads the amount off the **oracle text**, because `produced_mana` names colours and never amounts. Held to Python by `testdata/compile.json`: 60 oracle texts (every `pool` one read out of the real card pool and pasted verbatim, every `constructed` one a string nobody printed, aimed at a branch the real ones cannot reach), nine **hand-built pool records** for card shapes the 21-card pool does not hold, and nine whole decks compiled against `pooltest`'s real pool. Three findings the port made. **A deck where not one name resolves is refused as `PoolRequired`, not `NothingToSimulate`** — `get_cards` returns only what it found, so an empty mapping is indistinguishable from an absent pool; a wart, reproduced rather than fixed, and now pinned in both runtimes. **`category` is "utility" and Go's zero value is ""** — invisible to every tier and visible in exactly one place, the ADR 18 cache key. And four of the port's mutations survived the deck corpus entirely, because the pool holds no artifact-fronted DFC with a creature back, no land that is also a creature, no fetchland, and nothing whose `produced_mana` carries a string Scryfall would never send; the hand-built records exist for those four, and each of them then died. Checkable: `grep -rn 'sim/compile' go/internal/api go/internal/door go/cmd` prints **nothing** |
  | `sim/mulligan.py` — the 33-rule keep-rule grid search | **go** | `go/internal/sim/mulligan`, 2026-08-22 — **the engine only; no route has flipped**, so `POST .../policy` is still Python's. Its verdict is `Flat` measured **against the default**, never against the grid's range, and that is a decision rather than an implementation detail: the grid deliberately holds rules nobody would play, so a spread-based verdict would never fire. Held to Python by `testdata/mulligan.json` — the grid and its **order** (which decides ties, since `max` keeps the first extreme), plus eight full sweeps compared as `Float64bits`, because the table is a sort and the verdict is a `<` against `FLAT`, so one ulp is a different recommendation. **Two of the eight decks are built for this module and are in the corpus only because a mutation survived without them**: across ten real decks at seven sample sizes and six seeds, the winner was *never* tied, so nothing could observe which direction the mulligan-rate tie-break runs in or that the sort has a secondary key at all. `uncastable` (40 lands and 59 spells nobody can cast in the horizon) ties all 33 rows at 0.0 with twelve distinct mulligan rates; `quarter-steps` runs at **four games**, so every deployment is an exact multiple of 0.25 and rows land exactly `FLAT` apart — its seed was searched for, because `< FLAT` and `<= FLAT` name different `gentlest` rules only at a seed where the exact-`FLAT` row is also the gentlest one |
  | `sim/cache.py` — ADR 18's key, the `sim_cache` table, `fingerprint` | **go** | `go/internal/sim/cache`, 2026-08-22 — **the engine only; no route has flipped**. The question this lane had to answer deliberately: with both runtimes live, **a Go-computed row and a Python-computed row for the same deck and seed sit apart** — different keys, both in `sim_cache`, neither able to serve the other. That is the fingerprint doing its job. ADR 18's second consequence is that the engine's source is in the key so *no engine change can serve a pre-change number, including one nobody remembered to declare*, and two runtimes are the most extreme engine change there is; Tier 1's port is bit-exact against `REFERENCE_DIGEST`, but the cache may not be the thing that **assumes** that, because a colliding key would serve the other runtime's number under this runtime's name and look correct on the screen. The mechanical half is stronger than the prudential one: **a collision could not be arranged honestly**, since Go would have to hash `engine.py`'s bytes and the container has no Python after Phase 8. The cost is one recomputation per deck per runtime while both are live, and the Python rows age out through the `MaxRows` LRU. Go's fingerprint is a hash of **embedded Go source** — five packages, each carrying a `source.go`: `tier1`, `mana`, `sim`, `pyfloat` and `pyrand`. Each declares an explicit `//go:embed` list; the last three are fingerprinted where Python's counterparts are not, because `random`, `math.fsum` and the compiled card are CPython's or `engine.py`'s and cannot change under a running interpreter, while `internal/pyrand`, `pyfloat.Fsum` and `sim.Card` are ours. **That list said four packages, and named `sim.Fsum`, until 2026-08-22** — #249 moved `pyfloat` into a package of its own and this sentence rotted in two documents at once, which is why `tests/test_packaging.py` now reads the enumeration in both of them. `*.go` was rejected as the pattern: it matches `_test.go`, so every test edit would empty the deployed cache, and a test holds each list complete against its directory. The corpus records **the payload string** beside the key, not just the digest, because all three ways `encoding/json` differs from `json.dumps` present as the same opaque sixty-four characters — HTML escaping, `ensure_ascii` (the pool holds Bösium Strip and Déjà Vu; Go emits raw UTF-8 where Python writes `\uXXXX`), and float rendering (`extra` carries `mulligan.FLAT`). Handed Python's fingerprint, `Payload` reproduces Python's bytes exactly; that is what makes "the keys differ by one field" a claim rather than an unexamined difference between two serialisers |
  | `POST /api/sim/mana`, `/lands`, `/policy` — Tier 1 and the mulligan grid, as jobs | **go** | `go/internal/api/simruns.go` and `shelfruns.go`, 2026-08-22 — **Phase 5's flip, and the first job-shaped family to move.** Planning happens in the request and running in the job, exactly as Python does it: the ADR 18 key is a hash of the *compiled* deck, so knowing whether this is a hit means compiling first. A planning failure is carried into the job and returned from the worker rather than raised out of the route, because an optimisation that turns a reported error into a *different kind* of reported error has broken something. `deck_check` is attached **after** the cache on both paths, since the numbers are keyed on the compiled deck and the verdict is not — a rationale written or a `stage` promoted changes the verdict without moving a card. The land sweep caches per count, not per sweep, and its progress counts only the counts actually being simulated. `MOVES_THE_NUMBERS` crosses intact, because the client says something different about a banned card than about a missing rationale |
  | `POST /api/sim/shelf` — Tier 1.5's closed form, in the request | **go** | `go/internal/api/shelfruns.go`, 2026-08-22 — the one simulation route that is **not** a job, and it crossed as one: a plain route, no cache, no job row, because the closed form is arithmetic over an already-compiled deck measured at 0.03–0.04s. The sibling-duration rule came out different for the two routes in this module and stays that way |
  | `POST /api/sim/forge` — Tier 3 | python | **not part of the sim flip, despite the prefix.** It needs `sim/tier3`, which is Phase 7's engine and has not crossed. It is also what keeps the hybrid poll handler's proxy branch live for a simulation route, alongside the five Claude families |
  | the read side of `api/service.py` — the deck payload builders | **go** | `go/internal/deckread`, 2026-08-22 — extracted from `internal/api`'s handlers, and **the boundary guard chose the layering rather than a preference**. Python has always had this shape: `service.py` is what the routes call AND what `claude/tools.py` calls, so a tool result and a route payload are the same bytes by construction. The Go port had put that logic inside the handlers, which was fine while the routes were the only caller and stopped being fine the moment the Claude surfaces needed the same facts. Reaching them *through* `internal/api` was not an option: that package imports `internal/deckedit`, so `claude/boundary_test.go`'s transitive ban would have failed — correctly, since it would mean anything under the Claude surfaces could reach a deck write in one hop. So the shared code lives **below both**. `getDeck` went from 93 lines to 23; `Validate` and `Stats` exist as three-line functions for one reason, the **nil-versus-empty** distinction (`gate.Validate` is told the pool was never consulted by a NIL map and told the pool has never heard of these cards by an EMPTY one), which every caller getting right independently is a bug waiting for the second caller. The ordered-JSON helper moved to `internal/wire` in the same change, since two packages needed it and neither is the other's parent — which cost 124 composite literals their elided form, `go vet` flagging unkeyed cross-package fields that an in-package type never raised. Held to Python by the same `gate/testdata` fixtures the routes are held to, driven through the FUNCTIONS this time rather than through HTTP, because that is how the tools will call them |
  | `claude/stance.py` — the three axes, the presets, the ceiling | **go** | `go/internal/claude`, 2026-08-22 — Phase 6's first crossing, and the frame every mode plugs into, ported before any mode for the reason Python wrote it first: retrofitting a gate around modes that already exist is how the gate ends up with holes. Held to Python by `testdata/stance.json`, which is **exhaustive rather than sampled** — all 36 stances, all 1,296 clamp pairs — because the space is small enough that excluding nothing costs nothing, and `mana_oracle.py`'s hole was invisible for exactly as long as nobody asked what its sampler skipped. The corpus records **refusal text**, and that is what earned it: two divergences no structural test could see. Python repr-quotes with **single** quotes where Go's `%q` uses double, so `'nope' is not a stance preset` became `"nope" is not...` in a 422 body; and Python's json tells `7` from `7.5` **by the literal**, so `cannot read a stance from int` and `...from float` are two sentences that a plain `float64` decode collapses into one — fixed with `UseNumber`. Neither changes a stance; both change what a person reads. `describe()` is compared as **marshalled bytes**, since field order is the contract and only bytes carry it |
  | `claude/persona.py` — the seven voices | **go** | `go/internal/claude/persona.go`, 2026-08-22 — the roster crosses as **generated data** (`data/personas.json`, embedded), not as transcribed source: a voice is ~1.7KB of prose whose bytes reach a model, and hand-copying 11KB of English into a second language is the drift the generator exists to prevent. ADR 21's two structural claims cross with it. A voice is **appended, never substituted**, and last — `WithVoice` returns the base instructions unchanged for the plain persona, same bytes, so `converse`'s cache entry does not move. And `voice` **never reaches the client**: `RosterEntry` is its own type rather than a `Persona` with a `json:"-"` tag, so a field added to `Persona` cannot silently publish a prompt, and the guard is asserted at the type, at the generated data, and at the route — three places because they fail differently |
  | `tarot.py` — the 136-card deck, the weighted shuffle, the spread | **go** | `go/internal/tarot`, 2026-08-22 — **`internal/pyrand`'s first served caller**, and the reason it was built bit-exact rather than merely well-distributed: a seed minted by the Python door before the cutover must deal the same three cards from this one. The deck crosses as embedded data (its **order is part of the answer** — the sampler walks it accumulating weight); the shuffle, the reversals and `describe()`'s two detected paragraphs are code. Four of the corpus's seeds were **searched for** rather than swept, because a plain range reaches neither the Magic-card omen nor the doubled trump that `describe` itself calls the rarest thing the spread can do — seed 165 lands The Magician twice, once as the 1909 plate and once as Massimo. Two mutations survive the deal corpus and both are documented in the code rather than dropped: `mark < acc` versus `<=` is unobservable at ~2⁻⁵², and **`Fsum` versus a running total changes no spread at any corpus size** (200,000 seeds deal identically, measured), so the running total is tested **at the sum**, where it differs by 2 ULP, and driven through `weightedSample` rather than recomputed — the first version of that test called `pyfloat.Fsum` by hand and the mutation survived it |
  | `claude/ledger.py` — one row per conversation, and the roll-up | **go** | `go/internal/claude/ledger`, 2026-08-22 — the engine only. `Record` never fails the conversation that produced it (a dossier costs four minutes and real money; losing it because the accounting could not be written would be strictly worse than having no accounting), `app.db` is `mode=rw` and never `rwc`, and the row is **counters only** — no slug, no account, no question text, so ADR 17's who-may-read-what argument never has to be made for a table that would otherwise be a chat log, which a test asserts at the schema. Held to Python by eight roll-ups over six seeded rows; three mutations that a careless port would ship all die, the sharpest being **axis-dependent scan order** (the grouped column is SELECTed first, so `mode` and `model` swap position between the two queries and a fixed scan order puts the right numbers under the wrong names). `tiers.LabelFor` lands beside it, its family table deliberately NOT derived from the tier list: a ledger row can hold a model that is no tier at all, and resolving those through `Get` would label them with the default tier |
  | `claude/tools.py` — the seven read-only tools | **go** | `go/internal/claude/tools`, 2026-08-22 — the engine only. Schemas cross as **generated data** and dispatch stays **code**, which is the load-bearing split: a description is prescriptive prose about *when to call* and an under-described tool is the commonest reason a model answers from recall (rule 1's exact failure mode), while the calls have to be something `boundary_test.go` can see. Every handler goes to `internal/deckread`, so a tool result and a route payload are the same bytes by construction. Four properties each get a test: the allowlist is checked **on the name that arrived** rather than on what was advertised, a mode cannot widen its own set by asking, arguments are re-checked before dispatch (the API does not enforce `additionalProperties` without `strict`), and **no schema accepts a `why`**. `deckread.CardsNamed` lands with it and filters on NOTHING where the search filters to Commander-legal — a banned card comes back with its real oracle text and `legal_commander: false` rather than as a shrug, which was found by running a Claude turn and watching it answer from labelled recall, not by reasoning about it |
  | `claude/client.py` + `claude/modes.py` — the client, the `Mode`, and `converse` | **go** | `go/internal/claude/{client,mode,converse}.go`, 2026-08-22 — **the engine only; nothing flipped, and nothing calls it yet**, because no mode has crossed. `anthropic-sdk-go` v1.66.0 joins the module here. Three behaviours are the whole reason this is its own lane and they all cross intact: the **two cache breakpoints** (one fixed on the system block, which covers the tools because tools render first, and one *moving* onto the newest tool result — moved rather than added, since markers cap at four per request and the theme flow already spends one), the **container ride-along** on every turn after a server tool has run, and **`pause_turn` resumed and never returned**, which is the Forge-with-96-cards failure wearing a different hat. **This lane has no corpus, deliberately.** Every other Phase 6 package is held to Python by generated data, but the three claims above are properties of *the JSON that goes on the wire*, not of any value `converse` returns — a corpus over `Turn` would be blind to all of them, which is `tier1.Number`'s lesson arriving one phase later. So the loop is driven against a scripted API (`httptest`, reached through the real `Connect()` — the SDK honours `ANTHROPIC_BASE_URL`, so no production seam was added for testing) that keeps every request body, and the assertions are on those bodies. Thirteen deliberate mutations were driven before it was trusted, and the two that did not compile were fixed rather than counted. **The finding worth carrying is the SDK's `ExtraFields`**: it is a `map[string]any`, and the marshaller appends **novel** keys in Go's randomised map-iteration order while a key that *shadows* a struct field is substituted in place. Carrying the tool schema through it re-renders the tools block differently on every request — and since tools render first, that voids the entire prompt cache every single time, silently and for free. Measured, not guessed: one novel key gives one ordering, two give two. The schema therefore goes through the typed fields with exactly one novel key (`additionalProperties`, which the SDK has no field for), and the test asserts the **count** as well as the byte-stability, because the count is what fails deterministically. Two smaller divergences were found and fixed in passing: `converse` hands a recoverable tool failure back as `<PythonClassName>: <message>`, which needed the tools package to say which Python class each of its errors stands for (`PyName`) — and doing that exposed that Go's deck refusal read `no deck 'x'` where Python's `DeckNotFound(slug)` stringifies to the **bare slug**, so the model was being handed a different sentence; and the stance is now **validated** before the first turn, because `_scope_note` indexes a dict in Python (a KeyError) where a Go map lookup answers `""` and the mode would go out with its scope paragraph simply missing, visible nowhere. One Python worry does not survive the crossing: `sdk_installed()` has no analogue, since the SDK is linked into the binary rather than riding an optional extra, so `Available()` is the credential question alone. **Proved on the real wire**, 2026-08-22, by an opt-in live test (`MTGLAB_LIVE_CLAUDE=1`, skipped in CI and everywhere else) — the tool schemas validate, the model reaches for a tool, a refusal round-trips, and the breakpoints pay: **82 uncached input tokens for a two-turn conversation over a 2,791-token prefix**. That test asserts only what is invariant, and the reason is itself a trap worth recording — the cold-start ratio is the more interesting number and cannot be demanded, because the cache has a five-minute TTL and any earlier run leaves it warm, so a test insisting on it passes once and then fails for twenty minutes: a test manufacturing its own flake |
  | every mode's definition — prompt, tool set, response schema | **go** | `go/internal/claude/data/modes.json` + `modes.go`, 2026-08-22 — **all seven cross at once as generated data, including the modes Go has no orchestration for.** The definition is data; the code that assembles a brief and reads an answer back is the mode's own and crosses when it crosses. Rendering only the ported ones would make "has this mode crossed?" a question about two places. The response schemas are the sharpest case for generating rather than transcribing: **ADR 25's slot argument has no `defence`, `verdict` or `summary` property** and forbids extra ones, so the balanced answer — the attractive one, and a rationale generator wearing a hat — has nowhere to go; **ADR 34's scan has no field for a card name**. Those absences ARE the features, and an absence is exactly what a hand-copy drops with nothing looking wrong. Both are pinned by their own Go test rather than left to the generator. **The generator DISCOVERS the modes rather than listing them, and that is not fastidiousness — the first version was a list.** It was gathered by grepping for `= Mode(` and silently missed `scan.py`, which spells it `modes.Mode(...)`: seven became six, Go would have loaded six, every test would have agreed six was the number, and ADR 34's deliberate absence would have crossed as the absence of the whole mode. That is CLAUDE.md's four-times-recorded failure — a completeness claim inherited rather than re-checked — arriving in a new place, so the fix is mechanical: every module in the package is imported and every attribute that is a Mode is collected. Hosted tools cross as the API's own dicts and are mapped onto the SDK's typed unions at load, where a type nobody taught the switch about is a panic at startup rather than a mode that quietly went out without its search |
  | `claude/interview.py` — the rationale interview | **go** | `go/internal/claude/interview.go`, 2026-08-22 — **and the route flipped the same day** (`go/internal/api/interview.go`): `POST /api/decks/{owner}/{slug}/interview` is **the first Claude surface the door answers**, and deliberately the smallest one available — a plain route rather than a job, so the flip is a handler and not job plumbing. It stays plain for Python's own measured reason: the mode is *handed* its facts by `Brief` instead of being sent shopping for them, costs ~4,900 input tokens and no tool calls in the ordinary case, and sits in the seconds class. ADR 20's rule — a duration measured for one surface is a question to ask of every sibling — is asked here and answered no. Four things the route layer owns and the mode does not: the failure modes stay **apart** (422 for a question that is wrong, 404 for a deck this caller cannot see, 503 for no key at all, 502 for a call that came back unusable), and collapsing them is how somebody gets told their key is missing when the model was merely rate limited. The 502 default is **Python's and looks like a mistake**: `service.claude_interview` catches bare `Exception` around the whole of `ask()`, the brief included, so a pool failure while assembling the brief is a 502 there and is a 502 here. Two seams had to be built for it. **`auth.Scope` grew `ModelTier`** — the struct's comment had said the door has no use for it, which was true of every family that had crossed until a Claude route needed to know which seat is asking. And **`ErrStanceRejected`**, wrapped inside `Resolve` rather than in each mode so the six remaining modes inherit it. The Claude ledger shares the activity log's `app.db` handle rather than opening a second `mode=rw`. **The whole failure matrix was diffed against a running pair** — twelve cases through the door beside the same twelve straight to uvicorn — and that is what caught both of this lane's findings, neither of which any test or golden could see: the contract goldens record these bodies as `{"detail": "string"}`, which is true of every spelling of them. **One was a bug.** `Require` wrapped its reason as `fmt.Errorf("%w: …", ErrUnavailable)`, so the door answered *"claude is unavailable: no ANTHROPIC_API_KEY …"* where `str(ClaudeUnavailable(...))` is the bare reason — the sentinel's own words shipping as a prefix nobody wrote, into a `detail` the deck page renders verbatim; `unavailable` is now that error carrying the reason and nothing else. **The other was a wart, reproduced rather than fixed**, and it is the sharper one: a malformed stance is a **502**, not the 422 `api/app.py` appears to give. That route carries an `except ValueError` branch commented *"A malformed stance"* which raises 422 and is **dead code** — `service.claude_interview` re-raises only `ClaudeUnavailable`, `CardNotInDeck` and `DeckNotFound`, so a stance `ValueError` is swallowed by the broad `except Exception` and answered as `ClaudeFailed` long before the route's own branch is consulted. Go had been written to the docstring's *intent* and was therefore improving on Python, which is not a flip; it answers 502 now, in a branch that exists rather than falling through to the default so that the wart is visible at the one place somebody would fix it — one line here, the re-raise tuple there, both runtimes in one change, the way `edit.set_shared` went. Seventeen route tests, and the one that would not have existed without `tier1.Number`'s lesson asserts the report's **key order in the marshalled bytes**: `InterviewReport` had been proved field by field and never once put through `encoding/json`. `str(payload.get("card", ""))` is reproduced exactly, including the one input where it differs from the `or ""` spelling beside it — an explicit `null` reaches `str(None)` and asks the deck about a card called "None", which is a different 422 from a missing field. Proved on the real wire by a live case of its own (`MTGLAB_LIVE_CLAUDE=1`), which is the gate Phase 6 sets for each mode as it lands. The first mode, and the one ADR 15 was written before rather than after, because this is where rule 4 is easiest to lose. All three things that hold it cross: there is no write door (`boundary_test.go` covers this file automatically), the schema has no field for a rationale, and **every question is checked to be a question** — `OnlyQuestions` drops anything not ending in `?` and returns the count, because "it dropped two" is how a prompt that has started editorialising becomes visible. Thirteen mutations die against the tests, including the one that matters most: deleting the question-mark predicate. **The port found one real bug, and it is a Go-only one.** The brief is serialised straight into the opening message, so its key order is part of the bytes the model reads — and a nested block held as a bare `[]wire.KV` marshals as an array of `{"Key":..,"Value":..}` structs rather than as an object, because `MarshalOrdered` only recurses through values that are `wire.OrderedMap`. Still valid JSON, still gets an answer, from a model handed nonsense. The brief is `OrderedMap` all the way down now and a test asserts both the order and the absence of `"Key":` in the rendered bytes. Two smaller things worth the same care: the card is searched across the 99, the swap board AND the commander (all three have tests, and the swap-board one exists because a mutation survived without it), and a **deck-level gate issue carries no card** — reading one as being about this card is a nil dereference in Go where Python compares against `(i["card"] or "")` |
  | `GET /api/claude/personas`, `GET /api/tarot/reading` | **go** | `go/internal/api/claude.go`, 2026-08-22 — the Claude family's two free corners, flipped with the engine because both are deterministic and need no key, no pool and no network. The seed parameter is where this route's real divergences live, all three measured against the running app rather than reasoned about. **Starlette takes the LAST repeated value** where Go's `Query().Get()` takes the first, so `?seed=7&seed=9` is nine. **Pydantic's integer grammar is neither `strconv.ParseInt`'s nor Python's `int()`'s**: it accepts surrounding whitespace, a leading `+` and *single underscores between digits* (`1_0` is ten) — three 422s from a naive door — while refusing the fullwidth `７` that `int()` reads as seven, which is one 200 from a door written against `int()`. That last row is why the corpus oracle is `TypeAdapter(int)`: this lane first generated it from `int()` with a docstring asserting the two agreed, and they disagree on exactly one of twenty-four rows. And the seed is a **`*big.Int`**, because Python's integers are unbounded and `deal` echoes the seed it was handed — an int64 would truncate 2⁷⁰ into a different reading returned under a different number |
  | everything else under `/api` — `api/` (six of the eight job-submitting families, the rest of the Claude routes, `/api/health`, the upcoming sets), `decks/` (the wheel), the rest of `claude/`, `cli.py` | python | proxied to uvicorn on loopback; the two generic job routes have their own row above, since the reason they stay is not the reason these do |
  | `animist/`, `cardmotion` build, `bench/`, `mutate/` | python, permanently | ADR 38 decision 1 |
- **Flips are single PRs** with the contract run attached, deployed and
  walked before the next flip starts (main deploys itself; every flip is a
  release).
- **A board row is a claim about the code, and the code is what says whether
  it is true.** A row moves to **go** in the PR that *registers the routes*,
  never in the one before it — and the date on it is the date those routes
  landed, not the date somebody wrote the row. This is here because it was
  broken the day it was written: the accounts engine's PR (2026-08-22)
  described the account routes as **go** while `go/internal/api` had no such
  pattern in it, and a live capture showed `GET /api/auth/me` still reaching
  uvicorn. Nothing was served wrongly and the next PR made it true within
  hours — the cost is entirely to the *next session*, which reads this table
  as the frontier and would have been told a family was done while it was
  still proxied. Two of the three false completeness claims this project has
  found were in documents nobody could check; this one was checkable in one
  command, so **write rows that name the command** (`grep` for the pattern,
  `git show origin/main:` for the file) and run it before merging.
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
| Tier 1: same run, all cores | n/a (GIL; single worker by design) | **needs a machine with cores.** `nproc` on the instance answers 1 (2026-08-22), so the Go lane is one wide there too — this row is filled from a scaled machine, or from the dev Mac and labelled as the dev Mac; it is not a fact about the deployment either way |
| `data refresh` (`load_printings`) | ~16 min / 107,355 rows | |
| Idle RSS on the instance | 127 MB (peak 215 MB, 8 threads; 2026-08-21, 2h39m after deploy) | |
| Image size (compressed) | 121.3 MB, 10 layers (registry manifest); ~325 MB unpacked, 219 MB of it the venv | |
| Boot → healthy | ≈23 s machine update → health passing (deploy log, bounded below by the check cadence); merge → instance healthy 7 m 44 s; whole pipeline 9 m 37 s | |
| Direct dependencies (served app) | 8 Python (75 installed dists) | |
| `REFERENCE_DIGEST` | pinned | must match (or superseding pin + ADR note) |
| Mana oracle 13,944 cases | pinned digest | must match |
