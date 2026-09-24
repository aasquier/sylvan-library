package decklog

import (
	"context"
	"strings"
	"testing"
)

// The history's own corners: the reader asked for nothing, the recorder that
// never opened anything, and the sentence for an operation whose field this
// renderer has no word for.
//
// The rule underneath all three is the one ADR 28 set: **the log never fails
// the edit that produced it, and it never says nothing.** A recorder with no
// database warns and carries on; an operation nobody taught it still renders
// a sentence. Silence is the one failure mode a history cannot have, because
// an edit that happened and left no row is an edit nobody can find again.

// A reader that asks for nothing gets one entry rather than a query with a
// zero or a negative limit in it -- "show me the history" with no number is
// a page, never nothing and never the whole thing.
func TestAReaderThatAsksForNothingStillGetsSomething(t *testing.T) {
	t.Parallel()
	path := newScratchDB(t)
	recorder, err := NewRecorder(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	ctx := context.Background()
	for _, card := range []string{"Fixture Signet", "Fixture Bear", "Fixture Wastes"} {
		recorder.Record(ctx, "corners", nil, "", Edit{Kind: EditAdd, Card: card, Into: "cards"})
	}

	for _, limit := range []int{0, -1, -100} {
		got, err := Entries(ctx, recorder.DB(), nil, "corners", limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if len(got) != 1 {
			t.Errorf("a limit of %d returned %d entries", limit, len(got))
		}
	}
	// And a real limit still means what it says.
	got, err := Entries(ctx, recorder.DB(), nil, "corners", 2)
	if err != nil || len(got) != 2 {
		t.Errorf("a limit of 2 returned %d entries (%v)", len(got), err)
	}
}

// A history over a database that has gone reports the fault. An empty list
// reads as "nothing has ever happened to this deck", which is a different
// sentence from "I cannot read this deck's history" -- and a deck page that
// said the first one would have somebody believing their edits were never
// recorded.
func TestAHistoryOverADatabaseThatHasGoneReportsTheFault(t *testing.T) {
	t.Parallel()
	path := newScratchDB(t)
	recorder, err := NewRecorder(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	db := recorder.DB()
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := Entries(context.Background(), db, nil, "corners", DefaultLimit)
	if err == nil {
		t.Errorf("a history of %v came back over a database that has gone", got)
	}
}

// **A nil recorder is a real state**, not a bug: an instance with no `app.db`
// has one, and every call against it has to be safe. The handle it hands out
// is nil -- which `Entries` reads as a deck with no history -- and closing it
// is a no-op rather than a panic at shutdown.
func TestARecorderThatNeverOpenedAnythingIsStillSafeToUse(t *testing.T) {
	t.Parallel()
	var none *Recorder
	if db := none.DB(); db != nil {
		t.Error("a recorder with no database handed one over")
	}
	if err := none.Close(); err != nil {
		t.Errorf("closing a recorder that never opened anything: %v", err)
	}
	// And the reader treats that nil handle as an empty history rather than
	// as a fault, which is the laptop's normal state.
	got, err := Entries(context.Background(), none.DB(), nil, "corners", DefaultLimit)
	if err != nil || len(got) != 0 {
		t.Errorf("a deck on an instance with no accounts database has %v (%v)", got, err)
	}
	// An empty Recorder value is the same: Close is a no-op.
	if err := (&Recorder{}).Close(); err != nil {
		t.Errorf("closing an unopened recorder: %v", err)
	}
}

// The intake's sentence when nobody said which pass it was.
//
// `Field` carries `why` or `category`, and a pass added later would arrive
// here with a field this table has no word for -- or with none at all. The
// sentence still has to read, and it may not print the field's own token at a
// player (commandment 10).
func TestTheIntakeSentenceReadsWhenNobodySaidWhichPass(t *testing.T) {
	t.Parallel()
	action, summary := Describe(Edit{Kind: EditIntake, Cards: []string{"Fixture Signet"}})
	if action != "intake" {
		t.Errorf("the pass is filed under %q", action)
	}
	if !strings.Contains(summary, "entry") {
		t.Errorf("the sentence reads %q and names nothing that was drafted", summary)
	}
	// A pass with no cards at all still says what happened.
	if _, empty := Describe(Edit{Kind: EditIntake}); empty == "" {
		t.Error("an intake that wrote nothing said nothing")
	}
}

// A list-valued setting is joined into the sentence rather than dumped into
// it: `themes` is a list, and a container's default rendering would put
// brackets and quotes in front of somebody reading their own history.
func TestAListValuedSettingReadsAsWordsRatherThanAsAContainer(t *testing.T) {
	t.Parallel()
	for _, value := range []any{
		[]string{"aggro", "ramp"},
		[]any{"aggro", "ramp"},
	} {
		_, summary := Describe(Edit{Kind: EditSetDeck, Field: "themes", Value: value})
		if !strings.Contains(summary, "aggro, ramp") {
			t.Errorf("%#v rendered as %q", value, summary)
		}
		for _, bracket := range []string{"[", "]", `"`} {
			if strings.Contains(summary, bracket) {
				t.Errorf("%#v put %q into a sentence: %q", value, bracket, summary)
			}
		}
	}
}
