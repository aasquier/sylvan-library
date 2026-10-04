package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/alexedwards/argon2id"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/config"
)

// The last branches in `app.db`, and the fixtures that could reach them.
//
// `errorpaths_test.go` closes a handle (the first statement fails) and
// `halfwritten_test.go` spends a budget (a statement in the middle fails).
// What was left after those two is a shorter list than it looks, and it is
// three shapes rather than a dozen branches:
//
//   - **the row that is gone by the time it is read back.** Two writes here
//     insert or update and then re-read what they wrote, and both refuse
//     rather than hand a nil account back with no error. A healthy file never
//     does that; a file carrying a trigger does, which is what makes the
//     refusal drivable — and a trigger is not a contrivance for it, it is the
//     smallest thing that behaves like a second writer deleting underneath.
//   - **a column holding what a scan cannot take.** SQLite applies affinity
//     rather than enforcing it, so one `UPDATE` leaves text in an INTEGER
//     column and the reader has to say it cannot read the row instead of
//     reporting one account fewer.
//   - **the rollback that fails too.** A volume that has gone refuses the
//     ROLLBACK as well as the write, and `inTx` then has to get the connection
//     out of the pool — the live fault its own comment records, and the only
//     one no budget could reach, because `Tx.Rollback` goes to the driver's
//     transaction rather than through a connection's Exec.

// triggered is a real migrated `app.db`, read-write, carrying one extra
// trigger: whatever the caller hands in. It stands in for a file something
// else is writing to — the row this process just wrote being gone before this
// process reads it back.
func triggered(t *testing.T, trigger string) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db := OpenReadWrite(path)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	return db
}

// An account whose row is gone the moment it is made is refused, not returned
// as nothing at all. The caller of `Create` dereferences what it is handed, so
// "no error and no account" is the one answer this must never give.
func TestAnAccountWhoseRowVanishesUnderTheInsertIsRefused(t *testing.T) {
	t.Parallel()
	db := triggered(t, `CREATE TRIGGER vanish AFTER INSERT ON users BEGIN`+
		` DELETE FROM users WHERE id = NEW.id; END`)

	made, err := Create(context.Background(), db, "ghost", "ghost@example.test", false)
	if err == nil {
		t.Fatalf("an account that is not there came back as %+v", made)
	}
	if !errors.Is(err, ErrNoSuchUser) {
		t.Errorf("the refusal is %v, want it to name the missing account", err)
	}
	if made != nil {
		t.Errorf("a refused creation also handed back %+v", made)
	}
}

// The same question for a redeemed link: the account is read back after the
// transaction commits, and an account that has gone by then makes the link
// invalid rather than making the answer nil.
func TestARedeemedLinkWhoseAccountVanishesIsRefused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := triggered(t, `CREATE TRIGGER vanish AFTER UPDATE OF password_hash ON users`+
		` BEGIN DELETE FROM users WHERE id = NEW.id; END`)

	user, err := Create(ctx, db, "invited", "invited@example.test", false)
	if err != nil {
		t.Fatal(err)
	}
	token, err := IssueToken(ctx, db, user.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}

	claimed, err := RedeemToken(ctx, db, token, "a long enough passphrase",
		PurposeInvite, "")
	if err == nil {
		t.Fatalf("a link against an account that is gone was redeemed as %+v", claimed)
	}
	if !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("the refusal is %v, want the same sentence a bad link gets", err)
	}
	if claimed != nil {
		t.Errorf("a refused redemption also handed back %+v", claimed)
	}
}

