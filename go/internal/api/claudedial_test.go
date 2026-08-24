package api

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The dial's route. `internal/claude`'s corpus already holds the payload
// byte for byte across twenty resolutions; what is left here is
// everything the corpus cannot see, which is all of it request-shaped: which
// query value wins, when the library is resolved, and in what order the two
// refusals are decided.
//
// All of it was measured against the running app rather than read off the
// source, because two of the three are emergent -- they follow from when
// the recorded contract resolves its arguments, not from anything the
// handler says.

func TestTheDialAnswersWithoutADeck(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	status, payload, raw := as(t, a, alice, "/api/claude")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	// The three questions that must stay apart: a UI collapsing them tells
	// somebody their key is missing when they have turned it off.
	for _, key := range []string{"installed", "configured", "stance", "ceiling",
		"default", "presets", "never", "modes", "model"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("the dial has no %q", key)
		}
	}
	stance, _ := payload["stance"].(map[string]any)
	if stance["preset"] != "off" {
		t.Errorf("no deck and no surface resolved %v, want off", stance["preset"])
	}
}

// The **key order in the marshalled bytes**, which no field-by-field
// assertion sees and the settings gear renders in.
//
// `tier1.Number`'s lesson, and it has already paid here once: the corpus
// caught `never` carrying a typographic apostrophe where the record writes
// an ASCII one, a difference every structural check in this file would pass.
func TestTheDialsKeyOrderIsTheRecordedOne(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	_, _, raw := as(t, a, alice, "/api/claude")
	var keys []string
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if _, err := dec.Token(); err != nil { // opening brace
		t.Fatal(err)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"installed", "configured", "model", "stance", "ceiling",
		"default", "presets", "never", "modes"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("key order is\n  %v\nthe record says\n  %v", keys, want)
	}
}

// A deck's `status` is the whole of what the dial reads off it, and the two
// values that matter resolve differently: a theoretical deck opens wider than
// a built one, because a wild suggestion about a list costs a moment's thought
// and one about sleeved cardboard costs a trip to the box.
func TestTheDialReadsTheDecksStatus(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	for slug, want := range map[string]string{
		"rich":       "consultant",     // status: built
		"mono-green": "second-opinion", // status: theoretical
		"messy":      "consultant",     // status: shelved -- neither, so the narrow one
	} {
		status, payload, raw := as(t, a, alice, "/api/claude?slug="+slug)
		if status != 200 {
			t.Fatalf("%s: %d %s", slug, status, raw)
		}
		stance, _ := payload["stance"].(map[string]any)
		if stance["preset"] != want {
			t.Errorf("%s resolved %v, want %q", slug, stance["preset"], want)
		}
	}
}

// **The owner is resolved even when no deck is going to be read**, so
// `?owner=nobody` with no slug at all is a 404 rather than a dial.
//
// That is not something the handler chooses; it follows from the source
// being resolved as an *argument*,
// which evaluates before the call whether or not the
// slug branch inside will ever run. Measured against the running app,
// because reading it off the source is exactly how this gets written wrong:
// the natural Go shape resolves the source lazily and answers 200.
func TestAnUnknownOwnerIsA404EvenWithNoSlug(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	for _, target := range []string{
		"/api/claude?owner=nobody",
		"/api/claude?owner=nobody&slug=rich",
	} {
		status, payload, raw := as(t, a, alice, target)
		if status != 404 {
			t.Errorf("%s answered %d, want 404: %s", target, status, raw)
			continue
		}
		if detail, _ := payload["detail"].(string); !strings.Contains(detail, "nobody") {
			t.Errorf("%s said %q, want the owner named", target, detail)
		}
	}
}

// **The deck is read before the stance is parsed.** Both are the caller's
// fault and they carry different codes, so which one is reported when both are
// wrong is contract rather than taste -- and it is the deck's, measured.
func TestTheDeckIsRefusedBeforeTheStanceIs(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	// Both wrong: the deck wins.
	status, payload, raw := as(t, a, alice, "/api/claude?stance=garbage&slug=nope")
	if status != 404 {
		t.Fatalf("both wrong answered %d, want the deck's 404: %s", status, raw)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "nope") {
		t.Errorf("said %q, want the deck named", detail)
	}
	// The stance alone is still its own 422, carrying the preset list -- the
	// sentence the settings gear needs in order to clear a pin it cannot
	// honour.
	status, payload, raw = as(t, a, alice, "/api/claude?stance=garbage")
	if status != 422 {
		t.Fatalf("a bad stance answered %d, want 422: %s", status, raw)
	}
	detail, _ := payload["detail"].(string)
	if !strings.Contains(detail, "garbage") || !strings.Contains(detail, "collaborator") {
		t.Errorf("said %q, want the value and the presets", detail)
	}
}

