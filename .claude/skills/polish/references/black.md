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
  max_tokens/effort settings, and what `claude/ledger.py` has recorded.
  Numbers into the ledger — spend trends are the whole point of tracking this
  facet over runs. Count the modes from `claude/modes.py` rather than from
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
  CSS, scripts. `grep -rn "https://" web/src src/mtglab/web_dist --include`
  patterns, then classify each: (a) committable through the animist —
  do it (recipe, licence gate, PROVENANCE, `mtglab animist build`); (b)
  licence-bound to stay runtime (Wizards art) — leave it, with credit;
  (c) dead or replaceable — remove it.
- The animist is the only road for (a): never hand-place a binary, and run
  `mtglab animist measure` to pick the size knee rather than guessing.
- No CDN dependencies for code or fonts, ever — a CDN is a hotlink with
  better marketing, plus a privacy leak of every visitor's IP.
- Check caching headers on what we already serve — but **do not propose a long
  max-age here without reading `web/vite.config.ts` first.** This repo
  deliberately builds with *stable* filenames rather than content hashes,
  because the bundle is committed and hashing would add two files to git on
  every rebuild. `Cache-Control: no-cache` (`NO_CACHE` in `api/app.py`) is the
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

`mtglab bench` is that instruction in code. Run it rather than hand-timing:

```bash
mtglab bench run                 # the declared suite, warm, ledger-ready table
mtglab bench run --cold          # every cache emptied between samples
mtglab bench profile decks       # one target: database, imports, frames
mtglab bench caches              # hit rates
mtglab mutate list               # (White's facet, same shelf)
```

- **Cold and warm are two measurements, not one.** The shelf went 201ms to
  16ms warm and did not move at all cold, and a ledger row with one slot for a
  number can hold neither honestly. Record both, labelled. `bench run --cold`
  empties every registered cache between samples, which is only possible
  because it runs in-process.
- **Anything over the threshold gets profiled, and the profile goes in the
  ledger — not just the millisecond.** `bench run` does this unasked at 25ms.
  Paste the budget breakdown, not the headline.
- **Three budgets, and only one of them is yours to fix.** `bench profile`
  reports the database exactly (measured at the query probe in `cards/db.py`,
  never subtracted), the import-machinery call count, and everything else.
  The reason it is measured: **cProfile raises no event for an extension
  method**, so a DuckDB `execute` lands in the tottime of whatever Python
  called it — a profile of the card search blamed 38ms on a function whose
  body is three string joins. cProfile's frame table is a **ranking of which
  line**, never a budget of how many milliseconds; its clock is inflated per
  call, and the deck shelf profiles at 188ms against a 19ms wall.
- **A pattern sweep complements a profile and never replaces it.** The
  standing example is the one that got away: an AST sweep for n+1 lookups
  found the one real n+1 in the tree and was blind to everything *inside*
  `get_cards`, where DuckDB was probing `import pandas` twice per bound
  parameter — 1,768 failed imports, 162ms of a 200ms endpoint, all inside the
  import machinery. The statement **count** in `bench profile` is the n+1
  detector that needs no pattern; the import call count is the storm detector.
  **Read that count against `IMPORT_CALLS_SUSPECT` (200), never against
  zero** — this line said zero until 2026-08-19 and zero is not reachable.
  #181 *answered* DuckDB's probe with a `sys.modules` sentinel rather than
  removing it, so a warm bind still enters the import machinery: exactly two
  calls per bound value, and the warm suite runs 7–31 across its targets. A
  run holding the old sentence files a false finding on every profile it
  takes.
- **Every cache gets a hit-rate check, not only a correctness test.** `mtglab
  bench caches` reports the register in `caches.py`. A cache that never hits
  is complexity wearing a win's clothes, and the standing example was correct,
  tested, and dead: `oracle_columns` keyed on the connection object, in an app
  where every endpoint opens one handle, asks one question and closes it. No
  test could have found that; only a counter did. **A cache added since last
  run that is not in the register is a finding**, and `tests/test_caches.py`
  fails on it.
- **Backend numbers** for the routes a session actually hits, on the live
  instance as well as locally — the bench is in-process and cannot see the
  proxy, TLS, or the machine's own contention. Record; compare to last run.
- **Frontend: read the import graph, not the size table.** A size table cannot
  answer *when* a chunk loads, and that gap cost 113kB gzipped on three
  routes: `charts.js` was route-split and recorded as "loaded only where a
  chart renders", which was true of the chunk and false of the load — it was a
  *static* import of three lazy routes. **Route-splitting is not
  component-splitting.** Check `lib/deferred.tsx` and `components/lazycharts.tsx`
  for the pattern, and verify in a real browser's network waterfall which
  chunks a page actually pulls.
- Tier 1 hot path: `sim/tier1/engine.py` is numpy-vectorised and GIL-bound on
  one CPU worker by design. Profile before touching; a vectorisation win needs
  the determinism digest respected — engine changes move `SIM_VERSION`, which
  invalidates the ADR 18 cache, so a micro-win that dumps every cached result
  is not a win.
- Job pools: CPU pool at one worker (deliberate), NET pool for socket-bound
  work — confirm new job types landed in the right pool.
- FastAPI: response models that over-serialise (payload sizes in the browser's
  network panel), gzip/brotli on the deployed instance (check response headers
  live), static file serving with sane cache headers.
- React: re-render hygiene on the interactive surfaces — profile with the React
  DevTools profiler in a real browser, not by eye. Commandment 6 wants motion;
  motion must come from transforms and compositor-friendly properties, never
  layout-thrashing animation.
- **Reject on measurement, and write the rejection down.** An exact-name fast
  path for `get_cards` was tried and lost — it resolved 426 of 441 names and
  the remaining 15 still needed the full scan, so it paid for both. A rejected
  optimisation in the ledger is worth as much as a landed one, because the
  next run will have the same idea.
