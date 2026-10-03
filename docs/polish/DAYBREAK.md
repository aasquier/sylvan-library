# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one item —
headline first, context after — with a recommendation, so the whole file can
be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed. While an item
waits, its ledger record wears the marker `**(open) …**` at the head of its
bold lead, struck when you answer; `daybreakrecord_test.go` holds the two
files to each other in both directions off that marker.

Each item: what it is · what it costs to leave it · **the recommendation.**

> **2026-09-29, morning: eight rulings taken, ten items left.** Aaron
> answered the whole *a ruling* group in one line — ratchet, yes to both, six,
> unions, obituary, close, headline, marker — and each is landed and recorded
> in the ledger's Cleanup section under 2026-09-29. The evening before, the
> Cleanup step had regrouped the queue after the daylight rainbow (35 headings
> → 17; eighteen retired by merges, five moved to owed work — the 2026-09-28
> entry says where each went). What stays is grouped by what an answer costs
> you: a few clicks, a dollar, a watched deploy, a migration window — and the
> last group needs nothing today.
>
> **2026-09-29, evening: the shelf memo landed, nine left.** The one item in
> the *watched deploy* group that was the pass's to build went as its own PR
> in a session Aaron was in — Black's 2026-09-29 entry has the numbers — and
> the worker machine's half-landed deploy from the morning was read and
> repaired by hand first (Red, 2026-09-29). The other watched-deploy item,
> the dossier's `allies` golden, went as the branch stacked on it: two
> sentences and one golden re-recorded, eight left.
>
> **2026-10-02, afternoon: the library gathered again, seven left.** *Reality
> Fracture* released, and the refresh ran from the terminal the same
> afternoon — 38 seconds, the released commanders resolving on the instance;
> Green's 2026-10-02 entry has the read-back, including a pool file that did
> not hold near 85 MB and what it measured instead. The two walks the 09-29
> pair owed are in Black under the same date.
>
> **2026-10-03, daylight: the climb to 99 — two White items answered by the
> ask itself.** Aaron ruled the opposite of the regression-guard
> recommendation: *"get our test coverage up to 99%, rewrite stubborn code to
> be more testable"*. Eight Opus lanes ran in parallel on disjoint package
> groups and landed through one integration pull request: 652 → 136
> missing, **97.0 → 99.4**. The `pool.Pool` connector field the second item
> asked for is in (`pool.NewOver`, reached through `pooltest.OpenFaultyPool`),
> and the dossier's ten statements are closed with it. COVERAGE.md's *Left
> deliberately* carries the record, entry by entry. Ledger: White, 2026-10-03.
>
> **2026-10-03, night: seven lanes ran one after another — three merged, three
> are held for your eye, and one of the night's questions is about money.**
> Black added two: the Sonnet 5 price rise this project's whole rate table was
> built around was cancelled, which the table does not know, and a year-long
> `immutable` on an unversioned URL. Red added one: the press clause of
> commandment 17, now a register, with the design pass it measures waiting on a
> branch. All three are in the *watched deploy* group, and the pricing one asks
> to travel with the cache-write column below it. **Green, the Queen and the
> Coliseum each built a change that renders, took it to a green pull request
> and stopped there** — nothing a person can see merges while you are asleep —
> so the morning's first job is three walks and three merges, and the note at
> the head of the *watched deploy* group is the order to take them in. Each of
> those branches carries its own queue line onto this file as it lands, so this
> paragraph's arithmetic is a reading of `main` and not a forecast. The
> Colorless lane that wrote this paragraph added the eleventh item itself — one
> ruling, at the foot, about where the night's own tooling should live. The
> coverage lane, which landed after it, added two more: the climb re-measured
> at 97.0% with nothing left above two statements, so it asks whether 95.5
> stops being a target and becomes a regression guard with mutation testing
> as the instrument — a ruling and nothing else — and a ten-statement gap
> whose own item left this file during the 09-28 regroup without being
> answered, found because `docs/polish/COVERAGE.md` still points at it. Count
> with the recipe below, never with this paragraph.

**How many are open right now is a question for the file, not for this
paragraph.** Count them with the colour, never with the bold — and every item
heading starts `**Colour:`, with nothing between the colour and the colon,
because the recipe below cannot see a heading like `**White (leg two):` and
two of those hid from it for a week:

