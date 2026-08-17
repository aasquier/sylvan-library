# The Polish Ledger

The memory of the recurring polish pass (`.claude/skills/polish/`). One
section per color; each run updates its color's section **on the branch that
did the work**. Queued findings wait on Aaron and are not re-litigated;
deferred items name the trigger that revives them; measurements are recorded
even when healthy, because today's healthy number is next quarter's baseline.

Facet-to-color map and the run protocol live in the skill. This file holds
state, never checklists.

---

## White — Law & Protection

*Licensing/free-use (triple-checked) · security & isolation · testing discipline*

- **Last run:** 2026-08-16 (rainbow). Previous: 2026-08-16 (the skill's eval).
- **Fixed this run (2026-08-16, rainbow):**
  1. **`.[dev]` could not run a third of the suite, or the type checker.**
     `pip install -e ".[dev]"` — what CLAUDE.md's Setup section documents, and
     what it calls complete ("dev, which includes all of it") — ran **1444
     tests where CI ran 1918**. Six modules open with
     `pytest.importorskip("fastapi")` and fastapi was declared only in `api`:
     `test_api`, `test_admin`, `test_auth_api`, `test_auth_reset_api`,
     `test_library_write_gate`, and `test_isolation` — the route-classification
     sweep ADR 5 calls the highest-value test in the auth story. `mypy` failed
     too (`cli.py` imports uvicorn). Invisible from both sides: CI installs
     `.[dev,api]` and asserts those imports, and the pinned skip count is only
     counted in CI. **For once the laptop was the half testing less** — the
     inverse of the 2026-08-12 tiny_pool incident, and Commandment 11's local
     gauntlet was running on it. Fixed by adding `fastapi` and
     `uvicorn[standard]` to the `dev` extra (both already declared in `api`; no
     new dependency, image untouched). Pinned by two evaluating tests in
     `tests/test_packaging.py`: `dev` must cover every distribution the other
     extras declare, and every module in ci.yml's own import guard.
  2. **A test was writing the maintainer's real `app.db`.**
     `test_deck_source.py::test_library_resolution_edges` took no fixture and
     called `Library.source_for` for an unknown owner while authenticated,
     falling through to `Library._owner_id` — the branch whose own comment says
     opening `app.db` on a laptop means *creating* it. So the suite reached the
     real database (accounts, sessions, the only personal data this project
     stores) and ran `auth/db.py`'s **forward-only** migration ladder over it;
     on an older checkout that carries the real schema forward with no way
     back. The file's section comment claimed the opposite ("nothing touches
     the real one"). Contained under `use_paths`, and — the part that matters —
     `tests/conftest.py` gained `_real_app_db_untouched`, a detector that fails
     any test creating or writing the real file. Its two neighbours,
     `_no_usage_ledger` and `_no_deck_log`, both say they were found "by
     running the suite and then looking at the file"; this is that look, taken
     automatically. Exactly one offender suite-wide, found by instrumenting
     every teardown.
  All three guards **mutation-verified** (each broken, watched to fail,
  restored). `references/white.md` corrected in the same commit: it told runs
  to check `git status data/`, which is blind — `app.db` is gitignored, so the
  status is clean no matter what a test wrote.
- **Verified this run:** #126's traversal fix is **shut on the live instance**
  (probed post-deploy; see Measurements). The isolation sweep is **live** —
  an injected unclassified route failed `test_every_route_is_classified`,
  confirmed by mutation and then removed.
- **Fixed and landed (previous run):** pre-auth path traversal in the SPA catch-all
  (`api/app.py`), which served files resolved from `WEB_DIST / full_path`
  with no containment — an arbitrary out-of-tree read reachable without a
  session. Fixed by serving root files from a name→path dict keyed on the
  request, so no user input reaches the filesystem call. Mutation-verified
  test. Closed the two open CodeQL `py/path-injection` alerts. Landed as
  [#126](https://github.com/aasquier/sylvan-library/pull/126). The CodeQL
  fight (four commits) is written up in `references/white.md`.
- **Queued for Aaron:**
  1. **ReDoS cluster — 6 open CodeQL `py/polynomial-redos` warnings.**
     `auth/users.py` (`EMAIL_RE`, on the unauthenticated claim path) and five
     in `decks/decklist.py` (`_MARKER`, `_BRACKET`, `_PRINTING`, `_QTY`,
     `_HEADER`). Polynomial, low-impact; worst case is slow parsing on a long
     crafted input, never a wrong answer. Not fixed because anchoring six
     patterns is behaviour-sensitive on a load-bearing parser. Suggested
     direction: a max-length bound on the pasted decklist and the email
     *before* the regex runs, whose value is Aaron's call — cheaper and safer
     than rewriting the patterns. Wants a per-pattern test.
  2. **Package licence undeclared in `pyproject.toml`.** `[project]` has no
     `license` key, so the wheel metadata reads as UNKNOWN despite the MIT
     `LICENSE`. One-line change, but the correct form is
     setuptools-version-sensitive and only the `image` CI job can fully verify
     it — so it is queued (land it alone on a watchable branch), not bundled.
     *Confirmed still true 2026-08-16 (rainbow), and now from the primary
     source rather than from pyproject: a licence sweep of the installed
     metadata reports `mtg-lab  UNKNOWN` while every one of its 44 dependencies
     reports a real licence.*
  3. **Widen CI's card-data filename scan to catch a `.duckdb` anywhere.**
     `no-secrets-or-card-data` anchors that rule to `^data/.*\.(json|duckdb)$`,
     so a pool committed outside `data/` — the likely accident, since
     `MTGLAB_DATA_DIR` is env-overridable and somebody points it at a scratch
     dir inside the repo — passes the tracked-tree scan. The image job's
     `find / -name "*.duckdb"` is the backstop, but only for what ships. One
     added alternative (`|\.duckdb(\.wal)?$`) closes it; it cannot false-positive
     (no tracked file matches). **Not done this run only because `ci.yml` is
     Red's file and Red was running concurrently** — a deliberate concurrency
     call, not a doubt about the fix. Wants an evaluating test (the #86 lesson:
     assert the pattern against a truth table, never grep the workflow).
- **Deferred:**
  - `pytest-xdist` for parallel tests — a new dev dependency, so Aaron's call.
    **Evidence gathered this run, and it is favourable.** xdist is
    process-based, so the shared mutable state that would matter is per-worker:
    `config`'s path globals are module-level (restored by `use_paths`, never
    reassigned), and `tiny_pool`'s `_CACHE_DIR` is a per-process
    `TemporaryDirectory` copied into per-test `tmp_path`s — N workers pay one
    ~1s ingest each. The real `data/` is now *proven* untouched (the new
    detector), the real pool is read-only in the 2 skipped tests, and the real
    `decks/` is read-only in `test_edit.py` (it asserts the file is unchanged).
    One open risk: `.hypothesis/` is a single shared directory every worker
    writes; CI already sets `database=None`, local `dev` does not.
  - **Parameterize `cards/db.py::snapshot_prices`.** It interpolates `on_date`
    into `DATE '{on_date}'` and into the count query. Not reachable today — the
    only production caller (`cli.py:108`) passes no date, and the parameter is
    exercised only by a test with a literal — but it is string-built SQL taking
    a caller-supplied string. Trigger: the first time `on_date` is wired to a
    route, a scheduled job argument, or anything a user can influence.
    (Audited alongside it and *fine*: the other three f-string `execute` sites
    interpolate module constants only — `SCHEMA_VERSION`, `_ADDED_COLUMNS`, and
    two hardcoded table names.)
- **Measurements (2026-08-16, rainbow):**
  - Full suite: **1926 passed / 2 skipped in 148.0s**, skip gate on 2, `data/`
    clean afterwards. *Measured under concurrent load — four sibling polish
    agents on the same Intel Mac — so treat it as an upper bound, not a clean
    baseline.* Slow tail unchanged in shape: `test_sim_tier1` land sweep 6.07s,
    then `test_sim_tier1` 2.63/2.43s, `test_wheel` 2.24s, `test_api` 2.22s;
    nothing new in the top 25.
  - Test-count delta from the fix: 1444 → 1926 on a documented `.[dev]` install
    (474 previously skipped, 8 newly added).
  - **Live instance, #126 verified post-deploy.** Nine traversal payloads
    against the pre-auth SPA catch-all — `/../../../../etc/passwd`,
    `/....//....//etc/passwd`, `/..%2f..%2f..%2f..%2fetc/passwd`,
    `/%2e%2e%2f…`, `/etc/passwd`, `/../app/pyproject.toml`,
    `/../../data/app.db` and two more — every one returned the SPA shell
    (200, `text/html`, 1754 bytes, byte-identical to `/index.html` and
    `/library`). No file contents, no 500s. `/assets/../../../../etc/passwd`
    is refused 404 by the static mount. `/api/nope` answers **401**, not 404:
    the middleware refuses before routing, as designed.
  - Licences swept 2026-08-16 (rainbow), from installed metadata not summaries:
    **45 Python distributions** — MIT/BSD/Apache/MPL/PSF/MIT-CMU/MIT-0/CC0/
    Zlib/0BSD; **163 npm packages** — 130 MIT, 15 ISC, 6 Apache-2.0, 3
    BSD-3-Clause, 2 each MIT-0/BSD-2-Clause/MPL-2.0, 1 each BlueOak-1.0.0/
    CC0-1.0/"MIT AND ISC". **Zero AGPL/GPL/SSPL/UNLICENSED on either side.**
    Only `UNKNOWN`: the package itself (queued item 2).
  - `animist verify`: both recipes held. Committed media unchanged — 78 tarot
    + 4 ambience ivy; the 4 ivy copies under `web_dist/assets/` are
    **sha256-identical** to the recipe-verified sources, so the build output is
    covered transitively. No hand-placed binary, no committed font, no
    `@font-face`, no Wizards art under `git ls-files`.
  - Wizards art is runtime-only and **every hotlink renders a visible credit**,
    checked against the JSX rather than the docstring claiming it: seven
    `PageMasthead` call sites (credit is a required prop) plus the four
    non-masthead paintings — wheel (Gelon), keeper (Rafater), lab bench
    (Arthur Yuan), Ludevic (Aaron Miller). Fan Content + Scryfall attributions
    present in `App.tsx`. No monetization surface anywhere.
  - CodeQL: **6 open alerts, all `py/polynomial-redos`, unchanged** — same five
    patterns in `decks/decklist.py` (lines 145/150/163/171/211) and
    `auth/users.py:153`. The two `py/path-injection` from last run are closed.
  - Security spot-checks all still hold: cookies `HttpOnly; Secure;
    SameSite=Lax`; Argon2id at the OWASP minimum (m=19456 KiB, t=2, p=1);
    tokens in the URL fragment only (a query-string token is refused, and
    `Claim.test.tsx` pins it); 429 with `Retry-After` on login and reset;
    `include_email=True` has exactly one caller (`api/admin.py`) beside
    `mtglab users list` printing to the maintainer's own terminal.
- **Measurements (2026-08-16, first run):**
  - `animist verify`: both recipes held (tarot, ambience).
  - Committed media: 78 tarot (`RWS1909`, 1909 Rider PD), 4 ambience ivy (CC0,
    recipe-verified). No hand-placed binary bypassing the pipeline; no Wizards
    art under `git ls-files`. Licence gate: CC0/PD only, no `--force`.
  - No monetization surface; Fan Content + Scryfall attributions present.
  - Dependency licences swept 2026-08-16: Python deps MIT/BSD/Apache/MPL/PSF/
    CC0; npm direct deps MIT except TypeScript (Apache-2.0). No AGPL.
  - Security: cookies `HttpOnly; Secure; SameSite=Lax`; Argon2id at OWASP
    minimum; tokens via URL fragment; email omitted from `User.as_dict()`
    (2 sanctioned callers); SQL parameterized.
  - Testing: full suite ~1920 passed / 2 skipped in ~155s locally; skip gate
    at 2; `data/` not dirtied.

## Blue — Craft & Knowledge

*Python craft · TypeScript/React craft · Claude-first docs & memory*

- **Last run:** 2026-08-16 (rainbow). First Blue run — everything here is baseline.
- **Fixed and landed:**
  1. **`cards/db.py` graduated off the strict-mypy exception list**, which is
     now one module (`cli.py`) rather than two. All 24 errors were annotations,
     not a rewrite: `con` parameters took the `Connection` alias the module had
     already defined for exactly this, the JSON helpers took `dict[str, Any]`,
     and two `Any` returns became named locals. No behaviour changed.
  2. **`typescript/no-non-null-assertion` is now an oxlint error**, off for
     `*.test.ts(x)` only. `web/README.md` has stated "no non-null assertions
     outside test files" as load-bearing for months with **nothing enforcing
     it**, and four had drifted in: three `map.get(k)!.push(…)` group-bys
     (`components/review.tsx`, `routes/Library.tsx`, `routes/DeckDetail.tsx`)
     and `getElementById('root')!` in `main.tsx`. Fixed with get-or-create and
     an explicit throw. Rule verified by mutation — with the old code in place
     it reported exactly those four and nothing else.
  3. **Doc corrections** (the facet's own work): `components/stancemenu.tsx`
     was named as "the one control" in *both* CLAUDE.md and `web/README.md`
     and **does not exist** — it folded into the settings gear
     (`components/settings.tsx`) when that landed. `lore.py`, `decks/wheel.py`
     and `api/argueruns.py` were missing from CLAUDE.md's architecture block
     entirely. The "`colors.py` and `glossary.py` are the two modules" passage
     is now three (`lore.py` is reference prose of the same kind and says so in
     its own docstring). `user_decks.yaml` in HOSTING.md read as a filename and
     is a column.
- **Queued for Aaron:**
  1. **`cli.py` is the last strict-mypy exception and it has grown: 79 → 109.**
     59 `no-untyped-def` and 48 `no-untyped-call` (the argparse handlers calling
     each other), so it is annotations rather than a rewrite — just a lot of
     them, touching one 1900-line file. Too big to ride along with a polish
     fix set; it wants its own branch. The decision is whether that branch is
     worth an afternoon, given the module is exercised end-to-end by the suite
     and the list has been "meant to shrink" since it was written. **The number
     going up is the argument for doing it**: a module on the list absorbs
     every new untyped function without a word.
  2. **Adopt Claude Code hooks for the two traps that have already cost hours.**
     `.claude/` holds `launch.json` and `skills/` and **no `settings.json`**, so
     the harness enforces nothing. Two of this project's most expensive rules
     are prose-only — *never `git add -A`* (`decks/` is the app's live data
     directory; it once swept a test deck into a "docs only" PR) and *never
     `git stash` on this repo* (index corruption; commit WIP instead). A
     `PreToolUse` hook on `Bash` matching those two command shapes turns both
     into a refusal instead of a paragraph somebody has to have read. This run's
     whole thesis is that a rule enforced by nothing drifts, and these are the
     two with a measured cost. Queued rather than done because it changes how
     every session behaves and that is Aaron's call, not a polish run's.
     A third candidate, weaker: a `PostToolUse` hook reminding that `web_dist/`
     needs rebuilding after an edit under `web/src`.
  3. **`api/service.py` reaches past `cards/db.py` to `duckdb` directly**
     (`import duckdb; duckdb.connect(..., read_only=True)`), which is the one
     live exception to CLAUDE.md's "DuckDB stays behind `cards/db.py`". It is
     justified — `db.connect` creates the file, runs DDL and `ALTER`s, none of
     which a read-only app handle may do — but the rule now has an undocumented
     hole. Two ways out: add `db.connect_readonly()` and move the call, or write
     the exception into CLAUDE.md. Not done here because the read-only path is a
     deployed degradation path and getting it wrong is an outage, not a lint.
- **Deferred:**
  - **Adopting ruff's `N` group (39 in `src`, 1 in `tests`).** The only excluded
    group whose cost is now plausibly payable, but naming rules rename things,
    which is a wide diff for no behaviour change. Trigger: a session already
    renaming in the same modules.
  - **`ARG`, `PT`, `SLF` stay out** and the re-measure says why (below) — all
    three are overwhelmingly *test* findings, and the fixture/override idiom
    they fire on is correct here.
- **Measurements (2026-08-16, uncontended unless noted):**
  - **mypy:** clean over 82 source files. Strict-exception list: `cards/db.py`
    24 → **graduated**; `cli.py` **109** (was 79 when the checker landed —
    re-measured by removing the override block, not estimated).
  - **ruff:** clean. Excluded groups re-measured, split src/tests for the first
    time — the split is the finding: `ARG` **620** (16 src / 604 tests, was 310
    total), `PT` **112** (0 / 112, was 65), `SLF` **67** (0 / 67, was 63), `N`
    **40** (39 / 1, was 38). Everything except `N` lives almost entirely in
    `tests/`, so the recorded "cost" was never really a cost to `src`.
  - **pytest:** 1926 passed, 2 skipped (the pinned skip count) in **159s** on
    this Mac, uncontended and post-#129. Incidental cross-check of White's
    conftest fix: a full run left `data/app.db`'s mtime untouched, where the
    pre-#129 run in this same worktree had written it.
  - **frontend:** `npm --prefix web run check` green — typecheck, oxlint with
    the new rule, 432 Vitest tests across 23 files in ~27s.
  - **Layering, grepped not trusted:** `api/` imports `cli.py` **nowhere**;
    `mana.py` and `sim/` are stdlib+numpy only; `anthropic`, `PIL` and `dotenv`
    have **no** module-level import anywhere in `src/` (`argon2` does, and
    pyproject argues for it deliberately); no PEP 695 / 3.12-only syntax in
    `src/`, so the `>=3.11` floor holds. One exception, queued above.
  - **Frontend compatibility:** **zero regex lookbehind** under `web/src`
    (Safari 15 is the dev machine); no `.at()`, `structuredClone`, `findLast`,
    `toSorted` or `Object.hasOwn`; no `forwardRef`, `React.memo`, `defaultProps`
    or `propTypes` — React 19 idiom throughout. Index keys appear only on
    static decorative lists.
  - **Doc/tree consistency:** every source path named in CLAUDE.md, ROADMAP.md,
    `web/README.md`, ENGINEERING.md, HOSTING.md and the two skills now resolves
    against `git ls-files`. Branch protection re-read from the API: **six**
    required checks (`test (3.11)`, `test (3.12)`, `frontend`,
    `no-secrets-or-card-data`, `image`, `dependency-review`) — CLAUDE.md's "all
    six" is correct; CodeQL runs but is **not** a required context.

## Black — Ruthless Efficiency

*Claude API spend · static assets · performance*

- **Last run:** 2026-08-16 (rainbow)
- **Fixed and landed:**
  1. **The challenge board asked the pool once per deck.**
     `service.challenge_progress` (`/api/colors/progress`) ran `get_cards`
     inside its loop over `decks.slugs()` — one query per deck, for one name
     each. Now: read every deck, then one batched call. **6 queries → 1;
     p50 306ms → 94ms** on the real six-deck library (quiet machine, full
     pool). Linear in a library aimed at 32 slots, so the win grows.
     Mutation-verified test (`test_challenge_progress_asks_the_pool_once`)
     counts the calls rather than the milliseconds — wall clock is noise on a
     shared machine, the query count is what moved.
  2. **The spend ledger described cached tokens in a way its own data
     disproves.** `mtglab claude usage`, the `claude_usage` schema comment and
     `modes.Turn.cache_read_tokens` all called cache reads "the slice of
     'tokens in'". They are counted *beside* `input_tokens`, not inside it —
     the API reports `input_tokens` as the uncached remainder only. Locally
     that reads as 2,354 in vs **496,654 cached**, which is impossible as a
     slice and correct as a sibling: the prompt cache is doing almost all the
     work. Copy corrected in all three places, and it now says out loud that
     cache *writes* are recorded nowhere.
- **Queued for Aaron:**
  1. **Cache-write tokens are invisible, and they are the priciest class.**
     `modes.converse` records `input_tokens`, `output_tokens` and
     `cache_read_input_tokens`, and drops `cache_creation_input_tokens`
     entirely. Writes bill at **1.25× input** — the most expensive token this
     project buys — so the usage table is a *floor* on the bill, not the bill.
     It matters most exactly where the cache works best: a ~4k-token system
     prefix rewritten once per conversation past the 5-minute TTL costs more
     than everything currently counted. Fix is one column plus one assignment,
     but it is a **schema migration** (v9) on a forward-only ladder that
     applies on boot unwatched — so per CLAUDE.md it wants its own branch
     merged while somebody is watching, not a ride-along.
  2. **The theme conversation's second cache breakpoint cannot be read back on
     the next turn.** `theme._messages` puts the marker on the *closing
     instruction*, which is stripped and re-appended at the end of every turn,
     so turn N's cached region is not a byte-prefix of turn N+1's request —
     verified deterministically, no API calls, by rendering two consecutive
     turns and comparing. Its docstring claims the opposite ("a conversation's
     history is the part that grows"). It does pay inside one `converse` call
     (a `pause_turn` resume), just not across conversation turns. **Verified
     fix:** move the marker to the last settled *transcript* block, before the
     closing — that region *is* a prefix of the next turn's request. Queued
     rather than fixed because it changes what a paid path caches and the
     payoff can only be confirmed by spending; pairs naturally with (1), which
     is the instrument that would show it working.
- **Deferred:**
  - **Interview and single-card argue have neither a cache nor an in-flight
    dedupe key** — the only two paid modes with neither, and no written
    argument for it. In practice both are guarded client-side
    (`disabled={busy}` in `components/deckedit.tsx`), and the deck-sweep argue
    button that *isn't* guarded is the one covered server-side by
    `jobs.submit(key=…)` — so the coverage is coherent, just not uniform.
    Trigger: either endpoint becoming a background job, or gaining a second
    caller that is not the deck page.
  - **Long max-age for the immutable media** (219KB `ivy-canopy.webp`, three
    sprigs, 78 tarot PNGs). Tempting, and it collapses into the same trap as
    the JS: `assetFileNames: 'assets/[name].[ext]'` means an animist rebuild
    reuses the filename, so a long max-age would serve a stale asset. Trigger:
    media bytes growing enough that per-navigation revalidation matters — the
    fix would be content-hashing *media* names only, which do not need to stay
    diff-legible the way JS chunk names do.
- **Checklist corrected this run:** `references/black.md` said committed assets
  "should be aggressively cacheable (hashed filenames from Vite get long
  max-age)". This repo deliberately does the opposite and is right to: Vite is
  configured for stable filenames because the bundle is committed, and
  `Cache-Control: no-cache` is the half that makes stable names safe — added
  2026-08-13 after Safari served a stale `DeckDetail.js` against a redeployed
  server until the page crashed. Reference updated so the next Black run does
  not propose the change that already broke production.
- **Measurements (2026-08-16, quiet machine — serial rainbow, no siblings):**
  - **Claude spend to date** (local ledger, all of it 2026-08-16): 13
    conversations / 14 requests / 2,354 input / 25,000 output / 496,654 cache
    reads. Per mode: `theme-conversation:fortune-teller` 11 conv (1,520 in /
    15,592 out / 266,205 cached), `commander-dossier` 1 conv, 2 req (832 /
    9,241 / 230,449), `theme-conversation:chef` 1 conv (2 / 167 / 0). At Sonnet
    5 introductory rates ($2/$10 per MTok, through 2026-08-31; $3/$15 after)
    that is **≈$0.35 total, ≈$0.027 per conversation** — *excluding* unrecorded
    cache writes. Four of six modes have never run locally.
  - **Prompt cache is working**: cache reads outnumber fresh input **211:1**.
  - **Per-mode cached prefix** (system + tools, est. chars/4): interview 1,577
    tok · slot-argument 2,210 · dossier 1,580 · research 1,367 ·
    theme-conversation 3,495 · theme-proposal 4,427. All above Sonnet 5's
    1,024-token minimum cacheable prefix, interview and research with the
    least margin. Per-persona theme system prompts: plain 3,466 tok → chef
    3,875 → fortune-teller 3,962.
  - **Spend knobs**: model `claude-sonnet-5` everywhere (`MTGLAB_CLAUDE_MODEL`
    overrides, for A/B only). `max_tokens` 8,192 (interview, argue, theme
    conversation) / 16,384 (dossier, research, theme proposal). `effort: high`
    on all six, argued — lower levels reach for tools less, and a mode
    answering from recall instead of `get_cards` is rule 1 failing quietly.
    Web search `max_uses`: dossier 4, research 4 (shares the dossier's), theme
    proposal 3, theme conversation 1. `MAX_TOOL_TURNS` 6; theme conversation
    caps at 4, dossier/research/proposal at 8.
  - **Refusable-before-the-call** holds on every paid surface: no key (503),
    bad card/stance/transcript (422), floor not reached (409) — all refused in
    the request, so no mode errors *after* spending.
  - **Bundle** (committed `web_dist`, post-#130, gzip -9): total 1,196,126 raw
    / **511,611 gzip**. `charts.js` 398,447 / **111,212** (recharts, the known
    heavyweight — already its own chunk, loaded only where a chart renders);
    `app.js` 294,315 / 91,749; `DeckDetail.js` 75,090 / 17,756; `NewDeck.js`
    59,667 / 15,234; `index.css` 56,259 / 12,187; `ivy-canopy.webp` 219,330
    (WebP, gzip makes it larger). Route-level code splitting is already in
    place — nine `lazy()` routes in `App.tsx`, one chunk each.
  - **Live instance** (5 samples each, p50 time-to-first-byte from this Mac):
    `/` 240ms · `/assets/app.js` 192ms · `/assets/charts.js` 177ms ·
    `/assets/ivy-canopy.webp` 167ms · `/api/health` 211ms. API routes answer
    401 unauthenticated (auth is on), 164–169ms. Most of that is RTT to `sjc`;
    server work on `/api/health` is ~45ms.
  - **Compression and caching headers, read live**: `content-encoding: gzip` on
    HTML, JS and CSS; `vary: Accept-Encoding` present. No brotli. Every asset
    is `cache-control: no-cache` with a strong `etag`; a conditional request
    returns **304** correctly, so the cost is one revalidation RTT per
    navigation (HTTP/2-multiplexed), not a re-download.
  - **Static assets over hotlinks**: the served app references exactly one
    external host at runtime — `cards.scryfall.io`, for Wizards art, which
    White's licensing verdict keeps as a hotlink. It already carries a
    `<link rel="preconnect">`, which is the right mitigation for a mandatory
    hotlink. **No CDN for code, fonts, CSS or scripts**; no `@import url(http…)`,
    no `fonts.googleapis`/`gstatic`, no `unpkg`/`jsdelivr`. Remaining external
    strings in the bundle are the SVG namespace (`w3.org`) and doc URLs inside
    vendored library error messages — inert, nothing fetched. Nothing in
    category (c): no dead or replaceable external URL found.
  - **DuckDB query patterns**: swept `cards/db.py` callers for n+1 by walking
    the syntax tree for pool calls inside loops. One real hit — the
    `challenge_progress` fix above. Every other `get_cards` caller already
    batches. `get_cards` itself is a CTE + three hash semi-joins with a
    measured 108ms → 62ms note already in place.
  - **Tier 1 / `SIM_VERSION`**: not touched this run, deliberately. No engine
    change, so the ADR 18 cache is intact and the determinism digest did not
    move.

## Red — Speed & Alarum

*CI/CD · alerting & self-healing*

- **Last run:** 2026-08-16 (rainbow). First Red run; everything below is a
  first baseline, so read the numbers as a starting point and not as a trend.
- **Fixed and landed:** `concurrency` groups on `codeql.yml` and
  `dependency-review.yml`, which `ci.yml` has had since it was written and
  these two never did. **The waste was observed rather than theorised**, on
  this same afternoon: Blue pushed a fixup at 20:08:10Z and again at
  20:09:06Z, `ci.yml` run `31969659465` was **cancelled** as designed, and
  CodeQL run `31969659483` for the very same superseded commit ran happily to
  completion. Cancelled on `pull_request` only for CodeQL, because a cancelled
  `main` run leaves the Security tab without a baseline for that commit and
  the next PR's "new alerts only" comparison has nothing to compare against;
  unconditionally for dependency-review, which has no `main` run at all.
  `timeout-minutes: 10` on the dependency-review job, the one job in the repo
  that was still on the six-hour default.
- **Fixed and landed — the card-data scan is no longer anchored to `data/`**
  (White queued this one for Red deliberately, `ci.yml` being Red's file).
  `no-secrets-or-card-data` asked `^data/.*\.duckdb$`, so a pool built into
  `tests/fixtures/` or dropped at the repository root passed a scan named
  after it — the rule only ever asked *where* a file was, when what makes it a
  rule 5 violation is what it *is*. Widened with `\.duckdb$`, `\.jsonl\.gz$`
  and `\.json\.gz$`, the last two being Scryfall's bulk downloads, which the
  `image` job already refuses to find inside the container and nothing refused
  in the tree. Mutation-verified against a synthetic list: the old pattern
  passes five real leaks the new one catches, and both pass every legitimate
  file — including all 418 tracked ones, where the new pattern matches exactly
  what the old one did, which is nothing. **The asymmetry is the interesting
  part and is written into the comment:** `.duckdb`/`.jsonl.gz`/`.json.gz` may
  match anywhere because nothing legitimate here is named that; bare `.json`
  emphatically may not (`package.json`, `package-lock.json`, `tsconfig`), so
  it stays pinned to `data/`.
- **Also corrected, two notes that had drifted from the truth:** `ci.yml`'s
  Trivy note claimed the action was "unpinned, like every other action in this
  file" when all eleven are SHA-pinned — and the worry it recorded (pinning a
  scanner pins its database) turns out not to apply, which the *run log* shows
  rather than the docs: the vulnerability data restores from a separate
  date-keyed cache and moves daily. And `docs/HOSTING.md` §5's Backups runbook
  described the manual `app.db` procedure in detail and was silent about the
  volume snapshots Fly takes daily on its own.
- **Checklist conflict, now fixed in `references/red.md`:** the reference said
  the six required checks were "four `ci.yml` jobs, `dependency-review`,
  CodeQL". Read from the protection setting, they are `test (3.11)`,
  `test (3.12)`, `frontend`, `image`, `no-secrets-or-card-data`,
  `dependency-review` — `ci.yml` supplies **five** contexts because `test` is
  a matrix, and **CodeQL is not required at all**. `codeql.yml`'s own header
  says so and CLAUDE.md's count was right; only the reference had drifted.
- **Queued for Aaron:**
  1. **Nothing tells Aaron the site is down, and one specific failure is
     invisible to everything now watching.** Fly's HTTP check is configured
     and passing, but on Fly a failing service check makes the proxy *stop
     routing* — it does not restart the machine. The machine's restart policy
     is `on-failure`/`max_retries: 10`, which fires only if the process
     *exits*. So an app that hangs while still holding port 8080 is a total
     outage that nothing detects and nothing recovers. Recommendation, in
     this order: **UptimeRobot free tier** (50 monitors, 5-minute interval, no
     card) hitting `/api/health`, wired to **Pushover** (one-time $5, one
     platform) for the phone. Pushover over the alternatives on reliability
     rather than price: carrier email-to-SMS gateways are being retired and
     fail by silently dropping, which is the worst possible failure for an
     alert channel; ntfy.sh's free tier has no delivery guarantee and
     self-hosting it puts the alarm on infrastructure that can fail with the
     thing it watches; Twilio is genuinely reliable but is a metered bill and
     a phone number to maintain. Pushover is a push, not a text — it lands on
     the lock screen the same way without a carrier in the path. **Total cost
     $5, once.**
  2. **Whatever monitor is chosen must be configured for GET, not HEAD.**
     Measured today: `HEAD /` and `HEAD /api/health` both answer **405**,
     while GET answers 200. `fly.toml` already sets `method = "GET"`, which is
     why Fly's own check works. A monitor left on a HEAD default would alert
     continuously against a perfectly healthy site, and an alert channel that
     cries wolf in week one is an alert channel that gets muted.
  3. **A health endpoint that detects sickness, not just liveness — and the
     non-obvious half is that it must keep answering 200.** `/api/health`
     today opens DuckDB, counts oracle cards and printings, globs the bulk
     directory and counts decks. It never opens `app.db` and never looks at
     free space, so a corrupt auth database leaves the check green while every
     login fails, and a full volume leaves it green while every write fails.
     The trap in fixing it: Fly stops routing on a *failing* check, and with
     one machine that converts "logins are broken" into "the site is
     entirely down". So the recommendation is to **report the extra facts in
     the body and keep the status 200**, and let the external monitor decide
     what is worth waking somebody for. Roughly: `app_db` (does it open),
     `disk_free_mb`, `schema_version`.
  4. **`sha_pinning_required` is available and off.** GitHub now offers
     enforced SHA pinning for Actions free on public repos; the repo already
     pins all eleven references by hand and Dependabot keeps them current, so
     turning it on makes an existing convention structural instead of
     voluntary. `allowed_actions` is also `"all"`, which is the widest
     setting. Both are repository settings, so they are Aaron's to flip and
     were deliberately not touched by this run. Free, and pure win.
  5. **A merge queue, the next time more than two PRs are open at once.**
     Protection has `strict: true` (branch must be up to date), so with N open
     PRs every merge invalidates the other N−1 and forces a rebase plus a full
     re-run. Rainbow going serial (#128) dissolved the immediate case — one
     branch at a time never collides — so this is worth having only if
     concurrent PRs come back. Free for public repos; it changes contributor
     workflow, so queued rather than adopted.
  6. **A deploy does not take a snapshot, and that is the wrong way round.**
     Four deploys landed this afternoon (machine v61 → v65) and the newest
     volume snapshot still predates all four: Fly snapshots on its own daily
     schedule, which has no relationship to when the volume is at risk. The
     moment the volume is *most* at risk is the boot after a merge, because
     `auth/db.py`'s ladder is forward-only and applies unwatched (ADR 23) — so
     the one deploy that needs a rollback point is the one guaranteed not to
     have a fresh one. HOSTING §5 already says to take a manual `app.db`
     backup before a migration; the gap is that nothing *enforces* it and the
     schedule will not cover for a forgotten one. Cheapest honest fix is a
     step in the `deploy` job that snapshots before `flyctl deploy` — but that
     needs a `FLY_API_TOKEN` scope check and a decision about failing the
     deploy when the snapshot fails, so it is Aaron's call, not a safe fix.
- **Deferred:**
  - **Speeding up the `image` job — deferred because it would not help.**
    Its 292s median is 214s of arm64-under-QEMU, and that 214s is 142s of
    `pip install` plus 46s of `python -m venv`, neither of which caches:
    `RUN python -m venv` sits *below* `COPY src ./src` in the Dockerfile so
    every commit invalidates it, and the two buildx `type=gha` caches share
    one scope, so each run's amd64-only build overwrites the arm64 layers the
    previous run wrote. Both are fixable (move the venv above the COPYs; give
    each build its own `scope=`), and neither is provable on this Mac, which
    has no container runtime at all — so it would have to land alone and be
    watched. **The reason not to bother yet is stronger than "small win":**
    across ten runs the critical path was `image` six times and `test (3.12)`
    four, at medians of 292s and 284s. They are close enough that which one is
    the bottleneck *flips run to run*, so fixing either alone buys almost
    nothing. Trigger to revive: the test job gets materially faster (White's
    deferred `pytest-xdist` would do it), which would make `image` the
    bottleneck properly and worth the branch.
  - **A Fly volume-snapshot restore has never been performed.** Snapshots are
    current and daily, but the restore path is a *new* volume plus a machine
    re-attach, which is downtime and steps nobody has walked. Documented as
    unverified in HOSTING §5 rather than written up as a procedure, because
    writing down an untried runbook is worse than admitting there isn't one.
    Trigger: any change to the volume's shape or size, or the first time a
    real restore is needed — do it once deliberately before then.
  - **`www.sylvan-libraries.com` does not resolve or answer** (connection
    fails; the Fly cert covers the apex only). Harmless until a friend types
    it. Trigger: anyone reports the site not loading and turns out to have
    typed `www.`.
- **Measurements (2026-08-16):**
  - **CI per-job medians**, n=10 runs — the ten most recent, which are #128
    through #132 and their pushes, all taken after rainbow went serial, so
    **nothing here is contended**:
    `test (3.11)` **158s** (149–172) · `test (3.12)` **284s** (241–386) ·
    `frontend` **36s** (29–42) · `no-secrets-or-card-data` **5s** (4–7) ·
    `image` **292s** (49–336) · `deploy` **78s** (72–79, n=5).
    Separate workflows: `codeql` **~60s** on a PR, ~90s on a push;
    `dependency-review` **~12s**. One-day window, so this is a baseline and
    not yet a trend — but a clean one.
  - Full run wall clock **241–386s, median ~307s**, plus ~78s of deploy on a
    push. **The critical path flips**: `image` was the longest job in 6 of 10
    runs and `test (3.12)` in the other 4. That is the number to re-check
    before anyone optimises one job — at 292s vs 284s there is no bottleneck
    to attack, only a tie.
  - The 386s `test (3.12)` (run `31970110603`) is the observed tail and did
    not reproduce; the other nine sit in 241–288s. Worth remembering only so
    that a single slow run is not read as a regression.
  - Where the two long jobs go: `test (3.12)` is ~233s of `Tests`, roughly
    100s of it coverage instrumentation (`test (3.11)` runs the same suite
    bare at ~130s) — a deliberate trade, not a regression. `image` is 214s of
    arm64 QEMU: `pip install` 142s, `python -m venv` 46s, export 19s.
  - **Cache health: all green, read from run `31965727360`'s log rather than
    assumed.** pip hit (~107 MB, keyed on `pyproject.toml`), npm hit (~47 MB),
    QEMU binfmt hit, Trivy binary hit, Trivy vulnerability DB hit on a
    *date*-keyed key (so the SHA pin holds the action still while the data
    still moves daily), buildx amd64 layers all `CACHED`. The one miss is
    arm64, above.
  - No queueing observed — jobs started 2–5s after run creation across all
    ten, and serial rainbow means one run in flight at a time. Public repos
    get 20 concurrent jobs and one run is five, so there is ~4× headroom;
    concurrent PRs are what would eat it.
  - Actions hygiene: all **11** action references pinned to 40-char SHAs with
    version comments; no `pull_request_target`; workflow-level
    `permissions: contents: read` on all three files, with
    `security-events: write` scoped to the CodeQL job alone; default workflow
    permissions `read`; `can_approve_pull_request_reviews` false. Pinned
    invariants verified, not assumed: skip gate `expected=2` (and the local
    suite really does skip exactly 2), `image` the only container build,
    `deploy` `needs` all four `ci.yml` jobs and is gated to `refs/heads/main`
    outside the event check.
  - **Alerting posture inventory** — Fly HTTP check GET `/api/health`, 30s/5s/
    10s grace, passing, **does not restart on failure** · machine restart
    policy `on-failure` max 10, fires only on process exit · Dockerfile
    `HEALTHCHECK` present but inert on Fly Machines (`fly.toml`'s check is the
    live one, and the file says so) · GitHub deploy-job failure email to the
    actor · Fly account status emails, unverified whether they reach Aaron ·
    **external uptime monitoring: none · phone alerting: none.**
  - **External probe** (2026-08-16 ~20:55Z, quiet machine). Liveness and the
    gate, which is Red's half: `GET /api/health` **200**, `GET /` **200**,
    `GET /api/decks` **401**; `HEAD /` and `HEAD /api/health` both **405**.
    HTTP/2, TLSv1.3. Security headers on 200s: HSTS 31536000,
    `X-Content-Type-Options`, `X-Frame-Options: DENY`, `Referrer-Policy`,
    `Permissions-Policy`. **Response times are Black's**, measured the same
    afternoon and not re-taken here: `/` 240ms, `/api/health` 211ms p50 TTFB
    with ~45ms of server work.
  - **TLS expiry 2026-11-11** (Let's Encrypt, `CN=sylvan-libraries.com`,
    issued 2026-08-13, ~87 days remaining, Fly-managed and auto-renewing,
    **apex-only SAN**).
  - Instance: machine `84e19ef25041e8`, **version 65** (v61 at the start of
    this run — four merges landed during it, each replacing the machine, and
    the check stayed passing through all four), `iad`, `shared-cpu-1x`/1 GB,
    started, 1/1 checks passing. Health body: pool true, 35,390 oracle cards,
    107,338 printings, 7 decks, `pool_stale` false.
  - Volume `mtglab_data` 3 GB, encrypted. **4 snapshots, one per day, newest
    6h old, 5-day retention, 286 MiB stored — and unchanged across four
    deploys**, which is the finding rather than the number. Five days is Fly's
    default and is the number that matters: a corruption nobody notices inside
    five days has no snapshot behind it.

## Green — Growth & Resilience

*Browser & mobile compatibility · cloud resource watch · scalability*

- **Last run:** 2026-08-16 (rainbow). First Green run; every number below is a
  first baseline, so read them as a starting point and not as a trend.
- **Fixed and landed — `prefers-reduced-motion` now covers every animation.**
  `.ready-banner` (the theme interview's "you can stop now" invitation) ran
  `bubble-in` unguarded. It was the *only* gap: all 43 animation declarations
  were resolved against the nine guard blocks, and the other apparent misses
  are covered by a base class the element also carries — `.crystal-haze-a/b/c`
  by `.crystal-haze-puff`, `.crystal-fog-*` by `.crystal-fog`,
  `.scene-lane-*` by `.scene-lane`, `.leaf-fall`/`.page-fall` by
  `.forest-ambience { display: none }`. **Source order is what makes those
  work**, not specificity: the guards are equal-specificity single classes and
  win only because they sit later in the file, which was verified against the
  *served* stylesheet's rule indices in a real browser (haze declared at rules
  63–65, guarded at 91), not against the source.
- **Fixed and landed — `tests/test_browser_floor.py`, which makes the
  compatibility floor a declared value instead of a remembered one.** See the
  queued item below for what it found. Three tests, all mutation-verified:
  lowering `FLOOR` to 15.0 lists all nine offending features with attribution,
  and swapping the lookbehind patterns for a string that *is* in the bundle
  fails as designed.
- **Checklist conflict, and it is the reason the drift was invisible:**
  `references/green.md` says to grep `(?<` under `web/src` every run and to
  check new features "before they are used". Both halves are wrong in the same
  way. **`web/src` is not what a phone parses** — every feature that actually
  raised the floor arrived through a dependency, so Blue's source grep this
  same afternoon was clean and correct while the shipped bundle carried nine
  above-floor features. And **`(?<` also matches a named capture group**
  (`(?<name>...)`, Safari 11.3), so the prescribed grep has a false positive
  built in; the hazards are `(?<=` and `(?<!` only. The new test does both
  correctly against `web_dist/assets`. The reference should be updated to say
  "run the test" rather than "run the grep".
- **Queued for Aaron:**
  1. **The stated compatibility floor is Safari 15; the shipped bundle needs
     Safari 16.4.** This is the run's headline and it is a decision, not a bug
     to fix quietly. Tailwind v4 emits **56 `@property` rules** and **10
     `color-mix(in lab, …)`** values (both Safari 16.4); React adds
     `Object.hasOwn` ×7, `structuredClone` ×1 and `reportError` ×4 (Safari
     15.4). Nine features over the line in total, none of them in a file we
     wrote. **What it costs below the floor is quiet rather than fatal**, which
     is why nobody noticed: `@property` registers the `--tw-*` variables that
     Tailwind composes shadows, transforms, filters and rings out of, so where
     the at-rule is ignored those `var()`s are invalid and the whole
     declaration drops — the page lays out correctly with its depth gone.
     **The sharp point: Safari 15 on macOS 12 is this dev machine's own
     browser**, so Aaron can settle the severity question in ten seconds on a
     browser he already has, and no one has looked. The decision is whether the
     floor *is* 16.4 now (in which case `references/green.md`, CLAUDE.md and
     the memory note that all still say 15 are wrong and should be corrected),
     or whether getting back to 15 is worth what it costs — realistically
     pinning Tailwind v3, which is a large change. `FLOOR` in
     `tests/test_browser_floor.py` records 16.4 as *observed*, deliberately not
     as chosen; changing it is one line.
     **Addendum 2026-08-16: the laptop can now test above the floor.** "No
     one has looked" is half-settled: a real-WebKit rig exists on the dev
     machine — Playwright pinned at **1.45.3**, the last release shipping
     macOS 12 browser builds, whose `webkit_mac12_special` build is
     **WebKit 17.4** — and the deployed site loads in it clean, iPhone 13
     viewport, zero console errors. That verifies the *above*-floor side;
     what Safari 15 loses below the floor is still Aaron's ten-second check
     on his own desktop Safari, and the floor decision itself is still his.
     Note the pin is permanent on this hardware: 17.4 is the newest WebKit
     macOS 12 will ever run, so a future floor above 17.4 puts engine
     testing back out of the laptop's reach.
  2. **Five of eight nav destinations are unreachable on a phone, and one of
     them is Learn.** Measured at 375px: the nav strip is 736px of content in a
     343px window (**47% visible**), `overflow-x: auto` with the scrollbar
     explicitly suppressed (`[scrollbar-width:none]`,
     `[&::-webkit-scrollbar]:hidden`) and **no mask, fade or any other
     affordance**. Visible: Library, Start a deck, Import. Off-screen: Card
     search, Simulator, Laboratory, **Learn**, Accounts — with "Card search"
     cut mid-word, which reads as broken rather than as "swipe me". The strip
     is scroll-only below Tailwind's `lg` (1024px), so this is every phone and
     every tablet; at 1024 it goes `overflow-visible` and all eight fit.
     **Commandment 2 is what makes this the priority it is** — Learn is the
     page built to teach a newcomer the vocabulary, and a newcomer on a phone
     cannot find it. Not fixed here because the primary navigation is a design
     decision (Commandment 1). Three options, cheapest first: an edge fade
     mask on the scroll container; letting the strip wrap to two lines below
     `lg`; a proper disclosure menu.
     **Landed 2026-08-16: the wrap.** Aaron hit this on his own phone the
     same day ("I can't navigate to the lab"), which settled the design
     question the run had queued: the strip now wraps whole entries below
     `lg` (all eight destinations visible at 375px, three short lines) and
     the scroll-with-hidden-scrollbar mechanism is gone. Same branch fixed
     the sharper half of what his phone showed: a failed lazy route chunk
     unmounted the React root — black page, dead nav — and now lands in
     `RouteErrorBoundary` (reload once, then an in-theme card).
  3. **Touch targets: 82% of the deck page's controls are under 44px.** 27 of
     33 interactive elements at 375px. The dense action cluster the checklist
     names — Write why, Card art, Ask Claude, Argue slot, Entomb, + Add a card,
     Bulk entomb…, Review with Claude — is uniformly **27–28px tall**. The two
     worst are disclosure buttons at **16px and 17px** ("Who is Arahbo?", "What
     is Claude allowed to do here?"). Raising these is a visual pass across the
     app rather than a polish trim, hence queued. The cheap version that moves
     no pixels is a pseudo-element hit area; the honest version is a spacing
     scale decision.
  4. **`env(safe-area-inset-*)` appears nowhere, and adding it alone would be a
     no-op.** `.library-whisper` (the sprout, fixed `bottom: 1.25rem`) sits in
     the home-indicator band on a notched iPhone. The fix is *two* coupled
     changes — `viewport-fit=cover` on the viewport meta **and** `env()` insets
     on the fixed layers — because iOS reports all insets as 0 without the
     former. `viewport-fit=cover` changes what the page does under the notch,
     so it wants Aaron's physical phone to verify; that is why this is queued
     rather than fixed.
  5. **The admin resource panel — which numbers, and the boundary with Red.**
     Red owns the alerting thresholds and made the sharp point that
     `/api/health` must keep answering 200 (a failing Fly check stops routing,
     turning "logins are broken" into "the site is down" on a single machine).
     Green's half is the *surfacing*: a `GET /api/admin/resources`, admin-
     mounted so the prefix middleware refuses it before routing and a
     logged-in non-admin gets 403 (ADR 17), classified `admin` in
     `tests/test_isolation.py`. It should report volume used/free/percent,
     `app.db` and `mtg.duckdb` sizes, the decks tree, `SCHEMA_VERSION`, newest
     snapshot age, and job-registry occupancy against `MAX_JOBS`. Deliberately
     a *separate* endpoint from `/api/health` so Red's constraint is not
     compromised: health stays a liveness 200, this is the dashboard.
- **Deferred:**
  - **The shelf's serialization** (`/api/decks`, evidence under Measurements).
    Real, and the first bottleneck by a wide margin, but the design point is
    10 concurrent and the last user waits ~1.8s — annoying, not broken. Fixing
    it means caching the shelf with invalidation on deck write, which is a new
    cache and a design decision. **Trigger:** the design point moves past ~25
    concurrent, or the deck count grows past ~20 (the cost is linear in decks).
  - **Widening the `NET` job pool past 2.** Not a bottleneck to fix — it is a
    deliberate spend guard (`jobs.py`: "a queue is a cheaper way to say 'not
    four at once' than a rate limiter nobody has written yet"). **Trigger:** a
    real per-user quota on the Claude surfaces exists, at which point the
    queue stops being the cost control and can widen.
- **Measurements (2026-08-16):**
  - **Volume — 99M used of 2.9G, 4%, 2.7G available.** Breakdown:
    `mtg.duckdb` 76M, `/data/scryfall` 24M, `app.db` 212K, `/data/decks` 304K,
    plus `lost+found`. Nothing unexpected growing. At this rate the 3GB volume
    is not a concern; the pool is the only large object and it is replaced
    rather than appended.
  - **Machine:** `shared-cpu-1x`, 1 vCPU, 1024MB, region `iad`, one machine on
    volume `vol_vwnqxewn1y00oy9v`. `free` is not in the image so in-container
    memory was not readable; no OOM events and no restarts in the event log
    beyond the deploy itself. (Red has the machine version and check config.)
  - **Snapshots vs migrations — the check passes.** Newest snapshot ~4h old at
    time of reading; newest schema migration is `SCHEMA_VERSION = 8`, landed
    2026-08-15 in #109. Snapshot is newer than the migration. Retention is
    **5 days**, which — given ADR 23 applies migrations on boot, unwatched and
    forward-only — is the real recovery window for a bad one.
  - **Concurrency probe, local, machine quiet** (load 3.0/8 cores; the earlier
    contended run is discarded). Serial: `/api/health` 73ms, **`/api/decks`
    224ms**, `/api/decks/local/arahbo-cats` 117ms, `/api/colors` 3ms,
    `/api/glossary` 3ms, `/` 4ms. **At the design point (10 concurrent):
    `/api/decks` wall 1821ms, median 1513ms — 6.8× serial, i.e. essentially
    perfect serialization.** Deck detail is 2.6× (DuckDB releases the GIL);
    everything else is noise. At 30 concurrent the shelf is 5458ms, still
    linear. **Zero errors at every level** — no 500s, no timeouts, no SQLite
    lock failures. The first bottleneck is the shelf's pure-Python YAML and
    aggregation under the GIL, not SQLite, not memory, and not the job pools.
  - **SQLite posture is correct for the design point:** `journal_mode=WAL`,
    `foreign_keys=ON`, `busy_timeout=5000` in `auth/db.py::connect`, deferred
    isolation so `with con:` is a real transaction. ADR 4 stands; no change
    proposed.
  - **Where the design point lives in code** (100 accounts / 10 concurrent):
    `jobs.py` `CPU=1` (GIL-bound, deliberate), `NET=2` (spend guard, not
    throughput), `MAX_JOBS=200`; `auth/ratelimit.py` `PER_ACCOUNT` 10/15min,
    `PER_ADDRESS` 30/15min, `RESET_PER_MAILBOX` 3/hr, `RESET_PER_ADDRESS`
    10/hr, `CLAIM_PER_ADDRESS` 20/15min; `fly.toml` `soft_limit=20` /
    `hard_limit=40`; one uvicorn worker, bound by the in-process job registry
    (the Dockerfile's CMD comment is the argument and it is still correct).
    **Adaptability verdict: 500/50 is a config edit and a re-measure for
    everything except the shelf and the single worker.** Both are documented
    levers with triggers above rather than hidden assumptions, so the design
    point is a setting.
  - **Responsive sweep, screenshots taken at each** — 375 light, 375 dark
    (deck page), 768 light, 1024, 1280 light, 1280 dark. No horizontal overflow
    on any route at any width; the 3px seen at 375 is the emulator's classic
    scrollbar (`innerWidth` 378 vs `clientWidth` 375) and scrolls 0.5px, not a
    real overflow — a real iPhone uses overlay scrollbars. Theme resolution is
    correct: with no stored preference the app follows `prefers-color-scheme`
    and persists the resolved value.
  - **Live instance response times:** see Black's baseline (p50 TTFB `/` 240ms,
    `app.js` 192ms, `/api/health` 211ms with ~45ms server work; gzip on, no
    brotli, strong etag with 304 confirmed). Green watches these for
    *degradation* rather than re-measuring them.
  - **Local suite note:** this worktree symlinks the real pool, so pytest
    reported **1932 passed / 0 skipped**. CI has no pool and will still show
    its pinned 2 skips — the 0 is a property of the worktree, not a change.
