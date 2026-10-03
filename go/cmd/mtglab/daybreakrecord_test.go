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

// queueCountRecipe pulls the `grep -cE '<pattern>'` that DAYBREAK.md's own
// prose tells a reader to count open items with, out of the fenced block it
// is written in. The pattern is read rather than restated on purpose: a test
// that repeats the recipe cannot tell you the recipe is wrong, and the recipe
// is the half that has actually been wrong.
func queueCountRecipe(t *testing.T, root string) *regexp.Regexp {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "docs", "polish", "DAYBREAK.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		_, rest, ok := strings.Cut(line, "grep -cE '")
		if !ok {
			continue
		}
		pattern, _, ok := strings.Cut(rest, "'")
		if !ok || pattern == "" {
			continue
		}
		expr, err := regexp.Compile("(?m)" + pattern)
		if err != nil {
			t.Fatalf("DAYBREAK.md's own count recipe does not compile as a Go "+
				"regexp (%q): %v -- the recipe is what the morning counts with, "+
				"so a broken one is a broken count", pattern, err)
		}
		return expr
	}
	t.Fatal("DAYBREAK.md carries no `grep -cE '…'` recipe; without one this " +
		"guard has nothing to hold the extractor against and would pass on " +
		"anything")
	return nil
}

// TestTheQueuesOwnCountRecipeSeesEveryItem holds the morning's count to the
// morning's items. The queue is counted by a grep over item headings and
// acted on through the extractor above, and the two definitions can disagree
// in both directions: a heading the recipe cannot see is an item missing from
// every count ever quoted (measured once: a `**White (leg two):` heading and
// one sibling hid for a week), and a recipe hit inside an item's body is a
// phantom item the count invents. Nothing held them equal, which is the shape
// of claim this pass's standing question exists to find -- the convention
// "every heading starts `**Colour:`" was prose, and prose drifts.
func TestTheQueuesOwnCountRecipeSeesEveryItem(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	recipe := queueCountRecipe(t, root)
	body, err := os.ReadFile(filepath.Join(root, "docs", "polish", "DAYBREAK.md"))
	if err != nil {
		t.Fatal(err)
	}
	counted := len(recipe.FindAllStringIndex(string(body), -1))
	items := openDaybreakItems(t, root)
	if len(items) == 0 {
		t.Fatal("DAYBREAK.md has no open items; see the guard above -- an empty " +
			"queue reads exactly like a broken extractor")
	}
	// Name the offender rather than only the arithmetic: a bare "9 != 10" sends
	// the next session counting paragraphs by hand.
	for _, item := range items {
		head, _, _ := strings.Cut(item, "\n")
		if !recipe.MatchString(head) {
			t.Errorf("an open daybreak item's heading is invisible to the file's "+
				"own count recipe (%s), so every count quoted of this queue is "+
				"short by one; the heading is: %s", recipe, head)
		}
	}
	if counted != len(items) {
		t.Errorf("DAYBREAK.md's own recipe counts %d open items and the queue "+
			"holds %d; a recipe hit outside an item heading is a phantom the "+
			"morning count invents, and a heading the recipe misses is an item "+
			"nobody counts", counted, len(items))
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

// configAnchor matches the backticked tokens this guard holds: a `.yml` or a
// `.toml`, the two extensions that in this repository can only name a file the
// tree actually carries -- a workflow or a manifest.
//
// The two it deliberately does not hold are the whole argument. `.yaml` is a
// deck (ADR 30: the library lives on the volume and is gitignored), so a queue
// line naming one would be flagged for existing in the only place it is
// allowed to exist. `.sh` and `.md` are worse: a queue names what does **not**
// exist yet, which is what a queue is for -- measured 2026-10-03, the one open
// Colorless item names `gowrap.sh`, `deploy.sh`, `poll.sh` and `LANE_BRIEF.md`
// precisely because they live in a scratch directory and vanish with it, so
// four of five flags from the wider extractor were the queue working. That is
// the difference between this file and the records `licenserecord_test.go` and
// `skillrecord_test.go` hold: a record names what is, a queue names what
// should be. It is also why `LEDGER.md` is not held by anything here -- it is
// history, and history is supposed to name files that were deleted.
//
// Backticks are the anchor, which cuts both ways and is the rule to know: a
// queue line that discusses a name *because it does not exist* -- the one this
// guard was written for does exactly that -- writes it without them, and so
// does a sentence naming a bare extension. Both shapes failed the first run of
// this test on the same branch that wrote it, which is the cheapest possible
// proof that the marker means something.
//
// The leading `\.?[A-Za-z0-9_]` is why `.github/workflows/ci.yml` matches and
// a bare "`.yml`" in a sentence about extensions does not.
var configAnchor = regexp.MustCompile("`(\\.?[A-Za-z0-9_][A-Za-z0-9_./-]*\\.(?:yml|toml))`")

// resolvesInCheckout answers whether a path a document names is a file in this
// tree, allowing the path to be written relative to whatever directory the
// sentence is standing in -- `ci.yml` for `.github/workflows/ci.yml`. These
// documents speak from inside `go/`, from the repository root and from the
// skill directory in consecutive paragraphs, and a resolver that only tries
// the root calls 19 of `COVERAGE.md`'s 20 slashed paths broken.
func resolvesInCheckout(token string, files []string) bool {
	suffix := "/" + token
	for _, f := range files {
		if f == token || strings.HasSuffix(f, suffix) {
			return true
		}
	}
	return false
}

// TestTheQueueNamesConfigurationFilesThatExist holds the daybreak queue's
// workflow and manifest anchors to the tree. The rot it was written for was
// live for a month and cost a reader the whole trip: the deploy-snapshot item
// said the snapshot step was missing from `deploy.yml`, and this repository has
// never had such a file -- continuous deployment is the `deploy` job inside
// `.github/workflows/ci.yml`. Three cleanups re-verified the item's *claim*
// (still no snapshot step, true every time) and none of them opened the file
// named, because the name reads exactly like a file a repository would have.
//
// The extractor is proved against a built body first, so that a queue which
// happens to name no configuration file cannot turn this guard inert without
// anybody noticing -- the sibling guards above refuse to pass on an empty
// queue, which is right for them and wrong here: a morning with no workflow
// question in it is an ordinary morning, not a broken extractor.
func TestTheQueueNamesConfigurationFilesThatExist(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	files := checkoutFiles(t, root)

	built := "a line about `.github/workflows/ci.yml` and one about `deploy.yml`.\n" +
		"A deck in `decks/x/deck.yaml` and a script in `gowrap.sh` are not anchors."
	var found []string
	for _, m := range configAnchor.FindAllStringSubmatch(built, -1) {
		found = append(found, m[1])
	}
	if len(found) != 2 {
		t.Fatalf("the extractor read %v out of a body holding exactly two "+
			"configuration anchors; a miss here makes every green below "+
			"meaningless", found)
	}
	if !resolvesInCheckout(found[0], files) {
		t.Errorf("%q does not resolve in this checkout, and it is this "+
			"repository's own workflow file", found[0])
	}
	if resolvesInCheckout(found[1], files) {
		t.Errorf("%q resolves in this checkout; the guard below can no longer "+
			"fail, so delete it or find the rot it was written for", found[1])
	}

	body, err := os.ReadFile(filepath.Join(root, "docs", "polish", "DAYBREAK.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range configAnchor.FindAllStringSubmatch(string(body), -1) {
		token := m[1]
		if !resolvesInCheckout(token, files) {
			t.Errorf("docs/polish/DAYBREAK.md names the configuration file %q "+
				"and no file in this checkout answers to that path; a queue "+
				"line is read by somebody who then opens the file it names, so "+
				"either the path is stale (say where the thing moved to) or the "+
				"file was never there (which is how `deploy.yml` sent three "+
				"cleanups to a path this repository has never had)", token)
		}
	}
}
