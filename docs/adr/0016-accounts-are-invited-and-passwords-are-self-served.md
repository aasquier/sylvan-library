# 16. Accounts are invited, and passwords are self-served by email

**Status:** Proposed — no auth exists yet · **Decided:** 2026-08-12

**Supersedes the "no self-signup" half of
[ADR 5](0005-sessions-over-jwts-and-no-self-signup.md).** Everything else in
ADR 5 stands unchanged and is restated nowhere here: sessions over JWTs,
Argon2id at the OWASP minimum profile, the single scoped accessor, and the
adversarial isolation test are all still the decision. Read ADR 5 first; this
record changes exactly one thing.

## Context

ADR 5 decided that accounts would be provisioned by hand — `mtglab users add`,
`mtglab users passwd` — and argued the case well: no signup flow, no email
provider, no verification tokens, no reset tokens, no bot defence, no CAPTCHA,
no per-email cost. For fewer than a dozen personally-invited people that is a
real saving, and it is why the projected bill was near zero.

Asked for on 2026-08-12: username, password, **email setup and resets**, for
the hosted environment. That is a direct contradiction of the paragraph above,
so it gets a record rather than an edit.

The reason it is not merely a preference: "a forgotten password is the
maintainer running one command" is only cheap for the maintainer. For the
person locked out it is a message, a wait, and a password somebody else chose
and had to transmit. That last part is the substantive objection — **an
admin-set password is a password that has existed in plaintext in a chat
window.** The written plan's cheapest path is also the one that routes every
credential through an out-of-band channel nobody controls.

## Options considered

**Keep ADR 5 as written.** Rejected on the argument above, and because it makes
the maintainer a help desk for a system whose whole point is that friends can
use it without him.

**Cloudflare Access, and store no passwords at all.** Still the lowest-risk
option and still worth naming, because ADR 5 named it and nothing has changed
to weaken it: free to 50 users, a verified email in a header, no credential
store to breach. Rejected here for the reason it was rejected there — a login
flow the project does not control, and a hard dependency on Cloudflare sitting
in front of the whole app. Recorded again rather than quietly dropped, because
if the auth work below starts sprawling, this is the exit.

**Open self-signup with email verification.** Rejected. Sim jobs and the corpus
are expensive per user, and an open door needs rate limiting, bot defence and
an abuse story to protect something with no revenue behind it. The problem it
solves — strangers wanting accounts — still does not exist.

**Invite-only, with the password set and reset by the account holder.**
Chosen.

## Decision

**The maintainer creates accounts; the account holder owns the password.**

- `mtglab users invite <email>` creates a disabled account and issues a
  single-use, time-limited setup token. The invitee follows a link, sets their
  own password, and the account becomes usable. **No password is ever chosen by
  one person for another.**
- `POST /api/auth/reset` takes an email and, if it resolves, sends a
  single-use, time-limited reset link. Same token machinery as the invite —
  one implementation, two entry points, because a bespoke second path is how
  one of them ends up weaker.
- Email is sent through **Resend**, behind an `EmailSender` protocol with a
  console implementation for development and tests. **No test sends mail**, the
  same rule that keeps the Claude tests off the network.

Token rules, all of them load-bearing:

- **Stored hashed**, never in the clear, exactly as ADR 5 requires of session
  tokens and for the same reason: reading the database must not hand over live
  credentials.
- **Single use**, consumed on success.
- **Short-lived** — one hour for a reset, longer for an invite, which is a
  different risk because it grants nothing until used.
- **Changing a password invalidates every session for that user.** A reset is
  usually somebody suspecting compromise, and a reset that leaves the attacker
  logged in has answered the wrong question.
- **The reset endpoint answers identically whether or not the address exists**,
  and is rate-limited per address and per IP. ADR 5 already forbids leaking
  account existence through login timing; a reset form that says "no such user"
  gives away the same thing through the front door.

## Consequences

- **A transactional email provider is now a hard dependency of the deployment**
  — a `RESEND_API_KEY` in `fly secrets`, a verified sending domain, and
  deliverability as a thing that can break. `docs/HOSTING.md` §7 gains those
  items. This is precisely the infrastructure ADR 5 was proud of deleting, and
  it is the price.
- **The `EmailSender` seam is what keeps that dependency out of the tests.**
  The provider is one implementation; nothing above it knows the difference.
  If Resend is the wrong pick later, it is a class.
- **Email becomes user data**, held in `app.db`. Not corpus data and not
  collection data, so CLAUDE.md rule 5 does not forbid it — but it is the first
  personal data the project stores, and it inherits rule 5's reasoning about
  what a breach would mean. It must never reach a log line, an artifact, or a
  Claude tool result.
- The **invite** path, not the signup path, is what is built. Opening the door
  later is a flag and a rate limiter, not a rewrite, because the token
  machinery and the users table are the same either way.
- Nothing here changes ADR 5's isolation story, which remains the highest-value
  test in the auth work: log in as B, request A's resource, assert **404**.
