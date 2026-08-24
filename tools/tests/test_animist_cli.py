"""`animist`, end to end through `main()`, with the network faked
at the two seams (`sources._urllib_get`, `fetch._download`)."""

from __future__ import annotations

import io
import json
from pathlib import Path

import numpy as np
import pytest
from PIL import Image

from animist import config
from animist.cli import main

RECIPE = """
animist: 1
why_committed: it is ours
sources:
  ivy:
    provider: openverse
    identifier: abc-123
    title: "Ivy cascading"
    licence: CC0-1.0
outputs:
  - file: canopy.webp
    from: ivy
    ops:
      - resize: {width: 32}
    encode: {format: webp, quality: 82}
    expect: {width: 32, budget_kb: 50, metadata: none}
"""


@pytest.fixture
def scene(tmp_path, monkeypatch):
    """A recipe on disk, the network faked, the cache pointed at scratch."""
    directory = tmp_path / "scene"
    directory.mkdir()
    recipe = directory / "scene.recipe.yaml"
    recipe.write_text(RECIPE, encoding="utf-8")

    def fake_get(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 200, json.dumps({"license": "cc0",
                                "url": "https://img.example/leaves.jpg"}).encode()

    def fake_download(url: str, target: Path) -> None:
        rng = np.random.default_rng(5)
        image = Image.fromarray(
            rng.integers(0, 255, (48, 64, 3), dtype=np.uint8), mode="RGB")
        buffer = io.BytesIO()
        image.save(buffer, format="JPEG")
        target.write_bytes(buffer.getvalue())

    monkeypatch.setattr("animist.sources._urllib_get", fake_get)
    monkeypatch.setattr("animist.fetch._download", fake_download)
    with config.use_paths(data_dir=tmp_path / "data"):
        yield recipe


def test_build_writes_and_says_so(scene, capsys):
    main(["build", str(scene)])
    out = capsys.readouterr().out
    assert "wrote" in out and "canopy.webp" in out and "KB" in out
    assert "provenance" in out
    assert (scene.parent / "canopy.webp").exists()
    assert (scene.parent / "PROVENANCE.md").exists()


def test_build_dry_run_writes_nothing(scene, capsys):
    main(["build", str(scene), "--dry-run"])
    out = capsys.readouterr().out
    assert "would write" in out and "nothing touched" in out
    assert not (scene.parent / "canopy.webp").exists()


def test_build_refused_licence_exits_with_the_gate_sentence(
        scene, monkeypatch, capsys):
    def by_get(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 200, json.dumps({"license": "by",
                                "url": "https://img.example/x.jpg"}).encode()

    monkeypatch.setattr("animist.sources._urllib_get", by_get)
    with pytest.raises(SystemExit) as excinfo:
        main(["build", str(scene)])
    assert "refused" in str(excinfo.value)
    assert "allowlist" in str(excinfo.value)
    assert not (scene.parent / "canopy.webp").exists()


def test_an_invalid_recipe_is_refused_by_name(tmp_path, capsys):
    bad = tmp_path / "bad.recipe.yaml"
    bad.write_text("animist: 1\nwhy_committed: x\n", encoding="utf-8")
    with pytest.raises(SystemExit) as excinfo:
        main(["build", str(bad)])
    assert "refused" in str(excinfo.value) and "sources" in str(excinfo.value)


def test_fetch_reports_the_gate_and_the_cache(scene, capsys):
    main(["fetch", str(scene)])
    out = capsys.readouterr().out
    assert "ivy: cc0" in out and "1 file(s) cached" in out


def test_licence_prints_the_proof_without_downloading(scene, capsys):
    main(["licence", str(scene)])
    out = capsys.readouterr().out
    assert "ivy: cc0 -- confirmed" in out
    # The proof line ends with the exact API URL the gate consulted.
    proof = next(line for line in out.splitlines() if "confirmed" in line)
    assert proof.endswith("via https://api.openverse.org/v1/images/abc-123/")
    # The gate alone moves no image bytes.
    data = scene.parent.parent / "data"
    assert not data.exists() or not list(data.rglob("*.jpg"))


def test_licence_refusal_exits_one_and_names_each_source(
        scene, monkeypatch, capsys):
    def by_get(url: str, headers: dict[str, str]) -> tuple[int, bytes]:
        return 200, json.dumps({"license": "by",
                                "url": "https://img.example/x.jpg"}).encode()

    monkeypatch.setattr("animist.sources._urllib_get", by_get)
    with pytest.raises(SystemExit) as excinfo:
        main(["licence", str(scene)])
    assert excinfo.value.code == 1
    assert "ivy: REFUSED" in capsys.readouterr().out


def test_verify_holds_then_names_the_breach(scene, capsys):
    main(["build", str(scene)])
    capsys.readouterr()
    main(["verify", str(scene)])
    assert "held" in capsys.readouterr().out

    (scene.parent / "canopy.webp").unlink()
    with pytest.raises(SystemExit) as excinfo:
        main(["verify", str(scene)])
    assert excinfo.value.code == 1
    assert "missing" in capsys.readouterr().out


def test_verify_with_nothing_to_check_refuses(monkeypatch):
    monkeypatch.setattr("animist.cli._animist_all_recipes", list)
    with pytest.raises(SystemExit) as excinfo:
        main(["verify"])
    assert "ADR 29" in str(excinfo.value)


def test_measure_prints_the_curve_and_the_knee(scene, capsys):
    main(["measure", str(scene), "--output", "canopy.webp",
          "--widths", "16:48:16"])
    out = capsys.readouterr().out
    assert "16px" in out and "48px" in out
    assert "the knee is at" in out


def test_measure_widths_are_validated(scene):
    with pytest.raises(SystemExit) as excinfo:
        main(["measure", str(scene), "--output", "canopy.webp",
              "--widths", "16,32"])
    assert "three widths" in str(excinfo.value)
    with pytest.raises(SystemExit) as excinfo:
        main(["measure", str(scene), "--output", "canopy.webp",
              "--widths", "banana"])
    assert "could not read" in str(excinfo.value)


def test_measure_needs_a_real_file_output(scene):
    with pytest.raises(SystemExit) as excinfo:
        main(["measure", str(scene), "--output", "nope.webp"])
    assert "no `file:` output" in str(excinfo.value)


def test_build_unknown_output_is_refused(scene):
    with pytest.raises(SystemExit) as excinfo:
        main(["build", str(scene), "--output", "nope.webp"])
    assert "no output named" in str(excinfo.value)
