---
name: polish
description: "The recurring quality pass over the sylvan-library codebase and infrastructure, organised as the five colors of Magic. Use whenever Aaron asks for a polish pass, quality pass, sweep, or audit — of Python or TypeScript best practices, testing, performance, security, licensing/free-use compliance, Claude API efficiency, CI/CD, alerting, cloud resources, browser/mobile compatibility, scalability, or the repo's Claude-facing docs. Also triggers on a color invocation (polish white/blue/black/red/green), on 'polish colorless' or 'polish all' (all five in one merged report), on 'polish rainbow' (all five as separate parallel agents), on 'run the polish pass', and on any 'are we doing X right?' question about one of those areas — even if the word polish never appears."
---

# The Polish Pass

A recurring, ledger-driven quality pass over this codebase and its
infrastructure. Aaron's fourteen quality facets are organised as the five
colors of Magic; one invocation runs **one color, deeply**. The point is not a
skim of everything — it is that over a cycle of five runs, every facet gets a
real audit, and the ledger makes the next run smarter than the last.

## Before anything else

Read `CLAUDE.md` in full if you have not this session. **The Commandments
outrank every checklist in this skill.** In particular: this is a
collaboration (ask Aaron rather than guess at design decisions), the project
is free forever and lawful about it, surgical trims over mass restructuring,
and CI is never a surprise. When a checklist item here conflicts with a
Commandment or an ADR, the checklist loses — and note the conflict in the
ledger so the checklist gets fixed.

## The five colors

| Color | Theme | Facets |
|---|---|---|
| **White** | Law & Protection | Free-use/licensing compliance (triple-checked); security & user isolation; testing discipline |
| **Blue** | Craft & Knowledge | Python best practices; TypeScript/React best practices; Claude-first docs & memory audit |
| **Black** | Ruthless Efficiency | Claude API spend; static assets over hotlinks; performance & efficiency |
| **Red** | Speed & Alarum | CI/CD pipeline; alerting & self-healing |
| **Green** | Growth & Resilience | Browser & mobile compatibility; cloud resource watch; scalability & user adaptability |

Each color has a reference file — `references/white.md` and so on — holding
the full checklist, the repo-specific anchors, and the known traps. Read the
one you are running, and only that one; the others are for their own runs.

## Choosing the color

- If Aaron named a color or a facet, run that color. A facet name maps
  through the table above ("security sweep" → White; "CI runtime" → Red).
- If Aaron named **two** concerns, or an ask that spans exactly two colors
  ("CI feels slow and the volume's filling up" → Red + Green), run **those
  two colors** deeply, in WUBRG order, in one session. This is the middle
  ground between one color and all five: still real depth per color, not a
  shallow survey. Say which two you picked and why. (More than two distinct
  concerns is a colorless run — see below.)
- If Aaron asked for **all**, everything, or the full cycle in one report —
  run **colorless**. If he asked for all five as **separate** runs — run
  **rainbow**. Both are below.
- Invoked bare, read the ledger and pick the color that most needs a run:
  staleness first (longest since last run), then urgency (a trend line moving
  the wrong way, or queued findings piling up, beats mere staleness). Ties
  break in WUBRG order. Say which color you picked and why before starting.

## Colorless — all five, one merged report

`/polish colorless` (also `/polish all`) runs every color in one session and
blends them into a single report. Colorless is the right word: the five
identities dissolve into one generic pass. Be honest about what that costs —
it is a **survey of the whole realm**, deliberately shallower per color than a
solo run, because five solo runs' depth does not fit one session and
pretending otherwise produces five skims labelled as audits.

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

Tag colorless runs in the ledger as e.g. `2026-08-16 (colorless)` — the tag
matters because a colorless run resets staleness only softly: a color whose
last touch was a survey is staler than its date suggests, and the next bare
`/polish` should weigh that.

## Rainbow — all five, five separate agents

