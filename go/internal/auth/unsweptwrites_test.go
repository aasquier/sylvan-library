package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/aasquier/sylvan-library/go/internal/auth/authtest"
)

// The writes `halfwritten_test.go` does not reach.
//
// That file sweeps budgets over the three transactions whose half-finished
// states are dangerous: a password, an invite, the maintainer reconciliation.
// The rest of this package's writes are shorter — a guard, a read, an UPDATE —
// and were driven only at the budget that fails their *first* statement, where
// a closed handle already stands. The arms in between are the ones where the
// guard has passed and the write has not, which is the shape that leaves a
// caller believing something happened.
//
// One sweep rather than a counted budget per function, for the reason the other
// sweeps in this tree give: a counted budget is a restatement of today's
// statement order, and it goes quietly meaningless the day a query moves. The
// assertion is per operation and always the same pair — **an error means
// nothing changed, and no error means the change is there** — because those are
// the only two honest answers an account write has.

// twoAccounts seeds an admin who can sign in and an ordinary account over a
// handle with a statement budget, and hands back the ordinary one — the
// account every write below is aimed at.
//
// The admin is the load-bearing half even though it is never returned:
// `refuseIfLastAdmin` stands in front of three of these writes, and on a
// library with one account every one of them refuses at that guard without
// ever reaching the budget.
func twoAccounts(t *testing.T) (*User, *authFault) {
	t.Helper()
	db, fault := faultyAuth(t)
	ctx := context.Background()
	boss, err := Create(ctx, db, "gwyn", "gwyn@example.test", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SetPassword(ctx, db, boss.ID, "a long enough passphrase"); err != nil {
		t.Fatal(err)
	}
	squire := oneAccount(t, db)
	return squire, &authFault{db: db, fault: fault}
}

// authFault bundles the handle with its knob so one sweep can hold both.
type authFault struct {
	db    *sql.DB
	fault *authtest.Fault
}

// A sweep over every account write that had only its first statement driven.
//
// Each case says what it asked for and how to see whether it happened. What is
// asserted is never the driver's wording: it is that the rows agree with the
// answer. An account reported renamed that still answers to its old handle, or
// reported deleted and still in the roster, is the failure this whole file is
// about — the caller has no reason to try again, and nothing anywhere says the
// write did not land.
func TestNoAccountWriteReportsAChangeItDidNotMakeAtAnyBudget(t *testing.T) {
	t.Parallel()
	const widest = 8

	for _, tc := range []struct {
		what string
		// run performs the write on the ordinary account.
		run func(context.Context, *sql.DB, *User) error
		// landed reports whether the change is visible afterwards.
		landed func(context.Context, *sql.DB, *User) (bool, error)
	}{
		{"SetUsername",
			func(ctx context.Context, db *sql.DB, u *User) error {
				_, err := SetUsername(ctx, db, u.ID, "knight")
				return err
			},
			func(ctx context.Context, db *sql.DB, u *User) (bool, error) {
				got, err := GetByID(ctx, db, u.ID)
				if err != nil || got == nil {
					return false, err
				}
				return got.Username == "knight", nil
			}},
		{"SetDisabled",
			func(ctx context.Context, db *sql.DB, u *User) error {
				_, err := SetDisabled(ctx, db, u.ID, true)
				return err
			},
			func(ctx context.Context, db *sql.DB, u *User) (bool, error) {
				got, err := GetByID(ctx, db, u.ID)
				if err != nil || got == nil {
					return false, err
				}
				return got.Disabled, nil
			}},
		{"SetAdmin",
			func(ctx context.Context, db *sql.DB, u *User) error {
				return SetAdmin(ctx, db, u.ID, true)
			},
			func(ctx context.Context, db *sql.DB, u *User) (bool, error) {
				got, err := GetByID(ctx, db, u.ID)
				if err != nil || got == nil {
					return false, err
				}
				return got.IsAdmin, nil
			}},
		{"Delete",
			func(ctx context.Context, db *sql.DB, u *User) error {
				_, err := Delete(ctx, db, u.ID)
				return err
			},
			func(ctx context.Context, db *sql.DB, u *User) (bool, error) {
				got, err := GetByID(ctx, db, u.ID)
				if err != nil {
					return false, err
				}
				return got == nil, nil
			}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Parallel()
			refused, applied := 0, 0
			for budget := 0; budget <= widest; budget++ {
				squire, rig := twoAccounts(t)
				ctx := context.Background()
				rig.fault.After(budget)
				err := tc.run(ctx, rig.db, squire)
				rig.fault.Heal()

				landed, readErr := tc.landed(ctx, rig.db, squire)
				if readErr != nil {
					t.Fatalf("%s at budget %d: reading the account back: %v",
						tc.what, budget, readErr)
				}
				if err != nil {
					if landed {
						t.Errorf("%s refused at budget %d and the change is "+
							"there anyway: %v", tc.what, budget, err)
					}
					refused++
					continue
				}
				if !landed {
					t.Errorf("%s reported success at budget %d and nothing "+
						"changed -- the caller has no reason to try again",
						tc.what, budget)
				}
				applied++
			}
			// Both halves, or the sweep measured one state nine times.
			if refused == 0 || applied == 0 {
				t.Errorf("%s: %d budgets refused and %d applied -- the sweep "+
					"never crossed from one to the other",
					tc.what, refused, applied)
			}
		})
	}
}

// The roster over a handle that stops before it answers: a fault, never an
// empty list. "Nobody is registered" is a sentence the one person who could
// tell it is false reads off an empty roster.
func TestTheRosterIsAFaultRatherThanAnEmptyHouseWhenItCannotBeRead(t *testing.T) {
	t.Parallel()
	_, rig := twoAccounts(t)
	ctx := context.Background()

	whole, err := AllUsers(ctx, rig.db)
	if err != nil {
		t.Fatal(err)
	}
	if len(whole) != 2 {
		t.Fatalf("the fixture holds %d accounts, not two", len(whole))
	}

	rig.fault.After(0)
	got, err := AllUsers(ctx, rig.db)
	rig.fault.Heal()
	if err == nil {
		t.Fatalf("the roster answered %d accounts over a handle that stopped "+
			"answering", len(got))
	}
	if got != nil {
		t.Errorf("the refusal came back with %d accounts beside it", len(got))
	}
}

// An account that is not there is a named refusal rather than "no password".
//
// The two answers look the same to a caller that only checks the boolean, and
// they mean opposite things: `false` is an account somebody has to send an
// invite to, and this is an id that does not exist. The admin page prints one
// of them as a state a person can act on.
func TestAPasswordAskedAboutAnAccountThatIsNotThereNamesTheAccount(t *testing.T) {
	t.Parallel()
	squire, rig := twoAccounts(t)
	ctx := context.Background()

	has, err := HasPassword(ctx, rig.db, squire.ID)
	if err != nil || !has {
		t.Fatalf("the seeded account reads as %v (%v)", has, err)
	}

	has, err = HasPassword(ctx, rig.db, squire.ID+10_000)
	if err == nil {
		t.Fatal("an account that does not exist reported a password state")
	}
	if has {
		t.Error("it reported that the account can sign in")
	}
	if !errors.Is(err, ErrNoSuchUser) {
		t.Errorf("the refusal is not `no such user`: %v", err)
	}
}
