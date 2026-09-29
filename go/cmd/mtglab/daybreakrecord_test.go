package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The daybreak queue's own rule is that a waiting item lives in two places:
// one answerable line in `docs/polish/DAYBREAK.md`, the full record in
// `docs/polish/LEDGER.md`'s own section. The rule breaks in one direction —
// writing the queue line is what a run remembers, writing the record is what
// it runs out of night for — and the consequence is worse than invisibility,
// because an answered item *leaves* the queue, so an item whose only copy is
// the queue is destroyed by being answered. Measured once: four of six items
// opened in one morning had no record anywhere else.
//
// This is the guard on that rule. Every open item must name a ledger section,
// and the section names are read out of `LEDGER.md`'s own `## ` headings
// rather than restated here, so a renamed section moves the expectation with
// it. Two deliberate conservatisms: an item is a paragraph carrying the
// queue's own `**Recommendation:**` marker (the file's stated contract — an
// item without one is not answerable with "yes"), and a pointer is any of the
// ledger's section names within a few words of the word "ledger", because the
// pointers in the wild are heterogeneous ("Ledger: Blue, 2026-09-05", "see
// White's ledger entry", "recorded in the ledger (White and Blue)"). Both can
// miss a malformed pointer; neither can invent one.
//
// And the guard refuses to pass on nothing: a queue with zero open items
// reads exactly like a broken extractor, so emptiness is a failure by design
// — the rare genuinely-empty morning is a deliberate visit to this test, not
// a silent green.

// ledgerSections reads the section names out of LEDGER.md's own headings --
// the text before the em-dash in each `## ` line ("## White — Law &
// Protection" names the section "White").
func ledgerSections(t *testing.T, root string) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "docs", "polish", "LEDGER.md"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, line := range strings.Split(string(body), "\n") {
		rest, ok := strings.CutPrefix(line, "## ")
		if !ok {
			continue
		}
		name := rest
		if before, _, cut := strings.Cut(rest, " — "); cut {
			name = before
		}
		// An empty name would substring-match everything and turn the
		// guard inert, which is the failure mode part two of the skill
		// warns a proposed guard about.
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		t.Fatalf("no `## ` headings found in LEDGER.md; with no section names the check would pass on anything")
	}
	return names
}

// openDaybreakItems is every paragraph under an `## Open` heading that
// carries the queue's own item marker. Paragraphs end at a blank line or at
// the next heading -- the file has run items flush against a heading before.
func openDaybreakItems(t *testing.T, root string) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "docs", "polish", "DAYBREAK.md"))
	if err != nil {
		t.Fatal(err)
	}
	var items []string
	var para []string
	inOpen := false
	flush := func() {
		if len(para) == 0 {
			return
		}
		p := strings.Join(para, "\n")
		para = nil
		if inOpen && strings.Contains(p, "**Recommendation:**") {
			items = append(items, p)
		}
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			inOpen = strings.HasPrefix(strings.TrimPrefix(line, "## "), "Open")
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		para = append(para, line)
	}
	flush()
	return items
}

var ledgerWord = regexp.MustCompile(`(?i)ledger`)

func TestEveryOpenDaybreakItemNamesALedgerSectionThatExists(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	names := ledgerSections(t, root)
	patterns := make([]*regexp.Regexp, len(names))
	for i, name := range names {
		patterns[i] = regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	}
	items := openDaybreakItems(t, root)
	if len(items) == 0 {
		t.Fatal("DAYBREAK.md has no open items; an empty queue reads exactly " +
			"like a broken extractor, so this guard fails on nothing by design " +
			"-- if the queue is honestly empty, that is news this test wants " +
			"a deliberate visit over")
	}
	for _, item := range items {
		if namesALedgerSection(item, patterns) {
			continue
		}
		line, _, _ := strings.Cut(item, "\n")
		t.Errorf("an open daybreak item names no ledger section that exists "+
			"(the sections are %v), so answering it would destroy its only "+
			"copy; it begins: %s", names, line)
	}
}

// openMarker is the token a waiting ledger record wears at the head of its
// bold lead -- `**(open) …**` -- struck when Aaron answers. It exists so the
// reverse direction below has something a test can read: "queued" in ledger
// prose never did, and that is how five items waited in the ledger alone for
// a month while the forward guard stayed green.
var openMarker = regexp.MustCompile(`\*\*\(open\)`)

// openMarkersBySection counts the `(open)` markers under each `## ` section
// of LEDGER.md, keyed by the section's name as ledgerSections reads it.
func openMarkersBySection(t *testing.T, root string) map[string]int {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "docs", "polish", "LEDGER.md"))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	section := ""
	for _, line := range strings.Split(string(body), "\n") {
		if rest, ok := strings.CutPrefix(line, "## "); ok {
			section = rest
			if before, _, cut := strings.Cut(rest, " — "); cut {
				section = before
			}
			section = strings.TrimSpace(section)
			continue
		}
		counts[section] += len(openMarker.FindAllStringIndex(line, -1))
	}
	return counts
}

// TestTheLedgerAndTheQueueAgreeOnWhatIsOpen is the reverse of the guard above,
// read off the marker. A section carrying an `(open)` record that no queue
// line points at is the ledger-only breach caught by name; a queue line whose
// section carries no `(open)` is a record nobody marked, which the next
// answer would leave undiscoverable. A queue line that names the Cleanup
// section is saying who carried it, not where its record lives, so that one
// pointer demands no marker; Cleanup's entries record rulings and carry none.
func TestTheLedgerAndTheQueueAgreeOnWhatIsOpen(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	names := ledgerSections(t, root)
	patterns := make([]*regexp.Regexp, len(names))
	for i, name := range names {
		patterns[i] = regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	}
	pointedAt := map[string]bool{}
	for _, item := range openDaybreakItems(t, root) {
		for i, p := range patterns {
			// "carried by Cleanup" is a note about who re-verified the item,
			// never where its record lives, so a pointer at Cleanup demands
			// no marker there.
			if names[i] != "Cleanup" && namesALedgerSection(item, []*regexp.Regexp{p}) {
				pointedAt[names[i]] = true
			}
		}
	}
	marked := openMarkersBySection(t, root)
	total := 0
	for section, n := range marked {
		total += n
		if n > 0 && !pointedAt[section] {
			t.Errorf("LEDGER.md's %s section carries %d record(s) marked "+
				"(open) and no open daybreak item points at %s -- the record is "+
				"waiting where nobody reads", section, n, section)
		}
	}
	for section := range pointedAt {
		if marked[section] == 0 {
			t.Errorf("open daybreak items point at LEDGER.md's %s section and no "+
				"record there is marked (open) -- mark the one they mean, or the "+
				"answer will strike a line with no record behind it", section)
		}
	}
	if total == 0 {
		t.Fatal("LEDGER.md carries no (open) marker at all, which reads exactly " +
			"like a broken extractor; an honestly empty ledger is a deliberate " +
			"visit to this test")
	}
}

// namesALedgerSection looks for a known section name within a few words of
// any mention of the ledger, either side, which covers every pointer shape
// the queue actually writes.
func namesALedgerSection(item string, patterns []*regexp.Regexp) bool {
	for _, loc := range ledgerWord.FindAllStringIndex(item, -1) {
		lo := max(loc[0]-60, 0)
		hi := min(loc[1]+60, len(item))
		for _, p := range patterns {
			if p.MatchString(item[lo:hi]) {
				return true
			}
		}
	}
	return false
}
