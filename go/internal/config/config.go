// Package config is where things live on disk and the switches the
// environment sets — read once, into a value, and handed to whatever needs
// them.
//
// **A value, not a lookup.** This package used to answer every question by
// reading the environment at the moment it was asked, on the argument that an
// operator could then retune a running process without a restart ceremony.
// That argument was wrong twice over. A container's environment cannot be
// changed without replacing the container, so the reload it bought was
// theoretical; and the cost was not. Reading process-global state at call time
// meant the only way to test any of it was to *write* process-global state,
// which is [testing.T.Setenv], which makes a test unable to run in parallel
// with any other — a hundred and six of them, holding the suite serial. Worse,
// it gave a per-test stub server exactly one global slot to publish its URL
// through, so two stubs could never coexist. The environment is read here, at
// [Load], and nowhere else; everything downstream takes a [Config].
//
// One deliberate absence and one deliberate exception.
//
// There is no `.env` reader: the door is started by the container's CMD or by
// a developer who exports what it needs, and a file the process found by
// walking up from its working directory is one more thing a supervisor has to
// agree about.
//
// And **exactly one secret is named here**, [Config.ResendAPIKey], because the
// mail sender has to put that value in a header itself. Every other credential
// is carried by the component that needs it and never through this package --
// the Anthropic credential by `internal/claude`, the Fly tokens by
// `internal/flymetrics` and `internal/sim/tier3` -- on the rule that a value
// we never hold is a value we cannot log.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Config is this process's settings, resolved. Construct one with [Load] at
// the composition root; construct one with a struct literal in a test.
//
// Every field is a resolved value rather than a raw variable: defaults are
// applied by [Load], so nothing downstream re-implements "unset means data".
type Config struct {
	// DataDir is MTGLAB_DATA_DIR: the pool, the price history, `app.db`.
	DataDir string
	// DecksDir is MTGLAB_DECKS_DIR: the file tier's `<slug>/deck.yaml`.
	DecksDir string
	// RequireAuth is MTGLAB_REQUIRE_AUTH: off by default, as a laptop wants.
	RequireAuth bool
	// SecureCookies is MTGLAB_SECURE_COOKIES, defaulting to RequireAuth: the
	// two travel together, since auth on means deployed means TLS.
	SecureCookies bool
	// AdminEmail is MTGLAB_ADMIN_EMAIL, the maintainer's address (ADR 17) --
	// empty when unset. Read to resolve the maintainer's *handle*; it must
	// never itself reach a response, a log line or a URL.
	AdminEmail string
	// AdminUsername is MTGLAB_ADMIN_USERNAME, the maintainer's handle when it
	// is not to be derived from the address.
	AdminUsername string
	// BaseURL is where this instance answers, for links that have to work in
	// an inbox. MTGLAB_BASE_URL, no trailing slash.
	//
	// An invite or a reset is a URL somebody clicks days later in another
	// application, so it cannot be relative and cannot be guessed from the
	// request -- a `Host` header is client-supplied, and building a
	// password-reset link out of one is how a reset lands on an attacker's
	// domain. It is configuration, and it is **wrong-by-default rather than
	// absent**: the local port is what `mtglab ui` serves on, so a laptop
	// needs no setting and a deployment that forgets one sends links to
	// localhost, which is visibly broken rather than quietly hijackable.
	BaseURL string
	// EmailFrom is the From address on invites and resets. MTGLAB_EMAIL_FROM.
	//
	// Resend refuses anything outside a verified sending domain, so this is a
	// deploy-day setting rather than a preference (`docs/HOSTING.md` §7). The
	// default is only ever seen by the console sender.
	EmailFrom string
	// ClientIPHeader names the header a trusted proxy sets to the real client
	// IP, or empty.
	//
	// Unset by default, and deliberately so: rate limiting keyed on a header
	// any client can send is rate limiting an attacker opts out of by typing a
	// different number. Set this (`Fly-Client-IP`, `X-Forwarded-For`) only
	// when a proxy you control is guaranteed to overwrite it on the way in.
	ClientIPHeader string
	// ResendAPIKey is the transactional mail provider's key.
	//
	// The one secret this package carries, because the sender has to put it in
	// a header itself.
	ResendAPIKey string
}

// Defaults are the settings a laptop gets when it exports nothing.
//
// Named rather than inlined into [Load] so a test can start from the same
// footing a developer's machine has, and change the one field it is about.
func Defaults() Config {
	return Config{
		DataDir:   DefaultDataDir,
		DecksDir:  DefaultDecksDir,
		BaseURL:   DefaultBaseURL,
		EmailFrom: DefaultEmailFrom,
	}
}

