package auth

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// The last statement in each of the shorter writes.
//
// Everything here is the same shape as `halfwritten_test.go` and is split out
// only because it reads as a list rather than as an argument: one call, one
// budget, one assertion that the answer is a refusal. The value is not in any
// single line — it is that **every** write in this package now has something
// standing behind its second statement, so a swallowed error anywhere in the
// file is a failing test rather than a quiet success.

func TestTheStatementAfterTheFirstSaysSoWhenItGoes(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	user := oneAccount(t, db)
	session, err := CreateSession(ctx, db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	key := AddressKey("203.0.113.11", "login")
	sender := &countingSender{}

	for _, tc := range []struct {
		name string
		// budget is how many statements land before the one this is about.
		budget int
		call   func() error
	}{
		// A fresh link clears the old ones before it writes the new one, so
		// the DELETE is the first statement inside the transaction.
		{"the link that clears the last one", 1, func() error {
			_, e := IssueToken(ctx, db, user.ID, PurposeInvite)
			return e
		}},
		{"the link purge", 1, func() error {
			_, e := PurgeExpiredTokens(ctx, db, KeepUsedTokensFor)
			return e
		}},
		{"the session purge", 1, func() error {
			_, e := PurgeExpiredSessions(ctx, db)
			return e
		}},
		{"revoking an account's sessions", 1, func() error {
			_, e := DeleteSessionsForUser(ctx, db, user.ID)
			return e
		}},
		// The window is read first and then written, so the read lands and
		// the count does not.
		{"counting a failed attempt", 1, func() error {
			_, e := RecordFailure(ctx, db, key, PerAddress)
			return e
		}},
		{"forgetting a key's failures", 1, func() error {
			return ClearLimit(ctx, db, key)
		}},
		{"the rate-limit purge", 1, func() error {
			_, e := PurgeStaleLimits(ctx, db, KeepLimitsFor)
			return e
		}},
		// The row is read and then touched; the touch is the second.
		{"the serving resolver's touch", 0, func() error {
			_, e := LookupTouching(ctx, db, session)
			return e
		}},
		// An account is inserted and then read back, because the caller is
		// owed the row rather than the id.
		{"reading back a new account", 1, func() error {
			_, e := Create(ctx, db, "another", "another@example.test", false)
			return e
		}},
		// The address resolves and the link it needs does not write.
		{"the reset nobody can be sent", 1, func() error {
			return SendReset(ctx, db, "squire@example.test", sender, "https://example.test")
		}},
	} {
		fault.Heal()
		fault.After(tc.budget)
		err := tc.call()
		fault.Heal()
		if err == nil {
			t.Errorf("%s: survived a database with %d statements left in it",
				tc.name, tc.budget)
		}
	}
	if sender.sent != 0 {
		t.Fatalf("%d messages went out with no link behind them", sender.sent)
	}
}

// The account is claimed and then read back — and the read back is where the
// caller's row comes from, so a failure there cannot come back as a claimed
// account with nothing in it.
func TestALinkRedeemedOverALastFailedReadHandsBackNoAccount(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	user, err := Create(ctx, db, "invited", "invited@example.test", false)
	if err != nil {
		t.Fatal(err)
	}
	token, err := IssueToken(ctx, db, user.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	// The lookup, the account read, the BEGIN, the token UPDATE, the password
	// UPDATE, the session DELETE, the COMMIT — and then the read back.
	fault.After(7)
	claimed, err := RedeemToken(ctx, db, token, "a long enough passphrase", PurposeInvite, "")
	fault.Heal()
	if err == nil {
		t.Fatal("a redemption whose final read failed reported success")
	}
	if claimed != nil {
		t.Fatalf("a failed redemption handed back %+v", claimed)
	}
	// The write itself did land — this is the one shape where the link is
	// spent and the caller is told no, and it is right: the password is set,
	// so they can sign in rather than redeem again.
	if _, err := LookupToken(ctx, db, token, PurposeInvite); !errors.Is(err, ErrTokenUsed) {
		t.Fatalf("the link is not spent after the transaction committed: %v", err)
	}
	who, err := Authenticate(ctx, db, "invited", "a long enough passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if who == nil {
		t.Fatal("the password the committed transaction set does not work")
	}
}

// A backup refuses to overwrite. The file it would write over is the one
// somebody would restore from, and `mtglab` runs this right before a
// migration — so "move or remove it first" is the whole safety of the step.
func TestABackupRefusesToOverwriteWhatIsAlreadyThere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	dest := filepath.Join(dir, "backup.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(context.Background(), path, dest); err != nil {
		t.Fatal(err)
	}
	_, err := Backup(context.Background(), path, dest)
	if err == nil {
		t.Fatal("a second backup wrote over the first")
	}
	if !strings.Contains(err.Error(), "move or remove it first") {
		t.Fatalf("the refusal does not say what to do: %q", err)
	}
}

// A read that cannot read is never an empty answer. These four are the reads
// whose zero value is a sentence — "no password", "no admins", "nobody by
// that name" — and each of them is a sentence the instance would act on.
func TestTheAccountReadsRefuseRatherThanAnsweringEmptily(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	user := oneAccount(t, db)

	fault.After(0)
	defer fault.Heal()
	if has, err := HasPassword(ctx, db, user.ID); err == nil || has {
		t.Errorf("HasPassword answered %v, %v over a database that had gone", has, err)
	}
	if n, err := CountSessionsForUser(ctx, db, user.ID); err == nil || n != 0 {
		t.Errorf("CountSessionsForUser answered %d, %v", n, err)
	}
	if out, err := TokenOutstanding(ctx, db, user.ID, PurposeInvite); err == nil || out {
		t.Errorf("TokenOutstanding answered %v, %v", out, err)
	}
	if err := Ping(ctx, db); err == nil {
		t.Error("Ping answered a database that had gone")
	}
	if err := PingWritable(ctx, db); err == nil {
		t.Error("PingWritable answered a database that had gone")
	}
}
