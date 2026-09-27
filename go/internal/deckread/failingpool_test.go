package deckread

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2" // registers "duckdb"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// What every deck read does when the card pool answers an **error** rather
// than answering nothing.
//
// The distinction is the whole file and it is easy to lose. A machine with no
// pool is a fresh checkout before `mtglab data refresh`, and every read here
// degrades on purpose -- a deck still renders as names and rationales, and
// `pool_available` says which happened (`TestEveryDeckReadDegradesWithoutAPool`
// holds that half). A pool that opens and then **fails a query** is something
// else: a half-written refresh, a truncated restore, a schema older than the
// binary reading it. Those must not be folded into the degraded answer,
// because a deck page that quietly renders "we have never heard of any of
// these cards" over a broken pool is telling somebody their decklist is
// wrong.
//
// Two files stand in for the two shapes that fault really takes on a volume.
// A pool with **no tables at all** is the schema-less one `internal/api` also
// uses, and every read fails at its first query. A pool with **oracle cards
// and no printings** is the more interesting one, because it is the shape a
// refresh interrupted between its two loads leaves behind: the card lookups
// all succeed, and only the art, the prices and the printing history fail --
// which is exactly the set of reads nothing had ever driven against a fault.

// schemalessPool is a real DuckDB file with none of the pool's tables in it:
// the file opens happily and every query fails with a catalog error, so the
// failure arrives from inside `Use` rather than from `acquire`.
func schemalessPool(t *testing.T) *pool.Pool {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mtg.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	// One table, so the file is a database rather than an empty stub -- and
	// deliberately not one of ours, so nothing resolves.
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

// bentPool is the fixture with the given statements run over it: a
// real pool that a refresh, a restore or an older binary left in some
// particular state.
func bentPool(t *testing.T, statements ...string) *pool.Pool {
	t.Helper()
	path := pooltest.Build(t)
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	return p
}

// withBrokenPool leases one of the two faulty pools and hands the connection
// over. The lease itself must succeed -- that is the premise of every test
// below -- so a failure here is reported as one.
func withBrokenPool(t *testing.T, p *pool.Pool, fn func(c *pool.Conn)) {
	t.Helper()
	if err := p.Use(context.Background(), func(c *pool.Conn) error {
		fn(c)
		return nil
	}); err != nil {
		t.Fatalf("leasing the pool: %v", err)
	}
}

// aDeckThatAsksForEverything is one deck touching every read below: a
// commander that is really in the fixture, a chosen printing for it and for
// one of the 99, so the art paths are driven rather than skipped.
func aDeckThatAsksForEverything() *deck.Deck {
	return &deck.Deck{
		Slug: "broken-pool", Name: "A Deck Over A Broken Pool",
		Status: "theoretical", Stage: "draft",
		Commander:    []string{"Gyome, Master Chef"},
		CommanderArt: solRingPrintingA,
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Why: "ramp", Art: solRingPrintingB},
			{Name: "Forest", Why: "a land"},
		},
	}
}

// The premise: the fault really is a query error rather than the degraded
// answer. If `pool.Pool.Use` ever folds one into the other, every test in
// this file would keep passing while re-testing the degraded path a second
// time -- so the distinction is asserted rather than assumed.
func TestAPoolWithNoTablesFailsTheQueryRatherThanTheOpen(t *testing.T) {
	t.Parallel()
	err := schemalessPool(t).Use(context.Background(), func(c *pool.Conn) error {
		_, e := c.GetCards(context.Background(), []string{"Sol Ring"})
		return e
	})
	if err == nil {
		t.Fatal("a pool with no tables answered a card lookup")
	}
	if err == pool.ErrNoPool { //nolint:errorlint // the sentinel identity is the point
		t.Fatal("a pool that opens and fails its query was reported as an absent " +
			"pool -- this whole file would then be re-testing the degraded path")
	}
}

