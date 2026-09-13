# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one line with
a recommendation, so the whole file can be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed.

Each item: what it is · what it costs to leave it · **the recommendation.**

> **2026-09-13, morning: Aaron worked the queue with Claude.** Six items
> answered — three rulings, the watched pool refresh, #468's live listen, and
> the restore drill, which had been overdue since the ladder reached rung 17.
> Queue 12 → 6, and two of the six that remain are the same repository
> settings that were queued yesterday and the morning before. The drill and
> the refresh each turned up a measurement that changed a standing
> recommendation; both are below. **The ledger entries for the six are owed** —
> this session was not a polish run and did not improvise into another
> section's block, so the next Cleanup should carry the Answered list at the
> foot of this file into White, Green, Red and Colorless.

---

## Open — one command at the keyboard, and still not run

**Colorless: `fly` on this shell has refused its stored login since Saturday
morning, and the cause is a clock, not corruption.** Re-read 2026-09-13
17:00: `token expired (742h7m41s since login, timeout is 720h0m0s)` —
flyctl's 30-day interactive-session ceiling, clocked from deploy day's login
(Aug 13). The stored token itself still works; every leg's `FLY_API_TOKEN`
export bypasses the interactive check, which is how this morning's refresh
and the whole restore drill were driven. · *Cost of leaving it:* every
session pays the grep-export tax, and the first one that forgets reads a
healthy instance as unreachable. · **Recommendation:** run `fly auth login`
once at the keyboard — two minutes, resets the 30-day clock. Queued three
mornings running now. Ledger: Colorless, 2026-09-12.

## Open — a few clicks in the repository settings, and still not clicked

**White: the nine open torch Dependabot alerts are triaged in prose and
never dismissed on GitHub, so the security tab re-asks a settled question
forever.** The triage lives in `tools/pyproject.toml` (containment: dev-Mac
only, never ships, safetensors-only snapshot — pinned by real code since
#463). Re-read from the API 2026-09-13: still exactly 9 (1 critical, 3
medium, 5 low), all `pip/torch`, all `development` scope. They stay open
because dismissing needs repo-admin, which the pass does not have and should
not. · *Cost of leaving it:* every future security read spends the hour
re-deriving this paragraph. · **Recommendation:** dismiss all nine as
"tolerable risk — see tools/pyproject.toml's depth-extra triage" (two
minutes in the Security tab). Ledger: White, 2026-09-12.

**Red: `tools` gates the deploy now, and is still not a required check —
that half is a repository setting only you can flip.** Red's #285 fixed
`deploy`'s `needs`; the protection API answered seven contexts again on
2026-09-13 (`frontend`, `image`, `no-secrets-or-card-data`,
`dependency-review`, `go (amd64)`, `go (arm64)`, `go-lint`) and `tools` is
not among them. So a red toolbox blocks the deploy and not the merge — and
that gate holds every committed asset to its recipe, which is commandment
9's provenance half. · *Cost of leaving it:* a red `tools` still merges,
silently. · **Recommendation:**
`gh api -X POST …/protection/required_status_checks/contexts
-f 'contexts[]=tools'`, and while you are in settings, `allowed_actions` is
still `"all"` and non-provider secret patterns are still off — both free.
Ledger: Red, 2026-08-24, queued 9.

## Open — newly measured, and now worth doing

**Green: four fifths of the served card pool is dead pages, and the number
is no longer an estimate.** The deferred in-place-reload item named this
morning's refresh as its trigger, and the trigger fired hard. The refresh
grew `mtg.duckdb` 224,145,408 → 261,894,144 bytes (+16.8%) while the source
data grew 0.3%; then the restore drill rebuilt *the same rows* from scratch
on a throwaway volume — same loader, same bulk files, 35,517 oracle cards
and 108,583 printings either way — and got **54,538,240 bytes**. The served
pool is 4.8× the size it needs to be, and each refresh adds roughly 37 MB
that is not data. · *Cost of leaving it:* nothing breaks — `/data` is at
14% of 2.9 GB — but the bloat compounds every refresh, and it is paid for on
every snapshot, every restore and every boot that opens the file. · **What
would have to be true:** `pool.Refresh` builds into a temporary file and
renames over the old one, which also makes a failed refresh atomically
harmless rather than merely transactional. The manual form is already in
`docs/HOSTING.md` and was walked today. · **Recommendation:** land the
rebuild-and-rename; it is a contained change to one package with a
measurement behind it and 207 MB on the table today. Ledger: Green,
2026-09-13 — entry owed, the measurement is in the Answered list below.

