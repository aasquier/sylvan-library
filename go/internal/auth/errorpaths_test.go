package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/config"
)

// The error paths, swept.
//
// Almost every function in this package ends in the same two lines -- run a
// statement, return the error -- and almost none of those returns had ever
// been taken by a test. That is the branch that matters most and is easiest
// to get wrong: a swallowed error here does not crash, it **succeeds
// wrongly**. `SetDisabled` that ignored its UPDATE reports the account is off
// while it is still on; `HasPassword` that ignored its SELECT reports "no
// password" for somebody who has one, which is an invite the instance will
// happily re-send.
//
// A closed handle is the cheapest way to make every one of those statements
// fail, and it is a real failure rather than a contrived one: `app.db` lives
// on a network volume, and a handle whose file went away mid-request is
// exactly this.
//
// What is asserted is not the message -- SQLite's wording is not ours -- but
// that **an error comes back at all**, and that nothing claims success.

// closedDB is a real, fully-migrated app.db whose handle has been closed, so
// every statement against it fails at the driver.
func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	seed, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(appSchema(t)); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return db
}

// Every read says so when it cannot read. The pairing matters: a read that
// returns `(zero, nil)` on failure is indistinguishable from a real answer,
// so each of these is checked for an error *and* for not having invented one.
func TestEveryReadSaysSoWhenItCannotRead(t *testing.T) {
	t.Parallel()
	db := closedDB(t)
	ctx := context.Background()

	t.Run("Get", func(t *testing.T) {
		t.Parallel()
		user, err := Get(ctx, db, "alice")
		if err == nil {
			t.Fatalf("a dead handle answered %v", user)
		}
		if user != nil {
			t.Errorf("a failed lookup also returned %v", user)
		}
	})

	t.Run("GetByEmail", func(t *testing.T) {
		t.Parallel()
		user, err := GetByEmail(ctx, db, "alice@example.com")
		if err == nil {
			t.Fatalf("a dead handle answered %v", user)
		}
		if user != nil {
			t.Errorf("a failed lookup also returned %v", user)
		}
	})

	t.Run("AllUsers", func(t *testing.T) {
		t.Parallel()
		users, err := AllUsers(ctx, db)
		if err == nil {
			t.Fatalf("a dead handle listed %d users", len(users))
		}
	})

	t.Run("UsableAdminIDs", func(t *testing.T) {
		t.Parallel()
		// The lockout guard reads this. If it answered "no admins" on a
		// failure, every last-admin check would pass and the guard would be
		// gone exactly when the database is unwell.
		ids, err := UsableAdminIDs(ctx, db)
		if err == nil {
			t.Fatalf("a dead handle counted %v admins", ids)
		}
	})

	t.Run("HasPassword", func(t *testing.T) {
		t.Parallel()
		// A false here is an invite the instance re-sends to somebody who
		// already claimed their account.
		has, err := HasPassword(ctx, db, 1)
		if err == nil {
			t.Fatalf("a dead handle answered has=%v", has)
		}
	})

	t.Run("TokenOutstanding", func(t *testing.T) {
		t.Parallel()
		out, err := TokenOutstanding(ctx, db, 1, PurposeInvite)
		if err == nil {
			t.Fatalf("a dead handle answered %v", out)
		}
	})

	t.Run("CountSessionsForUser", func(t *testing.T) {
		t.Parallel()
		n, err := CountSessionsForUser(ctx, db, 1)
		if err == nil {
			t.Fatalf("a dead handle counted %d sessions", n)
		}
	})

	t.Run("LookupToken", func(t *testing.T) {
		t.Parallel()
		tok, err := LookupToken(ctx, db, "whatever", PurposeInvite)
		if err == nil {
			t.Fatalf("a dead handle answered %v", tok)
		}
	})

	t.Run("Authenticate", func(t *testing.T) {
		t.Parallel()
		// The one that must never answer "yes" by accident.
		user, err := Authenticate(ctx, db, "alice", "hunter2hunter2")
		if err == nil && user != nil {
			t.Fatal("a dead handle authenticated somebody")
		}
	})
}

