package claude

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Python's `int()` over a JSON-decoded value, for the theme interview's
// reading seed.
//
// **This is not the tarot route's seed parser and must not become it.**
// `/api/tarot/reading` takes its seed from a query string through FastAPI's
// `seed: int | None`, which is *Pydantic's* integer grammar -- measured
// against the running app in the tarot lane, and it refuses the fullwidth
// `７` that Python's own `int()` reads as seven. `theme._reading_for` spells
// it `int(seed)`. Two different functions on two different doors, and the
// difference is observable: a client that puts `"１０"` in a JSON body gets
// reading ten from Python.
//
// What `int()` accepts, and what each does here:
//
//   - an int -- itself, unbounded, so a `*big.Int`;
//   - a float -- **truncated toward zero**, so 5.9 is five, and NaN or an
//     infinity raises (`ValueError` / `OverflowError`, both caught by the
//     same handler in Python and both a refusal here);
//   - a bool -- one or zero, because `bool` is an `int` in Python;
//   - a string -- optional surrounding whitespace, an optional sign, and
//     digits with single underscores between them, where a "digit" is **any
//     Unicode decimal digit** and not only ASCII;
//   - anything else -- a `TypeError`, which is the same refusal.

//go:embed data/digits.json
var digitsFile []byte

// digitZeros is the first code point of every Unicode decimal-digit run. Each
// run is exactly ten long and zero-based, so a digit's value is its distance
// from the greatest start not above it.
//
// A table rather than a walk-down from the code point, because the walk-down
// is **wrong for 36 code points**: the mathematical digit blocks from U+1D7CE
// are four runs of ten butted together with no gap, so walking back off a
// bold `4` lands in the previous run and reads it as fourteen.
var digitZeros []rune

func init() {
	var payload struct {
		Zeros []rune `json:"zeros"`
	}
	if err := json.Unmarshal(digitsFile, &payload); err != nil {
		panic(fmt.Sprintf("claude: digits.json will not parse: %v", err))
	}
	digitZeros = payload.Zeros
}

// pyDigitValue is the decimal value of one Unicode digit, and whether it is
// one at all.
func pyDigitValue(r rune) (int, bool) {
	if r >= '0' && r <= '9' {
		return int(r - '0'), true
	}
	if !unicode.IsDigit(r) {
		return 0, false
	}
	return digitValueIn(digitZeros, r)
}

// digitValueIn is the table lookup, taking its table as an argument.
//
// **Split out so its guard can be reached at all.** Every code point Go calls
// a digit today sits inside a run this table knows, so `d < 10` never fires
// through `pyDigitValue` and a mutation dropping it survives every sweep of
// Unicode there is -- which is what "equivalent over the reachable input"
// looks like rather than a gap in the sweep. The case the guard is for cannot
// be reached from outside: **a rune Go's `unicode` tables call a digit and
// this table, swept from CPython, has never heard of**, which is a Unicode
// version moving under one runtime and not the other. Then the distance from
// the last known run start is arbitrary, and answering it would be a
// confident wrong digit where refusing is a refusal to guess. Handing the
// table in is how a test says that out loud.
func digitValueIn(zeros []rune, r rune) (int, bool) {
	i := sort.Search(len(zeros), func(i int) bool { return zeros[i] > r })
	if i == 0 {
		return 0, false
	}
	if d := r - zeros[i-1]; d >= 0 && d < 10 {
		return int(d), true
	}
	return 0, false
}

// asciiDigits rewrites a Unicode digit's value as its ASCII spelling. An
// index rather than `byte('0' + value)`, so the range that makes the
// conversion safe is the string's own length rather than a claim about
// `pyDigitValue` made somewhere else.
const asciiDigits = "0123456789"

// errNotAnInt stands for every way `int()` refuses, since Python catches
// `TypeError` and `ValueError` in one handler and says the same sentence for
// both.
var errNotAnInt = fmt.Errorf("not an integer")

// PyInt is `int(v)` over a JSON-decoded value, exported beside [PyFloat] and
// for the same reason it is: a second family needs Python's own integer
// grammar. The theme proposal's budget was the first; `POST /api/sim/forge`'s
// games count and seed are the second, and `strconv.Atoi` is not this — `1_0`
// is ten, a float truncates, a bool is 0 or 1, and a fullwidth digit reads.
//
// A `*big.Int` because Python's integers are unbounded and a Forge seed is
// echoed back to the caller: narrowing here would answer a different number
// than the one somebody asked with.
func PyInt(v any) (*big.Int, error) { return pyInt(v) }

// pyInt is `int(v)` over a JSON-decoded value.
func pyInt(v any) (*big.Int, error) {
	switch value := v.(type) {
	case bool:
		if value {
			return big.NewInt(1), nil
		}
		return big.NewInt(0), nil
	case string:
		return pyIntFromString(value)
	case json.Number:
		// The body is decoded with UseNumber, so an integer literal is still
		// exact here -- `int(10000000000000000001)` must not lose its last
		// digit on the way through a float64.
		if n, ok := new(big.Int).SetString(value.String(), 10); ok {
			return n, nil
		}
		f, err := value.Float64()
		if err != nil {
			return nil, errNotAnInt
		}
		return pyIntFromFloat(f)
	case float64:
		return pyIntFromFloat(value)
	case int:
		return big.NewInt(int64(value)), nil
	case int64:
		return big.NewInt(value), nil
	case *big.Int:
		return new(big.Int).Set(value), nil
	default:
		return nil, errNotAnInt
	}
}

// pyIntFromFloat is `int(f)`: truncation toward zero, and a refusal for a
// value that has no integer at all.
func pyIntFromFloat(f float64) (*big.Int, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return nil, errNotAnInt
	}
	n, _ := big.NewFloat(f).Int(nil)
	return n, nil
}

