# 17. The maintainer is named in the environment, and admin routes live behind a prefix

**Status:** Accepted · **Decided:** 2026-08-12 · **Implemented:** 2026-08-12

Builds on [ADR 5](0005-sessions-over-jwts-and-no-self-signup.md) and
[ADR 16](0016-accounts-are-invited-and-passwords-are-self-served.md) and
supersedes neither. ADR 5 decided sessions, the scoped accessor and the
adversarial isolation test; ADR 16 decided that accounts are invited and
passwords self-served. Both left the same hole, and this record fills it:
`is_admin` has been a column on `users` since the auth core landed, it is
carried on `UserScope` and reported by `/api/auth/me`, and **no route or
function has ever read it to decide anything.**

## Context

`docs/HOSTING.md` §7 has carried two entries since 2026-08-12: an admin UI with
authorization that means something, and "the maintainer is always an admin, on
every instance." The second is stated there as a *requirement*, and nothing in
the code guarantees it.

Three separate problems hide inside that sentence, and they are easier to see
apart than together:

1. **A fresh instance has no way in.** An empty `users` table means no session
   can ever be created, so any bootstrap that runs "as an admin" is circular.
   Today the answer is `fly ssh console` and `mtglab users add --admin`, which
   works and is a manual step nobody performs twice the same way.
2. **`set_admin` and `set_disabled` will each remove the last admin an instance
   has**, cheerfully and in one call. The maintainer is the only admin on this
   deployment by design, so both are one keystroke from a lockout whose only
   remedy is SSH.
3. **`is_admin` has no teeth.** An admin differs from any other account only in
   what `mtglab users list` prints beside their name.

The third is the one with the security argument in it, because the fix has to
survive the route somebody adds in a year. That is the same problem `api/auth.py`
already solved once for authentication, and the reasoning transfers whole: a
per-route dependency has to be remembered, and the route that will not have it
is the one nobody is thinking about while writing this.

## Options considered

### How the first admin is minted

**First account wins** — whichever account is created on an empty `users` table
becomes an admin. No new configuration, nothing to set on deploy day, and it
cannot be forgotten. Rejected on two counts. It is a **one-time event, not an
invariant**: it says nothing about the instance a week later, so it cannot
restore an admin who was demoted, and "always an admin, on every instance" is a
standing property rather than a moment. And it is silently wrong in a plausible
order of operations — `mtglab users invite friend@example.com` typed on a fresh
volume before the maintainer's own account exists makes the friend an admin,
with no error and no output that says so.

**`MTGLAB_ADMIN_EMAIL`** — an environment variable names the maintainer's
address, and the app reconciles that account to admin at every start. Chosen.
It survives the volume being recreated, because the configuration lives in
`fly.toml` and the platform's secrets rather than in the database it is about.
It is re-asserted on every boot rather than once, which is what makes it a
guarantee. And a restart is a recovery path that needs no shell.

**Both — the variable when set, first-account-wins when not.** Rejected, and it
is the option that looks most reasonable at first. It is two bootstrap paths to
reason about and to test, and the first-account footgun above still exists on
every instance where the variable is unset, which is every instance where
somebody would most want the safety net.

### What an admin route answers a non-admin

**404, per ADR 5.** That rule exists for resources that belong to one person: a
403 on `/api/jobs/{id}` confirms the id names something real, which is the fact
being protected. It does not transfer here. An admin route's *existence* is
public knowledge — this is a public repository, and the paths are in it — and
whether the caller is an admin is a fact the caller can already read off
`/api/auth/me`. A 404 would therefore protect nothing and cost every future
debugging session the difference between "not logged in", "not an admin" and
"typo in the path".

**403.** Chosen, with the reasoning above written into the code next to it so
the ADR 5 rule is not read as having been forgotten.

### Whether an admin sees email addresses

CLAUDE.md rule 5 and ADR 16 keep addresses out of logs, artifacts and Claude
tool results, and `User.as_dict()` omits the address unless asked. Until now the
only caller that asked was `mtglab users list`, printing to the maintainer's own
terminal.

**Admins see addresses.** Chosen, by the maintainer, on 2026-08-12. An account
whose only visible identity is a username defaulted from the local part of an
address is one the maintainer cannot confidently match to the person who asked
for it, and invites are keyed by address. The trade is accepted knowingly: the
admin list is now a second place addresses travel, and the constraint that
replaces "one caller" is narrower and still checkable — **an address may be
serialised only into a response that an admin authenticated for.**

## Decision

**One: `MTGLAB_ADMIN_EMAIL` names the maintainer, and the app reconciles that
account to admin every time it starts.**

`auth/bootstrap.py` runs on app startup and before any `mtglab users` command.
Unset, it does nothing at all, which is what a laptop wants. Set, it makes three
things true of the named address and logs whichever it had to change:

