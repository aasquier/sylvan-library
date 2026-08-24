package claude

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/claude/ledger"
	"github.com/aasquier/sylvan-library/go/internal/claude/tools"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// Research: the questions the pool cannot answer, with the pages that can.
//
// ADR 26 is the argument. The pool knows what every
// card does and the gate knows what is legal; neither knows what people think
// of a card this month, what a ruling means when it comes up, or that a set
// was spoiled last week.
//
// **It cannot reach a deck, and that is the load-bearing decision.** Look at
// what is absent: `ResearchPlan` has no slug and no source, `RunResearch`
// takes no deck, and the conversation's `Deps.Source` is nil by construction
// rather than by a caller remembering. So the mode cannot read a rationale,
// cannot see the 99, and cannot be asked what to cut from a list it has never
// been shown -- rule 4 is out of reach rather than forbidden. The same
// absence keeps deck conversation (ADR 15's third mode, deliberately unbuilt)
// from being built here by accident: making this mode into that one means
// adding a field on purpose, in a diff, and a test holds the field absent.
//
// Three instruments hold what is left, none of them the prompt. A cited page
// must be one the search returned (`KeepSources`), and here one step further:
// **a finding whose citations all failed is dropped and counted**, because
// unlike a dossier section there is no brief for it to rest on instead; if no
// source survives at all, the answer is refused. **A card the pool lacks is a
// labelled claim**, not a dropped one -- `in_pool: false` -- because a card
// spoiled ahead of the next `data refresh` is the third of the things this
// surface exists for. And a thin answer says it is thin, through
// `confidence`.
//
// Nothing is cached, and the absence is the decision: research's subject is
// the part of Magic that moves. `generated_at` rides in the payload as the
// honest substitute.

// MaxQuestion is the ceiling on a question's length in characters. Longer is
// a decklist, an essay, or a paste accident, and none of the three is a
// research question; refused in the request rather than minutes later.
const MaxQuestion = 2000

// ResearchConfidence is how sure the answer is that it is *agreed*, not how
// sure it is that it is right. Three, for the reason `Strengths` has three.
var ResearchConfidence = []string{"settled", "contested", "thin"}

// ResearchDefaultPreset is what "no preference" means here: `second-opinion`,
// not `collaborator` -- this mode answers a question that was asked, and
// collaborator's write axis has nothing to reach on a surface with no deck.
const ResearchDefaultPreset = "second-opinion"

// ResearchNever is the promise the payload carries about itself.
const ResearchNever = "This is Claude's reading of the cited pages, not the " +
	"tool's own answer. It has not seen any of your decks."

// ErrQuestionRejected is a question that is empty, or long enough to be
// something other than one. Its own type so the route answers 422 rather than
// spending a search to discover the same thing.
type ErrQuestionRejected struct{ Msg string }

func (e *ErrQuestionRejected) Error() string { return e.Msg }

// CheckQuestion reads the question as a string, or refuses. The read renders
// a falsy value empty before stripping -- so a number is its digits and an
// explicit null is empty -- and the ceiling counts code points, not bytes.
//
// Deliberately thin. There is no attempt to classify what the question is
// *about*: a model deciding whether a question counts as "about a deck" is
// exactly the judgement-call guard this project keeps refusing, and the
// structural version is that the mode has no deck to reach.
func CheckQuestion(raw any) (string, error) {
	question := textutil.Strip(plainOr(raw))
	if question == "" {
		return "", &ErrQuestionRejected{Msg: "Ask something. The research surface " +
			"takes a question about Magic in plain words."}
	}
	if n := textutil.Len(question); n > MaxQuestion {
		return "", &ErrQuestionRejected{Msg: fmt.Sprintf(
			"That question is %d characters, and the ceiling is %d. Anything "+
				"longer is usually a pasted decklist, and this surface cannot see "+
				"decks in any case -- ask the question on its own.", n, MaxQuestion)}
	}
	return question, nil
}

// QuestionKey is an identity for "somebody is asking this right now". Not a
// cache key -- nothing is stored -- but `jobs.Plan.Key`, so a second click
// inside the minutes a search takes joins the run already in flight.
// Whitespace and case are normalised because they do not make it a different
// question; anything else does.
//
// **This lowercases where a full case folding would fold**, and the gap is
// recorded rather than closed. Unicode full case folding rewrites `ß` to
// `ss` and `İ` to `i̇`, where `strings.ToLower` maps rune for rune; the two
// agree on every question a person has typed here and disagree on a handful
// of characters. The key never leaves this process: it is not in a job's
// payload, not on the wire, not in a store -- so there is nothing outside
// the process for it to agree *with*. What the key is for, two requests in
// one process joining, it does identically under either folding. Folding
// fully here would change the key for a property nobody can observe;
// `TestTheQuestionKeyLowercasesWhereFullFoldingDiffers` names the gap so it
// is known rather than found.
func QuestionKey(question string) string {
	normalised := strings.ToLower(textutil.SplitJoin(question))
	sum := sha256.Sum256([]byte(normalised))
	return "research:" + hex.EncodeToString(sum[:])[:16]
}

