package library_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// `Visible` -- the browse tab's whole answer -- and the two questions worth
// asking of it.
//
// **Who is on the list**, which is ADR 5 at the level above `SourceFor`: the
// caller's own decks, the showcase, then everybody else's *shared* decks, each
// owner once. A duplicate here is a deck shown twice on a shelf; a missing
// dedupe between "mine" and "the showcase" is the exact bug the `FileOwner`
// comment records having shipped once already.
//
// And **what it says when it cannot answer**. This is the closed-handle
// question `internal/api/closeddb_test.go` asks one layer up, asked here where
// the query actually is: a shelf built over a database that has gone must
// report a fault, because an empty list reads as "you have no decks" and that
// is a different sentence from "I cannot read your decks" -- and only one of
// the two is false. A user whose library briefly looked empty would have every
// reason to think something had eaten it.

// threeAccounts is an app.db with a maintainer, a second account holding one
// shared deck and one private one, and a third holding nothing at all.
func threeAccounts(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := authtest.NewScratchDB(path); err != nil {
		t.Fatal(err)
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	for _, row := range []struct {
		name, email string
		admin       bool
	}{
		{"alice", "alice@example.com", true},
		{"bob", "bob@example.com", false},
		{"carol", "carol@example.com", false},
	} {
		if _, err := auth.Create(ctx, db, row.name, row.email, row.admin); err != nil {
			t.Fatal(err)
		}
	}
	// Bob publishes one and keeps one; carol keeps both of hers.
	for _, row := range []struct {
		owner  int64
		slug   string
		shared int
	}{{2, "bobs-shared", 1}, {2, "bobs-private", 0}, {3, "carols-first", 0}, {3, "carols-second", 0}} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO user_decks (owner_id, slug, name, yaml, shared, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, '2026-08-28T00:00:00+00:00', '2026-08-28T00:00:00+00:00')`,
			row.owner, row.slug, row.slug, "name: "+row.slug+"\ncards: []\n", row.shared); err != nil {
			t.Fatal(err)
		}
	}
	return db, t.TempDir() // the decks root: empty, and never read here
}

// libFor builds the Library the decks route would build for this caller.
func libFor(t *testing.T, db *sql.DB, decks string, scope auth.Scope, maintainer string) *library.Library {
	t.Helper()
	r := library.Resolver{DecksDir: decks, AppDB: db, AppWriteDB: db,
		Maintainer: func(context.Context) (string, error) { return maintainer, nil }}
	lib, err := r.For(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func owners(list []library.Owned) []string {
	out := make([]string, 0, len(list))
	for _, o := range list {
		out = append(out, o.Owner)
	}
	return out
}

// The list is owner-first and every owner appears once -- including the
// maintainer looking at her own showcase, which is the collision the dedupe
// exists for.
func TestVisibleIsOwnerFirstAndNamesEachOwnerOnce(t *testing.T) {
	t.Parallel()
	db, decks := threeAccounts(t)
	ctx := context.Background()

	// Bob: himself, then the showcase, then alice and carol are absent --
	// alice because the showcase already stands for her, carol because
	// nothing of hers is shared.
	bob := libFor(t, db, decks, auth.Scope{UserID: 2, Username: "bob", Authenticated: true}, "alice")
	list, err := bob.Visible(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := owners(list); len(got) != 2 || got[0] != "bob" || got[1] != "alice" {
		t.Fatalf("bob sees %v, want [bob alice]", got)
	}

	// Alice IS the showcase. One entry, not two -- and it is hers, so it must
	// be the writable one rather than the read-only view of the same files.
	alice := libFor(t, db, decks, auth.Scope{UserID: 1, Username: "alice",
		IsAdmin: true, Authenticated: true}, "alice")
	list, err = alice.Visible(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := owners(list); len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("alice sees %v, want [alice bob] -- her own segment once, "+
			"then the one account with something shared", got)
	}

	// Carol has decks and shares none: she is on her own list and on nobody
	// else's.
	carol := libFor(t, db, decks, auth.Scope{UserID: 3, Username: "carol", Authenticated: true}, "alice")
	list, err = carol.Visible(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := owners(list)
	if len(got) != 3 || got[0] != "carol" || got[1] != "alice" || got[2] != "bob" {
		t.Fatalf("carol sees %v, want [carol alice bob]", got)
	}

	// The dedupe is by folded case, and it is asked of every caller rather
	// than only of the maintainer -- an implementation that deduped on the
	// exact string would pass the three orderings above and still show a
	// `Bob` row beside a `bob` one.
	for who, list := range map[string][]library.Owned{
		"bob": mustVisible(t, bob, ctx), "alice": mustVisible(t, alice, ctx),
		"carol": mustVisible(t, carol, ctx),
	} {
		seen := map[string]bool{}
		for _, o := range owners(list) {
			if seen[strings.ToLower(o)] {
				t.Errorf("%s's shelf names %q twice: %v", who, o, owners(list))
			}
			seen[strings.ToLower(o)] = true
		}
	}
}

func mustVisible(t *testing.T, lib *library.Library, ctx context.Context) []library.Owned {
	t.Helper()
	list, err := lib.Visible(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

// Auth off is the laptop: one library, filed under `local`, and **nothing
// below the showcase runs** -- the shared query would open app.db, which a
// laptop must not acquire as a side effect of listing decks.
func TestVisibleWithAuthOffStopsAtTheShowcase(t *testing.T) {
	t.Parallel()
	db, decks := threeAccounts(t)
	lib := libFor(t, db, decks, auth.Scope{}, "")
	list, err := lib.Visible(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := owners(list); len(got) != 1 || got[0] != library.LocalOwner {
		t.Fatalf("with auth off the shelf is %v, want [%s] -- bob's shared "+
			"deck is not reachable from a machine with no accounts on it",
			got, library.LocalOwner)
	}
}

// An authenticated caller with **no account behind them** is refused rather
// than handed the file tier. A session whose user row has gone is the real
// shape of this, and answering it with the curated six -- writable -- would
// be handing a stranger the maintainer's decks.
func TestAnAuthenticatedCallerWithNoAccountIsRefusedRatherThanGivenTheFiles(t *testing.T) {
	t.Parallel()
	db, decks := threeAccounts(t)
	lib := libFor(t, db, decks,
		auth.Scope{UserID: 0, Username: "ghost", Authenticated: true}, "alice")
	if _, err := lib.Mine(); err == nil {
		t.Fatal("a caller with no account was handed a library")
	} else {
		var missing library.ErrNotFound
		if !errors.As(err, &missing) {
			t.Errorf("the refusal is %T, want ErrNotFound", err)
		}
	}
	// And the whole shelf refuses with it rather than quietly dropping the
	// caller's own segment and showing them everybody else's.
	if list, err := lib.Visible(context.Background()); err == nil {
		t.Errorf("the shelf answered %v for a caller with no account", owners(list))
	}
}

// A shelf over a database that has gone **reports the fault**. Every one of
// these is a 200 with `[]` if the error is swallowed, and every one of those
// reads as "you have no decks".
func TestAShelfOverAClosedDatabaseReportsRatherThanReadingAsEmpty(t *testing.T) {
	t.Parallel()
	db, decks := threeAccounts(t)
	ctx := context.Background()
	lib := libFor(t, db, decks,
		auth.Scope{UserID: 2, Username: "bob", Authenticated: true}, "alice")
	// It works first, so what follows is the handle going and not the fixture
	// being wrong.
	if _, err := lib.Visible(ctx); err != nil {
		t.Fatalf("the fixture does not work before the close: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if list, err := lib.Visible(ctx); err == nil {
		t.Errorf("Visible answered %v over a closed database", owners(list))
	}
	if _, err := library.SharedDecks(ctx, db); err == nil {
		t.Error("SharedDecks answered over a closed database")
	}
	// SourceFor for somebody else has to reach the database to find out who
	// they are, and cannot tell "no such account" from "cannot ask" -- so it
	// must not answer the first when it means the second.
	if _, err := lib.SourceFor(ctx, "carol"); err == nil {
		t.Error("SourceFor named an account over a closed database")
	} else {
		var missing library.ErrNotFound
		if errors.As(err, &missing) {
			t.Error("a database that cannot be read answered 'no such account', " +
				"which tells the caller carol does not exist")
		}
	}
	if _, err := library.MaintainerUsername(ctx, db, "alice@example.com"); err == nil {
		t.Error("the maintainer lookup answered over a closed database")
	}
	// ...and the resolver refuses to build a Library at all rather than
	// building one with no maintainer, which would silently make the showcase
	// `local` and unfile six decks.
	r := library.Resolver{DecksDir: decks, AppDB: db, Maintainer: func(ctx context.Context) (string, error) {
		return library.MaintainerUsername(ctx, db, "alice@example.com")
	}}
	if _, err := r.For(ctx, auth.Scope{UserID: 2, Username: "bob", Authenticated: true}); err == nil {
		t.Error("the resolver built a library over a closed database")
	}
}

// With no app.db at all -- which is a laptop, and is not a fault -- the
// shared query answers nothing rather than reaching for a handle that is not
// there.
func TestWithNoDatabaseTheSharedListIsEmptyAndNotAnError(t *testing.T) {
	t.Parallel()
	shared, err := library.SharedDecks(context.Background(), nil)
	if err != nil {
		t.Fatalf("no database: %v", err)
	}
	if len(shared) != 0 {
		t.Fatalf("no database answered %d shared decks", len(shared))
	}
	name, err := library.MaintainerUsername(context.Background(), nil, "alice@example.com")
	if err != nil || name != "" {
		t.Fatalf("no database: maintainer %q, %v", name, err)
	}
}

// TestTheMaintainerIsAnAddressThatResolvesOrNobody covers the three answers
// the lookup has, and the shape rule in front of it.
//
// The rule matters because of what it protects: `MTGLAB_ADMIN_EMAIL` names an
// **address** and the showcase is filed under a **handle**, so a malformed
// value must leave the showcase at `local` rather than becoming a segment. A
// value that reached the URL would put an email address in a path, which ADR
// 17 forbids in as many words.
func TestTheMaintainerIsAnAddressThatResolvesOrNobody(t *testing.T) {
	t.Parallel()
	db, _ := threeAccounts(t)
	ctx := context.Background()
	for _, row := range []struct{ email, want string }{
		{"alice@example.com", "alice"},
		{"  ALICE@Example.COM  ", "alice"},        // trimmed and lowered
		{"nobody@example.com", ""},                // a real address, no account
		{"", ""},                                  // unconfigured
		{"   ", ""},                               // unconfigured, with a typo
		{"alice", ""},                             // no @
		{"@example.com", ""},                      // no local part
		{"alice@", ""},                            // no domain
		{"alice@example", ""},                     // no dot in the domain
		{"alice@.com", ""},                        // a domain that starts with one
		{"alice@example.", ""},                    // ...and one that ends with one
		{"alice@ex@ample.com", ""},                // a second @
		{"alice bob@example.com", ""},             // whitespace inside
		{"alice\t@example.com", ""},               // ...of every kind
		{"alice\n@example.com", ""},               //
		{"alice\r@example.com", ""},               //
		{strings.Repeat("a", 250) + "@e.com", ""}, // past 254 characters
	} {
		got, err := library.MaintainerUsername(ctx, db, row.email)
		if err != nil {
			t.Errorf("%q: %v", row.email, err)
			continue
		}
		if got != row.want {
			t.Errorf("%q resolved to %q, want %q", row.email, got, row.want)
		}
	}
}

// The actor an edit is attributed to (ADR 28) is the handle, and is empty for
// whoever is at the machine -- an activity log that wrote `local` there would
// be inventing a person.
func TestTheActorIsTheHandleOrNobody(t *testing.T) {
	t.Parallel()
	db, decks := threeAccounts(t)
	signedIn := libFor(t, db, decks,
		auth.Scope{UserID: 2, Username: "bob", Authenticated: true}, "alice")
	if got := signedIn.Actor(); got != "bob" {
		t.Errorf("actor %q, want bob", got)
	}
	laptop := libFor(t, db, decks, auth.Scope{}, "")
	if got := laptop.Actor(); got != "" {
		t.Errorf("actor %q with auth off, want nobody -- and specifically not "+
			"%q, which is a segment rather than a person", got, library.LocalOwner)
	}
}
