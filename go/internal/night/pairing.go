package night

import (
	"hash/fnv"
	"math/rand"
	"slices"
	"strconv"
	"strings"
)

// The deal: who fights whom tonight, decided once at run open and written as
// rows. Everything here is a pure function of the night key and the roster —
// no wall clock, no unseeded randomness — so the same night dealt twice is
// the same card of bouts, which is what lets a night be reproduced in
// principle and lets a test assert the deal rather than eyeball it. The
// shuffle runs on a generator seeded from the night key ([derive]); a plain
// hash rather than the mt19937 kernel, because nothing recorded rests on
// these bits — the promise is "stable", not "bit-compatible with a golden".

// PlanScheduled deals one scheduled night: round-robin across accounts
// (ADR 46 decision 5), each account owed at most `set.BoutsPerAccount` turns,
// at most `set.Bouts` bouts in all, and the house filling every remaining
// chair — house against house is a fine bout, it densifies the record.
//
// **A bout is a table rather than a pair since 2026-09-06**, and [tableSize]
// decides how many chairs each one has: two pods to every duel. So the unit
// the caps count is a *seat at a table*, not an opponent.
//
// The turn order is "one deck per account in turn": account order and each
// account's deck order are seeded shuffles, and an account with fewer decks
// than turns plays a deck again rather than sitting out — the cap is a share
// of the night, and a small shelf densifies what its owner is tuning. No
// account holds two chairs at one table, and no deck appears at one table
// twice; [seatTable] keeps both. A table the shelf cannot fill honestly ends
// the deal rather than seating a mirror. (A served instance always has a
// house — the showcase — so a short card is a test's case, not a night's.)
func PlanScheduled(nightKey string, house []string, players []Seat,
	set Settings) []Plan {
	rng := rand.New(rand.NewSource(derive("deal", nightKey))) //nolint:gosec // seeded on purpose: the deal must replay
	queue := playerTurns(rng, players, set.BoutsPerAccount)
	nextHouse := houseCycle(rng, house)

	plans := []Plan{}
	for len(plans) < set.Bouts {
		table, ok := seatTable(tableSize(len(plans)), &queue, nextHouse)
		if !ok {
			break // the shelf cannot fill another table honestly
		}
		plans = append(plans, plan(nightKey, table, set, len(plans)))
	}
	return plans
}

// tableSize decides whether bout `index` is a pod or a duel, and it is
// arithmetic rather than a coin so that a night's shape is the same every
// night and a test can assert it rather than sample it.
//
// Two in three are pods (Aaron, 2026-09-06), and the duel is the *last* of
// each group of three rather than the first: a night that is cut short by
// its window — which is the ordinary case, since the deal oversaturates —
// then loses whole groups rather than losing all of one kind. Interleaving
// is the whole point; a night of pods followed by a night's worth of duels
// would satisfy the ratio and still be the wrong night.
func tableSize(index int) int {
	if (index+1)%PodShareDenominator == 0 {
		return 2
	}
	return PodSeats
}

// seatTable draws one table of `size` chairs: players first, in queue order
// and never two decks of one account at the same table, then the house fills
// whatever is left. It reports false when the table cannot be filled — which
// on a served instance means the house shelf ran out of distinct decks, since
// a served instance always has a showcase.
//
// The queue is a pointer because a drawn deck leaves it: a scheduled night
// spends each account's turns, and a deck that sat down must not sit down
// again at the same table or later in the same night on somebody else's turn.
func seatTable(size int, queue *[]Seat, nextHouse func() (Seat, bool)) ([]Seat, bool) {
	table := make([]Seat, 0, size)
	// The players who are owed a turn, skipping any whose account is already
	// seated here. A skipped deck keeps its place in the queue for the next
	// table rather than being spent.
	for i := 0; i < len(*queue) && len(table) < size; {
		cand := (*queue)[i]
		if seatedOwner(table, cand) {
			i++
			continue
		}
		table = append(table, cand)
		*queue = append((*queue)[:i:i], (*queue)[i+1:]...)
	}
	// The house fills the rest, and never twice at one table: two copies of
	// one deck is a mirror nobody asked for and a seat map with a duplicate
	// slug in it.
	for len(table) < size {
		h, ok := nextHouse()
		if !ok {
			return nil, false
		}
		if seatedSlug(table, h.Slug) {
			// The cycle has wrapped onto a deck already at this table, so the
			// shelf is smaller than the table. Take the honest smaller table
			// rather than spinning: two is still a bout.
			break
		}
		table = append(table, h)
	}
	if len(table) < 2 {
		return nil, false
	}
	return table, true
}

