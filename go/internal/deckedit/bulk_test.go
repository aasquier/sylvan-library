package deckedit

import (
	"strings"
	"testing"
)

// The bulk edit's diff, which is the half of the operation that decides
// anything. The fold underneath it is the operations this package already
// verifies one at a time, so these tests are about the *plan*: who is added,
// whose reason changed, who goes to the graveyard, who is left where they are,
// and every one of the ways a paste can be refused before a line is written.
//
// The deck below is a curated one on purpose. Curated is the strict case --
// rule 4 applies at full strength -- and `draftFixture` covers the other side
// where it matters.
const bulkFixture = `slug: goreclaw
name: Goreclaw
status: theoretical
stage: curated
commander:
  - Goreclaw, Terror of Qal Sisma
cards:
  - name: Sol Ring
    category: ramp
    why: Two mana on turn one, and it always has been.
  - name: Cultivate
    category: ramp
    why: Fixes and ramps in one card.
  - name: Craterhoof Behemoth
    category: threat
    why: The game ends the turn this resolves.
  - name: Forest
    category: land
    why: Basic, untapped, and the only green source this list needs.
    qty: 36
swap_board:
  - name: Regal Behemoth
    category: threat
    why: Cut for speed; still tempting.
`

func plan(t *testing.T, text string, wanted ...BulkCard) *BulkPlan {
	t.Helper()
	p, err := PlanBulk(text, wanted)
	if err != nil {
		t.Fatalf("the plan was refused: %v", err)
	}
	return p
}

func names(cards []BulkAdd) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.Name)
	}
	return out
}

// The whole deck pasted back exactly as it stands. Nothing moves, and the
// plan says so rather than finding four differences in a file it just read.
func TestAPasteThatMatchesTheDeckChangesNothing(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if p.Touches() {
		t.Fatalf("a matching paste planned work: +%d ~%d #%d †%d",
			len(p.Add), len(p.Rewrite), len(p.Requantify), len(p.Entomb))
	}
	if len(p.Unchanged) != 4 {
		t.Errorf("%d cards reported unchanged, expected 4: %v", len(p.Unchanged), p.Unchanged)
	}
	if _, err := ApplyBulk(bulkFixture, p); err == nil {
		t.Error("a plan that does nothing was applied; the write, the log entry " +
			"and the gate run would all be about an edit nobody made")
	}
}

func TestAPasteAddsWhatTheDeckDoesNotHave(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36},
		BulkCard{Name: "Llanowar Reborn", Qty: 1, Category: "land",
			Why: "A land that grows a counter."})
	if got := names(p.Add); len(got) != 1 || got[0] != "Llanowar Reborn" {
		t.Fatalf("the additions were %v", got)
	}
	if len(p.Entomb) != 0 {
		t.Errorf("an addition buried something: %v", p.Entomb)
	}

	out, err := ApplyBulk(bulkFixture, p)
	if err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
	if !strings.Contains(out, "name: Llanowar Reborn") {
		t.Errorf("the card did not land:\n%s", out)
	}
	if !strings.Contains(out, "why: A land that grows a counter.") {
		t.Errorf("the person's own reason did not land:\n%s", out)
	}
}

// The feature Aaron asked for by name: the descriptions, in bulk, in the
// player's own words.
func TestAQuotedReasonRewritesTheOneThatIsThere(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1, Why: "Still the best card ever printed."},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if len(p.Rewrite) != 1 {
		t.Fatalf("%d rewrites planned: %+v", len(p.Rewrite), p.Rewrite)
	}
	// The old sentence rides along, because a confirmation that does not show
	// what is being replaced is a warning rather than a decision.
	if p.Rewrite[0].Was != "Two mana on turn one, and it always has been." {
		t.Errorf("the outgoing sentence was reported as %q", p.Rewrite[0].Was)
	}
	if p.Rewrite[0].Why != "Still the best card ever printed." {
		t.Errorf("the incoming sentence was reported as %q", p.Rewrite[0].Why)
	}

	out, err := ApplyBulk(bulkFixture, p)
	if err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
	if !strings.Contains(out, "why: Still the best card ever printed.") {
		t.Errorf("the new reason did not land:\n%s", out)
	}
	if strings.Contains(out, "Two mana on turn one") {
		t.Errorf("the old reason survived under the new one:\n%s", out)
	}
	// Every other card's own words are untouched, which is the property the
	// whole package exists for.
	if !strings.Contains(out, "why: Fixes and ramps in one card.") {
		t.Errorf("a neighbour's reason was damaged:\n%s", out)
	}
}

