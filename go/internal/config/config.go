// Package config is `mtglab/config.py` for the Go side: where things live on
// disk and the switches the environment sets, read at call time and never
// bound at start -- the rule that module exists to enforce (`use_paths` in a
// test, a container's volume in production).
//
// Two deliberate differences from the Python module. There is no `.env`
// reader: the door is started by the container's CMD or by a developer who
// exports what the Python server reads from its `.env`, and a file the
// process found by walking up from its working directory is one more thing a
// supervisor has to agree about. And secrets are not here at all, for the
// reason Python's docstring gives -- a value we never hold is a value we
// cannot log.
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
