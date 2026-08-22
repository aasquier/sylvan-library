package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Single-use, time-limited tokens -- `mtglab/auth/tokens.py`. One
// implementation, two entry points.
//
// ADR 16 is specific about why this is one module rather than an invite flow
// and a reset flow: **"a bespoke second path is how one of them ends up
// weaker."** So an invite and a reset differ here in exactly two ways -- the
// string in the `purpose` column and how long the row lives -- and every rule
// below applies to both without being written twice.
//
// The rules, all of them from ADR 16:
//
//   - **Stored hashed**, never in the clear. Same reasoning as a session, and
//     the same SHA-256 for the same reason: a 256-bit random token cannot be
//     guessed at any cost, so Argon2 would buy nothing and cost 19 MiB a check.
//   - **Single use.** `Redeem` marks the row consumed inside the transaction
//     that sets the password, so two clicks on one link cannot both succeed.
//   - **Short-lived** -- one hour for a reset, a week for an invite. Different
//     numbers because they are different risks: a reset answers something the
//     account holder is doing right now, and an invite grants nothing at all
//     until it is used.
//   - **Redeeming revokes every session for that account.**
//
// What is deliberately not here is anything that sends a message. Issuing a
// token and delivering it are separate jobs; `invites.go` joins them, and it is
// the only file in this package that talks to a network.

// TokenSize is 32 bytes, urlsafe-encoded to 43 characters -- the same size as
// a session token and for the same reason. This one travels in a URL fragment,
// so it has to survive being pasted out of a mail client.
const TokenSize = 32

// Purpose is why a token was issued. Not interchangeable, and checked on
// redemption: an invite lives longer than a reset, so a flow that accepted
// either would quietly hand a reset the invite's week.
type Purpose string

// The two purposes.
const (
	PurposeInvite Purpose = "invite"
	PurposeReset  Purpose = "reset"
)

// Lifetimes: an hour for a reset, exactly as ADR 16 says. A week for an
// invite -- it is a link somebody has to notice in their inbox, act on and
// possibly ask about first, and it confers nothing until it is used. An
// expired invite is a message to the maintainer; an expired reset is a person
// locked out for the length of one more round trip.
var Lifetimes = map[Purpose]time.Duration{
	PurposeInvite: 7 * 24 * time.Hour,
	PurposeReset:  time.Hour,
}

// The refusals. ErrTokenInvalid, ErrTokenExpired and ErrTokenUsed all mean
// "this link cannot be redeemed" and the endpoint answers them with a 400 and
// a counted rate-limit failure, because a stream of them is somebody guessing.
//
// ErrWrongPurpose is **deliberately not one of them**: it means the link is
// perfectly good and the request asked it to do something that kind of link
// does not do -- renaming an account from a reset link. Spending the token or
// counting a failure for that would punish the holder for their client's bug.
var (
	// ErrTokenInvalid is no such token: a typo, a forgery, or a link from a
	// purged database.
	ErrTokenInvalid = errors.New("invalid token")
	// ErrTokenExpired is real, but past its expiry.
	ErrTokenExpired = errors.New("expired token")
	// ErrTokenUsed is real, and already redeemed. Single use is the rule; this
	// is it holding.
	ErrTokenUsed = errors.New("used token")
	// ErrWrongPurpose is a good link asked the wrong question.
	ErrWrongPurpose = errors.New("wrong purpose")
)

// IsTokenError reports whether err is one of the three refusals a bad link
// produces -- the ones a route answers 400 for and counts. ErrWrongPurpose is
// not among them, which is the whole reason this predicate exists rather than
// a type switch at the call site.
func IsTokenError(err error) bool {
	return errors.Is(err, ErrTokenInvalid) || errors.Is(err, ErrTokenExpired) ||
		errors.Is(err, ErrTokenUsed)
}

// Token is a resolved token. It never carries the secret -- only what it
// resolved to.
type Token struct {
	UserID    int64
	Purpose   Purpose
	CreatedAt time.Time
	ExpiresAt time.Time
}

