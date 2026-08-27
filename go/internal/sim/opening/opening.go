// Package opening deals one opening hand and counts what is in it.
//
// It exists for commandment 2 and for nothing else. A person who has never
// played Magic has no idea what seven cards off the top of a deck look like,
// how often those seven are three lands and four spells, or how often they
// are one land and a fistful of six-drops. You cannot learn that from a
// probability table; you learn it by dealing hands until the shape of them
// stops surprising you. So this deals a hand, says plainly what is in it,
// and lets somebody deal another.
//
// # It counts. It does not advise.
//
// ADR 14 splits the world: deterministic code decides, Claude advises. A
// keep-or-mulligan call is an *opinion*, and this package deliberately does
// not have one -- not a verdict, not a score, not a traffic light. What it
// reports is arithmetic anybody could do by hand with the seven cards face
// up on the table: how many are lands, which of the commander's colours those
// lands can actually pay for, the earliest turn any spell here could be cast,
// and how many of the seven are reachable by turn three. The conclusion is
// the reader's, which is the whole point of the exercise -- a toy that told a
// newcomer "mulligan this" would teach them to ask it again instead of
// teaching them to read a hand.
//
// # The rule the numbers are counted under, and why it is the simple one
//
// **Lands only, one land a turn, nothing cast to help.** No mana rock is
// counted, no ritual, no fetch; a land that enters tapped costs the turn it
// really costs. That is deliberately the mental model a beginner already has
// -- "I play a land, I have that much mana" -- and it is the model the
// numbers must match, or the toy teaches something false. It runs
// *pessimistic*: a hand with Sol Ring in it will beat the number here, which
// is the safe direction for a figure nobody can check. The caveat says so in
// the same breath as the number ([Caveat]), because a simplification that is
// not stated is a lie with arithmetic on top.
//
// Within that rule the answer is exact rather than approximated. A hand's
// lands are at most seven, so every subset of them is enumerated (128 at the
// very worst) and each is asked of `mana.CanPay` -- the project's one mana
// solver, the same one Tier 1 pays its costs through, so a hand that says
// "turn two" here is a hand Tier 1 would also cast on turn two. Writing a
// second, simpler solver for a toy is how two answers about the same deck
// start disagreeing in public.
//
// # The shuffle is real and the seed is not on the wire
//
// The randomness is `internal/mt19937`, like every other random thing in this
// project, and [Deal] is a pure function of its seed: the same seed deals the
// same seven cards, for ever, which is what makes the corpus below a corpus
// and not a coincidence.
//
// What the *route* does with that is the part worth arguing. The Wheel takes
// a seed and reports one, because a fortune is a thing somebody wants to
// replay. A practice hand is not: Aaron's ask was explicit that a player just
// presses deal, and a seed control on a beginner's toy is a lever with
// nothing on the other end of it -- worse, it is a piece of machinery
// rendered at somebody who came here for cards (commandment 10). So the seed
// lives here, where the tests can hold it, and the server rolls a fresh one
// per deal and never says what it was.
package opening

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"math/bits"
	"sort"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/mana"
	"github.com/aasquier/sylvan-library/go/internal/mt19937"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/sim"
	"github.com/aasquier/sylvan-library/go/internal/sim/compile"
)

// Size is the opening seven. Not a parameter: this is the hand Magic deals,
// and a toy that let you deal nine would be teaching a different game.
const Size = 7

// Horizon is the turn `CastableByHorizon` counts through.
//
// Three, because that is the horizon a person actually holds in their head
// while deciding whether a hand can do anything -- Tier 1's own eight-turn
// figure is a question about a deck, and this is a question about seven
// cards. Named rather than inlined so the wire field and the sentence beside
// it cannot drift apart.
const Horizon = 3

