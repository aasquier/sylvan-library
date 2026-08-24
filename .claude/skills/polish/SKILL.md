---
name: polish
description: "The recurring quality pass over the sylvan-library codebase and infrastructure, organised as the five colors of Magic. Use whenever Aaron asks for a polish pass, quality pass, sweep, or audit — of Go or TypeScript/React best practices (or the animist media toolbox under tools/), testing (including mutation testing and t.Parallel discipline), performance and profiling, security, licensing/free-use compliance, Claude API efficiency, CI/CD, alerting, cloud resources, infrastructure efficiency/upgrades/cost, browser/mobile compatibility, scalability, the repo's Claude-facing docs (including trimming stale or verbose context), the quality of the controls themselves (dull buttons, missing hover/focus/press states, actions that never disable), or the spirit of Magic — sweeping copy and UI for chances to prefer Magic: the Gathering terminology and iconography over plain conversational English. Runs at night by default while Aaron sleeps, leaving anything it cannot settle alone in the daybreak queue (docs/polish/DAYBREAK.md) for the morning; also triggers on 'run the night pass', 'what is in the daybreak queue', and on asking what the overnight run found. Also covers hosted-first alignment (anything the deployed product needs that exists only as a local CLI or laptop workflow), the relic audit (early-dev leftovers — files, docs, commands — that no longer fit the current shape), and the measuring shelf (benchmarks, profiling, cache hit rates — the Go rebuild is an open item). Also triggers on a color invocation (polish white/blue/black/red/green), on 'polish colorless' (the artifacts pass over the skill, the ledger and the dev tooling), on 'polish cleanup' (the end phase that lands the stragglers every color queued), on 'polish converge' or 'polish all' (all five colors merged into one report), on 'polish rainbow' (all seven as separate runs, one at a time), on 'run the polish pass', on asking what polish findings are still outstanding or waiting on a ruling, and on any 'are we doing X right?' question about one of those areas — even if the word polish never appears."
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

## Nightbound — this pass runs while Aaron sleeps

**Assume he is asleep.** The polish pass is night work by design: Aaron starts
it and goes to bed, the orchestrator holds the context, and the next voice he
hears is a summary over coffee. Innistrad's word for it is the right one — a
run is *nightbound*, and what it cannot settle alone waits for **daybreak**.

Everything below follows from nobody being awake to ask.

**Never block. Ever.** There is no such thing as stopping to wait for an
answer — a blocked night run is eight idle hours. When a question appears,
write it to the daybreak queue in one line and **move to the next item**. A
run that ends with twelve things done and four questions asked has worked; a
run that ends with one question and nothing done has slept too.

**Only improvements land.** This is Aaron's bar and it is stricter than the
daytime one, because there is no one to catch a bad call. Before anything is
committed, it must be *provably* better, and provably means green:

- The full gauntlet, not a subset. Go's four gates, the web check, the
  toolbox's when `tools/` moved, and the bundle rebuilt if `web/src` changed.
- A test that fails without the fix, verified by breaking it once.
- No new dependency, no schema migration, no new service or spend, no
  contradiction of an ADR or Commandment.
- **If it cannot be proven green on this machine, it does not land at night.**
  The daytime rule already queues anything only CI can verify; at night that
  rule is absolute, because "push it and watch" needs a watcher.

When in doubt, the answer is always the queue. An improvement deferred to
morning costs a day; a regression merged at 3am costs the site.

**What may merge, and what may not.** Merging is deploying (ADR 23), so this
is the one place night work touches the live instance:

- **Anything a user can see does not merge at night.** Commandment 16 is
  explicit — Aaron walks UI changes in a browser before they land — so a
  night run takes those to a green PR and stops there, with the branch named
  in the daybreak queue and a note on where to look.
- **Everything else may merge when the required checks are green**: backend
  behaviour with no rendered change, tests, tooling, docs, the skill, the
  ledger. Watch the deploy to healthy afterwards; a red deploy is a daybreak
  item and a rollback (HOSTING has the runbook), never an overnight
  debugging session against the live instance.

**The daybreak queue** is `docs/polish/DAYBREAK.md`, and it exists because the
ledger is three thousand lines and nobody reads that at seven in the morning.
It is the one file Aaron opens. Rules that keep it worth opening:

- **One line per item, answerable without reading code**, and every item
  carries a **recommendation** so the whole file can be answered with "yes to
  all". A question with no proposed answer is homework, not a question.
