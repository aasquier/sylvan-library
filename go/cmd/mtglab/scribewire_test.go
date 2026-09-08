package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The scribe's wire is ASCII, and it is ASCII on purpose.
//
// # The fault this stands on
//
// `Scribe.java` reports a Forge match by writing JSON lines to a stream, and a
// Java `PrintStream` encodes with **`stdout.encoding`** -- which is not
// `file.encoding`, and which JEP 400 did not change. Measured on the Forge
// worker on 2026-09-07, an image that sets no `LANG` at all:
//
//	file.encoding   = UTF-8
//	stdout.encoding = ANSI_X3.4-1968      <- US-ASCII
//
// A US-ASCII encoder does not fail on a character it cannot carry. It
// substitutes `?`. So `Mjölnir, Storm Hammer` left the worker as
// `Mj?lnir, Storm Hammer`, missed the card pool's exact lower-cased-name
// lookup, and drew a plate with no painting on it. **Ninety-seven cards in the
// pool have a non-ASCII name** -- Lórien Revealed, Barad-dûr, Andúril, Nazgûl,
// Éowyn, Palantír of Orthanc -- and every one of them was blank in every match
// the Coliseum ever played. Aaron saw it and said where: "on the board during
// a match", 2026-09-07.
//
// **The substitution is lossy, which is the whole reason this guard is here
// rather than a fallback being there.** Nothing downstream can turn `Mj?lnir`
// back into a name; a fold would be guessing, and guessing at a card name is
// the one thing this project's first non-negotiable forbids. It has to not
// happen at the source.
//
// # Why a guard rather than a test
//
// `scribe/` has no test harness and deliberately no build system -- three
// files and one `javac` call, argued in `build.sh` -- and CI compiles it only
// as a Docker stage, which cannot run on the maintainer's Mac. So the two
// lines that hold the wire ASCII have no unit test standing over them and
// would regress in silence, on a surface nobody can exercise locally, into an
// image whose first feedback is a red `image` job. This reads them instead.
//
// It is a guard on *source text*, which is the weaker kind of test and is
// chosen knowing that: it can only prove the two decisions are still written
// down, never that they still work. `TestACardNamedOutsideASCIIKeepsItsName`
// in `internal/sim/tier3` holds the other end -- that a `\uXXXX` name arrives
// on the board intact -- and the pair is what the fix rests on.
func TestTheScribesWireIsASCIIOnly(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)

	t.Run("the JSON writer escapes above 0x7e", func(t *testing.T) {
		t.Parallel()
		body := readJava(t, filepath.Join(root, "scribe", "src", "scribe", "Json.java"))
		quote := methodBody(t, body, "private void quote(String s)")
		// The `default:` arm is the one that decides what happens to a
		// character with no named escape -- which is every accented letter in
		// every card name Wizards ever printed.
		if !regexp.MustCompile(`c\s*>\s*0x7e|c\s*>=\s*0x7f`).MatchString(quote) {
			t.Errorf("`Json.quote` no longer escapes above 0x7e.\n\n"+
				"Without it every non-ASCII card name leaves the worker as `?`\n"+
				"and draws a blank plate on the Coliseum board -- 97 cards in\n"+
				"the pool, Lórien Revealed and Barad-dûr among them. The wire\n"+
				"must be ASCII because `stdout.encoding` on the worker is\n"+
				"US-ASCII and its encoder substitutes rather than fails.\n\n"+
				"the method now reads:\n%s", quote)
		}
	})

	t.Run("the stream never falls back to a bare System.out", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(root, "scribe", "src", "scribe", "Main.java")
		body := readJava(t, path)
		if regexp.MustCompile(`PrintStream\s+\w+\s*=\s*System\.out\s*;`).MatchString(body) {
			t.Errorf("Main.java hands the scribe a bare `System.out`.\n\n" +
				"That stream encodes with `stdout.encoding`, which measured\n" +
				"ANSI_X3.4-1968 on the worker while `file.encoding` said UTF-8.\n" +
				"Build it with an explicit charset instead.")
		}
		if !strings.Contains(body, "StandardCharsets.UTF_8") {
			t.Errorf("Main.java no longer names a charset for the scribe's stream.\n\n" +
				"`new PrintStream(out, true, StandardCharsets.UTF_8)` is the\n" +
				"belt to `Json`'s braces: it keeps anything written through the\n" +
				"stream honest even if it never passed the escaper.")
		}
	})
}

// readJava is one of the scribe's sources, or a failure that says which.
func readJava(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the scribe's source is not where this guard looks: %v", err)
	}
	return string(body)
}

// methodBody is the text between a signature and the brace that closes it.
//
// Brace-counted rather than regexped to the end of the file: the point of this
// guard is that the escape lives in *this* method, and a search over the whole
// source would be satisfied by the same characters appearing in the comment
// that explains them -- which, in that file, they do.
func methodBody(t *testing.T, body, signature string) string {
	t.Helper()
	start := strings.Index(body, signature)
	if start < 0 {
		t.Fatalf("`%s` is no longer in the file this guard reads.\n"+
			"If it was renamed, rename it here; if it was deleted, the wire\n"+
			"has no escaper and the Coliseum board has blank cards again.",
			signature)
	}
	rest := body[start+len(signature):]
	depth := 0
	for i, r := range rest {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[:i+1]
			}
		}
	}
	t.Fatalf("`%s` never closes its brace", signature)
	return ""
}
