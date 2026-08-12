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

> **Amended 2026-08-11.** [ADR 14](adr/0014-python-decides-claude-advises.md)
> makes Forge the thing that plays real games, and `ROADMAP.md` now defers
> Tier 2 behind a Forge feasibility spike. **The trigger below is unchanged in
> shape and now waits on whichever simulator gets built first.** If that turns
> out to be Forge, this decision may never reopen at all: the expensive loop
> would live inside a JVM this project does not maintain, and the Python side
> would be orchestration, parsing and a card-coverage pre-flight — none of
> which is arithmetic in a hot loop. That is a *better* outcome than a Rust
> port, not a deferral of one. The rest of this section stands as written, and
> describes the Tier 2 case if Tier 2 is what gets built.

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

## 2. Property-based testing — built 2026-08-10

`mana.py` solves a bipartite matching problem: can this pool of sources pay
this cost? It is subtle enough that `tests/test_mana.py` exists specifically to
pin cases where naive source-counting is wrong. Those are examples, and
examples only cover what someone thought of.

`tests/test_mana_properties.py` covers what nobody thought of. Hypothesis
generates costs and pools, and each one is checked against two independent
references in `tests/mana_oracle.py`:

- `brute_force_can_pay` — enumerate every injective assignment of pips to mana
  units. Factorial, and there is nothing in it to get wrong beyond the
  definition itself.
- `hall_can_pay` — Hall's marriage theorem: a matching covering every pip
  exists iff every subset of pips has at least as many usable units as it has
  pips. A different theorem, so it fails differently instead of sharing a blind
  spot with the search.

Neither shares code with `mana.py`. The unit expansion is reimplemented inside
the oracle on purpose: a reference that imports the implementation cannot catch
a bug in the part they share.

Alongside the generated cases sits an **enumerated corpus** — 13,944
(cost, pool) pairs over a small alphabet chosen for structure rather than
realism, built with `combinations_with_replacement` rather than from a seed. It
yields the same cases in the same order on any machine in any language,
forever, which is what makes it usable as the differential corpus for a port
(§1). `python tests/mana_oracle.py` dumps it as JSON Lines; `--digest` prints
the hash pinned by `CORPUS_ANSWER_DIGEST`.

### What it found

**The solver is clean.** Every generated case and all 13,944 corpus cases agree
with both oracles, as do the monotonicity properties (an extra source never
hurts, widening a pip never hurts, a dearer cost is never easier) and the
order-invariance ones. `can_pay` and `engine._consume` — a second solver, which
exists because casting needs the leftovers rather than a yes/no — agree with
each other on every case. That pairing is the differential test §1 wants,
available today without a second language.

**The parser was not.** Phyrexian mana was dropped outright, so `{U/P}` parsed
to mana value 0 with no colours. Scryfall says Mental Misstep is cmc 1 and
blue. `decks/analyze.py` builds the curve from that number, so the Tivit list
filed Mental Misstep as a 0-drop and Phyrexian Metamorph as a 3-drop, and
reported an average mana value of **1.90 where the truth is 1.93**. Fixed by
keeping Phyrexian symbols in their own `ManaCost.phyrexian` field: they count
toward mana value and colour identity, and — correctly, because 2 life pays
them — still place no demand at all on the mana base. The corpus digest did not
move, which is the evidence that castability semantics did not change with it.

Two things about that bug are worth keeping. It was not in the clever algorithm
a reviewer would scrutinise; it was in the boring parser feeding it. And
`mana.py` was at **100% line coverage** with the faulty branch covered — by a
test that asserted the half of the behaviour that was right (`{G/P}` is not a
mana constraint) and never asked about the half that was wrong.

### Determinism — built 2026-08-10

`tests/test_determinism.py`, in three levels of increasing strength:

- **Within a process.** Seeded `run`, `simulate_game` and `sweep_land_counts`
  repeat exactly; two different seeds differ, which is what catches a seed that
  is accepted and then ignored; the caller's library is not mutated; the
  progress callback is inert; and every point of a sweep provably starts from
  the same state, which is what makes a sweep a comparison at all.
