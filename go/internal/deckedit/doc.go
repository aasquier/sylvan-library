package deckedit

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The document half of the oracle: a parsed deck is an ordinary map, and every
// operation mutates a deep copy of one to say what the text surgery *ought* to
// have produced. Nothing in this file touches text.

// deepCopy rebuilds the value shapes `deckyaml.Parse` yields, deeply.
// Scalars are immutable, so only the containers need new homes.
func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = deepCopy(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = deepCopy(val)
		}
		return out
	default:
		return v
	}
}

func copyDoc(doc map[string]any) map[string]any {
	out, _ := deepCopy(doc).(map[string]any)
	return out
}

// equalDocs asks whether two parsed documents hold the same values.
//
// `reflect.DeepEqual` would nearly do, and is deliberately not used: it
// separates `[]any(nil)` from `[]any{}`, which the two sides of this
// comparison reach by different routes -- one from the parser, one from a
// slice an operation emptied -- and a deck with its last swap-board card
// removed would fail verification for a difference no reader could see.
func equalDocs(a, b any) bool {
	switch left := a.(type) {
	case map[string]any:
		right, ok := b.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for k, lv := range left {
			rv, ok := right[k]
			if !ok || !equalDocs(lv, rv) {
				return false
			}
		}
		return true
	case []any:
		right, ok := b.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for i := range left {
			if !equalDocs(left[i], right[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

// firstDifference says where two documents first disagree, as a readable path.
//
// Only ever called on documents already known to differ, so it always finds
// something to say. Keys are walked in sorted order rather than insertion
// order -- Go maps have none -- which changes *which* difference is named when
// there are several, never whether one is found.
func firstDifference(expected, actual any, path string) string {
	switch want := expected.(type) {
	case map[string]any:
		if got, ok := actual.(map[string]any); ok {
			for _, key := range unionOfKeys(want, got) {
				wantValue, inWant := want[key]
				gotValue, inGot := got[key]
				if !inWant {
					return fmt.Sprintf("%s.%s appeared", path, key)
				}
				if !inGot {
					return fmt.Sprintf("%s.%s disappeared", path, key)
				}
				if !equalDocs(wantValue, gotValue) {
					return firstDifference(wantValue, gotValue, path+"."+key)
				}
			}
		}
	case []any:
		if got, ok := actual.([]any); ok {
			if len(want) != len(got) {
				where := path
				if where == "" {
					where = "the list"
				}
				return fmt.Sprintf("%s has %d entries, expected %d", where, len(got), len(want))
			}
			for i := range want {
				if !equalDocs(want[i], got[i]) {
					return firstDifference(want[i], got[i], fmt.Sprintf("%s[%d]", path, i))
				}
			}
		}
	}
	where := path
	if where == "" {
		where = "the document"
	}
	return fmt.Sprintf("%s is %s, expected %s", where, quotedValue(actual), quotedValue(expected))
}

func unionOfKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range []map[string]any{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// quotedValue renders a value in the recorded refusal spelling -- strings
// quoted, `None`/`True`/`False` for the unspeakables, containers in
// brackets -- because these strings land in refusal messages a person
// reads. A card name is the common case and it is the one that matters:
// `'Sol Ring'`, not `Sol Ring`, so the quotes show where the name begins
// and ends.
func quotedValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		if strings.Contains(t, "'") && !strings.Contains(t, `"`) {
			return `"` + t + `"`
		}
		return "'" + strings.ReplaceAll(t, "'", `\'`) + "'"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case map[string]any:
		parts := make([]string, 0, len(t))
		for _, k := range sortedKeys(t) {
			parts = append(parts, quotedValue(k)+": "+quotedValue(t[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, quotedValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprint(v)
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

// listOf reads a document key as a list of items, treating absent and null
// as empty.
func listOf(doc map[string]any, key string) []any {
	items, _ := doc[key].([]any)
	return items
}

// cardAt reads one list entry as a card mapping.
func cardAt(items []any, i int) map[string]any {
	if i < 0 || i >= len(items) {
		return nil
	}
	card, _ := items[i].(map[string]any)
	return card
}

// nameMatches is the comparison every duplicate check uses: case-folded and
// trimmed, exactly as the operations locate a card.
func nameMatches(card map[string]any, name string) bool {
	return strings.EqualFold(strings.TrimSpace(asString(card["name"])),
		strings.TrimSpace(name))
}
