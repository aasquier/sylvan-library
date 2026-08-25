package tier3_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The scribe's reader, against a real match played by a real Forge.
//
// `testdata/scribed-match.ndjson` is two seeded games of `gyome-food` against
// `atla-palani-dinos` — the pairing ADR 42 was argued on, chosen because both
// decks are *about* tokens and Forge's game log never mentions one. It is a
// frozen recording: 1,048 lines out of a JVM that took a minute to produce
// them, so every claim below is checked in a tenth of a second with no Java
// anywhere. That is ADR 14's division paying for itself — the board is a
// decision about a game of Magic, and decisions are testable.
//
// **These are the rulings, not the implementation.** Each test below is one
// of the four findings `board.go` opens with, held to the recording that
// produced it.

// scribed is the recorded match, fed through the reader.
func scribed(t *testing.T, watching bool) ([]tier3.EventLog, []tier3.GameResult) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "scribed-match.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	p := tier3.NewScribeParser(watching)
	var logs []tier3.EventLog
	var games []tier3.GameResult
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		log, game := p.Feed(line)
		if log != nil {
			logs = append(logs, *log)
		}
		if game != nil {
			games = append(games, *game)
		}
	}
	return logs, games
}

func TestTheScribesStreamPlaysBackAsRowsAndBeats(t *testing.T) {
	t.Parallel()
	logs, games := scribed(t, true)

	if len(games) != 2 {
		t.Fatalf("the recording holds %d games, want 2", len(games))
	}
	if len(logs) != 2 {
		t.Fatalf("%d games were narrated, want 2", len(logs))
	}

	// Both games were won by seat 1 in the recording, in 23787ms and 16888ms.
	// The label is Forge's own `Ai(<seat>)-<name>`, rebuilt so a row is
	// identical whichever program played the match.
	for i, want := range []struct {
		ms, turns, seat int
		label           string
	}{
		// Turns are **rounds**, which is Forge's own arithmetic: fifteen
		// player-turns is eight rounds and eighteen is nine. `scribe.go`
		// carries the bytecode this copies.
		{23787, 8, 1, "Ai(1)-Gyome, Master Chef — Food"},
		{16888, 9, 1, "Ai(1)-Gyome, Master Chef — Food"},
	} {
		got := games[i]
		if got.Milliseconds != want.ms {
			t.Errorf("game %d ran %dms, want %d", i+1, got.Milliseconds, want.ms)
		}
		if got.Turns == nil || *got.Turns != want.turns {
			t.Errorf("game %d took %v turns, want %d", i+1, got.Turns, want.turns)
		}
		if got.WinnerSeat == nil || *got.WinnerSeat != want.seat {
			t.Errorf("game %d was won by seat %v, want %d", i+1, got.WinnerSeat, want.seat)
		}
		if got.Winner == nil || *got.Winner != want.label {
			t.Errorf("game %d's winner reads %v, want %q", i+1, got.Winner, want.label)
		}
		if got.Draw || got.TimedOut {
			t.Errorf("game %d came back draw=%v clocked=%v, want neither",
				i+1, got.Draw, got.TimedOut)
		}
	}
}

// The pacing contract: a step per beat, one for one.
//
// This is what lets the room drain the beats at reading speed and have the
// picture move exactly when the sentence is spoken, from one clock. If these
// two lists ever differ in length the board and the account are telling
// different games.
func TestEveryBeatHasItsBoardStep(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)
	for _, log := range logs {
		if log.Board == nil {
			t.Fatalf("game %d came back with no board", log.Game)
		}
		if len(log.Board.Steps) != len(log.Events) {
			t.Errorf("game %d has %d beats and %d board steps; they must be "+
				"parallel or the picture and the account disagree",
				log.Game, len(log.Events), len(log.Board.Steps))
		}
		if len(log.Events) == 0 {
			t.Errorf("game %d raised no beats at all", log.Game)
		}
	}
}

