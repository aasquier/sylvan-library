"""No technology backing this app may reach a user's eye.

Commandment 10 names what may never render -- "not languages, not databases
or frameworks, not seeds, not model ids, not wire tokens" -- and until now
nothing enforced it. It was sharpened *after* a seed rendered on the Wheel
(2026-08-17), that one was removed, and the identical thing was left standing
two screens away: the simulator's provenance badge read `seed 7` and its form
control was labelled `Seed`, with the glossary entry behind the tooltip
saying "seed" four more times. Blue's 2026-08-19 rainbow leg found them by
reading every technology name in `web/src` by hand and reported exactly two
renders. A hand sweep is a measurement, not a guard; this file is the guard.

**Scope is what renders, and only that.** `seed` is the wire field, the
`sim.seed` glossary key, the `simruns.DEFAULT_SEED` constant and half the
comments in `sim/` -- all correct, all invisible, none of this file's
business. ADR 18's reproducibility is built on that field keeping its name.
So the sweep looks at three places a person actually reads: JSX text nodes,
the four attributes that render as words, and the glossary's own prose.

**One word, deliberately.** The drift guard next door (`test_deck_location
_drift.py`) argues that a tripwire should ban the sentence that drifted and
not the topic, and the same restraint applies to a vocabulary: banning every
technology name outright would fail on `deck.yaml`, which Blue recorded as
considered-and-*kept* -- a user edits that file and needs its name. `seed` is
here because it drifted twice, and because it has no legitimate rendered use
in a game that has a word of its own for the same idea.
"""

from __future__ import annotations

import re
from pathlib import Path

from mtglab import glossary

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / "web" / "src"

#: The word, and what Magic calls it instead. Kept as a table so the second
#: entry costs a line rather than a rewrite -- but read the docstring before
#: adding one: a word earns a place here by having drifted, not by sounding
#: technical.
FORBIDDEN = ((re.compile(r"\bseeds?\b", re.I), "Magic shuffles; say shuffle"),)

#: Attributes whose value is read by a person rather than by a machine.
#: `seed=` is pointedly *not* here: `tarot.tsx` sets it on an SVG
#: `feTurbulence`, where it is a filter parameter that no eye ever meets, and
#: a sweep that flagged it would be teaching the wrong lesson to whoever
#: silenced it.
RENDERED_ATTRS = ("label", "title", "placeholder", "aria-label")


#: Anything with one of these in it is code, not copy. The first draft had no
#: such filter and read `useState<Mode>('mana')` as a tag called `Mode`
#: containing the words that followed it -- TypeScript generics and arrow
#: functions both put a `>` where JSX puts one, and neither is a screen.
#:
#: The honest cost: rendered prose containing a bracket or an equals sign is
#: skipped, so this sweep is a floor rather than a proof. That is the right
#: trade at one banned word -- a missed sentence is a word that keeps
#: rendering, while a false positive is a guard somebody deletes.
CODE_PUNCTUATION = re.compile(r"[=;()]")


def _rendered_fragments(source: str) -> list[tuple[str, int]]:
    """Every run of characters in a `.tsx` file that a person will read.

    Two shapes, because the failures were one of each: the badge was a JSX
    text node (`<Badge>seed {seed}</Badge>`) and the control was an attribute
    (`label="Seed"`). Comments are skipped outright rather than filtered
    afterwards -- `web/src` explains the seed field at length and correctly,
    and a guard that made those comments unwriteable would be removed within
    the week.
    """
    stripped = re.sub(r"//[^\n]*|/\*.*?\*/", lambda m: " " * len(m.group(0)),
                      source, flags=re.S)
    found = []
    attrs = "|".join(RENDERED_ATTRS)
    for match in re.finditer(rf'\b(?:{attrs})="([^"]*)"', stripped):
        found.append((match.group(1), match.start(1)))
    # A JSX text node: whatever follows an opening tag, up to the next one,
    # with `{...}` blanked out -- an interpolated value is named by the code
    # and read by nobody.
    for match in re.finditer(r"<[A-Za-z][\w.]*(?:\s[^<>]*)?>", stripped):
        text = stripped[match.end():].split("<", 1)[0]
        text = re.sub(r"\{[^{}]*\}", " ", text)
        if text.strip() and not CODE_PUNCTUATION.search(text):
            found.append((text, match.end()))
    return found


def sources() -> list[Path]:
    return sorted(p for p in WEB.rglob("*.tsx") if ".test." not in p.name)


def test_there_are_sources_to_sweep() -> None:
    """A tripwire over an empty set passes forever and measures nothing."""
    assert len(sources()) > 20


def test_no_screen_shows_a_technology_by_name() -> None:
    """One test over every file: the sweep wants to report a list, not a file.

    `test_deck_location_drift` makes the argument -- fourteen sites in eleven
    files is one mistake, and the person fixing it wants all of them.
    """
    offences = []
    for path in sources():
        source = path.read_text()
        for text, offset in _rendered_fragments(source):
            for pattern, instead in FORBIDDEN:
                if match := pattern.search(text):
                    line = source.count("\n", 0, offset) + 1
                    offences.append(
                        f"{path.relative_to(ROOT)}:{line}: "
                        f"{match.group(0)!r} renders. {instead}.\n"
                        f"    ...{text.strip()[:90]}...")
    assert not offences, "\n" + "\n".join(offences)


def test_no_glossary_entry_teaches_a_technology_word() -> None:
    """The tooltip is the one place a beginner goes to be *told* the word.

    Derived over every term rather than asserted of the one that was wrong,
    which is the whole difference between this and a test that restates the
    claim it is meant to check.
    """
    offences = []
    for term in glossary.TERMS:
        for field, value in (("term", term.term), ("short", term.short),
                             ("long", term.long)):
            for pattern, instead in FORBIDDEN:
                if match := pattern.search(value):
                    offences.append(f"{term.key}.{field}: "
                                    f"{match.group(0)!r} renders. {instead}.")
    assert not offences, "\n" + "\n".join(offences)
