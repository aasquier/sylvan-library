---
name: polish
description: "The recurring quality pass over the sylvan-library codebase and infrastructure, organised as the five colors of Magic. Use whenever Aaron asks for a polish pass, quality pass, sweep, or audit — of Go or TypeScript/React best practices (or the animist media toolbox under tools/), testing (including mutation testing and t.Parallel discipline), performance and profiling, security, licensing/free-use compliance, Claude API efficiency, CI/CD, alerting, cloud resources, infrastructure efficiency/upgrades/cost, browser/mobile compatibility, scalability, the repo's Claude-facing docs (including trimming stale or verbose context), or the spirit of Magic — sweeping copy and UI for chances to prefer Magic: the Gathering terminology and iconography over plain conversational English. Also covers hosted-first alignment (anything the deployed product needs that exists only as a local CLI or laptop workflow), the relic audit (early-dev leftovers — files, docs, commands — that no longer fit the current shape), and the measuring shelf (benchmarks, profiling, cache hit rates — the Go rebuild is an open item). Also triggers on a color invocation (polish white/blue/black/red/green), on 'polish colorless' (the artifacts pass over the skill, the ledger and the dev tooling), on 'polish cleanup' (the end phase that lands the stragglers every color queued), on 'polish converge' or 'polish all' (all five colors merged into one report), on 'polish rainbow' (all seven as separate runs, one at a time), on 'run the polish pass', on asking what polish findings are still outstanding or waiting on a ruling, and on any 'are we doing X right?' question about one of those areas — even if the word polish never appears."
---

# The Polish Pass

A recurring, ledger-driven quality pass over this codebase and its
infrastructure. Aaron's quality facets are organised as the five colors of
Magic, plus colorless and a cleanup step; one invocation runs **one color,
deeply**. The point is not a skim of everything — it is that over a cycle of
seven runs every facet gets a real audit, the queue those audits fill gets
emptied rather than inherited, and the ledger makes the next run smarter than
the last.

## Before anything else

Read `CLAUDE.md` in full if you have not this session. **The Commandments
outrank every checklist in this skill.** In particular: this is a
collaboration (ask Aaron rather than guess at design decisions), the project
is free forever and lawful about it, surgical trims over mass restructuring,
and CI is never a surprise. When a checklist item here conflicts with a
Commandment or an ADR, the checklist loses — and note the conflict in the
ledger so the checklist gets fixed.

## The colors, and the two phases that are not colors

| Color | Theme | Facets |
|---|---|---|
| **White** | Law & Protection | Free-use/licensing compliance (triple-checked); security & user isolation; testing discipline |
| **Blue** | Craft & Knowledge | Go best practices *and the modern-Go sweep*; TypeScript/React best practices; **controls** (commandment 17 made checkable); the animist toolbox in `tools/`; Claude-first docs & memory audit, *including the code's own comments*; the spirit of Magic (the game's terminology and iconography over plain English, commandment 3) |
| **Black** | Ruthless Efficiency | Claude API spend; static assets over hotlinks; performance & efficiency |
| **Red** | Speed & Alarum | CI/CD pipeline; alerting & self-healing |
| **Green** | Growth & Resilience | Browser & mobile compatibility; cloud resource watch; scalability & user adaptability; hosted-first alignment (nothing the product needs exists only on a laptop) |
| **Colorless** | The Artifacts | The pass auditing itself: did last cycle's findings land; is each checklist still finding things; the dev tooling (the retired shelf's Go rebuild, `animist`); the relic sweep; cross-color leftovers |
| **Cleanup** | The End Step | Not a color and not an audit: the phase where the stragglers every color queued are put in front of Aaron and **landed** |

Each color has a reference file — `references/white.md` and so on — holding
the full checklist, the repo-specific anchors, and the known traps. Read the
one you are running, and only that one; the others are for their own runs.

## The measuring shelf

The purpose-built `bench` and `mutate` commands and the cache register
retired with the old backend, and **rebuilding that shelf over the Go
packages is an open ledger item.** Until it lands the stock Go toolchain is
the instrument set — richer than what it replaces — and the rule is to put
**raw tool output in the ledger, never a summary of it.**

