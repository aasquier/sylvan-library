# 46. The Coliseum runs at night

**Status:** Proposed — the record reads the ledger today; nothing runs on a
clock yet · **Recorded:** 2026-08-26 · Consumes [ADR
36](0036-the-match-ledger-records-declared-labels.md)'s dataset and is bounded
by [ADR 35]'s single worker.

## Context

Two asks, one surface, and they are the same feature seen from both ends.

Aaron, 2026-08-26:

> "We are going to want to start collecting data from coliseum runs to
> generate a coliseum stats tab from. It will have heads up win rates, deck win
> rates, type win rates, etc. It will help visualize and drive things like
> leaderboards in the future."

> "I would like players to be able to opt all, or some of their decks, into
> forge simulations at night. I think we might keep the name simple and call
> it 'Coliseum at Night'."

The first half shipped with this ADR: `ledger.Board` and
`GET /api/coliseum/standings`, rendered as **The record** beside **The sand**
in the Coliseum. The second half is designed here and **not built**, because
it is a scheduler and a scheduler is the kind of thing that either works or
silently does not.

Four facts about the ground it would stand on, all re-checked rather than
inherited:

- **Nothing in this application runs on a clock.** There is no cron, no boot
  ticker, no periodic wake-up. The one long-lived timer in the served process
  is `internal/pool`'s idle reaper, started lazily on the first pool open. A
  nightly run would be the first scheduler this app has ever had.
- **`internal/jobs` is not a scheduler and cannot become one by accident.** It
  is an in-memory, request-triggered registry — one per process, behind a
  mutex, with three lanes over goroutines. Restart the process and every job
  is gone. Nothing writes a job to `app.db`.
- **The `FORGE` lane is one wide**, deliberately: two JVMs race the shared
  deck directory. A match wants two and a half cores, and the hosted worker is
  one machine.
- **Merging deploys** (ADR 23). A deploy restarts the process at any hour,
  including the middle of a night run.

## Options considered

**A clock outside the app — cron in the image, or a scheduled workflow.** The
usual answer, and it fails on a fact specific to this deployment: the machine
is held awake but the *process* restarts on every merge, and an external clock
firing into a restarting process has no way to know whether tonight's work has
already happened. It also puts the schedule somewhere the app cannot read,
which makes "what is the Coliseum doing tonight" a question with two possible
answers. Rejected.

**Piggyback on requests — run a little of the night whenever somebody visits.**
Tempting because it needs no scheduler at all, and it is how the pool's reaper
avoids one. It fails on the shape of the traffic: this instance is quiet at
night by definition, so the hours the work is meant to happen in are exactly
the hours nothing triggers it. It would run the night at lunchtime. Rejected,
and worth writing down because it is the option that looks free.

**A ticker in the served process, with progress in rows.** Chosen. It is the
smallest thing that survives a restart at an arbitrary hour, and the only one
where the schedule, the roster and the record all live in the same place the
room reads.

**A second machine that wakes, runs, and sleeps.** Correct, more expensive, and
strictly a superset — the runner above does not care which machine the worker
is on. Deferred rather than rejected; it becomes attractive the moment the
night is long enough to be worth not paying for during the day.

**Opting in per account rather than per deck.** Simpler, and it is not what was
asked for ("all, or some of their decks"). It also throws away the one piece of
information that makes a night's work worth doing: a player usually wants to
know about the deck they are tuning, not about their whole shelf. Rejected.

## Decision

### 1. The opt-in is a per-deck flag, stored the way `shared` already is

A deck opts in, not an account, because Aaron asked for "all, or some". The
storage follows the one existing per-deck boolean exactly rather than
inventing a second rule for the same shape:

- **SQL-tier decks** (`user_decks`) get a column, and the column is the truth.
- **The file-backed curated library** keeps its truth in `deck.yaml`, written
  through a `deckedit` surgical setter beside `SetShared`.

