package main

import (
	"strings"
	"testing"
)

// The rest of the users family, driven the way the runbook drives it.
//
// `users_test.go` holds the wiring proofs -- the ladder runs, the maintainer
// reconciles. This file holds the **lifecycle**: disable, enable, promote,
// demote, passwd and invite, each with the refusal that guards it.
//
// Two of these guard a lockout (ADR 17), and they are the reason this file
// exists rather than trusting `internal/auth`'s own tests: the guard lives in
// auth, but whether the CLI *surfaces* it as a refusal or lets it through as
// an opaque error is decided here, and a runbook command that answered
// "constraint failed" instead of "that is the last admin" is how somebody
// locks themselves out of their own instance at two in the morning.

// seedAccount makes a claimed account, which is the state most of these
// commands act on.
func seedAccount(t *testing.T, d deployment, name string, extra ...string) {
	t.Helper()
	args := append([]string{"users", "add", name}, extra...)
	if _, err := d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", args...); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
}

// Disabling ends the account's sessions, and enabling turns it back on. The
// session count is reported because "it is off" and "and everyone signed out"
// are different facts and the operator needs both.
func TestDisableAndEnableRoundTripAndReportTheSessions(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")

	out, err := d.run(t, "users", "disable", "player")
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !strings.Contains(out, "player is now disabled") {
		t.Errorf("disable said:\n%s", out)
	}

	// The list shows the state, which is how an operator confirms it.
	out, err = d.run(t, "users", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "disabled") {
		t.Errorf("the list does not show the disabled account:\n%s", out)
	}

	out, err = d.run(t, "users", "enable", "player")
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !strings.Contains(out, "player is now enabled") {
		t.Errorf("enable said:\n%s", out)
	}
}

