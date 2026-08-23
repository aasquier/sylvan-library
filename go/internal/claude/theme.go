package claude

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand/v2"
	"regexp"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/tarot"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The theme interview: it asks about you, not about Magic (`claude/theme.py`,
// ADR 20).
//
// Two modes and one feature. `theme-conversation` talks -- prose questions, no
// pool, no deck -- and `theme-proposal` fires once at the end with a schema,
// the pool and a web search. Both cross here; their definitions crossed
// earlier, as data, with the other five.
//
// **Neither can see a deck**, and the absence is structural rather than
// promised: no deck source is passed, ever, and `boundary_test.go` fails the
// commit that reaches for one. The four instruments that hold ADR 20 all
// cross with it, and none of them is the system prompt:
//
//   - `Ground` intersects a claimed preference with the text of the person
//     supposedly holding it, and only **user** turns count -- quoting the
//     interviewer's own question back would let the mode ground a slot on a
//     preference it suggested.
//   - `Carry` is the readiness floor, and it may not go backwards. Short, shy
//     answers made the count go 0, 1, 0, 1, 0 before this existed, because
//     the report was built from this turn's reading alone.
//   - `MayPropose` counts in Go. There is no `ready` field in the schema for
//     a model to set.
//   - Every commander is resolved through the pool, and one whose identity is
//     not *exactly* the combination's is dropped and counted.
//
// The one thing here that is a fact about Go rather than about the design:
// `openingAngle` is `random.choice` over seven openings, and CPython's global
// `random` is seeded from the OS. Nothing reproducible rides on it, so this
// does not go through `pyrand` -- but it is a package var, because a corpus
// that could not hold it still would be a corpus of one opening question.

// SlotKinds is what the conversation is trying to learn, and deliberately not
// a list of things about Magic. `anchor` is the strongest single signal for
// picking a commander and the one a genuine newcomer will not have, which is
// why the floor is three rather than four.
var SlotKinds = []string{"taste", "temperament", "posture", "anchor"}

// SlotQuestions is what each kind is, as the closing instruction names it.
var SlotQuestions = map[string]string{
	"taste":       "A film, book, artwork, period or band they love that is not Magic.",
	"temperament": "How they are -- their sign, planner or improviser, what they do when a plan falls apart.",
	"posture":     "How they behave at game night: going for the throat, quietly building, or making deals.",
	"anchor":      "A Magic card, character or deck they already love. Optional -- a newcomer will not have one, and that is fine.",
}

const (
	// Floor is how many grounded slots before a proposal is available.
	// Three, so that having no favourite card does not lock a beginner out.
	Floor = 3
	// MaxExchanges is the conversation's ceiling. A multi-turn mode has no
	// natural stopping point; after this many the interview either proposes
	// from what it has or ends honestly without one.
	MaxExchanges = 10
	// MaxTurnChars: a single answer longer than this is not an answer, it is
	// a paste. The transcript is client-held and resent, so its size is the
	// client's to inflate and the bill is not.
	MaxTurnChars = 2000
	// MaxFactChars: a fun fact longer than this was not a fun fact. Same
	// argument, one field over.
	MaxFactChars = 600
	// TarotSource is how a reader cites `tarotlore`. Lowercase, because
	// KeepFact folds case before matching and the id is folded with it.
	TarotSource = "tarot:"
	// MinQuoteChars: below this a "quote" is a coincidence rather than
	// evidence. Three keeps real short answers -- "cat", "red", "80s".
	MinQuoteChars = 3
)

// ThemeAskNever and ThemeProposeNever are the promises each half's payload
// carries about itself.
const (
	ThemeAskNever     = "These are questions about you. What you build is your call."
	ThemeProposeNever = "The reading is Claude's interpretation of what you said, not a " +
		"finding. The cards and their colours are the pool's."
)

// ErrTranscriptRejected is a conversation this mode will not run: nothing is
// wrong with the pool, the key or the model, the request is malformed, and it
// is a 422.
type ErrTranscriptRejected struct{ Msg string }

func (e *ErrTranscriptRejected) Error() string { return e.Msg }

func rejectTranscript(format string, args ...any) error {
	return &ErrTranscriptRejected{Msg: fmt.Sprintf(format, args...)}
}

// ErrNotReady is a proposal asked for before three slots survived grounding.
// Its own error because its status is its own: 409, not 422 -- nothing is
// malformed and nothing failed, there simply is not enough yet.
type ErrNotReady struct{ Msg string }

func (e *ErrNotReady) Error() string { return e.Msg }

// ------------------------------------------------------------ reading the answer

// controlChars is anything in the C0 range, plus DEL. Not a security measure
// -- a defence against a real thing that happened.
var controlChars = regexp.MustCompile("[\x00-\x1f\x7f]")

// themeLog is where `prose` reports a repair. Quiet by construction otherwise.
var themeLog = slog.Default().With("logger", "mtglab.claude.theme")

// Prose is model-written text, fit to put in front of somebody: control
// characters out, whitespace collapsed.
//
// It exists because of one observed answer -- the model wrote a question
// containing `\f` where it meant "a fight", `json.loads` faithfully decoded
// that as a form feed, and the sentence rendered with a word's first letter
// eaten and no error anywhere.
//
// **It counts what it removed, and that is the point of the log line.** A
// turn was once reported as having lost its dashes, and the report could not
// be acted on because this function repairs the text and the evidence in one
// pass. `controlChars` cannot match a dash, so that report is still
// unexplained, and the only way to tell "the escape bug again, wearing
// different clothes" from "the model wrote it that way" is to know whether
// anything was substituted at all.
func Prose(text any) string {
	raw := pyStrOr(text)
	removed := 0
	cleaned := controlChars.ReplaceAllStringFunc(raw, func(string) string {
		removed++
		return " "
	})
	if removed > 0 {
		seen := map[string]bool{}
		for _, r := range raw {
			if r <= 0x1f || r == 0x7f {
				seen[fmt.Sprintf("U+%04X", r)] = true
			}
		}
		names := make([]string, 0, len(seen))
		for name := range seen {
			names = append(names, name)
		}
		sort.Strings(names)
		themeLog.Warn("prose() removed control characters from model text",
			"removed", removed, "codepoints", strings.Join(names, ", "))
	}
	return pySplitJoin(cleaned)
}

// normalise is lowercased and whitespace-collapsed, punctuation left alone.
//
// Deliberately not a fuzzy match: normalising away punctuation would let
// "cats" match "cats?" but would also start matching things that are not
// quotes, and the value of this check is that it is hard to pass by accident.
//
// `pyCasefold` and not `strings.ToLower` -- see `pycasefold.go`. This is the
// one call in the package where the two can disagree about strings a person
// actually typed.
func normalise(text string) string { return pyCasefold(pySplitJoin(text)) }

// TranscriptTurn is one client-held turn: plain text with a role, and nothing
// structural.
type TranscriptTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// Slot is one thing believed about the person, and the quote that earns it.
type Slot struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Quote string `json:"quote"`
}

// quoteSeparator joins the user's turns with a rune no keyboard produces, so
// a "quote" cannot span two turns and pick up words the user never put next
// to each other.
const quoteSeparator = " ␟ "

