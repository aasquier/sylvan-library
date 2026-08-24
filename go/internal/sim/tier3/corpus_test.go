package tier3_test

import (
	"encoding/json"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The recorded corpus: everything in `sim/tier3` that is a pure
// transformation, held to the frozen golden case for case.
//
// What is deliberately NOT here is the JVM and the private network — those are
// a subprocess and a socket, which a corpus cannot hold and which the tests
// beside this one, and a real match on a real distribution, do instead. What
// IS here is every point where Forge's text becomes a number somebody reads:
// the log parser, the `.dck` exporter, the coverage reading, the wire codec,
// and the shaped result the deck page renders.
//
// `testdata/forge.json` is a frozen recorded golden, never regenerated.

type forgeCorpus struct {
	Index    []string       `json:"index"`
	Logs     []logCase      `json:"logs"`
	Dck      []dckCase      `json:"dck"`
	Coverage []coverageCase `json:"coverage"`
	Wire     wireCase       `json:"wire"`
	Shape    shapeCorpus    `json:"shape"`
}

type logCase struct {
	Note             string           `json:"note"`
	Text             string           `json:"text"`
	Trustworthy      bool             `json:"trustworthy"`
	Unsupported      []string         `json:"unsupported"`
	DeckLoadFailures []string         `json:"deck_load_failures"`
	Games            []tier3.WireGame `json:"games"`
	IsGameResult     []bool           `json:"is_game_result"`
}

type dckCase struct {
	Note     string `json:"note"`
	Deck     string `json:"deck"`
	Bare     string `json:"bare"`
	Resolved string `json:"resolved"`
}

type coverageCase struct {
	Note          string            `json:"note"`
	Deck          string            `json:"deck"`
	SecondDeck    string            `json:"second_deck"`
	Slug          string            `json:"slug"`
	Checked       int               `json:"checked"`
	Resolved      map[string]string `json:"resolved"`
	Missing       []string          `json:"missing"`
	OK            bool              `json:"ok"`
	Renamed       [][]string        `json:"renamed"`
	Summary       string            `json:"summary"`
	SecondSummary string            `json:"second_summary"`
	Refusal       *string           `json:"refusal"`
}

type wireCase struct {
	Decks                []string           `json:"decks"`
	Reports              []tier3.WireReport `json:"reports"`
	Games                []tier3.WireGame   `json:"games"`
	Run                  json.RawMessage    `json:"run"`
	RunJSON              string             `json:"run_json"`
	GameJSON             []string           `json:"game_json"`
	RebuiltStartupSecond float64            `json:"rebuilt_startup_seconds"`
	RebuiltSeats         map[string]string  `json:"rebuilt_seats"`
	RebuiltWallSeconds   float64            `json:"rebuilt_wall_seconds"`
	OldShimRun           json.RawMessage    `json:"old_shim_run"`
	OldShimRunJSON       string             `json:"old_shim_run_json"`
}

type shapeCorpus struct {
	Caveat       string            `json:"caveat"`
	Clock        int               `json:"clock"`
	GamesDefault int               `json:"games_default"`
	GamesMax     int               `json:"games_max"`
	Decks        map[string]string `json:"decks"`
	Shapes       []struct {
		Note  string          `json:"note"`
		Shape json.RawMessage `json:"shape"`
	} `json:"shapes"`
	Rows []struct {
		Note string          `json:"note"`
		Row  json.RawMessage `json:"row"`
	} `json:"rows"`
	GamesDial []struct {
		Note    string         `json:"note"`
		Payload map[string]any `json:"payload"`
		Games   *int           `json:"games"`
		Raises  string         `json:"raises"`
	} `json:"games_dial"`
	SeedDial []struct {
		Note    string         `json:"note"`
		Payload map[string]any `json:"payload"`
		Seed    string         `json:"seed"`
		Raises  string         `json:"raises"`
	} `json:"seed_dial"`
	Labels []struct {
		Slugs []string `json:"slugs"`
		Games int      `json:"games"`
		Label string   `json:"label"`
	} `json:"labels"`
	Keys []struct {
		Addresses []string `json:"addresses"`
		Games     int      `json:"games"`
		Seed      string   `json:"seed"`
		Key       string   `json:"key"`
	} `json:"keys"`
}

func loadForgeCorpus(t *testing.T) forgeCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "forge.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus forgeCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus.Logs) == 0 || len(corpus.Dck) == 0 {
		t.Fatal("the Forge corpus is empty; testdata/forge.json is a frozen " +
			"golden -- restore it from version control")
	}
	return corpus
}

