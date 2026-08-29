package api

import (
	"strings"
	"testing"
)

// The description route's own tests. `internal/claude` already proves the mode
// -- one call rather than chunked, an empty strategy carried as nothing, the
// stance resolved and clamped -- so what these prove is the layer above it:
// which failure lands on which status, that the report survives the
// marshaller, and above all **that this route leaves the deck alone**.
//
// That last one is why this file exists at all. The same mode, called from the
// intake, writes `strategy` and `themes` into the deck file. Called from here
// it must not, because here the field may already hold a paragraph its owner
// wrote and the only thing that asked for this was a button labelled "ask".
// The test that would have caught the opposite is the one that reads the deck
// back afterwards, so that is the test.

const describeKaheera = "/api/decks/alice/kaheera/describe"

// ---- what is refused, and in what order ---------------------------------

// A deck this caller cannot see is a 404 and never a 403 (ADR 5): a refusal
// that explains would confirm the deck exists.
func TestTheDescriptionIs404ForADeckTheCallerCannotSee(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, _, raw := callAs(t, a, alice, "POST",
		"/api/decks/bob/bobs-private/describe", `{}`)
	if status != 404 {
		t.Fatalf("%d %s", status, raw)
	}
}

// **The one place this route diverges from the interview and the argument.**
// Those two answer anybody who can see a deck, because they are ways of
// thinking about somebody else's list. This one proposes a paragraph for a
// field, and a caller who cannot write the field cannot use the paragraph --
// so it goes through `writeTarget` and a deck she can see and may not edit is
// a 403. Absence hides; refusal explains.
func TestTheDescriptionIs403ForADeckTheCallerCannotWrite(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST",
		"/api/decks/bob/bobs-public/describe", `{}`)
	if status != 403 {
		t.Fatalf("%d %s -- a draft nobody can save is a call nobody can use", status, raw)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "bobs-public") {
		t.Errorf("the refusal does not name the deck: %v", payload)
	}
}

// The body is read before the deck, the order every Claude route here takes.
func TestTheDescriptionRefusesAMalformedBodyBeforeItLooksUpTheDeck(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, _, raw := callAs(t, a, alice, "POST",
		"/api/decks/alice/no-such-deck/describe", ``)
	if status != 422 {
		t.Fatalf("%d %s -- an empty body is refused before the deck is looked up", status, raw)
	}
}

// A stance that will not parse is the caller's mistake, not the call's.
func TestAMalformedStanceRefusesTheDescription(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", describeKaheera,
		`{"stance":"emperor"}`)
	if status != 422 {
		t.Fatalf("%d %s -- the request was wrong, not the call", status, raw)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "'emperor' is not a stance preset") {
		t.Errorf("the refusal was %q; it must be the parser's own sentence", detail)
	}
}

// No key is 503 and not 502: no call was made at all, which is what an
// instance with no credential answers every day.
func TestWithNoKeyTheDescriptionIs503(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", describeKaheera, `{}`)
	if status != 503 {
		t.Fatalf("%d %s", status, raw)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "ANTHROPIC_API_KEY") {
		t.Errorf("the refusal was %q", detail)
	}
}

// ---- what is answered ----------------------------------------------------

// At `initiative: off` no call is made and the report says so -- 200, with a
// reason, on an instance with no key at all. Not an empty paragraph that reads
// as "Claude had nothing to say about your deck".
func TestAtStanceOffTheDescriptionMakesNoCallAndSaysSo(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", describeKaheera, `{"stance":"off"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["asked"] != false {
		t.Errorf("asked is %v, want false", payload["asked"])
	}
	if reason, _ := payload["reason"].(string); reason == "" {
		t.Error("no call was made and the report does not say why")
	}
	// `[]` and never `null`: the client indexes this, and a fallback that
	// reads as a fact is the mistake this repo makes most often.
	if themes, held := payload["themes"].([]any); !held || len(themes) != 0 {
		t.Errorf("themes is %v, want an empty list rather than null", payload["themes"])
	}
}

