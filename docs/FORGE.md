# Tier 3: running Forge

[ADR 14](adr/0014-python-decides-claude-advises.md) puts game-playing on
[Forge](https://github.com/Card-Forge/forge) rather than on anything written
here. This is how to install it and what the bridge does.

Nothing on this page is committed. Forge's distribution, its card data and the
JVM all live under `~/.local/share/mtglab/`, outside the repository, because
`res/cardsfolder` is a second engine's card pool and `CLAUDE.md` rule 5
covers it.

## Install

**A JVM, 17 or newer.** `brew install openjdk@21` does *not* work on a Mac at
the Monterey ceiling: this Homebrew has no bottle for it, so it is a source
build, and the build refuses with `Your Xcode (12.4) ... is too outdated.
Please update to Xcode 14.2`. A prebuilt JDK needs no compiler at all:

```bash
mkdir -p ~/.local/share/mtglab/jdk-21 && curl -sSL \
  "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.12%2B8/OpenJDK21U-jdk_x64_mac_hotspot_21.0.12_8.tar.gz" \
  | tar -xz -C ~/.local/share/mtglab/jdk-21 --strip-components=1
```

`MTGLAB_JAVA` overrides the choice. Otherwise the bridge looks in that
directory and then on `PATH` — and it *checks the version* rather than trusting
it, because this machine's `/usr/bin/java` is 10.0.1 and Forge fails on it in a
way that reads like a Forge bug.

**Forge itself**, ~470 MB unpacked:

```bash
mkdir -p ~/.local/share/mtglab/forge && curl -sSL \
  "https://github.com/Card-Forge/forge/releases/download/forge-2.0.14/forge-installer-2.0.14.tar.bz2" \
  | tar -xj -C ~/.local/share/mtglab/forge
```

Despite the name, that asset *is* the distribution — `forge.sh`, the desktop
jar and `res/`. `MTGLAB_FORGE_HOME` overrides where it lives.

## Use

```bash
mtglab sim forge arahbo-cats atla-palani-dinos --games 10
mtglab sim forge tivit-cedh gyome-food --check-only   # pre-flight, no JVM
```

Two to four decks. `--check-only` is the card-coverage pre-flight on its own:
it reads a zip, needs no Java, and is the cheap thing to run first.

## What the bridge had to work around

Five things, each established by running Forge rather than by reading its wiki.

**`sim` still initialises AWT, and dies silently without a display.** Found on
the first live worker machine (2026-08-20): `forge.view.Main` touches fonts
and the toolkit before any game starts, a headless Linux answers with
`HeadlessException`, and Forge's own crash reporter swallows it whole — exit
code 1, zero bytes of output, no log file, diagnosed only by `-verbose:class`
showing that exception as the last class loaded. `-Djava.awt.headless=true`
makes it worse, not better (the AWT calls then throw instead of rendering).
The worker image's answer is a full JRE plus `xvfb`: `MTGLAB_JAVA` points at
a wrapper that runs every JVM under `xvfb-run -a`, so Forge gets a display
that renders nowhere. A Mac never shows this — a logged-in session always has
a display, which is why the spike couldn't have caught it.

**`-D` does not work for a single match.** The documented "absolute directory
to load decks from" is only wired into tournament mode; the single-match path
resolves deck names against Forge's user profile and ignores `-D` silently.
So the runner writes `forge.profile.properties` into the Forge install pointing
`userDir` at `~/.local/share/mtglab/forge-profile`, and puts generated `.dck`
files in `<userDir>/decks/commander/`. That is the one thing here that reaches
into the installation, and it is rewritten only when it would change.

**Forge must run with its own directory as the working directory**, the way
`forge.sh` does, or it cannot find `res/`.

**Card names in Forge are face names.** Its card index holds `Bala Ged
Recovery` and `Bala Ged Sanctuary`, never Scryfall's combined `Bala Ged
Recovery // Bala Ged Sanctuary` — and the same for split cards, `Alive` and
`Well` rather than `Alive // Well`. `coverage.resolve` is the single place a
Scryfall name becomes a Forge name, and the exporter writes exactly what the
pre-flight resolved, so a clean report and a correct `.dck` cannot drift apart.

**An unimplemented card does not stop a game.** This is the important one. A
deck with three names Forge does not know produced:

```
An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".
Forge could not find this card in the Database. ...
Game Result: Game 1 ended in 7212 ms. Ai(2)-Atla Palani ... has won!
```

A 96-card deck, a winner, a turn count, and a result line that says nothing is
wrong. Every number after that point is poisoned and nothing downstream could
tell. So coverage is checked twice, by two independent routes:

* **Before**, in the coverage pre-flight (`sim/tier3/coverage.go`), against
  `res/cardsfolder/cardsfolder.zip` — the same card scripts the engine loads,
  33,587 files yielding 34,532 names in under two seconds, cached per (path,
  mtime, size) so a Forge upgrade invalidates it. `RunGames` refuses rather
  than returning a flag, because a caller that could ignore a flag would.
* **After**, in the parser (`sim/tier3/parse.go`), by scanning Forge's own
  output for that warning. `ErrResultsUntrustworthy` discards results that
  otherwise look perfectly normal.

All six curated decks pass the pre-flight with no missing cards.

## Hosted: the worker machine

The deployed app plays matches too, and holds none of the above (ADR 35).
`Dockerfile.forge` bakes the JRE, the pinned 2.0.14 distribution and the
same `mtglab` binary the app runs — the worker is its `forge-shim`
subcommand — into a worker image; the deploy workflow keeps a
dedicated-CPU machine named `forge-worker` pointed at it, stopped between
matches. The app wakes it per job over the Machines API, runs the same
pre-flight against the worker's own card scripts (`/coverage`, on the
request thread, so a 422 still names the cards before any JVM boots), plays
the match over the private network (`/match`), and the shim stops its own
machine after `MTGLAB_FORGE_IDLE_SECONDS` of quiet. Results are rebuilt into
the same `SimRun` a local run returns — `sim/tier3/wire.go` is the seam,
and the recorded worker-wire corpus pins the shape from both sides. The
image holds GPL'd Forge and is pushed only to the app's private registry —
deployment, never distribution.

