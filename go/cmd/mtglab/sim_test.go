package main

// The `mtglab sim` family, driven the way a terminal drives it: SetArgs,
// Execute, and os.Stdout captured through a pipe -- because the commands
// print plainly, and the tables they print are the product.
//
// The fixture is the same one the rest of the suite stands on: the 21-card
// pool (`pooltest`) and the mono-green 99 the gate corpus carries.
// Everything seeded is asserted
// deterministic by running it twice, which is the promise `--seed` makes.

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	simcache "github.com/aasquier/sylvan-library/go/internal/sim/cache"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
)

// simHome points MTGLAB_DATA_DIR and MTGLAB_DECKS_DIR at a scratch tree, so
// no command in this file can see a real library. With `withPool` the
// 21-card DuckDB lands where `config.DBPath()` will look for it.
func simHome(t *testing.T, withPool bool) string {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("MTGLAB_DATA_DIR", dataDir)
	t.Setenv("MTGLAB_DECKS_DIR", filepath.Join(dataDir, "decks"))
	if withPool {
		raw, err := os.ReadFile(pooltest.Build(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dataDir, "mtg.duckdb"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dataDir
}

func writeSimDeck(t *testing.T, slug, text string) {
	t.Helper()
	dir := filepath.Join(settings().DecksDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deck.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// monoGreenText is the gate corpus's mono-green 99 -- Goreclaw over the
// 21-card pool, 95 Forests, and the deliberately banned Titan.
func monoGreenText(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "gate",
		"testdata", "mono-green.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// runSim executes one `mtglab sim ...` invocation on a fresh command tree and
// returns what it printed to os.Stdout. cobra's own chatter (usage on a
// refusal) is discarded; the commands themselves print plainly.
func runSim(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := simCommand()
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	read := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		read <- string(b)
	}()
	runErr := cmd.Execute()
	os.Stdout = old
	_ = w.Close()
	out := <-read
	_ = r.Close()
	return out, runErr
}

// ------------------------------------------------------------------- mana

func TestSimManaReportsTheGoldfish(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "mono-green", monoGreenText(t))

	out, err := runSim(t, "mana", "mono-green", "--games", "30", "--turns", "6", "--seed", "7")
	if err != nil {
		t.Fatalf("sim mana: %v", err)
	}
	t.Logf("sim mana output:\n%s", out)

	for _, want := range []string{
		"games=30  turns=6\n",
		"mulligan policy: keep 2-5 lands AND lands + ramp(mv<=2) >= 3\n",
		"median commander turn: ",
		"commander never cast by T6: ",
		"turns with a color-only block: ",
		"median first spell: T",
		"stalled turns (castless with a spell in hand): ",
		"\n  turn   lands   mana   unused   spells   P(commander down)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q\n%s", want, out)
		}
	}
	// Header block (8 lines), a blank, the table header, then one row per
	// turn -- the recorded report's shape exactly.
	if lines := strings.Split(strings.TrimRight(out, "\n"), "\n"); len(lines) != 10+6 {
		t.Errorf("report has %d lines, want %d\n%s", len(lines), 16, out)
	}

	// A seed is a promise: the same run twice is the same text twice.
	again, err := runSim(t, "mana", "mono-green", "--games", "30", "--turns", "6", "--seed", "7")
	if err != nil {
		t.Fatalf("second sim mana: %v", err)
	}
	if out != again {
		t.Errorf("seeded runs differ:\n%s\n----\n%s", out, again)
	}

	// Unseeded still answers -- it is merely not reproducible.
	if _, err := runSim(t, "mana", "mono-green", "--games", "10", "--turns", "4"); err != nil {
		t.Fatalf("unseeded sim mana: %v", err)
	}
}

func TestSimManaRefusesAMissingDeck(t *testing.T) {
	simHome(t, false)
	_, err := runSim(t, "mana", "nope")
	want := "no deck at " + filepath.Join(settings().DecksDir, "nope", "deck.yaml")
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestSimManaRefusesWithoutThePool(t *testing.T) {
	simHome(t, false)
	writeSimDeck(t, "mono-green", monoGreenText(t))
	_, err := runSim(t, "mana", "mono-green")
	want := "simulation needs the card pool -- run `mtglab data refresh` first"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// A deck where not one name resolves is refused as PoolRequired -- the wart
// `internal/sim/compile` pins and argues, and the closest thing to a
// "nothing to simulate" refusal this surface has. `NothingToSimulate` itself
// is UNREACHABLE from the CLI, deliberately: `sim mana` compiles through
// `compile.Deck`, which never returns it -- only `compile.Compile` does,
// and the CLI never calls that.
func TestSimManaRefusesADeckThePoolCannotSee(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "nobody", strings.Join([]string{
		"slug: nobody",
		"name: Nobody",
		"commander:",
		"  - No Such Card At All",
		"cards: []",
		"",
	}, "\n"))
	_, err := runSim(t, "mana", "nobody")
	want := "simulation needs the card pool -- run `mtglab data refresh` first"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// ------------------------------------------------------------------ lands

func TestSimLandsSweepsTheRange(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "mono-green", monoGreenText(t))

	out, err := runSim(t, "lands", "mono-green", "30", "32", "--games", "15")
	if err != nil {
		t.Fatalf("sim lands: %v", err)
	}
	t.Logf("sim lands output:\n%s", out)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != " lands  P(cmdr T5)  spells thru T8  wasted thru T8  mull%" {
		t.Errorf("header = %q", lines[0])
	}
	for i, count := range []string{"30", "31", "32"} {
		if !strings.HasPrefix(lines[1+i], "    "+count+"     ") {
			t.Errorf("row %d = %q, want a %s-land row", i, lines[1+i], count)
		}
	}
	if want := "Pick the land count where 'spells thru T8' plateaus -- past that " +
		"you are buying commander speed with flood."; lines[len(lines)-1] != want {
		t.Errorf("footer = %q", lines[len(lines)-1])
	}

	again, err := runSim(t, "lands", "mono-green", "30", "32", "--games", "15")
	if err != nil {
		t.Fatalf("second sim lands: %v", err)
	}
	if out != again {
		t.Errorf("the sweep is seeded by default (7) and must repeat:\n%s\n----\n%s", out, again)
	}
}

// A backwards range (low > high) sweeps nothing and refuses nothing:
// header, footer, exit 0. The recorded behaviour, kept rather than
// improved -- a refusal here would be an invention.
func TestSimLandsEmptyRangeIsHeaderAndFooterOnly(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "mono-green", monoGreenText(t))
	out, err := runSim(t, "lands", "mono-green", "32", "30", "--games", "5")
	if err != nil {
		t.Fatalf("sim lands: %v", err)
	}
	want := " lands  P(cmdr T5)  spells thru T8  wasted thru T8  mull%\n" +
		"\nPick the land count where 'spells thru T8' plateaus -- past that " +
		"you are buying commander speed with flood.\n"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestSimLandsRefusesADeckWithNoLands(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "no-lands", strings.Join([]string{
		"slug: no-lands",
		"name: No Lands",
		"commander:",
		"  - Goreclaw, Terror of Qal Sisma",
		"cards:",
		"  - name: Sol Ring",
		"    category: ramp",
		"    why: fixture",
		"",
	}, "\n"))
	_, err := runSim(t, "lands", "no-lands", "30", "40", "--games", "5")
	if err == nil || err.Error() != "deck has no lands to sweep" {
		t.Fatalf("err = %v, want the no-lands refusal", err)
	}
}

func TestSimLandsRefusesANonIntegerBound(t *testing.T) {
	simHome(t, false)
	_, err := runSim(t, "lands", "whatever", "a", "30")
	if err == nil || err.Error() != "argument low: invalid int value: 'a'" {
		t.Fatalf("err = %v", err)
	}
}

// ------------------------------------------------------------------ shelf

func TestSimShelfReadsTheClosedForm(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "mono-green", monoGreenText(t))

	out, err := runSim(t, "shelf", "mono-green")
	if err != nil {
		t.Fatalf("sim shelf: %v", err)
	}
	t.Logf("sim shelf output:\n%s", out)

	lines := strings.Split(out, "\n")
	if lines[0] != "Mono-Green Fixture" {
		t.Errorf("line 0 = %q", lines[0])
	}
	if lines[1] != "99 cards, 95 lands, judged at 90% consistency, on the play." {
		t.Errorf("line 1 = %q", lines[1])
	}
	for _, want := range []string{
		"COLOURED SOURCES -- what your own cards demand",
		"  G: you have ",
		" pip on T",
		"LAND COUNT -- a regression, not a simulation",
		"  You run 95. The fit says ",
		"LATEST CARDS -- cost against when the mana is actually there",
		"cost  on curve  reliable    lag",
		"Goreclaw, Terror of Qal Sisma",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("shelf is missing %q\n%s", want, out)
		}
	}

	// The closed form is arithmetic: no seed, so twice is identical.
	again, err := runSim(t, "shelf", "mono-green")
	if err != nil {
		t.Fatalf("second sim shelf: %v", err)
	}
	if out != again {
		t.Error("the closed form moved between two reads")
	}

	drawn, err := runSim(t, "shelf", "mono-green", "--on-the-draw", "--target", "0.8")
	if err != nil {
		t.Fatalf("sim shelf --on-the-draw: %v", err)
	}
	if want := "99 cards, 95 lands, judged at 80% consistency, on the draw."; !strings.Contains(drawn, want+"\n") {
		t.Errorf("draw shelf is missing %q\n%s", want, drawn)
	}
}

