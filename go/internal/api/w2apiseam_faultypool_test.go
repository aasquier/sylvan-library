package api

import (
	"net/http"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The card routes against a library that goes away **partway through** a request.
//
// `failingpool_test.go` is the pool with no tables, where every route fails at
// its first query; that proves a route notices a library it cannot read at all.
// What it cannot reach is a route that read something and then lost the handle,
// and these routes all read twice or three times: a shortlist and then the cards
// behind it, or the cards and then a count of the whole combination. Each of
// those second and third reads carries its own refusal and not one of them had
// been entered by anything, because until `pool.Connect` there was no way to
// hand a route a pool that answered once.
//
// **The assertion is the same sentence for every budget and it is the one that
// matters to a player**: at every point the library can go away, the route
// either refuses or answers in full. Never a shortlist missing its tail, never
// a combination whose count is quietly a different number -- a half answer
// rendered as a whole one is the app telling somebody something untrue about
// their own cards (commandment 2), and it is the failure a budget sweep is for.
//
// The budget is spent across every statement on the handle, `Columns`' included,
// so these sweep a range rather than naming a number (`pooltest.Fault` says why).
func TestNoCardRouteAnswersHalfWhenTheLibraryGoesAwayMidRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		what   string
		target string
	}{
		// The typeahead: a shortlist out of the oracle, then the cards behind
		// the names on it.
		{"the typeahead", "/api/cards/suggest?q=sol&limit=5"},
		// The commander field asked about a name no card carries: the lookup,
		// then a shortlist, then the cards behind that.
		{"the commander field", "/api/cards/commander?q=Sol+Rng"},
		// A colour combination: the champions and signature cards, then the
		// count of everything legal in those colours.
		{"a colour combination", "/api/colors/G"},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			// **One pool file per branch.** DuckDB keys a loaded database on
			// its path and refuses a second handle on one it already holds, so
			// two parallel sweeps over one file collide at the open.
			path := pooltest.Build(t)

			healthy := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t)})
			status, _, whole := call(t, healthy, "GET", tc.target, "")
			if status != http.StatusOK {
				t.Fatalf("%s answered %d over a healthy pool: %s", tc.what, status, whole)
			}

			refused, full := 0, 0
			for budget := 0; budget <= 10; budget++ {
				p, fault := pooltest.FaultyPoolOver(t, path)
				fault.After(budget)
				a := New(Config{Logger: quietLogger(), Pool: p})
				status, _, raw := call(t, a, "GET", tc.target, "")
				fault.Heal()
				p.Close()
				if status != http.StatusOK {
					refused++
					continue
				}
				if string(raw) != string(whole) {
					t.Errorf("at statement budget %d %s answered 200 with a "+
						"different answer than the whole one:\n got %s\nwant %s",
						budget, tc.what, raw, whole)
				}
				full++
			}
			if refused == 0 || full == 0 {
				t.Errorf("%s: %d budgets refused and %d answered in full -- the "+
					"sweep never crossed the reads", tc.what, refused, full)
			}
		})
	}
}

