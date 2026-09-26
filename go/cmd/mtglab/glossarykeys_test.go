package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/reference"
)

// The glossary keys, held to the table that serves them.
//
// `web/README.md` states the rule and, until this file, also admitted there
// was nothing behind it: *"a typo'd key fails **silently**: TypeScript cannot
// check a string against a JSON table, and a component test cannot either,
// because the glossary is mocked away there. The check that failed when a
// simulator control had no entry is gone; rebuilding it over the Go table is a
// queued item."* This is that rebuild.
//
// The silence is the whole problem. A `<Term name="tier-1">` whose key is
// missing renders as plain text with no affordance, which is deliberate and
// good -- it is what lets a word be marked up before its entry is written
// (`components/term.tsx` argues it) -- and it is also indistinguishable from a
// typo. So the surface a beginner leans on hardest is the one surface where
// being wrong looks exactly like being fine, and commandment 2 is the reason
// that matters: the glossary is how the site gets to use Magic's own word
// instead of a flatter one without shutting a newcomer out.
//
// # Why this is a Go test about TypeScript
//
// The authority is the **served** table, so the test reads
// [reference.Words] rather than re-parsing `glossary.json`. A Vitest sweep
// would have to reach across the repository into `go/internal/reference/data/`
// and parse the file, which asserts against a copy of the answer instead of
// against the answer. `datedcomments_test.go` already sweeps `web/src` from
// this package for the same reason.
//
// # The sweep is complete, and that is a checkable fact rather than a hope
//
// Both ways a key is written are read below, and the third assertion holds the
// **only** two sites that pass a key the extractor cannot see -- the one-line
// `help` helper in the two busiest routes -- against the shape whose callers
// the second regex does read. A third such indirection would fail that
// assertion rather than quietly shrink this guard's reach, which is the
// completeness lesson this repository keeps re-learning.

// glossaryProp is a key written straight onto the element: `<Term
// name="flood">`, `<HelpTip name="stat.card_lag" />`.
var glossaryProp = regexp.MustCompile(`<(?:Term|HelpTip)\s+name="([^"]*)"`)

// glossaryHelper is a key written through the routes' own one-liner:
// `help('sim.seed')`. Constrained to a key-shaped literal so that an
// unrelated `help(` -- should one ever exist -- is not read as a glossary key.
var glossaryHelper = regexp.MustCompile(`\bhelp\(['"]([a-z0-9._-]+)['"]\)`)

// glossaryIndirect is a key the extractor cannot follow: `name={…}`. Every
// one of these has to be a helper whose own callers are readable, and the
// third assertion below is where that is argued file by file.
var glossaryIndirect = regexp.MustCompile(`<(?:Term|HelpTip)\s+name=\{`)

// glossaryHelperFiles are the files allowed to pass a key indirectly, each
// because it defines exactly the one-line helper `glossaryHelper` reads. A
// file added here without that shape fails the third assertion; a file that
// grows a second indirection fails it too.
var glossaryHelperFiles = map[string]string{
	"routes/Simulator.tsx": "const help = (key: string) => <HelpTip name={key} />",
	"routes/Coliseum.tsx":  "const help = (key: string) => <HelpTip name={key} />",
}

// glossaryMark is one key as it was written, with where to look when it is
// wrong.
type glossaryMark struct {
	key  string
	rel  string
	line int
}

