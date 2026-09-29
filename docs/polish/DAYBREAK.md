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
`frc` commander decks) releases **2026-10-02** — three days from this
morning — and until a refresh runs after that day the shelves cannot resolve
a released product. The health body said `pool_age_days: 16` on 09-28 (bulk
files still `2026-09-13`). · *Cost of leaving it:* nothing until 10-02, then
names from a new set fail on import and search. · **Recommendation:** "Gather
the library again" on the Admin Upkeep tab in the week of 10-05 — a deployed
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
