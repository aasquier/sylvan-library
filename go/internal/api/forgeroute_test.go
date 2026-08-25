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
	"sync"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	matchledger "github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
)

// The two Forge routes, driven end to end against a **stub shim** — a real
// HTTP server speaking the real wire.
//
// A stub rather than a mock, and a real server rather than a recorder, for two
// reasons this repo has paid for separately. A recorder's context is never
// cancelled, so a job that captured `r.Context()` looks fine under one and
// stores nothing in production — the sim cache did exactly that from v183
// until a real `httptest.Server` asked. And a mocked client would test the
// mock: the ndjson framing, the Content-Type sniff and the per-read budget are
// the parts most likely to be wrong, and none of them exists above the socket.

// stubShim is a worker that plays whatever match the test tells it to.
type stubShim struct {
	games    []tier3.WireGame
	stream   bool
	coverage []tier3.WireReport
	// failCoverage answers /coverage with a 503, the shape an unbuilt worker
	// image gives.
	failCoverage bool
	// errorLine ends the stream with an error instead of a result.
	errorLine string
	errorType string
	// beats is one game's worth of narration, emitted before every game's row
	// — and only when the ask said to narrate, which is what the real shim
	// does. Nil means this stub has nothing to tell, which is also a shim too
	// old to have been taught.
	beats []tier3.GameEvent
	// hold, when non-nil, blocks the stream after each game until the test
	// lets it go. That is what makes the match theater observable: a finished
	// job clears its `partial` (deliberately — the result is
	// the whole answer and a leftover partial is a stale second copy of part
	// of it), so the only place to see a row seated live is *during* the
	// match.
	hold chan struct{}

	// `seen` is written by `net/http`'s per-connection goroutines, so it needs
	// the lock -- and the reason it needs it now is the `hold` field above.
	//
	// Holding the stream is what made the dedupe test honest (see
	// [TestTwoIdenticalAsksAreOneMatch]), and it is *also* what made two of
	// this stub's handlers run at once: with a match parked mid-stream, the
	// second ask's health poll is served by a second goroutine while the first
	// is still inside the handler. An unguarded `append` from two of them is a
	// real race, and the fix for the timing bug is what opened it. CI's arm64
	// runner found it; twenty `-race -count` runs on the maintainer's amd64
	// Mac did not, which is the same architecture story that test's own
	// comment tells.
	//
	// Read it with [stubShim.requests], never the field: a reader that runs
	// while anything is in flight races the writer just as surely.
	mu   sync.Mutex
	seen []string
	// asked is the last /match ask, so a test can assert what *crossed*
	// rather than only what came back — a client that quietly stopped sending
	// `narrate` would still look green against a stub that narrates anyway.
	asked map[string]any
}

// askedFor is the last /match ask this stub received. A copy, for `seen`'s
// reason.
func (s *stubShim) askedFor() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	for k, v := range s.asked {
		out[k] = v
	}
	return out
}

