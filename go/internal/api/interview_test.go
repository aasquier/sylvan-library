package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

// The interview route's own tests. `internal/claude` already proves the mode --
// the brief's key order, `OnlyQuestions`, the report's fields -- so what these
// prove is the layer above it: which failure lands on which status and in what
// order, that the caller's own library is what the conversation may reach, and
// that the report survives `encoding/json` on the way out.
//
// **That last one is the reason this file is not shorter.** A value can be
// proved exactly right by a corpus and still go onto the wire as something
// else; `tier1.Number` did, having been checked by `repr` and `Float64bits` and
// never once through a marshaller. `InterviewReport` had never been marshalled
// by anything before this route existed.

// scriptedClaude is a fake Anthropic in front of the real `Connect()`. Same
// trick `internal/claude`'s own tests use -- the SDK honours
// ANTHROPIC_BASE_URL, so no production seam exists to be tested instead of the
// real one -- reproduced here rather than shared because a test helper in
// another package is not importable, and because this one wants far less of it.
type scriptedClaude struct {
	replies  []string
	requests []map[string]any
	served   int
}

func (s *scriptedClaude) start(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		s.requests = append(s.requests, parsed)
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
			_, _ = fmt.Sscanf(reply, "!%d", &status)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"nope"}}`))
			return
		}
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)
	t.Setenv("ANTHROPIC_API_KEY", "test-key-not-a-real-one")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("MTGLAB_CLAUDE_MODEL", "")
	t.Setenv("MTGLAB_CLAUDE_STANCE_CEILING", "")
}

// answer is one scripted Message response.
func answer(stop, content string) string {
	return fmt.Sprintf(`{"id":"msg_x","type":"message","role":"assistant",`+
		`"model":"claude-sonnet-5","content":[%s],"stop_reason":%q,`+
		`"stop_sequence":null,"container":null,"usage":{"input_tokens":11,`+
		`"output_tokens":22,"cache_read_input_tokens":33,`+
		`"cache_creation_input_tokens":0}}`, content, stop)
}

func said(s string) string { return fmt.Sprintf(`{"type":"text","text":%q}`, s) }

// noCredential is an instance nobody has given a key to, which is CI and is
// what the contract suite's own interview case asserts against. Set explicitly
// because this machine's shell may well have one.
func noCredential(t *testing.T) {
	t.Helper()
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("MTGLAB_CLAUDE_STANCE_CEILING", "")
}

// deckAPIWithLog is deckAPI with somewhere to read the logs from, which one
// test needs because the thing it asserts is what the operator sees rather
// than what the caller gets.
func deckAPIWithLog(t *testing.T, into *bytes.Buffer) (*API, func()) {
	t.Helper()
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	a := New(Config{
		Logger:   slog.New(slog.NewTextHandler(into, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Pool:     pooltest.Open(t),
		DecksDir: decksDir(t), AdminEmail: "alice@example.com", AppDB: db,
	})
	return a, func() { db.Close() }
}

const kaheera = "/api/decks/alice/kaheera/interview"

// ---- what is refused, and in what order ---------------------------------

func TestTheInterviewNeedsACard(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	for _, body := range []string{`{}`, `{"card":""}`, `{"card":"   "}`} {
		status, payload, raw := callAs(t, a, alice, "POST", kaheera, body)
		if status != 422 {
			t.Fatalf("body %s answered %d %s", body, status, raw)
		}
		if payload["detail"] != "card is required" {
			t.Errorf("body %s said %v, want `card is required`", body, payload["detail"])
		}
	}
}

// A null card is NOT a missing card, which is the one input where
// `str(payload.get("card", ""))` and `str(payload.get("card") or "")` disagree.
// Python reaches `str(None)` and asks the deck about a card called "None"; the
// answer is still a 422, with a different sentence, and the sentence is the
// contract.
func TestANullCardIsThePythonStringNone(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", kaheera, `{"card":null}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	detail, _ := payload["detail"].(string)
	if !strings.Contains(detail, "'None' is not in kaheera") {
		t.Errorf("a null card answered %q; Python asks about the four-letter "+
			"string `None` and refuses it as a card the deck does not run", detail)
	}
}

