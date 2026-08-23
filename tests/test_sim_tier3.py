"""Tier 3: the `.dck` exporter, the coverage pre-flight, and the result parser.

No JVM, no Forge install, no network. Everything Forge-shaped in here is a
fixture built from strings Forge really printed, captured during the
feasibility spike -- which is the point: the parser has to keep matching
Forge's output, and a test that needs a 467 MB distribution to run is a test
that stops running.

The tests that *would* need Forge (`run.run_games` end to end) are deliberately
absent rather than skipped. What they would prove -- that a game completes --
is a property of Forge, not of this code.
"""

from __future__ import annotations

import io
import subprocess
from pathlib import Path

import pytest

from mtglab.decks.model import CardEntry, Deck
from mtglab.sim.tier3 import coverage, dck, parse


def make_deck(**kw) -> Deck:
    cards = kw.pop("cards", [CardEntry(name="Sol Ring", category="ramp", why="x")])
    return Deck(slug=kw.pop("slug", "test-deck"), name=kw.pop("name", "Test Deck"),
                commander=kw.pop("commander", ["Gyome, Master Chef"]),
                cards=cards, **kw)


# ------------------------------------------------------------- the exporter

def test_dck_has_the_sections_forge_ships():
    text = dck.to_dck(make_deck())
    assert text.startswith("[metadata]\nname=Test Deck\n")
    assert "[Commander]\n1 Gyome, Master Chef\n" in text
    assert "[Main]\n1 Sol Ring\n" in text
    # Every .dck Forge ships ends with the section, empty or not.
    assert text.rstrip().endswith("[Sideboard]")


def test_companion_goes_in_the_sideboard():
    """Forge has no companion section -- checked against its DeckSection enum."""
    text = dck.to_dck(make_deck(companion="Kaheera, the Orphanguard"))
    _, sideboard = text.split("[Sideboard]")
    assert sideboard.strip() == "1 Kaheera, the Orphanguard"


def test_quantities_are_written_not_expanded():
    text = dck.to_dck(make_deck(cards=[
        CardEntry(name="Forest", category="land", why="x", qty=36)]))
    assert "36 Forest" in text
    assert "1 Forest" not in text


def test_swap_board_is_not_exported():
    """The swap board is a list of candidates, not part of the 99."""
    deck = make_deck(cards=[CardEntry(name="Sol Ring", category="ramp", why="x")])
    deck.swap_board = [CardEntry(name="Mana Crypt", category="ramp", why="x")]
    assert "Mana Crypt" not in dck.to_dck(deck)


def test_names_are_remapped_from_the_coverage_report():
    """What gets written is exactly what the pre-flight verified."""
    deck = make_deck(cards=[
        CardEntry(name="Bala Ged Recovery // Bala Ged Sanctuary",
                  category="recursion", why="x")])
    text = dck.to_dck(deck, {"Bala Ged Recovery // Bala Ged Sanctuary":
                             "Bala Ged Recovery"})
    assert "1 Bala Ged Recovery\n" in text
    assert "//" not in text


def test_an_unmapped_name_is_written_unchanged_not_dropped():
    """Dropping it here would be the exact silent failure the pre-flight
    exists to catch, reproduced inside our own code."""
    text = dck.to_dck(make_deck(), names={})
    assert "1 Sol Ring" in text


def test_no_printing_is_pinned():
    """`|SET|n` would turn a Forge-side edition rename into a missing card."""
    assert "|" not in dck.to_dck(make_deck())


def test_write_dck_names_the_file_for_the_slug(tmp_path):
    path = dck.write_dck(make_deck(slug="gyome-food"), tmp_path)
    assert path.name == "gyome-food.dck"
    assert path.read_text(encoding="utf-8").startswith("[metadata]")


#: Slugs that must never become a filename. `{}` is filled with a directory
#: unique to the running test, so the escape target is this run's own and a
#: leftover from anywhere else cannot make the assertion pass or fail.
#:
#: That is not fastidiousness: proving this guard means *actually performing
#: the escape* with the guard removed, and the first mutation run of it wrote
#: a real `/tmp/escaped.dck` that then failed the next full suite. A test whose
#: subject is a file outside its own sandbox is a test with a memory.
HOSTILE_SLUGS = [
    "../escaped", "../../escaped", "{}/escaped", "sub/escaped",
    # `<slug>.dck` goes on Forge's own command line after `-d`, so a leading
    # hyphen is read as a flag rather than as a deck.
    "-n", "--help",
    "ok\x00.dck", "Escaped", "a.b", "", "two words", "trailing-",
]


