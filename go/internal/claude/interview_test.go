package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

const gateFixtures = "../gate/testdata"

func fixtureDeck(t *testing.T, slug string) *deck.Deck {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(gateFixtures, slug+".yaml"))
	if err != nil {
		t.Fatalf("reading the %s fixture: %v", slug, err)
	}
	d, err := deck.FromText(string(text), slug)
	if err != nil {
		t.Fatalf("parsing %s: %v", slug, err)
	}
	return d
}

func withPool(t *testing.T, fn func(c *pool.Conn)) {
	t.Helper()
	p := pooltest.Open(t)
	if err := p.Use(context.Background(), func(c *pool.Conn) error {
		fn(c)
		return nil
	}); err != nil {
		t.Fatalf("leasing the pool: %v", err)
	}
}

// ------------------------------------------------------- the question guard

// The whole of rule 4's edge in one predicate.
//
// The failure this guards against is a declarative sentence appearing in the
// column beside an empty rationale box, which reads as a draft whatever the
// surrounding UI calls it. It is crude on purpose and will occasionally drop
// something serviceable; that is the right trade.
func TestOnlyQuestionsDropsAnythingThatIsNotOne(t *testing.T) {
	items := []any{
		map[string]any{"question": "What does this beat out at three mana?",
			"angle": "cost", "fact": "there are nine cards at three"},
		// The whole reason the predicate exists: a rationale, offered.
		map[string]any{"question": "Sol Ring is the best ramp spell ever printed.",
			"angle": "role", "fact": "it costs one"},
		// A softened one is still not a question.
		map[string]any{"question": "You might say it fixes and ramps at once.",
			"angle": "role", "fact": ""},
		map[string]any{"question": "   Which one is really doing the job?   ",
			"angle": "redundancy", "fact": "two claim it"},
		// A bare question mark is not a question either.
		map[string]any{"question": "?", "angle": "role", "fact": ""},
		map[string]any{"question": "", "angle": "role", "fact": ""},
		"not even an object",
		nil,
	}
	kept, dropped := OnlyQuestions(items)
	if len(kept) != 2 {
		t.Fatalf("kept %d questions, want 2: %+v", len(kept), kept)
	}
	if dropped != 6 {
		t.Errorf("dropped %d, want 6 -- the count is how a prompt that has "+
			"started editorialising becomes visible", dropped)
	}
	if kept[1].Question != "Which one is really doing the job?" {
		t.Errorf("a question was not trimmed: %q", kept[1].Question)
	}
	for _, q := range kept {
		if !strings.HasSuffix(q.Question, "?") {
			t.Errorf("a non-question survived: %q", q.Question)
		}
	}
}

// An item with no angle still has one, because the client groups by it.
func TestAQuestionWithNoAngleGetsTheDefault(t *testing.T) {
	kept, _ := OnlyQuestions([]any{
		map[string]any{"question": "Why this one?", "fact": "x"},
	})
	if len(kept) != 1 || kept[0].Angle != "role" {
		t.Errorf("kept %+v, want one question at angle role", kept)
	}
}

// ------------------------------------------------------------------ the brief

// The facts are assembled rather than asked for, so the mode cannot ask about
// a card it never read. This is what proves the brief actually carries them.
func TestTheBriefCarriesTheCardTheGateAndTheCategory(t *testing.T) {
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		facts, err := Brief(context.Background(), c, d, "Primeval Titan")
		if err != nil {
			t.Fatalf("brief: %v", err)
		}
		card, _ := kv(facts, "card").(wire.OrderedMap)
		if got := asString(kv(card, "name")); got != "Primeval Titan" {
			t.Errorf("card name is %q", got)
		}
		if inPool, _ := kv(card, "in_pool").(bool); !inPool {
			t.Error("the card was not resolved against the pool")
		}
		// The case this project is deliberately wrong about: a banned card
		// comes back with its real text, so the interview can ask about the
		// ban rather than discovering it cannot look the card up.
		gateFacts, _ := kv(facts, "gate").(wire.OrderedMap)
		about, _ := kv(gateFacts, "about_this_card").([]wire.OrderedMap)
		if len(about) == 0 {
			t.Error("the gate flagged Primeval Titan as banned and the brief " +
				"did not carry it -- that is the one question worth asking here")
		}
		for _, issue := range about {
			if asString(kv(issue, "severity")) == "" || asString(kv(issue, "code")) == "" {
				t.Errorf("a gate issue crossed without its severity or code: %+v", issue)
			}
		}
		// The category row, and the siblings that make redundancy askable.
		category, _ := kv(facts, "category").(wire.OrderedMap)
		if asString(kv(category, "name")) != "threat" {
			t.Errorf("category is %v", kv(category, "name"))
		}
		siblings, _ := kv(category, "other_cards_in_it").([]wire.OrderedMap)
		if len(siblings) == 0 {
			t.Error("no sibling rationales: the most useful thing in the brief")
		}
		for _, s := range siblings {
			if asString(kv(s, "name")) == "Primeval Titan" {
				t.Error("the card is listed as its own sibling")
			}
		}
	})
}

