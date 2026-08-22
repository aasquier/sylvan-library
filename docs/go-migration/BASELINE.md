# Baseline — the Python backend, measured

The "before" column of the comparison Aaron asked for. Captured 2026-08-21 on
the dev Mac (macOS 12, Intel) unless a row says otherwise; performance rows
are quoted from `docs/polish/LEDGER.md` (Black, 2026-08-19 rainbow — quiet
machine, full pool) rather than re-measured, because another session was
active on this machine today and the ledger's own rule is *read the ledger
before re-measuring*. **Append-only**: re-measurements get a new dated block.

## Size

Method: `git ls-files 'src/mtglab' | grep '\.py$' | xargs wc -l`, rolled up by
package; raw line counts (comments and docstrings included — this codebase's
docstrings are load-bearing and porting them is part of the work).

| Package | Lines | Files | Migrates? |
| --- | ---: | ---: | --- |
| top-level (`cli.py`, `mana.py`, `tarot*.py`, `colors.py`, `glossary.py`, `lore.py`, `config.py`, `caches.py`, `symbols.py`, `ocr.py`) | 9,261 | 12 | yes — see note on reference prose |
| `api/` | 8,448 | 18 | yes |
| `claude/` | 6,620 | 15 | yes |
| `decks/` | 5,022 | 15 | yes |
| `sim/` | 4,336 | 17 | yes |
| `animist/` | 2,623 | 11 | **no — stays Python** |
| `auth/` | 2,607 | 10 | yes |
| `cards/` | 1,360 | 3 | yes |
| `cardmotion/` | 637 | 5 | build stays Python; serving routes (in `api/`) migrate |
| `bench/` | 626 | 4 | **no — stays Python** |
| `mutate/` | 524 | 3 | **no — stays Python** |
| `artifacts/` | 322 | 2 | yes |
| `prices/` | 0 | 1 | stub |
| **Total** | **42,386** | **116** | |

- **Migrating: ~38,000 lines** (total minus animist/bench/mutate/cardmotion-build).
  Of that, **~5,400 lines are reference prose as code** (`tarotlore.py` 2,159,
  `tarot.py` 1,048, `colors.py` 941, `glossary.py` 644, `lore.py` 620) — data
  to re-encode, not logic to re-derive.
- **Staying Python: ~4,400 lines** of dev-machine tooling (the animist asset
  pipeline, cardmotion build/depth, bench, mutate). See PLAN.md §2 for why.
- Frontend, for scale (not migrating): `web/src` is 38,724 lines of TS/TSX/CSS.
- Largest single files: `api/service.py` 2,775 · `cli.py` 2,695 ·
  `tarotlore.py` 2,159 · `claude/theme.py` 1,684 · `api/app.py` 1,535 ·
  `decks/edit.py` 1,110 · `cards/db.py` 1,064.

## Tests

- **2,668 tests collected** (`.venv/bin/python -m pytest --collect-only -q`,
  2026-08-21), across **90 files / 36,589 lines**.
- Full-suite wall clock: **352s** at 2,470 tests (ledger, 2026-08-19, this
  Mac, quiet). Coverage **95.749%** against a `fail_under` of 95
  (11,573 statements).
- Mutation baseline: kill rate **76%** (seed 0, 25-draw) / **72%** (seed 1),
  catalogue 1,260 sites across 18 modules — read the White section of the
  ledger before quoting these; the two draws are samples, not a trend.
- Route surface: **57 classified routes** in `tests/test_isolation.py`
  (2026-08-19 count; a handful added since — labels/themes editor); 75 route
  decorators counted under `src/mtglab/api` today.
- The two language-independent correctness pins a port inherits for free:
  the **13,944-case enumerated mana oracle** (`tests/mana_oracle.py`, pinned
  by `POOL_ANSWER_DIGEST`, dumpable as JSON Lines — built for exactly this,
  per ENGINEERING §2) and Tier 1's **`REFERENCE_DIGEST`** golden
  (`tests/test_determinism.py`, byte-identical on CPython 3.11/3.12).

