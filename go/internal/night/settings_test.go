package night_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/api"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/night"
)

// Every deployment in this file is a struct literal, which is the whole
// point of the switches riding config.Config raw: describing a night takes
// no process to mutate, and every misconfiguration is a sentence to assert
// on rather than a boot to watch.

func TestALaptopGetsNoScheduleAndTheDefaults(t *testing.T) {
	t.Parallel()
	s, err := night.SettingsFromConfig(config.Config{})
	if err != nil {
		t.Fatalf("a config with nothing set should resolve: %v", err)
	}
	if s.Scheduled {
		t.Error("no window means no scheduled nights")
	}
	if s.Zone != nil {
		t.Error("no zone was named, so none should be resolved")
	}
	if s.Bouts != night.DefaultBouts ||
		s.BoutsPerAccount != night.DefaultBoutsPerAccount ||
		s.Games != night.DefaultGames {
		t.Errorf("the caps did not default: %+v", s)
	}
}

func TestAConfiguredNightResolvesWhole(t *testing.T) {
	t.Parallel()
	s, err := night.SettingsFromConfig(config.Config{
		NightWindow:          "23:30-01:30",
		NightZone:            "America/Los_Angeles",
		NightBouts:           "8",
		NightBoutsPerAccount: "3",
		NightGames:           "5",
	})
	if err != nil {
		t.Fatalf("a fully-specified night should resolve: %v", err)
	}
	if !s.Scheduled {
		t.Error("a window was set, so the night is scheduled")
	}
	if s.Zone == nil || s.Zone.String() != "America/Los_Angeles" {
		t.Errorf("zone: %v", s.Zone)
	}
	if s.Window.String() != "23:30-01:30" {
		t.Errorf("window: %v", s.Window)
	}
	if s.Bouts != 8 || s.BoutsPerAccount != 3 || s.Games != 5 {
		t.Errorf("caps: %+v", s)
	}
}

// Each refusal is one sentence naming the switch it is about — the fail-fast
// the raw fields were carried for. The table drives every way an operator
// can get a night wrong, and asserts the sentence would tell them which line
// of their environment to look at.
func TestAMisconfiguredNightRefusesWithASentenceNamingTheSwitch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cfg   config.Config
		names string
	}{
		{"a malformed window",
			config.Config{NightWindow: "midnightish", NightZone: "UTC"},
			"MTGLAB_NIGHT_WINDOW"},
		{"a zero-length window",
			config.Config{NightWindow: "23:00-23:00", NightZone: "UTC"},
			"MTGLAB_NIGHT_WINDOW"},
		{"a window with no zone",
			config.Config{NightWindow: "23:30-01:30"},
			"MTGLAB_NIGHT_ZONE"},
		{"a zone the system does not know",
			config.Config{NightWindow: "23:30-01:30", NightZone: "America/LosAngeles"},
			"MTGLAB_NIGHT_ZONE"},
		// The zone is checked even while the window is off: the typo should
		// surface now, not the evening the window is switched on.
		{"a bad zone parked without a window",
			config.Config{NightZone: "Mare/Tranquillitatis"},
			"MTGLAB_NIGHT_ZONE"},
		{"bouts that are not a number",
			config.Config{NightBouts: "six"},
			"MTGLAB_NIGHT_BOUTS"},
		{"a night of zero bouts",
			config.Config{NightBouts: "0"},
			"MTGLAB_NIGHT_BOUTS"},
		{"a per-account share of nothing",
			config.Config{NightBoutsPerAccount: "0"},
			"MTGLAB_NIGHT_BOUTS_PER_ACCOUNT"},
		{"games that are not a number",
			config.Config{NightGames: "many"},
			"MTGLAB_NIGHT_GAMES"},
		{"zero games",
			config.Config{NightGames: "0"},
			"MTGLAB_NIGHT_GAMES"},
		{"more games than the door allows",
			config.Config{NightGames: "21"},
			"MTGLAB_NIGHT_GAMES"},
	} {
		_, err := night.SettingsFromConfig(tc.cfg)
		if err == nil {
			t.Errorf("%s resolved without complaint", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.names) {
			t.Errorf("%s: the sentence does not name %s: %v", tc.name, tc.names, err)
		}
	}
}

// A valid zone with no window is nights-off, not an error: that is exactly
// the state of an instance whose window was measured but not yet chosen.
func TestAZoneAloneIsJustNightsOff(t *testing.T) {
	t.Parallel()
	s, err := night.SettingsFromConfig(config.Config{NightZone: "America/Los_Angeles"})
	if err != nil {
		t.Fatalf("a zone with no window should resolve: %v", err)
	}
	if s.Scheduled {
		t.Error("a zone alone does not schedule a night")
	}
	if s.Zone == nil {
		t.Error("the zone was named and valid, so it should be carried")
	}
}

// The night's games ceiling is the door's own (`api.ForgeGamesMax`). The
// number is written in both packages because neither may import the other's
// baggage, so this holds the copies equal — a balancing invariant instead of
// a hand-copied list.
func TestTheNightsGamesCeilingIsTheDoors(t *testing.T) {
	t.Parallel()
	if night.GamesMax != api.ForgeGamesMax {
		t.Fatalf("night.GamesMax = %d, api.ForgeGamesMax = %d; the two ceilings must move together",
			night.GamesMax, api.ForgeGamesMax)
	}
}