### The worker carries a quarter of the distribution

The release is one download for every way Forge can be played — an Adventure
RPG, quest mode, a mobile build, a particle editor, card names in nine
languages — and the worker plays exactly one of them: `sim`, from the desktop
jar, in English. `Dockerfile.forge` deletes the rest after unpacking, which
takes 465MB down to roughly 110MB.

Measured before it landed (2026-08-24): a copy holding only the kept paths
played Arahbo vs Goreclaw at seed 12345 to the same Turn 9, the same winner
and the same 8.1s as the full install, with nothing on the stream but the
result.

Two things kept that look prunable. `res/skins` and `res/sound` are GUI
furniture no simulation renders — but AWT initialises anyway (the first fact
above), and resources are not the place to fight a headless mode Forge will
not give us. `res/deckgendecks` is 10MB of matrix data nothing here generates
a deck from, and dropping it still printed `Error reading matrix data` and a
caught `NullPointerException` at every boot; a rules engine that throws on
startup is not a place to save ten megabytes.

What the prune buys is registry storage, the push in CI, and the first pull
onto a new host. It does **not** buy cold-start latency — that is ~8s of JVM
boot and card-database load, unchanged.

### Staying current with Forge

`.github/workflows/forge-release.yml` checks weekly whether Card-Forge has
published a release newer than the pinned one, and opens an issue carrying the
exact `ARG` values when it has. It never opens a pull request, and that is the
decision rather than an omission: ADR 36 records `forge_version` on every
match because Forge's AI is the instrument each recorded game was measured
with. An upgrade changes the judge — ratings computed across an unversioned
one would silently mix two — so it re-runs the coverage pre-flight and moves
the ledger forward deliberately.

The goldens under `sim/tier3/testdata/` record the version a match **was**
played with and never move with an upgrade.

### Testing deploy skew on purpose

Every release updates the app **before** the worker, by several minutes and
deliberately: the app deploy is proven first, so a red worker sync is
feedback about the worker rather than a rollback of the app.

In that window the app talks to the *previous release's* shim, so the
client must read an older shim's wire, not merely today's. The case is
tested rather than waited for: build (or keep) an older release's `mtglab`,
start its shim, and drive it with today's client. With a distribution
present:

```bash
MTGLAB_FORGE_HOME=~/.local/share/mtglab/forge \
MTGLAB_FORGE_SHIM_PORT=8899 MTGLAB_FORGE_IDLE_SECONDS=0 \
  <older-release-mtglab> forge-shim &

cd go && MTGLAB_LIVE_FORGE=1 MTGLAB_OLD_SHIM_URL=http://127.0.0.1:8899 \
  go test ./cmd/... -run Shim -v
```

Deliberate wire renames die against it — and the first version of that test
survived two of them, because it asserted the seats map and a duration while
a row whose every value is nil still decodes. The assertions follow what the
app actually does with a game now: resolve its seat to a deck, and put a
number on the screen.

## Reading the results

`CLAUDE.md` says it and it is worth repeating here: **report per archetype,
never as a single ranking.** Forge's own documentation says its AI is "best
with Aggro and midrange decks", "poor to Ok in control decks" and "pretty bad
for most combo decks". Arahbo and Atla Palani are what it plays well; Tivit and
Gyome are what it plays badly. A table that sorts all six by win rate is a
table about Forge's AI, not about the decks.

A game that hits `-c` is recorded as `timed_out` and reported separately from
draws — the clock giving up is a measurement problem, a draw is a game outcome.
The default here is 300s rather than Forge's 120s for that reason.
