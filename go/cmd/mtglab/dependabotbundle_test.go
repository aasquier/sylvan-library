package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// Dependabot's bundle is rebuilt by the job that measures it.
//
// # The failure this closes
//
// `web_dist/` is committed and the `frontend` job refuses a PR whose bundle
// does not match its source. Dependabot moves `package.json` and
// `package-lock.json` and cannot run the build, so every bump that changed
// emitted bytes -- #427, #523, #556 -- went red on that gate with the whole
// suite green above it, read as a broken dependency, and was fixed by a
// person fetching the branch, building, and pushing the bundle. On
// 2026-10-07 Aaron asked for the routine to live in CI: "build the CI step
// that rebuilds the bundle for dependabot".
//
// # What a guard can hold about a workflow
//
// Not that it works -- that is proved by the next Dependabot PR going green
// on its own, and recorded there. What a guard can hold is the *shape* the
// step's safety argument depends on, each piece of which is one edit away
// from quietly ceasing to be true:
//
//   - the write grant is on the one job that needs it and is `contents`
//     only, and no other job in the file has grown one;
//   - that job's checkout keeps the workflow's credentials out of the tree,
//     because `npm ci` runs every dependency's install scripts and a job
//     that can write to a branch must not hand that right to code it just
//     downloaded;
//   - the step runs only for Dependabot, pushes only with its own token,
//     and refuses when that token is absent rather than pushing with
//     `GITHUB_TOKEN` -- a push that starts no checks and parks the PR on a
//     commit nothing tested;
//   - it runs before the gate, so the gate measures the bundle it committed;
//   - the runbook says how to mint the token, under the heading the step
//     names, because the day it lapses the step points there.
//
// Read off the YAML as data, the way `pipeline_test.go` reads `needs`, so a
// reordered or renamed step fails here rather than silently stopping.

