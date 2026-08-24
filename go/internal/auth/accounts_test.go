package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

// The write side of `mtglab/auth`: accounts, tokens, the rate limiter and the
// mail seam.
//
// Every test here builds a real `app.db` from the recorded schema
// (`authtest/app_schema.sql`) rather than from a table written by
// hand, for the reason `appSchema` gives.
//
// **No test in this file sends mail.** That is ADR 16's seam doing its job and
// it is checked rather than asserted in prose: `recordingSender` is what every
// invite and reset is handed, and the one test that exercises `ResendSender`
// injects a transport, so the request is built and inspected without ever
// reaching a socket.

const longEnough = "a passphrase long enough"

// newAccountsDB builds a scratch `app.db` and opens it the way the door will.
//
// The file is created here with `rwc`, which production deliberately cannot do
// -- `OpenReadWrite` is `mode=rw` and `TestOpenDoesNotCreateAMissingFile` pins
// that the read side will not create one either. A test standing in for
// the ladder is the one caller entitled to make the file.
func newAccountsDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.db")
	seed, err := sql.Open("sqlite", "file:"+path+"?mode=rwc&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(appSchema(t)); err != nil {
		t.Fatalf("building the scratch app.db: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := PingWritable(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

// claim gives an account a password, which is what makes it a *usable* admin
// and therefore what the LastAdmin guard counts.
func claim(t *testing.T, db *sql.DB, user *User) {
	t.Helper()
	if _, err := SetPassword(context.Background(), db, user.ID, longEnough); err != nil {
		t.Fatalf("claiming %s: %v", user.Username, err)
	}
}

func mustCreate(t *testing.T, db *sql.DB, name, email string, admin bool) *User {
	t.Helper()
	user, err := Create(context.Background(), db, name, email, admin)
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return user
}

// ---- accounts --------------------------------------------------------------

func TestAnAccountIsCreatedUnclaimedAndFoundEveryWay(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	created := mustCreate(t, db, "  Ada  ", " Ada@Example.COM ", true)

	if created.Username != "Ada" {
		t.Errorf("the username was not trimmed: %q", created.Username)
	}
	if created.Email != "ada@example.com" || !created.HasEmail {
		t.Errorf("the address was not lowercased: %q", created.Email)
	}
	// ADR 16: an invite is an *unclaimed* account, never a disabled one.
	if created.Disabled {
		t.Error("a fresh account is disabled; `disabled_at` is the revocation lever")
	}
	if has, err := HasPassword(ctx, db, created.ID); err != nil || has {
		t.Errorf("a fresh account has a password (%v, %v)", has, err)
	}

	// COLLATE NOCASE on the column, so the handle is case-insensitive.
	for _, name := range []string{"Ada", "ada", "ADA", " ada "} {
		found, err := Get(ctx, db, name)
		if err != nil || found == nil || found.ID != created.ID {
			t.Errorf("Get(%q) did not find the account (%v)", name, err)
		}
	}
	byEmail, err := GetByEmail(ctx, db, "ADA@example.com")
	if err != nil || byEmail == nil || byEmail.ID != created.ID {
		t.Errorf("GetByEmail did not find the account (%v)", err)
	}
	if missing, err := Get(ctx, db, "nobody"); err != nil || missing != nil {
		t.Errorf("Get for a name nobody holds returned %v (%v)", missing, err)
	}
}

func TestBothUniqueColumnsSayWhichOneCollided(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	mustCreate(t, db, "ada", "ada@example.com", false)

	_, err := Create(ctx, db, "ada", "other@example.com", false)
	if !errors.Is(err, ErrUserExists) || !strings.Contains(err.Error(), "username") {
		t.Errorf("a taken username said %v", err)
	}
	_, err = Create(ctx, db, "grace", "ADA@example.com", false)
	if !errors.Is(err, ErrUserExists) || !strings.Contains(err.Error(), "email address") {
		t.Errorf("a taken address said %v", err)
	}
}

func TestNormalisationRefusesTheRecordedSet(t *testing.T) {
	for _, bad := range []string{"", "a", "1", " ", "has space", "-leading",
		"way-too-long-a-username-for-any-of-this", "emoji🜁"} {
		if _, err := NormaliseUsername(bad); !errors.Is(err, ErrInvalidUsername) {
			t.Errorf("NormaliseUsername(%q) = %v, want a refusal", bad, err)
		}
	}
	for _, good := range []string{"ab", "a.b_c-d", "0a", strings.Repeat("a", 32)} {
		if _, err := NormaliseUsername(good); err != nil {
			t.Errorf("NormaliseUsername(%q): %v", good, err)
		}
	}
	// An absent address is not an error -- the maintainer's bootstrap account
	// can exist without one.
	for _, blank := range []string{"", "   "} {
		if got, err := NormaliseEmail(blank); err != nil || got != "" {
			t.Errorf("NormaliseEmail(%q) = %q, %v", blank, got, err)
		}
	}
	for _, bad := range []string{"nope", "a@b", "a@.com", "a@b.", "@b.com",
		"a b@c.com", "a@b@c.com", strings.Repeat("a", 250) + "@example.com"} {
		if _, err := NormaliseEmail(bad); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("NormaliseEmail(%q) = %v, want a refusal", bad, err)
		}
	}
	// The message quotes a bounded slice, never the caller's whole input.
	_, err := NormaliseEmail(strings.Repeat("x", 5000))
	if err == nil || len(err.Error()) > MaxEmail+80 {
		t.Errorf("an oversized address produced a %d-character message", len(err.Error()))
	}
}

// CLAUDE.md rule 5 and ADR 17: an address may be serialised only into a
// response an admin authenticated for, so `AsDict` withholds it unless asked.
func TestTheAddressIsWithheldUnlessAsked(t *testing.T) {
	db := newAccountsDB(t)
	user := mustCreate(t, db, "ada", "ada@example.com", false)

	plain := user.AsDict(false)
	if _, leaked := plain["email"]; leaked {
		t.Fatal("AsDict serialised the address without being asked")
	}
	for _, key := range []string{"id", "username", "is_admin", "disabled",
		"created_at", "model_tier"} {
		if _, ok := plain[key]; !ok {
			t.Errorf("AsDict dropped %q", key)
		}
	}
	if plain["model_tier"] != "sonnet" {
		t.Errorf("an account nobody granted anything reports %v", plain["model_tier"])
	}
	if got := user.AsDict(true)["email"]; got != "ada@example.com" {
		t.Errorf("an admin's view of the address is %v", got)
	}
	// An account with no address serialises null, not the empty string -- the
	// admin page tells "has none" from "has one" on exactly that.
	none := mustCreate(t, db, "grace", "", false)
	if got, ok := none.AsDict(true)["email"]; !ok || got != nil {
		t.Errorf("an account with no address serialised %#v", got)
	}
}

// ---- authenticate ----------------------------------------------------------

func TestAuthenticateRefusesEveryWayTheSame(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "ada@example.com", false)
	claim(t, db, ada)
	mustCreate(t, db, "unclaimed", "u@example.com", false)
	disabled := mustCreate(t, db, "disabled", "d@example.com", false)
	claim(t, db, disabled)
	if _, err := SetDisabled(ctx, db, disabled.ID, true); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ name, username, password string }{
		{"an unknown account", "nobody", longEnough},
		{"a wrong password", "ada", "wrong-but-long-enough"},
		{"an unclaimed invite", "unclaimed", longEnough},
		{"a disabled account", "disabled", longEnough},
	} {
		user, err := Authenticate(ctx, db, c.username, c.password)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if user != nil {
			t.Errorf("%s authenticated", c.name)
		}
	}

	user, err := Authenticate(ctx, db, "  ADA ", longEnough)
	if err != nil || user == nil || user.ID != ada.ID {
		t.Fatalf("the right password did not authenticate: %v, %v", user, err)
	}
}

