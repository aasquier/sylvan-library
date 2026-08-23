package claude

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// This file is the first half of `claude/modes.py`: what a mode *is*.
//
// [ADR 15] defines a Claude surface as four things: a system prompt, a tool
// set, a declaration of what it may write, and the user's stance over it. The
// first three are the mode and live here; the fourth is `stance.go` and is the
// user's.
//
// **The write declaration is a field, and it is checked empty.** Every mode
// ADR 15 names may write nothing, so `MayWrite` could have been left out
// entirely and the code would behave identically. It is here because the ADR's
// claim is that a mode *is* a capability declaration -- a field that exists and
// is asserted empty says that out loud, and it is the line a future ADR would
// have to change deliberately rather than a silence somebody could fill in by
// accident. The package has no write door regardless: `tools` is the whole
// surface, and `boundary_test.go` fails on the commit that names a write
// function anywhere under this tree.

// Mode is one Claude surface: a prompt, a tool set, and what it may write.
type Mode struct {
	Name    string
	Purpose string
	// Instructions is the system prompt, minus anything the stance contributes.
	Instructions string
	// ToolNames is a subset of the read-only registry. Naming one that does
	// not exist is an error at construction rather than a surprise at call
	// time.
	ToolNames []string
	// ServerTools are Anthropic-hosted tools. These run on Anthropic's side and
	// produce no round trip here, which is exactly why they are a separate
	// field from ToolNames: `tools.Run` is the door this package controls, and
	// a server tool does not go through it. Only the dossier and research use
	// one (`web_search_20260209`, ADR 19), and CLAUDE.md's no-crawler rule is
	// why it is a *hosted* search rather than something here that fetches.
	ServerTools []anthropic.ToolUnionParam
	// ServerToolNames is the same tools by the name the API calls them, kept
	// because the SDK's typed union does not hand one back and `/api/claude`
	// publishes them: `[t["name"] for t in mode.server_tools]`.
	//
	// A UI says "searches the web" about one of these and says nothing at all
	// about `get_cards`, which is the whole reason Python lists them apart
	// from ToolNames rather than merging the two.
	ServerToolNames []string
	// MayWrite is what this mode may change. Empty, and checked empty -- see
	// the file comment for why the field exists at all.
	MayWrite []string
	// MaxTokens: thinking and answer share this ceiling on Sonnet 5.
	MaxTokens int64
	// Effort is `high` deliberately, and not for depth. Lower effort levels
	// reach for tools less often, and a mode that answers from recall instead
	// of calling `get_cards` is rule 1 failing quietly.
	Effort anthropic.OutputConfigEffort
	// ResponseSchema is a JSON schema for the final answer, or nil for prose.
	ResponseSchema map[string]any
	// ScopeNotes is what each scope level means *for this mode*, keyed by the
	// scope axis's levels. Nil uses `defaultScopeNotes`, which is written for
	// the modes that are about one card in one deck -- and reads as nonsense to
	// one that is not. A mode with no card and no deck (research, ADR 26) has
	// to say what its own scope axis widens, or the prompt tells it to stay on
	// something that does not exist.
	ScopeNotes map[string]string
}

// ModeDefaults are the values a mode inherits when it does not say otherwise.
// Named constants rather than struct-literal defaults because Go has no
// dataclass field defaults, and a mode that silently got MaxTokens 0 would
// fail at the API rather than here.
const (
	DefaultModeMaxTokens int64                        = 8192
	DefaultModeEffort    anthropic.OutputConfigEffort = anthropic.OutputConfigEffortHigh
)

// NewMode fills a mode's defaults and checks it, or says why it cannot.
//
// This is `Mode.__post_init__`. Two checks, and both fail loudly rather than
// being tolerated: a mode declaring it may write anything is refused outright,
// and a tool name outside the registry is refused so a typo in a mode
// definition fails at startup rather than mid-conversation.
func NewMode(m Mode) (Mode, error) {
	if len(m.MayWrite) > 0 {
		return Mode{}, fmt.Errorf(
			"mode %s declares it may write %v. No mode may write anything "+
				"(ADR 15) -- and this package cannot reach a write path in any "+
				"case. Changing that needs a new ADR superseding 15, not a "+
				"value here", wire.PyRepr(m.Name), m.MayWrite)
	}
	if m.MaxTokens == 0 {
		m.MaxTokens = DefaultModeMaxTokens
	}
	if m.Effort == "" {
		m.Effort = DefaultModeEffort
	}
	// Returns an error for a name outside the registry.
	if _, err := tools.Schemas(m.ToolNames); err != nil {
		return Mode{}, err
	}
	return m, nil
}

// MustMode is NewMode for a package-level mode definition, where an invalid
// mode should stop the process exactly as an import-time raise does in Python.
//
// Safe to call from a package-level var initialiser even though it reaches the
// tools registry: `tools` is a different package, and Go finishes a dependency
// package's initialisation before starting its importer's. The file-order trap
// that emptied that registry once applies *within* a package, not across two.
func MustMode(m Mode) Mode {
	built, err := NewMode(m)
	if err != nil {
		panic(err)
	}
	return built
}