- **Say what it costs to leave.** "Nothing until the next set" and "the volume
  fills in nine days" are different questions and should not look alike.
- **It is a queue, not a ledger.** An answered item leaves this file and its
  outcome goes into the ledger's own section. A file that only grows stops
  being read, which is how the queue it replaced failed.
- **Improvements only.** The queue holds things that would make the project
  better, never chores invented to have something to ask. If a night found
  nothing worth asking, the honest queue is empty and the summary says so.

**Stop conditions**, where a night run leaves the work rather than pushing
through: a required check red for a reason it cannot explain; anything
touching user data, accounts or secrets; a security finding (it goes to
daybreak *first*, before any fix, because a half-fix advertises the hole); a
gauntlet that will not go green after one honest attempt. Each is a daybreak
line and a move to the next item — never a retry loop, and never a workaround
invented at 4am.

**The morning summary** is what he actually reads: what landed and where,
what is waiting in a green PR for his eye, the queue's questions, and what
was deliberately not touched. Numbers with it, since a night run is a run
like any other and the ledger wants its measurements.

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

2. **Upkeep — put the decisions in front of Aaron, in one pass.** Group what
   needs him by the kind of answer wanted — a ruling, a dollar, a dependency,
   a migration window — each with the context to decide in one line, the
   consequence of leaving it, and a recommendation. **Ask them together**: six
   sessions each asking one question is how a queue becomes permanent; one
   asking twelve is how it empties.

   **Awake, this beat is a conversation — the only one in the pass.** Asleep,
   it is the daybreak queue instead, same content and same one-line-plus-
   recommendation discipline, written to `docs/polish/DAYBREAK.md` rather than
   spoken. Never hold a night run open waiting for an answer; write and move
   on. Either way commandment 1 is the basis: asking is never the wrong move.

3. **Discard to hand size — land what fits, and say what did not.** Take his
   answers — or, at night, take the items that never needed one — and
   implement under the same non-negotiables every other run obeys: every fix
   gets a test, the full gauntlet before pushing, surgical over structural.
   **The cap is real and it is the point** — a cleanup that
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

`/polish rainbow` is **seven solo runs, chained**: White, Blue, Black, Red,
Green, Colorless, Cleanup — one subagent each, serially, each an ordinary run
of the protocol below with its own branch and PR, and the next not starting
until the previous one has merged. It is the most thorough option and the most
expensive, so it is Aaron's explicit call; do not reach for it when converge or
a solo run would do.

The order is not arbitrary and neither is the serialism:

- **Serial, because parallel churns.** Concurrent branches all expand adjacent
  sections of `docs/polish/LEDGER.md` and conflict by construction, and
  several open PRs queue several `image` builds for no gain. Merging each
  before starting the next means the conflicts never exist rather than getting
  resolved.
- **WUBRG, because Magic says so and the dependencies agree**: White's
  licensing law binds Black's static-assets facet, Black's spend numbers feed
  Green's quota proposal, Red's external probe is Green's baseline. Every
  cross-color dependency points forward.
- **Colorless sixth, because it audits the other five** — including closing
  the loop Blue cannot: Blue runs second and generates most of its value last,
  since the colors after it produce the drift it hunts.
- **Cleanup seventh, because it lands what the six queued.** Six audits with
  no closing phase hand Aaron six queues; ending here turns them into
  decisions and landed work.

Orchestration:

- **Main working tree, not a worktree.** One color is live at a time so there
  is nothing to isolate from, and main already has `web/node_modules`, a warm
  Go build cache and the card pool — a fresh worktree rebuilds all three
  before it can run the gauntlet.
- Give each agent exactly one color, its reference file, the ledger, and the
  protocol and non-negotiables below.
- **Each lands its own branch and PR.** Never seven runs' diffs on one branch,
  which is a mass restructure by the back door. Each agent updates **only its
  own ledger section**; colorless may correct another's, which is its job.
- **Merge before advancing**, checks green. A green merge deploys itself
  (ADR 23), so a rainbow is up to seven deploys — one more reason schema
  migrations stay queued. At night, the Nightbound merge rule governs instead.
- A color with only queued findings and no safe fix opens **no PR**: carry its
  ledger text onto the next branch and move on.
- **You are the collector.** A subagent's report never reaches Aaron, so relay
  each as it lands, resolve overlap when a later color re-proposes an earlier
  one's fix, and close with one consolidated summary. Tag ledger sections
  `YYYY-MM-DD (rainbow)`.