// The hash is upgraded when the plaintext is in hand -- the only moment it can
// be, and the reason `Authenticate` takes a writable handle.
func TestAWeakerHashIsUpgradedAtLogin(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "", false)

	// A real hash of the real password at *half* the memory cost -- valid
	// Argon2id, below this build's profile, and exactly the row a deploy that
	// raised the parameters leaves behind. Computed rather than pasted,
	// because a pasted one would be a hash of something nobody can check.
	const weakMemory = MemoryCostKiB / 2
	salt := []byte("0123456789abcdef")
	weak := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, weakMemory, TimeCost, Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(argon2.IDKey([]byte(longEnough),
			salt, TimeCost, weakMemory, Parallelism, keyLength)))
	if h := weak; !Verify(&h, longEnough) {
		t.Fatal("the weak fixture hash is not a hash of the password")
	}
	if !NeedsRehash(weak) {
		t.Fatal("a hash below the profile did not read as stale")
	}
	if _, err := db.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE id = ?",
		weak, ada.ID); err != nil {
		t.Fatal(err)
	}

	user, err := Authenticate(ctx, db, "ada", longEnough)
	if err != nil || user == nil {
		t.Fatalf("the right password did not authenticate: %v", err)
	}
	var stored string
	if err := db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?",
		ada.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == weak || NeedsRehash(stored) {
		t.Error("the stale hash was not upgraded at the one moment it could be")
	}
	if h := stored; !Verify(&h, longEnough) {
		t.Error("the upgraded hash is not a hash of the same password")
	}
}

