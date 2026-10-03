package cache

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

// **A fingerprint that cannot be computed disables caching, rather than hashing
// what it managed to read.**
//
// The empty string is the one answer that matters here and it had never been
// produced: every stored Tier 1 row is keyed on this digest, so a fingerprint
// taken over *part* of the engine would read as a different engine's — or worse,
// as the same one, serving a cached answer computed by code that has since
// changed. The guard beside the read is held by a test already; the one beside
// the listing is this, and both answer the same way on purpose.
//
// `fs.Glob` hands the pattern to the filesystem when it implements `GlobFS`, so
// an engine source whose own listing refuses is a filesystem a test can build
// and nothing about the embedded bytes has to move.

// refusingFS is a filesystem that holds a file and will not list what it holds.
type refusingFS struct{ fs.FS }

func (refusingFS) Glob(string) ([]string, error) {
	return nil, errors.New("this directory will not say what is in it")
}

func TestAnEngineThatCannotBeListedDisablesTheCacheEntirely(t *testing.T) {
	t.Parallel()
	real := fstest.MapFS{"tier1.go": {Data: []byte("package tier1")}}

	// Readable: a digest, and the same digest twice.
	plain := []engineSource{{"internal/sim/tier1", real}}
	first := fingerprintOf(plain)
	if first == "" {
		t.Fatal("a readable engine source produced no fingerprint at all")
	}
	if again := fingerprintOf(plain); again != first {
		t.Errorf("the same sources hashed to %q and then %q", first, again)
	}

	// Unlistable: no fingerprint, which is how a caller is told to compute
	// rather than to guess.
	if got := fingerprintOf([]engineSource{{"internal/sim/tier1",
		refusingFS{real}}}); got != "" {
		t.Errorf("an engine whose files cannot be listed hashed to %q", got)
	}

	// And a listing that names a file the filesystem will not hand over is the
	// same answer, from the other guard — asserted beside it so the pair cannot
	// drift apart.
	if got := fingerprintOf([]engineSource{{"internal/sim/tier1",
		fstest.MapFS{"tier1.go": {Data: []byte("x"), Mode: fs.ModeDir}}}}); got != "" {
		t.Errorf("an engine source that cannot be read hashed to %q", got)
	}
}

// **And an unidentifiable engine has no key**, which is what turns that empty
// fingerprint into "compute it" everywhere a result might otherwise be stored
// under a key no engine owns.
func TestAnEngineWithNoFingerprintHasNoKey(t *testing.T) {
	t.Parallel()
	in := Input{Games: 2000, Turns: 8, Seed: 7}

	if got := keyFrom("", "tier1", in); got != "" {
		t.Errorf("a run with no engine behind it keyed as %q", got)
	}
	keyed := keyFrom("an-engine", "tier1", in)
	if keyed == "" {
		t.Fatal("a named engine produced no key")
	}
	if keyFrom("another-engine", "tier1", in) == keyed {
		t.Error("two engines key the same run identically, so a stored row " +
			"would be served by code that has since changed")
	}
	// The real one is this process's own engine, and it has a key.
	if Key("tier1", in) != keyFrom(Fingerprint(), "tier1", in) {
		t.Error("the key is not the one this engine's fingerprint produces")
	}
}
