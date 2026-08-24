package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/analyze"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The rationale interview: it asks, you answer, your answer is the rationale.
//
// The first mode, and the one ADR 15 was written before rather than after --
// because this is exactly where rule 4 is easiest to lose. `decks import`
// brings in ninety-nine cards owing ninety-nine rationales, the obvious help is
// a tool that writes them, and every step from "ask a question" to "summarise
// what we just discussed into a rationale" is small and defensible on its own.
// So the line is drawn where it can be checked instead of promised.
//
// Three things hold it, and none of them is the system prompt:
//
// **There is no write door.** `tools` is the entire surface this package can
// reach, and `boundary_test.go` fails on the commit that names a write
// function anywhere under this tree. The prompt asks the model not to draft a
// rationale; the reason that request is safe to rely on is that nothing bad
// happens if it is ignored.
//
// **The answer's shape has no field for one.** The response schema has a slot
// for a question and a slot for the pool fact behind it, and no slot for prose
// about the card's merit. It lives as recorded data for exactly that
// reason -- see modes.go.
//
// **Every question is checked to be a question.** `OnlyQuestions` drops
// anything that does not end in a question mark and reports how many it
// dropped. Blunt, and meant to be.
//
// **Facts arrive before the model does.** Rule 1 says card facts come from the
// pool rather than recall. The mode has `get_cards` and is told to use it, but
// the card actually under discussion never depends on that: `Brief` assembles
// the oracle text, the gate's verdict, the category counts and the sibling
// rationales in deterministic Go and hands them over in the opening message.
// The tool is for the cards the conversation *reaches for*; the card in front
// of it is already on the table.

// ErrCardNotInDeck is asked about a card the deck does not run.
//
// Its own type because the caller's response differs: an unknown deck is a 404
// and this is a 422 -- the deck is fine, the question is not.
type ErrCardNotInDeck struct{ Card, Slug string }

func (e *ErrCardNotInDeck) Error() string {
	return fmt.Sprintf("%s is not in %s. The interview argues about a card "+
		"already in a deck; adding one is a different operation.",
		wire.Quote(e.Card), e.Slug)
}

// MaxSiblings caps the sibling rationales, which are the most useful thing in
// the brief and the most expensive. Ten is enough to see a category's shape.
const MaxSiblings = 10

// MaxQuestions: more than this and it stops being an interview and starts
// being a wall.
const MaxQuestions = 6

// Question is one thing to answer, and the fact it rests on. Key order is
// the recorded one, because this reaches a client.
type Question struct {
	Question string `json:"question"`
	Angle    string `json:"angle"`
	Fact     string `json:"fact"`
}

// OnlyQuestions keeps the questions and counts what was not one.
//
// The whole rule in one predicate: a question ends in a question mark. It is
// crude, it will occasionally drop something serviceable, and that is the right
// trade -- the failure this guards against is a declarative sentence appearing
// in the column beside an empty rationale box, which reads as a draft whatever
// the surrounding UI calls it.
//
// The count comes back rather than being swallowed, because "it dropped two" is
// how a prompt that has started editorialising becomes visible.
func OnlyQuestions(items []any) ([]Question, int) {
	kept := []Question{}
	dropped := 0
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			dropped++
			continue
		}
		text := strings.TrimSpace(asString(row["question"]))
		if !strings.HasSuffix(text, "?") || len(text) <= 1 {
			dropped++
			continue
		}
		angle := asString(row["angle"])
		if angle == "" {
			angle = "role"
		}
		kept = append(kept, Question{
			Question: text,
			Angle:    angle,
			Fact:     strings.TrimSpace(asString(row["fact"])),
		})
	}
	return kept, dropped
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// ---------------------------------------------------------------- the facts

