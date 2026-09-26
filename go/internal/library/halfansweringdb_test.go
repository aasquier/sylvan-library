package library_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The SQL tier when the volume goes away *partway through* a call.
//
// `closedhandles_test.go` takes a handle away before the call starts, which
// answers what happens when the **first** statement fails. Several of these
// calls are longer than one statement — a share flag reads the row and then
// updates it, a build deletes the old artifacts and inserts the new ones and
// then commits, the crypt's emptying deletes and then counts — and the arms
// between the first statement and the last had no fixture at all. They are the
// arms where the tier has done **some** of the work and has to decide what to
// tell the caller, and the wrong answer there is not an error: it is a caller
// told their deck was saved when the transaction rolled back.
//
// `app.db` is a file on a mount (ADR 23). A mount that goes between two
// statements of one call is the fault this is; `authtest.OpenFaulty` is that
// fault as a fixture, with a counter for how many more statements the handle
// will answer.
//
// **Budgets are swept rather than counted.** A counted budget restates today's
// statement sequence and goes quietly meaningless the day a query is added to
// the middle of a write. The sweep asks the question the tier actually has to
// answer: *at no point in this call may a failure be reported as a success, or
// an unreadable shelf as an empty one.*

// faultyTier is a writable SQL tier holding one deck, over a handle with a
// statement budget. Read and write are the same handle: the question here is
// what a statement failure does once a call is under way, which is the same
// question on either side.
//
// The deck is created and the budget lifted before the fixture is handed back,
// so every refusal below is the budget's doing and not an empty library's.
func faultyTier(t *testing.T) (*library.SQLSource, *authtest.Fault) {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := auth.Create(ctx, db, "alice", "alice@example.com", false); err != nil {
		t.Fatal(err)
	}
	src := library.NewSQLSource(db, db, 1, true, false)
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if _, err := src.WriteArtifacts(ctx, "gyome",
		artifacts.Files{{Name: "primer-quick.md", Text: "# a primer\n"}}); err != nil {
		t.Fatal(err)
	}
	fault.Heal()
	return src, fault
}

// theWrites is every write the tier offers, named, so a failure says which
// one reported a success it had not earned. Discovered by hand rather than
// from a table because there is no table — but `closedhandles_test.go` drives
// the same list, and the two going out of step is visible in a diff.
func theWrites(src *library.SQLSource) []struct {
	what string
	run  func(context.Context) error
} {
	return []struct {
		what string
		run  func(context.Context) error
	}{
		{"WriteText", func(ctx context.Context) error {
			return src.WriteText(ctx, "gyome",
				strings.Replace(sqlDeck, "Gyome", "Edited", 1))
		}},
		{"Create", func(ctx context.Context) error {
			return src.Create(ctx, "another", sqlDeck)
		}},
		// `true` rather than `false`: this fixture's deck is unshared, so
		// `false` is the standing no-op that never reaches the write at all.
		// (The file tier is the other way round — see COVERAGE.md.)
		{"SetShared", func(ctx context.Context) error {
			return src.SetShared(ctx, "gyome", true)
		}},
		{"SetColiseumAtNight", func(ctx context.Context) error {
			return src.SetColiseumAtNight(ctx, "gyome", true)
		}},
		{"WriteArtifacts", func(ctx context.Context) error {
			_, e := src.WriteArtifacts(ctx, "gyome",
				artifacts.Files{{Name: "primer-quick.md", Text: "# a newer primer\n"}})
			return e
		}},
		{"Delete", func(ctx context.Context) error {
			_, e := src.Delete(ctx, "gyome")
			return e
		}},
		{"Empty", func(ctx context.Context) error { _, e := src.Empty(ctx); return e }},
	}
}