// requests is what the stub has been asked, as of now. A copy, so the caller
// can range over it while the worker is still talking.
func (s *stubShim) requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func (s *stubShim) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.seen = append(s.seen, r.Method+" "+r.URL.Path)
		s.mu.Unlock()
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		case "/coverage":
			if s.failCoverage {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "no Forge card data at /opt/forge/res/cardsfolder/cardsfolder.zip"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"reports": s.coverage})
		case "/match":
			s.match(w, r)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *stubShim) match(w http.ResponseWriter, r *http.Request) {
	ask := map[string]any{}
	_ = json.NewDecoder(r.Body).Decode(&ask)
	s.mu.Lock()
	s.asked = ask
	s.mu.Unlock()

	if !s.stream {
		_ = json.NewEncoder(w).Encode(tier3.WireRun{Games: s.games,
			Seats: map[int]string{1: "kaheera", 2: "mono-green"}, WallSeconds: 12.5})
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	emit := func(v any) {
		raw, _ := json.Marshal(v)
		_, _ = w.Write(append(raw, '\n'))
		if flusher != nil {
			flusher.Flush()
		}
	}
	narrate, _ := ask["narrate"].(bool)
	for i, g := range s.games {
		// Beats first, then the row that closes their game: one pass over
		// Forge's output feeds the event parser before the result parser, so
		// this is the order the real shim emits in and the order the client
		// is entitled to expect.
		if narrate && s.beats != nil {
			emit(map[string]any{"events": tier3.EventLog{
				Game: i + 1, Events: s.beats}})
		}
		emit(map[string]any{"game": i + 1, "row": g})
		if s.hold != nil {
			<-s.hold
		}
	}
	if s.errorLine != "" {
		emit(map[string]any{"error": s.errorLine, "type": s.errorType})
		return
	}
	version := "2.0.14"
	emit(map[string]any{"result": tier3.WireRun{Games: s.games,
		Seats:       map[int]string{1: "kaheera", 2: "mono-green"},
		WallSeconds: 12.5, ForgeVersion: &version}})
}

func won(index, ms, seat, turns int) tier3.WireGame {
	label := fmt.Sprintf("Ai(%d)-x", seat)
	return tier3.WireGame{Index: index, Milliseconds: ms, Winner: &label,
		WinnerSeat: &seat, Turns: &turns}
}

// forgeAPI builds an API whose hosted worker points at the stub, with a real
// job registry and a real app.db.
func forgeAPI(t *testing.T, shim *stubShim) (*API, *jobs.Registry, string) {
	t.Helper()
	a, reg, dbPath, _ := forgeAPIIn(t, shim)
	return a, reg, dbPath
}

// forgeAPIIn is forgeAPI plus the library's own directory, for a test that
// has to put a deck in it that no fixture file can hold: adding one to
// `gate/testdata` would add it to every API test's library at once.
func forgeAPIIn(t *testing.T, shim *stubShim) (*API, *jobs.Registry, string, string) {
	t.Helper()
	dbPath := appDB(t)
	dir := decksDir(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	writeDB, err := auth.OpenReadWrite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writeDB.Close() })

	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := jobs.New(jobs.Config{Logger: quiet})
	// The machine this API is on: a hosted worker pointed at the stub shim,
	// or nothing at all. A value rather than MTGLAB_FORGE_WORKER_URL on the
	// process (ADR 40), which is why two of these can run at once -- and why
	// the stub each one built is the stub its own API talks to.
	var forge tier3.Settings
	if shim != nil {
		forge.WorkerURL = shim.serve(t).URL
	}
	worker := &tier3.Worker{Settings: forge, Boot: 5 * time.Second,
		Sleep: func(time.Duration) {}}
	a := New(Config{Logger: quiet, Pool: pooltest.Open(t), DecksDir: dir,
		AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: writeDB,
		Jobs: reg, ForgeWorker: worker, Forge: forge,
		MatchLedger: matchledger.FromDB(writeDB, quiet)})
	return a, reg, dbPath, dir
}

func forgeServer(t *testing.T, a *API) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(auth.WithScope(r.Context(), alice))
		if r.Method == http.MethodGet {
			a.forgeGate(w, r)
			return
		}
		a.simForge(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func postForge(t *testing.T, srv *httptest.Server, body string) (int, map[string]any) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/sim/forge", "application/json",
		strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	payload := map[string]any{}
	_ = json.Unmarshal(raw, &payload)
	return resp.StatusCode, payload
}