// seatedOwner reports whether this seat's account already holds a chair.
// The house never collides here — it has no account — and is handled by
// [seatedSlug] instead.
func seatedOwner(table []Seat, cand Seat) bool {
	if cand.Owner == nil {
		return false
	}
	for _, s := range table {
		if s.Owner != nil && *s.Owner == *cand.Owner {
			return true
		}
	}
	return false
}

func seatedSlug(table []Seat, slug string) bool {
	for _, s := range table {
		if s.Slug == slug {
			return true
		}
	}
	return false
}

// GamesFor and ClockFor are the two dials a table's size chooses, and they
// are functions rather than fields so that one seating cannot be given a
// pod's clock and a duel's game count.
func GamesFor(seats int, set Settings) int {
	if seats > 2 {
		return PodGames
	}
	return set.Games
}

// ClockFor is Forge's per-game seconds for a table of this size. A pod at a
// duel's clock is cut about one game in six and recorded as a draw; see
// [PodClock].
func ClockFor(seats int) int {
	if seats > 2 {
		return PodClock
	}
	return DuelClock
}

// PlanSample deals a measurement run: a full round-robin over the entire
// roster — house and players together, every pair once, caps ignored — so
// the sample saturates its window and the count of bouts a window holds is
// measured rather than guessed. The deadline is the only bound; whatever the
// window does not reach is skipped when it closes, and that skip count is
// itself part of the measurement.
//
// **It deals the same mix a scheduled night would**, two pods to every duel,
// because that is the whole point of a sample: the count it measures has to
// be the count the window will actually hold. A sample of pure duels would
// over-report a mixed night by an order of magnitude — a duel game runs ~16s
// against a pod's ~135s median — which is exactly the error the first sample
// hour made on 2026-09-06, before pods existed to be dealt.
func PlanSample(nightKey string, house []string, players []Seat,
	set Settings) []Plan {
	seats := make([]Seat, 0, len(house)+len(players))
	for _, slug := range house {
		seats = append(seats, Seat{Slug: slug})
	}
	seats = append(seats, players...)

	type pair struct{ a, b Seat }
	pairs := make([]pair, 0, len(seats)*(len(seats)-1)/2)
	for i := range seats {
		for j := i + 1; j < len(seats); j++ {
			pairs = append(pairs, pair{seats[i], seats[j]})
		}
	}
	rng := rand.New(rand.NewSource(derive("sample", nightKey))) //nolint:gosec // seeded on purpose: the deal must replay
	rng.Shuffle(len(pairs), func(i, j int) { pairs[i], pairs[j] = pairs[j], pairs[i] })

	// The round-robin decides *who*; the mix decides *how many chairs*. A pod
	// takes its first two seats from the pair and fills the rest from the
	// shuffled roster, skipping anyone already at the table, so every pair
	// still meets exactly once and the pod's other two are a deal rather than
	// a repeat of the pair before it.
	fill := 0
	plans := make([]Plan, 0, len(pairs))
	for _, p := range pairs {
		table := []Seat{p.a, p.b}
		for len(table) < tableSize(len(plans)) {
			cand := seats[fill%len(seats)]
			fill++
			if !seatedSlug(table, cand.Slug) && !seatedOwner(table, cand) {
				table = append(table, cand)
			}
			if fill > len(seats)*2 {
				break // the roster is too small to fill a pod; the pair plays
			}
		}
		plans = append(plans, plan(nightKey, table, set, len(plans)))
	}
	return plans
}

