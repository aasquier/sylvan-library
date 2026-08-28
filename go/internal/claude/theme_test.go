package claude

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The theme interview, held to the recorded corpus `testdata/theme.json`
// with the opening angle pinned and the clock frozen.
//
// Two things in here are the point rather than the coverage.
//
// **The casefold row.** `Ground` folds the person's own turns and the quote
// the model claims they said with Unicode full case folding, which
// `strings.ToLower` is not: ToLower leaves `ß` alone, and a German answer
// would silently stop grounding under it. The corpus carries a slot quoting
// `Straße` against a transcript that says `STRASSE`, and a test below names
// it, because a passing corpus of ASCII would have said nothing.
//
// **Two report shapes per half.** A turn that reached the model carries
// `never`; one that did not does not. A proposal that resolved something
// carries five keys the other four shapes leave off, and they sit in the
// *middle*. Every outcome is compared as marshalled bytes with key order, so
// a single tidy struct per half fails here rather than on somebody's screen.

type themeCorpus struct {
	Floor         int               `json:"floor"`
	MaxExchanges  int               `json:"max_exchanges"`
	MaxTurnChars  int               `json:"max_turn_chars"`
	MaxFactChars  int               `json:"max_fact_chars"`
	MinQuoteChars int               `json:"min_quote_chars"`
	SlotKinds     []string          `json:"slot_kinds"`
	SlotQuestions map[string]string `json:"slot_questions"`
	AngleIndex    int               `json:"frozen_angle_index"`
	Transcript    []TranscriptTurn  `json:"transcript"`
	Pages         []Page            `json:"pages"`

	Prose []struct {
		Note  string `json:"note"`
		Value any    `json:"value"`
		Prose string `json:"prose"`
	} `json:"prose"`

	Ground []struct {
		Note       string          `json:"note"`
		Slots      []any           `json:"slots"`
		Kept       json.RawMessage `json:"kept"`
		Dropped    int             `json:"dropped"`
		MayPropose bool            `json:"may_propose"`
	} `json:"ground"`

	Carry []struct {
		Note       string          `json:"note"`
		Previous   []Slot          `json:"previous"`
		Fresh      []Slot          `json:"fresh"`
		Carried    json.RawMessage `json:"carried"`
		MayPropose bool            `json:"may_propose"`
	} `json:"carry"`

	Repeats []struct {
		Note    string   `json:"note"`
		Text    string   `json:"text"`
		Told    []string `json:"told"`
		Repeats bool     `json:"repeats"`
	} `json:"repeats"`

	Told []struct {
		Note  string   `json:"note"`
		Raw   any      `json:"raw"`
		Told  []string `json:"told"`
		Error string   `json:"error"`
	} `json:"told"`

	Transcripts []struct {
		Note  string           `json:"note"`
		Raw   any              `json:"raw"`
		Turns []TranscriptTurn `json:"turns"`
		Error string           `json:"error"`
	} `json:"transcripts"`

	Facts []struct {
		Note string          `json:"note"`
		Raw  any             `json:"raw"`
		Fact json.RawMessage `json:"fact"`
	} `json:"facts"`

	Seeds []struct {
		Note  string `json:"note"`
		Raw   any    `json:"raw"`
		Seed  string `json:"seed"`
		Error string `json:"error"`
	} `json:"seeds"`

	Budgets []struct {
		Note      string   `json:"note"`
		Value     *float64 `json:"value"`
		Formatted string   `json:"formatted"`
	} `json:"budgets"`

	Floats []struct {
		Note   string  `json:"note"`
		Raw    any     `json:"raw"`
		Budget *string `json:"budget"`
		Error  string  `json:"error"`
	} `json:"floats"`

	Stances []struct {
		Note      string          `json:"note"`
		Requested any             `json:"requested"`
		Ceiling   string          `json:"ceiling"`
		Stance    json.RawMessage `json:"stance"`
		Error     string          `json:"error"`
	} `json:"stances"`

	Prompts struct {
		Frames []struct {
			Note    string   `json:"note"`
			Persona string   `json:"persona"`
			Seed    *int64   `json:"seed"`
			Told    []string `json:"told"`
			Frame   string   `json:"frame"`
		} `json:"frames"`
		Closings []struct {
			Note    string   `json:"note"`
			Slots   []Slot   `json:"slots"`
			Told    []string `json:"told"`
			Opening bool     `json:"opening"`
			Closing string   `json:"closing"`
		} `json:"closings"`
		Asks []struct {
			Note   string   `json:"note"`
			Budget *float64 `json:"budget"`
			Avoid  string   `json:"avoid"`
			Ask    string   `json:"ask"`
		} `json:"asks"`
		Grounded      []Slot   `json:"grounded"`
		ReadingSeed   int64    `json:"reading_seed"`
		OpeningAngles []string `json:"opening_angles"`
	} `json:"prompts"`

	Refusals []struct {
		Half      string          `json:"half"`
		Note      string          `json:"note"`
		Slots     []any           `json:"slots"`
		Requested any             `json:"requested"`
		Persona   any             `json:"persona"`
		Seed      any             `json:"seed"`
		Facts     any             `json:"facts"`
		OK        json.RawMessage `json:"ok"`
		Error     string          `json:"error"`
		ErrorKind string          `json:"error_kind"`
	} `json:"refusals"`

	Asks []struct {
		Note       string           `json:"note"`
		Transcript []TranscriptTurn `json:"transcript"`
		Slots      []any            `json:"slots"`
		Requested  any              `json:"requested"`
		Persona    any              `json:"persona"`
		Seed       any              `json:"seed"`
		Facts      any              `json:"facts"`
		Turn       *turnRecord      `json:"turn"`
		Report     json.RawMessage  `json:"report"`
	} `json:"asks"`

	Proposals []struct {
		Note      string          `json:"note"`
		Slots     []any           `json:"slots"`
		Requested any             `json:"requested"`
		Budget    *float64        `json:"budget"`
		Avoid     string          `json:"avoid"`
		Persona   any             `json:"persona"`
		Seed      any             `json:"seed"`
		Turn      *turnRecord     `json:"turn"`
		Report    json.RawMessage `json:"report"`
	} `json:"proposals"`
}