// TestAHostedMatchRunsAfterTheRequestHasGone is the recorded lesson, asked of
// this family: **a job's Run never touches the request's context.**
//
// A match is minutes of JVM on another machine, and `net/http` cancels the
// request's context the instant the handler returns. A closure that captured
// it would have its HTTP call to the shim aborted mid-match — and the job would
// fail with `context canceled` for every match anybody ever ran, which is a
// failure a recorder-driven test cannot see at all.
func TestAHostedMatchRunsAfterTheRequestHasGone(t *testing.T) {
	shim := &stubShim{stream: true, games: []tier3.WireGame{
		won(1, 5421, 1, 11), won(2, 4000, 2, 9), won(3, 6800, 1, 12)}}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"mono-green","games":3}`)
	if status != 200 {
		t.Fatalf("%d %v", status, payload)
	}
	reg.Wait()

	job := reg.Get(payload["id"].(string), alice.UserID)
	if job == nil {
		t.Fatal("the match job vanished")
	}
	if job.Status() != "done" {
		t.Fatalf("the match ended %q: %v", job.Status(), job.Payload().Error)
	}
	result, _ := job.Payload().Result.(forgeResult)
	if result.Played != 3 {
		t.Fatalf("played %d games, want 3", result.Played)
	}
	if result.Decks[0].Wins != 2 || result.Decks[1].Wins != 1 {
		t.Errorf("wins %d/%d, want 2/1", result.Decks[0].Wins, result.Decks[1].Wins)
	}
	// The match really crossed the wire: the stub saw a health poll, the
	// pre-flight and the match itself.
	want := []string{"GET /healthz", "POST /coverage", "GET /healthz", "POST /match"}
	if got := shim.requests(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the shim saw %v, want %v", got, want)
	}
}

// TestTheBarTicksPerGame is the match theater's half: each finished game
// reaches the job's `partial` as it happens, shaped by the same builder the
// final tally uses.
//
// **Observed mid-match, which is the only place it exists.** A finished job
// clears its partial — the result is the whole answer, and a
// leftover partial is a stale second copy of part of it — so a test that read
// the partial off a completed job would be asserting `nil` and calling it
// green. The stub holds the stream between games so the rows can be read
// while they are the only answer there is.
func TestTheBarTicksPerGame(t *testing.T) {
	shim := &stubShim{stream: true, hold: make(chan struct{}),
		games: []tier3.WireGame{won(1, 5421, 1, 11), won(2, 4000, 2, 9)}}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"mono-green","games":2}`)
	if status != 200 {
		t.Fatalf("%d %v", status, payload)
	}
	id := payload["id"].(string)

	// Game one has been emitted and the stream is held. The bar must already
	// show it.
	first := awaitPartial(t, reg, id, 1)
	if first.Rows[0].Game != 1 || first.Rows[0].Winner == nil || *first.Rows[0].Winner != "kaheera" {
		t.Fatalf("the first row was seated as %+v", first.Rows[0])
	}
	if got := reg.Get(id, alice.UserID).Payload().Done; got != 1 {
		t.Errorf("the bar reads %d of 2 after one game", got)
	}
	shim.hold <- struct{}{}

	second := awaitPartial(t, reg, id, 2)
	if second.Rows[1].Game != 2 || *second.Rows[1].Winner != "mono-green" {
		t.Fatalf("the second row was seated as %+v", second.Rows[1])
	}
	// **The rows are copies, not a shared slice.** The job hands out a fresh
	// slice per tick, so the payload a poll read a moment ago cannot grow
	// under the client that is rendering it.
	if len(first.Rows) != 1 {
		t.Errorf("the first partial grew to %d rows after the second game", len(first.Rows))
	}
	shim.hold <- struct{}{}
	reg.Wait()

	job := reg.Get(id, alice.UserID)
	if job.Status() != "done" {
		t.Fatalf("the match ended %q", job.Status())
	}
	// One builder, so a theater and a tale of the tape cannot disagree: every
	// row seated live is byte-identical to its final self.
	result := job.Payload().Result.(forgeResult)
	for i, live := range second.Rows {
		liveJSON, _ := json.Marshal(live)
		finalJSON, _ := json.Marshal(result.Rows[i])
		if string(liveJSON) != string(finalJSON) {
			t.Errorf("row %d live %s, final %s", i+1, liveJSON, finalJSON)
		}
	}
	// And the finished job carries no partial, which is the behaviour that
	// made the mid-match observation necessary in the first place.
	if job.Payload().Partial != nil {
		t.Errorf("a finished job kept a stale partial: %v", job.Payload().Partial)
	}
}

