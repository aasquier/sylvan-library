package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
)

// The small pieces the boot sequence and the runbook rest on: the maintainer
// reconciliation the serving process owes ADR 17, and the two renderers whose
// output lands in a runbook transcript.

// The reconciliation runs at boot, after the ladder and before the door, and
// is the one write nobody is watching. It has to be a no-op without an
// address, do its work with one, and hand its handle back rather than holding
// `app.db` open for the process's life -- the door opens its own handles with
// its own lifetimes.
func TestTheMaintainerReconciliationAtBootIsAWholeTransaction(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}

	// No address: nothing to reconcile, and no database is acquired for it --
	// which is what a laptop with no MTGLAB_ADMIN_EMAIL wants.
	if err := ensureMaintainerAtBoot(cfg); err != nil {
		t.Fatalf("no admin email: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app.db")); err == nil {
		t.Error("booting with no admin email created app.db anyway")
	}

	// The ladder runs first in `serve`, and creating the file is its job
	// rather than this one's.
	if err := auth.Migrate(cfg.AppDBPath()); err != nil {
		t.Fatal(err)
	}

	// With an address: the account exists afterwards, and it is an admin.
	cfg.AdminEmail = "keeper@example.com"
	cfg.AdminUsername = "keeper"
	if err := ensureMaintainerAtBoot(cfg); err != nil {
		t.Fatalf("reconciling: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "app.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	user, err := auth.Get(context.Background(), db, "keeper")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("the maintainer was not created")
	}
	if !user.IsAdmin {
		t.Error("the maintainer is not an admin")
	}

	// Twice is the same as once -- boots are frequent and this must not
	// accumulate accounts.
	if err := ensureMaintainerAtBoot(cfg); err != nil {
		t.Fatalf("the second boot: %v", err)
	}
	users, err := auth.AllUsers(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Errorf("two boots left %d accounts", len(users))
	}
}

// An address that cannot be reconciled must not stop the process from
// starting: an instance that refuses to boot over a mistyped environment
// variable is worse than one that boots and says so.
func TestAnUnusableAdminEmailDoesNotStopTheBoot(t *testing.T) {
	t.Parallel()
	cfg := config.Config{DataDir: t.TempDir(), AdminEmail: "not-an-address"}
	if err := auth.Migrate(cfg.AppDBPath()); err != nil {
		t.Fatal(err)
	}
	if err := ensureMaintainerAtBoot(cfg); err != nil {
		t.Errorf("a mistyped admin email failed the boot: %v", err)
	}
}

// A database that is not there is a real failure and is reported rather than
// swallowed: this reconciliation runs after the ladder, so a missing file at
// this point means the volume did not mount, and an instance that carried on
// would serve with no maintainer and no complaint.
func TestAMissingDatabaseFailsTheReconciliation(t *testing.T) {
	t.Parallel()
	err := ensureMaintainerAtBoot(config.Config{
		DataDir:    filepath.Join(t.TempDir(), "never-mounted"),
		AdminEmail: "keeper@example.com"})
	if err == nil {
		t.Error("a volume that did not mount reconciled anyway")
	}
}

// envOr is the fallback every boot flag reads through, and an empty value is
// an absent one. **Serial, and the last environment reader in this package on
// purpose**: Cobra needs `--web-dist` and `--tarot` to have defaults while it
// is building the command tree, before a [config.Load] could have run, so
// these two are read here rather than passed. ADR 39 named the exception and
// ADR 40 left it standing -- an environment variable set to the empty string is how a
// container spells "not set", and taking it literally would override a
// working default with nothing.
func TestEnvOrTreatsAnEmptyValueAsAbsent(t *testing.T) {
	t.Setenv("MTGLAB_TEST_ENVOR", "")
	if got := envOr("MTGLAB_TEST_ENVOR", "the default"); got != "the default" {
		t.Errorf("an empty value gave %q", got)
	}
	t.Setenv("MTGLAB_TEST_ENVOR", "explicit")
	if got := envOr("MTGLAB_TEST_ENVOR", "the default"); got != "explicit" {
		t.Errorf("an explicit value gave %q", got)
	}
	if got := envOr("MTGLAB_TEST_ENVOR_UNSET_ENTIRELY", "the default"); got != "the default" {
		t.Errorf("an unset variable gave %q", got)
	}
	// An empty fallback is still a legitimate answer.
	if got := envOr("MTGLAB_TEST_ENVOR_UNSET_ENTIRELY", ""); got != "" {
		t.Errorf("an empty fallback gave %q", got)
	}
}

// The byte counts in a runbook transcript are read by a person, so they wear
// separators. The boundary at four digits is where a naive loop puts a comma
// in front of the first digit.
func TestByteCountsWearSeparators(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{34512, "34,512"},
		{999999, "999,999"},
		{1000000, "1,000,000"},
		{89802672, "89,802,672"},
	} {
		if got := commas(tc.in); got != tc.want {
			t.Errorf("commas(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Never a leading separator, whatever the length.
	for n := int64(1); n < 2_000_000; n *= 7 {
		if got := commas(n); strings.HasPrefix(got, ",") {
			t.Errorf("commas(%d) = %q", n, got)
		}
	}
}
