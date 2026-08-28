package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/textutil"
)

// Research, held to the recorded corpus in `testdata/research.json`.
//
// What a person controls here is the question, so the corpus is heaviest
// where the body can vary: the plain rendering over a number, a list, a
// null; length in code points; the wider whitespace the recorded grammar
// strips. Every outcome of a run is compared as bytes, as the dossier's are.
// And the structural claim ADR 26 makes -- that nothing here can hold a deck
// -- is a test over the types rather than a sentence in a comment.

type researchCorpus struct {
	Questions []struct {
		Raw      any     `json:"raw"`
		Question *string `json:"question"`
		Rejected *string `json:"rejected"`
	} `json:"questions"`
	Keys []struct {
		Question string `json:"question"`
		Key      string `json:"key"`
	} `json:"keys"`
	CasefoldGap struct {
		Question      string `json:"question"`
		Key           string `json:"key"`
		LowercasedKey string `json:"lowercased_key"`
	} `json:"casefold_gap"`
	Labels []struct {
		Question string `json:"question"`
		Label    string `json:"label"`
	} `json:"labels"`
	StanceFor []struct {
		Ceiling   *string         `json:"ceiling"`
		Requested any             `json:"requested"`
		Describe  json.RawMessage `json:"describe"`
	} `json:"stance_for"`
	AskFor []struct {
		Question string `json:"question"`
		Message  string `json:"message"`
	} `json:"ask_for"`
	Reports []reportCase `json:"reports"`
}

func loadResearchCorpus(t *testing.T) researchCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "research.json"))
	if err != nil {
		t.Fatalf("reading the corpus: %v", err)
	}
	// UseNumber, because `readBody` does: a question that arrived as `7`
	// reaches CheckQuestion as a json.Number in production.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var corpus researchCorpus
	if err := decoder.Decode(&corpus); err != nil {
		t.Fatalf("decoding the corpus: %v", err)
	}
	if len(corpus.Questions) == 0 || len(corpus.Reports) == 0 {
		t.Fatal("the corpus is empty; testdata/research.json is a frozen golden and always carries both")
	}
	return corpus
}

// ------------------------------------------------------------ the question

func TestCheckQuestionMatchesTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadResearchCorpus(t)
	sawRejection, sawOddShape := false, false
	for _, row := range corpus.Questions {
		got, err := CheckQuestion(row.Raw)
		switch {
		case row.Rejected != nil:
			sawRejection = true
			var rejected *ErrQuestionRejected
			if !errors.As(err, &rejected) {
				t.Errorf("%v: the corpus refuses, this answered %q / %v", row.Raw, got, err)
				continue
			}
			if rejected.Error() != *row.Rejected {
				t.Errorf("%v: refusal\n got    %q\n corpus %q", row.Raw, rejected.Error(), *row.Rejected)
			}
		default:
			if err != nil {
				t.Errorf("%v: the corpus accepts %q, this refused: %v", row.Raw, *row.Question, err)
				continue
			}
			if got != *row.Question {
				t.Errorf("%v: question\n got    %q\n corpus %q", row.Raw, got, *row.Question)
			}
			if _, isString := row.Raw.(string); !isString {
				sawOddShape = true
			}
		}
	}
	if !sawRejection || !sawOddShape {
		t.Fatalf("the corpus must carry a refusal and a non-string question that is accepted (%v, %v)",
			sawRejection, sawOddShape)
	}
}

// The recorded grammar counts the four information separators as whitespace
// where `strings.TrimSpace` does not, so a question that is only `\x1c\x1d`
// is empty only because textutil.IsSpace says so. The corpus holds that
// case; this names the mechanism.
func TestTheInformationSeparatorsCountAsWhitespace(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace("\x1c\x1d") == "" {
		t.Skip("Go's TrimSpace now strips the information separators; the helper is redundant")
	}
	if textutil.Strip("\x1c\x1dq\x1e") != "q" {
		t.Errorf("textutil.Strip did not strip the information separators: %q", textutil.Strip("\x1c\x1dq\x1e"))
	}
	if _, err := CheckQuestion("\x1c\x1d"); err == nil {
		t.Error("a question of information separators was accepted; the recorded grammar refuses it as empty")
	}
}

func TestTheQuestionKeyMatchesTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadResearchCorpus(t)
	for _, row := range corpus.Keys {
		if got := QuestionKey(row.Question); got != row.Key {
			t.Errorf("QuestionKey(%q) = %q, corpus %q", row.Question, got, row.Key)
		}
	}
	// Two spellings of one question are one job.
	if QuestionKey("Is Goreclaw still played?") != QuestionKey("  is   goreclaw\tstill played?  ") {
		t.Error("whitespace and case made two keys of one question")
	}
	if QuestionKey("Is Goreclaw still played?") == QuestionKey("Is Goreclaw still played") {
		t.Error("a different question keyed the same")
	}
}

// The one recorded gap, pinned so it is known rather than found: Unicode
// full case folding rewrites `ß` to `ss` where this lowercases rune for
// rune. The key never leaves the process -- not in a payload, not in a store
// -- so nothing can observe the difference; what it is FOR, two requests in
// one process joining, either folding does. If this test ever fails because
// the two keys agree, a full folding arrived and the comment on QuestionKey
// is the thing to delete.
func TestTheQuestionKeyLowercasesWhereFullFoldingDiffers(t *testing.T) {
	t.Parallel()
	corpus := loadResearchCorpus(t)
	gap := corpus.CasefoldGap
	if got := QuestionKey(gap.Question); got != gap.LowercasedKey {
		t.Errorf("QuestionKey(%q) = %q, want the lowercased key %q", gap.Question, got, gap.LowercasedKey)
	}
	if gap.Key == gap.LowercasedKey {
		t.Fatal("the corpus's casefold gap is not a gap; pick an input that casefolds differently")
	}
}

// ------------------------------------------------------------ the stance

func TestResearchStanceForAgreesWithTheCorpus(t *testing.T) {
	// **Serial**: it calls `t.Setenv(CeilingEnv)` once per corpus row, to drive the ceiling
	// this mapping is read under, which Go
	// panics on inside a parallel test -- the environment is one slot for the
	// whole process, so two tests setting it at once are one test reading the
	// other's deployment.
	//
	// The fix is `claude.Ceiling` taking its ceiling as an argument rather
	// than reading the process, which is what ADR 39/40 did for every other
	// piece of configuration in this package.
	corpus := loadResearchCorpus(t)
	for _, row := range corpus.StanceFor {
		ceiling := ""
		if row.Ceiling != nil {
			ceiling = *row.Ceiling
		}
		t.Setenv(CeilingEnv, ceiling)
		got, err := ResearchStanceFor(row.Requested, nil)
		if err != nil {
			t.Errorf("ceiling %q requested %v: %v", ceiling, row.Requested, err)
			continue
		}
		assertSameJSONValue(t, "ceiling "+ceiling+" requested "+literalJSON(row.Requested),
			Describe(got), row.Describe)
	}
}

