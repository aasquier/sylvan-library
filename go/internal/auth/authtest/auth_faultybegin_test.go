package authtest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture forwards the driver's own refusal to begin, rather than
// swallowing it.
//
// `faultyConn.BeginTx` has two ways to answer no: the budget, which every test
// standing on this fixture uses, and the driver saying no on its own. The
// second one matters because it is the shape of the live fault
// `internal/auth`'s `discard` exists for: a connection carrying a transaction
// nobody closed refuses the next BEGIN with "cannot start a transaction within
// a transaction", and a fixture that quietly answered a transaction there
// would hide exactly the state those tests are about.
//
// A hand-written BEGIN on a pinned connection is how the app's own `exclusive`
// opens an immediate transaction, so this is not a contrived way in: it is the
// first half of what that function does, with the second half left undone.
func TestTheFaultyHandleForwardsADriverThatWillNotBegin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, fault, err := OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// One connection wide, so this is *the* connection, exactly as the app's
	// write handle is.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}

	// The budget is untouched -- nothing is being starved here -- so a
	// transaction handed back would be the fixture inventing one.
	tx, err := conn.BeginTx(ctx, nil)
	if err == nil {
		_ = tx.Rollback()
		t.Fatal("a transaction was begun inside a transaction")
	}
	if strings.Contains(err.Error(), ErrGoneAway.Error()) {
		t.Errorf("the refusal was the fixture's own budget rather than the "+
			"driver's: %v", err)
	}

	// And the fault is still armed for whoever asks next: refusing to begin
	// must not spend anything.
	fault.After(0)
	if _, err := conn.ExecContext(ctx, "ROLLBACK"); err == nil {
		t.Error("an armed fault answered a statement")
	}
}
