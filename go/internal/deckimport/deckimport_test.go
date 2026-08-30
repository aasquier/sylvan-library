package deckimport

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/pool/pooltest"
	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// The importer's oracle: a paste, resolved against the 21-card pool, beside
// the recorded draft `deck.yaml` for it (testdata/imports.json, a frozen
// golden).
//
// The pool is the load-bearing half. `CanonicalName` corrects casing and keeps
// a double-faced card written by its front face, `buildEntries` files a card
// as `land` on `IsLand` and never on a heading, and an unknown name is kept
// verbatim rather than dropped -- none of which can be checked without real
// records, and all of which decide what lands in somebody's file.

type importCase struct {
	Name           string   `json:"name"`
	Slug           string   `json:"slug"`
	Text           string   `json:"text"`
	Commander      []string `json:"commander"`
	Companion      string   `json:"companion"`
	Bracket        *int     `json:"bracket"`
	DeckName       string   `json:"deck_name"`
	Status         string   `json:"status"`
	Refused        string   `json:"refused"`
	YAML           string   `json:"yaml"`
	Unknown        []string `json:"unknown"`
	Notes          []string `json:"notes"`
	Unreadable     []line   `json:"unreadable"`
	Skipped        []line   `json:"skipped"`
	NeedsRationale int      `json:"needs_rationale"`
}

type line struct {
	LineNo int    `json:"line_no"`
	Text   string `json:"text"`
}

func TestBuildDeckWritesTheRecordedDraft(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/imports.json")
	if err != nil {
		t.Fatalf("reading the oracle: %v", err)
	}
	var cases []importCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("decoding the oracle: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("the oracle is empty; testdata/imports.json is a frozen golden and should never be")
	}

	cards := pooltest.Open(t)
	ctx := context.Background()
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			parsed := decklist.Parse(c.Text)
			var found map[string]*pool.CardRecord
			if err := cards.Use(ctx, func(conn *pool.Conn) error {
				var err error
				found, err = conn.GetCards(ctx, NamesIn(parsed, c.Commander, c.Companion))
				return err
			}); err != nil {
				t.Fatalf("asking the pool: %v", err)
			}

			report, err := BuildDeck(parsed, found, Options{
				Slug: c.Slug, Name: c.DeckName, Commander: c.Commander,
				Companion: c.Companion, Bracket: c.Bracket, Status: c.Status})

			if c.Refused != "" {
				if err == nil {
					t.Fatalf("the corpus refused with %q; this built a deck", c.Refused)
				}
				if !IsRefused(err) {
					t.Fatalf("refused with the wrong kind of error: %v", err)
				}
				if err.Error() != c.Refused {
					t.Errorf("refusal is\n  %s\nthe corpus says\n  %s", err.Error(), c.Refused)
				}
				return
			}
			if err != nil {
				t.Fatalf("the corpus built a deck; this refused: %v", err)
			}

			// The header carries the day it was written. Substituting our own
			// date proves the format as well as the value: a writer that
			// spelled it differently would leave the literal date in place
			// and the comparison below would show it.
			got := strings.Replace(report.YAML, time.Now().Format("2006-01-02"), "DATE", 1)
			if got != c.YAML {
				t.Errorf("the file differs from the recorded draft\n--- want ---\n%s\n--- got ---\n%s",
					c.YAML, got)
			}
			checkStrings(t, "unknown", report.Unknown, c.Unknown)
			checkStrings(t, "notes", report.Notes, c.Notes)
			checkLines(t, "unreadable", report.Unreadable, c.Unreadable)
			checkLines(t, "skipped", report.Skipped, c.Skipped)
			if report.NeedsRationale() != c.NeedsRationale {
				t.Errorf("needs_rationale %d, the corpus says %d",
					report.NeedsRationale(), c.NeedsRationale)
			}
		})
	}
}

func checkStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %v, the corpus says %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s %d is %q, the corpus says %q", what, i, got[i], want[i])
		}
	}
}