// The recorded reading takes the **last** repeated value; Go's
// Query().Get returns the first. Every one of this route's four parameters
// goes through the same helper, so all four are checked -- the failure mode is
// a client that appends rather than replaces, and it is silent.
func TestTheLastQueryValueWinsOnEveryParameter(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	// slug: the second one is the one that must 404.
	status, payload, _ := as(t, a, alice, "/api/claude?slug=rich&slug=nope")
	if status != 404 {
		t.Errorf("slug: answered %d, want the LAST slug's 404", status)
	} else if detail, _ := payload["detail"].(string); !strings.Contains(detail, "nope") {
		t.Errorf("slug: said %q, want the last value", detail)
	}
	// ...and the other way round, so this is not passing because both 404.
	if status, _, raw := as(t, a, alice, "/api/claude?slug=nope&slug=rich"); status != 200 {
		t.Errorf("slug: answered %d, want the LAST slug's 200: %s", status, raw)
	}
	// stance: a good pin after a bad one wins.
	if status, _, raw := as(t, a, alice, "/api/claude?stance=garbage&stance=off"); status != 200 {
		t.Errorf("stance: answered %d, want the LAST stance: %s", status, raw)
	}
	if status, _, _ := as(t, a, alice, "/api/claude?stance=off&stance=garbage"); status != 422 {
		t.Errorf("stance: answered %d, want the LAST stance's 422", status)
	}
	// surface: `theme` opens to second-opinion where an unowned name is off.
	_, payload, _ = as(t, a, alice, "/api/claude?surface=nonsense&surface=theme")
	stance, _ := payload["stance"].(map[string]any)
	if stance["preset"] != "second-opinion" {
		t.Errorf("surface: resolved %v, want the LAST surface's second-opinion", stance["preset"])
	}
	// owner: a good owner after a bad one is a dial rather than a 404.
	if status, _, raw := as(t, a, alice, "/api/claude?owner=nobody&owner=alice"); status != 200 {
		t.Errorf("owner: answered %d, want the LAST owner: %s", status, raw)
	}
}

// The create flow's default, which is the bug this parameter exists for: with
// no deck to derive from, the dial beside it reported `off` while
// `theme.stance_for` was about to run the conversation at `second-opinion`.
//
// And the branch's other half, which is the easy part to drop: a surface with
// a default only applies **when there is no deck**, so a theme surface asking
// about a built deck gets the deck's answer and not the surface's.
func TestASurfacesDefaultAppliesOnlyWithNoDeck(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	_, payload, _ := as(t, a, alice, "/api/claude?surface=theme")
	stance, _ := payload["stance"].(map[string]any)
	if stance["preset"] != "second-opinion" {
		t.Errorf("the create flow's dial resolved %v, want second-opinion", stance["preset"])
	}
	// `rich` is built, so the deck's consultant beats the surface's default.
	_, payload, _ = as(t, a, alice, "/api/claude?surface=theme&slug=rich")
	stance, _ = payload["stance"].(map[string]any)
	if stance["preset"] != "consultant" {
		t.Errorf("a theme surface WITH a built deck resolved %v, want the "+
			"deck's consultant -- the branch is `surface in defaults AND "+
			"deck is None`", stance["preset"])
	}
}

// ADR 5, reached through `Library` like every other per-deck route: a deck
// somebody else owns and has not shared is a 404 here too, not a 403 and not a
// dial about a stranger's deck.
func TestTheDialCannotSeeAnotherAccountsPrivateDeck(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	if status, _, raw := as(t, a, alice, "/api/claude?owner=bob&slug=bobs-private"); status != 404 {
		t.Errorf("bob's private deck answered %d to alice, want 404: %s", status, raw)
	}
	// Shared, so it resolves -- which is what makes the 404 above about
	// visibility rather than about the route being broken for other owners.
	if status, _, raw := as(t, a, alice, "/api/claude?owner=bob&slug=bobs-public"); status != 200 {
		t.Errorf("bob's shared deck answered %d to alice, want 200: %s", status, raw)
	}
}