- **Across processes.** `tests/determinism_probe.py` runs a fixed simulation in
  a fresh interpreter, because hash randomisation is set at process start and
  cannot be varied in-process. At two `PYTHONHASHSEED` values both the Tier 1
  digest and the mana corpus digest are unchanged: nothing here reads set or
  dict iteration order.
- **Against a pin.** `REFERENCE_DIGEST` is a golden, verified byte-identical on
  CPython 3.11.15 and 3.12.13. A change in what Tier 1 reports now has to be a
  decision someone writes down rather than a number that drifts.

A side result worth recording: the probe runs on a bare 3.11 with **numpy not
installed**. Tier 1 is pure stdlib today. That is the CLAUDE.md dependency rule
holding in practice, and it is what would make the seam in §1 cheap to cut if
the trigger ever fires.

### The suite CI ran was not the suite anyone was reading — fixed 2026-08-12

Worth recording as its own finding, because it invalidated every coverage
number in this document until it was fixed, and because nothing about it was
visible from a green check.

`data/mtg.duckdb` is a ~500MB Scryfall download that CI does not have and
should not have (ADR 6). So 29 tests opened with some variant of

```python
if not client.get("/api/health").json()["corpus"]:
    pytest.skip("no corpus available")
```

and every one of them passed on the maintainer's laptop and skipped on every
pull request. What skipped was not incidental: the entire card-fact surface —
swap, add, suggestions, card search, deck creation, the Tier 1 job endpoints,
and the Claude corpus tools. **The layer that exists to enforce rule 1 was the
layer no pull request ever exercised.** Locally the suite ran 692 tests at 87%;
CI ran 663 at 82%, and reported success.

The fix is `tests/tiny_corpus.py`, which now builds a genuine DuckDB corpus of
21 real cards in about a second. Two things made that work rather than merely
look like it:

- **The cards are real, read out of the corpus and pasted verbatim.** Rule 1
  applies to test data: a fixture naming Ajani and then claiming mono-white
  would teach the exact error CLAUDE.md cites. Ajani is in the fixture *with*
  its {R}{W} identity, so that cautionary tale is now executable.
- **The decks are synthetic.** `mono_green_deck()` is a legal 99 built only
  from those 21 cards, shaped like Goreclaw's real list — mono-green
  commander, exactly one banned card — so the same assertions run without
  needing all 35,000. Pointing the real decks at a 21-card corpus would have
  been the other option and a worse one: 90 unknown-card errors per deck, and
  tests that assert cleanliness would have had to be weakened to survive it.

Result: **740 tests and 90% coverage, with or without the real corpus.** Two
tests still need the full download and now say so with a `needs_full_corpus`
marker rather than a bare skip — the 32-way colour-combination check (a fixture
holding those 32 cards would verify the fixture rather than the table) and the
commander-search ordering bug (which needs more cards than the query limit to
reproduce at all). CI fails if that count moves, which is the control that was
missing: **a suite that quietly shrinks cannot be caught by a suite that
quietly shrinks.**

### Still open

**Mutation testing** (`mutmut`, or `cargo-mutants` on the Rust side) on
`decks/validate.py` and `mana.py`. Coverage says a line ran; mutation testing
says a test would have *noticed* if it were wrong. The Phyrexian bug above is
the argument in one example: 100% line coverage, and a mutation of that branch
would have survived.

**A mode's tool set is advertised, not enforced.** Found while writing
`tests/test_claude_modes.py`: `Mode.tool_names` decides which schemas the model
is *shown*, but `tools.run` dispatches against the global `READ_ONLY` registry,
so a model asking for a registered tool the mode did not offer gets a real
answer. The blast radius is bounded — all seven tools are read-only, and
`test_claude_boundary.py` proves the package cannot name a write function at
all — so this is a tidiness question rather than a safety one. It is recorded
rather than fixed because narrowing dispatch to the mode is a decision about
what ADR 15 means by "a mode is a tool set", not a bug fix.

---

## 3. Containerisation — built 2026-08-12

`Dockerfile`, `docker-entrypoint.sh`, `.dockerignore` and `fly.toml` are in the
repository, and the `image` job in CI builds and exercises the image on every
pull request. `docs/HOSTING.md` §4 is the deployment guide; this is the review
checklist that shaped the files.