func checkLines(t *testing.T, what string, got []decklist.Line, want []line) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d, the corpus says %d", what, len(got), len(want))
	}
	for i := range got {
		if got[i].LineNo != want[i].LineNo || got[i].Text != want[i].Text {
			t.Errorf("%s %d is %+v, the corpus says %+v", what, i, got[i], want[i])
		}
	}
}

// The companion hint has no card in the 21-card pool to fire on, so it gets a
// record of its own. It is a *hint* and never a decision: having a Companion
// ability is a pool fact, and concluding that this deck runs the card as its
// companion is a judgement the import does not make.
func TestACompanionOnTheBoardIsPointedAtRatherThanAssumed(t *testing.T) {
	t.Parallel()
	text := "1 Sol Ring\n"
	parsed := decklist.Parse(text + "Sideboard\n1 Kaheera, the Orphanguard\n")
	kaheera := &pool.CardRecord{
		Name:     "Kaheera, the Orphanguard",
		TypeLine: "Legendary Creature — Cat Beast",
		OracleText: "Companion — Each creature card in your starting deck is a " +
			"Cat, Elemental, Nightmare, Dinosaur, or Beast card.",
	}
	report, err := BuildDeck(parsed,
		map[string]*pool.CardRecord{"Kaheera, the Orphanguard": kaheera},
		Options{Slug: "hint", Commander: []string{"Goreclaw, Terror of Qal Sisma"},
			Status: "theoretical"})
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if report.Deck.Companion != nil {
		t.Error("a companion on the board must not be assumed to be the deck's")
	}
	var hinted bool
	for _, note := range report.Notes {
		if strings.Contains(note, "has a Companion ability") {
			hinted = true
		}
	}
	if !hinted {
		t.Errorf("the board's companion should be pointed at; notes were %v", report.Notes)
	}
}

// TestACommaIsUsuallyPartOfTheName is the bug found on 2026-08-24 by typing
// the name of the deck this whole library is built around.
//
// The import page split its commander field on commas before the request was
// even made, on the reasoning that a partner pair is two commanders. A comma
// is punctuation inside most legendary names -- Arahbo, Roar of the World;
// Gyome, Master Chef; Tivit, Seller of Secrets -- so naming a commander by
// hand produced two commanders, neither of them a card, and a deck reporting
// `unknown-card` twice for the two halves of one legend.
//
// The reading is decided by lookup rather than by punctuation, which is what
// keeps it inside this package's first rule: the whole string being a card is
// a fact, not a guess.
func TestACommaIsUsuallyPartOfTheName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		written string
		known   []string
		want    []string
		note    bool
	}{{
		name:    "one legend whose name contains a comma",
		written: "Arahbo, Roar of the World",
		known:   []string{"Arahbo, Roar of the World"},
		want:    []string{"Arahbo, Roar of the World"},
	}, {
		name:    "a pairing written the way players write one",
		written: "Thrasios, Triton Hero + Tymna the Weaver",
		known:   []string{"Thrasios, Triton Hero", "Tymna the Weaver"},
		want:    []string{"Thrasios, Triton Hero", "Tymna the Weaver"},
		note:    true,
	}, {
		name:    "a pairing whose halves have no commas of their own",
		written: "Sakashima of a Thousand Faces, Tymna the Weaver",
		known:   []string{"Sakashima of a Thousand Faces", "Tymna the Weaver"},
		want:    []string{"Sakashima of a Thousand Faces", "Tymna the Weaver"},
		note:    true,
	}, {
		// The whole string wins even when the parts would also resolve --
		// otherwise a card called "A, B" would be unreachable whenever cards
		// called "A" and "B" both exist.
		name:    "the whole card beats a pairing that would also resolve",
		written: "Arahbo, Roar of the World",
		known:   []string{"Arahbo, Roar of the World", "Arahbo", "Roar of the World"},
		want:    []string{"Arahbo, Roar of the World"},
	}, {
		name:    "neither reading resolves, so nothing is invented",
		written: "Nonsense, Not A Card",
		known:   []string{},
		want:    []string{"Nonsense, Not A Card"},
	}, {
		// Half a pairing is the wrong reading, not a pairing with a typo.
		name:    "one half unknown leaves the string whole",
		written: "Tymna the Weaver + Wgrsdlkj",
		known:   []string{"Tymna the Weaver"},
		want:    []string{"Tymna the Weaver + Wgrsdlkj"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cards := map[string]*pool.CardRecord{}
			for _, n := range tc.known {
				cards[n] = &pool.CardRecord{Name: n}
			}
			got, note := CommanderReading([]string{tc.written}, cards)
			if len(got) != len(tc.want) {
				t.Fatalf("read %q as %v, want %v", tc.written, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("read %q as %v, want %v", tc.written, got, tc.want)
					break
				}
			}
			if (note != "") != tc.note {
				t.Errorf("note %q, wanted one: %v", note, tc.note)
			}
		})
	}
}

