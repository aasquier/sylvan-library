package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The dev-local profiler mount (daybreak ruling 2026-08-23: half (a) yes,
// half (b) no): `/debug/pprof/` exists exactly when auth is off, which is a
// laptop by this repo's own definition (configComplaints). The two halves of
// that sentence are two different bugs waiting — a profiler missing from the
// laptop is the hot-spot patrol blind again, and a profiler reachable on the
// deployed instance is a heap walk of a process holding session tokens — so
// both are pinned here against the real boot, not against devProfiler alone:
// the condition in serveOn is the load-bearing line.

// profilerGet asks a booted server for one path and hands back the response
// with its body drained and read, so the server goroutine is never left
// mid-write.
func profilerGet(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("reading %s: %v", url, err)
	}
	return resp, string(body)
}

func TestTheProfilerServesTheLaptopOnly(t *testing.T) {
	t.Parallel()

	t.Run("auth off mounts it in front of the door", func(t *testing.T) {
		t.Parallel()
		d := scratchDeployment(t) // auth off: the laptop shape
		_, base, done := bootServer(t, d)
		resp, err := waitForHealth(t, base+"/api/health", done)
		if err != nil {
			t.Fatalf("the server never answered: %v", err)
		}
		_ = resp.Body.Close()

		// The index is pprof's own front page and names the profile family.
		idx, body := profilerGet(t, base+"/debug/pprof/")
		if idx.StatusCode != http.StatusOK {
			t.Fatalf("/debug/pprof/ answered %d with auth off", idx.StatusCode)
		}
		for _, profile := range []string{"goroutine", "heap"} {
			if !strings.Contains(body, profile) {
				t.Errorf("the profiler index does not offer %q", profile)
			}
		}
		// One real profile through Index's lookup path, because a mux that
		// serves only the front page is a shelf with no books on it.
		gor, body := profilerGet(t, base+"/debug/pprof/goroutine?debug=1")
		if gor.StatusCode != http.StatusOK || !strings.Contains(body, "goroutine") {
			t.Errorf("the goroutine profile answered %d: %.80s", gor.StatusCode, body)
		}
		// And the app still serves through the wrap — the mount must add a
		// prefix, never shadow the door.
		if health, _ := profilerGet(t, base+"/api/health"); health.StatusCode != http.StatusOK {
			t.Errorf("/api/health answered %d through the profiler wrap", health.StatusCode)
		}
	})

	t.Run("auth on never installs it", func(t *testing.T) {
		t.Parallel()
		d := scratchDeployment(t)
		d.RequireAuth = true
		_, base, done := bootServer(t, d)
		resp, err := waitForHealth(t, base+"/api/health", done)
		if err != nil {
			t.Fatalf("the server never answered: %v", err)
		}
		_ = resp.Body.Close()

		// With the wrap absent these paths reach the door and get its page
		// treatment for an unknown path — the 404 body here, the SPA shell on
		// an instance with a bundle — and either is fine. What must never
		// come back is the profiler's own answer: its index page, or a
		// profile download (every pprof profile ships as an attachment, which
		// nothing in the door ever sets). `/debug/pprof/profile` is left off
		// this list on purpose — under the mutation this guards against it
		// spends 30 seconds sampling CPU before answering.
		for _, path := range []string{"/debug/pprof/", "/debug/pprof/heap"} {
			resp, body := profilerGet(t, base+path)
			if strings.Contains(body, "Types of profiles available") {
				t.Errorf("%s served the profiler index with auth on", path)
			}
			if cd := resp.Header.Get("Content-Disposition"); cd != "" {
				t.Errorf("%s answered as a download (%q) with auth on: that is "+
					"a profile leaving a process that holds session tokens", path, cd)
			}
		}
	})
}
