package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The write surface's guards, as opposed to its operations: what a body has
// to look like before an edit is even attempted, and the two checks that need
// the card pool to answer (rule 1 -- a card nobody looked up is a card whose
// legality is a guess).
//
// These are the paths a browser reaches by accident and a script reaches on
// purpose, and every one of them has to refuse without writing. A guard that
// let a malformed body through would not fail loudly: it would write
// something slightly wrong into the file that is the truth.

// A printing id is checked against the pool because only a query can tell
// whether an id that *looks* like a printing is a printing **of this card**.
func TestAPrintingMustBeAPrintingOfTheCardItIsSetOn(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	ctx := context.Background()

	// A well-formed id that belongs to no card at all.
	err := rig.api.checkPrintingOf(ctx, "Sol Ring",
		"00000000-0000-0000-0000-000000000000", "the hint rides along.")
	if err == nil {
		t.Fatal("an id belonging to nothing was accepted")
	}
	if !strings.Contains(err.Error(), "is not a printing of Sol Ring") {
		t.Errorf("the refusal said %q", err)
	}
	if !strings.Contains(err.Error(), "the hint rides along.") {
		t.Errorf("the hint did not reach the caller: %q", err)
	}
}

// An instance with no card pool cannot answer the printing question, and a
// check that cannot be made must not become a refusal: the deck is the truth
// and a laptop with no pool still edits.
func TestThePrintingCheckIsSkippedWithoutACardPool(t *testing.T) {
	t.Parallel()
	a := New(Config{DecksDir: t.TempDir()})
	err := a.checkPrintingOf(context.Background(), "Llanowar Elves",
		"00000000-0000-0000-0000-000000000000", "hint")
	if err != nil {
		t.Errorf("no pool refused the edit: %v", err)
	}
}

// Setting a card's art through the pool-backed rig: a bad id is refused
// before anything is written, and the deck on disk is untouched.
func TestABadArtIdIsRefusedBeforeTheDeckIsWritten(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)

	status, payload, raw := rig.do(t, alice, "PATCH", cleanDeck+"/cards/Sol%20Ring",
		`{"field":"art","value":"00000000-0000-0000-0000-000000000000"}`)
	if status != 422 {
		t.Fatalf("a bad printing answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "is not a printing of") {
		t.Errorf("the refusal said %q", detail)
	}
	if rig.text(t) != before {
		t.Error("the deck was written despite the refusal")
	}
}

// Clearing the art is a real edit rather than a check: an empty value has no
// id to verify, so it must not be sent to the pool and must not be refused.
func TestClearingTheArtSkipsThePrintingCheck(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, _, raw := rig.do(t, alice, "PATCH", cleanDeck+"/cards/Sol%20Ring",
		`{"field":"art","value":""}`)
	if status != 200 {
		t.Fatalf("clearing the art answered %d: %s", status, raw)
	}
}

// A deck with no commander has no art to set, and says so rather than
// reaching for `d.Commander[0]`.
func TestSettingCommanderArtOnACommanderlessDeckIsRefused(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// `draft` is the fixture that is still a pile rather than a deck.
	status, payload, raw := rig.do(t, alice, "PATCH", "/api/decks/alice/draft",
		`{"field":"commander_art","value":"00000000-0000-0000-0000-000000000000"}`)
	if status == 200 {
		t.Fatalf("a commanderless deck accepted commander art: %s", raw)
	}
	if status == 422 {
		if detail := fmtDetail(payload); detail != "" &&
			!strings.Contains(detail, "no commander") &&
			!strings.Contains(detail, "not a printing") {
			t.Errorf("the refusal said %q", detail)
		}
	}
}

// A card that is not in the deck cannot be patched, and the 99 and the swap
// board are both searched case-insensitively -- while the graveyard
// deliberately is not, because an entombed card is frozen.
func TestFindCardSearchesTheNinetyNineAndTheSwapBoardOnly(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{
		Cards:     []deck.CardEntry{{Name: "Llanowar Elves"}},
		SwapBoard: []deck.CardEntry{{Name: "Birds of Paradise"}},
		Graveyard: []deck.CardEntry{{Name: "Sol Ring"}},
	}
	for _, name := range []string{"Llanowar Elves", "llanowar elves", "  LLANOWAR ELVES  "} {
		if got := findCard(d, name); got == nil || got.Name != "Llanowar Elves" {
			t.Errorf("%q found %v -- the search is case-folded and trimmed", name, got)
		}
	}
	if got := findCard(d, "birds of paradise"); got == nil {
		t.Error("the swap board is not searched")
	}
	if got := findCard(d, "Sol Ring"); got != nil {
		t.Error("the graveyard was searched -- an entombed card is frozen")
	}
	if got := findCard(d, "Nothing At All"); got != nil {
		t.Errorf("a card that is not there found %v", got)
	}

	// The entry is the deck's own, so a caller edits the deck rather than a
	// copy of it.
	found := findCard(d, "Llanowar Elves")
	found.Name = "changed"
	if d.Cards[0].Name != "changed" {
		t.Error("findCard handed back a copy")
	}
}