// TestNamesInFetchesEveryReading is the half that makes the choice above a
// lookup rather than a guess: a reading whose names were never fetched cannot
// resolve, so it would lose to the wrong one every time with nothing looking
// broken.
func TestNamesInFetchesEveryReading(t *testing.T) {
	t.Parallel()
	names := NamesIn(decklist.Parse("1 Forest\n"),
		[]string{"Thrasios, Triton Hero + Tymna the Weaver"}, "")
	for _, want := range []string{
		"Thrasios, Triton Hero + Tymna the Weaver", // the whole string
		"Thrasios, Triton Hero",                    // split on the plus
		"Tymna the Weaver",
		"Thrasios", // and on the commas, which is the weaker reading
		"Triton Hero + Tymna the Weaver",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("NamesIn did not ask the pool about %q; it has %v", want, names)
		}
	}
}

// fakeReader is a pool that knows a fixed set of names and scores everything
// else against them with a stub. No database, and no jaro-winkler either: the
// point of these tests is the DECISION, and a scorer here would only re-test
// DuckDB's implementation of a published metric.
type fakeReader struct {
	// scores maps a written name to what the pool would say about it.
	scores map[string][]Candidate
	// known is what `Cards` will hand over.
	known map[string]bool
	// asked records every name scored, so a test can prove a correctly spelled
	// card was never second-guessed.
	asked []string
}

func (f *fakeReader) Nearest(_ context.Context, written string, _ int) ([]Candidate, error) {
	f.asked = append(f.asked, written)
	return f.scores[written], nil
}

func (f *fakeReader) Cards(_ context.Context, names []string) (map[string]*pool.CardRecord, error) {
	out := map[string]*pool.CardRecord{}
	for _, n := range names {
		if f.known[n] {
			out[n] = &pool.CardRecord{Name: n}
		}
	}
	return out, nil
}

// TestRespellReadsOnlyWhatIsClear is Aaron's 2026-08-24 ruling and its floor
// in one table: do the matching on the backend and do not let misspelled
// things in -- but only where one card is clearly what was meant.
func TestRespellReadsOnlyWhatIsClear(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		written string
		found   []Candidate
		want    string // "" means: left unread
	}{{
		name:    "a plain typo is read",
		written: "Sol Rng",
		found:   []Candidate{{Name: "Sol Ring", Score: 0.975}, {Name: "Soul Strings", Score: 0.8889}},
		want:    "Sol Ring",
	}, {
		name:    "a single candidate over the floor is read",
		written: "Rhystic Stud" + "dy", // split so the spell checker sees a typo, not a word
		found:   []Candidate{{Name: "Rhystic Study", Score: 0.9857}},
		want:    "Rhystic Study",
	}, {
		// The whole reason there are two numbers. A close field is exactly
		// where a genuine new printing sits, and picking the winner of one is
		// how a card nobody meant gets into a deck.
		name:    "a close field is left alone even above the floor",
		written: "Ancient Tomb",
		found:   []Candidate{{Name: "Ancient Tombs", Score: 0.97}, {Name: "Ancient Tome", Score: 0.96}},
		want:    "",
	}, {
		name:    "keyboard mash is left alone",
		written: "asdfghjkl qwerty",
		found:   []Candidate{{Name: "Dog Walker", Score: 0.7097}},
		want:    "",
	}, {
		name:    "just under the floor is left alone",
		written: "Nearly Something",
		found:   []Candidate{{Name: "Nearly Somethin", Score: 0.9499}},
		want:    "",
	}, {
		name:    "nothing came back",
		written: "Whatever",
		found:   nil,
		want:    "",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reader := &fakeReader{
				scores: map[string][]Candidate{tc.written: tc.found},
				known:  map[string]bool{},
			}
			for _, c := range tc.found {
				reader.known[c.Name] = true
			}
			cards := map[string]*pool.CardRecord{}
			got, err := Respell(context.Background(), reader, []string{tc.written}, cards)
			if err != nil {
				t.Fatalf("Respell: %v", err)
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("read %q as %v; it should have been left alone", tc.written, got)
				}
				if _, ok := cards[tc.written]; ok {
					t.Error("a name that was left unread still landed in the lookup")
				}
				return
			}
			if len(got) != 1 || got[0].Read != tc.want {
				t.Fatalf("read %q as %v, want %s", tc.written, got, tc.want)
			}
			// The half that makes everything downstream work: the record is
			// installed under the name that was WRITTEN, so `CanonicalName`
			// finds it and hands back the real card's name.
			name, rec := CanonicalName(tc.written, cards)
			if rec == nil || name != tc.want {
				t.Errorf("the lookup resolves %q to %q/%v, want %s",
					tc.written, name, rec, tc.want)
			}
		})
	}
}

