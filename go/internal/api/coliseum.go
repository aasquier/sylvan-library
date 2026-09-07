package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

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
	// The board and the paintings its feats are owed, in one answer.
	//
	// Embedded rather than nested so the record's own shape is untouched: every
	// field `ledger.Standings` carries arrives exactly where it always did, and
	// `art` is one more key beside them.
	wire.JSON(w, http.StatusOK, struct {
		*ledger.Standings
		Art map[string]cardArt `json:"art"`
	}{board, a.featArt(r.Context(), board)})
}

// cardArt is one feat card's painting, and the credit that has to travel with
// it (commandment 19, and [pool.Art] for why they are never separated).
type cardArt struct {
	// Image is the whole card face, and there is no crop beside it on purpose —
	// see [pool.Art], where the reason is a printing's painter and a printing's
	// picture having to come out of the same row.
	//
	// It is also the safer half of Scryfall's own clause: the terms ask that a
	// crop be shown with its credit *elsewhere in the same interface*, and let a
	// full card image stand on its own because the card carries the artist's
	// name and the copyright line in the printing. This board prints the credit
	// anyway.
	Image    string `json:"image"`
	Artist   string `json:"artist"`
	Set      string `json:"set"`
	Printing string `json:"printing"`
}

// featArt resolves the paintings the three feat boards need, keyed by the card
// name exactly as the record spells it.
//
// **The ledger stays pool-free and this is why the lookup lives here.** A win
// rate is a gate (ADR 14): `ledger.Board` is deterministic arithmetic over
// recorded rows, tested without a pool and answering the same way twice. A
// painting is presentation. Reaching into the pool from inside the ledger would
// put a picture in the middle of a computation nobody wants pictures in, so the
// record is computed first and decorated second.
//
// **Forge's spelling is the key, and a feat may name a token.** The deepest
// stack always does, and the biggest creature often does — a 15/15 is as likely
// to be a Beast token as a card somebody cast. Forge says "Beast Token" where
// Scryfall says "Beast", so both spellings are asked for and the answer is filed
// back under the one the record uses. Raw first: the strip is only consulted
// when the pool has never heard the name as printed, so a real card is never
// mistaken for a token on the strength of how it ends.
//
// **No pool is not an error.** The record is a record without paintings, and
// refusing to show somebody their own bouts because the card pool has not been
// refreshed would be the worse answer by a mile.
func (a *API) featArt(ctx context.Context, board *ledger.Standings) map[string]cardArt {
	out := map[string]cardArt{}
	if board == nil {
		return out
	}
	named := make([]string, 0,
		len(board.Blows)+len(board.Giants)+len(board.Stacks))
	for _, b := range board.Blows {
		named = append(named, b.Card)
	}
	for _, g := range board.Giants {
		named = append(named, g.Card)
	}
	for _, s := range board.Stacks {
		named = append(named, s.Card)
	}
	if len(named) == 0 {
		return out
	}

	wanted := make([]string, 0, len(named)*2)
	for _, name := range named {
		wanted = append(wanted, name)
		if stripped := pool.TokenName(name); stripped != name {
			wanted = append(wanted, stripped)
		}
	}
	_ = a.usePool(ctx, func(c *pool.Conn) error {
		found, err := c.ArtFor(ctx, wanted)
		if err != nil {
			return err
		}
		for _, name := range named {
			art, ok := found[strings.ToLower(name)]
			if !ok {
				art, ok = found[strings.ToLower(pool.TokenName(name))]
			}
			if !ok {
				continue
			}
			out[name] = cardArt{Image: art.Image, Artist: art.Artist,
				Set: art.Set, Printing: art.Printing}
		}
		return nil
	})
	return out
}
