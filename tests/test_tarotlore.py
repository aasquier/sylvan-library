"""The tarot fact corpus, and the rule that the reader may not write one.

`tarotlore.py` is checked-in reference prose in the `colors.py`/`lore.py`
line, and it is held to the two things that make such a file safe: every
entry is attached to something real, and nothing generated can get in.

The load-bearing test here is `test_the_readers_own_words_are_discarded`.
The corpus is cited by **id**, and `theme.keep_fact` renders this file's text
rather than the model's -- so a reader that paraphrases, embellishes or
invents gets the true sentence anyway. That is the whole reason the mechanism
takes an id instead of a sentence, and it is worth a test that fails loudly if
anyone ever "simplifies" it back to trusting the payload.
"""

from __future__ import annotations

import pytest

from mtglab import tarot, tarotlore
from mtglab.claude import theme

VALID_CARD_KEYS = {c.key for c in tarot.DECK}


def test_every_fact_has_an_id_text_and_a_source() -> None:
    for fact in tarotlore.ALL:
        assert fact.id.strip(), fact
        assert fact.text.strip(), fact.id
        assert fact.source.strip(), fact.id


def test_ids_are_unique() -> None:
    """Ids are the wire format. A duplicate makes one fact unreachable."""
    ids = [f.id for f in tarotlore.ALL]
    assert len(ids) == len(set(ids)), \
        sorted(i for i in ids if ids.count(i) > 1)


def test_every_card_fact_names_a_card_in_the_deck() -> None:
    """A fact filed under a card that does not exist is never told.

    Silent, too: `for_card` simply returns nothing, so the failure looks like
    a card that happens to have no facts. Checked against the built deck
    rather than a hand-kept list of keys.
    """
    for fact in tarotlore.CARD_FACTS:
        assert fact.card in VALID_CARD_KEYS, f"{fact.id}: {fact.card!r}"


def test_deck_facts_belong_to_no_card() -> None:
    for fact in tarotlore.DECK_FACTS:
        assert fact.card == "", fact.id


def test_every_major_arcanum_has_facts() -> None:
    for key, name in tarot.MAJOR_ARCANA:
        assert tarotlore.for_card(key), f"{name} has nothing to say"


def test_every_minor_has_at_least_five(minimum: int = 5) -> None:
    """Aaron's number, 2026-08-18: at least five facts for every minor.

    Pinned rather than trusted because the corpus was written suit by suit
    and a card quietly left on four would be invisible — the reading would
    still work, it would just be thinner at that one card forever.
    """
    thin = {card.key: len(tarotlore.for_card(card.key))
            for card in tarot.DECK if card.arcana == "minor"
            and len(tarotlore.for_card(card.key)) < minimum}
    assert not thin, thin


def test_no_card_in_the_deck_is_silent() -> None:
    """All 78, including the Magic crossovers, which have no facts of their
    own and are not supposed to — they are real cards wearing a trump, and
    what they get is the deck tier plus whatever their trump carries."""
    natural = [c for c in tarot.DECK if c.arcana in ("major", "minor")
               and c.after is None]
    assert len(natural) == 78
    for card in natural:
        assert tarotlore.for_card(card.key), card.key


def test_no_fact_exceeds_the_wire_cap() -> None:
    """`theme.MAX_FACT_CHARS` bounds what a client may resend as told.

    A corpus entry longer than the cap would be told once and then rejected by
    `check_told` on the very next turn, which reads as the conversation
    breaking rather than as a fact being too long.
    """
    for fact in tarotlore.ALL:
        assert len(fact.text) <= theme.MAX_FACT_CHARS, fact.id


def test_every_card_in_the_deck_brings_the_whole_deck_tier() -> None:
    """Why the deck tier exists, stated as the invariant rather than a case.

    The sampler can deal any three of the 78, and a fortune-teller with
    nothing to say while the model thinks is the failure this corpus was
    written against. An earlier version of this test asserted that three
    named minors yielded *exactly* the deck tier — which quietly stopped
    testing anything the moment those minors got facts of their own. What
    must hold for ever is that no card can subtract from the deck tier.
    """
    deck_ids = {f.id for f in tarotlore.DECK_FACTS}
    assert deck_ids
    for card in tarot.DECK:
        offered = {f.id for f in tarotlore.for_reading([card.key])}
        assert deck_ids <= offered, card.key


def test_a_reading_offers_the_deck_and_its_own_cards() -> None:
    offered = tarotlore.for_reading(["00-fool", "16-tower"])
    ids = {f.id for f in offered}
    assert {f.id for f in tarotlore.DECK_FACTS} <= ids
    assert {f.id for f in tarotlore.for_card("00-fool")} <= ids
    assert {f.id for f in tarotlore.for_card("16-tower")} <= ids
    assert not any(f.card == "21-world" for f in offered)


