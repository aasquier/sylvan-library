package wire

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/pyfloat"
)

// PyRepr is Python's `repr()` of a str, for the sentences FastAPI routes
// build with `!r` -- `no colour combination 'nope'`, `no effect 'nope'`.
// Single quotes unless the text holds a single quote and no double quote,
// backslash and the quote escaped, control characters written as Python
// writes them. The frontend shows the sentence; the contract suite pins the
// shape; this keeps the words the same from either door.
func PyRepr(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == rune(quote):
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r > 0x7f && !unicode.IsPrint(r):
			if r <= 0xffff {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				fmt.Fprintf(&b, `\U%08x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte(quote)
	return b.String()
}

// PyStr is Python's `str()` over a JSON-decoded value.
//
// The routes that take a slug in the body spell it `str(payload.get("slug"))`,
// and for every value a real client sends — a string — `str` is the identity
// and this is a formality. For the values a client does *not* send it is not:
// `str(["x"])` is `"['x']"`, and Go's `fmt.Sprint` of the same decoded list is
// `"[x]"`. That difference lands in a 404's `detail`, which the deck page
// renders verbatim.
//
// **Found by diffing the pair on 2026-08-23, and it had been live since Phase
// 5**: `/api/sim/mana`, `/lands` and `/shelf` all answered a different sentence
// from Python's for a list-shaped slug, and nothing noticed because no test and
// no golden sends one. Fixed here rather than at four call sites, because a
// helper is where the four of them already agree.
func PyStr(v any) string {
	switch value := v.(type) {
	case nil:
		return "None"
	case string:
		return value
	case bool:
		if value {
			return "True"
		}
		return "False"
	}
	return PyReprValue(v)
}

// PyReprValue is Python's `repr()` over a JSON-decoded value.
//
// `str` and `repr` differ only for strings — `str("x")` is `x` and `repr("x")`
// is `'x'` — so a container renders its members with this and never with
// [PyStr]: `str(["x"])` is `"['x']"`, quotes and all.
func PyReprValue(v any) string {
	switch value := v.(type) {
	case nil:
		return "None"
	case string:
		return PyRepr(value)
	case bool:
		if value {
			return "True"
		}
		return "False"
	case json.Number:
		return pyNumber(value)
	case float64:
		return pyfloat.Repr(value)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, PyReprValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		// A JSON object decodes to a map, whose iteration order Go randomises
		// where a Python dict keeps the document's. Sorted here, deliberately
		// and imperfectly: this reaches a person only through a refusal
		// naming a value nobody meant to send, and a stable wrong order is
		// better than a different one every request. The alternative —
		// decoding every body through an ordered map — is a cost paid on
		// every request for a case no client reaches.
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, PyRepr(k)+": "+PyReprValue(value[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprint(v)
}

// pyNumber renders a JSON number the way Python's `repr` does: an integer
// literal keeps every digit (Python's ints are unbounded), and anything with a
// point or an exponent goes through `repr(float)` — so `1.0` stays `1.0` where
// Go's default would write `1`.
func pyNumber(n json.Number) string {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		return s
	}
	f, err := n.Float64()
	if err != nil {
		return s
	}
	return pyfloat.Repr(f)
}
