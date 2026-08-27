package door

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// A stopped door hands the card pool back, and until now it was the one thing
// it opened and kept.
//
// **The asymmetry is the bug.** [Door.Close] flushed the visitor ledger and
// closed `app.db` and left the DuckDB handle open — and `cmd/mtglab`'s `ui`
// builds that pool inside the `door.Config` literal, keeping no reference of
// its own, so nothing else in the process could ever have closed it. Every
// other command that opens a pool defers a `Close`; the serving path, which
// holds one longest, did not.
//
// **The pool has to be genuinely open for this to mean anything**, which is why
// it leases one first. [pool.Pool.Held] is false on a pool nobody has asked
// anything of, so a test that skipped the lease would pass identically against
// the bug and against the fix — the same shape as a job test that cannot tell a
// short-circuit from a fast worker. The lease is what makes the assertion able
// to fail.
func TestAStoppedDoorHandsTheCardPoolBack(t *testing.T) {
	t.Parallel()
	web, tarot := site(t)
	p := pool.New(pooltest.Build(t), nil)
	d, err := New(Config{WebDist: web, TarotDir: tarot, Pool: p,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Use(context.Background(), func(*pool.Conn) error {
		return nil
	}); err != nil {
		t.Fatalf("leasing the tiny pool: %v", err)
	}
	if !p.Held() {
		t.Fatal("the pool is not open after a lease, so this test cannot " +
			"tell a door that closes it from one that does not")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("closing the door: %v", err)
	}
	if p.Held() {
		t.Error("the door stopped and the DuckDB handle is still open; " +
			"nothing else in the serving process holds this pool, so it " +
			"stays open until the process dies")
	}
}
