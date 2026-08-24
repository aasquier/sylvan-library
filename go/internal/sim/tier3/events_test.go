package tier3

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The event parser's cases are written by hand from real Forge output rather
// than recorded from the parser, deliberately: a golden captured by running
// the code under test agrees with it by construction and would have agreed
// with every bug it ever had. `testdata/narrated-game.log` is a real
// `sim` run (Arahbo vs Goreclaw, seed 12345, Forge 2.0.14) and is asserted
// against for what the *story* has to hold, not for a byte dump.

func TestTheEventParserReadsTheBeatsItClaims(t *testing.T) {
	t.Parallel()

	// The player label is the real one: `Ai(<seat>)-<deck name>`, where the
	// name is whatever `[metadata] name=` said. These carry commas and an em
	// dash on purpose — a pattern that assumed a tidy label would pass a test
	// written with a tidy label and fail on every real match.
	const p1 = "Ai(1)-Arahbo, Roar of the World — Cats"
	const p2 = "Ai(2)-Goreclaw, Terror of Qal Sisma — Mono-Green Stompy"

	life := func(n int) *int { return &n }

	for _, tc := range []struct {
		name string
		line string
		want GameEvent
	}{
		{"a turn names its active player",
			"Turn: Turn 6 (" + p2 + ")",
			GameEvent{Kind: EventTurn, Seat: 2}},
		{"a kept hand",
			"Mulligan: " + p1 + " has kept a hand of 7 cards",
			GameEvent{Kind: EventMulligan, Seat: 1, Amount: 7}},
		{"a land drop, with the instance id discarded",
			"Land: " + p1 + " played Hushwood Verge (99)",
			GameEvent{Kind: EventLand, Seat: 1, Card: "Hushwood Verge"}},
		{"a cast spell",
			"Add To Stack: " + p2 + " cast Fauna Shaman",
			GameEvent{Kind: EventCast, Seat: 2, Card: "Fauna Shaman"}},
		{"a cast spell that names a target keeps only the spell",
			"Add To Stack: " + p1 + " cast Arahbo, Roar of the World targeting [Kaheera, the Orphanguard (100)]",
			GameEvent{Kind: EventCast, Seat: 1, Card: "Arahbo, Roar of the World"}},
		{"a permanent resolving",
			"Resolve Stack: Fauna Shaman - Creature 2 / 2",
			GameEvent{Kind: EventResolve, Card: "Fauna Shaman"}},
		{"an attack names both ends",
			"Combat: " + p2 + " assigned Fauna Shaman (126) to attack " + p1 + ".",
			GameEvent{Kind: EventAttack, Seat: 2, Card: "Fauna Shaman", TargetSeat: 1}},
		{"a block names the blocker and what it blocked",
			"Combat: " + p2 + " assigned Goreclaw, Terror of Qal Sisma (203) to block Pride Sovereign (82).",
			GameEvent{Kind: EventBlock, Seat: 2, Card: "Goreclaw, Terror of Qal Sisma",
				Target: "Pride Sovereign"}},
		{"combat damage to a player",
			"Damage: Fauna Shaman (126) deals 2 combat damage to " + p1 + ".",
			GameEvent{Kind: EventDamage, Card: "Fauna Shaman", Amount: 2, TargetSeat: 1}},
		{"damage to a creature, which is not combat damage and says so",
			"Damage: Pride Sovereign (82) deals 14 damage to Goreclaw, Terror of Qal Sisma (203).",
			GameEvent{Kind: EventDamage, Card: "Pride Sovereign", Amount: 14,
				Target: "Goreclaw, Terror of Qal Sisma"}},
		{"a life change carries the total after and the swing",
			"Life: Life: " + p1 + " 40 > 38",
			GameEvent{Kind: EventLife, Seat: 1, Amount: -2, Life: life(38)}},
		{"a lethal life change goes negative",
			"Life: Life: " + p2 + " 11 > -11",
			GameEvent{Kind: EventLife, Seat: 2, Amount: -22, Life: life(-11)}},
		{"a creature dying",
			"Zone Change: Sakura-Tribe Elder (188) was put into Graveyard from Battlefield.",
			GameEvent{Kind: EventDies, Card: "Sakura-Tribe Elder"}},
		{"a win, with Forge's own reason",
			"Game Outcome: " + p1 + " has won because all opponents have lost",
			GameEvent{Kind: EventOutcome, Seat: 1, Amount: 1,
				Note: "all opponents have lost"}},
		{"a loss",
			"Game Outcome: " + p2 + " has lost because life total reached 0",
			GameEvent{Kind: EventOutcome, Seat: 2, Amount: 0,
				Note: "life total reached 0"}},

		// The wrinkles, each found in real output and each the reason its
		// pattern is shaped the way it is.
		{"an unblocked attacker, with the Combat prefix Forge gives the first of a group",
			"Combat: " + p1 + " didn't block Fauna Shaman (126).",
			GameEvent{Kind: EventUnblocked, Seat: 1, Card: "Fauna Shaman"}},
		{"an unblocked attacker with no prefix, which is every one after the first",
			p2 + " didn't block Kaheera, the Orphanguard (100).",
			GameEvent{Kind: EventUnblocked, Seat: 2, Card: "Kaheera, the Orphanguard"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := NewEventParser()
			if done := p.Feed(tc.line); done != nil {
				t.Fatalf("a single line finished a game: %+v", done)
			}
			if len(p.events) != 1 {
				t.Fatalf("read %d events from %q, want 1", len(p.events), tc.line)
			}
			got := p.events[0]
			got.Turn = 0 // supplied by the surrounding game, not by the line
			if !sameEvent(got, tc.want) {
				t.Errorf("read %s\n got %+v\nwant %+v", tc.line, got, tc.want)
			}
		})
	}
}

