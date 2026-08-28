package claude

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The canonical dialect's closed set of inputs, and its four refusals.
//
// **These bytes are a cache key.** A stored dossier is found by hashing the
// sorted-key rendering of the response schema, so a change to any spelling
// below does not produce different-but-equivalent JSON -- it produces a
// different key, and the effect on a running instance is every stored dossier
// becoming unreachable at once while the code that wrote them still looks
// correct. `canonjson.go`'s package comment argues why the bytes are written
// by hand; this holds them to it.
//
// The corpus in `dossier_test.go` pins the schema's rendering, which is one
// document. What it cannot reach is the *shape of the dialect*: the types
// that never appear in that one document, and the four panics that exist so
// that a type which starts appearing is a crash rather than a wrong key that
// looks like a key. A silent `%g` where a float used to be refused is the
// exact failure this file is here to make loud.

// A value of every type in the closed set renders the recorded way, compact.
func TestTheClosedSetRendersTheRecordedBytes(t *testing.T) {
	t.Parallel()
	compact := dumpOptions{}
	for _, row := range []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, "null"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"string", "a", `"a"`},
		{"int", 7, "7"},
		{"int64", int64(-9), "-9"},
		// The reflected integer kinds, which no brief carries today and
		// which the writer accepts on purpose so that one starting to is not
		// a panic in production.
		{"int8", int8(-8), "-8"},
		{"int16", int16(300), "300"},
		{"int32", int32(-70000), "-70000"},
		{"uint", uint(7), "7"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4294967295), "4294967295"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		// An ordered map is written in its own order, whatever SortKeys says.
		{"ordered map", wire.OrderedMap{{Key: "b", Value: 1}, {Key: "a", Value: 2}},
			`{"b": 1, "a": 2}`},
		// ...and its unwrapped form is the same object, because the two are
		// the same type underneath and a client that built one by hand must
		// not get a different key.
		{"[]wire.KV", []wire.KV{{Key: "b", Value: 1}, {Key: "a", Value: 2}},
			`{"b": 1, "a": 2}`},
		{"empty ordered map", wire.OrderedMap{}, "{}"},
		{"empty []wire.KV", []wire.KV{}, "{}"},
		{"empty slice", []any{}, "[]"},
		// A nil slice is `[]`, never `null` -- the briefs' lists are built by
		// appending to an empty one, and a key that flipped between the two
		// on an empty deck would be two keys for one input.
		{"nil slice", []string(nil), "[]"},
		{"slice", []any{1, "two", nil, true}, `[1, "two", null, true]`},
		{"nested slice", []any{[]any{1}, []any{}}, `[[1], []]`},
		{"array", [2]int{1, 2}, "[1, 2]"},
	} {
		if got := dumpJSON(row.value, compact); got != row.want {
			t.Errorf("%s rendered %s, want %s", row.name, got, row.want)
		}
	}
}

// A pointer renders what it points at, and a nil one renders `null` -- so an
// optional field carries no information about whether the caller reached for
// a pointer or a value.
func TestAPointerRendersItsTargetAndANilOneRendersNull(t *testing.T) {
	t.Parallel()
	s, n, b := "a", 7, true
	for _, row := range []struct {
		value any
		want  string
	}{
		{&s, `"a"`},
		{&n, "7"},
		{&b, "true"},
		{(*string)(nil), "null"},
		{(*int)(nil), "null"},
		{(*wire.OrderedMap)(nil), "null"},
	} {
		if got := dumpJSON(row.value, dumpOptions{}); got != row.want {
			t.Errorf("%#v rendered %s, want %s", row.value, got, row.want)
		}
	}
	// A pointer inside a structure is followed too, so `null` in a rendered
	// brief means the value was absent rather than the field being one.
	got := dumpJSON(wire.OrderedMap{{Key: "a", Value: &s}, {Key: "b", Value: (*int)(nil)}},
		dumpOptions{})
	if want := `{"a": "a", "b": null}`; got != want {
		t.Errorf("rendered %s, want %s", got, want)
	}
}

