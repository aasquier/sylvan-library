package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The wheel's body reader, which is the one place in the package where an
// **absent** body is not a mistake.
//
// Every other write route requires one; the wheel's is optional, because
// spinning without a seed is the common case and a room that had to post
// `{}` to ask for a fresh card would be a room with a wart in it. That makes
// `readOptionalBody` a near-copy of `readBody` with one branch flipped, and a
// near-copy is exactly the thing that drifts: the shared half was proved
// through `readBody`'s own tests and this half had never been asked anything
// at all.
//
// The refusals are all `422` with the recorded `detail` shape, and what they
// are worth is that a client can tell **which** thing was wrong -- a body
// that is not JSON, a body that is JSON and is not an object, and a body with
// a second document glued to the end of it are three different mistakes with
// three different fixes.

// wheelRaw posts to one deck's wheel with the headers and bytes given, going
// through the handler the way the door reaches it. Separate from `call`
// because `call` sets `application/json` on every non-empty body -- which is
// right, every real client does -- and half of what is below is about what
// happens when a client does not.
func wheelRaw(t *testing.T, a *API, contentType, body string) (int, string) {
	t.Helper()
	target := "/api/decks/local/mono-green-clean/wheel"
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetPathValue("owner", "local")
	req.SetPathValue("slug", "mono-green-clean")
	rec := httptest.NewRecorder()
	a.deckWheel(rec, req)
	return rec.Code, rec.Body.String()
}

// A body that is not JSON -- by its content type, whatever its bytes say --
// is `dict_type`, and the **input is the raw text**, so the person reading
// the refusal sees what they actually sent.
func TestABodyWithoutAJSONContentTypeIsRefusedWithItsOwnBytes(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	for _, ct := range []string{
		"",                                  // none at all
		"text/plain",                        // the curl default
		"application/x-www-form-urlencoded", // a form post
		"text/json",                         // close, and not it: the type must be `application`
		"application",                       // no subtype
		"application/json; charset",         // unparseable parameters
	} {
		status, body := wheelRaw(t, a, ct, `{"seed":7}`)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("content type %q -> %d: %s", ct, status, body)
			continue
		}
		if !strings.Contains(body, `"type":"dict_type"`) {
			t.Errorf("content type %q -> %s", ct, body)
		}
		// The bytes themselves come back as the input, which is what makes
		// this refusal actionable rather than mysterious.
		if !strings.Contains(body, `"input":"{\"seed\":7}"`) {
			t.Errorf("content type %q did not hand the body back: %s", ct, body)
		}
	}
	// And the two spellings that ARE JSON go through, so this is a working
	// discriminator rather than a blanket refusal.
	for _, ct := range []string{"application/json", "application/json; charset=utf-8",
		"application/vnd.api+json"} {
		if status, body := wheelRaw(t, a, ct, `{"seed":7}`); status != http.StatusOK {
			t.Errorf("content type %q -> %d: %s", ct, status, body)
		}
	}
}

// JSON that is well-formed and is not an object is `dict_type` with the
// **parsed value** as the input -- a list stays a list, a number stays a
// number. The branch above hands back a string; this one does not, and the
// difference is how a client tells "you sent me text" from "you sent me the
// wrong JSON".
func TestJSONThatIsNotAnObjectIsRefusedWithTheValueItParsedTo(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	for _, row := range []struct{ body, input string }{
		{`[1,2]`, `"input":[1,2]`},
		{`7`, `"input":7`},
		{`"seed"`, `"input":"seed"`},
		{`true`, `"input":true`},
	} {
		status, got := wheelRaw(t, a, "application/json", row.body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s -> %d: %s", row.body, status, got)
			continue
		}
		if !strings.Contains(got, `"type":"dict_type"`) || !strings.Contains(got, row.input) {
			t.Errorf("%s -> %s, want dict_type carrying %s", row.body, got, row.input)
		}
	}
}

// An **empty** body and a literal `null` are both the empty mapping: no seed,
// a fresh spin, 200. This is the one branch that differs from `readBody`, and
// it is the reason the file exists.
func TestAnEmptyOrNullBodyIsAFreshSpinRatherThanARefusal(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	for _, row := range []struct{ ct, body string }{
		{"", ""},                        // nothing at all, no header
		{"application/json", ""},        // the header, and still nothing
		{"application/json", "null"},    // the JSON null
		{"application/json", "  null "}, // ...with whitespace around it
		{"application/json", "{}"},      // and the empty object it stands for
	} {
		status, got := wheelRaw(t, a, row.ct, row.body)
		if status != http.StatusOK {
			t.Errorf("body %q (%q) -> %d: %s", row.body, row.ct, status, got)
			continue
		}
		if !strings.Contains(got, `"pool_available":true`) || !strings.Contains(got, `"seed":`) {
			t.Errorf("body %q did not spin: %s", row.body, got)
		}
	}
}

// Two JSON documents in one body is `json_invalid` and names the offset the
// second one starts at -- **not** silently the first document, which is what
// a bare `json.Decode` would do and would mean a client whose retry logic
// double-wrote a payload got a spin it never asked for.
func TestASecondDocumentInTheBodyIsRefusedRatherThanIgnored(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	for _, body := range []string{`{"seed":1}{"seed":2}`, `{} []`, `null null`} {
		status, got := wheelRaw(t, a, "application/json", body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%q -> %d: %s", body, status, got)
			continue
		}
		if !strings.Contains(got, `"type":"json_invalid"`) ||
			!strings.Contains(got, `"Extra data"`) {
			t.Errorf("%q -> %s, want json_invalid/Extra data", body, got)
		}
	}
	// And JSON that will not parse at all is the same family with the
	// parser's own reason, so the two are told apart by `ctx.error`.
	status, got := wheelRaw(t, a, "application/json", `{"seed":`)
	if status != http.StatusUnprocessableEntity || !strings.Contains(got, `"type":"json_invalid"`) {
		t.Errorf("a truncated body -> %d: %s", status, got)
	}
	if strings.Contains(got, "Extra data") {
		t.Errorf("a truncated body read as extra data: %s", got)
	}
}