// Every read this package offers, against a pool that will not answer.
func TestNoDeckReadInventsAnAnswerOverAPoolThatWillNotAnswer(t *testing.T) {
	t.Parallel()
	d := aDeckThatAsksForEverything()
	ctx := context.Background()

	withBrokenPool(t, schemalessPool(t), func(c *pool.Conn) {
		for _, tc := range []struct {
			what string
			run  func() (any, error)
		}{
			{"PoolFor", func() (any, error) { return PoolFor(ctx, c, d) }},
			{"ChosenArts", func() (any, error) { return ChosenArts(ctx, c, []string{solRingPrintingA}) }},
			{"CardArtOverrides", func() (any, error) { return CardArtOverrides(ctx, c, d) }},
			{"Tiles", func() (any, error) { return Tiles(ctx, c, []*deck.Deck{d}, true, "alice") }},
			{"DeckPayload", func() (any, error) { return DeckPayload(ctx, c, d, true, "alice") }},
			{"Suggestions", func() (any, error) { return Suggestions(ctx, c, d, 5) }},
			{"Validate", func() (any, error) { return Validate(ctx, c, d) }},
			{"Stats", func() (any, error) { return Stats(ctx, c, d) }},
			{"SearchCards", func() (any, error) {
				return SearchCards(ctx, c, SearchQuery{Text: "sol"})
			}},
			{"CardsNamed", func() (any, error) { return CardsNamed(ctx, c, []string{"Sol Ring"}) }},
			{"CommanderDossier", func() (any, error) { return CommanderDossier(ctx, c, d) }},
		} {
			got, err := tc.run()
			if err == nil {
				t.Errorf("%s answered %v over a pool that cannot answer -- that "+
					"reads as a pool which has never heard of these cards", tc.what, got)
			}
		}
	})
}

// The commander panel over a pool whose printing history is gone.
//
// The card lookups succeed here, so the panel gets as far as the subtypes and
// the other cards carrying the character's name -- and then asks how often
// the commander has been printed, which is the read that fails. A panel that
// swallowed that would show "printed once, in no set", which is a claim about
// Magic rather than an absence.
func TestTheCommanderPanelRefusesRatherThanInventingAPrintingHistory(t *testing.T) {
	t.Parallel()
	d := aDeckThatAsksForEverything()
	ctx := context.Background()

	withBrokenPool(t, bentPool(t, "DROP TABLE printings"), func(c *pool.Conn) {
		// The premise: the card itself still reads, so the failure below is
		// the printing history's and not the lookup's.
		found, err := c.GetCards(ctx, []string{"Gyome, Master Chef"})
		if err != nil || found["Gyome, Master Chef"] == nil {
			t.Fatalf("the commander did not read back: %v", err)
		}
		if dossier, err := CommanderDossier(ctx, c, d); err == nil {
			t.Errorf("the panel answered %v over a pool with no printing history", dossier)
		}
	})
}

// The reads that go looking for art over a pool whose printings are gone.
//
// Each of these resolves its cards and then asks for a printing, which is the
// second half of a refresh and the half that can be missing on its own. What
// no page may do is render the default art while the deck file plainly says
// otherwise -- that reads as the site ignoring what somebody typed.
func TestTheArtAndPriceReadsRefuseAPoolWhosePrintingsAreGone(t *testing.T) {
	t.Parallel()
	d := aDeckThatAsksForEverything()
	ctx := context.Background()

	withBrokenPool(t, bentPool(t, "DROP TABLE printings"), func(c *pool.Conn) {
		for _, tc := range []struct {
			what string
			run  func() (any, error)
		}{
			{"the deck page", func() (any, error) { return DeckPayload(ctx, c, d, true, "alice") }},
			{"the shelf", func() (any, error) { return Tiles(ctx, c, []*deck.Deck{d}, true, "alice") }},
			{"a card's chosen printing", func() (any, error) { return CardArtOverrides(ctx, c, d) }},
			// The search asks the printings for the cheapest paper price
			// after the result is cut to sixty; a price of nothing would
			// read as "nobody sells this".
			{"the card search", func() (any, error) {
				return SearchCards(ctx, c, SearchQuery{Text: "sol"})
			}},
		} {
			if got, err := tc.run(); err == nil {
				t.Errorf("%s answered %v over a pool with no printings", tc.what, got)
			}
		}
	})
}

// A deck page whose cards chose printings, and whose commander did not,
// fails on the cards' own read -- the second of the two art paths, and the
// one a deck that never picked a hero image still takes.
func TestACardsChosenPrintingRefusesOnItsOwn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := &deck.Deck{
		Slug: "cards", Name: "Cards", Status: "theoretical", Stage: "draft",
		Commander: []string{"Gyome, Master Chef"},
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Why: "ramp", Art: solRingPrintingB},
			{Name: "Forest", Why: "a land"},
		},
	}
	withBrokenPool(t, bentPool(t, "DROP TABLE printings"), func(c *pool.Conn) {
		if got, err := DeckPayload(ctx, c, d, true, "alice"); err == nil {
			t.Errorf("the deck page answered %v over a printing it could not read", got)
		}
	})
}

