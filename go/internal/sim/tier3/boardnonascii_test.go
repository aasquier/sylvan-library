package tier3_test

import "testing"

// A card whose name is not ASCII arrives with its name.
//
// **This is the Go half of a fault that was live on the deployed board, and
// the Java half is the one that was broken.** `Scribe.java` writes to
// `System.out`, and a `PrintStream` encodes with `stdout.encoding` -- which is
// not `file.encoding` and does not follow JEP 400. Measured on the Forge
// worker, 2026-09-07, an image that sets no locale at all:
//
//	file.encoding   = UTF-8
//	stdout.encoding = ANSI_X3.4-1968      <- US-ASCII
//
// A US-ASCII encoder does not fail on a character it cannot carry; it
// substitutes `?`. So Mjolnir-with-an-umlaut left the worker as
// `Mj?lnir, Storm Hammer`, missed [pool.Conn.GetCards] -- an exact lookup on
// the lower-cased name -- and drew a plate with no painting on it. Ninety-seven
// cards in the pool have a non-ASCII name, and Lorien Revealed, Barad-dur,
// Anduril and Nazgul are not obscure ones: every one of them was blank in every
// match, and Aaron saw it happen (2026-09-07, "on the board during a match").
//
// **The substitution is lossy, which is why the fix could only be at the
// source**: nothing here could turn `Mj?lnir` back into a name, and a fold that
// tried would be guessing. `Json.java` now escapes everything above 0x7e as
// `\uXXXX`, so the wire is ASCII whatever the host's locale is.
//
// What this test holds is the other end of that contract: the parser must land
// the escape as the real name, because that string is the key the painting is
// looked up under. `encoding/json` decodes `\uXXXX` without being asked -- so
// this is a test that a dependency behaves, which earns its place only because
// the whole bug was one link in this chain quietly not behaving.
func TestACardNamedOutsideASCIIKeepsItsName(t *testing.T) {
	t.Parallel()
	// The escape exactly as the fixed `Json.java` writes it. A raw string
	// literal, so Go leaves the backslash alone and the parser is the thing
	// doing the decoding -- which is the point.
	const line = `{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,` +
		`"id":404,"card":"Mj\u00f6lnir, Storm Hammer",` +
		`"types":"Legendary Artifact - Equipment"}`

	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		line,
		endGame)

	if len(logs) != 1 {
		t.Fatalf("the run closed %d games, want 1", len(logs))
	}
	const want = "Mjölnir, Storm Hammer"
	for _, card := range logs[0].Board.Cards {
		if card.ID != 404 {
			continue
		}
		if card.Name != want {
			t.Fatalf("the board calls the card %q, want %q -- a name that does "+
				"not survive the wire is a card with no painting on the board",
				card.Name, want)
		}
		return
	}
	t.Fatalf("no card with id 404 reached the board's dictionary at all")
}
