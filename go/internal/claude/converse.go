package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// This file is the loop that runs a mode.
//
// **The loop lives beside the Mode rather than inside a mode's own file**
// because ADR 15 names several modes and this is the part that would otherwise
// be copied once per mode. Everything mode-specific -- which facts to
// assemble, how to read the answer back -- belongs to the mode's own file;
// what belongs here is the request shape and the tool round trip.
//
// Two things about the request shape, both Sonnet 5 specifics that `client.go`
// pins and this file depends on: `output_config` carries **both** the effort
// and the response format, and adaptive thinking runs by default -- so
// MaxTokens is a ceiling over thinking and answer together and needs headroom
// a non-thinking model would not have wanted.
//
// **Server tools are a second kind of tool and the loop treats them
// differently.** ToolNames are this package's own, dispatched through
// `tools.Run`; ServerTools run on Anthropic's side and never reach that door.
// They bring two things an own-tools-only loop does not handle. Their results
// are *evidence* -- with a response schema in play the API attaches no
// citations, so the pages a search actually returned are collected here and
// are the only thing a caller can check an answer's sources against. And a
// server-side tool loop that hits its own limit stops with `pause_turn`,
// carrying text that looks like a finished answer; that is resumed rather than
// returned, for the same reason `sim/tier3` refuses to report a game Forge
// played with 96 cards.

// ErrModeExhausted is the tool loop hitting its turn limit without the model
// finishing.
//
// Returned rather than handing back whatever the last turn happened to say. A
// truncated answer that looks complete is the failure mode worth avoiding
// here -- the same shape as a Forge game that plays on with 96 cards.
//
// A sentinel to match on, never a string to read: the error `Converse`
// actually returns is an `exhausted`, whose text is the recorded exhaustion
// sentence and nothing else -- see `unavailable` in client.go for why the
// sentinel's own words must not ship as a prefix.
var ErrModeExhausted = errors.New("mode exhausted")

// exhausted is ErrModeExhausted carrying the served sentence verbatim.
//
// A job's `error` field and a route's 502 `detail` both render the error's
// own text, so the sentence is the wire. Until 2026-08-23 this was
// `fmt.Errorf("%w: %s ...", ErrModeExhausted, ...)`, which read "mode
// exhausted: commander-dossier still wanted tools..." -- the sentinel's two
// words in front and the recorded full stop missing from the end.
type exhausted struct{ msg string }

func (e *exhausted) Error() string { return e.msg }

// Is makes `errors.Is(err, ErrModeExhausted)` true without the sentinel's
// text reaching anybody.
func (e *exhausted) Is(target error) bool { return target == ErrModeExhausted }

// apiFailure is the SDK refusing or failing a request, as the caller reads
// it: `Error()` is `Explain(err)` -- the sentence somebody reads at 2am --
// and `Unwrap()` is the SDK's own error, so `errors.As` still finds the
// status code underneath.
//
// Every caller renders an API failure as its explanation: a route's 502
// `detail`, a job's `error`. So the text that reaches a person is the
// explanation alone. Until
// 2026-08-23 this was `fmt.Errorf("%s: %s", mode.Name, Explain(err))`, which
// put the mode's name in front of every one of those sentences -- a prefix
// the recorded shape never carries, on the first Claude surfaces the door
// answered.
type apiFailure struct {
	err error
	// model is what was asked for, which two of Explain's branches name.
	model string
}

func (e *apiFailure) Error() string { return Explain(e.err, e.model) }
func (e *apiFailure) Unwrap() error { return e.err }

// MaxToolTurns is enough for a lookup, a search and a reconsider. A mode that
// has not finished by then is looping rather than working, and the ceiling
// turns that into an error instead of a bill.
const MaxToolTurns = 6

// ToolCall is one tool this conversation asked for, in the recorded key
// order.
type ToolCall struct {
	Tool      string         `json:"tool"`
	Arguments map[string]any `json:"arguments"`
}

