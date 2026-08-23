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
So `run.py` writes `forge.profile.properties` into the Forge install pointing
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

* **Before**, in `coverage.py`, against `res/cardsfolder/cardsfolder.zip` —
  the same card scripts the engine loads, 33,587 files yielding 34,532 names in
  under two seconds, cached per (path, mtime, size) so a Forge upgrade
  invalidates it. `run_games` raises `CoverageFailed` rather than returning a
  flag, because a caller that could ignore it would.
* **After**, in `parse.py`, by scanning Forge's own output for that warning.
  `ResultsUntrustworthy` discards results that otherwise look perfectly normal.

All six curated decks pass the pre-flight with no missing cards.

## Hosted: the worker machine

The deployed app plays matches too, and holds none of the above (ADR 35).
`Dockerfile.forge` bakes the JRE, the pinned 2.0.14 distribution and
`sim/tier3/shim.py` into a worker image; the deploy workflow keeps a
dedicated-CPU machine named `forge-worker` pointed at it, stopped between
matches. The app wakes it per job over the Machines API, runs the same
pre-flight against the worker's own card scripts (`/coverage`, on the
request thread, so a 422 still names the cards before any JVM boots), plays
the match over the private network (`/match`), and the shim stops its own
machine after `MTGLAB_FORGE_IDLE_SECONDS` of quiet. Results are rebuilt into
the same `SimRun` a local run returns — `sim/tier3/wire.py` is the seam, and
`tests/test_forge_worker.py` pins that both halves shape identically.
`docs/HOSTING.md` §7 has the provisioning runbook and the licensing note
(the image holds GPL'd Forge and is pushed only to the app's private
registry — deployment, never distribution).

**Since Phase 7 of the Go migration (2026-08-23) the worker runs `mtglab
forge-shim`**, a subcommand of the same binary the app's front door is, so
the image carries no interpreter at all. `shim.py` is still the CLI's local
path and still the reference the Go side is held to.

### Testing deploy skew on purpose

Every release updates the app **before** the worker, by several minutes and
deliberately: the app deploy is proven first, so a red worker sync is
feedback about the worker rather than a rollback of the app. That leaves a
real window in which the app talks to the *previous* image's shim, and on the
release that flipped this package the previous image was Python's.

That window opened on v195 and closed about a minute before the gate match
started — and it can never open that way again, because both images carry the
Go shim now. **The case is permanent even though the opportunity was not**,
so it is tested here rather than waited for. With a distribution present:

```bash
MTGLAB_FORGE_HOME=~/.local/share/mtglab/forge \
MTGLAB_FORGE_SHIM_PORT=8899 MTGLAB_FORGE_IDLE_SECONDS=0 \
  python -m mtglab.sim.tier3.shim &

cd go && MTGLAB_LIVE_FORGE=1 MTGLAB_PYTHON_SHIM_URL=http://127.0.0.1:8899 \
  go test ./cmd/... -run PythonShim -v
```

The Go worker client then drives the Python shim through a real match. Five
deliberate wire renames die against it — and the first version of that test
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