// awaitPartial waits for the job's partial to hold n rows.
//
// A poll rather than a synchronisation point because that is what the client
// does — `followJob` asks `/api/jobs/{id}` on a timer — so this exercises the
// same visibility the browser gets.
func awaitPartial(t *testing.T, reg *jobs.Registry, id string, n int) forgePartial {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job := reg.Get(id, alice.UserID)
		if job != nil {
			if partial, ok := job.Payload().Partial.(forgePartial); ok && len(partial.Rows) >= n {
				return partial
			}
			if job.Status() == "error" {
				t.Fatalf("the match failed while the bar was watched: %v", job.Payload().Error)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the bar never seated %d rows", n)
	return forgePartial{}
}

// TestAFinishedMatchIsRecorded is ADR 36 through the route: the ledger row is
// written from the job, on the app's own app.db, with the labels the decks
// wore when they played.
func TestAFinishedMatchIsRecorded(t *testing.T) {
	shim := &stubShim{stream: true, games: []tier3.WireGame{
		won(1, 5421, 1, 11), won(2, 4000, 2, 9)}}
	a, reg, dbPath := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv,
		`{"a_slug":"kaheera","b_slug":"mono-green","games":2,"seed":99}`)
	if status != 200 {
		t.Fatalf("%d %v", status, payload)
	}
	reg.Wait()

	db, err := auth.OpenReadWrite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	matches, err := matchledger.FromDB(db, slog.Default()).Recent(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("the ledger holds %d matches, want 1", len(matches))
	}
	m := matches[0]
	if m.Seed == nil || *m.Seed != 99 {
		t.Errorf("seed = %v, want 99", m.Seed)
	}
	if m.Clock != ForgeClock || m.GamesRequested != 2 {
		t.Errorf("clock %d games %d", m.Clock, m.GamesRequested)
	}
	if !m.Hosted {
		t.Error("a worker match was recorded as local")
	}
	if m.ForgeVersion == nil || *m.ForgeVersion != "2.0.14" {
		t.Errorf("forge version = %v, want 2.0.14", m.ForgeVersion)
	}
	if len(m.Seats) != 2 || m.Seats[0].Slug != "kaheera" || m.Seats[1].Slug != "mono-green" {
		t.Fatalf("seats: %v", m.Seats)
	}
	if m.Seats[0].Wins != 1 || m.Seats[1].Wins != 1 {
		t.Errorf("wins %d/%d, want 1/1", m.Seats[0].Wins, m.Seats[1].Wins)
	}
}

// TestTheRefusalsHappenInTheRequest is the `themeruns` division: everything
// refusable is refused with a status code, and the job only ever fails for
// runtime reasons.
//
// The four are kept apart on purpose — collapsing them tells somebody their
// worker is missing when the deck simply is not theirs.
func TestTheRefusalsHappenInTheRequest(t *testing.T) {
	for _, c := range []struct {
		note, body string
		want       int
		detail     string
	}{
		{"no a_slug", `{"b_slug":"mono-green"}`, 422, "a_slug is required"},
		{"no b_slug", `{"a_slug":"kaheera"}`, 422, "b_slug is required"},
		{"an empty a_slug", `{"a_slug":"","b_slug":"mono-green"}`, 422, "a_slug is required"},
		{"a falsy a_slug", `{"a_slug":0,"b_slug":"mono-green"}`, 422, "a_slug is required"},
		{"a deck nobody has", `{"a_slug":"nope","b_slug":"mono-green"}`, 404, "no deck 'nope'"},
		{"a deck nobody has, on the b side", `{"a_slug":"kaheera","b_slug":"nope"}`, 404, "no deck 'nope'"},
	} {
		t.Run(c.note, func(t *testing.T) {
			shim := &stubShim{stream: true}
			a, _, _ := forgeAPI(t, shim)
			srv := forgeServer(t, a)
			status, payload := postForge(t, srv, c.body)
			if status != c.want {
				t.Fatalf("answered %d, want %d (%v)", status, c.want, payload)
			}
			if got, _ := payload["detail"].(string); got != c.detail {
				t.Errorf("detail = %q, want %q", got, c.detail)
			}
		})
	}
}

// TestADeckForgeCannotPlayIsA422ThatNamesTheCards is the non-negotiable
// piece, at the route.
//
// A Forge game *plays on* without a card it does not implement and reports a
// winner that looks entirely normal, so the pre-flight refuses before a JVM
// starts — and the refusal names the cards, because the message is the fix.
func TestADeckForgeCannotPlayIsA422ThatNamesTheCards(t *testing.T) {
	shim := &stubShim{stream: true, coverage: []tier3.WireReport{
		{Slug: "kaheera", Checked: 100, Resolved: map[string]string{}, Missing: []string{}},
		{Slug: "mono-green", Checked: 99, Resolved: map[string]string{},
			Missing: []string{"Nonexistent Card 1", "Nonexistent Card 2"}},
	}}
	a, _, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"mono-green"}`)
	if status != 422 {
		t.Fatalf("answered %d, want 422 (%v)", status, payload)
	}
	detail, _ := payload["detail"].(string)
	for _, want := range []string{"Nonexistent Card 1", "Nonexistent Card 2",
		"so no result would mean anything"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the refusal did not say %q: %q", want, detail)
		}
	}
	// And no match was ever asked for.
	for _, seen := range shim.requests() {
		if seen == "POST /match" {
			t.Error("a match ran against a deck Forge cannot play")
		}
	}
}

// TestADeckWithNoCardsIsRefusedWithoutWakingTheWorker is the empty seat.
//
// Found on the deployed instance, in the Coliseum, as a **2-0 win**: the room
// offered `adrix-and-nev-twincasters` -- a draft with a commander, a name and
// nought else -- in the Challenger picker, sent it to Forge, and reported two
// clean wins to the other seat in 0.4 seconds. The account tab was the only
// thing that gave it away, one line down: "trying to draw cards from empty
// library", on turn one, twice.
//
// Coverage cannot catch this and it is not coverage's fault: it is a ratio,
// and nought missing of nought checked is a perfect score. So the refusal is
// its own, it counts what the deck *declares*, and it happens before the
// pre-flight -- an empty deck should not cost a worker boot to turn away.
func TestADeckWithNoCardsIsRefusedWithoutWakingTheWorker(t *testing.T) {
	t.Parallel()
	shim := &stubShim{stream: true, coverage: []tier3.WireReport{
		{Slug: "kaheera", Checked: 100, Resolved: map[string]string{}, Missing: []string{}},
	}}
	a, _, _, dir := forgeAPIIn(t, shim)
	srv := forgeServer(t, a)

	// A deck the moment after it is created: named, with a commander chosen
	// and not one card in it. `cards: []` rather than a missing key, because
	// the empty list is what the app writes and the missing key is what a
	// hand-rolled fixture writes.
	empty := filepath.Join(dir, "twincasters")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(empty, "deck.yaml"), []byte(
		"slug: twincasters\nname: Twincasters\nstage: draft\n"+
			"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards: []\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	status, payload := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"twincasters"}`)
	if status != 422 {
		t.Fatalf("answered %d, want 422 (%v)", status, payload)
	}
	detail, _ := payload["detail"].(string)
	// The address the caller asked with, so a two-deck refusal says which
	// seat is empty, and the fix, because the message is the fix.
	for _, want := range []string{"twincasters", "has no cards in it yet",
		"no result would mean anything", "add its cards"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the refusal did not say %q: %q", want, detail)
		}
	}
	// No match, and no coverage either: the worker machine was never woken.
	for _, seen := range shim.requests() {
		if seen == "POST /match" {
			t.Error("a match ran against a deck with no cards in it")
		}
		if seen == "POST /coverage" {
			t.Error("the worker was woken to refuse a deck with no cards")
		}
	}
}