// TestRespellNeverSecondGuessesAKnownCard is the guard that keeps a correctly
// spelled card safe. Without it, a real card with a better-scoring neighbour
// could be quietly replaced -- which is the failure mode that would make
// reading names at all a bad trade.
func TestRespellNeverSecondGuessesAKnownCard(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{
		scores: map[string][]Candidate{
			"Cultivate": {{Name: "Cultivator Colossus", Score: 0.99}},
		},
		known: map[string]bool{"Cultivator Colossus": true},
	}
	cards := map[string]*pool.CardRecord{"Cultivate": {Name: "Cultivate"}}
	got, err := Respell(context.Background(), reader, []string{"Cultivate"}, cards)
	if err != nil {
		t.Fatalf("Respell: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a card the pool already knows was re-read as %v", got)
	}
	if len(reader.asked) != 0 {
		t.Errorf("the pool was asked about a name it already knew: %v", reader.asked)
	}
	if cards["Cultivate"].Name != "Cultivate" {
		t.Errorf("the known record was replaced: %v", cards["Cultivate"])
	}
}

// TestRespellReportsWhatItRead is the other half of being allowed to correct
// at all. A silent substitution is the one version of this feature that would
// be worse than the strictness it replaces.
func TestRespellReportsWhatItRead(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{
		scores: map[string][]Candidate{
			"Sol Rng": {{Name: "Sol Ring", Score: 0.975}},
		},
		known: map[string]bool{"Sol Ring": true},
	}
	cards := map[string]*pool.CardRecord{}
	read, err := Respell(context.Background(), reader, []string{"Sol Rng"}, cards)
	if err != nil {
		t.Fatalf("Respell: %v", err)
	}
	parsed := decklist.Parse("1 Sol Rng\n1 Forest\n")
	cards["Forest"] = &pool.CardRecord{Name: "Forest", TypeLine: "Basic Land — Forest"}
	cards["Arahbo, Roar of the World"] = &pool.CardRecord{Name: "Arahbo, Roar of the World"}
	report, err := BuildDeck(parsed, cards, Options{
		Slug: "d", Commander: []string{"Arahbo, Roar of the World"}, Read: read})
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	if len(report.Read) != 1 || report.Read[0].Read != "Sol Ring" {
		t.Fatalf("the report does not carry the correction: %v", report.Read)
	}
	if report.Read[0].Written != "Sol Rng" {
		t.Errorf("the correction lost what was written: %v", report.Read[0])
	}
	// And NOT as prose in the notes: the typed pair is the record, so a
	// caller renders it rather than parsing a sentence out of `Notes`, and
	// the import page does not print the same fact twice.
	for _, n := range report.Notes {
		if strings.Contains(n, "Sol Rng") {
			t.Errorf("the correction was duplicated into a note: %q", n)
		}
	}
	// The deck holds the real card, by its real name.
	if len(report.Deck.Cards) == 0 || report.Deck.Cards[0].Name != "Sol Ring" {
		t.Errorf("the deck did not get the real card: %v", report.Deck.Cards)
	}
	if len(report.Unknown) != 0 {
		t.Errorf("a name that was read is still reported unknown: %v", report.Unknown)
	}
}

