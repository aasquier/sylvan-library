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
| [18](0018-a-cached-simulation-is-keyed-on-its-compiled-input.md) | A cached simulation is keyed on its compiled input, not on the deck file | Accepted |
| [19](0019-the-dossier-cites-three-sources.md) | The commander dossier cites three sources, and the page shows the seams | Accepted |
| [20](0020-the-theme-interview-reads-a-person.md) | The theme interview reads a person, and Python decides when it may propose | Accepted |
| [21](0021-a-persona-is-a-voice-and-the-spread-is-the-slots.md) | A persona is a voice, and a tarot spread is the slots wearing pictures | Accepted |
| [22](0022-decks-have-owners-and-sharing-is-a-flag.md) | Decks have owners, and sharing is a flag on the deck | Accepted |
| [23](0023-a-green-main-deploys-itself.md) | A green `main` deploys itself, and the deploy is not done until the instance answers | Accepted |

**A note on vocabulary.** 2 and 7 call the local card database the **corpus**.
That is what it was called when they were written; everywhere else in the
project it is now the **card pool**, renamed on 2026-08-14 for being a word
from linguistics rather than from Magic. The ADRs keep the old word because
they are records of what was decided and how it was said at the time. Same
thing, two names, and this is the only place both appear.

**Proposed** means the decision is made and argued but nothing implements it
yet. 4 and 5 were both recorded that way, and both have since been built and
deployed — their status fields are left as written, because a status is part of
the record rather than a live indicator. 16 was Proposed for a matter of hours:
the auth core landed the same day it was written and the email half the day
after, so it is the one part of the hosting plan that was code rather than
intent from the start.

17 finished the server half the same day, and it is worth reading next to 5
rather than after it: 5 decided that a resource belonging to one person is
reported as 404 and not 403, and 17 is the case where that rule deliberately
does *not* apply — an admin route's existence is published in this repository,
so 403 hides nothing and says something useful. The browser's way in — the
login screen and the claim page, `docs/HOSTING.md` §6 step 5c — landed later
the same day, so the admin surface is reachable rather than sitting behind a
door nobody can open.

18 is the first ADR that refines an earlier one without superseding any of it.
4 put a `sim_cache` table in `app.db` and described a sim result as a pure
function of "deck content + parameters"; that storage decision stands and the
description was wrong in a way that would have shipped stale numbers, because
card facts come from the corpus rather than from the deck file. Writing it down
as its own record rather than as a correction to 4 is the directory's own rule:
the sentence that was almost right is more useful visible than edited away.

19 is the first ADR written *ahead* of the code it constrains since 14 and 15,
and for the same reason: it is where a boundary is easiest to lose. 15's table
named four modes; the dossier is a fifth, and the first whose facts cannot all
come from the corpus — so the rule it needed was not "facts come from the
corpus" but a statement of which source may support which kind of claim. Read it
next to 7, which is the rule it is bending around rather than breaking.

20 is the third written ahead of its code, and it is where 15's frame gets its
hardest test: the first **conversational** surface in the project, so the first
with state between requests and no natural place to stop. Read it next to 19,
whose instruments it borrows and extends — a claimed source is intersected with
what the search returned, and now a claimed *preference* is intersected with
what the user actually typed. It also declines to inherit one of 19's rules:
an unsourced dossier is refused, an unsourced proposal is not, because a
proposal's load-bearing content is card pool facts and survives the loss.

21 adds the third thing a surface has. 15 gave a mode a **stance**, which is
all about *how much* the model does; a **persona** is *who it sounds like*,
which is orthogonal — so it is its own field, appended to the mode's
instructions rather than substituted for them. Its second half is the load-
bearing one: a tarot spread's three positions **are** the theme interview's
first three slots, so a card is dealt *for* a slot and 20's readiness
instrument works untouched. The querent's own words stay the only evidence —
a card is not something they said.

22 is the one written after a bug rather than before one. Deck ownership was
implicit in the filesystem until an invited account turned out to be able to
edit the curated six; the fix made an owner a field and sharing a flag, and
the ADR records why the answer is 404 for a deck you cannot see and 403 for
one you can see but may not write.

23 is also written after the thing it prevents, and the thing was an hour old:
the quality pass renamed a wire field, and the running instance went on
answering the old name because deploying was still something a person had to
remember. It is the directory's clearest example of a **silent** failure —
a skipped deploy raises nothing, unlike the failing test or the failing build
either side of it. Read the consequences rather than the decision; the
interesting half is what an auto-deploy costs, and the sharpest edge is that
a forward-only schema migration now applies without anybody watching.

16 supersedes exactly one paragraph of 5, which is the pattern this directory is
for: 5's "no self-signup" was a good argument that lost to a better one a day
before anybody wrote auth code, and both readings are worth having side by side.
The rest of 5 — sessions over JWTs, Argon2id, the scoped accessor, the isolation
test — is untouched.

14 and 15 were Proposed for exactly one day, which was the point: they drew the
boundary for a model integration before the first client call rather than after
it. Both are Accepted, and most of what they describe now exists: the client,
the tools, the stance, Forge, four modes across three features, and the rule
that no code path writes a `why` — which is enforced over the package's syntax
tree rather than by prompt. Three of the modes 15 tabulates are still unbuilt,
and there is still no UI for the stance dial. The notes at the top of each say
which half is which, because a status of Accepted on its own would overstate
it.

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
