// Package wheel is `decks/wheel.py`: the Wheel of Fortune's spin.
//
// Daniel Gelon's Alpha painting — a red-cloaked figure spins a plank wheel
// nailed to a great tree, and the wheel's face carries four fates: a cup, a
// heart, a sword, a skull. Deterministic code picks the fate and then a card
// in the deck's colours that answers to it, seeded so the same spin can be
// dealt again — Claude is not consulted, because a wheel has never needed an
// opinion (ADR 14). Nothing here writes a deck and no rationale is prefilled
// (rule 4); a card the deck already runs is excluded before the draw, and so
// is anything banned or outside the commander's identity.
//
// The randomness is `internal/mt19937`, because a seed is a promise: a spin
// somebody replayed on the Python side deals the same fate, the same face
// and the same card here — `randrange` over the symbols, then over a fate's
// faces, then over the candidate count, all off one generator. Held to
// Python by the generated `testdata/spins.json`.
package wheel

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/mt19937"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// Symbols are the four fates, in the order they sit on the painted wheel
// read clockwise from the top. The client draws its own wheel to the same
// order, so the index is the contract.
var Symbols = [4]string{"cup", "heart", "sword", "skull"}

// fate is a fate's screen prose and its pool filter, side by side so the
// two cannot drift apart separately.
type fate struct {
	label   string
	meaning string
	where   string
}

var fates = map[string]fate{
	"cup": {
		label:   "The Cup",
		meaning: "The cup runneth over — a card that refills your hand.",
		where: "(oracle_text ILIKE '%draw a card%'" +
			" OR oracle_text ILIKE '%draws a card%'" +
			" OR oracle_text ILIKE '%draw two cards%'" +
			" OR oracle_text ILIKE '%draw three cards%'" +
			" OR oracle_text ILIKE '%draw cards%')",
	},
	"heart": {
		label:   "The Heart",
		meaning: "A beating heart — a card that gives life back.",
		where:   "(oracle_text ILIKE '%gain%life%')",
	},
	"sword": {
		label:   "The Sword",
		meaning: "The sword falls — a card that removes or damages.",
		where: "(oracle_text ILIKE '%destroy target%'" +
			" OR oracle_text ILIKE '%deals%damage%'" +
			" OR oracle_text ILIKE '%exile target%')",
	},
	"skull": {
		label: "The Skull",
		meaning: "The skull grins — a card that fills the graveyard, " +
			"or robs it.",
		where: "(oracle_text ILIKE '%graveyard%'" +
			" OR oracle_text ILIKE '%sacrifice%'" +
			" OR oracle_text ILIKE '%dies%')",
	},
}

// face is one of a fate's two landings; its meaning and filter REPLACE the
// fate's base ones when it lands.
type face struct {
	key     string
	meaning string
	where   string
}

// faces maps a fate to its wire field and its landings, in the order
// Python's dict declares them — heads before tails, whole before broken —
// because the second `randrange` indexes that order exactly as `Symbols`'
// index is the first one's contract.
var faces = map[string]struct {
	field    string
	landings []face
}{
	"cup": {"coin", []face{
		{"heads",
			"The coin lands heads — the cup fills your hand: " +
				"a card that draws.",
			"(oracle_text ILIKE '%draw a card%'" +
				" OR oracle_text ILIKE '%draws a card%'" +
				" OR oracle_text ILIKE '%draw two cards%'" +
				" OR oracle_text ILIKE '%draw three cards%'" +
				" OR oracle_text ILIKE '%draw cards%')"},
		{"tails",
			"The coin lands tails — the cup spills into the " +
				"water: a card that mills.",
			"(oracle_text ILIKE '%mill%'" +
				" OR oracle_text ILIKE " +
				"'%library%into%graveyard%')"},
	}},
	"heart": {"heart_face", []face{
		{"whole",
			"The heart beats whole — a card that gives " +
				"life back.",
			"(oracle_text ILIKE '%gain%life%')"},
		{"broken",
			"The heart comes up broken — a card that drains " +
				"life away.",
			"(oracle_text ILIKE '%loses%life%'" +
				" OR oracle_text ILIKE '%lose%life%')"},
	}},
	"sword": {"sword_face", []face{
		{"edge",
			"The sword falls edge-first — a card that " +
				"removes or damages.",
			"(oracle_text ILIKE '%destroy target%'" +
				" OR oracle_text ILIKE '%deals%damage%'" +
				" OR oracle_text ILIKE '%exile target%')"},
		{"hilt",
			"The sword is offered hilt-first — a weapon to " +
				"take up: an equipment.",
			"(type_line ILIKE '%Equipment%')"},
	}},
	"skull": {"skull_face", []face{
		{"buried",
			"The skull grins — the grave takes: a card that " +
				"fills the graveyard.",
			"(oracle_text ILIKE '%mill%'" +
				" OR oracle_text ILIKE '%sacrifice%'" +
				" OR oracle_text ILIKE '%discard%')"},
		{"risen",
			"The skull whispers — the grave gives back: a " +
				"card that raises what was lost.",
			"(oracle_text ILIKE '%return%from%graveyard%'" +
				" OR oracle_text ILIKE '%from%graveyard%to the%')"},
	}},
}

