# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one line with
a recommendation, so the whole file can be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed.

Each item: what it is · what it costs to leave it · **the recommendation.**

> **2026-09-28, evening: the Cleanup step regrouped the queue after the
> daylight rainbow.** Every item was re-checked against the tree, the API and
> the instance rather than inherited. Eighteen headings left the file because
> the thing they were waiting on has **merged and deployed** — the five balls
> (#503, #509, #511, #516, #518), the two Hercules (#517), the MDFC land count
> (#506, "union"), the mode prompts (#513), the pool's age in the health body
> (#515), the faulty pool handle (#514) — and each is recorded, with where it
> went, in the ledger's Cleanup section under 2026-09-28. Five more were *work
> owed rather than a question* and moved to that same entry's "owed work"
> list, because a line here that needs no answer is furniture. What stays is
> grouped by what an answer costs you: a few clicks, a word, a dollar, a
> watched deploy, a migration window — and the last group needs nothing today.

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

## Open — a few clicks in the repository settings

**White: the nine open torch Dependabot alerts are triaged in prose and never
dismissed on GitHub, so the security tab re-asks a settled question forever.**
The triage lives in `tools/pyproject.toml` (containment: dev-Mac only, never
ships, safetensors-only snapshot — pinned by real code since #463). Re-read
from the API 2026-09-28: still exactly 9, all `pip/torch`, all `development`
scope. They stay open because dismissing needs repo-admin, which the pass does
not have and should not. · *Cost of leaving it:* every future security read
spends the hour re-deriving this paragraph. · **Recommendation:** dismiss all
nine as "tolerable risk — see tools/pyproject.toml's depth-extra triage" (two
minutes in the Security tab). Ledger: White, 2026-09-12; carried by Cleanup on
09-12, 09-19 and 09-28 for the same stated reason: the pass cannot click this.

## Open — a ruling, one word each

**White: the coverage floor — the trigger you set has arrived, and the word is
"ratchet" or "anchor".** You asked on 2026-09-24 for "the tree at 96 and the
gate at 95"; the tree then climbed 96.6 → 96.9 → and merged `main` printed
**97.0** in the `Coverage floor` step on 2026-09-28 (read off run 36504824056,
not off this file). Both readings of your ruling are defensible: hold 95.0 as a
number, or keep the ~1.5-point gap as the tree climbs. · *Cost of leaving it:*
nothing this week; at two points of gap a diff can lose a whole point of honest
coverage without a check going red, which is the one thing the floor exists to
notice. · **Recommendation:** **"ratchet"** — `MINIMUM` in `ci.yml` goes to
95.5 and the gap is kept, because what you asked for was slack for refactoring
and slack does not shrink as a codebase grows. Say **"anchor"** if 95 was a
floor the tree simply grows away from, and the line closes. Ledger: White,
2026-09-26 (rainbow, leg two), and the leg-one entry the same day.

**White: two named coverage levers should be closed as argued rather than left
open — `ApplyBulk`'s four fold branches and `prompt.secret`'s terminal arm.**
Both were worked on 09-26 and both are honestly unreachable: `PlanBulk`
refuses the same file through the same lookup, so `ApplyBulk`'s folds are
defensive depth behind a guard; and the password prompt's terminal branch would
need its terminal read handed in as a value, which moves the five statements
into the seam's own default rather than out of the tree. `COVERAGE.md` already
carries both arguments in place. · *Cost of leaving it:* one hour per future
coverage leg, spent rediscovering this. · **Recommendation:** "yes to both" —
they move into `COVERAGE.md`'s *Left deliberately* list, so the map keeps its
promise of being the thing you read before spending an hour on a branch that
cannot be entered honestly. Ledger: White, 2026-09-26, queued items 2 and 3.

**Black: the two prompt sentences #513 deliberately left as written — and in
both, the *code* may be the wrong side.** One word each. **(a)** The interview
asks for "three to five questions" (`interviewOpening`, `interview.go`) and
`MaxQuestions = 6` truncates the answer, so a sixth question is kept while a
seventh vanishes silently. *Recommendation:* **"six"** — the ask becomes "up to
six", a prompt whose number *is* the cap cannot drift from it; "five" is the
other consistent answer and brings the constant down. **(b)** The theme
interview's schema tells the model a re-stated slot set "replaces the previous
set rather than adding to it", and `Carry` unions — deliberately, because the
replace rule left a first-timer's reading going 0, 1, 0, 1, 0 and never ready
(commandment 2's failure exactly). *Recommendation:* **"unions"** — the
sentence becomes "everything you have learned since your last answer; anything
you said before is remembered whether you repeat it or not"; both halves still
pass `Ground` against the same transcript, so nothing is carried the person
did not say. · *Cost of leaving them:* (a) a seam nobody has tripped; (b) the
model re-states defensively and spends output tokens proving what the code
already guarantees. Ledger: Black, 2026-09-26 (rainbow, prompts).

**Green: `goreclaw-stompy` was the one deck in the checkout's `decks/` that
existed nowhere else, and it is deleted now.** It survives in git history
(`git show 5515f5f^:decks/goreclaw-stompy/deck.yaml`) plus four theme words in
the ledger; the mtg-lab skill's trigger list still names "mono-green/Goreclaw"
and the instance has no such deck (re-read 2026-09-28). · *Cost of leaving
it:* nothing — nothing served it. · **Recommendation:** if you still want it,
paste that file through the site's import page and the library owns it; if
not, say "obituary" and the trigger list drops it on the next Blue run.
Ledger: Green, 2026-09-19.

**Red: the "drill older than the newest migration" rule cannot be satisfied,
and the drill that proved it is walked and recorded.** Snapshot retention is
five days; a snapshot can only ever rehearse a rung landed in the last five
days, and `docs/HOSTING.md` §Backups already says so. · *Cost of leaving it:*
nothing — the wording is landed; this is the one residual question. ·
**Recommendation:** "close" — HOSTING's sentence is the rule, and a second copy
on a merge checklist is the kind of duplicate this file exists to refuse; say
"checklist" instead if you want the five-day drill named beside "land schema
changes when you can watch them" in CLAUDE.md. Ledger: Cleanup, 2026-09-19
(the drill and the retention finding are Red records carried by Cleanup).

**Colorless: this file's own rule says "one line per item" and not one item
has obeyed it for a month — is the rule wrong, or is the file?** Every item
here is a paragraph, and you have answered the file happily in that shape (25
in one morning on 09-05). A line-count gate would fail the file you blessed,
and rewording a rule to match drift is the move the pass forbids, so nobody has
touched either side. · *Cost of leaving it:* nothing today; the risk is the
file growing into the thing the queue exists to replace, with no rule anyone
can point at. · **Recommendation:** "headline" — the rule means *one item per
question, headline first, context after*, which is what the good items already
do; say the word and the sentence gets rewritten once and then held by the
shape of every new item. Ledger: Colorless, 2026-09-19, and 2026-09-26 for why
it arrived late.

**Colorless: the guard on this file reads one direction only, and the missing
direction is how five items hid for 26 days.** `daybreakrecord_test.go` proves
every line here names a ledger record; nothing proves every *waiting* ledger
record has a line, because "queued" in ledger prose carries no marker a test
can read — and an answered item correctly *leaves* this file, so a naive
reverse check would demand a line for every historical block ever written.
The missing piece is one convention, and a convention binds every future
session, which makes it yours rather than a run's. · *Cost of leaving it:* the
breach recurs in the only direction nothing watches; it has happened twice
that we know of. · **Recommendation:** "marker" — queued blocks that are still
open get a literal `(open)` after the heading, struck when you answer, and the
next Colorless run extends the guard to read both ways; say "no" and the
reverse walk stays a `git log -S` recipe in `references/colorless.md`, which is
honest but only runs when somebody remembers. Ledger: Colorless, 2026-09-19,
re-filed 2026-09-26.

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

**Black: every visit to the deck shelf re-reads and re-parses the whole
library — ~42 ms of CPU and 27 MB of allocation for 25 decks, on a machine
with two shared cores — and the memo that fixes it has been refused by the
harness twice.** `/api/decks` builds a fresh `FileSource` per request, so
nothing memoises anything; ~90% of the cost is inside the YAML library, so the
only lever that pays where it ships is not parsing a file that has not
changed. The lane brief for it was refused by the auto-mode classifier on
2026-09-26, twice, because it describes a watched deploy; it needs a session
you are in. · *Cost of leaving it:* the home page's shelf call spends 42 ms of
server CPU per visit for an answer that was identical last time, forever. ·
**What makes it a question:** the memo needs an owner that outlives a request
— the long-lived `*API`, handed down through `Resolver` into `NewFileSource`,
five to eight files — and its invalidation is the pool's file-stamp guarantee
applied to live user data. · **Recommendation:** "yes" — its own PR, built in
a session where you say the word and watch the deploy, with hit and miss
counters in from the start and rendered nowhere. Ledger: Black, 2026-09-26.

**Black: the dossier's `allies` gap is a frozen golden's to move, so it wants
its own watched branch.** #513 repaired nine prompt sentences and left the
dossier untouched on purpose: its instructions and schema are hash-frozen in
`testdata/dossier.json`, and its own drift — `allies` missing from the search
enumeration, in the prompt and again in `dossierOpening` — cannot land without
re-recording that golden, which invalidates every dossier the instance has
stored. · *Cost of leaving it:* the dossier mode is told to search for one
fewer thing than the schema asks it to report, on every deck it is asked
about. · **Recommendation:** "go" — one branch that fixes both sentences,
re-records the golden from the tool's own output and says so, and lands on a
morning you can watch; nothing else rides it. Ledger: Black, 2026-09-26
(rainbow, prompts).

**Red: a deploy takes no snapshot, and the boot after a merge is the moment
the volume is most at risk.** Fly snapshots daily on its own clock; the ladder
is forward-only and applies unwatched, and `deploy.yml` still has no snapshot
step. · *Cost of leaving it:* the one deploy that needs a rollback point is the
one guaranteed not to have a fresh one. · **Recommendation:** a `fly volumes
snapshots create` step ahead of `flyctl deploy` in the deploy job, non-fatal
on failure, once the deploy token's scope is checked — a workflow change only
CI can prove, so it lands as its own PR on a morning you can watch the deploy.
Ledger: Red, the queued list carried in the 2026-09-05 entry, item 6.

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

**Blue: the Settings room says "the torches are not lit yet", and the only
thing keeping that true is that you have not flipped the switch.** The line is
hand-written into the bundle (`web/src/routes/Settings.tsx`) and true today,
but the evening you set the five night secrets changes no code and rebuilds
nothing, so the room would keep telling people the arena is dark while it
fights. · *Cost of leaving it:* a small untruth on the one page where a person
decides to enter their decks. · **What would have to be true:** the Coliseum's
night shelf lands (ADR 46 names it as its own PR) and the settings room reads
whether a night is scheduled off the wire. · **Recommendation:** unchanged —
the copy becomes a fact the server owns when the shelf gives it something to
read. Ledger: Blue, 2026-09-05.

**Green: the pool is sixteen days old and still fine; the next refresh has a
date rather than a deadline.** *Reality Fracture* (`fra`, 249 cards, plus the
`frc` commander decks) releases **2026-10-02** — four days from this
regrouping — and until a refresh runs after that day the shelves cannot resolve
a released product. The health body says `pool_age_days: 16` today (bulk files
still `2026-09-13`). · *Cost of leaving it:* nothing until 10-02, then names
from a new set fail on import and search. · **Recommendation:** "Gather the
library again" on the Admin Upkeep tab in the week of 10-05 — a deployed
button now, no ssh — then read the pool file's size back once; the #472
rebuild took it 224 MB → 81 MB on 09-13 and it should hold near there. Ledger:
Green, 2026-09-19.

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
with a mechanism. · **What would have to be true:** somebody decides the wider
prose is worth a third extractor — the kit exists now, so it would be reused
rather than rewritten. · **Recommendation:** one more cycle of measured rot
and it stops being tidiness; until then this line is the reminder. Ledger:
White, 2026-08-24; narrowed Cleanup, 2026-09-05.