// Page is one page a hosted search returned, in the recorded key order.
type Page struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// Turn is what one exchange produced, including how it got there.
//
// ToolCalls is kept because ADR 14's third boundary is that a user can tell
// which system answered. For an opinion assembled from six pool lookups,
// "which system" is only half the answer -- the other half is what it read,
// and a caller that wants to show its working needs the list.
type Turn struct {
	Mode         string     `json:"mode"`
	Model        string     `json:"model"`
	StopReason   string     `json:"stop_reason"`
	Text         string     `json:"text"`
	ToolCalls    []ToolCall `json:"tool_calls"`
	InputTokens  int        `json:"input_tokens"`
	OutputTokens int        `json:"output_tokens"`
	// Searched is every page a server-side search actually returned, in the
	// order it came back. This is evidence, not decoration: with a response
	// schema in play the API attaches no citations to the answer, so the only
	// way to tell a page the model read from a URL it composed is to check the
	// answer's citations against this list. ADR 19 makes that check mandatory.
	Searched []Page `json:"searched"`
	// SearchErrors are search failures, kept rather than dropped. A search that
	// returned nothing and a search that failed look identical from an empty
	// Searched, and they want different answers.
	SearchErrors []string `json:"search_errors"`
	Refused      bool     `json:"refused"`
	// CacheReadTokens is prompt tokens served from the cache, at ~a tenth of
	// the input price. **Counted beside InputTokens, not inside it** -- the API
	// reports input tokens as the uncached remainder only, so this
	// conversation's whole prompt is InputTokens + CacheReadTokens (plus cache
	// writes, which nothing here records). Reading it as a slice understates
	// the prompt by however well the cache worked, which is backwards.
	//
	// Zero on a cold call; if it stays zero across repeated calls of the same
	// mode, the prefix is drifting and the breakpoints below are buying
	// nothing -- that is the number to check before believing the cache works.
	CacheReadTokens int `json:"cache_read_tokens"`
}

// Parsed is the answer as JSON, for a mode that constrained its format.
func (t Turn) Parsed(into any) error {
	// **UseNumber, for the reason the stance parser already carries it.**
	// The plain rendering downstream tells `3` from `3.0` by the literal --
	// they render "3" and "3.0" -- and a plain Go decode makes both
	// `float64(3)` and throws the literal away. The literal has to survive
	// the decode.
	decoder := json.NewDecoder(strings.NewReader(t.Text))
	decoder.UseNumber()
	return decoder.Decode(into)
}

// Request is everything a conversation needs that is not the mode.
type Request struct {
	Messages []anthropic.MessageParam
	Stance   Stance
	// Endpoint is where this conversation's calls go. The zero value is an
	// instance with no credential, which refuses -- so a caller that forgets
	// to set it gets a clean refusal rather than a live call it did not mean
	// to make.
	Endpoint Endpoint
	// Deps is what the mode's own tools may reach -- a deck source and a pool,
	// either of which may be absent.
	Deps tools.Deps
	// Tier is the asking account's model grant, resolved **once** per
	// conversation rather than per turn: a conversation that changed model
	// halfway would throw away its prompt cache -- caches are model-scoped --
	// and would put two models' answers in one transcript. Empty is every
	// account by default and means the house model. A background job captures
	// the tier at plan time, the way it captures a deck source, because a job
	// outlives the request that knew who was asking.
	Tier string
	// MaxTurns is the tool-loop ceiling; zero means MaxToolTurns.
	MaxTurns int
	// OnTurn fires as each model turn begins, and exists for the modes slow
	// enough to be background jobs: a theme proposal takes minutes, and a job
	// that reports nothing for that long is indistinguishable from a wedged
	// one. It is a **ceiling and not an estimate** -- a loop that finishes on
	// turn four of eight jumps straight to done. Anything more truthful would
	// need to know in advance how many searches the model was going to run.
	OnTurn func(done, max int)
	// Ledger is where the conversation's accounting lands. Nil is tolerated
	// and warns, exactly as a missing app.db does in `ledger.Record`.
	Ledger *ledger.Recorder
}

