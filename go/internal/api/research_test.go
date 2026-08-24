package api

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Research's route. `internal/claude` holds the mode to a corpus; what these
// prove is the layer above it -- the refusals and their order, the job born
// finished, the job whose result is the report, the label's cut, the in-flight
// dedupe -- and the one structural claim worth proving end to end: a tool the
// model reaches for that would touch a deck is refused, because the mode was
// never handed a library to reach.

const researchAt = "/api/claude/research"

const groundedAnswer = `{"answer":"It is still played in stompy lists.",` +
	`"findings":[{"claim":"cEDH primers rate it a trap.","source_ids":["s1"]},` +
	`{"claim":"Rests on nothing.","source_ids":[]}],` +
	`"cards":["Craterhoof Behemoth","Spoiled Card"],"confidence":"contested",` +
	`"sources":[{"id":"s1","title":"t","url":"https://edhrec.com/real"},` +
	`{"id":"s2","title":"t","url":"https://example.com/invented"}]}`

func TestResearchRefusesWhatItCanBeforeAnyJob(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	long := strings.Repeat("x", 2001)
	for _, row := range []struct {
		body   string
		status int
		detail string
	}{
		{`{}`, 422, "Ask something"},
		{`{"question":"   "}`, 422, "Ask something"},
		{`{"question":null}`, 422, "Ask something"},
		{`{"question":"` + long + `"}`, 422, "2001 characters, and the ceiling is 2000"},
		{`{"question":"q","stance":"emperor"}`, 422, "is not a stance preset"},
		// A real question and no key: 503 from the request, never a job.
		{`{"question":"Is Goreclaw still played?"}`, 503, "no ANTHROPIC_API_KEY"},
	} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", researchAt, row.body)
		if status != row.status {
			t.Errorf("%.60s answered %d, want %d: %s", row.body, status, row.status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, row.detail) {
			t.Errorf("%.60s said %q, want it to contain %q", row.body, detail, row.detail)
		}
	}
	if got := rig.jobs.All(alice.UserID); len(got) != 0 {
		t.Errorf("%d jobs were queued by refused requests", len(got))
	}
}

func TestAtStanceOffResearchIsAJobBornFinishedEvenWithNoKey(t *testing.T) {
	t.Parallel()
	rig := newJobRig(t, noCredential)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "POST", researchAt,
		`{"question":"Is Goreclaw still played?","stance":"off"}`)
	if status != 200 || payload["status"] != "done" || payload["kind"] != ResearchKind {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["label"] != "research: Is Goreclaw still played?" {
		t.Errorf("label is %v", payload["label"])
	}
	result, _ := payload["result"].(map[string]any)
	if result["asked"] != false || !strings.Contains(result["reason"].(string), "stance is off") {
		t.Errorf("result is %v", result)
	}
}

func TestResearchIsAJobWhoseResultIsTheReport(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("The Real Page")+","+said(groundedAnswer))}}
	claudeSet := api.start(t)
	rig := newJobRig(t, claudeSet)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "POST", researchAt,
		`{"question":"  Is Goreclaw still played?  "}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["kind"] != ResearchKind || payload["label"] != "research: Is Goreclaw still played?" {
		t.Fatalf("job is %s", raw)
	}
	done, doneRaw := rig.await(t, payload["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v", done["status"], done["error"])
	}
	want := []string{"answered_by", "mode", "model", "question", "asked", "reason", "stance",
		"research", "generated_at", "usage", "never"}
	if err := orderedAs(string(doneRaw), want); err != nil {
		t.Errorf("%v\n%s", err, doneRaw)
	}
	result, _ := done["result"].(map[string]any)
	resultRaw, _ := json.Marshal(result)
	if result["question"] != "Is Goreclaw still played?" || result["mode"] != "research" {
		t.Errorf("the report reads %s", resultRaw)
	}
	// The default stance is second-opinion -- not off, which `Resolve(nil,
	// nil)` would have answered.
	stance, _ := result["stance"].(map[string]any)
	if stance["preset"] != "second-opinion" {
		t.Errorf("the stance resolved to %v", stance["preset"])
	}
	body, _ := result["research"].(map[string]any)
	if err := orderedAs(string(doneRaw), []string{"answer", "findings", "cards", "confidence",
		"sources", "sources_dropped", "findings_dropped", "cards_unresolved", "searched"}); err != nil {
		t.Errorf("the body's key order: %v", err)
	}
	// The uncited finding is dropped and counted; the invented source too.
	findings, _ := body["findings"].([]any)
	if len(findings) != 1 || body["findings_dropped"] != float64(1) || body["sources_dropped"] != float64(1) {
		t.Errorf("findings %v dropped %v, sources dropped %v", findings, body["findings_dropped"], body["sources_dropped"])
	}
	// The pool's card carries its text; the spoiled one is labelled, not
	// dropped -- ADR 26's opposite of the dossier.
	cards, _ := body["cards"].([]any)
	if len(cards) != 2 || body["cards_unresolved"] != float64(1) {
		t.Fatalf("cards %v unresolved %v", cards, body["cards_unresolved"])
	}
	first, _ := cards[0].(map[string]any)
	second, _ := cards[1].(map[string]any)
	if first["in_pool"] != true || first["oracle_text"] == nil {
		t.Errorf("the pool's card reads %v", first)
	}
	if second["in_pool"] != false || len(second) != 2 || second["name"] != "Spoiled Card" {
		t.Errorf("the spoiled card reads %v; it is two keys and no more", second)
	}
	if result["never"] != "This is Claude's reading of the cited pages, not the tool's own answer. It has not seen any of your decks." {
		t.Errorf("the promise is %v", result["never"])
	}
}

