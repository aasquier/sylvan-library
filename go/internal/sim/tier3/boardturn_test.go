package tier3_test

import "testing"

// A card that turns over says so, because Forge renames it.
//
// **The dictionary files a card under the name it was first seen by and never
// revises it**, which is deliberate: a board folded to step three has to show
// what was true at step three, and rewriting the name would rewrite the past.
// So a rename cannot live in the dictionary — it has to travel as a change, on
// the step it happened, exactly like a type line or a tap does.
//
// **And it has to travel, because the type line cannot always carry it.** The
// browser's `faceInPlay` turns a card over by comparing the live type line
// against each face's, which answers for a Delver of Secrets becoming an
// Insectile Aberration. It cannot answer for two faces that share a type line:
// both halves of every Pathway are `Land`, so it sees two matches and rightly
// refuses to guess — and the board went on showing the front of a card
// standing on its back. Measured over the pool on 2026-09-07, 48 cards have
// two faces a type line cannot tell apart, and Aaron caught one on the
// deployed board the same day: they "show funny depending on what side they
// are on".
func TestACardForgeRenamesSaysSoOnTheStepItHappened(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		// Drawn as its front, which is how a modal double-faced card sits in
		// a hand.
		zoneLine(32, "Hengegate Pathway", "Land", "Hand", "in", 1),
		// Played as its back. Forge names the face; the type line is `Land`
		// either way and says nothing at all about which.
		zoneLine(32, "Mistgate Pathway", "Land", "Battlefield", "in", 1),
		turnLine(2, 2),
		endGame)

	if len(logs) != 1 {
		t.Fatalf("the run closed %d games, want 1", len(logs))
	}
	renamed := ""
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 32 && change.Name != "" {
				renamed = change.Name
			}
		}
	}
	if renamed != "Mistgate Pathway" {
		t.Errorf("the rename reached the board as %q, want %q -- without it "+
			"the room draws a Hengegate Pathway on a table holding a Mistgate "+
			"Pathway, and no type line can tell it otherwise", renamed,
			"Mistgate Pathway")
	}

	// The dictionary keeps what it learned. This is the half that makes
	// folding to an earlier step honest.
	filed := ""
	for _, card := range logs[0].Board.Cards {
		if card.ID == 32 {
			filed = card.Name
		}
	}
	if filed != "Hengegate Pathway" {
		t.Errorf("the dictionary now calls the card %q, want %q -- revising it "+
			"rewrites every step before the turn", filed, "Hengegate Pathway")
	}
}

// A card named again by the name it already has raises nothing.
//
// Every line that touches a card names it, so the ordinary case is a name
// repeated dozens of times in a game. If that raised a change, every card in
// every match would carry a rename on every step it did anything at all — and
// the browser would spend the match turning cards over to the face they were
// already showing.
func TestANameRepeatedIsNotARename(t *testing.T) {
	t.Parallel()
	logs := played(t, openGame, seatOne, seatTwo,
		turnLine(1, 1),
		zoneLine(41, "Sol Ring", "Artifact", "Battlefield", "in", 1),
		zoneLine(41, "Sol Ring", "Artifact", "Battlefield", "in", 1),
		turnLine(2, 2),
		endGame)

	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 41 && change.Name != "" {
				t.Fatalf("a card named twice by one name raised a rename to %q",
					change.Name)
			}
		}
	}
}
