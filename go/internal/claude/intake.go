// The two surfaces an imported deck's intake can call (ADR 41).
//
// Both are read-only, like every other mode in this package and for the same
// reason: `may_write` is empty, `NewMode` refuses anything else, and
// `boundary_test.go` bans this whole tree from the write engine transitively.
// What ADR 41 changed is not the surface but the caller -- `internal/api`'s
// intake takes what these answer and writes it through `deckedit`, which is a
// door this package still cannot open.
//
// **They are batched, and the batch size is the interesting decision.** An
// imported deck owes ninety-nine rationales; one call per card would be
// ninety-nine round trips, and one call for all ninety-nine would be a single
// answer that fails whole. Chunking is the middle, and it is not only a cost
// argument: a chunk that comes back unparseable costs its own cards and not
// the deck, so partial success is the normal outcome rather than an edge case.
//
// **Neither ever returns a card it was not asked about, and neither is
// required to return every card it was asked about.** The first is enforced
// here -- an answer naming a card outside the ask is dropped and counted --
// and the second is the point of the prompts: a card the model cannot ground
// is left out, and its owner writes that one, which is exactly where they were
// before the intake ran.
package claude

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// IntakeChunk is how many cards go into one call.
//
// Chosen against the shape of a Commander deck rather than a round number: a
// 99-card list with its lands already filed leaves roughly sixty cards to
// think about, so twenty is three calls and a short third one. Small enough
// that a chunk failing loses a fifth of the work and not all of it; large
// enough that the deck's context is worth sending three times rather than
// sixty.
const IntakeChunk = 20

// Draft is one rationale a model wrote, before anything has written it down.
//
// `Fact` never reaches the deck file. It is required by the schema so that a
// draft has to rest on something -- a sentence the model cannot ground has
// nowhere to put its grounding and is left out instead -- and it is carried
// here so the review queue can show what a draft was based on. A rationale
// whose fact is visible is a rationale somebody can disagree with, which is
// the whole point of drafting one for them.
type Draft struct {
	Card string `json:"card"`
	Why  string `json:"why"`
	Fact string `json:"fact"`
}

// Filing is one card's macro category, before anything has written it down.
type Filing struct {
	Card     string `json:"card"`
	Category string `json:"category"`
	Fact     string `json:"fact"`
}

// IntakeRequest is what both intake surfaces need. The same shape as the
// other modes' requests and for the same reason (ADR 39): a job outlives the
// request that planned it, so everything it needs travels with it as a value.
type IntakeRequest struct {
	Endpoint  Endpoint
	Requested any
	Deps      tools.Deps
	Tier      string
	Ledger    *ledger.Recorder
	Limit     *Stance
}

// IntakeOutcome is what one intake surface did, whether or not it did it.
//
// `Skipped` is the count of answers thrown away for naming a card outside the
// ask -- reported rather than silently dropped, because a mode answering about
// cards nobody asked about is a fact about the prompt and somebody should be
// able to see it climb.
//
// `Unanswered` is the other half of that reconciliation, and it was missing.
// `Skipped` counts rows that came back and were *rejected*; a card the model
// simply never mentioned came back as nothing at all and was counted nowhere.
// Asked for eighty-five and given eighty-four, the intake wrote eighty-four
// and said so in a number -- and which card had been left was recoverable
// only by diffing your own deck against your own paste.
//
// Names rather than a count, because the count was already there and was not
// the useful half. It is the deck's own spelling, so it matches what the page
// will show.
type IntakeOutcome struct {
	Stance     Stance
	Asked      bool
	Reason     string
	Skipped    int
	Unanswered []string
}

