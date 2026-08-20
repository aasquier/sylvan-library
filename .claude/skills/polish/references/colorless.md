# Colorless — The Artifacts

One facet with four parts, and a single question underneath all of them:
**is the pass itself still working?**

Colorless is the sixth run and it goes last, after White, Blue, Black, Red and
Green — which is how Magic sorts a collection and also the only order that
makes sense here, because this run audits what the other five just did. An
artifact in Magic belongs to no colour and works for whoever holds it; the
tooling, the ledger and this skill are exactly that. They serve every colour
and no colour owns them, so without a run of their own they are the thing
nobody ever sweeps.

It is also the cheapest run in the cycle by a distance. Nothing here needs the
live instance, a card pool or a network — it is reading, arithmetic, and one
or two tool invocations — so a rainbow that has run long is still worth
finishing.

## Part one — did last cycle's findings actually land?

The ledger's whole claim is that the pass is cumulative. This is where that
claim gets checked, and it is the part most likely to find something, because
nothing else in the skill ever re-reads an old entry.

- Walk every **queued for Aaron** item, oldest first. For each: has Aaron
  ruled? Did the ruling get acted on? Is the item still true — or has the code
  moved under it, so the question no longer means what it meant? A queued
  finding that has gone stale is worse than an open one, because it is
  occupying the queue *and* misdescribing the tree. Re-word it, close it, or
  say plainly that it is still waiting.
- Walk every **deferred** item and check its **trigger**, which is the field
  that makes deferral honest. A trigger that has arrived and gone unnoticed is
  a finding. A deferred item with no trigger recorded at all is a bug in the
  entry — give it one or promote it.
- Walk the **fixes** each colour claimed. Spot-check two or three against the
  tree: is the thing still fixed, and is the guard that holds it still there?
  This is where a fix quietly reverted by a later merge shows up.
- **Corrections outrank overwrites.** When a later run finds an earlier entry
  wrong, the ledger records the correction beside the original rather than
  editing it away — the Black section's 2026-08-19 entry is the model, keeping
  two explicit corrections to its own earlier claims. If a run has silently
  rewritten history, that is a finding.

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

The developer shelf is artifacts in the plainest sense: `mtglab bench`,
`mtglab mutate`, the cache register in `caches.py`, `mtglab animist verify`.
Nothing else in the cycle owns them, and a measuring tool that has gone wrong
is worse than no tool, because its numbers are believed.

- **Run each one and read the output as evidence about the tool**, not only
  about the code:

  ```bash
  mtglab bench run                 # does the suite still resolve its targets?
  mtglab bench caches              # is anything registered but dead?
  mtglab mutate list               # has the catalogue grown or shrunk, and why?
  mtglab mutate run --only <site>  # are the ledger's survivors still alive?
  mtglab animist verify
  ```

  A fresh seeded *draw* is White's job and re-drawing one here only adds a
  sample nobody asked for. The colorless questions are the ones a draw cannot
  answer: did the catalogue move, and are the survivors on record still there.

- **A skipped row is the finding.** `bench` reports unavailable targets by
  name for exactly this reason: a suite that quietly shrank still prints a
  table. If a target that used to run now says `skipped`, find out why before
  reading anything else on the page.
- **Check the tool against a bug it is supposed to catch.** The instruments
  here were each built from a specific failure, and the honest test is to
  reproduce that failure and watch the needle move — `tests/test_bench.py`
  does this for the import storm by removing #181's sentinel. A tool nobody
  has re-validated is a tool on trust.
- **Mutation kill rate is a trend, not a grade.** Record the sample size, the
  seed and the rate. A rate that moved needs its cause named: new tests, new
  code, or a different draw. Survivors carried forward unread across two runs
  are a finding about the pass, not about the suite.
- Ask what the shelf is still missing. The standing queued item is `mutmut` or
  `cosmic-ray` for an exhaustive run over one module, where the in-repo
  harness does sampling; anything else proposed here is a new dependency and
  therefore queued with the arithmetic, never adopted in the run.

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

## What this run never does

- **It does not re-audit the five colours.** If it finds a real bug in code,
  it records it in that colour's section as a finding for that colour's next
  run — this run's diff belongs to the skill, the ledger and the tooling.
  A colourless run that starts fixing Python has stopped being colourless.
- It does not open a PR for the ledger alone. Same rule as every other run:
  a doc change rides the branch that does the work. If the only output is
  ledger text and skill edits, that *is* the work and the PR is legitimate —
  this facet is the second place, after Blue's docs audit, where a mostly-prose
  PR is honest.
