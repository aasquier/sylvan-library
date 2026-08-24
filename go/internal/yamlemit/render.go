package yamlemit

import (
	"fmt"
	"strconv"

	"github.com/aasquier/sylvan-library/go/internal/deckyaml"
)

// Render writes one `key: value` pair as the recorded YAML lines for it, at
// the given indent.
//
// The scalar rules are delegated to the emitter rather than hand-rolled: a
// `why` containing a colon, a leading
// `>` or a trailing space each need different quoting, and getting one wrong
// writes a file that no longer parses.
//
// `fold` asks for the block style the deck files use for prose. It is a
// request, not an instruction: folding collapses single newlines and adjusts
// the trailing one, so for some strings it is not value-preserving. When the
// folded form would not read back as what was passed in, this falls back to
// the emitter's own choice, which always does. That fallback is why this
// function parses its own output -- the check is part of the recorded
// contract, kept here rather
// than moved somewhere tidier, because it is the only thing standing between
// a trailing space and a silently truncated rationale.
func Render(key string, value any, indent int, width int, fold bool) ([]string, error) {
	// A fold request wraps the string, so the request travels with the value.
	payload := value
	if fold {
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("yamlemit: only a string folds, not %T", value)
		}
		payload = folded(text)
	}
	text, err := dump(key, payload, max(20, width-indent))
	if err != nil {
		return nil, err
	}
	if fold {
		ok, err := roundTrips(text, key, value)
		if err != nil {
			return nil, err
		}
		if !ok {
			return Render(key, value, indent, width, false)
		}
	}
	pad := ""
	if indent > 0 {
		pad = spaces(indent)
	}
	var out []string
	for _, line := range splitLines(rstripNewlines(text)) {
		if line == "" {
			out = append(out, line)
			continue
		}
		out = append(out, pad+line)
	}
	return out, nil
}

// RenderWidth is Render's default width: 96 columns, which is where the deck
// files' own prose sits.
const RenderWidth = 96

// ProseWidth is the width used for rationale prose. The emitter treats
// `width` as the column it
// breaks *after*, so emitted lines run several characters wider than the
// number given. The hand-written notes in the deck files top out at 79
// columns; 72 lands under that, and it matters because a note is prose sitting
// next to other prose that nothing is allowed to reflow.
const ProseWidth = 72

// dump writes one `{key: value}` mapping in the recorded style,
// for the three value shapes a deck file's keys take: a scalar the caller
// wants folded, an ordinary scalar, and a list of scalars.
//
// The event sequence is walked by hand rather than driven by a
// serialiser, because a one-key mapping reaches only these states and the
// indent arithmetic is the part that has to be exact.
func dump(key string, value any, width int) (string, error) {
	e := newEmitter(width)

	// The mapping opens: one indent level, from "unset" to column 0.
	e.increaseIndent(false)

	// Then the key, written as a simple key.
	e.writeIndent()
	e.increaseIndent(true)
	e.processScalar(key, stylePlain, resolvesToString(key), true)
	e.popIndent()

	// The value follows on the same line, after the `:`.
	e.writeIndicator(":", false, false, false)

	switch v := value.(type) {
	case folded:
		e.increaseIndent(true)
		e.processScalar(string(v), styleFolded, resolvesToString(string(v)), false)
		e.popIndent()
	case []string:
		if len(v) == 0 {
			// An empty list goes to the flow writer,
			// so `themes: []` and not a block with nothing under it.
			e.writeIndicator("[", true, true, false)
			e.writeIndicator("]", false, false, false)
			break
		}
		// The sequence opens. `increaseIndent` never goes indentless,
		// which is what puts the dashes under the key.
		e.increaseIndent(false)
		for _, item := range v {
			e.writeIndent()
			e.writeIndicator("-", true, false, true)
			e.increaseIndent(true)
			e.processScalar(item, stylePlain, resolvesToString(item), false)
			e.popIndent()
		}
		e.popIndent()
	default:
		scalar, err := scalarOf(value)
		if err != nil {
			return "", err
		}
		e.increaseIndent(true)
		e.processScalar(scalar.text, stylePlain, scalar.implicit, false)
		e.popIndent()
	}

	// The mapping ends, then the document: the document's end writes one
	// last indent, which is the trailing newline every dump carries.
	e.popIndent()
	e.writeIndent()
	// ... and then the stream ends, which is where a `>+` block gets the
	// explicit `...` that says the scalar is over.
	if e.openEnded {
		e.writeIndicator("...", true, false, false)
		e.writeIndent()
	}
	return e.out.String(), nil
}

// folded marks a string the caller wants written as a `>` block: a wrapper
// type, so the request travels with the value all the way into `dump`.
type folded string

type scalar struct {
	text string
	// implicit is whether reading the plain form
	// back gives a value of the node's own type. For a string it is the
	// resolver's question -- the word "yes" is a boolean, so a `why` of "yes"
	// must be quoted. For an int or a bool it is always true, because the
	// canonical spelling of either always reads back as itself.
	implicit bool
}

func scalarOf(value any) (scalar, error) {
	switch v := value.(type) {
	case string:
		return scalar{text: v, implicit: resolvesToString(v)}, nil
	case int:
		return scalar{text: strconv.Itoa(v), implicit: true}, nil
	case int64:
		return scalar{text: strconv.FormatInt(v, 10), implicit: true}, nil
	case bool:
		if v {
			return scalar{text: "true", implicit: true}, nil
		}
		return scalar{text: "false", implicit: true}, nil
	default:
		return scalar{}, fmt.Errorf("yamlemit: cannot render %T", value)
	}
}

// Folded wraps a string the caller wants written as a `>` block, for the one
// caller that needs to ask for it explicitly rather than through `Render`'s
// `fold` argument.
func Folded(s string) any { return folded(s) }

// roundTrips answers whether the folded
// form still reads back as what was handed in.
func roundTrips(text, key string, value any) (bool, error) {
	doc, err := deckyaml.Parse([]byte(text))
	if err != nil {
		// Unparseable is the strongest possible "no". A parse failure inside
		// the round-trip check is answered as "fall back", which is both safe
		// and what the next attempt will prove or refuse.
		return false, nil //nolint:nilerr // an unreadable fold is a failed fold
	}
	if len(doc) != 1 {
		return false, nil
	}
	got, ok := doc[key]
	if !ok {
		return false, nil
	}
	want, err := scalarOf(value)
	if err != nil {
		return false, err
	}
	// Every folded value is a string; the parse gives one back or the fold
	// changed the type, which is a failed fold either way.
	text, isString := got.(string)
	if !isString {
		return false, nil
	}
	return text == want.text, nil
}

func spaces(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = ' '
	}
	return string(out)
}

func rstripNewlines(s string) string {
	end := len(s)
	for end > 0 && s[end-1] == '\n' {
		end--
	}
	return s[:end]
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}