// playerTurns is the round-robin queue: `turns` passes over the accounts in
// seeded order, one deck per account per pass. Both shuffles draw from the
// caller's generator over sorted inputs, so the queue is a function of the
// night key and the roster and nothing else. The sorts are total — owner ids
// are unique by construction and one owner cannot hold two decks of one slug
// — so an unstable sort cannot move a tie and no Stable form is owed.
func playerTurns(rng *rand.Rand, players []Seat, turns int) []Seat {
	byOwner := map[int64][]Seat{}
	owners := []int64{}
	for _, p := range players {
		if p.Owner == nil {
			continue // the house never rides the player queue
		}
		if _, seen := byOwner[*p.Owner]; !seen {
			owners = append(owners, *p.Owner)
		}
		byOwner[*p.Owner] = append(byOwner[*p.Owner], p)
	}
	slices.Sort(owners)
	rng.Shuffle(len(owners), func(i, j int) {
		owners[i], owners[j] = owners[j], owners[i]
	})
	for _, o := range owners {
		decks := byOwner[o]
		slices.SortFunc(decks, func(a, b Seat) int { return strings.Compare(a.Slug, b.Slug) })
		rng.Shuffle(len(decks), func(i, j int) {
			decks[i], decks[j] = decks[j], decks[i]
		})
	}
	queue := []Seat{}
	for turn := 0; turn < turns; turn++ {
		for _, o := range owners {
			decks := byOwner[o]
			queue = append(queue, decks[turn%len(decks)])
		}
	}
	return queue
}

// houseCycle deals the house's decks in seeded order, wrapping when the card
// outlasts the shelf. ok is false when there is no house at all.
func houseCycle(rng *rand.Rand, house []string) func() (Seat, bool) {
	shuffled := append([]string(nil), house...)
	slices.Sort(shuffled)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	next := 0
	return func() (Seat, bool) {
		if len(shuffled) == 0 {
			return Seat{}, false
		}
		s := Seat{Slug: shuffled[next%len(shuffled)]}
		next++
		return s, true
	}
}

// plan is one bout with its stable seed: night key, every seat in order, and
// the bout's place on the card, hashed — so replanning the same night deals
// the same seed to the same fight, and no two bouts of a night share one.
//
// The seats go into the hash in seat order, which means a pod dealt the same
// four decks in a different order is a different seed. That is correct rather
// than incidental: seat order is turn order, and turn order is most of what a
// four-player game is.
func plan(nightKey string, seats []Seat, set Settings, index int) Plan {
	parts := make([]string, 0, 2+len(seats)*2)
	parts = append(parts, "bout", nightKey)
	for _, s := range seats {
		parts = append(parts, seatKey(s), s.Slug)
	}
	parts = append(parts, strconv.Itoa(index))
	return Plan{Seats: seats, Games: GamesFor(len(seats), set),
		Clock: ClockFor(len(seats)), Seed: derive(parts...)}
}

// seatKey names a seat's owner for the seed derivation: the account id, or
// "house" — an owner id can never collide with it.
func seatKey(s Seat) string {
	if s.Owner == nil {
		return "house"
	}
	return strconv.FormatInt(*s.Owner, 10)
}

// derive hashes its parts into a non-negative int64 — FNV-1a with a NUL
// between parts so ("ab","c") and ("a","bc") cannot collide. Non-negative
// because the seed rides Forge's command line and the wire as text, and a
// minus sign is one more thing for somebody to quote wrong.
func derive(parts ...string) int64 {
	h := fnv.New64a()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return int64(h.Sum64() &^ (1 << 63)) //nolint:gosec // the top bit is cleared, so the conversion cannot overflow
}