```
grep -cE '^\*\*(White|Blue|Black|Red|Green|Colorless):' docs/polish/DAYBREAK.md
```

A count written into prose is a claim that rots the next time anyone adds a
line, and so is the recipe for checking it. Run the recipe; re-read the recipe.

---

## Open — a ruling, and nothing else

**Colorless: every night rewrites the same four tooling files in a scratch
directory, and three of them are recipes this project already describes in
prose.** Tonight's seven lanes ran off `gowrap.sh` (five lines: this Mac's
three Go exports, a `cd` into `go/`, `exec "$@"` — the wrapper the skill
describes because the harness refuses the compound form), `deploy.sh` (poll
the `tests` run on `main` by sha, then the deploy job, then the machine image
tags, then `/api/health`), `poll.sh` (the same loop for a pull request's
checks) and a shared `LANE_BRIEF.md` carrying the night rules, the harness
traps and the two test families that fail on load rather than on truth. All
four worked; all four vanish with the scratch directory. · *Cost of leaving
it:* every night spends its first twenty minutes turning prose into scripts
before it can run a gauntlet or watch a deploy, and the brief's traps get
re-learned one lane at a time instead of read once. · **One of them must not
be committed as written:** `poll.sh` freezes the eight required check names as
a literal, and the skill's own landing step says to read that list back from
the API *because it has grown twice with no prose noticing*. Read tonight, the
API returns exactly those eight — so the script is correct today, which is
what makes the frozen form a trap rather than an error. · **Recommendation:**
yes — a new `.claude/polish/` holding `gowrap.sh`, `deploy.sh` and the lane
brief, with `poll.sh` rewritten first to read its check list from
`gh api repos/aasquier/sylvan-library/branches/main/protection`. Three small
files and one rewrite, no dependency and nothing that ships. Ledger:
Colorless, 2026-10-03.

**Green: who may see which bouts? One answer unblocks both of the Coliseum's
two missing rooms, and neither can be built without it.** ADR 46 names the
night shelf as its own PR and deliberately leaves cross-account leaderboards to
you, pointing at `ledger.Scope` as the one place that would widen. Today the
scope is the narrow one — a match you were in, plus the house's own — and two
separate features run straight into it: the **night shelf** (a night is a
round-robin across accounts, so a shelf is either near-empty for everybody but
you, or it is the widening the ADR reserved) and **a way back to the deck from
the record** (the board prints a deck's slug and links nothing, because an
opponent's deck is 404 by ADR 5 and a link that sometimes dead-ends is worse
than none). · *Cost of leaving it:* the room keeps a record you cannot click
and a night you cannot read, and every session that reaches for either
re-derives this paragraph. Nothing is broken and nothing is urgent — the night
has not run yet. · **Recommendation:** rule that **a deck appearing on the
record is openable when it is the house's own or yours, and otherwise is not a
link at all** — the narrow reading, no new sharing, and enough to make the
record clickable this week; then rule separately on whether a night shelf shows
only your own and the house's bouts (same rule, and buildable the moment the
torches are lit) or the whole night (a sharing decision, and a bigger one). The
shelf's full shape — the route, the join, the tab, the empty state — is written
out in the ledger so it is not scoped twice. Ledger: Green, 2026-10-03 (night,
the Coliseum).

## Open — a few clicks in the repository settings

**Blue: the Coliseum has never fought a night, because the two switches that
open it were never set on the instance — and this line spent a month reading
as if it waited on something else.** `MTGLAB_NIGHT_WINDOW` and
`MTGLAB_NIGHT_ZONE` are read at boot; unset means no scheduled nights, which is
what `fly secrets list` has shown since ADR 46 landed (re-read 2026-10-03:
zero `MTGLAB_NIGHT_*`, in secrets and in `fly.toml`). The earlier wording
asked for the night shelf first; the shelf is a room to *read* the nights in,
and nothing stops the nights running without it. · *Cost of leaving it:* the
arena's record never moves, the Forge worker never wakes, and the Settings
room's "the torches are not lit yet" stays true for the wrong reason. ·
**Recommendation:** two minutes in your own session —
`fly secrets set MTGLAB_NIGHT_WINDOW=01:00-04:00 MTGLAB_NIGHT_ZONE=America/Los_Angeles -a sylvan-library`
(six bouts of ten games is about fifty minutes at the Forge's measured pace,
so a three-hour window ends early rather than short; the other three switches
keep their defaults) — then opt the decks that should fight into the arena
after dark in the Settings room. After the first night, that Settings line
becomes the untruth and the follow-up — the room reading the schedule off the
wire — is real work for the next Blue run. Ledger: Blue, 2026-09-05.

