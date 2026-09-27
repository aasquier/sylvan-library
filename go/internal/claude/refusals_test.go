package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// A stance that will not read is the caller's fault, and it must stay the
// caller's fault all the way out.
//
// `ErrStanceRejected` is what makes a route answer 422 rather than 502, and
// every surface reads the stance on its first line for exactly that reason --
// before the endpoint, before the mode, before anything is spent. A surface
// that folded a bad stance into its own error would turn "you sent me
// something I cannot read" into "the model failed", four minutes and one
// paid search later.
func TestABadStanceIsRefusedBeforeAnythingIsSpent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A number where an axis name belongs: readable JSON, unreadable stance.
	bad := json.RawMessage(`7.5`)
	// No endpoint at all, so anything that reached the wire would fail with a
	// different error and this test would notice.
	req := IntakeRequest{Requested: bad}
	d := intakeFixture()

	if _, _, err := DraftRationales(ctx, d, []string{"Cultivate"}, req); err == nil {
		t.Error("drafting accepted a stance that will not read")
	} else if !errors.Is(err, ErrStanceRejected) {
		t.Errorf("drafting refused with %v, which a route would answer 502 to", err)
	}
	if _, _, err := FileCards(ctx, d, []string{"Cultivate"}, req); err == nil {
		t.Error("filing accepted a stance that will not read")
	} else if !errors.Is(err, ErrStanceRejected) {
		t.Errorf("filing refused with %v, which a route would answer 502 to", err)
	}
	if _, _, err := DescribeDeck(ctx, d, req); err == nil {
		t.Error("describing accepted a stance that will not read")
	} else if !errors.Is(err, ErrStanceRejected) {
		t.Errorf("describing refused with %v, which a route would answer 502 to", err)
	}

	// And the theme proposal, whose stance is read after the transcript and
	// the slots but still before the call.
	if _, err := CheckProposal(nil, nil, bad, nil, "", nil, nil, "", nil, Endpoint{}); err == nil {
		t.Error("a proposal accepted a stance that will not read")
	} else if !errors.Is(err, ErrStanceRejected) {
		t.Errorf("the proposal refused with %v", err)
	}
}

// An API that answers with a failure ends the intake rather than being read
// as "this chunk had nothing to say".
//
// The distinction matters because a chunk that the model *declined* is a
// reported outcome -- its cards keep what they arrived with -- while a chunk
// the platform never answered is a fault. Reading the second as the first
// would file ninety-nine cards as unanswered and report a successful intake.
func TestAnIntakeStopsWhenThePlatformItselfFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := intakeFixture()

	for _, tc := range []struct {
		note string
		call func(Endpoint) error
	}{
		{"drafting", func(e Endpoint) error {
			_, _, err := DraftRationales(ctx, d, []string{"Cultivate"},
				IntakeRequest{Endpoint: e, Requested: "collaborator"})
			return err
		}},
		{"filing", func(e Endpoint) error {
			_, _, err := FileCards(ctx, d, []string{"Cultivate"},
				IntakeRequest{Endpoint: e, Requested: "collaborator"})
			return err
		}},
	} {
		api := &scriptedAPI{replies: []string{"!500", "!500", "!500", "!500", "!500", "!500"}}
		if err := tc.call(api.start(t)); err == nil {
			t.Errorf("%s reported success over an API that answered nothing but failures", tc.note)
		}
	}
}

// A mode whose tool set names something the registry does not have is
// refused where the conversation is BUILT, not where a tool is called.
//
// The difference is what gets spent: a mode that built its request and
// discovered the problem when the model asked for the tool would have paid
// for a turn first, and would have handed the model a refusal it could not
// act on.
func TestAConversationRefusesAnImpossibleToolSetBeforeItCalls(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{}}
	mode := Mode{Name: "invented", Instructions: "hi", ToolNames: []string{"set_card_field"}}
	_, err := Converse(context.Background(), mode, Request{
		Endpoint: api.start(t),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hello")),
		},
		Stance: testStance(t),
	})
	if err == nil {
		t.Fatal("a mode naming a tool that is not in the registry built a request")
	}
	if api.served != 0 {
		t.Errorf("%d calls were made for a request that could not be built", api.served)
	}
}

