package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// `LookupTouching` -- the one session read a serving request makes, and the
// only place in the module that both decides who somebody is and writes to
// the table while doing it.
//
// It is worth its own file because **every branch here is a security answer
// wearing a maintenance costume.** The row is deleted when it expires, so a
// stolen cookie stops working without waiting for a purge that nothing
// schedules. A row whose timestamps will not parse must refuse rather than be
// read optimistically -- `expires_at` is the whole of the expiry check, and a
// reader that shrugged at an unparseable one would be honouring a session
// with no end. And the touch is rate-limited to `TouchInterval` so that
// signing in does not turn every request into a write, which is a
// availability property rather than a security one and is the reason the
// branch is easy to get subtly wrong.
//
// The door's tests drive this through HTTP and prove the outcome; these prove
// the row, which is where the behaviour actually is.

// sessionRow reads back what the table holds for one token.
func sessionRow(t *testing.T, db *sql.DB, token string) (lastSeen sql.NullString, present bool) {
	t.Helper()
	err := db.QueryRow("SELECT last_seen_at FROM sessions WHERE token_hash = ?",
		HashToken(token)).Scan(&lastSeen)
	if err == sql.ErrNoRows {
		return lastSeen, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return lastSeen, true
}

// No cookie is nobody, and it must not become a query -- an anonymous request
// is the common case on a public page, and one that hit the database per
// visitor would be a table scan per bot.
func TestAnEmptyTokenIsNobodyAndIsNotLookedUp(t *testing.T) {
	t.Parallel()
	writer, _ := fixtureDB(t)
	// Closed, so any query at all is a failure rather than a slow success.
	closed, err := sql.Open("sqlite", "file:/nonexistent/nope.db?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()

	session, err := LookupTouching(context.Background(), closed, "")
	if err != nil || session != nil {
		t.Fatalf("an empty token gave %v, %v -- it reached the database", session, err)
	}
	// ...and a token that is merely unknown is also nobody, with no error.
	session, err = LookupTouching(context.Background(), writer, "not-a-session-token")
	if err != nil || session != nil {
		t.Fatalf("an unknown token gave %v, %v", session, err)
	}
}

// An expired session is **deleted** on the way past, not merely refused. The
// refusal is the security answer; the delete is what keeps a table of dead
// rows from growing on an instance with no cleanup job.
func TestAnExpiredSessionIsRefusedAndTheRowIsGone(t *testing.T) {
	t.Parallel()
	writer, _ := fixtureDB(t)
	const token = "expired-session-token"
	mint(t, writer, token, 1, time.Now().Add(-time.Minute))
	if _, present := sessionRow(t, writer, token); !present {
		t.Fatal("the fixture did not write a row")
	}

	session, err := LookupTouching(context.Background(), writer, token)
	if err != nil {
		t.Fatalf("an expired session raised: %v", err)
	}
	if session != nil {
		t.Fatal("an expired session was honoured")
	}
	if _, present := sessionRow(t, writer, token); present {
		t.Error("the expired row is still there; nothing else deletes it")
	}
	// A second look is the same answer, from an empty table.
	if session, err := LookupTouching(context.Background(), writer, token); err != nil || session != nil {
		t.Fatalf("the second look gave %v, %v", session, err)
	}
}

// A session that expires **exactly now** is expired. `After` and not
// `!Before`: the boundary belongs to the past, so a clock that lands on it
// refuses rather than granting one more request.
func TestASessionExpiringOnTheInstantIsAlreadyExpired(t *testing.T) {
	t.Parallel()
	writer, _ := fixtureDB(t)
	const token = "on-the-boundary-token"
	// A whole second in the past, rendered without sub-second digits, so the
	// comparison cannot be decided by a rounding this test did not intend.
	mint(t, writer, token, 1, time.Now().Add(-time.Second).Truncate(time.Second))
	if session, err := LookupTouching(context.Background(), writer, token); err != nil || session != nil {
		t.Fatalf("a session one second past its end gave %v, %v", session, err)
	}
}

// A row whose timestamps will not parse is a **fault**, not a session. The
// alternative -- reading past it -- is a session with no expiry that no
// amount of waiting ends.
func TestARowWithUnreadableTimestampsIsRefusedRatherThanRead(t *testing.T) {
	t.Parallel()
	writer, _ := fixtureDB(t)
	for _, row := range []struct{ token, created, expires string }{
		{"bad-expiry", isoformat(time.Now()), "not a timestamp"},
		{"empty-expiry", isoformat(time.Now()), ""},
		{"bad-creation", "not a timestamp", isoformat(time.Now().Add(time.Hour))},
	} {
		if _, err := writer.Exec(
			"INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at)"+
				" VALUES (?, 1, ?, ?, ?)",
			HashToken(row.token), row.created, row.expires, isoformat(time.Now())); err != nil {
			t.Fatal(err)
		}
		session, err := LookupTouching(context.Background(), writer, row.token)
		if err == nil {
			t.Errorf("%s: a row with %q/%q answered %v rather than raising",
				row.token, row.created, row.expires, session)
		}
		if session != nil {
			t.Errorf("%s: a session came back beside the error: %+v", row.token, session)
		}
	}
}

// The touch is rate-limited: a session seen a moment ago is not rewritten,
// and one seen longer ago than `TouchInterval` is. The distinction is the
// difference between one write per session and one write per request.
func TestTheTouchIsWrittenOnlyWhenTheIntervalHasPassed(t *testing.T) {
	t.Parallel()
	writer, _ := fixtureDB(t)
	setSeen := func(token string, seen any) {
		t.Helper()
		if _, err := writer.Exec(
			"UPDATE sessions SET last_seen_at = ? WHERE token_hash = ?", seen, HashToken(token)); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		name  string
		seen  any
		touch bool
	}{
		{"seen just now", isoformat(time.Now()), false},
		{"seen inside the interval", isoformat(time.Now().Add(-TouchInterval / 2)), false},
		{"seen past the interval", isoformat(time.Now().Add(-TouchInterval - time.Minute)), true},
		{"never seen", nil, true},
		{"seen at nothing", "", true},
		// The recovery the doc comment names: a value nothing can parse is
		// rewritten rather than left to be unparseable forever.
		{"seen at gibberish", "not a timestamp", true},
	} {
		token := "touch-" + row.name
		mint(t, writer, token, 1, time.Now().Add(time.Hour))
		setSeen(token, row.seen)
		before, _ := sessionRow(t, writer, token)

		session, err := LookupTouching(context.Background(), writer, token)
		if err != nil {
			t.Errorf("%s: %v", row.name, err)
			continue
		}
		if session == nil || session.UserID != 1 {
			t.Errorf("%s: a live session came back as %+v", row.name, session)
			continue
		}
		after, present := sessionRow(t, writer, token)
		if !present {
			t.Errorf("%s: the row went", row.name)
			continue
		}
		touched := after.Valid && after.String != before.String
		if touched != row.touch {
			t.Errorf("%s: last_seen_at went %v -> %v (touched=%v), want touched=%v",
				row.name, before, after, touched, row.touch)
		}
		// Whatever it decided, the row must now hold something readable --
		// otherwise the "unparseable is rewritten" recovery is a no-op that
		// simply writes another unparseable value.
		if row.touch {
			if _, err := ParseTimestamp(after.String); err != nil {
				t.Errorf("%s: the touch wrote %q, which does not parse: %v",
					row.name, after.String, err)
			}
		}
	}
}

// A live session comes back with the times the row holds, so a caller can
// tell how old it is -- and the two are the row's, not the clock's.
func TestALiveSessionCarriesTheRowsOwnTimes(t *testing.T) {
	t.Parallel()
	writer, _ := fixtureDB(t)
	const token = "live-session-token"
	expires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	mint(t, writer, token, 2, expires)

	session, err := LookupTouching(context.Background(), writer, token)
	if err != nil {
		t.Fatal(err)
	}
	if session == nil {
		t.Fatal("a live session was not found")
	}
	if session.UserID != 2 {
		t.Errorf("user %d, want 2", session.UserID)
	}
	if !session.ExpiresAt.Equal(expires) {
		t.Errorf("expires %v, the row says %v", session.ExpiresAt, expires)
	}
	if session.CreatedAt.IsZero() || session.CreatedAt.After(time.Now().Add(time.Minute)) {
		t.Errorf("created %v is not the row's", session.CreatedAt)
	}
}
