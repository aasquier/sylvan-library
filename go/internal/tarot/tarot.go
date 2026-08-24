// Package tarot is the 78-card deck, Magic's answers to it, the weighted
// shuffle, and the three-card spread.
//
// **Deterministic code decides** (ADR 14). A shuffle has a right answer and
// lives here; what a spread *means* has none and belongs to the reader. So
// this package holds all 136 cards and **no card's meaning** — that corpus
// is `internal/reference`'s tarot lore, and the reader quotes it by id
// rather than paraphrasing it.
//
// The load-bearing coupling is in SPREAD: its three slots ARE the theme
// interview's first three slot kinds, so a card is dealt *for* a slot and
// ADR 20's grounded-quote readiness works untouched — the querent's own words
// stay the only evidence, and a card is not something they said. That failure
// is silent, so a test pins it: drift, and the proposal button simply never
// lights up.
//
// # A seed is a promise
//
// This is `internal/mt19937`'s first real caller, and the reason it was
// built bit-exact rather than merely well-distributed. A reading outlives
// the request that produced it: the client carries one integer, and a
// reload must deal the same three cards or it is a different reading of the
// same person. That promise holds for seeds people already carry — which no
// amount of merely-good shuffling would give. Only a generator reproduced
// bit for bit against its recorded corpus does.
package tarot

import (
	cryptorand "crypto/rand"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/floats"
	"github.com/aasquier/sylvan-library/go/internal/mt19937"
)

//go:embed data/deck.json
var deckJSON []byte

// Card is one card. Key is both its identity and its picture's filename —
// except for a Magic crossover, whose picture is a hotlinked art crop and
// whose extra fields say so.
type Card struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Arcana string `json:"arcana"`
	// Suit is nil for a trump. A *string rather than "" because it marshals
	// as null, which is what the client checks.
	Suit   *string `json:"suit"`
	Number int     `json:"number"`
	ArtURL *string `json:"art_url"`
	Artist *string `json:"artist"`
	// After names the trump a Magic card answers to.
	After *string `json:"after"`
	// Echo is true for a real Magic card whose name and art carry a trump, as
	// opposed to the three printed *as* tarot cards. The reader is told which
	// kind landed, because "printed after The Fool" is a design fact and
	// "answers to The Tower" is an editorial one.
	Echo bool `json:"echo"`
	// Weight is how often it lands, against a natural card's 1.0.
	Weight float64 `json:"weight"`
	Note   *string `json:"note"`
	// Image and FaceName are derived facts, rendered into the data rather
	// than recomputed here — there is no second implementation to disagree.
	Image    string `json:"image"`
	FaceName string `json:"face_name"`
}

// Position is a place in the spread, and the slot the reader is fishing for
// there.
type Position struct {
	// Slot is the slot kind from the theme interview. The load-bearing field.
	Slot string `json:"slot"`
	// Name is what the reader calls it out loud.
	Name string `json:"name"`
	// Asks is what the position is asking, in the reader's own frame. It goes
	// into the prompt; it is not shown as a label, because a spread that
	// explains itself in advance is a form with candles on it.
	Asks string `json:"asks"`
}

var (
	// FullDeck is what actually gets shuffled: 136 cards, every trump answered
	// by a Magic card that carries it.
	//
	// Its ORDER is part of the answer, not a presentation detail: the sampler
	// walks this slice accumulating weight until it passes the mark. Reorder
	// it and every seed deals differently.
	FullDeck []Card

	// Spread is three cards, three required slots, in the order a reading
	// wants them: what you already are, what happens when it goes wrong, who
	// you are with others.
	Spread []Position

	// ByKey indexes the deck for lookups that are not deals.
	ByKey map[string]Card
)

func init() {
	var doc struct {
		Spread []Position `json:"spread"`
		Cards  []Card     `json:"cards"`
	}
	if err := json.Unmarshal(deckJSON, &doc); err != nil {
		panic(fmt.Sprintf("tarot: the embedded deck is unreadable: %v", err))
	}
	FullDeck, Spread = doc.Cards, doc.Spread
	ByKey = make(map[string]Card, len(FullDeck))
	for _, c := range FullDeck {
		ByKey[c.Key] = c
	}
}

// Drawn is a card, where it landed, and which way up.
//
// It marshals through MarshalJSON rather than through struct tags, because
// the recorded payload lists every card field first and then the three keys
// the deal adds — `reversed`, `slot`, `position`, in that order. An
// embedded struct would put them wherever the encoder felt like.
type Drawn struct {
	Card     Card
	Position Position
	// Reversed cards are traditional and they are free: the picture is rotated
	// in CSS. They give the reader somewhere to go when a card's upright
	// reading does not fit the person in front of them.
	Reversed bool
}

