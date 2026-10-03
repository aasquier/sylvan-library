package ledger

import (
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/floats"
)

// Two folds the boards are built out of, asked the questions a database cannot
// currently put to them.
//
// `standings_test.go` drives both through a real ledger, which is the right way
// round and reaches only the rows today's SQL can produce. These two ask what
// the fold does with a row shape the *reader* has to survive: a duel the read
// saw one side of, and two decks whose intervals are identical. The first is a
// guard against an index out of range — the one failure in this file that would
// take the whole board down rather than mis-sort it — and the second is the
// tie-break the board's ordering is defined by and which equal bounds are the
// only way to reach.

// aSeat is one row of the board read, with its games already tallied.
func aSeat(matchID int64, seat int, slug string, seats int) seatRow {
	return seatRow{matchID: matchID, seat: seat, slug: slug, seats: seats,
		archetype: "midrange", createdAt: "2026-10-03T00:00:00Z"}
}

// **A duel the read saw half of is skipped, not paired with itself.** The seat
// count is the size of the table the match was played at, and the rows are what
// the read returned; a reader that trusted the first and indexed the second
// would panic on the whole board rather than drop one line of it.
func TestAHalfReadDuelIsNoMeeting(t *testing.T) {
	t.Parallel()
	rows := []seatRow{
		// A duel with one row present and two seats claimed.
		aSeat(1, 1, "alpha", 2),
		// A pod, which is never a meeting however many rows it has.
		aSeat(2, 1, "alpha", 4),
		aSeat(2, 2, "beta", 4),
		// And a whole duel, so a pass cannot be "it found nothing at all".
		aSeat(3, 1, "alpha", 2),
		aSeat(3, 2, "beta", 2),
	}
	met := meetings(rows)
	if len(met) != 1 {
		t.Fatalf("the fold found %d meetings, want the one whole duel: %+v",
			len(met), met)
	}
	if met[0].A.Slug != "alpha" || met[0].B.Slug != "beta" {
		t.Errorf("the meeting is %s against %s", met[0].A.Slug, met[0].B.Slug)
	}
	if met[0].Matches != 1 {
		t.Errorf("the meeting counts %d matches, want 1", met[0].Matches)
	}
}

// **The count breaks a tie the bounds cannot.** Two decks with identical
// intervals are separated by which has shown more, and the name settles the last
// one so the board does not shuffle between two reads of the same data. Equal
// bounds are the ordinary case rather than a contrived one: every deck with no
// wins yet has a lower bound of zero.
func TestEqualBoundsAreSortedByWhatHasBeenShown(t *testing.T) {
	t.Parallel()
	rows := []ClassRecord{
		{Archetype: "aggro", Record: Record{Played: 1, Lower: floats.Float(0)}},
		{Archetype: "control", Record: Record{Played: 9, Lower: floats.Float(0)}},
		{Archetype: "combo", Record: Record{Played: 9, Lower: floats.Float(0)}},
		{Archetype: "ramp", Record: Record{Played: 2, Lower: floats.Float(0.4)}},
	}
	sortBoard(rows, func(c ClassRecord) (Record, string) { return c.Record, c.Archetype })

	got := []string{}
	for _, r := range rows {
		got = append(got, r.Archetype)
	}
	want := []string{"ramp", "combo", "control", "aggro"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the board reads %v, want %v — the bound first, then what "+
				"has been shown, then the name", got, want)
		}
	}
}
