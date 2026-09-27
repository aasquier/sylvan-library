package claude

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// A prompt goes stale when the code around it moves, and nothing here was
// watching that seam.
//
// `modes_test.go` holds each mode's *schema* to the design it encodes -- the
// slot argument's missing defence, the scan's missing card name. Nothing held
// the **instructions** to anything, so three runs of the polish pass read "the
// prompts are byte-untouched, so there is nothing new to read" and were reading
// the wrong half: `data/modes.json` had not moved since #392 and the code it
// describes had moved a great deal. What that cost, in the reading that found
// it: two prompts naming the wrong language for the gate, three telling the
// model its brief carried counts and a curve that were never in it, five
// holding tools they never mentioned, one saying a rationale is composed
// nowhere after ADR 41 made one surface compose them, one describing a single
// corpus where the runtime offers two, and nine of ten ending on a scope
// paragraph about a card, a deck or a gate that was not there.
//
// So these are the seams, each read off the source of truth rather than
// restated: the tool grant, the caps that truncate an answer, the verdicts a
// dropped alternative can be filed under, the source prefixes `KeepFact`
// accepts, the brief the intake actually assembles, and the scope axis itself.
// A test that merely repeated the prompt back would pass on the day the code
// moved, which is the entire failure being fixed.

// everyMode is every defined mode, so a new one is swept without being listed.
func everyMode(t *testing.T) []Mode {
	t.Helper()
	out := make([]Mode, 0, len(ModeNames()))
	for _, name := range ModeNames() {
		m, err := GetMode(name)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		t.Fatal("no modes at all")
	}
	return out
}

// modelFacingText is everything of a mode's definition that reaches a model:
// the instructions and every description in the response schema.
func modelFacingText(t *testing.T, m Mode) string {
	t.Helper()
	raw, err := json.Marshal(m.ResponseSchema)
	if err != nil {
		t.Fatalf("mode %q's schema will not marshal: %v", m.Name, err)
	}
	return m.Instructions + "\n" + string(raw)
}