// Caveat is what every dealt hand carries, and it is not decoration.
//
// Three claims, in the order somebody needs them: the hand came off a real
// shuffle, the counting is deliberately simple and here is how simple, and
// **nobody here has an opinion about whether to keep it**. The last clause is
// ADR 14 said in the room rather than in a document -- see the package
// comment.
const Caveat = "Seven cards off a real shuffle, then plain counting: " +
	"lands only, one land a turn, and nothing cast to help. No opponent, " +
	"no mulligan taken, and no opinion here about whether to keep it — " +
	"that part is yours."

// Card is one card as it lies on the table: enough to draw it, and the one
// number the reading computed about it.
//
// Field order is the wire order.
type Card struct {
	Name      string  `json:"name"`
	ManaCost  *string `json:"mana_cost"`
	TypeLine  string  `json:"type_line"`
	Image     *string `json:"image"`
	ManaValue int     `json:"mana_value"`
	IsLand    bool    `json:"is_land"`
	// Turn is the earliest turn this card could be cast off the lands in
	// this hand alone, or nil when these lands never pay for it. Nil for a
	// land, which is played rather than cast.
	Turn *int `json:"turn"`
}

// Reading is the arithmetic, and only the arithmetic.
//
// Every field is a count somebody could reproduce with the seven cards face
// up. There is deliberately no verdict field, no score and no colour-coded
// judgement: see the package comment.
type Reading struct {
	Lands  int `json:"lands"`
	Spells int `json:"spells"`
	// ColorsCovered and ColorsMissing partition the commander's colour
	// identity by whether a land in this hand can make it. Both sorted, both
	// empty rather than nil, and both empty together when the deck has no
	// commander to need anything.
	ColorsCovered []string `json:"colors_covered"`
	ColorsMissing []string `json:"colors_missing"`
	// FirstSpellTurn is the earliest turn any spell in this hand could be
	// cast, or nil when these lands cast none of them. Nil is a real and
	// common answer -- a one-land hand of four-drops -- and rendering it as
	// zero would read as "turn zero".
	FirstSpellTurn *int `json:"first_spell_turn"`
	// CastableByHorizon is how many of the seven could be cast on or before
	// [Horizon], each considered on its own rather than all together. "Three
	// of these could be cast by turn three" is a claim about reach, not about
	// a turn where you cast three spells.
	CastableByHorizon int `json:"castable_by_horizon"`
	Horizon           int `json:"horizon"`
}

// Hand is one deal: the cards, the counting, and enough about the deck they
// came out of to say whether the deal was over a whole deck.
type Hand struct {
	Cards   []Card  `json:"cards"`
	Reading Reading `json:"reading"`
	// DeckSize is how many cards were actually shuffled -- the compiled
	// library, which is the 99 and not the commander. It can be smaller than
	// the deck file says, which is what UnresolvedCount is for: a hand dealt
	// off a library six cards short came off a different deck, and saying so
	// is cheaper than being wrong quietly.
	DeckSize        int `json:"deck_size"`
	DeclaredSize    int `json:"declared_size"`
	UnresolvedCount int `json:"unresolved_count"`
	// Commander is the card whose colours the reading measured against, or
	// nil when the deck has none. Carried whole because the surface draws it
	// beside the hand -- it is the one card that is always available, and a
	// newcomer counting their colours needs to see what they are counting
	// toward.
	Commander *Card `json:"commander"`
	// AnsweredBy and Caveat travel with the numbers rather than beside them,
	// so a client cannot render the figures and drop the sentence that says
	// what kind of figures they are.
	AnsweredBy string `json:"answered_by"`
	Caveat     string `json:"caveat"`
}

