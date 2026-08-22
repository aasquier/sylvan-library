// Package decklist reads a decklist the way people actually paste one:
// `decks/decklist.py`, verbatim in every way that can change an answer.
//
// Pure text in, structured lines out. No card pool, no filesystem, no
// judgement about whether a card is real -- that is `deckimport`'s job, and
// keeping the two apart is what lets the grammar be tested exhaustively
// without a database.
//
// There is no decklist standard, so this parses the union of what the exports
// people use actually emit; the Python docstring has the table, and the
// patterns below are the same patterns.
//
// **Two things it deliberately does not do.** It never consults a card list to
// disambiguate, and it never silently drops a line it could not read. Lines it
// cannot turn into a name land in Unreadable with their line number, so the
// importer can report them.
//
// Three places where Go's regexp is not Python's `re`, and each is closed here
// rather than left to be discovered by somebody's paste:
//
//   - **`\s`**. Python's is `str.isspace()` -- which includes U+00A0, the
//     separators, and U+001C-U+001F -- and Go's is five ASCII characters. A
//     non-breaking space is exactly what arrives when a list is copied out of
//     a web page, so the class is spelled out (`sp`) and used everywhere the
//     original writes `\s`.
//   - **`splitlines()`**. Python breaks on eleven characters, Go's
//     `strings.Split` on one. A lone `\r` from an old export, or the U+2028
//     a browser paste can carry, would otherwise be one enormous line.
//   - **`\d`**. Python's matches any Unicode decimal digit and `int()` reads
//     it; `[0-9]` would read `٣ Forest` as a card named "٣ Forest" where
//     Python reads three Forests. `digitValue` is the six lines that keep
//     them agreeing.
//
// The one ambiguity that cannot be resolved here: a bare `3 Steps Ahead` is
// either three copies of "Steps Ahead" or one copy of the blue instant.
// Without a pool both readings are equally good, so the leading-number reading
// wins -- and the resulting unknown name gets reported rather than guessed at.
package decklist

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Where a line ends up. `swap_board` is the model's name for the maybeboard:
// Commander has no sideboard, and a list of cards you are considering is
// exactly what the deck file's swap board already holds. `ignored` is a
// section whose lines are not deck cards at all.
var sectionWords = map[string]string{
	"commander":   "commander",
	"commanders":  "commander",
	"companion":   "companion",
	"companions":  "companion",
	"deck":        "deck",
	"decklist":    "deck",
	"main":        "deck",
	"mainboard":   "deck",
	"main deck":   "deck",
	"maindeck":    "deck",
	"sideboard":   "swap_board",
	"maybeboard":  "swap_board",
	"considering": "swap_board",
	// Tokens are not cards in the 99. Recognised so they are reported as
	// skipped rather than resolved and then reported as unknown cards.
	"token":  "ignored",
	"tokens": "ignored",
}

func init() {
	// TappedOut and Deckstats group the deck under card-type headings. They
	// are still deck cards, so they all map to the same section -- and note
	// that a `Lands` heading does NOT set the land category. That comes from
	// the pool's `is_land`, which is right about the double-faced cards a
	// heading is wrong about.
	for _, stem := range []string{"creature", "instant", "artifact", "enchantment",
		"planeswalker", "land", "battle", "spell", "other"} {
		sectionWords[stem] = "deck"
		sectionWords[stem+"s"] = "deck"
	}
	sectionWords["sorcery"] = "deck"
	sectionWords["sorceries"] = "deck"
}

// sp is Python's `\s` for a str pattern: the characters `str.isspace()` is
// true for. `\t-\r` is U+0009 to U+000D; `\p{Z}` is the separator categories,
// which is where U+00A0 and the ordinary space live.
const sp = `[\t-\r \x{1c}-\x{1f}\x{85}\p{Z}]`

