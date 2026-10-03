package main

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// The `users` family's remaining faults: a column that is gone, a table that
// refuses the write, a row that vanishes under the command, and the sessions
// every one of them ends.
//
// `claimedschema_test.go` is the severe version of this -- a file that answers
// the ladder and holds nothing -- and it refuses at each command's *first*
// read. What it cannot reach is the half-broken database, where the lookup
// answers and the question after it does not, which is where every one of these
// commands keeps its remaining `return err`. The levers are both in
// docs/polish/COVERAGE.md: a schema older than the binary, one table at a time
// (16), and a database that refuses one write (14).
//
// **The question is never "did it fail" but "did it lie"**: an account reported
// promoted over a database that could not read the admins back, a tier reported
// granted over an update that was refused, an account reported gone that is
// still there. Each assertion is the pair -- an error came back, and the
// command did not also print the sentence that would have been a lie.

// appdb opens this deployment's `app.db` for a fixture's own surgery, and
// closes it again: the commands under test open their own handles, and a
// fixture that kept one would be racing them for the write lock.
func appdb(t *testing.T, d deployment) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+d.AppDBPath()+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// runSQL runs one of this test file's own literal statements against `app.db`.
// Nothing here is anybody's input.
func runSQL(t *testing.T, d deployment, statements ...string) {
	t.Helper()
	db := appdb(t, d)
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// columnRemoved rebuilds `users` without its `password_hash` column: the
// accounts are all there, every read that names them answers, and the one
// column three separate questions depend on has gone.
//
// It is the shape a restore that lost a page leaves behind, and the reason it
// is worth building rather than closing the handle is that it fails at a
// *named* read: `userColumns` does not mention `password_hash`, so the lookup
// every command starts with still succeeds and the failure lands exactly where
// the account's claim is asked about. A dropped column cannot be asked for
// with `ALTER TABLE` here (the schema's own index refuses it), so the table is
// rebuilt, dropped and renamed -- lever 16's recipe.
func columnRemoved(t *testing.T, d deployment) {
	t.Helper()
	runSQL(t, d,
		"CREATE TABLE users_rebuilt AS SELECT id, username, email, is_admin,"+
			" disabled_at, created_at, model_tier FROM users",
		"DROP TABLE users",
		"ALTER TABLE users_rebuilt RENAME TO users")
}

// signedIn gives an account that many live sessions, so the commands that end
// sessions have something to end and a number to report.
func signedIn(t *testing.T, d deployment, username string, sessions int) {
	t.Helper()
	db := auth.OpenReadWrite(d.AppDBPath())
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	user, err := auth.Get(ctx, db, username)
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatalf("no account %q to sign in", username)
	}
	for range sessions {
		if _, err := auth.CreateSession(ctx, db, user.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// Every command that ends sessions says how many it ended.
//
// The count is the one fact an operator is deciding on: "the password is
// changed" and "and three devices have to sign in again" are different
// sentences, and the second is the one that decides whether this is done now
// or after the game. Each of the three was only ever driven against an account
// nobody was signed in to, so the line that reports the number -- in all three
// -- had never run.
func TestEverySessionEndedIsCounted(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")

	// A password change ends every session the account had.
	signedIn(t, d, "player", 2)
	out, err := d.runWithInput(t, "a-longer-passphrase\na-longer-passphrase\n",
		"users", "passwd", "player")
	if err != nil {
		t.Fatalf("passwd: %v", err)
	}
	if !strings.Contains(out, "2 session(s) ended") {
		t.Errorf("a password change over two live sessions said:\n%s", out)
	}

	// Disabling does too, and says so on its own line.
	signedIn(t, d, "player", 1)
	out, err = d.run(t, "users", "disable", "player")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !strings.Contains(out, "1 session(s) ended") {
		t.Errorf("disabling an account somebody was signed in to said:\n%s", out)
	}

	// And a delete, which is the one where the number cannot be checked
	// afterwards by anybody.
	if _, err := d.run(t, "users", "enable", "player"); err != nil {
		t.Fatal(err)
	}
	signedIn(t, d, "player", 3)
	out, err = d.run(t, "users", "delete", "player", "--yes")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, "3 session(s) ended") {
		t.Errorf("deleting an account with three live sessions said:\n%s", out)
	}
}

// Every `users` command that asks whether an account has claimed its password
// refuses when it cannot find out, and none of them guesses.
//
// Three different questions rest on that one column -- "is this account
// active, invited or unclaimed", "has this address already been claimed", and
// "how many admins can actually sign in" -- and each has a plausible wrong
// answer that reads as a fact. An account shown as `no password` that really
// has one sends somebody to reset it; an invite sent to an address that is
// already claimed is a second account for one person; `0 admin(s) can sign in`
// over a readable database is how somebody decides to promote themselves and
// locks the instance.
func TestNoAccountCommandGuessesAtAClaimThatCannotBeRead(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")
	if _, err := d.run(t, "users", "invite", "visitor@example.com"); err != nil {
		t.Fatalf("seeding an invite: %v", err)
	}
	columnRemoved(t, d)

	// The roster, which reports the state of every account.
	out, err := d.run(t, "users", "list")
	if err == nil {
		t.Errorf("the roster reported states it could not read:\n%s", out)
	} else if strings.Contains(out, "no password") || strings.Contains(out, "active") {
		t.Errorf("the roster printed a state before refusing:\n%s", out)
	}

	// An invite to an address somebody already holds: the claim decides
	// whether this is a refusal pointing at the reset link or a new account.
	out, err = d.run(t, "users", "invite", "visitor@example.com")
	if err == nil {
		t.Errorf("an invite was sent without knowing whether the address was "+
			"already claimed:\n%s", out)
	} else if strings.Contains(out, "invited") {
		t.Errorf("the invite said it had been sent:\n%s", out)
	}

	// And an invite to a new address, which has to write the account.
	out, err = d.run(t, "users", "invite", "stranger@example.com")
	if err == nil {
		t.Errorf("an account was created in a table that cannot hold one:\n%s", out)
	} else if strings.Contains(out, "invited") {
		t.Errorf("the invite said it had been sent:\n%s", out)
	}

	// A promotion, whose last two lines are both about the claim: how many
	// admins can sign in, and whether this one can.
	out, err = d.run(t, "users", "promote", "player")
	if err == nil {
		t.Errorf("a promotion counted admins it could not read:\n%s", out)
	} else if strings.Contains(out, "admin(s) can sign in") {
		t.Errorf("the promotion printed an admin count before refusing:\n%s", out)
	}

	// And a delete, whose lockout guard is the same count.
	out, err = d.run(t, "users", "delete", "player", "--yes")
	if err == nil {
		t.Errorf("an account was deleted without the guard that reads the "+
			"admins:\n%s", out)
	} else if strings.Contains(out, "is gone") {
		t.Errorf("the delete said the account was gone:\n%s", out)
	}
}

// A write the database refuses is not reported as done.
//
// A `BEFORE UPDATE` trigger that raises is what a closed handle cannot be: the
// command's own lookups all answer, and only the write fails -- which is the
// state a disk that filled between the read and the write leaves, and the one
// where a command that printed its success line anyway would be telling an
// operator the opposite of what happened.
func TestAWriteTheDatabaseRefusesIsNotReportedAsDone(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")
	runSQL(t, d, "CREATE TRIGGER no_account_changes BEFORE UPDATE ON users"+
		" BEGIN SELECT raise(ABORT, 'the accounts are read-only'); END")

	out, err := d.run(t, "users", "promote", "player")
	if err == nil {
		t.Errorf("a promotion the database refused was reported as done:\n%s", out)
	} else if strings.Contains(out, "is now an admin") {
		t.Errorf("the promotion printed its success line:\n%s", out)
	}

	out, err = d.run(t, "users", "tier", "player", "--tier", "opus")
	if err == nil {
		t.Errorf("a tier the database refused was reported as granted:\n%s", out)
	} else if strings.Contains(out, "is answered by") {
		t.Errorf("the grant printed which Claude answers them:\n%s", out)
	}
	// The refusal is the database's, not the roster's: a real tier was asked
	// for, so this is not the "no such tier" arm wearing the same words.
	if err != nil && strings.Contains(err.Error(), "one of: default,") {
		t.Errorf("a refused write was reported as an unknown tier: %v", err)
	}

	// And a delete the same trigger's sibling refuses.
	runSQL(t, d, "CREATE TRIGGER no_account_removals BEFORE DELETE ON users"+
		" BEGIN SELECT raise(ABORT, 'the accounts are read-only'); END")
	out, err = d.run(t, "users", "delete", "player", "--yes")
	if err == nil {
		t.Errorf("an account the database would not remove was reported "+
			"gone:\n%s", out)
	} else if strings.Contains(out, "is gone") {
		t.Errorf("the delete printed its success line:\n%s", out)
	}
	// It really is still there.
	listed, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "player") {
		t.Errorf("the account went after all:\n%s", listed)
	}
}

// An account that goes away between the write and the read back is a refusal
// rather than a half-reported success.
//
// The promotion writes, then asks two questions about what it wrote: how many
// admins can sign in, and whether this one can. Between the write and those
// reads another administrator may have deleted the account -- `fly ssh console`
// in one window and the admin page in another is two writers on one `app.db` --
// and the trigger here is that race made deterministic. The command must not
// print `is now an admin` about a row that is not there.
func TestAPromotionOfAnAccountThatVanishesIsNotReportedAsDone(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")
	runSQL(t, d, "CREATE TRIGGER gone_after_the_write AFTER UPDATE ON users"+
		" BEGIN DELETE FROM users WHERE id = NEW.id; END")

	out, err := d.run(t, "users", "promote", "player")
	if err == nil {
		t.Fatalf("a promotion of an account that had gone was reported as "+
			"done:\n%s", out)
	}
	if strings.Contains(out, "admin(s) can sign in") {
		t.Errorf("the promotion printed a count about an account that had "+
			"gone:\n%s", out)
	}
}
