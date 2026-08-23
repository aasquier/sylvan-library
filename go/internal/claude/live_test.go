package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

// withRealPool is the full Scryfall pool when this machine has one
// (`data/mtg.duckdb` at the repo root, ~28 minutes of `mtglab data refresh`),
// and the 21-card test pool otherwise.
//
// The searching modes' live cases want the real one, and the first dossier run
// against the tiny pool showed why: the model is told to `get_cards` every
// competitor before writing a word about it, the tiny pool has heard of none
// of the commanders it names, and it spent all eight turns looking for one it
// could -- exhausting the ceiling with eight honest lookups. That was the
// fixture talking, not the code; against the real pool the same run finishes.
// The check is opt-in like the rest of this file, so CI never reaches it.
func withRealPool(t *testing.T, fn func(c *pool.Conn)) {
	t.Helper()
	real := filepath.Join("..", "..", "..", "data", "mtg.duckdb")
	if _, err := os.Stat(real); err != nil {
		t.Logf("no full pool at %s; using the 21-card test pool", real)
		withPool(t, fn)
		return
	}
	p := pool.New(real, nil)
	t.Cleanup(p.Close)
	if err := p.Use(context.Background(), func(c *pool.Conn) error {
		fn(c)
		return nil
	}); err != nil {
		t.Fatalf("leasing the full pool: %v", err)
	}
}

// The commander dossier on the real wire, and the things only a live call can
// answer: that the API accepts a schema carrying a hosted search beside it,
// that the container rides along on the turns after the search, that the
// pages the search returned reach `Turn.Searched`, and that the source check
// has something real to intersect. A scripted server answers all of that
// with whatever it is told. Costs one real search and the minutes it takes.
//
// The assertions are on what MUST be true rather than on what the model
// happened to write: every surviving source is one the search returned, every
// passage cites only survivors, every competitor is a pool row -- and the
// dossier was stored, because that is the row the deck page will serve.
func TestLiveTheDossierCitesPagesItActuallyRead(t *testing.T) {
	liveOrSkip(t)
	d := fixtureDeck(t, "kaheera")
	store := scratchStore(t)
	var report DossierReport
	withRealPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		plan, err := CheckDossier(ctx, c, "kaheera", d, DossierRequest{
			Requested: "second-opinion", Store: store, Limit: &Collaborator})
		if err != nil {
			t.Fatalf("the plan failed: %v", err)
		}
		if plan.Answer != nil {
			t.Fatalf("the plan answered without a call: %s", plan.Answer.Reason)
		}
		report, err = RunDossier(ctx, c, plan, DossierRun{
			Deps:   tools.Deps{Pool: c},
			OnTurn: func(done, max int) { t.Logf("turn %d of %d", done, max) },
		})
		if err != nil {
			t.Fatalf("the dossier did not answer: %v", err)
		}
		if store.Get(ctx, plan.Key) == nil {
			t.Error("the dossier was not stored")
		}
	})
	if !report.Asked {
		t.Fatalf("no call was made: %s", report.Reason)
	}
	if report.Reason != "" {
		// "No source survived checking" here is a real finding about the
		// prompt or the search, not a flake to retry.
		t.Fatalf("the run ended early: %s", report.Reason)
	}
	body, ok := report.Dossier.(DossierBody)
	if !ok {
		t.Fatalf("the dossier is a %T", report.Dossier)
	}
	if len(body.Sources) == 0 {
		t.Fatal("a dossier with no sources was not refused")
	}
	allowed := map[string]bool{}
	for _, s := range body.Sources {
		allowed[s.ID] = true
	}
	for name, passage := range map[string]Passage{"who": body.Who, "allies": body.Allies,
		"rivals": body.Rivals, "standing": body.Standing} {
		for _, id := range passage.SourceIDs {
			if !allowed[id] {
				t.Errorf("%s cites %q, which did not survive", name, id)
			}
		}
	}
	for _, c := range body.Competitors {
		if c.OracleText == nil {
			t.Errorf("competitor %q carries no pool text; it was not resolved", c.Name)
		}
	}
	t.Logf("%d sources (%d dropped), %d competitors (%d dropped), %d pages searched, "+
		"%d in / %d out / %d cached", len(body.Sources), body.SourcesDropped,
		len(body.Competitors), body.CompetitorsDropped, body.Searched,
		report.Usage.InputTokens, report.Usage.OutputTokens, report.Usage.CacheReadTokens)
	t.Logf("who: %s", body.Who.Prose)
	t.Logf("archetype (%s): %s", body.Archetype.Name, body.Archetype.Prose)
	for _, s := range body.Sources {
		t.Logf("  [%s] %s  %s", s.ID, s.Title, s.URL)
	}
}

