"""Ranked replacement candidates for a card that has to leave a deck.

Two decks currently fail the gate on a banned card each, and ADR 8 says the
gate blocks rather than substituting, because picking the replacement is a
deckbuilding judgement that belongs to the deck's owner. This module does not
change that. It answers a narrower, mechanical question:

    of every card that is legal here, which ones most resemble the one being
    removed?

That is a measurement, not a recommendation, and the distinction is the whole
design. Nothing here decides anything; it produces a shortlist with the reason
each candidate scored, so a human can disagree with it quickly. Every fact it
uses comes from the corpus -- CLAUDE.md rule 1 -- so it cannot assert a card
does something it does not.

Scoring is deliberately pure Python over `CardRecord`s. Only `candidate_pool`
touches the database, which keeps the interesting half testable without one.

The score
---------
Similarity is a weighted sum, each part in 0..1:

    card type      0.30   replacing a creature with an instant changes the deck
    mana value     0.20   a 6-drop is not replaced by a 2-drop
    keywords       0.15   Scryfall's own list, so "trample" is a fact not a guess
    oracle text    0.35   how much of what the old card said the new one says

then a popularity nudge of up to 0.10 from EDHREC rank, as a tiebreak only. It
is last and it is small on purpose: popular is not the same as correct, and a
scorer that leads with it just recommends staples.
"""

from __future__ import annotations

import math
import re
from dataclasses import dataclass

from mtglab.cards import db
from mtglab.cards.db import CardRecord, Connection
from mtglab.decks.model import Deck

# Card types, most specific first: a "Legendary Artifact Creature" is a
# creature, and matching it against an artifact would be the wrong call.
PRIMARY_TYPES = (
    "Creature", "Planeswalker", "Land", "Instant", "Sorcery",
    "Artifact", "Enchantment", "Battle",
)

PERMANENT_TYPES = frozenset(
    {"Creature", "Planeswalker", "Land", "Artifact", "Enchantment", "Battle"})

# English function words, plus the Magic templating that appears on almost
# every card and therefore distinguishes nothing. Kept as prose rather than a
# list literal (SIM905) because this is a word list people will edit, and a
# hundred quoted, comma-separated strings is markedly harder to scan and to
# diff than five lines of words.
STOPWORDS = frozenset("""
a an and as at be been but by can cant do does each end for from has have if in
into is it its may more must not of on onto or other others out over own put
same than that the their them then there these they this those to under until
up upon was were when whenever where which while with without you your
also another any are back both get gets much no only rather very
""".split())  # noqa: SIM905

_WORD = re.compile(r"[a-z]+")


@dataclass(frozen=True)
class Candidate:
    """One suggestion, with the arithmetic that produced it."""

    record: CardRecord
    score: float
    reasons: tuple[str, ...]

    @property
    def name(self) -> str:
        return self.record.name


def primary_type(type_line: str) -> str:
    """The card's main type, read from the front face.

    Front face only, for the same reason `CardRecord.is_land` reads it: a
    modal DFC's combined type line names both faces, and matching a creature
    against "Creature — Elf // Land" on the word "Land" is how a spell ends up
    suggested as a land.
    """
    front = (type_line or "").split(" // ")[0]
    for candidate in PRIMARY_TYPES:
        if candidate in front:
            return candidate
    return ""


def _tokens(*texts: str) -> set[str]:
    """Significant words, lowercased, stopwords and short words removed."""
    out: set[str] = set()
    for text in texts:
        for word in _WORD.findall((text or "").lower()):
            if len(word) > 2 and word not in STOPWORDS:
                out.add(word)
    return out


def _type_score(target: CardRecord, candidate: CardRecord) -> float:
    left, right = primary_type(target.type_line), primary_type(candidate.type_line)
    if not left or not right:
        return 0.0
    if left == right:
        return 1.0
    # A permanent for a permanent is a partial match: it still occupies a slot
    # on the battlefield, which is most of what a deck's shape cares about.
    if left in PERMANENT_TYPES and right in PERMANENT_TYPES:
        return 0.4
    return 0.0


def _curve_score(target: CardRecord, candidate: CardRecord) -> float:
    return max(0.0, 1.0 - abs(target.cmc - candidate.cmc) / 3.0)


def _keyword_score(target: CardRecord, candidate: CardRecord) -> float:
    left = {k.lower() for k in target.keywords}
    right = {k.lower() for k in candidate.keywords}
    if not left:
        # Nothing to match against. Scoring 0 rather than renormalising is fine
        # because it is 0 for every candidate, so the ranking is unaffected.
        return 0.0
    return len(left & right) / len(left | right)


def _text_score(target_tokens: set[str], candidate: CardRecord) -> float:
    """How much of what the old card said the new one also says.

    Asymmetric on purpose -- the denominator is the *target's* vocabulary, so a
    long card that happens to contain the target's words scores well, and a
    short card is not rewarded merely for being short.
    """
    if not target_tokens:
        return 0.0
    shared = target_tokens & _tokens(candidate.oracle_text)
    return len(shared) / len(target_tokens)


