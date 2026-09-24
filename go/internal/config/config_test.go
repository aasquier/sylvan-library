package config

import (
	"os"
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

// environment is a deployment described as a value: the map [LoadFrom] reads
// instead of the process, which is what lets the one test about the whole
// resolution run beside every other.
func environment(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

// TestLoadReadsAnEnvironmentOnce is about [LoadFrom], which is [Load] with
// the lookup handed in. It describes a fully-configured deployment as a map
// and asserts where every one of those values lands — the configrecord gate
// only proves each *name* is documented, never which field it reaches.
func TestLoadReadsAnEnvironmentOnce(t *testing.T) {
	t.Parallel()
	c := LoadFrom(environment(map[string]string{
		"MTGLAB_DATA_DIR":       "/data",
		"MTGLAB_DECKS_DIR":      "/data/decks",
		"MTGLAB_REQUIRE_AUTH":   "1",
		"MTGLAB_SECURE_COOKIES": "",
		"MTGLAB_BASE_URL":       "  https://example.test/  ",
		"MTGLAB_EMAIL_FROM":     "",
		"RESEND_API_KEY":        "re_not_a_real_key",
		// The five night switches, each set to a value no other could pass
		// as, so two wirings swapped in Load land somewhere an assertion
		// notices.
		"MTGLAB_NIGHT_WINDOW":            "23:30-01:30",
		"MTGLAB_NIGHT_ZONE":              "America/Los_Angeles",
		"MTGLAB_NIGHT_BOUTS":             "4",
		"MTGLAB_NIGHT_BOUTS_PER_ACCOUNT": "1",
		"MTGLAB_NIGHT_GAMES":             "7",
		"MTGLAB_ADMIN_EMAIL":             "keeper@example.test",
		"MTGLAB_ADMIN_USERNAME":          "keeper",
		"MTGLAB_CLIENT_IP_HEADER":        "Fly-Client-IP",
	}))
	if c.DataDir != "/data" || c.DecksDir != "/data/decks" {
		t.Errorf("dirs: %+v", c)
	}
	if !c.RequireAuth || !c.SecureCookies {
		t.Error("auth on should have carried secure cookies with it")
	}
	// Both halves of the one line that is not a plain copy: the surrounding
	// whitespace an exported-from-a-shell value carries, then the trailing
	// slash a base URL must not keep.
	if c.BaseURL != "https://example.test" {
		t.Errorf("the trailing slash or the padding survived: %q", c.BaseURL)
	}
	if !c.EmailFromIsDefault() {
		t.Errorf("an unset From address should be the default: %q", c.EmailFrom)
	}
	if c.ResendAPIKey != "re_not_a_real_key" {
		t.Errorf("key: %q", c.ResendAPIKey)
	}
	if c.AdminEmail != "keeper@example.test" || c.AdminUsername != "keeper" ||
		c.ClientIPHeader != "Fly-Client-IP" {
		t.Errorf("the maintainer and proxy settings landed wrong: %+v", c)
	}
	if c.NightWindow != "23:30-01:30" || c.NightZone != "America/Los_Angeles" ||
		c.NightBouts != "4" || c.NightBoutsPerAccount != "1" || c.NightGames != "7" {
		t.Errorf("the night switches landed on the wrong fields: %+v", c)
	}
}

// An environment that says nothing resolves to exactly [Defaults] — the
// laptop's footing, and the assertion that no field quietly reads a variable
// under a name nobody wrote down.
func TestAnEmptyEnvironmentResolvesToTheLaptopsDefaults(t *testing.T) {
	t.Parallel()
	if got := LoadFrom(environment(nil)); got != Defaults() {
		t.Fatalf("a silent environment did not resolve to the defaults: %+v", got)
	}
	// And the door that reads the real process is the same function, so a
	// Load on a machine is a LoadFrom over os.Getenv and nothing else.
	if Load() != LoadFrom(os.Getenv) {
		t.Fatal("Load and LoadFrom(os.Getenv) disagreed about this machine")
	}
}