// ---- the last admin --------------------------------------------------------

// ADR 17, and the subtle half: "usable" means enabled *and* holding a
// password. An instance whose only remaining admin is an unclaimed invite is
// locked out exactly as thoroughly as one with no admin at all.
func TestTheLastAdminWhoCanSignInIsRefusedEveryWay(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "ada@example.com", true)
	claim(t, db, ada)
	// A second admin who has never claimed their invite. It must not count.
	mustCreate(t, db, "ghost", "ghost@example.com", true)

	if ids, err := UsableAdminIDs(ctx, db); err != nil || len(ids) != 1 || !ids[ada.ID] {
		t.Fatalf("usable admins = %v (%v); an unclaimed invite was counted", ids, err)
	}
	if err := SetAdmin(ctx, db, ada.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("demoting the last usable admin said %v", err)
	}
	if _, err := SetDisabled(ctx, db, ada.ID, true); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("disabling the last usable admin said %v", err)
	}
	if _, err := Delete(ctx, db, ada.ID); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("deleting the last usable admin said %v", err)
	}
	// The sentence a route answers verbatim, so the sentinel's own words must
	// not be in front of it.
	err := SetAdmin(ctx, db, ada.ID, false)
	if !strings.HasPrefix(err.Error(), "refusing to revoke admin:") {
		t.Errorf("the refusal reads %q; a route answers this verbatim", err)
	}

	// Claim the ghost's invite and all three become allowed.
	ghost, _ := Get(ctx, db, "ghost")
	claim(t, db, ghost)
	if err := SetAdmin(ctx, db, ada.ID, false); err != nil {
		t.Errorf("demoting one of two usable admins: %v", err)
	}
}

// An operation on an account that is not a usable admin changes the count by
// nothing and is always allowed -- including when the count is already zero,
// where refusing would mean an instance with no admin could never disable
// anybody.
func TestANonAdminIsNeverRefusedByTheGuard(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	bob := mustCreate(t, db, "bob", "", false)
	claim(t, db, bob)
	if _, err := SetDisabled(ctx, db, bob.ID, true); err != nil {
		t.Errorf("disabling a non-admin with no admins on the instance: %v", err)
	}
	if _, err := Delete(ctx, db, bob.ID); err != nil {
		t.Errorf("deleting a non-admin with no admins on the instance: %v", err)
	}
}