var (
	// `Commander`, `Commander:`, `COMMANDER (1)`. The count is whatever the
	// exporter thought; it is not trusted, only tolerated.
	//
	// Taken apart in `headerSection` rather than matched by one pattern, and
	// the reason is measured: the single pattern this replaces let three
	// separate `\s*` runs compete for the same run of spaces, and one
	// 512-character line took 26 seconds. Both patterns here are unambiguous,
	// so each is linear -- and RE2 could not backtrack anyway, which is
	// exactly why the shape must not be reinvented: the Python side still can.
	headerCount = regexp.MustCompile(`\(` + sp + `*\p{Nd}+` + sp + `*\)$`)
	headerWord  = regexp.MustCompile(`^[A-Za-z][A-Za-z ]*$`)

	// A leading quantity. Capped at three digits so that `1996 World Champion`
	// parses as a name rather than as 1,996 copies of "World Champion".
	//
	// Python writes a `(?=\S)` lookahead, which RE2 has not got. The pattern
	// here matches the same prefix and the caller checks the next character
	// itself -- see `leadingQty`, where the equivalence is argued.
	qtyRe = regexp.MustCompile(`^(\p{Nd}{1,3})` + sp + `*[xX]?` + sp + `+`)

	// Trailing annotations, stripped right to left. Each is anchored to the
	// end so that a card name containing brackets or parentheses mid-string
	// survives.
	markerRe  = regexp.MustCompile(sp + `*\*([^*]*)\*` + sp + `*$`)
	bracketRe = regexp.MustCompile(sp + `*\[([^\]]*)\]` + sp + `*$`)
	// `(LTC) 284`, `(NEO) 123s`, `(WAR) 25★`, or a bare `(c21)`. The set code
	// is 2-6 alphanumerics, which is short enough not to swallow `B.F.M. (Big
	// Furry Monster)` or `Erase (Not the Urza's Legacy One)`.
	printingRe = regexp.MustCompile(
		sp + `*\(([A-Za-z0-9]{2,6})\)(?:` + sp + `+[A-Za-z0-9\-★]+)?` + sp + `*$`)

	// Deckstats puts the set code in front: `1 [C17] Arahbo, Roar of the
	// World`. Safe to strip unconditionally because no card name begins with a
	// bracket.
	leadingSetRe = regexp.MustCompile(`^\[[A-Za-z0-9]{2,6}\]` + sp + `*`)
)

// MaxLine is the longest line this will look at, in code points.
//
// The headroom is deliberate and Magic decides it: the longest card name ever
// printed is the 141-character Unhinged elemental whose name is a joke about
// long card names, and a line carries a quantity, a set code, a collector
// number and any markers besides. Nothing real comes close to this.
const MaxLine = 512

// Card is one readable line. The name is verbatim -- nothing has resolved it
// yet.
type Card struct {
	Name    string
	Qty     int
	Section string
	LineNo  int
}

// Line is a line that was not turned into a card, with its number.
type Line struct {
	LineNo int
	Text   string
}

// List is what a paste parsed into.
type List struct {
	Cards []Card
	// Lines that are not blank, not comments and not headers, but left no
	// name behind once the annotations were stripped.
	Unreadable []Line
	// Lines under a section that is not part of the deck, e.g. Tokens.
	Skipped []Line
}

// Section is every card filed under one section name, in order.
func (l List) Section(name string) []Card {
	out := []Card{}
	for _, c := range l.Cards {
		if c.Section == name {
			out = append(out, c)
		}
	}
	return out
}

// Commander is the names the list nominated for the command zone.
func (l List) Commander() []string {
	out := []string{}
	for _, c := range l.Section("commander") {
		out = append(out, c.Name)
	}
	return out
}

// Companion is the name the list nominated as its companion, or "".
func (l List) Companion() string {
	if found := l.Section("companion"); len(found) > 0 {
		return found[0].Name
	}
	return ""
}

// Parse reads a pasted decklist. Never fails: unreadable lines are reported.
func Parse(text string) List {
	out := List{Cards: []Card{}, Unreadable: []Line{}, Skipped: []Line{}}
	section := "deck"

	for i, raw := range splitLines(text) {
		lineNo := i + 1
		line := trimRight(raw)
		if trim(line) == "" {
			continue
		}

		// Over the bound is unreadable rather than fatal, because parsing
		// never fails and a pasted list is somebody's whole deck -- one absurd
		// line must not cost them the other ninety-eight. The reported text is
		// sliced for the same reason the bound exists: echoing the line back
		// whole would hand the input straight to a response and a log.
		if utf8.RuneCountInString(line) > MaxLine {
			out.Unreadable = append(out.Unreadable,
				Line{LineNo: lineNo, Text: firstRunes(trim(line), MaxLine)})
			continue
		}

		// MTGO writes comments as `//`, which is also the double-faced card
		// separator -- so only a line that *starts* with it is a comment.
		// Deckstats writes its section headers that way (`//Commander`), so
		// the word is checked before the line is discarded: mistaking that
		// header for a comment puts the commander in the 99, which is a wrong
		// deck rather than a missing one.
		left := trimLeft(line)
		commented := trim(strings.TrimLeft(left, "#/"))
		if strings.HasPrefix(left, "#") || strings.HasPrefix(left, "//") {
			if header, ok := headerSection(commented); ok {
				section = header
			}
			continue
		}

		if header, ok := headerSection(line); ok {
			section = header
			continue
		}

		body, markers := stripAnnotations(line)
		if body == "" {
			out.Unreadable = append(out.Unreadable, Line{LineNo: lineNo, Text: trim(line)})
			continue
		}

		qty := 1
		if n, rest, ok := leadingQty(body); ok {
			qty, body = n, trim(rest)
		}
		body = trim(leadingSetRe.ReplaceAllString(body, ""))
		if body == "" {
			out.Unreadable = append(out.Unreadable, Line{LineNo: lineNo, Text: trim(line)})
			continue
		}

		if section == "ignored" {
			out.Skipped = append(out.Skipped, Line{LineNo: lineNo, Text: body})
			continue
		}

		// A `*CMDR*` marker overrides the section it was found in: Moxfield's
		// plain export has no headers at all and marks the commander inline.
		where := section
		if markers["cmdr"] {
			where = "commander"
		}
		out.Cards = append(out.Cards,
			Card{Name: body, Qty: qty, Section: where, LineNo: lineNo})
	}
	return out
}

