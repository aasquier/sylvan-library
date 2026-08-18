"""What the tokens cost, in money.

**This module reverses a decision that was written down.** `mtglab claude usage`
reported tokens and not dollars, deliberately, and said why:

    prices move (Sonnet 5's introductory rate ends 2026-08-31) and a stale
    hardcoded price table would turn an honest count into a wrong invoice

That objection was right and is not withdrawn. What changed (2026-08-18,
Aaron's call) is that "how much is this instance costing" became a question
worth answering imperfectly rather than not at all -- an app that is free
forever is one whose bill somebody has to actually watch. So the table exists,
and every part of its design is an answer to the sentence above rather than a
way around it.

**Rates are dated, and the date is rendered.** `CHECKED` is when a human last
read the pricing page, and every surface that shows a figure shows that date
next to it. A number whose age is visible is a number a reader can discount;
one that merely looks current is the wrong invoice the old comment feared.

**A rate that is known to move is modelled, not flattened.** Sonnet 5's
introductory pricing ends 2026-08-31, and that is the whole reason the objection
was phrased the way it was -- so a table that pretended it did not would fail
on its own headline example within a fortnight of being written. A model may
carry a `Window`: a rate, and the date after which a different one applies.
The estimate picks by the conversation's own timestamp, so a bill spanning the
changeover is priced correctly on both sides of it.

**A model the table does not know is priced at nothing and counted.** The
ledger stores the *served-by* id, which can be a model this build never heard
of -- a fallback, a newer id, an A/B via `MTGLAB_CLAUDE_MODEL`. Guessing a rate
for it would be the wrong invoice; charging it zero without saying so would be
worse, because it would read as "cheap". Every caller gets `unpriced` beside
the figure and is expected to render it.

**The figure stays a floor, for the reason it always was.** Prompt cache
*writes* bill at 1.25x input and are recorded nowhere (schema v7 says so in the
column comments), so no arithmetic here can see them. Cache *reads* are
captured and are priced at a tenth of input, which is the number that justifies
the cache existing.

Rates are per million tokens, US dollars, from the Anthropic pricing page.
Re-check them by reading that page and editing this file; there is no fetch,
deliberately -- a price this project quotes should change when a person decided
it should, not when a request happened to succeed.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import date
from typing import Any

#: When a human last read the pricing page. Rendered beside every figure this
#: module produces, and the only honest substitute for a freshness guarantee --
#: the same instrument ADR 19 gave the dossier.
CHECKED = date(2026, 8, 18)

#: Where to re-check. Not fetched; see the module docstring.
SOURCE = "https://platform.claude.com/docs/en/pricing"


@dataclass(frozen=True)
class Rate:
    """Dollars per million tokens, input and output."""

    input: float
    output: float

    #: Prompt cache reads, as a fraction of the input rate. One number rather
    #: than a third column because it is the same ratio across the family and
    #: a per-model copy of it would be three chances to mistype a tenth.
    CACHE_READ_FRACTION = 0.1


@dataclass(frozen=True)
class Priced:
    """One model's rate, and the date any of it is known to change."""

    rate: Rate
    #: A rate that takes over `after`. `None` when nothing is scheduled --
    #: which is most models, and is not a promise that nothing will change.
    then: Rate | None = None
    #: The last day `rate` applies. Required when `then` is set.
    until: date | None = None

    def __post_init__(self) -> None:
        if (self.then is None) != (self.until is None):
            raise ValueError(
                "a scheduled rate change needs both `then` and `until` -- one "
                "without the other prices half a changeover and silently "
                "keeps the wrong side of it")

    def on(self, when: date) -> Rate:
        """The rate in force on `when`."""
        if self.until is not None and self.then is not None and when > self.until:
            return self.then
        return self.rate


