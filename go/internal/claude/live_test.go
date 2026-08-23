package claude

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The pipe, proved against the real API.
//
// Everything else in this package is driven against a scripted server, which
// is right: those tests are about the loop's behaviour and must run in CI, on
// every architecture, with no key and no spend. But a scripted server answers
// whatever it is told to, so it cannot say whether Anthropic **accepts** what
// this code sends -- whether the tool schemas validate, whether the cache
// breakpoints are placed where a cache read comes back, whether the request
// shape `client.go` pins from memory is the shape the API actually wants.
//
// That is commandment 14's argument about a page, applied to a wire: a green
// suite has not seen it. So this is opt-in, costs a few hundred tokens, and is
// what gets run after a key rotation or when something starts 401ing:
//
//	MTGLAB_LIVE_CLAUDE=1 ANTHROPIC_API_KEY=$(...) go test -run Live -v ./internal/claude/
//
// It is skipped everywhere else, CI included.
func liveOrSkip(t *testing.T) {
	t.Helper()
	if os.Getenv("MTGLAB_LIVE_CLAUDE") != "1" {
		t.Skip("live: set MTGLAB_LIVE_CLAUDE=1 (and a key) to spend tokens proving the wire")
	}
	if !Available() {
		t.Skip("live: no ANTHROPIC_API_KEY in the environment")
	}
}

func TestLiveTheSmallestCallProvesTheKey(t *testing.T) {
	liveOrSkip(t)
	report := Check(context.Background(), "")
	if !report.OK {
		t.Fatalf("the pipe is not open: %s", report.Error)
	}
	t.Logf("served by %s, stop %s, %d in / %d out: %q",
		report.ServedBy, report.StopReason, report.InputTokens,
		report.OutputTokens, report.Text)
}

// The one thing no scripted server can answer: does Anthropic accept the tool
// schemas, and does the cache breakpoint actually buy a cache read?
//
// Run twice with the same mode and stance. The prefix -- tools plus system --
// is byte-stable between them by construction, so the second call must report
// cache_read_input_tokens above zero. If it reports zero, the breakpoint is
// buying nothing, and the whole reason it is placed on the system block is
// wrong.
func TestLiveAToolRoundTripAndACacheRead(t *testing.T) {
	liveOrSkip(t)
	stance, err := Preset("second-opinion")
	if err != nil {
		t.Fatal(err)
	}
	// All seven tools and a real instruction block, because the prefix has to
	// clear Sonnet 5's 1,024-token minimum for a breakpoint to do anything at
	// all. A toy mode measures the floor, not the cache -- which is the same
	// fact `scan` runs into at 478 tokens, and it is a property of the prompt
	// rather than of the code.
	mode, err := NewMode(Mode{
		Name:    "live-pipe-check",
		Purpose: "prove the wire",
		ToolNames: []string{"deck_stats", "get_cards", "get_deck", "list_decks",
			"search_cards", "suggest_replacements", "validate_deck"},
		Instructions: "You are checking that a pipe is open, and the only thing " +
			"that matters is that you use the tools you have been given rather " +
			"than answering from memory. When asked what decks exist, call " +
			"list_decks and say what came back. Card facts come from the pool " +
			"and never from recall: if you find yourself about to state a " +
			"card's mana cost, colour identity or oracle text without having " +
			"looked it up, look it up. Answer in one short sentence, and if a " +
			"tool refuses, say plainly that it refused rather than guessing at " +
			"what it would have said.",
	})
	if err != nil {
		t.Fatal(err)
	}

	turns := make([]Turn, 0, 2)
	for attempt := 1; attempt <= 2; attempt++ {
		turn, err := Converse(context.Background(), mode, Request{
			Messages: ask(t),
			Stance:   stance,
		})
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		t.Logf("attempt %d: stop=%s tools=%d in=%d out=%d cached=%d text=%q",
			attempt, turn.StopReason, len(turn.ToolCalls), turn.InputTokens,
			turn.OutputTokens, turn.CacheReadTokens, turn.Text)
		turns = append(turns, turn)
	}
	first, second := turns[0], turns[1]

	// The tool schemas validated and the model reached for one -- which is also
	// rule 1 holding: it looked the library up rather than inventing it. (With
	// no deck source wired, the tool refuses; what is being proved is the round
	// trip, and that the model reports a refusal instead of guessing past it.)
	if len(first.ToolCalls) == 0 {
		t.Error("no tool was called: the model answered from recall, or the " +
			"schemas were not accepted")
	}
	if first.Text == "" && !first.Refused {
		t.Error("the conversation ended with no text and no refusal")
	}

	// The breakpoint pays twice -- across the turns of one tool loop, and
	// across consecutive calls of the same mode -- and both were measured on
	// the real API, 2026-08-22, with the seven tools and instruction block
	// below (a prefix of ~2,791 tokens):
	//
	//	cold:  attempt 1 in=82 cached=2791   (turn 1 wrote it, turn 2 read it)
	//	       attempt 2 in=82 cached=5582   (both turns read it)
	//	warm:  attempt 1 in=82 cached=5582   (both turns read it)
	//	       attempt 2 in=82 cached=5582
	//
	// 82 uncached input tokens for a two-turn conversation over a 2,791-token
	// prefix is the whole argument for where these markers are placed.
	//
	// **What is asserted is only the part that is invariant.** The cold column
	// is the more interesting one and it cannot be demanded: the cache has a
	// five-minute TTL and any earlier run leaves it warm, so a test insisting
	// on the cold ratio passes once and then fails for twenty minutes. That is
	// a test manufacturing its own flake -- so the assertion is that every call
	// reads a stable prefix from cache, which is true in both columns and is
	// the thing that breaks if the prefix ever starts drifting.
	if first.CacheReadTokens == 0 {
		t.Error("nothing was read from the cache -- the system breakpoint is " +
			"buying nothing, or the prefix is below the model's 1,024-token floor")
	}
	if second.CacheReadTokens < first.CacheReadTokens {
		t.Errorf("a repeat call read %d cached tokens where the first read %d "+
			"-- a byte-stable prefix cannot get less cacheable on the second "+
			"identical call, so something in it is drifting",
			second.CacheReadTokens, first.CacheReadTokens)
	}
}