// Ground keeps the slots whose quote is really in the user's own turns, and
// counts what it dropped.
//
// The whole readiness argument in one substring test. A model that has decided
// who somebody is will happily report that back as a preference they
// expressed, and this is the check that turns "I think you want blue" into a
// dropped row rather than a fact about the user.
//
// Crude on purpose, the same bluntness `OnlyQuestions` has: a model that
// quotes one stray real word can carry an invented reading past it. It catches
// wholesale invention, which is the failure that matters, and the dropped
// count is surfaced so a prompt that has started making things up reads as a
// number rather than as helpfulness.
func Ground(slots []any, transcript []TranscriptTurn) ([]Slot, int) {
	said := make([]string, 0, len(transcript))
	for _, t := range transcript {
		if t.Role == "user" {
			said = append(said, normalise(t.Text))
		}
	}
	haystack := strings.Join(said, quoteSeparator)

	kept := map[string]Slot{}
	dropped := 0
	for _, raw := range slots {
		item, ok := raw.(map[string]any)
		if !ok {
			dropped++
			continue
		}
		kind := pyStrip(pyStrOr(item["kind"]))
		quote := Prose(item["quote"])
		value := Prose(item["value"])
		needle := normalise(quote)
		if !slotKind(kind) || value == "" ||
			PyLen(needle) < MinQuoteChars || !strings.Contains(haystack, needle) {
			dropped++
			continue
		}
		// Last reading of a kind wins: the schema asks for the whole set each
		// turn, so a later turn refining "likes cats" into "likes big cats,
		// specifically" should replace it rather than sit beside it.
		kept[kind] = Slot{Kind: kind, Value: value, Quote: quote}
	}
	return inSlotOrder(kept), dropped
}

func slotKind(kind string) bool {
	for _, k := range SlotKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// inSlotOrder is `[kept[k] for k in SLOT_KINDS if k in kept]`: the canonical
// order, never the order the model happened to send.
func inSlotOrder(kept map[string]Slot) []Slot {
	out := []Slot{}
	for _, k := range SlotKinds {
		if slot, ok := kept[k]; ok {
			out = append(out, slot)
		}
	}
	return out
}

// Carry is what is known after a turn: what was known before, updated by this
// one.
//
// `closingFor` ends every mid-conversation instruction with "re-state every
// slot you are confident of, including ones from earlier turns", and for a
// long time that sentence was the only thing holding the readiness count up.
// It is a rule enforced by nothing, and it drifts: driven with the short, shy
// answers a first-timer actually gives, the count went 0, 1, 0, 1, 0 -- a
// model that mentioned `taste` on turn four and reached for `temperament` on
// turn five silently *un-knew* the first one.
//
// Downstream that is not a cosmetic wobble. `MayPropose` counts distinct
// kinds, so a count that can fall is a floor that can be walked away from,
// and the person answering sees a reading that never becomes ready no matter
// how much they say. That is commandment 2's failure exactly: the newcomer
// concludes they are answering wrong.
//
// Both halves have been through `Ground` against the same transcript, so
// nothing gets carried that the person did not say: this widens what is
// remembered, never what counts as evidence.
func Carry(previous, fresh []Slot) []Slot {
	kept := map[string]Slot{}
	for _, s := range previous {
		kept[s.Kind] = s
	}
	for _, s := range fresh {
		kept[s.Kind] = s
	}
	return inSlotOrder(kept)
}

// MayPropose is whether there is enough to propose from. Counted, never
// declared: there is no field anywhere in the conversation schema a model
// could set to change this answer.
func MayPropose(grounded []Slot) bool {
	kinds := map[string]bool{}
	for _, s := range grounded {
		kinds[s.Kind] = true
	}
	return len(kinds) >= Floor
}

// stopWords carry no meaning for the repeat check. Small on purpose: the
// check is about content words, and a longer list starts making decisions
// about what counts as content.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "and": true, "or": true,
	"in": true, "to": true, "is": true, "are": true, "was": true, "were": true,
	"it": true, "its": true, "that": true, "this": true, "for": true,
	"on": true, "as": true, "with": true, "by": true, "at": true, "from": true,
	"be": true, "has": true, "have": true, "had": true, "not": true,
	"but": true, "they": true, "their": true, "there": true,
}

// repeatOverlap is how much of the shorter fact's content vocabulary must
// appear in a fact already given before the new one counts as a repeat.
const repeatOverlap = 0.7

var wordRun = regexp.MustCompile(`[a-z0-9']+`)

func contentWords(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRun.FindAllString(normalise(text), -1) {
		if !stopWords[w] {
			out[w] = true
		}
	}
	return out
}

// Repeats is whether a fact is one already given, verbatim or lightly
// reworded.
//
// The backstop behind the prompt's "never give the same fact twice", which was
// enforced by nothing at all until 2026-08-16: facts ride in the report rather
// than the transcript, so the model literally could not see what it had
// already said. Crude the way `Ground` is crude, and for the same reason.
func Repeats(text string, told []string) bool {
	needle := normalise(text)
	if needle == "" {
		return false
	}
	words := contentWords(text)
	for _, old := range told {
		if needle == normalise(old) {
			return true
		}
		if len(words) == 0 {
			continue
		}
		oldWords := contentWords(old)
		small, large := words, oldWords
		if len(oldWords) < len(words) {
			small, large = oldWords, words
		}
		if len(small) == 0 {
			continue
		}
		shared := 0
		for w := range small {
			if large[w] {
				shared++
			}
		}
		// Both are counts, so this division is exact in either language and
		// the `>=` scan cannot land differently by an ulp.
		if float64(shared)/float64(len(small)) >= repeatOverlap {
			return true
		}
	}
	return false
}

// CheckTold validates the client-held list of facts already shown.
//
// The same door `CheckTranscript` is, one field over. Plain strings only -- a
// fact object has nowhere to ride -- and capped in both directions, because
// the list is resent every turn and the bill is not the client's.
//
// Tampering buys nothing, which is worth stating: a client that trims the list
// gets a fact it has already seen, and one that pads it loses facts it has
// not. Both are somebody lying to themselves about their own evening.
func CheckTold(raw any) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, rejectTranscript("facts must be a list of strings")
	}
	out := []string{}
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, rejectTranscript("fact %d is not a string", i)
		}
		text = pyStrip(text)
		if text == "" {
			continue
		}
		if PyLen(text) > MaxFactChars {
			return nil, rejectTranscript(
				"fact %d is %d characters; the cap is %d", i, PyLen(text), MaxFactChars)
		}
		out = append(out, text)
	}
	if len(out) > MaxExchanges {
		return nil, rejectTranscript(
			"%d facts is more than a %d-exchange conversation can have produced",
			len(out), MaxExchanges)
	}
	return out, nil
}

// Fact is a fun fact that came from somewhere, as the payload carries it.
type Fact struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	URL    string `json:"url"`
}

