package api

import (
	"strings"
	"testing"
)

// Renaming a deck over the route, which is where the ruling has to hold rather
// than in the editor alone.
//
// Aaron, 2026-08-29: "users should be able to edit their deck names after they
// are imported. I don't think that is currently possible." He was right --
// `name` was not a settable deck field, so an imported deck wore whatever the
// import form was given forever.
//
// The rename rides the deck patch that already exists. That is the point of
// there being one route rather than one per field: adding a settable field
// gives it ADR 5's absence rule, ADR 22's owner-qualified address and ADR 28's
// log entry without a line of route code, because `commit` is one function.
// These tests hold all three, because "it inherits them" is a claim and a
// claim is what this repository has been wrong about before.

// The fixture is the one that disagrees with itself: the deck lives in a
// folder called `mono-green-clean` and its file says `slug: mono-green`. The
// folder is the address, and neither of them is the name.
func TestADeckCanBeRenamedThroughTheRoute(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	_, was, _ := rig.do(t, alice, "GET", cleanDeck, "")

	status, body, raw := rig.do(t, alice, "PATCH", cleanDeck,
		`{"field":"name","value":"Goreclaw, Terror of Qal Sisma — Stompy"}`)
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if body["field"] != "name" || body["value"] != "Goreclaw, Terror of Qal Sisma — Stompy" {
		t.Errorf("the response describes a different edit: %v", body)
	}

	// Read it back through the route, so the assertion is about what the app
	// serves rather than about a line in a file.
	_, deckBody, _ := rig.do(t, alice, "GET", cleanDeck, "")
	if deckBody["name"] != "Goreclaw, Terror of Qal Sisma — Stompy" {
		t.Errorf("the deck reads back as %v", deckBody["name"])
	}

	// **The address did not move.** The GET above is the same URL as the one
	// before the rename and it still answers, and the deck's slug reads back
	// exactly as it did. If this ever fails, every link, artifact and log row
	// for this deck names something else.
	if deckBody["slug"] != was["slug"] {
		t.Errorf("the deck's slug moved from %v to %v under a rename",
			was["slug"], deckBody["slug"])
	}
	if !strings.Contains(rig.text(t), "slug: mono-green\n") {
		t.Errorf("the rename reached the deck's own slug key:\n%s", rig.text(t))
	}

	// ADR 28: one entry, from the one call site, saying what happened without
	// being an undo.
	entries := rig.history(t, "mono-green-clean", nil)
	if len(entries) != 1 {
		t.Fatalf("the history has %d entries, expected 1: %+v", len(entries), entries)
	}
	if entries[0].Action != "set-deck" ||
		entries[0].Summary != "set name to Goreclaw, Terror of Qal Sisma — Stompy" {
		t.Errorf("recorded %q / %q", entries[0].Action, entries[0].Summary)
	}
}

// The refusals, each of which is an answer about the deck rather than about
// the caller, so each is a 422 carrying the editor's own sentence.
func TestARenameToNothingIsRefused(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	before := rig.text(t)
	cases := []struct{ name, body, wants string }{
		{"blank", `{"field":"name","value":""}`, "a deck needs a name"},
		{"whitespace", `{"field":"name","value":"   "}`, "a deck needs a name"},
		{"nothing at all", `{"field":"name","value":null}`, "a deck needs a name"},
		{"a paragraph", `{"field":"name","value":"` +
			strings.Repeat("a very long deck name ", 6) + `"}`, "at most 80 characters"},
	}
	for _, c := range cases {
		status, body, raw := rig.do(t, alice, "PATCH", cleanDeck, c.body)
		if status != 422 {
			t.Errorf("%s: %d %s", c.name, status, raw)
			continue
		}
		if !strings.Contains(fmtDetail(body), c.wants) {
			t.Errorf("%s: %q does not mention %q", c.name, fmtDetail(body), c.wants)
		}
	}
	if rig.text(t) != before {
		t.Error("a refused rename reached the file")
	}
}

// ADR 5 at a write, on the one field somebody would most want to vandalise:
// a deck you cannot see is absent, and a deck you can see but not write
// explains itself.
func TestOnlyTheOwnerMayRenameADeck(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	const rename = `{"field":"name","value":"Mine Now"}`
	if status, _, _ := rig.do(t, bob, "PATCH", "/api/decks/alice/rich", rename); status != 404 {
		t.Errorf("a deck bob cannot see answered %d to a rename, expected 404", status)
	}
	if status, _, _ := rig.do(t, bob, "PATCH", cleanDeck, rename); status != 403 {
		t.Errorf("a shared deck bob may not edit answered %d to a rename, expected 403", status)
	}
	if !strings.Contains(rig.text(t), "name: Mono-Green Fixture") {
		t.Errorf("a refused rename reached the file:\n%s", rig.text(t))
	}
}