**White: nine Dependabot alerts, all `pip/torch`, all `development`, triaged in
prose since 09-12 and never dismissed — two minutes in the Security tab.**
Re-read from the API tonight: **9 open, 1 critical / 3 medium / 5 low, every
one `pip/torch`, every one `development` scope** — the same nine. · *Cost of
leaving it:* the security tab re-asks a settled question forever and every
security read pays to re-derive the answer. · **Recommendation:** dismiss all
nine as "tolerable risk — see `tools/pyproject.toml`'s depth-extra triage"
(dev-Mac only, never ships, safetensors-only, pinned by real code since #463).
· **Shorter than it was, on purpose:** the 09-28 Cleanup entry found that an
item carried only because *the pass cannot click in the Security tab* earns a
one-line ask rather than a paragraph re-verified each cleanup, and this is that
finding acted on instead of restated. Carried 09-12, 09-19, 09-28 and 10-03 for
that one reason; a fifth carry should stop re-reading the API. Ledger: White,
2026-09-12.

## Open — a dollar and an account

**Red: nothing off-platform tells you the site is down, and a hung process
that keeps its port is an outage nothing detects.** Fly's HTTP check stops
routing on failure and the restart policy fires only on exit, so a wedged
process is a total outage with no alarm. · *Cost of leaving it:* the first
person to notice an outage is a friend at breakfast. · **The two free minutes
are spent and they end at your login:** `fly` has no alert-rule subcommand,
fly-metrics.net answers 401 unauthenticated, and `FLY_METRICS_TOKEN` is a
read-only Prometheus credential rather than a Grafana one — so *whether any
alert rule exists* is two clicks in your own Fly session and nothing a run can
find out. · **Recommendation:** look once while you are in there, then
UptimeRobot's free tier on `GET /api/health` (GET, never HEAD: `HEAD /`
answers 405, and a Go guard asserts the GET) wired to Pushover ($5 once) for
the phone. The health body now reports `app_db`, `disk_free_mb`,
`schema_version` and `pool_age_days`, so the monitor has something to read
besides 200. Ledger: Red, the queued list carried in the 2026-09-05 entry,
items 1–2; answered as far as possible 2026-09-26.

## Open — a watched deploy

