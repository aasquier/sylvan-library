package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The lifecycle routes' own tests. `internal/deckimport` and `internal/deck`
// already prove the *bytes* against their goldens; what these prove is the
// layer above -- that a refusal lands on the right status in the right order, that
// nothing is written when one fires, and that ADR 5 survives a write.

func (r *writeRig) read(t *testing.T, slug string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(r.decks, slug, "deck.yaml"))
	if err != nil {
		return "", false
	}
	return string(raw), true
}

// ---- create ----------------------------------------------------------------

func TestCreateWritesADraftAndNothingElse(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"brand-new","name":"Brand New","commander":["Goreclaw, Terror of Qal Sisma"],"bracket":4}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	for _, key := range []string{"slug", "owner", "name", "stage", "status",
		"created", "commander", "companion", "color_identity", "combination",
		"total_cards"} {
		if _, present := body[key]; !present {
			t.Errorf("the answer has no %q: %s", key, raw)
		}
	}
	if body["stage"] != "draft" {
		t.Errorf("a created deck is a draft (ADR 13), not %v", body["stage"])
	}
	if combo, ok := body["combination"].(map[string]any); !ok || combo["key"] != "G" {
		t.Errorf("mono-green's combination should be G: %v", body["combination"])
	}

	text, found := rig.read(t, "brand-new")
	if !found {
		t.Fatal("the deck was reported created and no file was written")
	}
	d, err := deck.FromText(text, "brand-new")
	if err != nil {
		t.Fatalf("the created file does not parse: %v", err)
	}
	if d.Stage != "draft" || len(d.Cards) != 0 || d.Bracket == nil || *d.Bracket != 4 {
		t.Errorf("the file does not say what the answer did:\n%s", text)
	}
	// Creation is deliberately outside
	// ADR 28's log. Keeping it outside is a decision, not an omission: adding
	// it means a second call site, and one call site is the log's whole
	// design.
	if entries := rig.history(t, "brand-new", nil); len(entries) != 0 {
		t.Errorf("creation must not be logged; found %d entries", len(entries))
	}
}

func TestCreateRefusesBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, body, says string }{
		{"a slug with spaces", `{"slug":"not a slug","commander":["Sol Ring"]}`, "not a usable slug"},
		{"a slug already taken", `{"slug":"mono-green-clean","commander":["Sol Ring"]}`, "already exists"},
		{"no commander", `{"slug":"nobody"}`, "needs a commander"},
		{"three commanders", `{"slug":"three","commander":["Sol Ring","Sol Ring","Sol Ring"]}`,
			"at most two"},
		{"a status nobody has", `{"slug":"odd","commander":["Sol Ring"],"status":"shelved"}`,
			"is not one of"},
		{"a card the pool has not got", `{"slug":"unknown","commander":["Not A Real Card"]}`,
			"not in the pool"},
		{"a card that cannot lead", `{"slug":"illegal","commander":["Sol Ring"]}`,
			"cannot be your commander"},
	} {
		t.Run(c.name, func(t *testing.T) {
			rig := newWriteRig(t, noCredential)
			defer rig.close()
			status, body, raw := rig.do(t, alice, "POST", "/api/decks", c.body)
			if status != 422 {
				t.Fatalf("%d %s", status, raw)
			}
			detail, _ := body["detail"].(string)
			if !strings.Contains(detail, c.says) {
				t.Errorf("the refusal should mention %q: %s", c.says, detail)
			}
			for _, slug := range []string{"not a slug", "nobody", "three", "odd",
				"unknown", "illegal"} {
				if _, found := rig.read(t, slug); found {
					t.Errorf("a refused create wrote %s anyway", slug)
				}
			}
		})
	}
}

