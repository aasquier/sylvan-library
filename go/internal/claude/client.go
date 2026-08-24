package claude

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// This file constructs a client, and says clearly when we cannot.
//
// **The key is never bound to a name.** The SDK reads ANTHROPIC_API_KEY out of
// the environment itself, and nothing here ever holds the value. A value we do
// not hold is one we cannot log, serialise into an error response, or leak
// into a prompt. `CredentialPresent` therefore asks whether a credential is
// *present* and never what it is.
//
// **`Available` is the credential question alone.** The SDK is linked into
// the binary and cannot be absent, so there is no "is the SDK installed"
// question to ask; ADR 15's "off is a real position" survives intact — it
// just has one fewer way to be true.
//
// **And "set but empty" cannot read as present.** Anthropic's own precedence
// treats an empty ANTHROPIC_API_KEY as a credential that exists and then
// fails it as a 401, but the Go SDK reads `ok && v != ""`, and `os.Getenv`
// returns "" for both unset and blank, so a blank export never reaches
// `CredentialPresent` as true. The door has no `.env` reader in any case.
//
// The Sonnet 5 request-surface facts pinned here are depended on by
// `converse.go` rather than restated there: adaptive thinking is
// on by default, `budget_tokens` is a 400, sampling parameters are rejected,
// prefills are rejected, and depth is `output_config.effort`.

// Model is the house answer for every account nobody has said otherwise about.
//
// Aaron's call, argued in ADR 14: start on Sonnet and find out whether it is
// enough, rather than paying for Opus on the assumption that it is not.
//
// **Do not quietly raise this.** Moving up is a decision to make once there is
// evidence, and the override below exists so that evidence can be gathered --
// an A/B against a named model for one run -- not so a deployment can drift
// onto a costlier default nobody chose. The per-seat grant in `tiers` is the
// deliberate, one-account-at-a-time exception to that rule, not a loophole in
// it.
const Model = "claude-sonnet-5"

const modelEnv = "MTGLAB_CLAUDE_MODEL"

// DefaultMaxTokens is small on purpose: the floor for a health check and a
// first turn. Every mode carries its own ceiling.
const DefaultMaxTokens = 4096

// ErrUnavailable is "no client can be built" -- no credential is set.
//
// Distinct from an API error: this one is answerable locally, and the caller
// can say so instead of retrying.
var ErrUnavailable = errors.New("claude is unavailable")

// unavailable is ErrUnavailable carrying the fixable reason and NOTHING else.
//
// A `fmt.Errorf("%w: ...", ErrUnavailable)` would read "claude is unavailable:
// no ANTHROPIC_API_KEY ...", and that string is not internal: the recorded
// shape is the bare reason, the route puts it straight
// into a 422/503 `detail`, and the deck page renders `detail` verbatim. So the
// sentinel's own words would have shipped as a prefix nobody wrote, on the
// first Claude surface to answer from the door.
//
// Found by driving the real route rather than by any test: a schema-level
// golden describes this body only as `{"detail": "string"}`, which is true
// of both spellings.
// Same shape as `stanceRejection` -- match on the sentinel, answer in the
// recorded words -- and the same lesson as `converse` once handing the model
// `no deck 'x'` where the recorded `DeckNotFound` text is the bare slug.
type unavailable struct{ reason string }

func (e *unavailable) Error() string { return e.reason }

// Is makes `errors.Is(err, ErrUnavailable)` true without the sentinel's text
// reaching anybody.
func (e *unavailable) Is(target error) bool { return target == ErrUnavailable }

// Explain turns an SDK error into something worth reading a month later.
//
// The 401 case earns its own branch. The key this project runs on was created
// with a fixed lifetime and cannot be extended, so the first thing a rejected
// request means is probably "it expired", not "the integration broke" -- and
// the person reading the message will have forgotten the key had a lifetime.
//
// The SDK reports one error
// type carrying a status code, so this switches on the code. The messages
// are recorded sentences, and a test holds them to the golden word for word:
// they are what a person reads at 2am, and a quiet paraphrase
// would be losing the only part of this function that matters.
func Explain(err error, model string) string {
	if err == nil {
		return ""
	}
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		if errors.Is(err, ErrUnavailable) {
			return err.Error()
		}
		// The SDK reports a transport failure as a plain error with no
		// dedicated type to match on.
		if isConnection(err) {
			return "could not reach api.anthropic.com -- check network access."
		}
		return err.Error()
	}
	switch apierr.StatusCode {
	case 401:
		return "the key was rejected (401). It may have expired -- API keys " +
			"carry a fixed lifetime chosen when they are created, and an " +
			"expired one cannot be reactivated. Check the key at " +
			"platform.claude.com and create a new one if it has lapsed."
	case 403:
		return fmt.Sprintf("the key was refused this request (403) -- most "+
			"often a model the workspace cannot reach. Asked for %s.",
			wire.Quote(model))
	case 429:
		return "rate limited (429). Retry after the delay in the response headers."
	case 404:
		return fmt.Sprintf("no such model or endpoint (404). Asked for %s.",
			wire.Quote(model))
	default:
		return fmt.Sprintf("API error %d: %s", apierr.StatusCode, apierr.Error())
	}
}

// isConnection recognises the could-not-reach family of transport
// failures. Matched on the wrapped error's shape rather than its
// text where possible; the string check is the residue for what the standard
// library does not give a type to.
func isConnection(err error) bool {
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "dial tcp")
}

// CheckReport is what `Check` found, in the recorded field order -- so the
// CLI and a health route render the same facts in the same order.
type CheckReport struct {
	Model        string `json:"model"`
	OK           bool   `json:"ok"`
	Error        string `json:"error,omitempty"`
	ServedBy     string `json:"served_by,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	Text         string `json:"text,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
}

// CheckPrompt is the smallest real question there is.
const CheckPrompt = "Reply with exactly: pipe open"

// Check makes the smallest real call there is. Proves the key, the model and
// the wire.
//
// Costs a few dozen tokens, which is the point -- this is what gets run after
// a key rotation, or in six weeks when something 401s and the question is
// whether the integration broke or the key simply lapsed.
//
// Returns a report rather than printing one.
func Check(ctx context.Context, e Endpoint, prompt string) CheckReport {
	if prompt == "" {
		prompt = CheckPrompt
	}
	report := CheckReport{Model: e.ModelFor("")}
	con, err := e.Connect()
	if err != nil {
		report.Error = err.Error()
		return report
	}
	resp, err := con.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     report.Model,
		MaxTokens: DefaultMaxTokens,
		// Effort, not a thinking budget -- `budget_tokens` is a 400 here.
		// `low` because a health check has nothing to reason about, and the
		// default is `high`.
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		// Every failure mode of this call is something to report to a person,
		// and `Explain` is what decides how.
		report.Error = Explain(err, report.Model)
		return report
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	report.OK = resp.StopReason != anthropic.StopReasonRefusal
	report.ServedBy = resp.Model
	report.StopReason = string(resp.StopReason)
	report.Text = strings.TrimSpace(text.String())
	report.InputTokens = int(resp.Usage.InputTokens)
	report.OutputTokens = int(resp.Usage.OutputTokens)
	return report
}
