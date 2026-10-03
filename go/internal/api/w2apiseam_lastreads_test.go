package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/decklog"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The two reads that come **last**, and the library that stops before them.
//
// Both of these are a second or third pool read inside one request, and both sit
// behind reads that have already succeeded, which is why nothing could enter
// them until a pool could be handed in that answers for a while (`pool.Connect`).
// The camera hydrates a batch of readings after the readings are taken; the
// import files the deck and only then asks the pool which of its cards it knows,
// which is the answer the gate's verdict is computed from.

// **The camera refuses rather than hydrating half a batch.** A reading whose
// name the pool lost is dropped and counted (`dropped`, proved beside the moving
// pool); a *lookup that failed* is a different thing entirely, because every
// reading in the batch shares the one hydration and a failure there means the
// app knows nothing about any of forty cards somebody photographed. Counting
// those as dropped would report the whole batch as unreadable rather than as
// unanswered.
func TestTheCameraRefusesRatherThanHydratingHalfABatch(t *testing.T) {
	t.Parallel()

	const batch = `{"sightings":[{"set":"lea","number":"269"},{"title":"Sol Ring"}]}`
	path := pooltest.Build(t)

	healthy := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t)})
	status, _, whole := call(t, healthy, "POST", "/api/cards/identify", batch)
	if status != http.StatusOK {
		t.Fatalf("the camera answered %d over a healthy pool: %s", status, whole)
	}

	refused, full := 0, 0
	for budget := 0; budget <= 10; budget++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		a := New(Config{Logger: quietLogger(), Pool: p})
		status, _, raw := call(t, a, "POST", "/api/cards/identify", batch)
		fault.Heal()
		p.Close()
		if status != http.StatusOK {
			refused++
			continue
		}
		if string(raw) != string(whole) {
			t.Errorf("at statement budget %d the camera answered 200 with a "+
				"different reading than the whole one:\n got %s\nwant %s",
				budget, raw, whole)
		}
		full++
	}
	if refused == 0 || full == 0 {
		t.Errorf("%d budgets refused and %d answered in full -- the sweep never "+
			"crossed the hydration", refused, full)
	}
}

// **An import never reports a verdict it could not compute.** The last thing the
// import does inside its lease is ask the pool which of the built deck's cards
// it actually knows, and the gate's whole verdict -- legal or not, how many
// lands, which names are unknown -- is read off that answer. A library that went
// away between building the deck and checking it would leave every card looking
// unknown, which is the import telling somebody their decklist is wrong when
// nothing was wrong with it (commandment 2). So it refuses, and this sweep is
// the proof that it refuses wherever the library stops.
//
// The paste is a dry run, so the sweep writes no deck: what is under test is the
// reading, and a route that created twenty-odd files on the way would be
// measuring the file tier instead.
func TestAnImportRefusesRatherThanJudgingADeckItCouldNotCheck(t *testing.T) {
	t.Parallel()

	// One misspelling, so the shortlist tier runs too -- it is the read *after*
	// the verdict, and it is allowed to come back empty where the verdict is
	// not. Measured in `didyoumean_test.go`.
	const paste = `{"slug":"swept","commander":["Goreclaw, Terror of Qal Sisma"],` +
		`"dry_run":true,"text":"1 Sol Ring\n1 Cultvatr Colosus"}`

	// The library, the app database and the recorder are built once: a dry run
	// writes nothing, so every budget below is reading the same tier, and a rig
	// per budget would be measuring how fast a temp directory can be filled.
	decks := decksDir(t)
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	recorder, err := decklog.NewRecorder(dbPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()
	importer := func(p *pool.Pool) *API {
		return New(Config{Logger: quietLogger(), Pool: p, DecksDir: decks,
			AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: recorder.DB(),
			Recorder: recorder, ClaudeLedger: ledger.RecorderFrom(recorder.DB(), nil)})
	}

	healthy := importer(pooltest.Open(t))
	status, _, whole := callAs(t, healthy, alice, "POST", "/api/decks/import", paste)
	if status != http.StatusOK {
		t.Fatalf("the import answered %d over a healthy pool: %s", status, whole)
	}

	path := pooltest.Build(t)
	refused, full := 0, 0
	for budget := 0; budget <= 24; budget++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		status, _, raw := callAs(t, importer(p), alice, "POST", "/api/decks/import", paste)
		fault.Heal()
		p.Close()
		if status != http.StatusOK {
			refused++
			continue
		}
		if !sameImport(t, raw, whole) {
			t.Errorf("at statement budget %d the import answered 200 with a "+
				"different deck than the whole one:\n got %s\nwant %s",
				budget, raw, whole)
		}
		full++
	}
	if refused == 0 || full == 0 {
		t.Errorf("%d budgets refused and %d answered in full -- the sweep never "+
			"crossed the deck's own pool read", refused, full)
	}
}

// sameImport compares two import answers on everything the verdict is read off,
// leaving the shortlist out: `did_you_mean` is the one key on that payload the
// import is allowed to lose quietly, and `didYouMean`'s own sweep is where that
// is proved.
func sameImport(t *testing.T, got, want []byte) bool {
	t.Helper()
	strip := func(raw []byte) string {
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("an import answer that is not an object: %v (%s)", err, raw)
		}
		delete(payload, "did_you_mean")
		delete(payload, "did_you_mean_skipped")
		flat, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return string(flat)
	}
	return strip(got) == strip(want)
}
