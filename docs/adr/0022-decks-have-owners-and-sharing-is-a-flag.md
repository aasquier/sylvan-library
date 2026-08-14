# 22. Decks have owners, and sharing is a flag on the deck

**Status:** Accepted · **Recorded:** 2026-08-14

## Context

[#80](https://github.com/aasquier/sylvan-library/pull/80) stopped an invited
account editing the curated six, by deriving `FileDeckSource.writable` from
`caller.is_admin`. That was the right emergency fix and it is not the end
state, because it left the app **read-only for everybody who is not the
maintainer** — the three "start a deck" doors included. There is nowhere for
another person's deck to live.

What was asked for, 2026-08-14: people should be able to show each other their
decks; the maintainer's should always be visible; it should be **a tab somebody
opts into rather than something in the way**; other players' decks should be
organised **by username**; and shared decks should appear in the searches this
project builds later. Leaderboards and macro deck statistics are named as work
on top of this, not part of it.

This is [ADR 4](0004-two-embedded-databases.md)'s second deck tier arriving with
a second requirement attached, and it changes what three earlier decisions mean
in practice:

- **[ADR 1](0001-deck-yaml-in-git-is-the-source-of-truth.md)** — `deck.yaml` in
  git is the source of truth. Still true, and still true *only* of the curated
  six. Somebody else's deck has never been in that repo and must not be.
- **[ADR 5](0005-sessions-over-jwts-and-no-self-signup.md)** — another
  account's resource answers **404, not 403**, so ids cannot be probed.
- **#80** chose **403** for a refused deck write, because a curated deck's
  existence is not a secret: it is in a public repository and `GET /api/decks`
  had just listed it to that caller.

Those last two look contradictory and are the hard part of this decision.

## Options considered

### How a deck is addressed

**A global slug namespace**, leaving all nineteen `/api/decks/{slug}` routes
untouched. Rejected, and the reason is precisely ADR 5. Creating a deck whose
slug is taken by somebody's **private** deck must either answer "taken" — which
publishes that a private deck by that name exists, the exact leak ADR 5 forbids
— or silently assign a suffix, where the slug that comes back still discloses it
by observation. There is no third answer, because a global namespace makes
"is this name free" a question about every account at once.

**An opaque deck id in the path**, with the slug demoted to a display name. No
collisions and no enumeration, and rejected for a different reason: it breaks
the correspondence between the slug and `decks/<slug>/deck.yaml`, which ADR 1
keeps *permanently* for the curated six. A readable URL is also most of what
"show somebody your deck" means.

### What sharing is

**A per-deck access list.** Rejected for now — it needs a join table, a
share-with-a-person flow, a management UI, and an answer for what happens when
a named account is deleted. It is more precision than "show each other your
decks" asked for, and it is additive later if it is ever wanted.

**Unlisted links** readable without a session. Rejected: it puts deck content
outside the auth boundary, which is the boundary CLAUDE.md rule 5 and ADR 4 both
lean on.

### Where a file-backed deck's owner is recorded

**A new `owner:` field in `deck.yaml`.** Rejected. It would put an instance's
account model into six files that are the *portable* half of this project, and
it means nothing at all on a laptop with auth off.

## Decision

**Every deck has exactly one owner, and a `shared` flag.**

**1. Paths are owner-qualified: `/api/decks/{owner}/{slug}`.** `owner` is a
username. Slugs are unique **per owner**, never globally, so "is this slug
free" is only ever a question about the caller's own decks and answers nothing
about anybody else's.

**2. Two tiers, as ADR 4 said, and ownership is a rule rather than a column
for the first one.** The curated six stay file-backed in git, permanently, and
they belong to **the maintainer account** — the one `MTGLAB_ADMIN_EMAIL` names
and `auth/bootstrap.py` reconciles. Nothing is written into the six deck files.
Everyone else's decks are rows in `user_decks` in `app.db`, holding the same
YAML, so `Deck.load`, the gate and the artifact generator work unchanged on
both tiers. One parser, one validator, two sources.

**3. `shared` defaults differently per tier, and both defaults are the safe
one.** The curated six are **shared by default** — they are the showcase, and
that is what "the maintainer's should always be visible" means. A deck created
in the SQL tier is **private by default**, because `decks import` writes a
`stage: draft` with an empty `why` on all 99 cards, and publishing that the
instant it exists is not a thing anybody asked for.

**4. Visibility, and this is where ADR 5 and #80 stop contradicting each
other.** The rule is one sentence:

> **403 is only ever an answer about a deck the caller can already read.**

| Caller | Deck | Read | Write |
| --- | --- | --- | --- |
| The owner | their own | 200 | 200 |
| Anybody signed in | shared, someone else's | 200 | **403** |
| Anybody signed in | **private**, someone else's | **404** | **404** |
| No session | anything | 401, before routing | 401 |

A private deck is invisible, so every verb against it answers 404 — including
the writes, because a 403 there would confirm it exists and that is the leak
ADR 5 names. A shared deck is deliberately visible, so refusing a write with
404 would be a lie the caller can immediately disprove by reading it; 403 is
the honest answer and it is #80's answer, unchanged. **An unknown owner is a
404 for the same reason a private deck is** — otherwise the owner segment
enumerates the account list.

**5. The write gate generalises from `is_admin` to an owner comparison.**
`deps.deck_source` is still the one place that decides, which is what #80 built
it to be. `writable` becomes `deck.owner_id == caller.user_id` rather than
`caller.is_admin`; administering the instance does not confer editing somebody
else's decks, which is the property `tests/test_isolation.py` already asserts
for jobs.

**6. With auth off nothing changes for the person at the laptop.** `LOCAL`
gains the username `local` and owns every deck it can see. One person holding
the file the app reads is what that scope has always meant.

## Consequences

- **Nineteen route signatures and `web/src/lib/api.ts` change.** That is the
  bill for the namespace decision and it is mostly mechanical; the frontend's
  deck URLs are all built in one file, which is why the expensive option was
  affordable.
- **The CLI is untouched.** It operates on the file tier through
  `FileDeckSource` and never sees an owner segment, so `mtglab decks swap` and
  the artifact generator work exactly as before, on a laptop and over
  `fly ssh` alike.
- **`tests/test_isolation.py` gains a real subject.** Deck routes have been
  classified *shared* since they were written, which was correct about reading
  and silent about writing — the gap #80 fell through. A deck route is now
  **user-scoped** and checked adversarially: B asks for A's private deck and
  must get 404. This is the first time that generated sweep has had a
  user-owned resource other than a job to point at.
- **`shared` is a fourth deck-level field**, beside `status`, `stage` and the
  notes. For the file tier it may be set in `deck.yaml` and defaults to true;
  absent means shared, which is the opposite default from the SQL tier and is
  the same shape of argument `stage` makes — the six existing decks must never
  be silently hidden.
- **Browsing is by username**, which the path shape gives for free: the deck
  list groups under the owner it already names.
- **A rename breaks a URL.** Usernames are the readable key and #67 lets a
  holder choose theirs at claim time; renames after that are rare and the app
  builds its own links. Accepted rather than mitigated.
- **Deleting an account has to say what happens to its decks.** Out of scope
  here and it is now a real question, where before there were no decks to
  orphan. `ON DELETE CASCADE` is the default the schema will carry, matching
  `sessions` and `auth_tokens`.
- **Leaderboards and macro statistics get their subject.** They were asked for
  on top of this and they need an owner column to be answerable at all.
