package main

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/aasquier/sylvan-library/go/internal/auth"
	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/tiers"
)

// usersCommand is `mtglab users`: accounts on this box's app.db — the
// shell-side administration the deployed instance depends on, because
// `fly ssh console -C "mtglab users list"` is the runbook's answer to "who
// can sign in" (docs/HOSTING.md).
//
// Every subcommand goes through `connectUsers`, which runs the ladder and
// reconciles the maintainer (ADR 17) — so the CLI and the app agree about who
// administers the instance no matter which one ran last.
func usersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Accounts: create, invite, list, and administer",
	}
	cmd.AddCommand(usersAddCommand(), usersInviteCommand(), usersListCommand(),
		usersPasswdCommand(), usersDisableCommand(), usersEnableCommand(),
		usersPromoteCommand(), usersDemoteCommand(), usersTierCommand(),
		usersDeleteCommand())
	return cmd
}

// connectUsers opens app.db read-write with the ladder run and the
// maintainer reconciled first. The ladder rather than a bare open, because
// on a laptop the first `users add` is what creates the file — an open that
// could not mint the schema would strand the very first command.
func connectUsers(ctx context.Context) (*sql.DB, error) {
	path := config.AppDBPath()
	if err := auth.Migrate(path); err != nil {
		return nil, err
	}
	db, err := auth.OpenReadWrite(path)
	if err != nil {
		return nil, err
	}
	if err := auth.EnsureMaintainer(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func refused(format string, a ...any) error {
	return fmt.Errorf("refused: "+format, a...)
}

// promptNewPassword reads a password twice, from the terminal, never from an
// argument. When stdin is not a terminal it falls back to reading a line in
// the clear and warns that it may echo — the recorded fallback, and also
// what makes this path testable.
func promptNewPassword(who string) (string, error) {
	first, err := readSecret(fmt.Sprintf("  new password for %s: ", who))
	if err != nil {
		return "", err
	}
	again, err := readSecret("  again: ")
	if err != nil {
		return "", err
	}
	if first != again {
		return "", refused("the two entries did not match")
	}
	if err := auth.CheckStrength(first); err != nil {
		return "", refused("%s", err)
	}
	return first, nil
}

// stdinLines is the one buffered reader over stdin, shared by every prompt in
// the process: a second `bufio.NewReader` would lose whatever the first one
// buffered past its own line, which with two piped password entries is the
// second entry. Rebuilt when os.Stdin itself is swapped, which is how the
// tests hand each command its own pipe.
var stdinLines = struct {
	file   *os.File
	reader *bufio.Reader
}{}

func readStdinLine() (string, error) {
	if stdinLines.file != os.Stdin {
		stdinLines.file = os.Stdin
		stdinLines.reader = bufio.NewReader(os.Stdin)
	}
	line, err := stdinLines.reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func readSecret(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(fd) {
		raw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	// The non-terminal fallback, recorded warning included.
	fmt.Fprintln(os.Stderr, "Warning: Password input may be echoed.")
	return readStdinLine()
}

func usersAddCommand() *cobra.Command {
	var email string
	var admin, noPassword bool
	cmd := &cobra.Command{
		Use:   "add <username>",
		Short: "Create an account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			password := ""
			if !noPassword {
				var err error
				if password, err = promptNewPassword(args[0]); err != nil {
					return err
				}
			}
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			user, err := auth.Create(ctx, db, args[0], email, admin)
			if err != nil {
				// Named rather than blanket — exactly these three: a
				// genuine fault should stay an error, not read as "refused:".
				if errors.Is(err, auth.ErrUserExists) ||
					errors.Is(err, auth.ErrInvalidUsername) ||
					errors.Is(err, auth.ErrInvalidEmail) {
					return refused("%s", err)
				}
				return err
			}
			if password != "" {
				if _, err := auth.SetPassword(ctx, db, user.ID, password); err != nil {
					return err
				}
			}
			role := ""
			if user.IsAdmin {
				role = " (admin)"
			}
			fmt.Printf("  created %s%s in %s\n", user.Username, role, config.AppDBPath())
			if password == "" {
				fmt.Println("  no password set -- this account cannot log in yet.")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "the account's address (for invites and resets)")
	cmd.Flags().BoolVar(&admin, "admin", false, "grant admin")
	cmd.Flags().BoolVar(&noPassword, "no-password", false,
		"create the account unclaimed; it cannot log in until a password is set")
	return cmd
}

func usersInviteCommand() *cobra.Command {
	var username string
	var admin bool
	cmd := &cobra.Command{
		Use:   "invite <email>",
		Short: "Create an unclaimed account and mail its owner a claim link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			address, err := auth.NormaliseEmail(args[0])
			if err != nil {
				return refused("%s", err)
			}
			if address == "" {
				return refused("an invite needs an email address to send to")
			}
			sender, err := auth.SenderFromEnv(nil)
			if err != nil {
				return refused("%s", err)
			}
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			user, err := auth.GetByEmail(ctx, db, address)
			if err != nil {
				return err
			}
			if user != nil {
				has, err := auth.HasPassword(ctx, db, user.ID)
				if err != nil {
					return err
				}
				if has {
					return refused("%s has already claimed that address -- "+
						"they can use the reset link on the sign-in page",
						user.Username)
				}
			} else {
				name := username
				if name == "" {
					// The address' local part, and a refusal rather than a
					// mangling when it will not do: an invited person has to
					// be told the handle they were given — unlike the
					// maintainer bootstrap, which may normalise its own.
					name, _, _ = strings.Cut(address, "@")
				}
				name, err := auth.NormaliseUsername(name)
				if err != nil {
					return refused("%s; pass --username", err)
				}
				user, err = auth.Create(ctx, db, name, address, admin)
				if errors.Is(err, auth.ErrUserExists) {
					return refused("the username %s is taken -- pass "+
						"--username to choose another", quoted(name))
				}
				if err != nil {
					return err
				}
			}
			if err := auth.SendInvite(ctx, db, user, sender, ""); err != nil {
				return refused("%s", err)
			}
			role := ""
			if user.IsAdmin {
				role = " (admin)"
			}
			fmt.Printf("  invited %s%s\n", user.Username, role)
			fmt.Println("  they choose their own password; the link works once and " +
				"expires in a week.")
			return nil
		},
	}
	cmd.Flags().StringVar(&username, "username", "",
		"the login handle (default: the address' local part)")
	cmd.Flags().BoolVar(&admin, "admin", false, "grant admin")
	return cmd
}

func usersListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Every account, its state and its sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			everyone, err := auth.AllUsers(ctx, db)
			if err != nil {
				return err
			}
			if len(everyone) == 0 {
				fmt.Printf("  no accounts in %s\n", config.AppDBPath())
				fmt.Println("  create one with `mtglab users add <name>`")
				return nil
			}
			fmt.Printf("  %-20s %-28s %-12s %-12s sessions\n",
				"username", "email", "state", "answered by")
			for _, user := range everyone {
				state := "no password"
				switch has, err := auth.HasPassword(ctx, db, user.ID); {
				case err != nil:
					return err
				case user.Disabled:
					state = "disabled"
				case has:
					state = "active"
				default:
					invited, err := auth.TokenOutstanding(ctx, db, user.ID,
						auth.PurposeInvite)
					if err != nil {
						return err
					}
					if invited {
						state = "invited"
					}
				}
				live, err := auth.CountSessionsForUser(ctx, db, user.ID)
				if err != nil {
					return err
				}
				star := " "
				if user.IsAdmin {
					star = "*"
				}
				email := user.Email
				if email == "" {
					email = "-"
				}
				fmt.Printf(" %s%-20s %-28s %-12s %-12s %d\n", star,
					user.Username, email, state,
					tiers.Get(user.ModelTier).Label, live)
			}
			fmt.Println("\n  * admin")
			return nil
		},
	}
}

func usersPasswdCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "passwd <username>",
		Short: "Set a password; ends every session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			user, err := auth.Get(ctx, db, args[0])
			if err != nil {
				return err
			}
			if user == nil {
				return refused("no account %s", quoted(args[0]))
			}
			password, err := promptNewPassword(user.Username)
			if err != nil {
				return err
			}
			ended, err := auth.SetPassword(ctx, db, user.ID, password)
			if err != nil {
				return err
			}
			fmt.Printf("  password set for %s\n", user.Username)
			if ended > 0 {
				fmt.Printf("  %d session(s) ended -- every device signs in again.\n", ended)
			}
			return nil
		},
	}
}

