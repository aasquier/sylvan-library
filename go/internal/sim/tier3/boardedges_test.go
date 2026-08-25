package tier3_test

import (
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The board's edges: the states a real game reaches rarely and a recording
// therefore may not hold at all.
//
// `scribe_test.go` holds the rulings against two real games, which is the
// right way to prove them. This file is the other half — the paths a two-game
// recording happens not to walk, driven the same way the real stream is, one
// synthetic line at a time through the public reader. Nothing here reaches
// into the assembler: a test that could see `board` directly would be testing
// a shape rather than a behaviour, and the shape is not the contract.

// played feeds a run of scribe lines and returns the games it closed.
func played(t *testing.T, lines ...string) []tier3.EventLog {
	t.Helper()
	p := tier3.NewScribeParser(true)
	var out []tier3.EventLog
	for _, line := range lines {
		if log, _ := p.Feed(line); log != nil {
			out = append(out, *log)
		}
	}
	return out
}

// last is where a card ended up, or "" if it never moved.
func last(log tier3.EventLog, id int) string {
	zone := ""
	for _, step := range log.Board.Steps {
		for _, change := range step.Changes {
			if change.ID == id && change.Zone != "" {
				zone = change.Zone
			}
		}
	}
	return zone
}

const (
	openGame = `{"t":"game","game":1}`
	seatOne  = `{"t":"seat","game":1,"seat":1,"who":"Gyome — Food","life":40}`
	seatTwo  = `{"t":"seat","game":1,"seat":2,"who":"Atla — Eggs","life":40}`
	endGame  = `{"t":"result","game":1,"ms":1000,"seat":1,"winner":"Gyome — Food"}`
)

func TestAZoneTheBoardDoesNotDrawIsIgnored(t *testing.T) {
	t.Parallel()
	// Forge has zones this board has no row for — Sideboard, Ante, Merged, and
	// whatever a later release adds. They are dropped rather than crashed on,
	// because a Forge that learns a zone must not take a match down.
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"zone","game":1,"zone":"Sideboard","mode":"in","seat":1,"id":5,"card":"Kaheera, the Orphanguard","types":"Legendary Creature - Cat Beast"}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		endGame)
	if len(logs) != 1 {
		t.Fatalf("%d games closed, want 1", len(logs))
	}
	if zone := last(logs[0], 5); zone != "" {
		t.Errorf("a sideboard card reached the board as %q", zone)
	}
}