func TestDisablingRevokesAndDeletingCascades(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "", false)
	claim(t, db, ada)
	for i := 0; i < 3; i++ {
		if _, err := CreateSession(ctx, db, ada.ID); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := CountSessionsForUser(ctx, db, ada.ID); err != nil || n != 3 {
		t.Fatalf("session count = %d (%v)", n, err)
	}

	// Disabling revokes: an account that can no longer log in but whose cookie
	// still works has not been disabled in any sense the person doing it meant.
	revoked, err := SetDisabled(ctx, db, ada.ID, true)
	if err != nil || revoked != 3 {
		t.Fatalf("disabling revoked %d sessions (%v)", revoked, err)
	}
	// Re-enabling revokes nothing and is not a second disable.
	if revoked, err = SetDisabled(ctx, db, ada.ID, false); err != nil || revoked != 0 {
		t.Fatalf("re-enabling revoked %d (%v)", revoked, err)
	}
	if again, _ := Get(ctx, db, "ada"); again.Disabled {
		t.Fatal("the account is still disabled after being re-enabled")
	}

	token, err := IssueToken(ctx, db, ada.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSession(ctx, db, ada.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(ctx, db, ada.ID); err != nil {
		t.Fatal(err)
	}
	// ON DELETE CASCADE, which is only more than a comment because the DSN
	// turns `foreign_keys` on.
	if _, err := LookupToken(ctx, db, token, ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("a deleted account's token survived: %v", err)
	}
	var live int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM sessions WHERE user_id = ?", ada.ID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Errorf("%d sessions outlived the account they belonged to", live)
	}
	if _, err := Delete(ctx, db, ada.ID); !errors.Is(err, ErrNoSuchUser) {
		t.Errorf("deleting a gone account said %v", err)
	}
}

func TestRenamingKeepsSessionsAndSettingAPasswordDoesNot(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "", false)
	claim(t, db, ada)
	if _, err := CreateSession(ctx, db, ada.ID); err != nil {
		t.Fatal(err)
	}

	// A username is an identifier, not a credential: changing your handle
	// should not sign you out of your other browser.
	if _, err := SetUsername(ctx, db, ada.ID, "ada.lovelace"); err != nil {
		t.Fatal(err)
	}
	if n, err := CountSessionsForUser(ctx, db, ada.ID); err != nil || n != 1 {
		t.Errorf("a rename ended %d sessions (%v)", 1-n, err)
	}
	// A password change is the opposite, and one transaction: there is no
	// window in which the password has moved and the old cookies still work.
	if revoked, err := SetPassword(ctx, db, ada.ID, "another long passphrase"); err != nil || revoked != 1 {
		t.Errorf("a password change revoked %d sessions (%v)", revoked, err)
	}

	mustCreate(t, db, "grace", "", false)
	if _, err := SetUsername(ctx, db, ada.ID, "GRACE"); !errors.Is(err, ErrUserExists) {
		t.Errorf("renaming onto a taken handle said %v", err)
	}
	// ...but a change of capitalisation does not collide with itself.
	if _, err := SetUsername(ctx, db, ada.ID, "Ada.Lovelace"); err != nil {
		t.Errorf("recapitalising my own handle: %v", err)
	}
}

func TestTheModelTierIsWrittenStrictlyAndReadLoosely(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "", false)

	if err := SetModelTier(ctx, db, ada.ID, "nope"); !errors.Is(err, ErrUnknownTier) {
		t.Errorf("writing an unknown tier said %v", err)
	}
	if err := SetModelTier(ctx, db, ada.ID, "opus"); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(ctx, db, "ada"); got.ModelTier != "opus" {
		t.Errorf("the tier stored as %q", got.ModelTier)
	}
	// The default is stored as NULL, so changing which tier is the default
	// never means rewriting rows.
	if err := SetModelTier(ctx, db, ada.ID, "sonnet"); err != nil {
		t.Fatal(err)
	}
	var stored sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT model_tier FROM users WHERE id = ?",
		ada.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored.Valid {
		t.Errorf("the default tier was stored as %q rather than NULL", stored.String)
	}
	// The read path tolerates what the write path refuses: a stale key
	// resolves to the default rather than erroring, which is what makes a
	// rolled-back deploy survivable.
	if _, err := db.ExecContext(ctx, "UPDATE users SET model_tier = 'retired' WHERE id = ?",
		ada.ID); err != nil {
		t.Fatal(err)
	}
	stale, _ := Get(ctx, db, "ada")
	if got := stale.AsDict(false)["model_tier"]; got != "sonnet" {
		t.Errorf("a stale tier serialised as %v rather than the default", got)
	}
}

// ---- tokens ----------------------------------------------------------------

func TestATokenIsSingleUseAndSetsThePassword(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "ada@example.com", false)
	if _, err := CreateSession(ctx, db, ada.ID); err != nil {
		t.Fatal(err)
	}

	token, err := IssueToken(ctx, db, ada.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 {
		t.Errorf("the token is %d characters", len(token))
	}
	// Stored hashed: reading `app.db` must not hand over live links.
	var stored string
	if err := db.QueryRowContext(ctx, "SELECT token_hash FROM auth_tokens").
		Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || stored != HashToken(token) {
		t.Fatal("the token is not stored as its SHA-256")
	}
	if outstanding, err := TokenOutstanding(ctx, db, ada.ID, PurposeInvite); err != nil || !outstanding {
		t.Errorf("a fresh invite is not outstanding (%v)", err)
	}

	user, err := RedeemToken(ctx, db, token, longEnough, PurposeInvite, "ada.lovelace")
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "ada.lovelace" {
		t.Errorf("the rename did not land: %q", user.Username)
	}
	if has, _ := HasPassword(ctx, db, ada.ID); !has {
		t.Error("redeeming did not set a password")
	}
	// Redeeming revokes every session for that account (ADR 16).
	if n, _ := CountSessionsForUser(ctx, db, ada.ID); n != 0 {
		t.Errorf("%d sessions survived a redemption", n)
	}
	// Single use, and the second click says so rather than "invalid".
	_, err = RedeemToken(ctx, db, token, longEnough, PurposeInvite, "")
	if !errors.Is(err, ErrTokenUsed) {
		t.Errorf("a second redemption said %v", err)
	}
}

