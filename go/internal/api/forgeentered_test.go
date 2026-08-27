package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// How a permanent arrived survives the shaping, and an unanswered one stays
// unanswered.
//
// **This seam drops fields silently and that is its whole risk.** [forgeBeat]
// is a hand-written projection of [tier3.GameEvent] — every field is copied by
// name in one composite literal — so a field added to the parser reaches the
// browser only if somebody also added it here. Nothing fails when they do not:
// the wire simply carries less, the room draws what it can, and the feature is
// gone with a green suite behind it.
//
// The two beats below are the two halves of the distinction. A Bronzehide Lion
// was cast, and the room has already shown somebody paying for it. An End-Raze
// Forerunners was **put** onto the battlefield off an Atla Palani egg, and
// until this field existed the two were drawn identically. Both cards are
// verbatim from a real match — Atla Palani/Dinosaurs against Arahbo/Cats, seed
// 11, 2026-08-27.
func TestHowAPermanentArrivedReachesTheRoom(t *testing.T) {
	t.Parallel()
	shaped := newForgeBeats(tier3.EventLog{Game: 1, Events: []tier3.GameEvent{
		{Kind: tier3.EventEnters, Turn: 3, Seat: 2, ID: 136,
			Card: "Bronzehide Lion", Entered: "cast"},
		{Kind: tier3.EventEnters, Turn: 9, Seat: 1, ID: 99,
			Card: "End-Raze Forerunners", Entered: "put"},
		// The third state: a match played by the prose parser, or by a worker
		// image built before the scribe learned to ask. It is not a `put`.
		{Kind: tier3.EventEnters, Turn: 9, Seat: 1, ID: 42,
			Card: "Brimaz, King of Oreskos"},
		// An eminence trigger, for the other half of this change: the target
		// is the creature it made bigger, and it rides the same projection.
		{Kind: tier3.EventAbility, Turn: 3, Seat: 2, ID: 203,
			Card: "Arahbo, Roar of the World", Zone: "Command",
			Trigger: true, Target: "Bronzehide Lion"},
	}}, map[int]string{1: "atla-palani-dinos", 2: "arahbo-cats"}, nil, nil)

	if len(shaped.Beats) != 4 {
		t.Fatalf("%d beats crossed, want 4", len(shaped.Beats))
	}
	for i, want := range []string{"cast", "put", ""} {
		if got := shaped.Beats[i].Entered; got != want {
			t.Errorf("beat %d (%s) entered %q, want %q", i,
				shaped.Beats[i].Card, got, want)
		}
	}
	if got := shaped.Beats[3].Target; got != "Bronzehide Lion" {
		t.Errorf("the eminence beat was aimed at %q, want %q — the target is "+
			"what makes a commander in the command zone legible", got,
			"Bronzehide Lion")
	}

	// **Rendered, not read.** `omitempty` is what makes the third state
	// travel as an absence rather than as an empty string, and reading the
	// struct field cannot tell the two apart. A room distinguishes "put" from
	// "nobody said" on this key being missing.
	raw, err := json.Marshal(shaped.Beats)
	if err != nil {
		t.Fatal(err)
	}
	rendered := string(raw)
	for _, want := range []string{`"entered":"cast"`, `"entered":"put"`} {
		if !strings.Contains(rendered, want) {
			t.Errorf("the beats rendered without %s:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, `"entered":""`) {
		t.Errorf("a beat nobody answered rendered an empty `entered` rather "+
			"than omitting the key; a room cannot tell that from a `put`:\n%s",
			rendered)
	}
}
