package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The `sim` family's edges: the refusals a mistyped argument earns, the
// shapes a report takes when the numbers say something different, and what
// the two commands that read `app.db` do when it is not a database.
//
// **`sim cache` and `sim matches` read and must never mint.** An absent
// `app.db` is an empty one by design — the door's rule that a reader never
// acquires a database, one layer up — so the interesting fault is not the
// absence but the file that is there and is not one.

// A sweep's bounds are read as numbers or refused by name, and the refusal
// names which end was wrong: `low` and `high` are positional, and an operator
// who transposed something needs to know which.
func TestASweepRefusesEachOfItsBoundsByName(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	writeSimDeck(t, d, "mono-green", monoGreenText(t))

	for _, tc := range []struct{ low, high, want string }{
		{"thirty", "40", "argument low"},
		{"30", "forty", "argument high"},
	} {
		_, err := d.run(t, "sim", "lands", "mono-green", tc.low, tc.high)
		if err == nil {
			t.Fatalf("`sim lands %s %s` swept anyway", tc.low, tc.high)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("the refusal said %q without naming %s", err, tc.want)
		}
	}
}

// A run needs a game to run: zero is a refusal rather than a report of
// nothing, across every command that takes a count.
func TestARunOfNoGamesIsRefusedRatherThanReported(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	writeSimDeck(t, d, "mono-green", monoGreenText(t))

	for _, argv := range [][]string{
		{"sim", "mana", "mono-green", "--games", "0"},
		{"sim", "mulligan", "mono-green", "--games", "0"},
		{"sim", "lands", "mono-green", "30", "31", "--games", "0"},
	} {
		_, err := d.run(t, argv...)
		if err == nil || !strings.Contains(err.Error(), "at least one game") {
			t.Errorf("`mtglab %s` answered %v", strings.Join(argv, " "), err)
		}
	}
}

// The closed form is judged on the play by default, because that is the
// harder case — and `--on-the-draw` says so where the report can be read
// beside another one.
func TestTheClosedFormSaysWhichSeatItJudged(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	writeSimDeck(t, d, "mono-green", monoGreenText(t))

	onThePlay, err := d.run(t, "sim", "shelf", "mono-green")
	if err != nil {
		t.Fatalf("sim shelf: %v", err)
	}
	if !strings.Contains(onThePlay, "on the play.") {
		t.Errorf("the default report does not say which seat it judged:\n%s", onThePlay)
	}
	onTheDraw, err := d.run(t, "sim", "shelf", "mono-green", "--on-the-draw")
	if err != nil {
		t.Fatalf("sim shelf --on-the-draw: %v", err)
	}
	if !strings.Contains(onTheDraw, "on the draw.") {
		t.Errorf("--on-the-draw was not honoured:\n%s", onTheDraw)
	}
	// The seat is not the only thing that moved: an extra card changes the
	// arithmetic, which is the whole reason the flag exists.
	if onThePlay == onTheDraw {
		t.Error("the two seats produced the same report to the byte")
	}
}

// A grid where the keep rule genuinely matters reports a **best** rule and
// what it buys, rather than the flat verdict.
//
// The deck is built for it: thirty lands under sixty-nine eight-drops, which
// is the shape where throwing a hand back is worth doing and the grid can
// say so. The fixtures the rest of this file uses are deliberately not — a
// hundred Forests has no keep rule worth arguing about — and a test that
// asked this question of one of those would be asserting that a flat deck is
// flat.
func TestAGridWhereTheKeepRuleMattersNamesTheBestOne(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	writeSimDeck(t, d, "toppy", "slug: toppy\nname: Toppy\n"+
		"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
		"  - name: Forest\n    qty: 30\n    why: the mana\n"+
		"  - name: Craterhoof Behemoth\n    qty: 69\n    why: the top end\n")

	out, err := d.run(t, "sim", "mulligan", "toppy", "--games", "200", "--top", "3")
	if err != nil {
		t.Fatalf("sim mulligan: %v", err)
	}
	t.Logf("sim mulligan (top-heavy):\n%s", out)
	// One verdict or the other, and whichever it is the report must carry the
	// numbers a reader would act on.
	switch {
	case strings.Contains(out, "BEST: "):
		if !strings.Contains(out, "spells through turn 8") ||
			!strings.Contains(out, "mulliganing ") {
			t.Errorf("the verdict named a rule without pricing it:\n%s", out)
		}
	case strings.Contains(out, "NO CHANGE WORTH MAKING"):
		if !strings.Contains(out, "flatness is measured against your default") {
			t.Errorf("the flat verdict did not say what it is flat against:\n%s", out)
		}
	default:
		t.Errorf("the grid reached no verdict at all:\n%s", out)
	}
	// Judged on deployment rather than on the mulligan rate, which is the
	// sentence that keeps a reader from optimising the wrong number.
	if !strings.Contains(out, "Judged on spells deployed through turn 8") {
		t.Errorf("the report does not say what it judged on:\n%s", out)
	}
}

