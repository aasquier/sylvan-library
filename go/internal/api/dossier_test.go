package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The dossier's two routes. `internal/claude` holds the mode to a corpus; what
// these prove is the layer above it -- which refusal lands on which status
// and BEFORE any job, that a stored dossier and a stance of off are jobs born
// finished, that a real run is a job on the NET lane whose result is the
// report in Python's key order, that a second click joins the first, and what
// a failed call leaves in the job's error field.
//
// The first Claude surface to be a JOB rather than a plain route from the
// door, so the rig here is the first to wire a registry beside the ledger.

// jobRig is an API with everything a Claude job needs: the pool, the file
// tier, app.db both ways (the store writes, the ledger writes), and a
// registry to hold the job.
type jobRig struct {
	api   *API
	jobs  *jobs.Registry
	rec   *decklog.Recorder
	decks string
	close func()
}

func newJobRig(t *testing.T) *jobRig {
	t.Helper()
	decks := decksDir(t)
	// A deck with no commander, for the 422 that must come before any job.
	headless := filepath.Join(decks, "headless")
	if err := os.MkdirAll(headless, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(headless, "deck.yaml"), []byte(
		"slug: headless\nname: Headless\nstatus: theoretical\nstage: curated\n"+
			"commander: []\ncards:\n  - name: Sol Ring\n    category: ramp\n    why: Mana.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := decklog.NewRecorder(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{
		Logger: quiet, Pool: pooltest.Open(t), DecksDir: decks, AdminEmail: "alice@example.com",
		AppDB: db, AppWriteDB: recorder.DB(), Recorder: recorder,
		ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil), Jobs: reg,
	})
	return &jobRig{api: a, jobs: reg, rec: recorder, decks: decks,
		close: func() { recorder.Close(); db.Close() }}
}

// await waits for every worker and reads the job back through the route, as
// a client would -- the decoded payload for asserting on values, and the raw
// bytes for asserting on key ORDER, which a decoded map has already lost.
func (r *jobRig) await(t *testing.T, id string) (map[string]any, []byte) {
	t.Helper()
	return r.awaitAs(t, alice, id)
}

func (r *jobRig) awaitAs(t *testing.T, scope auth.Scope, id string) (map[string]any, []byte) {
	t.Helper()
	r.jobs.Wait()
	status, payload, raw := callAs(t, r.api, scope, "GET", "/api/jobs/"+id, "")
	if status != 200 {
		t.Fatalf("polling %s answered %d %s", id, status, raw)
	}
	return payload, raw
}

const dossierAt = "/api/decks/alice/kaheera/dossier"

// realPage is the one page every scripted search here returns: what makes a
// URL a source rather than a string the model typed.
const realPage = "https://edhrec.com/real"

// searchedPage is a hosted search's result block for realPage.
func searchedPage(title string) string {
	return fmt.Sprintf(`{"type":"web_search_tool_result","tool_use_id":"srv_1","content":[`+
		`{"type":"web_search_result","url":%q,"title":%q,"encrypted_content":"e","page_age":null}]}`,
		realPage, title)
}

// wholeDossier is a dossier the model might write: two real sources and one it
// composed, a competitor the pool has and one it has not.
const wholeDossier = `{"who":{"prose":"A bear of Qal Sisma.","source_ids":["s1"]},` +
	`"archetype":{"name":"Stompy","prose":"Big green creatures.","source_ids":["s1","s2"]},` +
	`"competitors":[{"card":"Craterhoof Behemoth","prose":"Ends games.","source_ids":["s1"]},` +
	`{"card":"Not A Real Card","prose":"x","source_ids":["s1"]}],` +
	`"allies":{"prose":"None worth the name.","source_ids":[]},` +
	`"rivals":{"prose":"Nobody.","source_ids":[]},` +
	`"standing":{"prose":"A precon face.","source_ids":["s1"]},` +
	`"sources":[{"id":"s1","title":"t","url":"https://edhrec.com/real"},` +
	`{"id":"s2","title":"t","url":"https://example.com/invented"}]}`

// ---- the free half ------------------------------------------------------

