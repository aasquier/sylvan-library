package claude

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// TestTheVoicesAreTheRecordedOnes reads the embedded roster back and checks
// it is the recorded shape — plain first, exactly one of them dealing, and
// every costume carrying a voice long enough to be one.
//
// **No count.** This test began as "expected seven voices" and the number was
// wrong within a session of a voice being added, which is `modes.go`'s lesson
// arriving a second time: a count written into source is a claim to re-check
// rather than a fact, and #257 is the scar where a hand-written list of modes
// silently lost one. What a count was really guarding — an embed that
// truncated, or a persona present in one of the file's two lists and missing
// from the other — is covered below by the roster's key-by-key agreement with
// `PersonaKeys` and by the length floor on each voice. The roster is meant to
// grow (ADR 21 names three more voices), and a test that has to be edited
// every time it does is a test nobody reads.
func TestTheVoicesAreTheRecordedOnes(t *testing.T) {
	t.Parallel()
	if len(PersonaKeys) < 2 {
		t.Fatalf("a grid needs the plain tile and at least one costume; got %v",
			PersonaKeys)
	}
	if PersonaKeys[0] != "plain" || DefaultPersona != "plain" {
		t.Errorf("the plain voice must open the grid and be the default; got %v / %q",
			PersonaKeys, DefaultPersona)
	}
	var deals []string
	for _, k := range PersonaKeys {
		if personas[k].Prop == PropTarot {
			deals = append(deals, k)
		}
	}
	if len(deals) != 1 || deals[0] != "fortune-teller" {
		t.Errorf("only the fortune teller deals; got %v", deals)
	}
	// The plain voice is empty ON PURPOSE — the base instructions already end
	// with a paragraph about how to write, and the default persona is that
	// paragraph. Anything here would be a second opinion about the same thing,
	// and WithVoice's identity branch depends on it.
	if personas["plain"].Voice != "" {
		t.Error("the plain voice must stay empty; it is the no-costume tile")
	}
	for _, k := range PersonaKeys[1:] {
		if len(personas[k].Voice) < 500 {
			t.Errorf("%s's voice is %d bytes — a dull persona is a bug, and a "+
				"truncated one is a broken embed", k, len(personas[k].Voice))
		}
	}
}

// TestTheRosterCannotCarryAVoice is the structural half of the roster's
// contract, and the reason RosterEntry is its own type.
//
// Not because a prompt in a public repository is a secret, but because a
// client that received one would eventually send one back, and "the persona is
// one of a fixed set" is worth keeping structural rather than polite. This
// asserts over the MARSHALLED bytes, so a `Voice` field added to RosterEntry
// with any tag at all fails here.
func TestTheRosterCannotCarryAVoice(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(Roster())
	if err != nil {
		t.Fatalf("marshalling the roster: %v", err)
	}
	if strings.Contains(strings.ToLower(string(body)), "voice") {
		t.Errorf("the roster leaked a voice field:\n%s", body)
	}
	// And the positive half: every persona is present, so the guard above is
	// not passing merely because the roster is empty.
	var got []RosterEntry
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("re-reading the roster: %v", err)
	}
	if len(got) != len(PersonaKeys) {
		t.Fatalf("roster has %d entries, %d personas", len(got), len(PersonaKeys))
	}
	for i, entry := range got {
		if entry.Key != PersonaKeys[i] {
			t.Errorf("roster[%d] is %q, want %q", i, entry.Key, PersonaKeys[i])
		}
		if entry.Label == "" || entry.Blurb == "" {
			t.Errorf("%s has no label or blurb; the tile would render empty", entry.Key)
		}
		// The roster carries the prop, and carries the same one the persona
		// does. The two lists are hand-kept in one file, so this is the join
		// that catches a room dressed on one side of it and bare on the other
		// — a fortune teller whose roster entry forgot the cards would render
		// a tile promising a spread that never arrives, or the reverse.
		if entry.Prop != personas[entry.Key].Prop {
			t.Errorf("%s is served with prop %q and read with prop %q",
				entry.Key, entry.Prop, personas[entry.Key].Prop)
		}
	}
	// A caller that sorts what it is handed must not reorder the tile grid for
	// the next request.
	first := Roster()
	first[0], first[1] = first[1], first[0]
	if Roster()[0].Key != PersonaKeys[0] {
		t.Error("Roster() handed out its own backing array")
	}
}