// Every write says so when it cannot write, and none of them reports how much
// it changed on a failure.
func TestEveryWriteSaysSoWhenItCannotWrite(t *testing.T) {
	t.Parallel()
	db := closedDB(t)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		t.Parallel()
		user, err := Create(ctx, db, "alice", "alice@example.com", false)
		if err == nil {
			t.Fatalf("a dead handle created %v", user)
		}
		if user != nil {
			t.Errorf("a failed create returned %v", user)
		}
	})

	t.Run("SetPassword", func(t *testing.T) {
		t.Parallel()
		ended, err := SetPassword(ctx, db, 1, "hunter2hunter2")
		if err == nil {
			t.Fatal("a dead handle set a password")
		}
		if ended != 0 {
			t.Errorf("a failed write claimed it ended %d sessions", ended)
		}
	})

	t.Run("SetDisabled", func(t *testing.T) {
		t.Parallel()
		ended, err := SetDisabled(ctx, db, 1, true)
		if err == nil {
			t.Fatal("a dead handle disabled an account")
		}
		if ended != 0 {
			t.Errorf("a failed write claimed it ended %d sessions", ended)
		}
	})

	t.Run("SetAdmin", func(t *testing.T) {
		t.Parallel()
		if err := SetAdmin(ctx, db, 1, true); err == nil {
			t.Fatal("a dead handle granted admin")
		}
	})

	t.Run("SetModelTier", func(t *testing.T) {
		t.Parallel()
		if err := SetModelTier(ctx, db, 1, "standard"); err == nil {
			t.Fatal("a dead handle set a tier")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		t.Parallel()
		if n, err := Delete(ctx, db, 1); err == nil {
			t.Fatalf("a dead handle deleted %d accounts", n)
		}
	})

	t.Run("CreateSession", func(t *testing.T) {
		t.Parallel()
		token, err := CreateSession(ctx, db, 1)
		if err == nil {
			t.Fatalf("a dead handle issued session %q", token)
		}
		if token != "" {
			t.Errorf("a failed create still handed out %q", token)
		}
	})

	t.Run("DeleteSession", func(t *testing.T) {
		t.Parallel()
		if err := DeleteSession(ctx, db, "whatever"); err == nil {
			t.Fatal("a dead handle deleted a session")
		}
	})

	t.Run("DeleteSessionsForUser", func(t *testing.T) {
		t.Parallel()
		n, err := DeleteSessionsForUser(ctx, db, 1)
		if err == nil {
			t.Fatalf("a dead handle ended %d sessions", n)
		}
	})

	t.Run("IssueToken", func(t *testing.T) {
		t.Parallel()
		token, err := IssueToken(ctx, db, 1, PurposeInvite)
		if err == nil {
			t.Fatalf("a dead handle issued %q", token)
		}
		if token != "" {
			t.Errorf("a failed issue still handed out %q", token)
		}
	})

	t.Run("RedeemToken", func(t *testing.T) {
		t.Parallel()
		user, err := RedeemToken(ctx, db, "whatever", "hunter2hunter2", PurposeInvite, "")
		if err == nil {
			t.Fatalf("a dead handle redeemed into %v", user)
		}
	})
}

// The purges run on a timer and nobody watches them, so a swallowed error
// there is a table that grows forever with nothing in the logs.
func TestEveryPurgeSaysSoWhenItCannotRun(t *testing.T) {
	t.Parallel()
	db := closedDB(t)
	ctx := context.Background()

	if n, err := PurgeExpiredSessions(ctx, db); err == nil {
		t.Errorf("a dead handle purged %d sessions", n)
	}
	if n, err := PurgeExpiredTokens(ctx, db, time.Hour); err == nil {
		t.Errorf("a dead handle purged %d tokens", n)
	}
	if n, err := PurgeStaleLimits(ctx, db, time.Hour); err == nil {
		t.Errorf("a dead handle purged %d rate-limit rows", n)
	}
}

// The purges are also asserted on a live database, because "returns an error
// when broken" is only half the contract -- the other half is that they
// actually delete what has expired and leave what has not.
func TestThePurgesTakeTheExpiredAndLeaveTheRest(t *testing.T) {
	t.Parallel()
	db := newAccountsDB(t)
	ctx := context.Background()
	user := mustCreate(t, db, "alice", "alice@example.com", true)

	live, err := CreateSession(ctx, db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A session that expired an hour ago, written directly because there is
	// no way to ask for a stale one.
	stale := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx,
		"INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at)"+
			" VALUES ('deadbeef', ?, ?, ?, ?)", user.ID, stale, stale, stale); err != nil {
		t.Fatal(err)
	}

	n, err := PurgeExpiredSessions(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged %d sessions, want the one that expired", n)
	}
	if got, err := Lookup(ctx, db, live); err != nil || got == nil {
		t.Errorf("the live session was purged too: %v %v", got, err)
	}

	// Tokens, the same shape.
	if _, err := IssueToken(ctx, db, user.ID, PurposeInvite); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO auth_tokens (token_hash, user_id, purpose, created_at, expires_at)"+
			" VALUES ('deadbeef', ?, 'invite', ?, ?)", user.ID, stale, stale); err != nil {
		t.Fatal(err)
	}
	n, err = PurgeExpiredTokens(ctx, db, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged %d tokens, want the one that expired", n)
	}
	if out, err := TokenOutstanding(ctx, db, user.ID, PurposeInvite); err != nil || !out {
		t.Errorf("the live invite was purged too: %v %v", out, err)
	}
}