// Research on the real wire, deck-blind: the question the pool cannot answer,
// answered from pages the search returned, with every finding resting on one.
func TestLiveResearchAnswersFromPagesItRead(t *testing.T) {
	liveOrSkip(t)
	plan, err := CheckResearch(
		"Why is Primeval Titan banned in Commander, and when was it banned?",
		nil, "", &Collaborator)
	if err != nil {
		t.Fatal(err)
	}
	var report ResearchReport
	withRealPool(t, func(c *pool.Conn) {
		report, err = RunResearch(context.Background(), c, plan, ResearchRun{
			OnTurn: func(done, max int) { t.Logf("turn %d of %d", done, max) },
		})
		if err != nil {
			t.Fatalf("research did not answer: %v", err)
		}
	})
	if !report.Asked {
		t.Fatalf("no call was made: %s", report.Reason)
	}
	if report.Reason != "" {
		t.Fatalf("the run ended early: %s", report.Reason)
	}
	body, ok := report.Research.(ResearchBody)
	if !ok {
		t.Fatalf("the answer is a %T", report.Research)
	}
	if len(body.Sources) == 0 || len(body.Findings) == 0 || body.Answer == "" {
		t.Fatalf("a thin answer came back whole: %+v", body)
	}
	if !slices.Contains(ResearchConfidence, body.Confidence) {
		t.Errorf("confidence is %q", body.Confidence)
	}
	for i, f := range body.Findings {
		if len(f.SourceIDs) == 0 {
			t.Errorf("finding %d rests on nothing", i)
		}
	}
	for _, card := range body.Cards {
		if resolved, ok := card.(ResearchCard); ok && resolved.OracleText == nil {
			t.Errorf("card %q is in the pool and carries no text", resolved.Name)
		}
	}
	t.Logf("%s (%s) -- %d findings (%d dropped), %d cards (%d unresolved), %d sources (%d dropped), %d searched",
		body.Answer, body.Confidence, len(body.Findings), body.FindingsDropped,
		len(body.Cards), body.CardsUnresolved, len(body.Sources), body.SourcesDropped, body.Searched)
	for _, f := range body.Findings {
		t.Logf("  - %s %v", f.Claim, f.SourceIDs)
	}
	for _, s := range body.Sources {
		t.Logf("  [%s] %s  %s", s.ID, s.Title, s.URL)
	}
}

// ---------------------------------------------------------- the theme lane

// liveTranscript is the conversation both live tests below are handed: three
// grounded kinds, in the short, shy register a first-timer actually types.
// That register is the point -- the readiness regression `Carry` was written
// after only reproduced with answers this thin.
var liveTranscript = []TranscriptTurn{
	{Role: "assistant", Text: "What is something you keep coming back to?"},
	{Role: "user", Text: "old horror films, the practical effects ones"},
	{Role: "assistant", Text: "And when a plan falls apart mid-evening?"},
	{Role: "user", Text: "I improvise"},
	{Role: "assistant", Text: "What are you like at game night?"},
	{Role: "user", Text: "I make deals"},
}

