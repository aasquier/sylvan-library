package textutil

import "testing"

// The storyteller's first question arrived with `—` in it and the page
// printed six characters of punctuation nobody wrote. These pin the repair:
// a well-formed escape becomes its character, a pair becomes one rune, and
// everything that is not a well-formed `\u` escape is left exactly alone.
func TestUnescapeTurnsASurvivingEscapeBackIntoItsCharacter(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`which story this deck wants to be — so tell me`: "which story this deck wants to be — so tell me",
		`été`:                  "été",
		`a 🔥 fire`:             "a 🔥 fire",
		`\ud83d alone`:         `\ud83d alone`,
		`\u12G4 not hex`:       `\u12G4 not hex`,
		`\u12`:                 `\u12`,
		`a newline \n stays`:   `a newline \n stays`,
		`a backslash \ stays`:  `a backslash \ stays`,
		"nothing to do":        "nothing to do",
		"":                     "",
		"already — an em dash": "already — an em dash",
	}
	for in, want := range cases {
		if got := Unescape(in); got != want {
			t.Errorf("Unescape(%q) = %q, want %q", in, got, want)
		}
	}
}
