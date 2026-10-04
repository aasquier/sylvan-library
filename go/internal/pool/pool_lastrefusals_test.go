package pool

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The refusals left over once the fixtures ran out, each reached by putting the
// world into the state it describes rather than by calling past a guard.
//
// Four of them are about a shelf and a half-built file -- a load that cannot
// commit, a rebuild that cannot create the file it is going to fill, a rebuild
// whose half-built file went away under it, and a bulk file whose array holds
// something that is not a card. One is a coercion whose only refusal is a value
// `encoding/json` will not take, which is `jsonText`'s own test one function
// along.

// A load whose commit fails says so, rather than reporting the rows it had
// appended as shelved.
//
// **The rows are the least of it.** The appender has already written everything
// and the transaction is the only thing standing between a half-load and the
// library; a commit that fails and is reported as success is a refresh that
// claims a hundred thousand printings and leaves the pool as it was. The caller
// above translates this into "the rows were gathered and did not get shelved",
// which is the only true sentence available.
//
// The fault arrives through the loader's own `row` argument -- the composition
// root passes `OracleRow`, and a test passes one that walks away between the
// last row and the commit. That is the shape of a volume detaching, and it is
// the only one that can reach a statement AFTER the appender: the appender talks
// to the driver connection directly, so a connector that refuses statements
// cannot be the refresh's handle.
func TestALoadThatCannotCommitDoesNotReportItsRowsAsShelved(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	path := aBulkFile(t, `{"oracle_id":"o1","name":"Fixture Chef"}`+"\n")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rows := 0
	n, err := loadInto(ctx, db, "", path, "oracle_cards", OracleColumns,
		SkipOracleLayout, func(card map[string]any) []any {
			out := OracleRow(card)
			rows++
			cancel() // the shelf goes away between the last row and the commit
			return out
		})
	if rows == 0 {
		t.Fatal("the loader never read a row, so nothing was ever appended and " +
			"this test is measuring the wrong refusal")
	}
	if err == nil {
		t.Fatalf("a load that could not commit reported %d rows shelved", n)
	}
	if n != 0 {
		t.Errorf("the refusal came back with a count of %d beside it", n)
	}
	// And the library really is as it was.
	var held int64
	if err := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM oracle_cards").Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != 0 {
		t.Errorf("the pool holds %d rows after a load that did not commit", held)
	}
}

