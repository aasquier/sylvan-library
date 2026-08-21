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

- [ ] Deployed **image size** (compressed + unpacked; `fly image show` or the
      registry) and CI image-build wall time.
- [ ] Instance **RSS** at idle and during a Tier 1 run (`fly ssh console`,
      `/proc/1/smaps_rollup` of the app process).
- [ ] **Boot-to-healthy** time (deploy log timestamps: machine start →
      first 200 from `/api/health`) — the per-merge downtime window.
- [ ] Fresh `mtglab bench run` + `--cold` on a quiet Mac (compare against the
      ledger block above; investigate any drift before trusting either).
- [ ] Fresh full-suite wall clock at 2,668 tests.
- [ ] `pip install -e ".[dev]"` wall time on a clean checkout (the Go
      toolchain-setup comparison point).