// Brief is everything deterministic Go knows about this card in this deck.
//
// Assembled rather than asked for. The mode could reach most of this through
// its tools, and would then be one forgotten tool call away from asking a
// question about a card it had not actually read -- rule 1 failing in the one
// place this feature spends all its time. Handing the facts over costs one
// round trip's worth of tokens and removes the failure mode. It matters most in
// the case this project is deliberately wrong about: a banned card comes back
// with its real text and `legal_commander: false`, so the interview can ask
// about the ban rather than discovering it cannot look the card up.
//
// **Ordered, and that is not tidiness.** The brief is serialised straight into
// the opening message, so its key order is part of the bytes the model reads
// and part of what the prompt cache hashes. A `map[string]any` here would be
// alphabetised by encoding/json -- the trap that shipped an alphabetised Notes
// tab -- so every level is `[]wire.KV`, in the recorded order.
func Brief(ctx context.Context, conn *pool.Conn, d *deck.Deck, card string) (wire.OrderedMap, error) {
	payload, err := deckread.DeckPayload(ctx, conn, d, false, "")
	if err != nil {
		return nil, err
	}
	entry, err := findCard(payload, d.Slug, card)
	if err != nil {
		return nil, err
	}
	name := entry.Name

	named, err := deckread.CardsNamed(ctx, conn, []string{name})
	if err != nil {
		return nil, err
	}
	var record any
	if len(named.Cards) > 0 {
		record = named.Cards[0]
	}

	report, err := deckread.Validate(ctx, conn, d)
	if err != nil {
		return nil, err
	}
	stats, err := deckread.Stats(ctx, conn, d)
	if err != nil {
		return nil, err
	}

	category := entry.Category
	row := categoryRow(stats, category)
	var commanderText any
	if cc, ok := kv(payload, "commander_card").(deckread.CardJSON); ok {
		commanderText = cc.OracleText
	}

	return wire.OrderedMap{
		{Key: "deck", Value: wire.OrderedMap{
			{Key: "slug", Value: kv(payload, "slug")},
			{Key: "name", Value: kv(payload, "name")},
			{Key: "commander", Value: kv(payload, "commander")},
			// The deck's axis, in the commander's own words. Half the useful
			// questions are some form of "what does this do for *that*".
			{Key: "commander_text", Value: commanderText},
			{Key: "bracket", Value: kv(payload, "bracket")},
			{Key: "stage", Value: kv(payload, "stage")},
			{Key: "status", Value: kv(payload, "status")},
			{Key: "color_identity", Value: kv(payload, "color_identity")},
			{Key: "strategy", Value: kv(payload, "strategy")},
			{Key: "total_cards", Value: kv(payload, "total_cards")},
			{Key: "land_count", Value: kv(payload, "land_count")},
			{Key: "cards_still_owing_a_rationale", Value: kv(payload, "needs_rationale")},
		}},
		{Key: "card", Value: wire.OrderedMap{
			{Key: "name", Value: name},
			{Key: "category", Value: category},
			{Key: "quantity", Value: entry.Qty},
			// Named for what it is. A blank here is the normal case after an
			// import, not an error, and the questions differ: with a rationale
			// the job is to interrogate it, without one it is to find it.
			{Key: "rationale_so_far", Value: entry.Why},
			{Key: "in_pool", Value: record != nil},
			{Key: "pool", Value: record},
		}},
		{Key: "gate", Value: wire.OrderedMap{
			{Key: "about_this_card", Value: issuesAbout(report, name)},
			{Key: "deck_errors", Value: len(report.Errors())},
			{Key: "deck_warnings", Value: len(report.Warnings())},
		}},
		{Key: "category", Value: wire.OrderedMap{
			{Key: "name", Value: category},
			{Key: "count", Value: pick(row, func(c analyze.Category) any { return c.Count })},
			{Key: "target_low", Value: pick(row, func(c analyze.Category) any { return c.TargetLow })},
			{Key: "target_high", Value: pick(row, func(c analyze.Category) any { return c.TargetHigh })},
			{Key: "verdict", Value: pick(row, func(c analyze.Category) any { return c.Status })},
			{Key: "other_cards_in_it", Value: siblingRationales(payload, category, name)},
		}},
		{Key: "curve", Value: wire.OrderedMap{
			{Key: "average_mana_value", Value: stats.Curve.AverageMV},
			{Key: "cards_at_this_mana_value", Value: curveBucket(stats, entry, record)},
		}},
	}, nil
}

// findCard is the card's entry in the deck, matched the way a person would type
// it -- and across all three places a card can live: the 99, the swap board,
// and the commander. A commander holds a slot and carries a rationale like
// anything else, so it is interviewable.
func findCard(payload wire.OrderedMap, slug, name string) (deckread.CardJSON, error) {
	wanted := strings.ToLower(strings.TrimSpace(name))
	for _, entry := range allEntries(payload) {
		if strings.ToLower(entry.Name) == wanted {
			return entry, nil
		}
	}
	return deckread.CardJSON{}, &ErrCardNotInDeck{Card: name, Slug: slug}
}

// allEntries is every card row the payload holds, in the recorded search
// order.
//
// The commander arrives as a single row rather than a list, which is why this
// is not one loop: `commander_card` is null for a deck whose commander the pool
// does not know, and a type switch is what tells that from an empty list.
func allEntries(payload wire.OrderedMap) []deckread.CardJSON {
	var out []deckread.CardJSON
	for _, key := range []string{"cards", "swap_board"} {
		if rows, ok := kv(payload, key).([]deckread.CardJSON); ok {
			out = append(out, rows...)
		}
	}
	switch commander := kv(payload, "commander_card").(type) {
	case deckread.CardJSON:
		out = append(out, commander)
	case *deckread.CardJSON:
		if commander != nil {
			out = append(out, *commander)
		}
	}
	return out
}

