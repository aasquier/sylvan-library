package yamlemit

// The whole-document dump: the recorded style over an entire payload, which
// is how a whole deck file is written.
//
// `Render` next door writes one `key: value` pair, because that is all
// the edit engine's text surgery ever needs -- it rewrites the lines it
// changes and
// leaves the rest of the file alone. This writes the *whole* file, and there
// are exactly three callers for that: a deck
// being created, a deck being imported, and `set_shared` on the file tier,
// which re-dumps rather than editing in place (a single boolean has no
// comment to destroy).
//
// Same rule as the emitter it drives: this is a reproduction, not a YAML
// writer. A created deck's file is the first thing its owner reads and the
// baseline every later `swaps.md` diffs against, so "valid YAML" is not the
// bar -- the recorded bytes are. `testdata/documents.json` is the corpus
// that says so.
//
// The node model is three types because Go maps have no order and the
// recorded style never sorts keys -- the payload's order *is* the file's
// order: Map is
// an ordered mapping, List a sequence, and everything else a scalar
// `scalarOf` understands.

// Pair is one key and its value inside a Map.
type Pair struct {
	Key   string
	Value any
}

// Map is a block mapping in the order it was built, which is the order it is
// written in.
type Map []Pair

// List is a block sequence. An empty one is written `[]`, in the flow style
// -- the recorded rule, and it is why a deck
// with no commander keeps its `commander: []` line instead of growing a
// block with nothing under it.
type List []any

// Dump writes a document at the given width,
// counted in code points like every column here.
func Dump(root any, width int) (string, error) {
	e := newEmitter(width)
	if err := e.node(root, false); err != nil {
		return "", err
	}
	// The document's end writes one last indent, which is the trailing
	// newline every dump carries; then the stream ends, and a `>+` block that
	// left the document open gets the explicit `...` that says the scalar is
	// over.
	e.writeIndent()
	if e.openEnded {
		e.writeIndicator("...", true, false, false)
		e.writeIndent()
	}
	return e.out.String(), nil
}

// node dispatches one value: a collection opens a block (or a flow pair,
// when it
// is empty), and a scalar is written between an indent push and pop.
//
// `simpleKey` says the node sits in key position, and only a mapping key
// ever does. It reaches `processScalar` as "do not split",
// which is what stops a key being wrapped onto a second line.
func (e *emitter) node(value any, simpleKey bool) error {
	switch v := value.(type) {
	case Map:
		if len(v) == 0 {
			// An empty mapping goes to the flow writer.
			e.writeIndicator("{", true, true, false)
			e.writeIndicator("}", false, false, false)
			return nil
		}
		return e.blockMapping(v)
	case List:
		if len(v) == 0 {
			e.writeIndicator("[", true, true, false)
			e.writeIndicator("]", false, false, false)
			return nil
		}
		return e.blockSequence(v)
	case folded:
		e.increaseIndent(true)
		e.processScalar(string(v), styleFolded, resolvesToString(string(v)), simpleKey)
		e.popIndent()
		return nil
	default:
		s, err := scalarOf(value)
		if err != nil {
			return err
		}
		e.increaseIndent(true)
		e.processScalar(s.text, stylePlain, s.implicit, simpleKey)
		e.popIndent()
		return nil
	}
}

// blockMapping writes a block mapping: each key in its simple or long form,
// then its value.
func (e *emitter) blockMapping(m Map) error {
	e.increaseIndent(false)
	for _, p := range m {
		e.writeIndent()
		if simpleEnoughForAKey(p.Key) {
			if err := e.node(p.Key, true); err != nil {
				return err
			}
			// The value follows on the same line, after the `:`.
			e.writeIndicator(":", false, false, false)
		} else {
			// A key that cannot sit on one line is written the long way, with
			// its own `?` and `:` lines. No deck field reaches this -- the
			// keys are `slug`, `name`, `cards` and their kin -- but a note's
			// key comes from a URL segment (`PUT .../notes/{key}`), so the
			// state is reachable by somebody rather than only by a fuzzer.
			e.writeIndicator("?", true, false, true)
			if err := e.node(p.Key, false); err != nil {
				return err
			}
			// The value gets its own line under the `:`.
			e.writeIndent()
			e.writeIndicator(":", true, false, true)
		}
		if err := e.node(p.Value, false); err != nil {
			return err
		}
	}
	e.popIndent()
	return nil
}

// blockSequence writes a block sequence. `increaseIndent` never goes
// indentless, which is what puts the dashes under their key
// instead of hard against the margin.
func (e *emitter) blockSequence(items List) error {
	e.increaseIndent(false)
	for _, item := range items {
		e.writeIndent()
		e.writeIndicator("-", true, false, true)
		if err := e.node(item, false); err != nil {
			return err
		}
	}
	e.popIndent()
	return nil
}

// simpleEnoughForAKey is the simple-key rule for the one kind of key a
// document here can hold: a scalar string. The full rule also counts an
// anchor and a
// tag into the length and admits an empty collection as a key; neither is
// reachable from a payload built out of Map, List and scalars.
func simpleEnoughForAKey(key string) bool {
	a := analyzeScalar(key)
	return runeLen(key) < 128 && !a.empty && !a.multiline
}
