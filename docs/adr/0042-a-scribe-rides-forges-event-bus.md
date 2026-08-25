# 42. A scribe rides Forge's event bus, and the board is a listener rather than a parser

**Status:** Proposed · **Decided:** 2026-08-25 with Aaron · Rides on ADR 14
(deterministic code decides, Claude advises), ADR 35 (the Forge and its
worker) and ADR 36 (the match ledger records what played). Supersedes nothing;
it opens a second, richer road beside the prose parser and does not close the
first.

## Context

The Coliseum can watch a match (#300). What it watches is an *account* — the
beats `internal/sim/tier3/events.go` scrapes out of Forge's game log with
twelve regular expressions. Aaron asked for a **board**: the battlefield drawn
from card art, life totals, lands in their own zone, a graveyard in a stack.

The account cannot become a board, and the reason is not effort. It was
measured on 2026-08-25, playing `gyome-food` against `atla-palani-dinos` —
two decks whose whole point is tokens — for fifteen turns:

- **Forge's game log has no category for a token or a counter.**
  `forge.game.GameLogEntryType` is a closed enum of nineteen values, seventeen
  of which that game printed. There is no `TOKEN` and no `COUNTER`. The only
  occurrences of the word "token" in 453 lines are the deck's own name and a
  trigger's echoed rules text; the only "+1/+1 counter" hits are likewise
  oracle text being printed, never an event.
- `sim -h` offers one output control, `-q`. There is no verbosity that helps,
  because verbosity selects among categories and the categories do not exist.
- So a board built from the log would draw Gyome's commander and a few
  creatures and **none of the Food, none of the Eggs** — the decks' entire
  point, missing, on exactly the archetypes that most need a picture.

Two findings from the same measurement correct the record kept on 2026-08-24,
and both are load-bearing:

- **Tapped state is in the log after all.** That day's reading counted the
  word "tapped" (five mentions in 727 lines) and concluded there was none. The
  tap events are the forty-five `Mana:` lines — `Mana: Forest (24) - {T}: Add
  {G}` is a specific land instance tapping, and the Untap step untaps it.
- **The graveyard is richer than deaths.** `Zone Change:` also carries mill
  (`… milled Marauding Raptor (169), Swords to Plowshares (154) …`) and
  `Discard:` carries its own, and the parser reads neither.

And every card in the log carries a per-game instance id — `Forest (24)`,
`Great Arashin City (79)` — which is what makes any of this trackable.

**The decisive finding is one layer down.** `GameLogFormatter` is not a logger;
it is a *visitor* over Forge's game event bus, and it renders about
twenty-five of the fifty-nine events that bus carries. Among the thirty-four
it discards:

| Event | What it gives a board |
|---|---|
| `GameEventZone(zoneType, player, mode, card)` | every card entering or leaving **any** zone, with whose zone it is |
| `GameEventCardChangeZone(card, from, to)` | the same movement, from both ends |
| `GameEventCardCounters(card, type, oldValue, newValue)` | counters, exactly |
| `GameEventPlayerCounters` | poison, energy, experience |
| `GameEventCardTapped(card, tapped)` | tapped state, exactly |
| `GameEventCardStatsChanged(cards)` | power and toughness as they change |
| `GameEventCardAttachment` | auras and equipment |
| `GameEventCardSacrificed`, `GameEventCardDestroyed` | how a permanent left |
| `GameEventCombatUpdate`, `GameEventManaPool` | the combat and the pool |

`GameEventTokenCreated` is a record with no fields — a bare signal — and it does
not matter: a token arriving on the battlefield is a `GameEventZone` carrying
the token's own `CardView`. The data is all there. The **log** is what throws
it away.

The bus is reachable without touching Forge:

```java
Match match = new Match(rules, players, title);
Game  game  = match.createGame();      // public
game.subscribeToEvents(scribe);        // public — before a card is drawn
match.startGame(game);                 // public — Forge's own loop
```

`createGame()` and `startGame()` are separate public methods on
`forge.game.Match`, so the `Game` exists before it runs and takes an extra
listener. No fork, no patch, no reflection, and no reimplementation of the
rules engine or the game loop.

Forge's own `SimulateMatch.simulateSingleMatch` already separates them, which
is what makes the insertion a single line rather than a rewrite:

```java
final Game g1 = mc.createGame();
//  <-- g1.subscribeToEvents(scribe) goes here
TimeLimitedCodeBlock.runWithTimeout(() -> mc.startGame(g1),
    mc.getRules().getSimTimeout(), TimeUnit.SECONDS);
```

The bus is Guava's `EventBus`: `GameLogFormatter`'s entry point carries
`@com.google.common.eventbus.Subscribe`, so a scribe is an
`IGameEventVisitor.Base` subclass with one annotated method and an override
per event it cares about. Events it does not override fall through to the
base and cost nothing.

**The five things a parity gate must hold identical** are now nameable rather
than hypothetical, read off `SimulateMatch.simulate`:

- `MyRandom.setRandom(new Random(seed))` — a **global** RNG, seeded once
  before the match. A scribe that seeds differently, or later, plays different
  games.
- `rules.setSimTimeout(c)`, enforced by `TimeLimitedCodeBlock.runWithTimeout`
  — the clock, and the same mechanism whose `TimeoutException` a CPU-starved
  worker was measured provoking on 2026-08-24. It bounds the game, not the
  process.
- `RegisteredPlayer.forCommander(deck)` rather than `new RegisteredPlayer` —
  the Commander seat, with its command zone.
- `rules.setAppliedVariants(EnumSet.of(GameType.Commander))`.
- `GamePlayerUtil.createAiPlayer(name, index, profile)` — the AI profile, and
  the seat index it is created with.

## Decision

**1. A scribe, not a fork.** A small Java program links Forge as a library,
attaches one visitor to the event bus, and prints newline-delimited JSON on
stdout. Forge's jar stays the digest-pinned upstream artefact
`Dockerfile.forge` already fetches, byte for byte. ADR 36's version pin
therefore keeps meaning exactly what it means today: the ledger records which
Forge played, and it is stock Forge.

**2. The licence boundary is a process boundary, and it is deliberate.**
Forge is GPL-3.0; this repository is MIT. A Java class that links Forge's
classes is a derived work and **ships GPL-3.0**, in its own directory, with
its own `LICENSE` and a header on every file saying so. Nothing MIT links it.
The Go side talks to it the way it talks to `sim` today — a subprocess, over
stdout — which is the arm's-length separation that keeps the rest of the
repository MIT. Commandment 9 is not satisfied by intent here; it is satisfied
by the boundary being real, and the boundary is a `fork`/`exec`.

**3. Nothing is recorded through the scribe until it is proven identical to
`sim`.** This is ADR 36's whole argument applied to ourselves. We drive
Forge's loop through our own `main`, and if that differs from `SimulateMatch`
in any way — mulligan handling, seeding, the clock, AI profile — every match
already in the ledger becomes incomparable with every match after it. A gate
plays the same decks on the same seed through both paths and fails unless the
outcomes, the turn counts and the durations agree.

**4. The prose parser stays.** `events.go` keeps its frozen goldens and keeps
working; the scribe is a second source, chosen when it is present. A worker
image without the scribe still plays matches and still narrates. This is the
same degrade-rather-than-fail rule every hop of the Forge wire already
follows, and it is what lets the scribe land without a flag day.

**5. The board is a renderer, not a new pipeline.** The scribe's events cross
the same shim stream, the same worker client and the same job the beats
already use (#300). What changes is what the room draws.

## Proven, not proposed

Written and run before this ADR was accepted, because an ADR that turns out
to be impossible is worse than no ADR. `scribe/` compiles against Forge 2.0.14
with one `javac` call and plays a real match. On Gyome/Food against
Atla/Eggs, seed 11 — the same pairing whose *log* never mentioned a single
token:

```json
{"t":"zone","zone":"Battlefield","mode":"in","who":"Gyome, Master Chef — Food",
 "id":211,"card":"Food Token","token":true,"types":"Artifact - Food"}
{"t":"counters","counter":"+1/+1","was":0,"now":1,"card":"Hazel's Brewmaster"}
```

473 lines for a game the prose told in 453, and every one of them typed: 268
zone movements, 131 tap events, 15 turns, 10 life changes, 5 counter changes,
40 of them about tokens.

**One number is worth carrying forward.** Before `stats` was deduplicated that
same game emitted **3,300** of them against 473 for everything else —
`GameEventCardStatsChanged` fires on nearly every priority pass and re-sends
the whole card whether anything moved or not. Only 23 were ever news. Any
future listener added to this bus should assume the same until it has counted.

**And the parity gate is green.** `TestTheScribePlaysTheSameMagicAsStockSim`
plays the two live fixtures on seed 11 through both paths and compares the
winner by seat, the clock-outs and the draws:

```
game 1 — stock: seat 1 turns 23 | scribe: seat 1
game 2 — stock: seat 2 turns  5 | scribe: seat 2
```

**Two games rather than one, and that is the whole design of the test.** The
games are different from each other — a 23-turn grind and a 5-turn rout — so
the run reproduces a seeded *sequence* rather than a single lucky match. A
scribe that seeded the RNG per game instead of once before the match would
agree on game one and diverge on game two, which is precisely the failure a
one-game gate cannot see.

Two things the run surfaced that the design has to answer, and neither is
settled here:

- **`PlayerView.getId()` came back 0 for the first seat**, where every other
  seat number in this project is 1-based (`SimRun.Seats`, `GameEvent.Seat`,
  the shaping layer's slug map). The scribe reports what Forge says; the Go
  side must not assume the two numbering schemes agree.
- **The winner crosses as the deck's name**, because that is what
  `createAiPlayer` was handed — the same string the prose parser reads off
  `Ai(1)-<name>`. Consistent, and still a name rather than a slug.

## Consequences

- **The board becomes honest, including for tokens and counters** — the two
  things that made a battlefield not worth drawing yesterday.
- **The parser's scars stop mattering.** `events.go`'s own comments record
  them: unbounded deck names, a `Combat:` prefix Forge drops on grouped lines
  that lost two unblocked attackers out of three, an outcome sentence that
  must be matched non-greedily or a loss reads as a win. Typed records off
  Forge's own view objects have none of these failure modes.
- **A new kind of artefact enters the repository.** `tools/` is Python and
  never ships; this is Java that ships in the worker image. It needs a JDK in
  `Dockerfile.forge`'s build stage and its own gates.
- **A GPL-3.0 component now lives in an MIT repository.** That is a real
  obligation, not a formality: the directory is marked, the boundary is a
  process, and `docs/` says both plainly.
- **Two sources of beats means two things to keep honest.** Mitigated by (4):
  the scribe is preferred and the parser is the floor, and the parity gate in
  (3) is what makes preferring it safe.

## What was rejected

- **Patching Forge's `GameLogFormatter`** to print more categories. It works,
  and it breaks ADR 36: the ledger's "Forge 2.0.14" would name a Forge nobody
  else has.
- **Reconstructing a board from the prose log.** Rejected on the measurement
  above — right for lands, life and cast creatures, silently wrong for every
  token and every counter, and silently wrong is the worst of the three.
- **Shipping no board at all** and keeping the account only. This was the
  standing position from 2026-08-24 and it was correct while the bus looked
  unreachable. It is not correct now.