def _popularity(rec: CardRecord) -> float:
    """EDHREC rank, log-scaled into 0..1. Unranked scores 0.

    Note a banned card is itself unranked, which is a small reminder that this
    number measures what people play, not what is good.
    """
    if not rec.edhrec_rank or rec.edhrec_rank < 1:
        return 0.0
    return max(0.0, 1.0 - math.log10(rec.edhrec_rank) / 5.0)


def score(target: CardRecord, candidate: CardRecord, *, why: str = "") -> Candidate:
    """Score one candidate against the card being replaced.

    `why` is the deck's own rationale for the slot. It is folded into the text
    comparison because it says what the card was *for*, which the oracle text
    often does not -- "ramp and threat in one card" is not a phrase that
    appears on Primeval Titan.
    """
    target_tokens = _tokens(target.oracle_text, why)

    parts = (
        (0.30, _type_score(target, candidate)),
        (0.20, _curve_score(target, candidate)),
        (0.15, _keyword_score(target, candidate)),
        (0.35, _text_score(target_tokens, candidate)),
    )
    similarity = sum(weight * value for weight, value in parts)
    total = similarity + 0.10 * _popularity(candidate)

    reasons: list[str] = []
    if _type_score(target, candidate) == 1.0:
        reasons.append(f"same card type ({primary_type(candidate.type_line)})")
    if candidate.cmc == target.cmc:
        reasons.append(f"same mana value ({int(target.cmc)})")
    elif abs(candidate.cmc - target.cmc) <= 1:
        reasons.append(f"mana value {int(candidate.cmc)} vs {int(target.cmc)}")
    shared_keywords = ({k.lower() for k in target.keywords}
                       & {k.lower() for k in candidate.keywords})
    if shared_keywords:
        reasons.append("shares " + ", ".join(sorted(shared_keywords)))
    shared_words = sorted(target_tokens & _tokens(candidate.oracle_text))
    if shared_words:
        reasons.append("text: " + ", ".join(shared_words[:6]))
    if candidate.edhrec_rank:
        reasons.append(f"EDHREC rank {candidate.edhrec_rank:,}")

    return Candidate(record=candidate, score=round(total, 4),
                     reasons=tuple(reasons))


def rank(target: CardRecord, candidates: list[CardRecord], *, why: str = "",
         limit: int = 5, exclude: frozenset[str] = frozenset()) -> list[Candidate]:
    """Score every candidate and return the best `limit`.

    Ties break by name, so the output is stable run to run -- a shortlist that
    reshuffles itself between two identical runs is one nobody trusts.
    """
    blocked = {n.lower() for n in exclude} | {target.name.lower()}
    scored = [
        score(target, rec, why=why)
        for rec in candidates
        if rec.name.lower() not in blocked
    ]
    scored.sort(key=lambda c: (-c.score, c.name))
    return scored[:limit]


def candidate_pool(con: Connection, target: CardRecord,
                   identity: frozenset[str], *,
                   pool_size: int = 400) -> list[CardRecord]:
    """Everything legal that could plausibly fill the slot.

    A prefilter, not a ranking: it narrows ~35k cards to a few hundred that are
    legal, inside the commander's identity, the right sort of card and roughly
    the right cost, and leaves the judgement to `rank`.
    """
    colors = sorted(c for c in identity if c in "WUBRG")
    listed = ", ".join(f"'{c}'" for c in colors) or "''"

    where = [
        "json_extract_string(legalities, 'commander') = 'legal'",
        f"len(list_filter(color_identity, x -> x NOT IN ({listed}))) = 0",
        "cmc BETWEEN ? AND ?",
    ]
    params: list[object] = [max(0.0, target.cmc - 2), target.cmc + 2]

    kind = primary_type(target.type_line)
    if kind:
        where.append("type_line LIKE ?")
        params.append(f"%{kind}%")

    return db.search(con, " AND ".join(where), params, limit=pool_size,
                     order_by="edhrec_rank NULLS LAST")


def replacements_for(deck: Deck, cards: dict[str, CardRecord], con: Connection,
                     name: str, *,
                     limit: int = 5) -> list[Candidate]:
    """Suggestions for one card in one deck.

    Returns an empty list rather than raising when the corpus does not know the
    card -- the gate has already said so, and failing twice for one problem
    helps nobody.
    """
    target = cards.get(name)
    if target is None:
        return []

    identity: frozenset[str] = frozenset()
    for commander in deck.commander:
        rec = cards.get(commander)
        if rec is not None:
            identity |= rec.color_identity

    why = next((c.why for c in deck.cards if c.name == name), "")
    # Never suggest something already in the list, or on the swap board.
    already = frozenset(
        [c.name for c in deck.cards] + [c.name for c in deck.swap_board] + deck.commander,
    )
    pool = candidate_pool(con, target, identity)
    return rank(target, pool, why=why, limit=limit, exclude=already)