- A rainbow may outlive a session. The merged PRs and the ledger are the
  resume point: read which colors already carry this rainbow's tag and pick up
  at the next.

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
   where the numbers go. Two rules, each bought with a wrong finding: **a
   large number is a question, not a datum** — anything slow gets profiled and
   the profile goes in the ledger, not just the millisecond — and **a probe
   finds *which*, only a profile finds *why***, so a sentence beginning
   "presumably" is not a finding.

   One standing question besides, whatever the color: **which of this facet's
   absolute claims is enforced by nothing?** "Always", "never", "every", "all"
   — in CLAUDE.md, in a package doc, in `web/README.md`, in this skill. For
   each, ask what fails if it stops being true; when the answer is *nothing*,
   it has already drifted or it will. This is the pass's most productive
   single question — it has found a dozen, most recently a 95% coverage floor
   that no gate had enforced since the Go crossing. **The fix is to make the
   claim machine-checked, never to reword it.**
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
4. **Fix.** Every bug fix gets a test, and **derive its expectation from the
   source of truth rather than restating the claim** — a test that repeats the
   sentence it checks cannot tell you the sentence is wrong, which is how a
   stale UI caption once sat for months beside a green assertion of itself.
   The working form reads the truth back: take the filename *off* the table
   rather than typing it, hold the dial's mode list equal to `ModeNames()` as
   a set. Then **verify by mutation, not by greenness**: break the thing,
   watch the test fail, restore it. Match the surrounding code's idiom and
   comment density; if a finding grows beyond surgical mid-fix, back it out to
   a queued item and say so.
5. **Verify.** The full local gauntlet before any push, from `go/` with the
   Mac's three exports set: `gofmt -l .` printing nothing, `go vet ./...`,
   `go test -race ./...`, `golangci-lint run ./...`; then
   `npm --prefix web run check`, rebuilding the committed bundle
   (`npm --prefix web run build`) if anything under `web/src` changed; and
   `cd tools && ruff check . && mypy && python -m pytest tests/ -q` when
   `tools/` moved. Confirm `data/app.db` was not dirtied by a running app with
   `ls -la data/` — `git status` is blind to it, and now doubly so.

   **For anything a user can see, drive the real surface**: a green jsdom test
   has not seen a layout, and a hover state has no jsdom at all. That includes
   authenticated flows on the deployed instance — the `claude` account is a
   plain user whose edits stay in its own decks, drivable through
   Claude-in-Chrome once Aaron has signed it in.

   **A security fix takes three extra beats**: (a) prove the bug with a
   *mutation-verified* test, so the fix demonstrably closes a real hole rather
   than decorating a safe one; (b) it is not done when the test passes but
   when **code scanning is also green** — CodeQL does not gate merging, but a
   correct guard its model refuses to recognise is a real cost, so budget for
   it; (c) after deploy, drive the live instance to confirm the hole is shut
   there and not only locally.
6. **Land.** Update the ledger on the branch, stage explicit paths (never
   `git add -A` — a hook refuses it, because `decks/` is live app data), open a
   PR, and watch the required checks — **read that list back from the API**
   rather than from a count remembered in a file, because it has grown twice
   with no prose noticing:
   `gh api repos/aasquier/sylvan-library/branches/main/protection --jq
   .required_status_checks.contexts`. A green merge deploys itself: seconds of
   downtime, and schema migrations apply on boot, which is why step 3 queues
   them. **At night the Nightbound merge rule governs** — nothing user-visible
   merges unwatched.
7. **Report.** What was fixed and where it landed; what is newly waiting on
   Aaron (the daybreak queue, if he is asleep); the numbers recorded and any
   trend that moved; which color the next run should be.

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
- **Never land anything at night that cannot be proven green here.** No
  merging a user-visible change unwatched, no schema migration, no "push it
  and see what CI says", no debugging the live instance at 4am. The daybreak
  queue is always available and always the right answer; a night that lands
  nothing and asks four good questions has done its job.

## Rigor is not uniform

The licensing facet gets **triple-check rigor**: Aaron named Claude the chief
legal officer of a project that exists at Wizards of the Coast's pleasure.
For anything touching assets or the Fan Content Policy, verify the primary
source (the recipe, the PROVENANCE entry, the licence text itself), not a
summary of it — and when genuinely uncertain whether something is compliant,
the answer is to ask Aaron and leave it out until then, never to ship it and
see. The full protocol is in `references/white.md`.
