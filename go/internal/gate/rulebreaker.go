package gate

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/pool"
)

// Rulebreaker is the deckbuilding clause Mystery Booster Commander Edition
// printed on eight of its commanders, and the reason this file exists is that
// without it the gate is confidently wrong: a Seluma deck's Angels and Swamps
// are legal Magic and the colour-identity check calls every one of them an
// error. A newcomer told their legal deck is illegal, by name, twice, is
// exactly the failure commandment 2 forbids.
//
// **The clause is read out of the card, never from a list of names kept here.**
// That is the same rule the companion checks follow (`companion.go`), and it is
// load-bearing for the same reason: a table of eight card names is a table that
// is wrong the day a ninth is spoiled, and wrong silently. What is kept here is
// the *grammar* Wizards templated the clause in -- and when a clause does not
// fit that grammar, this file says so out loud (see [Rulebreaker.Unsupported])
// rather than quietly widening nothing or quietly widening everything. That
// last part is ADR 8 at its sharpest edge: an unevaluated rule warns, never
// passes. ADR 51 argues the whole shape.
//
// Re-derive the set of printed clauses rather than trusting any list, here or
// in a commit message:
//
//	curl -s 'https://api.scryfall.com/cards/search?q=oracle%3Arulebreaker' | ...
//
// Three shapes are printed, and all three are handled:
//
//	Rulebreaker — A deck with this commander can have <things>.
//	Rulebreaker — A deck with this commander has no maximum deck size.
//	Rulebreaker — If <name> is your commander, the color identity of <things>
//	              in your deck can include one color of your choice not in your
//	              commander's color identity, and your deck can have any basic
//	              land cards.
//
// The third one -- one colour of the deckbuilder's choosing -- is the only one
// that looks like it needs somewhere to write the choice down. It does not, and
// [Rulebreakers.ColorChoice] argues why.

var (
	rulebreakerLine = regexp.MustCompile(`(?i)^rulebreaker\s*—\s*`)
	// The two sentence openings, stripped to leave the clause body. The second
	// names the card, which is the same thing "this commander" says.
	deckWithThis  = regexp.MustCompile(`(?i)^a deck with this commander\s+`)
	ifYourCmdr    = regexp.MustCompile(`(?i)^if .+? is your commander,\s+`)
	noMaximum     = regexp.MustCompile(`(?i)^has no maximum deck size$`)
	canHave       = regexp.MustCompile(`(?i)^can have\s+`)
	deckCanHave   = regexp.MustCompile(`(?i)^your deck can have\s+`)
	ofAnyIdentity = regexp.MustCompile(`(?i)^(.+?) cards(?: with mana value (\d+) or greater)? of any color identity$`)
	// Tolabow's half-sentence, which grants a colour rather than all of them.
	oneColorOfChoice = regexp.MustCompile(`(?i)^the color identity of (.+?) cards in your deck can include ` +
		`one color of your choice not in your commander's color identity$`)
	// A type-word list: "artifact creature and Equipment", "instant and sorcery".
	typeListJoin = regexp.MustCompile(`(?i)\s*,\s*|\s+and\s+|\s+or\s+`)
	// A type line's words, so "Angel" matches the subtype and never half of a
	// longer one.
	typeWord = regexp.MustCompile(`[A-Za-z]+`)
)

// allowance is one thing a clause lets the deck have. Exactly one of the four
// shapes is set; a clause is a list of these, because every printed clause but
// Grizzlegom's grants two things at once.
type allowance struct {
	// chosen is Tolabow: these cards reach one colour further than the
	// commander, not into every colour. Never true together with a land shape.
	chosen bool
	// types is a disjunction of conjunctions of type-line words. "artifact
	// creature and Equipment" reads as {{artifact, creature}, {Equipment}}:
	// a card matches if it carries every word of any one group.
	types [][]string
	// minMV is the "with mana value 7 or greater" floor, or 0 for none.
	minMV float64
	// anyLand and basicLand are the two land grants. "Any land" subsumes the
	// basics, which is why Grizzlegom's clause does not name them separately.
	anyLand   bool
	basicLand bool
}

func (a allowance) matches(rec *pool.CardRecord) bool {
	switch {
	case a.anyLand:
		return isLandFace(rec)
	case a.basicLand:
		return isBasicLandFace(rec)
	}
	if a.minMV > 0 && rec.CMC < a.minMV {
		return false
	}
	return matchesTypes(rec, a.types)
}

