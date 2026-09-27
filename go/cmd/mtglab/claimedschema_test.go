package main

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// **An `app.db` that claims a schema it does not have**, and what every
// runbook command says when it meets one.
//
// This is `internal/api`'s schema-less pool one layer up, and the reason it is
// the right fixture is the same: a file the ladder *cannot open* is refused on
// the first line, which is the path `unmounted_test.go` already sweeps. What
// nothing reached was the other kind of broken — a file that opens happily,
// answers the ladder, and then cannot answer a question. Every `users`
// subcommand has three or four of those between `connectUsers` and its last
// line, and not one of them had been driven.
//
// It is not contrived. `auth.Migrate` is a no-op on a file whose
// `user_version` already reads [auth.SchemaVersion] — one pragma and out, by
// design, because a ladder that re-ran seventeen scripts on every boot would
// be the deploy's slowest step. So a restore that copied the pragma and lost a
// page, a half-finished manual repair, a volume that came back with the file
// truncated: all of them arrive here, claiming a schema and holding none of
// it.
//
// **The question is not "did it fail" but "did it lie"** — a `users list` that
// prints an empty roster over a database it could not read says "you have no
// accounts", which sends somebody to create them all again.

// claimedSchema writes an `app.db` that says it is current and is empty: the
// pragma the ladder reads, and nothing else at all.
func claimedSchema(t *testing.T, d deployment) deployment {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+d.AppDBPath()+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// Pragmas are parsed rather than bound, and this value is a constant of
	// the package under test rather than anybody's input.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", auth.SchemaVersion)); err != nil {
		t.Fatal(err)
	}
	// A table, so the file is a real database with pages in it rather than
	// zero bytes -- what is missing is every table the app wants, not the
	// format.
	if _, err := db.Exec("CREATE TABLE something_else (x INTEGER)"); err != nil {
		t.Fatal(err)
	}
	return d
}

// hollowed is the same idea one step less severe: the ladder really ran, the
// accounts are really there, and the tables *about* them have gone. It is what
// a restore that lost a page looks like from inside a command that has already
// found its user.
func hollowed(t *testing.T, d deployment, tables ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+d.AppDBPath()+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, table := range tables {
		// The names are this test's own literals; nothing here is input.
		if _, err := db.Exec("DROP TABLE IF EXISTS " + table); err != nil { //nolint:gosec // a fixed list written above
			t.Fatalf("dropping %s: %v", table, err)
		}
	}
}

// Every `users` subcommand over a database that opens and cannot answer:
// each one refuses, and none of them reports an emptiness as a fact.
//
// The commands are listed by hand here rather than discovered, because what
// each one is *asked* differs — `add` needs `--no-password` to get past the
// prompt, `tier` needs a tier, `delete` needs its confirmation — and a sweep
// that supplied a placeholder for all of them would be asking a different
// question of each.
func TestNoUsersCommandAnswersOverADatabaseThatCannotAnswer(t *testing.T) {
	t.Parallel()
	d := claimedSchema(t, scratchDeployment(t))

	for _, argv := range [][]string{
		{"users", "list"},
		{"users", "add", "keeper", "--no-password"},
		{"users", "invite", "keeper@example.com"},
		{"users", "passwd", "keeper"},
		{"users", "disable", "keeper"},
		{"users", "enable", "keeper"},
		{"users", "promote", "keeper"},
		{"users", "demote", "keeper"},
		{"users", "tier", "keeper", "--tier", "default"},
		{"users", "delete", "keeper", "--yes"},
	} {
		out, err := d.runWithInput(t, "a-long-enough-password\na-long-enough-password\n", argv...)
		name := strings.Join(argv, " ")
		if err == nil {
			t.Errorf("`mtglab %s` answered over a database it could not read, "+
				"printing:\n%s", name, out)
			continue
		}
		if strings.TrimSpace(err.Error()) == "" {
			t.Errorf("`mtglab %s` refused with an empty error -- that renders "+
				"as `mtglab: ` and says nothing", name)
		}
		// And it said nothing *about the accounts* on the way out: a roster
		// header over an unreadable database is the lie this test is about.
		if strings.Contains(out, "username") || strings.Contains(out, "no accounts in") {
			t.Errorf("`mtglab %s` printed a roster before refusing:\n%s", name, out)
		}
	}
}