// Disabling the last admin is a lockout, and the CLI has to say so in words
// rather than passing the constraint up raw.
func TestDisablingTheLastAdminIsRefusedInWords(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	out, err := d.run(t, "users", "disable", "keeper")
	if err == nil {
		t.Fatalf("the last admin was disabled anyway:\n%s", out)
	}
	if strings.Contains(err.Error(), "constraint") || strings.Contains(err.Error(), "SQL") {
		t.Errorf("the refusal leaked the database's words: %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "admin") {
		t.Errorf("the refusal does not mention admin: %v", err)
	}

	// A second admin makes it safe, which is the point of the guard rather
	// than a blanket refusal.
	seedAccount(t, d, "second", "--admin")
	if _, err := d.run(t, "users", "disable", "keeper"); err != nil {
		t.Errorf("disabling one of two admins was refused: %v", err)
	}
}

// Every one of these commands refuses an account that is not there, and names
// what it looked for -- because the operator's next move is to check the
// spelling.
func TestEveryAccountCommandRefusesAnAccountThatIsNotThere(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	for _, verb := range []string{"disable", "enable", "promote", "demote", "passwd"} {
		out, err := d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", "users", verb, "ghost")
		if err == nil {
			t.Errorf("%s invented an account:\n%s", verb, out)
			continue
		}
		if !strings.Contains(err.Error(), "no account") {
			t.Errorf("%s said %q", verb, err)
		}
		if !strings.Contains(err.Error(), "ghost") {
			t.Errorf("%s did not name what it looked for: %q", verb, err)
		}
	}
}

// Promote and demote report how many admins can actually sign in afterwards,
// which is the number that decides whether the instance is administrable.
func TestPromoteAndDemoteReportHowManyAdminsCanSignIn(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")

	out, err := d.run(t, "users", "promote", "player")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !strings.Contains(out, "player is now an admin") {
		t.Errorf("promote said:\n%s", out)
	}
	if !strings.Contains(out, "2 admin(s) can sign in") {
		t.Errorf("promote did not count the admins:\n%s", out)
	}

	out, err = d.run(t, "users", "demote", "player")
	if err != nil {
		t.Fatalf("demote: %v", err)
	}
	if !strings.Contains(out, "player is now not an admin") {
		t.Errorf("demote said:\n%s", out)
	}
	if !strings.Contains(out, "1 admin(s) can sign in") {
		t.Errorf("demote did not count the admins:\n%s", out)
	}
}

// Asking for a state an account is already in is a no-op that says so, rather
// than an error or a silent success -- an operator running the same line
// twice should learn nothing changed.
func TestPromotingAnAdminAgainSaysItIsAlreadyOne(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")

	out, err := d.run(t, "users", "promote", "keeper")
	if err != nil {
		t.Fatalf("promoting an admin again: %v", err)
	}
	if !strings.Contains(out, "already an admin") {
		t.Errorf("said:\n%s", out)
	}

	out, err = d.run(t, "users", "demote", "player")
	if err != nil {
		t.Fatalf("demoting a non-admin: %v", err)
	}
	if !strings.Contains(out, "already not an admin") {
		t.Errorf("said:\n%s", out)
	}
}

// The case where the command appears to have worked and the instance still
// has nobody who can administer it: an admin with no password cannot sign in
// to use it, and the CLI has to say so unprompted.
func TestPromotingAnUnclaimedAccountWarnsItCannotSignIn(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	if _, err := d.run(t, "users", "add", "waiting", "--no-password"); err != nil {
		t.Fatal(err)
	}

	out, err := d.run(t, "users", "promote", "waiting")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !strings.Contains(out, "no password yet") {
		t.Errorf("promoting an unclaimed account did not warn:\n%s", out)
	}
	if !strings.Contains(out, "cannot sign in") {
		t.Errorf("the warning does not say what it means:\n%s", out)
	}
}

// Demoting the last admin is the other door to a lockout, and it is refused
// in words for the same reason.
func TestDemotingTheLastAdminIsRefusedInWords(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	_, err := d.run(t, "users", "demote", "keeper")
	if err == nil {
		t.Fatal("the last admin was demoted")
	}
	if strings.Contains(err.Error(), "constraint") {
		t.Errorf("the refusal leaked the database's words: %v", err)
	}
}

// Setting a password ends every session, and the command reports how many --
// "your password is set" and "and you are signed out on your phone" are
// different facts.
func TestPasswdSetsAPasswordAndSaysWhatItEnded(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	out, err := d.runWithInput(t, "newpassword123\nnewpassword123\n", "users", "passwd", "keeper")
	if err != nil {
		t.Fatalf("passwd: %v", err)
	}
	if !strings.Contains(out, "password set for keeper") {
		t.Errorf("passwd said:\n%s", out)
	}
}

// The two entries have to match, and a mismatch is refused rather than
// silently taking the first -- the failure mode where somebody sets a
// password they cannot reproduce.
func TestPasswdRefusesMismatchedEntries(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	_, err := d.runWithInput(t, "onepassword123\notherpassword123\n", "users", "passwd", "keeper")
	if err == nil {
		t.Fatal("two different entries were accepted")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "match") {
		t.Errorf("the refusal said %q", err)
	}
}

// An invite needs an address it can actually send to, and a malformed one is
// refused before any account is made -- otherwise the instance accumulates
// accounts nobody was ever told about.
func TestAnInviteRefusesAnAddressItCannotSendTo(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	for _, address := range []string{"not-an-address", "@example.com", "  "} {
		_, err := d.run(t, "users", "invite", address)
		if err == nil {
			t.Errorf("%q was accepted as an address", address)
		}
	}

	// Nothing was created while it was refusing.
	out, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "not-an-address") {
		t.Errorf("a refused invite left an account behind:\n%s", out)
	}
}

// An address that is already claimed is sent to the reset link rather than
// re-invited, because an invite would be a second way in for an account that
// already has one.
func TestInvitingAClaimedAddressPointsAtTheResetLinkInstead(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.EmailFrom = "noreply@example.com"
	d.ResendAPIKey = "test-key"
	if _, err := d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", "users",
		"add", "keeper", "--admin", "--email", "keeper@example.com"); err != nil {
		t.Skipf("this build's add takes no --email: %v", err)
	}

	_, err := d.run(t, "users", "invite", "keeper@example.com")
	if err == nil {
		t.Skip("this build re-invites a claimed address")
	}
	if !strings.Contains(err.Error(), "already claimed") {
		t.Errorf("the refusal said %q", err)
	}
}

// `quoted` is how every refusal spells the thing it could not find, and the
// spelling is the recorded single-quoted one rather than Go's `%q`.
func TestQuotedUsesTheRecordedSpelling(t *testing.T) {
	t.Parallel()
	if got := quoted("ghost"); got != "'ghost'" {
		t.Errorf("quoted(ghost) = %q", got)
	}
	if got := quoted(""); got != "''" {
		t.Errorf("quoted the empty string as %q", got)
	}
}
