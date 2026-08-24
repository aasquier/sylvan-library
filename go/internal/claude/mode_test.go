package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// ADR 15's claim is that a mode *is* a capability declaration. The field being
// checked empty is that claim in code -- and it is the line a future ADR would
// have to change deliberately rather than a silence somebody could fill in.
func TestAModeMayNotDeclareThatItWrites(t *testing.T) {
	t.Parallel()
	_, err := NewMode(Mode{Name: "greedy", ToolNames: []string{"list_decks"},
		MayWrite: []string{"why"}})
	if err == nil {
		t.Fatal("a mode declaring a write was accepted")
	}
	for _, want := range []string{"ADR 15", "greedy"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should mention %q: %v", want, err)
		}
	}
}

// A typo in a mode definition should fail where the mode is defined, not
// mid-conversation three turns in.
func TestAModeNamingAToolThatDoesNotExistIsRefused(t *testing.T) {
	t.Parallel()
	if _, err := NewMode(Mode{Name: "typo", ToolNames: []string{"list_deks"}}); err == nil {
		t.Fatal("a mode naming an unregistered tool was accepted")
	}
	// And a real write function is refused by the same door, which is the one
	// that matters: `tools` has no such entry, so asking for it is a typo as
	// far as this check is concerned -- and `boundary_test.go` is what stops
	// one being added.
	if _, err := NewMode(Mode{Name: "sneaky", ToolNames: []string{"set_card_field"}}); err == nil {
		t.Fatal("a mode naming a write tool was accepted")
	}
}

func TestMustModePanicsSoAnInvalidModeCannotStart(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Error("MustMode returned for an invalid mode")
		}
	}()
	MustMode(Mode{Name: "bad", MayWrite: []string{"why"}})
}

func TestAModeFillsItsDefaults(t *testing.T) {
	t.Parallel()
	m, err := NewMode(Mode{Name: "plain", ToolNames: []string{"list_decks"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.MaxTokens != DefaultModeMaxTokens {
		t.Errorf("MaxTokens %d, want %d -- zero would be a 400 at the API",
			m.MaxTokens, DefaultModeMaxTokens)
	}
	// `high` deliberately, and not for depth: lower effort levels reach for
	// tools less often, and a mode that answers from recall instead of calling
	// get_cards is rule 1 failing quietly.
	if m.Effort != anthropic.OutputConfigEffortHigh {
		t.Errorf("effort %q, want high", m.Effort)
	}
}

// Ours sorted, then Anthropic's in declaration order. Tools render first in the
// prompt, so this order is the prompt cache's prefix.
func TestServerToolsRenderAfterOursAndInDeclarationOrder(t *testing.T) {
	t.Parallel()
	search := anthropic.ToolUnionParam{
		OfWebSearchTool20260209: &anthropic.WebSearchTool20260209Param{},
	}
	m, err := NewMode(Mode{
		Name:        "researcher",
		ToolNames:   []string{"validate_deck", "get_cards", "list_decks"},
		ServerTools: []anthropic.ToolUnionParam{search},
	})
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := m.Schemas()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range schemas {
		if s.OfTool != nil {
			names = append(names, s.OfTool.Name)
			continue
		}
		names = append(names, "<server>")
	}
	want := "[get_cards list_decks validate_deck <server>]"
	if got := strings.Join([]string{"[", strings.Join(names, " "), "]"}, ""); got != want {
		t.Errorf("tool order is %s, want %s -- ours sorted, then the hosted "+
			"ones last", got, want)
	}
	// And a server tool really is a tool the request carries, not a name.
	raw, err := json.Marshal(schemas[len(schemas)-1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "web_search_20260209") {
		t.Errorf("the hosted search did not render: %s", raw)
	}
}

// A mode that was not asked about a card has to say what its own scope axis
// widens, or the prompt tells it to stay on something that does not exist.
func TestAModeMaySupplyItsOwnScopeNotes(t *testing.T) {
	t.Parallel()
	own := map[string]string{
		"flagged":  "Scope: the question as asked.",
		"adjacent": "Scope: the question, and what bears on it.",
		"rethink":  "Scope: whatever Magic makes of it.",
	}
	m, err := NewMode(Mode{Name: "research", Instructions: "Answer.", ScopeNotes: own})
	if err != nil {
		t.Fatal(err)
	}
	s, err := Preset("second-opinion")
	if err != nil {
		t.Fatal(err)
	}
	got := m.System(s)
	if !strings.Contains(got, own[s.Scope]) {
		t.Errorf("the mode's own scope note is missing:\n%s", got)
	}
	if strings.Contains(got, "the card you were asked about") {
		t.Error("a deck-less mode was told to stay on a card it was never given")
	}
}

// The default table is what a mode about one card in one deck gets.
func TestTheDefaultScopeNotesWidenWithTheDial(t *testing.T) {
	t.Parallel()
	m, err := NewMode(Mode{Name: "interview", Instructions: "Ask."})
	if err != nil {
		t.Fatal(err)
	}
	for scope, want := range map[string]string{
		"flagged":  "Do not range into the rest of the deck",
		"adjacent": "cards that\nactually interact with it",
		"rethink":  "the whole deck, including its axis",
	} {
		got := m.System(Stance{Initiative: "on-request", Scope: scope, Write: "none"})
		if !strings.Contains(got, strings.ReplaceAll(want, "\n", " ")) {
			t.Errorf("scope %q rendered:\n%s", scope, got)
		}
	}
}

// The system prompt is instructions then scope, with the mode's own text first
// -- a stance widens what a mode does, never what it is.
func TestTheSystemPromptIsInstructionsThenScope(t *testing.T) {
	t.Parallel()
	m, err := NewMode(Mode{Name: "x", Instructions: "  You are a thing.  "})
	if err != nil {
		t.Fatal(err)
	}
	got := m.System(Stance{Initiative: "on-request", Scope: "flagged", Write: "none"})
	if !strings.HasPrefix(got, "You are a thing.\n\nScope:") {
		t.Errorf("system prompt is %q", got)
	}
}
