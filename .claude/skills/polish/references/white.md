# White — Law & Protection

Three facets: free-use and licensing compliance (triple-checked), security and
user isolation, and testing discipline. White is the color of rules held for
the good of everyone at the table — the licence that lets this project exist,
the isolation that lets a friend trust it with their cards, and the suite that
lets every other run move fast.

## Facet: free-use & licensing (triple-check rigor)

The stakes, plainly: this project exists because Wizards of the Coast's Fan
Content Policy permits free fan projects. One violation — one committed piece
of their art, one monetized corner — and the standing to run the site at all
is gone. Commandment 9 makes this a hard boundary; Aaron has named this facet
the one to triple-check.

Triple-check means: for every asset and dependency you examine, (1) find the
claim of compliance, (2) verify it against the primary source — the licence
text, the recipe, the PROVENANCE entry — not a summary, and (3) confirm the
enforcement mechanism that keeps it true is still in place and still has no
override. Uncertain after that? Queue it for Aaron and treat it as
non-compliant until he rules.

Work the list:

- `cd tools && .venv/bin/animist verify` passes: every committed asset matches its
  recipe (ADR 29) — the toolbox owns this command; `mtglab` has no `animist`
  subcommand and never did. Then sweep for binaries that *bypassed* the
  pipeline: compare
  `git ls-files` image/font/media files against what the recipes and
  `PROVENANCE.md` files account for. A hand-placed binary is a finding even
  if its licence turns out fine — the pipeline exists so nobody has to trust
  a memory.
- The licence gate (`animist licence`, from `tools/`) still has no `--force` and no
  code path around it. Check the code, not the docs.
- Wizards' art is runtime-only, always: `PERSONA_ART` hotlinks with credit
  and nothing under `git ls-files` is a Wizards image. The tarot art is the
  1909 Rider printing — the 1971 recolouring is still in copyright, so any
  new tarot-adjacent asset needs its edition argued per file.
- ADR 6: no Scryfall bulk data, price data, or any redistribution of their
  files in the repo, the image, or an artifact. Scryfall attribution appears
  where their data renders.
- No monetization surface exists, even vestigially: no payment code, no
  donation links, no ad slots, nothing that takes a penny. Check the frontend
  too — a well-meaning "buy me a coffee" is a violation here.
- Dependency licences: sweep the Go module graph (`go-licenses report ./...`
  from `go/`, installed on demand), the toolbox's own metadata in
  `tools/pyproject.toml`, and `npm --prefix web ls` trees for licences
  incompatible with a free
  public project (AGPL in a dependency is a finding to queue, not necessarily fatal
  — Aaron rules). Record the sweep date in the ledger.
- Fonts, CSS, and anything served: each has a named free licence. If the
  provenance argument lives nowhere, that is the finding.

## Facet: security & user isolation

The design intent: isolation is the first thought. Anyone keeping cards
private on this site must actually have them private — from other users and
from accidents, not just from attackers.

- Every route is classified by the door's own sweep tests (derived from the served route table), and the sweep is
  live: try adding a fake unclassified route locally and confirm the suite
  fails (then remove it). The middleware refuses before routing — verify any
  new prefix landed in the right list.