// Deal shuffles the deck and turns over seven.
//
// `seed` nil rolls a fresh one, exactly as `wheel.Spin` does; a seed supplied
// replays the deal bit for bit, which is what the corpus rests on. The seed
// is never returned: see the package comment for why the route has no lever
// for it.
//
// The refusals are compile's, unchanged and deliberately not wrapped: a deck
// with no pool behind it is `*compile.PoolRequired` and a deck that compiles
// to nothing is `*compile.NothingToSimulate`, and the surface tells those two
// apart because they are different sentences to a person.
func Deal(d *deck.Deck, cards map[string]*pool.CardRecord, seed *big.Int) (*Hand, error) {
	report, err := compile.Compile(d, cards)
	if err != nil {
		return nil, err
	}
	if seed == nil {
		fresh, err := crand.Int(crand.Reader, new(big.Int).Lsh(big.NewInt(1), 32))
		if err != nil {
			return nil, fmt.Errorf("opening: %w", err)
		}
		seed = fresh
	}
	rng := mt19937.NewFromBig(seed)

	// A copy, always: the report's library is aliased qty times over and is
	// nobody's to reorder.
	library := append([]*sim.Card(nil), report.Library...)
	mt19937.ShuffleSlice(rng, library)
	drawn := library[:min(Size, len(library))]

	turns := earliestTurns(drawn)
	hand := make([]Card, len(drawn))
	for i, c := range drawn {
		hand[i] = describe(c, cards[c.Name], turns[i])
	}
	return &Hand{
		Cards:           hand,
		Reading:         read(drawn, turns, identityOf(d, cards)),
		DeckSize:        report.SimulatedSize(),
		DeclaredSize:    report.DeclaredSize,
		UnresolvedCount: len(report.Unresolved),
		Commander:       commanderCard(report.Commander, cards),
		AnsweredBy:      "dice and counting",
		Caveat:          Caveat,
	}, nil
}

// describe joins a compiled card to its pool record, which is where the
// picture and the printed cost live. A record that went missing cannot
// happen -- a card only compiled because the record was there -- but the nil
// check stays, because the alternative to it is a panic on a surface a
// newcomer is looking at.
func describe(c *sim.Card, rec *pool.CardRecord, turn *int) Card {
	out := Card{Name: c.Name, ManaValue: c.MV(), IsLand: c.IsLand, Turn: turn}
	if rec != nil {
		out.ManaCost, out.TypeLine, out.Image = rec.ManaCost, rec.TypeLine, rec.ImageNormal
	}
	return out
}

// commanderCard is the commander as a table card, or nil. It carries no turn:
// the command zone is not the hand, and a commander's turn is a different
// question with a different answer (Tier 1 owns it).
func commanderCard(c *sim.Card, cards map[string]*pool.CardRecord) *Card {
	if c == nil {
		return nil
	}
	out := describe(c, cards[c.Name], nil)
	return &out
}

// identityOf is the commander's colour identity, from Scryfall's own field
// and never derived from a mana cost (CLAUDE.md rule 2). Sorted, and empty
// for a deck with no commander or a colourless one -- in which case the
// reading simply has no colours to report, which is the honest answer rather
// than an omission.
func identityOf(d *deck.Deck, cards map[string]*pool.CardRecord) []string {
	if len(d.Commander) == 0 {
		return nil
	}
	rec := cards[d.Commander[0]]
	if rec == nil {
		return nil
	}
	out := append([]string(nil), rec.ColorIdentity...)
	sort.Strings(out)
	return out
}

// read is the whole of the arithmetic.
func read(hand []*sim.Card, turns []*int, identity []string) Reading {
	r := Reading{ColorsCovered: []string{}, ColorsMissing: []string{}, Horizon: Horizon}
	var makeable []sim.Source
	for _, c := range hand {
		if c.IsLand {
			r.Lands++
			makeable = append(makeable, c.Produces...)
			continue
		}
		r.Spells++
	}
	for _, color := range identity {
		want := map[string]bool{color: true}
		covered := false
		for _, src := range makeable {
			if src.Makes(want) {
				covered = true
				break
			}
		}
		if covered {
			r.ColorsCovered = append(r.ColorsCovered, color)
		} else {
			r.ColorsMissing = append(r.ColorsMissing, color)
		}
	}
	for _, turn := range turns {
		if turn == nil {
			continue
		}
		if r.FirstSpellTurn == nil || *turn < *r.FirstSpellTurn {
			t := *turn
			r.FirstSpellTurn = &t
		}
		if *turn <= Horizon {
			r.CastableByHorizon++
		}
	}
	return r
}