`/polish rainbow` runs the full cycle at **full solo depth**, one subagent per
color, in parallel. Rainbow is the opposite of colorless: every color shines
separately and completely, no survey shortcut. It is the most thorough option
and the most expensive — five real audits' worth of tokens — so it is Aaron's
explicit call and his tokens; do not reach for it when colorless or a solo run
would do.

Orchestration:

- Give each agent exactly one color and its reference file, the ledger, and
  the same run protocol and non-negotiables below. Spawn them in worktrees so
  their fixes never collide.
- **Each color lands its own branch and PR**, kept independently reviewable —
  do not merge five colors' diffs into one branch, which would be a
  mass-restructure by the back door and unreviewable. A color with only
  queued findings and no safe fix opens no PR; it just reports.
- You are the collector: gather the five reports, resolve any cross-color
  overlap (two agents proposing the same fix — keep one), update all five
  ledger sections, and hand Aaron one consolidated summary plus the list of
  open PRs. Tag each ledger section `2026-08-16 (rainbow)`.
- Watch each PR's checks; remember CI is a shared resource, so five PRs at
  once means five image builds — stagger or expect the queue.

## The ledger

`docs/polish/LEDGER.md` is the pass's memory: per color, the date of the last
run, the PR that landed it, findings queued for Aaron, deferred items, and
measured numbers (CI runtimes, bundle size, volume usage, response times).
It is what makes the pass cumulative rather than repetitive.

- Read it at the start of every run. A queued finding is *not* re-litigated —
  it is waiting on Aaron, so leave it unless this run learned something that
  changes it. A deferred item is fair game if its trigger has arrived.
- Update it at the end of every run, **on the same branch as the work** —
  never as its own PR (the no-doc-only-PR rule).
- Record measurements even when they are fine. A healthy number today is the
  baseline that makes next quarter's regression visible.

## The run protocol

1. **Prepare.** Read the ledger and the color's reference file. Branch from
   `origin/main` (never work on main; never `git stash` on this repo — commit
   WIP instead).
2. **Audit.** Work the reference checklist against the current tree and, where
   it says so, the live instance. Measure rather than guess — the reference
   files say what to measure and the ledger is where numbers go.
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
   only CI can fully verify*. The example that taught this: declaring the
   package licence in `pyproject.toml` is a one-liner, but the correct form is
   setuptools-version-sensitive and the `image` build cannot run on this Mac,
   so a blind edit risks a red `image` job — "CI is never a surprise" makes
   that a **queued** item, not a safe fix, even though the diff is trivial.
   The test is not diff size; it is *can I fully prove this green before
   pushing?* If the only proof is CI itself, queue it (or land it alone on a
   branch Aaron can watch), never bundle it into a fix set.
4. **Fix.** Every bug fix gets a test. Match the surrounding code's idiom and
   comment density. If a finding grows beyond surgical mid-fix, stop, back it
   out to a queued item, and say so.
5. **Verify.** The full local gauntlet before any push: complete `pytest`
   (the determinism digest and `SIM_VERSION` move as a pair), `ruff check src
   tests`, `mypy`, and `npm --prefix web run check`; rebuild the committed
   bundle (`npm --prefix web run build`) if anything under `web/src` changed.
   Check `data/app.db` was not dirtied by the suite. For UI-visible changes,
   drive the real surface — a green jsdom test has not seen a layout. For a
   **security fix**, three extra beats, learned the hard way: (a) prove the
   bug with a *mutation-verified* test — revert the guard, watch the test
   fail, restore it — so the fix is demonstrably closing a real hole, not
   decorating a safe one; (b) the fix is not done when the test passes, it is
   done when **code scanning is also green** — CodeQL is a required gate and
   its model may not recognise a correct guard, so budget for that; (c) once
   merged and deployed, drive the *live instance* to confirm the hole is
   actually shut, not just the local tree.
6. **Land.** Update the ledger on the branch, stage explicit paths (never
   `git add -A` — `decks/` is live app data), open a PR, and watch the six
   checks. Remember a green merge deploys itself: a few seconds of downtime,
   and schema migrations apply on boot — which is why step 3 queues them.
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
