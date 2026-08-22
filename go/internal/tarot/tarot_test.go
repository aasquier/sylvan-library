package tarot

import (
	"encoding/json"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pyrand"
)

type dealCorpus struct {
	Searched   map[string]int64 `json:"searched"`
	PoolTotals []struct {
		Cards     int    `json:"cards"`
		FsumBits  uint64 `json:"fsum_bits"`
		NaiveBits uint64 `json:"naive_bits"`
		Differ    bool   `json:"differ"`
	} `json:"pool_totals"`
	SeedStrings []struct {
		Text  string `json:"text"`
		OK    bool   `json:"ok"`
		Value string `json:"value"`
	} `json:"seed_strings"`
	Cases []struct {
		Seed     int64  `json:"seed"`
		AsDict   string `json:"as_dict"`
		Describe string `json:"describe"`
	} `json:"cases"`
}

func loadDeals(t *testing.T) dealCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "deals.json"))
	if err != nil {
		t.Fatalf("reading the deal corpus: %v", err)
	}
	var c dealCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decoding the deal corpus: %v", err)
	}
	if len(c.Cases) == 0 {
		t.Fatal("the deal corpus is empty")
	}
	return c
}

// TestASeedDealsTheSameSpreadAsPython is the promise this package exists to
// keep, and the reason pyrand was built bit-exact rather than merely
// well-distributed.
//
// Compared as the SERIALISED payload, not field by field: this is what the
// browser renders, and a reading with the right cards in the wrong wire shape
// is still a broken reload. Drawn.MarshalJSON is what that comparison checks,
// and it exists because tier1.Number taught this repo that a type proved
// correct by every other means can still be wrong on the wire.
func TestASeedDealsTheSameSpreadAsPython(t *testing.T) {
	for _, tc := range loadDeals(t).Cases {
		got, err := json.Marshal(Deal(big.NewInt(tc.Seed)))
		if err != nil {
			t.Fatalf("seed %d: marshalling: %v", tc.Seed, err)
		}
		if string(got) != tc.AsDict {
			t.Errorf("seed %d:\n go     %s\n python %s", tc.Seed, got, tc.AsDict)
		}
	}
}

// TestTheReadersProseIsPythonsOwn covers describe(), whose two extra
// paragraphs are detected facts no card field states directly.
func TestTheReadersProseIsPythonsOwn(t *testing.T) {
	for _, tc := range loadDeals(t).Cases {
		if got := Deal(big.NewInt(tc.Seed)).Describe(); got != tc.Describe {
			t.Errorf("seed %d describe:\n--- go ---\n%s\n--- python ---\n%s",
				tc.Seed, got, tc.Describe)
		}
	}
}

// TestTheSearchedSeedsReachEveryProseBranch is the test that keeps the corpus
// honest.
//
// Four of its seeds were searched for rather than swept, because a plain range
// reaches none of these: the last one is a trump landing twice at one table,
// which describe() itself calls the rarest thing this spread can do. If a
// future change to the deck or the weights stops those seeds producing those
// states, the branches go uncovered SILENTLY — the corpus still matches,
// because it would be regenerated from the same code. So the states are
// asserted here by name, against the Go implementation, rather than trusted.
func TestTheSearchedSeedsReachEveryProseBranch(t *testing.T) {
	c := loadDeals(t)
	for _, want := range []string{"crossover", "echo", "reversed", "alignment"} {
		seed, ok := c.Searched[want]
		if !ok {
			t.Errorf("the corpus names no seed for %q", want)
			continue
		}
		r := Deal(big.NewInt(seed))
		var reached bool
		switch want {
		case "crossover":
			for _, d := range r.Cards {
				reached = reached || d.Card.After != nil
			}
		case "echo":
			for _, d := range r.Cards {
				reached = reached || d.Card.Echo
			}
		case "reversed":
			for _, d := range r.Cards {
				reached = reached || d.Reversed
			}
		case "alignment":
			seen := map[string]int{}
			for _, d := range r.Cards {
				suit := ""
				if d.Card.Suit != nil {
					suit = *d.Card.Suit
				}
				k := d.Card.Arcana + "|" + suit + "|" + string(rune(d.Card.Number))
				seen[k]++
				reached = reached || seen[k] > 1
			}
		}
		if !reached {
			t.Errorf("seed %d no longer produces a %s spread", seed, want)
		}
	}
	// And the prose branches those states drive, checked at the text.
	prose := Deal(big.NewInt(c.Searched["alignment"])).Describe()
	if !strings.Contains(prose, "The stars have aligned") {
		t.Error("the alignment paragraph is unreachable in the corpus")
	}
	if !strings.Contains(prose, "an omen in its own right") {
		t.Error("the omen paragraph is unreachable in the corpus")
	}
}

