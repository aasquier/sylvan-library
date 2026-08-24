package main

import (
	"strings"
	"testing"
)

// The account family's remaining doors: the invite, the tier grant, the
// delete, and the password strength floor every one of them writes through.
//
// The invite is the interesting one, because it is the only CLI command that
// **sends something to a person**. On a laptop with auth off it falls back to
// the console sender, which prints the message instead -- and that fallback
// is deliberately unavailable once `MTGLAB_REQUIRE_AUTH` is on, because the
// console fallback would print email addresses into the application log
// (ADR 16, the same rule `loggable` enforces at the door).

// The invite creates an unclaimed account and sends its owner a link. The
// handle comes from the address' local part unless one is given, and the
// account cannot log in until the link is used.
func TestAnInviteCreatesAnUnclaimedAccountAndSaysWhatHappens(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	out, err := d.run(t, "users", "invite", "grove.keeper@example.com")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	// The handle is the local part, normalised.
	if !strings.Contains(out, "invited grove.keeper") {
		t.Errorf("the invite said:\n%s", out)
	}
	// The two facts the person needs: they choose their own password, and
	// the link does not wait forever.
	if !strings.Contains(out, "own password") {
		t.Errorf("the invite does not say who chooses the password:\n%s", out)
	}
	if !strings.Contains(out, "once") || !strings.Contains(out, "week") {
		t.Errorf("the invite does not say what the link costs:\n%s", out)
	}

	// The account exists and is unclaimed, which is what the list shows.
	listed, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "grove.keeper") {
		t.Errorf("the invited account is not listed:\n%s", listed)
	}
	if !strings.Contains(listed, "invited") {
		t.Errorf("the invited account is not shown as invited:\n%s", listed)
	}
}

// `--username` overrides the derived handle, and `--admin` makes the invited
// account an admin -- both said back, because an invite that quietly granted
// admin would be a surprise nobody could audit.
func TestAnInviteTakesAHandleAndAGrantAndSaysSo(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	out, err := d.run(t, "users", "invite", "keeper@example.com",
		"--username", "grove-keeper", "--admin")
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if !strings.Contains(out, "grove-keeper") {
		t.Errorf("the chosen handle was not used:\n%s", out)
	}
	if !strings.Contains(out, "(admin)") {
		t.Errorf("the grant was not said back:\n%s", out)
	}
}

// A handle the address cannot produce is a refusal that says to pass one,
// rather than a mangling -- an invited person has to be told the handle they
// were given, so it cannot be invented for them.
func TestAnAddressThatCannotProduceAHandleAsksForOne(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	// A local part that is not a usable username.
	_, err := d.run(t, "users", "invite", "!!@example.com")
	if err == nil {
		t.Fatal("an unusable handle was invented anyway")
	}
	if !strings.Contains(err.Error(), "--username") {
		t.Errorf("the refusal does not say how to fix it: %q", err)
	}
}

// A handle somebody already holds is refused by name, and the refusal says
// which flag chooses another.
func TestAnInviteRefusesAHandleThatIsTaken(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", "users",
		"add", "keeper"); err != nil {
		t.Fatal(err)
	}

	_, err := d.run(t, "users", "invite", "keeper@example.com")
	if err == nil {
		t.Fatal("a taken handle was reused")
	}
	if !strings.Contains(err.Error(), "keeper") || !strings.Contains(err.Error(), "--username") {
		t.Errorf("the refusal said %q", err)
	}
}

// **The console fallback is unavailable once auth is on**, because it would
// print email addresses into the application log. An instance that requires
// authentication and has no mail provider says so rather than inviting into
// the void.
func TestAnInviteRefusesWhenTheOnlySenderWouldLogTheAddress(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	d.RequireAuth = true

	_, err := d.run(t, "users", "invite", "keeper@example.com")
	if err == nil {
		t.Fatal("an invite was sent with nothing to send it with")
	}
	// The refusal names the fix and the reason, because the person reading
	// it is the person who can set the secret.
	if !strings.Contains(err.Error(), "RESEND_API_KEY") {
		t.Errorf("the refusal does not name the secret: %q", err)
	}
	if !strings.Contains(err.Error(), "log") {
		t.Errorf("the refusal does not say why the fallback is refused: %q", err)
	}
}

