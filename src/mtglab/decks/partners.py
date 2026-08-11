"""Two-commander pairings: Partner, Backgrounds, and their relatives.

The gate assumed one commander. That produced a wrong answer on a perfectly
legal deck: a Background is a `Legendary Enchantment — Background`, is not a
creature, and its text never says it can be your commander, so
`not-a-commander` rejected Jaheira + Raised by Giants -- a legal Baldur's Gate
pairing.

And a two-commander deck holds **98** cards, not 99, which the size check knew
nothing about. Commanders sit inside the 100; a companion does not, which is
why it is "effectively a 101st card" and does not change the deck size.

**Legendary is still required.** Battlebond printed ten *non-legendary*
creatures with `Partner with` (Lore Weaver, Ley Weaver, Chakram Slinger and
friends) for Two-Headed Giant limited. They are not commanders, and the
official ruling on those cards says so outright: "A nonlegendary creature can't
be your commander, even if it has a 'partner with' ability." Only the
Background exemption is real -- it is legal despite not being a creature
because "Choose a Background" makes it a second commander.

As in `companion.py`, the mechanics here were enumerated from the corpus rather
than from memory. That turned up more than the familiar list: `Partner—<label>`
is a generalised template, and the corpus currently carries four distinct
labels (Friends forever, Survivors, Character select, Father & son). Matching
on the label rather than hardcoding "Friends forever" means a new set adds
labels without touching this file.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

PARTNER = "partner"
PARTNER_WITH = "partner-with"
LABELED = "labeled-partner"
BACKGROUND_CHOOSER = "background-chooser"
DOCTORS_COMPANION = "doctors-companion"

# A card's own type line marks these; they carry no pairing ability of their
# own and can only ever be the *second* commander.
BACKGROUND_TYPE = "Background"
DOCTOR_TYPE = "Time Lord Doctor"


@dataclass(frozen=True)
class Pairing:
    kind: str
    label: str = ""             # LABELED: the text after "Partner—"
    partner_name: str = ""      # PARTNER_WITH: the specific card named

    def describe(self) -> str:
        if self.kind == LABELED:
            return f"Partner—{self.label}"
        if self.kind == PARTNER_WITH:
            return f"Partner with {self.partner_name}"
        return {
            PARTNER: "Partner",
            BACKGROUND_CHOOSER: "Choose a Background",
            DOCTORS_COMPANION: "Doctor's companion",
        }[self.kind]


def _front(type_line: str) -> str:
    return (type_line or "").split(" // ")[0]


def is_background(rec: Any) -> bool:
    return BACKGROUND_TYPE in _front(rec.type_line)


def is_doctor(rec: Any) -> bool:
    return DOCTOR_TYPE in _front(rec.type_line)


def pairing(rec: Any) -> Pairing | None:
    """The pairing ability printed on a card, if any."""
    text = rec.oracle_text or ""
    for line in text.split("\n"):
        line = line.strip()
        # Order matters: "Partner with" and "Partner—" both start with
        # "Partner", so the specific forms are tested first.
        m = re.match(r"^Partner with ([^(]+)", line)
        if m:
            return Pairing(PARTNER_WITH, partner_name=m.group(1).strip())
        m = re.match(r"^Partner\s*—\s*([^(]+)", line)
        if m:
            return Pairing(LABELED, label=m.group(1).strip())
        if re.match(r"^Partner(\s*\(|$)", line):
            return Pairing(PARTNER)
        if line.startswith("Choose a Background"):
            return Pairing(BACKGROUND_CHOOSER)
        if line.startswith("Doctor's companion"):
            return Pairing(DOCTORS_COMPANION)
    return None


def can_be_commander(rec: Any, *, paired: bool = False) -> bool:
    """Is this card legal in the command zone?

    `paired=True` when it is one of two commanders. That is what makes a
    Background legal -- and it is the *only* thing pairing changes. A pairing
    ability never waives the legendary requirement: the official ruling on the
    Battlebond partners is "A nonlegendary creature can't be your commander,
    even if it has a 'partner with' ability."
    """
    front = _front(rec.type_line)
    if "Legendary" in front and "Creature" in front:
        return True
    if "can be your commander" in (rec.oracle_text or "").lower():
        return True
    # A Background is a Legendary Enchantment, so it is never a commander on
    # its own -- but "Choose a Background" makes it a legal second one.
    return bool(paired and is_background(rec))


def nonlegendary_partner(rec: Any) -> bool:
    """A `Partner with` card that is not legendary, so it can never be a
    commander. Battlebond printed ten of these for Two-Headed Giant limited
    and they are a standing trap when reading the ability alone."""
    if "Legendary" in _front(rec.type_line):
        return False
    p = pairing(rec)
    return p is not None and p.kind == PARTNER_WITH


def _match(a: Any, pa: Pairing | None, b: Any, pb: Pairing | None) -> bool:
    """Is this ordered pair legal? Callers try both orders."""
    if pa is None:
        return False
    if pa.kind == PARTNER:
        return pb is not None and pb.kind == PARTNER
    if pa.kind == LABELED:
        return pb is not None and pb.kind == LABELED and pb.label.lower() == pa.label.lower()
    if pa.kind == PARTNER_WITH:
        return pa.partner_name.lower() == b.name.lower()
    if pa.kind == BACKGROUND_CHOOSER:
        return is_background(b)
    if pa.kind == DOCTORS_COMPANION:
        return is_doctor(b)
    return False


def check_pair(a: Any, b: Any) -> str | None:
    """None if the two cards may be commanders together, else why not."""
    pa, pb = pairing(a), pairing(b)

    if _match(a, pa, b, pb) or _match(b, pb, a, pa):
        return None

    if pa is None and pb is None:
        return ("neither card has a pairing ability, so they cannot be two "
                "commanders together")

    # Both have an ability but they do not fit each other. Say why precisely,
    # because "invalid pair" is useless at the table.
    if pa and pb and pa.kind == LABELED and pb.kind == LABELED:
        return (f"{a.name} has Partner—{pa.label} but {b.name} has "
                f"Partner—{pb.label}; both must have the same one")
    for one, p_one, other in ((a, pa, b), (b, pb, a)):
        if p_one is None:
            continue
        if p_one.kind == PARTNER_WITH:
            return (f"{one.name} has Partner with {p_one.partner_name}, so it "
                    f"can only pair with that card, not {other.name}")
        if p_one.kind == BACKGROUND_CHOOSER:
            return (f"{one.name} chooses a Background, but {other.name} is not "
                    f"a Background")
        if p_one.kind == DOCTORS_COMPANION:
            return (f"{one.name} is a Doctor's companion, but {other.name} is "
                    f"not a Doctor")
    lone = a if pa else b
    return (f"only {lone.name} has a pairing ability; both commanders need "
            f"one that matches")
