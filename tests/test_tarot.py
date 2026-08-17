"""The deck and the deal — the half of a reading that has right answers.

No card pool, no network, no key. `tarot.py` is stdlib and stays that way, which
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


# ------------------------------------------------- the Magic crossovers

def test_the_magic_tiers_join_the_shuffle_but_not_the_seventy_eight():
    """Punch list 2026-08-15 item 13, widened twice on 2026-08-16 and again
    at Aaron's verdicts on 2026-08-17: three printed tarot cards and
    thirty-eight echoes. The 78 stay the 78 -- the picture sweeps above run
    over `DECK` on purpose, because a Magic card's picture is a hotlinked
    art crop and not package data."""
    assert len(tarot.DECK) == 78
    assert len(tarot.CROSSOVERS) == 3
    assert len(tarot.ECHOES) == 38
    assert len(tarot.FULL_DECK) == 119
    for card in tarot.CROSSOVERS + tarot.ECHOES:
        assert card.after, f"{card.name} names no trump"
        assert card.artist, f"{card.name} credits nobody"
        assert card.art_url and card.art_url.startswith(
            "https://cards.scryfall.io/art_crop/"), card.name
        assert card.image == card.art_url
    assert all(not c.echo for c in tarot.CROSSOVERS)
    assert all(c.echo for c in tarot.ECHOES)


def test_every_trump_is_answered_by_a_magic_card():
    """Aaron, 2026-08-16: all the trump cards at least. The deep dives are
    finished only when every one of the 22 has its Magic answer, and this
    is the sentence that keeps a future trim from quietly unfinishing
    them."""
    majors = {c.number for c in tarot.CROSSOVERS + tarot.ECHOES
              if c.arcana == "major"}
    assert majors == set(range(22))


def test_every_magic_card_justifies_its_slot():
    """Rule 4's discipline applied to this deck's own slots: every Magic
    card carries a note stating its resonance in checkable facts, or it
    should not be in the deck."""
    for card in tarot.CROSSOVERS + tarot.ECHOES:
        assert card.note and len(card.note) > 40, \
            f"{card.name} cannot justify its slot"


def test_magic_cards_sit_at_their_originals_numbers():
    by_name = {c.name: c for c in tarot.CROSSOVERS + tarot.ECHOES}
    assert by_name["Flubs, the Fool"].number == 0
    assert by_name["Massimo, the Magician"].number == 1
    assert by_name["Willow Priestess"].number == 2
    assert by_name["Empress Galina"].number == 3
    assert by_name["Emperor Apatzec Intli IV"].number == 4
    assert by_name["Orzhov Pontiff"].number == 5
    assert by_name["True Love's Kiss"].number == 6
    assert by_name["Esika's Chariot"].number == 7
    assert by_name["Lion Umbra"].number == 8
    assert by_name["Homer, the Hermit"].number == 9
    assert by_name["Wheel of Fortune"].number == 10
    assert by_name["Balance"].number == 11
    assert by_name["Suspension Field"].number == 12
    assert by_name["Murderous Rider // Swift End"].number == 13
    assert by_name["Chalice of Life // Chalice of Death"].number == 14
    assert by_name["Asmodeus the Archfiend"].number == 15
    assert by_name["Command Tower"].number == 16
    assert by_name["Ephara, God of the Polis"].number == 17
    assert by_name["Imprisoned in the Moon"].number == 18
    assert by_name["Approach of the Second Sun"].number == 19
    assert by_name["Angelic Renewal"].number == 20
    assert by_name["The World Tree"].number == 21
    # The minors carry their originals' rank and suit the same way.
    assert (by_name["Wand of the Worldsoul"].suit,
            by_name["Wand of the Worldsoul"].number) == ("wands", 1)
    assert (by_name["Expedition Map"].suit,
            by_name["Expedition Map"].number) == ("wands", 2)
    assert (by_name["Goblin Gathering"].suit,
            by_name["Goblin Gathering"].number) == ("wands", 5)
    assert (by_name["Young Pyromancer"].suit,
            by_name["Young Pyromancer"].number) == ("wands", 11)
    assert (by_name["Hellrider"].suit,
            by_name["Hellrider"].number) == ("wands", 12)
    assert (by_name["Everflowing Chalice"].suit,
            by_name["Everflowing Chalice"].number) == ("cups", 1)
    assert (by_name["Rite of Harmony"].suit,
            by_name["Rite of Harmony"].number) == ("cups", 3)
    assert (by_name["Happily Ever After"].suit,
            by_name["Happily Ever After"].number) == ("cups", 10)
    assert (by_name["Tragic Poet"].suit,
            by_name["Tragic Poet"].number) == ("cups", 11)
    assert (by_name["Thassa, God of the Sea"].suit,
            by_name["Thassa, God of the Sea"].number) == ("cups", 13)
    assert (by_name["Sword of Truth and Justice"].suit,
            by_name["Sword of Truth and Justice"].number) == ("swords", 1)
    assert (by_name["Curse of the Pierced Heart"].suit,
            by_name["Curse of the Pierced Heart"].number) == ("swords", 3)
    assert (by_name["Startled Awake // Persistent Nightmare"].suit,
            by_name["Startled Awake // Persistent Nightmare"].number) == (
                "swords", 9)
    assert (by_name["Murder"].suit,
            by_name["Murder"].number) == ("swords", 10)
    assert (by_name["King Macar, the Gold-Cursed"].suit,
            by_name["King Macar, the Gold-Cursed"].number) == ("pentacles", 14)


def test_a_double_faced_echo_is_captioned_by_its_front_face():
    """Three echoes are DFCs, and the pool's name for the whole card is the
    right thing in the reader's prompt and the wrong thing hand-set in 12px
    small caps under a 1909 plate. `face_name` is the split, and it is on
    the wire because the CLI and the table must caption a card the same
    way -- a second `split(' // ')` in TypeScript would be the same
    decision made twice, and would disagree the day one of them changed."""
    by_name = {c.name: c for c in tarot.ECHOES}
    assert by_name["Murderous Rider // Swift End"].face_name == (
        "Murderous Rider")
    assert by_name["Chalice of Life // Chalice of Death"].face_name == (
        "Chalice of Life")
    assert by_name["Startled Awake // Persistent Nightmare"].face_name == (
        "Startled Awake")
    # A single-faced card is its own caption, which is what lets the client
    # render `face_name` unconditionally.
    for card in tarot.FULL_DECK:
        assert card.as_dict()["face_name"] == card.name.split(" // ")[0]
    assert tarot.BY_KEY["13-death"].face_name == "Death"


def test_describe_keeps_the_whole_card_name():
    """The other half of the same decision: `describe` names the card the
    pool names, because on a DFC the second half is half the resonance --
    Swift End is why Murderous Rider answers to Death at all."""
    seed = next(s for s in range(4000)
                if any(d.card.name.startswith("Murderous Rider")
                       for d in tarot.deal(s).cards))
    assert "Murderous Rider // Swift End" in tarot.deal(seed).describe()


def test_one_echo_per_original_at_most():
    """Two echoes on one original could land together and read as a
    misprint rather than an alignment -- the doubles paragraph is written
    for the natural-plus-Magic pair, and this is what keeps that the only
    kind."""
    slots = [(c.arcana, c.suit, c.number) for c in tarot.ECHOES]
    assert len(slots) == len(set(slots))


def test_a_crossover_deal_replays_from_its_seed():
    """The weighted sampler is as deterministic as the plain one was."""
    seed = next(s for s in range(500)
                if any(d.card.after for d in tarot.deal(s).cards))
    assert tarot.deal(seed).as_dict() == tarot.deal(seed).as_dict()


def test_magic_cards_land_about_every_third_reading():
    """The weight is the feature: a wink nobody ever sees is worthless, and
    Aaron asked for a Magic card landing to be a big deal -- which needs it
    frequent enough to happen and rare enough to stay an event.

    Fully deterministic -- the seeds are fixed, so this rate is a constant.
    The arithmetic says ~35% of three-card deals hold at least one of the
    thirty-two (mass 11.75 against 78 -- the tier widened, so each member's
    weight came down), and the band holds both 'a real presence' and 'not
    the norm'.
    """
    deals = [tarot.deal(s) for s in range(1500)]
    with_magic = sum(
        1 for r in deals if any(d.card.after for d in r.cards))
    rate = with_magic / len(deals)
    assert 0.27 < rate < 0.44, rate


def test_describe_tells_the_reader_what_kind_of_magic_card_landed():
    """The reader interprets; the deal states facts. A printed tarot card
    says which trump it is printed after; an echo says its art and name
    answer to one -- a design fact and an editorial one, kept distinct."""
    literal_seed = next(
        s for s in range(2000)
        if any(d.card.after and not d.card.echo
               for d in tarot.deal(s).cards))
    assert "a Magic card printed after" in tarot.deal(literal_seed).describe()

    echo_seed = next(
        s for s in range(2000)
        if any(d.card.echo for d in tarot.deal(s).cards))
    assert "answer to" in tarot.deal(echo_seed).describe()


def test_a_magic_card_is_called_an_omen_with_precedence():
    """Aaron, 2026-08-16: a Magic card landing is a big deal, and the
    instruction to treat it as one is Python's to give -- a prompt hope
    with no sentence behind it is the fact-repeat bug wearing a shawl."""
    seed = next(s for s in range(500)
                if any(d.card.after for d in tarot.deal(s).cards))
    text = tarot.deal(seed).describe()
    assert "Give it precedence" in text

    quiet = next(s for s in range(500)
                 if not any(d.card.after for d in tarot.deal(s).cards))
    assert "precedence" not in tarot.deal(quiet).describe()


def test_a_trump_landing_twice_is_named_as_an_alignment():
    """The stars-aligning case: the sampler draws without replacement over
    all 88, so the 1909 Fool and Flubs really can share a spread, and when
    they do the reader is told outright rather than left to notice."""
    seed = next(
        (s for s in range(20000) if (lambda r: len({
            d.card.number for d in r.cards if d.card.arcana == "major"
        }) < sum(1 for d in r.cards if d.card.arcana == "major"))(
            tarot.deal(s))),
        None)
    assert seed is not None, "no aligned spread in 20000 seeds"
    text = tarot.deal(seed).describe()
    assert "The stars have aligned" in text
