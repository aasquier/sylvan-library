package deckedit

import (
	"slices"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/yamlemit"
)

// The operations -- ten since ADR 41 added `DraftRationale`, and the number
// is written here only because the next reader will count them anyway. Each
// takes the file's text and hands back the text it
// would become -- never a file handle, which is what makes "a refused edit has
// changed nothing" a fact about the signature rather than a promise.

// ReplaceCard replaces one card entry's name and rationale, in place.
//
// `category` defaults to whatever the outgoing card was filed under, since a
// replacement usually fills the same role -- pass a non-nil pointer to move
// it.
//
// Returns a *Failed rather than a damaged file. Every other card, every note,
// every comment and every blank line survives untouched.
func ReplaceCard(text, oldName, newName, why string, category *string) (string, error) {
	if strings.TrimSpace(why) == "" {
		// Rule 4. A card that cannot justify its slot is a card to cut, and a
		// rationale invented by a machine is exactly the empty justification
		// that rule exists to prevent. Required even in a draft: a draft owes
		// rationales it has not written, which is not the same as a card whose
		// slot was actively reconsidered and still cannot be argued for.
		return "", failf("a replacement needs a `why`; refusing to invent one")
	}

	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	listKey, position, e, err := locateCard(doc, lines, oldName, CardLists)
	if err != nil {
		return "", err
	}

	changes := map[string]change{"name": set(newName), "why": set(why)}
	order := []string{"name", "why"}
	if category != nil {
		changes["category"] = set(*category)
		order = append(order, "category")
	}
	// `mana_cost` and `scryfall_id` describe the card that is leaving.
	// Carrying them over would attach one card's identity to another.
	changes["mana_cost"] = drop()
	changes["scryfall_id"] = drop()

	rebuilt, err := rewriteEntry(lines, e, changes, order)
	if err != nil {
		return "", err
	}
	updated := joinAround(lines, e.start, e.end, rebuilt)

	expected := copyDoc(doc)
	item := cardAt(listOf(expected, listKey), position)
	item["name"] = newName
	item["why"] = why
	if category != nil {
		item["category"] = *category
	}
	delete(item, "mana_cost")
	delete(item, "scryfall_id")
	return verified(updated, expected)
}

