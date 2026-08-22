package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"fmt"
	"time"
)

// The write half of `auth/sessions.py`. Its read half is `sessions.go`, which
// the door has had since Phase 2; these are the statements a login, a logout
// and a revocation make.

// TokenURLSafe is `secrets.token_urlsafe(n)`: n random bytes from the OS,
// urlsafe-base64 encoded with the padding stripped -- 43 characters for the 32
// bytes both a session and an auth token draw. The alphabet matters as much as
// the entropy: a session token rides in a cookie and an auth token rides in a
// URL fragment, and `+` and `/` survive neither reliably.
//
// It panics if the OS entropy source fails, which is what Python does one
// layer down: there is no sensible weaker token, and a caller handed one would
// mint a guessable session.
func TokenURLSafe(n int) string {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Sprintf("auth: no entropy for a token: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// CreateSession opens a session and returns its token. The only time the token
// exists: the caller sends it to the browser and forgets it, and there is no
// way to read it back out of the database, by design.
func CreateSession(ctx context.Context, db *sql.DB, userID int64) (string, error) {
	return createSession(ctx, db, userID, Lifetime)
}

func createSession(ctx context.Context, db *sql.DB, userID int64,
	lifetime time.Duration) (string, error) {

	token := TokenURLSafe(TokenBytes)
	now := time.Now().UTC()
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO sessions (token_hash, user_id, created_at, expires_at,"+
				" last_seen_at) VALUES (?, ?, ?, ?, ?)",
			HashToken(token), userID, isoAt(now), isoAt(now.Add(lifetime)), isoAt(now))
		return err
	})
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}
	return token, nil
}

// DeleteSession ends one session -- logging out, and half of regenerating on
// login.
func DeleteSession(ctx context.Context, db *sql.DB, token string) error {
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?",
			HashToken(token))
		return err
	})
	if err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	return nil
}

// DeleteSessionsForUser ends every session for one account and returns how
// many.
//
// Called whenever a password changes (ADR 16). A reset is usually somebody who
// suspects compromise, and a reset that leaves the attacker logged in has
// answered the wrong question.
func DeleteSessionsForUser(ctx context.Context, db *sql.DB, userID int64) (int64, error) {
	var ended int64
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
		if err != nil {
			return err
		}
		ended, _ = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("revoke sessions: %w", err)
	}
	return ended, nil
}

// CountSessionsForUser is how many live sessions this account has.
func CountSessionsForUser(ctx context.Context, db *sql.DB, userID int64) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM sessions WHERE user_id = ? AND expires_at > ?",
		userID, nowISO()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count sessions: %w", err)
	}
	return n, nil
}

// PurgeExpiredSessions drops every session past its expiry, returning how
// many.
func PurgeExpiredSessions(ctx context.Context, db *sql.DB) (int64, error) {
	var gone int64
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= ?", nowISO())
		if err != nil {
			return err
		}
		gone, _ = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("purge sessions: %w", err)
	}
	return gone, nil
}
