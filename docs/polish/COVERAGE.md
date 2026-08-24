# The coverage floor

The Python suite carried a 95% gate. The Go port did not inherit it — CI ran
`-cover` and threw the number away — so coverage drifted to **80.6%** with
nobody watching. PR #290 put a gate back and did the work to stand on it.

This file is the working map for the rest of the climb. It is a claim to
re-check, not a fact to inherit: **re-measure before trusting a number here.**

## How the number is measured

```bash
go test -count=1 -coverprofile=coverage.out -coverpkg=./... ./...
go tool cover -func=coverage.out | tail -1
```

`-coverpkg=./...` is load-bearing. Without it each package is measured only
against its own test binary, which answers a different question — how well a
package tests *itself* — and reads 0% for a package whose behaviour is
exercised entirely through its callers. The merged number is the one the floor
is about.

The per-file breakdown, which is what to work from:

```bash
tail -n +2 coverage.out | awk '{k=$1; n[k]=$2; c[k]+=$3} END {for(x in n) print x, n[x], c[x]}' \
  | awk '{split($1,a,":"); f=a[1]; sub(/^github.com\/aasquier\/sylvan-library\/go\//,"",f);
          t[f]+=$2; if($3>0) v[f]+=$2}
     END {for(k in t) if(t[k]-v[k]>0) printf "%6.1f%%  miss=%4d  %s\n", 100*v[k]/t[k], t[k]-v[k], k}' \
  | sort -k2 -t= -rn
```

The merge step matters: with `-coverpkg`, every test binary emits every block,
so a naive sum counts each statement forty-six times.

## Where it stands

| | statements | |
|---|---|---|
| covered | 14,503 | |
| uncovered | 1,645 | of which ~210 is unreachable in CI (below) |
| **total** | **16,148** | **89.8%** |

To reach 95%, ~838 more statements. Of the 1,645 uncovered, roughly 1,435 are
reachable without a JVM or a network, so the target is about 58% of what is
left — achievable, and a grind rather than a trick.

## What the floor cannot reach

Named here and in `ci.yml` so nobody has to rediscover it. Each needs a JVM
with a Forge distribution beside it, or the live Anthropic or Scryfall network:

| where | what | statements |
|---|---|---|
| `internal/sim/tier3/run.go` | `RunGames`'s body, `spawn` | ~99 |
| `cmd/mtglab/shim.go` | `match`, `matchStreamed`, `watchdog` (calls `os.Exit`) | ~45 |
| `cmd/mtglab/sim.go` | `sim forge`'s own reporting | ~50 |
| `cmd/mtglab/data.go` | `refresh`, which downloads from Scryfall | ~15 |
| `cmd/mtglab/claude.go` | `claude check`, which spends a real call | ~10 |

**Their seams are covered.** The worker client and the shim door both run
against stubs over real HTTP; `pool.DownloadBulkFrom` runs against a stub
Scryfall; every refusal path around all of them is driven. What is missing is
the call itself.

`claude check` is the one that could still be reached: it calls
`claude.EndpointFromEnv()` directly, and `claude.EndpointAt(baseURL, key)`
already exists as the test seam. Threading an `Endpoint` into
`claudeCheckCommand` would make it testable against a stub — a small change,
worth asking Aaron about first since it touches the CLI's shape.

## The remaining work, biggest first

Re-measure before starting. As of #290:

| miss | file | shape of the work |
|---|---|---|
| 64 | `cmd/mtglab/sim.go` | most of it is `sim forge`'s reporting (blocked); the rest is flag validation |
| 48 | `cmd/mtglab/users.go` | `connectUsers` failures, `readSecret`'s terminal branch |
| 45 | `internal/api/lifecycle.go` | create's companion and partner branches — **needs a companion or a partner pair in the 21-card pool**, which it does not have |
| 41 | `internal/api/edits.go` | entomb/return/exile on decks that have those sections |
| 33 | `internal/deckread/deckread.go` | the remaining art and payload branches |
| 32 | `internal/api/admin.go` | invite and reset failures with a stubbed sender |
| 29 | `internal/deckedit/ops.go` | the surgical spans' own error returns |
| 28 | `internal/gate/validate.go` | the rest of the companion restriction checkers |
| 27 | `internal/library/write.go` | `writeAtomically`'s failure branches |
| 26 | `internal/auth/users.go` | the last of the `if err != nil` sweep |
| 23 | `internal/sim/cache/store.go` | a closed store, and `Clear` |
| ~473 | files with ≤11 missing each | the long tail — small, and it is where the last two points live |

## Levers that worked

Worth reaching for again before writing anything bespoke:

1. **A closed handle.** `internal/auth/errorpaths_test.go` covers ~40 error
   branches by closing a real migrated database and calling everything. The
   assertion is not the message — SQLite's wording is not ours — but that an
   error comes back *at all* and nothing claims success.
2. **An unreadable directory.** `internal/api/unreadable_test.go` chmods the
   library to `0o000` and sweeps every deck route. One test, dozens of
   branches, and it is the deployed shape rather than a contrived one.
3. **A route sweep.** `internal/api/refusals_test.go` asks two questions of
   every route in a table — a deck that is not there, an owner that is not
   there — rather than spot-checking. The refusals are the part no structural
   rule can see.
4. **A stub over real HTTP.** `internal/sim/tier3/worker_test.go` and
   `cmd/mtglab/shimdoor_test.go` drive both halves of ADR 35 against
   `httptest.Server`, which exercises the real request building and the real
   streaming reader.
5. **A pool that fails.** *Not yet used.* Pointing `Config.Pool` at a corrupt
   DuckDB file should fire the error branch of every `usePool` call in the api
   package — the same trick as the unreadable library, and the biggest single
   lever left for `internal/api`.

## Traps hit on the way

- **A parent's `defer` runs before its parallel subtests finish.** Use
  `t.Cleanup` for a fixture the subtests share, or the closed handle surfaces
  as "the library could not answer that right now" on every one of them.
- **`-coverpkg` duplicates blocks.** Merge by block key before aggregating.
- **The wire spellings differ on purpose.** `add` takes `to`; `swap` takes
  `into`. Both are frozen.
- **A deck's `shared:` key is absent when it IS shared** — `true` removes the
  key rather than asserting the default. A private fixture must say so.
- **The editor will not scaffold a section that is not there.** A deck file
  with no `swap_board:` refuses rather than gaining one; use `rich.yaml`.
- **`rich.yaml`'s commander is not in the 21-card pool**, so identity resolves
  to colourless and only a colourless legal card may be added to it.
- **Fixtures may not assert card facts from memory** (rule 1). Where a test is
  about a mechanism rather than a card, use obviously synthetic names —
  `internal/gate/companioncheck_test.go` does.

## Recorded rather than fixed

`data snapshot` on a machine whose volume did not mount creates an empty pool
on the container's own disk and reports `snapshotted 0 prices for today` with a
green exit — a failed run that looks like a successful one.
`TestASnapshotOnAFreshMachineMintsAPoolAndReportsZero` pins today's behaviour.
Whether it should refuse instead is Aaron's call, not a patch.
