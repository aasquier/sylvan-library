package reference

import (
	"strings"
	"unicode"
)

// `tarotlore.py`'s three readers, over the corpus this package already
// embeds: what may be told at a table where these cards are face up, one
// fact by the id a reader cited, and the block of prose the theme
// interview's frame message carries.
//
// They live here rather than in `claude` because the data does, and because
// `tarot` reads them too -- the corpus is reference prose, the fourth of its
// kind, and its readers belong beside it.
//
// **The fact ids are the contract.** `theme.KeepFact` renders the corpus's
// own sentence for a `tarot:<id>` citation and discards the model's
// paraphrase, so an id that resolves to nothing has to be a dropped fact
// rather than a dropped turn -- `TarotFactByID` answers nil and the caller
// counts it, the way an unresolvable citation is treated everywhere else in
// this package's neighbourhood.

// TarotFactByID is the fact a reader cited, or nil if it invented the id.
//
// Case- and space-insensitive, which is not politeness. `theme.KeepFact`
// matches the `tarot:` prefix case-insensitively, so a reader that shouts
// `TAROT:PIXIE-FEE` gets past the prefix check and would then miss here on
// the one difference nobody would ever debug from a dropped-fact counter.
//
// Folded with the same rule Python's `casefold()` uses on both sides. Every
// committed id is ASCII, so the two agree today; matching the function
// rather than the corpus is what keeps that true of an id somebody adds.
func TarotFactByID(id string) *TarotFact {
	wanted := tarotFold(id)
	for i := range tarot.Facts {
		if tarotFold(tarot.Facts[i].ID) == wanted {
			return &tarot.Facts[i]
		}
	}
	return nil
}

// tarotFold is `s.strip().casefold()` for an id. Ids are kebab-case ASCII, so
// `ToLower` is the whole of `casefold` over the set that can occur -- and the
// strip is Python's, which counts four more characters as space than Go does.
func tarotFold(id string) string {
	return strings.ToLower(strings.TrimFunc(id, tarotIsSpace))
}

func tarotIsSpace(r rune) bool {
	// `str.isspace()`: Unicode whitespace plus the four information
	// separators, which `unicode.IsSpace` leaves out.
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// TarotFactsForCard is everything known about one card, whichever tier it
// belongs to.
func TarotFactsForCard(key string) []TarotFact {
	out := []TarotFact{}
	for _, f := range tarot.Facts {
		if f.Card != "" && f.Card == key {
			out = append(out, f)
		}
	}
	return out
}

// TarotFactsForReading is what may be told at a table where these cards are
// face up.
//
// **The deck tier first and always**: it is true of every spread, and it is
// why a reading of three minors is not a reading with nothing to say. Then
// the cards, in the order they were dealt, so a reader skimming the list
// meets the table's own cards in the table's own order.
//
// Duplicates are impossible (a card cannot be dealt twice) but the seen set
// keeps it that way if the sampler ever changes.
func TarotFactsForReading(keys []string) []TarotFact {
	out := []TarotFact{}
	seen := map[string]bool{}
	for _, f := range tarot.Facts {
		if f.Card == "" && !seen[f.ID] {
			seen[f.ID] = true
			out = append(out, f)
		}
	}
	for _, key := range keys {
		for _, f := range TarotFactsForCard(key) {
			if !seen[f.ID] {
				seen[f.ID] = true
				out = append(out, f)
			}
		}
	}
	return out
}

// TarotOffer is the facts, as the reader's frame message carries them.
//
// Prose with ids rather than JSON, for the reason `Reading.Describe` gives:
// this is read by a model being asked to sound like a person, and a data
// structure invites a data-structure answer.
//
// `told` drops what this querent has already heard -- the same list
// `theme.Repeats` checks against, applied one step earlier so the reader is
// not tempted by a fact it cannot use. Belt and braces on purpose: the prompt
// asks, this narrows what is asked about, and `KeepFact` still checks.
// Returns an empty string when there is nothing left to offer, and the caller
// omits the whole section rather than printing a heading over nothing.
func TarotOffer(keys []string, told []string) string {
	seen := map[string]bool{}
	for _, t := range told {
		seen[strings.TrimFunc(t, tarotIsSpace)] = true
	}
	lines := []string{}
	for _, f := range TarotFactsForReading(keys) {
		if seen[strings.TrimFunc(f.Text, tarotIsSpace)] {
			continue
		}
		lines = append(lines, "- "+f.ID+": "+f.Text)
	}
	if len(lines) == 0 {
		return ""
	}
	return "\n\nTrue things you know about this deck and these cards. To " +
		"tell one, put its id in the fact's `source` field as " +
		"`tarot:<id>` — the exact words below are what they will read, " +
		"so choose the one that belongs and let your question carry the " +
		"connection. Never retell one, and never write your own.\n" +
		strings.Join(lines, "\n")
}
