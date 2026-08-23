package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// The theme interview's two routes. `internal/claude` holds both halves to a
// corpus; what these prove is the layer above -- the four refusals and their
// statuses, the job born finished, the job whose result is the report, the
// labels, the deliberate absence of a dedupe -- and the two structural claims
// worth proving end to end: neither mode is offered a tool that could reach a
// deck, and neither handler is given a deck source to offer one from.

const (
	themeAt         = "/api/claude/theme"
	themeProposalAt = "/api/claude/theme/proposal"
)

// A transcript that reaches the floor, and slots quoting it. Three grounded
// kinds is what `may_propose` counts, so this is the shortest conversation
// the proposal will accept.
const readyTranscript = `[{"role":"assistant","text":"What do you love?"},` +
	`{"role":"user","text":"old horror films"},` +
	`{"role":"assistant","text":"And under pressure?"},` +
	`{"role":"user","text":"I improvise"},` +
	`{"role":"assistant","text":"At game night?"},` +
	`{"role":"user","text":"I make deals"}]`

const readySlots = `[{"kind":"taste","value":"horror","quote":"old horror films"},` +
	`{"kind":"temperament","value":"improviser","quote":"I improvise"},` +
	`{"kind":"posture","value":"deals","quote":"I make deals"}]`

const themeTurnAnswer = `{"question":"What did the practical effects give you?",` +
	`"fact":{"text":"White wants peace.","source":"taxonomy"},` +
	`"slots":[{"kind":"taste","value":"horror","quote":"old horror films"}]}`

const themeProposalAnswer = `{"combinations":[` +
	`{"key":"BG","reading":"Golgari.","grounding":"You said you improvise.",` +
	`"source_ids":["s1"],"commanders":[{"card":"Gyome, Master Chef",` +
	`"prose":"A troll chef.","source_ids":["s1"]}]},` +
	`{"key":"G","reading":"Mono-green.","grounding":"You said you make deals.",` +
	`"source_ids":[],"commanders":[{"card":"Craterhoof Behemoth","prose":"Wide."}]}],` +
	`"sources":[{"id":"s1","title":"t","url":"https://edhrec.com/real"},` +
	`{"id":"s2","title":"t","url":"https://example.com/invented"}]}`

// The four things each half refuses before anything is spent, and the three
// statuses between them. **409 is the one no other Claude route has**: a floor
// not yet reached is not a malformed request, and telling somebody who sent
// exactly the right thing too early that they sent something wrong is the
// commandment-2 failure in a status code.
func TestTheThemeRoutesRefuseWhatTheyCanBeforeAnyJob(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	for _, row := range []struct {
		at     string
		body   string
		status int
		detail string
	}{
		{themeAt, `{"transcript":"not a list"}`, 422, "transcript must be a list of turns"},
		{themeAt, `{"transcript":[{"role":"system","text":"hi"}]}`, 422, "only 'user' and 'assistant'"},
		{themeAt, `{"transcript":[{"role":"user","text":"hi"}]}`, 422, "starts with the interviewer's own"},
		{themeAt, `{"persona":"not-a-voice"}`, 422, "no persona 'not-a-voice'"},
		{themeAt, `{"persona":"fortune-teller","seed":"soon"}`, 422, "not a usable reading seed"},
		{themeAt, `{"stance":"emperor"}`, 422, "is not a stance preset"},
		{themeAt, `{"facts":["one",2]}`, 422, "fact 1 is not a string"},
		// A real turn and no key: 503 from the request, never a job.
		{themeAt, `{}`, 503, "no ANTHROPIC_API_KEY"},

		// The proposal's own, and the floor is the first thing it asks.
		{themeProposalAt, `{"transcript":` + readyTranscript + `}`, 409,
			"0 of 3 things are known about this person"},
		{themeProposalAt, `{"transcript":` + readyTranscript +
			`,"slots":[{"kind":"taste","value":"v","quote":"never typed this"}]}`, 409,
			"0 of 3 things are known"},
		{themeProposalAt, `{"transcript":"not a list","slots":` + readySlots + `}`, 422,
			"transcript must be a list of turns"},
		{themeProposalAt, `{"transcript":` + readyTranscript + `,"slots":` + readySlots +
			`,"persona":"not-a-voice"}`, 422, "no persona 'not-a-voice'"},
		{themeProposalAt, `{"transcript":` + readyTranscript + `,"slots":` + readySlots +
			`,"budget":"fifty"}`, 422, "could not convert string to float: 'fifty'"},
		{themeProposalAt, `{"transcript":` + readyTranscript + `,"slots":` + readySlots + `}`,
			503, "no ANTHROPIC_API_KEY"},
	} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", row.at, row.body)
		if status != row.status {
			t.Errorf("%s %.70s answered %d, want %d: %s", row.at, row.body, status, row.status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, row.detail) {
			t.Errorf("%s %.70s said %q, want it to contain %q", row.at, row.body, detail, row.detail)
		}
	}
	if got := rig.jobs.All(alice.UserID); len(got) != 0 {
		t.Errorf("%d jobs were queued by refused requests", len(got))
	}
}

