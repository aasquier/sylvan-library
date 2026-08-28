package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/reference"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// proseCard is the card shape `combination_detail` and `lore_shelves` render
// beside the prose: the card's own cost, type and text, in that key order.
type proseCard struct {
	Name          string   `json:"name"`
	ManaCost      *string  `json:"mana_cost"`
	TypeLine      string   `json:"type_line"`
	OracleText    string   `json:"oracle_text"`
	ColorIdentity []string `json:"color_identity"`
	Image         *string  `json:"image"`
	ArtCrop       *string  `json:"art_crop"`
}

func asProseCard(rec *pool.CardRecord) proseCard {
	return proseCard{Name: rec.Name, ManaCost: rec.ManaCost, TypeLine: rec.TypeLine,
		OracleText: rec.OracleText, ColorIdentity: rec.ColorIdentity,
		Image: rec.ImageNormal, ArtCrop: rec.ImageArtCrop}
}

type champion struct {
	Role string `json:"role"`
	proseCard
}

// combination is `GET /api/colors/{key}` -- `service.combination_detail`:
// one of the 32, with its champions and signature cards resolved through the
// pool and **dropped and counted** when a name does not resolve, plus the
// count of cards whose identity is exactly this combination. The
// canonicaliser collapses anything that is not WUBRG to "C", so an
// unknown key would be answered by Colourless; the spelling is checked
// afterwards, deliberately -- the recorded order of the two checks.
func (a *API) combination(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	upper := strings.ToUpper(key)
	letters := []string{}
	for _, ch := range upper {
		letters = append(letters, string(ch))
	}
	combo, found := reference.CombinationByKey(reference.KeyFor(letters))
	if found {
		want := map[string]bool{}
		for _, ch := range upper {
			if ch != 'C' {
				want[string(ch)] = true
			}
		}
		have := map[string]bool{}
		for _, c := range combo.Colors {
			have[c] = true
		}
		if len(want) != len(have) {
			found = false
		}
		for c := range want {
			if !have[c] {
				found = false
			}
		}
	}
	if !found {
		wire.Detail(w, http.StatusNotFound, "no colour combination "+wire.Quote(key))
		return
	}
	// Everything the taxonomy holds about one combination except the two
	// lists this route replaces with resolved cards -- so a reader that has
	// the detail never has to go back to `/api/colors` for a field. `Creed`
	// is a name and a citation rather than a card to draw, so it passes
	// through untouched by the pool lookup below.
	type base struct {
		Key        string           `json:"key"`
		Name       string           `json:"name"`
		Tier       string           `json:"tier"`
		Colors     []string         `json:"colors"`
		Size       int              `json:"size"`
		Tagline    string           `json:"tagline"`
		History    string           `json:"history"`
		Lore       string           `json:"lore"`
		Aliases    []string         `json:"aliases"`
		VerifiedBy string           `json:"verified_by"`
		Creed      *reference.Creed `json:"creed"`
	}
	type answer struct {
		base
		Pool       bool        `json:"pool"`
		Champions  []champion  `json:"champions"`
		Signature  []proseCard `json:"signature"`
		Dropped    int         `json:"dropped"`
		ExactTotal *int        `json:"exact_total"`
	}
	out := answer{base: base{Key: combo.Key, Name: combo.Name, Tier: combo.Tier, Colors: combo.Colors,
		Size: combo.Size, Tagline: combo.Tagline, History: combo.History, Lore: combo.Lore,
		Aliases: combo.Aliases, VerifiedBy: combo.VerifiedBy, Creed: combo.Creed},
		Champions: []champion{}, Signature: []proseCard{}}
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		wanted := []string{}
		for _, ch := range combo.Champions {
			wanted = append(wanted, ch.Card)
		}
		wanted = append(wanted, combo.Signature...)
		found, err := c.GetCards(r.Context(), wanted)
		if err != nil {
			return err
		}
		for _, ch := range combo.Champions {
			if rec := found[ch.Card]; rec != nil {
				out.Champions = append(out.Champions, champion{Role: ch.Role, proseCard: asProseCard(rec)})
			}
		}
		for _, name := range combo.Signature {
			if rec := found[name]; rec != nil {
				out.Signature = append(out.Signature, asProseCard(rec))
			}
		}
		out.Dropped = len(wanted) - len(out.Champions) - len(out.Signature)
		// Interpolated rather than parameterised because both values are the
		// table's own and neither is caller input.
		var total int
		if err := c.DB().QueryRowContext(r.Context(), fmt.Sprintf(
			"SELECT count(*) FROM oracle_cards WHERE "+
				"json_extract_string(legalities, 'commander') = 'legal' AND "+
				"len(list_filter(color_identity, x -> x NOT IN (%s))) = 0 "+
				"AND len(color_identity) = %d", deckread.QuotedList(combo.Colors), combo.Size)).Scan(&total); err != nil {
			return err
		}
		out.Pool = true
		out.ExactTotal = &total
		return nil
	})
	if errors.Is(err, pool.ErrNoPool) {
		out.Pool = false
		out.Champions = []champion{}
		out.Signature = []proseCard{}
		out.Dropped = 0
		out.ExactTotal = nil
		wire.JSON(w, http.StatusOK, out)
		return
	}
	if err != nil {
		a.fail(w, "colors/{key}", err)
		return
	}
	wire.JSON(w, http.StatusOK, out)
}

// lore is `GET /api/lore`: the fact volumes, with
// every named card resolved through the pool and dropped and counted when it
// does not resolve; with no pool at all the prose answers whole, every fact
// written to be complete without its cards -- the reference's writing rule.
func (a *API) lore(w http.ResponseWriter, r *http.Request) {
	type fact struct {
		Key    string           `json:"key"`
		Volume string           `json:"volume"`
		Fact   string           `json:"fact"`
		More   string           `json:"more"`
		Cards  []proseCard      `json:"cards"`
		Learn  *reference.Learn `json:"learn"`
	}
	type answer struct {
		Volumes []reference.Volume `json:"volumes"`
		Facts   []fact             `json:"facts"`
		Pool    bool               `json:"pool"`
		Dropped int                `json:"dropped"`
	}
	shelves := reference.Lore()
	wantedSet := map[string]bool{}
	for _, f := range shelves.Facts {
		for _, name := range f.Cards {
			wantedSet[name] = true
		}
	}
	wanted := make([]string, 0, len(wantedSet))
	for n := range wantedSet {
		wanted = append(wanted, n)
	}
	sort.Strings(wanted)
	found := map[string]*pool.CardRecord{}
	havePool := true
	err := a.usePool(r.Context(), func(c *pool.Conn) error {
		var err error
		found, err = c.GetCards(r.Context(), wanted)
		return err
	})
	if errors.Is(err, pool.ErrNoPool) {
		havePool = false
		err = nil
	}
	if err != nil {
		a.fail(w, "lore", err)
		return
	}
	out := answer{Volumes: shelves.Volumes, Facts: []fact{}, Pool: havePool}
	for _, f := range shelves.Facts {
		resolved := []proseCard{}
		for _, name := range f.Cards {
			rec := found[name]
			if rec == nil {
				if havePool {
					out.Dropped++
				}
				continue
			}
			resolved = append(resolved, asProseCard(rec))
		}
		out.Facts = append(out.Facts, fact{Key: f.Key, Volume: f.Volume, Fact: f.Fact, More: f.More,
			Cards: resolved, Learn: f.Learn})
	}
	if out.Volumes == nil {
		out.Volumes = []reference.Volume{}
	}
	wire.JSON(w, http.StatusOK, out)
}