// poisoned writes a value the column's affinity cannot convert, through a
// handle with foreign keys off -- one row, named by its own WHERE, because a
// whole-table UPDATE is refused by any UNIQUE the column takes part in.
//
// It is what a hand-run repair during an incident leaves behind, and the
// question it asks of every reader is the one that matters: a row it cannot
// read has to be said out loud, never counted as absent.
func poisoned(t *testing.T, path, statement string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

// The account list says so when one row cannot be read, rather than returning
// the accounts it managed and leaving the caller to think that is all of them.
// An admin surface that quietly loses the admin is how a lockout happens.
func TestAnAccountListSaysSoWhenARowCannotBeRead(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db := OpenReadWrite(path)
	t.Cleanup(func() { _ = db.Close() })
	for _, name := range []string{"ada", "bruno"} {
		if _, err := Create(ctx, db, name, name+"@example.test", false); err != nil {
			t.Fatal(err)
		}
	}
	if all, err := AllUsers(ctx, db); err != nil || len(all) != 2 {
		t.Fatalf("the healthy list is (%d accounts, %v)", len(all), err)
	}

	poisoned(t, path, `UPDATE users SET is_admin = 'yes' WHERE username = 'ada'`)

	all, err := AllUsers(ctx, db)
	if err == nil {
		t.Fatalf("a row that cannot be read came back as a list of %d", len(all))
	}
	if all != nil {
		t.Errorf("a failed list also returned %d accounts", len(all))
	}
}

// The admin count refuses a row it cannot read instead of answering "no usable
// admins", and the difference is the whole of ADR 17: an empty answer is what
// the last-admin guard reads as permission to demote the last one.
func TestTheAdminCountRefusesAnUnreadableIDRatherThanReportingNone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A `users` table whose id is not an INTEGER PRIMARY KEY, which is what a
	// restore rebuilt by hand leaves: the predicate still matches, and the id
	// is not a number. `UsableAdminIDs` takes any querier, which is the seam
	// its own comment is about.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "rebuilt.db")+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE users (id TEXT, is_admin INTEGER,` +
		` password_hash TEXT, disabled_at TEXT);` +
		` INSERT INTO users VALUES ('not a number', 1, 'a hash', NULL)`); err != nil {
		t.Fatal(err)
	}

	ids, err := UsableAdminIDs(ctx, db)
	if err == nil {
		t.Fatalf("an unreadable admin row was counted as %v", ids)
	}
	if len(ids) != 0 {
		t.Errorf("a failed count also returned %v", ids)
	}
}

// A password the rules refuse costs nothing: it is checked before a
// transaction is opened, so a weak one never reaches the file at all.
func TestSettingAPasswordRefusesAWeakOneBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()
	// A closed handle is the proof that nothing was asked of the database: if
	// the check happened later, this would fail at the driver instead.
	revoked, err := SetPassword(context.Background(), closedDB(t), 1, "short")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("a five-character password answered %v", err)
	}
	if revoked != 0 {
		t.Errorf("a refused password reported %d sessions revoked", revoked)
	}
}

// A rename the rules refuse is refused in the same breath, and the account
// keeps the name it had.
func TestRenamingAnAccountRefusesANameTheRulesRefuse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, _ := faultyAuth(t)
	user := oneAccount(t, db)

	name, err := SetUsername(ctx, db, user.ID, "not a legal name!")
	if err == nil {
		t.Fatalf("an illegal handle was accepted as %q", name)
	}
	if name != "" {
		t.Errorf("a refused rename handed back %q", name)
	}
	after, err := Get(ctx, db, user.Username)
	if err != nil || after == nil {
		t.Fatalf("the account went missing: (%v, %v)", after, err)
	}
}

// A login that has to upgrade an old hash says so when the upgrade cannot
// land, rather than signing somebody in over a write that went nowhere.
//
// The budget is swept rather than counted: the question is *at no point in
// this call may a failure become a false answer*, and the sweep asks it of
// every statement the login makes -- the lookup, the rehash, the re-read --
// without naming which one is which.
func TestALoginWhoseRehashCannotLandSaysSoRatherThanSigningYouIn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, fault := faultyAuth(t)

	const password = "a long enough passphrase"
	// A hash at weaker parameters than the pinned profile, which is what
	// `NeedsRehash` is for and the only way the rehash branch runs at all.
	weak, err := argon2id.CreateHash(password, &argon2id.Params{
		Memory: 4096, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32})
	if err != nil {
		t.Fatal(err)
	}
	if !NeedsRehash(weak) {
		t.Fatal("the fixture hash is not weaker than the profile")
	}
	user, err := Create(ctx, db, "ada", "ada@example.test", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE users SET password_hash = ? WHERE id = ?", weak, user.ID); err != nil {
		t.Fatal(err)
	}

	refused, signedIn := 0, 0
	for budget := 0; budget <= 3; budget++ {
		fault.After(budget)
		got, err := Authenticate(ctx, db, "ada", password)
		fault.Heal()
		switch {
		case err != nil:
			refused++
			if got != nil {
				t.Fatalf("budget %d: a failed login handed back %+v", budget, got)
			}
		default:
			signedIn++
			if got == nil || got.Username != "ada" {
				t.Fatalf("budget %d: a login with no error answered %+v", budget, got)
			}
		}
	}
	if refused == 0 || signedIn == 0 {
		t.Fatalf("the sweep saw %d refusals and %d logins; it needs both floors "+
			"or it measured one state four times", refused, signedIn)
	}
}

