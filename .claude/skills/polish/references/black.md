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

- Map the spend first: six modes (interview, argue, dossier, research, theme
  ×2), their models, their max_tokens/effort settings, and what
  `claude/ledger.py` has recorded. Numbers into the ledger — spend trends
  are the whole point of tracking this facet over runs.
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
- Check caching headers on what we already serve: committed assets should be
  aggressively cacheable (hashed filenames from Vite get long max-age).

## Facet: performance & efficiency

Measure, record, then optimise — an optimisation without a before/after
number in the ledger did not happen.

- **Backend numbers**: response times on the live instance for the routes a
  session actually hits (deck page payloads, sim submit/poll, colors,
  glossary). Cold and warm. Record; compare to last run.
- **Frontend numbers**: built bundle size (`npm --prefix web run build`
  output, per-chunk), and whether the biggest chunks earn their place —
  recharts is the known heavyweight; route-level code splitting is the
  standard lever if a chunk grows.
- Tier 1 hot path: `sim/tier1/engine.py` is numpy-vectorised and GIL-bound
  on one CPU worker by design. Profile before touching (`python -m cProfile`
  on a representative run); a vectorisation win needs the determinism digest
  respected — engine changes move `SIM_VERSION`, which invalidates the ADR
  18 cache, so a micro-win that dumps every cached result is not a win.
- Job pools: CPU pool at one worker (deliberate), NET pool for socket-bound
  work — confirm new job types landed in the right pool.
- DuckDB: query patterns in `cards/db.py` — n+1 lookups that could batch
  through `get_cards`, missing use of the price-history indexes.
- FastAPI: response models that over-serialise (payload sizes in the
  browser's network panel), gzip/brotli on the deployed instance (check
  response headers live), static file serving with sane cache headers.
- React: re-render hygiene on the interactive surfaces (the deck page and
  its panels) — profile with the React DevTools profiler in a real browser,
  not by eye. Commandment 6 wants motion; motion must come from transforms
  and compositor-friendly properties, never layout-thrashing animation.
