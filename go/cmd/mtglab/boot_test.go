package main

import (
	"bytes"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The boot sequence's two claims, held here rather than believed: that the
// summary line says what this process actually decided, and that it says none
// of what it was trusted with.
//
// Every test here is parallel, which it could not be while the code under test
// read the process environment: describing a deployment meant installing one,
// and `t.Setenv` and `t.Parallel` cannot share a test. Both functions now take
// the [config.Config] they describe, so a deployment is a struct literal and
// four of them can be asserted at once.

func renderSummary(t *testing.T, cfg config.Config, webDist, tarot string, pool bool) string {
	t.Helper()
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("configuration",
		bootSummary(cfg, tier3.Settings{}, webDist, tarot, pool)...)
	return buf.String()
}

func TestTheBootSummarySaysWhatWasDecided(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		cfg  config.Config
		pool bool
		want []string
	}{
		{
			name: "a deployment",
			cfg: config.Config{
				RequireAuth: true, SecureCookies: true,
				DataDir: "/data", DecksDir: "/data/decks",
				BaseURL: "https://example.test",
			},
			pool: true,
			want: []string{
				"auth=on", "cookies=secure", "data_dir=/data",
				"decks_dir=/data/decks", "web_dist=web_dist",
				"tarot=assets/tarot", "pool=present",
				"base_url=https://example.test", "mail=console",
			},
		},
		{
			name: "a laptop with no pool",
			cfg:  config.Config{},
			pool: false,
			want: []string{"auth=off", "cookies=plain", "pool=absent"},
		},
		{
			name: "a deployment that has a mail provider",
			cfg:  config.Config{RequireAuth: true, ResendAPIKey: "re_not_a_real_key"},
			pool: true,
			want: []string{"mail=provider"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			line := renderSummary(t, tc.cfg, "web_dist", "assets/tarot", tc.pool)
			for _, want := range tc.want {
				if !strings.Contains(line, want) {
					t.Errorf("the boot summary does not say %s:\n%s", want, line)
				}
			}
		})
	}
}

// secretShaped is how a credential is spelled, in an environment variable and
// in the [config.Config] field that now carries it.
var secretShaped = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET)`)

// TestTheBootSummaryLeaksNoSecret fills every credential-shaped field of the
// configuration with a value nothing else could produce, and demands that none
// of them come back out of the line.
//
// The list is *discovered* rather than typed -- by walking [config.Config]'s
// own fields -- so a credential added tomorrow is covered the day it lands
// rather than the day somebody remembers this test. That matters more here
// than anywhere else in the suite: a key printed into a deployment's log is
// leaked to everyone who can read the log, forever, and the leak is silent.
//
// Walking the struct is strictly stronger than the environment walk this used
// to do. The summary can only print what it is handed, so the fields of the
// value it is handed are the complete list of things it could possibly leak --
// where a walk of the environment could only cover the variables somebody
// thought to look for, and could never prove it had found them all.
func TestTheBootSummaryLeaksNoSecret(t *testing.T) {
	t.Parallel()
	const sentinel = "SENTINEL-do-not-log-"

	cfg := config.Config{}
	v := reflect.ValueOf(&cfg).Elem()
	set := 0
	for i := range v.NumField() {
		name := v.Type().Field(i).Name
		if !secretShaped.MatchString(name) || v.Field(i).Kind() != reflect.String {
			continue
		}
		v.Field(i).SetString(sentinel + name)
		set++
	}
	if set < 1 {
		t.Fatalf("no credential-shaped field found on config.Config; the "+
			"discovery has stopped working and this test would pass on "+
			"anything (found %d)", set)
	}

	line := renderSummary(t, cfg, "web_dist", "assets/tarot", true)
	if strings.Contains(line, sentinel) {
		t.Errorf("the boot summary printed a credential:\n%s", line)
	}
}

func TestTheComplaintsNameOnlyRelationshipsThatAreActuallyBroken(t *testing.T) {
	t.Parallel()
	// A deployment with every partner set, which the cases below break one at
	// a time.
	whole := config.Config{
		RequireAuth:  true,
		ResendAPIKey: "re_not_a_real_key",
		BaseURL:      "https://example.test",
		EmailFrom:    "sylvan-library <no-reply@example.test>",
		AdminEmail:   "someone@example.test",
	}
	without := func(mutate func(*config.Config)) config.Config {
		c := whole
		mutate(&c)
		return c
	}

	for _, tc := range []struct {
		name string
		cfg  config.Config
		want []string // the variables that must be named; nil means silence
	}{
		{
			// Auth off is a laptop: none of these relationships exists to break.
			name: "a laptop complains about nothing",
			cfg:  config.Defaults(),
		},
		{
			name: "a configured deployment complains about nothing",
			cfg:  whole,
		},
		{
			// Auth on with none of its partners set: every one of them is a
			// deployment that answers pages and fails the first password reset.
			name: "a bare auth-on deployment names all four",
			cfg:  config.Config{RequireAuth: true, BaseURL: config.DefaultBaseURL, EmailFrom: config.DefaultEmailFrom},
			want: []string{"RESEND_API_KEY", "MTGLAB_BASE_URL", "MTGLAB_EMAIL_FROM", "MTGLAB_ADMIN_EMAIL"},
		},
		{
			name: "no mail key",
			cfg:  without(func(c *config.Config) { c.ResendAPIKey = "" }),
			want: []string{"RESEND_API_KEY"},
		},
		{
			// The loopback check is about the *value*, not about the variable
			// being unset: an instance told to mail links to the local port has
			// the same problem as one that was never told anything.
			name: "an explicit loopback base URL still complains",
			cfg:  without(func(c *config.Config) { c.BaseURL = config.DefaultBaseURL }),
			want: []string{"MTGLAB_BASE_URL"},
		},
		{
			name: "the built-in From address",
			cfg:  without(func(c *config.Config) { c.EmailFrom = config.DefaultEmailFrom }),
			want: []string{"MTGLAB_EMAIL_FROM"},
		},
		{
			name: "no maintainer",
			cfg:  without(func(c *config.Config) { c.AdminEmail = "" }),
			want: []string{"MTGLAB_ADMIN_EMAIL"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := configComplaints(tc.cfg)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d complaint(s) %v, got %d: %v",
					len(tc.want), tc.want, len(got), got)
			}
			for _, want := range tc.want {
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
		})
	}
}
