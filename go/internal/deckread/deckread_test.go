package deckread

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The fixtures are `internal/gate/testdata`: each deck's YAML beside its
// recorded report, stats and suggestions. `internal/api` already drives them
// through the ROUTES, and those tests still pass — which is what proves this
// extraction moved nothing.
//
// These drive the same fixtures through the FUNCTIONS instead, which is not
// redundant: the Claude tools call them this way, with a pool they leased
// themselves and no HTTP anywhere. A builder that is only ever exercised
// through a handler is a builder whose direct contract is untested, and "tests
// must drive the trigger" is a lesson this repository has already paid for
// twice.
const fixtures = "../gate/testdata"

func fixtureDecks(t *testing.T) map[string]*deck.Deck {
	t.Helper()
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		t.Fatalf("reading the gate fixtures: %v", err)
	}
	out := map[string]*deck.Deck{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".yaml")
		text, err := os.ReadFile(filepath.Join(fixtures, e.Name()))
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		d, err := deck.FromText(string(text), slug)
		if err != nil {
			t.Fatalf("%s: %v", slug, err)
		}
		out[slug] = d
	}
	if len(out) == 0 {
		t.Fatal("no fixture decks; this file would pass vacuously")
	}
	return out
}

func withPool(t *testing.T, fn func(c *pool.Conn)) {
	t.Helper()
	p := pooltest.Open(t)
	if err := p.Use(context.Background(), func(c *pool.Conn) error {
		fn(c)
		return nil
	}); err != nil {
		t.Fatalf("leasing the pool: %v", err)
	}
}

// TestValidateAndStatsMatchTheGoldensWhenCalledDirectly is the fixture
// check, driven the way a Claude tool will drive it.
func TestValidateAndStatsMatchTheGoldensWhenCalledDirectly(t *testing.T) {
	t.Parallel()
	decks := fixtureDecks(t)
	withPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		for slug, d := range decks {
			rep, err := Validate(ctx, c, d)
			if err != nil {
				t.Errorf("%s validate: %v", slug, err)
				continue
			}
			raw, err := os.ReadFile(filepath.Join(fixtures, slug+".report.json"))
			if err != nil {
				continue
			}
			var want struct {
				WithPool []map[string]any `json:"with_pool"`
			}
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("%s report fixture: %v", slug, err)
			}
			wantErrs, wantWarns := 0, 0
			for _, i := range want.WithPool {
				if i["level"] == "error" {
					wantErrs++
				} else {
					wantWarns++
				}
			}
			if len(rep.Errors()) != wantErrs || len(rep.Warnings()) != wantWarns {
				t.Errorf("%s validate: %d errors %d warnings, want %d/%d",
					slug, len(rep.Errors()), len(rep.Warnings()), wantErrs, wantWarns)
			}
			if rep.OK() != (wantErrs == 0) {
				t.Errorf("%s validate: ok=%v with %d errors", slug, rep.OK(), wantErrs)
			}

			stats, err := Stats(ctx, c, d)
			if err != nil {
				t.Errorf("%s stats: %v", slug, err)
				continue
			}
			wantStats, err := os.ReadFile(filepath.Join(fixtures, slug+".stats.json"))
			if err != nil {
				continue
			}
			got, err := json.Marshal(stats)
			if err != nil {
				t.Fatalf("%s stats marshal: %v", slug, err)
			}
			if canonical(t, got) != canonical(t, wantStats) {
				t.Errorf("%s stats disagree\n got  %s\n want %s", slug, got, strings.TrimSpace(string(wantStats)))
			}
		}
	})
}