@pytest.mark.parametrize("slug", HOSTILE_SLUGS)
def test_a_hostile_slug_cannot_name_a_file(tmp_path, slug):
    """`create` and `import` check a slug because it becomes a directory name.
    A deck arriving as deck.yaml **text** -- over the private network, from the
    app to the worker (ADR 35) -- never passes either, and `Deck.from_text`
    reads whatever `slug:` says.

    The assertions go past the refusal on purpose: a test that only checked
    for a raise could not tell "refused" from "wrote it somewhere else", which
    is the whole thing being prevented. Found by CodeQL on the Go port and
    fixed in both runtimes at once.
    """
    sandbox = tmp_path / "inside"
    sandbox.mkdir()
    slug = slug.format(tmp_path / "absolute")
    with pytest.raises(ValueError, match="not a usable slug"):
        dck.write_dck(make_deck(slug=slug), sandbox)
    assert list(sandbox.iterdir()) == [], "the refused write left something"
    # Nothing appeared anywhere under this run's own directory, which is where
    # every escape above was aimed.
    assert [p.name for p in tmp_path.rglob("*.dck")] == []


def test_an_ordinary_slug_still_writes_where_it_should(tmp_path):
    """The control. Without it the table above passes against a `write_dck`
    that refused everything."""
    path = dck.write_dck(make_deck(slug="arahbo-cats"), tmp_path)
    assert path == tmp_path / "arahbo-cats.dck"
    assert path.exists()


# ------------------------------------------------------------- the coverage

INDEX = frozenset({"Sol Ring", "Gyome, Master Chef", "Forest",
                   "Bala Ged Recovery", "Bala Ged Sanctuary",
                   "Kaheera, the Orphanguard", "Alive", "Well"})


def test_a_fully_implemented_deck_passes():
    report = coverage.check(make_deck(), INDEX)
    assert report.ok
    assert report.missing == []
    assert report.resolved["Sol Ring"] == "Sol Ring"


def test_a_missing_card_is_named():
    deck = make_deck(cards=[CardEntry(name="Chicken Troupe", category="threat",
                                      why="x")])
    report = coverage.check(deck, INDEX)
    assert not report.ok
    assert report.missing == ["Chicken Troupe"]
    assert "Chicken Troupe" in report.summary()


def test_the_commander_and_companion_are_checked_too():
    deck = make_deck(commander=["Nonesuch, the Unprinted"],
                     companion="Kaheera, the Orphanguard")
    report = coverage.check(deck, INDEX)
    assert report.missing == ["Nonesuch, the Unprinted"]
    assert "Kaheera, the Orphanguard" in report.resolved


def test_a_scryfall_double_name_resolves_to_a_face():
    """Forge's index holds face names only -- never Scryfall's `A // B`."""
    assert coverage.resolve("Bala Ged Recovery // Bala Ged Sanctuary",
                            INDEX) == "Bala Ged Recovery"
    assert coverage.resolve("Alive // Well", INDEX) == "Alive"
    assert coverage.resolve("Nothing // Doing", INDEX) is None


def test_duplicate_names_are_counted_once():
    """Thirty-six missing Forests would be one problem, not thirty-six."""
    deck = make_deck(cards=[
        CardEntry(name="Forest", category="land", why="x", qty=36),
        CardEntry(name="Sol Ring", category="ramp", why="x")])
    report = coverage.check(deck, INDEX)
    assert report.checked == 3  # commander + Forest + Sol Ring


def test_renamed_lists_only_what_actually_changed():
    deck = make_deck(cards=[
        CardEntry(name="Bala Ged Recovery // Bala Ged Sanctuary",
                  category="recursion", why="x"),
        CardEntry(name="Sol Ring", category="ramp", why="x")])
    report = coverage.check(deck, INDEX)
    assert report.renamed == [("Bala Ged Recovery // Bala Ged Sanctuary",
                               "Bala Ged Recovery")]


