package claude

// The dial's readout: `service.claude_status`, the whole of what
// `GET /api/claude` answers with.
//
// It lives here rather than in `internal/deckread` -- where the other read
// payloads went -- because it cannot: `internal/claude` already imports
// `deckread` for the theme brief, so a payload builder there reaching back
// into this package would be an import cycle. Everything the dial reports is
// this package's anyway (the stance, the presets, the ceiling, the modes,
// whether a credential is present); the deck reaches it as a status string
// through the same narrow `DeckStatused` the stance takes, so this file adds
// no dependency at all.
//
// **It answers three separate questions and a UI that collapses them lies.**
// Installed, configured and wanted can each be false on their own, and "no
// opinions here" reads very differently from "you have not set a key".
// Nothing here reaches a network: the stance is arithmetic over a table and
// availability is a fact about the environment, so this answers on an
// instance with no pool, no account and no credential.

// DialNever is what holds at every setting, stated on the wire rather than
// only in an ADR because it is the sentence somebody should be able to read
// next to the control.
//
// Reworded on the second 2026-08-15 punch list: the old "No stance lets
// Claude..." parsed as a fragment about some stance called "No" rather than
// as the guarantee it is.
//
// **The apostrophe is ASCII**, and it is worth a line because the obvious
// guess is wrong: the frontend's own test fixtures spell this sentence with a
// typographic U+2019, and `service.py` writes a plain `'`. The corpus caught
// the mismatch on the first run, which is what a byte comparison is for --
// every field-by-field assertion in the world reads these two as "the never
// sentence" and passes.
const DialNever = "One rule holds at every setting: Claude never writes a " +
	"card's rationale. The why is always yours."

// dialSurfaces is `service._SURFACE_DEFAULTS`: the surfaces that own their own
// answer to "no preference", because they run with no deck to derive one from.
//
// **`scan` is missing, and that is Python's, not a slip here.** `scan.py`
// defines `stance_for` and its docstring says in as many words that it is
// public so `/api/claude` will not "render `off` for a surface that was about
// to run" -- but `_SURFACE_DEFAULTS` was never extended when ADR 34 landed
// (#180), so the dial never asks it and `?surface=scan` answers `off` to this
// day. Measured against the running app, reproduced here, and raised with
// Aaron: a flip that changes behaviour is not a flip. See `DialModes` for the
// same omission wearing its other hat.
var dialSurfaces = map[string]bool{"theme": true, "research": true}

// surfaceStanceFor asks the module that owns `surface` what it means by "no
// preference". A literal here would be a second copy of an answer two other
// files already give, and the two would drift the first time one moved.
func surfaceStanceFor(surface string, requested any) (Stance, error) {
	if surface == "research" {
		return ResearchStanceFor(requested, nil)
	}
	return ThemeStanceFor(requested, nil)
}

// DialDefault is `service._default_stance`: what "no preference" resolves to,
// asked of whoever owns the answer.
//
// Three cases and none of them is a literal. A deckless surface has its own; a
// deck has one derived from its `status`; and a caller who named neither gets
// `Off`, because "I have no idea what this is about" is the one case where
// silence is right.
//
// Note the asymmetry with `DefaultFor`, which answers `Consultant` for a nil
// deck: that is the right answer to "what does a deck default to" and the
// wrong one here, so this checks for the deck first. Python spells the same
// distinction `default_for(deck) if deck else OFF`.
func DialDefault(deck DeckStatused, surface string) Stance {
	if deck == nil && dialSurfaces[surface] {
		s, err := surfaceStanceFor(surface, nil)
		if err != nil {
			// Unreachable with a nil request -- neither surface parses
			// anything on that path -- and `Off` rather than a panic if it
			// ever becomes reachable, because failing closed is what the
			// ceiling does with an unreadable value too.
			return Off
		}
		return s
	}
	if deck == nil {
		return Off
	}
	return DefaultFor(deck)
}

// DeckWithStatus adapts a bare status string to what the stance reads.
//
// Exported because the dial's caller has a parsed deck and this package must
// not import `internal/deck` -- the same narrow seam `DeckStatused` exists for,
// reached from the other side.
func DeckWithStatus(status string) DeckStatused { return statusOnly{status: status} }

// DialAxis, DialStance and friends are structs in Python's key order, never
// maps: `encoding/json` sorts a map's keys where a dict keeps insertion order,
// and this payload is rendered in order by the settings gear.

// DialPreset is one row of the preset list: the name, the sentence that
// explains it, what it resolves to, and whether this deployment will honour it
// unclamped -- so a UI can grey out a level rather than offering one that is
// silently lowered.
type DialPreset struct {
	Name      string        `json:"name"`
	Blurb     string        `json:"blurb"`
	Stance    StanceReadout `json:"stance"`
	Available bool          `json:"available"`
}

// DialMode is one built mode, as the dial publishes it. Deliberately not the
// whole `Mode`: the instructions are a prompt and the schema is an
// implementation, and neither is any of a client's business.
//
// `Tools` and `ServerTools` are apart because they are different kinds of
// thing -- a hosted search is something a user cares about in a way
// `get_cards` is not, and only one of the two goes through this package's tool
// door.
type DialMode struct {
	Name        string   `json:"name"`
	Purpose     string   `json:"purpose"`
	Tools       []string `json:"tools"`
	ServerTools []string `json:"server_tools"`
	Writes      []string `json:"writes"`
}

