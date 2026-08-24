package analyze_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/analyze"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The differential cases again: each fixture deck's text beside its
// recorded stats answer over the 21-card pool, which is exactly what
// `GET /api/decks/{owner}/{slug}/stats` serves. DeckStats must encode to
// the same document -- same keys, same order, same numbers.

func TestDeckStatsMatchesTheGoldenCaseForCase(t *testing.T) {
	dir := filepath.Join("..", "gate", "testdata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := pooltest.Open(t)
	ctx := context.Background()
	checked := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".stats.json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".stats.json")
		text, err := os.ReadFile(filepath.Join(dir, name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		d, err := deck.FromText(string(text), name)
		if err != nil {
			t.Fatal(err)
		}
		names := append([]string{}, d.Commander...)
		for _, c := range d.Cards {
			names = append(names, c.Name)
		}
		for _, c := range d.SwapBoard {
			names = append(names, c.Name)
		}
		for _, c := range d.Graveyard {
			names = append(names, c.Name)
		}
		if d.Companion != nil {
			names = append(names, *d.Companion)
		}
		if err := p.Use(ctx, func(c *pool.Conn) error {
			cards, err := c.GetCards(ctx, names)
			if err != nil {
				return err
			}
			got, err := wire.Marshal(analyze.DeckStats(d, cards))
			if err != nil {
				return err
			}
			if canonical(t, got) != canonical(t, want) {
				t.Errorf("%s: DeckStats disagrees with the golden\n--- got\n%s\n--- want\n%s", name, got, strings.TrimSpace(string(want)))
			}
			// And the key order is the route's: a Go map would have sorted it.
			if !strings.HasPrefix(string(got), `{"slug":`) || !strings.Contains(string(got), `"curve":{"average_mv":`) {
				t.Errorf("%s: key order drifted: %.120s", name, got)
			}
			checked++
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d stats cases; the gate's testdata goldens have thinned", checked)
	}
}

// canonical re-encodes a JSON document with sorted keys and Go's number
// formatting, so `1.0` and `1` compare equal and order does not matter here
// (it is checked separately).
func canonical(t *testing.T, raw []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	out, _ := json.Marshal(v)
	return string(out)
}

// The corpus has to be able to fail.
//
// `OpeningHand` sums three hypergeometric probabilities into `keepable`,
// and it summed them with a `+=` loop until 2026-08-22 -- a different
// arithmetic, in its last bits, from the compensated sums the corpus
// records. The eight fixture decks that existed then sat at 99 cards on 95
// or 96 lands and at 106 on 96, and the two arithmetics agree at every one
// of those shapes, so the whole differential corpus was green against an
// implementation that answered a different number from the recording.
//
// `last-bit` is the ninth deck, cut for this: 99 cards on 91 lands, where
// 3.11 answers 0.010640320706772594 and 3.12 answers 0.010640320706772595.
// This asserts that some deck in the corpus still lands there.
func TestTheStatsCorpusSeparatesFsumFromARunningTotal(t *testing.T) {
	dir := filepath.Join("..", "gate", "testdata")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	differs, checked := 0, 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".stats.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Opening struct {
				Lands struct {
					Distribution []struct {
						Lands  int     `json:"lands"`
						Chance float64 `json:"chance"`
					} `json:"distribution"`
					Keepable float64 `json:"keepable"`
				} `json:"lands"`
			} `json:"opening"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		terms := []float64{}
		for _, row := range doc.Opening.Lands.Distribution {
			if row.Lands >= 2 && row.Lands <= 4 {
				terms = append(terms, row.Chance)
			}
		}
		if len(terms) == 0 {
			continue
		}
		checked++
		naive := 0.0
		for _, v := range terms {
			naive += v
		}
		if math.Float64bits(naive) != math.Float64bits(doc.Opening.Lands.Keepable) {
			differs++
			t.Logf("%s: running total %v, the golden says %v", e.Name(), naive, doc.Opening.Lands.Keepable)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d land distributions; the gate's testdata goldens have thinned", checked)
	}
	if differs == 0 {
		t.Fatal("no fixture deck's land distribution separates fsum from a " +
			"running total, so `keepable` could be summed left to right and " +
			"the corpus would stay green")
	}
	t.Logf("%d of %d decks separate fsum from a running total", differs, checked)
}
