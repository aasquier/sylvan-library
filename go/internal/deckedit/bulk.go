package deckedit

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

// The bulk edit: one pasted list, the whole 99, and a plan somebody agreed to
// before any of it happened.
//
// The eleventh operation, and the first one that is two functions rather than
// one. Everything else in this package answers "change this"; this one answers
// "what would change" first, and only then "change it" -- because the edit is
// large enough that a person cannot consent to it without being shown it. A
// dialog that says *this will overwrite things* is a warning. A dialog that
// says *these four cards go to the graveyard and these six reasons are
// rewritten, here they are* is a decision.
//
// **Nothing here is new text surgery.** `ApplyBulk` is a fold of the
// operations that already exist -- SetCardField, AddCard, EntombCard -- over
// the file's text, one at a time, exactly as `api.entombCards` folds
// EntombCard. That is the whole safety argument: each step is verified against
// its own document oracle by `verified`, so a bulk edit inherits the
// verification N times over instead of needing a new proof. What is new is the
// *diff*, and the diff touches no text at all.
//
// **The plan is computed from the file, so it can be tested without a pool.**
// Same split as `decklist` and `deckimport`: the caller resolves names against
// the card pool and hands down resolved cards; everything after that -- who is
// added, whose reason changed, who goes to the graveyard, who is left where
// they are and why -- is a reading of `deck.yaml` and is decided here.
//
// **A card that is in the 99 and not in the paste is ENTOMBED, never deleted**
// (Aaron, 2026-08-29). It keeps its `why`, it is one click from coming back
// (ADR 27), and the plan names every one of them before anybody agrees to
// anything. That ruling is what makes this operation safe to offer at all: the
// worst outcome of a paste with a typo in it is a card sitting in the
// graveyard with its reasoning intact.
//
// **The rationale rules are the ones that already hold, not new ones.** A line
// with a quoted reason is a person writing their own `why` -- the same act as
// typing it into the deck editor, which is what rule 4 asks for. A line
// *without* one leaves the reason the card already has: not inventing includes
// not deleting, and a plain Moxfield export carries no quotes at all. And a
// rewrite goes through SetCardField, so `why_by: claude` comes off by the same
// line of code that takes it off everywhere else (ADR 41) -- the sentence is a
// person's now.
//
// **Why the plan carries a fingerprint.** Somebody editing in another tab, or
// a double-click on the confirm button, must not have their plan applied to a
// deck it was not computed against: the entombments would be about cards the
// paste was never compared with. So a plan records the deck it read, and
// ApplyBulk refuses text that is not that deck. It does not close the window
// between the read and the write -- that window belongs to every edit route
// here and closing it takes a lock this package has never had -- it closes the
// much wider one between being shown a plan and agreeing to it.

// Fingerprint identifies one exact deck file.
//
// Over the whole text rather than over the cards, deliberately: a plan is a
// claim about a file, and a file with a different comment in it is a file
// somebody edited. Strict fails safe here, and the cost of being wrong in the
// strict direction is one wasted round trip.
//
// The value never renders. It rides on the wire and comes back, and what a
// person reads when it does not match is a sentence about their deck having
// changed (commandment 10).
func Fingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// BulkCard is one card a paste asked for, already resolved against the pool.
//
// `Category` is only ever read for a card being added -- the paste format has
// no category column, so a card already in the 99 keeps whatever it is filed
// under.
type BulkCard struct {
	Name     string
	Qty      int
	Category string
	// Why is the quoted rationale from the pasted line, "" when the line
	// carried none. Empty means "leave what is there", never "blank it".
	Why string
}

// BulkAdd is a card the paste has and the deck does not.
type BulkAdd struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Qty      int    `json:"qty"`
	Why      string `json:"why"`
}

// BulkRewrite is a rationale the paste replaces, with the one it replaces.
//
// `Was` is shown beside `Why` on the confirmation, which is the difference
// between agreeing to "six reasons will be rewritten" and agreeing to six
// specific sentences being replaced by six other specific sentences.
type BulkRewrite struct {
	Name string `json:"name"`
	Was  string `json:"was"`
	Why  string `json:"why"`
	// WasDrafted says the sentence being replaced carries `why_by: claude`
	// (ADR 41). Worth showing: replacing a draft with your own words is the
	// intended path, and replacing your own words is the one to look twice at.
	WasDrafted bool `json:"was_drafted"`
}

