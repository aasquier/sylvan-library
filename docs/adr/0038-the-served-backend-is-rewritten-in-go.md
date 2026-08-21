# 38. The served backend is rewritten in Go, and the bench stays Python

**Status:** Accepted · **Decided:** 2026-08-21 with Aaron · **Recorded:**
2026-08-21, on the Phase 1 branch · Supersedes the *frame* of
[ADR 3](0003-tier-1-stays-python.md) for the served app (its reasoning about
the hot loop stands — see Context); meets, rather than overrides, the
condition `docs/ENGINEERING.md` §1's deferred table set for Go. The plan,
the measured baseline and the seven rulings this records live in
[`docs/go-migration/`](../go-migration/README.md).

## Context

ADR 3 answered a narrow question — *should Tier 1's hot loop be ported for
speed?* — and answered no: an 18-second batch job nobody waits on, cached by
content hash, did not earn a rewrite, and if one ever came it should be Rust
embedded in the Python process rather than Go across a boundary. That
answer was right and is still right. **This decision answers a different
question: should the whole served backend be a compiled service?** — which
is exactly the shape ENGINEERING's own table reserved Go for ("only if the
engine ever becomes a standalone service"). The engine becomes one.

The honest framing, stated first because this directory is written for the
reviewer who asks *why did you do it that way*: **no profile demanded
this.** ENGINEERING §1's trigger never fired. The port is the owner's
architectural call, bought for durability, for static typing held by a
compiler rather than by mypy's discipline, for true parallelism, for a
single static binary, and for an operational footprint the 1GB machine
strains against. What the measurements supply is not the justification but
the **equivalence evidence** — the instruments that let a full rewrite land
without the numbers changing under anyone:

- `api/jobs.py` runs one CPU worker because Tier 1 is GIL-bound, and
  `fly.toml` records why a second process is unsafe (the job registry is
  in-process). In Go the CPU pool is a semaphore over goroutines.
- `fly.toml` records 512MB as too tight for DuckDB, numpy and an Argon2id
  hash at once; the app idles at 127MB RSS on the instance (peak 215MB,
  measured 2026-08-21) in a 325MB unpacked image, 121MB compressed.
- The 13,944-case enumerated mana oracle (`POOL_ANSWER_DIGEST`) and Tier 1's
  `REFERENCE_DIGEST` were built, per ENGINEERING §2, to be *"usable as the
  differential case set for a port… in any language, forever."*
- `decks/edit.py` is hand-rolled text surgery (ruamel measured and rejected
  in its own docstring), so Go never serialises a deck — it parses to check
  and refuses on mismatch, exactly as Python does.
- numpy is imported only under `animist/`; Tier 1, the wheel and the tarot
  deal are stdlib `random.Random`, which is what makes a bit-exact port
  (`pyrand`) possible rather than merely statistical.

## Options considered