type ciFrontendShape struct {
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Permissions map[string]string `yaml:"permissions"`
		Steps       []struct {
			Name string            `yaml:"name"`
			Uses string            `yaml:"uses"`
			If   string            `yaml:"if"`
			Run  string            `yaml:"run"`
			With map[string]any    `yaml:"with"`
			Env  map[string]string `yaml:"env"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

const (
	rebuildStep  = "Rebuild the bundle for Dependabot"
	gateStep     = "Committed bundle must match the source"
	bundleSecret = "DEPENDABOT_BUNDLE_TOKEN"
	runbookHead  = "### Dependabot and the bundle"
)

func readCIFrontendShape(t *testing.T) (ciFrontendShape, string) {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf ciFrontendShape
	if err := yaml.Unmarshal(body, &wf); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if _, ok := wf.Jobs["frontend"]; !ok {
		t.Fatalf("%s has no `frontend` job; this guard is reading the wrong file", path)
	}
	return wf, string(body)
}

func TestDependabotsBundleIsRebuiltByTheJobThatMeasuresIt(t *testing.T) {
	t.Parallel()
	wf, raw := readCIFrontendShape(t)

	// The grant: `contents: write` on `frontend`, nothing wider, nowhere else.
	if got := wf.Permissions["contents"]; got != "read" {
		t.Errorf("the workflow's own grant is `contents: %s`; it is `read`, and the one "+
			"job that writes raises its own", got)
	}
	for name, job := range wf.Jobs {
		for scope, level := range job.Permissions {
			switch {
			case name == "frontend" && scope == "contents" && level == "write":
			case level == "read" || level == "none":
			default:
				t.Errorf("job `%s` grants `%s: %s`. The only write in this file is "+
					"`frontend`'s `contents: write`, argued at the job; a second one "+
					"needs its own argument and its own guard.", name, scope, level)
			}
		}
	}
	front := wf.Jobs["frontend"]
	if front.Permissions["contents"] != "write" {
		t.Fatalf("`frontend` no longer grants `contents: write`; the rebuild step "+
			"cannot push without it (it has %v)", front.Permissions)
	}

	// The checkout: Dependabot's own branch, and no credentials left behind.
	var sawCheckout bool
	rebuild, gate := -1, -1
	for i, step := range front.Steps {
		if strings.HasPrefix(step.Uses, "actions/checkout@") {
			sawCheckout = true
			if v, ok := step.With["persist-credentials"]; !ok || v != false {
				t.Errorf("`frontend`'s checkout does not set `persist-credentials: false` "+
					"(got %v). `npm ci` runs every dependency's install scripts in a job "+
					"that can now write to a branch; the workflow token may not be in "+
					"the tree when it does.", v)
			}
			ref, _ := step.With["ref"].(string)
			if !strings.Contains(ref, "dependabot[bot]") || !strings.Contains(ref, "github.head_ref") {
				t.Errorf("`frontend`'s checkout `ref` is %q; on a Dependabot run it has to "+
					"be the head ref, or the bundle is built from the merge commit and "+
					"pushed onto a branch that never contained it", ref)
			}
		}
		switch step.Name {
		case rebuildStep:
			rebuild = i
		case gateStep:
			gate = i
		}
	}
	if !sawCheckout {
		t.Fatal("`frontend` has no actions/checkout step")
	}
	if rebuild < 0 {
		t.Fatalf("`frontend` has no step named %q; either the rebuild was removed "+
			"(delete this file with it) or it was renamed (rename it here)", rebuildStep)
	}
	if gate < 0 {
		t.Fatalf("`frontend` has no step named %q", gateStep)
	}
	if rebuild > gate {
		t.Errorf("%q runs after %q. The gate has to measure the bundle the rebuild "+
			"committed, so the rebuild comes first.", rebuildStep, gateStep)
	}

	// The step itself: Dependabot only, its own token, refuses without it.
	step := front.Steps[rebuild]
	if !strings.Contains(step.If, "github.actor == 'dependabot[bot]'") {
		t.Errorf("%q runs on `if: %s`; it is Dependabot's and must say so, or every "+
			"PR with a stale bundle gets a commit pushed onto it", rebuildStep, step.If)
	}
	want := fmt.Sprintf("${{ secrets.%s }}", bundleSecret)
	var tokenVar string
	for name, val := range step.Env {
		if val == want {
			tokenVar = name
		}
	}
	if tokenVar == "" {
		t.Fatalf("%q does not take `secrets.%s` into its env; the push has no token "+
			"of its own", rebuildStep, bundleSecret)
	}
	if n := strings.Count(raw, "secrets."+bundleSecret); n != 1 {
		t.Errorf("`secrets.%s` is read %d times in ci.yml; it is read once, in "+
			"%q's env, and nowhere a dependency's script could see it", bundleSecret, n, rebuildStep)
	}
	if !strings.Contains(step.Run, `-z "$`+tokenVar+`"`) || !strings.Contains(step.Run, "exit 1") {
		t.Errorf("%q does not refuse when `$%s` is empty. A push with the workflow's "+
			"own token starts no checks and parks the PR on a commit nothing "+
			"tested; the step has to stop and name the secret instead.", rebuildStep, tokenVar)
	}
	if !strings.Contains(step.Run, "x-access-token:${"+tokenVar+"}@github.com") {
		t.Errorf("%q does not push with `$%s`; a push by any other credential "+
			"either fails or starts no run", rebuildStep, tokenVar)
	}
	if !strings.Contains(step.Run, "git add -- web_dist") {
		t.Errorf("%q does not stage `web_dist` and only `web_dist`; a wider add "+
			"commits whatever else the build left behind", rebuildStep)
	}
	if !strings.Contains(step.Run, runbookHead[4:]) {
		t.Errorf("%q's refusal does not name the runbook section %q, which is "+
			"where the day the token lapses gets fixed", rebuildStep, runbookHead[4:])
	}

	// The runbook: the heading the step names, the secret, and the store.
	runbook, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "HOSTING.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(runbook)
	if !strings.Contains(doc, runbookHead) {
		t.Errorf("docs/HOSTING.md has no %q section; the step points there", runbookHead)
	}
	if !strings.Contains(doc, bundleSecret) || !strings.Contains(doc, "--app dependabot") {
		t.Errorf("docs/HOSTING.md does not say how to set `%s` as a *Dependabot* "+
			"secret (`gh secret set %s --app dependabot`); an Actions secret of the "+
			"same name is invisible to a run Dependabot starts, which is the one "+
			"way to set this up that looks right and does nothing", bundleSecret, bundleSecret)
	}
}
