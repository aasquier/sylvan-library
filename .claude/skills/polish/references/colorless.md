# Colorless — The Artifacts

One facet with five parts, and a single question underneath all of them:
**is the pass itself still working?**

Colorless is the sixth run and it goes last, after White, Blue, Black, Red and
Green — which is how Magic sorts a collection and also the only order that
makes sense here, because this run audits what the other five just did. An
artifact in Magic belongs to no colour and works for whoever holds it; the
tooling, the ledger and this skill are exactly that. They serve every colour
and no colour owns them, so without a run of their own they are the thing
nobody ever sweeps.

It is also the cheapest run in the cycle. Nothing here needs the live
instance or a network — parts one through four are reading, arithmetic, and
one or two tool invocations, and part five's comment sweep adds only local
edits proved by the local gauntlet — so a rainbow that has run long is still
worth finishing.

## Part one — is the ledger telling the truth?

The ledger's whole claim is that the pass is cumulative. This is where that
claim gets checked — **not by emptying the queue, which is the cleanup step's
job.** The division matters, because the two runs look alike from a distance:
**cleanup asks "can this be landed?"; colorless asks "is this entry honest?"**
Cleanup acts on the queue. Colorless audits it and leaves it.

- Walk every **deferred** item and check its **trigger**, the field that makes
  deferral honest. A trigger that has arrived and gone unnoticed is a finding.
  A deferred item with no trigger at all is a bug in the entry — give it one
  or promote it to the queue.
- Spot-check the **fixes** each colour claimed: two or three against the tree.
  Is the thing still fixed, and is the guard that holds it still there? This
  is where a fix quietly reverted by a later merge shows up, and nothing else
  in the cycle looks.
- Check that every waiting item appears in **both** places it must — the
  ledger for its context, `DAYBREAK.md` for its one line. An entry in one and
  not the other is the drift that makes a queue untrustworthy: waiting in the
  ledger alone means nobody sees it, and a daybreak line with no ledger entry
  means nobody can act on the yes. **This is the rule that actually breaks**,
  and it breaks in the daybreak-only direction, because writing the queue line
  is what a run remembers and writing the record is what it runs out of night
  for. Measured 2026-08-24: four of the six items opened the previous day
  existed only as daybreak lines. **Repair the missing side rather than
  reporting it** — a queue item whose only copy is the queue is destroyed by
  being answered, since the line leaves and takes the reasoning with it. Look
  for the third hiding place too: a finding written into the document it is
  about (a README, a package comment) and never entered anywhere.
  **And walk the other direction, which the guard cannot.**
  `daybreakrecord_test.go` proves every queue line has a record; nothing
  proves every record has a line, because "queued" in ledger prose carries no
  marker a test can read. So `grep -n 'Queued for Aaron' docs/polish/LEDGER.md`
  and, for every block, ask whether the file Aaron reads has ever carried it —
  `git log -S '<a phrase from the item>' -- docs/polish/DAYBREAK.md` answers
  "ever" rather than "now". Measured 2026-09-19: **five items had waited in
  the ledger alone since August**, each re-verified and re-carried by its own
  colour every run, each called out as a standing gap, none ever in front of
  him — and two more had been answered by the world without being struck.
  Both cleanups had untapped the queue and verified it against the ledger,
  which is the guard's direction, and so missed all seven. Re-file the live
  ones under one heading that says why they are arriving now; close the
  answered ones beside their originals.
- **The fourth hiding place is "it rides the report", and this run invented
  it.** Measured 2026-09-26: the two questions the 09-19 Colorless run raised
  about how the pass writes were recorded in the ledger with the words *"Both
  ride the report"*, the Cleanup run behind it wrote the same choice under its
  own heading, and `git log -S` on either question's own words against
  `DAYBREAK.md` returns **nothing, ever**. So the run that discovered the
  ledger-only breach created two more of exactly that class the same afternoon,
  and they sat for a week — which is the strongest available evidence that the
  failure is structural rather than careless. A report is read once and then it
  is gone; a question in one has no second copy and no address. **When this run
  finds a question it wants Aaron to answer, the line goes in `DAYBREAK.md`
  before the report is written, not after** — and when sweeping part one, grep
  the ledger for `ride the report` and its cousins alongside the
  `Queued for Aaron` blocks.
