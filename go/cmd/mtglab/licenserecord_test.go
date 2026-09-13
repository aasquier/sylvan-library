package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/claude"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
)

// The licensing record holds this project's right to exist, and every claim in
// it is an argument anchored to a thing somebody can go and read: the file that
// starts Forge as a separate process, the one that asks Scryfall for a bulk
// file, the recipe a committed picture came out of. **An anchor that resolves
// to nothing is not a small error.** It does not make the argument wrong, but
// it makes it uncheckable, which for a document whose whole value is that it
// can be re-verified is nearly the same thing.
//
// This is that check, and it exists because the Go crossing killed four
// anchors at once and nothing noticed: `NOTICE.md` still sent readers to
// `cards/db.py` and `sim/tier3/run.py` for the Scryfall and GPL arguments, the
// séance provenance sent them to `animist/sources.py` for the licence gate,
// and twenty-odd transformation lines told them to run `mtglab animist`, a
// command family that moved to `tools/` and left its name behind. All four
// were mechanically checkable and none was mechanically checked.
//
// The extraction the record is read with -- `repoPaths`, `mtglabVerb`, and
// the conservatisms they argue -- is the shared kit in `recordkit_test.go`,
// which every documents-versus-tree guard in this package reads through.

// licensingRecord is `NOTICE.md` plus every `PROVENANCE.md` in the tree --
// discovered by walking, never listed, so a new asset directory joins the
// check by existing rather than by somebody remembering.
func licensingRecord(t *testing.T) (root string, files []string) {
	t.Helper()
	root = repoRoot(t)
	skip := map[string]bool{".git": true, "node_modules": true, "web_dist": true, ".venv": true}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "PROVENANCE.md" || (d.Name() == "NOTICE.md" && filepath.Dir(path) == root) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	// Anti-vacuity: a walk that found nothing reads exactly like a walk that
	// found nothing wrong, and the root having moved is the likelier cause.
	found := false
	for _, f := range files {
		if f == "NOTICE.md" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the walk from %s found no NOTICE.md; it found %v", root, files)
	}
	return root, files
}

func TestTheLicensingRecordNamesOnlyPathsThatExist(t *testing.T) {
	t.Parallel()
	root, files := licensingRecord(t)
	checked := 0
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range repoPaths(string(body)) {
			checked++
			if _, err := os.Stat(filepath.Join(root, p)); err != nil {
				t.Errorf("%s names %s, which is not in the tree", rel, p)
			}
		}
	}
	// The extractor silently skipping everything would also pass.
	if checked < len(files) {
		t.Errorf("only %d paths found across %d documents; the extractor has "+
			"probably stopped matching", checked, len(files))
	}
}

func TestTheLicensingRecordNamesNoCommandTheBinaryLacks(t *testing.T) {
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
	root, files := licensingRecord(t)
	for _, rel := range files {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range mtglabVerb.FindAllStringSubmatch(string(body), -1) {
			if !have[m[1]] {
				t.Errorf("%s tells a reader to run `mtglab %s`, and this "+
					"binary has no such subcommand (it has %v)",
					rel, m[1], sortedKeys(have))
			}
		}
	}
}