// AddCard adds a card to the 99 or to the swap board.
//
// Inserted next to the cards it belongs with -- after the last entry already
// in its category -- rather than at the end of the list. The deck files are
// grouped by category under section banners, and appending a land to the
// bottom of the file would file it under whatever the last banner happened to
// say. Falls back to the end of the list when the category is new.
//
// The rationale is required unless the deck is a draft, where a blank `why` is
// the counted work the deck still owes (ADR 13). It is never generated: an
// empty `why` on a curated deck is refused, not filled in.
func AddCard(text, name, category, why string, qty int, listKey string) (string, error) {
	if !slices.Contains(CardLists, listKey) {
		return "", failf("cards live in %s, not %s",
			strings.Join(CardLists, " or "), quotedValue(listKey))
	}
	name = strings.TrimSpace(name)
	category = strings.TrimSpace(category)
	switch {
	case name == "":
		return "", failf("a card needs a name")
	case category == "":
		return "", failf("a card needs a category")
	case qty < 1:
		return "", failf("quantity must be at least 1")
	}

	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	if requiresRationale(doc) && strings.TrimSpace(why) == "" {
		// Rule 4, at the point where a card enters the deck. See ADR 12 rule 3.
		return "", failf("a card in a curated deck needs a `why`; refusing to invent one")
	}

	for _, key := range append(slices.Clone(CardLists), Graveyard) {
		for _, item := range listOf(doc, key) {
			card, ok := item.(map[string]any)
			if !ok || !nameMatches(card, name) {
				continue
			}
			if key == Graveyard {
				// A second entry alongside a graveyard copy would leave the
				// deck showing one card in two places with two rationales. The
				// graveyard copy carries the user's own `why` already, which
				// is exactly what a return restores.
				return "", failf("%s is in the graveyard; return it or exile it "+
					"instead of adding a second entry", quotedValue(name))
			}
			return "", failf("%s is already in %s; change its quantity or "+
				"rationale instead of adding a second entry", quotedValue(name), whereIs(key))
		}
	}
	if err := refuseTheCommander(doc, name); err != nil {
		return "", err
	}

	start, stop, err := blockSpan(lines, listKey)
	if err != nil {
		return "", err
	}
	spans := entrySpans(lines, start, stop)
	items := listOf(doc, listKey)
	if len(spans) != len(items) {
		return "", countMismatch(listKey, len(items), len(spans))
	}

	var position, at int
	var shape entry
	if len(spans) == 0 {
		// An empty list is written `swap_board: []`, which cannot carry block
		// items beneath it. Reopen the block before filling it.
		header, err := blockHeader(lines, listKey)
		if err != nil {
			return "", err
		}
		lines = replaceLine(lines, header, listKey+":")
		position, at = 0, header+1
		shape = entry{start: at, end: at, dashIndent: 2, keyIndent: 4}
	} else {
		anchor := len(spans) - 1
		for i, item := range items {
			card, ok := item.(map[string]any)
			if ok && strings.EqualFold(strings.TrimSpace(asString(card["category"])), category) {
				anchor = i
			}
		}
		shape = spans[anchor]
		content, _ := splitTail(lines[shape.start:shape.end], shape.keyIndent)
		position, at = anchor+1, shape.start+len(content)
	}

	rendered, err := cardLines(shape, name, category, strings.TrimSpace(why), qty)
	if err != nil {
		return "", err
	}
	updated := joinAround(lines, at, at, rendered)

	added := map[string]any{"name": name, "category": category}
	if qty != 1 {
		added["qty"] = int64(qty)
	}
	added["why"] = strings.TrimSpace(why)
	expected := copyDoc(doc)
	expected[listKey] = insertAt(listOf(expected, listKey), position, added)
	return verified(updated, expected)
}

// RemoveCard takes a card out of the 99 or the swap board, and nothing else
// out.
//
// Section banners and the blank lines around them belong to the cards below
// them, not to the card being removed, so they stay. The blank line *after* an
// entry goes with it, which is what keeps the spacing even.
func RemoveCard(text, name string) (string, error) {
	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	listKey, position, e, err := locateCard(doc, lines, name, CardLists)
	if err != nil {
		return "", err
	}

	_, cut := entryCut(lines, e, position, len(listOf(doc, listKey)), true)

	expected := copyDoc(doc)
	expected[listKey] = removeAt(listOf(expected, listKey), position)

	if len(listOf(expected, listKey)) == 0 {
		// A block key with nothing under it parses to None, not to an empty
		// list, and `Deck.from_text` would iterate it. Say `[]` explicitly.
		header, err := blockHeader(lines, listKey)
		if err != nil {
			return "", err
		}
		lines = replaceLine(lines, header, listKey+": []")
	}

	updated := strings.Join(append(slices.Clone(lines[:e.start]), lines[cut:]...), "\n")
	return verified(updated, expected)
}

