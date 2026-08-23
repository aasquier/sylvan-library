package door

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/api"
)

// The route table. It is deliberately not `http.ServeMux`, for two reasons
// a reader of net/http will expect to see argued. ServeMux cleans a path and
// answers a 301 to the cleaned one -- so `//api/colors` would be redirected
// where this router refuses it as JSON, and a router must never be more
// helpful than its own refusals. And it has opinions about trailing slashes
// and `{name...}` that nothing here wants.
//
// The rule: a request matches only when its (decoded) path is canonical
// (`NormalisePath` leaves it alone), the method matches, and the segments
// match a pattern literal for literal, with `{name}` capturing one segment
// into the request's path values. A path that matches on another method is
// the router's 405, `Allow` carrying the first matching route's method;
// anything else under /api is the catch-all's 404 -- both written by
// `dispatch`, which owns the refusal bodies.
//
// One refinement, because a literal path and a templated one can share a
// shape -- `/api/colors/progress` beside `/api/colors/{key}`. The most
// specific pattern wins (a literal segment beats a parameter), so the order
// routes are listed in never decides a match -- only a 405's `Allow`, which
// takes the first matching route in declaration order.
type routeTable struct {
	routes []compiledRoute
}

type compiledRoute struct {
	method   string
	pattern  string
	segments []string // "/api/decks/{owner}/{slug}" -> ["api","decks","{owner}","{slug}"]
	handler  http.Handler
}

// paramRe is a parameter segment: `{name}`, or `{name}.svg` -- FastAPI's
// `/api/symbols/{code}.svg`, where the parameter takes everything before a
// literal suffix.
var paramRe = regexp.MustCompile(`^\{([A-Za-z_][A-Za-z0-9_]*)\}([^{}/]*)$`)

// newRouteTable compiles the API's routes, refusing a table that could match
// one request two ways with nothing to choose between them: two patterns
// with the same method and the same shape, literal for literal and
// parameter for parameter. A literal against a parameter in the same slot
// is allowed and the literal wins.
func newRouteTable(routes []api.Route) (*routeTable, error) {
	t := &routeTable{}
	for _, r := range routes {
		if r.Handler == nil {
			return nil, fmt.Errorf("route %s %s has no handler", r.Method, r.Pattern)
		}
		if !strings.HasPrefix(r.Pattern, "/api/") {
			return nil, fmt.Errorf("route %s %s is not under /api", r.Method, r.Pattern)
		}
		if r.Pattern != NormalisePath(r.Pattern) {
			return nil, fmt.Errorf("route pattern %q is not canonical", r.Pattern)
		}
		segs := strings.Split(strings.TrimPrefix(r.Pattern, "/"), "/")
		for _, s := range segs {
			if s == "" {
				return nil, fmt.Errorf("route pattern %q has an empty segment", r.Pattern)
			}
			if strings.ContainsAny(s, "{}") && !paramRe.MatchString(s) {
				return nil, fmt.Errorf("route pattern %q has a malformed parameter %q", r.Pattern, s)
			}
		}
		c := compiledRoute{method: r.Method, pattern: r.Pattern, segments: segs, handler: r.Handler}
		for _, prior := range t.routes {
			if prior.method == c.method && sameShape(prior.segments, c.segments) {
				return nil, fmt.Errorf("routes %s and %s are the same shape for %s and nothing could choose",
					prior.pattern, c.pattern, c.method)
			}
		}
		t.routes = append(t.routes, c)
	}
	// Most specific first: more literal segments win, so a request that
	// matches both a literal and a template lands on the literal, whatever
	// order the API listed them in.
	sort.SliceStable(t.routes, func(i, j int) bool {
		return literals(t.routes[i].segments) > literals(t.routes[j].segments)
	})
	return t, nil
}

// sameShape reports whether two patterns are literal for literal and
// parameter for parameter -- the one pair nothing could choose between.
func sameShape(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if isParam(a[i]) != isParam(b[i]) {
			return false
		}
		if !isParam(a[i]) && a[i] != b[i] {
			return false
		}
	}
	return true
}

func literals(segments []string) int {
	n := 0
	for _, s := range segments {
		if !isParam(s) {
			n++
		}
	}
	return n
}

func isParam(seg string) bool {
	return paramRe.MatchString(seg)
}

// capture matches one request segment against a parameter segment: the
// name it binds and the value, or false -- a suffix must be there and the
// value before it must not be empty, as FastAPI's `[^/]+` requires.
func capture(seg, part string) (string, string, bool) {
	m := paramRe.FindStringSubmatch(seg)
	if m == nil {
		return "", "", false
	}
	name, suffix := m[1], m[2]
	if suffix == "" {
		return name, part, true
	}
	if !strings.HasSuffix(part, suffix) || len(part) == len(suffix) {
		return "", "", false
	}
	return name, strings.TrimSuffix(part, suffix), true
}

// match finds the handler for r, setting its path values — and names the
// pattern that matched, which is the route TEMPLATE the visitor ledger
// records (never the concrete path: a path can carry a slug and a slug can
// carry a person) — or reports that the request is not the door's to answer.
func (t *routeTable) match(r *http.Request) (http.Handler, string, bool) {
	raw := r.URL.Path
	if raw != NormalisePath(raw) {
		return nil, "", false
	}
	parts := strings.Split(strings.TrimPrefix(raw, "/"), "/")
	for _, c := range t.routes {
		if c.method != r.Method || len(c.segments) != len(parts) {
			continue
		}
		values := map[string]string{}
		matched := true
		for i, seg := range c.segments {
			if isParam(seg) {
				name, value, ok := capture(seg, parts[i])
				if !ok {
					matched = false
					break
				}
				values[name] = value
				continue
			}
			if seg != parts[i] {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		for name, value := range values {
			r.SetPathValue(name, value)
		}
		return c.handler, c.pattern, true
	}
	return nil, "", false
}

// Patterns is every ported route as `METHOD /path/{template}`, for tests
// that hold the table to `tests/contract/routes.json`.
func (t *routeTable) Patterns() []string {
	out := make([]string, 0, len(t.routes))
	for _, c := range t.routes {
		out = append(out, c.method+" "+c.pattern)
	}
	return out
}

// allowed reports whether the request's path matches any route on another
// method, and the `Allow` value for the 405 -- the first matching route's
// method, in declaration order, which is how Starlette's router answers it:
// the first partial match handles the refusal with its own methods.
func (t *routeTable) allowed(r *http.Request) (string, bool) {
	raw := r.URL.Path
	if raw != NormalisePath(raw) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(raw, "/"), "/")
	for _, c := range t.routes {
		if len(c.segments) != len(parts) {
			continue
		}
		matched := true
		for i, seg := range c.segments {
			if isParam(seg) {
				if _, _, ok := capture(seg, parts[i]); !ok {
					matched = false
					break
				}
				continue
			}
			if seg != parts[i] {
				matched = false
				break
			}
		}
		if matched {
			return c.method, true
		}
	}
	return "", false
}
