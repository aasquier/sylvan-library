# Daybreak

The morning read. The polish pass runs at night (`.claude/skills/polish/`,
"Nightbound"); anything it could not settle alone lands here as one line with
a recommendation, so the whole file can be answered with *yes to all*.

**This is a queue, not a record.** An answered item leaves — its outcome goes
into `LEDGER.md`'s own section. A file that only grows stops being opened,
which is exactly how the per-color queues it replaces failed.

Each item: what it is · what it costs to leave it · **the recommendation.**

> **2026-09-05: Aaron answered the whole file, yes to all.** The cleanup phase
> ran it out over five branches; sixteen of twenty-five items left, and the
> nine below are what is genuinely still in hand. Every one of them now says
> *what would have to be true* for the next cleanup to land it, because
> "deferred" twice is a finding about the phase rather than about the item.
> The whole accounting — before, after, who landed what — is the ledger's
> Cleanup entry for 2026-09-05.

---

## Open — 2026-09-12

**White: the live invalid example is gone — all 25 decks now pass the gate,
and CLAUDE.md's standing fact ("at least one curated deck fails the gate on
purpose, a banned card left as the gate's one honest live demonstration") has
quietly stopped being true.** Read off the deployed instance's own deck wire
this run: 25 curated, `errors: 0` on every one (two carry warnings only). The
library grew 17 → 25 since the last check confirmed one failing deck, and
somewhere in the growth the demonstration left. Decks are your data — no run
touches them, and no CI gate can see the volume. · *Cost of leaving it:*
nothing breaks; the gate just has no live proof it refuses anything, and the
prose promises one. · **Recommendation:** drop a banned card back into one
deck you like for the job (a minute in the deck editor), or rule the fact
retired and CLAUDE.md gets the sentence removed on the next working branch.
Ledger: White, 2026-09-12.

**Green: #468 merged at ~04:00 on your direct instruction — six fields
answer to their names and the door keeps hold of the keyboard — and one
beat of commandment 14 is still owed: an authenticated look at the
relabeled fields on the live site.** What stands verified: the six
`htmlFor` fixes in a real browser pre-merge, and the `/signin` focus catch
on the deployed door post-merge. The relabeled fields live on `/simulate`
and `/coliseum`, behind the login, so the live half waits on a signed-in
seat. · *Cost of leaving it:* nothing likely — the diff was four files and
the pattern uniform — but the live page has not been heard by a reader
since the labels moved. · **Recommendation:** next signed-in session, click
the GAMES caption on `/simulate` live — focus lands in the field, the
bubble's ? still opens the help. Thirty seconds. Ledger: Green, 2026-09-12
(the correction note under its heading carries the merge record).

**Green: the pool refresh is due — 13 days old tonight, across the two-week
line on Monday, and a released product is already invisible to it.** Scryfall
shows 363 paper cards released 2026-09-02 (`slz`) that the 08-30 bulk files
cannot know, with Reality Fracture's preview season about to start
(2026-10-02 release); `pool_stale: false` the whole time, because that flag
reads schema, not age. · *Cost of leaving it:* imports and search quietly
fail to resolve released cards, and legality ages. · **Recommendation:** run
the refresh from HOSTING this week, watched — and read `mtg.duckdb`'s size
after: it is 224,145,408 bytes tonight, and the deferred in-place-reload
item's trigger is exactly this refresh (materially past 214MB → the
rebuild-to-temp-and-rename earns its diff; a plateau → that entry closes).
Ledger: Green, 2026-09-12.

**Green: the local gauntlet's `go test -race ./...` can report a *cached*
green for the ten bundle guards after a bundle rebuild — proven, not
presumed.** Go's test cache does not track reads outside the module, and
`web_dist` sits outside `go/`; an experiment tonight showed `ok (cached)`
standing after the read file was changed to one that fails the assertion.
CI is immune (its gate already runs `-count=1`, ci.yml:218) — only the
laptop ritual is exposed, on exactly the tests that pin the Safari floor,
reduced motion and card-imagery compliance. · *Cost of leaving it:* a
session edits CSS, rebuilds, sees green locally, and CI is a surprise —
the thing commandment 11 forbids. · **Recommendation:** teach CLAUDE.md's
gate line the narrow form — add `go test -race -count=1 ./cmd/mtglab/`
after any bundle rebuild (the polish checklist already carries it) rather
than blanket `-count=1`, which would cost minutes of recompute on every
gauntlet CI happily pays but a laptop should not. Ledger: Green,
2026-09-12.

**White: the nine open torch Dependabot alerts are triaged in prose and
never dismissed on GitHub, so the security tab re-asks a settled question
forever.** The triage lives in `tools/pyproject.toml` (containment: dev-Mac
only, never ships, safetensors-only snapshot — and this run made the
snapshot pin real code, so the premise now holds by test rather than by
habit). The alerts (1 critical, 8 lesser, all `pip/torch`, all `development`
scope) stay open because dismissing needs repo-admin, which the pass does
not have and should not. · *Cost of leaving it:* every future security read
spends the hour re-deriving this paragraph. · **Recommendation:** dismiss
all nine as "tolerable risk — see tools/pyproject.toml's depth-extra triage"
(two minutes in the Security tab). Ledger: White, 2026-09-12.