// The floor is refused **before** the missing key is, which is the order that
// helps: "keep talking" is actionable and "there is no key" is not the
// caller's problem to solve.
func TestTheFloorIsRefusedBeforeTheMissingKey(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "POST", themeProposalAt,
		`{"transcript":`+readyTranscript+`}`)
	if status != 409 {
		t.Fatalf("%d %s", status, raw)
	}
	if detail, _ := payload["detail"].(string); strings.Contains(detail, "ANTHROPIC") {
		t.Errorf("the key was checked before the floor: %q", detail)
	}
}

// `float()`'s TypeError is not in the route's `except` list, so a list budget
// is a 500 in Python. Reproduced rather than tidied into the 422 beside it: a
// flip that changes behaviour is not a flip.
func TestAListBudgetIsPythonsUncaughtTypeError(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	status, _, raw := callAs(t, rig.api, alice, "POST", themeProposalAt,
		`{"transcript":`+readyTranscript+`,"slots":`+readySlots+`,"budget":[50]}`)
	if status != 500 {
		t.Errorf("a list budget answered %d, want 500: %s", status, raw)
	}
}

// Two ways a turn reaches nobody, both jobs born finished -- so the common
// cheap case costs exactly one request and never a poll.
func TestATurnThatReachesNobodyIsAJobBornFinished(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	for _, row := range []struct {
		note, body, reason string
	}{
		{"stance off", `{"stance":"off","slots":` + readySlots +
			`,"transcript":` + readyTranscript + `}`, "stance is off"},
		{"past the ceiling", `{"slots":` + readySlots + `,"transcript":` +
			exhaustedTranscript() + `}`, "as long as this conversation goes"},
	} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", themeAt, row.body)
		if status != 200 || payload["status"] != "done" || payload["kind"] != ThemeAskKind {
			t.Fatalf("%s: %d %s", row.note, status, raw)
		}
		if payload["label"] != "theme: a question, from 3 things known" {
			t.Errorf("%s: label is %v", row.note, payload["label"])
		}
		result, _ := payload["result"].(map[string]any)
		if result["asked"] != false {
			t.Errorf("%s: a call was made", row.note)
		}
		if reason, _ := result["reason"].(string); !strings.Contains(reason, row.reason) {
			t.Errorf("%s: reason is %q", row.note, reason)
		}
		// The readiness the client carried survives a turn that made no call.
		if result["grounded"] != float64(3) || result["may_propose"] != true {
			t.Errorf("%s: the reading was lost: %v", row.note, result)
		}
	}
}

// A stance of `off` on the proposal, and its label -- the singular is
// `” if n == 1 else 's'`, which puts "1 thing" against "3 things".
func TestTheProposalLabelCountsThingsKnown(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	_, payload, raw := callAs(t, rig.api, alice, "POST", themeProposalAt,
		`{"stance":"off","slots":`+readySlots+`,"transcript":`+readyTranscript+`}`)
	if payload["kind"] != ThemeProposalKind || payload["status"] != "done" {
		t.Fatalf("%s", raw)
	}
	if payload["label"] != "theme: colours and commanders, from 3 things known" {
		t.Errorf("label is %v", payload["label"])
	}
}