// Patching a card that is not in the deck is a refusal that names the card,
// because the caller's next move is to check the spelling.
func TestPatchingACardThatIsNotThereNamesIt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "PATCH", cleanDeck+"/cards/Black%20Lotus",
		`{"field":"category","value":"ramp"}`)
	if status != 422 {
		t.Fatalf("patching an absent card answered %d: %s", status, raw)
	}
	if detail := fmtDetail(payload); !strings.Contains(detail, "Black Lotus") {
		t.Errorf("the refusal did not name the card: %q", detail)
	}
}

// The category list is fixed, and an edit through this path is a choice from
// a list the caller was shown -- so a typo is refused here even though the
// gate only warns about one in a hand-written file.
func TestAnUnknownCategoryIsRefusedWithTheListOfRealOnes(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, _ := rig.do(t, alice, "PATCH", cleanDeck+"/cards/Sol%20Ring",
		`{"field":"category","value":"rampp"}`)
	if status != 422 {
		t.Fatalf("a mistyped category answered %d", status)
	}
	detail := fmtDetail(payload)
	if !strings.Contains(detail, "is not a category") {
		t.Errorf("the refusal said %q", detail)
	}
	if !strings.Contains(detail, "ramp") {
		t.Errorf("the refusal did not offer the real list: %q", detail)
	}

	// The check folds case and trims, so the same choice spelled loudly is
	// still the choice.
	if got, err := checkCategory("  RAMP  "); err != nil || got != "ramp" {
		t.Errorf("a loud category came back as %q (%v)", got, err)
	}
}

// `value` is the one key whose absence cannot be told from a deliberate
// blank, so it is required before the editor sees the body -- while `field`
// is deliberately allowed through, because the editor is where the reason a
// field cannot be written lives.
func TestAPatchNeedsAValueButAnUnknownFieldReachesTheEditor(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	for _, target := range []string{cleanDeck, cleanDeck + "/cards/Sol%20Ring"} {
		status, payload, _ := rig.do(t, alice, "PATCH", target, `{"field":"stage"}`)
		if status != 422 {
			t.Errorf("%s: a body with no value answered %d", target, status)
		}
		if detail := fmtDetail(payload); !strings.Contains(detail, "value is required") {
			t.Errorf("%s: the refusal said %q", target, detail)
		}
	}

	// A field the editor will not write still reaches it, so the caller
	// learns *why* rather than "no field given".
	status, payload, _ := rig.do(t, alice, "PATCH", cleanDeck,
		`{"field":"archetype","value":"aggro"}`)
	if status == 200 {
		t.Skip("archetype is settable on this build")
	}
	if detail := fmtDetail(payload); detail == "" || strings.Contains(detail, "no field given") {
		t.Errorf("an unsettable field was refused without a reason: %q", detail)
	}
}

// A `null` value is a deliberate blank rather than an absence, and clearing a
// field is a real edit.
func TestANullValueIsADeliberateBlank(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	field, value, ok := namedField(rec, map[string]any{"field": "pilot", "value": nil})
	if !ok {
		t.Fatalf("an explicit null was refused: %s", rec.Body.String())
	}
	if field != "pilot" || value != nil {
		t.Errorf("read %q = %v", field, value)
	}
}

// plainJSON turns the wire's numbers into the ints and floats the editor's
// own validation expects, all the way down a list -- a `json.Number` reaching
// the editor would be refused by the field that wanted an int.
func TestPlainJSONUnwrapsNumbersRecursively(t *testing.T) {
	t.Parallel()
	if got := plainJSON(json.Number("7")); got != 7 {
		t.Errorf("a whole number became %#v, want the int 7", got)
	}
	if got := plainJSON(json.Number("-3")); got != -3 {
		t.Errorf("a negative whole number became %#v", got)
	}
	// Not whole: it stays a float and is refused by the field that cares.
	if got := plainJSON(json.Number("2.5")); got != 2.5 {
		t.Errorf("a fraction became %#v, want the float 2.5", got)
	}
	// Not a number at all: the string survives rather than becoming zero.
	if got := plainJSON(json.Number("nonsense")); got != "nonsense" {
		t.Errorf("an unparseable number became %#v", got)
	}
	// ADR 37's themes arrive as a list, so the unwrapping has to recurse.
	list, ok := plainJSON([]any{json.Number("1"), "aristocrats",
		[]any{json.Number("2")}}).([]any)
	if !ok || len(list) != 3 {
		t.Fatalf("a list became %#v", list)
	}
	if list[0] != 1 || list[1] != "aristocrats" {
		t.Errorf("the list unwrapped to %#v", list)
	}
	inner, ok := list[2].([]any)
	if !ok || inner[0] != 2 {
		t.Errorf("the nested list did not unwrap: %#v", list[2])
	}
	// Everything else passes through untouched.
	for _, v := range []any{"plain", true, nil} {
		if got := plainJSON(v); got != v {
			t.Errorf("%#v became %#v", v, got)
		}
	}
}

