package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ADR 30's rule, made into something that fails.
//
// "Decks do not live in git and not on this laptop" is one of this project's
// absolute claims, and it was enforced by `.gitignore` -- which refuses an
// accident and not a `git add -f`, and which no check anywhere would ever go
// red over. The `image` job refuses a deck inside the container, so a *tracked*
// deck that `.dockerignore` keeps out of the image would sit in git with every
// check green. That matters beyond tidiness: a second standing copy of app data
// is the shape that quietly lost two rounds of deck labels, and the
// hosted-first facet of the polish pass exists because of it.
//
// So `ci.yml`'s tracked-file scan refuses deck paths now, and this holds that
// scan to the rule instead of restating it: **the pattern is read out of the
// workflow and executed**, never typed here. A session that deletes the
// alternative from `ci.yml` fails this test by name, on both Go legs, without
// waiting for a workflow to run.
//
// Why the cases below are in two lists rather than one table: the positives are
// the shapes ADR 30 and rule 5 forbid, and the negatives are the false
// positives this pattern could plausibly have -- a `.json` that is a manifest
// rather than Scryfall's, a fixture whose name ends in `-deck.yaml`. The second
// list is the one that earns its keep; the first list is what a careless
// widening would already satisfy.

// ciScanPattern lifts the extended regular expression out of the workflow's
// `grep -Ei` line. The anchor is the pipeline rather than the step's name,
// because a name is prose and gets reworded.
var ciScanPattern = regexp.MustCompile(`git ls-files \| grep -Ei \\\n\s*'\(([^']+)\)'`)

func trackedFileScan(t *testing.T) *regexp.Regexp {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := ciScanPattern.FindSubmatch(raw)
	if found == nil {
		t.Fatalf("no `git ls-files | grep -Ei '(...)'` scan found in %s -- "+
			"either the tracked-file scan is gone, in which case nothing "+
			"refuses a committed pool or a committed deck, or it was rewritten "+
			"and this guard has to be pointed at the new shape", path)
	}
	// `-i`, so the pattern is case-insensitive; `-E`, so it is POSIX extended,
	// which for this pattern's constructs is a subset of Go's syntax.
	body := strings.TrimSpace(string(found[1]))
	if body == "" {
		t.Fatal("the scan's alternation is empty, which would match nothing")
	}
	re, err := regexp.Compile("(?i)(" + body + ")")
	if err != nil {
		t.Fatalf("the workflow's scan pattern does not compile: %v\n%s", err, body)
	}
	return re
}

func TestTheTrackedFileScanRefusesDeckDataAndEveryOtherForbiddenShape(t *testing.T) {
	t.Parallel()
	scan := trackedFileScan(t)

	// What must never be tracked. The deck entries are this test's reason to
	// exist; the rest are here so a rewrite of the pattern that drops one of
	// the older rules fails here too.
	forbidden := []string{
		"decks/arahbo-cats/deck.yaml",
		"decks/arahbo-cats/swaps.md",
		"decks/arahbo-cats/primer.md",
		"decks/.snapshot/arahbo-cats.yaml",
		"deck.yaml",
		"go/testdata/deck.yaml",
		"data/mtg.duckdb",
		"tests/fixtures/pool.duckdb",
		"data/default_cards.json",
		"data/oracle_cards-2026-09-13.jsonl.gz",
		"app.db",
		".env",
		"my-collection.csv",
		"wishlist.txt",
	}
	for _, path := range forbidden {
		if !scan.MatchString(path) {
			t.Errorf("the scan would let %q be committed", path)
		}
	}

	// And the false positives the widening could have caused. Every one of
	// these is a real tracked path in this repository, so a pattern that
	// matched any of them would fail the build on `main`.
	allowed := []string{
		"package.json",
		"package-lock.json",
		"web/package.json",
		"web/tsconfig.json",
		"go/go.mod",
		"go/internal/deckyaml/testdata/rich-deck.yaml",
		"go/internal/gate/testdata/messy.yaml",
		"go/internal/pool/pooltest/testdata/tiny_pool.json",
		"go/internal/claude/data/modes.json",
		"web_dist/assets/app.js",
		"docs/HOSTING.md",
		".env.example",
		"fly.toml",
	}
	for _, path := range allowed {
		if scan.MatchString(path) {
			t.Errorf("the scan refuses %q, which is a tracked file in this "+
				"repository -- the pattern is too wide and `main` would go red",
				path)
		}
	}
}

