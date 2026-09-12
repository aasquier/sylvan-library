package brew

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/tarot"
)

// potCorpus is `testdata/deals.json`, which is FROZEN. See the note on
// [loadDeals] before touching it.
type potCorpus struct {
	Frozen      string `json:"frozen"`
	SeedStrings []struct {
		Text  string `json:"text"`
		OK    bool   `json:"ok"`
		Value string `json:"value"`
	} `json:"seed_strings"`
	Cases []struct {
		// A STRING, where the deck's corpus records an int64. The grammar is
		// unbounded and the corpus has to be able to carry a seed an int64
		// cannot -- 2**70 is a legal seed, and a corpus that could not hold
		// one could not record the row where truncation would show.
		Seed     string `json:"seed"`
		AsDict   string `json:"as_dict"`
		Describe string `json:"describe"`
	} `json:"cases"`
}

// loadDeals reads the frozen corpus.
//
// **`testdata/deals.json` was generated once, on 2026-09-12, and is frozen
// from that day forever.** It is a golden in the sense `internal/floats` and
// the simulator's recorded runs are goldens: it does not record what the code
// *ought* to do, it records what the code DID on the day somebody could still
// have changed their mind, and every seed handed out since rests on it. Never
// regenerate it. A failure here is a changed answer to a promise already
// made -- a reordered catalogue, an added ingredient, a different walk of the
// generator -- and the fix is to put the change back, not to re-record it.
func loadDeals(t *testing.T) potCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "deals.json"))
	if err != nil {
		t.Fatalf("reading the pot corpus: %v", err)
	}
	var c potCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decoding the pot corpus: %v", err)
	}
	if len(c.Cases) == 0 {
		t.Fatal("the pot corpus is empty")
	}
	return c
}

func bigSeed(t *testing.T, text string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(text, 10)
	if !ok {
		t.Fatalf("the corpus records %q as a seed and it is not a number", text)
	}
	return n
}

// TestASeedBrewsTheRecordedPot is the promise this package exists to keep.
//
// Compared as the SERIALISED payload, not field by field: this is what the
// browser renders, and a pot with the right three things in the wrong wire
// shape is still a broken reload. Chosen.MarshalJSON is what that comparison
// checks, and it exists because `tier1.Number` taught this repo that a type
// proved correct by every other means can still be wrong on the wire.
func TestASeedBrewsTheRecordedPot(t *testing.T) {
	t.Parallel()
	for _, tc := range loadDeals(t).Cases {
		got, err := json.Marshal(Deal(bigSeed(t, tc.Seed)))
		if err != nil {
			t.Fatalf("seed %s: marshalling: %v", tc.Seed, err)
		}
		if string(got) != tc.AsDict {
			t.Errorf("seed %s:\n got    %s\n golden %s", tc.Seed, got, tc.AsDict)
		}
	}
}

// TestTheWitchsProseIsTheRecordedProse covers Describe(), which is what
// actually reaches the model -- the wire shape above is the browser's half
// and this is the prompt's, and they are two different renderings of one
// pick.
func TestTheWitchsProseIsTheRecordedProse(t *testing.T) {
	t.Parallel()
	for _, tc := range loadDeals(t).Cases {
		if got := Deal(bigSeed(t, tc.Seed)).Describe(); got != tc.Describe {
			t.Errorf("seed %s describe:\n--- got ---\n%s\n--- golden ---\n%s",
				tc.Seed, got, tc.Describe)
		}
	}
}

// TestThePotsPlacesAreTheThemeInterviewsFirstThreeSlots pins the coupling
// whose failure is silent.
//
// [Order]'s three places ARE the theme interview's first three slot kinds,
// with `len(Order) == Floor`. An ingredient goes in *for* a slot, which is
// what lets the grounded-quote readiness of ADR 20 work untouched and keeps
// the querent's own words the only evidence -- an ingredient is not something
// they said. Drift, and nothing errors: the brew simply never comes ready.
//
// The three strings are written down HERE as literals rather than imported,
// for the same reason `tarot`'s equivalent test writes them down: this
// package must not depend on `internal/claude`, so the way to check that two
// lists agree is to keep two lists and check them.
func TestThePotsPlacesAreTheThemeInterviewsFirstThreeSlots(t *testing.T) {
	t.Parallel()
	want := []string{"taste", "temperament", "posture"}
	if len(Order) != len(want) {
		t.Fatalf("the pot has %d places, the floor is %d", len(Order), len(want))
	}
	for i, slot := range want {
		if Order[i].Slot != slot {
			t.Errorf("order[%d] is for %q, want %q", i, Order[i].Slot, slot)
		}
		if Order[i].Name == "" || Order[i].Asks == "" {
			t.Errorf("order[%d] has no name or no question", i)
		}
	}
}

