package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The slot argument: the case against a card, and no case for it.
//
// ADR 25, and the whole design is an absence. The interview holds rule 4 with
// a predicate anybody can read -- everything it returns must end in a question
// mark -- and this mode's output is *all* declarative sentences about a card's
// merit, so that guard would delete the feature. The replacement is that it
// argues **one direction and has no way to argue the other**: the response
// schema has no `defence`, `verdict` or `summary` field and forbids extra
// properties, so a balanced answer has nowhere to go.
//
// That matters because the balanced version is the attractive one and it is a
// rationale generator. A paragraph explaining why a card earns its place,
// grounded in the user's own deck, is a `why` in everything but authorship.
//
// Three consequences, and this file is two of them:
//
// **Every charge must cite a fact or it is dropped and counted.** The
// predicate moved from "is it a question" to "does it rest on anything", since
// every item here is declarative by design.
//
// **Alternatives are bare names and Python judges them.** `ResolveAlternatives`
// is where rule 2 becomes executable: each name is resolved through the pool
// and dropped if it does not exist, is banned, is already in the deck, or falls
// outside the deck's colour identity -- counted separately in each case,
// because "you invented that card" and "that card is off-colour" are different
// failures.
//
// The third -- a weak case is reported as weak via `strength` -- lives in the
// schema, which crossed as generated data.

// MaxCharges caps the case. More than five and it stops being an argument and
// starts being a pile.
const MaxCharges = 5

// MaxAlternatives caps what may be offered in place of the card.
const MaxAlternatives = 6

// Grounds are the kinds of case that can be made. An unrecognised value falls
// back rather than dropping the charge -- see OnlyCharges.
var Grounds = []string{"redundancy", "cost", "speed", "conditionality",
	"count", "ceiling", "legality"}

// Strengths is how much weight a charge carries. Three, because two collapses
// "this is fatal" into "this is worth a thought" and four invites a scale
// nobody can calibrate.
var Strengths = []string{"decisive", "serious", "minor"}

// Charge is one argument against the slot, and the fact it rests on. Key order
// is Python's, because this reaches a client.
type Charge struct {
	Claim    string `json:"claim"`
	Ground   string `json:"ground"`
	Fact     string `json:"fact"`
	Strength string `json:"strength"`
}

// OnlyCharges keeps the charges that cite something, and counts what did not.
//
// `OnlyQuestions` one mode along. There the predicate is "it ends in a question
// mark", because the failure guarded against is a declarative sentence. Here
// every item is declarative by design, so the predicate moves to the thing that
// actually separates an argument from an opinion: **a charge with no `fact` is
// not a charge.**
//
// The count comes back rather than being swallowed, because "it dropped two" is
// how a mode that has started asserting rather than citing becomes visible
// instead of merely persuasive.
func OnlyCharges(items []any) ([]Charge, int) {
	kept := []Charge{}
	dropped := 0
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			dropped++
			continue
		}
		claim := strings.TrimSpace(asString(row["claim"]))
		fact := strings.TrimSpace(asString(row["fact"]))
		if claim == "" || fact == "" {
			dropped++
			continue
		}
		ground := strings.TrimSpace(asString(row["ground"]))
		strength := strings.TrimSpace(asString(row["strength"]))
		kept = append(kept, Charge{
			Claim: claim,
			// Fall back rather than drop: an unrecognised enum value is a
			// labelling miss, and throwing away a cited argument over its tag
			// would cost more than it protects.
			Ground:   oneOf(ground, Grounds, "ceiling"),
			Fact:     fact,
			Strength: oneOf(strength, Strengths, "minor"),
		})
	}
	return kept, dropped
}

func oneOf(value string, allowed []string, fallback string) string {
	for _, a := range allowed {
		if value == a {
			return value
		}
	}
	return fallback
}

