package tier3

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// `sim/tier3/wire.py`: what crosses the private network between the app and
// the Forge worker.
//
// ADR 35's hosted shape splits Tier 3 across two machines: the app plans a
// match and shapes its results; the worker holds the distribution, the JVM and
// nothing else. This file is the seam — every byte that crosses is defined
// here, in both directions, so the shim (the worker's door) and the worker
// client (the app's side) cannot drift apart without a test noticing.
//
// Two deliberate choices, both inherited:
//
//   - **A deck travels as `deck.yaml` text.** `deck.FromText` is the one
//     parser (ADR 4 relies on that property), so inventing a second JSON
//     encoding of a deck would be a second parser to keep honest. The worker
//     re-parses with the same code the app used to load it.
//   - **Results come back as JSON and are rebuilt into a real [SimRun].** The
//     shape the API consumes is SimRun's — games, seats, wall clock — so the
//     client reconstructs game results rather than teaching the API a parallel
//     result type. A remote match and a local one are the same thing to
//     everything downstream, which is the point.
//
// **Every struct here carries its fields in Python's order**, which is the
// rule the job corpus bought: `encoding/json` sorts a map's keys and a Python
// dict does not, so a payload built from a `map[string]any` comes out
// alphabetised. The one exception is a coverage report's `resolved`, argued
// where it is written.

// DecksToWire is each deck as its own `deck.yaml` text, in seat order.
func DecksToWire(decks []*deck.Deck) ([]string, error) {
	out := make([]string, 0, len(decks))
	for _, d := range decks {
		text, err := d.Dump()
		if err != nil {
			return nil, err
		}
		out = append(out, text)
	}
	return out, nil
}

