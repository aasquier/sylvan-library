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

**The gate runs on the arm64 matrix leg only** (since #466): the number is
architecture-independent while every Go file compiles on both legs, and
`TestEveryGoFileCompilesOnBothCILegs` (`go/cmd/mtglab`) holds that premise.
Raise the floor against the arm64 leg's own print.

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
| #478's pass | 90.8% | 90.82% |
| the climb of 2026-09-24 (nine lanes, #488–#496) | **MEASURED_TOTAL%** | MERGED_TOTAL% |
| floor in `ci.yml` | **95.0** (set at a measured MEASURED_TOTAL, 2026-09-24) | |

The gate is at 95.0 and the tree is over 96 because Aaron asked for exactly
that pair: a diff can cost a few tenths of honest refactoring without going
red, and a diff that costs a whole point is what the floor exists to notice.

**How the climb was paid for.** Nine lanes, one per package group, each
handed the baseline's BY FUNCTION table for its files and a share of the
distance, run in two waves of four or five on this Mac with a dedicated
merge train landing them one at a time. The tree went from 1,892 missing
statements to MISSING_NOW in one day, and **not one test asserts less than it
should** — every lane was told a test that runs lines and checks nothing was
out of scope, and the levers below are what they found instead. The same
day every serial test in the tree became parallel; the two jobs turned out
to be one, because what blocked `t.Parallel()` (a URL in a package variable,
a token read off the process, an index every machine shared) was the same
thing that had kept the code from being driven.

**What is left is genuinely per-function** — the lanes named every branch
they judged unreachable and why, and those are collected under *Left
deliberately* below so nobody re-chases them. The two questions still
worth asking at this altitude are the ones this file has always ended on:
`gremlins unleash ./internal/floats` answers whether a test would have
*noticed*, and a LIVED mutant in code that reads as covered is worth more
than the next tenth.

## What the floor cannot reach

**Nothing, as a feature.** The table that stood here — `RunGames` and
`spawn`, the shim's `match`/`matchStreamed`/`watchdog`, `sim forge`'s
reporting, `data refresh` — was ~190 statements that "needed a JVM with
Forge beside it or the live Scryfall network". It emptied on 2026-09-24 by
the same move four times over: **the thing the code could not run without
was already a value, one argument short of being handed in.**

| what stood in the way | what it is now | what runs |
|---|---|---|
| a JVM | `tier3.Settings.Java` — and a shell script that answers `-version` and `cat`s a frozen game log is one | `RunGames`, `playSegment`, `spawn`, `sim forge`, the shim's `match` and `matchStreamed`, end to end |
| `os.Exit` in the watchdog | `idleWatch.exit`, a field | the watchdog's whole body |
| Scryfall | `pool.RefreshOptions.IndexURL`, reached through `api.Config.BulkIndex` and the `data refresh` command — an `httptest.Server` serving a bulk file is Scryfall | the refresh button's job body, the CLI's refresh, the seal, the five progress beats |
| a second process holding the pool | `writerlock_test.go`'s child — it always could | `OpenWriterWaiting`'s wait, poll and give-up |

`ci.yml`'s comment says the same thing in fewer words, so the next person to
ask why it is not 100 is sent here rather than to a list that no longer
exists.

## Left deliberately

Every lane named what it judged unreachable and why, so that a future pass
reads this list before spending an hour on a branch that cannot be entered
honestly. **A branch whose comment says it is unreachable is not coverage to
take**; calling past the guard proves nothing and leaves a test that reads
as meaningful to whoever finds it next.

- `deckyaml.orderedValue`'s map cases and `sortedKeys` (only reachable from
  them), and `checkCompanion`'s `condition == ""` — the two standing entries,
  still standing.
- **Second reads that cannot fail after a first succeeded**: `src.ReadText`
  in `internal/api/edits.go` after `writeTarget`'s `Get`; the duplicate
  `library.WriterFor` calls in `lifecycle.go`; `commanderRecords` after the
  same closure's `GetCards`. Only a race reaches them.
- **Scans and `rows.Err()` on a still-open handle**, across `library`,
  `deckread`, `decklog`, `pool` — with one exception: `authtest.Fault.RowsAfter`
  makes `rows.Err()` reachable, and `internal/auth`, `night` and `traffic`
  use it. Other packages could, at the price of a faulty connector of their
  own.
- **`lib.Mine()` failing** needs `Authenticated && UserID == 0`, which
  `auth.scopeFor` cannot produce; `myCrypts`/`myWritableLibraries`' empty
  answer likewise.
- **Guards over embedded data**: `GetMode`/`Preset(constant)`, `dial`'s
  self-documented branch, `json.Compact` after `json.Unmarshal` succeeded,
  `numbers.go`'s two separator guards, `tools.go`'s nil-props/nil-required/
  nil-handler arms (the data always carries `{}`, `[]` and all seven).
- **`crypto/rand` failing** (`shelves`, `sim/tier1`, `brew`, `tarot`) and
  the three `sim/karsten` memo tables that evict at 100,000 entries.
- **Trailing `return`s no loop can fall past**: `textutil.Head`,
  `decklist.firstRunes`; `decklist.digitValue`'s eleven-consecutive-Nd
  fallback; `consume`'s `len(leftovers) < cost.Generic`, arithmetically
  impossible after pip matching; `sim/tier1`'s branch labelled unreachable
  in its own comment.
- **`fs.Glob` swallows its `ReadDir` error**, so `Fingerprint`'s glob-error
  branch cannot fire with a literal `"*"`; the `fs.ReadFile` branch beside
  it can and does.
- **`users.go:prompt.secret`'s terminal branch** wants a real pty
  (`golang.org/x/sys` is already a dependency; platform-specific test code
  nobody has argued for yet) — the single biggest lever left in
  `cmd/mtglab`, at five statements.
- **Fixture decisions, not patches**: `simShelfCommand`'s `Approximated`
  tail wants a two-colour card in the 21-card pool; `coliseum.go`'s
  `rec != nil` arms want `Grand Coliseum` and `Jareth, Leonine Titan` in it;
  `simMulliganCommand`'s non-flat `BEST:` branch wants a deck the tiny pool
  cannot express. A hand-added row named after a real card is a claim about
  Magic nobody looked up (rule 1); adding those rows from the real pool is
  the honest route.
- **`gate/rulebreaker.go`** (6) is reachable but needs real ADR 51 clause
  text off a real card — a session with the pool, not a worktree.
- **`deckread/commander.go`**'s seven statements that need `GetCards` to
  succeed and a later `oracle_cards` query to fail: a pool that breaks
  mid-flight, which no fixture yet is.

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

The rest are from the climb of 2026-09-24, one lane each, and every one of
them is reusable:

12. **A scripted JVM.** `tier3.Settings.Java` was already the seam: a shell
    script that answers `-version` with a real-looking line and then `cat`s
    a frozen game log is a JVM as far as `spawn` can tell. `RunGames`,
    `playSegment`, the whole bout, `sim forge` and the shim's `match` all
    run without Forge (`gameclock_test.go`'s `fakeJava`, `cmd/mtglab`'s
    `testdata/fakejava`). See the trap below about minting executables.
13. **The job body behind a URL constant.** A package constant that reaches
    the network is a whole feature nothing can test. Make it a field whose
    zero value is the constant (`api.Config.BulkIndex`, `SetsFeed`) and the
    job body becomes drivable: `gatherTheLibrary` went 24 → 6.
14. **A database that refuses writes.** Three `BEFORE … RAISE(ABORT)`
    triggers reach every write-error branch a closed handle cannot, because
    a closed handle fails at the *first* call and these fail at the one
    that matters.
15. **The volume that goes away mid-transaction.** `authtest.OpenFaulty`
    returns a real migrated `app.db` over a connector that refuses every
    statement past a budget (`Fault.After`), and `Fault.RowsAfter` fails a
    result set partway so `rows.Err()` is reachable. It is what took
    `internal/auth` 96 → 28 and `night` 54 → 14: the branches where a
    half-finished write decides whether to roll back, whether to say so and
    whether to claim it did the work.
16. **A schema older than the binary — one table at a time.** `DROP TABLE`
    names which read depends on which table where a closed handle only
    proves "something failed": `statsActivity` 18 → 4 by dropping `users`,
    `auth_tokens`, `sessions`, `sim_cache`, `deck_log` in turn. The same
    trick on the pool (`DROP TABLE printings` after `pooltest.Build`) makes
    every card lookup succeed and only art, prices and printing history
    fail — stronger than the schema-less pool for anything downstream of
    `GetCards`. `ALTER TABLE … DROP COLUMN` is refused by the schema's
    index; rebuild with `CREATE TABLE … AS SELECT`, drop, rename.
17. **An `app.db` that claims a schema it does not have.** `auth.Migrate`
    is a no-op when `user_version` already reads `SchemaVersion`, so a file
    with the pragma and no tables walks straight past the ladder. It is the
    schema-less pool one layer up (`claimedschema_test.go`; `hollowed(t, d,
    tables...)` for "the accounts are there, the tables about them are
    gone").
18. **Two handles, one taken away.** `library.NewSQLSource(read, write, …)`
    holds separate handles by design, so closing exactly one asks the read
    question and the write question separately (`closedhandles_test.go`).
19. **An in-package `&Conn{db: db, pool: New("", nil)}`** is a full pool
    connection with no lease and no memo; it is what makes a hand-built
    DuckDB table with *some* of the pool's columns land the failure on a
    named read (`tokens_made`, `token_identities`, `card_art`).
20. **No maintainer configured plus a closed `app.db`.** With `AdminEmail`
    empty the maintainer lookup is skipped, so `a.library()` succeeds and
    the failure lands one layer in — the only way to reach the crypt's three
    routes and the two master switches.
21. **A crypt you can bury into and cannot read back** — `.trash` at
    `0o333`: rename in works, `ReadDir` does not. `0o500` is the other half.
    A shelf the process can read and not write (`0o500` on the decks dir)
    reaches `Create`, `WriteArtifacts` and `SetShared` failures — and
    `shared: true` removes the key, so a test of the write asks for `false`.
22. **A route-table-derived malformed-body sweep.** `readBody` had sixteen
    call sites and one of them tested.
23. **The `init` body as a function over a document.** Four packages
    (`reference`, `brew`, `tarot`, `prices`' table) had boot panics nothing
    could reach; `loadDeck(raw)`, `loadCauldron(raw)`, `indexArenas(doc)`
    and friends make the guard drivable without touching the committed
    data. The guard, not the path — `canonjson_test.go`'s shape.
24. **A filesystem as a value, for the one write that replaces
    `deck.yaml`.** `writeAtomically`'s five failure branches each name the
    deck that would not save and none can fire on a working disk;
    `writeAtomicallyOn(disk, …)` is a struct of five functions, a value
    rather than a hook so the package's tests stay parallel.

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

- **Minting an executable per test races a 30-second probe.** The first
  execution of a freshly written executable costs seconds on this Mac (6.2s
  measured idle, inside the test binary), and on Linux a script written by
  one parallel test can be exec'd by another while a third still holds the
  write fd open across its own fork — `text file busy`. `javaMajor` gives a
  candidate 30s; eight of them at once under `go test ./...` reported `Java
  None`, which reads like a broken JVM. The fix is one committed executable
  (`testdata/fakejava`) and per-test **data** it reads from the subprocess's
  cwd, which `spawn` already makes the Forge home.
- **A test fixture in a non-test package costs coverage.** The first
  `authtest/faulty.go` wrapped every optional driver interface with a
  fallback; the driver implements all of them, so 49 fallback statements sat
  uncovered and ate a third of a lane's gain. Cut to the roads it travels.
- **`unparam` fires on a test helper's constant argument at the fourth
  caller.** Adding a caller that passes the same value as the existing
  three turns a green lint red in a file you did not edit.
- **An aborted HTTP body must exceed the server's write buffer.**
  `panic(http.ErrAbortHandler)` after under 2 KB reaches the client as a
  failed `client.Do`, not as a short `io.Copy` — a different branch.
- **`sql.Open("duckdb", path)` fails eagerly.** A missing directory or a
  file of garbage errors at `Open`, not at `Ping`, so `pool.Open`'s ping
  branch stays unreachable and a corrupt file is `ErrNoPool`
  (`TestAFileThatIsNotADatabaseIsNoPoolRatherThanAFailingOne` holds it).
- **`Repeats`' overlap threshold is 0.7 of the shorter fact's vocabulary**,
  so a "reworded fact" written by eye silently does not reach it; measure
  the overlap before asserting.
- **A crypt folder that is not a name.** `safeSegment` sees the folder name
  in `Empty` and the parsed slug in `Restore`; a stamped folder trims to
  something non-empty and passes both, a folder named exactly `...` fails
  both.
- **The two sweeps in `failingpool_test.go` were asking a 404, not a pool**
  — `{slug}` was filled with a deck the fixture library does not hold, so
  every deck-scoped route stopped at ADR 5's 404 and the file read as
  working because the non-deck routes did reach the pool. A sweep aimed at
  a deck that exists **fails on a 404** now. A route sweep's filler is part
  of the assertion.

## Recorded rather than fixed

Behaviours pinned by tests that describe them rather than approve of them.
The first two are the same rule producing the same operational risk; the rest
were found by the climb of 2026-09-24 and are queued for a ruling in
`DAYBREAK.md`. Changing any of them is Aaron's call, not a patch.

- **`auth.exclusive` can hand a poisoned connection back to the pool.** It
  opens with a hand-written `BEGIN IMMEDIATE` on a pinned connection; when
  the ROLLBACK or the COMMIT also fails it returns that connection with the
  transaction still open, and the next `BEGIN IMMEDIATE` on it is refused —
  a handle that keeps answering reads while writing nothing.
  `clearStaleTransaction` in `internal/auth/halfwritten_test.go` carries the
  paragraph. Beside it: `SetPassword` assigns `revoked` inside the
  transaction, so a COMMIT that never lands returns a count about work that
  was rolled back.
- **A recorder opens a database it has not checked.** `claude/ledger` and
  `tier3/ledger`'s `NewRecorder` both `sql.Open` without a ping, so a
  missing `app.db` is discovered at the first *write* — and the Claude one
  warns rather than fails, so on an instance whose volume did not mount,
  conversations happen, cost money and are not recorded.
  `TestARecorderOverAMissingDatabaseOpensAndDiscoversItLater` holds the one;
  a sibling in `tier3/ledger` holds the other.
- **A swap that trades one chosen colour for another is refused**
  (`internal/api/edits.go`, `playableCard` computes `chosenColorReach` over
  the deck *including* the card on its way out). The deck the swap would
  produce is legal; the refusal names the order that works.
- **An unreadable `artifacts/` directory answers `[]` and `baseline:
  "unknown"`** (`internal/library/source.go`), the same words a never-built
  deck gets. The build refuses correctly; only the shelf lies.
- **The intake's slot sweep reports a silent zero when every call was
  refused** (`internal/api/intake.go`): its one note fires only on the
  credential going away, so a dead endpoint renders "0 of 2" with nothing
  said.
- **`data snapshot` over a pool with none of the pool's tables prints
  `snapshotted 0 prices for today` and exits 0** — `SnapshotPrices` creates
  what it needs rather than refusing. Not pinned; the cmd lane dropped the
  test rather than assert it.

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
