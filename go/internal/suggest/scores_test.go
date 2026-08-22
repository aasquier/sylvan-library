package suggest_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pyfloat"
	"github.com/aasquier/sylvan-library/go/internal/suggest"
)

// The scorer's own corpus (`tests/go_fixtures.py:score_cases`): synthetic
// target/candidate pairs built to land on chosen component scores, so the
// arithmetic is covered on purpose rather than wherever four real cards
// happened to fall.
//
// `TestReplacementsAgreeWithPython` next door is the *integration* case -- the
// Titan's slot, a real pool, the real ordering. It is four points in a
// four-dimensional space and it passed throughout the period when this
// package summed the four weighted parts left to right, which is CPython
// 3.11's `sum` and not the 3.12 the image runs.

type scoreRecord struct {
	Name       string   `json:"name"`
	TypeLine   string   `json:"type_line"`
	CMC        float64  `json:"cmc"`
	OracleText string   `json:"oracle_text"`
	Keywords   []string `json:"keywords"`
	EdhrecRank *int     `json:"edhrec_rank"`
}

func (r scoreRecord) record() *pool.CardRecord {
	return &pool.CardRecord{
		Name: r.Name, TypeLine: r.TypeLine, CMC: r.CMC,
		OracleText: r.OracleText, Keywords: r.Keywords,
		EdhrecRank: r.EdhrecRank,
	}
}

type scoreCase struct {
	Note       string      `json:"note"`
	Target     scoreRecord `json:"target"`
	Candidate  scoreRecord `json:"candidate"`
	Parts      []float64   `json:"parts"`
	Weights    []float64   `json:"weights"`
	Popularity float64     `json:"popularity"`
	Score      float64     `json:"score"`
	Reasons    []string    `json:"reasons"`
}

func loadScores(t *testing.T) []scoreCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "scores.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []scoreCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) < 8 {
		t.Fatalf("the corpus has shrunk to %d cases", len(corpus.Cases))
	}
	return corpus.Cases
}

func TestScoreAgreesWithPythonToTheBit(t *testing.T) {
	for _, c := range loadScores(t) {
		got := suggest.Score(c.Target.record(), c.Candidate.record(), "")
		if math.Float64bits(got.Score) != math.Float64bits(c.Score) {
			t.Errorf("%s: Score = %v (%#016x), Python = %v (%#016x)",
				c.Note, got.Score, math.Float64bits(got.Score),
				c.Score, math.Float64bits(c.Score))
		}
		g, _ := json.Marshal(got.Reasons)
		w, _ := json.Marshal(c.Reasons)
		if string(g) != string(w) {
			t.Errorf("%s reasons:\n got %s\nwant %s", c.Note, g, w)
		}
	}
}

// The corpus has to be able to fail, which is not the same as passing.
//
// Every case here would also pass if `Score` added its four weighted parts
// left to right, unless some case's components are ones where a running total
// and a correctly-rounded sum part company. That is the property this
// asserts, and it is the reason three of the cases exist at all -- no real
// pair of cards lands there, so the fixture was cut for it, exactly as
// `curve`'s `tie-breaker` deck was.
func TestTheScoreCorpusSeparatesFsumFromARunningTotal(t *testing.T) {
	cases := loadScores(t)
	differs, rounded := 0, 0
	for _, c := range cases {
		if len(c.Parts) != len(c.Weights) {
			t.Fatalf("%s: %d parts against %d weights", c.Note, len(c.Parts), len(c.Weights))
		}
		products := make([]float64, len(c.Parts))
		naive := 0.0
		for i := range c.Parts {
			products[i] = pyfloat.Rounded(c.Weights[i] * c.Parts[i])
			naive += products[i]
		}
		exact := pyfloat.Fsum(products)
		if math.Float64bits(naive) != math.Float64bits(exact) {
			differs++
			nudge := pyfloat.Rounded(0.10 * c.Popularity)
			if pyfloat.RoundTo(naive+nudge, 4) != pyfloat.RoundTo(exact+nudge, 4) {
				rounded++
			}
		}
	}
	if differs == 0 {
		t.Fatal("no case in the corpus separates fsum from a running total, " +
			"so this package could sum left to right and stay green")
	}
	if rounded == 0 {
		t.Fatal("no case changes the score at four places, so the corpus " +
			"cannot see the difference the wire would carry")
	}
	t.Logf("%d of %d cases separate fsum from a running total; %d of those "+
		"change round(score, 4), which is what is serialised and sorted on",
		differs, len(cases), rounded)
}