// kv reads one key out of an ordered payload. Linear, and deliberately so: the
// payloads here are tens of keys, and building a map to index them would be
// the very thing the ordering exists to avoid.
func kv(rows wire.OrderedMap, key string) any {
	for _, row := range rows {
		if row.Key == key {
			return row.Value
		}
	}
	return nil
}

func issuesAbout(report *gate.Report, name string) []wire.OrderedMap {
	out := []wire.OrderedMap{}
	wanted := strings.ToLower(name)
	for _, pair := range []struct {
		severity string
		issues   []gate.Issue
	}{{"error", report.Errors()}, {"warning", report.Warnings()}} {
		for _, issue := range pair.issues {
			// `card` is null for a deck-level issue -- a wrong card count, a
			// missing commander -- and those are not about this card.
			if issue.Card == nil || strings.ToLower(*issue.Card) != wanted {
				continue
			}
			out = append(out, wire.OrderedMap{
				{Key: "severity", Value: pair.severity},
				{Key: "code", Value: issue.Code},
				{Key: "message", Value: issue.Message},
			})
		}
	}
	return out
}

func categoryRow(stats analyze.Stats, category string) *analyze.Category {
	for i := range stats.Categories {
		if stats.Categories[i].Category == category {
			return &stats.Categories[i]
		}
	}
	return nil
}

// pick reads a field off the category row, or nil when the deck has no such
// category -- which is a real state, not an error: a card can be the only
// member of a category the bracket has no target for.
func pick(row *analyze.Category, field func(analyze.Category) any) any {
	if row == nil {
		return nil
	}
	return field(*row)
}

func siblingRationales(payload wire.OrderedMap, category, name string) []wire.OrderedMap {
	out := []wire.OrderedMap{}
	rows, _ := kv(payload, "cards").([]deckread.CardJSON)
	for _, entry := range rows {
		if entry.Category != category || entry.Name == name {
			continue
		}
		out = append(out, wire.OrderedMap{
			{Key: "name", Value: entry.Name},
			{Key: "why", Value: entry.Why},
		})
		if len(out) == MaxSiblings {
			break
		}
	}
	return out
}

// curveBucket is how many cards share this card's mana value. The pool's `cmc`
// wins over the deck entry's: the deck file records what
// somebody typed and the pool records what the card costs.
func curveBucket(stats analyze.Stats, entry deckread.CardJSON, record any) any {
	mv, ok := manaValue(entry, record)
	if !ok {
		return nil
	}
	for _, b := range stats.Curve.Buckets {
		if float64(b.MV) == mv {
			return b.Count
		}
	}
	return nil
}

func manaValue(entry deckread.CardJSON, record any) (float64, bool) {
	if card, ok := record.(deckread.NamedCard); ok {
		return card.CMC, true
	}
	if entry.CMC != nil {
		return *entry.CMC, true
	}
	return 0, false
}

// --------------------------------------------------------------- the answer

// InterviewReport is one response shape for every outcome, including not
// asking at all.
//
// `answered_by` is ADR 14's third boundary made a field: the gate's output and
// Claude's are never allowed to share a surface without a label, and a caller
// that has to infer which one it is holding will eventually infer wrong.
type InterviewReport struct {
	AnsweredBy       string         `json:"answered_by"`
	Mode             string         `json:"mode"`
	Model            string         `json:"model"`
	Slug             string         `json:"slug"`
	Card             string         `json:"card"`
	Asked            bool           `json:"asked"`
	Reason           string         `json:"reason"`
	Stance           StanceReadout  `json:"stance"`
	Questions        []Question     `json:"questions"`
	QuestionsDropped int            `json:"questions_dropped"`
	ToolCalls        []ToolCall     `json:"tool_calls"`
	Usage            InterviewUsage `json:"usage"`
	// Never is said in the payload rather than only in the UI, so a second
	// client cannot render this as anything other than what it is.
	Never string `json:"never"`
}

// InterviewUsage carries the cache figure deliberately: zero across repeated
// interviews means the prefix is drifting, and a number nobody can see is a
// number nobody checks.
type InterviewUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
}

// NeverSentence is the promise the payload carries about itself.
const NeverSentence = "These are questions. The rationale is yours to write."

func interviewReport(turn *Turn, slug, card string, effective Stance,
	questions []Question, dropped int, asked bool, reason string) InterviewReport {
	report := InterviewReport{
		AnsweredBy: "claude", Mode: ModeRationaleInterview,
		Slug: slug, Card: card, Asked: asked, Reason: reason,
		Stance: Describe(effective), Questions: questions,
		QuestionsDropped: dropped, ToolCalls: []ToolCall{},
		Never: NeverSentence,
	}
	if turn != nil {
		report.Model = turn.Model
		report.ToolCalls = turn.ToolCalls
		report.Usage = InterviewUsage{
			InputTokens: turn.InputTokens, OutputTokens: turn.OutputTokens,
			CacheReadTokens: turn.CacheReadTokens,
		}
	}
	return report
}

