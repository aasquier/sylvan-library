package deckread

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The card pool when it stops answering **partway through** a read.
//
// `failingpool_test.go` is the other half and it argues the distinction: a pool
// with no tables fails every read at its *first* query, and a pool whose
// printings were dropped fails every read that asks about a printing. Neither
// can reach the arm after a query that worked — and the commander panel is a
// dozen queries long. Its subtype counts, its related-cards walk, its printing
// history and its chosen printing each sit behind a lookup that succeeded, so a
// volume that detaches between two of them was a shape nothing in this package
// had ever driven.
//
// `pooltest.OpenFaultyPool` is that shape: the fixture pool behind a connector
// with a counter saying how many more statements, or how many more rows, the
// library will answer. It is new, and it is new because a Pool used to open its
// own file — `pool.Connect` carries that argument where the seam is.
//
// **Budgets are swept rather than counted.** A counted budget restates today's
// query order and goes quietly meaningless the day a query is added to the
// middle of the panel. The sweep asks the question the panel actually has to
// answer: *at no point may a half-read dossier come back without an error.*

// The fixture's Ajani printing, read out of `tiny_pool.json` rather than
// remembered: Modern Horizons 3, painted by Chris Rallis.
const ajaniPrinting = "0d16e8e0-31b2-4389-afd6-783c501f6fa0"

// theAjaniDeck is a deck whose commander shares its character name with another
// card in the fixture, which is what makes the related-cards walk run at all.
//
// Ajani, Nacatl Pariah's combined name splits at its first comma to "Ajani", and
// "Ajani Steadfast Emblem" is the other card the fixture holds under that name —
// so the panel's `other_cards` list is one row long rather than empty, and the
// loop that reads it is entered. Every fixture before this one had a commander
// nothing else was named after, which is why nobody had been inside that loop.
//
// The chosen printing is the commander's own, so the panel's last two queries —
// the ones that follow the deck's chosen art rather than the card's default —
// run too.
func theAjaniDeck() *deck.Deck {
	return &deck.Deck{
		Slug: "ajani", Name: "A Deck Led By Ajani",
		Status: "theoretical", Stage: "draft",
		Commander:    []string{"Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"},
		CommanderArt: ajaniPrinting,
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Why: "ramp"},
			{Name: "Forest", Why: "a land"},
		},
	}
}

// The panel over a healthy pool, first — so "short" has a number below and the
// related-cards walk is proved to run before anything is broken.
func TestTheCommanderPanelWalksTheCardsSharingTheCharactersName(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	d := theAjaniDeck()
	var dossier wire.OrderedMap
	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		out, err := CommanderDossier(t.Context(), c, d)
		dossier = out
		return err
	}); err != nil {
		t.Fatalf("the healthy panel: %v", err)
	}
	// Counts rather than two separate asserts, so a fixture that stopped holding
	// a second Ajani is a failure here rather than a silently empty loop
	// somewhere below.
	if n := relatedCount(dossier); n < 1 {
		t.Fatalf("the panel listed %d cards sharing the commander's character "+
			"name -- the fixture must hold a second one or the walk below is "+
			"never entered", n)
	}
	if n := listLen(dossier, "subtypes"); n < 1 {
		t.Fatalf("the panel counted %d subtypes -- the per-subtype queries "+
			"below are then never made", n)
	}
	// The chosen printing's painter, which is the one the deck asked for rather
	// than the card's default: proof the last two queries ran.
	artist, _ := field(cardOf(dossier), "artist").(*string)
	if artist == nil || *artist == "" {
		t.Error("the panel credits no painter for a deck that chose a printing")
	}
}

