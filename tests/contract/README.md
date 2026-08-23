# The contract suite

What the served app promises over the wire, written so it can be asked of
**any** implementation of it. This is the first instrument of the Go
migration's equivalence strategy
([`docs/go-migration/PLAN.md`](../../docs/go-migration/PLAN.md) §5, Phase 1),
and it is pure Python: it hardens the current app whether or not a second
implementation ever exists.

## Three ways to run it

```bash
pytest tests/contract                       # in-process TestClient; part of `pytest`
pytest tests/contract --live                # seeds a scratch, starts `mtglab ui` on it, drives it over TCP
pytest tests/contract --base-url http://127.0.0.1:8080 --data-dir DIR --decks-dir DIR
                                            # a server somebody else started against DIR
pytest tests/contract --update-golden       # re-record golden/ from what the server answers
```

The in-process mode runs inside the ordinary suite and CI's `test` job. The
`contract` CI job runs `--live`, which is the mode that proves the rig:
nothing on the far side of the socket is stubbed. The external mode drives a
server somebody else started against a directory this harness seeded — which
is how **the `contract` job also runs the suite against the Go server
alone**, the exact process the container runs. To do the same by hand:

```bash
# 1. seed a scratch the way --live does
PYTHONPATH=tests python -c 'from pathlib import Path; from contract import harness; harness.seed(harness.make_scratch(Path("/tmp/solo")))'
# 2. the environment the harness gives its child (contract.harness.SERVER_ENV), plus the dirs
export MTGLAB_REQUIRE_AUTH=1 MTGLAB_SECURE_COOKIES=0 MTGLAB_DATA_DIR=/tmp/solo/data MTGLAB_DECKS_DIR=/tmp/solo/decks MTGLAB_FORGE_HOME=/tmp/solo/no-forge-here
export MTGLAB_ADMIN_EMAIL= MTGLAB_ADMIN_USERNAME= ANTHROPIC_API_KEY= ANTHROPIC_AUTH_TOKEN= RESEND_API_KEY= FLY_METRICS_TOKEN= MTGLAB_FORGE_WORKER= MTGLAB_FLY_API_TOKEN= MTGLAB_CLIENT_IP_HEADER= MTGLAB_BASE_URL=
# every SERVER_ENV name, or a credential in your shell leaks into the run: a
# real ANTHROPIC_* answers "Claude available", a real Forge home plays a JVM
# match mid-suite, and MTGLAB_ADMIN_EMAIL bootstraps a maintainer beside the
# seeded accounts -- each of which reads as dozens of failures, not one.
# 3. the server (from go/; on this Mac export the toolchain and CGO_LDFLAGS first — see CLAUDE.md §Setup)
(cd go && go run ./cmd/mtglab ui --host 127.0.0.1 --port 8765 --web-dist ../src/mtglab/web_dist --tarot ../src/mtglab/assets/tarot &)
# 4. the suite
pytest tests/contract --base-url http://127.0.0.1:8765 --data-dir /tmp/solo/data --decks-dir /tmp/solo/decks
```

## What is in here

