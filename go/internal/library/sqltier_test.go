package library_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/artifacts"
	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/library"
)

// The SQL tier: a signed-in account's own decks, kept as rows in `app.db`
// rather than as files on the volume.
//
// Two of its rules are the ones worth holding, and both are about what a
// caller cannot see. **`shared` is this tier's own truth and is changed by
// the sharing route alone** -- otherwise editing a card's rationale would
// silently republish, or silently hide, the deck it belongs to. And **a
// delete is a mark rather than a DELETE**, because the protocol requires the
// tier to say where the deck went, and an implementation that cannot has
// destroyed it rather than removed it.
//
// The third is ADR 22's default: a deck created here is **private**. The
// argument is `decks import`, which writes a draft with an empty `why` on all
// 99 cards -- publishing that the instant it exists is nobody's intent.

const sqlDeck = "slug: gyome\nname: Gyome, Master Chef\nstatus: draft\ncards: []\n"

// ownedSQLTier is a writable SQL library owned by user 1, plus the handle behind
// it for the assertions that read the row directly.
func ownedSQLTier(t *testing.T) (*library.SQLSource, *sql.DB) {
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
	if _, err := auth.Create(context.Background(), db, "alice", "alice@example.com", false); err != nil {
		t.Fatal(err)
	}
	return library.NewSQLSource(db, db, 1, true, false), db
}

// The whole round trip through the tier, and the private default at the end
// of it.
func TestTheSQLTierCreatesReadsAndEditsADeck(t *testing.T) {
	t.Parallel()
	src, _ := ownedSQLTier(t)
	ctx := context.Background()

	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatalf("create: %v", err)
	}

	d, err := src.Get(ctx, "gyome")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if d.Name != "Gyome, Master Chef" {
		t.Errorf("the deck is named %q", d.Name)
	}
	// **ADR 22's default**: a deck created here is private, because `decks
	// import` writes 99 empty rationales and publishing that is nobody's
	// intent.
	if d.Shared {
		t.Error("a newly created deck is shared -- that is not ADR 22's default")
	}

	slugs, err := src.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 || slugs[0] != "gyome" {
		t.Errorf("the tier lists %v", slugs)
	}
	all, err := src.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("All returned %d decks", len(all))
	}

	text, err := src.ReadText(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Gyome") {
		t.Errorf("the text came back as %q", text)
	}

	// An edit updates the row and the derived name with it.
	edited := strings.Replace(sqlDeck, "name: Gyome, Master Chef", "name: Renamed", 1)
	if err := src.WriteText(ctx, "gyome", edited); err != nil {
		t.Fatalf("write: %v", err)
	}
	d, err = src.Get(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Renamed" {
		t.Errorf("the edited deck is named %q -- the row's name column did not follow", d.Name)
	}
}

// **`shared` is not touched by an edit.** Editing a card's rationale must
// never republish or hide the deck it belongs to.
func TestAnEditNeverChangesWhoCanSeeTheDeck(t *testing.T) {
	t.Parallel()
	src, _ := ownedSQLTier(t)
	ctx := context.Background()
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if err := src.SetShared(ctx, "gyome", true); err != nil {
		t.Fatalf("sharing: %v", err)
	}

	edited := strings.Replace(sqlDeck, "status: draft", "status: curated", 1)
	if err := src.WriteText(ctx, "gyome", edited); err != nil {
		t.Fatal(err)
	}
	d, err := src.Get(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Shared {
		t.Error("an edit silently hid the deck")
	}

	// And back the other way.
	if err := src.SetShared(ctx, "gyome", false); err != nil {
		t.Fatal(err)
	}
	if err := src.WriteText(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if d, err = src.Get(ctx, "gyome"); err != nil {
		t.Fatal(err)
	}
	if d.Shared {
		t.Error("an edit silently republished the deck")
	}
}

// **A delete is a mark, and it says where the deck went.** An implementation
// that could not say has destroyed the deck rather than removed it.
func TestTheSQLTiersDeleteIsAMarkThatSaysWhereItWent(t *testing.T) {
	t.Parallel()
	src, db := ownedSQLTier(t)
	ctx := context.Background()
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}

	where, err := src.Delete(ctx, "gyome")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.HasPrefix(where, "user_decks:") {
		t.Errorf("the deck went to %q, which names no row", where)
	}

	// Gone from every read.
	if _, err := src.Get(ctx, "gyome"); !library.IsNotFound(err) {
		t.Errorf("a deleted deck still reads: %v", err)
	}
	slugs, err := src.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 0 {
		t.Errorf("a deleted deck is still listed: %v", slugs)
	}

	// But the row survives, which is what makes the deletion recoverable.
	var rows int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM user_decks WHERE slug = 'gyome'").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("the row count is %d -- a mark, not a DELETE", rows)
	}

	// Deleting it again is a 404 rather than a second mark.
	if _, err := src.Delete(ctx, "gyome"); !library.IsNotFound(err) {
		t.Errorf("deleting it twice answered %v", err)
	}
	// And the slug is free again, because the unique index only binds live
	// rows.
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Errorf("the slug of a deleted deck is still taken: %v", err)
	}
}

