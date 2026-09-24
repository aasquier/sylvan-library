package main

import (
	"strings"
	"testing"
)

// The edges an operator reaches by typing: a slug that is not a deck, a tier
// nobody has, a name with an apostrophe in it, and the board a deck keeps
// beside its ninety-nine.
//
// Every one of them ends in a sentence somebody reads off a terminal on the
// deployed box, which is what makes them worth holding: `fly ssh console -C`
// is the whole interface there, and a refusal that says the wrong thing sends
// the maintainer after the wrong problem.

// A deck the simulator cannot load is refused by every command that takes a
// slug, before anything is swept or compiled.
func TestASweepOverADeckThatIsNotThereIsRefusedBeforeItSweeps(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	out, err := d.run(t, "sim", "lands", "nope", "30", "40")
	if err == nil {
		t.Fatalf("a sweep ran over a deck that is not there:\n%s", out)
	}
	if !strings.HasPrefix(err.Error(), "no deck at ") {
		t.Errorf("the refusal said %q", err)
	}
	if strings.Contains(out, "lands") {
		t.Errorf("the sweep printed its header before refusing:\n%s", out)
	}
}

// The swap board is part of the deck the simulator compiles: a card sitting on
// the board is looked up with the ninety-nine, because it is a card the deck
// may be carrying tomorrow and the mana base is judged against both.
func TestTheBoardIsCompiledBesideTheNinetyNine(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	writeSimDeck(t, d, "boarded", "slug: boarded\nname: Boarded\n"+
		"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
		"  - name: Forest\n    qty: 98\n    why: the mana\n"+
		"  - name: Sol Ring\n    why: the acceleration\n"+
		"swap_board:\n  - name: Craterhoof Behemoth\n    why: waiting in the wings\n")

	out, err := d.run(t, "sim", "shelf", "boarded")
	if err != nil {
		t.Fatalf("sim shelf over a deck with a board: %v", err)
	}
	if !strings.Contains(out, "Boarded") {
		t.Errorf("the report is not about the deck:\n%s", out)
	}
}

// A mana base that cannot pay for its own cards says **short** and by how
// much, because "you are short two green sources" is the sentence somebody
// acts on and "ok" is not.
func TestAManaBaseThatCannotPayItsOwnCardsSaysHowShort(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	// Two lands under ninety-seven green spells: whatever rung the closed form
	// judges, this deck is short of it.
	writeSimDeck(t, d, "thirsty", "slug: thirsty\nname: Thirsty\n"+
		"commander:\n  - Goreclaw, Terror of Qal Sisma\ncards:\n"+
		"  - name: Forest\n    qty: 2\n    why: all the mana there is\n"+
		"  - name: Craterhoof Behemoth\n    qty: 97\n    why: the top end\n")

	out, err := d.run(t, "sim", "shelf", "thirsty")
	if err != nil {
		t.Fatalf("sim shelf: %v", err)
	}
	// The verdict sits in its own column, after the `--`, which is what
	// separates it from the prose above the table that also says "short".
	const verdict = "-- short "
	if !strings.Contains(out, verdict) {
		t.Errorf("a deck with two lands under ninety-seven spells was called "+
			"ok on every rung:\n%s", out)
	}
	// And the shortfall is a number rather than a warning: `short 2` is
	// actionable, `short` alone is a mood.
	for _, line := range strings.Split(out, "\n") {
		idx := strings.Index(line, verdict)
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx+len(verdict):])
		if rest == "" || rest[0] < '0' || rest[0] > '9' {
			t.Errorf("a shortfall reads %q", strings.TrimSpace(line))
		}
		break
	}
}

// The grid marks the rule you are already using, so a reader can see what the
// best rule is being compared against rather than taking the gain on trust.
func TestTheGridMarksTheRuleYouAreAlreadyUsing(t *testing.T) {
	t.Parallel()
	d := simHome(t, true)
	writeSimDeck(t, d, "mono-green", monoGreenText(t))

	// Every rule shown, so the default is certainly among them -- with a short
	// list it may rank below the cut, and a mark nobody can see proves
	// nothing.
	out, err := d.run(t, "sim", "mulligan", "mono-green", "--games", "40", "--top", "99")
	if err != nil {
		t.Fatalf("sim mulligan: %v", err)
	}
	if !strings.Contains(out, "=") {
		t.Errorf("no row is marked as the default:\n%s", out)
	}
	if !strings.Contains(out, "*") {
		t.Errorf("no row is marked as the best:\n%s", out)
	}
}