func TestSuggestionsMatchTheGoldensWhenCalledDirectly(t *testing.T) {
	t.Parallel()
	decks := fixtureDecks(t)
	withPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		var checked int
		for slug, d := range decks {
			wantRaw, err := os.ReadFile(filepath.Join(fixtures, slug+".suggestions.json"))
			if err != nil {
				continue
			}
			targets, err := Suggestions(ctx, c, d, 5)
			if err != nil {
				t.Errorf("%s suggestions: %v", slug, err)
				continue
			}
			var wantDoc map[string]any
			if err := json.Unmarshal(wantRaw, &wantDoc); err != nil {
				t.Fatalf("%s suggestions fixture: %v", slug, err)
			}
			gotDoc, err := json.Marshal(map[string]any{
				"slug": slug, "pool_available": true, "targets": targets})
			if err != nil {
				t.Fatalf("%s marshal: %v", slug, err)
			}
			// The answer echoes the REQUESTED slug; the fixture carries the
			// deck's own, and `draft.yaml` says `slug: mono-green`.
			wantDoc["slug"] = slug
			wantNorm, _ := json.Marshal(wantDoc)
			if canonical(t, gotDoc) != canonical(t, wantNorm) {
				t.Errorf("%s suggestions disagree\n got  %s\n want %s", slug, gotDoc, wantNorm)
			}
			checked++
		}
		if checked == 0 {
			t.Error("no deck had a suggestions fixture; this test proved nothing")
		}
	})
}

// TestTheNilPoolDegradesRatherThanFailing is the shape every deck route
// already had, and the one a new caller is most likely to get wrong.
//
// A missing pool is not an error. It is a deck answered from its own file:
// `pool_available` says so, every card row falls back to name-and-rationale,
// and the gate is told it was never consulted. The Claude tools inherit this
// the moment they call these functions, which is the point of them being
// shared rather than reimplemented.
func TestTheNilPoolDegradesRatherThanFailing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for slug, d := range fixtureDecks(t) {
		body, err := DeckPayload(ctx, nil, d, true, "local")
		if err != nil {
			t.Errorf("%s: a nil pool failed: %v", slug, err)
			continue
		}
		found := false
		for _, pair := range body {
			if pair.Key == "pool_available" {
				found = true
				if pair.Value != false {
					t.Errorf("%s: pool_available is %v with no pool", slug, pair.Value)
				}
			}
		}
		if !found {
			t.Errorf("%s: the payload does not say whether a pool answered", slug)
		}
		if _, err := Validate(ctx, nil, d); err != nil {
			t.Errorf("%s: validate failed with no pool: %v", slug, err)
		}
		if _, err := Stats(ctx, nil, d); err != nil {
			t.Errorf("%s: stats failed with no pool: %v", slug, err)
		}
	}
}

// TestTheDeckPayloadKeepsTheRecordedKeyOrder is the regression that already
// shipped once: the deck page's Notes tab was alphabetical from v159 to
// v166, because `encoding/json` sorts a map's keys and the payload's order
// is deliberate. The payload is ordered pairs for that reason, and this
// pins the order rather than trusting the type to preserve it.
func TestTheDeckPayloadKeepsTheRecordedKeyOrder(t *testing.T) {
	t.Parallel()
	decks := fixtureDecks(t)
	var any *deck.Deck
	for _, d := range decks {
		any = d
		break
	}
	body, err := DeckPayload(context.Background(), nil, any, true, "local")
	if err != nil {
		t.Fatalf("building the payload: %v", err)
	}
	want := []string{
		"commander_art", "slug", "name", "writable", "owner", "shared",
		"pilot", "status", "stage", "needs_rationale", "commander",
		"companion", "bracket", "archetype", "themes", "strategy", "notes",
		"total_cards", "land_count", "color_identity", "commander_card",
		"cards", "swap_board", "graveyard", "pool_available",
	}
	if len(body) != len(want) {
		t.Fatalf("payload has %d keys, want %d", len(body), len(want))
	}
	for i, key := range want {
		if body[i].Key != key {
			t.Errorf("key %d is %q, want %q -- the deck page reads this order",
				i, body[i].Key, key)
		}
	}
	// And it must actually marshal in that order, which is the half the type
	// is responsible for.
	raw, err := wire.MarshalOrdered(body)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if !strings.HasPrefix(string(raw), `{"commander_art":`) {
		t.Errorf("the marshalled body does not lead with commander_art:\n%s", raw[:80])
	}
}

