package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
)

// `mtglab claude check` against a stub Anthropic.
//
// This is the runbook's answer to "is the key live" — run after a rotation, or
// in six weeks when something 401s and the question is whether the integration
// broke or the key simply lapsed (docs/HOSTING.md). Its happy path had never
// run: the command built its own endpoint with `claude.EndpointFromEnv()`, so
// the only way to exercise the report was to spend a real call against a real
// key, which no test may do. `docs/polish/COVERAGE.md` named this as the one
// network-blocked path that could still be reached if an `Endpoint` were
// threaded in. ADR 40 threaded it, and this is what that bought.
//
// Nothing here reaches the network. `claude.EndpointAt` points the SDK at an
// `httptest.Server`, so the real request building, the real transport and the
// real response decoding all run — only Anthropic is missing.

// stubAnthropic answers one Messages call with `body`, or with `status` when
// that is not 200.
func stubAnthropic(t *testing.T, status int, body string) claude.Endpoint {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"nope"}}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return claude.EndpointAt(srv.URL, "test-key-not-a-real-one")
}

// oneReply is a Messages response carrying a single line of text.
func oneReply(t *testing.T, text string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id": "msg_test", "type": "message", "role": "assistant",
		"model":       "claude-test",
		"content":     []map[string]any{{"type": "text", "text": text}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 12, "output_tokens": 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The report an open pipe produces: the model, the reply, and what it cost.
// Every one of those is a fact the runbook reads.
func TestClaudeCheckReportsAnOpenPipe(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.Pipe = stubAnthropic(t, http.StatusOK, oneReply(t, "Squire."))

	out, err := d.run(t, "claude", "check")
	if err != nil {
		t.Fatalf("an answering pipe reported closed: %v\n%s", err, out)
	}
	for _, want := range []string{"served by", "reply", "tokens", "Squire."} {
		if !strings.Contains(out, want) {
			t.Errorf("the report is missing %q:\n%s", want, out)
		}
	}
	// The accounting is the half a rotation check is actually for: a report
	// that says "open" without saying what it cost cannot be read as
	// evidence the call happened.
	if !strings.Contains(out, "12 in / 7 out") {
		t.Errorf("the report does not carry the token counts:\n%s", out)
	}
	// **Never the credential.** Not the key, not the URL it was sent to.
	if strings.Contains(out, "test-key-not-a-real-one") {
		t.Errorf("the report printed the credential:\n%s", out)
	}
	if strings.Contains(out, "status    unavailable") {
		t.Errorf("an answering pipe was reported unavailable:\n%s", out)
	}
}

// `--tools` lists the read-only roster beside the report. The roster is
// sorted, because an operator comparing two runs should not have to diff a
// map's iteration order.
func TestClaudeCheckListsTheToolRosterInOrder(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.Pipe = stubAnthropic(t, http.StatusOK, oneReply(t, "Squire."))

	out, err := d.run(t, "claude", "check", "--tools")
	if err != nil {
		t.Fatalf("check --tools: %v\n%s", err, out)
	}
	if !strings.Contains(out, "read-only") {
		t.Errorf("the roster does not say the tools are read-only:\n%s", out)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(line, "    ") && trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if len(names) < 2 {
		t.Fatalf("the roster listed %d tools:\n%s", len(names), out)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("the roster is not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

// A key that the API refuses is a nonzero exit and a reason — the shape of the
// answer six weeks from now, when the question is whether the key lapsed or
// the integration broke. The distinction is the reason line.
func TestClaudeCheckReportsARefusedKeyWithItsReason(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.Pipe = stubAnthropic(t, http.StatusUnauthorized, "")

	out, err := d.run(t, "claude", "check")
	if err == nil {
		t.Fatalf("a refused key exited zero:\n%s", out)
	}
	if !strings.Contains(out, "status    unavailable") {
		t.Errorf("a refused key was not reported unavailable:\n%s", out)
	}
	if !strings.Contains(out, "reason") {
		t.Errorf("a refused key carried no reason:\n%s", out)
	}
	// The model is reported either way: "which model did you try" is the
	// first thing anybody asks about a failed check.
	if !strings.Contains(out, "model") {
		t.Errorf("the failing report does not name the model:\n%s", out)
	}
}