// Converse runs mode over req.Messages until it stops asking for tools.
//
// The caller is responsible for having checked `stance.AllowsCalls()` first.
// That check is not repeated here on purpose: a function that silently did
// nothing when the stance was `off` would be indistinguishable from one that
// ran and found nothing to say, and "off means no calls" deserves a caller
// that had to decide rather than a default that happened.
func Converse(ctx context.Context, mode Mode, req Request) (Turn, error) {
	// Checked here because the alternative is silent: the scope note is a map
	// lookup, and a Go map lookup answers "" for an unknown scope -- the mode
	// would go out with its scope paragraph simply missing, a real change to
	// what the model was told, visible nowhere. A zero-value Stance is the
	// way that happens, and it is one struct literal away.
	if err := req.Stance.Validate(); err != nil {
		return Turn{}, fmt.Errorf("%s: %w", mode.Name, err)
	}
	con, err := req.Endpoint.Connect()
	if err != nil {
		return Turn{}, err
	}
	answering := req.Endpoint.ModelFor(req.Tier)
	schemas, err := mode.Schemas()
	if err != nil {
		return Turn{}, err
	}
	maxTurns := req.MaxTurns
	if maxTurns == 0 {
		maxTurns = MaxToolTurns
	}

	outputConfig := anthropic.OutputConfigParam{Effort: mode.Effort}
	if mode.ResponseSchema != nil {
		outputConfig.Format = anthropic.JSONOutputFormatParam{Schema: mode.ResponseSchema}
	}

	history := append([]anthropic.MessageParam(nil), req.Messages...)
	calls := []ToolCall{}
	pages := []Page{}
	searchErrors := []string{}
	seenURLs := map[string]bool{}
	container := ""
	var tokensIn, tokensOut, tokensCached int64

	// The one *moving* cache marker, on the newest tool-result block. The
	// system breakpoint below caches the byte-stable prefix; this one caches
	// the growing conversation, which for a searching mode is where the bulk
	// is -- turn six of a dossier otherwise re-buys turns one through five,
	// search results included, at full input price. It moves rather than
	// accumulates because the API allows four markers per request and the
	// theme flow already spends one inside its messages.
	//
	// It is a pointer into the history rather than a copy: `history` holds the
	// same `*ToolResultBlockParam`, so zeroing CacheControl here is what
	// removes the marker from the request. A zeroed CacheControl marshals as
	// absent -- proven by a test, because if it did not, every turn would add a
	// marker and the fifth would be a 400.
	var marked *anthropic.ToolResultBlockParam

	// API requests actually made, for the ledger: a conversation's cost in
	// round trips as well as tokens.
	requests := 0

	record := func(stopReason, model string) {
		req.Ledger.Record(ctx, ledger.Row{
			Mode: mode.Name, Model: model, StopReason: stopReason,
			Requests: requests, InputTokens: int(tokensIn),
			OutputTokens: int(tokensOut), CacheReadTokens: int(tokensCached),
		})
	}

	for done := 0; done < maxTurns; done++ {
		if req.OnTurn != nil {
			req.OnTurn(done, maxTurns)
		}
		params := anthropic.MessageNewParams{
			Model:     answering,
			MaxTokens: mode.MaxTokens,
			// A cache breakpoint on the system block caches the tools and the
			// system prompt together (tools render first, so the marker covers
			// both). That boundary is byte-stable for a given mode and stance
			// while everything after it -- the brief, the question, the tool
			// round trips -- varies, which is exactly where the caching
			// guidance says to cut. It pays twice: across the turns of one tool
			// loop, and across consecutive calls of the same mode, which is the
			// shape of interviewing a draft's 99 owed rationales one at a time.
			//
			// Below the model's minimum cacheable prefix the marker is inert,
			// and one mode is. Measured with `count_tokens`: the six
			// conversational modes run 2,062 (research) to 6,587 (theme
			// proposal) and interview 2,373, all clear of Sonnet 5's 1,024.
			// **`scan` is 478** and clears nothing -- that is the prompt being
			// short rather than a bug, and padding it to reach the floor would
			// buy a tenth of 478 tokens with 546 wasted ones.
			System: []anthropic.TextBlockParam{{
				Text:         mode.System(req.Stance),
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			}},
			Tools:        schemas,
			OutputConfig: outputConfig,
			Messages:     history,
		}
		// `container` only ever has a value once a *server* tool has run, and
		// it is required rather than optional after that: the dated web search
		// does its own result filtering inside a code-execution container, so a
		// second request carrying that first turn's blocks is refused with
		// "container_id is required when there are pending tool uses generated
		// by code execution" unless the container comes with it. Found by
		// running the dossier rather than by reading the shapes -- the failure
		// needs both a server tool *and* a second turn, so a single-turn probe
		// never sees it.
		if container != "" {
			params.Container = anthropic.MessageCreateParamsContainerUnion{
				OfString: anthropic.String(container),
			}
		}

		resp, err := con.Messages.New(ctx, params)
		if err != nil {
			// Nothing is recorded to the ledger on an API failure,
			// deliberately: the roll-up counts conversations, and a request
			// the API refused is not one.
			return Turn{}, &apiFailure{err: err, model: answering}
		}
		requests++
		tokensIn += resp.Usage.InputTokens
		tokensOut += resp.Usage.OutputTokens
		tokensCached += resp.Usage.CacheReadInputTokens
		if resp.Container.ID != "" {
			container = resp.Container.ID
		}

		// Checked before Content is read, because a refusal can carry an empty
		// content list and indexing into it is how this becomes a panic instead
		// of a message somebody can act on.
		if resp.StopReason == anthropic.StopReasonRefusal {
			record(string(resp.StopReason), resp.Model)
			return Turn{
				Mode: mode.Name, Model: resp.Model,
				StopReason: string(resp.StopReason), Text: "", ToolCalls: calls,
				InputTokens: int(tokensIn), OutputTokens: int(tokensOut),
				Searched: pages, SearchErrors: searchErrors, Refused: true,
				CacheReadTokens: int(tokensCached),
			}, nil
		}

		// Harvested before the stop-reason branch, so a turn that pauses or
		// ends still contributes what its searches found. Deduplicated on URL:
		// a mode that searches twice around a topic gets overlapping pages back
		// and the same source listed twice is noise in the evidence list.
		found, failed := serverResults(resp.Content)
		for _, page := range found {
			if !seenURLs[page.URL] {
				seenURLs[page.URL] = true
				pages = append(pages, page)
			}
		}
		searchErrors = append(searchErrors, failed...)

		// Appended whole, thinking blocks included. Sonnet 5 returns them with
		// empty text by default and they still have to go back unedited.
		history = append(history, assistantTurn(resp))

		// A server-side tool loop that hits its own iteration limit stops with
		// `pause_turn` and hands back text that reads finished. Returning it
		// here would be this project's own worst failure mode -- the Forge run
		// that plays on with 96 cards and reports a plausible winner. So it is
		// resumed instead: re-send with the paused turn last and the server
		// picks up where it stopped. Nothing is appended to nudge it along; a
		// trailing "continue" would be a new instruction rather than a
		// resumption. The loop's own ceiling is what stops this recurring.
		if resp.StopReason == anthropic.StopReasonPauseTurn {
			continue
		}

		if resp.StopReason != anthropic.StopReasonToolUse {
			var text strings.Builder
			for _, block := range resp.Content {
				if t, ok := block.AsAny().(anthropic.TextBlock); ok {
					text.WriteString(t.Text)
				}
			}
			record(string(resp.StopReason), resp.Model)
			return Turn{
				Mode: mode.Name, Model: resp.Model,
				StopReason: string(resp.StopReason),
				Text:       strings.TrimSpace(text.String()), ToolCalls: calls,
				InputTokens: int(tokensIn), OutputTokens: int(tokensOut),
				Searched: pages, SearchErrors: searchErrors,
				CacheReadTokens: int(tokensCached),
			}, nil
		}

		results := []anthropic.ContentBlockParamUnion{}
		var lastResult *anthropic.ToolResultBlockParam
		for _, block := range resp.Content {
			use, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			arguments := map[string]any{}
			// The block's input, parsed into a plain map. Numbers
			// arrive as float64, which is what the handlers already expect.
			if raw := use.JSON.Input.Raw(); raw != "" {
				if err := json.Unmarshal([]byte(raw), &arguments); err != nil {
					arguments = map[string]any{}
				}
			}
			calls = append(calls, ToolCall{Tool: use.Name, Arguments: arguments})

			text, isError, fatal := toolResult(ctx, mode, use.Name, arguments, req.Deps)
			if fatal != nil {
				return Turn{}, fmt.Errorf("%s: %w", mode.Name, fatal)
			}
			result := &anthropic.ToolResultBlockParam{
				ToolUseID: use.ID,
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: text}},
				},
				IsError: anthropic.Bool(isError),
			}
			results = append(results, anthropic.ContentBlockParamUnion{OfToolResult: result})
			lastResult = result
		}

		// Move the conversation's cache marker onto the newest results. The
		// next request then reads everything before them -- the earlier turns
		// and their search results -- from the cache at ~a tenth of the price,
		// instead of re-buying it. Moved rather than added: markers max out at
		// four per request, and earlier breakpoints keep working as read points
		// after the marker itself has gone.
		if lastResult != nil {
			if marked != nil {
				marked.CacheControl = anthropic.CacheControlEphemeralParam{}
			}
			marked = lastResult
			marked.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		history = append(history, anthropic.MessageParam{
			Role: anthropic.MessageParamRoleUser, Content: results,
		})
	}

	// `answering` -- what was *asked for* -- rather than a served-by value,
	// because this path has no final response to read one off. Every other
	// ledger row carries the response's model; this is the one that cannot, and
	// a row that named the house model while a tiered seat ran up the bill
	// would misattribute exactly the spend the tiers were added to make
	// visible.
	record("exhausted", answering)
	return Turn{}, &exhausted{msg: fmt.Sprintf("%s still wanted tools after %d turns "+
		"(%d calls made). Nothing was written; nothing is half-done.",
		mode.Name, maxTurns, len(calls))}
}

