package claude

import (
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

// How this process reaches the API, as a value.
//
// **The key is still never bound to a name.** That rule (see `client.go`) is
// the reason this type exists in the shape it does rather than as a struct with
// an APIKey field. [EndpointFromEnv] — the only one a serving process uses —
// stores a *boolean* about whether a credential exists and no options at all,
// so [Endpoint.Connect] calls `anthropic.NewClient()` with nothing, which is
// exactly the path where the SDK resolves the credential out of the
// environment itself. A value we do not hold is one we cannot log.
//
// What the value *does* buy is the thing the environment could not: a second
// endpoint. `ANTHROPIC_BASE_URL` is one global slot, so a test that stood up a
// stub server had to publish its URL to the whole process, and two stubs could
// never coexist (ADR 39). [EndpointAt] gives each caller its own.

// Endpoint is where this process's Claude calls go, and whether it can make
// any.
//
// The zero value is an instance nobody has given a key to: `Present` is false
// and every `Require` refuses. That is a deliberate default — a test that says
// nothing about Claude gets the state CI runs in, rather than whatever the
// developer's shell happens to export.
type Endpoint struct {
	// present is whether a credential exists. The question, never the value.
	present bool
	// opts are extra SDK options, and are empty for a real process: that is
	// the path where the SDK reads the credential itself. A test sets a base
	// URL and a throwaway key so its own stub is the endpoint.
	opts []option.RequestOption
	// model overrides the tier's and the house answer. MTGLAB_CLAUDE_MODEL.
	model string
}

// EndpointFromEnv is what a serving process uses: the credential question
// answered from the environment, and nothing held.
//
// Read once, at the composition root, rather than per call — ADR 39.
func EndpointFromEnv() Endpoint {
	return Endpoint{
		// "Set but empty" cannot read as present. Anthropic's own precedence
		// treats an empty ANTHROPIC_API_KEY as a credential that exists and
		// then fails it as a 401, but the SDK reads `ok && v != ""` and
		// `os.Getenv` returns "" for both unset and blank -- so a blank export
		// never reaches Present as true, and this agrees with the SDK it is
		// describing.
		present: os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != "",
		model:   os.Getenv(modelEnv),
	}
}

// EndpointAt points at an explicit URL with an explicit key.
//
// The test seam, and the A/B lever: every caller that builds one gets its own
// client, so two stub servers can run at once. Never used by a serving
// process, which is why the only key this type can hold is one the caller
// invented.
func EndpointAt(baseURL, apiKey string) Endpoint {
	return Endpoint{
		present: apiKey != "",
		opts:    []option.RequestOption{option.WithBaseURL(baseURL), option.WithAPIKey(apiKey)},
	}
}

// WithModel is this endpoint with the model override replaced -- the
// MTGLAB_CLAUDE_MODEL lever, as a value.
func (e Endpoint) WithModel(model string) Endpoint {
	e.model = model
	return e
}

// Present reports whether the SDK will find something to authenticate with.
// Deliberately only asks *whether*.
func (e Endpoint) Present() bool { return e.present }

// Require returns an `ErrUnavailable` with a fixable reason, or nothing.
//
// Split from Connect for the caller that needs to know *whether* a call is
// possible without making one -- a theme proposal is refused with no key from
// the request rather than four minutes into a job that was never going to
// work.
func (e Endpoint) Require() error {
	if !e.present {
		return &unavailable{reason: "no ANTHROPIC_API_KEY in the environment " +
			"-- put one in .env (see .env.example), or `fly secrets set` it " +
			"when deployed"}
	}
	return nil
}

// Connect is an SDK client, or `ErrUnavailable` with a fixable reason.
//
// A real process reaches here with no options, which is the path where the SDK
// resolves the credential from the environment on its own -- the whole point
// of never holding the key. A test reaches here with a base URL and a
// throwaway key, and gets a client pointed at its own stub.
func (e Endpoint) Connect() (anthropic.Client, error) {
	if err := e.Require(); err != nil {
		return anthropic.Client{}, err
	}
	return anthropic.NewClient(e.opts...), nil
}

// ModelFor is the model to call: this endpoint's override, the tier's, or
// [Model].
//
// Three sources, and the precedence between them is the decision:
//
//  1. **The override wins over everything.** It is MTGLAB_CLAUDE_MODEL, the
//     A/B lever ADR 14 left for the open question, and an A/B whose answer
//     depended on which seat happened to ask would not be one.
//  2. **The account's tier**, when it has one.
//  3. **[Model]**, the house answer.
//
// An unknown tier lands on the default rather than failing; `tiers.Get` says
// why. The empty string is how "no tier" arrives -- from a NULL column, or
// from a caller with no account in hand -- and `tiers.Get` reads it as the
// default too, so the second and third branches agree by construction. That
// agreement is asserted by a test rather than trusted, because if [Model] and
// the default tier's model ever drifted apart, a request with no tier and a
// request from a default seat would quietly run on different models.
func (e Endpoint) ModelFor(tier string) string {
	if e.model != "" {
		return e.model
	}
	if tier != "" {
		return tiers.Resolve(tier)
	}
	return Model
}

// Settings is everything a Claude surface is configured with: where the calls
// go, and how far a stance may be turned up.
//
// One value rather than two parameters because they travel together everywhere
// -- `api.Config` carries one field, and the door passes one field through.
type Settings struct {
	// Endpoint is where the calls go, and whether any can be made.
	Endpoint Endpoint
	// Ceiling is the most permissive stance this deployment allows anyone to
	// select, or nil for the built-in default.
	//
	// A pointer, and not a Stance, because the zero Stance is `Off` -- so a
	// value type would make "nobody configured a ceiling" mean "refuse
	// everything", which is a silent and total feature outage rather than a
	// default. Nil says the same thing the old unset variable did, in the same
	// convention the `limit *Stance` parameters already use.
	Ceiling *Stance
}

// SettingsFromEnv is what a serving process uses.
func SettingsFromEnv() Settings {
	ceiling := Ceiling()
	return Settings{Endpoint: EndpointFromEnv(), Ceiling: &ceiling}
}

// ceiling is this deployment's cap, or the built-in default.
//
// The nil case is the one every test takes and the one no serving process
// does: `cmd/mtglab` builds its Settings with [SettingsFromEnv], which always
// fills it.
func (s Settings) ceiling() Stance {
	if s.Ceiling != nil {
		return *s.Ceiling
	}
	return Collaborator
}