func TestTheCachedDossierIsFreeAndShapedAsPythons(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "GET", dossierAt, "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if err := orderedAs(string(raw), []string{"answered_by", "slug", "commander", "dossier", "cached", "generated_at"}); err != nil {
		t.Errorf("%v\n%s", err, raw)
	}
	if payload["cached"] != false || payload["answered_by"] != "" || payload["generated_at"] != nil {
		t.Errorf("an empty dossier reads %s", raw)
	}
	if payload["commander"] != "Goreclaw, Terror of Qal Sisma" {
		t.Errorf("commander is %v", payload["commander"])
	}
	if body, _ := payload["dossier"].(map[string]any); len(body) != 0 {
		t.Errorf("dossier is %v, want {}", payload["dossier"])
	}

	// No commander the pool knows: five keys, and no `answered_by` at all.
	status, payload, raw = callAs(t, rig.api, alice, "GET", "/api/decks/alice/headless/dossier", "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if _, present := payload["answered_by"]; present || len(payload) != 5 {
		t.Errorf("the headless shape is %s; Python's early return has five keys and no answered_by", raw)
	}
	if payload["commander"] != "" {
		t.Errorf("headless commander is %v", payload["commander"])
	}

	// Somebody else's private deck is a 404 (ADR 5), never a 403.
	if status, _, raw := callAs(t, rig.api, alice, "GET", "/api/decks/bob/bobs-private/dossier", ""); status != 404 {
		t.Errorf("%d %s", status, raw)
	}
}

// ---- what is refused, and before any job --------------------------------