**Red: the "drill older than the newest migration" rule cannot be satisfied,
and the drill that proved it is now walked.** Snapshot retention is five
days; rungs 0015–0017 landed 2026-09-06, so by 09-13 every surviving
snapshot already sat at rung 17 and the restore *read* the ladder rather
than climbing it. A snapshot can only ever exercise a migration landed in
the last five days. · *Cost of leaving it:* the rule reads as permanently
overdue, which is how a rule stops being read. · **Recommendation:** already
reworded in `docs/HOSTING.md` §Backups on this branch — drill within five
days of landing a migration, or accept that rung is never rehearsed, with
the retention-free `app.db` backup covering the rest. Nothing further owed
unless you want a post-migration drill added to the merge checklist.
Ledger: Red, 2026-09-13 — entry owed, the walk is in the Answered list below.

**Green: two curated decks warn that an MDFC's *land back* is filed under a
spell category, which may be the gate reading the wrong face.**
`one-blade-many-blessings` (Strength of the Harvest // Haven of the Harvest,
filed 'engine') and `school-of-hard-knocks` (Legion Leadership // Legion
Stronghold, 'utility'; Stump Stomp // Burnwillow Clearing, 'interaction') —
all three are modal double-faced cards whose *front* is a spell and whose
back is a land, and all three are filed by their front face, which is how a
player would file them. · *Cost of leaving it:* three standing warnings that
may be correct advice or may be a category check reading a combined type
line; nobody has looked. · **Recommendation:** worth one session's read of
the category check against a known MDFC, because if it is reading the wrong
face it is wrong everywhere, not only here. Not urgent — warnings only, zero
errors. Ledger: Green, 2026-09-13 — entry owed; noticed while re-checking the
gate claim in ruling 1 below.

## Open — watched tasks, for an hour or a seat

**Green: NINE real decks still stand in the checkout's `decks/`, and the
thing that was holding them is still running.** The eight from 08-24/25
(arahbo-cats through trostani-tokens) plus hylda-s-endless-winter, added
Sep 7. The reason to keep them expired when #436 merged 2026-09-05. ·
*Cost of leaving it:* any edit made through a local surface diverges
silently from the volume's truth. · **What would have to be true:** the
local server on 8765 — another session's, PID 19163, started 08:42 on 09-12
and **still listening at 17:00 on 09-13**, now 32 hours old — is stopped or
known to be finished with them. No session deletes files out from under a
running process it did not start. · **Recommendation:** `rm -rf decks/` in
the main checkout once 8765 is down; future local walks pull fresh from the
instance or point `MTGLAB_DECKS_DIR` at a scratch directory. Ledger: Green,
2026-09-05; re-verified through 2026-09-13.

## Open — deliberately waiting, nothing to do yet

**Blue: the Settings room says "the torches are not lit yet", and the only
thing keeping that true is that you have not flipped the switch.** The line
is hand-written into the bundle (`web/src/routes/Settings.tsx`) and true
today — the instance has no `MTGLAB_NIGHT_WINDOW` — but the evening you set
the five night secrets changes no code and rebuilds nothing, so the room
would keep telling people the arena is dark while it fights. · *Cost of
leaving it:* a small untruth on the one page where a person decides to enter
their decks. · **What would have to be true:** the Coliseum's night shelf
lands (ADR 46 names it as its own PR) and the settings room reads whether a
night is scheduled off the wire. · **Recommendation:** unchanged — the copy
becomes a fact the server owns when the shelf gives it something to read.
This line is the reminder. Ledger: Blue, 2026-09-05.

**White: `NOTICE.md` is held and the skills are held; the rest of the tree's
prose is still unguarded.** The narrowed remains of the docs-rot question.
`licenserecord_test.go` holds every repository path and `mtglab` verb the
licensing record names, `skillrecord_test.go` (#440) holds `.claude/skills/`,
and #469 moved the shared kit to `recordkit_test.go`. What none of them
reads is `docs/`, `web/README.md` and the package comments. · **This morning
made the case sharper, twice:** `docs/HOSTING.md` was found carrying two
bullets in one section that contradicted each other about
`auto_stop_machines`, and `CLAUDE.md`'s own standing fact about a
deliberately-invalid deck had quietly stopped being true — both fixed on
this branch, neither caught by anything but a person reading. · *Cost of
leaving it:* nothing legal; this is tidiness with a mechanism. · **What
would have to be true:** somebody decides the wider prose is worth a third
extractor — the kit exists now, so it would be reused rather than rewritten.
· **Recommendation:** the two rot instances found today are the first
measured evidence rather than assumed; one more cycle of that and it stops
being tidiness. Ledger: White, 2026-08-24; narrowed Cleanup, 2026-09-05.

---

## Answered — 2026-09-13, owed to the ledger

*(Aaron ruled; the next Cleanup run carries these into the color sections
and deletes them from here.)*

1. **White — the live invalid example.** *Ruled: the fact is retired.* Read
   off the deployed volume: 25 curated decks, `0 error(s)` on every one, two
   carrying warnings only. `CLAUDE.md`'s standing fact removed on this
   branch and replaced with a note saying it was retired and why, so a future
   session finding an invalid curated deck knows it is a real problem rather
   than the promised fixture.

2. **Colorless — the comment ratchet.** *Ruled: build it.* Landed this
   branch as `go/cmd/mtglab/datedcomments_test.go`. Ceilings measured
   2026-09-13: **114** in `go/` (Go's own comment scanner, `_test.go`
   excluded) and **292** in `web/src` (comment-led lines). The definition is
   pinned in code precisely because it could not be agreed by sweep — three
   defensible regexes over the same tree the same morning returned 114, 154
   and 479 for the Go side. Rise side has no slack; the fall side banks a
   material gain only (slack 10), because a ceiling needing a re-type on
   every branch is a tripwire that gets deleted. All three arms driven.

3. **Colorless — the stale worktree.** *Ruled: remove it.* Gone.
   `frosty-roentgen-e3fccf` was clean and detached at c57f4e7; after removal
   `find` and `git ls-files` agree at 34 recipe files, which confirms the
   phantom-67 diagnosis exactly.

4. **Green — the watched pool refresh.** *Done, 41 seconds, exit 0.* Pool at
   35,517 oracle cards / 108,583 printings, both bulk files 2026-09-13,
   `pool_stale: false`, `swept 2 older bulk files (102,573,381 bytes freed)`.
   The Zeta Set is in at the printing level — three `slz` rows for Arcane
   Signet with art crops, `promo: false`, released 2026-09-02, so
   `pool.ArtFor`'s earliest-printing rule is untouched and no deck's art
   moved. **One correction to the queued item:** it claimed imports and
   search would "fail to resolve released cards"; all 363 Zeta Set cards are
   reprints (`e:slz -is:reprint` is empty), so every one already resolved by
   name. What the refresh actually bought was printings, prices and a
   fortnight of oracle drift. The size finding is promoted to its own open
   item above.

5. **Green — #468's live listen.** *Done, and wider than the beat asked.*
   All 15 labels across `/simulate`, the Coliseum duel and the Coliseum
   four-player panel resolve to a real control (`label.control` populated on
   every one). Behaviourally: the GAMES caption focuses `INPUT#_r_2_`, the
   CHAMPION caption focuses `SELECT#_r_0_`, and the `?` bubble is a button
   carrying `aria-expanded="true"` that opens its help without taking focus
   into the field — commandment 20 satisfied, not merely #468's fix.

6. **Red — the volume restore drill.** *Walked, 4m49s wall clock; recovery
   time 96 seconds.* Into a throwaway app (`sylvan-library-drill`), never
   near `mtglab_data`; destroyed at the end, production verified healthy
   after. Proved: `schema=17 pool=present` on the restored volume, 25 deck
   directories back with four sampled decks validating `0 error(s)`, all
   three accounts with state and admin marker intact, ext4 replaying the
   snapshot's journal cleanly on mount, and a from-nothing pool rebuild in
   25 seconds. Procedure, commands and the "set no secrets on the drill app"
   warning are now `docs/HOSTING.md` §Backups. The retention-versus-ladder
   finding is promoted to its own open item above.
