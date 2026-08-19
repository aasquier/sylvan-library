# The Polish Ledger

The memory of the recurring polish pass (`.claude/skills/polish/`). One
section per color, plus **Colorless** — the artifacts run that audits this
file, the skill and the developer tooling, and goes last in a rainbow. Each
run updates its own section **on the branch that did the work**; Colorless is
the one allowed to correct another section, which is its job. Queued findings wait on Aaron and are not re-litigated;
deferred items name the trigger that revives them; measurements are recorded
even when healthy, because today's healthy number is next quarter's baseline.

Facet-to-color map and the run protocol live in the skill. This file holds
state, never checklists.

---

## White — Law & Protection

*Licensing/free-use (triple-checked) · security & isolation · testing discipline*

- **Last run:** 2026-08-19 (rainbow). Previous: 2026-08-16 (rainbow).
- **Fixed this run (2026-08-19, rainbow):**
  1. **The project distributes other people's code and type, and none of the
     notices travelled with it.** The camera door (#179/#180, three days old)
     made `ocr.py` the **only third-party code this project actually
     redistributes** — every other section of `NOTICE.md` is an argument for
     why something is *not* redistributed (Forge is a separate process,
     ffmpeg never reaches the image, Depth-Anything is never served). These
     bytes go to every browser that opens a viewfinder, and `NOTICE.md` had no
     Tesseract section at all; the Apache-2.0 claim existed only as an
     adjective, in `CLAUDE.md` and in a comment in `tests/test_isolation.py`.
     The sharpest part is self-evidencing: **`worker.min.js` opens with
     `/*! For license information please see worker.min.js.LICENSE.txt */`**,
     a pointer *relative to whatever origin serves the script* — so serving it
     first-party aimed every recipient at a 404, and what that file holds is
     the MIT and BSD-3-Clause notices for buffer, ieee754,
     regenerator-runtime and zlib.js, licences whose one condition is that the
     copyright travels with the copy. Fixed three ways: the notice is a fourth
     row on the shelf (pinned by digest like the rest, answering at
     `/api/ocr/worker.min.js.LICENSE.txt`); `licenses/Apache-2.0.txt` and
     `licenses/OFL-1.1.txt` carry the texts, the first also covering the
     minified `tesseract.js` Vite bundles into the **committed**
     `web_dist/assets/reader.js`, where the minifier drops the legal comments;
     and `NOTICE.md` gains the two sections with the verification written out.
     **Both tests mutation-verified** — the row deleted, watched to fail,
     restored — and the route test takes the name *off the shelf*
     (`name, = [n for n in ocr.ASSETS if n.endswith(".LICENSE.txt")]`) rather
     than restating it, because the first draft supplied its own table entry
     and stayed green against the bug.
  2. **The newest paid surface's job body had no tests at all.**
     `api/scanruns.py` sat at **61%**, the least-covered module in the app,
     and the missing lines were the whole closure: the `ModeExhausted` and
     SDK-failure translations, and — the part that matters — the hand-off to
     `identify_cards` that *is* ADR 34. The 42-tests-that-all-asked-about-a-deck
     shape, again: `tests/test_claude_scan.py` has 15 tests and every one is
     about the mode. Four tests added against `tiny_pool`'s real printings
     (not a faked `identify`, which would agree with any wrong answer), all
     mutation-verified: bypassing the pool, letting `ModeExhausted` escape,
     and raising on an unreadable capture each fail exactly one of them.
  3. **`mutate`'s target map had no entry for either runtime shelf.**
     `ocr.py` — the module deciding whether somebody else's WebAssembly runs
     in a browser — and `symbols.py` were absent, so the digest check, the
     size cap and the sticky refusal set had never been sampled. Added;
     catalogue **1,231 → 1,279 sites across 18 modules**.
  4. **A survivor closed: `sim/compile.py:102`, `0` → `1`.** The line's own
     comment states the rule ("a fetchland sacrifices itself, so it is
     net-zero lands and must not count here") and nothing pinned it. Widened,
     every land in the deck fetches another — `SimCard.is_ramp` turns true for
     all of them and the engine draws an extra land per land played — and
     `test_sim_compile`, `test_sim_tier1`, `test_determinism` and
     `test_sim_cache` were **all still green**, checked by hand rather than
     inferred from the mapped-test caveat. `test_a_fetchland_compiles_to_no_ramp_at_all`
     closes it, mutation-verified against that same mutant. Same shape as the
     `decks/validate.py:158` survivor the previous run closed: a stated rule
     with no pin.
- **Verified this run (2026-08-19, rainbow):**
  - **The trained data is what it says it is, by its bytes.** It is fetched
    from a *mirror* (`tessdata.projectnaptha.com`), not from the project that
    licences it, so the URL proves nothing. Gunzipped it hashes to the same
    git blob as `tesseract-ocr/tessdata_fast@4.0.0`'s own `eng.traineddata`
    (`bbef4675053b5b468cdb477053e28b1c698ba08e`, 4,113,088 bytes), and that
    repository's `LICENSE` is Apache-2.0, read rather than recalled.
  - **No UI credit is owed, and that is checked rather than assumed** — which
    matters because commandment 10 would have made it a genuine conflict.
    Apache-2.0 §4(d), the clause requiring attribution *inside a display*,
    attaches only when the upstream work ships a `NOTICE` file.
    `tesseract.js@7.0.0/NOTICE` and `tesseract.js-core@6.1.2/NOTICE` were both
    requested and both 404. §4(a) and §4(b)-(c) are the whole obligation, and
    the fix above discharges them. **Re-check on any version bump** rather than
    inheriting this — it is a fact about two packages, not about the licence.
  - **The isolation sweep is live**, confirmed by mutation: an unclassified
    `/api/unclassified/probe` failed `test_every_route_is_classified`, then
    removed.
  - **The suite does not touch the real `app.db`** — two consecutive full runs,
    hash, size and mtime byte-identical across both. (A mid-audit scare: the
    file's mtime moved during the first run. The second run is what settled
    it; `ls -la` alone could not.)
  - Queued item 3 from the last run — **widening CI's card-data filename scan
    — has landed.** Red closed it on 2026-08-16; `ci.yml` now matches
    `.duckdb`, `.jsonl.gz` and `.json.gz` anywhere in the tree, with the
    argument for which extensions may go global written out beside it.
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
  1. **ReDoS cluster — now 4 open CodeQL `py/polynomial-redos` warnings, and
     the pre-auth half is gone.** Re-read 2026-08-19: `auth/users.py:153`
     (`EMAIL_RE`, the one reachable *without a session*, on the claim path) no
     longer appears, and one of the five decklist patterns has also cleared.
     What remains is four in `decks/decklist.py` (lines 166, 171, 184, 254),
     all behind auth on the import path. **The urgency dropped with the
     unauthenticated one; the question did not change.** Still not fixed
     because anchoring the patterns is behaviour-sensitive on a load-bearing
     parser. Suggested direction unchanged: a max-length bound on the pasted
     decklist *before* the regex runs, whose value is Aaron's call. Wants a
     per-pattern test.
  2. **Package licence undeclared in `pyproject.toml`.** `[project]` has no
     `license` key, so the wheel metadata reads as UNKNOWN despite the MIT
     `LICENSE`. One-line change, but the correct form is
     setuptools-version-sensitive and only the `image` CI job can fully verify
     it — so it is queued (land it alone on a watchable branch), not bundled.
     *Confirmed still true 2026-08-16 (rainbow), and now from the primary
     source rather than from pyproject: a licence sweep of the installed
     metadata reports `mtg-lab  UNKNOWN` while every one of its 44 dependencies
     reports a real licence.*
  3. ~~**Widen CI's card-data filename scan to catch a `.duckdb` anywhere.**~~
     **Landed** — Red closed it 2026-08-16. Kept as a line rather than deleted
     so the next run does not re-find it.
  4. **The coverage floor is a tripwire, not a floor, and the decision is
     still open.** Carried from #182's tooling run and re-measured today:
     **95.136% against `fail_under = 95`**, which is ~16 statements of
     headroom on 11,512. The comment beside the number says its point is "a
     percent of headroom: the suite runs about 96" — that has not been true
     for two runs. It moved the *right* way this cycle (95.0 → 95.136, and
     this run's tests add more), but the margin is still inside ordinary
     churn, so the next uncovered branch turns CI red for no reason anybody
     will recognise. **Two honest options and drifting is neither**: move
     `fail_under` to 94 and say why, or spend a session on the four modules
     carrying the gap — `api/adminstats.py` 76%, `api/admin.py` 85%,
     `api/argueruns.py` 86%, `animist/fetch.py` 81%. (`api/scanruns.py` was
     the worst at 61% and is fixed above; `cardmotion/depth.py` reads 0% by
     design — it is the `depth` extra's loader and never runs in CI.) Aaron's
     call because a coverage floor is a policy, not a bug.
  5. **CLAUDE.md's Setup block cannot bootstrap this machine, and nothing
     says so.** Found by this cycle's clean-checkout install (below). It opens
     `python -m venv .venv` — there is **no `python` on this PATH at all**;
     `python3` is 3.7.3 and `/usr/bin/python3` is 3.8.2, both under the
     documented 3.11 floor. The only interpreter that satisfies it is the
     uv-managed 3.12 already inside `.venv`, and **`uv` appears nowhere in
     the file**. So the one document a new contributor is handed describes a
     bootstrap that cannot run where it was written, and the real one is
     undocumented. Not fixed here because the fix is a *choice* — document
     `uv`, name `python3.12`, or state a floor and let people solve it —
     and it is Blue's facet (Claude-first docs) as much as this one.
     **Flagged to Blue.**
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
    two hardcoded table names. Re-swept 2026-08-19 over everything added
    since: `claude/ledger.py::summary` interpolates a **column name**, which
    cannot be bound as a parameter, and guards it with an `AXES` tuple and an
    explicit raise; `decks/sqlsource.py` and `decks/wheel.py` build a `where`
    clause from fixed literals and bind every value. `snapshot_prices` is
    still the only one.)
  - **`ocr.installed()` has no caller.** Its docstring says it is "for the
    health endpoint and for a client that would rather not open a viewfinder
    it cannot use", and neither exists — the client asks for its three files
    by name. The shape `caches.py` was built for: a function can be correct,
    tested and never once called. Left rather than deleted because the
    camera-readiness use is plausibly coming, and a docstring note now records
    that the row added this run reads `False` on a working camera. Trigger:
    the next session that touches the camera door either wires it up or
    removes it.
  - **No request-body size limit before FastAPI parses JSON.** `claude/scan.py`
    caps the *decoded* capture at 4MB, but that runs after the whole body has
    been buffered and base64-decoded — and every `payload: dict[str, Any]`
    route shares the property, so it is neither new nor scan-specific. The
    mitigating fact is that all of them are behind auth, so the caller is
    somebody Aaron invited. Trigger: a public POST route, or memory pressure
    on the instance.
- **Measurements (2026-08-19, rainbow):**
  - Full suite: **2409 passed / 0 skipped in 175.0s** on this laptop, run
    twice. Zero skips is *correct here and different from CI*: the two
    `needs_full_pool` tests find the real pool on this machine and run, where
    CI's gate pins them at 2 skipped. **The count is what moved** — 1926 →
    2409 in three days, which is the camera door, the bench and the mutate
    harness arriving. Slow tail unchanged in shape and slightly faster:
    `test_sim_tier1` land sweep 5.60s, `test_wheel` 4.29s, then
    `test_sim_tier1` 2.47/2.27s. Nothing new in the top 25.
  - **Coverage 95.136%** (11,512 statements, 560 missed) against
    `fail_under = 95`. Up from #182's 95.0 and still inside churn — queued
    item 4.
  - **Mutation sample: kill rate 72% — 18 of 25, seed 1, drawn from 1,279
    sites**, judged by each module's mapped tests. Prior baseline 76% (19/25,
    seed 0, 1,231 sites); the two are different draws over a catalogue that
    grew by 48 sites, so read them as two samples rather than a trend.
    All seven survivors read, which is the work the tool does not do:
    - `sim/compile.py:102` (`0` → `1`) — **a real gap, closed above.**
    - `decks/suggest.py:183` (`<=` → `<`) — real and cosmetic: the "mana value
      3 vs 4" *sentence* disappears for an exactly-one-away pair. The score is
      untouched. Not worth a run's diff.
    - `sim/tier1/engine.py:400` (`1` → `2`) — the text report's row loop; drops
      turn 1 from a printed table. Real, cosmetic, untested.
    - `auth/ratelimit.py:48` and `colors.py:904`, both `True` → `False` — and
      both are **`@dataclass(frozen=True)`**. Nothing mutates a `Limit` or an
      `Era`, so no test could see it. **Worth passing to Colorless as a
      catalogue observation**: `constant` mutations landing on decorator
      keyword arguments are structural rather than behavioural and will
      survive forever, which quietly depresses every kill rate. Two of seven
      survivors here.
    - `ocr.py:147` (`30` → `31`) — the `urlopen` timeout. Equivalent by
      design: `tests/test_ocr.py` opens by saying no test sends a request.
    - `cards/identify.py:200` (`6` → `7`) — the longest set-code prefix tried
      against the corner. Equivalent *given the pool*: no real set code is
      seven characters, so the extra iteration can never match. Equivalent
      because of a fact about Scryfall, not about the code — worth knowing if
      that ever changes.
  - **Licences swept 2026-08-19 from installed metadata, not summaries:**
    **75 Python distributions** (was 45) — MIT/BSD/Apache/MPL/PSF/ISC/
    MIT-CMU/MIT-0/CC0/Zlib/0BSD/CNRI-Python; **180 npm packages** (was 163) —
    140 MIT, 15 ISC, 10 Apache-2.0, 4 MPL-2.0, 3 each BSD-3-Clause and
    BSD-2-Clause, 2 MIT-0, 1 each BlueOak-1.0.0/CC0-1.0/"MIT AND ISC".
    **Zero AGPL/GPL/SSPL/UNLICENSED on either side.** Only `UNKNOWN` is the
    package itself (queued item 2, still true). npm's Apache-2.0 count rose
    6 → 10, which is the Tesseract arrival showing up in the numbers.
  - **`animist verify`: 12 recipes, all held** (was 2 — the wheel, séance,
    learn and ambience recipes all landed since). The video outputs are
    genuinely decoded here rather than skipped: `_decode_video` returning
    `None` produces a *failure*, not a pass, so a missing `imageio_ffmpeg`
    cannot read as green.
  - **Committed-binary sweep: 150 tracked media files, every directory
    accounted for.** 78 tarot + 11 ambience + 11 wheel + 6 séance + 3 learn +
    3 claude, each with a recipe or a `PROVENANCE.md`; plus 4 woff2 fonts and
    their `PROVENANCE.md`; the rest are the build's copies under `web_dist`.
    **No hand-placed binary, no Wizards art under `git ls-files`, no
    monetization surface anywhere** (swept `web/src` and `src/mtglab` for
    donation/sponsor/payment/ad wording — the only hits are Magic flavour text
    and the word "checkout" meaning a git checkout).
  - **Fonts, verified three ways** and recorded because it is a *new* surface
    since the last White run: Google Fonts' `OFL.txt` and `METADATA.pb` per
    family; the two foundries' licence bodies compared and found identical;
    and — the one that decides it — the binaries themselves, whose `name`
    tables carry the copyright with the reserved font name (nameID 0) and the
    OFL's URL (nameID 14). nameID 13, the full licence text, is absent from
    all four, which is Google Fonts' universal subsetting behaviour; that is
    why `licenses/OFL-1.1.txt` now exists.
  - **CodeQL: 4 open alerts, all `py/polynomial-redos`, down from 6.** The
    `auth/users.py` one — the only pre-auth reachable pattern — has cleared.
    Nothing dismissed. `dependency-review` green on the last three runs.
  - **The clean-checkout install, done deliberately this cycle** (the item
    that found the `.[dev]` gap in 2026-08-16, by accident, and that nothing
    reproduces on purpose any more). A detached worktree at `origin/main`, a
    fresh venv, `pip install -e ".[dev]"` verbatim: **2407 passed / 2 skipped
    in 185.6s**, and the two skips are exactly CI's pinned pair
    (`test_api.py:1846` and `test_colors.py:153`, both `needs_full_pool`).
    `mypy` clean over 107 source files. 2407 + 2 == the main tree's 2409, so
    **it is the same suite** — the 2026-08-16 gap stays closed and
    `tests/test_packaging.py` is holding it. The one thing it did find is
    queued item 5: the interpreter the documented command names does not
    exist here.
  - **Security spot-checks all still hold**: cookies `HttpOnly; Secure;
    SameSite=Lax`; Argon2id at the OWASP minimum (m=19456 KiB, t=2, p=1);
    `MAX_PASSWORD_BYTES = 1024` bounds the hash input; no query-string token
    anywhere under `web/src`; `include_email=True` still has exactly one
    caller (`api/admin.py`) beside `mtglab users list`; `.env` gitignored and
    `fly.toml` carries no secret. **57 routes classified** in
    `tests/test_isolation.py`, the four newest (`/api/claude/scan`,
    `/api/symbols/{code}.svg`, `/api/ocr/{name}`, the two motion routes) all
    filed SHARED with the argument written beside each.
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

### Mutation testing is tooled — 2026-08-19

The facet has asked for random mutation sampling every run since 2026-08-16,
by hand, with `git checkout --` as the harness. `mtglab mutate` is that
protocol automated, and the automation fixed two things about it.

**The working tree is now unreachable.** Every mutation is applied to a
throwaway copy of the package and pytest is pointed at the copy with
`-o pythonpath=`. The hand version edited the real file and restored it, so an
interrupted run left a mutation in the tree with `git status` as the only
thing between that and a commit. `tests/test_mutate.py` proves the isolation
by content rather than by trusting the flag.

**The sample is seeded**, so a kill rate can be checked rather than only
quoted, and the number below can trend.

**Catalogue:** 1,231 sites across 16 modules — 340 boundary, 319 comparison,
180 arithmetic, 157 guard-drop, 154 boolean, 81 constant.

**Two bugs found in the harness by its own tests**, both of the shape this
pass keeps finding:

- **A mapping is a narrowing, and a typo widened it silently.** `TARGETS`
  named `tests/test_sim.py`, a file that has never existed, so every mutation
  of the simulator ran the *entire* suite instead of two files — correct,
  forty times slower, and invisible in a report that only says "killed".
  `missing_tests()` now pins every entry against the tree.
- **A custom target map narrowed the catalogue and not the tests.** One map
  now decides both.

**A third was a design flaw, not a typo.** The default timeout was 900
seconds, and the first sample ever drawn wedged for ten minutes: **a mutation
is a good way to write an infinite loop** — widen a `<` to a `<=` at a loop
bound and the condition stops being reachable. A timeout is a kill, so a
generous limit buys nothing but a run nobody can sit through. Now 180s.

**First baseline: kill rate 76%** — 19 of 25, seed 0, drawn from 1,231 sites,
judged by each module's mapped tests rather than the whole suite. Record the
seed with the rate; without it the number can only be quoted.

Six survivors, read rather than counted, because that is the work the tool
does not do:

- **`decks/validate.py:158`, `and` → `or`. A real gap, now closed.**
  `if drafting and pending` had neither half pinned: widening it to `or` left
  the whole suite green while opening two wrong reports — a finished draft
  told "0 of 99 cards still need a `why`", and a *curated* deck handed a
  draft's counted warning on top of the 99 blocking errors ADR 13 says it
  should get instead. The existing test covered only the case where both
  halves are true, which is exactly the case the mutation leaves alone.
  `test_a_complete_draft_is_told_nothing_about_rationales` closes it, and was
  mutation-verified against this same mutant.
- **`decks/edit.py:661`, `<` → `<=`.** Real but cosmetic: blank-line ownership
  when the entry being moved is the *last* in its list. No test covers that
  edge. Left as a finding rather than fixed — the surgical cap, and a
  formatting edge is not worth a run's diff.
- **`decks/analyze.py:33` and `decks/companion.py:139`, boundary constants.**
  Both are numbers in a table; the tests exercise the table's behaviour rather
  than its edges. Worth a look next White run, not this one.
- **`sim/cache.py:243` (`ensure_ascii`) and `:356` (`row["b"] or 0`).** Both
  effectively equivalent on any input this app produces — the cache key holds
  numbers and an ASCII keep_rule, and `row["b"]` is falsy only for an empty
  row. **Equivalent mutants are the expected cost of the method**, not a
  failure of it, and recording them here is what stops the next run
  re-investigating the same two.

**Finding for the next White run: the coverage floor has no headroom left.**
Measured while adding these tests — the tree *without* the new modules sits at
**95.0% against a `fail_under` of 95**, so the floor is not a floor any more,
it is a tripwire one uncovered branch from red. The `fail_under` comment says
the point of the number is "a percent of headroom", and that headroom is gone;
the suite was ~96% when it was written. Either the floor moves down
deliberately or the gap gets closed deliberately — drifting into it is the one
option that was never chosen. Not fixed here: this branch is the tooling, and
raising a coverage floor is a decision with its own argument.

**`mutmut` / `cosmic-ray` stay queued** rather than being dropped: the
in-repo harness covers the routine per-run sampling with no new dependency,
and the escalation worth Aaron's yes is an *exhaustive* run over one module,
which is what those tools do that this one does not. Ask again when a module's
kill rate is bad enough to want every mutant rather than a sample.

## Blue — Craft & Knowledge

*Python craft · TypeScript/React craft · Claude-first docs & memory · the
spirit of Magic*

- **New facet, never yet run:** *the spirit of Magic* (added 2026-08-19 at
  Aaron's ask — commandment 3 as a sweep: prefer the game's terminology and
  iconography over plain conversational English, within commandment 2's
  bounds). No baseline exists; the next Blue run owes it a first full sweep
  of the rendered surfaces, and Blue is staler than its date suggests until
  that happens.
- **Last run:** 2026-08-18 (punch-list item 5, Blue + Red in one session —
  Aaron's ask was API hygiene and dev-cycle relics, which is Blue, and alerts
  and instability signals, which is Red). Previous: 2026-08-16 (rainbow).
- **Fixed this run (2026-08-18):**
  1. **ADR 30 superseded half of ADR 1 and eighteen code comments never heard.**
     Decks came out of git on 2026-08-16; `docs/adr/README.md` records the
     supersession correctly and ADR 30's own text calls it "half a sentence of
     1, and it is the half everything since had leaned on". The leaning was
     never swept. Eighteen sites across eleven files still taught the old
     world, in four flavours: `deck.yaml` "is tracked in git"; "deck history is
     git history", offering `git log -p decks/gyome-food/deck.yaml` as the swap
     record; `swaps.md` diffing "the last git commit" when it diffs
     `artifacts/deck.last-built.yaml`; and **`git checkout` named as a deck's
     undo**, four times. The last two are not stale prose but wrong
     instructions — a session that believes them looks for a diff that cannot
     exist and offers a recovery that cannot work — and one of the eighteen was
     **printed to the terminal** by `mtglab decks log`. Third time this class
     has been caught (the ledger records "deck history is git history" shipping
     in two components, found 2026-08-16 by driving the live surface), so this
     run left a tripwire instead of a fourth correction:
     `tests/test_deck_location_drift.py`, scoped to `src/mtglab` and `web/src`
     because the ADRs are immutable and ADR 1 is *titled* with the superseded
     claim.
  2. **`mtglab animist verify` was pinned on 2 of 12 recipes.**
     `tests/test_animist_recipes_repo.py` named `ambience` and `tarot` by hand;
     the ten added since — the wheel's menagerie, the séance parchment, the
     Learn bookworm, the three ambience motions — were verified by nothing. The
     file *did* check that the glob could see them, and reachability had been
     mistaken for verification. Now parametrised over the same glob the CLI
     uses, so a recipe committed tomorrow is covered the day it lands, plus a
     test that the glob is exhaustive against `git ls-files`. All twelve hold.
     Costs 3s.
  3. **`.gitignore` was ignoring the deck *source package*, and nobody could
     have seen it.** ADR 30's rule is written `decks/`, and a bare directory
     pattern in gitignore matches **at every depth** — so it covered
     `src/mtglab/decks/` too, thirteen tracked modules. Nothing was broken:
     an ignore rule cannot untrack a tracked file, so the package worked, the
     suite passed, CI passed, and it had presumably been this way since ADR 30
     landed. What was broken was *tomorrow* — `git add src/mtglab/decks/x.py`
     refuses outright and `git add -A` skips it in silence, so the next module
     added there arrives in CI as an ImportError for a file the diff does not
     contain. ADR 28's own note dates the trigger: "the tenth edit operation is
     the one somebody adds in a year." **Found by tripping over it** — staging
     this run's own edits to `decks/edit.py` and friends is what made git
     object. Fixed by anchoring to `/decks/`; the app's data directory is still
     ignored (a test asserts that too, because the over-correction is worse
     than the bug) and `build/` still covers its copy.
     `tests/test_gitignore_shadowing.py` is the guard, and it is deliberately
     the *general* one: it asks `git check-ignore` whether a new file would be
     accepted in **every directory git already tracks a file in**, derived from
     `git ls-files` rather than a hand-kept tree list — because a hand-kept
     list needs exclusions for `src/mtg_lab.egg-info/` and `.claude/worktrees/`
     (both correctly ignored, neither source), and an exclusion list is where
     the next real offender gets waved through. Mutation-verified three ways:
     the original bug, the over-correction, and a *hypothetical future*
     over-broad rule (`assets/`), which it also catches.
- **The trap this run paid for, and the reason both fixes carry mutation
  evidence:** the tripwire in finding 1 **passed as decoration on its first
  draft.** It matched line by line, every offence it was written for happened
  to fit on one line, and it went green. Re-breaking `model.py` did not fire
  it, because the restored claim wrapped after "directory and". This codebase
  wraps comments at 79 columns, so *any sentence long enough to be worth
  banning spans two lines*. It now flattens each file to one line before
  matching, and is verified by re-injecting all nine original offences
  verbatim — 9 caught, 0 missed. A second draft failed the opposite way: a bare
  `\bin git\b` cannot tell the claim from its denial, and flagged every
  sentence written to *correct* the drift. It bans the affirmative verbs
  instead (`tracked|held|file-backed|committed|diffed in git`, `lives in git`).
  **The tripwire found four sites the hand sweep missed**, all in
  `source.py`/`sqlsource.py`, which is the argument for writing it at all.
- **Fixed and landed (previous run):**
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
  - **pytest (2026-08-18):** **2350 passed, 0 skipped in 208s** on this Mac
    (was 1926 / 2 skipped / 159s on 2026-08-16 — the suite grew 22% in two
    days). **The zero is not a regression in the skip gate**: the two
    `needs_full_pool` tests *pass* here because this machine has the pool, and
    skip in CI, which is where `ci.yml`'s `expected=2` counts them. Confirmed
    by running `-m needs_full_pool` alone: 2 passed. Recorded so the next run
    does not chase it.
  - **frontend (2026-08-18):** `npm --prefix web run check` green — **499
    Vitest tests across 28 files in ~37s** (was 432 / 23 / ~27s). Bundle
    rebuilt and byte-identical, comments being stripped.
  - **pytest (2026-08-16):** 1926 passed, 2 skipped in **159s**, uncontended
    and post-#129. Incidental cross-check of White's conftest fix: a full run
    left `data/app.db`'s mtime untouched, where the pre-#129 run in this same
    worktree had written it.
  - **frontend (2026-08-16):** green — 432 Vitest tests across 23 files in ~27s.
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

### Targeted performance pass — 2026-08-19

Not a rainbow run: Aaron reported the app feeling sluggish and this is what it
was. Every number below is a median on this Mac against the full pool.

- **DuckDB asked for pandas twice per bound parameter.** The Python client
  probes `import pandas` to decide whether a value is a DataFrame. Pandas is
  not a dependency, so every probe was an `ImportError` — and an `ImportError`
  walks all of `sys.path` and stats every entry. `get_cards` binds one
  parameter per card name, so the library shelf's single query bound 441 of
  them and paid **1,768 failed imports: 162ms of `/api/decks`'s 200ms, spent
  entirely in the import machinery.** A `None` sentinel in `sys.modules`
  (planted only where `find_spec` confirms pandas is genuinely absent) makes
  the probe fail without touching the path. 900-parameter query **162ms →
  20ms**. `cards/db.py::_duckdb`.
  **This was inside the "108ms → 62ms" `get_cards` note above and was never
  attributed** — the note measured the query and the storm was in the binding.
- **The pool was reloaded on every request.** DuckDB frees a database instance
  when its last connection closes, so open-answer-close paid the full load
  each time: **17.5ms → 0.7ms** with one handle held. `api/service.py::_pin`.
  It is a **30-second lease, not a claim** — a held read-only handle is a held
  lock, and a permanent keeper would make a running `mtglab ui` refuse every
  `mtglab data refresh`, which is the exact inversion of why the app opens
  read-only in the first place.
- **Deck files were re-parsed per request** — 18.0ms of YAML for seven files
  that had not changed. Cached on `(mtime_ns, size)`, **copied** on the way
  out because `Deck` is a mutable dataclass and `source.py` writes `shared` to
  what it returns. `decks/model.py::_PARSED`.
- **`ILIKE '%q%'` on `oracle_text`** ran a case-folding pattern matcher over
  35,390 rows. `contains(lower(col), ?)` is the same question walked once:
  **67.1ms → 18.6ms**, verified byte-identical against ILIKE over eight
  queries including accents and `//`. It also stops `%` and `_` in a search
  box behaving as wildcards nobody typed.
- **`get_cards` rewritten** to one list parameter instead of N placeholders
  (the old form built a different SQL string per name count, so no two shelves
  could share a plan) with the face-name `split_part` gated on `contains(name,
  ' // ')`. **62.4ms → 51.7ms**, same 443 rows.
- **`oracle_columns` cached on the pool file**, not the connection. The first
  attempt keyed it on the handle and was **correct and never once hit** —
  measured, six endpoints, one call per connection each, so the entry was
  written and thrown away with the connection that owned it. ~2.4ms a request.
- **Client: concurrent identical GETs are coalesced** (`lib/api.ts`). The shell
  and the Library both asked `/api/health` in the same tick on every visit.
  A queue of one, deleted the moment it settles — not a cache, so nothing is
  ever read after the network answered.

- **Card lookups memoised on the pool stamp.** The last item, and the one that
  needed the other six first: `get_cards` was still ~50ms because
  `lower(name)` cannot use `idx_oracle_name`, so every lookup was a full scan
  of 35,390 rows and there is no cheaper query to write. The questions repeat
  exactly, though -- the shelf asks the same 441 names until a deck is edited,
  the lore shelf the same ~120 forever -- so the answer is to stop asking.
  Keyed on `(pool path, mtime_ns, size, names)`, LRU-bounded at 16 entries
  (arithmetic: one shelf entry is 415 records at ~639kB, so 16 is ~10MB worst
  case and 2--3MB for the real working set, against a 1GB machine).
  **55.7ms → 0.003ms** on the shelf's own name set.
  Three things make it safe. `CardRecord` is now **frozen**, so two requests
  may hold the same record -- the package was swept for an assignment to any
  of its fields and there was not one, so this cost nothing. The **dict** is
  copied out, shallow, so a caller that pops from its result cannot reach the
  entry. And **write handles are never memoised**: a stamp claims the contents
  follow from mtime and size, which is only true of a handle that cannot
  write, since an ingest inserts rows while the file on disk still looks
  untouched.

| endpoint | before | after five fixes | after the memo |
|---|---|---|---|
| `/api/health` | 37ms | 6ms | **6ms** |
| `/api/decks` | 201ms | 75ms | **16ms** |
| deck detail | 80ms | 43ms | **6ms** |
| deck validate | 79ms | 43ms | **5ms** |
| `/api/lore` | 52ms | 43ms | **3ms** |
| `/api/cards/search?q=goblin` | 111ms | 43ms | **43ms** |

Search is unchanged by the memo and that is correct: it is a text scan rather
than a name lookup, so it never goes through `get_cards` at all.

Verified end to end rather than by the cache's own tests: a card added to a
deck through `service.add_card` moves the shelf's count 99 → 100 and appears
in the deck's 99, through both the deck-parse cache and this one.

**Correction to the bundle note above.** "`charts.js` … already its own chunk,
loaded only where a chart renders" was true of the chunk and false of the
load: it was a *static* import of three lazy routes, so the deck page,
simulator and admin each downloaded 113kB gzipped of recharts whether or not a
chart ever appeared — the deck page's are behind the Stats tab. Now genuinely
deferred (`lib/deferred.tsx`, `components/lazycharts.tsx`), with a placeholder
at each chart's declared height so the page cannot collapse and jump.
`DataTable` moved to `components/datatable.tsx` because it draws no chart and
was dragging recharts in behind it — the simulator did that three times over.
Verified in a browser: the deck page loads nine chunks and **no `charts.js`**,
which arrives on the first click of Stats.

**Correction to Green's concurrency probe.** "The first bottleneck is the
shelf's pure-Python YAML and aggregation under the GIL" had the right shape
and the wrong culprit: the dominant pure-Python cost was the pandas import
storm, not YAML. YAML was real but second, and both are now addressed. The
serialization finding stands and should be re-measured, since the shelf's
serial cost moved 201ms → 75ms.

**Rejected on measurement:** an exact-name fast path (`name IN (…)`, which is
index-eligible) to avoid the scan. It resolved 426 of 441 names and the
remaining 15 still needed the full scan, so it paid for both — a net loss. The
memo above is what replaced it.

**Still open.** The shelf's remaining ~16ms is YAML and aggregation, and the
memo does nothing for a *cold* cache — the first request after a deploy or a
`data refresh` pays the old price. That is the right trade (a warm instance is
the common case) but it means Green's concurrency probe should be re-run
against both states rather than one.


### The measuring shelf — 2026-08-19

Not a run: the tooling the run above proved was missing. `mtglab bench` and
`mtglab mutate` landed with the skill refinement, and Black's facet now says
to use them rather than to hand-time. Baselines below are this Mac against the
full pool, and they replace nothing above — they are the first numbers taken
with an instrument instead of a stopwatch.

**Warm** (`mtglab bench run --runs 15`). Warm is the common case on a live
instance and these are the numbers to compare next quarter against.

| target | warm median | p95 | database |
|---|---:|---:|---:|
| `GET /api/health` | 7.1ms | 8.8ms | — |
| `GET /api/decks` | 18.9ms | 23.0ms | — |
| `GET /api/lore` | 6.3ms | 29.1ms | — |
| `GET /api/colors` | 6.3ms | 7.2ms | — |
| `GET /api/glossary` | 5.3ms | 6.4ms | — |
| `GET /api/cards/search?q=goblin` | 46.1ms | 56.1ms | **37.3ms, 1 statement** |
| deck detail | 7.4ms | 8.3ms | — |
| deck validate | 6.2ms | 6.9ms | — |
| `db.get_cards` (100 names) | 0.2ms | 0.4ms | — |
| `Deck.load` | 0.0ms | 0.1ms | — |

**Cold** (`--cold`, every registered cache emptied between samples). This is
the row the old ledger format had no slot for, and the reason it needed one is
visible at a glance: the shelf is **7.3× slower cold**, and the memo is the
whole difference.

| target | cold median | database |
|---|---:|---:|
| `GET /api/decks` | **138.7ms** | 2.0ms, 3 statements |
| `GET /api/lore` | 83.5ms | 0.0ms |
| deck detail | 80.3ms | 0.9ms |
| deck validate | 74.8ms | 55.0ms, 2 statements |
| `GET /api/cards/search?q=goblin` | 69.4ms | 37.6ms |
| `db.get_cards` (100 names) | 83.2ms | 0.0ms |
| `db.connect_readonly` | **17.8ms** | — |
| `GET /api/colors` | 6.0ms | — |

`db.connect_readonly` at 17.8ms cold against 0.2ms warm is #181's keeper
measured independently — the entry above claimed 17.5ms → 0.7ms, and an
instrument that knew nothing about that claim reproduced it.

**Search is the one endpoint the memo cannot help, and now the tool says why
rather than a run inferring it:** 37.3ms of a 46.1ms wall is inside a single
DuckDB statement, measured at the query probe. It is a text scan, so it never
goes through `get_cards`. The lever is the query or an index; there is no
Python here to rewrite. That sentence used to require somebody to reason
carefully — it is now the second line of `bench profile`.

**Cache hit rates** (`mtglab bench caches --runs 5`), the instrument gap #5
asked for:

| cache | hits | misses | rate |
|---|---:|---:|---:|
| `deck.parsed` | 60 | 0 | 100% |
| `pool.columns` | 15 | 0 | 100% |
| `pool.keeper` | 36 | 0 | 100% |
| `pool.cards` | 27 | 3 | 90% |
| `auth.hasher`, `auth.dummy-hash`, `sets.upcoming` | 0 | 0 | *never asked* |

Three caches read **never asked** rather than 0%, which is deliberate: this
suite does not log in or fetch Scryfall's set list, and a report rendering
"nobody consulted it" identically to "it missed every time" would hide the
more interesting finding. Every registered cache that the suite *does*
exercise hits, so nothing here is currently dead weight.

**One bug the tools found in themselves, worth keeping.** The first cold run
printed a table saying `/api/health` cost 38.3ms cold and a profile beneath it
saying 7.2ms — the profiler was handed the bare callable while the sampler
wrapped it. A warm breakdown under a cold heading is read as the explanation
of the number above it. Fixed, with a test that fails if any profiled call
runs against a cache nobody emptied.

## Red — Speed & Alarum

*CI/CD · alerting & self-healing*

- **Last run:** 2026-08-18 (punch-list item 5, with Blue). Previous:
  2026-08-16 (rainbow), which was the first Red run and the baseline the
  numbers below are now a trend against.
- **Fixed this run (2026-08-18): the alarm tile could not tell "all clear"
  from "I could not ask".** "Edge failed" renders `fmtCount(edge_5xx)`, and
  `—` was the answer both when the edge served zero 5xx *and* when the query
  was broken. That is not hypothetical: #172 had found two Fly queries wrong
  for a fortnight, and an em-dash is exactly how they presented. Prometheus
  has no zero — `sum(increase(...{status=~"5.."}[24h]))` over a clean day
  matches no samples and returns the same empty vector a misspelt series name
  returns — so the one tile that is the instability alarm was the one tile
  incapable of reporting good news. Fixed with a **witness**: all three edge
  counters read `fly_edge_http_responses_count` and differ only in the
  `status` filter, so a populated `edge_2xx` proves the series exists and is
  being scraped, and an empty 4xx/5xx beside it is a real zero. With the
  witness itself empty nothing is claimed and every counter stays `None`.
  Deliberately **not** `or vector(0)` in the query, which is the usual fix and
  would make every result a number — throwing away the failure mode this
  module was built after. `_scalar`'s None-not-zero rule is untouched; the
  settling happens above it, where the witness is in scope. Mutation-verified
  both ways: dropping the call fails the quiet-day test, dropping the witness
  check fails the two tests that pin "nothing is claimed".
  This is trap #2 from the punch-list batch — *a probe that cannot fail
  differently is not a probe* — restated one turn on: **a readout that cannot
  succeed differently from failing is not a readout.**
- **Fixed and landed (previous run):** `concurrency` groups on `codeql.yml` and
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
- **Measurements (2026-08-18):**
  - **CI per-job medians, n=10** (the ten most recent `ci.yml` runs, #168–#173
    and their pushes), against the 2026-08-16 baseline:
    `test (3.11)` **186s** (155–215, was 158) · `test (3.12)` **300s**
    (274–338, was 284) · `frontend` **44s** (37–49, was 36) ·
    `no-secrets-or-card-data` **6s** (5–7, was 5) · `image` **304s** (62–363,
    was 292). Everything grew and nothing grew alarmingly; the suite itself
    gained 424 tests in the same window (see Blue), which accounts for the
    test legs. **The critical path is still a tie** — `image` 304s vs
    `test (3.12)` 300s — so the deferred "speed up `image`" item's whole
    argument holds unchanged, and re-checking this pair stays the precondition
    for touching either.
  - **Live probe (2026-08-18 23:36Z).** `GET /api/health` **200** in 364ms
    (TTFB 363ms), `GET /` **200** in 218ms, `GET /api/decks` **401**. Health
    body: pool true, 35,390 oracle cards, 107,338 printings, **7 decks**,
    `pool_stale` false. Note `pool_stale` is a *schema* check (does the pool
    predate the printed-stat columns), **not an age check** — the bulk file is
    `oracle_cards-2026-08-13.jsonl.gz`, five days old, and nothing reports
    that. Not filed as a finding; recorded so the next run does not read
    `pool_stale: false` as "the pool is fresh".
  - **Instance:** machine `84e19ef25041e8`, **version 108** (was 65 on
    2026-08-16 — 43 machine versions in two days, all deploys), `iad`,
    `shared-cpu-1x`/1 GB, 1/1 checks passing. Deployed at 23:32:42Z during
    this run; logs show a clean boot — health check failed once at 23:32:42
    and passed at 23:32:50, which is the 8-second startup window and not a
    fault. **No OOM, no unexplained restart, no 5xx in the log.**
  - **Volume:** 104 MB used of 2.9 GB (**4%**, 2.7 GB free) —
    `mtg.duckdb` 76M, `scryfall` 24M, `cache` 4.4M, `decks` 304K, `app.db`
    244K. **6 snapshots**, one per day, newest 9 hours old, 5-day retention,
    370 MiB stored. Headroom is not a concern this quarter.
  - **TLS expiry 2026-11-11** unchanged (Let's Encrypt, issued 2026-08-13,
    ~85 days remaining, apex-only SAN).
  - **Queued-item triggers checked:** the merge queue (#5) has **not** fired —
    zero open PRs, and serial work keeps it that way. Everything else in the
    queue below is still waiting on Aaron and unchanged by this run.
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


## Colorless — The Artifacts

*The pass auditing itself: last cycle's findings · are the checklists still
finding things · the developer tooling · cross-color leftovers*

- **Last run:** 2026-08-19 (the run that created this section). Previous: none
  — colorless was the name of the merged-survey mode until this session, and
  that mode is now `converge`.
- **What changed in the skill this run** (Aaron's ask, in a clean session
  after the targeted performance pass exposed seven structural gaps):
  1. **Black's performance facet was rewritten around profiling.** It used to
     say "record the response time", which a run did — 224ms for `/api/decks`
     — while the largest performance bug in the codebase sat inside that
     number. It now says a large number is a *question*, names cold and warm
     as two measurements, and hands the work to `mtglab bench`.
  2. **Green stops naming causes.** A load probe finds *which* endpoint; only
     a profile finds *why*. Green's 2026-08-16 entry guessed "pure-Python YAML
     under the GIL" — right shape, wrong culprit — and the guess sat here as a
     finding for three days. The facet now hands the endpoint to Black's
     profiler.
  3. **White's mutation bullet became a command.** See White's own section.
  4. **Blue's spirit-of-Magic facet grew a second half.** It only ever asked
     what plain English had crept *in*; it now also asks what lore and
     iconography is **absent** — the enrichment half, with five places the
     cheap wins live and three hard bounds (White's licence law, "enrichment
     is not decoration", commandment 15 first).
  5. **Colorless became its own run and moved last**; the merged five-color
     survey is now `converge`.
- **The tooling paid for itself on its first run.** `mtglab mutate` found a
  real hole in the gate's draft warning (White's section has the detail), and
  `mtglab bench`'s first cold run found a bug in `mtglab bench` — a warm
  profile printed under a cold heading. Both are the shape this pass exists
  for: *the instrument disagreeing with the story*.
- **Queued for Aaron:** nothing new. `mutmut`/`cosmic-ray` remain queued in
  White's section as the exhaustive-run escalation.
- **Deferred:**
  - **Blue is four facets and the longest reference; Red is two and the
     shortest.** Not obviously wrong — Blue's facets are cheap to sweep and
     Red's are expensive to probe — but worth re-checking once the enrichment
     half has run twice. *Trigger:* a Blue run that cannot finish its four
     facets in one session.
  - **The bench suite is in-process and cannot see the proxy, TLS, or the
     machine.** Live-instance numbers are still hand-taken. *Trigger:* a
     deployed regression the local bench could not reproduce.
- **Standing questions for the next colorless run**, so it starts with
  evidence rather than a blank page:
  - Did the enrichment half actually produce shortlists, or did it collapse
    back into the sweep? (The failure mode is predictable: the sweep has
    concrete greps and the enrichment needs taste.)
  - Is `mtglab bench run`'s target list still resolving everything? A row
    reading `skipped` is the finding, not the footnote.
  - Has the mutation kill rate moved, and is the cause named?
  - Are any survivors from a previous run still unread?
- **Staleness, honestly stated** for the next bare `/polish`: all five colors
  were last swept 2026-08-16 (rainbow); Black additionally had a targeted
  performance pass 2026-08-19 and now carries the freshest numbers in the
  file. **Blue is the stalest in substance** — its docs-and-memory audit
  predates six merged PRs and the whole camera-import feature — and it also
  owns the new enrichment half, which has never run. Blue next.
