// Package brew is the witch's cauldron: twenty-four ingredients in three
// categories, and the seeded pick of one from each.
//
// It is the second prop, and it is built the way the first one was. The
// fortune-teller has a tarot spread on the table before a word is said
// (`internal/tarot`); the witch has a pot already boiling, and this is what
// is going into it. **Deterministic code decides** (ADR 14): which three
// things the pot asks for has a right answer and lives here, while what any
// of it MEANS about the person on the other side of the steam has none and
// belongs to her.
//
// The load-bearing coupling is in [Order], and it is the same one
// `tarot.Spread` carries: its three places ARE the theme interview's first
// three slot kinds, in the interview's own order, and `len(Order)` is that
// interview's floor. So an ingredient is chosen *for* a slot, and the
// mechanic Aaron ruled falls out of it without touching a line of the
// interview -- when the querent's own words ground a slot, that slot's
// ingredient goes in; three grounded slots and the brew is ready. The
// querent's words stay the only evidence, because an ingredient is not
// something they said. That failure is silent, so a test pins the three
// strings. It pins them as **literals**: this package must not import
// `internal/claude`, so the coupling is checked the way `tarot` checks it,
// by writing the words down twice and asserting they agree.
//
// # A simpler contract than the deck's, on purpose
//
// The tarot deal is a weighted sample without replacement over 136 cards,
// and it drags `internal/floats` in with it -- a compensated running total,
// because a bare `total += w` is a different arithmetic in its last bits and
// the recorded corpus holds the compensated answer. **None of that is copied
// here, and the omission is the design.** A new prop gets to invent its own
// contract, and this one's is: three categories, uniform within each, one
// pick per category, no weights and therefore no floating point anywhere in
// the answer. The draw is [mt19937.Random.RandBelow], an integer rejection
// loop, so there is no float arithmetic in this package for a golden to have
// to pin.
//
// What that buys is stated plainly so nobody re-adds the machinery looking
// for parity: an ingredient here is exactly as likely as its seven
// neighbours, a pot cannot repeat an ingredient (each pick is from a
// different category), and the whole determinism surface is the catalogue's
// ORDER plus the generator.
//
// # A seed is a promise
//
// The same promise `internal/mt19937` was built bit-exact to keep. A brew
// outlives the request that produced it: the client carries one integer, and
// a reload must put the same three things in the pot or it is a different
// brew for the same person. The catalogue's order is therefore part of the
// answer and not a presentation detail -- the pick is by INDEX within a
// category, so moving an entry, or adding one, changes what every existing
// seed brews for that category. `data/cauldron.json` says so at the top and
// `testdata/deals.json` is the tripwire.
package brew

import (
	cryptorand "crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/mt19937"
)

//go:embed data/cauldron.json
var cauldronJSON []byte

// Ingredient is one thing that can go in the pot.
//
// Key is its identity and how `internal/reference`'s cauldron lore addresses
// it; Category is which of [Order]'s slots it is filed under; Note is what
// the witch sees it do in the water, which is flavour rather than fact --
// the true things about it live in the lore corpus, cited by id, never
// paraphrased here. That division is `tarot`'s, deliberately: the deck holds
// no card's meaning either.
type Ingredient struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Note     string `json:"note"`
}

// Position is a place in the pot, and the slot the witch is fishing for
// there.
type Position struct {
	// Slot is the slot kind from the theme interview. The load-bearing field.
	Slot string `json:"slot"`
	// Name is what she calls it out loud.
	Name string `json:"name"`
	// Asks is what the place is asking, in her own frame. It goes into the
	// prompt; it is not shown as a label, because a pot that explains itself
	// in advance is a form with steam coming off it.
	Asks string `json:"asks"`
}

var (
	// Catalogue is everything that can go in, in the one order that matters:
	// the pick walks a category's slice by index, so reordering it changes
	// what a seed brews. Grouped by category in the file for a reader's sake,
	// which the loader does not rely on.
	Catalogue []Ingredient

	// Order is three places, three required slots, in the order the interview
	// wants them: what you already are, what happens when it goes wrong, who
	// you are with others.
	Order []Position

	// ByCategory is the catalogue filed under each of [Order]'s slots, each
	// slice in catalogue order. This is what the pick indexes into.
	ByCategory map[string][]Ingredient

	// ByKey indexes the catalogue for lookups that are not picks.
	ByKey map[string]Ingredient
)

func init() {
	var doc struct {
		Order       []Position   `json:"order"`
		Ingredients []Ingredient `json:"ingredients"`
	}
	if err := json.Unmarshal(cauldronJSON, &doc); err != nil {
		panic(fmt.Sprintf("brew: the embedded cauldron is unreadable: %v", err))
	}
	Catalogue, Order = doc.Ingredients, doc.Order
	ByCategory = make(map[string][]Ingredient, len(Order))
	ByKey = make(map[string]Ingredient, len(Catalogue))
	for _, in := range Catalogue {
		if _, dup := ByKey[in.Key]; dup {
			panic(fmt.Sprintf("brew: cauldron.json names %q twice", in.Key))
		}
		ByKey[in.Key] = in
		ByCategory[in.Category] = append(ByCategory[in.Category], in)
	}
	// The checks below are at boot rather than in a test for one reason: the
	// pick calls RandBelow with a category's length, and RandBelow panics on
	// a non-positive bound. A category with nothing in it is therefore a
	// panic on somebody's request, four screens into a conversation. Caught
	// here it is a panic at start -- a build that must not ship, which is
	// what a damaged embed is.
	for _, place := range Order {
		if len(ByCategory[place.Slot]) == 0 {
			panic(fmt.Sprintf("brew: nothing in the catalogue is filed under %q, "+
				"so the pot could never be filled", place.Slot))
		}
	}
	for _, in := range Catalogue {
		var placed bool
		for _, place := range Order {
			placed = placed || place.Slot == in.Category
		}
		if !placed {
			panic(fmt.Sprintf("brew: %q is filed under %q, which is not a place "+
				"in the pot", in.Key, in.Category))
		}
	}
}