// The default is not `off`: a surface whose only control is a question box
// has been asked for a call. `Resolve(nil, nil)` would answer off, and that
// is the bug `/api/claude` once had.
func TestTheDefaultResearchStanceIsNotOff(t *testing.T) {
	// **Serial**: it calls `t.Setenv(CeilingEnv)` to blank the ceiling, because what it asserts
	// is the default and a ceiling on the machine would be a different question, which Go
	// panics on inside a parallel test -- the environment is one slot for the
	// whole process, so two tests setting it at once are one test reading the
	// other's deployment.
	//
	// The fix is `claude.Ceiling` taking its ceiling as an argument rather
	// than reading the process, which is what ADR 39/40 did for every other
	// piece of configuration in this package.
	t.Setenv(CeilingEnv, "")
	s, err := ResearchStanceFor(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !s.AllowsCalls() {
		t.Fatal("the default research stance makes no calls")
	}
	if s != SecondOpinion {
		t.Errorf("the default is %+v, want second-opinion", s)
	}
	// And a deployment ceiling still clamps it.
	t.Setenv(CeilingEnv, "consultant")
	s, _ = ResearchStanceFor(nil, nil)
	if s != Consultant {
		t.Errorf("under a consultant ceiling the default is %+v", s)
	}
	// Off is still reachable.
	s, _ = ResearchStanceFor("off", nil)
	if s.AllowsCalls() {
		t.Error("off was not reachable")
	}
}

func TestTheResearchOpeningIsTheRecordedShape(t *testing.T) {
	t.Parallel()
	corpus := loadResearchCorpus(t)
	for _, row := range corpus.AskFor {
		if got := researchOpening(row.Question); got != row.Message {
			t.Errorf("opening for %q:\n got    %q\n corpus %q", row.Question, got, row.Message)
		}
	}
}

// ------------------------------------------------------------- the runs

func TestEveryResearchOutcomeMatchesTheGolden(t *testing.T) {
	// **Serial, and measured rather than read**: `noEnvOverrides` calls
	// `t.Setenv`, so Go panics on a `t.Parallel()` here. Nothing in this body
	// says so -- the call is one helper away, and the helper is in `dossier_test.go`, a file away --
	// which is why the audit adds the line and runs the test rather than
	// reading for a reason.
	//
	// The blocker is `claude.Ceiling`, the last reader of
	// `MTGLAB_CLAUDE_STANCE_CEILING` left in this package: describing a
	// deployment still means installing one on the process here. When the
	// ceiling becomes a value the way the model override already did (ADR
	// 39/40), this test and its eight neighbours parallelise with nothing to
	// change but the deletion of this comment.
	noEnvOverrides(t)
	freezeClock(t)
	corpus := loadResearchCorpus(t)
	sawAnswer := false
	withPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		for _, row := range corpus.Reports {
			plan, err := CheckResearch(row.Question, row.Requested, "", nil, Endpoint{})
			if err != nil {
				t.Fatalf("%s: check: %v", row.Note, err)
			}
			var report ResearchReport
			if row.Turn == nil {
				if plan.Answer == nil {
					t.Errorf("%s: the plan wants a call and the corpus made none", row.Note)
					continue
				}
				report = *plan.Answer
			} else {
				if plan.Answer != nil {
					t.Errorf("%s: the plan answered without a call and the corpus called", row.Note)
					continue
				}
				report, err = readResearch(ctx, c, plan, row.Turn.turn(ModeResearch))
				if err != nil {
					t.Fatalf("%s: %v", row.Note, err)
				}
				if report.Reason == "" {
					sawAnswer = true
				}
			}
			assertSameJSONValue(t, row.Note, report, row.Report)
		}
	})
	if !sawAnswer {
		t.Fatal("no corpus case carries a grounded answer; the body's shape went untested")
	}
}

// ---------------------------------------------------------- the absence

// ADR 26's first decision, visible in the types: nothing a research run is
// handed can hold a deck. A field named for one, or typed as one, is the line
// somebody would add to make this mode deck-aware, and the diff that adds it
// has to fail something.
func TestTheResearchPlanCannotHoldADeck(t *testing.T) {
	t.Parallel()
	for _, typ := range []reflect.Type{
		reflect.TypeOf(ResearchPlan{}), reflect.TypeOf(ResearchRun{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			lower := strings.ToLower(field.Name)
			for _, banned := range []string{"source", "slug", "deck", "library", "owner"} {
				if strings.Contains(lower, banned) {
					t.Errorf("%s.%s names a %s; research cannot reach a deck (ADR 26)", typ.Name(), field.Name, banned)
				}
			}
			if strings.Contains(field.Type.String(), "deck") || strings.Contains(field.Type.String(), "library") {
				t.Errorf("%s.%s is a %s; research cannot reach a deck (ADR 26)", typ.Name(), field.Name, field.Type)
			}
		}
	}
}

// And the mode itself offers no deck tool -- the structural half, pinned
// here as a set rather than tool by tool.
func TestTheResearchModeOffersNoDeckTool(t *testing.T) {
	t.Parallel()
	mode, err := GetMode(ModeResearch)
	if err != nil {
		t.Fatal(err)
	}
	if len(mode.ToolNames) != 1 || mode.ToolNames[0] != "get_cards" {
		t.Errorf("research's tools are %v; the only pool door is get_cards", mode.ToolNames)
	}
}
