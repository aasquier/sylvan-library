package main

import (
	"math/big"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier1"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The `sim` family's guards, and the one write it makes.
//
// `sim_test.go` drives the happy paths and holds the tables. What is here is
// the arithmetic nobody asks for on purpose -- a run of zero games, a range
// that will not parse -- and the match ledger's own write, which is the only
// place in the CLI that **mints `app.db` rather than refusing without one**.
// That distinction is deliberate and argued in the code: `sim cache` will not
// create a database, because reading must not acquire one, while a match
// silently unrecorded on a fresh machine would be a regression.

// A run of no games is arithmetic about nothing, and every command that takes
// `--games` says so rather than dividing by zero somewhere further in.
func TestEverySimCommandRefusesARunOfNoGames(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "mono-green", monoGreenText(t))

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"mana", []string{"mana", "mono-green", "--games", "0"}},
		{"mana, negative", []string{"mana", "mono-green", "--games", "-5"}},
		{"lands", []string{"lands", "mono-green", "30", "40", "--games", "0"}},
		{"mulligan", []string{"mulligan", "mono-green", "--games", "0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runSim(t, tc.args...)
			if err == nil {
				t.Fatalf("a run of no games was attempted:\n%s", out)
			}
			if !strings.Contains(err.Error(), "at least one game") {
				t.Errorf("the refusal said %q", err)
			}
		})
	}
}

// A land range that will not parse names which of the two bounds was wrong,
// because "invalid int value" without a name sends somebody to check both.
func TestTheLandRangeNamesWhichBoundWouldNotParse(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "mono-green", monoGreenText(t))

	for _, tc := range []struct{ name, low, high, wants string }{
		{"the low bound", "thirty", "40", "argument low"},
		{"the high bound", "30", "forty", "argument high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runSim(t, "lands", "mono-green", tc.low, tc.high)
			if err == nil {
				t.Fatal("an unparseable bound was swept")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("the refusal said %q, want it to name %s", err, tc.wants)
			}
			// And it quotes what it read, so the caller sees the typo.
			if !strings.Contains(err.Error(), "invalid int value") {
				t.Errorf("the refusal said %q", err)
			}
		})
	}
}

// The match ledger's write is the CLI's one minting write, and it never fails
// the caller: the match has already been played and the JVM minutes are
// already spent, so a ledger that cannot be written is a warning rather than
// a lost result.
func TestTheMatchLedgerRecordsAFinishedMatchAndThenRendersIt(t *testing.T) {
	simHome(t, false)

	// A finished match, built by hand rather than played -- what is under
	// test is the recording and the rendering, not Forge.
	winner := "gyome"
	seat := 1
	turns := 9
	run := &tier3.SimRun{
		Output: tier3.SimOutput{Games: []tier3.GameResult{
			{Index: 0, Milliseconds: 12000, Winner: &winner, WinnerSeat: &seat, Turns: &turns},
			{Index: 1, Milliseconds: 8000, Draw: true},
		}},
		WallSeconds:  30,
		Seats:        map[int]string{1: "gyome", 2: "trostani"},
		ForgeVersion: "1.6.50",
	}
	decks := []*deck.Deck{
		{Slug: "gyome", Name: "Gyome"},
		{Slug: "trostani", Name: "Trostani"},
	}

	recordForgeMatch(run, decks, big.NewInt(7), 300, 2)

	// The ledger now renders it, which is the only way to see that the write
	// landed -- and the render is what a maintainer actually reads.
	out, err := runSim(t, "matches")
	if err != nil {
		t.Fatalf("matches: %v", err)
	}
	for _, want := range []string{"gyome", "trostani"} {
		if !strings.Contains(out, want) {
			t.Errorf("the ledger does not mention %q:\n%s", want, out)
		}
	}
	// The instrument is recorded beside the result (ADR 36): ratings mixed
	// across a Forge upgrade would silently blend two judges.
	if !strings.Contains(out, "1.6.50") {
		t.Errorf("the ledger does not record which Forge played:\n%s", out)
	}
}

