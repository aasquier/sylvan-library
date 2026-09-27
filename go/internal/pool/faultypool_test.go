package pool_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// Every read in this package against a pool that answered for a while and then
// stopped.
//
// **The fault the suite already had stops at the first statement**, and that is
// what this file is about. A schemaless DuckDB file — the fixture
// `internal/cards` and `internal/deckread` both build — fails every read where
// it starts, which is a real deploy state and the right test for it. What it
// cannot reach is the second half of a read: an iteration that fails rather
// than ends, the fourth of the four probes `Stale` makes, a price snapshot
// whose INSERT landed and whose count did not. Those branches decide whether a
// **partial** answer is handed to somebody as a whole one, and on this module
// that is never a technical detail: a card lookup short by half is a page
// telling a newcomer their decklist is wrong (commandment 2), and a token sheet
// that comes back `Read: true` with nothing in it says *this deck makes
// nothing* about a deck full of Food.
//
// So every sweep here asserts the same thing in different words: **no budget
// may produce an answer that is not the whole truth.** A read either refuses or
// returns exactly what the healthy pool returns. Both anti-vacuity floors are
// checked every time — at least one budget refused, and at least one
// succeeded — because a fixture that refuses everything passes the first half
// on its own, and a fixture that injects nothing passes the second.

// aFaultyPool is the tiny pool behind a handle that counts statements, plus the
// Conn the reads take. The handle starts healthy.
func aFaultyPool(t *testing.T) (*pool.Conn, *sql.DB, *pooltest.Fault) {
	t.Helper()
	db, fault := pooltest.OpenFaulty(t)
	return pool.ConnOver(db), db, fault
}

// faultyRead is one read of the pool, rendered as a string so a sweep can hold
// a budget's answer equal to the healthy one without caring how the answer is
// shaped.
type faultyRead struct {
	name string
	read func(context.Context, *pool.Conn) (string, error)
}

// The reads. Each names the cards it asks for out of the recorded fixture
// (`pooltest/testdata/tiny_pool.json`), and each renders deterministically —
// a map walk would make the comparison below a coin toss.
func faultyReads() []faultyRead {
	return []faultyRead{{
		name: "the columns oracle_cards has",
		read: func(ctx context.Context, c *pool.Conn) (string, error) {
			cols, err := c.Columns(ctx, "oracle_cards")
			if err != nil {
				return "", err
			}
			names := make([]string, 0, len(cols))
			for name := range cols {
				names = append(names, name)
			}
			sort.Strings(names)
			return strings.Join(names, ","), nil
		},
	}, {
		name: "a batched card lookup",
		read: func(ctx context.Context, c *pool.Conn) (string, error) {
			found, err := c.GetCards(ctx, []string{"Sol Ring", "Black Lotus"})
			if err != nil {
				return "", err
			}
			parts := make([]string, 0, len(found))
			for name, rec := range found {
				parts = append(parts, name+"="+rec.TypeLine)
			}
			sort.Strings(parts)
			return strings.Join(parts, ";"), nil
		},
	}, {
		name: "an ad-hoc card search",
		read: func(ctx context.Context, c *pool.Conn) (string, error) {
			recs, err := c.Search(ctx, "cmc >= ?", []any{6.0}, 10, "name", 0)
			if err != nil {
				return "", err
			}
			names := make([]string, 0, len(recs))
			for _, rec := range recs {
				names = append(names, rec.Name)
			}
			return strings.Join(names, ";"), nil
		},
	}, {
		name: "the painting a card was first given",
		read: func(ctx context.Context, c *pool.Conn) (string, error) {
			art, err := c.ArtFor(ctx, []string{"Sol Ring", "Primeval Titan"})
			if err != nil {
				return "", err
			}
			parts := make([]string, 0, len(art))
			for key, one := range art {
				parts = append(parts, key+"="+one.Artist+"/"+one.Printing)
			}
			sort.Strings(parts)
			return strings.Join(parts, ";"), nil
		},
	}, {
		name: "the tokens a deck makes",
		read: func(ctx context.Context, c *pool.Conn) (string, error) {
			sheet, err := c.TokensMade(ctx, []string{"Terastodon", "Gyome, Master Chef"})
			if err != nil {
				return "", err
			}
			parts := []string{fmt.Sprintf("read=%v", sheet.Read)}
			for _, made := range sheet.Tokens {
				parts = append(parts, made.Name+"<"+strings.Join(made.MadeBy, "+"))
			}
			return strings.Join(parts, ";"), nil
		},
	}}
}