// A maintainer address of nothing but spaces is skipped, and skipped without
// reading the database: a closed handle proves it never got that far. The
// reconciler's standing rule is that a malformed preference is not fatal, and
// whitespace is the spelling of it that looks like an address and is not.
func TestAMaintainerAddressOfNothingButSpaceIsSkipped(t *testing.T) {
	t.Parallel()
	if err := EnsureMaintainer(context.Background(), closedDB(t),
		config.Config{AdminEmail: "   "}); err != nil {
		t.Fatalf("a blank maintainer address refused to boot: %v", err)
	}
}

// A ladder with a rung missing applies nothing. The scripts are read whole
// before the first one runs, so the file is untouched -- which is the
// difference between a boot that refuses and a boot that half-migrates.
func TestALadderMissingARungRefusesBeforeTouchingTheFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	// Every rung but the last, read out of the embedded ladder itself rather
	// than typed: a hand-written stand-in would be a different ladder.
	short := fstest.MapFS{}
	for i := 1; i < SchemaVersion; i++ {
		name := fmt.Sprintf("migrations/%04d.sql", i)
		body, err := fs.ReadFile(migrationFS, name)
		if err != nil {
			t.Fatal(err)
		}
		short[name] = &fstest.MapFile{Data: body}
	}

	if err := migrate(ctx, conn, short); err == nil {
		t.Fatal("a ladder missing its top rung migrated the file anyway")
	}
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Errorf("the refused ladder left the file at version %d", version)
	}
	var tables int
	if err := conn.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type = 'table'").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("the refused ladder created %d table(s)", tables)
	}
}

// A transaction whose rollback fails as well takes its connection out of the
// pool, and the proof is the write *after* it: with one connection in the
// handle, a transaction left standing would refuse every later BEGIN and the
// instance would answer reads while writing nothing until it restarted. That
// was a live fault; this is the test that would have seen it.
func TestAFailedRollbackTakesTheConnectionWithIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, fault := faultyAuth(t)
	user := oneAccount(t, db)

	// The BEGIN lands, the UPDATE does not, and neither does the rollback.
	fault.After(1)
	fault.FailRollbacks()
	if _, err := SetPassword(ctx, db, user.ID, "the first new passphrase"); err == nil {
		t.Fatal("a password was set through a database that refused the write")
	}
	fault.Heal()

	// The handle has to be usable again, and the only thing that can have made
	// it so is the connection having been thrown away rather than pooled.
	const second = "the second new passphrase"
	if _, err := SetPassword(ctx, db, user.ID, second); err != nil {
		t.Fatalf("the handle never recovered from the failed rollback: %v", err)
	}
	signedIn, err := Authenticate(ctx, db, user.Username, second)
	if err != nil || signedIn == nil {
		t.Fatalf("the recovered password does not sign in: (%v, %v)", signedIn, err)
	}
}

// cancelAt is a log handler that cancels a context when the nth line lands.
//
// It is how a shutdown is made to arrive *between* two purges rather than
// before all three. The sweeper's log is a value a caller hands in, so the one
// thing that happens between the session sweep and the link sweep is a line
// being written -- and a shutdown landing exactly there is the case the
// sweep's own comment is about: a tick crossing a stop, where the remaining
// tables are left for the next boot instead of being reported as failures.
type cancelAt struct {
	after  int
	seen   int
	cancel context.CancelFunc
}

func (h *cancelAt) Enabled(context.Context, slog.Level) bool { return true }

func (h *cancelAt) Handle(context.Context, slog.Record) error {
	h.seen++
	if h.seen == h.after {
		h.cancel()
	}
	return nil
}

func (h *cancelAt) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *cancelAt) WithGroup(string) slog.Handler      { return h }