func TestATokenRefusesForEveryRecordedReason(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "ada@example.com", false)

	if _, err := LookupToken(ctx, db, "", ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("an empty token said %v", err)
	}
	if _, err := LookupToken(ctx, db, "never-minted", ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("an unknown token said %v", err)
	}

	// A reset asked to act as an invite gets the *same sentence* an unknown
	// token gets: a caller holding one and poking the other learns nothing.
	reset, err := IssueToken(ctx, db, ada.ID, PurposeReset)
	if err != nil {
		t.Fatal(err)
	}
	_, err = LookupToken(ctx, db, reset, PurposeInvite)
	if !errors.Is(err, ErrTokenInvalid) || err.Error() != "that link is not valid" {
		t.Errorf("a purpose mismatch said %q (%v)", err, err)
	}

	// A reset may not rename, and that refusal is *not* a token error: the
	// link is good and the client asked the wrong thing, so nothing is spent.
	_, err = RedeemToken(ctx, db, reset, longEnough, "", "somebody.else")
	if !errors.Is(err, ErrWrongPurpose) || IsTokenError(err) {
		t.Errorf("renaming from a reset said %v (token error: %v)", err, IsTokenError(err))
	}
	if outstanding, _ := TokenOutstanding(ctx, db, ada.ID, PurposeReset); !outstanding {
		t.Error("a refused rename spent the link")
	}

	// Expiry.
	expired, err := issueToken(ctx, db, ada.ID, PurposeInvite, -time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LookupToken(ctx, db, expired, ""); !errors.Is(err, ErrTokenExpired) {
		t.Errorf("an expired token said %v", err)
	}

	// A disabled account's link is dead, and gets the sentence that explains
	// nothing: disabling is the maintainer's lever and a reset must not undo
	// it.
	claim(t, db, ada)
	if _, err := SetDisabled(ctx, db, ada.ID, true); err != nil {
		t.Fatal(err)
	}
	live, err := IssueToken(ctx, db, ada.ID, PurposeReset)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RedeemToken(ctx, db, live, longEnough, "", ""); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("a disabled account's reset said %v", err)
	}
	after, _ := Get(ctx, db, "ada")
	if !after.Disabled {
		t.Fatal("redeeming re-enabled a disabled account")
	}
}

