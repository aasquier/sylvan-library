package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// TokenBytes is how many random bytes go into a token
// (43 characters once encoded). ADR 5 names it.
const TokenBytes = 32

// Lifetime is a session's life, matching the cookie's Max-Age.
const Lifetime = 14 * 24 * time.Hour

// TouchInterval is how often `last_seen_at` is worth a write. Every
// authenticated request would otherwise be one, and the value is only ever
// read by a human wondering when an account was last used. `LookupTouching`
// pays it; the read-only `Lookup` below never writes at all.
const TouchInterval = 5 * time.Minute

// Session is what a token resolved to. Never the token itself.
type Session struct {
	UserID    int64
	CreatedAt time.Time
	ExpiresAt time.Time
}

// HashToken is the SHA-256 of the token, as lowercase
// hex. What `app.db` stores, so that reading the file never hands over a live
// session (ADR 5).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Lookup resolves a token to its session, or nil when it is unknown or
// expired. Read-only: an expired row is simply not a session here, and the
// deleting and touching belong to `LookupTouching`, the write half.
func Lookup(ctx context.Context, db *sql.DB, token string) (*Session, error) {
	if token == "" {
		return nil, nil
	}
	var userID int64
	var created, expires string
	err := db.QueryRowContext(ctx,
		"SELECT user_id, created_at, expires_at FROM sessions WHERE token_hash = ?",
		HashToken(token)).Scan(&userID, &created, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session lookup: %w", err)
	}
	expiresAt, err := ParseTimestamp(expires)
	if err != nil {
		return nil, fmt.Errorf("session expires_at: %w", err)
	}
	if !expiresAt.After(time.Now()) {
		return nil, nil
	}
	createdAt, err := ParseTimestamp(created)
	if err != nil {
		return nil, fmt.Errorf("session created_at: %w", err)
	}
	return &Session{UserID: userID, CreatedAt: createdAt, ExpiresAt: expiresAt}, nil
}

// ParseTimestamp reads the recorded column format: RFC 3339
// with a `+00:00` offset and microseconds when they are non-zero
// (`2026-08-21T12:00:00.123456+00:00`, or `2026-08-21T12:00:00+00:00`). A
// timestamp with no offset at all -- which nothing in this package writes,
// but a hand edit might -- is read as UTC rather than refused.
func ParseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05.999999999", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("not an ISO 8601 timestamp: %q", s)
}