// EntombCard moves a card from the 99 to the graveyard, keeping everything it
// carries.
//
// The delete with an undo (ADR 27). The entry's lines move verbatim --
// category, quantity, overrides and above all the `why`, which is the user's
// own text and the thing a later ReturnCard restores without anything being
// re-invented. Newest entombment first, so the graveyard reads as a history.
//
// Only the 99 has a graveyard. A swap-board card is already outside the deck
// with the reason it did not make it, and burying that reason would say less
// than the board does.
func EntombCard(text, name string) (string, error) {
	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	listKey, position, e, err := locateCard(doc, lines, name, CardLists)
	if err != nil {
		return "", err
	}
	if listKey != "cards" {
		return "", failf("%s is on the swap board, which has no graveyard; "+
			"remove it instead", quotedValue(name))
	}

	content, cut := entryCut(lines, e, position, len(listOf(doc, "cards")), true)

	moved := cardAt(listOf(doc, "cards"), position)
	expected := copyDoc(doc)
	expected["cards"] = removeAt(listOf(expected, "cards"), position)
	expected[Graveyard] = append([]any{deepCopy(moved)}, listOf(expected, Graveyard)...)

	remaining := append(slices.Clone(lines[:e.start]), lines[cut:]...)
	if len(listOf(expected, "cards")) == 0 {
		header, err := blockHeader(remaining, "cards")
		if err != nil {
			return "", err
		}
		remaining = replaceLine(remaining, header, "cards: []")
	}

	header, err := blockHeader(remaining, Graveyard)
	if err != nil {
		// First burial: the block goes at the end of the file, where
		// `Deck.dump` and the key order both put it, after any trailing blank
		// lines so the key sits against the content above it.
		end := len(remaining)
		for end > 0 && strings.TrimSpace(remaining[end-1]) == "" {
			end--
		}
		updated := slices.Concat(remaining[:end], []string{Graveyard + ":"}, content, remaining[end:])
		return verified(strings.Join(updated, "\n"), expected)
	}

	if emptyGraveyard.MatchString(remaining[header]) {
		// A hand-written `graveyard: []` cannot carry block items beneath it.
		remaining = replaceLine(remaining, header, Graveyard+":")
	}
	updated := slices.Concat(remaining[:header+1], content, remaining[header+1:])
	return verified(strings.Join(updated, "\n"), expected)
}

// ReturnCard moves a card from the graveyard back into the 99.
//
// The undo half of EntombCard. The entry returns exactly as it left -- the
// `why` is the user's own words, so restoring it invents nothing and rule 4 is
// untouched -- and it is filed next to the cards it belongs with, the same
// category-anchored placement AddCard uses.
func ReturnCard(text, name string) (string, error) {
	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	_, position, e, err := locateCard(doc, lines, name, []string{Graveyard})
	if err != nil {
		return "", failf("%s is not in the graveyard", quotedValue(name))
	}

	moved := cardAt(listOf(doc, Graveyard), position)
	category := asString(moved["category"])
	if strings.TrimSpace(category) == "" {
		category = "utility"
	}
	movedName := asString(moved["name"])

	for _, key := range CardLists {
		for _, item := range listOf(doc, key) {
			card, ok := item.(map[string]any)
			if ok && nameMatches(card, movedName) {
				return "", failf("%s is already in %s; exile the graveyard copy "+
					"instead of returning it", quotedValue(movedName), whereIs(key))
			}
		}
	}
	if err := refuseTheCommander(doc, movedName); err != nil {
		return "", err
	}

	content, cut := entryCut(lines, e, position, len(listOf(doc, Graveyard)), false)

	expected := copyDoc(doc)
	expected[Graveyard] = removeAt(listOf(expected, Graveyard), position)
	remaining := append(slices.Clone(lines[:e.start]), lines[cut:]...)
	if len(listOf(expected, Graveyard)) == 0 {
		// An emptied graveyard is removed rather than left as `graveyard: []`:
		// absent is the field's normal state, and six curated decks should not
		// keep an empty coffin at the bottom of the file.
		delete(expected, Graveyard)
		remaining, err = dropBlockHeader(remaining, Graveyard)
		if err != nil {
			return "", err
		}
	}

	start, stop, err := blockSpan(remaining, "cards")
	if err != nil {
		return "", err
	}
	spans := entrySpans(remaining, start, stop)
	items := listOf(expected, "cards")
	if len(spans) != len(items) {
		return "", countMismatch("cards", len(items), len(spans))
	}

	var insertAtIndex, at int
	if len(spans) == 0 {
		header, err := blockHeader(remaining, "cards")
		if err != nil {
			return "", err
		}
		remaining = replaceLine(remaining, header, "cards:")
		insertAtIndex, at = 0, header+1
	} else {
		anchor := len(spans) - 1
		for i, item := range items {
			card, ok := item.(map[string]any)
			if ok && strings.EqualFold(strings.TrimSpace(asString(card["category"])), category) {
				anchor = i
			}
		}
		shape := spans[anchor]
		body, _ := splitTail(remaining[shape.start:shape.end], shape.keyIndent)
		insertAtIndex, at = anchor+1, shape.start+len(body)
	}

	expected["cards"] = insertAt(items, insertAtIndex, deepCopy(moved))
	updated := slices.Concat(remaining[:at], content, remaining[at:])
	return verified(strings.Join(updated, "\n"), expected)
}