// The whole report, through the marshaller, in the recorded key order.
func TestTheDescriptionReportReachesTheWire(t *testing.T) {
	t.Parallel()
	stub := &scriptedClaude{replies: []string{answer("end_turn", said(
		`{"strategy":"Cook Food, sacrifice it, drain the table. Slow to start.",`+
			`"themes":["food","aristocrats"],`+
			`"fact":"Gyome makes Food on every death; fourteen sacrifice outlets."}`))}}
	a, done := deckAPI(t, stub.start(t), true)
	defer done()

	status, payload, raw := callAs(t, a, alice, "POST", describeKaheera, `{}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	want := []string{"answered_by", "mode", "slug", "asked", "reason", "stance",
		"strategy", "themes", "fact", "never"}
	if err := orderedAs(string(raw), want); err != nil {
		t.Errorf("%v\n%s", err, raw)
	}
	// ADR 14's third boundary is a field, and it names the system rather than
	// the checkpoint that answered (commandment 10).
	if payload["answered_by"] != "claude" {
		t.Errorf("answered_by is %v", payload["answered_by"])
	}
	if payload["mode"] != "deck-description" {
		t.Errorf("mode is %v -- this route reaches the description mode", payload["mode"])
	}
	if payload["slug"] != "kaheera" {
		t.Errorf("slug is %v", payload["slug"])
	}
	if !strings.HasPrefix(payload["strategy"].(string), "Cook Food") {
		t.Errorf("the strategy did not survive: %v", payload["strategy"])
	}
	themes, _ := payload["themes"].([]any)
	if len(themes) != 2 || themes[0] != "food" {
		t.Errorf("the themes did not survive: %v", payload["themes"])
	}
	// The grounding travels with the draft. A paragraph whose facts are
	// visible is a paragraph somebody can disagree with, which is the whole
	// point of being offered one.
	if fact, _ := payload["fact"].(string); !strings.Contains(fact, "Gyome") {
		t.Errorf("the fact did not survive: %q", fact)
	}
	if never, _ := payload["never"].(string); !strings.Contains(never, "Nothing is saved") {
		t.Errorf("the payload does not carry its own promise: %q", never)
	}
}

// **The load-bearing test.** The same mode called from the intake writes the
// deck's `strategy` and its `themes`; called from here it must not, because
// here the field may already hold a paragraph its owner wrote.
//
// The fixture has one, which is what makes it the right deck to ask about: a
// route that wrote what it was told would replace a sentence a person put
// there, on a button whose label said "ask". Read off the file tier rather
// than through a GET, because the file is what the next build, the primer and
// the shelf all read.
//
// The log is asserted in the same breath (ADR 28). A line saying the
// description was edited, on a deck whose description is what it always was,
// is a lie in the one place this project keeps its history -- and the edit
// that makes the count mean something is made first, so the assertion is not
// 0 == 0 against a recorder that was never wired up.
func TestTheDescriptionRouteLeavesTheDeckAlone(t *testing.T) {
	t.Parallel()
	stub := &scriptedClaude{replies: []string{answer("end_turn", said(
		`{"strategy":"Something else entirely.","themes":["ramp"],"fact":"counts"}`))}}
	rig := newWriteRig(t, stub.start(t))
	defer rig.close()

	// A real edit first: it puts a paragraph in the file that a person typed,
	// and it puts a line in the log that a person's edit earned.
	const typed = "A paragraph a person typed, and the one that has to survive."
	if status, _, raw := rig.do(t, alice, "PATCH", cleanDeck,
		`{"field":"strategy","value":"`+typed+`"}`); status != 200 {
		t.Fatalf("the setup edit answered %d %s", status, raw)
	}
	before := rig.text(t)
	if !strings.Contains(before, typed) {
		t.Fatalf("the setup edit did not land; this test rests on it:\n%s", before)
	}
	history := len(rig.history(t, "mono-green-clean", nil))
	if history == 0 {
		t.Fatal("an edit was made and the log recorded nothing; this test rests on it")
	}

	status, payload, raw := rig.do(t, alice, "POST", cleanDeck+"/describe", `{}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["strategy"] != "Something else entirely." {
		t.Fatalf("the draft did not come back: %v", payload["strategy"])
	}
	if after := rig.text(t); after != before {
		t.Errorf("the deck file changed. This route proposes and the person "+
			"saves.\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
	if after := len(rig.history(t, "mono-green-clean", nil)); after != history {
		t.Errorf("the log grew from %d to %d entries; this route changes no field",
			history, after)
	}
}
