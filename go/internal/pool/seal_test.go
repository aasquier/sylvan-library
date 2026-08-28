package pool_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The seal, and the measurement it exists for.
//
// The admin page's refresh runs **inside the serving process**, which is a
// different problem from `mtglab data refresh` running beside it: DuckDB
// caches its database instance per file within one process, so this process's
// own read-only handle does not make a writer *wait*, it makes a writer
// *fail* — and fail with a message that has nothing to do with locking, so
// none of the patience `OpenWriterWaiting` was built for would ever be spent.
// `TestThisProcessCannotTakeTheWriterWhileItHoldsTheReader` is that fact,
// measured rather than remembered, and it is the whole reason `Pool.Seal`
// exists. If DuckDB ever changes its mind about this, that test says so.

// TestThisProcessCannotTakeTheWriterWhileItHoldsTheReader is the load-bearing
// measurement.
//
// Two claims, and the second is the one that matters: the writer cannot be
// taken, **and the failure is not one `Locked` recognises** — so a caller
// that merely waited would wait for nothing. Sealing is what actually works.
func TestThisProcessCannotTakeTheWriterWhileItHoldsTheReader(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	ctx := context.Background()

	// Open it the way the door does, and keep it open: the lease is ten
	// seconds and this test is over long before it lapses.
	if err := p.Use(ctx, func(c *pool.Conn) error {
		_, err := pool.Count(ctx, c.DB(), "oracle_cards")
		return err
	}); err != nil {
		t.Fatalf("the fixture pool would not open read-only: %v", err)
	}
	if !p.Held() {
		t.Fatal("the pool was not held after a use; this test measures nothing")
	}

	db, err := pool.OpenWriter(ctx, path)
	if err == nil {
		_ = db.Close()
		t.Fatal("the writer opened while this process held the reader — if " +
			"DuckDB now allows this, Pool.Seal is no longer necessary and " +
			"the refresh job can drop it")
	}
	if pool.Locked(err) {
		t.Errorf("the refusal reads as a cross-process lock (%v), which "+
			"OpenWriterWaiting would patiently wait out; in-process it never "+
			"clears on its own", err)
	}

	// Sealed, the same open succeeds — which is the seal's whole job.
	reopen, err := p.Seal(ctx, time.Second)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	defer reopen()
	db, err = pool.OpenWriter(ctx, path)
	if err != nil {
		t.Fatalf("the writer would not open against a sealed pool: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// A sealed pool reads as no pool at all, which is the degraded shape every
// read path in the app already handles — and ADR 6's "card lookups are
// briefly unavailable *during* a refresh ... by design".
func TestASealedPoolAnswersEveryReaderWithNoPool(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	reopen, err := p.Seal(ctx, time.Second)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if p.Held() {
		t.Error("the file is still open after a seal")
	}
	if !p.Sealed() {
		t.Error("a sealed pool does not say so")
	}

	err = p.Use(ctx, func(*pool.Conn) error {
		t.Error("a reader got inside a sealed library")
		return nil
	})
	if !errors.Is(err, pool.ErrNoPool) {
		t.Errorf("a sealed pool answered %v, want ErrNoPool", err)
	}

	// And it comes back. A seal that did not lift would be a library shut for
	// good by one refresh.
	reopen()
	if p.Sealed() {
		t.Error("the seal did not lift")
	}
	var cards int64
	if err := p.Use(ctx, func(c *pool.Conn) error {
		var countErr error
		cards, countErr = pool.Count(ctx, c.DB(), "oracle_cards")
		return countErr
	}); err != nil {
		t.Fatalf("the pool did not come back after the seal lifted: %v", err)
	}
	if cards == 0 {
		t.Error("the pool came back empty")
	}
}

// One rewrite at a time, at the level of the thing being rewritten.
func TestASecondSealIsRefused(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	reopen, err := p.Seal(ctx, time.Second)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	defer reopen()

	if _, err := p.Seal(ctx, time.Second); !errors.Is(err, pool.ErrSealed) {
		t.Errorf("a second seal answered %v, want ErrSealed", err)
	}
}

// Lifting a seal twice is the same as lifting it once. The refresh job lifts
// it from a defer, and a defer that fired after an explicit lift would
// otherwise unseal somebody *else's* refresh.
func TestLiftingASealTwiceIsHarmless(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	reopen, err := p.Seal(ctx, time.Second)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	reopen()
	reopen()
	if p.Sealed() {
		t.Fatal("the pool is sealed after two lifts")
	}

	// A second refresh may now seal it, which is the property the idempotence
	// is protecting.
	again, err := p.Seal(ctx, time.Second)
	if err != nil {
		t.Fatalf("a pool unsealed twice would not seal again: %v", err)
	}
	again()
}

// A reader already inside is waited for, and giving up is a normal outcome
// with a name.
//
// **No sleeps and no timing assumptions**: the reader signals that it is
// holding the lease, the seal is given a budget of zero, and zero is checked
// before the first wait — so the answer is decided by whether a lease is held,
// which the handshake guarantees, and never by how fast the machine is.
func TestASealGivesUpWhileSomebodyIsStillReading(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()

	holding := make(chan struct{})
	letGo := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- p.Use(ctx, func(*pool.Conn) error {
			close(holding)
			<-letGo
			return nil
		})
	}()
	<-holding

	if _, err := p.Seal(ctx, 0); !errors.Is(err, pool.ErrStillReading) {
		t.Errorf("a seal against a held pool answered %v, want ErrStillReading", err)
	}
	// Nothing was sealed, so the reader is unharmed and the next attempt is
	// free to try again. A failed seal that left the flag set would shut the
	// library with no way to open it.
	if p.Sealed() {
		t.Error("a seal that gave up left the library shut")
	}

	close(letGo)
	if err := <-done; err != nil {
		t.Errorf("the reader inside was disturbed: %v", err)
	}

	if reopen, err := p.Seal(ctx, time.Second); err != nil {
		t.Errorf("the pool would not seal once the reader left: %v", err)
	} else {
		reopen()
	}
}

// A seal that is cancelled while it waits reports the cancellation and leaves
// nothing shut.
func TestACancelledSealLeavesTheLibraryOpen(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)

	holding := make(chan struct{})
	letGo := make(chan struct{})
	go func() {
		_ = p.Use(context.Background(), func(*pool.Conn) error {
			close(holding)
			<-letGo
			return nil
		})
	}()
	<-holding

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A generous budget, so what ends this is the cancellation and not the
	// clock: the budget is only consulted after the lease check, and the
	// cancelled context is what the select then finds ready.
	if _, err := p.Seal(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled seal answered %v, want context.Canceled", err)
	}
	if p.Sealed() {
		t.Error("a cancelled seal left the library shut")
	}
	close(letGo)
}

// ---- the whole sequence ----------------------------------------------------

// bothKinds is a stand-in for the bulk index that serves the two files a
// refresh asks for, so `pool.Refresh` is exercised end to end rather than
// half of it.
type bothKinds struct {
	*httptest.Server
	asked map[string]int
}

func newBothKinds(t *testing.T, oracle, printings []map[string]any) *bothKinds {
	t.Helper()
	s := &bothKinds{asked: map[string]int{}}
	bodies := map[string][]byte{
		pool.OracleBulk:    jsonl(t, oracle),
		pool.PrintingsBulk: jsonl(t, printings),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/bulk-data", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
			map[string]any{"type": pool.OracleBulk, "updated_at": "2026-08-24T09:00:00.000+00:00",
				"jsonl_download_uri": s.URL + "/files/" + pool.OracleBulk + ".jsonl"},
			map[string]any{"type": pool.PrintingsBulk, "updated_at": "2026-08-24T09:00:00.000+00:00",
				"jsonl_download_uri": s.URL + "/files/" + pool.PrintingsBulk + ".jsonl"},
		}})
	})
	mux.HandleFunc("/files/", func(w http.ResponseWriter, r *http.Request) {
		kind := filepath.Base(r.URL.Path)
		kind = kind[:len(kind)-len(".jsonl")]
		s.asked[kind]++
		if body, ok := bodies[kind]; ok {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	s.Server = httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func jsonl(t *testing.T, cards []map[string]any) []byte {
	t.Helper()
	var out []byte
	for _, card := range cards {
		raw, err := json.Marshal(card)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw...)
		out = append(out, '\n')
	}
	return out
}

// The command's body, now a function: both halves gathered, both shelved, and
// the watcher told in order.
//
// The order is the assertion rather than the counts alone, because the order
// *is* the contract the CLI's printing depends on — it prints "downloading X"
// before the path and the path before the count, and a sequence that reported
// them in another order would print nonsense.
func TestARefreshGathersBothHalvesAndSaysSoInOrder(t *testing.T) {
	t.Parallel()
	fx := loadRefresh(t)
	oracle := make([]map[string]any, 0, len(fx.Oracle))
	for _, c := range fx.Oracle {
		oracle = append(oracle, c.Raw)
	}
	printings := make([]map[string]any, 0, len(fx.Printings))
	for _, p := range fx.Printings {
		printings = append(printings, p.Raw)
	}
	scryfall := newBothKinds(t, oracle, printings)

	dir := t.TempDir()
	var said []string
	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      filepath.Join(dir, "pool.duckdb"),
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{
		Gathering: func(kind string) { said = append(said, "gathering "+kind) },
		Gathered:  func(kind, _ string) { said = append(said, "gathered "+kind) },
		Shelved:   func(kind string, _ int64) { said = append(said, "shelved "+kind) },
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if counts.Oracle == 0 || counts.Printings == 0 {
		t.Fatalf("a refresh that loaded nothing reported success: %+v", counts)
	}
	want := []string{
		"gathering " + pool.OracleBulk, "gathered " + pool.OracleBulk,
		"shelved " + pool.OracleBulk,
		"gathering " + pool.PrintingsBulk, "gathered " + pool.PrintingsBulk,
		"shelved " + pool.PrintingsBulk,
	}
	if len(said) != len(want) {
		t.Fatalf("the watcher heard %v, want %v", said, want)
	}
	for i := range want {
		if said[i] != want[i] {
			t.Fatalf("the watcher heard %v, want %v", said, want)
		}
	}

	// And the pool it wrote is a real one: opened read-only, it answers the
	// counts the refresh claimed.
	p := pool.New(filepath.Join(dir, "pool.duckdb"), nil)
	t.Cleanup(p.Close)
	var cards int64
	if err := p.Use(context.Background(), func(c *pool.Conn) error {
		var countErr error
		cards, countErr = pool.Count(context.Background(), c.DB(), "oracle_cards")
		return countErr
	}); err != nil {
		t.Fatalf("the refreshed pool would not open: %v", err)
	}
	if cards != counts.Oracle {
		t.Errorf("the refresh claimed %d cards; the pool holds %d", counts.Oracle, cards)
	}
}

// The oracle-only flag stops before the large half, and stopping is not
// failing: the printings already on the shelves are left alone.
func TestAnOracleOnlyRefreshNeverAsksForThePrintings(t *testing.T) {
	t.Parallel()
	fx := loadRefresh(t)
	oracle := make([]map[string]any, 0, len(fx.Oracle))
	for _, c := range fx.Oracle {
		oracle = append(oracle, c.Raw)
	}
	scryfall := newBothKinds(t, oracle, nil)

	dir := t.TempDir()
	counts, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      filepath.Join(dir, "pool.duckdb"),
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    scryfall.URL + "/bulk-data",
		OracleOnly:  true,
	}, pool.RefreshWatcher{})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if counts.Oracle == 0 {
		t.Fatal("an oracle-only refresh loaded no oracle cards")
	}
	if counts.Printings != 0 {
		t.Errorf("an oracle-only refresh reported %d printings", counts.Printings)
	}
	if scryfall.asked[pool.PrintingsBulk] != 0 {
		t.Errorf("an oracle-only refresh downloaded the printings %d times",
			scryfall.asked[pool.PrintingsBulk])
	}
}

// A refresh that cannot take the writer says so with the phase attached, and
// says it *before* asking Scryfall for anything — which is what keeps a
// misconfigured instance from generating the bulk traffic ADR 6 asks us not
// to generate.
func TestARefreshThatCannotTakeTheWriterNeverDownloads(t *testing.T) {
	t.Parallel()
	scryfall := newBothKinds(t, nil, nil)
	dir := t.TempDir()
	// A pool "inside" a regular file. `OpenWriter` makes the directory it is
	// given, so a merely absent one is not a failure at all; a *file* where a
	// directory has to be is one that no amount of waiting fixes — which is
	// the case this test is about.
	blocked := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      filepath.Join(blocked, "pool.duckdb"),
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    scryfall.URL + "/bulk-data",
	}, pool.RefreshWatcher{})
	if err == nil {
		t.Fatal("a refresh with nowhere to write reported success")
	}
	if got := pool.PhaseOf(err); got != pool.PhaseShelves {
		t.Errorf("the failure is phase %q, want %q", got, pool.PhaseShelves)
	}
	if pool.Locked(err) {
		t.Error("a missing directory reads as a lock; a caller would wait for it forever")
	}
	if scryfall.asked[pool.OracleBulk] != 0 {
		t.Error("a refresh that could not write still downloaded the bulk data")
	}
	if _, err := os.Stat(filepath.Join(dir, "scryfall")); err == nil {
		t.Error("a refresh that could not write still made a download shelf")
	}
}

// A gathering that cannot reach the cards is a gather-phase failure, and the
// pool it could not fill is left where it was.
func TestARefreshThatCannotReachTheCardsSaysWhichPhase(t *testing.T) {
	t.Parallel()
	dead := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
	t.Cleanup(dead.Close)

	dir := t.TempDir()
	_, err := pool.Refresh(context.Background(), pool.RefreshOptions{
		DBPath:      filepath.Join(dir, "pool.duckdb"),
		ScryfallDir: filepath.Join(dir, "scryfall"),
		IndexURL:    dead.URL + "/bulk-data",
	}, pool.RefreshWatcher{})
	if err == nil {
		t.Fatal("a refresh against a source that answered nothing reported success")
	}
	if got := pool.PhaseOf(err); got != pool.PhaseGather {
		t.Errorf("the failure is phase %q, want %q", got, pool.PhaseGather)
	}
}
