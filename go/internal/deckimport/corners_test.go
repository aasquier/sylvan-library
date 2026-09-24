package deckimport

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/decklist"
	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// The intake's corners: the paste that is not a tidy export, and the pool
// that will not finish the sentence it started.
//
// Reading a misspelling at all is a trade Aaron ruled on (2026-08-24), and
// the whole trade rests on the reading being **conservative and reported**.
// So the two things held here are: the reader is never asked about a name
// twice or about nothing, and a pool that scores a name and then will not
// hand it over leaves the miss a miss -- an invented card would be exactly
// the silent substitution this feature was allowed on condition of avoiding.

// stubbornReader scores everything and then refuses, at whichever step the
// test asks it to.
type stubbornReader struct {
	scores    map[string][]Candidate
	failNear  bool
	failCards bool
	// forgets is a name it will score and then not hand over: a pool that
	// changed under a two-pass read.
	forgets map[string]bool
}

var errThePoolStopped = errors.New("the pool stopped answering")

func (s *stubbornReader) Nearest(_ context.Context, written string, _ int) ([]Candidate, error) {
	if s.failNear {
		return nil, errThePoolStopped
	}
	return s.scores[written], nil
}

func (s *stubbornReader) Cards(_ context.Context, names []string) (map[string]*pool.CardRecord, error) {
	if s.failCards {
		return nil, errThePoolStopped
	}
	out := map[string]*pool.CardRecord{}
	for _, n := range names {
		if !s.forgets[n] {
			out[n] = &pool.CardRecord{Name: n}
		}
	}
	return out, nil
}

// With no pool to read against, nothing is read -- and that is the laptop's
// shape, not a failure. An import still lands; the misspellings simply stay
// unknown cards, which is what they were before any of this existed.
func TestWithNoPoolToReadAgainstNothingIsRead(t *testing.T) {
	t.Parallel()
	cards := map[string]*pool.CardRecord{}
	got, err := Respell(context.Background(), nil, []string{"Sol Rng"}, cards)
	if err != nil {
		t.Fatalf("Respell with no reader: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a name was read with no pool to read it against: %v", got)
	}
	if len(cards) != 0 {
		t.Errorf("the lookup gained %v", cards)
	}
}

// A paste that repeats a name, or carries a blank line, asks the pool once
// and about nothing blank. It matters because scoring is the expensive half:
// a ninety-nine card list with a typo written four times is four queries
// where one will do, and a blank is a query about nothing.
func TestTheReaderIsAskedOncePerNameAndNeverAboutNothing(t *testing.T) {
	t.Parallel()
	asked := &countingReader{inner: &stubbornReader{scores: map[string][]Candidate{}}}
	names := []string{"Sol Rng", "Sol Rng", "", "   ", "Sol Rng", "Forrest"}
	if _, err := Respell(context.Background(), asked, names,
		map[string]*pool.CardRecord{}); err != nil {
		t.Fatalf("Respell: %v", err)
	}
	if asked.calls["Sol Rng"] != 1 {
		t.Errorf("a repeated name was scored %d times", asked.calls["Sol Rng"])
	}
	if asked.calls["Forrest"] != 1 {
		t.Errorf("the second name was scored %d times", asked.calls["Forrest"])
	}
	for _, blank := range []string{"", "   "} {
		if asked.calls[blank] != 0 {
			t.Errorf("the pool was asked about a blank line %d times", asked.calls[blank])
		}
	}
}

type countingReader struct {
	inner *stubbornReader
	calls map[string]int
}

func (c *countingReader) Nearest(ctx context.Context, written string, limit int) ([]Candidate, error) {
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[written]++
	return c.inner.Nearest(ctx, written, limit)
}

func (c *countingReader) Cards(ctx context.Context, names []string) (map[string]*pool.CardRecord, error) {
	return c.inner.Cards(ctx, names)
}