// Schemas is everything for the request's `tools`: ours, then Anthropic's.
//
// Server tools go last and in declaration order so the rendered block stays
// byte-stable -- tools render FIRST in the prompt, so an unstable order would
// invalidate the prompt cache on every call, for free and invisibly.
// `tools.Schemas` already sorts our half for the same reason.
//
// **The byte-stability is a hazard measured here, not an inherited caution.**
// The SDK's `ToolInputSchemaParam.ExtraFields` is a `map[string]any`, and the
// marshaller walks it in Go's randomised map-iteration order. Carrying the
// whole schema through it re-renders the tools block differently on every
// request -- and since tools render first, that invalidates the entire prompt
// cache every single time, for free, with nothing anywhere reporting it.
//
// The rule is finer than "avoid ExtraFields", and it was measured rather than
// guessed. An extra key that **shadows** a field the struct already emits is
// substituted in place and stays deterministic; only a **novel** key is
// appended, and it is appended in map order. So:
//
//	one novel key  -> one ordering, stable
//	two novel keys -> two orderings, observed
//
// This code has exactly one novel key -- `additionalProperties`, which the SDK
// has no field for -- and is therefore safe by a margin of one. A second is a
// randomised cache, so `TestTheToolsBlockIsByteStable` asserts the count as
// well as the stability: the count is what fails deterministically, and the
// message is what tells the next person why their harmless-looking key is not.
func (m Mode) Schemas() ([]anthropic.ToolUnionParam, error) {
	ours, err := tools.Schemas(m.ToolNames)
	if err != nil {
		return nil, err
	}
	out := make([]anthropic.ToolUnionParam, 0, len(ours)+len(m.ServerTools))
	for _, schema := range ours {
		param, err := toolParam(schema)
		if err != nil {
			return nil, err
		}
		out = append(out, param)
	}
	out = append(out, m.ServerTools...)
	return out, nil
}

// toolParam turns one entry of `tools.Schemas` into the SDK's tool union.
//
// The registry's map is the thing held to Python by a generated corpus, so
// this conversion is the seam where that proof could stop being about what
// goes on the wire. It is deliberately total: every key the registry writes is
// carried, `required` included when empty (a non-nil empty slice renders as
// `[]`, which is what Python emits), and an unexpected shape is an error
// rather than a silent drop.
func toolParam(schema map[string]any) (anthropic.ToolUnionParam, error) {
	name, _ := schema["name"].(string)
	description, _ := schema["description"].(string)
	inputSchema, ok := schema["input_schema"].(map[string]any)
	if !ok {
		return anthropic.ToolUnionParam{}, fmt.Errorf(
			"tool %s has no input_schema object", wire.PyRepr(name))
	}
	properties, _ := inputSchema["properties"].(map[string]any)
	required, _ := inputSchema["required"].([]string)
	if required == nil {
		required = []string{}
	}
	tool := anthropic.ToolParam{
		Name:        name,
		Description: anthropic.String(description),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: properties,
			Required:   required,
			// Exactly one key, on purpose, and one is the limit rather than a
			// coincidence -- see Schemas' comment. A second novel key makes
			// this block's byte order depend on map iteration, and the prompt
			// cache dies silently. If the SDK ever grows a typed field for
			// additionalProperties, move it there and delete this.
			ExtraFields: map[string]any{
				"additionalProperties": inputSchema["additionalProperties"],
			},
		},
	}
	return anthropic.ToolUnionParam{OfTool: &tool}, nil
}

// System is the system prompt, with what the stance widens appended.
//
// A stance may widen what a mode *does* and never what it is allowed to do
// (ADR 15), which is exactly the difference between this method and `Schemas`:
// the tool set does not move, the framing does.
func (m Mode) System(s Stance) string {
	return strings.TrimSpace(m.Instructions) + "\n\n" +
		strings.TrimSpace(scopeNote(s, m.ScopeNotes))
}

// defaultScopeNotes is how far from the question a stance lets a mode range.
//
// Only the scope axis says anything here, and that is deliberate rather than an
// oversight. **Initiative** decides whether a call happens at all -- `off`
// makes none, and above that the surface is invoked by someone clicking a
// button, so there is nothing left for it to gate. **Write autonomy** is moot:
// no mode may write. Inventing behaviour for the other two axes so the function
// looked symmetrical would be pretending the dial does more than it does.
var defaultScopeNotes = map[string]string{
	"flagged": "Scope: stay on the card you were asked about, and on anything " +
		"the gate flagged about it. Do not range into the rest of the deck.",
	"adjacent": "Scope: the card you were asked about, plus the cards that " +
		"actually interact with it -- others in its category, cards it needs " +
		"in play, cards that do its job more cheaply. You may ask about those " +
		"in service of the question at hand.",
	"rethink": "Scope: the whole deck, including its axis. If the honest " +
		"question is whether this card's *kind* of card belongs here at all -- " +
		"whether the deck is trying to do two things at once, whether the " +
		"commander wants a different shape -- ask that. It is still a " +
		"question, not a plan.",
}

// scopeNote picks the note for this stance, from the mode's own table or the
// default one.
//
// `overrides` exists because the default table is not universal: it talks about
// "the card you were asked about", which is exactly right for the interview and
// the slot argument and meaningless to a mode that was not asked about a card.
// A mode supplies its own table or gets the default; either way the text is
// byte-stable per mode and stance, which is what the prompt cache needs.
func scopeNote(s Stance, overrides map[string]string) string {
	table := overrides
	if table == nil {
		table = defaultScopeNotes
	}
	return table[s.Scope]
}
