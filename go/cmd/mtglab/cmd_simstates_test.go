package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
)

// Three states of the `sim` family nothing reached: a deck whose default
// mulligan rule is actually wrong, a cache file that opens and holds no cache,
// and a match that was played somewhere else.

// deckOfCopies writes a deck as a shape rather than as a list: so many lands,
// so many copies of one spell. Repetition is deliberate -- the gate would
// refuse this deck and the simulator will still run it (an invalid deck is
// simulated rather than refused), because what is being described here is a
// *curve*, not a decklist anybody would play.
func deckOfCopies(t *testing.T, d deployment, slug, commander, spell string, lands, spells int) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "slug: %s\nname: %s\ncommander:\n  - %s\ncards:\n", slug, slug, commander)
	for range lands {
		b.WriteString("  - name: Forest\n    category: land\n    why: x\n")
	}
	for range spells {
		fmt.Fprintf(&b, "  - name: %s\n    category: ramp\n    why: x\n", spell)
	}
	writeSimDeck(t, d, slug, b.String())
}

// The mulligan report's other half: a deck where a different keep rule really
// is better, and the search says which and by how much.
//
// **The branch nobody had a deck for.** Every mulligan test in this suite runs
// the mono-green fixture, which is 95 lands -- its default rule is already the
// best one, so the sweep is flat and the report takes its "NO CHANGE WORTH
// MAKING" arm every time. docs/polish/COVERAGE.md recorded the other arm as
// wanting "a deck the fixture cannot express"; it does not. Flatness is
// measured against *your default*, so all it takes is a deck the default rule
// is wrong for: cheap ramp behind too few lands, where the default's "keep 2-5
// lands and 3 mana pieces" throws away nearly a quarter of its hands for
// nothing.
//
// The numbers are asserted rather than the shape, because a seed is a promise
// (ADR 18): 60 games at seed 7 is one answer, bit for bit, and a report that
// recommended a rule by a margin that moved would be a report nobody could
// act on twice.
func TestTheMulliganReportNamesABetterRuleWhenThereIsOne(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	deckOfCopies(t, d, "lands-light", "Goreclaw, Terror of Qal Sisma", "Sol Ring", 36, 63)

	out, err := d.run(t, "sim", "mulligan", "lands-light",
		"--games", "60", "--seed", "7", "--top", "2")
	if err != nil {
		t.Fatalf("sim mulligan: %v", err)
	}
	t.Logf("sim mulligan output:\n%s", out)

	if strings.Contains(out, "NO CHANGE WORTH MAKING") {
		t.Fatalf("a deck whose default rule throws away a quarter of its "+
			"hands was told to keep it:\n%s", out)
	}
	for _, want := range []string{
		"BEST: keep 1-4 lands AND lands + ramp(mv<=2) >= 2\n",
		"  10.58 spells through turn 8, +0.61 against your default's 9.97,\n",
		"  mulliganing 10% of hands against 23%.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the recommendation is missing %q\n%s", want, out)
		}
	}
	// Twice is the same, which is the whole of what `--seed` promises.
	again, err := d.run(t, "sim", "mulligan", "lands-light",
		"--games", "60", "--seed", "7", "--top", "2")
	if err != nil {
		t.Fatal(err)
	}
	if out != again {
		t.Error("the same seed recommended a different rule the second time")
	}
}

// A cache file that opens and holds no cache degrades instead of failing, and
// claims nothing about what is in it.
//
// The sibling state is already driven -- an `app.db` that is a *directory*,
// where the ladder itself fails -- and this is the one a restore leaves: the
// pragma says the schema is current, so the ladder runs no scripts at all and
// the table the cache needs is simply not there. The recorded behaviour is to
// degrade rather than fail, because a cache that cannot be opened is a cache
// that is not used, never a command that refuses to report.
//
// What it must not do is claim rows or kinds it never read. (It does still
// print `enabled: yes`, because that line reports whether *caching* is switched
// on in this binary rather than whether this file works -- a seam between two
// sentences that is worth knowing about and is not this command's to change.)
func TestACacheFileWithNoCacheInItDegradesRatherThanFailing(t *testing.T) {
	t.Parallel()
	d := claimedSchema(t, scratchDeployment(t))

	out, err := d.run(t, "sim", "cache")
	if err != nil {
		t.Fatalf("`sim cache` failed rather than degrading: %v", err)
	}
	if !strings.Contains(out, "rows:    0 ") {
		t.Errorf("a cache that could not be opened reported rows:\n%s", out)
	}
	// Nothing about what kinds of result are in there, because nothing was
	// read: a kind line over an unopened table is an invented fact.
	if strings.Contains(out, "tier1") || strings.Contains(out, "shelf") {
		t.Errorf("the degraded report named cached kinds:\n%s", out)
	}
	// And clearing it is the same answer rather than a claim about rows it
	// could not have dropped.
	cleared, err := d.run(t, "sim", "cache", "--clear")
	if err != nil {
		t.Fatalf("`sim cache --clear` failed rather than degrading: %v", err)
	}
	if !strings.Contains(cleared, "cleared 0 cached result(s)") {
		t.Errorf("clearing an unopenable cache said:\n%s", cleared)
	}
}

// A match played on the worker says so, and a match played here says the other
// thing.
//
// One word in the ledger's own report, and the only one that answers "why was
// that match slower / why does that match have a Forge version and this one
// not". It is the hosted half that had never rendered, because every `sim
// forge` the CLI runs is local by construction -- the worker's matches arrive
// through the app (ADR 35) and are written with `Hosted: true` by a path no CLI
// test goes through.
func TestTheMatchLedgerSaysWhereAMatchWasPlayed(t *testing.T) {
	t.Parallel()
	d := simHome(t, false)
	path := d.AppDBPath()
	if err := auth.Migrate(path); err != nil {
		t.Fatal(err)
	}
	here, there := oneCardDeck(t, "arahbo", "Arahbo, Roar of the World"),
		oneCardDeck(t, "gyome", "Gyome, Master Chef")

	seat1 := 1
	run := &tier3.SimRun{
		Output: tier3.SimOutput{Games: []tier3.GameResult{
			{Index: 1, Milliseconds: 1500, WinnerSeat: &seat1},
		}},
		Seats:       map[int]string{1: "arahbo", 2: "gyome"},
		WallSeconds: 9.5,
	}
	rec := ledger.NewRecorder(path, nil)
	if id := rec.Record(context.Background(), ledger.Match{
		Run: run, Decks: []*deck.Deck{here, there},
		Clock: 300, GamesRequested: 1, Hosted: true,
	}); id == 0 {
		t.Fatal("the hosted match did not record")
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := d.run(t, "sim", "matches")
	if err != nil {
		t.Fatalf("sim matches: %v", err)
	}
	t.Logf("sim matches output:\n%s", out)
	if !strings.Contains(out, "(worker,") {
		t.Errorf("a match played on the worker was not said to be:\n%s", out)
	}
	if strings.Contains(out, "(local,") {
		t.Errorf("a hosted match was reported as having been played here:\n%s", out)
	}
}

// oneCardDeck is the smallest deck the ledger will record a seat for: a
// commander and one land. The ledger stores names and labels, not lists.
func oneCardDeck(t *testing.T, slug, commander string) *deck.Deck {
	t.Helper()
	d, err := deck.FromText(strings.Join([]string{
		"slug: " + slug,
		"name: " + slug,
		"commander:",
		"  - " + commander,
		"cards:",
		"  - name: Forest",
		"    category: land",
		"    why: x",
		"",
	}, "\n"), slug)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
