package ledger

import (
	"fmt"
	"math/big"
	"strconv"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// open is the deployment with the door open: one person, nothing to keep from
// them. Every test that is not about the scope reads through it.
var open = Scope{Open: true}

// deckNamed builds a deck the ledger will accept, with the labels a board
// groups on.
func deckNamed(t *testing.T, slug, commander string, themes ...string) *deck.Deck {
	t.Helper()
	text := fmt.Sprintf("slug: %s\nname: %s\ncommander:\n  - %s\n",
		slug, slug, commander)
	if len(themes) > 0 {
		text += "themes:\n"
		for _, th := range themes {
			text += "  - " + th + "\n"
		}
	}
	text += "cards:\n  - name: Sol Ring\n    category: ramp\n    why: fast\n"
	return deckOf(t, text)
}

// splitGames is `wins` wins for seat 1 and the rest for seat 2, which is every
// fixture below that only cares about a record.
func splitGames(wins, total int) []tier3.GameResult {
	out := make([]tier3.GameResult, 0, total)
	for i := 1; i <= total; i++ {
		seat := 2
		if i <= wins {
			seat = 1
		}
		out = append(out, game(i, 1000, intp(seat), intp(8), false, false))
	}
	return out
}

// played is a whole match between two decks, recorded as the house's own.
func played(t *testing.T, rec *Recorder, a, b *deck.Deck, wins, total int) {
	t.Helper()
	rec.Record(t.Context(), matchOf(big.NewInt(1), splitGames(wins, total),
		[]*deck.Deck{a, b}))
}

func find(t *testing.T, b *Standings, slug string) DeckRecord {
	t.Helper()
	for _, d := range b.Decks {
		if d.Slug == slug {
			return d
		}
	}
	t.Fatalf("no deck %q on the board", slug)
	return DeckRecord{}
}

// TestATwoGameStreakDoesNotTopTheBoard is the whole small-sample design in one
// test, and the reason the board sorts on the interval rather than the rate.
//
// A deck that won both its bouts has a perfect record. A deck that won forty
// of sixty-five has a merely good one. Sorted by rate, the first is champion
// of a leaderboard on the strength of two coin flips — which is precisely the
// confidently-wrong board commandment 2 forbids. Sorted by the lower bound of
// what has actually been shown, the deck that has proved something is ahead.
func TestATwoGameStreakDoesNotTopTheBoard(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	lucky := deckNamed(t, "lucky", "Arahbo, Roar of the World", "cats")
	proven := deckNamed(t, "proven", "Atla Palani, Nest Tender", "dinosaurs")
	foil := deckNamed(t, "foil", "Goreclaw, Terror of Qal Sisma", "stompy")

	played(t, rec, lucky, foil, 2, 2)
	played(t, rec, proven, foil, 40, 65)

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}

	if board.Decks[0].Slug != "proven" {
		t.Errorf("the board is led by %q; a 2-0 outsorted a 40-of-65,"+
			" which is the leaderboard bug this rule exists to stop",
			board.Decks[0].Slug)
	}

	l, p := find(t, board, "lucky"), find(t, board, "proven")
	if !(l.Record.Lower < p.Record.Lower) {
		t.Errorf("lower bounds %v (2-0) and %v (40-65) do not separate the two",
			float64(l.Record.Lower), float64(p.Record.Lower))
	}
	// The raw rate genuinely does favour the outsider. If it ever stops
	// doing so this test has become a tautology and proves nothing.
	if l.Record.Rate != nil {
		t.Errorf("a two-game record was given a rate of %v", float64(*l.Record.Rate))
	}
	if p.Record.Rate == nil {
		t.Fatal("a sixty-five game record was withheld a rate")
	}
	if float64(*p.Record.Rate) >= 1 {
		t.Errorf("rate %v", float64(*p.Record.Rate))
	}
}