// --------------------------------------------------------------- mulligan

func TestSimMulliganSearchesTheGrid(t *testing.T) {
	simHome(t, true)
	writeSimDeck(t, "mono-green", monoGreenText(t))

	out, err := runSim(t, "mulligan", "mono-green", "--games", "12", "--seed", "7", "--top", "3")
	if err != nil {
		t.Fatalf("sim mulligan: %v", err)
	}
	t.Logf("sim mulligan output:\n%s", out)

	lines := strings.Split(out, "\n")
	if lines[0] != "Mono-Green Fixture: 33 keep rules x 12 games (seed 7) ..." {
		t.Errorf("line 0 = %q", lines[0])
	}
	for _, want := range []string{
		"  spells T8   mull%  cmdr  rule\n",
		"\n  * best   = the default this simulator uses when you choose nothing\n\n",
		"Judged on spells deployed through turn 8: mulligan rate alone recommends keeping\n",
		"everything, and hand quality alone recommends mulliganing forever.\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mulligan report is missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "NO CHANGE WORTH MAKING.") && !strings.Contains(out, "BEST: keep ") {
		t.Errorf("no verdict rendered\n%s", out)
	}
	// --top 3: exactly three rule rows between the header and the legend.
	rows := 0
	for _, line := range lines {
		if strings.Contains(line, "lands AND lands + ramp") &&
			!strings.HasPrefix(line, "BEST:") &&
			!strings.HasPrefix(line, "mulligan policy") &&
			!strings.Contains(line, "Still worth knowing") {
			rows++
		}
	}
	if rows != 3 {
		t.Errorf("printed %d rule rows, want 3\n%s", rows, out)
	}
	// The best row wears its mark.
	if !strings.Contains(out, "\n*") {
		t.Errorf("no row is marked best\n%s", out)
	}
}

