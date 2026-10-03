package claude

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Everything this package refuses to START with, and the two lookups it makes
// of itself.
//
// Four embedded documents are read before anything in here can be asked a
// question — the modes, the digit table, the casefold table and the persona
// roster — and each one answers a damaged build with a panic rather than a
// degraded feature. Every one of those panics was unenterable until the loaders
// took their bytes from a caller instead of reaching for the embed: the
// committed files parse, so the sentence a maintainer would read at four in the
// morning was a sentence nothing had ever printed. They are cheap to drive and
// the message is the whole value, so each case asks that the message says which
// document and what was wrong with it.
//
// None of these touch package state: every loader returns its table, and the
// package's own vars are built from the embed by the same call. So they run
// beside every other test in the tree.

func recovered(t *testing.T, what string, run func()) string {
	t.Helper()
	var got string
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("%s: no panic", what)
			}
			got, _ = r.(string)
			if got == "" {
				t.Fatalf("%s: panicked with %T rather than a sentence: %v", what, r, r)
			}
		}()
		run()
	}()
	return got
}

// A modes file that will not parse, and one whose hosted tool this binary has
// never heard of. The second is the sharper of the two: a mode would otherwise
// go out *without* the search it declares, which for the dossier means a
// sourced page that cites nothing and for research means an answer from recall.
func TestTheModeRegistryRefusesADocumentItCannotBuild(t *testing.T) {
	t.Parallel()
	msg := recovered(t, "a modes file that is not JSON", func() {
		loadModes([]byte(`{"modes": [`))
	})
	if !strings.Contains(msg, "modes.json") || !strings.Contains(msg, "parse") {
		t.Errorf("the panic does not say the definitions would not parse: %s", msg)
	}

	msg = recovered(t, "a hosted tool nobody taught this switch about", func() {
		loadModes([]byte(`{"modes": [{"name": "newfangled",
			"server_tools": [{"type": "crystal_ball_20991231", "name": "scry"}]}]}`))
	})
	if !strings.Contains(msg, "newfangled") {
		t.Errorf("the panic does not name the mode: %s", msg)
	}
	if !strings.Contains(msg, "crystal_ball_20991231") {
		t.Errorf("the panic does not name the tool type: %s", msg)
	}

	// And the shape that must still load: a mode with no hosted tools at all,
	// which is most of them.
	built := loadModes([]byte(`{"modes": [{"name": "plain", "purpose": "p",
		"instructions": "i", "max_tokens": 10}]}`))
	if len(built) != 1 || built["plain"].Purpose != "p" {
		t.Errorf("a plain mode loaded as %#v", built)
	}
}

// modeOf and presetOf are the lookups this package makes of its own constants,
// and they answer a name that is not one by stopping the process with the name
// in the message.
//
// That is the trade the two functions exist for: nine surfaces used to carry an
// error hand-off each for a mode name that cannot be wrong, with a sentence in
// it nobody could ever read. The failure did not go away, it moved to the one
// place where a mistake in a constant belongs — and where, unlike an `if err !=
// nil` on a constant, it can be shown to work.
func TestTheInternalLookupsNameWhatTheyCouldNotFind(t *testing.T) {
	t.Parallel()
	msg := recovered(t, "a mode this package does not define", func() {
		_ = modeOf("rationale-interrogation")
	})
	if !strings.Contains(msg, "rationale-interrogation") {
		t.Errorf("the panic does not name the mode asked for: %s", msg)
	}
	if !strings.Contains(msg, ModeRationaleInterview) {
		t.Errorf("the panic does not say what there is: %s", msg)
	}

	msg = recovered(t, "a preset this package does not define", func() {
		_ = presetOf("collaboratr")
	})
	if !strings.Contains(msg, "collaboratr") {
		t.Errorf("the panic does not name the preset asked for: %s", msg)
	}
	if !strings.Contains(msg, "collaborator") {
		t.Errorf("the panic does not say what there is: %s", msg)
	}

	// The constants themselves, which is the premise: every one of them resolves
	// through the same lookup the surfaces use.
	for _, name := range ModeNames() {
		if got := modeOf(name); got.Name != name {
			t.Errorf("modeOf(%q) answered %q", name, got.Name)
		}
	}
	for _, name := range PresetNames {
		if got, err := Preset(name); err != nil || presetOf(name) != got {
			t.Errorf("presetOf(%q) and Preset(%q) disagree", name, name)
		}
	}
	// And the one thing the deckless surfaces are all for: their default is
	// their own preset rather than `off`.
	for _, preset := range []string{ScanDefaultPreset, ResearchDefaultPreset,
		IntakeDefaultPreset} {
		if got := defaultStance(preset, nil); got == Off {
			t.Errorf("the %s default clamped to off with no ceiling set", preset)
		}
		// A ceiling the deployment set is honoured over the surface's own
		// default, which is the clamp's whole job.
		shut := Off
		if got := defaultStance(preset, &shut); got != Off {
			t.Errorf("the %s default answered %v past a closed ceiling", preset, got)
		}
	}
}

