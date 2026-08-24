package tier3

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// The Forge and Fly half of this process's environment, resolved into a value.
//
// **A value, not a lookup — the second injection ADR 39 named.** ADR 39 moved
// the served configuration into [config.Config] and stopped at this package's
// door, on the honest grounds that Forge's variables are a different concern
// with a different owner. They were left reading [os.Getenv] at the moment
// somebody asked, twelve times across [run.go] and [worker.go], and every cost
// ADR 39 catalogued applied here too.
//
// Two of those costs were specific to this package and worth naming, because
// neither is visible from a test that suffers them:
//
//   - **The empty string meant "look at the environment".** `DesktopJar("")`,
//     `ImplementedNames("")`, `CheckCoverage(decks, "")` — every caller in the
//     tree passed the empty string, so the parameter documented an override
//     nobody used while hiding a global read behind a signature that looked
//     injected. A [Settings] has a resolved [Settings.Home] and no fallback;
//     a caller wanting a different distribution says [Settings.At].
//   - **The worker had one global slot per setting.** `MTGLAB_FORGE_WORKER_URL`
//     is how a test points the client at its own `httptest.Server`, and the
//     process has exactly one of those — so two stub shims could never run at
//     once, and nineteen worker tests were serial for that reason alone.
//
// Nothing here is a secret this package logs. [Settings.FlyAPIToken] and
// [Settings.ShimToken] are carried because the code has to put them in a
// header itself, on the same rule `internal/config` states: a value we never
// hold is a value we cannot leak, and the two we must hold never render.

// Settings is where Forge lives on this machine and how the hosted worker is
// reached. Construct one with [LoadSettings] at the composition root;
// construct one with a struct literal in a test.
//
// Every path field is resolved rather than raw: [LoadSettings] applies the
// defaults, so nothing downstream re-implements "unset means under the user's
// home".
type Settings struct {
	// Home is the unpacked distribution — MTGLAB_FORGE_HOME, or
	// `~/.local/share/mtglab/forge`.
	//
	// It is the only path in this project that points outside both the
	// checkout and its data directory, which is the point: nothing under it
	// may ever be tracked.
	Home string
	// Java is MTGLAB_JAVA: a JVM the operator chose, tried first and still
	// probed. Empty means "search the bundled JDK, then PATH".
	Java string
	// Profile is Forge's user data directory, ours rather than the user's own
	// — MTGLAB_FORGE_PROFILE, or `~/.local/share/mtglab/forge-profile`.
	//
	// Forge defaults to `~/Library/Application Support/Forge` on macOS.
	// Writing generated decks there would mix machine-generated files into
	// whatever the person has saved by hand.
	Profile string
	// BundledJDK is the JDK unpacked beside the distribution, tried after
	// [Settings.Java] and before PATH.
	BundledJDK string

	// ShimHost is MTGLAB_FORGE_SHIM_HOST, the address the worker's door binds.
	// `::` when unset, which is every address the machine has.
	ShimHost string
	// ShimPort is MTGLAB_FORGE_SHIM_PORT, where the shim answers.
	ShimPort int
	// ShimToken is MTGLAB_FORGE_SHIM_TOKEN: the bearer the shim demands and
	// the client sends. Empty means the door is open, which is what a laptop
	// running the shim by hand wants — the private network is already
	// org-scoped and this is the cheap second wall.
	ShimToken string
	// IdleSeconds is MTGLAB_FORGE_IDLE_SECONDS: quiet before the shim exits
	// and its machine stops. Zero means never.
	IdleSeconds int

	// WorkerURL is MTGLAB_FORGE_WORKER_URL — point straight at a running shim
	// and skip the Machines API entirely. How a test drives the real HTTP path,
	// and how a laptop talks to a hand-started shim.
	WorkerURL string
	// WorkerEnabled is MTGLAB_FORGE_WORKER: the dial. Off, the app probes for
	// a local Forge and the rest of this block is never consulted.
	WorkerEnabled bool
	// FlyAPIToken is MTGLAB_FLY_API_TOKEN, a deploy token stored with
	// `fly secrets set`. Never in `fly.toml`; the repo is public.
	FlyAPIToken string
	// FlyApp is which Fly app holds the worker — MTGLAB_FLY_APP, else
	// FLY_APP_NAME, which Fly injects into every machine. The override exists
	// for tests and for talking to the instance from a laptop.
	FlyApp string
	// Machine is MTGLAB_FORGE_MACHINE, the worker machine's name.
	Machine string
	// MemoryMB is MTGLAB_FORGE_MEMORY_MB, the JVM heap ceiling the shim gives
	// a match.
	//
	// Below [MemoryDefault] on purpose: the worker machine has 4GB total and
	// the heap is not the only resident -- metaspace, the card database's
	// off-heap share, and the shim process all live beside it. Measured
	// heads-up games run well inside it.
	MemoryMB int
}