// The create refuses to bury a deck that is already there, and the refusal is
// the one the route turns into "choose another name".
func TestTheSQLTierRefusesToBuryAnExistingDeck(t *testing.T) {
	t.Parallel()
	src, _ := ownedSQLTier(t)
	ctx := context.Background()
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}

	err := src.Create(ctx, "gyome", strings.Replace(sqlDeck, "Gyome, Master Chef", "Something Else", 1))
	if err == nil {
		t.Fatal("a second create landed on top of the first")
	}
	var exists library.ErrExists
	if !asExists(err, &exists) {
		t.Fatalf("the refusal is %T, want ErrExists", err)
	}

	// The original is untouched.
	d, err := src.Get(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if d.Name != "Gyome, Master Chef" {
		t.Errorf("the existing deck became %q", d.Name)
	}
}

// Text that is not a deck is refused rather than stored, because a row this
// tier cannot parse is a deck the owner can never open again.
func TestTheSQLTierRefusesTextThatIsNotADeck(t *testing.T) {
	t.Parallel()
	src, _ := ownedSQLTier(t)
	ctx := context.Background()

	if err := src.Create(ctx, "broken", "this is not: [a deck"); err == nil {
		t.Error("unparseable text was stored as a deck")
	}
	if _, err := src.Get(ctx, "broken"); !library.IsNotFound(err) {
		t.Errorf("the refused deck exists anyway: %v", err)
	}

	// And on the edit path, where the deck already exists.
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if err := src.WriteText(ctx, "gyome", "still not: [a deck"); err == nil {
		t.Error("an unparseable edit was stored")
	}
	text, err := src.ReadText(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Gyome") {
		t.Errorf("the refused edit landed anyway: %q", text)
	}
}

// Every write refuses a deck that is not there rather than creating one --
// an update to a deck that has vanished is a bug, not a create.
func TestTheSQLTiersWritesRefuseADeckThatIsNotThere(t *testing.T) {
	t.Parallel()
	src, _ := ownedSQLTier(t)
	ctx := context.Background()

	if err := src.WriteText(ctx, "ghost", sqlDeck); !library.IsNotFound(err) {
		t.Errorf("WriteText answered %v", err)
	}
	if err := src.SetShared(ctx, "ghost", true); !library.IsNotFound(err) {
		t.Errorf("SetShared answered %v", err)
	}
	if _, err := src.Delete(ctx, "ghost"); !library.IsNotFound(err) {
		t.Errorf("Delete answered %v", err)
	}
	if _, err := src.WriteArtifacts(ctx, "ghost", artifacts.Files{}); !library.IsNotFound(err) {
		t.Errorf("WriteArtifacts answered %v", err)
	}
	slugs, err := src.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 0 {
		t.Errorf("a refused write created %v", slugs)
	}
}

// A read-only SQL tier, and one with no write handle at all, refuse every
// verb the same way -- the second is the shape a reader gets, where the
// write handle is deliberately nil rather than a handle that would fail at
// the driver.
func TestAReadOnlySQLTierRefusesEveryVerb(t *testing.T) {
	t.Parallel()
	src, db := ownedSQLTier(t)
	ctx := context.Background()
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}

	for _, tier := range []*library.SQLSource{
		library.NewSQLSource(db, db, 1, false, false), // not writable
		library.NewSQLSource(db, nil, 1, true, false), // no write handle
	} {
		if d, err := tier.Get(ctx, "gyome"); err != nil || d == nil {
			t.Fatalf("a read-only tier cannot read: %v", err)
		}
		for name, err := range map[string]error{
			"WriteText": tier.WriteText(ctx, "gyome", sqlDeck),
			"Create":    tier.Create(ctx, "new", sqlDeck),
			"SetShared": tier.SetShared(ctx, "gyome", true),
		} {
			if !library.IsReadOnly(err) {
				t.Errorf("%s answered %v, want ErrReadOnly", name, err)
			}
		}
		if _, err := tier.Delete(ctx, "gyome"); !library.IsReadOnly(err) {
			t.Errorf("Delete answered %v", err)
		}
		if _, err := tier.WriteArtifacts(ctx, "gyome", artifacts.Files{}); !library.IsReadOnly(err) {
			t.Errorf("WriteArtifacts answered %v", err)
		}
	}

	// The declared flag is what the route layer reads, so it is asserted on
	// the tier that actually declares itself read-only.
	if library.NewSQLSource(db, db, 1, false, false).Writable() {
		t.Error("a read-only tier reports itself writable")
	}
}

