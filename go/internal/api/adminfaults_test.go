package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// The admin surface against a database that is *half* there, and against a
// mail provider that took the message and failed.
//
// `closeddb_test.go` takes the whole handle away at once, which is the volume
// detaching; this is the finer fault and the commoner one — a restore that
// replayed some tables and not others, a ladder rung that half applied. Every
// query in the activity view ends `if err != nil` and a working database never
// goes near any of them, so the branch that decides whether an admin is told
// "I cannot read this" or shown a healthy-looking instance with nothing in it
// had never been taken one table at a time.
//
// The assertion is the one `closeddb_test.go` argues: not the message —
// SQLite's wording is not ours — but that the answer is **readable and not a
// lie**. A 200 over a dropped `sessions` table reads as "nobody has ever
// signed in", which is the sentence one person would act on.

// dropping is an admin API over a scratch database with one table removed.
func dropping(t *testing.T, table string) *accountRig {
	t.Helper()
	rig := newAccountRig(t, true)
	if _, err := rig.db.Exec("DROP TABLE " + table); err != nil {
		rig.close()
		t.Fatalf("dropping %s: %v", table, err)
	}
	return rig
}

// The activity view, one missing table at a time. Each of these is a
// different `if err != nil` inside it, and the route must refuse on every one.
func TestTheActivityViewRefusesEachHalfOfADatabaseThatWillNotAnswer(t *testing.T) {
	t.Parallel()
	for _, table := range []string{"users", "auth_tokens", "sessions",
		"sim_cache", "deck_log"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			rig := dropping(t, table)
			defer rig.close()

			rec := rig.call(t, adminScope, "GET", "/api/admin/stats/activity", "", "")
			if rec.Code == http.StatusOK {
				t.Fatalf("a missing %s table answered 200: %s", table, rec.Body)
			}
			if rec.Body.Len() == 0 {
				t.Fatalf("a missing %s table answered %d with no body", table, rec.Code)
			}
			if said := detail(t, rec); strings.TrimSpace(said) == "" {
				t.Fatalf("a missing %s table answered %d with nothing to read: %s",
					table, rec.Code, rec.Body)
			}
			// And the refusal is the room's, not the driver's: an admin is
			// still a user (commandment 10).
			for _, leak := range []string{"SELECT", "sqlite", "SQL"} {
				if strings.Contains(detail(t, rec), leak) {
					t.Errorf("the refusal carries %q: %q", leak, detail(t, rec))
				}
			}
		})
	}
}

