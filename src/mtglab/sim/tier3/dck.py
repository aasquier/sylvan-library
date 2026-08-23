"""deck.yaml -> Forge's `.dck` format.

The format, read off the 13,994 `.dck` files Forge ships rather than guessed:

    [metadata]
    name=Arahbo, Roar of the World - Cats
    [Commander]
    1 Arahbo, Roar of the World
    [Main]
    1 Sol Ring
    36 Forest
    [Sideboard]
    1 Kaheera, the Orphanguard

Sections are the ten in `forge.deck.DeckSection`; the four above are the ones a
Commander deck uses. A line is `<qty> <name>`, optionally `|SET|<number>` to
pin a printing. **We never pin one.** deck.yaml records no set, `mtglab price
deck` already owns the question of which printing to buy, and pinning would
turn a Forge-side edition rename into a mysteriously missing card.

The companion goes in `[Sideboard]` because Forge has no companion section --
checked against the enum in the shipped jar, not assumed. That is also where
the rules put it.

**A `.dck` is a temporary file, never an artifact.** CLAUDE.md rule 3 fixes the
deliverables at five, and this is not a sixth: it is an input to a simulator,
written into a scratch directory for the length of a run. Writing it next to
`deck.yaml` would also put Forge-shaped card data in the repository, which
rule 5 forbids.
"""

from __future__ import annotations

import re
from pathlib import Path

from mtglab.decks.model import Deck

# Forge parses section headers case-insensitively (`compareToIgnoreCase` in
# DeckSection), but its own files are written in this casing and matching them
# means a `.dck` we produce is diffable against one Forge produced.
SECTION_COMMANDER = "[Commander]"
SECTION_MAIN = "[Main]"
SECTION_SIDEBOARD = "[Sideboard]"


def _card_line(qty: int, name: str) -> str:
    return f"{qty} {name}"


def to_dck(deck: Deck, names: dict[str, str] | None = None) -> str:
    """Render a deck as `.dck` text.

    `names` maps a deck.yaml name to the name Forge knows it by, and is
    `CoverageReport.resolved` in practice -- so what gets written is exactly
    what the pre-flight verified Forge implements. Omitted, every name is used
    as-is, which is right for the six curated decks (none has a `//` name) and
    is what makes this function testable without a Forge install.

    A name missing from `names` is written unchanged rather than dropped.
    Dropping it here would reproduce, inside our own code, the exact silent
    failure the pre-flight exists to catch.
    """
    lookup = names or {}

    def forge_name(name: str) -> str:
        return lookup.get(name, name)

    lines = ["[metadata]", f"name={deck.name}"]

    if deck.commander:
        lines.append(SECTION_COMMANDER)
        lines.extend(_card_line(1, forge_name(c)) for c in deck.commander)

    lines.append(SECTION_MAIN)
    # deck.yaml order is by category, which is how a human reads the deck.
    # Sorted here because a `.dck` is machine input, and a stable order makes
    # two exports diffable.
    for entry in sorted(deck.cards, key=lambda c: c.name):
        lines.append(_card_line(entry.qty, forge_name(entry.name)))

    # Always emitted, even when empty -- every `.dck` Forge ships ends with it.
    lines.append(SECTION_SIDEBOARD)
    if deck.companion:
        lines.append(_card_line(1, forge_name(deck.companion)))

    return "\n".join(lines) + "\n"


#: `service._SLUG`, checked here as well as at the door.
#:
#: **A slug reaches this function by two roads and only one of them has a
#: gate.** `POST /api/decks` and `POST /api/decks/import` both refuse a slug
#: that is not this shape, because a slug becomes a directory name. But a deck
#: can also arrive as deck.yaml *text* -- over the private network, from the
#: app to the worker (ADR 35) -- and `Deck.from_text` reads whatever `slug:`
#: says. The worker is a separate machine precisely so that somebody else's
#: rules engine is contained; trusting the caller's text to name a file is the
#: wrong way round.
#:
#: It is not hypothetical in either direction. `<slug>.dck` is written into
#: Forge's profile directory, so `../../..` escapes it; and the filename goes
#: on Forge's own command line after `-d`, so a slug beginning with `-` is read
#: as a flag rather than as a deck. Both are shut by the same alphabet.
#:
#: Found by CodeQL on the Go port of this module (2026-08-23) and fixed in both
#: runtimes at once, the way `edit.set_shared` was: the shape is identical here
#: and the CLI still writes these files.
SLUG = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)*$")


def write_dck(deck: Deck, directory: Path | str,
              names: dict[str, str] | None = None) -> Path:
    """Write `<directory>/<slug>.dck` and return the path.

    The filename is the slug, not the deck name: `forge sim -d` takes deck
    names on a command line, and a slug needs no quoting.

    **Refused rather than sanitised.** A cleaned-up filename is a file nobody
    asked for, and the coverage report's `slug` would then name something other
    than what is on disk -- which is the class of silent disagreement this
    whole package exists to prevent.
    """
    if not SLUG.match(deck.slug):
        raise ValueError(
            f"{deck.slug!r} is not a usable slug -- lowercase letters, "
            f"digits and single hyphens, e.g. 'arahbo-cats'")
    directory = Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    path = directory / f"{deck.slug}.dck"
    path.write_text(to_dck(deck, names), encoding="utf-8")
    return path