// Chosen is an ingredient and where in the pot it goes.
//
// It marshals through MarshalJSON rather than through struct tags, for the
// reason `tarot.Drawn` does: the recorded payload lists the ingredient's
// served fields and then the two the pick adds, `slot` and `position`, in
// that order, and an embedded struct would put them wherever the encoder
// felt like. Note which field is absent -- `category`, because on the table
// it is the position's `slot` and one question must not have two answers on
// the wire.
type Chosen struct {
	Ingredient Ingredient
	Position   Position
}

// MarshalJSON writes the ingredient's served fields and then the two the
// pick adds, in the recorded order.
func (c Chosen) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Key     string `json:"key"`
		Name    string `json:"name"`
		Note    string `json:"note"`
		Slot    string `json:"slot"`
		PosName string `json:"position"`
	}{
		Key: c.Ingredient.Key, Name: c.Ingredient.Name, Note: c.Ingredient.Note,
		Slot: c.Position.Slot, PosName: c.Position.Name,
	})
}

// Pot is what the cauldron is asking for. Seeded, so a reload wants the same
// three things.
//
// Seed is a *big.Int and not an int64, and that is not fastidiousness: the
// seed grammar is unbounded (seed.go holds it) and [Deal] echoes back the
// seed it was handed. A client may legitimately hold 2**70, and an int64
// would truncate it into a DIFFERENT brew returned under a DIFFERENT number,
// silently, on both halves of the promise this package exists to keep.
// big.Int marshals as a bare JSON number, so the wire shape is unchanged.
type Pot struct {
	Seed        *big.Int `json:"seed"`
	Ingredients []Chosen `json:"ingredients"`
}

// Deal picks one ingredient per category and lays them out in pot order.
//
// Seeded and returned with its seed, for the reason every long-running
// surface here is seeded (ADR 18): the brew outlives the request that
// produced it, and a pot that changed on reload would be a different brew for
// the same person.
//
// Pass nil for an unseeded pick, which mints one and re-seeds from it -- so
// the answer always carries a seed the client can come back with. `random`
// rather than a cryptographic source: this is a pick, not a credential, and
// being able to reproduce it from an integer is the whole point.
//
// The walk is one draw per place, in [Order]'s order, each against its own
// category -- and that is the recorded stream. Drawing all three from one
// shuffle, or taking the categories in any other order, would be a different
// walk of the generator and a different pot.
func Deal(seed *big.Int) Pot {
	used := seed
	if used == nil {
		used = big.NewInt(mintSeed())
	}
	rng := mt19937.NewFromBig(used)
	chosen := make([]Chosen, len(Order))
	for i, place := range Order {
		shelf := ByCategory[place.Slot]
		chosen[i] = Chosen{
			Ingredient: shelf[rng.RandBelow(int64(len(shelf)))],
			Position:   place,
		}
	}
	return Pot{Seed: new(big.Int).Set(used), Ingredients: chosen}
}

// Describe renders the pot as a line per ingredient, for the witch's prompt.
//
// Prose rather than JSON on purpose, the same call `tarot.Reading.Describe`
// makes: this is read by a model that is being asked to sound like a person,
// and handing it a data structure invites it to answer with one.
//
// **There is no second paragraph here and there is not going to be one.**
// The spread's `Describe` adds two, and both are DETECTED facts -- a Magic
// card that has walked into the reading, a trump that has landed twice at one
// table -- events the sampler can really produce and that a reader who is not
// told would read straight past. This pick has no such event: three
// categories, one from each, nothing rare can happen. An "and look, the pot
// has taken a shine to you" paragraph would be a rarity invented to match the
// other prop's shape, which is a lie with steam coming off it.
func (p Pot) Describe() string {
	lines := make([]string, 0, len(p.Ingredients))
	for _, c := range p.Ingredients {
		lines = append(lines, fmt.Sprintf("- %s (%s): %s — %s",
			c.Position.Name, c.Position.Asks, c.Ingredient.Name, c.Ingredient.Note))
	}
	return strings.Join(lines, "\n")
}

// mintSeed picks the integer an unseeded pick will be remembered by.
//
// A deliberate copy of `tarot.mintSeed`, which is package-private there.
// **Copied rather than shared, and the reason is the contract rather than the
// code**: the two props are two promises, and a seed minted for a pot must
// stay in the range a pot's clients have always been handed even if the
// deck's ever moves. The copy is three lines; a `internal/seedmint` shared by
// both would be a fourth package whose only job is to make two independent
// contracts move together.
//
// This is the one draw in this package that is NOT held to the recorded
// generator, and the licence is worth stating because every other line here
// is held: `mt19937` exists so that a seed SOMEBODY ALREADY HOLDS brews the
// same pot forever, and nobody holds a seed that has not been minted yet.
// What must match is the pick FROM a seed, which `mt19937.NewFromBig`
// guarantees; which unheld integer gets chosen is not observable by anyone.
//
// crypto/rand rather than math/rand so that two processes starting in the
// same millisecond cannot hand two people the same brew. It cannot fail on
// any platform this runs on; if it ever did, a fixed seed would silently give
// every visitor the same pot, so the failure is loud.
func mintSeed() int64 {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("brew: no entropy to draw from: %v", err))
	}
	// Masked to 31 bits, the same range the deck mints in: the seed is
	// rendered in a URL, and a negative or 64-bit one would be a surprise to
	// the client.
	return int64(binary.BigEndian.Uint64(b[:]) & (1<<31 - 1))
}
