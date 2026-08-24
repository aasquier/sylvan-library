package flymetrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The real HTTP transport behind the metrics panel, which every other test
// here injects around.
//
// The panel's whole design is that it never raises: a metrics panel that can
// 500 the admin page is a monitoring tool that takes the dashboard down with
// the thing it monitors. That promise rests on this function returning a
// status and a body rather than panicking, and on it bounding what it reads —
// Fly's API answering with something enormous must not become this process's
// memory while an admin waits on a page.
//
// Calling it directly against an `httptest.Server` runs the real request
// building, the real client and the real read, with nothing but Fly replaced.

func TestTheRealTransportSendsItsHeadersAndReadsTheAnswer(t *testing.T) {
	t.Parallel()
	var seen http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer srv.Close()

	status, body, err := realTransport(srv.URL, map[string]string{
		"Authorization": "Bearer a-test-token",
		"Accept":        "application/json",
	})
	if err != nil {
		t.Fatalf("querying a server that answered: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("the status came back as %d", status)
	}
	if string(body) != `{"status":"success"}` {
		t.Errorf("the body came back as %q", body)
	}
	// Every header the caller named reached the wire. The token is the one
	// that matters: without it Fly answers 401 and the panel reports a
	// configuration problem that is really a plumbing one.
	if got := seen.Get("Authorization"); got != "Bearer a-test-token" {
		t.Errorf("the request carried Authorization %q", got)
	}
	if got := seen.Get("Accept"); got != "application/json" {
		t.Errorf("the request carried Accept %q", got)
	}
}

// The read is bounded, for the same reason the mail transport's is: the body
// is parsed into a small answer, and a large one must not be held whole.
func TestTheRealTransportWillNotReadAnUnboundedBody(t *testing.T) {
	t.Parallel()
	const huge = 3 << 20
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, huge))
	}))
	defer srv.Close()

	_, body, err := realTransport(srv.URL, nil)
	if err != nil {
		t.Fatalf("a large answer failed the read: %v", err)
	}
	if len(body) >= huge {
		t.Errorf("the transport read %d bytes -- the ceiling is not holding", len(body))
	}
}

// A target that is not a URL, and a host that is not there: both are errors
// the panel turns into `configured: true, ok: false` with a reason, never a
// raise.
func TestTheRealTransportReportsWhatItCouldNotReach(t *testing.T) {
	t.Parallel()
	if _, _, err := realTransport("://not-a-url", nil); err == nil {
		t.Error("a malformed target built a request anyway")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	if _, _, err := realTransport(url, nil); err == nil {
		t.Error("querying a closed listener succeeded")
	}
}