// ExileCard removes a card from the graveyard for good.
//
// The one genuinely permanent delete, and it only ever acts on a card that was
// already entombed -- so reaching it takes two deliberate steps, which is the
// confirmation structure ADR 27 wants rather than a dialog asking "are you
// sure" about a click that already happened.
func ExileCard(text, name string) (string, error) {
	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	_, position, e, err := locateCard(doc, lines, name, []string{Graveyard})
	if err != nil {
		return "", failf("%s is not in the graveyard", quotedValue(name))
	}

	_, cut := entryCut(lines, e, position, len(listOf(doc, Graveyard)), false)

	expected := copyDoc(doc)
	expected[Graveyard] = removeAt(listOf(expected, Graveyard), position)
	remaining := append(slices.Clone(lines[:e.start]), lines[cut:]...)
	if len(listOf(expected, Graveyard)) == 0 {
		delete(expected, Graveyard)
		remaining, err = dropBlockHeader(remaining, Graveyard)
		if err != nil {
			return "", err
		}
	}
	return verified(strings.Join(remaining, "\n"), expected)
}

// SetCardField changes one field of one card: category, quantity, `why`, or
// art.
//
// This is the write path behind the rationale editor, and the one place a
// `why` can be filled in without replacing the card. The value comes from the
// caller and is written as given -- nothing here composes, templates, tidies
// or infers one (ADR 12 rule 3).
func SetCardField(text, name, field string, value any) (string, error) {
	if !slices.Contains(SettableFields, field) {
		return "", failf("%s is not settable; choose one of %s",
			quotedValue(field), strings.Join(SettableFields, ", "))
	}

	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	listKey, position, e, err := locateCard(doc, lines, name, CardLists)
	if err != nil {
		return "", err
	}

	var written any
	switch field {
	case "qty":
		qty, err := asInt(value)
		if err != nil {
			return "", failf("quantity must be a whole number, not %s", quotedValue(value))
		}
		if qty < 1 {
			return "", failf("quantity must be at least 1; remove the card instead")
		}
		written = qty
	case "art":
		// The card's own `commander_art`, with the same contract: a printing
		// id checked by shape here and against the pool one layer up, case
		// preserved, and a blank meaning "back to the default printing" --
		// which drops the key rather than writing `art: ''` into a file where
		// absence already says that.
		art := strings.TrimSpace(asString(value))
		if art != "" && !printingID.MatchString(art) {
			return "", failf("%s is not a Scryfall printing id. It should look "+
				"like a UUID; the deck page's art picker sets this for you.", quotedValue(art))
		}
		written = art
	default:
		text := asString(value)
		if field == "category" && strings.TrimSpace(text) == "" {
			return "", failf("a card needs a category")
		}
		if field == "why" {
			text = strings.TrimSpace(text)
			if text == "" && requiresRationale(doc) {
				return "", failf("a card in a curated deck needs a `why`; refusing to blank it")
			}
		}
		written = text
	}

	c := set(written)
	if field == "art" && written == "" {
		c = drop()
	}
	changes := map[string]change{field: c}
	order := []string{field}
	// **Writing a rationale un-marks it** (ADR 41). `why_by: claude` says a
	// model drafted this sentence; the moment a person writes over it the
	// sentence is theirs, and a mark left behind would be a lie in the one
	// file that is supposed to be the truth. Blanking counts too: what is
	// gone was not drafted by anybody.
	//
	// Here rather than in the intake because this is the single door every
	// `why` goes through -- the deck page, the CLI, an edit from a swap --
	// and a rule enforced at one of those and not the others is a rule that
	// holds until the second caller.
	if field == "why" {
		changes["why_by"] = drop()
		order = append(order, "why_by")
	}
	rebuilt, err := rewriteEntry(lines, e, changes, order)
	if err != nil {
		return "", err
	}
	updated := joinAround(lines, e.start, e.end, rebuilt)

	expected := copyDoc(doc)
	item := cardAt(listOf(expected, listKey), position)
	if c.drop {
		delete(item, field)
	} else if qty, ok := written.(int); ok {
		item[field] = int64(qty)
	} else {
		item[field] = written
	}
	if field == "why" {
		delete(item, "why_by")
	}
	return verified(updated, expected)
}

