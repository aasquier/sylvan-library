package night

import (
	"hash/fnv"
	"math/rand"
	"sort"
	"strconv"
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
// (ADR 46 decision 5), each account in at most `set.BoutsPerAccount` bouts,
// at most `set.Bouts` bouts in all, and the house filling every remaining
// seat and any leftover capacity — house against house is a fine bout, it
// densifies the record.
//
// The turn order is "one deck per account in turn": account order and each
// account's deck order are seeded shuffles, and an account with fewer decks
// than turns plays a deck again rather than sitting out — the cap is a share
// of the night, and a small shelf densifies what its owner is tuning. Two
// decks of one account are never seated against each other on a scheduled
// night; when no other account's deck is waiting, the opponent is the
// house's, and a deck with no opponent at all sits out unplanned. (A served
// instance always has a house — the showcase — so that last case is a test's,
// not a night's.)
func PlanScheduled(nightKey string, house []string, players []Seat,
	set Settings) []Plan {
	rng := rand.New(rand.NewSource(derive("deal", nightKey))) //nolint:gosec // seeded on purpose: the deal must replay
	queue := playerTurns(rng, players, set.BoutsPerAccount)
	nextHouse := houseCycle(rng, house)

	plans := []Plan{}
	for len(plans) < set.Bouts {
		var a, b Seat
		if len(queue) > 0 {
			a, queue = queue[0], queue[1:]
			if i := opponentFor(a, queue); i >= 0 {
				b = queue[i]
				queue = append(queue[:i:i], queue[i+1:]...)
			} else if h, ok := nextHouse(); ok {
				b = h
			} else {
				continue // nobody to fight: the deck sits out
			}
		} else {
			// The house fills the leftover capacity — two different decks,
			// or the card is as full as it can honestly be.
			h1, ok1 := nextHouse()
			h2, ok2 := nextHouse()
			if !ok1 || !ok2 || h1.Slug == h2.Slug {
				break
			}
			a, b = h1, h2
		}
		plans = append(plans, plan(nightKey, a, b, set.Games, len(plans)))
	}
	return plans
}

// PlanSample deals a measurement run: a full round-robin over the entire
// roster — house and players together, every pair once, caps ignored — so
// the sample saturates its window and the count of bouts a window holds is
// measured rather than guessed. The deadline is the only bound; whatever the
// window does not reach is skipped when it closes, and that skip count is
// itself part of the measurement.
func PlanSample(nightKey string, house []string, players []Seat, games int) []Plan {
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

	plans := make([]Plan, 0, len(pairs))
	for _, p := range pairs {
		plans = append(plans, plan(nightKey, p.a, p.b, games, len(plans)))
	}
	return plans
}

// playerTurns is the round-robin queue: `turns` passes over the accounts in
// seeded order, one deck per account per pass. Both shuffles draw from the
// caller's generator over sorted inputs, so the queue is a function of the
// night key and the roster and nothing else.
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
	sort.Slice(owners, func(i, j int) bool { return owners[i] < owners[j] })
	rng.Shuffle(len(owners), func(i, j int) {
		owners[i], owners[j] = owners[j], owners[i]
	})
	for _, o := range owners {
		decks := byOwner[o]
		sort.Slice(decks, func(i, j int) bool { return decks[i].Slug < decks[j].Slug })
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

// opponentFor finds the first queued deck from a different account, or -1.
func opponentFor(a Seat, queue []Seat) int {
	for i, cand := range queue {
		if *cand.Owner != *a.Owner {
			return i
		}
	}
	return -1
}

// houseCycle deals the house's decks in seeded order, wrapping when the card
// outlasts the shelf. ok is false when there is no house at all.
func houseCycle(rng *rand.Rand, house []string) func() (Seat, bool) {
	shuffled := append([]string(nil), house...)
	sort.Strings(shuffled)
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

// plan is one bout with its stable seed: night key, both seats, and the
// bout's place on the card, hashed — so replanning the same night deals the
// same seed to the same fight, and no two bouts of a night share one.
func plan(nightKey string, a, b Seat, games, index int) Plan {
	return Plan{SeatA: a, SeatB: b, Games: games,
		Seed: derive("bout", nightKey, seatKey(a), a.Slug, seatKey(b), b.Slug,
			strconv.Itoa(index))}
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
	return int64(h.Sum64() &^ (1 << 63))
}
