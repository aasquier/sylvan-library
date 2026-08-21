"""The fixtures the Go module reads, written by Python -- and the proof they match.

The Go migration's YAML spike (docs/go-migration/PLAN.md section 6) is a
parser-equivalence question: `decks/edit.py` is text surgery and Go never
*serialises* a deck, but it must *parse* one -- to validate, and to check an
edit against a parse-mutate-dump oracle -- exactly as PyYAML does. So Python
writes a deck that exercises every YAML shape `Deck.dump` can produce (folded
long plain scalars at width 100, quoted scalars with `: ` and `#` and braces,
apostrophes, unicode, multi-line notes, nested lists of maps, booleans,
integers, nulls never -- dump omits absent fields) and, beside it, PyYAML's own
reading of that text as canonical JSON. The Go test parses the YAML with
goccy and must produce the same JSON.

`tests/test_go_fixtures.py` holds the committed pair equal to what this
module generates now, so neither side can drift quietly; regenerate with

    python tests/go_fixtures.py

from the repository root after changing `rich_deck()` or the dumper.

**Since Phase 3 this also writes the reference prose** -- the 32 colour
combinations, the glossary, the lore shelves, the tarot deck's facts and the
labelling vocabulary -- as the JSON the Go module embeds and serves
(`go/internal/reference/data/`, rendered by `mtglab.reference`). Same
command, same drift test, same rule: the committed bytes are what Python
renders now, or the suite says so and names this command.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

import yaml

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab import reference
from mtglab.decks.model import CardEntry, Deck

ROOT = Path(__file__).resolve().parents[1]
TESTDATA = ROOT / "go" / "internal" / "deckyaml" / "testdata"
YAML_PATH = TESTDATA / "rich-deck.yaml"
JSON_PATH = TESTDATA / "rich-deck.parsed.json"
#: Where the Go module embeds the reference prose from (`go:embed data/*`).
REFERENCE_DIR = ROOT / "go" / "internal" / "reference" / "data"


def rich_deck() -> Deck:
    """A deck built to trip a second parser, not to be played."""
    long_why = ("Two mana on turn one, and it always has been: the single most "
                "played card in the format, the first thing every primer tells a "
                "newcomer to find, and the card this line exists to make room for "
                "-- which is why it is first.")
    return Deck(
        slug="rich-fixture",
        name="Rich Fixture: Every Shape the Dumper Writes",
        status="built",
        stage="curated",
        shared=False,
        pilot="Mark's wife",
        commander=["Syr Gwyn, Hero of Ashvale"],
        commander_art="0aae2e33-0000-4000-8000-000000000000",
        companion="Kaheera, the Orphanguard",
        bracket=3,
        legacy_archetype="midrange",
        themes=["knights", "equipment", "voltron"],
        strategy=("Suit up a knight and swing: Syr Gwyn's trigger draws a card and "
                  "the equipment costs nothing, so every turn after the third is "
                  "a cantrip with a sword attached -- panache *and* martial prowess."),
        notes={
            "mulligan": "Keep any seven with two lands and an equipment; ship a hand\n"
                        "with no knight by turn three.",
            "politics": "The table forgets Ashvale until it is too late: say nothing.",
            "weird": "colon: and # hash, plus braces {G}{W} and a trailing space ",
        },
        cards=[
            CardEntry("Sol Ring", "ramp", long_why),
            CardEntry("Embercleave", "equipment",
                      "It's the card with the apostrophe -- and the one that ends "
                      "games: flash, trample, double strike.",
                      scryfall_id="c3a4b2d1-1111-4222-8333-444455556666",
                      tags=["finisher", "flash"]),
            CardEntry("Æther Vial", "utility", "Unicode in the name: Æ — é — ü.",
                      mana_cost="{1}", art="deadbeef-0000-4000-8000-000000000001"),
            CardEntry("Forest", "land", "* starts with a star, which must be quoted",
                      qty=12),
            CardEntry("Plains", "land", "#not a comment", qty=13),
            CardEntry("Lightning Greaves", "equipment", "yes"),
            CardEntry("Null Rod", "hate", "null"),
            CardEntry("Counterspell", "interaction", "12"),
            CardEntry("Swords to Plowshares", "interaction", "1.5 mana, in effect"),
            CardEntry("Knight of the White Orchid", "ramp",
                      "cost {1}{W}{W} -- braces again, inside a longer sentence that "
                      "also runs past the hundred-character fold so both rules fire "
                      "at once.",
                      mana_cost="{1}{W}{W}"),
        ],
        swap_board=[CardEntry("Sword of Feast and Famine", "equipment",
                              "Waiting on a slot.")],
        graveyard=[CardEntry("Gilded Lotus", "ramp", "Entombed: too slow for a "
                             "deck that wants its mana on turn two.")],
    )


def render() -> tuple[str, str]:
    """The YAML text and PyYAML's reading of it as canonical JSON."""
    text = rich_deck().dump()
    parsed = yaml.safe_load(text)
    return text, json.dumps(parsed, sort_keys=True, indent=2,
                            ensure_ascii=False) + "\n"


def write() -> None:
    TESTDATA.mkdir(parents=True, exist_ok=True)
    text, parsed = render()
    YAML_PATH.write_text(text, encoding="utf-8")
    JSON_PATH.write_text(parsed, encoding="utf-8")
    print(f"wrote {YAML_PATH}\nwrote {JSON_PATH}")
    REFERENCE_DIR.mkdir(parents=True, exist_ok=True)
    for name, body in reference.render().items():
        (REFERENCE_DIR / name).write_text(body, encoding="utf-8")
        print(f"wrote {REFERENCE_DIR / name}")


if __name__ == "__main__":
    write()