// A whole conversation turn as a job, with the report's key order asserted in
// the marshalled bytes -- `never` is the last key and appears only because a
// call was made.
func TestTheThemeTurnIsAJobWhoseResultIsTheReport(t *testing.T) {
	api := &scriptedClaude{replies: []string{answer("end_turn", said(themeTurnAnswer))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "POST", themeAt,
		`{"transcript":`+readyTranscript+`}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	done, doneRaw := rig.await(t, payload["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v", done["status"], done["error"])
	}
	if err := orderedAs(string(doneRaw), []string{"answered_by", "mode", "model",
		"stance", "usage", "persona", "asked", "question", "fact", "facts_dropped",
		"slots", "slots_dropped", "grounded", "floor", "may_propose", "exchanges",
		"max_exchanges", "reason", "never"}); err != nil {
		t.Errorf("%v\n%s", err, doneRaw)
	}
	result, _ := done["result"].(map[string]any)
	if result["mode"] != "theme-conversation" || result["persona"] != "plain" {
		t.Errorf("the report reads %v", result)
	}
	// The default stance here is second-opinion, never off -- the surface runs
	// before a deck exists and `Resolve(nil, nil)` would have said off.
	stance, _ := result["stance"].(map[string]any)
	if stance["preset"] != "second-opinion" {
		t.Errorf("the stance resolved to %v", stance["preset"])
	}
	if result["question"] != "What did the practical effects give you?" {
		t.Errorf("the question is %v", result["question"])
	}
	// One slot came back and three were carried; the count is what `carry`
	// holds up, not what this turn happened to mention.
	if result["grounded"] != float64(1) || result["may_propose"] != false {
		t.Errorf("grounded %v may_propose %v", result["grounded"], result["may_propose"])
	}
	if result["exchanges"] != float64(3) || result["floor"] != float64(3) {
		t.Errorf("exchanges %v floor %v", result["exchanges"], result["floor"])
	}
}

// A whole proposal, and the three checks that make it ADR 20's rather than a
// paragraph: the invented source is dropped and counted, the mono-green legend
// cannot lead a Golgari slot, and every commander shown carries pool text.
func TestTheThemeProposalIsAJobWhoseResultIsTheProposal(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("The Real Page")+","+said(themeProposalAnswer))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "POST", themeProposalAt,
		`{"transcript":`+readyTranscript+`,"slots":`+readySlots+`,"budget":50}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	done, doneRaw := rig.await(t, payload["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v", done["status"], done["error"])
	}
	if err := orderedAs(string(doneRaw), []string{"answered_by", "mode", "model",
		"stance", "usage", "persona", "asked", "combinations", "sources",
		"sources_dropped", "commanders_dropped", "combinations_dropped", "searched",
		"slots", "slots_dropped", "reason", "never"}); err != nil {
		t.Errorf("%v\n%s", err, doneRaw)
	}
	result, _ := done["result"].(map[string]any)
	if result["sources_dropped"] != float64(1) {
		t.Errorf("the invented source was not dropped: %v", result["sources_dropped"])
	}
	combos, _ := result["combinations"].([]any)
	if len(combos) != 2 {
		t.Fatalf("%d combinations: %s", len(combos), doneRaw)
	}
	first, _ := combos[0].(map[string]any)
	if first["key"] != "BG" || first["name"] == "" || first["tagline"] == "" {
		t.Errorf("the combination is %v", first)
	}
	// `source_ids` keeps only the citation that survived checking.
	ids, _ := first["source_ids"].([]any)
	if len(ids) != 1 || ids[0] != "s1" {
		t.Errorf("source_ids is %v", ids)
	}
	commanders, _ := first["commanders"].([]any)
	if len(commanders) != 1 {
		t.Fatalf("%d commanders", len(commanders))
	}
	cmd, _ := commanders[0].(map[string]any)
	if cmd["name"] != "Gyome, Master Chef" || cmd["oracle_text"] == nil {
		t.Errorf("the commander is %v", cmd)
	}
	// Prose and grounding stay apart all the way to the page: one of them can
	// be wrong and the other cannot.
	if first["reading"] == first["grounding"] {
		t.Error("the reading and its grounding have been merged")
	}
	if result["never"] == nil || !strings.Contains(result["never"].(string), "the pool's") {
		t.Errorf("the promise is %v", result["never"])
	}
}

// The budget rides in the closing instruction as `$50`, not `$50.000000` --
// Python's `:g`, which Go's default `%g` is not.
func TestTheBudgetReachesTheModelInPythonsFormat(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("t")+","+said(themeProposalAnswer))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	_, payload, _ := callAs(t, rig.api, alice, "POST", themeProposalAt,
		`{"transcript":`+readyTranscript+`,"slots":`+readySlots+
			`,"budget":50,"avoid":"  no blue  "}`)
	done, _ := rig.await(t, payload["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v", done["status"], done["error"])
	}
	sent, _ := json.Marshal(api.requests[0])
	if !strings.Contains(string(sent), `They have about $50 for the whole deck`) {
		t.Error("the budget did not reach the model as Python spells it")
	}
	// `avoid` is stripped, and it is the user's words rather than an
	// instruction this side composed.
	if !strings.Contains(string(sent), `steer away from, in their words: no blue`) {
		t.Error("what to avoid did not reach the model, or was not stripped")
	}
}

// **Two turns in flight are two conversations, not one question asked twice.**
// The opposite call from research's dedupe, and the reason is that the
// transcript is client-held: a second tab is a second person's evening, and
// collapsing them would hand one of them the other's question.
func TestTwoIdenticalThemeTurnsAreTwoJobs(t *testing.T) {
	reply := answer("end_turn", said(themeTurnAnswer))
	api := &scriptedClaude{hold: make(chan struct{}), replies: []string{reply, reply}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	body := `{"transcript":` + readyTranscript + `}`
	_, first, _ := callAs(t, rig.api, alice, "POST", themeAt, body)
	_, second, _ := callAs(t, rig.api, alice, "POST", themeAt, body)
	close(api.hold)
	if first["id"] == second["id"] {
		t.Error("two conversations were collapsed into one job")
	}
	_, _ = rig.await(t, first["id"].(string))
	_, _ = rig.await(t, second["id"].(string))
	if api.served != 2 {
		t.Errorf("%d calls for two turns, want 2", api.served)
	}
}

// ADR 20's first decision, end to end: neither mode is offered a tool that
// could reach a deck, and the conversation half is offered no client tool at
// all.
func TestNeitherThemeModeIsOfferedADeckTool(t *testing.T) {
	for _, row := range []struct {
		at, body, reply string
		want            string
	}{
		{themeAt, `{"transcript":` + readyTranscript + `}`, said(themeTurnAnswer), "web_search"},
		{themeProposalAt, `{"transcript":` + readyTranscript + `,"slots":` + readySlots + `}`,
			searchedPage("t") + "," + said(themeProposalAnswer), "get_cards,search_cards,web_search"},
	} {
		api := &scriptedClaude{replies: []string{answer("end_turn", row.reply)}}
		api.start(t)
		rig := newJobRig(t)
		_, payload, _ := callAs(t, rig.api, alice, "POST", row.at, row.body)
		done, _ := rig.await(t, payload["id"].(string))
		if done["status"] != "done" {
			t.Fatalf("%s: the job ended %v: %v", row.at, done["status"], done["error"])
		}
		tools, _ := api.requests[0]["tools"].([]any)
		names := []string{}
		for _, tool := range tools {
			entry, _ := tool.(map[string]any)
			names = append(names, strings.TrimSpace(entry["name"].(string)))
		}
		got := strings.Join(names, ",")
		if got != row.want {
			t.Errorf("%s offered %v, want %v", row.at, got, row.want)
		}
		for _, banned := range []string{"get_deck", "deck_", "suggest", "validate"} {
			if strings.Contains(got, banned) {
				t.Errorf("%s offers %s; a theme mode that can reach a deck is "+
					"the deck conversation ADR 20 leaves unbuilt", row.at, banned)
			}
		}
		rig.close()
	}
}

// A failed call is a job in state `error`, in the words `explain` gives it --
// not a 502, because by then the response has been sent.
func TestAFailedThemeCallIsAReadableJobError(t *testing.T) {
	api := &scriptedClaude{replies: []string{"!401"}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	_, payload, _ := callAs(t, rig.api, alice, "POST", themeAt,
		`{"transcript":`+readyTranscript+`}`)
	done, _ := rig.await(t, payload["id"].(string))
	if done["status"] != "error" ||
		!strings.HasPrefix(done["error"].(string), "the key was rejected (401)") {
		t.Errorf("the job reads %v / %v", done["status"], done["error"])
	}
}

// exhaustedTranscript is a conversation that has already gone as long as it
// goes: MAX_EXCHANGES user turns, opened by the interviewer.
//
// **Built on `readyTranscript` rather than out of filler**, and the first
// draft of this was filler -- which correctly lost all three slots, because
// `Ground` re-checks every quote against the turns and a client is not the
// authority on what its user said. The padding goes after the real answers so
// the quotes are still there to find.
func exhaustedTranscript() string {
	turns := []string{
		`{"role":"assistant","text":"What do you love?"}`,
		`{"role":"user","text":"old horror films"}`,
		`{"role":"assistant","text":"And under pressure?"}`,
		`{"role":"user","text":"I improvise"}`,
		`{"role":"assistant","text":"At game night?"}`,
		`{"role":"user","text":"I make deals"}`,
	}
	for i := 0; i < 7; i++ {
		turns = append(turns, `{"role":"assistant","text":"q"}`,
			`{"role":"user","text":"a"}`)
	}
	return "[" + strings.Join(turns, ",") + "]"
}

// The label's singular is `” if n == 1 else 's'`, which is the conditional
// the other way round from how it reads: **zero things are "0 things"** and
// only one is "1 thing". Nothing above reached either edge, so a mutation
// that spelled it `n == 0` survived a whole suite.
func TestTheLabelsCountThingsPythonsWay(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	for _, row := range []struct{ slots, want string }{
		{`[]`, "theme: a question, from 0 things known"},
		{`[{"kind":"taste","value":"horror","quote":"old horror films"}]`,
			"theme: a question, from 1 thing known"},
		{readySlots, "theme: a question, from 3 things known"},
	} {
		_, payload, raw := callAs(t, rig.api, alice, "POST", themeAt,
			`{"stance":"off","transcript":`+readyTranscript+`,"slots":`+row.slots+`}`)
		if payload["label"] != row.want {
			t.Errorf("label is %v, want %q: %s", payload["label"], row.want, raw)
		}
	}
	// And the proposal's own, at its floor -- three is the fewest it accepts,
	// so "1 thing" is unreachable there and the singular is the ask's alone.
	_, payload, _ := callAs(t, rig.api, alice, "POST", themeProposalAt,
		`{"stance":"off","transcript":`+readyTranscript+`,"slots":`+readySlots+`}`)
	if payload["label"] != "theme: colours and commanders, from 3 things known" {
		t.Errorf("the proposal label is %v", payload["label"])
	}
}

// **The job's context is its own, and only a real server can prove it.**
//
// `net/http` cancels a request's context the moment the handler returns, and
// the handler returns as soon as the job id is written -- so a worker holding
// `r.Context()` is cancelled before it speaks to anybody. The sim cache stored
// nothing at all from v183 because of exactly this, and the recorder every
// other test in this file uses never cancels, so none of them can see it.
//
// The dossier proves it by a store write surviving; theme stores nothing, so
// what is asserted here is the call itself. The scripted API is held until the
// response has been read, which makes "the handler returned first" a fact
// rather than a race.
func TestTheThemeJobOutlivesTheRequestThatMadeIt(t *testing.T) {
	api := &scriptedClaude{hold: make(chan struct{}),
		replies: []string{answer("end_turn", said(themeTurnAnswer))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rig.api.claudeTheme(w, r.WithContext(auth.WithScope(r.Context(), alice)))
	}))
	defer srv.Close()
	resp, err := http.Post(srv.URL+themeAt, "application/json",
		strings.NewReader(`{"transcript":`+readyTranscript+`}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
	var job map[string]any
	_ = json.Unmarshal(body, &job)
	// The response is read, so the handler has returned and its context is
	// cancelled. Only now let the model answer.
	close(api.hold)
	done, _ := rig.await(t, job["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v -- a worker on the request's context "+
			"is cancelled the moment the id is written", done["status"], done["error"])
	}
	result, _ := done["result"].(map[string]any)
	if result["asked"] != true || result["question"] == "" {
		t.Errorf("the turn came back empty: %v", result)
	}
}
