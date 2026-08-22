package mana

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The differential case set: `tests/mana_oracle.py`'s 13,944 (cost, pool)
// pairs, answered here and compared with Python's answers to each of them.
//
// This is the instrument PLAN section 5 item 2 is about, and it is older than
// the port -- `docs/ENGINEERING.md` section 1 built that oracle to be "usable
// as the differential case set for a compiled port… in any language, on any
// machine, forever". The shape of this file follows from taking that
// literally. **The case set is rebuilt here rather than read.** Go
// re-enumerates it from the same alphabets and the same limits, in the same
// order, and only the answers come out of the corpus; a port that replayed a
// dump of 13,944 rows would have proved its solver and left the sentence above
// unproven.
//
// Two digests, because a single one cannot say which half broke. The
// enumeration digest is over the case *names* alone and knows nothing about
// castability, so a failure there means the case set was rebuilt wrongly and
// the solver has not been tested at all. The answer digest is the project's
// own golden, the constant `CASES_ANSWER_DIGEST` in
// `tests/test_mana_properties.py`, and a failure there -- with the first one
// passing -- is castability and nothing else. That split is the draw corpus's
// lesson next door, applied again: record the generator apart from its
// consumers and a failure localises itself.

type castabilityCorpus struct {
	AnswersDigest     string   `json:"answers_digest"`
	EnumerationDigest string   `json:"enumeration_digest"`
	MaxPips           int      `json:"max_pips"`
	MaxGeneric        int      `json:"max_generic"`
	MaxSources        int      `json:"max_sources"`
	CasePips          []string `json:"case_pips"`
	CaseUnits         []string `json:"case_units"`
	Costs             []string `json:"costs"`
	Pools             []string `json:"pools"`
	Answers           []string `json:"answers"`
	Cases             int      `json:"cases"`
	Payable           int      `json:"payable"`
}

func loadCorpus(t *testing.T) castabilityCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/castability.json")
	if err != nil {
		t.Fatalf("reading the case set: %v (regenerate with "+
			"`python tests/go_fixtures.py`)", err)
	}
	var corpus castabilityCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parsing the case set: %v", err)
	}
	return corpus
}

// combinationsWithReplacement yields index tuples exactly as Python's
// `itertools.combinations_with_replacement` does -- non-decreasing, in
// lexicographic order, one empty tuple when r is 0, and nothing at all when
// the alphabet is empty and r is not.
//
// The order is not a detail. `all_cases()` promises the same cases in the same
// sequence forever, and the answer digest is taken over that sequence, so a
// correct set of cases in a different order fails exactly as loudly as a wrong
// one -- which is the intent.
func combinationsWithReplacement(n, r int) [][]int {
	if n == 0 && r > 0 {
		return nil
	}
	indices := make([]int, r)
	var out [][]int
	for {
		out = append(out, append([]int(nil), indices...))
		i := r - 1
		for ; i >= 0; i-- {
			if indices[i] != n-1 {
				break
			}
		}
		if i < 0 {
			return out
		}
		next := indices[i] + 1
		for j := i; j < r; j++ {
			indices[j] = next
		}
	}
}

// letters splits a colour set as the corpus writes it -- sorted and run
// together, so "UW" is the Azorius pip and "BGRUW" is any colour.
func letters(s string) []string { return strings.Split(s, "") }

// caseCosts is `mana_oracle.case_costs`: every cost of up to MaxPips pips
// drawn with replacement from the pip alphabet, at each generic count.
func caseCosts(corpus castabilityCorpus) []Cost {
	var out []Cost
	for size := 0; size <= corpus.MaxPips; size++ {
		for _, combo := range combinationsWithReplacement(len(corpus.CasePips), size) {
			pips := make([][]string, len(combo))
			for i, at := range combo {
				pips[i] = letters(corpus.CasePips[at])
			}
			for generic := 0; generic <= corpus.MaxGeneric; generic++ {
				out = append(out, Cost{Generic: generic, Pips: pips})
			}
		}
	}
	return out
}

// casePools is `mana_oracle.case_pools`: every pool of 1..MaxSources
// single-mana sources over the unit alphabet.
func casePools(corpus castabilityCorpus) [][]Source {
	var out [][]Source
	for size := 1; size <= corpus.MaxSources; size++ {
		for _, combo := range combinationsWithReplacement(len(corpus.CaseUnits), size) {
			pool := make([]Source, len(combo))
			for i, at := range combo {
				pool[i] = NewSource(letters(corpus.CaseUnits[at]), 1)
			}
			out = append(out, pool)
		}
	}
	return out
}