# --------------------------------------------------------------- the parser

# Captured from a real run. Forge prints the turn count before the result line
# it belongs to, which is why the parser holds it.
WON = """\
Ai(1)-Arahbo, Roar of the World — Cats vs Ai(2)-Atla Palani — Naya - one game
Game Outcome: Turn 11
Game Outcome: Ai(1)-Arahbo, Roar of the World — Cats has lost because life total reached 0
Game Outcome: Ai(2)-Atla Palani — Naya has won because all opponents have lost
Match Result: Ai(1)-Arahbo, Roar of the World — Cats: 0 Ai(2)-Atla Palani — Naya: 1

Game Result: Game 1 ended in 16702 ms. Ai(2)-Atla Palani — Naya has won!
"""

# The reason coverage.py exists: three cards Forge could not find, a game
# played anyway with 96 cards, and a result line that says nothing is wrong.
DROPPED = """\
An unsupported card was requested: "Nonexistent Card 1" from "[N.A.]".
Forge could not find this card in the Database. Any chance you might have mistyped the card name?
An unsupported card was requested: "Nonexistent Card 2" from "[N.A.]".
Game Outcome: Turn 9
Game Result: Game 1 ended in 7212 ms. Ai(2)-Atla Palani — Naya has won!
"""


def test_a_win_parses_with_its_seat_and_timing():
    out = parse.parse(WON)
    assert len(out.games) == 1
    game = out.games[0]
    assert game.index == 1
    assert game.milliseconds == 16702
    assert game.turns == 11
    assert game.draw is False
    assert game.winner_seat == 2
    assert out.trustworthy


def test_a_draw_parses():
    out = parse.parse("Game Outcome: Turn 24\n"
                      "Game Result: Game 3 ended in a Draw! Took 51234 ms.\n")
    game = out.games[0]
    assert game.draw and game.winner is None and game.winner_seat is None
    assert game.milliseconds == 51234
    assert game.turns == 24


def test_a_clock_out_is_distinguished_from_a_real_draw():
    """`-c` expiring is a measurement problem; a draw is a game outcome."""
    out = parse.parse("Stopping slow match as draw\n"
                      "Game Result: Game 1 ended in a Draw! Took 300100 ms.\n")
    assert out.games[0].draw and out.games[0].timed_out


def test_a_dropped_card_makes_the_run_untrustworthy():
    out = parse.parse(DROPPED)
    # Forge still reported a perfectly ordinary-looking result.
    assert len(out.games) == 1 and out.games[0].winner_seat == 2
    # And it is worthless.
    assert not out.trustworthy
    assert out.unsupported == ["Nonexistent Card 1", "Nonexistent Card 2"]


def test_a_deck_that_would_not_load_is_untrustworthy():
    out = parse.parse("Could not load deck - cats.dck, match cannot start\n")
    assert not out.trustworthy
    assert out.deck_load_failures == ["cats.dck"]


def test_several_games_parse_in_order():
    text = "".join(
        f"Game Outcome: Turn {n}\n"
        f"Game Result: Game {n} ended in {n * 1000} ms. Ai(1)-X has won!\n"
        for n in (1, 2, 3))
    out = parse.parse(text)
    assert [g.index for g in out.games] == [1, 2, 3]
    assert [g.turns for g in out.games] == [1, 2, 3]
    assert all(g.winner_seat == 1 for g in out.games)


def test_noise_is_ignored():
    """Forge interleaves card-database complaints and AI warnings throughout."""
    out = parse.parse(
        "The card Treetop Recluse was not assigned to any set. Adding it...\n"
        "Warning: default implementation of confirmAction is used by X\n"
        "Read cards: 33617 archived files in 1 ms\n")
    assert out.games == [] and out.trustworthy


# ------------------------------------------------------ the run-level guard

def test_run_games_refuses_fewer_than_two_decks():
    from mtglab.sim.tier3 import run
    with pytest.raises(ValueError, match="at least two decks"):
        run.run_games([make_deck()])


