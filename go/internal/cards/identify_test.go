package cards_test

import (
	"context"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/cards"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

func TestFromCornerReadsTheBlockAsPythonDoes(t *testing.T) {
	codes := map[string]bool{"LTC": true, "CHR": true, "MH3": true, "LTR": true}
	// The real Lord of the Rings Sol Ring capture from the module docstring.
	s := cards.FromCorner("U0284\nLTCENLIK", codes)
	if s.SetCode != "LTC" || s.CollectorNumber != "0284" {
		t.Fatalf("%+v", s)
	}
	// The artist never gets a vote: CHRISRAHN on its own line has CHR as a
	// prefix but is not the first token of the set-code line... it IS the
	// first token of its line, so it matches -- which is the measured
	// Python behaviour too (the longest real prefix of a line's first token).
	s = cards.FromCorner("CHRISRAHN", codes)
	if s.SetCode != "CHR" {
		t.Fatalf("%+v", s)
	}
	// A year is the copyright line, not a collector number; a digit-first
	// token is never a set code.
	s = cards.FromCorner("2024 Wizards\n123", codes)
	if s.CollectorNumber != "123" || s.SetCode != "" {
		t.Fatalf("%+v", s)
	}
	if s := cards.FromCorner("", codes); s.SetCode != "" || s.CollectorNumber != "" {
		t.Fatal("empty")
	}
	if cards.FaceNumber("284/281") != "284" || cards.FaceNumber(" 0123 ") != "0123" || cards.FaceNumber("") != "" {
		t.Fatal("face number")
	}
}

func TestReadResolvesOnlyThroughAPrinting(t *testing.T) {
	ctx := context.Background()
	p := pooltest.Open(t)
	if err := p.Use(ctx, func(c *pool.Conn) error {
		readings, err := cards.Read(ctx, c, []cards.Sighting{
			{SetCode: "LTC", CollectorNumber: "284/281"},            // the corner tier, padded on the face
			{Corner: "U0284\nLTCENLIK"},                             // the raw block, read against the pool's codes
			{Title: "Sol Rng"},                                      // a title only ever offers
			{SetCode: "ZZZ", CollectorNumber: "1", Title: "Forest"}, // a bad corner falls through to the title
			{}, // nothing at all
		})
		if err != nil {
			return err
		}
		if len(readings) != 5 {
			t.Fatalf("%d readings", len(readings))
		}
		if readings[0].Via != "printing" || readings[0].Resolved != "Sol Ring" {
			t.Fatalf("corner: %+v", readings[0])
		}
		if readings[1].Via != "printing" || readings[1].Resolved != "Sol Ring" {
			t.Fatalf("raw corner: %+v", readings[1])
		}
		if readings[2].Via != "title" || readings[2].Resolved != "" || len(readings[2].Candidates) == 0 ||
			readings[2].Candidates[0].Name != "Sol Ring" {
			t.Fatalf("title: %+v", readings[2])
		}
		if readings[3].Via != "title" || readings[3].Candidates[0].Name != "Forest" {
			t.Fatalf("fallthrough: %+v", readings[3])
		}
		if readings[4].Via != "nothing" || len(readings[4].Candidates) != 0 || readings[4].Candidates == nil {
			t.Fatalf("nothing: %+v", readings[4])
		}
		// A banned card is identifiable; an emblem is not a card.
		cands, _ := cards.ByTitle(ctx, c, "Primeval Titan", 1)
		if len(cands) != 1 || cands[0].Name != "Primeval Titan" {
			t.Fatalf("banned: %+v", cands)
		}
		cands, _ = cards.ByTitle(ctx, c, "Ajani Steadfast Emblem", 5)
		for _, cand := range cands {
			if cand.Name == "Ajani Steadfast Emblem" {
				t.Fatal("an emblem was offered as a card")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