## Performance (quoted from the polish ledger, 2026-08-19)

Warm medians / p95, `mtglab bench run --runs 15`, this Mac, full pool:

| Target | warm p50 | p95 | of which DuckDB |
| --- | ---: | ---: | ---: |
| `GET /api/health` | 7.3ms | 8.2ms | — |
| `GET /api/decks` | 16.5ms | 18.0ms | 0.98ms, 1 stmt |
| `GET /api/colors` | 6.1ms | 7.2ms | — |
| `GET /api/glossary` | 4.5ms | 4.9ms | — |
| `GET /api/lore` | 5.9ms | 7.2ms | — |
| `GET /api/cards/search?q=goblin` | 43.8ms | 49.0ms | **37.9ms, 1 stmt** |
| deck detail | 7.1ms | 8.0ms | — |
| deck validate | 6.0ms | 6.7ms | — |

Cold (caches emptied per sample): `/api/decks` 134.2ms · `db.get_cards`
83.2ms · deck detail 80.0ms · `/api/health` 38.1ms. The cold/warm gap is
memoisation, and the cold number includes an import-storm class Go simply
does not have — a fair comparison quotes both.

Context for reading these: the search row shows **37.9 of 43.8ms inside one
DuckDB statement** — language-independent; Go will not move it. The things Go
*should* move are the cold column, RSS, image size, boot, and concurrency
(below). Long-running surfaces are network-bound Claude calls (theme proposal
226s, dossier 236s, research 265s measured) and the Forge worker (~40s wall
per 3-game match) — also language-independent.

Live instance TTFB p50 from this Mac (5 samples, 2026-08-19): `/` 237ms ·
`/api/health` 213ms — dominated by RTT, not compute.

## Runtime & operational shape

- **Concurrency ceiling is a documented Python concession.** `api/jobs.py`
  runs **one** CPU worker because Tier 1 is pure Python and GIL-bound
  (CLAUDE.md); `fly.toml` warns a second uvicorn worker is unsafe because the
  job registry is a module-level dict. Tier-1 multiprocessing measured 2.42×
  on 4 workers (ENGINEERING §1) and was never wired in.
- **Memory:** deployed VM is `shared-cpu-1x` / **1GB**, with `fly.toml`
  recording that 512MB was too tight for DuckDB + numpy + Argon2id + a sweep
  at once.
- **Dev environment weight:** `.venv` is **1.0GB** (dev extras). `web_dist`
  7.8MB, packaged tarot assets 4.6MB, full pool `data/mtg.duckdb` 110MB
  (never shipped).
- **CI wall clock:** last four green `ci.yml` runs on main: **~6.5–10 min**
  (2026-08-21, `gh run list`).
- **Data refresh:** 28 min measured — ~9 min download + **~16 min
  `load_printings`** at ~110 rows/s, profiled to DuckDB's prepared-statement
  path driven once per row (ledger, Black §queued 3). Relevant because
  go-duckdb's Appender is the bulk path that fix wants anyway.
- Python runtime pins: CPython 3.11/3.12 (uv-managed), FastAPI ≥0.110,
  uvicorn, duckdb ≥1.0, numpy ≥1.26 (**imported only by `animist/`** —
  measured today; Tier 1 is stdlib `random.Random`), pyyaml, argon2-cffi,
  anthropic ≥0.69.

## To capture in a quiet window (Phase 0 checklist)

Owed before the plan's Phase 0 exit — none was safely measurable today with a
sibling session active and no wish to disturb the instance:

- [x] Deployed **image size** (compressed + unpacked; `fly image show` or the
      registry) and CI image-build wall time. *(block below)*