// The transaction's whole reason for being: a name that collides must leave a
// **retryable** invite rather than a spent link and an account nobody can get
// into.
func TestATakenNameRollsTheTokenSpendBack(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	mustCreate(t, db, "grace", "", false)
	ada := mustCreate(t, db, "ada", "ada@example.com", false)
	token, err := IssueToken(ctx, db, ada.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := RedeemToken(ctx, db, token, longEnough, PurposeInvite, "GRACE"); !errors.Is(err, ErrUserExists) {
		t.Fatalf("claiming a taken handle said %v", err)
	}
	if has, _ := HasPassword(ctx, db, ada.ID); has {
		t.Error("the password was written even though the rename failed")
	}
	// The link still works, which is the property being bought.
	user, err := RedeemToken(ctx, db, token, longEnough, PurposeInvite, "ada.lovelace")
	if err != nil {
		t.Fatalf("the invite did not survive a refused name: %v", err)
	}
	if user.Username != "ada.lovelace" {
		t.Errorf("the retry landed as %q", user.Username)
	}
}

// One live link per purpose: a re-issued reset replaces the one somebody is
// worried about, and leaves an unclaimed invite alone.
func TestIssuingReplacesOnlyItsOwnPurpose(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "ada@example.com", false)

	invite, err := IssueToken(ctx, db, ada.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	first, err := IssueToken(ctx, db, ada.ID, PurposeReset)
	if err != nil {
		t.Fatal(err)
	}
	second, err := IssueToken(ctx, db, ada.ID, PurposeReset)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LookupToken(ctx, db, first, PurposeReset); !errors.Is(err, ErrTokenInvalid) {
		t.Errorf("the replaced reset still resolves: %v", err)
	}
	if _, err := LookupToken(ctx, db, second, PurposeReset); err != nil {
		t.Errorf("the fresh reset does not resolve: %v", err)
	}
	if _, err := LookupToken(ctx, db, invite, PurposeInvite); err != nil {
		t.Errorf("issuing a reset dropped the outstanding invite: %v", err)
	}
}

func TestPurgingKeepsSpentLinksLongEnoughToExplainThemselves(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "ada@example.com", false)

	spent, err := IssueToken(ctx, db, ada.ID, PurposeInvite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RedeemToken(ctx, db, spent, longEnough, PurposeInvite, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := issueToken(ctx, db, ada.ID, PurposeReset, -time.Hour); err != nil {
		t.Fatal(err)
	}
	gone, err := PurgeExpiredTokens(ctx, db, KeepUsedTokensFor)
	if err != nil || gone != 1 {
		t.Fatalf("the purge took %d rows (%v); it should take the expired one only", gone, err)
	}
	if _, err := LookupToken(ctx, db, spent, ""); !errors.Is(err, ErrTokenUsed) {
		t.Errorf("a spent link stopped saying it was spent: %v", err)
	}
}

// ---- the rate limiter ------------------------------------------------------

func TestTheBudgetIsSpentCountedAndCleared(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	key := AccountKey(" Ada ")
	if key != "user:ada" {
		t.Fatalf("the account key is %q", key)
	}

	for i := 0; i < PerAccount.Failures; i++ {
		if spent, err := Exhausted(ctx, db, key, PerAccount); err != nil || spent {
			t.Fatalf("exhausted after %d of %d (%v)", i, PerAccount.Failures, err)
		}
		if _, err := RecordFailure(ctx, db, key, PerAccount); err != nil {
			t.Fatal(err)
		}
	}
	spent, err := Exhausted(ctx, db, key, PerAccount)
	if err != nil || !spent {
		t.Fatalf("the budget was not spent after %d failures (%v)", PerAccount.Failures, err)
	}
	wait, err := RetryAfter(ctx, db, key, PerAccount)
	if err != nil || wait < 1 || wait > int(PerAccount.Window.Seconds()) {
		t.Errorf("Retry-After = %d (%v)", wait, err)
	}
	// A success clears it: somebody logging in repeatedly and correctly is not
	// what this is for.
	if err := ClearLimit(ctx, db, key); err != nil {
		t.Fatal(err)
	}
	if spent, err := Exhausted(ctx, db, key, PerAccount); err != nil || spent {
		t.Errorf("the budget survived a success (%v)", err)
	}
}

func TestALapsedWindowStartsAgain(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	key := AddressKey("198.51.100.7", "login")

	for i := 0; i < PerAddress.Failures; i++ {
		if _, err := RecordFailure(ctx, db, key, PerAddress); err != nil {
			t.Fatal(err)
		}
	}
	// Age the window past its close, which is what a fixed window forgets.
	stale := isoAt(time.Now().UTC().Add(-PerAddress.Window - time.Minute))
	if _, err := db.ExecContext(ctx,
		"UPDATE login_attempts SET window_start = ? WHERE key = ?", stale, key); err != nil {
		t.Fatal(err)
	}
	if spent, err := Exhausted(ctx, db, key, PerAddress); err != nil || spent {
		t.Errorf("a lapsed window is still exhausted (%v)", err)
	}
	// A window nothing will read again. `older_than` is a day in production;
	// here it is a minute, because the row above was aged by sixteen.
	if gone, err := PurgeStaleLimits(ctx, db, time.Minute); err != nil || gone != 1 {
		t.Errorf("the purge took %d rows (%v)", gone, err)
	}
}

// The scopes are the point: failing to redeem a link must not spend the budget
// somebody needs to log in.
func TestTheKeyspacesDoNotOverlap(t *testing.T) {
	keys := map[string]bool{
		AccountKey("ada"):                           true,
		AddressKey("198.51.100.7", "login"):         true,
		AddressKey("198.51.100.7", "reset"):         true,
		AddressKey("198.51.100.7", "claim"):         true,
		AddressKey("198.51.100.7", "claim-preview"): true,
		EmailKey("ada@example.com"):                 true,
	}
	if len(keys) != 6 {
		t.Fatalf("two of the six budgets share a key: %v", keys)
	}
	// The mailbox is hashed, so a limiter for an address with no account does
	// not accumulate that address.
	mailbox := EmailKey(" ADA@Example.com ")
	if strings.Contains(mailbox, "ada") || strings.Contains(mailbox, "@") {
		t.Errorf("the mailbox key holds the address: %q", mailbox)
	}
	if mailbox != EmailKey("ada@example.com") {
		t.Error("the mailbox key is not normalised before hashing")
	}
	if len(mailbox) != len("mailbox:")+32 {
		t.Errorf("the mailbox key is %q", mailbox)
	}
}

// ---- the mail seam ---------------------------------------------------------

// recordingSender is what every test that would send mail is handed. ADR 16's
// seam exists for exactly this.
type recordingSender struct {
	sent []Message
	err  error
}

func (r *recordingSender) Send(m Message) error {
	if r.err != nil {
		return r.err
	}
	r.sent = append(r.sent, m)
	return nil
}

func TestAnInviteCarriesALinkWithTheTokenInItsFragment(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	ada := mustCreate(t, db, "ada", "ada@example.com", false)
	sender := &recordingSender{}

	if err := SendInvite(ctx, db, ada, sender, "https://example.com/"); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("%d messages went out", len(sender.sent))
	}
	message := sender.sent[0]
	if message.To != "ada@example.com" {
		t.Errorf("the invite went to %q", message.To)
	}
	prefix := "https://example.com/auth/claim#token="
	at := strings.Index(message.Body, prefix)
	if at < 0 {
		t.Fatalf("the invite carries no claim link:\n%s", message.Body)
	}
	// **In the fragment, never the query string.** A fragment reaches no
	// server's access log; the query-string spelling would put a live
	// credential into every hop that serves the page.
	if strings.Contains(message.Body, "claim?token=") {
		t.Error("the token is in a query string")
	}
	token := strings.Fields(message.Body[at+len(prefix):])[0]
	if _, err := LookupToken(ctx, db, token, PurposeInvite); err != nil {
		t.Errorf("the link in the message does not resolve: %v", err)
	}
	// The recovery sentence travels with the message, because a mail app that
	// strips the fragment leaves nothing the server can see.
	if !strings.Contains(message.Body, "including the part after the #") {
		t.Error("the invite drops the what-to-do-if-the-link-fails sentence")
	}
}

// `SendReset` cannot tell its caller whether the address resolved, which is
// the shape that makes ADR 16's identical answer a property of the signature
// rather than a rule somebody has to remember.
func TestAResetIsSilentForEveryAddressThatCannotHaveOne(t *testing.T) {
	ctx := context.Background()
	db := newAccountsDB(t)
	claimed := mustCreate(t, db, "ada", "ada@example.com", false)
	claim(t, db, claimed)
	disabled := mustCreate(t, db, "gone", "gone@example.com", false)
	claim(t, db, disabled)
	if _, err := SetDisabled(ctx, db, disabled.ID, true); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, db, "noaddress", "", false)

	for _, address := range []string{
		"nobody@example.com", // no such account
		"gone@example.com",   // disabled: a lever they could undo is not a lever
		"not-an-address",     // not shaped like one at all
		"",
	} {
		sender := &recordingSender{}
		if err := SendReset(ctx, db, address, sender, "https://example.com"); err != nil {
			t.Errorf("%q: %v", address, err)
		}
		if len(sender.sent) != 0 {
			t.Errorf("%q: a message went out", address)
		}
	}

	sender := &recordingSender{}
	if err := SendReset(ctx, db, "ADA@example.com", sender, "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("a real address produced %d messages", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0].Body, "expires in an hour") {
		t.Error("the reset message is not the reset message")
	}

	// An *unclaimed* account does get one, and that is deliberate: somebody
	// whose invite expired asking for a reset is the same request in different
	// words, and refusing it would leave them with nothing to do but email the
	// maintainer.
	waiting := mustCreate(t, db, "waiting", "waiting@example.com", false)
	if has, _ := HasPassword(ctx, db, waiting.ID); has {
		t.Fatal("the fixture account is not unclaimed")
	}
	unclaimed := &recordingSender{}
	if err := SendReset(ctx, db, "waiting@example.com", unclaimed, "https://example.com"); err != nil {
		t.Fatal(err)
	}
	if len(unclaimed.sent) != 1 {
		t.Error("an unclaimed account was refused a reset link")
	}
}

