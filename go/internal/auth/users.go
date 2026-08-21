package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// User is an account as the door needs to know it: enough to decide whether a
// request may pass and whether it may pass under the admin prefix. No email,
// no password hash -- the same rule `auth/users.py:User` keeps, one layer
// further in: a field never loaded cannot leak.
type User struct {
	ID       int64
	Username string
	IsAdmin  bool
	Disabled bool
}

// GetByID reads one account, or nil when there is none.
func GetByID(ctx context.Context, db *sql.DB, id int64) (*User, error) {
	var u User
	var disabledAt sql.NullString
	var isAdmin int64
	err := db.QueryRowContext(ctx,
		"SELECT id, username, is_admin, disabled_at FROM users WHERE id = ?", id,
	).Scan(&u.ID, &u.Username, &isAdmin, &disabledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("user lookup: %w", err)
	}
	u.IsAdmin = isAdmin != 0
	u.Disabled = disabledAt.Valid && disabledAt.String != ""
	return &u, nil
}
