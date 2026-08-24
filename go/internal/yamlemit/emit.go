// Package yamlemit is the deck file's emitter: one recorded YAML style,
// byte for byte.
//
// It exists because of what an edit *is* here. `deck.yaml` is the source of
// truth (ADR 1) and `swaps.md` is literally a diff of it, so the size of an
// edit is part of its correctness: a one-card swap has to be a one-card
// diff. The edit engine achieves that by rewriting only the lines it
// changes -- and the lines it writes come out of this emitter, at a given
// width, with fixed choices about quoting and folding. A second emitter
// that merely produced *valid* YAML would produce a different diff for the
// same edit, and two writers would disagree about what a swap looks like
// while both were "right". So this is not a general YAML writer; it is one
// recorded style, and the test that matters is byte equality against the
// frozen corpus (`testdata/render.json`).
//
// Three decisions carry the style, each because a reasonable alternative
// gives a different answer:
//
//   - the scalar analysis, which decides what styles a string may take. A
//     trailing space forbids the block style; a space before a line break
//     forbids everything except double quotes.
//   - the implicit resolver (resolve.go), because the plain style is only
//     offered to a value that reads back as a string -- and it reads back
//     under YAML 1.1 rules, deliberately, not 1.2's.
//   - the line breaking in `writeFolded` and `writePlain`, which breaks
//     *after* the configured width rather than at it, and only at a single
//     space.
//
// Everything here counts columns in **code points**, not bytes. A `why`
// with an em dash in it wraps where the corpus wraps it, not one byte
// earlier.
package yamlemit

import (
	"fmt"
	"strconv"
	"strings"
)

// The emitter's fixed defaults for the knobs the deck writer does not set.
const (
	bestIndent   = 2
	defaultWidth = 80
	lineBreak    = "\n"

	// The two character sets PyYAML spells inline as string literals. NEL and
	// the two Unicode separators are line breaks in YAML 1.1 and are written
	// as escapes here so this file stays readable in a terminal.
	whitespaceSet = "\x00 \t\r\n\u0085\u2028\u2029"
	breakSet      = "\n\u0085\u2028\u2029"
)

// scalarStyle is PyYAML's `self.style`; the empty string is plain.
type scalarStyle string

const (
	stylePlain        scalarStyle = ""
	styleSingleQuoted scalarStyle = "'"
	styleDoubleQuoted scalarStyle = `"`
	styleFolded       scalarStyle = ">"
)

// emitter is PyYAML's `Emitter`, narrowed to the states a one-key mapping
// reaches. The field names are Python's on purpose: this file is meant to be
// read beside `yaml/emitter.py` whenever either of them moves.
type emitter struct {
	out        strings.Builder
	column     int
	indent     int // -1 stands for Python's None
	indents    []int
	whitespace bool
	indention  bool
	bestWidth  int

	// openEnded is PyYAML's flag for "the last thing written left the
	// document open": a `>+` block keeps its trailing newlines, so the stream
	// has to be closed with an explicit `...` or a reader cannot tell where
	// the scalar stopped. Any indicator written afterwards clears it. It
	// reaches a deck file only through a rationale that ends in a blank line,
	// which is rare and exactly why it would otherwise never be noticed.
	openEnded bool
}

func newEmitter(width int) *emitter {
	// `if width and width > self.best_indent*2: self.best_width = width`
	best := defaultWidth
	if width > bestIndent*2 {
		best = width
	}
	return &emitter{
		indent:     -1,
		whitespace: true,
		indention:  true,
		bestWidth:  best,
	}
}

// write appends without touching the column. Every call site in PyYAML
// adjusts `self.column` itself -- sometimes deliberately not at all, since a
// line break resets it -- so the two stay separate here as well.
func (e *emitter) write(data string) { e.out.WriteString(data) }

func runeLen(s string) int { return len([]rune(s)) }

// increaseIndent is `_FoldedDumper.increase_indent`: the override that passes
// `indentless=False` unconditionally, so a list sits indented under its key
// the way every hand-written deck file writes one.
func (e *emitter) increaseIndent(flow bool) {
	e.indents = append(e.indents, e.indent)
	switch {
	case e.indent < 0 && flow:
		e.indent = bestIndent
	case e.indent < 0:
		e.indent = 0
	default:
		e.indent += bestIndent
	}
}

func (e *emitter) popIndent() {
	e.indent = e.indents[len(e.indents)-1]
	e.indents = e.indents[:len(e.indents)-1]
}

func (e *emitter) writeLineBreak(data string) {
	if data == "" {
		data = lineBreak
	}
	e.whitespace = true
	e.indention = true
	e.column = 0
	e.write(data)
}

