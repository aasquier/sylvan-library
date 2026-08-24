package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// Every defined mode is loadable here, and the count is not a number
// this file remembers.
//
// `data/modes.json` was built **by discovery** rather than from a list, for
// a reason worth keeping: an earlier hand list silently missed the scan
// mode, whose definition is spelled differently from its siblings. Seven
// became six with nothing looking wrong.
// So the assertion here is that the loader loads exactly what the file
// holds; the file itself is frozen recorded data.
func TestEveryDefinedModeLoads(t *testing.T) {
	var file modeFile
	raw, err := os.ReadFile("data/modes.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Modes) == 0 {
		t.Fatal("no modes in data/modes.json")
	}
	var want []string
	for _, m := range file.Modes {
		want = append(want, m.Name)
	}
	sort.Strings(want)
	if got := ModeNames(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("loaded %v, the data file holds %v", got, want)
	}
	// The named constants are names that exist, not names somebody hoped for.
	for _, name := range []string{
		ModeRationaleInterview, ModeSlotArgument, ModeCommanderDossier,
		ModeResearch, ModeThemeConversation, ModeThemeProposal, ModeScan,
	} {
		if _, err := GetMode(name); err != nil {
			t.Errorf("the constant %q names no mode: %v", name, err)
		}
	}
}

// ADR 15: no mode may write anything, and the field is checked empty rather
// than left silent. Every mode goes through MustMode at load, so a mode
// declaring a write would panic the process -- this is the assertion that the
// data really is empty rather than the check being vacuous.
func TestNoModeDeclaresAWrite(t *testing.T) {
	for _, name := range ModeNames() {
		m, err := GetMode(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.MayWrite) != 0 {
			t.Errorf("mode %q declares it may write %v", name, m.MayWrite)
		}
	}
}

// properties pulls a response schema's top-level property names.
func properties(t *testing.T, m Mode) map[string]any {
	t.Helper()
	if m.ResponseSchema == nil {
		t.Fatalf("mode %q has no response schema", m.Name)
	}
	props, ok := m.ResponseSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("mode %q's schema has no properties object", m.Name)
	}
	return props
}

// ADR 25's whole design, and it is an ABSENCE -- which is exactly what a
// hand-copied schema loses with nothing looking wrong.
//
// The slot argument makes the case against a slot and has no way to make the
// case for one. That matters because the balanced version is the attractive
// one and it is a rationale generator: a paragraph explaining why a card earns
// its place, grounded in the user's own deck, is a `why` in everything but
// authorship. Guarding it in the UI would not be guarding it.
func TestTheSlotArgumentHasNowhereToPutADefence(t *testing.T) {
	m, err := GetMode(ModeSlotArgument)
	if err != nil {
		t.Fatal(err)
	}
	props := properties(t, m)
	for _, forbidden := range []string{"defence", "defense", "verdict", "summary"} {
		if _, present := props[forbidden]; present {
			t.Errorf("the slot argument's schema has a %q field -- ADR 25's "+
				"design is that a balanced answer has nowhere to go", forbidden)
		}
	}
	// And one cannot be added at run time.
	if extra, _ := m.ResponseSchema["additionalProperties"].(bool); extra {
		t.Error("the slot argument allows additional properties, so the model " +
			"can invent the field the schema deliberately omits")
	}
	if _, present := props["charges"]; !present {
		t.Error("the slot argument lost its charges array: the schema no " +
			"longer carries itself")
	}
}

// ADR 34: Claude reads a photographed card and DOES NOT NAME IT. The response
// schema has no field for a card name -- `identify` decides what card it is,
// against the pool. A better camera, never a better judge.
func TestTheScanHasNoFieldForACardName(t *testing.T) {
	m, err := GetMode(ModeScan)
	if err != nil {
		t.Fatal(err)
	}
	props := properties(t, m)
	for _, forbidden := range []string{"card", "card_name", "name", "match", "identified"} {
		if _, present := props[forbidden]; present {
			t.Errorf("the scan schema has a %q field -- ADR 34 is that the "+
				"model transcribes what is printed and never says which card "+
				"it is", forbidden)
		}
	}
	// What it does have: what is printed, where it is printed.
	for _, wanted := range []string{"title", "corner"} {
		if _, present := props[wanted]; !present {
			t.Errorf("the scan schema lost its %q field", wanted)
		}
	}
	if extra, _ := m.ResponseSchema["additionalProperties"].(bool); extra {
		t.Error("the scan allows additional properties, so the model can " +
			"volunteer the card name the schema omits")
	}
}

// A hosted tool this package cannot build is a mode that would go out without
// its search -- silently, since the request is still valid without it. Loading
// panics instead; this proves the ones actually declared do build, and that
// the modes ADR 19 and ADR 26 give a search to still have one.
func TestTheHostedSearchSurvivesTheCrossing(t *testing.T) {
	for _, name := range []string{ModeCommanderDossier, ModeResearch} {
		m, err := GetMode(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.ServerTools) == 0 {
			t.Errorf("mode %q lost its hosted web search -- it cites pages it "+
				"actually read, and it cannot read any without one", name)
			continue
		}
		schemas, err := m.Schemas()
		if err != nil {
			t.Fatalf("mode %q: %v", name, err)
		}
		raw, err := json.Marshal(schemas)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "web_search_20260209") {
			t.Errorf("mode %q's search did not render onto the wire: %s", name, raw)
		}
		// `max_uses` is a real limit rather than decoration: it is what stops a
		// four-minute dossier becoming a twelve-minute one.
		if !strings.Contains(string(raw), "max_uses") {
			t.Errorf("mode %q's search lost its max_uses bound: %s", name, raw)
		}
	}
}

// An unknown hosted tool type must be loud. The loader panics, and this is
// what proves the branch exists rather than trusting the comment.
func TestAnUnknownHostedToolIsRefused(t *testing.T) {
	_, err := serverToolSpec{Type: "web_search_29991231", Name: "web_search"}.param()
	if err == nil {
		t.Fatal("an unknown hosted tool type was accepted")
	}
	if !strings.Contains(err.Error(), "web_search_29991231") {
		t.Errorf("the refusal should name the type it could not build: %v", err)
	}
}

// Every mode's prompt is really there. A blank instruction block would still
// build a valid request and would produce an answer -- a much worse one, from
// a model told nothing about what it is doing.
func TestEveryModeCarriesItsPromptAndSchema(t *testing.T) {
	for _, name := range ModeNames() {
		m, err := GetMode(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(strings.TrimSpace(m.Instructions)) < 200 {
			t.Errorf("mode %q's instructions are %d characters -- that is not a "+
				"system prompt, it is an empty field that will still answer",
				name, len(m.Instructions))
		}
		if m.ResponseSchema == nil {
			t.Errorf("mode %q has no response schema", name)
		}
		if m.MaxTokens == 0 {
			t.Errorf("mode %q has no token ceiling", name)
		}
	}
}
