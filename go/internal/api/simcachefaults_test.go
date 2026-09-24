package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/jobs"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/sim/cache"
)

// ADR 18's cache from the side nothing had driven: the replay, and the row
// that will not decode.
//
// A cached Tier 1 answer is a promise that a second ask costs nothing and says
// the same thing. `simruns_test.go` proves that for the mana run; the policy
// search — thirty-three simulations, the most expensive thing in the room and
// the one where a replay is worth most — was only ever computed, never
// replayed, so the whole hit path was unreached.
//
// The other half is the row the cache cannot read back. Nothing writes one;
// a schema that moved under a stored payload would, and the guard's job is to
// **recompute rather than to answer something it does not understand**. That
// is a branch a working deployment never takes and the only branch where a
// wrong choice is silent — a half-decoded result renders as a search whose
// rows are all zero.

// policyRig is a deck API with a real `sim_cache` behind it.
type policyRig struct {
	api *API
	reg *jobs.Registry
	db  *sql.DB
}

func newPolicyRig(t *testing.T) *policyRig {
	t.Helper()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbPath := appDB(t)
	db, err := auth.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := cache.Open(dbPath, quiet)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	write, err := auth.OpenReadWrite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = write.Close() })

	reg := jobs.New(jobs.Config{Logger: quiet})
	a := New(Config{Logger: quiet, Pool: pooltest.Open(t), DecksDir: decksDir(t),
		AdminEmail: "alice@example.com", AppDB: db, Jobs: reg, SimCache: store})
	return &policyRig{api: a, reg: reg, db: write}
}

func (r *policyRig) search(t *testing.T) map[string]any {
	t.Helper()
	status, payload := shelfPost(t, r.api, "/api/sim/policy",
		`{"slug":"kaheera","games":200,"turns":8,"seed":7}`)
	if status != http.StatusOK {
		t.Fatalf("the policy search answered %d: %v", status, payload)
	}
	return payload
}

// The second ask of the same question is a job born finished, and it says so:
// `cached` is true and the stamp is the first run's. Quoting a replay as a
// fresh answer is the one thing ADR 18 asks this path never to do.
func TestASecondPolicySearchIsAReplayAndSaysSo(t *testing.T) {
	t.Parallel()
	rig := newPolicyRig(t)

	first := rig.search(t)
	rig.reg.Wait()
	done := rig.reg.Get(first["id"].(string), alice.UserID)
	if done == nil || done.Status() != string(jobs.Done) {
		t.Fatalf("the first search did not finish: %v", done)
	}
	fresh, _ := asMap(t, done.Result())["cached"].(bool)
	if fresh {
		t.Error("the first, computed search reported itself cached")
	}

	second := rig.search(t)
	if second["status"] != string(jobs.Done) {
		t.Fatalf("the second ask was not born finished: %v", second["status"])
	}
	result, _ := second["result"].(map[string]any)
	if result["cached"] != true {
		t.Fatalf("the replay reports cached=%v", result["cached"])
	}
	if result["computed_at"] == nil {
		t.Error("the replay carries no stamp; a cached number has to say when")
	}
	// The deck's own state rides on the replay too -- a cached answer beside a
	// deck that has since broken must still carry the diagnosis.
	if result["deck_check"] == nil {
		t.Error("the replay dropped the deck check")
	}
	// And it is the same answer, not merely a fast one.
	if !sameRows(asMap(t, done.Result()), result) {
		t.Error("the replay differs from the run it replays")
	}
}