// A Go map sorts its keys and **refuses to render without being told to**,
// because it has no order of its own -- and a renderer that picked one at
// random would produce a different key on every run of the same binary.
func TestAGoMapSortsItsKeysAndRefusesToGuessAnOrder(t *testing.T) {
	t.Parallel()
	m := map[string]any{"b": 1, "a": 2, "C": 3, "á": 4}
	// Sorted by code point, so an uppercase letter sorts before every
	// lowercase one and a non-ASCII key sorts after both -- and the key is
	// **escaped** on the way out, which is the second of the three places
	// this dialect and `encoding/json` disagree. The sort is on the string
	// and the escape is on the rendering, so a key that sorted by its escaped
	// form would order differently once anything above ASCII appeared.
	got := dumpJSON(m, dumpOptions{SortKeys: true})
	if want := `{"C": 3, "a": 2, "b": 1, "\u00e1": 4}`; got != want {
		t.Errorf("rendered %s\n    want %s", got, want)
	}
	if got := dumpJSON(map[string]any{}, dumpOptions{SortKeys: true}); got != "{}" {
		t.Errorf("an empty map rendered %s", got)
	}
	// Without SortKeys it is a crash rather than an arbitrary order.
	assertPanics(t, "a Go map without SortKeys", "insertion order", func() {
		_ = dumpJSON(m, dumpOptions{})
	})
}

// TestTheRefusedTypesArePanicsAndNotGuesses holds the three crashes.
//
// Each of them exists because the plausible alternative is worse. A float
// written with `%g` is a key that looks like a key and is not the recorded
// one; an unknown type rendered by `fmt` is the same fault wearing a
// different hat. A panic here is a build that fails on the machine of
// whoever added the field, which is the only place this can be cheap.
func TestTheRefusedTypesArePanicsAndNotGuesses(t *testing.T) {
	t.Parallel()
	assertPanics(t, "a float64", "canonical decimal", func() {
		_ = dumpJSON(1.5, dumpOptions{})
	})
	assertPanics(t, "a float32", "canonical decimal", func() {
		_ = dumpJSON(float32(1.5), dumpOptions{})
	})
	// Including a float that happens to be whole, which is the one somebody
	// would be tempted to let through.
	assertPanics(t, "a whole float", "canonical decimal", func() {
		_ = dumpJSON(2.0, dumpOptions{})
	})
	// A float inside a structure is refused just as loudly as one at the top.
	assertPanics(t, "a float in a list", "canonical decimal", func() {
		_ = dumpJSON([]any{1, 2.5}, dumpOptions{})
	})
	assertPanics(t, "a float in an object", "canonical decimal", func() {
		_ = dumpJSON(wire.OrderedMap{{Key: "a", Value: 0.5}}, dumpOptions{})
	})
	// And a type the dialect has never seen.
	assertPanics(t, "a struct", "no rendering", func() {
		_ = dumpJSON(struct{ A int }{1}, dumpOptions{})
	})
	assertPanics(t, "a channel", "no rendering", func() {
		_ = dumpJSON(make(chan int), dumpOptions{})
	})
	assertPanics(t, "a map with the wrong key type", "no rendering", func() {
		_ = dumpJSON(map[int]any{1: 2}, dumpOptions{SortKeys: true})
	})
}

// assertPanics runs fn and requires a panic whose message contains want.
//
// The message matters as much as the panic: whoever hits this is adding a
// field, and "no rendering for a float64" tells them what to do where a bare
// index-out-of-range would not.
func assertPanics(t *testing.T, what, want string, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Errorf("%s did not panic; it rendered something instead", what)
			return
		}
		if msg, _ := recovered.(string); !strings.Contains(msg, want) {
			t.Errorf("%s panicked with %v, want it to mention %q", what, recovered, want)
		}
	}()
	fn()
}

// The indented spelling is the one the brief is sent as, and it differs from
// the compact one in more than whitespace: the item separator loses its
// space and gains a newline, while the key separator keeps its space.
func TestTheIndentedSpellingIsTheOtherDialect(t *testing.T) {
	t.Parallel()
	value := wire.OrderedMap{
		{Key: "cards", Value: []any{"a", "b"}},
		{Key: "empty", Value: []any{}},
		{Key: "nested", Value: wire.OrderedMap{{Key: "k", Value: 1}}},
	}
	got := dumpJSON(value, dumpOptions{Indent: 2})
	want := strings.Join([]string{
		`{`,
		`  "cards": [`,
		`    "a",`,
		`    "b"`,
		`  ],`,
		`  "empty": [],`,
		`  "nested": {`,
		`    "k": 1`,
		`  }`,
		`}`,
	}, "\n")
	if got != want {
		t.Errorf("rendered\n%s\n    want\n%s", got, want)
	}
	// An empty container stays on one line in both spellings -- `{\n}` would
	// be a second way to write nothing.
	if got := dumpJSON(wire.OrderedMap{}, dumpOptions{Indent: 2}); got != "{}" {
		t.Errorf("an empty object indented to %q", got)
	}
	if got := dumpJSON([]any{}, dumpOptions{Indent: 2}); got != "[]" {
		t.Errorf("an empty list indented to %q", got)
	}
}