// flattened collapses a prompt's own line wrapping, so a phrase can be looked
// for as a phrase. Every one of these files is hard-wrapped at about 78
// columns, which means any sentence long enough to be worth checking has a
// newline somewhere in the middle of it.
func flattened(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// numberWord is how a prompt spells a small cap, since these are sentences
// rather than configuration. Only as far as the caps actually go.
var numberWord = map[int]string{
	1: "one", 2: "two", 3: "three", 4: "four", 5: "five", 6: "six",
	7: "seven", 8: "eight", 9: "nine", 10: "ten", 11: "eleven", 12: "twelve",
}

// Commandment 10 binds here too. A prompt is model-facing, but a model asked
// about its own instructions echoes them, and the sentence that broke this was
// not a leak risk in the abstract -- it was simply *wrong*: the gate has never
// been anything but Go, and two prompts called it deterministic Python for a
// year. The fix in both was to drop the language rather than swap it, because
// the true sentence is "the gate is deterministic" and the language was never
// the point.
func TestNoPromptNamesTheTechnologyUnderneathIt(t *testing.T) {
	t.Parallel()
	// Words that are only ever a technology. Deliberately not "Go", "Java" or
	// "SQL" -- "go" is in half these sentences, and a word that needs a parser
	// to judge is a word this sweep would be wrong about.
	forbidden := []string{
		"Python", "Golang", "JavaScript", "TypeScript", "DuckDB", "SQLite",
		"PostgreSQL", "Postgres", "React", "Kubernetes", "Docker", "Anthropic",
		"Sonnet", "Opus", "Haiku",
	}
	for _, m := range everyMode(t) {
		text := modelFacingText(t, m)
		for _, word := range forbidden {
			if strings.Contains(text, word) {
				t.Errorf("mode %q's model-facing text names %q. Commandment 10: "+
					"no technology renders, and the two sentences that put a "+
					"language here were wrong about it as well -- say what is "+
					"true (the gate is deterministic) and name nothing",
					m.Name, word)
			}
		}
	}
}

// A tool a mode holds and never mentions is either a prompt that forgot it or
// a grant nobody needs, and both are worth failing over.
//
// The grant is the source of truth: `mode.ToolNames` is what `tools.Schemas`
// puts on the wire. Five of the ten modes held between two and five tools and
// named exactly one of them -- and `Mode.Effort` is `high` precisely because a
// mode that answers from recall instead of calling `get_cards` is rule 1
// failing quietly, which is the same failure one step earlier.
func TestEveryModeNamesTheToolsItWasGranted(t *testing.T) {
	t.Parallel()
	for _, m := range everyMode(t) {
		for _, tool := range m.ToolNames {
			if !strings.Contains(m.Instructions, tool) {
				t.Errorf("mode %q is granted %s and its prompt never says the "+
					"name. Name it and say what it is for, or drop the grant -- "+
					"a tool the model was not told about is a tool it reaches "+
					"for last", m.Name, tool)
			}
		}
	}
}

// `defaultScopeNotes` talks about "the card you were asked about" and "the
// rest of the deck", which is exactly right for the two modes that were handed
// one card in one deck and nonsense to every other.
//
// The two sides are read off the definitions themselves: a mode takes the
// default when `ScopeNotes` is nil, and a mode is about one card in one deck
// when it says so in its own opening sentence. They must agree. That is what
// `Mode.ScopeNotes`' own doc comment asks for -- "a mode with no card and no
// deck has to say what its own scope axis widens, or the prompt tells it to
// stay on something that does not exist" -- and for a long time exactly one of
// the ten did it.
func TestOnlyAModeAboutOneCardInOneDeckKeepsTheDefaultScopeNotes(t *testing.T) {
	t.Parallel()
	// The phrase both such prompts open on, and the thing the default table is
	// written about.
	const aboutOneCard = "one card in one deck"
	for _, m := range everyMode(t) {
		usesDefault := m.ScopeNotes == nil
		isAboutOneCard := strings.Contains(flattened(m.Instructions), aboutOneCard)
		switch {
		case usesDefault && !isAboutOneCard:
			t.Errorf("mode %q takes the default scope notes, which end every "+
				"prompt with \"stay on the card you were asked about\" and "+
				"\"do not range into the rest of the deck\" -- and this mode's "+
				"prompt never says it is about %s. Give it its own table",
				m.Name, aboutOneCard)
		case !usesDefault && isAboutOneCard:
			t.Errorf("mode %q is about %s and supplies its own scope notes. "+
				"The default table was written for exactly this mode; a second "+
				"copy of it is a second thing to keep in step",
				m.Name, aboutOneCard)
		}
	}
}

// A mode's own table must answer the whole scope axis, or a stance renders an
// empty paragraph and the prompt simply stops mid-thought.
//
// `Scope` is the axis, so a fourth level added there fails here rather than
// silently dropping the note on the modes that answer for themselves.
func TestAModesOwnScopeNotesAnswerEveryLevelOfTheAxis(t *testing.T) {
	t.Parallel()
	for _, m := range everyMode(t) {
		if m.ScopeNotes == nil {
			continue
		}
		for _, level := range Scope {
			note := strings.TrimSpace(m.ScopeNotes[level])
			if note == "" {
				t.Errorf("mode %q has its own scope notes and nothing for %q -- "+
					"a stance at that level renders a prompt that ends on a "+
					"blank line", m.Name, level)
			}
		}
		if len(m.ScopeNotes) != len(Scope) {
			t.Errorf("mode %q's scope notes hold %d entries for an axis of %d "+
				"levels (%v) -- a key the axis does not have is never read",
				m.Name, len(m.ScopeNotes), len(Scope), Scope)
		}
	}
}

// A cap that truncates an answer the model was never told about is a silent
// cut: the reader sees a list that stops, and the model spent output tokens on
// the part that was thrown away.
//
// Every number here is read off the constant that does the truncating, so
// raising one fails until the sentence moves with it.
func TestThePromptsSayTheCapsThatTruncateTheirAnswers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode string
		cap  int
		what string
	}{
		{ModeSlotArgument, MaxCharges, "charges (argue.go trims past it)"},
		{ModeSlotArgument, MaxAlternatives, "alternatives (ResolveAlternatives trims past it)"},
		{ModeResearch, MaxFindings, "findings (research.go trims past it)"},
		{ModeResearch, MaxResearchCards, "cards (ResolveCards trims past it)"},
	} {
		m, err := GetMode(tc.mode)
		if err != nil {
			t.Fatal(err)
		}
		word, ok := numberWord[tc.cap]
		if !ok {
			t.Fatalf("the cap for %s is %d and this test has no word for it",
				tc.what, tc.cap)
		}
		if !strings.Contains(modelFacingText(t, m), word) {
			t.Errorf("mode %q caps %s at %d and its model-facing text never "+
				"says %q -- the cut happens anyway, out of sight",
				tc.mode, tc.what, tc.cap, word)
		}
	}
}

