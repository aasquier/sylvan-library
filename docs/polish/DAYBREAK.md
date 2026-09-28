# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one line with
a recommendation, so the whole file can be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed.

Each item: what it is · what it costs to leave it · **the recommendation.**

> **2026-09-19, midday: the rainbow's Cleanup leg regrouped the queue.** Every
> item below was re-checked against the tree, the API and the instance this
> afternoon rather than inherited; the groups are ordered by what an answer
> costs you — a walk, a few clicks, a word, a dollar, a watched deploy, a
> migration window — and the last group needs nothing today. The 09-13
> Answered list that used to sit at the foot of this file is gone: all eleven
> records are in the ledger now (White, Green, Red-carried-by-Cleanup and
> Colorless; the Cleanup section's 2026-09-19 entry says where each went). The
> regrouping left **fourteen** items open — twelve inherited and two surfaced by
> the untap: Red's #481 walk line, which had lived only on its parked branch,
> and a merge-queue item whose ledger trigger fired that day with four PRs open
> at once.

**How many are open right now is a question for the file, not for this
paragraph.** Count them with the colour, never with the bold:

```
grep -cE '^\*\*(White|Blue|Black|Red|Green|Colorless):' docs/polish/DAYBREAK.md
```

A count written into prose is a claim that rots the next time anyone adds a
line — this one said "fourteen" through a week in which five colour lanes and
two design passes added and closed items — and so is the recipe for checking
it. Run the recipe; re-read the recipe.

---

## Open — a walk before a merge (commandment 16)

**Black: the mode prompts are repaired and the PR is green and parked — this is
the "nine of ten prompts have drifted" item, built.** Nine stale sentences
across seven modes; five modes that held between two and five tools and named
one; seven that now say what their own scope dial widens, through the
`ScopeNotes` field that exists for exactly that. The three intake modes stop
telling the model its brief carries counts, a curve and the other cards — it
carries none of them, and that was the one costing answer quality, because a
model told it already has a fact does not go and get it; they name `get_deck`
and `deck_stats` instead. Two prompts stop calling the gate by a language it has
never been. The slot argument stops naming three filters where four run, and
stops saying the deck file records only what its owner wrote (ADR 41);
`research` stops saying a rationale is composed nowhere. `deck-description`
stops assuming an import is asking. The theme interview learns there are two
corpora of true things, not one. The dossier is untouched on purpose — its
instructions and schema are hash-frozen in `testdata/dossier.json` — and its own
drift (`allies` missing from the search enumeration, in the prompt and again in
`dossierOpening`) is recorded for a branch of its own, since landing it moves a
frozen golden and invalidates every stored dossier. · *Cost of leaving it:*
every sentence fixed here is one a model is currently believing on the surfaces
a newcomer actually talks to, the tarot table among them. · **Recommendation:**
walk and merge. **Nothing renders differently and there is no page to click
through** — the walk is a *read*: `git show <pr> -- go/internal/claude/data/modes.json`
and read the seven changed prompts as sentences somebody is about to be told.
The two questions left unanswered are the line below, under *a ruling*. Then,
once merged, the cheapest place to see it working on the deployed site is the
**deck description** — any deck → Description → ask: one call, one paragraph,
about ten seconds. The interview is the richer read and costs six questions to
get through. Ledger: Black, 2026-09-26 (rainbow, prompts).

**Red: PR (the Queen's ball) is green and parked — every plate in the app now
carries a light that walks its rim.** Aaron asked for "a gleaming edge that
circles around the button, blood, fire, whatever is appropriate"; one block in
`index.css` gives the whole `.btn` family a masked conic arc driven by a
registered `@property`, with a material per voice, and the fortune-teller's
primary finally leaves the chart blue for the table's brass. · *Cost of
leaving it:* the controls stay the thing Aaron called basic, and the séance
blue keeps outliving two PRs' worth of suggestions. · **Recommendation:** walk
and merge. `mtglab-ui` on 8765 (no dev server needed — the bundle is in the
diff): `/` — watch the green *Import a decklist* for one **7.5s** lap, hover
it (the light brightens and the lap drops to **2.6s**), press it (it settles);
Tab to it and the vine ring sits outside the gleam. Then `/new` → **Help me
decide** → the fortune-teller → the table: *Turn them over* is brass now, not
blue, and *Shuffle again* beside it lights only when you reach for it. Then
any deck → the folded Wheel at the foot → unfold → **Spin the wheel** wears
the ember, at **6s** a lap, the loudest in the app. Both themes. Ledger: Red,
2026-09-26 (rainbow).

**Red: `reducedmotion_test.go` could not see an animation added to a `.btn`
pseudo-element — CLOSED, and it found a live one on its way out.** The rule is
now `class::pseudo` rather than `class`, with `display: none` still covering
both pseudo-elements; the mutation that stayed green this morning (guard
deleted, bundle rebuilt) now fails by name. The first thing it caught was real:
`.entombing::after`, the black wash that closes over a card being entombed,
ran its full 340ms under reduced motion because the guard beside it only named
`.entombing`. · *Cost of leaving it:* none now. · **Recommendation:** nothing
to rule on; it rides `the-second-ball`. Ledger: Green, 2026-09-26 (rainbow).

**Green: the phone's 44px floor is BUILT and wants a walk — and one number in
it is Aaron's to rule on.** PR `the-second-ball` (stacked on
`the-gleaming-edge`) puts every control family behind `@media (pointer:
coarse)` with a `min-height: 44px`, and gives the wordmark a pseudo-element
halo instead because a signature may not be resized. Measured on the committed
bundle at 375×812: `/import` went from **23 of 25 controls under the floor to
3**, `/` from 20 of 21. The shape was settled by the stylesheet rather than by
taste — `.btn` is `overflow: hidden`, so a halo on it is a halo that is not
there. · *The number to rule on:* the phone's header goes **201px → 265px**
(24.8% → **32.6%** of a 375×812 screen), because ten nav entries wrap onto four
rows and every row grew 12px. It furls on the first scroll, so the cost is
bounded. · **Recommendation:** walk it and merge. If 265px is too much, the
answer is **fewer nav entries on a phone, not smaller targets** — say the word
and that becomes its own slice. `mtglab-ui` on 8765, phone width, `/` and
`/import`: the nav rows, the filter selects and *Import a decklist* are all
44 tall; the wordmark is unchanged to the eye and 44 to the thumb. Ledger:
Green, 2026-09-26 (rainbow).

**Green: the 404 is a room now, and the way out of it wants a sweep behind
it.** `the-second-ball` gives the wrong-turn page *Misleading Signpost* (Wilds
of Eldraine Commander #47, the extended-art printing, art by Julian Kok Joon
Wen), hotlinked and credited in the same room, in the empty library's own
three-layer treatment. Its link left `--series-1` for a new `.prose-link` in
`index.css`. · *Cost of leaving the rest:* `grep -rn "color: 'var(--series-1)'"
web/src` counts **12** inline on 2026-09-26 — a chart's series colour inking
doors and emphasis across five files, where a `:hover` can never reach it. ·
**Recommendation:** a Red slice points the eleven survivors at `.prose-link`
and records any it deliberately leaves, with the reason. Ledger: Red,
2026-09-26 (rainbow).

**Green: the reading room takes the room's light, and a keyboard can finally
reach a turned card.** `the-third-ball` (stacked on `the-second-ball`) lays the
candlelight on the cards as a layer of its own — commandment 19's shape, never
a filter — gives the flip the specular a piece of cardstock throws as it turns,
puts the three position names back on the cloth under the face-down cards (the
stylesheet had been claiming that since 2026-08-18 and the page stopped doing
it), and gives the turned card a tab stop so the `:focus-visible` rules it has
always carried stop being dead code. · *Cost of leaving it:* those rules could
never fire, so a keyboard at a desk lost the zoom, the card's name and the
**artist credit** on a Magic crossover — which Scryfall's guidelines ask be
findable "somehow". · **Recommendation:** walk it and merge; it is the room
commandment 15 rations last. `mtglab-ui` on 8765, `/new` → Help me decide → the
table. Turn one card at a time and watch the light cross it (**760ms**, the
flip's own); the other two keep their printed names until their own card turns.
Then Tab to a turned card. Ledger: Green, 2026-09-26 (rainbow).

**Green: the reading room's Magic crossovers were outside commandment 19's
gate, and are inside it now.** `cardimagery_test.go`'s `artBearing` knew only
`seance-vision` — the card *reflected* in the crystal ball — while the cards on
the table themselves render `cards.scryfall.io/art_crop` URLs through
`.tarot-rws-art`. Seven classes added on `the-third-ball`, mutation-verified. ·
*Cost of leaving it:* index.css records that the first cut of that surface put
"a desaturating filter, a cream screen and a honey wash" over exactly that
element; it came off for being ugly, and nothing would have said it was
forbidden. · **Recommendation:** nothing to decide — noted so the next sweep
knows the covered list is kept by walking JSX, not by waiting for a violation.
Worth one Colorless slice asking which *other* rooms render card art with no
entry. Ledger: Green, 2026-09-26 (rainbow).

**Red: the card a hover holds up wears its own colours now, the row it came
out of answers a hand, and the 99's thirteen shelves finally say what they
mean — all three want your eye.** PR (the fifth ball) is green and parked
against `main`. Door **8765** (`mtglab-ui`, the committed bundle), any deck →
**The 99**, unfold a group. Hover a card's name: the preview is framed in that
card's **colour identity** — a mono-red card rimmed in ember, a two-colour card
in gold, an artifact in steel — and the light walks the frame in **3.4s** a lap
(measured 3.41). Hover anywhere else on the row: it washes faintly green and a
quiet vine light runs its rim, **4.5s** a lap (measured 4.52). Then **Tab**
into the list — a keyboard could not land on a single card in a deck before
this branch — and press **Enter** to hold one up. Both themes, and narrow to a
phone: hover does not exist there, the shelf marks open on a tap. · *Cost of
leaving it:* commandment 16 — merging deploys, and all of this is a user
surface. · **Recommendation:** walk it, then merge. The one taste call that is
yours is the wash: 7% of the vine over the surface, which is near the floor of
what is visible, because "subtly" was the word in the ask. Ledger: Red and
Blue, 2026-09-26 (rainbow).

**Red: a card thumbnail is reachable by a keyboard in exactly one of the
twenty-odd places `CardHover` is used, and the other nineteen are a sweep.**
`CardHover` grew an opt-in `reachable` on the fifth ball — a tab stop, a focus
ring and Enter to hold the card up — and it is set on the deck page's 99 only.
The rest were left alone deliberately and the reason is on the prop: four pass
`tapOpens={false}` because the child is already a control, and at least one
wraps a `display: contents` tile with no box for a ring to be drawn around, so
each site is a judgement rather than a find-and-replace. · *Cost of leaving
it:* card search, the swap board, the combo panel and the token shelf still
answer a mouse and a thumb and not a keyboard — the same gap the 99 had, on
smaller surfaces. · **Recommendation:** one Green slice, by surface, recording
which sites were examined and deliberately left inline **with the reason** —
that last part is what stops it being rediscovered every cycle. Ledger: Red,
2026-09-26 (rainbow).

## Open — a few clicks in the repository settings

**White: the nine open torch Dependabot alerts are triaged in prose and
never dismissed on GitHub, so the security tab re-asks a settled question
forever.** The triage lives in `tools/pyproject.toml` (containment: dev-Mac
only, never ships, safetensors-only snapshot — pinned by real code since
#463). Re-read from the API 2026-09-19: still exactly 9 (1 critical, 3
medium, 5 low), all `pip/torch`, all `development` scope. They stay open
because dismissing needs repo-admin, which the pass does not have and should
not. · *Cost of leaving it:* every future security read spends the hour
re-deriving this paragraph. · **Recommendation:** dismiss all nine as
"tolerable risk — see tools/pyproject.toml's depth-extra triage" (two
minutes in the Security tab). Ledger: White, 2026-09-12; carried twice by
Cleanup (09-12, 09-19) for the same stated reason.

## Open — a ruling, one word each

**Green: the land-count ruling is BUILT AND GREEN in PR #506, parked for your
word — and it is one word, on a question slightly wider than this line used to
ask.** `land_count` counted the category a card was filed under, so a modal
DFC like Stump Stomp // Burnwillow Clearing filed under 'interaction' was a
land to the curve and a spell to the count, and the opening-hand odds were one
land short. #506 makes the land count the exact complement of the curve's
skip. **The widening:** that necessarily also counts a *plain* land filed
under a spell slot, which is what moves the one golden — and #506's body
offers the narrow "modal DFCs only" variant if you want it instead. · *Cost of
leaving it:* two decks read a land low (`one-blade-many-blessings` by one,
`school-of-hard-knocks` by two, re-confirmed off the volume today); the other
23 are unaffected either way. · **What it costs to say yes:** eleven numbers
in one frozen golden, every one listed old → new in the PR, all consequences
of `messy`'s `land_count` going 96 → 97; no fingerprinted package and so no
Tier 1 cache discarded. · **Recommendation:** read §3 and §4 of #506 and
answer **"union"** (merge it) or **"MDFC only"** (and no golden moves at all).
Ledger: Green, 2026-09-26.

**Green: nothing reports how old the card pool is, and this is the fourth run
to say so.** `pool_stale` asks whether the pool predates the *columns* the app
reads, so it answers `false` for a pool of any age; the only age signal in the
product is a date inside a bulk filename that a person has to go and read.
Today: 13 days old, `pool_stale: false`. · *Cost of leaving it:* legality and
prices answer out of date silently, and the one number that would have said so
is the one nothing reports. · **Recommendation:** "yes" — add `pool_age_days`
to the health body beside the `disk_free_mb` that #502 adds, as a small
follow-on once #502 lands. Deliberately not built today: two lanes editing
`health.go` on one afternoon is a merge conflict for nothing. Ledger: Green,
2026-09-26.

**Green: `goreclaw-stompy` was the one deck in the checkout's `decks/` that
existed nowhere else, and it is deleted now.** It survives in git history
(`git show 5515f5f^:decks/goreclaw-stompy/deck.yaml`, the last commit before
ADR 30 moved the library out) plus four theme words recorded in the ledger. ·
*Cost of leaving it:* nothing — nothing served it. · **Recommendation:** if
you still want it, paste that file through the site's import page and the
library owns it; if not, say "obituary" and the mtg-lab skill's trigger list
drops "mono-green/Goreclaw" (which the instance contradicts) on the next Blue
run. Ledger: Green, 2026-09-19.

**Green: touch targets under 44px, app-wide — 50 controls on `/` at 1440
this week, 21 of 23 on `/import` at phone width when first measured.** Nav
links 32px, "Entomb" 66×28, the filter selects 36px. · *Cost of leaving it:*
a thumb misses what a pointer never does, and commandment 2's newcomer is the
one most likely to arrive on a phone. · **Recommendation:** rule on the shape
— a spacing-scale change (every control grows) or a pseudo-element hit area
that moves no pixels — or close it as a desktop-first ruling; either answer
ends a finding re-measured four runs running. Ledger: Green, 2026-08-24 (the
browser facet's queued item 3); re-measured 2026-09-19.

**Red: the "drill older than the newest migration" rule cannot be satisfied,
and the drill that proved it is walked and recorded.** Snapshot retention is
five days; a snapshot can only ever rehearse a rung landed in the last five
days, and `docs/HOSTING.md` §Backups already says so. · *Cost of leaving it:*
nothing — the wording is landed; this is the one residual question. ·
**Recommendation:** "close" — HOSTING's sentence is the rule, and a second
copy on a merge checklist is the kind of duplicate this file exists to
refuse; say "checklist" instead if you want the five-day drill named beside
"land schema changes when you can watch them" in CLAUDE.md. Ledger: Cleanup,
2026-09-19 (the drill and the retention finding are recorded there as Red
records carried by Cleanup).

**White: the coverage floor's 1.6-point gap — should it track the tree, or stay
anchored at 95?** You asked on 2026-09-24 for "the tree at 96 and the gate at
95" and `ci.yml` records that as a deliberate margin, so a diff can cost a few
tenths of honest refactoring without going red. The tree now measures 96.7 and
both readings of your ruling are defensible: hold 95.0 as a number, or keep the
~1.6 gap as the tree climbs. · *Cost of leaving it:* nothing today — the gate
works either way; it only matters the day the tree passes 97 and nobody knows
whether a click is owed. · **Recommendation:** "margin" — leave 95.0 alone
until the tree reaches 97.0, then click to 95.5 and keep the gap, because what
you asked for was slack for refactoring and slack does not shrink as a
codebase grows. Say "anchor" if you meant 95 as a floor the tree simply grows
away from. Ledger: White, 2026-09-26.

**White: two named coverage levers should be closed as argued rather than left
open — `ApplyBulk`'s four fold branches and `prompt.secret`'s terminal arm.**
Both were worked this run and both are honestly unreachable: `PlanBulk` refuses
the same file through the same lookup, so `ApplyBulk`'s folds are defensive
depth behind a guard and reaching them needs a `BulkPlan` the planner would
never build; and the password prompt's terminal branch would need its terminal
read handed in as a value, which moves the five uncovered statements into the
seam's own default rather than removing them. · *Cost of leaving it:* one hour
per future coverage leg, spent rediscovering this — `COVERAGE.md` currently
advertises the password one as "the single biggest lever left in `cmd/mtglab`".
· **Recommendation:** "yes to both" — move both into `COVERAGE.md`'s *Left
deliberately* with the reasons above, so the list keeps its promise of being
the thing you read before spending an hour on a branch that cannot be entered
honestly. Ledger: White, 2026-09-26, queued items 2 and 3.

**White: the determinism replay is owed** — tarot, brew, wheel and Tier 1
against the live instance, which this run could not do (a worktree lane with no
browser; the pane belonged to another lane). · *Cost of leaving it:* the
once-a-cycle check that a seed is still a promise where users live slips a
week; the local goldens are unaffected and green. · **Recommendation:** no
ruling wanted — it is work, and the next White or Green run with browser access
takes it, using 09-19's recorded baselines (tarot seed 1909 → 741 bytes, sha256
`e406f504…`; brew → 503 bytes, sha256 `54c5036e…`; Tier 1 in the two-ask
cached form). Ledger: White, 2026-09-26.

**Black: the two prompt sentences the "nine of ten prompts" item said to put to
you rather than guess — and in both, the *code* may be the wrong side.** The
rest of that item is built and parked (see *a walk before a merge*); these two
are left exactly as they are, one word each. · **(a) The interview asks for
"three to five questions" and `MaxQuestions = 6` truncates the answer.** Note
where each lives: the ask is in `interviewOpening` (`interview.go:533`), not the
prompt, and the cap is a constant whose comment says "more than this and it
stops being an interview and starts being a wall". So a model that follows the
ask perfectly is never cut, and a sixth question is kept while a seventh
vanishes silently. *Cost of leaving it:* nothing visible — a seam nobody has
tripped. **Recommendation:** say **"six"** and make the ask read "up to six" —
the cap is the number a person actually feels, six is not a wall, and a prompt
whose number *is* the cap cannot drift from it. "five" is the other consistent
answer and brings the constant down instead. · **(b) The theme interview's
schema tells the model a re-stated slot set "replaces the previous set rather
than adding to it", and `Carry` unions.** The code is deliberately the newer
side: `Carry`'s own comment says the replace rule "is a rule enforced by
nothing, and it drifts", and records what it cost — driven with the short
answers a first-timer actually gives, the readiness count went 0, 1, 0, 1, 0, so
somebody answering honestly watched a reading that never became ready. That is
commandment 2's failure exactly. *Cost of leaving it:* the model is told its
omission erases an earlier reading when it does not, so it re-states
defensively and spends output tokens proving what the code already guarantees.
**Recommendation:** say **"unions"** — the sentence becomes "everything you have
learned since your last answer; anything you said before is remembered whether
you repeat it or not" — since both halves still pass `Ground` against the same
transcript, so nothing is carried that the person did not say. Ledger: Black,
2026-09-26 (rainbow, prompts).

**Colorless: this file's own rule says "one line per item" and not one item has
obeyed it for a month — is the rule wrong, or is the file?** Every item here is
a paragraph, and you have answered the file happily in that shape (25 in one
morning on 09-05). A line-count gate would fail the file you blessed, and
rewording a rule to match drift is the move the pass forbids, so nobody has
touched either side. This question was raised on 2026-09-19 and sent to "the
report" instead of here, which is why you are seeing it a week late. · *Cost of
leaving it:* nothing today; the risk is the file growing into the
three-thousand-line thing the queue exists to replace, with no rule anyone can
point at. · **Recommendation:** "headline" — the rule means *one item per
question, headline first, context after*, which is what the good items already
do; say the word and the sentence gets rewritten once and then held by the
shape of every new item. Ledger: Colorless, 2026-09-19, and 2026-09-26 for why
it is arriving now.

**Colorless: the guard on this file reads one direction only, and the missing
direction is how five items hid for 26 days.**
`daybreakrecord_test.go` proves every line here names a ledger record; nothing
proves every *waiting* ledger record has a line, because "queued" in ledger
prose carries no marker a test can read — and an answered item correctly
*leaves* this file, so a naive reverse check would demand a line for every
historical block ever written. The missing piece is one convention: a marker on
a queued ledger block that says *still open*. A convention binds every future
session, which makes it yours rather than a run's. · *Cost of leaving it:* the
breach recurs in the only direction nothing watches — it has happened twice
that we know of, most recently to this very question. · **Recommendation:**
"marker" — queued blocks that are still open get a literal `(open)` after the
heading, struck when you answer, and the next Colorless run extends the guard
to read both ways; say "no" and the reverse walk stays a `git log -S` recipe in
`references/colorless.md`, which is honest but only runs when somebody
remembers. Ledger: Colorless, 2026-09-19, re-filed 2026-09-26.

**White (leg two): the ratchet trigger you named has arrived — does the floor
click?** The grind took the tree 96.6 → **96.9**, and the day's first leg (PR
#504) is another tenth, so merged `main` should print **97.0** on the arm64
leg. 97.0 is the exact number leg one's own recommendation set as the
condition: *hold 95.0 until the tree reaches 97.0, then click to 95.5 and keep
the gap.* · *Cost of leaving it:* nothing this week; the gap grows to two full
points, and at two points a diff can lose a whole point of honest coverage
without a check going red — which is the one thing the floor exists to
notice. · **Recommendation:** **"ratchet"** — 95.5, and only once a merged
`main` actually prints 97.0 or better in the `Coverage floor` step (read it off
the run, not off this file). The floor did not move on this branch on purpose:
`ci.yml` records the gap as your own 2026-09-24 ruling, and a lane does not
overwrite a ruling with its own arithmetic. Ledger: White, 2026-09-26
(rainbow, leg two).

**White (leg two): the next grind is a fixture, not a sweep.** What is left
after this is `internal/pool` (58 statements), `internal/sim/tier3` (67) and
`cmd/mtglab` (46), and the pool's share wants **a faulty DuckDB connector** —
`authtest.OpenFaulty` is SQLite-only, and `rebuild.go`'s `startRebuild` and
`finish` are the same "a statement failed in the middle of a long write" shape
this leg closed everywhere else. · *Cost of leaving it:* the refresh's own
failure arms stay undriven, and the refresh is the one job that can leave the
card pool half-written on the deployed volume. · **Recommendation:** **"go"** —
a `pooltest.OpenFaulty` built the same way (borrow the registered driver off a
throwaway handle, wrap the three context forms, count statements) is a day's
work and reusable, and it is the last named fixture gap in the tree. Ledger:
White, 2026-09-26 (rainbow, leg two).

## Open — a dollar and an account

**Red: nothing off-platform tells you the site is down, and a hung process
that keeps its port is an outage nothing detects.** Fly's HTTP check stops
routing on failure and the restart policy fires only on exit, so a wedged
process is a total outage with no alarm. · *Cost of leaving it:* the first
person to notice an outage is a friend at breakfast. · **The two free minutes
are spent and they end at your login:** `fly` has no alert-rule subcommand at
all, fly-metrics.net answers 401 unauthenticated, and `FLY_METRICS_TOKEN` is a
read-only Prometheus credential rather than a Grafana one — so *whether any
alert rule exists* is two clicks in your own Fly session and nothing a run can
find out. (`fly synthetics` is new and is not the answer: the agent runs on
Fly, so it cannot report that Fly is down.) · **Recommendation:** look once
while you are in there, then UptimeRobot's free tier on `GET /api/health`
(GET, never HEAD: `HEAD /` answers 405, and a Go guard asserts the GET now)
wired to Pushover ($5 once) for the phone. The health body reports `app_db`,
`disk_free_mb` and `schema_version` as of 2026-09-26, so the monitor has
something to read besides 200. Ledger: Red, the queued list carried in the
2026-09-05 entry, items 1–2; answered as far as possible 2026-09-26.

## Open — a watched deploy

**Red: a deploy takes no snapshot, and the boot after a merge is the moment
the volume is most at risk.** Fly snapshots daily on its own clock; the
ladder is forward-only and applies unwatched, and `deploy.yml` still has no
snapshot step. · *Cost of leaving it:* the one deploy that needs a rollback
point is the one guaranteed not to have a fresh one — and the 09-13 drill
showed a snapshot can only ever rehearse a rung landed in the last five days.
· **Recommendation:** a `fly volumes snapshots create` step ahead of `flyctl
deploy` in the deploy job, non-fatal on failure, once the deploy token's
scope is checked — a workflow change only CI can prove, so it lands as its
own PR on a morning you can watch the deploy. Ledger: Red, the queued list
carried in the 2026-09-05 entry, item 6.

**Black: every visit to the deck shelf re-reads and re-parses the whole
library — ~42 ms of CPU and 27 MB of allocation for 25 decks of 100 cards, on
a machine with two shared cores.** `/api/decks` builds a fresh `FileSource`
per request, so nothing memoises anything; ~90% of the cost is inside the YAML
library, so the only lever that pays where it ships is not parsing a file that
has not changed (fanning the reads out with `convoke` was measured and
rejected — its worker rule runs the serial loop on two cores). · *Cost of
leaving it:* the home page's shelf call spends 42 ms of server CPU per visit
for an answer that was identical last time, forever. · **What makes it a
question:** the memo needs an owner that outlives a request — the long-lived
`*API`, handed down through `Resolver` into `NewFileSource`, five to eight
files — and its invalidation is the pool's file-stamp guarantee applied to
live user data. · **Recommendation:** "yes" — its own PR on a morning you can
watch the deploy, with hit and miss counters in from the start and rendered
nowhere. Ledger: Black, 2026-09-26.

## Open — a migration window

**Black: prompt-cache *writes* are invisible in both usage ledgers, so the
spend figures are a little under.** `cache_creation_input_tokens` appears
nowhere outside two test fixtures; writes bill at 1.25× input. · *Cost of
leaving it:* the dollar figure on the Admin panel and in `mtglab claude
usage` under-reads by the write premium, and 09-26 put a floor under it
rather than calling it small: every mode's cacheable prefix was measured, and
the instance's 213 conversations wrote **at least ≈490,000 cache tokens**,
billing at 1.25× input — **$1.22–$1.84 unrecorded against a recorded $9.0034,
a 14–20% under-read** — with a provable ceiling of $24–37 because writes can
never exceed reads. A 20×-wide bracket is what the column collapses. ·
**Recommendation:** add the
column on a day you can watch the boot (a migration is your window by
standing rule); nothing until then. The theme mode's unreadable second cache
breakpoint (Black, 2026-08-24, item 2 — worth at most ~0.8% of that mode's
input) waits on this column as its instrument and is not a question of its
own. Ledger: Black, 2026-08-24 (the carried list); re-checked 2026-09-19.

## Open — deliberately waiting, nothing to do yet

**Blue: the Settings room says "the torches are not lit yet", and the only
thing keeping that true is that you have not flipped the switch.** The line
is hand-written into the bundle (`web/src/routes/Settings.tsx`) and true
today — `fly secrets list` still shows no `MTGLAB_NIGHT_WINDOW` — but the
evening you set the five night secrets changes no code and rebuilds nothing,
so the room would keep telling people the arena is dark while it fights. ·
*Cost of leaving it:* a small untruth on the one page where a person decides
to enter their decks. · **What would have to be true:** the Coliseum's night
shelf lands (ADR 46 names it as its own PR) and the settings room reads
whether a night is scheduled off the wire. · **Recommendation:** unchanged —
the copy becomes a fact the server owns when the shelf gives it something to
read. This line is the reminder. Ledger: Blue, 2026-09-05.

**Green: the pool is thirteen days old and still fine; the next refresh has a
date rather than a deadline.** *Reality Fracture* (`fra`, 249 cards, plus the
`frc` commander decks) releases 2026-10-02, and until a refresh runs after that
day the shelves cannot resolve a released product. Re-read 2026-09-26: bulk
files still `2026-09-13`, 35,517 oracle / 108,583 printings, byte-identical to
09-19 — the premise has not moved, only the age, and the trigger is the release
rather than the age. · *Cost of leaving it:* nothing until 10-02, then names
from a new set fail on import and search. ·
**Recommendation:** "Gather the library again" on the Admin Upkeep tab in the
week of 10-05 — a deployed button now, no ssh — then read the pool file's
size back once; the #472 rebuild took it 224 MB → 81 MB on 09-13 and it should
hold near there. Ledger: Green, 2026-09-19.

**Black: the cache-read price is one constant for the whole family, and the
family stopped agreeing — but nothing is mispriced yet.** `prices.CacheReadFraction`
is 0.1 for every model on the argument that the ratio is the same across the
family; Claude Fable 5.1 prices cache reads at $0.25/MTok, which is 0.025× its
input. `claude-fable-5-1` is not in `Table`, and the instance runs Sonnet 5 on
every row, so today this is a trigger rather than an error. · *Cost of leaving
it:* nothing until a model with a different read fraction is added to `Table`,
at which point its cache reads are priced 4× high, silently. · **What would
have to be true:** the fraction moves onto `Priced` beside the rate, which
means extending `testdata/prices.json` — a frozen golden, and not a thing a
polish run extends on its own. · **Recommendation:** land it in the same branch
that adds such a model; meanwhile the cheap half is a guard that every model in
`Table` is on a recorded list of "cache reads really are a tenth here", so the
next one has to say. Ledger: Black, 2026-09-26 (deferred 2026-09-19).

**White: `NOTICE.md` is held and the skills are held; the rest of the tree's
prose is still unguarded.** `licenserecord_test.go` holds every repository
path and `mtglab` verb the licensing record names, `skillrecord_test.go`
(#440) holds `.claude/skills/`, and #469 moved the shared kit to
`recordkit_test.go`. What none of them reads is `docs/`, `web/README.md` and
the package comments — and the 09-13 morning found two rot instances there
(`docs/HOSTING.md` contradicting itself about `auto_stop_machines`;
`CLAUDE.md`'s retired invalid-deck fact), both fixed, neither caught by
anything but a person reading. · *Cost of leaving it:* nothing legal; this is
tidiness with a mechanism. · **What would have to be true:** somebody decides
the wider prose is worth a third extractor — the kit exists now, so it would
be reused rather than rewritten. · **Recommendation:** one more cycle of
measured rot and it stops being tidiness; until then this line is the
reminder. Ledger: White, 2026-08-24; narrowed Cleanup, 2026-09-05.
