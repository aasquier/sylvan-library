package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

// The security layer must actually run, and only a derived test can say so.
//
// **Twice now, a Debian security release has silently stopped the site
// deploying.** The Dockerfiles run `apt-get upgrade`, so the fix is in the
// build; CI caches the build with `cache-from: type=gha`, and nothing above
// that line in a Dockerfile changes when Debian publishes something — so the
// layer is served warm, the upgrade never happens, and the image ships the
// old package. Trivy notices, the required `image` check goes red on `main`,
// and `deploy` never fires. CVE-2026-53615 was the first; CVE-2026-14456
// (openssl, fixed in `3.5.7-1~deb13u2`) was the second, and the remedy
// written down after the first was "bust the cache by hand", which is a
// remedy nobody performed because nobody was watching for it.
//
// `SECURITY_EPOCH` is the fix: CI puts today's date in the layer's cache key,
// so it rebuilds daily whether or not anybody is paying attention. This test
// is what keeps that wiring intact, and it is written the way
// `pipeline_test.go` argues for — **derived, never restated**. A list of build
// steps typed in here would pass on the day somebody adds a fourth one and
// forgets the argument, which is the only day it would matter.
//
// Two halves, because the wiring can break at either end and each end looks
// perfectly fine on its own:
//
//   - every step in `ci.yml` that builds an image passes the argument, and
//   - every Dockerfile that upgrades packages declares it, **in the stage
//     that does the upgrading and above the line that does it**. A bare `ARG`
//     is scoped to its build stage: one declared before the first `FROM` is a
//     global that a stage sees only if it re-declares it, and one written
//     below the `RUN` it is meant to key does nothing at all. Both mistakes
//     build cleanly and cache exactly as wrongly as no argument at all.
const securityEpochArg = "SECURITY_EPOCH"

// buildAction is the action that builds an image, matched on its owner and
// name so a version bump does not quietly turn this test off.
const buildAction = "docker/build-push-action@"

type ciSteps struct {
	Jobs map[string]struct {
		Steps []struct {
			Name string            `yaml:"name"`
			Uses string            `yaml:"uses"`
			With map[string]string `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func TestEveryImageBuildKeysTheSecurityLayerOnADate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wf ciSteps
	if err := yaml.Unmarshal(body, &wf); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	builds := 0
	for job, spec := range wf.Jobs {
		for _, step := range spec.Steps {
			if !strings.HasPrefix(step.Uses, buildAction) {
				continue
			}
			builds++
			args := step.With["build-args"]
			if !strings.Contains(args, securityEpochArg+"=") {
				t.Errorf("%s job %q, step %q builds an image without %s — "+
					"its `apt-get upgrade` layer will be served from cache "+
					"until something else in the Dockerfile changes, which "+
					"is how a deploy stops happening",
					path, job, step.Name, securityEpochArg)
				continue
			}
			// A constant would be worse than nothing: it reads as wired up
			// and caches exactly as hard as no argument at all. The value has
			// to come from somewhere that moves.
			if !strings.Contains(args, "${{") {
				t.Errorf("%s job %q, step %q passes a literal %q — the point "+
					"is a key that changes on its own", path, job,
					step.Name, args)
			}
		}
	}
	// Anti-vacuity: a renamed action or a changed `with` shape would make the
	// loop above find nothing and say nothing, which is the failure mode this
	// whole file exists to avoid.
	if builds < 3 {
		t.Fatalf("%s has %d image builds; the app image, its arm64 twin and "+
			"the Forge worker are three, so the shape has probably changed "+
			"and this test is no longer looking at anything", path, builds)
	}
}

// upgradeLine is a layer that takes Debian's security updates. `-y` is not
// optional in a Dockerfile, so requiring it keeps this off an unrelated
// mention of the word in prose.
var upgradeLine = regexp.MustCompile(`(?m)^\s*(RUN\s+|&&\s+)?apt-get upgrade -y`)

func TestEveryUpgradingDockerfileDeclaresTheEpochAboveIt(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	found, err := filepath.Glob(filepath.Join(root, "Dockerfile*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) < 2 {
		t.Fatalf("%d Dockerfiles at %s; the app's and the Forge worker's are "+
			"two", len(found), root)
	}
	upgrading := 0
	for _, path := range found {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		at := upgradeLine.FindStringIndex(string(body))
		if at == nil {
			continue
		}
		upgrading++
		// Everything above the upgrade, which is the only region where the
		// argument does anything.
		above := string(body)[:at[0]]
		// ...and only since the `FROM` that opened this stage, because an
		// `ARG` does not cross one.
		if cut := strings.LastIndex(above, "\nFROM "); cut >= 0 {
			above = above[cut:]
		}
		if !strings.Contains(above, "ARG "+securityEpochArg) {
			t.Errorf("%s upgrades packages without declaring `ARG %s` in the "+
				"same stage above it — CI passes the argument and this stage "+
				"cannot see it, so the layer caches forever and the upgrade "+
				"stops happening", filepath.Base(path), securityEpochArg)
		}
	}
	if upgrading == 0 {
		t.Fatal("no Dockerfile takes Debian's security updates any more; " +
			"either that is a real regression or this test is matching the " +
			"wrong line")
	}
}
