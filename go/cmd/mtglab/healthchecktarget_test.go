package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/api"
	"github.com/aasquier/sylvan-library/go/internal/door"
)

// **The alarm's own wiring, held to the tree.**
//
// Two files outside Go name the route the platform polls: `fly.toml`'s
// `[[http_service.checks]]` polls it from outside the container and the
// `Dockerfile`'s `HEALTHCHECK` polls it from inside. Both name it as a string,
// both assert in prose that it is on `PublicPaths`, and until this guard
// nothing held either claim to the code.
//
// The cost of the drift is the largest in the repository and it is silent in
// both directions. A renamed or re-methoded route leaves `fly.toml` polling a
// 404: the platform stops routing to the machine, and with one machine that is
// the whole site dark with every test green. The container's own check fails
// the same way and restarts the process in a loop. Dropping the path from
// `PublicPaths` does it too, because a check that gets the sign-in redirect
// instead of 200 is a check that fails — which is exactly why both files' own
// comments bother to say the path is public.
//
// Every fact here is read off the source of truth rather than typed: the path
// and the method come out of `fly.toml`, the second copy comes out of the
// `Dockerfile`, and the answer comes from [api.API.Routes] and
// [door.PublicPaths].
func TestThePlatformsHealthCheckNamesAServedPublicRoute(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)

	method, path := flyHealthCheck(t, filepath.Join(root, "fly.toml"))
	if method == "" || path == "" {
		t.Fatalf("fly.toml's health check names method %q path %q; one of them "+
			"is missing, and a check with no target is not a check", method, path)
	}

	// The image's own probe, which must be the same endpoint — both files say
	// so in prose, and a container checking one route while the platform checks
	// another is two different opinions about whether the app is alive.
	if inImage := dockerHealthCheckPath(t, filepath.Join(root, "Dockerfile")); inImage != path {
		t.Errorf("the image's HEALTHCHECK probes %q and fly.toml checks %q; "+
			"they are documented as the same endpoint", inImage, path)
	}

	// Served, with that method. `Routes` is the whole surface the door answers,
	// so a path absent from it is a 404 no matter what else is true.
	served := false
	for _, r := range api.New(api.Config{}).Routes() {
		if r.Pattern == path && r.Method == method {
			served = true
			break
		}
	}
	if !served {
		t.Errorf("nothing in the served route table answers %s %s, so the "+
			"platform's health check polls a 404 and stops routing to the "+
			"only machine there is", method, path)
	}

	// And public, or the check gets the door rather than the route.
	if !door.PublicPaths[path] {
		t.Errorf("%s is not on door.PublicPaths, so with auth on the platform's "+
			"health check is refused before routing", path)
	}

	// The method is the recorded one for a reason worth keeping beside this:
	// the route answers GET and nothing else, so any monitor configured with
	// HEAD's default would alert continuously against a healthy site. That is
	// the caveat on the queued external-monitor proposal, and it is why
	// `fly.toml` spells the method out instead of taking a default.
	if method != http.MethodGet {
		t.Errorf("fly.toml checks with %s; the health route answers GET, and a "+
			"HEAD or POST check fails against a perfectly well instance", method)
	}
}

// flyHealthCheck reads the first `[[http_service.checks]]` block's method and
// path. Scoped to the block deliberately: `fly.toml` has other `path` keys, and
// a file-wide regexp would happily match one of those.
func flyHealthCheck(t *testing.T, path string) (method, target string) {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // a repository file, from repoRoot
	if err != nil {
		t.Fatalf("reading fly.toml: %v", err)
	}
	value := regexp.MustCompile(`^\s*(method|path)\s*=\s*"([^"]*)"`)
	inBlock := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[[http_service.checks]]") {
			inBlock = true
			continue
		}
		// Any other table header ends the block.
		if inBlock && strings.HasPrefix(trimmed, "[") {
			break
		}
		if !inBlock {
			continue
		}
		if m := value.FindStringSubmatch(line); m != nil {
			if m[1] == "method" {
				method = m[2]
			} else {
				target = m[2]
			}
		}
	}
	if !inBlock {
		t.Fatal("fly.toml has no [[http_service.checks]] block at all, so the " +
			"platform is not checking this app's health from outside the container")
	}
	return method, target
}

// dockerHealthCheckPath reads the path out of the `HEALTHCHECK` instruction's
// probe URL — the URL rather than a literal, so a move to another port or host
// still resolves to the same comparison.
func dockerHealthCheckPath(t *testing.T, file string) string {
	t.Helper()
	raw, err := os.ReadFile(file) //nolint:gosec // a repository file, from repoRoot
	if err != nil {
		t.Fatalf("reading Dockerfile: %v", err)
	}
	probe := regexp.MustCompile(`HEALTHCHECK[\s\S]*?"(https?://[^"]+)"`)
	m := probe.FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("the Dockerfile has no HEALTHCHECK probing a URL, so nothing " +
			"inside the container notices the process stopped answering")
	}
	u, err := url.Parse(m[1])
	if err != nil {
		t.Fatalf("the HEALTHCHECK probes %q, which is not a URL: %v", m[1], err)
	}
	return u.Path
}
