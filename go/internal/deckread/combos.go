package deckread

import (
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The combos block on the wire.
//
// **Every name is resolved here rather than on the page**, and that is the one
// design decision in this file. A combos entry is three or four card names in
// a heading, and the page wants a painting for each of them the moment somebody
// hovers one. Serving the names alone would mean the deck page firing a
// lookup per name after it had already rendered -- for cards it has, in a
// payload that already went past the pool. So the row carries the picture, and
// `PoolFor` was widened to fetch it in the same query the rest of the deck uses.
//
// **`in_deck` is served rather than worked out by the client.** The page could
// intersect the names against the 99 itself, and would get it wrong in exactly
// one place: the commander. Half the combos in Commander run through the card
// in the command zone, which is not in `cards` and is emphatically in the deck.
// One reading of that, on the side that has the deck.

// ComboCardRef is one card a combo names, with what the page needs to draw it.
type ComboCardRef struct {
	Name string
	// The whole card, hot-linked (ADR 6) -- what a hover shows. Null for a name
	// the pool does not know, which is a state the page has to draw rather than
	// a state to hide: a misspelling that silently rendered nothing would be a
	// combo missing a piece with nothing saying so.
	Image *string
	// The painting alone, for a thumbnail. A full card at 60 pixels is a
	// grey rectangle; this is the crop every other card row on the deck page
	// draws at that size.
	ArtCrop *string
	// Whether this deck actually has the card: in the 99, or in the command
	// zone. False is meaningful rather than missing -- it is what makes a
	// near-miss a near-miss.
	InDeck bool
}

// MarshalJSON writes the ref in its recorded key order.
func (r ComboCardRef) MarshalJSON() ([]byte, error) {
	return wire.MarshalOrdered([]wire.KV{
		{Key: "name", Value: r.Name},
		{Key: "image", Value: r.Image},
		{Key: "art_crop", Value: r.ArtCrop},
		{Key: "in_deck", Value: r.InDeck},
	})
}

// ComboJSON is one catalogued machine.
type ComboJSON struct {
	Cards    []ComboCardRef
	Produces string
	How      string
	Setup    string
	// Needs and Cut are the near-miss pair, null on a machine that assembles.
	Needs *ComboCardRef
	Cut   *ComboCardRef
	// By is `claude` on a drafted entry and empty on one a person wrote
	// (ADR 41's rule, applied to a block instead of a sentence). Omitted when
	// empty, for the reason `CardJSON.WhyBy` is: a mark is a thing that is
	// *there*, and writing `"by": ""` on every entry hands the page an empty
	// string to weigh on each of them.
	By string
}

// MarshalJSON writes the row key by key, like every other body here. The
// struct is the shape; this is the wire.
func (c ComboJSON) MarshalJSON() ([]byte, error) {
	out := []wire.KV{
		{Key: "cards", Value: c.Cards},
		{Key: "produces", Value: c.Produces},
		{Key: "how", Value: c.How},
		{Key: "setup", Value: c.Setup},
		{Key: "needs", Value: c.Needs},
		{Key: "cut", Value: c.Cut},
	}
	if c.By != "" {
		out = append(out, wire.KV{Key: "by", Value: c.By})
	}
	return wire.MarshalOrdered(out)
}

// ComboRows is the deck's combos block, resolved.
//
// `cards` is what the pool knew, which may be an empty map when there is no
// pool at all -- and that is a degraded answer rather than a wrong one: every
// ref comes back with a null picture, the page draws the names, and the gate
// says separately that nothing was verified.
func ComboRows(d *deck.Deck, cards map[string]*pool.CardRecord) []ComboJSON {
	in := map[string]bool{}
	for _, card := range d.Cards {
		in[strings.ToLower(strings.TrimSpace(card.Name))] = true
	}
	for _, name := range d.Commander {
		in[strings.ToLower(strings.TrimSpace(name))] = true
	}
	ref := func(name string) ComboCardRef {
		out := ComboCardRef{Name: name, InDeck: in[strings.ToLower(strings.TrimSpace(name))]}
		if rec := cards[name]; rec != nil {
			out.Image, out.ArtCrop = rec.ImageNormal, rec.ImageArtCrop
		}
		return out
	}
	rows := []ComboJSON{}
	for _, combo := range d.Combos {
		row := ComboJSON{Cards: []ComboCardRef{}, Produces: combo.Produces,
			How: combo.How, Setup: combo.Setup, By: combo.By}
		for _, name := range combo.Cards {
			row.Cards = append(row.Cards, ref(name))
		}
		if combo.Needs != "" {
			needs := ref(combo.Needs)
			row.Needs = &needs
		}
		if combo.Cut != "" {
			cut := ref(combo.Cut)
			row.Cut = &cut
		}
		rows = append(rows, row)
	}
	return rows
}