func loadThemeCorpus(t *testing.T) themeCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "theme.json"))
	if err != nil {
		t.Fatalf("the theme corpus is missing; testdata/theme.json is a "+
			"frozen golden: %v", err)
	}
	var corpus themeCorpus
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&corpus); err != nil {
		t.Fatal(err)
	}
	return corpus
}

// freezeAngle pins the opening angle the way the corpus was written. The
// draw is OS-seeded and nothing reproducible rides on which one comes out --
// only that a test can hold it still.
func freezeAngle(t *testing.T, index int) {
	t.Helper()
	was := openingAngle
	openingAngle = func() string { return OpeningAngles[index] }
	t.Cleanup(func() { openingAngle = was })
}

// The constants the whole module is built out of. Cheap, and the one check
// that fails loudly when a cap moves here and not in the corpus.
func TestTheThemeConstantsMatchTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range []struct {
		what      string
		got, want int
	}{
		{"FLOOR", Floor, corpus.Floor},
		{"MAX_EXCHANGES", MaxExchanges, corpus.MaxExchanges},
		{"MAX_TURN_CHARS", MaxTurnChars, corpus.MaxTurnChars},
		{"MAX_FACT_CHARS", MaxFactChars, corpus.MaxFactChars},
		{"MIN_QUOTE_CHARS", MinQuoteChars, corpus.MinQuoteChars},
	} {
		if row.got != row.want {
			t.Errorf("%s is %d here and %d in the corpus", row.what, row.got, row.want)
		}
	}
	if strings.Join(SlotKinds, ",") != strings.Join(corpus.SlotKinds, ",") {
		t.Errorf("the slot kinds are %v here and %v in the corpus", SlotKinds, corpus.SlotKinds)
	}
	for kind, want := range corpus.SlotQuestions {
		if SlotQuestions[kind] != want {
			t.Errorf("%s reads\n  %q\nwant\n  %q", kind, SlotQuestions[kind], want)
		}
	}
	if len(SlotQuestions) != len(corpus.SlotQuestions) {
		t.Errorf("%d slot questions here, %d in the corpus", len(SlotQuestions), len(corpus.SlotQuestions))
	}
	if strings.Join(OpeningAngles, "|") != strings.Join(corpus.Prompts.OpeningAngles, "|") {
		t.Error("the opening angles have drifted from the corpus")
	}
}

func TestProseAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Prose {
		if got := Prose(row.Value); got != row.Prose {
			t.Errorf("%s: Prose(%#v) is %q, the corpus says %q", row.Note, row.Value, got, row.Prose)
		}
	}
}

func TestGroundMatchesTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Ground {
		kept, dropped := Ground(row.Slots, corpus.Transcript)
		assertSameJSONValue(t, "ground: "+row.Note, kept, row.Kept)
		if dropped != row.Dropped {
			t.Errorf("%s: dropped %d, the corpus dropped %d", row.Note, dropped, row.Dropped)
		}
		if got := MayPropose(kept); got != row.MayPropose {
			t.Errorf("%s: may_propose is %v, the corpus says %v", row.Note, got, row.MayPropose)
		}
	}
}