// The rate limiter is the door's own guard, and its failure mode is the
// dangerous direction: a limiter that swallowed its error would let every
// attempt through exactly when the database is unwell.
func TestTheRateLimiterFailsLoudlyRatherThanOpen(t *testing.T) {
	t.Parallel()
	db := closedDB(t)
	ctx := context.Background()

	if _, err := RecordFailure(ctx, db, "addr:127.0.0.1", PerAddress); err == nil {
		t.Error("a dead handle recorded a failure without complaint")
	}
	if err := ClearLimit(ctx, db, "addr:127.0.0.1"); err == nil {
		t.Error("a dead handle cleared a limit without complaint")
	}
	if _, err := RetryAfter(ctx, db, "addr:127.0.0.1", PerAddress); err == nil {
		t.Error("a dead handle answered a retry-after without complaint")
	}
}

// The limiter's live behaviour. The pairing is the subtle part: `Exhausted`
// answers *whether* a key is out of attempts and `RetryAfter` answers *how
// long* the window has left -- and RetryAfter is **never zero**, because a
// `Retry-After: 0` reads as "try now", which is the one thing the answer is
// saying not to do. Reading RetryAfter as "is it limited" would report every
// fresh key as limited for a full window.
func TestTheLimiterCountsUpToItsBudgetAndForgetsOnSuccess(t *testing.T) {
	t.Parallel()
	db := newAccountsDB(t)
	ctx := context.Background()
	key := AddressKey("198.51.100.7", "signin")
	// A tight budget so a handful of failures is comfortably over it,
	// whatever the real per-address allowance happens to be.
	tight := Limit{Failures: 3, Window: 15 * time.Minute}

	exhausted, err := Exhausted(ctx, db, key, tight)
	if err != nil {
		t.Fatal(err)
	}
	if exhausted {
		t.Fatal("a key nobody has failed against is already exhausted")
	}

	for i := 1; i <= 3; i++ {
		n, err := RecordFailure(ctx, db, key, tight)
		if err != nil {
			t.Fatal(err)
		}
		if n != i {
			t.Fatalf("failure %d counted %d", i, n)
		}
	}
	if exhausted, err = Exhausted(ctx, db, key, tight); err != nil || !exhausted {
		t.Fatalf("three failures against a budget of three: exhausted=%v (%v)", exhausted, err)
	}

	// The header value: a positive number of seconds, never zero.
	wait, err := RetryAfter(ctx, db, key, tight)
	if err != nil {
		t.Fatal(err)
	}
	if wait < 1 {
		t.Errorf("Retry-After is %d -- zero reads as 'try now'", wait)
	}
	if wait > int(tight.Window.Seconds()) {
		t.Errorf("Retry-After is %d, longer than the %v window", wait, tight.Window)
	}

	// A successful sign-in forgets the failures, which is what keeps a
	// household behind one address from locking itself out over a typo.
	if err := ClearLimit(ctx, db, key); err != nil {
		t.Fatal(err)
	}
	if exhausted, err = Exhausted(ctx, db, key, tight); err != nil || exhausted {
		t.Errorf("clearing left the key exhausted (%v)", err)
	}

	// A lapsed window is forgotten without anybody clearing it: the row is
	// backdated past the budget's window and the count starts over.
	if _, err := RecordFailure(ctx, db, key, tight); err != nil {
		t.Fatal(err)
	}
	lapsed := time.Now().UTC().Add(-2 * tight.Window).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx,
		"UPDATE login_attempts SET window_start = ?, failures = 99 WHERE key = ?",
		lapsed, key); err != nil {
		t.Fatal(err)
	}
	if exhausted, err = Exhausted(ctx, db, key, tight); err != nil || exhausted {
		t.Errorf("a lapsed window is still exhausted (%v)", err)
	}
	// And the stale row is purgeable.
	n, err := PurgeStaleLimits(ctx, db, tight.Window)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged %d stale windows, want 1", n)
	}
}

