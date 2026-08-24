package wire

import (
	"strconv"
	"strings"
)

// Ordered JSON.
//
// The wire keeps the recorded key order where a reader might notice — the
// deck page's payload is read by people in DevTools as much as by the client
// — so the hand-built bodies in the API are ordered pairs rather than maps.
// encoding/json sorts a map's keys where the recorded payloads keep the
// order they were built in, and that difference has shipped once already:
// the deck page's Notes tab was alphabetical from v159 to v166, scrambling
// a deliberate reading order into nonsense.
//
// These lived in `internal/api` until the deck reads were
// extracted into `internal/deckread` so the Claude tools could call the same
// payload builders the routes call. Two packages needed them, and neither is
// the other's parent — so they came here, where the envelope already lives.

// KV is one ordered pair.
type KV struct {
	Key   string
	Value any
}

// OrderedMap is a map that marshals in its own order.
type OrderedMap []KV

// MarshalJSON writes the pairs in the order they were built.
func (o OrderedMap) MarshalJSON() ([]byte, error) { return MarshalOrdered(o) }

// MarshalOrdered renders pairs as a JSON object, in order, through Marshal —
// so the HTML-escaping and float rules that make this file match the
// recorded encoding apply to every value inside an ordered body too.
func MarshalOrdered(pairs []KV) ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(p.Key))
		b.WriteByte(':')
		raw, err := Marshal(p.Value)
		if err != nil {
			return nil, err
		}
		b.Write(raw)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}