// poolString is the pool half of `mana_oracle.case_id`: each source's colours
// sorted and run together, an `xN` suffix when it makes more than one mana,
// space separated.
func poolString(pool []Source) string {
	parts := make([]string, len(pool))
	for i, source := range pool {
		colors := append([]string(nil), source.Colors...)
		sort.Strings(colors)
		parts[i] = strings.Join(colors, "")
		if source.Amount != 1 {
			parts[i] += "x" + strconv.Itoa(source.Amount)
		}
	}
	return strings.Join(parts, " ")
}

// caseID is `mana_oracle.case_id`: a stable one-line name for a case.
func caseID(cost Cost, pool []Source) string {
	return cost.String() + " <- [" + poolString(pool) + "]"
}

func TestTheCaseSetIsRebuiltExactlyAsPythonEnumeratesIt(t *testing.T) {
	corpus := loadCorpus(t)
	costs, pools := caseCosts(corpus), casePools(corpus)

	if len(costs) != len(corpus.Costs) {
		t.Fatalf("rebuilt %d costs, Python enumerated %d",
			len(costs), len(corpus.Costs))
	}
	for i, cost := range costs {
		if got := cost.String(); got != corpus.Costs[i] {
			t.Fatalf("cost %d is %q, Python's is %q", i, got, corpus.Costs[i])
		}
	}
	if len(pools) != len(corpus.Pools) {
		t.Fatalf("rebuilt %d pools, Python enumerated %d",
			len(pools), len(corpus.Pools))
	}
	for i, pool := range pools {
		if got := poolString(pool); got != corpus.Pools[i] {
			t.Fatalf("pool %d is %q, Python's is %q", i, got, corpus.Pools[i])
		}
	}
	if got := len(costs) * len(pools); got != corpus.Cases {
		t.Fatalf("the cross product is %d cases, Python counted %d",
			got, corpus.Cases)
	}

	digest := sha256.New()
	for _, cost := range costs {
		for _, pool := range pools {
			digest.Write([]byte(caseID(cost, pool) + "\n"))
		}
	}
	if got := hex.EncodeToString(digest.Sum(nil)); got != corpus.EnumerationDigest {
		t.Fatalf("the rebuilt case set hashes to %s, Python's to %s -- the "+
			"enumeration differs, so nothing below has tested the solver",
			got, corpus.EnumerationDigest)
	}
}

func TestCanPayAnswersEveryCaseAsPythonDoes(t *testing.T) {
	corpus := loadCorpus(t)
	costs, pools := caseCosts(corpus), casePools(corpus)
	if len(costs) != len(corpus.Answers) {
		t.Fatalf("%d costs against %d rows of answers",
			len(costs), len(corpus.Answers))
	}

	var disagreements []string
	payable := 0
	digest := sha256.New()
	for i, cost := range costs {
		row := corpus.Answers[i]
		if len(row) != len(pools) {
			t.Fatalf("answer row %d has %d columns, want %d",
				i, len(row), len(pools))
		}
		for j, pool := range pools {
			got := CanPay(cost, pool, 0)
			if got {
				payable++
			}
			want := row[j] == '1'
			if got != want {
				disagreements = append(disagreements,
					caseID(cost, pool)+": Go says "+yesno(got)+
						", Python says "+yesno(want))
			}
			digest.Write([]byte(caseID(cost, pool) + "=" + bit(got) + "\n"))
		}
	}

	if len(disagreements) > 0 {
		shown := disagreements
		if len(shown) > 10 {
			shown = shown[:10]
		}
		t.Fatalf("%d of %d cases disagree with Python. First %d:\n  %s",
			len(disagreements), corpus.Cases, len(shown),
			strings.Join(shown, "\n  "))
	}
	if payable != corpus.Payable {
		t.Errorf("%d cases are payable, Python counted %d", payable, corpus.Payable)
	}
	if got := hex.EncodeToString(digest.Sum(nil)); got != corpus.AnswersDigest {
		t.Fatalf("the answer set hashes to %s, the pinned golden is %s",
			got, corpus.AnswersDigest)
	}
}

