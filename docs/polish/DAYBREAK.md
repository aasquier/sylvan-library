# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one line with
a recommendation, so the whole file can be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed.

Each item: what it is · what it costs to leave it · **the recommendation.**

---

## Open — 2026-08-24

**1. ADR 5's isolation sweep did not cross to Go, and it is the one test that
ADR itself calls the highest-value test in the whole auth story.** Its decision
text reads: *"For every user-scoped endpoint, a test logs in as user B,
requests user A's resource and asserts 404, not 403 … **parametrised over the
route table, so an endpoint added without scoping fails the suite.**"* Python
had it — 57 routes classified, each filed with its argument. The Go tree has no
equivalent: the door's sweeps derive public-vs-protected and the admin prefix
from the route table (both genuinely machine-checked), but *whose data a route
serves* is checked only by a scattering of hand-written per-route 404 tests
with no completeness guard over the 23 `/api/decks/{owner}/…` patterns. The
structural half ADR 5 also asked for **is** in place — one accessor,
`api.sourceFor` → `library.SourceFor`, and every deck handler read this run
goes through it — so this is a missing guard rather than a known hole, and no
leak was found by spot-check. · *Cost of leaving it:* the next deck route
written without the accessor ships silently; nothing in the suite notices, and
the thing it would leak is another person's decklist. · **Recommendation:**
rebuild it, and in ADR 5's own parametrised form rather than as more one-off
tests — iterate the served route table, and for every pattern carrying
`{owner}` drive it as a signed-in stranger against a private deck and assert
404. Deliberately **not** attempted tonight: mis-filing one route as shared
would certify a hole as shut, which is worse than the gap, and that judgement
wants daylight. Ledger: White, 2026-08-24.

