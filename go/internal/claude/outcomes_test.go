package claude

import (
	"context"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The outcomes a conversation can have that are neither a good answer nor a
// fault: the model declined, the answer did not parse, the answer named
// things nobody asked about.
//
// Every mode has a branch for each, every branch returns a *report* rather
// than an error, and that is the decision worth holding: a refusal that came
// back as a 500 would take the diagnosis away from the person exactly when
// they want it (commandment 2), while a refusal that came back as an empty
// result would read as "there is nothing to say about this card", which is
// the opposite of what happened.

// A card the model files under a category it was never asked about is
// skipped and counted, and the same for one filed under no category at all.
//
// The deck's own spelling wins over the model's, which is why the match is
// case-folded and the answer is rewritten rather than trusted: a filing under
// "cultivate" must land on the deck's `Cultivate`, and a filing for a card
// that is not in the ask is a card the model invented.
func TestAFilingForACardNobodyAskedAboutIsSkippedAndCounted(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"filings":[` +
			`{"card":"cultivate","category":"ramp","fact":"Search for a basic land."},` +
			`{"card":"Rhystic Study","category":"draw","fact":"nobody asked"},` +
			`{"card":"Beast Within","category":"   ","fact":"filed under nothing"}` +
			`]}`)}.json()}}

	filings, outcome, err := FileCards(context.Background(), intakeFixture(),
		[]string{"Cultivate", "Beast Within"},
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("filing: %v", err)
	}
	if len(filings) != 1 || filings[0].Card != "Cultivate" {
		t.Fatalf("the filings are %+v, want only the deck's own Cultivate", filings)
	}
	if outcome.Skipped != 2 {
		t.Errorf("skipped %d, want the invented card and the blank category",
			outcome.Skipped)
	}
	if len(outcome.Unanswered) != 1 || outcome.Unanswered[0] != "Beast Within" {
		t.Errorf("unanswered is %q; a card filed under nothing was never filed",
			outcome.Unanswered)
	}
}

// A refusal costs its own chunk and no others.
//
// The chunk's cards keep what they arrived with -- a blank rationale, a
// `utility` filing -- which is exactly where the deck already was, so a lost
// chunk costs nothing that was not already owed. Failing the whole intake
// instead would throw away every chunk that did come back, which on a
// ninety-nine-card import is most of the work and all of the money.
func TestAModelThatDeclinesCostsItsOwnChunkAndNoOthers(t *testing.T) {
	t.Parallel()
	api := &scriptedAPI{replies: []string{
		reply{stop: "refusal", content: textBlock("")}.json(),
	}}

	filings, outcome, err := FileCards(context.Background(), intakeFixture(),
		[]string{"Cultivate", "Beast Within"},
		IntakeRequest{Endpoint: api.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("a refusal became an error: %v", err)
	}
	if len(filings) != 0 {
		t.Errorf("a refused chunk produced filings: %+v", filings)
	}
	if !outcome.Asked {
		t.Error("the outcome says nothing was asked, and a call was made and paid for")
	}
	if len(outcome.Unanswered) != 2 {
		t.Errorf("unanswered is %q, want both cards -- a refused chunk leaves "+
			"its cards exactly where they were", outcome.Unanswered)
	}
	// And the same refusal through the drafting pass, which is the one ADR 41
	// gates: nothing drafted is better than a blank marked `why_by: claude`.
	api2 := &scriptedAPI{replies: []string{
		reply{stop: "refusal", content: textBlock("")}.json(),
	}}
	drafts, _, err := DraftRationales(context.Background(), intakeFixture(),
		[]string{"Cultivate"}, IntakeRequest{Endpoint: api2.start(t), Requested: "collaborator"})
	if err != nil {
		t.Fatalf("a refused draft became an error: %v", err)
	}
	if len(drafts) != 0 {
		t.Errorf("a refused chunk produced drafts: %+v", drafts)
	}
}

// The owner's own rationales ride in the opening so the model can hear how
// they write -- and they are capped, because a chunk that spends its context
// on sixty of somebody else's sentences has less room for the twenty it was
// asked about.
//
// Only the owner's: a rationale marked `why_by: claude` is the model's own
// writing, and quoting it back as the register to match is how a house style
// becomes a feedback loop.
func TestTheOpeningQuotesAtMostSixOfTheOwnersRationales(t *testing.T) {
	t.Parallel()
	d := &deck.Deck{
		Slug: "gyome-food", Name: "Gyome — Food", Stage: "draft", Status: "built",
		Commander: []string{"Gyome, Master Chef"},
		Themes:    []string{"food", "aristocrats"},
	}
	for i := 0; i < 12; i++ {
		d.Cards = append(d.Cards, deck.CardEntry{
			Name: "Owner Card " + string(rune('A'+i)), Category: "ramp",
			Why: "the owner wrote this one " + string(rune('A'+i)),
		})
	}
	d.Cards = append(d.Cards, deck.CardEntry{
		Name: "Drafted Card", Category: "ramp",
		Why: "claude wrote this one", WhyBy: "claude",
	})

	facts := intakeDeckFacts(d)
	quoted := strings.Count(facts, "the owner wrote this one ")
	if quoted != 6 {
		t.Errorf("%d of the owner's rationales reached the prompt, want the cap of 6", quoted)
	}
	if strings.Contains(facts, "claude wrote this one") {
		t.Error("a rationale Claude drafted was quoted back as the register to " +
			"match; that is a house style eating itself")
	}
	if !strings.Contains(facts, "food, aristocrats") {
		t.Error("the owner's declared themes did not reach the opening")
	}
}

// The slot argument's three not-an-answer outcomes, and the one shape they
// share: a report with `asked: true`, a reason a person can read, and empty
// lists rather than null ones.
//
// ADR 25 is why the empty lists matter more here than elsewhere. This mode
// has no field for a defence, so its whole output is charges and
// alternatives; a null list where an empty one belongs is the client
// rendering nothing at all with no explanation beside it.
func TestTheSlotArgumentsNotAnAnswerOutcomesStillReportThemselves(t *testing.T) {
	t.Parallel()
	d := fixtureDeck(t, "mono-green")

	for _, tc := range []struct {
		note, body, stop, wantIn string
	}{
		{"the model declined", "", "refusal", "declined"},
		{"the answer did not parse", "not JSON at all", "end_turn", "did not parse"},
	} {
		api := &scriptedAPI{replies: []string{
			reply{stop: tc.stop, in: 500, out: 10, content: textBlock(tc.body)}.json(),
		}}
		ep := api.start(t)
		withPool(t, func(c *pool.Conn) {
			report, err := Argue(context.Background(), c, d, "Sol Ring",
				ArgueRequest{Endpoint: ep, Requested: "second-opinion"})
			if err != nil {
				t.Fatalf("%s: %v", tc.note, err)
			}
			if asked, _ := kv(report, "asked").(bool); !asked {
				t.Errorf("%s: the report says nothing was asked, and a call was paid for", tc.note)
			}
			reason, _ := kv(report, "reason").(string)
			if !strings.Contains(reason, tc.wantIn) {
				t.Errorf("%s: the reason reads %q, want one saying %q", tc.note, reason, tc.wantIn)
			}
			charges, ok := kv(report, "charges").([]Charge)
			if !ok || charges == nil {
				t.Errorf("%s: charges is %#v, want an empty list -- the client "+
					"iterates it", tc.note, kv(report, "charges"))
			} else if len(charges) != 0 {
				t.Errorf("%s: %d charges came back with no readable answer", tc.note, len(charges))
			}
		})
	}
}

// What the person is weighing, in their own words, reaches the model --
// because the whole point of the focus box is that the argument is about the
// slot *they* are worried about rather than the one the model finds easiest
// to attack.
func TestTheFocusTheOwnerTypedReachesTheOpening(t *testing.T) {
	t.Parallel()
	const focus = "I keep drawing it on turn eight with nothing to ramp into"
	api := &scriptedAPI{replies: []string{reply{stop: "end_turn", content: textBlock(
		`{"charges":[],"alternatives":[]}`)}.json()}}
	ep := api.start(t)
	d := fixtureDeck(t, "mono-green")
	withPool(t, func(c *pool.Conn) {
		if _, err := Argue(context.Background(), c, d, "Sol Ring", ArgueRequest{
			Endpoint: ep, Requested: "second-opinion", Focus: focus}); err != nil {
			t.Fatalf("argue: %v", err)
		}
	})
	if len(api.raw) == 0 {
		t.Fatal("no call was made at all")
	}
	if !strings.Contains(api.raw[0], "What I am weighing, in my words") {
		t.Error("the focus was dropped on its way to the model")
	}
	if !strings.Contains(api.raw[0], "turn eight with nothing to ramp into") {
		t.Error("the owner's own words did not reach the opening")
	}
}

// The repeated-fact backstop compares the SHORTER list of content words
// against the longer one, in whichever order the two arrive -- so a fact
// reworded into half as many words is still the same fact, and a fact
// expanded into twice as many is too.
//
// It matters because the facts ride in the report rather than in the
// transcript, so the model cannot see what it has already said: this is the
// only thing between a querent and hearing the same sentence three ways.
func TestARewordedFactIsCaughtInEitherDirection(t *testing.T) {
	t.Parallel()
	const long = "Gyome makes Food tokens when your creatures die, each and every turn"
	const short = "Gyome makes Food when creatures die"

	if !Repeats(short, []string{long}) {
		t.Error("a fact shortened by half was not recognised as one already given")
	}
	if !Repeats(long, []string{short}) {
		t.Error("a fact expanded was not recognised as one already given")
	}
	if !Repeats(long, []string{long}) {
		t.Error("a fact repeated verbatim was not recognised")
	}
	// A fact made entirely of words the comparison ignores has nothing to
	// compare, so it is not a repeat of anything -- which is the right answer:
	// dropping it would silence a fact for being short.
	if Repeats(long, []string{"and the of a to"}) {
		t.Error("a fact with no content words at all was read as a repeat")
	}
	if Repeats("", []string{long}) {
		t.Error("an empty fact was read as a repeat")
	}
	if Repeats("Gyome costs five mana", []string{long}) {
		t.Error("a different fact was read as a repeat")
	}
}

// A proposal's combinations are dropped rather than repaired, and the three
// reasons are counted separately because they mean different things: an item
// that is not an object at all is the model's output being malformed, a key
// that is not one of the 32 is a combination nothing can render, and a
// combination whose every legend went missing is a colour name with a
// paragraph under it.
func TestACombinationNothingCanRenderIsDroppedRatherThanRepaired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	allowed := map[string]bool{"s1": true}
	withPool(t, func(c *pool.Conn) {
		got, dropped, lost, err := resolveCombinations(ctx, c, []any{
			// Not an object: not a combination, and not counted as one either.
			"WG",
			// A key that is not one of the 32.
			map[string]any{"key": "WXYZ", "commanders": []any{
				map[string]any{"card": "Gyome, Master Chef"}}},
			// A real key whose commanders are all invented.
			map[string]any{"key": "G", "commanders": []any{
				map[string]any{"card": "Invented Legend"}}},
			// A real key with no commanders named at all.
			map[string]any{"key": "W", "commanders": []any{}},
		}, allowed)
		if err != nil {
			t.Fatalf("resolving: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("%d combination(s) survived, want none", len(got))
		}
		if lost != 3 {
			t.Errorf("lost %d, want the unknown key and the two with no "+
				"confirmed commander", lost)
		}
		if dropped != 1 {
			t.Errorf("dropped %d commanders, want the one invented legend", dropped)
		}
		// A list that is not a list at all is no combinations rather than a
		// panic: this is the model's output, decoded.
		got, _, _, err = resolveCombinations(ctx, c, "not a list", allowed)
		if err != nil || len(got) != 0 {
			t.Errorf("a non-list gave %#v / %v", got, err)
		}
	})
}

// Every deckless surface the dial can be asked about answers its OWN default
// rather than `off`, and that includes the two that were added after the
// dial's table was written.
//
// `Resolve(nil, nil)` is `off` -- correct for "I have no idea what this is
// about" and wrong for a screen whose only control is a question box, a
// button over a photograph, or an import sheet. The intake's absence from
// this list is the bug ADR 41 shipped and the reason the table is asserted
// rather than read.
func TestEveryDecklessSurfaceAnswersItsOwnDefaultRatherThanOff(t *testing.T) {
	t.Parallel()
	for _, surface := range []string{"research", "scan", "intake", "theme"} {
		got, err := surfaceStanceFor(surface, nil, nil)
		if err != nil {
			t.Errorf("%s: %v", surface, err)
			continue
		}
		if !got.AllowsCalls() {
			t.Errorf("%s answers %+v with no deck; that surface would stand "+
				"down on the one page it belongs to", surface, got)
		}
		if got.MayWrite() {
			t.Errorf("%s answers %+v, which may write -- no deckless default "+
				"reaches the write axis", surface, got)
		}
		// And the dial publishes the same answer, which is the half that
		// renders beside the control.
		if shown := DialDefault(nil, surface, nil); shown != got {
			t.Errorf("%s: the dial shows %+v where the surface uses %+v",
				surface, shown, got)
		}
	}
	// A surface nobody named is dispatched to the theme's answer rather than
	// to a fifth copy of one, and `dialSurfaces` is what stops the dial from
	// publishing that answer for a name it does not know: the switch is a
	// fallback, the set is the gate, and only the set decides what the dial
	// says out loud.
	got, err := surfaceStanceFor("invented-surface", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	theme, err := ThemeStanceFor(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != theme {
		t.Errorf("a surface nobody named answered %+v, not the theme's %+v", got, theme)
	}
	if shown := DialDefault(nil, "invented-surface", nil); shown != Off {
		t.Errorf("the dial published %+v for a surface it does not know; only "+
			"a name in dialSurfaces gets that surface's own answer, and with "+
			"no deck either the rest is off", shown)
	}
}

// kvOrder keeps the one thing a `wire.OrderedMap` exists for honest in this
// file's assertions: reading a key out of a report is a scan, not a map
// lookup, and a report that lost its order would still answer these.
var _ = wire.OrderedMap{}
