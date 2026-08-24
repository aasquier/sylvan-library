// Package config is where things live on disk and the switches the
// environment sets — read at call time and never bound at start, so a test
// can point the process at a scratch directory and a container at its
// volume without a restart ceremony.
//
// Two deliberate absences. There is no `.env` reader: the door is started
// by the container's CMD or by a developer who exports what it needs, and a
// file the process found by walking up from its working directory is one
// more thing a supervisor has to agree about. And secrets are not here at
// all -- a value we never hold is a value we cannot log.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// DataDir is MTGLAB_DATA_DIR: the pool, the price history, `app.db`.
// Defaults to `data`.
func DataDir() string {
	if v := strings.TrimSpace(os.Getenv("MTGLAB_DATA_DIR")); v != "" {
		return v
	}
	return "data"
}

// DecksDir is MTGLAB_DECKS_DIR: the file tier's `<slug>/deck.yaml`.
// Defaults to `decks`.
func DecksDir() string {
	if v := strings.TrimSpace(os.Getenv("MTGLAB_DECKS_DIR")); v != "" {
		return v
	}
	return "decks"
}

// DBPath is `config.DB_PATH`: the card pool, derived from DataDir and never
// set on its own.
func DBPath() string { return filepath.Join(DataDir(), "mtg.duckdb") }

// AppDBPath is `config.APP_DB_PATH`: the SQLite half (ADR 4).
func AppDBPath() string { return filepath.Join(DataDir(), "app.db") }

// ScryfallDir is `config.SCRYFALL_DIR`: the bulk downloads a refresh ingests.
func ScryfallDir() string { return filepath.Join(DataDir(), "scryfall") }

// CacheDir is `data/cache/<name>` -- the runtime shelves (symbols, ocr,
// cardmotion) live under it.
func CacheDir(name string) string { return filepath.Join(DataDir(), "cache", name) }

// Flag reads an on/off variable the way `config._flag` does: unset or blank
// is the default; otherwise one of 1/true/yes/on, case-insensitively, is on
// and anything else is off.
func Flag(name string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// RequireAuth is MTGLAB_REQUIRE_AUTH: off by default, as a laptop wants.
func RequireAuth() bool { return Flag("MTGLAB_REQUIRE_AUTH", false) }

// SecureCookies is MTGLAB_SECURE_COOKIES, defaulting to RequireAuth: the two
// travel together, since auth on means deployed means TLS.
func SecureCookies() bool { return Flag("MTGLAB_SECURE_COOKIES", RequireAuth()) }

// AdminEmail is MTGLAB_ADMIN_EMAIL, the maintainer's address (ADR 17) --
// empty when unset. Read to resolve the maintainer's *handle*; it must never
// itself reach a response, a log line or a URL.
func AdminEmail() string { return strings.TrimSpace(os.Getenv("MTGLAB_ADMIN_EMAIL")) }

// AdminUsername is MTGLAB_ADMIN_USERNAME, the maintainer's handle when it
// is not to be derived from the address.
func AdminUsername() string { return strings.TrimSpace(os.Getenv("MTGLAB_ADMIN_USERNAME")) }

// BaseURL is where this instance answers, for links that have to work in an
// inbox. MTGLAB_BASE_URL, no trailing slash.
//
// An invite or a reset is a URL somebody clicks days later in another
// application, so it cannot be relative and cannot be guessed from the
// request -- a `Host` header is client-supplied, and building a password-reset
// link out of one is how a reset lands on an attacker's domain. It is
// configuration, and it is **wrong-by-default rather than absent**: the local
// port is what `mtglab ui` serves on, so a laptop needs no setting and a
// deployment that forgets one sends links to localhost, which is visibly
// broken rather than quietly hijackable.
func BaseURL() string {
	if v := strings.TrimRight(strings.TrimSpace(os.Getenv("MTGLAB_BASE_URL")), "/"); v != "" {
		return v
	}
	return "http://127.0.0.1:8765"
}

// EmailFrom is the From address on invites and resets. MTGLAB_EMAIL_FROM.
//
// Resend refuses anything outside a verified sending domain, so this is a
// deploy-day setting rather than a preference (`docs/HOSTING.md` §7). The
// default is only ever seen by the console sender.
func EmailFrom() string {
	if v := strings.TrimSpace(os.Getenv("MTGLAB_EMAIL_FROM")); v != "" {
		return v
	}
	return "mtglab <no-reply@localhost>"
}

// ClientIPHeader names the header a trusted proxy sets to the real client IP,
// or empty.
//
// Unset by default, and deliberately so: rate limiting keyed on a header any
// client can send is rate limiting an attacker opts out of by typing a
// different number. Set this (`Fly-Client-IP`, `X-Forwarded-For`) only when a
// proxy you control is guaranteed to overwrite it on the way in.
func ClientIPHeader() string {
	return strings.TrimSpace(os.Getenv("MTGLAB_CLIENT_IP_HEADER"))
}

// ResendAPIKey is the transactional mail provider's key, read at call time.
//
// The one secret this package names, and it is named rather than held because
// the sender has to put it in a header itself -- unlike ANTHROPIC_API_KEY,
// which the SDK reads on its own and which nothing here should ever copy.
func ResendAPIKey() string { return strings.TrimSpace(os.Getenv("RESEND_API_KEY")) }
