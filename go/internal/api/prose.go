package api

import (
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The reference prose that needs no pool, no deck source and no network --
// the one corner of the app that works on a fresh clone before `data
// refresh` has ever run. Each answer is the
// embedded payload, byte for byte; the prose is checked-in JSON under
// `reference/data/` (see that package), so there is nothing to
// compute here and nothing to get wrong but the route.

// colors is `GET /api/colors`: the 32 combinations, the five colours and
// the three eras. Names only for the champions and signature cards; the
// cards come from `/api/colors/{key}`, which needs a pool.
func (a *API) colors(w http.ResponseWriter, _ *http.Request) {
	wire.Raw(w, http.StatusOK, reference.ColorsJSON())
}

// glossary is `GET /api/glossary`: the vocabulary.
func (a *API) glossary(w http.ResponseWriter, _ *http.Request) {
	wire.Raw(w, http.StatusOK, reference.GlossaryJSON())
}

// themes is `GET /api/themes`: the labelling vocabulary, so the deck page's
// editor can offer it, with the four class words in piloted order beside it
// (ADR 37). A route rather than a table copied into TypeScript, for the
// reason `SIMULATOR_KEYS` exists: a copy drifts silently.
func (a *API) themes(w http.ResponseWriter, _ *http.Request) {
	wire.Raw(w, http.StatusOK, reference.ThemesJSON())
}
