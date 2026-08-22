package cache

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
)

// Python's `json.dumps(..., sort_keys=True, separators=(",", ":"),
// ensure_ascii=True)`, written out rather than delegated to `encoding/json`.
//
// The key is a **sha256 over these bytes**, so this is one of the few places
// in the port where "equivalent JSON" is not equivalent at all. Three
// differences would each have produced a silently different key:
//
//   - **Go escapes `<`, `>` and `&`; Python does not.** `SetEscapeHTML(false)`
//     fixes that one, and it is the one everybody knows.
//   - **`ensure_ascii=True` escapes every non-ASCII rune as `\uXXXX`, with a
//     surrogate pair above the BMP; Go emits raw UTF-8.** This is not exotic
//     input: the pool holds Bösium Strip, Déjà Vu and Círdan the Shipwright,
//     so a real deck would have keyed differently in the two runtimes for a
//     reason nobody chose.
//   - **A float renders as CPython's `repr`**, which switches to exponential
//     notation at different boundaries than Go's `%g`. `Extra` carries
//     `mulligan.Flat` (0.25), so the path is live rather than theoretical.
//
// `encoding/json` could have been coaxed through the first and the third with
// an encoder and a `json.Marshaler`, and never through the second. Writing the
// bytes is shorter than the workaround and says what it means.

// writeString is `json.encoder.encode_basestring_ascii`.
//
// Escapes: `"` and `\`; the five short forms `\b \f \n \r \t`; everything
// below 0x20 and everything **above 0x7e** as `\uXXXX`, lower-case hex. `/`
// is not escaped, and neither are `<`, `>` or `&`.
//
// A rune above the BMP becomes a UTF-16 surrogate pair, because Python's
// encoder works in UTF-16 code units. Invalid UTF-8 in a Go string decodes to
// U+FFFD, which is a shape a Python `str` cannot hold at all -- there is no
// faithful answer, and the replacement character is at least a stable one.
func writeString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			switch {
			case r >= 0x20 && r <= 0x7e:
				b.WriteByte(byte(r))
			case r > 0xffff:
				// Both halves land in 0xD800-0xDFFF by construction, so
				// neither narrowing can lose a bit.
				hi, lo := utf16.EncodeRune(r)
				writeHex4(b, uint16(hi)) //nolint:gosec // a surrogate, by construction
				writeHex4(b, uint16(lo)) //nolint:gosec // a surrogate, by construction
			default:
				writeHex4(b, uint16(r)) //nolint:gosec // guarded by the case above
			}
		}
	}
	b.WriteByte('"')
}

const hexDigits = "0123456789abcdef"

func writeHex4(b *strings.Builder, v uint16) {
	b.WriteString(`\u`)
	b.WriteByte(hexDigits[(v>>12)&0xf])
	b.WriteByte(hexDigits[(v>>8)&0xf])
	b.WriteByte(hexDigits[(v>>4)&0xf])
	b.WriteByte(hexDigits[v&0xf])
}

// writeAny renders the values `Input.Extra` may hold.
//
// The set is small and closed on purpose: a mapping, a list, an int, a float,
// a bool, a string, and nil. Anything else panics rather than being rendered
// as something plausible -- a cache key that silently swallowed an unknown
// type would collide for two different inputs, which is worse than the crash
// by exactly the margin ADR 18 is about.
func writeAny(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		writeBool(b, x)
	case string:
		writeString(b, x)
	case int:
		b.WriteString(strconv.Itoa(x))
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case float64:
		writeFloat(b, x)
	case []int:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Itoa(e))
		}
		b.WriteByte(']')
	case []string:
		writeStrings(b, x)
	case [][]int:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeAny(b, e)
		}
		b.WriteByte(']')
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			writeAny(b, e)
		}
		b.WriteByte(']')
	case map[string]any:
		names := make([]string, 0, len(x))
		for name := range x {
			names = append(names, name)
		}
		// `sort_keys=True` sorts by code point; Go compares bytes, and for
		// valid UTF-8 the two orders are the same.
		sort.Strings(names)
		b.WriteByte('{')
		for i, name := range names {
			if i > 0 {
				b.WriteByte(',')
			}
			writeKey(b, name)
			writeAny(b, x[name])
		}
		b.WriteByte('}')
	default:
		panic("sim/cache: no canonical JSON form for a value in Input.Extra")
	}
}

// writeFloat is `json`'s float, which is `float.__repr__` for a finite value
// and the three bare words for the others.
//
// `tier1.ReprFloat` already reproduces CPython's rendering, corpus and all, so
// this borrows it rather than growing a second one -- the same reasoning that
// keeps `sim.Round` the only banker's-rounding implementation in the module.
func writeFloat(b *strings.Builder, f float64) {
	switch {
	case math.IsNaN(f):
		b.WriteString("NaN")
	case math.IsInf(f, 1):
		b.WriteString("Infinity")
	case math.IsInf(f, -1):
		b.WriteString("-Infinity")
	default:
		b.WriteString(tier1.ReprFloat(f))
	}
}