// Caveat states which system answered (ADR 14 boundary 3) without naming one
// (commandment 10): what the user needs is the distinction — blind dice,
// nobody's judgment. The wire still carries `answered_by: "python"` and
// `seed` as tokens for clients and tests; rendering them is the sin, not
// sending them. (`answered_by` keeps Python's spelling across the port for
// exactly that reason: it is a wire token, not a rendered fact, and clients
// already key on it.)
const Caveat = "The wheel is blind dice over the card pool — a fate, then a " +
	"random legal card in this deck's colours that answers to it. " +
	"A suggestion to argue with, never a recommendation: the " +
	"rationale, if it earns one, is yours to write."

// Spin is one turn of the wheel: a fate, and a card that answers to it.
//
// Seeded exactly like Tier 1: a nil seed invents its own and reports it, so
// any spin can be spun again. The candidate is drawn by counted offset over
// a name-ordered query rather than `ORDER BY random()`, because DuckDB's
// randomness cannot be seeded from here and a spin that cannot be replayed
// is a number with no provenance.
func Spin(ctx context.Context, d *deck.Deck, identity map[string]bool,
	c *pool.Conn, seed *big.Int) (wire.OrderedMap, error) {
	if seed == nil {
		fresh, err := crand.Int(crand.Reader, new(big.Int).Lsh(big.NewInt(1), 32))
		if err != nil {
			return nil, fmt.Errorf("wheel: %w", err)
		}
		seed = fresh
	}
	rng := mt19937.NewFromBig(seed)
	symbol := Symbols[rng.RandRange(int64(len(Symbols)))]
	chosen := fates[symbol]

	// The second landing, for the fates that have one: same rng, so the
	// seed replays coin and card alike.
	meaning, fateWhere := chosen.meaning, chosen.where
	faceField, faceKey := "", ""
	if f, ok := faces[symbol]; ok {
		landed := f.landings[rng.RandRange(int64(len(f.landings)))]
		faceField, faceKey = f.field, landed.key
		meaning, fateWhere = landed.meaning, landed.where
	}

	inDeck := deckNames(d)
	colors := identityColors(identity)
	fits := "len(color_identity) = 0"
	if len(colors) > 0 {
		quoted := make([]string, len(colors))
		for i, col := range colors {
			quoted[i] = "'" + col + "'"
		}
		fits = "len(list_filter(color_identity, x -> x NOT IN (" +
			strings.Join(quoted, ", ") + "))) = 0"
	}
	notInDeck := "1=1"
	if len(inDeck) > 0 {
		notInDeck = "name NOT IN (" +
			strings.TrimSuffix(strings.Repeat("?, ", len(inDeck)), ", ") + ")"
	}
	where := strings.Join([]string{
		"json_extract_string(legalities, 'commander') = 'legal'",
		fits,
		fateWhere,
		notInDeck,
	}, " AND ")

	args := make([]any, len(inDeck))
	for i, name := range inDeck {
		args[i] = name
	}
	var total int64
	err := c.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM oracle_cards WHERE "+where, args...).Scan(&total)
	if err != nil {
		return nil, fmt.Errorf("wheel: %w", err)
	}

	base := wire.OrderedMap{
		{Key: "symbol", Value: symbol},
		{Key: "label", Value: chosen.label},
		{Key: "meaning", Value: meaning},
		{Key: "seed", Value: seed},
		{Key: "answered_by", Value: "python"},
		{Key: "caveat", Value: Caveat},
	}
	if faceField != "" {
		base = append(base, wire.KV{Key: faceField, Value: faceKey})
	}
	if total == 0 {
		return append(base,
			wire.KV{Key: "card", Value: nil},
			wire.KV{Key: "reason", Value: "The pool holds no legal card in " +
				"these colours that answers to this fate. The wheel has " +
				"spoken; it just had nothing to hand you."}), nil
	}

	picked, err := c.Search(ctx, where, args, 1, "name",
		int(rng.RandRange(total)))
	if err != nil {
		return nil, fmt.Errorf("wheel: %w", err)
	}
	if len(picked) == 0 {
		// A pool refreshed between the count and the fetch, or an offset
		// past the end. Rare enough to report rather than to retry.
		return append(base,
			wire.KV{Key: "card", Value: nil},
			wire.KV{Key: "reason", Value: "The card slipped off the wheel — spin again."}), nil
	}
	rec := picked[0]
	ci := append([]string(nil), rec.ColorIdentity...)
	sort.Strings(ci)
	return append(base, wire.KV{Key: "card", Value: wire.OrderedMap{
		{Key: "name", Value: rec.Name},
		{Key: "mana_cost", Value: rec.ManaCost},
		{Key: "type_line", Value: rec.TypeLine},
		{Key: "oracle_text", Value: rec.OracleText},
		{Key: "color_identity", Value: ci},
		{Key: "image", Value: rec.ImageNormal},
		{Key: "art_crop", Value: rec.ImageArtCrop},
	}}), nil
}

// deckNames is the exclusion set: the 99, the command zone and the
// companion, sorted — UTF-8 byte order equals code-point order, which is
// what Python's `sorted` gives.
func deckNames(d *deck.Deck) []string {
	seen := map[string]bool{}
	for _, c := range d.Cards {
		seen[c.Name] = true
	}
	for _, name := range d.Commander {
		seen[name] = true
	}
	if d.Companion != nil {
		seen[*d.Companion] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// identityColors is `sorted(c for c in identity if c in "WUBRG")`.
func identityColors(identity map[string]bool) []string {
	out := []string{}
	for _, col := range []string{"B", "G", "R", "U", "W"} {
		if identity[col] {
			out = append(out, col)
		}
	}
	return out
}
