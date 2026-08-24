package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
)

// Creating and importing a deck: the refusals, and the two privacy rules the
// account routes carry.
//
// A create is the one write that has nothing to check against -- there is no
// existing deck to compare with, so every guard has to be on the way in. The
// commander must be a real card the pool can vouch for (rule 1), a partner
// pair must actually be able to partner, the slug must be a slug, and the
// name must come from somewhere.
//
// The privacy rule is `loggable`, and it is unconditional (ADR 16): **an
// email address must never reach a log line**. Somebody typing their address
// into the username box is a real and frequent thing, and the domain is the
// part that helps without being personal.

// A slug becomes a directory name under the library root, so the grammar is
// enforced rather than sanitised -- a sanitised slug is a deck filed
// somewhere its owner did not ask for.
func TestTheSlugGrammarIsEnforcedRatherThanSanitised(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	// t.Cleanup rather than defer: a parent's defer runs the moment it
	// returns, which is *before* its parallel subtests finish -- and the
	// closed handle then surfaces as "the library could not answer that
	// right now" on every one of them.
	t.Cleanup(done)

	for _, tc := range []struct {
		name string
		slug string
		ok   bool
	}{
		{"a plain slug", "gyome", true},
		{"hyphenated", "mono-green-stompy", true},
		{"with digits", "deck-2026", true},
		{"upper case", "Gyome", false},
		{"a space", "mono green", false},
		{"a leading hyphen", "-gyome", false},
		{"a trailing hyphen", "gyome-", false},
		{"a double hyphen", "mono--green", false},
		{"a path separator", "a/b", false},
		{"a traversal", "../escape", false},
		{"a dot", "gyome.deck", false},
		{"underscore", "mono_green", false},
		{"empty", "", false},
		{"only a hyphen", "-", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body, _ := json.Marshal(map[string]any{
				"slug": tc.slug, "commander": "Sol Ring"})
			status, payload, raw := callAs(t, a, alice, "POST", "/api/decks", string(body))
			if tc.ok {
				// A valid slug gets past the grammar; it may still fail for
				// another reason, and what is pinned is only that the
				// refusal is not about the slug.
				if detail, _ := payload["detail"].(string); strings.Contains(detail, "slug") &&
					strings.Contains(detail, "letters") {
					t.Errorf("%q was refused by the grammar: %s", tc.slug, raw)
				}
				return
			}
			if status == http.StatusOK {
				t.Fatalf("%q was accepted as a slug: %s", tc.slug, raw)
			}
			if status == http.StatusInternalServerError {
				t.Errorf("%q answered 500: %s", tc.slug, raw)
			}
		})
	}
}

// A commander nobody looked up is a commander whose legality is a guess
// (rule 1), so the create refuses rather than writing a deck the gate will
// fail the moment anybody opens it.
func TestACreateRefusesACommanderThePoolCannotVouchFor(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	// t.Cleanup rather than defer: a parent's defer runs the moment it
	// returns, which is *before* its parallel subtests finish -- and the
	// closed handle then surfaces as "the library could not answer that
	// right now" on every one of them.
	t.Cleanup(done)

	for _, tc := range []struct{ name, body, wants string }{
		{"no commander at all", `{"slug":"new-deck"}`, ""},
		{"a commander that does not exist", `{"slug":"new-deck","commander":"Nonexistent Card"}`,
			"Nonexistent Card"},
		{"three commanders", `{"slug":"new-deck","commander":["Sol Ring","Forest","Island"]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, payload, raw := callAs(t, a, alice, "POST", "/api/decks", tc.body)
			if status == http.StatusOK {
				t.Fatalf("%s was accepted: %s", tc.name, raw)
			}
			if status == http.StatusInternalServerError {
				t.Errorf("%s answered 500: %s", tc.name, raw)
			}
			if tc.wants != "" {
				if detail, _ := payload["detail"].(string); !strings.Contains(detail, tc.wants) {
					t.Errorf("the refusal said %q, want it to name %q", detail, tc.wants)
				}
			}
		})
	}
}

// A create that would land on an existing slug is refused in words rather
// than burying the deck that is there.
func TestACreateRefusesASlugThatIsTaken(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	// t.Cleanup rather than defer: a parent's defer runs the moment it
	// returns, which is *before* its parallel subtests finish -- and the
	// closed handle then surfaces as "the library could not answer that
	// right now" on every one of them.
	t.Cleanup(done)

	status, payload, raw := callAs(t, a, alice, "POST", "/api/decks",
		`{"slug":"mono-green-clean","commander":"Goreclaw, Terror of Qal Sisma"}`)
	if status == http.StatusOK {
		t.Fatalf("a taken slug was accepted: %s", raw)
	}
	if detail, _ := payload["detail"].(string); !strings.Contains(detail, "mono-green-clean") {
		t.Errorf("the refusal said %q without naming the slug", detail)
	}
}

// An import refuses a decklist it cannot read rather than filing a deck that
// is mostly holes -- and it names the lines it could not read, because that
// is the whole diagnosis.
func TestAnImportRefusesAListItCannotRead(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, claude.Settings{}, true)
	// t.Cleanup rather than defer: a parent's defer runs the moment it
	// returns, which is *before* its parallel subtests finish -- and the
	// closed handle then surfaces as "the library could not answer that
	// right now" on every one of them.
	t.Cleanup(done)

	for _, tc := range []struct{ name, body string }{
		{"no slug", `{"text":"1 Sol Ring"}`},
		{"a bad slug", `{"slug":"Not A Slug","text":"1 Sol Ring"}`},
		{"nothing to import", `{"slug":"imported","text":""}`},
		{"no text at all", `{"slug":"imported"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, _, raw := callAs(t, a, alice, "POST", "/api/decks/import", tc.body)
			if status == http.StatusOK {
				t.Errorf("%s was imported: %s", tc.name, raw)
			}
			if status == http.StatusInternalServerError {
				t.Errorf("%s answered 500: %s", tc.name, raw)
			}
		})
	}
}