// The rule that makes a plain Moxfield export safe to paste: a line with no
// quoted column is not a claim that the card has no reason.
func TestALineWithNoReasonLeavesTheReasonAlone(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if len(p.Rewrite) != 0 {
		t.Fatalf("an unquoted line was read as a blanking: %+v", p.Rewrite)
	}
}

// ADR 41's mark, through the door every `why` goes through. The plan says
// which sentences were drafted so a person can see which ones are theirs.
func TestRewritingADraftedReasonTakesTheMarkOff(t *testing.T) {
	t.Parallel()
	drafted := strings.Replace(bulkFixture,
		"    why: Two mana on turn one, and it always has been.\n",
		"    why: Two mana on turn one, and it always has been.\n    why_by: claude\n", 1)

	p := plan(t, drafted,
		BulkCard{Name: "Sol Ring", Qty: 1, Why: "My words now."},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if len(p.Rewrite) != 1 || !p.Rewrite[0].WasDrafted {
		t.Fatalf("the plan did not report the sentence as drafted: %+v", p.Rewrite)
	}

	out, err := ApplyBulk(drafted, p)
	if err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
	if strings.Contains(out, "why_by: claude") {
		t.Errorf("a person's sentence is still marked as drafted, which is a lie "+
			"in the one file that is supposed to be the truth:\n%s", out)
	}
}

func TestAQuantityMovesWithoutTouchingTheReason(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 34})
	if len(p.Requantify) != 1 || p.Requantify[0].Was != 36 || p.Requantify[0].Qty != 34 {
		t.Fatalf("the quantity change was read as %+v", p.Requantify)
	}
	out, err := ApplyBulk(bulkFixture, p)
	if err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
	if !strings.Contains(out, "qty: 34") {
		t.Errorf("the quantity did not land:\n%s", out)
	}
	if !strings.Contains(out, "why: Basic, untapped, and the only green source this list needs.") {
		t.Errorf("the reason was damaged by a quantity change:\n%s", out)
	}
}

// Aaron's ruling, 2026-08-29: a card the paste does not name is ENTOMBED. It
// keeps its `why` and it comes back from the deck page.
func TestACardTheListDoesNotNameIsEntombedAndKeepsItsReason(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if len(p.Entomb) != 1 || p.Entomb[0] != "Cultivate" {
		t.Fatalf("the burials were %v", p.Entomb)
	}

	out, err := ApplyBulk(bulkFixture, p)
	if err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
	graveyard := out[strings.Index(out, "graveyard:"):]
	if !strings.Contains(graveyard, "name: Cultivate") {
		t.Fatalf("the card did not reach the graveyard:\n%s", out)
	}
	if !strings.Contains(graveyard, "why: Fixes and ramps in one card.") {
		t.Errorf("the card was buried without the reason it was played for, so "+
			"bringing it back would cost somebody their own words:\n%s", out)
	}
	// And it really left the 99, rather than being copied into both.
	if strings.Count(out, "name: Cultivate") != 1 {
		t.Errorf("Cultivate is in two places at once:\n%s", out)
	}
}

