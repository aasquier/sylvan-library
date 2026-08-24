package tier3

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
)

// The deck exporter: deck.yaml -> Forge's `.dck` format.
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
	// two exports diffable. The recorded sort is stable, and
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

// SlugPattern is `service._SLUG`, checked here as well as at the door.
//
// **A slug reaches this function by two roads and only one of them has a
// gate.** `POST /api/decks` and `POST /api/decks/import` both refuse a slug
// that is not this shape, because a slug becomes a directory name. But a deck
// can also arrive as deck.yaml *text* — over the private network, from the
// app to the worker — and `deck.FromText` reads whatever `slug:` says. The
// worker is a separate machine precisely so that somebody else's rules engine
// is contained; trusting the caller's text to name a file is the wrong way
// round.
//
// It is not hypothetical in either direction. `<slug>.dck` is written into
// Forge's profile directory, so `../../..` escapes it; and the filename goes
// on Forge's own command line after `-d`, so a slug beginning with `-` is read
// as a flag rather than as a deck. Both are shut by the same six characters of
// alphabet.
var SlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// WriteDck writes `<directory>/<slug>.dck` and returns the path.
//
// The filename is the slug, not the deck name: `forge sim -d` takes deck names
// on a command line, and a slug needs no quoting.
//
// **Refused rather than sanitised.** A cleaned-up filename is a file nobody
// asked for, and the coverage report's `slug` would then name something other
// than what is on disk — which is the class of silent disagreement this whole
// package exists to prevent.
func WriteDck(d *deck.Deck, directory string, names map[string]string) (string, error) {
	if !SlugPattern.MatchString(d.Slug) {
		return "", fmt.Errorf("%q is not a usable slug -- lowercase letters, "+
			"digits and single hyphens, e.g. 'arahbo-cats'", d.Slug)
	}
	if err := os.MkdirAll(directory, 0o755); err != nil { //nolint:gosec // scratch input; Forge reads this directory as the same user
		return "", err
	}
	path := filepath.Join(directory, d.Slug+".dck")
	if err := os.WriteFile(path, []byte(ToDck(d, names)), 0o644); err != nil { //nolint:gosec // a `.dck` is scratch input to a simulator, read as the same user
		return "", err
	}
	return path, nil
}