// assistantTurn is the response, re-sent as the next request's assistant
// message: **every block exactly as it arrived**, raw, rather than through
// the SDK's typed `ToParam`.
//
// Found on the real wire, 2026-08-23, by the dossier's live case and by
// nothing else. The dated web search filters its results inside a
// code-execution container, and the turn that ran it comes back carrying a
// `code_execution_tool_result` whose content is the **encrypted** result
// variant (`encrypted_code_execution_result`, `encrypted_stdout`).
// `anthropic-sdk-go` v1.66.0's `ToParam` has no branch for that variant: it
// falls into the plain-result branch, renders an empty `code_execution_result`
// with its required `content` elided, and the API refuses the whole request
// with a 400 naming the union it failed to match -- on the second turn of
// every dossier, after the search has been paid for. (`param.Override` on the
// block is the SDK's own escape hatch, used in its own `ToParam` for the text
// editor's variant of the same shape.) PLAN section 9 named exactly this risk:
// "anthropic-sdk-go beta-surface lag (server-tool details)". Research passed
// its live case the same hour only because its search happened not to run
// the filter.
//
// Raw for every block rather than for the one that broke, because that is
// the recorded behavior and because the next variant the SDK
// has not heard of would otherwise fail the same way, silently, in a
// conversation that had already cost four minutes.
func assistantTurn(resp *anthropic.Message) anthropic.MessageParam {
	content := make([]anthropic.ContentBlockParamUnion, 0, len(resp.Content))
	for _, block := range resp.Content {
		content = append(content,
			param.Override[anthropic.ContentBlockParamUnion](json.RawMessage(block.RawJSON())))
	}
	return anthropic.MessageParam{Role: anthropic.MessageParamRoleAssistant, Content: content}
}