- [x] **Multi-stage build** — builder installs into a venv, runtime copies it,
      so pip and any future compiler stay out of the shipped image.
- [x] **No Node stage, deliberately**, which is where this list argued with
      itself. It asked both that the no-Node property be kept and that the image
      build prove the bundle rebuilds from source. Those pull opposite ways, and
      the second was already satisfied — the `frontend` job runs the real
      `npm run build` and fails on any diff against the committed
      `src/mtglab/web_dist/`, on every PR. Proving it again inside the image
      would be a slower duplicate bought by making the image depend on Node and
      the npm registry. **CI proves the bundle is current; the image ships it.**
- [x] **Non-root user.** The app process runs as `mtglab` (uid 10001). It gets
      there through an entrypoint rather than a `USER` line: Fly attaches the
      volume owned by `root:root` and the mount shadows the image's own
      ownership, so PID 1 starts as root, fixes `/data`, and `exec`s the app
      under `setpriv`. A bare `USER` would look stricter and leave the app
      unable to write its own volume. CI asserts the owner of PID 1 in the
      running container, which is the claim that matters.
- [ ] **Read-only root filesystem.** Not done, and now *possible* rather than
      contradictory: it required decks to stop living in the image, which they
      have. Everything the app writes is under `/data`. Left off because Fly's
      `fly.toml` has no switch for it, so it would only bind a plain
      `docker run`, and an untested claim in a deployment file is worse than an
      absent one.
- [x] **`HEALTHCHECK` hitting `/api/health`**, using stdlib `urllib` rather
      than installing `curl` for one request. That path is on `PUBLIC_PATHS`,
      so it answers with auth on, and CI pins that it reports `"corpus": false`
      on a fresh volume instead of failing — an unseeded instance is a correct
      state between deploy and seeding, and a health check that 500s there
      would have the platform restarting a healthy machine forever.
- [x] **Multi-arch** (`linux/amd64`, `linux/arm64`) via buildx. Nothing is
      pushed; publishing needs the signing and provenance conversation in §5.
- [x] **Image scanning** — Trivy, failing on HIGH/CRITICAL, `ignore-unfixed`
      because a CVE with no available patch is not something the build can act
      on and would turn the gate into noise that gets disabled.
- [x] **Never bake the corpus in.** Scryfall asks that bulk data not be
      redistributed, and it belongs on the volume. Enforced twice now: the
      `no-secrets-or-corpus` job checks what is *tracked*, and the `image` job
      greps the *built image*, which is a different question the moment a
      `.dockerignore` line is deleted.

**The reason CI carries so much of this.** The maintainer's machine is macOS 12
on Intel: Docker Desktop supports the three most recent macOS releases and will
not install, and Homebrew there is too stale to build Colima. **No container can
be built on it at all.** So the `image` job is not belt-and-braces — it is the
only place this Dockerfile is ever built, and every property above is asserted
against a running container rather than read off the file.

---

## 4. Frontend

The stack is already modern: React 19, Vite 8, Tailwind 4, TypeScript 6,
oxlint, Recharts. The gaps are testing and interaction, not framework choice.

- ~~**No frontend tests at all.**~~ **Built 2026-08-10** — Vitest + Testing
  Library, 35 tests over the three pieces with real logic. `npm test` runs
  them; CI runs it before the build, so a broken component fails as a failing
  test rather than as a bundle that builds fine and misbehaves in a browser.

  - **`followJob`**, the job-polling state machine (queued → running →
    done/error, plus cancel). Timers are faked, so the 400ms interval is
    asserted rather than waited out, and the cancel path is pinned — an
    unmounted Simulator screen must stop polling, not poll forever.
  - **The library filter and sort**, including the two filters applied at once,
    and the colour filter being a membership test rather than an exact identity
    match.
  - **`identityName`**, which is a lookup table plus a rotation trick to make it
    order-independent — exactly the code that works for the cases someone tried
    and quietly fails for the rest. All ten guilds, all ten shards and wedges,
    five-colour, mono and colourless are pinned.

  Deliberately *not* tested: the pip-vs-sources maths named in the original
  plan. It turned out to be computed server-side in `decks/analyze.py`, where
  `tests/test_analyze.py` already covers it; the frontend only displays the
  number. Writing a frontend test for it would have tested nothing.

  **It found a real bug.** `/api/decks` carries `errors: null` to mean "the
  corpus was missing, so the gate never ran", explicitly so that it is not
  rendered as a pass — and the library card rendered it exactly like a clean
  deck, because the condition was `errors !== null && errors > 0`. With no
  corpus, Goreclaw and Atla Palani, both of which run a banned card, looked
  precisely as clean as the four decks that pass. Fixed with a `not checked`
  badge, and pinned by the test that caught it.

