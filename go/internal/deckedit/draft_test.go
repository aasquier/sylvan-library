package deckedit

import (
	"strings"
	"testing"
)

// The provenance of a drafted sentence, which under [ADR 41] is the only thing
// left carrying a distinction the gate no longer makes.
//
// Aaron ruled on 2026-08-28 that a drafted rationale satisfies promotion to
// `curated`. That ruling is what makes these tests load-bearing rather than
// tidy: before it, a deck full of drafts could not be promoted and the mark
// was a nicety. After it, the mark is the whole record of who wrote the deck's
// thinking, so a draft that loses its mark is a real defect and a person's
// sentence that gains one is a worse defect.
const draftFixture = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
	"cards:\n  - name: Sol Ring\n    why: ''\n  - name: Arcane Signet\n" +
	"    why: the second-best two-mana rock\n"

func TestADraftedRationaleIsWrittenWithTheHandThatWroteIt(t *testing.T) {
	t.Parallel()
	out, err := DraftRationale(draftFixture, "Sol Ring", "Fast mana; every deck wants it")
	if err != nil {
		t.Fatalf("the draft was refused: %v", err)
	}
	if !strings.Contains(out, "why: Fast mana; every deck wants it") {
		t.Errorf("the rationale did not land:\n%s", out)
	}
	if !strings.Contains(out, "why_by: claude") {
		t.Errorf("a drafted rationale was written unmarked, which is a sentence "+
			"whose author is lost the moment it is read:\n%s", out)
	}
}

// The refusal that matters most, and the one an intake running twice would
// otherwise walk straight into.
func TestADraftNeverWritesOverARationaleSomebodyHas(t *testing.T) {
	t.Parallel()
	_, err := DraftRationale(draftFixture, "Arcane Signet", "a drafted sentence")
	if err == nil {
		t.Fatal("a draft overwrote a rationale that was already there -- the " +
			"person's words are gone and the mark now describes the wrong sentence")
	}
	if !strings.Contains(err.Error(), "already has a rationale") {
		t.Errorf("the refusal said %q", err)
	}
}

func TestADraftRefusesToWriteNothing(t *testing.T) {
	t.Parallel()
	for _, empty := range []string{"", "   ", "\n\t "} {
		if _, err := DraftRationale(draftFixture, "Sol Ring", empty); err == nil {
			t.Errorf("an empty draft (%q) was written; a marked blank claims a "+
				"model wrote nothing, which is worse than an unmarked blank", empty)
		}
	}
}

// Editing a draft is adopting it. The mark comes off in `SetCardField` and not
// in the intake, because SetCardField is the single door every `why` goes
// through -- the deck page, the CLI, a swap -- and a rule enforced at one of
// those holds until the second caller.
func TestWritingOverADraftTakesTheMarkOff(t *testing.T) {
	t.Parallel()
	drafted, err := DraftRationale(draftFixture, "Sol Ring", "Fast mana")
	if err != nil {
		t.Fatalf("the draft was refused: %v", err)
	}
	if !strings.Contains(drafted, "why_by: claude") {
		t.Fatalf("nothing to un-mark:\n%s", drafted)
	}

	adopted, err := SetCardField(drafted, "Sol Ring", "why", "Ramp, and it is never cut")
	if err != nil {
		t.Fatalf("the edit was refused: %v", err)
	}
	if !strings.Contains(adopted, "why: Ramp, and it is never cut") {
		t.Errorf("the person's rationale did not land:\n%s", adopted)
	}
	if strings.Contains(adopted, "why_by") {
		t.Errorf("the mark survived a person writing over the sentence, so the "+
			"deck file now credits a model for words somebody typed:\n%s", adopted)
	}
}

// Blanking counts as writing over it: what is gone was not drafted by anybody.
func TestBlankingADraftAlsoTakesTheMarkOff(t *testing.T) {
	t.Parallel()
	drafted, err := DraftRationale(draftFixture, "Sol Ring", "Fast mana")
	if err != nil {
		t.Fatalf("the draft was refused: %v", err)
	}
	blanked, err := SetCardField(drafted, "Sol Ring", "why", "")
	if err != nil {
		t.Fatalf("blanking a draft in a draft deck was refused: %v", err)
	}
	if strings.Contains(blanked, "why_by") {
		t.Errorf("an empty rationale is still marked as drafted:\n%s", blanked)
	}
}

// `why_by` is not something anybody types. It is written by DraftRationale and
// dropped by SetCardField, so it tracks who last touched the sentence rather
// than what somebody last claimed about it.
func TestTheMarkIsNotAFieldAnybodyCanSet(t *testing.T) {
	t.Parallel()
	if _, err := SetCardField(draftFixture, "Sol Ring", "why_by", "aaron"); err == nil {
		t.Error("`why_by` is settable by hand, so the provenance of every " +
			"rationale in the library is now a claim rather than a record")
	}
}

// A deck's `strategy` is the paragraph the shelf, the deck page and the primer
// all render, and until 2026-08-29 nothing in this app could write it.
//
// The editor left it out of `SettableDeckFields` and sent it to `SetNote` in a
// comment — but `SetNote` writes into the `notes:` mapping and cannot reach a
// top-level key, so the field was settable only by hand-editing a file that
// lives on the deployed instance's volume.
func TestADecksStrategyCanBeWritten(t *testing.T) {
	t.Parallel()
	const bare = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"cards:\n  - name: Sol Ring\n    why: ramp\n"

	out, err := SetDeckField(bare, "strategy",
		"Golgari Food aristocrats. Gyome turns every nontoken creature into a "+
			"meal, and the deck drains the table sacrificing them.")
	if err != nil {
		t.Fatalf("writing a strategy was refused: %v", err)
	}
	if !strings.Contains(out, "strategy:") {
		t.Fatalf("the key was not written:\n%s", out)
	}
	if !strings.Contains(out, "Golgari Food aristocrats.") {
		t.Errorf("the text did not land:\n%s", out)
	}
	// It goes where the emitter puts it, not at the end of the file: a deck
	// written by hand and a deck written here have to look alike.
	if strings.Index(out, "strategy:") > strings.Index(out, "cards:") {
		t.Errorf("strategy was appended after the cards:\n%s", out)
	}

	// Rewriting replaces rather than doubling.
	again, err := SetDeckField(out, "strategy", "A different plan entirely.")
	if err != nil {
		t.Fatalf("rewriting was refused: %v", err)
	}
	if strings.Count(again, "strategy:") != 1 {
		t.Errorf("the strategy was written twice:\n%s", again)
	}
	if strings.Contains(again, "Golgari Food") {
		t.Errorf("the old strategy survived under the new one:\n%s", again)
	}
}

// Blanking is refused rather than written: an empty `strategy:` puts a blank
// paragraph at the top of the generated primer, and the way to have no
// strategy is to not have the key.
func TestABlankStrategyIsRefused(t *testing.T) {
	t.Parallel()
	const deck = "slug: gyome\nname: Gyome\nstatus: theoretical\nstage: draft\n" +
		"strategy: A plan.\ncards:\n  - name: Sol Ring\n    why: ramp\n"
	for _, empty := range []string{"", "   ", "\n\t"} {
		if _, err := SetDeckField(deck, "strategy", empty); err == nil {
			t.Errorf("a blank strategy (%q) was written", empty)
		}
	}
}