// KeepFact is a fun fact, if it came from somewhere. Nil otherwise.
//
// Two legitimate origins, checked differently. `taxonomy` means the colour
// reference data in the system prompt -- checked-in, human-written, carrying
// `verified_by`, and so trusted at the file level rather than the sentence
// level. Anything else must be a URL the search actually returned, checked the
// way `KeepSources` checks a citation, because a response schema suppresses
// the API's own citations and a URL in the payload is otherwise a string the
// model typed.
//
// A `tarot:` id is stricter than either: **the corpus's words, not the
// model's.** The id was the whole ask; `text` came back only because the
// schema requires it, and a fun fact paraphrased at a fortune-teller's table
// is the one thing at that table that would be a lie.
func KeepFact(raw any, searched []Page) *Fact {
	item, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	text := Prose(item["text"])
	source := Prose(item["source"])
	if text == "" || source == "" {
		return nil
	}
	folded := pyCasefold(source)
	if strings.HasPrefix(folded, TarotSource) {
		entry := reference.TarotFactByID(source[len(TarotSource):])
		if entry == nil {
			return nil
		}
		return &Fact{Text: entry.Text, Source: entry.Source, URL: ""}
	}
	if folded == "taxonomy" {
		return &Fact{Text: text, Source: "taxonomy", URL: ""}
	}
	for _, page := range searched {
		if CanonicalURL(page.URL) == CanonicalURL(source) {
			title := page.Title
			if title == "" {
				title = page.URL
			}
			return &Fact{Text: text, Source: title, URL: page.URL}
		}
	}
	return nil
}

// ------------------------------------------------------------- the transcript

// CheckTranscript validates a client-held transcript, and refuses anything
// else.
//
// ADR 20 keeps conversation state on the client, which means the wire format
// is a thing a client composes -- so this is the door. **It is not Anthropic's
// message format and must never become it.** An endpoint that accepted
// `messages` blocks would be a free proxy for somebody else's spend, which on
// a hosted instance is the entire game.
//
// Tampering with the *content* is allowed and buys nothing: every card in the
// proposal is re-resolved against the pool, every source is checked against
// the search, and readiness is recomputed here from the text rather than
// carried in it.
func CheckTranscript(raw any) ([]TranscriptTurn, error) {
	if raw == nil {
		return []TranscriptTurn{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, rejectTranscript("transcript must be a list of turns")
	}
	if len(items) > MaxExchanges*2+1 {
		return nil, rejectTranscript(
			"this conversation is over after %d exchanges. Propose from what "+
				"it has, or start again.", MaxExchanges)
	}
	out := []TranscriptTurn{}
	for i, raw := range items {
		turn, ok := raw.(map[string]any)
		if !ok {
			return nil, rejectTranscript("turn %d is not an object", i)
		}
		role := pyStrip(pyStrOr(turn["role"]))
		text := pyStrip(pyStrOr(turn["text"]))
		if role != "user" && role != "assistant" {
			return nil, rejectTranscript(
				"turn %d has role %s; only 'user' and 'assistant' cross this "+
					"boundary", i, wire.PyRepr(role))
		}
		if text == "" {
			return nil, rejectTranscript("turn %d is empty", i)
		}
		if PyLen(text) > MaxTurnChars {
			return nil, rejectTranscript("turn %d is %d characters; the cap is %d",
				i, PyLen(text), MaxTurnChars)
		}
		// Alternation is deliberately **not** required: the Messages API
		// accepts consecutive same-role turns and combines them. Requiring it
		// turned a recoverable hiccup into a wedged conversation -- when a turn
		// comes back without a usable question there is no assistant turn to
		// record, so the next answer is legitimately a second user turn in a
		// row, and refusing it means the only way out is starting over.
		out = append(out, TranscriptTurn{Role: role, Text: text})
	}
	// The **interviewer** opens, so a transcript starts with an assistant turn
	// rather than a user one. That is backwards from every other conversation
	// shape in this codebase and it is the point of the feature: somebody who
	// does not know what to build cannot be expected to speak first.
	if len(out) > 0 && out[0].Role != "assistant" {
		return nil, rejectTranscript(
			"a transcript starts with the interviewer's own opening question")
	}
	return out, nil
}

// themeFrame is the app's own opening turn, and it exists for a wire-level
// reason rather than a conversational one: the Messages API requires
// `messages[0]` to be a user turn, and this interview's first *real* turn is
// the interviewer's question. Without a frame the whole transcript is off by
// one role. Byte-stable, so it costs nothing across turns or conversations.
const themeFrame = "Somebody has opened the deckbuilder and does not know " +
	"what to build. Interview them."

// frameFor is the opening frame, the spread when there is one, and what is
// known.
//
// **Here rather than in the system prompt, and that is a caching decision.** A
// mode's instructions are byte-stable so `Converse` can cache them; a spread is
// different every reading, so putting it there would make every first turn a
// cache miss for the whole block.
//
// `told == nil` means **no corpus at all**, and that is why it is not simply an
// empty slice. Only the turn can tell a fact; the proposal's schema has no
// `fact` field, so offering it a hundred entries would be seven kilobytes of
// prompt with nowhere to go. An empty non-nil slice still means "offer
// everything, nothing told yet", which is a real first turn.
func frameFor(reading *tarot.Reading, told []string) string {
	if reading == nil {
		return themeFrame
	}
	frame := fmt.Sprintf("%s\n\nYou have already dealt three cards, face up, "+
		"in this order. These are the cards on the table and there are no "+
		"others:\n%s", themeFrame, reading.Describe())
	if told == nil {
		return frame
	}
	keys := make([]string, 0, len(reading.Cards))
	for _, drawn := range reading.Cards {
		keys = append(keys, drawn.Card.Key)
	}
	return frame + reference.TarotOffer(keys, told)
}

// themeMessages is the transcript as a request: a frame, the conversation, and
// the ask.
//
// The system block carries the instructions and the taxonomy and is
// byte-stable per mode, so `Converse` already caches it. That is enough for a
// single-shot mode and not enough here: a conversation's history is the part
// that grows, and it grows by appending, which is the ideal shape for a second
// breakpoint. Without one, every turn re-reads the whole conversation at full
// price. `Converse`'s own moving marker only ever lands on a tool-result block
// it created, so this one survives it.
func themeMessages(transcript []TranscriptTurn, closing, frame string) []anthropic.MessageParam {
	out := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(frame)),
	}
	for _, t := range transcript {
		block := anthropic.NewTextBlock(t.Text)
		if t.Role == "assistant" {
			out = append(out, anthropic.NewAssistantMessage(block))
			continue
		}
		out = append(out, anthropic.NewUserMessage(block))
	}

	marker := anthropic.NewTextBlock(closing)
	marker.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()

	// The closing instruction rides with the last user turn rather than as a
	// turn of its own, so the transcript the model sees is the conversation and
	// nothing else. The breakpoint goes after it: everything up to here is
	// settled, and the next turn appends.
	last := &out[len(out)-1]
	if last.Role == anthropic.MessageParamRoleUser {
		blocks := append([]anthropic.ContentBlockParamUnion{}, last.Content...)
		last.Content = append(blocks, marker)
		return out
	}
	// The conversation ends on the interviewer's own question, which only
	// happens if a client asks for another turn without answering. Append the
	// instruction as its own turn rather than editing theirs.
	return append(out, anthropic.NewUserMessage(marker))
}