// The rationale column, read against a pool.
//
// This is the half `decklist` deliberately cannot do. The grammar hands up two
// readings of a line that ends in a quoted run and refuses to choose; the
// choice is a lookup, and these are the four shapes it has to get right.
func TestReadRationalesAsksThePoolWhichReadingItIs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		line     string
		known    []string
		wantName string
		wantWhy  string
		wantNote bool
		why      string
	}{
		{
			name:     "a reason on a card the pool knows",
			line:     `1 Sol Ring (LTC) 284 "fast mana, every deck"`,
			known:    []string{"Sol Ring"},
			wantName: "Sol Ring",
			wantWhy:  "fast mana, every deck",
			why:      "the peeled reading resolves, so the quoted run is a reason",
		},
		{
			name:     "an epithet the pool knows as part of the name",
			line:     `1 Kongming, "Sleeping Dragon"`,
			known:    []string{`Kongming, "Sleeping Dragon"`},
			wantName: `Kongming, "Sleeping Dragon"`,
			wantWhy:  "",
			wantNote: true,
			why:      "the whole reading resolves and the peeled one does not",
		},
		{
			name:     "that same card WITH a reason",
			line:     `1 Kongming, "Sleeping Dragon" "a lord I keep cutting"`,
			known:    []string{`Kongming, "Sleeping Dragon"`},
			wantName: `Kongming, "Sleeping Dragon"`,
			wantWhy:  "a lord I keep cutting",
			why:      "only the LAST quoted run is the column, so the peel is already right",
		},
		{
			name:     "a misspelling with a reason",
			line:     `1 Sol Rng "fast mana, every deck"`,
			known:    []string{"Sol Ring"},
			wantName: "Sol Rng",
			wantWhy:  "fast mana, every deck",
			why: "the pool knows neither reading, so the peeled one is kept and " +
				"Respell gets a name to score instead of a name plus a sentence",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cards := map[string]*pool.CardRecord{}
			for _, n := range tc.known {
				cards[n] = &pool.CardRecord{Name: n}
			}
			read, notes := ReadRationales(decklist.Parse(tc.line), cards)
			if len(read.Cards) != 1 {
				t.Fatalf("read %d cards, wanted 1 (%s)", len(read.Cards), tc.why)
			}
			got := read.Cards[0]
			if got.Name != tc.wantName {
				t.Errorf("name: got %q, wanted %q (%s)", got.Name, tc.wantName, tc.why)
			}
			if got.Why != tc.wantWhy {
				t.Errorf("why: got %q, wanted %q (%s)", got.Why, tc.wantWhy, tc.why)
			}
			if got.Unpeeled != "" {
				t.Errorf("Unpeeled survived as %q; a chosen reading leaves no choice "+
					"behind, or the second NamesIn asks about both again", got.Unpeeled)
			}
			if gotNote := len(notes) > 0; gotNote != tc.wantNote {
				t.Errorf("notes %v, wanted a note: %v -- a reason somebody thought "+
					"they wrote and did not get has to be said out loud", notes, tc.wantNote)
			}
		})
	}
}