// pyIntFromString is `int(s)` for a `str`: Python's integer literal grammar.
//
// Underscores are separators and only between digits -- not leading, not
// trailing, not doubled, not next to the sign -- which is the same rule the
// tarot route's scanner applies to a query value. Everything else about the
// two differs.
func pyIntFromString(s string) (*big.Int, error) {
	body := strings.TrimFunc(s, pyIsSpace)
	if body == "" {
		return nil, errNotAnInt
	}
	negative := false
	switch body[0] {
	case '+':
		body = body[1:]
	case '-':
		negative = true
		body = body[1:]
	}
	if body == "" {
		return nil, errNotAnInt
	}
	var digits strings.Builder
	runes := []rune(body)
	for i, r := range runes {
		if r == '_' {
			if i == 0 || i == len(runes)-1 {
				return nil, errNotAnInt
			}
			if _, ok := pyDigitValue(runes[i-1]); !ok {
				return nil, errNotAnInt
			}
			if _, ok := pyDigitValue(runes[i+1]); !ok {
				return nil, errNotAnInt
			}
			continue
		}
		value, ok := pyDigitValue(r)
		if !ok {
			return nil, errNotAnInt
		}
		digits.WriteByte(asciiDigits[value])
	}
	out, ok := new(big.Int).SetString(digits.String(), 10)
	if !ok {
		return nil, errNotAnInt
	}
	if negative {
		out.Neg(out)
	}
	return out, nil
}

// pyFormatG is `f"{v:g}"`: the format the proposal's budget sentence uses.
//
// Go's `'g'` with an explicit precision of 6 **is** Python's default `g` --
// same significant-digit count, same trailing-zero removal, same switch to
// exponent form outside `[-4, precision)`, same two-digit exponent. The
// precision has to be spelled out: Go's default for `'g'` is -1, which is the
// shortest round-tripping form, and `f"{1234567.0:g}"` is `1.23457e+06` where
// that would give `1.234567e+06`.
//
// The three special values are Python's spelling and not Go's, which is the
// whole reason this is a function rather than a call. A budget arrives as
// `float(payload["budget"])`, and `float("inf")` is a thing a client can
// send.
func pyFormatG(v float64) string {
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	}
	return strconv.FormatFloat(v, 'g', 6, 64)
}

// ErrFloatType is `float()`'s `TypeError`: the argument is not a string or a
// number. Its own error because the theme proposal's route catches
// `ValueError` and **not** this one, so the two reach the client as different
// statuses.
var ErrFloatType = fmt.Errorf("float() argument must be a string or a real number")

// PyFloat is `float(v)` over a JSON-decoded value.
//
// Not `strconv.ParseFloat`, and the differences all reach a user. Python's
// grammar takes underscores between digits (`1_0` is ten), any Unicode
// decimal digit (`５` is five), and the bare words `inf`, `infinity` and
// `nan` in any case with an optional sign -- while refusing the hex float
// and the `p` exponent that Go accepts. The refusal message is Python's own,
// because it is what reaches the client as a 422's `detail`.
func PyFloat(v any) (float64, error) {
	switch value := v.(type) {
	case bool:
		if value {
			return 1, nil
		}
		return 0, nil
	case float64:
		return value, nil
	case json.Number:
		return pyFloatFromString(value.String())
	case string:
		return pyFloatFromString(value)
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	default:
		return 0, fmt.Errorf("%w, not '%s'", ErrFloatType, pyTypeName(v))
	}
}

func pyFloatFromString(raw string) (float64, error) {
	refuse := func() (float64, error) {
		return 0, fmt.Errorf("could not convert string to float: %s", wire.PyRepr(raw))
	}
	body := strings.TrimFunc(raw, pyIsSpace)
	if body == "" {
		return refuse()
	}
	sign := 1.0
	switch body[0] {
	case '+':
		body = body[1:]
	case '-':
		sign = -1
		body = body[1:]
	}
	switch pyCasefold(body) {
	case "inf", "infinity":
		return sign * math.Inf(1), nil
	case "nan":
		// Python's `float("-nan")` is a NaN too; the sign is carried and never
		// observed, so it is dropped here rather than pretended about.
		return math.NaN(), nil
	}
	// Rebuild the literal in ASCII: underscores out, Unicode digits folded to
	// their values, everything else passed through for `strconv` to judge.
	var ascii strings.Builder
	runes := []rune(body)
	for i, r := range runes {
		if r == '_' {
			// A separator, and only between digits -- the same rule `int()`
			// applies, and the reason `1_0.5` is ten and a half while `1._5`
			// is not a number.
			if i == 0 || i == len(runes)-1 {
				return refuse()
			}
			if _, ok := pyDigitValue(runes[i-1]); !ok {
				return refuse()
			}
			if _, ok := pyDigitValue(runes[i+1]); !ok {
				return refuse()
			}
			continue
		}
		if value, ok := pyDigitValue(r); ok {
			ascii.WriteByte(asciiDigits[value])
			continue
		}
		if r >= 0x80 {
			return refuse()
		}
		ascii.WriteByte(byte(r))
	}
	text := ascii.String()
	// Go accepts spellings Python does not, and every one of them would be a
	// silent widening of what this endpoint takes.
	if strings.ContainsAny(text, "xXpP") || strings.Contains(text, "0x") {
		return refuse()
	}
	out, err := strconv.ParseFloat(text, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			// `float("1e400")` is `inf` in Python, not an error: the literal
			// overflows and Python says so by answering an infinity.
			return sign * out, nil
		}
		return refuse()
	}
	return sign * out, nil
}
