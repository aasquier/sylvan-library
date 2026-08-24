package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/aasquier/sylvan-library/go/internal/config"
	"github.com/aasquier/sylvan-library/go/internal/wire"
)

// The maintainer, reconciled to admin at every start.
// ADR 17.
//
// A fresh instance has an empty `users` table, which means no session can be
// created, which means any bootstrap that runs "as an admin" is circular. The
// way out of that circle is configuration: `MTGLAB_ADMIN_EMAIL` names the
// maintainer, and `EnsureMaintainer` makes three things true of that address
// every time the app or a `mtglab users` command starts.
//
//   - The account **exists** -- created unclaimed (`password_hash IS NULL`) if
//     it did not, which is ADR 16's own shape for an account whose password
//     nobody else has ever seen. Its handle comes from `MTGLAB_ADMIN_USERNAME`,
//     or is derived from the address when that is unset.
//   - It is an **admin**.
//   - It is **not disabled**.
//
// Those are the three it *reconciles*. The username is not among them: it is
// used when the account is created and never afterwards, because a handle
// appears in URLs and in `mtglab users list` output, and changing somebody's
// at boot is a surprise this has no way to warn them about.
//
// Reconciling rather than only creating is the whole point, and it is the
// difference between this and the first-account-wins option ADR 17 rejected.
// A demotion, an accidental disable, or a backup restored from before the
// account was made an admin are all repaired by a restart -- an instance whose
// maintainer has been locked out is one reboot from being administrable again,
// with no shell.
//
// **Nothing here sends mail.** A boot that depends on a mail provider is a
// boot that fails when the provider does, and the account does not need one to
// become usable: `SendReset` serves unclaimed accounts deliberately, so the
// maintainer claims a bootstrapped account from the sign-in page's reset link,
// or over `mtglab users invite` from a shell. That also keeps this file inside
// the package rule that no test sends mail.
//
// Unset, `EnsureMaintainer` does nothing and touches no file. `mtglab ui` on a
// laptop has no accounts at all and must not acquire one as a side effect of
// being run.
//
// The read half -- the maintainer's *handle*, per request, writing nothing --
// is `library.MaintainerUsername`, which lives with the deck reads and
// deliberately stays there: a reader that lives beside the write would be one
// import away from becoming it.

// UsernameFor is a login handle from an address' local part, sanitised to fit.
//
// The fallback when `MTGLAB_ADMIN_USERNAME` is unset, and a guess:
// `ada.lovelace@example.com` becomes `ada.lovelace`, which is usually the name
// its owner would have picked and sometimes is not. Set the variable when it
// is not.
//
// `mtglab users invite` has the same problem and answers it differently -- it
// refuses and asks for `--username`, because an invited person has to be told
// the handle they were given and one invented by a mangling rule is a bad
// thing to have to explain.
//
// Here there is nobody to ask. This runs unattended at boot, and a refusal
// would mean an instance with no admin because an address had a `+` in it. So
// it mangles, deterministically, and the last resort is `admin`.
func UsernameFor(email string) string {
	local, _, _ := strings.Cut(email, "@")
	var kept strings.Builder
	for _, c := range local {
		// The keep-filter is Unicode-wide (letters and every numeric
		// category) on purpose: a `ü` is *kept* here and then fails
		// UsernamePattern's ASCII alphabet below -- landing on "admin", the
		// recorded fallback. An ASCII-only filter would instead strip the
		// letter and mint a handle this rule never minted before.
		if unicode.IsLetter(c) || unicode.IsNumber(c) || strings.ContainsRune("._-", c) {
			kept.WriteRune(c)
		}
	}
	cleaned := strings.TrimLeft(kept.String(), "._-")
	name, err := NormaliseUsername(cleaned)
	if err != nil {
		return "admin"
	}
	return name
}

// wantedUsername is the configured handle if there is a usable one, else
// derived from email.
//
// A malformed `MTGLAB_ADMIN_USERNAME` is logged and ignored rather than
// fatal, for the same reason a malformed address is: an instance that refuses
// to start because a preference is misspelled has turned a cosmetic problem
// into an outage.
func wantedUsername(configured, email string) string {
	if configured == "" {
		return UsernameFor(email)
	}
	name, err := NormaliseUsername(configured)
	if err != nil {
		slog.Default().Error(fmt.Sprintf("MTGLAB_ADMIN_USERNAME is unusable (%s); "+
			"deriving the handle from the address instead", err))
		return UsernameFor(email)
	}
	return name
}