// **The slug arrives as a QUERY parameter here, which no other deck route
// does** -- everywhere else it is a path segment the door has already
// canonicalised, so `..` cannot reach a handler. This route is the one place
// a caller hands the library an arbitrary string.
//
// CodeQL flagged exactly that on the PR that added this route: two high
// `go/path-injection` alerts on `FileSource.path`, which this handler had
// given a new taint source. The sanitiser is real -- `path()` refuses a slug
// containing a separator, and one that is only dots -- but CodeQL does not
// recognise `strings.ContainsAny` plus `strings.Trim` as one, and a comment
// saying "the door normalises it" stopped being the whole answer the moment a
// query parameter could carry it.
//
// **The first version of this test proved nothing, and the mutation run said
// so.** It asked for `../../../../etc/passwd` and asserted a 404 -- but
// `path()` also stats the file, so that 404 arrives whether the guard fires
// or not. Deleting the guard entirely left the test green. So the target here
// is a **real deck.yaml planted outside the root**: with the guard it is a
// 404, and without it the handler reads a deck it must never see. That is a
// probe that can fail differently, which is the only kind worth writing about
// a security boundary.
func TestTheDialsSlugCannotWalkOutOfTheLibrary(t *testing.T) {
	t.Parallel()
	// Two decks the caller must never reach, both outside the library. The
	// second one is directly in the parent, and it is what makes the
	// **dot-only** branch testable: without it `..` joins to
	// `<parent>/deck.yaml`, so there has to be a readable deck exactly there
	// or the 404 arrives for the boring reason. Found by mutation -- the
	// first version planted only the nested one and the dot-only mutant
	// survived.
	outside := t.TempDir()
	planted := filepath.Join(outside, "planted")
	if err := os.MkdirAll(planted, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("name: Planted\nstatus: built\ncards: []\n")
	if err := os.WriteFile(filepath.Join(planted, "deck.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "deck.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(outside, "library")
	if err := os.MkdirAll(filepath.Join(root, "mono-green"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mono-green", "deck.yaml"),
		[]byte("name: Real\nstatus: theoretical\ncards: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	a := New(Config{Pool: pooltest.Open(t), DecksDir: root,
		AdminEmail: "alice@example.com", AppDB: db})

	for _, slug := range []string{
		// The load-bearing ones: each of these RESOLVES to the planted deck
		// if the guard is removed.
		"../planted",
		"./../planted",
		"mono-green/../../planted",
		// Percent-encoded, so the decoder hands the handler a real separator
		// rather than the literal text -- the case a guard written against
		// the raw query string would miss.
		"..%2fplanted",
		// A backslash is refused too. **On this OS that is belt and braces
		// and a mutation survives removing it**, since `\` is an ordinary
		// filename character on darwin and linux; the check earns its place
		// only where `\` separates, and it is kept rather than trimmed
		// because the cost is one `ContainsAny` argument.
		`..\..\planted`,
		// And the shapes the dot-only branch exists for.
		"..", "...", ".",
		"mono-green\x00.txt",
	} {
		target := "/api/claude?slug=" + url.QueryEscape(slug)
		status, payload, raw := as(t, a, alice, target)
		if status != 404 {
			t.Errorf("slug %q answered %d, want 404 -- it may have READ a deck "+
				"outside the library: %s", slug, status, raw)
			continue
		}
		// The refusal is the recorded `no deck '<slug>'` and nothing more. It
		// ECHOES what the caller sent, which is not a leak -- they typed it --
		// but it must not carry the server's own paths or the OS's errno text,
		// which is what a handler reporting `os.ReadFile`'s error would do.
		detail, _ := payload["detail"].(string)
		if !strings.HasPrefix(detail, "no deck ") {
			t.Errorf("slug %q refused with %q, want the deck's own 404", slug, detail)
		}
		for _, leak := range []string{"no such file", "permission denied", "deck.yaml", outside} {
			if strings.Contains(detail, leak) {
				t.Errorf("slug %q leaked the server's %q: %q", slug, leak, detail)
			}
		}
	}
	// A real slug inside the library still resolves, so this is not passing
	// because everything 404s.
	if status, _, raw := as(t, a, alice, "/api/claude?slug=mono-green"); status != 200 {
		t.Errorf("a real deck answered %d, want 200: %s", status, raw)
	}
}