- [x] Instance **RSS** at idle *(block below)*; **during a Tier 1 run** not
      taken — it needs a signed-in request against the instance, which the
      harness cannot make; a CLI run on the box would measure a second
      process, not the app. Owed to the Phase 5 measurement, where the GIL
      dividend is measured anyway.
- [x] **Boot-to-healthy** time (deploy log timestamps: machine start →
      first 200 from `/api/health`) — the per-merge downtime window. *(block below)*
- [ ] Fresh `mtglab bench run` + `--cold` on a quiet Mac — **deliberately not
      taken 2026-08-21**: the Mac was not quiet (load 4.2, `mediaanalysisd`
      at 32% CPU), and a number taken then would have had to be explained
      away against the ledger's. The ledger's 2026-08-19 block stands as the
      baseline; re-measure on a quiet machine before Phase 8's comparison.
- [x] Fresh full-suite wall clock *(block below)*.
- [x] `pip install -e ".[dev]"` wall time on a clean checkout (the Go
      toolchain-setup comparison point). *(block below)*

## 2026-08-21, later — the quiet-window captures (Phase 0 remainder)

Taken the same day, on the Phase 1 branch, from reads only: the registry
manifest, `/proc` on the instance over `fly ssh console`, the last `main`
run's job and step timestamps (`gh run view --json jobs`, run 32509614960,
commit 0f87bde), and a clean venv in the scratchpad. Nothing was deployed or
restarted to get them.

| Metric | Measured | How |
| --- | ---: | --- |
| App image, compressed | **121.3 MB** (10 layers) | sum of layer sizes in the registry's v2 manifest for the deployed tag |
| App image, unpacked | **≈325 MB** | `du` on the instance: `/opt/venv` 219M, `/usr/lib` 48M, `/usr/local/lib/python3.12` 28M, `/usr/bin` 23M, `/var` 6.3M, `/etc` 1.3M |
| Idle RSS, the app process | **127.2 MB** (VmRSS 127,224 kB; VmHWM 214.9 MB; 8 threads) | `/proc/<pid>/status` of `mtglab ui`, 2 h 39 m after the deploy; PID 1 is `/fly/init` at 5 MB |
| Instance memory | 985 MB total, **680 MB available** | `/proc/meminfo` |
| Machine update → health passing | **≈23.6 s** | deploy log: "Waiting for machine … to reach a good state" 17:50:36.3 → "Checking health" 17:50:59.3 → app answered 17:50:59.98. Bounded below by the HEALTHCHECK's 10 s grace and 30 s interval, so the process itself boots faster than this reads |
| Merge → instance healthy | **7 m 44 s** | run created 17:43:16 → smoke test passed 17:51:00 |
| Whole pipeline (incl. worker image push) | **9 m 37 s** | 17:43:16 → 17:52:53 |
| `test (3.12)` job / its Tests step | 6 m 16 s / 5 m 40 s (under coverage) | job timestamps; `test (3.11)` 3 m 27 s / 3 m 04 s |
| `image` job / amd64 build / Forge-worker build | 2 m 27 s / 45 s (cache replay) / 68 s | step timestamps; `image-arm64` build 44 s native |
| `pip install -e ".[dev,api]"` in CI | 15–16 s (pip cache warm) | step timestamps, both matrix legs |
| `pip install -e ".[dev]"` on this Mac, clean venv | **79 s**, 330 MB venv (pip cache warm) | `python3.12 -m venv` + install, timed |
| Full suite wall clock, this Mac | **265 s at 2,904 tests** (the contract suite and its proof added ~236) | `pytest -q` on the Phase 1 branch, same afternoon, load ~3–4 — *faster* than the ledger's 352 s at 2,470 on a quiet day, which is a question for the next bench pass rather than a datum to trust |
| Go on this Mac | **`go1.26.7 darwin/amd64` runs**; 1.27 requires macOS 13 | official per-version installer into `~/sdk`, 20 s; release notes for 1.26 and 1.27 read the same day |

