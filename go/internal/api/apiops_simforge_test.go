package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// `POST /api/sim/forge`'s three refusals before a match is ever planned, and
// the invite route's two.
//
// Everything about this route is tested against a library that opens and a
// worker that answers. These are the three states where it does not: the
// library cannot be assembled at all, one owner's shelf cannot be reached, and
// the pre-flight broke for a reason that is nothing to do with coverage.

const forgeAt = "/api/sim/forge"

// aForgeAsk is the body the route wants: two decks the fixture library holds.
const aForgeAsk = `{"a_slug":"kaheera","b_slug":"mono-green","games":1}`

// **A library that cannot be assembled refuses the match.**
//
// With a maintainer configured, assembling the library reads `app.db` to resolve
// them — so a volume that has gone takes the route out before it has looked at a
// deck. A 200 here is impossible, but a *panic* was not, and a sentence is what
// the Simulator renders.
func TestAMatchOverALibraryThatCannotBeAssembledIsRefused(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", "file:"+appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t),
		DecksDir: decksDir(t), AppDB: db, AdminEmail: "alice@example.com",
		Jobs: jobs.New(jobs.Config{Logger: quietLogger()})})

	status, payload, raw := callAs(t, a, alice, "POST", forgeAt, aForgeAsk)
	if status == http.StatusOK || status == http.StatusAccepted {
		t.Fatalf("a match was planned over a library that will not open: %s", raw)
	}
	if said, _ := payload["detail"].(string); strings.TrimSpace(said) == "" {
		t.Errorf("it answered %d with nothing to read: %s", status, raw)
	}
}

// **One owner's shelf failing refuses the match**, and the sweep is what finds
// it: the decks are resolved one owner at a time, and the budget lands wherever
// today's statement order puts it.
//
// No maintainer configured, so assembling the library costs no statement and the
// failure lands one layer in — on the lookup that turns an owner's name into
// their shelf (lever 20's shape, one route along).
func TestAMatchRefusesWhenAnOwnersShelfCannotBeReached(t *testing.T) {
	t.Parallel()
	refused, allowed, settled := 0, 0, 0
	for budget := 0; budget <= 16 && settled < 2; budget++ {
		path := t.TempDir() + "/app.db"
		db, fault, err := authtest.OpenFaulty(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := auth.Create(t.Context(), db, "bob", "bob@example.com", false); err != nil {
			t.Fatal(err)
		}
		a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t),
			DecksDir: decksDir(t), AppDB: db, AppWriteDB: db, AppDBPath: path,
			// A worker URL, so the arena gate — which runs before the decks are
			// resolved — lets the request through to the lookup this is about.
			Forge: tier3.Settings{WorkerURL: "https://worker.invalid"},
			Jobs:  jobs.New(jobs.Config{Logger: quietLogger()})})

		fault.After(budget)
		status, _, raw := callAs(t, a, bob, "POST", forgeAt,
			`{"a_slug":"kaheera","b_slug":"mono-green","games":1}`)
		fault.Heal()
		_ = db.Close()

		what := fmt.Sprintf("a match at budget %d", budget)
		answersHonestly(t, what, status, raw)
		if status == http.StatusOK || status == http.StatusAccepted {
			t.Errorf("%s was planned against decks this caller does not "+
				"have: %s", what, raw)
		}
		// Both floors, as a *class* rather than as a success: a statement
		// failure is the route's 500, and getting past the owner lookup is the
		// 404 that says the shelf was read and holds no such deck. One of each
		// is what makes the sweep a measurement rather than one state counted
		// seventeen times.
		switch status {
		case http.StatusInternalServerError:
			refused++
			settled = 0
		case http.StatusNotFound:
			allowed++
			settled++
		}
	}
	if refused == 0 {
		t.Error("no budget in the sweep made the shelf lookup fail, so the " +
			"sweep never reached a failure")
	}
	if allowed == 0 {
		t.Error("no budget got past the shelf lookup, so the sweep is " +
			"measuring a broken fixture rather than the route")
	}
}

