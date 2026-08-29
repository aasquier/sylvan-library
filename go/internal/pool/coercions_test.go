package pool

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// The two coercion layers the card pool is built out of: what the DuckDB
// driver hands back on the way in (`rows.go`, the refresh) and on the way out
// (`records.go`, every read).
//
// They are worth pinning rather than trusting because **the driver's widths
// are not the schema's**. A column declared INTEGER can arrive as `int32`,
// `int64` or `float64` depending on how it was written and which query
// produced it, and a reading that only handled the width this machine
// happened to see would return zero on another -- silently, as a mana value
// of 0 or a price of nothing.
//
// The pool's own package comment records the case that made this concrete: a
// double-faced card whose mana cost came back NULL once cast Etali on turn
// one, because a fallback was missing rather than wrong.

// `IsLand` is the one predicate the whole mana base rests on, and its rule is
// narrower than "the type line says Land": a modal DFC with a land back is a
// land drop, and a *transforming* permanent with a land back is not -- you
// cast the front, and the back arrives only by flipping.
func TestIsLandSeparatesAModalBackFaceFromATransformingOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, typeLine, layout string
		want                   bool
	}{
		{"a basic", "Basic Land — Forest", "normal", true},
		{"a nonbasic", "Land", "normal", true},
		{"a legendary land", "Legendary Land", "normal", true},
		{"a creature", "Creature — Elf Druid", "normal", false},
		{"an artifact", "Artifact", "normal", false},
		// A modal DFC: you may put the back down as your land drop.
		{"a modal DFC with a land back", "Creature — Elf // Land", "modal_dfc", true},
		{"a modal DFC land front", "Land // Instant", "modal_dfc", true},
		// A transforming permanent: the back arrives only by flipping, so
		// it is not a land drop and must not be counted as one.
		{"a transforming permanent with a land back",
			"Creature — Human Werewolf // Land", "transform", false},
		{"a flip card with a land back", "Creature — Human // Land", "flip", false},
		// The word appearing inside another word does not make it a land --
		// but the front-face read is a substring test, so this pins what it
		// actually does rather than what it might.
		{"a Landwalk creature", "Creature — Merfolk", "normal", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &CardRecord{TypeLine: tc.typeLine, Layout: tc.layout}
			if got := rec.IsLand(); got != tc.want {
				t.Errorf("%q (%s) is land=%v, want %v", tc.typeLine, tc.layout, got, tc.want)
			}
		})
	}
}

// The creature read is the front face too, matching `power`: a card whose
// back is a creature has no power on the front, and counting it as one would
// put a nonexistent body in the curve.
func TestIsCreatureReadsTheFrontFaceOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		typeLine string
		want     bool
	}{
		{"Creature — Elf Druid", true},
		{"Legendary Creature — Cat Warrior", true},
		{"Artifact Creature — Golem", true},
		{"Land // Creature — Werewolf", false},
		{"Instant", false},
		{"Creature — Human // Land", true},
		{"", false},
	} {
		rec := &CardRecord{TypeLine: tc.typeLine}
		if got := rec.IsCreature(); got != tc.want {
			t.Errorf("%q is creature=%v, want %v", tc.typeLine, got, tc.want)
		}
	}

	// And the front face itself, which both predicates read through.
	rec := &CardRecord{TypeLine: "Creature — Human // Creature — Werewolf"}
	if got := rec.FrontTypeLine(); got != "Creature — Human" {
		t.Errorf("the front face is %q", got)
	}
	if got := (&CardRecord{TypeLine: "Instant"}).FrontTypeLine(); got != "Instant" {
		t.Errorf("a single-faced type line read as %q", got)
	}
}

// **Colour identity comes from Scryfall's own field** (rule 2), never derived
// from the mana cost -- so the membership test is over that list and nothing
// else.
func TestColourMembershipIsOverScryfallsOwnList(t *testing.T) {
	t.Parallel()
	cost := "{2}{U}"
	rec := &CardRecord{ColorIdentity: []string{"G", "W"}, ManaCost: &cost}

	for _, c := range []string{"G", "W"} {
		if !rec.HasColor(c) {
			t.Errorf("%s is not in %v", c, rec.ColorIdentity)
		}
	}
	// Not in the identity, even though it is in the mana cost -- which is
	// the whole point of rule 2.
	if rec.HasColor("U") {
		t.Error("a colour from the mana cost was read as identity")
	}
	for _, c := range []string{"B", "R", "", "g"} {
		if rec.HasColor(c) {
			t.Errorf("%q read as present in %v", c, rec.ColorIdentity)
		}
	}
	// A colourless card is in no colour at all.
	if (&CardRecord{}).HasColor("G") {
		t.Error("a colourless card has a colour")
	}
}