Everything below runs from `go/` with this Mac's three exports set (CGO on,
the 1.26 toolchain on `PATH` *and* `GOROOT`); `animist` runs from `tools/`.

```bash
go test -run '^$' -bench . -benchmem ./internal/<pkg>/   # ns/op AND allocs/op
go test -run '^$' -bench . -count=10 ./internal/<pkg>/   # then: benchstat a.txt b.txt
go test -cpuprofile cpu.out -memprofile mem.out -run TestX ./internal/<pkg>/
go tool pprof -top -nodecount=25 cpu.out                 # -http=: for the graph
go test -json ./... | grep '"Action":"pass"'             # per-test durations
go build -gcflags=-m ./internal/<pkg>/ 2>&1 | grep escapes
go test -race -count=1 -cover ./...                      # what CI runs
cd tools && animist verify                               # assets vs recipes
```

Four rules, each bought by a real wrong answer:

- **`-benchmem` or it is not a benchmark.** In Go the usual cause of slow is
  allocation, and `ns/op` alone cannot see it. A change that moves `ns/op`
  by 5% and `allocs/op` by zero has probably measured the machine's mood;
  `-count=10` through `benchstat` is what tells the two apart, and a
  single-run delta is not a finding.
- **A CPU profile is nearly blind inside cgo, which is where the card pool
  lives.** DuckDB work lands under `runtime.cgocall` with no shape to it,
  so the database half must be timed *at the query* and never inferred from
  the profile or found by subtraction. This is the retired shelf's hardest
  lesson in its Go form: profile the Go half, clock the cgo half.
- **A cache can be correct, tested, and never once used.** Only a counter
  finds that; no test can. A cache added since the last run with no hit
  count is a finding.
- **Mutation runs go on a throwaway copy** of the package, never the working
  tree — and a harness that cannot prove it ran is reporting `SURVIVED` for
  a mutant that never compiled.

## Choosing the color

- If Aaron named a color or a facet, run that color. A facet name maps
  through the table above ("security sweep" → White; "CI runtime" → Red).
- If Aaron named **two** concerns, or an ask that spans exactly two colors
  ("CI feels slow and the volume's filling up" → Red + Green), run **those
  two colors** deeply, in WUBRG order, in one session. This is the middle
  ground between one color and all five: still real depth per color, not a
  shallow survey. Say which two you picked and why. (More than two distinct
  concerns is a colorless run — see below.)
- If Aaron asked about the **pass itself** — the skill, the ledger, the
  tooling, whether old findings ever landed — that is **colorless**. It is a
  run of its own, not a survey.
- If Aaron asked to **clear the queue** — land the stragglers, act on what is
  waiting on him, "what is still outstanding from the polish runs" — that is
  **cleanup**. Colorless and cleanup are neighbours and easy to confuse:
  colorless *audits* whether the queue is honest, cleanup *empties* it.
- If Aaron asked for **all**, everything, or the full cycle in one report —
  run **converge** (`/polish all` is the same thing). If he asked for all of
  them as **separate** runs — run **rainbow**. Both are below.
- Invoked bare, read the ledger and pick the color that most needs a run:
  staleness first (longest since last run), then urgency (a trend line moving
  the wrong way, or queued findings piling up, beats mere staleness). Ties
  break in WUBRG order, with colorless last. Say which color you picked and
  why before starting.

## Colorless — the artifacts, and the pass auditing itself

`/polish colorless` is its own run, not a survey of the other five. Its
subject is everything that belongs to no color and therefore to nobody: **the
skill, the ledger, and the developer tooling.** Whether last cycle's queued
findings ever landed. Whether each checklist is still finding things or has
started reciting them. Whether the instruments still measure what they claim
— and whether the retired shelf's Go rebuild is still shaped right. And the
findings that fell between two colors, which in practice means they fell out
of both.

It goes **after** White, Blue, Black, Red and Green — because it audits what
they just did, and because that is the order Magic sorts a collection in. Only
the cleanup step comes after it. `references/colorless.md` is the checklist.

