// Package decklog is the deck activity log: what has been done to a deck,
// newest first (ADR 28). Every entry is written from the one commit call
// site, and the rows are read back for the deck page's History tab. `owner_id IS ?` rather than `= ?`, because the file
// tier's owner is NULL and `NULL = NULL` is not true in SQL -- the whole
// reason the query reads the way it does.
package decklog

import (
	"context"
	"database/sql"
)

// DefaultLimit is how many entries a reader gets when it does not say.
const DefaultLimit = 50

// Entry is one row, as the wire carries it: id, when, who, the verb and the
// sentence. `actor` is a username or null for whoever was at the machine.
type Entry struct {
	ID        int64   `json:"id"`
	CreatedAt string  `json:"created_at"`
	Actor     *string `json:"actor"`
	Action    string  `json:"action"`
	Summary   string  `json:"summary"`
}

// Entries is `log.entries`: this deck's history, newest first. A nil db is
// an instance with no app.db yet, which is a deck with no history.
func Entries(ctx context.Context, db *sql.DB, ownerID *int64, slug string, limit int) ([]Entry, error) {
	out := []Entry{}
	if db == nil {
		return out, nil
	}
	if limit < 1 {
		limit = 1
	}
	var owner any
	if ownerID != nil {
		owner = *ownerID
	}
	rows, err := db.QueryContext(ctx,
		"SELECT id, created_at, actor, action, summary FROM deck_log"+
			" WHERE owner_id IS ? AND slug = ? ORDER BY id DESC LIMIT ?", owner, slug, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e Entry
		var actor sql.NullString
		if err := rows.Scan(&e.ID, &e.CreatedAt, &actor, &e.Action, &e.Summary); err != nil {
			return nil, err
		}
		if actor.Valid {
			a := actor.String
			e.Actor = &a
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