func TestTheInterviewRefusesACardTheDeckDoesNotRun(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", kaheera, `{"card":"Black Lotus"}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	detail, _ := payload["detail"].(string)
	if !strings.Contains(detail, "Black Lotus") || !strings.Contains(detail, "kaheera") {
		t.Errorf("the refusal was %q; it must name the card and the deck", detail)
	}
}

// A stance that will not parse is a 502, and that is Python's answer rather
// than the right one.
//
// `api/app.py` carries an `except ValueError` branch commented "A malformed
// stance" that raises 422 -- and it is unreachable, because
// `service.claude_interview` re-raises only three exception types and a stance
// ValueError is not among them, so the broad `except Exception` has already
// turned it into `ClaudeFailed`. **Measured against the running pair**: Python
// answers 502. The port reproduces warts; it does not quietly improve on them,
// because a flip that changes behaviour is not a flip.
//
// What IS asserted as correct is the sentence: the parser's own words, so a
// person reads "'emperor' is not a stance preset" either way.
func TestAMalformedStanceIsPythons502(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", kaheera,
		`{"card":"Sol Ring","stance":"emperor"}`)
	if status != 502 {
		t.Fatalf("%d %s -- Python answers 502 here, measured", status, raw)
	}
	detail, _ := payload["detail"].(string)
	if !strings.Contains(detail, "'emperor' is not a stance preset") {
		t.Errorf("the refusal was %q; it must be the parser's own sentence", detail)
	}
}

// The deck is resolved through `Library` like every other per-deck route, so a
// deck bob cannot see is a 404 and never a 403 (ADR 5).
func TestTheInterviewIs404ForADeckTheCallerCannotSee(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, _, raw := callAs(t, a, bob, "POST", "/api/decks/bob/kaheera/interview",
		`{"card":"Sol Ring"}`)
	if status != 404 {
		t.Fatalf("%d %s", status, raw)
	}
}

// The body is read before the deck, which is FastAPI's order: `payload` is
// validated while the dependencies are being solved and `lib.source_for(owner)`
// is not reached until the handler's first line.
func TestAMissingCardIsAnsweredBeforeAMissingDeck(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST",
		"/api/decks/alice/no-such-deck/interview", `{}`)
	if status != 422 {
		t.Fatalf("%d %s -- the body is refused before the deck is looked up", status, raw)
	}
	if payload["detail"] != "card is required" {
		t.Errorf("said %v, want the body's refusal", payload["detail"])
	}
}

// No key is 503 and not 502: no call was made at all. This is the contract
// suite's own interview case, which runs on an instance with no credential.
//
// **The sentence is asserted whole**, and that is not pedantry: it shipped
// wrong once. `Require` wrapped the reason with `fmt.Errorf("%w: ...",
// ErrUnavailable)`, so the door answered "claude is unavailable: no
// ANTHROPIC_API_KEY ..." where Python's `str(ClaudeUnavailable(...))` is the
// bare reason -- a prefix nobody wrote, rendered verbatim by the deck page.
// A `strings.Contains(detail, "ANTHROPIC_API_KEY")` check passed both
// spellings, which is exactly how it survived to be found by curling the
// running pair instead.
func TestWithNoKeyTheInterviewIs503(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", kaheera, `{"card":"Sol Ring"}`)
	if status != 503 {
		t.Fatalf("%d %s", status, raw)
	}
	const want = "no ANTHROPIC_API_KEY in the environment -- put one in .env " +
		"(see .env.example), or `fly secrets set` it when deployed"
	if detail, _ := payload["detail"].(string); detail != want {
		t.Errorf("the refusal is\n  %q\nPython's is\n  %q", detail, want)
	}
}

// ---- what is answered ----------------------------------------------------

