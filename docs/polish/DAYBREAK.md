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