// A body over the megabyte ceiling is cut at it, and what the cut leaves is
// unparseable rather than a truncated object quietly accepted.
func TestABodyPastTheCeilingIsCutAndTheRemainderIsRefused(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	// One key whose value runs past 1MiB. The reader stops mid-string, so
	// what the decoder sees is an unterminated document.
	body := `{"seed":1,"pad":"` + strings.Repeat("x", 1<<20) + `"}`
	status, got := wheelRaw(t, a, "application/json", body)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a %d-byte body -> %d: %s", len(body), status, got)
	}
	if !strings.Contains(got, `"type":"json_invalid"`) {
		t.Errorf("a body past the ceiling -> %s", got)
	}
}

// A body the request cannot even be read from is `missing` -- the one branch
// no client produces on purpose and every proxy produces eventually, when a
// connection dies between the headers and the body.
func TestABodyThatCannotBeReadIsTheMissingRefusal(t *testing.T) {
	t.Parallel()
	a := wheelAPI(t)
	req := httptest.NewRequest(http.MethodPost,
		"/api/decks/local/mono-green-clean/wheel", failingReader{})
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("owner", "local")
	req.SetPathValue("slug", "mono-green-clean")
	rec := httptest.NewRecorder()
	a.deckWheel(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"type":"missing"`) {
		t.Errorf("a body that would not read -> %s", rec.Body)
	}
}

// failingReader is a connection that drops after the headers.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("the connection went") }

// A pool that opens and then cannot answer is a 500 with a sentence -- not a
// 200 carrying a spin built from nothing, and not the degraded
// `pool_available: false`, which would tell somebody their library is merely
// empty when it is actually broken.
func TestTheWheelOnAPoolThatCannotAnswerSaysSoRatherThanSpinning(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: schemalessPool(t), DecksDir: decksDir(t)})
	status, body, raw := call(t, a, http.MethodPost,
		"/api/decks/local/mono-green-clean/wheel", "")
	if status == http.StatusOK {
		t.Fatalf("a pool that cannot answer spun anyway: %s", raw)
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("%d: %s", status, raw)
	}
	if detail, _ := body["detail"].(string); detail == "" {
		t.Errorf("the refusal said nothing: %s", raw)
	}
	// And specifically not the degraded shape, which is the lie.
	if strings.Contains(string(raw), "pool_available") {
		t.Errorf("a broken pool answered in the no-pool shape: %s", raw)
	}
}

// ADR 5, on the route's other early return: somebody else's private deck is
// 404 -- and it is the **same** 404 a deck that was never there gets, so the
// wheel cannot be used to ask whether a deck exists.
//
// The wheel is worth asking this of specifically. It is a POST, so it reads
// as a write and is easy to leave off an isolation sweep; and it is the one
// route on a deck that anybody may reach for on a shared deck, so the line
// between "shared" and "yours" is doing real work here rather than being a
// formality.
func TestTheWheelOfAPrivateDeckIsTheSame404AsADeckThatIsNotThere(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	// Alice is the maintainer and an admin; bob's private deck is still not
	// hers to spin, and neither is a deck nobody has ever made.
	private, _, privateRaw := callAs(t, a, alice, http.MethodPost,
		"/api/decks/bob/bobs-private/wheel", "")
	absent, _, absentRaw := callAs(t, a, alice, http.MethodPost,
		"/api/decks/bob/never-existed/wheel", "")
	if private != http.StatusNotFound {
		t.Fatalf("bob's private deck answered %d: %s", private, privateRaw)
	}
	if absent != http.StatusNotFound {
		t.Fatalf("an absent deck answered %d: %s", absent, absentRaw)
	}
	// The two bodies differ only where the caller's own slug is echoed back,
	// which carries nothing the caller did not already type. Substitute it
	// and they must be the same bytes -- a "you may not see this one" that
	// read differently from "there is no such one" is exactly how an owner
	// segment becomes a way to enumerate somebody's shelf.
	normalised := strings.Replace(string(privateRaw), "bobs-private", "never-existed", 1)
	if normalised != string(absentRaw) {
		t.Errorf("a private deck and an absent one answer differently, so the "+
			"route says which decks exist:\n private %s\n absent  %s", privateRaw, absentRaw)
	}
	// And neither says anything about permission, which would be the same
	// leak wearing politer words. Read off the normalised body, because the
	// fixture's own slug is the word `private` and the caller typed it.
	for _, raw := range []string{normalised, string(absentRaw)} {
		for _, word := range []string{"permission", "allowed", "forbidden", "private", "shared"} {
			if strings.Contains(strings.ToLower(raw), word) {
				t.Errorf("the refusal said %q: %s", word, raw)
			}
		}
	}
}

// ...and bob's own shared deck IS spinnable by alice, so the test above is a
// working distinction rather than a route that 404s everything.
func TestASharedDecksWheelSpinsForSomebodyElse(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, _, raw := callAs(t, a, alice, http.MethodPost,
		"/api/decks/bob/bobs-public/wheel", "")
	if status != http.StatusOK {
		t.Fatalf("bob's shared deck answered %d: %s", status, raw)
	}
	if !strings.Contains(string(raw), `"seed":`) {
		t.Errorf("no spin came back: %s", raw)
	}
}