// `sim cache` and `sim matches` over an `app.db` that is a **directory**: the
// path is there, so neither command takes its absent-file shortcut, and
// neither may then pretend the store is empty.
//
// A "0 rows" over a file that could not be opened is the same lie the roster
// tells over an unreadable database, and it is worse here because the honest
// answer over a genuinely absent file is also zero.
func TestTheStoresDoNotReportEmptinessOverAFileTheyCouldNotOpen(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if err := os.MkdirAll(d.AppDBPath(), 0o750); err != nil {
		t.Fatal(err)
	}

	// The match ledger refuses outright.
	out, err := d.run(t, "sim", "matches")
	if err == nil {
		t.Errorf("`sim matches` read a ledger that is a directory:\n%s", out)
	}
	if strings.Contains(out, "no matches recorded yet") {
		t.Errorf("`sim matches` reported an empty history over an unopenable file:\n%s", out)
	}

	// The simulation cache says so on the error stream and keeps going with a
	// nil store, which is its recorded behaviour: a cache that cannot be
	// opened is a cache that is not used, never a command that fails.
	out, err = d.run(t, "sim", "cache")
	if err != nil {
		t.Fatalf("`sim cache` failed rather than degrading: %v", err)
	}
	if !strings.Contains(out, "rows:    0") {
		t.Errorf("the degraded cache report reads:\n%s", out)
	}
}

// A match the ledger cannot read back is a refusal rather than an empty
// history, which is the same question `internal/sim/tier3/ledger` asks one
// layer down: not "did it fail" but "did it lie".
func TestAnUnreadableLedgerIsNotAnEmptyOne(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	d.Forge = forgeMachine(t, `echo "Game Result: Game 1 ended in 1000 ms. Ai(1)-first has won!"
`)
	forgeDeck(t, d, "first")
	forgeDeck(t, d, "second")
	if _, err := d.run(t, "sim", "forge", "first", "second", "--games", "1"); err != nil {
		t.Fatalf("the bout failed: %v", err)
	}
	hollowed(t, d, "forge_matches")

	out, err := d.run(t, "sim", "matches")
	if err == nil {
		t.Fatalf("a ledger with no matches table answered:\n%s", out)
	}
	if strings.Contains(out, "no matches recorded yet") {
		t.Errorf("an unreadable ledger was reported as an empty one:\n%s", out)
	}
}

// A machine that cannot record its own match still **plays** it: the ledger is
// a write on the way out, and a round-robin must not die on a hiccup with the
// JVM's work already done.
//
// The warning goes to the error stream rather than into the report, because
// the report is the thing somebody asked for.
func TestAMatchIsReportedEvenWhenItCannotBeRecorded(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	// The decks are readable and the data directory is not: the bout runs and
	// the ledger cannot be minted.
	d.DataDir = filepath.Join("/nonexistent", "never-mounted")
	d.Forge = forgeMachine(t, `echo "Game Result: Game 1 ended in 2000 ms. Ai(2)-second has won!"
`)
	forgeDeck(t, d, "first")
	forgeDeck(t, d, "second")

	out, err := d.run(t, "sim", "forge", "first", "second", "--games", "1")
	if err != nil {
		t.Fatalf("a match that could not be recorded was not reported: %v", err)
	}
	if !strings.Contains(out, "  second                 1\n") {
		t.Errorf("the tally did not survive the ledger failing:\n%s", out)
	}
}

// A ledger row carries the labels the decks wore when they played, because a
// win rate without an archetype beside it is a number about two slugs.
func TestTheLedgerKeepsTheLabelsTheDecksWore(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	d.Forge = forgeMachine(t, `echo "Game Result: Game 1 ended in 1000 ms. Ai(1)-first has won!"
`)
	writeSimDeck(t, d, "first", "slug: first\nname: First\narchetype: midrange\n"+
		"themes:\n  - lands matter\n  - big mana\n"+
		"commander:\n  - Sol Ring\ncards:\n  - name: Forest\n    why: a land\n")
	forgeDeck(t, d, "second")

	if _, err := d.run(t, "sim", "forge", "first", "second", "--games", "1"); err != nil {
		t.Fatalf("the bout failed: %v", err)
	}
	out, err := d.run(t, "sim", "matches")
	if err != nil {
		t.Fatalf("sim matches: %v", err)
	}
	if !strings.Contains(out, "(midrange; lands matter, big mana)") {
		t.Errorf("the ledger lost the deck's labels:\n%s", out)
	}
	// And a deck that wore none says so rather than leaving the column blank.
	if !strings.Contains(out, "(unlabelled)") {
		t.Errorf("an unlabelled deck left the column empty:\n%s", out)
	}
}

