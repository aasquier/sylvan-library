# The scribe

A listener on Forge's game event bus. It plays a match through Forge's own
code and writes what happens as newline-delimited JSON — every card entering
or leaving every zone, tapped state, counters, life, turns — so the Coliseum
can draw a board instead of a transcript.

ADR 42 is the argument. The short version: Forge's *log* renders about
twenty-five of the fifty-nine events its bus carries, and
`forge.game.GameLogEntryType` has no category for a token or a counter at all.
A Food deck can play fifteen turns without the log mentioning one Food. The
events were always there; nothing was listening.

## This directory is GPL-3.0. The rest of the repository is MIT.

Not a technicality, and not negotiable. This code links Forge's classes, Forge
is GPL-3.0, and a work that links it is a derived work. So:

- everything under `scribe/` is **GPL-3.0** (`LICENSE` here is the full text,
  and every source file carries the header);
- nothing MIT links it. The Go side runs it the way it runs `sim` today — as
  a **subprocess**, over stdout — and that process boundary is what keeps the
  rest of the repository MIT;
- Forge itself is never modified. The worker image fetches the upstream
  release tarball, digest-pinned, exactly as it already did, so ADR 36's
  version pin keeps meaning what it says: the ledger records which Forge
  played, and it is stock Forge.

If you are here to change something, the boundary is the rule to keep. A Go
import of anything in this directory would collapse it.

## Building

Needs a JDK 21 and the Forge jar. No Maven, no Gradle, no network at build
time — one `javac` call, because that is genuinely all this is:

```bash
./build.sh /path/to/forge-gui-desktop-VERSION-jar-with-dependencies.jar
```

## Running

Positional and dumb, because the Go side builds this argv and nothing else
ever will:

```
java -cp <forge.jar>:out scribe.Main <clock-seconds> <games> <seed|-> <a.dck> <b.dck> [...]
```

## What it emits

One JSON object per line, each with a `t` for its kind. Measured on one real
game (Gyome/Food against Atla/Eggs, seed 11, 2026-08-25):

| `t` | lines | what it carries |
|---|---|---|
| `zone` | 268 | a card entering or leaving a zone: which zone, whose, id, name, whether it is a token, its type line and stats |
| `tapped` | 131 | a card tapping or untapping |
| `stats` | 23 | power, toughness or type actually changing |
| `attack` | 16 | an attacker, and the player or planeswalker it was sent at |
| `damage` | 16 | damage: how much, from what, onto a card or a seat |
| `unblocked` | 15 | an attacker nobody blocked |
| `turn` | 15 | a turn beginning, whose, Forge's number for it, and their life |
| `land` | 14 | a land played |
| `cast` | 14 | a **spell** being cast — `isSpell()`, so never a trigger |
| `life` | 10 | a life total after it changed |
| `counters` | 5 | a counter kind, its old total and its new one |
| `block` | 1 | a blocker, and the attacker it stopped |
| `seat` | 2 | the roster at game start: seat, name, starting life |
| `outcome` | 2 | one of Forge's own outcome sentences, with the last turn |
| `mulligan` | — | a hand thrown back (neither player did, that game) |
| `game`, `result` | 2 | the frame around a game |
| `mana` | — | a seat's whole floating pool, as symbols: `"GGW"`, or `""` |
| `sacrificed` | — | a permanent its controller sacrificed |
| `combat_end` | — | Forge saying combat is over |
| `ability` | — | an ability on the stack: its source, that source's zone, and whether the game raised it |

The last four arrived later (ADR 45) and are not in that game's counts. On a
separate recorded match — a Kaheera deck against itself, 2026-08-26 — a
fourteen-turn game raised 50 `mana`, 13 `combat_end`, 4 `ability` and 1
`sacrificed`, and a forty-six-turn game raised 10 `ability`. Two card fields
came with them: `keywords`, the instance's **live** set rather than its
printing's, and `copied_by`, set only on a card made as a copy.