// The Moxfield case, and the one that would otherwise be a silent skip: an
// export carries the commander under `SIDEBOARD:`, so a pasted list names a
// card that must never be in the 99.
func TestTheCommanderAndTheBoardAreLeftWhereTheyAre(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Goreclaw, Terror of Qal Sisma", Qty: 1},
		BulkCard{Name: "Regal Behemoth", Qty: 1},
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if len(p.Left) != 2 {
		t.Fatalf("%d lines were left, expected the commander and the board card: %+v",
			len(p.Left), p.Left)
	}
	if len(p.Add) != 0 {
		t.Errorf("a card outside the 99 was planned into it: %v", names(p.Add))
	}
	if p.Touches() {
		t.Errorf("the edit found work to do in a list that matches the deck")
	}
	for _, left := range p.Left {
		if !strings.Contains(left.Reason, "99") {
			t.Errorf("%s was left with the reason %q, which does not say where it is",
				left.Name, left.Reason)
		}
	}
}

func TestACardInTheGraveyardIsLeftThere(t *testing.T) {
	t.Parallel()
	buried, err := EntombCard(bulkFixture, "Cultivate")
	if err != nil {
		t.Fatal(err)
	}
	p := plan(t, buried,
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if len(p.Left) != 1 || p.Left[0].Name != "Cultivate" {
		t.Fatalf("the graveyard card was handled as %+v", p.Left)
	}
	if !strings.Contains(p.Left[0].Reason, "graveyard") {
		t.Errorf("the reason given was %q", p.Left[0].Reason)
	}
	// AddCard refuses a card that is in the graveyard, so a plan that tried
	// would refuse mid-fold instead of reporting.
	if len(p.Add) != 0 {
		t.Errorf("a buried card was planned back into the 99: %v", names(p.Add))
	}
}

// Rule 4 at the moment a card enters a curated deck, predicted rather than
// discovered halfway through the fold.
func TestACuratedDeckRefusesANewCardWithNoReason(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36},
		BulkCard{Name: "Terastodon", Qty: 1, Category: "threat"})
	if len(p.Blocked) != 1 || p.Blocked[0].Name != "Terastodon" {
		t.Fatalf("the plan did not block the unreasoned card: %+v", p.Blocked)
	}
	if !strings.Contains(p.Blocked[0].Reason, "quotes") {
		t.Errorf("the refusal does not say how to fix it: %q", p.Blocked[0].Reason)
	}
	// All or nothing. Half the paste applied would leave a deck a card short
	// of what somebody pasted, which is a state nobody chose.
	if _, err := ApplyBulk(bulkFixture, p); err == nil {
		t.Fatal("a blocked plan was applied")
	}
}

// The other side of the same rule: a draft owes its reasons rather than
// requiring them (ADR 13), so a bare list lands and the gate counts what is
// still outstanding.
func TestADraftTakesACardWithNoReasonYet(t *testing.T) {
	t.Parallel()
	draft := strings.Replace(bulkFixture, "stage: curated", "stage: draft", 1)
	p := plan(t, draft,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36},
		BulkCard{Name: "Terastodon", Qty: 1, Category: "threat"})
	if len(p.Blocked) != 0 {
		t.Fatalf("a draft blocked a card it only owes a reason for: %+v", p.Blocked)
	}
	if !p.Draft {
		t.Error("the plan does not say the deck is a draft, so the surface cannot")
	}
	if _, err := ApplyBulk(draft, p); err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
}

// Two lines, one card. `deckimport` decided this and argued it: quantities
// add, and the first reason wins because choosing between two would be
// composing one.
func TestRepeatedLinesAreAddedTogetherAndSaidSo(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 30, Why: "the first reason"},
		BulkCard{Name: "forest", Qty: 6, Why: "the second reason"})
	if len(p.Merged) != 1 || p.Merged[0] != "Forest" {
		t.Fatalf("the merge was not reported: %v", p.Merged)
	}
	if len(p.Requantify) != 0 {
		t.Errorf("30 + 6 did not come to the 36 already in the deck: %+v", p.Requantify)
	}
	if len(p.Rewrite) != 1 || p.Rewrite[0].Why != "the first reason" {
		t.Errorf("the wrong reason won: %+v", p.Rewrite)
	}
}

