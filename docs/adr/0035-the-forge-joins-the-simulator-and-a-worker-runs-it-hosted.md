# 35. The Forge joins the Simulator, and a worker runs it hosted

**Status:** Accepted · **Decided:** 2026-08-20 with Aaron · Rides on ADR 14
(Forge plays the games) and closes the open decision recorded 2026-08-11 as
"Can Forge run where the app runs?" — the feasibility spike answered the
prior question, and this picks the shape.

## Context

The Tier 3 bridge has been finished at the CLI since 2026-08-11: `mtglab sim
forge` plays headless Commander games, all six curated decks are fully
covered by Forge's card scripts, coverage is checked before **and** after
every run because an unimplemented card does not stop a game, and clock-outs
are reported apart from draws. What never existed was an app surface — and
the hosted app is the target; CLI-only is the live failure mode.

The spike's numbers, all measured on the 2015 MBP (8 logical CPUs), decide
most of what follows:

- Heads-up medians sit at 4.6–6.8s per game with a long tail (one Trostani
  game ran 134.5s); ten games are 67–262s of wall clock, plus ~9s of JVM and
  card-database start that amortises per invocation.
- Four-player pods hit the 300s clock in **40% of games**. A clocked game is
  the measurement giving up, and there is no clock setting at which pods are
  both honest and quick on this hardware.
- The deployed machine is `shared-cpu-1x` with 1GB. Forge is a ~470MB JVM
  desktop application that holds 100–200% CPU for minutes and defaults to a
  4GB heap ceiling here.

Three hosted shapes were recorded with the spike: local-only, server-side on
the app machine, and a separate worker. None was chosen then.

## Decision

**1. The Simulator grows a Forge mode, gated on the environment.** A
`GET /api/forge` gate answers whether the distribution and a JVM are
reachable from the server's process — the `/api/claude` contract: a fact
about the environment, no network, no JVM booted to ask. Where the answer is
no, the mode is honestly absent — the option does not render, nothing is
greyed out, and the maintainer-facing `why` string never reaches a user's
eyes (commandment 10).

**2. Heads-up only, and that is a decision rather than a v1 shortcut.** A
mode whose four-player results are 40% clock is not honest enough to ship;
the CLI still plays pods for whoever wants to watch one. Lifting this means
re-measuring on whatever hardware the worker (below) lands on, not deleting
a guard.

**3. Everything refusable is refused in the request** (the `themeruns`
division): missing decks 404, malformed asks 422, absent Forge 503, and a
deck Forge cannot fully play a 422 that names the cards — `check_coverage`
reads a zip on the request thread and was designed to. The job holds exactly
one thing: the subprocess. Unlike `simruns`, planning failures are not
deferred into the job; that deferral is compatibility with a shape this new
surface never had.

**4. A third job lane, `FORGE`, with one worker.** A match is a thread
asleep in `subprocess.run` while a JVM works, so it fits neither existing
lane: in `CPU` it blocks Tier 1 for minutes; in `NET` two concurrent JVMs
race the shared `.dck` directory `ensure_profile` hands out and saturate the
machine. One worker makes both impossible by construction, and in-flight
dedup (`Plan.key`, the dossier's money-bug rule) makes a second identical
click join the live match.

**5. Hosted, Forge runs on an on-demand worker machine, not on the app
machine.** A second Fly machine in the same app, built from a worker image
(JRE + the Forge distribution + a thin shim, with `forge.profile.properties`
baked at build time), sized with a **dedicated** CPU, held **stopped** when
idle. The app starts it per job through the Machines API, results return
over the private network, and the machine stops itself. Billing is
per-second while running: a ten-game match costs fractions of a cent, and a
stopped machine costs only its rootfs storage.

Server-side on the app machine is rejected on honesty grounds, not just
sizing: the spike's timings are facts about eight fast cores. On throttled
shared CPU the tail stretches toward the clock, and a clock that starts
clipping real games converts them into fake draws — an underpowered server
does not run slow, it corrupts the measurement. A permanently larger machine
pays always-on money for occasionally-used CPU and still shares cores with
DuckDB and the app.

**6. The worker is its own branch.** This decision lands with the app
surface; the worker image and the Machines API plumbing follow separately,
because the container half can only be proven in CI (this Mac cannot build
images) and a machine-management change deserves its own review. Until it
lands, the deployed instance answers the gate honestly: no Forge here.

## Consequences

- The laptop's `mtglab ui` has real games today; the deployed instance shows
  the mode's absence honestly until the worker lands. That gap is stated,
  not hidden — the same posture as `docs/FORGE.md` §install.
- The two hosted-shape problems named with the spike dissolve: the profile
  write bakes into the worker image, and a one-at-a-time lane cannot race
  the `.dck` directory.
- Results quote medians and tails, never a mean alone, and clock-outs are a
  separate column end to end — parse, payload, tiles.
- Nothing is cached. Forge matches are seeded by default (Tier 1's doctrine
  and its literal default seed), but a cache key would have to name the
  engine's own behaviour, and `forge_home` upgrades under it. In-flight
  dedup is the whole memory until someone measures that repeat asks are
  common.
- Goals 2, 3 and 7 unblock: the tier list's engine-vs-engine half has a
  surface to grow on, per archetype, never as a single ranking.