def test_the_subprocess_timeout_is_derived_not_unbounded(monkeypatch):
    """Forge's `-c` bounds a game; nothing bounded the process until now, and
    one measured game ran 134 seconds. The bound is the interval on the timer
    that kills the JVM, so that is what this captures."""
    captured: dict = {}
    run = _stub_forge(monkeypatch, WON, captured)
    run.run_games([make_deck(), make_deck()], games=4, clock=300)
    assert captured["timer_seconds"] == 60 + 4 * 300


def test_check_coverage_raises_rather_than_returning_a_flag(monkeypatch):
    """A caller that could ignore the pre-flight would eventually ignore it."""
    from mtglab.sim.tier3 import run
    monkeypatch.setattr(run, "implemented_names", lambda *a, **k: INDEX)
    deck = make_deck(cards=[CardEntry(name="Chicken Troupe", category="threat",
                                      why="x")])
    with pytest.raises(run.CoverageFailed, match="Chicken Troupe"):
        run.check_coverage([deck])


# ---------------------------------------------------- setup, before any game
#
# `sim/tier3/run.py` sat at 50%: everything that shells out to Forge was
# untestable without a 467 MB download, and the *setup* code around it went
# untested by association. That code is pure filesystem and environment logic,
# and it is where the confusing failures live -- a wrong Java gets blamed on
# Forge, and a profile written into the wrong directory mixes generated decks
# into whatever the user saved by hand.

def _fake_java(path, version: str):
    """A script that answers `-version` the way a JVM does: on stderr."""
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        f'#!/bin/sh\necho \'openjdk version "{version}" 2026-01-01\' >&2\n',
        encoding="utf-8")
    path.chmod(0o755)
    return path


def test_the_java_override_is_used_when_it_is_new_enough(tmp_path, monkeypatch):
    from mtglab.sim.tier3 import run as forge
    java = _fake_java(tmp_path / "jdk21" / "bin" / "java", "21.0.2")
    monkeypatch.setenv("MTGLAB_JAVA", str(java))
    assert forge.java_binary() == java


def test_a_java_that_is_too_old_is_rejected_rather_than_used(tmp_path, monkeypatch):
    """The specific trap: this machine's /usr/bin/java is 10, and Forge fails
    on it in a way that reads like a Forge bug rather than a Java one."""
    from mtglab.sim.tier3 import run as forge
    old = _fake_java(tmp_path / "old" / "bin" / "java", "10.0.1")
    monkeypatch.setenv("MTGLAB_JAVA", str(old))
    monkeypatch.setattr(forge.shutil, "which", lambda _: None)
    monkeypatch.setattr(forge, "BUNDLED_JDK", tmp_path / "absent")

    with pytest.raises(forge.ForgeNotInstalled) as exc:
        forge.java_binary()
    # The version it found has to appear, or the advice is unactionable.
    assert "10" in str(exc.value)
    assert "MTGLAB_JAVA" in str(exc.value)


def test_a_java_that_does_not_exist_is_skipped_not_crashed_on(tmp_path,
                                                              monkeypatch):
    from mtglab.sim.tier3 import run as forge
    good = _fake_java(tmp_path / "jdk21" / "bin" / "java", "21.0.2")
    monkeypatch.setenv("MTGLAB_JAVA", str(tmp_path / "nothing" / "java"))
    monkeypatch.setattr(forge, "BUNDLED_JDK", tmp_path / "absent")
    monkeypatch.setattr(forge.shutil, "which", lambda _: str(good))
    assert forge.java_binary() == good


def test_no_java_at_all_says_what_was_checked(tmp_path, monkeypatch):
    from mtglab.sim.tier3 import run as forge
    monkeypatch.delenv("MTGLAB_JAVA", raising=False)
    monkeypatch.setattr(forge, "BUNDLED_JDK", tmp_path / "absent")
    monkeypatch.setattr(forge.shutil, "which", lambda _: None)
    with pytest.raises(forge.ForgeNotInstalled) as exc:
        forge.java_binary()
    assert "nothing" in str(exc.value)


def test_the_newest_desktop_jar_wins(tmp_path):
    """Forge distributions accumulate jars across upgrades; running an old one
    silently plays with an old card pool."""
    from mtglab.sim.tier3 import run as forge
    for version in ("1.6.50", "1.6.9", "2.0.1"):
        (tmp_path / f"forge-gui-desktop-{version}-jar-with-dependencies.jar").touch()
    assert forge.desktop_jar(tmp_path).name == \
        "forge-gui-desktop-2.0.1-jar-with-dependencies.jar"


