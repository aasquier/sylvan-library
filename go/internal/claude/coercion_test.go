package claude

import (
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"
)

// The two number readings and the canonical string form.
//
// These exist because **`strconv` is not this**, and every difference reaches
// a user. `1_0` is ten, a fullwidth `５` is five, `inf` and `nan` are words --
// and a hex float, which `strconv.ParseFloat` accepts, is not a number here.
// The refusal messages are recorded shapes: they land in a 422's `detail`,
// which somebody reads.
//
// The integer is a `*big.Int` for one concrete reason: a Forge seed is echoed
// back to whoever asked with it. Narrowing to int64 would answer a different
// number than the request named, and the caller would have no way to tell.
//
// The string form matters for a different reason: it is what the prompt cache
// keys on. Two payloads that mean the same thing must render the same bytes,
// or every request is a cache miss and the bill is several times what it
// should be.

// The seed is the case that makes the arbitrary precision load-bearing.
func TestAnIntegerBeyondInt64SurvivesIntact(t *testing.T) {
	t.Parallel()
	// Twenty digits: past int64's ceiling and past float64's exact range.
	const huge = "10000000000000000001"

	got, err := IntValue(json.Number(huge))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got.String() != huge {
		t.Errorf("read %s, want %s -- a seed is echoed back to whoever asked "+
			"with it, so a narrowed one answers a different number", got, huge)
	}

	// The same number as a string, which is how it arrives from a query
	// parameter rather than a body.
	got, err = IntValue(huge)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got.String() != huge {
		t.Errorf("the string form read as %s", got)
	}
}

// The integer grammar, which is Python's `int()` rather than `strconv.Atoi`.
func TestTheIntegerGrammarIsTheRecordedOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want int64
	}{
		{"a bare number", json.Number("42"), 42},
		{"a negative", json.Number("-42"), -42},
		{"an explicit plus", "+42", 42},
		{"a string", "42", 42},
		{"surrounding space", "  42  ", 42},
		// `1_0` is ten: underscores are separators between digits.
		{"an underscore separator", "1_0", 10},
		{"several separators", "1_000_000", 1000000},
		{"a negative with separators", "-1_000", -1000},
		// A float truncates toward zero rather than rounding.
		{"a float truncates", 3.9, 3},
		{"a negative float truncates toward zero", -3.9, -3},
		{"a JSON float", json.Number("3.9"), 3},
		// A bool is 0 or 1.
		{"true", true, 1},
		{"false", false, 0},
		{"a Go int", 42, 42},
		{"a Go int64", int64(42), 42},
		// A fullwidth digit reads, because the grammar takes any Unicode
		// decimal digit.
		{"a fullwidth digit", "４２", 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := IntValue(tc.in)
			if err != nil {
				t.Fatalf("%#v: %v", tc.in, err)
			}
			if got.Int64() != tc.want {
				t.Errorf("read %s, want %d", got, tc.want)
			}
		})
	}
}

// Underscores are separators and **only between digits** -- not leading, not
// trailing, not doubled, not next to the sign. Everything else that is not a
// number is refused rather than becoming zero.
func TestTheIntegerGrammarRefusesWhatIsNotANumber(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
	}{
		{"empty", ""},
		{"only space", "   "},
		{"a word", "forty-two"},
		{"a lone sign", "-"},
		{"a lone plus", "+"},
		{"a leading underscore", "_10"},
		{"a trailing underscore", "10_"},
		{"a doubled underscore", "1__0"},
		{"an underscore after the sign", "-_10"},
		{"an underscore before the sign", "_-10"},
		{"a bare underscore", "_"},
		{"trailing rubbish", "42abc"},
		{"a hex literal", "0x2a"},
		{"not a number at all", []any{42}},
		{"an object", map[string]any{"n": 42}},
		{"nothing", nil},
		{"NaN", math.NaN()},
		{"infinity", math.Inf(1)},
		{"negative infinity", math.Inf(-1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := IntValue(tc.in)
			if err == nil {
				t.Fatalf("%#v was read as %s", tc.in, got)
			}
			if got != nil {
				t.Errorf("a refusal also returned %s", got)
			}
		})
	}
}

// A `*big.Int` in is a copy out, so a caller cannot mutate the value it was
// handed and change what the request said.
func TestAnIntegerIsCopiedRatherThanAliased(t *testing.T) {
	t.Parallel()
	original := big.NewInt(42)
	got, err := IntValue(original)
	if err != nil {
		t.Fatal(err)
	}
	got.SetInt64(99)
	if original.Int64() != 42 {
		t.Error("the reading aliased its argument")
	}
}

