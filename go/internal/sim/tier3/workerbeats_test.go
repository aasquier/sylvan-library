package tier3

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The narration half of the worker wire: the flag goes out, the beats come
// back, and a shim that has never heard of either is still understood.
//
// The three properties are the ones a deploy can break independently. The app
// and the worker ship the same artefact now, but they are *deployed* minutes
// apart on purpose, so every line of this stream has to degrade rather than
// fail — that is what `wire.go` is for and what this exercises.

// A match asked to narrate carries the flag, and the beats it gets back reach
// the listener whole.
func TestTheAskCarriesNarrationAndTheBeatsComeBack(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	asked := make(chan map[string]any, 1)
	shim.on("match", func(w http.ResponseWriter, r *http.Request) {
		var ask map[string]any
		_ = json.NewDecoder(r.Body).Decode(&ask)
		asked <- ask

		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		lines := []string{
			// Beats before the row that closes their game: one pass over
			// Forge's output feeds the event parser first, so this is the
			// order the real shim emits in.
			`{"events":{"game":1,"events":[` +
				`{"kind":"turn","turn":3,"seat":1},` +
				`{"kind":"land","turn":3,"seat":1,"card":"Bojuka Bog"},` +
				`{"kind":"attack","turn":3,"seat":1,"card":"Gyome, Master Chef",` +
				`"target_seat":2},` +
				`{"kind":"outcome","turn":3,"seat":2,` +
				`"note":"has lost trying to draw cards from empty library"}` +
				`],"truncated":true}}`,
			`{"game":1,"row":{"index":0,"milliseconds":1200,"draw":false,` +
				`"winner":"gyome","winner_seat":1,"turns":9,"timed_out":false}}`,
			`{"result":{"games":[` +
				`{"index":0,"milliseconds":1200,"draw":false,"winner":"gyome",` +
				`"winner_seat":1,"turns":9,"timed_out":false}],` +
				`"seats":{"1":"gyome","2":"trostani"},"wall_seconds":4.5}}`,
		}
		for _, line := range lines {
			_, _ = io.WriteString(w, line+"\n")
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	var heard []EventLog
	var order []string
	run, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome"), testDeck("trostani")},
		MatchAsk{Games: 1, Clock: 300, Seed: big.NewInt(3), Narrate: true,
			OnEvents: func(log EventLog) {
				heard = append(heard, log)
				order = append(order, "events")
			},
			OnGame: func(int, *GameResult) { order = append(order, "game") }})
	if err != nil {
		t.Fatalf("the narrated match failed: %v", err)
	}
	if run == nil || len(run.Games()) != 1 {
		t.Fatal("the run did not come back")
	}

	// The flag crossed. Asserted on the ask rather than on the answer,
	// because a stub that narrates unconditionally would hide a client that
	// had quietly stopped asking.
	ask := <-asked
	if narrate, _ := ask["narrate"].(bool); !narrate {
		t.Errorf("the ask did not carry narration: %v", ask)
	}

	if len(heard) != 1 {
		t.Fatalf("%d event logs were heard, want 1", len(heard))
	}
	log := heard[0]
	if log.Game != 1 || len(log.Events) != 4 {
		t.Fatalf("the log came across as game %d with %d beats",
			log.Game, len(log.Events))
	}
	if !log.Truncated {
		t.Error("the far side's cut did not cross; a short list looks whole")
	}
	if log.Events[1].Kind != EventLand || log.Events[1].Card != "Bojuka Bog" {
		t.Errorf("the land beat came across as %+v", log.Events[1])
	}
	if log.Events[2].TargetSeat != 2 {
		t.Errorf("the attack lost its target seat: %+v", log.Events[2])
	}
	// The reason is kept whole and follows the verb, which is what makes
	// "<player> <verb> <note>" a sentence wherever it is rendered.
	if log.Events[3].Note != "has lost trying to draw cards from empty library" {
		t.Errorf("the outcome's reason came across as %q", log.Events[3].Note)
	}

	// Beats first, then the row. A listener that stashes the beats and
	// publishes them with the row depends on this, and it is a property of
	// the far side's single pass rather than of anything here — so it is
	// worth failing loudly if it ever inverts.
	if len(order) != 2 || order[0] != "events" || order[1] != "game" {
		t.Errorf("the stream arrived as %v, want [events game]", order)
	}
}

// A shim too old to narrate simply never sends the line, and the match is
// otherwise untouched — the same way a pre-theater shim's row-less tick still
// ticks the bar. Deploy skew degrades to "no beats", never to a failed match.
func TestAShimThatCannotNarrateStillPlaysTheMatch(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w,
			`{"game":1,"row":{"index":0,"milliseconds":1200,"draw":false,`+
				`"winner":"gyome","winner_seat":1,"turns":9,"timed_out":false}}`+"\n"+
				`{"result":{"games":[`+
				`{"index":0,"milliseconds":1200,"draw":false,"winner":"gyome",`+
				`"winner_seat":1,"turns":9,"timed_out":false}],`+
				`"seats":{"1":"gyome"},"wall_seconds":4.5}}`+"\n")
	})

	told := 0
	run, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome")},
		MatchAsk{Games: 1, Clock: 300, Narrate: true,
			OnEvents: func(EventLog) { told++ }})
	if err != nil {
		t.Fatalf("an old shim's silence failed the match: %v", err)
	}
	if len(run.Games()) != 1 {
		t.Fatalf("the run came back with %d games", len(run.Games()))
	}
	if told != 0 {
		t.Errorf("beats were invented from a stream that carried none (%d)", told)
	}
}

// Beats arriving for nobody must not take the match down with them. A room
// that stopped listening, or a caller that never asked to, is a nil callback —
// and the line is simply dropped.
func TestBeatsWithNobodyListeningAreDropped(t *testing.T) {
	t.Parallel()
	shim := newStubShim(t)
	shim.on("match", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w,
			`{"events":{"game":1,"events":[{"kind":"turn","turn":1,"seat":1}]}}`+"\n"+
				`{"game":1}`+"\n"+
				`{"result":{"games":[`+
				`{"index":0,"milliseconds":10,"draw":false,"winner":"gyome",`+
				`"winner_seat":1,"turns":2,"timed_out":false}],`+
				`"seats":{"1":"gyome"},"wall_seconds":1.0}}`+"\n")
	})

	run, err := shim.worker(time.Second, "").RunMatch(context.Background(),
		[]*deck.Deck{testDeck("gyome")}, MatchAsk{Games: 1, Clock: 300})
	if err != nil {
		t.Fatalf("unheard beats failed the match: %v", err)
	}
	if len(run.Games()) != 1 {
		t.Fatalf("the run came back with %d games", len(run.Games()))
	}
}
