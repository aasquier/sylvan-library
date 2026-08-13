"""The deck and the deal — the half of a reading that has right answers.

No corpus, no network, no key. `tarot.py` is stdlib and stays that way, which
is what lets these run everywhere the mana solver does.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from mtglab import tarot
from mtglab.claude import theme

ASSETS = Path(tarot.__file__).parent / "assets" / "tarot"


def test_the_deck_is_seventy_eight_cards():
    assert len(tarot.DECK) == 78
    assert len({c.key for c in tarot.DECK}) == 78, "two cards share a key"
    assert len({c.name for c in tarot.DECK}) == 78, "two cards share a name"


def test_twenty_two_trumps_and_four_suits_of_fourteen():
    majors = [c for c in tarot.DECK if c.arcana == "major"]
    assert len(majors) == 22
    assert [c.number for c in majors] == list(range(22))
    for suit in tarot.SUITS:
        ranks = sorted(c.number for c in tarot.DECK if c.suit == suit)
        assert ranks == list(range(1, 15)), f"{suit} is not 1-14"


def test_strength_is_eight_and_justice_is_eleven():
    """Waite's swap, not a typo. A deck that 'fixed' it would be a different one."""
    assert tarot.BY_KEY["08-strength"].name == "Strength"
    assert tarot.BY_KEY["11-justice"].name == "Justice"


def test_every_card_has_its_picture():
    """The failure this catches is a renamed card that deals as a broken image.

    Checked against the files rather than a list, because the list would be the
    same mistake written twice.
    """
    missing = [c.key for c in tarot.DECK
               if not (ASSETS / f"{c.key}.webp").is_file()]
    assert not missing, f"cards with no picture: {missing}"


def test_no_picture_belongs_to_no_card():
    """The other direction: an orphan file is a rename half-done."""
    keys = {c.key for c in tarot.DECK}
    orphans = sorted(p.stem for p in ASSETS.glob("*.webp") if p.stem not in keys)
    assert not orphans, f"pictures with no card: {orphans}"


def test_the_spread_is_the_first_three_slot_kinds():
    """The load-bearing coupling, pinned.

    ADR 20's readiness counts grounded slots and `FLOOR` is three. The tarot
    door reuses that instrument unchanged only because a card is dealt *for* a
    slot — so if these ever drift apart, a reading stops being able to reach
    the floor and the proposal button never lights up.
    """
    assert [p.slot for p in tarot.SPREAD] == list(theme.SLOT_KINDS[:3])
    assert len(tarot.SPREAD) == theme.FLOOR


def test_a_seed_deals_the_same_three_cards():
    a, b = tarot.deal(1909), tarot.deal(1909)
    assert a.as_dict() == b.as_dict()
    assert a.seed == 1909


def test_an_unseeded_deal_reports_the_seed_it_used():
    """So a reload can ask for the same reading back."""
    r = tarot.deal()
    assert isinstance(r.seed, int)
    assert tarot.deal(r.seed).as_dict() == r.as_dict()


def test_a_deal_is_three_distinct_cards_in_spread_order():
    r = tarot.deal(7)
    assert len(r.cards) == 3
    assert len({d.card.key for d in r.cards}) == 3, "the same card dealt twice"
    assert [d.position for d in r.cards] == list(tarot.SPREAD)


@pytest.mark.parametrize("seed", range(40))
def test_reversals_are_not_stuck(seed):
    """Both ways up occur. A deck that never reverses is a bug you cannot see."""
    assert all(isinstance(d.reversed, bool) for d in tarot.deal(seed).cards)


def test_reversals_happen_both_ways_across_many_deals():
    seen = {d.reversed for s in range(40) for d in tarot.deal(s).cards}
    assert seen == {True, False}


def test_describe_names_every_position_and_card():
    r = tarot.deal(1909)
    text = r.describe()
    for d in r.cards:
        assert d.position.name in text
        assert d.card.name in text
    assert "reversed" in text or not any(d.reversed for d in r.cards)


def test_image_paths_are_served_from_the_tarot_mount():
    assert tarot.BY_KEY["17-star"].image == "/tarot/17-star.webp"
