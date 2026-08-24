package main

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"testing"
)

// The boot sequence's two claims, held here rather than believed: that the
// summary line says what this process actually decided, and that it says none
// of what it was trusted with.
//
// Neither test uses `t.Parallel`: every one of them drives the process
// environment, which is the same thing the code under test reads.

func renderSummary(t *testing.T, webDist, tarot string, pool bool) string {
	t.Helper()
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("configuration",
		bootSummary(webDist, tarot, pool)...)
	return buf.String()
}

func TestTheBootSummarySaysWhatWasDecided(t *testing.T) {
	t.Setenv("MTGLAB_REQUIRE_AUTH", "1")
	t.Setenv("MTGLAB_SECURE_COOKIES", "")
	t.Setenv("MTGLAB_DATA_DIR", "/data")
	t.Setenv("MTGLAB_DECKS_DIR", "/data/decks")
	t.Setenv("MTGLAB_BASE_URL", "https://example.test")
	t.Setenv("RESEND_API_KEY", "")
	line := renderSummary(t, "web_dist", "assets/tarot", true)

	for _, want := range []string{
		"auth=on", "cookies=secure", "data_dir=/data", "decks_dir=/data/decks",
		"web_dist=web_dist", "tarot=assets/tarot", "pool=present",
		"base_url=https://example.test", "mail=console",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("the boot summary does not say %s:\n%s", want, line)
		}
	}

	t.Setenv("MTGLAB_REQUIRE_AUTH", "0")
	t.Setenv("MTGLAB_BASE_URL", "")
	off := renderSummary(t, "web_dist", "assets/tarot", false)
	for _, want := range []string{"auth=off", "cookies=plain", "pool=absent"} {
		if !strings.Contains(off, want) {
			t.Errorf("the boot summary does not say %s:\n%s", want, off)
		}
	}
}

// secretShaped is how a credential is spelled in this project's environment.
var secretShaped = regexp.MustCompile(`(KEY|TOKEN|SECRET)`)

// TestTheBootSummaryLeaksNoSecret sets every credential-shaped switch this
// tree reads to a value nothing else could produce, and demands that none of
// them come back out of the line.
//
// The list is *discovered* rather than typed, off the same walk that holds
// `.env.example` honest — so a credential added tomorrow is covered the day it
// lands rather than the day somebody remembers this test. That matters more
// here than anywhere else in the suite: a key printed into a deployment's log
// is leaked to everyone who can read the log, forever, and the leak is silent.
func TestTheBootSummaryLeaksNoSecret(t *testing.T) {
	shipped, _ := switchesInGoSource(t, repoRoot(t))
	const sentinel = "SENTINEL-do-not-log-"
	set := 0
	for name := range shipped {
		if !secretShaped.MatchString(name) {
			continue
		}
		t.Setenv(name, sentinel+name)
		set++
	}
	if set < 3 {
		t.Fatalf("only %d credential-shaped switches found; the discovery has "+
			"stopped working and this test would pass on anything", set)
	}
	line := renderSummary(t, "web_dist", "assets/tarot", true)
	if strings.Contains(line, sentinel) {
		t.Errorf("the boot summary printed a credential:\n%s", line)
	}
}

func TestTheComplaintsNameOnlyRelationshipsThatAreActuallyBroken(t *testing.T) {
	// Auth off is a laptop: none of these relationships exists to break.
	t.Setenv("MTGLAB_REQUIRE_AUTH", "0")
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("MTGLAB_BASE_URL", "")
	t.Setenv("MTGLAB_EMAIL_FROM", "")
	t.Setenv("MTGLAB_ADMIN_EMAIL", "")
	if got := configComplaints(); len(got) != 0 {
		t.Fatalf("auth off should complain about nothing, got %v", got)
	}

	// Auth on with none of its partners set: every one of them is a deployment
	// that answers pages and fails the first password reset.
	t.Setenv("MTGLAB_REQUIRE_AUTH", "1")
	got := configComplaints()
	if len(got) != 4 {
		t.Fatalf("expected four complaints from a bare auth-on deployment, got %d: %v", len(got), got)
	}
	for _, want := range []string{"RESEND_API_KEY", "MTGLAB_BASE_URL", "MTGLAB_EMAIL_FROM", "MTGLAB_ADMIN_EMAIL"} {
		found := false
		for _, c := range got {
			if strings.Contains(c, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("nothing complained about %s: %v", want, got)
		}
	}

	// And a deployment that set them all is quiet again -- the half that keeps
	// a warning worth reading.
	t.Setenv("RESEND_API_KEY", "re_not_a_real_key")
	t.Setenv("MTGLAB_BASE_URL", "https://example.test")
	t.Setenv("MTGLAB_EMAIL_FROM", "sylvan-library <no-reply@example.test>")
	t.Setenv("MTGLAB_ADMIN_EMAIL", "someone@example.test")
	if got := configComplaints(); len(got) != 0 {
		t.Fatalf("a configured deployment should complain about nothing, got %v", got)
	}

	// The loopback check is about the *value*, not about the variable being
	// unset: an instance told to mail links to the local port has the same
	// problem as one that was never told anything.
	t.Setenv("MTGLAB_BASE_URL", "http://127.0.0.1:8765")
	if got := configComplaints(); len(got) != 1 || !strings.Contains(got[0], "MTGLAB_BASE_URL") {
		t.Fatalf("an explicit loopback base URL should still complain, got %v", got)
	}
}
