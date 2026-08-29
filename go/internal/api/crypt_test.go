package api

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
)

// The crypt's two routes, and the leak they were built to close.
//
// The bug, in the maintainer's words on 2026-08-29: *"when we currently entomb
// decks we are leaking implementation details about where on our filesystem
// their deleted decks are, they shouldn't know that, if anything we just need
// them to have an easy way to bring them back if possible."* Both halves of
// that sentence are held here -- the path is gone, **and** the way back exists,
// because removing the path without building the way back would have taken
// away the only recovery a player had and called it a fix.
//
// `TestNothingAboutADeletionNamesWhatIsUnderneath` is the regression proper.
// It sweeps every body the family can produce, refusals included, because the
// leak was never in the happy path anybody looked at twice -- it was in a
// success response that had carried a filesystem path since the feature was
// written, under a sentence recommending a shell.

// crypt is `GET /api/decks/entombed` for one caller.
func (r *writeRig) crypt(t *testing.T, who auth.Scope) []map[string]any {
	t.Helper()
	status, body, raw := r.do(t, who, "GET", "/api/decks/entombed", "")
	if status != 200 {
		t.Fatalf("the crypt answered %d %s", status, raw)
	}
	rows, _ := body["entombed"].([]any)
	out := []map[string]any{}
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// A player's crypt holds their own decks and nobody else's. There is no owner
// segment on either route, so this is structural rather than checked: the only
// crypt a request can name is the caller's own.
func TestTheCryptShowsThisPlayersDecksAndNobodyElses(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	// alice is the maintainer, so her library is the file tier -- and the
	// fixture leaves one hand-placed entry in it.
	hers := rig.crypt(t, alice)
	if len(hers) != 1 || hers[0]["slug"] != "gone" {
		t.Fatalf("alice's crypt is %+v", hers)
	}
	// Nothing recorded when that one was buried, so the answer says so rather
	// than inventing a date.
	if hers[0]["entombed_at"] != nil {
		t.Errorf("an entry with no burial time was given %v", hers[0]["entombed_at"])
	}

	his := rig.crypt(t, bob)
	if len(his) != 1 || his[0]["slug"] != "bobs-deleted" {
		t.Fatalf("bob's crypt is %+v", his)
	}
	if his[0]["name"] != "Gone" {
		t.Errorf("bob's entombed deck is named %v", his[0]["name"])
	}
	// The handles are not interchangeable: bob asking for alice's is asking
	// about a deck that is not in his crypt, which is an absence.
	status, _, raw := rig.do(t, bob, "POST",
		"/api/decks/entombed/"+str(hers[0], "id")+"/return", "")
	if status != 404 {
		t.Fatalf("bob raised a deck out of alice's crypt: %d %s", status, raw)
	}
}

// **The arrangement `Mine` alone would have got wrong.** With no maintainer
// configured (ADR 17: the maintainer is named in the environment, and here
// the address names nobody), an admin writes the file tier through the owner
// segment while `Mine` answers with their own rows. A deck buried through one
// would have been raised through the other, and the notice on screen would
// have offered a handle the restore route could not find. The crypt is every
// library the caller may *write*, so both are in it.
func TestACryptIsEveryLibraryTheCallerCanWrite(t *testing.T) {
	t.Parallel()
	decks := decksDir(t)
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	writeDB, err := auth.OpenReadWrite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writeDB.Close()
	// The one difference from `newWriteRig`: an admin address no account
	// carries, so `MaintainerUsername` resolves to nothing.
	a := New(Config{Pool: pooltest.Open(t), DecksDir: decks,
		AdminEmail: "nobody@example.com", AppDB: db, AppWriteDB: writeDB})

	status, body, raw := callAs(t, a, alice, "GET", "/api/decks/entombed", "")
	if status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	rows, _ := body["entombed"].([]any)
	slugs := map[string]bool{}
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			slugs[str(m, "slug")] = true
		}
	}
	// The file tier's hand-placed entry: alice reaches it because with no
	// maintainer the six are an admin's to write.
	if !slugs["gone"] {
		t.Errorf("the file tier's crypt is missing: %v", slugs)
	}
	// And still nobody else's: bob's buried row is his, maintainer or no
	// maintainer.
	if slugs["bobs-deleted"] {
		t.Errorf("another account's crypt leaked in: %v", slugs)
	}
}