def test_a_directory_with_no_jar_names_the_env_var(tmp_path):
    from mtglab.sim.tier3 import run as forge
    with pytest.raises(forge.ForgeNotInstalled) as exc:
        forge.desktop_jar(tmp_path)
    assert "MTGLAB_FORGE_HOME" in str(exc.value)


def test_forge_version_reads_the_jar_name_and_never_raises(tmp_path):
    """The match ledger's provenance column (ADR 36): the version is the jar
    name's middle, and a directory with no Forge answers None -- the recorder
    must not fail a match over a name it cannot parse."""
    from mtglab.sim.tier3 import run as forge
    (tmp_path / "forge-gui-desktop-2.0.14-jar-with-dependencies.jar").touch()
    assert forge.forge_version(tmp_path) == "2.0.14"
    assert forge.forge_version(tmp_path / "empty") is None


def test_an_unreadable_home_is_not_installed_rather_than_a_crash(tmp_path):
    """The deployed 500 of 2026-08-20: `Path.home()` is `/root` in the
    container while the app runs as `mtglab`, so the probe's stat raised
    `PermissionError` and the gate at `/api/forge` answered 500 instead of
    `available: false`. A directory this process cannot look inside holds no
    Forge this process can run."""
    from mtglab.sim.tier3 import run as forge
    locked = tmp_path / "locked"
    locked.mkdir()
    locked.chmod(0o000)
    try:
        with pytest.raises(forge.ForgeNotInstalled):
            forge.desktop_jar(locked / "forge")
    finally:
        locked.chmod(0o755)


def test_the_profile_is_ours_not_the_users_own(tmp_path, monkeypatch):
    """Forge defaults to the user's own data directory. Writing generated decks
    there would mix them into whatever the person saved by hand."""
    from mtglab.sim.tier3 import run as forge
    monkeypatch.setenv("MTGLAB_FORGE_PROFILE", str(tmp_path / "mine"))
    assert forge.forge_profile() == tmp_path / "mine"

    monkeypatch.delenv("MTGLAB_FORGE_PROFILE")
    assert "mtglab" in str(forge.forge_profile())


def test_ensure_profile_writes_the_marker_and_returns_the_deck_dir(tmp_path,
                                                                  monkeypatch):
    from mtglab.sim.tier3 import run as forge
    home = tmp_path / "forge"
    home.mkdir()
    monkeypatch.setenv("MTGLAB_FORGE_PROFILE", str(tmp_path / "profile"))

    deck_dir = forge.ensure_profile(home)
    assert deck_dir == tmp_path / "profile" / "decks" / "commander"
    assert deck_dir.is_dir()

    marker = home / "forge.profile.properties"
    assert f"userDir={tmp_path / 'profile'}" in marker.read_text()


def test_ensure_profile_does_not_rewrite_an_unchanged_marker(tmp_path,
                                                             monkeypatch):
    """It reaches into a shared Forge install, so a no-op run must be a no-op
    on disk."""
    from mtglab.sim.tier3 import run as forge
    home = tmp_path / "forge"
    home.mkdir()
    monkeypatch.setenv("MTGLAB_FORGE_PROFILE", str(tmp_path / "profile"))

    forge.ensure_profile(home)
    marker = home / "forge.profile.properties"
    before = marker.stat().st_mtime_ns
    forge.ensure_profile(home)
    assert marker.stat().st_mtime_ns == before


def test_ensure_profile_rewrites_when_the_profile_moves(tmp_path, monkeypatch):
    from mtglab.sim.tier3 import run as forge
    home = tmp_path / "forge"
    home.mkdir()
    monkeypatch.setenv("MTGLAB_FORGE_PROFILE", str(tmp_path / "first"))
    forge.ensure_profile(home)

    monkeypatch.setenv("MTGLAB_FORGE_PROFILE", str(tmp_path / "second"))
    forge.ensure_profile(home)
    assert f"userDir={tmp_path / 'second'}" in \
        (home / "forge.profile.properties").read_text()


