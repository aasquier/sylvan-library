package claude

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	_ "modernc.org/sqlite"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The pipe is tested by standing a scripted API in front of the real loop.
//
// No seam is added to production for it: the SDK honours ANTHROPIC_BASE_URL,
// so these tests drive the actual `Connect()` -- credential resolution
// included -- against an httptest server that plays a fixed sequence of
// responses and keeps every request body it was sent.
//
// That is deliberate, and it is what lets the interesting assertions exist at
// all. The three behaviours this file is really about -- the container
// ride-along, the moving cache marker, and the byte-stability of the tools
// block -- are properties of **the JSON that goes on the wire**, not of any
// value the loop returns. A test that inspected `Turn` could not see one of
// them. (`a-type-is-only-checked-in-the-shape-you-checked-it`, again: the
// corpus that proves a value can be blind to what encoding/json does with it.)

// scriptedAPI is a fake Anthropic that answers with `replies` in order and
// records what it was asked.
type scriptedAPI struct {
	replies  []string
	requests []map[string]any
	raw      []string
	served   int
}

// start stands the stub up and returns the [Endpoint] pointing at it.
//
// **Returning it rather than installing it is the whole point.** This used to
// publish the URL through ANTHROPIC_BASE_URL, which is one slot for the whole
// process -- so two stubs could not coexist and every test that used one was
// serial (ADR 39). Each caller now gets its own.
func (s *scriptedAPI) start(t *testing.T) Endpoint {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Errorf("request %d was not JSON: %v", s.served, err)
		}
		s.requests = append(s.requests, parsed)
		s.raw = append(s.raw, string(body))
		if s.served >= len(s.replies) {
			t.Errorf("the script ran out: request %d had no reply", s.served)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		reply := s.replies[s.served]
		s.served++
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(reply, "!") { // "!<status>" is an API failure
			var status int
			fmt.Sscanf(reply, "!%d", &status)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"nope"}}`))
			return
		}
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return EndpointAt(srv.URL, "test-key-not-a-real-one")
}

// reply builds one Message response body.
type reply struct {
	stop      string
	content   string // raw JSON array body, without the brackets
	container string
	in, out   int
	cached    int
	model     string
}

func (r reply) json() string {
	model := r.model
	if model == "" {
		model = "claude-sonnet-5"
	}
	container := "null"
	if r.container != "" {
		container = fmt.Sprintf(
			`{"id":%q,"expires_at":"2026-09-01T00:00:00Z","skills":[]}`, r.container)
	}
	return fmt.Sprintf(`{"id":"msg_x","type":"message","role":"assistant",`+
		`"model":%q,"content":[%s],"stop_reason":%q,"stop_sequence":null,`+
		`"container":%s,"usage":{"input_tokens":%d,"output_tokens":%d,`+
		`"cache_read_input_tokens":%d,"cache_creation_input_tokens":0}}`,
		model, r.content, r.stop, container, r.in, r.out, r.cached)
}

func textBlock(s string) string { return fmt.Sprintf(`{"type":"text","text":%q}`, s) }
func thinkingBlock() string     { return `{"type":"thinking","thinking":"","signature":"sig"}` }
func toolUse(id, name, input string) string {
	return fmt.Sprintf(`{"type":"tool_use","id":%q,"name":%q,"input":%s}`, id, name, input)
}
func searchOK(pages ...string) string {
	items := make([]string, 0, len(pages))
	for _, p := range pages {
		items = append(items, fmt.Sprintf(
			`{"type":"web_search_result","url":%q,"title":%q,"encrypted_content":"e","page_age":null}`,
			p, "title of "+p))
	}
	return fmt.Sprintf(`{"type":"web_search_tool_result","tool_use_id":"srv_1",`+
		`"content":[%s]}`, strings.Join(items, ","))
}
func searchErr(code string) string {
	return fmt.Sprintf(`{"type":"web_search_tool_result","tool_use_id":"srv_1",`+
		`"content":{"type":"web_search_tool_result_error","error_code":%q}}`, code)
}

func testMode(t *testing.T, tweak func(*Mode)) Mode {
	t.Helper()
	m := Mode{
		Name:         "test-mode",
		Purpose:      "exercise the loop",
		Instructions: "You are being tested.",
		ToolNames:    []string{"list_decks", "get_deck"},
	}
	if tweak != nil {
		tweak(&m)
	}
	built, err := NewMode(m)
	if err != nil {
		t.Fatalf("building the test mode: %v", err)
	}
	return built
}

// testStance is a resolved stance, because Converse now requires one -- a
// zero-value Stance would render an empty scope paragraph.
func testStance(t *testing.T) Stance {
	t.Helper()
	s, err := Preset("second-opinion")
	if err != nil {
		t.Fatalf("building a stance: %v", err)
	}
	return s
}

func ask(t *testing.T) []anthropic.MessageParam {
	t.Helper()
	return []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("what is in the library?")),
	}
}

// scratchLedger is a recorder over a throwaway app.db, and the path to read it
// back with. `Summarise` aggregates `stop_reason` away, and these tests are
// mostly about which stop reason was recorded, so the rows are read raw.
func scratchLedger(t *testing.T) (*ledger.Recorder, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatalf("seeding a scratch app.db: %v", err)
	}
	r, err := ledger.NewRecorder(path, nil)
	if err != nil {
		t.Fatalf("opening the ledger: %v", err)
	}
	t.Cleanup(func() { r.Close() })
	return r, path
}

// ---------------------------------------------------------------- the headline

// A paused turn carries text that reads finished. Returning it is this
// project's own worst failure mode wearing a different hat -- the Forge run
// that plays on with 96 cards and reports a plausible winner.
func TestAPausedTurnIsResumedAndNeverReturned(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "pause_turn", container: "cont_1",
			content: textBlock("Here is the complete answer.") + "," + searchOK("https://a")}.json(),
		reply{stop: "end_turn",
			content: textBlock("The actual answer.")}.json(),
	}}
	ep := api.start(t)

	turn, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if turn.Text != "The actual answer." {
		t.Errorf("a paused turn's text was returned as the answer: %q", turn.Text)
	}
	if turn.StopReason != "end_turn" {
		t.Errorf("stop reason %q, want end_turn", turn.StopReason)
	}
	if len(api.requests) != 2 {
		t.Fatalf("%d requests, want 2 -- the pause was not resumed", len(api.requests))
	}
	// The pause is resumed by re-sending, with nothing appended to nudge it
	// along: a trailing "continue" would be a new instruction rather than a
	// resumption.
	msgs, _ := api.requests[1]["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("resumed with %d messages, want 2 (the ask and the paused turn)", len(msgs))
	}
	last, _ := msgs[1].(map[string]any)
	if role, _ := last["role"].(string); role != "assistant" {
		t.Errorf("the resumed request's last message is %q, want the paused assistant turn", role)
	}
	// A paused turn still contributes what its searches found.
	if len(turn.Searched) != 1 || turn.Searched[0].URL != "https://a" {
		t.Errorf("a paused turn's search results were dropped: %+v", turn.Searched)
	}
}

// The dated web search filters inside a code-execution container, so a second
// request carrying the first turn's blocks is refused unless the container
// rides along. It must NOT ride along before one exists.
func TestTheContainerRidesAlongOnceAServerToolHasRun(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "tool_use", container: "cont_7",
			content: searchOK("https://a") + "," + toolUse("tu_2", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: textBlock("done")}.json(),
	}}
	ep := api.start(t)

	if _, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)}); err != nil {
		t.Fatalf("converse: %v", err)
	}
	if len(api.requests) != 3 {
		t.Fatalf("%d requests, want 3", len(api.requests))
	}
	if _, present := api.requests[0]["container"]; present {
		t.Error("the first request carried a container before any server tool had run")
	}
	if _, present := api.requests[1]["container"]; present {
		t.Error("the second request carried a container before one had been issued")
	}
	if got := api.requests[2]["container"]; got != "cont_7" {
		t.Errorf("the third request's container is %v, want cont_7 -- "+
			"without it the API refuses the pending server-tool blocks", got)
	}
}

// A container, once issued, keeps riding even on a turn that reports none.
func TestTheContainerIsKeptWhenATurnReportsNone(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", container: "cont_7", content: toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "tool_use", content: toolUse("tu_2", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: textBlock("done")}.json(),
	}}
	ep := api.start(t)
	if _, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)}); err != nil {
		t.Fatalf("converse: %v", err)
	}
	for i := 1; i < 3; i++ {
		if got := api.requests[i]["container"]; got != "cont_7" {
			t.Errorf("request %d's container is %v, want cont_7 kept from turn one", i, got)
		}
	}
}

// ------------------------------------------------------------------- caching

// countMarkers walks a request body and counts cache_control breakpoints,
// separately for the system block and the tool results, and says which tool
// result carries one.
func countMarkers(t *testing.T, req map[string]any) (system int, onResults []string) {
	t.Helper()
	for _, b := range req["system"].([]any) {
		if _, ok := b.(map[string]any)["cache_control"]; ok {
			system++
		}
	}
	msgs, _ := req["messages"].([]any)
	for _, m := range msgs {
		content, ok := m.(map[string]any)["content"].([]any)
		if !ok {
			continue
		}
		for _, block := range content {
			b, _ := block.(map[string]any)
			if b["type"] != "tool_result" {
				continue
			}
			if _, ok := b["cache_control"]; ok {
				onResults = append(onResults, b["tool_use_id"].(string))
			}
		}
	}
	return system, onResults
}

// The conversation's marker MOVES rather than accumulating. Markers max out at
// four per request, so a marker added per turn would make the fifth turn a 400
// -- and the earlier breakpoints keep working as read points once the marker
// itself has gone.
func TestTheConversationCacheMarkerMovesRatherThanAccumulating(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "tool_use", content: toolUse("tu_2", "list_decks", "{}")}.json(),
		reply{stop: "tool_use", content: toolUse("tu_3", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: textBlock("done")}.json(),
	}}
	ep := api.start(t)
	if _, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)}); err != nil {
		t.Fatalf("converse: %v", err)
	}

	want := []string{"", "tu_1", "tu_2", "tu_3"}
	for i, req := range api.requests {
		system, onResults := countMarkers(t, req)
		if system != 1 {
			t.Errorf("request %d has %d system breakpoints, want exactly 1", i, system)
		}
		if want[i] == "" {
			if len(onResults) != 0 {
				t.Errorf("request %d marked tool results %v, but none existed yet", i, onResults)
			}
			continue
		}
		if len(onResults) != 1 {
			t.Errorf("request %d carries %d tool-result markers (%v), want exactly 1 -- "+
				"a marker per turn is a 400 by the fifth", i, len(onResults), onResults)
			continue
		}
		if onResults[0] != want[i] {
			t.Errorf("request %d marked %q, want the newest result %q",
				i, onResults[0], want[i])
		}
	}
}

// The tools block renders FIRST in the prompt, so an unstable rendering
// invalidates everything after it on every single request -- silently, and for
// free. The SDK's ExtraFields marshals in Go map-iteration order, which is
// randomised; this is the test that keeps the schema out of it.
func TestTheToolsBlockIsByteStable(t *testing.T) {
	t.Parallel()
	mode := testMode(t, func(m *Mode) {
		m.ToolNames = []string{"deck_stats", "get_cards", "get_deck", "list_decks",
			"search_cards", "suggest_replacements", "validate_deck"}
	})
	var first string
	for i := 0; i < 200; i++ {
		schemas, err := mode.Schemas()
		if err != nil {
			t.Fatalf("schemas: %v", err)
		}
		raw, err := json.Marshal(schemas)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if i == 0 {
			first = string(raw)
			continue
		}
		if string(raw) != first {
			t.Fatalf("the tools block is not byte-stable -- the prompt cache is "+
				"being invalidated on every request.\nfirst: %s\nnow:   %s", first, string(raw))
		}
	}
	// And the schema really did survive the conversion, `required: []`
	// included -- the empty case is the one a typed field would have dropped.
	if !strings.Contains(first, `"additionalProperties":false`) {
		t.Error("additionalProperties did not reach the wire")
	}
	if !strings.Contains(first, `"required":[]`) {
		t.Error(`a tool with no required arguments should still render "required":[]`)
	}

	// The loop above fails at random. This does not.
	//
	// Instability needs two ExtraFields keys the SDK has no field of its own
	// for; with two, the loop catches it within a few iterations, but the
	// failure it prints is a byte diff nobody can act on. Counting the novel
	// keys is the same guard stated as a rule, and it is the one that will
	// still be here explaining itself when somebody adds a second key that
	// looks entirely harmless.
	one, err := toolParam(map[string]any{
		"name": "x", "description": "y",
		"input_schema": map[string]any{
			"type": "object", "properties": map[string]any{},
			"required": []string{}, "additionalProperties": false,
		},
	})
	if err != nil {
		t.Fatalf("toolParam: %v", err)
	}
	novel := 0
	for key := range one.OfTool.InputSchema.ExtraFields {
		switch key {
		case "properties", "required", "type": // the SDK emits these itself
		default:
			novel++
		}
	}
	if novel > 1 {
		t.Errorf("the tool input schema carries %d ExtraFields keys the SDK has "+
			"no field for; more than one is appended in Go's randomised map "+
			"order, which re-renders the tools block on every request and "+
			"silently voids the prompt cache. Put the new key through a typed "+
			"field, or shadow one the struct already emits.", novel)
	}
}

// The system breakpoint covers the tools too, because tools render first. That
// is only true if the marker is on the system block and the tools are in the
// same request -- which is what this asserts, rather than the comment claiming
// it.
func TestTheSystemBreakpointIsPresentOnEveryRequest(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: textBlock("done")}.json(),
	}}
	ep := api.start(t)
	if _, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)}); err != nil {
		t.Fatalf("converse: %v", err)
	}
	for i, req := range api.requests {
		if _, ok := req["tools"]; !ok {
			t.Errorf("request %d sent no tools", i)
		}
		system, _ := countMarkers(t, req)
		if system != 1 {
			t.Errorf("request %d has %d system breakpoints, want 1", i, system)
		}
	}
}

// The assistant turn goes back whole, thinking blocks included. Sonnet 5
// returns them with empty text by default and they still have to be replayed
// unedited -- a turn that stripped them would be replaying a different
// conversation than the one the model had.
func TestThinkingBlocksAreReplayedUnedited(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use",
			content: thinkingBlock() + "," + toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: textBlock("done")}.json(),
	}}
	ep := api.start(t)
	if _, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)}); err != nil {
		t.Fatalf("converse: %v", err)
	}
	msgs, _ := api.requests[1]["messages"].([]any)
	if len(msgs) < 2 {
		t.Fatalf("the assistant turn was not replayed: %v", msgs)
	}
	assistant, _ := msgs[1].(map[string]any)
	blocks, _ := assistant["content"].([]any)
	var kinds []string
	for _, b := range blocks {
		kinds = append(kinds, fmt.Sprint(b.(map[string]any)["type"]))
	}
	if len(kinds) != 2 || kinds[0] != "thinking" {
		t.Errorf("the replayed turn holds %v, want the thinking block first and "+
			"intact -- it is part of the conversation the model had", kinds)
	}
	if sig := blocks[0].(map[string]any)["signature"]; sig != "sig" {
		t.Errorf("the thinking block's signature was not replayed: %v", sig)
	}
}

// ------------------------------------------------------------ server results

// A search that failed and a search that found nothing look identical from an
// empty page list, and they want different answers. On success the content is
// a list; on failure it is a single error object.
func TestAFailedSearchIsNotAnEmptyOne(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", content: searchErr("max_uses_exceeded") + "," + textBlock("hm")}.json(),
	}}
	ep := api.start(t)
	turn, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if len(turn.Searched) != 0 {
		t.Errorf("a failed search produced pages: %+v", turn.Searched)
	}
	if len(turn.SearchErrors) != 1 || turn.SearchErrors[0] != "max_uses_exceeded" {
		t.Errorf("search errors %v, want [max_uses_exceeded] -- a failure read "+
			"as an empty result set is a search that 'found nothing'", turn.SearchErrors)
	}
}

func TestSearchPagesAreDeduplicatedOnURL(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: searchOK("https://a", "https://b") + "," +
			toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: searchOK("https://b", "https://c") + "," + textBlock("done")}.json(),
	}}
	ep := api.start(t)
	turn, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	var got []string
	for _, p := range turn.Searched {
		got = append(got, p.URL)
	}
	want := []string{"https://a", "https://b", "https://c"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("pages %v, want %v in the order they came back, deduplicated", got, want)
	}
}

// ------------------------------------------------------------------ refusals

// A refusal can carry an empty content list. Reading content first is how this
// becomes a panic instead of a message somebody can act on.
func TestARefusalWithNoContentIsAnAnswerAndNotAPanic(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "refusal", content: "", in: 11, out: 3}.json(),
	}}
	ep := api.start(t)
	led, ledPath := scratchLedger(t)
	turn, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t), Ledger: led})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if !turn.Refused || turn.Text != "" || turn.StopReason != "refusal" {
		t.Errorf("refusal not reported as one: %+v", turn)
	}
	rows := ledgerRows(t, ledPath)
	if len(rows) != 1 || rows[0].StopReason != "refusal" {
		t.Errorf("a refusal should still be recorded: %+v", rows)
	}
}

// ---------------------------------------------------------------- tool round trip

// Three failures are recoverable and are handed back as tool results: the
// model's move is to ask differently. A refused write in particular should read
// as "that door does not exist" rather than ending the conversation.
func TestARefusedToolIsATurnAndNotTheEnd(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: toolUse("tu_1", "set_card_field",
			`{"slug":"gyome","card":"Bake into a Pie","field":"why","value":"because"}`)}.json(),
		reply{stop: "end_turn", content: textBlock("understood")}.json(),
	}}
	ep := api.start(t)
	turn, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)})
	if err != nil {
		t.Fatalf("a refused tool ended the conversation: %v", err)
	}
	if turn.Text != "understood" {
		t.Errorf("text %q", turn.Text)
	}
	body := api.raw[1]
	if !strings.Contains(body, "ToolNotAllowed") {
		t.Errorf("the model was not told the recorded fault name it is owed: %s", body)
	}
	if !strings.Contains(body, `"is_error":true`) {
		t.Error("a refused tool was handed back as a success")
	}
	// And the call is still on the record: ADR 14's third boundary is that a
	// user can tell what was read, refusals included.
	if len(turn.ToolCalls) != 1 || turn.ToolCalls[0].Tool != "set_card_field" {
		t.Errorf("tool calls %+v", turn.ToolCalls)
	}
}

// A deck that is not there is recoverable too, and the model is told the name
// that failed -- which is all the refusal owes it, since the recovery is to
// call list_decks and ask again.
func TestAMissingDeckIsRecoverableAndNamesTheSlug(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: toolUse("tu_1", "get_deck", `{"slug":"no-such-deck"}`)}.json(),
		reply{stop: "end_turn", content: textBlock("ok")}.json(),
	}}
	ep := api.start(t)
	if _, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t), Deps: tools.Deps{Source: emptySource{}}}); err != nil {
		t.Fatalf("a missing deck ended the conversation: %v", err)
	}
	if body := api.raw[1]; !strings.Contains(body, "DeckNotFound: no-such-deck") {
		t.Errorf("want `DeckNotFound: no-such-deck` -- the recorded fault name, "+
			"then the bare slug.\ngot: %s", body)
	}
}

// ------------------------------------------------------------------ ceilings

// A mode that has not finished by the ceiling is looping rather than working.
// Handing back the last turn's text is the failure this refuses to commit.
func TestTheTurnCeilingIsAnErrorAndNotAnAnswer(t *testing.T) {
	t.Parallel()
	replies := make([]string, 6)
	for i := range replies {
		replies[i] = reply{stop: "tool_use", in: 10, out: 2,
			content: toolUse(fmt.Sprintf("tu_%d", i), "list_decks", "{}")}.json()
	}
	api := &scriptedAPI{replies: replies}
	ep := api.start(t)
	led, ledPath := scratchLedger(t)

	_, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t), Ledger: led})
	if !errors.Is(err, ErrModeExhausted) {
		t.Fatalf("want ErrModeExhausted, got %v", err)
	}
	if api.served != MaxToolTurns {
		t.Errorf("made %d requests, want the ceiling of %d", api.served, MaxToolTurns)
	}
	rows := ledgerRows(t, ledPath)
	if len(rows) != 1 {
		t.Fatalf("want one ledger row for an exhausted conversation, got %d", len(rows))
	}
	// The tokens a conversation burned before failing are exactly the ones
	// worth seeing in a roll-up.
	if rows[0].StopReason != "exhausted" || rows[0].Requests != MaxToolTurns {
		t.Errorf("exhausted row is %+v", rows[0])
	}
	if rows[0].InputTokens != 60 || rows[0].OutputTokens != 12 {
		t.Errorf("exhausted row lost the tokens it burned: %+v", rows[0])
	}
}

// ------------------------------------------------------------------- the ledger

func TestCacheReadsAreCountedBesideInputTokensAndNotInside(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", in: 100, out: 10, cached: 0,
			content: toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", in: 7, out: 20, cached: 2000, content: textBlock("done")}.json(),
	}}
	ep := api.start(t)
	turn, err := Converse(context.Background(), testMode(t, nil), Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)})
	if err != nil {
		t.Fatalf("converse: %v", err)
	}
	if turn.InputTokens != 107 || turn.OutputTokens != 30 || turn.CacheReadTokens != 2000 {
		t.Errorf("token accounting is %d in / %d out / %d cached, want 107/30/2000 "+
			"summed across turns", turn.InputTokens, turn.OutputTokens, turn.CacheReadTokens)
	}
}

// An API failure records nothing: the roll-up counts conversations, and a
// request the API refused is not one.
func TestAnAPIFailureRecordsNothing(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{"!401"}}
	ep := api.start(t)
	led, ledPath := scratchLedger(t)
	_, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t), Ledger: led})
	if err == nil {
		t.Fatal("a 401 was not reported")
	}
	// And it is reported in the words somebody reads at 2am -- as the whole
	// of `err.Error()`, not merely inside it. The recorded failure text is
	// the explanation and nothing else; a route's 502 and a
	// job's error both render Explain(err, Model) and would hide a prefix here, so
	// this is the one place a stray "mode: " on the error itself is visible
	// -- a mutation run found the assertion missing.
	if !strings.Contains(err.Error(), "It may have expired") {
		t.Errorf("a 401 should say the key may have expired: %v", err)
	}
	if err.Error() != Explain(err, Model) {
		t.Errorf("the error reads %q; it must be exactly its explanation %q", err.Error(), Explain(err, Model))
	}
	if rows := ledgerRows(t, ledPath); len(rows) != 0 {
		t.Errorf("an API failure was recorded as a conversation: %+v", rows)
	}
}

func TestTheServedByModelIsRecordedAndNotTheAskedForOne(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", model: "claude-sonnet-5-served", content: textBlock("hi")}.json(),
	}}
	ep := api.start(t)
	led, ledPath := scratchLedger(t)
	if _, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t), Ledger: led}); err != nil {
		t.Fatalf("converse: %v", err)
	}
	rows := ledgerRows(t, ledPath)
	if len(rows) != 1 || rows[0].Model != "claude-sonnet-5-served" {
		t.Errorf("rows %+v, want the model that served the answer", rows)
	}
}

// ---------------------------------------------------------------- turn reporting

// OnTurn is a ceiling and not an estimate: a loop that finishes on turn two of
// six jumps straight to done.
func TestOnTurnFiresAsEachTurnBeginsAndIsACeiling(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", content: toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: textBlock("done")}.json(),
	}}
	ep := api.start(t)
	var seen [][2]int
	if _, err := Converse(context.Background(), testMode(t, nil), Request{
		Endpoint: ep,
		Messages: ask(t),
		Stance:   testStance(t),
		OnTurn:   func(done, max int) { seen = append(seen, [2]int{done, max}) },
	}); err != nil {
		t.Fatalf("converse: %v", err)
	}
	want := [][2]int{{0, MaxToolTurns}, {1, MaxToolTurns}}
	if fmt.Sprint(seen) != fmt.Sprint(want) {
		t.Errorf("OnTurn saw %v, want %v", seen, want)
	}
}

// A zero-value Stance is one struct literal away, and its scope paragraph is
// simply missing -- a real change to what the model is told, visible nowhere,
// because a Go map lookup answers "" for an unknown scope.
func TestAZeroValueStanceIsRefusedRatherThanRenderingAnEmptyScope(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", content: textBlock("hi")}.json(),
	}}
	ep := api.start(t)
	_, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t)}) // no Stance
	if err == nil {
		t.Fatal("a zero-value stance was accepted -- the mode went out with no " +
			"scope paragraph at all")
	}
	if api.served != 0 {
		t.Errorf("%d requests were made before the stance was checked", api.served)
	}
	// And a resolved stance really does put its paragraph in the prompt, so the
	// check above is guarding something that exists.
	sys := testMode(t, nil).System(testStance(t))
	if !strings.Contains(sys, "Scope:") {
		t.Errorf("a resolved stance rendered no scope paragraph: %q", sys)
	}
}

// Turn's json tags are the recorded field names, and a mode's payload is
// built
// from them. They are pinned here rather than left as decoration: a tag that
// nothing marshals is a promise nobody is keeping, and the last time a type was
// proved correct without going through encoding/json it reached the wire as
// something else entirely.
func TestATurnKeepsTheRecordedFieldNames(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(Turn{
		Mode: "m", Model: "claude-sonnet-5", StopReason: "end_turn", Text: "t",
		ToolCalls:       []ToolCall{{Tool: "get_deck", Arguments: map[string]any{"slug": "gyome"}}},
		InputTokens:     1,
		OutputTokens:    2,
		Searched:        []Page{{URL: "https://a", Title: "A"}},
		SearchErrors:    []string{},
		CacheReadTokens: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"mode":"m","model":"claude-sonnet-5","stop_reason":"end_turn",` +
		`"text":"t","tool_calls":[{"tool":"get_deck","arguments":{"slug":"gyome"}}],` +
		`"input_tokens":1,"output_tokens":2,` +
		`"searched":[{"url":"https://a","title":"A"}],"search_errors":[],` +
		`"refused":false,"cache_read_tokens":3}`
	if string(raw) != want {
		t.Errorf("a Turn renders as\n %s\nwant\n %s", raw, want)
	}
}

