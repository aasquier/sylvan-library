package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The two generic job routes, and they are the **hybrid** PLAN section 10
// argued for rather than the plain port it argued against.
//
// # Why they could not flip before, and why they can now
//
// A job lives in the registry of the process that submitted it, and a registry
// is per-process. Until 2026-08-22 every one of the eight job-submitting
// families was Python's, so a Go handler here would have answered from a
// registry nothing wrote to: `/api/jobs` empty forever, `/api/jobs/{id}` a 404
// for every id the app hands out -- and `followJob` reads a 404 as *"the
// server restarted and the run died with it"*, so all seven of its call sites
// would have reported every long job lost the instant it was submitted.
// `TestTheGenericJobRoutesAreStillPythons` was the tripwire; it is gone
// because the thing it guarded has now happened on purpose.
//
// PLAN section 10 left exactly two shapes: all eight families and both these
// routes in one change, or the hybrid built *with the first family flip*,
// where both of its branches are finally live. **The first is not reachable**
// -- five of the eight families need `claude/` and one needs `sim/tier3`, and
// neither engine has crossed -- so the second is not a preference, it is the
// only shape available. The sim family is what makes the Go branch non-empty;
// `/api/sim/forge` and the five Claude families keep the proxy branch
// non-empty. The argument against building this *today* was that every live
// branch would be the proxy, and that stopped being true in this commit.
//
// # The rule each route follows
//
// **One by id: ours if we own it, otherwise ask Python.** A Go registry miss
// is not the same question as "no such job", and this does not try to tell
// them apart -- it proxies, and lets the process that might hold the job
// answer. That also settles ADR 5 without a second rule: a Go job belonging to
// somebody else misses the owner-scoped lookup, gets proxied, and Python 404s
// an id it has never seen. Somebody else's job is a 404 either way -- arrived
// at rather than asserted.
//
// **The list is the union.** A caller's jobs are spread across two registries
// and a list showing one of them would be wrong in the way nobody can see: the
// missing rows look exactly like jobs that were never submitted.
//
// # Rows stay raw, and that is the load-bearing decision here
//
// Python's rows are held as `json.RawMessage` and written back byte for byte.
// Decoding them into a struct and re-encoding would be the obvious shape and
// it is a **wire bug**: `Payload.Result` is an `any`, a decoded JSON object
// lands in it as a `map[string]any`, and `encoding/json` **sorts a map's
// keys** where a Python dict keeps insertion order. Every simulation result,
// every dossier, every theme proposal would come back through this route with
// its fields alphabetised. That is not hypothetical -- it is the regression
// that shipped on the deck page's Notes tab from v159 to v166, found the same
// way and written up in the same words.
//
// So this route parses exactly one field out of each row, the one it has to
// sort on, and touches nothing else.
//
// The sort is `created_at` **as text, descending**, which is what both
// registries already do (`all_jobs` is `sorted(..., reverse=True)` over the
// ISO string; `Registry.All` is the same comparison). Text and not a parsed
// time on purpose: `internal/jobs` reproduces Python's `isoformat()` exactly,
// elided microseconds and `+00:00` included, and every field in it is fixed
// width -- so string order *is* chronological order, and parsing would add a
// second representation for no gain.
//
// A cross-registry tie goes to Go's row. That is a choice with nothing behind
// it -- two jobs would have to be created in the same microsecond in two
// different processes -- and is written down only so it is not discovered.

// jobRow is one row on its way through, and the sort key read off it.
type jobRow struct {
	createdAt string
	raw       json.RawMessage
	fromGo    bool
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	scope := auth.ScopeFrom(r.Context())

	// Python's half first, so a failure to reach it fails the whole route
	// rather than quietly serving half a list. A partial list is the failure
	// this union exists to prevent; answering with ours alone would be
	// committing it in the name of resilience.
	rows, ok := a.upstreamJobs(w, r)
	if !ok {
		return
	}

	if a.jobs != nil {
		for _, job := range a.jobs.All(scope.UserID) {
			payload := job.Payload()
			raw, err := wire.Marshal(payload)
			if err != nil {
				a.log.Error("a job did not serialise", "id", payload.ID, "err", err)
				continue
			}
			rows = append(rows, jobRow{createdAt: payload.CreatedAt, raw: raw, fromGo: true})
		}
	}

	// Stable, so the tie rule above is the one that applies and not whatever a
	// pivot happened to do. Each half arrives already sorted, which is what
	// makes stability enough to keep the merge deterministic.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].createdAt != rows[j].createdAt {
			return rows[i].createdAt > rows[j].createdAt
		}
		return rows[i].fromGo && !rows[j].fromGo
	})

	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, row := range rows {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(row.raw)
	}
	buf.WriteByte(']')
	wire.Raw(w, http.StatusOK, buf.Bytes())
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request) {
	scope := auth.ScopeFrom(r.Context())
	if a.jobs != nil {
		if job := a.jobs.Get(r.PathValue("job_id"), scope.UserID); job != nil {
			wire.JSON(w, http.StatusOK, job.Payload())
			return
		}
	}
	a.proxyOrNotFound(w, r)
}

// upstreamJobs runs the request through the proxy and reads back the list it
// answers with, one raw row at a time.
//
// The proxy is an `http.Handler` and writes into a `ResponseWriter`, so the
// only way to read what it said is to hand it one that keeps the bytes. That
// is what `httptest.NewRecorder` is, and using it outside a test is
// deliberate rather than lazy: the alternative is a second HTTP client here,
// with its own timeout, retry and header rules -- a second proxy that would
// have to agree with the real one forever.
//
// **A non-200 from Python is passed through verbatim.** If the upstream is
// refusing -- 401 after a restart, 503 while it boots -- the caller must see
// that refusal rather than an empty list with our rows appended to it.
func (a *API) upstreamJobs(w http.ResponseWriter, r *http.Request) ([]jobRow, bool) {
	if a.upstream == nil {
		return nil, true
	}
	rec := httptest.NewRecorder()
	a.upstream.ServeHTTP(rec, r)
	res := rec.Result()
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		for name, values := range res.Header {
			for _, v := range values {
				w.Header().Add(name, v)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(rec.Body.Bytes())
		return nil, false
	}

	var raws []json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raws); err != nil {
		a.log.Error("the upstream job list did not decode", "err", err)
		wire.Detail(w, http.StatusBadGateway, "the job list is unavailable")
		return nil, false
	}
	rows := make([]jobRow, 0, len(raws))
	for _, raw := range raws {
		// Only the sort key is parsed. See the note above on why nothing else
		// is touched.
		var head struct {
			CreatedAt string `json:"created_at"`
		}
		// A row with no readable `created_at` sorts last rather than failing
		// the list: this is a view, and one that refuses to render because a
		// single row is malformed is worse than one that shows it.
		_ = json.Unmarshal(raw, &head)
		rows = append(rows, jobRow{createdAt: head.CreatedAt, raw: raw})
	}
	return rows, true
}

// proxyOrNotFound hands the request to the proxy, or answers 404 when there is
// no upstream to ask -- a laptop with no Python behind it, and every instance
// after Phase 8.
func (a *API) proxyOrNotFound(w http.ResponseWriter, r *http.Request) {
	if a.upstream == nil {
		wire.Detail(w, http.StatusNotFound, "no such job")
		return
	}
	a.upstream.ServeHTTP(w, r)
}