// Ruling 2: the library never crosses.
//
// Forge reports every zone, so we *could* show the top of anyone's deck.
// Showing a hand is a broadcast; showing the library is showing the answers.
// It is dropped in Go so that it cannot reach a browser by being forgotten
// about somewhere further along.
func TestTheLibraryIsNeverOnTheBoard(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)
	for _, log := range logs {
		for _, step := range log.Board.Steps {
			for _, change := range step.Changes {
				if change.Zone == "library" || change.Zone == "Library" {
					t.Fatalf("game %d put a card in the library; the library "+
						"is the one zone that may not be shown", log.Game)
				}
			}
		}
	}
	// And the hand *is* shown — the ruling is a pair, and a test that only
	// proved the library was absent would pass on a board with no zones at all.
	hands := 0
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.Zone == tier3.ZoneHand {
				hands++
			}
		}
	}
	if hands < 7 {
		t.Errorf("only %d cards ever reached a hand; both hands are meant to "+
			"be drawable", hands)
	}
}

// Ruling 3: Forge's phantom is not a card.
//
// A `Commander Effect` sits in each command zone with a real id, a real name
// and an empty type line. Drawing it would put a blank card beside every
// commander.
func TestForgesCommanderPhantomIsNotDrawn(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)
	for _, log := range logs {
		for _, step := range log.Board.Steps {
			for _, change := range step.Changes {
				for _, card := range log.Board.Cards {
					if card.ID == change.ID && card.Name == "Commander Effect" {
						t.Fatalf("game %d moved Forge's `Commander Effect` "+
							"phantom onto the board", log.Game)
					}
				}
			}
		}
	}
}

// Ruling 1: the stack is not a zone, and the proof is a card that would
// otherwise come back from the dead.
//
// `Sakura-Tribe Elder` sacrifices itself: it reaches the graveyard and *then*
// raises a `Stack in`, because Forge puts the source card on the stack when
// its ability is activated. A board that modelled the stack as a set would
// take it out of the graveyard and stand it back up.
func TestAnAbilityOnTheStackDoesNotRaiseTheDead(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)

	var elder int
	for _, card := range logs[0].Board.Cards {
		if card.Name == "Sakura-Tribe Elder" {
			elder = card.ID
		}
	}
	if elder == 0 {
		t.Skip("the recording no longer holds a Sakura-Tribe Elder")
	}

	// The whole journey, because the sequence is what proves the ruling. In
	// the recording the Elder is drawn, cast, sacrifices itself, and is later
	// exiled out of the graveyard — with a `Stack in` sitting between the
	// graveyard and the exile. If the stack were a zone, that line would stand
	// the Elder back up and the sequence would grow a battlefield in the
	// middle of it.
	var journey []string
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			if change.ID == elder && change.Zone != "" {
				journey = append(journey, change.Zone)
			}
		}
	}
	// `gone` after the hand is the Elder **on the stack**, which is the ruling
	// rather than a gap in it: a card being cast is in no zone this board
	// draws, so it leaves the hand and arrives on the battlefield with a
	// moment of nowhere in between. That is what being on the stack looks like
	// to a spectator, and it is the honest picture.
	want := []string{tier3.ZoneHand, tier3.ZoneGone, tier3.ZoneBattlefield,
		tier3.ZoneGraveyard, tier3.ZoneExile}
	if len(journey) != len(want) {
		t.Fatalf("Sakura-Tribe Elder went %v; want %v — an ability going on "+
			"the stack is not a zone change", journey, want)
	}
	for i := range want {
		if journey[i] != want[i] {
			t.Errorf("Sakura-Tribe Elder went %v; want %v", journey, want)
			break
		}
	}
}

// Lands sit in their own row, which is Aaron's ask and the difference between
// a battlefield and a pile.
func TestLandsGetTheirOwnZone(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)

	lands, permanents := 0, 0
	for _, step := range logs[0].Board.Steps {
		for _, change := range step.Changes {
			switch change.Zone {
			case tier3.ZoneLand:
				lands++
			case tier3.ZoneBattlefield:
				permanents++
			}
		}
	}
	if lands == 0 {
		t.Error("no card ever reached the land row in a fifteen-turn game")
	}
	if permanents == 0 {
		t.Error("no card ever reached the battlefield in a fifteen-turn game")
	}
}

// The tokens are the reason any of this exists.
//
// ADR 42's measurement: four Food-makers resolved and Forge's *log* said the
// word "token" exactly never. The bus says it forty times.
func TestTheTokensAreOnTheBoard(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)

	kinds := map[string]bool{}
	for _, card := range logs[0].Board.Cards {
		if card.Token {
			kinds[card.Name] = true
		}
	}
	if len(kinds) < 3 {
		t.Errorf("the board holds %d kinds of token (%v); the recorded game "+
			"made Food, Clue, Treasure, Egg and Spirit", len(kinds), kinds)
	}
	if !kinds["Food Token"] {
		t.Error("a Gyome game with no Food on the board is the exact failure " +
			"the scribe was written for")
	}
}