The tempting alternative — *the deck file is the truth, always* — reads better
against this project's opening sentence and is wrong here for one concrete
reason: the runner's first question every night is "which decks are in?", and
answering it by parsing every YAML in the library is a scan where an indexed
read belongs. `shared` already made this trade and already carries the
two-tier split; a second flag doing it differently would be two rules to
remember and one of them to get wrong.

### 2. Night is a window, and the window is configuration

Not a moment. A run that starts at exactly 02:00 and takes ninety minutes is a
run that either finishes or does not, with nothing in between to reason about.
A window — open, run until it closes — makes overrun a *state* rather than an
accident, and the zone is configuration because the friends this instance
serves are not all in one.

### 3. The runner is a ticker in the served process, and its progress is on disk

There is one instance, so a ticker at boot that wakes every few minutes and
asks "is the window open, and what is left of tonight's work?" is the smallest
thing that can work. What it must **not** do is hold that answer in memory:
deploys land at any hour and restart the process, so a night's progress kept
the way `internal/jobs` keeps a job would be either re-run from the top or
dropped on the floor, and which one you got would depend on when Aaron merged.

So the night's roster and its progress are rows, and the runner is
crash-safe by being resumable rather than by being careful.

### 4. Interactive work wins, always

The night submits through the same one-wide `FORGE` lane as a person pressing
*Send them in*. That is not a limitation to work around — it is the whole
concurrency design, and it means a live match and the night's batch can never
collide over the worker. Where they queue, **the person waiting in the room
wins**: a nightly batch that makes somebody watch a spinner has taken the
room's own purpose away to feed a statistic about it.

### 5. One account cannot have the night

A per-account cap per night, and a round-robin across accounts rather than a
walk down a list. Somebody who opts forty decks in gets their share, not the
whole window. The total is capped as well, because the window is a promise
about when the machine is quiet and not a budget to spend to the last minute.

### 6. A match in flight is finished, never abandoned

If the window closes mid-match, that match runs to its end and the runner
stops. This is the ledger's own rule one level up: the match has been played
and paid for in JVM minutes, and throwing it away at the buzzer wastes exactly
the resource the window exists to ration.

### 7. It records through the one call site, and reads back through the record

No second writer (ADR 36). A night's matches are ordinary recorded matches,
they land on the same boards as everything else, and **the record does not
distinguish them** — a win is a win whoever was awake for it.

### 8. Nothing about the machinery renders

Commandment 10. The room may say the night's bouts are fought while nobody is
watching; it may not say what schedules them, what runs them, or where. "The
Coliseum at night" is the whole vocabulary this needs.

## What this ADR deliberately does not decide

- **Cross-account leaderboards.** The record's scope today is the narrow one:
  you see a match you were in, plus the house's own. Widening that is a
  sharing decision and it is Aaron's, not a side effect of whichever read got
  written first. `ledger.Scope` is the one place it would widen, and it says
  so.
- **Whether the night runs on the hosted worker or its own machine.** The
  design above is correct either way; the second is a cost question.

## Consequences

- **A schema change**, which under ADR 23 means it applies on boot of a live
  deploy. It is forward-only, so this ADR being *Proposed* is load-bearing:
  the column should land when the design is agreed, not before, because a rung
  of the ladder cannot be taken back.
- **The first scheduler.** Everything above about crash-safety, fairness and
  yielding exists because a background loop in a served process is a new class
  of thing here, and every one of those properties is invisible until the
  night it is missing.
- **A known fault becomes a nightly one.** The forge worker's first match
  after any deploy times out on a cold image, and its idle timeout is 180
  seconds. A run that starts cold will lose its first match to that, every
  time, until the fault is fixed — which makes fixing it a prerequisite rather
  than a nice-to-have.
- **The record gets denser fast**, which is the point: the small-sample rules
  in `ledger.Board` exist precisely so a board is honest while the ledger is
  thin, and a night's work is how it stops being thin.

[ADR 35]: 0035-the-forge-joins-the-simulator-and-a-worker-runs-it-hosted.md
