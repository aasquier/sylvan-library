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
    commander: list[str] = field(default_factory=list)
    companion: str | None = None
    bracket: int | None = None
    strategy: str = ""
    # Free-form notes that flow into the advanced primer.
    notes: dict[str, str] = field(default_factory=dict)
    cards: list[CardEntry] = field(default_factory=list)
    swap_board: list[CardEntry] = field(default_factory=list)
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
        raw = yaml.safe_load(text) or {}

        commander = raw.get("commander") or []
        if isinstance(commander, str):
            commander = [commander]

        return cls(
            slug=raw.get("slug") or slug or "",
            name=raw.get("name") or slug or "",
            status=str(raw.get("status") or "theoretical").strip().lower(),
            stage=str(raw.get("stage") or "curated").strip().lower(),
            commander=list(commander),
            companion=raw.get("companion"),
            bracket=raw.get("bracket"),
            strategy=raw.get("strategy", ""),
            notes=dict(raw.get("notes", {})),
            cards=[CardEntry.from_obj(c) for c in raw.get("cards", [])],
            swap_board=[CardEntry.from_obj(c) for c in raw.get("swap_board", [])],
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

        text = yaml.dump(payload, Dumper=_Dumper, sort_keys=False,
                         allow_unicode=True, width=100, default_flow_style=False)
        if path is not None:
            target = Path(path)
            if target.is_dir():
                target = target / "deck.yaml"
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(text, encoding="utf-8")
        return text
