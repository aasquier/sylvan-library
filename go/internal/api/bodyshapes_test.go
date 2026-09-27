package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// What every write route does with a body that is well-formed JSON and is not
// an object.
//
// `readBody` is one function and sixteen routes call it, and until this file
// existed **not one of those sixteen call sites had ever been asked the
// question** -- the branch was proved once, in `wheelbody_test.go`, against
// the one route that reads its body optionally. That is the shape of gap this
// repo keeps finding: a shared helper is tested where it is written and
// nowhere it is used, so a route that forgets to check the second return value
// reads exactly like one that checks it.
//
// The answer a caller gets matters more here than the status code. `[1,2,3]`
// posted to `/cards` is somebody's client sending a list where the app wants a
// card, and the recorded answer is the validation envelope -- `dict_type`,
// located at `body`, carrying what arrived -- rather than a 500, and rather
// than a 200 over a card called "".
//
// **The list of routes is the one `refusals_test.go` holds equal to the served
// table**, so a write route added tomorrow is asked this question without
// anybody remembering to, plus the six top-level writes that take no owner
// segment and therefore sit outside that sweep's prefix.

// topLevelWrites is every write under `/api/decks` that names no owner: the
// two that make a deck, the crypt's two, and the two master switches. They
// are written out rather than derived because each needs a target that exists
// -- but `TestEveryTopLevelWriteSweptHereIsServed` holds every one of them
// against the route table, so the list cannot name a route the app dropped.
var topLevelWrites = []deckRoute{
	{"POST", "/api/decks", `{"slug":"fresh","commander":"Goreclaw, Terror of Qal Sisma"}`},
	{"POST", "/api/decks/import", `{"slug":"fresh","text":"1 Sol Ring"}`},
	{"POST", "/api/decks/entombed/{id}/return", ""},
	{"DELETE", "/api/decks/entombed", ""},
	{"PUT", "/api/decks/shared", `{"shared":true}`},
	{"PUT", "/api/decks/coliseum-at-night", `{"coliseum_at_night":true}`},
}

// The guard that keeps the list above honest, in the direction that can go
// wrong quietly: a route named here and no longer served makes this file look
// like it sweeps six things when it sweeps five.
func TestEveryTopLevelWriteSweptHereIsServed(t *testing.T) {
	t.Parallel()
	served := map[string]bool{}
	for _, r := range New(Config{}).Routes() {
		served[r.Method+" "+r.Pattern] = true
	}
	for _, route := range topLevelWrites {
		if !served[route.method+" "+route.suffix] {
			t.Errorf("%s %s is swept for a malformed body and is not served -- "+
				"this sweep is smaller than it reads",
				route.method, route.suffix)
		}
	}
}

// bodyTarget fills a served pattern with values that exist, or "" for one
// this sweep cannot ask sensibly.
//
// The deck is a real deck on purpose: a 404 would sweep ADR 5's refusal a
// second time instead of the body check, which is how `failingpool_test.go`
// came to aim every deck route at a slug the fixture library does not have.
func bodyTarget(pattern string) string {
	filled := strings.NewReplacer(
		"{owner}", "alice", "{slug}", "mono-green-clean",
		"{name}", "Forest", "{key}", "plan", "{id}", "no-such-handle",
		"{job_id}", "no-such-job", "{code}", "G",
	).Replace(pattern)
	if strings.Contains(filled, "{") {
		return "" // an oracle id, an effect, a printing: nothing to fill
	}
	return filled
}

// A body that is valid JSON and is not an object is refused in the recorded
// validation shape, by every route that reads one.
//
// **The list is the served route table itself**, so a route mounted tomorrow
// is asked this question without anybody adding it here. The routes that read
// no body at all are not named in a list either: they answer whatever they
// were going to answer, and what this asserts of them is only that they did
// not fall over. The refusals are counted instead, and the floor is what keeps
// a sweep that has stopped reaching `readBody` from passing as a green one.
func TestAWriteBodyThatIsNotAnObjectIsRefusedInTheRecordedShape(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	refused, swept := 0, 0
	for _, route := range rig.api.Routes() {
		if route.Method == http.MethodGet {
			continue
		}
		target := bodyTarget(route.Pattern)
		if target == "" {
			continue
		}
		swept++
		status, payload, raw := rig.do(t, alice, route.Method, target, `[1,2,3]`)
		// 500 is the one forbidden answer. A 503 is not about the body at
		// all -- it is an instance saying it cannot do this kind of work,
		// which is as true of a well-formed request, and this rig has
		// neither a job registry nor a night store.
		if status == http.StatusInternalServerError {
			t.Errorf("%s %s answered 500 for a list where an object was wanted "+
				"-- a caller's malformed body is never the library's fault: %s",
				route.Method, target, raw)
			continue
		}
		if !json.Valid(raw) && len(raw) > 0 {
			t.Errorf("%s %s answered non-JSON: %s", route.Method, target, raw)
			continue
		}
		detail, isList := payload["detail"].([]any)
		if status != http.StatusUnprocessableEntity || !isList || len(detail) == 0 {
			// A route that never reads a body, or one that refused for a
			// reason of its own. It answered something, which is all this
			// sweep asks of it.
			continue
		}
		first, _ := detail[0].(map[string]any)
		if first["type"] != "dict_type" {
			continue
		}
		refused++
		loc, _ := first["loc"].([]any)
		if len(loc) != 1 || loc[0] != "body" {
			t.Errorf("%s %s located the refusal at %v, not at the body",
				route.Method, target, loc)
		}
		if msg, _ := first["msg"].(string); !strings.Contains(msg, "dictionary") {
			t.Errorf("%s %s refused with %q, which does not say what was wanted",
				route.Method, target, msg)
		}
	}
	// A floor rather than an equality, because the number is the app's to
	// change and a sweep that fell to nothing is the failure worth catching.
	if refused < 18 {
		t.Fatalf("only %d of %d write routes refused a non-object body -- this "+
			"sweep has stopped reaching the check it exists for", refused, swept)
	}
}

// The other three readings `readBody` distinguishes, asked of a route that
// reads a required body, because the four answers are a decision procedure and
// a procedure with one case proved is not proved.
//
// The distinctions are not cosmetic. "You sent nothing" and "you sent
// something I could not parse" send a person to different places, and the
// offset in the third is the only thing that turns "invalid JSON" into a
// column number.
func TestTheFourWaysABodyCanFailAreToldApart(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	for _, row := range []struct {
		name, body, want string
	}{
		{"nothing at all", "", "missing"},
		{"a bare string", `"Sol Ring"`, "dict_type"},
		{"JSON that will not parse", `{"name":`, "json_invalid"},
		{"a null body", `null`, "missing"},
		{"a second document after the first", `{"name":"Sol Ring"} {"name":"Forest"}`, "json_invalid"},
	} {
		status, payload, raw := rig.do(t, alice, "POST",
			cleanDeck+"/cards", row.body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d, not 422: %s", row.name, status, raw)
			continue
		}
		detail, _ := payload["detail"].([]any)
		if len(detail) == 0 {
			t.Errorf("%s answered 422 with no validation list: %s", row.name, raw)
			continue
		}
		first, _ := detail[0].(map[string]any)
		if first["type"] != row.want {
			t.Errorf("%s was read as %v, not %s -- the four failures answer "+
				"alike and a person cannot tell which one they made",
				row.name, first["type"], row.want)
		}
	}
}
