package prices

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

type pricesFile struct {
	Table map[string]struct {
		Input, Output float64
		ThenInput     *float64 `json:"then_input"`
		ThenOutput    *float64 `json:"then_output"`
		Until         string
	}
	CacheReadFraction float64 `json:"cache_read_fraction"`
	Cases             []struct {
		Name string
		When string
		Rows []struct {
			Model           string
			Conversations   int64
			InputTokens     int64 `json:"input_tokens"`
			OutputTokens    int64 `json:"output_tokens"`
			CacheReadTokens int64 `json:"cache_read_tokens"`
		}
		Rendered string
	}
}

func load(t *testing.T) pricesFile {
	t.Helper()
	raw, err := os.ReadFile("testdata/prices.json")
	if err != nil {
		t.Fatalf("prices.json: %v (regenerate with `python tests/go_fixtures.py`)", err)
	}
	var fx pricesFile
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

// TestTheTableMatchesPython holds the two copies of the price table equal —
// an edit to `prices.py` fails here until this file moves too, which is the
// whole reason the corpus records the table and not only the answers.
func TestTheTableMatchesPython(t *testing.T) {
	fx := load(t)
	if len(fx.Table) != len(Table) {
		t.Fatalf("Python prices %d models, Go %d", len(fx.Table), len(Table))
	}
	if fx.CacheReadFraction != CacheReadFraction {
		t.Fatalf("cache read fraction %v != %v", fx.CacheReadFraction, CacheReadFraction)
	}
	for model, want := range fx.Table {
		got, ok := Table[model]
		if !ok {
			t.Errorf("%s is priced in Python and absent here", model)
			continue
		}
		if got.Rate.Input != want.Input || got.Rate.Output != want.Output {
			t.Errorf("%s: rate %v/%v != %v/%v", model,
				got.Rate.Input, got.Rate.Output, want.Input, want.Output)
		}
		if (want.ThenInput != nil) != (got.Then != nil) {
			t.Errorf("%s: scheduled change disagreement", model)
			continue
		}
		if want.ThenInput != nil && (got.Then.Input != *want.ThenInput ||
			got.Then.Output != *want.ThenOutput || got.Until != want.Until) {
			t.Errorf("%s: window %v until %q != %v/%v until %q", model,
				got.Then, got.Until, *want.ThenInput, *want.ThenOutput, want.Until)
		}
	}
}

// TestEveryEstimateMatchesPython is the corpus: `estimate(...).as_dict()`
// compared as marshalled bytes — the half-to-even rounding, the window on
// both sides of Sonnet 5's changeover, and the unpriced accounting.
func TestEveryEstimateMatchesPython(t *testing.T) {
	fx := load(t)
	if len(fx.Cases) < 6 {
		t.Fatalf("only %d cases; the corpus has thinned", len(fx.Cases))
	}
	for _, tc := range fx.Cases {
		rows := make([]Row, 0, len(tc.Rows))
		for _, r := range tc.Rows {
			rows = append(rows, Row{Model: r.Model, Conversations: r.Conversations,
				InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
				CacheRead: r.CacheReadTokens})
		}
		got, err := wire.MarshalOrdered(Over(rows, tc.When).AsDict())
		if err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		if string(got) != tc.Rendered {
			t.Errorf("%s diverged:\n got %s\nwant %s", tc.Name, got, tc.Rendered)
		}
	}
}