// TestTheSpreadIsTheThemeInterviewsFirstThreeSlots pins the coupling whose
// failure is silent.
//
// tarot.SPREAD's three positions ARE the theme interview's first three slot
// kinds, with len(SPREAD) == FLOOR. A card is dealt *for* a slot, which is what
// lets ADR 20's grounded-quote readiness work untouched and keeps the querent's
// own words the only evidence — a card is not something they said. Drift, and
// nothing errors: the proposal button simply never lights up.
func TestTheSpreadIsTheThemeInterviewsFirstThreeSlots(t *testing.T) {
	want := []string{"taste", "temperament", "posture"}
	if len(Spread) != len(want) {
		t.Fatalf("the spread is %d positions, the floor is %d", len(Spread), len(want))
	}
	for i, slot := range want {
		if Spread[i].Slot != slot {
			t.Errorf("spread[%d] is for %q, want %q", i, Spread[i].Slot, slot)
		}
		if Spread[i].Name == "" || Spread[i].Asks == "" {
			t.Errorf("spread[%d] has no name or no question", i)
		}
	}
}

// TestTheDeckIsAllOfIt guards the embed itself. A truncated data file would
// make every seeded deal above disagree, but it would disagree confusingly;
// this says what actually happened.
func TestTheDeckIsAllOfIt(t *testing.T) {
	if len(FullDeck) != 136 {
		t.Errorf("the shuffled deck is %d cards, want 136 "+
			"(78 natural, plus Magic's crossovers and echoes)", len(FullDeck))
	}
	var natural, crossover, echo int
	for _, c := range FullDeck {
		switch {
		case c.After == nil:
			natural++
		case c.Echo:
			echo++
		default:
			crossover++
		}
	}
	if natural != 78 {
		t.Errorf("%d natural cards, want the full 1909 deck of 78", natural)
	}
	if crossover == 0 || echo == 0 {
		t.Errorf("both Magic tiers must be present: %d crossovers, %d echoes",
			crossover, echo)
	}
	// Every card's weight must be positive, or the sampler's accumulate-until-
	// past-the-mark loop could never select it and it would be in the deck in
	// name only.
	for _, c := range FullDeck {
		if c.Weight <= 0 {
			t.Errorf("%s has weight %v and can never be dealt", c.Key, c.Weight)
		}
	}
}

// TestAnUnseededDealIsStillReproducibleFromItsOwnSeed covers the one path the
// corpus cannot: nobody holds a seed that has not been minted yet, so what
// must hold is that the answer carries the seed that reproduces it.
func TestAnUnseededDealIsStillReproducibleFromItsOwnSeed(t *testing.T) {
	seen := map[string]bool{}
	for range 32 {
		first := Deal(nil)
		if first.Seed.Sign() < 0 || first.Seed.Cmp(big.NewInt(1<<31)) >= 0 {
			t.Fatalf("minted seed %s is outside randrange(2**31)", first.Seed)
		}
		seen[first.Seed.String()] = true
		again := Deal(first.Seed)
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(again)
		if string(a) != string(b) {
			t.Fatalf("seed %s did not re-deal itself:\n %s\n %s", first.Seed, a, b)
		}
	}
	if len(seen) < 30 {
		t.Errorf("32 unseeded deals produced only %d distinct seeds; the mint "+
			"is not drawing from entropy", len(seen))
	}
}

// TestTheRunningTotalIsAnFsumAndNotASum tests the one thing the deals above
// structurally cannot.
//
// Swapping pyfloat.Fsum for `total += w` changes no spread in this corpus, and
// no corpus of any size would change: tarot.py measured 200,000 seeds dealing
// identically, because mark would have to land inside a 2.8e-14 window out of
// 90.2 to notice — about 3e-16 per draw. Searching for a separating seed is not
// slow, it is hopeless, and a mutation that cannot be killed by the obvious
// instrument is usually reported as "equivalent" and dropped.
//
// It is not equivalent. A bare sum over floats is interpreter-dependent —
// CPython 3.12 accumulates it compensated, 3.11 adds left to right — and a Go
// port written as a running total reproduces 3.11, the interpreter the image
// is NOT running. The two differ here by 2 ULP on every pool a deal touches.
// Nobody has been dealt a wrong spread by it yet, and "it has not been claimed
// yet" is not the same as "it cannot be": a seed is a promise to somebody who
// reloads the page.
//
// So the sum is tested at the sum, where the difference is visible, rather
// than at the deal, where it is not. The corpus records BOTH arithmetics and
// asserts they disagree, so this test proves it can fail before it passes.
func TestTheRunningTotalIsAnFsumAndNotASum(t *testing.T) {
	c := loadDeals(t)
	if len(c.PoolTotals) == 0 {
		t.Fatal("the corpus records no pool totals; the fsum claim is untested")
	}
	// Driven through weightedSample itself, not through a hand-rolled
	// pyfloat.Fsum call. That distinction is the whole test: recomputing the
	// sum here passes against a weightedSample that adds in a loop, which was
	// confirmed by mutation before this was rewritten.
	_, totals := weightedSample(pyrand.New(0), len(Spread))
	if len(totals) != len(c.PoolTotals) {
		t.Fatalf("the sampler used %d totals, the corpus records %d",
			len(totals), len(c.PoolTotals))
	}
	for i, row := range c.PoolTotals {
		if !row.Differ || row.FsumBits == row.NaiveBits {
			t.Errorf("the %d-card pool does not separate fsum from a running "+
				"total; this test cannot fail and so proves nothing", row.Cards)
			continue
		}
		got := math.Float64bits(totals[i])
		if got == row.NaiveBits {
			t.Errorf("draw %d (%d cards): the sampler is computing 3.11's sum, "+
				"not an fsum", i+1, row.Cards)
			continue
		}
		if got != row.FsumBits {
			t.Errorf("draw %d (%d cards): go %d, python fsum %d (a naive total "+
				"would be %d)", i+1, row.Cards, got, row.FsumBits, row.NaiveBits)
		}
	}
}

