"""ADR 32: the runtime tier, driven end to end through fakes.

No test here imports torch (the suite must run identically with and without
the `depth` extra), and none touches the network: the depth model is a
Protocol fake, the art download is a seam that writes bytes it already has,
and card facts come from `tiny_pool`. The HTTP serving tier these
derivatives feed stayed with the app (it answers from the Go door now), so
its route tests stayed there too; what this file holds is the derivation.
"""

from __future__ import annotations

import io
from pathlib import Path

import numpy as np
import pytest
from PIL import Image

import tiny_pool
from animist import config
from cardmotion import cache
from cardmotion.build import (
    BuildRefused,
    build_derivative,
    resolve_subject,
)
from cardmotion.effects import EFFECTS, Effect, derive


class FakeDepthModel:
    """Nearer at the bottom, farther at the top -- a landscape's shape."""

    def infer(self, image: Image.Image) -> Image.Image:
        width, height = image.size
        rows = np.linspace(0, 255, height, dtype=np.uint8)
        return Image.fromarray(np.tile(rows[:, None], (1, width)), mode="L")


def art_image(width: int = 64, height: int = 48) -> Image.Image:
    rng = np.random.default_rng(9)
    return Image.fromarray(
        rng.integers(0, 255, (height, width, 3), dtype=np.uint8), mode="RGB")


def fake_download(url: str, target: Path) -> None:
    buffer = io.BytesIO()
    art_image().save(buffer, format="JPEG")
    target.write_bytes(buffer.getvalue())


@pytest.fixture
def pool(tmp_path):
    # Read-write, through the fixture's own connector: several tests insert
    # printings mid-flight, which the toolbox's read-only `pool.connect`
    # exists to refuse. That refusal has its own tests in test_pool.py.
    with config.use_paths(data_dir=tmp_path / "data"):
        con = tiny_pool.connect(tiny_pool.build(config.DB_PATH))
        try:
            yield con
        finally:
            con.close()


# ------------------------------------------------------------ the effects

def test_the_fingerprint_moves_with_the_knobs() -> None:
    base = EFFECTS["depth-drift"]
    tweaked = Effect(key=base.key, needs_depth=True, fps=base.fps,
                     seconds=base.seconds, params={"amplitude": 9.0,
                                                   "zoom": 1.06})
    assert base.fingerprint() != tweaked.fingerprint()
    assert base.fingerprint() == EFFECTS["depth-drift"].fingerprint()