func TestSenderFromEnvRefusesTheConsoleWhenDeployed(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("MTGLAB_REQUIRE_AUTH", "1")
	sender, err := SenderFromEnv(nil)
	if !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("a deployment with no key got %T (%v)", sender, err)
	}
	// The sentence a route answers verbatim as a 503.
	if !strings.HasPrefix(err.Error(), "this instance requires authentication") {
		t.Errorf("the refusal reads %q", err)
	}

	// A laptop gets the console sender, and the printed link is the feature.
	t.Setenv("MTGLAB_REQUIRE_AUTH", "0")
	sender, err = SenderFromEnv(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sender.(ConsoleSender); !ok {
		t.Fatalf("a laptop got %T", sender)
	}

	t.Setenv("RESEND_API_KEY", "re_not_a_real_key")
	t.Setenv("MTGLAB_REQUIRE_AUTH", "1")
	if sender, err = SenderFromEnv(nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := sender.(*ResendSender); !ok {
		t.Fatalf("a key got %T", sender)
	}
}

func TestTheConsoleSenderWritesWhereAPersonCanReadIt(t *testing.T) {
	var out strings.Builder
	sender := ConsoleSender{Stream: &out}
	if err := sender.Send(Message{To: "ada@example.com", Subject: "hello",
		Body: "one\ntwo"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NOT sent", "ada@example.com", "hello", "one", "two"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the console output is missing %q:\n%s", want, out.String())
		}
	}
}

