package tier3

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The far side of finding Forge: a distribution that is *there* and still
// refuses to be read.
//
// `install_test.go` asks what a machine with nothing on it says. These ask what
// a machine with something broken on it says, which is the shape every real
// report of this has taken — a home directory the process cannot write, a
// cardsfolder zip half-restored, a JVM that answers the version probe with
// something nobody can read. All of them are one chmod or one patched byte away
// from the fixtures already here, and none of them had ever run.
//
// Nothing below asserts a message's wording: the operating system's sentences
// are not ours. What is asserted is that a refusal came back at all, that
// nothing claimed to have found a jar or a card it had not, and — where a
// failure is deliberately survivable — that the cards either side of the broken
// one are still in the index.

// aJarNamedNothing is the one jar name the glob matches and the version regexp
// does not: `*` matches an empty string and `(.+)` does not, so the distribution
// is found and its version is unreadable. The ledger stores that as "not
// reported" rather than guessing (ADR 36), which is the branch this reaches.
func TestAJarWithNoVersionInItsNameIsFoundAndNotNamed(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	jar := filepath.Join(home, "forge-gui-desktop--jar-with-dependencies.jar")
	if err := os.WriteFile(jar, []byte("not really a jar"), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := installedAt(home).DesktopJar()
	if err != nil {
		t.Fatalf("a jar the glob matched was refused: %v", err)
	}
	if found != jar {
		t.Errorf("the jar found is %q", found)
	}
	if got := installedAt(home).ForgeVersion(); got != "" {
		t.Errorf("an unparseable jar name reported version %q, which the "+
			"ledger would store as the instrument every rating was measured "+
			"with", got)
	}
}

// A Forge home with a bracket in its path is refused rather than reported as a
// distribution with no jar in it. `filepath.Glob`'s only error is a pattern it
// cannot parse, and the pattern is built out of the operator's own path.
func TestAForgeHomeThatCannotBeGlobbedIsRefused(t *testing.T) {
	t.Parallel()
	home := filepath.Join(t.TempDir(), "for[ge")
	if err := os.MkdirAll(home, 0o750); err != nil {
		t.Fatal(err)
	}
	jar, err := installedAt(home).DesktopJar()
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Fatalf("an unglobbable home answered %q, %v", jar, err)
	}
	if jar != "" {
		t.Errorf("a jar came back from a home that could not be searched: %q", jar)
	}
	if got := installedAt(home).ForgeVersion(); got != "" {
		t.Errorf("the version reads %q", got)
	}
}

// The profile is written into Forge's own program directory, and a directory
// this process cannot write is the deployed shape of that going wrong — the
// home is `/root` while the app runs as `mtglab`. Both halves refuse: the
// marker file Forge reads, and the scratch deck directory underneath the
// profile.
func TestAProfileThatCannotBeWrittenIsRefusedRatherThanAssumed(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("a read-only directory is not a refusal for this user")
	}

	// The program directory cannot be written: the marker never lands.
	home := t.TempDir()
	if err := os.Chmod(home, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(home, 0o700) })
	deckDir, err := (Settings{Home: home, Profile: t.TempDir()}).EnsureProfile()
	if err == nil {
		t.Errorf("an unwritable Forge home handed back %q", deckDir)
	}

	// The program directory is fine and the profile cannot hold a deck
	// directory, because something that is not a directory is already there.
	writable := t.TempDir()
	blocked := filepath.Join(t.TempDir(), "profile")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	deckDir, err = (Settings{Home: writable, Profile: blocked}).EnsureProfile()
	if err == nil {
		t.Errorf("a profile that cannot hold decks handed back %q", deckDir)
	}
}

// A deck file is refused when the directory cannot be made and when the file
// cannot be written, and in both cases no path comes back — a path that names
// a file Forge will not find is worse than a refusal, because `-d` would then
// be pointed at nothing.
func TestADeckFileThatCannotBeWrittenIsRefusedWithNoPath(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Getuid() == 0 {
		t.Skip("a read-only directory is not a refusal for this user")
	}
	d := testDeck("alpha")

	// The directory cannot be created: a plain file sits where it would go.
	blocked := filepath.Join(t.TempDir(), "decks")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if path, err := WriteDck(d, blocked, nil); err == nil {
		t.Errorf("a deck was written to %q under a file", path)
	}

	// The directory exists and refuses the write.
	readOnly := t.TempDir()
	if err := os.Chmod(readOnly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnly, 0o700) })
	if path, err := WriteDck(d, readOnly, nil); err == nil {
		t.Errorf("a deck was written to %q in a read-only directory", path)
	}
}