// The digit table, the casefold table and the persona roster, each refused the
// same way.
func TestTheEmbeddedTablesRefuseADamagedDocument(t *testing.T) {
	t.Parallel()
	msg := recovered(t, "a digit table that is not JSON", func() {
		loadDigitZeros([]byte(`{"zeros": [`))
	})
	if !strings.Contains(msg, "digits.json") {
		t.Errorf("the panic does not say which table: %s", msg)
	}
	if got := loadDigitZeros([]byte(`{"zeros": [48, 1632]}`)); len(got) != 2 || got[0] != '0' {
		t.Errorf("a good digit table loaded as %v", got)
	}

	msg = recovered(t, "a casefold table that is not JSON", func() {
		loadFolds([]byte(`{"folds": [`))
	})
	if !strings.Contains(msg, "casefold.json") {
		t.Errorf("the panic does not say which table: %s", msg)
	}
	if got := loadFolds([]byte(`{"folds": [{"cp": 304, "fold": "i̇"}]}`)); len(got) != 1 {
		t.Errorf("a good casefold table loaded as %v", got)
	}

	msg = recovered(t, "a persona roster that is not JSON", func() {
		parseRoster([]byte(`{"personas": [`))
	})
	if !strings.Contains(msg, "persona") {
		t.Errorf("the panic does not say which document: %s", msg)
	}
	if got := parseRoster([]byte(`{"default": "plain", "personas": []}`)); got.Default != "plain" {
		t.Errorf("a good roster parsed as %#v", got)
	}
}

// The brief's commander arrives as a row or as a pointer to one, and the type
// switch that tells those apart is why this is not one loop.
//
// Both shapes are driven here because the distinction is load-bearing and
// invisible: `commander_card` is null for a deck whose commander the pool does
// not know, and a nil pointer must contribute nothing rather than an empty card
// that the caller would then look up by its blank name.
func TestTheCommanderIsReadFromEitherShapeItArrivesIn(t *testing.T) {
	t.Parallel()
	cards := []deckread.CardJSON{{Name: "Sol Ring"}, {Name: "Swamp"}}
	boss := deckread.CardJSON{Name: "Gyome, Master Chef"}

	cases := []struct {
		note      string
		commander any
		want      []string
	}{
		{"a row", boss, []string{"Sol Ring", "Swamp", "Gyome, Master Chef"}},
		{"a pointer to a row", &boss, []string{"Sol Ring", "Swamp", "Gyome, Master Chef"}},
		{"a nil pointer", (*deckread.CardJSON)(nil), []string{"Sol Ring", "Swamp"}},
		{"nothing at all", nil, []string{"Sol Ring", "Swamp"}},
	}
	for _, tc := range cases {
		payload := wire.OrderedMap{
			{Key: "cards", Value: cards},
			{Key: "commander_card", Value: tc.commander},
		}
		var got []string
		for _, entry := range allEntries(payload) {
			got = append(got, entry.Name)
		}
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: the entries are %v, want %v", tc.note, got, tc.want)
		}
	}
}

// A brief that will not render is reported rather than sent half-built.
//
// Both openings put the facts in the prompt as indented JSON, and both are
// handed the facts by their caller. A value that cannot be rendered is the one
// failure either has, and it must come back as a failure: a prompt with the
// facts missing would be a model asked to argue about a card it was never shown,
// which is the one state these modes exist to prevent and the most expensive one
// to discover from the answer.
func TestAnOpeningRefusesFactsItCannotRender(t *testing.T) {
	t.Parallel()
	// A channel is not JSON. Nothing a brief holds is one — this is the
	// property under test rather than a shape anybody expects.
	unrenderable := wire.OrderedMap{
		{Key: "card", Value: wire.OrderedMap{{Key: "name", Value: make(chan int)}}},
	}
	if got, err := argueOpening(unrenderable, "the mana base"); err == nil {
		t.Errorf("the slot argument's opening rendered %d bytes of unrenderable facts", len(got))
	} else if got != "" {
		t.Errorf("the refusal came back with a prompt beside it: %q", got)
	}
	if got, err := interviewOpening(unrenderable, "the mana base"); err == nil {
		t.Errorf("the interview's opening rendered %d bytes of unrenderable facts", len(got))
	} else if got != "" {
		t.Errorf("the refusal came back with a prompt beside it: %q", got)
	}

	// The same facts, renderable: the prompt carries them, and the focus the
	// person typed is quoted at the end rather than interpolated into the ask.
	fine := wire.OrderedMap{{Key: "card", Value: wire.OrderedMap{
		{Key: "name", Value: "Sol Ring"}}}}
	for _, tc := range []struct {
		note string
		run  func(wire.OrderedMap, string) (string, error)
	}{{"slot argument", argueOpening}, {"interview", interviewOpening}} {
		got, err := tc.run(fine, "  the mana base  ")
		if err != nil {
			t.Fatalf("%s: %v", tc.note, err)
		}
		if !strings.Contains(got, `"name": "Sol Ring"`) {
			t.Errorf("%s: the facts are not in the prompt:\n%s", tc.note, got)
		}
		if !strings.HasSuffix(got, "the mana base") {
			t.Errorf("%s: the person's own words are not the last thing said:\n%s", tc.note, got)
		}
	}
}

// A mode may only narrow the tool registry, and a name that is not in it is a
// refusal rather than a quietly shorter tool list.
//
// The narrowing is how ADR 15's "a mode is a tool set" holds, so the failure
// that matters is the silent one: a mode advertising six tools where it declared
// seven would lose a read nobody asked about, and the model would answer from
// recall instead of from the pool. `MustMode` refuses an unknown name at load,
// which is why this is asked of a Mode built here — the guard has to work for
// the Mode that gets built next, not only for the ten in the data.
func TestAModeCannotAdvertiseAToolThatIsNotThere(t *testing.T) {
	t.Parallel()
	if got, err := (Mode{Name: "invented",
		ToolNames: []string{"list_decks", "set_card_field"}}).Schemas(); err == nil {
		t.Errorf("a mode asking for a tool that is not registered got %d schemas", len(got))
	}
	// And the registry's own answer still comes back whole, so the refusal above
	// is about the name rather than about every call.
	got, err := modeOf(ModeResearch).Schemas()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Error("research advertises no tools at all")
	}
}
