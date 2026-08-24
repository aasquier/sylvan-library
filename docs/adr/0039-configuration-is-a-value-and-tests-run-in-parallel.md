# 39. Configuration is a value, and tests run in parallel

**Status:** Accepted · **Decided:** 2026-08-24 with Aaron · **Recorded:**
2026-08-24, on the branch doing the work · Supersedes the "read at call time"
reasoning in `internal/config`'s package comment (rewritten here to argue the
opposite); leaves [ADR 14](0014-python-decides-claude-advises.md),
[15](0015-claude-surfaces-are-modes-with-capabilities.md),
[16](0016-accounts-are-invited-and-passwords-are-self-served.md) and
[17](0017-the-maintainer-is-named-in-the-environment.md) untouched — what is
configured and who decides it does not change, only when it is read.

## Context

`internal/config` answered every question by reading the environment at the
moment it was asked. Its package comment argued the case plainly: values are
"read at call time and never bound at start, so a test can point the process at
a scratch directory and a container at its volume without a restart ceremony."
`claude.Ceiling` made the same argument in its own words — "an operator changing
the cap should not have to restart the process to lower it."

The argument was wrong twice over, and the second half is the expensive one.

**The reload it bought was theoretical.** A container's environment cannot be
changed without replacing the container, and nothing in `docs/HOSTING.md` ever
retuned a running process. The property was real and nobody used it.

**The cost was not theoretical.** Reading process-global state at call time
means the only way to test any of it is to *write* process-global state — which
is `t.Setenv`, which Go refuses to let a test combine with `t.Parallel`, because
the environment is shared by the whole binary. There were 106 such calls. Each
one pinned its test to serial execution, and the taint travelled: a helper that
called `t.Setenv` made every test that called *that helper* serial too, through
plain functions and through methods, invisibly.

The suite ran in 2m28s wall against 6m59s of CPU. Packages already ran in
parallel; tests inside a package did not. One package, `internal/api`, took
137.6s of the 148 — 204 tests, one at a time.

The worst of it was not slowness. `internal/api`'s Claude tests stand up a stub
`httptest.Server` per test and then have to publish its URL to the code under
test, and the only channel available was `ANTHROPIC_BASE_URL`:

```go
srv := httptest.NewServer(...)          // per-test
t.Setenv("ANTHROPIC_BASE_URL", srv.URL) // process-global
```

That is not merely parallel-hostile. It is a single global slot, so two tests
with two stubs cannot coexist at all, at any speed. The shape had nowhere to go.

The tests were also defending against the machine they ran on. A helper named
`noCredential` existed to blank `ANTHROPIC_API_KEY` "because this machine's
shell may well have one", and `bootstrap_test.go` set both admin variables to
`""` on every test so that "a developer's own MTGLAB_ADMIN_EMAIL leaking into a
test would make the suite pass or fail depending on whose laptop ran it".

## Options considered

**Leave it, and accept a serial suite.** The honest baseline. It costs 45
seconds a run today, and it grows with the test count — but the real objection
is not the clock. `ANTHROPIC_BASE_URL` as the only channel for a stub server's
URL is a design that cannot express two stubs, so some tests could never be
written at all. Rejected because the ceiling is on what is *testable*, not on
how fast.

**Keep call-time reads; give tests a mutex around the environment.** A shared
lock every env-touching test acquires. It works, and it is what a suite reaches
for when the code cannot be changed. Rejected because it makes the serialisation
explicit rather than removing it — the tests still run one at a time, and now
there is a lock to forget. It also does nothing for the two-stubs problem.

**Load once into package-level variables at `init`.** One read, no threading, a
small diff. Rejected because it trades a testability problem for a worse one: a
value captured at `init` cannot be varied by a test *at all*, and Cobra builds
its command tree at `init` too, so the capture would race the process's own
setup. Package-level mutable state is what `internal/sim/cache`'s tests already
demonstrate the cost of.

**Resolve once into a value and pass it.** Chosen. It is the pattern three
constructors in this module already use, so it adds no new concept — it closes
the six places that leaked past it.

## Decision

**Configuration is a value, resolved once, and passed.**

