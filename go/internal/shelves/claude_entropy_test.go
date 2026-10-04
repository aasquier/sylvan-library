package shelves

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dryReader is a machine with no randomness left to name a file with.
type dryReader struct{}

func (dryReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// A stage name needs four random bytes, and a source that cannot supply them
// stops the write rather than staging under a name two cold asks might share.
//
// The refusal matters for the reason the atomic write exists at all: the whole
// scheme is that each writer stages under a name nobody else is using, so a
// nonce nobody could generate must not quietly become a fixed one. Nothing is
// left in the directory afterwards — not the stage, not the target.
func TestAShelfWillNotStageAFileItCannotNameUniquely(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "cache", "W.svg")

	err := writeAtomic(target, []byte("<svg/>"), dryReader{})
	if err == nil {
		t.Fatal("a write with no entropy to name its stage went ahead anyway")
	}
	if !strings.Contains(err.Error(), "entropy") {
		t.Logf("the refusal is %v -- any failure will do, as long as it is one", err)
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Error("the target was written despite the refusal")
	}
	entries, readErr := os.ReadDir(filepath.Dir(target))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Errorf("the directory holds %d files after a refused write", len(entries))
	}

	// And the real reader still writes, so the refusal above is about the
	// entropy rather than about the path.
	if err := writeAtomic(target, []byte("<svg/>"), rand.Reader); err != nil {
		t.Fatalf("an ordinary write to the same path: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "<svg/>" {
		t.Errorf("wrote %q", got)
	}
}
