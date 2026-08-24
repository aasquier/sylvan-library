package claude

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTheSevenVoicesAreTheRecordedOnes reads the embedded roster back and
// checks it is the recorded shape — seven voices, plain first, exactly one
// of them dealing.
func TestTheSevenVoicesAreTheRecordedOnes(t *testing.T) {
	t.Parallel()
	if len(PersonaKeys) != 7 {
		t.Fatalf("expected seven voices, got %d: %v", len(PersonaKeys), PersonaKeys)
	}
	if PersonaKeys[0] != "plain" || DefaultPersona != "plain" {
		t.Errorf("the plain voice must open the grid and be the default; got %v / %q",
			PersonaKeys, DefaultPersona)
	}
	var deals []string
	for _, k := range PersonaKeys {
		if personas[k].Deals {
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
	}
	// A caller that sorts what it is handed must not reorder the tile grid for
	// the next request.
	first := Roster()
	first[0], first[1] = first[1], first[0]
	if Roster()[0].Key != PersonaKeys[0] {
		t.Error("Roster() handed out its own backing array")
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
