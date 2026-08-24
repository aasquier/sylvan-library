package suggest_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/suggest"
)

// The differential case: the suggestions payload over the mono-green
// fixture -- the banned Titan's slot, its four candidates, their scores and
// their reasons -- against the recorded answer.
func TestReplacementsMatchTheRecordedAnswer(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "gate", "testdata")
	text, err := os.ReadFile(filepath.Join(dir, "mono-green.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "mono-green.suggestions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Targets []struct {
			Card       string `json:"card"`
			Code       string `json:"code"`
			Why        string `json:"why"`
			Candidates []struct {
				Name    string   `json:"name"`
				Score   float64  `json:"score"`
				Reasons []string `json:"reasons"`
			} `json:"candidates"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if len(want.Targets) != 1 || want.Targets[0].Card != "Primeval Titan" {
		t.Fatalf("the fixture is not the Titan's slot: %+v", want.Targets)
	}
	d, err := deck.FromText(string(text), "mono-green")
	if err != nil {
		t.Fatal(err)
	}
	p := pooltest.Open(t)
	ctx := context.Background()
	if err := p.Use(ctx, func(c *pool.Conn) error {
		names := append([]string{}, d.Commander...)
		for _, card := range d.Cards {
			names = append(names, card.Name)
		}
		cards, err := c.GetCards(ctx, names)
		if err != nil {
			return err
		}
		got, err := suggest.ReplacementsFor(ctx, c, d, cards, "Primeval Titan", 5)
		if err != nil {
			return err
		}
		wantC := want.Targets[0].Candidates
		if len(got) != len(wantC) {
			t.Fatalf("%d candidates, want %d", len(got), len(wantC))
		}
		for i, cand := range got {
			if cand.Name() != wantC[i].Name || cand.Score != wantC[i].Score {
				t.Errorf("candidate %d: %s %.4f, want %s %.4f", i, cand.Name(), cand.Score, wantC[i].Name, wantC[i].Score)
			}
			g, _ := json.Marshal(cand.Reasons)
			w, _ := json.Marshal(wantC[i].Reasons)
			if string(g) != string(w) {
				t.Errorf("candidate %d reasons:\n got %s\nwant %s", i, g, w)
			}
		}
		// An unknown card is an empty list, never an error.
		none, err := suggest.ReplacementsFor(ctx, c, d, cards, "Not A Card", 5)
		if err != nil || len(none) != 0 {
			t.Fatalf("unknown card: %v %v", none, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheScorersReadCardsAsRecorded(t *testing.T) {
	t.Parallel()
	if suggest.PrimaryType("Legendary Artifact Creature — Golem") != "Creature" {
		t.Fatal("a creature is a creature first")
	}
	if suggest.PrimaryType("Creature — Elf // Land") != "Creature" || suggest.PrimaryType("") != "" {
		t.Fatal("front face")
	}
	toks := suggest.Tokens("Whenever a creature you control dies, draw a card. An", "")
	if toks["whenever"] || !toks["creature"] || !toks["control"] || toks["you"] || toks["an"] || !toks["dies"] {
		t.Fatalf("tokens %v", toks)
	}
}
