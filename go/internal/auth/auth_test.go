package auth

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// Vectors Python wrote (2026-08-21, `.venv` argon2-cffi 25.1.0, CPython
// 3.12), with the commands that produce them so they can be regenerated:
//
//	from argon2 import PasswordHasher
//	PasswordHasher(time_cost=2, memory_cost=19456, parallelism=1).hash(pw)
//
//	import hashlib; hashlib.sha256(token.encode()).hexdigest()
//
// These are the fixture passwords `tests/contract/harness.py` seeds; the
// hashes are of known strings and are not secrets.
const (
	alicePassword = "correct-horse-battery-staple"
	aliceHash     = "$argon2id$v=19$m=19456,t=2,p=1$fzytJABNZ+uVVawoDerNkQ$r7JnoQumok+cn4Jr9hWi8WqNSWV5kpxT6n5y8j044iY"
	bobPassword   = "a-different-long-passphrase"
	bobHash       = "$argon2id$v=19$m=19456,t=2,p=1$uFbjsIOh9O/qVtgX5lZHYA$/zCCj7IIw2IxHgjYEHpEaf7BC7gDvjaDsi0hVFsHxKE"

	fixtureToken     = "fixture-token-AAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	fixtureTokenHash = "25db52bb467767ed987f3a5d32af58aaf9e632df182f26b4e23014977d248bd7"
)

// appSchema is `app.db` as Python's migration ladder leaves it.
//
// It was a hand-copied transcription of the ladder's *first* rung until
// 2026-08-22, which was fine while the door only read `users.id`, `username`,
// `is_admin` and `disabled_at` -- and stopped being fine the moment the
// accounts flip needed `model_tier`, a column rung 10 adds. `authtest`'s
// package comment records what that cost and why the bytes are generated now.
func appSchema(t *testing.T) string {
	t.Helper()
	return authtest.Schema()
}

// isoformat is what `datetime.now(UTC).isoformat()` writes.
func isoformat(t time.Time) string {
	t = t.UTC()
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}

// fixtureDB writes an app.db the way Python would have (WAL, the v1 tables),
// with alice (admin), bob, and a disabled account, and returns a read-only
// handle the way the door opens one.
func fixtureDB(t *testing.T) (*sql.DB, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	writer, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { writer.Close() })
	if _, err := writer.Exec(appSchema(t)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, row := range []struct {
		id       int64
		name     string
		hash     string
		admin    int
		disabled string
	}{
		{1, "alice", aliceHash, 1, ""},
		{2, "bob", bobHash, 0, ""},
		{3, "mallory", bobHash, 0, isoformat(now)},
	} {
		var disabled any
		if row.disabled != "" {
			disabled = row.disabled
		}
		if _, err := writer.Exec(
			"INSERT INTO users (id, username, password_hash, is_admin, disabled_at, created_at) VALUES (?, ?, ?, ?, ?, ?)",
			row.id, row.name, row.hash, row.admin, disabled, isoformat(now)); err != nil {
			t.Fatal(err)
		}
	}
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	return writer, reader
}

func mint(t *testing.T, writer *sql.DB, token string, userID int64, expires time.Time) {
	t.Helper()
	if _, err := writer.Exec(
		"INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?, ?)",
		HashToken(token), userID, isoformat(time.Now()), isoformat(expires), isoformat(time.Now())); err != nil {
		t.Fatal(err)
	}
}

func TestTokenHashMatchesPython(t *testing.T) {
	if got := HashToken(fixtureToken); got != fixtureTokenHash {
		t.Fatalf("HashToken = %s, Python wrote %s", got, fixtureTokenHash)
	}
}

