"""The PROVENANCE entry: established shape, verbatim prose, idempotent."""

from __future__ import annotations

import dataclasses
from pathlib import Path

from animist.provenance import render, update
from animist.recipe import load_recipe
from animist.sources import Confirmed, LicenceConfirmation, RemoteFile

RECIPE = """
animist: 1
why_committed: >
  This photograph is CC0, so committing the derived asset is clean.
sources:
  ivy:
    provider: openverse
    identifier: abc-123
    landing: https://example.org/ivy
    title: "Ivy cascading"
    found_via: "Openverse with a license=cc0 filter"
    licence: CC0-1.0
outputs:
  - file: ivy-canopy.webp
    from: ivy
    ops:
      - matte_green: {fade_below_frac: 0.42}
      - resize: {width: 2048}
    encode: {format: webp, quality: 82}
    expect: {width: 2048, metadata: none}
"""


def loaded(tmp_path: Path):
    path = tmp_path / "ambience.recipe.yaml"
    path.write_text(RECIPE, encoding="utf-8")
    recipe = load_recipe(path)
    confirmed = Confirmed(
        LicenceConfirmation("ivy", "cc0", "2026-08-16",
                            "https://api.openverse.org/v1/images/abc-123/"),
        (RemoteFile("leaves.jpg", "https://img.example/leaves.jpg", "cc0"),))
    return recipe, confirmed


def test_entry_has_the_established_shape(tmp_path: Path) -> None:
    recipe, confirmed = loaded(tmp_path)
    entry = render(recipe, {"ivy": confirmed}, {"ivy": ["ivy-canopy.webp"]})
    assert entry.startswith("## ivy-canopy.webp")
    assert ('- **Source**: "Ivy cascading", <https://example.org/ivy>, '
            "found via Openverse with a license=cc0 filter.") in entry
    # The licence line is the gate's proof, dated by the code that checked.
    assert ("- **Licence**: cc0. Confirmed through the Openverse record API "
            "at fetch time (2026-08-16).") in entry
    assert "`matte_green`: fade_below_frac=0.42." in entry
    assert "`resize`: width=2048." in entry
    assert "Encoded WEBP, quality 82." in entry
    # The prose is verbatim from the recipe -- never composed here.
    assert ("Why committed rather than hotlinked: This photograph is CC0, "
            "so committing the derived asset is clean.") in entry


def test_skipped_files_are_reported_in_the_entry(tmp_path: Path) -> None:
    recipe, confirmed = loaded(tmp_path)
    guarded = Confirmed(confirmed.confirmation, confirmed.files,
                        skipped=("Banner.jpg",))
    recipe.sources["ivy"] = dataclasses.replace(
        recipe.sources["ivy"], require_filename_prefix="IVY")
    entry = render(recipe, {"ivy": guarded}, {"ivy": ["ivy-canopy.webp"]})
    assert "Skipped by the filename guard" in entry
    assert "`Banner.jpg`" in entry


def test_update_creates_appends_and_replaces(tmp_path: Path) -> None:
    recipe, confirmed = loaded(tmp_path)
    entry = render(recipe, {"ivy": confirmed}, {"ivy": ["ivy-canopy.webp"]})

    # No PROVENANCE.md: one is created with the tarot-rule opening.
    path = update(recipe, entry)
    text = path.read_text(encoding="utf-8")
    assert "The rule here is the tarot deck's" in text
    assert "<!-- animist:begin ambience -->" in text

    # Hand-written prose outside the markers survives a re-render untouched.
    hand_written = "\n## a hand-written section\n\nThe 1971 trap, argued.\n"
    path.write_text(text + hand_written, encoding="utf-8")
    update(recipe, entry.replace("quality 82", "quality 82"))
    again = path.read_text(encoding="utf-8")
    assert "The 1971 trap, argued." in again
    assert again.count("<!-- animist:begin ambience -->") == 1

    # Replacement is idempotent: a second identical update changes nothing.
    update(recipe, entry)
    assert path.read_text(encoding="utf-8") == again