**Stay Python and add workers.** Rejected. The registry is in-process by
design (ADR 23's shape; `fly.toml`), so horizontal workers break job
visibility without a shared store that this app has deliberately not
needed; it buys neither the typing nor the binary nor the footprint.

**Rust, as ADR 3 sketched.** Right for the hot loop in-process; wrong for a
whole service. The choice is between a Python app with a Rust library
inside it and a compiled service — and the owner's call was the service.

**A big-bang rewrite on a long branch.** Rejected. `main` deploys itself
(ADR 23), 218 squash merges had landed in twelve days when this was
decided, and a second Claude session lands work concurrently; a branch that
waits to pass everything is a fork factory. Recorded because it is the
obvious shape and the one that kills rewrites.

**Port *everything*, the asset pipeline included.** Rejected. `animist/`
(Pillow, imageio-ffmpeg), `cardmotion` build (torch behind the `depth`
extra), `bench/` and `mutate/` are ~4,400 lines that never ship in the image
(ADR 29, ADR 32, `test_packaging.py`). Torch has no Go path, Pillow no peer
worth chasing, and offline asset tooling buys no user value ported. This is
the one genuine "Python gains us something" and it is carved out, not
argued around.

**A strangler behind a Go front door, one route family at a time.**
Accepted.

## Decision

1. **The served app is rewritten in Go** — `api/`, `auth/`, `decks/`,
   `sim/`, `cards/`, `claude/`, `artifacts/`, the reference-prose modules,
   `mana.py`, `tarot.py`, `symbols.py`, `ocr.py`, `caches.py`, `config.py`,
   and the runbook half of `cli.py`. **`animist/`, `cardmotion` build and
   depth, `bench/` and `mutate/` stay a Python dev package indefinitely**,
   renamed `mtglab-bench` at retirement; `mtglab` stays the name of the Go
   binary so every runbook command survives verbatim.
2. **The mechanism is a strangler.** The Go binary takes `:8080` early:
   auth middleware first, deny-before-route in front of *both* runtimes,
   static `web_dist`, and a reverse proxy to uvicorn on loopback for
   everything not yet ported. **Route families flip atomically** — a
   job-shaped feature's submit and poll move together, because a job born in
   one runtime is a 404 in the other — and the Forge lane exists in exactly
   one runtime at every moment. Both share the volume: `app.db` in WAL mode
   with a busy timeout, `/data/decks`, the DuckDB pool read-only.
3. **Equivalence is gated, per phase, by instruments that already exist or
   are built first**: the contract suite (`tests/contract/`, built with this
   ADR — base-URL mode, the route classification as one shared JSON table,
   golden wire shapes, mutation-proven), the mana oracle ported as data, a
   bit-exact `pyrand` so Tier 1 reproduces `REFERENCE_DIGEST` and a
   client-held seed deals the same spread across the cutover (statistical
   equivalence only by a written decision), and the closed forms to a pinned
   epsilon. Every flip is walked on the deployed pair before it is called
   done (commandments 14 and 16).
4. **Sessions and passwords survive the cutover.** Argon2id PHC hashes
   verify as-is; the session-token scheme is matched and proven against
   fixtures Python wrote. A one-time global sign-out is the declared
   fallback, taken deliberately or not at all.
5. **The toolchain**: stdlib `net/http`; `go-duckdb` (CGO); `goccy/go-yaml`
   for the oracle *parse* only; `alexedwards/argon2id`; the official
   `anthropic-sdk-go`; stdlib `testing` with `go-cmp` and native fuzzing;
   `golangci-lint` + `go test -race` + `go vet` as required checks. **And
   `spf13/cobra` for all CLI work — Aaron's explicit requirement, not a
   default open to revisiting.** Go **1.26** on the module, because Go 1.26
   is the last release that runs on this Mac's macOS 12 (Go 1.27, August
   2026, requires macOS 13 — read from both release notes 2026-08-21);
   verified by running `go1.26.7` here the same day. That is a runway, and
   it is named so nobody is surprised: roughly until Go 1.28 ships, after
   which development on this machine moves to the CI/container loop or to
   a newer machine.
6. **It is the main line.** Simulator rungs 4–5 pause for about one
   build-week — ~4–7 working days at the repository's *measured* velocity
   (~19 merges/day over twelve days), a figure corrected twice by Aaron
   against the git history before it was written down, and re-priced on
   Phase 2's actuals rather than argued further.

## Consequences

- **Two runtimes in the image until Phase 8**, and the image gets bigger
  before it gets smaller; stated so the interim is never read as the
  outcome. Retirement is a phase with a gate and a published comparison
  (PLAN Appendix B, BASELINE.md as the "before" column), not a someday.
- **Every enforcement mechanism ports or the port is incomplete** (PLAN §8):
  the route classification is one table both suites read; the claude/
  write-boundary becomes a `go/analysis` pass over typed call graphs; the
  schemas' deliberate absences (no field for a `why`, a defence, a card name)
  are struct types plus a table test; the register of caches, the skip gate,
  the coverage floor and the secrets scans each have a named Go answer.
- **ADRs 5, 8 and 11–37 carry forward unchanged as constraints on the Go
  implementation.** A rewrite of the code is not a rewrite of the decisions;
  where a Go handler would make a different call, that is a new ADR, not a
  quiet divergence.
- **`web/src` does not change.** The committed bundle keeps working against
  the Go backend unmodified, which pins wire shapes, the job lifecycle, and
  FastAPI's `{detail}` error envelope (422 validation shape included) for as
  long as the frontend is not deliberately changed later.
- **Commandment 10 holds in the new language**: "Go" appears in docs and CI
  and never in copy; the port must not change a rendered word.
- **A module marked `porting` on the board is frozen for feature work**; a
  change that cannot wait lands on the Python side and is flagged as owed to
  the port, which does not flip that family until it is mirrored. Schema
  changes keep their standing rule and land in the runtime that owns the
  ladder — Python until Phase 8.
- **Scope creep wearing a helpful face is refused**: the port changes
  behaviour only where a gate says so. The one blessed exception is
  `load_printings` via go-duckdb's Appender at Phase 8, because the ledger
  already queued it and the driver hands it over.
