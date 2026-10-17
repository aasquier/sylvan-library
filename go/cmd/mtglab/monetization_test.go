package main

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// **"This project never charges a solitary penny, for anything"** is
// commandment 9, and the White checklist asks every run to confirm that no
// monetization surface exists, "even vestigially: no payment code, no
// donation links, no ad slots" -- adding that a well-meaning *buy me a
// coffee* in the frontend is a violation here, not a kindness. Until now that
// was a grep somebody remembered to run. The Fan Content Policy is the
// standing this site exists on, and the one clause of it a tired session is
// likeliest to breach is this one, because a donation link looks like
// gratitude rather than commerce. So the grep is a test.
//
// **What is swept: the project's own source.** `web/src`, `web/public` and
// `web/index.html`; every non-test Go file; the toolbox's Python; `scribe/`;
// the Dockerfiles and `fly.toml`. Not `web_dist/`, and the reason is a
// measurement rather than a shortcut: the built reader bundle carries the OCR
// library's own `package.json` as data, `funding` field and all, so a sweep of
// the bundle fires on `opencollective.com/tesseractjs` -- a library thanking
// its backers in a string nobody renders. What the bundle *reaches for* at
// runtime is `hotlinkrecord_test.go`'s business; what this project *writes*
// is this file's. Test files and `testdata/` are skipped because this file
// has to hold the vocabulary somewhere, and `docs/` and `.claude/` are skipped
// because the rule is allowed to name what it forbids.
//
// **What a penny looks like: a hostname.** Words fail here -- the keeper's
// copy says "donated material" of a necromancer, and `sponsor` is a verb --
// so the vocabulary is the hosts a payment, donation or ad actually goes
// through, which nothing in this repository has a reason to spell. The one
// shop link that *is* allowed, TCGplayer's mass entry (price and cart, never
// checkout -- CLAUDE.md's "Out of scope"), carries no affiliate parameter
// and is not in the list; if it ever grows one, the `?partner=` entry below
// catches the parameter rather than the host.

// pennyMarks is the vocabulary: lower-case, matched case-insensitively against
// every line of the swept files.
var pennyMarks = []string{
	// Donations and tips.
	"ko-fi.com", "patreon.com", "buymeacoffee.com", "gofundme.com",
	"opencollective.com", "github.com/sponsors", "liberapay.com",
	// Payments.
	"paypal.com", "paypal.me", "stripe.com", "venmo.com", "cash.app",
	"coinbase.com", "bitcoin:",
	// Advertising.
	"adsbygoogle", "googlesyndication", "doubleclick.net", "amazon-adsystem",
	"data-ad-slot",
	// An affiliate tag on an otherwise-allowed shop link -- the URL-parameter
	// shape only, because *partner* is a Magic word this tree spells often.
	"?partner=", "&partner=", "?affiliate=", "&affiliate=", "utm_campaign=affiliate",
}

// pennyMarksIn is the reader: every mark a line carries, in vocabulary order.
func pennyMarksIn(line string) []string {
	lower := strings.ToLower(line)
	var found []string
	for _, mark := range pennyMarks {
		if strings.Contains(lower, mark) {
			found = append(found, mark)
		}
	}
	return found
}

// ownSourceDirs are the trees swept, relative to the repository root, and
// ownSourceFiles the root-level files that are not under any of them.
var (
	ownSourceDirs  = []string{"web/src", "web/public", "go", "tools", "scribe"}
	ownSourceFiles = []string{"web/index.html", "fly.toml", "Dockerfile", "Dockerfile.forge"}
	// Directories a sweep never enters: generated, vendored, or the tests
	// and fixtures that may legitimately spell the vocabulary.
	skippedDirs = map[string]bool{
		"node_modules": true, ".venv": true, "testdata": true, "tests": true,
		"target": true, "build": true, ".git": true,
	}
	// Extensions the project writes source in. A binary asset cannot carry a
	// link anybody can click, and reading one as lines is noise.
	sweptExtensions = map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".css": true,
		".html": true, ".py": true, ".java": true, ".toml": true, ".yaml": true,
		".yml": true, ".json": true, ".sh": true, ".md": true,
	}
)

// pennyHits is every `path:line: mark` in the files this guard sweeps.
func pennyHits(t *testing.T, root string) []string {
	t.Helper()
	var hits []string
	sweep := func(path string) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		rel, _ := filepath.Rel(root, path)
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1<<20), 1<<24)
		for n := 1; scanner.Scan(); n++ {
			for _, mark := range pennyMarksIn(scanner.Text()) {
				hits = append(hits, rel+":"+strconv.Itoa(n)+": "+mark)
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range ownSourceDirs {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skippedDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if strings.HasSuffix(name, "_test.go") || !sweptExtensions[filepath.Ext(name)] {
				return nil
			}
			sweep(path)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range ownSourceFiles {
		if _, err := os.Stat(filepath.Join(root, file)); err != nil {
			t.Fatalf("%s is named as swept source and is not in the tree: %v", file, err)
		}
		sweep(filepath.Join(root, file))
	}
	sort.Strings(hits)
	return hits
}

// TestNothingHereTakesAPenny is commandment 9's second sentence as a gate:
// the project's own source names no payment, donation or advertising host.
func TestNothingHereTakesAPenny(t *testing.T) {
	t.Parallel()
	if hits := pennyHits(t, repoRoot(t)); len(hits) > 0 {
		t.Errorf("the tree names a way to take money, and this project never "+
			"charges a solitary penny (commandment 9):\n  - %s\n\n"+
			"If it is a library's own funding note, it belongs in the built "+
			"bundle, which this guard does not read; if it is ours, it goes.",
			strings.Join(hits, "\n  - "))
	}
}

// TestThePennyReaderStillSeesAPenny keeps the guard above from rotting into a
// vacuous pass: a sweep that found nothing because its reader is broken reads
// exactly like a clean tree.
func TestThePennyReaderStillSeesAPenny(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		`<a href="https://Ko-Fi.com/sylvan">buy me a coffee</a>`:                   {"ko-fi.com"},
		`<script async src="https://pagead2.googlesyndication.com/x.js"></script>`: {"googlesyndication"},
		`https://www.tcgplayer.com/massentry?partner=sylvan`:                       {"?partner="},
		`https://www.tcgplayer.com/massentry`:                                      nil,
		`creatures assembled from donated material`:                                nil,
	}
	for line, want := range cases {
		got := pennyMarksIn(line)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("pennyMarksIn(%q) = %v, want %v", line, got, want)
		}
	}
}