// **A pre-flight that broke for a reason of its own is a 500 with a sentence.**
//
// Two of the three arms are known states — coverage refused, and the worker not
// installed — and the third is everything else: a worker that answers the
// coverage call with something that is not an answer at all, which is a proxy or
// a half-deployed image rather than a fact about the decks. The diagnosis goes
// to the log and what reaches the room names no machinery.
func TestAPreflightThatBrokeForItsOwnReasonIs500WithASentence(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
		// A 200 that is not an answer: the shape a proxy in front of a worker
		// gives when the worker itself is not there.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`<html>not the worker</html>`))
	}))
	defer srv.Close()

	appDB, err := auth.Open(appDB(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = appDB.Close() })
	settings := tier3.Settings{WorkerURL: srv.URL}
	a := New(Config{Logger: quietLogger(), Pool: pooltest.Open(t),
		DecksDir: decksDir(t), AdminEmail: "alice@example.com", AppDB: appDB,
		Forge: settings, Jobs: jobs.New(jobs.Config{Logger: quietLogger()}),
		ForgeWorker: &tier3.Worker{Settings: settings, Boot: 5 * time.Second,
			Sleep: func(time.Duration) {}}})

	status, payload, raw := callAs(t, a, alice, "POST", forgeAt, aForgeAsk)
	if status != http.StatusInternalServerError {
		t.Fatalf("a pre-flight that broke answered %d: %s", status, raw)
	}
	said, _ := payload["detail"].(string)
	if strings.TrimSpace(said) == "" {
		t.Fatalf("it answered %d with nothing to read: %s", status, raw)
	}
	// Commandment 10's one standing exception is the word Forge; everything
	// underneath it stays out of the room.
	for _, machinery := range []string{"127.0.0.1", "http://", "json",
		"html", "/coverage", "unmarshal"} {
		if strings.Contains(strings.ToLower(said), strings.ToLower(machinery)) {
			t.Errorf("the sentence a player reads carries %q: %q", machinery, said)
		}
	}
}

// ---- the invite route ------------------------------------------------------

// **An invite on an instance with mail and no accounts database is refused.**
//
// The order is deliberate: mail first, because an instance with no mail
// configured should say so rather than create an account whose invite can never
// be sent. Which leaves the arm past it — mail is fine and there is nowhere to
// write the account — reachable only on an instance that has one and not the
// other, and that is a real deployment: the secret is set and the volume did not
// mount.
func TestAnInviteWithMailAndNoAccountsDatabaseIsRefused(t *testing.T) {
	t.Parallel()
	a := New(Config{Logger: quietLogger(), DecksDir: t.TempDir(),
		EmailSender: &recordedSender{}})

	status, payload, raw := callAs(t, a, adminScope, "POST", "/api/admin/users",
		`{"email":"newcomer@example.com"}`)
	if status == http.StatusOK || status == http.StatusCreated {
		t.Fatalf("an invite was issued with nowhere to write it: %s", raw)
	}
	if said, _ := payload["detail"].(string); strings.TrimSpace(said) == "" {
		t.Errorf("it answered %d with nothing to read: %s", status, raw)
	}
}

// **Inviting an address that is already claimed, over a handle that stops
// answering.** The route reads the account, then asks whether it has a password,
// and only the second question distinguishes "send them a reset instead" from
// "here is a fresh invite" — so a statement failure between the two must refuse
// rather than issue a second invite to somebody who already has an account.
func TestInvitingAClaimedAddressRefusesAHandleThatStopsAnswering(t *testing.T) {
	t.Parallel()
	refused, allowed, settled := 0, 0, 0
	for budget := 0; budget <= 16 && settled < 2; budget++ {
		path := t.TempDir() + "/app.db"
		db, fault, err := authtest.OpenFaulty(path)
		if err != nil {
			t.Fatal(err)
		}
		// An account that has already claimed its address: the one state where
		// the password question is asked at all.
		claimed, err := auth.Create(t.Context(), db, "bob", "bob@example.com", false)
		if err != nil {
			t.Fatal(err)
		}
		hash, err := seededHash()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(),
			"UPDATE users SET password_hash = ? WHERE id = ?",
			hash, claimed.ID); err != nil {
			t.Fatal(err)
		}
		a := New(Config{Logger: quietLogger(), AppDB: db, AppWriteDB: db,
			AppDBPath: path, DecksDir: t.TempDir(),
			EmailSender: &recordedSender{}})

		fault.After(budget)
		status, _, raw := callAs(t, a, adminScope, "POST", "/api/admin/users",
			`{"email":"bob@example.com"}`)
		fault.Heal()
		_ = db.Close()

		what := fmt.Sprintf("inviting a claimed address at budget %d", budget)
		answersHonestly(t, what, status, raw)
		switch status {
		case http.StatusOK, http.StatusCreated:
			t.Errorf("%s issued an invite to an address that is already "+
				"claimed: %s", what, raw)
		case http.StatusConflict:
			// The right answer: they have claimed it, send a reset instead.
			allowed++
			settled++
		default:
			refused++
			settled = 0
		}
	}
	if refused == 0 {
		t.Error("the invite answered the conflict at every budget, so the " +
			"sweep never reached a failure")
	}
	if allowed == 0 {
		t.Error("the invite never reached the conflict, so the sweep is " +
			"measuring a broken fixture rather than the route")
	}
}
