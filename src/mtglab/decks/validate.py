"""The gate. Nothing is emitted until the deck passes this.

This module exists because of two specific, real mistakes: proposing
Ajani, Nacatl Pariah for a G/W deck (its back face is R/W, so its identity is
illegal) and describing Arahbo's doubling ability as eminence (it is not --
eminence is only the +3/+3, the doubling needs him on the battlefield).

Both are *checkable facts*, and both were missed by reasoning from memory
instead of from the card. So identity and oracle text are now looked up, and
`mtglab decks validate` fails loudly rather than producing a confident,
wrong document.

The gate has exactly one mode switch, and it is `deck.stage` (ADR 13). In a
draft a missing `why` warns; in a curated deck it blocks. Everything else --
legality, colour identity, singleton, size, companion and partner rules -- is a
fact about cards, and facts do not become negotiable because a deck is new. The
stage is read off the deck rather than passed in per call so that there is one
answer to "what is this deck", not a caller's opinion of it.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from mtglab.decks import companion, partners
from mtglab.decks.model import (
    ARCHETYPES,
    CATEGORIES,
    DECK_STAGES,
    DECK_STATUSES,
    THEMES,
    Deck,
)

if TYPE_CHECKING:
    from mtglab.cards.db import CardRecord

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


def validate(deck: Deck, cards: dict[str, CardRecord] | None = None, *,
             expected_size: int = 99) -> ValidationReport:
    """Validate a deck. `cards` maps name -> CardRecord from the local pool.

    Passing `cards=None` runs only the structural checks and warns that the
    card-level checks were skipped -- it never silently claims the deck is fine.
    """
    rep = ValidationReport()

    # ---- structure ------------------------------------------------------
    if not deck.commander:
        rep.add("error", "no-commander", "deck has no commander")

    # Structural, so it belongs above the `cards is None` early return -- a
    # nonsense status must not become invisible whenever the pool is missing.
    if deck.status not in DECK_STATUSES:
        rep.add("error", "deck-status",
                f"status {deck.status!r} is not one of {', '.join(DECK_STATUSES)}")

    # An unrecognised stage is an error rather than a silent fallback to
    # `curated`. Falling back would make a typo -- `stage: drafted` -- quietly
    # turn every missing rationale into a blocking error, which is the opposite
    # of what the author asked for and hard to work out from the output.
    if deck.stage not in DECK_STAGES:
        rep.add("error", "deck-stage",
                f"stage {deck.stage!r} is not one of {', '.join(DECK_STAGES)}")
    drafting = deck.stage == "draft"

    # The labelling axes warn rather than block, the same bargain as
    # `unknown-category` below: the warning is for a hand-written file, while
    # an edit through the app chooses from the vocabulary and is refused
    # outright by `edit.set_deck_field`. Absent is fine -- an unlabelled deck
    # is undeclared, not wrong -- but a label outside the vocabulary would
    # silently fragment the very grouping it exists for.
    if deck.archetype and deck.archetype not in ARCHETYPES:
        rep.add("warn", "unknown-archetype",
                f"archetype {deck.archetype!r} is not one of "
                f"{', '.join(ARCHETYPES)}")
    for theme in deck.themes:
        if theme not in THEMES:
            rep.add("warn", "unknown-theme",
                    f"theme {theme!r} is not in the vocabulary; the list "
                    f"grows by editing THEMES in decks/model.py")

    # Two commanders share the command zone, so the deck holds 98 rather than
    # 99. `expected_size` is the single-commander default; adjust it by however
    # many commanders there actually are.
    reasons = []
    if len(deck.commander) > 1:
        expected_size -= len(deck.commander) - 1
        reasons.append(f"{len(deck.commander)} commanders")

    # Yorion is the one companion that changes how big the deck must be
    # ("at least twenty cards more than the minimum deck size"), so the size
    # check has to know about it or a legal Yorion deck fails here. Keyed by
    # name because this runs before the pool is consulted, and because deck
    # size is a property of the deck rather than a judgement about a card.
    bonus = companion.DECK_SIZE_BONUS.get((deck.companion or "").lower(), 0)
    if bonus:
        expected_size += bonus
        reasons.append(f"+{bonus} for {deck.companion}")

    if len(deck.commander) > 2:
        rep.add("error", "too-many-commanders",
                f"{len(deck.commander)} commanders listed; Commander allows at "
                "most two, and only with a pairing ability")

    if deck.total_cards != expected_size:
        because = f" ({', '.join(reasons)})" if reasons else ""
        rep.add("error", "deck-size",
                f"deck has {deck.total_cards} cards in the 99, "
                f"expected {expected_size}{because}")

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

    # Rule 4 is not negotiable, but it is also not a wall an imported deck has
    # to climb before anyone can look at it. In a curated deck a missing `why`
    # is a claim the deck cannot support, and it blocks, one error per card.
    #
    # In a draft it is outstanding work, and it collapses to a single counted
    # warning. That is not cosmetic: a fresh 99-card import produces 99 issues
    # that all say the same thing, which is a wall nobody reads and would bury
    # the one error that matters -- ADR 8 requires warnings to stay rare enough
    # to be read. ADR 13's argument is precisely that a number is a better
    # prompt than a wall, so the count is the report and the deck file is the
    # per-card list.
    pending = deck.unjustified
    if drafting and pending:
        shown = ", ".join(c.name for c in pending[:6])
        more = f", and {len(pending) - 6} more" if len(pending) > 6 else ""
        rep.add("warn", "draft-incomplete",
                f"{len(pending)} of {len(deck.cards)} cards still need a `why` "
                f"({shown}{more}). Write them, then set `stage: curated` -- the "
                "gate refuses the promotion while any card is still blank")

    for card in deck.cards:
        if card.category not in CATEGORIES:
            rep.add("warn", "unknown-category",
                    f"category {card.category!r} is not one of {', '.join(CATEGORIES)}",
                    card.name)
        if not card.why.strip() and not drafting:
            rep.add("error", "missing-rationale",
                    "no `why` -- every inclusion must justify itself", card.name)

    # ---- card-level -----------------------------------------------------
    if cards is None:
        rep.add("warn", "unverified",
                "no card pool supplied; identity, legality and text were NOT checked")
        return rep

    all_names = deck.commander + [c.name for c in deck.cards] + \
        [c.name for c in deck.swap_board]
    if deck.companion:
        all_names.append(deck.companion)

    missing = [n for n in all_names if n not in cards]
    for name in missing:
        rep.add("error", "unknown-card",
                "not found in the local pool -- check spelling, or refresh "
                "the Scryfall data if this is a new card", name)

    # Colour identity is derived from the commander(s), so it must resolve.
    cmd_records = [cards[c] for c in deck.commander if c in cards]
    if cmd_records:
        identity: frozenset[str] = frozenset()
        for rec in cmd_records:
            identity |= rec.color_identity
        # A Background is a legal commander only as one of a pair, so whether
        # a card qualifies depends on how many commanders there are.
        paired = len(cmd_records) > 1
        for rec in cmd_records:
            if not partners.can_be_commander(rec, paired=paired):
                if partners.nonlegendary_partner(rec):
                    # The Battlebond trap: the ability is there, the type line
                    # is not. Say so, because "does not say it can be your
                    # commander" reads like a data problem rather than a rule.
                    extra = (" -- a nonlegendary creature can't be your "
                             "commander even with a 'partner with' ability")
                elif not paired and partners.is_background(rec):
                    extra = (" -- a Background is only legal as a second "
                             "commander, and this deck lists one")
                else:
                    extra = ""
                rep.add("error", "not-a-commander",
                        f"type line is {rec.type_line!r} and it does not say "
                        f"it can be your commander{extra}", rec.name)

        if len(cmd_records) == 2:
            problem = partners.check_pair(*cmd_records)
            if problem:
                rep.add("error", "illegal-pairing", problem)

        for card in deck.cards:
            # A fresh name: `rec` above is the loop variable over commander
            # records and is never None, and reusing it here would widen it.
            card_rec = cards.get(card.name)
            if card_rec is None:
                continue
            illegal = card_rec.color_identity - identity
            if illegal:
                rep.add("error", "color-identity",
                        f"identity {{{''.join(sorted(card_rec.color_identity))}}} includes "
                        f"{{{''.join(sorted(illegal))}}}, outside the commander's "
                        f"{{{''.join(sorted(identity)) or 'C'}}}", card.name)
            if not card_rec.legal_commander:
                rep.add("error", "banned",
                        "not legal in Commander", card.name)
            if card.category == "land" and not card_rec.is_land:
                rep.add("warn", "category-mismatch",
                        f"filed under 'land' but type line is {card_rec.type_line!r}",
                        card.name)
            if card.category != "land" and card_rec.is_land:
                rep.add("warn", "category-mismatch",
                        f"is a land but filed under {card.category!r}", card.name)

    # ---- companion ------------------------------------------------------
    if deck.companion:
        _check_companion(deck, cards, identity if cmd_records else None, rep)

    return rep


def _check_companion(deck: Deck, cards: dict[str, CardRecord],
                     identity: frozenset[str] | None,
                     rep: ValidationReport) -> None:
    """Validate the companion itself and its deckbuilding restriction.

    Previously this confirmed the card had a Companion ability and stopped, so
    the restriction -- the entire reason a companion costs you anything -- went
    unchecked, as did the companion's own legality and colour identity.
    """
    name = deck.companion
    if name is None:
        return                      # no companion declared; the caller checks
    rec = cards.get(name)
    if rec is None:
        return                      # already reported as unknown-card above

    if not companion.is_companion(rec):
        rep.add("error", "not-a-companion",
                "listed as companion but has no Companion ability", name)
        return

    if not rec.legal_commander:
        rep.add("error", "companion-banned",
                "not legal in Commander, so it cannot be your companion", name)

    if identity is not None:
        illegal = rec.color_identity - identity
        if illegal:
            rep.add("error", "companion-color-identity",
                    f"identity {{{''.join(sorted(rec.color_identity))}}} includes "
                    f"{{{''.join(sorted(illegal))}}}, outside the commander's "
                    f"{{{''.join(sorted(identity)) or 'C'}}}", name)

    if name.lower() in (c.name.lower() for c in deck.cards):
        rep.add("error", "companion-in-99",
                "the companion sits outside the 100, not in the deck", name)

    # "Your starting deck" includes the commander, and excludes the companion.
    entries = [(c.name, cards[c.name]) for c in deck.cards if c.name in cards]
    entries += [(n, cards[n]) for n in deck.commander if n in cards]

    result = companion.check(name, entries, cards)
    if result.unsupported:
        # Never report a restriction as satisfied when it was never evaluated.
        rep.add("warn", "companion-unchecked",
                f"deckbuilding restriction was NOT verified -- "
                f"{result.unsupported}. Condition: {result.condition or 'unknown'}",
                name)
        return

    if result.violations:
        shown = ", ".join(sorted(result.violations)[:6])
        more = len(result.violations) - 6
        if more > 0:
            shown += f", and {more} more"
        level = "error" if result.exact else "warn"
        detail = "" if result.exact else " (heuristic check -- verify by hand)"
        rep.add(level, "companion-restriction",
                f"{len(result.violations)} card(s) break the companion "
                f"restriction{detail}: {shown}. Condition: {result.condition}",
                name)


def reserved_list(deck: Deck, cards: dict[str, CardRecord]) -> list[str]:
    """Reserved List cards in the deck -- surfaced because it is a standing
    constraint the user toggles per deck, not a fixed rule."""
    return sorted(c.name for c in deck.cards
                  if (r := cards.get(c.name)) is not None and r.reserved)
