# 47. One architecture, and a required check gates something real

**Status:** Accepted · **Decided:** 2026-08-26 with Aaron · Supersedes the
*two-architecture check* clause of [ADR
23](0023-a-green-main-deploys-itself.md), and records a decision — building
the image for more than one architecture — that had never been written in an
ADR at all.

## Context

Aaron, looking at the pipeline: **why are we building for an architecture we
do not run?**

Nobody had asked. That is the whole of the context, and the rest of this
section is the answer, re-checked rather than inherited.

- **The running app machine reports `x86_64`** (`fly ssh console -C "uname
  -m"`, 2026-08-26). The app and the forge-worker are both in `iad`.
- **`deploy` builds one architecture.** `flyctl deploy --local-only` produces
  `linux/amd64` and pushes that; the worker image beside it is `linux/amd64`
  too.
- **`image-arm64` built `linux/arm64` with `push: false`.** No registry, no
  manifest, no pull. The artifact was created on a runner and deleted with
  it, every pull request, for a week.
- **`docs/ENGINEERING.md` listed multi-arch as a bare bullet with no
  rationale** — "`linux/amd64` required, `linux/arm64` as `image-arm64`, via
  buildx" — which is a description, not an argument. Nothing anywhere said
  what it was for.
- **The `image` job's own comment argues for building the image *at all*,
  and it argues for one build.** The dev Mac is macOS 12 on Intel: Docker
  Desktop will not install and Homebrew is too stale for Colima, so CI is the
  only place the Dockerfile is ever built. That is a strong reason to build
  it once in CI. It is not a reason to build it twice.

**How it survived: it kept getting cheaper.** arm64 first ran inside `image`
under QEMU, where it cost 182 of that job's 302 seconds — 60% of the longest
job in the pipeline. On 2026-08-19 it moved to a native `ubuntu-24.04-arm`
runner, dropped to 59 seconds, and became a required check the same day. Both
changes were improvements. Neither asked whether the thing being improved
should exist. That is worth naming as a shape rather than as an incident: **an
optimisation can carry a useless thing forward for years by making it stop
hurting.** The question "is this fast enough?" cannot return the answer "this
should not run."

**What a required check is.** It has exactly one power — refusing a merge —
and its real cost is not runner minutes, it is that refusal. A check whose
subject nothing depends on cannot protect anything, so every failure it ever
produces is a false one: a red mark on a change that is fine, on a machine
that does not exist. It has no upside to trade against that. The floor for
requiring a check on `main` is that it gates something real.

## Options considered

1. **Keep it, as insurance against a future move to arm.** The honest
   accounting: an arm move is not on any roadmap, and if it happens the cost
   of *not* having had this check is one porting session, paid once, by
   somebody who is already doing an infrastructure migration and watching.
   The cost of having it is a required check on every pull request forever.
   Insurance is worth its premium when the loss is large or the event likely;
   this is neither.

2. **Keep the job but drop it from the required list.** Tempting, and it is
   the milder version of the same argument. Refused because it leaves a job
   nobody reads: green is ignored, red is ignored, and the next person to
   look finds a build for an architecture nothing runs and has to re-derive
   this whole page to decide what to do about it. An unrequired check that
   nothing consumes is a comment written in YAML.

3. **Make it real — push a multi-arch manifest and run the arm64 image.**
   This is the version where the check would gate something. It needs a
   registry, which `docs/ENGINEERING.md` §5 keeps behind a signing and
   provenance conversation, and it needs an arm64 machine to run the result
   on, which would be the migration this was supposedly insurance against.
   Not refused on the merits — it is simply a different project, and it would
   start with a new ADR rather than with this job.

4. **Remove `image-arm64`, keep `go (arm64)`.** Chosen.

## Decision

**Remove the `image-arm64` job and its required-check context. Keep `go
(arm64)`.**

