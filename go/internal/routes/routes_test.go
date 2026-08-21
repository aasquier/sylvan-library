package routes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheSharedTableLoadsAndValidates(t *testing.T) {
	c, err := Load(DefaultPath())
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminPrefix != "/api/admin" {
		t.Fatalf("admin prefix %q", c.AdminPrefix)
	}
	if len(c.Public) < 7 || len(c.Protected()) < 40 {
		t.Fatalf("%d public, %d protected -- the table has shrunk", len(c.Public), len(c.Protected()))
	}
	for _, route := range c.Every() {
		concrete := c.Concrete(route)
		if strings.ContainsAny(concrete, "{}") {
			t.Fatalf("%s has a placeholder the file does not fill: %s", route, concrete)
		}
	}
}

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "routes.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const good = `{"admin_prefix": "/api/admin", "public": ["/api/health"],
 "shared": {"/api/decks": "every caller may list; nothing private"},
 "user_scoped": {"/api/jobs": "the caller's own jobs, ADR 5"},
 "admin": {"/api/admin/users": "every account on the instance"},
 "placeholders": {"{slug}": "x"}}`

func TestValidationNamesWhatIsWrong(t *testing.T) {
	cases := map[string]string{
		"twice":    strings.Replace(good, `"shared": {"/api/decks"`, `"shared": {"/api/health"`, 1),
		"reason":   strings.Replace(good, `"the caller's own jobs, ADR 5"`, `"short"`, 1),
		"not api":  strings.Replace(good, `"/api/decks"`, `"/decks"`, 1),
		"strayed":  strings.Replace(good, `"/api/decks"`, `"/api/admin/decks"`, 1),
		"misfiled": strings.Replace(good, `"/api/admin/users"`, `"/api/users"`, 1),
		"missing":  strings.Replace(good, `"placeholders": {"{slug}": "x"}`, `"extra": {}`, 1),
	}
	for name, body := range cases {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("%s: a broken table loaded", name)
		}
	}
	if _, err := Load(write(t, good)); err != nil {
		t.Fatalf("the good table was refused: %v", err)
	}
}
