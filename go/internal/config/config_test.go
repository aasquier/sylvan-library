package config

import (
	"path/filepath"
	"testing"
)

func TestFlagReadsLikeConfigFlag(t *testing.T) {
	for raw, want := range map[string]bool{"1": true, "true": true, " YES ": true, "On": true,
		"0": false, "false": false, "no": false, "maybe": false} {
		t.Setenv("MTGLAB_TEST_FLAG", raw)
		if got := Flag("MTGLAB_TEST_FLAG", true); got != want {
			t.Errorf("Flag(%q) = %v, want %v", raw, got, want)
		}
	}
	t.Setenv("MTGLAB_TEST_FLAG", "")
	if !Flag("MTGLAB_TEST_FLAG", true) || Flag("MTGLAB_TEST_FLAG", false) {
		t.Fatal("a blank flag did not fall back to its default")
	}
}

func TestThePathsDeriveFromTheDataDir(t *testing.T) {
	t.Setenv("MTGLAB_DATA_DIR", "")
	t.Setenv("MTGLAB_DECKS_DIR", "")
	if DataDir() != "data" || DecksDir() != "decks" {
		t.Fatalf("defaults: %s %s", DataDir(), DecksDir())
	}
	if AppDBPath() != filepath.Join("data", "app.db") || DBPath() != filepath.Join("data", "mtg.duckdb") {
		t.Fatalf("%s %s", AppDBPath(), DBPath())
	}
	t.Setenv("MTGLAB_DATA_DIR", "/data")
	t.Setenv("MTGLAB_DECKS_DIR", "/data/decks")
	if DBPath() != "/data/mtg.duckdb" || ScryfallDir() != "/data/scryfall" ||
		CacheDir("symbols") != "/data/cache/symbols" || DecksDir() != "/data/decks" {
		t.Fatalf("%s %s %s", DBPath(), ScryfallDir(), CacheDir("symbols"))
	}
}

func TestSecureCookiesFollowRequireAuthUnlessToldOtherwise(t *testing.T) {
	t.Setenv("MTGLAB_REQUIRE_AUTH", "1")
	t.Setenv("MTGLAB_SECURE_COOKIES", "")
	if !SecureCookies() {
		t.Fatal("auth on should mean secure cookies")
	}
	t.Setenv("MTGLAB_REQUIRE_AUTH", "0")
	if SecureCookies() {
		t.Fatal("auth off should mean plain cookies")
	}
	t.Setenv("MTGLAB_SECURE_COOKIES", "1")
	if !SecureCookies() {
		t.Fatal("an explicit override was ignored")
	}
}