func setDisabledCommand(use, short string, disabled bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <username>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			user, err := auth.Get(ctx, db, args[0])
			if err != nil {
				return err
			}
			if user == nil {
				return refused("no account %s", quoted(args[0]))
			}
			ended, err := auth.SetDisabled(ctx, db, user.ID, disabled)
			if errors.Is(err, auth.ErrLastAdmin) {
				// The likelier of the two doors to a lockout (ADR 17).
				return refused("%s", err)
			}
			if err != nil {
				return err
			}
			word := "enabled"
			if disabled {
				word = "disabled"
			}
			fmt.Printf("  %s is now %s\n", user.Username, word)
			if ended > 0 {
				fmt.Printf("  %d session(s) ended.\n", ended)
			}
			return nil
		},
	}
}

func usersDisableCommand() *cobra.Command {
	return setDisabledCommand("disable", "Shut an account off; ends its sessions", true)
}

func usersEnableCommand() *cobra.Command {
	return setDisabledCommand("enable", "Turn a disabled account back on", false)
}

func setAdminCommand(use, short string, isAdmin bool) *cobra.Command {
	return &cobra.Command{
		Use:   use + " <username>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			user, err := auth.Get(ctx, db, args[0])
			if err != nil {
				return err
			}
			if user == nil {
				return refused("no account %s", quoted(args[0]))
			}
			already := "not an admin"
			if isAdmin {
				already = "an admin"
			}
			if user.IsAdmin == isAdmin {
				fmt.Printf("  %s is already %s\n", user.Username, already)
				return nil
			}
			if err := auth.SetAdmin(ctx, db, user.ID, isAdmin); err != nil {
				if errors.Is(err, auth.ErrLastAdmin) {
					return refused("%s", err)
				}
				return err
			}
			admins, err := auth.UsableAdminIDs(ctx, db)
			if err != nil {
				return err
			}
			claimed, err := auth.HasPassword(ctx, db, user.ID)
			if err != nil {
				return err
			}
			fmt.Printf("  %s is now %s\n", user.Username, already)
			if isAdmin && !claimed {
				// The case where the command appears to have worked and the
				// instance still has nobody who can administer it.
				fmt.Println("  they have no password yet, so they cannot sign in to use it.")
			}
			fmt.Printf("  %d admin(s) can sign in.\n", len(admins))
			return nil
		},
	}
}

