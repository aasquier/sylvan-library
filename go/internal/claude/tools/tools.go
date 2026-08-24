// Package tools is the read-only surface a mode may
// reach, and the door that decides what it actually gets.
//
// Two halves, deliberately kept apart. The **schemas** are recorded data
// (`data/tools.json`): a tool description
// is prescriptive prose about *when to call*, and an under-described tool is
// the most common reason a model answers from recall instead — which in this
// codebase is the exact failure rule 1 exists to prevent. Those bytes are
// load-bearing and are not retyped by hand.
//
// The **dispatch** is code, and that is the half that matters here. Every
// function this package can call is a read: `internal/deckread`'s payload
// builders and the pool's two lookups, nothing else. Not because a list says
// so, but because `internal/claude`'s boundary analysis walks the typed call
// graph of everything under the Claude surfaces and fails on any path that
// reaches a deck write — this package included, transitively.
//
// # The allowlist is checked on the name that arrived
//
// A model can ask for any tool name it likes, including one never advertised.
// `Run` decides on the name it actually received rather than trusting what was
// put in the `tools` block, which is why asking for `set_card_field` gets a
// refusal and not a write. `allowed` narrows further to a mode's declared set,
// so "a mode is a tool set" (ADR 15) holds at the door and not only in the
// advertisement.
package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed data/tools.json
var toolsJSON []byte

// Tool is one tool: a schema for the model, and the function it actually
// calls.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
	Required    []string       `json:"required"`
	// AdditionalProperties is always false and is stated anyway: the API does
	// not enforce it without `strict`, which is why Run checks arguments
	// again. Telling the model the shape beats correcting it after the fact.
	AdditionalProperties bool `json:"additional_properties"`
	// TakesSource marks the deck-facing tools. They are handed the caller's
	// deck source so nothing here reads the filesystem directly — the same
	// rule the routes follow, and what lets a second deck tier arrive as one
	// dependency swap.
	TakesSource bool `json:"takes_source"`

	// fn is set by the registry at construction, never by the data. A tool
	// whose schema exists but whose function nobody wired is a load-time
	// failure, not a runtime surprise.
	fn Handler
}

// Handler is what a tool does. Arguments arrive already checked against the
// schema; the source is nil for the two tools that need no deck.
type Handler func(ctx context.Context, args map[string]any, deps Deps) (any, error)

// Deps is what a tool may reach: a deck source, and a pool. Both may be
// absent, and both absences are answers rather than errors — a deck tool with
// no source refuses, a card tool with no pool says the pool is not there.
type Deps struct {
	Source any
	Pool   any
}

// ErrNotAllowed is a tool that this package does not expose, or that this mode
// did not offer.
//
// Its own type rather than a sentinel string so a caller can tell it from a
// tool that ran and failed. `converse` hands it back to the model as an
// `is_error` result rather than abandoning the turn: a refused write should
// read to the model as "that door does not exist", not as the end of the
// conversation.
type ErrNotAllowed struct{ Msg string }

func (e *ErrNotAllowed) Error() string { return e.Msg }

// WireName is the fault name this error wears on the wire to the model.
//
// `converse` hands a recoverable tool failure back to the model as
// `<Name>: <message>`. The name therefore reaches the model and is a
// recorded token, not this type's Go name -- and declaring it on the error
// is what makes "this failure is recoverable" a property of the error rather
// than a list `converse` keeps.
// An error type without this method is a fault the loop refuses to paper over.
func (e *ErrNotAllowed) WireName() string { return "ToolNotAllowed" }

// ErrArgumentsRejected is arguments that do not match the schema, checked
// before dispatch.
type ErrArgumentsRejected struct{ Msg string }

func (e *ErrArgumentsRejected) Error() string { return e.Msg }

// WireName is this error's recorded fault name. See ErrNotAllowed.WireName.
func (e *ErrArgumentsRejected) WireName() string { return "ToolArgumentsRejected" }

// ErrDeckNotFound is a slug that named no deck.
//
// **Its message is the bare slug**, which looks wrong and is the recorded
// shape: the fault carries nothing but the slug, so the model is handed
// `DeckNotFound: gyome`. The name still reaches the model -- which is the
// whole of what the refusal owes it, since the recovery is to call
// `list_decks` and ask again -- and keeping the recorded words is worth
// more than a prettier sentence.
type ErrDeckNotFound struct{ Slug string }

func (e *ErrDeckNotFound) Error() string { return e.Slug }

// WireName is this error's recorded fault name. See ErrNotAllowed.WireName.
func (e *ErrDeckNotFound) WireName() string { return "DeckNotFound" }

var registry map[string]*Tool

// Names is every registered tool, sorted — which is also the order Schemas
// renders. Sorted rather than insertion-ordered because tools render FIRST in
// the prompt, so an unstable order would invalidate the prompt cache on every
// turn, for free and invisibly.
var Names []string