func TestADossierRefusesWhatItCanBeforeAnyJob(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	for _, row := range []struct {
		path, body string
		status     int
		detail     string
	}{
		{"/api/decks/alice/headless/dossier", `{}`, 422, "headless has no commander the pool can find"},
		{dossierAt, `{"stance":"emperor"}`, 422, "is not a stance preset"},
		{dossierAt, `{"stance":{"scope":"everywhere"}}`, 422, "is not a scope level"},
		{"/api/decks/bob/bobs-private/dossier", `{}`, 404, "no deck"},
		// No key, and a real plan: 503 from the request, never a job.
		{dossierAt, `{}`, 503, "no ANTHROPIC_API_KEY"},
	} {
		status, payload, raw := callAs(t, rig.api, alice, "POST", row.path, row.body)
		if status != row.status {
			t.Errorf("%s %s answered %d, want %d: %s", row.path, row.body, status, row.status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, row.detail) {
			t.Errorf("%s %s said %q, want it to contain %q", row.path, row.body, detail, row.detail)
		}
	}
	if got := rig.jobs.All(alice.UserID); len(got) != 0 {
		t.Errorf("%d jobs were queued by refused requests", len(got))
	}
}

// ---- the jobs born finished ----------------------------------------------

// A stance of off needs no key: it is decided before the key is asked for,
// and it costs nothing, so it is answered now as a job that took no time.
func TestAtStanceOffTheDossierIsAJobBornFinishedEvenWithNoKey(t *testing.T) {
	noCredential(t)
	rig := newJobRig(t)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "POST", dossierAt, `{"stance":"off"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["status"] != "done" || payload["kind"] != DossierKind {
		t.Fatalf("not a finished dossier job: %s", raw)
	}
	if payload["label"] != "dossier: Goreclaw, Terror of Qal Sisma" {
		t.Errorf("label is %v", payload["label"])
	}
	result, _ := payload["result"].(map[string]any)
	if result["asked"] != false || !strings.Contains(fmt.Sprint(result["reason"]), "stance is off") {
		t.Errorf("result is %v", result)
	}
	if body, _ := result["dossier"].(map[string]any); len(body) != 0 {
		t.Errorf("a stance-off report carries a dossier: %v", result["dossier"])
	}
}

// ---- a real run ----------------------------------------------------------

func TestWritingADossierIsAJobWhoseResultIsTheReport(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("The Real Page")+","+said(wholeDossier))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()

	status, payload, raw := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if payload["kind"] != DossierKind || payload["label"] != "dossier: Goreclaw, Terror of Qal Sisma" {
		t.Fatalf("job is %s", raw)
	}
	id, _ := payload["id"].(string)
	done, doneRaw := rig.await(t, id)
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v", done["status"], done["error"])
	}
	// The worker reported its ceiling -- eight turns -- and the registry
	// closed the bar when it finished.
	if done["total"] != float64(8) || done["done"] != float64(8) {
		t.Errorf("progress is %v/%v, want 8/8", done["done"], done["total"])
	}
	// Key order is read off the bytes the route wrote: the job payload wraps
	// the report, and the report's keys appear in it in order.
	want := []string{"answered_by", "mode", "model", "slug", "commander", "asked", "reason",
		"stance", "dossier", "generated_at", "cached", "usage", "never"}
	if err := orderedAs(string(doneRaw), want); err != nil {
		t.Errorf("%v\n%s", err, doneRaw)
	}
	result, _ := done["result"].(map[string]any)
	resultRaw, _ := json.Marshal(result)
	if result["asked"] != true || result["cached"] != false || result["reason"] != "" {
		t.Errorf("the report reads %s", resultRaw)
	}
	body, _ := result["dossier"].(map[string]any)
	if err := orderedAs(string(doneRaw), []string{"who", "archetype", "competitors", "allies",
		"rivals", "standing", "sources", "sources_dropped", "competitors_dropped", "searched"}); err != nil {
		t.Errorf("the body's key order: %v", err)
	}
	// The invented source is dropped and counted; the section that cited it
	// no longer points at it.
	if body["sources_dropped"] != float64(1) || body["searched"] != float64(1) {
		t.Errorf("sources_dropped %v searched %v", body["sources_dropped"], body["searched"])
	}
	archetype, _ := body["archetype"].(map[string]any)
	if ids, _ := archetype["source_ids"].([]any); len(ids) != 1 || ids[0] != "s1" {
		t.Errorf("the archetype still cites the dropped source: %v", archetype["source_ids"])
	}
	// The competitor the pool lacks is dropped and counted; the one it has
	// carries the pool's own text.
	competitors, _ := body["competitors"].([]any)
	if len(competitors) != 1 || body["competitors_dropped"] != float64(1) {
		t.Fatalf("competitors %v dropped %v", competitors, body["competitors_dropped"])
	}
	if first, _ := competitors[0].(map[string]any); first["name"] != "Craterhoof Behemoth" || first["oracle_text"] == nil {
		t.Errorf("the surviving competitor is %v", first)
	}
	if result["never"] != "This is Claude's writing over cited pages. The card facts beside it are the pool's." {
		t.Errorf("the promise is %v", result["never"])
	}

	// It was stored: a second ask is a job born finished, served from the
	// store and saying so -- and the free GET now answers with it.
	status, payload, raw = callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	if status != 200 || payload["status"] != "done" {
		t.Fatalf("%d %s", status, raw)
	}
	hit, _ := payload["result"].(map[string]any)
	if hit["asked"] != false || hit["cached"] != true || !strings.Contains(fmt.Sprint(hit["reason"]), "Served from the store") {
		t.Errorf("the second ask reads %v", hit)
	}
	hitBody, _ := json.Marshal(hit["dossier"])
	fresh, _ := json.Marshal(body)
	if string(hitBody) != string(fresh) {
		t.Errorf("the stored dossier differs from the fresh one:\n %s\n %s", hitBody, fresh)
	}
	status, payload, raw = callAs(t, rig.api, alice, "GET", dossierAt, "")
	if status != 200 || payload["cached"] != true || payload["answered_by"] != "claude" {
		t.Errorf("the GET after a run reads %s", raw)
	}
	if api.served != 1 {
		t.Errorf("%d calls were made for one dossier; the store should have answered the rest", api.served)
	}

	// `refresh` writes it again.
	api.replies = append(api.replies, answer("end_turn",
		searchedPage("The Real Page")+","+said(wholeDossier)))
	status, payload, raw = callAs(t, rig.api, alice, "POST", dossierAt, `{"refresh":true}`)
	if status != 200 || payload["status"] == "done" && api.served == 1 {
		t.Fatalf("refresh did not write again: %d %s", status, raw)
	}
	_, _ = rig.await(t, payload["id"].(string))
	if api.served != 2 {
		t.Errorf("refresh made %d calls in total, want 2", api.served)
	}
}

// Two clicks inside the window are one run. The first job is held in flight
// behind a gate, and the second POST must be handed the same id rather than
// starting a second paid search -- the 2026-08-13 double spend, closed.
func TestTwoAsksInFlightAreOneDossierJob(t *testing.T) {
	api := &scriptedClaude{hold: make(chan struct{}), replies: []string{
		answer("end_turn", searchedPage("t")+","+said(wholeDossier))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()

	_, first, raw := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	_, second, raw2 := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	close(api.hold)
	if first["id"] != second["id"] {
		t.Errorf("two asks made two jobs: %s / %s", raw, raw2)
	}
	_, _ = rig.await(t, first["id"].(string))
	if api.served != 1 {
		t.Errorf("%d paid runs for one commander asked twice", api.served)
	}
}

// ---- what a failed call leaves behind --------------------------------------

// The 236-second run is the one place nobody is watching, so what it leaves
// in the job's error field is the whole of what a person gets to debug from
// -- and it is `explain(exc)`'s sentence, bare, as Python records it.
func TestAFailedDossierCallIsAReadableJobError(t *testing.T) {
	api := &scriptedClaude{replies: []string{"!401"}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	status, payload, raw := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	done, _ := rig.await(t, payload["id"].(string))
	if done["status"] != "error" {
		t.Fatalf("the job ended %v", done["status"])
	}
	errText, _ := done["error"].(string)
	if !strings.HasPrefix(errText, "the key was rejected (401)") {
		t.Errorf("the error reads %q; Python records explain()'s sentence and nothing in front of it", errText)
	}
	if done["result"] != nil {
		t.Errorf("a failed job carries a result: %v", done["result"])
	}
}

// The turn ceiling is Python's `ModeExhausted` sentence, full stop included
// and nothing in front of it.
func TestAnExhaustedDossierRecordsPythonsSentence(t *testing.T) {
	replies := make([]string, 8)
	for i := range replies {
		replies[i] = answer("tool_use", fmt.Sprintf(
			`{"type":"tool_use","id":"tu_%d","name":"get_cards","input":{"names":["Sol Ring"]}}`, i))
	}
	api := &scriptedClaude{replies: replies}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	_, payload, _ := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	done, _ := rig.await(t, payload["id"].(string))
	const want = "commander-dossier still wanted tools after 8 turns (8 calls made). " +
		"Nothing was written; nothing is half-done."
	if done["status"] != "error" || done["error"] != want {
		t.Errorf("the exhausted job reads %v / %v, want %q", done["status"], done["error"], want)
	}
}

// ---- the accounting -------------------------------------------------------

func TestTheDossierRecordsUnderItsOwnMode(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("t")+","+said(wholeDossier))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	_, payload, _ := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	_, _ = rig.await(t, payload["id"].(string))
	rows, err := rig.rec.DB().Query("SELECT mode FROM claude_usage")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	modes := []string{}
	for rows.Next() {
		var mode string
		if err := rows.Scan(&mode); err != nil {
			t.Fatal(err)
		}
		modes = append(modes, mode)
	}
	if len(modes) != 1 || modes[0] != "commander-dossier" {
		t.Errorf("the ledger holds %v, want one commander-dossier row", modes)
	}
}

// The job's store write happens AFTER the request has gone, through a real
// server whose request context is cancelled the moment the handler returns
// -- the shape that hid the sim cache's dead-context write for a day. The
// recorder every other test here uses never cancels, so it could not see
// this; a real `httptest.Server` can.
func TestTheDossierJobStoresAfterTheRequestHasGone(t *testing.T) {
	api := &scriptedClaude{replies: []string{
		answer("end_turn", searchedPage("t")+","+said(wholeDossier))}}
	api.start(t)
	rig := newJobRig(t)
	defer rig.close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No mux in front of the handler here, so the path values the door
		// would have set are set by hand.
		r.SetPathValue("owner", "alice")
		r.SetPathValue("slug", "kaheera")
		rig.api.claudeDossier(w, r.WithContext(auth.WithScope(r.Context(), alice)))
	}))
	defer srv.Close()
	resp, err := http.Post(srv.URL+dossierAt, "application/json", strings.NewReader(`{}`))
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
	done, _ := rig.await(t, job["id"].(string))
	if done["status"] != "done" {
		t.Fatalf("the job ended %v: %v", done["status"], done["error"])
	}
	// The second ask is served from the store, which is only true if the
	// write survived the request's context being cancelled under it.
	status, payload, raw := callAs(t, rig.api, alice, "POST", dossierAt, `{}`)
	if status != 200 || payload["status"] != "done" {
		t.Fatalf("%d %s", status, raw)
	}
	if hit, _ := payload["result"].(map[string]any); hit["cached"] != true {
		t.Errorf("the dossier was not stored by a job run after its request had gone: %v", hit)
	}
}