// A pool that stops answering halfway through stops the read, at either of
// the two passes. The import fails rather than landing a deck whose names
// were partly corrected -- half a reading is not a reading anybody agreed to.
func TestAPoolThatStopsAnsweringStopsTheReadingRatherThanHalfDoingIt(t *testing.T) {
	t.Parallel()
	scores := map[string][]Candidate{"Sol Rng": {{Name: "Sol Ring", Score: 0.975}}}

	for _, tc := range []struct {
		what   string
		reader *stubbornReader
	}{
		{"while scoring", &stubbornReader{scores: scores, failNear: true}},
		{"while fetching the winners", &stubbornReader{scores: scores, failCards: true}},
	} {
		cards := map[string]*pool.CardRecord{}
		got, err := Respell(context.Background(), tc.reader, []string{"Sol Rng"}, cards)
		if err == nil {
			t.Errorf("a pool that stopped %s still produced %v", tc.what, got)
		}
		if len(cards) != 0 {
			t.Errorf("a pool that stopped %s still installed %v", tc.what, cards)
		}
	}
}

// A pool that scores a name and then will not hand the card over leaves the
// miss a miss. Nothing is invented on the way past: the alternative is a deck
// holding a card nobody can look up, under a name nobody typed.
func TestANameScoredAndThenWithheldStaysAMiss(t *testing.T) {
	t.Parallel()
	reader := &stubbornReader{
		scores:  map[string][]Candidate{"Sol Rng": {{Name: "Sol Ring", Score: 0.975}}},
		forgets: map[string]bool{"Sol Ring": true},
	}
	cards := map[string]*pool.CardRecord{}
	got, err := Respell(context.Background(), reader, []string{"Sol Rng"}, cards)
	if err != nil {
		t.Fatalf("Respell: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a card the pool would not hand over was reported as read: %v", got)
	}
	if _, installed := cards["Sol Rng"]; installed {
		t.Error("a record the pool never handed over was installed under the " +
			"written name")
	}
}

// ---- what the paste says about the command zone ---------------------------

// A companion nominated by the paste itself is taken, without anybody having
// to fill in the form field beside it.
func TestACompanionTheListNominatesIsTaken(t *testing.T) {
	t.Parallel()
	parsed := decklist.Parse("Commander\n1 Fixture Leader\n\nCompanion\n1 Fixture Follower\n\n" +
		"Deck\n1 Fixture Signet\n")
	cards := map[string]*pool.CardRecord{
		"Fixture Leader":   {Name: "Fixture Leader", TypeLine: "Legendary Creature — Bear"},
		"Fixture Follower": {Name: "Fixture Follower", TypeLine: "Legendary Creature — Cat"},
		"Fixture Signet":   {Name: "Fixture Signet", TypeLine: "Artifact"},
	}
	report, err := BuildDeck(parsed, cards, Options{Slug: "companion"})
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	if report.Deck.Companion == nil || *report.Deck.Companion != "Fixture Follower" {
		t.Fatalf("the companion came out as %v", report.Deck.Companion)
	}
	// And it is not also in the 99, which is the mistake the command zone
	// exists to prevent.
	for _, c := range report.Deck.Cards {
		if c.Name == "Fixture Follower" {
			t.Error("the companion is also in the 99")
		}
	}
}

// Two reasons about two cards in the command zone are labelled with the card
// each was about.
//
// One reason is kept exactly as it was written; two unattributed sentences in
// one note is a worse record of what somebody said than two attributed ones,
// and the note is the only place that sentence survives the import.
func TestTwoCommandZoneReasonsAreLabelledWithTheCardsTheyAreAbout(t *testing.T) {
	t.Parallel()
	parsed := decklist.Parse(
		"Commander\n1 Fixture Leader \"she is the whole plan\"\n\n" +
			"Companion\n1 Fixture Follower \"the deck was already built for her\"\n\n" +
			"Deck\n1 Fixture Signet \"it makes mana\"\n")
	cards := map[string]*pool.CardRecord{
		"Fixture Leader":   {Name: "Fixture Leader", TypeLine: "Legendary Creature — Bear"},
		"Fixture Follower": {Name: "Fixture Follower", TypeLine: "Legendary Creature — Cat"},
		"Fixture Signet":   {Name: "Fixture Signet", TypeLine: "Artifact"},
	}
	report, err := BuildDeck(parsed, cards, Options{Slug: "zone"})
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	note := ""
	for _, kv := range report.Deck.Notes {
		if kv.Key == "command_zone" {
			note, _ = kv.Value.(string)
		}
	}
	if note == "" {
		t.Fatalf("the command zone kept no reasons: %v", report.Deck.Notes)
	}
	for _, want := range []string{
		"Fixture Leader: she is the whole plan",
		"Fixture Follower: the deck was already built for her",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("the note is %q and does not carry %q", note, want)
		}
	}
	// The 99's own reason stays with its card rather than being swept in.
	if strings.Contains(note, "it makes mana") {
		t.Errorf("a card's own reason was swept into the command zone note: %q", note)
	}
}