- the account exists — created **unclaimed** (`password_hash IS NULL`) if it did
  not, which is ADR 16's own shape for an account whose password nobody else has
  ever seen;
- it is an admin;
- it is not disabled.

`MTGLAB_ADMIN_USERNAME` names the handle that account is created with, because
deriving one from the address' local part is a guess and sometimes the wrong
one. It is deliberately **not** reconciled: a username appears in URLs and in
`mtglab users list`, and renaming somebody at boot is a surprise nothing here
could warn them about, so an account found by address keeps the name it has.

**There is no `MTGLAB_ADMIN_PASSWORD` and there will not be one.** ADR 16 is
unconditional that no password is ever chosen by one person for another, and a
password in the environment is additionally a password in `fly secrets list`,
in a process listing, and in whatever file it was pasted into on the way there.
The bootstrapped account is unclaimed; its holder chooses the password.

**No mail is sent at boot.** A boot that depends on a mail provider is a boot
that fails when the provider does, and the account is reachable without one:
`send_reset` already serves unclaimed accounts deliberately, so the maintainer
claims a bootstrapped account from the sign-in page's reset link, or over
`mtglab users invite` from a shell.

Reconciling rather than only creating is the whole point. A demotion, an
accidental disable, or a restored backup from before the account was made an
admin are all repaired by a restart.

**Two: neither the CLI nor any route may remove the last admin.**

`users.set_admin(..., False)` and `users.set_disabled(..., True)` refuse when
the target is the last admin who can actually sign in, raising `LastAdmin`. It
lives in `auth/users.py` — in the core, not in a handler — so the CLI, the admin
routes and anything written later inherit it rather than each remembering it.
The check and the write share one `BEGIN IMMEDIATE` transaction, because a rule
enforced by a read followed by an unrelated write is a rule two concurrent
callers can walk through together.

"Can actually sign in" means enabled **and** holding a password. An instance
whose only remaining admin is an unclaimed invite is locked out exactly as
thoroughly as one with no admin at all, and a guard that counted it would be a
guard that reports success while the lockout happens.

**Three: admin routes live under `/api/admin`, and the middleware enforces the
prefix before routing.**

The same mechanism and the same argument as `PUBLIC_PATHS`. A path under the
prefix is refused to a non-admin without the handler being consulted, so a new
admin route is protected by where it is mounted rather than by what its author
remembered. The route functions additionally depend on `deps.Admin`, which is
belt to the middleware's braces and the only protection an admin route mounted
outside the prefix by mistake would have.

`tests/test_isolation.py` gains a fourth classification, `ADMIN`, alongside
public, shared and user-scoped. It is generated from the route table like the
rest: every route under the prefix must be classified `ADMIN` and every `ADMIN`
route must be under the prefix, so the two ways of getting it wrong — an admin
route filed as shared, and an admin route mounted somewhere the middleware does
not look — are both failures rather than reviews. The sweep then logs in as a
non-admin and requires 403 from each.

## Consequences

**A fresh deploy has one more thing to set**, and getting it wrong is visible:
no `MTGLAB_ADMIN_EMAIL` means no admin, which means the admin page 403s and the
remedy is the documented `fly ssh console` path. That is the failure mode this
trades for, and it is loud.

**The variable is now a credential-adjacent setting.** Anybody who can set it
can make themselves an admin on the next boot. That is true of anybody who can
edit `fly.toml` or run `fly secrets`, who can already deploy arbitrary code to
the instance, so it grants nothing new — but it means the value belongs in the
same care as the rest of the deployment configuration, and it is why the
reconciliation writes a log line every time it changes something.

**The maintainer cannot demote or disable themselves through any surface** while
they are the only admin. The intended way to hand the instance over is to
promote the successor first; the ordering is forced, which is the point.

**`mtglab users` stays**, and this ADR is a reason rather than an obstacle. It
is the bootstrap path when a volume is fresh, the break-glass path when mail is
misconfigured, and the only path that works when the admin page is what is
broken. `promote` and `demote` land with this record, because "a way to grant it
after the fact" was the third thing §7 asked for and `set_admin` had no caller.

**An address now reaches an HTTP response.** The rule that replaces "one caller"
is stated above and pinned by a test: `include_email=True` is reachable from
`mtglab users list` and from admin-authenticated responses, and from nothing
else. If a third caller ever appears, that test is where the argument has to be
made again.

**What this does not do is give the browser a way in.** There is still no login
screen and no claim page (`docs/HOSTING.md` §6 step 5c). With auth on, the admin
page is behind a login that does not exist yet; with auth off — every way the
app is actually run today — `LOCAL` is an admin and the page works. That is the
right order: the enforcement is what a login screen would otherwise ship
untested behind it.