// BulkRequantify is a quantity the paste changes. Basic lands are why this
// exists at all.
type BulkRequantify struct {
	Name string `json:"name"`
	Was  int    `json:"was"`
	Qty  int    `json:"qty"`
}

// BulkLeft is a pasted card the edit will not touch, and the reason.
//
// Never silent, which is the point of the type. A paste that named the
// commander -- Moxfield's export carries it under `SIDEBOARD:` -- or a card
// sitting on the swap board is a paste whose author expects something to
// happen, and a quietly skipped line is this repo's most-repeated bug.
type BulkLeft struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// BulkBlocked is a pasted card the edit *cannot* apply, and why.
//
// One case reaches it: a curated deck refuses a new card with no `why` (rule
// 4, ADR 13), and AddCard would refuse mid-fold. Predicted here rather than
// discovered there, because a fold that stopped at card forty would leave the
// person reading a refusal about a plan they had already agreed to. A blocked
// plan is refused whole and says exactly what to type.
type BulkBlocked struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// BulkPlan is what a paste would do to a deck, and what it would not.
type BulkPlan struct {
	// Basis is the deck this plan was read from. See Fingerprint.
	Basis string `json:"basis"`
	// Draft says the deck is a draft, so a card may arrive without a reason
	// (ADR 13). The surface says so rather than leaving somebody to discover
	// which rules apply to their deck.
	Draft      bool             `json:"draft"`
	Add        []BulkAdd        `json:"add"`
	Rewrite    []BulkRewrite    `json:"rewrite"`
	Requantify []BulkRequantify `json:"requantify"`
	// Entomb is every card in the 99 the paste did not name, in the order the
	// deck holds them. Named in full, always: this is the half somebody has to
	// agree to.
	Entomb []string `json:"entomb"`
	// Unchanged is every card the paste named that is already exactly that.
	Unchanged []string `json:"unchanged"`
	// Merged is a name that appeared on more than one line. The quantities are
	// added together and the first reason wins, which is `deckimport`'s rule
	// and argued there: choosing between two reasons would be composing one.
	Merged  []string      `json:"merged"`
	Left    []BulkLeft    `json:"left"`
	Blocked []BulkBlocked `json:"blocked"`
}

// Touches reports whether the plan would change anything at all.
func (p *BulkPlan) Touches() bool {
	return len(p.Add) > 0 || len(p.Rewrite) > 0 || len(p.Requantify) > 0 ||
		len(p.Entomb) > 0
}

// Reasons the edit leaves a pasted card where it is. Sentences rather than
// keywords because they are read by a person, and each says what to do
// instead: a dead end with no exit is the failure commandment 2 names.
const (
	leftCommander = "sits in the command zone, which is outside the 99"
	leftCompanion = "is this deck's companion, which sits outside the 99"
	leftSwapBoard = "is on the swap board. This edit works on the 99 only, so " +
		"it was left there — move it across from the swap board itself"
	leftGraveyard = "is in the graveyard. Bring it back from the deck page and " +
		"it returns with the reason it was buried with"
)

// What a plan refuses over. Both are things AddCard and SetCardField would
// refuse *during* the fold, predicted here so nobody agrees to a plan that
// then stops at card forty -- and both say what to type instead, because a
// dead end with no exit is the failure commandment 2 names.
const (
	blockedNoWhy = "is new to a curated deck, and a card in a curated deck " +
		"needs a reason. Put one in quotes at the end of its line"
	// `0 Sol Ring`. It probably means "take this out", and probably is not
	// good enough: leaving the line off is how a list says that, and it is
	// already what every card the list omits means.
	blockedNoneOfIt = "is written with a quantity of 0. If you meant to take " +
		"it out of the deck, leave the line off your list entirely — anything " +
		"your list does not name goes to the graveyard"
)

