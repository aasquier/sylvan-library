package library

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The five ways the atomic write can fail, and the one promise it makes about
// all of them.
//
// `deck.yaml` is the source of truth (ADR 1) and since ADR 30 there is no
// revision behind it, so the write that replaces one is done through a
// temporary file and a rename: the deck on the disk is either the old one or
// the new one, never half of each. That leaves five calls that can refuse --
// the write, the sync, the close, the mode match and the rename -- and every
// one of them wraps its error with the path, because the operator reading the
// log has to know **which deck would not save**.
//
// None of those five can be made to fire on a working disk.
// `unwritable_test.go` takes the directory's write bit away, which is the
// deployed fault (a volume mounted read-only, a directory owned by somebody
// else, a full disk) and reaches only the first call: `CreateTemp`. The other
// four had never run. `writeAtomicallyOn` takes its filesystem as a value for
// exactly this, and what is asserted here is not the wording -- the operating
// system's messages are not ours -- but that **a refusal comes back, it names
// the deck, and the file on the disk is still the old one**. A write reported
// as landed over a deck that did not change is the answer somebody acts on.

// stubFile stands in for the temporary file, refusing at one chosen step. The
// rest is a real `*os.File`, so everything that is not the fault under test
// behaves exactly as it does in the app.
type stubFile struct {
	*os.File
	failWrite bool
	failSync  bool
	failClose bool
}

var errTheDiskSaidNo = errors.New("the disk said no")

func (s *stubFile) WriteString(text string) (int, error) {
	if s.failWrite {
		return 0, errTheDiskSaidNo
	}
	return s.File.WriteString(text)
}

func (s *stubFile) Sync() error {
	if s.failSync {
		return errTheDiskSaidNo
	}
	return s.File.Sync()
}

func (s *stubFile) Close() error {
	if s.failClose {
		// The real handle still closes: a test that leaked one would fail
		// on Windows and leave temporary files behind everywhere else.
		_ = s.File.Close()
		return errTheDiskSaidNo
	}
	return s.File.Close()
}

// A deck on the disk, and the path to its file.
func aDeckOnTheDisk(t *testing.T) (root, path, original string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, "gyome")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	original = "slug: gyome\nname: The Deck That Was Already There\ncards: []\n"
	path = filepath.Join(dir, "deck.yaml")
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, path, original
}

// Every step of the write, refused in turn.
func TestNoStepOfTheAtomicWriteReportsSuccessWhenTheDiskRefuses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		step string
		// bend turns the real filesystem into one that fails at this step.
		bend func(fs *disk)
	}{
		{"creating the temporary file", func(fs *disk) {
			fs.createTemp = func(string, string) (tempFile, error) { return nil, errTheDiskSaidNo }
		}},
		{"writing the bytes", func(fs *disk) {
			fs.createTemp = failingAt(t, func(s *stubFile) { s.failWrite = true })
		}},
		{"flushing them to the disk", func(fs *disk) {
			fs.createTemp = failingAt(t, func(s *stubFile) { s.failSync = true })
		}},
		{"closing the handle", func(fs *disk) {
			fs.createTemp = failingAt(t, func(s *stubFile) { s.failClose = true })
		}},
		{"matching the old file's mode", func(fs *disk) {
			fs.chmod = func(string, os.FileMode) error { return errTheDiskSaidNo }
		}},
		{"the rename itself", func(fs *disk) {
			fs.rename = func(string, string) error { return errTheDiskSaidNo }
		}},
	} {
		t.Run(tc.step, func(t *testing.T) {
			t.Parallel()
			_, path, original := aDeckOnTheDisk(t)
			fs := realDisk()
			tc.bend(&fs)

			err := writeAtomicallyOn(fs, path, "slug: gyome\nname: The Edit\ncards: []\n")
			if err == nil {
				t.Fatal("the write reported success while the disk refused -- " +
					"the caller has no reason to try again")
			}
			// **The path, in the message.** An operator reading this has to
			// know which deck would not save.
			if !strings.Contains(err.Error(), path) {
				t.Errorf("the refusal is %q and does not name the deck's file", err)
			}
			// And the underlying fault survives the wrapping rather than
			// being flattened into a sentence of ours.
			if !errors.Is(err, errTheDiskSaidNo) {
				t.Errorf("the refusal %q dropped what the disk actually said", err)
			}

			// The deck on the disk is still the old one. This is the whole
			// point of the temporary file: never half of each.
			raw, readErr := os.ReadFile(path) //nolint:gosec // the test's own temp dir
			if readErr != nil {
				t.Fatalf("the deck file is gone after a refused write: %v", readErr)
			}
			if string(raw) != original {
				t.Errorf("a refused write changed the deck on the disk to:\n%s", raw)
			}
		})
	}
}

// failingAt builds a `createTemp` that hands back a real temporary file
// wearing one refusal.
func failingAt(t *testing.T, bend func(*stubFile)) func(string, string) (tempFile, error) {
	t.Helper()
	return func(dir, pattern string) (tempFile, error) {
		f, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		s := &stubFile{File: f}
		bend(s)
		return s, nil
	}
}

// The successful write, through the same seam: the bytes land, the file keeps
// the mode it had, and no temporary file is left in the deck's directory --
// the glob wants `deck.yaml`, so a stray one would be picked up by nothing
// and would sit there forever.
func TestAnAtomicWriteLandsAndLeavesNothingBehind(t *testing.T) {
	t.Parallel()
	root, path, _ := aDeckOnTheDisk(t)
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}

	const edited = "slug: gyome\nname: The Edit\ncards: []\n"
	if err := writeAtomicallyOn(realDisk(), path, edited); err != nil {
		t.Fatalf("the write refused: %v", err)
	}
	raw, err := os.ReadFile(path) //nolint:gosec // the test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != edited {
		t.Errorf("the deck on the disk reads:\n%s", raw)
	}
	// **The mode is the old file's**, not the 0600 a temporary file is born
	// with: a mode that changes on the first edit is the kind of surprise a
	// volume backup notices.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("the deck file's mode became %v", info.Mode().Perm())
	}

	entries, err := os.ReadDir(filepath.Join(root, "gyome"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".deck-") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}
