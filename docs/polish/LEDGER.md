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

     **Precision added 2026-08-19 (stragglers), because the run reported this
     URL as "answers 200 on sylvan-libraries.com" and unqualified that is
     false.** It answers **401 to anyone without a session**, which Green
     re-confirmed by curl the same day — `/api/ocr/*` is not in
     `PUBLIC_PATHS`. Both observations are true and the difference is a
     cookie. The arrangement is still correct and should **not** be "fixed"
     by making the notice public: Apache-2.0 asks that the notice travel with
     the *distribution*, the distribution here is the worker served to a
     signed-in browser, and the notice reaches exactly that audience by
     exactly that door. What the ledger should carry is the qualified claim —
     *200 to the audience that receives the code* — not the bare status.
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

     **Answered 2026-08-19 (stragglers), and the answer is that the suggested
     direction was taken three days ago and CodeQL cannot see it.** The bound
     exists: `decklist.MAX_LINE = 512`, applied once in `parse`'s loop before
     the first `search`, with `test_no_pattern_is_handed_more_than_the_bound`
     covering all five patterns and three boundary tests beside it — landed in
     [#132](https://github.com/aasquier/sylvan-library/pull/132) on 2026-08-16,
     the same PR that took `_HEADER` apart. Its value is argued from Magic
     rather than rounded to: the longest card name ever printed is the
     141-character Unhinged elemental whose name is a joke about long card
     names, and a line carries a quantity, a set code, a collector number and
     markers besides. **That fix is what cleared the fifth decklist alert and
     the pre-auth one**; these four survived it.
     **Proved rather than assumed, from CodeQL's own SARIF** (the analysis of
     `main` at `cf0a640`, pulled through the API rather than read off the
     alert list). Every one of the four taint paths runs
     `decklist.py:217 line` → `decklist.py:248 line` — **straight past the
     guard at line 226**. `py/polynomial-redos` for Python has no string-length
     barrier, so a correct bound is invisible to it and no further bounding
     will ever clear these. The fourth path adds the other half of the same
     story: it leaves `_strip_annotations` through the return value at line
     188 and re-enters as `body`, a node no guard on `line` could ever cover.
     **Two options left, and both are Aaron's.** (a) Dismiss the four as false
     positives — the input is bounded at 512 characters before any pattern
     runs, the argument is one sentence, and dismissing a security alert on a
     public repository is a maintainer's call rather than a run's. (b) Rewrite
     the patterns so they are linear rather than merely bounded — the leading
     `\s*` in `_MARKER`/`_BRACKET`/`_PRINTING` and the `\s*[xX]?\s+` in `_QTY`
     are what the query names. **(b) is not a safe fix and cannot be made one
     here**, for a reason worth keeping: a pull-request CodeQL analysis reports
     only alerts *new* against the base, so a PR that fixed these would report
     `results: 0` exactly as a PR that did nothing does. The proof would arrive
     after merge and after deploy, which is one step worse than the case the
     skill already says to queue. The parser is also six decks' import path and
     `decklist.py`'s own comment argues against per-pattern surgery.
     Recommendation: (a), and leave the code alone.
  2. ~~**Package licence undeclared in `pyproject.toml`.**~~ **Landed in
     [#135](https://github.com/aasquier/sylvan-library/pull/135), 2026-08-16**
     — `license = "MIT"`, `license-files = ["LICENSE"]`, `setuptools>=77`.
     Verified 2026-08-19 by *building* the metadata rather than reading it:
     `License-Expression: MIT`, `License-File: LICENSE`.
     **The lesson is worth more than the item.** The 2026-08-16 entry said
     "confirmed still true… now from the primary source rather than from
     pyproject", and it was wrong in exactly the way it was congratulating
     itself for avoiding: for an **editable** install, `importlib.metadata`
     reports what was recorded when `pip install -e` last ran, not what
     `pyproject.toml` says now. The same sweep re-run today still says
     `mtg-lab UNKNOWN` off this laptop's `.dist-info`, three days after the
     fix merged. *Installed metadata is a cache, and the primary source for a
     packaging question is a build.*
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

     **CLOSED 2026-08-19 (stragglers, PR #194). Aaron's ruling: close the gap,
     not lower the floor.** **95.185% → 95.749%** (11,573 statements, 492
     missed against 557), so the headroom went from ~21 statements to **~87**.
     The four modules: `adminstats` 78 → 97, `admin` 85 → 99, `argueruns`
     86 → 98, `animist/fetch` 81 → 97. Suite 2,439 → 2,470 tests, and the
     wall clock went *down* (377s → 352s), which is noise rather than a claim.
     **Coverage was the measurement and not the goal, and the tests are worth
     more than the percentage.** What they assert is behaviour nothing had
     ever asserted: the admin routes' whole refusal surface (503 when mail is
     unconfigured, 502 on a bounced invite with the account still there, 422
     on a tier this build does not ship — reachable, it turns out, only *from*
     a granted tier, since `tiers.get` answers the default for an unknown key
     on both sides of the comparison); the **Linux half** of the memory panel,
     which is the half the container actually runs and the dev Mac never
     reaches; the argue sweep's credential-vanishing path, which is its
     docstring's headline behaviour; and the animist downloader's `.part`
     discipline, checked *during* the body read rather than after it.
     The best of them is not a coverage test at all. `adminstats._user_state`
     restates `admin._state` on purpose, and its docstring argues that a
     disagreement would show as the accounts table and the dashboard tile
     contradicting each other on one page —
     `test_the_accounts_table_and_the_dashboard_agree_about_every_state`
     drives **both** spellings over all four states on one connection, which
     turns that comment into a check. Mutation-verified, like the `.part` pair.
     **Two lines were left uncovered deliberately.** `api/admin.py:337-338`
     (the delete route's `LastAdmin`) is **unreachable through HTTP**: the
     caller must be a usable admin to pass the middleware, and the handler
     refuses self-deletion first, so no request can delete the last one. The
     rule itself is tested at `users.delete`. Defensive depth, correctly kept,
     and worth naming so the next run does not spend an hour on it.
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
  - ~~**Parameterize `cards/db.py::snapshot_prices`.**~~ **Landed in
    [#135](https://github.com/aasquier/sylvan-library/pull/135), 2026-08-16 —
    corrected by Colorless 2026-08-19.** The code reads
    `CAST(? AS DATE)` with a bound `params` list, and the docstring above it
    says "`on_date` is **bound, not interpolated**"; #135's own commit message
    opens with "The date was interpolated" and explains the fix. **The entry
    below was re-affirmed verbatim three days later by a run that re-swept
    every *other* f-string site in the file and never re-read this one** —
    including the sentence "`snapshot_prices` is still the only one", which was
    true of the sweep and false of the item. Same PR, same day, four items:
    three were marked landed here and this was the fourth. This is Blue's
    2026-08-19 lesson in its deferred form — *not re-litigated* must mean the
    argument is not reopened, never that the tree is not re-checked — and it is
    now the **third** time this shape has been caught, so the standing habit is
    to re-verify every carried item against the tree before copying it forward.
    Kept as a line rather than deleted so the next run does not re-find it.
    (Audited alongside it and *fine*: the other three f-string `execute` sites
    interpolate module constants only — `SCHEMA_VERSION`, `_ADDED_COLUMNS`, and
    two hardcoded table names. Re-swept 2026-08-19 over everything added
    since: `claude/ledger.py::summary` interpolates a **column name**, which
    cannot be bound as a parameter, and guards it with an `AXES` tuple and an
    explicit raise; `decks/sqlsource.py` and `decks/wheel.py` build a `where`
    clause from fixed literals and bind every value. That sweep stands; its
    conclusion — "`snapshot_prices` is still the only one" — does not, because
    `snapshot_prices` had stopped being one of them.)
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
      **Colorless acted on this 2026-08-19 and it was bigger than two:** 19
      sites across the 18 modules, every one `frozen=True`, and **22% of the
      whole `constant` class**. They are out of the catalogue now
      (`operators._decorator_flags`), so **1,279 → 1,260 and `constant` 86 →
      67**. Read this run's 72% against the old denominator; the next White
      draw is against the new one, and the shift is a removed floor of
      guaranteed survivors rather than a suite that improved.
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
    **Zero AGPL/GPL/SSPL/UNLICENSED on either side.** npm's Apache-2.0 count
    rose 6 → 10, which is the Tesseract arrival showing up in the numbers.
    The one `UNKNOWN` — the package itself — is **stale metadata, not a
    finding**; see queued item 2. A build says `License-Expression: MIT`.
  - **What the fix does *not* reach, recorded so it is a decision rather than
    an oversight:** `licenses/` is repo-level, and the `Dockerfile` copies only
    `pyproject.toml` and `src/`, so neither the image nor the wheel carries the
    Apache text beside the `reader.js` it ships. Neither is published — the
    image is built in CI and deployed, the wheel is not released — and the
    people who actually receive `reader.js` receive it from the repository or
    from the site, both of which now carry it. Widening `license-files` to
    glob `licenses/*` would close it, and that is a **packaging** change only
    the `image` job can fully verify, which is the case the skill says to
    queue rather than bundle. Trigger: publishing the image or the wheel
    anywhere.
  - **Live instance, pre-auth posture re-probed 2026-08-19** (before this
    branch deployed): `/api/health` 200 and **everything else 401** —
    `/api/decks`, `/api/symbols/W.svg`, `/api/ocr/worker.min.js`,
    `/api/admin/users`, and `/api/nope`, which is the proof the middleware
    refuses *before routing* rather than the router 404ing. Headers all
    present: HSTS `max-age=31536000`, `nosniff`, `X-Frame-Options: DENY`,
    `Referrer-Policy: same-origin`, and #184's
    `Permissions-Policy: camera=(self), microphone=(), geolocation=()`.
    Note the symmetry this run cares about: `/api/ocr/*` is behind auth, so
    the licence notice is **exactly as reachable as the code it explains** —
    only signed-in accounts receive either.
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
  **The next White run did not look, and Colorless re-ran them by name on
  2026-08-19: all four are still alive** — `analyze.py:33` `8`→`9` and
  `12`→`13`, `companion.py:139` `<`→`<=` and `3`→`4`. Not a failure of the
  draw but of its shape: a fresh seeded sample of 25 from 1,260 sites will
  never revisit a named site, so "worth a look next run" means *forever*
  unless somebody asks directly. `mtglab mutate run --only <path:line>` is
  that verb now, and it costs a second a site. **Still open, and still
  White's.**
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

- **Last run:** 2026-08-19 (rainbow) — the *spirit of Magic* facet's first
  outing, so its section below is a baseline rather than a delta. Previous:
  2026-08-18 (punch-list item 5, Blue + Red in one session).
- **Fixed this run (2026-08-19, rainbow):**
  1. **The one paragraph in CLAUDE.md that states the re-check-completeness
     rule had drifted again, in the other direction.** It says *"a sentence in
     this file asserting completeness is a claim to re-check against the code,
     not a fact to inherit"* — a rule written after 2026-08-16 found `dev
     (which includes all of it)` false. What nobody re-checked was the
     enumeration in front of it: **it named four extras where `pyproject.toml`
     declares five**, and the missing one is `depth`, the single deliberate
     exception to the very claim the sentence makes. The code has known this
     all along — `tests/test_packaging.py` pins `depth`-out-of-`dev` in *both*
     directions with ADR 32's argument written out — so the file that a fresh
     session actually reads was the only place the exception did not exist.
     Corrected, and `test_the_setup_section_names_every_extra` now derives the
     set from `pyproject.toml` (parsed with `tomllib`, not matched: the regex
     form of `declared_extras` read every `name = [` to the end of the file and
     also collected `markers`, `select` and eleven other tool keys — harmless
     while the only question was "are these two present", wrong the moment
     anything asked what the whole set is). Mutation-verified both ways: an
     undocumented extra fails it, and un-naming `depth` throughout the section
     fails it. **Deliberately one-directional** — an extra must be named, the
     section may say anything else it likes, because prose is not a table.
  2. **White's queued interpreter item, closed — and its premise was wrong.**
     White found `python -m venv .venv` unrunnable here (no `python` at all;
     `python3` 3.7.3, `/usr/bin/python3` 3.8.2, both under the 3.11 floor) and
     queued it as a *choice* on the grounds that "the only interpreter that
     satisfies it is the uv-managed 3.12 already inside `.venv`". Re-verified
     rather than inherited: **`python3.11` and `python3.12` are both on `PATH`**
     (`~/.local/bin`, uv-managed CPython 3.11.15 / 3.12.13), and
     `python3.12 -m venv` produces a working venv *with pip*. So there was no
     choice to make — the documented command names an interpreter that is not
     there while a working one is, which is a correction. Now
     `python3.12 -m venv .venv   # any 3.11+ will do`, with the three
     interpreter facts recorded beneath it. (The repo's own `.venv` has **no
     pip**, because `uv venv` does not install one; that is a fact about this
     tree, not about the bootstrap, and it is recorded in the environment
     memory so nobody "fixes" the doc to match it.)
  3. **The deck History tab told a reader their earlier edits were "in git".**
     Rendered, on the empty state, and false twice over: ADR 30 took decks out
     of git, so there is no revision holding them, and ADR 28 made the activity
     log the only record there is. It is also **commandment 10** — a
     technology named to a user — which is the half that makes the sentence
     wrong even where it might have been true. Two things let it live. The
     Vitest assertion beside it read `expect(getByText(/in git, not here/))`:
     **a test written against a claim cannot tell you the claim is wrong**,
     which is the [[guards-outlive-their-reason]] shape a third time. And
     `tests/test_deck_location_drift.py` — the tripwire written for exactly
     this class on 2026-08-18 — missed it, because its affirmative list bans
     *verbs* (`tracked`, `held`, `committed`, `diffed`) and this sentence used
     none: it put the **edits** in git rather than the deck. Copy replaced
     ("anything earlier left no trace but the deck itself"), the assertion
     re-pointed, and the tripwire given a subject-anchored pattern —
     `(edits|changes|swaps|history|revisions|records) … (is|are|was|were) in
     git`. Anchoring on the subject rather than on the word *deck* is what
     makes a bare copula safe here: the file's own corrections read "no deck is
     in git" and "none of them is in git", and a deck-anchored pattern flags
     both. Mutation-verified by re-injecting the original sentence.
- **Also landed this run, outside the repo:** two memory corrections.
  `gh-cli-is-in-local-bin` told every session to wrap `gh`, `npm`, `node`,
  `uv` and `fly` in `bash -lc` — **no longer true**: a plain Bash call resolves
  all six (the tool's shell is initialised from the profile now), so the
  wrapper is pure noise on every command. The traps it carries are kept (a
  `|| echo '[]'` fallback turning a missing command into an empty result; check
  `command -v` before declaring a tool absent), and Monitor is marked
  *unverified* rather than assumed. And `MEMORY.md`'s index still listed the
  `camera=()` bug as open, which #184 closed on 2026-08-19 — the entry it
  points at had already been corrected, so only the index was lying.
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
  1. **A deck that picks a printing credits the wrong painter, and three of
     the six do it right now.** Found by the About Claude keeper duty
     (commandment 18), which is where this facet reads artist credits, and
     then confirmed against live deck data. `service.get_deck` swaps the
     commander's **images** for the chosen printing and says so in a comment
     — "the chosen printing replaces the images and nothing else. Oracle text,
     cost, type line and colour identity are the *card's*, and they do not vary
     by printing". True, and incomplete: **`artist` and `flavor_text` do vary
     by printing**, and `_card_json(full=True)` sends both from the
     `oracle_cards` row, which is Scryfall's representative printing and not
     the one on screen. `DeckDetail.tsx` then renders
     `Art by {card.artist} · {card.printing.set_name}` — **the set name from
     the chosen printing and the painter from a different one**, side by side
     in the same sentence. Its own comment states the intended rule exactly
     ("the artist belongs to the printing being shown… otherwise two decks on
     the same commander credit the same painter for different paintings") and
     the code does not keep it.
     Verified without recalling a single card: for each of the three decks with
     a `commander_art`, the chosen `printings.image_normal` differs from
     `oracle_cards.image_normal`, so the two rows are different paintings.
     Atla Palani shows *The List* and credits `dmc`'s painter; Gyome shows
     *Commander 2021* and credits `soc`'s; Trostani shows *Return to Ravnica*
     and credits `c19`'s.
     **Not fixable without a pool change, which is why it is queued:** the
     `printings` table has no `artist` column and no `flavor_text` column, so
     there is nothing to read. The fix is to carry both through the printings
     ingest in `cards/db.py`, bump `SCHEMA_VERSION`, and have `_chosen_art`
     and `_card_art_overrides` return them — which obliges every instance to
     `data refresh` before the credit is right, and `/api/health`'s
     `pool_stale` is exactly the schema check that would say so. The stopgap
     (suppress the credit when a printing was chosen) trades a wrong
     attribution for a missing one, and this project's own standard is that
     every hotlinked painting carries a visible credit — so it is Aaron's call
     which way that goes, not a polish run's. **Cross-color: this is White's
     attribution facet as much as Blue's, and Black's ingest.**

     **CORRECTION, and it is the more useful half: the count was wrong. One
     deck of six credited the wrong painter, not three.** The verification
     quoted above — "the chosen `printings.image_normal` differs from
     `oracle_cards.image_normal`, so the two rows are different paintings" —
     is an **inference, and an unsound one**. A different image URL is a
     different *scan*: a reprint with a new stamp or a new frame gets its own
     URL for the same painting. The field that actually answers "is this the
     same painting" is Scryfall's `illustration_id`, which this pool does not
     store, so the audit reached for the nearest thing and reported it as a
     check. Read out of the 2026-08-19 bulk file rather than the pool:

     | deck | chosen printing | oracle row | same illustration? |
     |---|---|---|---|
     | Atla Palani | `plst` DMC-142, Ekaterina Burmak | `dmc` #142, Ekaterina Burmak | **yes** — `19ac91ff…` both |
     | Gyome | `c21` #332, Steve Prescott | `soc` #313, Steve Prescott | **yes** — `15f7d58c…` both |
     | Trostani | `rtr` #206, **Chippy** | `c19` #204, **Sidharth Chaturvedi** | **no** — `4c60d46e…` vs `893c51a5…` |

     So the page said *"Art by Sidharth Chaturvedi · Return to Ravnica"* over
     Chippy's Return to Ravnica painting, and printed the `c19` flavour text
     ("There are no soloists in the chorus of Selesnya.") under a printing that
     has none. The other two were right by luck: their pinned printing is a
     re-scan of the same painting, so the oracle row's painter *was* the
     painting's painter. **The bug is exactly as real and the fix is
     unchanged** — a credit that is correct by coincidence is not a correct
     credit — but the blast radius was one deck. The lesson is the pass's own:
     *a probe finds which, only the right field finds why*; the audit had the
     right instinct and the wrong column, and wrote it up as a verification
     because the numbers came out of a query.

     **CLOSED 2026-08-19 (PR #195). Aaron ruled: fix it properly, on its own
     branch.** The schema, not the stopgap. `printings` gained `artist` and
     `flavor_text`; `_ADDED_COLUMNS` is keyed by table now, because it held
     only `oracle_cards`' additions and `printings` had never gained one — so
     the mechanism that makes an old pool *readable* did not reach the table
     this change touches. `_chosen_arts` and `_card_art_overrides` return both
     credits, `get_deck` assigns them **unconditionally** (an `or` fallback
     would restore the bug in its quietest form), and `commander_dossier` —
     which sends the same two fields and renders neither — follows the same
     rule so the surface that starts rendering them cannot rediscover it.

     Three things worth keeping. **The staleness question is one word, not
     two:** `pool_is_stale` now asks about the painter as well as the printed
     stats, because both NULLs read as a confident fact ("this card has no
     power", "this painting is unsigned") and `health()` giving two flags
     would mean nobody read either. It short-circuits on an **empty
     `printings`** first, since `--oracle-only` is a supported refresh and a
     deliberate state must not read as an old one. **The credit degrades to
     absent, never to the oracle row's** — that is the stopgap this item
     rejected, but only for the window before a refresh, and `DeckDetail.tsx`
     now renders the set name on its own so a degraded line still says which
     printing was chosen. And **`cardmotion.resolve_subject` stopped asking
     Scryfall**: its docstring said in so many words that "the pool's
     `printings` table stores no artist", which was the truest sentence in the
     repo until this landed and is exactly the shape of claim that goes stale
     silently. It reads the pool first and keeps the fetch as the fallback.

     The measured before/after, looked up rather than recalled, is in the
     *Measured* table below.

     **It left one thing queued, found while landing it: `pool_stale` is
     answered by nobody.** `/api/health` has reported it since the printed-stat
     columns landed, and grepping for a consumer turns up none — the client's
     `Health` interface in `lib/api.ts` does not even declare the field, so
     `App.tsx` and `Library.tsx` both drop it on the floor. The signal is
     therefore honest and invisible: between this deploy and the volume's
     `data refresh`, the instance renders three decks' credit lines as a set
     name with no painter and the only place that explains why is a `curl`.
     Six lines to fix, and the precedent is exact — **Green 5a put
     `SCHEMA_VERSION` on the Admin dashboard for the same argument** (PR #192),
     a number that changes while nobody is watching. Queued rather than fixed
     because it is a UI change and commandment 16 applies. *Cross-color:
     Green's dashboard, Red's alerting.*
  2. **The simulator renders a raw seed, and commandment 10 names seeds
     explicitly.** `routes/Simulator.tsx` draws `<Badge>seed {seed}</Badge>`
     beside every result and a `NumberField label="Seed"` in the controls. The
     commandment's own parenthesis says it was sharpened on 2026-08-17 "after
     'Python rolls' and **a seed rendered on the Wheel**" — the Wheel's was
     taken out and this one was never asked about, which is the
     duration-measured-for-one-surface lesson wearing a different hat. It is
     queued rather than fixed because the *whether* is settled and the *what*
     is not, and two readings of the commandment give different answers:
     (a) **rename the rendered label only** — the glossary's own entry already
     says "the number the shuffler starts from", so `Seed` → `Shuffle` and
     `seed 7` → `shuffle 7` is Magic's word for the thing, strictly clearer to
     a newcomer (commandment 2), and leaves the wire field `seed`, the key
     `sim.seed` and `SIMULATOR_KEYS` untouched — exactly the flavour-the-label
     pattern `lib/claudecopy.ts` is; or (b) **stop showing the number at all**,
     on the reading that a rendered integer *is* the technology however it is
     labelled, keeping "New sample" as the only control. (a) preserves ADR 18's
     reproducibility surface, which is a real feature and the reason runs are
     seeded by default; (b) is the stricter reading of the commandment. Aaron's
     call, and it is a UI change either way, so commandment 16 applies.

     **Landed 2026-08-19 (stragglers), reading (a), PR #191.** Aaron chose the
     rename. `seed 7` → `shuffle 7`, `label="Seed"` → `"Shuffle"`, and the
     `sim.seed` glossary entry relabelled and reworded — it said "seed" four
     more times behind the tooltip, which is the one place a beginner goes to
     be *told* the word. The wire field, the key and `SIMULATOR_KEYS` are
     untouched, so ADR 18's reproducibility surface is intact. The sweep also
     found a fourth site nobody had counted: `/learn`'s search box offered
     "mulligan, ramp, seed…" as example words, and after the relabel that
     example matched nothing. `tests/test_technology_never_renders.py` is the
     tripwire, mutation-verified against all five original shapes.
  3. ~~**`cli.py` is the last strict-mypy exception and it keeps growing:
     79 → 109 → 126.**~~ **CLOSED 2026-08-19 (stragglers, PR #194): 126 → 0,
     and the exception list is now empty.** Annotations only — no restructure,
     and the reason it was cheaper than the rising number suggested is worth
     recording. **53 of the 73 `no-untyped-def` errors were one signature
     written 53 times** (an argparse handler takes a `Namespace` and returns
     nothing), applied in a single pass. **The 52 `no-untyped-call` errors
     were not separate work at all** — they were the same 73 seen from the
     call site, and annotating each callee cleared them with no edit at any
     caller; 126 → 73 → 8 → 0 in three passes. What was left is what every
     earlier graduation left: `Any` coming back out of a JSON-shaped dict,
     named into a local. Types needed only in a signature are imported under
     `TYPE_CHECKING`, so the CLI's deliberate import laziness is intact
     (`mtglab.cli` imports in 178 ms).
     **The claim is now machine-checked rather than commented.**
     `test_no_first_party_module_is_exempt_from_strict_mypy` reads the
     override table out of `pyproject.toml` and what counts as first-party off
     `src/`, so the next module put back on the list fails a test — which is
     the standing question applied to this facet's own trend line, since
     nothing had been stopping the count from rising.
  4. **`numpy` is a core dependency and only the `animist` extra uses it.**
     `pyproject.toml` declares `numpy>=1.26` under `[project] dependencies`;
     the only importers in the whole package are `animist/{ops,encode,motion}.py`,
     which the extra's own comment describes as build-time tooling ("the app
     and the container never transform an image"). So every base install and
     the deployed image carry it for code that never runs there, and CLAUDE.md's
     "Keep `mana.py` and `sim/` dependency-light (stdlib + numpy)" is a
     permission nothing takes up — `sim/` is pure stdlib today. Moving it into
     `animist` (and into `dev`, which vendors that extra) is a **packaging**
     change only the `image` job can fully verify, which is the case the skill
     says to queue rather than bundle. Worth pairing with White's deferred
     `license-files` widening, which is the same job's feedback loop.
  5. ~~**Adopt Claude Code hooks for the two traps that have already cost
     hours.**~~ **Landed in [#135](https://github.com/aasquier/sylvan-library/pull/135),
     2026-08-16** — and the entry above was **stale when it was written**.
     `.claude/settings.json` and `.claude/hooks/guard-git.py` are both tracked,
     the `PreToolUse` matcher on `Bash` is exactly the proposal, and the hook's
     own docstring cites this facet's reasoning back at it. The claim
     "`.claude/` holds `launch.json` and `skills/` and no `settings.json`" was
     already false by two days when the 2026-08-18 run recorded it.
     **This is worth more than the item, and it is Colorless's business:** the
     rule that a queued finding is *not re-litigated* is what preserved it. The
     rule is right — it stops a run bikeshedding a decision waiting on Aaron —
     but "not re-litigated" has to mean "the argument is not reopened", never
     "the tree is not re-checked". Two of Blue's three queued items had in fact
     landed (this and item 4), and neither run noticed.
     The one part still open is the weaker third candidate: a `PostToolUse`
     hook reminding that `web_dist/` needs rebuilding after an edit under
     `web/src`. (A missed rebuild is caught by CI's `frontend` job, so this is
     convenience rather than a guard, which is why it stayed behind.)
  6. ~~**`api/service.py` reaches past `cards/db.py` to `duckdb` directly.**~~
     **Closed.** #181's performance pass took the first of the two proposed
     ways out: `cards/db.py::connect_readonly` exists and `api/service.py`
     calls it, with the only remaining `duckdb` name in that file a
     `TYPE_CHECKING` import of `DuckDBPyConnection` for the cast. Re-grepped
     2026-08-19 across the whole package: **no `duckdb.connect` outside
     `cards/db.py`**, so CLAUDE.md's rule now holds without an exception.
     Kept as a line so the next run does not re-find it.
  7. **`ROADMAP.md` has become a narrative log, and is 3,109 lines.** CLAUDE.md
     describes it as "goals vs reality, open decisions"; a great deal of it is
     now a per-PR account of work already landed, with its own supersession
     markers (the crystal-ball sections are correctly marked superseded by the
     séance photograph, and still there in full). The four planning documents
     total **7,073 lines**, which is the per-session cost the scrub facet
     exists to watch. Not scrubbed here because deciding what a roadmap is for
     is Aaron's, and because a diff that large is the opposite of a polish run.
     A concrete proposal to say yes or no to: keep ROADMAP as goals, open
     decisions and the next two or three items, and move the landed narratives
     to a `docs/HISTORY.md` that nothing reads at session start.

     **Half-answered 2026-08-19 (stragglers, PR #194), and the structural half
     is still Aaron's.** The stragglers pass was asked to weigh whether
     Colorless's argument for *not* trimming the ledger also protects
     `ROADMAP.md`. **It does not, and the measurement is why.** The ledger is
     long because it keeps corrections beside their originals and healthy
     measurements alongside regressions — both of which a later run reads.
     ROADMAP is long because of **one section**: "The near-term TODO" is 1,896
     of the 3,109 lines (61%), it holds sixteen phase items, and **fifteen of
     them have landed**. That is not a correction and not a measurement; it is
     an account of what happened, which git already has.
     So the trim taken here is the surgical half only, and it *adds* rather
     than cuts: a **"Where things stand"** block at the head of that section
     saying what is actually open, pointing at the straggler list below as the
     live one, and stating plainly that when the two disagree the ledger wins.
     A fresh session now gets the answer in twenty lines instead of inferring
     it from 1,896. One stale claim went with it — item 5 said the deployed
     Claude surfaces were "entirely unexercised there", which the `claude`
     live-testing seat has made false since 2026-08-16.
     **What is still queued is the `docs/HISTORY.md` split itself**, unchanged
     and unargued-against: moving the landed narratives out is a decision about
     what a roadmap is for, and it is a large diff in the project's most-read
     planning document. The deferred item below used to pair the ledger's own
     trim to this ruling; **that pairing is cut here**, because the two files
     are long for different reasons and the answer is not the same for both.
- **Deferred:**
  - **`printings.illustration_id`, the column that would have caught the
    miscount above.** Scryfall's stable id for a *painting*, as distinct from a
    printing: it is what makes "these two rows show the same art" a lookup
    instead of a guess at image URLs, and guessing at image URLs is how Blue 1
    came to say three decks when it was one. Left out of the 2026-08-19 fix
    deliberately — the credit needs `artist` and nothing else, and a second
    column means a second mandatory `data refresh` for a field no code reads.
    Trigger: the first feature that has to answer "is this a different
    painting" — deduplicating the art picker's near-identical tiles is the
    likely one, since a card with eight re-scans of one painting currently
    offers eight tiles.
  - **Adopting ruff's `N` group (43 in `src`, 1 in `tests`).** The only excluded
    group whose cost is now plausibly payable, but naming rules rename things,
    which is a wide diff for no behaviour change. Trigger: a session already
    renaming in the same modules.
  - **`ARG`, `PT`, `SLF` stay out** and the re-measure says why (below) — all
    three are overwhelmingly *test* findings, and the fixture/override idiom
    they fire on is correct here.
  - **Three dead public functions, left rather than deleted**, following the
    precedent White set with `ocr.installed()` this cycle. Found by walking the
    package's syntax tree for module-level public defs and counting references
    across `src`, `tests` and `web`. (a) `cardmotion.depth_available()` —
    docstring says "entry points ask this before touching the heavy modules"
    and none do; the mechanism it duplicates is *better*, since
    `depth.load_model()` catches the `ImportError` and raises a `DepthError`
    carrying the install line, which `cli.py` already prints. (b)
    `decks/validate.py::reserved_list()` — the deck-level Reserved List rollup.
    CLAUDE.md's working style says "Reserved List is allowed or forbidden **per
    deck** — check the deck file", and no deck field toggles it, no gate rule
    reads it, nothing calls the function; what *does* ship is the per-card
    `reserved` flag and a badge in `CardSearch.tsx`. So the per-deck half of
    that sentence is aspiration, not code. (c) `src/mtglab/prices/` is an empty
    package created in the first commit (`f49c94f`, Phase 1) and never filled,
    imported or referenced — it ships in the wheel as a namespace with nothing
    in it. Trigger for all three: the next session working in that area, or a
    colorless run willing to take a delete-only diff.
- **Measurements (2026-08-19, the wrong-painter fix — this Mac, full pool,
  re-ingested that afternoon):**
  - **`GET /api/decks`, branch against `origin/main`, same pool, same
    session** — because the credit had to be carried by a query that already
    runs, and "already runs" is a claim to check rather than assert. Four
    samples each of `mtglab bench run --only`, alternated: **branch 21.1 /
    21.3 / 19.3 / 21.7ms** warm median against **main 25.6 / 19.0 / 19.9 /
    20.5ms**. A ~1ms difference inside a spread main's own samples already
    cover twice over — no regression, and no claim of an improvement either.
    The number that actually settles it is structural: `bench profile` reports
    **1 statement per request on both**, with the branch's read as
    `SELECT id, image_normal, set_name, set_code, artist, flavor_text FROM
    printings WHERE id IN (?,?,?)`. Two more columns on the row Black made
    batched in #187, not a second query and not an N+1.
  - **Against the ledger's recorded 16.5ms, these are not comparable and must
    not be read as a regression.** The pool was rebuilt between the two runs
    (35,393 oracle rows, 107,355 printings, 78MB → 98MB) and the shelf now
    serves seven decks rather than six. 21ms is the new baseline; the next run
    compares against that.
  - **pytest:** 2478 passed, 0 skipped in 236.6s (was 2419 in 176.1s at the
    rainbow — this branch adds 8). **Vitest:** 543 in 55.5s (was 537; this
    branch adds 2). `ruff` clean, `mypy` clean over 107 files with the
    strict-exception list still empty.
  - **The pool itself, since the staleness probe now depends on it:**
    **107,355 of 107,355 printings carry an artist**, zero NULL. So a NULL
    `printings.artist` really does mean "this pool predates the column"
    rather than "Scryfall never attributed this printing", which is the
    assumption `pool_is_stale` rests on and the reason it is checked here
    rather than assumed.
- **Measurements (2026-08-19, rainbow — quiet machine, serial run):**
  - **mypy:** clean over **107** source files (was 82 on 2026-08-16 — the
    camera door, the bench and the mutate harness). Strict-exception list is
    still one module: `cli.py` **126**, re-measured by removing the override
    block rather than estimated (73 `no-untyped-def`, 52 `no-untyped-call`,
    1 `no-any-return`). Trend 79 → 109 → 126.
  - **ruff:** clean. Excluded groups re-measured src/tests, against
    2026-08-16: `ARG` **720** (20 src / 700 tests, was 16/604) · `PT` **149**
    (0 / 149, was 0/112) · `SLF` **121** (0 / 121, was 0/67) · `N` **44**
    (43 / 1, was 39/1). The shape is unchanged and is the finding: everything
    but `N` is still ~100% test-side, so the headline numbers overstate what
    adopting them would cost `src` by two orders of magnitude. **A note for
    whoever re-measures next**: `ruff check --select X | tail -1` is *not* the
    count — the last line is "No fixes available…" and the total sits above it.
    That mistake read `PT` as 0 here before it was caught.
  - **pytest:** **2419 passed, 0 skipped in 176.1s**, three of which are this
    run's. The zero is correct here and different from CI — the two
    `needs_full_pool` tests find the real pool on this machine — and `data/`
    was clean afterwards (`app.db` mtime unmoved, checked with `ls -la` rather
    than `git status`, which is blind to it).
  - **frontend:** `npm --prefix web run check` green — **537 Vitest tests
    across 31 files in ~26s** (was 499 / 28 / ~37s on 2026-08-18). Bundle
    rebuilt; only `DeckDetail.js` moved, which is fix 3.
  - **Layering, grepped not trusted:** `api/` imports `cli.py` **nowhere**;
    **no `duckdb.connect` outside `cards/db.py`** (queued item 6, now closed —
    this line said "item 4", corrected by Colorless);
    `mana.py` and `sim/` are stdlib-only — note *not* stdlib+numpy, since
    **numpy appears in no module outside `animist/`** (queued item 4, likewise
    corrected from "item 2"); no
    module-level `anthropic`, `PIL`, `dotenv`, `torch` or `numpy` anywhere in
    `src/`; no 3.12-only syntax, so the `>=3.11` floor holds.
  - **Frontend compatibility:** **zero regex lookbehind** under `web/src`
    (Safari 15 is the dev machine); no `structuredClone`, `findLast`,
    `toSorted`, `toReversed` or `Object.hasOwn`; the one `.at(-1)` is in a
    `.test.tsx`, which runs under Node and never ships. No `forwardRef`,
    `React.memo`, `defaultProps` or `propTypes` — React 19 idiom throughout.
    No non-null assertion outside tests, now that oxlint holds it.
  - **Doc/tree consistency, counted rather than read.** Every source path named
    in CLAUDE.md, ROADMAP.md, `web/README.md`, ENGINEERING.md, HOSTING.md and
    the two skills resolves against `git ls-files`. Every *number* CLAUDE.md
    asserts was evaluated against the code and **all of them hold**: 7
    read-only tool schemas, 7 personas, 78 tarot cards, 377 `tarotlore` facts
    (359 of them card facts), 986 set codes in the pool, 41 glossary terms,
    32 colour combinations. The two that had drifted were the two nobody
    counts — the extras enumeration (fix 1) and `cli.py`'s error count.
    Architecture block gaps, recorded rather than filled because the block is
    selective by design: `prices/`, `decks/library.py`, `decks/sqlsource.py`
    and four `api/` modules (`admin`, `adminstats`, `traffic`, `flymetrics`)
    are named nowhere in it.
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

### The spirit of Magic — first sweep, 2026-08-19

The facet's baseline. Nothing here is a delta; the point of this section is
that the next run starts where this one stopped rather than re-reading the
same 24,712 lines of `web/src`.

**Swept:** every rendered string reachable by grep across `web/src/routes`,
`web/src/components` and `web/src/lib` — labels, buttons, empty states, error
and loading copy, placeholders, `aria-label`s and short JSX text nodes — plus
a technology-name sweep for `git`, `YAML`, `Python`, `DuckDB`, `SQLite`,
`SQL`, `React`, `FastAPI`, `JSON`, `HTTP`, `database`, `server`,
`localStorage`, `seed` and `token`.

**The headline, and it is not a flavour finding:** the vast majority of those
hits are in *comments*, which commandment 10 does not reach. Exactly **two**
render. One is fixed above (the History tab's "in git"); the other is the
simulator's seed, **queued as item 2** (this line said "item 0"; corrected by
Colorless) because the wording is a decision. That is
a good result for a first sweep and worth writing down as the baseline —
this app is not leaking its machinery, it leaked twice.

**Considered and rejected, so the next run does not re-litigate it:**

- **`deck.yaml`, rendered in four places** (`Import.tsx` twice, `DeckDetail.tsx`
  twice). Left alone. It reads as a technology name and is not one *here*:
  this is a local-first tool whose CLAUDE.md calls the file the source of
  truth, `Import.tsx` shows you the file it is about to write, and the deck
  page's rationale tab says the words live there rather than in a database
  nobody can open. Taking the name away would make the sentences vaguer, not
  more immersive. Revisit only if the hosted instance ever gets users who have
  no shell.
- **The admin dashboard's "Tokens in / Tokens out / Cache reads"**
  (`routes/Admin.tsx`). Maintainer-facing behind `/api/admin`, and the units
  *are* the subject of the page. Not a commandment 10 surface.
- **"Cancel", "Close", "Username", "Email", "Submit".** Generic, and correctly
  so — commandment 2 draws the line and none of these has a Magic word that
  leaves a newcomer better off. A "Cancel" button that says "Fizzle" is a
  regression wearing a costume.

**The enrichment shortlist — what is absent that could be there.** Nothing
from this list was built this run; the run's diff was already at its surgical
cap with three fixes, and every item below is user-visible, so commandment 16
applies to each. Ordered by cost:

1. **Rarity has a symbol and renders nowhere.** `lib/api.ts:180` carries
   `rarity` on every card and no surface draws it — `CardSearch.tsx` finds room
   for a "reserved list" badge and not for the one mark every Magic player
   reads first. The expansion symbol's colour *is* the rarity (black, silver,
   gold, orange), which makes this `symbols.py`'s kind of problem rather than a
   new asset: check whether Scryfall's set-symbol endpoint is on the same
   licence footing the mana symbols are (ADR 33) before proposing anything.
2. **The fortune-teller's table is where commandment 15 says an enrichment
   goes first**, and the corpus is already checked in: 377 facts across all 78
   cards, of which only the reader's cited ones are ever seen. A dealt card's
   own picture facts could carry the plate's detail on hover, without a model
   in the loop and without a new asset.
3. **Empty states could carry a card's own words.** `lore.py`, `colors.py` and
   `tarotlore.py` are checked-in prose, and the pool holds flavour text that is
   free, licensed to render and better written than any copy a checklist
   produces. "Nothing on the shelf yet." (`Library.tsx:229`) is the obvious
   first candidate. Rule 1 binds hard here: the line must come *from the pool*,
   never from recall, because it will be rendered.
4. **Zones that are not named as zones.** Checked this run and mostly already
   right — the graveyard, exile, entomb and return are all in use. The
   remaining generic ones are the simulator's "Wasted"/"No land" table columns,
   which are Magic concepts (mana burn's descendant, the missed land drop)
   wearing spreadsheet labels.

## Black — Ruthless Efficiency

*Claude API spend · static assets · performance*

- **Last run:** 2026-08-19 (rainbow). Previous: 2026-08-16 (rainbow), plus two
  un-run entries from the same week — the targeted performance pass and the
  measuring shelf — both kept below.
- **Measured out of band (2026-08-20, ADR 35 live — the hosted Forge's cost
  shape, from the first paid matches on the instance):** wake from `stopped`
  is **~5s** (hand-timed against the real machine; `worker.BOOT_SECONDS=90`
  is the refusal ceiling, not the expectation). JVM boot plus the card
  database is **~14s** of startup inside the match. A 3-game heads-up match:
  median **5.5–5.8s/game**, **~40s wall**, ~4s of it on the app's request
  thread — inside the spike's local band, so the dedicated CPU is honest.
  The shim self-stops after 180s idle, so one match occupies the machine for
  roughly wall + 180s; at performance-2x/4GB rates that is **~1¢ per
  match**, and a stopped machine bills only its rootfs storage. Baseline,
  not a finding — the number a future "the worker feels slow/expensive"
  gets measured against.
  1. **The shelf asked for one printing per pinned deck.** `_tiles` batches
     the gate, the pool lookup and (one layer down) each deck's *card* art —
     `_card_art_overrides` refuses the N+1 in its own docstring — and then
     called `_chosen_art` inside the per-deck loop, which is one
     `SELECT … FROM printings WHERE id = ?` per deck that pinned a commander
     printing. Three of the seven local decks pin one, so the shelf issued
     **3 queries where 1 does**; a library aimed at 32 slots would issue 32.
     Now `_chosen_arts` takes the run of ids and `_chosen_art` is one deck's
     worth of it, so the deck page keeps the single-row form and there is
     still only one SQL string. **`/api/decks`: 3 statements → 1, database
     2.13ms → 0.98ms, warm p50 17.4ms → 16.5ms, cold p50 138.9ms → 134.2ms.**
     Mutation-verified test counts the `printings` queries rather than the
     milliseconds (`test_the_shelf_asks_for_every_pinned_printing_at_once`),
     the same instrument `challenge_progress` got, and covers the stale-id
     case: an id the pool no longer holds drops out of the batch and its tile
     falls back to the default art rather than blanking.
  2. **The one CDN left in the bundle was held off by a comment.**
     `cdn.jsdelivr.net` appears in `assets/reader.js` — it is tesseract.js's
     *default* `workerPath`, and `reader.ts` overrides it along with
     `corePath` and `langPath` so every byte comes from `/api/ocr`. That is
     right, and `reader.ts` says why in a comment. But a comment is not a
     guard, and the failure it prevents is the quiet kind: a `createWorker`
     call missing one option fetches **unpinned WebAssembly from a third
     party**, bypassing the SHA-256 pin `ocr.py` exists to enforce, leaking
     every visitor's IP, and still working. `reader.test.ts` now mocks
     `tesseract.js` and asserts all three paths are set and first-party.
     Mutation-verified by deleting `corePath`. (White's rule holds
     throughout: the engine files stay uncommitted and unhotlinked, served
     first-party out of the runtime cache. Nothing about that arrangement
     moved.)
  3. **Two comments named a cause nobody had measured**, which is the exact
     failure this facet was rebuilt around. `bench/profile.py` attributed the
     ~900 cold import calls to "a fresh DuckDB connect does real path work" —
     checkable, and wrong: cold `db.oracle_columns` connects and reports
     **1**. Traced by recording every `__import__` during a cold
     `/api/decks`: **912 calls, 892 of them `import pandas`** from `db.py`'s
     parameter binding, still asked twice per bound value and now answered by
     the `sys.modules` sentinel. Timed: **892 defused probes cost 0.96ms
     against 86.7ms undefused** — so a cold ~900 is #181's fix *holding*, and
     a warm one is the alarm. Separately, `modes.py` claimed the interview's
     cached prefix is "~1.5k tokens"; measured with `count_tokens` it is
     **2,373**, and every prefix figure in this ledger was chars/4 and read
     ~40% low. Both comments now carry the measurement and the date.
- **Queued for Aaron (both carried from 2026-08-16, re-checked this run):**
  1. **Cache-write tokens are still invisible, and they are still the priciest
     class.** Unchanged in code: `modes.converse` records `input_tokens`,
     `output_tokens` and `cache_read_input_tokens`, and drops
     `cache_creation_input_tokens`. Writes bill at **1.25× input**, so
     `mtglab claude usage`'s **$1.64** is a floor on the bill, not the bill —
     and `prices.py`'s own docstring says so. Two things changed around it.
     The migration is now **v11, not v9**: schema v9 and v10 both landed for
     other work since this was queued, which is evidence the "own branch,
     merged while somebody is watching" cost is one this repo already pays
     routinely. And the instrument matters more than it did, because the
     Sonnet 5 introductory rate ends **2026-08-31** and every figure this
     table produces rises 50% on 2026-09-01.
  2. **The theme conversation's second cache breakpoint — payoff now bounded.**
     The mechanism finding is unchanged and not re-litigated: `theme._messages`
     still marks the *closing instruction*, which is stripped and re-appended
     each turn, and the previous run verified deterministically that turn N's
     marked region is not a byte-prefix of turn N+1's request. What this run
     adds is what it is worth. Across **77 real theme-conversation turns** in
     the local ledger, uncached input totals 9,815 against 1,252,433 cache
     reads — **99.2% of the prompt is already served from cache**, with the
     per-turn uncached remainder running 2–1,455 tokens and including the
     querent's new answer, which can never be cached. So the proposed fix
     (move the marker to the last settled transcript block) is worth at most
     ~0.8% of this mode's input. Still queued rather than fixed — it changes
     what a paid path caches and only spending confirms it — but it is a
     small item, not a large one, and (1) remains the prerequisite instrument.
  3. **`load_printings` ingests at ~110 rows a second, and the whole refresh
     takes 28 minutes.** Added 2026-08-19, found by watching a `data refresh`
     that the wrong-painter fix (Blue 1) made mandatory — the previous bulk
     file was dated 2026-08-10, so nobody had timed one in over a week and
     `CLAUDE.md` had said *"several minutes"* since the tool was written.
     **Measured, not estimated:** both downloads complete in ~9 minutes
     (24.5MB oracle, 77.5MB default, both gzipped), and `load_printings`
     spends the remaining **~16 minutes on 107,355 rows**.

     **The cause is profiled rather than guessed**, which is the difference
     between this and a plausible sentence: macOS `sample` on the live process
     puts the time in `duckdb::DataChunk::Initialize` and in `malloc`/`free`
     churn across sixteen threads. That is allocator overhead, not insertion —
     the signature of `executemany` driving DuckDB's **prepared-statement path
     once per row**, re-initialising a DataChunk and engaging the parallel
     executor each time, instead of taking a bulk path at all. It is
     pre-existing: Blue 1's branch names the INSERT's columns explicitly
     (a strict improvement on the positional `VALUES (?…)` it replaced) and
     leaves the batching exactly as `main` has it.

     **Direction, for whoever takes it:** DuckDB can read the JSONL itself
     (`read_json_auto` over the gzipped file, one statement) or accept an
     Arrow table, either of which replaces the row loop with a real bulk
     path. Measure before and after — a large number is a question, not a
     datum — and note that `load_oracle` has the same shape over 35,393 rows
     and should be measured in the same sitting rather than assumed fine.

     **There is a robustness half, and it may matter more than the speed.**
     `load_printings` runs `DELETE FROM printings` *before* the loop, so those
     16 minutes are a window in which an interrupted refresh leaves the pool
     with **no printings at all** — every deck page falls back to default art
     and the art picker offers nothing, with no error anywhere to explain it.
     A single bulk statement is atomic where a 22-batch loop is not, so the
     two arguments point the same way. Written into `CLAUDE.md`'s refresh
     paragraph in the meantime, which is where somebody looks before pressing
     Ctrl-C. **Not implemented deliberately** — Aaron's call was to queue it
     rather than tangle a loader rewrite into a branch already waiting on his
     eye.
- **Deferred (re-checked, both still deferred):**
  - **Interview and single-card argue have neither a cache nor an in-flight
    dedupe key.** The trigger named last time was "either endpoint becoming a
    background job". It half-arrived and resolved itself: the *deck sweep*
    became a job (`api/argueruns.py`) and it carries
    `key=f"{slug}:{fingerprint}"`; the single-card endpoints stayed
    synchronous on a measured seconds-scale claim written into
    `app.py`'s docstring. Every other paid surface is now covered — scan by
    `key=f"scan:{digest}"`, research by `request.key`, dossier by key *and*
    the `oracle_id` cache, theme deliberately by neither with the argument
    written down. Trigger unchanged: either single-card endpoint becoming a
    job, or gaining a caller that is not the deck page.
  - **Long max-age for the immutable media** (219KB `ivy-canopy.webp`, 126KB
    `bookworm-still.webp`, 78KB `tarot-back.webp`, the 78 tarot PNGs).
    Unchanged and still blocked on the same thing: `assetFileNames:
    'assets/[name].[ext]'` means an animist rebuild reuses the filename, so a
    long max-age would serve a stale asset. Trigger: media bytes growing
    enough that per-navigation revalidation matters — the fix is
    content-hashing *media* names only, which need not stay diff-legible the
    way JS chunk names do.
- **Measurements (2026-08-19, rainbow — this Mac, quiet, full pool):**
  - **Claude spend to date** (local ledger, 2026-08-16 → 2026-08-19): 86
    conversations / 95 requests / 17,133 input / 124,765 output / 1,804,679
    cache reads = **≈$1.64** at list rates, up from ≈$0.35 three days ago.
    Per mode: `theme-conversation:fortune-teller` 74 conv (9,815 / 92,990 /
    1,252,433), `theme-proposal:fortune-teller` 3 conv / 11 req (30 / 21,892
    / 321,797), `commander-dossier` 1 conv / 2 req, `scan` 5 conv (6,450 /
    199 / **0**), one conv each for the therapist, chef and storyteller
    voices. Every row is `claude-sonnet-5`; no tiered seat has spent yet.
  - **Prompt cache ratio 105:1**, down from 211:1 — and the whole drop is
    `scan`, which reads nothing. Excluding it the ratio is 169:1.
  - **Per-mode cached prefix, measured rather than estimated.** `count_tokens`
    is free and replaces the chars/4 figures, which read ~40% low: research
    2,062 · dossier 2,298 · interview 2,373 · slot-argument 3,298 ·
    theme-conversation 5,234 (personas 5,716 barkeep → 5,849 fortune-teller)
    · theme-proposal 6,587 · **scan 478**. Server-tool schemas are excluded
    (the endpoint refuses them), so the four searching modes are a floor.
  - **`scan`'s cache marker is inert, and that is correct.** 478 tokens clears
    neither Sonnet 5's 1,024-token minimum nor the 512 an Opus or Fable seat
    gets, and the ledger proves it empirically: **five identical scans three
    seconds apart, 1,290 input tokens each, zero cache reads on every one.**
    Not a bug and not worth padding — buying a tenth of 478 tokens with 546
    wasted ones is a loss. It is the first mode below the floor, which is why
    the 2026-08-16 claim "all above Sonnet 5's minimum" needed this note.
    Cost of the residue: ~$0.001 a scan, ~$0.10 for a 100-card camera import.
  - **What a camera deck import costs, since Green will want it.** Measured at
    1,290 input and ~40 output tokens a card, a 100-card import is 129,000 in
    / 4,000 out = **≈$0.30 today, ≈$0.45 from 2026-09-01**, on the paid tier.
    It is the first surface whose cost scales with *how much a person does*
    rather than how many times they ask a question, which is what makes it
    the one worth a quota conversation. The free tier exists beside it (local
    OCR, ~24s a card against Claude's ~3.1s) and is the reason this is a
    choice rather than a bill — but see the cross-color note below.
  - **Spend knobs, seven modes now.** Model `claude-sonnet-5` for everybody by
    default; `tiers.py` grants `opus` (`claude-opus-5`) or `fable`
    (`claude-fable-5`) per account, resolved **once per conversation** in
    `converse` so a turn cannot change model and throw its cache away.
    `max_tokens` 8,192 (interview, argue, theme conversation) / 16,384
    (dossier, research, theme proposal) / **2,048 (scan)**. `effort: high`
    everywhere except **scan, which is `low` — and argued as accuracy, not
    thrift**: higher levels make a transcriber infer, which is the one
    behaviour ADR 34 forbids. Web search `max_uses`: dossier 4, research 4,
    theme proposal 3, theme conversation 1; scan has no tools at all.
    `MAX_TOOL_TURNS` 6.
  - **`prices.py` verified against the `claude-api` skill's pricing table**,
    rate by rate: Fable/Mythos $10/$50, Opus 5 and 4.6–4.8 $5/$25, Sonnet 5
    $2/$10 introductory **through 2026-08-31** then $3/$15, Sonnet 4.6 $3/$15,
    Haiku 4.5 $1/$5, cache reads at 0.1×. Every one matches, the intro window
    is modelled rather than flattened, and `CHECKED = 2026-08-18` renders
    beside every figure. **Consequence for Green's quota work: on 2026-09-01
    the same traffic costs 50% more** — today's $1.64 becomes ≈$2.46.
  - **Refusable-before-the-call still holds on every paid surface**, scan
    included: `scan._payload` refuses an unknown media type or bad base64
    before a request is built, so no mode errors *after* spending.
  - **Warm suite** (`bench run --runs 15`, post-fix). Compare next quarter
    against these:

| target | warm median | p95 | database |
|---|---:|---:|---:|
| `GET /api/health` | 7.3ms | 8.2ms | — |
| `GET /api/decks` | **16.5ms** | 18.0ms | — |
| `GET /api/lore` | 5.9ms | 7.2ms | — |
| `GET /api/colors` | 6.1ms | 7.2ms | — |
| `GET /api/glossary` | 4.5ms | 4.9ms | — |
| `GET /api/cards/search?q=goblin` | 43.8ms | 49.0ms | **37.9ms, 1 statement** |
| deck detail | 7.1ms | 8.0ms | — |
| deck validate | 6.0ms | 6.7ms | — |
| `db.get_cards` (100 names) | 0.2ms | 0.3ms | — |
| `db.oracle_columns` | 0.2ms | 0.2ms | — |
| `db.connect_readonly` | 0.2ms | 0.3ms | — |
| `Deck.load` | 0.0ms | 0.1ms | — |

  - **Cold suite** (`--cold`, every registered cache emptied between samples):
    `/api/decks` **134.2ms** (3 statements, was 138.9/5) · `db.get_cards`
    83.2ms · `/api/lore` 80.4ms · deck detail 80.0ms · deck validate 77.7ms ·
    search 67.1ms · `/api/health` 38.1ms · `db.oracle_columns` 29.1ms ·
    `db.connect_readonly` 15.5ms · `/api/colors` 6.3ms. The shelf is still
    **8× slower cold** and the memo is still the whole difference.
  - **Search is unchanged and the tool still says why**: 37.9ms of a 43.8ms
    wall inside one DuckDB statement — a text scan over 35,390 rows that never
    goes through `get_cards`, so no memo can help it. The lever is the query
    or an index, and both are ingest-shaped; nothing to do here surgically.
  - **Cache hit rates** (`bench caches --runs 5`), unchanged and healthy:
    `deck.parsed` 60/0 (100%) · `pool.columns` 15/0 (100%) · `pool.keeper`
    36/0 (100%) · `pool.cards` 27/3 (90%). `auth.hasher`,
    `auth.dummy-hash` and `sets.upcoming` read *never asked*, which is the
    suite not logging in rather than a dead cache. **No cache has been added
    since the register landed**, and nothing in it is dead weight.
  - **Live instance** (5 samples each, p50 time-to-first-byte from this Mac):
    `/` 237ms · `/assets/app.js` 222ms · `/assets/charts.js` 216ms ·
    `/api/health` 213ms. Within noise of 2026-08-16 (240 / 192 / 177 / 211)
    and dominated by RTT to `sjc`.
  - **⚠️ The header check needs a GET, not a HEAD — this nearly became a false
    regression.** `curl -I` against the instance returns *no*
    `content-encoding` and the raw `content-length`, which reads exactly like
    compression having been switched off. A real GET shows the truth:
    `content-encoding: gzip`, `vary: Accept-Encoding`, and `app.js` arriving
    in **90,824 bytes** against 291,488 raw — matching the local `gzip -9`
    figure to five bytes. Still no brotli. Every asset is `cache-control:
    no-cache` with a strong `etag`, and a conditional request returns **304**,
    so the cost stays one revalidation RTT per navigation. (A HEAD of `/` also
    answers `application/json`, 31 bytes — the SPA catch-all only handles GET.
    No browser HEADs the document root; noted so the next run does not chase
    it.)
  - **Static assets over hotlinks**: the served app still references exactly
    one external host at runtime — `cards.scryfall.io`, for Wizards' art,
    which White's licensing verdict keeps as a hotlink, with a
    `<link rel="preconnect">` warming it. **No CDN for code, fonts, CSS or
    scripts.** The `cdn.jsdelivr.net` string now in `assets/reader.js` is
    tesseract.js's inert default, overridden at the one call site and pinned
    by a test as of this run (fix 2). Everything else external in the bundle
    is inert: the SVG namespace, `bit.ly` inside Immer's minified-error
    message, and repository/funding URLs in vendored package metadata.
    `edhrec.com`, `console.anthropic.com`, `platform.claude.com` and
    `fly-metrics.net` are links a person clicks, not fetches. Nothing in
    category (c): no dead or replaceable external URL found.
  - **Cross-color, for Red or Green: an auth question with a spend answer.**
    `/api/ocr` is not in `PUBLIC_PATHS`, and memory records it answering 401
    on the instance with the worker's own fetches *unverified*. Not re-derived
    here and not Black's to settle — but the consequence is: if the free
    reader cannot load its engine deployed, **every camera scan falls through
    to the paid tier**, and the number above stops being a choice. Settling it
    needs a signed-in pass on the real instance (commandment 14), which is
    Green's ground.
  - **`mtglab animist verify`: every committed asset held to its recipe.**
  - **Bundle** (committed `web_dist`, gzip -9): `charts.js` 399,398 /
    **111,241** · `app.js` 291,488 / **90,829** · `ivy-canopy.webp` 219,330 ·
    `bookworm-still.webp` 125,892 · `index.css` 99,824 / 21,844 ·
    `DeckDetail.js` 95,860 / 26,743 · `tarot-back.webp` 77,818 ·
    `NewDeck.js` 52,855 / 15,203 · `reader.js` 17,231 / 7,190. **The
    `charts.js` deferral holds**: `recharts` is imported only by
    `components/charts.tsx`, and the three heavy routes import
    `components/lazycharts.tsx` instead — re-checked over the import graph,
    not the size table, which is the distinction that cost 113kB last time.
  - **Tier 1 / `SIM_VERSION`**: not touched. No engine change, so the ADR 18
    cache is intact and the determinism digest did not move.

### The 2026-08-16 run

Superseded by the block above; kept for its numbers and its
reasoning. Its queued and deferred items are carried forward there.

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
- **Queued then, and carried forward above — do not act on this copy:**
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
- **Deferred then, both re-checked above:**
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
- **Checklist corrected that run:** `references/black.md` said committed assets
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

- **Last run:** 2026-08-19 (rainbow). Previous: 2026-08-18 (punch-list item 5,
  with Blue), and 2026-08-16 (rainbow), which was the first Red run and the
  baseline the numbers below are a trend against.
- **Fixed this run (2026-08-19, rainbow): the deploy job's `needs` list is
  the whole safety argument and nothing checked it.** `deploy` cannot start
  until the four jobs above it are green, which is what makes it
  *structurally* unable to ship a red suite — and, for ADR 23's manual button,
  it is the **only** guard there is: branch protection governs merging and has
  nothing to say about a `workflow_dispatch` on `main`. Adding a job to
  `ci.yml` is already known to be two steps (the `image` job's own comment
  says so about the required-checks setting); it is really three, and the
  third was unenforced. Miss it and the new job runs, goes red, and the
  instance is replaced anyway — presenting as one red check beside a
  successful deploy, which reads like flakiness rather than an unguarded
  release. `test_the_deploy_job_waits_for_every_other_job_in_the_file` in
  `tests/test_packaging.py` **derives** the expected set from the file's own
  job list rather than restating it, which is the #86 lesson applied to the
  other half of the same job: the sibling test right above it exists because
  a substring check passed against the exact bug it was written for. Same
  module, same text-not-YAML idiom, no new dependency. **Mutation-verified
  three ways** — drop `image` from `needs`, add a job nobody wired in, name a
  job that does not exist — each fails it, and the message names which
  direction. The most useful consequence is forward-looking: queued item 7
  below proposes splitting `image` in two, and this is the test that catches
  the split forgetting to update `needs`.
- **Fixed (2026-08-18): the alarm tile could not tell "all clear"
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

     *Sharpened 2026-08-19, and it does not replace the above.*
     `docs/HOSTING.md` §5 already says alerting belongs in **Fly's managed
     Grafana**, auto-provisioned per organisation at `fly-metrics.net` and
     free, and names the two rules worth having first (memory above ~85%, a
     5xx rate at the edge). That substrate is closer to reachable than this
     item implied: `FLY_METRICS_TOKEN` **is set** as a secret, so Fly's
     Prometheus is live and the admin page already reads it. Whether any alert
     *rule* exists there is unverified — it is behind Aaron's Fly login and
     this run did not drive it. The reason it does not close this item is
     fate-sharing: managed Grafana runs on Fly, reads Fly's own Prometheus,
     and would be alerting about Fly — the same objection that ruled out
     self-hosting ntfy. So the honest recommendation is **both, and they are
     not redundant**: Grafana for *symptom* rules it is genuinely good at and
     already paid for (memory, 5xx rate, migration-failed-on-boot), and an
     off-platform probe for *liveness*, which by definition cannot live on the
     platform it watches. Cheapest first step is therefore free, not $5: ask
     Aaron whether fly-metrics.net has any rule at all.
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
  7. **The `image` job spends 60% of itself emulating an architecture nothing
     deploys to — three options, and Aaron picks.** Profiled this run (run
     `32312916793`, `image` 302s): setup 23s, amd64 build 63s, the eight
     container assertions 8s, Trivy 16s, and the multi-arch build **182s** —
     which is arm64 and nothing else, because every amd64 layer is already
     cached by then. Inside it, `RUN pip install` **101.5s** and
     `RUN python -m venv` **32.8s**, both running an emulated interpreter
     under QEMU. Then the fact that reframes it: `uname -m` on the live
     machine answers **`x86_64`**. The instance is amd64, `flyctl deploy
     --local-only` builds amd64 on an amd64 runner, and nothing publishes the
     arm64 manifest (`push: false`) — so this leg is a portability check for a
     host that does not exist yet. The Dockerfile's own comment ("anything
     this would deploy to is arm64") is the decision it was added under, which
     is exactly why changing it is Aaron's and not a safe fix. The options:
     **(a) drop the arm64 build** — settles the critical-path tie outright,
     `image` falls to roughly 120s, and the cost is losing a claim nobody
     currently relies on; **(b) move it to a native runner** —
     `ubuntu-24.04-arm` went GA and **free for public repositories** on
     2025-08-07, so the same check runs unemulated at roughly amd64 speed,
     either as a second job in parallel or as a matrix leg; **(c) keep it.**
     If (b), three things move together: each build needs its own buildx
     `scope=` so the architectures stop sharing one gha entry, the native leg
     no longer needs `docker/setup-qemu-action`, and **a second job must be
     added to `deploy`'s `needs`** — which this run's new test now forces
     rather than trusting. Either (a) or (b) is the first thing in six runs
     that would actually move the wall clock, because it is the only lever
     that acts on the job that is the critical path half the time.
  8. **Secret scanning and push protection are off, and both are free on a
     public repository.** Confirmed rather than assumed: the API answers
     `404 Secret scanning is disabled on this repository`. What the repo has
     instead is `no-secrets-or-card-data`'s `git grep` for `sk-ant-…`, and
     that guard is **post-hoc by construction** — it runs after the push, on a
     public repository, so when it fails the key is already published, which
     is precisely why its own failure message says "REVOKE IT in the console
     first". Push protection refuses the push at the client, before the bytes
     leave the laptop. And the self-healing half is the part that belongs to
     this facet: Anthropic has been a GitHub secret-scanning **partner** since
     2024-08-20, so a key that does leak from a public repo is forwarded to
     Anthropic, revoked, and the owner notified — detection *and* remediation,
     with nobody in the loop. Repository settings, so Aaron's to flip
     alongside #4. The CI grep stays either way: it also enforces the
     card-data half of rule 5, and a repo-specific belt costs six seconds.
- **Deferred:**
  - **Speeding up the `image` job in place — still deferred, and half of the
    2026-08-16 reasoning behind it is now disproven.** The trigger this item
    named ("the test job gets materially faster") **has not arrived and moved
    further away**: `test (3.12)` went 284s → 300s → **317s** while `image`
    sat at 292s → 304s → **302s**, and across n=20 today the critical path was
    `test (3.12)` **11 times** and `image` **9**. It is still a coin flip, so
    fixing either alone still buys almost nothing. (Blue's leg reported
    `image` 6m0s against `test (3.12)` 3m36s from **its own PR run** and read
    that as the trigger firing; over twenty runs it is a single-run artifact —
    that 216s is the fastest `test (3.12)` in the whole window. One PR's
    medians are a lead, not a baseline.)
    Two corrections to the mechanism, both from the run log rather than from
    reasoning. **The cache-scope claim was wrong**: arm64 layers *do* survive
    between runs — `#9 WORKDIR`, `#21 COPY pyproject.toml`, `#25 apt upgrade`
    and `#26 useradd` all came back `CACHED` on the arm64 side, so the
    amd64-only build is not overwriting them and there is nothing to fix
    there. What misses is exactly what should: everything below
    `COPY src ./src`, because src changed. **And the numbers shrank**: the
    QEMU leg is 182s now, not 214s, of which `pip install` is 101.5s (not
    142s), `venv` 32.8s (not 46s) and the export 32.5s. So the only piece
    recoverable *without* changing architecture is the 32.8s venv, by moving
    `RUN python -m venv /opt/venv` above `COPY src ./src` — it depends on
    nothing but the base image and today every commit invalidates it. ~33s off
    a 302s job, unprovable on this Mac, and pointless while the path is a tie.
    **Queued item 7 supersedes this as the thing actually worth doing**: drop
    the arm64 leg or run it native, either of which takes ~180s rather than
    ~33s. Trigger to revive *this* item: Aaron rules "keep it as is" on 7, and
    the venv move then becomes the only lever left.
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
- **Measurements (2026-08-19, rainbow):**
  - **CI per-job medians, n=20** — every successful `ci.yml` run of the day,
    13:41Z to 23:21Z, which is #179–#187 and their pushes. Against 2026-08-18
    and the 2026-08-16 baseline:
    `test (3.11)` **186s** (154–201, was 186, 158) · `test (3.12)` **317s**
    (216–339, was 300, 284) · `frontend` **45s** (36–49, was 44, 36) ·
    `no-secrets-or-card-data` **6s** (4–10, was 6, 5) · `image` **302s**
    (273–364, was 304, 292) · `deploy` **81s** (74–91, n=9, was 78).
    Full-run wall clock **301–364s, median 329s** (was ~307s). Separate
    workflows: `codeql` **48–60s per language** on a PR and **84–96s** on a
    push; `dependency-review` **5–9s**. Everything is a few percent up on a
    day that added ~1,400 lines of tests; nothing is a step change. **Critical
    path: `test (3.12)` 11 of 20, `image` 9 of 20** — still the tie the
    deferred item is built on, at 317s vs 302s.
  - **`image` internal profile** (run `32312916793`), because a 302s job is a
    question: setup+checkout+qemu+buildx 23s · **amd64 build 63s** · the eight
    container assertions 8s · Trivy 16s · **multi-arch build 182s** · post 5s.
    The 182s is arm64 alone (every amd64 layer is cached by then):
    `COPY src` 6.6s, **`python -m venv` 32.8s**, **`pip install` 101.5s**,
    export 32.5s, everything else `CACHED`. See queued item 7 — the live
    machine is `x86_64`, so this is emulation for a host nobody deploys to.
  - **Cache health: all green, read from run `32312916793`'s log.** pip hit
    (~119 MB, keyed on `pyproject.toml`), npm hit (~65 MB), QEMU binfmt hit,
    Trivy binary hit, Trivy vulnerability DB hit on the date-keyed
    `cache-trivy-2026-08-19`, buildx amd64 layers `CACHED` — **and, correcting
    2026-08-16, arm64 layers too.**
  - **Concurrency verified by observation, including a null result that is
    not a fault.** Three `ci.yml` PR runs were cancelled today as designed
    (white, blue, black). CodeQL's superseded Black run was *not* cancelled
    and that is correct: it finished at 23:21:39, **seven seconds before** the
    superseding push at 23:21:46, so there was nothing to cancel.
    `dependency-review` at 5–9s is never slow enough to be caught at all. A
    group with no cancellations is not evidence of a broken group.
  - **Actions hygiene:** **18 `uses:` references across 11 distinct actions**,
    every one a 40-char SHA with a version comment. (The 2026-08-16 entry's
    "all 11 action references" was the distinct-action count; both numbers are
    recorded now so the next run does not read a mismatch as drift.) No
    `pull_request_target`; workflow-level `permissions: contents: read` on all
    three files with `security-events: write` scoped to CodeQL's job alone;
    default workflow permissions `read`; `can_approve_pull_request_reviews`
    false. Repository still `allowed_actions: "all"` and
    `sha_pinning_required: false` (queued item 4).
  - **Pinned invariants, read rather than assumed.** Required contexts from
    the protection setting are the same six: `test (3.11)`, `test (3.12)`,
    `frontend`, `image`, `no-secrets-or-card-data`, `dependency-review`;
    `strict` true, `enforce_admins` true, linear history true, zero required
    reviews. Skip gate still `expected=2` (`addopts = "-ra"` is what makes
    `grep -c '^SKIPPED'` work, and a change there fails loud rather than
    quiet) — **correcting the 2026-08-16 entry's parenthetical**, the *local*
    suite skips **0**, not 2: this Mac has `data/mtg.duckdb`, so both
    `needs_full_pool` tests run rather than skip, and 2 is the CI number by
    construction. 2,421 passed locally this run. `image` still the only place
    the Dockerfile is *checked* — the
    `deploy` job builds it a second time with `flyctl deploy --local-only`,
    which ci.yml already says out loud. `deploy` still `needs` all four, and
    **as of this run a test says so.**
  - **Live probe (2026-08-19 23:33Z, before the day's last deploy).**
    `GET /` **200** in 389ms (TTFB 388ms, 1,897 bytes) · `GET /api/health`
    **200** in 258ms · `GET /api/decks` **401** in 233ms.
    `HEAD /` and `HEAD /api/health` still **405** — queued item 2 unchanged.
    Headers on a **GET** (Black's leg nearly filed a false regression off
    `curl -I`, so: GET): HSTS 31536000, `X-Content-Type-Options`,
    `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`, and
    `Permissions-Policy: camera=(self), microphone=(), geolocation=()` — #184
    is live. Health body: pool true, 35,390 oracle cards, 107,338 printings,
    7 decks, `pool_stale` false, bulk file still
    `oracle_cards-2026-08-13.jsonl.gz` — **six days old, and `pool_stale` is a
    schema check, not an age check** (unchanged note from 2026-08-18).
  - **Instance:** machine `84e19ef25041e8`, **version 120** (was 108 on
    2026-08-18 — 12 machine versions in a day, all deploys), `iad`,
    `shared-cpu-1x`/1 GB, 1/1 checks passing, `uname -m` **`x86_64`**. Black's
    deploy was watched live at 23:35: image pulled in 6.7s, machine created
    and started in 10.8s, health check failed at 23:35:21 and passed at
    23:35:29 — **the 8-second window is reproducible**, and Fly logs it as
    "Services exposed on ports [80, 443] will have intermittent failures",
    which is the per-merge downtime ADR 23 accepts, named by the platform.
    **No OOM, no unexplained restart, no 5xx in the log.**
  - **Volume:** 115 MB used of 2.9 GB (**5%**, 2.6 GB free) — `mtg.duckdb`
    76M, `scryfall` 24M, **`cache` 16M (was 4.4M** — the OCR shelf, the mana
    symbols and the card-motion derivatives), `decks` 304K, `app.db` 244K.
    **5 snapshots**, one per day, newest 9 hours old, 5-day retention, 447 MiB
    stored (was 370). Still no snapshot tied to a deploy (queued item 6).
  - **TLS expiry 2026-11-11** unchanged (Let's Encrypt, issued 2026-08-13,
    ~84 days remaining, apex-only SAN). `www.sylvan-libraries.com` still fails
    to connect — deferred item, trigger has not fired.
  - **`FLY_API_TOKEN` expires 2027-08-14 18:22Z**, which is exactly what
    `ci.yml`'s failure-path message prints. Checked because a runbook that
    names a date is a runbook that can drift off it; it has not.
  - **Alerting posture — unchanged from 2026-08-18 in every line that
    matters.** Fly HTTP check GET `/api/health`, passing, **does not restart
    on failure** · machine restart policy `on-failure`/10, fires only on
    process exit · Dockerfile `HEALTHCHECK` present but inert on Fly Machines
    · GitHub deploy-job failure email to the actor · **external uptime
    monitoring: none · phone alerting: none.** One delta:
    `FLY_METRICS_TOKEN` is set, so Fly's Prometheus is live — see the
    sharpening note on queued item 1.
  - **Scanner backlog, recorded here because an advisory scanner nobody is
    alerted about is an alerting fact.** 4 open CodeQL alerts, all
    `py/polynomial-redos` in `src/mtglab/decks/decklist.py`, **unchanged since
    2026-08-15** — that is White's item and is tracked in White's section; no
    channel exists that would have told anybody. 9 open Dependabot alerts (the
    torch pin cluster, unchanged). Secret scanning: off — queued item 8.
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

- **Last run:** 2026-08-19 (rainbow). Previous: 2026-08-16 (rainbow), which was
  the first Green run — so this is the first Green run with a baseline to move
  against, and everything below is read as a trend where one exists.
- **Fixed this run (2026-08-19, rainbow): two animations reach the browser
  that no reduced-motion guard could ever have arrested, and the reason they
  were missed is that the previous run checked the wrong file.** That run
  resolved 43 animation declarations against nine guard blocks and recorded
  the result as complete; three days later `web/src/index.css` had 111 and the
  *bundle* had 114. The two genuinely loose ones were never in that file at
  all: `animate-spin` on `Spinner` (`components/ui.tsx` — the shared spinner,
  so every waiting surface in the app: sims, every Claude mode, the camera
  door, imports) and `animate-pulse` on the lazy-chunk skeleton
  (`lib/deferred.tsx`). Both are **Tailwind utilities**, so they exist only in
  the built artifact. That is `test_browser_floor.py`'s lesson one facet over
  and in the same afternoon — **what a phone parses is the artifact, not the
  source** — and it is worth naming as a shape rather than an incident: *the
  correction a run makes in one facet is owed to its siblings in the same
  section.* Fixed in `web/src/index.css`: the spinner is **slowed** (1s →
  2.4s) rather than stopped, because a status indicator that has stopped
  turning says "this is broken", which is the one thing it must not say while
  the server is still working; the skeleton is stopped outright and rests on
  its inline opacity. **The cascade was measured, not assumed** — Tailwind
  puts utilities in `@layer utilities` and the guard is unlayered, so it
  should win, and it was proved by flipping the built block's condition to
  `no-preference`, loading the served sheet in a real browser and reading
  `animationDuration: 2.4s` / `animationName: none` off `getComputedStyle`.
- **Fixed this run — `tests/test_reduced_motion.py`, so the next one fails a
  test instead of a friend's inner ear.** It reads the *bundle*, resolves every
  animating rule against the guard blocks, and carries `COVERED_BY` for the two
  mechanisms a stylesheet cannot show: a base class on the same element
  (`.lab-bubble-2` beside `.lab-bubble`) and an ancestor the guard removes
  outright (`.wheel-spark` inside `.wheel-strike { display: none }`). The table
  is self-checking in both directions — a cover that is not guarded fails, and
  an entry for something that no longer animates fails, so it cannot rot into
  a list of excuses. Three tests, **all mutation-verified**: deleting the two
  new guards names them, misspelling a cover fails two tests at once, and a
  fabricated entry fails the third. Of 114 animating rules, 47 have no guard
  by class, 45 of those are covered by the two mechanisms, and the two that
  were not are the bug above.
- **Fixed this run — the admin cache tile named two shelves out of three, and
  the one it omitted was the newest and second-largest.** `Caches` rendered
  `motion … · symbols …` while `ocr.py`'s reading engine sat beside them at
  **6,050,615 bytes — 38% of the deployed cache** — present in the total and
  absent from the breakdown, so a line that reads as an itemisation was not
  one. (Locally it is starker: the tile named 5% of 226 MB.) The fix is not
  just a third row: `_cache_breakdown` now also reports **`other_bytes`**, the
  remainder the named shelves do not account for, because a fixed list of
  tenants can only ever miss the next one and a remainder cannot. Two tests,
  both mutation-verified — reverting to the two-tenant list, and folding the
  remainder to zero, each fail a different one — plus a rendering test that
  pins the empty shelf being dropped rather than printed as a dash. Verified
  on a real rendered surface, not in jsdom: `motion 11 MB · reader 5.8 MB ·
  symbols 23 kB · other 210 MB`.
- **Settled this run — the free reading engine loads behind the login, on the
  deployed instance.** This was the open question Black and White handed
  across, and it was spend-shaped: `/api/ocr` is not in `PUBLIC_PATHS` and
  answers **401** to a request with no cookie (re-confirmed by curl tonight,
  all three files), so if the Web Worker's own fetches did not carry the
  session, every camera scan would fall through to the paid Claude path and a
  100-card import would stop being a ≈$0.30 choice. White had proved a *page*
  fetch reaches it; the worker is a different request path. Driven on
  https://sylvan-libraries.com with a live session, in a Blob worker built
  exactly the way `tesseract.js`'s `spawnWorker` builds one: **`importScripts`
  ok, worker `fetch` of the wasm core 200, worker `fetch` of the traineddata
  200, synchronous `XMLHttpRequest` of the traineddata 200** — that last one
  because Emscripten's FS layer is the path a `fetch` test would have missed.
  Then end to end from the committed bundle's own `reader.js`: the real
  engine created, **616 ms to load**, and a synthetic corner recognised as
  `LTC 0242` at 91% confidence in 1.1 s. Reproduced first locally against a
  server with `MTGLAB_REQUIRE_AUTH=1`, which answers 401 to the same URL
  without a cookie. **No change needed, and the question is closed.**
  One trap found on the way and worth keeping: a first attempt used the raw
  `'/api/ocr/worker.min.js'` and failed with *"The URL … is invalid"* — a
  path-absolute reference **cannot be resolved against a `blob:` base**.
  tesseract.js's `resolvePaths` absolutises against `window.location.href`
  before the blob is built, so the app is fine; a hand-rolled worker would
  not be.
- **Also settled: the paid path cannot be reached by accident.**
  `askClaude` in `components/camera.tsx` is per-card and driven by a button
  with the sentence beside it — there is no automatic fallback from a failed
  local read, so a broken engine costs a person a click, never a charge.
- **Checklist note for `references/green.md`:** the browser-compat facet was
  corrected on 2026-08-16 to say "audit the artifact, run the test" — and the
  *motion accessibility* bullet three lines below it still reads as a
  judgement call ("Animations should respect it"). It should name
  `tests/test_reduced_motion.py` the way the bullet above names
  `test_browser_floor.py`, for exactly the reason this run found: the hand
  version of that check was performed correctly, in the wrong file, by the run
  that wrote the correction.
- **Queued for Aaron:**
  1. **The stated compatibility floor is Safari 15; the shipped bundle needs
     Safari 16.4.** Unchanged and still Aaron's decision. Tailwind v4 emits 56
     `@property` rules and 10 `color-mix(in lab, …)` values (both 16.4); React
     adds `Object.hasOwn`, `structuredClone` and `reportError` (15.4). What it
     costs below the floor is quiet rather than fatal: the `--tw-*` variables
     go unregistered, so shadows, transforms, filters and rings drop while the
     layout stays correct. **Safari 15 on macOS 12 is this dev machine's own
     browser**, so the severity question is a ten-second check on a browser
     Aaron already has, and it still has not been made. Above the floor is
     verified: the Playwright rig pinned at 1.45.3 (**WebKit 17.4**, the newest
     macOS 12 will ever run) loads the deployed site clean at an iPhone 13
     viewport.
     **New this run, and it bears on which way to resolve it:** the camera door
     requires 16.4 by a *second, independent* route. `lib/reader.ts` pins
     `corePath` to `tesseract-core-simd-lstm.wasm.js`, and **WebAssembly SIMD
     is Safari 16.4**. So "get back to 15" would have to answer the camera door
     separately — either the non-SIMD core (slower, on the tier that is already
     8× slower than Claude) or the door refusing on old phones. Recorded as an
     input to the decision, not as an argument either way.

     **CLOSED 2026-08-19 (stragglers, PR #194). Aaron's ruling: declare 16.4
     and document it.** Both routes re-verified against the artifacts rather
     than against this entry, and the counts had already moved — the committed
     stylesheet carries **53** `@property` rules and **17** `color-mix(in lab`
     values today, not the 56 and 10 recorded above, which is the argument for
     re-measuring a count rather than quoting one. The second route holds:
     `lib/reader.ts` still asks for `/api/ocr/tesseract-core-simd-lstm.wasm.js`
     and `ocr.ASSETS` has no non-SIMD sibling to fall back to.
     Written down **once**, in `tests/test_browser_floor.py`'s docstring, with
     `web/README.md` and `references/green.md` pointing at it rather than
     restating it — three runs derived these same two facts and left the number
     open, which is what a decision recorded in a ledger and nowhere else
     costs. `references/blue.md`'s lookbehind bullet lost its "Safari 15 is a
     real user" justification, which had been the wrong reason for a right
     rule since Tailwind v4 landed.
     **The second route is now pinned, because it is the removable one.**
     `test_the_camera_door_still_holds_the_floor_independently` reads the core
     name off `ocr.ASSETS` and checks `reader.ts` asks for that same file; a
     swap to the plain core is a one-word edit in two files that would silently
     leave Tailwind holding the floor alone. Mutation-verified. The tripwire
     cannot reach the served engine itself — it is fetched at run time and git
     holds none of it — so the names are what there is to hold.
     **One consequence to carry forward:** Safari 15.6 on macOS 12 is now
     *below* the declared floor, so this dev machine's own browser has stopped
     being a witness for it and the Playwright/WebKit 17.4 rig is the only one
     on this hardware.
  2. **Five of eight nav destinations were unreachable on a phone.**
     **Landed 2026-08-16** — the strip wraps below `lg`, all eight visible at
     375px, and the same branch fixed the failed lazy chunk that unmounted the
     React root. Re-verified this run: the nav renders three short lines at
     375px and no route has page-level horizontal overflow. Kept only because
     it is the case that settled the design question.
  3. **Touch targets under 44px, and it is the whole app rather than the deck
     page.** Re-measured this run on `/import` at 375px: **21 of 23**
     interactive elements under 44px. Nav links are 32px, the settings gear
     28px, "Back to the library" **17px**. The 2026-08-16 reading was 27 of 33
     on the deck page; this run's contribution is that the pattern is not
     local to that page, so the fix is a spacing-scale decision (or a
     pseudo-element hit area that moves no pixels), which is why it stays
     queued.
  4. **`env(safe-area-inset-*)` appears nowhere, and adding it alone would be a
     no-op.** Unchanged. `.library-whisper` (fixed `bottom: 1.25rem`) sits in
     the home-indicator band on a notched iPhone; the fix is *two* coupled
     changes — `viewport-fit=cover` on the viewport meta **and** `env()` insets
     — because iOS reports every inset as 0 without the former, and
     `viewport-fit=cover` changes what the page does under the notch, so it
     wants Aaron's physical phone.
  5. **The admin resource panel — mostly built, and here is precisely what is
     left.** Green queued this on 2026-08-16 as a proposal; #165/#170 built
     most of it, so this run closes the built half rather than re-litigating
     it. **Already there and read this run through a live admin session:**
     `/api/admin/stats/system` (volume total/used/free with the mount named,
     process RSS, machine memory, load, cpus), `/api/admin/stats/storage`
     (`app.db`, pool, bulk files, the cache and now all three of its shelves
     plus the remainder, decks and trashed), `/api/admin/stats/activity`
     (`jobs.census()`, so registry occupancy is covered),
     `/api/admin/stats/fly` (memory and the edge counters off Fly's own
     Prometheus). All admin-mounted, so the prefix middleware refuses them
     before routing and ADR 17 holds. **What remains is two numbers:**
     (a) **`SCHEMA_VERSION` is surfaced nowhere** — not on the panel, not in
     `/api/health` — and it is at **10** now, up from 8 at the last Green run.
     That is the number ADR 23 makes most worth seeing: migrations apply on
     boot, unwatched and forward-only, and there is currently no way to see
     what version the volume's `app.db` actually reached except an ssh. Recipe:
     one field on `/api/admin/stats/system`, one tile on the Machine tab beside
     Process memory. Six lines, and deliberately not taken this run because it
     would be a third user-visible change in a pass already owing an eye-walk.

     **Landed 2026-08-19 (stragglers), PR #192.** A `schema` field on
     `/api/admin/stats/system` and a Schema tile on the Machine row. Two
     numbers rather than the one the recipe called for: `applied` is read off
     the file, `expected` off the code running here, because **a lone version
     number cannot be wrong** and the mismatch is the only state worth an
     alarm. The read is a `mode=ro` connection rather than `db.connect` --
     that one *applies* migrations, so the recipe's obvious implementation
     would have let an admin upgrade the volume by refreshing a stats tab.
     Mutation testing earned its place twice here: it killed a first draft
     whose `expected` was a literal that happened to equal the constant (the
     restated-claim shape, in the code this time rather than the test), and
     it falsified a docstring that credited the wrong guard for not
     conjuring a database.

     **And then it bit, which is the part worth keeping.** The mutation that
     proves this guard replaces the read-only connection with `db.connect` --
     the exact wrongness the guard exists to catch -- and the test that
     carries it had moved `SCHEMA_VERSION` to 4242 without a `use_paths`
     sandbox. So the mutation run migrated the **maintainer's own `app.db`**
     to version 4242, where `_apply_migrations` returns early and would have
     silently skipped every future migration. Found by Aaron's eye on the
     tile itself -- the number rendered `v4242 / code here expects v10`,
     which is the feature working. Repaired to 10, data intact
     (`integrity_check ok`, one user), and the test now runs in `tmp_path`;
     the same three mutations afterwards leave the file byte-identical.

     Three things this argues, none of them about schema versions.
     **`mtglab mutate` applies its wrongnesses to a throwaway copy of the
     package for exactly this reason, and a hand-rolled `sed`-and-run loop
     has no such copy** -- the harness's own safety was reasoned about and
     the ad-hoc version inherited none of it. **A test that installs a fake
     constant must be sandboxed even when it only reads**, because the
     mutation that checks it is not required to only read. And **a
     destructive test failure reads exactly like a successful kill**: the
     run failed, which is what "killed" means, so the damage was recorded as
     a pass and only surfaced two steps later on a screen.
     (b) **Newest snapshot age**, which needs a Fly API call the app does not
     make today and is therefore a real design decision rather than a field.
     Red's queued item 6 (a snapshot taken *at* deploy) is the sharper half of
     the same worry.
- **Deferred:**
  - **The shelf's serialization** (`/api/decks`). Still real and still the
    first bottleneck — but the arithmetic under it moved by an order of
    magnitude and the trigger has to move with it. At 10 concurrent the last
    user now waits **134 ms**, not 1,821 ms; at 30, **388 ms**, not 5,458 ms.
    The *shape* is unchanged — 10 concurrent requests take 9.3× one request,
    which is still essentially perfect serialization — so what #181 bought was
    a 15× smaller constant rather than a different curve. **Profiled rather
    than guessed, which is this facet's own 2026-08-19 correction applied to
    itself:** `mtglab bench profile "GET /api/decks"` puts `list_library`
    first and `copy.deepcopy` second (13.9 ms of a 16.6 ms wall by cProfile's
    inflated clock, 117,550 dict lookups and 71,450 `id()` calls — deepcopy's
    memo bookkeeping), which is `Deck.load` handing out a **copy** of its
    parse cache on every hit. That copy is deliberate and argued in
    `decks/model.py`: the cached `Deck` is mutable and callers write to it, and
    copying is 5.2 ms where parsing is 18.0 ms. Nothing to fix; the cause is
    now named instead of assumed. **Trigger, restated:** the design point
    moves past ~150 concurrent, or the deck count grows past ~40 (the cost is
    still linear in decks).
  - **Widening the `NET` job pool past 2.** Unchanged — it is a deliberate
    spend guard, not a throughput setting. **Trigger:** a real per-user quota
    on the Claude surfaces exists, at which point the queue stops being the
    cost control and can widen. Black's dated note sharpens why that matters:
    Sonnet 5 introductory pricing closes **2026-08-31**, after which the same
    traffic costs ~50% more.
- **Measurements (2026-08-19, rainbow):**
  - **The shelf, warm — the trend that moved.** Local, one uvicorn worker,
    machine load 4.6/8 cores (noisier than 2026-08-16's 3.0/8, which makes
    these numbers conservative rather than flattering). Serial medians of 5,
    against 2026-08-16: `/api/health` **5.9 ms** (was 73) · `/api/decks`
    **14.4 ms** (was 224) · `/api/decks/local/arahbo-cats` **7.5 ms**
    (was 117) · `/api/colors` **2.6 ms** (was 3) · `/api/glossary` **2.0 ms**
    (was 3) · `/` **2.5 ms** (was 4). Concurrent on `/api/decks`: at n=10 wall
    **134 ms**, median 106 ms (was 1,821 / 1,513); at n=30 wall **388 ms**
    (was 5,458). Deck detail at n=10 is 50 ms wall. **Zero errors at every
    level** — no 500s, no timeouts, no SQLite lock failures. That is #181 and
    #187, and it is a 13.6× improvement in what the tenth concurrent visitor
    waits.
  - **Warm and cold as two numbers** (`mtglab bench run`, then `--cold`,
    12 targets × 12 runs; medians):
    `/api/health` 7.1 / **36.0 ms** · `/api/decks` 16.5 / **134.8 ms** ·
    `/api/lore` 6.1 / 78.2 ms · `/api/colors` 5.8 / 6.2 ms · `/api/glossary`
    4.8 / 4.8 ms · `/api/cards/search?q=goblin` 44.7 / 66.1 ms ·
    deck detail 7.5 / 80.9 ms · deck validate 6.2 / 77.1 ms ·
    `db.get_cards (100)` 0.2 / 80.5 ms · `db.oracle_columns` 0.2 / 29.2 ms ·
    `db.connect_readonly` 0.2 / 15.4 ms · `Deck.load` 0.0 / 0.2 ms. The two
    columns are the memo: the reference-prose routes barely move, everything
    touching the pool moves by two orders of magnitude. The only warm target
    over 25 ms is card search, and it is **database-bound** (35.0 ms of 42.5
    inside one statement) — Black's ground, not a Python lever.
  - **Reduced-motion coverage, counted rather than recalled:** 114 animating
    rules in the bundle, 12 guard blocks, 87 guarded classes. 47 rules have no
    guard by class; 45 are covered by a base class on the same element or a
    `display: none` ancestor (each read out of the component that renders it,
    now recorded in `COVERED_BY`); 2 were loose and are fixed. On 2026-08-16
    the same count over `web/src/index.css` alone was 43.
  - **The browser floor holds at Safari 16.4**, unchanged across the 81 pull
    requests merged since 2026-08-16 (#107–#188). `tests/test_browser_floor.py` green; the floor is
    still set by `@property` and `color-mix(in lab`, no new feature crossed it,
    and no regex lookbehind reached the bundle. **Declared rather than merely
    held as of 2026-08-19** (stragglers, PR #194) — Aaron's ruling, the
    argument in that file's docstring, and the counts re-measured off the
    committed stylesheet: **53 `@property`, 17 `color-mix(in lab`, 2
    `color-mix(in oklab`.**
  - **The OCR shelf's served JavaScript, scanned for the first time.** It is
    the newest third-party code the project actually redistributes and it is
    *not* in `web_dist`, so the floor tripwire cannot see it (and cannot be
    made to in CI — the files are fetched at runtime and git holds none of
    them). Scanned by hand this run: 4,065,876 bytes of `worker.min.js` plus
    `tesseract-core-simd-lstm.wasm.js`, **zero features above the floor, zero
    lookbehind**. The one coupling it does carry is the SIMD core — see queued
    item 1.
  - **Responsive sweep at 375×812, nine routes** (`/`, `/claude`, `/learn`,
    `/admin`, `/research`, `/cards`, `/simulator`, a deck page, `/new`):
    **no page-level horizontal overflow anywhere** — `scrollWidth` equals
    `clientWidth` equals 375 on every one. One element scrolls inside its own
    container: the Admin accounts table, **325 px visible of 776 px (42%)**,
    and unlike the nav strip that became queued item 2 its scrollbar is *not*
    suppressed (`scrollbar-width: auto`, a 12 px gutter). Recorded rather than
    filed: a wide data table on an admin page is the accepted pattern, and the
    thing that made the nav strip a bug — a newcomer unable to find Learn —
    does not apply.
  - **Volume: 115 MB of 2.9 GB (5%), 2.6 GB free** — unchanged across the
    v122 deploy, and unchanged from Red's reading three hours earlier. Read
    two ways this run, which is the useful part: `df` over ssh, and the app's
    own `/api/admin/stats/system`, which agrees (120,094,720 used of
    3,077,595,136). **Cache breakdown, from the admin endpoint:** 15,908,250 B
    total = cardmotion 9,808,398 + **ocr 6,050,615** + symbols 49,237, with
    nothing else. `app.db` 249,856 B (was 212K on 2026-08-16), pool 78,655,488,
    bulk 24,528,727, decks 8 directories / 208,665 B, 0 trashed.
  - **Machine memory, readable for the first time.** 2026-08-16 could not
    report it (`free` is not in the image); the admin panel now answers it
    without an ssh. **285,650,944 of 1,008,824,320 — 28.3%**, off Fly's own
    Prometheus, with the app process at 146,911,232 B RSS. Load 0.00 across
    all three windows, 1 vCPU. Nowhere near the ~85% Red would alert on.
  - **Edge, 24 h: 653 2xx, 18 4xx, 0 5xx** (`/api/admin/stats/fly`). The
    witness Red built means that zero is a real zero rather than a broken
    query.
  - **Live probe, 2026-08-20 00:36Z, machine v122** (post-deploy, GET
    throughout — HEAD still answers 405, Red's queued item 2):
    `GET /` **200** in 459 ms · `GET /api/health` **200** in 238 ms ·
    `GET /api/decks` **401** in 218 ms. `/assets/app.js` serves
    `content-encoding: gzip` with `vary: Accept-Encoding`. `/api/ocr/*` answers
    **401** without a cookie, all three files.
  - **What one camera user costs, which is the first per-user bandwidth
    number this project has had.** The reading engine is **5.8 MB**, served
    `Cache-Control: public, max-age=31536000, immutable`, so it crosses the
    wire once per browser and never again. Against 653 edge responses in 24 h,
    one new camera user is worth roughly sixty page loads. Nothing is near a
    limit; recorded because it is the first thing here whose cost scales with
    what a person *does* rather than with how many people there are.
  - **The bill, itemised — list rates, not an invoice** (nothing in `flyctl`
    reads billing, so these are the published rates and the repo's own
    measured figure, and should be checked against a real statement once):
    machine `shared-cpu-1x`/1 GB held awake ≈ **$0.19/day ≈ $5.70/mo**
    (`fly.toml`'s own recorded number, and the block explains why it is held);
    volume 3 GB at $0.15/GB/mo ≈ **$0.45/mo**; bandwidth at this volume ≈ **$0**;
    Anthropic ≈ **$1.64/mo**, rising to ≈**$2.46/mo from 2026-09-01** when
    Sonnet 5's introductory pricing closes (Black). **Total ≈ $7.8/mo today,
    ≈ $8.6/mo in September.** The largest single line is the held-awake
    machine, and `fly.toml` already carries the commented-out block that
    removes most of it — the trigger for uncommenting it is "primary
    development is done", which is Aaron's call and not a calendar.
  - **Upgrade scan.** Image and both CI legs are on **Python 3.12**
    (`python:3.12-slim`, matrix 3.11/3.12) with `requires-python >= 3.11`;
    3.13 and 3.14 exist and moving the base image is a queued-class change by
    the protocol's own rule, since only the `image` job can prove it and that
    job cannot run on this Mac. Dependabot: **9 open alerts, all `torch`, all
    severity low** — the dev-Mac-only `depth` pin that never enters the
    container — unchanged since 2026-08-18. No other dependency major is
    parked. Fly machine generation and volume shape unchanged; the
    Fly-vs-Hetzner swap in HOSTING §7 still has its trigger unfired (Forge
    going server-side).
  - **Where the design point lives in code — verified unchanged**
    (100 accounts / 10 concurrent): `jobs.py` `CPU=1`, `NET=2`,
    `MAX_JOBS=200`; `auth/ratelimit.py` `PER_ACCOUNT` 10/15min, `PER_ADDRESS`
    30/15min, `RESET_PER_MAILBOX` 3/hr, `RESET_PER_ADDRESS` 10/hr,
    `CLAIM_PER_ADDRESS` 20/15min; `fly.toml` `soft_limit=20`/`hard_limit=40`;
    one uvicorn worker. SQLite posture also unchanged and still correct:
    `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`, deferred
    isolation. **Adaptability verdict: 500/50 is now a config edit and a
    re-measure for everything**, including the shelf — at 30 concurrent it
    answers in 388 ms, so the one exception the 2026-08-16 run had to carve
    out has closed on its own.
  - **Local suite:** 2,426 passed, 0 skipped (this machine has the pool, so
    both `needs_full_pool` tests run; CI's pinned 2 skips are a property of
    CI). `data/app.db` byte-identical before and after the run —
    checked by digest, not by `git status`. The probe server *did* write to it
    while running, which is expected and is why it was stopped first.
- **Measurements and findings (2026-08-16, the first Green run):**
  - **`prefers-reduced-motion` gap closed:** `.ready-banner` ran `bubble-in`
    unguarded; the other apparent misses were covered by a base class the
    element also carries. **Source order is what makes those work**, not
    specificity — verified against the *served* stylesheet's rule indices in a
    real browser (haze declared at rules 63–65, guarded at 91). The claim that
    this covered *every* animation was true of `web/src/index.css` and false of
    the bundle; see this run's first entry.
  - **`tests/test_browser_floor.py` landed**, making the floor a declared value
    instead of a remembered one, and finding that it had moved to 16.4. The
    checklist conflict it exposed — grep `web/src` for `(?<`, which has a
    false positive on named capture groups and looks in a file a phone never
    parses — has since been corrected in `references/green.md`.
  - Volume **99M of 2.9G (4%)**; `mtg.duckdb` 76M, `/data/scryfall` 24M,
    `app.db` 212K, `/data/decks` 304K. Machine `shared-cpu-1x`/1 GB in `iad`;
    in-container memory **not readable** (`free` absent from the image); no
    OOM, no restarts. Snapshots newer than the newest migration
    (`SCHEMA_VERSION` 8), 5-day retention.
  - Serial: `/api/health` 73ms, **`/api/decks` 224ms**, deck detail 117ms,
    `/api/colors` 3ms, `/api/glossary` 3ms, `/` 4ms. At 10 concurrent
    `/api/decks` wall **1821ms**, median 1513ms — 6.8× serial. At 30
    concurrent, 5458ms. Zero errors at every level. **The cause recorded here
    was a guess and was wrong** — "the shelf's pure-Python YAML and
    aggregation under the GIL"; the first cost was 162ms of failed
    `import pandas` inside DuckDB's parameter binding, which is why
    `references/green.md` now says to profile rather than to name a cause.
  - Responsive sweep at 375 (light and dark), 768, 1024, 1280 (light and
    dark): no horizontal overflow on any route at any width; the 3px at 375 is
    the emulator's classic scrollbar, not an overflow. Theme resolution
    follows `prefers-color-scheme` with no stored preference and persists the
    resolved value.


### The lease that never came due — 2026-08-19 (stragglers)

Filed under Green because it is a **cloud-resource and deployed-behaviour**
finding, not a code-craft one: nothing about the app on a laptop was wrong.

**`mtglab data refresh` could not run on the instance at all.** `data refresh`
needs DuckDB's exclusive lock; the app holds a shared one through the pool
keeper, whose lease is `service._KEEPER_IDLE`. That was **30.0s against a 30s
`/api/health` check** — and `service.health` opens the pool (it counts both
tables and calls `pool_stale`). The lease was renewed by the one caller that
never stops asking, exactly as often as it expired. Forty consecutive attempts
over five minutes were refused, every one at `connect`, every one naming the
same holder.

`_reap_keeper`'s own docstring argues that the keeper *must not* lock a refresh
out — "that is a worse bug than the 17.5ms `_pin` exists to save". **The
argument was right and the constant under it was never checked against the
platform.** A number is not protected by the paragraph above it.

Fixed at 10.0s, which still spans the burst the lease exists for (a page load
is four requests) and leaves two thirds of every check cycle free.
`tests/test_pool_keeper.py` **derives** the ceiling from `fly.toml` rather than
restating 30s, so moving the check's interval fails there instead of silently
re-closing the door; it also pins the floor, so nobody shrinks the lease past
the burst it was bought for, and pins that `health` still opens the pool, since
that coupling is what makes the interval a ceiling at all. Mutation-verified
four ways including the production value.

**Two things this cost, both worth carrying.** The runbook's step 6 had
**never worked on a populated volume** — the single run that verified it was
the *first* load, when no pool file existed to lock. A step verified once on
an empty volume is verified in the one state you will never be in again.
And a failed `fly ssh` job reported **exit code 0** twice, because the
`| tail` pipeline returned its own status; the same shape as "a destructive
test failure reads exactly like a successful kill", one layer out.

Still open after this: the volume's pool is stale until the refresh actually
runs, so three decks show a set name with no painter. That is #195's designed
degradation, not a new fault.

## Colorless — The Artifacts

*The pass auditing itself: last cycle's findings · are the checklists still
finding things · the developer tooling · cross-color leftovers*

- **Last run:** 2026-08-19 (rainbow) — the first colorless run to have five
  colors to audit. Previous: 2026-08-19 (the run that created this section).
- **Fixed this run (2026-08-19, rainbow):**
  1. **The mutation catalogue held 19 sites no test could ever kill, and they
     were 22% of one whole class.** White handed this over as an observation
     about two survivors; measured, it is a category. `@dataclass(frozen=True)`
     is a *declaration*, read by `dataclass` at import and by no code path
     afterwards, so flipping it changes nothing any assertion reaches — 19 such
     sites across the 18 declared modules, every one `frozen=True`, and **22%
     of the 86 `constant` sites**. That is not an equivalent mutant (a fact
     about one line, worth recording once) but a **shape the repo keeps
     adding**, so it compounds: a floor of guaranteed survivors dragging every
     kill rate down for a reason that says nothing about the suite.
     `operators._decorator_flags` excludes them — **1,279 → 1,260, `constant`
     86 → 67** — deliberately narrow, booleans on a decorator's keyword
     arguments only, because `@lru_cache(maxsize=16)` is a *boundary* on a
     number a test can absolutely notice and stays in. Three tests, all
     mutation-verified: dropping the exclusion fails two, widening it to every
     boolean fails a third, and the third reads the real catalogue rather than
     the fixture so the two halves cannot drift apart in silence.
  2. **`mtglab mutate run --only <path:line>`, because a recorded survivor had
     no verb.** Every run writes down the survivors it read, and the obvious
     next question — *is that one still alive?* — could only be asked by
     drawing a fresh random sample that will never revisit a named site. So
     survivors accumulated as prose nobody could cheaply re-check, and two went
     two runs unread. First use found four still alive (White's section has
     them) and **a bug in the new flag**: `--only decks/analyze.py:33` also
     swept in line 336, because the first draft matched `relpath:line` as one
     string. A path fragment is still a substring; the number is compared as a
     number. A pattern matching nothing **raises** rather than reporting a
     flawless kill rate over no mutants — the harness's own shipped bug (a
     mistyped test filename silently *widening* what ran) pointed the other
     way. Both mutation-verified.
  3. **`bench profile` told runs that zero warm imports was the only right
     answer, in three places, while its own threshold said 200 and its own
     output said 7–31.** Both cannot be true and the threshold is the half that
     behaves: #181 did not *remove* DuckDB's per-parameter `import pandas`
     probe, it **answered** it with a `sys.modules` sentinel, so a warm bind
     still enters the import machinery and still counts — it simply stopped
     walking `sys.path`. Traced rather than reasoned: a warm `/api/decks` makes
     25 `__import__` calls a request, six of them `pandas`, all resolving from
     `sys.modules`; a defused three-parameter bind reports **exactly 6**,
     deterministically, two per bound value. The three sentences now say to
     read the count against `IMPORT_CALLS_SUSPECT`, and
     `test_a_warm_request_imports_something_and_that_is_not_the_alarm` pins the
     band **from below** — the direction nobody was watching, and the one where
     a later "fix" makes every healthy profile read as a finding.
     Mutation-verified by tightening the threshold to 0. *An instrument that
     cries wolf is one nobody reads, and its numbers are still in the ledger.*
  4. **Four checklist corrections and one hand-off honoured.**
     `references/green.md`'s motion bullet was still a judgement call
     ("Animations should respect it") three lines under the correction that
     says to audit the artifact — Green filed the note rather than applying it
     and named Colorless as the owner; it names `tests/test_reduced_motion.py`
     now, with the reason. `references/black.md` said **six** Claude modes
     through the very run that measured seven. `references/white.md` hard-coded
     a catalogue size that had already moved twice in a week. `references/red.md`
     carried the skip gate as "2" without Red's own correction that **the local
     suite skips 0** and 2 is CI's number by construction; it also now names
     the deploy-`needs` test Red landed.
- **Promoted into `SKILL.md`, where every run reads it** (both patterns were
  found independently by more than one color this rainbow, which is the test
  for promotion):
  - **"A rule enforced by nothing drifts"** was already written down — in
    `references/colorless.md`, the one file only the sixth run opens. Blue,
    Red and Green each rediscovered it in the same day (an extras list, a
    `needs` list, a motion guard), and White found a fifth (a stated rule with
    no pin at `sim/compile.py:102`). It is now a standing question in step 2 of
    the run protocol. The colorless reference keeps the half that is genuinely
    its own: sweeping *the skill's* absolutes, and the corollary that a guard
    needs a test proving it is not inert.
  - **"Derive the expectation; never restate the claim."** Blue found the
    sharp case (`expect(getByText(/in git, not here/))` sat green beside a
    sentence that was false twice over) and White found the near-miss (a
    licence-notice test whose first draft supplied its own shelf entry). Red's
    deploy test and Blue's extras test are the working form, and both say so in
    their own docstrings. Now in step 4 beside "every bug fix gets a test",
    with mutation verification as the second beat.
- **What changed in the skill on the previous run** (Aaron's ask, in a clean
  session after the targeted performance pass exposed seven structural gaps):
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
- **Queued for Aaron:** nothing new from this run. `mutmut`/`cosmic-ray`
  remain queued in White's section as the exhaustive-run escalation. What this
  run owes instead is the list below.

### Every straggler, deduplicated and verified — 2026-08-19

Aaron's ruling this rainbow was *"let's fix all stragglers at the end"*, and a
stragglers pass follows this run. This is its input: **22 items open across
five sections — White 2, Blue 6, Black 2, Red 8, Green 4, Colorless 0 — each
re-checked against the tree rather than copied forward**, because three
separate colors this cycle found a carried item that had already landed and
one of those was three days old. Sections own their own entries; this is an
index with a verdict, not a second copy.

**Verified already landed, and marked so in their own sections** (do not
re-open):

| item | landed |
|---|---|
| White · package licence in `pyproject.toml` | #135, 2026-08-16 |
| White · widen CI's card-data filename scan | Red, 2026-08-16 |
| White · CLAUDE.md's Setup block names a missing interpreter | Blue, #186 |
| **White · parameterize `snapshot_prices`** (deferred) | **#135 — found by this run; it had been re-affirmed verbatim three days after it was fixed** |
| Blue · Claude Code hooks | #135 (the `PostToolUse` third is still open) |
| Blue · `api/service.py` reaching past `cards/db.py` | #181 |
| Green · five of eight nav destinations unreachable on a phone | 2026-08-16 |

**What the stragglers pass has closed so far**, tagged in each item below and
in its own colour's section: Blue 2 (#191), Green 5a (#192), Red 7 (#193), and
the four repository settings (Red 4 + Red 8, done through the API rather than
a PR). Then — bundled as one branch on Aaron's instruction, PR #194 —
**White 4** (coverage), **Blue 3** (`cli.py`), **Green 1** (the Safari floor),
half of **Blue 7** (ROADMAP), and an *answer* rather than a fix for **White 1**
(the ReDoS alerts, which turn out to be unfixable-by-bounding; see the entry).
That is **nine of the twenty-two** the run opened with.

**Still open — Aaron's ruling wanted.** Ordered by what a fresh session can
act on soonest, not by color:

1. **Red 4 + Red 8 — four repository settings, all free, all pure win, all
   Aaron's to flip** (a run cannot). Re-read from the API tonight:
   `sha_pinning_required: false`, `allowed_actions: "all"`, secret scanning
   **disabled**, push protection **disabled**. The repo already pins all
   eleven actions by hand, so the first makes a convention structural; the
   second and third matter because `no-secrets-or-card-data`'s grep is
   *post-hoc by construction* — it runs after the push, on a public repo, so
   when it fails the key is already published. Anthropic has been a GitHub
   secret-scanning partner since 2024, so a leaked key is forwarded and
   revoked with nobody in the loop. **These four are the cheapest yes in the
   whole list and should be first.**
2. **Red 1 + Red 3 — nothing tells Aaron the site is down.** One question, two
   halves, and the cheapest first step is **free**: does `fly-metrics.net`
   have any alert rule at all? (`FLY_METRICS_TOKEN` is set, so Fly's
   Prometheus is live and the admin page already reads it.) Then the
   off-platform half, which cannot live on the platform it watches:
   UptimeRobot free + Pushover ($5 once). Red 3 is its precondition —
   `/api/health` must report sickness (`app_db` opens, `disk_free_mb`,
   `schema_version`) **while still answering 200**, because Fly stops routing
   on a failing check and with one machine that turns "logins are broken" into
   "the site is down". **Red 2 rides along and is not optional:** `HEAD /` and
   `HEAD /api/health` answer **405**, so a monitor left on a HEAD default
   alerts continuously against a healthy site.
3. **Blue 2 — the simulator renders a raw seed, and commandment 10 names seeds
   explicitly.** The *whether* is settled; the *what* is two readings.
   (a) flavour the label only — `Seed` → `Shuffle`, wire field untouched, the
   `lib/claudecopy.ts` pattern; (b) stop showing the number, keeping "New
   sample" as the only control. (a) preserves ADR 18's reproducibility
   surface; (b) is the stricter reading. **Commandment 16 either way.**
   **CLOSED 2026-08-19 (stragglers, PR #191): reading (a), plus a fourth
   render the audit had missed and a tripwire so there is no fifth.**
4. **Blue 1 — three of six decks credit the wrong painter right now.** A deck
   that pins a printing gets that printing's `set_name` and the *oracle* row's
   `artist`, side by side in one sentence, and the code's own comment states
   the rule it breaks. Not fixable without a pool change (`printings` has no
   `artist` or `flavor_text` column), so it is **schema + a mandatory
   `data refresh`** — which is why it is Aaron's. The stopgap trades a wrong
   credit for a missing one, against a project standard that every painting
   carries a visible credit. *Cross-color: White's attribution facet, Black's
   ingest.*
   **CLOSED 2026-08-19 (PR #195): the schema, not the stopgap.** Aaron's
   ruling. Blue's section has the full account; the operational half is that
   **every instance owes a `data refresh`** before the credit comes back, and
   `/api/health` says `pool_stale` until it does.
5. **Black 1 — cache-write tokens are invisible and are the priciest class**
   (1.25× input). Re-verified: `cache_creation_input_tokens` appears **nowhere
   in `src/`**. One column plus one assignment, but a **schema migration**
   (now v11) on a forward-only ladder that applies on boot unwatched — so it
   wants its own branch merged while somebody is watching. **Dated urgency:**
   Sonnet 5's introductory rate ends **2026-08-31**, and this is the
   instrument that would show what the 50% rise actually costs.
6. **Green 5a — `SCHEMA_VERSION` is surfaced nowhere.** Re-grepped: not in
   `/api/health`, not on the admin panel, and it is at 10. Six lines, one
   field and one tile — and it is precisely the number ADR 23 makes worth
   seeing, since migrations apply on boot with nobody watching. Pairs with (5)
   and with Red 6. **CLOSED 2026-08-19 (stragglers, PR #192), as a pair —
   applied and expected — since one number cannot disagree with anything.** *(Green 5b — newest snapshot age — needs a Fly API call the
   app does not make, and Red 6 is the sharper half of the same worry.)*
7. **Red 6 — a deploy takes no snapshot, which is exactly backwards.** The
   volume is most at risk on the boot after a merge; Fly's snapshots are daily
   and unrelated to that. Needs a `FLY_API_TOKEN` scope check and a decision
   about failing the deploy when the snapshot fails.
8. **Red 7 — the `image` job spends 60% of itself emulating an architecture
   nothing deploys to.** 182s of arm64 QEMU on a job whose live machine
   answers `x86_64`. Three options written out: drop it, move it to the now-free
   `ubuntu-24.04-arm` runner, or keep it. **The only lever in six runs that
   would move CI's wall clock**, and the Dockerfile's own comment is the
   decision it was added under, which is why it is not a safe fix.

   **CLOSED 2026-08-19 (stragglers, PR #193). Aaron chose the native runner.**
   `image` is amd64-only and keeps every check that needs a loaded image;
   `image-arm64` builds `linux/arm64` on `ubuntu-24.04-arm` and does nothing
   else. Cache scopes are now named per architecture — left at the default
   the two jobs share one and each evicts the other's layers every run, which
   is a cache that costs storage and returns nothing.

   **Measured on the first run: `image` 302s → 79s, `image-arm64` 59s, the two
   in parallel.** Recorded because the prediction written here before the run
   was *wrong in the good direction* — it warned the first build would be
   slower than steady state, the new scopes being cold. QEMU turned out to
   dominate so completely that a cold native build beats a warm emulated one;
   the cold-scope effect was real and an order of magnitude too small to see.
   `image` is no longer the critical path at all: **`test (3.12)` at 4m31s is,
   unambiguously**, which retires the coin flip Red deferred this item on
   through six runs and points the next CI-time question at the Python suite.

   **`deploy` needs five jobs now**, which `tests/test_packaging.py` enforced
   the moment the job was added — #188's guard earning itself back within a
   day, and mutation-verified again here by unwiring the new job.

   **`image-arm64` became a required check the same day**, after the merge —
   Aaron's call, applied through the API and read back: **seven contexts now**,
   with admin enforcement, strict and linear history all confirmed unchanged.
   It was landed *without* being required, and the PR and four doc files said
   so plainly rather than describing the list they were about to ask for,
   because ENGINEERING §5's own scar is a table that claimed a check was
   required two days before it was. Adding it necessarily comes second: the
   context has to exist before it can be demanded.
9. **White 4 — the coverage floor is a tripwire, not a floor.** 95.136%
   against `fail_under = 95`, ~16 statements of headroom on 11,512, and the
   comment beside it still claims "the suite runs about 96". Two honest
   options — move the floor to 94 and say why, or spend a session on
   `api/adminstats.py` 76%, `api/admin.py` 85%, `api/argueruns.py` 86%,
   `animist/fetch.py` 81%. Drifting is the one option nobody chose.

   **CLOSED 2026-08-19 (stragglers, PR #194) — Aaron's ruling: close the gap,
   not lower the floor. 95.185% → 95.749%, headroom ~21 → ~87 statements, and
   the four thin modules are 97/99/98/97.** The stale "the suite runs about
   96" comment is replaced with the measurement and the date. Detail,
   including the two lines left uncovered on purpose, is in White's own
   section.
10. **White 1 — four open CodeQL `py/polynomial-redos`**, re-read from the API
    tonight, all in `decks/decklist.py` (166/171/184/254) and all behind auth.
    The pre-auth one cleared. Suggested direction unchanged: bound the pasted
    decklist's length *before* the regex runs; the value is Aaron's.
    **ANSWERED 2026-08-19 (stragglers), no code change: the bound landed in
    #132 three days ago and CodeQL cannot see it.** Proved from the SARIF of
    `main` at `cf0a640` — all four taint paths run `line` at 217 straight to
    `line` at 248, past the guard at 226, because `py/polynomial-redos` for
    Python has no string-length barrier. So no further bounding clears these.
    Aaron's two options are in White's own section, with the recommendation
    (dismiss as false positives) and the reason the alternative cannot be
    proved before merge. **This is the fourth cycle running in which a carried
    item turned out to have already landed**, and the standing question below
    said a fourth would mean the habit needs a mechanism. **The trigger has
    fired; the mechanism is now owed** — and this item shows what it has to
    catch, since the tree was *not* re-checked against a fix that had been
    sitting in `decklist.py` for three days under a comment naming CodeQL.
11. **Green 1 — the stated floor is Safari 15; the bundle needs 16.4.** And
    the camera door now requires 16.4 by a second, independent route
    (`tesseract-core-simd-lstm.wasm.js`; WebAssembly SIMD is 16.4). Safari 15
    on macOS 12 is this dev machine's own browser, so the severity question is
    a ten-second check that still has not been made.
    **CLOSED 2026-08-19 (stragglers, PR #194). Aaron's ruling: declare 16.4
    and document it.** Both routes re-verified against the artifacts (the
    counts had moved: 53 and 17, not 56 and 10), written down once in
    `tests/test_browser_floor.py`, and the removable route — the SIMD core —
    is now pinned by a mutation-verified test.
12. **Green 3 + Green 4 — the phone.** 21 of 23 interactive elements under
    44px on `/import` at 375px ("Back to the library" is 17px), and
    `env(safe-area-inset-*)` appears nowhere — the latter needs
    `viewport-fit=cover` *and* the insets together, and Aaron's physical phone.
    One spacing-scale decision covers most of both.
13. **Blue 3 — `cli.py` is the last strict-mypy exception and it keeps
    growing: 79 → 109 → 126.** Annotations rather than a rewrite, but in one
    2,400-line file, so it wants its own branch. **The third consecutive rise
    is the argument.**
    **CLOSED 2026-08-19 (stragglers, PR #194): 126 → 0 and the exception list
    is empty.** Cheaper than the number suggested — 53 errors were one
    signature written 53 times, and the 52 call-site errors were the same
    errors seen from the other end. Now guarded by a test that reads the
    override table out of `pyproject.toml`.
14. **Blue 4 — `numpy` is a core dependency and only `animist` imports it.**
    Verified: the only importers are `animist/{ops,encode,motion}.py`, so every
    base install and the deployed image carry it for code that never runs
    there. A **packaging** change only the `image` job can prove — pair it with
    White's deferred `license-files` widening, same feedback loop.
15. **Blue 7 — `ROADMAP.md` is 3,109 lines and has become a narrative log.**
    Unchanged tonight. The four planning documents total 7,073 lines, which is
    a per-session cost. Concrete proposal to say yes or no to: keep ROADMAP as
    goals, open decisions and the next two or three items; move the landed
    narratives to a `docs/HISTORY.md` nothing reads at session start.
    **HALF-ANSWERED 2026-08-19 (stragglers, PR #194).** Asked whether
    Colorless's reason for not trimming the ledger also protects ROADMAP: **it
    does not.** The ledger is long because of corrections and measurements a
    later run reads; ROADMAP is long because of one section — "The near-term
    TODO" is 1,896 of 3,109 lines and fifteen of its sixteen items have
    landed. The surgical half is taken (a twenty-line "Where things stand"
    block at its head, and one stale claim corrected); **the `docs/HISTORY.md`
    split is still queued and still Aaron's.** The deferred pairing below —
    "the answer should be the same for both files" — should be cut.
16. **Black 2 — the theme conversation's second cache breakpoint**, and the
    honest note is that it is now **bounded at ~0.8%** of that mode's input
    (99.2% of the prompt is already served from cache across 77 real turns).
    Small, not large; still needs (5) as its instrument.
17. **Blue 5 (remainder) — a `PostToolUse` hook** reminding that `web_dist/`
    needs rebuilding after an edit under `web/src`. Convenience, not a guard:
    CI's `frontend` job already catches a missed rebuild.
18. **Red 5 — a merge queue, and its trigger has not fired.** Checked tonight:
    **one** open PR, a Dependabot bump, against a threshold of "more than two
    at once". Serial rainbow is what dissolved the case and it keeps
    dissolving it. Listed for completeness rather than for action — the
    stragglers pass should read this one and move on.

**Findings for a color's own next run** (not Aaron's, recorded so they are not
lost between them): the four live mutants at `decks/analyze.py:33` and
`decks/companion.py:139`, still alive after two runs — White's, and now
re-checkable with one command.

- **Deferred:**
  - **Blue is four facets and the longest reference; Red is two and the
     shortest.** Not obviously wrong — Blue's facets are cheap to sweep and
     Red's are expensive to probe — but worth re-checking once the enrichment
     half has run twice. *Trigger:* a Blue run that cannot finish its four
     facets in one session.
  - **The bench suite is in-process and cannot see the proxy, TLS, or the
     machine.** Live-instance numbers are still hand-taken. *Trigger:* a
     deployed regression the local bench could not reproduce.
  - **The ledger is 2,600 lines and the pass reads it at the start of every
     run.** Blue's queued item 7 makes the same argument about `ROADMAP.md`,
     and this file is on the same curve — three dated measurement blocks in
     Red alone, five in Black. Not trimmed here, because the two things that
     make it long are the two things that make it work: corrections kept
     *beside* their originals, and measurements kept even when healthy. The
     honest cut is the third copy of a number nobody compares against.
     *Trigger:* a run that cannot read its own section in one sitting.
     **The pairing to Blue's item 7 is cut, 2026-08-19 (stragglers):** the
     stragglers pass measured both files and they are long for different
     reasons — ROADMAP's bulk is one 1,896-line section of landed narrative,
     this file's is corrections and measurements kept on purpose. The answer
     is not the same for both, so this item no longer waits on that one.
- **Is each checklist still finding things, or reciting them?** Verdict per
  color, measured as *what its last run found* against *what its file spends
  its words on*:
  - **White — earning it, and the most.** Four fixes, none of which the file
    would have found by reciting: the only third-party code this project
    actually redistributes had no notice; the newest paid surface's job body
    had no tests; two runtime shelves were missing from the mutate map. The
    facet that produced them is the *clean-checkout install*, which the file
    added deliberately after the last gap was found by accident. Keep it.
  - **Blue — earning it, and the enrichment half worked.** The previous
    colorless run predicted the failure mode ("the sweep has concrete greps
    and the enrichment needs taste") and it did not happen: half two produced a
    real four-item shortlist ordered by cost, plus — the part that makes the
    next run cheaper — a *considered and rejected* list with the argument for
    each. Half one's result is worth recording as a baseline: exactly **two**
    technology names render across 24,712 lines of `web/src`, and everything
    else is comments, which commandment 10 does not reach.
  - **Black — earning it, and its own correction bit.** The 2026-08-19 rewrite
    around profiling was Colorless's doing last run, and this run it caught two
    comments naming an *unmeasured* cause — including one inside `bench` itself.
    That is the facet working as rebuilt. Its stale line ("six modes") is fixed
    above and was a count, not a method.
  - **Red — earning it, and it is the file that most needs re-reading rather
    than re-running.** Its fix this run came from asking what *nothing*
    checked, not from a checklist line. Its two facets are expensive to probe
    and cheap to recite, and eight open items is the largest queue in the
    file — but every one is genuinely Aaron-shaped, so the queue length is the
    facet's honest output rather than its backlog.
  - **Green — earning it, and it produced this run's sharpest lesson.** Its own
    2026-08-16 correction said *audit the artifact*, and three bullets later it
    hand-audited motion in the source, correctly, in the wrong file, and
    recorded the sweep as complete. **The correction a run makes in one facet
    is owed to its siblings in the same section** — that is the generalisation,
    it is Green's phrasing, and it is why the motion bullet is now a test.
- **The tooling, run and read as evidence about itself:**
  - `mtglab animist verify` — **12 recipes, all held.** The old memory note
    that only 2 of 12 were pinned is stale twice over: Blue parametrised
    `tests/test_animist_recipes_repo.py` over the CLI's own glob on 2026-08-18,
    and White confirmed the video outputs are genuinely decoded (a missing
    `imageio_ffmpeg` produces a *failure*, not a silent pass).
  - `mtglab bench run` — **12 targets, 12 resolved, no `skipped` row.** Warm
    medians within noise of Black's post-fix table (`/api/decks` 17.6ms,
    `/api/health` 7.9ms, search 45.1ms). The only target over 25ms is search,
    profiled unasked, database-bound at 84% inside one statement — the tool
    routing rather than describing, as designed.
  - `mtglab bench caches` — unchanged and healthy: `deck.parsed` 60/0,
    `pool.columns` 15/0, `pool.keeper` 36/0, `pool.cards` 27/3. Three read
    *never asked* because this suite does not log in. **No cache has been added
    since the register landed and nothing in it is dead.**
  - `mtglab mutate` — catalogue **1,260 sites across 18 modules** after fix 1
    (356 boundary, 327 comparison, 192 arithmetic, 163 guard, 155 boolean, 67
    constant). Kill rate is a *trend and needs a denominator*: 76% (19/25,
    seed 0, 1,231 sites) → 72% (18/25, seed 1, 1,279) → the next draw is
    against 1,260 with a floor of guaranteed survivors removed, so a rise there
    is the catalogue, not the suite. **No fresh draw taken here on purpose** —
    that is White's, and a second sample from a different run only adds noise.
  - **`mtglab bench profile`'s import counter disagreed with its own
    documentation**, which is the one thing on this shelf that had actually
    gone wrong. Fixed above. *The instrument was right and its story was
    wrong, which is the harder direction to notice.*
- **Standing questions for the next colorless run**, so it starts with
  evidence rather than a blank page:
  - Did the stragglers pass actually clear the list above, and did anything on
    it turn out to have landed already *again*? That is now three cycles
    running; if it happens a fourth, the habit needs a mechanism.
  - Has the mutation kill rate moved against the **1,260** denominator, and is
    the cause named — new tests, new code, or a different draw?
  - Were the survivors on record re-run with `--only`, or did the new verb go
    the way of the old prose?
  - Blue's enrichment shortlist produced four items and built none (the run hit
    its surgical cap). Did the next Blue run build one, or does the shortlist
    just grow?
  - `references/blue.md` is 227 lines against `references/red.md`'s 97. The
    deferred item below wants re-checking once the enrichment half has run
    twice; it has now run once.
- **Staleness, honestly stated** for the next bare `/polish`: **all six colors
  carry the 2026-08-19 rainbow tag**, so nothing is stale by date and the
  ordering has to come from substance instead. **Red first.** It owns eight of
  the twenty-one open items, two of them (alerting, and the four repository
  settings) are the only ones where the current state is *nothing is watching*,
  and its one lever on CI's wall clock has been deferred through six runs on a
  tie that has not broken. Green second, because its two phone items are the
  only ones a newcomer meets first (commandment 2) and both need Aaron's own
  hardware. After the stragglers pass, re-read this line rather than inheriting
  it — a pass that clears half the list changes the answer.
