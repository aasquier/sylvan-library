package claude

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// personaJSON is the recorded roster of voices. Embedded as data rather
// than transcribed into code: a voice is ~1.7KB of
// prose whose bytes reach a model, seven of them, and hand-copying 11KB of
// English into string literals is precisely the drift the embed exists to
// prevent.
//
//go:embed data/personas.json
var personaJSON []byte

// Persona is a voice the interview can adopt.
//
// A persona is NOT a stance and is deliberately not modelled as one: the three
// axes are about how much the model does, and folding "who does it sound like"
// into that would make one dial mean two things. A persona may not widen what
// a mode does, exactly as a stance may not (ADR 15). It changes register and
// nothing else.
type Persona struct {
	Key string
	// Label is what the door calls it.
	Label string
	// Blurb is one line under the label. Written for somebody who has never
	// played.
	Blurb string
	// Voice is APPENDED after a mode's instructions, never in place of them.
	// That is the whole design: the rules that make the interview work — one
	// question at a time, ask about them and never about Magic, every slot
	// carries a quote of their own words, never propose — are not a persona's
	// to soften. A voice is appended; a contract is not.
	Voice string
	// Deals reports whether this persona is dealt a spread before it starts.
	// Only the fortune teller is, and internal/tarot owns the deal.
	Deals bool
}

// RosterEntry is a persona as the door may serve it — which is a Persona with
// Voice removed. Its own type rather than a Persona marshalled with a `json:"-"`
// tag, so that adding a field to Persona cannot silently publish it.
type RosterEntry struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Blurb string `json:"blurb"`
	Deals bool   `json:"deals"`
}

var (
	// PersonaKeys is insertion order — the order the tile grid renders in,
	// which puts the plain voice first and the fortune teller second. A map
	// would alphabetise it and barkeep would open the door.
	PersonaKeys []string
	personas    map[string]Persona
	roster      []RosterEntry

	// DefaultPersona is what an absent or unreadable value becomes: the plain
	// interview, because that is what every client written before personas
	// existed sends today — nothing.
	DefaultPersona string
)

func init() {
	var doc struct {
		Default  string        `json:"default"`
		Personas []Persona     `json:"personas"`
		Roster   []RosterEntry `json:"roster"`
	}
	// The embedded file is generated and checked in, so a failure here is a
	// broken build rather than a bad request: panic is the honest response.
	if err := json.Unmarshal(personaJSON, &doc); err != nil {
		panic(fmt.Sprintf("claude: the embedded persona roster is unreadable: %v", err))
	}
	DefaultPersona = doc.Default
	personas = make(map[string]Persona, len(doc.Personas))
	for _, p := range doc.Personas {
		PersonaKeys = append(PersonaKeys, p.Key)
		personas[p.Key] = p
	}
	roster = doc.Roster
}

// UnknownPersonaError is a persona nobody has written. A 422, and it names
// what there is.
//
// Its own type because the caller's
// answer is to send a different string, not to retry, and certainly not to
// read it as the model failing.
type UnknownPersonaError struct{ Requested any }

func (e *UnknownPersonaError) Error() string {
	quoted := make([]string, len(PersonaKeys))
	for i, k := range PersonaKeys {
		quoted[i] = wire.Quote(k)
	}
	return fmt.Sprintf("no persona %s; there is %s",
		literalAny(e.Requested), strings.Join(quoted, ", "))
}

// GetPersona resolves the persona for a request, refusing an unknown one
// rather than guessing.
//
// nil is the default and not an error — every client written before this
// existed sends exactly that, and they should keep working unchanged.
func GetPersona(requested any) (Persona, error) {
	if requested == nil {
		return personas[DefaultPersona], nil
	}
	key, ok := requested.(string)
	if !ok {
		if raw, isRaw := requested.(json.RawMessage); isRaw {
			var decoded any
			if err := json.Unmarshal(raw, &decoded); err == nil {
				return GetPersona(decoded)
			}
		}
		return Persona{}, &UnknownPersonaError{Requested: requested}
	}
	who, ok := personas[key]
	if !ok {
		return Persona{}, &UnknownPersonaError{Requested: key}
	}
	return who, nil
}

// Roster is the voices, for the door to render. No prompts: Voice stays
// server-side.
//
// Returns a copy, because a handler that sorted the slice it was handed would
// reorder the tile grid for every later request in the process.
func Roster() []RosterEntry {
	out := make([]RosterEntry, len(roster))
	copy(out, roster)
	return out
}

// WithVoice is the same mode, speaking differently.
//
// Appended, never substituted, and LAST — recency is worth something and the
// reference data in between is a table rather than an instruction. The default
// persona gets the base instructions back unchanged, same bytes, so nothing
// about the interview as it already exists moves.
func WithVoice(baseName, baseInstructions string, who Persona) (string, string) {
	if who.Voice == "" {
		return baseName, baseInstructions
	}
	return baseName + ":" + who.Key, baseInstructions + "\n\n" + who.Voice
}