// stripAnnotations peels printing and category annotations off the end of a
// line, returning what is left and any `*...*` markers found, lowercased.
// Applied in a loop because they combine: `(2X2) 297 *F* *CMDR*`.
func stripAnnotations(text string) (string, map[string]bool) {
	markers := map[string]bool{}
	for {
		if m := markerRe.FindStringSubmatchIndex(text); m != nil {
			markers[strings.ToLower(trim(text[m[2]:m[3]]))] = true
			text = text[:m[0]]
			continue
		}
		if m := bracketRe.FindStringSubmatchIndex(text); m != nil {
			// Archidekt writes the deck's own category here, e.g. `[Ramp]` or
			// `[Commander{top}]`. Only the commander label is kept, and only
			// because it is a fact the exporter stated rather than something
			// inferred from the card -- ADR 13 leaves every other category to
			// a human, on the grounds that a guessed category ends up asserted
			// in a generated primer.
			label := strings.ToLower(trim(strings.SplitN(text[m[2]:m[3]], "{", 2)[0]))
			if label == "commander" || label == "commanders" {
				markers["cmdr"] = true
			}
			text = text[:m[0]]
			continue
		}
		if m := printingRe.FindStringIndex(text); m != nil {
			text = text[:m[0]]
			continue
		}
		return trim(text), markers
	}
}

// headerSection peels a section header apart: a word, one separator, an
// ignored count.
//
// The order is the one the old pattern required -- separator before count --
// so `Commander: (1)` is a header and `Commander (1):` is not, exactly as
// before.
func headerSection(line string) (string, bool) {
	text := trim(line)
	if m := headerCount.FindStringIndex(text); m != nil {
		text = trimRight(text[:m[0]])
	}
	// One separator, never a run: `[:\-]?` is what the pattern allowed, and
	// trimming the set would quietly start accepting `Deck--`.
	if strings.HasSuffix(text, ":") || strings.HasSuffix(text, "-") {
		text = trimRight(text[:len(text)-1])
	}
	if !headerWord.MatchString(text) {
		return "", false
	}
	section, ok := sectionWords[strings.Join(strings.FieldsFunc(
		strings.ToLower(text), isPySpace), " ")]
	return section, ok
}

// leadingQty is `_QTY.match` with its `(?=\S)` lookahead done by hand.
//
// The lookahead makes the pattern require a non-space after the whitespace
// run. Python's engine backtracks to find one: `sp+` is greedy, so it takes
// every space and the lookahead then asks what follows. If that is a space the
// match failed for good -- a shorter `sp+` only puts the engine back on
// whitespace -- and if it is the end of the line, likewise. So "greedy match,
// then the next character must exist and must not be a space" is the same
// answer, and it is what this does.
func leadingQty(body string) (int, string, bool) {
	m := qtyRe.FindStringSubmatchIndex(body)
	if m == nil {
		return 0, body, false
	}
	rest := body[m[1]:]
	if rest == "" {
		return 0, body, false
	}
	if r, _ := utf8.DecodeRuneInString(rest); isPySpace(r) {
		return 0, body, false
	}
	qty := 0
	for _, r := range body[m[2]:m[3]] {
		qty = qty*10 + digitValue(r)
	}
	return qty, rest, true
}

// digitValue is `int()` on one Unicode decimal digit. Every Nd block is ten
// contiguous code points beginning at its own zero, so walking back to the
// start of the run gives the value -- which is how `int("٣")` gets 3.
func digitValue(r rune) int {
	if r >= '0' && r <= '9' {
		return int(r - '0')
	}
	for value := 0; value < 10; value++ {
		if !unicode.Is(unicode.Nd, r-rune(value)-1) {
			return value
		}
	}
	return 0
}

// splitLines is Python's `str.splitlines()`: eleven line boundaries, not one.
func splitLines(text string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !isLineBreak(r) {
			i += size
			continue
		}
		out = append(out, text[start:i])
		i += size
		// `\r\n` is one boundary, not two.
		if r == '\r' && i < len(text) && text[i] == '\n' {
			i++
		}
		start = i
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}

func isLineBreak(r rune) bool {
	switch r {
	case '\n', '\v', '\f', '\r', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// isPySpace is `str.isspace()`. Go's `unicode.IsSpace` is the same set minus
// the four information separators, which Python counts and Go does not.
func isPySpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

func trim(s string) string      { return strings.TrimFunc(s, isPySpace) }
func trimLeft(s string) string  { return strings.TrimLeftFunc(s, isPySpace) }
func trimRight(s string) string { return strings.TrimRightFunc(s, isPySpace) }

func firstRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}
