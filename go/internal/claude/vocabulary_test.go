package claude

import (
	"encoding/json"
	"testing"
)

// The four little readers this package's refusals and payloads are spelled
// with: what a decoded value is *called*, how it is *quoted*, how it is
// *rendered* and whether it is *true*.
//
// None of them is exported, none is more than a type switch, and all four
// reach a person: `typeName` and `literalAny` are how a 422 says
// `'7.5' is not an initiative level`, `plain` is how a `media_type` that
// arrived as a list gets named back in the refusal, and `truthy` decides
// whether a falsy stance counts as no stance at all.
//
// They were covered only through the arms the corpora happen to carry, which
// is most of the JSON vocabulary and none of the Go one -- so a `float64`
// reaching `plain` from a decoder that forgot `UseNumber`, or an `int` from a
// Go caller, went through branches nothing had ever run. The gap matters in
// one direction: every one of these has a `default` arm that answers
// *something*, so a missing case does not fail, it renders a sentence with
// Go's own spelling in it where the recorded one was meant to be.

// everyShape is one value of each arm all four readers switch on, including
// the Go-side ones no decoder produces. `json.Number` appears three times
// because its own arm branches on the literal rather than on the value.
type shapeRow struct {
	note     string
	value    any
	typeName string
	literal  string
	plain    string
	truthy   bool
}

func everyShape() []shapeRow {
	return []shapeRow{
		{"null", nil, "NoneType", "None", "None", false},
		{"true", true, "bool", "True", "True", true},
		{"false", false, "bool", "False", "False", false},
		{"an integer literal", json.Number("7"), "int", "7", "7", true},
		{"a float literal", json.Number("7.5"), "float", "7.5", "7.5", true},
		{"an exponent literal", json.Number("7e2"), "float", "7e2", "7e2", true},
		{"a zero literal", json.Number("0"), "int", "0", "0", false},
		{"an unreadable number literal", json.Number("xyz"), "int", "xyz", "xyz", false},
		{"a float64 from a decoder with no UseNumber", 7.5, "float", "7.5", "7.5", true},
		{"a zero float64", 0.0, "float", "0", "0", false},
		{"a Go int", 7, "int", "7", "7", true},
		{"a Go int64", int64(7), "int", "7", "7", true},
		{"a string", "rethink", "str", "'rethink'", "rethink", true},
		{"an empty string", "", "str", "''", "", false},
		{"a list", []any{"a", nil}, "list", "[a <nil>]", "['a', None]", true},
		{"an empty list", []any{}, "list", "[]", "[]", false},
		{"an object", map[string]any{"write": "applies"}, "dict",
			"map[write:applies]", "{'write': 'applies'}", true},
		{"an empty object", map[string]any{}, "dict", "map[]", "{}", false},
		// The arm every one of these switches falls off the end into. A Go
		// value no JSON decoder makes, which is what a caller inside this
		// process can still hand in.
		{"something none of them has a case for", struct{ X int }{3}, "object",
			"{3}", "{3}", true},
	}
}

// `typeName` is the vocabulary a 422 names a bad value's *kind* in, and it is
// deliberately Python's rather than Go's: the corpus holds `NoneType`, `str`
// and `dict`, not `<nil>`, `string` and `map[string]interface {}`.
func TestTheRefusalVocabularyNamesEveryShapeTheRecordedWay(t *testing.T) {
	t.Parallel()
	for _, row := range everyShape() {
		if got := typeName(row.value); got != row.typeName {
			t.Errorf("%s: typeName is %q, the recorded word is %q",
				row.note, got, row.typeName)
		}
	}
}

// `literalAny` quotes the value itself into the same sentence. Scalars get an
// exact rendering and containers fall through to `%v`, which is the residue
// the corpus deliberately never reaches -- pinned here so that a change to it
// is a decision rather than a surprise.
func TestARefusalQuotesEveryScalarTheRecordedWay(t *testing.T) {
	t.Parallel()
	for _, row := range everyShape() {
		if got := literalAny(row.value); got != row.literal {
			t.Errorf("%s: literalAny is %q, want %q", row.note, got, row.literal)
		}
	}
}