// The first mode's own live case, and the gate Phase 6 sets for each of them
// as it lands: a real deck, a real pool, a real key, and the mode's own
// response schema round-tripping through the API.
//
// What only a live call can answer here is whether the API **accepts the
// schema** -- `RATIONALE_INTERVIEW`'s is generated data crossed from Python,
// and a schema the API rejects is a 400 that no scripted server would ever
// produce. The rest is worth watching rather than asserting hard: the model is
// asked for three to five questions and `OnlyQuestions` drops anything that is
// not one, so a drop count above zero is a real signal about the prompt and
// not a test failure.
func TestLiveTheRationaleInterviewAsksRealQuestions(t *testing.T) {
	liveOrSkip(t)
	d := fixtureDeck(t, "kaheera")
	stance, err := Preset("second-opinion")
	if err != nil {
		t.Fatal(err)
	}
	var report InterviewReport
	withPool(t, func(c *pool.Conn) {
		var runErr error
		report, runErr = Interview(context.Background(), c, d, "Sol Ring", InterviewRequest{
			Requested: stance,
			Deps:      tools.Deps{Pool: c},
			Limit:     &Collaborator,
		})
		if runErr != nil {
			t.Fatalf("the interview did not answer: %v", runErr)
		}
	})
	if !report.Asked {
		t.Fatalf("no call was made: %s", report.Reason)
	}
	if report.Reason != "" {
		t.Fatalf("the conversation ended early: %s", report.Reason)
	}
	if len(report.Questions) == 0 {
		t.Fatal("the interview came back with nothing to ask")
	}
	for i, q := range report.Questions {
		if !strings.HasSuffix(strings.TrimSpace(q.Question), "?") {
			t.Errorf("question %d is not a question: %q", i, q.Question)
		}
		if strings.TrimSpace(q.Fact) == "" {
			t.Errorf("question %d rests on nothing", i)
		}
	}
	if report.Never != NeverSentence {
		t.Errorf("the payload's promise is %q", report.Never)
	}
	t.Logf("%d questions (%d dropped) from %s, %d in / %d out / %d cached",
		len(report.Questions), report.QuestionsDropped, report.Model,
		report.Usage.InputTokens, report.Usage.OutputTokens,
		report.Usage.CacheReadTokens)
	for _, q := range report.Questions {
		t.Logf("  [%s] %s  (%s)", q.Angle, q.Question, q.Fact)
	}
}

// The slot argument on the real wire, and the one thing only a live call can
// answer: does the API accept a schema whose whole design is an ABSENCE?
//
// ADR 25's response schema has no `defence`, `verdict` or `summary` and
// forbids extra properties. A schema the API rejects is a 400 no scripted
// server would produce -- and this is the mode where that would matter most,
// because the failure mode of a rejected schema is falling back to prose,
// which is exactly the balanced answer the absence exists to prevent.
//
// The assertions are on what must be true rather than on what the model
// happened to say: every charge cites something (Python drops the ones that do
// not, so a drop count above zero is a signal about the prompt, not a
// failure), and nothing anywhere in the payload argues FOR the card.
func TestLiveTheSlotArgumentMakesOnlyTheCaseAgainst(t *testing.T) {
	liveOrSkip(t)
	d := fixtureDeck(t, "kaheera")
	stance, err := Preset("second-opinion")
	if err != nil {
		t.Fatal(err)
	}
	var report []wire.KV
	withPool(t, func(c *pool.Conn) {
		var runErr error
		report, runErr = Argue(context.Background(), c, d, "Sol Ring", ArgueRequest{
			Requested: stance,
			Deps:      tools.Deps{Pool: c},
			Limit:     &Collaborator,
		})
		if runErr != nil {
			t.Fatalf("the argument did not answer: %v", runErr)
		}
	})
	field := func(key string) any {
		for _, row := range report {
			if row.Key == key {
				return row.Value
			}
		}
		return nil
	}
	if asked, _ := field("asked").(bool); !asked {
		t.Fatalf("no call was made: %v", field("reason"))
	}
	if reason, _ := field("reason").(string); reason != "" {
		t.Fatalf("the conversation ended early: %s", reason)
	}
	charges, _ := field("charges").([]Charge)
	if len(charges) == 0 {
		t.Fatal("the argument came back with no case at all")
	}
	for i, c := range charges {
		if strings.TrimSpace(c.Fact) == "" {
			t.Errorf("charge %d rests on nothing", i)
		}
		if !slices.Contains(Grounds, c.Ground) {
			t.Errorf("charge %d has ground %q, which is not one of %v", i, c.Ground, Grounds)
		}
		if !slices.Contains(Strengths, c.Strength) {
			t.Errorf("charge %d has strength %q", i, c.Strength)
		}
	}
	// The absence, checked on the bytes that came back rather than on the
	// schema that went out.
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"defence"`, `"verdict"`, `"summary"`, `"in_favour"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Errorf("the answer carried a %s field", forbidden)
		}
	}
	t.Logf("%d charges (%d dropped) from %v", len(charges),
		field("charges_dropped"), field("model"))
	for _, c := range charges {
		t.Logf("  [%s/%s] %s  (%s)", c.Ground, c.Strength, c.Claim, c.Fact)
	}
	t.Logf("alternatives: %v", field("alternatives_dropped"))
}
