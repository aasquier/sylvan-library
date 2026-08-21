// Package api is `src/mtglab/api`, one family at a time: the served
// application's routes as they move across the door (docs/go-migration/
// PLAN.md section 4; the port board in section 10 says which have). The door
// asks this package for its routes and answers them itself, ahead of the
// proxy; anything not listed here still goes to the Python server behind it.
//
// Three rules, each the plan's:
//
//   - A route family moves whole. A job-shaped feature's submit and poll
//     flip together because the registry is per-process; a read family
//     flips when every route in it is here and the contract suite is green
//     through the door.
//   - A route here answers exactly what Python answers: the same status,
//     the same envelope, the same shape. `tests/contract/golden/` is the
//     record and `wire` is how the bytes get written.
//   - Nothing here is a new route. `tests/test_isolation.py` requires every
//     classified path to exist in FastAPI's table, so a Go-only `/api` path
//     would be a ghost to it; a route arrives here *from* Python, and the
//     door test `TestEveryPortedRouteIsInTheSharedTable` holds the list to
//     `tests/contract/routes.json`.
package api

import (
	"log/slog"
	"net/http"
)

// Config is what the ported routes need. It grows with the families: the
// pool arrives with the card reads, the deck library with the deck reads.
type Config struct {
	Logger *slog.Logger
}

// API holds the ported routes' dependencies.
type API struct {
	log *slog.Logger
}

// New builds the ported routes.
func New(cfg Config) *API {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &API{log: cfg.Logger}
}

// Route is one ported route: a method, a path template in the syntax
// `tests/contract/routes.json` uses (`/api/decks/{owner}/{slug}`), and the
// handler. Path values are read with `r.PathValue(name)`.
type Route struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}

// Routes is every route the Go side answers today. Order is not
// significant: the door matches on method and segments, and two routes that
// could both match one path would be a bug the door's table refuses at
// start.
func (a *API) Routes() []Route {
	return []Route{
		// The reference prose with no pool behind it (Phase 3, the first
		// family to move): fixed taxonomy, fixed vocabulary, fixed words.
		{Method: http.MethodGet, Pattern: "/api/colors", Handler: a.colors},
		{Method: http.MethodGet, Pattern: "/api/glossary", Handler: a.glossary},
		{Method: http.MethodGet, Pattern: "/api/themes", Handler: a.themes},
	}
}