// A deck-level gate issue -- a wrong card count, a missing commander -- carries
// no card, and is not about this card. Reading it as one would have been a nil
// dereference at best and somebody else's error at worst.
func TestADeckLevelGateIssueIsNotAboutThisCard(t *testing.T) {
	d := fixtureDeck(t, "messy")
	withPool(t, func(c *pool.Conn) {
		facts, err := Brief(context.Background(), c, d, firstCardName(t, d))
		if err != nil {
			t.Fatalf("brief: %v", err)
		}
		gateFacts, _ := kv(facts, "gate").(wire.OrderedMap)
		if errs, _ := kv(gateFacts, "deck_errors").(int); errs == 0 {
			t.Skip("the messy fixture stopped failing the gate; nothing to prove here")
		}
		about, _ := kv(gateFacts, "about_this_card").([]wire.OrderedMap)
		for _, issue := range about {
			if asString(kv(issue, "code")) == "" {
				t.Errorf("a deck-level issue was attributed to a card: %+v", issue)
			}
		}
	})
}

// topLevelKeys reads an object's own keys, in order, ignoring nested ones.
//
// A plain `strings.Index` scan is not enough and the first version of this test
// used one: `category` appears both as a top-level block and as a field inside
// `card`, so the naive scan found the nested one and reported the brief as
// misordered when it was not.
func topLevelKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	var keys []string
	depth, inString, escaped, wantKey := 0, false, false, false
	start := 0
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case ch == '\\':
				escaped = true
			case ch == '"':
				inString = false
				if wantKey && depth == 1 {
					keys = append(keys, string(raw[start:i]))
					wantKey = false
				}
			}
			continue
		}
		switch ch {
		case '"':
			inString, start = true, i+1
			// A string opening at depth 1 right after `{` or `,` is a key.
			wantKey = depth == 1 && lastMeaningful(raw[:i]) != ':'
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return keys
}

func lastMeaningful(raw []byte) byte {
	for i := len(raw) - 1; i >= 0; i-- {
		switch raw[i] {
		case ' ', '\n', '\t', '\r':
			continue
		default:
			return raw[i]
		}
	}
	return 0
}

// The brief is serialised straight into the prompt, so its key order is part of
// the bytes the model reads and part of what the cache hashes. A map here would
// alphabetise it, and every nested block has to be ordered too -- a bare
// []wire.KV renders as an array of {"Key":..,"Value":..} structs, which is
// still valid JSON and still gets an answer, from a model handed nonsense.
func TestTheBriefKeepsPythonsKeyOrder(t *testing.T) {
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		facts, err := Brief(context.Background(), c, d, "Sol Ring")
		if err != nil {
			t.Fatalf("brief: %v", err)
		}
		raw, err := json.Marshal(facts)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"deck", "card", "gate", "category", "curve"}
		if got := topLevelKeys(t, raw); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("the brief's blocks are %v, want %v in Python's order", got, want)
		}
		// And the nested blocks are objects rather than struct arrays.
		if !strings.Contains(string(raw), `{"deck":{"slug":`) {
			t.Errorf("the deck block is not an ordered object: %.140s", raw)
		}
		if strings.Contains(string(raw), `"Key":`) {
			t.Error("a nested block rendered as raw {\"Key\":..,\"Value\":..} " +
				"structs -- valid JSON, and nonsense to the model")
		}
		card, _ := kv(facts, "card").(wire.OrderedMap)
		wantCard := []string{"name", "category", "quantity", "rationale_so_far",
			"in_pool", "pool"}
		cardRaw, err := json.Marshal(card)
		if err != nil {
			t.Fatal(err)
		}
		if got := topLevelKeys(t, cardRaw); fmt.Sprint(got) != fmt.Sprint(wantCard) {
			t.Errorf("the card block is %v, want %v", got, wantCard)
		}
	})
}

// The interview argues about a card already in a deck; adding one is a
// different operation, and the caller's response differs (422, not 404).
func TestACardTheDeckDoesNotRunIsItsOwnRefusal(t *testing.T) {
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		_, err := Brief(context.Background(), c, d, "Black Lotus")
		var missing *ErrCardNotInDeck
		if !errors.As(err, &missing) {
			t.Fatalf("want ErrCardNotInDeck, got %v", err)
		}
		if !strings.Contains(err.Error(), "'Black Lotus'") {
			t.Errorf("the refusal should quote the card the way Python does: %v", err)
		}
	})
}

