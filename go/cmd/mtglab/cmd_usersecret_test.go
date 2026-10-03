package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// The hidden half of a password prompt: what happens when the input really is
// a terminal.
//
// **This was closed by argument and is open again, for one reason.** It was
// worked out on 2026-09-26 and ruled closed on 2026-09-29: a real pty needs
// platform-specific test code, and the obvious seam was rejected because its
// own default would still call [golang.org/x/term.ReadPassword] -- the
// uncovered lines moving into the seam rather than out of the tree. What is
// different now is that the default turns out to be **drivable without a pty**:
// a hidden read is a question to the kernel about a descriptor's termios, and a
// descriptor that is not a terminal is refused in no time at all, with nothing
// read and nothing echoed. So the seam earns its keep -- both halves run, and
// the real pair is held to the promise the branch exists to keep.
//
// That promise is the only thing worth testing here, and it is a security
// property rather than a convenience: **a hidden read must never quietly
// degrade into an echoed one.** The clear-text fallback exists, says so out
// loud, and is chosen by the branch *above* the read. If `ReadPassword` ever
// answered a pipe by reading it in the clear, a password piped into `mtglab
// users passwd` would be echoed with no warning at all.

// A password typed at a terminal comes back whole, and the typing is not read
// out of the stream.
//
// The second half is the interesting assertion: the hidden read takes the line
// from the terminal, so whatever is sitting in the prompt's own buffered reader
// has to be untouched afterwards. A terminal branch that fell through to
// [prompt.line] would pass the first check and fail this one.
func TestAPasswordTypedAtATerminalIsReadWithoutTouchingTheStream(t *testing.T) {
	t.Parallel()
	r, w := pipePair(t)
	if _, err := w.WriteString("a-line-nobody-should-read\n"); err != nil {
		t.Fatal(err)
	}
	asked := 0
	p := &prompt{in: r, err: io.Discard, lines: bufio.NewReader(r), tty: terminal{
		is:   func(int) bool { return true },
		read: func(int) ([]byte, error) { asked++; return []byte("hunter2hunter2"), nil },
	}}

	got, err := p.secret("  password: ")
	if err != nil {
		t.Fatalf("a terminal that answered was refused: %v", err)
	}
	if got != "hunter2hunter2" {
		t.Errorf("the password came back as %q", got)
	}
	if asked != 1 {
		t.Errorf("the terminal was asked %d times", asked)
	}
	// Nothing was taken from the stream, so the line is still there.
	line, err := p.line()
	if err != nil {
		t.Fatal(err)
	}
	if line != "a-line-nobody-should-read" {
		t.Errorf("the hidden read consumed the stream: the next line is %q", line)
	}
}

// A terminal that cannot be read is a refusal, and no password at all.
//
// The fallback is not reached from here on purpose: an operator at a real
// terminal whose read failed must not have the next keystrokes taken in the
// clear as a consolation.
func TestATerminalThatCannotBeReadYieldsNoPassword(t *testing.T) {
	t.Parallel()
	r, _ := pipePair(t)
	p := &prompt{in: r, err: io.Discard, lines: bufio.NewReader(r), tty: terminal{
		is:   func(int) bool { return true },
		read: func(int) ([]byte, error) { return []byte("half-typed"), errors.New("the terminal went away") },
	}}

	got, err := p.secret("  password: ")
	if err == nil {
		t.Fatalf("a terminal that failed handed back %q", got)
	}
	if got != "" {
		t.Errorf("a failed read still produced a password: %q", got)
	}
}

// The process's own terminal, and the one promise it has to keep: handed a
// descriptor that cannot hide what is typed, the hidden read **refuses**
// rather than reading it in the clear.
//
// This is the seam's own default (docs/polish/COVERAGE.md, lever 36) and the
// reason the seam is honest: the line the deployed binary runs is the line this
// drives. A pipe stands in for "not a terminal" because that is exactly what a
// script piping a password has, and it is a fact about this process rather than
// a restatement of the code -- no pty, no clock, no platform code.
func TestTheRealTerminalRefusesToHideWhatItCannotHide(t *testing.T) {
	t.Parallel()
	tty := realTerminal()
	r, w := pipePair(t)
	if _, err := w.WriteString("the-password\n"); err != nil {
		t.Fatal(err)
	}
	fd := int(r.Fd())

	if tty.is(fd) {
		t.Error("a pipe was taken for a terminal, so a piped password would " +
			"be read with no warning that it had been echoed")
	}
	raw, err := tty.read(fd)
	if err == nil {
		t.Errorf("the hidden read answered over something that cannot hide "+
			"anything, handing back %q", raw)
	}
	if len(raw) != 0 {
		t.Errorf("a refused read still produced %q", raw)
	}
	// And it took nothing: the line is still queued for whoever reads next,
	// which is what makes the clear-text fallback above it correct.
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(line, "\n") != "the-password" {
		t.Errorf("the refused read consumed %q from the stream", line)
	}
}

// A prompt over something that is a file and not a terminal still falls to the
// clear-text read, and warns -- the path a script takes, and the one the
// commands' own tests drive through a `strings.Reader`. Asserted here over a
// real `*os.File`, because that is the type the branch above tests for and the
// only one that reaches the terminal question at all.
func TestAPipedPasswordIsReadInTheClearAndSaidSo(t *testing.T) {
	t.Parallel()
	r, w := pipePair(t)
	if _, err := w.WriteString("piped-and-echoed\n"); err != nil {
		t.Fatal(err)
	}
	said := &strings.Builder{}
	p := &prompt{in: r, err: said, lines: bufio.NewReader(r), tty: realTerminal()}

	got, err := p.secret("  password: ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "piped-and-echoed" {
		t.Errorf("the piped password came back as %q", got)
	}
	if !strings.Contains(said.String(), "may be echoed") {
		t.Errorf("a password read in the clear was not announced: %q", said)
	}
}

// pipePair is an `*os.File` on both ends: the only kind of reader that can be
// a terminal, and therefore the only one that reaches the branch above.
func pipePair(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	return r, w
}