// An iteration that failed is never an answer. Every read, at every row budget
// from nothing to well past what the deepest of them spends: either a refusal,
// or the whole truth.
//
// **The row budget is spent across every query on the handle**, which is why
// this is a sweep rather than a number. `GetCards` asks what columns the pool
// has before it asks for a card, and that first result set is one row per
// column — so the budget at which its *own* iteration breaks is a number about
// today's schema, and hard-coding it would be a test about the column count.
func TestNoReadOfTheCardPoolAnswersFromAnIterationThatFailed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Past the deepest read: the columns of `oracle_cards` are one result set on
	// their own, and the token sheet walks four more after them.
	const budgets = 48
	for _, r := range faultyReads() {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			c, _, fault := aFaultyPool(t)

			truth, err := r.read(ctx, c)
			if err != nil {
				t.Fatalf("the healthy pool could not answer at all: %v", err)
			}

			refused, answered, firstAnswer := 0, 0, -1
			for budget := range budgets {
				fault.RowsAfter(budget)
				got, err := r.read(ctx, c)
				if err != nil {
					if firstAnswer >= 0 {
						t.Fatalf("row budget %d refused after %d had already "+
							"answered; the budget is not monotone and the sweep "+
							"below proves nothing", budget, firstAnswer)
					}
					refused++
					continue
				}
				if got != truth {
					t.Fatalf("row budget %d answered %q where the whole pool "+
						"answers %q -- a read cut short was handed over as a "+
						"complete one", budget, got, truth)
				}
				if firstAnswer < 0 {
					firstAnswer = budget
				}
				answered++
				// The budget is monotone — asserted above — so once two
				// budgets in a row have answered in full there is nothing
				// left to learn, and the ceiling above becomes a bound on
				// the search rather than a bill the suite pays. The shallow
				// reads here stop after four runs; only the token sheet
				// walks the whole range.
				if answered >= 2 {
					break
				}
			}
			fault.Heal()

			if refused == 0 {
				t.Error("no row budget refused: the fault was never injected, " +
					"so every assertion above was about a healthy pool")
			}
			if answered == 0 {
				t.Errorf("no row budget up to %d answered: the fixture refuses "+
					"everything, and a sweep that only ever sees refusals "+
					"cannot tell a guard from a broken handle", budgets)
			}
		})
	}
}

// The same sweep on the statement budget rather than the row budget. It is the
// other half of the fault and it reaches different branches: a query that never
// ran at all, rather than one whose result set died halfway.
func TestNoReadOfTheCardPoolAnswersFromAQueryThatNeverRan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// The deepest read here (the token sheet) is five statements.
	const budgets = 8
	for _, r := range faultyReads() {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			c, _, fault := aFaultyPool(t)

			truth, err := r.read(ctx, c)
			if err != nil {
				t.Fatalf("the healthy pool could not answer at all: %v", err)
			}

			refused, answered := 0, 0
			for budget := range budgets {
				fault.After(budget)
				got, err := r.read(ctx, c)
				if err != nil {
					if !errors.Is(err, pooltest.ErrPoolGoneAway) {
						t.Fatalf("statement budget %d failed with %v, which is "+
							"not the fault this test injected", budget, err)
					}
					refused++
					continue
				}
				if got != truth {
					t.Fatalf("statement budget %d answered %q where the whole "+
						"pool answers %q", budget, got, truth)
				}
				answered++
			}
			fault.Heal()

			if refused == 0 {
				t.Error("no statement budget refused: the fault was never injected")
			}
			if answered == 0 {
				t.Errorf("no statement budget up to %d answered", budgets)
			}
		})
	}
}