// Every way a named alternative can be thrown away must be a way the prompt
// says it can be thrown away.
//
// `DroppedAlternatives`' fields ARE that list -- the struct is what
// `ResolveAlternatives` files a rejection under and what the report carries
// back -- so this walks it by reflection. A sixth verdict added there fails
// here until somebody says how the prompt names it, which is the point: the
// prompt named three of the four filters for a year while the schema beside it
// named four, and the missing one (already-in-deck) is the verdict the code
// checks *first*.
func TestTheAlternativesRuleNamesEveryVerdictAnAlternativeCanGet(t *testing.T) {
	t.Parallel()
	// One phrase per verdict, and the phrase must be in the model-facing text.
	// `NoPool` is not a filter -- it is "there is no pool at all", which is the
	// base install and not something a model can avoid -- so it is named here
	// as deliberately unsaid rather than left out of the walk.
	says := map[string]string{
		"NotInPool":     "pool",
		"Banned":        "ban list",
		"OffColour":     "colour identity",
		"AlreadyInDeck": "already runs",
		"NoPool":        "", // nothing to tell the model; see above
	}
	m, err := GetMode(ModeSlotArgument)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m.ResponseSchema)
	if err != nil {
		t.Fatal(err)
	}
	// Checked in BOTH halves, separately, because the drift was that they
	// disagreed: the schema's `alternatives` description named four filters and
	// the instructions beside it named three. Either one alone would have
	// passed a sweep over the whole text.
	halves := map[string]string{
		"the instructions": flattened(m.Instructions),
		"the schema":       string(raw),
	}
	fields := reflect.TypeOf(DroppedAlternatives{})
	for i := range fields.NumField() {
		name := fields.Field(i).Name
		phrase, known := says[name]
		if !known {
			t.Errorf("DroppedAlternatives grew a %q verdict and this test does "+
				"not know whether the slot argument's prompt tells the model "+
				"about it. Decide, then say so here", name)
			continue
		}
		if phrase == "" {
			continue
		}
		for where, text := range halves {
			if !strings.Contains(text, phrase) {
				t.Errorf("an alternative can be dropped for %s and %s of the "+
					"slot argument never says %q -- the model spends a name on "+
					"a card that was never going to be shown", name, where, phrase)
			}
		}
	}
}

// ADR 41: one surface drafts rationales, on an import, only when somebody
// ticks the box, and every sentence is marked. Two prompts still said a
// rationale is composed nowhere at all.
//
// The mode's existence is the fact being checked against, so this retires
// itself honestly: if a future ADR removes `rationale-draft`, the sentences
// become true again and the test says so instead of failing.
func TestNoPromptSaysARationaleIsComposedNowhere(t *testing.T) {
	t.Parallel()
	if _, err := GetMode(ModeRationaleDraft); err != nil {
		t.Skipf("there is no drafting mode any more (%v), so a prompt saying "+
			"nothing composes a rationale would be telling the truth", err)
	}
	// The two recorded sentences, and a third shape of the same claim.
	stale := []string{
		"refuses to compose it anywhere",
		"records rationales the user wrote",
		"no tool writes one",
	}
	for _, m := range everyMode(t) {
		for _, phrase := range stale {
			if strings.Contains(m.Instructions, phrase) {
				t.Errorf("mode %q's prompt says %q. ADR 41: %s drafts them on "+
					"an import when asked and marks every sentence. Say which "+
					"surface does it and that this is not that surface",
					m.Name, phrase, ModeRationaleDraft)
			}
		}
	}
}

