package tools

import (
	"strings"
	"testing"
)

// The two refusals that run before anything else in this package can be asked a
// question, driven over documents the committed one is not.
//
// Both are panics on purpose: a schema file that will not parse, or a tool
// described in the data with no function beside it, is a broken build rather
// than a bad request, and the only honest moment to say so is before the first
// conversation. Until `loadRegistry` took its bytes from a caller, neither could
// be shown to work — the embedded file parses and `registerHandlers` wires every
// tool in it, so the two sentences a maintainer would have to read sat behind
// branches nothing could enter.
func TestTheRegistryRefusesSchemasItCannotRead(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"tools": [`,
		`not json at all`,
		`{"tools": {"list_decks": {}}}`,
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("%q loaded as a registry", raw)
					return
				}
				if msg, _ := r.(string); !strings.Contains(msg, "unreadable") {
					t.Errorf("the panic over %q does not say the schemas could "+
						"not be read: %v", raw, r)
				}
			}()
			_, _ = loadRegistry([]byte(raw))
		}()
	}
}

// A schema with no handler stops the load by name, so that `Run` can dispatch
// without asking and nobody meets the wiring mistake in a conversation.
func TestAToolWithNoHandlerStopsTheLoadByName(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a registry holding an unwired tool was accepted")
		}
		msg, _ := r.(string)
		if !strings.Contains(msg, "forgotten_tool") {
			t.Errorf("the panic does not name the tool: %v", r)
		}
		if !strings.Contains(msg, "handler") {
			t.Errorf("the panic does not say what is missing: %v", r)
		}
	}()
	set, _ := loadRegistry([]byte(`{"tools": [
		{"name": "wired_tool", "description": "has one"},
		{"name": "forgotten_tool", "description": "has none"}
	]}`))
	set["wired_tool"].fn = listDecks
	requireWired(set)
}

// And the whole of the committed data passes it, which is the premise the
// deleted check in `Run` rested on.
func TestEveryRegisteredToolIsWired(t *testing.T) {
	t.Parallel()
	requireWired(registry)
	if len(registry) == 0 {
		t.Fatal("the registry is empty, so holding it wired proves nothing")
	}
	for name, tool := range registry {
		if tool.fn == nil {
			t.Errorf("%s has no handler", name)
		}
		// The normalisation `Schemas` used to do at render time. A null here
		// reaches the model as `"properties": null`, which is not an object.
		if tool.Properties == nil {
			t.Errorf("%s carries a null properties map", name)
		}
		if tool.Required == nil {
			t.Errorf("%s carries a null required list", name)
		}
	}
}

// A tool whose schema omits `properties` or `required` gets an empty object and
// an empty list at load, rather than carrying a null into the `tools` block.
//
// The committed file spells both on every tool, which is why this is asked of a
// document built here: the normalisation is for the eighth tool somebody adds in
// a hurry, and an untested normalisation is a promise rather than a behaviour.
func TestAToolMissingItsPropertiesIsFilledInAtLoad(t *testing.T) {
	t.Parallel()
	set, names := loadRegistry([]byte(`{"tools": [
		{"name": "second", "description": "b"},
		{"name": "first", "description": "a", "properties": {"slug": {}},
		 "required": ["slug"]}
	]}`))
	if len(names) != 2 || names[0] != "first" || names[1] != "second" {
		t.Fatalf("the names are %v, want them sorted", names)
	}
	bare := set["second"]
	if bare.Properties == nil || len(bare.Properties) != 0 {
		t.Errorf("properties is %#v, want an empty map", bare.Properties)
	}
	if bare.Required == nil || len(bare.Required) != 0 {
		t.Errorf("required is %#v, want an empty list", bare.Required)
	}
	// What the data did carry is left alone.
	if full := set["first"]; len(full.Properties) != 1 || len(full.Required) != 1 {
		t.Errorf("the declared schema was overwritten: %#v %#v",
			full.Properties, full.Required)
	}
}