// TestAWorkerWithNoForgeIsA503 keeps the distribution's absence apart from a
// deck's problem: one is the instance, the other is the request.
func TestAWorkerWithNoForgeIsA503(t *testing.T) {
	shim := &stubShim{stream: true, failCoverage: true}
	a, _, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)
	status, payload := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"mono-green"}`)
	if status != 503 {
		t.Fatalf("answered %d, want 503 (%v)", status, payload)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "no Forge card data") {
		t.Errorf("the 503 did not carry the worker's words: %q", detail)
	}
}

// TestAMatchThatFailsIsAJobInError is the other side of the division: a
// failure the request could not have known about arrives as a job in state
// `error`, never as a status code, because the response has already gone.
func TestAMatchThatFailsIsAJobInError(t *testing.T) {
	shim := &stubShim{stream: true, games: []tier3.WireGame{won(1, 100, 1, 3)},
		errorLine: "ResultsUntrustworthy: Forge reported problems that invalidate the run",
		errorType: "ResultsUntrustworthy"}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"mono-green","games":1}`)
	if status != 200 {
		t.Fatalf("a runtime failure answered %d in the request", status)
	}
	reg.Wait()
	job := reg.Get(payload["id"].(string), alice.UserID)
	if job.Status() != "error" {
		t.Fatalf("the job ended %q, want error", job.Status())
	}
	if got := job.Payload().Error; got == nil || !strings.Contains(*got, "ResultsUntrustworthy") {
		t.Errorf("the job's error lost the worker's words: %v", got)
	}
}

