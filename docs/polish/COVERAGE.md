# The coverage floor

The Python suite carried a 95% gate. The Go port did not inherit it — CI ran
`-cover` and threw the number away — so coverage drifted with nobody watching.
PR #290 put a gate back and did the work to stand on it.

This file is the working map for the rest of the climb. It is a claim to
re-check, not a fact to inherit: **re-measure before trusting a number here.**
Two of this file's own claims have already been wrong; both are corrected below
and both are recorded, because the way they were wrong is the useful part.

## How the number is measured

**There are two formulas and they do not agree.** This is the first correction,
and it matters because one of them is the gate:

```bash
# What CI gates on. This is the authoritative number.
go test -count=1 -coverprofile=coverage.out -coverpkg=./... ./...
go tool cover -func=coverage.out | awk '/^total:/ {print $NF}'
```

`-coverpkg=./...` is load-bearing. Without it each package is measured only
against its own test binary, which answers a different question — how well a
package tests *itself* — and reads 0% for a package whose behaviour is
exercised entirely through its callers.

The consequence nobody had noticed: with `-coverpkg`, every test binary emits
every block, so the profile holds forty-six copies of each. `go tool cover
-func` and a hand merge fold those duplicates differently and land **a tenth or
two apart** — at the same commit, `-func` read 90.1% where a hand merge read
89.8%. Neither is wrong; they are different roundings of the same data. This
file used to quote the merge while `ci.yml` gated on `-func`, which is how a
floor and a map can drift apart without either being edited.

The per-file breakdown, which is what to work from — merge first, and read it
as a map rather than as the score:

```bash
tail -n +2 coverage.out | awk '{k=$1" "$2; n[k]=$2; c[k]+=$3} END {for(x in n) print x, n[x], c[x]}' \
  | awk '{split($1,a,":"); f=a[1]; sub(/^github.com\/aasquier\/sylvan-library\/go\//,"",f);
          t[f]+=$2; if($3>0) v[f]+=$2}
     END {for(k in t) if(t[k]-v[k]>0) printf "%6.1f%%  miss=%4d  %s\n", 100*v[k]/t[k], t[k]-v[k], k}' \
  | sort -k2 -t= -rn
```

**Work from functions, not files.** A file with 41 missing statements sounds
like a lever and is usually a long tail: `internal/api/edits.go` reads as the
sixth-worst file in the tree and is fourteen functions each missing one to six
statements. Rank by function instead — merge the profile, then attribute each
uncovered block to the function it falls in — and the difference between a
lever and a grind is visible before the work starts.

## Where it stands

| | `go tool cover -func` | merged |
|---|---|---|
| PR #290 | 90.1% | 89.81% |
| this pass | **90.4%** | 90.40% |
| floor in `ci.yml` | **90.0** | |

To reach 95%, roughly 740 more statements. Of what is left, about 210 needs a
JVM or a live network (below). **This is now a grind and not a hunt**: the
sweeps are spent, and what remains is one to six statements per function across
a few hundred functions. Budget accordingly — this pass moved ~145 statements
across nine test files, and that was the cheap end.

## What the floor cannot reach

Named here and in `ci.yml` so nobody has to rediscover it:

| where | what | statements |
|---|---|---|
| `internal/sim/tier3/run.go` | `RunGames`'s body, `spawn` | ~85 |
| `cmd/mtglab/shim.go` | `match`, `matchStreamed`, `watchdog` (calls `os.Exit`) | ~45 |
| `cmd/mtglab/sim.go` | `sim forge`'s own reporting | ~31 |
| `cmd/mtglab/data.go` | `refresh`, which downloads from Scryfall | ~28 |

**Their seams are covered.** The worker client and the shim door both run
against stubs over real HTTP; `pool.DownloadBulkFrom` runs against a stub
Scryfall; every refusal path around all of them is driven. What is missing is
the call itself.

`claude check` **was on this list and is not any more.** It called
`claude.EndpointFromEnv()` directly, so the only way to run the report was to
spend a real call. ADR 40 made the endpoint an argument to the command tree;
`cmd/mtglab/claudecheck_test.go` now drives the whole report — open pipe,
refused key, `--tools` roster — against a stub over real HTTP.

