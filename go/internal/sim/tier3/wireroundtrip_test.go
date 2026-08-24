package tier3

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The encoders the *worker* uses, which nothing on a laptop had ever called.
//
// ADR 35's wire has two ends and they are written in the same file, so it is
// easy to believe it is tested when only half of it is. Every existing test
// here drives the **client**: a stub shim writes bytes by hand, and
// `ReportsFromWire` and `RunFromWire` decode them. The encoding half —
// `ReportsToWire` and `RunToWire` — runs only inside `mtglab forge-shim`
// after a real JVM has played a real match, so on any machine without Forge
// it never runs at all. Both sat at 0%.
//
// That is exactly the half a wire can least afford to guess at, because the
// two ends are deployed separately: the app and the worker are different
// images, and a shim from last week answers an app from today. So what is
// asked here is the property, not the bytes — **encode then decode is the
// identity** — plus the two things the encoders do that a conversion would
// not: they normalise nil to empty, and they carry the Forge version as an
// optional rather than as a blank string.

func TestAReportSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()
	original := []CoverageReport{
		{Slug: "gyome", Checked: 100, Resolved: map[string]string{
			"Gyome, Master Chef": "Gyome, Master Chef",
			"Sol Ring":           "Sol Ring",
		}, Missing: []string{"A Card Forge Lacks"}},
		{Slug: "trostani", Checked: 99, Resolved: map[string]string{}, Missing: []string{}},
	}

	back := ReportsFromWire(ReportsToWire(original))
	if !reflect.DeepEqual(back, original) {
		t.Errorf("a report did not survive the round trip:\nsent: %+v\ngot:  %+v",
			original, back)
	}
}

// **`nil` becomes empty on the way out, and stays empty on the way back.**
// A JSON `null` where the client expects a list is the difference between "no
// cards were missing" and a nil dereference in whatever reads it — and the two
// ends are different images, so the reader is not necessarily the version that
// was written against.
func TestTheEncoderNormalisesNilToEmpty(t *testing.T) {
	t.Parallel()
	wire := ReportsToWire([]CoverageReport{{Slug: "bare", Checked: 0}})
	if len(wire) != 1 {
		t.Fatalf("encoded %d reports, want 1", len(wire))
	}
	if wire[0].Resolved == nil {
		t.Error("a nil resolution map crossed as null")
	}
	if wire[0].Missing == nil {
		t.Error("a nil missing list crossed as null")
	}
	// And it really renders as `{}` and `[]` rather than as `null`, which is
	// the assertion a struct-level check would miss.
	raw, err := json.Marshal(wire[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("a bare report rendered a null: %s", raw)
	}
}

// An empty set of reports encodes as an empty list, never as `null`: the shim
// answers `/coverage` with this, and a client decoding `null` into a slice
// gets a nil it then ranges over expecting reports.
func TestNoReportsEncodesAsAnEmptyList(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(ReportsToWire(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Errorf("no reports encoded as %s, want []", raw)
	}
	if got := ReportsFromWire(nil); got == nil || len(got) != 0 {
		t.Errorf("no reports decoded as %+v, want an empty slice", got)
	}
}

func TestAFinishedRunSurvivesTheRoundTrip(t *testing.T) {
	t.Parallel()
	winner, seat, turns := "gyome", 1, 9
	original := &SimRun{
		Argv: []string{},
		Output: SimOutput{Games: []GameResult{
			{Index: 0, Milliseconds: 12000, Winner: &winner, WinnerSeat: &seat, Turns: &turns},
			{Index: 1, Milliseconds: 8000, Draw: true},
			{Index: 2, Milliseconds: 300000, TimedOut: true},
		}},
		WallSeconds:  30.5,
		Seats:        map[int]string{1: "gyome", 2: "trostani"},
		ForgeVersion: "1.6.50",
	}

	back := RunFromWire(RunToWire(original))
	if !reflect.DeepEqual(back.Games(), original.Games()) {
		t.Errorf("the games did not survive:\nsent: %+v\ngot:  %+v",
			original.Games(), back.Games())
	}
	if !reflect.DeepEqual(back.Seats, original.Seats) {
		t.Errorf("the seats did not survive: %+v", back.Seats)
	}
	if back.WallSeconds != original.WallSeconds {
		t.Errorf("the wall clock became %v", back.WallSeconds)
	}
	// The instrument crosses with the result (ADR 36): ratings mixed across a
	// Forge upgrade would silently blend two judges, so a run that knows which
	// Forge played must not lose it in transit.
	if back.ForgeVersion != "1.6.50" {
		t.Errorf("the Forge version became %q", back.ForgeVersion)
	}
}

// **An unreported version is absent, not blank.** `ForgeVersion` is a pointer
// on the wire precisely so the ledger can tell "this shim did not say" from
// "this shim said empty" — a shim old enough not to send it at all decodes to
// the same nothing.
func TestAnUnreportedForgeVersionCrossesAsAbsentRatherThanBlank(t *testing.T) {
	t.Parallel()
	wire := RunToWire(&SimRun{Output: SimOutput{Games: []GameResult{}},
		Seats: map[int]string{}})
	if wire.ForgeVersion != nil {
		t.Errorf("an unreported version crossed as %q", *wire.ForgeVersion)
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var decoded WireRun
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("the run this encoder wrote will not decode: %v (%s)", err, raw)
	}
	if got := RunFromWire(decoded); got.ForgeVersion != "" {
		t.Errorf("an absent version decoded as %q", got.ForgeVersion)
	}
}

// The whole journey as the shim actually makes it: encode, marshal, unmarshal,
// decode. The two `json` steps are where the seat map's integer keys become
// strings and have to come back, which is what `WireRun.UnmarshalJSON` exists
// for and what a struct-only round trip would step over.
func TestARunCrossesJSONWithItsSeatNumbersIntact(t *testing.T) {
	t.Parallel()
	original := &SimRun{
		Output:      SimOutput{Games: []GameResult{{Index: 0, Milliseconds: 1000}}},
		WallSeconds: 1,
		Seats:       map[int]string{1: "gyome", 2: "trostani", 10: "atla"},
	}
	raw, err := json.Marshal(RunToWire(original))
	if err != nil {
		t.Fatal(err)
	}
	var decoded WireRun
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	back := RunFromWire(decoded)
	if !reflect.DeepEqual(back.Seats, original.Seats) {
		t.Errorf("the seat numbers came back as %+v, want %+v", back.Seats, original.Seats)
	}
	// Seat 10 is the one that catches a string sort standing in for an
	// integer one.
	if back.Seats[10] != "atla" {
		t.Errorf("seat 10 came back as %q", back.Seats[10])
	}
}
