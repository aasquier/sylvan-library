package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The Forge surface's two unwatched halves: the **local** run path, and the
// seams whose defaults only a deployed process takes.
//
// Everything in this package drives the hosted path, because the hosted path is
// a stub shim and the local one is a JVM. What that left unreached is not the
// JVM — it is the three lines of *this* package that choose it, report it, and
// name it, and those are reachable for the asking: a machine with no worker
// configured, a match asked for anyway, and the sentence a player gets instead
// of the diagnosis.

// ---- the seams' own defaults -----------------------------------------------

// **The worker a process without one builds.** `forgeWorker` hands back the
// injected client when a test gave it one, and otherwise builds a client over
// this machine's own settings — which is the line the deployed binary runs and
// which, by construction, no test that injects a client can reach.
func TestTheWorkerAnInstanceBuildsForItselfCarriesItsOwnSettings(t *testing.T) {
	t.Parallel()
	settings := tier3.Settings{WorkerURL: "https://worker.invalid"}
	a := New(Config{Logger: quietLogger(), Forge: settings})
	built := a.forgeWorker()
	if built == nil {
		t.Fatal("an instance with no injected client built no worker at all")
	}
	if built.Settings.WorkerURL != settings.WorkerURL {
		t.Errorf("the worker was built over %q, want this machine's own %q",
			built.Settings.WorkerURL, settings.WorkerURL)
	}
	// And an injected one still wins, so the default is a fallback rather than
	// an override.
	injected := &tier3.Worker{Settings: tier3.Settings{WorkerURL: "https://other.invalid"}}
	withClient := New(Config{Logger: quietLogger(), Forge: settings,
		ForgeWorker: injected})
	if withClient.forgeWorker() != injected {
		t.Error("an instance handed a worker built a second one anyway")
	}
}

// **The pre-flight asks the machine that holds the card scripts**, and on a
// machine with no worker that is this one. The local arm had never run: every
// test here configures a worker, so every pre-flight went over the wire.
func TestThePreflightAsksThisMachineWhenNoWorkerIsConfigured(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger()})
	err := a.preflight(t.Context(), false, []*deck.Deck{
		{Slug: "gyome", Name: "Gyome"}})
	if err == nil {
		t.Fatal("a machine with no Forge on it passed a coverage check")
	}
}

// **The gate's "available with nothing to explain" arm is left open on
// purpose.** It is the one reading of `forgeStatus` that says yes without a
// worker — the maintainer's own laptop — and reaching it needs a JVM that
// answers the version probe. The probe's budget is thirty seconds, and this
// suite under full parallel load has already blown it once against the
// committed stand-in (`internal/sim/tier3/testdata/fakejava`): a new
// load-sensitive test in this package, for one statement, is the wrong trade.
// `internal/sim/tier3` owns both halves of the question that arm asks and
// drives them there.

// ---- the local match path --------------------------------------------------

// **A local match that cannot run says so in the room's own words.**
//
// The job's error is rendered verbatim by the room — "The match failed: …" — so
// this is the last place a path, a variable name or a status code can be
// stopped (commandment 10). The local arm of the run is what a machine with no
// worker takes, and nothing had ever taken it here.
func TestALocalMatchThatCannotRunFailsInWordsAPlayerCanRead(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t),
		// No worker and no Forge: the local path, with nothing to run.
		Forge: tier3.Settings{Home: filepath.Join(t.TempDir(), "no-forge-here")}})

	_, _, err := a.playForgeMatch(reportFunc(func(int, int) {}), forgeMatch{
		decks: []*deck.Deck{
			{Slug: "gyome", Name: "Gyome"}, {Slug: "mono-green", Name: "Mono Green"}},
		addresses: []string{"alice/gyome", "alice/mono-green"},
		ownerIDs:  []*int64{nil, nil},
		games:     1,
		hosted:    false,
	})
	if err == nil {
		t.Fatal("a local match with no Forge on the machine reported success")
	}
	// Commandment 10: the sentence a player reads names nothing underneath it.
	for _, machinery := range []string{"MTGLAB_", "http", "HTTP", ".jar",
		"/var/folders", "exec", "500", "503"} {
		if strings.Contains(err.Error(), machinery) {
			t.Errorf("the sentence a player reads carries %q: %q",
				machinery, err.Error())
		}
	}
}

// ---- the narrated hosted match ---------------------------------------------

// boardShim is a worker that narrates: it sends a battlefield before each
// game's row, and the row it closes with ended on a killing blow.
//
// A stub of its own rather than the shared one, because the two facts it carries
// are exactly the two nothing else sends — a `board` on the event log, and a
// `killer` on the row. Both are the scribe's (ADR 42), and a worker image built
// before the scribe sends neither, which is why the client has arms for their
// absence and why their presence had never been driven from here.
type boardShim struct{}