// The row an ASCII-only corpus could never have carried, named so the
// reason survives.
//
// Full case folding folds `ß` to `ss`; `strings.ToLower` leaves it alone.
// The person typed STRASSE and the model quoted Straße, which is a real
// German spelling and the only reachable disagreement between the two
// foldings in this module. Grounding it is the difference between a
// readiness count that moves and one that does not, which is what a newcomer
// reads as answering wrong.
func TestGroundFoldsFullyWhereToLowerWouldNot(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	found := false
	for _, row := range corpus.Ground {
		if !strings.Contains(row.Note, "casefold") {
			continue
		}
		found = true
		kept, dropped := Ground(row.Slots, corpus.Transcript)
		if len(kept) != 1 || dropped != 0 {
			t.Fatalf("the casefold row kept %d and dropped %d; ToLower is not "+
				"casefold and this is where it shows", len(kept), dropped)
		}
	}
	if !found {
		t.Fatal("the corpus no longer carries a casefold row; the one " +
			"reachable difference between casefold and ToLower is untested")
	}
	// And directly, so the reason is legible without the corpus.
	if casefold("Straße") != casefold("STRASSE") {
		t.Error("casefold does not make Straße and STRASSE equal")
	}
	// Spelled out rather than compared inline: the point is what `ToLower`
	// does to these two strings, which is the function a tidier `EqualFold`
	// would replace and the reason `casefold` exists.
	lowered, shouted := strings.ToLower("Straße"), strings.ToLower("STRASSE")
	if lowered == shouted {
		t.Error("ToLower now agrees with casefold here, so this test is moot")
	}
}

func TestCarryAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Carry {
		carried := Carry(row.Previous, row.Fresh)
		assertSameJSONValue(t, "carry: "+row.Note, carried, row.Carried)
		if got := MayPropose(carried); got != row.MayPropose {
			t.Errorf("%s: may_propose is %v, the corpus says %v", row.Note, got, row.MayPropose)
		}
	}
}

// The floor may not go backwards, stated as a property rather than a row.
// This is what `Carry` exists for and the failure it was written after.
func TestTheReadinessCountNeverFalls(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	known := []Slot{}
	for _, row := range corpus.Carry {
		before := len(known)
		known = Carry(known, row.Fresh)
		if len(known) < before {
			t.Fatalf("%s: the count fell from %d to %d", row.Note, before, len(known))
		}
	}
}

func TestRepeatsMatchesTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Repeats {
		if got := Repeats(row.Text, row.Told); got != row.Repeats {
			t.Errorf("%s: repeats is %v, the corpus says %v", row.Note, got, row.Repeats)
		}
	}
}

func TestCheckToldMatchesTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Told {
		told, err := CheckTold(row.Raw)
		if row.Error != "" {
			if err == nil {
				t.Errorf("%s: kept %v where the corpus says %q", row.Note, told, row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("%s: refused with\n  %q\nthe corpus says\n  %q", row.Note, err, row.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused with %q where the corpus kept %v", row.Note, err, row.Told)
			continue
		}
		if strings.Join(told, "|") != strings.Join(row.Told, "|") {
			t.Errorf("%s: kept %v, the corpus kept %v", row.Note, told, row.Told)
		}
	}
}

func TestCheckTranscriptAgreesWithTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Transcripts {
		turns, err := CheckTranscript(row.Raw)
		if row.Error != "" {
			if err == nil {
				t.Errorf("%s: accepted %d turns where the corpus says %q", row.Note, len(turns), row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("%s: refused with\n  %q\nthe corpus says\n  %q", row.Note, err, row.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused with %q where the corpus accepts", row.Note, err)
			continue
		}
		if len(turns) != len(row.Turns) {
			t.Errorf("%s: %d turns, the corpus has %d", row.Note, len(turns), len(row.Turns))
			continue
		}
		for i, turn := range turns {
			if turn != row.Turns[i] {
				t.Errorf("%s: turn %d is %+v, the corpus has %+v", row.Note, i, turn, row.Turns[i])
			}
		}
	}
}

func TestKeepFactMatchesTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Facts {
		assertSameJSONValue(t, "keep_fact: "+row.Note, KeepFact(row.Raw, corpus.Pages), row.Fact)
	}
}

// A `tarot:` citation renders the corpus's own sentence, never the model's.
// The whole of ADR 21's well in one assertion: the id was the ask, `text` came
// back only because the schema requires it, and a fun fact paraphrased at a
// fortune-teller's table is the one thing at that table that would be a lie.
func TestATarotFactIsTheCorpussWordsNotTheModels(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Facts {
		if !strings.Contains(row.Note, "paraphrased away") {
			continue
		}
		fact := KeepFact(row.Raw, corpus.Pages)
		if fact == nil {
			t.Fatal("the tarot fact was dropped")
		}
		if strings.Contains(fact.Text, "the model's own words") {
			t.Error("the model's paraphrase reached the querent")
		}
		return
	}
	t.Fatal("the corpus no longer carries a paraphrased tarot citation")
}

// The seed grammar is the lenient one, deliberately not the tarot route's
// strict decimal grammar. The fullwidth digit is the row that tells them
// apart: `/api/tarot/reading` refuses `７` and this reads it as seven.
func TestTheSeedGrammarIsTheLenientOneNotTheRoutes(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	sawFullwidth := false
	for _, row := range corpus.Seeds {
		got, err := intValue(row.Raw)
		if row.Error != "" {
			if err == nil {
				t.Errorf("%s: read %s where the corpus refuses", row.Note, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused where the corpus reads %s", row.Note, row.Seed)
			continue
		}
		if got.String() != row.Seed {
			t.Errorf("%s: read %s, the corpus reads %s", row.Note, got, row.Seed)
		}
		if strings.Contains(row.Note, "fullwidth") {
			sawFullwidth = true
		}
	}
	if !sawFullwidth {
		t.Fatal("the corpus no longer carries the fullwidth digit, which is " +
			"the one row separating the lenient grammar from the route's")
	}
}

func TestTheBudgetFormatMatchesTheGolden(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Budgets {
		var got string
		switch row.Note {
		case "inf":
			got = formatG(inf(1))
		case "-inf":
			got = formatG(inf(-1))
		case "nan":
			got = formatG(nan())
		default:
			if row.Value == nil {
				t.Fatalf("a budget row with no value and no note: %+v", row)
			}
			got = formatG(*row.Value)
		}
		if got != row.Formatted {
			t.Errorf("%v: formatted %q, the golden says %q", row.Value, got, row.Formatted)
		}
	}
}

func TestThemeStanceForMatchesTheCorpus(t *testing.T) {
	// **Serial**: it calls `t.Setenv(CeilingEnv)` per corpus row, which Go
	// panics on inside a parallel test -- the environment is one slot for the
	// whole process, so two tests setting it at once are one test reading the
	// other's deployment.
	//
	// The fix is `claude.Ceiling` taking its ceiling as an argument rather
	// than reading the process, which is what ADR 39/40 did for every other
	// piece of configuration in this package.
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Stances {
		t.Setenv(CeilingEnv, row.Ceiling)
		stance, err := ThemeStanceFor(row.Requested, nil)
		if row.Error != "" {
			if err == nil {
				t.Errorf("%s: resolved where the corpus says %q", row.Note, row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("%s: refused with\n  %q\nthe corpus says\n  %q", row.Note, err, row.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused with %q", row.Note, err)
			continue
		}
		assertSameJSONValue(t, "stance: "+row.Note, Describe(stance), row.Stance)
	}
}

// The three prompts, as bytes. All of them are assembled here from data this
// side owns, all of them reach a model, and none of them shows up in any
// report -- a frame that quietly lost its spread changes what was asked and is
// visible nowhere else.
func TestThePromptsAreBytes(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	freezeAngle(t, corpus.AngleIndex)

	for _, row := range corpus.Prompts.Frames {
		who, err := GetPersona(row.Persona)
		if err != nil {
			t.Fatal(err)
		}
		var seed *big.Int
		if row.Seed != nil {
			seed = big.NewInt(*row.Seed)
		}
		if got := frameFor(readingFor(who, seed), row.Told); got != row.Frame {
			t.Errorf("frame %q:\n got    %q\n corpus %q", row.Note, got, row.Frame)
		}
	}

	for _, row := range corpus.Prompts.Closings {
		transcript := corpus.Transcript
		if row.Opening {
			transcript = nil
		}
		if got := closingFor(row.Slots, transcript, row.Told); got != row.Closing {
			t.Errorf("closing %q:\n got    %q\n corpus %q", row.Note, got, row.Closing)
		}
	}

	for _, row := range corpus.Prompts.Asks {
		if got := proposalAsk(corpus.Prompts.Grounded, row.Budget, row.Avoid); got != row.Ask {
			t.Errorf("ask %q:\n got    %q\n corpus %q", row.Note, got, row.Ask)
		}
	}
}

// What each check refuses, and with which of the two errors -- because their
// statuses differ. `ErrNotReady` is a 409: nothing is malformed and nothing
// failed, there simply is not enough yet, and a 422 would read as "you sent
// something wrong" to a client that sent exactly the right thing too early.
func TestWhatTheTwoChecksRefuse(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	for _, row := range corpus.Refusals {
		var err error
		if row.Half == "ask" {
			transcript := anyTranscript(corpus.Transcript)
			if row.Note == "a malformed transcript" {
				transcript = []any{map[string]any{"role": "system", "text": "hi"}}
			}
			if row.Note == "an empty conversation is fine" {
				transcript = nil
			}
			_, err = CheckAsk(transcript, anySlots(row.Slots), row.Requested,
				row.Persona, row.Seed, row.Facts, "", nil, Endpoint{})
		} else {
			_, err = CheckProposal(anyTranscript(corpus.Transcript), anySlots(row.Slots),
				row.Requested, nil, "", row.Persona, row.Seed, "", nil, Endpoint{})
		}
		if row.Error == "" {
			if err != nil {
				t.Errorf("%s/%s: refused with %q where the corpus does not", row.Half, row.Note, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s/%s: accepted where the corpus says %q", row.Half, row.Note, row.Error)
			continue
		}
		if err.Error() != row.Error {
			t.Errorf("%s/%s: refused with\n  %q\nthe corpus says\n  %q", row.Half, row.Note, err, row.Error)
		}
		var notReady *ErrNotReady
		if isNotReady(err, &notReady) != (row.ErrorKind == "not-ready") {
			t.Errorf("%s/%s: the error kind is not the corpus's %q -- and the "+
				"two have different status codes", row.Half, row.Note, row.ErrorKind)
		}
	}
}

// Every outcome of a conversation turn, driven with the corpus's recorded
// Turn and compared as marshalled bytes.
func TestEveryThemeAskOutcomeMatchesTheGolden(t *testing.T) {
	// **Serial, and measured rather than read**: `noEnvOverrides` calls
	// `t.Setenv`, so Go panics on a `t.Parallel()` here. Nothing in this body
	// says so -- the call is one helper away, and that helper is in
	// `dossier_test.go`, a file away -- which is why the audit adds the line
	// and runs the test rather than reading for a reason.
	//
	// The blocker is `claude.Ceiling`, the last reader of
	// `MTGLAB_CLAUDE_STANCE_CEILING` left in this package: describing a
	// deployment still means installing one on the process here. When the
	// ceiling becomes a value the way the model override already did (ADR
	// 39/40), this test and its eight neighbours parallelise with nothing to
	// change but the deletion of this comment.
	noEnvOverrides(t)
	corpus := loadThemeCorpus(t)
	freezeAngle(t, corpus.AngleIndex)
	for _, row := range corpus.Asks {
		plan, err := CheckAsk(anyTranscript(row.Transcript), anySlots(row.Slots), row.Requested,
			row.Persona, row.Seed, row.Facts, "", nil, Endpoint{})
		if err != nil {
			t.Fatalf("%s: %v", row.Note, err)
		}
		var got any
		if plan.Answer != nil {
			if row.Turn != nil {
				t.Fatalf("%s: the corpus made a call and this plan did not", row.Note)
			}
			got = *plan.Answer
		} else {
			if row.Turn == nil {
				t.Fatalf("%s: this plan would call where the corpus did not", row.Note)
			}
			who, err := GetPersona(plan.Persona)
			if err != nil {
				t.Fatal(err)
			}
			mode, err := themeMode(ModeThemeConversation, who)
			if err != nil {
				t.Fatal(err)
			}
			got = readAsk(plan, who, mode.Name, row.Turn.turn(mode.Name))
		}
		assertSameJSONValue(t, "ask: "+row.Note, got, row.Report)
	}
}

// Every outcome of a proposal, the same way -- and this one needs the pool,
// because every commander named is resolved through it or dropped.
func TestEveryThemeProposalOutcomeAgreesWithTheCorpus(t *testing.T) {
	// **Serial, and measured rather than read**: `noEnvOverrides` calls
	// `t.Setenv`, so Go panics on a `t.Parallel()` here. Nothing in this body
	// says so -- the call is one helper away, and that helper is in
	// `dossier_test.go`, a file away -- which is why the audit adds the line
	// and runs the test rather than reading for a reason.
	//
	// The blocker is `claude.Ceiling`, the last reader of
	// `MTGLAB_CLAUDE_STANCE_CEILING` left in this package: describing a
	// deployment still means installing one on the process here. When the
	// ceiling becomes a value the way the model override already did (ADR
	// 39/40), this test and its eight neighbours parallelise with nothing to
	// change but the deletion of this comment.
	noEnvOverrides(t)
	corpus := loadThemeCorpus(t)
	withPool(t, func(c *pool.Conn) {
		ctx := context.Background()
		for _, row := range corpus.Proposals {
			plan, err := CheckProposal(anyTranscript(corpus.Transcript), anySlots(row.Slots),
				row.Requested, row.Budget, row.Avoid, row.Persona, row.Seed, "", nil, Endpoint{})
			if err != nil {
				t.Fatalf("%s: %v", row.Note, err)
			}
			var got any
			if plan.Answer != nil {
				if row.Turn != nil {
					t.Fatalf("%s: the corpus made a call and this plan did not", row.Note)
				}
				got = *plan.Answer
			} else {
				if row.Turn == nil {
					t.Fatalf("%s: this plan would call where the corpus did not", row.Note)
				}
				who, err := GetPersona(plan.Persona)
				if err != nil {
					t.Fatal(err)
				}
				mode, err := themeMode(ModeThemeProposal, who)
				if err != nil {
					t.Fatal(err)
				}
				got, err = readProposal(ctx, c, plan, who, mode.Name, row.Turn.turn(mode.Name))
				if err != nil {
					t.Fatalf("%s: %v", row.Note, err)
				}
			}
			assertSameJSONValue(t, "proposal: "+row.Note, got, row.Report)
		}
	})
}

// The second cache breakpoint rides the closing instruction, which is what
// stops every turn re-reading the whole conversation at full price.
//
// Asserted on the built messages rather than on a report, because it is not in
// one: `Converse`'s own moving marker only ever lands on a tool-result block
// it created, so nothing downstream would notice this going missing.
func TestTheCacheBreakpointRidesTheClosingInstruction(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	messages := themeMessages(corpus.Transcript, "CLOSING", "FRAME")

	// One block carries a marker, it is the last one, and it is the closing.
	marked := 0
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfText != nil && block.OfText.CacheControl.Type != "" {
				marked++
				if block.OfText.Text != "CLOSING" {
					t.Errorf("the marker is on %q, not the closing instruction", block.OfText.Text)
				}
			}
		}
	}
	if marked != 1 {
		t.Fatalf("%d cache breakpoints in the messages, want exactly one -- the "+
			"API allows four per request and Converse spends one more", marked)
	}

	last := messages[len(messages)-1]
	if last.Role != anthropic.MessageParamRoleUser {
		t.Fatal("the conversation must end on a user turn")
	}
	// The fixture ends on a user turn, so the closing rides *with* it rather
	// than as a turn of its own: the transcript the model sees is the
	// conversation and nothing else.
	if len(last.Content) != 2 {
		t.Errorf("the last turn has %d blocks; the closing should ride with "+
			"the answer, not replace or follow it", len(last.Content))
	}
	if messages[0].Content[0].OfText.Text != "FRAME" {
		t.Error("the frame is not first, so the roles are off by one")
	}
}

// A transcript that ends on the interviewer's own question gets the closing as
// a turn of its own rather than an edit to somebody else's.
func TestAnUnansweredQuestionGetsItsOwnClosingTurn(t *testing.T) {
	t.Parallel()
	messages := themeMessages([]TranscriptTurn{
		{Role: "assistant", Text: "What do you love?"},
	}, "CLOSING", "FRAME")
	last := messages[len(messages)-1]
	if last.Role != anthropic.MessageParamRoleUser || len(last.Content) != 1 {
		t.Fatalf("the closing did not get its own user turn: %+v", last)
	}
	if messages[len(messages)-2].Role != anthropic.MessageParamRoleAssistant {
		t.Error("the interviewer's own question was edited rather than answered")
	}
}

// ADR 20's first decision, held in the types: neither plan can carry a deck.
// The same shape `TestTheResearchPlanCannotHoldADeck` holds for ADR 26.
func TestNeitherThemePlanCanHoldADeck(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"AskPlan", "ProposalPlan"} {
		for _, banned := range []string{"Deck", "Source", "Slug", "Owner", "Library"} {
			if themePlanHasField(name, banned) {
				t.Errorf("%s has a %s field; a theme mode that can reach a deck "+
					"is the deck conversation ADR 20 leaves unbuilt", name, banned)
			}
		}
	}
}

// The ASCII fast path in `casefold` is an optimisation, never a second
// definition. All 128 through both, plus the whole table.
func TestTheCasefoldFastPathAgreesWithTheTable(t *testing.T) {
	t.Parallel()
	for r := rune(0); r < 0x80; r++ {
		s := string(r)
		fast, ok := casefoldASCII(s)
		if !ok {
			t.Fatalf("U+%04X is not ASCII to the fast path", r)
		}
		want := s
		if folded, in := folds[r]; in {
			want = folded
		}
		if fast != want {
			t.Errorf("U+%04X folds to %q on the fast path and %q in the table", r, fast, want)
		}
	}
	// And the table is a table of real folds: nothing maps to itself.
	for r, folded := range folds {
		if folded == string(r) {
			t.Errorf("U+%04X is in the table and folds to itself", r)
		}
	}
}

// Every Unicode decimal digit reads as its value, including the four
// mathematical runs that sit adjacent with no gap -- which is what the
// walk-down heuristic gets wrong and the table gets right.
func TestEveryUnicodeDigitReadsAsItsValue(t *testing.T) {
	t.Parallel()
	seen, bold := 0, 0
	for r := rune(0); r <= 0x10FFFF; r++ {
		if !unicode.IsDigit(r) {
			continue
		}
		seen++
		value, ok := digitValue(r)
		if !ok {
			t.Fatalf("U+%04X is category Nd and has no value", r)
			continue
		}
		if value < 0 || value > 9 {
			t.Fatalf("U+%04X reads as %d", r, value)
		}
		if r >= 0x1D7CE && r <= 0x1D7FF {
			bold++
			if want := int((r - 0x1D7CE) % 10); value != want {
				t.Errorf("U+%04X reads as %d, want %d -- the adjacent "+
					"mathematical runs are exactly what a walk-down misreads",
					r, value, want)
			}
		}
	}
	if seen == 0 || bold != 50 {
		t.Fatalf("swept %d digits and %d mathematical ones; the table looks wrong", seen, bold)
	}
}

// ---------------------------------------------------------------- helpers

func inf(sign int) float64 { return math.Inf(sign) }
func nan() float64         { return math.NaN() }

// anyTranscript rebuilds a decoded transcript as the `[]any` of maps a real
// request carries. Deliberately not a second accepted type on
// `CheckTranscript`: the door's whole job is to read what a client composed,
// and a test driving it through a Go type would be testing a path production
// never takes.
func anyTranscript(turns []TranscriptTurn) any {
	out := make([]any, 0, len(turns))
	for _, turn := range turns {
		out = append(out, map[string]any{"role": turn.Role, "text": turn.Text})
	}
	return out
}

// anySlots hands a corpus row's slots over as the `any` the checks take,
// keeping a nil nil rather than turning it into an empty list.
func anySlots(slots []any) any {
	if slots == nil {
		return nil
	}
	return slots
}

func isNotReady(err error, target **ErrNotReady) bool { return errors.As(err, target) }

// themePlanHasField reports whether one of the two plans declares a field by
// that name, over the typed struct rather than a grep -- so a field added
// through an embedded type is caught too.
func themePlanHasField(plan, field string) bool {
	var typ reflect.Type
	switch plan {
	case "AskPlan":
		typ = reflect.TypeOf(AskPlan{})
	case "ProposalPlan":
		typ = reflect.TypeOf(ProposalPlan{})
	default:
		panic("no such plan " + plan)
	}
	_, ok := typ.FieldByName(field)
	return ok
}

// `ReadBudget`, driven as the route drives it.
//
// **Through `ReadBudget` and not `FloatValue` by hand**, which is the tarot
// lane's lesson: a test that reimplements the call it is checking passes
// against a mutant of the caller. The falsy check, the grammar and the one
// refusal are all this function's, so all three are asked of it.
//
// Two halves. The **grammar** is the recorded one and is what a
// reimplementation gets wrong -- `1_000.5`, the fullwidth `５０`, `Infinity`,
// `1e400` overflowing to inf, and the `0x1p4` that `strconv` would read and
// this grammar refuses. Every accepted value is compared by its canonical
// decimal spelling, so one ulp is a diff.
//
// The **refusal** is one sentence for every way the field can fail, which it
// was not until 2026-08-23: a list was an uncaught 500 and a bad string a
// 422 quoting machinery at the user. The old test asserted that split and
// had a tripwire demanding the corpus keep it; both are gone, and what
// stands in their place asserts the opposite -- that no refusal names
// anything that computes, and that the corpus still holds a case of each
// former kind so this cannot be green against a corpus that lost one.
func TestTheBudgetParseMatchesTheRecordedCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadThemeCorpus(t)
	sawFormerTypeFailure, sawFormerStringFailure := false, false
	for _, row := range corpus.Floats {
		got, err := ReadBudget(row.Raw)
		if row.Error != "" {
			if err == nil {
				t.Errorf("%s: read %v where the corpus refuses with %q", row.Note, got, row.Error)
				continue
			}
			if err.Error() != row.Error {
				t.Errorf("%s: refused with\n  %q\nthe corpus says\n  %q", row.Note, err, row.Error)
			}
			if !errors.Is(err, ErrBudgetRejected) {
				t.Errorf("%s: refused with something other than the one refusal", row.Note)
			}
			// Commandment 10: the half a status code cannot check. This
			// sentence is rendered verbatim by the create flow.
			for _, leak := range []string{"float", "TypeError", "ValueError", "convert", "strconv"} {
				if strings.Contains(err.Error(), leak) {
					t.Errorf("%s: the refusal leaks %q at the user: %q", row.Note, leak, err)
				}
			}
			// Which of the two former failure families this row belongs to --
			// a wrong type or an unreadable string. Both must still be in the
			// corpus, or "they answer alike" is a claim about one of them.
			if _, isString := row.Raw.(string); isString {
				sawFormerStringFailure = true
			} else {
				sawFormerTypeFailure = true
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused with %q where the corpus reads %v", row.Note, err, row.Budget)
			continue
		}
		if row.Budget == nil {
			// A falsy budget is no budget: the truthiness check runs before
			// the grammar ever does, so an empty list never reaches the
			// refusal above.
			if got != nil {
				t.Errorf("%s: read %v where the corpus reads none", row.Note, *got)
			}
			continue
		}
		if got == nil {
			t.Errorf("%s: read nothing where the corpus reads %s", row.Note, *row.Budget)
			continue
		}
		if want := *row.Budget; floatLiteral(*got) != want {
			t.Errorf("%s: read %s, the corpus reads %s", row.Note, floatLiteral(*got), want)
		}
	}
	if !sawFormerTypeFailure || !sawFormerStringFailure {
		t.Fatal("the corpus no longer holds both budget failures -- a wrong " +
			"type (a list) and an unreadable string. They answer alike now, " +
			"and that is only tested while both are here")
	}
}

// floatLiteral is the corpus's decimal spelling of a float: the three special
// values spelled `nan`, `inf` and `-inf`, and otherwise the shortest
// round-tripping form, with a trailing `.0` where nothing else marks it a
// float.
func floatLiteral(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	out := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(out, ".eEni") {
		out += ".0"
	}
	return out
}

// The `d < 10` guard, driven against a table that does not know every digit.
//
// Through `digitValue` this is unreachable: `unicode.IsDigit` rejects
// anything outside a known run before the guard is consulted, so a mutation
// dropping it survives a sweep of all 680 digits. The case it exists for is
// a Unicode version moving under the toolchain while the recorded table
// stands still -- Go calling a rune a digit the table has never heard of --
// and `digitValueIn` takes its table as an argument precisely so that case
// can be built.
func TestADigitBeyondTheKnownRunsIsRefusedRatherThanGuessed(t *testing.T) {
	t.Parallel()
	// A table that stops at ASCII, standing in for one written before a block
	// existed. `\u0660` (Arabic-Indic zero) is a digit Go knows and this table
	// does not.
	stale := []rune{'0'}
	for _, r := range []rune{0x660, 0x661, 0x669, 0x1FBF0} {
		if value, ok := digitValueIn(stale, r); ok {
			t.Errorf("U+%04X is past every run this table knows and read as "+
				"%d; the guard is what stops a wrong digit being confident",
				r, value)
		}
	}
	// Below the first run start there is nothing to measure from either.
	if value, ok := digitValueIn(stale, '.'); ok {
		t.Errorf("U+002E read as %d", value)
	}
	// And the run the table does know still answers, so the guard is a
	// ceiling rather than a wall in front of the last block.
	for i := rune(0); i < 10; i++ {
		if value, ok := digitValueIn(stale, '0'+i); !ok || value != int(i) {
			t.Errorf("U+%04X reads as %d/%v", '0'+i, value, ok)
		}
	}
	// The real table answers the highest run, which is where a truncated one
	// would look identical to a correct one.
	last := digitZeros[len(digitZeros)-1]
	for i := rune(0); i < 10; i++ {
		if value, ok := digitValue(last + i); !ok || value != int(i) {
			t.Errorf("U+%04X reads as %d/%v", last+i, value, ok)
		}
	}
}
