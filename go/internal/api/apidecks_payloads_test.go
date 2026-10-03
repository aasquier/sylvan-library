package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
	"github.com/aasquier/sylvan-library/go/internal/sim/karsten"
)

// The payload shapers, asked the questions the library's own decks never ask.
//
// Every one of these is a pure function of a value the engine hands it, and
// every one carries a guard for a shape no fixture deck produces: more than six
// of something, a list that is nil rather than empty, a count past the 99. They
// are the lane's own version of lever 25 -- the refusal at the mapper rather
// than through the route -- and what they buy is not the coverage. A `nil` that
// reaches the wire as `null` where the frontend iterates is this repo's most
// repeated bug shape (`a-fallback-that-reads-as-a-fact`), and these are the
// three places left where only a guard stands between the two.

// The gate's errors and the compiler's unresolved names are both capped at six
// for the sentence that renders them, and an absent list crosses the wire as an
// empty one rather than as nothing at all.
func TestTheDeckCheckCapsItsListsAtSixAndNeverSendsNothing(t *testing.T) {
	t.Parallel()

	// Nine cards, no rationale on any of them, against a pool that has heard of
	// none: `unknown-card` nine times over, which is well past the cap.
	d := &deck.Deck{Slug: "nine", Name: "Nine Strangers", Stage: "curated",
		Commander: []string{"Gyome, Master Chef"}}
	for _, name := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"} {
		d.Cards = append(d.Cards, deck.CardEntry{Name: name, Category: "ramp"})
	}
	verdict := gate.Validate(d, map[string]*pool.CardRecord{}, gate.DefaultSize)
	if len(verdict.Errors()) <= 6 {
		t.Fatalf("the fixture only produces %d errors, which does not reach the cap",
			len(verdict.Errors()))
	}

	report := &compile.Report{DeclaredSize: 100,
		Unresolved: []string{"A", "B", "C", "D", "E", "F", "G", "H", "I"}}
	check := checkPayload(report, verdict)
	if len(check.Errors) != 6 {
		t.Errorf("%d errors crossed the wire, want the six the sentence renders", len(check.Errors))
	}
	if check.ErrorCount != len(verdict.Errors()) {
		t.Errorf("the count says %d where the gate found %d", check.ErrorCount, len(verdict.Errors()))
	}
	if len(check.Unresolved) != 6 {
		t.Errorf("%d unresolved names crossed the wire, want six", len(check.Unresolved))
	}
	if check.UnresolvedCount != 9 {
		t.Errorf("the unresolved count says %d, want all nine", check.UnresolvedCount)
	}

	// And the other end of the same field: a deck that resolved completely has
	// no list at all, and `null` is not the answer.
	clean := checkPayload(&compile.Report{DeclaredSize: 100}, verdict)
	if clean.Unresolved == nil {
		t.Fatal("a deck with nothing unresolved sends a nil list")
	}
	if len(clean.Unresolved) != 0 {
		t.Errorf("it sends %v instead of an empty list", clean.Unresolved)
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		t.Fatal(err)
	}
	if !jsonHas(raw, `"unresolved":[]`) {
		t.Errorf("the wire carries %s", raw)
	}
}