// The quantity coercion carries a recorded quirk: zero is falsy, so
// `{"qty": 0}` is one card rather than none. Everything unparseable fails
// with the message the recorded implementation raised.
func TestBodyQtyKeepsTheRecordedCoercion(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		body map[string]any
		want int
	}{
		{"absent", map[string]any{}, 1},
		{"null", map[string]any{"qty": nil}, 1},
		{"zero is falsy", map[string]any{"qty": json.Number("0")}, 1},
		{"one", map[string]any{"qty": json.Number("1")}, 1},
		{"four", map[string]any{"qty": json.Number("4")}, 4},
		{"negative", map[string]any{"qty": json.Number("-2")}, -2},
	} {
		got, err := bodyQty(tc.body)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}

	for _, tc := range []struct {
		name, wants string
		body        map[string]any
	}{
		{"a fraction", "invalid literal for int()",
			map[string]any{"qty": json.Number("1.5")}},
		{"a string", "invalid literal for int()",
			map[string]any{"qty": "four"}},
		{"a bool", "int() argument must be a string or a number",
			map[string]any{"qty": true}},
		{"a list", "int() argument must be a string or a number",
			map[string]any{"qty": []any{}}},
	} {
		_, err := bodyQty(tc.body)
		if err == nil {
			t.Errorf("%s was accepted", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.wants) {
			t.Errorf("%s said %q, want something containing %q", tc.name, err, tc.wants)
		}
	}
}

// The content-type test is the recorded one: `application/json`, anything
// ending `+json`, parameters ignored, and an absent header is not JSON.
func TestTheContentTypeTestIsTheRecordedOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"APPLICATION/JSON", true},
		{"application/merge-patch+json", true},
		{"application/vnd.api+json", true},
		{"", false},
		{"text/json", false},
		{"application/xml", false},
		{"application/jsonx", false},
		{"text/plain; charset=utf-8", false},
		{"application/json; charset=", false},
		{"nonsense", false},
	} {
		if got := isJSONRequest(tc.in); got != tc.want {
			t.Errorf("isJSONRequest(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// The parse position is one-based and comes off whichever error Go's decoder
// raised, so a caller can point at the character.
func TestTheDecodeOffsetIsReadFromWhicheverErrorWasRaised(t *testing.T) {
	t.Parallel()

	var syntax any
	err := json.Unmarshal([]byte(`{"a": }`), &syntax)
	if err == nil {
		t.Fatal("that parsed")
	}
	if got := decodeOffset(err); got < 1 {
		t.Errorf("a syntax error reported offset %d", got)
	}

	var typed struct {
		A int `json:"a"`
	}
	err = json.Unmarshal([]byte(`{"a": "not a number"}`), &typed)
	if err == nil {
		t.Fatal("that parsed")
	}
	if got := decodeOffset(err); got < 1 {
		t.Errorf("a type error reported offset %d", got)
	}

	// Anything else is position 1 rather than zero -- the offset is
	// one-based, and a zero would point before the document.
	if got := decodeOffset(context.Canceled); got != 1 {
		t.Errorf("an unrelated error reported offset %d, want 1", got)
	}
}

// readBody's refusals, each of which a browser reaches by accident.
func TestReadBodyRefusesEveryShapeThatIsNotAJSONObject(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, contentType, body string
	}{
		{"an empty body", "application/json", ""},
		{"the wrong content type", "text/plain", `{"a":1}`},
		{"no content type at all", "", `{"a":1}`},
		{"malformed JSON", "application/json", `{"a":`},
		{"a bare null", "application/json", `null`},
		{"a list", "application/json", `[1,2]`},
		{"a bare string", "application/json", `"hello"`},
		{"a bare number", "application/json", `7`},
		{"two documents", "application/json", `{"a":1} {"b":2}`},
	} {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(tc.body))
		if tc.contentType != "" {
			req.Header.Set("Content-Type", tc.contentType)
		}
		rec := httptest.NewRecorder()
		if _, ok := readBody(rec, req); ok {
			t.Errorf("%s was accepted", tc.name)
			continue
		}
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d, want 422", tc.name, rec.Code)
		}
	}

	// And the shape that is right.
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"qty": 3}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	body, ok := readBody(rec, req)
	if !ok {
		t.Fatalf("a good body was refused: %s", rec.Body.String())
	}
	// Numbers survive as json.Number, which is what lets `plainJSON` tell
	// `3` from `3.0`.
	if _, isNumber := body["qty"].(json.Number); !isNumber {
		t.Errorf("qty decoded as %T, not json.Number", body["qty"])
	}
}

// `str` is the coercion four route families apply before a value becomes a
// slug, an owner or a card name -- and its stringification is deliberately
// not fmt.Sprint, because the difference lands in a 404's detail verbatim.
func TestStrUsesTheRecordedStringification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, key, want string
		body            map[string]any
	}{
		{"absent", "slug", "", map[string]any{}},
		{"null", "slug", "", map[string]any{"slug": nil}},
		{"a string", "slug", "gyome", map[string]any{"slug": "gyome"}},
		{"a list", "slug", wire.Plain([]any{"x"}), map[string]any{"slug": []any{"x"}}},
	} {
		if got := str(tc.body, tc.key); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
