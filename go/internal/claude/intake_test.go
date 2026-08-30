package claude

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The chunking, which decides how much one bad answer costs.
func TestAnIntakeIsChunkedSoOneBadAnswerCostsItsOwnCards(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		in   []string
		size int
		want [][]string
		why  string
	}{
		{
			name: "nothing to do makes no calls", in: nil, size: 3, want: [][]string{},
			why: "a deck whose cards all have rationales must not spend a call saying so",
		},
		{
			name: "an exact multiple does not leave an empty chunk",
			in:   []string{"a", "b", "c", "d"}, size: 2,
			want: [][]string{{"a", "b"}, {"c", "d"}},
			why:  "an empty final chunk is a call with no cards in it",
		},
		{
			name: "a remainder rides in a short chunk",
			in:   []string{"a", "b", "c"}, size: 2,
			want: [][]string{{"a", "b"}, {"c"}},
			why:  "the tail is asked about, not dropped",
		},
		{
			name: "fewer cards than the chunk is one call",
			in:   []string{"a"}, size: 20,
			want: [][]string{{"a"}},
			why:  "the common case for a deck that arrived with its own reasons",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := chunked(tc.in, tc.size); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("chunked(%v, %d) = %v, wanted %v (%s)",
					tc.in, tc.size, got, tc.want, tc.why)
			}
		})
	}
}

// A name that came back is matched against the ask under the fold, and handed
// on in the DECK's spelling.
//
// Both halves matter and they fail differently. A card nobody asked about is
// a write to a card the person did not put in front of the model, and it must
// never happen. A card whose casing came back changed is a write that simply
// misses -- `deckedit` locates by name -- so the answer is silently lost, and
// a silently lost draft is the kind of thing that gets reported as "it only
// did some of them".
func TestAnAnswerIsMatchedToTheAskAndKeepsTheDecksSpelling(t *testing.T) {
	t.Parallel()
	asked := foldedSet([]string{"Sol Ring", "Arcane Signet", "Kongming, \"Sleeping Dragon\""})

	if _, held := asked[Casefold("sol ring")]; !held {
		t.Error("a name that came back lowercased did not match the ask")
	}
	if proper := asked[Casefold("SOL RING")]; proper != "Sol Ring" {
		t.Errorf("matched to %q; the deck's own spelling is what the write looks for", proper)
	}
	if _, held := asked[Casefold("Rhystic Study")]; held {
		t.Error("a card nobody asked about matched the ask, so a model volunteering " +
			"an extra card would have it written into somebody's deck")
	}
	if proper := asked[Casefold("kongming, \"sleeping dragon\"")]; proper == "" {
		t.Error("a card name with quotes in it did not survive the fold")
	}
}

// The facts the opening carries are the deck's identity, and the rationales it
// quotes back are the OWNER's -- never ones a previous intake drafted.
//
// Quoting a draft back as an example of how the owner writes is a model
// learning its own register from itself, which is how a second run drifts
// further from the person than the first one did.
func TestTheOpeningQuotesTheOwnersRationalesAndNotItsOwn(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{
		Slug: "gyome-food", Name: "Gyome — Food", Stage: "draft", Status: "built",
		Commander: []string{"Gyome, Master Chef"},
		Themes:    []string{"food", "aristocrats"},
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Category: "ramp", Why: "the fastest rock there is"},
			{Name: "Cultivate", Category: "ramp", Why: "drafted earlier", WhyBy: "claude"},
			{Name: "Blank", Category: "utility"},
		},
	}
	opening := draftOpening(d, []string{"Blank"})

	for _, want := range []string{"Gyome — Food", "Gyome, Master Chef", "food, aristocrats",
		"the fastest rock there is", "- Blank"} {
		if !strings.Contains(opening, want) {
			t.Errorf("the opening does not carry %q:\n%s", want, opening)
		}
	}
	if strings.Contains(opening, "drafted earlier") {
		t.Error("the opening quotes a rationale a previous intake drafted back as an " +
			"example of how the owner writes, so each run learns its register from " +
			"itself and drifts further from the person")
	}
}