// ------------------------------------------------------------------ cache

func TestSimCacheListsAndClears(t *testing.T) {
	dataDir := simHome(t, false)
	path := filepath.Join(dataDir, "app.db")

	// An absent app.db is read as an empty one, never minted -- the door's
	// rule is that a reader never acquires a database, and the printed
	// words are identical to a real empty store's.
	out, err := runSim(t, "cache")
	if err != nil {
		t.Fatalf("sim cache: %v", err)
	}
	if want := "store:   " + path + "\nenabled: yes\nrows:    0 (0.0 kB)\n"; out != want {
		t.Errorf("empty cache = %q, want %q", out, want)
	}

	// Mint the store the way the door does, put one row in, and look again.
	if err := auth.Migrate(path); err != nil {
		t.Fatal(err)
	}
	store, err := simcache.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	store.Put(context.Background(), "k1", "mana", struct {
		X int `json:"x"`
	}{1})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	out, err = runSim(t, "cache")
	if err != nil {
		t.Fatalf("sim cache: %v", err)
	}
	for _, want := range []string{
		"rows:    1 (0.0 kB)\n",
		fmt.Sprintf("  %-18s %d\n", "mana", 1),
		"computed between ",
		" UTC\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cache listing is missing %q\n%s", want, out)
		}
	}

	out, err = runSim(t, "cache", "--clear")
	if err != nil {
		t.Fatalf("sim cache --clear: %v", err)
	}
	if want := "cleared 1 cached result(s) from " + path + "\n"; out != want {
		t.Errorf("clear = %q, want %q", out, want)
	}

	out, err = runSim(t, "cache")
	if err != nil {
		t.Fatalf("sim cache after clear: %v", err)
	}
	if !strings.Contains(out, "rows:    0 (0.0 kB)\n") {
		t.Errorf("cache did not empty\n%s", out)
	}
}