// DraftRationale writes a rationale a model drafted, and marks it as one.
//
// The tenth operation, and the only one that exists because of [ADR 41]. It is
// deliberately NOT `SetCardField(field="why")` with an extra argument, for
// three reasons that all point the same way:
//
//   - **It is greppable.** Every write of a drafted sentence in this repo is a
//     call to this function, so "what can put a model's words in a deck" has a
//     one-line answer that a reader can check rather than a set of call sites
//     that have to be traced.
//   - **`boundary_test.go` names the write surface**, and this is on it. The
//     Claude tree cannot call this any more than it can call SetCardField --
//     which is the point of ADR 41 landing in the caller rather than the mode.
//   - **It refuses what SetCardField allows.** A drafted rationale never
//     overwrites a rationale that is already there, and never writes an empty
//     one. Both are below.
//
// Refusing to overwrite is the load-bearing half. A person's sentence and a
// draft are indistinguishable once written, so the only moment this can be got
// right is before the write -- and an intake that ran twice, or ran over the
// quoted column PR #391 introduced, would otherwise replace somebody's own
// words with a model's and mark the result as drafted, which loses the words
// and tells the truth about the wrong sentence.
func DraftRationale(text, name, why string) (string, error) {
	why = strings.TrimSpace(why)
	if why == "" {
		return "", failf("refusing to write an empty rationale for %s", quotedValue(name))
	}

	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	listKey, position, e, err := locateCard(doc, lines, name, CardLists)
	if err != nil {
		return "", err
	}

	existing := cardAt(listOf(doc, listKey), position)
	if held, ok := existing["why"].(string); ok && strings.TrimSpace(held) != "" {
		return "", failf("%s already has a rationale; a draft never writes over one",
			quotedValue(name))
	}

	changes := map[string]change{"why": set(why), "why_by": set(DraftedBy)}
	order := []string{"why", "why_by"}
	rebuilt, err := rewriteEntry(lines, e, changes, order)
	if err != nil {
		return "", err
	}
	updated := joinAround(lines, e.start, e.end, rebuilt)

	expected := copyDoc(doc)
	item := cardAt(listOf(expected, listKey), position)
	item["why"] = why
	item["why_by"] = DraftedBy
	return verified(updated, expected)
}