// IssueToken mints a token and returns it. The only time it exists in the
// clear.
//
// **Any outstanding token of the same purpose for that account is dropped**,
// so there is one live link per purpose at a time. That is what makes a
// re-issued reset a *replacement*: somebody who suspects a message went astray
// asks for another, and the one they are worried about stops working the
// moment they do. Tokens of the *other* purpose are left alone, because an
// unclaimed invite and a reset request answer different questions.
//
// Used rows survive, so a second click still reports "already used" rather
// than the flat "invalid" that leaves somebody wondering whether they mistyped
// it.
func IssueToken(ctx context.Context, db *sql.DB, userID int64, purpose Purpose) (string, error) {
	return issueToken(ctx, db, userID, purpose, Lifetimes[purpose])
}

func issueToken(ctx context.Context, db *sql.DB, userID int64, purpose Purpose,
	lifetime time.Duration) (string, error) {

	token := TokenURLSafe(TokenSize)
	now := time.Now().UTC()
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM auth_tokens WHERE user_id = ? AND purpose = ?"+
				" AND used_at IS NULL", userID, string(purpose)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO auth_tokens (token_hash, user_id, purpose, created_at,"+
				" expires_at) VALUES (?, ?, ?, ?, ?)",
			HashToken(token), userID, string(purpose), isoAt(now), isoAt(now.Add(lifetime)))
		return err
	})
	if err != nil {
		return "", fmt.Errorf("issue token: %w", err)
	}
	return token, nil
}

// LookupToken resolves a token, or says why it cannot be used.
//
// Returning three distinct errors rather than nil, unlike a session lookup,
// because the refusals are worth telling apart *to the person holding the
// link* -- "this expired" and "you already used this" are actionable and
// "invalid" is not. None of them tells anybody anything they did not already
// have: the token is the secret, and possessing it is the only way to get an
// answer that is not ErrTokenInvalid.
//
// An empty purpose is "any". A caller that does not care is a caller that
// would accept a week-old invite as an hour-old reset.
func LookupToken(ctx context.Context, db *sql.DB, token string, purpose Purpose) (*Token, error) {
	if token == "" {
		return nil, failf("%w: no token supplied", ErrTokenInvalid)
	}
	var userID int64
	var found, created, expires string
	var usedAt sql.NullString
	err := db.QueryRowContext(ctx,
		"SELECT user_id, purpose, created_at, expires_at, used_at"+
			" FROM auth_tokens WHERE token_hash = ?", HashToken(token)).
		Scan(&userID, &found, &created, &expires, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, failf("%w: that link is not valid", ErrTokenInvalid)
	}
	if err != nil {
		return nil, fmt.Errorf("token lookup: %w", err)
	}
	if purpose != "" && Purpose(found) != purpose {
		// Deliberately the same message as an unknown token. A caller who has
		// an invite and is poking the reset endpoint learns nothing from it.
		return nil, failf("%w: that link is not valid", ErrTokenInvalid)
	}
	if usedAt.Valid && usedAt.String != "" {
		return nil, failf("%w: that link has already been used", ErrTokenUsed)
	}
	expiresAt, err := ParseTimestamp(expires)
	if err != nil {
		return nil, fmt.Errorf("token expires_at: %w", err)
	}
	if !expiresAt.After(time.Now()) {
		return nil, failf("%w: that link has expired", ErrTokenExpired)
	}
	createdAt, err := ParseTimestamp(created)
	if err != nil {
		return nil, fmt.Errorf("token created_at: %w", err)
	}
	return &Token{UserID: userID, Purpose: Purpose(found),
		CreatedAt: createdAt, ExpiresAt: expiresAt}, nil
}