// MarshalJSON writes the card's served fields and then the three the deal
// adds, in the recorded order.
//
// Written WITH the type rather than when a route needed it, which is the
// lesson `tier1.Number` cost: that type was bit-exact by repr and by
// Float64bits and still went onto the wire as a struct dump, because
// nothing had ever asked what encoding/json did with it. Note also which card fields
// are absent — `art_url`, `echo` and `weight` are the sampler's business and
// the reader's, never the browser's.
func (d Drawn) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Key      string  `json:"key"`
		Name     string  `json:"name"`
		Arcana   string  `json:"arcana"`
		Suit     *string `json:"suit"`
		Number   int     `json:"number"`
		Image    string  `json:"image"`
		Artist   *string `json:"artist"`
		After    *string `json:"after"`
		Note     *string `json:"note"`
		FaceName string  `json:"face_name"`
		Reversed bool    `json:"reversed"`
		Slot     string  `json:"slot"`
		PosName  string  `json:"position"`
	}{
		Key: d.Card.Key, Name: d.Card.Name, Arcana: d.Card.Arcana,
		Suit: d.Card.Suit, Number: d.Card.Number, Image: d.Card.Image,
		Artist: d.Card.Artist, After: d.Card.After, Note: d.Card.Note,
		FaceName: d.Card.FaceName, Reversed: d.Reversed,
		Slot: d.Position.Slot, PosName: d.Position.Name,
	})
}

// Reading is a dealt spread. Seeded, so a reload shows the same cards.
//
// Seed is a *big.Int and not an int64, which is not fastidiousness: the
// seed grammar is unbounded (seed.go holds it), and `Deal` echoes back the
// seed it was handed. A client may legitimately hold
// 2**70 — and an int64 would truncate it into a DIFFERENT reading returned
// under a DIFFERENT number, silently, on both halves of the promise this
// package exists to keep. big.Int marshals as a bare JSON number, so the wire
// shape is unchanged.
type Reading struct {
	Seed  *big.Int `json:"seed"`
	Cards []Drawn  `json:"cards"`
}

// weightedSample draws k distinct cards from FullDeck, crossovers at their
// weight.
//
// A weighted draw without replacement has no single library spelling, so
// this is the classic successive draw: pick against the remaining total,
// remove, repeat.
//
// **The running total is Fsum because deterministic has to mean
// deterministic everywhere.** A bare running total over floats is method-
// and order-sensitive in its last bits, and the recorded totals are the
// compensated answer. EchoWeight is 0.14, which no binary float holds
// exactly, so this was not a corner case: all 9,180 of the 134-card pools
// this loop reaches on its third draw total differently under a plain
// `total += w` than under `Fsum` — and the corpus holds the Fsum answer.
// It returns the running totals it used alongside the cards, and Deal
// discards them. That second value exists for one reason: the fsum this
// function depends on is invisible in the cards. A test that recomputes
// floats.Fsum by hand proves the floats package works and says nothing
// about whether THIS loop calls it — the mutation survives, which was
// measured rather than guessed. Handing the totals back is the smallest change that lets a test
// drive the real path instead of a re-implementation of it.
func weightedSample(rng *mt19937.Random, k int) ([]Card, []float64) {
	pool := make([]Card, len(FullDeck))
	copy(pool, FullDeck)
	out := make([]Card, 0, k)
	totals := make([]float64, 0, k)
	for range k {
		weights := make([]float64, len(pool))
		for i, c := range pool {
			weights[i] = c.Weight
		}
		total := floats.Fsum(weights)
		totals = append(totals, total)
		mark := rng.Float64() * total
		acc := 0.0
		picked := -1
		for i, c := range pool {
			acc += c.Weight
			if mark < acc {
				picked = i
				break
			}
		}
		if picked < 0 {
			// Float summation can leave mark a hair past acc; the last card
			// was the answer.
			picked = len(pool) - 1
		}
		out = append(out, pool[picked])
		pool = append(pool[:picked], pool[picked+1:]...)
	}
	return out, totals
}

// Deal shuffles, cuts, and lays three cards out.
//
// Seeded and returned with its seed, for the reason every long-running surface
// here is seeded (ADR 18): the reading outlives the request that produced it,
// and a spread that changed on reload would be a different reading of the same
// person.
//
// Pass nil for an unseeded deal, which mints one and re-seeds from it — so the
// answer always carries a seed the client can come back with. `random` rather
// than a cryptographic source: this is a shuffle, not a credential, and being
// able to reproduce it from an integer is the whole point.
func Deal(seed *big.Int) Reading {
	used := seed
	if used == nil {
		used = big.NewInt(mintSeed())
	}
	rng := mt19937.NewFromBig(used)
	drawn, _ := weightedSample(rng, len(Spread))
	cards := make([]Drawn, len(drawn))
	for i, card := range drawn {
		// Order matters: the recorded stream rolls every reversal after
		// every card is picked, one per card, in spread order — a reversal
		// interleaved with the draws would be a different walk of the
		// generator and a different spread.
		cards[i] = Drawn{Card: card, Position: Spread[i], Reversed: rng.Float64() < 0.5}
	}
	return Reading{Seed: new(big.Int).Set(used), Cards: cards}
}

