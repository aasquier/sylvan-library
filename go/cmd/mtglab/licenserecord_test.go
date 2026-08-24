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
// Three deliberate conservatisms, because a guard that cries wolf gets
// deleted: only backticked tokens that *contain a slash* are treated as
// repository paths (a bare `LICENSE.txt` in this file is Forge's, not ours);
// only those ending in an extension this repository actually writes; and
// **files only, never directories**. All three can miss a stale anchor; none
// of them can invent one.
//
// The directory rule was bought by CI on the first push, which is the right
// way round: a laptop passed and both architectures failed on `NOTICE.md`'s
// `data/` — the very sentence that says the card pool is gitignored and so is
// *not* in the tree. A directory named in this record is as likely to be one
// the repository deliberately does not carry as one it does, and the anchors
// that rot are files anyway: all four this test was written for are files.

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

// repoRoot climbs until it finds the pair that can only be the repository
// root, so this test survives the package moving.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		_, notice := os.Stat(filepath.Join(dir, "NOTICE.md"))
		_, mod := os.Stat(filepath.Join(dir, "go", "go.mod"))
		if notice == nil && mod == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no repository root above the working directory")
		}
		dir = parent
	}
}

var backticked = regexp.MustCompile("`([^`\n]+)`")

// repoExtensions is the closing half of the conservatism above: a token is a
// path only if it ends in one of these. Restating a list is normally the wrong
// move in this repository, and it is the right one here because the failure
// mode of a wrong entry is a *missed* stale anchor rather than a false alarm.
var repoExtensions = map[string]bool{
	".go": true, ".py": true, ".ts": true, ".tsx": true, ".md": true,
	".yaml": true, ".yml": true, ".toml": true, ".json": true, ".css": true,
	".sql": true, ".txt": true, ".js": true, ".html": true, ".sh": true,
}

// repoPaths is the repository paths one document names.
func repoPaths(body string) []string {
	var out []string
	for _, m := range backticked.FindAllStringSubmatch(body, -1) {
		token := m[1]
		if !strings.Contains(token, "/") {
			continue // a bare filename in prose is somebody else's file
		}
		if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "http") {
			continue // a URL path or a URL: `/api/ocr/{name}`, not a file
		}
		// `tesseract.js@7.0.0/NOTICE` is a package spec; `{name}` is a route
		// template; the rest are prose, code fragments and ffmpeg filtergraphs.
		if strings.ContainsAny(token, "@{} <>*!()[]:#,\"'=$|\\%;") {
			continue
		}
		if strings.HasSuffix(token, "/") {
			continue // a directory: see the note above about `data/`
		}
		if repoExtensions[filepath.Ext(token)] {
			out = append(out, token)
		}
	}
	return out
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

// mtglabVerb finds `mtglab <verb>` wherever the record tells a reader to run
// something. The expectation comes off [newRoot] rather than out of a list
// here, which is the whole point: the day a subcommand is added, renamed or
// moved out to `tools/`, this test knows without being edited.
var mtglabVerb = regexp.MustCompile(`\bmtglab\s+([a-z][a-z-]*)`)

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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