// The float grammar takes the three bare words in any case, with an optional
// sign -- and refuses the hex float and the `p` exponent that `strconv`
// accepts.
func TestTheFloatGrammarTakesWordsAndRefusesStrconvsExtras(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   any
		want float64
	}{
		{"a number", json.Number("1.5"), 1.5},
		{"a string", "1.5", 1.5},
		{"an exponent", "1.5e3", 1500},
		{"a separator", "1_0.5", 10.5},
		{"a fullwidth digit", "５", 5},
		{"surrounding space", "  1.5  ", 1.5},
		{"true", true, 1},
		{"false", false, 0},
		{"a Go float", 1.5, 1.5},
		{"a Go int", 3, 3},
		{"a Go int64", int64(3), 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := FloatValue(tc.in)
			if err != nil {
				t.Fatalf("%#v: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("read %v, want %v", got, tc.want)
			}
		})
	}

	// The bare words, in any case and with either sign.
	for _, tc := range []struct {
		in   string
		want float64
	}{
		{"inf", math.Inf(1)},
		{"INF", math.Inf(1)},
		{"Infinity", math.Inf(1)},
		{"+inf", math.Inf(1)},
		{"-inf", math.Inf(-1)},
		{"-INFINITY", math.Inf(-1)},
	} {
		got, err := FloatValue(tc.in)
		if err != nil {
			t.Errorf("%q: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%q read as %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, word := range []string{"nan", "NaN", "-nan", "+NAN"} {
		got, err := FloatValue(word)
		if err != nil {
			t.Errorf("%q: %v", word, err)
			continue
		}
		if !math.IsNaN(got) {
			t.Errorf("%q read as %v", word, got)
		}
	}

	// What `strconv` takes and this does not.
	for _, tc := range []struct{ name, in string }{
		{"a hex float", "0x1p4"},
		{"a p exponent", "1p4"},
		{"a word", "one point five"},
		{"empty", ""},
		{"a lone sign", "-"},
		{"trailing rubbish", "1.5abc"},
		{"a leading underscore", "_1.5"},
	} {
		if got, err := FloatValue(tc.in); err == nil {
			t.Errorf("%s (%q) was read as %v", tc.name, tc.in, got)
		}
	}

	// The refusal is a recorded shape, because it lands in a 422's detail.
	_, err := FloatValue("one point five")
	if err == nil {
		t.Fatal("that parsed")
	}
	if !strings.Contains(err.Error(), "could not convert string to float") {
		t.Errorf("the refusal said %q", err)
	}
	if !strings.Contains(err.Error(), "'one point five'") {
		t.Errorf("the refusal does not quote the value: %q", err)
	}

	// A type that is not a number at all names the type it got.
	_, err = FloatValue([]any{1.5})
	if err == nil {
		t.Fatal("a list was read as a float")
	}
	if !strings.Contains(err.Error(), "float() argument must be") {
		t.Errorf("the refusal said %q", err)
	}
}

// **The string form is what the prompt cache keys on.** Two payloads that
// mean the same thing must render the same bytes, and a payload that renders
// differently between runs is a cache miss on every request -- several times
// the bill, silently.
func TestTheCanonicalStringFormIsStableAndASCII(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, in, want string
	}{
		{"plain", "Sol Ring", `"Sol Ring"`},
		{"a quote", `He said "hi"`, `"He said \"hi\""`},
		{"a backslash", `a\b`, `"a\\b"`},
		{"a newline", "a\nb", `"a\nb"`},
		{"a carriage return", "a\rb", `"a\rb"`},
		{"a tab", "a\tb", `"a\tb"`},
		{"a backspace", "a\bb", `"a\bb"`},
		{"a form feed", "a\fb", `"a\fb"`},
		{"a control character", "a\x01b", `"a\u0001b"`},
		// Everything above 0x7e is escaped too, so the whole form is ASCII.
		{"an accent", "Café", `"Caf\u00e9"`},
		{"a card name with an em dash", "A—B", `"A\u2014B"`},
		// Above the BMP: a UTF-16 surrogate pair, lower-case hex.
		{"an emoji", "\U0001F301", `"\ud83c\udf01"`},
		// These four are deliberately NOT escaped.
		{"a slash", "a/b", `"a/b"`},
		{"angle brackets", "<b>", `"<b>"`},
		{"an ampersand", "a&b", `"a&b"`},
		{"empty", "", `""`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			writeJSONString(&b, tc.in)
			got := b.String()
			if got != tc.want {
				t.Errorf("rendered %s, want %s", got, tc.want)
			}
			// Whatever went in, what comes out is ASCII -- the whole point
			// of the dialect.
			for i := 0; i < len(got); i++ {
				if got[i] > 0x7e {
					t.Errorf("%q rendered a byte above 0x7e: %s", tc.in, got)
					break
				}
			}
			// And it is still JSON, so a reader gets the original back.
			var back string
			if err := json.Unmarshal([]byte(got), &back); err != nil {
				t.Fatalf("%s is not JSON: %v", got, err)
			}
			if back != tc.in {
				t.Errorf("round-tripped to %q, want %q", back, tc.in)
			}
		})
	}
}

// A byte sequence that was never a string has no faithful answer, and the
// replacement character is a stable one -- stable being the property the
// cache needs.
func TestInvalidUTF8RendersAsAStableReplacement(t *testing.T) {
	t.Parallel()
	broken := string([]byte{0x41, 0xff, 0x42})

	var first, second strings.Builder
	writeJSONString(&first, broken)
	writeJSONString(&second, broken)
	if first.String() != second.String() {
		t.Fatalf("two renderings of the same bytes differ:\n%s\n%s", first.String(), second.String())
	}
	got := first.String()
	// Escaped, like everything else above 0x7e: U+FFFD is `\ufffd`.
	if !strings.Contains(got, `\ufffd`) {
		t.Errorf("invalid UTF-8 rendered as %s, want the replacement character", got)
	}
	if !json.Valid([]byte(got)) {
		t.Errorf("invalid UTF-8 rendered as invalid JSON: %s", got)
	}
}

// The hex is lower case, which is the recorded dialect -- upper-case hex is
// equally valid JSON and would be a different cache key for the same string.
func TestTheEscapeHexIsLowerCase(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeJSONString(&b, "ÿ—")
	got := b.String()
	if strings.ContainsAny(got, "ABCDEF") {
		t.Errorf("rendered upper-case hex: %s -- that is a different cache key "+
			"for the same string", got)
	}
	if !strings.Contains(got, `\u00ff`) || !strings.Contains(got, `\u2014`) {
		t.Errorf("rendered %s", got)
	}
}