// Default values for the settings that have one.
const (
	// DefaultShimHost is every address the machine has, which is what a
	// process reachable only over a private network wants.
	DefaultShimHost = "::"
	// DefaultShimPort is where the shim answers when nobody says.
	DefaultShimPort = 8080
	// DefaultIdleSeconds is quiet before the shim stops its own machine.
	//
	// Three minutes keeps a follow-up match on a warm machine (the JVM
	// restarts per match either way; this saves the machine start) at a cost
	// of well under a cent.
	DefaultIdleSeconds = 180
	// DefaultMachine is the name the deploy workflow gives the worker.
	DefaultMachine = "forge-worker"
	// DefaultMemoryMB is the heap a hosted match gets.
	DefaultMemoryMB = 3072
)

// Defaults are the settings a laptop gets when it exports nothing.
//
// Named rather than inlined into [LoadSettings] so a test can start from the
// same footing a developer's machine has and change the one field it is about.
func Defaults() Settings {
	base := filepath.Join(homeDir(), ".local", "share", "mtglab")
	return Settings{
		Home:        filepath.Join(base, "forge"),
		Profile:     filepath.Join(base, "forge-profile"),
		BundledJDK:  filepath.Join(base, "jdk-21"),
		ShimHost:    DefaultShimHost,
		ShimPort:    DefaultShimPort,
		IdleSeconds: DefaultIdleSeconds,
		Machine:     DefaultMachine,
		MemoryMB:    DefaultMemoryMB,
	}
}

// LoadSettings reads the environment and resolves it into a [Settings].
//
// **This is the only reader of the Forge and Fly variables**, and `cmd/mtglab`
// calls it once, in `main`. That single reader is what lets a test describe a
// machine with a struct literal instead of mutating the process it runs in.
func LoadSettings() Settings {
	s := Defaults()
	if v := env("MTGLAB_FORGE_HOME"); v != "" {
		s.Home = v
	}
	s.Java = env("MTGLAB_JAVA")
	if v := env("MTGLAB_FORGE_PROFILE"); v != "" {
		s.Profile = v
	}
	if v := env("MTGLAB_FORGE_SHIM_HOST"); v != "" {
		s.ShimHost = v
	}
	s.ShimPort = envInt("MTGLAB_FORGE_SHIM_PORT", DefaultShimPort)
	s.ShimToken = env("MTGLAB_FORGE_SHIM_TOKEN")
	s.IdleSeconds = envInt("MTGLAB_FORGE_IDLE_SECONDS", DefaultIdleSeconds)
	s.WorkerURL = strings.TrimRight(env("MTGLAB_FORGE_WORKER_URL"), "/")
	s.WorkerEnabled = env("MTGLAB_FORGE_WORKER") != ""
	s.FlyAPIToken = env("MTGLAB_FLY_API_TOKEN")
	s.FlyApp = env("MTGLAB_FLY_APP")
	if s.FlyApp == "" {
		s.FlyApp = env("FLY_APP_NAME")
	}
	if v := env("MTGLAB_FORGE_MACHINE"); v != "" {
		s.Machine = v
	}
	s.MemoryMB = envInt("MTGLAB_FORGE_MEMORY_MB", DefaultMemoryMB)
	return s
}

// At is this configuration pointed at a different distribution.
//
// The replacement for the `forgeHome string` parameter that used to mean
// "the environment" when it was empty: an override is now visibly an
// override, and there is no value of it that reaches back into the process.
func (s Settings) At(home string) Settings {
	if home != "" {
		s.Home = home
	}
	return s
}

// Configured reports whether the hosted worker is the way to run Forge here —
// a fact about the environment, decided when it was read.
//
// The gate calls this and answers `available: true` without any network: the
// same contract `/api/claude` set, where configuration is a fact of the
// environment and reachability is discovered when work is actually asked for.
func (s Settings) Configured() bool {
	return s.WorkerURL != "" || (s.WorkerEnabled && s.FlyAPIToken != "")
}

// env is the one read, trimmed the one way — the same rule `internal/config`
// applies, restated rather than imported because nothing else here needs that
// package and a settings type should not drag one in for a two-line helper.
func env(name string) string { return strings.TrimSpace(os.Getenv(name)) }

// envInt is [env] for a number, where unparseable is indistinguishable from
// unset. A port of `banana` is a typo, and falling back to the default is what
// every previous version of this did.
func envInt(name string, fallback int) int {
	if raw := env(name); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
	}
	return fallback
}

// homeDir is the user's home, which is only ever a base for [Defaults].
func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return os.Getenv("HOME")
}