// The empty collections are `[]` and never `null`: a mode that searched nothing
// and a mode whose search list was lost must not look the same to a caller
// checking citations against it.
func TestATurnsEmptyEvidenceIsAnEmptyListAndNotNull(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", content: textBlock("no searching here")}.json(),
	}}
	ep := api.start(t)
	turn, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(turn)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"searched":[]`, `"search_errors":[]`, `"tool_calls":[]`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("want %s in %s", want, raw)
		}
	}
}

// ---------------------------------------------------------------- helpers

// emptySource is a deckread.Source with nothing on its shelves -- enough to
// prove that "the library is reachable and the deck is not in it" is a
// different answer from "there is no library".
type emptySource struct{}

func (emptySource) Slugs(context.Context) ([]string, error) { return nil, nil }
func (emptySource) Get(_ context.Context, slug string) (*deck.Deck, error) {
	return nil, fmt.Errorf("no deck %q", slug)
}
func (emptySource) All(context.Context) ([]*deck.Deck, error)        { return nil, nil }
func (emptySource) ReadText(context.Context, string) (string, error) { return "", nil }
func (emptySource) Writable() bool                                   { return false }

func ledgerRows(t *testing.T, path string) []ledger.Row {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("opening the scratch app.db: %v", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(context.Background(),
		"SELECT mode, model, stop_reason, requests, input_tokens,"+
			" output_tokens, cache_read_tokens FROM claude_usage ORDER BY id")
	if err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	defer rows.Close()
	out := []ledger.Row{}
	for rows.Next() {
		var r ledger.Row
		if err := rows.Scan(&r.Mode, &r.Model, &r.StopReason, &r.Requests,
			&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens); err != nil {
			t.Fatalf("scanning a ledger row: %v", err)
		}
		out = append(out, r)
	}
	return out
}

// The assistant turn goes back as it arrived -- every block's own bytes --
// and not through the SDK's typed conversion. The case that found it: the
// dated web search filters inside a code-execution container, and the turn
// that ran it carries a `code_execution_tool_result` whose content is the
// ENCRYPTED result variant. `ToParam` in anthropic-sdk-go v1.66.0 has no
// branch for that variant and renders an empty plain result the API refuses
// with a 400 -- on the second turn of every dossier, after the search was
// paid for. Found by the live case, 2026-08-23, and by nothing else.
func TestTheAssistantTurnIsResentAsItArrived(t *testing.T) {
	t.Parallel()
	encrypted := `{"type":"code_execution_tool_result","tool_use_id":"srv_9",` +
		`"content":{"type":"encrypted_code_execution_result","encrypted_stdout":"c2VjcmV0",` +
		`"content":[],"return_code":0,"stderr":""}}`
	api := &scriptedAPI{replies: []string{
		reply{stop: "tool_use", container: "cont_1",
			content: thinkingBlock() + "," + encrypted + "," + searchOK("https://a") + "," +
				toolUse("tu_1", "list_decks", "{}")}.json(),
		reply{stop: "end_turn", content: textBlock("done")}.json(),
	}}
	ep := api.start(t)
	if _, err := Converse(context.Background(), testMode(t, nil),
		Request{Endpoint: ep, Messages: ask(t), Stance: testStance(t)}); err != nil {
		t.Fatalf("converse: %v", err)
	}
	if len(api.requests) != 2 {
		t.Fatalf("%d requests, want 2", len(api.requests))
	}
	msgs, _ := api.requests[1]["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("the second request carried %d messages, want 3", len(msgs))
	}
	assistant, _ := msgs[1].(map[string]any)
	blocks, _ := assistant["content"].([]any)
	if len(blocks) != 4 {
		t.Fatalf("the assistant turn carried %d blocks, want 4 (thinking, result, search, tool use)", len(blocks))
	}
	sent, _ := json.Marshal(blocks[1])
	var want any
	_ = json.Unmarshal([]byte(encrypted), &want)
	var got any
	_ = json.Unmarshal(sent, &got)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the code-execution result was re-sent as\n  %s\nnot as it arrived\n  %s", sent, encrypted)
	}
	if !strings.Contains(string(sent), `"encrypted_code_execution_result"`) {
		t.Errorf("the encrypted variant was lost on the way back: %s", sent)
	}
	// And the thinking block kept its signature, which it must.
	thinking, _ := json.Marshal(blocks[0])
	if !strings.Contains(string(thinking), `"signature":"sig"`) {
		t.Errorf("the thinking block lost its signature: %s", thinking)
	}
}