// A bracket that is not a number is the coercion's own refusal, raised
// before the handler body -- so it is a 422 naming the field rather than
// one of the editor's sentences.
func TestCreateRefusesABracketThatIsNotANumber(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	status, body, raw := rig.do(t, alice, "POST", "/api/decks",
		`{"slug":"bad-bracket","commander":["Goreclaw, Terror of Qal Sisma"],"bracket":"four"}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	if detail, _ := body["detail"].(string); !strings.HasPrefix(detail, "bracket must be a number") {
		t.Errorf("detail is %q", detail)
	}
}

// ---- import ----------------------------------------------------------------

const paste = "1 Sol Ring\n1 Cultivator Colossus\n30 Forest\n"

func TestImportWritesADraftWithEveryRationaleOwed(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"pasted","name":"Pasted","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"text":"`+strings.ReplaceAll(paste, "\n", "\\n")+`"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	for _, key := range []string{"slug", "owner", "name", "stage", "status",
		"created", "commander", "companion", "total_cards", "land_count",
		"swap_board", "needs_rationale", "unknown", "unreadable", "skipped",
		"notes", "yaml", "ok", "errors", "warnings"} {
		if _, present := body[key]; !present {
			t.Errorf("the answer has no %q: %s", key, raw)
		}
	}
	if body["needs_rationale"].(float64) != 3 {
		t.Errorf("every imported card owes a rationale (ADR 13): %v", body["needs_rationale"])
	}
	text, found := rig.read(t, "pasted")
	if !found {
		t.Fatal("the import was reported created and no file was written")
	}
	if !strings.Contains(text, "# Imported ") {
		t.Errorf("the imported file should open with its header:\n%s", text)
	}
}

// The preview runs the identical code path and writes nothing, which is what
// makes it a preview rather than a description of one.
func TestADryRunWritesNothing(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"previewed","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"dry_run":true,"text":"`+strings.ReplaceAll(paste, "\n", "\\n")+`"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["created"] != false {
		t.Errorf("a dry run has not created anything: %v", body["created"])
	}
	if body["yaml"] == "" {
		t.Error("a dry run still answers with the file it would have written")
	}
	if _, found := rig.read(t, "previewed"); found {
		t.Error("a dry run wrote a deck")
	}
}

func TestImportRefusesAListWithNothingInIt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	status, body, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"empty-paste","commander":["Goreclaw, Terror of Qal Sisma"],"text":"# just a comment"}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	if detail, _ := body["detail"].(string); !strings.Contains(detail, "nothing in that list") {
		t.Errorf("detail is %q", detail)
	}
}

func TestImportPassesTheImportersOwnRefusalThrough(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	status, body, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"headless","text":"`+strings.ReplaceAll(paste, "\n", "\\n")+`"}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	if detail, _ := body["detail"].(string); !strings.Contains(detail, "no commander in this list") {
		t.Errorf("detail is %q", detail)
	}
	if _, found := rig.read(t, "headless"); found {
		t.Error("a refused import wrote a deck")
	}
}

// ---- delete ----------------------------------------------------------------

// The answer says the deck is recoverable and hands over the handle that
// recovers it. It used to say *where the deck went* -- a filesystem path,
// rendered to the player under an instruction to open a shell -- and the
// sweep in `crypt_test.go` is what holds that gone for good.
func TestDeleteMovesTheDeckAndSaysItCanComeBack(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, alice, "DELETE",
		"/api/decks/alice/mono-green-clean?confirm=mono-green-clean", "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if recoverable, _ := body["recoverable"].(bool); !recoverable {
		t.Error("a delete that cannot say the deck is recoverable has destroyed it")
	}
	id, _ := body["crypt_id"].(string)
	if id == "" {
		t.Fatal("no handle came back, so nothing can raise the deck")
	}
	// The deck really did move rather than vanish: the crypt says so, and
	// this is the only place that reads the directory to prove it, because
	// nothing above the library layer is told the folder's name any more.
	buried, err := os.ReadDir(filepath.Join(rig.decks, ".trash"))
	if err != nil {
		t.Fatalf("the crypt could not be read: %v", err)
	}
	moved := false
	for _, e := range buried {
		if !strings.HasPrefix(e.Name(), "mono-green-clean-") {
			continue
		}
		moved = true
		if _, err := os.Stat(filepath.Join(rig.decks, ".trash", e.Name(), "deck.yaml")); err != nil {
			t.Errorf("the deck was destroyed rather than moved: %v", err)
		}
	}
	if !moved {
		t.Fatalf("nothing in the crypt is the deck that was just deleted: %v", buried)
	}
	if _, found := rig.read(t, "mono-green-clean"); found {
		t.Error("the deck is still in the library")
	}
	// Deletion is outside `_commit` too, for the same reason creation is.
	if entries := rig.history(t, "mono-green-clean", nil); len(entries) != 0 {
		t.Errorf("deletion must not be logged; found %d entries", len(entries))
	}
}

// The confirmation is deliberately not a yes/no, and it takes either spelling
// because a 26-character slug typed by eye is a gate nobody gets through.
func TestDeleteNeedsAConfirmationSomebodyHadToType(t *testing.T) {
	t.Parallel()
	for name, query := range map[string]string{
		"nothing at all": "",
		"a boolean":      "?confirm=true",
		"another slug":   "?confirm=kaheera",
	} {
		t.Run(name, func(t *testing.T) {
			rig := newWriteRig(t, noCredential)
			defer rig.close()
			status, body, raw := rig.do(t, alice, "DELETE",
				"/api/decks/alice/mono-green-clean"+query, "")
			if status != 422 {
				t.Fatalf("%d %s", status, raw)
			}
			if detail, _ := body["detail"].(string); !strings.Contains(detail, "confirm by typing") {
				t.Errorf("detail is %q", detail)
			}
			if _, found := rig.read(t, "mono-green-clean"); !found {
				t.Error("a refused delete moved the deck anyway")
			}
		})
	}
	for name, query := range map[string]string{
		"the word":            "?confirm=bury",
		"the word, shouted":   "?confirm=BURY",
		"the slug, in capers": "?confirm=Mono-Green-Clean",
	} {
		t.Run(name, func(t *testing.T) {
			rig := newWriteRig(t, noCredential)
			defer rig.close()
			status, _, raw := rig.do(t, alice, "DELETE",
				"/api/decks/alice/mono-green-clean"+query, "")
			if status != 200 {
				t.Fatalf("%d %s", status, raw)
			}
		})
	}
}

// ---- sharing ---------------------------------------------------------------

func TestSharingIsASurgicalEditAndAnswersWithTheDeck(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)

	status, body, raw := rig.do(t, alice, "PUT",
		"/api/decks/alice/mono-green-clean/shared", `{"shared":false}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["shared"] != false || body["slug"] == nil {
		t.Errorf("the answer should be the whole deck, off display: %s", raw)
	}
	after := rig.text(t)
	if added := lineDiff(before, after); len(added) != 1 || added[0] != "shared: false" {
		t.Errorf("taking a deck off display should cost one line, not %v", added)
	}

	// And true removes the key, so a deck put back on display is the file it
	// was -- byte for byte.
	if status, _, raw = rig.do(t, alice, "PUT",
		"/api/decks/alice/mono-green-clean/shared", `{"shared":true}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if rig.text(t) != before {
		t.Errorf("the round trip changed the file:\n%s", rig.text(t))
	}
}

func TestSharingNeedsTheFlag(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	status, body, raw := rig.do(t, alice, "PUT",
		"/api/decks/alice/mono-green-clean/shared", `{}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["detail"] != "shared is required" {
		t.Errorf("detail is %v", body["detail"])
	}
}

// The recorded truthiness, not Go's cast: `"no"` is true and `0` is false,
// which is what this route has always done.
func TestTheSharingFlagIsReadWithTheRecordedTruthiness(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]bool{
		`{"shared":"no"}`: true,
		`{"shared":0}`:    false,
		`{"shared":[]}`:   false,
		`{"shared":1}`:    true,
		`{"shared":null}`: false,
	} {
		t.Run(body, func(t *testing.T) {
			rig := newWriteRig(t, noCredential)
			defer rig.close()
			status, answer, raw := rig.do(t, alice, "PUT",
				"/api/decks/alice/mono-green-clean/shared", body)
			if status != 200 {
				t.Fatalf("%d %s", status, raw)
			}
			if answer["shared"] != want {
				t.Errorf("%s made shared %v, not %v", body, answer["shared"], want)
			}
		})
	}
}

// ADR 5 survives a write. Bob's private deck is absent from alice's source, so
// every verb against it is a 404 -- and a 403 raised before the lookup would
// confirm it exists. The delete route got this wrong the
// day these four were written and a test caught it; the same ordering
// governs all of them.
func TestAnotherAccountsPrivateDeckIsA404ToEveryLifecycleVerb(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	for _, c := range []struct{ method, target, body string }{
		{"DELETE", "/api/decks/bob/bobs-private?confirm=bobs-private", ""},
		{"PUT", "/api/decks/bob/bobs-private/shared", `{"shared":true}`},
	} {
		status, _, raw := rig.do(t, alice, c.method, c.target, c.body)
		if status != 404 {
			t.Errorf("%s %s answered %d, not 404: %s", c.method, c.target, status, raw)
		}
	}
	// A deck she *can* see and does not own is the other answer: 403, because
	// its existence is not the secret.
	for _, c := range []struct{ method, target, body string }{
		{"DELETE", "/api/decks/bob/bobs-public?confirm=bobs-public", ""},
		{"PUT", "/api/decks/bob/bobs-public/shared", `{"shared":false}`},
	} {
		status, _, raw := rig.do(t, alice, c.method, c.target, c.body)
		if status != 403 {
			t.Errorf("%s %s answered %d, not 403: %s", c.method, c.target, status, raw)
		}
	}
}

// lineDiff is the lines in `after` that were not in `before`.
func lineDiff(before, after string) []string {
	had := map[string]int{}
	for _, line := range strings.Split(before, "\n") {
		had[line]++
	}
	added := []string{}
	for _, line := range strings.Split(after, "\n") {
		if had[line] > 0 {
			had[line]--
			continue
		}
		added = append(added, line)
	}
	return added
}