// `plain` is the plain-text read: what a field *says* when it was supposed to
// be a string and was not. `Plain` is the same function, exported for
// `internal/api`'s own boundary refusals -- and the two must not drift, since
// a route and a mode refusing the same body in different words is the shape
// of bug that survives both packages' tests.
func TestThePlainTextReadIsTheSameOneTheRoutesUse(t *testing.T) {
	t.Parallel()
	for _, row := range everyShape() {
		if got := plain(row.value); got != row.plain {
			t.Errorf("%s: plain is %q, want %q", row.note, got, row.plain)
		}
		if got := Plain(row.value); got != row.plain {
			t.Errorf("%s: the exported Plain is %q, want %q", row.note, got, row.plain)
		}
	}
}

// `truthy` decides whether a falsy `stance` in a request body counts as no
// stance at all -- so a `0`, a `""` and an empty object all mean "I did not
// ask", which is what lets a UI send its whole state without turning an
// unset control into a request.
func TestAFalsyValueIsNoAnswerAtAll(t *testing.T) {
	t.Parallel()
	for _, row := range everyShape() {
		if got := truthy(row.value); got != row.truthy {
			t.Errorf("%s: truthy is %v, want %v", row.note, got, row.truthy)
		}
	}
}

// The two entry points into the stance parser that the corpus's JSON rows
// cannot describe, because neither is JSON: a Go nil and a Stance already in
// hand. Both are how `Resolve` is called from inside this process.
func TestAStanceAlreadyInHandIsReadWithoutReparsing(t *testing.T) {
	t.Parallel()
	got, err := StanceFromObj(nil)
	if err != nil {
		t.Fatalf("a nil request: %v", err)
	}
	if got != Off {
		t.Errorf("a nil request read as %+v, want off -- a half-written stance "+
			"may only ever be quieter", got)
	}
	for _, want := range []Stance{Off, Consultant, SecondOpinion, Collaborator} {
		got, err := StanceFromObj(want)
		if err != nil {
			t.Errorf("%+v: %v", want, err)
			continue
		}
		if got != want {
			t.Errorf("a Stance handed in came back as %+v", got)
		}
	}
	// A hand-built Stance is passed through unvalidated on purpose -- the
	// constructors validate, and a caller inside this process that builds one
	// by hand owns it. The clamp is what stops it mattering.
	nonsense := Stance{Initiative: "loud", Scope: "everything", Write: "all"}
	got, err = StanceFromObj(nonsense)
	if err != nil {
		t.Fatalf("a hand-built stance: %v", err)
	}
	if got != nonsense {
		t.Errorf("a hand-built stance came back changed: %+v", got)
	}
	if clamped := Clamp(nonsense, Collaborator); clamped != Off {
		t.Errorf("clamping an unreadable stance gave %+v, want off", clamped)
	}
	if clamped := Clamp(Collaborator, nonsense); clamped != Off {
		t.Errorf("clamping to an unreadable ceiling gave %+v, want off", clamped)
	}
}

// Bytes that are not JSON at all are refused by the kind they are, not by the
// decoder's own complaint -- the same sentence a badly typed value gets, so
// one 422 vocabulary covers both.
func TestBytesThatAreNotJSONAreRefusedByTheirKind(t *testing.T) {
	t.Parallel()
	got, err := StanceFromObj(json.RawMessage(`{"write":`))
	if err == nil {
		t.Fatalf("truncated JSON was read as %+v", got)
	}
	const want = "cannot read a stance from object"
	if err.Error() != want {
		t.Errorf("the refusal reads %q, want %q", err, want)
	}
	// And the two shapes that are not a refusal at all: empty bytes and a
	// literal null, both of which mean "no stance" rather than a bad one.
	for _, raw := range []string{"", "   ", "null"} {
		got, err := StanceFromObj(json.RawMessage(raw))
		if err != nil {
			t.Errorf("%q: %v", raw, err)
			continue
		}
		if got != Off {
			t.Errorf("%q read as %+v, want off", raw, got)
		}
	}
}

// A deck object with no status at all is the row `DefaultFor`'s corpus cannot
// carry: nil, rather than a deck whose status string is empty.
func TestADeckThatIsNotThereDefaultsToTheNarrowStance(t *testing.T) {
	t.Parallel()
	if got := DefaultFor(nil); got != Consultant {
		t.Errorf("a nil deck defaults to %+v, want consultant", got)
	}
	// And the asymmetry that makes `DialDefault` a separate function: the
	// same nil deck through `Resolve` is off, because there is no deck to
	// read a default from.
	got, err := Resolve(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != Off {
		t.Errorf("no deck and no request resolved to %+v, want off", got)
	}
}