// A stored row the cache can no longer read back is recomputed rather than
// half-answered.
func TestAPolicyRowThatWillNotDecodeIsRecomputed(t *testing.T) {
	t.Parallel()
	rig := newPolicyRig(t)

	first := rig.search(t)
	rig.reg.Wait()
	if done := rig.reg.Get(first["id"].(string), alice.UserID); done == nil ||
		done.Status() != string(jobs.Done) {
		t.Fatalf("the first search did not finish: %v", done)
	}

	// The shape that arrives when a payload outlives the struct it was
	// written from. Nothing in the app writes this; a schema that moved would.
	res, err := rig.db.Exec(`UPDATE sim_cache SET result_json = '"not a policy result"'`)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		t.Fatal("no cached row to corrupt -- the search cached nothing")
	}

	second := rig.search(t)
	if second["status"] == string(jobs.Done) {
		t.Fatalf("an undecodable row was served as a finished answer: %v", second)
	}
	rig.reg.Wait()
	again := rig.reg.Get(second["id"].(string), alice.UserID)
	if again == nil || again.Status() != string(jobs.Done) {
		t.Fatalf("the recomputed search ended %v", again)
	}
	out := asMap(t, again.Result())
	if out["cached"] != false {
		t.Errorf("the recomputed answer reports cached=%v", out["cached"])
	}
	if rows, _ := out["rows"].([]any); len(rows) == 0 {
		t.Error("the recomputed answer has no rows")
	}
}

// asMap renders a result through the wire, which is how every caller sees it.
func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func sameRows(a, b map[string]any) bool {
	left, _ := json.Marshal(a["rows"])
	right, _ := json.Marshal(b["rows"])
	return string(left) == string(right)
}

// Every simulation route, on a process with no job registry.
//
// Nothing serving is in this state — a real process always has one — which is
// exactly why it is worth sweeping: it is the state every test that does not
// care about jobs runs in, so a route that reached into a nil registry would
// be found by whichever test happened to touch it rather than by this.
//
// Two things are asked, and the first is the one that discovers. **No route
// may hand back a job**, because there is nowhere for a job to live, and a
// payload carrying an id and a status is a promise the caller will poll
// forever — so the check is on the shape of the answer rather than on a list
// of which routes submit. (`/api/sim/shelf` legitimately answers inline: Tier
// 1.5 is arithmetic and runs in the request. It carries no id, so it passes,
// which is the sweep working rather than an exemption.) And a route that did
// reach the submit must refuse **503**: nothing is broken, there is just
// nowhere to put the work.
func TestNoSimulationRouteHandsBackAJobWhenThereIsNoRegistry(t *testing.T) {
	t.Parallel()
	a, done := deckAPI(t, noCredential, true)
	defer done()
	if a.jobs != nil {
		t.Fatal("this sweep needs an instance with no registry")
	}

	swept, refused := 0, 0
	for _, route := range a.Routes() {
		if route.Method != http.MethodPost || !strings.HasPrefix(route.Pattern, "/api/sim/") {
			continue
		}
		swept++
		status, payload, raw := callAs(t, a, alice, route.Method, route.Pattern,
			simBodyFor(route.Pattern))
		if _, isJob := payload["id"]; isJob {
			if _, hasStatus := payload["status"]; hasStatus {
				t.Errorf("%s handed back a job on a process with no registry "+
					"to run it in: %s", route.Pattern, raw)
				continue
			}
		}
		if status == http.StatusServiceUnavailable {
			refused++
			if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
				t.Errorf("%s answered 503 with nothing a person could read: %s",
					route.Pattern, raw)
			}
			continue
		}
		if status >= 400 {
			// A refusal of the request itself, which is a different sentence
			// and still has to be one.
			if detail, _ := payload["detail"].(string); strings.TrimSpace(detail) == "" {
				t.Errorf("%s answered %d with nothing a person could read: %s",
					route.Pattern, status, raw)
			}
		}
	}
	if swept < 3 {
		t.Errorf("only %d simulation routes were swept -- the filter has "+
			"stopped matching the route table", swept)
	}
	if refused < 2 {
		t.Errorf("only %d routes reached the submit and refused -- the bodies "+
			"below have stopped being valid, so this swept grammar checks", refused)
	}
}

// simBodyFor is a request body each simulation route would accept if the
// process could run the work, so a route refuses for the reason this sweep is
// about rather than at its own grammar check.
func simBodyFor(pattern string) string {
	if strings.HasSuffix(pattern, "/forge") {
		return `{"a_slug":"kaheera","b_slug":"mono-green-clean","games":1}`
	}
	return `{"slug":"kaheera","games":40}`
}