func usersPromoteCommand() *cobra.Command {
	return setAdminCommand("promote", "Grant admin", true)
}

func usersDemoteCommand() *cobra.Command {
	return setAdminCommand("demote", "Revoke admin", false)
}

func usersTierCommand() *cobra.Command {
	var tier string
	cmd := &cobra.Command{
		Use:   "tier <username>",
		Short: "Choose which Claude answers an account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			// `default` clears the grant rather than naming the default tier,
			// so "nobody has chosen anything" has one spelling in the column
			// whichever door it came through.
			wanted := tier
			if wanted == "default" {
				wanted = ""
			}
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			user, err := auth.Get(ctx, db, args[0])
			if err != nil {
				return err
			}
			if user == nil {
				return refused("no account %s", quoted(args[0]))
			}
			if err := auth.SetModelTier(ctx, db, user.ID, wanted); err != nil {
				if errors.Is(err, auth.ErrUnknownTier) {
					keys := make([]string, 0, len(tiers.All))
					for _, t := range tiers.Roster() {
						keys = append(keys, t.Key)
					}
					return refused("no such tier %s -- one of: default, %s",
						quoted(tier), strings.Join(keys, ", "))
				}
				return err
			}
			chosen := tiers.Get(wanted)
			fmt.Printf("  %s is answered by %s\n", user.Username, chosen.Label)
			fmt.Printf("  %s\n", chosen.Blurb)
			return nil
		},
	}
	cmd.Flags().StringVar(&tier, "tier", "default",
		"a tier key, or `default` to clear the grant")
	_ = cmd.MarkFlagRequired("tier")
	return cmd
}

func usersDeleteCommand() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <username>",
		Short: "Delete an account for good; confirm by typing the name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			db, err := connectUsers(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()
			user, err := auth.Get(ctx, db, args[0])
			if err != nil {
				return err
			}
			if user == nil {
				return refused("no account %s", quoted(args[0]))
			}
			live, err := auth.CountSessionsForUser(ctx, db, user.ID)
			if err != nil {
				return err
			}
			role := ""
			if user.IsAdmin {
				role = " (admin)"
			}
			fmt.Printf("  %s%s, %d session(s)\n", user.Username, role, live)
			fmt.Println("  deleted for good -- there is no undo and no trash")
			if !yes {
				fmt.Printf("  type '%s' to delete it: ", user.Username)
				line, _ := readStdinLine()
				// Usernames are ASCII by the handle pattern, so a simple fold
				// and a full Unicode casefold cannot disagree about a match.
				if !strings.EqualFold(strings.TrimSpace(line), user.Username) {
					return refused("that is not the username")
				}
			}
			ended, err := auth.Delete(ctx, db, user.ID)
			if errors.Is(err, auth.ErrLastAdmin) {
				// The third and worst door to a lockout: this one cannot be
				// walked back at all.
				return refused("%s", err)
			}
			if err != nil {
				return err
			}
			fmt.Printf("  %s is gone\n", user.Username)
			if ended > 0 {
				fmt.Printf("  %d session(s) ended.\n", ended)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation (scripts)")
	return cmd
}

// quoted wraps what somebody typed for a refusal sentence — single quotes,
// the pair swapped to double when the text itself carries a single — the
// recorded spelling of every "no account ..." answer.
func quoted(s string) string {
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	return "'" + strings.ReplaceAll(s, `\`, `\\`) + "'"
}