// sameEvent compares two events including the pointer field, which `==` would
// compare by address.
func sameEvent(a, b GameEvent) bool {
	if (a.Life == nil) != (b.Life == nil) {
		return false
	}
	if a.Life != nil && *a.Life != *b.Life {
		return false
	}
	a.Life, b.Life = nil, nil
	return a == b
}

func TestTheEventParserIgnoresWhatItCannotRead(t *testing.T) {
	t.Parallel()

	// Every one of these is a real line from a real run. Forge interleaves its
	// card database, its AI's complaints and its own boot log with the game,
	// and a parser that tried to model all of it would break on the first
	// release that printed something new.
	for _, line := range []string{
		"Phase: Ai(1)-Arahbo, Roar of the World — Cats' Untap step",
		"Mana: Forest (136) - {T}: Add {G}.",
		"Add To Stack: Ai(2)-Goreclaw — Stompy triggered Mosswort Bridge",
		"Add To Stack: Ai(2)-Goreclaw — Stompy activated Sakura-Tribe Elder",
		"Resolve Stack: Whenever a creature you control attacks, you may put a quest counter on Beastmaster Ascension.",
		"Resolve Stack: Survival of the Fittest",
		"Replacement Effect: Castle Garenbrig enters tapped unless you control a Forest.",
		"The card A-Unholy Heat was not assigned to any set. Adding it to UNKNOWN set...",
		"Read cards: 33617 archived files in 1 ms (25 parts) using thread pool",
		"15:53:32 [INFO ] GuiBase: APP: Forge v.2.0.14-SNAPSHOT-08.08",
		"Player Control: Ai(1)-Arahbo — Cats has restored control over themself",
		"Zone Change: Kaheera, the Orphanguard has moved from Command Zone to Ai(1)-Arahbo — Cats's hand.",
		"",
	} {
		p := NewEventParser()
		if done := p.Feed(line); done != nil {
			t.Errorf("%q finished a game", line)
		}
		if len(p.events) != 0 {
			t.Errorf("%q read as %+v, want nothing", line, p.events[0])
		}
	}
}

// narratedGame is the recorded run every story assertion below reads.
func narratedGame(t *testing.T) []EventLog {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "narrated-game.log"))
	if err != nil {
		t.Fatalf("reading the recorded game: %v", err)
	}
	logs := ParseEvents(string(raw))
	if len(logs) != 1 {
		t.Fatalf("the recording holds %d games, want 1", len(logs))
	}
	return logs
}

func TestARecordedGameTellsACoherentStory(t *testing.T) {
	t.Parallel()

	log := narratedGame(t)[0]
	if log.Game != 1 {
		t.Errorf("game = %d, want 1", log.Game)
	}
	if log.Truncated {
		t.Error("a 477-line game tripped the cap")
	}
	// The whole point of the exercise: a nine-turn game is about a hundred
	// beats, not five hundred lines. A parser that started matching phase
	// steps would sail past this.
	if len(log.Events) < 80 || len(log.Events) > 160 {
		t.Errorf("read %d events, want a hundred or so", len(log.Events))
	}

	var (
		turns    []int
		seenTurn bool
		wins     int
		losses   int
	)
	for i, e := range log.Events {
		switch e.Kind {
		case EventMulligan:
			if seenTurn {
				t.Errorf("event %d: a mulligan after the game began", i)
			}
			if e.Turn != 0 {
				t.Errorf("event %d: a mulligan on turn %d", i, e.Turn)
			}
		case EventTurn:
			seenTurn = true
			turns = append(turns, e.Turn)
		case EventOutcome:
			if e.Amount == 1 {
				wins++
			} else {
				losses++
			}
		}
		if seenTurn && e.Turn == 0 {
			t.Errorf("event %d (%s) carries no turn after the game began", i, e.Kind)
		}
		if e.Seat < 0 || e.Seat > 2 {
			t.Errorf("event %d: seat %d in a two-handed game", i, e.Seat)
		}
	}

	// Turns are announced in order and start at one. Forge counts a turn per
	// player rather than per round, which is why seventeen of them fit in a
	// game the result line calls turn nine.
	if len(turns) == 0 || turns[0] != 1 {
		t.Fatalf("turns began at %v, want 1 first", turns)
	}
	for i := 1; i < len(turns); i++ {
		if turns[i] != turns[i-1]+1 {
			t.Errorf("turn %d followed turn %d", turns[i], turns[i-1])
		}
	}
	if wins != 1 || losses != 1 {
		t.Errorf("the game ended %d-%d, want exactly one of each", wins, losses)
	}
}

