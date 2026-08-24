package auth

import (
	"context"
	"database/sql"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/config"
)

// The maintainer reconciler's contract, driven through the same scratch
// `app.db` the accounts tests use.
//
// Every test here hands `EnsureMaintainer` the configuration it is about.
// This used to set both variables through t.Setenv -- including to "", to stop
// a developer's own MTGLAB_ADMIN_EMAIL from deciding whether the suite passed
// on their laptop. Neither the blanking nor the serial execution t.Setenv
// forced is needed now: an unconfigured instance is `config.Config{}`, which
// no environment can reach into, so every test here runs in parallel.

func maintainer(email, username string) config.Config {
	return config.Config{AdminEmail: email, AdminUsername: username}
}

func ensure(t *testing.T, db *sql.DB, cfg config.Config) {
	t.Helper()
	if err := EnsureMaintainer(context.Background(), db, cfg); err != nil {
		t.Fatalf("EnsureMaintainer: %v", err)
	}
}

func TestEnsureMaintainerIsANoOpWhenUnconfigured(t *testing.T) {
	t.Parallel()
	db := newAccountsDB(t)
	ensure(t, db, maintainer("", "ignored-without-an-address"))
	everyone, err := AllUsers(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(everyone) != 0 {
		t.Fatalf("an unconfigured bootstrap created %d account(s)", len(everyone))
	}
}

func TestEnsureMaintainerCreatesTheAccountUnclaimedAndAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newAccountsDB(t)
	ensure(t, db, maintainer("Ada.Lovelace@Example.COM", ""))

	account, err := GetByEmail(ctx, db, "ada.lovelace@example.com")
	if err != nil || account == nil {
		t.Fatalf("no maintainer account was created (%v)", err)
	}
	// The handle is derived from the *normalised* address when the variable is
	// unset -- lowercased with it, because `UsernameFor` is handed the
	// normalised form and never the raw one.
	if account.Username != "ada.lovelace" {
		t.Errorf("the handle is %q, not the normalised local part", account.Username)
	}
	if !account.IsAdmin || account.Disabled {
		t.Errorf("the maintainer is admin=%v disabled=%v", account.IsAdmin, account.Disabled)
	}
	// ADR 16's shape: unclaimed, never a password somebody else chose.
	if has, err := HasPassword(ctx, db, account.ID); err != nil || has {
		t.Errorf("a bootstrapped account has a password (%v, %v)", has, err)
	}

	// Idempotent: the steady state is every boot after the first changing
	// nothing.
	ensure(t, db, maintainer("Ada.Lovelace@Example.COM", ""))
	everyone, err := AllUsers(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(everyone) != 1 {
		t.Fatalf("a second boot created a second account: %d", len(everyone))
	}
}

func TestEnsureMaintainerHonoursTheConfiguredHandle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newAccountsDB(t)
	ensure(t, db, maintainer("keeper@example.com", "grove-keeper"))
	account, err := GetByEmail(ctx, db, "keeper@example.com")
	if err != nil || account == nil {
		t.Fatalf("no maintainer account was created (%v)", err)
	}
	if account.Username != "grove-keeper" {
		t.Errorf("MTGLAB_ADMIN_USERNAME was not honoured: %q", account.Username)
	}
}

func TestEnsureMaintainerFallsBackWhenTheConfiguredHandleIsUnusable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A malformed preference is logged and ignored, never fatal: the handle
	// falls back to the address' local part.
	db := newAccountsDB(t)
	ensure(t, db, maintainer("keeper@example.com", "!! not a handle !!"))
	account, err := GetByEmail(ctx, db, "keeper@example.com")
	if err != nil || account == nil {
		t.Fatalf("no maintainer account was created (%v)", err)
	}
	if account.Username != "keeper" {
		t.Errorf("the fallback handle is %q, not the local part", account.Username)
	}
}

func TestEnsureMaintainerReconcilesAdminAndEnabledButNeverTheHandle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// The account exists already -- claimed, demoted and disabled, which is
	// the restored-from-an-old-backup shape the reconciliation exists for.
	db := newAccountsDB(t)
	existing := mustCreate(t, db, "aaron", "aaron@example.com", false)
	claim(t, db, existing)
	if _, err := SetDisabled(ctx, db, existing.ID, true); err != nil {
		t.Fatalf("disabling the fixture: %v", err)
	}

	// The configured handle differs on purpose: the username is used at
	// creation and never afterwards, because renaming somebody at boot is a
	// surprise this has no way to warn them about.
	ensure(t, db, maintainer("aaron@example.com", "somebody-else"))

	account, err := GetByID(ctx, db, existing.ID)
	if err != nil || account == nil {
		t.Fatalf("the account vanished (%v)", err)
	}
	if !account.IsAdmin {
		t.Error("the maintainer was not promoted")
	}
	if account.Disabled {
		t.Error("the maintainer was not re-enabled")
	}
	if account.Username != "aaron" {
		t.Errorf("the handle was reconciled to %q; it must never be", account.Username)
	}
	everyone, err := AllUsers(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(everyone) != 1 {
		t.Fatalf("reconciling created an account: %d", len(everyone))
	}
}

func TestEnsureMaintainerStepsAroundATakenHandle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// A friend invited as `aaron` before the maintainer was configured.
	// Renaming theirs would be worse, so the maintainer becomes `aaron2`.
	db := newAccountsDB(t)
	mustCreate(t, db, "aaron", "friend@example.com", false)

	ensure(t, db, maintainer("aaron@example.com", ""))
	account, err := GetByEmail(ctx, db, "aaron@example.com")
	if err != nil || account == nil {
		t.Fatalf("no maintainer account was created (%v)", err)
	}
	if account.Username != "aaron2" {
		t.Errorf("the collision landed on %q, not aaron2", account.Username)
	}
	if !account.IsAdmin {
		t.Error("the stepped-around maintainer is not an admin")
	}
}

func TestEnsureMaintainerLogsAndSkipsAMalformedAddress(t *testing.T) {
	t.Parallel()
	// Loud, and not fatal: refusing to start would turn a typo in one
	// environment variable into an instance that serves nothing.
	db := newAccountsDB(t)
	ensure(t, db, maintainer("not-an-address", ""))
	everyone, err := AllUsers(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(everyone) != 0 {
		t.Fatalf("a malformed address minted %d account(s)", len(everyone))
	}
}

func TestUsernameForManglesDeterministically(t *testing.T) {
	t.Parallel()
	cases := []struct{ email, want string }{
		// The usual case: the local part is the name its owner would pick.
		{"ada.lovelace@example.com", "ada.lovelace"},
		// Characters outside the handle alphabet are dropped, not replaced.
		{"ada+decks@example.com", "adadecks"},
		{"Ada Lovelace@example.com", "AdaLovelace"},
		// Leading punctuation is stripped so the pattern's first-character
		// rule can pass.
		{"._-ada@example.com", "ada"},
		// Too short for a handle after cleaning: the last resort is `admin`.
		{"a@example.com", "admin"},
		{"._-a@example.com", "admin"},
		// The Unicode-wide keep-filter keeps the letter, and the ASCII handle
		// pattern then refuses the whole -- `admin`, never a mangled `mller`.
		{"müller@example.com", "admin"},
		{"42fun@example.com", "42fun"},
	}
	for _, c := range cases {
		if got := UsernameFor(c.email); got != c.want {
			t.Errorf("UsernameFor(%q) = %q, want %q", c.email, got, c.want)
		}
	}
}
