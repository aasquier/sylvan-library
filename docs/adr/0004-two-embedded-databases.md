# 4. Two embedded databases when hosting, and two tiers of deck

**Status:** Proposed — nothing is deployed yet · **Recorded:** 2026-08-10

## Context

The goal is one hosted instance that friends log into, not a fork per person. A
fork gives every friend a 500 MB Scryfall download, a Python toolchain, and
their own stale copy of the code; one instance means one upgrade path.

That collides with [ADR 1](0001-deck-yaml-in-git-is-the-source-of-truth.md).
Friends' decks cannot live in the maintainer's git repo, so a second storage
path is needed — and the question is how much of the local-first model to keep.

## Options considered

**Move every deck into the database.** Rejected. It gives up the git swap
record, which is one of the few genuinely novel things this project does, in
exchange for one code path instead of two. Bad trade.

**Give each user a git repo on the volume.** Tempting, because it would preserve
`swaps.md` for everyone. Rejected: git operations per request, concurrent writes
to repos, and a much larger failure surface, for a feature nobody has asked for.
Revisit if they do.

**One database for everything.** Rejected — see below; the two stores have
different lifecycles.

## Decision

**Two tiers of deck.** The six curated decks stay file-backed in git,
*permanently*, and ship read-only inside the image; everyone can view them, and
that is the showcase. User decks live in SQLite on the volume, one row per deck.

**Two embedded databases, zero managed services:**

| Store | Engine | Contents | Access |
| --- | --- | --- | --- |
| `/data/mtg.duckdb` | DuckDB, ~63 MB | Scryfall corpus, prices | Read-mostly, rebuilt by `data refresh` |
| `/data/app.db` | SQLite | users, sessions, user decks, cached sim results | Read-write |

Keep them separate. DuckDB is an analytics engine holding regenerable public
data; SQLite is transactional state that must be backed up. Different
lifecycles, different backup rules — do not merge them for tidiness.

SQLite over Postgres for `app.db`: a file on a volume already paid for, no
second service, no pooling, no extra $7/month, and it will not break a sweat at
this concurrency. WAL mode on, so readers do not block the writer.

`user_decks.yaml` stores the same YAML the file-backed decks use, so `Deck.load`,
the gate and the artifact generator all work unchanged on both tiers. **One
parser, one validator, two sources.**

## Consequences

- The maintainer's workflow does not change at all: still edit YAML locally,
  still run `mtglab decks build`, still commit.
- The seam this needs is a `DeckSource` protocol with a `FileDeckSource` today
  and an `SqlDeckSource` later. It is worth building *before* hosting, because
  the API reads the filesystem directly in only four places right now and will
  read it in more later. It also makes the API testable against an in-memory
  source, which pays for itself immediately.
- Backups are asymmetric and that is deliberate: the corpus needs none, because
  `data refresh` rebuilds it. `app.db` is irreplaceable and must be backed up
  with SQLite's online backup — never a plain `cp` of a live WAL database, which
  can capture a torn copy. Those backups contain password hashes; keep them
  private.
- CLAUDE.md rule 5 carries over past git. If collection, wishlist or purchase
  features ever ship, they are per-user, behind auth, never in a public view,
  and never in a backup put somewhere shared — because a public inventory of
  expensive cards tied to a real identity is a targeting list.