// The stale-plan guard. Somebody editing in another tab, or a second click on
// the confirm button, must not have their plan applied to a deck it was never
// compared with.
func TestAPlanIsRefusedAgainstADeckThatMovedUnderIt(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})

	// Somebody else got there first, through an entirely ordinary edit.
	moved, err := SetCardField(bulkFixture, "Sol Ring", "why", "Somebody else wrote this.")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyBulk(moved, p)
	if err == nil {
		t.Fatal("a plan was applied to a deck it was not computed against; the " +
			"burials would be about a comparison nobody made")
	}
	if !strings.Contains(err.Error(), "changed while the plan was on screen") {
		t.Errorf("the refusal said %q, which does not tell somebody what happened", err)
	}
	// And the honest half: nothing was written, and a fresh plan works.
	fresh := plan(t, moved,
		BulkCard{Name: "Sol Ring", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if _, err := ApplyBulk(moved, fresh); err != nil {
		t.Fatalf("the re-read plan was refused too: %v", err)
	}
}

// The plan is a claim about a file, so a file that differs only in a comment
// is a different file. Strict fails safe, and the cost is one round trip.
func TestACommentIsEnoughToMoveTheDeck(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture, BulkCard{Name: "Sol Ring", Qty: 1})
	if _, err := ApplyBulk("# somebody wrote a note\n"+bulkFixture, p); err == nil {
		t.Error("a plan read one file and was applied to another")
	}
}

// A list naming nothing the deck holds is a whole-deck replacement, which is
// legitimate: every old card goes to the graveyard, named one by one, and
// every new one arrives. The confirmation is what makes it a choice.
func TestAListThatSharesNoCardsEmptiesTheNinetyNineIntoTheGraveyard(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Terastodon", Qty: 1, Category: "threat", Why: "Blows up three things."})
	if len(p.Entomb) != 4 {
		t.Fatalf("%d cards were planned for the graveyard, expected the whole 99: %v",
			len(p.Entomb), p.Entomb)
	}
	out, err := ApplyBulk(bulkFixture, p)
	if err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
	// The card list emptied and refilled: `cards: []` cannot carry entries
	// beneath it, and a bulk edit is the operation most likely to find out.
	if !strings.Contains(out, "name: Terastodon") {
		t.Errorf("the replacement did not land:\n%s", out)
	}
	if strings.Contains(out, "cards: []") {
		t.Errorf("the 99 was left empty with a card in it:\n%s", out)
	}
	for _, name := range []string{"Sol Ring", "Cultivate", "Craterhoof Behemoth", "Forest"} {
		if !strings.Contains(out[strings.Index(out, "graveyard:"):], "name: "+name) {
			t.Errorf("%s did not reach the graveyard:\n%s", name, out)
		}
	}
}

// A file this package cannot edit safely is refused while the plan is being
// read, not while it is being applied -- discovering it mid-fold would be
// discovering it after somebody agreed to something.
//
// The bare-string entry below is one of the three shapes ADR 12 anticipated:
// the deck parses to two cards and the text offers one `- name:` line, so no
// span can be trusted and every operation here refuses rather than guessing.
func TestAFileTheEditorCannotReadIsRefusedAtPlanTime(t *testing.T) {
	t.Parallel()
	ragged := "slug: x\nname: X\nstage: draft\ncards:\n  - Sol Ring\n" +
		"  - name: Cultivate\n    category: ramp\n    why: ramp\n"
	if _, err := PlanBulk(ragged, []BulkCard{{Name: "Sol Ring", Qty: 1}}); err == nil {
		t.Error("a deck the editor cannot walk produced a plan")
	}
	if _, err := PlanBulk("- this is not a mapping\n", nil); err == nil {
		t.Error("a file that is not a deck produced a plan")
	}
}

func TestAMissingPlanIsRefusedRatherThanCrashing(t *testing.T) {
	t.Parallel()
	if _, err := ApplyBulk(bulkFixture, nil); err == nil {
		t.Error("a nil plan was applied")
	}
}