## The remaining work, biggest first

By **function**, re-measured this pass. Everything JVM- or network-blocked is
omitted:

| miss | where | shape of the work |
|---|---|---|
| 19 | `internal/gate/validate.go:checkCompanion` | the companion-in-the-99 branch and the violation renderer, including the "and N more" truncation past six. `internal/gate/companioncheck_test.go` has the synthetic-record pattern to extend |
| 18 | `internal/api/lifecycle.go:importDeck` | — |
| 18 | `internal/api/lifecycle.go:createDeck` | both need **a companion or a partner pair in the 21-card pool**, which it does not have. The fixture is `internal/pool/pooltest`'s embedded JSON; adding to it is a decision, not a patch, because other tests count what is in there |
| 13 | `internal/deckyaml/deckyaml.go:orderedValue` | the value shapes the emitter has not been handed |
| 12 | `internal/pool/loaders.go:load` | — |
| 12 | `internal/deckread/commander.go:CommanderDossier` | — |
| 12 | `internal/claude/canonjson.go:writeValue` | the branches of the canonical encoder no fixture reaches |
| 11 | `internal/api/wheel.go:readOptionalBody` | a malformed or absent body on the wheel route |
| 10 | `internal/pool/refresh.go:DownloadBulkFrom` | the stub Scryfall exists; these are its failure shapes |
| 10 | `internal/deck/deck.go:cardFrom` | — |
| 10 | `internal/api/admin.go:inviteAccount` | invite failures with a stubbed sender |
| ~9 each | `internal/api/jobruns.go:listJobs`, `internal/gate/validate.go:Validate`, `internal/claude/tools/handlers.go:searchCards` | — |
| ~1,300 | everything else | one to six statements per function; the last four points live here |

## Levers that worked

Worth reaching for again before writing anything bespoke. The first four are
from #290; the rest are this pass.

1. **A closed handle.** `internal/auth/errorpaths_test.go` covers ~40 error
   branches by closing a real migrated database and calling everything. The
   assertion is not the message — SQLite's wording is not ours — but that an
   error comes back *at all* and nothing claims success.
2. **An unreadable directory.** `internal/api/unreadable_test.go` chmods the
   library to `0o000` and sweeps every deck route.
3. **A route sweep.** `internal/api/refusals_test.go` asks two questions of
   every route in a table rather than spot-checking.
4. **A stub over real HTTP.** `internal/sim/tier3/worker_test.go` and
   `cmd/mtglab/shimdoor_test.go` drive both halves of ADR 35 against
   `httptest.Server`.
5. **A closed handle, one layer up.** `internal/api/closeddb_test.go` closes
   both `app.db` handles under a built API and sweeps every GET plus every
   admin and account route. What it asks is not "did it 500" but **"did it
   lie"** — a 200 carrying `[]` over a database that has gone reads as "you
   have no accounts", which is a different sentence from "I cannot read your
   accounts" and the only one of the two that is false.
6. **An unmountable volume, at the CLI.** `cmd/mtglab/unmounted_test.go` points
   a `deployment` at `/nonexistent` and drives every leaf command the tree has.
   Every `users` subcommand opens `app.db` on its first line, so one fixture
   takes fourteen `if err != nil` branches.
7. **A schema-less pool.** `internal/api/failingpool_test.go` — see the
   correction below for why this is the right shape and the obvious one is not.
8. **The real transport, called directly.** `httpPost` in `internal/auth` and
   `realTransport` in `internal/flymetrics` are the defaults the injectable
   seams fall back to, so both ran only in production and both sat at 0%. An
   in-package test hands each one an `httptest.Server` URL: the real client,
   the real read, the real 1MiB body ceiling, with only the provider replaced.
9. **The half of a wire that only the other end writes.** `ReportsToWire` and
   `RunToWire` run only inside `mtglab forge-shim` after a real match, so on a
   machine without Forge they never ran at all — while every existing test
   drove the decoders. The property, not the bytes: encode-then-decode is the
   identity, `nil` normalises to empty, and an unreported Forge version crosses
   as absent rather than blank.

