package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSurface is the deck-write surface, by name.
//
// `set_card_field` is the one ADR 15 names — it is the only route to the `why`
// field — but the whole write surface is here, because a mode that can call
// `swap_card` can launder a rationale through its `why` argument just as
// effectively. The snake_case spellings are deliberate: the model asks for
// tools by the names it was told, and those names are the registry's own
// recorded vocabulary.
var writeSurface = []string{
	"add_card", "remove_card", "set_card_field", "set_deck_field",
	"set_note", "swap_card", "import_deck", "create_deck", "_commit",
	// The Go spellings too, in case a future wiring uses them.
	"AddCard", "RemoveCard", "SetCardField", "SetDeckField", "SetNote",
	"ReplaceCard", "Create", "Delete", "WriteText", "SetShared",
}

// TestTheRegistryExposesNoWriteFunction is the runtime half of the boundary
// invariant. `internal/claude`'s analysis pass holds the source; this holds the
// door.
func TestTheRegistryExposesNoWriteFunction(t *testing.T) {
	t.Parallel()
	for _, name := range writeSurface {
		if _, exposed := registry[name]; exposed {
			t.Errorf("%q is in the read-only registry", name)
		}
	}
	if len(Names) != 7 {
		t.Errorf("the registry has %d tools, want the seven read-only ones: %v",
			len(Names), Names)
	}
}

// TestAskingForAWriteByNameIsRefused is the property that matters more than
// the advertisement: a model can request any tool name it likes, including one
// never offered, and `Run` decides on the name it actually received.
func TestAskingForAWriteByNameIsRefused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for _, name := range writeSurface {
		_, err := Run(ctx, name, map[string]any{
			"slug": "arahbo-cats", "name": "Sol Ring",
			"field": "why", "value": "Fast mana, obviously.",
		}, Deps{}, nil)
		if err == nil {
			t.Errorf("%q was dispatched", name)
			continue
		}
		var notAllowed *ErrNotAllowed
		if !asNotAllowed(err, &notAllowed) {
			t.Errorf("%q was refused, but not as a refusal: %T %v", name, err, err)
		}
	}
}

// TestAModeCannotWidenTheToolSetByAsking is ADR 15 at the door: a stance may
// widen what a mode does, never what it is allowed to do — and neither may a
// mode.
func TestAModeCannotWidenTheToolSetByAsking(t *testing.T) {
	t.Parallel()
	if _, err := Schemas([]string{"get_deck", "set_card_field"}); err == nil {
		t.Error("a mode declared a write tool and Schemas allowed it")
	}
	// And at dispatch: a REGISTERED tool the mode did not offer is refused
	// exactly like an unregistered one.
	_, err := Run(context.Background(), "search_cards", nil, Deps{}, []string{"get_deck"})
	if err == nil {
		t.Error("a tool outside the mode's set was dispatched")
	} else if !strings.Contains(err.Error(), "not offered by this mode") {
		t.Errorf("refusal does not say the mode did not offer it: %v", err)
	}
}

// TestNoToolSchemaAcceptsAWhy is belt and braces on the argument side.
//
// Even a read-only tool would be a laundering route if it took prose destined
// for a rationale. Nothing in the registry may have a `why` input.
func TestNoToolSchemaAcceptsAWhy(t *testing.T) {
	t.Parallel()
	for _, name := range Names {
		for prop := range registry[name].Properties {
			if strings.Contains(strings.ToLower(prop), "why") {
				t.Errorf("%s takes a %q", name, prop)
			}
		}
	}
	// Asserted over the rendered schemas too, since that is what the model
	// actually receives.
	schemas, err := Schemas(nil)
	if err != nil {
		t.Fatalf("rendering schemas: %v", err)
	}
	raw, _ := json.Marshal(schemas)
	var doc []map[string]any
	_ = json.Unmarshal(raw, &doc)
	for _, s := range doc {
		props := s["input_schema"].(map[string]any)["properties"].(map[string]any)
		for prop := range props {
			if strings.Contains(strings.ToLower(prop), "why") {
				t.Errorf("%v advertises a %q input", s["name"], prop)
			}
		}
	}
}

// TestEverySchemaIsWired catches the half that would otherwise fail at request
// time: a description that exists as data with no function behind it.
func TestEverySchemaIsWired(t *testing.T) {
	t.Parallel()
	for _, name := range Names {
		if registry[name].fn == nil {
			t.Errorf("%s has a schema but nothing is wired to it", name)
		}
		if registry[name].Description == "" {
			t.Errorf("%s has no description; an under-described tool is the "+
				"most common reason a model answers from recall instead", name)
		}
		if len(registry[name].Description) < 200 {
			t.Errorf("%s's description is %d bytes -- these are prescriptive "+
				"about WHEN to call, not one-liners", name, len(registry[name].Description))
		}
	}
}

// TestTheSchemasAreTheRecordedOnes reads the embedded data back and holds
// the rendered block to it, including the sort.
//
// The order is load-bearing and its failure is silent: tools render FIRST in
// the prompt, so an unstable order invalidates the prompt cache on every turn
// — for free, and invisibly.
func TestTheSchemasAreTheRecordedOnes(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("data", "tools.json"))
	if err != nil {
		t.Fatalf("reading the schemas: %v", err)
	}
	var doc struct {
		Tools []struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Required    []string `json:"required"`
			TakesSource bool     `json:"takes_source"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(doc.Tools) != len(Names) {
		t.Fatalf("%d schemas, %d registered", len(doc.Tools), len(Names))
	}
	schemas, err := Schemas(nil)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	for i, s := range schemas {
		if s["name"] != doc.Tools[i].Name {
			t.Errorf("schema %d is %v, want %s -- the order must be stable or "+
				"the prompt cache misses every turn", i, s["name"], doc.Tools[i].Name)
		}
		if s["description"] != doc.Tools[i].Description {
			t.Errorf("%v's description is not the generated one", s["name"])
		}
		inner := s["input_schema"].(map[string]any)
		if inner["additionalProperties"] != false {
			t.Errorf("%v allows additional properties", s["name"])
		}
	}
}

// TestArgumentsAreCheckedBeforeDispatch covers the schema check `Run` does on
// the way in — which the API does NOT enforce without `strict`, which is why
// it is done here as well as advertised.
func TestArgumentsAreCheckedBeforeDispatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// An unknown argument is refused by name.
	_, err := Run(ctx, "get_deck", map[string]any{"slug": "x", "why": "nope"}, Deps{}, nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected argument(s) why") {
		t.Errorf("an unknown argument was accepted: %v", err)
	}
	// A missing required argument is refused by name.
	_, err = Run(ctx, "get_deck", map[string]any{}, Deps{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing required argument(s) slug") {
		t.Errorf("a missing slug was accepted: %v", err)
	}
	// An EMPTY required argument counts as missing -- nil and "" both read
	// as absent.
	_, err = Run(ctx, "get_deck", map[string]any{"slug": ""}, Deps{}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing required argument(s) slug") {
		t.Errorf("an empty slug was accepted: %v", err)
	}
	// A tool with no required arguments runs with none — and refuses for a
	// reason about the SOURCE, not about its arguments.
	_, err = Run(ctx, "list_decks", nil, Deps{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no deck library") {
		t.Errorf("list_decks refused for the wrong reason: %v", err)
	}
}

func asNotAllowed(err error, target **ErrNotAllowed) bool {
	return errors.As(err, target)
}