// TestTheWireCarriesBothSpellingsOfTheProp is the one-release overlap, read
// off the marshalled bytes rather than off the struct.
//
// `prop` is what the client reads now; `deals` is the boolean it replaced, and
// it stays on the wire for exactly as long as a browser tab opened before this
// change might still be talking to this server. The derivation is a method on
// RosterEntry so the two cannot drift, and this is where that is held: a
// hand-written `deals` in the data, or a `prop` renamed without the boolean
// following it, both land here.
func TestTheWireCarriesBothSpellingsOfTheProp(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(Roster())
	if err != nil {
		t.Fatalf("marshalling the roster: %v", err)
	}
	var got []struct {
		Key   string `json:"key"`
		Prop  string `json:"prop"`
		Deals bool   `json:"deals"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("re-reading the roster: %v", err)
	}
	dealt := 0
	for _, entry := range got {
		if entry.Deals != (entry.Prop == PropTarot) {
			t.Errorf("%s is served prop %q with deals %v; the two spellings "+
				"disagree and an old tab would lose the cards",
				entry.Key, entry.Prop, entry.Deals)
		}
		if entry.Deals {
			dealt++
		}
	}
	// Or every assertion above passes on a roster where nobody deals at all.
	if dealt != 1 {
		t.Errorf("%d entries deal on the wire, want exactly the fortune teller", dealt)
	}
}

// TestAnUnknownPropIsInert is what the named prop buys over the boolean: two
// rooms now have something on the table, and neither one's plumbing may reach
// the other's.
//
// Every spelling here is a room both deals have to ignore — a prop nobody has
// written code for, and the four near-misses that would fire if either
// comparison were ever loosened into a fold or a prefix.
//
// **What it deliberately no longer asserts is that an unknown prop loses its
// seed.** `seedFor` asks "is there anything on the table", not "is it cards",
// because the alternative — a list of the props that carry one — is a silent
// no-op the day a third prop lands, and it was exactly that on the day the
// cauldron did: the witch's seed went on the floor and her pot was re-picked
// every turn. A seed is about the *room*; what it picks is the deal's
// business, and that is what the two nil assertions below hold.
func TestAnUnknownPropIsInert(t *testing.T) {
	t.Parallel()
	for _, prop := range []string{"scrying-bowl", "Tarot", "tarot ", "Cauldron", "cauldron "} {
		who := Persona{Key: "somebody", Prop: prop}
		if reading := readingFor(who, big.NewInt(1234)); reading != nil {
			t.Errorf("prop %q: was dealt a spread", prop)
		}
		if pot := potFor(who, big.NewInt(1234)); pot != nil {
			t.Errorf("prop %q: was filled a pot", prop)
		}
	}
	// A room with nothing on the table at all is the one that still drops the
	// seed: there is nothing for it to reproduce.
	bare := Persona{Key: "nobody", Prop: ""}
	seed, err := seedFor(bare, "1234")
	if err != nil {
		t.Errorf("a propless room's seed was refused rather than dropped: %v", err)
	}
	if seed != nil {
		t.Errorf("a propless room kept the seed %v", seed)
	}
	// The positive controls, or the loop above passes for reasons that have
	// nothing to do with the prop. Each prop gets its own object and neither
	// gets the other's.
	for _, tc := range []struct {
		prop  string
		deals bool
		fills bool
	}{
		{PropTarot, true, false},
		{PropCauldron, false, true},
	} {
		who := Persona{Key: "somebody", Prop: tc.prop}
		got, err := seedFor(who, "1234")
		if err != nil || got == nil {
			t.Fatalf("prop %q lost its seed: %v / %v", tc.prop, got, err)
		}
		if dealt := readingFor(who, got) != nil; dealt != tc.deals {
			t.Errorf("prop %q: dealt a spread = %v, want %v", tc.prop, dealt, tc.deals)
		}
		if filled := potFor(who, got) != nil; filled != tc.fills {
			t.Errorf("prop %q: filled a pot = %v, want %v", tc.prop, filled, tc.fills)
		}
	}
}

// TestTheTwoPropsAnswerOnlyTheirOwnRoom walks the real roster rather than
// hand-built personas, because the hand-built ones cannot catch the failure
// that actually happened: `data/personas.json` carries the prop **twice** —
// once on the persona the prompt is built from and once on the roster entry
// the wire serves — and the witch's first landed with only the roster half
// changed. Every frame she was ever handed would have been the plain one.
//
// So this asks the shipped roster: exactly one voice is dealt a spread,
// exactly one is filled a pot, and they are not the same voice.
func TestTheTwoPropsAnswerOnlyTheirOwnRoom(t *testing.T) {
	t.Parallel()
	seed := big.NewInt(1909)
	var dealt, filled []string
	for _, key := range PersonaKeys {
		who, err := GetPersona(key)
		if err != nil {
			t.Fatalf("%s is in PersonaKeys and GetPersona refuses it: %v", key, err)
		}
		if readingFor(who, seed) != nil {
			dealt = append(dealt, key)
		}
		if potFor(who, seed) != nil {
			filled = append(filled, key)
		}
	}
	if len(dealt) != 1 || dealt[0] != "fortune-teller" {
		t.Errorf("dealt a spread: %v, want exactly the fortune teller", dealt)
	}
	if len(filled) != 1 || filled[0] != "witch" {
		t.Errorf("filled a pot: %v, want exactly the witch — and the likeliest "+
			"cause of an empty list is a `prop` changed on the roster entry in "+
			"data/personas.json and not on the persona beside it", filled)
	}
}

// TestTheWitchsFrameNamesHerPotAndRefusesToExplainIt holds the two sentences
// the cauldron arm of `frameFor` exists for, because neither is visible in any
// report and both are the difference between a pot and a progress bar.
//
// `theme_test.go`'s recorded corpus already pins the frame byte for byte; this
// says *why* those bytes, so that a later edit that rewords the block has to
// decide about these two on purpose rather than by accident.
func TestTheWitchsFrameNamesHerPotAndRefusesToExplainIt(t *testing.T) {
	t.Parallel()
	who, err := GetPersona("witch")
	if err != nil {
		t.Fatal(err)
	}
	seed := big.NewInt(1909)
	pot := potFor(who, seed)
	if pot == nil {
		t.Fatal("the witch was filled no pot")
	}
	frame := frameFor(readingFor(who, seed), pot, nil)
	for _, want := range []string{
		// ADR 20's grounded-quote rule, in the room's own vocabulary.
		"Their words are the only thing that ever goes in.",
		// The mechanic stays the interface's to reveal.
		"never read them their own pot",
		// An ingredient is not evidence about anybody.
		"Nothing you set out is evidence of anything about them.",
		// One at a time, or the pot fills before the questions land.
		"ONE AT A TIME",
	} {
		if !strings.Contains(frame, want) {
			t.Errorf("the witch's frame no longer says %q", want)
		}
	}
	// And the pot itself is in it, by name and by place.
	for _, chosen := range pot.Ingredients {
		if !strings.Contains(frame, chosen.Ingredient.Name) {
			t.Errorf("the frame does not name %q, which is in the pot",
				chosen.Ingredient.Name)
		}
		if !strings.Contains(frame, chosen.Position.Name) {
			t.Errorf("the frame does not name the place %q", chosen.Position.Name)
		}
	}
	// The spread's own sentence must not have followed her into the hut.
	if strings.Contains(frame, "dealt three cards") {
		t.Error("the witch's frame carries the fortune teller's spread")
	}
}

// TestTheWitchCitesHerOwnCorpus is `KeepFact`'s second arm: a `cauldron:` id
// resolves to the corpus's own sentence and the model's paraphrase is thrown
// away, exactly as a `tarot:` id already did. An id nobody has is a dropped
// fact rather than a dropped turn.
func TestTheWitchCitesHerOwnCorpus(t *testing.T) {
	t.Parallel()
	lore := reference.Cauldron()
	if len(lore.Facts) == 0 {
		t.Fatal("the cauldron corpus is empty and this test is measuring nothing")
	}
	first := lore.Facts[0]
	for _, spelling := range []string{
		CauldronSource + first.ID,
		strings.ToUpper(CauldronSource + first.ID),
	} {
		fact := KeepFact(map[string]any{
			"text": "the model's own words", "source": spelling,
		}, nil)
		if fact == nil {
			t.Fatalf("%q was dropped", spelling)
		}
		if fact.Text != first.Text {
			t.Errorf("%q: the querent was handed %q, not the corpus's own words",
				spelling, fact.Text)
		}
		if fact.Source != first.Source || fact.URL != "" {
			t.Errorf("%q: credited %q / %q", spelling, fact.Source, fact.URL)
		}
	}
	if fact := KeepFact(map[string]any{
		"text": "x", "source": CauldronSource + "not-a-fact",
	}, nil); fact != nil {
		t.Errorf("an invented cauldron id was kept: %+v", fact)
	}
}

// TestAnUnknownPersonaIsRefusedByName pins the refusal text, which reaches a
// 422 body — and whose recorded quoting is single quotes, never Go's.
func TestAnUnknownPersonaIsRefusedByName(t *testing.T) {
	t.Parallel()
	if _, err := GetPersona(nil); err != nil {
		t.Errorf("nil must be the default, not an error: %v", err)
	}
	for _, bad := range []any{"nope", "", "PLAIN", 7, true} {
		_, err := GetPersona(bad)
		if err == nil {
			t.Errorf("%v was accepted as a persona", bad)
			continue
		}
		msg := err.Error()
		if !strings.HasPrefix(msg, "no persona ") || !strings.Contains(msg, "'plain'") {
			t.Errorf("refusal for %v does not name what there is: %s", bad, msg)
		}
		if strings.Contains(msg, `"plain"`) {
			t.Errorf("refusal uses double quotes where the recorded shape single-quotes: %s", msg)
		}
	}
}

// TestAVoiceIsAppendedNeverSubstituted is ADR 21's claim, asserted as the
// bytes rather than as an intention.
//
// The base instructions must still appear in
// every persona's prompt, verbatim. If a voice ever replaced the
// contract instead of following it, the interview's own rules — one question at
// a time, never propose, every slot quotes them — would become a persona's to
// soften, which is the one thing ADR 21 says a voice may not do.
func TestAVoiceIsAppendedNeverSubstituted(t *testing.T) {
	t.Parallel()
	const base = "## The rules\n\nOne question at a time. Never propose."
	for _, key := range PersonaKeys {
		who := personas[key]
		name, got := WithVoice("theme-conversation", base, who)
		if !strings.HasPrefix(got, base) {
			t.Errorf("%s: the base instructions are not at the front:\n%s", key, got)
		}
		if who.Voice == "" {
			// The default persona gets the base object back unchanged — same
			// name, same bytes. `converse` caches on this block, so a renamed
			// mode for the no-costume tile would be a cache miss per turn.
			if got != base || name != "theme-conversation" {
				t.Errorf("%s changed a mode that has no voice: %q / %q", key, name, got)
			}
			continue
		}
		if !strings.HasSuffix(got, who.Voice) {
			t.Errorf("%s: the voice is not last; recency is worth something", key)
		}
		if name != "theme-conversation:"+key {
			t.Errorf("%s: mode name is %q — each persona needs its own cache "+
				"entry, and a shared name would collide them", key, name)
		}
	}
}