// InterviewRequest is what an interview needs beyond the deck and the card.
type InterviewRequest struct {
	// Endpoint is where this call goes. Carried on the plan rather than
	// looked up, so a background job that outlives its request still knows
	// which endpoint it was planned against (ADR 39).
	Endpoint Endpoint
	// Requested is the stance in any form Resolve accepts.
	Requested any
	// Focus is the user's own steer, quoted rather than interpolated.
	Focus  string
	Deps   tools.Deps
	Tier   string
	Ledger *ledger.Recorder
	// Limit is the deployment's ceiling; nil reads it from the environment.
	Limit *Stance
}

// Interview asks the user about one card's slot.
//
// The stance is resolved against the deck's own default and then clamped to the
// deployment's ceiling, so a client asking for more than a hosted instance
// allows gets the instance's answer rather than an error.
//
// **At `initiative: off` this makes no call and says so.** Not an empty list
// that looks like it had nothing to say -- `asked: false` and a reason.
func Interview(ctx context.Context, conn *pool.Conn, d *deck.Deck, card string,
	req InterviewRequest) (InterviewReport, error) {
	facts, err := Brief(ctx, conn, d, card)
	if err != nil {
		return InterviewReport{}, err
	}
	cardFacts, _ := kv(facts, "card").(wire.OrderedMap)
	name := asString(kv(cardFacts, "name"))
	deckFacts, _ := kv(facts, "deck").(wire.OrderedMap)

	effective, err := Resolve(req.Requested,
		statusOnly{asString(kv(deckFacts, "status"))}, req.Limit)
	if err != nil {
		return InterviewReport{}, err
	}

	if !effective.AllowsCalls() {
		return interviewReport(nil, d.Slug, name, effective, []Question{}, 0, false,
			"The stance is off, so no call was made. Everything else about "+
				"this deck still works."), nil
	}

	mode, err := GetMode(ModeRationaleInterview)
	if err != nil {
		return InterviewReport{}, err
	}
	opening, err := interviewOpening(facts, req.Focus)
	if err != nil {
		return InterviewReport{}, err
	}

	turn, err := Converse(ctx, mode, Request{
		Endpoint: req.Endpoint,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(opening)),
		},
		Stance: effective, Deps: req.Deps, Tier: req.Tier, Ledger: req.Ledger,
	})
	if err != nil {
		return InterviewReport{}, err
	}

	if turn.Refused {
		return interviewReport(&turn, d.Slug, name, effective, []Question{}, 0, true,
			"The model declined to answer this one."), nil
	}

	var payload struct {
		Questions []any `json:"questions"`
	}
	if err := turn.Parsed(&payload); err != nil {
		// The schema makes this close to impossible, and "close to" is why the
		// branch exists: a truncated answer is not valid JSON either.
		//
		// The nil error is the point rather than an oversight, which is what
		// the linter is asking about. A conversation that happened and came
		// back unreadable is a *report* -- it cost tokens, it has a stop
		// reason worth showing, and the caller wants to render "the answer did
		// not parse" beside the stance and the usage. Returning the decode
		// error instead would throw all of that away and turn a 200 with an
		// explanation into a 500 with none.
		//nolint:nilerr // an unreadable answer is a reported outcome, not a fault
		return interviewReport(&turn, d.Slug, name, effective, []Question{}, 0, true,
			fmt.Sprintf("The answer did not parse (stop reason: %s). Nothing "+
				"was written.", turn.StopReason)), nil
	}

	questions, dropped := OnlyQuestions(payload.Questions)
	if len(questions) > MaxQuestions {
		questions = questions[:MaxQuestions]
	}
	return interviewReport(&turn, d.Slug, name, effective, questions, dropped, true, ""), nil
}

// interviewOpening is the first user message: the facts, then the ask.
func interviewOpening(facts wire.OrderedMap, focus string) (string, error) {
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
		"Ask three to five questions that would help me write this card's " +
			"rationale, or decide it does not deserve one.",
	}
	if strings.TrimSpace(focus) != "" {
		// The user's own steer, quoted rather than interpolated into the
		// instruction, so it reads as something they said rather than
		// something the tool decided.
		lines = append(lines, "",
			"What I am stuck on, in my words: "+strings.TrimSpace(focus))
	}
	return strings.Join(lines, "\n"), nil
}

// statusOnly adapts a bare status string to what Resolve wants. The deck's own
// status is what `DefaultFor` reads -- a theoretical deck opens wider than a
// built one -- and by this point the brief already holds it.
type statusOnly struct{ status string }

func (s statusOnly) DeckStatus() string { return s.status }
