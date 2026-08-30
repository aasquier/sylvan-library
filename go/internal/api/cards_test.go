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

// The typeahead behind "add a card". The tiers are `cards.Suggest`'s and are
// tested there; what this asks is the half the browser depends on -- that
// every row arrives whole, that the painting arrives with its painter, and
// that the two things the write path would otherwise only say *after* a
// rationale has been written are on the row before it.
func TestSuggestAnswersWholeRowsWithTheirPainterNamed(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	status, body, raw := call(t, a, "GET", "/api/cards/suggest?q=Sol+Rng", "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	rows := body["cards"].([]any)
	if len(rows) != 1 {
		t.Fatalf("a misspelled name offered %d rows: %s", len(rows), raw)
	}
	row := rows[0].(map[string]any)
	for _, key := range []string{"name", "mana_cost", "type_line", "oracle_text",
		"color_identity", "image", "artist", "legal_commander", "is_land", "score", "via"} {
		if _, ok := row[key]; !ok {
			t.Errorf("an offered card lacks %q: %s", key, raw)
		}
	}
	if row["name"] != "Sol Ring" || row["via"] != "near" {
		t.Fatalf("Sol Rng: %v", row)
	}
	// **The painting and the painter travel together.** ADR 6 hot-links the
	// image; ADR 32 and commandment 9 owe the artist a credit wherever it
	// renders, and a row that carried a picture with no name to put under it
	// would make that credit impossible in the browser.
	if row["image"] == nil || row["artist"] == nil {
		t.Fatalf("a picture with nobody credited for it: %v", row)
	}
	if !strings.HasPrefix(row["image"].(string), "https://") {
		t.Fatalf("the image is not a hot-link: %v", row["image"])
	}
	// A banned card is offered and *marked*, never hidden -- hiding it is
	// indistinguishable from it not existing, and search (which does filter)
	// is exactly where that goes wrong today.
	_, banned, rawBanned := call(t, a, "GET", "/api/cards/suggest?q=Black+Lotus", "")
	lotus := banned["cards"].([]any)
	if len(lotus) == 0 || lotus[0].(map[string]any)["name"] != "Black Lotus" {
		t.Fatalf("a banned card was hidden: %s", rawBanned)
	}
	if lotus[0].(map[string]any)["legal_commander"] != false {
		t.Fatalf("a banned card was not marked: %s", rawBanned)
	}
	// `is_land` is the one category a card pool fact can fill, and it is the
	// importer's own rule rather than a second one.
	_, lands, _ := call(t, a, "GET", "/api/cards/suggest?q=Llanowar", "")
	if lands["cards"].([]any)[0].(map[string]any)["is_land"] != true {
		t.Fatalf("Llanowar Reborn is a land: %v", lands)
	}
	_, notLand, _ := call(t, a, "GET", "/api/cards/suggest?q=Sol+Ring", "")
	if notLand["cards"].([]any)[0].(map[string]any)["is_land"] != false {
		t.Fatalf("Sol Ring is not a land: %v", notLand)
	}
	// Nothing like it is an empty list, never null -- the browser iterates it.
	if _, _, empty := call(t, a, "GET", "/api/cards/suggest?q=zzzz-no-such-card", ""); string(empty) != `{"cards":[]}` {
		t.Fatalf("nothing: %s", empty)
	}
	// And the limit is bounded the way every other route's is.
	for _, target := range []string{"/api/cards/suggest?limit=abc", "/api/cards/suggest?limit=0",
		"/api/cards/suggest?limit=21"} {
		if status, _, raw := call(t, a, "GET", target, ""); status != 422 {
			t.Errorf("%s: %d %s", target, status, raw)
		}
	}
	if _, few, _ := call(t, a, "GET", "/api/cards/suggest?q=e&limit=2", ""); len(few["cards"].([]any)) != 2 {
		t.Fatalf("limit=2: %v", few)
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
	// Every field the detail owes a reader, `creed` and `sigil` included: both
	// pass through untouched by the pool lookup and both are `null` on most of
	// the 32, which is a shape a renderer branches on -- and a shape a payload
	// that silently stopped carrying the key would break without failing.
	for _, key := range []string{"key", "name", "tier", "colors", "size", "tagline", "history", "lore",
		"aliases", "verified_by", "creed", "sigil", "pool", "champions", "signature", "dropped", "exact_total"} {
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

// The import page's commander box, answered while somebody types in it.
//
// The four states are the contract, because the border reads `state` and
// nothing else: a green box over a card that cannot lead is the bug this route
// exists to prevent.
func TestTheCommanderBoxAnswersTheFieldRatherThanTheName(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	for _, tc := range []struct {
		name, typed, state string
		says               []string
		why                string
	}{
		{
			name: "nothing typed", typed: "", state: "blank",
			why: "a blank box is the documented way to use the list's own commander",
		},
		{
			name: "a legendary creature", typed: "Gyome, Master Chef", state: "ready",
			says: []string{"Gyome, Master Chef", "can lead this deck"},
			why:  "the comma is part of the name, and one lookup settles it",
		},
		{
			name: "the same name in the wrong case", typed: "gyome, master chef", state: "ready",
			says: []string{"Gyome, Master Chef"},
			why:  "the pool resolves casing, so a person typing in lower case is not wrong",
		},
		{
			name: "a card that is not a commander", typed: "Sol Ring", state: "trouble",
			says: []string{"Sol Ring", "cannot sit in the command zone", "legendary creature"},
			why: "`legal_commander` is true of Sol Ring, so the format's answer " +
				"would have lit this box green -- the rules question is a different one",
		},
		{
			name: "a legend that is banned", typed: "Emrakul, the Aeons Torn", state: "trouble",
			says: []string{"Emrakul, the Aeons Torn", "not legal in Commander"},
			why:  "it may sit in the command zone and still may not lead a deck",
		},
		{
			name: "two legends that do not pair", typed: "Gyome, Master Chef + Goreclaw, Terror of Qal Sisma",
			state: "trouble",
			says:  []string{"cannot lead together", "pairing ability"},
			why:   "a pair is a third fact again, and the gate's own sentence says which",
		},
		{
			name: "a misspelling", typed: "Gyome, Master Cheff", state: "unknown",
			says: []string{"No card here is called", "Gyome, Master Cheff"},
			why:  "a wrong name gets help, and the shortlist is the help",
		},
		{
			name: "a double-faced card by its front face", typed: "Etali, Primal Conqueror",
			state: "ready", says: []string{"Etali, Primal Conqueror can lead this deck"},
			why: "the library writes face names, so answering with the combined " +
				"name would be this box disagreeing with the import below it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, body, raw := call(t, a, "GET",
				"/api/cards/commander?q="+url.QueryEscape(tc.typed), "")
			if status != 200 {
				t.Fatalf("%d %s", status, raw)
			}
			if body["state"] != tc.state {
				t.Fatalf("state is %v, wanted %q (%s): %s",
					body["state"], tc.state, tc.why, raw)
			}
			sentence, _ := body["sentence"].(string)
			for _, want := range tc.says {
				if !strings.Contains(sentence, want) {
					t.Errorf("the sentence does not say %q (%s): %q", want, tc.why, sentence)
				}
			}
			// Every state answers with both lists present, so the browser never
			// reads a null where it expects to iterate.
			for _, key := range []string{"commanders", "did_you_mean"} {
				if _, ok := body[key].([]any); !ok {
					t.Errorf("%q is not a list: %s", key, raw)
				}
			}
		})
	}
}

// The shortlist offers cards that cannot lead, marked, rather than hiding
// them -- the same argument the typeahead records, and for the same reason: a
// card hidden from the list is indistinguishable from a card that does not
// exist. Commanders come first so the marking costs nobody a scroll.
func TestTheCommanderBoxOffersNearNamesWithTheOnesThatCanLeadFirst(t *testing.T) {
	t.Parallel()
	a := New(Config{Pool: pooltest.Open(t)})
	_, body, raw := call(t, a, "GET", "/api/cards/commander?q=Sol+Rng", "")
	offers := body["did_you_mean"].([]any)
	if len(offers) != 1 {
		t.Fatalf("%d offers for one unknown name: %s", len(offers), raw)
	}
	offer := offers[0].(map[string]any)
	if offer["written"] != "Sol Rng" {
		t.Fatalf("the offer does not say what was written: %s", raw)
	}
	rows := offer["candidates"].([]any)
	if len(rows) == 0 {
		t.Fatalf("a misspelling offered nothing: %s", raw)
	}
	row := rows[0].(map[string]any)
	for _, key := range []string{"name", "mana_cost", "type_line", "color_identity",
		"may_command", "legal_commander", "pairing", "score"} {
		if _, ok := row[key]; !ok {
			t.Errorf("a candidate lacks %q: %s", key, raw)
		}
	}
	// Sol Ring is offered, and marked as unable to lead rather than dropped.
	if row["name"] != "Sol Ring" {
		t.Fatalf("Sol Rng did not offer Sol Ring: %s", raw)
	}
	if row["may_command"] != false || row["legal_commander"] != true {
		t.Fatalf("Sol Ring is legal in Commander and cannot lead a deck; the row "+
			"says otherwise: %v", row)
	}
	// A picture is deliberately absent: this strip sits under a text field, and
	// a card image owes its painter a credit in the room it renders in.
	for _, key := range []string{"image", "art_crop", "artist"} {
		if _, ok := row[key]; ok {
			t.Errorf("the field's answer carries %q, which owes a credit it has "+
				"nowhere to print: %s", key, raw)
		}
	}
	// The ones that can lead sort ahead of the ones that cannot.
	_, near, nearRaw := call(t, a, "GET", "/api/cards/commander?q=Gyome+Master+Che", "")
	for _, o := range near["did_you_mean"].([]any) {
		seen := false
		for _, c := range o.(map[string]any)["candidates"].([]any) {
			row := c.(map[string]any)
			leads := row["may_command"] == true && row["legal_commander"] == true
			if !leads {
				seen = true
				continue
			}
			if seen {
				t.Fatalf("%v can lead and is listed below one that cannot: %s",
					row["name"], nearRaw)
			}
		}
	}
}

func TestWithoutAPoolTheAnswersDegradeToTheRecordedShapes(t *testing.T) {
	t.Parallel()
	a := New(Config{})
	// The commander box says nothing at all rather than `unknown`: a red box
	// over a correctly-typed commander is worse than no box.
	if _, body, raw := call(t, a, "GET", "/api/cards/commander?q=Gyome", ""); body["message"] == nil ||
		body["state"] != "blank" || len(body["commanders"].([]any)) != 0 {
		t.Fatalf("commander: %s", raw)
	}
	if _, body, _ := call(t, a, "GET", "/api/cards/search?q=sol", ""); body["total"] != float64(0) || body["message"] == nil {
		t.Fatalf("search: %v", body)
	}
	if _, body, _ := call(t, a, "POST", "/api/cards/identify", `{"sightings": []}`); body["message"] == nil {
		t.Fatalf("identify: %v", body)
	}
	if _, body, _ := call(t, a, "GET", "/api/cards/suggest?q=sol", ""); body["message"] == nil ||
		len(body["cards"].([]any)) != 0 {
		t.Fatalf("suggest: %v", body)
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

// mustJSON is a sentence as it appears on the wire, quoting and escaping
// included, so a test that pins a whole response body can hold the *shape*
// exactly while reading the *words* off the constant that owns them. A
// player-facing sentence copied into a test literal is two sources of truth
// for one string, and the copy is the one that goes stale silently.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