// glossaryMarks walks the frontend source and returns every key written in a
// shape that can be read, plus the per-file count of the ones that cannot.
func glossaryMarks(t *testing.T) (marks []glossaryMark, indirect map[string]int) {
	t.Helper()
	root := filepath.Join(repoRoot(t), "web", "src")
	indirect = map[string]int{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".tsx") && !strings.HasSuffix(path, ".ts") {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".test.tsx") || strings.HasSuffix(base, ".test.ts") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for n, line := range strings.Split(string(body), "\n") {
			for _, re := range []*regexp.Regexp{glossaryProp, glossaryHelper} {
				for _, hit := range re.FindAllStringSubmatch(line, -1) {
					marks = append(marks, glossaryMark{key: hit[1], rel: rel, line: n + 1})
				}
			}
			indirect[rel] += len(glossaryIndirect.FindAllString(line, -1))
		}
		if indirect[rel] == 0 {
			delete(indirect, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return marks, indirect
}

// TestEveryGlossaryMarkNamesATermTheServerServes is the rule.
func TestEveryGlossaryMarkNamesATermTheServerServes(t *testing.T) {
	t.Parallel()

	served := map[string]bool{}
	for _, term := range reference.Words().Terms {
		served[term.Key] = true
	}
	if len(served) < 20 {
		t.Fatalf("the served glossary holds %d terms, which is too few to be "+
			"the real table -- this guard would pass everything", len(served))
	}

	marks, _ := glossaryMarks(t)
	if len(marks) < 30 {
		t.Fatalf("found only %d glossary marks under web/src; the extractor has "+
			"stopped matching the way keys are written and an empty sweep "+
			"passes", len(marks))
	}

	for _, mark := range marks {
		if served[mark.key] {
			continue
		}
		t.Errorf(`%s:%d marks "%s", which %s serves no entry for. A missing key `+
			`renders as plain text with no affordance -- deliberately, so a word `+
			`can be marked up before its entry is written -- so a typo here is `+
			`invisible on the page. Either add the entry to the served table or `+
			`fix the spelling.`,
			mark.rel, mark.line, mark.key,
			filepath.Join("go", "internal", "reference", "data", "glossary.json"))
	}
	distinct := map[string]bool{}
	for _, mark := range marks {
		distinct[mark.key] = true
	}
	t.Logf("%d glossary marks under web/src naming %d distinct terms, over a "+
		"served table of %d", len(marks), len(distinct), len(served))
}

// TestBothWaysAGlossaryKeyIsWrittenAreRead is the anti-vacuity half: each
// extractor has to be carrying its own weight, or a guard that reads one shape
// passes a tree that has quietly moved to the other.
func TestBothWaysAGlossaryKeyIsWrittenAreRead(t *testing.T) {
	t.Parallel()

	root := filepath.Join(repoRoot(t), "web", "src")
	counts := map[string]int{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".tsx") || strings.HasSuffix(base, ".test.tsx") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		counts["prop"] += len(glossaryProp.FindAllString(string(body), -1))
		counts["helper"] += len(glossaryHelper.FindAllString(string(body), -1))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	for _, shape := range []string{"prop", "helper"} {
		if counts[shape] == 0 {
			t.Errorf("the %q extractor matches nothing in the tree. Either that "+
				"way of writing a key is gone -- in which case delete the "+
				"pattern rather than leaving a dead half -- or the pattern has "+
				"drifted from the code and is no longer guarding it.", shape)
		}
	}
}

// TestNoGlossaryKeyTravelsSomewhereThisGuardCannotFollow keeps the sweep
// complete. A `name={…}` is a key the extractor cannot read; the only ones
// allowed are the two routes' one-line helper, whose callers it does read.
func TestNoGlossaryKeyTravelsSomewhereThisGuardCannotFollow(t *testing.T) {
	t.Parallel()

	_, indirect := glossaryMarks(t)
	files := make([]string, 0, len(indirect))
	for rel := range indirect {
		files = append(files, rel)
	}
	sort.Strings(files)

	for _, rel := range files {
		shape, allowed := glossaryHelperFiles[rel]
		if !allowed {
			t.Errorf("%s passes a glossary key as `name={…}`, which this guard "+
				"cannot follow -- so that key is back to failing silently. Pass "+
				"a literal, or add the file here with the helper shape whose "+
				"callers the sweep reads.", rel)
			continue
		}
		if indirect[rel] != 1 {
			t.Errorf("%s has %d `name={…}` sites; it is allowed exactly one, the "+
				"helper. A second one is a key travelling out of this guard's "+
				"reach behind a file that already looks accounted for.",
				rel, indirect[rel])
		}
		body, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "src", rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), shape) {
			t.Errorf("%s no longer contains `%s`. It is exempted from the "+
				"literal-key rule only because of that one line, so if the "+
				"helper has changed shape the exemption has to be re-argued.",
				rel, shape)
		}
	}
	for rel := range glossaryHelperFiles {
		if _, found := indirect[rel]; !found {
			t.Errorf("%s is exempted here but passes no key indirectly any more. "+
				"Delete the exemption: a stale one is a hole waiting for the "+
				"next file to be added beside it.", rel)
		}
	}
}
