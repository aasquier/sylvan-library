# 30. Decks are live app data, not repository content

**Status:** Accepted · **Decided:** 2026-08-16 · Supersedes the *location*
half of [ADR 1](0001-deck-yaml-in-git-is-the-source-of-truth.md); its
*format* half stands untouched.

## Context

ADR 1 put `decks/<slug>/deck.yaml` in git and made two claims in one
sentence: that the YAML file is the source of truth, and that the repository
is where it lives. The first claim has only gotten stronger — every gate,
simulator, artifact and edit operation reads and writes that file. The second
has been eroded by nearly everything built since:

- **The app became the editor.** ADR 11 and 12 gave every edit a route, and
  the hosted instance keeps its decks on a volume at `/data/decks` precisely
  because decks baked into an image layer would lose every edit at the next
  deploy. From that day there were two copies — the repository's and the
  instance's — and `docs/HOSTING.md` had to grow a section admitting that
  neither is automatically authoritative.
- **`decks/` is the local app's live data directory.** Running the app writes
  it. That collision has cost real incidents: a test deck swept into a
  "docs only" pull request by `git add -A`, and eventually a PreToolUse hook
  that refuses `git add -A` outright. A directory that is both working data
  and tracked content makes every commit a hazard.
- **Deck history got a purpose-built record.** ADR 28's activity log records
  what was done to a deck and by whom, keyed on the deck rather than on a
  file path. ADR 1's "deck history is git history" was already the weaker of
  the two records for any deck edited through the app.
- **The decks are the maintainer's personal data.** Rule 5 keeps collection
  and wishlist files out of a public repository; a curated decklist tied to a
  real identity is milder but the same shape. The repo's job is the tool, not
  the toybox — "decks are fixtures, not the product" has been the working
  rule for a while, and the fixtures the suite actually uses are synthetic
  (`tests/tiny_pool.py`).

The immediate prompt is ADR 32's runtime derivation tier, whose argument —
git stays clean of everything that is not the tool — reads cleanest when it
is true without exceptions.

## Options considered

**Keep the decks tracked.** The status quo. Rejected: the two-copies problem
is permanent and grows a new edge with every editing feature; the working
tree is dirty whenever the app has been used; and the repository carries data
that belongs to a person rather than to the project.

**Move them to a private repository.** Rejected. It answers the publicity
point only, keeps every synchronisation question alive, and adds a second
remote to a project whose deck history already has a better record (ADR 28)
and whose disaster story already has a better answer (volume snapshots and
`backups/`).

**Untrack them: decks are app data under `MTGLAB_DECKS_DIR`.** Chosen. This
is where every sibling already lives — the pool, `app.db`, the caches — all
of them things the app owns, none of them in git.

## Decision

`decks/` is gitignored and untracked, locally and everywhere. `deck.yaml`
remains the only source of truth for a deck — ADR 1's format decision is
untouched and everything that enforces it (the gate, the five artifacts, the
surgical edits) is unchanged.

Consequently:

- **The image carries no deck seed.** `/app/decks-seed` and its copy step in
  `docs/HOSTING.md` §4 are gone. A fresh instance legitimately starts with
  zero decks and is populated the way the pool is: a documented run —
  restore a backup over sftp, or import through the app.
- **`swaps.md` diffs against the build snapshot, not against git.**
  `write_all` stashes `artifacts/deck.last-built.yaml` on every build, and a
  bare `mtglab decks build` diffs against it. "Commit before editing" becomes
  "build before editing", which the workflow already does.
- **The suite reads no real decks.** Tests that leaned on the curated six as
  convenient fixtures use `tiny_pool.mono_green_deck()` or an inline `Deck`;
  a checkout has no `decks/` directory and everything must pass that way.

## Consequences

- The two-copies problem is resolved by removal: each instance has one live
  copy, and "which is authoritative" stops being a question with a per-deck
  answer.
- The removal commit does not rewrite history: the six decklists as of
  2026-08-16 remain readable in old revisions. That is accepted — they were
  published deliberately for two years — but the *live* lists diverge from
  that snapshot immediately and permanently.
- Git no longer backs up the decks. The volume snapshot schedule and the
  `backups/` pull in HOSTING §5 are now the whole recovery story, and they
  were already the recovery story for every deck created in the app.
- ADR 1's diffability argument is served two ways: the activity log says what
  happened, and the build snapshot gives `swaps.md` a baseline. Neither is a
  server-side undo, which ADR 27 rejected and this decision does not revisit.
- The `image` CI job loses its decks-seed check. Its pool check stays; the
  deploy health check's `decks > 0` assertion stays and matters *more*,
  because the volume is now the only copy.
- A contributor cloning the repo sees an empty library until they import
  something, which is the honest state: the tool is the product, and it has
  always had to work before any particular deck exists.
