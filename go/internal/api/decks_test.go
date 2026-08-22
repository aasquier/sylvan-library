package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	_ "modernc.org/sqlite"
)

// decksDir writes the gate's fixture decks as a file tier: `<slug>/deck.yaml`
// for each `<name>.yaml` in go/internal/gate/testdata, and one built deck
// with artifacts and a snapshot beside it.
func decksDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("..", "gate", "testdata")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".yaml")
		dir := filepath.Join(root, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		text, _ := os.ReadFile(filepath.Join(src, e.Name()))
		if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), text, 0o644); err != nil {
			t.Fatal(err)
		}
		// `kaheera`, because its YAML `slug:` matches its directory: the
		// artifacts are looked up under the deck's own slug, as
		// `service.list_artifacts` looks them up, and `mono-green-clean`'s
		// file still says `slug: mono-green`.
		if slug == "kaheera" {
			arts := filepath.Join(dir, "artifacts")
			_ = os.MkdirAll(arts, 0o755)
			_ = os.WriteFile(filepath.Join(arts, "primer-quick.md"), []byte("# Quick\n"), 0o644)
			_ = os.WriteFile(filepath.Join(arts, "moxfield.txt"), []byte("1 Sol Ring\n"), 0o644)
			_ = os.WriteFile(filepath.Join(arts, "deck.last-built.yaml"), text, 0o644) // current
			_ = os.WriteFile(filepath.Join(arts, "notes.txt"), []byte("not a deliverable"), 0o644)
		}
	}
	// Scaffolding and the trash are invisible to the shelf.
	_ = os.MkdirAll(filepath.Join(root, "_template"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "_template", "deck.yaml"), []byte("name: T\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".trash", "gone"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".trash", "gone", "deck.yaml"), []byte("name: G\n"), 0o644)
	return root
}

// appDB writes an app.db in Python's shape with alice (the maintainer, by
// email), bob, one of bob's decks shared and one private, a log line for
// the file tier's mono-green, and returns its path.
func appDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ddl := `
CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT NOT NULL UNIQUE COLLATE NOCASE,
  password_hash TEXT, email TEXT UNIQUE COLLATE NOCASE, is_admin INTEGER NOT NULL DEFAULT 0,
  disabled_at TEXT, created_at TEXT NOT NULL);
CREATE TABLE user_decks (id INTEGER PRIMARY KEY AUTOINCREMENT, owner_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  slug TEXT NOT NULL, name TEXT NOT NULL, yaml TEXT NOT NULL, shared INTEGER NOT NULL DEFAULT 0,
  deleted_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE user_deck_artifacts (id INTEGER PRIMARY KEY AUTOINCREMENT, deck_id INTEGER NOT NULL REFERENCES user_decks(id) ON DELETE CASCADE,
  name TEXT NOT NULL, body TEXT NOT NULL, built_at TEXT NOT NULL, UNIQUE(deck_id, name));
CREATE TABLE deck_log (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at TEXT NOT NULL, owner_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
  slug TEXT NOT NULL, actor TEXT, action TEXT NOT NULL, summary TEXT NOT NULL);`
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	now := "2026-08-21T12:00:00+00:00"
	for _, stmt := range []string{
		`INSERT INTO users (id, username, email, is_admin, created_at) VALUES (1, 'alice', 'alice@example.com', 1, '` + now + `')`,
		`INSERT INTO users (id, username, email, is_admin, created_at) VALUES (2, 'bob', 'bob@example.com', 0, '` + now + `')`,
		`INSERT INTO user_decks (id, owner_id, slug, name, yaml, shared, created_at, updated_at) VALUES
		   (10, 2, 'bobs-public', 'Bobs Public', 'name: Bobs Public` + "\n" + `commander: [Gyome, Master Chef]` + "\n" + `stage: draft` + "\n" + `cards:` + "\n" + `  - Sol Ring` + "\n" + `', 1, '` + now + `', '` + now + `'),
		   (11, 2, 'bobs-private', 'Bobs Private', 'name: Bobs Private` + "\n" + `cards: []` + "\n" + `', 0, '` + now + `', '` + now + `'),
		   (12, 2, 'bobs-deleted', 'Gone', 'name: Gone` + "\n" + `', 1, '` + now + `', '` + now + `')`,
		`UPDATE user_decks SET deleted_at = '` + now + `' WHERE id = 12`,
		`INSERT INTO user_deck_artifacts (deck_id, name, body, built_at) VALUES (10, 'moxfield.txt', '1 Sol Ring', '2026-08-21T12:00:00+00:00'),
		   (10, 'deck.last-built.yaml', 'name: Bobs Public` + "\n" + `commander: [Gyome, Master Chef]` + "\n" + `stage: draft` + "\n" + `cards:` + "\n" + `  - Sol Ring` + "\n" + `', '2026-08-21T12:00:00+00:00')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary) VALUES ('` + now + `', NULL, 'mono-green', NULL, 'add', 'added Sol Ring as ramp')`,
		`INSERT INTO deck_log (created_at, owner_id, slug, actor, action, summary) VALUES ('` + now + `', 2, 'bobs-public', 'bob', 'edit', 'edited the deck')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%v\n%s", err, stmt)
		}
	}
	return path
}

func deckAPI(t *testing.T, withAppDB bool) (*API, func()) {
	t.Helper()
	cfg := Config{Pool: pooltest.Open(t), DecksDir: decksDir(t), AdminEmail: "alice@example.com"}
	var db *sql.DB
	if withAppDB {
		var err error
		db, err = auth.Open(appDB(t))
		if err != nil {
			t.Fatal(err)
		}
		cfg.AppDB = db
	}
	return New(cfg), func() {
		if db != nil {
			db.Close()
		}
	}
}

// as runs a GET as a caller.
func as(t *testing.T, a *API, scope auth.Scope, target string) (int, map[string]any, []byte) {
	t.Helper()
	return callAs(t, a, scope, "GET", target, "")
}

func callAs(t *testing.T, a *API, scope auth.Scope, method, target, body string) (int, map[string]any, []byte) {
	t.Helper()
	ctx := auth.WithScope(context.Background(), scope)
	return callCtx(t, a, ctx, method, target, body)
}

var (
	alice = auth.Scope{UserID: 1, Username: "alice", IsAdmin: true, Authenticated: true}
	bob   = auth.Scope{UserID: 2, Username: "bob", Authenticated: true}
)

func TestTheShelfListsWhatTheCallerMaySee(t *testing.T) {
	a, done := deckAPI(t, true)
	defer done()
	// Alice is the maintainer: the file tier under her name, writable, and
	// bob's shared deck after it.
	status, _, raw := as(t, a, alice, "/api/decks")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	var tiles []map[string]any
	_ = json.Unmarshal(raw, &tiles)
	owners := map[string]int{}
	slugs := map[string]bool{}
	for _, tile := range tiles {
		owners[tile["owner"].(string)]++
		slugs[tile["slug"].(string)] = true
		for _, key := range []string{"slug", "owner", "name", "writable", "shared", "pilot", "status", "stage",
			"needs_rationale", "commander", "companion", "bracket", "archetype", "themes", "total_cards",
			"land_count", "strategy", "art_crop", "color_identity", "errors", "warnings", "showcase"} {
			if _, ok := tile[key]; !ok {
				t.Fatalf("tile lacks %q: %v", key, tile)
			}
		}
	}
	if owners["alice"] < 7 || owners["bob"] != 1 || slugs["bobs-private"] || slugs["bobs-deleted"] || slugs["_template"] || slugs["gone"] {
		t.Fatalf("owners %v slugs %v", owners, slugs)
	}
	// Two fixture files say `slug: mono-green` -- the banned-Titan deck and
	// its clean twin -- so the shelf carries both, one with the gate's one
	// error and one with none; the counts are the gate's, on the shelf.
	counts := map[float64]bool{}
	for _, tile := range tiles {
		if tile["owner"] == "alice" && (tile["writable"] != true || tile["showcase"] != true) {
			t.Fatalf("alice's tile: %v", tile)
		}
		if tile["owner"] == "bob" && (tile["writable"] != false || tile["showcase"] != false) {
			t.Fatalf("bob's tile for alice: %v", tile)
		}
		if tile["slug"] == "mono-green" {
			counts[tile["errors"].(float64)] = true
		}
	}
	if !counts[0] || !counts[1] || len(counts) != 2 {
		t.Fatalf("the gate's counts on the shelf: %v", counts)
	}
	// Bob: his own first (both decks, writable), then alice's showcase
	// read-only, and his tiles carry no showcase flag.
	_, _, raw = as(t, a, bob, "/api/decks")
	_ = json.Unmarshal(raw, &tiles)
	if tiles[0]["owner"] != "bob" || tiles[0]["writable"] != true || tiles[0]["showcase"] != false {
		t.Fatalf("bob's first tile: %v", tiles[0])
	}
	sawPrivate, sawAlice := false, false
	for _, tile := range tiles {
		if tile["slug"] == "bobs-private" {
			sawPrivate = true
		}
		if tile["owner"] == "alice" {
			sawAlice = true
			if tile["writable"] != false || tile["showcase"] != true {
				t.Fatalf("alice's tile for bob: %v", tile)
			}
		}
	}
	if !sawPrivate || !sawAlice {
		t.Fatal("bob's shelf is incomplete")
	}
	// Auth off: one library under `local`, writable, everything showcase.
	off, doneOff := deckAPI(t, false)
	defer doneOff()
	_, _, raw = as(t, off, auth.Local, "/api/decks")
	_ = json.Unmarshal(raw, &tiles)
	if len(tiles) < 7 || tiles[0]["owner"] != "local" || tiles[0]["writable"] != true || tiles[0]["showcase"] != true {
		t.Fatalf("local shelf: %d %v", len(tiles), tiles[0])
	}
}

func TestADeckIsReachableExactlyAsFarAsItsLibrary(t *testing.T) {
	a, done := deckAPI(t, true)
	defer done()
	// Another account's private deck is a 404, never a 403; a shared one
	// reads; an unknown owner is the same 404 as an unknown deck.
	cases := []struct {
		who    auth.Scope
		path   string
		status int
	}{
		{alice, "/api/decks/bob/bobs-public", 200},
		{alice, "/api/decks/bob/bobs-private", 404},
		{alice, "/api/decks/bob/bobs-deleted", 404},
		{alice, "/api/decks/nobody/whatever", 404},
		{alice, "/api/decks/alice/mono-green", 200},
		{alice, "/api/decks/alice/no-such-deck", 404},
		{bob, "/api/decks/alice/mono-green", 200},
		{bob, "/api/decks/bob/bobs-private", 200},
		{bob, "/api/decks/local/mono-green", 404},
		{auth.Local, "/api/decks/alice/mono-green", 404},
	}
	for _, c := range cases {
		status, body, _ := as(t, a, c.who, c.path)
		if status != c.status {
			t.Errorf("%s as %s: %d, want %d (%v)", c.path, c.who.Username, status, c.status, body)
			continue
		}
		if status == 404 && !strings.HasPrefix(body["detail"].(string), "no deck '") {
			t.Errorf("%s: detail %v", c.path, body["detail"])
		}
	}
	off, doneOff := deckAPI(t, false)
	defer doneOff()
	if status, _, _ := as(t, off, auth.Local, "/api/decks/local/mono-green"); status != 200 {
		t.Fatalf("local: %d", status)
	}
	if status, body, _ := as(t, off, auth.Local, "/api/decks/alice/mono-green"); status != 404 || body["detail"] != "no deck 'alice'" {
		t.Fatalf("local, wrong owner: %d %v", status, body)
	}
}

func TestTheDeckPayloadIsServiceGetDeck(t *testing.T) {
	a, done := deckAPI(t, false)
	defer done()
	status, body, raw := as(t, a, auth.Local, "/api/decks/local/mono-green")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	for _, key := range []string{"commander_art", "slug", "name", "writable", "owner", "shared", "pilot", "status", "stage",
		"needs_rationale", "commander", "companion", "bracket", "archetype", "themes", "strategy", "notes",
		"total_cards", "land_count", "color_identity", "commander_card", "cards", "swap_board", "graveyard", "pool_available"} {
		if _, ok := body[key]; !ok {
			t.Errorf("deck lacks %q", key)
		}
	}
	if !strings.HasPrefix(string(raw), `{"commander_art":"","slug":"mono-green","name":`) {
		t.Fatalf("key order: %.80s", raw)
	}
	cmd := body["commander_card"].(map[string]any)
	if cmd["name"] != "Goreclaw, Terror of Qal Sisma" || cmd["category"] != "commander" || cmd["known"] != true ||
		cmd["oracle_id"] == nil || cmd["artist"] == nil {
		t.Fatalf("commander card: %v", cmd)
	}
	if _, has := cmd["flavor_text"]; !has {
		t.Fatal("the hero row carries flavor_text, null or not")
	}
	cards := body["cards"].([]any)
	sol := cards[0].(map[string]any)
	if sol["name"] != "Sol Ring" || sol["known"] != true || sol["power"] != nil || sol["cmc"] != float64(1) {
		t.Fatalf("sol ring row: %v", sol)
	}
	if _, has := sol["flavor_text"]; has {
		t.Fatal("a 99 row carried the hero fields")
	}
	if len(sol) != 19 { // six of the deck's, thirteen of the pool's
		t.Fatalf("a known row has %d keys, want 19: %v", len(sol), sol)
	}
	if body["pool_available"] != true || body["owner"] != "local" || body["writable"] != true {
		t.Fatalf("%v %v %v", body["pool_available"], body["owner"], body["writable"])
	}
	// An unknown card is a short row.
	_, messy, _ := as(t, a, auth.Local, "/api/decks/local/messy")
	for _, c := range messy["cards"].([]any) {
		row := c.(map[string]any)
		if row["name"] == "Not A Real Card" {
			if row["known"] != false || len(row) != 6 {
				t.Fatalf("unknown row: %v", row)
			}
		}
	}
}

func TestValidateStatsSuggestionsAgreeWithTheFixtures(t *testing.T) {
	a, done := deckAPI(t, false)
	defer done()
	dir := filepath.Join("..", "gate", "testdata")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".yaml")
		// validate: Python's report, errors and warnings apart.
		wantRaw, _ := os.ReadFile(filepath.Join(dir, slug+".report.json"))
		var want struct {
			WithPool []map[string]any `json:"with_pool"`
		}
		_ = json.Unmarshal(wantRaw, &want)
		status, body, raw := as(t, a, auth.Local, "/api/decks/local/"+slug+"/validate")
		if status != 200 {
			t.Fatalf("%s validate: %d %s", slug, status, raw)
		}
		gotErrs, gotWarns := body["errors"].([]any), body["warnings"].([]any)
		wantErrs, wantWarns := 0, 0
		for _, i := range want.WithPool {
			if i["level"] == "error" {
				wantErrs++
			} else {
				wantWarns++
			}
		}
		if len(gotErrs) != wantErrs || len(gotWarns) != wantWarns || body["ok"] != (wantErrs == 0) {
			t.Errorf("%s validate: %d/%d, want %d/%d", slug, len(gotErrs), len(gotWarns), wantErrs, wantWarns)
		}
		if len(gotErrs) > 0 {
			first := gotErrs[0].(map[string]any)
			if len(first) != 3 {
				t.Errorf("%s: an issue has keys %v, want code/message/card", slug, first)
			}
		}
		// stats: the whole document.
		wantStats, _ := os.ReadFile(filepath.Join(dir, slug+".stats.json"))
		_, _, raw = as(t, a, auth.Local, "/api/decks/local/"+slug+"/stats")
		if canonicalJSON(t, raw) != canonicalJSON(t, wantStats) {
			t.Errorf("%s stats disagree\n got %s\nwant %s", slug, raw, strings.TrimSpace(string(wantStats)))
		}
		// suggestions: the whole document where Python had something to say.
		// Both runtimes echo the *requested* slug; the fixture was rendered
		// by the deck's own (`draft.yaml` says `slug: mono-green`), so that
		// one field is normalised before the compare.
		if wantSugg, err := os.ReadFile(filepath.Join(dir, slug+".suggestions.json")); err == nil {
			_, _, raw = as(t, a, auth.Local, "/api/decks/local/"+slug+"/suggestions")
			var wantDoc map[string]any
			_ = json.Unmarshal(wantSugg, &wantDoc)
			wantDoc["slug"] = slug
			wantNorm, _ := json.Marshal(wantDoc)
			if canonicalJSON(t, raw) != canonicalJSON(t, wantNorm) {
				t.Errorf("%s suggestions disagree\n got %s\nwant %s", slug, raw, wantNorm)
			}
		} else {
			_, body, _ = as(t, a, auth.Local, "/api/decks/local/"+slug+"/suggestions")
			if body["pool_available"] != true || len(body["targets"].([]any)) != 0 {
				t.Errorf("%s suggestions: %v", slug, body)
			}
		}
	}
	if status, _, _ := as(t, a, auth.Local, "/api/decks/local/mono-green/suggestions?limit=0"); status != 422 {
		t.Fatalf("limit=0: %d", status)
	}
}

func canonicalJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	out, _ := json.Marshal(v)
	return string(out)
}

func TestCommanderPrintingsLogAndArtifacts(t *testing.T) {
	a, done := deckAPI(t, true)
	defer done()
	// The commander's panel: counted, not recalled.
	status, body, raw := as(t, a, alice, "/api/decks/alice/mono-green/commander")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	card := body["card"].(map[string]any)
	if card["name"] != "Goreclaw, Terror of Qal Sisma" || card["oracle_id"] == nil {
		t.Fatalf("card %v", card)
	}
	subtypes := body["subtypes"].([]any)
	if len(subtypes) != 1 || subtypes[0].(map[string]any)["name"] != "Bear" {
		t.Fatalf("subtypes %v", subtypes)
	}
	if p := body["printings"].(map[string]any); p["count"].(float64) < 1 || p["first_released"] == nil {
		t.Fatalf("printings %v", p)
	}
	if !strings.HasPrefix(string(raw), `{"slug":"mono-green","card":{"name":`) {
		t.Fatalf("order %.60s", raw)
	}
	// A deck with no commander: the empty panel.
	if status, _, _ := as(t, a, alice, "/api/decks/bob/bobs-public/commander"); status != 200 {
		t.Fatalf("a commander the pool lacks: %d", status)
	}
	_, body, _ = as(t, a, alice, "/api/decks/alice/rich/commander")
	if body["card"] != nil || len(body["subtypes"].([]any)) != 0 || body["printings"] != nil {
		t.Fatalf("unknown commander: %v", body)
	}
	// Printings of the commander, and of a card in the 99; a card not held
	// is a 422.
	_, body, raw = as(t, a, alice, "/api/decks/alice/mono-green/printings")
	if body["commander"] != "Goreclaw, Terror of Qal Sisma" || body["selected"] != "" || !strings.HasPrefix(string(raw), `{"slug":"mono-green","commander":`) {
		t.Fatalf("printings %s", raw)
	}
	_, body, _ = as(t, a, alice, "/api/decks/alice/mono-green/printings?card=sol+ring")
	if body["commander"] != "Sol Ring" || len(body["printings"].([]any)) < 1 {
		t.Fatalf("card printings %v", body)
	}
	first := body["printings"].([]any)[0].(map[string]any)
	for _, key := range []string{"id", "set_code", "set_name", "collector_number", "rarity", "released_at", "promo", "image", "art_crop", "price_usd", "selected"} {
		if _, ok := first[key]; !ok {
			t.Errorf("printing lacks %q", key)
		}
	}
	if status, body, _ := as(t, a, alice, "/api/decks/alice/mono-green/printings?card=Nope"); status != 422 || body["detail"] != "'Nope' is not in this deck" {
		t.Fatalf("not held: %d %v", status, body)
	}
	// The log, keyed on the library: the file tier's NULL owner, bob's id.
	_, body, _ = as(t, a, alice, "/api/decks/alice/mono-green/log")
	entries := body["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["summary"] != "added Sol Ring as ramp" || entries[0].(map[string]any)["actor"] != nil {
		t.Fatalf("log %v", body)
	}
	_, body, _ = as(t, a, bob, "/api/decks/bob/bobs-public/log")
	if entries := body["entries"].([]any); len(entries) != 1 || entries[0].(map[string]any)["actor"] != "bob" {
		t.Fatalf("bob's log %v", body)
	}
	if status, _, _ := as(t, a, alice, "/api/decks/alice/mono-green/log?limit=abc"); status != 422 {
		t.Fatalf("limit=abc: %d", status)
	}
	// The artifacts shelf: the file tier's directory, and the SQL tier's rows.
	_, body, raw = as(t, a, alice, "/api/decks/alice/kaheera/artifacts")
	arts := body["artifacts"].([]any)
	if len(arts) != 2 || arts[0].(map[string]any)["name"] != "primer-quick.md" || body["baseline"] != "current" || body["buildable"] != true {
		t.Fatalf("artifacts %s", raw)
	}
	if _, body, _ = as(t, a, alice, "/api/decks/alice/mono-green/artifacts"); body["baseline"] != "unknown" || len(body["artifacts"].([]any)) != 0 {
		t.Fatalf("unbuilt %v", body)
	}
	_, body, _ = as(t, a, alice, "/api/decks/alice/kaheera/artifacts/moxfield.txt")
	if body["text"] != "1 Sol Ring\n" || body["name"] != "moxfield.txt" {
		t.Fatalf("artifact %v", body)
	}
	// (A `../deck.yaml` never reaches the handler: the door proxies a path
	// that is not canonical, and Python's router answers its own 404.)
	for _, name := range []string{"notes.txt", "deck.last-built.yaml", "primer-advanced.md"} {
		if status, body, _ := as(t, a, alice, "/api/decks/alice/kaheera/artifacts/"+name); status != 404 ||
			!strings.HasPrefix(body["detail"].(string), "no artifact '") {
			t.Errorf("%s: %d %v", name, status, body)
		}
	}
	_, body, _ = as(t, a, bob, "/api/decks/bob/bobs-public/artifacts")
	if arts := body["artifacts"].([]any); len(arts) != 1 || body["baseline"] != "current" || body["buildable"] != false {
		t.Fatalf("sql artifacts %v", body)
	}
	if _, body, _ = as(t, a, bob, "/api/decks/bob/bobs-public/artifacts/moxfield.txt"); body["text"] != "1 Sol Ring" {
		t.Fatalf("sql artifact %v", body)
	}
	_ = time.Now
}

func TestChallengeProgressScoresTheLibrary(t *testing.T) {
	a, done := deckAPI(t, false)
	defer done()
	_, body, raw := as(t, a, auth.Local, "/api/colors/progress")
	if body["pool"] != true || body["total"] != float64(32) || body["filled"].(float64) < 1 {
		t.Fatalf("%s", raw)
	}
	slots := body["slots"].([]any)
	if len(slots) != 32 || !strings.HasPrefix(string(raw), `{"pool":true,"filled":`) {
		t.Fatalf("slots %d %.60s", len(slots), raw)
	}
	green := false
	for _, s := range slots {
		slot := s.(map[string]any)
		if slot["key"] == "G" && len(slot["decks"].([]any)) >= 2 {
			green = true
		}
	}
	if !green {
		t.Fatal("the mono-green fixtures did not fill the G slot")
	}
}
