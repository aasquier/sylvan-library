package pooltest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixture builder's own refusals.
//
// `authtest`'s `refusals_test.go` makes the argument and it holds here too: a
// fixture that cannot build what it promised must **say so**, because the
// alternative is a path to nothing handed back to a test that then measures
// emptiness and goes green. These two refusals are the ones with a real cause --
// a shelf nothing can be written to, and a column list that does not match the
// table -- and neither had ever been taken.

// refusedTB is a `testing.TB` that remembers a refusal instead of ending the
// test, so the builder's `Fatal` paths can be walked from inside a test that
// survives them.
//
// `Fatal` must not return -- every caller above it carries on as though the
// fixture had been built -- so it panics with its own sentinel and the caller
// recovers that one and nothing else. The embedded `testing.TB` is there for the
// unexported method the interface requires; the four calls the builder actually
// makes are overridden, and anything else would be a nil dereference loud enough
// to find.
type refusedTB struct {
	testing.TB
	dir  string
	said string
}

type refusal struct{}

func (t *refusedTB) Helper()           {}
func (t *refusedTB) TempDir() string   { return t.dir }
func (t *refusedTB) Cleanup(func())    {}
func (t *refusedTB) Fatal(args ...any) { t.stop(fmt.Sprint(args...)) }
func (t *refusedTB) Fatalf(f string, a ...any) {
	t.stop(fmt.Sprintf(f, a...))
}

func (t *refusedTB) stop(said string) {
	t.said = said
	panic(refusal{})
}

// refused runs fn with a stand-in TB and reports what it refused with, or "" if
// it did not refuse at all.
func refused(t *testing.T, dir string, fn func(testing.TB)) (said string) {
	t.Helper()
	stub := &refusedTB{TB: t, dir: dir}
	// The sentinel is recovered and the refusal is read off the stub, which is
	// the only place it survives: a `return` inside the deferred function's
	// reach is the whole reason `said` is named.
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(refusal); !ok {
				panic(r)
			}
		}
		said = stub.said
	}()
	fn(stub)
	return stub.said
}

// A fixture that cannot write its database says so rather than handing back the
// name of a file that is not there.
//
// The shelf is read-only, which under CI is a cache directory mounted the wrong
// way and on a laptop is a temp dir somebody's own tooling locked down. What
// must not happen is a path coming back: every test above this one opens it, and
// a pool with no tables in it answers "we have never heard of any of these
// cards" to everything.
func TestTheFixtureSaysSoWhenItCannotWriteItsDatabase(t *testing.T) {
	t.Parallel()
	shut := filepath.Join(t.TempDir(), "shut")
	if err := os.MkdirAll(shut, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(shut, 0o700) })

	var path string
	said := refused(t, shut, func(tb testing.TB) { path = Build(tb) })
	if said == "" {
		t.Fatalf("the fixture built %q on a shelf nothing can be written to", path)
	}
	if path != "" {
		t.Errorf("the refusal came back naming %q", path)
	}
	// The refusal carries the shelf's own words, whichever step reached the
	// disk first -- this driver opens eagerly, so it is the open rather than the
	// schema, and either is the same news. Asserted by the path rather than by a
	// step's name, so a driver that starts connecting later does not make this
	// test a lie.
	if !strings.Contains(said, "tiny.duckdb") {
		t.Errorf("the refusal does not say which file it could not write: %q", said)
	}
}

// A row the table will not take is a refusal naming the table, not a quietly
// shorter fixture.
//
// The frozen corpus and the schema are supposed to agree, and the day they stop
// agreeing -- a column renamed in `schema.sql`, a column added to
// `tiny_pool.json` -- every test standing on this fixture would be reading a
// library missing whatever row the insert dropped. So the loader refuses by
// name, and this is that refusal taken with a column list the table does not
// have.
func TestAFixtureRowTheTableWillNotTakeIsRefusedByName(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tiny.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE oracle_cards (name VARCHAR)`); err != nil {
		t.Fatal(err)
	}

	said := refused(t, t.TempDir(), func(tb testing.TB) {
		insert(tb, db, "oracle_cards", []string{"a_column_that_is_not_there"},
			[][]any{{"Fixture Chef"}})
	})
	if said == "" {
		t.Fatal("a row naming a column the table does not have was accepted")
	}
	if !strings.Contains(said, "oracle_cards") {
		t.Errorf("the refusal does not name the table being filled: %q", said)
	}
}
