package tier3

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The JVM hunt, asked about the machines it refuses rather than the one it
// takes.
//
// Three answers it can give and only one of them had ever run: a JVM that is
// there and too old, a JVM that answers in a format nobody can read, and a JVM
// found on the path rather than named outright. The first two end in the same
// sentence and must not read the same way inside it — a reader of
// `Checked: /usr/bin/java (Java 11)` knows to install a newer one, and
// `Checked: /usr/bin/java (Java None)` says the thing did not answer at all.
//
// The stand-in JVMs are committed files (`testdata/fakejava-*`), never written
// by a test: `testdata/fakejava`'s own comment carries the argument, and both
// faults a minted executable produces surface here as `Java None`, which is one
// of the two answers under test.

// aJVM is the absolute path of one of the committed stand-ins, which is what
// [Settings.Java] holds on a real machine.
func aJVM(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// A JVM that is present and too old is refused *by its version*, which is the
// difference between "install a newer Java" and "this is not a Java".
func TestAJVMTooOldForForgeIsRefusedByItsVersion(t *testing.T) {
	t.Parallel()
	java := aJVM(t, "fakejava-ancient")
	found, err := (Settings{Java: java}).JavaBinary()
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Fatalf("an old JVM answered %q, %v", found, err)
	}
	if !strings.Contains(err.Error(), "(Java 11)") {
		t.Errorf("the refusal does not say which Java it found: %v", err)
	}
	if strings.Contains(err.Error(), "Java None") {
		t.Errorf("a JVM that answered was reported as one that did not: %v", err)
	}
}

// A JVM whose version is shaped right and too large to hold is not a candidate,
// and renders as the word the served message has always used for "could not
// tell" rather than as a number nobody checked.
func TestAJVMWhoseVersionCannotBeHeldIsNotACandidate(t *testing.T) {
	t.Parallel()
	java := aJVM(t, "fakejava-unreadable")
	if major, ok := javaMajor(java); ok {
		t.Errorf("a twenty-digit version was read as Java %d", major)
	}
	found, err := (Settings{Java: java}).JavaBinary()
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Fatalf("an unreadable version answered %q, %v", found, err)
	}
	if !strings.Contains(err.Error(), "(Java None)") {
		t.Errorf("the refusal reads %v", err)
	}
}

// **The path is searched last and the entry it finds is probed, not trusted.**
// Nothing had driven the path branch at all, because every other test names its
// JVM outright — so the one road a deployment actually takes (`java` on `PATH`,
// no `MTGLAB_JAVA`) was the road no test walked.
//
// A symlink rather than a copy: the executable is still the committed stand-in,
// which is what keeps the first-run scan out of this.
func TestAJVMOnThePathIsFoundAndProbed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	link := filepath.Join(dir, "java")
	if err := os.Symlink(aJVM(t, "fakejava"), link); err != nil {
		t.Fatal(err)
	}

	// An empty entry is skipped and a directory called `java` is not a JVM, so
	// the list carries both ahead of the real one.
	notAJVM := t.TempDir()
	if err := os.Mkdir(filepath.Join(notAJVM, "java"), 0o750); err != nil {
		t.Fatal(err)
	}
	list := strings.Join([]string{"", notAJVM, dir}, string(os.PathListSeparator))

	found, err := (Settings{PathList: list}).JavaBinary()
	if err != nil {
		t.Fatalf("a JVM on the path was not found: %v", err)
	}
	if found != link {
		t.Errorf("the JVM found is %q, want the one on the path", found)
	}

	// And an old one on the path is still refused, naming the path entry — the
	// probe runs on what the search found rather than on what it was told.
	old := t.TempDir()
	if err := os.Symlink(aJVM(t, "fakejava-ancient"), filepath.Join(old, "java")); err != nil {
		t.Fatal(err)
	}
	if found, err := (Settings{PathList: old}).JavaBinary(); err == nil {
		t.Errorf("an old JVM on the path was accepted as %q", found)
	} else if !strings.Contains(err.Error(), filepath.Join(old, "java")) {
		t.Errorf("the refusal does not name the path entry it probed: %v", err)
	}
}

// A bout whose profile cannot be prepared is refused before a JVM is ever
// looked for: the pre-flight passes, the scratch deck directory cannot be made,
// and no subprocess starts.
func TestABoutWhoseProfileCannotBeMadeNeverStarts(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "2.0.14", "Sol Ring", "Forest")
	blocked := filepath.Join(t.TempDir(), "profile")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A JVM that would answer if anything asked it, so a pass cannot be the
	// JVM search failing first.
	forge := Settings{Home: home, Profile: blocked, Java: aJVM(t, "fakejava")}

	run, err := forge.RunGames([]*deck.Deck{testDeck("alpha"), testDeck("beta")},
		RunOptions{Games: 1})
	if err == nil {
		t.Fatalf("a bout ran with no profile to write decks into: %v", run.Argv)
	}
	if run != nil {
		t.Error("a refused bout still handed back a run")
	}
}