**Two encodings in those four are worth knowing before reading the code**, and
both are argued in `Scribe.java` against the bytecode. `GameEventCombatUpdate`
is *not* the end-of-combat signal — it is raised only by Forge's human input
handlers and never fires in a headless match, so `GameEventCombatEnded` is the
one here. And Forge's colourless mana is byte 32 (`ManaAtom`) rather than 0
(`MagicColor`), so asking `MagicColor.COLORLESS` for a count returns zero
forever and a Sol Ring's mana simply never appears.

534 lines for a game the prose log told in 453 — but every line is typed, and
the board is in them. **The `stats` figure is the one to watch.** Before it
was deduplicated that same game emitted 3,300 of them: `GameEventCardStatsChanged`
fires on nearly every priority pass and re-sends the whole card whether
anything moved or not. Only 23 were ever news.

**Seats are one-based everywhere in this stream**, which they were not at
first. `PlayerView.getId()` is zero-based for the first seat while
`SimRun.Seats` and the result line below count from one, so both schemes were
live in one stream and said different things about the same player. The scribe
now learns Forge's own player order off `GameEventGameStarted.players()`, puts
it in the stream as the `seat` lines, and reports one seat number.

**An attacker mapped to itself means it was not blocked**, and that is Forge's
encoding rather than a guess at one — `PhaseHandler` builds the multimap as
`putAll(attacker, blockers.isEmpty() ? List.of(attacker) : blockers)`. Reading
it as a block made fourteen of sixteen in a measured game read as a creature
blocking itself. `Scribe.java` carries the bytecode.

## What is not here

**Any opinion about the game.** No rules are evaluated, nothing is inferred,
and no board is reconstructed. Every line is one event Forge announced,
rendered as itself. Assembling a board from them happens on the far side of
the pipe, in Go, where it is testable without a JVM — ADR 14's division
applied to a subprocess.

## The parity obligation

Forge builds each `Game` with its own private `EventBus` and registers only
its own log on it, so a listener can only be attached between
`Match.createGame()` and `Match.startGame()`. That means `Main` owns a copy of
those lines rather than calling `SimulateMatch.simulateSingleMatch`, and
**that copy is the risk**: if it drives Forge differently from `sim`, every
match already in the match ledger becomes incomparable with every match after
it.

Five things must stay identical, and they are marked PARITY in `Main.java`:
the global RNG seeding, `RegisteredPlayer.forCommander`, the applied variants
set, `setSimTimeout` with Forge's own timed block, and the AI player and seat
index.

`TestTheScribePlaysTheSameMagicAsStockSim` in
`go/internal/sim/tier3/parity_test.go` is the gate, and it is green:

```
game 1 — stock: seat 1 turns 23 | scribe: seat 1
game 2 — stock: seat 2 turns  5 | scribe: seat 2
```

**Two games, deliberately.** They are different games — a grind and a rout —
so the run reproduces a seeded *sequence*. A scribe that reseeded per game
would agree on the first and diverge on the second, which a one-game gate
cannot see. Run it after changing anything marked PARITY:

```bash
MTGLAB_LIVE_FORGE=1 MTGLAB_SCRIBE_CLASSES=$(pwd)/scribe/out \
  go test ./internal/sim/tier3/ -run TestTheScribePlaysTheSameMagicAsStockSim -v
```

**The gate drives both paths through `RunGames`**, which is stronger than
building an argv by hand: the two runs differ in exactly one field of one
struct (`Settings.ScribeClasses`), so anything that disagrees is the scribe
playing different Magic and not the harness asking differently. It compares the
winner by seat, the turn count, the draws and the clock-outs, and asserts a
board was reported at all.

The turn count is in that list for a reason it caught the first time it ran.
`GameEventGameOutcome.lastTurnNumber()` is a *player-turn* count, and Forge's
own `GameLogFormatter` renders the log's outcome line as
`Math.ceil(lastTurnNumber() / 2.0)` — so the log has always reported **rounds**
and the bus reports player-turns. The stock path said 23 and the scribe said
46. The Go side now applies Forge's own halving, because matching Forge is the
requirement rather than being right in general.

It needs a JVM, a Forge distribution and about a minute, so it does not run in
CI. What CI does prove is that the scribe still compiles against the jar the
worker image ships — `Dockerfile.forge` builds it in a stage of its own, so a
`FORGE_URL` bump that moved an event is a red build rather than a listener
that quietly stopped hearing something.
