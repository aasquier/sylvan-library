package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

// The slot argument's route. `internal/claude` already holds the mode's two
// halves to a corpus, and the interview's tests already cover the shape the
// two
// share -- so these are about what is different: the alternatives arriving on
// the wire, and the two SHAPES `alternatives_dropped` has.

const argueAt = "/api/decks/alice/kaheera/argue"

func TestTheArgumentIsTheInterviewsTwinOnRefusals(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	for _, row := range []struct {
		body   string
		status int
		detail string
	}{
		{`{}`, 422, "card is required"},
		{`{"card":"   "}`, 422, "card is required"},
		{`{"card":"Black Lotus"}`, 422, "'Black Lotus' is not in kaheera"},
		// One mapping, shared with the interview: a stance that will not read
		// is the caller's to fix. (502 until 2026-08-23, when the ruling
		// landed -- see refuseClaude.)
		{`{"card":"Sol Ring","stance":"emperor"}`, 422, "is not a stance preset"},
		{`{"card":"Sol Ring"}`, 503, "no ANTHROPIC_API_KEY"},
	} {
		status, payload, raw := callAs(t, a, alice, "POST", argueAt, row.body)
		if status != row.status {
			t.Errorf("%s answered %d, want %d: %s", row.body, status, row.status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, row.detail) {
			t.Errorf("%s said %q, want it to contain %q", row.body, detail, row.detail)
		}
	}
}

func TestTheArgumentIs404ForADeckTheCallerCannotSee(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	if status, _, raw := callAs(t, a, bob, "POST", "/api/decks/bob/kaheera/argue",
		`{"card":"Sol Ring"}`); status != 404 {
		t.Fatalf("%d %s", status, raw)
	}
}

// At `initiative: off` no call is made -- and `alternatives_dropped` carries
// **four** keys, because that is what the recorded no-run default holds. The
// five-key shape only exists once the alternatives have actually been
// resolved.
func TestAtStanceOffTheDroppedBlockHasTheRecordedFourKeys(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", argueAt,
		`{"card":"Sol Ring","stance":"off"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["asked"] != false {
		t.Errorf("asked is %v", payload["asked"])
	}
	dropped, _ := payload["alternatives_dropped"].(map[string]any)
	if len(dropped) != 4 {
		t.Errorf("alternatives_dropped has %d keys (%v); the recorded no-run "+
			"default has four -- already_in_deck is not among them", len(dropped), keysOf(dropped))
	}
	if _, present := dropped["already_in_deck"]; present {
		t.Error("already_in_deck is present on a report where nothing ran; " +
			"the recorded default omits it and a client reading five keys " +
			"here would be reading a shape the wire never sends")
	}
}

// A whole argument, through the marshaller: charges kept and counted,
// alternatives resolved against the real pool, and the five-key dropped block.
func TestTheArgumentReachesTheWireWithItsAlternativesJudged(t *testing.T) {
	api := &scriptedClaude{replies: []string{answer("end_turn", said(
		`{"charges":[`+
			`{"claim":"Ramp is already over target.","ground":"count","fact":"ramp holds 12 of 8-12.","strength":"serious"},`+
			`{"claim":"It is simply bad."},`+
			`{"claim":"Too slow for the curve.","ground":"speed","fact":"average mana value is 3.9.","strength":"vibes"}],`+
			`"alternatives":["Craterhoof Behemoth","Ajani, Nacatl Pariah","Primeval Titan","Not A Real Card"]}`))}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	status, payload, raw := callAs(t, a, alice, "POST", argueAt, `{"card":"Sol Ring"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	want := []string{"answered_by", "mode", "model", "slug", "card", "asked",
		"reason", "stance", "charges", "charges_dropped", "alternatives",
		"alternatives_dropped", "tool_calls", "usage", "never"}
	if err := orderedAs(string(raw), want); err != nil {
		t.Errorf("%v\n%s", err, raw)
	}
	if payload["mode"] != "slot-argument" {
		t.Errorf("mode is %v", payload["mode"])
	}

	// The charge with no `fact` is dropped and COUNTED; the one with a
	// nonsense `strength` is KEPT with a fallback, because a labelling miss is
	// not a reason to throw away a cited argument.
	charges, _ := payload["charges"].([]any)
	if len(charges) != 2 {
		t.Fatalf("%d charges survived, want 2: %v", len(charges), payload["charges"])
	}
	if payload["charges_dropped"] != float64(1) {
		t.Errorf("charges_dropped is %v, want 1", payload["charges_dropped"])
	}
	second, _ := charges[1].(map[string]any)
	if second["strength"] != "minor" {
		t.Errorf("an unknown strength came through as %v, want the `minor` fallback", second["strength"])
	}

	// Rule 2, executable and end to end. Kaheera is mono-green; Ajani's back
	// face is R/W, so it is off-colour -- CLAUDE.md's first recorded error,
	// reported here under the pool's full `A // B` spelling.
	alts, _ := payload["alternatives"].([]any)
	if len(alts) != 1 {
		t.Fatalf("%d alternatives survived, want 1 (Craterhoof): %v", len(alts), payload["alternatives"])
	}
	if first, _ := alts[0].(map[string]any); first["name"] != "Craterhoof Behemoth" {
		t.Errorf("the surviving alternative is %v", first["name"])
	}
	dropped, _ := payload["alternatives_dropped"].(map[string]any)
	if len(dropped) != 5 {
		t.Errorf("a real run's dropped block has %d keys (%v), want five",
			len(dropped), keysOf(dropped))
	}
	for bucket, want := range map[string]string{
		"off_colour":  "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger",
		"banned":      "Primeval Titan",
		"not_in_pool": "Not A Real Card",
	} {
		list, _ := dropped[bucket].([]any)
		if len(list) != 1 || list[0] != want {
			t.Errorf("%s is %v, want [%q] -- each reason is counted apart, "+
				"because `you invented that card` and `that card is "+
				"off-colour` are different failures", bucket, dropped[bucket], want)
		}
	}
	if payload["never"] != claudeArgueNever {
		t.Errorf("the promise is %v", payload["never"])
	}
}

const claudeArgueNever = "This is the case against the card, and only that. " +
	"A card that survives it still needs a rationale, and the rationale is yours to write."

// The cap, which nothing else reaches: a model that returns eight charges gets
// five on the wire. `charges_dropped` counts what FAILED THE PREDICATE, not
// what the cap trimmed -- those are different numbers and only one of them is
// about the model editorialising.
func TestTheCaseIsCappedAtFiveCharges(t *testing.T) {
	items := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		items = append(items, fmt.Sprintf(
			`{"claim":"Charge %d.","ground":"cost","fact":"fact %d","strength":"minor"}`, i, i))
	}
	api := &scriptedClaude{replies: []string{answer("end_turn", said(
		`{"charges":[`+strings.Join(items, ",")+`],"alternatives":[]}`))}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	status, payload, raw := callAs(t, a, alice, "POST", argueAt, `{"card":"Sol Ring"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	charges, _ := payload["charges"].([]any)
	if len(charges) != 5 {
		t.Errorf("%d charges reached the wire, want the cap of 5", len(charges))
	}
	if payload["charges_dropped"] != float64(0) {
		t.Errorf("charges_dropped is %v; the cap is not a drop -- every one of "+
			"these cited a fact", payload["charges_dropped"])
	}
}

// The deck's own cards are not alternatives to anything in it, and "the deck"
// includes the COMMANDER. Goreclaw is green and legal, so without the command
// zone in the in-deck set it sails through every other filter and gets offered
// as a replacement for a card in its own deck.
func TestTheCommanderIsNotOfferedAsAnAlternative(t *testing.T) {
	api := &scriptedClaude{replies: []string{answer("end_turn", said(
		`{"charges":[],"alternatives":["Goreclaw, Terror of Qal Sisma"]}`))}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	status, payload, raw := callAs(t, a, alice, "POST", argueAt, `{"card":"Sol Ring"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if alts, _ := payload["alternatives"].([]any); len(alts) != 0 {
		t.Errorf("the deck's own commander was offered as an alternative: %v", alts)
	}
	dropped, _ := payload["alternatives_dropped"].(map[string]any)
	list, _ := dropped["already_in_deck"].([]any)
	if len(list) != 1 || list[0] != "Goreclaw, Terror of Qal Sisma" {
		t.Errorf("already_in_deck is %v, want the commander", dropped["already_in_deck"])
	}
}

// A tool the argument reaches for sees the caller's own library. `deck_stats`
// is the pick because it is in THIS mode's tool set and needs a source -- the
// interview's version of this test survived a mutation by driving a tool the
// mode does not offer, and the lesson generalises.
func TestTheArgumentsToolsSeeTheCallersOwnLibrary(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("tool_use", `{"type":"tool_use","id":"tu_1","name":"deck_stats","input":{"slug":"kaheera"}}`),
		answer("end_turn", said(`{"charges":[],"alternatives":[]}`)),
	}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	if status, _, raw := callAs(t, a, alice, "POST", argueAt, `{"card":"Sol Ring"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if len(api.requests) != 2 {
		t.Fatalf("%d requests, want 2", len(api.requests))
	}
	sent, err := json.Marshal(api.requests[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sent), "no deck library is reachable") {
		t.Fatal("the tool was handed no source; the route must pass the caller's own")
	}
	// A real deck_stats result names the category buckets. A refusal does not.
	if !strings.Contains(string(sent), "ramp") {
		t.Error("deck_stats did not answer over the caller's library")
	}
}

func TestTheArgumentAsksTheSeatsOwnModel(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"charges":[],"alternatives":[]}`))}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	seated := alice
	seated.ModelTier = "opus"
	if status, _, raw := callAs(t, a, seated, "POST", argueAt, `{"card":"Sol Ring"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if got, want := api.requests[0]["model"], tiers.Resolve("opus"); got != want {
		t.Errorf("the tiered seat was answered by %v, want %q", got, want)
	}
}

func TestTheArgumentRecordsUnderItsOwnMode(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"charges":[],"alternatives":[]}`))}}
	api.start(t)
	rig := newWriteRig(t)
	if status, _, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/kaheera/argue", `{"card":"Sol Ring"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	mode, _, _, _ := oneClaudeRow(t, rig)
	if mode != "slot-argument" {
		t.Errorf("the ledger row is for mode %q", mode)
	}
}

func keysOf(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
