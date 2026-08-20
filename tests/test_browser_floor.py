"""The browser the site still works in, pinned against what the bundle needs.

**The floor is Safari 16.4, declared by Aaron on 2026-08-19.** Before that it
was stated as Safari 15 on macOS 12 -- this machine's own browser, and the
reason `web/src` is swept for regex lookbehind every polish run. That statement
stopped being true and nothing said so.

Two independent things now hold it there, which is what settled the decision:
either alone would have to be argued away, and neither is a line anybody wrote
on purpose.

1. **The look of the site.** Tailwind v4 emits `@property` and
   `color-mix(in lab, ...)`, both Safari 16.4, into the stylesheet -- 53 and 17
   of them respectively in the bundle as committed. Below the floor this is
   quiet rather than fatal (see below), which is why it went unnoticed for a
   release.
2. **The camera door.** `web/src/lib/reader.ts` pins `corePath` to
   `tesseract-core-simd-lstm.wasm.js` and `ocr.py` serves exactly that file
   first-party with no non-SIMD sibling in `ASSETS`; **WebAssembly SIMD is
   Safari 16.4**. Below the floor the reader does not degrade, it fails to
   start. `test_the_camera_door_still_holds_the_floor_independently` is that
   route made checkable, because it is the one a refactor could remove without
   touching a line of CSS -- and the day it goes, the floor rests on Tailwind
   alone and the decision is worth re-asking.

Recording it rather than re-deriving it is the point: three separate polish
runs measured the same two facts and left the number an open question.

The sweep that missed it looked in the wrong place. Grepping `web/src` finds
what *we* wrote, and every feature that actually raised the floor arrived
through a dependency: Tailwind's two above, and React's `Object.hasOwn` and
`structuredClone` (both Safari 15.4). None of it appears in a single file we
author, so a hand grep of the source could never have caught it, and did not.
The counts above are re-measured off the committed bundle each time they are
written down, because they move with every Tailwind upgrade and a remembered
count is how the last one went stale.

What that costs below the floor is not a white page -- it is quieter, which is
why it went unnoticed. `@property` registers the `--tw-*` custom properties
that Tailwind composes shadows, transforms, filters and rings out of; where
the at-rule is ignored those variables are unregistered, `var()` yields the
guaranteed-invalid value, and the whole `box-shadow` / `transform` /
`backdrop-filter` declaration drops. The page renders, laid out correctly,
with its depth quietly gone.

So this file does two things a checklist cannot. It scans the artifact users
actually download -- `web_dist/assets`, not `web/src` -- and it makes the floor
a **declared value** rather than a remembered one, so the next dependency that
raises it fails a test instead of a friend's phone.

It is a tripwire, not a compiler. Matching `structuredClone` in a minified
bundle proves the string is there, not that the call runs; the class of drift
it catches is the one that happened -- a floor raised by an upgrade nobody
priced -- and for that a string is enough.
"""

from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "src" / "mtglab" / "web_dist" / "assets"
READER = ROOT / "web" / "src" / "lib" / "reader.ts"

#: The oldest Safari the site supports.
#:
#: **Raised from 15.0 to 16.4 by Tailwind v4 and recorded on 2026-08-16;
#: declared by Aaron on 2026-08-19**, once the camera door turned out to reach
#: the same number by a route of its own. Lowering it means removing what
#: `FLOOR_SETTING` names below *and* answering the reader's SIMD core -- the
#: module docstring has both.
FLOOR = (16, 4)

#: Substring -> (Safari version that shipped it, what it is).
#:
#: Only markers unambiguous enough in minified output to be worth asserting on.
#: A miss here is cheap (the tripwire stays silent); a false positive would
#: cost somebody an afternoon, so borderline strings are left out on purpose.
FEATURES: dict[str, tuple[tuple[int, int], str]] = {
    # CSS
    "@property": ((16, 4), "registered custom properties (Tailwind v4)"),
    "color-mix(in lab": ((16, 4), "lab colour interpolation (Tailwind v4)"),
    "color-mix(in oklab": ((16, 2), "oklab colour interpolation (Tailwind v4)"),
    "color-mix(in srgb": ((16, 2), "colour mixing, hand-written in index.css"),
    "container-type": ((16, 0), "container queries"),
    "@layer": ((15, 4), "cascade layers (Tailwind v4)"),
    ":focus-visible": ((15, 4), "focus ring only for keyboard users"),
    "oklch(": ((15, 4), "oklch colours"),
    "dvh": ((15, 4), "dynamic viewport units"),
    "svh": ((15, 4), "small viewport units"),
    ":has(": ((15, 4), "the parent selector"),
    "text-wrap:": ((17, 4), "text-wrap: balance / pretty"),
    "@container": ((16, 0), "container query at-rule"),
    "field-sizing": ((18, 0), "auto-sizing form controls"),
    "anchor-name": ((18, 0), "CSS anchor positioning"),
    # JS
    "Object.hasOwn": ((15, 4), "Object.hasOwn (React)"),
    "structuredClone": ((15, 4), "structuredClone (React)"),
    "reportError": ((15, 4), "reportError (React)"),
    ".toSorted(": ((16, 4), "non-mutating array sort"),
    ".toReversed(": ((16, 4), "non-mutating array reverse"),
    ".toSpliced(": ((16, 4), "non-mutating array splice"),
    "Object.groupBy": ((17, 4), "Object.groupBy"),
    "Map.groupBy": ((17, 4), "Map.groupBy"),
    "Array.fromAsync": ((18, 0), "Array.fromAsync"),
    "Promise.withResolvers": ((17, 4), "Promise.withResolvers"),
}

