package api

import (
	"net/http"
	"strings"
	"testing"
)

// The board a deck has never kept.
//
// `mono-green-clean`'s file has no `swap_board:` block at all, which was the
// shape of most of the library and the shape nothing in the app could do
// anything with: the section did not render, and the one route that could have
// written to it answered 422. These are the tests for the route that can.

// **A deck with no board gets one, with the card on it.** The whole feature in
// one call.
func TestABoardIsStartedOnADeckThatHasNone(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// The refusal this route exists to answer is still the refusal, which is
	// checked first so that a passing test below cannot be the old behaviour
	// wearing a new route.
	status, _, raw := rig.do(t, alice, "POST", cleanDeck+"/cards",
		`{"name":"Craterhoof Behemoth","category":"payoff","why":"a finisher","to":"swap_board"}`)
	if status == http.StatusOK {
		t.Fatalf("`/cards` scaffolded a board after all: %s", raw)
	}

	status, payload, raw := rig.do(t, alice, "POST", cleanDeck+"/board",
		`{"name":"Craterhoof Behemoth","category":"payoff","why":"the finisher, if the curve can carry it"}`)
	if status != http.StatusOK {
		t.Fatalf("starting a board answered %d: %s", status, raw)
	}
	if into, _ := payload["into"].(string); into != "swap_board" {
		t.Errorf("the response says the card went to %q: %s", into, raw)
	}

	text := rig.text(t)
	at := strings.Index(text, "swap_board")
	if at < 0 {
		t.Fatalf("no board was written:\n%s", text)
	}
	if !strings.Contains(text[at:], "Craterhoof Behemoth") {
		t.Errorf("the card is not on the board:\n%s", text)
	}
	// And it is NOT in the 99, which is the half that matters: a card silently
	// in the deck is one its owner does not know is in the deck.
	if strings.Contains(text[:at], "Craterhoof Behemoth") {
		t.Errorf("the card landed in the 99 as well:\n%s", text)
	}
	// The deck is still 99 cards. A board is beside the deck, not in it.
	if total, _ := payload["total_cards"].(float64); int(total) != 99 {
		t.Errorf("the board changed the deck's size to %v: %s", payload["total_cards"], raw)
	}
}

// **A second card joins the board rather than opening another one**, which is
// what lets the frontend call one route without knowing which kind of deck it
// is holding.
func TestASecondCardJoinsTheBoardTheFirstOneStarted(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	for _, card := range []string{"Craterhoof Behemoth", "Terastodon"} {
		status, _, raw := rig.do(t, alice, "POST", cleanDeck+"/board",
			`{"name":"`+card+`","category":"payoff","why":"weighing it against the curve"}`)
		if status != http.StatusOK {
			t.Fatalf("putting %s on the board answered %d: %s", card, status, raw)
		}
	}
	text := rig.text(t)
	if n := strings.Count(text, "\nswap_board"); n != 1 {
		t.Fatalf("the deck has %d board blocks, not 1:\n%s", n, text)
	}
	at := strings.Index(text, "swap_board")
	for _, card := range []string{"Craterhoof Behemoth", "Terastodon"} {
		if !strings.Contains(text[at:], card) {
			t.Errorf("%s is not on the board:\n%s", card, text)
		}
	}
}

// **The board inherits the gate and the log** rather than remembering to call
// them, which is what every deck write in this package does and the reason
// this route goes out through `answer`.
func TestStartingABoardIsRecordedLikeAnyOtherAdd(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", cleanDeck+"/board",
		`{"name":"Craterhoof Behemoth","category":"payoff","why":"the finisher, if the curve can carry it"}`)
	if status != http.StatusOK {
		t.Fatalf("starting a board answered %d: %s", status, raw)
	}
	// The gate's verdict rides back, which is what makes an edit answerable.
	if _, ok := payload["ok"]; !ok {
		t.Errorf("no gate verdict came back: %s", raw)
	}

	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) == 0 {
		t.Fatal("the edit was not recorded (ADR 28)")
	}
	// The history says where the card went, in the same words a board add
	// through the other route would have used.
	if !strings.Contains(entries[0].Summary, "swap board") {
		t.Errorf("the log entry does not say the card went to the board: %q",
			entries[0].Summary)
	}
	if !strings.Contains(entries[0].Summary, "Craterhoof Behemoth") {
		t.Errorf("the log entry does not name the card: %q", entries[0].Summary)
	}
}

// **Everything the 99 refuses, the board refuses**, and in the same sentence.
// Starting a board is a change to the file's shape; it is not a looser set of
// rules about what may go on one.
func TestTheBoardRefusesWhatTheDeckRefuses(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	for _, tc := range []struct{ name, body, want string }{
		{"a card outside the commander's identity",
			`{"name":"Rhystic Study","category":"card-advantage","why":"weighing a blue splash"}`,
			"outside the commander's"},
		{"a card the pool does not have",
			`{"name":"Not A Real Card","category":"payoff","why":"a reason"}`,
			"not a card"},
		{"a blank rationale on a curated deck",
			`{"name":"Craterhoof Behemoth","category":"payoff","why":"  "}`,
			"`why`"},
		{"a category nobody uses",
			`{"name":"Craterhoof Behemoth","category":"removal","why":"a reason"}`,
			"is not a category"},
		{"a card already in the 99",
			`{"name":"Sol Ring","category":"ramp","why":"a reason"}`,
			"already in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Each case gets its own rig: a refusal must leave the file alone,
			// and sharing one would let an earlier case's write explain a
			// later one's refusal.
			rig := newWriteRig(t, noCredential)
			defer rig.close()

			status, payload, raw := rig.do(t, alice, "POST", cleanDeck+"/board", tc.body)
			if status == http.StatusOK {
				t.Fatalf("it was accepted: %s", raw)
			}
			if detail := fmtDetail(payload); !strings.Contains(detail, tc.want) {
				t.Errorf("the refusal said %q, which does not carry %q", detail, tc.want)
			}
			// **A refused write leaves no board behind.** The shape change and
			// the card go in one write or not at all -- a deck must never be
			// left holding an empty board a failed request opened.
			if strings.Contains(rig.text(t), "swap_board") {
				t.Errorf("a refused write left a board on the deck:\n%s", rig.text(t))
			}
		})
	}
}

// The route is a deck write like the other ten, so it is behind the same
// door: a deck the caller cannot see is absent, not forbidden (ADR 5).
func TestTheBoardRouteIsBehindTheWriteDoor(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, _, raw := rig.do(t, bob, "POST", cleanDeck+"/board",
		`{"name":"Craterhoof Behemoth","category":"payoff","why":"a reason"}`)
	if status == http.StatusOK {
		t.Fatalf("another account wrote alice's deck: %s", raw)
	}
	if strings.Contains(rig.text(t), "swap_board") {
		t.Errorf("a refused caller still opened a board:\n%s", rig.text(t))
	}
}