// The dial must not answer `off` for a surface that is about to run.
//
// This is the bug `dialSurfaces` exists for and the third time it has been
// found the same way -- by loading the page, not by reading the code. The
// import screen has no deck by construction, so a default derived from a deck
// is a default derived from nothing, and `off` stands the whole sheet down on
// the one page it belongs to.
func TestTheIntakeSurfaceDoesNotAnswerOffWithNoDeck(t *testing.T) {
	t.Parallel()
	got, err := IntakeStanceFor(nil, nil)
	if err != nil {
		t.Fatalf("the intake surface has no default: %v", err)
	}
	if !got.AllowsCalls() {
		t.Errorf("the intake's own default is %+v, which makes no call -- so the "+
			"sheet stands down for everybody on the import screen", got)
	}
	// And it does NOT come with a write: ADR 41's second gate is a decision
	// the user makes on the stance dial, and a surface default that handed it
	// over would be the gate opening itself.
	if got.MayWrite() {
		t.Errorf("the intake's default may write (%+v), so ADR 41's second gate "+
			"is satisfied by the surface rather than by the user", got)
	}
	// A stance the caller asked for still wins, clamped.
	asked, err := IntakeStanceFor("off", nil)
	if err != nil {
		t.Fatalf("an explicit stance was refused: %v", err)
	}
	if asked.AllowsCalls() {
		t.Errorf("an explicit `off` was overridden by the surface default: %+v", asked)
	}
}

// ---- the three surfaces, against a scripted endpoint -----------------------

// A deck small enough to reason about, with one card already reasoned about.
func intakeFixture() *deck.Deck {
	return &deck.Deck{
		Slug: "gyome-food", Name: "Gyome — Food", Stage: "draft", Status: "built",
		Commander: []string{"Gyome, Master Chef"},
		Cards: []deck.CardEntry{
			{Name: "Sol Ring", Category: "ramp", Why: "the fastest rock there is"},
			{Name: "Cultivate", Category: "utility"},
			{Name: "Beast Within", Category: "utility"},
		},
	}
}

func TestDraftingReturnsOnlyCardsThatWereAskedAbout(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"drafts":[` +
			`{"card":"cultivate","why":"Ramp and fixing in one card.","fact":"Search for a basic land."},` +
			`{"card":"Beast Within","why":"Answers anything at all.","fact":"Destroy target permanent."},` +
			`{"card":"Rhystic Study","why":"Nobody asked about this one.","fact":"invented"}` +
			`]}`)}.json()}}
	ep := api.start(t)

	drafts, outcome, err := DraftRationales(context.Background(), intakeFixture(),
		[]string{"Cultivate", "Beast Within"},
		IntakeRequest{Endpoint: ep, Requested: "collaborator"})
	if err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if !outcome.Asked {
		t.Fatalf("no call was made: %+v", outcome)
	}
	if len(drafts) != 2 {
		t.Fatalf("got %d drafts, wanted 2: %+v", len(drafts), drafts)
	}
	// The deck's own spelling, not the model's: `deckedit` locates by name, so
	// a lowercased answer that is handed on verbatim is a write that misses.
	if drafts[0].Card != "Cultivate" {
		t.Errorf("a lowercased name was not matched back to the deck's: %q", drafts[0].Card)
	}
	if drafts[0].Why != "Ramp and fixing in one card." {
		t.Errorf("the rationale was altered on the way past: %q", drafts[0].Why)
	}
	// The card nobody asked about is dropped and counted, never written.
	for _, d := range drafts {
		if d.Card == "Rhystic Study" {
			t.Error("a card nobody asked about survived, so a volunteered extra " +
				"would be written into somebody's deck")
		}
	}
	if outcome.Skipped != 1 {
		t.Errorf("skipped %d, wanted 1 -- the count is how somebody sees this "+
			"climbing", outcome.Skipped)
	}
	// Both cards were answered, so nothing was left. A reconciliation that
	// invents a complaint about a complete run is worse than none.
	if len(outcome.Unanswered) != 0 {
		t.Errorf("a complete run left %q behind", outcome.Unanswered)
	}
}

// **The card that never came back is named, and the volunteered one is not.**
//
// `Skipped` counts rows that arrived and were *rejected*; a card the model
// simply never mentioned arrives as nothing and used to be counted nowhere.
// So a run asked for two and given one wrote one, reported "skipped: 1" about
// an entirely different card, and named the missing one to nobody -- which
// left diffing your own deck against your own paste as the only way to find
// it. Found doing exactly that on a real import, 2026-08-30.
func TestTheDraftingReportsTheCardThatNeverCameBack(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"drafts":[` +
			// Lowercased on purpose: an answer is an answer whatever case it
			// arrives in, and matching on anything but the casefold would
			// report a card that was drafted perfectly well as missing.
			`{"card":"beast within","why":"Answers anything at all.","fact":"Destroy target permanent."},` +
			`{"card":"Rhystic Study","why":"Nobody asked about this one.","fact":"invented"}` +
			`]}`)}.json()}}

	_, outcome, err := DraftRationales(context.Background(), intakeFixture(),
		[]string{"Cultivate", "Beast Within"},
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if len(outcome.Unanswered) != 1 || outcome.Unanswered[0] != "Cultivate" {
		t.Fatalf("unanswered %q, want [Cultivate]: `Beast Within` was answered "+
			"in lower case and `Rhystic Study` was never asked about -- the "+
			"second is a rejection, which is what Skipped is for",
			outcome.Unanswered)
	}
	if outcome.Skipped != 1 {
		t.Errorf("skipped %d, want 1 -- the volunteered card is the rejected "+
			"one, and it must not appear in both counts", outcome.Skipped)
	}
}