// A sweep over every statement the panel makes: the dossier either comes back
// whole or comes back as an error, and never as a panel with a hole in it.
//
// Each budget gets its own Pool over the same file, because a Pool remembers
// (`pooltest.FaultyPoolOver` argues it).
func TestTheCommanderPanelRefusesAtEveryStatementAPoolCanStopAnswering(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	d := theAjaniDeck()

	// Above the longest the panel is: a columns read, a card lookup, two
	// queries per subtype, the related-cards walk, the oracle id, the printing
	// count, the first set's name, and the chosen printing's own two.
	const widest = 16
	refused, whole := 0, 0
	for budget := 0; budget <= widest; budget++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		var dossier wire.OrderedMap
		err := p.Use(t.Context(), func(c *pool.Conn) error {
			out, e := CommanderDossier(t.Context(), c, d)
			dossier = out
			return e
		})
		fault.Heal()
		p.Close()
		if err != nil {
			refused++
			continue
		}
		if !panelIsWhole(dossier) {
			t.Errorf("at budget %d the panel answered with a hole in it and no "+
				"error: %#v", budget, dossier)
		}
		whole++
	}
	// Both halves have to happen or the sweep measured one state seventeen times.
	if refused == 0 || whole == 0 {
		t.Errorf("%d budgets refused and %d answered in full -- the sweep never "+
			"crossed from one to the other", refused, whole)
	}
}

// The related-cards walk cut short partway down its result set, which is the
// one fault that hands back a **shorter list** rather than an error.
//
// The panel used to ask the scan whether it had failed — which, over
// destinations `database/sql` has no value to refuse, it never had — and never
// asked the walk. So a pool that answered the commander and then went away
// between two related cards produced a strip presented as the whole of what
// shares the character's name. `commander.go` argues the fix where it is.
func TestTheCommanderPanelRefusesAWalkCutShortRatherThanAShortStrip(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	d := theAjaniDeck()

	// The healthy count first, so "short" has a number.
	want := -1
	p := pooltest.Open(t)
	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		dossier, err := CommanderDossier(t.Context(), c, d)
		if err != nil {
			return err
		}
		want = relatedCount(dossier)
		return nil
	}); err != nil {
		t.Fatalf("the healthy panel: %v", err)
	}
	if want < 1 {
		t.Fatalf("the fixture's Ajani has %d related cards -- this test needs one", want)
	}

	// The budget is spent across every query on the handle, and the panel's
	// first move reads one row per column of `oracle_cards` — so the walk this
	// test is about sits some way in. Swept rather than computed: the sweep
	// crosses it wherever it is.
	refused, full := 0, 0
	for rows := 0; rows <= 70; rows++ {
		fp, fault := pooltest.FaultyPoolOver(t, path)
		fault.RowsAfter(rows)
		var dossier wire.OrderedMap
		err := fp.Use(t.Context(), func(c *pool.Conn) error {
			out, e := CommanderDossier(t.Context(), c, d)
			dossier = out
			return e
		})
		fault.Heal()
		fp.Close()
		if err != nil {
			refused++
			continue
		}
		if got := relatedCount(dossier); got != want {
			t.Errorf("at row budget %d the panel listed %d of %d cards sharing "+
				"the character's name, with no error", rows, got, want)
		}
		full++
	}
	if refused == 0 || full == 0 {
		t.Errorf("%d row budgets refused and %d answered in full -- the sweep "+
			"never crossed the walk", refused, full)
	}
}

// The two reads that ask the pool again once something else has answered: the
// shortlist, which goes looking for a replacement after the gate names a card,
// and the search's commander filter, which asks what the rows actually are
// after the rows are in. Both arms sit behind a query that worked.
func TestTheReadsThatAskThePoolASecondTimeRefuseWhenItHasGoneAway(t *testing.T) {
	t.Parallel()
	// Gyome's identity is {B}{G} and Swords to Plowshares is white, so the gate
	// names it and the shortlist goes looking for something legal instead.
	outside := &deck.Deck{
		Slug: "outside", Name: "A Deck Reaching Outside Its Identity",
		Status: "theoretical", Stage: "draft",
		Commander: []string{"Gyome, Master Chef"},
		Cards: []deck.CardEntry{
			{Name: "Swords to Plowshares", Why: "removal, and the wrong colour"},
			{Name: "Forest", Why: "a land"},
		},
	}

	for _, tc := range []struct {
		what string
		run  func(c *pool.Conn) (any, error)
	}{
		{"the shortlist", func(c *pool.Conn) (any, error) {
			return Suggestions(t.Context(), c, outside, 5)
		}},
		{"the commander search", func(c *pool.Conn) (any, error) {
			return SearchCards(t.Context(), c,
				SearchQuery{Text: "gyome", CommandersOnly: true})
		}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			// **One pool file per branch.** DuckDB keys a loaded database on
			// its path and refuses a second handle on one it already holds, so
			// two parallel sweeps over one file collide at the open rather than
			// at the budget.
			path := pooltest.Build(t)
			refused, answered := 0, 0
			for budget := 0; budget <= 10; budget++ {
				p, fault := pooltest.FaultyPoolOver(t, path)
				fault.After(budget)
				err := p.Use(t.Context(), func(c *pool.Conn) error {
					_, e := tc.run(c)
					return e
				})
				fault.Heal()
				p.Close()
				if err != nil {
					refused++
					continue
				}
				answered++
			}
			if refused == 0 || answered == 0 {
				t.Errorf("%s: %d budgets refused and %d answered -- the sweep "+
					"never crossed from one to the other", tc.what, refused, answered)
			}
		})
	}
}