def test_ensure_profile_refuses_a_missing_forge_home(tmp_path):
    from mtglab.sim.tier3 import run as forge
    with pytest.raises(forge.ForgeNotInstalled):
        forge.ensure_profile(tmp_path / "absent")


# ------------------------------------------- the card index, from a real zip

def _forge_home(tmp_path, names=("Sol Ring", "Forest")):
    """An unpacked-Forge lookalike: just the cardsfolder zip, three entries."""
    import zipfile
    home = tmp_path / "forge"
    folder = home / "res" / "cardsfolder"
    folder.mkdir(parents=True)
    with zipfile.ZipFile(folder / "cardsfolder.zip", "w") as z:
        for name in names:
            slug = name.lower().replace(" ", "_")
            z.writestr(f"{slug[:1]}/{slug}.txt",
                       f"Name:{name}\nManaCost:1\nTypes:Artifact\n")
        z.writestr("readme.md", "not a card script")
        z.writestr("s/", "")   # a directory entry, skipped
    return home


def test_implemented_names_reads_forges_own_scripts(tmp_path):
    names = coverage.implemented_names(_forge_home(tmp_path))
    assert names == frozenset({"Sol Ring", "Forest"})


def test_implemented_names_is_cached_until_the_zip_changes(tmp_path):
    home = _forge_home(tmp_path)
    first = coverage.implemented_names(home)
    assert coverage.implemented_names(home) is first, \
        "same path, same mtime, same size -- the index must not be re-read"


def test_a_missing_distribution_names_the_env_var(tmp_path):
    with pytest.raises(coverage.ForgeNotInstalled, match="MTGLAB_FORGE_HOME"):
        coverage.implemented_names(tmp_path / "nowhere")


def test_a_clean_report_mentions_its_face_renames():
    deck = make_deck(cards=[
        CardEntry(name="Bala Ged Recovery // Bala Ged Sanctuary",
                  category="utility", why="x")])
    report = coverage.check(deck, INDEX)
    assert report.ok
    assert "resolved to a face name" in report.summary()


# ---------------------------------------------- the run, faked at subprocess

class _FakeProc:
    """What `Popen` hands back, shaped for the streaming read loop."""

    def __init__(self, text: str):
        self.stdout = io.StringIO(text)
        self.returncode = 0
        self.killed = False

    def wait(self, timeout=None):
        return self.returncode

    def kill(self):
        self.killed = True


def _stub_forge(monkeypatch, stdout: str, captured: dict | None = None,
                fire_timer: bool = False):
    """Fake the JVM at the `Popen` seam `run_games` actually uses.

    `fire_timer: True` runs the deadline callback the moment the timer
    starts, which is how the timeout path is driven without waiting for one.
    """
    from mtglab.sim.tier3 import run

    monkeypatch.setattr(run, "implemented_names", lambda *a, **k: INDEX)
    monkeypatch.setattr(run, "ensure_profile", lambda *a, **k: None)
    monkeypatch.setattr(run.dck, "write_dck",
                        lambda d, *a, **k: type("P", (), {"name": "x.dck"})())
    monkeypatch.setattr(run, "java_binary", lambda: "java")
    monkeypatch.setattr(run, "desktop_jar", lambda *a: "forge.jar")

    def fake_popen(argv, **kw):
        proc = _FakeProc(stdout)
        if captured is not None:
            captured["argv"] = argv
            captured["proc"] = proc
        return proc
    monkeypatch.setattr(run.subprocess, "Popen", fake_popen)

    class FakeTimer:
        def __init__(self, seconds, fn):
            if captured is not None:
                captured["timer_seconds"] = seconds
            self.fn = fn

        def start(self):
            if fire_timer:
                self.fn()

        def cancel(self):
            pass
    monkeypatch.setattr(run.threading, "Timer", FakeTimer)
    return run


def test_run_games_maps_seats_and_carries_the_seed(monkeypatch):
    """The full path around the subprocess: Ai(2) resolves to the second
    deck's slug, startup time is wall minus play, and `-s` rides the argv."""
    captured: dict = {}
    run = _stub_forge(monkeypatch, WON, captured)
    a = make_deck()
    b = make_deck(slug="second", commander=["Gyome, Master Chef"])
    result = run.run_games([a, b], games=1, clock=300, seed=7)
    assert "-s" in captured["argv"] and "7" in captured["argv"]
    game = result.games[0]
    assert result.winner_slug(game) == "second"
    assert result.startup_seconds >= 0.0


