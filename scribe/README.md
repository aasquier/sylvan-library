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
| `turn` | 15 | a turn beginning, and whose |
| `land` | 14 | a land played |
| `life` | 10 | a life total after it changed |
| `counters` | 5 | a counter kind, its old total and its new one |
| `game`, `outcome`, `result` | 3 | the frame around a game |

473 lines for a game the prose log told in 453 — but every line is typed, and
the board is in them. **The `stats` figure is the one to watch.** Before it
was deduplicated that same game emitted 3,300 of them: `GameEventCardStatsChanged`
fires on nearly every priority pass and re-sends the whole card whether
anything moved or not. Only 23 were ever news.

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
index. ADR 42 requires a gate that plays the same decks on the same seed
through both paths and fails unless they agree — until that gate is green,
nothing recorded through the scribe belongs in the ledger.