func (e *emitter) writeIndent() {
	indent := max(e.indent, 0)
	if !e.indention || e.column > indent || (e.column == indent && !e.whitespace) {
		e.writeLineBreak("")
	}
	if e.column < indent {
		e.whitespace = true
		e.write(strings.Repeat(" ", indent-e.column))
		e.column = indent
	}
}

func (e *emitter) writeIndicator(indicator string, needWhitespace, whitespace, indention bool) {
	data := indicator
	if !e.whitespace && needWhitespace {
		data = " " + indicator
	}
	e.whitespace = whitespace
	e.indention = e.indention && indention
	e.column += runeLen(data)
	e.openEnded = false
	e.write(data)
}

// ---------------------------------------------------------------- analysis

// analysis is PyYAML's `ScalarAnalysis`, minus the fields nothing here reads.
type analysis struct {
	empty             bool
	multiline         bool
	allowBlockPlain   bool
	allowSingleQuoted bool
	allowBlock        bool
}

// analyzeScalar is `Emitter.analyze_scalar`, ported statement for statement.
// `allow_unicode` is true throughout, because `edit.py` dumps with
// `allow_unicode=True` so that a card whose name opens with Æ keeps that
// letter instead of turning it into an escape sequence.
//
// `allow_flow_plain` and `allow_double_quoted` are computed by the original
// and dropped here: the flow level is always zero on this path, and double
// quoting is the fallback that is always permitted.
func analyzeScalar(scalar string) analysis {
	if scalar == "" {
		return analysis{empty: true, allowBlockPlain: true, allowSingleQuoted: true}
	}
	runes := []rune(scalar)
	n := len(runes)

	var (
		blockIndicators   bool
		lineBreaks        bool
		specialCharacters bool

		leadingSpace  bool
		leadingBreak  bool
		trailingSpace bool
		trailingBreak bool
		breakSpace    bool
		spaceBreak    bool
	)

	if strings.HasPrefix(scalar, "---") || strings.HasPrefix(scalar, "...") {
		blockIndicators = true
	}

	precededByWhitespace := true
	followedByWhitespace := n == 1 || containsRune(whitespaceSet, runes[1])
	previousSpace := false
	previousBreak := false

	for index := 0; index < n; {
		ch := runes[index]

		if index == 0 {
			if containsRune("#,[]{}&*!|>'\"%@`", ch) {
				blockIndicators = true
			}
			if (ch == '?' || ch == ':' || ch == '-') && followedByWhitespace {
				blockIndicators = true
			}
		} else {
			if ch == ':' && followedByWhitespace {
				blockIndicators = true
			}
			if ch == '#' && precededByWhitespace {
				blockIndicators = true
			}
		}

		if containsRune(breakSet, ch) {
			lineBreaks = true
		}
		if ch != '\n' && (ch < 0x20 || ch > 0x7E) {
			printable := (ch == 0x85 ||
				(ch >= 0xA0 && ch <= 0xD7FF) ||
				(ch >= 0xE000 && ch <= 0xFFFD) ||
				(ch >= 0x10000 && ch < 0x10FFFF)) && ch != 0xFEFF
			if !printable {
				specialCharacters = true
			}
		}

		switch {
		case ch == ' ':
			if index == 0 {
				leadingSpace = true
			}
			if index == n-1 {
				trailingSpace = true
			}
			if previousBreak {
				breakSpace = true
			}
			previousSpace = true
			previousBreak = false
		case containsRune(breakSet, ch):
			if index == 0 {
				leadingBreak = true
			}
			if index == n-1 {
				trailingBreak = true
			}
			if previousSpace {
				spaceBreak = true
			}
			previousSpace = false
			previousBreak = true
		default:
			previousSpace = false
			previousBreak = false
		}

		index++
		precededByWhitespace = containsRune(whitespaceSet, ch)
		followedByWhitespace = index+1 >= n || containsRune(whitespaceSet, runes[index+1])
	}

	a := analysis{multiline: lineBreaks, allowBlockPlain: true, allowSingleQuoted: true, allowBlock: true}
	if leadingSpace || leadingBreak || trailingSpace || trailingBreak {
		a.allowBlockPlain = false
	}
	if trailingSpace {
		a.allowBlock = false
	}
	if breakSpace {
		a.allowBlockPlain = false
		a.allowSingleQuoted = false
	}
	if spaceBreak || specialCharacters {
		a.allowBlockPlain = false
		a.allowSingleQuoted = false
		a.allowBlock = false
	}
	if lineBreaks {
		a.allowBlockPlain = false
	}
	if blockIndicators {
		a.allowBlockPlain = false
	}
	return a
}

