package api

import (
	"errors"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// What the arena says when a match cannot be played.
//
// **The room was reciting the hosting stack at whoever pressed the button.** A
// Tier 3 match whose worker would not come up answered, verbatim, on the live
// site:
//
//	The match failed: forge worker: Machines API GET
//	/apps/sylvan-library/machines/080e90dec3d918/wait?state=started&timeout=60
//	answered 408: {"error":"deadline_exceeded: machine failed to reach desired
//	state, started, currently stopped"}
//
// A host's control plane, the app's name, a machine id, an HTTP verb, a status
// code and raw JSON — to a newcomer who pressed one button in a room about
// Magic. Commandment 10 says no technology backing this site ever renders, and
// that renders all of it. Found on 2026-08-25 by playing a real match on the
// deployed instance, which is the only place it exists; Aaron asked for it
// fixed on 2026-08-28, and named it first of the two because it is the one that
// hurts a beginner.
//
// **The detail is not deleted, it is redirected.** Everything above is exactly
// what somebody fixing the arena wants, and none of it is what somebody
// watching a match wants — so it goes to the log, where the first audience
// reads, and the room is handed a sentence written for the second. Every caller
// here logs before it speaks; a fault that reached a person and left no trace
// would be a worse bug than the one being fixed.
//
// **The vocabulary already existed.** The gate says *"has no cards in it yet,
// so no result would mean anything — add its cards and send them in again"*,
// which is the register: what happened, in words about the game, and what to do
// next. These are that sentence's siblings.
//
// One thing this deliberately does **not** do is dress up a refusal that is
// already about Magic. A deck with cards Forge cannot play names those cards,
// and that message is the most useful thing this whole surface produces —
// [tier3.ErrCoverageFailed] never comes through here.

// forgeTrouble is the room's own words for a match that could not be played.
//
// Three answers, and the split is what [tier3.ErrWorkerNotReady] was added for:
// *come back in a minute* and *this will never work here* are different news,
// and until that sentinel existed every one of these was the same error.
func forgeTrouble(err error) string {
	switch {
	case err == nil:
		return ""
	// The machine that plays the games is not answering *yet*. Almost always
	// the first match after a change lands, because the arena is rebuilt and
	// the first person through the door waits for it to open. Saying so, and
	// saying that trying again works, is the whole of what a person needs —
	// and it is true, which is why it is worth saying.
	case errors.Is(err, tier3.ErrWorkerNotReady):
		return "the arena would not open in time. This is nearly always a " +
			"moment's bad luck rather than anything wrong with these decks — " +
			"send the two of them in again and it usually opens straight away"
	// Nothing here can play games at all: a checkout with no Forge in it, or an
	// instance that was never given an arena. Telling somebody to try again
	// would be sending them round a loop that has no exit.
	case errors.Is(err, tier3.ErrForgeNotInstalled):
		return "there is nobody to play the games here yet, so this arena is " +
			"dark. Nothing is wrong with these decks"
	// The match started and did not finish: a stream that ended without a
	// result, a shim that went away mid-game, a JVM that died. Rare, and
	// recoverable in exactly the same way.
	default:
		return "the match broke off before it reached a result. Send the two " +
			"decks in again"
	}
}