// DraftRationales asks for a `why` on each named card.
//
// The stance is resolved and clamped exactly as every other surface does it,
// and at `initiative: off` no call is made and the outcome says so. **The
// write axis is NOT checked here**: this function answers, it does not write,
// and conflating "may Claude speak" with "may the intake write" inside a
// read-only package is how the gate ends up in the wrong layer. `internal/api`
// checks `MayWrite` before it calls this and again before it writes.
func DraftRationales(ctx context.Context, d *deck.Deck, cards []string,
	req IntakeRequest) ([]Draft, IntakeOutcome, error) {

	effective, err := Resolve(req.Requested, statusOnly{d.Status}, req.Limit)
	if err != nil {
		return nil, IntakeOutcome{}, err
	}
	if !effective.AllowsCalls() {
		return nil, IntakeOutcome{Stance: effective, Reason: draftOffReason}, nil
	}
	mode, err := GetMode(ModeRationaleDraft)
	if err != nil {
		return nil, IntakeOutcome{}, err
	}

	wanted := foldedSet(cards)
	out := []Draft{}
	skipped := 0
	for _, chunk := range chunked(cards, IntakeChunk) {
		var payload struct {
			Drafts []Draft `json:"drafts"`
		}
		asked, err := askIntake(ctx, mode, effective, req,
			draftOpening(d, chunk), &payload)
		if err != nil {
			return nil, IntakeOutcome{}, err
		}
		if !asked {
			continue
		}
		for _, row := range payload.Drafts {
			proper, held := wanted[Casefold(row.Card)]
			if !held || strings.TrimSpace(row.Why) == "" {
				skipped++
				continue
			}
			// The deck's own spelling, never the model's: a name that came
			// back with different casing has to match the card the write will
			// look for, and `deckedit` locates by name.
			row.Card = proper
			out = append(out, row)
		}
	}
	return out, IntakeOutcome{Stance: effective, Asked: true, Skipped: skipped,
		Unanswered: unanswered(cards, wanted, drafted(out))}, nil
}

// drafted is the set of cards an answer actually arrived for, casefolded.
func drafted(out []Draft) map[string]bool {
	got := make(map[string]bool, len(out))
	for _, dr := range out {
		got[Casefold(dr.Card)] = true
	}
	return got
}

// unanswered is what was asked about and never came back.
//
// **The half of the reconciliation that was missing.** `Skipped` counts rows
// that arrived and were rejected; a card the model simply never mentioned
// arrived as nothing and was counted nowhere -- so an intake asked for
// eighty-five and given eighty-four wrote eighty-four, reported the two
// numbers, and named the missing card to nobody.
//
// Walked in the order the cards were asked in rather than over a map, because
// this list is put in front of a person and an order nobody chose reads as a
// fault of its own.
func unanswered(cards []string, wanted map[string]string, got map[string]bool) []string {
	out := []string{}
	for _, name := range cards {
		folded := Casefold(name)
		if proper, held := wanted[folded]; held && !got[folded] {
			out = append(out, proper)
		}
	}
	return out
}

// FileCards asks which macro category each named card belongs under.
//
// The categories are an enum in the mode's response schema, so an answer
// outside the twelve cannot come back -- which is why nothing here re-checks
// the vocabulary and why the enum is where that check belongs. A category
// invented in prose would have to be validated in three places; one that
// cannot be returned needs validating nowhere.
func FileCards(ctx context.Context, d *deck.Deck, cards []string,
	req IntakeRequest) ([]Filing, IntakeOutcome, error) {

	effective, err := Resolve(req.Requested, statusOnly{d.Status}, req.Limit)
	if err != nil {
		return nil, IntakeOutcome{}, err
	}
	if !effective.AllowsCalls() {
		return nil, IntakeOutcome{Stance: effective, Reason: draftOffReason}, nil
	}
	mode, err := GetMode(ModeIntakeFiling)
	if err != nil {
		return nil, IntakeOutcome{}, err
	}

	wanted := foldedSet(cards)
	out := []Filing{}
	skipped := 0
	for _, chunk := range chunked(cards, IntakeChunk) {
		var payload struct {
			Filings []Filing `json:"filings"`
		}
		asked, err := askIntake(ctx, mode, effective, req,
			filingOpening(d, chunk), &payload)
		if err != nil {
			return nil, IntakeOutcome{}, err
		}
		if !asked {
			continue
		}
		for _, row := range payload.Filings {
			proper, held := wanted[Casefold(row.Card)]
			if !held || strings.TrimSpace(row.Category) == "" {
				skipped++
				continue
			}
			row.Card = proper
			out = append(out, row)
		}
	}
	got := make(map[string]bool, len(out))
	for _, f := range out {
		got[Casefold(f.Card)] = true
	}
	return out, IntakeOutcome{Stance: effective, Asked: true, Skipped: skipped,
		Unanswered: unanswered(cards, wanted, got)}, nil
}