// The staleness walk, statement by statement.
//
// **It is six statements and every one of them can fail**, which is the reason
// this one names a number instead of sweeping for it. Read off `stale.go`'s
// `probeStaleness`, in order: the probe for any card at all, the columns of
// `oracle_cards`, the probe for a printed power, the probe for any printing at
// all, the columns of `printings`, the probe for a named painter. A walk that
// cannot be finished must come back as a failure and **never** as a verdict,
// because "not stale" from a pool nobody could read is the same false
// reassurance one rung up — and `/api/health` is the most-asked route on the
// instance, so the reassurance would be printed over and over.
func TestTheStalenessWalkRefusesAtEveryOneOfItsSixStatements(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _, fault := aFaultyPool(t)

	// The recorded fixture carries `power` and a named painter, so a pool that
	// can be walked is not stale. Taken rather than assumed: the assertion below
	// is that a fault never changes this answer, not that it happens to be false.
	fault.Heal()
	whole, err := pool.Stale(ctx, c)
	if err != nil {
		t.Fatalf("the healthy fixture could not be walked: %v", err)
	}

	const statements = 6
	for budget := range statements + 2 {
		fault.After(budget)
		verdict, err := pool.Stale(ctx, c)
		switch {
		case budget < statements:
			if err == nil {
				t.Errorf("budget %d walked the pool to verdict %v; only %d of "+
					"the walk's statements had run", budget, verdict, budget)
			}
			if verdict {
				t.Errorf("budget %d came back refusing AND carrying a verdict", budget)
			}
		default:
			if err != nil {
				t.Errorf("budget %d refused a walk whose every statement was "+
					"allowed: %v", budget, err)
			}
			if verdict != whole {
				t.Errorf("budget %d answered stale=%v where the whole pool "+
					"answers %v", budget, verdict, whole)
			}
		}
	}
	fault.Heal()
}

// The token sheet's own probe, which is the statement between the column check
// and the first real query.
//
// **`Read: false` is the whole assertion.** `TokenSheet.Read` is what the deck
// page reads to tell "nobody has looked yet" from "this deck makes nothing",
// and a refusal that came back `Read: true` with an empty list would be the
// second sentence about a deck for which the first is true.
func TestATokenSheetRefusedByItsProbeSaysNobodyLooked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, _, fault := aFaultyPool(t)

	// One statement: the columns of `oracle_cards` answer, and the probe that
	// asks whether `all_parts` was ever filled does not.
	fault.After(1)
	sheet, err := c.TokensMade(ctx, []string{"Terastodon"})
	fault.Heal()
	if err == nil {
		t.Fatalf("a probe that failed produced a sheet: %+v", sheet)
	}
	if sheet.Read {
		t.Error("a refused token sheet came back Read: true -- the deck page " +
			"would print `this deck makes nothing` about a fault")
	}
	if len(sheet.Tokens) != 0 {
		t.Errorf("a refused token sheet carried %d tokens", len(sheet.Tokens))
	}
}

// The token sheet all the way down, including the painting of a token this pool
// actually has a printing of.
//
// The recorded fixture names Terastodon's Elephant in `all_parts` and carries
// no printing of it, which is a real and deliberate shape — Scryfall prints
// tokens this library filters out — but it means `tokenArtByOracle` is never
// reached from the recorded rows alone. One invented printing, doctored into
// this test's own temp copy, is what puts it on the path. **Invented, and it
// reads as invented**: the id is the one Terastodon's card really names, so the
// join is the real join, and every other value is a fixture's rather than a
// claim about a painting nobody looked up.
func TestTheTokenPaintingLookupRefusesAnIterationThatFailed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c, db, fault := aFaultyPool(t)

	const elephant = "2853ba8b-650e-49e3-ab12-86b76e02743b"
	if _, err := db.ExecContext(ctx, `INSERT INTO printings
	    (id, oracle_id, name, set_code, set_name, collector_number, rarity,
	     released_at, digital, promo, finishes, image_normal, artist)
	    VALUES (?, 'fixture-elephant-oracle', 'Elephant', 'fix', 'Fixture Set',
	            'T1', 'common', '2019-10-04'::DATE, false, false,
	            ['nonfoil']::VARCHAR[], 'https://example.invalid/elephant.jpg',
	            'Fixture Painter')`, elephant); err != nil {
		t.Fatalf("doctoring a token printing in: %v", err)
	}

	fault.Heal()
	whole, err := c.TokensMade(ctx, []string{"Terastodon"})
	if err != nil {
		t.Fatal(err)
	}
	if len(whole.Tokens) != 1 || whole.Tokens[0].Art == nil {
		t.Fatalf("the doctored printing did not reach the sheet: %+v", whole)
	}
	if got := whole.Tokens[0].Art.Artist; got != "Fixture Painter" {
		t.Fatalf("the sheet credited %q rather than the doctored painter", got)
	}

	// Now every row budget. The sheet either refuses or credits the same
	// painting; what it must never do is draw a plate with no painter on it
	// because the iteration that would have named one died (commandment 19 --
	// a picture and its credit come out of the same row or not at all).
	refused, answered := 0, 0
	for budget := range 48 {
		fault.RowsAfter(budget)
		sheet, err := c.TokensMade(ctx, []string{"Terastodon"})
		if err != nil {
			refused++
			if sheet.Read {
				t.Fatalf("row budget %d refused and still said it had looked", budget)
			}
			continue
		}
		answered++
		if len(sheet.Tokens) != len(whole.Tokens) {
			t.Fatalf("row budget %d answered with %d tokens where the whole "+
				"pool answers %d", budget, len(sheet.Tokens), len(whole.Tokens))
		}
		if sheet.Tokens[0].Art == nil {
			t.Fatalf("row budget %d drew the Elephant with no painter credited "+
				"-- the pool's answer was cut short and passed on anyway", budget)
		}
		// Two full answers is proof enough; see the sweep above.
		if answered >= 2 {
			break
		}
	}
	fault.Heal()
	if refused == 0 || answered == 0 {
		t.Errorf("the sweep saw %d refusals and %d answers; it needs both to "+
			"mean anything", refused, answered)
	}
}

