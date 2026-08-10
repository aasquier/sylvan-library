"""The gate. Nothing is emitted until the deck passes this.

This module exists because of two specific, real mistakes: proposing
Ajani, Nacatl Pariah for a G/W deck (its back face is R/W, so its identity is
illegal) and describing Arahbo's doubling ability as eminence (it is not --
eminence is only the +3/+3, the doubling needs him on the battlefield).

Both are *checkable facts*, and both were missed by reasoning from memory
instead of from the card. So identity and oracle text are now looked up, and
`mtglab decks validate` fails loudly rather than producing a confident,
wrong document.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from mtglab.decks.model import CATEGORIES, Deck

SINGLETON_EXEMPT = {
    "plains", "island", "swamp", "mountain", "forest", "wastes",
    "snow-covered plains", "snow-covered island", "snow-covered swamp",
    "snow-covered mountain", "snow-covered forest",
}


@dataclass
class Issue:
    level: str          # "error" blocks generation; "warn" does not
    code: str
    message: str
    card: str | None = None

    def __str__(self) -> str:
        where = f" [{self.card}]" if self.card else ""
        return f"{self.level.upper():<5} {self.code}{where}: {self.message}"


@dataclass
class ValidationReport:
    issues: list[Issue] = field(default_factory=list)

    @property
    def errors(self) -> list[Issue]:
        return [i for i in self.issues if i.level == "error"]

    @property
    def warnings(self) -> list[Issue]:
        return [i for i in self.issues if i.level == "warn"]

    @property
    def ok(self) -> bool:
        return not self.errors

    def add(self, level: str, code: str, message: str, card: str | None = None) -> None:
        self.issues.append(Issue(level, code, message, card))

    def render(self) -> str:
        if not self.issues:
            return "OK -- no issues."
        return "\n".join(str(i) for i in self.issues)


def validate(deck: Deck, cards: dict | None = None, *,
             expected_size: int = 99) -> ValidationReport:
    """Validate a deck. `cards` maps name -> CardRecord from the local corpus.

    Passing `cards=None` runs only the structural checks and warns that the
    card-level checks were skipped -- it never silently claims the deck is fine.
    """
    rep = ValidationReport()

    # ---- structure ------------------------------------------------------
    if not deck.commander:
        rep.add("error", "no-commander", "deck has no commander")

    if deck.total_cards != expected_size:
        rep.add("error", "deck-size",
                f"deck has {deck.total_cards} cards in the 99, expected {expected_size}")

    seen: dict[str, int] = {}
    for card in deck.cards:
        key = card.name.lower()
        seen[key] = seen.get(key, 0) + card.qty
    for name, count in seen.items():
        if count > 1 and name not in SINGLETON_EXEMPT:
            rep.add("error", "singleton", f"appears {count} times", name)

    for cmd in deck.commander:
        if cmd.lower() in seen:
            rep.add("error", "commander-in-99",
                    "commander is also listed in the 99", cmd)

    for card in deck.cards:
        if card.category not in CATEGORIES:
            rep.add("warn", "unknown-category",
                    f"category {card.category!r} is not one of {', '.join(CATEGORIES)}",
                    card.name)
        if not card.why.strip():
            rep.add("error", "missing-rationale",
                    "no `why` -- every inclusion must justify itself", card.name)

    # ---- card-level -----------------------------------------------------
    if cards is None:
        rep.add("warn", "unverified",
                "no card corpus supplied; identity, legality and text were NOT checked")
        return rep

    all_names = deck.commander + [c.name for c in deck.cards] + \
        [c.name for c in deck.swap_board]
    if deck.companion:
        all_names.append(deck.companion)

    missing = [n for n in all_names if n not in cards]
    for name in missing:
        rep.add("error", "unknown-card",
                "not found in the local corpus -- check spelling, or refresh "
                "the Scryfall data if this is a new card", name)

    # Colour identity is derived from the commander(s), so it must resolve.
    cmd_records = [cards[c] for c in deck.commander if c in cards]
    if cmd_records:
        identity: frozenset[str] = frozenset()
        for rec in cmd_records:
            identity |= rec.color_identity
        for rec in cmd_records:
            if "Legendary" not in rec.type_line or "Creature" not in rec.type_line:
                if "can be your commander" not in (rec.oracle_text or "").lower():
                    rep.add("error", "not-a-commander",
                            f"type line is {rec.type_line!r} and it does not say "
                            "it can be your commander", rec.name)

        for card in deck.cards:
            rec = cards.get(card.name)
            if rec is None:
                continue
            illegal = rec.color_identity - identity
            if illegal:
                rep.add("error", "color-identity",
                        f"identity {{{''.join(sorted(rec.color_identity))}}} includes "
                        f"{{{''.join(sorted(illegal))}}}, outside the commander's "
                        f"{{{''.join(sorted(identity)) or 'C'}}}", card.name)
            if not rec.legal_commander:
                rep.add("error", "banned",
                        "not legal in Commander", card.name)
            if card.category == "land" and not rec.is_land:
                rep.add("warn", "category-mismatch",
                        f"filed under 'land' but type line is {rec.type_line!r}",
                        card.name)
            if card.category != "land" and rec.is_land:
                rep.add("warn", "category-mismatch",
                        f"is a land but filed under {card.category!r}", card.name)

    # ---- companion ------------------------------------------------------
    if deck.companion:
        rec = cards.get(deck.companion)
        if rec and "companion" not in (rec.oracle_text or "").lower():
            rep.add("error", "not-a-companion",
                    "listed as companion but has no Companion ability", deck.companion)

    return rep


def reserved_list(deck: Deck, cards: dict) -> list[str]:
    """Reserved List cards in the deck -- surfaced because it is a standing
    constraint the user toggles per deck, not a fixed rule."""
    return sorted(c.name for c in deck.cards
                  if (r := cards.get(c.name)) is not None and r.reserved)