// TestTheLogParserAgreesWithTheCorpus drives every shape a `forge sim -q`
// stream
// can take through [tier3.Parse] and compares the whole parse.
//
// The whole parse rather than a game count, because the fields that separate a
// real draw from a clock-out and a winner from a seat are exactly the ones a
// careless port folds together — and a count would be green for all of them.
func TestTheLogParserAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadForgeCorpus(t)
	for _, c := range corpus.Logs {
		t.Run(c.Note, func(t *testing.T) {
			got := tier3.Parse(c.Text)
			if got.Trustworthy() != c.Trustworthy {
				t.Errorf("trustworthy = %v, want %v", got.Trustworthy(), c.Trustworthy)
			}
			if !equalStrings(got.Unsupported, c.Unsupported) {
				t.Errorf("unsupported = %v, want %v", got.Unsupported, c.Unsupported)
			}
			if !equalStrings(got.DeckLoadFailures, c.DeckLoadFailures) {
				t.Errorf("deck load failures = %v, want %v",
					got.DeckLoadFailures, c.DeckLoadFailures)
			}
			if len(got.Games) != len(c.Games) {
				t.Fatalf("parsed %d games, want %d", len(got.Games), len(c.Games))
			}
			for i, want := range c.Games {
				if diff := gameDiff(tier3.GameToWire(got.Games[i]), want); diff != "" {
					t.Errorf("game %d: %s", i+1, diff)
				}
			}
		})
	}
}

// TestTheStatelessPredicateAgreesWithTheCorpus asks `IsGameResult` of
// every
// line, because it is a second seam over the same regexes: a port could get
// the machine right and the predicate wrong, and only the tick would notice.
func TestTheStatelessPredicateAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadForgeCorpus(t)
	for _, c := range corpus.Logs {
		lines := splitLikeTheCorpus(c.Text)
		if len(lines) != len(c.IsGameResult) {
			t.Errorf("%s: split into %d lines, the corpus got %d",
				c.Note, len(lines), len(c.IsGameResult))
			continue
		}
		for i, line := range lines {
			if got := tier3.IsGameResult(line); got != c.IsGameResult[i] {
				t.Errorf("%s line %d (%q): IsGameResult = %v, want %v",
					c.Note, i+1, line, got, c.IsGameResult[i])
			}
		}
	}
}

// TestTheExporterWritesTheRecordedBytes holds the `.dck` to the corpus
// byte for byte,
// resolved and unresolved.
//
// Byte for byte because a `.dck` is what Forge actually reads: a section
// header in the wrong case, a missing empty `[Sideboard]`, or a sort that puts
// two cards the other way round is a file that either fails or — worse —
// works differently.
func TestTheExporterWritesTheRecordedBytes(t *testing.T) {
	t.Parallel()
	corpus := loadForgeCorpus(t)
	index := indexOf(corpus.Index)
	for _, c := range corpus.Dck {
		t.Run(c.Note, func(t *testing.T) {
			d := parseDeck(t, c.Deck)
			if got := tier3.ToDck(d, nil); got != c.Bare {
				t.Errorf("bare export:\n got %q\nwant %q", got, c.Bare)
			}
			report := tier3.Check(d, index)
			if got := tier3.ToDck(d, report.Resolved); got != c.Resolved {
				t.Errorf("resolved export:\n got %q\nwant %q", got, c.Resolved)
			}
		})
	}
}