- **Replacement suggestions, added 2026-08-10.** `decks/suggest.py` scores
  similarity to a card being removed and `mtglab decks suggest` / the deck
  page's validation tab surface a shortlist. Scoring is pure over
  `CardRecord`s, so 17 tests cover it without a database; only the pool query
  touches DuckDB. It reports and never edits, which is
  [ADR 8](adr/0008-the-gate-blocks.md) held rather than revisited.

  The honest limitation, recorded because the top of a ranked list reads like
  an answer: similarity is not quality. It ranks Regal Behemoth above
  Cultivator Colossus for Goreclaw's Primeval Titan slot purely on a one-mana
  curve difference, where Colossus is the closer fit. The fix for that is a
  human reading five candidates, not weights tuned until one deck comes out
  right.

- **A four-colour deck would render as "WUBR".** Found while testing
  `identityName`: the four-colour names (Yore-Tiller, Glint-Eye, Dune-Brood,
  Ink-Treader, Witch-Maw) are not in the table. Latent — none of the six
  curated decks is four-colour — so it is recorded in the test rather than
  fixed on speculation.
- **Playwright** for a handful of end-to-end paths against the real API. The
  card-search field that collapsed to 14px would have been caught by one
  assertion on a rendered width.
- **Accessibility**: `axe-core` in CI. Cheap, and reviewers notice.
- The bundle is 661 kB (195 kB gzipped) and Vite already warns. Route-level
  code splitting is the obvious fix if it ever matters.

---

## 5. CI/CD

Current pipeline, as of 2026-08-12: pytest on 3.11/3.12, coverage with a **90%**
floor, a **skip-count gate**, ruff, **mypy**, frontend typecheck under
**`strict`**, **oxlint with `--deny-warnings`**, `npm test`, the build,
committed-bundle drift check, a secrets/corpus guard, and — since
containerisation landed — an **`image` job** that builds the Dockerfile for two
architectures, runs it, and scans it (§3).

Four of those are new, and three of the four are guards against a check being
green while not checking:

- **The skip gate.** See §2 — CI ran 663 of 692 tests and said so nowhere. The
  build now fails if the skipped count is anything other than the two declared
  `needs_full_corpus` tests.
- **`tsc` under `strict`.** `tsconfig.app.json` had `noUnusedLocals` and
  friends but never `"strict": true`, so `strictNullChecks` was off across all
  5,913 lines of frontend and a null deref type-checked clean. Turning it on
  produced **zero errors** — the code was already written defensively; what was
  missing was anything holding it that way. It immediately caught one latent
  problem: Recharts types `dataKey` as `string | number | ((obj) => unknown)`,
  and a function is not a valid React key.
- **oxlint.** A devDependency with a `lint` script that CI never ran. Both
  warnings it had to report were dead re-exports.
- **mypy**, strict by default with a named list of ten modules that are not
  there yet. Direction over starting point: lax-by-default means a new module
  is born unchecked and nobody notices. 24 of 42 modules passed `--strict` on
  the first run, including all of `mana.py`, `sim/tier1/` and `sim/tier3/`. It
  found three real latent problems — a `str | None` reaching `.lower()` on the
  companion path, an optional assigned to a non-optional in `get_cards`, and
  six `catch (e: any)` clauses in the frontend where `e.message` was unchecked
  property access.

The ladder from here, roughly in order of value per unit of effort:

### The repository is public, and `main` is protected

Decided 2026-08-10, after a commit landed on `main` directly because a merge
returned the working copy there and nobody noticed. CI ran and passed, so
nothing broke — but "nothing broke this time" is not a control.