// The list is put in front of a person, so it is walked in the order the cards
// were asked in. An order nobody chose reads as a fault of its own -- and a
// map's, which is what this would have been, changes between runs.
func TestTheUnansweredAreNamedInTheOrderTheyWereAskedIn(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"drafts":[{"card":"Cultivate","why":"Ramp and fixing.","fact":"a fact"}]}`)}.json()}}

	_, outcome, err := DraftRationales(context.Background(), intakeFixture(),
		[]string{"Sol Ring", "Cultivate", "Beast Within"},
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("drafting: %v", err)
	}
	want := []string{"Sol Ring", "Beast Within"}
	if !slices.Equal(outcome.Unanswered, want) {
		t.Errorf("unanswered %q, want %q", outcome.Unanswered, want)
	}
}

// The filing pass has the same hole and the same fix: a card `FileCards` never
// placed keeps the category it arrived under, and nothing said which card that
// was.
func TestTheFilingReportsTheCardItNeverPlaced(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"filings":[{"card":"Cultivate","category":"ramp","fact":"Search for a basic land."}]}`)}.json()}}

	_, outcome, err := FileCards(context.Background(), intakeFixture(),
		[]string{"Cultivate", "Beast Within"},
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("filing: %v", err)
	}
	if len(outcome.Unanswered) != 1 || outcome.Unanswered[0] != "Beast Within" {
		t.Errorf("unanswered %q, want [Beast Within]", outcome.Unanswered)
	}
}

// A draft with no text is not a draft. It would be written as a marked blank,
// which claims a model wrote nothing and is worse than an unmarked one.
func TestAnEmptyDraftIsNotCarried(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"drafts":[{"card":"Cultivate","why":"   ","fact":"nothing"}]}`)}.json()}}
	drafts, outcome, err := DraftRationales(context.Background(), intakeFixture(),
		[]string{"Cultivate"}, IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if len(drafts) != 0 {
		t.Errorf("an empty rationale was carried: %+v", drafts)
	}
	if outcome.Skipped != 1 {
		t.Errorf("skipped %d, wanted 1", outcome.Skipped)
	}
}

// **A chunk that does not parse costs its own cards and not the deck.** This
// is the reason for chunking at all, and it is the branch a single call could
// not have.
func TestAnUnreadableChunkCostsItsOwnCardsAndNoOthers(t *testing.T) {
	t.Parallel()
	names := make([]string, 0, IntakeChunk+2)
	for i := range IntakeChunk + 2 {
		names = append(names, fmt.Sprintf("Card %d", i))
	}
	good := `{"drafts":[{"card":"Card 20","why":"A reason.","fact":"a fact"},` +
		`{"card":"Card 21","why":"Another reason.","fact":"a fact"}]}`
	api := &scriptedAPI{replies: []string{
		// The first chunk is truncated JSON; the second is fine.
		reply{stop: "max_tokens", content: textBlock(`{"drafts":[{"card":"Card 0"`)}.json(),
		reply{stop: "end_turn", content: textBlock(good)}.json(),
	}}
	drafts, outcome, err := DraftRationales(context.Background(), intakeFixture(), names,
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if api.served != 2 {
		t.Errorf("made %d calls for %d cards; %d to a chunk should be two",
			api.served, len(names), IntakeChunk)
	}
	if len(drafts) != 2 {
		t.Fatalf("got %d drafts; the readable chunk's cards should survive the "+
			"unreadable one: %+v", len(drafts), drafts)
	}
	if !outcome.Asked {
		t.Error("the outcome says nothing was asked, though two calls were made")
	}
}

func TestFilingReturnsTheCategoriesItWasGiven(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"filings":[{"card":"Cultivate","category":"ramp","fact":"Search for a basic land."},` +
			`{"card":"Beast Within","category":"interaction","fact":"Destroy target permanent."}]}`)}.json()}}
	filings, outcome, err := FileCards(context.Background(), intakeFixture(),
		[]string{"Cultivate", "Beast Within"},
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("filing: %v", err)
	}
	if len(filings) != 2 || filings[0].Category != "ramp" ||
		filings[1].Category != "interaction" {
		t.Fatalf("the filings did not come back as given: %+v (%+v)", filings, outcome)
	}
}

