package tier3

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// Finding Forge, which is every failure a Tier 3 run hits before a card is
// ever played.
//
// **Most of these run in parallel now.** They did not until ADR 40: the
// discovery functions read `MTGLAB_FORGE_HOME`, `MTGLAB_JAVA` and
// `MTGLAB_FORGE_PROFILE` from the process, so a test could only describe a
// machine by writing the process -- `t.Setenv`, which Go panics on inside a
// parallel test. A [Settings] literal says the same thing to one caller
// instead of to every goroutine in the binary.
//
// The ones still serial hold something shared and say so where they stand:
// the coverage index is package-level state (guarded, so `-race` is quiet --
// it is `ClearIndex` and the hit counters that collide, which nothing but a
// reading of the test would report), two Java tests write `PATH`, which is
// genuinely the process's, and one is about [LoadSettings] itself.
//
// The distinction the messages draw is the one that matters and the one a
// first attempt got wrong: **a missing directory is not an unreadable one**.
// Deployed, Forge's home is `/root` while the app runs as `mtglab`, and
// conflating the two made `/api/forge` answer 500 where it should have said
// `available: false`.

// installedAt is a machine with Forge unpacked at one path and nothing else
// said about it -- the literal that replaced `MTGLAB_FORGE_HOME`.
//
// A helper rather than the struct spelled out at each call site for a reason
// Go picks: `if got := Settings{Home: x}.ForgeVersion(); ...` does not parse,
// because the parser cannot tell the composite literal from the `if` body's
// opening brace. A function call has no such problem, and reads as the
// sentence the test means.
func installedAt(home string) Settings { return Settings{Home: home} }

