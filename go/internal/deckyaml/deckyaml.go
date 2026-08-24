// Package deckyaml parses a deck file -- and only parses.
//
// A load-bearing shape decision: deck writes are hand-rolled text surgery
// (`deckedit`), so nothing here ever serialises a deck. What the app needs
// YAML for is reading -- to validate, and to check an edit against a
// parse-mutate-dump oracle -- and the residual risk is parser divergence on
// the shapes the emitter writes (single-quoted scalars folded at width 100,
// doubled apostrophes, plain multi-line scalars, quoted look-alikes of
// booleans and nulls). This package is the answer: goccy/go-yaml behind one
// function, held to a recorded fixture and the recorded reading of it.
//
// The parse is into plain values (map[string]any, []any, string, int64,
// float64, bool, nil); the typed Deck is built over this.
//
// **`Parse` throws the key order away, and `ParseOrdered` is where it is
// kept.** A map has no order at all, and for almost everything here that
// costs nothing -- the deck's own
// fields are named one at a time by `FromText` and written back in an order
// `Dump` builds by hand. The exception is `notes:`, whose keys are the
// author's rather than the model's; the artifacts snapshot dumps a *parsed*
// deck, so somebody had to remember them. Rather than order every mapping in
// the module and change what `deckedit` reads, the order lives in a second
// entry point that yields Map instead of map[string]any and that `Parse` is
// now written in terms of -- one parse, one traversal, and no second set of
// rules about what a value may be.
package deckyaml

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/goccy/go-yaml"
)

// Pair is one key and its value inside a Map.
type Pair struct {
	Key   string
	Value any
}

// Map is a YAML mapping in the order the document wrote it.
//
// The same shape as `yamlemit.Map` next door and deliberately not the same
// type: that one is a payload somebody is about to *emit*, built key by key
// by a dumper, and this one is what a file said. They meet in
// `deck.Dump`, which converts.
type Map []Pair

// Get is the lookup a map would have given, at the cost of a scan. A deck's
// `notes:` holds a handful of keys, so the scan is the cheaper half of the
// bargain that keeps the order.
func (m Map) Get(key string) (any, bool) {
	for _, p := range m {
		if p.Key == key {
			return p.Value, true
		}
	}
	return nil, false
}

// MarshalJSON writes the mapping as a JSON object in the file's order --
// the recorded wire order. The stock encoder sorts a map's keys instead, and
// a deck's `notes:` once reached the wire alphabetised for exactly that
// reason. Nothing in the app reads the order -- but the same request
// answering with differently ordered bytes across releases is the kind of
// difference that is only ever noticed by whoever is diffing at the time.
func (m Map) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, p := range m {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(p.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		value, err := json.Marshal(p.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Plain is this mapping with the order thrown away -- what `Parse` answers,
// and what every caller that only ever looks keys up wants.
func (m Map) Plain() map[string]any {
	out := make(map[string]any, len(m))
	for _, p := range m {
		out[p.Key] = plain(p.Value)
	}
	return out
}

// Parse reads one YAML document into plain values. Mapping keys are strings,
// as every key in a deck file is; a document whose top level is not a mapping
// is refused, which is what `Deck.from_text` would do a line later.
func Parse(text []byte) (map[string]any, error) {
	top, err := ParseOrdered(text)
	if err != nil {
		return nil, err
	}
	return top.Plain(), nil
}

// ParseOrdered is Parse with every mapping's key order kept: mappings come
// back as Map, at every depth, and everything else is what Parse yields.
func ParseOrdered(text []byte) (Map, error) {
	var doc any
	if err := yaml.UnmarshalWithOptions(text, &doc, yaml.UseOrderedMap()); err != nil {
		return nil, fmt.Errorf("deck yaml: %w", err)
	}
	top, ok := orderedValue(doc).(Map)
	if !ok {
		return nil, fmt.Errorf("deck yaml: the document is not a mapping")
	}
	return top, nil
}

// orderedValue turns goccy's MapSlice/MapItem into Map, and normalises the
// scalars where goccy's own choice is not the recorded one.
func orderedValue(v any) any {
	switch t := v.(type) {
	case yaml.MapSlice:
		out := make(Map, 0, len(t))
		for _, item := range t {
			out = append(out, Pair{Key: fmt.Sprint(item.Key), Value: orderedValue(item.Value)})
		}
		return out
	case map[string]any:
		// Unreachable under `UseOrderedMap`, and kept because a decoder that
		// stopped honouring the option would otherwise fail silently rather
		// than loudly. Sorted, so what it yields is at least the same twice.
		out := make(Map, 0, len(t))
		for _, k := range sortedKeys(t) {
			out = append(out, Pair{Key: k, Value: orderedValue(t[k])})
		}
		return out
	case map[any]any:
		byName := make(map[string]any, len(t))
		for k, val := range t {
			byName[fmt.Sprint(k)] = val
		}
		out := make(Map, 0, len(byName))
		for _, k := range sortedKeys(byName) {
			out = append(out, Pair{Key: k, Value: orderedValue(byName[k])})
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = orderedValue(val)
		}
		return out
	case uint64:
		// goccy reads a non-negative integer as uint64; the recorded value
		// shape is a signed integer. A value that does not fit int64 is
		// left as it is -- nothing in a deck file is that large, and a
		// silent wrap would be worse.
		if t <= math.MaxInt64 {
			return int64(t)
		}
		return t
	case int:
		return int64(t)
	default:
		return v
	}
}

// plain is Map's own undoing, one level at a time: the shapes `Parse`
// promises, with every ordered mapping flattened back into a Go map.
func plain(v any) any {
	switch t := v.(type) {
	case Map:
		return t.Plain()
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = plain(val)
		}
		return out
	default:
		return v
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