// TestARateIsWithheldBelowTheFloor is the second rule: below [RateFloor] there
// is no rate at all, and the denominator is present either way.
func TestARateIsWithheldBelowTheFloor(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	foil := deckNamed(t, "foil", "Goreclaw, Terror of Qal Sisma", "stompy")

	// One short of the floor, and exactly on it.
	shy := deckNamed(t, "shy", "Arahbo, Roar of the World", "cats")
	atFloor := deckNamed(t, "atfloor", "Atla Palani, Nest Tender", "dinosaurs")
	played(t, rec, shy, foil, RateFloor-1, RateFloor-1)
	played(t, rec, atFloor, foil, RateFloor, RateFloor)

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}

	below := find(t, board, "shy")
	if below.Record.Rate != nil {
		t.Errorf("a record of %d games was given a rate", below.Record.Played)
	}
	if below.Record.Played != RateFloor-1 {
		t.Errorf("played %d, want %d", below.Record.Played, RateFloor-1)
	}
	if below.Record.Settled {
		t.Error("a record below the floor was called settled")
	}

	on := find(t, board, "atfloor")
	if on.Record.Rate == nil {
		t.Fatalf("a record of exactly %d games was withheld a rate", RateFloor)
	}
	if float64(*on.Record.Rate) != 1 {
		t.Errorf("rate %v, want 1", float64(*on.Record.Rate))
	}
	// The floor and the proven line travel on the wire, so one surface
	// cannot drift from another about what "too few" means.
	if board.Floor != RateFloor || board.Proven != Proven {
		t.Errorf("board carries floor %d proven %d", board.Floor, board.Proven)
	}
}

// TestTheIntervalStaysOnTheLine pins the properties the sort depends on: a
// bound is always a probability, and it never collapses to a point.
func TestTheIntervalStaysOnTheLine(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ wins, n int }{
		{0, 0}, {0, 1}, {1, 1}, {0, 10}, {10, 10}, {5, 10}, {1, 1000},
		{999, 1000},
	} {
		lower, upper := wilson(c.wins, c.n)
		if lower < 0 || lower > 1 || upper < 0 || upper > 1 {
			t.Errorf("%d of %d gave [%v, %v], off the line", c.wins, c.n, lower, upper)
		}
		if lower > upper {
			t.Errorf("%d of %d gave an inverted interval [%v, %v]",
				c.wins, c.n, lower, upper)
		}
		if c.n > 0 && lower == upper {
			t.Errorf("%d of %d collapsed the interval to %v; the sort key"+
				" stops separating anything", c.wins, c.n, lower)
		}
	}
	// Nothing observed excludes nothing.
	if lower, upper := wilson(0, 0); lower != 0 || upper != 1 {
		t.Errorf("an empty record gave [%v, %v], want the whole line", lower, upper)
	}
	// More evidence is a narrower interval. This is the property that makes
	// the lower bound a fair sort key rather than an arbitrary one.
	narrow, wide := width(wilson(50, 100)), width(wilson(5, 10))
	if narrow >= wide {
		t.Errorf("100 games gave a %v-wide interval and 10 games %v; evidence"+
			" is not narrowing anything", narrow, wide)
	}
}

func width(lower, upper float64) float64 { return upper - lower }

// TestAClockOutCountsForNobodyOnTheBoard carries ADR 36's second rule into the
// aggregate: a game the measurement gave up on is evidence about the clock and
// must not silently vanish between the ledger and the board.
func TestAClockOutCountsForNobodyOnTheBoard(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	a := deckNamed(t, "cats", "Arahbo, Roar of the World", "cats")
	b := deckNamed(t, "dinos", "Atla Palani, Nest Tender", "dinosaurs")

	rec.Record(t.Context(), matchOf(big.NewInt(3), []tier3.GameResult{
		game(1, 1000, intp(1), intp(8), false, false),
		game(2, 1000, intp(2), intp(8), false, false),
		// A real draw: in the denominator, and a win for neither.
		game(3, 1000, nil, intp(8), true, false),
		// A clock-out with a winner line printed after giving up: out of
		// the denominator entirely, reported apart.
		game(4, 300000, intp(1), nil, false, true),
	}, []*deck.Deck{a, b}))

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	cats := find(t, board, "cats")
	if cats.Record.Played != 3 {
		t.Errorf("played %d, want 3 — the clock-out is not a game anybody played",
			cats.Record.Played)
	}
	if cats.Record.Wins != 1 {
		t.Errorf("wins %d, want 1; the clock-out's winner line was counted",
			cats.Record.Wins)
	}
	if cats.Record.Draws != 1 {
		t.Errorf("draws %d, want 1", cats.Record.Draws)
	}
	if cats.Record.Losses != 1 {
		t.Errorf("losses %d, want 1", cats.Record.Losses)
	}
	if cats.Record.TimedOut != 1 {
		t.Errorf("timed out %d, want 1 — a clock-out reported apart is the"+
			" only way a surface can say where the missing game went",
			cats.Record.TimedOut)
	}
	// The house count is every game the Forge ran, clock-outs included, so
	// the difference from the records is visible rather than lost.
	if board.Games != 4 || board.TimedOut != 1 {
		t.Errorf("board games %d timed out %d, want 4 and 1", board.Games, board.TimedOut)
	}
}

