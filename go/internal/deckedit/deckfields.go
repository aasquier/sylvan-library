package deckedit

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// The value rules for the deck's own fields. Each one is a decision rather
// than a default, and each is the reason its field is in SettableDeckFields
// while `commander` and `notes` are not.

// emptyGraveyard matches a hand-written `graveyard: []`, which cannot carry
// block items beneath it and has to be reopened before the first burial.
var emptyGraveyard = regexp.MustCompile(`^` + Graveyard + `:\s*\[\s*\]`)

// Archetypes is `model.ARCHETYPES` in its piloted order, easiest to hardest.
// Read through the generated vocabulary rather than copied, for the reason
// `GET /api/themes` exists: a second copy in a second language drifts silently
// and nobody learns which one was right.
var Archetypes = reference.Themes().Archetypes

// deckFieldValue is the per-field validation `set_deck_field` does before it
// touches a line. It returns the value as it will be written -- a string, an
// int, or the deduplicated theme list.
func deckFieldValue(field string, value any) (any, error) {
	switch field {
	case "strategy":
		// The one prose field the deck's own keys carry. Trimmed and required
		// non-empty for the same reason a note is: an empty `strategy:` puts a
		// blank paragraph at the top of the generated primer, and the way to
		// have no strategy is to not have the key.
		text := strings.TrimSpace(asString(value))
		if text == "" {
			return nil, failf("a strategy needs text; remove the line instead of blanking it")
		}
		return text, nil
	case "bracket":
		bracket, err := asInt(value)
		if err != nil {
			return nil, failf("bracket must be a number, not %s", quotedValue(value))
		}
		if bracket < 1 || bracket > 5 {
			return nil, failf("bracket runs from 1 to 5")
		}
		return bracket, nil

	case "name":
		// What the deck is called, and the one settable field that is read
		// before anything else about the deck: the shelf card, the page's
		// heading, the printed primer's title.
		//
		// Blank is refused, and that is the whole difference between this
		// field and `pilot`. Emptying a pilot means "nobody claims this one",
		// which is a real thing to say; emptying a name says nothing, because
		// `Deck.from_text` falls back to the slug and the shelf would go on
		// showing a name nobody chose as though somebody had. That is this
		// repository's most-repeated bug -- a fallback rendered as a fact --
		// so the refusal happens here rather than the fallback happening
		// later.
		//
		// Whitespace is collapsed rather than refused. A name arrives pasted
		// as often as typed, and a doubled space or a trailing tab is a
		// slip rather than a decision -- where a name of the wrong *length*
		// is a decision, and gets an answer instead of a correction.
		name := strings.Join(strings.Fields(asString(value)), " ")
		if name == "" {
			return nil, failf("a deck needs a name -- it is what the shelf and " +
				"the top of the deck page call it. Rename it to something rather " +
				"than to nothing.")
		}
		if runes := []rune(name); len(runes) > DeckNameMax {
			return nil, failf("a deck name runs at most %d characters; %s… is %d. "+
				"What the deck is *doing* belongs in its description, which has "+
				"room for it.", DeckNameMax,
				quotedValue(string(runes[:min(24, len(runes))])), len(runes))
		}
		return name, nil

	case "pilot":
		// A person's name: free text, case kept, and emptying it is a real
		// operation -- it means "nobody claims this one". The cap is about the
		// file and the shelf, not about people; nobody signs a deck with a
		// paragraph.
		pilot := strings.TrimSpace(asString(value))
		if runes := []rune(pilot); len(runes) > PilotMax {
			return nil, failf("a pilot name runs at most %d characters; %s… is %d",
				PilotMax, quotedValue(string(runes[:min(20, len(runes))])), len(runes))
		}
		return pilot, nil

	case "commander_art":
		// A Scryfall printing id, so free text with no enum to check it
		// against. Case is preserved rather than lowered: the value is an
		// opaque identifier belonging to somebody else's system, and this is
		// the one settable field where lowering it would corrupt the value.
		// Emptying it is a real operation -- it means "back to the default
		// printing" -- so a blank is allowed through rather than refused.
		art := strings.TrimSpace(asString(value))
		if art != "" && !printingID.MatchString(art) {
			return nil, failf("%s is not a Scryfall printing id. It should look "+
				"like a UUID; the deck page's art picker sets this for you, and "+
				"`mtglab decks set <slug> --art <set-code>` takes a set code and "+
				"looks the id up.", quotedValue(art))
		}
		return art, nil

	case "themes":
		// The open identity list. A comma-separated string or a list both
		// arrive here (the CLI sends the former, JSON the latter); either way
		// every entry must be in the vocabulary, which is how a typo is a
		// refusal instead of a theme nobody else's filter will ever match.
		// Order is kept and duplicates are dropped -- the first mention wins.
		var themes []string
		for _, item := range asList(value) {
			theme := strings.ToLower(strings.TrimSpace(item))
			if theme == "" {
				continue
			}
			if !reference.IsTheme(theme) {
				return nil, failf("%s is not in the theme vocabulary; the list "+
					"is a curated shelf, and a typo would file this deck where "+
					"no filter looks. Known themes: %s", quotedValue(theme),
					strings.Join(reference.Themes().Themes, ", "))
			}
			if !slices.Contains(themes, theme) {
				themes = append(themes, theme)
			}
		}
		if themes == nil {
			themes = []string{}
		}
		return themes, nil

	default:
		word := strings.ToLower(strings.TrimSpace(asString(value)))
		allowed := reference.Deck().DeckStages
		if field == "status" {
			allowed = reference.Deck().DeckStatuses
		}
		if !slices.Contains(allowed, word) {
			return nil, failf("%s must be one of %s, not %s",
				field, strings.Join(allowed, ", "), quotedValue(word))
		}
		return word, nil
	}
}

// refusePromotion is the check that makes promoting a draft honest.
func refusePromotion(doc map[string]any) error {
	cards := listOf(doc, "cards")
	if len(cards) == 0 {
		// Vacuously true is not true enough. "Every card is justified" is
		// trivially satisfied by a deck with no cards, so the blank-`why`
		// check below passes and an empty deck promotes itself to curated -- a
		// claim that the thinking is done about a deck that does not exist
		// yet.
		return failf("this deck has no cards yet, so there is nothing to have " +
			"justified; add the 99 before promoting it to curated")
	}
	var blank []string
	for _, item := range cards {
		card, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(asString(card["why"])) != "" {
			continue
		}
		name := asString(card["name"])
		if name == "" {
			name = "?"
		}
		blank = append(blank, name)
	}
	if len(blank) == 0 {
		return nil
	}
	shown := strings.Join(blank[:min(6, len(blank))], ", ")
	more := ""
	if len(blank) > 6 {
		more = fmt.Sprintf(", and %d more", len(blank)-6)
	}
	return failf("%d card(s) still have no `why` (%s%s); a curated deck "+
		"justifies every slot, so write them before promoting -- this refuses "+
		"rather than writing a deck you would have to un-promote",
		len(blank), shown, more)
}

// namesAClass reports whether a theme list declares one of the four words the
// rating boards group by -- the condition under which a themes edit shadows,
// and therefore removes, a pre-ADR-37 `archetype:` key.
func namesAClass(themes []string) bool {
	for _, theme := range themes {
		if reference.ArchetypeIndex(theme) >= 0 {
			return true
		}
	}
	return false
}

// asList reads the two shapes a theme list arrives in: a comma-separated
// string from the CLI, a list from JSON.
func asList(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return strings.Split(v, ",")
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, asString(item))
		}
		return out
	default:
		return []string{asString(value)}
	}
}