// Description is what a deck is trying to do, before anything has written it
// down. `Themes` are the deck's index terms and `Strategy` is the paragraph;
// both are ordinary deck fields a person can edit afterwards, and neither is
// marked -- `why_by` exists because a rationale is a claim about somebody's
// thinking, and a strategy paragraph in a draft deck is not the same object.
type Description struct {
	Strategy string   `json:"strategy"`
	Themes   []string `json:"themes"`
	Fact     string   `json:"fact"`
}

// DescribeDeck asks what the deck is trying to do.
//
// One call rather than chunked: unlike the other two this is a question about
// the deck as a whole, and asking it in twenty-card pieces would produce five
// paragraphs about five fifths of a deck.
func DescribeDeck(ctx context.Context, d *deck.Deck,
	req IntakeRequest) (Description, IntakeOutcome, error) {

	effective, err := Resolve(req.Requested, statusOnly{d.Status}, req.Limit)
	if err != nil {
		return Description{}, IntakeOutcome{}, err
	}
	if !effective.AllowsCalls() {
		return Description{}, IntakeOutcome{Stance: effective, Reason: draftOffReason}, nil
	}
	mode, err := GetMode(ModeDeckDescription)
	if err != nil {
		return Description{}, IntakeOutcome{}, err
	}
	var payload Description
	asked, err := askIntake(ctx, mode, effective, req, describeOpening(d), &payload)
	if err != nil {
		return Description{}, IntakeOutcome{}, err
	}
	if !asked || strings.TrimSpace(payload.Strategy) == "" {
		return Description{}, IntakeOutcome{Stance: effective, Asked: true,
			Reason: "No description came back, so the deck keeps the one it had."}, nil
	}
	return payload, IntakeOutcome{Stance: effective, Asked: true}, nil
}

func describeOpening(d *deck.Deck) string {
	return strings.Join([]string{
		intakeDeckFacts(d),
		"",
		"Say what this deck is trying to do: three or four plain sentences " +
			"for somebody who has never seen the list, including one clause on " +
			"what it is bad at. Then give its index terms.",
		"",
		"Read the list before you write. The commander's reputation is not " +
			"evidence about this deck -- what is in it is.",
	}, "\n")
}

// IntakeDefaultPreset is what "no preference" means on the import screen.
//
// `consultant` and deliberately not higher: it speaks when spoken to, which is
// exactly what a ticked box is, and it writes nothing -- so the four actions
// that were always allowed are offered and the one ADR 41 gated is not, until
// the person raises their own stance. The sheet then says where that setting
// is, which is a better first meeting with the feature than a control that
// appears without being asked for.
const IntakeDefaultPreset = "consultant"

// IntakeStanceFor is this surface's default stance, and the clamp over what
// was asked for.
//
// **The import screen has no deck to derive a default from**, which is the
// same hole `research` and `scan` sit in and it bites harder here: the deck
// this is about does not exist yet, by construction, so `Resolve(nil, nil)`
// answers `off` and the whole sheet would stand down for every user on the
// one page it belongs to. Found by loading the page rather than by reading
// the code -- the component's own tests mock the dial, so they could not see
// it, and `dialSurfaces` is the third time this exact absence has produced
// the same bug.
func IntakeStanceFor(requested any, limit *Stance) (Stance, error) {
	if requested == nil {
		ceil := Ceiling()
		if limit != nil {
			ceil = *limit
		}
		preset, err := Preset(IntakeDefaultPreset)
		if err != nil {
			return Stance{}, err
		}
		return Clamp(preset, ceil), nil
	}
	return Resolve(requested, nil, limit)
}

const draftOffReason = "The stance is off, so no call was made. Everything " +
	"else about this deck still works."

