package night

import (
	"context"
	"database/sql"
	"errors"
)

// The roster's two halves, and why they are read so differently.
//
// The players are rows: rung 13 stored the standing consent as
// `user_decks.coliseum_at_night` with a partial index built for exactly this
// query, so "whose decks are in?" is an indexed read over the opted-in few.
// The house is not rows at all — the showcase decks live on the file tier,
// they carry no flag, and none is consulted, because **the house always
// plays** (Aaron, 2026-09-05, closing rung 13's open ruling). The runner is
// handed the house's slugs through a seam ([RunnerConfig.House]) rather than
// reading the directory here, so this package never learns where the file
// tier lives and a test can put anyone it likes in the house.
//
// Neither half is filtered for playability. A deck the gate rejects or Forge
// cannot cover is mustered anyway and refused at its own bout's pre-flight,
// where the refusal becomes one honest `skipped` row with a reason — the
// deliberately-invalid showcase deck surfacing that way every night is the
// gate's live demonstration working, not a bug.

// PlayerDecks is the night's opening question — every deck whose owner has
// entered it for the games after dark. Rung 13's partial-index query: opted
// in, not in the crypt. Ordered by owner then slug so the pairing starts from
// a stable deal, whatever order SQLite would have liked.
func (s *Store) PlayerDecks(ctx context.Context) ([]Seat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT owner_id, slug FROM user_decks`+
			` WHERE coliseum_at_night = 1 AND deleted_at IS NULL`+
			` ORDER BY owner_id, slug`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Seat{}
	for rows.Next() {
		var owner int64
		var slug string
		if err := rows.Scan(&owner, &slug); err != nil {
			return nil, err
		}
		id := owner
		out = append(out, Seat{Owner: &id, Slug: slug})
	}
	return out, rows.Err()
}

// Entered is the same question one deck at a time, asked again at the bout's
// own turn. The card is dealt from the flag as it stood at run open, but the
// flag is *standing* consent, and standing consent can be withdrawn between
// the deal and the fight — an owner who stepped back out at 23:10 must not
// have their deck read at 23:40 on the strength of how 22:00 looked.
// Deletion was always honoured (the read fails and the bout skips); this
// closes the same door for the quieter exit. A deck that has left
// `user_decks` entirely answers false too, which the fight-time check
// reports the same way.
func (s *Store) Entered(ctx context.Context, owner int64, slug string) (bool, error) {
	var one int
	err := s.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_decks WHERE owner_id = ? AND slug = ?`+
			` AND coliseum_at_night = 1 AND deleted_at IS NULL LIMIT 1`,
		owner, slug).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
