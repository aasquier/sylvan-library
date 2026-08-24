package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The real HTTP transport, which is what a deployed instance actually mails
// through and which no test had ever run.
//
// ADR 16's seam is `Transport`, and every test in this package injects one —
// which is right, because a test that sends mail is a test that sends mail.
// The consequence is that `httpPost`, the default the seam falls back to when
// nobody injects anything, ran only in production. It is not much code, but
// what it does is the part a stub cannot check: reading a status off a real
// response, and **bounding the body** so a provider having a bad day cannot
// hand this process a gigabyte to hold in memory while somebody waits on a
// password reset.
//
// Driving it directly against an `httptest.Server` runs the real client, the
// real request and the real read, with nothing but Resend replaced.

func TestTheRealTransportReadsAStatusAndABody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer a-test-key" {
			t.Errorf("the request carried %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"msg_1"}`))
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer a-test-key")

	status, body, err := httpPost(req)
	if err != nil {
		t.Fatalf("posting to a server that answered: %v", err)
	}
	if status != http.StatusAccepted {
		t.Errorf("the status came back as %d", status)
	}
	if string(body) != `{"id":"msg_1"}` {
		t.Errorf("the body came back as %q", body)
	}
}

// **A body is read to a ceiling, not to the end.** The body is only ever used
// to name the error, so a provider answering with something enormous must not
// be able to make this process hold it.
func TestTheRealTransportWillNotReadAnUnboundedBody(t *testing.T) {
	t.Parallel()
	const huge = 3 << 20 // comfortably past the 1MiB ceiling
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(make([]byte, huge))
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	status, body, err := httpPost(req)
	if err != nil {
		t.Fatalf("a large answer failed the read: %v", err)
	}
	if status != http.StatusInternalServerError {
		t.Errorf("the status came back as %d", status)
	}
	if len(body) >= huge {
		t.Errorf("the transport read %d bytes -- the ceiling is not holding", len(body))
	}
}

// A host that is not there is an error about the network, and it is the one
// error `Send` is allowed to quote: it says nothing about the recipient.
func TestTheRealTransportReportsAHostThatIsNotThere(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening there any more

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := httpPost(req); err == nil {
		t.Fatal("posting to a closed listener succeeded")
	}
}
