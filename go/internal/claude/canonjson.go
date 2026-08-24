package claude

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The canonical JSON dialect, for the two places this package hashes bytes
// or hands the model a rendered document.
//
// **The dossier's cache key hashes the sorted-key rendering of
// RESPONSE_SCHEMA**, and the key is how a stored dossier is found and
// served -- so "equivalent JSON" is not equivalent here at
// all; the bytes are the key. Three things `encoding/json` would get wrong:
// it escapes `<`, `>` and `&` where Python does not; it writes non-ASCII as
// raw UTF-8 where `ensure_ascii=True` writes `\uXXXX` with a surrogate pair
// above the BMP; and with `sort_keys=True` Python's item separator is `, `
// and its key separator `: `, where `encoding/json` writes neither space.
//
// **The brief's opening message is `json.dumps(facts, indent=2,
// default=str)`**, and that one goes to the model rather than into a digest.
// The two runtimes never share a conversation or a prompt cache, so the
// bytes there do not have to agree -- but reproducing them costs the same
// renderer a second option, and buys a corpus row that pins the whole brief
// against Python's, pool facts and all, rather than its key order alone.
//
// The writer is a transcription of `json.encoder` with the set of inputs
// closed on purpose: a string, a bool, nil, an integer, a pointer to any of
// those (`null` when nil), an ordered map, a string-keyed Go map (sorted or
// refused -- a Go map has no order to reproduce), and slices of any of the
// above. **Floats are refused.** Neither input carries one today, and a
// float would need CPython's `repr`, which lives in `sim/tier1` and is not
// this package's to reach for; a renderer that silently wrote `%g` would be
// a wrong key that looked like a key. `sim/cache` holds the first
// transcription of this encoder (compact separators, for ADR 18's key) and
// is private to that package; this is the second, and the package comment
// there is the argument for why a key's bytes are written out by hand.

// dumpOptions selects between Python's two spellings used here.
type dumpOptions struct {
	// Indent is `indent=N`; zero is `indent=None`. With an indent Python's
	// item separator becomes `,` plus a newline, and without one it is `, `.
	Indent int
	// SortKeys is `sort_keys=True`. Required for a Go map, which has no
	// order of its own; an ordered map is written in its order either way.
	SortKeys bool
}

// dumpJSON renders v as Python would. It panics on a value outside the closed
// set above, because every caller hashes or sends the result and a plausible
// rendering of an unknown type is worse than a crash.
func dumpJSON(v any, opt dumpOptions) string {
	var b strings.Builder
	writeValue(&b, v, opt, 0)
	return b.String()
}

func writeValue(b *strings.Builder, v any, opt dumpOptions, level int) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
		return
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		return
	case string:
		writeJSONString(b, x)
		return
	case int:
		b.WriteString(strconv.Itoa(x))
		return
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
		return
	case wire.OrderedMap:
		writeObject(b, x, opt, level)
		return
	case []wire.KV:
		writeObject(b, x, opt, level)
		return
	case map[string]any:
		if !opt.SortKeys {
			panic("claude: dumpJSON was handed a Go map without sort_keys; a " +
				"map has no insertion order to reproduce, so use wire.OrderedMap")
		}
		names := make([]string, 0, len(x))
		for name := range x {
			names = append(names, name)
		}
		// `sort_keys=True` sorts by code point; Go compares bytes, and for
		// valid UTF-8 the two orders are the same.
		sort.Strings(names)
		pairs := make([]wire.KV, 0, len(names))
		for _, name := range names {
			pairs = append(pairs, wire.KV{Key: name, Value: x[name]})
		}
		writeObject(b, pairs, opt, level)
		return
	case float64, float32:
		panic("claude: dumpJSON was handed a float; CPython's repr is not " +
			"reproduced here, and nothing this package renders carries one")
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			b.WriteString("null")
			return
		}
		writeValue(b, rv.Elem().Interface(), opt, level)
	case reflect.Slice, reflect.Array:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			// A nil slice is Python's `[]` here, never `null`: the brief's
			// lists are built by appending to an empty one.
			b.WriteString("[]")
			return
		}
		n := rv.Len()
		if n == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i := 0; i < n; i++ {
			if i > 0 {
				writeItemSep(b, opt)
			}
			writeNewline(b, opt, level+1)
			writeValue(b, rv.Index(i).Interface(), opt, level+1)
		}
		writeNewline(b, opt, level)
		b.WriteByte(']')
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		b.WriteString(strconv.FormatInt(rv.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.WriteString(strconv.FormatUint(rv.Uint(), 10))
	default:
		panic(fmt.Sprintf("claude: dumpJSON has no Python rendering for a %T", v))
	}
}

func writeObject(b *strings.Builder, pairs []wire.KV, opt dumpOptions, level int) {
	if len(pairs) == 0 {
		b.WriteString("{}")
		return
	}
	b.WriteByte('{')
	for i, kv := range pairs {
		if i > 0 {
			writeItemSep(b, opt)
		}
		writeNewline(b, opt, level+1)
		writeJSONString(b, kv.Key)
		b.WriteString(": ")
		writeValue(b, kv.Value, opt, level+1)
	}
	writeNewline(b, opt, level)
	b.WriteByte('}')
}

// writeItemSep is the item separator: `,` with an indent (the newline
// follows from writeNewline), `, ` without.
func writeItemSep(b *strings.Builder, opt dumpOptions) {
	b.WriteByte(',')
	if opt.Indent == 0 {
		b.WriteByte(' ')
	}
}

func writeNewline(b *strings.Builder, opt dumpOptions, level int) {
	if opt.Indent == 0 {
		return
	}
	b.WriteByte('\n')
	b.WriteString(strings.Repeat(" ", opt.Indent*level))
}

// writeJSONString is `json.encoder.encode_basestring_ascii`: `"` and `\`
// escaped, the five short forms, everything below 0x20 and everything
// **above 0x7e** as `\uXXXX` in lower-case hex, a rune above the BMP as a
// UTF-16 surrogate pair. `/`, `<`, `>` and `&` are not escaped. Invalid
// UTF-8 decodes to U+FFFD, which a Python `str` cannot hold at all -- there
// is no faithful answer, and the replacement character is a stable one.
func writeJSONString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || r > 0x7e:
			if r > 0xffff {
				hi, lo := utf16.EncodeRune(r)
				writeHex4(b, hi)
				writeHex4(b, lo)
			} else {
				writeHex4(b, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

// writeHex4 writes the low sixteen bits of v as `\uXXXX`; every caller
// hands it a BMP code point or one half of a surrogate pair, both of which
// fit.
func writeHex4(b *strings.Builder, v rune) {
	const digits = "0123456789abcdef"
	b.WriteString(`\u`)
	b.WriteByte(digits[(v>>12)&0xf])
	b.WriteByte(digits[(v>>8)&0xf])
	b.WriteByte(digits[(v>>4)&0xf])
	b.WriteByte(digits[v&0xf])
}