// Every write, at every budget from "the next statement fails" up past the
// longest of them.
//
// The assertion is not that it failed — a budget wide enough for the whole
// call must succeed, and does. It is that a call which **did** hit the budget
// says so, because the one thing a half-finished write may not do is report
// the work as done.
func TestNoSQLWriteReportsSuccessWhenTheDatabaseStopsAnsweringPartwayThrough(t *testing.T) {
	t.Parallel()
	// Above the longest write: a build is a begin, a delete, an insert per
	// file, an update and a commit.
	const widest = 10

	for _, name := range writeNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			landed := 0
			for budget := 0; budget <= widest; budget++ {
				// A fresh tier per budget: a write that landed would change
				// what the next budget is measuring.
				src, fault := faultyTier(t)
				var tc struct {
					what string
					run  func(context.Context) error
				}
				for _, candidate := range theWrites(src) {
					if candidate.what == name {
						tc = candidate
					}
				}
				fault.After(budget)
				err := tc.run(t.Context())
				fault.Heal()
				if err != nil {
					// A refusal must not read as "no such deck": a 404 would
					// tell somebody their deck had been deleted.
					if library.IsNotFound(err) {
						t.Errorf("%s at budget %d reported a missing deck when "+
							"the database was the thing that was missing: %v",
							name, budget, err)
					}
					continue
				}
				landed++
			}
			// The anti-vacuity floor: a name whose every budget refused would
			// mean the fixture never lets this write work at all, and the
			// subtest would be proving nothing about the arms in between.
			if landed == 0 {
				t.Errorf("%s never succeeded at any budget up to %d -- this "+
					"subtest is refusing for a reason that is not the budget",
					name, widest)
			}
		})
	}
}

// writeNames is the same list by name, so the sweep above can run one write
// per subtest against a fixture built inside that subtest.
func writeNames() []string {
	out := []string{}
	for _, tc := range theWrites(nil) {
		out = append(out, tc.what)
	}
	return out
}

// Every read, at every budget: a shelf the tier cannot read is never reported
// as a shelf with nothing on it.
//
// That distinction is the whole of `closedhandles_test.go`'s argument and it is
// the one a *partial* failure can break where a total one cannot: a read whose
// first statement worked has something to hand back.
func TestNoSQLReadReportsEmptinessWhenTheDatabaseStopsAnsweringPartwayThrough(t *testing.T) {
	t.Parallel()
	const widest = 8

	for _, budget := range allBudgets(widest) {
		src, fault := faultyTier(t)
		ctx := t.Context()
		fault.After(budget)
		for _, tc := range []struct {
			what  string
			run   func() (int, error)
			whole int
		}{
			{"Slugs", func() (int, error) {
				out, err := src.Slugs(ctx)
				return len(out), err
			}, 1},
			{"All", func() (int, error) {
				out, err := src.All(ctx)
				return len(out), err
			}, 1},
			{"Artifacts", func() (int, error) {
				out, err := src.Artifacts(ctx, "gyome")
				return len(out), err
			}, 1},
		} {
			got, err := tc.run()
			if err != nil {
				continue
			}
			if got < tc.whole {
				t.Errorf("%s answered %d of %d at budget %d without an error -- "+
					"a shelf read short reads as a shelf that is short",
					tc.what, got, tc.whole, budget)
			}
		}
		fault.Heal()
	}
}

// allBudgets is 0..widest, as a slice, so the loop above reads as a sweep
// rather than as an index.
func allBudgets(widest int) []int {
	out := make([]int, 0, widest+1)
	for i := 0; i <= widest; i++ {
		out = append(out, i)
	}
	return out
}