func liveSlots(t *testing.T) []Slot {
	t.Helper()
	slots, dropped := Ground([]any{
		map[string]any{"kind": "taste", "value": "practical-effects horror",
			"quote": "old horror films"},
		map[string]any{"kind": "temperament", "value": "improviser", "quote": "I improvise"},
		map[string]any{"kind": "posture", "value": "dealmaker", "quote": "I make deals"},
	}, liveTranscript)
	if len(slots) != 3 || dropped != 0 {
		t.Fatalf("the fixture does not reach the floor: %d kept, %d dropped", len(slots), dropped)
	}
	return slots
}

// One conversation turn on the real wire: a question comes back, it ends in a
// question mark, and every slot it claims is quoted from the transcript.
//
// The assertion that matters is the last one. `Ground` is the instrument ADR
// 20 rests on, and the failure it catches -- a model that has decided who
// somebody is and reports that back as something they said -- can only be
// observed against a real model's output.
func TestLiveTheThemeTurnAsksAboutThePerson(t *testing.T) {
	liveOrSkip(t)
	slots := liveSlots(t)
	raw := make([]any, 0, len(slots))
	for _, s := range slots {
		raw = append(raw, map[string]any{"kind": s.Kind, "value": s.Value, "quote": s.Quote})
	}
	plan, err := CheckAsk(transcriptAsAny(liveTranscript), raw, "second-opinion",
		nil, nil, nil, "", &Collaborator)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsCall() {
		t.Fatalf("the plan answered without a call: %s", plan.Answer.Reason)
	}
	var answer any
	withRealPool(t, func(c *pool.Conn) {
		answer, err = RunAsk(context.Background(), c, plan, ThemeRun{
			OnTurn: func(done, max int) { t.Logf("turn %d of %d", done, max) },
		})
		if err != nil {
			t.Fatalf("the turn did not answer: %v", err)
		}
	})
	report, ok := answer.(AskAnswered)
	if !ok {
		t.Fatalf("the turn is a %T -- a call was made, so it carries `never`", answer)
	}
	if report.Reason != "" {
		t.Fatalf("the turn ended early: %s", report.Reason)
	}
	if !strings.HasSuffix(report.Question, "?") {
		t.Errorf("the question does not end in a question mark: %q", report.Question)
	}
	// The readiness the client carried survives, and every slot is still
	// quoted from what the person typed -- re-checked here rather than trusted,
	// because this is the one place a real model is on the other end.
	if report.Grounded < 3 || !report.MayPropose {
		t.Errorf("the reading fell to %d of %d", report.Grounded, report.Floor)
	}
	said := strings.ToLower(strings.Join([]string{
		liveTranscript[1].Text, liveTranscript[3].Text, liveTranscript[5].Text}, " | "))
	for _, slot := range report.Slots {
		if !strings.Contains(said, strings.ToLower(slot.Quote)) {
			t.Errorf("slot %q is quoted as %q, which nobody typed", slot.Kind, slot.Quote)
		}
	}
	if report.Fact != nil && report.Fact.Source == "" {
		t.Error("a fact came back from nowhere")
	}
	t.Logf("question: %s", report.Question)
	if report.Fact != nil {
		t.Logf("fact (%s): %s", report.Fact.Source, report.Fact.Text)
	}
	for _, slot := range report.Slots {
		t.Logf("  %s: %s  <- %q", slot.Kind, slot.Value, slot.Quote)
	}
	t.Logf("%d in / %d out / %d cached", report.Usage.InputTokens,
		report.Usage.OutputTokens, report.Usage.CacheReadTokens)
}

