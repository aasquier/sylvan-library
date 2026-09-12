package reference

import (
	"strings"
)

// The cauldron lore's three readers, over the corpus this package already
// embeds: what may be told at a pot with these things in it, one fact by the
// id the witch cited, and the block of prose the theme interview's frame
// message carries.
//
// They live here rather than in `claude` for the reason the tarot lore's do:
// the data does, and the corpus is reference prose -- finite, editable, free
// -- so its readers belong beside it.
//
// **The fact ids are the contract.** A citation renders the corpus's own
// sentence and discards the model's paraphrase, so an id that resolves to
// nothing has to be a dropped fact rather than a dropped turn --
// [CauldronFactByID] answers nil and the caller counts it, the way an
// unresolvable citation is treated everywhere else in this package's
// neighbourhood.
//
// This file is `tarotlore.go` with one corpus swapped for another, and the
// duplication is deliberate rather than lazy. The alternative is a generic
// fact reader over an interface, and what that buys is one copy of eight
// short functions; what it costs is that the two corpora then have to keep
// agreeing about what a fact IS. They do today and they should not have to:
// a card is dealt and an ingredient is chosen, a card has a tier of facts
// true of the deck and an ingredient has a tier true of the pot, and the day
// one of those diverges the generic version is the thing standing in the way.

// CauldronFactByID is the fact the witch cited, or nil if she invented the id.
//
// Case- and space-insensitive, which is not politeness: a prefix check on the
// citation is made case-insensitively, so a turn that shouts
// `CAULDRON:ROWAN-RED-THREAD` gets past it and would then miss here on the one
// difference nobody would ever debug from a dropped-fact counter.
//
// Folded with [tarotFold] -- the other corpus's helper, called by its own
// name rather than copied under a new one. It is one rule (lower, then trim
// the wide space class), and the reason it exists is that both corpora are
// cited through the same case-insensitive prefix check. A second copy would
// be a second rule waiting to disagree with the first about what a
// non-breaking space is.
func CauldronFactByID(id string) *CauldronFact {
	wanted := tarotFold(id)
	for i := range cauldron.Facts {
		if tarotFold(cauldron.Facts[i].ID) == wanted {
			return &cauldron.Facts[i]
		}
	}
	return nil
}

// CauldronFactsForIngredient is everything known about one thing that goes in.
func CauldronFactsForIngredient(key string) []CauldronFact {
	out := []CauldronFact{}
	for _, f := range cauldron.Facts {
		if f.Ingredient != "" && f.Ingredient == key {
			out = append(out, f)
		}
	}
	return out
}

// CauldronFactsForBrew is what may be told at a pot with these things in it.
//
// **The pot tier first and always**: it is true of every brew, and it is why
// a pot of three quiet ingredients is not a pot with nothing to say. Then the
// ingredients, in the order they were chosen, so a reader skimming the list
// meets the pot's own contents in the pot's own order.
//
// Duplicates are impossible -- the pick takes one ingredient per category, so
// nothing can be in twice -- but the seen set keeps it that way if the pick
// ever changes.
func CauldronFactsForBrew(keys []string) []CauldronFact {
	out := []CauldronFact{}
	seen := map[string]bool{}
	for _, f := range cauldron.Facts {
		if f.Ingredient == "" && !seen[f.ID] {
			seen[f.ID] = true
			out = append(out, f)
		}
	}
	for _, key := range keys {
		for _, f := range CauldronFactsForIngredient(key) {
			if !seen[f.ID] {
				seen[f.ID] = true
				out = append(out, f)
			}
		}
	}
	return out
}

// CauldronOffer is the facts, as the witch's frame message carries them.
//
// Prose with ids rather than JSON, for the reason `Pot.Describe` gives: this
// is read by a model being asked to sound like a person, and a data structure
// invites a data-structure answer.
//
// `told` drops what this querent has already heard -- the same list the
// repeat check works against, applied one step earlier so she is not tempted
// by a fact she cannot use. Belt and braces on purpose: the prompt asks, this
// narrows what is asked about, and the citation check still checks. Returns
// an empty string when there is nothing left to offer, and the caller omits
// the whole section rather than printing a heading over nothing.
func CauldronOffer(keys []string, told []string) string {
	seen := map[string]bool{}
	for _, t := range told {
		seen[strings.TrimFunc(t, tarotIsSpace)] = true
	}
	lines := []string{}
	for _, f := range CauldronFactsForBrew(keys) {
		if seen[strings.TrimFunc(f.Text, tarotIsSpace)] {
			continue
		}
		lines = append(lines, "- "+f.ID+": "+f.Text)
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n\nTrue things you know about this pot and what is going into " +
		"it. To tell one, put its id in the fact's `source` field as " +
		"`cauldron:<id>` — the exact words below are what they will read, " +
		"so choose the one that belongs and let your question carry the " +
		"connection. Never retell one, and never write your own.\n" +
		strings.Join(lines, "\n")
}
