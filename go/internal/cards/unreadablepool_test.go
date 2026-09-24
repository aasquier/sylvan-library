package cards_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2" // registers "duckdb"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// What the reader does when the pool opens and then cannot answer.
//
// **The wrong answer here is silence.** Every one of these three tiers has a
// shape it uses for "I looked and found nothing" -- no set codes, no printing,
// an empty shortlist -- and every one of them is one dropped `err` away from
// being what a half-written refresh produces. Somebody photographs a fanned
// spread, the pool is mid-rebuild, and the page says *none of these are cards
// I recognise*: a sentence about their collection, delivered with complete
// confidence, about a fault that has nothing to do with their cards at all.
// That is commandment 2's failure mode exactly -- a newcomer would believe it.
//
// The fault is a **schemaless pool**, for the reason `internal/api` settled
// on it: a corrupt file cannot be opened at all and comes back as the
// documented "no pool yet", so it re-drives the degraded path rather than this
// one. It takes a file DuckDB opens happily and a `SELECT` it cannot answer --
// a half-written refresh, a truncated restore, a schema older than the binary.

// schemalessPool is a real DuckDB file with none of the pool's tables in it:
// the open and the lease both succeed, and every query fails from inside.
func schemalessPool(t *testing.T) *pool.Pool {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mtg.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	// One table, so the file is a database rather than an empty stub -- and
	// deliberately not one of ours, so nothing the reader asks for resolves.
	if _, err := db.Exec(`CREATE TABLE half_a_refresh (name VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	return p
}

// onSchemalessPool runs ask against a Conn over such a file.
func onSchemalessPool(t *testing.T, ask func(ctx context.Context, c *pool.Conn)) {
	t.Helper()
	ctx := context.Background()
	if err := schemalessPool(t).Use(ctx, func(c *pool.Conn) error {
		ask(ctx, c)
		return nil
	}); err != nil {
		t.Fatalf("the fixture never got a lease, so this proves nothing: %v", err)
	}
}

// Each tier on its own, because each has its own empty answer to be mistaken
// for: no set codes, no printing, no shortlist.
func TestEveryTierFailsRatherThanReportingNothingRecognised(t *testing.T) {
	t.Parallel()
	onSchemalessPool(t, func(ctx context.Context, c *pool.Conn) {
		if codes, err := cards.SetCodes(ctx, c); err == nil {
			t.Errorf("a pool that cannot be read answered with %d set codes; "+
				"every corner would then be unreadable and nothing would say why",
				len(codes))
		}
		name, err := cards.ByPrinting(ctx, c, "LTC", "284")
		if err == nil {
			t.Errorf("the corner tier answered %q on a pool it cannot query", name)
		}
		cands, err := cards.ByTitle(ctx, c, "Sol Ring", cards.Candidates)
		if err == nil {
			t.Errorf("the title tier offered %+v on a pool it cannot query", cands)
		}
	})
}

// And through `Read`, which is the whole batch a page actually sends. One
// sighting per tier, each asked on its own, because a reading that gave up on
// the first fault would still look right if the other two were swallowed.
func TestAReadingOfAnUnreadablePoolIsARefusalNotAnEmptyList(t *testing.T) {
	t.Parallel()
	onSchemalessPool(t, func(ctx context.Context, c *pool.Conn) {
		for _, one := range []struct {
			tier     string
			sighting cards.Sighting
		}{
			{"the corner block, which asks the pool for its set codes",
				cards.Sighting{Corner: "U0284\nLTCENLIK"}},
			{"a corner already read, which is a printing lookup",
				cards.Sighting{SetCode: "LTC", CollectorNumber: "284/281"}},
			{"a title, which is the shortlist",
				cards.Sighting{Title: "Sol Ring"}},
		} {
			readings, err := cards.Read(ctx, c, []cards.Sighting{one.sighting})
			if err == nil {
				t.Errorf("%s: read back %+v from a pool that cannot answer",
					one.tier, readings)
			}
			if readings != nil {
				t.Errorf("%s: a refusal came back carrying readings anyway: %+v",
					one.tier, readings)
			}
		}
	})
}

// The bounds, which are the other half of "a capture is untrusted input".
//
// All three are a *reader* on the other side -- a crop that swept in half the
// card, a title field that came back as a paragraph, a client that sent its
// whole binder -- and all three are cheap to reach and were never driven.
func TestACornerStopsAtEightLines(t *testing.T) {
	t.Parallel()
	codes := map[string]bool{"LTC": true}
	// The bottom-left block is four or five lines on a real card. Nine lines
	// means the crop took in something else, and the set code down at the
	// bottom of it is not this card's.
	deep := strings.Repeat("\n", 8) + "LTCENLIK"
	if s := cards.FromCorner(deep, codes); s.SetCode != "" {
		t.Errorf("a ninth line was read as the set code: %+v", s)
	}
	// The blank lines themselves are skipped rather than counted as tokens,
	// so a block with gaps in it still reads.
	if s := cards.FromCorner("\n\nLTCENLIK\n\nU0284", codes); s.SetCode != "LTC" ||
		s.CollectorNumber != "0284" {
		t.Errorf("blank lines stopped the read: %+v", s)
	}
}

// A collector number the split cannot find is nothing, not the whole string.
func TestAFaceNumberThatIsNotOneIsEmpty(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"/281", "  ", "///"} {
		if got := cards.FaceNumber(text); got != "" {
			t.Errorf("FaceNumber(%q) = %q, want nothing at all", text, got)
		}
	}
}

// A title longer than a card's is cut before it is scored, so a paragraph
// cannot decide a shortlist by its tail.
func TestATitleIsCutBeforeItIsScored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := pooltest.Open(t).Use(ctx, func(c *pool.Conn) error {
		head := strings.Repeat("x", cards.MaxTitle)
		first, err := cards.ByTitle(ctx, c, head+" Sol Ring", 3)
		if err != nil {
			return err
		}
		second, err := cards.ByTitle(ctx, c, head+" Forest", 3)
		if err != nil {
			return err
		}
		if len(first) != 3 {
			t.Fatalf("the fixture returned %d candidates, so this proves nothing", len(first))
		}
		for i := range first {
			if first[i] != second[i] {
				t.Fatalf("two titles differing only past rune %d scored "+
					"differently: %+v vs %+v", cards.MaxTitle, first[i], second[i])
			}
		}

		// And a limit below one is one rather than a query with `LIMIT 0`,
		// which would answer every title with "nothing recognised".
		one, err := cards.ByTitle(ctx, c, "Sol Ring", 0)
		if err != nil {
			return err
		}
		if len(one) != 1 {
			t.Errorf("a limit of zero offered %d names, want one", len(one))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A batch longer than a spread is cut, and the reply is still one reading per
// sighting *of the batch it read* -- never a short list silently paired
// against the caller's longer one.
func TestABatchIsCutToTheSpreadItCouldBe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := pooltest.Open(t).Use(ctx, func(c *pool.Conn) error {
		batch := make([]cards.Sighting, cards.MaxSightings+5)
		for i := range batch {
			batch[i] = cards.Sighting{Title: "Sol Ring"}
		}
		readings, err := cards.Read(ctx, c, batch)
		if err != nil {
			return err
		}
		if len(readings) != cards.MaxSightings {
			t.Errorf("%d sightings came back as %d readings, want the cap of %d",
				len(batch), len(readings), cards.MaxSightings)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