// TestAPodIsNotAHeadToHead is the meetings rule: three other decks were in the
// room, so nobody beat anybody in particular.
func TestAPodIsNotAHeadToHead(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	a := deckNamed(t, "cats", "Arahbo, Roar of the World", "cats")
	b := deckNamed(t, "dinos", "Atla Palani, Nest Tender", "dinosaurs")
	c := deckNamed(t, "bears", "Goreclaw, Terror of Qal Sisma", "stompy")
	d := deckNamed(t, "food", "Gyome, Master Chef", "food")

	rec.Record(t.Context(), matchOf(big.NewInt(4), splitGames(3, 4),
		[]*deck.Deck{a, b, c, d}))

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Meetings) != 0 {
		t.Errorf("a four-seat match manufactured %d head-to-head rows",
			len(board.Meetings))
	}
	// The pod's games still count where they mean what they say.
	if find(t, board, "cats").Record.Wins != 3 {
		t.Error("the pod's games went missing from the deck board")
	}

	// And a two-seat match does make one.
	played(t, rec, a, b, 6, 10)
	board, err = rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Meetings) != 1 {
		t.Fatalf("%d meetings, want 1", len(board.Meetings))
	}
	m := board.Meetings[0]
	if m.Played != 10 || m.AWins+m.BWins != 10 {
		t.Errorf("meeting played %d, %d/%d wins", m.Played, m.AWins, m.BWins)
	}
	// A pair is stored in one stable orientation, so it cannot drift into
	// two mirror rows.
	if m.A.Slug != "cats" || m.B.Slug != "dinos" {
		t.Errorf("meeting oriented %q vs %q", m.A.Slug, m.B.Slug)
	}
}

// TestAMeetingAccumulatesAcrossMatches — the same two decks meeting twice is
// one line with both matches on it, not two lines.
func TestAMeetingAccumulatesAcrossMatches(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	a := deckNamed(t, "cats", "Arahbo, Roar of the World", "cats")
	b := deckNamed(t, "dinos", "Atla Palani, Nest Tender", "dinosaurs")
	played(t, rec, a, b, 3, 4)
	played(t, rec, a, b, 1, 4)

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Meetings) != 1 {
		t.Fatalf("%d meetings, want 1", len(board.Meetings))
	}
	m := board.Meetings[0]
	if m.Matches != 2 || m.Played != 8 || m.AWins != 4 || m.BWins != 4 {
		t.Errorf("meeting %d matches %d played %d-%d", m.Matches, m.Played,
			m.AWins, m.BWins)
	}
	if find(t, board, "cats").Matches != 2 {
		t.Error("the deck board lost a match")
	}
}

// TestTheClassBoardGroupsOnTheRecordedLabel keeps ADR 36's snapshot rule in the
// aggregate, and leaves the unlabelled off the board entirely.
func TestTheClassBoardGroupsOnTheRecordedLabel(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	a := deckNamed(t, "cats", "Arahbo, Roar of the World", "aggro")
	b := deckNamed(t, "dinos", "Atla Palani, Nest Tender", "midrange")
	bare := deckNamed(t, "bare", "Goreclaw, Terror of Qal Sisma")
	played(t, rec, a, b, 7, 10)
	played(t, rec, bare, a, 2, 6)

	// Relabelled afterwards. History must not move.
	a.Themes = []string{"control"}

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	classes := map[string]ClassRecord{}
	for _, c := range board.Archetypes {
		classes[c.Archetype] = c
	}
	if _, ok := classes["control"]; ok {
		t.Error("a relabelling rewrote a class the deck never played as")
	}
	if _, ok := classes[""]; ok {
		t.Error("the unlabelled were pooled into a class of their own")
	}
	aggro, ok := classes["aggro"]
	if !ok {
		t.Fatalf("no aggro class; got %v", board.Archetypes)
	}
	// aggro won 7 of 10 in the first match and lost 2 of 6 in the second.
	if aggro.Record.Played != 16 || aggro.Record.Wins != 11 {
		t.Errorf("aggro %d of %d", aggro.Record.Wins, aggro.Record.Played)
	}
	if aggro.Decks != 1 {
		t.Errorf("aggro counted %d decks, want 1", aggro.Decks)
	}
}