func canonical(t *testing.T, raw []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("canonicalising: %v", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("canonicalising: %v", err)
	}
	return string(out)
}

// The cheapest price is asked for in its own statement, after the LIMIT — see
// the argument beside the search query. What the split must not change is the
// answer, and there are four shapes it could get wrong: the minimum across
// several *priced* printings, an unpriced printing sitting beside a priced
// one, a card whose printings are all unpriced, and a card with no printings
// at all.
//
// The fixture carries three of those; the fourth is doctored in, because the
// fixture has no card with two different prices and a `max` in place of the
// `min` would otherwise pass. Doctoring the test's own temp copy is what
// `stale_test.go` already does, and it is not a fixture edit: the recorded
// corpus on disk is untouched.
func TestSearchPricesEachCardAtItsCheapestPrintingOrNotAtAll(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := pooltest.Build(t)
	db, err := pooltest.Writer(path)
	if err != nil {
		t.Fatalf("opening the fixture to doctor it: %v", err)
	}
	// Two more Sol Ring printings: one cheaper than the recorded 2.51, one
	// dearer. Only the cheapest may reach a search result.
	if _, err := db.Exec(`INSERT INTO printings (id, oracle_id, name, price_usd)
             SELECT 'black-cheap', oracle_id, name, 0.42 FROM oracle_cards WHERE name = 'Sol Ring'
             UNION ALL
             SELECT 'black-dear', oracle_id, name, 99.99 FROM oracle_cards WHERE name = 'Sol Ring'`); err != nil {
		t.Fatalf("doctoring the printings: %v", err)
	}
	_ = db.Close()

	p := pool.New(path, nil)
	t.Cleanup(p.Close)
	if err := p.Use(ctx, func(c *pool.Conn) error {
		found, err := SearchCards(ctx, c, SearchQuery{Limit: 200})
		if err != nil {
			return err
		}
		prices := map[string]*float64{}
		for _, card := range found {
			prices[card.Name] = card.PriceUSD
		}
		// Four printings now: 0.42, 2.51, 99.99 and one unpriced.
		if got := prices["Sol Ring"]; got == nil || *got != 0.42 {
			t.Fatalf("Sol Ring priced at %s, want 0.42 -- the cheapest of four",
				showPrice(got))
		}
		// Two printings, neither priced.
		if got, ok := prices["Goreclaw, Terror of Qal Sisma"]; !ok || got != nil {
			t.Fatalf("Goreclaw priced at %s, want no price at all", showPrice(got))
		}
		// No printings at all — a card the pool knows and nobody sells.
		if got, ok := prices["Swords to Plowshares"]; !ok || got != nil {
			t.Fatalf("Swords to Plowshares priced at %s, want no price at all",
				showPrice(got))
		}
		// Every other card's price is the one the printings table holds,
		// computed here rather than typed: a test that restates the number it
		// checks cannot tell you the number is wrong. This is also what
		// catches a price landing on the wrong row.
		for name, got := range prices {
			var want *float64
			row := c.DB().QueryRowContext(ctx,
				`SELECT min(p.price_usd) FROM printings p JOIN oracle_cards o
                   ON o.oracle_id = p.oracle_id WHERE o.name = ?`, name)
			var v any
			if err := row.Scan(&v); err != nil {
				return err
			}
			want = AsFloatPtr(v)
			switch {
			case want == nil && got == nil:
			case want == nil || got == nil, *want != *got:
				t.Fatalf("%s priced at %s, the printings say %s",
					name, showPrice(got), showPrice(want))
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("searching: %v", err)
	}
}

// showPrice renders a nullable price for a failure message. Without it a
// `%v` on a *float64 prints the address, which is the one number nobody
// wants at the moment a test breaks.
func showPrice(p *float64) string {
	if p == nil {
		return "no price"
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}