| File | What it is |
| --- | --- |
| `routes.json` | **The route classification** — public / shared / user-scoped / admin, with a reason per route, plus the placeholders a sweep fills. The one table: `tests/test_isolation.py` reads it for the in-process sweep, this package for the live one, and the Go module (from Phase 2) for its own. `api/auth.py:PUBLIC_PATHS` must equal its `public` list; a test holds them equal. |
| `routes.py` | The Python reader of that file; validates at load. A Go reader does the same job from the same bytes. |
| `harness.py` | The three transports, the one idempotent seeder (alice administers, bob does not, `throttle` exists to be locked out, two `tiny_pool` decks, the 21-card pool), and the **mutations** — eight ways to break the app on purpose. |
| `checks.py` | Every assertion, as a plain function, with its contract in the docstring. |
| `shapes.py` | A JSON body reduced to its *shape*: keys and kinds, never data. |
| `cases.py` | Every golden record, named in one place — the stateless table and the names the stateful scenarios produce — plus what is deliberately **not** pinned and why. |
| `goldens.py` | The store under `golden/`. |
| `golden/*.json` | The recorded shapes, one file per family: `reads`, `claude`, `auth`, `jobs`, `edits`, `admin`. Review a diff here the way you would review a schema change, because that is what it is. |
| `test_contract_routes.py` | The middleware properties, generated from `routes.json`: 401 before routing, 403 under the admin prefix, no public route refused, the hardening headers on every response. |
| `test_contract_isolation.py` | ADR 5 and ADR 22 over the wire: B asks for A's things and gets 404, never 403; admin is not an exemption. |
| `test_contract_golden.py` | The stateless goldens, and the checks that none is stale or missing. |
| `test_contract_sequences.py` | The stateful goldens: a deck created, edited, promoted, built and deleted; jobs submitted and polled; a session opened and closed; the admin surface. Everything created carries the run id and is deleted, so a run is repeatable against a server that remembers the last one. |

The suite's own proof is [`tests/test_contract_harness.py`](../test_contract_harness.py):
each check is run against an app mutated in one way and shown to raise, and
the whole suite is run once, as wired, against the `envelope` mutation
(`MTGLAB_CONTRACT_MUTATE=envelope`) and shown to go red across the protected
sweep. A contract suite that cannot be shown to fail may already be passing
against nothing.

## What a golden pins, and what it does not

A record is a status, a content type, a handful of headers (`cache-control`,
`location`; `retry-after` and the session cookie as *presence*, values
masked), and the body's **shape** — keys and kinds, lists merged across their
elements, optional keys marked `?`. Not the body: ids, timestamps and card
facts are the pool's business. `int` and `float` both read as `number`,
because JSON has one number and a Go encoder writes `1.0` as `1`. Volatile
keys (a job's progress at the instant of submission; `computed_at` inside a
simulation, which is null on a fresh run and a timestamp on a cache hit) are
pinned as present and left open in kind.

The fixture is part of the contract. Every golden was recorded against the
21-card `tiny_pool` and the two decks the seeder writes; a route whose shape
depends on what the pool holds (`/api/colors/{key}` resolves champions
through it) records the shape that pool produces. Pointing the suite at the
deployed instance, which has the full pool and no seeder, is not a Phase 1
target and would need credentials the harness does not hold.

Deliberately unpinned — `/api/sets/upcoming` (asks Scryfall), the success
paths of `/api/symbols/{code}.svg` and `/api/ocr/{name}` (fill a cache from
the network), `/openapi.json`, `/docs`, `/redoc` (FastAPI's own; whether the
Go front door serves, proxies or retires them is a decision for the port,
not a shape to freeze), and every Claude route's *success* (the harness runs
with the key blanked, so those are pinned at their 503 and 422 refusals;
real conversations on the deployed pair are Phase 6's gate). `cases.py`
carries the same list beside the table.

## Adding a route

1. Classify it in `routes.json` — `tests/test_isolation.py` fails until you
   do, and so does this package.
2. If it is public, add it to `api/auth.py:PUBLIC_PATHS` as well; the
   agreement test fails until both say the same.
3. Add a golden case in `cases.py` (stateless) or a step in a sequence, run
   `pytest tests/contract --update-golden`, and review the new record in
   `golden/` as you would a schema.
4. If a new kind of refusal or header joins the wire, add a mutation to
   `harness.MUTATIONS` and its proof to `tests/test_contract_harness.py`;
   `test_every_mutation_is_proven` holds the two lists equal.
5. If it is public, the Go front door's `PublicPaths` (`go/internal/door/auth.go`)
   must name it too — `TestTheCodeMatchesTheSharedTable` holds that map equal
   to this file, as `test_isolation.py` holds `api/auth.py`. Three readers,
   one table.