// Typed the way a person types it.
func TestACardIsFoundHoweverItWasTyped(t *testing.T) {
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		for _, typed := range []string{"sol ring", "  SOL RING  ", "Sol Ring"} {
			facts, err := Brief(context.Background(), c, d, typed)
			if err != nil {
				t.Fatalf("%q: %v", typed, err)
			}
			card, _ := kv(facts, "card").(wire.OrderedMap)
			if got := asString(kv(card, "name")); got != "Sol Ring" {
				t.Errorf("%q resolved to %q, want the pool's spelling", typed, got)
			}
		}
	})
}

// The commander is a card the interview can be asked about too -- it holds a
// slot and carries a rationale like anything else.
func TestTheCommanderCanBeInterviewed(t *testing.T) {
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		facts, err := Brief(context.Background(), c, d, "Goreclaw, Terror of Qal Sisma")
		if err != nil {
			t.Fatalf("the commander could not be interviewed: %v", err)
		}
		card, _ := kv(facts, "card").(wire.OrderedMap)
		if asString(kv(card, "name")) != "Goreclaw, Terror of Qal Sisma" {
			t.Errorf("resolved to %v", kv(card, "name"))
		}
	})
}

// A card on the swap board holds no slot yet, and is exactly the card somebody
// is deciding about -- so it is interviewable too. Python looks in all three
// places and so does this.
func TestACardOnTheSwapBoardCanBeInterviewed(t *testing.T) {
	d := fixtureDeck(t, "rich")
	withPool(t, func(c *pool.Conn) {
		facts, err := Brief(context.Background(), c, d, "Sword of Feast and Famine")
		if err != nil {
			t.Fatalf("a swap-board card could not be interviewed: %v", err)
		}
		card, _ := kv(facts, "card").(wire.OrderedMap)
		if got := asString(kv(card, "name")); got != "Sword of Feast and Famine" {
			t.Errorf("resolved to %q", got)
		}
		if got := asString(kv(card, "rationale_so_far")); got == "" {
			t.Error("the swap-board card's existing rationale was dropped -- " +
				"with one, the job is to interrogate it")
		}
	})
}

// ------------------------------------------------------------- the interview

// At `initiative: off` no call is made and the payload says so. Not an empty
// list that looks like it had nothing to say.
func TestAtStanceOffNothingIsAskedAndItSaysSo(t *testing.T) {
	api := &scriptedAPI{replies: []string{}}
	api.start(t)
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		off, err := Preset("off")
		if err != nil {
			t.Fatal(err)
		}
		report, err := Interview(context.Background(), c, d, "Sol Ring",
			InterviewRequest{Requested: "off", Limit: &off})
		if err != nil {
			t.Fatalf("interview: %v", err)
		}
		if api.served != 0 {
			t.Errorf("%d API calls were made at stance off", api.served)
		}
		if report.Asked {
			t.Error("the report claims it asked")
		}
		if report.Reason == "" {
			t.Error("no reason given for not asking -- an empty question list " +
				"with no reason is indistinguishable from having nothing to say")
		}
		if report.Questions == nil {
			t.Error("questions is null rather than an empty list")
		}
		if report.Never != NeverSentence {
			t.Error("the payload dropped its own promise about what it is")
		}
	})
}

// The whole round trip, with the model's answer scripted: the questions come
// back, a declarative is dropped and counted, and the payload says who answered.
func TestAnInterviewReturnsQuestionsAndDropsWhatIsNotOne(t *testing.T) {
	answer := `{"questions":[` +
		`{"question":"What does this beat out at three mana?","angle":"cost","fact":"nine cards at three"},` +
		`{"question":"Sol Ring is simply the best ramp there is.","angle":"role","fact":"it costs one"},` +
		`{"question":"Which of your two threats is really the finisher?","angle":"redundancy","fact":"both claim it"}` +
		`]}`
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", in: 900, out: 120, cached: 2000,
			content: textBlock(answer)}.json(),
	}}
	api.start(t)
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		report, err := Interview(context.Background(), c, d, "Sol Ring",
			InterviewRequest{Requested: "second-opinion"})
		if err != nil {
			t.Fatalf("interview: %v", err)
		}
		if !report.Asked || len(report.Questions) != 2 {
			t.Fatalf("report is %+v", report)
		}
		if report.QuestionsDropped != 1 {
			t.Errorf("dropped %d, want 1 -- the declarative", report.QuestionsDropped)
		}
		if report.AnsweredBy != "claude" {
			t.Errorf("answered_by is %q -- ADR 14's third boundary is a field",
				report.AnsweredBy)
		}
		if report.Usage.CacheReadTokens != 2000 {
			t.Errorf("the cache figure did not reach the payload: %+v", report.Usage)
		}
		// The brief really was handed over: the request carries the card's
		// oracle text without the model having called a tool for it.
		if !strings.Contains(api.raw[0], "rationale_so_far") {
			t.Error("the opening message carried no brief")
		}
	})
}