// The fingerprint is a function of the text and of nothing else.
//
// The stability half is checked against a *copy* rather than against the same
// expression twice, which staticcheck reads -- correctly -- as a comparison
// that cannot fail. A copy is the real question anyway: the text arrives from
// two separate reads of the same file, not from one variable.
func TestTheFingerprintIsStableAndSeparatesFiles(t *testing.T) {
	t.Parallel()
	again := string([]byte(bulkFixture))
	if Fingerprint(bulkFixture) != Fingerprint(again) {
		t.Error("two readings of one file disagreed")
	}
	if Fingerprint(bulkFixture) == Fingerprint(bulkFixture+"\n") {
		t.Error("two different files share a fingerprint")
	}
}

// `0 Sol Ring` is a quantity SetCardField refuses, so it is predicted here
// rather than discovered at card forty. It probably means "take this out", and
// probably is not good enough to act on: the refusal says how to say that.
func TestAQuantityOfNoneIsRefusedWithTheWayToSayIt(t *testing.T) {
	t.Parallel()
	p := plan(t, bulkFixture,
		BulkCard{Name: "Sol Ring", Qty: 0},
		BulkCard{Name: "Cultivate", Qty: 1},
		BulkCard{Name: "Craterhoof Behemoth", Qty: 1},
		BulkCard{Name: "Forest", Qty: 36})
	if len(p.Blocked) != 1 || p.Blocked[0].Name != "Sol Ring" {
		t.Fatalf("a quantity of 0 was not caught: %+v", p.Blocked)
	}
	if !strings.Contains(p.Blocked[0].Reason, "leave the line off") {
		t.Errorf("the refusal does not say what to write instead: %q", p.Blocked[0].Reason)
	}
	// And it is not ALSO reported for burial: the list named the card, so
	// saying both would describe two different fates for one line.
	for _, name := range p.Entomb {
		if name == "Sol Ring" {
			t.Errorf("the card is blocked and buried at once: %v", p.Entomb)
		}
	}
	if _, err := ApplyBulk(bulkFixture, p); err == nil {
		t.Error("a blocked plan was applied")
	}
}

// A folded rationale parses with a trailing newline, and the plan renders the
// outgoing sentence inside quotation marks beside the incoming one. Untrimmed,
// that is a visible gap before the closing quote of somebody's own writing --
// found by looking at the real page, which no assertion about the fixture
// above could have found because the fixture's reasons are one-liners.
func TestTheOutgoingSentenceIsReportedWithoutItsFoldingWhitespace(t *testing.T) {
	t.Parallel()
	folded := "slug: g\nname: G\nstage: curated\ncards:\n" +
		"  - name: Sol Ring\n    category: ramp\n    why: >\n" +
		"      Two mana on turn one, and it always has been.\n"
	p := plan(t, folded, BulkCard{Name: "Sol Ring", Qty: 1, Why: "Mine now."})
	if len(p.Rewrite) != 1 {
		t.Fatalf("%d rewrites planned: %+v", len(p.Rewrite), p.Rewrite)
	}
	if want := "Two mana on turn one, and it always has been."; p.Rewrite[0].Was != want {
		t.Errorf("the outgoing sentence is reported as %q, want %q", p.Rewrite[0].Was, want)
	}
}

// The other half of the same trim: a paste whose reason differs from the
// deck's only by the folding whitespace is not a rewrite at all.
func TestAFoldedReasonPastedBackUnchangedIsNotARewrite(t *testing.T) {
	t.Parallel()
	folded := "slug: g\nname: G\nstage: curated\ncards:\n" +
		"  - name: Sol Ring\n    category: ramp\n    why: >\n" +
		"      Two mana on turn one, and it always has been.\n"
	p := plan(t, folded, BulkCard{Name: "Sol Ring", Qty: 1,
		Why: "Two mana on turn one, and it always has been."})
	if len(p.Rewrite) != 0 {
		t.Fatalf("a reason pasted back verbatim was read as a change: %+v", p.Rewrite)
	}
	if p.Touches() {
		t.Error("the plan found work in a list that says what the deck says")
	}
}
