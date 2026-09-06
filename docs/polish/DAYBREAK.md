# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one line with
a recommendation, so the whole file can be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed.

Each item: what it is · what it costs to leave it · **the recommendation.**

---

## Open — 2026-09-05

**White's morning items 1 and 2 both resolved overnight by Blue — nothing
left to decide, one thing still owed.** The settle-test flake (item 2) is
fixed the way item 2 asked — an event the test awaits, never a deadline —
merged as **#431** and green on four straight CI legs; White's **#430 then
merged clean** and its deploy was walked (the `/PROVENANCE.md` marker flipped
from the SPA shell to text, health 200, the door renders, the shelves still
401 pre-auth). Both findings and fixes are recorded in the ledger (White and
Blue, 2026-09-05). · *Still owed, unchanged:* the once-per-cycle determinism
replay against the deployed instance — tarot and wheel answer 401 pre-auth
and Claude never signs in. · **Recommendation:** sign the `claude` seat in
through Claude-in-Chrome some evening and the next White run rides it; the
same seat unlocks Blue's owed walks (the fortune-teller's table and the
`/claude` keeper duty, skipped two runs straight for the same 401).

**Blue: the Settings room says "the torches are not lit yet", and the only
thing keeping that true is that you have not flipped the switch.** The line
is hand-written into the bundle (`web/src/routes/Settings.tsx`) and true
today — the instance has no `MTGLAB_NIGHT_WINDOW` — but the evening you set
the five night secrets changes no code and rebuilds nothing, so the room
would keep telling people the arena is dark while it fights, and nothing
fails when it starts lying. · *Cost of leaving it:* a small untruth on the
one page where a person decides to enter their decks, starting the first
scheduled night. · **Recommendation:** when the Coliseum's night shelf lands
(ADR 46 names it as its own PR), have the settings room read whether a night
is scheduled off the wire and render either the unlit-torches line or the
real window — the copy becomes a fact the server owns instead of a promise
the bundle froze. Nothing to do before then; this line is the reminder.
Ledger: Blue, 2026-09-05.

**Black: the Admin panel now prices last month's tokens at this month's
rates, and the whole pre-September bill reads ~50% high.** The Sonnet 5
introductory window closed 2026-09-01 and the price table flipped correctly —
but `adminstats` prices every window (week, month, all-time) at *today's*
rate, so every token spent before the flip is billed at rates that were not
in force when it was spent: the laptop's ledger cost **≈$1.71** at the rates
actually paid and the panel's formula now answers **≈$2.56** for the same
rows. Nothing else is wrong — tokens, modes and models are all recorded
faithfully; only the dollars drift, and only for spend that crossed the
boundary. · *Cost of leaving it:* the all-time figure is wrong forever (the
week and month windows heal as they slide past the boundary), on the one
page that exists to answer "what is Claude costing me". · **Recommendation:**
yes — price each window segment at the rate in force during it; `prices` can
derive the boundary dates from its own table and the roll-up needs an
`until` beside its `since`, ~60–80 lines plus tests, one sitting. Built by
whoever you hand it to with your call on one small semantics choice: "price
at spend date" (recommended — it is what "estimated spend" means) versus
"today's rate, labelled as a projection". It renders on the Admin panel, so
it wants your eye before it merges. Ledger: Black, 2026-09-05.

**Green: the import page scrolled sideways on every phone, and the culprit
was the example that teaches the `why` format — fixed on green PR #438, and
the walk is one narrow window.** At 375px the page panned 237px because the
example decklist's `pre` set the grid column's minimum width instead of
scrolling inside it; `min-w-0` on the two grid items is the whole diff, with
a mechanism-derived test (mutation-verified, 52/52, full gauntlet green).
*The walk:* start `./mtglab ui` (8765) and `npm --prefix web run dev`
(5173), open `/import`, and narrow the window below ~700px — or devtools
device toolbar at 375px. Before: the whole page pans sideways under your
finger. After: the page holds still and the example scrolls inside its own
box. Nothing animates, no cycle time, ten seconds. · *Cost of leaving it:*
the PR sits unmerged and the live import room keeps drifting sideways on
every phone — the room a newcomer pastes their first deck into. ·
**Recommendation:** walk it, merge it. (The seeded persona-tile report was
also chased: it does not reproduce — all eight tiles, both doors and the
dealt spread carry real accessible names, measured on the served bundle;
the ledger has the numbers.) Ledger: Green, 2026-09-05.