`image` is unchanged and still does everything it did: it builds the app
image `linux/amd64`, loads it into the daemon, runs the container, asserts
non-root and a read-only root filesystem, greps the artifact for card pool
data, scans it with Trivy, and builds the Forge worker image. **This removes a
second architecture, not a check.**

**`go (arm64)` stays, and it is a genuinely different argument.** The
temptation is to read the two as one decision about arm64 and settle them
together; that is exactly the reasoning error that let the image build
survive. They differ in what they prove and in what they cost:

- A *container* built for `linux/arm64` proves the Dockerfile assembles for a
  machine that does not exist and will never be booted. Nothing consumes it.
- A *compiler* run against this module proves something nothing else does.
  The module carries a CGO dependency — the DuckDB driver links a prebuilt
  `libduckdb` per platform — so "it builds" is a per-architecture claim, and
  the `go` matrix's second leg is the **only arm64 compiler and the only
  arm64 scheduler this project has**: the dev Mac is Intel and cannot answer
  either question locally.

  **And it has already earned it, on the record.** `internal/api`'s
  `forgeroute_test.go` documents a genuine data race — two stub handlers
  appending to one slice — with the note that *CI's arm64 runner found it;
  twenty `-race -count` runs on the maintainer's amd64 Mac did not.* The
  same file records a second one: a test that raced itself and stayed green
  only for as long as this laptop stayed slower than the arm64 runner.
  Separately, `internal/floats`, `internal/prices` and `internal/suggest`
  carry comments naming arm64's fused multiply-add, because `a*b + c*d`
  fuses there and the recorded goldens are exact.

  It is also among the cheapest jobs in the file.

One is a duplicate of a build nothing runs. The other has twice surfaced
things nothing else in this project could have surfaced, and is the only
place an FMA divergence in the frozen goldens' arithmetic could ever show up.
Removing both because they share the word "arm64" would have been the same
category error in the other direction.

## Consequences

- **The required list on `main` becomes seven**: `frontend`, `image`,
  `no-secrets-or-card-data`, `dependency-review`, `go (amd64)`, `go (arm64)`,
  `go-lint`. `docs/ENGINEERING.md` §5 keeps it, and it remains a read-back
  from the API rather than a number remembered in a document.

- **The order of the two steps is forced, and this is the part that will bite
  somebody.** Adding a job is "write it, then require it," in either order
  without consequence. Removing one is not symmetric: a required context whose
  job no longer exists never reports, so the pull request waits on a status
  that can never arrive and there is no green to wait for. **The context comes
  off `main` before the branch deleting the job can merge.** ENGINEERING §5
  now says so where the `POST`/`DELETE` commands live.

- **`deploy`'s `needs` list loses a name, and that list is the whole safety
  argument** for continuous deployment (ADR 23) — it is the only guard against
  a `workflow_dispatch` shipping a partial suite, because branch protection
  governs merging and has nothing to say about a manual run.
  `TestTheDeployJobWaitsForEveryOtherJobInTheFile` derives the expected set
  from `ci.yml`'s own `jobs:` keys rather than restating it, so removing the
  job from both places keeps the invariant proven rather than merely intact.

- **ADR 23's `--local-only` paragraph is superseded in one clause.** It says
  the image is built twice per run, "once by `image` for the two-architecture
  check, once here for the one architecture actually deployed." The
  duplication is still real and still fine; the reason was never portability.
  It is that there is no registry between the two jobs. Everything else in
  ADR 23 stands.

- **If the instance ever moves to arm**, this is a porting session, not a
  regression: rebuild the image for `linux/arm64`, discover whatever the
  Dockerfile assumes, fix it once. `go (arm64)` means the Go half of that
  session is already known to compile and pass its tests, which is the
  expensive half of a CGO port.

- **What would reverse this**: an arm64 machine that actually runs the image
  — a second Fly region on arm, a cheaper arm instance class, or a
  contributor deploying this themselves on arm hardware. At that point option
  3 above is the one to write up, with a registry and a pushed manifest, and
  the check would gate something real. Until then, it did not.