// Arguments the model sent that are not an object at all become an empty
// argument map rather than failing the turn.
//
// The tool then refuses for the reason that is actually useful -- "missing
// required argument(s) slug" -- which the model can act on, instead of the
// conversation ending on a decoder's complaint about bytes the model cannot
// see.
func TestToolArgumentsThatAreNotAnObjectBecomeNoArgumentsAtAll(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: toolUse("tu_1", "get_deck", `[1,2,3]`)}.json(),
		reply{stop: "end_turn", content: textBlock("I could not read that deck.")}.json(),
	}}
	turn, err := Converse(context.Background(), testMode(t, nil), Request{
		Endpoint: api.start(t),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("show me a deck")),
		},
		Stance:   testStance(t),
		MaxTurns: 4,
	})
	if err != nil {
		t.Fatalf("a tool call with unreadable arguments ended the conversation: %v", err)
	}
	if len(turn.ToolCalls) != 1 {
		t.Fatalf("the turn recorded %d tool calls", len(turn.ToolCalls))
	}
	if len(turn.ToolCalls[0].Arguments) != 0 {
		t.Errorf("arguments read as %#v, want none at all", turn.ToolCalls[0].Arguments)
	}
	// The model was handed a refusal it can act on, not a decoder's complaint.
	if len(api.raw) < 2 || !strings.Contains(api.raw[1], "missing required argument") {
		t.Error("the model was not told which argument was missing")
	}
}

// A hosted search that FAILED is not a search that found nothing, and the
// two arrive in the same block shaped differently: a list on success, a
// single error object on failure.
//
// Reading the second as the first is how a research answer comes back with
// "no source survived checking" over an outage, which reads to the person as
// "the web has nothing on this". The three edges here are the ones the happy
// path never shows: an error with no code, a result with no URL, and a
// result with no title.
func TestAFailedOrHalfEmptySearchResultIsReadForWhatItIs(t *testing.T) {
	t.Parallel()
	const untitled = `{"type":"web_search_result","url":"https://b","title":"",` +
		`"encrypted_content":"e","page_age":null}`
	const urlless = `{"type":"web_search_result","url":"","title":"nowhere",` +
		`"encrypted_content":"e","page_age":null}`
	blocks := `{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[` +
		untitled + "," + urlless + `]},` +
		`{"type":"web_search_tool_result","tool_use_id":"srv_2",` +
		`"content":{"type":"web_search_tool_result_error","error_code":""}},` +
		textBlock("here is what I found")

	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", content: blocks}.json(),
	}}
	turn, err := Converse(context.Background(), testMode(t, nil), Request{
		Endpoint: api.start(t),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("search for something")),
		},
		Stance: testStance(t),
	})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if len(turn.Searched) != 1 {
		t.Fatalf("%d pages survived, want the one with a URL: %+v", len(turn.Searched), turn.Searched)
	}
	if turn.Searched[0].URL != "https://b" {
		t.Errorf("the surviving page is %+v", turn.Searched[0])
	}
	// A page with no title is shown by its URL rather than as a blank link.
	if turn.Searched[0].Title != "https://b" {
		t.Errorf("an untitled page's title is %q, want its own URL", turn.Searched[0].Title)
	}
	// And the failure is reported even though it carried no code, because an
	// unreported failure is the one that reads as "found nothing".
	if len(turn.SearchErrors) != 1 {
		t.Fatalf("%d search errors, want the one that failed: %+v", len(turn.SearchErrors), turn.SearchErrors)
	}
	if turn.SearchErrors[0] == "" {
		t.Error("a failure with no error code was recorded as an empty string, " +
			"which reads as no failure at all")
	}
}

