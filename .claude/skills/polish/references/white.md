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

- `cd tools && animist verify` passes: every committed asset matches its
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

**The 95% floor did not survive the crossing, and saying so is this facet's
own medicine.** It was a real gate once; today CI runs `go test -race
-count=1 -cover ./...`, which prints a number and gates on nothing, and no
threshold lives in `ci.yml`, `.golangci.yml` or any doc. A rule enforced by
nothing had drifted, and only this file still asserted it. Measured
2026-08-23: **80.3%** of statements covered by the whole suite, **74.1%**
counting each package's own tests only. Until Aaron rules (it is in
`docs/polish/DAYBREAK.md`), treat coverage as a **watched number, not a
gate**: record both figures every run and treat a fall as a finding.

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
  effort spent for zero seconds, and the tree makes the point unusually
  starkly. Measured 2026-08-23 on this Mac, `go test -count=1 ./...`:

  | | |
  |---|---|
  | whole suite, wall clock | **1m13s** |
  | `internal/api` | **63.1s** |
  | `internal/claude` | 33.9s |
  | `internal/gate` | 13.2s |
  | everything else | under 11s each |

  Read that table twice. `internal/api` alone is **86% of the wall clock**, so
  the suite's time is that one package's time and nothing else is worth a
  minute of anyone's attention until it moves. The user column says 3m12s
  against 1m13s wall — the machine is already three-way busy, which is the
  package-level parallelism working and the reason within-package
  serialisation is the whole remaining cost.
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
- **`t.Parallel()`, from a standing start of zero.** Measured 2026-08-23:
  **831 test functions across 115 files and not one call.** Within a package
  every test waits its turn, and the slow packages here are one package each,
  so this is the lever that acts directly on the tail:
  - **The default is parallel; the exception is what needs arguing.** A test
    earns its serial place by touching real shared state — the process
    environment (`t.Setenv` makes a test un-parallelisable and the compiler
    enforces it), the working directory, a fixed port, a shared database
    handle written by more than one test, or a global the subject mutates.
    Everything reading a `t.TempDir`, a fresh in-memory database, or a
    `httptest.Server` of its own is parallel-safe by construction.
  - **Subtests need it twice.** `t.Parallel()` in the parent starts the
    package's other parallel tests; `t.Parallel()` inside each `t.Run` body is
    what makes the table's rows run together. A table with one and not the
    other is the common half-done case — and remember a parallel subtest's
    body runs *after* the parent function returns, so anything the parent
    deferred has already happened.
  - **Prove it, do not assume it.** `go test -race -count=2 ./internal/<pkg>/`
    on the packages touched: the race detector is the whole reason this is a
    safe fix rather than a queued one, and `-count=2` catches state left
    behind between runs. A conversion that cannot be proven green this way is
    a finding *about the test*, not a reason to skip the conversion.
  - Record in the ledger how many functions were converted, the package's wall
    time before and after, and — the part that makes the next run cheaper —
    **which tests were examined and left serial, with the reason.**
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
- Skips are a budget, not a convenience: every `t.Skip` in the tree is
  conditional on a real absence (a live instance, a Forge install, a full
  pool), and a drift in the skip census is a finding even when CI is green.
- No test sends mail, spends a token, or touches the network — confirm the
  seams (the mail sender, faked Claude turns, faked subprocesses) still
  hold for anything added since last run.
- Verify new guard tests by mutation, not by greenness: a test written to
  hold a boundary gets the boundary broken locally once to prove it fires.
  The standing example is a whole class of code that is maintainer-dependent
  — it takes a different path for an admin than for anyone else — and whose
  admin path the default fixtures never take, so it is untested and *looks*
  tested.
- **Mutation sampling is suspended, not retired as a practice** (Aaron's
  standing ask, 2026-08-16): the harness died with the old backend and its
  Go rebuild is the ledger's open item. Until it lands, this facet's sampling
  is the hand protocol on a *throwaway copy* of a package — never the working
  tree — and the survivors the old ledger recorded stay listed there so the
  rebuilt tool can re-ask them by name on its first run.

  **The named candidate is `go-gremlins/gremlins`** — a standalone binary with
  mutation-score thresholds, so it installs on demand exactly as
  `go-licenses` does and costs the project no `go.mod` entry. (The livelier
  alternative, `gtramontina/ooze`, runs inside `go test` and therefore *is* a
  dependency, which is a bigger ask here; `zimmski/go-mutesting`, the classic,
  has not moved since 2024.) Adoption is queued in `DAYBREAK.md`, not a
  decision a run takes. When it lands, run it **one package at a time and
  start with the determinism kernels** — `floats`, `mt19937`, `textutil`,
  `yamlemit`, `gate` — where correctness risk concentrates and packages are
  small. Never point it at `internal/api`, which is 63 seconds per test run
  before a single mutant is generated.
- After the suite, **`git status data/` proves nothing** — `app.db` is
  gitignored, so a test that writes the developer's real database leaves
  the status clean. Use `ls -la data/` and treat a fresh mtime on
  `data/app.db` as the finding; a test reaching past its scratch directory
  gets fixed, never accommodated.