// A paste full of somebody else's category words says so once, names a few
// and counts the rest.
//
// Archidekt writes its own free-text column here -- "Big Beaters", "Card
// Draw" -- so a straight export can arrive with ninety-nine words in it.
// Ninety-nine identical complaints would bury the unknown cards and the
// missing reasons underneath a list of things that are not wrong.
func TestAPasteFullOfSomebodyElsesCategoriesSaysSoOnce(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("Commander\n1 Fixture Leader\n\nDeck\n")
	cards := map[string]*pool.CardRecord{
		"Fixture Leader": {Name: "Fixture Leader", TypeLine: "Legendary Creature — Bear"},
	}
	// Nine cards, each filed under a different word this library does not
	// know: more than the six a sentence shows.
	for i := 0; i < 9; i++ {
		name := "Fixture Card " + string(rune('A'+i))
		b.WriteString("1 " + name + " [Somebody Elses Column " + string(rune('A'+i)) + "]\n")
		cards[name] = &pool.CardRecord{Name: name, TypeLine: "Artifact"}
	}

	report, err := BuildDeck(decklist.Parse(b.String()), cards, Options{Slug: "archidekt"})
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	said := 0
	var note string
	for _, n := range report.Notes {
		if strings.Contains(n, "category word(s)") {
			said++
			note = n
		}
	}
	if said != 1 {
		t.Fatalf("the paste produced %d complaints about its categories: %v", said, report.Notes)
	}
	if !strings.Contains(note, "9 category word(s)") {
		t.Errorf("the note is %q and does not count them", note)
	}
	if !strings.Contains(note, "and 3 more") {
		t.Errorf("the note is %q -- it names six and counts the rest", note)
	}
	// The cards themselves landed, filed the way they always were.
	if len(report.Deck.Cards) != 9 {
		t.Errorf("%d of the nine cards landed", len(report.Deck.Cards))
	}
}

// One entry holding two names is read as a pairing, and the reading is
// reported rather than applied silently -- it is a guess about what somebody
// meant, and a deck that quietly gained a second commander is a deck whose
// colour identity changed without anybody saying so.
func TestAnEntryHoldingTwoNamesIsReadAsAPairingAndSaysSo(t *testing.T) {
	t.Parallel()
	parsed := decklist.Parse("Deck\n1 Fixture Signet\n")
	cards := map[string]*pool.CardRecord{
		"Fixture Signet": {Name: "Fixture Signet", TypeLine: "Artifact"},
		"Fixture Alpha":  {Name: "Fixture Alpha", TypeLine: "Legendary Creature — Bear"},
		"Fixture Beta":   {Name: "Fixture Beta", TypeLine: "Legendary Creature — Cat"},
	}
	report, err := BuildDeck(parsed, cards, Options{
		Slug: "pair", Commander: []string{"Fixture Alpha + Fixture Beta"}})
	if err != nil {
		t.Fatalf("BuildDeck: %v", err)
	}
	if len(report.Deck.Commander) != 2 {
		t.Fatalf("the entry was read as %v", report.Deck.Commander)
	}
	said := false
	for _, n := range report.Notes {
		if strings.Contains(n, "read as a pairing") {
			said = true
		}
	}
	if !said {
		t.Errorf("the pairing was read without saying so: %v", report.Notes)
	}
}