// The column is the user's own words reaching the deck file, which is the
// whole point of it: rule 4 forbids a rationale the tool composed, and this
// composes nothing.
func TestAPastedReasonBecomesTheCardsWhy(t *testing.T) {
	t.Parallel()
	text := "1 Arahbo, Roar of the World (C17) 27 *CMDR*\n" +
		"1 Sol Ring \"fast mana, and it never gets cut\"\n" +
		"1 Cultivate\n"
	cards := map[string]*pool.CardRecord{
		"Arahbo, Roar of the World": {Name: "Arahbo, Roar of the World"},
		"Sol Ring":                  {Name: "Sol Ring"},
		"Cultivate":                 {Name: "Cultivate"},
	}
	read, notes := ReadRationales(decklist.Parse(text), cards)
	report, err := BuildDeck(read, cards, Options{Slug: "arahbo-cats", Notes: notes})
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	byName := map[string]string{}
	for _, c := range report.Deck.Cards {
		byName[c.Name] = c.Why
	}
	if got := byName["Sol Ring"]; got != "fast mana, and it never gets cut" {
		t.Errorf("Sol Ring's why is %q; the quoted column is carried verbatim", got)
	}
	if got := byName["Cultivate"]; got != "" {
		t.Errorf("Cultivate's why is %q; a card that arrived without a reason "+
			"still arrives without one, and nothing here writes it", got)
	}
	if report.Rationales != 1 {
		t.Errorf("counted %d rationales, wanted 1", report.Rationales)
	}
	if report.NeedsRationale() != 1 {
		t.Errorf("%d still owed, wanted 1 (Cultivate)", report.NeedsRationale())
	}
	// The file's own header is part of what the person reads, and a deck that
	// arrived with reasons was not "NOT yet reasoned about".
	if strings.Contains(report.YAML, "NOT yet reasoned about") {
		t.Error("the header still calls a deck with a written reason unreasoned about")
	}
	if !strings.Contains(report.YAML, "1 of its reasons written") {
		t.Errorf("the header does not say what arrived:\n%s", report.YAML)
	}
	if !strings.Contains(report.YAML, "fast mana, and it never gets cut") {
		t.Error("the reason did not reach the deck file")
	}
}

// The category column reaches the card entry, and the word the person wrote
// outranks the one that would have been inferred.
func TestAPastedCategoryFilesTheCard(t *testing.T) {
	t.Parallel()
	text := "1 Arahbo, Roar of the World (C17) 27 *CMDR*\n" +
		"1 Sol Ring \"fast mana\" [ramp]\n" +
		"1 Rhystic Study [CARD-ADVANTAGE]\n" +
		"1 Cultivate\n" +
		"1 Forest [ramp]\n"
	cards := map[string]*pool.CardRecord{
		"Arahbo, Roar of the World": {Name: "Arahbo, Roar of the World"},
		"Sol Ring":                  {Name: "Sol Ring"},
		"Rhystic Study":             {Name: "Rhystic Study"},
		"Cultivate":                 {Name: "Cultivate"},
		"Forest":                    {Name: "Forest", TypeLine: "Basic Land — Forest"},
	}
	read, notes := ReadRationales(decklist.Parse(text), cards)
	report, err := BuildDeck(read, cards, Options{Slug: "arahbo-cats", Notes: notes})
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	filed := map[string]string{}
	why := map[string]string{}
	for _, c := range report.Deck.Cards {
		filed[c.Name], why[c.Name] = c.Category, c.Why
	}
	for _, tc := range []struct{ card, want, because string }{
		{"Sol Ring", "ramp", "the word in brackets is what the person filed it under"},
		{"Rhystic Study", "card-advantage", "the match is case-folded, and the stored value is the model's own casing"},
		{"Cultivate", "utility", "a line with no category still takes the default"},
		{"Forest", "ramp", "a stated category outranks the land inference, which exists to fill a blank"},
	} {
		if got := filed[tc.card]; got != tc.want {
			t.Errorf("%s filed as %q, wanted %q -- %s", tc.card, got, tc.want, tc.because)
		}
	}
	if got := why["Sol Ring"]; got != "fast mana" {
		t.Errorf("Sol Ring's why is %q; the category column must not disturb the reason", got)
	}
	for _, note := range report.Notes {
		if strings.Contains(note, "not ones this library files by") {
			t.Errorf("every word here is a real category, but the import complained: %q", note)
		}
	}
}

