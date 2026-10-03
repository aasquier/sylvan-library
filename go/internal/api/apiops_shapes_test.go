package api

import (
	"math/big"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/night"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The edges of the shapers — the branches every ordinary match walks past.
//
// Each of these is a pure function of its arguments, and each one's uncovered
// arm is an **absence**: a step that reported no changes, a card whose faces
// the record cannot describe, a seat with no owner, a run with no games, a
// board that was never narrated. The ordinary fixture carries none of those,
// which is exactly why they sat unreached — a two-deck match of three finished
// games has a value in every field.
//
// They are called directly rather than through a route because that is where
// they live: `shapeForge` and `newForgeBoard` are what a route hands its
// answer to, and driving a whole match to reach `if out.Rows == nil` would be
// proving the fixture rather than the function.

// ---- the row and the reel --------------------------------------------------

// **A killing blow names the chair that died, by slug.** The victim's name is
// resolved through the `seatSlug` closure the caller hands in, and that closure
// is only ever called for a game that ended on a blow — so until something
// drove a row *with* a killer, the one line that turns a seat number into a
// deck's name had never run.
func TestARowWithAKillingBlowNamesTheSeatThatDied(t *testing.T) {
	t.Parallel()
	turns := 9
	seat := 2
	label := "Ai(1)-x"
	row := newForgeRow(tier3.GameResult{
		Index: 1, Milliseconds: 5400, Turns: &turns,
		Winner: &label, WinnerSeat: &seat,
		Killer: &tier3.KillingBlow{Amount: 21, Card: "Craterhoof Behemoth",
			Combat: true, Seat: 2, Turn: 9, Sources: 6},
	}, nil, func(s int) string {
		return map[int]string{1: "gyome", 2: "mono-green"}[s]
	})
	if row.Killer == nil {
		t.Fatal("a game that ended on a blow crossed with no blow on it")
	}
	if row.Killer.Victim != "mono-green" {
		t.Errorf("the blow landed on %q, want mono-green -- the seat number "+
			"was not resolved through the caller's own naming", row.Killer.Victim)
	}
	if row.Killer.Amount != 21 || row.Killer.Sources != 6 || !row.Killer.Combat {
		t.Errorf("the blow crossed as %+v", row.Killer)
	}
}

// A match whose games are all draws has no rows to rank, and the shaper must
// still answer a **list** rather than a null: a room that ranged over `rows`
// would otherwise have nothing to range over and a client decoding `null` into
// an array has a different bug on every platform.
func TestAMatchThatPlayedNoGamesStillCarriesAnEmptyRowList(t *testing.T) {
	t.Parallel()
	gyome := &deck.Deck{Slug: "gyome", Name: "Gyome"}
	mono := &deck.Deck{Slug: "mono-green", Name: "Mono Green"}
	out := shapeForge([]*deck.Deck{gyome, mono},
		[]string{"alice/gyome", "alice/mono-green"}, 3,
		tier3.ClockForSeats(2), big.NewInt(7),
		tier3.RunFromWire(tier3.WireRun{Seats: map[int]string{1: "gyome"}}), nil)
	if out.Rows == nil {
		t.Error("a match with no games shaped its rows as a null")
	}
	if out.Played != 0 {
		t.Errorf("played %d games out of a run with none", out.Played)
	}
	// And nothing invented a median out of no measurements.
	if out.MedianSeconds != nil || out.MaxSeconds != nil {
		t.Errorf("a match with no games reported a median %v and a max %v",
			out.MedianSeconds, out.MaxSeconds)
	}
}

// **The replay is capped and the cap is the shaper's.** A narrated match of
// many games hands up a beat reel per game, and the room replays the first
// [ForgeReplayGames] of them; a payload carrying all of them would be the whole
// match's narration in one response, which is the shape this cap exists to
// refuse.
func TestTheShapedReplayIsCutToTheGamesTheRoomWillReplay(t *testing.T) {
	t.Parallel()
	gyome := &deck.Deck{Slug: "gyome", Name: "Gyome"}
	beats := make([]forgeBeats, 0, ForgeReplayGames+3)
	for i := 0; i < ForgeReplayGames+3; i++ {
		beats = append(beats, forgeBeats{Game: i + 1})
	}
	out := shapeForge([]*deck.Deck{gyome}, []string{"alice/gyome"},
		len(beats), tier3.ClockForSeats(1), big.NewInt(1),
		tier3.RunFromWire(tier3.WireRun{Seats: map[int]string{1: "gyome"}}), beats)
	if len(out.Beats) != ForgeReplayGames {
		t.Fatalf("%d games of narration crossed, want the %d the room replays",
			len(out.Beats), ForgeReplayGames)
	}
	// The first ones, not the last: a replay starts at game one.
	if out.Beats[0].Game != 1 {
		t.Errorf("the replay starts at game %d", out.Beats[0].Game)
	}
}

// A board nobody narrated carries an **empty** reel of steps rather than a
// null, for the row list's reason: the room steps through them.
func TestABoardWithNoStepsCarriesAnEmptyReelRatherThanANull(t *testing.T) {
	t.Parallel()
	board := newForgeBoard(aReel(0), map[int]string{1: "gyome"}, nil, nil, 0)
	if board == nil {
		t.Fatal("a reel with seats and no steps shaped to nothing")
	}
	if board.Steps == nil {
		t.Error("a board with no steps shaped them as a null")
	}
	if len(board.Steps) != 0 {
		t.Errorf("%d steps came out of a reel with none", len(board.Steps))
	}
}

// ---- the granted-keyword subtraction ---------------------------------------

// A step that reported no changes at all is **skipped rather than rebuilt**,
// and a change that reported no live keywords is left exactly as it arrived.
//
// Both arms are about a mixed reel, which is the deployed shape: the scribe
// sends a change set only for the cards that moved, so most steps in a long
// game carry nothing and most changes in a busy step are not about keywords.
// A copy pass that marked those would publish an empty `granted` on a card
// nobody asked about — a badge saying "nothing grants this" where there is no
// badge to draw.
func TestOnlyTheChangesThatReportedKeywordsAreMarked(t *testing.T) {
	t.Parallel()
	steps := []tier3.BoardStep{
		// A step with nothing in it: the commonest step in a long game.
		{Turn: 1, Seat: 1},
		{Turn: 2, Seat: 1, Changes: []tier3.BoardChange{
			// A change about something other than keywords -- life, counters,
			// a tap -- and no live set beside it.
			{ID: 7},
			liveChange(120, "Vigilance"),
		}},
	}
	got := grantKeywords(steps, map[int][]string{120: {}, 7: {"Flying"}})
	if len(got) != 2 {
		t.Fatalf("%d steps came back, want 2", len(got))
	}
	if got[0].Changes != nil {
		t.Errorf("a step with no changes grew some: %+v", got[0].Changes)
	}
	if got[1].Changes[0].Granted != nil {
		t.Error("a change that reported no live keywords was marked anyway, " +
			"which draws a badge on a card nobody said anything about")
	}
	if got[1].Changes[1].Granted == nil || len(*got[1].Changes[1].Granted) != 1 {
		t.Errorf("the change that did report keywords was not marked: %+v",
			got[1].Changes[1])
	}
}

// A blank in the printing's keyword list matches nothing.
//
// Scryfall's `keywords` is a list of words, and an empty string in it is what a
// half-written row looks like. Compared loosely it would match every live
// keyword at once and every granted badge on the board would vanish, which is
// the failure that looks like the feature working.
func TestABlankInThePrintingsKeywordsMatchesNothing(t *testing.T) {
	t.Parallel()
	if accountedFor("Vigilance", []string{"", "  "}) {
		t.Error("a blank in the printing's keywords accounted for vigilance")
	}
	if !accountedFor("Ward:2", []string{"", "Ward"}) {
		t.Error("a real keyword past a blank was not found, so the blank " +
			"stopped the search instead of being skipped")
	}
}

// A record whose face list does not line up with its own names is painted
// **not at all**, rather than painted wrong.
//
// A pool too old to carry per-face images answers a two-named card with no
// faces, and a room that zipped the two lists would hang the front picture on
// the back half — Wizards' painting under somebody else's name, which is
// commandment 19's territory and not a cosmetic slip.
func TestACardWhoseFacesDoNotLineUpIsNotPaintedAtAll(t *testing.T) {
	t.Parallel()
	front := "https://cards.example/front.jpg"
	rec := &pool.CardRecord{
		Name:   "Etali, Primal Conqueror // Etali, Primal Sickness",
		Layout: "transform",
		Faces:  []pool.CardFace{{Name: "Etali, Primal Conqueror", ImageNormal: &front}},
	}
	if got := facePicturesOf(rec, []string{
		"Etali, Primal Conqueror", "Etali, Primal Sickness"}); got != nil {
		t.Errorf("a record with %d faces and 2 names was painted %v",
			len(rec.Faces), got)
	}
}

// ---- the small readings ----------------------------------------------------

// A card whose row carries no art crop has one **derived** from its full
// picture, so a camera review still shows a crop rather than a blank tile.
func TestACardWithNoCropOnItsRowStillGetsOne(t *testing.T) {
	t.Parallel()
	normal := "https://cards.scryfall.io/normal/front/a/b/abc.jpg"
	got := identified(&pool.CardRecord{Name: "Sol Ring",
		TypeLine: "Artifact", ImageNormal: &normal})
	if got.ArtCrop == nil {
		t.Fatal("a card with a picture and no crop was given no crop")
	}
	if *got.ArtCrop == normal {
		t.Errorf("the crop is the whole picture: %q", *got.ArtCrop)
	}
	if want := pool.ArtCropFrom(&normal); want == nil || *got.ArtCrop != *want {
		t.Errorf("the crop is %q, want the pool's own derivation", *got.ArtCrop)
	}
}

// Only blanks were written, so the box says **nothing** rather than lighting
// green over no commander at all.
func TestAVerdictOverNoSeatsIsBlankRatherThanReady(t *testing.T) {
	t.Parallel()
	state, sentence := commanderVerdict(nil, nil)
	if state != "blank" {
		t.Errorf("a verdict over no seats reads %q, want blank -- anything "+
			"else lights the box over nothing", state)
	}
	if sentence != "" {
		t.Errorf("a blank verdict said %q", sentence)
	}
}

// A seat the house holds has no owner, and the wire says so with an **absent**
// owner rather than with a zero — which is a real account id.
func TestANightSeatCrossesItsOwnerWhenItHasOne(t *testing.T) {
	t.Parallel()
	bobID := int64(2)
	owned := nightSeat(night.Seat{Owner: &bobID, Slug: "bobs-public"})
	house := nightSeat(night.Seat{Slug: "kaheera"})
	value := func(m wire.OrderedMap, key string) any {
		for _, kv := range m {
			if kv.Key == key {
				return kv.Value
			}
		}
		t.Fatalf("no %q in %+v", key, m)
		return nil
	}
	if got := value(owned, "owner"); got != int64(2) {
		t.Errorf("an owned seat's owner crossed as %#v, want 2", got)
	}
	if got := value(house, "owner"); got != nil {
		t.Errorf("a house seat's owner crossed as %#v, want nothing", got)
	}
}