// PlanBulk reads what a paste would do to this deck. It changes nothing.
//
// `wanted` is the paste, resolved: names as the pool spells them, quantities
// summed by the caller's grammar, and the rationale column verbatim. Names the
// pool could not resolve never arrive here -- they are reported beside the
// plan by whoever did the resolving, because a name that is not a card is a
// fact about the pool and not about this deck.
func PlanBulk(text string, wanted []BulkCard) (*BulkPlan, error) {
	doc, lines, err := open(text)
	if err != nil {
		return nil, err
	}
	// Not read, but a plan that cannot be applied should fail while nothing is
	// at stake. `locateCard` refuses a file whose parse and text disagree, and
	// discovering that halfway through a fold is discovering it too late.
	if start, stop, err := blockSpan(lines, "cards"); err == nil {
		if spans := entrySpans(lines, start, stop); len(spans) != len(listOf(doc, "cards")) {
			return nil, countMismatch("cards", len(listOf(doc, "cards")), len(spans))
		}
	}

	plan := &BulkPlan{
		Basis: Fingerprint(text), Draft: !requiresRationale(doc),
		Add: []BulkAdd{}, Rewrite: []BulkRewrite{}, Requantify: []BulkRequantify{},
		Entomb: []string{}, Unchanged: []string{}, Merged: []string{},
		Left: []BulkLeft{}, Blocked: []BulkBlocked{},
	}

	outside := map[string]string{}
	for _, name := range listOf(doc, "commander") {
		if key := foldName(asString(name)); key != "" {
			outside[key] = leftCommander
		}
	}
	if key := foldName(asString(doc["companion"])); key != "" {
		outside[key] = leftCompanion
	}
	for _, item := range listOf(doc, "swap_board") {
		if card, ok := item.(map[string]any); ok {
			outside[foldName(asString(card["name"]))] = leftSwapBoard
		}
	}
	for _, item := range listOf(doc, Graveyard) {
		if card, ok := item.(map[string]any); ok {
			outside[foldName(asString(card["name"]))] = leftGraveyard
		}
	}

	// The 99 as it stands, in file order -- which is the order the
	// entombments are reported in, so the list reads like the deck.
	held := map[string]map[string]any{}
	order := []string{}
	for _, item := range listOf(doc, "cards") {
		card, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key := foldName(asString(card["name"]))
		if key == "" || held[key] != nil {
			continue
		}
		held[key] = card
		order = append(order, key)
	}

	named := map[string]bool{}
	for _, card := range mergeBulk(wanted, plan) {
		key := foldName(card.Name)
		if key == "" {
			continue
		}
		if reason, ok := outside[key]; ok {
			plan.Left = append(plan.Left, BulkLeft{Name: card.Name, Reason: reason})
			continue
		}
		if card.Qty < 1 {
			// Named, so the same card is not *also* reported for burial. The
			// list did mention it; what it asked for is the thing that cannot
			// be done, and the refusal above says what to write instead.
			named[key] = true
			plan.Blocked = append(plan.Blocked,
				BulkBlocked{Name: card.Name, Reason: blockedNoneOfIt})
			continue
		}
		existing := held[key]
		if existing == nil {
			// Rule 4 at the point a card enters the deck, predicted rather
			// than discovered. AddCard raises exactly this refusal.
			if !plan.Draft && strings.TrimSpace(card.Why) == "" {
				plan.Blocked = append(plan.Blocked,
					BulkBlocked{Name: card.Name, Reason: blockedNoWhy})
				continue
			}
			plan.Add = append(plan.Add, BulkAdd{Name: card.Name,
				Category: card.Category, Qty: card.Qty,
				Why: strings.TrimSpace(card.Why)})
			continue
		}
		named[key] = true
		name := asString(existing["name"])
		moved := false
		if was := qtyOf(existing); was != card.Qty {
			plan.Requantify = append(plan.Requantify,
				BulkRequantify{Name: name, Was: was, Qty: card.Qty})
			moved = true
		}
		// A blank column leaves the reason alone. The paste is not asserting
		// that this card has no reason; it is an export with nowhere to put
		// one, which is the ordinary case.
		if why, was := strings.TrimSpace(card.Why),
			strings.TrimSpace(asString(existing["why"])); why != "" && why != was {
			// `Was` is trimmed because it is rendered inside quotation marks
			// beside the new sentence, and a folded `why: >` block parses with
			// a trailing newline -- which shows up on screen as a gap before
			// the closing quote of somebody's own writing.
			plan.Rewrite = append(plan.Rewrite, BulkRewrite{Name: name,
				Was: was, Why: why,
				WasDrafted: asString(existing["why_by"]) == DraftedBy})
			moved = true
		}
		if !moved {
			plan.Unchanged = append(plan.Unchanged, name)
		}
	}

	for _, key := range order {
		if !named[key] {
			plan.Entomb = append(plan.Entomb, asString(held[key]["name"]))
		}
	}
	return plan, nil
}