// Describe renders the spread as a line per card, for the reader's prompt.
//
// Prose rather than JSON on purpose: this is read by a model that is being
// asked to sound like a person, and handing it a data structure invites it to
// answer with one.
//
// Two paragraphs can follow the cards, and both are detected facts rather than
// interpretations. A Magic card in the spread is called out as an omen the
// reader should give precedence — the game the reading serves has walked into
// it. And a trump landing twice, once as the 1909 printing and once as Magic's
// own card, is named as the alignment it is: the sampler draws without
// replacement across all 136, so the two Fools really can share a table, and a
// reader who is not told how rare that is will read past it.
func (r Reading) Describe() string {
	lines := make([]string, 0, len(r.Cards))
	for _, d := range r.Cards {
		line := fmt.Sprintf("- %s (%s): %s", d.Position.Name, d.Position.Asks, d.Card.Name)
		switch {
		case d.Card.After != nil && d.Card.Echo:
			line += fmt.Sprintf(" — a real Magic card whose art and name answer to "+
				"%s; read it as %s wearing Magic's own painting",
				*d.Card.After, *d.Card.After)
		case d.Card.After != nil:
			line += fmt.Sprintf(" — a Magic card printed after %s, "+
				"which you may read as you would read %s",
				*d.Card.After, *d.Card.After)
		}
		if d.Reversed {
			line += ", reversed"
		}
		lines = append(lines, line)
	}
	text := strings.Join(lines, "\n")

	for _, d := range r.Cards {
		if d.Card.After != nil {
			text += "\n\nA Magic card on this table is an omen in its own right: " +
				"the game this reading is in service of has walked into the " +
				"spread. Give it precedence — let it colour the reading more " +
				"than a natural card would, say so plainly, and make sure " +
				"they feel how uncommon a visit it is."
			break
		}
	}

	// Keyed on suit as well as number since the echoes reached into the
	// minors: the natural Ace of Swords and its Magic answer aligning is every
	// bit the event the two Fools are.
	//
	// A slice of keys in first-seen order, not a map: the paragraphs follow
	// the spread's own order, and with two alignments in one spread a bare
	// map would order them by iteration randomness instead.
	type trumpKey struct {
		arcana string
		suit   string
		number int
	}
	var order []trumpKey
	grouped := map[trumpKey][]string{}
	for _, d := range r.Cards {
		suit := ""
		if d.Card.Suit != nil {
			suit = *d.Card.Suit
		}
		k := trumpKey{d.Card.Arcana, suit, d.Card.Number}
		if _, seen := grouped[k]; !seen {
			order = append(order, k)
		}
		grouped[k] = append(grouped[k], d.Card.Name)
	}
	for _, k := range order {
		names := grouped[k]
		if len(names) > 1 {
			text += fmt.Sprintf("\n\nThe stars have aligned: one trump has landed twice "+
				"at this table, as %s. That is the rarest thing this spread can do. "+
				"Make it the centre of the reading, and tell them exactly what they "+
				"are looking at.", joinAnd(names))
		}
	}
	return text
}

// joinAnd joins the names sharing a trump with " and ".
func joinAnd(names []string) string { return strings.Join(names, " and ") }

// mintSeed picks the integer an unseeded deal will be remembered by.
//
// The one draw in this package that is NOT held to the recorded generator,
// and the licence is worth stating because every other line here is held:
// `mt19937` exists so that a seed SOMEBODY ALREADY HOLDS deals the same
// three cards forever, and nobody holds a seed that has not been minted
// yet. What must match is the deal FROM a seed, which `mt19937.New`
// guarantees; which unheld integer gets chosen is not observable by anyone.
//
// crypto/rand rather than math/rand so that two processes starting in the same
// millisecond cannot hand two people the same reading. It cannot fail on any
// platform this runs on; if it ever did, a fixed seed would silently give
// every visitor the same spread, so the failure is loud.
func mintSeed() int64 {
	var b [8]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("tarot: no entropy to deal from: %v", err))
	}
	// Masked to 31 bits — the minted range has always been [0, 2**31): the
	// seed is rendered in a URL, and a negative or 64-bit one would be a
	// surprise to the client.
	return int64(binary.BigEndian.Uint64(b[:]) & (1<<31 - 1))
}