# Checked 2026-08-18 against the pricing page. Sonnet 5 is the one with a
# scheduled change, and it is the example the old no-price-table comment named:
# $2/$10 introductory through 2026-08-31, $3/$15 after. Modelling it is the
# point -- see the module docstring.
PRICES: dict[str, Priced] = {
    "claude-fable-5": Priced(Rate(input=10.00, output=50.00)),
    "claude-mythos-5": Priced(Rate(input=10.00, output=50.00)),
    "claude-opus-5": Priced(Rate(input=5.00, output=25.00)),
    "claude-opus-4-8": Priced(Rate(input=5.00, output=25.00)),
    "claude-opus-4-7": Priced(Rate(input=5.00, output=25.00)),
    "claude-opus-4-6": Priced(Rate(input=5.00, output=25.00)),
    "claude-sonnet-5": Priced(
        Rate(input=2.00, output=10.00),
        then=Rate(input=3.00, output=15.00),
        until=date(2026, 8, 31),
    ),
    "claude-sonnet-4-6": Priced(Rate(input=3.00, output=15.00)),
    "claude-haiku-4-5": Priced(Rate(input=1.00, output=5.00)),
}

_PER_MILLION = 1_000_000


@dataclass
class Estimate:
    """What a set of ledger rows came to, and what could not be priced.

    `unpriced_models` is not a footnote. A row whose model this table does not
    know contributes nothing to `usd`, so a caller that renders the figure
    without rendering this is showing a number that is wrong downward and
    reads as reassuring.
    """

    usd: float = 0.0
    #: Conversations whose model carried no rate.
    unpriced: int = 0
    #: Which ids those were, so somebody can go and add them.
    unpriced_models: set[str] = field(default_factory=set)

    @property
    def complete(self) -> bool:
        """Whether every row counted was priced."""
        return self.unpriced == 0

    def as_dict(self) -> dict[str, Any]:
        return {
            "usd": round(self.usd, 4),
            "unpriced": self.unpriced,
            "unpriced_models": sorted(self.unpriced_models),
            "complete": self.complete,
            "checked": CHECKED.isoformat(),
        }


def rate_for(model: str, when: date | None = None) -> Rate | None:
    """The rate for `model` on `when`, or `None` if this table has never
    heard of it. `when` defaults to today."""
    priced = PRICES.get(model)
    if priced is None:
        return None
    return priced.on(when or date.today())


def cost(*, model: str, input_tokens: int, output_tokens: int,
         cache_read_tokens: int = 0, when: date | None = None) -> float | None:
    """What one conversation cost, or `None` for a model with no rate.

    `None` rather than `0.0`, and the distinction is the whole reason this
    returns an optional: a conversation that cost nothing and a conversation
    nobody can price are different facts, and a caller that cannot tell them
    apart will report the second as the first.

    Cache reads are priced separately at a tenth of input. They are counted
    *beside* `input_tokens` and never inside it, because the API reports
    `input_tokens` as the uncached remainder only -- adding them would
    double-count the cheap half of every cached prompt.
    """
    rate = rate_for(model, when)
    if rate is None:
        return None
    return (
        input_tokens * rate.input
        + output_tokens * rate.output
        + cache_read_tokens * rate.input * Rate.CACHE_READ_FRACTION
    ) / _PER_MILLION


def estimate(rows: list[dict[str, Any]], *,
             when: date | None = None) -> Estimate:
    """Total `rows`, each carrying `model` and the three token counts.

    Rows are the ledger's own shape (`ledger.summary(by='model')` and its
    per-mode sibling), so this works over any grouping that kept the model.
    A row with no `model` key at all is unpriceable by definition and counts
    as one, rather than raising -- a roll-up that lost the axis is a caller
    bug, and the honest report of it is a figure that says it is incomplete.
    """
    out = Estimate()
    for row in rows:
        model = str(row.get("model") or "")
        conversations = int(row.get("conversations") or 1)
        amount = cost(
            model=model,
            input_tokens=int(row.get("input_tokens") or 0),
            output_tokens=int(row.get("output_tokens") or 0),
            cache_read_tokens=int(row.get("cache_read_tokens") or 0),
            when=when,
        )
        if amount is None:
            out.unpriced += conversations
            out.unpriced_models.add(model or "(none recorded)")
            continue
        out.usd += amount
    return out


def render(amount: float) -> str:
    """A dollar figure a person can read, down to the tenth of a cent.

    Sub-cent totals are the ordinary case on an instance this size, and
    `$0.00` beside a real conversation reads as "nothing happened" rather than
    as "not very much" — which is the wrong lesson to draw from a bill that is
    working. Four decimals until a cent, two after.
    """
    if amount and abs(amount) < 0.01:
        return f"${amount:.4f}"
    return f"${amount:,.2f}"
