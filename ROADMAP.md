# Roadmap

Direction, not history: where the project stands, what the next sessions
do, and the longer arc in one line each. What changed and why lives in git,
`docs/adr/`, and `docs/HISTORY.md`'s pointer. When this file disagrees with
`docs/polish/LEDGER.md` about open work, the ledger is right.

## Where things stand — 2026-08-24

The deployed instance is **one Go binary** serving everything: the deck
library and its five artifacts, the gate, all three simulation tiers
(goldfish, the Karsten/curve shelf, Forge), the Claude surfaces (seven
modes behind the stance dial), the tarot table, auth and the Admin page,
and both ledgers. The frontend is the committed `web_dist/` bundle. Decks
live on the instance's volume, nowhere else (ADR 30). The library's contents
are data rather than documentation — ask it (`mtglab decks list`, then
`decks validate <slug>`) rather than inheriting a count from prose.

The machine is `shared-cpu-2x`/1GB — two shared cores since 2026-08-23, for
about $0.70 a month more. It buys throughput rather than speed: a Tier 1 run
is single-goroutine, so one sweep is no faster, but the CPU job lane is
`GOMAXPROCS` wide and now runs two sims at once instead of one, and a request
arriving mid-sim no longer queues behind it. `performance-1x` is the next real
step, costs five times the whole bill, and is the one that would move a single
sweep — it wants a measured wall time behind it, never a hunch.

Performance headlines, measured 2026-08-23/24 on the dev Mac and the
instance: pool refresh ~27 s end to end; Tier 1 (20k games) ~1.9 s a core
and concurrent across cores; boot to healthy ~4 s.

## The near-term TODO

In order:

1. **The owed walks** (commandment 16): the Claude sweep and the camera
   door in a local browser with Aaron's eyes on them, then the full
   deployed walk-through. The Coliseum's gear fan is owed one too — it wants
   a live match with something equipped on the board.
2. **The combat square-off**: the attacker/blocker arrows are the last
   part of the arena Aaron is not happy with. They go, replaced by a bout
   on the centre stage against a real arena backdrop, with several blockers
   ranked on the defending side. A picture hunt plus a layout nobody has
   drawn yet (N facing one), so it is its own session.
3. **The Simulator learns, continued**: deck ratings and land-count
   regression over the match ledger (ADR 36), then the Tier 2 question —
   adversarial simulation between decks — which starts as a design
   argument, not code.
4. **Assisted refactor**: swap recommendations that argue from three
   sources with three different epistemic statuses — the gate
   (reproducible), the simulator (seeded measurement), and Claude's slot
   argument (an opinion, ADR 25). The three problems to solve first are
   recorded in git history with the original design.
5. **Open deck rulings**: two banned cards still need replacements chosen
   (Goreclaw's is deliberate — the live invalid example stays). Deck facts
   are volume facts; verify before acting.
6. **Ledger stragglers** (`docs/polish/LEDGER.md`): repository settings
   (secret scanning, push protection), uptime watching, the wrong-painter
   credit (needs a pool schema change and a refresh), cache-write tokens
   (schema v11), the phone's touch targets, region-scoped motion (Syr
   Gwyn's torch flame). Two more that nothing checks today: the measuring
   shelf's rebuild, and the `deploy` job's `needs` list.

## The longer arc

The numbered ambitions this project was started around, each now either
built or holding its place in line:

1. **Simulate decks** — built: three tiers, cached and seeded (ADR 18).
2. **Adversarial simulation** — next in line (Tier 2, above).
3. **Real games against a real engine** — built: Forge, hosted worker, and
   the Coliseum, which is now the only room that starts a match (ADR 35,
   ADR 36). The Simulator kept the arithmetic; watching moved to the room
   built for it. **The board is built too** — the listener on Forge's event
   bus is ADR 42, and the arena draws two seats, three lanes a side, the
   closed zones, the trench's three death clocks and a centre stage that
   opens a scene chosen by what kind of card arrived. What is left here is
   craft rather than capability: the combat square-off (below) is the one
   named piece of it.
4. **Shopping and swaps** — built: prices from Scryfall, swap boards, no
   checkout ever (out of scope by rule).
5. **Five artifacts per deck** — built, generated only (ADR 8).
6. **Scan upcoming sets** — built: the spoiler scan modes.
7. **Tier list of curated decks** — built: the shelf and ratings pages.
8. **Onboarding for someone new to Commander** — built and always first
   (commandment 2): the reference prose, the colour taxonomy, the theme
   interview, the tarot table (commandment 15).
9. **Shared decks and a leaderboard** — sharing built (ADR 22); the
   leaderboard waits on Tier 2.
10. **Assisted refactor** — the near-term item above.

## Standing constraints

Free forever; Wizards' Fan Content Policy is a hard boundary; no purchase
automation; no marketplace scraping; no rules engine of our own (Forge
plays the games). The full operating rules live in `CLAUDE.md` — the
Commandments — and the runbook in `docs/HOSTING.md`.
