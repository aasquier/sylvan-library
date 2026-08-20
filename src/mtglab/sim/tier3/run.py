"""Find Forge, hand it decks, run games, time them.

Three facts about `forge.jar sim` that the shape of this module comes from, all
established by running it rather than by reading the wiki:

1. **`-D` does not work for a single match.** The help text advertises it as
   "absolute directory to load decks from", but the string lives in
   `simulateTournament`; the single-match path resolves deck names against
   Forge's user profile and ignores `-D` entirely. So decks are placed where
   Forge will look, and the supported way to move that is
   `forge.profile.properties`.
2. **Forge must run with its own directory as the working directory**, the way
   `forge.sh` does, or it cannot find `res/`.
3. **An unimplemented card does not stop a game.** It prints a warning and
   plays on. Hence `check_coverage` below, and hence this module raising rather
   than returning when the pre-flight fails -- a caller that could ignore it
   would eventually ignore it.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import threading
import time
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path

from mtglab import config
from mtglab.decks.model import Deck
from mtglab.sim.tier3 import dck, parse
from mtglab.sim.tier3.coverage import (
    CoverageReport,
    ForgeNotInstalled,
    check,
    implemented_names,
)

# Forge needs 17+. It runs on 21 here; the floor is what matters.
JAVA_MINIMUM = 17

BUNDLED_JDK = Path.home() / ".local" / "share" / "mtglab" / "jdk-21"


class CoverageFailed(RuntimeError):
    """A deck contains cards Forge does not implement. Nothing may run."""


class ResultsUntrustworthy(RuntimeError):
    """Forge itself reported a dropped card or an unloadable deck.

    The second of the two coverage checks, and the one that fires if a name
    ever slips past the index. Raised *after* a run, discarding results that
    otherwise look perfectly normal.
    """


# ------------------------------------------------------------------ the JVM

_VERSION = re.compile(r'version "(\d+)')


def _java_major(binary: Path) -> int | None:
    try:
        proc = subprocess.run([str(binary), "-version"], capture_output=True,
                              text=True, timeout=30)
    except (OSError, subprocess.SubprocessError):
        return None
    # `java -version` writes to stderr. Has done for decades.
    match = _VERSION.search(proc.stderr or proc.stdout or "")
    return int(match.group(1)) if match else None


def java_binary() -> Path:
    """A JVM new enough to run Forge.

    `MTGLAB_JAVA` wins, then the JDK unpacked beside the Forge distribution,
    then whatever is on PATH. The PATH entry is checked rather than trusted:
    this machine's `/usr/bin/java` is 10.0.1, which fails Forge in a way that
    reads like a Forge bug rather than a Java one.
    """
    candidates: list[Path] = []
    override = os.environ.get("MTGLAB_JAVA")
    if override:
        candidates.append(Path(override))
    candidates.append(BUNDLED_JDK / "Contents" / "Home" / "bin" / "java")
    candidates.append(BUNDLED_JDK / "bin" / "java")
    found = shutil.which("java")
    if found:
        candidates.append(Path(found))

    tried: list[str] = []
    for candidate in candidates:
        if not candidate.exists():
            continue
        major = _java_major(candidate)
        if major is not None and major >= JAVA_MINIMUM:
            return candidate
        tried.append(f"{candidate} (Java {major})")

    raise ForgeNotInstalled(
        f"no Java {JAVA_MINIMUM}+ found -- set MTGLAB_JAVA to one. Checked: "
        + (", ".join(tried) or "nothing"))


def desktop_jar(forge_home: Path | None = None) -> Path:
    home = Path(forge_home) if forge_home is not None else config.forge_home()
    # A directory this process cannot even look inside is a directory with no
    # Forge in it. Deployed, `Path.home()` is `/root` while the app runs as
    # `mtglab`, so the glob's stat raises `PermissionError` -- and the gate at
    # `/api/forge` answered 500 instead of `available: false` until this was
    # caught on the live instance (2026-08-20).
    try:
        jars = sorted(home.glob("forge-gui-desktop-*-jar-with-dependencies.jar"))
    except OSError as exc:
        raise ForgeNotInstalled(
            f"no Forge distribution readable at {home} ({exc}) -- set "
            f"MTGLAB_FORGE_HOME to an unpacked Forge distribution") from exc
    if not jars:
        raise ForgeNotInstalled(
            f"no Forge desktop jar in {home} -- set MTGLAB_FORGE_HOME to an "
            f"unpacked Forge distribution")
    return jars[-1]


_JAR_VERSION = re.compile(r"^forge-gui-desktop-(.+)-jar-with-dependencies\.jar$")


def forge_version(forge_home: Path | None = None) -> str | None:
    """Which Forge is installed, read off the jar's own name.

    The match ledger records it (ADR 36): Forge's AI is the instrument every
    recorded game was measured with, and an upgrade changes the instrument --
    ratings computed across an unversioned upgrade would silently mix two
    judges. `None` when the name does not parse, which the ledger stores as
    "not reported" rather than guessing.
    """
    try:
        match = _JAR_VERSION.match(Path(desktop_jar(forge_home)).name)
    except ForgeNotInstalled:
        return None
    return match.group(1) if match else None


# -------------------------------------------------------------- the profile

def forge_profile() -> Path:
    """Forge's user data directory, ours rather than the user's own.

    Forge defaults to `~/Library/Application Support/Forge` on macOS. Writing
    generated decks into that would mix machine-generated files into whatever
    the person has saved by hand, so `forge.profile.properties` points Forge at
    a directory this project owns. `MTGLAB_FORGE_PROFILE` overrides.
    """
    override = os.environ.get("MTGLAB_FORGE_PROFILE")
    if override:
        return Path(override)
    return Path.home() / ".local" / "share" / "mtglab" / "forge-profile"


def ensure_profile(forge_home: Path | None = None) -> Path:
    """Write `forge.profile.properties` and return the commander deck dir.

    This is the one thing that reaches into the Forge installation, because
    Forge reads that file from its own program directory and nowhere else. It
    is rewritten only when the contents would change, so a run does not
    needlessly touch a shared install.
    """
    home = Path(forge_home) if forge_home is not None else config.forge_home()
    if not home.exists():
        raise ForgeNotInstalled(f"no Forge distribution at {home}")

    profile = forge_profile()
    wanted = (f"userDir={profile}\n"
              f"cacheDir={profile / 'cache'}\n"
              f"cardPicsDir={profile / 'cache' / 'pics'}\n")
    marker = home / "forge.profile.properties"
    if not marker.exists() or marker.read_text(encoding="utf-8") != wanted:
        marker.write_text(wanted, encoding="utf-8")

    deck_dir = profile / "decks" / "commander"
    deck_dir.mkdir(parents=True, exist_ok=True)
    return deck_dir


# -------------------------------------------------------------- the running

@dataclass
class SimRun:
    """One `forge sim` invocation, and what it cost."""

    argv: list[str]
    output: parse.SimOutput
    # Wall clock for the whole subprocess: JVM start, card database load, and
    # every game. The startup share is fixed, so it amortises across `-n`.
    wall_seconds: float
    # Seat number (1-based, the order decks were passed) -> deck slug.
    seats: dict[int, str] = field(default_factory=dict)
    coverage: list[CoverageReport] = field(default_factory=list)
    # Which Forge played, from the jar's name -- or None when the run was
    # rebuilt from a wire payload that predates the field (deploy skew).
    forge_version: str | None = None

    @property
    def games(self) -> list[parse.GameResult]:
        return self.output.games

    @property
    def startup_seconds(self) -> float:
        """Wall time not spent inside a game -- JVM boot and the card database.

        The number that decides whether a hosted Forge is one long-lived
        process or a subprocess per request.
        """
        played = sum(g.milliseconds for g in self.games) / 1000
        return max(0.0, self.wall_seconds - played)

    def winner_slug(self, game: parse.GameResult) -> str | None:
        if game.winner_seat is None:
            return None
        return self.seats.get(game.winner_seat)


def raise_unless_covered(reports: list[CoverageReport]) -> None:
    """One message format for a failed pre-flight, wherever it was computed.

    Split out of `check_coverage` when the worker landed (ADR 35): the shim
    computes reports on the worker machine and `worker.py` re-raises them on
    the request thread, and two hand-written copies of this message would
    drift the day one of them was edited.
    """
    broken = [r for r in reports if not r.ok]
    if broken:
        raise CoverageFailed(
            "Forge does not implement every card, so no result would mean "
            "anything:\n" + "\n".join(r.summary() for r in broken))


def check_coverage(decks: list[Deck],
                   forge_home: Path | None = None) -> list[CoverageReport]:
    """Pre-flight every deck. Raises `CoverageFailed` if any card is missing.

    Separated from `run_games` so a caller can pre-flight without a JVM -- it
    reads a zip and needs no Java at all, which makes it the cheap check to run
    first and the one an API can run on a request thread.
    """
    index = implemented_names(forge_home)
    reports = [check(deck, index) for deck in decks]
    raise_unless_covered(reports)
    return reports


def run_games(decks: list[Deck], *, games: int = 1, clock: int = 300,
              seed: int | None = None, memory_mb: int = 4096,
              forge_home: Path | None = None,
              timeout: float | None = None,
              on_game: Callable[[int, parse.GameResult], None] | None = None,
              ) -> SimRun:
    """Play `games` Commander games between `decks` and return the results.

    `clock` is Forge's `-c`: seconds before a game is called a draw. The
    default here is 300 rather than Forge's 120 because CLAUDE.md says so and
    because Tivit games really do run long -- a clock-out is recorded as
    `timed_out`, never quietly folded into the draw rate.

    Every deck is pre-flighted first, and the output is checked again
    afterwards. Both have to pass or this raises.

    `timeout` bounds the whole subprocess and defaults to something derived
    rather than to nothing. Forge's own `-c` bounds each *game*, so
    `games * clock` plus an allowance for JVM start is the honest ceiling --
    and an unbounded wait is not hypothetical here, since one measured Trostani
    game ran 134 seconds.

    `on_game` is called with the count of games finished so far (1, 2, ...)
    and the game just parsed, as each result line arrives -- what lets a job
    tick per game and the match theater show the row behind the tick. The
    output is streamed either way; the callback only decides whether anybody
    hears about it. Ticks are best-effort progress, never results -- the
    tally that matters is the parser's complete output below, after the
    second coverage check, and since #204 the ticks and the tally are one
    `StreamParser` rather than two readers sharing regexes.
    """
    if len(decks) < 2:
        raise ValueError("a game needs at least two decks")
    if timeout is None:
        timeout = 60 + games * clock

    reports = check_coverage(decks, forge_home)
    deck_dir = ensure_profile(forge_home)
    home = Path(forge_home) if forge_home is not None else config.forge_home()

    names: list[str] = []
    seats: dict[int, str] = {}
    for seat, (deck, report) in enumerate(zip(decks, reports, strict=True), 1):
        path = dck.write_dck(deck, deck_dir, report.resolved)
        names.append(path.name)
        seats[seat] = deck.slug

    argv = [
        str(java_binary()), f"-Xmx{memory_mb}m",
        "-Dio.netty.tryReflectionSetAccessible=true", "-Dfile.encoding=UTF-8",
        "-jar", str(desktop_jar(home)),
        "sim", "-d", *names, "-f", "Commander",
        "-n", str(games), "-c", str(clock), "-q",
    ]
    if seed is not None:
        argv += ["-s", str(seed)]

    # Forge writes the card-database complaints to stderr and the results to
    # stdout, but not reliably -- the unsupported-card warning arrives on both.
    # stderr is folded into stdout so one stream can be read live (the tick)
    # and parsed whole (the tally); the union is what makes the second
    # coverage check dependable, exactly as it was under `capture_output`.
    started = time.monotonic()
    proc = subprocess.Popen(argv, cwd=home, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, text=True)
    # A blocking readline honours no deadline of its own, so the deadline is a
    # timer that kills the JVM -- EOF then ends the read loop, and the flag is
    # what tells a killed run from a finished one.
    expired = threading.Event()

    def _out_of_time() -> None:
        expired.set()
        proc.kill()

    timer = threading.Timer(timeout, _out_of_time)
    timer.start()
    lines: list[str] = []
    parser = parse.StreamParser()
    finished_games = 0
    try:
        assert proc.stdout is not None  # PIPE above guarantees it
        for raw in proc.stdout:
            lines.append(raw)
            game = parser.feed(raw)
            if game is not None:
                finished_games += 1
                if on_game is not None:
                    on_game(finished_games, game)
        proc.wait()
    finally:
        timer.cancel()
    elapsed = time.monotonic() - started
    if expired.is_set():
        raise subprocess.TimeoutExpired(argv, timeout)

    text = "".join(lines)
    output = parser.output

    run = SimRun(argv=argv, output=output, wall_seconds=elapsed,
                 seats=seats, coverage=reports,
                 forge_version=forge_version(forge_home))

    if not output.trustworthy:
        raise ResultsUntrustworthy(
            "Forge reported problems that invalidate the run:\n"
            + "\n".join(
                [f"  dropped card: {n}" for n in output.unsupported]
                + [f"  deck failed to load: {n}"
                   for n in output.deck_load_failures]))
    if not output.games:
        raise ResultsUntrustworthy(
            f"Forge produced no game results in {elapsed:.1f}s. Last output:\n"
            + "\n".join(text.splitlines()[-15:]))
    return run
