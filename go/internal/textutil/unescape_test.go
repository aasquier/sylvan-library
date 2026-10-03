package textutil

import "testing"

// esc spells one `\uXXXX` escape without this file ever holding the two
// characters that begin one.
//
// That is not fussiness, it is the bug this test was rewritten for. The
// original cases were written as `—`, `é` and a surrogate pair — and
// what landed in the file was an em dash, two `é`s and a fire, because
// something between the keyboard and the commit read the escapes and obliged.
// Three cases then asked whether a string with no `\u` in it survives the early
// return, which it does, and the comment over them claimed they proved the
// decode. Assembling the escape from a backslash and a `u` cannot be decoded by
// anything on the way in.
func esc(hex string) string { return `\` + "u" + hex }

// A well-formed escape becomes its character, a surrogate pair becomes one
// rune, and everything that is not a well-formed `\u` escape is left exactly
// alone.
func TestUnescapeTurnsASurvivingEscapeBackIntoItsCharacter(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		// The real thing: the storyteller's first question, as it arrived.
		{"which story this deck wants to be " + esc("2014") + " so tell me",
			"which story this deck wants to be — so tell me"},
		{esc("00e9") + "t" + esc("00e9"), "été"},
		// Uppercase hex names the same code point; `É` is the capital.
		{esc("00C9") + "t" + esc("00E9"), "Été"},
		// A surrogate pair is one character, and the fire is the reason this
		// function exists in an app about cards people set alight.
		{"a " + esc("d83d") + esc("dd25") + " fire", "a 🔥 fire"},
		// The three ways a surrogate can fail to be a character. Each stays the
		// text it was rather than becoming U+FFFD: a high half with nothing
		// after it, a low half with nothing before it, and two highs in a row.
		{esc("d83d") + " alone", esc("d83d") + " alone"},
		{esc("dd25") + " alone", esc("dd25") + " alone"},
		{esc("d83d") + esc("d83d") + " twin", esc("d83d") + esc("d83d") + " twin"},
		// A high half followed by a perfectly good escape that is not its other
		// half: the half stays as text, the escape decodes.
		{esc("d83d") + esc("0041"), esc("d83d") + "A"},
		// Not well formed, so not this function's business.
		{esc("12G4") + " not hex", esc("12G4") + " not hex"},
		{esc("12"), esc("12")},
		{`a newline \n stays`, `a newline \n stays`},
		{`a backslash \ stays`, `a backslash \ stays`},
		// Nothing to do: the early return, which is the common case.
		{"nothing to do", "nothing to do"},
		{"", ""},
		{"already — an em dash", "already — an em dash"},
		{"a 🔥 already lit", "a 🔥 already lit"},
	}
	for _, tc := range cases {
		if got := Unescape(tc.in); got != tc.want {
			t.Errorf("Unescape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Head cuts on a code-point boundary, counted rather than measured in bytes.
//
// The cases that matter are the ones the old shape's unreachable trailing
// return stood in for: n past the end, n exactly the length, and a string whose
// runes are wider than one byte, where a byte-counted cut would split a
// character in half.
func TestHeadCutsOnARuneBoundary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"Syr Gwyn", 3, "Syr"},
		{"Syr Gwyn", 8, "Syr Gwyn"},
		{"Syr Gwyn", 99, "Syr Gwyn"},
		{"Syr Gwyn", 0, ""},
		{"", 4, ""},
		{"été", 2, "ét"},
		{"été", 3, "été"},
		{"🔥🔥🔥", 1, "🔥"},
		{"🔥🔥🔥", 3, "🔥🔥🔥"},
		{"a🔥b", 2, "a🔥"},
	}
	for _, tc := range cases {
		if got := Head(tc.in, tc.n); got != tc.want {
			t.Errorf("Head(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
		}
	}
}
