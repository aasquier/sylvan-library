"""The stance: how much of this the user wants, and off is a real position.

[ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
splits a Claude surface into a **mode** (a system prompt, a tool set, and what
it may write) and a **stance** (the user's dial over it). This module is the
dial. There are no modes yet -- deliberately: the stance is the frame every
mode plugs into, and retrofitting a gate around modes that already exist is how
the gate ends up with holes in it.

**Three axes, not one slider.** "Never interrupt me, but go wild when I ask" is
a coherent and probably common setting that a single slider cannot express, so
initiative, scope and write autonomy move independently. Named presets exist
because most people will never touch the axes.

**Off means no calls.** Not quiet-but-still-watching, not a muted UI over a
running integration -- `allows_calls()` returns False and nothing reaches the
network. That is a trust property first and a cost control second, and it is
why this module is deterministic, needs no key, and is testable with no
network: the decision not to call cannot itself depend on a call.

**A stance may widen what a mode does; never what it is allowed to do.** The
capability set lives in `tools.READ_ONLY` and the write surface is closed by
`tests/test_claude_boundary.py`. Nothing here can open either. In particular
the top of the write axis is *not* permission to write a rationale -- rule 4
holds at every stance, and this module cannot name a write function at all.

**The ceiling is the deployment's, not the user's.** `ceiling()` reads the
environment, so a hosted instance can cap what any user may select while a
local run on the maintainer's own key does not have to. `resolve()` clamps a
request to it. That is the shape docs/HOSTING.md §7 wants: the dial is a user
preference, the cap is an operator decision, and the two are separate.
"""

from __future__ import annotations

import os
from dataclasses import dataclass, replace
from typing import Any

# Each axis is ordered, least to most. Index *is* the level, which is what
# makes clamping a `min()` rather than a table of special cases.
INITIATIVE = ("off", "on-request", "volunteers", "interjects")
SCOPE = ("flagged", "adjacent", "rethink")
WRITE = ("none", "proposes", "applies")

AXES = ("initiative", "scope", "write")
LEVELS: dict[str, tuple[str, ...]] = {
    "initiative": INITIATIVE, "scope": SCOPE, "write": WRITE,
}

AXIS_QUESTIONS = {
    "initiative": "When may it speak?",
    "scope": "How far from the question may it range?",
    "write": "What may it change?",
}

LEVEL_MEANINGS = {
    ("initiative", "off"): "Never. No API calls are made at all.",
    ("initiative", "on-request"): "Only when you ask it something.",
    ("initiative", "volunteers"): "It may offer an observation when it has one.",
    ("initiative", "interjects"): "It may speak up while you are working.",
    ("scope", "flagged"): "Only the cards the gate already flagged.",
    ("scope", "adjacent"): "Those cards and the ones that interact with them.",
    ("scope", "rethink"): "The whole deck, including its axis.",
    ("write", "none"): "Nothing. It talks; you type.",
    ("write", "proposes"): "It may assemble a batch for you to approve in one go.",
    ("write", "applies"): "It may make reversible edits without asking each time.",
}


def _index(axis: str, level: str) -> int:
    try:
        return LEVELS[axis].index(level)
    except KeyError:
        raise ValueError(f"{axis!r} is not one of {', '.join(AXES)}") from None
    except ValueError:
        raise ValueError(
            f"{level!r} is not a {axis} level; expected one of "
            f"{', '.join(LEVELS[axis])}") from None


@dataclass(frozen=True)
class Stance:
    """One setting of the three axes.

    Frozen because a stance is a decision, not a mutable session object: a mode
    that could edit the stance it was handed would be widening its own dial,
    which is the one thing ADR 15 says a stance is for preventing.
    """

    initiative: str = "off"
    scope: str = "flagged"
    write: str = "none"

    def __post_init__(self) -> None:
        for axis in AXES:
            _index(axis, getattr(self, axis))

    # ---- the questions everything else asks -----------------------------

    @property
    def allows_calls(self) -> bool:
        """Whether anything may reach the API at all.

        The single most important line in this module. `initiative: off` is not
        a quiet mode -- it is the absence of an integration, and every caller
        checks this before building a client rather than after.
        """
        return self.initiative != "off"

    @property
    def may_write(self) -> bool:
        """Whether a mode may change a deck *at all* -- not what it may change.

        False at `write: none`. True above it, and still bounded by the edit
        operations themselves: a card added to a curated deck needs a `why`,
        and no stance can supply one. ADR 15 traces that in full; the point
        here is that this property is necessary and nowhere near sufficient.
        """
        return self.write != "none"

    def at_least(self, axis: str, level: str) -> bool:
        """Is this stance at or above `level` on `axis`?"""
        return _index(axis, getattr(self, axis)) >= _index(axis, level)

    def to_dict(self) -> dict[str, str]:
        return {axis: getattr(self, axis) for axis in AXES}

    @classmethod
    def from_obj(cls, obj: Any) -> Stance:
        """Parse a stance from a preset name or a mapping of axes.

        Accepts a partial mapping: unnamed axes take `OFF`'s value rather than
        the most permissive one, so a malformed or half-written request can
        only ever be quieter than intended.
        """
        if obj is None:
            return OFF
        if isinstance(obj, Stance):
            return obj
        if isinstance(obj, str):
            return preset(obj)
        if isinstance(obj, dict):
            unknown = sorted(set(obj) - set(AXES))
            if unknown:
                raise ValueError(
                    f"{unknown} are not stance axes; expected "
                    f"{', '.join(AXES)}")
            return replace(OFF, **{a: obj[a] for a in AXES if a in obj})
        raise ValueError(f"cannot read a stance from {type(obj).__name__}")


