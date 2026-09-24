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
	"github.com/aasquier/sylvan-library/go/internal/sim/tier3"
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

// envOr is the fallback the two boot flags read through, and an empty value is
// an absent one -- because an environment variable set to the empty string is
// how a container spells "not set", and taking it literally would override a
// working default with nothing.
//
// **The deployment is a map here**, which is the whole of what ADR 39's shape
// bought this function: it used to read [os.Getenv], so the only way to
// describe a machine that had set the variable was [testing.T.Setenv] -- a
// write to the process, and the last serial test in this package. The lookup is
// an argument now, so four machines can be described in one parallel test and
// none of them is this one.
func TestEnvOrTreatsAnEmptyValueAsAbsent(t *testing.T) {
	t.Parallel()
	machine := func(pairs map[string]string) func(string) string {
		return func(name string) string { return pairs[name] }
	}
	for _, tc := range []struct {
		what     string
		env      map[string]string
		fallback string
		want     string
	}{
		{"an empty value", map[string]string{"MTGLAB_WEB_DIST": ""}, "the default", "the default"},
		{"an explicit value", map[string]string{"MTGLAB_WEB_DIST": "explicit"}, "the default", "explicit"},
		{"an unset variable", map[string]string{}, "the default", "the default"},
		// An empty fallback is still a legitimate answer.
		{"an empty fallback", map[string]string{}, "", ""},
		// Whitespace is a value somebody meant: a path can begin with a space
		// and this is not the place to second-guess one.
		{"a value that is only a space", map[string]string{"MTGLAB_WEB_DIST": " "}, "the default", " "},
	} {
		if got := envOr(machine(tc.env), "MTGLAB_WEB_DIST", tc.fallback); got != tc.want {
			t.Errorf("%s gave %q, want %q", tc.what, got, tc.want)
		}
	}
	// And the name is read rather than ignored: a machine that set the other
	// flag's variable has said nothing about this one.
	elsewhere := machine(map[string]string{"MTGLAB_TAROT_DIR": "somewhere else"})
	if got := envOr(elsewhere, "MTGLAB_WEB_DIST", "the default"); got != "the default" {
		t.Errorf("a variable set under another name gave %q", got)
	}
}

// And the flags themselves carry it: `--web-dist` and `--tarot` are the two
// defaults [envOr] exists for, so a `ui` command built with nothing said about
// them offers the built-in pair rather than an empty string.
//
// The assertion is the *help*, which is what an operator actually reads: a
// flag whose default vanished would still parse, still serve nothing, and say
// so nowhere.
func TestTheBootFlagsOfferTheBuiltInPathsWhenNobodySaysOtherwise(t *testing.T) {
	t.Parallel()
	cmd := uiCommand(config.Config{}, tier3.Settings{})
	for flag, want := range map[string]string{
		"web-dist": "web_dist",
		"tarot":    filepath.Join("assets", "tarot"),
	} {
		f := cmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("`mtglab ui` has no --%s", flag)
			continue
		}
		// An operator's machine may have said otherwise, and this test is not
		// about that machine -- what it holds is that the default is a path
		// rather than nothing at all.
		if f.DefValue == "" {
			t.Errorf("--%s defaults to nothing; the built-in is %q", flag, want)
		}
	}
	// The flag that describes an action this command has never taken stays
	// gone: `--no-open` was parsed into a variable and thrown away while its
	// help promised "serve without opening a browser".
	if cmd.Flags().Lookup("no-open") != nil {
		t.Error("`--no-open` is back, and nothing in this process opens anything")
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