// A read cut short partway down its result set, which is the only fault that
// hands back a **shorter list** rather than an error.
//
// Every loop over a result set here ends by asking `rows.Err()` whether the
// walk failed or merely finished. Without that question a shelf of four decks
// reads back as a shelf of two, and nothing anywhere says so — the worst shape
// a read can fail in, and the one a healthy database and a gone handle both
// hide.
func TestNoSQLReadHandsBackAShortListWhenTheIterationFails(t *testing.T) {
	t.Parallel()
	src, fault := faultyTier(t)
	ctx := t.Context()
	// Four decks and two artifacts, so a set can be cut short in the middle
	// rather than at its end.
	for _, slug := range []string{"cats", "dinos", "stompy"} {
		if err := src.Create(ctx, slug,
			strings.Replace(sqlDeck, "gyome", slug, -1)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := src.WriteArtifacts(ctx, "gyome", artifacts.Files{
		{Name: "primer-quick.md", Text: "# a primer\n"},
		{Name: "swaps.md", Text: "# swaps\n"},
	}); err != nil {
		t.Fatal(err)
	}

	for rows := 0; rows <= 6; rows++ {
		fault.RowsAfter(rows)
		slugs, slugErr := src.Slugs(ctx)
		all, allErr := src.All(ctx)
		shelf, shelfErr := src.Artifacts(ctx, "gyome")
		fault.Heal()

		if slugErr == nil && len(slugs) != 4 {
			t.Errorf("Slugs answered %d of 4 decks with no error at row budget %d",
				len(slugs), rows)
		}
		if allErr == nil && len(all) != 4 {
			t.Errorf("All answered %d of 4 decks with no error at row budget %d",
				len(all), rows)
		}
		if shelfErr == nil && len(shelf) != 2 {
			t.Errorf("Artifacts answered %d of 2 files with no error at row "+
				"budget %d", len(shelf), rows)
		}
	}
}

// `Visible` is the browse tab's whole answer, and it is three reads deep: the
// caller's own tier, the showcase, then one `SourceFor` per owner who has
// shared something. A budget that runs out between them is the case
// `visible_test.go`'s closed handle cannot reach — there the very first read
// fails, and the loop over other people's shelves is never entered at all.
//
// The claim is the one that file makes, with the partial case added: a shelf
// that could not be fully built must **fail**, because a list one owner short
// is indistinguishable from a list of everyone who has shared anything.
func TestVisibleFailsRatherThanDroppingAnOwnerItCouldNotResolve(t *testing.T) {
	t.Parallel()
	const widest = 12

	// The healthy answer first, so "shorter than it should be" has a number.
	// Bob rather than the maintainer: alice's own tier *is* the showcase, so
	// her shelf dedupes to two owners and the loop this test is about — one
	// `SourceFor` per other owner who has shared something — runs a single
	// time. Bob's shelf is his own, the showcase, and carol's.
	db, fault, decks := faultyAccounts(t)
	lib := libFor(t, db, decks, auth.Scope{UserID: 2, Username: "bob",
		Authenticated: true}, "alice")
	whole, err := lib.Visible(t.Context())
	if err != nil {
		t.Fatalf("the healthy shelf: %v", err)
	}
	if len(whole) < 3 {
		t.Fatalf("the fixture's shelf is %d owners long, not the three it "+
			"should be (mine, the showcase, carol's shared)", len(whole))
	}

	refused, complete := 0, 0
	for budget := 0; budget <= widest; budget++ {
		fault.After(budget)
		got, err := lib.Visible(t.Context())
		fault.Heal()
		if err != nil {
			refused++
			continue
		}
		if len(got) != len(whole) {
			t.Errorf("at budget %d the shelf came back %d owners long instead "+
				"of %d, with no error -- an owner was dropped silently",
				budget, len(got), len(whole))
		}
		complete++
	}
	// Both halves have to happen, or the sweep measured one state twice.
	if refused == 0 || complete == 0 {
		t.Errorf("%d budgets refused and %d answered in full -- the sweep never "+
			"crossed from one to the other", refused, complete)
	}
}

// faultyAccounts is `visible_test.go`'s accounts over a handle with a statement
// budget: a maintainer, a second account with one shared deck and one private,
// and a third with one private one.
func faultyAccounts(t *testing.T) (*sql.DB, *authtest.Fault, string) {
	t.Helper()
	db, fault, err := authtest.OpenFaulty(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, row := range []struct {
		name, email string
		admin       bool
	}{
		{"alice", "alice@example.com", true},
		{"bob", "bob@example.com", false},
		{"carol", "carol@example.com", false},
	} {
		if _, err := auth.Create(ctx, db, row.name, row.email, row.admin); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		owner  int64
		slug   string
		shared int
	}{{2, "bobs-shared", 1}, {2, "bobs-private", 0}, {3, "carols-shared", 1}} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO user_decks (owner_id, slug, name, yaml, shared,`+
				` created_at, updated_at)`+
				` VALUES (?, ?, ?, ?, ?, '2026-08-28T00:00:00+00:00',`+
				` '2026-08-28T00:00:00+00:00')`,
			row.owner, row.slug, row.slug,
			"name: "+row.slug+"\ncards: []\n", row.shared); err != nil {
			t.Fatal(err)
		}
	}
	fault.Heal()
	return db, fault, t.TempDir()
}