Two of these are the honest opposite of a clean number and are recorded as
such. The **boot** figure is a ceiling, not the process's own start time,
because the only timestamps available sit on either side of a health check
that polls every 30 s. And the **RSS during a Tier 1 run** was not taken:
it needs a signed-in request on the instance and the harness holds no
credentials there; the measurement belongs with Phase 5, where the GIL
dividend is measured on the same machine anyway.

## 2026-08-21, later still — the pair, deployed (Phase 2, release v156)

The first "during" block rather than a "before" one: the Go front door in
front of the Python server, as the container has run since PR #220 merged
(22:27 UTC; v156 live 22:33). Read the same way as the block above — the
registry manifest via `fly image show`, and `/proc` on the instance over
`fly ssh console` with a base64'd script, since the image has no `ps` — a
few minutes after the release, with nothing restarted to get them. The
interim is larger than the outcome, as PLAN §4 said it would be.

| Metric | Measured | How |
| --- | ---: | --- |
| App image (flyctl's figure) | **347 MB** | `fly image show`; ~325 MB before — +22 MB, the 11.3 MB static door binary among it and a `golang` build stage that ships nothing |
| Idle RSS, the door | **31.6 MB** (8 threads) | `/proc/<pid>/status` of `/opt/door/mtglab`, uid 10001, the container's pid 652 |
| Idle RSS, the Python server behind it | **104 MB** (VmHWM 105.6 MB) | same, the supervised child |
| Idle RSS, the pair | **≈136 MB** | the two above, against 127 MB for Python alone in the block above |
| Merge → release live | **≈6 min** | 22:27 → 22:33 (the PR's CI had already run; main's run re-ran the five and deployed) |
| Pool on the instance at the read | 35,393 oracle cards, 7 decks | `/api/health` through the door |

Two notes. Fly's edge compresses door-served assets (`app.js` arrived
`content-encoding: gzip` from a door that sends no gzip of its own), so the
door carries no compressor for the duration. And the door's RSS is the
reference-prose-free Phase 2 binary; Phase 3 embeds ~230 KB of JSON and,
with the pool, links libduckdb — the next block measures that.

## 2026-08-22 — the pair with the pool in the door (Phase 3, release v158)

Read the same way as the block above, a few minutes after v158 (PR #225:
the door reads the card pool through libduckdb; `/api/cards/search`,
`/api/cards/identify`, `/api/colors/{key}` and `/api/lore` answered by Go),
right after a signed-in walk had asked the door for a search and a lore
shelf -- so the pool had just been opened and was inside its lease.

| Metric | Measured | How |
| --- | ---: | --- |
| Idle RSS, the door | **119.2 MB** (VmHWM 121.5 MB; 8 threads) | `/proc/<pid>/status` of `/opt/door/mtglab`, pid 653 |
| Idle RSS, the Python server behind it | **104.0 MB** (VmHWM 126.4 MB) | same, pid 673 |
| Idle RSS, the pair | **≈223 MB** | against ≈136 MB for the Phase 2 pair and 127 MB for Python alone |
| App image, compressed | **147.3 MB** (12 layers) | sum of layer sizes in the registry's v2 manifest for the deployed tag (`curl -u "x:$(fly auth token)" registry.fly.io/v2/sylvan-library/manifests/<tag>`); 121.3 MB before the door, ~+22 MB (347 MB unpacked per flyctl) with the static door at v156 |

The door's number is the one to read: **+88 MB over the static Phase 2
door**, which is libduckdb loaded in-process plus the pool's instance held
open on the ten-second lease -- the same memory Python pays when it opens
the pool, now paid by both halves of the pair -- and the image grew by the
same cause (the CGO door binary links libduckdb's static archive). It is the interim PLAN §4
named (the image and the pair get bigger before they get smaller) and it is
well inside the 1 GB machine; the door alone after Phase 8 is the number
Appendix B wants, and this block is what it is compared against.
