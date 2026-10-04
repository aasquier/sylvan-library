package cards_test

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The reader against a pool that answers for a while and then stops.
//
// `unreadablepool_test.go` is the other half: a pool with no tables, where every
// read fails at its first query. What it cannot reach is a read **cut short** --
// the query ran, some rows came back, and the walk failed instead of ending. For
// this package that is the worst fault available, because every one of these
// reads is about telling somebody what card they are holding:
//
//   - a corner lookup that saw one of two matching printings answers a *name*
//     where the honest answer is "this pair names two different cards";
//   - a set-code list cut short turns a real set code into one the pool has
//     never heard of, and the corner tier then falls through to the title;
//   - a title shortlist cut short drops the right card off the bottom of five.
//
// `pooltest.OpenFaultyPool` is the fixture (`pool.Connect` argues the seam that
// made it reachable from outside `internal/pool`). The sweep is over row budgets
// rather than a counted one: what is asserted is that **a short answer is never
// handed back without an error**, wherever in the walk the library goes away.
func TestNoCardReadAnswersAShortListWhenThePoolGoesAwayMidWalk(t *testing.T) {
	t.Parallel()

	// The whole answers first, so "short" has numbers.
	type whole struct {
		codes      int
		printing   string
		candidates int
		suggestion int
	}
	var want whole
	if err := pooltest.Open(t).Use(t.Context(), func(c *pool.Conn) error {
		codes, err := cards.SetCodes(t.Context(), c)
		if err != nil {
			return err
		}
		want.codes = len(codes)
		if want.printing, err = cards.ByPrinting(t.Context(), c, "MH3", "1"); err != nil {
			return err
		}
		titles, err := cards.ByTitle(t.Context(), c, "Sol Ring", 5)
		if err != nil {
			return err
		}
		want.candidates = len(titles)
		typed, err := cards.Suggest(t.Context(), c, "sol", 5)
		if err != nil {
			return err
		}
		want.suggestion = len(typed)
		return nil
	}); err != nil {
		t.Fatalf("the healthy reader: %v", err)
	}
	if want.codes < 2 || want.candidates < 2 || want.suggestion < 1 {
		t.Fatalf("the fixture answers %+v -- these sweeps need result sets long "+
			"enough to cut in the middle", want)
	}

	for _, tc := range []struct {
		what string
		// run answers a size and an error, so "shorter than whole" is one
		// comparison for every read.
		run  func(c *pool.Conn) (int, error)
		size int
	}{
		{"the set codes", func(c *pool.Conn) (int, error) {
			got, err := cards.SetCodes(t.Context(), c)
			return len(got), err
		}, want.codes},
		{"a title shortlist", func(c *pool.Conn) (int, error) {
			got, err := cards.ByTitle(t.Context(), c, "Sol Ring", 5)
			return len(got), err
		}, want.candidates},
		{"the typeahead", func(c *pool.Conn) (int, error) {
			got, err := cards.Suggest(t.Context(), c, "sol", 5)
			return len(got), err
		}, want.suggestion},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			// **One pool file per branch.** DuckDB keys a loaded database on
			// its path and refuses a second handle on one it already holds, so
			// two parallel sweeps over one file collide at the open rather
			// than at the budget. Each branch builds its own.
			path := pooltest.Build(t)
			refused, full := 0, 0
			for rows := 0; rows <= 40; rows++ {
				p, fault := pooltest.FaultyPoolOver(t, path)
				fault.RowsAfter(rows)
				var got int
				err := p.Use(t.Context(), func(c *pool.Conn) error {
					n, e := tc.run(c)
					got = n
					return e
				})
				fault.Heal()
				p.Close()
				if err != nil {
					refused++
					continue
				}
				if got != tc.size {
					t.Errorf("at row budget %d %s answered %d of %d with no error",
						rows, tc.what, got, tc.size)
				}
				full++
			}
			if refused == 0 || full == 0 {
				t.Errorf("%s: %d row budgets refused and %d answered in full -- "+
					"the sweep never crossed the walk", tc.what, refused, full)
			}
		})
	}
}

// The corner lookup is the one whose short answer is a **wrong name** rather
// than a short list, so it gets its own sweep.
//
// `ByPrinting` asks for two rows and answers only when exactly one came back:
// one row is the card, two mean the set code and number name two different cards
// and the tier refuses to choose. A walk that stopped after the first of two
// would therefore turn a refusal into a confident answer -- the one outcome ADR
// 34 says this tier may never produce, because it is the tier with no judgement
// in it.
func TestTheCornerLookupNeverNamesACardFromHalfAResultSet(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t) // this test's own file; see the sweep above

	refused, answered := 0, 0
	for rows := 0; rows <= 40; rows++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.RowsAfter(rows)
		var got string
		err := p.Use(t.Context(), func(c *pool.Conn) error {
			name, e := cards.ByPrinting(t.Context(), c, "MH3", "1")
			got = name
			return e
		})
		fault.Heal()
		p.Close()
		if err != nil {
			refused++
			if got != "" {
				t.Errorf("at row budget %d the refusal came back naming %q",
					rows, got)
			}
			continue
		}
		answered++
	}
	if refused == 0 || answered == 0 {
		t.Errorf("%d row budgets refused and %d answered -- the sweep never "+
			"crossed from one to the other", refused, answered)
	}
}
