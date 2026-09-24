package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
)

// The `app.db` nobody handed in, and the budget a link preview spends.
//
// [API.accountsDB] prefers the handle the door opened at start and otherwise
// opens one **lazily** — and that second road is not tidiness. On a fresh
// volume `app.db` can appear *after* boot: minted by the CLI, or by a ladder
// run the door started without. The lazy open is what makes the first login
// after that work rather than answering out of a decision taken minutes
// earlier. Every test in this package hands a handle in, so the road that
// matters on a fresh machine was the one nothing drove.
//
// `refusals_test.go`'s `TestTheLazyDatabaseOpenHandlesEveryStateOfTheFile`
// asks the same three questions of [API.appDB], which is the **read** side and
// a different handle with a different lazy field. The two are separate on
// purpose — the read one is opened `mode=ro`, so a write through it would fail
// at the driver rather than at the gate that is supposed to answer — and a
// road proven on one of them is not proven on the other.

// A database that appeared after boot is found, and found once.
func TestADatabaseThatAppearedAfterBootIsOpenedOnFirstUse(t *testing.T) {
	t.Parallel()
	path := appDB(t)
	a := New(Config{AppDBPath: path,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	db, present := a.accountsDB()
	if !present || db == nil {
		t.Fatal("a database on disk was not found by the lazy open")
	}
	// It really is that database, not an empty one minted beside it.
	user, err := auth.Get(context.Background(), db, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("the lazily-opened handle does not see the accounts that are in it")
	}

	// And a second ask is the same handle: the lazy open is memoised, so a
	// polled route does not open a file per request.
	again, present := a.accountsDB()
	if !present || again != db {
		t.Error("the lazy open was not memoised; every ask opens app.db again")
	}
}

// The two ways there is nothing to open, each answered as an **empty**
// database rather than as an error -- `internal/auth/writes.go` argues why
// that is the honest answer, and why nothing here creates the file.
func TestWithNothingToOpenTheAccountsDatabaseIsSimplyAbsent(t *testing.T) {
	t.Parallel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	// No path at all.
	if _, present := New(Config{Logger: quiet}).accountsDB(); present {
		t.Error("an instance with no path found a database")
	}
	// A path naming a file that has never been written.
	path := filepath.Join(t.TempDir(), "app.db")
	absentFile := New(Config{Logger: quiet, AppDBPath: path})
	if _, present := absentFile.accountsDB(); present {
		t.Error("a path naming nothing found a database")
	}
	// And nothing was created on the way past: only the boot ladder may make
	// the file, which is `internal/auth/writes.go`'s standing rule and the
	// reason an absent database is read as an empty one.
	if _, err := os.Stat(path); err == nil {
		t.Error("looking for app.db created it")
	}
}

// The preview's own budget.
//
// It is a **separate bucket from the claim's**, and the reason is worth
// pinning: a page that previews on mount would otherwise spend the budget a
// person needs to actually redeem their link, so somebody who opened the
// invite twice could no longer use it. What this asserts is that the preview
// does run out — an unbudgeted preview is an oracle for guessing tokens — and
// that running out of previews leaves the claim itself workable.
func TestThePreviewRunsOutOfTriesWithoutSpendingTheClaims(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()
	token := rig.inviteToken(t, "waiting")

	// Wrong guesses, until the bucket is empty. The bound is generous on
	// purpose; the loop is bounded harder so a budget that stopped spending
	// fails here rather than running forever.
	throttled := false
	for i := 0; i < 200 && !throttled; i++ {
		rec := rig.call(t, anonymous, "POST", "/api/auth/claim/preview",
			`{"token":"definitely-not-a-token"}`, "")
		switch rec.Code {
		case http.StatusBadRequest:
			continue
		case http.StatusTooManyRequests:
			throttled = true
			if !strings.Contains(detail(t, rec), "wait and try again") {
				t.Errorf("the refusal reads %q", detail(t, rec))
			}
		default:
			t.Fatalf("a wrong guess answered %d: %s", rec.Code, rec.Body)
		}
	}
	if !throttled {
		t.Fatal("two hundred wrong guesses were all answered -- the preview " +
			"is an oracle for guessing tokens")
	}

	// The real link still redeems: the preview's bucket is its own, so
	// somebody else's guessing has not locked this person out of their invite.
	rec := rig.call(t, anonymous, "POST", "/api/auth/claim",
		`{"token":"`+token+`","password":"`+goodPassword+`"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("a spent preview budget cost the claim itself: %d %s",
			rec.Code, rec.Body)
	}
}

// A preview over a database whose token table has gone refuses as a fault and
// not as a bad link: "that link is not valid" would send somebody to ask for
// a new invite, and the invite is fine.
func TestAPreviewOverAHalfMissingDatabaseIsAFaultNotABadLink(t *testing.T) {
	t.Parallel()
	rig := dropping(t, "auth_tokens")
	defer rig.close()

	rec := rig.call(t, anonymous, "POST", "/api/auth/claim/preview",
		`{"token":"whatever"}`, "")
	if rec.Code == http.StatusOK {
		t.Fatalf("a preview answered 200 over a database that will not answer: %s",
			rec.Body)
	}
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("a database fault was reported as a bad link, which sends "+
			"somebody to ask for an invite they already have: %s", rec.Body)
	}
	if strings.TrimSpace(detail(t, rec)) == "" {
		t.Fatalf("the refusal carries nothing a person could read: %s", rec.Body)
	}
}
