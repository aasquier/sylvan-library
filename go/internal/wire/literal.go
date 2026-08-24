package wire

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/floats"
)

// Quote renders user text as a quoted literal inside a served sentence --
// `no colour combination 'nope'`, `no effect 'nope'`. Single quotes unless
// the text holds a single quote and no double quote; backslash and the
// quote escaped; other control characters as `\xNN`, unprintables beyond
// ASCII as `\uNNNN`/`\UNNNNNNNN`. The frontend shows the sentence verbatim
// and the recorded corpus pins the bytes, so the rule is exact rather than
// roughly right.
func Quote(s string) string {
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

// Plain renders a JSON-decoded value for interpolation into a served
// sentence. For the value every real client sends — a string — it is the
// identity and this is a formality. For the values a client does *not* send
// it is not: a decoded list renders as `['x']` — brackets, quotes and a
// comma-space — where `fmt.Sprint` would write `[x]`. That difference lands
// in a 404's `detail`, which the deck page renders verbatim; `nil` is
// `None` and booleans are `True`/`False`, the sentence vocabulary the
// recorded corpus pins.
//
// One helper rather than four call sites — `/api/sim/mana`, `/lands` and
// `/shelf` all interpolate a body's slug into the same refusal — because a
// helper is where the four of them are forced to agree.
func Plain(v any) string {
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
	return Literal(v)
}

// Literal renders a JSON-decoded value as a literal. It differs from
// [Plain] only for strings — `x` plain, `'x'` as a literal — and a container
// renders its members with this and never with [Plain]: a decoded list is
// `['x']`, quotes and all.
func Literal(v any) string {
	switch value := v.(type) {
	case nil:
		return "None"
	case string:
		return Quote(value)
	case bool:
		if value {
			return "True"
		}
		return "False"
	case json.Number:
		return literalNumber(value)
	case float64:
		return floats.Repr(value)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, Literal(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		// A JSON object decodes to a map, whose iteration order is
		// randomised where the document had one. Sorted here, deliberately
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
			parts = append(parts, Quote(k)+": "+Literal(value[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprint(v)
}

// literalNumber renders a JSON number as a literal: an integer keeps every
// digit exactly as the document wrote it, however wide, and anything with a
// point or an exponent goes through [floats.Repr] — so `1.0` stays `1.0`
// where a plain format would write `1`.
func literalNumber(n json.Number) string {
	s := n.String()
	if !strings.ContainsAny(s, ".eE") {
		return s
	}
	f, err := n.Float64()
	if err != nil {
		return s
	}
	return floats.Repr(f)
}
