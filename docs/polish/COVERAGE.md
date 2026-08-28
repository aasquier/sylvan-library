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
| #290's follow-up | 90.4% | 90.40% |
| this pass | **90.8%** | 90.82% |
| floor in `ci.yml` | **90.5** | |

To reach 95%, **752 more statements** out of 17,977. Of what is left, about
184 needs a JVM or a live network (below), so the reachable remainder is
around 570.

**Budget from what a pass actually moves.** #290's follow-up moved ~145
statements across nine test files and called that the cheap end; this pass
moved **105 across eight**, and it was picking the named levers off the list
below rather than sweeping. At that rate 95% is five or six more passes of
this size, and every one of them is further into the tail than the last. It is
reachable and it is not reachable in an afternoon; a session that promises the
number will deliver tests that assert nothing instead.

**Coverage is not the question worth asking at this altitude.** A statement
that ran is not a statement anything checked, and the tail is full of one-line
error returns where the difference is total. `gremlins unleash
./internal/floats` answers the other question — whether a test would have
*noticed* — and a pass spent on a LIVED mutant in code that already reads as
covered buys more than a pass spent on the next tenth of a point.

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
| 19 | `internal/api/lifecycle.go:importDeck` | — |
| 18 | `internal/api/lifecycle.go:createDeck` | both need **a companion or a partner pair in the 21-card pool**, which it does not have. The fixture is `internal/pool/pooltest`'s embedded JSON; adding to it is a decision, not a patch, because other tests count what is in there |
| 13 | `internal/deckyaml/deckyaml.go:orderedValue` | **do not take this one.** Its two map cases carry a comment saying they are unreachable under `UseOrderedMap` and are kept as a loud failure if the decoder ever stops honouring the option. A test that reached them would have to call the unexported writer directly, and would prove that a branch nothing can enter still works |
| 12 | `internal/pool/loaders.go:load` | — |
| 12 | `internal/deckread/commander.go:CommanderDossier` | query-failure branches; `internal/api/failingpool_test.go`'s `schemalessPool` is the lever |
| 12 | `internal/api/upkeep.go:gatherTheLibrary` | the refresh button's own job body |
| 11 | `internal/pool/refresh.go:OpenWriterWaiting` | the writer's door: the wait, the poll, the give-up. Needs a second process holding the file — `internal/pool/writerlock_test.go` already builds one |
| 10 | `internal/pool/tokensmade.go:TokensMade` | — |
| 10 | `internal/api/adminstats.go:statsActivity` | — |
| 10 | `internal/api/admin.go:inviteAccount` | invite failures with a stubbed sender |
| ~9 each | `internal/api/jobruns.go:listJobs`, `internal/gate/validate.go:Validate`, `internal/claude/tools/handlers.go:searchCards`, `internal/api/forge.go:simForge`, `internal/api/coliseum.go:coliseum` | — |
| ~1,250 | everything else | one to six statements per function; the last four points live here |

Taken off this list by the pass that wrote this table, each with the test that
did it: `gate:checkCompanion` (19 → `reportshape_test.go`),
`claude/canonjson.go:writeValue` (12 → `canonjson_test.go`),
`api/wheel.go:readOptionalBody` (11 → `wheelbody_test.go`),
`pool/refresh.go:DownloadBulkFrom` (10 → `downloadfaults_test.go`),
`deck/deck.go:cardFrom` (10 → `coercions_test.go`), and
`api/lifecycle.go:didYouMean` (14 → `didyoumean_test.go`).

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

10. **The guard, not the path.** `internal/claude/canonjson.go` refuses a
    float and refuses a Go map without `SortKeys`, both by panicking, because
    these bytes are a cache key and a plausible rendering is worse than a
    crash. Four panics and a dozen type arms went from unreached to held by
    one table in `canonjson_test.go`, and what it buys is not the coverage: a
    silent `%g` where a float used to be refused would move every stored
    dossier's key at once, with nothing failing.
11. **The refusal a person reads.** Every "and N more" truncation, every
    "did you mean", every sentence with a card's name in it. They are cheap
    to reach, they are what commandment 2 actually consists of, and they were
    uncovered almost without exception — `reportshape_test.go` and
    `didyoumean_test.go` are the shape.

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

- **A plausible typo tests the resolver, not the shortlist.** The import
  resolves a name on its own once it is close enough and clearly ahead of the
  field, so `Cultivator Colossis` (0.9789) never reaches `didYouMean` at all —
  it is simply read as the card, `unknown` comes back empty, and a test written
  from it passes while proving nothing. What lands in the shortlist is the
  narrow band between the resolver's bar and `mentionFloor`: **measure the
  spelling before writing the assertion.** `didyoumean_test.go` carries the
  three that work and the one at 0.8993 that deliberately does not.
- **A branch whose comment says it is unreachable is not coverage to take.**
  `deckyaml.orderedValue`'s map cases and `checkCompanion`'s `condition == ""`
  are both guards standing behind something that already excludes them. The
  only way to reach them is to call past the guard, which proves nothing and
  leaves a test that will read as meaningful to whoever finds it next.
- **A test-parallelism audit that greps for `t.Parallel()` counts comments.**
  Writing "Go panics on a `t.Parallel()` here" into a serial test's body made
  this pass's own audit report twelve serial tests as parallel. Match code
  lines only.
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

  Retrying on a fresh port was the first answer and it was the wrong one — it
  covered the half where the bind *failed* and left the half where the bind was
  merely *late*, which is the half that kept firing: it cost #337, #341, and a
  green `main` at 46474eb whose `deploy` job skipped, so merged work never
  reached the site until someone re-ran it by hand. Measured, the two sides
  never had a chance of agreeing — the boot took 0.2s to reach its bind when
  idle and 5.3s under load, while the wait gave up after a flat ~2.2s, because
  100 sleeps of 20ms is a wall clock a starved CPU does not stretch. **A
  constant racing something unbounded loses eventually.**
  `heldPort` in `cmd/mtglab/serve_test.go` now keeps the listener and hands it
  to `serveOn`/`serveShimOn`, which take one rather than a port number: nothing
  can take the port, and a probe against a bound port is accepted into the
  backlog and waits for the boot instead of racing it. Under a same-machine A/B
  at load 224 the old shape failed 5 of 15 and the new one 0 of 15.
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