// The two key builders are namespaced apart, so an address and a mailbox can
// never share a bucket -- one budget spending another's is a lockout nobody
// can explain.
func TestTheKeyBuildersNamespaceTheirBudgetsApart(t *testing.T) {
	t.Parallel()
	addr := AddressKey("198.51.100.7", "signin")
	if addr == "" {
		t.Fatal("an address keyed to nothing")
	}
	if addr != AddressKey("198.51.100.7", "signin") {
		t.Error("the same address keyed two ways")
	}
	if addr == AddressKey("198.51.100.8", "signin") {
		t.Error("two addresses share a key")
	}
	if addr == AddressKey("198.51.100.7", "reset") {
		t.Error("two scopes share a key -- one budget spends the other's")
	}

	box := EmailKey("Alice@Example.COM")
	if box != EmailKey("  alice@example.com  ") {
		t.Error("the mailbox key is not folded and trimmed")
	}
	if box == EmailKey("bob@example.com") {
		t.Error("two mailboxes share a key")
	}
	if box == addr {
		t.Error("a mailbox and an address share a key")
	}
	// **The mailbox is hashed and the IP is not**, and the asymmetry is the
	// argued one: a reset request for an address with no account would
	// otherwise store that address, which is personal data the project has
	// no reason to hold for somebody who is not a user. An IP is not that.
	if strings.Contains(box, "alice") || strings.Contains(box, "example.com") {
		t.Error("the mailbox key carries the address in the clear")
	}
	if !strings.Contains(addr, "198.51.100.7") {
		t.Error("the address key no longer carries the address -- if that is " +
			"deliberate, EmailKey's argument now applies here too")
	}
}

// The address key is stable and namespaced, so an address can never collide
// with a username key.
func TestTheAddressKeyIsNamespaced(t *testing.T) {
	t.Parallel()
	one := AddressKey("198.51.100.7", "signin")
	if one == "" {
		t.Fatal("an address keyed to nothing")
	}
	if one != AddressKey("198.51.100.7", "signin") {
		t.Error("the same address keyed two ways")
	}
	if one == AddressKey("198.51.100.8", "signin") {
		t.Error("two addresses share a key")
	}
	// The scope is what keeps one budget from spending another's: a failed
	// sign-in must not count against the reset allowance.
	if one == AddressKey("198.51.100.7", "reset") {
		t.Error("two scopes share a key -- one budget spends the other's")
	}
}

// The maintainer bootstrap is the one write that runs unattended at boot
// (ADR 17), so its failure has to reach the caller rather than leaving an
// instance with no administrator and no complaint.
func TestTheMaintainerBootstrapSaysSoWhenItCannotRun(t *testing.T) {
	t.Parallel()
	db := closedDB(t)
	err := EnsureMaintainer(context.Background(), db,
		config.Config{AdminEmail: "alice@example.com", AdminUsername: "alice"})
	if err == nil {
		t.Fatal("a dead handle reconciled the maintainer")
	}
}

// The invite and reset mails both need somewhere to send to, and both refuse
// rather than issuing a token nobody will ever receive -- a token issued and
// not delivered is a live credential with no owner.
func TestAMailFailureDoesNotLeaveALiveTokenBehind(t *testing.T) {
	t.Parallel()
	db := newAccountsDB(t)
	ctx := context.Background()
	user := mustCreate(t, db, "alice", "alice@example.com", false)

	failing := senderFunc(func(Message) error {
		return errors.New("the mail provider is down")
	})
	if err := SendInvite(ctx, db, user, failing, ""); err == nil {
		t.Fatal("a failed send reported success")
	}
	if err := SendReset(ctx, db, "alice@example.com", failing, ""); err == nil {
		t.Fatal("a failed reset reported success")
	}

	// An account with no address has nowhere to send to, and that is refused
	// before a token is ever issued.
	noAddress := &User{ID: user.ID, Username: "nobody"}
	if err := SendInvite(ctx, db, noAddress, failing, ""); err == nil {
		t.Fatal("an invite was sent to an account with no address")
	}

	// A reset for an address nobody holds must not say so -- the whole point
	// of that route is that it costs the same either way.
	if err := SendReset(ctx, db, "stranger@example.com",
		senderFunc(func(Message) error { return nil }), ""); err != nil {
		t.Errorf("a reset for an unknown address reported %v", err)
	}
}

// senderFunc adapts a closure to the mail sender.
type senderFunc func(Message) error

func (f senderFunc) Send(m Message) error { return f(m) }