def test_offer_drops_what_has_already_been_told() -> None:
    first = tarotlore.by_id("pixie-fee")
    assert first is not None
    text = tarotlore.offer(["00-fool"], told=(first.text,))
    assert "pixie-fee:" not in text
    assert "pixie-name:" in text


def test_offer_is_empty_rather_than_a_heading_over_nothing() -> None:
    everything = tuple(f.text for f in tarotlore.for_reading(["00-fool"]))
    assert tarotlore.offer(["00-fool"], told=everything) == ""


def test_by_id_is_forgiving_about_case_and_space() -> None:
    """`keep_fact` folds case on the prefix, so this must fold it on the id.
    A reader shouting `TAROT:PIXIE-FEE` would otherwise pass the prefix check
    and vanish into the dropped-fact counter."""
    assert tarotlore.by_id("PIXIE-FEE") is tarotlore.by_id("pixie-fee")
    assert tarotlore.by_id("  pixie-fee ") is tarotlore.by_id("pixie-fee")
    assert tarotlore.by_id("no-such-fact") is None


# ------------------------------------------------------- the boundary itself

def test_a_cited_fact_is_kept_and_carries_its_own_source() -> None:
    kept = theme.keep_fact(
        {"text": "roughly what the corpus says", "source": "tarot:pixie-fee"},
        searched=[])
    assert kept is not None
    entry = tarotlore.by_id("pixie-fee")
    assert entry is not None
    assert kept["text"] == entry.text
    assert kept["source"] == entry.source
    assert kept["url"] == ""


def test_the_readers_own_words_are_discarded() -> None:
    """The point of citing an id rather than a sentence.

    A reader that embellishes -- here, inventing a figure that is not in the
    corpus and would be untrue -- has the embellishment thrown away and the
    true sentence rendered in its place. If this ever fails because somebody
    made `keep_fact` trust the payload, the fortune-teller has become a
    surface that can state a falsehood as a fact, which is the one thing this
    table must not do.
    """
    kept = theme.keep_fact(
        {"text": "Pamela Colman Smith was paid exactly two pounds and "
                 "later won a lawsuit about it.",
         "source": "tarot:pixie-fee"},
        searched=[])
    assert kept is not None
    assert "lawsuit" not in kept["text"]
    assert "two pounds" not in kept["text"]
    entry = tarotlore.by_id("pixie-fee")
    assert entry is not None
    assert kept["text"] == entry.text


@pytest.mark.parametrize("source", [
    "tarot:no-such-fact", "tarot:", "tarot", "tarot:pixie fee",
])
def test_an_unresolvable_id_is_dropped(source: str) -> None:
    """Dropped and counted, never raised — one bad reference costs the fact
    and not the turn, which is how every citation check here behaves."""
    assert theme.keep_fact({"text": "anything at all", "source": source},
                           searched=[]) is None


def test_the_other_two_sources_still_work() -> None:
    """The corpus is a third origin, not a replacement.

    `taxonomy` still trusts the model's own sentence (the colour data is small
    and sits in the prompt), and a real URL is still checked against what the
    search returned.
    """
    taxonomy = theme.keep_fact(
        {"text": "Selesnya is white-green.", "source": "taxonomy"},
        searched=[])
    assert taxonomy is not None and taxonomy["source"] == "taxonomy"

    page = theme.keep_fact(
        {"text": "something a page said.", "source": "https://example.test/a"},
        searched=[{"url": "https://example.test/a", "title": "A page"}])
    assert page is not None and page["url"] == "https://example.test/a"


def test_the_spread_puts_its_own_cards_in_the_frame() -> None:
    """End to end: deal, build the frame, and find the dealt cards' facts.

    Seeded, so this is the same three cards every run — and the deck tier is
    asserted separately because it is there whatever lands.
    """
    reading = tarot.deal(seed=7)
    frame = theme._frame_for(reading, ())
    assert "pixie-fee:" in frame
    for drawn in reading.cards:
        for fact in tarotlore.for_card(drawn.card.key):
            assert f"{fact.id}:" in frame


def test_the_proposal_frame_carries_no_facts() -> None:
    """The proposal schema has no `fact` field, so offering it the corpus
    would be seven kilobytes of prompt with nowhere to go.

    Asserted on the built frame rather than on the call site, because what
    matters is what reaches the model. `told=None` is the opt-out and an empty
    tuple is a real first turn — a distinction worth pinning, since collapsing
    the two is the obvious "simplification".
    """
    reading = tarot.deal(seed=7)
    assert tarotlore.offer([d.card.key for d in reading.cards]) not in ("",)
    assert "pixie-fee:" not in theme._frame_for(reading)
    assert "pixie-fee:" in theme._frame_for(reading, ())
    assert reading.describe() in theme._frame_for(reading)