// DroppedAlternatives is what did not survive, by reason.
//
// Kept per reason rather than as a total: "two were off-colour" and "two do not
// exist" say different things about the answer, and a single number says
// neither.
//
// **`AlreadyInDeck` is `omitempty` and the other four are not**, which is a
// wart reproduced rather than tidied. `argue._report`'s default for a run that
// never happened lists FOUR keys -- `not_in_pool`, `banned`, `off_colour`,
// `no_pool` -- while `resolve_alternatives` returns FIVE. So a stance-off
// report and a refused report carry four keys and a real one carries five, and
// a Go struct that always rendered five would differ from Python on exactly the
// paths where no call was made. See argueReport, which builds the empty case
// through a separate type for the same reason.
type DroppedAlternatives struct {
	NotInPool     []string `json:"not_in_pool"`
	Banned        []string `json:"banned"`
	OffColour     []string `json:"off_colour"`
	AlreadyInDeck []string `json:"already_in_deck"`
	NoPool        []string `json:"no_pool"`
}

// emptyDropped is the four-key shape a report with no run carries. Its own
// type rather than a flag, because the difference is which keys exist.
type emptyDropped struct {
	NotInPool []string `json:"not_in_pool"`
	Banned    []string `json:"banned"`
	OffColour []string `json:"off_colour"`
	NoPool    []string `json:"no_pool"`
}

func noneDropped() emptyDropped {
	return emptyDropped{NotInPool: []string{}, Banned: []string{},
		OffColour: []string{}, NoPool: []string{}}
}

// ResolveAlternatives resolves suggested names against the pool and drops what
// does not survive.
//
// Four filters, and Python owns all four because all four have right answers
// (ADR 14). A name that does not resolve is a card the model made up or
// misspelled. A card that is banned cannot go in. **A card outside the
// commander's colour identity cannot go in either, and that check is the reason
// this function exists** -- CLAUDE.md's first recorded error is *Ajani, Nacatl
// Pariah* proposed for a G/W deck, whose back face is R/W and whose identity
// therefore is not. Rule 2: the pool's `color_identity` already accounts for
// back faces, so it is read and never derived.
//
// `inDeck` is the fourth: the pool spellings, casefolded, of every card the
// deck already runs. Checked **first** of the four verdicts because it is the
// most specific true thing to say -- Goreclaw's Primeval Titan is both banned
// and in that deck, and "you already run it" is the answer that helps.
//
// A dropped card is reported under **the pool's spelling**, which for a
// double-faced card is the full `A // B` name: naming both faces is what makes
// an off-colour verdict on Ajani legible rather than baffling.
func ResolveAlternatives(ctx context.Context, conn *pool.Conn, names []any,
	identity []string, inDeck map[string]bool) ([]deckread.NamedCard, DroppedAlternatives, error) {
	wanted := []string{}
	seen := map[string]bool{}
	for _, raw := range names {
		name := strings.TrimSpace(asString(raw))
		key := strings.ToLower(name)
		if name != "" && !seen[key] {
			seen[key] = true
			wanted = append(wanted, name)
		}
	}

	dropped := DroppedAlternatives{NotInPool: []string{}, Banned: []string{},
		OffColour: []string{}, AlreadyInDeck: []string{}, NoPool: []string{}}
	if len(wanted) == 0 {
		return []deckread.NamedCard{}, dropped, nil
	}

	found, err := deckread.CardsNamed(ctx, conn, wanted)
	if err != nil {
		return nil, dropped, err
	}
	if !found.PoolAvailable {
		// Every name comes back unresolved when there is no pool, and filing
		// that under `not_in_pool` would accuse the model of inventing six
		// cards. A base install has no pool and this mode still has to say
		// something true.
		dropped.NoPool = append([]string{}, wanted...)
		return []deckread.NamedCard{}, dropped, nil
	}

	// Indexed under BOTH spellings. A double-faced card resolves from either
	// face but comes back under its full `A // B` name, so an index keyed only
	// on the pool's spelling misses every DFC a model names by its front face
	// -- and misses it *silently*, which is the one thing this function exists
	// not to do. `AskedAs` is what CardsNamed returns for exactly this.
	byKey := map[string]deckread.NamedCard{}
	for _, record := range found.Cards {
		byKey[strings.ToLower(record.Name)] = record
		if record.AskedAs != nil && *record.AskedAs != "" {
			byKey[strings.ToLower(*record.AskedAs)] = record
		}
	}

	dropped.NotInPool = append(dropped.NotInPool, found.NotFound...)
	allowed := map[string]bool{}
	for _, c := range identity {
		allowed[c] = true
	}
	kept := []deckread.NamedCard{}
	already := map[string]bool{}
	for _, name := range wanted {
		record, ok := byKey[strings.ToLower(name)]
		if !ok {
			// Belt and braces: NotFound should already hold it. If some future
			// resolution rule lets a name fall between the two, it is counted
			// here rather than disappearing.
			if !contains(dropped.NotInPool, name) {
				dropped.NotInPool = append(dropped.NotInPool, name)
			}
			continue
		}
		// Two faces of one card are one card. Deduplicated on the pool's
		// spelling rather than the caller's, the only stable identity.
		if already[record.Name] {
			continue
		}
		already[record.Name] = true
		if inDeck[strings.ToLower(record.Name)] {
			dropped.AlreadyInDeck = append(dropped.AlreadyInDeck, record.Name)
			continue
		}
		if !record.LegalCommander {
			dropped.Banned = append(dropped.Banned, record.Name)
			continue
		}
		if !withinIdentity(record.ColorIdentity, allowed) {
			dropped.OffColour = append(dropped.OffColour, record.Name)
			continue
		}
		kept = append(kept, record)
	}
	if len(kept) > MaxAlternatives {
		kept = kept[:MaxAlternatives]
	}
	return kept, dropped, nil
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func withinIdentity(colors []string, allowed map[string]bool) bool {
	for _, c := range colors {
		if !allowed[c] {
			return false
		}
	}
	return true
}

// ArgueNever is the promise this payload carries about itself.
const ArgueNever = "This is the case against the card, and only that. A card " +
	"that survives it still needs a rationale, and the rationale is yours to write."

// ArgueUsage is the same three counters the interview reports.
type ArgueUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
}

