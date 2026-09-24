package cache_test

import (
	"math"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
)

// Every shape a per-kind extra may take, and the one canonical rendering of
// each.
//
// **This is a guard rather than a path**, the same argument
// `internal/claude/canonjson.go` makes: these bytes are hashed into a cache
// key, so a plausible-but-different spelling is worse than a crash. A float
// written as Go's `%g` instead of the recorded decimal form, an `int64`
// rendered through the `float64` arm, a map whose keys came out in Go's own
// order — each would move every stored row's key at once, with nothing failing
// and the cache merely reading as cold forever.
//
// The door is [cache.Payload], which is what the recorded corpus compares and
// therefore the only honest way to ask: the renderings below are the bytes a
// stored key was computed over, not an inner function's opinion.

// extra is the payload for one value in `Input.Extra`, which is where every
// arm of the renderer is reachable from.
func extra(t *testing.T, value any) string {
	t.Helper()
	return cache.Payload("engine", "kind", cache.Input{
		Extra: map[string]any{"v": value},
	})
}

// holds reports whether the payload renders `"v"` as exactly `want`.
func holds(t *testing.T, value any, want string) {
	t.Helper()
	got := extra(t, value)
	if !strings.Contains(got, `"extra":{"v":`+want+`}`) {
		t.Errorf("%#v renders as %s\n  want the fragment \"v\":%s", value, got, want)
	}
}

func TestEveryValueAnExtraMayHoldHasOneCanonicalForm(t *testing.T) {
	t.Parallel()
	// The scalars. `8` and `8.0` are different keys on purpose: an integer
	// arriving as a whole float would orphan every row computed over the
	// integer spelling, which is the trap in building an `Extra` out of
	// decoded JSON, where every number is a `float64`.
	holds(t, nil, "null")
	holds(t, true, "true")
	holds(t, false, "false")
	holds(t, 8, "8")
	holds(t, int64(9007199254740993), "9007199254740993")
	holds(t, 8.0, "8.0")
	holds(t, 0.25, "0.25")
	// The canonical decimal rendering switches to exponential notation at
	// different boundaries than Go's `%g`, which is the whole reason this
	// borrows `tier1.ReprFloat` rather than formatting a float itself.
	holds(t, 1e21, "1e+21")
	holds(t, 100000.0, "100000.0")

	// The lists, each with its own arm because the element types differ.
	holds(t, []int{7, 8}, "[7,8]")
	holds(t, []int{}, "[]")
	holds(t, []string{"b", "a"}, `["b","a"]`)
	holds(t, [][]int{{1, 2}, {3}}, "[[1,2],[3]]")
	holds(t, []any{1, "a", nil, true}, `[1,"a",null,true]`)

	// A nested mapping sorts its keys, because the recorded order is by code
	// point and Go's own map order is by nothing at all.
	holds(t, map[string]any{"b": 2, "a": 1}, `{"a":1,"b":2}`)
	holds(t, map[string]any{}, "{}")
}

// The three values that are not numbers are spelled as bare words rather than
// as null: a run whose rate came back as NaN and one whose rate was zero are
// different runs, and a key that folded them together would serve one for the
// other.
func TestTheFloatsThatAreNotNumbersAreSpelledOut(t *testing.T) {
	t.Parallel()
	holds(t, math.NaN(), "NaN")
	holds(t, math.Inf(1), "Infinity")
	holds(t, math.Inf(-1), "-Infinity")
}

// A value with no canonical form panics rather than being rendered as
// something plausible.
//
// **The crash is the feature.** A key that silently swallowed an unknown type
// would collide for two different inputs — two runs sharing one stored answer
// — which is worse than a crash by exactly the margin ADR 18 is about, because
// the wrong number would look exactly like the right one on the screen.
func TestAValueWithNoCanonicalFormRefusesRatherThanGuesses(t *testing.T) {
	t.Parallel()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatal("a value with no canonical form was rendered into a cache key")
		}
		if text, ok := got.(string); !ok ||
			!strings.Contains(text, "no canonical JSON form") {
			t.Errorf("the panic reads %v", got)
		}
	}()
	_ = extra(t, struct{ Unrenderable bool }{})
}

// Every escape the key's string form makes, including the three that no card
// name has ever contained and that nothing else in this package reaches.
//
// They are here because the escape table is a contract with the deployed
// database rather than a style: `\t` written as a raw tab would be a different
// key for the same input, and the row it orphaned would be somebody's
// twenty-thousand-game simulation.
func TestTheStringFormEscapesEveryThingItSaysItDoes(t *testing.T) {
	t.Parallel()
	holds(t, "a\bb", `"a\bb"`)
	holds(t, "a\fb", `"a\fb"`)
	holds(t, "a\tb", `"a\tb"`)
	holds(t, "a\nb\rc", `"a\nb\rc"`)
	holds(t, `a"b\c`, `"a\"b\\c"`)
	// Control characters below 0x20 with no short form, as lower-case hex.
	holds(t, "a\x01b", `"a\u0001b"`)
	// Everything above 0x7e is escaped too: the pool holds Bösium Strip and
	// Déjà Vu, so raw UTF-8 here would orphan a real deck's rows.
	holds(t, "Bösium", `"B\u00f6sium"`)
	// Above the BMP, a UTF-16 surrogate pair, because the recorded layout
	// counts in UTF-16 code units.
	holds(t, "\U0001F600", `"\ud83d\ude00"`)
	// And the three `encoding/json` would have escaped and this does not,
	// which is the first of the three behaviours that made this hand-written.
	holds(t, "a<b>c&d", `"a<b>c&d"`)
	// `/` is not escaped either.
	holds(t, "a/b", `"a/b"`)
}