// themeMode is one half's mode, wearing a persona's voice.
//
// Built per request where Python builds `CONVERSATION_MODES` once at import,
// and the difference is nothing: Python's comment there warns that building
// one per request would "silently turn every conversation into a cache miss",
// which is a claim about the *bytes* rather than about object identity. The
// API caches on the prompt it is sent, and concatenating the same two strings
// gives the same prompt every time. `GetMode` returns the registry's Mode by
// value, so the copy's name and instructions are the only things that move.
func themeMode(base string, who Persona) (Mode, error) {
	mode, err := GetMode(base)
	if err != nil {
		return Mode{}, err
	}
	// Appended, never substituted: the instructions carry the rules that make
	// this feature work, and a persona is not allowed a say in them.
	mode.Name, mode.Instructions = WithVoice(mode.Name, mode.Instructions, who)
	return mode, nil
}

// readingFor is the spread this conversation is being read from, or nil.
//
// Re-dealt from the seed on every turn rather than stored. The deal is
// deterministic, so the client carrying one integer is enough to make the
// cards the same cards for the whole reading -- the same trick the transcript
// uses, and the reason this mode still needs no table.
//
// **No error, where Python's `_reading_for` raises one.** There the seed is
// still text and `int(seed)` is the thing that can fail; here `seedFor` has
// already read it at check time, which is where a malformed one has to be
// refused anyway -- a job in state `error` four minutes later is not a 422.
// By the time this runs the seed is a number and `tarot.Deal` takes any.
func readingFor(who Persona, seed *big.Int) *tarot.Reading {
	if !who.Deals || seed == nil {
		return nil
	}
	reading := tarot.Deal(seed)
	return &reading
}

// ThemeStanceFor is the stance that applies when there is no deck to derive
// one from.
//
// Every other mode reads `status: built | theoretical` off the deck it is
// about (ADR 15). This one runs *before a deck exists*, and resolving with no
// deck is `off` -- correct as a default for "I have no idea what this is
// about", wrong here, where the answer is knowable: a deck that has not been
// built yet is as theoretical as a deck can get.
//
// Still clamped to the deployment ceiling, so `off` remains reachable and an
// operator who has turned this off has turned it off.
//
// **Exported because the dial has to be able to ask.** `/api/claude` renders
// the stance readout beside the create flow, and with no deck to name it would
// otherwise resolve to `off` while this was about to run the conversation at
// `second-opinion`.
func ThemeStanceFor(requested any, limit *Stance) (Stance, error) {
	if requested == nil {
		ceil := Ceiling()
		if limit != nil {
			ceil = *limit
		}
		return Clamp(SecondOpinion, ceil), nil
	}
	return Resolve(requested, nil, limit)
}

// ----------------------------------------------------------------- the modes

// Usage and StanceReadout are shared with the other modes; a theme report's
// envelope is the same five keys before the per-half fields begin.

// themeEnvelope is `_report`'s fixed head: one response shape for every
// outcome, including not asking at all.
//
// `answered_by` is ADR 14's third boundary as a field. It matters more here
// than anywhere else in the package: this is the first surface whose output is
// *meant* to be enjoyed, and a reader enjoying it should still never be unsure
// which system produced a sentence.
type themeEnvelope struct {
	AnsweredBy string        `json:"answered_by"`
	Mode       string        `json:"mode"`
	Model      string        `json:"model"`
	Stance     StanceReadout `json:"stance"`
	Usage      Usage         `json:"usage"`
}

func envelopeFor(turn *Turn, mode string, effective Stance) themeEnvelope {
	out := themeEnvelope{AnsweredBy: "claude", Mode: mode, Stance: Describe(effective)}
	if turn != nil {
		out.Model = turn.Model
		out.Usage = Usage{InputTokens: turn.InputTokens,
			OutputTokens: turn.OutputTokens, CacheReadTokens: turn.CacheReadTokens}
	}
	return out
}

// AskReport is one conversation turn's answer, in Python's key order.
//
// **`Fact` is a pointer and `Slots` is never nil**, because both distinctions
// are on the wire: Python writes `"fact": null` for a turn with none, and
// `"slots": []` for a reading with nothing in it. A nil slice marshals as
// `null`, which is a different payload and a client that renders it as one.
type AskReport struct {
	themeEnvelope
	Persona      string `json:"persona"`
	Asked        bool   `json:"asked"`
	Question     string `json:"question"`
	Fact         *Fact  `json:"fact"`
	FactsDropped int    `json:"facts_dropped"`
	Slots        []Slot `json:"slots"`
	SlotsDropped int    `json:"slots_dropped"`
	Grounded     int    `json:"grounded"`
	Floor        int    `json:"floor"`
	MayPropose   bool   `json:"may_propose"`
	Exchanges    int    `json:"exchanges"`
	MaxExchanges int    `json:"max_exchanges"`
	Reason       string `json:"reason"`
}

// AskAnswered is the same turn when a call was actually made and read. It
// carries one key more, and the extra key is the whole reason there are two
// types: Python passes `never` only on the path that reached the model, so a
// single struct would put it on the wire in exactly the cases Python leaves
// it off. The same shape `argue`'s four-versus-five keys already has.
type AskAnswered struct {
	AskReport
	Never string `json:"never"`
}

// ProposalReport is a proposal that did not complete, in Python's key order.
type ProposalReport struct {
	themeEnvelope
	Persona      string        `json:"persona"`
	Asked        bool          `json:"asked"`
	Combinations []Combination `json:"combinations"`
	Sources      []Source      `json:"sources"`
	Slots        []Slot        `json:"slots"`
	SlotsDropped int           `json:"slots_dropped"`
	Reason       string        `json:"reason"`
}

// ProposalAnswered is a proposal that came back with something to propose.
// Five keys more, and they sit *between* `sources` and `slots` rather than
// after -- which is why this is not the embedding trick `AskAnswered` uses
// but a type of its own.
type ProposalAnswered struct {
	themeEnvelope
	Persona      string        `json:"persona"`
	Asked        bool          `json:"asked"`
	Combinations []Combination `json:"combinations"`
	Sources      []Source      `json:"sources"`
	// ADR 20 diverges from ADR 19 here and it is deliberate: an unsourced
	// dossier is refused, an unsourced proposal is not. A dossier is entirely
	// web-sourced claims, so stripping them leaves voice; a proposal's
	// load-bearing content is a colour combination and six real commanders,
	// and both are pool facts that survive a failed search. The dropped
	// counts are surfaced instead.
	SourcesDropped      int    `json:"sources_dropped"`
	CommandersDropped   int    `json:"commanders_dropped"`
	CombinationsDropped int    `json:"combinations_dropped"`
	Searched            int    `json:"searched"`
	Slots               []Slot `json:"slots"`
	SlotsDropped        int    `json:"slots_dropped"`
	Reason              string `json:"reason"`
	Never               string `json:"never"`
}

// Commander is one legend the pool confirmed can lead a combination.
type Commander struct {
	Name      string   `json:"name"`
	Prose     string   `json:"prose"`
	SourceIDs []string `json:"source_ids"`
	// The pool's own fields, carried through under Python's spellings. All
	// five are nullable in the pool and so are pointers here: a card with no
	// oracle text writes `null`, never `""`.
	ManaCost      *string  `json:"mana_cost"`
	TypeLine      *string  `json:"type_line"`
	OracleText    *string  `json:"oracle_text"`
	ColorIdentity []string `json:"color_identity"`
	Image         *string  `json:"image"`
	ArtCrop       *string  `json:"art_crop"`
}

