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
> Colorless; the Cleanup section's 2026-09-19 entry says where each went).
> **Fourteen items are open as this file is written**, twelve inherited and
> two surfaced by the untap — Red's #481 walk line, which had lived only on
> its parked branch, and a merge-queue item whose ledger trigger fired today
> with four PRs open at once. Count them with the colour, never the bold:
>
> ```
> grep -cE '^\*\*(White|Blue|Black|Red|Green|Colorless):' docs/polish/DAYBREAK.md
> ```
>
> A count is a claim to re-check, and so is the recipe for checking it.

---

## Open — a walk before a merge (commandment 16)

**Red: PR #481 is green and parked — commandment 17's focus clause made
checkable, and the twenty-one controls it found dressed.** Two art-picker
tiles were genuinely silent to the keyboard (an inline `outline` outranks
every focus ring); ten classes — `.card-action`, `.menu-row`, `.nav-link`,
`.wordmark`, `.shelf-learn`, `.reader-tile`, `.art-pick-tile`,
`.wheel-folded`, `.wheel-fold-btn`, `.wheel-spin-btn` — answered hover and
left focus to the browser's default ring; all now share their hover reply
with `:focus-visible` and wear `.btn`'s vine ring. · *Cost of leaving it:*
keyboard users get the OS blue line on this site's dark rooms, and the guard
sits unmerged so the next inline `outline` lands unnoticed. ·
**Recommendation:** walk and merge. `mtglab-ui` on 8765, then **Tab, never
click** (Chrome draws `:focus-visible` for the keyboard only): `/` — Tab once
(wordmark rings, tree shivers 700ms), Tab on (each nav link rings, its
underline sprouts); gear → Tab (menu rows ring inset); any deck → Card actions
bar → Tab across; deck → Change art → Tab into the tiles (lift + ring; on
Trostani, Card art → Command Tower, the chosen tile keeps its blue ring under
a vine halo); the folded wheel at the deck's foot → Tab, Enter, Tab to Fold
away and Spin the wheel (glint crosses, 0.7s); `/new` → Help me decide →
Different reader → Tab onto a reader tile; `/` → From the shelves → Another
until "In the Learn room →" shows, Tab to it. Ledger: Red, 2026-09-19 (the
entry rides #481 itself).

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

**Green: `Deck.LandCount()` counts by category alone, so a modal DFC filed as
a spell is missing from the land count *and* from the opening-hand
arithmetic.** The deck wire's `land_count`, and `analyze.OpeningHand` which
reads it, both ask only "is the category 'land'"; `analyze.CurveOf` and
`PipRequirements` ask `IsLand()` instead. So a card like Stump Stomp //
Burnwillow Clearing filed under 'interaction' falls between them — too
land-like for the curve, not land-like enough for the land count — and the
opening-hand land probabilities are computed one land short. · *Cost of
leaving it:* three cards across two decks today; the numbers are quietly a
little pessimistic, and nothing says so. · **What would have to be true:**
`land_count` is on the wire and the opening-hand table is recorded in
`gate/testdata/*.stats.json`, so changing the rule moves frozen goldens and is
deliberately not a thing a session does on its own. · **Recommendation:**
rule that an MDFC counts toward `land_count` — it genuinely is a land drop,
and "yes" makes the two arithmetics agree — on its own branch with the
goldens re-recorded deliberately. Ledger: Green, 2026-09-19.

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

**Red: `/api/health` reports the pool and the process, never `app.db` or the
volume's free space, so a corrupt auth database or a full disk leaves it
green.** · *Cost of leaving it:* every login can fail behind a passing check.
· **Recommendation:** "yes" — add `app_db` (does it open), `disk_free_mb` and
`schema_version` to the body and keep the status 200 (Fly stops routing on a
failing check, and with one machine that turns "logins are broken" into "the
site is down"), and it becomes the next Red run's first fix rather than a
queued idea; it is a ruling only because the free-space read is
platform-shaped and the arm64 leg of CI is the only full proof, so it lands
as its own watched PR. Ledger: Red, the queued list carried in the 2026-09-05
entry, item 3; re-verified 2026-09-19.

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

**Red: a merge queue — its ledger trigger ("the next time more than two PRs
are open at once") fired today, with three Dependabot PRs and #481 open
together.** Protection is `strict`, so every merge invalidates the others and
costs each a branch update and a full re-run (~7 minutes of CI apiece,
which is what landing the Dependabot trio one at a time costs). · *Cost of
leaving it:* about
twenty CI minutes on the one morning a month Dependabot batches, and nothing
otherwise — serial rainbows never collide. · **Recommendation:** "close" —
a merge queue changes the contributor workflow to save a monthly twenty
minutes; the trigger was a threshold, not a pain. Ledger: Red, the queued
list carried in the 2026-09-05 entry, item 5; trigger fired 2026-09-19.

**Black: nine of the ten Claude mode prompts have drifted from the code around
them, and the drift is in what the model is told.** Three runs of this reading
ended "the prompts are byte-untouched, so there is nothing to read" — true of
the prompts and false of the reading, because a prompt goes stale when its code
moves. The worst three: the intake, draft and description modes all promise the
model "the counts and the curve ... were given to you" when the brief carries
neither and the two tools that do are granted but never named; nine of ten
modes end on a scope paragraph about a card, a deck or a gate that is not there
(`scan` — a two-string transcription — is told not to range into the rest of the
deck); and `theme-conversation` is told a re-stated slot set *replaces* the
previous one when the code unions them. Six smaller ones behind those. · *Cost
of leaving it:* every one of these is a sentence a model is currently believing,
on the surfaces a newcomer actually talks to, and the tarot table is one of
them. · **Recommendation:** "yes" — one branch, one PR, one walk; brief
sentences first, the scope paragraphs through each mode's own `ScopeNotes`
field (which exists for exactly this), and two of them — the interview's "three
to five questions" against a cap of six, and the theme's replace-versus-union
rule — put to you rather than guessed. Queued rather than landed because it is
all model-facing text and the dossier's instructions are hash-frozen in a
corpus. Ledger: Black, 2026-09-26.

## Open — a dollar and an account

**Red: nothing off-platform tells you the site is down, and a hung process
that keeps its port is an outage nothing detects.** Fly's HTTP check stops
routing on failure and the restart policy fires only on exit, so a wedged
process is a total outage with no alarm. · *Cost of leaving it:* the first
person to notice an outage is a friend at breakfast. · **Recommendation:**
two free minutes first — does fly-metrics.net hold any alert rule at all? —
then UptimeRobot's free tier on `GET /api/health` (GET, never HEAD: `HEAD /`
answers 405) wired to Pushover ($5 once) for the phone. Ledger: Red, the
queued list carried in the 2026-09-05 entry, items 1–2; on the queue since
09-19 only, in the ledger since 08-16.

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

**Green: the pool is six days old and fine; the next refresh has a date
rather than a deadline.** *Reality Fracture* (`fra`, 249 cards, plus the `frc`
commander decks) releases 2026-10-02, and until a refresh runs after that day
the shelves cannot resolve a released product. · *Cost of leaving it:* nothing
until 10-02, then names from a new set fail on import and search. ·
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
