package claude

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// `str.casefold()`, and the fourth reproduction of somebody else's
// arithmetic in this port after `pyrand`, `pyyaml` and `pyfloat`.
//
// **`strings.ToLower` is not this function.** Casefolding is Unicode *full*
// case folding, which lowercasing is not: `ß` folds to `ss`, `ſ` to `s`, `ς`
// to `σ`, `ﬁ` to `fi`. 211 code points disagree with `str.lower()` and 104
// of them fold to more than one character, so no rune-to-rune mapping can
// express it.
//
// It earns its keep at exactly one place and that place is load-bearing:
// `theme.Ground` folds the user's own turns and the quote the model claims
// they said, then asks whether one contains the other. A slot that grounds
// in Python and drops here is a readiness count that will not move, which
// is commandment 2's failure -- the newcomer concludes they are answering
// wrong. The other callers (`KeepFact`'s `tarot:` prefix, a fact id, a
// commander's name against the pool) are the same substring test one layer
// out.
//
// **The table is the whole of Unicode, not the disagreements.** Recording
// only the 211 differences would leave the rest resting on Go's `unicode`
// tables agreeing with CPython's, which is a claim about two projects'
// Unicode versions rather than one this port gets to make. `casefold.json`
// is swept out of a real interpreter by `tests/go_fixtures.py`; a rune
// absent from it folds to itself. The committed file held equal to a fresh
// sweep is the oracle, so there is no second corpus -- and CI renders it
// under 3.11 and 3.12, which is where a Unicode version moving under the
// project would surface.
//
// The neighbouring modes still spell this `strings.ToLower` (`argue`'s
// in-deck set, `interview`'s card match, `research`'s in-flight key, whose
// gap is named by a test of its own). Those fold a card name against the
// pool's own spelling of it, where both sides are the same string and the
// two functions cannot disagree; switching them is a behaviour change to
// already-flipped routes and wants its own corpus, not a ride-along here.

//go:embed data/casefold.json
var casefoldFile []byte

// pyFolds is every code point whose fold is not itself. Multi-character
// folds are why the value is a string.
var pyFolds map[rune]string

func init() {
	var payload struct {
		Folds []struct {
			CP   rune   `json:"cp"`
			Fold string `json:"fold"`
		} `json:"folds"`
	}
	if err := json.Unmarshal(casefoldFile, &payload); err != nil {
		panic(fmt.Sprintf("claude: casefold.json will not parse: %v", err))
	}
	pyFolds = make(map[rune]string, len(payload.Folds))
	for _, f := range payload.Folds {
		pyFolds[f.CP] = f.Fold
	}
}

// pyCasefold is `str.casefold()`.
//
// ASCII is answered without touching the table, which is what the whole of
// a transcript is in the ordinary case: `A`-`Z` shift by 32 and every other
// byte below 0x80 folds to itself. Both facts are in the table too, so the
// fast path is an optimisation and never a second definition -- a test
// walks all 128 through both.
// PyCasefold is `str.casefold()`, exported for `internal/api`: the argue
// sweep folds a NAME SOMEBODY TYPED against the deck's own spelling, which is
// two different strings and therefore the one place in this family where
// `casefold` and `lower` can genuinely disagree.
func PyCasefold(s string) string { return pyCasefold(s) }

func pyCasefold(s string) string {
	if ascii, done := pyCasefoldASCII(s); done {
		return ascii
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if folded, ok := pyFolds[r]; ok {
			b.WriteString(folded)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// pyCasefoldASCII folds a string that is entirely ASCII, and reports whether
// it was one. Separate so the test that proves the fast path agrees with the
// table can drive it directly.
func pyCasefoldASCII(s string) (string, bool) {
	upper := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			return "", false
		}
		if 'A' <= c && c <= 'Z' {
			upper = true
		}
	}
	if !upper {
		return s, true
	}
	out := []byte(s)
	for i, c := range out {
		if 'A' <= c && c <= 'Z' {
			out[i] = c + 32
		}
	}
	return string(out), true
}