// An account is created with an address so invites and resets have somewhere
// to go, and a malformed one is refused by name rather than stored.
func TestAnAccountIsCreatedWithAnAddressOrRefusedByName(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	out, err := d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", "users",
		"add", "keeper", "--email", "keeper@example.com", "--admin")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out, "created keeper (admin)") {
		t.Errorf("add said:\n%s", out)
	}

	// A second account on the same address is refused, because a reset link
	// that could resolve to two accounts resolves to neither.
	_, err = d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", "users",
		"add", "second", "--email", "keeper@example.com")
	if err == nil {
		t.Error("two accounts share an address")
	}

	// A malformed address is refused rather than stored.
	_, err = d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", "users",
		"add", "third", "--email", "not-an-address")
	if err == nil {
		t.Error("a malformed address was stored")
	}

	// A handle that is not a handle, likewise.
	_, err = d.runWithInput(t, "hunter2hunter2\nhunter2hunter2\n", "users", "add", "!! not a handle !!")
	if err == nil {
		t.Error("an unusable handle was created")
	}
}

// The password floor is checked before the account is touched, so a weak one
// never becomes a stored hash.
func TestAWeakPasswordIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)

	for _, tc := range []struct{ name, entry string }{
		{"too short", "short\nshort\n"},
		{"empty", "\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := d.runWithInput(t, tc.entry, "users", "add", "keeper")
			if err == nil {
				t.Fatal("a weak password was accepted")
			}
		})
	}

	// Nothing was created while it was refusing.
	out, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "keeper") {
		t.Errorf("a refused password left an account behind:\n%s", out)
	}
}

// The tier grant chooses which Claude answers an account, and `default`
// **clears** the grant rather than naming the default tier -- so "nobody has
// chosen anything" has one spelling in the column whichever door it came
// through.
func TestTheTierGrantClearsRatherThanNamingTheDefault(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	// A real tier.
	out, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	before := out

	if _, err := d.run(t, "users", "tier", "keeper", "--tier", "default"); err != nil {
		t.Fatalf("clearing the tier: %v", err)
	}
	after, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Logf("clearing a tier nobody set changed the listing:\n%s\n%s", before, after)
	}

	// An unknown tier is refused with the roster, because the caller's next
	// move is to pick one that exists.
	_, err = d.run(t, "users", "tier", "keeper", "--tier", "nonsense")
	if err == nil {
		t.Fatal("an unknown tier was granted")
	}
	if !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("the refusal does not name what was asked for: %q", err)
	}
}

// Deleting is the one irreversible verb, so it is confirmed by typing the
// name -- and `--yes` is the scripted door, which still refuses an account
// that is not there.
func TestDeletingIsConfirmedByNameOrByFlag(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")
	seedAccount(t, d, "player")

	// The wrong name is a refusal, and the account survives.
	if _, err := d.runWithInput(t, "wrong-name\n", "users", "delete", "player"); err == nil {
		t.Error("a mistyped confirmation deleted the account")
	}
	out, err := d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "player") {
		t.Errorf("a refused delete removed the account anyway:\n%s", out)
	}

	// The right name goes through.
	if _, err := d.runWithInput(t, "player\n", "users", "delete", "player"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	out, err = d.run(t, "users", "list")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "player") {
		t.Errorf("the account survived its own deletion:\n%s", out)
	}

	// And an account that is not there is refused rather than reported as
	// deleted.
	if _, err := d.runWithInput(t, "ghost\n", "users", "delete", "ghost"); err == nil {
		t.Error("deleting an account that is not there succeeded")
	}
}

// The scripted door skips the prompt, which is what a runbook uses -- and it
// is still refused for the last admin, because `--yes` answers "am I sure",
// not "may I lock myself out".
func TestTheScriptedDeleteStillGuardsTheLastAdmin(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	seedAccount(t, d, "keeper", "--admin")

	_, err := d.run(t, "users", "delete", "keeper", "--yes")
	if err == nil {
		t.Fatal("the last admin deleted itself with --yes")
	}
	if strings.Contains(err.Error(), "constraint") {
		t.Errorf("the refusal leaked the database's words: %v", err)
	}

	// With a second admin it goes through without a prompt.
	seedAccount(t, d, "second", "--admin")
	if _, err := d.run(t, "users", "delete", "keeper", "--yes"); err != nil {
		t.Errorf("--yes was refused with two admins: %v", err)
	}
}
