package analyze_test

import (
	"context"
	"encoding/json"
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

// The differential cases again (`tests/go_fixtures.py`): each fixture deck's
// text beside `service.stats_for`'s answer over the 21-card pool, which is
// exactly what `GET /api/decks/{owner}/{slug}/stats` serves. DeckStats must
// encode to the same document -- same keys, same order, same numbers.

func TestDeckStatsAgreesWithPythonCaseForCase(t *testing.T) {
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
				t.Errorf("%s: DeckStats disagrees with Python\n--- got\n%s\n--- want\n%s", name, got, strings.TrimSpace(string(want)))
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
		t.Fatalf("only %d stats cases; regenerate with `python tests/go_fixtures.py`", checked)
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