// The alternatives half of the slot argument is rule 2 made executable, and
// it reports a pool that cannot answer rather than deciding every alternative
// is off-colour.
//
// That failure mode is the one worth naming: with no identity to check
// against, every card the model named would be reported as ITS mistake --
// the most misleading answer this function can give, and indistinguishable
// from a model that invented eight cards.
func TestResolvingAlternativesOverAFailingPoolReportsTheFailure(t *testing.T) {
	t.Parallel()
	broken := schemalessConn(t)
	got, dropped, err := ResolveAlternatives(context.Background(), broken,
		[]any{"Sol Ring", "Arcane Signet"}, []string{"G"}, nil, nil)
	if err == nil {
		t.Fatalf("alternatives resolved to %+v (dropped %+v) over a pool that "+
			"cannot answer", got, dropped)
	}
	if got != nil {
		t.Errorf("a failed lookup still answered %+v", got)
	}
}

// A double-faced card named by its front face and by its whole name is one
// card, not two.
//
// The de-duplication runs twice on purpose and the second pass is the one
// this asserts: the first keys on what the model *typed*, the second on what
// the pool *resolved*. The two are the same string for almost every card and
// different strings for exactly this population -- the pool holds `A // B`
// and a model names the face it knows -- so without the second pass a reader
// gets the same card listed twice, which reads as an answer about two things.
//
// The names are the fixture pool's own rather than remembered, and what is
// asserted about them is a mechanism: that two spellings collapse, never
// anything about what the card does.
func TestACardNamedByItsFaceAndByItsWholeNameIsOneCard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	const whole = "Etali, Primal Conqueror // Etali, Primal Sickness"
	const front = "Etali, Primal Conqueror"
	withPool(t, func(c *pool.Conn) {
		cards, unresolved, err := ResolveCards(ctx, c, []any{whole, front}, MaxResearchCards)
		if err != nil {
			t.Fatalf("resolving: %v", err)
		}
		if unresolved != 0 {
			t.Fatalf("%d of the two spellings did not resolve; this test needs "+
				"both to reach the same pool row", unresolved)
		}
		if len(cards) != 1 {
			t.Errorf("one card named two ways resolved to %d entries: %+v", len(cards), cards)
		}
		// And the order is still the order named: the first spelling wins.
		single, _, err := ResolveCards(ctx, c, []any{front}, MaxResearchCards)
		if err != nil {
			t.Fatal(err)
		}
		if len(single) != 1 {
			t.Fatalf("the front face alone resolved to %d entries", len(single))
		}
	})
}

// A plan carrying a persona the roster does not have is refused rather than
// falling back to the plain voice.
//
// The fallback is right at the *door*, where a person may have sent anything
// and the plain voice is a safe read. It is wrong here: a plan is something
// this process built, so a persona it cannot resolve means the plan and the
// roster disagree, and answering in a different voice than the one the
// conversation has been running in is worse than saying so.
func TestAPlanWhoseVoiceIsNotInTheRosterIsRefused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if _, err := RunAsk(ctx, nil, &AskPlan{Persona: "invented-voice"}, ThemeRun{}); err == nil {
		t.Error("an ask ran under a voice that is not in the roster")
	}
	if _, err := RunProposal(ctx, nil, &ProposalPlan{Persona: "invented-voice"},
		ThemeRun{}); err == nil {
		t.Error("a proposal ran under a voice that is not in the roster")
	}
	// And with a real voice and no endpoint, the refusal comes from the pipe
	// rather than from the roster -- which is what says the check above was
	// about the persona and not about everything failing.
	_, err := RunProposal(ctx, nil, &ProposalPlan{Persona: DefaultPersona,
		Effective: Collaborator}, ThemeRun{})
	if err == nil {
		t.Error("a proposal reached the wire with no credential at all")
	} else if !errors.Is(err, ErrUnavailable) {
		t.Errorf("the refusal is %v, want the unavailable-endpoint one", err)
	}
}