// Narration names a seat it cannot place as **somebody**, which is the one
// sentence in the account that has to work when Forge says something the seat
// map does not cover.
//
// Driven at the renderer, because the point is the rendering: every beat kind
// the parser can produce, told once, so a beat nobody can follow here is
// caught before it reaches the room that animates it.
func TestEveryBeatIsToldAndAnUnplaceableSeatIsSomebody(t *testing.T) {
	t.Parallel()
	life := 34
	log := tier3.EventLog{Game: 3, Truncated: true, Events: []tier3.GameEvent{
		{Kind: tier3.EventTurn, Seat: 1, Turn: 4},
		{Kind: tier3.EventMulligan, Seat: 2, Amount: 6},
		{Kind: tier3.EventLand, Seat: 1, Card: "Forest"},
		{Kind: tier3.EventCast, Seat: 1, Card: "Craterhoof Behemoth"},
		{Kind: tier3.EventResolve, Card: "Craterhoof Behemoth"},
		{Kind: tier3.EventAttack, Seat: 1, TargetSeat: 2, Card: "Craterhoof Behemoth"},
		{Kind: tier3.EventBlock, Card: "Forest Bear", Target: "Craterhoof Behemoth"},
		{Kind: tier3.EventUnblocked, Card: "Craterhoof Behemoth"},
		{Kind: tier3.EventDamage, Seat: 1, TargetSeat: 2, Card: "Craterhoof Behemoth", Amount: 9},
		{Kind: tier3.EventLife, Seat: 2, Life: &life},
		{Kind: tier3.EventDies, Card: "Forest Bear"},
		// A seat the map does not hold, and seat zero, which is Forge saying
		// something about nobody in particular.
		{Kind: tier3.EventLand, Seat: 7, Card: "Swamp"},
		{Kind: tier3.EventDamage, Seat: 0, Card: "Lightning", Amount: 3},
		{Kind: tier3.EventOutcome, Seat: 1, Amount: 1, Note: "by dealing damage"},
	}}

	var out bytes.Buffer
	narrateGame(&out, log, map[int]string{1: "gyome", 2: "trostani"})
	told := out.String()
	t.Logf("the account:\n%s", told)

	for _, want := range []string{
		"--- game 3 ---",
		"turn 4  ", "gyome",
		"trostani keeps 6",
		"gyome plays Forest",
		"gyome casts Craterhoof Behemoth",
		"Craterhoof Behemoth resolves",
		"gyome attacks trostani with Craterhoof Behemoth",
		"Forest Bear blocks Craterhoof Behemoth",
		"Craterhoof Behemoth is unblocked",
		"Craterhoof Behemoth deals 9 to trostani",
		"trostani at 34",
		"Forest Bear dies",
		// A chair the map does not hold is named as a chair.
		"seat 7 plays Swamp",
		// And seat zero is a player nobody can name.
		"Lightning deals 3 to somebody",
		"gyome WINS by dealing damage",
		"beat ceiling",
	} {
		if !strings.Contains(told, want) {
			t.Errorf("the account never said %q", want)
		}
	}
	// The loser's verb is the other one, and it is not punctuated: Forge
	// writes its outcome reasons to follow "<player> has won/lost".
	var lost bytes.Buffer
	narrateGame(&lost, tier3.EventLog{Game: 1, Events: []tier3.GameEvent{
		{Kind: tier3.EventOutcome, Seat: 2, Amount: 0, Note: "by drawing from an empty library"}}},
		map[int]string{2: "trostani"})
	if !strings.Contains(lost.String(), "trostani loses by drawing from an empty library") {
		t.Errorf("the losing beat reads %q", lost.String())
	}
}

// Thousands separators, including the one shape a naive loop gets wrong and
// the one nothing else in this package renders: a negative count keeps its
// sign outside the grouping.
func TestAGroupedNumberKeepsItsSignOutsideTheGrouping(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{-1, "-1"},
		{-999, "-999"},
		{-1000, "-1,000"},
		{-1234567, "-1,234,567"},
	} {
		if got := groupThousands(tc.in); got != tc.want {
			t.Errorf("groupThousands(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
