package tarot

import (
	"math/big"
	"strings"
)

// ParseSeed reads the `?seed=` query value the way FastAPI's `seed: int | None`
// does, returning nil-and-false for anything it refuses.
//
// Pydantic's integer grammar is not strconv.ParseInt's, and it is not Python's
// `int()` either — it sits between them, which is why this is a hand-written
// scanner rather than either library call:
//
//   - Surrounding whitespace is stripped, so "  7  " is seven.
//   - A leading "+" is allowed, and "0007" is seven rather than octal.
//   - SINGLE UNDERSCORES BETWEEN DIGITS are separators, so "1_0" is ten.
//     Not leading, not trailing, not doubled, not next to the sign.
//   - Only ASCII digits count. Python's own int() takes any Unicode decimal
//     digit and reads the fullwidth "７" as seven; Pydantic refuses it, and so
//     does this. That single row is the reason the corpus is generated from
//     TypeAdapter(int) rather than from int() — measured, not assumed.
//
// The value is a *big.Int because Python's are unbounded and the seed is
// echoed back on the wire: 2**70 is a legitimate seed and an int64 would
// answer a different reading under a different number.
func ParseSeed(raw string) (*big.Int, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, false
	}
	body := text
	switch body[0] {
	case '+', '-':
		body = body[1:]
	}
	if body == "" {
		return nil, false
	}
	var digits strings.Builder
	digits.Grow(len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c >= '0' && c <= '9':
			digits.WriteByte(c)
		case c == '_':
			// A separator is only a separator with a digit on each side.
			if i == 0 || i == len(body)-1 {
				return nil, false
			}
			prev, next := body[i-1], body[i+1]
			if prev < '0' || prev > '9' || next < '0' || next > '9' {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	out, ok := new(big.Int).SetString(digits.String(), 10)
	if !ok {
		return nil, false
	}
	if text[0] == '-' {
		out.Neg(out)
	}
	return out, true
}
