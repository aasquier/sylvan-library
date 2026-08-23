package api

import (
	"bytes"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The two generic job routes: the view over the registry every job-submitting
// family writes. A job lives in the registry of the process that submitted
// it, jobs are owner-scoped (ADR 5: somebody else's job is a 404, never a
// 403), and the list is `created_at` descending — as text, which is safe
// because the registry's timestamps are fixed-width ISO instants, so string
// order is chronological order.
//
// Each row is marshalled on its own and the array assembled by hand, keeping
// every payload's field order exactly as its struct declares it — a slice
// through a generic encoder was how the Notes tab once shipped alphabetised.

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	scope := auth.ScopeFrom(r.Context())
	var buf bytes.Buffer
	buf.WriteByte('[')
	if a.jobs != nil {
		first := true
		for _, job := range a.jobs.All(scope.UserID) {
			payload := job.Payload()
			raw, err := wire.Marshal(payload)
			if err != nil {
				a.log.Error("a job did not serialise", "id", payload.ID, "err", err)
				continue
			}
			if !first {
				buf.WriteByte(',')
			}
			first = false
			buf.Write(raw)
		}
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
	wire.Detail(w, http.StatusNotFound, "no such job")
}