// ResearchStanceFor is this surface's default stance, and the clamp over
// what was asked for. There is no deck to derive a
// default from, and `Resolve(nil, nil)` is `off` -- right for "I have no idea
// what this is about" and wrong for a screen whose only control is a
// question box. Somebody typing a question has asked for a call.
func ResearchStanceFor(requested any, limit *Stance) (Stance, error) {
	if requested == nil {
		ceil := Ceiling()
		if limit != nil {
			ceil = *limit
		}
		preset, err := Preset(ResearchDefaultPreset)
		if err != nil {
			return Stance{}, err
		}
		return Clamp(preset, ceil), nil
	}
	return Resolve(requested, nil, limit)
}

// ---------------------------------------------------------------- the answer

// ResearchBody is the answer as served, in the recorded key order.
type ResearchBody struct {
	Answer   string    `json:"answer"`
	Findings []Finding `json:"findings"`
	// Cards is the cards the answer named: a `ResearchCard` for one the pool
	// has, an `UnresolvedCard` for one it lacks, in the order named.
	Cards []any `json:"cards"`
	// Confidence falls back to `thin` rather than dropping the answer, the
	// way `OnlyCharges` falls back on an unrecognised ground: an answer is
	// worth more than its label, and thin is the honest default for one that
	// arrived mislabelled.
	Confidence string   `json:"confidence"`
	Sources    []Source `json:"sources"`
	// All three surfaced rather than logged; a number that climbs is a prompt
	// that has started inventing.
	SourcesDropped  int `json:"sources_dropped"`
	FindingsDropped int `json:"findings_dropped"`
	// CardsUnresolved is how many named cards the pool has never heard of.
	// Not an error: for a spoiler question the right value is above zero.
	CardsUnresolved int `json:"cards_unresolved"`
	Searched        int `json:"searched"`
}

// ResearchReport is one response shape for every outcome, in the recorded
// key order. This mode needs `answered_by` more than any built so far: its
// output is prose about Magic with citations under it, which is the exact
// look of something reproducible, and it is not.
type ResearchReport struct {
	AnsweredBy string        `json:"answered_by"`
	Mode       string        `json:"mode"`
	Model      string        `json:"model"`
	Question   string        `json:"question"`
	Asked      bool          `json:"asked"`
	Reason     string        `json:"reason"`
	Stance     StanceReadout `json:"stance"`
	Research   any           `json:"research"`
	// GeneratedAt is when it was written. Nothing is cached and the subject
	// moves, so this is the honest substitute for a freshness claim rather
	// than a cache stamp.
	GeneratedAt string `json:"generated_at"`
	Usage       Usage  `json:"usage"`
	Never       string `json:"never"`
}

func researchReport(turn *Turn, question string, effective Stance, body any,
	asked bool, reason string) ResearchReport {
	report := ResearchReport{
		AnsweredBy: "claude", Mode: ModeResearch, Question: question,
		Asked: asked, Reason: reason, Stance: Describe(effective),
		Research: body, GeneratedAt: now(), Never: ResearchNever,
	}
	if body == nil {
		report.Research = emptyObject
	}
	if turn != nil {
		report.Model = turn.Model
		report.Usage = Usage{InputTokens: turn.InputTokens,
			OutputTokens: turn.OutputTokens, CacheReadTokens: turn.CacheReadTokens}
	}
	return report
}

// ------------------------------------------------------------- the two halves

// ResearchPlan is what `CheckResearch` settled and everything `RunResearch`
// needs.
//
// **Note what it does not carry, and could not**: a deck source, a slug, a
// deck. ADR 26's first decision is visible in the type, and
// `TestTheResearchPlanCannotHoldADeck` holds it there.
type ResearchPlan struct {
	Question  string
	Key       string
	Effective Stance
	// Tier is the asking seat's model grant, captured here for the same reason
	// the dossier's plan captures one: a job outlives the request that knew
	// who was asking, and re-deriving it in the worker would mean the worker
	// had a way to ask, which it must not.
	Tier   string
	Answer *ResearchReport
}

// NeedsCall reports whether anything still has to be asked of Anthropic.
func (p *ResearchPlan) NeedsCall() bool { return p.Answer == nil }