// TestThePreFlightAgreesWithTheCorpus holds the coverage reading and
// every
// sentence it says.
//
// The refusal text is in the corpus because it is what a 422 carries and the
// deck page renders verbatim — the `unavailable` lesson, where a sentinel's
// own words shipped as a prefix nobody wrote.
func TestThePreFlightAgreesWithTheCorpus(t *testing.T) {
	t.Parallel()
	corpus := loadForgeCorpus(t)
	index := indexOf(corpus.Index)
	for _, c := range corpus.Coverage {
		t.Run(c.Note, func(t *testing.T) {
			d := parseDeck(t, c.Deck)
			report := tier3.Check(d, index)
			if report.Slug != c.Slug {
				t.Errorf("slug = %q, want %q", report.Slug, c.Slug)
			}
			if report.Checked != c.Checked {
				t.Errorf("checked = %d, want %d", report.Checked, c.Checked)
			}
			if !reflect.DeepEqual(report.Resolved, orEmptyMap(c.Resolved)) {
				t.Errorf("resolved = %v, want %v", report.Resolved, c.Resolved)
			}
			if !equalStrings(report.Missing, c.Missing) {
				t.Errorf("missing = %v, want %v", report.Missing, c.Missing)
			}
			if report.OK() != c.OK {
				t.Errorf("ok = %v, want %v", report.OK(), c.OK)
			}
			if got := report.Summary(); got != c.Summary {
				t.Errorf("summary:\n got %q\nwant %q", got, c.Summary)
			}
			renamed := report.Renamed()
			if len(renamed) != len(c.Renamed) {
				t.Fatalf("renamed %d pairs, want %d", len(renamed), len(c.Renamed))
			}
			for i, pair := range c.Renamed {
				if renamed[i][0] != pair[0] || renamed[i][1] != pair[1] {
					t.Errorf("renamed[%d] = %v, want %v", i, renamed[i], pair)
				}
			}

			reports := []tier3.CoverageReport{report}
			if c.SecondDeck != "" {
				reports = append(reports, tier3.Check(parseDeck(t, c.SecondDeck), index))
				if got := reports[1].Summary(); got != c.SecondSummary {
					t.Errorf("second summary:\n got %q\nwant %q", got, c.SecondSummary)
				}
			}
			err := tier3.RaiseUnlessCovered(reports)
			switch {
			case c.Refusal == nil && err != nil:
				t.Errorf("refused a covered deck: %v", err)
			case c.Refusal != nil && err == nil:
				t.Errorf("did not refuse; the corpus says %q", *c.Refusal)
			case c.Refusal != nil && err.Error() != *c.Refusal:
				t.Errorf("refusal:\n got %q\nwant %q", err.Error(), *c.Refusal)
			}
		})
	}
}

// TestTheWireIsTheRecordedBytes holds the seam in both directions.
//
// The **bytes**, not just the shape: the two ends of this wire can be on
// different deploys for the minutes a deploy takes, and key order is
// what `encoding/json` gets wrong for free when a payload is built from a map.
func TestTheWireIsTheRecordedBytes(t *testing.T) {
	t.Parallel()
	corpus := loadForgeCorpus(t)

	for i, want := range corpus.Wire.GameJSON {
		raw, err := json.Marshal(corpus.Wire.Games[i])
		if err != nil {
			t.Fatal(err)
		}
		if compact(t, string(raw)) != compact(t, want) {
			t.Errorf("game %d on the wire:\n got %s\nwant %s", i+1, raw, want)
		}
	}

	var run tier3.WireRun
	if err := json.Unmarshal(corpus.Wire.Run, &run); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if compact(t, string(raw)) != compact(t, corpus.Wire.RunJSON) {
		t.Errorf("a run on the wire:\n got %s\nwant %s", raw, corpus.Wire.RunJSON)
	}

	// A run rebuilt from the wire computes the same derived numbers — the
	// property that lets a remote match and a local one be the same thing to
	// everything downstream.
	rebuilt := tier3.RunFromWire(run)
	if got := rebuilt.StartupSeconds(); got != corpus.Wire.RebuiltStartupSecond {
		t.Errorf("rebuilt startup = %v, want %v", got, corpus.Wire.RebuiltStartupSecond)
	}
	if got := rebuilt.WallSeconds; got != corpus.Wire.RebuiltWallSeconds {
		t.Errorf("rebuilt wall = %v, want %v", got, corpus.Wire.RebuiltWallSeconds)
	}
	for seat, slug := range corpus.Wire.RebuiltSeats {
		n := 0
		if _, err := json.Marshal(seat); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(seat), &n); err != nil {
			t.Fatalf("seat key %q: %v", seat, err)
		}
		if rebuilt.Seats[n] != slug {
			t.Errorf("seat %d = %q, want %q", n, rebuilt.Seats[n], slug)
		}
	}

	// **Deploy skew, both directions.** A shim from before `forge_version`
	// omits it, and the app must read that as "not reported" rather than as
	// an error — which is what keeps a deploy that updates the app a few
	// minutes before its worker from breaking a match over a field.
	var old tier3.WireRun
	if err := json.Unmarshal(corpus.Wire.OldShimRun, &old); err != nil {
		t.Fatal(err)
	}
	if rebuiltOld := tier3.RunFromWire(old); rebuiltOld.ForgeVersion != "" {
		t.Errorf("an old shim's run claimed Forge %q", rebuiltOld.ForgeVersion)
	}
	// And it re-encodes as the corpus records it, whole-second wall clock
	// included: the recorded wire says `1.0` where `encoding/json` would
	// write `1`, and a test
	// that only decoded this row would never have seen the difference.
	oldRaw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if normalise(string(oldRaw)) != normalise(corpus.Wire.OldShimRunJSON) {
		t.Errorf("an old shim's run on the wire:\n got %s\nwant %s",
			oldRaw, corpus.Wire.OldShimRunJSON)
	}

	// The decks cross as their own deck.yaml text, which is what makes
	// `deck.FromText` the one parser on both machines.
	decks, err := tier3.DecksFromWire(corpus.Wire.Decks)
	if err != nil {
		t.Fatal(err)
	}
	if len(decks) != 2 || decks[0].Slug != "atla-palani" || decks[1].Slug != "arahbo" {
		t.Errorf("the decks did not survive the wire: %v", decks)
	}
	back, err := tier3.DecksToWire(decks)
	if err != nil {
		t.Fatal(err)
	}
	for i := range back {
		if back[i] != corpus.Wire.Decks[i] {
			t.Errorf("deck %d did not round-trip:\n got %q\nwant %q",
				i, back[i], corpus.Wire.Decks[i])
		}
	}
}