func TestARecordedGameKillsSomebodyOnCamera(t *testing.T) {
	t.Parallel()

	// The story the theater exists to show, and the arithmetic that proves
	// three separate patterns read the right halves of their lines.
	//
	// **Forge resolves a whole combat before it moves anybody's life.** Three
	// attackers dealing 11, 11 and nothing produce three `Damage:` lines and
	// one `Life:` line for -22 — so the invariant is not per blow, it is per
	// combat: the damage dealt to a seat since its last life change sums to
	// exactly that change. The first version of this test asserted the blow
	// and failed against a real game, which is the recording earning its keep.
	log := narratedGame(t)[0]

	pending := map[int]int{}
	checked := 0
	for i, e := range log.Events {
		switch e.Kind {
		case EventDamage:
			// Damage to a creature is not damage to a player; only the seat
			// form moves a life total.
			if e.TargetSeat != 0 {
				pending[e.TargetSeat] += e.Amount
			}
		case EventLife:
			dealt := pending[e.Seat]
			pending[e.Seat] = 0
			// A life change with no damage behind it is somebody paying a
			// cost or gaining life, and is not this test's business.
			if dealt == 0 {
				continue
			}
			if e.Amount != -dealt {
				t.Errorf("event %d: seat %d took %d damage, life moved %d",
					i, e.Seat, dealt, e.Amount)
			}
			checked++
		}
	}
	if checked < 3 {
		t.Fatalf("only %d life changes were explained by damage; the recorded "+
			"game has several", checked)
	}

	// It ends where the result line said it did.
	last := log.Events[len(log.Events)-1]
	if last.Kind != EventOutcome {
		t.Errorf("the last beat is %s, want the outcome", last.Kind)
	}
	if !strings.Contains(narratedTail(log), "life total reached 0") {
		t.Error("the recorded game was won on life; the reason did not survive")
	}
}

func narratedTail(log EventLog) string {
	var notes []string
	for _, e := range log.Events {
		if e.Kind == EventOutcome {
			notes = append(notes, e.Note)
		}
	}
	return strings.Join(notes, " | ")
}

func TestGamesAreNumberedAsTheyFinish(t *testing.T) {
	t.Parallel()

	// Two games' worth of beats around two result lines. The parser is per
	// run, not per game: it has to close one game and open the next without
	// carrying a turn number or an event across the seam.
	text := strings.Join([]string{
		"Mulligan: Ai(1)-A has kept a hand of 7 cards",
		"Turn: Turn 1 (Ai(1)-A)",
		"Land: Ai(1)-A played Forest (1)",
		"Game Result: Game 1 ended in 5000 ms. Ai(1)-A has won!",
		"Turn: Turn 1 (Ai(2)-B)",
		"Land: Ai(2)-B played Island (2)",
		"Game Result: Game 2 ended in a Draw! Took 900 ms.",
	}, "\n")

	logs := ParseEvents(text)
	if len(logs) != 2 {
		t.Fatalf("read %d games, want 2", len(logs))
	}
	if logs[0].Game != 1 || logs[1].Game != 2 {
		t.Errorf("games numbered %d and %d", logs[0].Game, logs[1].Game)
	}
	if len(logs[0].Events) != 3 {
		t.Errorf("game 1 kept %d beats, want 3", len(logs[0].Events))
	}
	// The second game's first beat is its own turn 1, not a continuation.
	if got := logs[1].Events[0]; got.Kind != EventTurn || got.Turn != 1 || got.Seat != 2 {
		t.Errorf("game 2 opened with %+v", got)
	}
	// A drawn game still closes and still carries its beats.
	if len(logs[1].Events) != 2 {
		t.Errorf("game 2 kept %d beats, want 2", len(logs[1].Events))
	}
}

func TestTheEventCapIsNeverSilent(t *testing.T) {
	t.Parallel()

	// A game that outruns the cap keeps its opening and says so. The failure
	// this forbids is a short list that looks complete — the same class of
	// silence the coverage pre-flight exists to prevent one layer down.
	p := NewEventParser()
	for i := 0; i < EventCap+50; i++ {
		p.Feed("Land: Ai(1)-A played Forest (1)")
	}
	done := p.Feed("Game Result: Game 1 ended in 5000 ms. Ai(1)-A has won!")
	if done == nil {
		t.Fatal("the result line did not finish the game")
	}
	if len(done.Events) != EventCap {
		t.Errorf("kept %d beats, want the cap of %d", len(done.Events), EventCap)
	}
	if !done.Truncated {
		t.Error("the log was cut and did not say so")
	}
}