// The fortune-teller's table is not the only room with a corpus of true things
// any more, and the interview's prompt described only that one.
//
// The prefixes `KeepFact` accepts are the source of truth: each is a constant,
// each is handed out by a `frameFor` arm, and a fact citing one the prompt
// never mentioned is discarded with the model none the wiser.
func TestTheFactRuleNamesEveryCorpusTheRuntimeWillAccept(t *testing.T) {
	t.Parallel()
	m, err := GetMode(ModeThemeConversation)
	if err != nil {
		t.Fatal(err)
	}
	text := modelFacingText(t, m)
	for _, prefix := range []string{TarotSource, CauldronSource} {
		if !strings.Contains(text, prefix) {
			t.Errorf("KeepFact accepts a %q id and the theme interview's "+
				"model-facing text never mentions that spelling -- a fact from "+
				"that corpus is cited in the only form the prompt taught, "+
				"which is the wrong one, and thrown away silently", prefix)
		}
	}
	// And the two ways that are not an id, which the same predicate accepts.
	for _, other := range []string{"taxonomy", "URL"} {
		if !strings.Contains(text, other) {
			t.Errorf("the theme interview's model-facing text never mentions "+
				"%q, which KeepFact accepts", other)
		}
	}
}

// What the intake's three modes are told the brief carries, against the brief.
//
// `intakeDeckFacts` is the brief -- the same function all three openings begin
// with -- so it is built here for a real deck and read. It is deliberately
// small: the counts, the curve and the rest of the list are the tools' job and
// its own comment says so. All three prompts claimed otherwise, which is a
// model told it already has facts it will therefore not go and get.
func TestTheIntakePromptsDoNotPromiseFactsTheBriefDoesNotCarry(t *testing.T) {
	t.Parallel()
	// The intake's own fixture, plus the declared themes the brief carries when
	// a deck has any -- so every concept the prompts claim has a fair chance of
	// being found, and an absence is the prompt's fault rather than the deck's.
	d := intakeFixture()
	d.Themes = []string{"food", "aristocrats"}
	brief := intakeDeckFacts(d)

	// One word per concept a prompt might claim, and the marker that says
	// whether the brief really holds it. Read off the brief, not asserted about
	// it: `intakeDeckFacts` changing shape changes these answers.
	concepts := []struct{ word, marker string }{
		{"commander", "Commander:"},
		{"themes", "Themes the owner declared:"},
		{"rationales", "Rationales the owner has already written"},
		// The three the tools carry and the brief never did.
		{"counts", "counts"},
		{"curve", "curve"},
		{"other cards", "Every card"},
	}

	for _, name := range []string{ModeRationaleDraft, ModeIntakeFiling, ModeDeckDescription} {
		m, err := GetMode(name)
		if err != nil {
			t.Fatal(err)
		}
		// Everything this prompt says about its brief: from each "brief gives
		// you" to the end of that sentence.
		for _, claim := range sentencesAfter(m.Instructions, "brief gives you") {
			for _, c := range concepts {
				if !strings.Contains(claim, c.word) {
					continue
				}
				if !strings.Contains(brief, c.marker) {
					t.Errorf("mode %q tells the model its brief gives it %q, in "+
						"%q, and intakeDeckFacts carries no such thing. The "+
						"whole brief is:\n%s", name, c.word, claim, brief)
				}
			}
		}
		// The positive half, and the one that actually costs answer quality: a
		// mode whose brief has no counts must be told where the counts are.
		if !strings.Contains(m.Instructions, "deck_stats") {
			t.Errorf("mode %q's brief carries no counts and its prompt never "+
				"names deck_stats, which has them", name)
		}
	}
}

// sentencesAfter is every clause from an occurrence of `marker` to the next
// full stop, so a claim can be read rather than matched as a whole phrase.
//
// Crude on purpose: the thing being read is prose, and a claim that runs past a
// full stop is a claim this reader will miss. It is here to catch the sentence
// pattern that went wrong three times ("the brief gives you X, Y and Z"), not
// to parse English.
func sentencesAfter(text, marker string) []string {
	out := []string{}
	rest := text
	for {
		at := strings.Index(rest, marker)
		if at < 0 {
			return out
		}
		rest = rest[at+len(marker):]
		end := strings.Index(rest, ".")
		if end < 0 {
			end = len(rest)
		}
		out = append(out, strings.Join(strings.Fields(rest[:end]), " "))
	}
}