// SetDeckField changes one of the deck's own fields: stage, status, a label,
// and kin.
//
// The one that matters is `stage`, because promoting a draft to curated is the
// last step of an import and was, until this operation existed, the last thing
// in the whole lifecycle that could only be done in a text editor.
//
// **Promotion is refused while any card is blank.** The gate would catch it
// either way -- a curated deck reports one `missing-rationale` error per card
// -- but catching it here means the deck is never written into a state its
// author has to undo. That is the same shape as refusing a swap with no
// rationale rather than writing one and failing it afterwards.
//
// A trailing comment on the line survives: `status: built  # the cards are
// sleeved up` is the author's note about the vocabulary, not about the value,
// and ADR 12 rule 1 says an edit touches only what it changes.
func SetDeckField(text, field string, value any) (string, error) {
	if field == "archetype" {
		// Named specially because it *was* settable before ADR 37, so the
		// refusal has to say where the label went, not just what exists.
		return "", failf("the archetype is a reading of the themes now (ADR 37): "+
			"declare a strategy word there instead -- %s -- and the "+
			"worst-piloted one declared becomes the deck's board",
			strings.Join(Archetypes, ", "))
	}
	if !slices.Contains(SettableDeckFields, field) {
		return "", failf("%s is not a settable deck field; choose one of %s",
			quotedValue(field), strings.Join(SettableDeckFields, ", "))
	}

	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}

	written, err := deckFieldValue(field, value)
	if err != nil {
		return "", err
	}

	if field == "stage" && written == "curated" {
		if err := refusePromotion(doc); err != nil {
			return "", err
		}
	}

	var updated string
	start, end, found := topLevelSpan(lines, field)
	if !found {
		// The key is absent -- `stage` is, in every deck written before ADR
		// 13. Place it where `Deck.dump` would rather than at the end of the
		// file.
		at := 0
		for _, key := range deckKeyOrder {
			if key == field {
				break
			}
			s, e, ok := topLevelSpan(lines, key)
			if !ok {
				continue
			}
			content, _ := splitTail(lines[s:e], 0)
			at = max(at, s+len(content))
		}
		rendered, err := render(field, written, 0)
		if err != nil {
			return "", err
		}
		updated = joinAround(lines, at, at, rendered)
	} else {
		_, tail := splitTail(lines[start:end], 0)
		rendered, err := render(field, written, 0)
		if err != nil {
			return "", err
		}
		if match := scalarLine.FindStringSubmatch(lines[start]); match != nil &&
			match[3] != "" && len(rendered) == 1 {
			rendered = []string{rendered[0] + "  " + match[3]}
		}
		updated = joinAround(lines, start, end, append(rendered, tail...))
	}

	expected := copyDoc(doc)
	expected[field] = documentValue(written)

	if themes, ok := written.([]string); ok && field == "themes" && namesAClass(themes) {
		// The edit that shadows a pre-ADR-37 `archetype:` key is the edit that
		// removes it. Deck writes are surgical (ADR 12) -- nothing on the
		// app's write path ever calls `Deck.dump` on an existing file -- so
		// `dump`'s own once-shadowed-dropped rule would never fire here and
		// the dead key would outlive the migration it warns about. A themes
		// edit that declares no class word leaves the key alone: it is still
		// the deck's only board, load-bearing until shadowed.
		shadowed := strings.Split(updated, "\n")
		if s, e, ok := topLevelSpan(shadowed, "archetype"); ok {
			_, tail := splitTail(shadowed[s:e], 0)
			updated = joinAround(shadowed, s, e, tail)
			delete(expected, "archetype")
		}
	}

	return verified(updated, expected)
}

