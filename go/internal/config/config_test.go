package config

import (
	"path/filepath"
	"testing"
)

func TestParseFlagReadsLikeConfigFlag(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]bool{
		"1": true, "true": true, " YES ": true, "On": true,
		"0": false, "false": false, "no": false, "maybe": false,
	} {
		if got := ParseFlag(raw, true); got != want {
			t.Errorf("ParseFlag(%q, true) = %v, want %v", raw, got, want)
		}
	}
	if !ParseFlag("", true) || ParseFlag("", false) {
		t.Fatal("a blank flag did not fall back to its default")
	}
}

func TestThePathsDeriveFromTheDataDir(t *testing.T) {
	t.Parallel()
	def := Defaults()
	if def.DataDir != "data" || def.DecksDir != "decks" {
		t.Fatalf("defaults: %s %s", def.DataDir, def.DecksDir)
	}
	if def.AppDBPath() != filepath.Join("data", "app.db") ||
		def.DBPath() != filepath.Join("data", "mtg.duckdb") {
		t.Fatalf("%s %s", def.AppDBPath(), def.DBPath())
	}

	c := Config{DataDir: "/data", DecksDir: "/data/decks"}
	if c.DBPath() != "/data/mtg.duckdb" || c.ScryfallDir() != "/data/scryfall" ||
		c.CacheDir("symbols") != "/data/cache/symbols" || c.DecksDir != "/data/decks" {
		t.Fatalf("%s %s %s", c.DBPath(), c.ScryfallDir(), c.CacheDir("symbols"))
	}
}

// TestSecureCookiesFollowRequireAuthUnlessToldOtherwise is about the rule
// [Load] applies, so it states the rule as inputs rather than as a process to
// mutate: three deployments, described at once, asserted in parallel.
func TestSecureCookiesFollowRequireAuthUnlessToldOtherwise(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		requireAuth bool
		raw         string
		want        bool
	}{
		{"auth on means secure cookies", true, "", true},
		{"auth off means plain cookies", false, "", false},
		{"an explicit override wins over auth off", false, "1", true},
		{"an explicit refusal wins over auth on", true, "0", false},
	} {
		if got := ParseFlag(tc.raw, tc.requireAuth); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTheDefaultsAreTheOnesTheBootSummaryAsksAbout(t *testing.T) {
	t.Parallel()
	def := Defaults()
	if !def.BaseURLIsDefault() {
		t.Error("a laptop's base URL should read as the default one")
	}
	if !def.EmailFromIsDefault() {
		t.Error("a laptop's From address should read as the default one")
	}
	deployed := Config{BaseURL: "https://example.test", EmailFrom: "a <b@example.test>"}
	if deployed.BaseURLIsDefault() || deployed.EmailFromIsDefault() {
		t.Error("a configured deployment still read as default")
	}
	// The loopback check is about the *value*, not about the variable being
	// unset: an instance told to mail links to the local port has the same
	// problem as one that was never told anything.
	if !(Config{BaseURL: DefaultBaseURL}).BaseURLIsDefault() {
		t.Error("an explicit loopback base URL did not read as the default")
	}
}

// A laptop that exports nothing schedules no nights: the five night switches
// come off [Defaults] empty, because their resolution -- defaults included --
// belongs to `internal/night`'s SettingsFromConfig, the one reader allowed to
// refuse. A default written here as well would be the same value in two
// places, one of them silently.
func TestTheNightSwitchesArriveRawAndEmpty(t *testing.T) {
	t.Parallel()
	def := Defaults()
	if def.NightWindow != "" || def.NightZone != "" || def.NightBouts != "" ||
		def.NightBoutsPerAccount != "" || def.NightGames != "" {
		t.Fatalf("a laptop's night switches should all be unset: %+v", def)
	}
}

// TestLoadReadsTheEnvironmentOnce is the one test in this package that touches
// the process, because [Load] is the one function that reads it. It cannot be
// parallel, and that is the whole point of it being alone: everything else
// here describes a Config instead of setting one up.
func TestLoadReadsTheEnvironmentOnce(t *testing.T) {
	t.Setenv("MTGLAB_DATA_DIR", "/data")
	t.Setenv("MTGLAB_DECKS_DIR", "/data/decks")
	t.Setenv("MTGLAB_REQUIRE_AUTH", "1")
	t.Setenv("MTGLAB_SECURE_COOKIES", "")
	t.Setenv("MTGLAB_BASE_URL", "https://example.test/")
	t.Setenv("MTGLAB_EMAIL_FROM", "")
	t.Setenv("RESEND_API_KEY", "re_not_a_real_key")

	c := Load()
	if c.DataDir != "/data" || c.DecksDir != "/data/decks" {
		t.Errorf("dirs: %+v", c)
	}
	if !c.RequireAuth || !c.SecureCookies {
		t.Error("auth on should have carried secure cookies with it")
	}
	if c.BaseURL != "https://example.test" {
		t.Errorf("the trailing slash survived: %q", c.BaseURL)
	}
	if !c.EmailFromIsDefault() {
		t.Errorf("an unset From address should be the default: %q", c.EmailFrom)
	}
	if c.ResendAPIKey != "re_not_a_real_key" {
		t.Errorf("key: %q", c.ResendAPIKey)
	}
}