func TestParsesWhatIsoformatWrites(t *testing.T) {
	for _, s := range []string{
		"2026-08-21T12:00:00.123456+00:00",
		"2026-08-21T12:00:00+00:00",
		"2026-08-21T12:00:00.123456", // naive; read as UTC
	} {
		got, err := ParseTimestamp(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if got.UTC().Year() != 2026 || got.UTC().Hour() != 12 {
			t.Fatalf("%s parsed as %v", s, got)
		}
	}
	if _, err := ParseTimestamp("yesterday"); err == nil {
		t.Fatal("a non-timestamp parsed")
	}
}

func TestResolvesASessionWrittenInPythonsShape(t *testing.T) {
	ctx := context.Background()
	writer, reader := fixtureDB(t)
	mint(t, writer, fixtureToken, 1, time.Now().Add(Lifetime))

	scope, err := Resolve(ctx, reader, fixtureToken)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Authenticated || scope.Username != "alice" || !scope.IsAdmin || scope.UserID != 1 {
		t.Fatalf("alice's session resolved to %+v", scope)
	}

	// bob is not an admin.
	mint(t, writer, "bob-token", 2, time.Now().Add(Lifetime))
	scope, err = Resolve(ctx, reader, "bob-token")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Authenticated || scope.IsAdmin || scope.Username != "bob" {
		t.Fatalf("bob's session resolved to %+v", scope)
	}
}

// A resolved session carries the account's model tier, because that is a fact
// about the caller a handler is allowed to know and must read fresh per
// request -- `api/deps.py` puts `model_tier` on `UserScope` for exactly the
// reason it puts `is_admin` there.
//
// This test exists because a mutation survived without it: the field can be
// declared on Scope, set by a handler's own test fixture, and never once
// copied out of the user row, with every route test still green. The first
// Claude route is what reads it, and it reads it from here.
func TestAResolvedSessionCarriesTheAccountsModelTier(t *testing.T) {
	ctx := context.Background()
	writer, reader := fixtureDB(t)
	if _, err := writer.Exec("UPDATE users SET model_tier = ? WHERE id = ?", "opus", 2); err != nil {
		t.Fatal(err)
	}
	mint(t, writer, "bob-tiered", 2, time.Now().Add(Lifetime))
	scope, err := Resolve(ctx, reader, "bob-tiered")
	if err != nil {
		t.Fatal(err)
	}
	if scope.ModelTier != "opus" {
		t.Errorf("bob's seat resolved to tier %q, want opus", scope.ModelTier)
	}
	// NULL is the ordinary case and must stay empty rather than becoming the
	// default's key: which tier is the default may change, and a row that
	// never asked for one must not be rewritten by having been read.
	mint(t, writer, "alice-house", 1, time.Now().Add(Lifetime))
	scope, err = Resolve(ctx, reader, "alice-house")
	if err != nil {
		t.Fatal(err)
	}
	if scope.ModelTier != "" {
		t.Errorf("an account with no tier resolved to %q, want empty", scope.ModelTier)
	}
}

func TestAnUnknownExpiredOrDisabledSessionIsAnonymous(t *testing.T) {
	ctx := context.Background()
	writer, reader := fixtureDB(t)
	mint(t, writer, "expired", 1, time.Now().Add(-time.Minute))
	mint(t, writer, "disabled-account", 3, time.Now().Add(Lifetime))
	for _, token := range []string{"", "never-minted", "expired", "disabled-account"} {
		scope, err := Resolve(ctx, reader, token)
		if err != nil {
			t.Fatalf("%q: %v", token, err)
		}
		if scope != Anonymous {
			t.Fatalf("%q resolved to %+v, want anonymous", token, scope)
		}
	}
}

func TestTheReaderCannotWrite(t *testing.T) {
	_, reader := fixtureDB(t)
	if _, err := reader.Exec("DELETE FROM sessions"); err == nil {
		t.Fatal("the door's handle wrote to app.db; it must be read-only")
	}
}

func TestOpenDoesNotCreateAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Ping(context.Background(), db); err == nil {
		t.Fatal("Ping succeeded against a file that does not exist")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Open created app.db; Python owns that file")
	}
}

func TestVerifiesPythonsArgon2Hashes(t *testing.T) {
	for _, v := range []struct{ pw, hash string }{{alicePassword, aliceHash}, {bobPassword, bobHash}} {
		h := v.hash
		if !Verify(&h, v.pw) {
			t.Fatalf("a hash argon2-cffi wrote did not verify: %s", v.hash)
		}
		if Verify(&h, v.pw+"x") {
			t.Fatal("a wrong password verified")
		}
	}
	if NeedsRehash(aliceHash) {
		t.Fatal("Python's hash at the pinned parameters reads as needing a rehash")
	}
}

func TestHashesInTheShapePythonReads(t *testing.T) {
	h, err := HashPassword(alicePassword)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash is not in argon2-cffi's PHC form: %s", h)
	}
	if !Verify(&h, alicePassword) {
		t.Fatal("round trip failed")
	}
	if NeedsRehash(h) {
		t.Fatal("a fresh hash needs a rehash")
	}
	weak := "$argon2id$v=19$m=4096,t=1,p=1$fzytJABNZ+uVVawoDerNkQ$r7JnoQumok+cn4Jr9hWi8WqNSWV5kpxT6n5y8j044iY"
	if !NeedsRehash(weak) {
		t.Fatal("a weaker hash does not need a rehash")
	}
}

func TestEveryFailureIsFalse(t *testing.T) {
	for _, h := range []string{"", "not-a-hash", "$argon2i$v=19$m=19456,t=2,p=1$abc$def", "$2b$12$bcrypt"} {
		h := h
		if Verify(&h, alicePassword) {
			t.Fatalf("%q verified", h)
		}
	}
	if Verify(nil, alicePassword) {
		t.Fatal("an account with no password verified")
	}
}

func TestStrengthFloorMatchesPython(t *testing.T) {
	if err := CheckStrength("short"); err == nil {
		t.Fatal("an 11-character password was accepted")
	}
	if err := CheckStrength(strings.Repeat("a", MinPasswordLength)); err != nil {
		t.Fatal(err)
	}
	if err := CheckStrength(strings.Repeat("a", MaxPasswordBytes+1)); err == nil {
		t.Fatal("a 1025-byte password was accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("HashPassword stored a weak password")
	}
}