// ArgueRequest is what a slot argument needs beyond the deck and the card.
type ArgueRequest struct {
	Requested any
	Focus     string
	Deps      tools.Deps
	Tier      string
	Ledger    *ledger.Recorder
	Limit     *Stance
}

// argueReport is built as ordered fields rather than a struct, because the
// `alternatives_dropped` block has a DIFFERENT SET OF KEYS depending on whether
// a call happened -- see DroppedAlternatives. A single struct cannot express
// that, and smoothing it over would put a fifth key on the wire in the two
// cases Python leaves it off.
func argueReport(turn *Turn, slug, card string, effective Stance,
	charges []Charge, dropped int, alternatives []deckread.NamedCard,
	altDropped any, asked bool, reason string) []wire.KV {
	model, toolCalls, usage := "", []ToolCall{}, ArgueUsage{}
	if turn != nil {
		model = turn.Model
		toolCalls = turn.ToolCalls
		usage = ArgueUsage{InputTokens: turn.InputTokens,
			OutputTokens: turn.OutputTokens, CacheReadTokens: turn.CacheReadTokens}
	}
	if alternatives == nil {
		alternatives = []deckread.NamedCard{}
	}
	return []wire.KV{
		// ADR 14's third boundary made a field, and this mode needs it more
		// than the interview did: an interview returns questions, which nobody
		// mistakes for a verdict, and this returns a reasoned case against a
		// card, which reads like one.
		{Key: "answered_by", Value: "claude"},
		{Key: "mode", Value: ModeSlotArgument},
		{Key: "model", Value: model},
		{Key: "slug", Value: slug},
		{Key: "card", Value: card},
		{Key: "asked", Value: asked},
		{Key: "reason", Value: reason},
		{Key: "stance", Value: Describe(effective)},
		{Key: "charges", Value: charges},
		{Key: "charges_dropped", Value: dropped},
		{Key: "alternatives", Value: alternatives},
		{Key: "alternatives_dropped", Value: altDropped},
		{Key: "tool_calls", Value: toolCalls},
		{Key: "usage", Value: usage},
		{Key: "never", Value: ArgueNever},
	}
}