Branch protection is unavailable on private repositories on the Free plan;
both classic protection and rulesets return `403 — Upgrade to GitHub Pro or
make this repository public`. So the choice was $4/month, a local git hook that
only guards one machine, or making the repository public. Public won on its own
merits: this document is explicitly written for the case where peers read the
repo, the history audit came back clean (no corpus, collection, credential or
`.env` file in any of 29 commits), and public repositories get unmetered
Actions minutes. The cost, stated plainly: the author's email address is in the
metadata of 19 commits and is now public.

The settings on `main`, recorded so they can be rebuilt:

| Setting | Value | Why |
| --- | --- | --- |
| Pull request required | yes, **0 approvals** | A solo maintainer cannot approve their own PR, so requiring 1 would deadlock the repo |
| Required checks | `test (3.11)`, `test (3.12)`, `frontend`, `no-secrets-or-corpus`, `image` | The whole pipeline. Renaming a CI job silently stops gating until this list is updated |
| Strict (branch up to date) | yes | Checks that passed against a stale base did not test what is being merged |
| Enforce for admins | **yes** | Off, it does not apply to the only contributor, which makes it decorative |
| Force pushes, deletions | blocked | |
| Linear history | required | Matches the squash-merge history already in use |

The escape hatch is turning admin enforcement off in settings — deliberately a
visible act rather than a `--no-verify` away.

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

## 6. Architecture Decision Records — written 2026-08-10

The most transferable rigor available here, and it cost almost nothing.

[`docs/adr/`](adr/README.md) holds ten, one per significant decision, each
recording context, the options considered, the decision, and the consequences.
They are immutable once accepted: a decision that changes gets a new ADR that
supersedes the old one, and the old one stays, because reasoning that turned out
to be wrong is usually the most useful thing in the directory.

Six were the seed list — deck.yaml in git, DuckDB for the corpus, Tier 1 stays
Python, two embedded databases, sessions over JWTs, and never redistributing
Scryfall bulk. Four more earned a place because they were already argued here
and a reader would otherwise have to reconstruct them: card facts come from the
corpus rather than memory (7), the gate blocks rather than routing around an
illegal deck (8), the built frontend bundle is committed (9), and correctness is
established against independent oracles (10).

Writing them found one thing worth having found: **three documents disagreed
about when the corpus gets refreshed.** This file said "at boot", ROADMAP said
"weekly by cron", and HOSTING said neither works — Fly volumes attach to exactly
one machine, so a scheduled second Machine cannot mount the corpus, and boot is
on the request path under scale-to-zero. ADR 6 records the resolution and both
stale lines are corrected. Forcing every decision into "options considered ·
decision · consequences" is what surfaced it.

A reviewer who reads ADR 3 and finds a profile, a table of numbers, and an
explicit "we did not port Tier 1, here is why" learns more about the engineer
than any amount of Rust would tell them.

---

## Suggested order

Decided 2026-08-10: do this list before any hosting work. Hosting is not
imminent, but see "Cloud-compatible by construction" above — steps 3 and 5
exist partly so that when it happens it is additive.

1. ~~**Property-based tests on `mana.py`** plus determinism tests.~~ **Done
   2026-08-10** — see §2. It found one real bug (Phyrexian mana value) and
   produced the enumerated corpus a port would be tested against.
2. ~~**ADRs for the decisions already made.**~~ **Done 2026-08-10** — ten of
   them in [`docs/adr/`](adr/README.md); see §6. Writing them caught a
   three-way contradiction about corpus refresh.
3. ~~**A `DeckSource` abstraction and a request scope.**~~ **Done 2026-08-10** —
   `decks/source.py` and `api/deps.py`. Every deck-facing route now takes the
   request scope, and the API is tested against an in-memory source.
4. ~~**Frontend tests**~~ **Done 2026-08-10** — 35 of them, on the job-polling
   state machine, the library filters and `identityName`; see §4. Found the
   library rendering "the gate never ran" identically to "the deck passed".