// The account's vocabulary, held against a real match.
//
// **This test exists because a whole beat kind went missing and nothing
// noticed.** `dies` never fired at all: Forge announces `Battlefield out`
// before `Graveyard in`, so by the time the graveyard arrived the card's zone
// was already `gone` and "did this come from the battlefield?" had no answer
// left. Eighteen cards reached graveyards in this recording and the account
// said none of them was destroyed — and every other test still passed, because
// they all asked about the board and none asked what the room would *say*.
//
// So this asks the shape of the question that was missing: over two real games
// of Commander, does each beat a person watching would expect actually get
// raised? A kind that stops appearing is a kind that stopped working.
func TestTheAccountKeepsItsWholeVocabulary(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)

	seen := map[tier3.EventKind]int{}
	for _, log := range logs {
		for _, e := range log.Events {
			seen[e.Kind]++
		}
	}
	// Every kind two fair Commander decks must produce between them. Blocks
	// are deliberately not here — Forge's AI chump-blocked exactly once in
	// these two games, and a gate that depends on an opponent's judgement is
	// a gate that fails on a re-record.
	for _, kind := range []tier3.EventKind{
		tier3.EventTurn, tier3.EventLand, tier3.EventCast, tier3.EventEnters,
		tier3.EventAttack, tier3.EventUnblocked, tier3.EventDamage,
		tier3.EventLife, tier3.EventDies, tier3.EventOutcome,
	} {
		if seen[kind] == 0 {
			t.Errorf("no %q beat was raised across two whole games; the room "+
				"has gone quiet about something it used to say", kind)
		}
	}
	t.Logf("the account said: %v", seen)
}

// Not watching costs nothing: the rows are the same and no board is built.
func TestAMatchNobodyWatchesCarriesNoBoard(t *testing.T) {
	t.Parallel()
	watched, watchedGames := scribed(t, true)
	quiet, quietGames := scribed(t, false)

	if len(quiet) != len(watched) {
		t.Fatalf("a quiet run closed %d games, a watched one %d",
			len(quiet), len(watched))
	}
	// Rendered rather than compared field by field: a [GameResult] holds
	// three pointers, so `==` asks whether two runs happened to allocate at
	// the same address, which is a question with no bearing on anything. The
	// wire codec is what every consumer sees, so the wire codec is what is
	// held equal.
	for i := range quietGames {
		quiet, err := json.Marshal(tier3.GameToWire(quietGames[i]))
		if err != nil {
			t.Fatal(err)
		}
		watched, err := json.Marshal(tier3.GameToWire(watchedGames[i]))
		if err != nil {
			t.Fatal(err)
		}
		if string(quiet) != string(watched) {
			t.Errorf("game %d differs between a watched and a quiet run:\n"+
				"  watched %s\n  quiet   %s", i+1, watched, quiet)
		}
	}
	for _, log := range quiet {
		if log.Board != nil {
			t.Errorf("game %d built a board nobody asked for", log.Game)
		}
		if len(log.Events) != 0 {
			t.Errorf("game %d raised %d beats nobody asked for",
				log.Game, len(log.Events))
		}
	}
}

// What a watched game costs on the wire, stated rather than left to be
// discovered — the same courtesy `ForgeBeatsMax` extends one layer up.
//
// This is a **budget, not a golden**: it fails when a change makes a game's
// board cost several times what it costs today, and says the number so the
// next person does not have to measure it again.
func TestTheBoardsSizeIsStated(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)

	for _, log := range logs {
		raw, err := json.Marshal(log)
		if err != nil {
			t.Fatal(err)
		}
		const budget = 120 * 1024
		t.Logf("game %d: %d beats, %d cards, %d steps, %d bytes on the wire",
			log.Game, len(log.Events), len(log.Board.Cards),
			len(log.Board.Steps), len(raw))
		if len(raw) > budget {
			t.Errorf("game %d costs %d bytes, over the %d-byte budget — the "+
				"job's partial is re-sent on every poll, so this is bandwidth "+
				"a watching room pays continuously", log.Game, len(raw), budget)
		}
	}
}
