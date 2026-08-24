package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// A fixed window in SQLite, guarding the endpoints that take a password.
//
// A fixed window in SQLite is enough: this is a private site with a dozen
// accounts on one machine, and the thing being prevented is an unattended
// script working through a password list, not a distributed campaign.
//
// **Two keys per attempt, because either alone is trivially sidestepped.** By
// account only, and an attacker sprays one password across every username. By
// address only, and a botnet's worth of addresses each get a full budget
// against one account. The account key bounds how hard any single account can
// be attacked; the address key bounds how much any single client can attempt.
//
// Only *failures* count, and a success clears both. Somebody logging in
// repeatedly and correctly -- a shared machine, a flaky network, a test suite
// -- is not what this is for, and locking them out would be the limiter causing
// the outage it exists to prevent.
//
// The window is fixed rather than sliding, which means an attacker who times
// it right gets two full budgets across a boundary. That is a real property and
// an acceptable one: 20 attempts instead of 10, against Argon2 at 19 MiB a go,
// is not the difference between safe and breached.
//
// Two things worth restating rather than tidying. The table is still called
// `login_attempts`, which is now half a lie -- it counts attempts against a
// budget, and the reset endpoint is the caller that made the distinction
// visible: a reset has no "success" to clear the counter with, so *every*
// request is counted. And "address" is overloaded: `AddressKey` has always
// meant the client's IP, while ADR 16's "rate-limited per address" means the
// mailbox. The mailbox keys are `EmailKey`, which hashes -- and the reason it
// hashes is not secrecy, it is that a limiter keyed on the plaintext would
// accumulate the addresses of people who do *not* have accounts, which is
// personal data this project has no reason to hold.

// Limit is how many failures inside how long.
type Limit struct {
	Failures int
	Window   time.Duration
}

// Describe is the sentence `mtglab users` prints.
func (l Limit) Describe() string {
	return fmt.Sprintf("%d attempts per %d minutes", l.Failures, int(l.Window.Minutes()))
}

// The budgets, each argued below and none of them tunable in passing.
//
// Ten wrong passwords for one account in a quarter hour is somebody who has
// forgotten theirs; the eleventh is a script. Thirty from one address is
// generous for a household behind one NAT and far short of useful for
// guessing. Three reset links to one mailbox in an hour covers "it did not
// arrive, try again" twice over; beyond that somebody is mail-bombing an
// address they do not control. Twenty claims is not about guessing a 256-bit
// token -- it is that `/api/auth/claim` is unauthenticated and hashes a
// password with Argon2, and 19 MiB per request is worth a ceiling even behind
// a check that rejects a bad token before any of it is spent.
var (
	PerAccount      = Limit{Failures: 10, Window: 15 * time.Minute}
	PerAddress      = Limit{Failures: 30, Window: 15 * time.Minute}
	ResetPerMailbox = Limit{Failures: 3, Window: time.Hour}
	ResetPerAddress = Limit{Failures: 10, Window: time.Hour}
	ClaimPerAddress = Limit{Failures: 20, Window: 15 * time.Minute}
)

// AccountKey is one account's budget.
func AccountKey(username string) string {
	return "user:" + strings.ToLower(strings.TrimSpace(username))
}

// AddressKey is a client's IP, per flow.
//
// Scoped so that failing to redeem a link cannot spend the budget somebody
// needs to log in. One shared IP counter across every unauthenticated endpoint
// would make any one of them a way to lock a client out of the others.
func AddressKey(address, scope string) string {
	if scope == "" {
		scope = "login"
	}
	return "ip:" + scope + ":" + address
}

// EmailKey is a mailbox, keyed by hash.
//
// Never the plaintext: a reset request for an address with no account would
// otherwise store that address, which is personal data the project has no
// reason to be holding for somebody who is not a user. 32 hex characters of
// SHA-256 is 128 bits, a great deal more than is needed to keep a handful of
// accounts from colliding.
func EmailKey(address string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(address))))
	return "mailbox:" + hex.EncodeToString(sum[:])[:32]
}

// current is this key's window start and failure count, both reset if the
// window has lapsed.
func current(ctx context.Context, db *sql.DB, key string, limit Limit) (time.Time, int, error) {
	now := time.Now().UTC()
	var started string
	var failures int
	err := db.QueryRowContext(ctx,
		"SELECT window_start, failures FROM login_attempts WHERE key = ?", key).
		Scan(&started, &failures)
	if errors.Is(err, sql.ErrNoRows) {
		return now, 0, nil
	}
	if err != nil {
		return now, 0, fmt.Errorf("rate limit: %w", err)
	}
	at, err := ParseTimestamp(started)
	if err != nil {
		return now, 0, fmt.Errorf("rate limit window_start: %w", err)
	}
	if now.Sub(at) >= limit.Window {
		return now, 0, nil
	}
	return at, failures, nil
}

// Exhausted asks whether this key is out of attempts. A read; it records
// nothing.
func Exhausted(ctx context.Context, db *sql.DB, key string, limit Limit) (bool, error) {
	_, failures, err := current(ctx, db, key, limit)
	return failures >= limit.Failures, err
}

// RecordFailure counts one failed attempt against this key and returns the new
// count.
func RecordFailure(ctx context.Context, db *sql.DB, key string, limit Limit) (int, error) {
	started, failures, err := current(ctx, db, key, limit)
	if err != nil {
		return 0, err
	}
	err = inTx(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO login_attempts (key, window_start, failures)"+
				" VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET"+
				" window_start = excluded.window_start, failures = excluded.failures",
			key, isoAt(started), failures+1)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("rate limit: %w", err)
	}
	return failures + 1, nil
}

// ClearLimit forgets this key's failures. Called on a successful login.
func ClearLimit(ctx context.Context, db *sql.DB, key string) error {
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM login_attempts WHERE key = ?", key)
		return err
	})
	if err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}
	return nil
}

// RetryAfter is seconds until this key's window lapses, for a `Retry-After`
// header. Never below one: a header of zero reads as "try now", which is the
// one thing the answer is saying not to do.
func RetryAfter(ctx context.Context, db *sql.DB, key string, limit Limit) (int, error) {
	started, _, err := current(ctx, db, key, limit)
	if err != nil {
		return 1, err
	}
	remaining := started.Add(limit.Window).Sub(time.Now().UTC())
	if seconds := int(remaining.Seconds()); seconds > 1 {
		return seconds, nil
	}
	return 1, nil
}

// PurgeStaleLimits drops windows nothing will read again, returning how many.
func PurgeStaleLimits(ctx context.Context, db *sql.DB, olderThan time.Duration) (int64, error) {
	cutoff := isoAt(time.Now().UTC().Add(-olderThan))
	var gone int64
	err := inTx(ctx, db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			"DELETE FROM login_attempts WHERE window_start <= ?", cutoff)
		if err != nil {
			return err
		}
		gone, _ = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("purge rate limits: %w", err)
	}
	return gone, nil
}