// TestTheCatalogueIsAllOfIt guards the embed itself. A truncated data file
// would make every seeded pick above disagree, but it would disagree
// confusingly; this says what actually happened.
func TestTheCatalogueIsAllOfIt(t *testing.T) {
	t.Parallel()
	if len(Catalogue) != 24 {
		t.Errorf("the catalogue is %d ingredients, want 24 (eight per place)",
			len(Catalogue))
	}
	for _, place := range Order {
		shelf := ByCategory[place.Slot]
		if len(shelf) != 8 {
			t.Errorf("%q has %d ingredients, want 8", place.Slot, len(shelf))
		}
		// The shelf is in catalogue order, which is what the pick indexes
		// into. Checked against the catalogue itself rather than assumed, so
		// a loader that started grouping by map iteration would be caught.
		var walk int
		for _, in := range Catalogue {
			if in.Category != place.Slot {
				continue
			}
			if walk >= len(shelf) || shelf[walk].Key != in.Key {
				t.Fatalf("%q shelf is not in catalogue order at %d", place.Slot, walk)
			}
			walk++
		}
	}
	seen := map[string]bool{}
	for _, in := range Catalogue {
		if seen[in.Key] {
			t.Errorf("%s is in the catalogue twice", in.Key)
		}
		seen[in.Key] = true
		if in.Name == "" || in.Note == "" || in.Category == "" {
			t.Errorf("%s needs a name, a category and a note", in.Key)
		}
		if ByKey[in.Key].Name != in.Name {
			t.Errorf("%s does not index to itself", in.Key)
		}
	}
}

// TestEveryIngredientIsReachableFromSomeSeed is the test that would catch an
// off-by-one in the bound.
//
// `RandBelow(len(shelf)-1)` passes every golden above -- the corpus would
// have been regenerated from it and nothing in it names the last ingredient
// of a category by accident. What such a mutation actually does is make three
// ingredients, one per category, unreachable by ANY seed, forever, silently.
// So the property is asserted over a sweep instead: every ingredient in the
// catalogue is brewed by some seed under two thousand.
func TestEveryIngredientIsReachableFromSomeSeed(t *testing.T) {
	t.Parallel()
	reached := map[string]bool{}
	for seed := int64(0); seed < 2000; seed++ {
		pot := Deal(big.NewInt(seed))
		if len(pot.Ingredients) != len(Order) {
			t.Fatalf("seed %d filled %d places", seed, len(pot.Ingredients))
		}
		inPot := map[string]bool{}
		for i, c := range pot.Ingredients {
			// The place is the pot's, in the pot's order -- and the ingredient
			// really comes off that place's shelf, which is the whole of what
			// "one per category" means.
			if c.Position.Slot != Order[i].Slot {
				t.Fatalf("seed %d put %s in the %s place", seed, c.Ingredient.Key,
					c.Position.Slot)
			}
			if c.Ingredient.Category != c.Position.Slot {
				t.Fatalf("seed %d put a %s ingredient in the %s place", seed,
					c.Ingredient.Category, c.Position.Slot)
			}
			if inPot[c.Ingredient.Key] {
				t.Fatalf("seed %d put %s in twice", seed, c.Ingredient.Key)
			}
			inPot[c.Ingredient.Key] = true
			reached[c.Ingredient.Key] = true
		}
	}
	for _, in := range Catalogue {
		if !reached[in.Key] {
			t.Errorf("no seed under two thousand ever reaches %s; it is in the "+
				"catalogue in name only", in.Key)
		}
	}
}

// TestThePickIsUniformAndNotWeighted is the other half of that, and it is the
// one that would notice the deck's machinery being copied in here later.
//
// The contract this package advertises is that an ingredient is exactly as
// likely as its seven neighbours -- no weights, no floats. Over a fixed sweep
// that claim is a countable thing, so it is counted. Deterministic by
// construction: the seeds are 0 through 7999, so this test cannot be flaky,
// it can only be right or wrong forever.
func TestThePickIsUniformAndNotWeighted(t *testing.T) {
	t.Parallel()
	const sweep = 8000
	counts := map[string]int{}
	for seed := int64(0); seed < sweep; seed++ {
		for _, c := range Deal(big.NewInt(seed)).Ingredients {
			counts[c.Ingredient.Key]++
		}
	}
	for _, place := range Order {
		shelf := ByCategory[place.Slot]
		want := sweep / len(shelf)
		low, high := want*8/10, want*12/10
		for _, in := range shelf {
			if counts[in.Key] < low || counts[in.Key] > high {
				t.Errorf("%s landed %d times in %d pots, want near %d (%d..%d) "+
					"-- the pick is not uniform over its shelf",
					in.Key, counts[in.Key], sweep, want, low, high)
			}
		}
	}
}

// TestAnUnseededPickIsStillReproducibleFromItsOwnSeed covers the one path the
// corpus cannot: nobody holds a seed that has not been minted yet, so what
// must hold is that the answer carries the seed that reproduces it.
func TestAnUnseededPickIsStillReproducibleFromItsOwnSeed(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 32 {
		first := Deal(nil)
		if first.Seed.Sign() < 0 || first.Seed.Cmp(big.NewInt(1<<31)) >= 0 {
			t.Fatalf("minted seed %s is outside [0, 2**31)", first.Seed)
		}
		seen[first.Seed.String()] = true
		again := Deal(first.Seed)
		a, _ := json.Marshal(first)
		b, _ := json.Marshal(again)
		if string(a) != string(b) {
			t.Fatalf("seed %s did not re-brew itself:\n %s\n %s", first.Seed, a, b)
		}
	}
	if len(seen) < 30 {
		t.Errorf("32 unseeded picks produced only %d distinct seeds; the mint "+
			"is not drawing from entropy", len(seen))
	}
}