## Corrections to this file

**A corrupt pool is not a failing pool.** #290 predicted that pointing
`Config.Pool` at a corrupt DuckDB file would "fire the error branch of every
`usePool` call — the biggest single lever left for `internal/api`". It does
neither. `pool.Pool.Use` cannot *open* a corrupt file, so it returns
`ErrNoPool` — byte-identical to the answer an absent pool gives — and the sweep
re-drives the degraded path that was already covered.

The fault worth reaching is a file DuckDB opens happily and then cannot answer:
a half-written refresh, a truncated restore, a schema older than the binary. A
real database with none of the pool's tables in it produces exactly that, and
`TestASchemalessPoolFailsTheQueryRatherThanTheOpen` asserts the distinction
directly — because if `Use` ever folds one into the other, every test in that
file would keep passing while testing the wrong thing.

It was worth **24 statements**, not a package. Most `usePool` error branches
were already reached through the degraded path; only the ones that distinguish
a query failure from an absent pool were new.

**The two formulas.** See *How the number is measured*. This file quoted one
and `ci.yml` gated on the other.

## Traps hit on the way

- **A parent's `defer` runs before its parallel subtests finish.** Use
  `t.Cleanup` for a fixture the subtests share.
- **`-coverpkg` duplicates blocks.** Merge by block key before aggregating —
  and know that `go tool cover -func` merges them differently.
- **The wire spellings differ on purpose.** `add` takes `to`; `swap` takes
  `into`. Both are frozen.
- **A deck's `shared:` key is absent when it IS shared** — `true` removes the
  key rather than asserting the default. So `SetShared(…, true)` on a fixture
  built from `oneDeck` is the standing no-op and never reaches the disk: a test
  of the *write* has to ask for `false`. This cost a debugging round in
  `internal/library/unwritable_test.go`.
- **The editor will not scaffold a section that is not there.** Use `rich.yaml`.
- **`rich.yaml`'s commander is not in the 21-card pool**, so identity resolves
  to colourless.
- **Fixtures may not assert card facts from memory** (rule 1). Where a test is
  about a mechanism rather than a card, use obviously synthetic names.
- **`/api/jobs` does not read `app.db`.** The registry is in-memory, so `[]` is
  honest whatever the database is doing. A hand-kept "these read the database"
  list put it on the wrong side and failed the sweep on correct behaviour.
- **A free port is not a held one.** `net.Listen(":0")`, read the number,
  close — and between that close and the bind, anything on the machine may take
  it. `go test ./...` runs forty-odd binaries at once, most standing up
  `httptest.Server`s out of the same ephemeral range, so widening parallelism
  makes this *more* likely rather than less. It cost one red `main` after #291,
  on amd64 only. Worse than the flake was the message: the test polled for two
  seconds and reported **"the server never answered"** when the truth was "the
  port was taken", which sends the next person after a bug that is not there.
  `bootServer`/`bootShim` in `cmd/mtglab/serve_test.go` retry on a fresh port
  and watch the boot channel, so a real refusal is still a real refusal.
- **A sweep that sweeps nothing passes.** Every table-driven sweep here carries
  a floor (`if swept < 15`), because a pattern filler that stops matching the
  route table is silent otherwise — and silent is indistinguishable from green.

## Recorded rather than fixed

Two behaviours pinned by tests that describe them rather than approve of them.
Both are the same rule producing the same operational risk, and changing either
is Aaron's call, not a patch.

**`data snapshot` on a machine whose volume did not mount** creates an empty
pool on the container's own disk and reports `snapshotted 0 prices for today`
with a green exit.
`TestASnapshotOnAFreshMachineMintsAPoolAndReportsZero` holds it.

**`sim cache` and `sim matches` on the same machine** report `rows: 0` and `no
matches recorded yet`, green. The rule behind it is a good one — a read must
never acquire a database, so an absent `app.db` is an empty history — but on an
unmounted volume it says "you have no matches" when the truth is "I cannot read
your matches". `TestAReaderOnAnUnmountableVolumeReportsEmptinessRatherThanAFault`
holds it, and fails the day it changes.