// More than MaxQuestions and it stops being an interview and starts being a
// wall.
func TestAnInterviewIsCappedAtSixQuestions(t *testing.T) {
	var items []string
	for i := 0; i < 9; i++ {
		items = append(items, `{"question":"Question number `+string(rune('a'+i))+`?","angle":"role","fact":"f"}`)
	}
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", content: textBlock(
			`{"questions":[` + strings.Join(items, ",") + `]}`)}.json(),
	}}
	api.start(t)
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		report, err := Interview(context.Background(), c, d, "Sol Ring",
			InterviewRequest{Requested: "second-opinion"})
		if err != nil {
			t.Fatalf("interview: %v", err)
		}
		if len(report.Questions) != MaxQuestions {
			t.Errorf("returned %d questions, want the cap of %d",
				len(report.Questions), MaxQuestions)
		}
	})
}

// An answer that will not parse is a report saying so, not a crash and not a
// blank list that reads as "nothing to ask".
func TestAnUnparseableAnswerIsReportedAsOne(t *testing.T) {
	api := &scriptedAPI{replies: []string{
		reply{stop: "max_tokens", content: textBlock(`{"questions":[{"quest`)}.json(),
	}}
	api.start(t)
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		report, err := Interview(context.Background(), c, d, "Sol Ring",
			InterviewRequest{Requested: "second-opinion"})
		if err != nil {
			t.Fatalf("interview: %v", err)
		}
		if len(report.Questions) != 0 {
			t.Error("questions came back from an answer that did not parse")
		}
		if !strings.Contains(report.Reason, "max_tokens") {
			t.Errorf("the reason should name the stop reason: %q", report.Reason)
		}
	})
}

// A refusal is a report, not an error.
func TestARefusedInterviewIsAReport(t *testing.T) {
	api := &scriptedAPI{replies: []string{reply{stop: "refusal", content: ""}.json()}}
	api.start(t)
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		report, err := Interview(context.Background(), c, d, "Sol Ring",
			InterviewRequest{Requested: "second-opinion"})
		if err != nil {
			t.Fatalf("interview: %v", err)
		}
		if !report.Asked || report.Reason == "" || len(report.Questions) != 0 {
			t.Errorf("refusal reported as %+v", report)
		}
	})
}

// The user's steer is quoted as theirs rather than folded into the instruction.
func TestTheUsersFocusIsQuotedAsTheirs(t *testing.T) {
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", content: textBlock(`{"questions":[]}`)}.json(),
	}}
	api.start(t)
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		if _, err := Interview(context.Background(), c, d, "Sol Ring",
			InterviewRequest{Requested: "second-opinion",
				Focus: "  I cannot tell if it is win-more  "}); err != nil {
			t.Fatalf("interview: %v", err)
		}
		if !strings.Contains(api.raw[0], "in my words: I cannot tell if it is win-more") {
			t.Errorf("the focus was not quoted as the user's: %s", api.raw[0])
		}
	})
}

// The payload's field order is Python's, and it reaches a client.
func TestTheInterviewReportKeepsPythonsFieldOrder(t *testing.T) {
	raw, err := json.Marshal(InterviewReport{
		AnsweredBy: "claude", Mode: ModeRationaleInterview, Model: "m",
		Slug: "s", Card: "c", Asked: true,
		Questions: []Question{}, ToolCalls: []ToolCall{}, Never: NeverSentence,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{`"answered_by"`, `"mode"`, `"model"`, `"slug"`, `"card"`,
		`"asked"`, `"reason"`, `"stance"`, `"questions"`, `"questions_dropped"`,
		`"tool_calls"`, `"usage"`, `"never"`}
	at := -1
	for _, key := range want {
		next := strings.Index(string(raw), key)
		if next < 0 {
			t.Fatalf("the report lost %s: %s", key, raw)
		}
		if next < at {
			t.Errorf("%s is out of Python's order", key)
		}
		at = next
	}
}

func firstCardName(t *testing.T, d *deck.Deck) string {
	t.Helper()
	if len(d.Cards) == 0 {
		t.Fatal("fixture deck has no cards")
	}
	return d.Cards[0].Name
}
