package claude

// The dial's readout: the whole of what
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
// **It answers separate questions and a UI that collapses them lies.**
// Configured and wanted can each be false on their own, and "no opinions
// here" reads very differently from "you have not set a key". `installed` is
// the third, and here it is a constant -- argued at the field, and the reason
// the frontend renders one unavailable sentence rather than two.
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
// typographic U+2019, and the served sentence carries a plain `'`. The corpus caught
// the mismatch on the first run, which is what a byte comparison is for --
// every field-by-field assertion in the world reads these two as "the never
// sentence" and passes.
const DialNever = "One rule holds at every setting: Claude never writes a " +
	"card's rationale. The why is always yours."

// dialSurfaces is the surfaces that own their own
// answer to "no preference", because they run with no deck to derive one from.
//
// **`scan` joined it on 2026-08-23, and its absence had been a bug for three
// months.** `ScanStanceFor` has existed since ADR 34 landed (#180), public
// in as many words so `/api/claude` would not "render `off` for a surface
// that was about to run" -- and nothing ever
// asked it, because this set was not extended. Found by asking what the dial
// *answered* rather than what it meant to, and ruled with
// Aaron, the way the shared-flag and the
// stance and budget warts went. Nothing in the app sends `surface=scan`, which
// is exactly how it survived -- a doc comment is not a test, and an unused
// parameter is not a caller.
var dialSurfaces = map[string]bool{"theme": true, "research": true, "scan": true}

// surfaceStanceFor asks the module that owns `surface` what it makes of
// `requested`.
//
// **One dispatch, and the single copy is the fix rather than a tidy-up.**
// The answer once lived as two identical copies -- the default path and the
// pinned path, differing only in passing the caller's pin -- so adding a
// surface meant editing two places, and the day `scan` arrived neither was
// edited. A literal here would be a third copy of an answer three other
// files already give.
func surfaceStanceFor(surface string, requested any, limit *Stance) (Stance, error) {
	switch surface {
	case "research":
		return ResearchStanceFor(requested, limit)
	case "scan":
		return ScanStanceFor(requested, limit)
	default:
		return ThemeStanceFor(requested, limit)
	}
}

// DialDefault is what "no preference" resolves to,
// asked of whoever owns the answer.
//
// Three cases and none of them is a literal. A deckless surface has its own; a
// deck has one derived from its `status`; and a caller who named neither gets
// `Off`, because "I have no idea what this is about" is the one case where
// silence is right.
//
// Note the asymmetry with `DefaultFor`, which answers `Consultant` for a nil
// deck: that is the right answer to "what does a deck default to" and the
// wrong one here, so this checks for the deck first.
func DialDefault(deck DeckStatused, surface string, limit *Stance) Stance {
	if deck == nil && dialSurfaces[surface] {
		s, err := surfaceStanceFor(surface, nil, limit)
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

// DialAxis, DialStance and friends are structs in the recorded key order,
// never maps: `encoding/json` sorts a map's keys,
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

// dialModeOrder is the order the dial lists them in: the
// interview, the argument, the dossier, research, the theme interview's two
// halves, and the camera.
//
// **Seven, and it was six until 2026-08-23.** The list was last extended by
// #93 when research became the sixth; ADR 34's scan landed in #180 and never
// touched it, so a payload whose own comment called itself "the modes that
// exist" was one short for three months -- the sixth completeness claim in
// this project to rot. Ruled with Aaron and fixed in one change.
//
// Still an explicit list rather than a derivation: the order is a decision
// somebody makes, never an artifact of a registry's
// iteration. `TestTheDialListsEveryMode` is what stops it
// rotting again -- it holds this list equal to `ModeNames()` as a SET, so the
// next mode added fails here rather than three months later.
var dialModeOrder = []string{
	ModeRationaleInterview,
	ModeSlotArgument,
	ModeCommanderDossier,
	ModeResearch,
	ModeThemeConversation,
	ModeThemeProposal,
	ModeScan,
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
			// An empty list rather than null here: `[]` is the recorded
			// shape, and a client that indexes it must not meet a null.
			Tools:       nonNil(mode.ToolNames),
			ServerTools: nonNil(mode.ServerToolNames),
			Writes:      nonNil(mode.MayWrite),
		})
	}
	return out
}

// nonNil renders an absent slice as `[]` rather than `null`, which is the
// recorded shape and what the client's
// `ClaudeMode` type declares.
func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// Status is the dial, resolved.
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
func Status(requested any, deck DeckStatused, surface string, set Settings) (Dial, error) {
	var effective Stance
	var err error
	// Both of these take the deployment's ceiling rather than looking one up:
	// passing nil here would let each fall back to the package default, and
	// the dial would then report a stance more permissive than the deployment
	// actually honours -- which is the wrong direction for that mistake.
	if deck == nil && dialSurfaces[surface] {
		effective, err = surfaceStanceFor(surface, requested, set.Ceiling)
	} else {
		effective, err = Resolve(requested, deck, set.Ceiling)
	}
	if err != nil {
		return Dial{}, err
	}
	limit := set.ceiling()
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
		// **A constant, and argued.** The field once asked whether an
		// optional SDK was installed; here the SDK is linked into the binary,
		// so the answer cannot be false. The field stays on the wire because
		// the client reads it, and true is the truth everywhere this door
		// runs.
		Installed:  true,
		Configured: set.Endpoint.Present(),
		// `ModelFor` with **no tier**: the house answer, not this seat's.
		// The dial does not pass the caller's tier even though every mode
		// route does, so an account on a non-default tier is told the house
		// model here. Recorded; the field is on the wire and nothing renders
		// it.
		Model:   set.Endpoint.ModelFor(""),
		Stance:  Describe(effective),
		Ceiling: Describe(limit),
		// The same question the effective stance just answered with no pin.
		Default: Describe(DialDefault(deck, surface, set.Ceiling)),
		Presets: presets,
		Never:   DialNever,
		Modes:   DialModes(),
	}, nil
}