// The request `ResendSender` builds, inspected without a socket. The transport
// is a seam inside the seam and it exists because the one thing that ever
// broke delivery was a header.
func TestTheProviderRequestCarriesTheUserAgent(t *testing.T) {
	var captured *http.Request
	var body []byte
	sender, err := NewResendSender("re_key", "mtglab <no-reply@example.com>",
		func(req *http.Request) (int, []byte, error) {
			captured = req
			body, _ = io.ReadAll(req.Body)
			return 200, []byte(`{"id":"1"}`), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(Message{To: "ada@example.com", Subject: "s", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if captured.URL.String() != ResendEndpoint || captured.Method != http.MethodPost {
		t.Errorf("the request went %s %s", captured.Method, captured.URL)
	}
	if got := captured.Header.Get("User-Agent"); got != UserAgent {
		t.Errorf("User-Agent = %q; Cloudflare answers a default one with 403", got)
	}
	if got := captured.Header.Get("Authorization"); got != "Bearer re_key" {
		t.Errorf("Authorization = %q", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["from"] != "mtglab <no-reply@example.com>" || payload["text"] != "b" {
		t.Errorf("the payload is %v", payload)
	}
	if to, ok := payload["to"].([]any); !ok || len(to) != 1 || to[0] != "ada@example.com" {
		t.Errorf("`to` is %#v; Resend wants a list", payload["to"])
	}

	// A refusal names the status and the provider's own error name, and
	// **never the recipient** -- their error bodies quote the address back.
	refusing, err := NewResendSender("re_key", "from@example.com",
		func(*http.Request) (int, []byte, error) {
			return 422, []byte(`{"name":"validation_error","message":"ada@example.com is invalid"}`), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	err = refusing.Send(Message{To: "ada@example.com", Subject: "s", Body: "b"})
	if !errors.Is(err, ErrEmailNotSent) {
		t.Fatalf("a refusal produced %v", err)
	}
	if !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "validation_error") {
		t.Errorf("the failure does not name the status and the error: %q", err)
	}
	if strings.Contains(err.Error(), "ada@example.com") {
		t.Errorf("the failure carries the recipient into a log line: %q", err)
	}

	// A body that is not the provider's JSON is worth saying so: that case is
	// something *in front of* the provider, and the two want different fixes.
	blocked, err := NewResendSender("re_key", "from@example.com",
		func(*http.Request) (int, []byte, error) {
			return 403, []byte("<!DOCTYPE html><title>error code: 1010</title>"), nil
		})
	if err != nil {
		t.Fatal(err)
	}
	err = blocked.Send(Message{To: "ada@example.com", Subject: "s", Body: "b"})
	if !strings.Contains(err.Error(), "not the provider's JSON") {
		t.Errorf("a WAF page reported as %q, which reads like a permission problem", err)
	}
}

func TestOpenReadWriteDoesNotCreateAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.db")
	db, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := PingWritable(context.Background(), db); err == nil {
		t.Fatal("the write handle opened a database that does not exist")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("OpenReadWrite created app.db; only Migrate may make the file")
	}
}