// The legalities column is JSON, and the driver hands it over as a decoded
// map, as text, or as bytes depending on how the pool stored it. All three
// read, and anything else is an empty map rather than a nil one -- a nil
// would make every legality lookup a miss, which reads as "banned".
func TestLegalitiesReadHoweverTheDriverHandsThemOver(t *testing.T) {
	t.Parallel()
	want := map[string]any{"commander": "legal"}

	for _, tc := range []struct {
		name string
		in   any
	}{
		{"a decoded map", map[string]any{"commander": "legal"}},
		{"text", `{"commander":"legal"}`},
		{"bytes", []byte(`{"commander":"legal"}`)},
	} {
		got := legalities(tc.in)
		if got["commander"] != want["commander"] {
			t.Errorf("%s read as %v", tc.name, got)
		}
	}

	// Everything unreadable is an empty map, never nil: a nil map would make
	// every lookup a miss, and a miss reads as "not legal".
	for _, tc := range []struct {
		name string
		in   any
	}{
		{"nothing", nil},
		{"unparseable text", "not json"},
		{"unparseable bytes", []byte("not json")},
		{"a number", 7},
		{"a list", []any{"legal"}},
	} {
		got := legalities(tc.in)
		if got == nil {
			t.Errorf("%s read as a nil map -- every legality would be a miss", tc.name)
		}
		if len(got) != 0 {
			t.Errorf("%s read as %v", tc.name, got)
		}
	}
}

