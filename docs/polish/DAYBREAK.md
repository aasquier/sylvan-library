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

## Open — 2026-09-05

**White/Blue: the once-per-cycle determinism replay is still owed, and one
sign-in unlocks it and two more besides.** The tarot deal, the Wheel's spin
and every Tier 1 result are seeded promises, and the way we prove a deploy
kept them is to replay them against the deployed instance — which answers 401
before a login, and Claude never signs in to anything. The same 401 is why
Blue's two owed walks (the fortune-teller's table, the `/claude` keeper duty)
have now been skipped three runs running. · *Cost of leaving it:* the one
contract the project calls a promise is checked only on this laptop, and
commandment 14 says a green suite has not seen the page. · **What would have
to be true:** the `claude` seat signed in through Claude-in-Chrome once, in an
evening you are at the keyboard; the next White run rides the session and the
walks come with it. · **Recommendation:** unchanged — sign it in when it is
convenient; nothing else here is waiting on anything. Ledger: White,
2026-09-05.

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

**Red: the Anthropic key expires in about five days (~2026-09-10), and from
then a 401 on every Claude surface will read as a broken integration while
actually being this date.** Renewal is two steps and minutes: a fresh key from
the Anthropic console, then `fly secrets set ANTHROPIC_API_KEY=…` — the set
restarts the machine (seconds of downtime, same as any deploy). · *Cost of
leaving it:* every Claude room on the site goes dark mid-week with a
misleading error, and the next session debugs an integration that is merely
lapsed. · **What would have to be true:** nothing but the rotation. It stays
in the queue because **nothing here can see whether it happened** — checked
2026-09-05, `fly secrets list` prints names and digests and no dates, so the
key's age is not a fact this file can read. · **Recommendation:** rotate it
before the 10th and say so; everything else on the calendar is comfortable
(TLS 2026-11-11 Fly-renews, domain and `FLY_API_TOKEN` August 2027). Ledger:
Red, 2026-09-05.

**Red: the coverage floor is computed twice per run and you ruled it should be
computed once — the ruling is recorded, the change is not made.** The
`Coverage floor` step runs the whole suite a second time on **both** matrix
legs (84s amd64, 47s arm64) for a number that cannot differ between them: the
tree holds zero arch-tagged non-test Go files. · *Cost of leaving it:* ~84
seconds on every push and pull request, forever, growing with the suite. ·
**What would have to be true:** a quiet pipeline. A `ci.yml` semantics change
is only provable by CI itself — one branch, one watched run — and the cleanup
phase ran against a merge train that had five branches queued behind it, so a
change to what a required gate measures could not be watched honestly. ·
**Recommendation:** yes, unchanged — gate the step on the arm64 leg (`if:
matrix.arch == 'arm64'` or the file's equivalent), on its own branch, on a
morning when the queue is empty. Ledger: Red, 2026-09-05.

**Green: eight real decks still stand in the checkout's `decks/`, and the
thing that was holding them is gone.** arahbo-cats through trostani-tokens,
`deck.yaml` mtimes 08-24/25 — the laptop-standing-copy shape that lost two
rounds of labels and created the hosted-first facet. The reason to keep them
was Red's held walk; **#436 merged 2026-09-05**, so that reason has expired. ·
*Cost of leaving it:* any edit made through a local surface diverges silently
from the volume's truth, and nothing fails when it does. · **What would have
to be true:** the local server on 8765 — another session's, up since Aug 28
and answering 200 as of tonight — is stopped or known to be finished with
them. Cleanup did not delete files out from under a running process it did not
start, which is the same rule that keeps this phase off the main checkout
during a merge train. · **Recommendation:** `rm -rf decks/` in the main
checkout once 8765 is down; future local walks pull fresh from the instance or
point `MTGLAB_DECKS_DIR` at a scratch directory. Ledger: Green, 2026-09-05.

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
crosses it; the ladder is at rung 14 and no drill has crossed any of it.
Snapshots are healthy — five, 5-day retention. · *Cost of leaving it:* the
library's one standing copy (ADR 30) is behind a procedure nobody has ever
run, and five days is all the retention there is. · **What would have to be
true:** an hour, and a **scratch** volume forked from the newest snapshot
attached to a throwaway machine — never `mtglab_data`. Cleanup will not fork a
volume unwatched, and no test can stand in for the walk. · **Recommendation:**
walk it once and date it in `docs/HOSTING.md` §5. Cents of volume for an hour.
Ledger: Red, 2026-08-24, queued 11.

## Open — 2026-08-23

**6. A pprof mount, so the hot-spot patrol can profile the serving process
itself.** Two halves, separable, and you said yes to (a): **dev-local**,
mounting `net/http/pprof` only when auth is off — a laptop-only surface that
lets a patrol profile `mtglab ui` under real request-shaped load. Not built by
cleanup: it is a change to how the door assembles itself, which is a Blue or
Colorless run's work rather than a queue-emptying one, and the tree still
contains no pprof (checked 2026-09-05). (b) **live** — the same mount
admin-gated behind ADR 17 — remains yours: **heap profiles carry process
memory**, and this process's memory holds session tokens and Argon2id
parameters, so live would mean CPU-profile-only, and commandment 10 keeps it
invisible to users either way. · *Cost of leaving it:* the patrol reads
test-shaped load and outside clocks, which is honest but blind to
request-shaped hot spots. · **What would have to be true:** for (a), a colored
run picking it up — it is perhaps forty lines and one boot-time condition. For
(b), a hot spot the local mount cannot explain. · **Recommendation:** (a) yes,
still; (b) can keep waiting. Ledger: Colorless, 2026-08-24 (recovered
records).

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

*(Answered items move here in one line with the ruling and the date, then out
entirely once the ledger carries them.)*