func init() {
	var doc struct {
		Tools []*Tool `json:"tools"`
	}
	if err := json.Unmarshal(toolsJSON, &doc); err != nil {
		panic(fmt.Sprintf("claude/tools: the embedded schemas are unreadable: %v", err))
	}
	registry = make(map[string]*Tool, len(doc.Tools))
	for _, t := range doc.Tools {
		registry[t.Name] = t
		Names = append(Names, t.Name)
	}
	sort.Strings(Names)
	// Explicitly last, and explicitly here: see registerHandlers.
	registerHandlers()
}

// Register wires a handler to a schema the embedded data already carries.
//
// Separate from the data on purpose: the schema is prose and must not drift,
// the handler is a call the boundary analysis has to be able to see. A name
// with no schema, or a schema wired twice, is a programming error and panics
// at load rather than answering strangely at request time.
func Register(name string, fn Handler) {
	t, ok := registry[name]
	if !ok {
		panic(fmt.Sprintf("claude/tools: no schema for %q; the registry is "+
			"the embedded data/tools.json and this name is not in it", name))
	}
	if t.fn != nil {
		panic(fmt.Sprintf("claude/tools: %q is already wired", name))
	}
	t.fn = fn
}

// Schemas is the tool definitions for the `tools` parameter of a Messages
// request.
//
// `names` subsets the registry, which is how a mode declares a narrower tool
// set than the package offers. It can only ever NARROW: an unknown name is a
// refusal, not a silently ignored entry — a mode cannot widen its own reach by
// asking for something that is not there.
func Schemas(names []string) ([]map[string]any, error) {
	chosen := names
	if names == nil {
		chosen = Names
	}
	picked := make([]string, 0, len(chosen))
	for _, name := range chosen {
		if _, ok := registry[name]; !ok {
			return nil, &ErrNotAllowed{Msg: fmt.Sprintf("no such tool: %s", quoted(name))}
		}
		picked = append(picked, name)
	}
	sort.Strings(picked)
	out := make([]map[string]any, 0, len(picked))
	for _, name := range picked {
		t := registry[name]
		props := t.Properties
		if props == nil {
			props = map[string]any{}
		}
		required := t.Required
		if required == nil {
			required = []string{}
		}
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"input_schema": map[string]any{
				"type":                 "object",
				"properties":           props,
				"required":             required,
				"additionalProperties": t.AdditionalProperties,
			},
		})
	}
	return out, nil
}

// Run executes one tool call from a model response.
//
// The allowlist check happens HERE, on the name the model actually sent,
// rather than being assumed from what was advertised in `tools`. A model can
// ask for anything; this function is what decides that asking for
// `set_card_field` gets a refusal instead of a write.
//
// `allowed` narrows dispatch to a mode's declared tool set. A registered tool
// the mode did not offer is refused exactly like an unregistered one; nil
// means the whole read-only registry, which is what a direct caller such as
// the CLI gets.
func Run(ctx context.Context, name string, arguments map[string]any, deps Deps, allowed []string) (any, error) {
	t, ok := registry[name]
	if !ok {
		return nil, &ErrNotAllowed{Msg: fmt.Sprintf(
			"%s is not an available tool. This surface is read-only: "+
				"available tools are %s.", quoted(name), strings.Join(Names, ", "))}
	}
	if allowed != nil && !contains(allowed, name) {
		offered := append([]string{}, allowed...)
		sort.Strings(offered)
		return nil, &ErrNotAllowed{Msg: fmt.Sprintf(
			"%s is not offered by this mode. Its tools are %s.",
			quoted(name), strings.Join(offered, ", "))}
	}
	if t.fn == nil {
		return nil, &ErrNotAllowed{Msg: fmt.Sprintf(
			"%s has a schema but no handler; nothing is wired to it", quoted(name))}
	}

	args := map[string]any{}
	for k, v := range arguments {
		args[k] = v
	}
	var unknown []string
	for k := range args {
		if _, known := t.Properties[k]; !known {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, &ErrArgumentsRejected{Msg: fmt.Sprintf(
			"%s: unexpected argument(s) %s", name, strings.Join(unknown, ", "))}
	}
	var missing []string
	for _, r := range t.Required {
		v, present := args[r]
		if !present || v == nil || v == "" {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		return nil, &ErrArgumentsRejected{Msg: fmt.Sprintf(
			"%s: missing required argument(s) %s", name, strings.Join(missing, ", "))}
	}

	// Drop nulls rather than forwarding them: the read functions distinguish
	// "not filtering on price" from a filter, and their own defaults say so
	// more clearly than an explicit null from a model would.
	call := map[string]any{}
	for k, v := range args {
		if v != nil {
			call[k] = v
		}
	}
	return t.fn(ctx, call, deps)
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// quoted renders a name as a quoted literal for the refusal sentences, which
// reach the model as tool-result text: single quotes unless the name holds a
// single quote and no double quote.
func quoted(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
}
