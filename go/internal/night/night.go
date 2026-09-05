// Package night is the Coliseum at Night's engine (ADR 46): the rows a
// night's work lives in, the window it runs inside, the settings that open
// it, the seeded deal that decides who fights whom, and the ticker that
// works the card one bout at a time.
//
// The night is the app's first scheduler, and everything here follows from
// one fact about the ground it stands on: merging deploys (ADR 23), so the
// process restarts at any hour, including mid-night. A run's roster and its
// progress are therefore rows in `app.db` (rung 14) rather than anything held
// in memory — every question the runner asks is answered from the [Store],
// which is what makes a restarted process resume the night it was holding
// instead of re-running it or dropping it. Crash-safe by being resumable, not
// by being careful.
//
// Three rules are this package's to hold:
//
//   - **One night at a time.** [Store.StartRun] refuses while a run is
//     unfinished, and the schema itself holds one *scheduled* run per local
//     date (`night_runs_one_per_night`), so a restart that asks again cannot
//     double tonight.
//   - **A settled bout stays settled.** `done`, `failed` and `skipped` are
//     terminal: every transition names the states it may leave, and a write
//     that matched no row is an error rather than a shrug. The one honest
//     ambiguity is recorded as itself — a bout orphaned `playing` by a
//     restart is re-marked `failed` with its `match_id` left NULL, because
//     whether the match recorded is genuinely unknown.
//   - **The record never learns about the night.** A bout carries the
//     `forge_matches` id it produced (ADR 46 decision 7); the ledger gains no
//     marker, and "last night's games" is a join from this side.
//
// Configuration resolves here and nowhere else. The five MTGLAB_NIGHT_*
// switches ride [config.Config] raw, and [SettingsFromConfig] is the one
// reader: it parses them together, applies the defaults, and refuses in a
// plain sentence — a night that quietly ran on the wrong clock, or on a
// silently-substituted default, is the worst version of this feature. The
// boot calls it before the runner starts, so a misconfigured night is a
// refusal to serve rather than a surprise at 23:30.
//
// The actual playing of a bout is deliberately not in this package: the
// [Runner] fights through [BoutPlayer], an interface the route layer
// implements around the same play-and-record core the interactive match
// drives, so nothing here imports `internal/api` and every piece is testable
// against a fake. The same seam carries the two facts of the world the loop
// must respect but cannot own — whether the arena's one lane is busy with a
// person's work, and which decks are the house's — as functions handed in at
// construction.
package night

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/config"
)

// Default values for the settings that have one, applied by
// [SettingsFromConfig] rather than by `config.Load` — the resolver that can
// refuse is the resolver that defaults, so the value exists in one place.
const (
	// DefaultBouts is the cap on bouts in one scheduled night when nobody
	// says: six bouts of ten games is roughly a two-hour window on measured
	// match times, which is the shape ADR 46 sketched.
	DefaultBouts = 6
	// DefaultBoutsPerAccount is one account's share of a scheduled night
	// (ADR 46 decision 5): enough to learn something about two decks, never
	// the whole window.
	DefaultBoutsPerAccount = 2
	// DefaultGames is games per bout when nobody says.
	DefaultGames = 10
	// GamesMax is the ceiling on games per bout — the same one the
	// Coliseum's door enforces on an interactive match (`api.ForgeGamesMax`,
	// and a test holds the two equal, because a copied number is a number
	// that drifts).
	GamesMax = 20
)

// Settings is the night resolved: whether one is scheduled at all, the window
// and zone when it is, and the three caps. Construct with
// [SettingsFromConfig] at the composition root; construct with a struct
// literal in a test.
type Settings struct {
	// Scheduled reports whether a window is configured. False means no
	// scheduled nights — admin-triggered sample runs still work, which is
	// how the first real window gets measured before it gets set.
	Scheduled bool
	// Window is the nightly stretch, meaningful only when Scheduled.
	Window Window
	// Zone is whose wall the window's clock hangs on; nil unless Scheduled.
	Zone *time.Location
	// Bouts caps one scheduled night's total. Samples ignore it.
	Bouts int
	// BoutsPerAccount is one account's share of a scheduled night.
	BoutsPerAccount int
	// Games is how many games one bout plays.
	Games int
}

// SettingsFromConfig resolves the five night switches, or refuses with a
// sentence naming the switch and what it needs. It is the only reader of the
// raw values [config.Config] carries, and the boot treats its error as a
// refusal to serve: every sentence here is the fail-fast the switches were
// promised, said before the night instead of during it.
func SettingsFromConfig(cfg config.Config) (Settings, error) {
	s := Settings{}
	var err error
	if s.Bouts, err = count("MTGLAB_NIGHT_BOUTS", cfg.NightBouts, DefaultBouts); err != nil {
		return Settings{}, err
	}
	if s.BoutsPerAccount, err = count("MTGLAB_NIGHT_BOUTS_PER_ACCOUNT",
		cfg.NightBoutsPerAccount, DefaultBoutsPerAccount); err != nil {
		return Settings{}, err
	}
	if s.Games, err = count("MTGLAB_NIGHT_GAMES", cfg.NightGames, DefaultGames); err != nil {
		return Settings{}, err
	}
	if s.Bouts < 1 {
		return Settings{}, fmt.Errorf(
			"MTGLAB_NIGHT_BOUTS is %d; a night needs at least one bout to be worth opening", s.Bouts)
	}
	if s.BoutsPerAccount < 1 {
		return Settings{}, fmt.Errorf(
			"MTGLAB_NIGHT_BOUTS_PER_ACCOUNT is %d; an account's share of the night is at least one bout",
			s.BoutsPerAccount)
	}
	if s.Games < 1 || s.Games > GamesMax {
		return Settings{}, fmt.Errorf(
			"MTGLAB_NIGHT_GAMES is %d; a bout plays between 1 and %d games, the same ceiling an interactive match has",
			s.Games, GamesMax)
	}
	// The zone is validated whenever it is set, window or no window: an
	// operator who parks a typo here while the window is commented out would
	// otherwise discover it the evening they uncomment the window.
	if cfg.NightZone != "" {
		if s.Zone, err = time.LoadLocation(cfg.NightZone); err != nil {
			return Settings{}, fmt.Errorf(
				"MTGLAB_NIGHT_ZONE %q is not an IANA time zone this system knows (try \"America/Los_Angeles\")",
				cfg.NightZone)
		}
	}
	if cfg.NightWindow == "" {
		return s, nil
	}
	if s.Window, err = ParseWindow(cfg.NightWindow); err != nil {
		return Settings{}, err
	}
	if s.Zone == nil {
		return Settings{}, fmt.Errorf(
			"MTGLAB_NIGHT_ZONE is required when MTGLAB_NIGHT_WINDOW is set: the window is wall-clock time, and the instance must be told whose wall")
	}
	s.Scheduled = true
	return s, nil
}

// count reads one of the three cap switches: empty is the default, anything
// else must be a whole number. Range checks live with the caller, which knows
// what the number is for.
func count(name, raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s is %q, which is not a whole number", name, raw)
	}
	return n, nil
}