func (boardShim) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		case "/coverage":
			_ = json.NewEncoder(w).Encode(map[string]any{"reports": []tier3.WireReport{}})
		case "/match":
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
			// The battlefield first, then the row that closes its game: the
			// order one pass over Forge's output produces, and the order the
			// client is entitled to expect.
			emit(map[string]any{"events": tier3.EventLog{Game: 1,
				Board: &tier3.BoardReel{
					Seats: []tier3.BoardSeat{
						{Seat: 1, Name: "Gyome", Life: 40},
						{Seat: 2, Name: "Mono Green", Life: 0}},
					Cards: []tier3.BoardCard{
						{ID: 1, Name: "Craterhoof Behemoth", Seat: 1,
							Types: "Legendary Creature - Beast"}},
					Steps: []tier3.BoardStep{{Turn: 9, Seat: 1}},
				}}})
			turns, seat := 9, 1
			label := "Ai(1)-x"
			emit(map[string]any{"game": 1, "row": tier3.WireGame{
				Index: 1, Milliseconds: 5400, Winner: &label,
				WinnerSeat: &seat, Turns: &turns,
				Killer: &tier3.KillingBlow{Amount: 21,
					Card: "Craterhoof Behemoth", Combat: true,
					Seat: 2, Turn: 9, Sources: 6},
			}})
			version := "2.0.14"
			emit(map[string]any{"result": tier3.WireRun{
				Games:       []tier3.WireGame{{Index: 1, Milliseconds: 5400}},
				Seats:       map[int]string{1: "gyome", 2: "mono-green"},
				WallSeconds: 7.5, ForgeVersion: &version}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A narrated match carries its **battlefield** and its **killing blow** all the
// way through, and both are resolved against the pool on the way.
//
// The board's paintings are looked up once for the whole match, and the blow's
// victim is named by slug through the closure the row builder is handed — two
// lines that only a match with a scribe behind it reaches.
func TestANarratedMatchCarriesItsBoardAndItsKillingBlow(t *testing.T) {
	t.Parallel()
	srv := boardShim{}.serve(t)
	worker := &tier3.Worker{
		Settings: tier3.Settings{WorkerURL: srv.URL},
		Boot:     5 * time.Second, Sleep: func(time.Duration) {},
	}
	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t),
		ForgeWorker: worker, Forge: tier3.Settings{WorkerURL: srv.URL}})

	var partials []forgePartial
	out, _, err := a.playForgeMatch(partialFunc(func(_, _ int, value any) {
		if p, ok := value.(forgePartial); ok {
			partials = append(partials, p)
		}
	}), forgeMatch{
		decks: []*deck.Deck{
			{Slug: "gyome", Name: "Gyome"}, {Slug: "mono-green", Name: "Mono Green"}},
		addresses: []string{"alice/gyome", "alice/mono-green"},
		ownerIDs:  []*int64{nil, nil},
		games:     1, narrate: true, hosted: true,
	})
	if err != nil {
		t.Fatalf("a narrated match failed: %v", err)
	}
	if len(out.Beats) != 1 {
		t.Fatalf("%d games of narration came back, want 1", len(out.Beats))
	}
	board := out.Beats[0].Board
	if board == nil {
		t.Fatal("a narrated match came back with no battlefield on it")
	}
	if len(board.Cards) != 1 {
		t.Fatalf("%d cards on the board, want 1", len(board.Cards))
	}
	// The painting was resolved against the pool, which is the whole reason the
	// board is resolved once per match rather than once per game.
	if board.Cards[0].Image == "" {
		t.Errorf("the board's one card went unpainted: %+v", board.Cards[0])
	}

	// And the live row the room seated carries the blow, with the chair that
	// died named by its deck rather than by its number.
	var seated *forgeBlow
	for _, p := range partials {
		for _, row := range p.Rows {
			if row.Killer != nil {
				seated = row.Killer
			}
		}
	}
	if seated == nil {
		t.Fatal("no row seated live carried the killing blow")
	}
	if seated.Victim != "mono-green" {
		t.Errorf("the blow landed on %q, want mono-green", seated.Victim)
	}
}

// partialFunc adapts a closure to [jobs.Progress], keeping the partial.
type partialFunc func(done, total int, value any)

func (f partialFunc) Report(done, total int)                   { f(done, total, nil) }
func (f partialFunc) ReportPartial(done, total int, value any) { f(done, total, value) }
