package claude

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

func TestTheModelPrecedenceIsEnvThenTierThenHouse(t *testing.T) {
	t.Setenv("MTGLAB_CLAUDE_MODEL", "")
	if got := ModelFor(""); got != Model {
		t.Errorf("no override and no tier is %q, want the house model %q", got, Model)
	}
	if got := ModelFor("opus"); got != tiers.Resolve("opus") {
		t.Errorf("a tiered seat is %q, want %q", got, tiers.Resolve("opus"))
	}
	// An unknown tier lands on the default rather than failing.
	if got := ModelFor("archmage"); got != Model {
		t.Errorf("an unknown tier is %q, want the default %q", got, Model)
	}

	t.Setenv("MTGLAB_CLAUDE_MODEL", "claude-under-test")
	for _, tier := range []string{"", "opus", "fable", "archmage"} {
		if got := ModelFor(tier); got != "claude-under-test" {
			t.Errorf("the A/B override lost to tier %q: got %q", tier, got)
		}
	}
}

// The house model and the default tier's model must be the same string.
//
// `ModelFor("")` takes the third branch and `ModelFor("sonnet")` takes the
// second, so if these two ever drifted apart, a request with no account
// attached and a request from a default seat would quietly run on different
// models -- and every measurement taken through one door would be about the
// other.
func TestTheHouseModelAndTheDefaultTierAgree(t *testing.T) {
	t.Setenv("MTGLAB_CLAUDE_MODEL", "")
	if got := tiers.Resolve(tiers.DefaultKey); got != Model {
		t.Fatalf("the default tier resolves to %q but the house model is %q -- "+
			"a request with no tier and a request from a default seat would "+
			"run on different models", got, Model)
	}
	if ModelFor("") != ModelFor(tiers.DefaultKey) {
		t.Error("no tier and the default tier answer differently")
	}
}

func TestACredentialIsAskedAboutAndNeverRead(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	if CredentialPresent() || Available() {
		t.Error("no credential should read as unavailable")
	}
	if err := Require(); err == nil {
		t.Error("Require accepted an absent credential")
	} else if !strings.Contains(err.Error(), ".env") {
		t.Errorf("the refusal should say how to fix it: %v", err)
	}
	// A blank key is not a credential: `os.Getenv` reads unset and blank the
	// same way, so "exported but empty" cannot fool the check -- and this is
	// what says so.
	t.Setenv("ANTHROPIC_API_KEY", "")
	if CredentialPresent() {
		t.Error("a blank ANTHROPIC_API_KEY read as a credential")
	}
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "tok")
	if !CredentialPresent() {
		t.Error("an auth token is a credential too")
	}
}

// The 401 branch exists because the key this project runs on carries a fixed
// lifetime, and the person reading the message will have forgotten that.
func TestARejectedKeyIsExplainedAsAnExpiryFirst(t *testing.T) {
	api := &scriptedAPI{replies: []string{"!401"}}
	api.start(t)
	report := Check(context.Background(), "")
	if report.OK {
		t.Fatal("a 401 reported ok")
	}
	for _, want := range []string{"401", "may have expired", "platform.claude.com"} {
		if !strings.Contains(report.Error, want) {
			t.Errorf("the 401 message is missing %q: %s", want, report.Error)
		}
	}
}

func TestCheckReportsWhatTheSmallestCallProved(t *testing.T) {
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", model: "claude-sonnet-5", in: 14, out: 3,
			content: textBlock("  pipe open  ")}.json(),
	}}
	api.start(t)
	report := Check(context.Background(), "")
	if !report.OK || report.Text != "pipe open" || report.ServedBy != "claude-sonnet-5" {
		t.Errorf("check report is %+v", report)
	}
	if report.InputTokens != 14 || report.OutputTokens != 3 {
		t.Errorf("check lost its token counts: %+v", report)
	}
	// A refusal is not ok, even though the call itself succeeded.
	api2 := &scriptedAPI{replies: []string{reply{stop: "refusal", content: ""}.json()}}
	api2.start(t)
	if Check(context.Background(), "").OK {
		t.Error("a refusal reported ok")
	}
}

// The report's field order is the recorded one, and it is rendered by both
// the CLI and a health route -- so it is checked through encoding/json rather
// than field by field.
func TestTheCheckReportKeepsTheRecordedFieldOrder(t *testing.T) {
	raw, err := json.Marshal(CheckReport{
		Model: "m", OK: true, ServedBy: "s", StopReason: "end_turn",
		Text: "t", InputTokens: 1, OutputTokens: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"m","ok":true,"served_by":"s","stop_reason":"end_turn",` +
		`"text":"t","input_tokens":1,"output_tokens":2}`
	if string(raw) != want {
		t.Errorf("check report renders as\n %s\nwant\n %s", raw, want)
	}
}
