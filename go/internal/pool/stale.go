package pool

import (
	"context"
	"fmt"
)

// Stale is `cards/db.py:pool_is_stale`: does this pool predate the columns
// the app now reads? A database loaded before those columns existed answers
// every query about them with NULL, which reads exactly like "this card has
// no power" — the quiet wrong answer rule 1 exists to prevent, arriving
// through the very field added to prevent it. Two questions because two
// tables have grown — the printed stats on `oracle_cards`, and the painter
// on `printings` — and they share one word because nobody would read two
// flags separately.
//
// Cheap: one probe per table, short-circuited by LIMIT. Creatures are over
// half the pool and paper printings essentially all name a painter, so a
// current database answers immediately.
func Stale(ctx context.Context, c *Conn) (bool, error) {
	any, err := probe(ctx, c, "SELECT 1 FROM oracle_cards LIMIT 1")
	if err != nil {
		return false, err
	}
	if !any {
		// An empty pool is not a stale one — there is nothing to be wrong
		// about, and `health` already reports the pool as missing.
		return false, nil
	}
	oracle, err := c.Columns(ctx, "oracle_cards")
	if err != nil {
		return false, err
	}
	if !oracle["power"] {
		return true, nil
	}
	powered, err := probe(ctx, c,
		"SELECT 1 FROM oracle_cards WHERE power IS NOT NULL LIMIT 1")
	if err != nil {
		return false, err
	}
	if !powered {
		return true, nil
	}
	// `--oracle-only` is a supported refresh, so an empty `printings` is a
	// deliberate state rather than an old one and must not read as stale.
	printed, err := probe(ctx, c, "SELECT 1 FROM printings LIMIT 1")
	if err != nil {
		return false, err
	}
	if !printed {
		return false, nil
	}
	printings, err := c.Columns(ctx, "printings")
	if err != nil {
		return false, err
	}
	if !printings["artist"] {
		return true, nil
	}
	signed, err := probe(ctx, c,
		"SELECT 1 FROM printings WHERE artist IS NOT NULL LIMIT 1")
	if err != nil {
		return false, err
	}
	return !signed, nil
}

func probe(ctx context.Context, c *Conn, query string) (bool, error) {
	rows, err := c.DB().QueryContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("pool probe: %w", err)
	}
	defer func() { _ = rows.Close() }()
	found := rows.Next()
	return found, rows.Err()
}