// CheckResearch is everything that can be decided without spending
// anything. An `ErrQuestionRejected` and a stance rejection
// come back to the caller, which is what keeps their 422 rather than
// flattening two answers into one job in state `error` minutes later.
func CheckResearch(raw any, requested any, tier string, limit *Stance) (*ResearchPlan, error) {
	question, err := CheckQuestion(raw)
	if err != nil {
		return nil, err
	}
	effective, err := ResearchStanceFor(requested, limit)
	if err != nil {
		return nil, err
	}
	plan := &ResearchPlan{Question: question, Key: QuestionKey(question),
		Effective: effective, Tier: tier}
	if !effective.AllowsCalls() {
		answer := researchReport(nil, question, effective, nil, false,
			"The stance is off, so no call was made. Nothing else about the "+
				"app is affected.")
		plan.Answer = &answer
	}
	return plan, nil
}

// ResearchRun is what `RunResearch` needs beyond the plan. There is no deck
// source in it, and `RunResearch` builds the conversation's `Deps` itself
// with the source left nil -- the pool is the only thing the tools may reach.
type ResearchRun struct {
	Ledger *ledger.Recorder
	OnTurn func(done, max int)
}

// RunResearch makes the call, checks what came back, and hands it over.
// Nothing is stored at the end, and the absence is the decision (ADR 26).
func RunResearch(ctx context.Context, conn *pool.Conn, plan *ResearchPlan, run ResearchRun) (ResearchReport, error) {
	if plan.Answer != nil {
		return *plan.Answer, nil
	}
	mode, err := GetMode(ModeResearch)
	if err != nil {
		return ResearchReport{}, err
	}
	turn, err := Converse(ctx, mode, Request{
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(researchOpening(plan.Question))),
		},
		Stance: plan.Effective,
		// No deck source, and the nil is the point rather than a default
		// nobody filled in: `get_cards` does not take one, and every tool that
		// would have needed one is absent from this mode's tool set.
		Deps:   tools.Deps{Source: nil, Pool: conn},
		Tier:   plan.Tier,
		Ledger: run.Ledger,
		// A search, a look at what came back, a lookup of the cards named, a
		// second search, and the write-up. The dossier's eight, for the same
		// reason: a paused turn can spend one.
		MaxTurns: 8,
		OnTurn:   run.OnTurn,
	})
	if err != nil {
		return ResearchReport{}, err
	}
	return readResearch(ctx, conn, plan, turn)
}

// readResearch is the half of RunResearch after the call, split from it so
// the corpus can drive it with a Turn built by hand.
func readResearch(ctx context.Context, conn *pool.Conn, plan *ResearchPlan, turn Turn) (ResearchReport, error) {
	question, effective := plan.Question, plan.Effective
	if turn.Refused {
		return researchReport(&turn, question, effective, nil, true,
			"The model declined to answer this one."), nil
	}
	var payload map[string]any
	if err := turn.Parsed(&payload); err != nil {
		//nolint:nilerr // an unreadable answer is a reported outcome, not a fault
		return researchReport(&turn, question, effective, nil, true,
			fmt.Sprintf("The answer did not parse (stop reason: %s).", turn.StopReason)), nil
	}
	claimed, _ := payload["sources"].([]any)
	sources, sourcesDropped := KeepSources(claimed, turn.Searched)
	if len(sources) == 0 {
		// ADR 26, following ADR 19: an unsourced research answer is a model
		// talking about Magic from memory with a search box drawn around it.
		return researchReport(&turn, question, effective, nil, true,
			"No source survived checking, so there is nothing to stand behind "+
				"an answer."+noSourceDetail(turn, sourcesDropped)), nil
	}
	allowed := map[string]bool{}
	for _, s := range sources {
		allowed[s.ID] = true
	}
	findingsRaw, _ := payload["findings"].([]any)
	findings, findingsDropped := OnlyGrounded(findingsRaw, allowed)
	if len(findings) > MaxFindings {
		findings = findings[:MaxFindings]
	}
	cardsRaw, _ := payload["cards"].([]any)
	cards, unresolved, err := ResolveCards(ctx, conn, cardsRaw, MaxResearchCards)
	if err != nil {
		return ResearchReport{}, err
	}
	confidence := strings.TrimSpace(plainOr(payload["confidence"]))
	body := ResearchBody{
		Answer:          strings.TrimSpace(plainOr(payload["answer"])),
		Findings:        findings,
		Cards:           cards,
		Confidence:      oneOf(confidence, ResearchConfidence, "thin"),
		Sources:         sources,
		SourcesDropped:  sourcesDropped,
		FindingsDropped: findingsDropped,
		CardsUnresolved: unresolved,
		Searched:        len(turn.Searched),
	}
	return researchReport(&turn, question, effective, body, true, ""), nil
}

// researchOpening frames the question as the user's rather than as the
// tool's -- quoted rather than interpolated into an instruction, so what
// follows reads as something a person typed.
func researchOpening(question string) string {
	return strings.Join([]string{
		"Here is the question, in the user's own words. Search for what you " +
			"need, look up every card you are going to name, and answer it.",
		"",
		question,
	}, "\n")
}