// A price snapshot tells a pool with no prices from a pool that stopped
// answering, and reports nothing it did not verify.
//
// **`ErrNoPool` is the reserved sentence and it has to stay reserved.**
// `SnapshotPrices` is the one command a cron runs unattended, and it folds its
// first count's failure into "there is no pool" on purpose (the function says
// why). Every fault *after* that count is a different event and must not wear
// the same word, or a volume that detached mid-write is reported as an empty
// library and nobody goes looking.
func TestAPriceSnapshotTellsAnEmptyPoolFromOneThatStopped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, fault := pooltest.OpenFaulty(t)

	fault.Heal()
	written, err := pool.SnapshotPrices(ctx, db)
	if err != nil {
		t.Fatalf("the healthy fixture could not be snapshotted: %v", err)
	}
	if written == 0 {
		t.Fatal("the recorded fixture snapshotted no prices at all; the " +
			"assertions below would not be about a working write")
	}

	// Budget 0: the count of printings never runs, which is the one fault that
	// is reported as an absent pool.
	fault.After(0)
	if _, err := pool.SnapshotPrices(ctx, db); !errors.Is(err, pool.ErrNoPool) {
		t.Errorf("a count that could not run answered %v, not the pool's own "+
			"absent-library sentence", err)
	}

	// Budget 1: the count answers and the INSERT does not.
	fault.After(1)
	got, err := pool.SnapshotPrices(ctx, db)
	if err == nil {
		t.Errorf("a snapshot whose write failed reported %d prices", got)
	}
	if errors.Is(err, pool.ErrNoPool) {
		t.Error("a write that failed was reported as an empty pool; a volume " +
			"that went away mid-snapshot would read as a library with no prices")
	}
	if got != 0 {
		t.Errorf("a failed snapshot reported %d prices written", got)
	}

	// Budget 2: the write lands and the count of what landed does not. Nothing
	// may be reported as recorded on the strength of a count nobody read.
	fault.After(2)
	got, err = pool.SnapshotPrices(ctx, db)
	fault.Heal()
	if err == nil {
		t.Errorf("a snapshot that could not count its own rows reported %d", got)
	}
	if got != 0 {
		t.Errorf("a snapshot that could not count its own rows reported %d "+
			"prices written", got)
	}
}

// A load that cannot even open its transaction says so, and loads nothing.
//
// The bulk file is never opened: the BEGIN is the first statement, and a load
// that could not start one must not go on to empty the table it was going to
// refill.
func TestALoadThatCannotOpenATransactionRefusesBeforeReadingAnything(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, fault := pooltest.OpenFaulty(t)

	before := countOracle(t, db)

	fault.After(0)
	total, err := pool.LoadOracle(ctx, db, "no-such-bulk-file.jsonl")
	fault.Heal()
	if !errors.Is(err, pooltest.ErrPoolGoneAway) {
		t.Fatalf("a load whose BEGIN was refused answered %v", err)
	}
	if total != 0 {
		t.Errorf("a load that never began reported %d rows", total)
	}
	if after := countOracle(t, db); after != before {
		t.Errorf("oracle_cards went from %d rows to %d; a load that could not "+
			"open a transaction emptied the table anyway", before, after)
	}
}

func countOracle(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM oracle_cards").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
