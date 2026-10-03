package api

import (
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/traffic"
)

// The upkeep room's two faults, and the admin panel's three readings that only
// an odd volume produces.

// **A library already shut for a rewrite cannot be shut again**, and the
// gathering has to hand the latch back when it finds it so.
//
// Two refreshes at once is what the latch is for and it is checked at the route;
// this is the fault *below* it — the card pool itself sealed by something else,
// which on the instance is a `mtglab data refresh` run over the console while
// the button is pressed in a browser. The sentence a person gets names no file
// and no variable (commandment 10), and the latch must not be left holding, or
// every refresh afterwards answers "a gathering is already under way" forever.
func TestAGatheringOverAnAlreadySealedLibrarySaysSoAndLetsGo(t *testing.T) {
	t.Parallel()
	p := pooltest.Open(t)
	// Sealed by somebody else, and left sealed: the state the refresh walks in
	// on.
	reopen, err := p.Seal(t.Context(), 0)
	if err != nil {
		t.Fatalf("sealing the fixture: %v", err)
	}
	t.Cleanup(reopen)

	a := New(Config{Logger: quietLogger(), Pool: p,
		PoolPath: filepath.Join(t.TempDir(), "pool.duckdb"),
		DecksDir: t.TempDir(),
		Jobs:     jobs.New(jobs.Config{Logger: quietLogger()})})

	_, err = a.gatherTheLibrary(reportFunc(func(int, int) {}))
	if err == nil {
		t.Fatal("a gathering over a library already shut for a rewrite reported success")
	}
	for _, machinery := range []string{"duckdb", ".go:", "/var/folders",
		"MTGLAB_", "sql"} {
		if strings.Contains(strings.ToLower(err.Error()),
			strings.ToLower(machinery)) {
			t.Errorf("the sentence a person reads carries %q: %q", machinery, err)
		}
	}
	// The latch is back, so the next press is a real attempt rather than a 409
	// about a gathering that is not happening.
	if _, running := a.gathering.running(); running {
		t.Error("a failed gathering left the latch holding, so every refresh " +
			"afterwards answers that one is already under way")
	}
}

// **A refresh nothing will queue hands the library back.**
//
// The latch is claimed before the job is made, so the one arm where it can be
// claimed and never released is a queue that refuses — and the cost of getting
// that wrong is permanent: a latch left holding answers 409 to every press until
// the process restarts. The fixture is a registry that was never built, which is
// the one state `Submit` has an error for.
func TestARefreshNothingWillQueueHandsTheLibraryBack(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger(),
		PoolPath: filepath.Join(t.TempDir(), "pool.duckdb"),
		DecksDir: t.TempDir(),
		Jobs:     &jobs.Registry{}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/library/refresh", nil)
	a.refreshLibrary(rec, req.WithContext(auth.WithScope(req.Context(), adminScope)))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("a refresh nothing would queue answered %d: %s", rec.Code, rec.Body)
	}
	if said := detail(t, rec); strings.TrimSpace(said) == "" {
		t.Fatalf("it answered %d with nothing to read: %s", rec.Code, rec.Body)
	}
	if _, running := a.gathering.running(); running {
		t.Error("a refresh that could not be queued left the latch holding")
	}
}

// ---- the storage panel's odd volumes ---------------------------------------

// **A path that is neither a file nor a directory is sized as nothing.**
//
// The storage panel adds up a handful of paths off the volume, and a socket, a
// pipe or a device node among them is a path with no size worth reporting — so
// it answers absent rather than nought, which is the difference between "nothing
// there" and "nothing to ask" that this whole panel is careful about.
func TestAPathThatIsNeitherFileNorDirectorySizesAsNothing(t *testing.T) {
	t.Parallel()
	// A unix socket: it stats, it is not regular, and it is not a directory.
	// `t.TempDir()` on this platform can be long enough to run into the
	// socket-path limit, so the listener is built under a short one.
	dir, err := os.MkdirTemp("", "sock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("this platform would not make a unix socket to size: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	if got := sizeOf(path); got != nil {
		t.Errorf("a socket was sized at %d bytes", *got)
	}
	// And a directory with a file in it still adds up, so the nil above is
	// about the socket rather than about the function being broken.
	shelf := t.TempDir()
	if err := os.WriteFile(filepath.Join(shelf, "deck.yaml"),
		[]byte("name: X\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	size := sizeOf(shelf)
	if size == nil || *size != 8 {
		t.Errorf("a shelf holding eight bytes sized as %v", size)
	}
}

// **The visitor ledger refuses rather than reporting a quiet month.**
//
// Thirty days of route templates and status classes, and a handle that has gone
// would otherwise render as an instance nobody has visited — which is the one
// answer a traffic panel must never give, because it is indistinguishable from
// the real thing.
func TestTheTrafficPanelRefusesALedgerThatWillNotAnswer(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", "file:"+appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	a := New(Config{Logger: quietLogger(), Traffic: traffic.New(db, quietLogger())})

	status, payload, raw := callAs(t, a, alice, "GET",
		"/api/admin/stats/traffic", "")
	if status == http.StatusOK {
		t.Fatalf("the traffic panel answered 200 over a closed ledger: %s", raw)
	}
	if said, _ := payload["detail"].(string); strings.TrimSpace(said) == "" {
		t.Errorf("it answered %d with nothing to read: %s", status, raw)
	}
}

// **An `app.db` that is there and is not a database is no accounts database.**
//
// `sql.Open` only records the DSN, so the lazy open could never discover this on
// its own — it handed back a handle that failed at the first INSERT, which is
// exactly where `auth.PingWritable` exists to stop a failure being discovered.
// With the handle proved before it is kept, the state answers the way an absent
// database answers: the routes treat it as empty, and a sign-in is refused
// rather than 500.
func TestAnAppDBThatIsNotADatabaseIsNoAccountsDatabase(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "app.db")
	if err := os.WriteFile(path, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := New(Config{Logger: quietLogger(), AppDBPath: path,
		DecksDir: t.TempDir(), RequireAuth: true})

	if db, present := a.accountsDB(); present {
		t.Errorf("a file that is not a database was kept as the accounts "+
			"handle: %v", db)
	}
	// And the route it serves refuses rather than breaking: the same answer an
	// instance with no `app.db` at all gives.
	status, payload, raw, _ := postSignIn(t, a, "/api/auth/login",
		`{"username":"alice","password":"`+goodPassword+`"}`, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("a login against a file that is not a database answered %d: %s",
			status, raw)
	}
	if said, _ := payload["detail"].(string); strings.TrimSpace(said) == "" {
		t.Errorf("it answered %d with nothing to read: %s", status, raw)
	}
	// A real database over the same seam still answers, so the refusal above is
	// about the file rather than about the lazy open being broken.
	good := New(Config{Logger: quietLogger(), AppDBPath: appDB(t),
		DecksDir: t.TempDir(), RequireAuth: true})
	if _, present := good.accountsDB(); !present {
		t.Error("a real app.db was refused by the lazy open")
	}
}
