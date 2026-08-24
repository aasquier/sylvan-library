package tools

import (
	"context"
	"fmt"

	"github.com/aasquier/sylvan-library/go/internal/deck"
	"github.com/aasquier/sylvan-library/go/internal/deckread"
	"github.com/aasquier/sylvan-library/go/internal/pool"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The seven read-only tools, wired to `internal/deckread` — the same payload
// builders the routes answer with, which is the whole reason that package was
// extracted. A tool result and a route payload are therefore the same bytes by
// construction.
//
// Every handler here is a READ. That is not a promise this comment makes; it
// is what `internal/claude`'s boundary analysis checks over the typed call
// graph, this package included.

// registerHandlers wires every schema to its function.
//
// Called from the package's single init in tools.go rather than being an init
// of its own, and that is not style. Go runs a package's init functions in
// FILE-NAME order, so `handlers.go` fires before `tools.go` — which meant the
// registry was still empty when the first Register landed, and the package
// panicked at load with "no schema for list_decks". An ordering that depends
// on a filename is an ordering nobody can see; this one is a call.
func registerHandlers() {
	Register("list_decks", listDecks)
	Register("get_deck", getDeck)
	Register("validate_deck", validateDeck)
	Register("deck_stats", deckStats)
	Register("suggest_replacements", suggestReplacements)
	Register("get_cards", getCards)
	Register("search_cards", searchCards)
}

// source pulls the deck source out of Deps, refusing rather than guessing when
// there is none. A deck tool with no source is a surface misconfiguration, and
// the model should be told the door is shut rather than handed an empty
// library it would then reason about as if the shelf were bare.
func source(deps Deps) (deckread.Source, error) {
	src, ok := deps.Source.(deckread.Source)
	if !ok || src == nil {
		return nil, &ErrNotAllowed{Msg: "no deck library is reachable from this surface"}
	}
	return src, nil
}

// conn pulls the pool out of Deps. Unlike a missing source this is NOT a
// refusal for the deck tools: a deck answered without a pool is still a deck,
// and `pool_available` says which happened. Only the two card tools require
// one, because a card lookup with no pool has nothing to say.
func conn(deps Deps) *pool.Conn {
	c, _ := deps.Pool.(*pool.Conn)
	return c
}

// deckFor resolves the `slug` argument to a deck, or refuses by name.
//
// The refusal names the slug rather than being a stack trace, because it is
// handed back to the model as a tool result: the model's recovery is to call
// `list_decks` and ask again, and it can only do that if it is told which name
// failed. `converse` prefixes the class name, so what the model reads is
// `DeckNotFound: gyome` -- see ErrDeckNotFound for why the message is bare.
func deckFor(ctx context.Context, deps Deps, args map[string]any) (deckread.Source, string, *deck.Deck, error) {
	src, err := source(deps)
	if err != nil {
		return nil, "", nil, err
	}
	slug, _ := args["slug"].(string)
	d, err := src.Get(ctx, slug)
	if err != nil {
		return nil, "", nil, &ErrDeckNotFound{Slug: slug}
	}
	return src, slug, d, nil
}

func listDecks(ctx context.Context, _ map[string]any, deps Deps) (any, error) {
	src, err := source(deps)
	if err != nil {
		return nil, err
	}
	decks, err := src.All(ctx)
	if err != nil {
		return nil, err
	}
	return deckread.Tiles(ctx, conn(deps), decks, src.Writable(), "")
}

func getDeck(ctx context.Context, args map[string]any, deps Deps) (any, error) {
	src, _, d, err := deckFor(ctx, deps, args)
	if err != nil {
		return nil, err
	}
	body, err := deckread.DeckPayload(ctx, conn(deps), d, src.Writable(), "")
	if err != nil {
		return nil, err
	}
	// Ordered pairs go to the model as an ordered object, for the same reason
	// they go to the browser as one: the key order is the recorded one, and
	// the model reads a deck top-down exactly as a person does.
	return wire.OrderedMap(body), nil
}

func validateDeck(ctx context.Context, args map[string]any, deps Deps) (any, error) {
	_, _, d, err := deckFor(ctx, deps, args)
	if err != nil {
		return nil, err
	}
	rep, err := deckread.Validate(ctx, conn(deps), d)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": rep.OK(), "errors": rep.Errors(), "warnings": rep.Warnings()}, nil
}

func deckStats(ctx context.Context, args map[string]any, deps Deps) (any, error) {
	_, _, d, err := deckFor(ctx, deps, args)
	if err != nil {
		return nil, err
	}
	return deckread.Stats(ctx, conn(deps), d)
}

func suggestReplacements(ctx context.Context, args map[string]any, deps Deps) (any, error) {
	_, slug, d, err := deckFor(ctx, deps, args)
	if err != nil {
		return nil, err
	}
	c := conn(deps)
	if c == nil {
		return map[string]any{"slug": slug, "pool_available": false, "targets": []any{}}, nil
	}
	limit := 5
	if n, ok := args["limit"].(float64); ok && n > 0 {
		limit = int(n)
	}
	targets, err := deckread.Suggestions(ctx, c, d, limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"slug": slug, "pool_available": true, "targets": targets}, nil
}

func getCards(ctx context.Context, args map[string]any, deps Deps) (any, error) {
	c := conn(deps)
	if c == nil {
		return nil, fmt.Errorf("the card pool is not available on this instance")
	}
	raw, _ := args["names"].([]any)
	names := make([]string, 0, len(raw))
	for _, n := range raw {
		if s, ok := n.(string); ok {
			names = append(names, s)
		}
	}
	return deckread.CardsNamed(ctx, c, names)
}

func searchCards(ctx context.Context, args map[string]any, deps Deps) (any, error) {
	c := conn(deps)
	if c == nil {
		return nil, fmt.Errorf("the card pool is not available on this instance")
	}
	q := deckread.SearchQuery{Limit: 60}
	if v, ok := args["q"].(string); ok {
		q.Text = v
	}
	if v, ok := args["identity"].(string); ok {
		q.Identity = v
	}
	if v, ok := args["type_line"].(string); ok {
		q.TypeLine = v
	}
	if v, ok := args["sort"].(string); ok {
		q.Sort = v
	}
	if v, ok := args["identity_exact"].(bool); ok {
		q.IdentityExact = v
	}
	if v, ok := args["commanders_only"].(bool); ok {
		q.CommandersOnly = v
	}
	if v, ok := args["cmc_max"].(float64); ok {
		q.CMCMax, q.HaveCMC = v, true
	}
	if v, ok := args["price_max"].(float64); ok {
		q.PriceMax, q.HavePrice = v, true
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		// Capped here as well as in the schema: the schema is advice to the
		// model and this is the rule.
		q.Limit = min(int(v), 200)
	}
	found, err := deckread.SearchCards(ctx, c, q)
	if err != nil {
		return nil, err
	}
	return map[string]any{"cards": found, "total": len(found)}, nil
}