// TestResolvedOrderIsUnobservable pins the one claim `wire.go` makes about a
// field it does *not* reproduce the recorded key order for.
//
// The argument is that `resolved` crosses between our own two processes and is
// read by neither: the receiver calls RaiseUnlessCovered, which reads `slug`,
// `checked` and `missing`. If that ever stops being true, this fails with the
// comment that made the claim.
func TestResolvedOrderIsUnobservable(t *testing.T) {
	t.Parallel()
	scrambled := []tier3.WireReport{{
		Slug: "x", Checked: 3,
		Resolved: map[string]string{"b": "b", "a": "a", "c": "c"},
		Missing:  nil,
	}}
	if err := tier3.RaiseUnlessCovered(tier3.ReportsFromWire(scrambled)); err != nil {
		t.Fatalf("a covered report was refused: %v", err)
	}
	broken := []tier3.WireReport{{
		Slug: "x", Checked: 3,
		Resolved: map[string]string{"a": "a"},
		Missing:  []string{"Nope"},
	}}
	err := tier3.RaiseUnlessCovered(tier3.ReportsFromWire(broken))
	if err == nil {
		t.Fatal("a report with a missing card was not refused")
	}
	// And the refusal names the cards rather than the resolutions, which is
	// the whole reason the order does not matter.
	if want := "Nope"; !contains(err.Error(), want) {
		t.Errorf("the refusal did not name the missing card: %q", err.Error())
	}
	if contains(err.Error(), `"a"`) {
		t.Errorf("the refusal rendered the resolved map: %q", err.Error())
	}
}

func indexOf(names []string) map[string]bool {
	index := map[string]bool{}
	for _, n := range names {
		index[n] = true
	}
	return index
}

func parseDeck(t *testing.T, text string) *deck.Deck {
	t.Helper()
	d, err := deck.FromText(text, "")
	if err != nil {
		t.Fatalf("the corpus deck did not parse: %v", err)
	}
	return d
}

func orEmptyMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle || indexOfSub(haystack, needle) >= 0)
}

func indexOfSub(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func compact(t *testing.T, raw string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("not JSON: %v (%s)", err, raw)
	}
	// Re-encode through the same path both sides use, so the comparison is
	// about key order and values rather than about whitespace.
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	_ = out
	return normalise(raw)
}