# ------------------------------------------------------------- the presets

OFF = Stance("off", "flagged", "none")

#: Answers when asked, stays inside what the gate flagged, writes nothing.
#: The first position that does anything, and the safest one that does.
CONSULTANT = Stance("on-request", "flagged", "none")

#: Offers observations unprompted and will look at neighbouring cards. Still
#: writes nothing -- the jump to volunteering is about *when* it speaks.
SECOND_OPINION = Stance("volunteers", "adjacent", "none")

#: Will question the deck's whole axis, and will assemble an edit batch for one
#: approval. Note the write level: `proposes`, not `applies`. Nothing lands
#: without a person looking at it.
COLLABORATOR = Stance("volunteers", "rethink", "proposes")

PRESETS: dict[str, Stance] = {
    "off": OFF,
    "consultant": CONSULTANT,
    "second-opinion": SECOND_OPINION,
    "collaborator": COLLABORATOR,
}

PRESET_BLURBS = {
    "off": "No calls, ever. The gate, the solver and the simulator all still "
           "work -- this turns off opinions, not the tool.",
    "consultant": "Speaks when spoken to, about what the gate already flagged.",
    "second-opinion": "Volunteers observations and looks at neighbouring "
                      "cards. Still writes nothing.",
    "collaborator": "Questions the deck's axis and batches edits for one "
                    "approval. Never writes a rationale -- that rule has no "
                    "stance above it.",
}


def preset(name: str) -> Stance:
    key = (name or "").strip().lower()
    if key not in PRESETS:
        raise ValueError(
            f"{name!r} is not a stance preset; expected one of "
            f"{', '.join(sorted(PRESETS))}")
    return PRESETS[key]


# ------------------------------------------------------------ the defaults

def default_for(deck: Any) -> Stance:
    """The stance a deck starts at, derived from what the deck already says.

    ADR 15: `status: built | theoretical` already draws the distinction that
    matters. Goreclaw and Tivit are lists under consideration, where a wild
    suggestion costs a moment's thought; Arahbo and Gyome are sleeved
    cardboard, where acting on one costs money and a trip to the box. So a
    theoretical deck defaults to a wider stance and a built deck to a narrower
    one, and neither needs configuring.

    Still conservative on both counts: the widest default is `SECOND_OPINION`,
    which writes nothing. Reaching a stance that can edit is always a choice
    somebody makes.
    """
    status = str(getattr(deck, "status", "") or "").strip().lower()
    return SECOND_OPINION if status == "theoretical" else CONSULTANT


# ------------------------------------------------------------- the ceiling

CEILING_ENV = "MTGLAB_CLAUDE_STANCE_CEILING"


def ceiling() -> Stance:
    """The most permissive stance this deployment allows anyone to select.

    Defaults to `COLLABORATOR` -- the top of what presets offer -- because a
    local run on the maintainer's own key should not need configuring to work.
    A hosted instance sets `MTGLAB_CLAUDE_STANCE_CEILING` (a preset name) and
    every request is clamped to it, including ones from a client that asks for
    more. Set it to `off` and the deployment makes no calls at all, whatever
    any user has saved.

    Read at call time, not at import: an operator changing the cap should not
    have to restart the process to lower it.
    """
    raw = os.environ.get(CEILING_ENV)
    if not raw:
        return COLLABORATOR
    try:
        return preset(raw)
    except ValueError:
        # An unreadable ceiling is not a reason to run uncapped. Fail closed:
        # a typo in a deployment variable should cost a feature, not open one.
        return OFF


def clamp(requested: Stance, limit: Stance) -> Stance:
    """`requested`, with every axis lowered to `limit` where it exceeds it.

    Per axis rather than whole-stance, so a ceiling of "may speak freely but
    may never write" is expressible -- which is the shape a hosted instance
    most plausibly wants.
    """
    return Stance(**{
        axis: LEVELS[axis][min(_index(axis, getattr(requested, axis)),
                               _index(axis, getattr(limit, axis)))]
        for axis in AXES
    })


def resolve(requested: Any = None, *, deck: Any = None,
            limit: Stance | None = None) -> Stance:
    """The stance that actually applies: what was asked, defaulted, then capped.

    The one function callers should need. `requested` may be a `Stance`, a
    preset name, a partial mapping, or None -- None means "use the deck's
    default", which is how a UI that has never been touched still behaves
    sensibly.
    """
    if requested is None:
        asked = default_for(deck) if deck is not None else OFF
    else:
        asked = Stance.from_obj(requested)
    return clamp(asked, ceiling() if limit is None else limit)


def describe(stance: Stance) -> dict[str, Any]:
    """A stance rendered for a UI: the axes, their levels, and what each means.

    Includes `preset` when the stance happens to equal one, so a dial that was
    set by name can show the name back rather than three axis readings.
    """
    named = next((n for n, s in PRESETS.items() if s == stance), None)
    return {
        "preset": named,
        "allows_calls": stance.allows_calls,
        "may_write": stance.may_write,
        "axes": [{
            "axis": axis,
            "question": AXIS_QUESTIONS[axis],
            "level": getattr(stance, axis),
            "means": LEVEL_MEANINGS[(axis, getattr(stance, axis))],
            "levels": list(LEVELS[axis]),
        } for axis in AXES],
    }