// The shelf's picture falls back rather than going blank.
//
// A tile's art comes from the commander's crop, and a card in the pool may
// have a picture without one -- an older refresh, a printing Scryfall has not
// cropped. The whole card is the fallback, and it is the difference between a
// shelf and a row of grey rectangles. The same holds one step further along:
// a *chosen* printing whose picture yields no crop is still a picture.
func TestTheShelfsPictureFallsBackRatherThanGoingBlank(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// The commander's crop removed, and one printing given a picture whose
	// address no crop can be derived from.
	p := bentPool(t,
		"UPDATE oracle_cards SET image_art_crop = NULL WHERE name = 'Gyome, Master Chef'",
		"UPDATE printings SET image_normal = 'https://example.invalid/a-picture.jpg'"+
			" WHERE id = '"+solRingPrintingA+"'")

	plain := &deck.Deck{Slug: "plain", Name: "Plain", Status: "theoretical", Stage: "draft",
		Commander: []string{"Gyome, Master Chef"},
		Cards:     []deck.CardEntry{{Name: "Forest", Why: "a land"}}}
	chosen := &deck.Deck{Slug: "chosen", Name: "Chosen", Status: "theoretical", Stage: "draft",
		Commander: []string{"Gyome, Master Chef"}, CommanderArt: solRingPrintingA,
		Cards: []deck.CardEntry{{Name: "Forest", Why: "a land"}}}

	withBrokenPool(t, p, func(c *pool.Conn) {
		tiles, err := Tiles(ctx, c, []*deck.Deck{plain, chosen}, true, "alice")
		if err != nil {
			t.Fatalf("the shelf: %v", err)
		}
		if len(tiles) != 2 {
			t.Fatalf("two decks made %d tiles", len(tiles))
		}
		if tiles[0].ArtCrop == nil {
			t.Error("a commander with a picture and no crop left its tile blank")
		}
		if tiles[1].ArtCrop == nil {
			t.Error("a chosen printing with a picture and no crop left its tile blank")
		} else if *tiles[1].ArtCrop != "https://example.invalid/a-picture.jpg" {
			t.Errorf("the chosen printing's tile shows %q", *tiles[1].ArtCrop)
		}
	})
}

// A deck page whose commander alone chose a printing fails on that alone --
// the card-level override is not what refuses here, so the commander's own
// path is the one being driven.
func TestACommandersChosenPrintingRefusesOnItsOwn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := &deck.Deck{
		Slug: "hero", Name: "Hero", Status: "theoretical", Stage: "draft",
		Commander: []string{"Gyome, Master Chef"}, CommanderArt: solRingPrintingA,
		Cards: []deck.CardEntry{{Name: "Forest", Why: "a land"}},
	}
	withBrokenPool(t, bentPool(t, "DROP TABLE printings"), func(c *pool.Conn) {
		if got, err := DeckPayload(ctx, c, d, true, "alice"); err == nil {
			t.Errorf("the deck page answered %v over a printing it could not read", got)
		}
	})
}

// A pool written by an older refresh, whose printings carry no painter and no
// flavour text.
//
// This is not a fault: the columns arrived later, and a pool refreshed before
// they did is a pool the app must still serve. What it must not do is fail --
// so the query names them as nulls rather than selecting columns that are not
// there, and the page renders the picture with no painter beside it.
//
// **The picture is the one thing that must be real.** A printing row with no
// image is skipped entirely rather than handed over as a chosen printing with
// nothing to show, because a card page whose hero is a blank rectangle is
// worse than one that fell back to the default art.
func TestAPoolFromAnOlderRefreshStillAnswersForItsArt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// The table is rebuilt without the two later columns rather than altered,
	// because the index the schema puts on it holds the original.
	p := bentPool(t,
		"CREATE TABLE older AS SELECT id, oracle_id, name, set_code, set_name,"+
			" collector_number, rarity, released_at, digital, promo, finishes,"+
			" image_normal, price_usd, price_usd_foil, price_eur, tcg_product_id"+
			" FROM printings",
		"DROP TABLE printings",
		"ALTER TABLE older RENAME TO printings",
		"UPDATE printings SET image_normal = NULL WHERE id = '"+solRingPrintingB+"'")

	withBrokenPool(t, p, func(c *pool.Conn) {
		arts, err := ChosenArts(ctx, c, []string{solRingPrintingA, solRingPrintingB})
		if err != nil {
			t.Fatalf("a pool with no painter column refused the read: %v", err)
		}
		chosen, ok := arts[solRingPrintingA]
		if !ok {
			t.Fatalf("the printing did not come back: %v", arts)
		}
		if chosen.Image == nil {
			t.Error("a printing with a picture came back without one")
		}
		if chosen.Artist != nil || chosen.FlavorText != nil {
			t.Errorf("the painter and the flavour were invented: %v / %v",
				chosen.Artist, chosen.FlavorText)
		}
		// And the pictureless printing is simply absent.
		if _, ok := arts[solRingPrintingB]; ok {
			t.Error("a printing with no picture was handed over as a chosen one")
		}
	})
}
