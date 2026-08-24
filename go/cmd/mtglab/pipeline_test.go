package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
)

// The `deploy` job's `needs` list is the whole safety argument for continuous
// deployment (ADR 23), and it is the *only* guard there is for the manual
// button: branch protection governs merging and has nothing to say about a
// `workflow_dispatch` on `main`. So adding a job to `ci.yml` is really three
// steps — writing it, requiring it on `main`, and wiring it into `needs` — and
// the third is the one with no artifact anywhere except the line itself.
//
// This test existed once, in the Python suite, deriving the expected set from
// the file's own job list. It died with that suite, and the invariant drifted
// within one commit: the `tools` job arrived in "The interpreter departs" and
// nothing wired it in, so every deploy since has shipped without the toolbox
// gate — the committed art held to its recipes, and `mypy`/`ruff`/`pytest`
// over `tools/` — having ever had to be green. That is exactly the failure the
// old test was written to catch, happening in the window where it was absent.
//
// **It derives, never restates.** A list of job names typed in here would
// pass on the day somebody adds the eighth job and forgets it, which is the
// only day it matters. The expectation is `ci.yml`'s own `jobs:` keys.

// ciWorkflow is the shape this test needs of `.github/workflows/ci.yml` and
// nothing more. `needs` is modelled as a list because that is what the file
// writes; GitHub also accepts a bare string, so a future single-dependency job
// would fail to unmarshal here rather than pass vacuously — loud, and the
// right way round.
type ciWorkflow struct {
	Jobs map[string]struct {
		Needs []string `yaml:"needs"`
	} `yaml:"jobs"`
}

func readCIWorkflow(t *testing.T) ciWorkflow {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf ciWorkflow
	if err := yaml.Unmarshal(body, &wf); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	// Anti-vacuity. A parse that produced an empty map reads exactly like a
	// file with nothing wrong in it, and a renamed key is the likelier cause.
	if len(wf.Jobs) < 2 {
		t.Fatalf("%s parsed to %d jobs; the shape has probably changed", path, len(wf.Jobs))
	}
	if _, ok := wf.Jobs["deploy"]; !ok {
		t.Fatalf("%s has no `deploy` job; this test is checking the wrong file", path)
	}
	return wf
}

func TestTheDeployJobWaitsForEveryOtherJobInTheFile(t *testing.T) {
	t.Parallel()
	wf := readCIWorkflow(t)

	want := map[string]bool{}
	for name := range wf.Jobs {
		if name != "deploy" {
			want[name] = true
		}
	}
	got := map[string]bool{}
	for _, name := range wf.Jobs["deploy"].Needs {
		got[name] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("the `deploy` job does not wait for `%s`, so a red `%s` "+
				"still deploys. Add it to `needs` in .github/workflows/ci.yml "+
				"(deploy needs %v, the file has %v)",
				name, name, sortedKeys(got), sortedKeys(want))
		}
	}
	// The other direction: naming a job that does not exist is not a typo
	// GitHub forgives — the whole workflow fails to start, and it fails on
	// `main`, after the merge.
	for name := range got {
		if !want[name] {
			t.Errorf("the `deploy` job waits for `%s`, and no such job exists "+
				"in .github/workflows/ci.yml (it has %v)", name, sortedKeys(want))
		}
	}
}
