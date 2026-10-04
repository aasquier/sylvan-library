package night

import (
	"math/rand"
	"testing"
	"time"
)

// Two pieces of the night that only this package can reach, and both are
// arguments its own comments make and nothing checked.
//
// The file is `package night` rather than `night_test` for exactly that
// reason: the pairing's queue and the store's timestamp round trip are
// unexported, and the alternative to calling them is to assert their
// behaviour through a public surface that averages it away.

// The house never rides the player queue. `playerTurns` is what decides how
// many bouts each *account* gets, and a house deck has no account -- so a
// seat with no owner has to fall out of the queue rather than be dealt turns
// as though somebody owned it. Dealing the house turns would spend the
// per-account share on a deck that already plays every night.
func TestTheHouseNeverRidesThePlayerQueue(t *testing.T) {
	t.Parallel()
	ada, bruno := int64(1), int64(2)
	seats := []Seat{
		{Slug: "goreclaw"}, // the house: no owner
		{Owner: &ada, Slug: "gyome"},
		{Slug: "atla"}, // the house again
		{Owner: &bruno, Slug: "tivit"},
	}

	// Seeded, like the pairing's own generator: the queue is a function of the
	// roster and the seed and nothing else.
	rng := rand.New(rand.NewSource(7)) //nolint:gosec // seeded on purpose
	queue := playerTurns(rng, seats, 2)

	if len(queue) != 4 {
		t.Fatalf("two accounts at two turns each dealt %d seats", len(queue))
	}
	owners := map[int64]int{}
	for _, s := range queue {
		if s.Owner == nil {
			t.Fatalf("the house is in the player queue: %+v", queue)
		}
		owners[*s.Owner]++
	}
	if owners[ada] != 2 || owners[bruno] != 2 {
		t.Errorf("the turns came out %v, want two each", owners)
	}
}

// An instant the recorded format cannot express comes back as itself.
//
// The format carries a four-digit year, so a year outside it renders to
// something RFC3339 will not read back -- and the answer a reader needs then
// is the instant it was handed, never the zero time. This is the arm the
// function's comment used to call unreachable; it is one `time.Date` away,
// and the zero time would be a silently wrong stamp on a row.
func TestAnInstantTheFormatCannotHoldComesBackAsItself(t *testing.T) {
	t.Parallel()

	ordinary := time.Date(2026, 9, 6, 22, 5, 0, 0, time.UTC)
	if got := restamp(ordinary); !got.Equal(ordinary) {
		t.Errorf("an ordinary instant restamped to %v", got)
	}

	for _, beyond := range []time.Time{
		time.Date(99999, 1, 2, 3, 4, 5, 0, time.UTC),
		time.Date(-5000, 1, 2, 3, 4, 5, 0, time.UTC),
	} {
		got := restamp(beyond)
		if got.IsZero() {
			t.Errorf("%v restamped to the zero time", beyond)
		}
		if !got.Equal(beyond) {
			t.Errorf("%v restamped to %v, want itself", beyond, got)
		}
	}
}