// DecksFromWire re-parses what crossed.
//
// The slug argument to `FromText` is empty because a wired deck carries its
// own `slug:` — Python passes none either, and a deck that has lost its slug
// would be a deck the ledger could not name.
func DecksFromWire(texts []string) ([]*deck.Deck, error) {
	out := make([]*deck.Deck, 0, len(texts))
	for _, text := range texts {
		d, err := deck.FromText(text, "")
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// WireReport is one [CoverageReport] on the wire, in Python's key order.
//
// `Resolved` is a plain map, and it is the one field in this file whose key
// order is **not** Python's. The argument, stated rather than skipped: it
// crosses between our own two processes and is read by neither — the receiver
// calls [RaiseUnlessCovered], which reads `slug`, `checked` and `missing` and
// nothing else, and the `.dck` that `resolved` exists to write is written on
// the machine that computed it. Ordering it would need an ordered map threaded
// through the whole pre-flight to make a difference nothing can observe.
// `TestResolvedOrderIsUnobservable` pins that claim so the day something
// starts reading it, this comment fails with it.
type WireReport struct {
	Slug     string            `json:"slug"`
	Checked  int               `json:"checked"`
	Resolved map[string]string `json:"resolved"`
	Missing  []string          `json:"missing"`
}

// ReportsToWire encodes the pre-flight.
func ReportsToWire(reports []CoverageReport) []WireReport {
	out := make([]WireReport, 0, len(reports))
	for i := range reports {
		resolved := reports[i].Resolved
		if resolved == nil {
			resolved = map[string]string{}
		}
		missing := reports[i].Missing
		if missing == nil {
			missing = []string{}
		}
		out = append(out, WireReport{Slug: reports[i].Slug,
			Checked: reports[i].Checked, Resolved: resolved, Missing: missing})
	}
	return out
}

// ReportsFromWire decodes it.
func ReportsFromWire(payload []WireReport) []CoverageReport {
	out := make([]CoverageReport, 0, len(payload))
	for _, r := range payload {
		resolved := r.Resolved
		if resolved == nil {
			resolved = map[string]string{}
		}
		out = append(out, CoverageReport{Slug: r.Slug, Checked: r.Checked,
			Resolved: resolved, Missing: r.Missing})
	}
	return out
}

// WireGame is one game, encoded once for both journeys it makes.
//
// A finished run's games cross inside [WireRun]; since the match theater, each
// game also crosses *alone*, on the stream's `{"game": n}` line, the moment it
// ends. One codec for both is what keeps a streamed row and the same row in
// the final tally byte-identical — the drift this file exists to make
// impossible.
type WireGame struct {
	Index        int     `json:"index"`
	Milliseconds int     `json:"milliseconds"`
	Draw         bool    `json:"draw"`
	Winner       *string `json:"winner"`
	WinnerSeat   *int    `json:"winner_seat"`
	Turns        *int    `json:"turns"`
	TimedOut     bool    `json:"timed_out"`
}

// GameToWire encodes one game.
//
// A **conversion** rather than a field-by-field literal, which is stronger
// rather than merely shorter: Go permits it only while the two structs have
// identical fields in identical order (tags are ignored), so adding a field to
// [GameResult] and forgetting [WireGame] is a compile error here instead of a
// field silently dropped on the wire. A literal would have been the quiet
// version of the same mistake.
func GameToWire(g GameResult) WireGame { return WireGame(g) }

// GameFromWire decodes one game, by the same conversion for the same reason.
func GameFromWire(w WireGame) GameResult { return GameResult(w) }

// WireRun is a finished [SimRun], minus what does not travel.
//
// `argv` stays on the worker (a path on another machine is noise here) and
// `coverage` is not repeated — the pre-flight already crossed on its own.
// `startup_seconds` is derived from games and wall clock, so it is not
// serialised; the rebuilt run computes the same number.
// **This struct carries no JSON tags, deliberately.** Both halves of its
// codec are hand-written — [WireRun.MarshalJSON] names the keys in Python's
// order and [WireRun.UnmarshalJSON] reads them through its own inner struct —
// so tags here would be read by nothing while looking authoritative. They were
// here, and a mutation run proved them dead: renaming `forge_version` on the
// field changed no byte in either direction. The names live in those two
// methods, which is where to change them.
type WireRun struct {
	Games []WireGame
	// Seats is written through an ordered encode because JSON keys are
	// strings and a Go map would alphabetise them: `"10"` sorts before
	// `"2"`. Heads-up never reaches ten seats and the CLI's pods reach four,
	// so nothing observable turns on it today — which is exactly when a
	// difference like this is cheap to get right.
	Seats map[int]string
	// WallSeconds renders through `floats.Float` in MarshalJSON below:
	// Python writes `1.0` for a whole-second match and `encoding/json`
	// writes `1`, on a wire a Python shim and a Go app can be on
	// opposite ends of.
	WallSeconds float64
	// ForgeVersion is for the match ledger (ADR 36). Optional in both
	// directions on purpose: an old shim omits it and an old app ignores it,
	// so deploy skew degrades to "not reported" rather than to an error.
	ForgeVersion *string
}

// MarshalJSON writes the run in Python's key order, with `seats` in seat
// order.
func (w WireRun) MarshalJSON() ([]byte, error) {
	games := w.Games
	if games == nil {
		games = []WireGame{}
	}
	seatNumbers := make([]int, 0, len(w.Seats))
	for seat := range w.Seats {
		seatNumbers = append(seatNumbers, seat)
	}
	sort.Ints(seatNumbers)
	seats := make(wire.OrderedMap, 0, len(seatNumbers))
	for _, seat := range seatNumbers {
		seats = append(seats, wire.KV{Key: itoa(seat), Value: w.Seats[seat]})
	}
	return wire.MarshalOrdered([]wire.KV{
		{Key: "games", Value: games},
		{Key: "seats", Value: seats},
		{Key: "wall_seconds", Value: floats.Float(w.WallSeconds)},
		{Key: "forge_version", Value: w.ForgeVersion},
	})
}

// RunToWire encodes a finished run.
func RunToWire(run *SimRun) WireRun {
	games := make([]WireGame, 0, len(run.Games()))
	for _, g := range run.Games() {
		games = append(games, GameToWire(g))
	}
	out := WireRun{Games: games, Seats: run.Seats, WallSeconds: run.WallSeconds}
	if run.ForgeVersion != "" {
		v := run.ForgeVersion
		out.ForgeVersion = &v
	}
	return out
}

// RunFromWire rebuilds a run from what crossed.
func RunFromWire(payload WireRun) *SimRun {
	games := make([]GameResult, 0, len(payload.Games))
	for _, g := range payload.Games {
		games = append(games, GameFromWire(g))
	}
	seats := map[int]string{}
	for seat, slug := range payload.Seats {
		seats[seat] = slug
	}
	run := &SimRun{Argv: []string{}, Output: SimOutput{Games: games},
		WallSeconds: payload.WallSeconds, Seats: seats}
	if payload.ForgeVersion != nil {
		run.ForgeVersion = *payload.ForgeVersion
	}
	return run
}

// UnmarshalJSON reads a run, taking `seats` keys back to integers the way
// `run_from_wire` does (`int(seat)`).
func (w *WireRun) UnmarshalJSON(data []byte) error {
	var raw struct {
		Games        []WireGame        `json:"games"`
		Seats        map[string]string `json:"seats"`
		WallSeconds  float64           `json:"wall_seconds"`
		ForgeVersion *string           `json:"forge_version"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	w.Games = raw.Games
	w.WallSeconds = raw.WallSeconds
	w.ForgeVersion = raw.ForgeVersion
	w.Seats = map[int]string{}
	for seat, slug := range raw.Seats {
		n, err := parseSeat(seat)
		if err != nil {
			return err
		}
		w.Seats[n] = slug
	}
	return nil
}

func itoa(n int) string { return strconv.Itoa(n) }

func parseSeat(s string) (int, error) { return strconv.Atoi(s) }
