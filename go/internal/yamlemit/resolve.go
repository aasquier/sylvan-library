package yamlemit

import "regexp"

// The recorded style's implicit resolver table, verbatim in every
// way that can change an answer.
//
// It is here because of what `chooseScalarStyle` does with it: a plain
// scalar is only allowed when the value would read back as the *same type*,
// and that question is this table's. A card whose `why` is the word "yes"
// must be written `'yes'`, because plain `yes` is a boolean in YAML 1.1 --
// the version the recorded style resolves under and therefore the version
// this table implements too, however dated the rule looks.
//
// Two details are load-bearing and easy to lose in transcription. The null
// pattern really does end in an **empty alternative** (the `|` just before
// `)$`): that is why an empty string resolves to null, and it is a recorded
// rule rather than a typo to tidy. And the table is indexed by the
// value's **first character** -- a value whose first character indexes no
// list is resolved against nothing at all, so `regexp.MatchString` is asked
// only after that lookup, never instead of it.
//
// The float pattern deliberately requires a sign in the exponent
// (`[eE][-+][0-9]+`). `1e3` is therefore a *string* and must not be
// quoted; `1.0e+3` is a float and must be. That is YAML 1.1, and re-deriving
// it "sensibly" would quote different values than the frozen corpus quotes.
var implicitResolvers = []struct {
	first string
	re    *regexp.Regexp
}{
	{"yYnNtTfFoO", regexp.MustCompile(
		`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$`)},
	{"-+0123456789.", regexp.MustCompile(
		`^(?:[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?` +
			`|\.[0-9][0-9_]*(?:[eE][-+][0-9]+)?` +
			`|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*` +
			`|[-+]?\.(?:inf|Inf|INF)` +
			`|\.(?:nan|NaN|NAN))$`)},
	{"-+0123456789", regexp.MustCompile(
		`^(?:[-+]?0b[0-1_]+` +
			`|[-+]?0[0-7_]+` +
			`|[-+]?(?:0|[1-9][0-9_]*)` +
			`|[-+]?0x[0-9a-fA-F_]+` +
			`|[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+)$`)},
	{"<", regexp.MustCompile(`^(?:<<)$`)},
	// The empty first-character slot is real: `resolve` looks the empty string
	// up when the value is empty, and this is the entry it finds.
	{"~nN", regexp.MustCompile(`^(?:~|null|Null|NULL|)$`)},
	{"0123456789", regexp.MustCompile(
		`^(?:[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]` +
			`|[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?` +
			`(?:[Tt]|[ \t]+)[0-9][0-9]?` +
			`:[0-9][0-9]:[0-9][0-9](?:\.[0-9]*)?` +
			`(?:[ \t]*(?:Z|[-+][0-9][0-9]?(?::[0-9][0-9])?))?)$`)},
	{"=", regexp.MustCompile(`^(?:=)$`)},
	{"!&*", regexp.MustCompile(`^(?:!|&|\*)$`)},
}

// resolvesToString reports whether writing `value` as a plain scalar would
// read back as a string -- the implicit answer `chooseScalarStyle`
// consults before it will consider the plain style at all.
//
// The null entry's first-character list really holds a fourth, *empty* slot
// beside `~`, `n` and `N`. That
// slot is only ever reached by an empty value -- a non-empty one is
// looked up by its first character, which is never the empty string -- so it
// is answered before the loop rather than spelled as a rune.
func resolvesToString(value string) bool {
	if value == "" {
		// The empty value looks up the empty first-character slot, which is
		// the null entry, whose pattern matches the empty string.
		return false
	}
	first := []rune(value)[0]
	for _, entry := range implicitResolvers {
		if !containsRune(entry.first, first) {
			continue
		}
		if entry.re.MatchString(value) {
			return false
		}
	}
	return true
}

func containsRune(set string, ch rune) bool {
	for _, r := range set {
		if r == ch {
			return true
		}
	}
	return false
}
