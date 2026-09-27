package wire_test

// The edges of the two renderings: what [wire.Quote] does with the characters
// a keyboard does not have, what [wire.Literal] does with the number types a
// hand-built body carries, and what the envelope does when a value will not
// encode at all.
//
// These matter because the sentences are read by a person. A rationale
// pasted from a word processor arrives full of characters nothing on a
// keyboard typed, and the refusal that quotes it back has to stay one line of
// readable text rather than becoming a wall of raw bytes or, worse, a
// half-written body.

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

func TestQuoteEscapesEveryCharacterAKeyboardDoesNotHave(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text", "Sol Ring", `'Sol Ring'`},
		{"a carriage return", "one\rtwo", `'one\rtwo'`},
		{"a tab and a newline", "one\t\ntwo", `'one\t\ntwo'`},
		{"a bell", "one\atwo", `'one\x07two'`},
		{"a letter beyond ASCII stays itself", "Fixture Ægis", `'Fixture Ægis'`},
		{"an unprintable in the basic plane", "a\u200bb", `'a\u200bb'`},
		{"an unprintable beyond it", "a\U000E0001b", `'a\U000e0001b'`},
		{"a quote inside switches the quoting", "it's", `"it's"`},
		{"both quotes keeps the single and escapes it", `it's a "thing"`, `'it\'s a "thing"'`},
		{"a backslash", `C:\x`, `'C:\\x'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := wire.Quote(tc.in); got != tc.want {
				t.Fatalf("Quote(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestLiteralRendersEveryNumberAHandBuiltBodyCarries(t *testing.T) {
	t.Parallel()
	// A decoded body brings json.Number; a body the API builds itself brings
	// Go's own types. Both cross the same wire, so both are rendered here,
	// and a float keeps its point (`1.0`, never `1`) because the recorded
	// payloads distinguish the two.
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"a decoded integer", json.Number("12"), "12"},
		{"a decoded float", json.Number("1.0"), "1.0"},
		{"a decoded exponent", json.Number("1e2"), "100.0"},
		{"a number too big to be a float", json.Number("1e999"), "1e999"},
		{"a Go float", 1.5, "1.5"},
		{"a Go float that is whole", float64(3), "3.0"},
		{"a Go int", 7, "7"},
		{"a Go int64", int64(-7), "-7"},
		{"nothing", nil, "None"},
		{"a list", []any{"x", 1}, `['x', 1]`},
		{"something the wire has no rule for", struct{ A int }{2}, "{2}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := wire.Literal(tc.in); got != tc.want {
				t.Fatalf("Literal(%#v) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestAValueThatWillNotEncodeIsAnEnvelopeRatherThanHalfABody(t *testing.T) {
	t.Parallel()
	// A channel is the stand-in for the real thing: a field somebody added to
	// a payload that encoding cannot express. The handler has already had its
	// chance to refuse, so what is left is to answer *something whole* — a
	// 500 with the envelope the client knows how to read, never a 200 with
	// the first half of an object in it.
	if _, err := wire.Marshal(make(chan int)); err == nil {
		t.Fatal("a channel encoded")
	}
	if _, err := wire.MarshalOrdered([]wire.KV{
		{Key: "ok", Value: 1}, {Key: "bad", Value: make(chan int)},
	}); err == nil {
		t.Fatal("an ordered body with an unencodable value came back whole")
	}

	rec := httptest.NewRecorder()
	wire.JSON(rec, http.StatusOK, map[string]any{"cards": make(chan int)})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Body.String(); got != `{"detail":"internal error"}` {
		t.Fatalf("body %q", got)
	}
	if got := rec.Header().Get("Content-Length"); got != "27" {
		t.Fatalf("content-length %q for a %d-byte body", got, rec.Body.Len())
	}
	// And the ordinary case still answers what it was given, so the above is
	// not passing because everything 500s.
	rec = httptest.NewRecorder()
	wire.JSON(rec, http.StatusOK, map[string]any{"n": math.MaxInt32})
	if rec.Code != http.StatusOK || rec.Body.String() != `{"n":2147483647}` {
		t.Fatalf("%d %q", rec.Code, rec.Body.String())
	}
}