// A sweep that is stopped partway through leaves the rest of the tables alone
// and announces nothing. Three tables, so three places a shutdown can land;
// the first is held elsewhere and these are the two after it.
func TestASweepStoppedBetweenTablesLeavesTheRestForTheNextBoot(t *testing.T) {
	t.Parallel()
	for _, after := range []int{1, 2} {
		t.Run(map[int]string{1: "after the sessions", 2: "after the links"}[after],
			func(t *testing.T) {
				t.Parallel()
				db, fault := faultyAuth(t)
				ctx, cancel := context.WithCancel(context.Background())
				t.Cleanup(cancel)
				handler := &cancelAt{after: after, cancel: cancel}
				announced := false
				s := NewSweeper(SweeperConfig{DB: db, Log: slog.New(handler),
					Swept: func(SweepCounts) { announced = true }})

				// Nothing answers, so every purge fails -- and the shutdown
				// lands on the line the first failure writes.
				fault.After(0)
				got := s.SweepOnce(ctx)
				fault.Heal()

				if got != (SweepCounts{}) {
					t.Errorf("a sweep that was stopped reported %+v", got)
				}
				if announced {
					t.Error("a sweep that was stopped announced itself as finished")
				}
				if handler.seen != after {
					t.Errorf("the sweep wrote %d lines, want %d -- it carried on "+
						"past the stop instead of leaving the rest for the next boot",
						handler.seen, after)
				}
			})
	}
}

// The mail sender posts to the provider by default and says so when it cannot
// build the request at all. Both halves of the one input that can fail: the
// address it is pointed at.
func TestTheMailSenderPostsWhereItIsPointedAndRefusesWhatItCannotBuild(t *testing.T) {
	t.Parallel()

	t.Run("the default is the provider's own address", func(t *testing.T) {
		t.Parallel()
		var asked string
		sender, err := NewResendSender("a-key", "squire@example.test",
			func(req *http.Request) (int, []byte, error) {
				asked = req.URL.String()
				return http.StatusOK, []byte(`{"id":"1"}`), nil
			})
		if err != nil {
			t.Fatal(err)
		}
		if err := sender.Send(Message{To: "ada@example.test", Subject: "s", Body: "b"}); err != nil {
			t.Fatal(err)
		}
		if asked != ResendEndpoint {
			t.Errorf("the request went to %q, want the provider's own URL", asked)
		}
	})

	t.Run("a real post, over the real transport", func(t *testing.T) {
		t.Parallel()
		var gotAuth, gotAgent, gotBody string
		srv := httptest.NewServer(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				gotAgent = r.Header.Get("User-Agent")
				buf := make([]byte, 512)
				n, _ := r.Body.Read(buf)
				gotBody = string(buf[:n])
				w.WriteHeader(http.StatusOK)
			}))
		t.Cleanup(srv.Close)

		// No transport handed in, so the real one runs: the real client, the
		// real headers, the real body, with only the address replaced.
		sender, err := NewResendSender("a-key", "squire@example.test", nil)
		if err != nil {
			t.Fatal(err)
		}
		sender.endpoint = srv.URL
		if err := sender.Send(Message{To: "ada@example.test",
			Subject: "An invitation", Body: "Come and see."}); err != nil {
			t.Fatal(err)
		}
		if gotAuth != "Bearer a-key" {
			t.Errorf("the Authorization header is %q", gotAuth)
		}
		if gotAgent != UserAgent {
			t.Errorf("the User-Agent is %q, want %q", gotAgent, UserAgent)
		}
		for _, want := range []string{"ada@example.test", "An invitation", "Come and see."} {
			if !strings.Contains(gotBody, want) {
				t.Errorf("the body does not carry %q: %s", want, gotBody)
			}
		}
	})

	t.Run("an address that is not one", func(t *testing.T) {
		t.Parallel()
		sender, err := NewResendSender("a-key", "squire@example.test",
			func(*http.Request) (int, []byte, error) {
				t.Error("a request that could not be built was posted anyway")
				return http.StatusOK, nil, nil
			})
		if err != nil {
			t.Fatal(err)
		}
		sender.endpoint = ":://not a url"
		err = sender.Send(Message{To: "ada@example.test", Subject: "s", Body: "b"})
		if !errors.Is(err, ErrEmailNotSent) {
			t.Fatalf("an unbuildable request answered %v", err)
		}
	})
}
