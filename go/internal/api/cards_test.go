package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// call runs one route by method and path, with an optional body,
// and decodes the JSON answer. The route is chosen the way the door chooses
// it -- segments matched literal for literal, a parameter capturing one,
// the most specific pattern first -- so a test reaches the handler a real
// request would.
func call(t *testing.T, a *API, method, target, body string) (int, map[string]any, []byte) {
	t.Helper()
	return callCtx(t, a, context.Background(), method, target, body)
}

// callCtx is call with a caller on the context (the deck tests sign in).
func callCtx(t *testing.T, a *API, ctx context.Context, method, target, body string) (int, map[string]any, []byte) {
	t.Helper()
	// Parsed rather than split on "?", so the path is *decoded* the way the
	// door decodes it: the door matches on `r.URL.Path`, so a card called
	// `Llanowar Reborn` arrives as one segment with a space in it, and a
	// harness that matched the raw form would hand the handler a name no deck
	// contains.
	asked, err := url.Parse(target)
	if err != nil {
		t.Fatalf("bad target %q: %v", target, err)
	}
	path := asked.Path
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	var found *Route
	values := map[string]string{}
	best := -1
	for _, r := range a.Routes() {
		if r.Method != method {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(r.Pattern, "/"), "/")
		if len(segs) != len(parts) {
			continue
		}
		captured := map[string]string{}
		literals := 0
		matched := true
		for i, seg := range segs {
			if strings.HasPrefix(seg, "{") {
				name, suffix, _ := strings.Cut(seg[1:], "}")
				if suffix != "" && (!strings.HasSuffix(parts[i], suffix) || len(parts[i]) == len(suffix)) {
					matched = false
					break
				}
				captured[name] = strings.TrimSuffix(parts[i], suffix)
				continue
			}
			if seg != parts[i] {
				matched = false
				break
			}
			literals++
		}
		if matched && literals > best {
			route := r
			found, values, best = &route, captured, literals
		}
	}
	if found == nil {
		t.Fatalf("no route for %s %s", method, target)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	if body != "" {
		// A body is parsed as JSON only when the content type says so,
		// and the raw bytes stand as a *string* otherwise -- so a
		// request without this header is a `dict_type` refusal,
		// however well-formed the JSON is. Every real client sends it; a rig
		// that did not was asking a question no client asks, and it hid the
		// gap `readBody` had until 2026-08-22.
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range values {
		req.SetPathValue(name, value)
	}
	rec := httptest.NewRecorder()
	found.Handler(rec, req)
	var parsed map[string]any
	if rec.Body.Len() > 0 && json.Valid(rec.Body.Bytes()) {
		_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	}
	return rec.Code, parsed, rec.Body.Bytes()
}

func TestSearchAnswersAsServiceSearchCardsDoes(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	status, body, raw := call(t, a, "GET", "/api/cards/search?q=sol", "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	cards := body["cards"].([]any)
	if len(cards) != 1 || body["total"] != float64(1) {
		t.Fatalf("search sol: %s", raw)
	}
	card := cards[0].(map[string]any)
	for _, key := range []string{"name", "mana_cost", "cmc", "type_line", "oracle_text", "color_identity",
		"edhrec_rank", "image", "art_crop", "reserved", "price_usd"} {
		if _, ok := card[key]; !ok {
			t.Errorf("search card lacks %q", key)
		}
	}
	if card["name"] != "Sol Ring" || card["price_usd"] == nil {
		t.Fatalf("sol ring: %v", card)
	}
	// Key order is the route's.
	if !strings.HasPrefix(string(raw), `{"cards":[{"name":"Sol Ring","mana_cost":"{1}","cmc":1,`) {
		t.Fatalf("order: %.80s", raw)
	}
	// A banned card is invisible to search (it filters to legal), an
	// identity narrows, an exact identity is equality, commanders_only
	// decides with the gate's rule.
	if _, body, _ = call(t, a, "GET", "/api/cards/search?q=primeval+titan", ""); body["total"] != float64(0) {
		t.Fatal("a banned card was searchable")
	}
	_, green, _ := call(t, a, "GET", "/api/cards/search?identity=G&limit=200", "")
	_, exact, _ := call(t, a, "GET", "/api/cards/search?identity=G&identity_exact=true&limit=200", "")
	_, colourless, _ := call(t, a, "GET", "/api/cards/search?identity_exact=1&limit=200", "")
	if green["total"].(float64) <= exact["total"].(float64) || colourless["total"].(float64) < 1 {
		t.Fatalf("identity filters: subset %v exact %v colourless %v", green["total"], exact["total"], colourless["total"])
	}
	for _, c := range colourless["cards"].([]any) {
		if len(c.(map[string]any)["color_identity"].([]any)) != 0 {
			t.Fatalf("exact colourless returned a coloured card: %v", c)
		}
	}
	_, commanders, _ := call(t, a, "GET", "/api/cards/search?commanders_only=yes&limit=200", "")
	for _, c := range commanders["cards"].([]any) {
		name := c.(map[string]any)["name"].(string)
		if name == "Sol Ring" || name == "Forest" {
			t.Fatalf("commanders_only offered %s", name)
		}
	}
	if commanders["total"].(float64) < 3 {
		t.Fatalf("too few commanders: %v", commanders["total"])
	}
	// An empty search is an empty list, not null.
	_, _, raw = call(t, a, "GET", "/api/cards/search?q=zzzz-no-such-card", "")
	if string(raw) != `{"cards":[],"total":0}` {
		t.Fatalf("empty: %s", raw)
	}
	// Sorting, price, cmc, type_line.
	_, byName, _ := call(t, a, "GET", "/api/cards/search?sort=name&type_line=creature&cmc_max=7&limit=200", "")
	names := []string{}
	for _, c := range byName["cards"].([]any) {
		names = append(names, c.(map[string]any)["name"].(string))
		if c.(map[string]any)["cmc"].(float64) > 7 {
			t.Fatal("cmc_max ignored")
		}
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("not by name: %v", names)
		}
	}
	_, cheap, _ := call(t, a, "GET", "/api/cards/search?price_max=0.01&limit=200", "")
	if cheap["total"].(float64) != 0 {
		t.Fatalf("price_max: %v", cheap)
	}
}

func TestSearchRefusesBadParametersWithTheValidationList(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	for _, target := range []string{"/api/cards/search?limit=abc", "/api/cards/search?limit=0",
		"/api/cards/search?limit=201", "/api/cards/search?cmc_max=tall", "/api/cards/search?price_max=x",
		"/api/cards/search?identity_exact=maybe", "/api/cards/search?commanders_only=2"} {
		status, body, raw := call(t, a, "GET", target, "")
		if status != 422 {
			t.Errorf("%s: %d %s", target, status, raw)
			continue
		}
		detail, ok := body["detail"].([]any)
		if !ok || len(detail) != 1 {
			t.Errorf("%s: detail %s", target, raw)
			continue
		}
		first := detail[0].(map[string]any)
		for _, key := range []string{"type", "loc", "msg", "input"} {
			if _, has := first[key]; !has {
				t.Errorf("%s: validation error lacks %q: %s", target, key, raw)
			}
		}
		if loc := first["loc"].([]any); loc[0] != "query" {
			t.Errorf("%s: loc %v", target, loc)
		}
	}
	// Two bad parameters are two errors in one answer, in declaration order.
	_, body, _ := call(t, a, "GET", "/api/cards/search?limit=abc&cmc_max=x", "")
	detail := body["detail"].([]any)
	if len(detail) != 2 || detail[0].(map[string]any)["loc"].([]any)[1] != "cmc_max" {
		t.Fatalf("two errors: %v", detail)
	}
	// The recorded spellings of a bool.
	for _, target := range []string{"/api/cards/search?identity_exact=on", "/api/cards/search?identity_exact=OFF",
		"/api/cards/search?identity_exact=t", "/api/cards/search?identity_exact=No"} {
		if status, _, raw := call(t, a, "GET", target, ""); status != 200 {
			t.Errorf("%s: %d %s", target, status, raw)
		}
	}
}

func TestIdentifyCountsResolvedAndOfferedApart(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	status, body, raw := call(t, a, "POST", "/api/cards/identify",
		`{"sightings": [{"set": "LTC", "number": "284/281"}, {"title": "Sol Rng"}, {"corner": "U0284\nLTCENLIK"}, {}, "not an object"]}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	readings := body["readings"].([]any)
	if len(readings) != 4 {
		t.Fatalf("%d readings (the string entry is skipped): %s", len(readings), raw)
	}
	first := readings[0].(map[string]any)
	if first["via"] != "printing" || first["resolved"].(map[string]any)["name"] != "Sol Ring" ||
		len(first["candidates"].([]any)) != 0 {
		t.Fatalf("corner: %v", first)
	}
	second := readings[1].(map[string]any)
	if second["via"] != "title" || second["resolved"] != nil {
		t.Fatalf("title: %v", second)
	}
	cand := second["candidates"].([]any)[0].(map[string]any)
	if cand["name"] != "Sol Ring" || cand["score"] == nil {
		t.Fatalf("candidate: %v", cand)
	}
	for _, key := range []string{"name", "mana_cost", "type_line", "color_identity", "image", "art_crop", "score"} {
		if _, ok := cand[key]; !ok {
			t.Errorf("candidate lacks %q", key)
		}
	}
	if readings[3].(map[string]any)["via"] != "nothing" {
		t.Fatalf("empty: %v", readings[3])
	}
	if body["resolved"] != float64(2) || body["offered"] != float64(1) || body["unread"] != float64(1) || body["dropped"] != float64(0) {
		t.Fatalf("counts: %s", raw)
	}
	// Refusals: not a list, not an object, not JSON.
	if status, body, _ := call(t, a, "POST", "/api/cards/identify", `{}`); status != 422 ||
		body["detail"] != "sightings must be a list of {set, number, title}" {
		t.Fatalf("not a list: %d %v", status, body)
	}
	if status, body, _ := call(t, a, "POST", "/api/cards/identify", `[]`); status != 422 ||
		body["detail"].([]any)[0].(map[string]any)["type"] != "dict_type" {
		t.Fatalf("not an object: %d %v", status, body)
	}
	if status, body, _ := call(t, a, "POST", "/api/cards/identify", `{`); status != 422 ||
		body["detail"].([]any)[0].(map[string]any)["type"] != "json_invalid" {
		t.Fatalf("not JSON: %d %v", status, body)
	}
}

func TestCombinationResolvesThroughThePoolAndDropsWhatItLacks(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	status, body, raw := call(t, a, "GET", "/api/colors/G", "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["key"] != "G" || body["pool"] != true || body["exact_total"] == nil {
		t.Fatalf("G: %s", raw)
	}
	// The 21-card pool holds none of Mono-Green's champions or signature
	// cards, so every name drops and is counted.
	champions, signature := body["champions"].([]any), body["signature"].([]any)
	if len(champions) != 0 || len(signature) != 0 || body["dropped"].(float64) < 1 {
		t.Fatalf("dropped: %s", raw)
	}
	if body["exact_total"].(float64) < 3 {
		t.Fatalf("exact_total %v; the fixture has several mono-green legal cards", body["exact_total"])
	}
	for _, key := range []string{"key", "name", "tier", "colors", "size", "tagline", "history", "lore",
		"aliases", "verified_by", "pool", "champions", "signature", "dropped", "exact_total"} {
		if _, ok := body[key]; !ok {
			t.Errorf("combination lacks %q", key)
		}
	}
	// Spellings: lower case and reversed order land; a stray C is ignored;
	// anything else is the 404 with the quoted literal in it.
	for _, key := range []string{"gw", "WG", "wgc", "c", "C"} {
		if status, _, raw := call(t, a, "GET", "/api/colors/"+key, ""); status != 200 {
			t.Errorf("%s: %d %s", key, status, raw)
		}
	}
	status, body, _ = call(t, a, "GET", "/api/colors/nope", "")
	if status != 404 || body["detail"] != "no colour combination 'nope'" {
		t.Fatalf("nope: %d %v", status, body)
	}
	if status, body, _ := call(t, a, "GET", "/api/colors/GX", ""); status != 404 || body["detail"] != "no colour combination 'GX'" {
		t.Fatalf("GX: %d %v", status, body)
	}
}

func TestLoreResolvesNamesAndCountsTheDropped(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	status, body, raw := call(t, a, "GET", "/api/lore", "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["pool"] != true || body["dropped"].(float64) < 1 {
		t.Fatalf("lore: pool %v dropped %v", body["pool"], body["dropped"])
	}
	facts := body["facts"].([]any)
	if len(facts) < 30 || len(body["volumes"].([]any)) != 5 {
		t.Fatalf("%d facts", len(facts))
	}
	resolvedAny := false
	for _, f := range facts {
		fact := f.(map[string]any)
		for _, key := range []string{"key", "volume", "fact", "more", "cards", "learn"} {
			if _, ok := fact[key]; !ok {
				t.Fatalf("fact lacks %q", key)
			}
		}
		if fact["cards"] == nil {
			t.Fatal("a fact's cards were null rather than []")
		}
		if len(fact["cards"].([]any)) > 0 {
			resolvedAny = true
		}
	}
	// Black Lotus is in the fixture and on the shelves.
	if !resolvedAny {
		t.Fatal("no fact resolved a card the fixture holds")
	}
}

func TestWithoutAPoolTheAnswersDegradeToTheRecordedShapes(t *testing.T) {
	t.Parallel()
	a := New(Config{})
	if _, body, _ := call(t, a, "GET", "/api/cards/search?q=sol", ""); body["total"] != float64(0) || body["message"] == nil {
		t.Fatalf("search: %v", body)
	}
	if _, body, _ := call(t, a, "POST", "/api/cards/identify", `{"sightings": []}`); body["message"] == nil {
		t.Fatalf("identify: %v", body)
	}
	if _, body, _ := call(t, a, "GET", "/api/colors/G", ""); body["pool"] != false || body["exact_total"] != nil ||
		len(body["champions"].([]any)) != 0 {
		t.Fatalf("colors: %v", body)
	}
	if _, body, _ := call(t, a, "GET", "/api/lore", ""); body["pool"] != false || body["dropped"] != float64(0) {
		t.Fatalf("lore: %v", body)
	}
	// And a route's status is still 200: a missing pool is degraded, not an error.
	if status, _, _ := call(t, a, "GET", "/api/lore", ""); status != http.StatusOK {
		t.Fatal(status)
	}
}