// fakeForge builds a Forge distribution good enough for every check that does
// not need a JVM: a versioned desktop jar and a cardsfolder zip holding the
// named card scripts.
func fakeForge(t *testing.T, version string, cards ...string) string {
	t.Helper()
	home := t.TempDir()
	if version != "" {
		jar := filepath.Join(home,
			fmt.Sprintf("forge-gui-desktop-%s-jar-with-dependencies.jar", version))
		if err := os.WriteFile(jar, []byte("not really a jar"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeCardsfolder(t, home, cards...)
	return home
}

// writeCardsfolder puts a real zip where Forge keeps its card scripts.
func writeCardsfolder(t *testing.T, home string, cards ...string) {
	t.Helper()
	path := filepath.Join(home, cardsfolder)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path) //nolint:gosec // a test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for i, card := range cards {
		w, err := zw.Create(fmt.Sprintf("cardsfolder/%c/card%d.txt",
			strings.ToLower(card)[0], i))
		if err != nil {
			t.Fatal(err)
		}
		// Forge's card scripts lead with the name and carry more after it.
		if _, err := fmt.Fprintf(w, "Name:%s\nManaCost:G\nTypes:Creature\n", card); err != nil {
			t.Fatal(err)
		}
	}
	// A directory entry and a non-script file, both of which the reader skips.
	if _, err := zw.Create("cardsfolder/"); err != nil {
		t.Fatal(err)
	}
	if w, err := zw.Create("cardsfolder/README.md"); err == nil {
		_, _ = w.Write([]byte("Name:Not A Card\n"))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

// The jar is found by glob and the newest name wins, because a distribution
// upgraded in place leaves both behind.
func TestTheDesktopJarIsTheNewestOnePresent(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50")
	older := filepath.Join(home, "forge-gui-desktop-1.6.49-jar-with-dependencies.jar")
	if err := os.WriteFile(older, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	jar, err := installedAt(home).DesktopJar()
	if err != nil {
		t.Fatalf("a real distribution was refused: %v", err)
	}
	if !strings.Contains(jar, "1.6.50") {
		t.Errorf("chose %s, want the newer 1.6.50", filepath.Base(jar))
	}
}

// A missing directory and an unreadable one are different facts and carry
// different sentences. The second is the deployed shape -- home is `/root`
// and the app is `mtglab` -- and conflating them made the gate answer 500.
func TestAMissingForgeIsNotAnUnreadableOne(t *testing.T) {
	t.Parallel()
	// Absent: falls through to the jar message.
	absent := filepath.Join(t.TempDir(), "nothing-here")
	_, err := installedAt(absent).DesktopJar()
	if err == nil {
		t.Fatal("a directory that is not there produced a jar")
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("an absent Forge is %T, want ErrForgeNotInstalled", err)
	}
	if !strings.Contains(err.Error(), "no Forge desktop jar in") {
		t.Errorf("an absent directory said %q", err)
	}

	// Present but with no jar: the same sentence, because there is nothing
	// wrong with the machine.
	empty := t.TempDir()
	_, err = installedAt(empty).DesktopJar()
	if err == nil || !strings.Contains(err.Error(), "no Forge desktop jar in") {
		t.Errorf("an empty directory said %v", err)
	}

	// Unreadable: the refusal, reserved for a real permission error.
	if os.Geteuid() == 0 {
		t.Skip("root reads everything, so there is no unreadable directory to build")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
	_, err = installedAt(locked).DesktopJar()
	if err == nil {
		t.Fatal("an unreadable directory produced a jar")
	}
	if !strings.Contains(err.Error(), "no Forge distribution readable at") {
		t.Errorf("an unreadable directory said %q -- that is the absent-directory sentence", err)
	}
}

// The version is read off the jar's own name, because the match ledger
// records which Forge played (ADR 36): an upgrade changes the instrument, and
// ratings mixed across one would silently blend two judges.
func TestTheForgeVersionIsReadOffTheJarName(t *testing.T) {
	t.Parallel()
	if got := installedAt(fakeForge(t, "1.6.50")).ForgeVersion(); got != "1.6.50" {
		t.Errorf("read %q, want 1.6.50", got)
	}
	if got := installedAt(fakeForge(t, "2.0.0-SNAPSHOT")).ForgeVersion(); got != "2.0.0-SNAPSHOT" {
		t.Errorf("read %q", got)
	}
	// No distribution: empty, which the ledger stores as "not reported"
	// rather than guessing.
	if got := installedAt(filepath.Join(t.TempDir(), "gone")).ForgeVersion(); got != "" {
		t.Errorf("a missing Forge reported version %q", got)
	}
	// A jar whose name does not parse is also "not reported".
	odd := t.TempDir()
	if err := os.WriteFile(filepath.Join(odd,
		"forge-gui-desktop-jar-with-dependencies.jar"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := installedAt(odd).ForgeVersion(); got != "" {
		t.Errorf("an unparseable jar name reported %q", got)
	}
}

// The card index is read from Forge's own scripts, and the reader skips
// everything that is not a card script.
// **Serial**: it shares the package-level coverage index, which is guarded
// (so `-race` is quiet) but not partitioned -- `ClearIndex` and the hit
// counters are what collide, and only a reading of the test would report it.
func TestTheCardIndexIsReadFromForgesOwnScripts(t *testing.T) {
	ClearIndex()
	home := fakeForge(t, "1.6.50", "Llanowar Elves", "Sol Ring", "Forest")

	names, err := installedAt(home).ImplementedNames()
	if err != nil {
		t.Fatalf("reading the index: %v", err)
	}
	for _, want := range []string{"Llanowar Elves", "Sol Ring", "Forest"} {
		if !names[want] {
			t.Errorf("the index is missing %q", want)
		}
	}
	// The README is not a card script, so its `Name:` line is not a card.
	if names["Not A Card"] {
		t.Error("a non-.txt file was read as a card script")
	}
	if len(names) != 3 {
		t.Errorf("the index holds %d names, want 3: %v", len(names), names)
	}
}

// The index is cached on (path, mtime, size), so upgrading Forge in place
// invalidates it rather than serving a stale answer -- which matters
// precisely because an upgrade is when coverage changes.
// **Serial**: it shares the package-level coverage index, which is guarded
// (so `-race` is quiet) but not partitioned -- `ClearIndex` and the hit
// counters are what collide, and only a reading of the test would report it.
func TestTheCardIndexIsCachedAndAnUpgradeInvalidatesIt(t *testing.T) {
	ClearIndex()
	home := fakeForge(t, "1.6.50", "Llanowar Elves")

	if _, err := installedAt(home).ImplementedNames(); err != nil {
		t.Fatal(err)
	}
	hits, misses := IndexStats()
	if hits != 0 || misses != 1 {
		t.Fatalf("the first read was %d hits and %d misses", hits, misses)
	}

	if _, err := installedAt(home).ImplementedNames(); err != nil {
		t.Fatal(err)
	}
	if hits, misses = IndexStats(); hits != 1 || misses != 1 {
		t.Fatalf("the second read was %d hits and %d misses -- the cache never hit", hits, misses)
	}

	// An upgrade in place: same path, different bytes.
	writeCardsfolder(t, home, "Llanowar Elves", "Craterhoof Behemoth")
	if err := os.Chtimes(filepath.Join(home, cardsfolder),
		nowPlus(t, 120), nowPlus(t, 120)); err != nil {
		t.Fatal(err)
	}
	names, err := installedAt(home).ImplementedNames()
	if err != nil {
		t.Fatal(err)
	}
	if !names["Craterhoof Behemoth"] {
		t.Error("the upgraded distribution served the stale index")
	}
	if _, misses = IndexStats(); misses != 2 {
		t.Errorf("an upgrade did not miss the cache (misses=%d)", misses)
	}

	// And the register clears, the way every registered cache does.
	ClearIndex()
	if hits, misses = IndexStats(); hits != 0 || misses != 0 {
		t.Errorf("after clearing: %d hits, %d misses", hits, misses)
	}
}

// nowPlus is a timestamp `seconds` from now, for aging a file past the index
// key or back behind a rewrite check.
func nowPlus(t *testing.T, seconds int) time.Time {
	t.Helper()
	return time.Now().Add(time.Duration(seconds) * time.Second)
}

// Every path that cannot find Forge says which environment variable would
// fix it, because the person reading the message is the person who can.
func TestEveryMissingForgeMessageNamesTheVariableThatFixesIt(t *testing.T) {
	t.Parallel()
	gone := filepath.Join(t.TempDir(), "gone")

	_, err := installedAt(gone).CardsfolderPath()
	if err == nil {
		t.Fatal("a missing distribution produced a cardsfolder path")
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("the error is %T", err)
	}
	if !strings.Contains(err.Error(), "MTGLAB_FORGE_HOME") {
		t.Errorf("the message does not name the variable: %q", err)
	}

	_, err = installedAt(gone).ImplementedNames()
	if err == nil || !strings.Contains(err.Error(), "MTGLAB_FORGE_HOME") {
		t.Errorf("the index's refusal said %q", err)
	}

	_, err = installedAt(gone).DesktopJar()
	if err == nil || !strings.Contains(err.Error(), "MTGLAB_FORGE_HOME") {
		t.Errorf("the jar's refusal said %q", err)
	}
}

// A cardsfolder that is not a zip costs the whole pre-flight rather than
// being silently read as empty -- an empty index would report every card in
// every deck as unimplemented, which reads as a deck problem.
// **Serial**: it shares the package-level coverage index, which is guarded
// (so `-race` is quiet) but not partitioned -- `ClearIndex` and the hit
// counters are what collide, and only a reading of the test would report it.
func TestAnUnreadableCardsfolderIsRefusedRatherThanReadAsEmpty(t *testing.T) {
	ClearIndex()
	home := t.TempDir()
	path := filepath.Join(home, cardsfolder)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("this is not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := installedAt(home).ImplementedNames(); err == nil {
		t.Fatal("a corrupt cardsfolder was read as an empty index")
	} else if !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("the refusal said %q", err)
	}
}

// The pre-flight reads a zip and needs no Java at all, which is what makes it
// the cheap check an API can run on a request thread.
// **Serial**: it shares the package-level coverage index, which is guarded
// (so `-race` is quiet) but not partitioned -- `ClearIndex` and the hit
// counters are what collide, and only a reading of the test would report it.
func TestThePreFlightRunsWithoutAJVMAndNamesWhatIsMissing(t *testing.T) {
	ClearIndex()
	home := fakeForge(t, "1.6.50", "Sol Ring", "Forest")

	covered := &deck.Deck{Slug: "covered",
		Commander: []string{"Sol Ring"},
		Cards:     []deck.CardEntry{{Name: "Forest"}},
	}
	reports, err := installedAt(home).CheckCoverage([]*deck.Deck{covered})
	if err != nil {
		t.Fatalf("a fully covered deck failed the pre-flight: %v", err)
	}
	if len(reports) != 1 || reports[0].Checked != 2 || len(reports[0].Missing) != 0 {
		t.Fatalf("the report is %+v", reports[0])
	}

	// A card Forge lacks fails the pre-flight and is named, because coverage
	// is checked before and after precisely so a dropped card is never
	// silent.
	short := &deck.Deck{Slug: "short",
		Commander: []string{"Sol Ring"},
		Cards:     []deck.CardEntry{{Name: "Nonexistent Card"}},
	}
	_, err = installedAt(home).CheckCoverage([]*deck.Deck{short})
	if err == nil {
		t.Fatal("a deck with an unimplemented card passed the pre-flight")
	}
	if !errors.Is(err, ErrCoverageFailed) {
		t.Errorf("the failure is %T, want ErrCoverageFailed", err)
	}
	if !strings.Contains(err.Error(), "Nonexistent Card") {
		t.Errorf("the failure did not name the card: %q", err)
	}

	// No Forge at all fails as not-installed rather than as a coverage
	// problem: those are different questions with different answers.
	_, err = installedAt(filepath.Join(t.TempDir(), "gone")).CheckCoverage([]*deck.Deck{covered})
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("a missing Forge failed as %v", err)
	}
}

// The pre-flight counts each distinct card once, so a deck with four Forests
// is one check rather than four -- and the commander and companion are
// checked alongside the 99.
// **Serial**: it shares the package-level coverage index, which is guarded
// (so `-race` is quiet) but not partitioned -- `ClearIndex` and the hit
// counters are what collide, and only a reading of the test would report it.
func TestThePreFlightCountsEachCardOnceAndIncludesTheCommandZone(t *testing.T) {
	ClearIndex()
	home := fakeForge(t, "1.6.50", "Sol Ring", "Forest", "Kaheera, the Orphanguard")
	companion := "Kaheera, the Orphanguard"

	d := &deck.Deck{Slug: "d",
		Commander: []string{"Sol Ring"},
		Companion: &companion,
		Cards: []deck.CardEntry{
			{Name: "Forest"}, {Name: "Forest"}, {Name: "Forest"},
		},
	}
	index, err := installedAt(home).ImplementedNames()
	if err != nil {
		t.Fatal(err)
	}
	report := Check(d, index)
	if report.Checked != 3 {
		t.Errorf("checked %d distinct cards, want 3", report.Checked)
	}
	if len(report.Missing) != 0 {
		t.Errorf("missing %v", report.Missing)
	}
	if report.Resolved[companion] != companion {
		t.Errorf("the companion resolved to %q", report.Resolved[companion])
	}
}

// The profile points Forge at a directory this project owns, so generated
// decks never mix into whatever the person has saved by hand. It is rewritten
// only when the contents would change, so a run does not needlessly touch a
// shared install.
func TestTheProfileIsWrittenOnceAndOwnsItsOwnDirectory(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50")
	profile := filepath.Join(t.TempDir(), "profile")
	machine := Settings{Home: home, Profile: profile}

	deckDir, err := machine.EnsureProfile()
	if err != nil {
		t.Fatalf("writing the profile: %v", err)
	}
	if want := filepath.Join(profile, "decks", "commander"); deckDir != want {
		t.Errorf("the deck directory is %s, want %s", deckDir, want)
	}
	if info, err := os.Stat(deckDir); err != nil || !info.IsDir() {
		t.Errorf("the deck directory was not made: %v", err)
	}

	marker := filepath.Join(home, "forge.profile.properties")
	first, err := os.ReadFile(marker) //nolint:gosec // a test's own temp dir
	if err != nil {
		t.Fatalf("the profile marker was not written: %v", err)
	}
	if !strings.Contains(string(first), "userDir="+profile) {
		t.Errorf("the marker points elsewhere: %q", first)
	}

	// Unchanged contents: the file is left alone rather than rewritten.
	if err := os.Chtimes(marker, nowPlus(t, -3600), nowPlus(t, -3600)); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := machine.EnsureProfile(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(marker)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged profile was rewritten anyway")
	}

	// A different profile does rewrite it.
	elsewhere := Settings{Home: home, Profile: filepath.Join(t.TempDir(), "other")}
	if _, err := elsewhere.EnsureProfile(); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(marker) //nolint:gosec // a test's own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if string(second) == string(first) {
		t.Error("pointing the profile elsewhere did not rewrite the marker")
	}
}

// There is nothing to configure on a machine with no Forge, so the profile
// refuses rather than scaffolding a directory beside an install that is not
// there.
func TestTheProfileRefusesWithoutADistribution(t *testing.T) {
	t.Parallel()
	_, err := Settings{
		Home:    filepath.Join(t.TempDir(), "gone"),
		Profile: filepath.Join(t.TempDir(), "profile"),
	}.EnsureProfile()
	if err == nil {
		t.Fatal("a machine with no Forge configured a profile anyway")
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("the refusal is %T", err)
	}
}

// The environment overrides exist so an operator can point at an install this
// code would never have guessed, and each falls back to the same
// `~/.local/share/mtglab` layout.
//
// **Serial on purpose, and the only test here that is about the environment
// at all.** [LoadSettings] is the one reader in the process (ADR 40), so this
// is where reading it is tested; everything else in this file describes a
// machine with a literal and never touches the process it runs in.
func TestTheOverridesWinAndTheFallbacksAgreeOnTheLayout(t *testing.T) {
	t.Setenv("MTGLAB_FORGE_HOME", "/somewhere/else")
	t.Setenv("MTGLAB_FORGE_PROFILE", "/profile/here")
	t.Setenv("MTGLAB_JAVA", "/jvm/here")
	t.Setenv("MTGLAB_FORGE_MACHINE", "another-worker")
	t.Setenv("MTGLAB_FORGE_SHIM_PORT", "9999")
	// A trailing slash is trimmed here rather than at every call site --
	// otherwise the client would request every path with a double slash.
	t.Setenv("MTGLAB_FORGE_WORKER_URL", "http://shim.internal:8080/")
	loaded := LoadSettings()
	for _, c := range []struct{ what, got, want string }{
		{"the Forge home", loaded.Home, "/somewhere/else"},
		{"the profile", loaded.Profile, "/profile/here"},
		{"the JVM", loaded.Java, "/jvm/here"},
		{"the machine", loaded.Machine, "another-worker"},
		{"the worker URL", loaded.WorkerURL, "http://shim.internal:8080"},
	} {
		if c.got != c.want {
			t.Errorf("%s override lost: %q, want %q", c.what, c.got, c.want)
		}
	}
	if loaded.ShimPort != 9999 {
		t.Errorf("the port override lost: %d", loaded.ShimPort)
	}

	for _, name := range []string{"MTGLAB_FORGE_HOME", "MTGLAB_FORGE_PROFILE",
		"MTGLAB_JAVA", "MTGLAB_FORGE_MACHINE", "MTGLAB_FORGE_SHIM_PORT",
		"MTGLAB_FORGE_WORKER_URL"} {
		t.Setenv(name, "")
	}
	base := filepath.Join(homeDir(), ".local", "share", "mtglab")
	fell := LoadSettings()
	if fell.Home != filepath.Join(base, "forge") {
		t.Errorf("the default Forge home is %q", fell.Home)
	}
	if fell.Profile != filepath.Join(base, "forge-profile") {
		t.Errorf("the default profile is %q", fell.Profile)
	}
	if fell.BundledJDK != filepath.Join(base, "jdk-21") {
		t.Errorf("the bundled JDK is %q", fell.BundledJDK)
	}
	if fell.Java != "" {
		t.Errorf("an unset MTGLAB_JAVA resolved to %q, want the search", fell.Java)
	}
	if fell.Machine != DefaultMachine || fell.ShimPort != DefaultShimPort {
		t.Errorf("the defaults are %q and %d", fell.Machine, fell.ShimPort)
	}
	if fell.IdleSeconds != DefaultIdleSeconds || fell.MemoryMB != DefaultMemoryMB {
		t.Errorf("the shim defaults are %d and %d", fell.IdleSeconds, fell.MemoryMB)
	}
	if fell.ShimHost != DefaultShimHost {
		t.Errorf("the default shim host is %q", fell.ShimHost)
	}
	// Everything lives under one directory, which is the point: nothing
	// under it may ever be tracked.
	for _, path := range []string{fell.Home, fell.Profile, fell.BundledJDK} {
		if !strings.HasPrefix(path, base) {
			t.Errorf("%q escapes %q", path, base)
		}
	}
	// An unparseable port is a typo, and reads as unset rather than as zero --
	// which would bind the shim to whatever the kernel handed out.
	t.Setenv("MTGLAB_FORGE_SHIM_PORT", "banana")
	if got := LoadSettings().ShimPort; got != DefaultShimPort {
		t.Errorf("an unparseable port resolved to %d, want the default", got)
	}
}

// homeDir falls back to `$HOME` when the user database cannot answer, which
// is the container's shape rather than a laptop's.
func TestHomeDirAlwaysAnswers(t *testing.T) {
	t.Parallel()
	if got := homeDir(); got == "" {
		t.Error("no home directory at all")
	}
}

// A binary that will not answer `-version` is not a candidate. This machine's
// own `/usr/bin/java` is 10.0.1 and fails Forge in a way that reads like a
// Forge bug rather than a Java one, which is why the probe exists at all.
func TestAJavaThatWillNotAnswerIsNotACandidate(t *testing.T) {
	t.Parallel()
	if _, ok := javaMajor(filepath.Join(t.TempDir(), "not-a-binary")); ok {
		t.Error("a path with no file on it probed as a JVM")
	}
	// A real binary that answers nothing resembling a version.
	if _, ok := javaMajor("/bin/echo"); ok {
		t.Error("a binary with no version string probed as a JVM")
	}
}

// With no JVM anywhere, the refusal lists what was tried and how to fix it --
// and a candidate that could not be probed renders as `None`, the served
// message's long-standing spelling of "could not tell".
func TestNoJavaAnywhereListsWhatWasTried(t *testing.T) {
	// **Serial**: it calls `t.Setenv("PATH", ...)`, which Go panics on inside
	// a parallel test -- and here the process's own `PATH` is the point rather
	// than the setup. The search this drives looks along it, so emptying it is
	// the only way to ask what happens on a machine with no JVM from a machine
	// that may well have one. Nothing can make this parallel; a second test
	// running beside it would be running with no `PATH`.
	// A file that exists and is not a JVM, so the probe fails rather than
	// the stat.
	fake := filepath.Join(t.TempDir(), "java")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil { //nolint:gosec // a test's own temp dir
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // nothing named java on it; genuinely the process's

	_, err := Settings{Java: fake, BundledJDK: t.TempDir()}.JavaBinary()
	if err == nil {
		t.Skip("this machine has a Java new enough to satisfy the search")
	}
	if !errors.Is(err, ErrForgeNotInstalled) {
		t.Errorf("the refusal is %T, want ErrForgeNotInstalled", err)
	}
	for _, want := range []string{"MTGLAB_JAVA", fake, "None"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q: %q", want, err)
		}
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("Java %d+", JavaMinimum)) {
		t.Errorf("the refusal does not name the floor: %q", err)
	}
}

// With nothing on any candidate path at all, the refusal still reads as a
// sentence rather than trailing off after "Checked:".
func TestARefusalWithNothingToListStillReads(t *testing.T) {
	// **Serial**, for its neighbour's reason: `t.Setenv("PATH", ...)`, and the
	// emptied `PATH` is the fixture rather than an artefact of one.
	t.Setenv("PATH", t.TempDir())

	_, err := Settings{
		Java:       filepath.Join(t.TempDir(), "nope"),
		BundledJDK: t.TempDir(),
	}.JavaBinary()
	if err == nil {
		t.Skip("this machine found a JVM anyway")
	}
	if !strings.Contains(err.Error(), "Checked: nothing") {
		t.Errorf("with no candidates the refusal said %q", err)
	}
}