Two things to hold on to. **It does not re-audit the five colors**: a real
code bug found here gets written into that color's section as a finding for
that color's next run, because this run's diff belongs to the skill, the
ledger and the tooling. And it is the **cheapest run in the cycle** — no live
instance, no card pool, no network — so a rainbow that has run long is still
worth finishing.

## The Cleanup Step — where the stragglers get admitted

Every color leaves things behind. That is by design — the triage in step 3
sends anything needing Aaron's ruling, a dollar, a dependency or a migration
to the ledger rather than into the run's diff — but the consequence is that
the ledger fills with real findings nobody ever *lands*. Six audits produce
six queues and no seventh pass to clear them, so the queue only grows.
`/polish cleanup` is that pass, and it is Aaron's ask by name (2026-08-23).

It borrows Magic's own last step of the turn, and the borrowing is exact:
the cleanup step is where "until end of turn" effects end, damage wears off,
and **the active player discards down to hand size**. You cannot carry
everything forward. That is the honest shape of this phase.

**It is not a color and not an audit.** It opens no checklist and hunts no
new findings; its entire input is the ledger. A cleanup run that starts
auditing has become a seventh color and stopped doing the one job nothing
else does.

Run it in three beats:

1. **Untap — read the whole queue and re-check it.** Every queued and
   deferred item across all six sections, oldest first. For each, one of four
   answers: *still true and still needs Aaron*; *still true and no longer
   needs him* (the code moved, or a ruling elsewhere already covers it — this
   is now landable); *gone stale* (re-word it or close it, saying why); or
   *already done* by some other branch, which happens and should be recorded
   rather than re-litigated. This beat alone regularly finds that a third of
   the queue is not what it says it is.

2. **Upkeep — put the decisions in front of Aaron, in one pass.** This is
   the phase that is *allowed to be a conversation*, and it is the only one.
   Group what needs him by the kind of answer wanted — a ruling, a dollar, a
   dependency, a migration window — and give each item the context to be
   decided in one line and the consequence of leaving it. **Ask them
   together.** Six sessions each asking one question is how a queue becomes
   permanent; one session asking twelve is how it empties. Commandment 1 is
   the whole basis of this beat: asking is never the wrong move.

3. **Discard to hand size — land what fits, and say what did not.** Take his
   answers and implement, under the same non-negotiables every other run
   obeys: every fix gets a test, the full gauntlet before pushing, surgical
   over structural. **The cap is real and it is the point** — a cleanup that
   touches forty files has become the mass restructure the pass forbids. Land
   the highest-value handful, and for everything still in hand write the
   reason it stayed: not "deferred" again, but *what would have to be true*
   for the next cleanup to land it. Silent carry-forward is the failure mode;
   an item that has been carried three cleanups without a stated reason is
   itself a finding.

Two rules that make it work:

- **An extra cleanup step is a real thing in Magic, and here too.** If
  landing a fix produces a new finding — and it does, because touching queued
  work is how you learn what it was hiding — you do not stop the phase. You
  record it and run the beats again, once. Twice is a signal the queue needed
  a color's run, not a cleanup.
- **It goes last, after Colorless.** Colorless audits whether the checklists
  and the tooling still work and rewrites them; Cleanup acts on what they
  produced. Running it before Colorless means landing decisions that
  Colorless is about to re-frame.

Tag its ledger entries `YYYY-MM-DD (cleanup)`, and — the part that makes the
phase measurable rather than merely virtuous — **record the queue depth
before and after.** A cleanup that did not shorten the queue should say so
plainly, with the reason.

## Converge — all five colors, one merged report

`/polish converge` (also `/polish all`) runs every color in one session and
blends them into a single report. Converge is the right word: Magic's mechanic
counts how many colors of mana were spent on one spell, and this is one spell
paid for with all five. Be honest about what that costs — it is a **survey of
the whole realm**, deliberately shallower per color than a solo run, because
five solo runs' depth does not fit one session and pretending otherwise
produces five skims labelled as audits.

Run it in two phases:

