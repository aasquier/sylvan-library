package api

import (
	"errors"
	"net/http"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3/ledger"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// `GET /api/coliseum`: the six arenas a Tier 3 match is watched in.
//
// A Forge match takes minutes, and until now those minutes were a progress
// bar. This is what fills them: an arena drawn for the match, its champions,
// and a rotation of facts about the thing the whole surface is an homage to.
// It answers before any match is asked for, because the arena has to be on
// screen while the worker is still waking up.
//
// **Every card name resolves through the pool and is dropped and counted when
// it does not**, which is `/api/colors/{key}`'s rule and the reference prose
// rule generally: the text is checked in and finite, the card facts are the
// pool's, and a name the pool has never heard of is dropped rather than
// guessed at. With no pool at all the prose still answers whole — the facts
// are the point and they are all text; only the art goes missing.
func (a *API) coliseum(w http.ResponseWriter, r *http.Request) {
	// arena is one house, with its cards resolved.
	type arena struct {
		Key       string                   `json:"key"`
		Name      string                   `json:"name"`
		Plane     string                   `json:"plane"`
		Art       reference.ArenaArt       `json:"art"`
		Motion    string                   `json:"motion"`
		Palette   reference.Palette        `json:"palette"`
		Backdrop  *proseCard               `json:"backdrop"`
		Champions []champion               `json:"champions"`
		Facts     []reference.ColiseumFact `json:"facts"`
	}
	type answer struct {
		Arenas []arena `json:"arenas"`
		// Zones is the paintings the board's own three zones wear, plus the
		// mark a graveyard raises. Checked-in prose with a pinned printing, so
		// unlike everything else here it needs no pool at all — which is why
		// it is set before the pool is opened and survives not having one.
		Zones   []reference.ZoneArt `json:"zones"`
		Pool    bool                `json:"pool"`
		Dropped int                 `json:"dropped"`
	}

	source := reference.Arenas()
	out := answer{Arenas: make([]arena, 0, len(source)),
		Zones: reference.Zones()}
	for i := range source {
		src := &source[i]
		out.Arenas = append(out.Arenas, arena{
			Key: src.Key, Name: src.Name, Plane: src.Plane, Art: src.Art,
			Motion:  src.Motion,
			Palette: src.Palette, Champions: []champion{}, Facts: src.Facts})
	}

	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		// One query for every name in the file rather than one per arena: the
		// six arenas name a few dozen cards between them and the pool is a
		// round trip, not a lookup table.
		wanted := []string{}
		for i := range source {
			wanted = append(wanted, source[i].Card)
			for _, ch := range source[i].Champions {
				wanted = append(wanted, ch.Card)
			}
		}
		found, err := c.GetCards(r.Context(), wanted)
		if err != nil {
			return err
		}
		resolved := 0
		for i := range source {
			src := &source[i]
			if rec := found[src.Card]; rec != nil {
				card := asProseCard(rec)
				out.Arenas[i].Backdrop = &card
				resolved++
			}
			for _, ch := range src.Champions {
				rec := found[ch.Card]
				if rec == nil {
					continue
				}
				out.Arenas[i].Champions = append(out.Arenas[i].Champions,
					champion{Role: ch.Role, proseCard: asProseCard(rec)})
				resolved++
			}
		}
		out.Dropped = len(wanted) - resolved
		out.Pool = true
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		// The prose answers whole; only the paintings are missing. An arena
		// with no backdrop is a legible state the frontend renders as its
		// palette alone, which is why this is a 200 rather than a refusal.
		out.Pool = false
		out.Dropped = 0
		wire.JSON(w, http.StatusOK, out)
		return
	}
	if err != nil {
		a.fail(w, "coliseum", err)
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

// `GET /api/coliseum/standings`: what the ledger has learned from the matches
// it recorded (ADR 36, ADR 46).
//
// The room's memory. Every finished match has been written down since ADR 36,
// and until now nothing read any of it back — the ledger had exactly one
// reader, a CLI listing, and the accumulated games sat there answering nobody.
// This is the board built out of them: how each deck has fared, how each class
// has, and who has beaten whom.
//
// **Deterministic arithmetic on recorded rows** (ADR 14). No pool, no network,
// no opinion: a win rate is a gate rather than a judgement, and it is computed
// the same way twice. The interval beside it is arithmetic too — see
// `ledger.Board` for why a board sorted on a rate is a board that lies about
// small samples, which is the one real judgement anywhere in this path.
//
// **Scoped to the viewer**, and absent rather than forbidden (ADR 5): a match
// the caller was not in and the house did not host is not on the board at all.
// The rule is `ledger.Scope`'s to define; this handler only says who is asking.
func (a *API) coliseumStandings(w http.ResponseWriter, r *http.Request) {
	scope := auth.ScopeFrom(r.Context())
	board, err := a.matchLedger().Board(r.Context(), ledger.Scope{
		Viewer: scope.UserID,
		// An unauthenticated caller cannot reach a non-public route while
		// the door is locked -- the middleware refuses before routing --
		// so reaching here unauthenticated *is* the open deployment: one
		// person, and nobody to keep anything from.
		Open: !scope.Authenticated,
	})
	if err != nil {
		a.fail(w, "coliseum standings", err)
		return
	}
	wire.JSON(w, http.StatusOK, board)
}
