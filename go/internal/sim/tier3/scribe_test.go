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

// A token that leaves the battlefield ceases to exist -- rule 111.7 with rule
// 704.5d -- and the board has to say so, because a graveyard full of tokens is
// a zone Magic does not have.
//
// The rule is two-sided and both sides matter here. A dying token *does* go to
// its owner's graveyard: that is why its death triggers, and it is why the
// `dies` beat is still correct to fire. Then a state-based action removes it
// from the game, and it can never move again. So there is no moment at which
// anybody could look and find it in there.
//
// The recording is the right witness. Both decks in it are about tokens, and
// seat one sacrifices Treasures for mana all game -- so the graveyard it ends
// with is exactly the pile that used to be wrong, and it is wrong in the way
// that hurts: thirty tokens burying the handful of real cards somebody is
// reading the zone to find.
func TestATokenLeavingTheBattlefieldGoesToTheEther(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)

	// The recording has to contain the thing being ruled on, or this passes by
	// describing a game that never happened.
	sacrificed := 0
	for _, log := range logs {
		for _, step := range log.Board.Steps {
			for _, change := range step.Changes {
				if change.Zone == tier3.ZoneGraveyard {
					sacrificed++
				}
			}
		}
	}
	if sacrificed == 0 {
		t.Fatal("nothing reached a graveyard in this recording, so it cannot " +
			"say anything about what does")
	}

	// Fold every game to its end and read the zones, the way the browser does.
	for _, log := range logs {
		token := map[int]string{}
		for _, card := range log.Board.Cards {
			if card.Token {
				token[card.ID] = card.Name
			}
		}
		if len(token) == 0 {
			t.Fatalf("game %d made no tokens; both decks are about them",
				log.Game)
		}
		zone := map[int]string{}
		for _, step := range log.Board.Steps {
			for _, change := range step.Changes {
				if change.Zone != "" {
					zone[change.ID] = change.Zone
				}
			}
		}
		for id, name := range token {
			switch zone[id] {
			case tier3.ZoneBattlefield, tier3.ZoneLand, tier3.ZoneGone, "":
				// On the table, or gone from the game. Both are real.
			default:
				t.Errorf("game %d: %s (%d) is drawn in %q -- a token that has "+
					"left the battlefield has ceased to exist and is in no "+
					"zone at all", log.Game, name, id, zone[id])
			}
		}
	}

	// And silencing the zone must not silence the account. The beat is raised
	// from the scribe's line rather than from the board's answer, and that
	// separation is the whole reason this is safe: creatures still die.
	died := 0
	for _, log := range logs {
		for _, e := range log.Events {
			if e.Kind == tier3.EventDies {
				died++
			}
		}
	}
	if died == 0 {
		t.Error("nothing died in either game -- taking tokens out of the " +
			"graveyard must not take the deaths out of the account")
	}

	// The tokens in this recording are Food and Treasure, which are artifacts
	// and therefore do not *die* -- see the rule 700.4 test below. Both facts
	// are true at once and neither implies the other: the token is gone from
	// the zone, and it was never announced as dying.
	for _, log := range logs {
		for _, e := range log.Events {
			if e.Kind == tier3.EventDies && strings.Contains(e.Card, "Token") {
				t.Errorf("game %d: %q was announced as dying, and every token "+
					"in this recording is an artifact", log.Game, e.Card)
			}
		}
	}
}

// Only creatures and planeswalkers die -- rule 700.4 -- and the account has to
// use the word the way the game does.
//
// **This fired for every permanent.** A Commander game narrated "Wooded
// Foothills dies" several times a turn, which teaches somebody learning the
// game the wrong meaning of a real Magic term, and commandment 2 will not have
// it. It hid for as long as it did because the skull it draws landed on the
// *graveyard pile*, where a fetchland's death was one anonymous flicker among
// many; holding the dying card on the sand put the skull on the card, and a
// skull on a land is impossible to miss.
//
// The recording is a good witness: four fetchlands crack in it, a Saga
// finishes, and The Great Henge goes. Every one of those used to die.
func TestOnlyCreaturesAndPlaneswalkersDie(t *testing.T) {
	t.Parallel()
	logs, _ := scribed(t, true)

	// What each name is, taken from the board's own dictionary rather than
	// assumed -- the same source the ruling reads.
	types := map[string]string{}
	for _, log := range logs {
		for _, card := range log.Board.Cards {
			if card.Types != "" {
				types[card.Name] = card.Types
			}
		}
	}

	deaths := 0
	for _, log := range logs {
		for _, e := range log.Events {
			if e.Kind != tier3.EventDies {
				continue
			}
			deaths++
			line := types[e.Card]
			if line == "" {
				t.Errorf("game %d: %q died and the board never said what it "+
					"is", log.Game, e.Card)
				continue
			}
			if !strings.Contains(line, "Creature") &&
				!strings.Contains(line, "Planeswalker") {
				t.Errorf("game %d: %q is %q -- it was sacrificed or "+
					"destroyed, and only creatures and planeswalkers die",
					log.Game, e.Card, line)
			}
		}
	}
	if deaths == 0 {
		t.Fatal("nothing died in two games of Commander, so this test is " +
			"describing a game that did not happen")
	}

	// And the ones that should not have died really are in this recording, so
	// a future change that reopens the hole has something to trip over.
	fetched := false
	for _, log := range logs {
		for _, card := range log.Board.Cards {
			if strings.Contains(card.Types, "Land") &&
				!strings.Contains(card.Types, "Creature") {
				fetched = true
			}
		}
	}
	if !fetched {
		t.Error("no plain land in the recording, so nothing here proves a " +
			"land is not announced as dying")
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