// chooseScalarStyle is `Emitter.choose_scalar_style`, narrowed to the two
// requests this package makes -- no style at all, or `>` for a folded block --
// and to a flow level that is always zero and a `canonical` that is always
// false.
//
// `implicit` is the serialiser's `implicit[0]`: whether the plain form reads
// back as the node's own type. It is the resolver's answer for a string and
// unconditionally true for a number, which is why it arrives as an argument
// rather than being asked here -- `qty: 12` is plain and `why: '12'` is not,
// and the two differ only in what type the node started as.
func chooseScalarStyle(requested scalarStyle, a analysis, implicit, simpleKey bool) scalarStyle {
	if requested == stylePlain && implicit {
		// A key has to fit on one line, so an empty or multi-line scalar
		// cannot be plain in that position however harmless it looks.
		unsafeAsKey := simpleKey && (a.empty || a.multiline)
		if !unsafeAsKey && a.allowBlockPlain {
			return stylePlain
		}
	}
	if requested == styleFolded && !simpleKey && a.allowBlock {
		return styleFolded
	}
	if requested == stylePlain || requested == styleSingleQuoted {
		unsafeAsKey := simpleKey && a.multiline
		if a.allowSingleQuoted && !unsafeAsKey {
			return styleSingleQuoted
		}
	}
	return styleDoubleQuoted
}

// ----------------------------------------------------------------- writers

func determineBlockHints(text string) string {
	if text == "" {
		return ""
	}
	hints := ""
	runes := []rune(text)
	if containsRune(" "+breakSet, runes[0]) {
		hints += strconv.Itoa(bestIndent)
	}
	switch last := runes[len(runes)-1]; {
	case !containsRune(breakSet, last):
		hints += "-"
	case len(runes) == 1 || containsRune(breakSet, runes[len(runes)-2]):
		hints += "+"
	}
	return hints
}

func (e *emitter) writePlain(text string, split bool) {
	if text == "" {
		return
	}
	if !e.whitespace {
		e.column++
		e.write(" ")
	}
	e.whitespace = false
	e.indention = false
	runes := []rune(text)
	n := len(runes)
	spaces, breaks := false, false
	start, end := 0, 0
	for end <= n {
		ch := rune(-1)
		if end < n {
			ch = runes[end]
		}
		switch {
		case spaces:
			if ch != ' ' {
				if start+1 == end && e.column > e.bestWidth && split {
					e.writeIndent()
					e.whitespace = false
					e.indention = false
				} else {
					e.emit(runes[start:end])
				}
				start = end
			}
		case breaks:
			if ch < 0 || !containsRune(breakSet, ch) {
				e.emitBreaks(runes[start:end])
				e.writeIndent()
				e.whitespace = false
				e.indention = false
				start = end
			}
		default:
			if ch < 0 || ch == ' ' || containsRune(breakSet, ch) {
				e.emit(runes[start:end])
				start = end
			}
		}
		if ch >= 0 {
			spaces = ch == ' '
			breaks = containsRune(breakSet, ch)
		}
		end++
	}
}

func (e *emitter) writeFolded(text string) {
	hints := determineBlockHints(text)
	e.writeIndicator(">"+hints, true, false, false)
	if strings.HasSuffix(hints, "+") {
		e.openEnded = true
	}
	e.writeLineBreak("")
	runes := []rune(text)
	n := len(runes)
	leadingSpace, spaces, breaks := true, false, true
	start, end := 0, 0
	for end <= n {
		ch := rune(-1)
		if end < n {
			ch = runes[end]
		}
		switch {
		case breaks:
			if ch < 0 || !containsRune(breakSet, ch) {
				// A single newline folds to a space, so a newline the author
				// meant to keep survives only as the blank line PyYAML writes
				// here. `_render`'s round-trip check is what catches the cases
				// where even that is not faithful.
				if !leadingSpace && ch >= 0 && ch != ' ' && runes[start] == '\n' {
					e.writeLineBreak("")
				}
				leadingSpace = ch == ' '
				for _, br := range runes[start:end] {
					e.writeLineBreak(breakData(br))
				}
				if ch >= 0 {
					e.writeIndent()
				}
				start = end
			}
		case spaces:
			if ch != ' ' {
				if start+1 == end && e.column > e.bestWidth {
					e.writeIndent()
				} else {
					e.emit(runes[start:end])
				}
				start = end
			}
		default:
			if ch < 0 || ch == ' ' || containsRune(breakSet, ch) {
				e.emit(runes[start:end])
				if ch < 0 {
					e.writeLineBreak("")
				}
				start = end
			}
		}
		if ch >= 0 {
			breaks = containsRune(breakSet, ch)
			spaces = ch == ' '
		}
		end++
	}
}