// A rebuild that cannot create the file it is about to fill refuses before it
// touches anything, and names the path it could not prepare.
//
// The shelf is read-only here, which on the instance is a volume mounted the
// wrong way or a directory a previous run left owned by root (`rebuild.finish`
// argues the ownership half). The served pool is already open, so the refusal
// has to come from the *new* file rather than from the old one.
func TestARebuildThatCannotMakeItsNewFileRefusesBeforeTouchingTheOldOne(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	db, err := OpenWriter(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	r, err := startRebuild(context.Background(), db, dbPath)
	if err == nil {
		r.abandon(context.Background())
		t.Fatal("a rebuild prepared a new file on a shelf nothing can be written to")
	}
	// The old pool is untouched, which is the whole promise of the rebuild path.
	var n int64
	if err := db.QueryRowContext(context.Background(),
		"SELECT count(*) FROM oracle_cards").Scan(&n); err != nil {
		t.Errorf("the served pool stopped answering after a refused rebuild: %v", err)
	}
}

// A rebuild whose half-built file goes away under it refuses at the last step
// rather than renaming nothing into place.
//
// `.rebuilding` is litter by design -- the next run removes one it finds -- so a
// tidy-up that removes the file while a refresh is still filling it is a real
// shape. What must not happen is the rename going ahead: the served pool would
// be replaced by a file that is not there.
func TestARebuildWhoseHalfBuiltFileVanishedRefusesRatherThanRenaming(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool.duckdb")
	db, err := OpenWriter(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	r, err := startRebuild(ctx, db, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(r.buildPath); err != nil {
		t.Fatalf("taking the half-built file away: %v", err)
	}
	if err := r.finish(ctx); err == nil {
		t.Fatal("a rebuild finished over a half-built file that was not there")
	}
	// The served pool is still the served pool: nothing was renamed over it.
	if _, err := os.Stat(dbPath); err != nil {
		t.Errorf("the served pool is gone after a refused finish: %v", err)
	}
}

// A bulk file whose array holds something that is not a card is refused rather
// than skipped.
//
// Scryfall has served two shapes and this reader takes both; the array form is
// the legacy one, and a file whose elements are not objects is a truncated or
// mangled download. Reading past it would shelve a library missing however many
// cards came after the bad element, with nothing anywhere saying so.
func TestABulkArrayHoldingSomethingThatIsNotACardIsRefused(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "oracle_cards-2026-08-24.json")
	if err := os.WriteFile(path,
		[]byte(`[{"oracle_id":"o1","name":"Fixture Chef"}, 7]`), 0o600); err != nil {
		t.Fatal(err)
	}
	seen := 0
	err := IterCards(path, func(map[string]any) error {
		seen++
		return nil
	})
	if err == nil {
		t.Fatalf("a bulk array with a number in it read as %d cards and no error", seen)
	}
	if seen != 1 {
		t.Errorf("the reader yielded %d cards before refusing, want the one good one", seen)
	}
}

// The JSONL pass's own two refusals, taken directly.
//
// `IterCards` dispatches to this on a second pass, having already opened the
// file and its gzip once -- so from there neither of these can fail without a
// race. They are a function's own contract rather than a branch of its caller's,
// and a reader that opened nothing and reported no cards would be a pool loaded
// from a file nobody read.
func TestTheJSONLPassRefusesAFileItCannotReadAtAll(t *testing.T) {
	t.Parallel()
	seen := 0
	count := func(map[string]any) error { seen++; return nil }

	if err := iterJSONL(filepath.Join(t.TempDir(), "never-downloaded.jsonl"),
		count); err == nil {
		t.Error("the reader reported no error over a file that is not there")
	}

	// A `.gz` whose contents are not gzipped at all: the name says compressed,
	// the bytes do not, and a download cut off at its first byte looks like this.
	notZipped := filepath.Join(t.TempDir(), "oracle_cards-2026-08-24.jsonl.gz")
	if err := os.WriteFile(notZipped, []byte("{\"name\":\"Fixture Chef\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := iterJSONL(notZipped, count); err == nil {
		t.Error("the reader read a file whose gzip header is not a gzip header")
	}
	if seen != 0 {
		t.Errorf("the reader yielded %d cards out of files it could not read", seen)
	}
}

// And the gzip half of the first pass, for completeness of the pair: a real
// gzip stream reads, and the reader dispatches on the first token rather than on
// the name.
func TestAGzippedBulkFileReadsOnItsContentRatherThanItsName(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "oracle_cards-2026-08-24.jsonl.gz")
	fh, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(fh)
	if err := json.NewEncoder(zw).Encode(
		map[string]any{"oracle_id": "o1", "name": "Fixture Chef"}); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fh.Close(); err != nil {
		t.Fatal(err)
	}
	seen := 0
	if err := iterJSONL(path, func(map[string]any) error { seen++; return nil }); err != nil {
		t.Fatalf("reading a gzipped bulk file: %v", err)
	}
	if seen != 1 {
		t.Errorf("a gzipped file of one card read as %d cards", seen)
	}
}

// A sub-document `encoding/json` will not take becomes NULL rather than
// stopping the refresh.
//
// This is `jsonText`'s guard one function along, for the column most cards do
// not have at all: a card that related to nothing has always been NULL there,
// and a document that cannot be encoded is the same answer rather than a refresh
// that fails over one card's related-parts list.
func TestAnUnencodableRelatedPartsDocumentBecomesNothing(t *testing.T) {
	t.Parallel()
	if got := jsonOrNull(make(chan int)); got != nil {
		t.Errorf("a value no encoder will take became %#v rather than nothing", got)
	}
	// And a document that *is* encodable travels as the document rather than as
	// text, which is the rule this function shares with its sibling.
	doc := []any{map[string]any{"component": "token"}}
	if got := jsonOrNull(doc); got == nil {
		t.Error("a real related-parts document became nothing")
	}
}

// A row builder that does not match the column list is refused before a single
// row lands, and the emptying it already did rolls back with it.
//
// `OracleColumns` and `OracleRow` are a pair, and so are the printings'; the
// loader takes both as arguments precisely so a rebuild can aim them at a
// catalog, which means nothing in the type system holds them together. A
// mismatch is a bug in a commit rather than a state of the volume -- and the
// outcome that must not happen is the one the transaction exists for: the table
// is emptied first, so a loader that gave up halfway without rolling back would
// leave the library empty and report a failure nobody could undo.
func TestALoaderWhoseRowsDoNotMatchItsColumnsEmptiesNothing(t *testing.T) {
	t.Parallel()
	db, _ := aWritablePool(t)
	ctx := context.Background()
	// One row already on the shelf, so "emptied and left emptied" is visible.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO oracle_cards (oracle_id, name) VALUES ('o0', 'Fixture Chef')`,
	); err != nil {
		t.Fatal(err)
	}
	path := aBulkFile(t, `{"oracle_id":"o1","name":"Fixture Sous Chef"}`+"\n")

	n, err := loadInto(ctx, db, "", path, "oracle_cards", OracleColumns,
		SkipOracleLayout, func(map[string]any) []any {
			// One value for twenty-eight columns.
			return []any{"Fixture Sous Chef"}
		})
	if err == nil {
		t.Fatalf("a row of one value was appended to a table of %d columns, "+
			"%d times", len(OracleColumns), n)
	}
	var held int64
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM oracle_cards").Scan(&held); err != nil {
		t.Fatal(err)
	}
	if held != 1 {
		t.Errorf("the pool holds %d rows after a refused load; the row that was "+
			"already there is the whole point of the transaction", held)
	}
}