func TestDescribingADeckReturnsAStrategyAndItsThemes(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"strategy":"Cook Food, sacrifice it, drain the table. Slow to start.",` +
			`"themes":["food","aristocrats"],"fact":"Gyome makes Food on every death."}`)}.json()}}
	got, outcome, err := DescribeDeck(context.Background(), intakeFixture(),
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("describing: %v", err)
	}
	if !outcome.Asked || got.Strategy == "" {
		t.Fatalf("no description came back: %+v (%+v)", got, outcome)
	}
	if len(got.Themes) != 2 || got.Themes[0] != "food" {
		t.Errorf("the themes did not survive: %+v", got.Themes)
	}
	// One call for the whole deck, never chunked: a description in twenty-card
	// pieces is five paragraphs about five fifths of a deck.
	if api.served != 1 {
		t.Errorf("made %d calls; a description is asked once", api.served)
	}
}

// A description that came back empty leaves the deck's own alone rather than
// writing a blank strategy over it.
func TestAnEmptyDescriptionChangesNothing(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"strategy":"  ","themes":[],"fact":"nothing"}`)}.json()}}
	got, outcome, err := DescribeDeck(context.Background(), intakeFixture(),
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("describing: %v", err)
	}
	if got.Strategy != "" {
		t.Errorf("an empty strategy was carried: %q", got.Strategy)
	}
	if outcome.Reason == "" {
		t.Error("nothing happened and the outcome does not say why")
	}
}

// At a stance that makes no calls, all three answer without reaching the
// network at all -- and say so, rather than returning an empty list that looks
// like they had nothing to offer.
func TestTheIntakeSurfacesMakeNoCallAtAStanceThatIsOff(t *testing.T) {
	t.Parallel()
	// No replies scripted: any request at all would fail the test.
	api := &scriptedAPI{}
	ep := api.start(t)
	d := intakeFixture()
	req := IntakeRequest{Endpoint: ep, Requested: "off"}

	drafts, outcome, err := DraftRationales(context.Background(), d, []string{"Cultivate"}, req)
	if err != nil || len(drafts) != 0 || outcome.Asked || outcome.Reason == "" {
		t.Errorf("drafting at `off`: %+v %+v %v", drafts, outcome, err)
	}
	filings, outcome, err := FileCards(context.Background(), d, []string{"Cultivate"}, req)
	if err != nil || len(filings) != 0 || outcome.Asked || outcome.Reason == "" {
		t.Errorf("filing at `off`: %+v %+v %v", filings, outcome, err)
	}
	described, outcome, err := DescribeDeck(context.Background(), d, req)
	if err != nil || described.Strategy != "" || outcome.Asked || outcome.Reason == "" {
		t.Errorf("describing at `off`: %+v %+v %v", described, outcome, err)
	}
	if api.served != 0 {
		t.Errorf("%d calls were made at a stance that makes none", api.served)
	}
}

// The filing and description openings carry the deck, and the filing one says
// the lands are already done -- a model that files thirty-six Forests is a
// model that spent its chunk on the one thing the pool already decided.
func TestTheFilingAndDescriptionOpeningsSayWhatTheyNeedTo(t *testing.T) {
	t.Parallel()
	d := intakeFixture()
	filing := filingOpening(d, []string{"Cultivate"})
	if !strings.Contains(filing, "Lands are already filed") {
		t.Errorf("the filing opening does not say the lands are done:\n%s", filing)
	}
	if !strings.Contains(filing, "- Cultivate") {
		t.Errorf("the filing opening does not carry its cards:\n%s", filing)
	}
	describe := describeOpening(d)
	if !strings.Contains(describe, "Gyome, Master Chef") {
		t.Errorf("the description opening does not name the commander:\n%s", describe)
	}
	if !strings.Contains(describe, "bad at") {
		t.Errorf("the description opening does not ask for the honest clause:\n%s", describe)
	}
}
