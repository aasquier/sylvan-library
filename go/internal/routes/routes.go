// Package routes reads `tests/contract/routes.json` -- the one route
// classification both the Python middleware and the Go front door are held
// to (docs/go-migration/PLAN.md section 8). This is the Go reader of it,
// `tests/contract/routes.py`'s twin: the same file, the same validation at
// load, so a malformed entry fails every test that imports this rather than
// silently shrinking a sweep.
//
// Only tests import it. The door's middleware does not read the table at
// request time -- its public list and admin prefix are code, exactly as
// `api/auth.py:PUBLIC_PATHS` is -- and a test in `internal/door` holds the
// code equal to the file, which is the arrangement `test_isolation.py`
// already keeps on the Python side.
package routes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Classification is every `/api` route, filed, plus what a sweep fills a
// placeholder with.
type Classification struct {
	AdminPrefix  string            `json:"admin_prefix"`
	Public       []string          `json:"public"`
	Shared       map[string]string `json:"shared"`
	UserScoped   map[string]string `json:"user_scoped"`
	Admin        map[string]string `json:"admin"`
	Placeholders map[string]string `json:"placeholders"`
}

// Protected is every route that must answer 401 without a session.
func (c *Classification) Protected() []string {
	var out []string
	for _, table := range []map[string]string{c.Shared, c.UserScoped, c.Admin} {
		for route := range table {
			out = append(out, route)
		}
	}
	sort.Strings(out)
	return out
}

// Every route the file knows, public included.
func (c *Classification) Every() []string {
	out := append(c.Protected(), c.Public...)
	sort.Strings(out)
	return out
}

// Concrete fills a templated path with the file's placeholders. The values
// need not resolve: every sweep asserts on a refusal that happens before any
// handler runs.
func (c *Classification) Concrete(path string) string {
	for placeholder, value := range c.Placeholders {
		path = strings.ReplaceAll(path, placeholder, value)
	}
	return path
}

// File is where the table lives, relative to the repository root.
const File = "tests/contract/routes.json"

// DefaultPath resolves File from this source file's position in the tree
// (`go/internal/routes/`), so a test in any package finds it without being
// told where the repository root is.
func DefaultPath() string {
	_, here, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(here), "..", "..", "..", File)
}

// Load reads and validates the file, naming what is wrong.
func Load(path string) (*Classification, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Classification
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if err := c.validate(filepath.Base(path)); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Classification) validate(name string) error {
	if c.AdminPrefix == "" || c.Placeholders == nil || c.Public == nil ||
		c.Shared == nil || c.UserScoped == nil || c.Admin == nil {
		return fmt.Errorf("%s: missing one of admin_prefix, public, shared, user_scoped, admin, placeholders", name)
	}
	seen := map[string]string{}
	file := func(bucket string, route string) error {
		if !strings.HasPrefix(route, "/api") {
			return fmt.Errorf("%s: %s is not an /api route", name, route)
		}
		if prior, dup := seen[route]; dup {
			return fmt.Errorf("%s: %s is filed as both %s and %s", name, route, prior, bucket)
		}
		seen[route] = bucket
		return nil
	}
	for _, route := range c.Public {
		if err := file("public", route); err != nil {
			return err
		}
	}
	for bucket, table := range map[string]map[string]string{
		"shared": c.Shared, "user_scoped": c.UserScoped, "admin": c.Admin} {
		for route, reason := range table {
			if len(reason) <= 15 {
				return fmt.Errorf("%s: %s is filed as %s without a reason", name, route, bucket)
			}
			if err := file(bucket, route); err != nil {
				return err
			}
		}
	}
	under := func(route string) bool {
		return route == c.AdminPrefix || strings.HasPrefix(route, c.AdminPrefix+"/")
	}
	for route := range c.Admin {
		if !under(route) {
			return fmt.Errorf("%s: filed as admin but not under %s: %s", name, c.AdminPrefix, route)
		}
	}
	for route, bucket := range seen {
		if bucket != "admin" && under(route) {
			return fmt.Errorf("%s: under %s but not filed as admin: %s", name, c.AdminPrefix, route)
		}
	}
	return nil
}