// **An address must never reach a log line** (ADR 16), and somebody typing
// theirs into the username box is frequent enough to be the case that
// matters. The domain is kept because it answers "is this one person or a
// script working through a list" without being personal.
func TestAnAddressInTheUsernameBoxIsRedactedBeforeItIsLogged(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		// A username cannot contain `@`, so anything that does is an address.
		{"alice@example.com", "<redacted>@example.com"},
		{"Alice.Smith+tag@mail.example.co.uk", "<redacted>@mail.example.co.uk"},
		{"@example.com", "<redacted>@example.com"},
		// A real username passes through, because it is the answer these
		// lines exist to give.
		{"alice", "alice"},
		{"grove-keeper", "grove-keeper"},
		{"", ""},
	} {
		got := loggable(tc.in)
		if got != tc.want {
			t.Errorf("loggable(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// Whatever went in, no local part comes out.
		if strings.Contains(tc.in, "@") {
			local, _, _ := strings.Cut(tc.in, "@")
			if local != "" && strings.Contains(got, local) {
				t.Errorf("loggable(%q) = %q -- the local part survived", tc.in, got)
			}
		}
	}
}

// The or-empty idiom every account handler reads its fields through: the
// falsiness check runs **before** the stringification, so `0` and `false`
// arrive as the empty string rather than as "0" and "False".
func TestTheOrEmptyReadNeverStringifiesAFalsyValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   any
		want bool
	}{
		{"null", nil, true},
		{"false", false, true},
		{"the empty string", "", true},
		{"zero", json.Number("0"), true},
		{"an empty list", []any{}, true},
		{"an empty object", map[string]any{}, true},

		{"true", true, false},
		{"a string", "alice", false},
		{"a number", json.Number("1"), false},
		{"a list", []any{1}, false},
		{"an object", map[string]any{"a": 1}, false},
		{"an unexpected type", 7, false},
	} {
		if got := falsy(tc.in); got != tc.want {
			t.Errorf("falsy(%#v) = %v, want %v", tc.in, got, tc.want)
		}
	}

	// The point of the ordering: a zero must not reach a field as "0", and
	// a false must not reach one as "False".
	for _, key := range []string{"zero", "no", "blank", "absent"} {
		body := map[string]any{
			"zero": json.Number("0"), "no": false, "blank": "",
		}
		if got := field(body, key); got != "" {
			t.Errorf("field(%q) = %q, want the empty string", key, got)
		}
	}
	if got := field(map[string]any{"name": "alice"}, "name"); got != "alice" {
		t.Errorf("a real value read as %q", got)
	}
}

// The account routes are the one place an address is deliberately
// serialised, and only into a response an admin authenticated for. Everywhere
// else it must be absent -- swept over the whole deck family, because a leak
// is a field that appeared rather than a test that failed.
func TestNoDeckRouteEverSerialisesAnAddress(t *testing.T) {
	t.Parallel()
	rig := newAccountRig(t, true)
	defer rig.close()

	for _, target := range []string{
		"/api/decks",
		"/api/colors",
		"/api/glossary",
		"/api/themes",
	} {
		rec := rig.call(t, adminScope, http.MethodGet, target, "", "")
		if strings.Contains(rec.Body.String(), "@example.com") {
			t.Errorf("%s serialised an address: %s", target, rec.Body.String())
		}
	}
}
