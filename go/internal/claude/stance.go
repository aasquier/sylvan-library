// Package claude is src/mtglab/claude ported: the dial, the voices, the modes,
// and the pipe that reaches Anthropic.
//
// The order the port takes is the order the Python was written in, and for the
// same reason. The stance came first there — before any mode existed —
// because it is the frame every mode plugs into, and retrofitting a gate
// around modes that already exist is how the gate ends up with holes in it.
// Seven modes plugged into it without changing it, which is the evidence that
// the order was right; this file therefore ports it first and alone.
//
// Two invariants cross with the code and are not negotiable at any stance:
//
//   - **Off means no calls.** AllowsCalls reports false and nothing reaches the
//     network. That is a trust property first and a cost control second, and it
//     is why this file is deterministic, needs no key, and is testable with no
//     network: the decision not to call cannot itself depend on a call.
//   - **A stance may widen what a mode does, never what it is allowed to do.**
//     In particular the top of the write axis is not permission to write a
//     rationale. Rule 4 holds at every stance, and nothing in this package may
//     name a deck-write function at all — which boundary.go's analysis pass
//     enforces over the call graph rather than over a prompt.
package claude

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Each axis is ordered, least to most. Index *is* the level, which is what
// makes clamping a min() rather than a table of special cases.
var (
	Initiative = []string{"off", "on-request", "volunteers", "interjects"}
	Scope      = []string{"flagged", "adjacent", "rethink"}
	Write      = []string{"none", "proposes", "applies"}
)

// Axes is the field order Python's AXES tuple has, which is also the order
// describe() renders them in. A map would alphabetise the wire; this is a
// slice for the reason every ported payload is a struct.
var Axes = []string{"initiative", "scope", "write"}

var levels = map[string][]string{
	"initiative": Initiative,
	"scope":      Scope,
	"write":      Write,
}

var axisQuestions = map[string]string{
	"initiative": "When may it speak?",
	"scope":      "How far from the question may it range?",
	"write":      "What may it change?",
}

var levelMeanings = map[[2]string]string{
	{"initiative", "off"}:        "Never. No API calls are made at all.",
	{"initiative", "on-request"}: "Only when you ask it something.",
	{"initiative", "volunteers"}: "It may offer an observation when it has one.",
	{"initiative", "interjects"}: "It may speak up while you are working.",
	{"scope", "flagged"}:         "Only the cards the gate already flagged.",
	{"scope", "adjacent"}:        "Those cards and the ones that interact with them.",
	{"scope", "rethink"}:         "The whole deck, including its axis.",
	{"write", "none"}:            "Nothing. It talks; you type.",
	{"write", "proposes"}:        "It may assemble a batch for you to approve in one go.",
	{"write", "applies"}:         "It may make reversible edits without asking each time.",
}