// SetShared puts the deck on display to other accounts, or takes it off
// (ADR 22).
//
// The tenth operation, and it exists because the ninth's comment above made a
// claim that was not true: *nothing on the app's write path ever calls
// `Deck.dump` on an existing file*. The share toggle's original write did,
// and it was the only thing that did. A load-and-dump round trip rewrites
// the whole file -- so one press of the deck page's share toggle took a
// hand-written curated deck's section banners, its trailing comments, its
// folded blocks and its `swap_board: []` with it, and since ADR 30 there is
// no revision to get them back from. Found by holding the write to the
// recorded bytes, which meant asking what the bytes were; ruled by Aaron
// 2026-08-22, and this surgical operation is the fix.
//
// Deliberately not a SettableDeckFields entry. `shared` is the one deck fact
// the two tiers keep in different places -- `deck.yaml` here, a column in the
// SQL tier -- which is why it has its own route rather than riding the PATCH
// beside it, and putting it in that list would publish it on that route as a
// side effect.
//
// **True removes the key rather than writing it.** Absent already means
// shared (`Deck.shared` defaults to true and `dump` omits it), so writing
// `shared: true` would grow every curated file a line asserting the default
// it already had -- the same self-cleaning round trip `commander_art` and the
// shadowed `archetype:` use.
func SetShared(text string, shared bool) (string, error) {
	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	start, end, found := topLevelSpan(lines, "shared")
	expected := copyDoc(doc)

	if shared {
		delete(expected, "shared")
		if !found {
			return verified(text, expected)
		}
		_, tail := splitTail(lines[start:end], 0)
		return verified(joinAround(lines, start, end, tail), expected)
	}

	expected["shared"] = false
	rendered, err := render("shared", false, 0)
	if err != nil {
		return "", err
	}
	if !found {
		// Placed where `Deck.dump` would put it -- after the commander,
		// before the pilot -- rather than at the end of the file, which is
		// the rule SetDeckField follows for an absent `stage:`.
		at := 0
		for _, key := range deckKeyOrder {
			if key == "shared" {
				break
			}
			s, e, ok := topLevelSpan(lines, key)
			if !ok {
				continue
			}
			content, _ := splitTail(lines[s:e], 0)
			at = max(at, s+len(content))
		}
		return verified(joinAround(lines, at, at, rendered), expected)
	}
	_, tail := splitTail(lines[start:end], 0)
	if match := scalarLine.FindStringSubmatch(lines[start]); match != nil && match[3] != "" {
		rendered = []string{rendered[0] + "  " + match[3]}
	}
	return verified(joinAround(lines, start, end, append(rendered, tail...)), expected)
}

// SetNote sets one deck-level note, the prose the advanced primer reads
// directly.
//
// Notes are the deck's thinking -- the mulligan rule, the pitfalls, the lines
// -- and they survive regeneration because they live in the source of truth
// rather than in an artifact. Creates the `notes:` block if the deck has none,
// placing it where `Deck.dump` would: after the strategy, before the cards.
func SetNote(text, key, value string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", failf("a note needs a key")
	}
	if !keyLine.MatchString(key + ":") {
		return "", failf("%s is not a usable note key; use letters, digits and underscores", quotedValue(key))
	}
	if strings.TrimSpace(value) == "" {
		// Symmetrical with a card's `why`: an empty note is not a note, and
		// writing one would put a blank heading in the advanced primer.
		return "", failf("a note needs text")
	}
	value = strings.TrimSpace(value)

	doc, lines, err := open(text)
	if err != nil {
		return "", err
	}
	notes, hasNotes := doc["notes"]
	notesMap, isMap := notes.(map[string]any)
	if hasNotes && notes != nil && !isMap {
		return "", failf("`notes:` is not a mapping in this deck file")
	}

	// Whether the block exists is a question about the text, not the parse: a
	// `notes:` header with nothing under it parses to None, and answering from
	// the parse would write the deck a second `notes:` block.
	spanStart, spanEnd, err := blockSpan(lines, "notes")
	hasBlock := err == nil

	var updated string
	if !hasBlock {
		// `cards:` is the anchor because every deck has one, and it is what
		// notes sit above in the order `Deck.dump` writes.
		anchor := len(lines)
		if start, _, err := blockSpan(lines, "cards"); err == nil {
			anchor = start - 1
		}
		for anchor > 0 && strings.TrimSpace(lines[anchor-1]) == "" {
			anchor--
		}
		body, err := yamlemit.Render(key, value, 2, yamlemit.ProseWidth, true)
		if err != nil {
			return "", err
		}
		updated = joinAround(lines, anchor, anchor, append([]string{"", "notes:"}, body...))
	} else {
		indent := 2
		for i := spanStart; i < spanEnd; i++ {
			if strings.TrimSpace(lines[i]) != "" {
				indent = indentOf(lines[i])
				break
			}
		}
		start, end := -1, -1
		for i := spanStart; i < spanEnd; i++ {
			match := keyLine.FindStringSubmatch(lines[i])
			if match == nil || len(match[1]) != indent || match[2] != key {
				continue
			}
			start, end = i, spanEnd
			for j := i + 1; j < spanEnd; j++ {
				if strings.TrimSpace(lines[j]) != "" && indentOf(lines[j]) <= indent {
					end = j
					break
				}
			}
			break
		}
		rendered, err := yamlemit.Render(key, value, indent, yamlemit.ProseWidth, true)
		if err != nil {
			return "", err
		}
		if start < 0 {
			body, _ := splitTail(lines[spanStart:spanEnd], indent)
			at := spanStart + len(body)
			updated = joinAround(lines, at, at, rendered)
		} else {
			_, tail := splitTail(lines[start:end], indent)
			updated = joinAround(lines, start, end, append(rendered, tail...))
		}
	}

	expected := copyDoc(doc)
	merged := map[string]any{}
	for k, v := range notesMap {
		merged[k] = v
	}
	merged[key] = value
	expected["notes"] = merged
	return verified(updated, expected)
}