// **A card script that will not open or will not read costs that card, not the
// pre-flight** — the `errors="replace"` argument, which the reader states and
// nothing had driven. One entry is given a compression method no reader knows
// and another's bytes are corrupted under its own checksum; the index comes
// back without them and with everything else.
func TestACardScriptThatCannotBeReadCostsThatCardAlone(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeBrokenCardsfolder(t, home, "Sealed", "Corrupt", "Forest")

	names, err := remembering(home).ImplementedNames()
	if err != nil {
		t.Fatalf("one unreadable script refused the whole index: %v", err)
	}
	if !names["Forest"] {
		t.Error("the readable card is missing from the index, so one broken " +
			"script cost the whole pre-flight")
	}
	for _, lost := range []string{"Sealed", "Corrupt"} {
		if names[lost] {
			t.Errorf("%q was read out of a script that cannot be read", lost)
		}
	}
}

// writeBrokenCardsfolder writes a cardsfolder zip whose first card carries a
// compression method no reader implements, whose second card's bytes no longer
// match its own checksum, and whose third is plain.
//
// Patched afterwards rather than written that way: `archive/zip` will not
// *create* either fault, which is the point — this is a half-restored file or a
// zip some other tool wrote, not something this project can produce.
func writeBrokenCardsfolder(t *testing.T, home string, sealed, corrupt, plain string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, card := range []string{sealed, corrupt, plain} {
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:   "cardsfolder/" + card + ".txt",
			Method: zip.Store,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("Name:" + card + "\nTypes:Creature\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()

	// The compression method lives at offset 8 of a local header and offset 10
	// of a central directory entry; 99 is the AES marker, which Go's reader
	// knows about and cannot decompress.
	local := bytes.Index(raw, []byte("PK\x03\x04"))
	central := bytes.Index(raw, []byte("PK\x01\x02"))
	if local < 0 || central < 0 {
		t.Fatal("the zip has no headers to patch")
	}
	binary.LittleEndian.PutUint16(raw[local+8:], 99)
	binary.LittleEndian.PutUint16(raw[central+10:], 99)

	// And one stored byte flipped inside the second card's name leaves the
	// entry's CRC naming bytes that are no longer there.
	at := bytes.Index(raw, []byte("Name:"+corrupt))
	if at < 0 {
		t.Fatal("the second card is not in the zip")
	}
	raw[at+5]++

	path := filepath.Join(home, cardsfolder)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A cardsfolder zip that is not a zip at all is a refusal naming the file,
// because every card in the pre-flight depends on it — the one failure in this
// reader that is *not* survivable.
func TestACardsfolderThatIsNotAZipIsRefused(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, cardsfolder)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("this is not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	names, err := installedAt(home).ImplementedNames()
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Fatalf("an unreadable archive answered %d names, %v", len(names), err)
	}
}

// A distribution with a jar and no card data is refused by the index as well as
// by the path lookup, and the index that refuses it remembers nothing — a
// machine with no card data must not come back as a machine that implements no
// cards, which would read as every deck being uncovered.
func TestAnIndexOfAMachineWithNoCardDataRemembersNothing(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50")
	if err := os.Remove(filepath.Join(home, cardsfolder)); err != nil {
		t.Fatal(err)
	}
	s := remembering(home)
	if names, err := s.ImplementedNames(); !errors.Is(err, ErrForgeNotInstalled) {
		t.Fatalf("a machine with no card data answered %d names, %v", len(names), err)
	}
	if hits, misses := s.Index.Stats(); hits != 0 || misses != 0 {
		t.Errorf("the index recorded %d hits and %d misses for a read that "+
			"never happened", hits, misses)
	}
}

// A [CardIndex] built as a zero value remembers as well as one from
// [NewCardIndex] — the table is made on first use, so a [Settings] literal
// holding `&CardIndex{}` is a machine with a memory rather than a machine whose
// memory silently drops everything.
func TestAZeroCardIndexStillRemembers(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "", "Forest", "Island")
	s := Settings{Home: home, Index: &CardIndex{}}

	first, err := s.ImplementedNames()
	if err != nil {
		t.Fatalf("the first read: %v", err)
	}
	if !first["Forest"] {
		t.Error("the first read came back without the card in the zip")
	}
	second, err := s.ImplementedNames()
	if err != nil {
		t.Fatalf("the second read: %v", err)
	}
	if len(second) != len(first) {
		t.Errorf("the remembered index holds %d names, the read %d",
			len(second), len(first))
	}
	hits, misses := s.Index.Stats()
	if hits != 1 || misses != 1 {
		t.Errorf("a zero index recorded %d hits and %d misses, so the second "+
			"read did not come out of memory", hits, misses)
	}
}
