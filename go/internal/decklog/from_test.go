package decklog

import "testing"

// The swap's `From` field: one new field, one new clause, and the recorded
// sentence untouched. `From` says where the incoming card was lifted from --
// "swap_board" when the promotion took it off the deck's own board, empty for
// the straight swap -- and the only thing it may ever do to the history is add
// the one clause a person would say. The frozen corpus
// (testdata/describe.json) pins the plain sentence from the outside; these pin
// the field's behaviour from the inside, including the two ways it must do
// nothing at all.

// The promotion's own sentence, exactly as the route records it.
func TestASwapFromTheBoardSaysSo(t *testing.T) {
	t.Parallel()
	action, summary := Describe(Edit{Kind: EditSwap, Card: "Sol Ring",
		SwapIn: "Craterhoof Behemoth", From: "swap_board"})
	if action != "swap" {
		t.Errorf("the promotion is filed under %q, so it is not queryable as a swap", action)
	}
	if want := "swapped Sol Ring out for Craterhoof Behemoth from the swap board"; summary != want {
		t.Errorf("summary:\n got  %q\n want %q", summary, want)
	}
}

// The straight swap's sentence is byte-identical to what every entry before
// the field existed already says -- the field defaulting to empty IS the old
// behaviour, which is what lets the corpus rows decode without knowing it.
func TestAStraightSwapSentenceIsUntouched(t *testing.T) {
	t.Parallel()
	_, summary := Describe(Edit{Kind: EditSwap, Card: "Sol Ring", SwapIn: "Craterhoof Behemoth"})
	if want := "swapped Sol Ring out for Craterhoof Behemoth"; summary != want {
		t.Errorf("summary:\n got  %q\n want %q", summary, want)
	}
}

// A From nobody taught this renderer adds no clause. "swap_board" is a wire
// token with a player's phrase on file; a value without one must fall back to
// the plain sentence rather than print the token itself, because no internal
// name ever reaches a user's eyes (commandment 10).
func TestAFromNobodyTaughtRendersNoClause(t *testing.T) {
	t.Parallel()
	_, summary := Describe(Edit{Kind: EditSwap, Card: "Sol Ring",
		SwapIn: "Craterhoof Behemoth", From: "the_attic"})
	if want := "swapped Sol Ring out for Craterhoof Behemoth"; summary != want {
		t.Errorf("an unknown From changed the sentence:\n got  %q\n want %q", summary, want)
	}
}

// The fallback outlives the new field: an Edit whose kind this has never seen
// still says something, From set or not. Silence is the one failure mode a
// history cannot have, and a new field must not become a way to reach it.
func TestTheFallbackOutlivesTheFromField(t *testing.T) {
	t.Parallel()
	action, summary := Describe(Edit{From: "swap_board"})
	if action != "edit" || summary != "edited the deck" {
		t.Errorf("an unrecognised operation carrying a From said %q / %q", action, summary)
	}
}
