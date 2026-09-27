package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// The import's own refusals, and the bracket both collection routes share.
//
// Every one of these is a sentence somebody reads immediately after pasting a
// decklist they care about, which is the moment commandment 2 is about. The
// create route's twins are all driven; the import's were not, because the
// import's tests spend their attention on what it *reads* -- the misspellings,
// the quoted reasons, the sideboard -- and stop before the four things it
// simply will not do.

// The three the import refuses on the request itself, each naming the thing
// that was wrong.
func TestTheImportRefusesADuplicateSlugAnUnknownStatusAndAnUnusableSlug(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	const list = `{"commander":["Goreclaw, Terror of Qal Sisma"],"text":"1 Sol Ring"`

	for _, row := range []struct {
		name, body string
		says       []string
	}{
		// `mono-green-clean` is already on the file tier.
		{"a slug the shelf already holds",
			list + `,"slug":"mono-green-clean"}`,
			[]string{"mono-green-clean", "already exists", "pick another slug"}},
		{"a status that is not one of the two",
			list + `,"slug":"fresh","status":"nearly"}`,
			[]string{"nearly", strings.Join(reference.Deck().DeckStatuses, ", ")}},
		{"a slug nothing could be filed under",
			list + `,"slug":"Mono Green!"}`,
			[]string{"usable slug", "arahbo-cats"}},
		{"a list with nothing in it that parses as a card",
			`{"slug":"fresh","commander":["Goreclaw, Terror of Qal Sisma"],"text":"   \n\n   "}`,
			[]string{"nothing in that list parsed as a card"}},
	} {
		status, payload, raw := rig.do(t, alice, "POST", "/api/decks/import", row.body)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d: %s", row.name, status, raw)
			continue
		}
		detail := fmtDetail(payload)
		for _, want := range row.says {
			if !strings.Contains(detail, want) {
				t.Errorf("%s: the refusal never says %q: %q", row.name, want, detail)
			}
		}
	}

	// And nothing was written by any of them.
	if _, ok := rig.read(t, "fresh"); ok {
		t.Error("a refused import left a deck behind")
	}
}

// The bracket, which both collection routes coerce the same way and refuse the
// same way -- and the refusal names the field, because `422` on an import with
// ninety-nine good cards in it has to say which one of them was the problem,
// and the answer is none of them.
func TestTheBracketIsCoercedTheSameWayByBothCollectionRoutes(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	for _, row := range []struct {
		name, bracket string
		want          any
	}{
		{"a number", `3`, float64(3)},
		{"a number with a fraction, truncated", `3.7`, float64(3)},
		{"a number as text", `"4"`, float64(4)},
		{"empty text, which is no bracket at all", `""`, nil},
		{"null", `null`, nil},
		{"true, which the recorded coercion reads as one", `true`, float64(1)},
	} {
		slug := "bracket-" + strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r
			}
			return -1
		}, row.name)
		status, _, raw := rig.do(t, alice, "POST", "/api/decks",
			`{"slug":"`+slug+`","commander":"Goreclaw, Terror of Qal Sisma",`+
				`"bracket":`+row.bracket+`}`)
		if status != http.StatusOK {
			t.Errorf("%s answered %d: %s", row.name, status, raw)
			continue
		}
		_, payload, deckRaw := rig.do(t, alice, "GET", "/api/decks/alice/"+slug, "")
		if got := payload["bracket"]; got != row.want {
			t.Errorf("%s landed as %v, want %v: %s", row.name, got, row.want, deckRaw)
		}
	}

	// The refusals. A bracket that is neither a number nor a number in
	// quotes, and a number too large to be one.
	for _, row := range []struct{ name, bracket string }{
		{"text that is not a number", `"three"`},
		{"a number outside the range a float can hold", `1e999`},
		{"a list", `[3]`},
	} {
		for _, target := range []string{"/api/decks", "/api/decks/import"} {
			status, payload, raw := rig.do(t, alice, "POST", target,
				`{"slug":"refused","commander":["Goreclaw, Terror of Qal Sisma"],`+
					`"text":"1 Sol Ring","bracket":`+row.bracket+`}`)
			if status != http.StatusUnprocessableEntity {
				t.Errorf("%s %s answered %d: %s", target, row.name, status, raw)
				continue
			}
			if detail := fmtDetail(payload); !strings.Contains(detail, "bracket") {
				t.Errorf("%s %s refused without naming the field: %q",
					target, row.name, detail)
			}
		}
	}
}

// Neither route reports a deck it could not write.
//
// A shelf the process can read and not write is the deployed shape a volume
// mounted read-only leaves behind, and it is the one where "created: true" is
// worst: the answer carries the slug, the commander and the colours, so the
// page renders a deck that is not anywhere.
func TestNeitherCreateNorImportReportsADeckItCouldNotWrite(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("root writes everywhere")
	}
	decks := decksDir(t)
	if err := os.Chmod(decks, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(decks, 0o750) })

	db, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks,
		AdminEmail: "alice@example.com", AppDB: db, AppWriteDB: db})

	for _, row := range []struct{ target, body string }{
		{"/api/decks", `{"slug":"unwritable","commander":"Goreclaw, Terror of Qal Sisma"}`},
		{"/api/decks/import", `{"slug":"unwritable","commander":["Goreclaw, Terror of Qal Sisma"],"text":"1 Sol Ring"}`},
	} {
		status, payload, raw := callAs(t, a, alice, "POST", row.target, row.body)
		if status == http.StatusOK {
			t.Errorf("%s answered 200 over a shelf it cannot write: %s", row.target, raw)
			continue
		}
		if !saysSomething(payload) {
			t.Errorf("%s answered %d with nothing to read: %s", row.target, status, raw)
		}
		if _, err := os.Stat(filepath.Join(decks, "unwritable")); err == nil {
			t.Errorf("%s left something behind", row.target)
		}
	}
}

// A dry run answers the whole report and writes nothing, which is the one
// thing that makes a preview a preview.
func TestADryRunImportAnswersTheReportAndLeavesNoDeck(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, payload, raw := rig.do(t, alice, "POST", "/api/decks/import",
		`{"slug":"previewed","commander":["Goreclaw, Terror of Qal Sisma"],`+
			`"text":"1 Sol Ring\n1 Forest\n1 Nothing Like This","dry_run":true}`)
	if status != http.StatusOK {
		t.Fatalf("a dry run answered %d: %s", status, raw)
	}
	if payload["created"] != false {
		t.Errorf("a dry run reported the deck as created: %s", raw)
	}
	// The report is the whole point of the preview: the counts and the misses
	// have to be there, or there is nothing to decide on.
	for _, key := range []string{"total_cards", "land_count", "unknown",
		"did_you_mean", "needs_rationale", "ok", "errors", "warnings", "yaml"} {
		if _, present := payload[key]; !present {
			t.Errorf("the preview lacks %q: %s", key, raw)
		}
	}
	if unknown, _ := payload["unknown"].([]any); len(unknown) == 0 {
		t.Errorf("a line the pool does not know was not reported: %s", raw)
	}
	if _, ok := rig.read(t, "previewed"); ok {
		t.Error("a dry run wrote a deck")
	}
}