// toolResult runs one tool call and says what the model should be told: the
// text of the tool_result block, whether it is an error, and -- separately --
// a fault that ends the conversation.
//
// Three failures are NOT faults and are handed back as `is_error` tool results
// instead: a tool that is not allowed, a tool whose arguments were rejected,
// and a deck that is not there. All three are things the model can recover
// from by asking differently, and a refused `set_card_field` in particular
// should read to the model as "that door does not exist" rather than ending
// the conversation.
//
// Which failures those are is decided by the error itself: an error carrying a
// `WireName` is one the tools package has declared recoverable, and the name it
// gives is the fault's name as the model hears it. Anything
// else is a fault this loop refuses to paper over -- a pool that will not open
// is not something the model can ask its way around.
func toolResult(ctx context.Context, mode Mode, name string, args map[string]any,
	deps tools.Deps) (text string, isError bool, fatal error) {
	out, err := tools.Run(ctx, name, args, deps, mode.ToolNames)
	if err != nil {
		var named interface{ WireName() string }
		if errors.As(err, &named) {
			return fmt.Sprintf("%s: %s", named.WireName(), err.Error()), true, nil
		}
		return "", false, err
	}
	// `wire.Marshal` rather than encoding/json's default, so the prose a tool
	// returns reaches the model as itself. Two rendering choices are
	// deliberate and both cheaper: non-ASCII is not `\u`-escaped (fewer
	// tokens for a card named Bösium Strip), and there are no spaces after
	// the separators. Neither is pinned by a corpus -- these bytes go to the
	// model, never onto a recorded wire.
	//
	// A failure here is ours rather than the model's: telling the model
	// "that tool broke" would hide an encoder bug behind a conversation that
	// carried on.
	raw, err := wire.Marshal(out)
	if err != nil {
		return "", false, fmt.Errorf("encoding %s result: %w", name, err)
	}
	return string(raw), false, nil
}

