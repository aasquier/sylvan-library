package main

import (
	"strings"
	"testing"
)

// `cards show` against the 21-card pool: rule 1's lookup as a command.
//
// Every test here is parallel, which is new (ADR 40): pointing the command at
// a fixture pool used to mean exporting MTGLAB_DATA_DIR onto the process, and
// reading what it said used to mean swapping [os.Stdout] for a pipe. Both are
// now arguments -- see `clitest_test.go`.

func TestCardsShowPrintsThePoolsFacts(t *testing.T) {
	t.Parallel()
	out, err := scratchDeployment(t).withPool(t).run(t, "cards", "show", "Sol Ring")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	for _, want := range []string{"Sol Ring", "{1}", "Artifact", "{C}{C}"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestCardsShowNamesWhatThePoolLacks(t *testing.T) {
	t.Parallel()
	_, err := scratchDeployment(t).withPool(t).run(t,
		"cards", "show", "Sol Ring", "Imaginary Card, the Unprinted")
	if err == nil || !strings.Contains(err.Error(), "not in the pool: Imaginary Card, the Unprinted") {
		t.Fatalf("want the not-in-pool refusal, got %v", err)
	}
}

// A pool that is not there at all is a refusal naming the fix, not a crash --
// the state a fresh machine is in before its first `data refresh`.
func TestCardsShowRefusesWithoutAPool(t *testing.T) {
	t.Parallel()
	_, err := scratchDeployment(t).run(t, "cards", "show", "Sol Ring")
	if err == nil || !strings.Contains(err.Error(), "mtglab data refresh") {
		t.Fatalf("want the no-pool refusal naming the fix, got %v", err)
	}
}

// The keyless report, which is the state CI runs in and the state a machine
// is in before `fly secrets set`. No call is spent: a [claude.Endpoint] with
// no credential answers from the value alone.
func TestClaudeCheckWithoutAKeyReportsUnavailable(t *testing.T) {
	t.Parallel()
	out, err := scratchDeployment(t).run(t, "claude", "check")
	if err == nil {
		t.Fatal("a keyless check must exit non-zero")
	}
	if !strings.Contains(out, "status    unavailable") || !strings.Contains(out, "reason") {
		t.Errorf("unexpected keyless report:\n%s", out)
	}
}

// `--tools` lists the read-only roster, and does it on a keyless machine --
// the roster is a fact about this binary, not about the pipe being open.
func TestClaudeCheckListsTheReadOnlyRoster(t *testing.T) {
	t.Parallel()
	out, _ := scratchDeployment(t).run(t, "claude", "check", "--tools")
	if !strings.Contains(out, "status    unavailable") {
		t.Errorf("a keyless check reported otherwise:\n%s", out)
	}
}
