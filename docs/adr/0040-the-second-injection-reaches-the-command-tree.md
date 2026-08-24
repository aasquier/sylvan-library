# 40. The second injection reaches the command tree

**Status:** Accepted · **Decided:** 2026-08-24 with Aaron · **Recorded:**
2026-08-24, on the branch doing the work · Finishes the injection
[ADR 39](0039-configuration-is-a-value-and-tests-run-in-parallel.md) named and
deliberately did not attempt; supersedes the "read at call time" reasoning
still standing in `internal/sim/tier3`. Leaves the Anthropic credential where
ADR 39 left it — `claude.Endpoint` (#289) already moved that, and the SDK's
own key resolution is untouched here.

## Context

ADR 39 ended with a paragraph headed **What it does not cover**, and this is
that paragraph.

It made the served settings a value and stopped at `cmd/`. Two things were
left, and the second was never named at all.

**The Forge and Fly variables.** `internal/sim/tier3` read thirteen of them at
the point of use — `MTGLAB_FORGE_HOME`, `MTGLAB_JAVA`, `MTGLAB_FORGE_PROFILE`,
`MTGLAB_FORGE_WORKER`, `MTGLAB_FORGE_WORKER_URL`, `MTGLAB_FLY_API_TOKEN`,
`MTGLAB_FLY_APP`, `FLY_APP_NAME`, `MTGLAB_FORGE_MACHINE`,
`MTGLAB_FORGE_SHIM_PORT`, `MTGLAB_FORGE_SHIM_TOKEN`,
`MTGLAB_FORGE_IDLE_SECONDS`, `MTGLAB_FORGE_MEMORY_MB` — the last of which was
read through a second, private `envInt` in `cmd/mtglab/shim.go` and had escaped
every previous count.

The package's public functions took a `forgeHome string` that meant "look at
the environment" when it was empty:

```go
func DesktopJar(forgeHome string) (string, error) {
	home := forgeHome
	if home == "" {
		home = ForgeHome()          // os.Getenv, three frames down
	}
```

Every caller in the tree passed `""`. The parameter documented an override
nobody used while hiding a global read behind a signature that looked injected
— the worst of both, because a reader checking whether the code was testable
would have concluded that it was.

**`cmd/mtglab` never got the value at all.** ADR 39 called it "the composition
root", and it was, in the sense that it held the only `config.Load`. But that
`Load` sat behind a helper:

```go
// settings is this process's configuration.
//
// **The one call to [config.Load] in the binary.**
func settings() config.Config { return config.Load() }
```

called from thirty-one places, every one of them inside a `RunE` — which is to
say, at request time, from the environment, once per command. A function that
reads the environment on demand is a global variable wearing a call. The
comment above it was true and beside the point.

**The third global was the one nobody had written down.** Even with the
configuration injected, the CLI tests could not have run in parallel, because
the commands printed with bare `fmt.Printf` and the tests captured them by
swapping `os.Stdout` for a pipe — twenty-one times. `mtglab users` went
further: `readSecret` read `os.Stdin` directly, and a package-level
`stdinLines` cached a `bufio.Reader` over it, rebuilt whenever the file changed
identity. Its own comment gave the reason outright:

> Rebuilt when os.Stdin itself is swapped, **which is how the tests hand each
> command its own pipe.**

A mechanism in shipped code, existing solely to survive a test writing a
process global. That is the shape ADR 39 was about, one layer down.

**The measurement.** The claim inherited into this session was "82 serial tests
in `cmd/mtglab` and `internal/sim/tier3`, some blocked by `t.Setenv`". Neither
half held. The count was 126, and the way to find out which were genuinely
blocked was not to read them: add `t.Parallel()` to each one, run it alone, and
let Go answer.

```
BLOCKED 112      # panics: "can not use t.Parallel"
OK       14
```

**112 of 126**, and not one of them was a test about the environment. They were
tests about listing decks, adding an account, refusing a bad flag.

## Options considered

**Thread `config.Config` through Cobra and stop.** Unblocks the configuration
half. But the tests still swap `os.Stdout`, so not one of the eighty-eight
`cmd/mtglab` tests could actually go parallel — a large diff for zero
parallelism. The two globals had to fall together or neither was worth moving.

**Keep the free functions, drop the empty-string fallback.** Make every
`tier3` path an explicit required argument and leave the type out of it. Small
and honest for the four path settings — and no home at all for the eight Fly
and shim ones, which would have stayed on `os.Getenv` or grown a second
parameter list at every call site.

**A `tier3.Settings` value with the discovery functions as methods.** Mirrors
`config.Config` exactly, which is the pattern this repository already has. The
`Worker` carries one, so the client is configured by the thing that constructed
it rather than by the process it happens to be running in. Chosen.

**Give the commands an `io.Writer` field.** Rejected in favour of
`cmd.OutOrStdout()`, which Cobra has always provided and which propagates from
the root — so a test sets one writer on `newRoot` and every subcommand's output
lands in it, with no field to thread and no field to forget.

## Decision

**The configuration is an argument to the command tree, and the Forge
environment is a value like every other.**

`newRoot(cfg config.Config, forge tier3.Settings, pipe claude.Endpoint)`. The
environment is read exactly three times in the process, all of them on one line
of `main`. `settings()` is gone; there is deliberately no replacement.

`tier3.Settings` carries the thirteen Forge and Fly variables, resolved by
`tier3.LoadSettings`. `ForgeHome`, `JavaBinary`, `DesktopJar`, `ForgeVersion`,
`ForgeProfile`, `EnsureProfile`, `CardsfolderPath`, `ImplementedNames`,
`CheckCoverage`, `RunGames` and `Configured` become methods on it.
`RunOptions.Home` is gone — the distribution is a property of the settings that
run the match, not of one invocation. An override is `Settings.At(home)`, which
is visibly an override and has no value that reaches back into the process.
`api.Config` and `door.Config` gain a `Forge` field beside the `Claude` one
they already had, with the same zero-value contract: a machine with no Forge
answers `available: false` and a reason, which is the state CI is in.

**Every command writes through `cmd.OutOrStdout()` and prompts through
`cmd.InOrStdin()`.** The `prompt` type replaces `readSecret`, `readStdinLine`
and the `stdinLines` cache; the buffered reader is still shared within one
prompt, for the original reason — a second `bufio.Reader` would swallow the
second piped password — but it is shared by being a field rather than by being
global.

**`decks validate` returns `errFailedGate` instead of calling `osExit`.** The
package-level `var osExit = os.Exit` existed so a test could observe the code
rather than die with the process. `main` maps the sentinel to a status and
prints nothing, which is the recorded behaviour, and the deferred closes now
run on the way out — which `os.Exit` skipped.

One test driver, in `cmd/mtglab/clitest_test.go`: a `deployment` value with its
own directories, `run(t, args...)` returning what the command wrote. Twelve
per-family drivers went with it.

## Consequences

**Serial tests in the two packages: 126 → 9.** Tree-wide the suite holds 1,151
parallel tests. `cmd/mtglab` runs in 3.4s where it was one test at a time.

**A test exists now that could not have existed before.**
`TestTheShimTokenRidesOnEveryRequest` stands up two stub shims at once — one
demanding a bearer token, one open — and asserts both. Previously the client
read the shim's address from `MTGLAB_FORGE_WORKER_URL`, so the process had one
slot for it and the test was two sequential halves that each rewrote the
environment. This is the same failure ADR 39 found in `ANTHROPIC_BASE_URL`, in
a package it did not reach.

**One test was silently testing nothing, and the compiler found it.**
`TestTheProfileIsWrittenOnceAndOwnsItsOwnDirectory` set `MTGLAB_FORGE_PROFILE`
and then asserted that `EnsureProfile` wrote its marker there. After the
injection it failed, because the variable no longer steers anything — and the
failure showed that the assertion had been passing on a value the test set and
the code read through a global, rather than on anything the test had handed it.
A test that steers through a process global cannot tell you whether the code
reads its argument.

**What stays serial, and each says so where it stands.** `LoadSettings`'s own
test and the `FLY_APP_NAME` override, which are about the reader. `envOr`,
which is ADR 39's standing Cobra exception — flag defaults for `--web-dist` and
`--tarot` are needed while the command tree is being built, before a `Load`
could have run. Two JVM searches that write `PATH`, which is genuinely the
process's. Six that share the package-level coverage index — guarded, so
`-race` is quiet; it is `ClearIndex` and the hit counters that collide.

And one that the audit could not see: `TestTheServerBootsAnswersAndStopsOnASignal`
sends `SIGTERM` to `os.Getpid()`. It passes alone, so the mechanical check
cleared it — but a second `serve` running beside it would take the same signal,
and the failure would land on the *other* test and read as a flake there.
**A ground-truth audit answers "does Go refuse this", not "is this safe".** The
second question still needs a reading.

**What this costs.** `errcheck` had to learn about `fmt.Fprint*`, since
`fmt.Print*` is on its default ignore list and the injected form is not. The
exclusion is argued in `.golangci.yml` and is narrow by construction: nothing
in this codebase serves an HTTP response through `fmt`, every use outside
`cmd/` writes to a builder that cannot fail, and the door's own writes go
through `internal/wire`, whose errors are checked and stay checked.

`ForgeHome()`, `ForgeProfile()` and the `forgeHome string` parameter are gone
from the package's surface. Nothing outside the module imports it.

**What is left.** `internal/claude`'s `Ceiling` still reads `os.Getenv` at call
time, and `flymetrics.Token()` still reads its own. Both are small, both are
the same shape, and neither blocks a test that is not already about the
environment. They are worth doing; they are not worth pretending this ADR did.