// A search cut short partway down its own result set: the rows that came back
// are not the rows there were, and a research page that showed them as the
// whole answer would be telling somebody the pool holds less than it does.
func TestTheCardSearchRefusesAResultSetCutShort(t *testing.T) {
	t.Parallel()
	path := pooltest.Build(t)
	want := -1
	p := pooltest.Open(t)
	if err := p.Use(t.Context(), func(c *pool.Conn) error {
		out, err := SearchCards(t.Context(), c, SearchQuery{Text: "a"})
		want = len(out)
		return err
	}); err != nil {
		t.Fatalf("the healthy search: %v", err)
	}
	if want < 2 {
		t.Fatalf("the fixture answered %d cards for the letter a -- this test "+
			"needs a result set long enough to cut in the middle", want)
	}

	refused, full := 0, 0
	for rows := 0; rows <= 40; rows++ {
		fp, fault := pooltest.FaultyPoolOver(t, path)
		fault.RowsAfter(rows)
		var got int
		err := fp.Use(t.Context(), func(c *pool.Conn) error {
			out, e := SearchCards(t.Context(), c, SearchQuery{Text: "a"})
			got = len(out)
			return e
		})
		fault.Heal()
		fp.Close()
		if err != nil {
			refused++
			continue
		}
		if got != want {
			t.Errorf("at row budget %d the search answered %d of %d cards with "+
				"no error", rows, got, want)
		}
		full++
	}
	if refused == 0 || full == 0 {
		t.Errorf("%d row budgets refused and %d answered in full", refused, full)
	}
}

// field is one key of an ordered body, or nil.
func field(body wire.OrderedMap, key string) any {
	for _, kv := range body {
		if kv.Key == key {
			return kv.Value
		}
	}
	return nil
}

func cardOf(dossier wire.OrderedMap) wire.OrderedMap {
	card, _ := field(dossier, "card").(wire.OrderedMap)
	return card
}

// relatedCount is the length of the panel's `other_cards` list, or -1 when the
// key is not a list at all.
func relatedCount(dossier wire.OrderedMap) int {
	switch typed := field(dossier, "other_cards").(type) {
	case []wire.OrderedMap:
		return len(typed)
	case []any:
		return len(typed)
	default:
		return -1
	}
}

func listLen(dossier wire.OrderedMap, key string) int {
	switch typed := field(dossier, key).(type) {
	case []wire.OrderedMap:
		return len(typed)
	case []any:
		return len(typed)
	case []string:
		return len(typed)
	default:
		return -1
	}
}

// panelIsWhole is the shape a complete dossier has: a card, and the lists and
// counts that hang off it. A panel that came back without an error and without
// one of them would be the hole this file exists to refuse.
func panelIsWhole(dossier wire.OrderedMap) bool {
	if field(dossier, "card") == nil {
		// The empty shape -- a commander the pool does not hold -- is whole in
		// its own right and is not what the sweep is about.
		return true
	}
	return field(dossier, "printings") != nil &&
		relatedCount(dossier) >= 0 && listLen(dossier, "subtypes") >= 0
}