`config.Load` reads the environment and returns a `config.Config`. It is the
only reader of the settings that value carries. `cmd/mtglab` — the composition
root — calls it, and everything below takes the resulting value.

Three readers remain, each deliberate: `cmd/mtglab`'s `envOr`, which supplies
flag defaults for `--web-dist` and `--tarot` while Cobra is still building the
command tree and before any `Load` could have run; and the Forge and Anthropic
variables, which belong to the second injection this ADR does not attempt.

This is not a new pattern here; it is the existing one, finished. `api.New(cfg
Config)`, `door.New(cfg Config)` and `jobs.New(cfg Config)` were already
constructor-injected. The environment reads were leaks *past* that pattern, and
only six of them were below `cmd/`.

Concretely:

- `config.DataDir()` and its fourteen siblings become fields on `config.Config`,
  with the derived paths (`DBPath`, `AppDBPath`, `CacheDir`, …) as methods.
- `config.Flag(name, fallback)` becomes `config.ParseFlag(raw, fallback)` — the
  rule, testable without a process to install it on.
- `auth.SenderFromEnv(transport)` becomes `auth.SenderFor(MailSettings,
  transport)`. The *choice* stays late, so an instance with no key still answers
  503 at the moment somebody sends rather than refusing to boot; only the read
  moves to startup.
- `auth.EnsureMaintainer(ctx, db)` takes the config. `auth.ClaimLink` requires
  its `baseURL` instead of falling back to reading one.
- `api.Config` gains `Mail` and `ClientIPHeader`; `door.Config` passes both
  through.
- `bootSummary` and `configComplaints` take the `config.Config` they describe.

**And every test that is not holding something shared calls `t.Parallel`.**

## Consequences

The suite runs in **1m43s**, down from 2m28s, with 663 tests parallel.
`internal/api` went from 137.6s to 87.0s and `internal/auth` from 23.2s to
11.4s. `t.Setenv` fell from 106 calls to the handful that are genuinely about
the environment — `config.Load`'s own test, and the CLI tests that drive a real
command end to end.

Tests describe deployments instead of installing them. `configComplaints` was
one procedural test that set variables, asserted, reset them and asserted again;
it is now seven cases in a table, each independent, all parallel. Nothing
defends against the developer's shell any more, because nothing can reach it.

One test got strictly stronger. `TestTheBootSummaryLeaksNoSecret` used to
discover credential-shaped *environment variables* and check none appeared in
the boot line. It now walks `config.Config`'s own fields by reflection — and
since the summary can only print what it is handed, the fields of that value are
provably the complete list of what it could leak. The old spelling could only
cover the variables somebody thought to look for.

**What this costs.** Live-retuning a running process is gone. Nothing used it,
and lowering a stance ceiling or moving a data directory now needs the restart
that replacing a container was always going to involve anyway.

**What it does not cover.** `internal/claude` still reads `ANTHROPIC_*` and
`MTGLAB_CLAUDE_MODEL`, and `internal/sim/tier3` still reads the Forge variables.
Those are a second injection, deliberately not attempted here — and the
credential one carries an argument this ADR does not overrule: `Connect` builds
its SDK client with no options *precisely* so the key is resolved inside the SDK
and never held in our own memory, where it could be logged. Injecting a test
seam there has to keep that property, which is a different design problem from
this one. Until then, the tests that stub Claude or Forge stay serial. They are
about 195 of 858.

**A lesson worth recording.** The first parallel sweep was driven by a
file-level search for `t.Setenv` and it panicked twice, on a helper and then on
a *method* in another file. The second was driven by a transitive taint walk and
still shipped a data race: `internal/sim/cache`'s tests swap package-level
globals (`engineSources`, `fingerprintVal`, `fingerprintOnce`) to fingerprint a
different source set, which no search for `t.Setenv` would ever find. `-race`
caught it; nothing else would have. **A test is unsafe beside its neighbours if
it writes anything shared — the environment is only the most obvious thing it
can write.**

`claude.Ceiling` still reads at call time and still carries the old argument in
its own comment. That is the second injection named above, not an oversight —
the argument falls when the code does, and not before.