// serverResults is the pages a hosted search returned this turn, and any search
// that failed.
//
// Two shapes matter and they are easy to confuse. On success a
// `web_search_tool_result` block's content is a **list** of results; on failure
// it is a single **error object**. Reading the second as the first is how a
// failed search becomes an empty page list that reads as "found nothing" -- so
// the shape is checked rather than assumed. The union exposes both variants
// at once, so the check is on the raw JSON's own shape -- is it a list --
// which is the question that matters.
func serverResults(content []anthropic.ContentBlockUnion) ([]Page, []string) {
	pages := []Page{}
	errs := []string{}
	for _, block := range content {
		result, ok := block.AsAny().(anthropic.WebSearchToolResultBlock)
		if !ok {
			continue
		}
		if !isJSONArray(result.Content.RawJSON()) {
			code := string(result.Content.ErrorCode)
			if code == "" {
				code = result.Content.RawJSON()
			}
			errs = append(errs, code)
			continue
		}
		for _, item := range result.Content.AsWebSearchResultBlockArray() {
			if item.URL == "" {
				continue
			}
			title := item.Title
			if title == "" {
				title = item.URL
			}
			pages = append(pages, Page{URL: item.URL, Title: title})
		}
	}
	return pages, errs
}

// isJSONArray reports whether raw is a JSON array.
func isJSONArray(raw string) bool {
	trimmed := strings.TrimLeft(raw, " \t\r\n")
	return strings.HasPrefix(trimmed, "[")
}