// **A shortlist the library cannot build is not a failed import.** `didYouMean`
// is the only read in this package that swallows its own refusal, and it is
// right to: the name is already reported as unknown, which is the load-bearing
// half, and a paste of ninety-nine cards should not be thrown away because the
// pool went away while somebody was being offered a spelling. What it may never
// do is offer half a shortlist, so the proof is one name at a time -- whole, or
// nothing, at every point the library can stop answering.
func TestTheImportsShortlistIsWholeOrAbsentWhenTheLibraryGoesAway(t *testing.T) {
	t.Parallel()

	// Measured against this fixture by `didyoumean_test.go`, which argues the
	// band: wrong enough that the importer will not resolve it, close enough
	// that this tier offers the card back.
	const misspelt = "Cultvatr Colosus"
	path := pooltest.Build(t)

	var want int
	if err := pooltest.Open(t).Use(t.Context(), func(c *pool.Conn) error {
		got, skipped := didYouMean(t.Context(), c, []string{misspelt})
		if len(got) != 1 || skipped != 0 {
			t.Fatalf("the healthy shortlist answered %d entries and skipped %d", len(got), skipped)
		}
		want = len(got[0].Candidates)
		return nil
	}); err != nil {
		t.Fatalf("the healthy pool: %v", err)
	}
	if want == 0 {
		t.Fatalf("%q was offered nothing to lose", misspelt)
	}

	empty, whole := 0, 0
	for budget := 0; budget <= 6; budget++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		var got []suggestion
		err := p.Use(t.Context(), func(c *pool.Conn) error {
			got, _ = didYouMean(t.Context(), c, []string{misspelt})
			return nil
		})
		fault.Heal()
		p.Close()
		if err != nil {
			// The lease itself could not be taken, which is a different fault
			// and one the import's own refusal answers.
			continue
		}
		switch {
		case len(got) == 0:
			empty++
		case len(got) == 1 && len(got[0].Candidates) == want:
			whole++
		default:
			t.Errorf("at statement budget %d the shortlist was neither whole "+
				"nor absent: %+v", budget, got)
		}
	}
	if empty == 0 || whole == 0 {
		t.Errorf("%d budgets answered an empty shortlist and %d a whole one -- "+
			"the sweep never crossed the read", empty, whole)
	}
}

// **The write door's colour read refuses rather than guessing.**
// `chosenColorReach` is the one check in the app whose answer changes as the
// deck does (Tolabow's clause, read back off the instants and sorceries already
// filed), and a pool that will not say what those cards are leaves it with no
// honest answer at all. Handing back an empty reach would be worse than
// refusing: the colour would read as unspent and the next off-colour card would
// be let into the file.
func TestTheChosenColourReadRefusesWhenTheLibraryWillNotAnswer(t *testing.T) {
	t.Parallel()

	d := &deck.Deck{Commander: []string{"Tolabow, Loch Rascal"},
		Cards: []deck.CardEntry{{Name: "Swords to Plowshares"}}}
	p, fault := pooltest.FaultyPoolOver(t, pooltest.Build(t))
	fault.After(0)
	var colors []string
	err := p.Use(t.Context(), func(c *pool.Conn) error {
		var readErr error
		colors, readErr = chosenColorReach(t.Context(), c, d, nil, nil)
		return readErr
	})
	if err == nil {
		t.Fatalf("a pool that answers nothing reported a reach of %v", colors)
	}
	if colors != nil {
		t.Errorf("a refused read still handed back %v", colors)
	}
}

// **The add-a-card door reads twice and owes a refusal on both.** The card being
// added is looked up, and then the deck's commanders are, because the clause
// that decides whether the card may go in is printed on the commander (ADR 51).
// A library that answers the first and not the second cannot be allowed to let
// the card in on a commander it never read -- the identity would be empty, and
// an empty identity admits everything.
func TestTheAddDoorRefusesWhenTheLibraryGoesAwayBeforeTheCommander(t *testing.T) {
	t.Parallel()

	d := &deck.Deck{Commander: []string{"Goreclaw, Terror of Qal Sisma"},
		Cards: []deck.CardEntry{{Name: "Sol Ring"}}}
	path := pooltest.Build(t)

	refused, allowed := 0, 0
	for budget := 0; budget <= 6; budget++ {
		p, fault := pooltest.FaultyPoolOver(t, path)
		fault.After(budget)
		a := New(Config{Logger: quietLogger(), Pool: p})
		rec, err := a.playableCard(t.Context(), d, "Llanowar Reborn", "the pool is away")
		fault.Heal()
		p.Close()
		if err != nil {
			if rec != nil {
				t.Errorf("at statement budget %d the door refused and handed "+
					"back %s anyway", budget, rec.Name)
			}
			refused++
			continue
		}
		if rec == nil || rec.Name != "Llanowar Reborn" {
			t.Errorf("at statement budget %d the door allowed %v", budget, rec)
		}
		allowed++
	}
	if refused == 0 || allowed == 0 {
		t.Errorf("%d budgets refused and %d allowed -- the sweep never crossed "+
			"the two reads", refused, allowed)
	}
}
