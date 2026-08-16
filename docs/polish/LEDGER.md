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

- **Last run:** never
- **Queued for Aaron:** —
- **Deferred:** —
- **Measurements:** —

## Green — Growth & Resilience

*Browser & mobile compatibility · cloud resource watch · scalability*

- **Last run:** never
- **Queued for Aaron:** —
- **Deferred:** —
- **Measurements:** —
