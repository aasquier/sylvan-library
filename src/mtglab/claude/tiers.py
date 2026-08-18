"""Which Claude answers for whom.

Every Claude surface has called one model since ADR 14: Sonnet, chosen because
most of what the modes do is conversation over tool results the pool already
computed, and because "find out whether it is enough" is cheaper than assuming
it is not. `client.MODEL` still says that, and it is still the answer for
anybody this module does not name.

What changed (2026-08-18, Aaron's call) is that the answer is now **per
account**. A handful of people -- Aaron, his sister, a few friends -- may reach
for a more capable model, and everybody else stays on the default. That is a
different decision from the one ADR 14 deferred: not "is Sonnet enough for this
project", which is still open, but "this instance is free and somebody has to
be able to spend more on one seat than on another".

Three rules hold this together, and each is here rather than at a call site:

**A tier is a name, never a model id.** Accounts carry `'opus'`, not
`claude-opus-5`. A model id is a thing that gets superseded -- twice already in
this file's short history -- and a column full of them is a migration every
time. `resolve` is the only place the two meet.

**An unknown tier resolves to the default rather than raising.** The column is
data on a volume that outlives any deploy, so a tier this code no longer knows
about is a thing that *will* happen -- a renamed key, a rolled-back deploy, a
row somebody edited with `sqlite3`. The failure a person wants there is "you
got the ordinary model", not a 500 on every Claude surface they touch.

**The roster is what the Admin page offers.** `TIERS` is serialised to the
client and the client sends a key back; there is no second list in TypeScript
to drift from this one, and `blurb` is written to be read by the maintainer
choosing, not by the person chosen for.

On what the models cost: nothing here prices anything, deliberately. That
belongs beside the usage ledger, which is where a number in dollars can be put
next to the tokens it came from.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Tier:
    """One seat's model, and why a maintainer would grant it."""

    key: str
    #: The model id `client.model` resolves this to. The one place a model id
    #: appears outside `client.MODEL`.
    model: str
    label: str
    #: One line, addressed to whoever is deciding. Rendered on the Admin page.
    blurb: str


#: The tier every account has unless a maintainer says otherwise. Its key is
#: also what `resolve` falls back to, so it must never leave `TIERS`.
DEFAULT_TIER = "sonnet"

TIERS: tuple[Tier, ...] = (
    Tier(
        key="sonnet",
        model="claude-sonnet-5",
        label="Sonnet",
        blurb="The house answer. Fast, and enough for conversation over facts "
              "the pool already worked out.",
    ),
    Tier(
        key="opus",
        model="claude-opus-5",
        label="Opus",
        blurb="Deeper reasoning on the questions the pool cannot settle — the "
              "meta, a commander's place in history, a theme read from a "
              "person rather than a card.",
    ),
    Tier(
        key="fable",
        model="claude-fable-5",
        label="Fable",
        blurb="The most capable there is, and the most expensive. Worth a seat "
              "only where the answer is the whole point.",
    ),
)

_BY_KEY = {tier.key: tier for tier in TIERS}

# Stated as an assertion rather than trusted: `resolve` returning `None` for a
# key it was told is always valid would be a very quiet bug.
assert DEFAULT_TIER in _BY_KEY


def get(key: str | None) -> Tier:
    """The tier `key` names, or the default for anything else.

    `None` is the ordinary case — an account nobody has granted anything. An
    *unknown* key lands here too, and that is the deliberate part: see the
    module docstring on why a stale value in the column must not be an error.
    """
    if key is None:
        return _BY_KEY[DEFAULT_TIER]
    return _BY_KEY.get(key, _BY_KEY[DEFAULT_TIER])


def resolve(key: str | None) -> str:
    """The model id an account on `key` should be answered by."""
    return get(key).model


def known(key: str) -> bool:
    """Whether `key` names a tier. The check a write path owes the caller.

    Reading tolerates an unknown key; *writing* one must not, or the Admin
    page's next release quietly grants a tier that does not exist.
    """
    return key in _BY_KEY


def roster() -> list[dict[str, str]]:
    """The tiers, for the Admin page. Keys and prose — never a model id.

    Commandment 10: no technology a user may see is ever named, and a model id
    is exactly that. The maintainer picks "Opus"; what that resolves to is this
    module's business.
    """
    return [{"key": t.key, "label": t.label, "blurb": t.blurb} for t in TIERS]