// `users tier` reaches `app.db` like every other subcommand, and it is the one
// the volume sweep cannot drive: `--tier` is required, so cobra refuses it
// before the body runs and the sweep's placeholder never gets that far.
func TestGrantingATierNeedsAVolumeAndAnAccount(t *testing.T) {
	t.Parallel()
	if out, err := unmounted(t).run(t, "users", "tier", "keeper", "--tier", "default"); err == nil {
		t.Errorf("a tier was granted on a volume that did not mount:\n%s", out)
	}

	d := scratchDeployment(t)
	_, err := d.run(t, "users", "tier", "nobody", "--tier", "default")
	if err == nil {
		t.Fatal("a tier was granted to an account that does not exist")
	}
	if !strings.Contains(err.Error(), "no account 'nobody'") {
		t.Errorf("the refusal said %q", err)
	}
}

// A name with an apostrophe in it is quoted with the other pair, so the
// refusal reads as a name rather than as a broken string. Usernames cannot
// hold one, which is exactly why this is worth pinning: the refusal is
// rendered before the name is validated, so whatever somebody typed lands
// here.
func TestARefusalQuotesWhateverWasTypedWithoutBreakingOnIt(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	for _, tc := range []struct{ typed, want string }{
		{"keeper", "'keeper'"},
		{"o'brien", `"o'brien"`},
		{`say "hello"`, `'say "hello"'`},
	} {
		_, err := d.run(t, "users", "passwd", tc.typed)
		if err == nil {
			t.Fatalf("an account named %q was found", tc.typed)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q was quoted as %q, want %q", tc.typed, err.Error(), tc.want)
		}
	}
}

// A password prompt that runs out of input gives up rather than treating the
// end of the pipe as an entry: `echo pw | mtglab users passwd x` supplies one
// line where two are wanted, and a command that read the second as the empty
// string would set a password nobody typed.
func TestAPromptThatRunsOutOfInputGivesUp(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatalf("seeding an account: %v", err)
	}
	_, err := d.runWithInput(t, "only-one-entry-here\n", "users", "passwd", "keeper")
	if err == nil {
		t.Fatal("a password was set from half an entry")
	}
	// It is the pipe ending rather than a mismatch: two entries that differ is
	// a different sentence and a different mistake.
	if strings.Contains(err.Error(), "did not match") {
		t.Errorf("an exhausted pipe was reported as a mismatch: %v", err)
	}
}

// A password set over a database whose sessions have gone is refused after the
// hash is written — which is a real state and the honest report of it: the
// command cannot say how many devices it signed out, so it does not say it
// did.
func TestAPasswordThatCannotEndTheSessionsItReplacesSaysSo(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatalf("seeding an account: %v", err)
	}
	hollowed(t, d, "sessions")

	out, err := d.runWithInput(t, "correct-horse-battery\ncorrect-horse-battery\n",
		"users", "add", "second-keeper")
	if err == nil {
		t.Fatalf("an account was claimed without its sessions being reckoned:\n%s", out)
	}
	if strings.Contains(out, "cannot log in yet") {
		t.Errorf("the command reported a state it never reached:\n%s", out)
	}
}

// The maintainer reconciliation runs on every `users` subcommand too, so an
// address that cannot be reconciled refuses there as well as at boot: the CLI
// and the app agree about who administers the instance no matter which one ran
// last.
func TestAUsersCommandRefusesWhenTheMaintainerCannotBeReconciled(t *testing.T) {
	t.Parallel()
	d := claimedSchema(t, scratchDeployment(t))
	d.AdminEmail = "keeper@example.com"
	d.AdminUsername = "keeper"

	out, err := d.run(t, "users", "list")
	if err == nil {
		t.Fatalf("the roster was printed over an instance nobody could administer:\n%s", out)
	}
}

// An invite to an address nobody holds creates the account and mails the link
// — and when the ledger that link lives in has gone, it is a refusal rather
// than a report of an invite nobody will ever receive.
func TestAnInviteWithNowhereToRecordItIsRefused(t *testing.T) {
	t.Parallel()
	d := scratchDeployment(t)
	if _, err := d.run(t, "users", "add", "keeper", "--no-password"); err != nil {
		t.Fatalf("seeding an account: %v", err)
	}
	hollowed(t, d, "auth_tokens")

	// A fresh address, so this reaches the invite rather than stopping at the
	// username already being taken.
	out, err := d.run(t, "users", "invite", "newcomer@example.com")
	if err == nil {
		t.Fatalf("an invite was reported sent with nowhere to record it:\n%s", out)
	}
	if strings.Contains(out, "invited ") {
		t.Errorf("the command said the invite went out:\n%s", out)
	}
}