// Combination is one of the 32, with the reading that reached it.
type Combination struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Colors  []string `json:"colors"`
	Tier    string   `json:"tier"`
	Tagline string   `json:"tagline"`
	// Kept apart all the way to the page, which is the acceptance criterion
	// rather than a nicety: one of these can be wrong and the other cannot.
	Reading    string      `json:"reading"`
	Grounding  string      `json:"grounding"`
	SourceIDs  []string    `json:"source_ids"`
	Commanders []Commander `json:"commanders"`
}

// ------------------------------------------------------------ the ask, planned

// AskPlan is a conversation turn that has passed every check not needing the
// network -- `theme.AskRequest`, plus the answer when there is nothing to ask.
//
// A turn was measured at 4.3-37.7 seconds with **one outlier at 133.8s**, and
// the transport ceiling it has to fit under is bounded above at 236s only
// because that is where the dossier broke. Nobody knows where it actually is.
// A turn that overruns it fails the way that one did -- no status code, no
// access-log line, and a finished answer thrown away -- so the call goes in a
// job and the refusals stay in the request.
type AskPlan struct {
	History []TranscriptTurn
	// Carried is the slots the client sent, re-grounded against the transcript
	// on the way in -- the client is not the authority on what its user said.
	Carried   []Slot
	Effective Stance
	Persona   string
	Seed      *big.Int
	// Told is the facts already shown, client-held and resent the way the
	// transcript is. Quoted back in the closing instruction so the model can
	// honour "never give the same fact twice", and checked by `Repeats`
	// because a rule enforced by nothing drifts.
	Told []string
	// Tier is the asking seat's model grant, captured at plan time for the
	// same reason a deck source is: a job outlives the request that knew who
	// was asking, and a worker with a way to re-derive it would be a worker
	// with a way to reach the database.
	Tier string
	// Answer is filled when running this reaches nobody -- stance `off`, or a
	// conversation past its ceiling. Neither is an error; both are answers,
	// and both are available immediately, so the caller hands back a job born
	// finished rather than making somebody poll for a sentence Go already had.
	Answer *AskReport
}

// NeedsCall reports whether anything still has to be asked of Anthropic.
func (p *AskPlan) NeedsCall() bool { return p.Answer == nil }

// Exchanges is how many turns the person has taken.
func (p *AskPlan) Exchanges() int { return countExchanges(p.History) }

func countExchanges(transcript []TranscriptTurn) int {
	n := 0
	for _, t := range transcript {
		if t.Role == "user" {
			n++
		}
	}
	return n
}

// CheckAsk is `theme.check_ask`: everything that can refuse a turn, done
// before anything is spent.
//
// Unlike `CheckProposal` there is no floor to fail: a conversation with
// nothing in it yet is exactly the case this mode exists for.
func CheckAsk(transcript, slots, requested, persona, seed, facts any,
	tier string, limit *Stance) (*AskPlan, error) {
	history, err := CheckTranscript(transcript)
	if err != nil {
		return nil, err
	}
	effective, err := ThemeStanceFor(requested, limit)
	if err != nil {
		return nil, err
	}
	// Resolved here rather than in the worker: an unknown persona is a
	// malformed request and belongs with the other 422s, not a moment later as
	// a job in state `error`.
	who, err := GetPersona(persona)
	if err != nil {
		return nil, err
	}
	dealt, err := seedFor(who, seed)
	if err != nil {
		return nil, err
	}
	told, err := CheckTold(facts)
	if err != nil {
		return nil, err
	}
	slotList, _ := slots.([]any)
	carried, _ := Ground(slotList, history)

	plan := &AskPlan{History: history, Carried: carried, Effective: effective,
		Persona: who.Key, Seed: dealt, Told: told, Tier: tier}
	if !effective.AllowsCalls() {
		answer := askRefusal(plan, who, "The stance is off, so no call was made.")
		plan.Answer = &answer
		return plan, nil
	}
	if plan.Exchanges() >= MaxExchanges {
		// The conversation ceiling, and it is not the tool loop's. Reported as
		// a finished conversation rather than an error: what it has is what it
		// has, and if that is three grounded slots there is still a proposal.
		answer := askRefusal(plan, who, fmt.Sprintf(
			"That is %d exchanges, which is as long as this conversation goes.",
			MaxExchanges))
		plan.Answer = &answer
		return plan, nil
	}
	return plan, nil
}

// seedFor resolves the reading seed at check time, so an unusable one is a
// 422 now rather than a job in state `error` later. It also raises for a
// persona that does not deal, exactly as `_reading_for` does -- which is to
// say it does not: a seed handed to a plain voice is dropped, not refused.
func seedFor(who Persona, seed any) (*big.Int, error) {
	if !who.Deals || seed == nil {
		return nil, nil
	}
	n, err := pyInt(seed)
	if err != nil {
		// `int(seed)` raises `TypeError` for a list and `ValueError` for a
		// string that is not a number; Python catches both and says the same
		// sentence, so this does too.
		return nil, rejectTranscript("not a usable reading seed: %s", pyReprAny(seed))
	}
	return n, nil
}

// askRefusal is the report for a turn that reached nobody. Its own helper
// because the two callers differ only in the sentence.
func askRefusal(plan *AskPlan, who Persona, reason string) AskReport {
	name, _ := WithVoice(ModeThemeConversation, "", who)
	return AskReport{
		themeEnvelope: envelopeFor(nil, name, plan.Effective),
		Persona:       who.Key,
		Asked:         false,
		Question:      "",
		Fact:          nil,
		FactsDropped:  0,
		Slots:         plan.Carried,
		SlotsDropped:  0,
		Grounded:      len(plan.Carried),
		Floor:         Floor,
		MayPropose:    MayPropose(plan.Carried),
		Exchanges:     plan.Exchanges(),
		MaxExchanges:  MaxExchanges,
		Reason:        reason,
	}
}

// ThemeRun is what the two halves need beyond their plan. There is no deck
// source in it, and both `Converse` calls leave the source nil -- ADR 20's
// first decision, visible in the type.
type ThemeRun struct {
	Ledger *ledger.Recorder
	OnTurn func(done, max int)
}

// RunAsk is `theme.run_ask`: ask the next question, hear the answer.
//
// Stateless. The transcript arrives from the client and leaves again; nothing
// is stored, which is why this needs no table, no expiry sweep and no ADR 5
// ownership rule -- and, as it turns out, is the reason the most personal
// thing this app handles never touches the server's disk.
func RunAsk(ctx context.Context, conn *pool.Conn, plan *AskPlan, run ThemeRun) (any, error) {
	if plan.Answer != nil {
		return *plan.Answer, nil
	}
	who, err := GetPersona(plan.Persona)
	if err != nil {
		return nil, err
	}
	mode, err := themeMode(ModeThemeConversation, who)
	if err != nil {
		return nil, err
	}
	turn, err := Converse(ctx, mode, Request{
		Messages: themeMessages(plan.History,
			closingFor(plan.Carried, plan.History, plan.Told),
			frameFor(readingFor(who, plan.Seed), plan.Told)),
		Stance: plan.Effective,
		// No deck source, and the nil is the point rather than a default
		// nobody filled in.
		Deps:   tools.Deps{Source: nil, Pool: conn},
		Tier:   plan.Tier,
		Ledger: run.Ledger,
		// No client tools at all, so the only reason to go round again is the
		// one search resuming after a pause_turn.
		MaxTurns: 4,
		OnTurn:   run.OnTurn,
	})
	if err != nil {
		return nil, err
	}
	return readAsk(plan, who, mode.Name, turn), nil
}

