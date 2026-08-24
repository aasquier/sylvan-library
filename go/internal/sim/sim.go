// Package sim holds the vocabulary the simulator's tiers share: a compiled
// card, the mana it makes, and the handful of exact float behaviours the
// recorded numbers rest on.
//
// It is deliberately small and deliberately *below* the tiers. The compiled
// card could plausibly live in Tier 1 -- the goldfish engine is its heaviest
// consumer, and both closed forms (`sim/karsten`, `sim/curve`) read it too
// -- but then two closed forms would import the engine to reach a type, and
// the dependency would run the wrong way round: Tier 1.5
// is arithmetic over a compiled deck and has nothing to say about shuffling.
// So the type sits one package lower, `sim/compile` builds it, and every
// tier looks down.
//
// Nothing in this package samples, seeds, or consults a model.
package sim

// Source is something that produces mana.
//
// Colors is which colours it can make -- {"C"} for genuinely colourless, all
// five for an any-colour source. Amount is how many it makes per activation,
// which is the field Scryfall's `produced_mana` does not carry and
// `sim/compile` reads off the oracle text instead (Sol Ring makes two).
type Source struct {
	Colors []string `json:"colors"`
	Amount int      `json:"amount"`
}

// Makes reports whether this source can produce any colour in `want` -- a
// non-empty intersection of the two colour sets.
func (s Source) Makes(want map[string]bool) bool {
	for _, c := range s.Colors {
		if want[c] {
			return true
		}
	}
	return false
}

// Cost is a parsed mana cost as the simulator carries it: generic, one
// colour set
// per coloured pip, and the Phyrexian symbols kept apart.
//
// Phyrexian is a count that matters and a demand that does not: two life always
// pays it, so it places nothing on a mana base while still adding one to mana
// value. `mana.Cost` is the parser's own type; this is the same shape carried
// through a compiled deck, and the two agree because `sim/compile` builds
// one from the other. Keeping it parsed rather than printed is what makes a
// disagreement about parsing a `mana` failure instead of an arithmetic one.
type Cost struct {
	Generic   int        `json:"generic"`
	Pips      [][]string `json:"pips"`
	Phyrexian [][]string `json:"phyrexian"`
	HasX      bool       `json:"has_x"`
}

// ManaValue is generic plus one per pip plus one per
// Phyrexian symbol. X counts as 0.
func (c Cost) ManaValue() int { return c.Generic + len(c.Pips) + len(c.Phyrexian) }

// Card is a card reduced to what the simulator
// can reason about.
type Card struct {
	Name         string `json:"name"`
	Cost         Cost   `json:"cost"`
	Category     string `json:"category"`
	IsLand       bool   `json:"is_land"`
	EntersTapped bool   `json:"enters_tapped"`
	// Produces is the mana this permanent makes once it is on the battlefield
	// and able.
	Produces []Source `json:"produces"`
	// ProduceDelay is turns before `Produces` is usable: 0 for rocks and
	// lands, 1 for a creature with summoning sickness.
	ProduceDelay int `json:"produce_delay"`
	// FetchesLands is land-fetch ramp (Cultivate, Rampant Growth): lands moved
	// to the battlefield. Such a card has no `Produces` at all, which is the
	// distinction the curve advice was systematically wrong about until it
	// learned it.
	FetchesLands int `json:"fetches_lands"`
}

// MV is the card's mana value.
func (c Card) MV() int { return c.Cost.ManaValue() }

// IsRamp reports the card makes mana, or fetches lands.
func (c Card) IsRamp() bool { return len(c.Produces) > 0 || c.FetchesLands > 0 }