**Green: eight real decks have stood in the checkout's `decks/` since
Aug 24–25, and the one-copy rule says the volume is the only standing
copy.** arahbo-cats through trostani-tokens, deck.yaml mtimes 08-24/25 —
the exact laptop-standing-copy shape that lost two rounds of labels and
created the hosted-first facet. Not deleted tonight: the live local server
on 8765 (another session's, up since Aug 28) plausibly serves them, and
Red's held walk (#436) needs a writable local deck. · *Cost of leaving it:*
any edit made through a local surface diverges silently from the volume's
17-deck truth, and nothing fails when it does. · **Recommendation:** after
walking #436, delete the eight — they are scratch; future local walks pull
fresh from the instance or point `MTGLAB_DECKS_DIR` at a scratch directory
the way the 08-24 measurement run did. Ledger: Green, 2026-09-05.

**Colorless: a 14MB shadow of the deleted Python app still stands at the
repo root, and the zero-trace ruling never saw it because git cannot.**
`build/lib/mtglab/` is a setuptools build of the old `src/mtglab` — `cli.py`,
`animist/`, a full copy of the tarot art — gitignored, untracked, mtimes
2026-08-13, found by the relic sweep counting 100 recipe files where the
tree holds 33. · *Cost of leaving it:* nothing runs it, but any future sweep
or grep over the checkout that forgets an exclude inherits a phantom Python
app the crossing erased from the tree — the exact false-conclusion shape the
completeness-list trap records. · **Recommendation:** delete it
(`rm -rf build/`) — a build artifact of a package that no longer exists, no
personal data in it (checked: code and public art only), and the zero-trace
ruling already wants it gone; queued rather than done only because a relic
is a decision, never a silent deletion. Ledger: Colorless, 2026-09-05.

## Open — 2026-08-24

**1. ADR 5's isolation sweep is half-built now, and the missing half is the
completeness guard.** *(Narrowed 2026-09-05 — the sweeps themselves landed
with #290:* `go/internal/api/refusals_test.go` *asks every deck route about a
deck that is not there, an owner that is not there, and — the ADR 5 sweep —
another account's deck, absent not forbidden.)* What did **not** land is the
parametrised form the ADR asks for: the route lists are hand-typed in the
test file rather than derived from the served route table, so an `{owner}`
route added without being listed is unswept and nothing notices — and the
stranger sweep drives only the write routes, with the reads still uncovered.
White's 2026-09-05 run re-read the route table and confirms this is still a
missing guard rather than a known hole (one accessor, every handler through
it). · *Cost of leaving it:* the next deck route written without the accessor
ships silently, and the thing it would leak is another person's decklist. ·
**Recommendation:** finish it in ADR 5's own form — derive the pattern set
from the served route table, hold the test's lists equal to it, and drive the
read routes as a stranger too. Mis-filing a shared route certifies a hole as
shut, which is why it wants daylight. Ledger: White, 2026-08-24; narrowed
Colorless, 2026-09-05.

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

*(Items 3 and 4 — answered/landed, verified 2026-09-05: the five Python
CodeQL ghosts are dismissed and the open-alert list reads **0** (API
read-back tonight), so a nonzero count now means something real; and the
Library of Alexandria correction merged as #283, 2026-08-24T13:48Z — the
lore shelf says "Magic's first expansion", read back from `lore.json`.
Outcomes recorded in the ledger, Colorless 2026-09-05. White's item 2 above
still open.)*

**Black —**

*(Item 1 — merge PR #284 — answered: merged 2026-08-24T13:32Z and deployed;
outcome recorded in the ledger, Black 2026-09-05. Items below still open.)*

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

*(Item 5 — the five motionless `Loading…` labels — answered/landed, verified
2026-09-05: Red built the spinner pass the same morning and it merged as
#285, 2026-08-24T13:52Z, "five pages stop holding still", walked and
deployed since; the shared `Spinner` now turns in those waits. Outcome
recorded in the ledger, Colorless 2026-09-05.)*

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
built on 2026-08-24 because the whole 2026-08-23 block would have failed it;
*that blocker is gone* — the 2026-09-05 colorless run retired that block's
answered items and gave every survivor its ledger pointer, so the file would
pass the guard today and a yes can be built on directly.
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

**Green —**

*(Items 1 and 4 — answered/landed, verified 2026-09-05: PR #286 merged and
deployed 2026-08-24T14:01Z, its live regions in the tree and the bundle —
the VoiceOver listen joins the owed authenticated walks; and the Scryfall
prune landed since as #420 `pool.SweepBulk`, the volume now holding exactly
one file of each kind, 98MB, down from 121. Outcomes recorded in the ledger,
Green 2026-09-05. Items below still open.)*

*(Item 2 — the light theme's muted grey — answered/landed, verified
2026-09-05: it merged as #405 on 2026-08-29 and went further than the ask —
seven tokens re-stepped for the light page, `--text-muted` now `#73716c` at
4.62:1 with dark's untouched, and `palettecontrast_test.go` gates the whole
light palette so the next un-stepped ink fails a test instead of a reader.
Outcome recorded in the ledger, Colorless 2026-09-05. Items below still
open.)*

**3. Announcing that an answer *arrived*, not just that a wait began.** The PR
above makes every wait and every refusal audible, which is the half that
generalises to all forty-odd surfaces at once. The other half does not: the
announcement lives on the spinner, so it goes when the spinner goes and the
arrival is still silent. Doing it properly is about thirty surfaces, each with a
real choice about what the sentence says. · *Cost of leaving it:* somebody using
a reader knows a wait started and never hears it end. · **Recommendation:** yes,
as its own pass with the wording written deliberately, and start with the four
that matter most — the tarot deal, the Wheel stopping, a simulation finishing,
and the card search's result count. Ledger: Green, 2026-08-24, queued 2.

**5. Three cleanup routines exist, are tested, and nothing calls them.** Expired
sessions, spent invite and reset links, and old rate-limit windows all have a
purge function written for them — and the running app has no scheduled sweep at
all, so none of it is ever removed. The rate-limit rows in particular grow with
traffic rather than with people, so having a hundred accounts does not bound
them. It is 348 KB today, so this is a shape rather than a crisis. · *Cost of
leaving it:* a table on the volume that only grows, and three routines a reader
would reasonably assume are running. · **Recommendation:** yes — one sweep on
boot and then daily, calling all three and logging what it removed. Queued
rather than done because it deletes rows from the accounts database, which a
night run does not do unwatched. Ledger: Green, 2026-08-24, queued 4.

**6. The old-Safari test rig this checklist tells every run to use does not
exist, and your Mac's own Safari is below the floor we promise.** The Green
checklist says real WebKit is testable here through a pinned Playwright rig with
its story in the engineering doc; there is no such dependency in the tree, the
engineering doc has no such story, and the only place that version string has
ever appeared is the checklist itself. Combined with the recorded fact that this
laptop's Safari is *older* than the version the site declares support for, there
is currently **no way on this hardware to see the site in the oldest browser it
claims to work in**. · *Cost of leaving it:* the floor is now guarded against new
features arriving (tonight's test) and witnessed by nobody; a rendering bug only
older WebKit shows would reach a friend first. · **Recommendation:** pick one and
write it down — stand the rig up for real, or strike the claim and say plainly
that the floor is checked statically and witnessed on your phone. Leaving it
claimed is the only wrong answer. Ledger: Green, 2026-08-24, queued 5.

**7. The hosted site tells visitors its cards are on their own machine, and two
capabilities still only exist on your laptop.** Four strings say "the local
pool" — the library masthead renders *"7 decks · 35,393 cards in the local
pool"* to anybody who visits. Separately: **card-art motion** can only be built
by pulling the live decks down, running the toolbox on this Mac and pushing
files back up, which is precisely the shape that lost two rounds of deck labels
last week; and the **match ledger** the app writes on the instance has no
deployed reader at all — the only way to see it is a terminal. · *Cost of
leaving it:* a small untruth told to every visitor, plus one capability that
will diverge the same way labels did. · **Recommendation:** the copy is a
wording call and so the house mother's — "35,393 cards on the shelves" reads
better anyway; the card-art path is the one to answer first because it is
already the written runbook. Ledger: Green, 2026-08-24, queued 6.
## Open — 2026-08-23

*(Item 1 — the coverage floor — answered/landed, verified 2026-09-05: the
floor came back with #290 on 2026-08-24 and got the fakes with it — the
`refusals`/`unreadable` sweeps, tier3's worker and install tests, the pool
download tests — then ratcheted 89.5 → 90.0 → 90.5 (#387), where it gates
today in `ci.yml`'s own formula with `docs/polish/COVERAGE.md` as its story;
the tree measures 90.8 against it, up from 80.3 when this was asked. Outcome
recorded in the ledger, Colorless 2026-09-05.)*

**2. ADR 38 cites `docs/go-migration/`, twice, and the directory is gone.**
The zero-trace sweep deleted it; the ADR links it in its header and its
context. ADRs are immutable. · *Cost of leaving it:* an accepted record points
at nothing, and every future reader of ADR 38 hits it. · **Recommendation:** a
short superseding note recording that the directory was deliberately removed
and where its content went — cheaper than restoring it, and honest about why.
*(Re-verified still open 2026-09-05 — ADRs run to 0049 and none supersedes
the cite.)* Ledger: Cleanup — the "Not yet run" backlog.

*(Item 3 — the unchecked `needs` list — answered/landed, verified 2026-09-05:
rebuilt by Red as `go/cmd/mtglab/pipeline_test.go` on #285, 2026-08-24,
deriving the expected set from `ci.yml`'s own `jobs:` keys — and the hole it
was written against had already fired once, which Red's 2026-08-24 entry
records. Outcome in the ledger, Colorless 2026-09-05.)*

*(Item 4 — does night work merge itself — answered in use, recorded
2026-09-05: the Nightbound section of the skill settles the commandment 16
reading, and Aaron's own rainbow invocations since instruct it verbatim —
user-visible work stops at a green PR, everything else merges on green
required checks; three night merges rode that rule on 2026-09-05 alone, each
deploy walked. The stricter reading remains one sentence away if a 3am
deploy ever proves unwelcome. Outcome in the ledger, Colorless 2026-09-05.)*

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
time. A colorless run's natural first job. *(Still open 2026-09-05; the
implementation note it needs — five implied roots, 42 tokens measured — is in
the ledger.)* Ledger: Colorless, 2026-08-24 (recovered records).

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
spot the local mount cannot explain. *(Still open 2026-09-05 — the tree still
contains no pprof.)* Ledger: Colorless, 2026-08-24 (recovered records).

---

## Answered

- **Mutation testing: adopt `gremlins`** — yes, 2026-08-23. Installed on
  demand (`go install github.com/go-gremlins/gremlins/cmd/gremlins@latest`),
  no `go.mod` entry, one package at a time, determinism kernels first, never
  `internal/api`. The protocol now lives in the skill (White's testing facet);
  `docs/ENGINEERING.md` names it as the project's mutation tool.

*(Answered items move here in one line with the ruling and the date, then out
entirely once the ledger carries them.)*