// Argue makes the case against one card's slot.
//
// Same signature and same stance handling as Interview, deliberately: these are
// the two per-card modes and a client that can drive one should not have to
// learn a second shape to drive the other.
//
// **At `initiative: off` this makes no call and says so** -- `asked: false` and
// a reason, never an empty case that reads as "nothing to say about this card",
// which is the opposite of what an unset dial means.
func Argue(ctx context.Context, conn *pool.Conn, d *deck.Deck, card string,
	req ArgueRequest) ([]wire.KV, error) {
	facts, err := Brief(ctx, conn, d, card)
	if err != nil {
		return nil, err
	}
	cardFacts, _ := kv(facts, "card").(wire.OrderedMap)
	name := asString(kv(cardFacts, "name"))
	deckFacts, _ := kv(facts, "deck").(wire.OrderedMap)

	effective, err := Resolve(req.Requested,
		statusOnly{asString(kv(deckFacts, "status"))}, req.Limit)
	if err != nil {
		return nil, err
	}

	if !effective.AllowsCalls() {
		return argueReport(nil, d.Slug, name, effective, []Charge{}, 0, nil,
			noneDropped(), false,
			"The stance is off, so no call was made. Everything else about "+
				"this deck still works."), nil
	}

	mode, err := GetMode(ModeSlotArgument)
	if err != nil {
		return nil, err
	}
	opening, err := argueOpening(facts, req.Focus)
	if err != nil {
		return nil, err
	}

	turn, err := Converse(ctx, mode, Request{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(opening)),
		},
		Stance: effective, Deps: req.Deps, Tier: req.Tier, Ledger: req.Ledger,
	})
	if err != nil {
		return nil, err
	}

	if turn.Refused {
		return argueReport(&turn, d.Slug, name, effective, []Charge{}, 0, nil,
			noneDropped(), true, "The model declined to answer this one."), nil
	}

	var payload struct {
		Charges      []any `json:"charges"`
		Alternatives []any `json:"alternatives"`
	}
	if err := turn.Parsed(&payload); err != nil {
		//nolint:nilerr // an unreadable answer is a reported outcome, not a fault
		return argueReport(&turn, d.Slug, name, effective, []Charge{}, 0, nil,
			noneDropped(), true,
			fmt.Sprintf("The answer did not parse (stop reason: %s). Nothing "+
				"was written.", turn.StopReason)), nil
	}

	charges, dropped := OnlyCharges(payload.Charges)
	if len(charges) > MaxCharges {
		charges = charges[:MaxCharges]
	}

	// The deck's own names, read off the deck we already hold rather than
	// carried in the brief: the brief is what the model reads, and 99 names in
	// the prompt would be tokens spent teaching it a rule Python enforces
	// anyway.
	inDeck := map[string]bool{}
	for _, c := range d.Cards {
		inDeck[strings.ToLower(c.Name)] = true
	}
	for _, c := range d.Commander {
		inDeck[strings.ToLower(c)] = true
	}
	if d.Companion != nil && *d.Companion != "" {
		inDeck[strings.ToLower(*d.Companion)] = true
	}

	// **Loud, not silent.** `Brief` takes this straight from `DeckPayload`,
	// where it is a `[]string`, so a failed assertion means that shape moved
	// underneath this file. The quiet version -- fall back to an empty
	// identity -- would make EVERY alternative off-colour and report each one
	// as the model's mistake, which is the most misleading answer this
	// function can give.
	identity, ok := kv(deckFacts, "color_identity").([]string)
	if !ok {
		return nil, fmt.Errorf("the brief's color_identity is %T, not []string"+
			" -- the deck payload's shape moved", kv(deckFacts, "color_identity"))
	}
	alternatives, altDropped, err := ResolveAlternatives(ctx, conn,
		payload.Alternatives, identity, inDeck)
	if err != nil {
		return nil, err
	}
	return argueReport(&turn, d.Slug, name, effective, charges, dropped,
		alternatives, altDropped, true, ""), nil
}

// argueOpening is the first user message: the facts, then the ask.
func argueOpening(facts wire.OrderedMap, focus string) (string, error) {
	raw, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return "", fmt.Errorf("rendering the brief: %w", err)
	}
	lines := []string{
		"Here is everything the tool already knows about this card in this " +
			"deck. All of it is deterministic: the card text is the pool's, " +
			"the verdict is the gate's, the counts are computed.",
		"",
		string(raw),
		"",
		"Make the strongest honest case that this card does not deserve its " +
			"slot. If that case is weak, return fewer charges and mark them " +
			"minor rather than padding it.",
	}
	if strings.TrimSpace(focus) != "" {
		lines = append(lines, "",
			"What I am weighing, in my words: "+strings.TrimSpace(focus))
	}
	return strings.Join(lines, "\n"), nil
}
