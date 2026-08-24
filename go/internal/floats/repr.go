package floats

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Repr is CPython's `repr(float)`, which is also what `json.dumps` writes for
// one — `float.__repr__` is the encoder's number renderer.
//
// It moved here from `internal/sim/tier1`, where it was written for the
// simulator's digest, when a second family needed it: the Forge result's five
// float fields go onto a wire a person reads in DevTools, and
// `encoding/json` writes `4` for the float64 `4.0` where Python writes `4.0`.
// That is the third of the three ways `encoding/json` differs from
// `json.dumps` (after HTML escaping and `ensure_ascii`), and the one with no
// encoder option to turn it off.
//
// The shortest decimal that round-trips, rendered fixed when the decimal
// point sits within reach and exponential when it does not: Python switches
// at a decimal exponent below -3 or above 16, so `1e16` reads `1e+16` while
// `1e15` reads `1000000000000000.0`, and `0.0001` stays fixed while
// `0.00001` becomes `1e-05`. A fixed rendering always carries a decimal
// point (`100.0`, never `100`); an exponential one never gains a spurious
// `.0`, and its exponent is at least two digits.
//
// Go's shortest formatting supplies the digits; only the presentation is
// Python's. The boundaries above are pinned by a corpus taken from CPython
// rather than trusted from this comment.
func Repr(v float64) string {
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	}
	sign := ""
	if math.Signbit(v) {
		sign, v = "-", -v
	}
	if v == 0 {
		return sign + "0.0"
	}

	// Shortest round-trip digits, and where the decimal point falls.
	s := strconv.FormatFloat(v, 'e', -1, 64)
	epos := strings.IndexByte(s, 'e')
	digits := strings.Replace(s[:epos], ".", "", 1)
	exp, err := strconv.Atoi(s[epos+1:])
	if err != nil {
		panic("floats: unreadable float exponent in " + s)
	}
	decpt := exp + 1

	if decpt < -3 || decpt > 16 {
		out := digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		e, esign := decpt-1, "+"
		if e < 0 {
			e, esign = -e, "-"
		}
		return fmt.Sprintf("%s%se%s%02d", sign, out, esign, e)
	}
	switch {
	case decpt <= 0:
		return sign + "0." + strings.Repeat("0", -decpt) + digits
	case decpt >= len(digits):
		return sign + digits + strings.Repeat("0", decpt-len(digits)) + ".0"
	default:
		return sign + digits[:decpt] + "." + digits[decpt:]
	}
}

// Float is a float64 that marshals the way Python writes one.
//
// A named type rather than a helper at every call site, because the failure it
// prevents is *forgetting* — a payload struct with a bare `float64` renders
// `4` for `4.0` and nothing goes red, since a client reads the number either
// way and a shape-checking golden sees "number" both times. Declaring the
// field as this type makes the decision once, where the struct is written.
type Float float64

// MarshalJSON writes the number Python would write.
func (f Float) MarshalJSON() ([]byte, error) {
	return []byte(Repr(float64(f))), nil
}

// UnmarshalJSON reads one back, so a payload round-trips through a cache or a
// wire without changing type.
func (f *Float) UnmarshalJSON(raw []byte) error {
	v, err := strconv.ParseFloat(strings.TrimSpace(string(raw)), 64)
	if err != nil {
		return err
	}
	*f = Float(v)
	return nil
}
