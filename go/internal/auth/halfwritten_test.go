package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/config"
)

// The write that got halfway.
//
// `errorpaths_test.go` closes a handle and proves that the *first* statement
// of every function here says so when it fails. This file is the other half,
// and it is the half where the interesting mistakes live: almost every write
// in this package is a transaction three or four statements long, and each
// step after the first is a place where the row has already moved and the
// function has to decide what to tell its caller. A password set while the
// old sessions survive, a link spent on a rename that was refused, an account
// reported disabled by an UPDATE that matched nothing — none of those crash,
// they **succeed wrongly**, which is the failure a test has to go looking for.
//
// `authtest.OpenFaulty` is the fixture: a real migrated `app.db` whose driver
// stops answering after a chosen number of statements, which is what a
// network volume detaching mid-transaction looks like from in here. Budgets
// below are counted in statements — the BEGIN, each exec, each query, the
// COMMIT — and every one of them is paired with an assertion about the rows,
// so a budget that lands on the wrong statement fails on the state rather
// than passing quietly.

func faultyAuth(t *testing.T) (*sql.DB, *authtest.Fault) {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, fault
}

// oneAccount seeds a claimed, enabled, non-admin account.
func oneAccount(t *testing.T, db *sql.DB) *User {
	t.Helper()
	ctx := context.Background()
	u, err := Create(ctx, db, "squire", "squire@example.test", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SetPassword(ctx, db, u.ID, "a long enough passphrase"); err != nil {
		t.Fatal(err)
	}
	return u
}

// Setting a password is one transaction on purpose: there must be no window
// in which the password has changed and the old sessions are still live. Each
// budget below stops the transaction one statement later, and the assertion
// is the same every time — the password did not change *and* the sessions did
// not go, because either one alone is the failure ADR 16 points at.
func TestAPasswordThatDoesNotFinishWritingChangesNothing(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	user := oneAccount(t, db)
	if _, err := CreateSession(ctx, db, user.ID); err != nil {
		t.Fatal(err)
	}

	// The BEGIN, the password UPDATE, the session DELETE, the COMMIT.
	for _, budget := range []int{0, 1, 2, 3} {
		fault.Heal()
		fault.After(budget)
		_, err := SetPassword(ctx, db, user.ID, "a different passphrase")
		fault.Heal()
		if err == nil {
			t.Fatalf("a password was set with only %d statements' worth of database", budget)
		}
		// The count beside an error is not a claim: `revoked` is assigned
		// inside the transaction, so a COMMIT that never lands leaves it
		// holding a number about work that was rolled back. The rows are the
		// assertion, and they are the only one.
		if signedIn, err := CountSessionsForUser(ctx, db, user.ID); err != nil {
			t.Fatal(err)
		} else if signedIn != 1 {
			t.Fatalf("the session count moved to %d on a reset that did not land", signedIn)
		}
		who, err := Authenticate(ctx, db, "squire", "a long enough passphrase")
		if err != nil {
			t.Fatal(err)
		}
		if who == nil {
			t.Fatalf("the old password stopped working after a reset that failed at "+
				"statement %d", budget+1)
		}
	}

	fault.Heal()
	revoked, err := SetPassword(ctx, db, user.ID, "a different passphrase")
	if err != nil || revoked != 1 {
		t.Fatalf("the recovered reset: revoked=%d err=%v", revoked, err)
	}
}

// Every setter that ends in `WHERE id = ?` has a branch for the row that was
// not there, and the branch matters more than it looks: without it the UPDATE
// matches nothing, the function returns nil, and the caller is told an
// account it cannot even see has been disabled, demoted or renamed.
func TestAnAccountThatIsNotThereIsARefusalRatherThanASilentNoOp(t *testing.T) {
	t.Parallel()
	db, _ := faultyAuth(t)
	ctx := context.Background()
	// A real admin, so the last-admin guard never fires first and hides
	// what is being asked here.
	if _, err := Create(ctx, db, "keeper", "keeper@example.test", true); err != nil {
		t.Fatal(err)
	}
	if _, err := SetPassword(ctx, db, 1, "a long enough passphrase"); err != nil {
		t.Fatal(err)
	}

	const ghost = int64(9999)
	for name, err := range map[string]error{
		"a password set on nobody": func() error {
			_, e := SetPassword(ctx, db, ghost, "a long enough passphrase")
			return e
		}(),
		"a rename of nobody": func() error {
			_, e := SetUsername(ctx, db, ghost, "nobody")
			return e
		}(),
		"disabling nobody": func() error { _, e := SetDisabled(ctx, db, ghost, true); return e }(),
		"promoting nobody": SetAdmin(ctx, db, ghost, true),
		"tiering nobody":   SetModelTier(ctx, db, ghost, ""),
		"deleting nobody":  func() error { _, e := Delete(ctx, db, ghost); return e }(),
	} {
		if !errors.Is(err, ErrNoSuchUser) {
			t.Errorf("%s answered %v, want ErrNoSuchUser", name, err)
		}
	}

	// And the sentinel is the whole point: the message a route puts on the
	// wire is the sentence, not the sentinel's own name.
	_, err := Delete(ctx, db, ghost)
	if strings.Contains(err.Error(), "no such user:") {
		t.Fatalf("the refusal repeated its sentinel's text: %q", err)
	}
}

// The three account writes that take the write lock before they read run
// through `exclusive`, whose own failures are the ones ADR 17's rule rests
// on: a BEGIN IMMEDIATE that does not take, or a COMMIT that does not land,
// must not leave a caller believing the guard was applied.
func TestAnExclusiveWriteThatCannotTakeTheLockChangesNothing(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	keeper, err := Create(ctx, db, "keeper", "keeper@example.test", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SetPassword(ctx, db, keeper.ID, "a long enough passphrase"); err != nil {
		t.Fatal(err)
	}
	other, err := Create(ctx, db, "squire", "squire@example.test", false)
	if err != nil {
		t.Fatal(err)
	}

	// BEGIN IMMEDIATE, the UPDATE, the COMMIT. A grant skips the last-admin
	// guard entirely — only revocation is checked — so there is no census
	// read in this one; the disable below is where that statement shows up.
	for _, budget := range []int{0, 1, 2} {
		fault.Heal()
		fault.After(budget)
		err := SetAdmin(ctx, db, other.ID, true)
		fault.Heal()
		if err == nil {
			t.Fatalf("admin was granted with only %d statements' worth of database", budget)
		}
		fresh, err := GetByID(ctx, db, other.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.IsAdmin {
			t.Fatalf("a grant that failed at statement %d promoted the account anyway",
				budget+1)
		}
	}

	// The tier and the disable run the same shape, and the same two things
	// have to be true of each: an error out, and nothing moved.
	fault.After(1)
	tierErr := SetModelTier(ctx, db, other.ID, "")
	fault.Heal()
	if tierErr == nil {
		t.Fatal("a tier was chosen over a database that had gone")
	}
	fault.After(2)
	revoked, disableErr := SetDisabled(ctx, db, other.ID, true)
	fault.Heal()
	if disableErr == nil || revoked != 0 {
		t.Fatalf("an account was disabled over a database that had gone: %v", disableErr)
	}
	fresh, err := GetByID(ctx, db, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Disabled {
		t.Fatal("an account is disabled after the write that would have disabled it failed")
	}

	fault.Heal()
	if err := SetAdmin(ctx, db, other.ID, true); err != nil {
		t.Fatal(err)
	}
	// Disabling an admin still revokes their sessions, and the count is the
	// proof the second half of that transaction ran at all.
	if _, err := CreateSession(ctx, db, other.ID); err != nil {
		t.Fatal(err)
	}
	if revoked, err := SetDisabled(ctx, db, other.ID, true); err != nil || revoked != 1 {
		t.Fatalf("the recovered disable: revoked=%d err=%v", revoked, err)
	}
}

// A link is spent and a password is set in one transaction, and the reason is
// worth restating because every budget below is a test of it: a rename the
// database refuses has to leave a **retryable invite**, not a spent link and
// an account nobody can get into.
func TestAnInviteThatFailsPartwayIsStillRedeemable(t *testing.T) {
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

	// The lookup, the account read, the BEGIN, the token UPDATE, the rename,
	// the password UPDATE, the session DELETE, the COMMIT, the re-read.
	for _, budget := range []int{1, 2, 3, 4, 5, 6, 7} {
		fault.Heal()
		fault.After(budget)
		claimed, err := RedeemToken(ctx, db, token, "a long enough passphrase",
			PurposeInvite, "renamed")
		fault.Heal()
		if err == nil {
			t.Fatalf("a link was redeemed with only %d statements' worth of database", budget)
		}
		if claimed != nil {
			t.Fatalf("a failed redemption handed back an account: %+v", claimed)
		}
		if _, err := LookupToken(ctx, db, token, PurposeInvite); err != nil {
			t.Fatalf("the link was spent by a redemption that failed at statement %d: %v",
				budget+1, err)
		}
	}

	fault.Heal()
	claimed, err := RedeemToken(ctx, db, token, "a long enough passphrase",
		PurposeInvite, "renamed")
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Username != "renamed" {
		t.Fatalf("the recovered redemption named the account %q", claimed.Username)
	}
	if _, err := LookupToken(ctx, db, token, PurposeInvite); !errors.Is(err, ErrTokenUsed) {
		t.Fatalf("a spent link is not reported spent: %v", err)
	}
}

// A name somebody else holds is refused by the UNIQUE index rather than by a
// check, which is what makes it race-proof — and the refusal has to be the
// one a person can act on ("that username is already taken"), with the link
// still good for another try.
func TestAClaimedNameLeavesTheLinkRetryable(t *testing.T) {
	t.Parallel()
	db, _ := faultyAuth(t)
	ctx := context.Background()
	if _, err := Create(ctx, db, "taken", "taken@example.test", false); err != nil {
		t.Fatal(err)
	}
	user, err := Create(ctx, db, "invited", "invited@example.test", false)
	if err != nil {
		t.Fatal(err)
	}
	token, err := IssueToken(ctx, db, user.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	// COLLATE NOCASE on the column, so a different capitalisation is the
	// same name.
	if _, err := RedeemToken(ctx, db, token, "a long enough passphrase",
		PurposeInvite, "Taken"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("a taken name was accepted: %v", err)
	}
	if _, err := LookupToken(ctx, db, token, PurposeInvite); err != nil {
		t.Fatalf("a refused name spent the link: %v", err)
	}
	if _, err := RedeemToken(ctx, db, token, "a long enough passphrase",
		PurposeInvite, "free"); err != nil {
		t.Fatalf("the retry was refused: %v", err)
	}
}

// A row whose `used_at` is an empty string is read as unused and written as
// used, and the two halves of that disagreement are exactly the race the
// `WHERE used_at IS NULL` clause exists for: the lookup says the link is
// live, the UPDATE matches nothing, and what comes back has to be "already
// used" rather than a silent success.
func TestALinkThatWasSpentBetweenTheLookupAndTheWriteSaysSo(t *testing.T) {
	t.Parallel()
	db, _ := faultyAuth(t)
	ctx := context.Background()
	user, err := Create(ctx, db, "invited", "invited@example.test", false)
	if err != nil {
		t.Fatal(err)
	}
	token, err := IssueToken(ctx, db, user.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE auth_tokens SET used_at = '' WHERE token_hash = ?",
		HashToken(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := RedeemToken(ctx, db, token, "a long enough passphrase",
		PurposeInvite, ""); !errors.Is(err, ErrTokenUsed) {
		t.Fatalf("the losing side of the race answered %v, want ErrTokenUsed", err)
	}
}

// A timestamp column that will not parse is a refusal, never a guess. Every
// one of these is a row that could only exist because something else wrote it
// wrong, and reading it as "now" or as "the epoch" would turn a corrupt row
// into an expired session, a live link, or an unlimited login budget —
// each of which is worse than an error.
func TestACorruptTimestampIsARefusalRatherThanAGuess(t *testing.T) {
	t.Parallel()
	db, _ := faultyAuth(t)
	ctx := context.Background()
	user := oneAccount(t, db)

	token, err := IssueToken(ctx, db, user.ID, PurposeReset)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"expires_at", "created_at"} {
		if _, err := db.ExecContext(ctx,
			"UPDATE auth_tokens SET "+column+" = 'not a timestamp' WHERE token_hash = ?",
			HashToken(token)); err != nil {
			t.Fatal(err)
		}
		if _, err := LookupToken(ctx, db, token, PurposeReset); err == nil {
			t.Errorf("a token whose %s is unreadable resolved anyway", column)
		}
		// Put it back before corrupting the next one, so each column is
		// tested on its own rather than behind the previous failure.
		if _, err := db.ExecContext(ctx,
			"UPDATE auth_tokens SET "+column+" = ? WHERE token_hash = ?",
			isoAt(time.Now().Add(time.Hour)), HashToken(token)); err != nil {
			t.Fatal(err)
		}
	}

	session, err := CreateSession(ctx, db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"expires_at", "created_at"} {
		if _, err := db.ExecContext(ctx,
			"UPDATE sessions SET "+column+" = 'not a timestamp' WHERE token_hash = ?",
			HashToken(session)); err != nil {
			t.Fatal(err)
		}
		if _, err := Lookup(ctx, db, session); err == nil {
			t.Errorf("a session whose %s is unreadable resolved anyway", column)
		}
		if _, err := LookupTouching(ctx, db, session); err == nil {
			t.Errorf("the touching lookup resolved a session whose %s is unreadable", column)
		}
		if _, err := db.ExecContext(ctx,
			"UPDATE sessions SET "+column+" = ? WHERE token_hash = ?",
			isoAt(time.Now().Add(time.Hour)), HashToken(session)); err != nil {
			t.Fatal(err)
		}
	}

	// And the rate limiter's own window, where a wrong answer is an
	// attempt budget nobody is spending.
	key := AddressKey("203.0.113.7", "")
	if _, err := RecordFailure(ctx, db, key, PerAddress); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE login_attempts SET window_start = 'not a timestamp' WHERE key = ?",
		key); err != nil {
		t.Fatal(err)
	}
	if _, err := Exhausted(ctx, db, key, PerAddress); err == nil {
		t.Error("a rate-limit window that will not parse was read as a budget")
	}
	if _, err := RetryAfter(ctx, db, key, PerAddress); err == nil {
		t.Error("Retry-After was computed from a window that will not parse")
	}
}

// The serving resolver's session lookup owes the table two writes, and each
// of them failing has to reach the caller: a session whose expired row could
// not be deleted must not be handed back as live, and a touch that did not
// land is a `last_seen_at` the admin page will read as a signed-out account.
func TestTheTouchingLookupSaysSoWhenItsOwnWritesFail(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	user := oneAccount(t, db)

	live, err := CreateSession(ctx, db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A session last seen long enough ago to be worth touching — the branch
	// the write below lives in.
	if _, err := db.ExecContext(ctx,
		"UPDATE sessions SET last_seen_at = ? WHERE token_hash = ?",
		isoAt(time.Now().Add(-48*time.Hour)), HashToken(live)); err != nil {
		t.Fatal(err)
	}
	// The read lands; the touch is the statement that goes.
	fault.After(1)
	got, err := LookupTouching(ctx, db, live)
	fault.Heal()
	if err == nil || got != nil {
		t.Fatalf("a session was resolved over a touch that failed: %+v %v", got, err)
	}

	expired, err := CreateSession(ctx, db, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE sessions SET expires_at = ? WHERE token_hash = ?",
		isoAt(time.Now().Add(-time.Hour)), HashToken(expired)); err != nil {
		t.Fatal(err)
	}
	fault.After(1)
	got, err = LookupTouching(ctx, db, expired)
	fault.Heal()
	if err == nil || got != nil {
		t.Fatalf("an expired session whose delete failed came back as %+v (%v)", got, err)
	}
	// Healed, the same row is deleted on the way past rather than left for a
	// purge — which is the behaviour the failure above was standing in front of.
	if got, err := LookupTouching(ctx, db, expired); err != nil || got != nil {
		t.Fatalf("the expired session resolved: %+v %v", got, err)
	}
	if n, err := CountSessionsForUser(ctx, db, user.ID); err != nil {
		t.Fatal(err)
	} else if n != 1 {
		t.Fatalf("%d sessions survive; the expired row was not swept on the way past", n)
	}
}

// The sweep steps past a purge that failed and takes the other two tables
// anyway — but a sweep cut short by its own context goes quietly, because
// that is a shutdown crossing a tick rather than a fault, and a log line
// there would cry wolf on every restart.
func TestTheSweepStepsPastAFailureAndGoesQuietOnAShutdown(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	logged := &strings.Builder{}
	sweeper := NewSweeper(SweeperConfig{DB: db,
		Log: slog.New(slog.NewTextHandler(logged, nil))})
	t.Cleanup(sweeper.Stop)

	fault.After(0)
	counts := sweeper.SweepOnce(context.Background())
	fault.Heal()
	for _, said := range []string{"the session sweep failed", "the link sweep failed",
		"the rate-limit sweep failed", "the accounts database was swept"} {
		if !strings.Contains(logged.String(), said) {
			t.Errorf("the sweep never said %q; it said:\n%s", said, logged.String())
		}
	}
	if counts != (SweepCounts{}) {
		t.Fatalf("a sweep that purged nothing reported %+v", counts)
	}

	// The same broken database under a cancelled context: three failures and
	// not one word about them.
	quiet := &strings.Builder{}
	stopping := NewSweeper(SweeperConfig{DB: db,
		Log: slog.New(slog.NewTextHandler(quiet, nil))})
	t.Cleanup(stopping.Stop)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fault.After(0)
	stopping.SweepOnce(ctx)
	fault.Heal()
	if strings.Contains(quiet.String(), "sweep failed") {
		t.Fatalf("a shutdown crossing a tick was logged as a fault:\n%s", quiet.String())
	}
	if strings.Contains(quiet.String(), "the accounts database was swept") {
		t.Fatalf("a sweep cut short claimed to have swept:\n%s", quiet.String())
	}

	// And a sweeper built with no log at all still works — the zero value is
	// a working default, not a nil dereference on the first purge.
	fault.Heal()
	bare := NewSweeper(SweeperConfig{DB: db})
	t.Cleanup(bare.Stop)
	if got := bare.SweepOnce(context.Background()); got != (SweepCounts{}) {
		t.Fatalf("a sweep of an empty database reported %+v", got)
	}
}

// Reconciling the maintainer is four or five statements deep depending on
// what it finds, and every one of them is a place the volume can go. A
// failure there is an error — unlike a malformed address, which is logged and
// carried past — because it is a fact about the disk rather than about a
// preference.
func TestTheMaintainerReconciliationStopsAtTheStepThatFailed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := config.Config{AdminEmail: "keeper@example.test"}

	// A fresh instance: the address read, the handle check, the INSERT, the
	// read-back.
	for _, budget := range []int{0, 1, 2} {
		db, fault := faultyAuth(t)
		fault.After(budget)
		err := EnsureMaintainer(ctx, db, cfg)
		fault.Heal()
		if err == nil {
			t.Fatalf("a maintainer was reconciled with only %d statements' worth "+
				"of database", budget)
		}
		if who, err := GetByEmail(ctx, db, "keeper@example.test"); err != nil {
			t.Fatal(err)
		} else if who != nil && budget < 2 {
			t.Fatalf("an account appeared from a reconciliation that failed at "+
				"statement %d", budget+1)
		}
	}

	// An existing account that has to be promoted, and one that has to be
	// re-enabled: both are a second write after the read, and both have to
	// carry their failure out.
	for _, tc := range []struct {
		name            string
		admin, disabled bool
	}{
		{"promoting an existing account", false, false},
		{"re-enabling a disabled maintainer", true, true},
	} {
		db, fault := faultyAuth(t)
		account, err := Create(ctx, db, "keeper", "keeper@example.test", tc.admin)
		if err != nil {
			t.Fatal(err)
		}
		if tc.disabled {
			// A second admin, so disabling the first is not the last-admin
			// refusal in disguise.
			if _, err := Create(ctx, db, "other", "other@example.test", true); err != nil {
				t.Fatal(err)
			}
			if _, err := SetPassword(ctx, db, 2, "a long enough passphrase"); err != nil {
				t.Fatal(err)
			}
			if _, err := SetDisabled(ctx, db, account.ID, true); err != nil {
				t.Fatal(err)
			}
		}
		// The address read lands; the write that follows it does not.
		fault.After(1)
		err = EnsureMaintainer(ctx, db, cfg)
		fault.Heal()
		if err == nil {
			t.Errorf("%s: the reconciliation survived a database that had gone", tc.name)
		}
	}
}

// The maintainer's handle is taken, so the reconciliation walks: `aaron`,
// `aaron2`, `aaron3`. Renaming the friend who got there first would be worse
// than the maintainer having a number on the end of their name.
func TestATakenMaintainerHandleWalksToAFreeOne(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	if _, err := Create(ctx, db, "keeper", "someone.else@example.test", false); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AdminEmail: "keeper@example.test"}
	if err := EnsureMaintainer(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	made, err := GetByEmail(ctx, db, "keeper@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if made == nil || made.Username != "keeper2" {
		t.Fatalf("the maintainer was named %v, want keeper2", made)
	}

	// And the walk's own read can fail. `keeper` and `keeper2` are both taken
	// now, and a third address asking for the same handle has to walk past
	// both: the address read answers, the look at `keeper` answers taken, and
	// the look at `keeper2` is the statement that goes.
	fault.After(2)
	err = EnsureMaintainer(ctx, db, config.Config{AdminEmail: "third@example.test",
		AdminUsername: "keeper"})
	fault.Heal()
	if err == nil {
		t.Fatal("the handle walk survived a database that had gone")
	}
}

// Ninety-nine names deep the walk gives up, and it says which handle it was
// looking near — a maintainer who cannot be created at all is a lockout, and
// the sentence is the only thing standing between that and a silent one.
func TestAHandleWithNoFreeNeighbourIsARefusalThatNamesIt(t *testing.T) {
	t.Parallel()
	db, _ := faultyAuth(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO users (username, created_at) VALUES ('keeper', ?)",
		nowISO()); err != nil {
		t.Fatal(err)
	}
	for suffix := 2; suffix < 100; suffix++ {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO users (username, created_at) VALUES (?, ?)",
			fmt.Sprintf("keeper%d", suffix), nowISO()); err != nil {
			t.Fatal(err)
		}
	}
	_, err := uniqueUsername(ctx, db, "keeper")
	if !errors.Is(err, ErrUserExists) {
		t.Fatalf("a crowded handle answered %v, want ErrUserExists", err)
	}
	if !strings.Contains(err.Error(), "keeper") {
		t.Fatalf("the refusal did not name the handle: %q", err)
	}
}

// An iteration that fails partway is not a short list. Both of these read a
// whole table into memory, and a loop that stopped early without asking
// `rows.Err()` would hand back the rows it happened to get — which for the
// admin census is a lockout guard counting the wrong number.
func TestAnIterationThatFailsPartwayIsNotAShortList(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	for _, name := range []string{"ada", "bee", "cyd"} {
		made, err := Create(ctx, db, name, name+"@example.test", true)
		if err != nil {
			t.Fatal(err)
		}
		// A usable admin is the flag plus a password plus not disabled, so
		// the census reads nothing at all off unclaimed invites.
		if _, err := SetPassword(ctx, db, made.ID, "a long enough passphrase"); err != nil {
			t.Fatal(err)
		}
	}

	fault.RowsAfter(1)
	if users, err := AllUsers(ctx, db); err == nil {
		t.Errorf("a list that stopped after one row came back as %d accounts", len(users))
	}
	fault.Heal()

	fault.RowsAfter(1)
	if admins, err := UsableAdminIDs(ctx, db); err == nil {
		t.Errorf("the admin census came back as %d over a failed iteration", len(admins))
	}
	fault.Heal()

	// Healed, the same two reads are whole — which is what makes the
	// assertions above about the iteration rather than about the fixture.
	if users, err := AllUsers(ctx, db); err != nil || len(users) != 3 {
		t.Fatalf("the recovered list: %d accounts, %v", len(users), err)
	}
}

// failf is how every refusal in this package is spelled, and it panics on a
// call shape it cannot honour rather than shipping a wrong sentence. Three
// shapes, each caught the first time the line runs.
func TestFailfRefusesACallShapeItCannotHonour(t *testing.T) {
	t.Parallel()
	for name, call := range map[string]func(){
		"a format that does not start with the sentinel": func() {
			_ = failf("something went wrong: %s", "detail")
		},
		"no arguments at all": func() { _ = failf("%w: nothing follows") },
		"a first argument that is not an error": func() {
			_ = failf("%w: %d", "not an error", 1)
		},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			call()
		}()
	}
	// And the shape it does honour: the sentinel is findable, its own text is
	// not repeated, and the sentence is what a route would put on the wire.
	err := failf("%w: %s is already registered", ErrUserExists, "that username")
	if !errors.Is(err, ErrUserExists) {
		t.Fatal("the sentinel did not survive failf")
	}
	if err.Error() != "that username is already registered" {
		t.Fatalf("the sentence is %q", err.Error())
	}
}

// The small answers nothing else was asking for: the rate limiter's own
// sentences and its floor, and the rehash check on a hash it cannot read.
func TestTheRateLimitersOwnAnswers(t *testing.T) {
	t.Parallel()
	if got := (Limit{Failures: 10, Window: 15 * time.Minute}).Describe(); got !=
		"10 attempts per 15 minutes" {
		t.Errorf("Describe() = %q", got)
	}
	// An unscoped address key is the login flow's, named rather than blank:
	// one shared counter across every unauthenticated endpoint would make any
	// one of them a way to lock a client out of the others.
	if got := AddressKey("203.0.113.7", ""); got != "ip:login:203.0.113.7" {
		t.Errorf("an unscoped key is %q", got)
	}
	if AddressKey("203.0.113.7", "reset") == AddressKey("203.0.113.7", "") {
		t.Error("two flows share one address counter")
	}

	db, _ := faultyAuth(t)
	ctx := context.Background()
	key := AddressKey("203.0.113.9", "login")
	// A window that lapsed ages ago: Retry-After never reads zero, because a
	// header of zero reads as "try now", which is the one thing the answer is
	// saying not to do.
	if _, err := RecordFailure(ctx, db, key, Limit{Failures: 3, Window: time.Nanosecond}); err != nil {
		t.Fatal(err)
	}
	if got, err := RetryAfter(ctx, db, key, Limit{Failures: 3, Window: time.Nanosecond}); err != nil ||
		got != 1 {
		t.Fatalf("RetryAfter on a lapsed window = %d, %v; want 1", got, err)
	}

	// A stored hash nothing can decode is not a hash to rehash: saying yes
	// here would rewrite an unreadable column on every login attempt.
	if NeedsRehash("not an argon2 hash at all") {
		t.Error("an undecodable hash was reported as needing a rehash")
	}
}

// The mail sender's two refusals that never leave the process: a key that is
// not there, and a provider that cannot be reached.
func TestTheMailSenderRefusesWithoutAKeyAndSaysSoWhenItCannotReachTheProvider(t *testing.T) {
	t.Parallel()
	if _, err := NewResendSender("   ", "a <b@example.test>", nil); !errors.Is(err,
		ErrEmailNotConfigured) {
		t.Fatalf("an empty key built a sender: %v", err)
	}
	sender, err := NewResendSender("re_key", "a <b@example.test>",
		func(*http.Request) (int, []byte, error) {
			return 0, nil, errors.New("dial tcp: no route to host")
		})
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(Message{To: "somebody@example.test", Subject: "s", Body: "b"})
	if !errors.Is(err, ErrEmailNotSent) {
		t.Fatalf("an unreachable provider answered %v", err)
	}
	// The network's own words are safe to quote — the error is about the
	// wire, not about the recipient — and a message with nothing in it is
	// the one thing this may never say.
	if !strings.Contains(err.Error(), "could not reach the mail provider") {
		t.Fatalf("the failure did not say what went wrong: %q", err)
	}
	if strings.Contains(err.Error(), "somebody@example.test") {
		t.Fatalf("the failure quoted the recipient's address: %q", err)
	}
}

// An invite needs a link, and a link is a row: when the row cannot be written
// no message goes out, because a mail carrying a token the database never
// recorded is worse than no mail at all.
func TestAnInviteWithNoRowToStandOnSendsNothing(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	user := oneAccount(t, db)
	sender := &countingSender{}

	fault.After(0)
	inviteErr := SendInvite(ctx, db, user, sender, "https://example.test")
	resetErr := SendReset(ctx, db, "squire@example.test", sender, "https://example.test")
	fault.Heal()
	if inviteErr == nil {
		t.Error("an invite was sent over a database that could not record its link")
	}
	if resetErr == nil {
		t.Error("a reset was sent over a database that could not record its link")
	}
	if sender.sent != 0 {
		t.Fatalf("%d messages went out with no token row behind them", sender.sent)
	}

}

// A backup that cannot be written is an error, not a zero. `mtglab` runs this
// before a migration, and a green exit over a file that is not there would be
// a ladder climbed with no way back down.
func TestABackupThatCannotBeWrittenSaysSo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Backup(context.Background(), path,
		filepath.Join(dir, "no", "such", "place", "app.db")); err == nil {
		t.Fatal("a backup into a directory that is not there reported success")
	}
	// And the working shape, so the failure above is about the destination
	// rather than about the fixture.
	version, err := Backup(context.Background(), path, filepath.Join(dir, "copy.db"))
	if err != nil {
		t.Fatal(err)
	}
	if version != SchemaVersion {
		t.Fatalf("the backup reports version %d, want %d", version, SchemaVersion)
	}
}

// The ladder itself, rung by rung, against a volume that goes away partway
// up. A migration that stops halfway has to say which rung — `app.db` is the
// one file this app cannot rebuild from anything else, and the sentence is
// what tells a maintainer whether to restore from the backup taken a moment
// earlier.
func TestALadderThatStopsPartwayNamesWhereItStopped(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		budget int
		rows   int
	}{
		// The version read, the foreign-keys pragma, then seventeen scripts,
		// then the version write, then the check.
		{"reading the version it is at", 0, -1},
		{"turning foreign keys off", 1, -1},
		{"the first rung", 2, -1},
		{"stamping the new version", 2 + SchemaVersion, -1},
		{"the foreign-key check it signs off with", 3 + SchemaVersion, -1},
		{"reading the check's own answer", -1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db, fault, err := authtest.OpenFaultyEmpty(
				filepath.Join(t.TempDir(), "app.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })

			if tc.budget >= 0 {
				fault.After(tc.budget)
			}
			if tc.rows >= 0 {
				fault.RowsAfter(tc.rows)
			}
			err = migrate(context.Background(), conn)
			fault.Heal()
			if err == nil {
				t.Fatalf("the ladder climbed past %s over a database that had gone", tc.name)
			}
			if !strings.Contains(err.Error(), "app.db") {
				t.Fatalf("the failure does not name the file: %q", err)
			}
		})
	}

	t.Run("and the whole ladder over a healthy file", func(t *testing.T) {
		t.Parallel()
		db, _, err := authtest.OpenFaultyEmpty(filepath.Join(t.TempDir(), "app.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		conn, err := db.Conn(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		if err := migrate(context.Background(), conn); err != nil {
			t.Fatal(err)
		}
		var version int
		if err := conn.QueryRowContext(context.Background(),
			"PRAGMA user_version").Scan(&version); err != nil {
			t.Fatal(err)
		}
		if version != SchemaVersion {
			t.Fatalf("the ladder left the file at version %d, want %d", version, SchemaVersion)
		}
		// Idempotent: a file already at the top costs one pragma read and
		// changes nothing.
		if err := migrate(context.Background(), conn); err != nil {
			t.Fatal(err)
		}
	})
}

// countingSender is the seam ADR 16 asks for, counting rather than sending.
type countingSender struct{ sent int }

func (c *countingSender) Send(Message) error { c.sent++; return nil }

// A transaction that cannot end cleanly does not poison the handle.
//
// Found on 2026-09-24 by the tests above, which had to roll a stale
// transaction back by hand between cases to stay green: when the COMMIT or
// the ROLLBACK failed, `exclusive` and `inTx` returned their pinned
// connection to the pool with the driver's transaction still open. With one
// connection in the pool that was the whole handle -- reads answered, writes
// went nowhere, and the next BEGIN IMMEDIATE was refused. `writes.go` argues
// the fix; this holds it, on both shapes and on both ways of failing.
//
// The probe is a bare ROLLBACK on the pool: on a clean handle SQLite refuses
// it ("no transaction is active"), and on a poisoned one it succeeds -- so
// the assertion is that the ROLLBACK FAILS. Then the write that follows has
// to land, because a discarded connection is replaced by a fresh one and
// nothing about the failure is remembered.
func TestAWriteWhoseCommitOrRollbackFailsDoesNotPoisonTheHandle(t *testing.T) {
	t.Parallel()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	keeper := oneAccount(t, db)
	other, err := Create(ctx, db, "page", "page@example.test", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSession(ctx, db, keeper.ID); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		what   string
		budget int
		write  func() error
	}{
		// exclusive: BEGIN IMMEDIATE, the UPDATE, the COMMIT. Budget 2 fails
		// the COMMIT; budget 1 fails the UPDATE and then the ROLLBACK too.
		{"an exclusive write whose commit fails", 2,
			func() error { return SetAdmin(ctx, db, other.ID, true) }},
		{"an exclusive write whose rollback fails", 1,
			func() error { return SetAdmin(ctx, db, other.ID, true) }},
		// inTx: BEGIN, the password UPDATE, the session DELETE, the COMMIT.
		{"a deferred transaction whose commit fails", 3,
			func() error { _, err := SetPassword(ctx, db, keeper.ID, "another passphrase"); return err }},
		{"a deferred transaction whose rollback fails", 1,
			func() error { _, err := SetPassword(ctx, db, keeper.ID, "another passphrase"); return err }},
	} {
		fault.Heal()
		fault.After(c.budget)
		err := c.write()
		fault.Heal()
		if err == nil {
			t.Fatalf("%s: the write claimed to land", c.what)
		}
		if _, err := db.ExecContext(ctx, "ROLLBACK"); err == nil {
			t.Fatalf("%s: the pool handed back a connection with the transaction still open",
				c.what)
		}
		if err := SetAdmin(ctx, db, other.ID, true); err != nil {
			t.Fatalf("%s: the write after it was refused: %v", c.what, err)
		}
		fresh, err := GetByID(ctx, db, other.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !fresh.IsAdmin {
			t.Fatalf("%s: the write after it answered nil and changed nothing", c.what)
		}
		if err := SetAdmin(ctx, db, other.ID, false); err != nil {
			t.Fatal(err)
		}
	}
}
