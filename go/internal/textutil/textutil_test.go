package textutil_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// CPython's whitespace and line-boundary tables, held to a real interpreter.
//
// The corpus sweeps **the whole of Unicode** rather than the code points where
// Go and Python are known to differ, for the reason `pycasefold`'s table
// gives: recording only the disagreements would leave the rest resting on Go's
// `unicode` tables agreeing with CPython's, which is a claim about two
// projects' Unicode versions and not one this port gets to make.
//
// It is **identical under 3.11 and 3.12**, verified by rendering under each and
// diffing — the check CLAUDE.md requires of any sweep over a character
// property, after the theme lane found two tables that failed together for two
// entirely different reasons.

type textCorpus struct {
	SpaceRanges [][2]int `json:"space_ranges"`
	BreakRanges [][2]int `json:"break_ranges"`
	Strips      []struct {
		Note      string `json:"note"`
		Text      string `json:"text"`
		Stripped  string `json:"stripped"`
		RStripped string `json:"rstripped"`
		SplitJoin string `json:"split_join"`
	} `json:"strips"`
	Splits []struct {
		Note  string   `json:"note"`
		Text  string   `json:"text"`
		Lines []string `json:"lines"`
	} `json:"splits"`
	Heads []struct {
		Note string `json:"note"`
		Text string `json:"text"`
		N    int    `json:"n"`
		Len  int    `json:"len"`
		Head string `json:"head"`
	} `json:"heads"`
}

func load(t *testing.T) textCorpus {
	t.Helper()
	var corpus textCorpus
	raw, err := os.ReadFile(filepath.Join("testdata", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.SpaceRanges) == 0 {
		t.Fatal("the pytext corpus is empty; run tests/go_fixtures.py")
	}
	return corpus
}

func inRanges(ranges [][2]int, r rune) bool {
	for _, span := range ranges {
		if int(r) >= span[0] && int(r) <= span[1] {
			return true
		}
	}
	return false
}

// isCharacter skips the surrogates, which are not characters and which Go
// cannot put in a string anyway.
func isCharacter(r rune) bool { return r < 0xD800 || r > 0xDFFF }

// TestIsSpaceIsStrIsSpaceForEveryCodePoint sweeps all 1,114,112 of them.
func TestIsSpaceIsStrIsSpaceForEveryCodePoint(t *testing.T) {
	corpus := load(t)
	wrong := 0
	for r := rune(0); r <= 0x10FFFF; r++ {
		if !isCharacter(r) {
			continue
		}
		want := inRanges(corpus.SpaceRanges, r)
		if got := textutil.IsSpace(r); got != want {
			if wrong < 10 {
				t.Errorf("IsSpace(U+%04X) = %v, CPython says %v", r, got, want)
			}
			wrong++
		}
	}
	if wrong > 10 {
		t.Errorf("... and %d more", wrong-10)
	}
}

// TestGoAndPythonReallyDisagreeAboutWhitespace is the reason this package
// exists, stated as a test rather than as a comment.
//
// If `unicode.IsSpace` were CPython's set, `strings.TrimSpace` would do and
// none of this would be here. It is not: the four information separators
// U+001C..U+001F are whitespace to `str.strip()` and not to Go. A test that
// only checked agreement would go green the day somebody replaced this package
// with the standard library.
func TestGoAndPythonReallyDisagreeAboutWhitespace(t *testing.T) {
	var differ []rune
	for r := rune(0); r <= 0x10FFFF; r++ {
		if !isCharacter(r) {
			continue
		}
		if textutil.IsSpace(r) != unicode.IsSpace(r) {
			differ = append(differ, r)
		}
	}
	want := []rune{0x1c, 0x1d, 0x1e, 0x1f}
	if len(differ) != len(want) {
		t.Fatalf("Go and CPython differ on %d code points (%U), want exactly %U",
			len(differ), differ, want)
	}
	for i, r := range want {
		if differ[i] != r {
			t.Errorf("difference %d is U+%04X, want U+%04X", i, differ[i], r)
		}
	}
}

// TestSplitLinesAgreesWithPython holds `str.splitlines()` case for case.
func TestSplitLinesAgreesWithPython(t *testing.T) {
	corpus := load(t)
	for _, c := range corpus.Splits {
		t.Run(c.Note, func(t *testing.T) {
			got := textutil.SplitLines(c.Text)
			if len(got) != len(c.Lines) {
				t.Fatalf("split %q into %q, want %q", c.Text, got, c.Lines)
			}
			for i := range got {
				if got[i] != c.Lines[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], c.Lines[i])
				}
			}
		})
	}
}