// **The driver's widths are not the schema's.** A reading that only handled
// the width this machine happened to see would return zero on another --
// silently, as a mana value of 0.
func TestEveryNumericWidthTheDriverCanHandBackReads(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   any
		want float64
	}{
		{"float64", float64(3.5), 3.5},
		{"float32", float32(3.5), 3.5},
		{"int32", int32(3), 3},
		{"int64", int64(3), 3},
		{"int", 3, 3},
	} {
		if got := asFloat(tc.in); got != tc.want {
			t.Errorf("asFloat(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
	// Anything else is zero, which for a mana value is the honest answer:
	// there is nothing to read.
	for _, bad := range []any{nil, "3.5", true, []any{3.5}} {
		if got := asFloat(bad); got != 0 {
			t.Errorf("asFloat(%#v) = %v", bad, got)
		}
	}

	for _, tc := range []struct {
		name string
		in   any
		want int
	}{
		{"int32", int32(42), 42},
		{"int64", int64(42), 42},
		{"int", 42, 42},
		{"float64", float64(42.9), 42},
	} {
		got := asIntPtr(tc.in)
		if got == nil {
			t.Errorf("asIntPtr(%s) read as nothing", tc.name)
			continue
		}
		if *got != tc.want {
			t.Errorf("asIntPtr(%s) = %d, want %d", tc.name, *got, tc.want)
		}
	}
	// A NULL column is nothing rather than zero: an unranked card and a
	// card ranked 0 are different facts.
	for _, bad := range []any{nil, "42", true, float32(42)} {
		if got := asIntPtr(bad); got != nil {
			t.Errorf("asIntPtr(%#v) = %d, want nothing", bad, *got)
		}
	}
}

// A VARCHAR[] is never nil: an empty list is `[]` on the wire, and a nil
// would render as `null` on a page that iterates it.
func TestAStringArrayIsNeverNil(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want int
	}{
		{"from the driver", []any{"G", "W"}, 2},
		{"already a slice", []string{"G", "W"}, 2},
		{"an empty list", []any{}, 0},
		{"nothing", nil, 0},
		{"the wrong type", "G", 0},
		// Non-strings inside are dropped rather than failing the row.
		{"a mixed list", []any{"G", 7, nil, "W"}, 2},
	} {
		got := asStrings(tc.in)
		if got == nil {
			t.Errorf("%s read as nil -- the page iterates this", tc.name)
			continue
		}
		if len(got) != tc.want {
			t.Errorf("%s read as %v, want %d entries", tc.name, got, tc.want)
		}
	}

	// The two scalar readings, and their absence.
	if got := asString("x"); got != "x" {
		t.Errorf("asString = %q", got)
	}
	if got := asString(7); got != "" {
		t.Errorf("a non-string read as %q", got)
	}
	if got := asStringPtr("x"); got == nil || *got != "x" {
		t.Errorf("asStringPtr = %v", got)
	}
	if got := asStringPtr(nil); got != nil {
		t.Errorf("nothing read as %q", *got)
	}
	if !asBool(true) || asBool(false) || asBool("true") || asBool(nil) {
		t.Error("the boolean reading takes something other than a bool")
	}
}

// The refresh's own coercions, on the way in. These decide what a row *is*,
// so a wrong answer here is a wrong card in the pool rather than a wrong
// rendering of a right one.
func TestTheRefreshCoercionsWriteNullRatherThanZero(t *testing.T) {
	t.Parallel()

	// A price: a JSON number or a float, and nothing else.
	if got := asDouble(json.Number("1.25")); got != 1.25 {
		t.Errorf("asDouble(json 1.25) = %v", got)
	}
	if got := asDouble(float64(1.25)); got != 1.25 {
		t.Errorf("asDouble(1.25) = %v", got)
	}
	for _, bad := range []any{nil, "1.25", json.Number("not-a-number"), true} {
		if got := asDouble(bad); got != nil {
			t.Errorf("asDouble(%#v) = %v, want NULL", bad, got)
		}
	}

	// A rank: narrowed to int32, because an EDHREC rank fits comfortably.
	if got := asInt32(json.Number("42")); got != int32(42) {
		t.Errorf("asInt32(json 42) = %v", got)
	}
	if got := asInt32(float64(42)); got != int32(42) {
		t.Errorf("asInt32(42.0) = %v", got)
	}
	for _, bad := range []any{nil, "42", json.Number("1.5"), true} {
		if got := asInt32(bad); got != nil {
			t.Errorf("asInt32(%#v) = %v, want NULL", bad, got)
		}
	}

	// A release date, which is only a date if it parses as one.
	got := asDate("2026-08-24")
	when, ok := got.(time.Time)
	if !ok || when.Year() != 2026 || when.Month() != time.August || when.Day() != 24 {
		t.Errorf("asDate read %v", got)
	}
	for _, bad := range []any{nil, "", "not-a-date", "24/08/2026", 20260824} {
		if got := asDate(bad); got != nil {
			t.Errorf("asDate(%#v) = %v, want NULL", bad, got)
		}
	}

	// The TCGplayer id is text or nothing, and **zero is nothing** -- the
	// recorded `str(... or "") or None`, where a falsy id is no id.
	if got := tcgID(json.Number("12345")); got != "12345" {
		t.Errorf("tcgID(12345) = %v", got)
	}
	if got := tcgID("12345"); got != "12345" {
		t.Errorf("tcgID(\"12345\") = %v", got)
	}
	for _, bad := range []any{nil, "", json.Number("0"), json.Number(""), 12345, true} {
		if got := tcgID(bad); got != nil {
			t.Errorf("tcgID(%#v) = %v, want NULL", bad, got)
		}
	}
}

// A sub-document is handed over as the decoded document it already is, and
// anything unencodable falls back to the caller's `empty` rather than to a
// NULL the reader would trip on.
//
// **This test used to assert the bug.** It read `jsonText` back as a
// `string` and checked the text — which is exactly what a JSON column must
// not be given, and the assertion passed for as long as the library was
// empty. What it should have been asking all along is what the value *is*,
// because that is what the Appender writes.
func TestASubDocumentIsHandedOverDecodedOrAsTheGivenEmpty(t *testing.T) {
	t.Parallel()
	doc := map[string]any{"commander": "legal"}
	got := jsonText(doc, map[string]any{})
	if _, isText := got.(string); isText {
		t.Errorf("stored as text %#v -- a JSON column takes a value, and a "+
			"string becomes a JSON string that json_extract_string reads NULL from", got)
	}
	if m, ok := got.(map[string]any); !ok || m["commander"] != "legal" {
		t.Errorf("stored as %#v, want the decoded document", got)
	}
	// The absent value is the caller's, and it is a document too.
	if got := jsonText(nil, map[string]any{}); !reflect.DeepEqual(got, map[string]any{}) {
		t.Errorf("nothing stored as %#v, want the given empty object", got)
	}
	if got := jsonText(nil, []any{}); !reflect.DeepEqual(got, []any{}) {
		t.Errorf("the empty is not the caller's: %#v", got)
	}
	// Unencodable falls back rather than failing the whole row.
	if got := jsonText(make(chan int), map[string]any{}); !reflect.DeepEqual(got, map[string]any{}) {
		t.Errorf("an unencodable value stored as %#v", got)
	}
	// `all_parts`' sibling keeps NULL for absent and passes the rest through.
	if got := jsonOrNull(nil); got != nil {
		t.Errorf("an absent all_parts stored as %#v, want NULL", got)
	}
	if got := jsonOrNull([]any{doc}); reflect.TypeOf(got).Kind() == reflect.String {
		t.Errorf("all_parts stored as text %#v", got)
	}

	// And the row's own boolean, which is strict: only a real true is true.
	if !truthy(true) {
		t.Error("true is not true")
	}
	for _, bad := range []any{false, nil, "true", 1} {
		if truthy(bad) {
			t.Errorf("%#v read as true", bad)
		}
	}

	// A list column drops non-strings rather than failing the row.
	if got := asList([]any{"a", 7, nil, "b"}); len(got) != 2 {
		t.Errorf("asList read %v", got)
	}
	if got := asList(nil); got == nil || len(got) != 0 {
		t.Errorf("asList(nil) = %#v, want an empty slice", got)
	}
}
