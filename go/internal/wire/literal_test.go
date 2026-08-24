package wire_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The plain and literal renderings over a JSON-decoded value, held to a
// recorded corpus.
//
// The cases are JSON **documents** rather than value trees, so the test
// decodes the very bytes the corpus's renderings were recorded from — the
// only arrangement that checks the coercion rather than two hand-written
// trees that happen to agree. Decoded with `UseNumber`, as every route body
// is, so `1.0` is still `1.0` and a huge integer still has all its digits.

type plainCase struct {
	Note     string `json:"note"`
	Document string `json:"document"`
	Str      string `json:"str"`
	Repr     string `json:"repr"`
	// GoSortsTo is set on the one shape the renderer cannot reproduce: a
	// JSON object decodes to a map, whose iteration order is randomised, so
	// [wire.Literal] sorts the keys where the recorded rendering keeps the
	// document's. The corpus records what is answered *instead*, so the
	// limit is asserted rather than omitted — and the day bodies are
	// decoded through an ordered map, this is the row that says so.
	GoSortsTo string `json:"go_sorts_to"`
}

func loadPlain(t *testing.T) []plainCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "pystr.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []plainCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("the plain/literal corpus is empty; testdata/pystr.json is a frozen golden")
	}
	return cases
}

func TestPlainAndQuoteMatchTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	for _, c := range loadPlain(t) {
		t.Run(c.Note, func(t *testing.T) {
			decoder := json.NewDecoder(bytes.NewReader([]byte(c.Document)))
			decoder.UseNumber()
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatalf("the corpus document did not decode: %v", err)
			}
			want, wantRepr := c.Str, c.Repr
			if c.GoSortsTo != "" {
				// The known limit. Asserting the document-order rendering
				// here would be asserting a lie; asserting nothing would let
				// the gap widen unseen. So the corpus carries both and this
				// checks the one actually given.
				want, wantRepr = c.GoSortsTo, c.GoSortsTo
				if c.Str == c.GoSortsTo {
					t.Fatal("the corpus row no longer separates the two orders")
				}
			}
			if got := wire.Plain(value); got != want {
				t.Errorf("Plain(%s) = %q, want %q", c.Document, got, want)
			}
			if got := wire.Literal(value); got != wantRepr {
				t.Errorf("Literal(%s) = %q, want %q", c.Document, got, wantRepr)
			}
		})
	}
}

// TestAListSlugRendersAsTheCorpusRecords is the case that started it: a body
// field that is a list reaches a 404's `detail`, which the deck page renders
// verbatim, and `fmt.Sprint` writes a different sentence from the recorded
// one.
func TestAListSlugRendersAsTheCorpusRecords(t *testing.T) {
	t.Parallel()
	if got := wire.Plain([]any{"x"}); got != "['x']" {
		t.Errorf("a list slug rendered %q, want %q", got, "['x']")
	}
	// And the difference is real rather than theoretical: this is exactly
	// what the shared helper used to produce.
	if got := wire.Plain([]any{"x"}); got == "[x]" {
		t.Error("the helper is still using Go's own rendering")
	}
}
