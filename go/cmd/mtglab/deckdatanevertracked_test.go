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
// That is the right direction for a guard about not committing things, and the
// skip list below names the directories where untracked local data is expected
// (`decks/` most of all: the one-copy rule permits scratch there, which is the
// whole distinction between present and tracked).
func TestEveryFileInThisCheckoutPassesTheTrackedFileScan(t *testing.T) {
	t.Parallel()
	scan := trackedFileScan(t)
	root := repoRoot(t)
	skip := map[string]bool{
		".git": true, "node_modules": true, ".venv": true,
		// App data and downloads a working checkout is allowed to hold.
		"decks": true, "data": true,
		// Worktrees of this same repository live here; each is its own tree.
		".claude": true,
	}
	seen := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			if rel != "." && skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		seen++
		if scan.MatchString(filepath.ToSlash(rel)) {
			t.Errorf("%s is in this checkout and the tracked-file scan refuses "+
				"it -- either it must not be committed (check `git ls-files`) or "+
				"the pattern is too wide", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// An empty walk would pass and prove nothing; the repository has over a
	// thousand tracked files, so a few hundred is a floor with slack in it.
	if seen < 300 {
		t.Fatalf("only %d files walked from %s, so this swept almost nothing",
			seen, root)
	}
}
