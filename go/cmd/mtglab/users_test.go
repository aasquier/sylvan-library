package main

import (
	"os"
	"strings"
	"testing"
)

// The users family, driven exactly as a shell drives it: cobra Execute over a
// scratch MTGLAB_DATA_DIR, stdin piped (which takes readSecret's getpass
// fallback — the same path `echo pw | mtglab users passwd x` takes), stdout
// captured. The account logic itself is internal/auth's and is tested there;
// these hold the WIRING — the ladder runs, the maintainer reconciles, the
// refusals come out as refusals — because a CLI whose glue was never driven
// is how a runbook command breaks on the box it was written for.

func TestUsersListOnAFreshDirectorySaysSo(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	out, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "no accounts in") ||
		!strings.Contains(out, "mtglab users add") {
		t.Errorf("unexpected empty-list answer:\n%s", out)
	}
	// The connect helper ran the ladder: the file exists now, schema and all.
	if _, err := os.Stat(d.AppDBPath()); err != nil {
		t.Errorf("the ladder did not create app.db: %v", err)
	}
}

func TestUsersAddNoPasswordThenList(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	out, err := d.run(t, "users", "add", "keeper", "--no-password")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out, "created keeper in") ||
		!strings.Contains(out, "cannot log in yet") {
		t.Errorf("unexpected add output:\n%s", out)
	}
	out, err = d.run(t, "users", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "keeper") || !strings.Contains(out, "no password") {
		t.Errorf("unexpected list:\n%s", out)
	}
}

func TestUsersAddPromptedPasswordViaTheFallback(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	// Two matching entries down the pipe — getpass's non-tty fallback.
	out, err := d.runWithInput(t, "correct-horse-battery\ncorrect-horse-battery\n", "users",
		"add", "keeper")
	if err != nil {
		t.Fatalf("add with password: %v", err)
	}
	if !strings.Contains(out, "created keeper") {
		t.Errorf("unexpected output:\n%s", out)
	}
	out, err = d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "active") {
		t.Errorf("a claimed account should list as active:\n%s", out)
	}
}

func TestUsersAddRefusesMismatchedEntries(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	_, err := d.runWithInput(t, "one-entry-here\nanother-entry\n", "users", "add", "keeper")
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("want the two-entries refusal, got %v", err)
	}
}

func TestUsersPasswdOnAMissingAccountRefuses(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	_, err := d.run(t, "users", "passwd", "nobody")
	if err == nil || !strings.Contains(err.Error(), "refused: no account 'nobody'") {
		t.Fatalf("want the no-account refusal, got %v", err)
	}
}

func TestUsersTierRefusalNamesTheRoster(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatal(err)
	}
	_, err := d.run(t, "users", "tier", "keeper", "--tier", "imaginary")
	if err == nil || !strings.Contains(err.Error(), "no such tier 'imaginary'") ||
		!strings.Contains(err.Error(), "default, ") {
		t.Fatalf("want the roster in the refusal, got %v", err)
	}
	out, err := d.run(t, "users", "tier", "keeper", "--tier", "default")
	if err != nil {
		t.Fatalf("clearing the tier: %v", err)
	}
	if !strings.Contains(out, "keeper is answered by") {
		t.Errorf("unexpected tier output:\n%s", out)
	}
}

func TestUsersDemoteGuardsTheLastAdmin(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.runWithInput(t, "pw-that-is-long-enough\npw-that-is-long-enough\n", "users",
		"add", "root", "--admin"); err != nil {
		t.Fatal(err)
	}
	_, err := d.run(t, "users", "demote", "root")
	if err == nil || !strings.Contains(err.Error(), "refused:") {
		t.Fatalf("demoting the only signable admin must refuse, got %v", err)
	}
}

func TestUsersDeleteConfirmsByTypedName(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatal(err)
	}
	_, err := d.runWithInput(t, "somebody-else\n", "users", "delete", "keeper")
	if err == nil || !strings.Contains(err.Error(), "that is not the username") {
		t.Fatalf("a wrong confirmation must refuse, got %v", err)
	}
	out, err := d.runWithInput(t, "KEEPER\n", "users", "delete", "keeper")
	if err != nil {
		t.Fatalf("a case-folded confirmation deletes, got %v", err)
	}
	if !strings.Contains(out, "keeper is gone") {
		t.Errorf("unexpected delete output:\n%s", out)
	}
}

func TestUsersInviteWithoutMailRefuses(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.RequireAuth = true
	_, err := d.run(t, "users", "invite", "friend@example.com")
	if err == nil || !strings.Contains(err.Error(), "refused:") {
		t.Fatalf("an invite with no mail sender must refuse, got %v", err)
	}
}

func TestConnectUsersReconcilesTheMaintainer(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.AdminEmail = "aaron@example.com"
	out, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "*aaron") {
		t.Errorf("the maintainer was not reconciled into the list:\n%s", out)
	}
}
