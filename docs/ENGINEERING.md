# Engineering direction

Where the project goes next, and *why* — written for the case where peers read
this repo and play-test the tool.

The audience matters. Engineers reviewing a solo project are not impressed by a
technology list; they are impressed by decisions that survive the question
"why did you do it that way?" This document tries to answer that question in
advance, including in the places where the honest answer is "we measured, and
it wasn't worth it."

**The compiled-language rewrite is deferred**, with a written trigger — see §1
and the deferred list at the end. The near-term work is testing rigor,
container hardening and CI, none of which needs a second language.

---

## 1. A compiled backend — deferred, with a trigger

**Decision: stay on Python for now.** Not "not yet, probably soon" — genuinely
deferred until a measurement says otherwise, with the trigger written down
below so the call gets re-made on evidence rather than on appetite.

### Do not rewrite Tier 1

The instinct is to port `sim/tier1/` to Rust. The measurements say don't:

| Metric | Measured |
| --- | --- |
| One goldfish game | 0.89 ms |
| 20,000 games (`sim mana`) | ~18 s |
| 11-count sweep at 25,000 games | ~4 min |
| Hot spot | `_consume`, ~50% incl. callees |
| Process parallelism | 1.65x at 2 workers, 2.42x at 4 |

An 18-second batch job, run a handful of times a week, on a machine that is
idle ~99% of the time. A 30x speedup saves seventeen seconds of wall clock
nobody is waiting on. Meanwhile caching results by deck-content hash removes
most of the work outright, because deck files change rarely and the same
numbers get viewed repeatedly.

**A reviewer will ask what the rewrite bought.** "It's faster" is not an
answer when the profile says the workload was never the bottleneck. Porting
Tier 1 for its own sake is the textbook resume-driven rewrite and it reads that
way. Skip it.

### Tier 2 is the candidate — but build it in Python first

Tier 2 — the pod simulator — is the only workload here with a plausible case
for a compiled language. Four seats making actual decisions over more turns is
maybe **50–100x the work per game**. At Tier 1's measured 0.89 ms/game that
projects to 45–90 ms per game, so a 10,000-game matchup is 8–15 minutes and a
six-deck round-robin is fifteen of them.

**That projection is an extrapolation, not a measurement**, and it is doing a
lot of work. The honest move is to build Tier 2 in Python, measure it, and
only then decide. Writing Rust against a guess about a simulator that does not
exist yet is the same mistake as porting Tier 1, one level removed.

### The trigger

Reopen this decision when **any** of these is true, and not before:

- A single Tier 2 matchup at 10,000 games takes **> 5 minutes** after
  profiling and after the cheap wins below.
- A full six-deck round-robin cannot finish in **under an hour** on the dev
  machine.
- Profiling shows a genuine hot loop that is not fixable in Python — that is,
  the time is in arithmetic and branching rather than in allocation,
  attribute lookup, or an algorithm that should be better.

Exhaust these first, in order, because they are cheaper than a port and some
of them are worth doing regardless:

1. **Cache by deck-content hash.** A sim result is a pure function of
   `(deck content, parameters, seed)`. Most repeat runs should cost nothing.
2. **`multiprocessing` across games.** Measured at 2.42x on 4 workers, ~20
   lines, and games are trivially independent.
3. **Algorithmic work in the policy engine**, which is where a pod simulator's
   time will actually go — not in the mana solver.
4. **`__slots__`, precomputed lookup tables, and avoiding per-game
   allocation** in the inner loop. Ordinary Python optimisation routinely buys
   2–5x on this shape of code.

If the trigger fires, the plan below is what to do. If it never fires, that is
a good outcome and the extrapolation above was simply wrong — which is worth
recording either way.

### If the trigger fires: Rust, not Go