// ---------------------------------------------------------------- matches

func TestSimMatchesOnAnEmptyLedger(t *testing.T) {
	dataDir := simHome(t, false)
	want := "no matches recorded yet -- `mtglab sim forge` records as it plays\n"

	// No app.db at all.
	out, err := runSim(t, "matches")
	if err != nil {
		t.Fatalf("sim matches: %v", err)
	}
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}

	// An app.db with the tables and no rows says the same thing.
	if err := auth.Migrate(filepath.Join(dataDir, "app.db")); err != nil {
		t.Fatal(err)
	}
	out, err = runSim(t, "matches")
	if err != nil {
		t.Fatalf("sim matches: %v", err)
	}
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

func TestSimMatchesRendersTheLedger(t *testing.T) {
	dataDir := simHome(t, false)
	path := filepath.Join(dataDir, "app.db")
	if err := auth.Migrate(path); err != nil {
		t.Fatal(err)
	}

	arahbo, err := deck.FromText(strings.Join([]string{
		"slug: arahbo",
		"name: Arahbo",
		"commander:",
		"  - Arahbo, Roar of the World",
		"cards:",
		"  - name: Forest",
		"    category: land",
		"    why: x",
		"",
	}, "\n"), "arahbo")
	if err != nil {
		t.Fatal(err)
	}
	gyome, err := deck.FromText(strings.Join([]string{
		"slug: gyome",
		"name: Gyome",
		"commander:",
		"  - Gyome, Master Chef",
		"cards:",
		"  - name: Forest",
		"    category: land",
		"    why: x",
		"",
	}, "\n"), "gyome")
	if err != nil {
		t.Fatal(err)
	}

	seat1 := 1
	run := &tier3.SimRun{
		Output: tier3.SimOutput{Games: []tier3.GameResult{
			{Index: 1, Milliseconds: 1500, WinnerSeat: &seat1},
			{Index: 2, Milliseconds: 900, Draw: true, TimedOut: true},
		}},
		Seats:       map[int]string{1: "arahbo", 2: "gyome"},
		WallSeconds: 12.5,
	}
	rec, err := ledger.NewRecorder(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if id := rec.Record(context.Background(), ledger.Match{
		Run: run, Decks: []*deck.Deck{arahbo, gyome},
		Clock: 300, GamesRequested: 2, Hosted: false,
	}); id == 0 {
		t.Fatal("the match did not record")
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	out, err := runSim(t, "matches")
	if err != nil {
		t.Fatalf("sim matches: %v", err)
	}
	t.Logf("sim matches output:\n%s", out)
	for _, want := range []string{
		"ledger: " + path + "\n",
		"\n#1  ",
		" UTC  (local, unseeded)\n",
		fmt.Sprintf("  %-22s %2d win  (unlabelled)\n", "arahbo", 1),
		fmt.Sprintf("  %-22s %2d wins  (unlabelled)\n", "gyome", 0),
		"  2 of 2 games  (1 hit the 300s clock)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("matches is missing %q\n%s", want, out)
		}
	}
	// The clocked-out game's win line counted for nobody, and it is not a
	// real draw either -- the two are never folded together.
	if strings.Contains(out, "real draw") {
		t.Errorf("a clock-out rendered as a real draw\n%s", out)
	}
}

// ------------------------------------------------------------------ forge
//
// Only the refusal paths: this box has no Forge distribution, and CI never
// will (the live proof is MTGLAB_LIVE_FORGE=1 in internal/sim/tier3).

func TestSimForgeNeedsTwoDecks(t *testing.T) {
	simHome(t, false)
	t.Setenv("MTGLAB_FORGE_HOME", filepath.Join(t.TempDir(), "missing"))
	writeSimDeck(t, "solo", "slug: solo\nname: Solo\ncards: []\n")
	_, err := runSim(t, "forge", "solo")
	if err == nil || err.Error() != "a game needs at least two decks" {
		t.Fatalf("err = %v", err)
	}
}

func TestSimForgeRefusesWithoutForge(t *testing.T) {
	simHome(t, false)
	home := filepath.Join(t.TempDir(), "missing")
	t.Setenv("MTGLAB_FORGE_HOME", home)
	writeSimDeck(t, "a", "slug: a\nname: A\ncards: []\n")
	writeSimDeck(t, "b", "slug: b\nname: B\ncards: []\n")

	for _, args := range [][]string{
		{"forge", "a", "b"},
		{"forge", "a", "--check-only"},
	} {
		_, err := runSim(t, args...)
		if err == nil {
			t.Fatalf("%v did not refuse", args)
		}
		if !strings.Contains(err.Error(), "no Forge card data at ") ||
			!strings.Contains(err.Error(), "set MTGLAB_FORGE_HOME to an unpacked Forge distribution") {
			t.Errorf("%v refused with %q", args, err.Error())
		}
	}

	// A deck that does not exist is refused before Forge is even looked for.
	_, err := runSim(t, "forge", "a", "zz")
	want := "no deck at " + filepath.Join(settings().DecksDir, "zz", "deck.yaml")
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// ------------------------------------------------- the formatting helpers
//
// Each one renders a number the recorded tables lean on, so each is pinned
// on the inputs where a naive spelling drifts: thousands grouping, negative
// slice bounds, and widths counted in code points rather than bytes.

func TestTableTextHelpers(t *testing.T) {
	t.Parallel()
	if got := groupThousands(20); got != "20" {
		t.Errorf("groupThousands(20) = %q", got)
	}
	if got := groupThousands(20000); got != "20,000" {
		t.Errorf("groupThousands(20000) = %q", got)
	}
	if got := groupThousands(1234567); got != "1,234,567" {
		t.Errorf("groupThousands(1234567) = %q", got)
	}
	if got := headOf([]int{1, 2, 3, 4, 5}, -2); len(got) != 3 || got[2] != 3 {
		t.Errorf("headOf(-2) = %v", got)
	}
	if got := headOf([]int{1, 2}, 99); len(got) != 2 {
		t.Errorf("headOf(99) = %v", got)
	}
	if got := headOf([]int{1, 2}, -99); len(got) != 0 {
		t.Errorf("headOf(-99) = %v", got)
	}
	// Bösium Strip is in the pool; padding must count code points.
	if got := padRight("Bösium", 8); got != "Bösium  " {
		t.Errorf("padRight = %q", got)
	}
	if got := padLeft("é", 3); got != "  é" {
		t.Errorf("padLeft = %q", got)
	}
	if got := headRunes("Déjà Vu", 4); got != "Déjà" {
		t.Errorf("headRunes = %q", got)
	}
	if got := percent(0.9, 0); got != "90%" {
		t.Errorf("percent(0.9, 0) = %q", got)
	}
	if got := percent(0.123, 1); got != "12.3%" {
		t.Errorf("percent(0.123, 1) = %q", got)
	}
	if got := signed(0.01); got != "+0.01" {
		t.Errorf("signed = %q", got)
	}
	if got := signed(-0.25); got != "-0.25" {
		t.Errorf("signed = %q", got)
	}
	if got := gFormat(4); got != "4" {
		t.Errorf("gFormat(4) = %q", got)
	}
	if got := gFormat(4.5); got != "4.5" {
		t.Errorf("gFormat(4.5) = %q", got)
	}
}