// earliestTurns is the earliest turn each card in the hand could be cast off
// the lands in the same hand, positionally alongside it. Nil for a land, and
// nil for a spell these lands never pay for.
//
// # The enumeration, and why it is exact
//
// Play one land a turn and cast nothing else, and the mana available on a
// turn is exactly *some subset of the lands in this hand*. So enumerate the
// subsets. A subset S of size k is fully available on turn k -- unless every
// land in it enters tapped, in which case the one played on turn k is still
// tapped and S is only whole on turn k+1. (You can always arrange for the
// last land played to be an untapped one when the subset holds any, and a
// subset that is only *partly* available on some turn is a smaller subset
// this loop also visits.) Take the earliest turn over every subset that pays.
//
// At most seven lands, so at most 128 subsets, asked of `mana.CanPay` once
// per spell: a few hundred solver calls for a hand, which is nothing beside
// the round trip that carried the request.
func earliestTurns(hand []*sim.Card) []*int {
	var lands []*sim.Card
	for _, c := range hand {
		if c.IsLand {
			lands = append(lands, c)
		}
	}
	// Each subset paired with the turn it is whole on, cheapest turn first,
	// so the first subset that pays for a card is that card's answer.
	type option struct {
		turn    int
		sources []mana.Source
	}
	options := make([]option, 0, 1<<len(lands))
	for mask := 1; mask < 1<<len(lands); mask++ {
		var sources []mana.Source
		untapped := false
		for i, land := range lands {
			if mask&(1<<i) == 0 {
				continue
			}
			if !land.EntersTapped {
				untapped = true
			}
			for _, src := range land.Produces {
				sources = append(sources, mana.Source(src))
			}
		}
		turn := bits.OnesCount(uint(mask))
		if !untapped {
			turn++
		}
		options = append(options, option{turn: turn, sources: sources})
	}
	sort.SliceStable(options, func(a, b int) bool { return options[a].turn < options[b].turn })

	out := make([]*int, len(hand))
	for i, c := range hand {
		if c.IsLand {
			continue
		}
		cost := mana.Cost{
			Generic:   c.Cost.Generic,
			Pips:      c.Cost.Pips,
			HasX:      c.Cost.HasX,
			Phyrexian: c.Cost.Phyrexian,
		}
		for _, opt := range options {
			// xValue 0, for Tier 1's reason: a spell cast for X of nought is
			// still a spell cast, and guessing a bigger X would be inventing
			// a play rather than counting one.
			if mana.CanPay(cost, opt.sources, 0) {
				turn := opt.turn
				out[i] = &turn
				break
			}
		}
	}
	return out
}

// DealFromPool is [Deal] with the pool read done for it: the one call a
// served route makes, so the route never has to know which names a deck
// puts in front of the pool.
func DealFromPool(ctx context.Context, c *pool.Conn, d *deck.Deck,
	seed *big.Int) (*Hand, error) {
	cards, err := poolFor(ctx, c, d)
	if err != nil {
		return nil, err
	}
	return Deal(d, cards, seed)
}

// poolFor is the deck's names looked up. Only the commander and the 99 --
// the swap board and the graveyard are not shuffled into anything, and
// asking for them would be a wider query for cards no hand can contain.
func poolFor(ctx context.Context, c *pool.Conn, d *deck.Deck) (map[string]*pool.CardRecord, error) {
	if c == nil {
		return nil, &compile.PoolRequired{}
	}
	names := append([]string{}, d.Commander...)
	for _, card := range d.Cards {
		names = append(names, card.Name)
	}
	return c.GetCards(ctx, names)
}
