package tier3

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// `sim/tier3/dck.py`: deck.yaml -> Forge's `.dck` format.
//
// The format, read off the 13,994 `.dck` files Forge ships rather than
// guessed:
//
//	[metadata]
//	name=Arahbo, Roar of the World - Cats
//	[Commander]
//	1 Arahbo, Roar of the World
//	[Main]
//	1 Sol Ring
//	36 Forest
//	[Sideboard]
//	1 Kaheera, the Orphanguard
//
// Sections are the ten in `forge.deck.DeckSection`; the four above are the
// ones a Commander deck uses. A line is `<qty> <name>`, optionally
// `|SET|<number>` to pin a printing. **We never pin one.** deck.yaml records
// no set, `mtglab price deck` already owns which printing to buy, and pinning
// would turn a Forge-side edition rename into a mysteriously missing card.
//
// The companion goes in `[Sideboard]` because Forge has no companion section
// — checked against the enum in the shipped jar, not assumed. That is also
// where the rules put it.
//
// **A `.dck` is a temporary file, never an artifact.** CLAUDE.md rule 3 fixes
// the deliverables at five, and this is not a sixth: it is an input to a
// simulator, written into a scratch directory for the length of a run.

// Forge parses section headers case-insensitively (`compareToIgnoreCase` in
// DeckSection), but its own files are written in this casing and matching them
// means a `.dck` we produce is diffable against one Forge produced.
const (
	SectionCommander = "[Commander]"
	SectionMain      = "[Main]"
	SectionSideboard = "[Sideboard]"
)

// ToDck renders a deck as `.dck` text.
//
// `names` maps a deck.yaml name to the name Forge knows it by, and is
// [CoverageReport.Resolved] in practice — so what gets written is exactly what
// the pre-flight verified Forge implements. Nil, every name is used as-is,
// which is right for the six curated decks (none has a `//` name) and is what
// makes this function testable without a Forge install.
//
// A name missing from `names` is written unchanged rather than dropped.
// Dropping it here would reproduce, inside our own code, the exact silent
// failure the pre-flight exists to catch.
func ToDck(d *deck.Deck, names map[string]string) string {
	forgeName := func(name string) string {
		if got, ok := names[name]; ok {
			return got
		}
		return name
	}

	lines := []string{"[metadata]", "name=" + d.Name}

	if len(d.Commander) > 0 {
		lines = append(lines, SectionCommander)
		for _, c := range d.Commander {
			lines = append(lines, cardLine(1, forgeName(c)))
		}
	}

	lines = append(lines, SectionMain)
	// deck.yaml order is by category, which is how a human reads the deck.
	// Sorted here because a `.dck` is machine input, and a stable order makes
	// two exports diffable. `sorted(key=...)` is stable in Python, and
	// `sort.SliceStable` is what reproduces that for two cards of one name.
	entries := make([]deck.CardEntry, len(d.Cards))
	copy(entries, d.Cards)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	for _, entry := range entries {
		lines = append(lines, cardLine(entry.Qty, forgeName(entry.Name)))
	}

	// Always emitted, even when empty — every `.dck` Forge ships ends with it.
	lines = append(lines, SectionSideboard)
	if d.Companion != nil {
		lines = append(lines, cardLine(1, forgeName(*d.Companion)))
	}

	return strings.Join(lines, "\n") + "\n"
}

func cardLine(qty int, name string) string {
	return fmt.Sprintf("%d %s", qty, name)
}

// WriteDck writes `<directory>/<slug>.dck` and returns the path.
//
// The filename is the slug, not the deck name: `forge sim -d` takes deck names
// on a command line, and a slug needs no quoting.
func WriteDck(d *deck.Deck, directory string, names map[string]string) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil { //nolint:gosec // matches Python's umask; Forge reads this directory as the same user
		return "", err
	}
	path := filepath.Join(directory, d.Slug+".dck")
	if err := os.WriteFile(path, []byte(ToDck(d, names)), 0o644); err != nil { //nolint:gosec // matches Python's umask; a `.dck` is scratch input to a simulator
		return "", err
	}
	return path, nil
}