**Colorless: `fly` on this shell has refused its stored login since Saturday
morning, and the cause is a clock, not corruption.** `LOG_LEVEL=debug fly
auth whoami` says it plainly: `token expired (740h38m since login, timeout
is 720h0m0s)` — flyctl's 30-day interactive-session ceiling, clocked from
deploy day's login (Aug 13), lapsed 09-12 ~08:00. The stored token itself
still works; every leg's `FLY_API_TOKEN` export bypasses the interactive
check, which is why the workaround holds. · *Cost of leaving it:* every
session pays the grep-export tax, and the first one that forgets reads a
healthy instance as unreachable. · **Recommendation:** run `fly auth login`
once at the keyboard — two minutes, resets the 30-day clock. Ledger:
Colorless, 2026-09-12.

**Colorless: the comment sweep is losing to the tree by arithmetic — one
slice retires ~20 dated lines a cycle and this week added ~50 (go 183 → 235,
web/src 368 → 426) — and the only fix that scales is a ceiling, which would
bind every future session and is therefore yours.** A ratchet test over
dated comments outside tests (the count may not rise; a session adding a
date-is-the-fact comment bumps the ceiling consciously, the test saying how
to decide) is the coverage-floor pattern applied to prose residue. · *Cost
of leaving it:* the residue grows without bound and the sweep becomes
ritual. · **Recommendation:** yes to the ratchet — Colorless builds it next
run; or rule the totals advisory and the sweep keeps its judgment-only
shape. Ledger: Colorless, 2026-09-12.

**Colorless: one clean, week-stale harness worktree survives at
`.claude/worktrees/frosty-roentgen-e3fccf`** (detached at c57f4e7, Sep 6,
`git status` empty — the auto-clean that should have removed it did not),
and its 33 duplicate recipes are why a `find`-shaped count reads 67 recipe
files where the tree tracks 34. A relic is a decision, never a silent
deletion. · *Cost of leaving it:* phantom rows in every future count, and a
seed for the known worktree Spotlight storm. · **Recommendation:**
`git worktree remove .claude/worktrees/frosty-roentgen-e3fccf` — it is clean
and unreferenced, so nothing is lost. Ledger: Colorless, 2026-09-12.

## Open — 2026-09-05

**Blue: the Settings room says "the torches are not lit yet", and the only
thing keeping that true is that you have not flipped the switch.** The line is
hand-written into the bundle (`web/src/routes/Settings.tsx`) and true today —
the instance has no `MTGLAB_NIGHT_WINDOW` — but the evening you set the five
night secrets changes no code and rebuilds nothing, so the room would keep
telling people the arena is dark while it fights, and nothing fails when it
starts lying. · *Cost of leaving it:* a small untruth on the one page where a
person decides to enter their decks, starting the first scheduled night. ·
**What would have to be true:** the Coliseum's night shelf lands (ADR 46 names
it as its own PR) and the settings room reads whether a night is scheduled off
the wire. · **Recommendation:** unchanged, and deliberately not landed by
cleanup — the copy becomes a fact the server owns when the shelf gives it
something to read, and doing it before that would be a second hand-written
promise. This line is the reminder. Ledger: Blue, 2026-09-05.

**Green: NINE real decks now stand in the checkout's `decks/` — the pile is
growing, not draining — and the thing that was holding them is gone.** The
eight from 08-24/25 (arahbo-cats through trostani-tokens) plus
**hylda-s-endless-winter, added Sep 7** — the laptop-standing-copy shape that
lost two rounds of labels and created the hosted-first facet, and it gained a
deck since this line was written. The reason to keep them was Red's held
walk; **#436 merged 2026-09-05**, so that reason has expired. · *Cost of
leaving it:* any edit made through a local surface diverges silently from the
volume's truth — and the growth proves local surfaces are still being used. ·
**What would have to be true:** the local server on 8765 — another session's,
PID 19163, started fresh at 08:42 on 09-12 (`ps lstart`; the Aug-28 server is
gone, so local servers keep being *started*, which is the point) and writing
`data/`'s WAL that evening — is stopped or known to be finished with them. No
night run deletes files out from under a running process it did not start. ·
**Recommendation:** `rm -rf decks/` in the main checkout once 8765 is down;
future local walks pull fresh from the instance or point `MTGLAB_DECKS_DIR`
at a scratch directory. Ledger: Green, 2026-09-05; re-read and sharpened
2026-09-12.

## Open — 2026-08-24