// ApplyBulk folds a plan over the deck's text, and hands back the text it
// would become.
//
// Refuses before it starts if the text is not the deck the plan was read
// from, or if the plan is blocked, or if it would do nothing -- all three
// before a single line is rewritten, which is what makes a refused bulk edit
// exactly as harmless as any other refused edit here.
//
// The order is the argued one. Reasons and quantities first, against the deck
// as it stands. Then the additions, which are anchored to the last card
// already filed under their category and want that card still present. Then
// the burials, last, so they land in the graveyard in the order the plan named
// them and so nothing is buried before every other step has succeeded.
func ApplyBulk(text string, plan *BulkPlan) (string, error) {
	if plan == nil {
		return "", failf("there is no plan to apply")
	}
	if plan.Basis != Fingerprint(text) {
		return "", failf("this deck changed while the plan was on screen, so " +
			"nothing was written. Take another look and the plan will be " +
			"worked out again against the deck as it is now.")
	}
	if len(plan.Blocked) > 0 {
		return "", failf("%s %s", quotedValue(plan.Blocked[0].Name), plan.Blocked[0].Reason)
	}
	if !plan.Touches() {
		return "", failf("this list matches the deck already, so there is nothing to change")
	}

	var err error
	for _, r := range plan.Rewrite {
		if text, err = SetCardField(text, r.Name, "why", r.Why); err != nil {
			return "", err
		}
	}
	for _, q := range plan.Requantify {
		if text, err = SetCardField(text, q.Name, "qty", q.Qty); err != nil {
			return "", err
		}
	}
	for _, a := range plan.Add {
		if text, err = AddCard(text, a.Name, a.Category, a.Why, a.Qty, "cards"); err != nil {
			return "", err
		}
	}
	for _, name := range plan.Entomb {
		if text, err = EntombCard(text, name); err != nil {
			return "", err
		}
	}
	return text, nil
}

// mergeBulk folds repeated names into one card, and records that it did.
//
// `deckimport.buildEntries` decided both halves of this and argued them:
// quantities add, and the first reason wins because choosing between two
// reasons somebody wrote on two lines would be composing one. Repeated here
// rather than shared because that function also resolves, categorises and
// demotes, and the two-line rule is the only part a bulk edit wants.
func mergeBulk(wanted []BulkCard, plan *BulkPlan) []BulkCard {
	out := make([]BulkCard, 0, len(wanted))
	at := map[string]int{}
	for _, card := range wanted {
		key := foldName(card.Name)
		if key == "" {
			continue
		}
		i, seen := at[key]
		if !seen {
			at[key] = len(out)
			out = append(out, card)
			continue
		}
		out[i].Qty += card.Qty
		if strings.TrimSpace(out[i].Why) == "" {
			out[i].Why = card.Why
		}
		if !slices.Contains(plan.Merged, out[i].Name) {
			plan.Merged = append(plan.Merged, out[i].Name)
		}
	}
	return out
}

// qtyOf is a card entry's quantity, defaulting to one the way the deck files
// do -- `qty:` is written only when it is not 1.
func qtyOf(card map[string]any) int {
	if _, ok := card["qty"]; !ok {
		return 1
	}
	qty, err := asInt(card["qty"])
	if err != nil || qty < 1 {
		return 1
	}
	return qty
}

// foldName is the comparison every list in this file is keyed on, and it is
// `nameMatches`' comparison written for a map key rather than a pair.
func foldName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
