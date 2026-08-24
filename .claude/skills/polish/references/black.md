# Black — Ruthless Efficiency

Three facets: Claude API spend, static assets over hotlinks, and the
performance pass. Black is the color that gets exactly what it pays for and
pays for nothing twice. Every token comes out of Aaron's pocket and every
wasted millisecond out of a free user's patience; neither is being paid back.

One boundary before anything: when this color's static-assets facet collides
with White's licensing law, **White wins**. An asset whose licence forbids
committing stays a hotlink (Wizards' art is the standing example), no matter
what it costs in performance. Black is ruthless, not lawless.

## Facet: Claude API spend

Load the `claude-api` skill before reading or writing any integration code —
repo rule, and the pricing/caching facts there beat memory.

- Map the spend first: **seven** modes across six features (interview, argue,
  dossier, research, theme ×2, and **scan** — ADR 34's card transcriber, which
  this file said "six" through the run that measured it), their models, their
  max_tokens/effort settings, and what the Claude ledger has recorded.
  Numbers into the ledger — spend trends are the whole point of tracking this
  facet over runs. Count the modes from `internal/claude`'s mode table rather than from
  this sentence; the number moves and this line will not.
- Prompt caching: `converse` caches on its system block, which is why
  personas are **appended** and dealt cards ride in the message. Audit any
  change since last run for something that moved per-turn content into the
  system prompt (kills the cache) or per-conversation content into a resend
  loop (pays repeatedly).
- Deduplication and memoisation posture: dossier cached on `oracle_id`,
  in-flight dedupe via `jobs.submit(key=…)`, research and theme deliberately
  uncached (argued in their ADRs — do not re-add silently; if the economics
  say otherwise, queue it with the numbers). Every mode should be covered by
  either a cache, a key, or a written argument for neither.
- Right-size the model per mode: Sonnet 5 is the target by Aaron's explicit
  call. Do not silently change models; if a mode's task profile clearly fits
  a cheaper or better-priced model, queue the proposal with measured
  token counts and the pricing math.
- Web search costs per call: dossier and research budgets (`max_uses`) should
  be deliberate numbers, not defaults.
- Failure spend: a mode that errors after its Anthropic call has spent the
  tokens anyway. Check that validation happens request-side before the call
  (the refusable-is-refused-in-the-request pattern) for anything new.

## Facet: static assets over hotlinks

Aaron's rule: prefer assets we serve over links we chase — spend the effort
once, deploy it, and be fast forever.

- Inventory every external URL the served app references: images, fonts,
  CSS, scripts. `grep -rn "https://" web/src web_dist` (the built bundle is
  the half a source grep cannot see), then classify each: (a) committable
  through the animist — do it (recipe, licence gate, PROVENANCE, then
  `animist build` from `tools/`); (b) licence-bound to stay runtime (Wizards
  art) — leave it, with credit; (c) dead or replaceable — remove it.
- The animist is the only road for (a): never hand-place a binary, and run
  `animist measure` to pick the size knee rather than guessing.
- No CDN dependencies for code or fonts, ever — a CDN is a hotlink with
  better marketing, plus a privacy leak of every visitor's IP.
- Check caching headers on what we already serve — but **do not propose a long
  max-age here without reading `web/vite.config.ts` first.** This repo
  deliberately builds with *stable* filenames rather than content hashes,
  because the bundle is committed and hashing would add two files to git on
  every rebuild. `Cache-Control: no-cache` (the door's static tiers) is the
  half that makes stable names safe, and it was added 2026-08-13 after Safari
  assigned its own heuristic lifetime and served a stale `DeckDetail.js`
  against a redeployed server until the page crashed. Verified 2026-08-16:
  conditional requests return 304, so the cost is one revalidation RTT per
  navigation, not a re-download. A long max-age is only back on the table
  paired with content hashing, and that is a queued decision, not a fix.

## Facet: performance & efficiency

**Profile first, then measure, then optimise.** That order is the correction
the 2026-08-19 pass earned: the previous version of this facet said to record
response times, a run duly recorded `/api/decks` at 224ms, and the largest
performance bug in the codebase sat inside that number for three days. A
millisecond is a datum. **A large millisecond is a question**, and nothing
here used to say so.

The purpose-built `bench` command retired with the old backend and its Go
rebuild is an open ledger item, so this facet runs on the stock toolchain.
That is a trade up, not down: Go ships a sampling profiler, an allocation
profiler, a blocking profiler and a race detector, and none of them need a
harness. Everything below is from `go/` with the three exports set.

```bash
go test -run '^$' -bench . -benchmem -count=10 ./internal/<pkg>/ > new.txt
benchstat old.txt new.txt                     # the only honest before/after
go test -run TestX -cpuprofile cpu.out -memprofile mem.out ./internal/<pkg>/
go tool pprof -top -nodecount=25 cpu.out      # -http=: for flame and graph
go tool pprof -top -sample_index=alloc_objects mem.out
go build -gcflags=-m ./internal/<pkg>/ 2>&1 | grep escapes
```

- **Benchmark the function; probe the route.** A Go benchmark answers about a
  function, and most of what this facet chases is an HTTP route. Time the
  route from outside (`curl -w '%{time_total}'` warm and cold, local and
  live), then take the number that is too big *into* a benchmark or a profile
  of the package behind it. The route number is the question; the profile is
  the answer.
- **`ns/op` without `allocs/op` is half a measurement.** In Go the usual cause
  of a slow path is allocation and the garbage it makes, and the alloc profile
  (`-sample_index=alloc_objects`) names the line. `-gcflags=-m` says which
  values escaped to the heap and why — often a one-word fix (a pre-sized
  slice, a value receiver, a `strings.Builder`).
- **A single run is not a result.** Go benchmarks on a laptop move several
  percent between runs on thermal noise alone. `-count=10` through `benchstat`,
  and a delta it marks insignificant (`~`) is not a finding — writing it down
  as one is how a run reports a win it did not have.
- **The profiler is nearly blind inside cgo, which is exactly where the card
  pool lives.** DuckDB work arrives as `runtime.cgocall` with no shape under
  it, so a pool-heavy route's profile will look mysteriously flat. **Clock the
  database at the query** and profile the Go half; never infer the database's
  share by subtracting what the profile shows. This is the retired shelf's
  hardest-won lesson, and it survived the language change intact — the reason
  changed, the discipline did not.
- **The n+1 detector that needs no pattern is a statement count.** Count
  queries per request (a counting `sql.DB` wrapper in a test, or the pool's own
  instrumentation) rather than grepping for loops around calls: a sweep over
  source finds the n+1s it has a shape for and is blind to the ones inside a
  helper, which is where the expensive ones live.
- **Contention is its own profile.** `-blockprofile` and `-mutexprofile` find
  what a CPU profile cannot: goroutines waiting rather than working. The
  standing suspects are the single-writer database and any shared handle —
  worth a look whenever a route is fast alone and slow under a load probe.
- **Concurrency is a throughput tool, and this facet owns the question of
  whether it bought anything.** Blue owns the *shape* of concurrent code —
  which primitive, whose error, whose lifetime — and refuses a goroutine
  added on a hunch. Black owns the number that would justify one. The order
  is fixed and the wrong order is how concurrency gets added to code that was
  never the bottleneck: **profile, then find the wait, then parallelise the
  wait.** Three shapes that actually pay here, each needing a measurement
  first:
  - **Fan-out over independent work** — several pool queries or file reads a
    request makes in sequence and could make at once. `errgroup` bounds it and
    carries the first error out; `golang.org/x/sync` is already an indirect
    dependency, so it costs no new module.
  - **A read-mostly structure behind a plain mutex.** `sync.RWMutex` or an
    `atomic.Pointer` swap only wins when readers actually contend — and the
    inventory is **15 `sync.Mutex` and zero `RWMutex`**, which is a question
    to ask with `-mutexprofile`, never a blanket conversion. Under low
    contention `RWMutex` is *slower*.
  - **Work sized to the machine.** `GOMAXPROCS` follows the machine and the
    machine now has two shared cores, so a lane sized by a literal does not
    take the second one. A worker count that is a constant is a finding worth
    at least a comment saying why it is constant.

  And the standing caution, because it is the expensive mistake: **more
  goroutines on a two-core shared machine mostly buys scheduling.** A win that
  exists on this 8-core laptop and not on the instance is not a win; measure
  where it ships.
- **Every cache gets a hit-rate check, not only a correctness test.** A cache
  that never hits is complexity wearing a win's clothes, and the standing
  example was correct, tested, and dead — keyed on a per-request handle in an
  app where every request opens its own. No test could have found that; only a
  counter did. The register that used to hold every cache retired with the old
  backend, so until the Go shelf is rebuilt, **a cache added since last run
  with no hit count anywhere is a finding**, and rebuilding the register is
  the standing proposal.
- **Backend numbers** for the routes a session actually hits, on the live
  instance as well as locally — a local measurement cannot see the proxy, TLS,
  or the machine's own contention, and since 2026-08-23 the machine has two
  shared cores rather than one. Record; compare to last run.
- **Frontend: read the import graph, not the size table.** A size table cannot
  answer *when* a chunk loads, and that gap cost 113kB gzipped on three
  routes: `charts.js` was route-split and recorded as "loaded only where a
  chart renders", which was true of the chunk and false of the load — it was a
  *static* import of three lazy routes. **Route-splitting is not
  component-splitting.** Check `lib/deferred.tsx` and `components/lazycharts.tsx`
  for the pattern, and verify in a real browser's network waterfall which
  chunks a page actually pulls.
- **Tier 1 is the one hot path where a win can cost more than it saves.** The
  ADR 18 cache is keyed partly on an **engine fingerprint** — a hash of five
  embedded packages, listed in `internal/sim/cache`'s `engineSources` — so any
  change inside them rotates the key and orphans every stored result. A
  micro-optimisation that dumps the cache is not a win; measure the saving
  against a cold recompute of everything before proposing it. Determinism
  binds too: the seeded generator and the exact-float helpers are contract,
  and a faster sum that disagrees with the frozen goldens is a bug.
- Concurrency shape: the job lanes are CPU-bound and socket-bound work kept
  apart — confirm a new job type landed in the right one. `GOMAXPROCS` follows
  the machine, so the second core widens the CPU lane with no code change and
  a lane sized by a literal `1` would silently not take it.
- The door: check what the served payloads actually weigh in the browser's
  network panel, that compression is on for them at the deployed instance
  (read the response headers live, not the handler), and that the static tiers
  still send what `web/vite.config.ts`'s stable filenames require.
- React: re-render hygiene on the interactive surfaces — profile with the React
  DevTools profiler in a real browser, not by eye. Commandment 6 wants motion;
  motion must come from transforms and compositor-friendly properties, never
  layout-thrashing animation.
- **Reject on measurement, and write the rejection down.** An exact-name fast
  path for `get_cards` was tried and lost — it resolved 426 of 441 names and
  the remaining 15 still needed the full scan, so it paid for both. A rejected
  optimisation in the ledger is worth as much as a landed one, because the
  next run will have the same idea.