- The 403/404 law: another person's things are **404** (ADR 5); an admin
  route to a non-admin is **403** (ADR 17, argued); deck writes are
  owner-only (ADR 22, #80). Check any route added since the last run against
  all three.
- Email addresses: `auth.User.AsDict` takes `includeEmail` and omits the
  address unless asked; exactly two callers may ask (`mtglab users list`, the
  admin routes). Grep for every call site each run — a third one is the
  finding — and for new log lines or tool results that could carry an address.
- Session hygiene: cookie flags (HttpOnly, Secure, SameSite), Argon2id
  parameters against current OWASP guidance, rate limiting on login/reset
  still answering 429 with Retry-After, reset responses still uniform for
  existing and non-existing addresses.
- Tokens: invite/reset links are single-use, hashed at rest, and arrive in
  the URL fragment — never the query string, which would log a live
  credential. Confirm no new surface reintroduced a query-string token.
- Secrets: CI's filename and content scans still cover the tracked tree
  including `web_dist/`; `.env` gitignored; `fly.toml` carries no secrets;
  the Anthropic key reaches code only via environment.
- Supply chain and static analysis: read the latest CodeQL and
  dependency-review results rather than assuming green means examined; note
  anything dismissed and why.
- SQL: everything through parameterized queries behind `internal/auth` and
  `internal/pool`; string-built SQL anywhere is a finding.

### Fixing a security finding — the hard-won protocol

A real fix landed here (the SPA catch-all path traversal, PR #126) and cost
four commits to get green. The lessons are worth more than the fix:

- **Two jobs, not one: close the hole *and* satisfy the scanner.** The bug is
  fixed when the vulnerability is gone; the work is *done* when CodeQL is
  also green. Those are different — and neither is the merge gate, because
  CodeQL is advisory here (Red's facet argues why), so nothing stops a red
  scan shipping except you. They are different because CodeQL's model may not
  recognise a perfectly correct guard. Two containment checks on the resolved
  paths *contained* the traversal correctly — the test proved it — and CodeQL
  flagged both anyway, because it does not model either as a barrier on this
  query. Expect the same of `filepath.Clean` plus a prefix check: correct, and
  not necessarily legible to the scanner.
- **When a guard isn't recognised, break the taint provenance instead of
  hunting for a guard form the scanner likes.** Do not build the sensitive
  value out of user input at all. The traversal fix stopped joining the
  request path onto the static root and made the request path a pure map key,
  so the served file comes from a trusted directory listing and no user input
  reaches the filesystem call — nothing for the taint tracker to follow. This
  is both safer *and* legible to the scanner, and it is the move to reach for
  first, not fourth.
- **Mutation-verify every security test.** Revert the guard, watch the test
  fail, restore it. A security test that passes against the *broken* code is
  worse than none — it certifies a hole as shut.
- **Verify on the live instance after deploy.** A merged fix auto-deploys
  (ADR 23); drive the real surface to confirm the hole is actually closed in
  production, because the whole class of deployment-only bugs lives in the gap
  between the local tree and the running instance.
- Each CI round on a fix like this is a ~5-minute image build. Diagnose from
  the *actual* alert (`gh api .../code-scanning/alerts`) — which sink, which
  line, new-on-this-PR vs pre-existing-on-main — rather than guessing and
  re-pushing; guessing is what made this four commits instead of two.

## Facet: testing discipline

Aaron's bar is the *right* tests, not coverage tests — and a suite that stays
fast enough that adding tests never feels expensive.

**The coverage floor is a gate now, and this paragraph is the record of how a
claim becomes one.** For a month this file opened on *"the 95% floor is a
claim no gate enforces"* — true when it was written, and the pass's own
favourite example of a rule enforced by nothing. It is answered: `ci.yml`'s
**`Coverage floor`** step runs `go test -count=1 -coverprofile
-coverpkg=./...` on the arm64 leg and fails the build under `MINIMUM`, which
stands at **95.0**. Two guards hold the shape of that number rather than the
number itself, and they are what you check rather than re-deriving the
history: `go/cmd/mtglab/coveragefloor_test.go`
(`TestEveryGoFileCompilesOnBothCILegs` — one leg may compute the total only
while every Go file compiles on both, so an arch-tagged file fails by name
instead of quietly narrowing the measurement), and `docs/polish/COVERAGE.md`,
which is the per-function map and the list of levers.

So coverage is **a gate with a watched margin**, and the two jobs are
different. The gate is CI's. The watched number is this facet's: read the
total with `go tool cover -func` over a `-coverpkg=./...` profile — the same
arithmetic the step uses, which is *not* what a hand merge of the same
profile reads — record it in the ledger every run, and treat a fall as a
finding even when it clears 95.0. **The floor is a ratchet: raise it when the
tree passes a higher number, never lower it to make a red check green.** The
size of the margin between the tree and the floor is Aaron's ruling of
2026-09-24 rather than a number to optimise, so a run that wants to click the
floor up reads `ci.yml`'s own comment first.

Two traps in the measuring itself, one of which caught this run:

- **`-coverpkg=./...` changes what every per-package line means.** With it,
  each package reports its coverage *of the whole module* — so a determinism
  kernel with excellent tests prints `0.4%` and reads like a hole. Use the
  plain `-cover` run to rank packages and the `-coverpkg` run only for the
  module total. Reading one number in the other's frame produces a confident,
  completely wrong finding.
- **Read the report for *meaningless* coverage too.** A package at 100%
  through tests that assert nothing is worse than an honest gap, because it
  reads as done. This is why the mutation work below outranks the percentage.

- **Check the environment before believing the run.** Compare the local
  package and test counts against CI's — a passing suite that ran *less than
  CI ran* is the failure mode this facet exists for, and it reads exactly
  like success. A green local suite is evidence only once you know it is the
  same suite. On this Mac that means the three exports (toolchain PATH and
  GOROOT, the CGO ldflag) are set, because without CGO neither `internal/pool`
  nor anything above it typechecks and the linter silently covers less; and it
  means remembering CI runs the suite on **two architectures** and this laptop
  is one of them.
- **Once per cycle, follow the documented setup from a clean checkout.**
  `git worktree add` to a scratch path, follow CLAUDE.md's Setup block
  *verbatim* — nothing nobody wrote down — and compare the test count with
  CI's. The documented instructions and the working environment drift apart
  indefinitely unless someone deliberately stands where a new contributor
  stands. Two known snags to expect rather than rediscover: a fresh worktree
  has no card pool and no `web/node_modules`, and a borrowed toolbox venv
  runs the *other* tree's sources against this tree's tests.
- Measure first: `go test -count=1 ./... 2>&1 | tail` for wall time, and
  `go test -json` piped through a duration sort for the slow tail. Record
  both in the ledger. A test that got slower has a reason; find it.
### Keeping the suite fast — the standing sweep

A slow suite is not a cosmetic problem: it is the thing that makes adding a
test feel expensive, and Aaron's bar is the *right* tests, which is a bar you
only clear when writing one is cheap. Go is unusually good at this, and the
tree is using almost none of it.

**Measure before touching anything, and record it.** `go test -count=1 ./...`
for the wall clock; `go test -json` sorted by elapsed for the per-package
tail. Two whole-suite facts to hold on to before optimising a single test:

- **Go already runs different packages in parallel.** So the suite's wall time
  is roughly its *slowest package*, not its total — which means the only work
  that shortens the run is work on the tail. Optimising a fast package is
  effort spent for zero seconds.

  **Take the table yourself; do not read one from this file.** A frozen table
  lived here for a month (1m13s wall, `internal/api` 63.1s = 86% of it) and
  every figure in it had rotted: the tree is thousands of tests bigger, every
  one of them parallel, and the wall clock on this Mac now moves by a factor of
  five with the number of sessions running beside you. A suite time quoted
  without the load beside it is a measurement of the laptop's mood. So:

  ```bash
  uptime                                   # before, and again after
  go test -count=1 ./... 2>&1 | tail       # the wall clock
  go test -json ./... | <sort by elapsed>  # the per-package tail
  ```

  The numbers go in the ledger, dated, with the load — that is what makes them
  comparable to the next run's. The *shape* is the lasting fact and the reason
  to take the table at all: find the one or two packages that are the wall
  clock, and spend nothing on the rest. For a number that is genuinely
  comparable across weeks, read CI's per-job medians instead of this laptop's
  wall clock (Red's facet records them every run, n≈40 runs a window) — the
  runners are the same machine every time, which is the whole reason that trend
  line is honest and this one is not.
- **`-count=1` deliberately defeats the test cache**, and CI passes it. That
  is correct for a gate and wrong for a working loop: leaving it off locally
  lets an untouched package answer instantly, so use it when you need the
  truth and drop it while iterating.
- **`-race` roughly halves throughput**, and it is worth every second — it is
  what makes a parallelism sweep a safe fix rather than a gamble. Never quote
  a race-detected time as the suite's time, or the trend line lies.

Then the levers, in the order that pays:

- **The expensive fixture, built once.** The card pool is the standing example
  — a package that opens one per test is paying for it every time, and
  `TestMain` plus a package-level handle (or `sync.OnceValue`) pays once.
  Look for the same shape in database migrations and any golden that is parsed
  per case rather than per package.
- **`t.Parallel()` is spent as a lever and is a gate instead.** This bullet
  used to open on *"from a standing start of zero — 831 test functions across
  115 files and not one call"*, and both halves are gone: as of **2026-09-24**
  every top-level test under `go/` calls it, zero are serial, and
  `go/cmd/mtglab/serialregister_test.go` fails **by name** on any test whose
  own body does not (a call inside a `t.Run` closure does not count for the
  parent; `TestMain` and benchmarks are not tests). It also logs the total, so
  the count is read off the register rather than written down anywhere —
  including here. The register used to be a list of argued exceptions; the
  thirty-nine it held on 09-19 turned out to be **ten pieces of shared state**,
  each a fact about the code rather than the tests, and CLAUDE.md's Testing
  section carries the three shapes they became.

  What is left for this facet is therefore not conversion but the two things
  that keep it true:
  - **A new test Go refuses is a finding about the code.** `t.Setenv` reached
    through *any* helper panics with "can not use t.Parallel"; `-race` reports
    a package-level write, but only when two tests happen to overlap, so read
    for those as well. Either answer names a piece of shared state, and the fix
    is always the same: make it a value and hand it in — a reader of the
    process becomes a `func(string) string`, a package-level variable becomes a
    field, a signal becomes a channel. **Never add a serial exception**; there
    is no register to add it to any more.
  - **Subtests that share a fixture use `t.Cleanup`, never `defer`.** A parallel
    subtest's body runs *after* the parent function returns, so a parent's
    `defer` has already fired by the time the subtest touches the thing.
  - **Prove it with `go test -race -count=2 ./internal/<pkg>/`.** `-count=2`
    catches state left behind between runs, and the race detector is the whole
    reason this class of change is a safe fix rather than a queued one.
- **Sleeps are the other half of the tail.** Every `time.Sleep` in a test is
  wall time bought to avoid thinking about synchronisation, and it is both slow
  *and* flaky — too short and it fails on a loaded runner, too long and
  everyone pays. Replace with the thing actually being waited for: a channel, a
  `sync.WaitGroup`, `httptest`'s own synchrony, or `testing/synctest` for code
  that genuinely reasons about time, which gives a fake clock and makes the
  wait free. A test that got a *longer* sleep to fix a flake is a finding.
- **Split the subject, not the suite.** A test that needs the network, a real
  pool, or a Forge install is a different animal from a unit test; the tree
  already gates those on a real absence. Keep that honest rather than reaching
  for `testing.Short()`, which mostly teaches people to run a subset and call
  it the suite.
- **A table beats twenty functions** for both speed and reading: one setup,
  many cases, each a `t.Run` that can be parallel and named well enough to
  fail informatively.
- **Do not chase a fast suite into a weak one.** Every second saved by
  deleting coverage is a second charged to a future bug. The trade is only
  ever *the same assertions, less waiting* — and if a conversion makes a test
  harder to read, it was not worth it. Record the wall time each run so the
  trend is visible; a suite that got slower has a cause worth naming.
- Hunt duplicated setup: fixtures and helpers belong in the shared test
  helpers (`internal/pool/pooltest`, `internal/auth`'s authtest fixtures) —
  three tests hand-rolling the same scaffolding is a finding.
- **The determinism replay, once per cycle, against the live instance.**
  Determinism is contract (CLAUDE.md): a seed is a promise — the tarot deal a
  browser reloads, the Wheel's spin, every Tier 1 run. The goldens hold it
  locally; this replays it where users live. Keep one recorded seed per
  surface in the ledger with its full response, and each cycle ask the
  deployed instance the same seed and byte-compare: the same tarot seed must
  deal the same spread, the same wheel seed the same fate. One nuance for
  Tier 1: its cache key includes the engine fingerprint, so after an engine
  change a recompute under the same seed is *correct* — check the fingerprint
  before calling a Tier 1 difference drift. Tarot and Wheel have no such out;
  drift there is a broken promise and outranks everything else in this facet.
- Skips are a budget, not a convenience: every `t.Skip` in the tree is
  conditional on a real absence (a live instance, a Forge install, a full
  pool), and a drift in the skip census is a finding even when CI is green.
- No test sends mail, spends a token, or touches the network — confirm the
  seams (the mail sender, faked Claude turns, faked subprocesses) still
  hold for anything added since last run.
- **A test that asserts something about work *in flight* must hold the work in
  flight.** Otherwise it races itself and its greenness is a fact about the
  machine, not the code. The standing example: the Forge in-flight dedupe test
  posted two identical asks and expected one job — with a stub that finished
  instantly, so on a quick enough machine the first job was already done and a
  second job was the *correct* answer. It passed on this laptop and went red
  on CI's arm64 runner. The fix is a gate the test controls (this stub already
  had one), never a sleep. **Diagnose this class by making the race certain**
  — sleep between the two actions and watch it fail every time — then fix it
  and confirm the fix survives that same sleep. Suspect every test whose
  subject is a cache, a dedupe, a job, a lock or a stream.
- Verify new guard tests by mutation, not by greenness: a test written to
  hold a boundary gets the boundary broken locally once to prove it fires.
  The standing example is a whole class of code that is maintainer-dependent
  — it takes a different path for an admin than for anyone else — and whose
  admin path the default fixtures never take, so it is untested and *looks*
  tested.
- **Mutation sampling is a live practice again, and `gremlins` is the tool**
  (Aaron's standing ask since 2026-08-16, ruled 2026-08-23). It is a
  standalone binary with mutation-score thresholds, so it installs on demand
  exactly as `go-licenses` does and costs the project no `go.mod` entry:

  ```bash
  go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
  gremlins unleash ./internal/floats/          # `run` and `r` are aliases
  ```

  Read the report by status: **KILLED** is a test doing its job, **LIVED** is
  the finding (a mutant the suite never noticed), **NOT COVERED** is a line no
  test reaches at all, and **NOT VIABLE** means the mutation did not build and
  says nothing about the tests. `--threshold-efficacy` fails the run below a
  KILLED/(KILLED+LIVED) ratio, which is the flag to reach for once a package
  has a number worth holding — **not before**, because a threshold invented
  ahead of a baseline either fails at once or certifies nothing.

  How to run it here, learned from the packages rather than from the docs:
  **one package at a time, starting with the determinism kernels** —
  `floats`, `mt19937`, `textutil`, `yamlemit`, `gate` — where correctness risk
  concentrates and packages are small. **Never point it at `internal/api`**,
  which is 63 seconds per test run before a single mutant is generated. Record
  the score per package in the ledger the first time each is run; a mutation
  score with nothing to compare against is a number, not a finding.

  Two things it does not replace. **A survivor is a question, not a task** —
  a LIVED mutant in a branch the product never takes is noise, and saying so
  in the ledger is the right answer. And the **hand protocol still stands** for
  anything gremlins cannot reach (the frontend, `tools/`, a single guard test
  you just wrote): break the thing on a *throwaway copy* of the package —
  never the working tree — watch the test fail, restore it.
- After the suite, **`git status data/` proves nothing** — `app.db` is
  gitignored, so a test that writes the developer's real database leaves
  the status clean. Use `ls -la data/` and treat a fresh mtime on
  `data/app.db` as the finding; a test reaching past its scratch directory
  gets fixed, never accommodated.