// readAsk is the half of `run_ask` after the call, split from it so the
// corpus can drive it with a Turn built by hand.
func readAsk(plan *AskPlan, who Persona, modeName string, turn Turn) any {
	base := AskReport{
		themeEnvelope: envelopeFor(&turn, modeName, plan.Effective),
		Persona:       who.Key,
		Asked:         true,
		Slots:         plan.Carried,
		Grounded:      len(plan.Carried),
		Floor:         Floor,
		MayPropose:    MayPropose(plan.Carried),
		Exchanges:     plan.Exchanges(),
		MaxExchanges:  MaxExchanges,
	}
	if turn.Refused {
		base.Reason = "The model declined to answer this one."
		return base
	}
	var payload map[string]any
	if err := turn.Parsed(&payload); err != nil {
		base.Reason = fmt.Sprintf("The answer did not parse (stop reason: %s).",
			turn.StopReason)
		return base
	}

	question := Prose(payload["question"])
	// The same predicate the rationale interview uses, and for a related
	// reason: a declarative sentence here is the mode telling somebody what
	// they think instead of asking.
	if !strings.HasSuffix(question, "?") {
		question = ""
	}
	slotList, _ := payload["slots"].([]any)
	fresh, dropped := Ground(slotList, plan.History)
	// What the turn heard, on top of what was already known -- never instead
	// of it. See `Carry`.
	grounded := Carry(plan.Carried, fresh)

	// The backstop behind "never give the same fact twice": a fact the person
	// has already been told is dropped and counted, the same fate an unsourced
	// one meets in `KeepFact`. Counted rather than swallowed, because a model
	// that keeps reaching for its favourite fact should read as a number
	// somewhere, not as an evening quietly getting duller.
	fact := KeepFact(payload["fact"], turn.Searched)
	factsDropped := 0
	if fact != nil && Repeats(fact.Text, plan.Told) {
		fact = nil
		factsDropped = 1
	}

	base.Question = question
	base.Fact = fact
	base.FactsDropped = factsDropped
	base.Slots = grounded
	base.SlotsDropped = dropped
	base.Grounded = len(grounded)
	base.MayPropose = MayPropose(grounded)
	if question == "" {
		base.Reason = "Nothing usable came back."
	}
	return AskAnswered{AskReport: base, Never: ThemeAskNever}
}

// OpeningAngles are ways into the opening question, one drawn per
// conversation. All of them are still about the person and none is about
// Magic (ADR 20); what varies is only where the first question lands, so the
// interview never opens on the same beat twice in a row.
var OpeningAngles = []string{
	"something they keep returning to -- a film, an album, a book, a game",
	"the last thing that completely absorbed them, whatever it was",
	"how they are when a plan falls apart in the middle of the evening",
	"the part they end up playing when their friends are all in one room",
	"a place, real or fictional, or a period of history that pulls at them",
	"what they would do with a free evening and nobody to answer to",
	"the kind of villain -- or hero -- they catch themselves rooting for",
}

// openingAngle draws one. A variable so a corpus can hold it still.
//
// **Not `pyrand`.** Python spells this `random.choice`, on the global
// `Random` seeded from the OS -- so there is no seed to reproduce and nothing
// downstream depends on which one comes out. What a corpus needs is only that
// it can pin one, which a package var gives it.
// conversation starts from, and nothing downstream depends on the draw.
//
//nolint:gosec // a die, not a secret: it varies which of seven openings a
var openingAngle = func() string { return OpeningAngles[rand.IntN(len(OpeningAngles))] }

// closingFor is what to ask the model for, given how far along the
// conversation is.
//
// Assembled here from the grounded slots rather than left to the model to work
// out, so "what is still missing" is the same answer the button is computed
// from. A mode that thought it had heard something the readiness check
// disagreed with would ask the wrong next question.
func closingFor(grounded []Slot, transcript []TranscriptTurn, told []string) string {
	if len(transcript) == 0 {
		// The angle is drawn here rather than left to the model. One fixed
		// opening instruction produced one recognisable opening question, and
		// somebody who started the interview twice read the second as a script
		// -- "seemingly hard-coded" was the exact report, about the one part of
		// this feature that never was. A model asked to vary itself converges
		// on its favourite opener; a die does not.
		return "Open the conversation. One warm, specific question about them " +
			"-- not about Magic, and not a list. Introduce yourself in a " +
			"sentence first so they know what this is. Tonight, open from " +
			"this angle rather than whatever you usually reach for: " +
			openingAngle() + "."
	}
	// The ground already covered, so the no-repeats rule is followable rather
	// than aspirational. After the conversation itself, before the ask.
	covered := ""
	if len(told) > 0 {
		listed := make([]string, 0, len(told))
		for _, t := range told {
			listed = append(listed, "- "+t)
		}
		covered = "\n\nFacts you have already told them, exactly as they " +
			"read them:\n" + strings.Join(listed, "\n") + "\nDo not repeat " +
			"any of these, in these words or in other words -- a fact " +
			"covering the same ground is omitted, never reworded."
	}
	have := map[string]bool{}
	for _, s := range grounded {
		have[s.Kind] = true
	}
	missing := []string{}
	for _, k := range SlotKinds {
		if !have[k] && k != "anchor" {
			missing = append(missing, k+": "+SlotQuestions[k])
		}
	}
	if len(missing) > 0 {
		return "Ask the next question. Still unknown -- " +
			strings.Join(missing, "; ") + " Follow what they are actually " +
			"interested in rather than working through that list in order, " +
			"and re-state every slot you are confident of, including ones " +
			"from earlier turns." + covered
	}
	// Ready is the short circuit, not a licence to keep dealing. An earlier
	// version of this said "keep going while they are enjoying it", and what
	// that produced -- most visibly at the fortune-teller's table, where three
	// answered cards *are* the three slots -- was an interview that ambled on
	// toward the ten-exchange ceiling as if the ceiling were the goal.
	return "You now know enough to read them, and they should hear that from " +
		"you: say plainly, in your own voice, that you have what you need " +
		"and they can ask for their colours whenever they like. You may " +
		"close with one light, clearly optional question -- something that " +
		"sharpens what you already know or might turn up an anchor, never " +
		"a new line of enquiry -- and it must still end in a question " +
		"mark. Re-state every slot you are confident of." + covered
}

// ------------------------------------------------------- the proposal, planned