// index is Python's _index: the level's position on its axis, or an error
// worded exactly as Python words it, since both strings reach a 422 body.
func index(axis, level string) (int, error) {
	ordered, ok := levels[axis]
	if !ok {
		return 0, fmt.Errorf("%s is not one of %s", wire.Quote(axis), strings.Join(Axes, ", "))
	}
	for i, candidate := range ordered {
		if candidate == level {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%s is not a %s level; expected one of %s",
		wire.Quote(level), axis, strings.Join(ordered, ", "))
}

// Stance is one setting of the three axes.
//
// A value type rather than a pointer, which is Go's spelling of Python's
// frozen dataclass: a mode that could edit the stance it was handed would be
// widening its own dial, the one thing ADR 15 says a stance is for preventing.
type Stance struct {
	Initiative string
	Scope      string
	Write      string
}

func (s Stance) get(axis string) string {
	switch axis {
	case "initiative":
		return s.Initiative
	case "scope":
		return s.Scope
	case "write":
		return s.Write
	}
	return ""
}

func (s *Stance) set(axis, level string) {
	switch axis {
	case "initiative":
		s.Initiative = level
	case "scope":
		s.Scope = level
	case "write":
		s.Write = level
	}
}

// Validate is Python's __post_init__, which a Go struct literal cannot run for
// itself. Every constructor here calls it; a hand-built Stance is the caller's
// to check, and the constructors exist so that it rarely has to be.
func (s Stance) Validate() error {
	for _, axis := range Axes {
		if _, err := index(axis, s.get(axis)); err != nil {
			return err
		}
	}
	return nil
}

// AllowsCalls reports whether anything may reach the API at all.
//
// The single most important line in this package. Initiative "off" is not a
// quiet mode — it is the absence of an integration, and every caller checks
// this before building a client rather than after.
func (s Stance) AllowsCalls() bool { return s.Initiative != "off" }

// MayWrite reports whether a mode may change a deck at all — not what it may
// change. False at write "none". True above it, and still bounded by the edit
// operations themselves: a card added to a curated deck needs a why, and no
// stance can supply one. This property is necessary and nowhere near
// sufficient.
func (s Stance) MayWrite() bool { return s.Write != "none" }

// AtLeast reports whether this stance is at or above level on axis.
func (s Stance) AtLeast(axis, level string) (bool, error) {
	mine, err := index(axis, s.get(axis))
	if err != nil {
		return false, err
	}
	theirs, err := index(axis, level)
	if err != nil {
		return false, err
	}
	return mine >= theirs, nil
}

// The presets.
var (
	// Off is the bottom of every axis, and the value a partial or malformed
	// request falls back to — so a half-written stance can only ever be
	// quieter than intended, never louder.
	Off = Stance{"off", "flagged", "none"}

	// Consultant answers when asked, stays inside what the gate flagged, and
	// writes nothing. The first position that does anything, and the safest
	// one that does.
	Consultant = Stance{"on-request", "flagged", "none"}

	// SecondOpinion offers observations unprompted and will look at
	// neighbouring cards. Still writes nothing — the jump to volunteering is
	// about *when* it speaks.
	SecondOpinion = Stance{"volunteers", "adjacent", "none"}

	// Collaborator will question the deck's whole axis and assemble an edit
	// batch for one approval. Note the write level: proposes, not applies.
	// Nothing lands without a person looking at it.
	Collaborator = Stance{"volunteers", "rethink", "proposes"}
)

// PresetNames is insertion order, not sorted order: it is what describe() and
// the /api/claude payload iterate, and Python's dict keeps the order below.
var PresetNames = []string{"off", "consultant", "second-opinion", "collaborator"}

var presets = map[string]Stance{
	"off":            Off,
	"consultant":     Consultant,
	"second-opinion": SecondOpinion,
	"collaborator":   Collaborator,
}

// PresetBlurbs is what the dial says under each name.
var PresetBlurbs = map[string]string{
	"off": "No calls, ever. The gate, the solver and the simulator all still " +
		"work -- this turns off opinions, not the tool.",
	"consultant": "Speaks when spoken to, about what the gate already flagged.",
	"second-opinion": "Volunteers observations and looks at neighbouring " +
		"cards. Still writes nothing.",
	"collaborator": "Questions the deck's axis and batches edits for one " +
		"approval. Never writes a rationale -- that rule has no " +
		"stance above it.",
}

// Preset resolves a preset by name, refusing an unknown one rather than
// guessing. The refusal lists what there is, sorted — Python builds that list
// with sorted(PRESETS), which is not PresetNames' order, and the string
// reaches a 422 body.
func Preset(name string) (Stance, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if s, ok := presets[key]; ok {
		return s, nil
	}
	known := make([]string, 0, len(presets))
	for k := range presets {
		known = append(known, k)
	}
	sort.Strings(known)
	return Stance{}, fmt.Errorf("%s is not a stance preset; expected one of %s",
		wire.Quote(name), strings.Join(known, ", "))
}

// DeckStatused is what DefaultFor needs of a deck, which is one string. The
// narrow interface is deliberate: this package sits below internal/deck and
// must not import it, exactly as mana sits below sim.
type DeckStatused interface {
	DeckStatus() string
}

// DefaultFor is the stance a deck starts at, derived from what the deck
// already says.
//
// ADR 15: status built|theoretical already draws the distinction that matters.
// Goreclaw and Tivit are lists under consideration, where a wild suggestion
// costs a moment's thought; Arahbo and Gyome are sleeved cardboard, where
// acting on one costs money and a trip to the box. So a theoretical deck
// defaults to a wider stance and a built deck to a narrower one, and neither
// needs configuring.
//
// Still conservative on both counts: the widest default is SecondOpinion,
// which writes nothing. Reaching a stance that can edit is always a choice
// somebody makes.
func DefaultFor(deck DeckStatused) Stance {
	if deck == nil {
		return Consultant
	}
	if strings.ToLower(strings.TrimSpace(deck.DeckStatus())) == "theoretical" {
		return SecondOpinion
	}
	return Consultant
}

// CeilingEnv names the deployment's cap.
const CeilingEnv = "MTGLAB_CLAUDE_STANCE_CEILING"

// Ceiling is the most permissive stance this deployment allows anyone to
// select.
//
// Defaults to Collaborator — the top of what presets offer — because a local
// run on the maintainer's own key should not need configuring to work. A
// hosted instance sets MTGLAB_CLAUDE_STANCE_CEILING and every request is
// clamped to it, including ones from a client that asks for more.
//
// Read at call time, not at package init: an operator changing the cap should
// not have to restart the process to lower it.
func Ceiling() Stance {
	raw := os.Getenv(CeilingEnv)
	if raw == "" {
		return Collaborator
	}
	s, err := Preset(raw)
	if err != nil {
		// An unreadable ceiling is not a reason to run uncapped. Fail closed:
		// a typo in a deployment variable should cost a feature, not open one.
		return Off
	}
	return s
}

// Clamp returns requested with every axis lowered to limit where it exceeds
// it.
//
// Per axis rather than whole-stance, so a ceiling of "may speak freely but may
// never write" is expressible — which is the shape a hosted instance most
// plausibly wants.
func Clamp(requested, limit Stance) Stance {
	var out Stance
	for _, axis := range Axes {
		mine, err := index(axis, requested.get(axis))
		if err != nil {
			return Off
		}
		theirs, err := index(axis, limit.get(axis))
		if err != nil {
			return Off
		}
		if theirs < mine {
			mine = theirs
		}
		out.set(axis, levels[axis][mine])
	}
	return out
}