// isBasicLandFace is the front face again (`isLandFace` argues why the front
// face is the card's face for a deckbuilding question), narrowed to the basics.
// Snow-covered basics are "Basic Snow Land — Plains" and are basics.
func isBasicLandFace(rec *pool.CardRecord) bool {
	front := Front(rec.TypeLine)
	return strings.Contains(front, "Basic") && strings.Contains(front, "Land")
}

// matchesTypes asks the type line for whole words, not substrings: "Angel"
// must be the subtype, and a subtype that merely contains those letters is a
// different card type.
func matchesTypes(rec *pool.CardRecord, groups [][]string) bool {
	if len(groups) == 0 {
		return false
	}
	have := map[string]bool{}
	for _, w := range typeWord.FindAllString(Front(rec.TypeLine), -1) {
		have[strings.ToLower(w)] = true
	}
	for _, group := range groups {
		all := len(group) > 0
		for _, want := range group {
			if !have[strings.ToLower(want)] {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// Rulebreaker is one commander's clause, read.
type Rulebreaker struct {
	// Commander is the card the clause is printed on.
	Commander string
	// Text is the clause as printed, "Rulebreaker — " and all.
	Text string
	// NoMaxDeckSize is Whtz: the 100 becomes a floor rather than a number.
	NoMaxDeckSize bool
	// Unsupported is why the clause could not be read, and it is the whole
	// point of the type. A clause this file cannot parse is reported to the
	// player as unapplied -- the identity check then runs as though the card
	// were an ordinary commander, which is the *old* wrong answer rather than
	// a new one, and the report says so instead of leaving them to wonder.
	Unsupported string

	allow []allowance
}

// ReadRulebreaker is the clause printed on one card, parsed. The zero value --
// no text, no allowances, nothing unsupported -- is a card with no clause, and
// [Rulebreaker.Printed] is how callers tell that apart from a clause that
// failed to read.
func ReadRulebreaker(rec *pool.CardRecord) Rulebreaker {
	line := ""
	for _, l := range strings.Split(rec.OracleText, "\n") {
		if l = strings.TrimSpace(l); rulebreakerLine.MatchString(l) {
			line = strings.TrimSpace(reminderTail.ReplaceAllString(l, ""))
			break
		}
	}
	if line == "" {
		return Rulebreaker{}
	}
	rb := Rulebreaker{Commander: rec.Name, Text: line}
	body := strings.TrimSuffix(strings.TrimSpace(rulebreakerLine.ReplaceAllString(line, "")), ".")

	switch {
	case deckWithThis.MatchString(body):
		body = deckWithThis.ReplaceAllString(body, "")
		if noMaximum.MatchString(body) {
			rb.NoMaxDeckSize = true
			return rb
		}
		if !canHave.MatchString(body) {
			rb.Unsupported = "the clause does not say what the deck can have"
			return rb
		}
		rb.allow, rb.Unsupported = readGrants(canHave.ReplaceAllString(body, ""))
	case ifYourCmdr.MatchString(body):
		rb.allow, rb.Unsupported = readChoiceClause(ifYourCmdr.ReplaceAllString(body, ""))
	default:
		rb.Unsupported = "the clause opens in a way this check has not been taught"
	}
	return rb
}

// readGrants reads "<things> and any basic land cards" -- the shape five of the
// six printed grants take -- by peeling the basics off the end first, because
// the head can carry an "and" of its own ("artifact creature and Equipment").
func readGrants(body string) ([]allowance, string) {
	out := []allowance{}
	if head, ok := cutClause(body, "any basic land cards"); ok {
		out = append(out, allowance{basicLand: true})
		body = head
	}
	switch {
	case strings.EqualFold(body, "any basic land cards"):
		return append(out, allowance{basicLand: true}), ""
	case strings.EqualFold(body, "any land cards"):
		return append(out, allowance{anyLand: true}), ""
	}
	a, why := readTypeGrant(body)
	if why != "" {
		return nil, why
	}
	return append(out, a), ""
}

// readTypeGrant reads "<types> cards [with mana value N or greater] of any
// color identity".
func readTypeGrant(body string) (allowance, string) {
	m := ofAnyIdentity.FindStringSubmatch(body)
	if m == nil {
		return allowance{}, fmt.Sprintf("this check cannot tell which cards %s names", wire2(body))
	}
	a := allowance{types: readTypeList(m[1])}
	if len(a.types) == 0 {
		return allowance{}, fmt.Sprintf("this check cannot tell which cards %s names", wire2(body))
	}
	if m[2] != "" {
		mv, err := strconv.Atoi(m[2])
		if err != nil { // unreachable: the group is \d+
			return allowance{}, "the mana value in the clause is not a number"
		}
		a.minMV = float64(mv)
	}
	return a, ""
}

// readChoiceClause reads Tolabow's sentence: a colour for the instants and
// sorceries, and the basics.
func readChoiceClause(body string) ([]allowance, string) {
	out := []allowance{}
	rest := body
	if head, ok := cutClause(rest, "your deck can have any basic land cards"); ok {
		out = append(out, allowance{basicLand: true})
		rest = head
	}
	if deckCanHave.MatchString(rest) {
		grants, why := readGrants(deckCanHave.ReplaceAllString(rest, ""))
		return append(out, grants...), why
	}
	m := oneColorOfChoice.FindStringSubmatch(rest)
	if m == nil {
		return nil, fmt.Sprintf("this check cannot tell what %s grants", wire2(rest))
	}
	types := readTypeList(m[1])
	if len(types) == 0 {
		return nil, fmt.Sprintf("this check cannot tell which cards %s names", wire2(rest))
	}
	return append(out, allowance{chosen: true, types: types}), ""
}

// cutClause peels a trailing ", and <tail>" or " and <tail>" off a sentence and
// returns the head with its comma trimmed. Tolabow's sentence is long enough to
// take the Oxford-style comma before its last clause and the other seven are
// not, which is a difference of one character and was worth exactly one
// unparsed clause before this existed.
func cutClause(body, tail string) (string, bool) {
	head, ok := strings.CutSuffix(body, " and "+tail)
	if !ok {
		return body, false
	}
	return strings.TrimRight(head, ", "), true
}

// readTypeList splits "artifact creature and Equipment" into the groups
// [[artifact creature] [Equipment]].
func readTypeList(s string) [][]string {
	out := [][]string{}
	for _, part := range typeListJoin.Split(s, -1) {
		words := strings.Fields(strings.TrimSpace(part))
		if len(words) > 0 {
			out = append(out, words)
		}
	}
	return out
}

// wire2 quotes a fragment for a message. `wire.Quote` is for whole values; a
// clause fragment reads better in plain double quotes beside the sentence it
// came from.
func wire2(s string) string { return `"` + s + `"` }

// Printed: does this card carry a Rulebreaker clause at all?
func (r Rulebreaker) Printed() bool { return r.Text != "" }

// Clause is the sentence without its "Rulebreaker — " label, for a message
// that has already said the word.
func (r Rulebreaker) Clause() string {
	return strings.TrimSpace(rulebreakerLine.ReplaceAllString(r.Text, ""))
}

// Rulebreakers is the clauses of a deck's commanders, in commander order. None
// of the eight printed Rulebreaker cards has a pairing ability, so today this
// is always zero or one clause -- it is a list because "a deck with this
// commander" is true of each commander a deck has, and a pair that grants two
// things should grant both without anyone editing this file.
type Rulebreakers []Rulebreaker

// ReadRulebreakers reads every commander's clause, skipping the cards with
// none.
func ReadRulebreakers(cmdRecords []*pool.CardRecord) Rulebreakers {
	out := Rulebreakers{}
	for _, rec := range cmdRecords {
		if rec == nil {
			continue
		}
		if rb := ReadRulebreaker(rec); rb.Printed() {
			out = append(out, rb)
		}
	}
	return out
}

// AnyIdentity: may this card sit in the deck whatever its colour identity is?
func (rs Rulebreakers) AnyIdentity(rec *pool.CardRecord) bool {
	for _, rb := range rs {
		for _, a := range rb.allow {
			if !a.chosen && a.matches(rec) {
				return true
			}
		}
	}
	return false
}

// OneChosenColor: is this card in the group that reaches one colour further
// than the commander, rather than into all of them?
func (rs Rulebreakers) OneChosenColor(rec *pool.CardRecord) bool {
	for _, rb := range rs {
		for _, a := range rb.allow {
			if a.chosen && a.matches(rec) {
				return true
			}
		}
	}
	return false
}

// NoMaximumDeckSize: does a commander lift the ceiling on the deck's size?
func (rs Rulebreakers) NoMaximumDeckSize() bool {
	for _, rb := range rs {
		if rb.NoMaxDeckSize {
			return true
		}
	}
	return false
}

// Unread is every clause that could not be parsed, for the caller to report.
func (rs Rulebreakers) Unread() Rulebreakers {
	out := Rulebreakers{}
	for _, rb := range rs {
		if rb.Unsupported != "" {
			out = append(out, rb)
		}
	}
	return out
}

// Why is the tail an identity error carries when the deck has a clause that did
// not cover the card: the clause itself, quoted, so the player reads what
// Wizards printed rather than this file's paraphrase of it -- and so the next
// question, "then which cards *does* it cover", is answered on the same line
// instead of sending a newcomer to look the commander up. Empty when there is
// no readable clause to quote, which keeps the sentence every other deck in the
// library gets exactly as it was.
func (rs Rulebreakers) Why() string {
	quoted := []string{}
	for _, rb := range rs {
		if rb.Unsupported == "" && len(rb.allow) > 0 {
			quoted = append(quoted, fmt.Sprintf("%s's Rulebreaker does not cover it: %s",
				rb.Commander, wire2(rb.Clause())))
		}
	}
	if len(quoted) == 0 {
		return ""
	}
	return " -- and " + strings.Join(quoted, "; ")
}

// TypeHints is the type-line words a clause mentions, for a caller that has to
// narrow a card index before it can apply the clause properly.
//
// **It is deliberately a superset, and [Rulebreakers.AnyIdentity] remains the
// only authority.** The words come back flat, so The Everforger's "artifact
// creature and Equipment" yields artifact, creature and Equipment separately: a
// caller matching *any* of them sees every card the clause covers plus some it
// does not -- a plain artifact, a creature that is not one. That is the right
// shape for a prefilter and the wrong shape for a rule, and the difference is
// the reason this returns words rather than a predicate. A second copy of the
// predicate, in SQL, would be a fact kept in two places by hand, and this repo
// has had five of those rot.
//
// Empty when no clause names a type, which is the caller's signal that there is
// nothing to widen and its own query should go out unchanged.
func (rs Rulebreakers) TypeHints() []string {
	seen := map[string]bool{}
	for _, rb := range rs {
		if rb.Unsupported != "" {
			continue
		}
		for _, a := range rb.allow {
			if a.anyLand || a.basicLand {
				seen["Land"] = true
			}
			for _, group := range a.types {
				for _, word := range group {
					seen[word] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// ColorChoice is Tolabow's clause settled without storing anything, and the
// argument is worth writing down because the obvious design is a field in the
// deck file.
//
// The clause lets the instants and sorceries include "one color of your choice"
// from outside the commander's identity. A deck is legal exactly when some such
// colour exists -- that is, when the off-identity colours those cards actually
// use number one or none. Nothing in the game ever asks which colour was picked
// afterwards: it changes no cost, no ability, no zone. So the choice is not a
// fact about the deck that has to be remembered, it is a fact that can be read
// back off the deck, and reading it back is not a heuristic -- it is the same
// question the rule asks, answered exactly.
//
// The alternative was a `rulebreaker_color:` key in the deck file, which would
// have reached the model, the emitter, the edit engine, the wire and the
// browser for one card, and would then have had to be kept in step with the
// cards themselves by hand -- a second copy of a fact, which is how this repo's
// prose has gone wrong five times.
//
// It returns the off-identity colours the covered cards reach, sorted, and the
// names that reach them. Two or more colours is the illegal deck.
func (rs Rulebreakers) ColorChoice(entries []Entry, identity map[string]bool) (colors []string, by map[string][]string) {
	by = map[string][]string{}
	found := map[string]bool{}
	for _, e := range entries {
		if e.Rec == nil || !rs.OneChosenColor(e.Rec) {
			continue
		}
		for c := range minus(setOf(e.Rec.ColorIdentity), identity) {
			found[c] = true
			by[c] = append(by[c], e.Name)
		}
	}
	for c := range found {
		colors = append(colors, c)
	}
	sort.Strings(colors)
	for c := range by {
		sort.Strings(by[c])
	}
	return colors, by
}
