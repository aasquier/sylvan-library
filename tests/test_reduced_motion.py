"""Every animation the browser is handed, against the reduced-motion guards.

Commandment 6 wants a page that moves. `prefers-reduced-motion: reduce` is a
promise to the people for whom movement is nausea rather than delight, and the
two are only compatible while every animation is actually reachable by a
guard. On 2026-08-16 that was checked by hand -- 43 animation declarations
resolved against nine guard blocks -- and the ledger recorded the result as
complete. Three days later the stylesheet had 111 of them.

**A hand count is not a guard, and the hand count also looked in the wrong
place.** It swept `web/src/index.css`, and the two animations that were
genuinely unguarded were never in that file: `animate-spin` on `Spinner`
(`components/ui.tsx`) and `animate-pulse` on the lazy-chunk skeleton
(`lib/deferred.tsx`) are Tailwind utilities, so they exist only in the built
bundle. That is exactly the mistake `test_browser_floor.py` was written to
correct, one facet over and in the same afternoon: **what a phone parses is
the artifact, not the source.** So this file reads the artifact too.

## What "covered" means here, and what this file can and cannot see

A rule is covered when any class in its selector appears inside a
`prefers-reduced-motion: reduce` block. Most guards are that direct. Two
mechanisms are not, and both are real:

- **A base class on the same element.** `.lab-bubble-2` only sets a delay; the
  element is `class="lab-bubble lab-bubble-2"` and `.lab-bubble` is guarded.
- **An ancestor that is removed outright.** `.wheel-spark` is inside
  `.wheel-strike`, which the guard sets to `display: none` -- a sword-strike
  is ceremony, and the honest reduced version is that it does not happen.

Neither is visible in a stylesheet, so `COVERED_BY` records them. That table
is an assertion about the DOM which this file cannot check -- what it *can*
check, and does, is that the cover named is really guarded and that the thing
being excused really still animates. A typo, a deleted guard or a dead entry
all fail; only a wrong claim about containment survives, and that one is a
line of TSX away from the entry.

It is a tripwire, not a rendering engine. It proves a guard exists, not that
the guard wins the cascade -- source order decides that, and the one time it
mattered it was verified against the served sheet's rule indices in a real
browser. The class of drift it catches is the one that happened: an animation
added, or arriving through a dependency, with nobody to notice it never got a
guard.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
BUNDLE = ROOT / "src" / "mtglab" / "web_dist" / "assets" / "index.css"

#: An animating class -> (the class that actually arrests it, why).
#:
#: Every entry is a fact about the DOM rather than about the stylesheet: the
#: element carries both classes, or it sits inside the covering one. Each was
#: read out of the component that renders it, and the component is named so
#: the next person can re-read it rather than trust this line.
COVERED_BY: dict[str, tuple[str, str]] = {
    # Same element, base class plus a modifier -- components/theme.tsx,
    # routes/Research.tsx, components/forest.tsx.
    "flask-bubble-2": ("flask-bubble", "class='flask-bubble flask-bubble-2'"),
    "flask-bubble-3": ("flask-bubble", "class='flask-bubble flask-bubble-3'"),
    "lab-bubble-2": ("lab-bubble", "class='lab-bubble lab-bubble-2'"),
    "lab-bubble-3": ("lab-bubble", "class='lab-bubble lab-bubble-3'"),
    "lab-bubble-4": ("lab-bubble", "class='lab-bubble lab-bubble-4'"),
    "lab-steam-2": ("lab-steam", "class='lab-steam lab-steam-2'"),
    "lab-flame-2": ("lab-flame", "class='lab-flame lab-flame-2'"),
    "scene-lane-left": ("scene-lane", "class='scene-lane scene-lane-left'"),
    "scene-lane-right": ("scene-lane", "class='scene-lane scene-lane-right'"),
    "scene-sunbeam-b": ("scene-sunbeam",
                        "class='scene-sunbeam scene-sunbeam-b'"),
    # Inside an ancestor the guard sets to `display: none` -- the whole fate
    # effect is ceremony, and components/wheel.tsx nests each of these under
    # the element named here.
    "wheel-coin3d": ("wheel-coinspin", "inside the spinning coin"),
    "wheel-heart-bloom": ("wheel-heartfx", "inside the heart"),
    "wheel-ghost": ("wheel-ghosts", "inside the shades"),
    "wheel-offer-blade": ("wheel-offer", "inside the offering"),
    "wheel-offer-gleam": ("wheel-offer", "inside the offering"),
    "wheel-clash-blade": ("wheel-strike", "inside the strike"),
    "wheel-strike-flash": ("wheel-strike", "inside the strike"),
    "wheel-sparkburst": ("wheel-strike", "inside the strike"),
    "wheel-spark": ("wheel-strike", "inside the strike"),
    "wheel-blood": ("wheel-strike", "inside the strike"),
    # components/forest.tsx: the whole ambience layer is removed, because a
    # firefly holding perfectly still is not a firefly, it is a spot.
    "firefly": ("forest-ambience", "inside the ambience layer"),
    "leaf-fall": ("forest-ambience", "inside the ambience layer"),
    "page-fall": ("forest-ambience", "inside the ambience layer"),
}

#: `animation`, `animation-name`, `animation-duration`, `animation-delay`,
#: `animation-play-state` -- and deliberately not `animation-timing-function`,
#: which appears inside `@keyframes` steps and animates nothing by itself.
_DECLARES = re.compile(
    r"(?<![-\w])animation(?:-name|-duration|-delay|-play-state)?\s*:")
_ARRESTS = re.compile(r"(?<![-\w])animation\s*:\s*none\s*;?\s*$")
_RULE = re.compile(r"([^{}@;]+)\{([^{}]*)\}")
_CLASS = re.compile(r"\.([A-Za-z0-9_-]+)")


def _guard_spans(css: str) -> list[tuple[int, int]]:
    """Byte ranges of every `prefers-reduced-motion: reduce` at-rule.

    Brace-counted rather than regexed: the block holds nested rules, so the
    first `}` is not the end of it.
    """
    spans = []
    for at in re.finditer(r"@media[^{]*prefers-reduced-motion[^{]*\{", css):
        i, depth = at.end(), 1
        while i < len(css) and depth:
            depth += 1 if css[i] == "{" else -1 if css[i] == "}" else 0
            i += 1
        spans.append((at.start(), i))
    return spans


def _split(css: str) -> tuple[list[tuple[str, set[str]]], set[str]]:
    """(rules that animate, classes any guard mentions)."""
    spans = _guard_spans(css)
    guarded: set[str] = set()
    animating: list[tuple[str, set[str]]] = []
    for rule in _RULE.finditer(css):
        selector, body = rule.group(1).strip(), rule.group(2)
        inside = any(a <= rule.start(1) < b for a, b in spans)
        if inside:
            guarded |= set(_CLASS.findall(selector))
        elif _DECLARES.search(body) and not _ARRESTS.search(body.strip()):
            animating.append((selector, set(_CLASS.findall(selector))))
    return animating, guarded


@pytest.fixture(scope="module")
def sheet() -> tuple[list[tuple[str, set[str]]], set[str]]:
    if not BUNDLE.is_file():
        pytest.skip("no committed bundle in this checkout")
    return _split(BUNDLE.read_text(encoding="utf-8"))


def test_every_animation_in_the_bundle_can_be_arrested(
        sheet: tuple[list[tuple[str, set[str]]], set[str]]) -> None:
    """Nothing the browser downloads may move with no way to stop it."""
    animating, guarded = sheet
    covers = {k for k, (c, _) in COVERED_BY.items() if c in guarded}
    loose = sorted({sel for sel, classes in animating
                    if not (classes & (guarded | covers))})
    assert not loose, (
        "These rules animate and no `prefers-reduced-motion: reduce` block "
        "reaches them:\n  " + "\n  ".join(loose)
        + "\n\nAdd a guard in web/src/index.css and rebuild the bundle, or -- "
          "if the element already carries a guarded base class or sits inside "
          "a guarded ancestor -- record that in COVERED_BY with the component "
          "it was read from. Reduced, not necessarily removed: a status "
          "indicator that stops turning says the wrong thing."
    )


def test_every_cover_named_is_itself_guarded(
        sheet: tuple[list[tuple[str, set[str]]], set[str]]) -> None:
    """The excuse table cannot excuse anything with a guard that is gone."""
    _, guarded = sheet
    broken = sorted(f"{k} -> {cover} ({why})"
                    for k, (cover, why) in COVERED_BY.items()
                    if cover not in guarded)
    assert not broken, (
        "COVERED_BY names a cover that no reduced-motion block mentions, so "
        "the thing it excuses is not arrested by anything:\n  "
        + "\n  ".join(broken)
    )


def test_the_cover_table_holds_nothing_that_stopped_animating(
        sheet: tuple[list[tuple[str, set[str]]], set[str]]) -> None:
    """An excuse for something deleted is a claim nobody will re-check.

    Without this the table only ever grows, and a stale entry could later
    excuse a *different* rule that happens to reuse the class name.
    """
    animating, _ = sheet
    live = {c for _, classes in animating for c in classes}
    dead = sorted(k for k in COVERED_BY if k not in live)
    assert not dead, (
        f"COVERED_BY still excuses {dead}, which no longer animates in the "
        "bundle. Drop the entries."
    )