// TestEveryLineBoundaryIsOneCPythonKnows sweeps the boundary table, which is a
// DIFFERENT set from the whitespace one — and the near-miss this package was
// written for: U+001C..U+001E are both, **U+001F is whitespace and not a
// boundary**, and U+2028/U+2029 are both.
func TestEveryLineBoundaryIsOneCPythonKnows(t *testing.T) {
	corpus := load(t)
	for r := rune(0); r <= 0x10FFFF; r++ {
		if !isCharacter(r) {
			continue
		}
		want := inRanges(corpus.BreakRanges, r)
		got := len(textutil.SplitLines("a"+string(r)+"b")) == 2
		if got != want {
			t.Fatalf("U+%04X splits = %v, CPython says %v", r, got, want)
		}
	}
}

// TestUnitSeparatorIsWhitespaceButNotABoundary states the near-miss on its
// own, because it is the single fact most likely to be got wrong by somebody
// deriving one table from the other.
func TestUnitSeparatorIsWhitespaceButNotABoundary(t *testing.T) {
	if !textutil.IsSpace(0x1f) {
		t.Error("U+001F must be whitespace to str.strip()")
	}
	if got := textutil.SplitLines("a\x1fb"); len(got) != 1 {
		t.Errorf("U+001F split a line into %q; it is not a boundary", got)
	}
	for _, r := range []rune{0x1c, 0x1d, 0x1e} {
		if !textutil.IsSpace(r) {
			t.Errorf("U+%04X must be whitespace", r)
		}
		if got := textutil.SplitLines("a" + string(r) + "b"); len(got) != 2 {
			t.Errorf("U+%04X must be a line boundary; got %q", r, got)
		}
	}
}

// TestStripAndFriendsAgreeWithPython holds the three trimmers.
func TestStripAndFriendsAgreeWithPython(t *testing.T) {
	corpus := load(t)
	for _, c := range corpus.Strips {
		t.Run(c.Note, func(t *testing.T) {
			if got := textutil.Strip(c.Text); got != c.Stripped {
				t.Errorf("Strip(%q) = %q, want %q", c.Text, got, c.Stripped)
			}
			if got := textutil.RStrip(c.Text); got != c.RStripped {
				t.Errorf("RStrip(%q) = %q, want %q", c.Text, got, c.RStripped)
			}
			if got := textutil.SplitJoin(c.Text); got != c.SplitJoin {
				t.Errorf("SplitJoin(%q) = %q, want %q", c.Text, got, c.SplitJoin)
			}
		})
	}
}

// TestLenAndHeadCountCodePoints: `len(s)` and `s[:n]` are code points, not
// bytes, and a question of 2,000 accented characters is 2,000 to Python and
// well over any byte ceiling.
func TestLenAndHeadCountCodePoints(t *testing.T) {
	corpus := load(t)
	for _, c := range corpus.Heads {
		t.Run(c.Note, func(t *testing.T) {
			if got := textutil.Len(c.Text); got != c.Len {
				t.Errorf("Len(%q) = %d, want %d", c.Text, got, c.Len)
			}
			if got := textutil.Head(c.Text, c.N); got != c.Head {
				t.Errorf("Head(%q, %d) = %q, want %q", c.Text, c.N, got, c.Head)
			}
		})
	}
}

// TestIsoformatElidesTheFractionAtAZeroMicrosecond is the behaviour the job
// corpus found and two older writers in this module still miss: Python's
// `isoformat()` drops the fractional part entirely when the microsecond is
// zero, and a fixed six-digit layout does not.
func TestIsoformatElidesTheFractionAtAZeroMicrosecond(t *testing.T) {
	whole := time.Date(2026, 8, 23, 1, 23, 45, 0, time.UTC)
	if got := textutil.Isoformat(whole); got != "2026-08-23T01:23:45+00:00" {
		t.Errorf("Isoformat at a zero microsecond = %q", got)
	}
	fraction := time.Date(2026, 8, 23, 1, 23, 45, 678901000, time.UTC)
	if got := textutil.Isoformat(fraction); got != "2026-08-23T01:23:45.678901+00:00" {
		t.Errorf("Isoformat with microseconds = %q", got)
	}
	// Nanoseconds are truncated, not rounded — `datetime` has no finer unit.
	sub := time.Date(2026, 8, 23, 1, 23, 45, 678901999, time.UTC)
	if got := textutil.Isoformat(sub); got != "2026-08-23T01:23:45.678901+00:00" {
		t.Errorf("Isoformat truncating nanoseconds = %q", got)
	}
	// And the offset is `+00:00`, never `Z`: it is a sort key as TEXT in more
	// than one table here.
	if got := textutil.Isoformat(whole); got[len(got)-6:] != "+00:00" {
		t.Errorf("Isoformat ended %q, want +00:00", got[len(got)-6:])
	}
}
