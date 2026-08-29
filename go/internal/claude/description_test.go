package claude

import (
	"encoding/json"
	"strings"
	"testing"
)

// The report shape the deck page's description assist reads.
//
// `DescribeDeck` itself is proved next door in `intake_test.go`; what is here
// is the rendering, and the two things it can get wrong are both things this
// project has got wrong before: a nil slice reaching a client as `null` when
// the client indexes it, and a payload that carries an opinion without saying
// whose it is.

func TestADescriptionReportSaysWhoAnsweredAndCarriesItsOwnPromise(t *testing.T) {
	t.Parallel()
	collaborator, err := Preset("collaborator")
	if err != nil {
		t.Fatal(err)
	}
	got := DescriptionFor("gyome-food", Description{
		Strategy: "Cook Food, sacrifice it, drain the table.",
		Themes:   []string{"food", "aristocrats"},
		Fact:     "Gyome makes Food on every death.",
	}, IntakeOutcome{Stance: collaborator, Asked: true})

	if got.AnsweredBy != "claude" {
		t.Errorf("answered_by is %q -- ADR 14's third boundary is a field", got.AnsweredBy)
	}
	if got.Mode != ModeDeckDescription {
		t.Errorf("mode is %q, want %q", got.Mode, ModeDeckDescription)
	}
	if got.Slug != "gyome-food" || got.Strategy == "" || got.Fact == "" {
		t.Errorf("the description did not survive: %+v", got)
	}
	// The promise travels in the payload, not only in the component, so a
	// second client cannot render this as anything other than a draft.
	if !strings.Contains(got.Never, "Nothing is saved") {
		t.Errorf("never is %q", got.Never)
	}
	if got.Never != DescriptionNever {
		t.Errorf("never is not the constant: %q", got.Never)
	}
}

// A theme list nobody filled in goes out as `[]`, never `null`. The client
// indexes it, and "a fallback that reads as a fact" is the mistake this repo
// makes most often -- here it would be `null.length` and a blank panel.
func TestADescriptionWithNoThemesRendersAnEmptyListRatherThanNull(t *testing.T) {
	t.Parallel()
	got := DescriptionFor("kaheera", Description{Strategy: "A fixture."},
		IntakeOutcome{Asked: true})
	if got.Themes == nil {
		t.Fatal("themes is nil; it must be an empty slice")
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"themes":[]`) {
		t.Errorf("themes reached the wire as something other than []: %s", raw)
	}
}

// The not-asked outcome is a real answer and keeps its reason: a stance of
// `off` costs nothing and says so, and the client must be able to tell that
// apart from a call that failed.
func TestADescriptionThatWasNeverAskedKeepsItsReason(t *testing.T) {
	t.Parallel()
	got := DescriptionFor("kaheera", Description{},
		IntakeOutcome{Stance: Off, Reason: "The stance is off, so no call was made."})
	if got.Asked {
		t.Error("asked is true on an outcome that never asked")
	}
	if got.Reason == "" {
		t.Error("nothing happened and the report does not say why")
	}
	if got.Strategy != "" {
		t.Errorf("a strategy appeared out of nothing: %q", got.Strategy)
	}
}