// A tool the model reaches for that would touch a deck is refused as
// `ToolNotAllowed`, because the mode was never handed one -- and the tools the
// request carried were `get_cards` and the hosted search, nothing else.
func TestResearchCannotReachADeckThroughAnyTool(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("tool_use", `{"type":"tool_use","id":"tu_1","name":"get_deck","input":{"slug":"kaheera"}}`),
		answer("end_turn", searchedPage("t")+","+said(groundedAnswer)),
	}}
	claudeSet := api.start(t)
	rig := newJobRig(t, claudeSet)
	defer rig.close()
	_, payload, _ := callAs(t, rig.api, alice, "POST", researchAt, `{"question":"What does kaheera run?"}`)
	done, _ := rig.await(t, payload["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v", done["status"], done["error"])
	}
	if len(api.requests) != 2 {
		t.Fatalf("%d requests, want 2", len(api.requests))
	}
	tools, _ := api.requests[0]["tools"].([]any)
	names := []string{}
	for _, tool := range tools {
		row, _ := tool.(map[string]any)
		names = append(names, strings.TrimSpace(row["name"].(string)))
	}
	if strings.Join(names, ",") != "get_cards,web_search" {
		t.Errorf("the request offered %v; research has get_cards and the search, nothing else", names)
	}
	sent, _ := json.Marshal(api.requests[1])
	if !strings.Contains(string(sent), "ToolNotAllowed") {
		t.Error("get_deck was not refused; the mode reached a deck")
	}
	if strings.Contains(string(sent), "Sol Ring") {
		t.Error("the deck's cards reached the model")
	}
}

func TestTwoIdenticalQuestionsInFlightAreOneJob(t *testing.T) {
	// Two runs' worth of replies: alice's one job, and bob's own.
	reply := answer("end_turn", searchedPage("t")+","+said(groundedAnswer))
	api := &scriptedClaude{hold: make(chan struct{}), replies: []string{reply, reply}}
	claudeSet := api.start(t)
	rig := newJobRig(t, claudeSet)
	defer rig.close()
	_, first, _ := callAs(t, rig.api, alice, "POST", researchAt, `{"question":"Is Goreclaw still played?"}`)
	_, second, _ := callAs(t, rig.api, alice, "POST", researchAt, `{"question":"  is GORECLAW still   played? "}`)
	// Another seat asking the same words is NOT handed alice's job (ADR 5).
	_, bobs, _ := callAs(t, rig.api, bob, "POST", researchAt, `{"question":"Is Goreclaw still played?"}`)
	close(api.hold)
	if first["id"] != second["id"] {
		t.Errorf("two spellings of one question made two jobs: %v / %v", first["id"], second["id"])
	}
	if bobs["id"] == first["id"] {
		t.Error("bob was handed alice's job")
	}
	_, _ = rig.await(t, first["id"].(string))
	_, _ = rig.awaitAs(t, bob, bobs["id"].(string))
	if api.served != 2 {
		t.Errorf("%d calls for two seats' one question each, want 2", api.served)
	}
}

func TestAFailedResearchCallIsAReadableJobError(t *testing.T) {
	t.Parallel()
	api := &scriptedClaude{replies: []string{"!401"}}
	claudeSet := api.start(t)
	rig := newJobRig(t, claudeSet)
	defer rig.close()
	_, payload, _ := callAs(t, rig.api, alice, "POST", researchAt, `{"question":"Why?"}`)
	done, _ := rig.await(t, payload["id"].(string))
	if done["status"] != "error" || !strings.HasPrefix(done["error"].(string), "the key was rejected (401)") {
		t.Errorf("the job reads %v / %v", done["status"], done["error"])
	}
}

// The label is the plan's: sixty characters of the question, cut in
// code points, right-stripped, an ellipsis when it was cut -- held to the
// corpus the claude package's tests are held to.
func TestTheResearchLabelMatchesTheCorpus(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "claude", "testdata", "research.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Labels []struct {
			Question string `json:"question"`
			Label    string `json:"label"`
		} `json:"labels"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Labels) == 0 {
		t.Fatal("the corpus has no labels")
	}
	for _, row := range corpus.Labels {
		if got := researchLabel(row.Question); got != row.Label {
			t.Errorf("label for %.30q:\n go     %q\n python %q", row.Question, got, row.Label)
		}
	}
}
