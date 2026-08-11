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
| [5](0005-sessions-over-jwts-and-no-self-signup.md) | Sessions over JWTs, and no self-signup | Proposed |
| [6](0006-never-redistribute-scryfall-bulk-data.md) | Never redistribute Scryfall bulk data; refresh it in place, on demand | Accepted |
| [7](0007-card-facts-come-from-the-corpus.md) | Card facts come from the corpus, never from memory | Accepted |
| [8](0008-the-gate-blocks.md) | The gate blocks, and an unevaluated rule warns rather than passing | Accepted |
| [9](0009-commit-the-built-frontend-bundle.md) | Commit the built frontend bundle | Accepted |
| [10](0010-correctness-against-independent-oracles.md) | Correctness is established against independent oracles | Accepted |
| [11](0011-the-api-may-apply-a-swap.md) | The API may apply a swap the user has decided on | Accepted |

**Proposed** means the decision is made and argued but nothing implements it
yet — 4 and 5 both describe a deployment that does not exist.

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