func TestACounterGoingToZeroLeavesTheCard(t *testing.T) {
	t.Parallel()
	// The whole set crosses whenever any of it changes, because a browser
	// holding a partial set has no way to know a kind went to zero.
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,"id":7,"card":"Hazel's Brewmaster","types":"Creature - Squirrel Warlock","power":3,"toughness":4}`,
		`{"t":"counters","game":1,"counter":"+1/+1","was":0,"now":2,"id":7,"card":"Hazel's Brewmaster"}`,
		`{"t":"counters","game":1,"counter":"shield","was":0,"now":1,"id":7,"card":"Hazel's Brewmaster"}`,
		`{"t":"turn","game":1,"turn":2,"seat":2,"who":"Atla — Eggs","life":40}`,
		`{"t":"counters","game":1,"counter":"+1/+1","was":2,"now":0,"id":7,"card":"Hazel's Brewmaster"}`,
		`{"t":"turn","game":1,"turn":3,"seat":1,"who":"Gyome — Food","life":40}`,
		endGame)

	var newest []tier3.BoardCounter
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 7 && change.Counters != nil {
				newest = change.Counters
			}
		}
	}
	if len(newest) != 1 || newest[0].Kind != "shield" || newest[0].N != 1 {
		t.Errorf("after the +1/+1 counters left, the card carries %v; want the "+
			"shield alone", newest)
	}
}

func TestCountersRenderInAStableOrder(t *testing.T) {
	t.Parallel()
	// Insertion order would be a Go map's, which is randomised — a recorded
	// payload would flap between runs and no golden could ever be written.
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,"id":9,"card":"Marchesa","types":"Legendary Creature - Human"}`,
		`{"t":"counters","game":1,"counter":"stun","was":0,"now":1,"id":9,"card":"Marchesa"}`,
		`{"t":"counters","game":1,"counter":"+1/+1","was":0,"now":3,"id":9,"card":"Marchesa"}`,
		`{"t":"counters","game":1,"counter":"loyalty","was":0,"now":2,"id":9,"card":"Marchesa"}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		endGame)

	var kinds []string
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 9 && change.Counters != nil {
				kinds = nil
				for _, c := range change.Counters {
					kinds = append(kinds, c.Kind)
				}
			}
		}
	}
	want := []string{"+1/+1", "loyalty", "stun"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Errorf("counters came out %v, want %v sorted by kind", kinds, want)
	}
}

func TestALandThatAnimatesChangesRows(t *testing.T) {
	t.Parallel()
	// The split is on the type line, so a creature-land moves rows when it
	// stops being a land — which is honest about what it currently is. Reaching
	// this needs a type line that *changes*, which a two-game recording of two
	// fair decks does not happen to hold.
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,"id":11,"card":"Dryad Arbor","types":"Land Creature - Forest Dryad","power":1,"toughness":1}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		`{"t":"stats","game":1,"id":11,"card":"Dryad Arbor","types":"Creature - Dryad","power":3,"toughness":3}`,
		`{"t":"turn","game":1,"turn":2,"seat":2,"who":"Atla — Eggs","life":40}`,
		endGame)

	if zone := last(logs[0], 11); zone != tier3.ZoneBattlefield {
		t.Errorf("Dryad Arbor stopped being a land and stayed in %q; want the "+
			"battlefield row", zone)
	}
}

func TestAPermanentThatDiesTappedComesBackUntapped(t *testing.T) {
	t.Parallel()
	// Forge raises no untap event for a permanent that leaves the battlefield,
	// so without this a creature that died tapped would lie on its side in the
	// graveyard forever.
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,"id":13,"card":"Dockside Chef","types":"Creature - Human Citizen","power":2,"toughness":2}`,
		`{"t":"tapped","game":1,"tapped":true,"id":13,"card":"Dockside Chef"}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"out","seat":1,"id":13,"card":"Dockside Chef"}`,
		`{"t":"zone","game":1,"zone":"Graveyard","mode":"in","seat":1,"id":13,"card":"Dockside Chef"}`,
		endGame)

	tapped := true
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == 13 && change.Tapped != nil {
				tapped = *change.Tapped
			}
		}
	}
	if tapped {
		t.Error("a creature that died tapped is still tapped in the graveyard")
	}
	if zone := last(logs[0], 13); zone != tier3.ZoneGraveyard {
		t.Errorf("it ended in %q, want the graveyard", zone)
	}
}

func TestForgesOwnComplaintsStillInvalidateAScribedRun(t *testing.T) {
	t.Parallel()
	// **The failure this package exists to never serve.** An unimplemented card
	// does not stop a game: Forge prints a warning and plays on, reporting a
	// winner and a turn count that look entirely normal. That warning arrives
	// on the same streams whatever program is driving Forge, so the scribe's
	// reader has to see it too — it is not JSON, and a reader that only
	// understood its own lines would have thrown the one line that matters.
	p := tier3.NewScribeParser(true)
	for _, line := range []string{
		openGame, seatOne, seatTwo,
		`An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".`,
		`Could not load deck - broken.dck, match cannot start`,
		endGame,
	} {
		p.Feed(line)
	}
	out := p.Output()
	if out.Trustworthy() {
		t.Fatal("a run Forge complained about came back trustworthy; a match " +
			"with a dropped card reports a clean winner and means nothing")
	}
	if len(out.Unsupported) != 1 || out.Unsupported[0] != "Nonexistent Card 1" {
		t.Errorf("unsupported cards came back as %v", out.Unsupported)
	}
	if len(out.DeckLoadFailures) != 1 {
		t.Errorf("deck failures came back as %v", out.DeckLoadFailures)
	}
	// And the game still closed, because the row is the record and the caller
	// above is the one that decides a run is worthless.
	if len(out.Games) != 1 {
		t.Errorf("%d games closed, want 1", len(out.Games))
	}
}