1. **Audit everything first, fix nothing.** Work each color's reference in
   turn, collecting triaged findings and recording measurements as you go.
   Auditing before fixing keeps the survey complete even if the session runs
   long, and lets cross-color findings meet each other (Black's spend
   numbers inform Green's quota proposal; Red's probe is Green's baseline).
2. **Then land one bounded fix set.** From the full findings list, implement
   the safe fixes worth their diff — surgical cap still applies, so prefer
   the highest-value handful over everything that qualifies. One branch, one
   PR, ledger updated across all five sections.

Tag converge runs in the ledger as e.g. `2026-08-19 (converge)` — the tag
matters because a converge run resets staleness only softly: a color whose
last touch was a survey is staler than its date suggests, and the next bare
`/polish` should weigh that.

## Rainbow — all seven, one at a time

`/polish rainbow` runs the full cycle at **full solo depth** — one subagent
each, **serially, in WUBRG order, then colorless, then cleanup**: White, Blue,
Black, Red, Green, Colorless, Cleanup. Rainbow is the opposite of converge:
every color shines separately and completely, no survey shortcut. It is the
most thorough option and the most expensive — six real audits plus the phase
that lands what they queue — so it is Aaron's explicit call and his tokens; do
not reach for it when converge or a solo run would do.

**The cleanup step is what makes a rainbow finish rather than merely stop.**
Six audits with no seventh phase hand Aaron six queues; ending on cleanup
turns them into decisions and landed work. It is also the beat where he is
actually needed, so do not run it while he is away — it is a conversation.

The shape to hold in your head: **rainbow is six solo runs and a cleanup,
chained.** Each is
an ordinary run of the protocol below — main working tree, own branch, own PR —
and the next does not start until the previous one's PR has merged.

Why serial, and why that order:

- **Serial, because parallel churns.** Five branches each expanding an
  adjacent section of `docs/polish/LEDGER.md` conflict with one another by
  construction, and five PRs open at once means five `image` builds queueing
  for no gain. Merging each color before starting the next cuts every branch
  from a main that already holds its predecessors' entries, so the conflicts
  never exist rather than getting resolved. (Parallel was tried once, on
  2026-08-16; that churn is why this paragraph exists.)
- **One subagent each, for context rather than collision.** Seven runs' depth
  does not fit one context window — that is the whole difference between
  rainbow and converge — so each still gets its own fresh agent.
- **WUBRG, because Magic says so and the dependencies happen to agree.**
  Commandment 3 settles the order on its own, but it is also topologically
  correct: White's licensing law binds Black's static-assets facet, Black's
  spend numbers feed Green's quota proposal, and Red's external probe is
  Green's baseline. Every cross-color dependency points forward.
- **Colorless last, because it audits the other five.** It reads what this
  rainbow just found, checks each checklist against it, and closes the loop
  that Blue's docs-and-memory audit cannot: Blue runs second but generates
  most of its value last, since the colors after it produce the drift it
  hunts. That used to be a wrinkle the ledger carried into the next cycle.
  It is now a run.

Orchestration:

- Run each color **in the main working tree**, not a worktree. Only one color
  is live at a time so there is nothing to isolate from, and the main tree
  already has `web/node_modules`, a warm Go build cache and the card pool — a
  fresh worktree has none of the three and must rebuild all of them before it
  can run the gauntlet, which was the parallel version's hidden tax.
- Give each agent exactly one color and its reference file, the ledger, and
  the same run protocol and non-negotiables below.
- **Each lands its own branch and PR**, kept independently reviewable — never
  seven runs' diffs on one branch, which would be a mass-restructure by the back
  door. Each agent updates **only its own ledger section**; colorless is the
  one exception, since correcting an entry another color wrote is its job.
- **Merge before advancing.** Watch the required checks, merge when green, and
  only then start the next. Remember what merging means here: a green merge deploys
  itself (ADR 23), so a rainbow is up to seven deploys and seven brief downtimes —
  one more reason schema migrations stay a queued item.
- A color with only queued findings and no safe fix opens **no PR**. Carry its
  ledger text onto the next branch and move straight on; there is nothing to
  merge and so nothing to wait for.
- You are the collector: relay each color's report as it lands — the agent's
  own report never reaches Aaron — resolve cross-color overlap when a later
  color re-proposes an earlier one's fix, and close with one consolidated
  summary. Tag each ledger section `YYYY-MM-DD (rainbow)`.
- A rainbow may outlive a session, and that is fine. The merged PRs and the
  ledger are the resume point: read the ledger, see which colors already carry
  this rainbow's tag, and pick up at the next one in WUBRG order — colorless
  after Green, cleanup after colorless.

## The ledger

`docs/polish/LEDGER.md` is the pass's memory: per color, the date of the last
run, the PR that landed it, findings queued for Aaron, deferred items, and
measured numbers (CI runtimes, bundle size, volume usage, response times).
It is what makes the pass cumulative rather than repetitive.

- Read it at the start of every run. A queued finding is *not* re-litigated —
  it is waiting on Aaron, so leave it unless this run learned something that
  changes it. A deferred item is fair game if its trigger has arrived. **The
  cleanup step is the one phase this rule does not bind**, because emptying
  that queue is its entire job.
- Update it at the end of every run, **on the same branch as the work** —
  never as its own PR (the no-doc-only-PR rule).
- Record measurements even when they are fine. A healthy number today is the
  baseline that makes next quarter's regression visible.

## The run protocol

1. **Prepare.** Read the ledger and the color's reference file. Branch from
   `origin/main` (never work on main; never `git stash` on this repo — commit
   WIP instead).
2. **Audit.** Work the reference checklist against the current tree and, where
   it says so, the live instance. **Measure with the tools rather than by
   hand, and never guess a cause.** The shelf above is the instrument set; the
   reference files say which tool answers which question, and the ledger is
   where the numbers go. Two rules the 2026-08-19 pass paid for: **a large
   number is a question, not a datum** — anything slow gets profiled and the
   profile goes in the ledger, not just the millisecond — and **a probe finds
   *which*, only a profile finds *why***, so a sentence beginning "presumably"
   is not a finding.

   One standing question besides, whatever the color: **which of this facet's
   absolute claims is enforced by nothing?** "Always", "never", "every", "all"
   — in CLAUDE.md, in a docstring, in `web/README.md`, in this skill. For each,
   ask what fails if it stops being true; when the answer is *nothing*, it has
   already drifted or it will. The pass learned this four times before the
   2026-08-19 rainbow found five more in one day — an extras list, a `needs`
   list, a motion guard, a stated rule with no pin, a prose-only lint rule —
   which is why it now lives here, where every run reads it, rather than only
   in the colorless reference, where the sixth run does. **The fix is to make
   the claim machine-checked, never to reword it.**
3. **Triage every finding** into exactly one of:
   - **Safe fix** — implement it this run. Safe means: behavior-preserving or
     bug-fixing with a test; no new runtime dependency; no schema migration;
     no new service, spend, or infrastructure; no contradiction of an ADR or
     Commandment; surgical rather than structural.
   - **Queued for Aaron** — record it in the ledger with enough context that a
     fresh session can act on his yes. Design decisions, anything costing
     money, new dependencies or services, schema migrations, and anything an
     ADR already decided (that gets a *superseding ADR proposal*, never an
     edit — ADRs are immutable).
   - **Deferred** — real but not yet worth doing; record the trigger that
     would make it worth doing.

   The line between safe and queued is fuzziest for the *one-line change that
   only CI can fully verify*. Three live cases, all one-liners and all
   queued-class: anything in a `Dockerfile`, because the `image` and
   `image-arm64` jobs are the only place a container is ever built and neither
   can run on this Mac; anything inside a platform-tagged file, because a
   `_linux.go` body is not typechecked by a green lint on darwin; and anything
   arm64-shaped, because the Go matrix's second leg is the only arm64 compiler
   this project has. The test is not diff size; it is *can I fully prove this
   green before pushing?* If the only proof is CI itself, queue it (or land it
   alone on a branch Aaron can watch), never bundle it into a fix set.
4. **Fix.** Every bug fix gets a test — and **derive the expectation from the
   source of truth rather than restating the claim.** A test that repeats the
   sentence it is checking cannot tell you the sentence is wrong: the deck
   History tab said earlier edits were "in git" for two ADRs' worth of time
   beside a green `expect(getByText(/in git, not here/))`, and a licence-notice
   test drafted its own shelf entry and stayed green against the bug it was
   written for. The working form reads the truth back: the route test takes the
   filename *off* the shelves table rather than restating it, and the dial's
   mode list is held equal to `ModeNames()` as a set, so the next mode added
   fails there rather than three months later. Then
   **verify by mutation, not by greenness**: break the thing, watch the test
   fail, restore it. Match the surrounding code's idiom and comment density,
   and if a finding grows beyond surgical mid-fix, stop, back it out to a
   queued item, and say so.
5. **Verify.** The full local gauntlet before any push, from `go/` with the
   Mac's three exports set: `go vet ./...`, `go test -race ./...`,
   `golangci-lint run ./...`, `gofmt -l .` printing nothing; then
   `npm --prefix web run check`, rebuilding the committed bundle
   (`npm --prefix web run build`) if anything under `web/src` changed; and
   `cd tools && ruff check . && mypy && python -m pytest tests/ -q` when
   `tools/` moved. Check `data/app.db` was not dirtied by a running app
   (`ls -la data/`, not `git status`, which is blind to a gitignored
   file). For UI-visible changes,
   drive the real surface — a green jsdom test has not seen a layout. Since
   2026-08-16 that includes authenticated flows on the deployed instance:
   the `claude` account (a plain user; its edits are confined to its own
   decks and attributed by name in deck History) can be driven through the
   Claude-in-Chrome integration once Aaron has signed it in — so "verify on
   the live instance" no longer stops at the login page. For a
   **security fix**, three extra beats, learned the hard way: (a) prove the
   bug with a *mutation-verified* test — revert the guard, watch the test
   fail, restore it — so the fix is demonstrably closing a real hole, not
   decorating a safe one; (b) the fix is not done when the test passes, it is
   done when **code scanning is also green** — CodeQL is a required gate and
   its model may not recognise a correct guard, so budget for that; (c) once
   merged and deployed, drive the *live instance* to confirm the hole is
   actually shut, not just the local tree.
6. **Land.** Update the ledger on the branch, stage explicit paths (never
   `git add -A` — `decks/` is live app data), open a PR, and watch the required
   checks — **read that list back from the API** rather than from a count
   remembered in a file, because it has grown twice with no prose noticing:
   `gh api repos/aasquier/sylvan-library/branches/main/protection --jq
   .required_status_checks.contexts`. Remember a green merge deploys itself: a
   few seconds of downtime, and schema migrations apply on boot — which is why
   step 3 queues them.
7. **Report.** End with: what was fixed (and where it landed), what is newly
   queued for Aaron's ruling, the numbers recorded and any trend that moved,
   and which color the next run should be.

## What a run never does

These hold even when a checklist item seems to point the other way:

- **Never violate the free-use boundary, even transiently.** No Wizards art
  committed, no Scryfall bulk redistributed, no monetization code. White
  audits this; every color obeys it. When Black's static-assets facet meets
  White's licensing rule, White wins — a hotlink whose licence forbids
  committing stays a hotlink.
- **Never commit** pool data, `app.db`, secrets, or an email address into
  code, logs, artifacts, or the ledger.
- **Never write a card's `why`**, and never weaken the boundary tests that
  keep model output away from deck writes.
- **Never mass-restructure.** A polish run that touches forty files has
  stopped polishing. Depth over breadth; queue the big idea instead.
- **Never let the ledger become a second copy of the code's own records.**
  It holds findings, decisions-in-waiting, and measurements — not changelogs
  (git has those) and not rationale text.

## Rigor is not uniform

The licensing facet gets **triple-check rigor**: Aaron named Claude the chief
legal officer of a project that exists at Wizards of the Coast's pleasure.
For anything touching assets or the Fan Content Policy, verify the primary
source (the recipe, the PROVENANCE entry, the licence text itself), not a
summary of it — and when genuinely uncertain whether something is compliant,
the answer is to ask Aaron and leave it out until then, never to ship it and
see. The full protocol is in `references/white.md`.