// At `initiative: off` no call is made and the report says so -- 200, with a
// reason, on an instance with no key at all. Not an empty question list that
// looks like the model had nothing to say.
func TestAtStanceOffNoCallIsMadeAndTheReportSaysSo(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	status, payload, raw := callAs(t, a, alice, "POST", kaheera,
		`{"card":"Sol Ring","stance":"off"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["asked"] != false {
		t.Errorf("asked is %v, want false", payload["asked"])
	}
	if reason, _ := payload["reason"].(string); !strings.Contains(reason, "stance is off") {
		t.Errorf("reason is %q", reason)
	}
	if qs, _ := payload["questions"].([]any); qs == nil || len(qs) != 0 {
		t.Errorf("questions is %v, want an empty list rather than null", payload["questions"])
	}
}

// The whole report, through the marshaller, in Python's key order.
func TestTheReportReachesTheWireInPythonsOrder(t *testing.T) {
	api := &scriptedClaude{replies: []string{answer("end_turn", said(
		`{"questions":[{"question":"What does it accelerate into?","angle":"curve","fact":"ramp holds 1 slot"},`+
			`{"question":"Would a Signet do?","angle":"alternatives","fact":"colour identity is G"},`+
			`{"not a question":true,"question":"It is simply good.","angle":"a","fact":"b"}]}`))}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	status, payload, raw := callAs(t, a, alice, "POST", kaheera, `{"card":"Sol Ring"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	// Field order is Python's dict order, and the goldens hold it.
	want := []string{"answered_by", "mode", "model", "slug", "card", "asked",
		"reason", "stance", "questions", "questions_dropped", "tool_calls",
		"usage", "never"}
	if err := orderedAs(string(raw), want); err != nil {
		t.Errorf("%v\n%s", err, raw)
	}
	if payload["answered_by"] != "claude" {
		t.Errorf("answered_by is %v -- ADR 14's third boundary is a field", payload["answered_by"])
	}
	if payload["card"] != "Sol Ring" || payload["slug"] != "kaheera" {
		t.Errorf("card/slug are %v / %v", payload["card"], payload["slug"])
	}
	if payload["model"] != "claude-sonnet-5" {
		t.Errorf("model is %v", payload["model"])
	}
	// The declarative third item is dropped and COUNTED. "It dropped two" is
	// how a prompt that has started editorialising becomes visible.
	qs, _ := payload["questions"].([]any)
	if len(qs) != 2 {
		t.Fatalf("%d questions survived, want 2: %v", len(qs), payload["questions"])
	}
	if payload["questions_dropped"] != float64(1) {
		t.Errorf("questions_dropped is %v, want 1", payload["questions_dropped"])
	}
	first, _ := qs[0].(map[string]any)
	if first["question"] != "What does it accelerate into?" || first["angle"] != "curve" ||
		first["fact"] != "ramp holds 1 slot" {
		t.Errorf("the first question came through as %v", first)
	}
	usage, _ := payload["usage"].(map[string]any)
	if usage["input_tokens"] != float64(11) || usage["output_tokens"] != float64(22) ||
		usage["cache_read_tokens"] != float64(33) {
		t.Errorf("usage is %v -- the cache figure is the evidence the prefix holds", usage)
	}
	if payload["never"] != "These are questions. The rationale is yours to write." {
		t.Errorf("the promise is %v", payload["never"])
	}
	if payload["tool_calls"] == nil {
		t.Error("tool_calls marshalled as null; Python sends a list")
	}
}

// The brief goes into the opening message, so what the model is handed has to
// be the deck the route resolved -- not a deck the mode went and found.
func TestTheModelIsHandedTheDecksOwnFacts(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"questions":[]}`))}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	if status, _, raw := callAs(t, a, alice, "POST", kaheera, `{"card":"Sol Ring","focus":"is it too slow"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if len(api.requests) != 1 {
		t.Fatalf("%d requests, want 1 -- the brief means no tool call is needed", len(api.requests))
	}
	sent, err := json.Marshal(api.requests[0])
	if err != nil {
		t.Fatal(err)
	}
	body := string(sent)
	for _, want := range []string{
		"Sol Ring",                      // the card under discussion
		"Goreclaw, Terror of Qal Sisma", // the deck's own commander
		"rationale_so_far",              // named for what it is
		"Two mana on turn one",          // the rationale it already carries
		"is it too slow",                // the user's steer, quoted
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the opening message did not carry %q", want)
		}
	}
	// The brief is serialised into the prompt, so a nested block rendered as
	// `[{"Key":..,"Value":..}]` is valid JSON and nonsense to the model.
	if strings.Contains(body, `"Key":`) || strings.Contains(body, `"Value":`) {
		t.Error("the brief went out as wire.KV structs rather than as an object")
	}
}

// A tool the conversation reaches for sees exactly the library the caller does
// -- ADR 22 holding one hop further in than the route.
//
// **The tool has to be one this mode actually offers**, which is the whole
// reason this test is written the way it is. It drove `list_decks` first and
// survived the mutation that hands the tools no source at all: the
// rationale interview's set is `get_cards`, `get_deck`, `deck_stats` and
// `validate_deck`, so `converse` refused the call as a tool the mode never
// offered and the source was never consulted. A test that drives the wrong
// mechanism passes for a reason that has nothing to do with what it claims.
//
// `get_deck` is the pick because its result carries something the brief does
// not: the brief holds the card under discussion and its **category** siblings,
// so a threat in a deck being interviewed about a ramp slot appears only if a
// real deck read actually happened.
func TestTheToolsSeeTheCallersOwnLibrary(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("tool_use", `{"type":"tool_use","id":"tu_1","name":"get_deck","input":{"slug":"kaheera"}}`),
		answer("end_turn", said(`{"questions":[]}`)),
	}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	// bob reading alice's shared deck: the tool must answer over alice's
	// source, which is the one the route resolved.
	if status, _, raw := callAs(t, a, bob, "POST", "/api/decks/alice/kaheera/interview",
		`{"card":"Sol Ring"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if len(api.requests) != 2 {
		t.Fatalf("%d requests, want 2", len(api.requests))
	}
	sent, err := json.Marshal(api.requests[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(sent), "no deck library is reachable") {
		t.Fatal("the tool was handed no source; the route must pass the caller's own")
	}
	// Not in the brief -- Vorinclex is a threat and the interview is about a
	// ramp slot -- so its presence is a real deck read and nothing else.
	if !strings.Contains(string(sent), "Vorinclex") {
		t.Error("get_deck did not answer over the caller's library")
	}
}

// An empty stance is not a stance: `payload.get("stance") or None` reads a
// falsy value as "none was asked for", so the deck's own default answers. A
// route that passed it through would refuse an empty form field as a typo.
func TestAFalsyStanceFallsBackToTheDecksDefault(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	for _, body := range []string{
		`{"card":"Sol Ring","stance":""}`,
		`{"card":"Sol Ring","stance":{}}`,
		`{"card":"Sol Ring","stance":null}`,
	} {
		// 503 and not 422: the stance resolved, and then there was no key.
		status, _, raw := callAs(t, a, alice, "POST", kaheera, body)
		if status != 503 {
			t.Errorf("body %s answered %d %s, want 503 -- an empty stance is "+
				"the deck's default, not a refusal", body, status, raw)
		}
	}
}

// A stance may arrive as an object of axes rather than a preset name, which
// is a path the stance corpus exercises against `json.RawMessage` and that
// nothing had ever driven through a route: `readBody` decodes to
// `map[string]any`, a different arm of `StanceFromObj` entirely.
func TestAStanceMayArriveAsAnObjectOfAxes(t *testing.T) {
	noCredential(t)
	a, done := deckAPI(t, true)
	defer done()
	// Valid axes: resolved, then no key -- 503, not 422.
	status, _, raw := callAs(t, a, alice, "POST", kaheera,
		`{"card":"Sol Ring","stance":{"initiative":"on-request","scope":"flagged","write":"none"}}`)
	if status != 503 {
		t.Fatalf("a well-formed stance object answered %d %s", status, raw)
	}
	// An axis that is not one takes the same path as a bad preset, so it is
	// Python's 502 as well -- and the sentence carries Python's list repr
	// rather than Go's.
	status, payload, raw := callAs(t, a, alice, "POST", kaheera,
		`{"card":"Sol Ring","stance":{"vibe":"on-request"}}`)
	if status != 502 {
		t.Fatalf("an unknown axis answered %d %s", status, raw)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "['vibe'] are not stance axes") {
		t.Errorf("the refusal was %q; it must carry Python's own list repr", detail)
	}
}

// Which Claude answers is a fact about the caller, read fresh off the request's
// scope. It reached `auth.Scope` with this route: every earlier ported family
// had no use for it, and the struct's comment said so.
func TestATieredSeatIsAskedOfItsOwnModel(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"questions":[]}`))}}
	api.start(t)
	a, done := deckAPI(t, true)
	defer done()

	seated := alice
	seated.ModelTier = "opus"
	if status, _, raw := callAs(t, a, seated, "POST", kaheera, `{"card":"Sol Ring"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	want := tiers.Resolve("opus")
	if got := api.requests[0]["model"]; got != want {
		t.Errorf("the tiered seat was answered by %v, want %q -- the tier is "+
			"read off the scope, not off the house default", got, want)
	}
	if want == claude.Model {
		t.Fatal("this test proves nothing while the opus tier resolves to the house model")
	}
}

// A client's typo is not an incident.
//
// The stance branch and the default branch both answer 502 while the wart
// above stands, so `ErrStanceRejected` currently changes exactly one thing a
// test can observe -- and this is it. A malformed stance is somebody
// mistyping into a form; a failed call is the instance having a bad day, and
// only the second belongs in the error log. Without this the sentinel is an
// equivalent mutant, which a mutation run said out loud.
//
// It also keeps the branch honest for the day the wart is fixed: whoever
// changes that 502 to a 422 finds a test already standing here saying what
// the branch is for.
func TestAClientsTypoIsNotLoggedAsAFailure(t *testing.T) {
	noCredential(t)
	var logged bytes.Buffer
	a, done := deckAPIWithLog(t, &logged)
	defer done()

	if status, _, raw := callAs(t, a, alice, "POST", kaheera,
		`{"card":"Sol Ring","stance":"emperor"}`); status != 502 {
		t.Fatalf("%d %s", status, raw)
	}
	if strings.Contains(logged.String(), "the Claude route failed") {
		t.Errorf("a mistyped stance was logged as a route failure:\n%s", logged.String())
	}

	// A call that actually failed is logged, so the absence above means
	// something. Same status to the caller, different thing to the operator.
	logged.Reset()
	// Several, because the SDK retries a 5xx before giving up; the script
	// running out is itself a test failure, so this cannot pass by accident.
	api := &scriptedClaude{replies: []string{"!500", "!500", "!500", "!500", "!500"}}
	api.start(t)
	b, doneB := deckAPIWithLog(t, &logged)
	defer doneB()
	if status, _, raw := callAs(t, b, alice, "POST", kaheera, `{"card":"Sol Ring"}`); status != 502 {
		t.Fatalf("%d %s", status, raw)
	}
	if !strings.Contains(logged.String(), "the Claude route failed") {
		t.Errorf("a genuine SDK failure was not logged:\n%s", logged.String())
	}
}

// ---- the accounting ------------------------------------------------------

// Every conversation lands in the ledger, and a conversation that never
// happened does not.
func TestTheInterviewRecordsWhatItSpent(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", said(`{"questions":[]}`))}}
	api.start(t)

	rig := newWriteRig(t)
	if status, _, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/kaheera/interview", `{"card":"Sol Ring"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	mode, model, in, out := oneClaudeRow(t, rig)
	if mode != "rationale-interview" {
		t.Errorf("the row is for mode %q", mode)
	}
	if model != "claude-sonnet-5" {
		t.Errorf("the row names model %q", model)
	}
	if in != 11 || out != 22 {
		t.Errorf("the row counted %d in / %d out, want 11 / 22", in, out)
	}
}

func TestAConversationThatNeverHappenedIsNotRecorded(t *testing.T) {
	noCredential(t)
	rig := newWriteRig(t)
	if status, _, raw := callAs(t, rig.api, alice, "POST",
		"/api/decks/alice/kaheera/interview", `{"card":"Sol Ring","stance":"off"}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	var rows int
	if err := rig.recorder.DB().QueryRow("SELECT COUNT(*) FROM claude_usage").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d ledger rows for a stance that made no call", rows)
	}
}

// ---- helpers -------------------------------------------------------------

// orderedAs checks that `keys` appear in the JSON text in the order given.
// Key order is part of the payload here -- the deck page renders some of it
// unsorted, and a Go map alphabetises where a Python dict does not.
func orderedAs(body string, keys []string) error {
	at := 0
	for _, key := range keys {
		idx := strings.Index(body[at:], `"`+key+`":`)
		if idx < 0 {
			return fmt.Errorf("key %q is missing or out of order after %q", key, keys)
		}
		at += idx + 1
	}
	return nil
}

func oneClaudeRow(t *testing.T, rig *writeRig) (mode, model string, in, out int) {
	t.Helper()
	rows, err := rig.recorder.DB().Query(
		"SELECT mode, model, input_tokens, output_tokens FROM claude_usage")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := 0
	for rows.Next() {
		if err := rows.Scan(&mode, &model, &in, &out); err != nil {
			t.Fatal(err)
		}
		found++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if found != 1 {
		t.Fatalf("%d ledger rows, want exactly 1", found)
	}
	return mode, model, in, out
}