**2. `NOTICE.md`'s dead anchors are fixed, but nothing stops the next four.**
This run repaired three dead repo paths and a dead command name in the
licensing record — anchors the Go crossing killed (see White's ledger entry) —
and added `go/cmd/mtglab/licenserecord_test.go` to hold them: every
repository path `NOTICE.md` and the `PROVENANCE.md` files name must exist, and
every `mtglab <verb>` they tell a reader to run must be a real subcommand,
read off the binary's own command tree. It does **not** cover the rest of the
tree's prose, where the same rot is visible — `docs/`, `web/README.md`, package
comments, `.claude/skills/yas-queen/references/house-codes.md`. · *Cost of
leaving it:* nothing legal; the licensing record itself is now held. This is
the wider docs-rot question, and it is the same shape as daybreak item 5 from
2026-08-23. · **Recommendation:** if you say yes to that item, have its
implementation reuse this test's two helpers rather than write a second pair —
both halves are built, tested and mutation-verified.

## Open — 2026-08-23

**1. The 95% coverage floor is gone, and the drop is half unit-change and half
real.** CI's `-cover` prints a number and gates on nothing; no threshold
exists in `ci.yml`, `.golangci.yml` or any doc, and only the polish skill
still asserted it. Measured 2026-08-23: **80.3%** of statements across the
whole suite (`-coverpkg=./...`, the figure comparable to the old floor).

The old gate covered `src/mtglab` at 95.749% on **11,573 Python statements**.
The same product in Go is **16,050 statements** — 39% more for identical
behaviour, because Go writes its error paths as code where Python's exceptions
propagate invisibly. Classifying all 3,158 uncovered statements by reading
their source: **33% are error handling** (`if err != nil` and its returns),
lines that largely did not exist to be counted before. Excluding error paths
from the denominator puts it at **86.0%**.

The other **54% (1,714 statements) is ordinary logic, and that part is a real
regression** — and it is not diffuse. `pyproject.toml`'s own coverage note
records what bought the last five points in Python on 2026-08-14: the CLI's
Claude renderers, the theme modes faked at `Turn`, **Forge's run path faked at
`subprocess`**, **the Scryfall ingest against fake bulk files**, and the SQL
deck tier. Those are precisely today's gaps — `sim/tier3` sits at **25.6%**
(`run.go` 169 uncovered, `worker.go` 110), `pool/refresh.go` 111,
`cmd/mtglab/users.go` 126, plus the admin/sim/shelf route families.
**The fakes were not rebuilt in Go.** The migration's safety net was
byte-identical output diffing across a case matrix — stronger than coverage
for proving the port, but it leaves no unit tests behind for the paths it
exercised. · *Cost of leaving it:* Forge's run path and the pool refresh are
both live code with no test holding them.

· **Recommendation, two parts:** (a) make coverage a *watched* number, not a
gate — record it every White run, treat a fall as a finding — since a
threshold set today either fails CI at once or certifies nothing; and (b)
queue the three fakes for rebuild in Go, in this order: the shim/subprocess
fake for `tier3` (biggest single gap, and the only one covering code that
talks to a worker machine), the bulk-file fake for `pool/refresh`, then the
CLI's pool-backed commands. That is the honest path back, and it is worth
more than any number.

**2. ADR 38 cites `docs/go-migration/`, twice, and the directory is gone.**
The zero-trace sweep deleted it; the ADR links it in its header and its
context. ADRs are immutable. · *Cost of leaving it:* an accepted record points
at nothing, and every future reader of ADR 38 hits it. · **Recommendation:** a
short superseding note recording that the directory was deliberately removed
and where its content went — cheaper than restoring it, and honest about why.

**3. The `deploy` job's `needs` list is checked by nothing.** The test that
derived the expected job set from `ci.yml`'s own job list died with the old
suite. A job added without `needs` now deploys off a partial suite, silently.
· *Cost of leaving it:* one forgotten line ships an unverified deploy. ·
**Recommendation:** rebuild it in Go as a test that parses `ci.yml` and
derives the set — it was a real guard and it is a small one.

**4. Does night work merge itself?** The skill's current rule, written
2026-08-23 and derived from commandments 14 and 16 rather than from a ruling:
anything a user can see stops at a green PR for Aaron's eye; everything else
may merge when the required checks are green. · *Cost of leaving it:* nothing
— it is already the conservative reading. · **Recommendation:** confirm it, or
tighten it to "a night run never merges" if a 3am deploy is unwelcome at all.

**5. The polish skill is entirely unenforced prose, which is the one thing it
tells every run to hunt.** Its own standing question is "which absolute claim
is enforced by nothing?" — and the answer, for the skill itself, is *all of
them*. Today's refresh fixed by hand: a command that does not exist
(`mtglab animist verify` — animist is a `tools/` script), a path that moved
(`src/mtglab/web_dist`), a required-checks list wrong for months, a test cited
as the model of good practice that no longer exists, and a claim that CodeQL
gates merging. Every one is mechanically checkable. · *Cost of leaving it:*
the skill rots at exactly the rate the tree moves, and only a colorless run
notices, once a cycle. · **Recommendation:** a small Go test that reads
`.claude/skills/**/*.md`, extracts the repo paths and `mtglab`/`animist`
subcommands they name, and asserts each resolves — paths against the tree,
subcommands against the CLI's own command table. Perhaps sixty lines, it
derives its expectation from the source of truth rather than restating it (the
pass's own rule), and it would have caught four of the five above at commit
time. A colorless run's natural first job.

**6. A pprof mount, so the hot-spot patrol can profile the serving process
itself.** Red's new patrol profiles at the package seam because the door has
no profiling endpoint at all — the tree contains no pprof anywhere. Two
halves, separable: (a) **dev-local**, mounting `net/http/pprof` only when
auth is off, which is a laptop-only surface and lets a patrol profile
`mtglab ui` under real request-shaped load; (b) **live**, the same mount
admin-gated behind ADR 17's 403-by-prefix, which is what would catch a hot
spot that only exists against the real pool and the real library. · *Cost of
leaving it:* the patrol reads test-shaped load and outside clocks, which is
honest but blind to request-shaped hot spots. · **Recommendation:** (a) yes —
small, laptop-only, no deployed surface; (b) is genuinely useful but is a
door change with a real caveat: **heap profiles carry process memory**, and
this process's memory holds session tokens and Argon2id parameters, so live
would mean CPU-profile-only and admin-gated, and commandment 10 keeps it
invisible to users either way. (b) is your call, and it can wait for a hot
spot the local mount cannot explain.

---

## Answered

- **Mutation testing: adopt `gremlins`** — yes, 2026-08-23. Installed on
  demand (`go install github.com/go-gremlins/gremlins/cmd/gremlins@latest`),
  no `go.mod` entry, one package at a time, determinism kernels first, never
  `internal/api`. The protocol now lives in the skill (White's testing facet);
  `docs/ENGINEERING.md` names it as the project's mutation tool.

*(Answered items move here in one line with the ruling and the date, then out
entirely once the ledger carries them.)*