// TestAPreTheaterShimStillTicks is the deploy-skew case, and it is the reason
// both halves of the stream are optional.
//
// A shim from before the theater sends `{"game": n}` with no row: the bar must
// still move and `partial` simply stays sparse, because an app deployed a few
// minutes before its worker must not break a match over a field.
func TestAPreTheaterShimStillTicks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		case "/match":
			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher, _ := w.(http.Flusher)
			for i := 1; i <= 2; i++ {
				_, _ = fmt.Fprintf(w, "{\"game\":%d}\n", i)
				if flusher != nil {
					flusher.Flush()
				}
			}
			raw, _ := json.Marshal(tier3.WireRun{
				Games: []tier3.WireGame{won(1, 100, 1, 3), won(2, 200, 2, 4)},
				Seats: map[int]string{1: "a", 2: "b"}, WallSeconds: 1})
			_, _ = fmt.Fprintf(w, "{\"result\":%s}\n", raw)
		}
	}))
	defer srv.Close()

	var ticks []int
	seatedRows := 0
	worker := &tier3.Worker{
		Settings: tier3.Settings{WorkerURL: srv.URL},
		Boot:     5 * time.Second, Sleep: func(time.Duration) {},
	}
	run, err := worker.RunMatch(t.Context(), nil, tier3.MatchAsk{
		Games: 2, Clock: 300,
		OnGame: func(finished int, game *tier3.GameResult) {
			ticks = append(ticks, finished)
			if game != nil {
				seatedRows++
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ticks) != 2 || ticks[0] != 1 || ticks[1] != 2 {
		t.Errorf("the bar ticked %v, want [1 2]", ticks)
	}
	if seatedRows != 0 {
		t.Errorf("a pre-theater shim seated %d rows; it sends none", seatedRows)
	}
	if len(run.Games()) != 2 {
		t.Errorf("the final tally holds %d games, want 2", len(run.Games()))
	}
}

// TestAnOldShimAnswersFlatAndIsStillUnderstood is the other direction of the
// same skew: a shim from before the stream ignores the flag and answers one
// plain JSON body. The Content-Type is what says which conversation this is.
func TestAnOldShimAnswersFlatAndIsStillUnderstood(t *testing.T) {
	shim := &stubShim{stream: false, games: []tier3.WireGame{won(1, 100, 1, 3)}}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	status, payload := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"mono-green","games":1}`)
	if status != 200 {
		t.Fatalf("%d %v", status, payload)
	}
	reg.Wait()
	job := reg.Get(payload["id"].(string), alice.UserID)
	if job.Status() != "done" {
		t.Fatalf("a flat answer ended the job %q", job.Status())
	}
	if got := job.Payload().Result.(forgeResult).Played; got != 1 {
		t.Errorf("played %d, want 1", got)
	}
}

// TestTheGateAnswersOnConfigurationAlone is `/api/forge` — the same contract
// `/api/claude` set: configuration is a fact of the environment, and
// reachability is discovered when work is actually asked for.
func TestTheGateAnswersOnConfigurationAlone(t *testing.T) {
	shim := &stubShim{}
	a, _, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)
	resp, err := http.Get(srv.URL + "/api/forge")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["available"] != true {
		t.Errorf("a configured worker answered %v", payload)
	}
	if payload["why"] != nil {
		t.Errorf("why should be null when available: %v", payload["why"])
	}
	// **No machine was woken to answer it.** The gate is asked on every visit
	// to the Simulator; one that started a JVM would be a bill.
	if got := shim.requests(); len(got) != 0 {
		t.Errorf("the gate reached the worker: %v", got)
	}
}

// TestTwoIdenticalAsksAreOneMatch is the dedupe: the FORGE lane is one wide,
// so a second identical click must join the live job rather than queue a
// second JVM behind the first.
func TestTwoIdenticalAsksAreOneMatch(t *testing.T) {
	// **The stub has to hold the stream, and that is the whole test.** The
	// dedupe under test is an *in-flight* join: a second ask joins the first
	// only while the first is still running. Without `hold` this stub finishes
	// the moment it starts, so on a quick enough machine match one is already
	// done when ask two arrives -- and a second job is then the correct
	// answer, which reads as a broken dedupe. It is a test that races itself,
	// and it stayed green here for as long as this laptop stayed slower than
	// CI's arm64 runner. Holding the stream makes the window a fact rather
	// than a hope; the failing form is reproducible by sleeping between the
	// two asks.
	hold := make(chan struct{})
	shim := &stubShim{stream: true, games: []tier3.WireGame{won(1, 100, 1, 3)},
		hold: hold}
	a, reg, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)

	body := `{"a_slug":"kaheera","b_slug":"mono-green","games":1,"seed":5}`
	_, first := postForge(t, srv, body)
	_, second := postForge(t, srv, body)
	if first["id"] != second["id"] {
		t.Errorf("two identical asks made two matches: %v and %v", first["id"], second["id"])
	}
	// Closed rather than sent to once: every later game in this test reads
	// from it too, and a closed channel releases all of them.
	close(hold)
	reg.Wait()

	// And a *different* match is its own job, queued honestly behind it.
	_, other := postForge(t, srv, `{"a_slug":"kaheera","b_slug":"mono-green","games":2,"seed":5}`)
	if other["id"] == first["id"] {
		t.Error("a different games count joined the first match")
	}
	reg.Wait()
}

// TestAGamesCountThatIsNotANumberIsTheRecordedFiveHundred is the pin the
// [forgeGames] comment promised and, for a while, did not
// have -- and what it pins moved when the real bytes were measured: the
// recorded uncaught 500 is **plain-text** three
// words, not a JSON detail. The first version of this route answered
// `{"detail": "invalid literal ..."}` here, a divergence no golden could see
// because the goldens record shape and no golden records this case at all.
func TestAGamesCountThatIsNotANumberIsTheRecordedFiveHundred(t *testing.T) {
	shim := &stubShim{stream: true}
	a, _, _ := forgeAPI(t, shim)
	srv := forgeServer(t, a)
	for _, body := range []string{
		`{"a_slug":"kaheera","b_slug":"mono-green","games":"many"}`,
		`{"a_slug":"kaheera","b_slug":"mono-green","seed":"soon"}`,
	} {
		resp, err := http.Post(srv.URL+"/api/sim/forge", "application/json",
			strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("%s answered %d: %s", body, resp.StatusCode, raw)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Fatalf("%s: Content-Type %q", body, ct)
		}
		if string(raw) != "Internal Server Error" {
			t.Fatalf("%s: body %q", body, raw)
		}
	}
}