// The mana shelf's two capped lists, asked the same way.
func TestTheShelfPayloadCapsItsCardsAndNeverSendsNothing(t *testing.T) {
	t.Parallel()

	tiers := []karsten.PipTier{
		{Pips: 1, Turn: 2, Need: 20, Have: 9,
			Cards: []string{"a", "b", "c", "d", "e", "f", "g", "h"}},
		// A rung nothing in the deck asks for: no cards at all, and a nil
		// slice is not an empty list.
		{Pips: 3, Turn: 6, Need: 30, Have: 30},
	}
	out := colorsPayload([]karsten.ColorRequirement{{Color: "G", Have: 9, HaveLands: 7, Tiers: tiers}})
	if len(out) != 1 || len(out[0].Tiers) != 2 {
		t.Fatalf("the payload is %+v", out)
	}
	if got := out[0].Tiers[0]; len(got.Cards) != 6 || got.CardCount != 8 {
		t.Errorf("the first rung sent %d of %d cards", len(got.Cards), got.CardCount)
	}
	if got := out[0].Tiers[1]; got.Cards == nil {
		t.Error("a rung nothing demands sends a nil list of cards")
	} else if len(got.Cards) != 0 {
		t.Errorf("it sends %v", got.Cards)
	}

	// `approximated` is the shelf's own version of the same field: absent on a
	// deck whose every demand is exact, and still a list.
	shelf := karsten.Shelf{DeckSize: 100, Lands: 36, Target: 0.9,
		Odds: []karsten.CardOdds{{Name: "Sol Ring", MV: 0}}}
	payload := shelfPayloadFrom("mono-green", &deck.Deck{Name: "Mono Green"}, shelf)
	if payload.Approximated == nil {
		t.Fatal("a shelf with nothing approximated sends a nil list")
	}
	if len(payload.Approximated) != 0 {
		t.Errorf("it sends %v", payload.Approximated)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !jsonHas(raw, `"approximated":[]`) {
		t.Errorf("the wire carries %s", raw)
	}
}

// A plate for a token this pool has no printing of: no picture, and `made_by`
// is a list rather than nothing, because the page iterates it.
func TestATokenPlateWithNoMakerNamedStillSendsAList(t *testing.T) {
	t.Parallel()
	plates := tokenPlates([]pool.TokenMade{{Name: "Food", TypeLine: "Token Artifact — Food"}})
	if len(plates) != 1 {
		t.Fatalf("%d plates", len(plates))
	}
	fields := map[string]any{}
	for _, kv := range plates[0] {
		fields[kv.Key] = kv.Value
	}
	made, ok := fields["made_by"].([]string)
	if !ok {
		t.Fatalf("made_by is %T", fields["made_by"])
	}
	if made == nil || len(made) != 0 {
		t.Errorf("made_by is %v", made)
	}
	if fields["image"] != (*string)(nil) || fields["artist"] != (*string)(nil) {
		t.Errorf("a plate with no printing claims a picture: %v", fields)
	}
	raw, err := json.Marshal(plates[0])
	if err != nil {
		t.Fatal(err)
	}
	if !jsonHas(raw, `"made_by":[]`) {
		t.Errorf("the wire carries %s", raw)
	}
}

// A sweep count above the 99 keeps no spells at all rather than a negative
// number of them. The land sweep's own bounds stop at 60, so nothing on the
// wire reaches this -- which is exactly why the guard is worth a test: `resize`
// is documented as a pure function of the library and the count, and a caller
// that trusts that sentence is entitled to hand it any count.
func TestResizingPastTheNinetyNineKeepsNoSpells(t *testing.T) {
	t.Parallel()
	library := []*sim.Card{
		{Name: "Forest", IsLand: true},
		{Name: "Sol Ring"},
		{Name: "Llanowar Elves"},
	}
	out, err := resize(library, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 120 {
		t.Fatalf("a 120-land deck compiled to %d cards", len(out))
	}
	for _, c := range out {
		if !c.IsLand {
			t.Fatalf("%s survived a count that leaves no room for it", c.Name)
		}
	}

	// And a deck with nothing to cycle is a refusal rather than an empty sweep.
	if _, err := resize([]*sim.Card{{Name: "Sol Ring"}}, 36); err == nil {
		t.Error("a deck with no lands was swept anyway")
	}
}

// A number no float can hold, and a number spelled as a fraction: the two
// shapes a request can carry that `strconv` refuses and the clamp would have
// no chance to bound. Both fall back rather than reaching the engine as zero.
func TestASimulationsNumberFallsBackRatherThanArrivingAsZero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  any
		want int
	}{
		// Valid JSON and not a float64: `Float64` answers out of range.
		{"a number past every float", json.Number("1e999"), 5000},
		{"a fraction spelled out", "4.5", 4},
		{"a word", "lots", 5000},
	}
	for _, tc := range cases {
		if got := simInt(map[string]any{"games": tc.raw}, "games", 5000); got != tc.want {
			t.Errorf("%s: games read %d, want %d", tc.name, got, tc.want)
		}
	}
}

// The baseline a build stashed and nothing can parse reads as `unknown`, which
// is the same answer a deck that has never been built gets -- and deliberately
// so: the next build overwrites it, and `different` would be a claim about a
// deck nobody can see.
func TestABaselineNothingCanParseIsUnknownRatherThanDifferent(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{Slug: "mono-green", Name: "Mono Green"}
	if got := baselineState(d, "cards: [unclosed", true); got != "unknown" {
		t.Errorf("an unparseable snapshot reads %q", got)
	}
	if got := baselineState(d, "", false); got != "unknown" {
		t.Errorf("no snapshot at all reads %q", got)
	}
}

// A job leases the pool the same way a request does, and both of the ways there
// is no pool to lease hand the work a nil connection rather than an error --
// which is what lets a job answer "no pool" in the room's own words instead of
// failing.
func TestAJobsPoolLeaseHandsNilWhenThereIsNoPool(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		api  *API
	}{
		{"an instance built with no pool", New(Config{})},
		// A pool whose file is not there: the lease itself answers ErrNoPool,
		// one layer further in than the nil check above.
		{"a pool whose file has gone", New(Config{
			Pool: pool.New(filepath.Join(t.TempDir(), "absent.duckdb"), slog.Default())})},
	}
	for _, tc := range cases {
		calls := 0
		var handed *pool.Conn
		err := tc.api.leasePool(context.Background(), func(c *pool.Conn) error {
			calls++
			handed = c
			return nil
		})
		if err != nil {
			t.Errorf("%s: the lease failed: %v", tc.name, err)
		}
		if calls != 1 {
			t.Errorf("%s: the work ran %d times", tc.name, calls)
		}
		if handed != nil {
			t.Errorf("%s: the work was handed a connection", tc.name)
		}
	}
}

// jsonHas is a substring check on a marshalled payload, named so the assertions
// above read as questions about the wire rather than about Go strings.
func jsonHas(raw []byte, want string) bool { return strings.Contains(string(raw), want) }
