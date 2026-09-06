package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The skills under `.claude/skills/` are instructions to future Claude
// sessions, and they rot at exactly the rate the tree moves: one refresh
// found a command that does not exist, a path that had moved, and a test
// cited as the model of good practice that had been deleted -- each
// mechanically checkable and none mechanically checked. This file is that
// check, reusing the licensing record's extraction (`repoPaths`,
// `mtglabVerb`) so the two guards drift together or not at all.
//
// The one thing skill prose does that the licensing record does not: it
// writes paths against more than one implied root. `docs/polish/LEDGER.md`
// is repository-relative, `internal/pool` is written from `go/`,
// `lib/api.ts` from `web/src/`, and `references/white.md` from the skill's
// own directory. So a token resolves if it exists under *any* of those
// roots, and fails only when it exists under none -- measured before this
// was built: a single-root check would have failed 25 times on a healthy
// tree. (A fifth implied root, `go/internal/`, served only extension-less
// directory shorthand like `sim/tier1`, which the files-only conservatism
// inherited from `repoPaths` already skips; an unused root is one more
// place a dead token could false-pass, so it is left out until a real
// token needs it.)

// scratchPrefixes are file names a skill writes relative to a session
// directory it builds at runtime rather than to this repository -- the scry
// skill's `<scratchpad>/scry/<slug>/` layout, declared in its own SKILL.md,
// keeps its fetched EDHRec pages under `edhrec/`. Restating a list is
// normally the wrong move here, and it is the right one for the same reason
// as `repoExtensions`: a wrong entry means a missed stale anchor, never a
// false alarm, and a new scratch convention fails loudly until it is argued
// onto this list.
var scratchPrefixes = []string{"edhrec/"}

// skillDocs walks `.claude/skills` for every markdown file -- discovered,
// never listed, so a new skill joins the check by existing.
func skillDocs(t *testing.T) (root string, files []string) {
	t.Helper()
	root = repoRoot(t)
	err := filepath.WalkDir(filepath.Join(root, ".claude", "skills"),
		func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, rel)
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	// Anti-vacuity: a walk that found nothing reads exactly like a walk
	// that found nothing wrong.
	found := false
	for _, f := range files {
		if f == filepath.Join(".claude", "skills", "polish", "SKILL.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the walk found no polish SKILL.md; it found %v", files)
	}
	return root, files
}

// skillDir is the skill's top directory for one of its files --
// `references/white.md` in a reference file resolves against the skill,
// never against the reference file's own directory.
func skillDir(rel string) string {
	parts := strings.SplitN(filepath.ToSlash(rel), "/", 4)
	return filepath.Join(parts[0], parts[1], parts[2])
}

func TestTheSkillsNameOnlyPathsThatResolve(t *testing.T) {
	t.Parallel()
	root, files := skillDocs(t)
	checked := 0
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		roots := []string{
			root,
			filepath.Join(root, "go"),
			filepath.Join(root, "web", "src"),
			filepath.Join(root, skillDir(rel)),
		}
	tokens:
		for _, p := range repoPaths(string(body)) {
			for _, prefix := range scratchPrefixes {
				if strings.HasPrefix(p, prefix) {
					continue tokens
				}
			}
			checked++
			for _, r := range roots {
				if _, err := os.Stat(filepath.Join(r, p)); err == nil {
					continue tokens
				}
			}
			t.Errorf("%s names %s, which resolves under no root "+
				"(the repository, go/, web/src/, or the skill's own directory)",
				rel, p)
		}
	}
	// The extractor silently skipping everything would also pass.
	if checked < len(files) {
		t.Errorf("only %d paths found across %d skill documents; the "+
			"extractor has probably stopped matching", checked, len(files))
	}
}

func TestTheSkillsNameNoCommandTheBinaryLacks(t *testing.T) {
	t.Parallel()
	have := map[string]bool{}
	for _, c := range newRoot(config.Config{}, tier3.Settings{}, claude.Endpoint{}).Commands() {
		have[c.Name()] = true
		for _, alias := range c.Aliases {
			have[alias] = true
		}
	}
	if len(have) == 0 {
		t.Fatal("the root command has no subcommands; the check would pass on anything")
	}
	root, files := skillDocs(t)
	matched := 0
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range mtglabVerb.FindAllStringSubmatch(string(body), -1) {
			matched++
			if !have[m[1]] {
				t.Errorf("%s tells a reader to run `mtglab %s`, and this "+
					"binary has no such subcommand (it has %v)",
					rel, m[1], sortedKeys(have))
			}
		}
	}
	if matched == 0 {
		t.Error("no `mtglab <verb>` found in any skill; the extractor has " +
			"probably stopped matching")
	}
}

// addParser reads the toolbox's own command table: every subcommand animist
// has is an add_parser call in its cli, so the expectation moves with the
// tool rather than with a list here.
var addParser = regexp.MustCompile(`add_parser\(\s*"([a-z-]+)"`)

// animistVerb is matched only inside code spans, because "animist" is also
// an English noun in this prose ("the animist toolbox") where the next word
// is never a subcommand.
var animistVerb = regexp.MustCompile(`\banimist\s+([a-z][a-z-]*)`)

// codeSpans is every inline-backticked span plus every line inside a fenced
// block -- the two places prose tells a reader to run something.
func codeSpans(body string) []string {
	var out []string
	for _, m := range backticked.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	fenced := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			out = append(out, line)
		}
	}
	return out
}

func TestTheSkillsNameNoCommandTheToolboxLacks(t *testing.T) {
	t.Parallel()
	root, files := skillDocs(t)
	body, err := os.ReadFile(filepath.Join(root, "tools", "animist", "cli.py"))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, m := range addParser.FindAllStringSubmatch(string(body), -1) {
		have[m[1]] = true
	}
	if len(have) == 0 {
		t.Fatal("no add_parser calls found in the animist cli; the check would pass on anything")
	}
	matched := 0
	for _, rel := range files {
		doc, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, span := range codeSpans(string(doc)) {
			for _, m := range animistVerb.FindAllStringSubmatch(span, -1) {
				matched++
				if !have[m[1]] {
					t.Errorf("%s tells a reader to run `animist %s`, and the "+
						"toolbox has no such subcommand (it has %v)",
						rel, m[1], sortedKeys(have))
				}
			}
		}
	}
	if matched == 0 {
		t.Error("no `animist <verb>` found in any skill's code spans; the " +
			"extractor has probably stopped matching")
	}
}
