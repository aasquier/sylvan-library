package claude

import "testing"

// The storyteller's first question reached the page with `—` printed
// as six characters (2026-10-03): the model wrote the escape inside the
// field's text, and the wire's one decode had already happened. `Prose` is
// the one cleaner every prose field passes through, so the repair lives
// there and this pins it there — at the entry, before the control-character
// sweep, so the sweep sees characters rather than escapes.
func TestProseRepairsAnEscapeTheModelWroteInsideItsText(t *testing.T) {
	t.Parallel()
	got := Prose(`which story this deck wants to be — so tell me`)
	want := "which story this deck wants to be — so tell me"
	if got != want {
		t.Fatalf("Prose() = %q, want %q", got, want)
	}
	// And a text with nothing to repair is the text it was.
	if got := Prose("plain words, kept"); got != "plain words, kept" {
		t.Fatalf("Prose() rewrote plain text: %q", got)
	}
}
