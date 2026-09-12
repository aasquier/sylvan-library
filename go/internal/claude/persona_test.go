package claude

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"
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

// TestAnUnknownPropIsInert is what the named prop buys over the boolean: the
// room after this one has a cauldron on the table, and adding it must not
// reach the deal.
//
// Every spelling here is a room the tarot plumbing has to ignore — the empty
// prop, a prop nobody has written code for, and the two near-misses that would
// fire if the comparison were ever loosened into a fold or a prefix.
func TestAnUnknownPropIsInert(t *testing.T) {
	t.Parallel()
	for _, prop := range []string{"", "cauldron", "Tarot", "tarot "} {
		who := Persona{Key: "somebody", Prop: prop}
		seed, err := seedFor(who, "1234")
		if err != nil {
			t.Errorf("prop %q: a seed it should be dropping was refused: %v", prop, err)
		}
		if seed != nil {
			t.Errorf("prop %q: kept the seed %v", prop, seed)
		}
		if reading := readingFor(who, big.NewInt(1234)); reading != nil {
			t.Errorf("prop %q: was dealt a spread", prop)
		}
	}
	// The positive control, or the loop above passes for reasons that have
	// nothing to do with the prop.
	reader := Persona{Key: "reader", Prop: PropTarot}
	seed, err := seedFor(reader, "1234")
	if err != nil || seed == nil {
		t.Fatalf("the dealing prop lost its seed: %v / %v", seed, err)
	}
	if readingFor(reader, seed) == nil {
		t.Error("the dealing prop was dealt no spread")
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
