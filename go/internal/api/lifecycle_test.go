package api

import (
	"os"
	"path/filepath"
	"strconv"
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
		"swap_board", "needs_rationale", "why_by", "unknown", "unreadable",
		"skipped", "notes", "yaml", "ok", "errors", "warnings"} {
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

// ADR 49: the paste may declare that its quoted reasons were drafted, and the
// route holds the door -- one hand ever drafts, so one value is ever legal.
func TestImportSignsTheReasonsThePasteDeclared(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	reasoned := "1 Sol Ring \"fast mana, and it never gets cut\"\n30 Forest\n"
	status, body, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"signed","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"why_by":"claude","text":`+strconv.Quote(reasoned)+`}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["why_by"] != "claude" {
		t.Errorf("the answer does not echo the hand: %v", body["why_by"])
	}
	text, found := rig.read(t, "signed")
	if !found {
		t.Fatal("the import was reported created and no file was written")
	}
	// The indented form counts only the marks on cards; the drafted header
	// names the mark in its own prose, and that mention is not one.
	if strings.Count(text, "\n    why_by: claude") != 1 {
		t.Errorf("only Sol Ring carried a reason, so only Sol Ring is marked:\n%s", text)
	}
	if !strings.Contains(text, "reasons drafted by") {
		t.Errorf("the file's header does not say the reasons were drafted:\n%s", text)
	}

	// Any other name is a claim the deck file has no way to record.
	status, body, raw = rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"missigned","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"why_by":"aaron","text":`+strconv.Quote(reasoned)+`}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	if detail, _ := body["detail"].(string); !strings.HasPrefix(detail, "why_by can only be") {
		t.Errorf("detail is %q", detail)
	}
	if _, found := rig.read(t, "missigned"); found {
		t.Error("a refused declaration wrote a deck anyway")
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

// ---- the night games -------------------------------------------------------

// nightOf reads one of bob's decks' night flag back through the read route, so
// the assertions below check what a browser would be told rather than what the
// write route said about itself.
//
// Bob's, and read as bob, like `sharedOf` below: his are the only decks in the
// fixture that can hold this flag at all -- alice is the maintainer, so hers
// are the file tier's.
func nightOf(t *testing.T, rig *writeRig, slug string) bool {
	t.Helper()
	status, body, raw := rig.do(t, bob, "GET", "/api/decks/bob/"+slug, "")
	if status != 200 {
		t.Fatalf("reading bob/%s back: %d %s", slug, status, raw)
	}
	night, ok := body["coliseum_at_night"].(bool)
	if !ok {
		t.Fatalf("the deck does not say whether it is entered for the night games: %s", raw)
	}
	return night
}

// The owner enters their own deck and the answer is the deck, saying so.
//
// Bob rather than alice, and that is the whole reason this rig has two
// accounts: alice is the maintainer, so her decks are the file tier's, which
// has nowhere to keep this flag (the test below drives that). Bob's are the
// only decks in the fixture that can actually hold it.
func TestEnteringADeckForTheNightGamesAnswersWithTheDeck(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	if nightOf(t, rig, "bobs-private") {
		t.Fatal("a deck is entered for the night games before anybody asked")
	}
	status, body, raw := rig.do(t, bob, "PUT",
		"/api/decks/bob/bobs-private/coliseum-at-night", `{"coliseum_at_night":true}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["coliseum_at_night"] != true || body["slug"] == nil {
		t.Errorf("the answer should be the whole deck, entered: %s", raw)
	}
	if !nightOf(t, rig, "bobs-private") {
		t.Error("the deck did not stay entered")
	}

	// And withdrawing puts it back.
	if status, _, raw = rig.do(t, bob, "PUT",
		"/api/decks/bob/bobs-private/coliseum-at-night", `{"coliseum_at_night":false}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if nightOf(t, rig, "bobs-private") {
		t.Error("the deck did not come back out of the night games")
	}
}

// **The two flags are not the same switch.** They sit in adjacent columns of
// the same shape, they are written by two routes built from one shape, and the
// settings page shows them side by side -- so an UPDATE naming the wrong
// column would look completely correct until somebody's private deck went on
// display because they entered it for the night games.
func TestTheNightFlagAndSharingDoNotTouchEachOther(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, _, raw := rig.do(t, bob, "PUT",
		"/api/decks/bob/bobs-private/coliseum-at-night", `{"coliseum_at_night":true}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	_, body, raw := rig.do(t, bob, "GET", "/api/decks/bob/bobs-private", "")
	if body["shared"] != false {
		t.Errorf("entering a private deck for the night games put it on display: %s", raw)
	}

	// And the other way round: sharing must not enter anything.
	if status, _, raw = rig.do(t, bob, "PUT",
		"/api/decks/bob/bobs-public/shared", `{"shared":false}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if nightOf(t, rig, "bobs-public") {
		t.Error("taking a deck off display entered it for the night games")
	}
}

func TestEnteringTheNightGamesNeedsTheFlag(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	status, body, raw := rig.do(t, bob, "PUT",
		"/api/decks/bob/bobs-private/coliseum-at-night", `{}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["detail"] != "coliseum_at_night is required" {
		t.Errorf("detail is %v", body["detail"])
	}
}

// ADR 5 again, on the new route: bob's private deck is absent from alice's
// source, so this is a 404 and not a 403 -- a 403 would confirm it exists.
// The deck she *can* see and does not own is the other answer.
func TestTheNightRouteKeepsADR5(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	if status, _, raw := rig.do(t, alice, "PUT",
		"/api/decks/bob/bobs-private/coliseum-at-night",
		`{"coliseum_at_night":true}`); status != 404 {
		t.Errorf("bob's private deck answered alice %d, not 404: %s", status, raw)
	}
	if status, _, raw := rig.do(t, alice, "PUT",
		"/api/decks/bob/bobs-public/coliseum-at-night",
		`{"coliseum_at_night":true}`); status != 403 {
		t.Errorf("bob's shared deck answered alice %d, not 403: %s", status, raw)
	}
	// Neither refusal wrote anything.
	if nightOf(t, rig, "bobs-private") || nightOf(t, rig, "bobs-public") {
		t.Error("a refused request entered a deck anyway")
	}
}

// The file tier refuses, and refuses as a fact about the deck rather than
// about the caller: alice owns these decks outright and may change everything
// else about them, so a 403 saying "not yours to change" would be false.
//
// **This is the shape of the gap Aaron should know about**, not a bug: the
// flag lives in one place, the file tier has no row in it, and the maintainer's
// own decks are the file tier's. The settings page is expected to say so
// before anybody presses anything; this proves the server does not lie if they
// do.
func TestTheFileTierCannotEnterTheNightGamesAndSaysWhy(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)
	status, body, raw := rig.do(t, alice, "PUT",
		"/api/decks/alice/mono-green-clean/coliseum-at-night", `{"coliseum_at_night":true}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	detail, _ := body["detail"].(string)
	if !strings.Contains(detail, "night gate") {
		t.Errorf("the refusal should say the gate is shut, in words a player reads: %q", detail)
	}
	// Commandment 10: the sentence a player is shown names nothing underneath.
	for _, leak := range []string{"column", "row", "SQL", "sqlite", "database", "table"} {
		if strings.Contains(strings.ToLower(detail), strings.ToLower(leak)) {
			t.Errorf("the refusal names %q to a player: %q", leak, detail)
		}
	}
	if rig.text(t) != before {
		t.Error("a refused entry rewrote the deck file")
	}
}

// ---- the master switches ---------------------------------------------------

// The master control's whole job, driven rather than described: bob's shelf
// starts **mixed** -- one shared, one not -- and one press makes it uniform.
//
// Mixed is the interesting starting state and the reason the master is a
// three-state control rather than a checkbox: a switch that could only be read
// as on or off would have to lie about this shelf before anybody touched it.
func TestTheMasterSwitchTakesAMixedShelfToAllOn(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// The fixture's shelf: bobs-public shared, bobs-private not. Asserted
	// rather than assumed -- if the fixture ever stops being mixed, this test
	// stops testing what it says it tests and nothing else would notice.
	if !sharedOf(t, rig, "bobs-public") || sharedOf(t, rig, "bobs-private") {
		t.Fatal("the fixture shelf is no longer mixed, so this test proves nothing")
	}

	status, body, raw := rig.do(t, bob, "PUT", "/api/decks/shared", `{"shared":true}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["shared"] != true {
		t.Errorf("the receipt should say what was asked for: %s", raw)
	}
	// Two, not three: the deleted deck is not on the shelf and must not be
	// counted, woken or written.
	if body["changed"] != float64(2) {
		t.Errorf("changed is %v, want 2 -- the crypt is not part of the shelf: %s",
			body["changed"], raw)
	}
	if !sharedOf(t, rig, "bobs-public") || !sharedOf(t, rig, "bobs-private") {
		t.Error("the master switch did not reach every deck")
	}

	// And off takes all of them off, from uniform rather than from mixed.
	if status, _, raw = rig.do(t, bob, "PUT", "/api/decks/shared", `{"shared":false}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if sharedOf(t, rig, "bobs-public") || sharedOf(t, rig, "bobs-private") {
		t.Error("the master switch left a deck on display")
	}
}

// The same for the night games, which start uniformly off -- so the assertion
// that matters is that one press reaches every deck rather than the first.
func TestTheMasterSwitchEntersEveryDeckForTheNightGames(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, bob, "PUT",
		"/api/decks/coliseum-at-night", `{"coliseum_at_night":true}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["coliseum_at_night"] != true || body["changed"] != float64(2) {
		t.Errorf("the receipt is wrong: %s", raw)
	}
	if !nightOf(t, rig, "bobs-public") || !nightOf(t, rig, "bobs-private") {
		t.Error("the master switch did not enter every deck")
	}
	// Sharing is untouched by the sweep, the same way it is untouched by the
	// single-deck write.
	if sharedOf(t, rig, "bobs-private") {
		t.Error("the night sweep put a private deck on display")
	}

	if status, _, raw = rig.do(t, bob, "PUT",
		"/api/decks/coliseum-at-night", `{"coliseum_at_night":false}`); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if nightOf(t, rig, "bobs-public") || nightOf(t, rig, "bobs-private") {
		t.Error("the master switch left a deck entered")
	}
}

// **The master switch reaches the caller's shelf and stops there.** It takes no
// owner segment precisely so that no path can name somebody else's library --
// this drives the guarantee rather than trusting the route's shape, because a
// handler that resolved `Visible` without filtering on writability would serve
// the same URL and quietly publish every deck alice can see.
func TestTheMasterSwitchTouchesNobodyElsesDecks(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	if status, _, raw := rig.do(t, alice, "PUT",
		"/api/decks/shared", `{"shared":false}`); status != 200 {
		t.Fatalf("alice's own sweep: %d %s", status, raw)
	}
	// Alice's sweep ran over the file tier, which is hers. Bob's shelf is
	// exactly as it was.
	if !sharedOf(t, rig, "bobs-public") {
		t.Error("alice's master switch took bob's deck off display")
	}
	if sharedOf(t, rig, "bobs-private") {
		t.Error("alice's master switch put bob's private deck on display")
	}
}

func TestTheMasterSwitchNeedsTheFlag(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	for _, c := range []struct{ target, want string }{
		{"/api/decks/shared", "shared is required"},
		{"/api/decks/coliseum-at-night", "coliseum_at_night is required"},
	} {
		status, body, raw := rig.do(t, bob, "PUT", c.target, `{}`)
		if status != 422 {
			t.Errorf("%s answered %d, not 422: %s", c.target, status, raw)
			continue
		}
		if body["detail"] != c.want {
			t.Errorf("%s said %v, want %q", c.target, body["detail"], c.want)
		}
	}
}

// The night sweep over a shelf that cannot hold the flag refuses, and refuses
// **before writing anything** -- the file tier's first deck is where it stops,
// so there is no half-entered library to explain.
func TestTheNightSweepRefusesAShelfThatCannotHoldIt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()
	before := rig.text(t)
	status, body, raw := rig.do(t, alice, "PUT",
		"/api/decks/coliseum-at-night", `{"coliseum_at_night":true}`)
	if status != 422 {
		t.Fatalf("%d %s", status, raw)
	}
	if detail, _ := body["detail"].(string); !strings.Contains(detail, "night gate") {
		t.Errorf("the sweep's refusal should be the tier's own sentence: %q", detail)
	}
	if rig.text(t) != before {
		t.Error("a refused sweep rewrote a deck file")
	}
}

// sharedOf reads a deck's `shared` back through the read route, as its owner.
func sharedOf(t *testing.T, rig *writeRig, slug string) bool {
	t.Helper()
	status, body, raw := rig.do(t, bob, "GET", "/api/decks/bob/"+slug, "")
	if status != 200 {
		t.Fatalf("reading bob/%s back: %d %s", slug, status, raw)
	}
	shared, ok := body["shared"].(bool)
	if !ok {
		t.Fatalf("the deck does not say whether it is shared: %s", raw)
	}
	return shared
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