// Dial is the whole of `GET /api/claude`.
type Dial struct {
	Installed  bool          `json:"installed"`
	Configured bool          `json:"configured"`
	Model      string        `json:"model"`
	Stance     StanceReadout `json:"stance"`
	Ceiling    StanceReadout `json:"ceiling"`
	Default    StanceReadout `json:"default"`
	Presets    []DialPreset  `json:"presets"`
	Never      string        `json:"never"`
	Modes      []DialMode    `json:"modes"`
}

// dialModeOrder is the order `service.claude_status` lists them in, and it is
// **six of the seven**: the interview, the argument, the dossier, research,
// then the theme interview's two halves.
//
// **`scan` is absent, and that is Python's omission reproduced.** The list was
// last extended by #93 when research became the sixth mode; ADR 34's scan
// landed in #180 and never touched it, so a payload whose own comment calls
// itself "the modes that exist" has been one short ever since. It is the sixth
// completeness claim in this project to rot and the reason `ModeNames` is
// derived rather than written down.
//
// Written as an explicit list rather than derived, precisely because it cannot
// be derived: the correct derivation gives seven. `TestTheDialListsPythonsSix`
// pins both halves -- the order, and the absence -- so this cannot drift
// quietly in either direction, and fixing it is one line in each runtime on
// the day it is ruled.
var dialModeOrder = []string{
	ModeRationaleInterview,
	ModeSlotArgument,
	ModeCommanderDossier,
	ModeResearch,
	ModeThemeConversation,
	ModeThemeProposal,
}

// DialModes is the built modes as the dial publishes them.
func DialModes() []DialMode {
	out := make([]DialMode, 0, len(dialModeOrder))
	for _, name := range dialModeOrder {
		mode, err := GetMode(name)
		if err != nil {
			// A name in this list that is not a mode is a programming error
			// caught at startup by the test above, not a runtime condition.
			panic(err)
		}
		out = append(out, DialMode{
			Name:    mode.Name,
			Purpose: mode.Purpose,
			// `list(...)` in Python, and an empty list rather than null here:
			// `[]` is what a dict comprehension over an empty tuple produces,
			// and a client that indexes it must not meet a null.
			Tools:       nonNil(mode.ToolNames),
			ServerTools: nonNil(mode.ServerToolNames),
			Writes:      nonNil(mode.MayWrite),
		})
	}
	return out
}

// nonNil renders an absent slice as `[]` rather than `null`, which is what
// `list(...)` over an empty tuple gives Python and what the client's
// `ClaudeMode` type declares.
func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// Status is `service.claude_status`: the dial, resolved.
//
// `deck` is nil when no slug was named. `surface` names which mode is asking
// and exists because the answer is not the same for all of them -- without it
// the dial beside the create flow reported `off` while the conversation it
// governs was about to run at `second-opinion`. That bug is why the parameter
// exists, and it is worth remembering that all 42 tests on this endpoint
// passed while it was live, because every one of them asked about a deck.
//
// Returns an error only for a stance that will not read, which is the caller's
// 422. Nothing else here can fail.
func Status(requested any, deck DeckStatused, surface string) (Dial, error) {
	var effective Stance
	var err error
	if deck == nil && dialSurfaces[surface] {
		effective, err = surfaceStanceFor(surface, requested)
	} else {
		effective, err = Resolve(requested, deck, nil)
	}
	if err != nil {
		return Dial{}, err
	}
	limit := Ceiling()
	presets := make([]DialPreset, 0, len(PresetNames))
	for _, name := range PresetNames {
		preset, presetErr := Preset(name)
		if presetErr != nil {
			return Dial{}, presetErr
		}
		presets = append(presets, DialPreset{
			Name:   name,
			Blurb:  PresetBlurbs[name],
			Stance: Describe(preset),
			// Whether this deployment will actually honour it unclamped.
			Available: Clamp(preset, limit) == preset,
		})
	}
	return Dial{
		// **A constant, and argued.** Python asks whether `import anthropic`
		// works, because the SDK rides with the `claude` extra and a base
		// install has neither. Go has no such question: the SDK is linked into
		// this binary, so the answer cannot be false. The two agree everywhere
		// the door actually runs -- the container installs `.[api,claude]`, so
		// Python answers true there too -- and after the port retires Python
		// there is no extra left to be missing.
		Installed:  true,
		Configured: CredentialPresent(),
		// `client.model()` with **no tier**: the house answer, not this seat's.
		// Python's dial does not pass the caller's tier even though every mode
		// route does, so an account on a non-default tier is told the house
		// model here. Reproduced; the field is on the wire and nothing renders
		// it.
		Model:   ModelFor(""),
		Stance:  Describe(effective),
		Ceiling: Describe(limit),
		// The same question the effective stance just answered with no pin.
		Default: Describe(DialDefault(deck, surface)),
		Presets: presets,
		Never:   DialNever,
		Modes:   DialModes(),
	}, nil
}
