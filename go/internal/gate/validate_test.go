package gate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/gate"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The differential cases: each fixture deck's text beside its recorded
// report -- with the 21-card pool and without -- and this test must produce
// the same issues, in the same order, with the same sentences. That is the
// Phase 3 gate for the gate: validate matches the golden case for case.

type issue struct {
	Level   string  `json:"level"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Card    *string `json:"card"`
}

type reports struct {
	WithPool    []issue `json:"with_pool"`
	WithoutPool []issue `json:"without_pool"`
}

func cases(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	if len(names) < 5 {
		t.Fatalf("only %d cases; the testdata decks are frozen goldens and at least 5 should be here", len(names))
	}
	return names
}

func load(t *testing.T, name string) (*deck.Deck, reports) {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("testdata", name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := deck.FromText(string(text), name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := os.ReadFile(filepath.Join("testdata", name+".report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want reports
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	return d, want
}

func asIssues(r *gate.Report) []issue {
	out := []issue{}
	for _, i := range r.Issues {
		out = append(out, issue{Level: i.Level, Code: i.Code, Message: i.Message, Card: i.Card})
	}
	return out
}

func same(t *testing.T, name, mode string, got, want []issue) {
	t.Helper()
	g, _ := json.MarshalIndent(got, "", " ")
	w, _ := json.MarshalIndent(want, "", " ")
	if string(g) != string(w) {
		t.Errorf("%s (%s): the gate disagrees with the recorded report\n--- got\n%s\n--- want\n%s", name, mode, g, w)
	}
}

func TestTheGateMatchesTheGoldenCaseForCase(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	ctx := context.Background()
	for _, name := range cases(t) {
		d, want := load(t, name)
		// Without a pool: the structural checks and the `unverified` warning.
		same(t, name, "without_pool", asIssues(gate.Validate(d, nil, gate.DefaultSize)), want.WithoutPool)
		// With the pool: exactly the names `service._pool_for` looks up.
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
			same(t, name, "with_pool", asIssues(gate.Validate(d, cards, gate.DefaultSize)), want.WithPool)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheReportKnowsItsErrorsFromItsWarnings(t *testing.T) {
	t.Parallel()
	// Without a pool the banned card cannot be seen: the report is OK, and
	// carries exactly the one warning that says so.
	d, _ := load(t, "mono-green")
	r := gate.Validate(d, nil, gate.DefaultSize)
	if !r.OK() || len(r.Errors()) != 0 || len(r.Warnings()) != 1 || r.Warnings()[0].Code != "unverified" {
		t.Fatalf("%+v", r.Issues)
	}
	// Two commanders shrink the 99 to 98; Yorion grows it by twenty.
	pair, _ := load(t, "pair")
	found := false
	for _, i := range gate.Validate(pair, nil, gate.DefaultSize).Issues {
		if i.Code == "deck-size" && strings.Contains(i.Message, "expected 98 (2 commanders)") {
			found = true
		}
	}
	if !found {
		t.Fatal("the two-commander size was not adjusted")
	}
}