- **Corrections outrank overwrites.** When a later run finds an earlier entry
  wrong, the ledger records the correction beside the original rather than
  editing it away. A run that silently rewrote history is a finding.

## Part two — is the checklist finding things, or reciting them?

A reference file is a hypothesis about where bugs live. It goes stale exactly
like documentation, and it fails the same silent way: every item passes, the
run reports green, and the bugs are somewhere the file never looks.

- For each colour, ask **what its last run actually found** and compare that to
  what its reference file spends its words on. A facet that has produced no
  finding in two cycles is either genuinely healthy — say so and thin the
  section — or pointed at the wrong thing. Say which.
- Ask the harder version: **what got past it?** Every bug found outside the
  pass (a session that went hunting, something Aaron noticed, a live failure)
  is evidence about a checklist. Set the bug against the line that should have
  caught it and rewrite that line. This is how the four biggest corrections in
  this skill were written, and every one of them started as "the checklist
  said record the number, and the number was recorded, and the bug was inside
  it".
- **The skill's own citations are held now, and that is the third leg of
  `skillrecord_test.go`.** It held every *path* and every `mtglab` and
  `animist` *verb* a skill names; what it did not hold — while its own doc
  comment named that rot as one of the three it was built for — was a **test
  cited by name**, which is the strongest claim the pass makes ("this rule is
  enforced, and here is the thing that enforces it"). A renamed or deleted test
  left the skill promising enforcement that was gone, silently, which is this
  section's own "silent permission" failure one level up.
  `TestTheSkillsNameOnlyTestsThatExist` closes it. So when you make a claim in
  a reference file machine-checked, **point at the guard by test name** — the
  citation is now itself checked, which makes naming the test strictly better
  than describing it.
- **A rule enforced by nothing drifts** — the pass's own lasting lesson, and
  since 2026-08-19 it is a standing question in `SKILL.md` step 2, because
  three colors found it independently in one rainbow while the lesson sat in
  this file, which only the sixth run reads. What stays *here* is the sweep of
  the skill's own surface: the reference files and this one are full of
  absolutes nobody checks. And the corollary, which belongs to whoever
  proposes a guard: **a guard whose failure mode is silent permission is worse
  than none**, so it needs a test that fails when the guard is *inert*.
- Check the skill's own description line still triggers on the words Aaron
  actually uses. A skill that does not fire is a skill that does not exist.

## Part three — the tooling

The developer shelf is artifacts in the plainest sense — and part of it is
still an absence. The cache register is half-built (`mtglab sim cache` lists
the Tier 1 cache's contents and can clear them; both caches in the tree count
their own use in-process — the door's ETag memo through `etagCounts` in
`go/internal/door/static.go`, the Tier 1 store through `cache.Store.Counts` in
`go/internal/sim/cache/store.go` — and nothing yet reads either count out of a
running instance).

**"There is no bench suite" stopped being true on 2026-09-26 and the standing
item is narrower now.** Benchmarks live beside the code as `*_bench_test.go`;
for a month that meant only the four determinism kernels, and that day two
landed outside them — the deck shelf's read-and-parse and the door's compressor
— each written by the colour that needed the number, and one of them produced
the run's biggest measured win. So a `Benchmark…` outside the kernels is now
ordinary rather than a finding. What is **still** missing is the two things a
*suite* is: **one command that runs them all**, and **a recorded ledger of
results**, which is the difference between a benchmark delta and a benchmark
trend. Re-scope this item to the runner and the ledger; do not carry it as
"write the first benchmark", which is answered.
Mutation sampling has left that list — `gremlins` is the tool now, and
White's testing facet owns it. What else survives is `animist verify` in
`tools/`, plus the stock Go toolchain the other colors measure with (the
shelf section in `SKILL.md` lists it). Nothing else in the cycle owns the
shelf, and a measuring tool that has gone wrong is worse than no tool,
because its numbers are believed.

**A live question for the rebuild, every run: what would the Go shelf measure
that the stock tools cannot?** If the honest answer keeps coming back "very
little beyond a cache register and a benchmark ledger", that is a finding
about the rebuild's *shape* — say so and re-scope it, rather than carrying a
plan for a tool the toolchain made redundant.

- **Run each one and read the output as evidence about the tool**, not only
  about the code:

  ```bash
  cd tools && .venv/bin/animist verify   # committed assets vs their recipes
  ```

  For the retired instruments the question is the rebuild item itself: is it
  still queued, still shaped right, and has anything landed that changes what
  the Go shelf should measure first. The colorless questions the old tools
  answered — did the catalogue move, are the recorded survivors still alive —
  go unanswered until the shelf returns, and that gap is worth a ledger line
  each run so it cannot silently become permanent.

- **A shrunken run still prints a table.** The retired bench named its
  unavailable targets for exactly this reason, and the stock tools do not: `go
  test` prints `no test files` in the same green column as a passing package,
  and a benchmark filtered out by `-run` reports nothing at all rather than
  reporting that it ran nothing. Count what ran before reading what it said.
- **Check the tool against a bug it is supposed to catch.** The instruments
  here were each built from a specific failure, and the honest test is to
  reproduce that failure and watch the needle move. A tool nobody has
  re-validated is a tool on trust.
- **Mutation kill rate is a trend, not a grade.** Record the sample size, the
  seed and the rate. A rate that moved needs its cause named: new tests, new
  code, or a different draw. Survivors carried forward unread across two runs
  are a finding about the pass, not about the suite.
- Ask what the shelf is still missing, and notice how the list keeps
  shortening: `gremlins` answered the exhaustive-mutator item, every cache in
  the tree counts its hits, and benchmarks now exist outside the kernels. So
  three standing items remain and each is a *runner* or a *record* rather than
  a first instance: `benchstat`, which is the difference between a benchmark
  delta and a benchmark *finding* (installed on demand, never in the
  toolchain); the bench suite's one command and its results ledger; and reading
  a cache's counts out of a running instance. **Every one of the three is
  plumbing over an instrument that already exists**, which is a real fact about
  the shape of the rebuild and worth saying out loud each run rather than
  re-deriving. Anything else proposed here is a new dependency — queued with
  the arithmetic, never adopted mid-run.

## Part four — the leftovers

Findings that belong to two colours tend to belong to neither in practice.
This is where they get a home.

- Read the five sections side by side for **contradictions**: two colours
  proposing opposite things, or the same fix proposed twice under different
  names. Resolve it here and write down which colour owns it next time.
- Note the **dependency order** that keeps not working. The standing one:
  Blue's docs-and-memory audit runs second but generates most of its value
  last, because the three colours after it produce the drift it hunts. Do not
  reorder the rainbow to fix it — carry the drift into this run and let
  Colorless close it, which is what going last is for.
- **Refresh the staleness picture.** A colour whose last touch was a
  `converge` survey is staler than its date says. Write the honest ordering
  for the next bare `/polish` at the bottom of the ledger so the next session
  does not have to re-derive it.
- **The relic sweep** (Aaron's ask, 2026-08-21, sharpened 2026-08-23: *make
  the artifact search thorough*). The failure this exists to prevent is not
  missing a relic — it is **sweeping by memory**, walking the parts of the
  tree a session already has in context and calling that the tree. A relic
  lives exactly where nobody looks, so the sweep has to be *enumerative*.

  **Enumerate, then judge. Never the other way round.** Six passes, each one
  a command whose output is a complete list, and each one finds a different
  kind of leftover:

  ```bash
  git ls-files | sed 's#/[^/]*$##' | sort -u        # 1. every tracked directory
  git ls-files | grep -vE '\.(go|ts|tsx|css|md|json|ya?ml)$'   # 2. odd file types
  git log --diff-filter=A --name-only --since=... --format=  # 3. what arrived, by era
  git ls-files -- '*.md' | xargs -n1 head -1        # 4. every doc, by its own title
  ./mtglab --help  (and each subcommand's)          # 5. every command and flag
  # 6. non-source files nothing else in the tree names -- see the note below
  git ls-files | grep -E '\.(md|ya?ml|toml|sh|sql|txt|json)$' \
    | grep -v /testdata/ | while read -r f; do b=$(basename "$f")
      grep -rqF --exclude-dir={.git,node_modules,web_dist} --exclude="$b" \
        "$b" . || echo "$f"; done
  ```

  Pass 1 is the one that pays and the one always skipped: **read the whole
  directory list out loud against the current architecture** and stop on any
  directory you cannot immediately say the purpose of. Pass 5 is its
  equivalent for behaviour — a CLI grows commands and never loses them, and
  a flag whose data moved to the volume is a relic that still runs.

  **Pass 6 only works over non-source files, and the version that did not say
  so was useless.** Go and TypeScript reach each other by package and import,
  never by filename, so a basename sweep over the whole tree reports ~180
  source files as unreferenced — a list nobody reads, which is the same
  failure as not running the pass. Source reachability is the compiler's
  question and `deadcode`/`knip`'s, not this sweep's; what this sweep can see
  is a doc, a workflow, a recipe or a schema that nothing names. Read its
  output expecting *convention-loaded* survivors — an embedded migration glob,
  a `tsconfig`, a `dependabot.yml` GitHub reads by name — and stop on anything
  that is not one. **2026-08-24: 12 rows, all of them the migration ladder.**
  Two mechanics worth knowing before reading a row: a file almost never
  contains its own basename, so the self-exclusion is belt-and-braces rather
  than load-bearing; and **naming a file in the ledger makes it referenced**,
  so a relic queued here and left alone stops appearing in later sweeps.

  The kinds to expect, so none is dismissed as "probably fine": directories
  from an earlier shape; template and fixture files describing a workflow
  nobody runs; doc sections narrating a phase that ended; commands and flags
  whose data or purpose migrated (Green's hosted-first facet owns the
  *capability* question, this sweep owns the *existence* question); test
  helpers for deleted subjects; ignore rules matching nothing; config keys
  and environment variables nothing reads; dependencies nothing imports;
  and **links pointing at paths that no longer exist** — the standing
  example being an accepted ADR citing a directory a later sweep deleted,
  which is unfixable in place because ADRs are immutable and is therefore
  exactly the kind of thing that must be *found and raised* rather than
  quietly tidied.

  For each: name it, say what shape it belonged to, and queue keep/retire for
  Aaron — **a relic is a decision, never a silent deletion.** The test is
  Green's, generalised: a thing earns its place by what the *current* shape
  needs from it, not by having always been there. Record in the ledger which
  passes were actually run, because a sweep that ran two of six and reported
  "nothing found" is the failure mode this whole entry exists to stop.

## Part five — the comments are artifacts too

The relic sweep at line granularity, and Aaron's standard for it is one
sentence: **the only thing that should ever be in a comment is what helps
Claude develop the code** — the primary-developer frame in SKILL.md. This
repo deliberately writes comments that carry an argument, and that rule is
not in question; what creeps in beside the argument is *residue of the
making* — the date somebody noticed, the punch list it came off, the PR that
fixed it, the sentence it used to say.

**The baseline, with the commands that produce it**, because a number nobody
can reproduce is not a baseline — the first measurement here was recorded bare
and its `web/src` half could not be reproduced by any obvious variant on the
next run (87, 117 and 151 were all available; the recorded figure was 101):

```bash
grep -rnE '^\s*//.*20[0-9]{2}-[0-9]{2}-[0-9]{2}' go --include=*.go | wc -l
grep -rnE '^\s*//.*20[0-9]{2}-[0-9]{2}-[0-9]{2}' go --include=*.go \
  | grep -vc _test.go
grep -rnE '(^|\s)(//|\*|/\*).*20[0-9]{2}-[0-9]{2}-[0-9]{2}' web/src \
  --include=*.ts --include=*.tsx --include=*.css | wc -l
grep -rnE '#[0-9]{2,3}|\bv[0-9]{3}\b' go --include=*.go   # the other residue
```

**2026-08-24: 89 dated in Go, 60 outside tests, 87 under `web/src`.** Dates are
only the visible half — PR numbers, `vNNN` build tags and punch-list references
are the same residue and the last grep is how they are found.

The line, and it is sharp:

- **Keep** what serves discovery of the code itself: the invariant, the trap,
  the thing that looks wrong and is not, the reason the obvious alternative
  was rejected. *"Anchored, because a bare pattern also matches the embedded
  data directories"* is doing work forever.
- **Cut** everything that serves history: when it happened, who found it,
  which run fixed it, what it used to say — unless the old shape is one a
  reader would otherwise reintroduce, and then one clause is the whole
  budget.
- **A date stays only when the date is the fact**: an expiry, a version
  floor, a pricing cutover, a deprecation window — **and a validation
  measurement**, which is the fourth kind and was found by sweeping. *"Validated
  against 3,000 Tier 1 games per deck on 2026-08-21: mean error -0.06 mana"* is
  not a diary entry, because its date is how a reader decides between trusting
  the formula and re-validating it. Strip the date and the sentence quietly
  claims to be current forever.

The test for every comment: **would a fresh Claude session, reading only the
code, act differently for having read this sentence?** If no, it goes; if
yes, it stays whatever its style. Convert rather than delete when a real
point is buried in narration — the point survives, the diary does not. Sweep
a bounded slice each run (a package, or one route family) and record in the
ledger which slices are done, so the sweep finishes over cycles instead of
restarting every one.

**The slice budget loses to the tree, measured 2026-09-12**: one slice
retires ~20 dated lines a cycle while the week added ~50 (go 183 → 235,
web/src 368 → 426). **Aaron ruled the ratchet in (2026-09-13) and it is
built**: `go/cmd/mtglab/datedcomments_test.go` pins the count outside tests
— Go's own comment scanner for `go/`, comment-led lines for `web/src` — under
a ceiling that may not rise unnoticed and is lowered when a sweep banks a
material gain (slack 10). Two consequences for this part. The greps above are
still how a *family* is chosen (the ratchet counts the tree, not the file),
but the number that goes in the ledger is the ratchet's, read with `go test
-count=1 -v -run TestDatedCommentsDoNotOutgrowTheirCeiling ./cmd/mtglab/` —
a sweep that quotes its own grep is back to the unreproducible baseline this
section opened with. And a slice that retires more than the slack **must
lower the ceiling in the same diff**, which is the test failing on the fall
side and is the sweep's own mutation check: a slice that did not trip it did
not bank anything.

**The keep/cut rule itself now lives in `datedcomments_test.go`'s const block
rather than here**, written there on 2026-09-26 after the ratchet had moved
twice on each side: one question (*would a fresh session act differently for
knowing the day?*) and three answers (a ruling loses its date, a
when-it-happened clause loses its date, a validation measurement keeps it).
Read it there, because that is where a session raising or lowering a ceiling is
already looking. Two facts about choosing a family that belong here instead:
**a swept family re-accumulates** — `internal/sim/tier3` was swept on
2026-08-24 and the Coliseum's next fortnight refilled it to 29, which is why
this is a standing job rather than a finishable one — and **the fingerprint is
the only hard exclusion**, so a package that merely *sounds* like the engine
(`sim/tier3`, `textutil`) is fair game; check the list in
`internal/sim/cache`'s `engineSources` rather than guessing from the name.

The sweep keeps its judgment-only shape — it is how the
keep/cut line gets applied — so choose the densest untouched family, read
every line, and let the ceiling say whether the totals shrank.

**Five packages are not sweepable, and the reason is ADR 18.**
`internal/sim`, `internal/sim/tier1`, `internal/mana`, `internal/floats` and
`internal/mt19937` each embed their own source (`SourceFS`) and are hashed
into the Tier 1 cache's engine fingerprint — `internal/sim/cache`'s
`engineSources` is the list, and its own doc comment now says this. **A
reflowed comment in any of them changes the key and discards every stored row
on the volume.** Nothing fails, no test speaks, and the instance silently
recomputes what it had already paid for. Choose a slice outside them.
**Aaron ruled it (2026-09-05): a prose-only edit in those five packages is
never made.** A comment there that is genuinely wrong is queued with its
cost attached and fixed beside a real change to the same package, so the
cache is discarded once for a reason rather than twice for tidiness.

## What this run never does

- **It does not re-audit the five colours.** If it finds a real bug in code,
  it records it in that colour's section as a finding for that colour's next
  run — this run's diff belongs to the skill, the ledger and the tooling.
  A colourless run that starts fixing product code has stopped being
  colourless. **One carve-out, Aaron's: comment-only diffs are this run's to
  make.** A comment is not product behaviour, so part five's sweep breaks no
  rule about touching product code, and the gauntlet still runs to prove
  exactly that. **The carve-out used to justify itself with "the binary and the
  committed bundle come out byte-identical", and that is false in five
  packages** — see part five's fingerprint note. The correction is worth
  keeping visible rather than editing away, because the wrong reason is the
  kind a fresh session would re-derive.
- It does not open a PR for the ledger alone. Same rule as every other run:
  a doc change rides the branch that does the work. If the only output is
  ledger text and skill edits, that *is* the work and the PR is legitimate —
  this facet is the second place, after Blue's docs audit, where a mostly-prose
  PR is honest.