> **Three branches are stacked behind your eye this morning, and the order is
> cheaper than it looks.** `polish/green-2026-10-03` (#532, the colour wheel's
> five discs), `polish/queen-2026-10-03` (#533, the press reply) and
> `polish/coliseum-2026-10-03` (#534, the Forge match's remaining-time line).
> Read as a bundle problem this is frightening and it is not one: the three
> rebuilt **three disjoint files** under `web_dist/assets/` — `pentagram.js`,
> `DeckDetail.js` plus `index.css`, and `Coliseum.js` — so no two of them touch
> the same built file and the bundle cannot conflict between them. What
> conflicts is `DAYBREAK.md` and `LEDGER.md`, which all three edit.
>
> **That paragraph was reasoning; this one is a reading.** For each branch, the
> files changed on the branch and changed on `main` since that branch's own
> merge base were intersected, and for all three the answer is **exactly those
> two documents and nothing else** — so the disjoint-bundle claim is now
> measured rather than argued. **All three were brought up to `main` at the
> end of the night and read `MERGEABLE`** — the orchestrator did the two
> document resolves so the morning would not. Merging any one of them puts the
> other two behind again, on these same two files: try `gh pr update-branch`
> first, and when it refuses (it declines a branch whose merge is not clean),
> the resolve is a local `git merge origin/main` on the branch, **keep both
> sides of the two document hunks**, push. Nothing was reflowed; it is a
> two-minute resolve each. (#534's own ruling item was folded into the existing `## Open — a
> ruling, and nothing else` group on its branch, so no second group lands.) **Never
> hand-resolve a file under `web_dist/`** — it is generated, and a hand-merged
> bundle is a file no build can reproduce; if one ever does conflict, take
> either side, run `npm --prefix web run build`, commit what that writes, and
> then `go test -race -count=1 ./cmd/mtglab/`, because Go's test cache tracks
> nothing outside `go/` and the guards that read the bundle will otherwise
> answer a stale green. When the last one is in, one `npm --prefix web run
> build` on `main` should write **no** diff — that is the proof the three
> partial rebuilds compose, and it costs a minute.

**Red: the third clause of commandment 17 is built and waiting for your eye —
three classes took the press reply, the register fell from fifteen to ten, and
every line of it renders.** The register landed overnight as test-only and
merged; the design pass it measures is on `polish/queen-2026-10-03`, green and
unmerged because a merge is a deploy. `.card-action`, `.strip-tab` and
`.disclosure-toggle` now answer a press with two halves — a 1px settle plus a
ground or an inset sink — and the second half exists because a reply built out
of movement alone vanishes under `prefers-reduced-motion`, which is what
`.chip-toggle` and `.chip-place` currently do to their own press. One `.btn`-
family bug rode along: the deck page's *Tag a pilot* button carried an inline
`border` that duplicated its class's own and silently reset the `border-color`
its `:hover` sets, so the edge of that control never answered the pointer at
all; a new guard refuses the shape tree-wide. · *Cost of leaving it:* nothing
breaks, and the register holds the number where it is — but ten dressed
buttons stay two-thirds of a control and the branch goes stale against the
bundle. · **Where to look:** `mtglab-ui` and `web-dev` in `.claude/launch.json`
(the Go server on 8765, Vite on 5173; auth is off locally so the deck pages
open), then **the Coliseum's tab strip** (`/coliseum`, the row reading *The
sand · The record · The laurels*) and **a deck page's card actions and
category folds**. Press and **hold** one of each: it should sink a pixel and
take a faint ground, and let go cleanly. Nothing here animates on a loop, so
there is no cycle time to wait out — the only thing with a clock on that page
is the Coliseum's own hero video, which is an 11-second loop and is not part of
this. · **Recommendation:** merge it. Then rule separately on the ten classes
left, which are the deliberately bespoke ones (`.art-pick-tile`,
`.reader-tile`, `.wheel-folded`, `.tarot-hinge` and their siblings) — a settle
may be wrong on a card tile — and on whether `.chip-toggle` and `.chip-place`
should keep a non-moving half of their press under reduced motion, which is
the same correction applied backwards to an already-argued rule. Ledger: Red,
2026-10-03 (night, the Queen).

**Green: the colour wheel's five discs now answer a thumb, and its ten guild
lines cannot — 44px there is geometry, not effort.** The 44px floor #509 took
is a `min-height` under `(pointer: coarse)`, and an SVG shape has no
min-height, so the fifteen controls in the wheel were never covered by it:
measured on a phone, a disc was **38px** and a guild line is **14.6px** across.
The discs are fixed on `polish/green-2026-10-03` (an invisible hit circle,
**38 → 46px** measured live, mutation-verified test; and, by your ruling on
the walk, the five discs now wear the official symbols every other pip wears,
with the drawn marks as the fallback — the one thing on the branch that
renders differently). The lines would need a **60-user-unit** band each — ten of them,
converging on five points — so every pair would overlap and the star's
crossings would belong to whichever line was drawn last. · *Cost of leaving
it:* a thumb on a phone picks the wrong guild, or none; `/learn` carries 32
real `/colors/…` links, so nobody is shut out of the page, they just lose the
diagram as a way in. · **Where to look, when you want to:** `web-dev` and
`mtglab-ui` in `.claude/launch.json` (Vite on 5173 against the Go server on
8765), then `/learn` → *The colours* at a phone width — the wheel is the
pentagram under the tabs. Nothing animates there, so there is no cycle to wait
out; the only thing to check by eye is that it looks **exactly** as it did, and
the only thing to check by hand is tapping a disc near its edge. ·
**Recommendation:** merge the branch for the five discs, and for the ten lines
take the phone-only answer rather than a wider band — the wheel stays the
pointer's affordance and the guild list beneath it is the finger's, which is
also what the hover caption already implies (`(hover: hover)` is false on a
phone). Ledger: Green, 2026-10-03.

**Red: a deploy takes no snapshot, and the boot after a merge is the moment
the volume is most at risk.** Fly snapshots daily on its own clock; the ladder
is forward-only and applies unwatched, and the `deploy` job in
`.github/workflows/ci.yml` still has no snapshot step — re-read tonight: the
only `snapshots` word anywhere in the workflows is a Forge release tag. **This
line said deploy.yml for a month and there has never been such a file in this
repository** (`git log --all` knows nothing of it), so three carries sent a
reader to a path that does not exist; continuous deployment lives in `ci.yml`'s
own `deploy` job, gated on the `tests` job, and that is where the step goes. ·
*Cost of leaving it:* the one deploy that needs a rollback point is the
one guaranteed not to have a fresh one. · **Recommendation:** a `fly volumes
snapshots create` step ahead of the `flyctl deploy --local-only` call in that
job, non-fatal on failure, once the deploy token's scope is checked — a
workflow change only CI can prove, so it lands as its own PR on a morning you
can watch the deploy. Ledger: Red, the queued list carried in the 2026-09-05
entry, item 6; the filename corrected by Cleanup, 2026-10-03.

**Black: the Sonnet 5 price increase was cancelled, the price table still
applies it from September 1st, and the "go and check the rates" link on your
own admin panel answers 404.** The pricing page now says the $2/$10 launch
pricing *is* the standard price and the scheduled rise to $3/$15 "will not
occur" — so every recorded conversation dated on or after 2026-09-01 is
priced 50% high. Tonight's read of the instance is $9.0034; priced flat at
$2/$10 the same tokens come to about $7.78, which puts roughly 31% of the
figure inside the window that never happened and makes the panel **over-read
by about $1.22, some 13.6%**. That is almost exactly the size of the
cache-write **under**-read queued below, pointing the other way, which is why
neither ever looked like a wrong number. · *Cost of leaving it:* the one
dollar figure this project reports about itself is wrong in both directions
at once, and the pointer to the page that would settle it is dead. · **The
reason it is not already fixed:** three of the seven cases in
`prices/testdata/prices.json` exist to pin that boundary, so removing the
window re-records a frozen golden — not a thing a night run does. ·
**Recommendation:** yes to all three in one branch — drop Sonnet 5's
`Until`/`Then` window, re-record those three golden cases deliberately, and
point `prices.Source` at
`https://platform.claude.com/docs/en/about-claude/pricing`; land it alongside
the cache-write column below, since the two corrections cancel and shipping
one alone moves the headline figure the wrong way. Ledger: Black,
2026-10-03.

**Black: the card reader's files promise a browser a year of immutability on
a URL with no version in it, so the next engine upgrade quietly breaks the
camera for anyone who visited before.** `/api/ocr/*` answers
`max-age=31536000, immutable`; the version stamp is in the path on the
volume, not in the URL a browser keys on. The worker and the engine core are
version-coupled, so a returning visitor who still has one of them cached and
fetches the other fresh gets a mismatched pair and a reader that fails with
no error. · *Cost of leaving it:* nothing at all until somebody bumps the
reading engine — and then it is invisible, because it only affects people who
used the camera before. · **Recommendation:** put the stamp the shelf already
computes into the URL (`/api/ocr/<stamp>/<name>`), which makes `immutable`
honest and costs one route pattern and the three paths in `reader.ts`; the
cheaper alternative is to drop to the door's own `no-cache`-plus-ETag and pay
one revalidation per visit. It moves a served route and the committed bundle
together, so it wants a deploy you are watching. Ledger: Black, 2026-10-03.

**Green: a Forge match now tells you how much longer it has to run, and it is
waiting for your eye.** The longest wait in the application — ten whole games
of Commander — showed how far in you were and never how much was left; the
sentence under the feed spanned a factor of twenty, which is the difference
between forty seconds and twenty minutes. The stage now says *"6 more games to
fight — about 5m 6s, at the pace of the ones already fought"*, measured off
the match's own games (median, never a mean; silent until two have landed,
because a first game is the slowest of the match). No CSS — the Queen's branch
holds `index.css` tonight — and no Go, no route, no schema. · *Cost of leaving
it:* nothing breaks, and every person who sends two decks in goes on guessing
whether to wait; the branch goes stale against the bundle. · **Where to look:**
`mtglab-ui` in `.claude/launch.json` (the Go server on 8765 serves the
committed bundle, auth is off locally), then **the Coliseum** (`/coliseum`) —
pick two decks, *Send them in*, and read the line under the feed once the
second game lands. There is no Forge on this Mac, so the cheap version is the
room's own fixture route: open `/coliseum`, patch `window.fetch` to answer
`/api/jobs/fixture` with a running job carrying four `partial.rows`, then go to
`/coliseum?m=fixture` — the recipe is in the ledger entry. Nothing here
animates on a loop, so there is no cycle time to wait out; the hero video on
that page is an 11-second loop and is not part of this. · **Recommendation:**
merge it. Ledger: Green, 2026-10-03 (night, the Coliseum).

## Open — a migration window

**Black: prompt-cache *writes* are invisible in both usage ledgers, so the
spend figures are a little under.** `cache_creation_input_tokens` appears
nowhere outside two test fixtures; writes bill at 1.25× input. · *Cost of
leaving it:* the dollar figure on the Admin panel and in `mtglab claude usage`
under-reads by the write premium; 09-26 put a floor under it — every mode's
cacheable prefix measured, the instance's 213 conversations wrote **at least
≈490,000 cache tokens**, **$1.22–$1.84 unrecorded against a recorded $9.0034,
a 14–20% under-read** — with a provable ceiling of $24–37. A 20×-wide bracket
is what the column collapses. · **Recommendation:** add the column on a day
you can watch the boot (a migration is your window by standing rule); nothing
until then. The theme mode's unreadable second cache breakpoint (Black,
2026-08-24, item 2) waits on this column as its instrument and is not a
question of its own. Ledger: Black, 2026-08-24 (the carried list); re-checked
2026-09-19 and 2026-09-28.

## Open — deliberately waiting, nothing to do yet

**Black: the cache-read price is one constant for the whole family, and the
family stopped agreeing — but nothing is mispriced yet.**
`prices.CacheReadFraction` is 0.1 for every model on the argument that the
ratio is the same across the family; Claude Fable 5.1 prices cache reads at
0.025× its input. `claude-fable-5-1` is not in `Table`, and the instance runs
Sonnet 5 on every row, so today this is a trigger rather than an error. ·
*Cost of leaving it:* nothing until a model with a different read fraction is
added to `Table`, at which point its cache reads are priced 4× high, silently.
· **What would have to be true:** the fraction moves onto `Priced` beside the
rate, which means extending `testdata/prices.json` — a frozen golden, and not
a thing a polish run extends on its own. · **Recommendation:** land it in the
same branch that adds such a model. Ledger: Black, 2026-09-26 (deferred
2026-09-19).

**White: `NOTICE.md` is held and the skills are held; the rest of the tree's
prose is still unguarded.** `licenserecord_test.go` holds every repository
path and `mtglab` verb the licensing record names, `skillrecord_test.go` holds
`.claude/skills/`, and the shared kit is `recordkit_test.go`. What none of them
reads is `docs/`, `web/README.md` and the package comments — and the 09-13
morning found two rot instances there, both fixed, neither caught by anything
but a person reading. · *Cost of leaving it:* nothing legal; this is tidiness
with a mechanism — and the mechanism bit this month: the snapshot item above
sent three readers to a deploy.yml that has never existed in this repository.
· **The cycle of measured rot arrived, and it also measured why the extractor
cannot simply be pointed at `docs/`.** Run tonight over the two files of this
pass, the existing `repoPaths` rule flags **5 anchors in `DAYBREAK.md`, and 4
of the 5 are the queue doing its job** — `gowrap.sh`, `deploy.sh`, `poll.sh`
and `LANE_BRIEF.md` are named by the Colorless item *because they are not in
the tree*; the fifth is a gitignored deck file. A queue names what does not
exist yet; a record names what does, which is why the same extractor is honest
on `NOTICE.md` and would cry wolf here. The map is the other case:
`COVERAGE.md` names **41 anchors and 40 resolve**, the one miss being a
gitignored `deck.yaml` — but only once the resolver allows a path written
relative to `go/`, because that file speaks from inside the Go tree.
Root-anchored resolution alone calls 19 of its 20 slashed paths broken. ·
**Recommendation:** not a third extractor over `docs/` wholesale — a **suffix
resolver** added to the kit plus a guard on the *maps* (`COVERAGE.md`,
`web/README.md`), never on this queue, and never on `LEDGER.md`, which is
history and is supposed to name files that were deleted. The first increment
landed with this measurement (Cleanup, 2026-10-03: this queue's workflow and
manifest anchors are held, which is the one slice with no false alarms in it).
Ledger: White, 2026-08-24; narrowed Cleanup, 2026-09-05; measured Cleanup,
2026-10-03.
