package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/aasquier/sylvan-library/go/internal/config"
)

// Issue a link and deliver it. The one
// implementation both entry points call.
//
// `mtglab users invite` and `POST /api/auth/reset` are the two doors ADR 16
// describes, and this is what is behind both of them, because the ADR is
// explicit that a second path is how one of the two ends up weaker. What
// differs between them is a Purpose, a subject line and a sentence; the token
// rules and the link are shared by construction.
//
// **The token travels in the URL fragment**, not the query string:
//
//	https://example.com/auth/claim#token=<43 characters>
//
// A fragment is never sent to a server. The query-string spelling would put a
// live credential into the access log of every hop that serves the page -- the
// platform's router, any proxy in front of it, and the `Referer` header of
// anything the page later loads. The frontend reads `location.hash` and posts
// the token to `/api/auth/claim`, which is the only request that carries it,
// and that request is a POST with a JSON body rather than a URL.
//
// `SendReset` takes an address rather than an account **and reports nothing
// about the outcome either way**. That is not indifference to the result -- it
// is the shape that makes ADR 16's "the reset endpoint answers identically
// whether or not the address exists" hard to get wrong. A handler that cannot
// see whether the lookup hit cannot branch on it, so the identical response is
// a property of this signature rather than a rule somebody has to remember
// while editing the route.

// ClaimPath is where an emailed link lands.
const ClaimPath = "/auth/claim"

// ifTheLinkFails goes in both messages, because the failure is in neither of
// them.
//
// **Some mail apps drop the `#...` when you click**, and nothing about that
// reaches the server: the fragment is client-side by design, so a stripped
// link looks exactly like no link at all, and re-sending produces one that
// fails identically. Seen on the deployed instance 2026-08-13, where the
// *visible* URL in a plain-text reset was whole and the click arrived with an
// empty hash.
//
// So the recovery has to travel with the message rather than live on the page
// somebody cannot reach. The claim screen takes a pasted address; this is the
// sentence that tells them to try it. Worth the four lines: without it the
// person is locked out and cannot say why.
const ifTheLinkFails = "If that opens a page asking for a link rather than a password box, copy\n" +
	"the whole address above -- including the part after the # -- and paste it\n" +
	"into your browser instead. Some mail apps cut the link short.\n"

// ClaimLink is the URL that goes in the message. See the file comment for the
// `#`.
func ClaimLink(token, baseURL string) string {
	base := baseURL
	if base == "" {
		base = config.BaseURL()
	}
	return strings.TrimRight(base, "/") + ClaimPath + "#token=" + token
}

func inviteMessage(to, link string) Message {
	return Message{
		To:      to,
		Subject: "Your sylvan-library account",
		Body: "Somebody has set up an account for you on sylvan-library, a " +
			"Commander deckbuilding and simulation tool.\n" +
			"\n" +
			"Choose a password to finish setting it up:\n" +
			"\n" +
			link + "\n" +
			"\n" +
			ifTheLinkFails +
			"\n" +
			"The link works once and expires in a week. Nobody, including " +
			"whoever invited you, ever sees the password you pick.\n" +
			"\n" +
			"If you were not expecting this, you can ignore it -- the account " +
			"cannot be used until somebody follows that link.\n",
	}
}

func resetMessage(to, link string) Message {
	return Message{
		To:      to,
		Subject: "Reset your sylvan-library password",
		Body: "Somebody asked to reset the password on your sylvan-library " +
			"account.\n" +
			"\n" +
			"Choose a new one here:\n" +
			"\n" +
			link + "\n" +
			"\n" +
			ifTheLinkFails +
			"\n" +
			"The link works once and expires in an hour. Setting a new " +
			"password signs you out everywhere.\n" +
			"\n" +
			"If this was not you, nothing has changed and you can ignore this " +
			"message.\n",
	}
}

// SendInvite issues an invite for an existing unclaimed account and mails the
// link.
//
// The account is created by the caller rather than here, because `users
// invite` has decisions to make about it -- the username, the admin flag --
// that a reset does not.
func SendInvite(ctx context.Context, db *sql.DB, user *User, sender EmailSender,
	baseURL string) error {

	if !user.HasEmail || user.Email == "" {
		return fmt.Errorf("an invite needs an address to send to")
	}
	token, err := IssueToken(ctx, db, user.ID, PurposeInvite)
	if err != nil {
		return err
	}
	return sender.Send(inviteMessage(user.Email, ClaimLink(token, baseURL)))
}

// SendReset mails a reset link, if that address resolves to an account that
// can use one.
//
// It reports nothing about the outcome in the three cases where no message
// goes out: no such address, a disabled account, and an address that is not
// shaped like one at all. The only error it can return is one the *sender*
// produced, which is a fact about the mail provider rather than about the
// mailbox -- and the public endpoint drops even that, after the response has
// already gone.
//
// **A disabled account gets no reset link.** Disabling is the maintainer's
// revocation lever, and one the disabled party can undo from their own inbox
// is not a lever. `RedeemToken` refuses them too, so this is defence in depth
// rather than the only check.
//
// An *unclaimed* account does get one, and that is deliberate: somebody whose
// invite expired asking for a reset is the same request in different words,
// and refusing it would leave them with nothing to do but email the
// maintainer.
func SendReset(ctx context.Context, db *sql.DB, email string, sender EmailSender,
	baseURL string) error {

	account, err := GetByEmail(ctx, db, email)
	if err != nil {
		if errors.Is(err, ErrInvalidEmail) {
			// Not shaped like an address, so it resolves to nobody -- the same
			// outcome as an address that simply has no account, and it gets the
			// same silence.
			return nil
		}
		return err
	}
	if account == nil || account.Disabled || !account.HasEmail {
		return nil
	}
	token, err := IssueToken(ctx, db, account.ID, PurposeReset)
	if err != nil {
		return err
	}
	return sender.Send(resetMessage(account.Email, ClaimLink(token, baseURL)))
}
