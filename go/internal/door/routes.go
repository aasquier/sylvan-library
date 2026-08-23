package door

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/api"
)

// The ported routes' table: what the door answers itself under /api, and
// the rule for when. It is deliberately not `http.ServeMux`, for three
// reasons a reader of net/http will expect to see argued. ServeMux cleans a
// path and answers a 301 to the cleaned one -- so `//api/colors` would be
// redirected where Python answers it (a JSON 404 through its router), and
// the door must never be *more* helpful than the runtime it stands in for.
// It answers 405 for a path whose pattern matches on another method -- but
// during the migration `GET /api/decks/{owner}/{slug}` is Go's while `PATCH`
// and `DELETE` on the same path are Python's, and the right answer to a
// method this side does not serve is the proxy, not a refusal. And it has
// opinions about trailing slashes and `{name...}` that nothing here wants.
//
// So the rule is the narrow one: a request is the door's only when its raw
// path is already canonical (`NormalisePath` leaves it alone and it carries
// no escape that changes its segments), the method matches, and the
// segments match a pattern literal for literal, with `{name}` capturing one
// segment into the request's path values. Everything else -- a doubled
// slash, a trailing slash, a dotted segment, a HEAD, a method this side has
// not ported -- goes to Python exactly as it arrived, which is what the door
// did with every /api request before any route moved. At retirement (Phase
// 8) the fall-through becomes the router's own 404 and 405, and that is the
// moment to write them, not now.
//
// Two refinements, both because a literal path and a templated one can
// share a shape -- `/api/colors/progress` beside `/api/colors/{key}`, which
// FastAPI resolves by declaring the literal first. Among the Go routes the
// most specific pattern wins (a literal segment beats a parameter), so the
// order routes are listed in never decides anything. And while a literal
// sibling still lives in Python, the API *reserves* it: `api.Proxied` names
// the exact paths a template must not capture, and the table hands those to
// the proxy before any pattern is consulted. The door test holds every
// reserved path to `routes.json` too, so a typo there is a failing test and
// not a route quietly answered by the wrong runtime.
type routeTable struct {
	routes   []compiledRoute
	reserved map[string]bool
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
func newRouteTable(routes []api.Route, reserved []string) (*routeTable, error) {
	t := &routeTable{reserved: map[string]bool{}}
	for _, p := range reserved {
		if p != NormalisePath(p) || !strings.HasPrefix(p, "/api/") {
			return nil, fmt.Errorf("reserved path %q is not a canonical /api path", p)
		}
		t.reserved[p] = true
	}
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
	if r.URL.RawPath != "" || raw != NormalisePath(raw) || t.reserved[raw] {
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

// Reserved is every path handed to the proxy ahead of any pattern.
func (t *routeTable) Reserved() []string {
	out := make([]string, 0, len(t.reserved))
	for p := range t.reserved {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