**Rust over Go, for this specific shape.** The hot loop is a library embedded
in a Python process, not a service. Rust via **PyO3 + maturin** keeps one
deployable, one process, and no serialisation boundary between the API and the
engine. Go would need cgo (awkward, loses much of Go's ergonomics) or a
separate process with IPC (a network hop and a second thing to deploy, for a
function call). Rust also gets `rayon` for the trivially-parallel game loop
and `criterion` for benchmarks CI can gate on.

The seam already exists on purpose: CLAUDE.md keeps `mana.py` and `sim/`
stdlib-plus-numpy precisely so they could move.

### Assembly: the honest answer

Hand-written assembly in a Commander simulator would read as unserious to the
audience this is meant to impress. It is unportable, it defeats the optimiser
more often than it beats it, and there is no kernel here where the compiler is
demonstrably leaving performance on the table.

There *is* a defensible low-level story, and it is SIMD:

- The Tier 2 inner loop shuffles and draws constantly. A batched
  xoshiro256++ generating lanes of random numbers with `std::simd` (or
  `wide`/`pulp`) is a legitimate optimisation with a real speedup.
- Batching *games* rather than cards — simulating 8 games in lockstep across
  SIMD lanes — is the structure that actually exploits it.

The rule that makes this impressive rather than indulgent: **profile first,
benchmark the scalar version, and keep it.** A `criterion` bench showing
"scalar 12.4 ms vs SIMD 3.1 ms, same output for the same seed" is engineering.
The same code with no benchmark is decoration.

### The thing that will actually impress a reviewer

**Differential testing between the Python reference and the Rust
implementation.** Keep `sim/tier1/` in Python as the executable specification.
When the Rust engine lands, run both over the same seeds and assert identical
output. That:

- justifies keeping both implementations instead of looking like dead code,
- turns "I rewrote it in Rust" into "I rewrote it in Rust and proved it agrees
  with the reference on N thousand seeded games",
- and catches the porting bugs that would otherwise surface as subtly wrong
  percentages in a primer.

This is the single highest-value item in this document.

---

## 2. Property-based testing, before any rewrite

`mana.py` solves a bipartite matching problem: can this pool of sources pay
this cost? It is subtle enough that `tests/test_mana.py` exists specifically to
pin cases where naive source-counting is wrong.

That is a textbook fit for **Hypothesis**. Generate random costs and random
mana pools, then assert the solver agrees with a deliberately slow brute-force
oracle that tries every assignment. Any disagreement is a real bug, found
automatically, in the most correctness-critical function in the project.

Do this *before* the Rust work, because the same generated cases become the
differential-test corpus for the port.

Cheap adjacent wins:

- **Determinism tests.** A seeded simulation must produce identical output
  across runs, platforms and Python versions. Cheap to assert, and it is the
  precondition for differential testing to mean anything.
- **Mutation testing** (`mutmut`, or `cargo-mutants` on the Rust side) on
  `decks/validate.py` and `mana.py`. Coverage says a line ran; mutation
  testing says a test would have *noticed* if it were wrong. 87% line coverage
  with a poor mutation score is a thing worth knowing about your own suite.

---

## 3. Containerisation

Already unblocked: `config.py` reads `MTGLAB_DATA_DIR` and `MTGLAB_DECKS_DIR`,
which was the last thing tying the app to a working directory.
`docs/HOSTING.md` has a working `Dockerfile` and `fly.toml`.

What to add for a repo that gets reviewed:

- **Multi-stage build.** Node stage builds the frontend, Python stage installs
  the package, final stage is a slim runtime. Today `web_dist/` is committed
  so the image needs no Node — keep that property, but the CI image build
  should prove the bundle can be rebuilt from source.
- **Non-root user**, read-only root filesystem, and a `HEALTHCHECK` hitting
  `/api/health` (which already reports corpus state).
- **Multi-arch** (`linux/amd64`, `linux/arm64`) via buildx. The dev machine is
  Intel; anything modern people deploy to is arm64.
- **Image scanning** (Trivy or Grype) as a CI job, failing on HIGH/CRITICAL.
- **Never bake the corpus in.** Already documented — Scryfall asks that bulk
  data not be redistributed, and it belongs on the volume.

---

## 4. Frontend

The stack is already modern: React 19, Vite 8, Tailwind 4, TypeScript 6,
oxlint, Recharts. The gaps are testing and interaction, not framework choice.

- **No frontend tests at all.** Vitest + Testing Library on the pieces with
  real logic — the filter/sort in `Library.tsx`, the pip-vs-sources maths, the
  job-polling state machine in the simulator. That last one is a genuine state
  machine (queued → running → done/error) and it is untested.
- **Playwright** for a handful of end-to-end paths against the real API. The
  card-search field that collapsed to 14px would have been caught by one
  assertion on a rendered width.
- **Accessibility**: `axe-core` in CI. Cheap, and reviewers notice.
- The bundle is 661 kB (195 kB gzipped) and Vite already warns. Route-level
  code splitting is the obvious fix if it ever matters.

---

## 5. CI/CD

Current pipeline: pytest on 3.11/3.12, coverage with an 80% floor, ruff,
frontend typecheck + build, committed-bundle drift check, and a secrets/corpus
guard. That is already better than most solo projects. The ladder from here,
roughly in order of value per unit of effort:

### Claude review on every PR

`anthropics/claude-code-action@v1` is the official action. Verified workflow
shape:

```yaml
name: Claude review
on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
      id-token: write
    steps:
      - uses: actions/checkout@v6
        with: { fetch-depth: 1 }
      - uses: anthropics/claude-code-action@v1
        with:
          anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
          prompt: |
            REPO: ${{ github.repository }}
            PR NUMBER: ${{ github.event.pull_request.number }}
            Review this PR. This is a Magic: the Gathering toolkit; read
            CLAUDE.md first. Weight these heavily:
              - Any card behaviour asserted from memory rather than looked up
                in the corpus. This project has a written history of exactly
                that going wrong.
              - Numbers in prose or generated artifacts that no test pins.
              - Silent-wrong-answer risks: a change that makes a simulation or
                a gate report something confidently incorrect.
            Use `gh pr comment` for top-level feedback and
            `mcp__github_inline_comment__create_inline_comment` for specifics.
          claude_args: |
            --allowedTools "mcp__github_inline_comment__create_inline_comment,Bash(gh pr comment:*),Bash(gh pr diff:*),Bash(gh pr view:*)"
```

Set up with `/install-github-app` from an interactive `claude` terminal, which
handles the GitHub App and the `ANTHROPIC_API_KEY` secret. Note the review is
billed per run, so gate it on `pull_request` rather than every push if that
matters.

`anthropics/claude-code-security-review` is a separate official action for
security-focused analysis and is worth adding alongside.

There is also **`/code-review ultra`**, a multi-agent cloud review you trigger
yourself from the terminal on the current branch or a PR number. It is
user-triggered and billed, so it cannot be wired into CI — but it is the
heavier tool for a change worth the scrutiny.

### Adversarial testing, which is the part that has teeth

An LLM reviewer is a good reader, not a proof. The adversarial rigor should be
mechanical:

- **Differential tests** (§1) — Python reference vs Rust implementation.
- **Property-based tests** (§2) — Hypothesis against a brute-force oracle.
- **Fuzzing.** `deck.yaml` is parsed input. `cargo-fuzz` on the Rust parser, or
  Hypothesis on the YAML loader, should never produce a crash or a silent
  mis-parse — only a clean validation error.
- **Mutation testing** to grade the suite rather than the code.
- **Golden artifacts.** The five generated documents are the shareable
  surface. Snapshot them and diff in CI, so a formatting change is a visible
  decision instead of an accident.

### Supply chain and release

- Pin actions by commit SHA, not tag.
- `dependency-review-action` on PRs; Dependabot or Renovate for updates.
- Generate an SBOM (`syft`) and attach it to releases.
- Sign container images with `cosign`, publish via OIDC rather than long-lived
  registry credentials.
- Tagged releases with generated notes; the version in `pyproject.toml` is
  still `0.1.0` and should start moving.

### Benchmarks as a gate

Once Rust lands: `criterion` benchmarks in CI with regression detection, so a
performance claim in a PR description is backed by a number the pipeline
checked. This is also what makes the SIMD work legible.

---

## 6. Architecture Decision Records

The most transferable rigor available here, and it costs almost nothing.

Add `docs/adr/NNNN-title.md`, one per significant decision, each recording
context, the options considered, the decision, and the consequences. Seed it
with the ones already made and argued in this repo:

1. `deck.yaml` in git as the source of truth, and deck history as git history.
2. DuckDB for the corpus; why not Postgres or SQLite.
3. Tier 1 stays Python; Tier 2 goes to Rust — with the measurements.
4. Two embedded databases for hosting (DuckDB read-only + SQLite read-write).
5. Sessions over JWTs, and no self-signup.
6. Never redistribute Scryfall bulk data; refresh at boot instead.

A reviewer who reads ADR 3 and finds a profile, a table of numbers, and an
explicit "we did not port Tier 1, here is why" learns more about the engineer
than any amount of Rust would tell them.

---

## Suggested order

Decided 2026-08-10: do this list before any hosting work. Hosting is not
imminent, but see "Cloud-compatible by construction" above — steps 3 and 5
exist partly so that when it happens it is additive.

1. **Property-based tests on `mana.py`** plus determinism tests. Cheap, finds
   real bugs, and builds the corpus a port would be tested against.
2. **ADRs for the decisions already made.** An afternoon, and it reframes the
   whole repo for a reader.
3. **A `DeckSource` abstraction and a request scope.** Four call sites and one
   dependency. Makes the API testable against an in-memory source now, and
   makes user decks additive later.
4. **Frontend tests** (Vitest on the job-polling state machine and the
   filters, which are the only pieces with real logic).
5. **Container hardening** — multi-stage, non-root, multi-arch, scanned,
   health-checked. Proves the deployment story without deploying.
6. **Tier 2 in Python**, then profile it.

Steps 1–5 make the existing project defensible without adding a language, and
leave hosting a matter of adding an auth layer and a second deck source rather
than reworking what is here.

Automated PR review (§5) is **deliberately parked** — priced out 2026-08-10.
Copilot Free does not include PR review; Copilot Pro is $10/mo and consumes
Actions minutes on a private repo; the Claude action is pay-per-run and needs
an API key. Revisit when PR volume justifies it.

## Cloud-compatible by construction

Hosting is not imminent, but it should not require a rewrite when it happens.
This section is the short list of seams to get right *during* the near-term
work, so that hosting is additive. It is deliberately short — most of the
codebase needs nothing.

**Already done, do not regress:**

- Paths come from `config.py` and honour `MTGLAB_DATA_DIR` /
  `MTGLAB_DECKS_DIR`. A container mounting a volume at `/data` just works.
- `api/` does not import `cli.py`. The web layer has no command-line
  dependency to untangle later.

**Worth doing now, because it is cheap now and invasive later:**

1. **A deck source abstraction.** The API reads the filesystem directly in
   exactly four places (`service.py`: the health count, `_load_deck`, and the
   library listing). Hosting's two-tier model keeps the curated decks
   file-backed *permanently* and adds user decks from SQLite — so a
   `DeckSource` protocol with a `FileDeckSource` implementation is not
   scaffolding, it is the shape the system ends up with. Adding
   `SqlDeckSource` later becomes additive instead of touching every endpoint.
   It also makes the API testable against an in-memory source, which serves
   the near-term testing goal on its own.

2. **A request scope, even with one implementation.** Nothing models "who is
   asking". Six of thirteen endpoints are deck-scoped. Introducing a single
   FastAPI dependency that yields the caller's deck source — returning the
   file-backed library for now — means auth later swaps *one* implementation
   rather than rewriting handlers. `docs/HOSTING.md` §1 already requires all
   user-scoped queries to go through one accessor for isolation; this is that
   accessor, built before it has anything to isolate.

**Accepted, not oversights:**

- `api/jobs.py` is an in-process registry. That is correct for a single
  machine with scale-to-zero, and externalising it would only matter for
  multiple instances — which the hosting plan does not call for. Revisit only
  if the deployment ever runs more than one.
- The frontend needs no preparation. Login is a route, a session context and
  401 handling; all of that is additive to what exists. Do not pre-build it.

## Deferred until measured

Parked deliberately, not forgotten. Each needs evidence before it starts.

| Item | Reopen when |
| --- | --- |
| **Tier 2 inner loop in Rust** (PyO3, rayon, criterion) | The §1 trigger fires — a matchup over 5 minutes, or a round-robin over an hour, after caching, multiprocessing and ordinary Python optimisation |
| **SIMD-batched RNG** | A profile of the *Rust* engine shows the generator and shuffle are hot. Not before Rust exists |
| **Differential testing Python vs Rust** | Ships with the port, not separately — it is what makes the port defensible |
| **Go** | Only if the engine ever becomes a standalone service rather than a library in-process. Not the current shape |

Note that two items in the sections above stay on the near-term list even
though the port is parked, because they pay off on the Python engine on their
own terms: **property-based testing of `mana.py`** (§2) and **determinism
tests**. Both are also prerequisites for a credible port later — the generated
cases become the differential corpus — so doing them now is not wasted work in
either branch of the decision.