// The two references answer the same 13,944 cases, so the agreement Python
// records between its three implementations is re-established here between
// Go's three rather than inherited from the corpus.
//
// This is the check that would survive the corpus being deleted, and it is the
// one that catches a shared misreading: if Go's solver and Python's had both
// been wrong in the same way, the test above would pass and this one would
// not.
func TestTheOraclesAgreeWithTheSolverAcrossTheWholeCaseSet(t *testing.T) {
	corpus := loadCorpus(t)
	costs, pools := caseCosts(corpus), casePools(corpus)

	var brute, hall []string
	for _, cost := range costs {
		for _, pool := range pools {
			got := CanPay(cost, pool, 0)
			if bruteForceCanPay(cost, pool, 0) != got {
				brute = append(brute, caseID(cost, pool))
			}
			if hallCanPay(cost, pool, 0) != got {
				hall = append(hall, caseID(cost, pool))
			}
		}
	}
	if len(brute) > 0 {
		t.Errorf("%d case(s) disagree with the brute-force search, first: %s",
			len(brute), brute[0])
	}
	if len(hall) > 0 {
		t.Errorf("%d case(s) disagree with Hall's condition, first: %s",
			len(hall), hall[0])
	}
}

// The two pieces the 13,944 cases cannot judge, judged.
//
// The enumerated set is a strong instrument and it has a narrow waist: **every
// pool in it is a single-mana source.** `case_pools` builds `ManaSource(c)` and
// nothing else, so all 13,944 cases together say nothing whatever about Sol
// Ring -- the amounts that matter, 0 and 2 and negative, are reachable only
// here and in the fuzzer. That is a second, smaller instance of the blind spot
// this package's fuzz target was written about, and it is worth stating twice:
// a case count is not coverage.
//
// So [ExpandUnits] is held to `oracleUnits`, the oracle's own expansion, over
// exactly those amounts -- the same independence `mana_oracle.py` buys by
// reimplementing `_units` rather than importing `expand_units`.
//
// And `colorSet.intersects` is held to `oracleOverlap`: the same question
// asked of packed bits and of strings. The solver compares six-bit masks and
// the oracles compare colour names, so this is the one test that says the
// packing is faithful rather than merely plausible -- and it is what lets the
// oracles judge the solver at all, since a shared representation would have
// made their agreement circular.
func TestTheSolversRepresentationsAgreeWithTheOraclesOwn(t *testing.T) {
	sets := [][]string{
		nil, {}, {"W"}, {"U"}, {"C"}, {"U", "W"}, {"B", "G", "R", "U", "W"},
		{"W", "W"}, {"Z"}, {"W", "Z"},
	}

	for _, colors := range sets {
		for _, amount := range []int{-3, -1, 0, 1, 2, 5} {
			source := Source{Colors: colors, Amount: amount}
			got := ExpandUnits([]Source{source})
			want := oracleUnits([]Source{source})
			if len(got) != len(want) {
				t.Fatalf("ExpandUnits(%v x%d) made %d units, the oracle made %d",
					colors, amount, len(got), len(want))
			}
			for i := range got {
				if !sameColors(got[i], want[i]) {
					t.Fatalf("ExpandUnits(%v x%d) unit %d is %v, the oracle's is %v",
						colors, amount, i, got[i], want[i])
				}
			}
			if n := unitCount([]Source{source}); n != len(want) {
				t.Errorf("unitCount(%v x%d) = %d, but the expansion is %d long",
					colors, amount, n, len(want))
			}
		}
	}

	// A mixed pool, because the per-source loop above cannot see an error in
	// how they are joined -- a pool whose first source makes nothing.
	mixed := []Source{
		{Colors: []string{"W"}, Amount: 0},
		{Colors: []string{"C"}, Amount: 2},
		{Colors: []string{"G"}, Amount: -1},
		NewSource([]string{"U", "W"}, 1),
	}
	if got, want := len(ExpandUnits(mixed)), len(oracleUnits(mixed)); got != want {
		t.Errorf("a mixed pool expands to %d units, the oracle says %d", got, want)
	}
	if n := unitCount(mixed); n != 3 {
		t.Errorf("unitCount of the mixed pool is %d, want 3", n)
	}

	for _, unit := range sets {
		for _, pip := range sets {
			packed := toColorSet(unit).intersects(toColorSet(pip))
			if plain := oracleOverlap(unit, pip); packed != plain {
				t.Errorf("%v vs %v: the packed colours say %v, the strings say %v",
					unit, pip, packed, plain)
			}
		}
	}
}

// sameColors is set equality over two colour lists, for failure messages
// above. It is the test's own, not the solver's -- comparing units with the
// thing under test would be the circularity this whole file avoids.
func sameColors(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func bit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func yesno(b bool) string {
	if b {
		return "payable"
	}
	return "not payable"
}