func TestALineThisReaderCannotParseIsDroppedNotFatal(t *testing.T) {
	t.Parallel()
	// A dropped line is a hole in a picture; a thrown one is a lost match. The
	// scribe's own error line is in here too — it emits one rather than letting
	// an exception take the game down, and this side must not choke on it.
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"zone","game":1,"zone":`,
		`{"t":"scribe_error","game":1,"detail":"java.lang.IllegalStateException"}`,
		`{"t":"zone","game":1,"zone":"Battlefield","mode":"in","seat":1,"id":15,"card":"Academy Manufactor","types":"Artifact Creature - Assembly-Worker","power":1,"toughness":3}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		endGame)
	if len(logs) != 1 {
		t.Fatalf("%d games closed, want 1", len(logs))
	}
	if zone := last(logs[0], 15); zone != tier3.ZoneBattlefield {
		t.Errorf("the card after the broken line landed in %q", zone)
	}
}

func TestADeckPlayedAgainstItselfLeavesTheOutcomeSeatOff(t *testing.T) {
	t.Parallel()
	// Both seats carry the same name, so the sentence names a player that
	// could be either. A guess would be worse than a gap — the same wart
	// `shapeForge` records one layer up, and unreachable by convention rather
	// than refused.
	logs := played(t,
		openGame,
		`{"t":"seat","game":1,"seat":1,"who":"Gyome — Food","life":40}`,
		`{"t":"seat","game":1,"seat":2,"who":"Gyome — Food","life":40}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		`{"t":"outcome","game":1,"turn":8,"winner":"Gyome — Food","said":"Gyome — Food has won because all opponents have lost"}`,
		endGame)

	var outcome *tier3.GameEvent
	for i := range logs[0].Events {
		if logs[0].Events[i].Kind == tier3.EventOutcome {
			outcome = &logs[0].Events[i]
		}
	}
	if outcome == nil {
		t.Fatal("no outcome beat was raised")
	}
	if outcome.Seat != 0 {
		t.Errorf("the outcome was credited to seat %d; with two seats sharing "+
			"a name there is no honest answer", outcome.Seat)
	}
	// The sentence still reads correctly, which is the half that matters.
	if outcome.Note != "because all opponents have lost" {
		t.Errorf("the reason came back as %q", outcome.Note)
	}
	if outcome.Amount != 1 {
		t.Errorf("a win came back as Amount %d, want 1", outcome.Amount)
	}
}

func TestAMulliganIsABeat(t *testing.T) {
	t.Parallel()
	// Neither player mulliganed in the recorded match, so this path has no
	// coverage from it at all — and a mulligan is the first thing that happens
	// in a great many games.
	logs := played(t, openGame, seatOne, seatTwo,
		`{"t":"mulligan","game":1,"seat":2,"who":"Atla — Eggs"}`,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		endGame)

	found := false
	for _, e := range logs[0].Events {
		if e.Kind == tier3.EventMulligan && e.Seat == 2 {
			found = true
		}
	}
	if !found {
		t.Errorf("no mulligan beat was raised; the game's beats were %v",
			logs[0].Events)
	}
}

func TestAGameWithNoOutcomeFallsBackToTheTurnsItSaw(t *testing.T) {
	t.Parallel()
	// A clocked-out game is stopped rather than finished, so Forge raises no
	// outcome — and a row still has to report a length. The highest turn seen
	// is the honest answer, halved the way Forge halves its own.
	p := tier3.NewScribeParser(true)
	for _, line := range []string{
		openGame, seatOne, seatTwo,
		`{"t":"turn","game":1,"turn":1,"seat":1,"who":"Gyome — Food","life":40}`,
		`{"t":"turn","game":1,"turn":2,"seat":2,"who":"Atla — Eggs","life":40}`,
		`{"t":"turn","game":1,"turn":3,"seat":1,"who":"Gyome — Food","life":40}`,
		`{"t":"result","game":1,"ms":300000,"draw":true,"timed_out":true}`,
	} {
		p.Feed(line)
	}
	games := p.Output().Games
	if len(games) != 1 {
		t.Fatalf("%d games closed, want 1", len(games))
	}
	if games[0].Turns == nil || *games[0].Turns != 2 {
		t.Errorf("three player-turns came back as %v rounds, want 2",
			games[0].Turns)
	}
	if !games[0].TimedOut || games[0].WinnerSeat != nil {
		t.Errorf("a clock-out came back as %+v", games[0])
	}
}
