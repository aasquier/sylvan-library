"""The browser the site still works in, pinned against what the bundle needs.

The floor has always been stated as **Safari 15 on macOS 12** -- this machine's
own browser, and the reason `web/src` is swept for regex lookbehind every
polish run. That statement stopped being true and nothing said so.

The sweep looked in the wrong place. Grepping `web/src` finds what *we* wrote,
and every feature that actually raised the floor arrived through a dependency:
Tailwind v4 emits 56 `@property` rules and ten `color-mix(in lab, ...)` values
(both Safari 16.4), and React ships `Object.hasOwn` and `structuredClone`
(both Safari 15.4). None of it appears in a single file we author, so a hand
grep of the source could never have caught it, and did not.

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

Whether 16.4 is the *right* floor is Aaron's call, not this file's. This
records where the code stands so the decision is made with a number in hand.
"""

from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "src" / "mtglab" / "web_dist" / "assets"

#: The oldest Safari the committed bundle is allowed to require.
#:
#: **Raised from 15.0 to 16.4 by Tailwind v4, not chosen.** Recorded here on
#: 2026-08-16 rather than decided: the dependency set already required it, and
#: a floor nobody has written down is a floor nobody can hold. Lowering it
#: means removing what `FLOOR_SETTING` names below.
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