5. **The deck lifecycle** — import, the draft stage, and the rest of the edit
   operations. Added 2026-08-11, and it jumps ahead of container hardening
   deliberately: `docs/HOSTING.md` §6 assumes user decks can be created, and
   nothing can. Hardening a container that serves an empty library first would
   be polishing the wrong end. Planned in `ROADMAP.md`; decided in
   [ADR 12](adr/0012-decks-are-edited-by-surgical-operations.md) and
   [ADR 13](adr/0013-an-imported-deck-is-a-draft.md).
6. **Container hardening** — multi-stage, non-root, multi-arch, scanned,
   health-checked. Proves the deployment story without deploying.
7. **A simulator that plays decks against each other** — the Forge bridge
   first, Tier 2 in Python only if Forge cannot answer the question — then
   profile whichever got built.

Steps 1–6 make the existing project defensible without adding a language, and
leave hosting a matter of adding an auth layer and a second deck source rather
than reworking what is here.

Automated PR review (§5) is **deliberately parked** — priced out 2026-08-10.
Copilot Free does not include PR review; Copilot Pro is $10/mo; the Claude
action is pay-per-run and needs an API key. One input to that pricing has since
changed: the repository is public now, so Actions minutes are no longer metered
and the "it eats the free allowance" half of the argument is gone. The
per-review cost is not, so this stays parked — but re-price it, rather than
re-reading the old conclusion, when PR volume justifies a look.

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
**Newly true, worth not regressing:**

- **The one new thing that writes to the working directory is contained.**
  Hypothesis keeps a `.hypothesis/` cache; the CI profile in
  `tests/conftest.py` turns off the example database, but — measured, not
  assumed — a unicode and constants cache is still written regardless. The knob
  is `HYPOTHESIS_STORAGE_DIRECTORY`, and with it set nothing lands in the repo.
  That is one line in the container for §3's read-only root filesystem, and it
  is written down here so it is found before the build fails rather than after.
  The same profile derandomises, so a pull request never goes red because
  Hypothesis rolled different examples; deterministic coverage comes from the
  enumerated corpus instead.

**Worth doing now, because it is cheap now and invasive later:**

1. ~~**A deck source abstraction.**~~ **Built 2026-08-10** —
   `decks/source.py`. A `DeckSource` protocol with three methods (`slugs`,
   `get`, `all`), a `FileDeckSource` and a `MemoryDeckSource`. It is not
   scaffolding: hosting's two-tier model keeps the curated decks file-backed
   *permanently* and adds user decks from SQLite, so this is the shape the
   system ends up with, and `SqlDeckSource` is now additive rather than a
   change to every endpoint.

   `slugs()` is separate from `all()` on purpose — `/api/health` wants a count,
   and parsing every deck to produce it is silly now and worse later. It also
   means one unreadable deck file cannot take the health endpoint down.

   One constraint the protocol carries, and future implementations must
   respect: **a `DeckSource` is a locator, not a connection.** Background
   simulation jobs capture one and outlive their request by minutes, so a SQL
   source opens and closes per call rather than holding a handle.

2. ~~**A request scope, even with one implementation.**~~ **Built
   2026-08-10** — `api/deps.py`, one dependency, wired into every deck-facing
   route as a single `Decks` annotation. It returns the curated file-backed
   library today; when auth arrives it reads the session and returns a source
   that unions the curated decks with the caller's, and no handler changes.
   `docs/HOSTING.md` §1 requires all user-scoped queries to go through one
   accessor for isolation; this is that accessor, built before it has anything
   to isolate, which is the only time it is cheap.

   Deliberately *not* built: a `UserScope`. There is no session and no user
   table, and a one-field object modelling a user that does not exist is
   guessing at a shape rather than preparing for one.

   The payoff arrived immediately rather than at hosting time: `/api/decks`
   against an empty library is now a two-line test instead of a filesystem
   fixture, and there is a test that the endpoints read the scope rather than
   the filesystem at all.

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

Two items in the sections above stayed on the near-term list even though the
port is parked, because they pay off on the Python engine on their own terms:
**property-based testing of `mana.py`** (§2) and **determinism tests**. Both
shipped on 2026-08-10, and both earned their place in the branch where the port
never happens — one real bug found, and Tier 1's output now pinned. The corpus
and the pinned digests are also exactly what a port would be tested against, so
neither branch of the §1 decision wasted the work.