// Default values for the settings that have one.
const (
	// DefaultDataDir is where the pool and `app.db` live when nobody says.
	DefaultDataDir = "data"
	// DefaultDecksDir is the file tier's root when nobody says.
	DefaultDecksDir = "decks"
	// DefaultBaseURL is where `mtglab ui` serves, and so the wrong-by-default
	// value [Config.BaseURL] documents.
	//
	// Named rather than inlined because the boot summary has to be able to
	// *ask* whether this instance is still on it -- a deployment that never
	// set MTGLAB_BASE_URL mails links to a loopback address, and the first
	// person to find that out should not be someone waiting on a password
	// reset.
	DefaultBaseURL = "http://127.0.0.1:8765"
	// DefaultEmailFrom is the console sender's From line, and a value Resend
	// will refuse: `localhost` is nobody's verified sending domain.
	DefaultEmailFrom = "mtglab <no-reply@localhost>"
)

// Load reads the environment and resolves it into a [Config].
//
// **This is the only reader of the settings on [Config]**, and `cmd/mtglab`
// calls it once per command, at the top. That single reader is what keeps a
// test from having to mutate the process in order to describe a deployment.
//
// Three things in the tree still read the environment on their own, and each
// is deliberate rather than missed. `cmd/mtglab`'s `envOr` supplies *flag
// defaults* for `--web-dist` and `--tarot`, which Cobra needs while it is
// building the command tree -- before a `Load` could have run. The Forge shim
// (`cmd/mtglab/shim.go`) and `internal/sim/tier3` read the Forge variables,
// and `internal/claude` reads the Anthropic ones; both are the second
// injection ADR 39 names and does not attempt.
func Load() Config {
	c := Defaults()
	if v := env("MTGLAB_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := env("MTGLAB_DECKS_DIR"); v != "" {
		c.DecksDir = v
	}
	c.RequireAuth = ParseFlag(env("MTGLAB_REQUIRE_AUTH"), false)
	// Secure cookies follow auth unless explicitly told otherwise, which is
	// why this reads the flag it just resolved rather than the variable again.
	c.SecureCookies = ParseFlag(env("MTGLAB_SECURE_COOKIES"), c.RequireAuth)
	c.AdminEmail = env("MTGLAB_ADMIN_EMAIL")
	c.AdminUsername = env("MTGLAB_ADMIN_USERNAME")
	if v := strings.TrimRight(env("MTGLAB_BASE_URL"), "/"); v != "" {
		c.BaseURL = v
	}
	if v := env("MTGLAB_EMAIL_FROM"); v != "" {
		c.EmailFrom = v
	}
	c.ClientIPHeader = env("MTGLAB_CLIENT_IP_HEADER")
	c.ResendAPIKey = env("RESEND_API_KEY")
	return c
}

// env is the one read, trimmed the one way.
func env(name string) string { return strings.TrimSpace(os.Getenv(name)) }

// ParseFlag reads an on/off setting the way `config._flag` does: blank is the
// default; otherwise one of 1/true/yes/on, case-insensitively, is on and
// anything else is off.
//
// Takes the raw string rather than a variable name, so the rule can be tested
// without a process to set it on.
func ParseFlag(raw string, fallback bool) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// DBPath is the card pool, derived from DataDir and never set on its own.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "mtg.duckdb") }

// AppDBPath is the SQLite half (ADR 4).
func (c Config) AppDBPath() string { return filepath.Join(c.DataDir, "app.db") }

// ScryfallDir is where a refresh parks the bulk downloads it ingests.
func (c Config) ScryfallDir() string { return filepath.Join(c.DataDir, "scryfall") }

// CacheDir is `<data>/cache/<name>` -- the runtime shelves (symbols, ocr,
// cardmotion) live under it.
func (c Config) CacheDir(name string) string { return filepath.Join(c.DataDir, "cache", name) }

// BaseURLIsDefault reports whether links this instance mails will point at the
// local port. True when the variable is unset, and equally true when somebody
// set it to the loopback address on purpose -- which is the same fact.
func (c Config) BaseURLIsDefault() bool { return c.BaseURL == DefaultBaseURL }

// EmailFromIsDefault reports whether the From address is still the one no
// provider will accept -- the boot summary's question, for the same reason
// [Config.BaseURLIsDefault] exists.
func (c Config) EmailFromIsDefault() bool { return c.EmailFrom == DefaultEmailFrom }