// A word this library does not file by is said once, not ninety-nine times,
// and it costs the card neither its place nor its reason.
//
// The count is the whole point. Archidekt writes its own free-text category in
// exactly this column, so a straight export can arrive with a hundred words in
// it -- and a hundred complaints would bury the unknown cards and the missing
// reasons under a list of things that are not even wrong.
func TestAnUnplaceableCategoryIsSaidOnceAndCostsNothing(t *testing.T) {
	t.Parallel()
	text := "1 Arahbo, Roar of the World (C17) 27 *CMDR*\n" +
		"1 Sol Ring \"fast mana\" [Big Beaters]\n" +
		"1 Rhystic Study \"the tax\" [Big Beaters]\n" +
		"1 Cultivate [Mana Rocks]\n"
	cards := map[string]*pool.CardRecord{
		"Arahbo, Roar of the World": {Name: "Arahbo, Roar of the World"},
		"Sol Ring":                  {Name: "Sol Ring"},
		"Rhystic Study":             {Name: "Rhystic Study"},
		"Cultivate":                 {Name: "Cultivate"},
	}
	read, notes := ReadRationales(decklist.Parse(text), cards)
	report, err := BuildDeck(read, cards, Options{Slug: "arahbo-cats", Notes: notes})
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if len(report.Deck.Cards) != 3 {
		t.Fatalf("%d cards landed, wanted 3: an unreadable category must not cost "+
			"anybody a card", len(report.Deck.Cards))
	}
	for _, c := range report.Deck.Cards {
		if c.Category != "utility" {
			t.Errorf("%s filed as %q; a word that could not be placed leaves the "+
				"card filed the way it would have been anyway", c.Name, c.Category)
		}
	}
	if report.Rationales != 2 {
		t.Errorf("counted %d rationales, wanted 2: the reason survives a category "+
			"nobody could place", report.Rationales)
	}
	said := []string{}
	for _, note := range report.Notes {
		if strings.Contains(note, "not ones this library files by") {
			said = append(said, note)
		}
	}
	if len(said) != 1 {
		t.Fatalf("%d notes about category words, wanted exactly 1:\n%s",
			len(said), strings.Join(said, "\n"))
	}
	// Two distinct words over three lines: the count is of words, not of lines,
	// so a category written on ninety-nine cards is still one word.
	if !strings.Contains(said[0], "2 category word(s)") {
		t.Errorf("the note counts something other than the distinct words: %q", said[0])
	}
	for _, word := range []string{"Big Beaters", "Mana Rocks"} {
		if !strings.Contains(said[0], word) {
			t.Errorf("the note does not name %q, so nobody can act on it: %q", word, said[0])
		}
	}
	// Naming the real ones is what makes the sentence actionable rather than a
	// complaint, and reading them off the model is what keeps this honest when
	// the vocabulary changes.
	for _, real := range reference.Deck().Categories {
		if !strings.Contains(said[0], real) {
			t.Errorf("the note does not offer %q as a category anybody could use: %q",
				real, said[0])
		}
	}
}

// A deck that arrived complete is told so, because the next thing its owner
// wants is the promote control and not a lecture about empty fields.
func TestAFullyReasonedImportSaysNothingIsOwed(t *testing.T) {
	t.Parallel()
	text := "1 Arahbo, Roar of the World (C17) 27 *CMDR* \"the whole deck\"\n" +
		"1 Sol Ring \"fast mana\"\n"
	cards := map[string]*pool.CardRecord{
		"Arahbo, Roar of the World": {Name: "Arahbo, Roar of the World"},
		"Sol Ring":                  {Name: "Sol Ring"},
	}
	read, notes := ReadRationales(decklist.Parse(text), cards)
	report, err := BuildDeck(read, cards, Options{Slug: "arahbo-cats", Notes: notes})
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	if report.NeedsRationale() != 0 {
		t.Fatalf("%d still owed, wanted 0", report.NeedsRationale())
	}
	if !strings.Contains(report.YAML, "Nothing is owed") {
		t.Errorf("the header does not say the deck is complete:\n%s", report.YAML)
	}
	// Still a draft: `draft` is the deck saying nobody has looked at it whole
	// yet, and the gate reporting its problems as warnings is the diagnosis a
	// newcomer wants on the first screen (ADR 13). Promotion is one control
	// away on the deck page.
	if report.Deck.Stage != "draft" {
		t.Errorf("stage is %q; an import lands as a draft however complete it is",
			report.Deck.Stage)
	}
}