// ProposalPlan is a proposal that has passed every check not needing the
// network -- `theme.ProposalRequest`, plus the answer when there is nothing to
// ask.
//
// The split exists because the expensive half runs in a background job and the
// refusals must not go with it. A malformed transcript, a floor not yet
// reached and an unparseable stance are all decidable here, and each has a
// status code the UI acts on -- 422, 409, 422. Carried into a worker they
// would arrive as a *job* in state `error`, which turns three distinct
// answers into one string somebody has to pattern-match.
type ProposalPlan struct {
	History   []TranscriptTurn
	Grounded  []Slot
	Dropped   int
	Effective Stance
	Budget    *float64
	Avoid     string
	// Persona is the voice that ran the conversation, carried so the payoff
	// arrives in the same one. A reading that turns into a committee memo at
	// the moment it matters has thrown away the only thing the persona was for.
	Persona string
	Seed    *big.Int
	Tier    string
	Answer  *ProposalReport
}

// NeedsCall reports whether anything still has to be asked of Anthropic.
func (p *ProposalPlan) NeedsCall() bool { return p.Answer == nil }

// CheckProposal is `theme.check_proposal`: everything that can refuse a
// proposal, done before anything is spent.
//
// Refuses below the floor rather than proposing anyway. The button is dark in
// the UI for the same reason, but a floor that only existed in the client
// would not be one.
func CheckProposal(transcript, slots, requested any, budget *float64, avoid string,
	persona, seed any, tier string, limit *Stance) (*ProposalPlan, error) {
	history, err := CheckTranscript(transcript)
	if err != nil {
		return nil, err
	}
	slotList, _ := slots.([]any)
	grounded, dropped := Ground(slotList, history)
	effective, err := ThemeStanceFor(requested, limit)
	if err != nil {
		return nil, err
	}
	if !MayPropose(grounded) {
		return nil, &ErrNotReady{Msg: fmt.Sprintf(
			"%d of %d things are known about this person, and every one has "+
				"to be something they actually said. Keep talking.",
			len(grounded), Floor)}
	}
	// Resolved here rather than in the worker: an unknown persona is a
	// malformed request and belongs with the other 422s, not four minutes
	// later as a job in state `error`.
	who, err := GetPersona(persona)
	if err != nil {
		return nil, err
	}
	dealt, err := seedFor(who, seed)
	if err != nil {
		return nil, err
	}

	plan := &ProposalPlan{History: history, Grounded: grounded, Dropped: dropped,
		Effective: effective, Budget: budget, Avoid: avoid, Persona: who.Key,
		Seed: dealt, Tier: tier}
	if !effective.AllowsCalls() {
		// Stance `off`: a real position, answered without spending anything,
		// and answered *now* rather than a second from now on a worker thread.
		name, _ := WithVoice(ModeThemeProposal, "", who)
		answer := ProposalReport{
			themeEnvelope: envelopeFor(nil, name, effective),
			Persona:       who.Key,
			Asked:         false,
			Combinations:  []Combination{},
			Sources:       []Source{},
			Slots:         grounded,
			SlotsDropped:  dropped,
			Reason:        "The stance is off, so no call was made.",
		}
		plan.Answer = &answer
	}
	return plan, nil
}

// RunProposal is `theme.run_proposal`: read around, name legends, check every
// one.
//
// **Measured at 226 seconds** with four searches, since it reads a dozen-odd
// pages and resolves every legend it names against the pool. That is why the
// caller may hand in an `OnTurn` -- run as a background job this is the only
// thing that says it is still moving.
func RunProposal(ctx context.Context, conn *pool.Conn, plan *ProposalPlan, run ThemeRun) (any, error) {
	if plan.Answer != nil {
		return *plan.Answer, nil
	}
	who, err := GetPersona(plan.Persona)
	if err != nil {
		return nil, err
	}
	mode, err := themeMode(ModeThemeProposal, who)
	if err != nil {
		return nil, err
	}
	turn, err := Converse(ctx, mode, Request{
		Messages: themeMessages(plan.History,
			proposalAsk(plan.Grounded, plan.Budget, plan.Avoid),
			// `told` is nil, and the nil is load-bearing: the proposal's
			// schema has no `fact` field, so offering it a hundred entries
			// would be seven kilobytes of prompt with nowhere to go.
			frameFor(readingFor(who, plan.Seed), nil)),
		Stance: plan.Effective,
		Deps:   tools.Deps{Source: nil, Pool: conn},
		Tier:   plan.Tier,
		Ledger: run.Ledger,
		// A search, a look at what came back, a commander search, a get_cards
		// to confirm, and the write-up -- with a paused turn able to spend one.
		MaxTurns: 8,
		OnTurn:   run.OnTurn,
	})
	if err != nil {
		return nil, err
	}
	return readProposal(ctx, conn, plan, who, mode.Name, turn)
}

// readProposal is the half of `run_proposal` after the call, split from it so
// the corpus can drive it with a Turn built by hand.
func readProposal(ctx context.Context, conn *pool.Conn, plan *ProposalPlan,
	who Persona, modeName string, turn Turn) (any, error) {
	base := ProposalReport{
		themeEnvelope: envelopeFor(&turn, modeName, plan.Effective),
		Persona:       who.Key,
		Asked:         true,
		Combinations:  []Combination{},
		Sources:       []Source{},
		Slots:         plan.Grounded,
		SlotsDropped:  plan.Dropped,
	}
	if turn.Refused {
		base.Reason = "The model declined to write this one."
		return base, nil
	}
	var payload map[string]any
	if err := turn.Parsed(&payload); err != nil {
		base.Reason = fmt.Sprintf("The answer did not parse (stop reason: %s).",
			turn.StopReason)
		//nolint:nilerr // an unreadable answer is a reported outcome, not a fault
		return base, nil
	}

	claimed, _ := payload["sources"].([]any)
	sources, sourcesDropped := KeepSources(claimed, turn.Searched)
	allowed := map[string]bool{}
	for _, s := range sources {
		allowed[s.ID] = true
	}
	combinations, commandersDropped, combinationsDropped, err :=
		resolveCombinations(ctx, conn, payload["combinations"], allowed)
	if err != nil {
		return nil, err
	}
	if len(combinations) == 0 {
		// The one refusal this mode does make, and it is not ADR 19's. A
		// dossier with no source is empty; a proposal with no *combination*
		// has nothing to propose, which is a different failure and the only
		// one worth refusing over.
		base.Sources = sources
		base.Reason = "Nothing came back that resolved against the pool, so " +
			"there is nothing to suggest."
		return base, nil
	}

	return ProposalAnswered{
		themeEnvelope:       base.themeEnvelope,
		Persona:             who.Key,
		Asked:               true,
		Combinations:        combinations,
		Sources:             sources,
		SourcesDropped:      sourcesDropped,
		CommandersDropped:   commandersDropped,
		CombinationsDropped: combinationsDropped,
		Searched:            len(turn.Searched),
		Slots:               plan.Grounded,
		SlotsDropped:        plan.Dropped,
		Reason:              "",
		Never:               ThemeProposeNever,
	}, nil
}