// askIntake runs one chunk and decodes it, reporting whether the answer is
// usable rather than failing the whole intake on one bad chunk.
//
// A refusal or an unparseable answer returns `false, nil` on purpose: the
// caller carries on with the next chunk, and the cards in this one keep the
// state they already had, which is a blank rationale or a `utility` filing.
// Both are exactly where the deck was before the intake ran, so a lost chunk
// costs nothing that was not already owed.
func askIntake(ctx context.Context, mode Mode, effective Stance,
	req IntakeRequest, opening string, into any) (bool, error) {

	turn, err := Converse(ctx, mode, Request{
		Endpoint: req.Endpoint,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(opening)),
		},
		Stance: effective, Deps: req.Deps, Tier: req.Tier, Ledger: req.Ledger,
	})
	if err != nil {
		return false, err
	}
	if turn.Refused {
		return false, nil
	}
	if err := turn.Parsed(into); err != nil {
		return false, nil //nolint:nilerr // a chunk that did not parse costs its own cards
	}
	return true, nil
}

// draftOpening is the first user message for one chunk of cards.
func draftOpening(d *deck.Deck, chunk []string) string {
	return strings.Join([]string{
		intakeDeckFacts(d),
		"",
		"Draft the `why` for these cards, which have none:",
		bulleted(chunk),
		"",
		"One or two plain sentences each, saying what the card is doing in " +
			"THIS deck. Leave out any card you cannot ground in its real text " +
			"-- a card you skip costs nothing, and its owner writes that one.",
	}, "\n")
}

// filingOpening is the same for the filing pass.
func filingOpening(d *deck.Deck, chunk []string) string {
	return strings.Join([]string{
		intakeDeckFacts(d),
		"",
		"File these cards under the heading that describes the job each one " +
			"does in this deck:",
		bulleted(chunk),
		"",
		"Lands are already filed by the card pool and are not in this list.",
	}, "\n")
}

// intakeDeckFacts is what the deck is, in the deck's own words.
//
// Deliberately small. The tools carry the detail -- `get_deck` has the whole
// list, `get_cards` has the oracle text, `deck_stats` has the counts -- and
// repeating all of that in the opening would send the same deck three times
// per chunk to say what one tool call says on demand. What has to be here is
// the identity, because a model that has to call a tool to learn whose deck
// this is will write the first chunk without knowing.
func intakeDeckFacts(d *deck.Deck) string {
	lines := []string{
		fmt.Sprintf("The deck is %q (slug %s), stage %s, status %s.",
			d.Name, d.Slug, d.Stage, d.Status),
	}
	if len(d.Commander) > 0 {
		lines = append(lines, "Commander: "+strings.Join(d.Commander, " + ")+".")
	}
	if len(d.Themes) > 0 {
		lines = append(lines, "Themes the owner declared: "+strings.Join(d.Themes, ", ")+".")
	}
	// The rationales already written, which are the deck's vocabulary. Capped
	// because they are examples of how this owner writes rather than a
	// transcript, and a chunk that spends its context on sixty of somebody
	// else's sentences has less room for the twenty it was asked about.
	written := []string{}
	for _, c := range d.Cards {
		if why := strings.TrimSpace(c.Why); why != "" && c.WhyBy == "" {
			written = append(written, fmt.Sprintf("- %s (%s): %s", c.Name, c.Category, why))
		}
		if len(written) == 6 {
			break
		}
	}
	if len(written) > 0 {
		lines = append(lines,
			"", "Rationales the owner has already written, so you can hear how "+
				"they write. Match the register, not the wording:", strings.Join(written, "\n"))
	}
	return strings.Join(lines, "\n")
}

func bulleted(names []string) string {
	rows := make([]string, 0, len(names))
	for _, n := range names {
		rows = append(rows, "- "+n)
	}
	return strings.Join(rows, "\n")
}

// foldedSet maps each name to itself under the fold, so an answer can be
// matched back to the deck's own spelling.
func foldedSet(names []string) map[string]string {
	out := make(map[string]string, len(names))
	for _, n := range names {
		out[Casefold(n)] = n
	}
	return out
}

// chunked splits a list into runs of at most size. A nil or empty list yields
// nothing, so a caller with no work makes no calls.
func chunked(names []string, size int) [][]string {
	out := [][]string{}
	for start := 0; start < len(names); start += size {
		end := min(start+size, len(names))
		out = append(out, names[start:end])
	}
	return out
}