// A ledger write that cannot happen is a warning, never a failure -- the
// match is already paid for by the time there is anything to record.
func TestAFailedLedgerWriteDoesNotTakeTheMatchWithIt(t *testing.T) {
	// A data directory that is not there: the ladder cannot run.
	t.Setenv("MTGLAB_DATA_DIR", "/nonexistent/never-mounted")
	t.Setenv("MTGLAB_DECKS_DIR", "/nonexistent/never-mounted/decks")

	run := &tier3.SimRun{
		Output:      tier3.SimOutput{Games: []tier3.GameResult{{Index: 0, Milliseconds: 1000}}},
		WallSeconds: 5,
		Seats:       map[int]string{1: "gyome"},
	}
	// The whole contract: it returns rather than panicking or exiting.
	recordForgeMatch(run, []*deck.Deck{{Slug: "gyome", Name: "Gyome"}},
		big.NewInt(1), 300, 1)
}

// `sim matches` reads and must never mint: an empty instance is told there
// are no matches rather than having a database made for it. (`sim cache` has
// the same rule; the recording write above is the deliberate exception.)
func TestReadingTheLedgerOnAFreshMachineMintsNothing(t *testing.T) {
	dir := simHome(t, false)

	out, err := runSim(t, "matches")
	if err != nil {
		t.Fatalf("matches on a fresh machine: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("an empty ledger printed nothing at all")
	}
	if fileExists(dir + "/app.db") {
		t.Error("reading the ledger created app.db -- a read must not acquire a database")
	}
}

// The rendering helpers are what every table in this family is built from,
// and each has a boundary a naive version gets wrong.
func TestTheTableHelpersHandleTheirBoundaries(t *testing.T) {
	t.Parallel()

	// A column narrower than its content is not truncated: a truncated card
	// name in a terminal is worse than a ragged column.
	if got := padRight("Craterhoof Behemoth", 5); got != "Craterhoof Behemoth" {
		t.Errorf("padRight truncated to %q", got)
	}
	if got := padRight("ab", 5); got != "ab   " {
		t.Errorf("padRight(ab, 5) = %q", got)
	}
	if got := padRight("", 3); got != "   " {
		t.Errorf("padRight of nothing = %q", got)
	}
	if got := padRight("abc", 3); got != "abc" {
		t.Errorf("an exact fit became %q", got)
	}

	// A number nobody has renders as the word, never as a zero: zero turns
	// and "the commander never landed" are different facts, and a table that
	// printed 0 for the second would read as the fastest deck in the pod.
	if got := numberOrNone(nil); got != "None" {
		t.Errorf("numberOrNone(nil) = %q", got)
	}
	if got := numberOrNone(&tier1.Number{Int: 5}); got != "5" {
		t.Errorf("an integer rendered as %q", got)
	}
	// A float renders through the determinism kernel's own repr rather than
	// through %v, because the recorded goldens hold those bytes.
	if got := numberOrNone(&tier1.Number{IsFloat: true, Float: 5.5}); !strings.Contains(got, "5.5") {
		t.Errorf("a float rendered as %q", got)
	}
	if got := floatOrNone(nil); got != "None" {
		t.Errorf("no value rendered as %q -- zero and nothing are different", got)
	}
	f := 1.5
	if got := floatOrNone(&f); !strings.Contains(got, "1.5") {
		t.Errorf("floatOrNone(1.5) = %q", got)
	}

	// Thousands separators, at the boundary a naive loop gets wrong.
	for _, tc := range []struct {
		in   int
		want string
	}{{0, "0"}, {999, "999"}, {1000, "1,000"}, {20000, "20,000"}} {
		if got := groupThousands(tc.in); got != tc.want {
			t.Errorf("groupThousands(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
