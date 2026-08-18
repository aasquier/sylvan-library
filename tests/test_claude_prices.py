"""What the tokens cost: the table that reverses a written-down decision.

`mtglab claude usage` reported tokens and not dollars, deliberately, and said
why: *"prices move (Sonnet 5's introductory rate ends 2026-08-31) and a stale
hardcoded price table would turn an honest count into a wrong invoice."*

That objection is not withdrawn — it is the specification these tests check the
table against. Four properties, each answering one clause of it:

* **The rate that is known to move is modelled**, on both sides of the date the
  objection itself named. A table that flattened it would fail on its own
  headline example within a fortnight of being written.
* **An unknown model is counted, never priced at zero.** The ledger stores a
  served-by id, which can be a fallback or an id this build never heard of.
* **The figure is a floor**, because cache *writes* bill at 1.25x input and are
  recorded nowhere — so nothing here can see them.
* **Cache reads are priced beside the input tokens, never inside them.** The
  API reports `input_tokens` as the uncached remainder only.

No network: this file never fetches a price, and neither does the module.
"""

import sys
from datetime import date
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

from mtglab.claude import client, prices, tiers

BEFORE = date(2026, 8, 20)   # inside Sonnet 5's introductory window
AFTER = date(2026, 9, 20)    # after it lapses


# ----------------------------------------------------- the table's own shape

def test_every_tier_this_instance_can_grant_has_a_rate():
    """The table and the tier roster must not drift apart.

    A grant nobody can price is a seat whose spend silently vanishes from the
    total — the exact failure `unpriced` exists to make loud, arriving through
    the one door that should never produce it.
    """
    for tier in tiers.TIERS:
        assert prices.rate_for(tier.model) is not None, tier.model


def test_the_house_model_has_a_rate():
    assert prices.rate_for(client.MODEL) is not None


def test_a_scheduled_change_needs_both_halves():
    """Half a changeover prices the wrong side of it and says nothing."""
    with pytest.raises(ValueError):
        prices.Priced(prices.Rate(1.0, 2.0), then=prices.Rate(3.0, 4.0))
    with pytest.raises(ValueError):
        prices.Priced(prices.Rate(1.0, 2.0), until=date(2026, 8, 31))


# ------------------------------------------------------------ the changeover

def test_the_introductory_rate_applies_before_it_lapses():
    rate = prices.rate_for("claude-sonnet-5", BEFORE)
    assert (rate.input, rate.output) == (2.00, 10.00)


def test_the_list_rate_applies_after_it_lapses():
    """The assertion the old no-price-table comment is really about.

    2026-08-31 is the date that comment named. A table that answered $2/$10 on
    2026-09-20 would be the wrong invoice it warned of, produced by the very
    mechanism added to avoid one.
    """
    rate = prices.rate_for("claude-sonnet-5", AFTER)
    assert (rate.input, rate.output) == (3.00, 15.00)


def test_the_last_day_of_the_window_still_gets_the_old_rate():
    """`until` is inclusive — an off-by-one here overcharges a whole day."""
    rate = prices.rate_for("claude-sonnet-5", date(2026, 8, 31))
    assert rate.input == 2.00
    assert prices.rate_for("claude-sonnet-5", date(2026, 9, 1)).input == 3.00


def test_a_model_with_no_scheduled_change_prices_the_same_either_side():
    for when in (BEFORE, AFTER):
        assert prices.rate_for("claude-opus-5", when).input == 5.00


# ------------------------------------------------------------- the arithmetic

def test_a_conversation_costs_what_the_rates_say():
    # 1M in at $2 + 1M out at $10 = $12.00, on the introductory rate.
    assert prices.cost(model="claude-sonnet-5", input_tokens=1_000_000,
                       output_tokens=1_000_000, when=BEFORE) == pytest.approx(12.0)


def test_cache_reads_are_priced_beside_the_input_and_never_inside_it():
    """A tenth of input, and *additional* to it.

    The API reports `input_tokens` as the uncached remainder only, so a prompt's
    whole cost is the two added together. Folding cache reads into the input
    count would double-charge the cheap half of every cached prompt.
    """
    plain = prices.cost(model="claude-sonnet-5", input_tokens=1_000_000,
                        output_tokens=0, when=BEFORE)
    cached = prices.cost(model="claude-sonnet-5", input_tokens=1_000_000,
                         output_tokens=0, cache_read_tokens=1_000_000,
                         when=BEFORE)
    assert plain == pytest.approx(2.0)
    assert cached == pytest.approx(2.2)          # 2.00 + a tenth of 2.00


def test_a_dearer_tier_costs_more_for_the_same_work():
    """The grant has to mean something, arithmetically."""
    work = {"input_tokens": 100_000, "output_tokens": 100_000, "when": BEFORE}
    sonnet = prices.cost(model="claude-sonnet-5", **work)
    opus = prices.cost(model="claude-opus-5", **work)
    fable = prices.cost(model="claude-fable-5", **work)
    assert sonnet < opus < fable


# -------------------------------------------------------- what cannot be priced

def test_an_unknown_model_is_none_and_not_zero():
    """Two different facts, and a caller that cannot tell them apart will
    report the second as the first."""
    assert prices.rate_for("claude-archaeopteryx-9") is None
    assert prices.cost(model="claude-archaeopteryx-9", input_tokens=1_000_000,
                       output_tokens=1_000_000) is None


def test_an_unpriceable_row_is_counted_rather_than_silently_dropped():
    est = prices.estimate([
        {"model": "claude-sonnet-5", "conversations": 3,
         "input_tokens": 1_000_000, "output_tokens": 0, "cache_read_tokens": 0},
        {"model": "claude-archaeopteryx-9", "conversations": 5,
         "input_tokens": 9_000_000, "output_tokens": 9_000_000,
         "cache_read_tokens": 0},
    ], when=BEFORE)
    assert est.usd == pytest.approx(2.0)          # the priced row only
    assert est.unpriced == 5                      # conversations, not rows
    assert est.unpriced_models == {"claude-archaeopteryx-9"}
    assert not est.complete


def test_a_rollup_that_lost_the_model_axis_is_incomplete_rather_than_wrong():
    """A per-mode rollup carries `(various)` in its model column, because
    grouping by one axis aggregates the other. Pricing it would mean guessing a
    rate for rows that span models, and the guess would look like arithmetic —
    so it reports as unpriced instead."""
    est = prices.estimate([
        {"mode": "commander-dossier", "model": "(various)", "conversations": 4,
         "input_tokens": 1_000_000, "output_tokens": 0, "cache_read_tokens": 0},
    ], when=BEFORE)
    assert est.usd == 0.0
    assert est.unpriced == 4
    assert not est.complete


def test_an_empty_ledger_is_complete_and_costs_nothing():
    est = prices.estimate([])
    assert est.usd == 0.0 and est.complete


def test_the_estimate_carries_the_date_it_was_priced_at():
    """A figure whose age is invisible is the wrong invoice the old comment
    feared; one a reader can discount is not."""
    body = prices.estimate([]).as_dict()
    assert body["checked"] == prices.CHECKED.isoformat()
    assert body["complete"] is True


# ------------------------------------------------------------------ rendering

def test_a_sub_cent_total_is_not_rendered_as_nothing():
    """`$0.00` beside real conversations reads as "nothing happened" rather
    than "not very much", which is the wrong lesson from a working bill."""
    assert prices.render(0.0034) == "$0.0034"
    assert prices.render(0.0) == "$0.00"
    assert prices.render(1.628) == "$1.63"
    assert prices.render(12345.6) == "$12,345.60"
