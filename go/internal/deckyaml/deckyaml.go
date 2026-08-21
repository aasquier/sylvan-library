// Package deckyaml parses a deck file the way PyYAML does -- and only parses.
//
// The load-bearing discovery of the migration plan (docs/go-migration/PLAN.md
// section 6): `decks/edit.py` is hand-rolled text surgery, so Go never
// serialises a deck. What it needs YAML for is reading -- to validate, and to
// check an edit against a parse-mutate-dump oracle -- and the residual risk is
// parser divergence on the shapes PyYAML emits (single-quoted scalars folded
// at width 100, doubled apostrophes, plain multi-line scalars, quoted
// look-alikes of booleans and nulls). This package is the Phase 2 spike for
// that: goccy/go-yaml behind one function, proven against a fixture Python
// wrote and PyYAML's own reading of it (`tests/go_fixtures.py`).
//
// The parse is into plain Go values (map[string]any, []any, string, int64,
// float64, bool, nil), which is what `yaml.safe_load` yields in Python terms;
// a typed Deck arrives in Phase 3 and will be built over this.
package deckyaml

import (
	"fmt"
	"math"

	"github.com/goccy/go-yaml"
)

// Parse reads one YAML document into plain values. Mapping keys are strings,
// as every key in a deck file is; a document whose top level is not a mapping
// is refused, which is what `Deck.from_text` would do a line later.
func Parse(text []byte) (map[string]any, error) {
	var doc any
	if err := yaml.UnmarshalWithOptions(text, &doc, yaml.UseOrderedMap()); err != nil {
		return nil, fmt.Errorf("deck yaml: %w", err)
	}
	plain := plainValue(doc)
	top, ok := plain.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("deck yaml: the document is not a mapping")
	}
	return top, nil
}

// plainValue turns goccy's MapSlice/MapItem (ordered maps) into the
// map/slice shapes the rest of the module -- and encoding/json -- expect.
func plainValue(v any) any {
	switch t := v.(type) {
	case yaml.MapSlice:
		out := make(map[string]any, len(t))
		for _, item := range t {
			out[fmt.Sprint(item.Key)] = plainValue(item.Value)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = plainValue(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = plainValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = plainValue(val)
		}
		return out
	case uint64:
		// goccy reads a non-negative integer as uint64; PyYAML gives an int.
		// A value that does not fit int64 is left as it is -- nothing in a
		// deck file is that large, and a silent wrap would be worse.
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