// The schema reading beside the stats: a fact about the volume, answered as
// absent whenever it cannot be had rather than as a version that is not true.
//
// Three ways it cannot be had, and all three answer the same way on purpose —
// the number renders beside the storage figures, and a wrong one would be read
// as a ladder rung that has not run.
func TestTheSchemaReadingIsAbsentRatherThanWrong(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// No database path at all: an instance with no app.db.
	if got := New(Config{}).schemaApplied(); got != nil {
		t.Errorf("an instance with no database reports schema %v", got)
	}
	// A path that names nothing.
	absent := New(Config{AppDBPath: filepath.Join(dir, "never-written.db")})
	if got := absent.schemaApplied(); got != nil {
		t.Errorf("a missing file reports schema %v", got)
	}
	// A file that is there and is not a database -- half a restore, or a
	// download that landed on the wrong name.
	notADB := filepath.Join(dir, "app.db")
	if err := os.WriteFile(notADB, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := New(Config{AppDBPath: notADB}).schemaApplied(); got != nil {
		t.Errorf("a file that is not a database reports schema %v", got)
	}

	// And the real one still answers a number, so the three nils above are
	// about the fault and not about the reading being broken.
	rig := newAccountRig(t, true)
	defer rig.close()
	version, ok := rig.api.schemaApplied().(int)
	if !ok || version <= 0 {
		t.Errorf("a real database reports schema %v", rig.api.schemaApplied())
	}
}

// refusingSender is a mail provider that took the message and failed --
// a key that has been revoked, a provider that is down, a domain that has
// stopped verifying.
type refusingSender struct{ tried int }

func (s *refusingSender) Send(auth.Message) error {
	s.tried++
	return errors.New("the provider would not take it")
}

// An invite whose account was created and whose link did not go out says so
// plainly, because the fix is to press invite again — which this endpoint
// supports — rather than to wonder whether half of something happened.
func TestAnInviteWhoseMailFailedSaysTheAccountExistsAnyway(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	sender := &refusingSender{}
	rig.api.email = sender

	rec := rig.call(t, adminScope, "POST", "/api/admin/users",
		`{"email":"unlucky@example.com","username":"unlucky"}`, "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("a failed send answered %d: %s", rec.Code, rec.Body)
	}
	if sender.tried != 1 {
		t.Errorf("the provider was asked %d times", sender.tried)
	}
	said := detail(t, rec)
	if !strings.Contains(said, "the account exists") {
		t.Errorf("the refusal reads %q and never says the account is there", said)
	}

	// And it is: the half that happened happened, which is the whole reason
	// the sentence says so.
	account, err := auth.Get(context.Background(), rig.db, "unlucky")
	if err != nil {
		t.Fatal(err)
	}
	if account == nil {
		t.Fatal("the 502 said the account exists and it does not")
	}

	// Pressing invite again once mail works is the documented fix, so it has
	// to actually work: the same address, a sender that takes it, a 201.
	rig.api.email = rig.sender
	rec = rig.call(t, adminScope, "POST", "/api/admin/users",
		`{"email":"unlucky@example.com"}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("the retry answered %d: %s", rec.Code, rec.Body)
	}
	if len(rig.sender.messages()) != 1 {
		t.Errorf("the retry sent %d messages", len(rig.sender.messages()))
	}
}

// An invite against a database whose token table has gone: the account cannot
// be told apart from one that is already claimed, so nothing is created and
// nothing is sent. What must not happen is a 201 over a write that failed.
func TestAnInviteOverAHalfMissingDatabaseCreatesNothingAndSendsNothing(t *testing.T) {
	t.Parallel()
	rig := dropping(t, "auth_tokens")
	defer rig.close()

	rec := rig.call(t, adminScope, "POST", "/api/admin/users",
		`{"email":"nobody@example.com","username":"nobody"}`, "")
	if rec.Code == http.StatusCreated {
		t.Fatalf("an invite answered 201 over a database that will not answer: %s",
			rec.Body)
	}
	if strings.TrimSpace(detail(t, rec)) == "" {
		t.Fatalf("the refusal carries nothing a person could read: %s", rec.Body)
	}
	if len(rig.sender.messages()) != 0 {
		t.Errorf("%d messages went out for an invite that was refused",
			len(rig.sender.messages()))
	}
}

// The roster, likewise. An admin looking at an empty list over a database
// that cannot be read has been told the instance has no accounts, which is
// the one answer that is worse than an error.
func TestTheRosterRefusesRatherThanReportingNobody(t *testing.T) {
	t.Parallel()
	rig := dropping(t, "auth_tokens")
	defer rig.close()

	rec := rig.call(t, adminScope, "GET", "/api/admin/users", "", "")
	if rec.Code == http.StatusOK {
		t.Fatalf("the roster answered 200 over a database that will not answer: %s",
			rec.Body)
	}
	if strings.TrimSpace(detail(t, rec)) == "" {
		t.Fatalf("the refusal carries nothing a person could read: %s", rec.Body)
	}
}

// A patch whose write lands and whose read-back cannot be built.
//
// `waiting` is the unclaimed account, and an unclaimed account's *state* is
// the one thing that has to ask the token table — so with that table gone the
// grant succeeds and the row the route answers with cannot be assembled. It
// must refuse rather than answer a half-built body: a roster row with a
// missing state renders as a blank cell beside a real account, which reads as
// an account in no state at all.
func TestAPatchThatCannotBuildItsAnswerRefusesRatherThanHalfSayingIt(t *testing.T) {
	t.Parallel()
	rig := dropping(t, "auth_tokens")
	defer rig.close()

	rec := rig.call(t, adminScope, "PATCH", "/api/admin/users/waiting",
		`{"is_admin":true}`, "")
	if rec.Code == http.StatusOK {
		t.Fatalf("a patch answered 200 with a body it could not assemble: %s",
			rec.Body)
	}
	if strings.TrimSpace(detail(t, rec)) == "" {
		t.Fatalf("the refusal carries nothing a person could read: %s", rec.Body)
	}
	// The write itself did land, which is exactly why the refusal must not
	// pretend otherwise by being silent about it -- the next roster read will
	// show the grant.
	account, err := auth.Get(context.Background(), rig.db, "waiting")
	if err != nil {
		t.Fatal(err)
	}
	if account == nil || !account.IsAdmin {
		t.Error("the grant was reported as failed and also not made")
	}
}