// ----------------------------------------------------------------- helpers

// entryCut finds an entry's own lines and the index the removal stops at.
//
// An entry owns the blank line after it, so taking the card takes the gap and
// the spacing stays even -- unless the gap leads to a section banner, which
// owns the blank above it: removing the last land must not weld
// `# ---- RAMP 14` onto the land before it. The graveyard has no banners, so
// its operations pass `respectBanners=false` and skip that question.
func entryCut(lines []string, e entry, position, total int, respectBanners bool) (content []string, cut int) {
	content, tail := splitTail(lines[e.start:e.end], e.keyIndent)
	cut = e.start + len(content)
	if position >= total-1 {
		return content, cut
	}
	if respectBanners {
		for _, line := range tail {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				return content, cut
			}
		}
	}
	for cut < e.end && strings.TrimSpace(lines[cut]) == "" {
		cut++
	}
	return content, cut
}

// dropBlockHeader removes an emptied block's own line, and the blank line that
// led to it.
func dropBlockHeader(lines []string, key string) ([]string, error) {
	header, err := blockHeader(lines, key)
	if err != nil {
		return nil, err
	}
	start := header
	for start > 0 && strings.TrimSpace(lines[start-1]) == "" {
		start--
	}
	return append(slices.Clone(lines[:start]), lines[header+1:]...), nil
}

// joinAround splices `body` in place of lines[start:end] and joins the result.
func joinAround(lines []string, start, end int, body []string) string {
	return strings.Join(slices.Concat(lines[:start], body, lines[end:]), "\n")
}

func replaceLine(lines []string, at int, text string) []string {
	out := slices.Clone(lines)
	out[at] = text
	return out
}

func insertAt(items []any, position int, value any) []any {
	return slices.Insert(slices.Clone(items), position, value)
}

func removeAt(items []any, position int) []any {
	return slices.Delete(slices.Clone(items), position, position+1)
}

func whereIs(key string) string {
	if key == "cards" {
		return "the deck"
	}
	return "the swap board"
}

// refuseTheCommander is the check AddCard and ReturnCard share: the commander
// sits outside the 99 and may not also be in it.
func refuseTheCommander(doc map[string]any, name string) error {
	for _, commander := range listOf(doc, "commander") {
		if strings.EqualFold(strings.TrimSpace(asString(commander)), strings.TrimSpace(name)) {
			return failf("%s is the commander, which sits outside the 99", quotedValue(name))
		}
	}
	return nil
}

func asInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		// JSON numbers arrive as float64. A fractional one is not a whole
		// number, which is what the message says.
		if v != float64(int(v)) {
			return 0, failf("not whole")
		}
		return int(v), nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, failf("not a number")
		}
		return n, nil
	default:
		return 0, failf("not a number")
	}
}

// documentValue turns a written value into what the parser will give back for
// it, so the expected document and the re-read one are comparable.
func documentValue(written any) any {
	switch v := written.(type) {
	case int:
		return int64(v)
	case []string:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = item
		}
		return out
	default:
		return written
	}
}