// TestTheBoardIsScopedToTheViewer is ADR 5 in the ledger: another account's
// match is not there at all.
func TestTheBoardIsScopedToTheViewer(t *testing.T) {
	t.Parallel()
	rec, db := scratch(t)
	mine := deckNamed(t, "mine", "Arahbo, Roar of the World", "cats")
	yours := deckNamed(t, "yours", "Atla Palani, Nest Tender", "dinosaurs")
	house := deckNamed(t, "house", "Syr Gwyn, Hero of Ashvale", "knights")

	// Real accounts: `forge_seats.owner_id` is a foreign key, so a seat
	// attributed to a user who does not exist is a match recorded nowhere.
	for _, q := range []string{
		`INSERT INTO users (id, username, email, created_at)` +
			` VALUES (7, 'me', 'me@example.com', '2026-08-26T00:00:00')`,
		`INSERT INTO users (id, username, email, created_at)` +
			` VALUES (9, 'you', 'you@example.com', '2026-08-26T00:00:00')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}

	me, you := int64(7), int64(9)
	rec.Record(t.Context(), Match{
		Run:   &tier3.SimRun{Output: tier3.SimOutput{Games: splitGames(2, 3)}},
		Decks: []*deck.Deck{mine, house}, Clock: 300, GamesRequested: 3,
		OwnerIDs: []*int64{&me, nil}})
	rec.Record(t.Context(), Match{
		Run:   &tier3.SimRun{Output: tier3.SimOutput{Games: splitGames(3, 3)}},
		Decks: []*deck.Deck{yours, house}, Clock: 300, GamesRequested: 3,
		OwnerIDs: []*int64{&you, nil}})

	// And an exhibition on the house's own shelf, owned by nobody.
	shelf := deckNamed(t, "shelf", "Jetmir, Nexus of Revels", "tokens")
	played(t, rec, house, shelf, 2, 3)

	board, err := rec.Board(t.Context(), Scope{Viewer: me})
	if err != nil {
		t.Fatal(err)
	}
	// My own match, and the house's exhibition. Not the other account's —
	// and *that* is the case the cheap rule got wrong: both of us fought a
	// house deck, so "any house seat makes it public" would have handed me
	// their whole record.
	if board.Matches != 2 {
		t.Errorf("the viewer sees %d matches, want 2 (mine and the house's)",
			board.Matches)
	}
	for _, d := range board.Decks {
		if d.Slug == "yours" {
			t.Error("another account's deck is on the viewer's board")
		}
	}
	// A match is the unit of entitlement: the opponent seat is visible or
	// the viewer's own record would have losses with no author.
	if _, err := findMaybe(board, "house"); err != nil {
		t.Error("the house's own seat vanished from a match the viewer may see")
	}
	// The house's own record on my board counts only the matches I may see,
	// so it cannot become a side channel onto the other account's games.
	if h, err := findMaybe(board, "house"); err == nil && h.Record.Played != 6 {
		t.Errorf("the house shows %d games on the viewer's board, want 6"+
			" — a third account's match leaked in through a shared opponent",
			h.Record.Played)
	}

	// The other account sees the mirror image.
	theirs, err := rec.Board(t.Context(), Scope{Viewer: you})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range theirs.Decks {
		if d.Slug == "mine" {
			t.Error("the viewer's deck is on another account's board")
		}
	}

	// The open deployment sees everything, because there is nobody to keep
	// anything from.
	all, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	if all.Matches != 3 {
		t.Errorf("the open deployment sees %d matches, want 3", all.Matches)
	}
}

func findMaybe(b *Standings, slug string) (DeckRecord, error) {
	for _, d := range b.Decks {
		if d.Slug == slug {
			return d, nil
		}
	}
	return DeckRecord{}, fmt.Errorf("no deck %q", slug)
}

// TestAnEmptyLedgerIsAnEmptyBoard — the state most accounts are in, and the
// one a nil slice would render as a crash rather than a welcome.
func TestAnEmptyLedgerIsAnEmptyBoard(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	if board.Matches != 0 || board.Games != 0 {
		t.Errorf("an empty ledger reported %d matches and %d games",
			board.Matches, board.Games)
	}
	if board.Decks == nil || board.Archetypes == nil || board.Meetings == nil {
		t.Error("an empty board carries a nil slice, which serialises as" +
			" null and is not a list a surface can walk")
	}
	if board.Since != "" || board.Until != "" {
		t.Errorf("an empty ledger dated itself %q..%q", board.Since, board.Until)
	}
	// A nil recorder is the no-app.db case rather than an error, exactly as
	// it is for writing.
	var none *Recorder
	empty, err := none.Board(t.Context(), open)
	if err != nil || empty == nil || empty.Floor != RateFloor {
		t.Errorf("a nil recorder gave (%v, %v)", empty, err)
	}
}

// TestTheLabelsShownAreTheLatestOnesWorn — the record is history and must not
// move; the name beside it is how you find the deck today.
func TestTheLabelsShownAreTheLatestOnesWorn(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	foil := deckNamed(t, "foil", "Goreclaw, Terror of Qal Sisma", "stompy")
	first := deckNamed(t, "cats", "Arahbo, Roar of the World", "aggro")
	played(t, rec, first, foil, 1, 2)

	// The same deck, later, under a new commander and a new class.
	later := deckNamed(t, "cats", "Jetmir, Nexus of Revels", "midrange")
	played(t, rec, later, foil, 1, 2)

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	cats := find(t, board, "cats")
	if cats.Record.Played != 4 {
		t.Errorf("played %d, want 4 — the tallies span every match", cats.Record.Played)
	}
	if len(cats.Commander) != 1 || cats.Commander[0] != "Jetmir, Nexus of Revels" {
		t.Errorf("the board shows commander %v, want the latest", cats.Commander)
	}
	// But both classes it played as are still on the class board.
	classes := map[string]bool{}
	for _, c := range board.Archetypes {
		classes[c.Archetype] = true
	}
	if !classes["aggro"] || !classes["midrange"] {
		t.Errorf("the class board forgot a class the deck played as: %v", classes)
	}
}

// The split Aaron asked for on 2026-09-06: a deck's duel record and its pod
// record are separate questions, because the baselines differ — 50% heads-up
// against 25% at a four-seat table — and pooling them answers neither.
func TestADecksDuelAndPodRecordsAreKeptApart(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	a := deckNamed(t, "cats", "Arahbo, Roar of the World", "cats")
	b := deckNamed(t, "dinos", "Atla Palani, Nest Tender", "dinosaurs")
	c := deckNamed(t, "bears", "Goreclaw, Terror of Qal Sisma", "stompy")
	d := deckNamed(t, "food", "Gyome, Master Chef", "food")

	// Six of ten heads-up, then three of four in a pod.
	played(t, rec, a, b, 6, 10)
	rec.Record(t.Context(), matchOf(big.NewInt(4), splitGames(3, 4),
		[]*deck.Deck{a, b, c, d}))

	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	cats := find(t, board, "cats")
	if cats.Duel.Played != 10 || cats.Duel.Wins != 6 {
		t.Errorf("the duel record reads %d/%d, want 6 of 10",
			cats.Duel.Wins, cats.Duel.Played)
	}
	if cats.Pod.Played != 4 || cats.Pod.Wins != 3 {
		t.Errorf("the pod record reads %d/%d, want 3 of 4",
			cats.Pod.Wins, cats.Pod.Played)
	}
	// And the pooled record is still the whole of it, so nothing is lost by
	// the split — only separated.
	if cats.Record.Played != 14 || cats.Record.Wins != 9 {
		t.Errorf("the pooled record reads %d/%d, want 9 of 14",
			cats.Record.Wins, cats.Record.Played)
	}
	if board.Duels != 1 || board.Pods != 1 {
		t.Errorf("the board counted %d duels and %d pods, want one of each",
			board.Duels, board.Pods)
	}
	if board.Duels+board.Pods != board.Matches {
		t.Errorf("%d duels + %d pods != %d matches",
			board.Duels, board.Pods, board.Matches)
	}
}

// The top ten, largest first — and it holds ten however many are recorded.
func TestTheBiggestKillingBlowsRankFirst(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	a := deckNamed(t, "cats", "Arahbo, Roar of the World", "cats")
	b := deckNamed(t, "dinos", "Atla Palani, Nest Tender", "dinosaurs")

	// Twelve games, each with a blow of a different size, recorded as one
	// match per game so every blow has its own row.
	for _, n := range []int{7, 41, 3, 19, 60, 12, 5, 33, 28, 2, 51, 16} {
		games := splitGames(1, 1)
		games[0].Killer = &tier3.KillingBlow{Amount: n, Card: "Blow " + strconv.Itoa(n),
			Sources: 1, Combat: true, Seat: 2, Turn: 11}
		rec.Record(t.Context(), matchOf(big.NewInt(int64(n)), games,
			[]*deck.Deck{a, b}))
	}
	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}
	if len(board.Blows) != TopBlows {
		t.Fatalf("the leaderboard holds %d blows, want %d", len(board.Blows), TopBlows)
	}
	want := []int{60, 51, 41, 33, 28, 19, 16, 12, 7, 5}
	for i, k := range board.Blows {
		if k.Amount != want[i] {
			t.Errorf("row %d is %d, want %d", i, k.Amount, want[i])
		}
	}
	top := board.Blows[0]
	if top.Card != "Blow 60" || top.Sources != 1 || !top.Combat || top.Turn != 11 {
		t.Errorf("the top blow reads %+v", top)
	}
	// The seat that died is resolved to a deck through the match's roster,
	// which is the whole reason that join exists.
	if top.Victim != "dinos" {
		t.Errorf("the top blow killed %q, want dinos", top.Victim)
	}
	if top.Seats != 2 {
		t.Errorf("the top blow was at a table of %d, want 2", top.Seats)
	}
	// A game with no blow contributes no row: the two below the cut are 3
	// and 2, and nothing null-ish is on the board.
	for _, k := range board.Blows {
		if k.Amount <= 0 || k.Card == "" {
			t.Errorf("a blank blow reached the board: %+v", k)
		}
	}
}

// The two feat boards Aaron asked for on 2026-09-06, each largest-first and
// each resolving its seat to a deck through the match's own roster.
func TestTheFeatBoardsRankLargestFirst(t *testing.T) {
	t.Parallel()
	rec, _ := scratch(t)
	a := deckNamed(t, "cats", "Arahbo, Roar of the World", "cats")
	b := deckNamed(t, "dinos", "Atla Palani, Nest Tender", "dinosaurs")

	for _, n := range []int{4, 19, 7, 40, 12} {
		games := splitGames(1, 1)
		games[0].Biggest = &tier3.BigCreature{Card: "Giant " + strconv.Itoa(n),
			Power: n, Toughness: n + 1, Seat: 1, Turn: 9}
		games[0].TallestStack = &tier3.TokenStack{Card: "Cat Token",
			Count: n * 2, Seat: 2, Turn: 9}
		rec.Record(t.Context(), matchOf(big.NewInt(int64(n)), games,
			[]*deck.Deck{a, b}))
	}
	board, err := rec.Board(t.Context(), open)
	if err != nil {
		t.Fatal(err)
	}

	wantPower := []int{40, 19, 12, 7, 4}
	if len(board.Giants) != len(wantPower) {
		t.Fatalf("the giants board holds %d, want %d", len(board.Giants), len(wantPower))
	}
	for i, g := range board.Giants {
		if g.Power != wantPower[i] {
			t.Errorf("giant %d is %d power, want %d", i, g.Power, wantPower[i])
		}
	}
	top := board.Giants[0]
	if top.Card != "Giant 40" || top.Toughness != 41 || top.Turn != 9 {
		t.Errorf("the top giant reads %+v", top)
	}
	// Seat 1 is `cats`, resolved through the roster rather than guessed.
	if top.Deck != "cats" {
		t.Errorf("the top giant stood for %q, want cats", top.Deck)
	}

	wantCount := []int{80, 38, 24, 14, 8}
	if len(board.Stacks) != len(wantCount) {
		t.Fatalf("the stacks board holds %d, want %d", len(board.Stacks), len(wantCount))
	}
	for i, s := range board.Stacks {
		if s.Count != wantCount[i] {
			t.Errorf("stack %d is %d, want %d", i, s.Count, wantCount[i])
		}
	}
	// Seat 2 is `dinos` — a different seat from the giants, so a board that
	// joined on the wrong column would show cats here.
	if board.Stacks[0].Deck != "dinos" || board.Stacks[0].Card != "Cat Token" {
		t.Errorf("the top stack reads %+v", board.Stacks[0])
	}
}