func (e *emitter) writeSingleQuoted(text string, split bool) {
	e.writeIndicator("'", true, false, false)
	runes := []rune(text)
	n := len(runes)
	spaces, breaks := false, false
	start, end := 0, 0
	for end <= n {
		ch := rune(-1)
		if end < n {
			ch = runes[end]
		}
		switch {
		case spaces:
			if ch != ' ' {
				if start+1 == end && e.column > e.bestWidth && split && start != 0 && end != n {
					e.writeIndent()
				} else {
					e.emit(runes[start:end])
				}
				start = end
			}
		case breaks:
			if ch < 0 || !containsRune(breakSet, ch) {
				e.emitBreaks(runes[start:end])
				e.writeIndent()
				start = end
			}
		default:
			if ch < 0 || ch == ' ' || ch == '\'' || containsRune(breakSet, ch) {
				if start < end {
					e.emit(runes[start:end])
					start = end
				}
			}
		}
		if ch == '\'' {
			e.column += 2
			e.write("''")
			start = end + 1
		}
		if ch >= 0 {
			spaces = ch == ' '
			breaks = containsRune(breakSet, ch)
		}
		end++
	}
	e.writeIndicator("'", false, false, false)
}

// escapeReplacements is PyYAML's ESCAPE_REPLACEMENTS.
var escapeReplacements = map[rune]string{
	0x00: "0", 0x07: "a", 0x08: "b", 0x09: "t", 0x0A: "n", 0x0B: "v",
	0x0C: "f", 0x0D: "r", 0x1B: "e", '"': `"`, '\\': `\`,
	0x85: "N", 0xA0: "_", 0x2028: "L", 0x2029: "P",
}

func (e *emitter) writeDoubleQuoted(text string, split bool) {
	e.writeIndicator(`"`, true, false, false)
	runes := []rune(text)
	n := len(runes)
	start, end := 0, 0
	for end <= n {
		ch := rune(-1)
		if end < n {
			ch = runes[end]
		}
		printable := (ch >= 0x20 && ch <= 0x7E) ||
			(ch >= 0xA0 && ch <= 0xD7FF) || (ch >= 0xE000 && ch <= 0xFFFD)
		mustEscape := ch < 0 || ch == '"' || ch == '\\' || ch == 0xFEFF ||
			ch == 0x85 || ch == 0x2028 || ch == 0x2029 || !printable
		if mustEscape {
			if start < end {
				e.emit(runes[start:end])
				start = end
			}
			if ch >= 0 {
				data := escapeFor(ch)
				e.column += runeLen(data)
				e.write(data)
				start = end + 1
			}
		}
		if 0 < end && end < n-1 && (ch == ' ' || start >= end) &&
			e.column+(end-start) > e.bestWidth && split {
			data := string(runes[start:end]) + `\`
			if start < end {
				start = end
			}
			e.column += runeLen(data)
			e.write(data)
			e.writeIndent()
			e.whitespace = false
			e.indention = false
			if runes[start] == ' ' {
				e.column++
				e.write(`\`)
			}
		}
		end++
	}
	e.writeIndicator(`"`, false, false, false)
}

func escapeFor(ch rune) string {
	if replacement, ok := escapeReplacements[ch]; ok {
		return `\` + replacement
	}
	switch {
	case ch <= 0xFF:
		return fmt.Sprintf(`\x%02X`, ch)
	case ch <= 0xFFFF:
		return fmt.Sprintf(`\u%04X`, ch)
	default:
		return fmt.Sprintf(`\U%08X`, ch)
	}
}

// emit writes a run of runes and advances the column by its length in code
// points, which is the pairing every writer above repeats.
func (e *emitter) emit(runes []rune) {
	data := string(runes)
	e.column += len(runes)
	e.write(data)
}

// emitBreaks is the run `write_plain` and `write_single_quoted` share: a
// leading `\n` writes one extra break, because a single newline folds away.
func (e *emitter) emitBreaks(runes []rune) {
	if len(runes) > 0 && runes[0] == '\n' {
		e.writeLineBreak("")
	}
	for _, br := range runes {
		e.writeLineBreak(breakData(br))
	}
}

// breakData maps a break character to what `write_line_break` is handed: the
// default for `\n`, the character itself for NEL and the separators.
func breakData(br rune) string {
	if br == '\n' {
		return ""
	}
	return string(br)
}

// processScalar is `Emitter.process_scalar`.
func (e *emitter) processScalar(value string, requested scalarStyle, implicit, simpleKey bool) {
	a := analyzeScalar(value)
	switch chooseScalarStyle(requested, a, implicit, simpleKey) {
	case styleDoubleQuoted:
		e.writeDoubleQuoted(value, !simpleKey)
	case styleSingleQuoted:
		e.writeSingleQuoted(value, !simpleKey)
	case styleFolded:
		e.writeFolded(value)
	default:
		e.writePlain(value, !simpleKey)
	}
}