def test_run_games_ticks_once_per_finished_game(monkeypatch):
    """The output is streamed and `on_game` hears each result line as it
    passes — the count and, since the match theater, the game it completed.
    Ticks are progress, never results: the tally is the parser's complete
    output, and the strongest guarantee available is asserted below — the
    game a tick carried IS the tally's game, by identity, because both come
    off one `StreamParser`."""
    two_games = WON + "Game Result: Game 2 ended in a Draw! Took 9000 ms.\n"
    run = _stub_forge(monkeypatch, two_games)
    ticks: list[int] = []
    heard: list[object] = []

    def on_game(n, game):
        ticks.append(n)
        heard.append(game)

    result = run.run_games([make_deck(), make_deck()], games=2,
                           on_game=on_game)
    assert ticks == [1, 2]
    assert len(result.games) == 2
    assert all(a is b for a, b in zip(heard, result.games, strict=True))


def test_the_stream_parser_hands_back_each_game_as_it_completes():
    """The incremental half of `parse`: a held turn line attaches to the
    result that follows it, noise returns nothing, and what `feed` hands back
    is the very object the accumulated output keeps."""
    sp = parse.StreamParser()
    assert sp.feed("Game Outcome: Turn 11") is None
    game = sp.feed("Game Result: Game 1 ended in 16702 ms. Ai(2)-X has won!")
    assert game is not None
    assert (game.turns, game.winner_seat) == (11, 2)
    assert sp.feed("some interleaved AI chatter") is None
    assert sp.feed("Stopping slow match as draw") is None
    clocked = sp.feed("Game Result: Game 2 ended in 300000 ms. Ai(1)-X has won!")
    assert clocked is not None and clocked.timed_out
    assert sp.output.games == [game, clocked]
    assert sp.output.games[0] is game


def test_a_tick_line_and_a_parsed_result_are_the_same_pattern():
    """`is_game_result` shares the parser's own regexes; a line the tick
    counts is a line the tally will keep, and nothing else ticks."""
    assert parse.is_game_result(
        "Game Result: Game 1 ended in 16702 ms. Ai(2)-X has won!")
    assert parse.is_game_result(
        "Game Result: Game 2 ended in a Draw! Took 9000 ms.")
    assert not parse.is_game_result("Game Outcome: Turn 11")
    assert not parse.is_game_result(
        'An unsupported card was requested: "X" from "[N.A.]".')


def test_a_run_past_its_deadline_is_killed_and_raises(monkeypatch):
    """The deadline is a timer that kills the JVM; the read loop then ends at
    EOF and the flag tells a killed run from a finished one."""
    captured: dict = {}
    run = _stub_forge(monkeypatch, WON, captured, fire_timer=True)
    with pytest.raises(subprocess.TimeoutExpired):
        run.run_games([make_deck(), make_deck()], games=1)
    assert captured["proc"].killed


def test_a_dropped_card_raises_rather_than_reporting(monkeypatch):
    """The Forge-plays-on-with-96-cards failure: the result parses fine and
    must still be refused, because a flag would eventually be ignored."""
    run = _stub_forge(monkeypatch, DROPPED)
    with pytest.raises(run.ResultsUntrustworthy, match="dropped card"):
        run.run_games([make_deck(), make_deck()], games=1)


def test_a_run_with_no_games_at_all_is_refused_with_forges_output(monkeypatch):
    run = _stub_forge(monkeypatch, "Read cards: 33617 archived files in 1 ms\n")
    with pytest.raises(run.ResultsUntrustworthy, match="no game results"):
        run.run_games([make_deck(), make_deck()], games=1)


def test_a_java_probe_that_cannot_execute_is_none(monkeypatch):
    from mtglab.sim.tier3 import run

    def boom(argv, **kw):
        raise OSError("not executable")
    monkeypatch.setattr(run.subprocess, "run", boom)
    assert run._java_major(Path("java-that-is-not-there")) is None