// ADR 49: a paste may declare that its quoted reasons were drafted, and then
// every card whose reason actually arrived is marked -- never a card whose
// reason is still owed, because a mark on an empty `why` would claim
// authorship of a sentence nobody wrote. The swap board's reasons came from
// the same hand and take the same mark, and the commander's -- which lands as
// a note, where no mark can travel -- is named out loud in the report instead.
func TestTheDeclaredHandMarksOnlyWhatItWrote(t *testing.T) {
	t.Parallel()
	text := "1 Arahbo, Roar of the World (C17) 27 *CMDR* \"the whole deck\"\n" +
		"1 Sol Ring \"fast mana, and it never gets cut\"\n" +
		"1 Cultivate\n" +
		"Sideboard:\n" +
		"1 Beast Within \"single target removal, waiting its turn\"\n"
	cards := map[string]*pool.CardRecord{
		"Arahbo, Roar of the World": {Name: "Arahbo, Roar of the World"},
		"Sol Ring":                  {Name: "Sol Ring"},
		"Cultivate":                 {Name: "Cultivate"},
		"Beast Within":              {Name: "Beast Within"},
	}

	read, notes := ReadRationales(decklist.Parse(text), cards)
	report, err := BuildDeck(read, cards, Options{
		Slug: "arahbo-cats", Notes: notes, WhyBy: "claude"})
	if err != nil {
		t.Fatalf("building: %v", err)
	}

	marks := map[string]string{}
	for _, c := range append(slices.Clone(report.Deck.Cards), report.Deck.SwapBoard...) {
		marks[c.Name] = c.WhyBy
	}
	if got := marks["Sol Ring"]; got != "claude" {
		t.Errorf("Sol Ring's mark is %q; a carried reason takes the declared hand", got)
	}
	if got := marks["Beast Within"]; got != "claude" {
		t.Errorf("Beast Within's mark is %q; the swap board's reasons were "+
			"drafted by the same hand", got)
	}
	if got := marks["Cultivate"]; got != "" {
		t.Errorf("Cultivate's mark is %q; a reason still owed takes no mark", got)
	}
	// The indented form counts only the marks on cards -- the drafted header
	// names `why_by: claude` in its own prose, and that mention is not a mark.
	if got := strings.Count(report.YAML, "\n    why_by: claude"); got != 2 {
		t.Errorf("the file carries %d marks, wanted 2:\n%s", got, report.YAML)
	}
	// The file's own first paragraph must not claim an authorship its marks
	// deny: a declared paste gets the drafted header, not the person one.
	if strings.Contains(report.YAML, "by the person who pasted it") {
		t.Errorf("the header claims the person wrote reasons the marks call "+
			"drafted:\n%s", report.YAML)
	}
	if !strings.Contains(report.YAML, "reasons drafted by") {
		t.Errorf("the header does not say the reasons were drafted:\n%s", report.YAML)
	}
	zoneNamed := false
	for _, n := range report.Notes {
		if strings.Contains(n, "where no mark can follow it") {
			zoneNamed = true
		}
	}
	if !zoneNamed {
		t.Errorf("the commander's drafted sentence went to `notes` unnamed; "+
			"the report must say it aloud: %v", report.Notes)
	}

	// The same paste with nothing declared: nobody is marked, because an
	// unmarked reason means a person wrote it and that claim must never be
	// manufactured by a default.
	read, notes = ReadRationales(decklist.Parse(text), cards)
	unmarked, err := BuildDeck(read, cards, Options{Slug: "arahbo-cats", Notes: notes})
	if err != nil {
		t.Fatalf("building the undeclared paste: %v", err)
	}
	if strings.Contains(unmarked.YAML, "why_by") {
		t.Errorf("a paste that declared nothing grew a mark:\n%s", unmarked.YAML)
	}
	if !strings.Contains(unmarked.YAML, "by the person who pasted it") {
		t.Errorf("an undeclared paste keeps the person's own header:\n%s", unmarked.YAML)
	}
}