// proposalAsk is the closing instruction, carrying the readings and the
// filters.
//
// Constraints ride here rather than as slots, because they are filters on a
// query and not readings of a person (ADR 20). A beginner should not have to
// declare a budget before the tool will talk to them, and `price_max` is
// literally a `search_cards` argument.
func proposalAsk(grounded []Slot, budget *float64, avoid string) string {
	// `json.dumps(grounded, indent=2)`, and it goes to the model rather than
	// into a digest -- but the same renderer writes it, so the bytes agree
	// with Python's and the corpus can pin the whole instruction.
	rows := make([]any, 0, len(grounded))
	for _, s := range grounded {
		rows = append(rows, wire.OrderedMap{
			{Key: "kind", Value: s.Kind},
			{Key: "value", Value: s.Value},
			{Key: "quote", Value: s.Quote},
		})
	}
	lines := []string{
		"That is the conversation. Here is what was heard, and every one of " +
			"these is quoted from something they actually typed:",
		"",
		pyDumps(rows, pyDumpOptions{Indent: 2}),
		"",
	}
	// `if budget:` -- a zero budget is no budget, which is Python's truthiness
	// and not a nil check.
	if budget != nil && *budget != 0 {
		lines = append(lines, fmt.Sprintf(
			"They have about $%s for the whole deck, so pass price_max to "+
				"search_cards and prefer commanders that do not need an "+
				"expensive shell.", pyFormatG(*budget)))
	}
	if pyStrip(avoid) != "" {
		lines = append(lines, "Colours or things to steer away from, in their "+
			"words: "+pyStrip(avoid))
	}
	lines = append(lines,
		"Propose two colour combinations and three commanders for each. The "+
			"second combination should take a different reading of the same "+
			"person, so the contrast shows them a choice was being made.")
	return strings.Join(lines, "\n")
}

// resolveCombinations is `_combinations`: combinations with a real key, and
// commanders the pool confirms.
//
// Two checks in one pass, and both drop rather than repair. A `key` that is
// not one of the 32 is a combination that cannot be rendered or turned into a
// create-flow state, and a commander that does not resolve is a card the model
// invented -- the same instrument the dossier's competitors check points at
// the same failure.
func resolveCombinations(ctx context.Context, conn *pool.Conn, raw any,
	allowed map[string]bool) ([]Combination, int, int, error) {
	valid := map[string]reference.Combination{}
	for _, c := range reference.Colors().Combinations {
		valid[c.Key] = c
	}
	items, _ := raw.([]any)
	out := []Combination{}
	dropped, lost := 0, 0
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		combo, ok := valid[strings.ToUpper(pyStrip(pyStrOr(row["key"])))]
		if !ok {
			lost++
			continue
		}
		commanders, missed, err := resolveCommanders(ctx, conn, row["commanders"], combo, allowed)
		if err != nil {
			return nil, 0, 0, err
		}
		dropped += missed
		if len(commanders) == 0 {
			// A combination with no confirmed commander is a colour name and a
			// paragraph. The user cannot act on it, so it is not shown -- and
			// it is counted, because losing half a proposal silently is how a
			// measured run looks fine and a real one looks thin. Measured: a
			// combination goes when every legend named for it turned out to
			// have a *subset* identity, which is legal in those colours and
			// does not make a deck that fills that slot.
			lost++
			continue
		}
		out = append(out, Combination{
			Key:        combo.Key,
			Name:       combo.Name,
			Colors:     combo.Colors,
			Tier:       combo.Tier,
			Tagline:    combo.Tagline,
			Reading:    Prose(row["reading"]),
			Grounding:  Prose(row["grounding"]),
			SourceIDs:  allowedIDs(row["source_ids"], allowed),
			Commanders: commanders,
		})
	}
	if len(out) > 2 {
		out = out[:2]
	}
	return out, dropped, lost, nil
}

// allowedIDs is `[str(i) for i in (raw or []) if str(i) in allowed]`: a
// citation the sources check did not keep is not a citation.
func allowedIDs(raw any, allowed map[string]bool) []string {
	items, _ := raw.([]any)
	out := []string{}
	for _, item := range items {
		id := pyStr(item)
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out
}

// resolveCommanders is `_commanders`: named legends, resolved against the pool
// or dropped and counted.
//
// Two things are checked and the second is the one that matters. The name has
// to exist -- and the card has to actually be able to lead *this* combination,
// because a mono-white legend is legal in a Selesnya deck but a deck it leads
// is a mono-white deck and fills a different one of the 32. Rule 1 is not the
// only rule the pool is enforcing here.
func resolveCommanders(ctx context.Context, conn *pool.Conn, raw any,
	combo reference.Combination, allowed map[string]bool) ([]Commander, int, error) {
	rows := []map[string]any{}
	items, _ := raw.([]any)
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			rows = append(rows, row)
		}
	}
	names := make([]string, 0, len(rows))
	wanted := []string{}
	for _, row := range rows {
		name := pyStrip(pyStrOr(row["card"]))
		names = append(names, name)
		if name != "" {
			wanted = append(wanted, name)
		}
	}
	if len(wanted) == 0 {
		return []Commander{}, len(rows), nil
	}

	looked, err := deckread.CardsNamed(ctx, conn, wanted)
	if err != nil {
		return nil, 0, err
	}
	found := map[string]deckread.NamedCard{}
	for _, record := range looked.Cards {
		found[pyCasefold(record.Name)] = record
	}
	identity := map[string]bool{}
	for _, c := range combo.Colors {
		identity[c] = true
	}

	out := []Commander{}
	dropped := 0
	for i, row := range rows {
		record, ok := found[pyCasefold(names[i])]
		if !ok || !sameIdentity(record.ColorIdentity, identity) {
			dropped++
			continue
		}
		out = append(out, Commander{
			Name:          record.Name,
			Prose:         Prose(row["prose"]),
			SourceIDs:     allowedIDs(row["source_ids"], allowed),
			ManaCost:      record.ManaCost,
			TypeLine:      record.TypeLine,
			OracleText:    record.OracleText,
			ColorIdentity: record.ColorIdentity,
			Image:         record.Image,
			ArtCrop:       record.ArtCrop,
		})
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out, dropped, nil
}

// sameIdentity is `set(record["color_identity"] or []) != identity`, the other
// way round. A **set** comparison and not a length-and-contains one: the pool
// is the authority on both sides, but a record that repeated a colour would
// otherwise compare unequal to the combination it belongs to.
func sameIdentity(colors []string, identity map[string]bool) bool {
	seen := map[string]bool{}
	for _, c := range colors {
		if !identity[c] {
			return false
		}
		seen[c] = true
	}
	return len(seen) == len(identity)
}

// ------------------------------------------------------------- the job labels

// AskLabel and ProposalLabel are `themeruns.plan_ask`'s and
// `plan_proposal`'s. A job list is a list of one-liners, and these are what
// somebody reads to tell "asked me something" from "spent four minutes".
func AskLabel(plan *AskPlan) string {
	return fmt.Sprintf("theme: a question, from %d thing%s known",
		len(plan.Carried), plural(len(plan.Carried)))
}

// ProposalLabel is the expensive half's.
func ProposalLabel(plan *ProposalPlan) string {
	return fmt.Sprintf("theme: colours and commanders, from %d thing%s known",
		len(plan.Grounded), plural(len(plan.Grounded)))
}

// plural is `” if n == 1 else 's'`, which is **not** `n != 1` spelled the
// usual way round: Python's conditional puts the singular first, so zero
// things are "0 things" and one is "1 thing".
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