**White 2. `NOTICE.md` is held and the skills are held; the rest of the tree's
prose is still unguarded.** This is the narrowed remains of the docs-rot
question. `go/cmd/mtglab/licenserecord_test.go` holds every repository path
and `mtglab` verb the licensing record names, and **#440 added
`skillrecord_test.go`**, which does the same for every path and verb under
`.claude/skills/`. What neither reads is `docs/`, `web/README.md` and the
package comments — where the same rot is visible and where a dead path costs a
reader minutes rather than a licence. · *Cost of leaving it:* nothing legal;
the two records that carry obligations are guarded. This is tidiness with a
mechanism, which is why it keeps surviving triage. · **What would have to be
true:** somebody decides the wider prose is worth a third extractor — the two
helpers already exist and would be reused rather than rewritten. ·
**Recommendation:** leave it open one more cycle and let the next Colorless
run say whether `docs/` has actually rotted since the crossing, measured
rather than assumed; a guard nobody needs is its own kind of debt. Ledger:
White, 2026-08-24; narrowed Cleanup, 2026-09-05.

**Red 2. `tools` gates the deploy now, and is still not a required check —
that half is a repository setting only you can flip.** Red's #285 fixed
`deploy`'s `needs`; the protection API still answers seven contexts and
`tools` is not among them (read back 2026-09-05: `frontend`, `image`,
`no-secrets-or-card-data`, `dependency-review`, `go (amd64)`, `go (arm64)`,
`go-lint`). So a red toolbox blocks the deploy and not the merge — and that
gate holds every committed asset to its recipe, which is commandment 9's
provenance half. · *Cost of leaving it:* a red `tools` still merges, silently.
· **What would have to be true:** one API call from an account with admin
rights on the repository, which the pass does not have and should not. ·
**Recommendation:** `gh api -X POST …/protection/required_status_checks/contexts
-f 'contexts[]=tools'`, and while you are in settings, `allowed_actions` is
still `"all"` and non-provider secret patterns are still off — both free.
Ledger: Red, 2026-08-24, queued 9.

**Red 4. The volume restore drill has still never been walked, and the ladder
it would cross grew again.** A drill older than the newest schema migration is
due by Red's own rule, because the ladder is forward-only and a restore
crosses it; the ladder is at rung 17 (0015–0017 landed 09-06 with the
Coliseum's records) and no drill has crossed any of it.
Snapshots are healthy — five, 5-day retention. · *Cost of leaving it:* the
library's one standing copy (ADR 30) is behind a procedure nobody has ever
run, and five days is all the retention there is. · **What would have to be
true:** an hour, and a **scratch** volume forked from the newest snapshot
attached to a throwaway machine — never `mtglab_data`. Cleanup will not fork a
volume unwatched, and no test can stand in for the walk. · **Recommendation:**
walk it once and date it in `docs/HOSTING.md` §5. Cents of volume for an hour.
Ledger: Red, 2026-08-24, queued 11.

---

## Answered

- **Mutation testing: adopt `gremlins`** — yes, 2026-08-23. Installed on
  demand (`go install github.com/go-gremlins/gremlins/cmd/gremlins@latest`),
  no `go.mod` entry, one package at a time, determinism kernels first, never
  `internal/api`. The protocol now lives in the skill (White's testing facet);
  `docs/ENGINEERING.md` names it as the project's mutation tool.
- **Everything Aaron answered on 2026-09-05** — sixteen items, including two
  he delegated back (the Safari claim, and what `mtglab claude usage` prints).
  They are out of this file entirely because the ledger carries them: see the
  Cleanup section's 2026-09-05 entry for the full accounting, and each color's
  own 2026-09-05 (cleanup) block for the outcome.
- **Red's coverage-floor double-computation** — landed 2026-09-12 (Red) as
  #466 and proven on the watched main run: the floor computes once, on the
  arm64 leg (51s, printing 91.3% against 90.8), the amd64 leg — the critical
  path — skips it (~84s back per run), and the premise is a test now
  (`TestEveryGoFileCompilesOnBothCILegs`). Ledger: Red, 2026-09-12.
- **The Anthropic key** — rotated by Aaron 2026-09-12, lasts through
  year-end; `fly secrets` still prints no dates, but the digest is on the
  ledger now, so the *next* rotation is visible as a delta. Ledger: Red,
  2026-09-12.
- **Blue's two owed walks** — done 2026-09-12 by the Blue rainbow leg, riding
  the signed-in seat: the fortune-teller's table is still the belle of the
  ball and the `/claude` page is in good order, its gallery credits
  re-verified against the pool. Ledger: Blue, 2026-09-12.
- **The pprof mount, half (a)** — landed 2026-09-12 (Blue): `/debug/pprof/`
  mounts in front of the door only when auth is off, mutation-verified both
  ways. Half (b) (live, admin-gated) is a *deferred* ledger item now, its
  trigger a hot spot the local mount cannot explain. Ledger: Blue,
  2026-09-12.

*(Answered items move here in one line with the ruling and the date, then out
entirely once the ledger carries them.)*