// RedeemToken consumes a token and sets that account's password. One
// transaction.
//
// `username` renames the account as it is claimed, and is **refused on a
// reset**. An invite's holder is naming themselves for the first time; a
// reset's holder already has a name other people have seen, and a forgotten
// password is not a reason to be handed a rename -- worse, it makes "somebody
// got into my email" and "somebody took over my identity here" the same
// incident. The gate is the token's own purpose, read from the database, so a
// client that sends the field anyway cannot widen it.
//
// Everything that must not happen separately happens in one transaction: the
// token is marked used, the password is written, and every session for the
// account is deleted. A failure anywhere rolls back all three, so there is no
// state in which the link is spent but the password is not set -- which is the
// failure that turns a forgotten password into a locked-out account.
//
// Three things happen *before* the transaction opens, on purpose:
//
//   - the token is resolved, so an invalid link costs a primary-key lookup
//     rather than an Argon2 hash. The endpoint is unauthenticated; hashing
//     first would make it a 19 MiB-per-request denial of service.
//   - the password is checked and hashed, because ~50ms of Argon2 inside a
//     write transaction is 50ms of held lock for no reason.
//   - the account is checked for `disabled_at`. **A reset must not re-enable a
//     disabled account.** Disabling is the maintainer's revocation lever, and a
//     lever somebody can undo from their own inbox is not one.
func RedeemToken(ctx context.Context, db *sql.DB, token, password string,
	purpose Purpose, username string) (*User, error) {

	resolved, err := LookupToken(ctx, db, token, purpose)
	if err != nil {
		return nil, err
	}

	account, err := GetByID(ctx, db, resolved.UserID)
	if err != nil {
		return nil, err
	}
	// A cascade should prevent the first; a disabled account is the second and
	// gets the same sentence, because it is not a holder who is owed an
	// explanation of why.
	if account == nil || account.Disabled {
		return nil, failf("%w: that link is not valid", ErrTokenInvalid)
	}

	if username != "" && resolved.Purpose != PurposeInvite {
		return nil, failf("%w: a reset link cannot change your username", ErrWrongPurpose)
	}

	// Shape-checked before the transaction, for the same reason the password
	// is: a name the rules refuse should cost a regex and not a write. The
	// *uniqueness* of it cannot be settled here -- only the UNIQUE index can do
	// that without a race -- so the rename happens inside, where a collision
	// rolls the token spend back with it.
	if username != "" {
		if _, err := NormaliseUsername(username); err != nil {
			return nil, err
		}
	}

	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	err = inTx(ctx, db, func(tx *sql.Tx) error {
		marked, err := tx.ExecContext(ctx,
			"UPDATE auth_tokens SET used_at = ?"+
				" WHERE token_hash = ? AND used_at IS NULL", nowISO(), HashToken(token))
		if err != nil {
			return err
		}
		if n, _ := marked.RowsAffected(); n == 0 {
			// Redeemed by another request between the lookup above and here.
			// `UPDATE ... WHERE used_at IS NULL` is what makes single-use a
			// property of the database rather than of the check order.
			return failf("%w: that link has already been used", ErrTokenUsed)
		}
		if username != "" {
			// Failing here rolls back the `used_at` above with it, which is the
			// whole reason this is not two transactions: a taken name must
			// leave a *retryable* invite rather than a spent link and an
			// account nobody can get into.
			if _, err := applyUsername(ctx, tx, resolved.UserID, username); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?",
			hash, resolved.UserID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", resolved.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}

	refreshed, err := GetByID(ctx, db, resolved.UserID)
	if err != nil {
		return nil, err
	}
	if refreshed == nil { // unreachable in practice
		return nil, failf("%w: that link is not valid", ErrTokenInvalid)
	}
	return refreshed, nil
}

// TokenOutstanding reports whether this account has a live link of that kind.
func TokenOutstanding(ctx context.Context, db *sql.DB, userID int64,
	purpose Purpose) (bool, error) {

	var n int64
	err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM auth_tokens WHERE user_id = ?"+
			" AND purpose = ? AND used_at IS NULL AND expires_at > ?",
		userID, string(purpose), nowISO()).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("outstanding tokens: %w", err)
	}
	return n > 0, nil
}

// KeepUsedTokensFor is how long a spent row lingers, so that "already used"
// survives long enough to be the answer somebody actually gets when they click
// a link twice a week apart.
const KeepUsedTokensFor = 30 * 24 * time.Hour

// PurgeExpiredTokens drops what nothing will read again, returning how many
// rows went. Expired-and-unused rows go immediately; used ones linger.
func PurgeExpiredTokens(ctx context.Context, db *sql.DB, keepUsedFor time.Duration) (int64, error) {
	now := time.Now().UTC()
	var gone int64
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			"DELETE FROM auth_tokens WHERE (used_at IS NULL AND expires_at <= ?)"+
				" OR (used_at IS NOT NULL AND used_at <= ?)",
			isoAt(now), isoAt(now.Add(-keepUsedFor)))
		if err != nil {
			return err
		}
		gone, _ = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("purge tokens: %w", err)
	}
	return gone, nil
}
