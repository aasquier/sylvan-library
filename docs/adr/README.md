# Architecture decision records

One file per significant decision: the context, the options that were
considered, what was decided, and what it costs. They are written to answer
"why did you do it that way?" in advance.

These are **immutable once accepted**. A decision that changes gets a new ADR
that supersedes the old one, and the old one stays — the reasoning that turned
out to be wrong is often the most useful thing in the directory.

Most of these are retrospective: the decisions were made and argued while the
project was built, and written down on 2026-08-10. Each records both dates where
they differ.

| # | Decision | Status |
| --- | --- | --- |
| [1](0001-deck-yaml-in-git-is-the-source-of-truth.md) | `deck.yaml` in git is the source of truth, and deck history is git history | Accepted |
| [2](0002-duckdb-for-the-card-corpus.md) | DuckDB for the card corpus — not SQLite, not Postgres, not the API | Accepted |
| [3](0003-tier-1-stays-python.md) | Tier 1 stays Python; a compiled port is deferred with a written trigger | Accepted |
| [4](0004-two-embedded-databases.md) | Two embedded databases when hosting, and two tiers of deck | Proposed |
| [5](0005-sessions-over-jwts-and-no-self-signup.md) | Sessions over JWTs, and no self-signup | Proposed — "no self-signup" superseded by [16](0016-accounts-are-invited-and-passwords-are-self-served.md) |
| [6](0006-never-redistribute-scryfall-bulk-data.md) | Never redistribute Scryfall bulk data; refresh it in place, on demand | Accepted |
| [7](0007-card-facts-come-from-the-corpus.md) | Card facts come from the corpus, never from memory | Accepted |
| [8](0008-the-gate-blocks.md) | The gate blocks, and an unevaluated rule warns rather than passing | Accepted |
| [9](0009-commit-the-built-frontend-bundle.md) | Commit the built frontend bundle | Accepted |
| [10](0010-correctness-against-independent-oracles.md) | Correctness is established against independent oracles | Accepted |
| [11](0011-the-api-may-apply-a-swap.md) | The API may apply a swap the user has decided on | Accepted |
| [12](0012-decks-are-edited-by-surgical-operations.md) | Decks are edited by surgical operations over text | Accepted |
| [13](0013-an-imported-deck-is-a-draft.md) | An imported deck is a draft until every card is justified | Accepted |
| [14](0014-python-decides-claude-advises.md) | Python decides, Claude advises, and Forge plays the games | Accepted |
| [15](0015-claude-surfaces-are-modes-with-capabilities.md) | Claude surfaces are modes with a user-set stance, and no stance may write a rationale | Accepted |
| [16](0016-accounts-are-invited-and-passwords-are-self-served.md) | Accounts are invited, and passwords are self-served by email | Accepted |
| [17](0017-the-maintainer-is-named-in-the-environment.md) | The maintainer is named in the environment, and admin routes live behind a prefix | Accepted |

**Proposed** means the decision is made and argued but nothing implements it
yet — 4 and 5 still describe a deployment that does not exist. 16 was Proposed
for a matter of hours: the auth core landed the same day it was written and the
email half the day after, so it is the one part of the hosting plan that is
code rather than intent.

17 finished the server half the same day, and it is worth reading next to 5
rather than after it: 5 decided that a resource belonging to one person is
reported as 404 and not 403, and 17 is the case where that rule deliberately
does *not* apply — an admin route's existence is published in this repository,
so 403 hides nothing and says something useful. What is still missing is the
browser's way in: **there is no login screen and no claim page**, which
`docs/HOSTING.md` §6 step 5c tracks. The admin surface now exists and, with
auth on, sits behind a door nobody can open yet.

16 supersedes exactly one paragraph of 5, which is the pattern this directory is
for: 5's "no self-signup" was a good argument that lost to a better one a day
before anybody wrote auth code, and both readings are worth having side by side.
The rest of 5 — sessions over JWTs, Argon2id, the scoped accessor, the isolation
test — is untouched.

14 and 15 were Proposed for exactly one day, which was the point: they drew the
boundary for a model integration before the first client call rather than after
it. Both are now Accepted and **partly** implemented — the client, the tools and
the rule that no code path writes a `why`; not the modes, the stances, or Forge.
Each carries a note at the top saying which half is which, because a status of
Accepted on its own would overstate it.

## Where the longer arguments live

An ADR is the decision. The working documents are the analysis behind them:

- [`ROADMAP.md`](../../ROADMAP.md) — goals versus reality, and open decisions.
- [`docs/ENGINEERING.md`](../ENGINEERING.md) — the next phase, with the
  measurements behind ADR 3 and ADR 10.
- [`docs/HOSTING.md`](../HOSTING.md) — the deployment guide behind ADR 4, 5
  and 6, including the profiling numbers and the DuckDB locking rule.

## Adding one

Copy the shape of any file here: a title, `**Status:**` and dates, then
**Context**, **Options considered**, **Decision**, **Consequences**. Number it
next in sequence. The options section is the part that ages best — a decision
without its rejected alternatives is an assertion.
