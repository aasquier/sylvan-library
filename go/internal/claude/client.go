package claude

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/tiers"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// This file is `claude/client.py`: constructing a client, and saying clearly
// when we cannot.
//
// **The key is never bound to a name.** The SDK reads ANTHROPIC_API_KEY out of
// the environment itself, and nothing here ever holds the value. A value we do
// not hold is one we cannot log, serialise into an error response, or leak
// into a prompt. `CredentialPresent` therefore asks whether a credential is
// *present* and never what it is.
//
// **One thing Python worries about here does not exist in Go.** `client.py`
// imports `anthropic` inside functions because the SDK rides with an optional
// extra, so `sdk_installed()` is a real question there: a base install has a
// gate, a solver and a simulator that all work with no account. Here the SDK is
// linked into the binary and cannot be absent, so `Available` is the credential
// question alone. ADR 15's "off is a real position" survives intact — it just
// has one fewer way to be true.
//
// **A second Python worry also evaporates.** `config.py` deletes a blank
// ANTHROPIC_API_KEY at import, because Anthropic's precedence treats an empty
// string as a credential that is present and then fails it as a 401. Go's SDK
// reads `ok && v != ""`, and `os.Getenv` returns "" for both unset and blank,
// so "set but empty" cannot reach `CredentialPresent` and read as true. The
// door has no `.env` reader in any case.
//
// The Sonnet 5 request-surface facts that `client.py`'s docstring pins are
// depended on by `converse.go` rather than restated here: adaptive thinking is
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

// ModelFor is the model to call: the environment's override, this tier's, or
// `Model`.
//
// Three sources, and the precedence between them is the decision:
//
//  1. **MTGLAB_CLAUDE_MODEL wins over everything.** It is the A/B lever ADR 14
//     left for the open question, and an A/B whose answer depended on which
//     seat happened to ask would not be one.
//  2. **The account's tier**, when it has one.
//  3. **`Model`**, the house answer.
//
// An unknown tier lands on the default rather than failing; `tiers.Get` says
// why. The empty string is how "no tier" arrives -- from a NULL column, or
// from a caller with no account in hand -- and `tiers.Get` reads it as the
// default too, so the second and third branches agree by construction. That
// agreement is asserted by a test rather than trusted, because if `Model` and
// the default tier's model ever drifted apart, a request with no tier and a
// request from a default seat would quietly run on different models.
//
// Read at call time throughout, so none of the three is captured at import by
// a process that outlives the request.
func ModelFor(tier string) string {
	if override := os.Getenv(modelEnv); override != "" {
		return override
	}
	if tier != "" {
		return tiers.Resolve(tier)
	}
	return Model
}

// CredentialPresent reports whether the SDK will find something to
// authenticate with. Deliberately only asks *whether*.
func CredentialPresent() bool {
	return os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != ""
}

// Available reports whether this process can talk to Claude at all. Cheap; no
// network.
func Available() bool { return CredentialPresent() }

// Require returns an `ErrUnavailable` with a fixable reason, or nothing.
//
// Split out of `Connect` for the caller that needs to know *whether* a call is
// possible without making one -- a theme proposal is refused with no key from
// the request rather than four minutes into a job that was never going to
// work.
func Require() error {
	if !CredentialPresent() {
		return &unavailable{reason: "no ANTHROPIC_API_KEY in the environment " +
			"-- put one in .env (see .env.example), or `fly secrets set` it " +
			"when deployed"}
	}
	return nil
}

// unavailable is ErrUnavailable carrying the fixable reason and NOTHING else.
//
// A `fmt.Errorf("%w: ...", ErrUnavailable)` would read "claude is unavailable:
// no ANTHROPIC_API_KEY ...", and that string is not internal: `str(exc)` on
// Python's `ClaudeUnavailable` is the bare reason, the route puts it straight
// into a 422/503 `detail`, and the deck page renders `detail` verbatim. So the
// sentinel's own words would have shipped as a prefix nobody wrote, on the
// first Claude surface to answer from the door.
//
// Found by curling the pair rather than by any test: the contract goldens
// record this body as `{"detail": "string"}`, which is true of both spellings.
// Same shape as `stanceRejection` -- match on the sentinel, answer in Python's
// words -- and the same lesson as `converse` handing the model `no deck 'x'`
// where Python's `DeckNotFound` stringifies to the bare slug.
type unavailable struct{ reason string }

func (e *unavailable) Error() string { return e.reason }

// Is makes `errors.Is(err, ErrUnavailable)` true without the sentinel's text
// reaching anybody.
func (e *unavailable) Is(target error) bool { return target == ErrUnavailable }

// Connect is an SDK client, or `ErrUnavailable` with a fixable reason.
//
// Constructed with no options on purpose: that is the path where the SDK
// resolves the credential from the environment itself, which is the whole
// point of never holding the key here.
func Connect() (anthropic.Client, error) {
	if err := Require(); err != nil {
		return anthropic.Client{}, err
	}
	return anthropic.NewClient(), nil
}

// Explain turns an SDK error into something worth reading a month later.
//
// The 401 case earns its own branch. The key this project runs on was created
// with a fixed lifetime and cannot be extended, so the first thing a rejected
// request means is probably "it expired", not "the integration broke" -- and
// the person reading the message will have forgotten the key had a lifetime.
//
// Python branches on the SDK's exception classes; the Go SDK reports one error
// type carrying a status code, so this switches on the code. The messages are
// the same sentences, and a test holds them to Python's word for word: they
// are what a person reads at 2am, and a port that quietly paraphrased them
// would be losing the only part of this function that matters.
func Explain(err error) string {
	if err == nil {
		return ""
	}
	var apierr *anthropic.Error
	if !errors.As(err, &apierr) {
		if errors.Is(err, ErrUnavailable) {
			return err.Error()
		}
		// The SDK reports a transport failure as a plain error, where Python
		// has an APIConnectionError class to match on.
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
			wire.PyRepr(ModelFor("")))
	case 429:
		return "rate limited (429). Retry after the delay in the response headers."
	case 404:
		return fmt.Sprintf("no such model or endpoint (404). Asked for %s.",
			wire.PyRepr(ModelFor("")))
	default:
		return fmt.Sprintf("API error %d: %s", apierr.StatusCode, apierr.Error())
	}
}

// isConnection recognises the transport failures Python catches as
// APIConnectionError. Matched on the wrapped error's shape rather than its
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

// CheckReport is what `Check` found, in the field order Python's dict is built
// in -- so the CLI and a health route render the same facts in the same order.
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
func Check(ctx context.Context, prompt string) CheckReport {
	if prompt == "" {
		prompt = CheckPrompt
	}
	report := CheckReport{Model: ModelFor("")}
	con, err := Connect()
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
		report.Error = Explain(err)
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