// normalise strips the insignificant whitespace the recorded wire carries
// after `:`
// and `,` so a comparison is about order and values. It walks the string
// rather than re-encoding, because re-encoding through a map is exactly the
// operation that would hide the bug this test is looking for.
func normalise(raw string) string {
	out := make([]byte, 0, len(raw))
	inString, escaped := false, false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == ' ' || c == '\n' || c == '\t' {
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func gameDiff(got, want tier3.WireGame) string {
	if got.Index != want.Index {
		return diffOf("index", got.Index, want.Index)
	}
	if got.Milliseconds != want.Milliseconds {
		return diffOf("milliseconds", got.Milliseconds, want.Milliseconds)
	}
	if got.Draw != want.Draw {
		return diffOf("draw", got.Draw, want.Draw)
	}
	if !samePtrString(got.Winner, want.Winner) {
		return diffOf("winner", show(got.Winner), show(want.Winner))
	}
	if !samePtrInt(got.WinnerSeat, want.WinnerSeat) {
		return diffOf("winner_seat", showInt(got.WinnerSeat), showInt(want.WinnerSeat))
	}
	if !samePtrInt(got.Turns, want.Turns) {
		return diffOf("turns", showInt(got.Turns), showInt(want.Turns))
	}
	if got.TimedOut != want.TimedOut {
		return diffOf("timed_out", got.TimedOut, want.TimedOut)
	}
	return ""
}

func diffOf(field string, got, want any) string {
	return field + " = " + toString(got) + ", want " + toString(want)
}

func toString(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func samePtrString(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func samePtrInt(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func show(v *string) string {
	if v == nil {
		return "null"
	}
	return *v
}

func showInt(v *int) string {
	if v == nil {
		return "null"
	}
	return big.NewInt(int64(*v)).String()
}

// TestAHostileSlugCannotNameAFile is the guard CodeQL asked for, driven
// rather than asserted.
//
// **The test plants something real on the other side of the boundary.** A
// refusal that merely returns an error proves nothing on its own — the write
// might have failed for a dull reason — so each case checks that the escape
// target does NOT exist afterwards, and the control case checks that an
// ordinary slug still writes exactly where it should. A guard whose test
// cannot tell "refused" from "wrote it somewhere else" is not a guard.
func TestAHostileSlugCannotNameAFile(t *testing.T) {
	t.Parallel()
	// Every escape is aimed inside the running test's own directory, so a
	// leftover from anywhere else cannot make this pass or fail. Not
	// fastidiousness: proving the guard means *actually performing the escape*
	// with it removed, and an early mutation run of this guard wrote a
	// real `/tmp/escaped.dck` that then failed the next full suite. A test
	// whose subject is a file outside its own sandbox is a test with a memory.
	root := t.TempDir()
	for _, c := range []struct{ note, slug string }{
		{"a parent-directory escape", "../escaped"},
		{"a deep escape", "../../escaped"},
		{"an absolute path", filepath.Join(root, "absolute", "escaped")},
		{"a nested path", "sub/escaped"},
		// `<slug>.dck` goes on Forge's own command line after `-d`, so a
		// leading hyphen is read as a flag rather than as a deck.
		{"a slug that is a flag", "-n"},
		{"a slug that is a long flag", "--help"},
		{"a null byte", "ok\x00.dck"},
		{"an upper-case slug", "Escaped"},
		{"a dotted slug", "a.b"},
		{"an empty slug", ""},
		{"a space", "two words"},
		{"a trailing hyphen", "trailing-"},
	} {
		t.Run(c.note, func(t *testing.T) {
			dir := filepath.Join(root, "sandbox", c.note)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			d := &deck.Deck{Slug: c.slug, Name: "Hostile"}
			path, err := tier3.WriteDck(d, dir, nil)
			if err == nil {
				t.Fatalf("a slug of %q was accepted and wrote %s", c.slug, path)
			}
			// Nothing was written anywhere under this run's own directory,
			// which is where every escape above was aimed.
			var written []string
			_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
				if err == nil && !e.IsDir() && strings.HasSuffix(p, ".dck") {
					written = append(written, p)
				}
				return nil
			})
			if len(written) != 0 {
				t.Errorf("a refused write produced %v", written)
			}
		})
	}

	// The control: an ordinary slug still writes, exactly where it should.
	// Without this the whole table would pass against a WriteDck that refused
	// everything.
	ok := t.TempDir()
	path, err := tier3.WriteDck(&deck.Deck{Slug: "arahbo-cats", Name: "Cats"}, ok, nil)
	if err != nil {
		t.Fatalf("an ordinary slug was refused: %v", err)
	}
	if want := filepath.Join(ok, "arahbo-cats.dck"); path != want {
		t.Errorf("wrote %s, want %s", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the file is not there: %v", err)
	}
}
