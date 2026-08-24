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
with no completeness guard over the 22 `/api/decks/{owner}/…` patterns. The
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

**3. Five code-scanning alerts are permanently red and can never close by
themselves.** All five are Python findings on files the Go crossing deleted,
and the Python analysis never runs again — so nothing will ever mark them
fixed, and the list will read "5 open" forever. The cost is not the findings
(the code is gone); it is that a permanently-red alert list teaches everyone
to stop looking at code scanning, which is the one place a real finding would
appear. The six *Go* alerts are already dismissed, each with a sound argument.
· *Cost of leaving it:* nothing today, and a genuine alert missed on the day
there is one. · **Recommendation:** dismiss all five as "no longer relevant"
— it is your account and your call, which is the only reason this is a
question rather than a fix. Ledger: Blue, 2026-08-24.

**4. A one-word correction to the artists shelf is built, green, and waiting
for your eye.** The lore shelf tells a reader that Library of Alexandria is
"Alpha's own". It is not: its first printing is Arabian Nights, 1993-12-17,
and Alpha is the 295 cards of 1993-08-05 (read out of the Scryfall bulk, not
recalled — Mark Poole did paint it, so only the set is wrong). It renders, so
it stopped at a green PR rather than merging. · *Cost of leaving it:* the
shelf teaches a newcomer something false about the game's own history, to the
one audience most likely to notice. · **Recommendation:** merge it. **The walk
is cheaper than it looks, and here is the honest version:** the diff is one
word in checked-in prose and touches no render path, so the two-second check
is `curl -s localhost:8765/api/lore | grep -o "Mark Poole[^\"]*"`. If you want
it on the page: `.claude/launch.json`'s `mtglab-ui` entry (or `go/mtglab ui
--port 8765`), then http://127.0.0.1:8765/ — the shelf sits under the Library
masthead and shows **one fact at a time from a random opening offset**, so
finding this one means clicking **Another** up to 42 times. Nothing animates
and there is no cycle time to wait out. Ledger: Blue, 2026-08-24.

**Black —**

**1. PR #284 is green and unmerged, and nothing in it renders.** Three fixes,
all backend: `/api/health` answers in **2.0ms instead of 4.4ms** (it was
asking eight database statements to answer a question that needs two — the
platform's own health check is one of its callers); the card-search page's
**opening query drops 87.7ms to 53.1ms** and a type filter 71.6ms to 52.9ms,
because the search was buying a price for every row the top-sixty was about to
throw away; and the standing claim *"the served app hotlinks nothing it could
serve itself"* is now machine-checked over the built bundle instead of
re-verified by hand every quarter. The JSON on the wire is byte-for-byte what
it was — same cards, same order, same prices — so there is nothing to walk. ·
*Cost of leaving it:* the wins sit on a branch, and the hotlink guard is not
protecting anything until it runs in CI. · **Recommendation:** merge it; it
only stopped because tonight's harness refused `gh pr merge`. Ledger: Black,
2026-08-24.

**2. The Claude spend ledger lost its command line in the crossing, so the
deployed instance's bill can only be read by signing in.** The accounting
itself is fine — every conversation is recorded, and the Admin panel rolls it
up — but Python's `mtglab claude usage` did not come across, and Claude never
signs in to anything. The practical result is that the polish pass can only
ever report the *laptop's* spend, which is nearly idle (≈$1.71 since
2026-08-16, three conversations in the last five days), while the instance is
where the money actually goes. · *Cost of leaving it:* every future spend
number in the ledger is the wrong machine's, and the one place a runaway cost
would show up is a page nobody opens. Worth noticing now rather than later:
the Sonnet 5 introductory rate ends **2026-08-31**, and the same traffic costs
50% more from 1 September (the code already models the change; nothing needs
editing). · **Recommendation:** yes — restore `mtglab claude usage`, so
`fly ssh console -C "mtglab claude usage"` answers it from anywhere. It is
about eighty lines over machinery that already exists; the only real questions
are what it prints by default (per mode, per model, or both) and whether it
shows dollars beside the tokens. Say which and it can be built in one sitting.
Ledger: Black, 2026-08-24.

**3. A price limit on the card search filters the results *after* the search
has already picked sixty, so a budget shows a fraction of what matches.**
Measured tonight: asking for sixty cards under $1 returns **23**; asking for
two hundred returns **74**. The page always asks for sixty, so a newcomer who
sets a budget sees a short list and reasonably concludes that is all there is.
Nothing is wrong with the prices themselves — it is the order the two steps
run in. · *Cost of leaving it:* the one filter a beginner is most likely to
reach for is the one that quietly lies about how much of the game they can
afford, which is commandment 2 failing in the exact place it matters. ·
**Recommendation:** yes — make the price a condition of the search rather than
a filter over its answer, so "sixty cards under $1" means sixty cards under
$1. It is a small change and it is a *behaviour* change (different cards
appear), which is the only reason it is a question rather than a fix. Ledger:
Black, 2026-08-24.

**4. The card pool's memory is thrown away every time nobody has asked it
anything for ten seconds, and the lore shelf pays 84× for that.** Measured
tonight on the full pool: the shelf answers in **0.9ms** while the pool is
held open and **75.6ms** on the first request after a ten-second gap, because
the remembered card lookups are filed under the *open* rather than under the
pool file — and the pool is handed back on a timer so that `mtglab data
refresh` can always get in, which is a rule worth keeping and must not be
touched. A reap and a refresh are not the same event, though: only a refresh
makes the old answers wrong. · *Cost of leaving it:* on a quiet instance
almost every visitor is the first one, so almost everybody pays the slow
number and nobody sees the fast one; the front page's own shelf is the worst
case. · **Recommendation:** yes — let the remembered lookups survive a
hand-back as long as the pool file has not changed underneath them, which is
the rule the code already says it follows. It is a change to how long
something is remembered rather than to what is remembered, which is the only
reason it waited for daylight. Ledger: Black, 2026-08-24.

**5. Five of the seven `Loading…` labels are flat text with no motion, and one
of them is the door into the fortune-teller's table.** Blue found them by
driving the real site (their ledger has the list); this run timed what each
one is waiting for, because "it feels slow" and "it *is* slow" want different
answers. **It is not slow.** Every one of the five waits on a single request
that the server answers in well under a millisecond — so there is nothing to
speed up, and the whole of it is the page sitting still while it waits. ·
*Cost of leaving it:* the site's promise is that it is alive and moving, and
the five places it visibly is not include the way in to the reading — the one
room that is meant to get the best of everything. · **Recommendation:** yes —
give them the same treatment the two good ones already have (`App.tsx` uses a
proper spinner in both of its waits), or a held frame so nothing jumps when
the answer lands. It renders, so it wants your eye before it merges. Ledger:
Blue and Black, 2026-08-24.

**Colorless —**

**1. Four of the six questions asked yesterday morning exist only in this
file.** The pass's own rule is that a waiting item lives in two places — one
line here, the full record in `LEDGER.md` — and yesterday's run wrote six lines
and no records. Two of the six made it into the ledger as one-liners in a
backlog list; the coverage floor, the night-merge rule, the skill-prose guard
and the pprof mount have nothing anywhere else. That is worse than being
invisible: this file says an answered item *leaves*, so **answering one of
those four destroys the only copy of why it was asked**. Tonight's run wrote
recovered records for all four into the Colorless section, so nothing is
currently at risk — the question is whether the rule gets a guard. ·
*Cost of leaving it:* it has happened once in six items and will happen again
the next time a run is short of night; each time, the price is paid by the
person answering. · **Recommendation:** yes — a small test asserting every open
item here names the ledger section holding its record, with the section names
read out of `LEDGER.md`'s own headings rather than restated, and a failure when
it finds no items at all so an inert guard cannot pass quietly. It was not
built tonight because the whole 2026-08-23 block would fail it — none of those
six names a ledger section and four have none to name — and repairing them
means editing that block, which this rainbow was told to leave alone.
Ledger: Colorless, 2026-08-24.

**2. Editing a comment in five particular packages silently throws away the
deployed instance's whole simulation cache.** `internal/sim`,
`internal/sim/tier1`, `internal/mana`, `internal/floats` and
`internal/mt19937` embed their own source, and ADR 18 hashes those bytes into
the key every stored Tier 1 result is filed under — so reflowing a sentence in
a doc comment changes the key and the instance recomputes everything it had
already paid for. Nothing fails and no test speaks. Found by doing it: three
such edits were made during tonight's comment sweep and reverted, and the
warning now sits in the code, in the skill, and beside the rule that had said a
comment-only diff is always free. · *Cost of leaving it:* nothing breaks, ever
— it is wasted compute on the instance and a trap that reads as safe. ·
**Recommendation:** rule that prose-only edits in those five packages are not
made; a comment that is genuinely wrong there gets raised with its cost
attached and fixed alongside a real change to the same package, so the cache is
discarded once for a reason rather than twice for tidiness. Ledger: Colorless,
2026-08-24.

**Red —**

**1. The walk owed on Red's PR, and it is four minutes.** Five surfaces that
used to answer a wait with a motionless `Loading…` now turn the shared spinner.
Start both halves — `./mtglab ui` (port 8765) and `npm --prefix web run dev`
(port 5173, which proxies `/api` and `/tarot` to it) — then look at
**`/learn`** (Words tab: "Reading the glossary…"; Colours tab: "Reading the
colour guide…") and **`/decks/new`** ("Reading the colour guide…", and at the
tarot door "Gathering the readers…" and "Opening the interview…"). **Cycle
time: the spinner turns once per second, but the state itself is only on
screen for about 200ms against a local API** — those endpoints do 0.6–0.7ms of
server work — so throttle the network in devtools (Slow 4G) or you will watch
a hole. · *Cost of leaving it:* nothing; the PR is green and unmerged. ·
**Recommendation:** walk it, merge it. The change is **unwalked on the
deployed instance because it never deployed** — nothing merged tonight.
Ledger: Red, 2026-08-24.

**2. `tools` gated nothing at all, and half of that is still yours to flip.**
`deploy`'s `needs` named six of the file's seven other jobs: the `tools` job
arrived with the Go crossing and nothing wired it in, so **every deploy since
has shipped without the toolbox gate ever having had to be green** — and that
gate holds every committed asset to its recipe, which is commandment 9's
provenance half, not a lint. Red's PR fixes `needs` and rebuilds the guard
that derives the set from `ci.yml`'s own job list (this is 2026-08-23's item 3
with the hypothetical removed — it had already happened). `tools` is also not
a *required context*, which is a repository setting. · *Cost of leaving it:* a
red toolbox still merges, silently. · **Recommendation:** `gh api -X POST
…/protection/required_status_checks/contexts -f 'contexts[]=tools'`, and while
you are in settings, `allowed_actions` is still `"all"` and non-provider
secret patterns are still off — both free. Ledger: Red, 2026-08-24, queued 9.

**3. Five CodeQL alerts stand open against Python that no longer exists, and
nothing will ever close them.** Four `py/polynomial-redos` and one
`py/stack-trace-exposure`, all pointing into `src/mtglab/**`. `codeql.yml`'s
matrix is now `javascript-typescript` and `go`, so Python is never re-analysed
and the alerts can never auto-close. · *Cost of leaving it:* the Security tab
permanently reads "5 open", so the next real Go or TypeScript alert arrives in
a list nobody has been able to empty — the "cries wolf, gets muted" failure
`codeql.yml`'s own header warns about. · **Recommendation:** dismiss all five
as `won't fix`, citing #272. Yours because dismissing a security alert is a
security action. Ledger: Red, 2026-08-24, queued 10.

**4. The volume restore drill is now due on its own rule, not just wished
for.** Red's checklist says a drill older than the newest schema migration is
due, because the ladder is forward-only and a restore crosses it — and the
crossing rebuilt the ladder entirely (twelve scripts under
`go/internal/auth/migrations/`). The restore path has still never been walked;
HOSTING §5 says so honestly. Snapshots are healthy: five, newest 21h, 5-day
retention, 798 MiB. · *Cost of leaving it:* the library's one standing copy
(ADR 30) is behind a procedure nobody has ever run, and five days is all the
retention there is. · **Recommendation:** walk it once against a **scratch**
volume forked from the newest snapshot and a throwaway machine — never against
`mtglab_data` — then date it in HOSTING §5. Cents of volume for an hour.
Ledger: Red, 2026-08-24, queued 11.

**5. 91% of the api suite's allocation is one password hash, in the package
that is the CI critical path.** `go (amd64)` is the longest job in 20 of 22
runs, and its whole cost is one step (171s of tests; the arm64 sibling runs
the identical step in 89s). Profiling the suite: `internal/api` is 87.63s of
473 package-seconds, and **1,805 MB of its 1,981 MB of allocation is
`argon2.initBlocks`** — the test rig hashes at the production
`MemoryCostKiB = 19_456`, roughly 95 real Argon2id hashes per run. CPU cost is
only 1.9%; this is GC pressure. · *Cost of leaving it:* CI stays as slow as it
is, and every contributor waits for it. · **Recommendation:** hand it to
Black, not to a quick fix — the caveat is that `MemoryCostKiB` is load-bearing
for `NeedsRehash`, so a cheap test profile must not become the thing the
rehash check compares against. Red found *where*; whether and how is Black's
by the skill's own division. Ledger: Red, 2026-08-24, queued 12 note.

**6. Seven controls start async work and never stop accepting clicks — two of
them write.** Re-measured tonight: 131 buttons outside tests, 21 whose
`onClick` starts async work, 7 with no `disabled`. The two that matter are
`DeckDetail.tsx`'s `save()` and `returnCard()`, so a double click is a double
edit and ADR 28 records both. This is your standing complaint from 2026-08-23,
still true. · *Cost of leaving it:* a duplicated edit in somebody's deck, and
five other controls that read as broken when nothing happens. ·
**Recommendation:** yes, as its own small branch — the two writes first. The
pattern is a busy flag driving `disabled` **and** a visible pending state,
both halves or neither; `Spinner` and `.btn:disabled` already exist. Not
attempted tonight because doing it properly inside `DeckDetail`'s state is
more than the night had left. Ledger: Red, 2026-08-24, queued 12.

**7. Two dates inside the next three weeks, and nothing watches either.**
**Sonnet 5's introductory pricing ends 2026-08-31 — seven days** — after which
the same traffic costs 50% more; `prices.Table` already models both sides, so
there is nothing to build and nothing will break, but the bill changes. **The
Anthropic key expires around 2026-09-10 — eighteen days**; expiry presents as
a 401 on every Claude surface, which reads exactly like a broken integration.
Everything else is comfortable: TLS 2026-11-11, `FLY_API_TOKEN` 2027-08-14,
the domain 2027-08-13. · *Cost of leaving it:* one surprise bill and one
morning spent debugging an integration that is merely lapsed. ·
**Recommendation:** put both in your own calendar — the project has no place
to hold a date, which is itself the honest answer until queued item 1's
external monitoring exists. Ledger: Red, 2026-08-24, expiry calendar.
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