// The proposal on the real wire, and the two things only a real run proves:
// every commander named resolves through the pool with an identity that is
// *exactly* the combination's, and every citation survived the source check.
//
// Slow -- measured at 226 seconds in Python -- which is why it is opt-in.
func TestLiveTheThemeProposalNamesRealCommanders(t *testing.T) {
	liveOrSkip(t)
	slots := liveSlots(t)
	raw := make([]any, 0, len(slots))
	for _, s := range slots {
		raw = append(raw, map[string]any{"kind": s.Kind, "value": s.Value, "quote": s.Quote})
	}
	plan, err := CheckProposal(transcriptAsAny(liveTranscript), raw, "second-opinion",
		nil, "", nil, nil, "", &Collaborator)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsCall() {
		t.Fatalf("the plan answered without a call: %s", plan.Answer.Reason)
	}
	var answer any
	withRealPool(t, func(c *pool.Conn) {
		answer, err = RunProposal(context.Background(), c, plan, ThemeRun{
			OnTurn: func(done, max int) { t.Logf("turn %d of %d", done, max) },
		})
		if err != nil {
			t.Fatalf("the proposal did not answer: %v", err)
		}
	})
	report, ok := answer.(ProposalAnswered)
	if !ok {
		if thin, was := answer.(ProposalReport); was {
			t.Fatalf("the proposal came back thin: %s", thin.Reason)
		}
		t.Fatalf("the proposal is a %T", answer)
	}
	if len(report.Combinations) == 0 {
		t.Fatal("nothing was proposed")
	}
	allowed := map[string]bool{}
	for _, s := range report.Sources {
		allowed[s.ID] = true
	}
	for _, combo := range report.Combinations {
		if len(combo.Commanders) == 0 {
			t.Errorf("%s came back with no commander and was not dropped", combo.Key)
		}
		for _, id := range combo.SourceIDs {
			if !allowed[id] {
				t.Errorf("%s cites %q, which did not survive", combo.Key, id)
			}
		}
		want := map[string]bool{}
		for _, c := range combo.Colors {
			want[c] = true
		}
		for _, cmd := range combo.Commanders {
			if cmd.OracleText == nil {
				t.Errorf("%s carries no pool text; it was not resolved", cmd.Name)
			}
			// The check a subset identity would pass and must not: a mono-white
			// legend is legal in a Selesnya deck and leads a mono-white one.
			//
			// **As a set, because the two orders genuinely differ.** The first
			// version of this compared the two joined -- and the live run
			// found Orzhov, where `colors.py` says `[W B]` and the pool says
			// `[B W]`. The code was right (`sameIdentity` compares sets, as
			// Python's `set(...) != identity` does); the assertion was the
			// thing that could only be wrong against a real answer.
			got := map[string]bool{}
			for _, c := range cmd.ColorIdentity {
				got[c] = true
			}
			if len(got) != len(want) {
				t.Errorf("%s leads %s with identity %v, want exactly %v",
					cmd.Name, combo.Key, cmd.ColorIdentity, combo.Colors)
				continue
			}
			for c := range want {
				if !got[c] {
					t.Errorf("%s leads %s with identity %v, want exactly %v",
						cmd.Name, combo.Key, cmd.ColorIdentity, combo.Colors)
					break
				}
			}
		}
	}
	t.Logf("%d combinations (%d dropped), %d commanders dropped, %d sources "+
		"(%d dropped), %d pages searched, %d in / %d out / %d cached",
		len(report.Combinations), report.CombinationsDropped, report.CommandersDropped,
		len(report.Sources), report.SourcesDropped, report.Searched,
		report.Usage.InputTokens, report.Usage.OutputTokens, report.Usage.CacheReadTokens)
	for _, combo := range report.Combinations {
		t.Logf("%s (%s): %s", combo.Key, combo.Name, combo.Reading)
		t.Logf("  grounded in: %s", combo.Grounding)
		for _, cmd := range combo.Commanders {
			t.Logf("  - %s  %v", cmd.Name, cmd.ColorIdentity)
		}
	}
}

// transcriptAsAny rebuilds a transcript as the `[]any` of maps a real request
// carries, so the live tests go through the same door a client does.
func transcriptAsAny(turns []TranscriptTurn) any {
	out := make([]any, 0, len(turns))
	for _, turn := range turns {
		out = append(out, map[string]any{"role": turn.Role, "text": turn.Text})
	}
	return out
}