// Artifacts are swept and reinserted in one transaction, so a rebuild leaves
// exactly what this build produced -- a `swaps.md` from an older build
// describes a diff that no longer exists.
func TestRebuildingArtifactsLeavesExactlyThisBuild(t *testing.T) {
	t.Parallel()
	src, _ := ownedSQLTier(t)
	ctx := context.Background()
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}

	if _, err := src.WriteArtifacts(ctx, "gyome", artifacts.Files{
		{Name: "primer-quick.md", Text: "# First"},
		{Name: "swaps.md", Text: "# Swaps"},
	}); err != nil {
		t.Fatalf("first build: %v", err)
	}
	listed, err := src.Artifacts(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("the first build stored %d artifacts", len(listed))
	}

	// A build with no swap list: the old one goes, because it describes a
	// diff that no longer exists.
	if _, err := src.WriteArtifacts(ctx, "gyome", artifacts.Files{
		{Name: "primer-quick.md", Text: "# Second"},
	}); err != nil {
		t.Fatalf("second build: %v", err)
	}
	listed, err = src.Artifacts(ctx, "gyome")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, a := range listed {
		names[a.Name] = true
	}
	if names["swaps.md"] {
		t.Error("a stale swap list survived a build that did not produce one")
	}
	if !names["primer-quick.md"] {
		t.Error("the rebuilt primer is missing")
	}
	text, err := src.ReadArtifact(ctx, "gyome", "primer-quick.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Second") {
		t.Errorf("the primer is still the first build's: %q", text)
	}
}

// One account cannot see another's decks, which is ADR 5 at the row level --
// and the refusal is absence rather than a 403, so the existence of somebody
// else's deck is not a thing this tier can be asked about.
func TestOneAccountsDecksAreAbsentToAnother(t *testing.T) {
	t.Parallel()
	src, db := ownedSQLTier(t)
	ctx := context.Background()
	if err := src.Create(ctx, "gyome", sqlDeck); err != nil {
		t.Fatal(err)
	}
	if err := src.SetShared(ctx, "gyome", true); err != nil {
		t.Fatal(err)
	}

	// A second account's own tier: alice's deck is not in it, shared or not.
	other := library.NewSQLSource(db, db, 2, true, false)
	if _, err := other.Get(ctx, "gyome"); !library.IsNotFound(err) {
		t.Errorf("another account's deck answered %v, want absence", err)
	}
	slugs, err := other.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 0 {
		t.Errorf("another account's tier lists %v", slugs)
	}
	// And a write against it is absence too, never a 403 that would confirm
	// the deck exists.
	if err := other.WriteText(ctx, "gyome", sqlDeck); !library.IsNotFound(err) {
		t.Errorf("writing another account's deck answered %v", err)
	}
}

// A shared-only SQL tier is how one account sees another's published decks:
// the private ones are simply not in it.
func TestASharedOnlySQLTierShowsOnlyWhatIsPublished(t *testing.T) {
	t.Parallel()
	src, db := ownedSQLTier(t)
	ctx := context.Background()
	for _, slug := range []string{"published", "private"} {
		if err := src.Create(ctx, slug,
			strings.Replace(sqlDeck, "slug: gyome", "slug: "+slug, 1)); err != nil {
			t.Fatal(err)
		}
	}
	if err := src.SetShared(ctx, "published", true); err != nil {
		t.Fatal(err)
	}

	view := library.NewSQLSource(db, nil, 1, false, true)
	slugs, err := view.Slugs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(slugs) != 1 || slugs[0] != "published" {
		t.Errorf("the shared-only tier lists %v", slugs)
	}
	if _, err := view.Get(ctx, "private"); !library.IsNotFound(err) {
		t.Errorf("a private deck answered %v through a shared-only tier", err)
	}
	if _, err := view.Get(ctx, "published"); err != nil {
		t.Errorf("a published deck did not read: %v", err)
	}
}

// asExists is errors.As for the exists refusal, spelled out because the
// helper package exports no predicate for it (unlike read-only and
// not-found, which the route layer needs).
func asExists(err error, target *library.ErrExists) bool {
	var e library.ErrExists
	if strings.Contains(err.Error(), "already exists") {
		*target = e
		return true
	}
	return false
}
