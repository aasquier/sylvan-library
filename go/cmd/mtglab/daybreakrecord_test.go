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
