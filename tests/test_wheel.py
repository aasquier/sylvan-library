"""The Wheel of Fortune (punch list 2026-08-15 item 9).

Deterministic Python rolling seeded dice over the tiny pool. Everything
here runs against `tiny_pool`'s 21 real cards, so the filters are checked
against oracle text Scryfall actually wrote.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import tiny_pool
from mtglab import config
from mtglab.cards import db
from mtglab.decks import wheel
from tiny_pool import mono_green_deck


@pytest.fixture
def pool(tmp_path):
    """A real DuckDB pool with 21 real cards, built in about a second."""
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_pool.build(config.DB_PATH)


def _spin(pool, seed):
    con = db.connect(pool)
    try:
        return wheel.spin(mono_green_deck(), frozenset({"G"}), con, seed=seed)
    finally:
        con.close()


def test_a_seeded_spin_is_reproducible(pool):
    once = _spin(pool, 7)
    again = _spin(pool, 7)
    assert once["symbol"] == again["symbol"]
    assert once["card"] == again["card"]
    assert once["seed"] == 7


def test_an_unseeded_spin_reports_the_seed_it_rolled(pool):
    con = db.connect(pool)
    try:
        out = wheel.spin(mono_green_deck(), frozenset({"G"}), con)
    finally:
        con.close()
    assert isinstance(out["seed"], int)
    # And that seed replays to the same fate and card.
    replay = _spin(pool, out["seed"])
    assert replay["symbol"] == out["symbol"]
    assert replay["card"] == out["card"]


def test_the_card_respects_identity_ban_and_the_deck_itself(pool):
    """Rules 2 and 4's perimeter, plus item 5's lesson: whatever fate comes
    up, the card is legal, castable here, and not already sleeved."""
    deck = mono_green_deck()
    in_deck = {c.name for c in deck.cards} | set(deck.commander)
    for seed in range(24):
        out = _spin(pool, seed)
        assert out["answered_by"] == "python"
        assert out["symbol"] in wheel.SYMBOLS
        if out["card"] is None:
            continue
        assert set(out["card"]["color_identity"]) <= {"G"}
        assert out["card"]["name"] not in in_deck
        assert out["card"]["name"] != "Primeval Titan"  # banned
        assert out["card"]["name"] != "Black Lotus"  # banned


def test_every_fate_is_reachable(pool):
    seen = {(_spin(pool, s))["symbol"] for s in range(40)}
    assert seen == set(wheel.SYMBOLS)


def test_the_caveat_says_which_system_answered(pool):
    out = _spin(pool, 3)
    # Commandment 10 (Aaron, 2026-08-17): the caveat states the
    # distinction — dice, not judgment — without naming an
    # implementation. "Python" rendering anywhere user-facing is the bug.
    assert "Python" not in out["caveat"]
    assert "dice" in out["caveat"]
    assert "yours to write" in out["caveat"]


def test_every_fate_lands_twice(pool):
    """The second landing (Aaron, 2026-08-17, expanded the same evening):
    every fate carries a face key now — the coin, the heart, the winning
    sword, the skull's give-or-take. The meaning names the face that
    landed, both faces of each fate are reachable, and the whole thing
    replays from one seed."""
    fields = {"cup": "coin", "heart": "heart_face",
              "sword": "sword_face", "skull": "skull_face"}
    words = {"heads": "heads", "tails": "tails",
             "whole": "whole", "broken": "broken",
             "edge": "edge", "hilt": "hilt",
             "buried": "grave takes", "risen": "grave gives"}
    seen: dict[str, set[str]] = {s: set() for s in wheel.SYMBOLS}
    for seed in range(160):
        out = _spin(pool, seed)
        field = fields[out["symbol"]]
        face = out[field]
        expected = set(wheel.FACES[out["symbol"]][1])
        assert face in expected
        assert words[face] in out["meaning"]
        seen[out["symbol"]].add(face)
        # No other fate's face key leaks onto this spin.
        for other, key in fields.items():
            if other != out["symbol"]:
                assert key not in out
    for symbol, faces in seen.items():
        assert faces == set(wheel.FACES[symbol][1]), symbol
    # And a face replays with its seed, card and all.
    once = _spin(pool, 11)
    again = _spin(pool, 11)
    assert once == again


def test_no_candidates_is_an_honest_reason_not_an_error(pool):
    """A colourless-only identity in the tiny pool leaves some fates with
    nothing to hand out; the wheel still names the fate and says so."""
    con = db.connect(pool)
    try:
        out = wheel.spin(mono_green_deck(), frozenset(), con, seed=1)
    finally:
        con.close()
    assert out["symbol"] in wheel.SYMBOLS
    if out["card"] is None:
        assert out["reason"]
