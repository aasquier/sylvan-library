package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// Every committed picture, video and font is accounted for where it stands.
// Commandment 9's provenance half, made a gate: the pipeline (ADR 29) exists
// so nobody has to trust a memory, and until now the check that nothing
// *bypassed* it was a hand sweep in the White polish run — which is how four
// PWA icons landed in `web/public` with no record anywhere and sat
// unaccounted for a week.
//
// The file set is `git ls-files`, not a walk: the claim is about what is
// *committed*, and a walk cannot tell a tracked stray from a toolbox venv's
// test fixtures (the first draft of this test flagged sympy's). `web_dist/`
// is exempt as build copies, byte-identical to their accounted sources.
//
// A media file is accounted for by exactly one of three records, each already
// load-bearing somewhere else:
//
//   - its directory's `PROVENANCE.md` names it (basename or stem — the video
//     entries name a stem once for the webm/mp4/still family);
//   - a recipe beside it names it as a `file:` output, whose bytes
//     `animist verify` holds (ADR 29);
//   - a recipe beside it declares an `each:` output in the file's own format
//     — the set convention, under which the recipe owns *every* file of that
//     format in the directory and `animist verify` pins the count, so a 79th
//     tarot card fails the toolbox gate and this test only has to know the
//     set exists.
//
// What this deliberately does not judge is whether the record is *true* —
// that is the White run's triple-check, per file, against primary sources.
// This holds the cheaper invariant that makes the triple-check finite: no
// committed media without a record to check.
func TestEveryCommittedPictureIsAccountedForWhereItStands(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	cmd := exec.Command("git", "-C", root, "ls-files", "-z")
	out, err := cmd.Output()
	if err != nil {
		// Failing loud rather than skipping: a gate that goes quiet when its
		// instrument is missing reads exactly like a gate that passed.
		t.Fatalf("git ls-files failed (%v) — this test needs the repository, "+
			"which is also the thing it checks", err)
	}
	found := 0
	var unaccounted []string
	for _, rel := range strings.Split(string(bytes.TrimRight(out, "\x00")), "\x00") {
		if rel == "" || strings.HasPrefix(rel, "web_dist/") {
			continue
		}
		if !mediaExtensions[strings.ToLower(filepath.Ext(rel))] {
			continue
		}
		found++
		if !accounted(t, filepath.Join(root, filepath.FromSlash(filepath.Dir(rel))), filepath.Base(rel)) {
			unaccounted = append(unaccounted, rel)
		}
	}
	for _, f := range unaccounted {
		t.Errorf("%s: no PROVENANCE.md entry and no recipe accounts for it — "+
			"write the record beside it before it ships", f)
	}
	// Anti-vacuity: the tarot deck alone is 78 files, frozen at 78 by its own
	// recipe. A listing with fewer has lost the repository, not the problem.
	if found < 78 {
		t.Fatalf("git ls-files under %s yielded only %d media files; it has "+
			"missed the tarot deck and is checking nothing", root, found)
	}
}

// mediaExtensions restates a list, which is normally the wrong move here and
// is the right one for the same reason `licenserecord_test.go` argues for its
// extension table: a missing entry misses a stray binary, it cannot invent
// one, and the sweep that would notice the miss (White's) keeps running.
var mediaExtensions = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".svg": true, ".avif": true, ".ico": true,
	".mp4": true, ".webm": true, ".mov": true,
	".mp3": true, ".ogg": true, ".wav": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true,
}

// accounted says whether one media file has a record standing beside it.
func accounted(t *testing.T, dir, name string) bool {
	t.Helper()
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if body, err := os.ReadFile(filepath.Join(dir, "PROVENANCE.md")); err == nil {
		if strings.Contains(string(body), name) || strings.Contains(string(body), stem) {
			return true
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".recipe.yaml") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var recipe struct {
			Outputs []struct {
				File   string `yaml:"file"`
				Each   string `yaml:"each"`
				Encode struct {
					Format string `yaml:"format"`
				} `yaml:"encode"`
			} `yaml:"outputs"`
		}
		if err := yaml.Unmarshal(body, &recipe); err != nil {
			t.Fatalf("%s: %v", filepath.Join(dir, e.Name()), err)
		}
		for _, out := range recipe.Outputs {
			if out.File == name {
				return true
			}
			if out.Each != "" && out.Encode.Format == format {
				return true
			}
		}
	}
	return false
}