// TestEveryFileInThisCheckoutPassesTheTrackedFileScan runs the workflow's own
// pattern over the tree the tests are reading, so the two lists above cannot
// drift away from reality while both stay green.
//
// It walks what is on disk rather than asking git, because a test that shells
// out to git is a test about the harness. The difference is real and runs the
// safe way: a file present but untracked -- somebody's scratch deck, a local
// pool -- would be flagged here and is not a CI failure, so the one thing this
// can do wrongly is fail a laptop for a file the laptop is allowed to have.
// That is the right direction for a guard about not committing things, and
// `checkoutFiles` names what a working checkout is allowed to hold.
func TestEveryFileInThisCheckoutPassesTheTrackedFileScan(t *testing.T) {
	t.Parallel()
	scan := trackedFileScan(t)
	root := repoRoot(t)
	files := checkoutFiles(t, root)
	for _, rel := range files {
		if scan.MatchString(rel) {
			t.Errorf("%s is in this checkout and the tracked-file scan refuses "+
				"it -- either it must not be committed (check `git ls-files`) or "+
				"the pattern is too wide", rel)
		}
	}
	// An empty walk would pass and prove nothing; the repository has over a
	// thousand tracked files, so a few hundred is a floor with slack in it.
	if len(files) < 300 {
		t.Fatalf("only %d files walked from %s, so this swept almost nothing",
			len(files), root)
	}
}

// checkoutFiles is every file under root, slash-separated and relative, less
// what a working checkout is allowed to hold untracked. Three kinds of that:
// the directories named in `skip` (app data, worktrees, dependencies -- and
// `decks/` most of all: the one-copy rule permits scratch there, which is the
// whole distinction between present and tracked); any directory carrying its
// own `.gitignore` of exactly `*`, which git could never track a file from and
// which every Python venv writes for itself; and any entry the root
// `.gitignore` names verbatim (`.env`, `.hypothesis/`). That last reading is
// deliberately literal -- a line with a glob character, a leading `!` or an
// inner slash is not followed -- and a line it cannot follow makes the walk
// stricter, never laxer, which is the safe direction for a guard about not
// committing things. Measured once on the dev Mac: two extra venvs, a
// hypothesis cache and a `.env` at the root failed this test for a hundred
// seconds of walking torch, and none of them was a fact about the code.
func checkoutFiles(t *testing.T, root string) []string {
	t.Helper()
	skip := map[string]bool{
		".git": true, "node_modules": true, ".venv": true,
		"decks": true, "data": true,
		// Worktrees of this same repository live here; each is its own tree.
		".claude": true,
	}
	ignored, ignoredDirs := literalIgnores(t, filepath.Join(root, ".gitignore"))
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if skip[name] || ignored[name] || ignoredDirs[name] || ignoresEverything(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if ignored[name] {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// literalIgnores reads the names a `.gitignore` lists verbatim: `names` for
// lines that match a file or a directory of that name at any depth, `dirs`
// for lines with git's trailing slash, which match directories only. A
// missing file is no ignores at all.
func literalIgnores(t *testing.T, path string) (names, dirs map[string]bool) {
	t.Helper()
	names, dirs = map[string]bool{}, map[string]bool{}
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return names, dirs
		}
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") ||
			strings.ContainsAny(line, `*?[\`) {
			continue
		}
		dir := strings.HasSuffix(line, "/")
		line = strings.TrimSuffix(line, "/")
		if strings.Contains(line, "/") {
			continue
		}
		if dir {
			dirs[line] = true
		} else {
			names[line] = true
		}
	}
	return names, dirs
}

// ignoresEverything reports a directory whose own `.gitignore` is the single
// rule `*` -- nothing under it can be tracked from within it.
func ignoresEverything(dir string) bool {
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	return err == nil && strings.TrimSpace(string(body)) == "*"
}

// TestTheCheckoutWalkSkipsWhatGitCouldNeverTrack builds the three shapes on
// a temporary root and reads the walk back: the venv's own `*`, the root
// file's literal names at every depth, the trailing-slash rule for
// directories only -- and the one it must NOT follow, a glob, so a pattern
// the reading cannot honour leaves the file in the walk rather than out.
func TestTheCheckoutWalkSkipsWhatGitCouldNeverTrack(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "# local\n.env\n.env.*\n.hypothesis/\n*.log\n!keep.log\nbuild/out\n")
	write(".env", "secret")
	write("sub/.env", "secret")
	write(".hypothesis/cache", "")
	write("sub/.hypothesis/cache", "")
	write("venv/.gitignore", "*\n")
	write("venv/lib/site.py", "")
	write("half/.gitignore", "*.pyc\n")
	write("half/kept.py", "")
	write("keep.txt", "")
	write("noise.log", "")
	// A FILE named like the directory rule stays: `.hypothesis/` is git's
	// directories-only spelling.
	write("sub2/.hypothesis", "")

	got := map[string]bool{}
	for _, rel := range checkoutFiles(t, root) {
		got[rel] = true
	}
	for _, want := range []string{"keep.txt", "half/kept.py", "half/.gitignore",
		"noise.log", "sub2/.hypothesis", ".gitignore"} {
		if !got[want] {
			t.Errorf("%s left the walk, and nothing in the ignore file says it may", want)
		}
	}
	for _, gone := range []string{".env", "sub/.env", ".hypothesis/cache",
		"sub/.hypothesis/cache", "venv/lib/site.py", "venv/.gitignore"} {
		if got[gone] {
			t.Errorf("%s is in the walk, and git could never track it", gone)
		}
	}
}
