# 23. A green `main` deploys itself

**Status:** Accepted · **Decided:** 2026-08-14 · **Recorded:** 2026-08-14

## Context

Deploying has been a person running `fly deploy` from a laptop since the
instance went up on 2026-08-13 (`docs/HOSTING.md` §4 step 5). That worked
while deploying was a deliberate event. It stops working as soon as it is
routine, for a reason the 2026-08-14 quality pass demonstrated within an hour
of merging: `main` renamed the card pool's wire field and the running instance
kept answering `"corpus": true`, because nobody had deployed yet. **The deploy
that gets forgotten is the one where `main` and the instance quietly
disagree**, and nothing reports that disagreement — both halves look healthy.

The asymmetry is the point. A failing test is loud, a failing build is loud,
and a *skipped deploy* is silent by construction. It is discovered later, by
somebody debugging behaviour that the source no longer explains.

Three facts about this deployment shape constrain what is possible:

- **One machine, one volume.** `min_machines_running = 1` with a volume at
  `/data`, and a Fly volume attaches to exactly one machine. Fly cannot roll:
  a deploy stops the instance and starts the replacement. A few seconds of
  downtime is inherent, and was equally true by hand.
- **Migrations run on boot.** `auth/db.py` walks a schema ladder when the app
  starts. It does not walk back down.
- **The volume holds state that exists nowhere else** — the card pool, and
  every deck written by the app's editing routes. A deploy that came up
  against a fresh volume would look entirely healthy while having lost all of
  it.

## Options considered

### Keep deploying by hand

Nothing to build, and every deploy is watched by somebody who can react. But
it is the option that just failed: it depends on remembering, and the failure
mode is silence rather than an alarm. It also makes the deploy a small ritual
with a laptop and a `flyctl` install in it, which is exactly the kind of step
that does not happen on a Tuesday.

### Deploy on a tag or a release

More ceremony, and a real answer for a project with versioned consumers. This
one has a single instance and a single maintainer; a tag would be a second
bookkeeping step whose only job is to say "yes, really", which is what the
merge already said.

### Deploy on merge, behind a required approval

A GitHub Environment with a required reviewer, so each merge queues a deploy
that waits for a click. Genuinely safer for the migration case. Rejected
because it is a button either way, and a button that appears after every merge
is a button that gets clicked without reading — it buys ceremony rather than
scrutiny. The manual path below covers the case where somebody does want to
choose the moment.

### Deploy on merge, gated on the checks (chosen)

A `deploy` job in `ci.yml` that `needs` all four existing jobs and runs only
for a push to `main` or an explicit `workflow_dispatch`.

## Decision

**A push to `main` whose four checks are green deploys itself**, and there is
a manual button for the same workflow.

`needs: [test, frontend, no-secrets-or-card-data, image]` is the whole safety
argument, and it is worth being precise about why it is not redundant with
branch protection. Protection governs **merging**; it has nothing to say about
a `workflow_dispatch` run. `needs` makes the deploy job *structurally* unable
to start until every check has passed, whatever started the run.

The manual button runs the **whole workflow**, not the deploy job alone. A
button that skips the suite is a button that eventually ships something red.

`--local-only`, so the image builds on a runner that is already paid for
rather than on a Fly builder Machine that would be billed for. The image is
therefore built twice per run — once by `image` for the two-architecture
check, once here for the one architecture actually deployed. Collapsing them
means publishing to a registry, which `docs/ENGINEERING.md` §5 keeps behind a
signing and provenance conversation.

**A deploy is not done when `flyctl` exits.** The job then asserts three
things against the live instance, each pinning a distinct failure:

| Assertion | What its failure means |
| --- | --- |
| `/api/health` answers 200 | it is up at all |
| `"pool": true` | the volume is mounted and the card pool survived |
| `decks > 0` | the decks on the volume are still there |

The last two are the ones worth having. A fresh or unmounted volume answers
`"pool": false` — the correct state for a new instance and a catastrophe for
this one — and `deck.yaml` is written by the app's editing routes, so a deploy
that lost the volume loses edits that exist in no repository.

The token is **app-scoped** (`fly tokens create deploy -a sylvan-library`),
not an org-wide credential, so what leaks if it leaks is the ability to deploy
this one app.

## Consequences

- **Every merge to `main` goes live within about ten minutes**, most of which
  is the `image` job. That is the intended behaviour and it is also the thing
  to remember before merging something half-finished. `main` was already
  meant to be deployable; this makes the claim load-bearing rather than
  aspirational.
- **Every merge costs a few seconds of downtime.** Inherent to one machine and
  one volume, not introduced here — but it happens more often now.
- **A bad migration auto-applies.** This is the sharpest edge, and the one an
  approval gate would have blunted. `auth/db.py`'s ladder is forward-only, so
  rolling the *code* back does not roll the *schema* back. A schema change
  therefore deserves the treatment a deploy used to get: land it on its own,
  and watch it. The runbook is in `docs/HOSTING.md` §5.
- **A failed smoke test does not roll back**, it fails loudly and leaves the
  release list in the log. Automatic rollback was considered and rejected as
  the wrong reflex here: with a forward-only schema, an automatic revert can
  make things worse than the state it is reverting from, and the failure modes
  the smoke test catches (an unmounted volume) are not fixed by redeploying
  the previous image.
- **The instance can still be deployed by hand.** `fly deploy` from a laptop
  is unchanged and is the rollback path.
- This supersedes nothing. `docs/HOSTING.md` §4 step 5 remains how a *first*
  deploy is done, and how a deploy is done when CI is not the thing doing it.
