"""The deck file is the source of truth. Everything else is derived.

A deck lives at `decks/<slug>/deck.yaml` and is tracked in git. The five
deliverables (quick primer, advanced primer, annotated 99, Moxfield TXT,
swap list) are *generated* from it, never hand-maintained. Deck history is
git history: `git log -p decks/gyome-food/deck.yaml` is the swap record.

Design note: every card carries its own `why`. That field is the reason this
project exists -- it is the thing that decays when decklists live in a web app.
Validation refuses to emit artifacts when a card is missing one.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

#: The C loader when libyaml is compiled in, the pure-Python one otherwise.
#: `yaml.safe_load` always takes the Python path even with libyaml bound, and
#: the difference is not small: 36ms against 7ms for one curated deck file,
#: measured 2026-08-14 — and the shelf parses six per request. Both loaders
#: accept only the same safe subset, so nothing but the speed changes.
SAFE_LOADER: type[yaml.SafeLoader] = getattr(yaml, "CSafeLoader", yaml.SafeLoader)


def load_yaml(text: str) -> Any:
    """`yaml.safe_load`, taking the libyaml fast path when it exists.

    The one YAML entry point for deck text — `edit.py` goes through it too, so
    a parser choice is made once rather than re-decided per call site.
    """
    return yaml.load(text, SAFE_LOADER)


# Macro categories tracked for balance analysis. The set is deliberately small
# and fixed so that category counts are comparable across decks.
CATEGORIES = (
    "land",
    "ramp",
    "card-advantage",
    "tutor",
    "interaction",
    "protection",
    "threat",
    "engine",
    "sac-outlet",
    "payoff",
    "recursion",
    "win-con",
    "utility",
)


class _Dumper(yaml.SafeDumper):
    """pyyaml with sequences indented under their key.

    Its default puts `- name: Sol Ring` hard against the left margin, which is
    legal YAML and unlike every hand-written deck in the repository. Since a
    generated deck file is then hand-edited and diffed in git, matching the
    house style is worth five lines.
    """

    def increase_indent(self, flow: bool = False, indentless: bool = False) -> None:
        return super().increase_indent(flow, False)


@dataclass
class CardEntry:
    """One card in the 99 (or the command zone, or the swap board)."""

    name: str
    category: str
    why: str = ""
    qty: int = 1
    scryfall_id: str | None = None
    # Optional overrides, used when a card is too new for the local DB.
    mana_cost: str | None = None
    tags: list[str] = field(default_factory=list)

    @classmethod
    def from_obj(cls, obj: Any) -> CardEntry:
        if isinstance(obj, str):
            return cls(name=obj, category="utility")
        return cls(
            name=obj["name"],
            category=obj.get("category", "utility"),
            why=obj.get("why", ""),
            qty=int(obj.get("qty", 1)),
            scryfall_id=obj.get("scryfall_id"),
            mana_cost=obj.get("mana_cost"),
            tags=list(obj.get("tags", [])),
        )

    def to_obj(self) -> dict[str, Any]:
        out: dict[str, Any] = {"name": self.name, "category": self.category}
        if self.why:
            out["why"] = self.why
        if self.qty != 1:
            out["qty"] = self.qty
        if self.scryfall_id:
            out["scryfall_id"] = self.scryfall_id
        if self.mana_cost:
            out["mana_cost"] = self.mana_cost
        if self.tags:
            out["tags"] = self.tags
        return out


# Whether the cards are physically in a box, or the deck is still a plan.
# Defaults to `theoretical` when absent, so nothing is ever silently claimed as
# owned -- a wrong "built" sends someone to a shelf that has no deck on it.
DECK_STATUSES = ("built", "theoretical")

# Whether the deck has been reasoned about, or is a list somebody pasted in.
# Orthogonal to `status`: all four combinations are real, and ADR 13 has the
# 2x2. A draft is honestly incomplete -- the gate reports its missing `why`
# fields as warnings and counts them, rather than burying the deck in errors it
# was always going to have.
#
# Defaults to `curated` when absent, which is the OPPOSITE default from
# `status`, and for the same reason. The six existing decks justify every card,
# and a default of `draft` would silently demote them. New decks declare
# themselves drafts explicitly.
DECK_STAGES = ("draft", "curated")


@dataclass
class Deck:
    slug: str
    name: str
    status: str = "theoretical"
    stage: str = "curated"
    # Whether anybody with a session may read this deck, or only its owner
    # (ADR 22). A third orthogonal axis: `status` is about cardboard, `stage`
    # is about thinking, and this is about who may look.
    #
    # Defaults to True when absent -- the same argument `stage` makes, pointed
    # the other way. The curated six are the showcase and predate the field, so
    # absent must never silently hide them. A deck created in the SQL tier is
    # written with `shared: false` explicitly, because `decks import` produces
    # a draft with an empty `why` on all 99 cards and publishing that the
    # instant it exists is nobody's intent.
    #
    # Sharing governs *reading*. Writing is the owner's alone either way, which
    # is `Library`'s rule and not this flag's.
    shared: bool = True
    commander: list[str] = field(default_factory=list)
    # Which printing's art the deck shows for its commander: a Scryfall
    # printing id, or empty for "whatever the pool considers the default".
    #
    # A deck property rather than a per-viewer setting, deliberately.
    # `deck.yaml` is the source of truth (ADR 1) and the choice should travel
    # with the deck through git the way every other decision about it does --
    # a Secret Lair Goreclaw is a fact about that deck, not a preference of
    # whoever is looking at it.
    commander_art: str = ""
    companion: str | None = None
    bracket: int | None = None
    strategy: str = ""
    # Free-form notes that flow into the advanced primer.
    notes: dict[str, str] = field(default_factory=dict)
    cards: list[CardEntry] = field(default_factory=list)
    swap_board: list[CardEntry] = field(default_factory=list)
    # Cards entombed from the 99 (ADR 27): out of the deck, but not gone. Each
    # entry keeps its category and its `why` -- the user's own words, which is
    # what lets a return restore the card without anything being re-invented --
    # and stays here until it is returned or exiled. Newest first.
    graveyard: list[CardEntry] = field(default_factory=list)
    source_path: Path | None = None

    # ---- derived views -------------------------------------------------

    @property
    def total_cards(self) -> int:
        """Cards in the 99 (commander and companion sit outside it)."""
        return sum(c.qty for c in self.cards)

    @property
    def by_category(self) -> dict[str, list[CardEntry]]:
        out: dict[str, list[CardEntry]] = {}
        for card in self.cards:
            out.setdefault(card.category, []).append(card)
        return out

    @property
    def category_counts(self) -> dict[str, int]:
        return {k: sum(c.qty for c in v) for k, v in self.by_category.items()}

    @property
    def land_count(self) -> int:
        return self.category_counts.get("land", 0)

    @property
    def unjustified(self) -> list[CardEntry]:
        """Cards with no `why` yet -- the work a draft still owes.

        A count rather than a wall of red is the whole argument of ADR 13: "17
        cards still need a rationale" is a to-do list, and promoting the deck
        to `curated` is what finishing it looks like.
        """
        return [c for c in self.cards if not c.why.strip()]

    def card_names(self, *, include_commander: bool = True) -> list[str]:
        names: list[str] = []
        if include_commander:
            names.extend(self.commander)
        for card in self.cards:
            names.extend([card.name] * card.qty)
        return names

    # ---- io ------------------------------------------------------------

    @classmethod
    def load(cls, path: str | Path) -> Deck:
        path = Path(path)
        if path.is_dir():
            path = path / "deck.yaml"
        return cls.from_text(path.read_text(encoding="utf-8"),
                             slug=path.parent.name, source_path=path)

    @classmethod
    def from_text(cls, text: str, *, slug: str | None = None,
                  source_path: Path | None = None) -> Deck:
        """Parse deck YAML that is not necessarily a file.

        `docs/HOSTING.md` stores a user's deck as the same YAML in a database
        row, so the parser has to work on text. One parser, one validator, two
        sources -- which is the property ADR 4 is relying on.
        """
        raw = load_yaml(text) or {}

        commander = raw.get("commander") or []
        if isinstance(commander, str):
            commander = [commander]

        return cls(
            slug=raw.get("slug") or slug or "",
            name=raw.get("name") or slug or "",
            status=str(raw.get("status") or "theoretical").strip().lower(),
            stage=str(raw.get("stage") or "curated").strip().lower(),
            # `is None` rather than `or True`: `shared: false` is falsey, and
            # the whole point of the field is that it can say no.
            shared=True if raw.get("shared") is None else bool(raw["shared"]),
            commander=list(commander),
            commander_art=str(raw.get("commander_art") or "").strip(),
            companion=raw.get("companion"),
            bracket=raw.get("bracket"),
            strategy=raw.get("strategy", ""),
            notes=dict(raw.get("notes", {})),
            cards=[CardEntry.from_obj(c) for c in raw.get("cards", [])],
            swap_board=[CardEntry.from_obj(c) for c in raw.get("swap_board", [])],
            graveyard=[CardEntry.from_obj(c) for c in raw.get("graveyard", [])],
            source_path=source_path,
        )

    def dump(self, path: str | Path | None = None) -> str:
        payload: dict[str, Any] = {
            "slug": self.slug,
            "name": self.name,
            "status": self.status,
            "stage": self.stage,
            "commander": self.commander,
        }
        # Written only when it says no, for the same reason `commander_art` is
        # written only when set: absent already means shared, so emitting
        # `shared: true` would rewrite all six curated files to assert the
        # default they already had.
        if not self.shared:
            payload["shared"] = False
        # Omitted when unset, so the six decks that predate it are unchanged by
        # a round trip and a `commander_art:` line in a diff always means
        # somebody picked one.
        if self.commander_art:
            payload["commander_art"] = self.commander_art
        if self.companion:
            payload["companion"] = self.companion
        if self.bracket is not None:
            payload["bracket"] = self.bracket
        if self.strategy:
            payload["strategy"] = self.strategy
        if self.notes:
            payload["notes"] = self.notes
        cards = [c.to_obj() for c in self.cards]
        if self.stage == "draft":
            # A blank `why:` is the to-do list written into the file itself, so
            # the work shows up where it has to be done rather than only in the
            # gate's output. Omitted for a curated deck, where an empty
            # rationale is a blocking error and should not be pre-typed.
            for obj in cards:
                obj.setdefault("why", "")
        payload["cards"] = cards
        if self.swap_board:
            payload["swap_board"] = [c.to_obj() for c in self.swap_board]
        # Written only when occupied, like the swap board: an empty graveyard
        # is the normal state and six curated decks should not each grow a
        # `graveyard: []` line asserting it.
        if self.graveyard:
            payload["graveyard"] = [c.to_obj() for c in self.graveyard]

        text = yaml.dump(payload, Dumper=_Dumper, sort_keys=False,
                         allow_unicode=True, width=100, default_flow_style=False)
        if path is not None:
            target = Path(path)
            if target.is_dir():
                target = target / "deck.yaml"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(text, encoding="utf-8")
        return text
