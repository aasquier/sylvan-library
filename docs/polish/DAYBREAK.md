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

> **2026-09-24, evening: the coverage climb queued nine rulings.** Nine lanes
> took the Go tree from 91.2% to over 96 and every serial test to parallel in
> one day (#488–#496 and the floor PR after them; `COVERAGE.md` is the map).
> Each lane was told to pin what it found rather than fix it, and the items
> below are what they pinned. None blocks anything; all are one word.

**Blue: `auth.exclusive` can hand a poisoned connection back to the pool.**
It opens with a hand-written `BEGIN IMMEDIATE` on a pinned connection; when
the ROLLBACK or the COMMIT *also* fails it returns that connection with the
transaction still open, so every later statement on it runs inside a
transaction nobody will commit and the next `BEGIN IMMEDIATE` is refused — on
a live instance, a handle that keeps answering reads while writing nothing.
`SetPassword` beside it returns a `revoked` count about work that was rolled
back. · *Cost of leaving it:* a volume hiccup mid-write becomes a silently
read-only account store until restart. · **Recommendation:** on a failed
rollback or commit, mark the connection bad through `Conn.Raw` (the driver
supports it) so the pool discards it; return zero beside an error. A
behaviour change in `internal/auth/writes.go`, one function. Held by
`clearStaleTransaction` in `halfwritten_test.go`.

**Blue: both ledgers' `NewRecorder` open `app.db` without a ping**, so a
missing file is found at the first write, and the Claude one warns rather
than fails — on an instance whose volume did not mount, conversations happen,
cost money and are not recorded. · *Cost:* the usage ledger reads empty for a
night nobody noticed. · **Recommendation:** `auth.PingWritable` at open for
the Claude recorder (a conversation that cannot be recorded should not
start); keep the tier3 one lazy, since a read must never acquire a database.
Held by `TestARecorderOverAMissingDatabaseOpensAndDiscoversItLater`.

**Green: three sentences a player reads are pinned as wrong.** (1) A swap
that trades one chosen colour for another is refused because `playableCard`
checks reach over the deck *including* the outgoing card (`edits.go`); the
deck it would produce is legal. (2) An unreadable `artifacts/` directory
answers `[]` and `baseline: "unknown"`, the words a never-built deck gets
(`library/source.go`). (3) The intake's slot sweep prints "0 of 2" with
nothing said when every call was refused rather than unavailable
(`intake.go`). · *Cost:* each is a small lie to a newcomer (commandment 2).
· **Recommendation:** yes to all three — check the swap against the deck
after the removal; surface the unreadable shelf as a fault; count refusals on
the sweep line. Each is a few lines and each moves recorded copy, which is
why no lane took them.

**Blue: `mtglab data snapshot` over a pool with none of the pool's tables
prints `snapshotted 0 prices for today` and exits 0** — `SnapshotPrices`
creates what it needs rather than refusing. · *Cost:* the same "did it lie"
shape the climb closed elsewhere, on the one command a cron runs. ·
**Recommendation:** refuse when `printings` is absent, in the same words
`data refresh` uses for a pool it cannot read.

**Blue: two things with no caller.** `tier3.Defaults()` — `LoadSettingsFrom`
uses `DefaultsFrom` now — kept only as the footing `Settings`' doc comment
points at; and `strictB64Decode`'s all-padding guard (`claude/scan.go`),
which every string that could reach it is refused before. · *Cost:* nothing
today; dead API reads as live. · **Recommendation:** delete both.

**Green: three fixture rows from the real pool.** `coliseum.go`'s arena
backdrop and champions (`Grand Coliseum`, `Jareth, Leonine Titan` and
friends), `simShelfCommand`'s `Approximated` tail (any two-colour card), and
`gate/rulebreaker.go` (a real ADR 51 clause) are the last branches that want
a real card in the 21-card `tiny_pool.json`. · *Cost:* ~15 statements and,
more to the point, the Coliseum's own room is the one route whose art
resolution nothing drives. · **Recommendation:** a session with the pool
copies the rows from it (`cards show`, never from memory) and re-checks the
tests that count what is in there.

**Blue: two `yamlemit` tests call a writer directly rather than through a
spelling** — `writePlain`'s break handling and `roundTrips`' error answer —
because the style's analysis never offers plain to a multi-line scalar and
`Render` folds only strings. Both say so where they stand. · *Cost:* if that
reads as calling past a guard, seven statements. · **Recommendation:** keep
them; they are units the package comment names as carrying the style.

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

## Open — a migration window

**Black: prompt-cache *writes* are invisible in both usage ledgers, so the
spend figures are a little under.** `cache_creation_input_tokens` appears
nowhere outside two test fixtures; writes bill at 1.25× input. · *Cost of
leaving it:* the dollar figure on the Admin panel and in `mtglab claude
usage` under-reads by the write premium — small against today's ≈$11
all-time, and a schema migration to fix. · **Recommendation:** add the
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
