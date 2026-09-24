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
// **All of these run in parallel**, and the road here was three separate
// injections. ADR 40 took the first: the discovery functions read
// `MTGLAB_FORGE_HOME`, `MTGLAB_JAVA` and `MTGLAB_FORGE_PROFILE` from the
// process, so a test could only describe a machine by writing the process --
// `t.Setenv`, which Go panics on inside a parallel test. A [Settings] literal
// says the same thing to one caller instead of to every goroutine in the
// binary.
//
// The other two were left behind and are closed here. The coverage index was
// package-level state with a `ClearIndex` for the suite to call, so the five
// tests that asked what it had remembered needed the whole process; it is a
// [CardIndex] now and each of them builds its own. And the JVM search called
// `exec.LookPath`, so the only way to describe a machine with no Java on it was
// to empty the `PATH` of every test in the binary; [Settings.PathList] is that
// list as a value.
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

// remembering is [installedAt] with a card index of its own -- one machine's
// memory, belonging to one test, which is the whole of what used to be a
// package-level map and a `ClearIndex`.
func remembering(home string) Settings {
	return Settings{Home: home, Index: NewCardIndex()}
}

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
func TestTheCardIndexIsReadFromForgesOwnScripts(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50", "Llanowar Elves", "Sol Ring", "Forest")

	names, err := remembering(home).ImplementedNames()
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
func TestTheCardIndexIsCachedAndAnUpgradeInvalidatesIt(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50", "Llanowar Elves")
	machine := remembering(home)

	if _, err := machine.ImplementedNames(); err != nil {
		t.Fatal(err)
	}
	hits, misses := machine.Index.Stats()
	if hits != 0 || misses != 1 {
		t.Fatalf("the first read was %d hits and %d misses", hits, misses)
	}

	if _, err := machine.ImplementedNames(); err != nil {
		t.Fatal(err)
	}
	if hits, misses = machine.Index.Stats(); hits != 1 || misses != 1 {
		t.Fatalf("the second read was %d hits and %d misses -- the cache never hit", hits, misses)
	}

	// A copy of the settings is the same machine, so it shares the memory: the
	// index is carried by pointer precisely because `Settings` is copied by
	// value everywhere and one distribution must not be scanned twice.
	if _, err := machine.At(home).ImplementedNames(); err != nil {
		t.Fatal(err)
	}
	if hits, _ = machine.Index.Stats(); hits != 2 {
		t.Errorf("a copy of the settings missed the index (hits=%d)", hits)
	}

	// An upgrade in place: same path, different bytes.
	writeCardsfolder(t, home, "Llanowar Elves", "Craterhoof Behemoth")
	if err := os.Chtimes(filepath.Join(home, cardsfolder),
		nowPlus(t, 120), nowPlus(t, 120)); err != nil {
		t.Fatal(err)
	}
	names, err := machine.ImplementedNames()
	if err != nil {
		t.Fatal(err)
	}
	if !names["Craterhoof Behemoth"] {
		t.Error("the upgraded distribution served the stale index")
	}
	if _, misses = machine.Index.Stats(); misses != 2 {
		t.Errorf("an upgrade did not miss the cache (misses=%d)", misses)
	}
}

// A machine with no index reads the card scripts every time rather than
// crashing on the memory it does not have -- which is what a [Settings] literal
// is, and what every test in this file that asks once relies on.
func TestSettingsWithNoIndexReadTheScriptsEveryTime(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50", "Llanowar Elves")
	forgetful := installedAt(home)
	if forgetful.Index != nil {
		t.Fatal("a bare Settings literal arrived with an index")
	}
	for i := range 2 {
		names, err := forgetful.ImplementedNames()
		if err != nil {
			t.Fatalf("read %d: %v", i+1, err)
		}
		if !names["Llanowar Elves"] {
			t.Errorf("read %d lost the card", i+1)
		}
	}
	// Nil counts nothing, because nothing happened to a cache that is not
	// there -- and a caller asking is not a caller crashing.
	if hits, misses := forgetful.Index.Stats(); hits != 0 || misses != 0 {
		t.Errorf("a nil index reported %d hits and %d misses", hits, misses)
	}
	// And the loaded configuration always has one, which is what makes the
	// deployed worker scan its 33,000 card scripts once rather than per
	// request.
	if LoadSettingsFrom(lookup(nil)).Index == nil {
		t.Error("a loaded configuration carries no card index")
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
func TestAnUnreadableCardsfolderIsRefusedRatherThanReadAsEmpty(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := filepath.Join(home, cardsfolder)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("this is not a zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := remembering(home).ImplementedNames(); err == nil {
		t.Fatal("a corrupt cardsfolder was read as an empty index")
	} else if !strings.Contains(err.Error(), "unreadable") {
		t.Errorf("the refusal said %q", err)
	}
}

// The pre-flight reads a zip and needs no Java at all, which is what makes it
// the cheap check an API can run on a request thread.
func TestThePreFlightRunsWithoutAJVMAndNamesWhatIsMissing(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50", "Sol Ring", "Forest")

	covered := &deck.Deck{Slug: "covered",
		Commander: []string{"Sol Ring"},
		Cards:     []deck.CardEntry{{Name: "Forest"}},
	}
	reports, err := remembering(home).CheckCoverage([]*deck.Deck{covered})
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
func TestThePreFlightCountsEachCardOnceAndIncludesTheCommandZone(t *testing.T) {
	t.Parallel()
	home := fakeForge(t, "1.6.50", "Sol Ring", "Forest", "Kaheera, the Orphanguard")
	companion := "Kaheera, the Orphanguard"

	d := &deck.Deck{Slug: "d",
		Commander: []string{"Sol Ring"},
		Companion: &companion,
		Cards: []deck.CardEntry{
			{Name: "Forest"}, {Name: "Forest"}, {Name: "Forest"},
		},
	}
	index, err := remembering(home).ImplementedNames()
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

// lookup is an environment described rather than installed: the
// `func(string) string` [LoadSettingsFrom] reads, backed by a map this test
// owns. Absent is the empty string, which is what [os.Getenv] says too.
func lookup(env map[string]string) func(string) string {
	return func(name string) string { return env[name] }
}

// The environment overrides exist so an operator can point at an install this
// code would never have guessed, and each falls back to the same
// `~/.local/share/mtglab` layout.
//
// **The only test here that is about the environment at all.**
// [LoadSettingsFrom] is the one reader in the process (ADR 40), so this is
// where reading it is tested; everything else in this file describes a machine
// with a literal. It parallelises because the environment it reads is a map
// rather than the process -- the injection ADR 39 gave `config.LoadFrom`,
// arriving here last.
func TestTheOverridesWinAndTheFallbacksAgreeOnTheLayout(t *testing.T) {
	t.Parallel()
	loaded := LoadSettingsFrom(lookup(map[string]string{
		"MTGLAB_FORGE_HOME":      "/somewhere/else",
		"MTGLAB_FORGE_PROFILE":   "/profile/here",
		"MTGLAB_JAVA":            "/jvm/here",
		"MTGLAB_FORGE_MACHINE":   "another-worker",
		"MTGLAB_FORGE_SHIM_PORT": "9999",
		// A trailing slash is trimmed here rather than at every call site --
		// otherwise the client would request every path with a double slash.
		"MTGLAB_FORGE_WORKER_URL": "http://shim.internal:8080/",
		"PATH":                    "/usr/local/bin:/usr/bin",
	}))
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
	// The executable search path is carried whole rather than parsed here: the
	// JVM hunt walks it, and this is the one read of it.
	if loaded.PathList != "/usr/local/bin:/usr/bin" {
		t.Errorf("the search path is %q", loaded.PathList)
	}

	// An environment that says nothing at all, which is a laptop that exported
	// nothing -- and whose home this test now gets to name.
	home := t.TempDir()
	base := filepath.Join(home, ".local", "share", "mtglab")
	fell := LoadSettingsFrom(lookup(map[string]string{"HOME": home}))
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
	typo := LoadSettingsFrom(lookup(map[string]string{
		"MTGLAB_FORGE_SHIM_PORT":    "banana",
		"MTGLAB_FORGE_IDLE_SECONDS": "   ",
		"MTGLAB_FORGE_MEMORY_MB":    "lots",
	}))
	if typo.ShimPort != DefaultShimPort {
		t.Errorf("an unparseable port resolved to %d, want the default", typo.ShimPort)
	}
	if typo.IdleSeconds != DefaultIdleSeconds || typo.MemoryMB != DefaultMemoryMB {
		t.Errorf("the other two numbers fell to %d and %d", typo.IdleSeconds, typo.MemoryMB)
	}

	// The rest of the block, each read once, because a variable that is read
	// nowhere is indistinguishable from one that is read wrong.
	rest := LoadSettingsFrom(lookup(map[string]string{
		"MTGLAB_FORGE_SHIM_HOST":  "127.0.0.1",
		"MTGLAB_FORGE_SHIM_TOKEN": "  a-bearer  ",
		"MTGLAB_FORGE_WORKER":     "1",
		"MTGLAB_FLY_API_TOKEN":    "a-deploy-token",
		"MTGLAB_SCRIBE_CLASSES":   "/opt/scribe",
	}))
	if rest.ShimHost != "127.0.0.1" {
		t.Errorf("the shim host is %q", rest.ShimHost)
	}
	// Trimmed, because a header value with a stray newline in it is a header
	// the far side refuses for a reason nobody can see.
	if rest.ShimToken != "a-bearer" {
		t.Errorf("the shim token is %q", rest.ShimToken)
	}
	if !rest.WorkerEnabled || !rest.Configured() {
		t.Errorf("the dial and a token did not configure a worker: %+v", rest.WorkerEnabled)
	}
	if rest.ScribeClasses != "/opt/scribe" {
		t.Errorf("the scribe classes are %q", rest.ScribeClasses)
	}
}

// A home directory is answered from the lookup first and from the user
// database second, which is the container's shape rather than a laptop's --
// and the reason the second half is there at all is that a machine may have no
// `HOME` and still have a user.
func TestTheHomeDirectoryIsReadFromTheLookupFirst(t *testing.T) {
	t.Parallel()
	if got := homeDirFrom(lookup(map[string]string{"HOME": "/home/squire"})); got != "/home/squire" {
		t.Errorf("the lookup's home lost: %q", got)
	}
	// Nothing in the lookup falls through to the user database, which on every
	// machine this project runs on answers something.
	if got := homeDirFrom(lookup(nil)); got == "" {
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
	t.Parallel()
	// A file that exists and is not a JVM, so the probe fails rather than
	// the stat.
	fake := filepath.Join(t.TempDir(), "java")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil { //nolint:gosec // a test's own temp dir
		t.Fatal(err)
	}
	// A search path with nothing named java on it -- said as a value, which is
	// the whole of why this no longer has to empty the process's own.
	_, err := Settings{
		Java: fake, BundledJDK: t.TempDir(), PathList: t.TempDir(),
	}.JavaBinary()
	if err == nil {
		t.Fatal("a machine with no JVM on any candidate path found one")
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
	t.Parallel()
	_, err := Settings{
		Java:       filepath.Join(t.TempDir(), "nope"),
		BundledJDK: t.TempDir(),
	}.JavaBinary()
	if err == nil {
		t.Fatal("a machine with nothing on any path found a JVM")
	}
	if !strings.Contains(err.Error(), "Checked: nothing") {
		t.Errorf("with no candidates the refusal said %q", err)
	}
}

// The search path is walked in order and only an executable file on it counts
// -- a directory called `java`, or a data file, is not a JVM, and neither is an
// empty entry.
func TestTheJavaSearchWalksTheGivenPathInOrder(t *testing.T) {
	t.Parallel()
	first, second := t.TempDir(), t.TempDir()
	// A directory named java on the first entry, which must be stepped over
	// rather than handed back as a binary.
	if err := os.Mkdir(filepath.Join(first, "java"), 0o750); err != nil {
		t.Fatal(err)
	}
	// And a file on the second that is not executable.
	plain := t.TempDir()
	if err := os.WriteFile(filepath.Join(plain, "java"), []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(second, "java")
	if err := os.WriteFile(real, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { //nolint:gosec // a test's own temp dir
		t.Fatal(err)
	}

	// An empty entry is skipped rather than resolved against the working
	// directory, which is what a trailing colon on a real `PATH` means.
	list := strings.Join([]string{first, plain, "", second}, string(os.PathListSeparator))
	found, ok := Settings{PathList: list}.javaOnPath()
	if !ok {
		t.Fatal("the executable at the end of the path was not found")
	}
	if found != real {
		t.Errorf("the search found %q, want %q", found, real)
	}
	if _, ok := (Settings{}).javaOnPath(); ok {
		t.Error("an empty search path produced a JVM")
	}
}