// TestTheSeedGrammarIsTheRecordedOne walks every string the corpus records,
// accepted and refused alike.
//
// Three of the accepted rows are 422s from a door written with
// strconv.ParseInt -- "  7  ", "+7" and above all "1_0", which is ten. Two of
// the refused rows are 200s from a door that took any Unicode decimal digit:
// the fullwidth "７" and the Arabic-Indic "٧". The recorded grammar sits
// between those two readings and matches neither library, so it is
// hand-written and this is what holds it there.
func TestTheSeedGrammarIsTheRecordedOne(t *testing.T) {
	t.Parallel()
	c := loadDeals(t)
	if len(c.SeedStrings) == 0 {
		t.Fatal("the corpus records no seed strings; the grammar is untested")
	}
	var accepted, refused int
	for _, row := range c.SeedStrings {
		got, ok := ParseSeed(row.Text)
		if ok != row.OK {
			t.Errorf("seed %q: got ok=%v, the corpus says ok=%v", row.Text, ok, row.OK)
			continue
		}
		if !ok {
			refused++
			continue
		}
		accepted++
		if got.String() != row.Value {
			t.Errorf("seed %q: got %s, the corpus says %s", row.Text, got, row.Value)
		}
	}
	// A corpus of all-accepts or all-refuses would pass against a parser that
	// answered one way for everything.
	if accepted == 0 || refused == 0 {
		t.Errorf("the corpus does not exercise both branches: %d accepted, "+
			"%d refused", accepted, refused)
	}
}

// TestTheGrammarIsTheDecksGrammarToTheLetter is what keeps a deliberate copy
// honest.
//
// `seed.go` argues why `tarot.ParseSeed` is copied rather than called: two
// props are two promises, and a pot whose accepted spellings were whatever
// the deck currently accepts would move the day the deck moved. That argument
// buys the right to diverge later; it does not buy the right to have diverged
// already by a typo. So the two are walked over one corpus and required to
// agree, today, character for character.
//
// The import is test-only on purpose -- nothing this package SERVES depends
// on `internal/tarot`, and a reader checking that can check it by grepping
// the non-test files.
func TestTheGrammarIsTheDecksGrammarToTheLetter(t *testing.T) {
	t.Parallel()
	for _, row := range loadDeals(t).SeedStrings {
		mine, okMine := ParseSeed(row.Text)
		theirs, okTheirs := tarot.ParseSeed(row.Text)
		if okMine != okTheirs {
			t.Errorf("%q: the pot says ok=%v and the deck says ok=%v",
				row.Text, okMine, okTheirs)
			continue
		}
		if okMine && mine.Cmp(theirs) != 0 {
			t.Errorf("%q: the pot reads %s and the deck reads %s",
				row.Text, mine, theirs)
		}
	}
}

// TestAnOversizedSeedIsNotTruncated is the row an int64 port gets wrong twice
// over: a different brew, returned under a different number.
func TestAnOversizedSeedIsNotTruncated(t *testing.T) {
	t.Parallel()
	huge, ok := ParseSeed("1180591620717411303424") // 2**70
	if !ok {
		t.Fatal("2**70 is a legal seed by the recorded grammar and must parse")
	}
	p := Deal(huge)
	if p.Seed.Cmp(huge) != 0 {
		t.Errorf("the pot came back under seed %s, not %s", p.Seed, huge)
	}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshalling an oversized seed: %v", err)
	}
	if !strings.Contains(string(body), `"seed":1180591620717411303424`) {
		t.Errorf("the seed did not survive the wire:\n%s", body)
	}
	// And it must actually fill the pot -- mt19937 seeds through `seedWords`,
	// whose key grows a word past 2**32 and again past 2**64.
	if len(p.Ingredients) != len(Order) {
		t.Errorf("an oversized seed filled %d places", len(p.Ingredients))
	}
}

// TestTheServedPayloadIsTheIngredientAndItsPlace pins what the wire carries
// and, just as deliberately, what it does not.
//
// `category` is absent: on the table it is the position's `slot`, and one
// question with two answers on the wire is how a client ends up rendering the
// disagreement. The field order is the recorded one, because the goldens
// above compare bytes.
func TestTheServedPayloadIsTheIngredientAndItsPlace(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(Deal(big.NewInt(7)).Ingredients[0])
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	for _, key := range []string{"key", "name", "note", "slot", "position"} {
		if _, ok := got[key]; !ok {
			t.Errorf("the served ingredient has no %q: %s", key, body)
		}
	}
	if _, leaked := got["category"]; leaked {
		t.Errorf("the wire carries `category` as well as `slot`: %s", body)
	}
	if len(got) != 5 {
		t.Errorf("the served ingredient has %d fields, want 5: %s", len(got), body)
	}
	if !strings.HasPrefix(string(body), `{"key":`) {
		t.Errorf("field order is not the recorded one: %s", body)
	}
}