// The round trip a player actually makes: entomb it, see it in the crypt,
// bring it back, find it on the shelf.
func TestADeckEntombedComesBackFromTheCrypt(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, bob, "DELETE",
		"/api/decks/bob/bobs-private?confirm=bury", "")
	if status != 200 {
		t.Fatalf("delete: %d %s", status, raw)
	}
	id := str(body, "crypt_id")
	if id == "" || body["recoverable"] != true {
		t.Fatalf("the deletion did not offer a way back: %v", body)
	}
	if status, _, _ := rig.do(t, bob, "GET", "/api/decks/bob/bobs-private", ""); status != 404 {
		t.Fatalf("the entombed deck still answers %d", status)
	}

	// The crypt names it, and names it with the handle the deletion handed
	// back -- the notice on screen and the crypt tab are talking about the
	// same entry.
	found := false
	for _, row := range rig.crypt(t, bob) {
		if row["id"] == id && row["slug"] == "bobs-private" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the crypt does not hold what was just buried: %+v", rig.crypt(t, bob))
	}

	status, body, raw = rig.do(t, bob, "POST", "/api/decks/entombed/"+id+"/return", "")
	if status != 200 {
		t.Fatalf("return: %d %s", status, raw)
	}
	if body["slug"] != "bobs-private" || body["restored"] != true {
		t.Errorf("the restore answered %v", body)
	}
	if status, _, raw := rig.do(t, bob, "GET", "/api/decks/bob/bobs-private", ""); status != 200 {
		t.Fatalf("the raised deck answers %d %s", status, raw)
	}
	// And it is out of the crypt: a deck cannot be both on the shelf and
	// buried.
	for _, row := range rig.crypt(t, bob) {
		if row["id"] == id {
			t.Error("the raised deck is still listed as entombed")
		}
	}
}

// A deck always comes back under its own name, so the one refusal that is
// about the library rather than the caller is a slug a living deck holds. It
// is 422 with the question, never a rename nobody asked for.
func TestARestoreOntoALivingSlugIsRefusedInWords(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	_, body, _ := rig.do(t, bob, "DELETE", "/api/decks/bob/bobs-private?confirm=bury", "")
	id := str(body, "crypt_id")
	// The slug is free now, so something else takes it.
	if status, _, raw := rig.do(t, bob, "POST", "/api/decks",
		`{"slug":"bobs-private","name":"Second Try",`+
			`"commander":["Goreclaw, Terror of Qal Sisma"]}`); status != 200 {
		t.Fatalf("the replacement deck: %d %s", status, raw)
	}

	status, refusal, raw := rig.do(t, bob, "POST", "/api/decks/entombed/"+id+"/return", "")
	if status != 422 {
		t.Fatalf("raising onto a living deck answered %d %s", status, raw)
	}
	detail := str(refusal, "detail")
	if !strings.Contains(detail, "bobs-private") {
		t.Errorf("the refusal does not say which deck is in the way: %q", detail)
	}
	// Still buried, and the living deck untouched.
	if status, _, _ := rig.do(t, bob, "GET", "/api/decks/bob/bobs-private", ""); status != 200 {
		t.Error("the refused restore disturbed the living deck")
	}
}

// A handle nobody holds is an absence with a sentence a player can act on --
// not `no deck '<sixteen hex characters>'`, which names a thing they have
// never seen.
func TestAHandleTheCryptDoesNotHoldIsAPlainAbsence(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	status, body, raw := rig.do(t, bob, "POST", "/api/decks/entombed/deadbeefdeadbeef/return", "")
	if status != 404 {
		t.Fatalf("%d %s", status, raw)
	}
	detail := str(body, "detail")
	if !strings.Contains(detail, "crypt") || strings.Contains(detail, "deadbeef") {
		t.Errorf("the absence reads %q", detail)
	}
}

// **The regression.** Nothing this family says to a player names what is
// underneath the site: not a path, not the trash directory, not a table, not a
// shell. Commandment 10 -- a filesystem path is as much a leak as a model id.
//
// Swept over every body the family produces rather than asserted on the one
// that was wrong, because the leak lived in a *success* response for as long
// as the feature existed and no test had ever read one for this.
func TestNothingAboutADeletionNamesWhatIsUnderneath(t *testing.T) {
	t.Parallel()
	rig := newWriteRig(t, noCredential)
	defer rig.close()

	bodies := []string{}
	record := func(t *testing.T, who auth.Scope, method, target, body string) map[string]any {
		t.Helper()
		_, parsed, raw := rig.do(t, who, method, target, body)
		bodies = append(bodies, string(raw))
		return parsed
	}

	deleted := record(t, bob, "DELETE", "/api/decks/bob/bobs-private?confirm=bury", "")
	id := str(deleted, "crypt_id")
	record(t, bob, "GET", "/api/decks/entombed", "")
	record(t, alice, "GET", "/api/decks/entombed", "")
	// The refusals too: a leak in an error message is a leak.
	record(t, bob, "POST", "/api/decks/entombed/deadbeefdeadbeef/return", "")
	record(t, bob, "DELETE", "/api/decks/bob/bobs-public?confirm=nope", "")
	record(t, bob, "POST", "/api/decks/entombed/"+id+"/return", "")

	// The decks directory itself, and every word that would give it away. The
	// last three are the sentence that shipped: *"The deck moves to
	// decks/.trash/ rather than being erased, so this is reversible from the
	// shell."*
	forbidden := []string{
		rig.decks, filepath.Base(rig.decks),
		".trash", "/tmp/", "user_decks", "deck.yaml",
		"shell", "terminal", "filesystem", "directory", "folder",
	}
	for _, raw := range bodies {
		for _, word := range forbidden {
			if word == "" {
				continue
			}
			if strings.Contains(strings.ToLower(raw), strings.ToLower(word)) {
				t.Errorf("a deletion answer names %q, which is not a player's business:\n%s",
					word, raw)
			}
		}
	}
}