def test_depth_drift_derives_a_loop() -> None:
    art = art_image()
    depth = FakeDepthModel().infer(art)
    clip = derive(art, depth, EFFECTS["depth-drift"])
    assert len(clip.frames) == EFFECTS["depth-drift"].frames
    assert clip.fps == 24
    assert clip.size == art.size
    # The parallax must actually move something between frames.
    first = np.asarray(clip.frames[0]).astype(float)
    quarter = np.asarray(clip.frames[len(clip.frames) // 4]).astype(float)
    assert np.abs(first - quarter).mean() > 0.5


def test_depth_drift_loops_closed() -> None:
    """The camera path is a closed ellipse, so the wrap step must look like
    an interior step -- the same assertion the animist's noise makes."""
    art = art_image()
    clip = derive(art, FakeDepthModel().infer(art), EFFECTS["depth-drift"])
    frames = [np.asarray(f).astype(float) for f in clip.frames]
    step = np.abs(frames[1] - frames[0]).mean()
    wrap = np.abs(frames[0] - frames[-1]).mean()
    assert wrap <= max(step, 0.5) * 1.5


def test_slow_pan_needs_no_depth() -> None:
    clip = derive(art_image(), None, EFFECTS["slow-pan"])
    assert len(clip.frames) == EFFECTS["slow-pan"].frames


def test_breath_needs_no_depth() -> None:
    clip = derive(art_image(), None, EFFECTS["breath"])
    assert len(clip.frames) == EFFECTS["breath"].frames


def test_depth_drift_without_a_map_is_refused() -> None:
    from cardmotion.effects import EffectError

    with pytest.raises(EffectError, match="depth"):
        derive(art_image(), None, EFFECTS["depth-drift"])


# ------------------------------------------------------------- the cache

def test_the_key_moves_with_every_input(tmp_path) -> None:
    effect = EFFECTS["slow-pan"]
    a = cache.cache_key("oracle-1", "https://x/art.jpg", effect)
    assert a == cache.cache_key("oracle-1", "https://x/art.jpg", effect)
    assert a != cache.cache_key("oracle-2", "https://x/art.jpg", effect)
    assert a != cache.cache_key("oracle-1", "https://x/other.jpg", effect)


def test_ready_means_attribution_written_last(tmp_path) -> None:
    with config.use_paths(data_dir=tmp_path / "data"):
        entry = cache.locate("oracle-1", "https://x/a.jpg",
                             EFFECTS["slow-pan"])
        entry.directory.mkdir(parents=True)
        entry.file(cache.LOOP_WEBM).write_bytes(b"partial")
        assert not entry.ready, "a half-built entry must read as absent"
        cache.write_attribution(entry, oracle_id="oracle-1", card_name="X",
                                artist="A. Painter", art_url="https://x/a.jpg",
                                scryfall_uri="https://scryfall.com/x",
                                effect=EFFECTS["slow-pan"])
        assert entry.ready
        assert cache.find_ready("oracle-1", EFFECTS["slow-pan"]) is not None
        assert cache.find_ready("oracle-9", EFFECTS["slow-pan"]) is None


def test_find_ready_matches_the_painting_being_shown(tmp_path) -> None:
    """The Gyome/Trostani bug: a deck that swapped printings was served the
    old painting's loop, because the lookup ignored the art entirely."""
    with config.use_paths(data_dir=tmp_path / "data"):
        effect = EFFECTS["slow-pan"]
        entry = cache.locate("oracle-1", "https://x/a.jpg?123", effect)
        entry.directory.mkdir(parents=True)
        cache.write_attribution(entry, oracle_id="oracle-1", card_name="X",
                                artist="A. Painter",
                                art_url="https://x/a.jpg?123",
                                scryfall_uri="https://scryfall.com/x",
                                effect=effect)
        # The painting the page shows, query string and all, matches on stem
        # -- a bulk refresh rewriting the cache-buster must not orphan it.
        assert cache.find_ready("oracle-1", effect,
                                "https://x/a.jpg?456") is not None
        # A different painting is a miss, never the nearest neighbour.
        assert cache.find_ready("oracle-1", effect,
                                "https://x/other.jpg") is None
        # No art means the older any-art answer.
        assert cache.find_ready("oracle-1", effect) is not None


# ------------------------------------------------------------- the build

def test_build_writes_the_derivative_from_pool_facts(pool) -> None:
    entry = build_derivative(pool, card="Gyome, Master Chef",
                             effect_key="slow-pan", download=fake_download)
    assert entry.ready
    meta = entry.attribution()
    assert meta["card_name"] == "Gyome, Master Chef"
    assert meta["artist"], "attribution must name the artist"
    assert meta["oracle_id"]
    assert "Fan Content" in meta["notice"]
    for name in (cache.LOOP_WEBM, cache.LOOP_MP4, cache.POSTER_WEBP):
        assert entry.file(name).stat().st_size > 0
    assert not entry.file(cache.DEPTH_PNG).exists(), \
        "slow-pan never inferred a depth map"


def test_build_with_the_fake_model_ships_the_depth_map(pool) -> None:
    entry = build_derivative(pool, card="Gyome, Master Chef",
                             effect_key="depth-drift",
                             model=FakeDepthModel(), download=fake_download)
    assert entry.ready
    assert entry.file(cache.DEPTH_PNG).stat().st_size > 0


def test_a_depth_effect_without_a_model_is_refused(pool) -> None:
    with pytest.raises(BuildRefused, match="depth"):
        build_derivative(pool, card="Gyome, Master Chef",
                         effect_key="depth-drift", download=fake_download)


def test_an_unknown_card_is_refused_by_rule_one(pool) -> None:
    with pytest.raises(BuildRefused, match="pool"):
        build_derivative(pool, card="No Such Card", effect_key="slow-pan",
                         download=fake_download)


def test_an_unknown_effect_is_refused_with_the_roster(pool) -> None:
    with pytest.raises(BuildRefused, match="slow-pan"):
        build_derivative(pool, card="Gyome, Master Chef",
                         effect_key="wobble", download=fake_download)


def test_resolve_subject_reads_the_pool_row(pool) -> None:
    subject = resolve_subject(pool, "gyome, master chef")
    assert subject.card_name == "Gyome, Master Chef"
    assert subject.art_url


def _insert_printing(pool, printing_id: str, normal_url: str,
                     artist: str | None = None) -> None:
    pool.execute(
        "INSERT INTO printings (id, name, image_normal, artist) "
        "VALUES (?, ?, ?, ?)",
        [printing_id, "Gyome, Master Chef", normal_url, artist])


def test_a_printing_the_pool_can_credit_needs_no_network(pool) -> None:
    """`printings.artist` exists since 2026-08-19, so the common case is a
    local read. The fetch is a function that raises: if the credit came back
    right, nothing asked Scryfall."""
    def never(pid: str):
        raise AssertionError("the pool already knows this painter")

    _insert_printing(pool, "print-1",
                     "https://cards.scryfall.io/normal/front/9/9/p1.jpg",
                     artist="Pool Painter")
    subject = resolve_subject(pool, "Gyome, Master Chef", art_id="print-1",
                              fetch_card_json=never)
    assert subject.artist == "Pool Painter"


def test_resolve_subject_honours_the_chosen_printing(pool) -> None:
    """A deck's `commander_art` names the painting to derive; a printing the
    pool cannot credit falls back to Scryfall at build time rather than
    crediting the default printing's painter."""
    _insert_printing(pool, "print-2",
                     "https://cards.scryfall.io/normal/front/9/9/p2.jpg")
    subject = resolve_subject(
        pool, "Gyome, Master Chef", art_id="print-2",
        fetch_card_json=lambda pid: {"artist": "Alt Painter",
                                     "scryfall_uri": "https://scryfall.com/p2"})
    assert subject.art_url == \
        "https://cards.scryfall.io/art_crop/front/9/9/p2.jpg"
    assert subject.artist == "Alt Painter"
    assert subject.scryfall_uri == "https://scryfall.com/p2"


def test_a_chosen_printing_with_no_reachable_artist_degrades(pool) -> None:
    def unreachable(pid: str):
        raise OSError("offline")

    _insert_printing(pool, "print-3",
                     "https://cards.scryfall.io/normal/front/9/9/p3.jpg")
    subject = resolve_subject(pool, "Gyome, Master Chef", art_id="print-3",
                              fetch_card_json=unreachable)
    assert subject.artist == "(artist unrecorded)", \
        "an unknown painter must not be credited as the default printing's"


def test_a_chosen_printing_the_pool_lacks_is_refused(pool) -> None:
    with pytest.raises(BuildRefused, match="chosen printing"):
        resolve_subject(pool, "Gyome, Master Chef", art_id="print-gone")


def test_two_printings_are_two_derivatives(pool) -> None:
    """The originals cache and the derivative key must both carry the art's
    identity, or the second build silently reuses the first painting."""
    _insert_printing(pool, "print-2",
                     "https://cards.scryfall.io/normal/front/9/9/p2.jpg")
    first = build_derivative(pool, card="Gyome, Master Chef",
                             effect_key="slow-pan", download=fake_download)
    second = build_derivative(
        pool, card="Gyome, Master Chef", effect_key="slow-pan",
        download=fake_download, art_id="print-2",
        fetch_card_json=lambda pid: {"artist": "Alt Painter"})
    assert first.directory != second.directory
    assert second.attribution()["artist"] == "Alt Painter"


# -------------------------------------------------------------- the sync

class _DeckStub:
    def __init__(self, slug, commander, commander_art=""):
        self.slug = slug
        self.commander = commander
        self.commander_art = commander_art


def test_sync_builds_the_missing_and_skips_the_present(pool) -> None:
    from cardmotion.build import sync

    decks = [
        _DeckStub("gyome-food", ["Gyome, Master Chef"]),
        _DeckStub("no-commander", []),
    ]
    report = sync(pool, decks, effect_key="slow-pan",
                  download=fake_download)
    assert report.built == ["gyome-food"]
    assert report.skipped == ["no-commander"]
    assert report.refused == []

    # A second sweep finds the derivative and builds nothing.
    again = sync(pool, decks, effect_key="slow-pan", download=fake_download)
    assert again.built == []
    assert again.present == ["gyome-food"]


def test_sync_rebuilds_after_an_art_swap(pool) -> None:
    """Aaron's resync case: the deck picks a new printing, the old loop no
    longer matches what the page shows, and the sweep builds the new one."""
    from cardmotion.build import sync

    deck = _DeckStub("gyome-food", ["Gyome, Master Chef"])
    sync(pool, [deck], effect_key="slow-pan", download=fake_download)

    _insert_printing(pool, "print-2",
                     "https://cards.scryfall.io/normal/front/9/9/p2.jpg")
    deck.commander_art = "print-2"
    report = sync(pool, [deck], effect_key="slow-pan",
                  download=fake_download,
                  fetch_card_json=lambda pid: {"artist": "Alt Painter"})
    assert report.built == ["gyome-food"], \
        "a swapped painting is a missing derivative, not a present one"


def test_sync_loads_the_model_lazily_and_at_most_once(pool) -> None:
    from cardmotion.build import sync

    loads = []

    def loader():
        loads.append(1)
        return FakeDepthModel()

    decks = [_DeckStub("gyome-food", ["Gyome, Master Chef"])]
    sync(pool, decks, effect_key="depth-drift", download=fake_download,
         model_loader=loader)
    assert loads == [1]
    # Fully synced: the model is never loaded at all.
    sync(pool, decks, effect_key="depth-drift", download=fake_download,
         model_loader=loader)
    assert loads == [1]
