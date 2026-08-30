package gate_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The combos block is a reading *of* the deck, so everything the gate has to
// say about one is a warning. Held here rather than argued in a comment,
// because "warnings, never errors" is the sort of claim that rots into an
// error the first time somebody thinks a check looks important.

// A deck with three walls, catalogued. Small on purpose: every issue below is
// about the combos block, and the deck-size and rationale errors that a
// four-card deck also raises are simply not what is being read.
func catalogued(combos ...deck.Combo) *deck.Deck {
	return &deck.Deck{
		Slug: "walls", Name: "Walls", Status: "theoretical", Stage: "curated",
		Commander: []string{"Arcades, the Strategist"},
		Cards: []deck.CardEntry{
			{Name: "Axebane Guardian", Category: "ramp", Why: "Taps for defenders.", Qty: 1},
			{Name: "High Alert", Category: "engine", Why: "Walls swing.", Qty: 1},
			{Name: "Suspicious Bookcase", Category: "utility", Why: "A body.", Qty: 1},
		},
		Combos: combos,
	}
}

// Which combo issues a deck raises, and at what level.
func comboIssues(t *testing.T, d *deck.Deck, cards map[string]*pool.CardRecord) []gate.Issue {
	t.Helper()
	out := []gate.Issue{}
	for _, issue := range gate.Validate(d, cards, gate.DefaultSize).Issues {
		if strings.HasPrefix(issue.Code, "combo-") {
			if issue.Level != "warn" {
				t.Errorf("%s is level %q; a combos block never blocks a deck",
					issue.Code, issue.Level)
			}
			out = append(out, issue)
		}
	}
	return out
}

// Held together so the four structural readings are one table: each is a way a
// catalogue drifts away from the deck beside it, and each names the card to
// look at rather than the entry.
func TestTheGateReadsACatalogueAgainstTheDeckBesideIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		combo deck.Combo
		code  string
		card  string
		says  string
	}{
		{
			name: "a piece that was cut afterwards",
			combo: deck.Combo{Cards: []string{"Axebane Guardian", "Freed from the Real"},
				Produces: "infinite mana"},
			code: "combo-piece-missing", card: "Freed from the Real",
			says: "is not in the 99",
		},
		{
			name: "a card it is waiting for that already arrived",
			combo: deck.Combo{Cards: []string{"Axebane Guardian"},
				Needs: "High Alert", Cut: "Suspicious Bookcase", Produces: "infinite mana"},
			code: "combo-needs-in-99", card: "High Alert",
			says: "this machine is complete",
		},
		{
			name: "a card to bring in with no slot for it",
			combo: deck.Combo{Cards: []string{"Axebane Guardian"},
				Needs: "Umbral Mantle", Produces: "infinite mana"},
			code: "combo-no-cut", card: "Umbral Mantle",
			says: "only a suggestion once there is a slot",
		},
		{
			name: "a cut of a card the deck does not have",
			combo: deck.Combo{Cards: []string{"Axebane Guardian"},
				Needs: "Umbral Mantle", Cut: "Wall of Omens", Produces: "infinite mana"},
			code: "combo-cut-missing", card: "Wall of Omens",
			says: "already free",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := comboIssues(t, catalogued(tc.combo), nil)
			if len(got) != 1 {
				t.Fatalf("expected one combo warning, got %+v", got)
			}
			if got[0].Code != tc.code {
				t.Errorf("code is %q, want %q", got[0].Code, tc.code)
			}
			if got[0].Card == nil || *got[0].Card != tc.card {
				t.Errorf("the warning names %v, want %q", got[0].Card, tc.card)
			}
			if !strings.Contains(got[0].Message, tc.says) {
				t.Errorf("the message does not say %q: %s", tc.says, got[0].Message)
			}
		})
	}
}

// A machine whose pieces are all sleeved raises nothing, and neither does a
// near-miss that names its trade honestly. The gate is quiet about a catalogue
// that is true.
func TestAnHonestCatalogueRaisesNothing(t *testing.T) {
	t.Parallel()
	d := catalogued(
		deck.Combo{Cards: []string{"Axebane Guardian", "High Alert"},
			Produces: "infinite colored mana", How: "1) Tap. 2) Untap."},
		deck.Combo{Cards: []string{"Axebane Guardian"}, Needs: "Umbral Mantle",
			Cut: "Suspicious Bookcase", Produces: "infinite colored mana"},
	)
	if got := comboIssues(t, d, nil); len(got) != 0 {
		t.Errorf("a true catalogue was warned about: %+v", got)
	}
}

// The commander is a piece like any other. It sits outside the 99 and half the
// combos in Commander run through it, so a check that only read `cards:` would
// warn about every one of them.
func TestTheCommanderCountsAsAPieceTheDeckHas(t *testing.T) {
	t.Parallel()
	d := catalogued(deck.Combo{
		Cards:    []string{"Arcades, the Strategist", "High Alert"},
		Produces: "walls that draw cards and swing",
	})
	if got := comboIssues(t, d, nil); len(got) != 0 {
		t.Errorf("the commander was read as a card the deck does not have: %+v", got)
	}
}

// A name nobody can look up costs the entry its picture and nothing else, so
// it is a warning where the same absence in the 99 is an error. Driven with an
// empty map rather than nil: nil means the pool was never consulted, and the
// card-level checks do not run at all.
func TestANameThePoolDoesNotKnowIsAWarningRatherThanAnError(t *testing.T) {
	t.Parallel()
	d := catalogued(deck.Combo{
		Cards: []string{"Axebane Guardian", "Hgih Alert"}, Produces: "a typo"})
	got := comboIssues(t, d, map[string]*pool.CardRecord{})
	unknown := []gate.Issue{}
	for _, issue := range got {
		if issue.Code == "combo-unknown-card" {
			unknown = append(unknown, issue)
		}
	}
	// Every name in the entry is unresolvable against an empty pool, so both
	// are reported -- the point is that each is reported once, by name.
	if len(unknown) != 2 {
		t.Fatalf("expected both names reported, got %+v", got)
	}
	if unknown[1].Card == nil || *unknown[1].Card != "Hgih Alert" {
		t.Errorf("the misspelling is not named: %+v", unknown[1])
	}
	if !strings.Contains(unknown[1].Message, "no card to show") {
		t.Errorf("the message does not say what is actually lost: %s", unknown[1].Message)
	}
}

// One name, one warning, however many entries mention it. `ComboNames` is
// deduplicated for exactly this: a piece shared by three machines is one
// spelling to fix, not three rows saying so.
func TestASharedPieceIsReportedOnce(t *testing.T) {
	t.Parallel()
	d := catalogued(
		deck.Combo{Cards: []string{"Axebane Guardian"}, Produces: "mana"},
		deck.Combo{Cards: []string{"axebane guardian", "High Alert"}, Produces: "more mana"},
	)
	n := 0
	for _, issue := range comboIssues(t, d, map[string]*pool.CardRecord{}) {
		if issue.Code == "combo-unknown-card" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("expected two names reported once each, got %d", n)
	}
}
