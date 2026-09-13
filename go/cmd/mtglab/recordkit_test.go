package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The shared kit for this package's documents-versus-tree guards. Four legs
// of one night each wrote a guard that holds a prose record to the tree --
// the licensing record, the config record, the pipeline, the skills -- and
// every one reaches for the same moves: find the repository root from inside
// a test binary, read the repository paths a document names, read the verbs
// it tells a reader to run. The kit grew up inside `licenserecord_test.go`
// and lived there long after other guards depended on it; it sits in its own
// file because a session looking for "how do I assert a doc's path resolves"
// should find it by name, not by having already read the licensing test.

// repoRoot climbs until it finds the pair that can only be the repository
// root, so these tests survive the package moving.
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

// Two of the extractor's three deliberate conservatisms live below, because
// a guard that cries wolf gets deleted: only backticked tokens that *contain
// a slash* are treated as repository paths (a bare `LICENSE.txt` in a
// document is somebody else's file), and only those ending in an extension
// this repository actually writes. Both can miss a stale anchor; neither can
// invent one. The third -- files only, never directories -- is the
// trailing-slash skip in [repoPaths], and it was bought by CI on the first
// push, which is the right way round: a laptop passed and both architectures
// failed on `NOTICE.md`'s `data/` -- the very sentence that says the card
// pool is gitignored and so is *not* in the tree. A directory named in a
// record is as likely to be one the repository deliberately does not carry
// as one it does, and the anchors that rot are files anyway.

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

// mtglabVerb finds `mtglab <verb>` wherever a record tells a reader to run
// something. The expectation comes off [newRoot] rather than out of a list,
// which is the whole point: the day a subcommand is added, renamed or moved
// out to `tools/`, the guards reading this know without being edited.
var mtglabVerb = regexp.MustCompile(`\bmtglab\s+([a-z][a-z-]*)`)

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