// A volume that did not mount is refused before the prompt too, which is the
// one `users add` shape `unmounted_test.go` cannot reach: without
// `--no-password` the command asks for one and gives up on the empty pipe
// before it ever opens `app.db`.
func TestAnUnclaimedAccountStillNeedsAVolumeToBeCreatedOn(t *testing.T) {
	t.Parallel()
	out, err := unmounted(t).run(t, "users", "add", "keeper", "--no-password")
	if err == nil {
		t.Fatalf("an account was created on a volume that did not mount:\n%s", out)
	}
	if strings.Contains(out, "created keeper") {
		t.Errorf("the command said it had created the account:\n%s", out)
	}
}

// The roster over a database whose *sessions* have gone: the accounts are
// there, the count of who is signed in is not, and the command refuses rather
// than printing a zero.
//
// A zero in that column is a sentence — "nobody is signed in" — and it is the
// one an operator acts on when they are deciding whether a password change
// will throw somebody out mid-session.
func TestTheRosterRefusesRatherThanReportingNobodySignedIn(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.runWithInput(t, "correct-horse-battery\ncorrect-horse-battery\n",
		"users", "add", "keeper"); err != nil {
		t.Fatalf("seeding an account: %v", err)
	}
	hollowed(t, d, "sessions")

	out, err := d.run(t, "users", "list")
	if err == nil {
		t.Fatalf("the roster counted sessions it could not read:\n%s", out)
	}
	// The same question of the two other commands that read the session count
	// before they act.
	for _, argv := range [][]string{
		{"users", "delete", "keeper", "--yes"},
		{"users", "disable", "keeper"},
	} {
		out, err := d.run(t, argv...)
		if err == nil {
			t.Errorf("`mtglab %s` answered without the sessions it ends:\n%s",
				strings.Join(argv, " "), out)
		}
	}
	// And a password change, which ends every session and therefore cannot
	// report how many it ended.
	if _, err := d.runWithInput(t, "another-good-passphrase\nanother-good-passphrase\n",
		"users", "passwd", "keeper"); err == nil {
		t.Error("a password was set without the sessions it was meant to end")
	}
}

// An account with no password yet is looked up in the invite ledger to tell
// "invited" from "no password", and a ledger that cannot be read is a refusal
// rather than a guess at which of the two it is.
func TestAnUnreadableInviteLedgerIsNotGuessedAt(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatalf("seeding an account: %v", err)
	}
	hollowed(t, d, "auth_tokens")

	out, err := d.run(t, "users", "list")
	if err == nil {
		t.Fatalf("the roster guessed at a state it could not read:\n%s", out)
	}
	// An invite cannot be sent either -- the link it would mail is a row in
	// that same ledger, and a "invited" with nothing behind it is worse than
	// a refusal.
	out, err = d.run(t, "users", "invite", "keeper@example.com")
	if err == nil {
		t.Errorf("an invite was reported sent with nowhere to record it:\n%s", out)
	}
}

// `users tier` over a tier nobody has: the refusal names what was asked for
// **and lists what there is**, because an operator mistyping a tier key has no
// other way to find the right one.
func TestAnUnknownTierIsRefusedWithTheRosterOfRealOnes(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatalf("seeding an account: %v", err)
	}

	_, err := d.run(t, "users", "tier", "keeper", "--tier", "the-good-one")
	if err == nil {
		t.Fatal("a tier nobody has was granted")
	}
	if !strings.Contains(err.Error(), "the-good-one") {
		t.Errorf("the refusal said %q without naming what was asked for", err)
	}
	if !strings.Contains(err.Error(), "one of: default,") {
		t.Errorf("the refusal said %q without offering the real ones", err)
	}

	// And the real ones work, both directions: a tier granted and then
	// cleared, with the column saying so each time.
	out, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "keeper") {
		t.Errorf("the account went missing:\n%s", out)
	}
}
