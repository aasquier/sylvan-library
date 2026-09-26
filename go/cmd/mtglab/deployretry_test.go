package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The step that re-points the Forge worker after a deploy talks to the host's
// Machines API seven times, and **a single transient on any one of them fails
// the whole deploy job** — `set -euo pipefail` plus a bare `curl -fsS` is a
// one-shot call. It happened twice in the week of 2026-09-19: a 409 on the
// update, and a connection reset whose run reads `failure` over a release that
// had already deployed and was serving. Both healed on the next deploy, which
// is exactly what makes them expensive: a red check that is not about the code
// is the kind a person learns to skim.
//
// So every call in that step carries the retry flags, and this holds it —
// because the failure mode of the fix is somebody adding an eighth call later
// and not knowing. The step's own comment argues the flags; this asserts them.
//
// It is also the only proof available on a laptop. The `deploy` job runs on
// pushes to `main` alone, so no pull request ever exercises this shell: the
// choice is a guard that reads the file or nothing at all.
func TestEveryMachinesAPICallInTheDeployRetries(t *testing.T) {
	t.Parallel()
	const step = "Point the forge-worker machine at it, and put it to sleep"
	body := stepBody(t, step)

	// The flags themselves, and the one that matters: curl's own notion of a
	// "transient" error is timeouts plus a short list of status codes, and a
	// connection reset is on neither — so `--retry` without
	// `--retry-all-errors` would not have retried either real failure.
	for _, flag := range []string{"RETRY=(", "--retry 5", "--retry-all-errors", "--max-time"} {
		if !strings.Contains(body, flag) {
			t.Errorf("the step defines no %s, so its calls are one-shot", flag)
		}
	}

	calls := 0
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "curl ") {
			continue
		}
		calls++
		if !strings.Contains(trimmed, `"${RETRY[@]}"`) {
			t.Errorf("a call with no retry: %s", trimmed)
		}
	}
	// Anti-vacuity: a parser that found an empty block would pass every
	// assertion above and prove nothing.
	if calls < 6 {
		t.Errorf("found %d curl calls in %q; the step makes at least six, so "+
			"this test is reading the wrong block", calls, step)
	}
}

// stepBody returns one `ci.yml` step's lines, from its `- name:` to the next
// step at the same indent. Indent-scoped rather than regexp'd over the whole
// file, because every other job in there runs curls of its own and a file-wide
// sweep would judge them by this step's rule.
func stepBody(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("reading ci.yml: %v", err)
	}
	const head = "      - name: "
	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, line := range lines {
		if line == head+name {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("ci.yml has no step named %q", name)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], head) || strings.HasPrefix(lines[i], "      - uses:") ||
			(lines[i] != "" && !strings.HasPrefix(lines[i], "       ")) {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}
