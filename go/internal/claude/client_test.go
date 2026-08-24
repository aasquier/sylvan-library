package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

func TestTheModelPrecedenceIsOverrideThenTierThenHouse(t *testing.T) {
	t.Parallel()
	plain := Endpoint{}
	if got := plain.ModelFor(""); got != Model {
		t.Errorf("no override and no tier is %q, want the house model %q", got, Model)
	}
	if got := plain.ModelFor("opus"); got != tiers.Resolve("opus") {
		t.Errorf("a tiered seat is %q, want %q", got, tiers.Resolve("opus"))
	}
	// An unknown tier lands on the default rather than failing.
	if got := plain.ModelFor("archmage"); got != Model {
		t.Errorf("an unknown tier is %q, want the default %q", got, Model)
	}

	// The A/B lever, which used to be MTGLAB_CLAUDE_MODEL installed on the
	// process and is now a field: it beats every tier.
	ab := plain.WithModel("claude-under-test")
	for _, tier := range []string{"", "opus", "fable", "archmage"} {
		if got := ab.ModelFor(tier); got != "claude-under-test" {
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
	t.Parallel()
	if got := tiers.Resolve(tiers.DefaultKey); got != Model {
		t.Fatalf("the default tier resolves to %q but the house model is %q -- "+
			"a request with no tier and a request from a default seat would "+
			"run on different models", got, Model)
	}
	if (Endpoint{}).ModelFor("") != (Endpoint{}).ModelFor(tiers.DefaultKey) {
		t.Error("no tier and the default tier answer differently")
	}
}

func TestACredentialIsAskedAboutAndNeverRead(t *testing.T) {
	t.Parallel()
	// The zero Endpoint is an instance nobody gave a key to -- which is CI,
	// and which no longer depends on what the developer's shell exports.
	if (Endpoint{}).Present() {
		t.Error("no credential should read as unavailable")
	}
	if err := (Endpoint{}).Require(); err == nil {
		t.Error("Require accepted an absent credential")
	} else if !strings.Contains(err.Error(), ".env") {
		t.Errorf("the refusal should say how to fix it: %v", err)
	}
	if _, err := (Endpoint{}).Connect(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Connect without a credential gave %v, want ErrUnavailable", err)
	}
	// A blank key is not a credential: EndpointAt reads an empty key the same
	// way the SDK reads an empty variable, so "set but empty" cannot fool it.
	if EndpointAt("http://example.test", "").Present() {
		t.Error("a blank key read as a credential")
	}
	if !EndpointAt("http://example.test", "tok").Present() {
		t.Error("a key should read as a credential")
	}
}

// TestTheEndpointNeverHoldsTheRealCredential is the rule `endpoint.go` is
// shaped around, asserted rather than trusted: the constructor a serving
// process uses stores whether a key exists and never the key.
func TestTheEndpointNeverHoldsTheRealCredential(t *testing.T) {
	t.Parallel()
	e := EndpointFromEnv()
	if len(e.opts) != 0 {
		t.Errorf("EndpointFromEnv carried %d SDK option(s); it must carry none, "+
			"because no options is the path where the SDK resolves the "+
			"credential itself and this package never holds it", len(e.opts))
	}
}

// The 401 branch exists because the key this project runs on carries a fixed
// lifetime, and the person reading the message will have forgotten that.
func TestARejectedKeyIsExplainedAsAnExpiryFirst(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{"!401"}}
	ep := api.start(t)
	report := Check(context.Background(), ep, "")
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
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "end_turn", model: "claude-sonnet-5", in: 14, out: 3,
			content: textBlock("  pipe open  ")}.json(),
	}}
	ep := api.start(t)
	report := Check(context.Background(), ep, "")
	if !report.OK || report.Text != "pipe open" || report.ServedBy != "claude-sonnet-5" {
		t.Errorf("check report is %+v", report)
	}
	if report.InputTokens != 14 || report.OutputTokens != 3 {
		t.Errorf("check lost its token counts: %+v", report)
	}
	// A refusal is not ok, even though the call itself succeeded.
	api2 := &scriptedAPI{replies: []string{reply{stop: "refusal", content: ""}.json()}}
	ep2 := api2.start(t)
	if Check(context.Background(), ep2, "").OK {
		t.Error("a refusal reported ok")
	}
}

// The report's field order is the recorded one, and it is rendered by both
// the CLI and a health route -- so it is checked through encoding/json rather
// than field by field.
func TestTheCheckReportKeepsTheRecordedFieldOrder(t *testing.T) {
	t.Parallel()
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