// TestTheMarkComparisonIsStrictAndThatIsUnobservable records the package's
// other surviving mutation, so the next session does not spend an afternoon on
// it.
//
// `mark < acc` and `mark <= acc` differ only when a uniform double lands
// EXACTLY on an accumulated weight boundary. That is reachable in principle and
// has probability around 2^-52 per draw in practice, so no corpus separates
// them and none should be built trying. It stays `<` because that is what
// Python does and the port's rule is to reproduce rather than to improve;
// this test pins the boundary behaviour that IS observable — the fallback when
// the loop runs off the end, which the strictness makes reachable at all.
func TestTheMarkComparisonIsStrictAndThatIsUnobservable(t *testing.T) {
	// Every draw must return a card, including the path where float summation
	// leaves mark a hair past the final accumulation.
	for seed := int64(0); seed < 2000; seed++ {
		r := Deal(big.NewInt(seed))
		if len(r.Cards) != len(Spread) {
			t.Fatalf("seed %d dealt %d cards", seed, len(r.Cards))
		}
		seen := map[string]bool{}
		for _, d := range r.Cards {
			if seen[d.Card.Key] {
				t.Fatalf("seed %d dealt %s twice; the sample is not without "+
					"replacement", seed, d.Card.Key)
			}
			seen[d.Card.Key] = true
		}
	}
}

// TestTheSeedGrammarIsPydanticsAndNotStrconvs walks every string the corpus
// records, accepted and refused alike.
//
// Three of the accepted rows are 422s from a door written with
// strconv.ParseInt — "  7  ", "+7" and above all "1_0", which is ten. Two of
// the refused rows are 200s from a door written with Python's int(), which
// takes any Unicode decimal digit: the fullwidth "７" and the Arabic-Indic
// "٧". Pydantic sits between the two libraries and matches neither, so the
// grammar is hand-written and this is what holds it there.
func TestTheSeedGrammarIsPydanticsAndNotStrconvs(t *testing.T) {
	c := loadDeals(t)
	if len(c.SeedStrings) == 0 {
		t.Fatal("the corpus records no seed strings; the grammar is untested")
	}
	var accepted, refused int
	for _, row := range c.SeedStrings {
		got, ok := ParseSeed(row.Text)
		if ok != row.OK {
			t.Errorf("seed %q: go ok=%v, python ok=%v", row.Text, ok, row.OK)
			continue
		}
		if !ok {
			refused++
			continue
		}
		accepted++
		if got.String() != row.Value {
			t.Errorf("seed %q: go %s, python %s", row.Text, got, row.Value)
		}
	}
	// A corpus of all-accepts or all-refuses would pass against a parser that
	// answered one way for everything.
	if accepted == 0 || refused == 0 {
		t.Errorf("the corpus does not exercise both branches: %d accepted, "+
			"%d refused", accepted, refused)
	}
}

// TestAnOversizedSeedIsNotTruncated is the row an int64 port gets wrong twice
// over: a different reading, returned under a different number.
func TestAnOversizedSeedIsNotTruncated(t *testing.T) {
	huge, ok := ParseSeed("1180591620717411303424") // 2**70
	if !ok {
		t.Fatal("2**70 is a legal Python seed and must parse")
	}
	r := Deal(huge)
	if r.Seed.Cmp(huge) != 0 {
		t.Errorf("the reading came back under seed %s, not %s", r.Seed, huge)
	}
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshalling an oversized seed: %v", err)
	}
	if !strings.Contains(string(body), `"seed":1180591620717411303424`) {
		t.Errorf("the seed did not survive the wire:\n%s", body[:120])
	}
	// And it must actually deal — pyrand seeds through init_by_array, which
	// grows the key past 2**32 and again past 2**64.
	if len(r.Cards) != len(Spread) {
		t.Errorf("an oversized seed dealt %d cards", len(r.Cards))
	}
}