// uniqueUsername is wanted, or wanted2, wanted3... if somebody already has it.
//
// Only reachable when the configured address is new *and* its handle collides
// with an existing account -- a friend invited as `aaron` before the
// maintainer was configured. Renaming theirs would be worse.
func uniqueUsername(ctx context.Context, db *sql.DB, wanted string) (string, error) {
	taken, err := Get(ctx, db, wanted)
	if err != nil {
		return "", err
	}
	if taken == nil {
		return wanted, nil
	}
	for suffix := 2; suffix < 100; suffix++ {
		candidate := fmt.Sprintf("%s%d", wanted, suffix)
		taken, err := Get(ctx, db, candidate)
		if err != nil {
			return "", err
		}
		if taken == nil {
			return candidate, nil
		}
	}
	return "", failf("%w: no free username near %s", ErrUserExists, wire.Quote(wanted))
}

// EnsureMaintainer makes the configured address an enabled admin. A no-op when
// `MTGLAB_ADMIN_EMAIL` is unset.
//
// Idempotent, and silent when there is nothing to do: the steady state is
// every boot after the first finding the account already correct and writing
// no log line. Every *change* is logged, because a reconciliation that
// silently promotes an account is one nobody can audit after the fact.
//
// The two malformed-configuration cases return nil rather than an error --
// logged, then carried past: refusing to start would turn a typo
// in one environment variable into an instance that serves nothing, when the
// app is perfectly capable of running while its admin is misconfigured. A
// database failure is still an error -- that one is about the volume, not the
// preference.
func EnsureMaintainer(ctx context.Context, db *sql.DB, cfg config.Config) error {
	address := cfg.AdminEmail
	if address == "" {
		return nil
	}

	normalised, err := NormaliseEmail(address)
	if err != nil {
		// Loud, and not fatal. The address is the maintainer's own and is
		// already in the deployment config, so it is the one address ADR 16's
		// no-logging rule is not protecting from the maintainer.
		slog.Default().Error(fmt.Sprintf("MTGLAB_ADMIN_EMAIL=%s is not an email "+
			"address; no maintainer account was reconciled", wire.Quote(address)))
		return nil //nolint:nilerr // a malformed preference is logged and skipped, never fatal — the reconciler's standing rule
	}
	if normalised == "" { // unreachable: the `if address == ""` above
		return nil
	}

	account, err := GetByEmail(ctx, db, normalised)
	if err != nil {
		return err
	}
	if account == nil {
		wanted := wantedUsername(cfg.AdminUsername, normalised)
		name, err := uniqueUsername(ctx, db, wanted)
		if err != nil {
			return err
		}
		if name != wanted {
			slog.Default().Warn(fmt.Sprintf("the handle %s is taken, so the "+
				"maintainer account is %s -- rename the other account if that "+
				"is wrong", wire.Quote(wanted), wire.Quote(name)))
		}
		created, err := Create(ctx, db, name, normalised, true)
		if err != nil {
			return err
		}
		slog.Default().Warn(fmt.Sprintf("created maintainer account %s from "+
			"MTGLAB_ADMIN_EMAIL -- it has no password yet; use the reset link "+
			"on the sign-in page or `mtglab users invite`",
			wire.Quote(created.Username)))
		return nil
	}

	if !account.IsAdmin {
		// Promotion never trips the LastAdmin guard; only revocation is
		// checked, so this cannot refuse.
		if err := SetAdmin(ctx, db, account.ID, true); err != nil {
			return err
		}
		slog.Default().Warn(fmt.Sprintf("promoted %s to admin from "+
			"MTGLAB_ADMIN_EMAIL", wire.Quote(account.Username)))
	}
	if account.Disabled {
		// Re-enabling is the break-glass half, and it is deliberate: whoever
		// can set this variable can already deploy code to the instance, so it
		// confers nothing new -- and without it a maintainer disabled by
		// accident has no way back that does not involve SSH.
		if _, err := SetDisabled(ctx, db, account.ID, false); err != nil {
			return err
		}
		slog.Default().Warn(fmt.Sprintf("re-enabled %s from MTGLAB_ADMIN_EMAIL",
			wire.Quote(account.Username)))
	}
	return nil
}