#: What holds the floor where it is. If one of these ever stops appearing the
#: floor may be able to come down -- which is a decision to take deliberately,
#: so the test says so rather than letting it drift the other way unnoticed.
FLOOR_SETTING = ("@property", "color-mix(in lab")

#: Lookbehind, and only lookbehind. The standing rule greps `(?<`, which also
#: matches a *named capture group* -- `(?<name>...)` has worked since Safari
#: 11.3 and is not the hazard. These two are.
LOOKBEHIND = ("(?<=", "(?<!")


def bundle() -> str:
    """Every byte of the committed frontend, as one string."""
    parts = [p.read_text(encoding="utf-8", errors="replace")
             for p in sorted(ASSETS.iterdir())
             if p.suffix in {".js", ".css"}]
    return "\n".join(parts)


@pytest.fixture(scope="module")
def blob() -> str:
    if not ASSETS.is_dir():
        pytest.skip("no committed bundle in this checkout")
    return bundle()


def test_the_bundle_stays_within_the_declared_floor(blob: str) -> None:
    """Nothing users download may need a browser newer than `FLOOR`."""
    over = []
    for marker, (needs, what) in FEATURES.items():
        if needs > FLOOR and marker in blob:
            over.append(f"  {marker!r} needs Safari {needs[0]}.{needs[1]} - {what}")
    assert not over, (
        "The committed bundle needs a newer browser than the declared floor "
        f"of Safari {FLOOR[0]}.{FLOOR[1]}:\n" + "\n".join(sorted(over))
        + "\n\nThis is the drift this file exists to catch. Either stop using "
          "the feature, or raise FLOOR deliberately and say so in the ledger -- "
          "raising it is a decision about whose phone still works, not a "
          "formality."
    )


def test_no_regex_lookbehind_reaches_the_browser(blob: str) -> None:
    """Lookbehind is the one JS feature this project has always refused.

    Checked against the bundle rather than `web/src`, because a dependency can
    ship one just as easily as we can and only one of those two places is what
    a phone actually parses.
    """
    found = [pat for pat in LOOKBEHIND if pat in blob]
    assert not found, (
        f"Regex lookbehind {found} reached the committed bundle. Safari did "
        "not support it until 16.4, and a lookbehind in a served regex is a "
        "SyntaxError at parse time -- the whole module fails to load, which "
        "is the white-page failure rather than a degraded one."
    )


def test_the_floor_setting_features_are_still_what_holds_it(blob: str) -> None:
    """Keeps the floor a setting rather than a ratchet.

    `FLOOR` is 16.4 because of these. If a dependency upgrade stops emitting
    them the floor could come *down*, and nobody would ever look -- a floor
    only ever gets checked when it fails upward.
    """
    absent = [m for m in FLOOR_SETTING if m not in blob]
    assert not absent, (
        f"{absent} no longer appears in the bundle. These are what put the "
        f"floor at Safari {FLOOR[0]}.{FLOOR[1]}; without them it may be able "
        "to come down. Re-run the scan, lower FLOOR if it can go, and record "
        "the new number in docs/polish/LEDGER.md."
    )


def test_the_camera_door_still_holds_the_floor_independently() -> None:
    """The second route to 16.4, and the one no CSS scan can see.

    The reader's core is fetched at run time and served from `data/cache`, so
    git holds none of it and `bundle()` above cannot reach it. What is in the
    repository is the *name* the client asks for and the table the server will
    answer with -- and both say SIMD, which is Safari 16.4 and is a hard
    failure below it rather than a quiet one.

    Read off `ocr.ASSETS` rather than restated, the same way the route test
    takes the filename off that table: a swap to the plain core would be a
    one-word edit in two files, and it would silently make Tailwind the only
    thing holding the floor.
    """
    from mtglab import ocr

    simd = [name for name in ocr.ASSETS if "core" in name]
    assert simd == ["tesseract-core-simd-lstm.wasm.js"], (
        f"the reading engine's core is now {simd}. If it is no longer a SIMD "
        "build, the camera door no longer requires Safari 16.4 and the floor "
        "rests on Tailwind alone -- which is a decision to re-take, not a "
        "line to update."
    )
    asked = READER.read_text(encoding="utf-8")
    assert f"/api/ocr/{simd[0]}" in asked, (
        "the client asks for a core the server does not serve; one of "
        "`lib/reader.ts` and `ocr.ASSETS` moved without the other."
    )
