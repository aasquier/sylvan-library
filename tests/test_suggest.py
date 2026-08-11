"""The replacement scorer.

`suggest.py` ranks cards by measurable similarity to the one being removed. It
does not decide anything -- ADR 8 keeps that with the deck's owner -- so what
these tests pin is that the measurements are the measurements: the type test
reads the front face, the curve term actually falls off with distance, and the
ranking is stable enough to trust twice.

Scoring is pure over `CardRecord`s, so none of this needs a database.
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.cards.db import CardRecord
from mtglab.decks import suggest


def card(name, *, cost="{4}{G}{G}", cmc=6.0, type_line="Creature — Beast",
         text="", keywords=(), rank=None, identity=("G",)) -> CardRecord:
    return CardRecord(
        name=name, mana_cost=cost, cmc=cmc, type_line=type_line,
        oracle_text=text, color_identity=frozenset(identity),
        produced_mana=(), legal_commander=True, reserved=False,
        edhrec_rank=rank, image_normal=None, keywords=tuple(keywords),
    )


TITAN = card(
    "Primeval Titan",
    text="Trample. Whenever this creature enters or attacks, search your library "
         "for up to two land cards, put them onto the battlefield tapped, then "
         "shuffle.",
    keywords=("Trample",),
)


# ------------------------------------------------------------ primary type

def test_primary_type_prefers_the_most_specific():
    assert suggest.primary_type("Legendary Artifact Creature — Golem") == "Creature"
    assert suggest.primary_type("Legendary Enchantment — Background") == "Enchantment"
    assert suggest.primary_type("Instant") == "Instant"


def test_primary_type_reads_the_front_face_only():
    """The same trap `CardRecord.is_land` had: Scryfall's type line names both
    faces, so matching on the word 'Land' anywhere suggests a spell as a land."""
    assert suggest.primary_type("Creature — Elf // Land") == "Creature"
    assert suggest.primary_type("Land // Creature — Elf") == "Land"


def test_primary_type_of_nothing_is_empty():
    assert suggest.primary_type("") == ""


# ------------------------------------------------------------- the components

def test_same_type_beats_permanent_beats_unrelated():
    creature = card("A", type_line="Creature — Beast")
    artifact = card("B", type_line="Artifact")
    instant = card("C", type_line="Instant")
    assert suggest._type_score(TITAN, creature) == 1.0
    assert suggest._type_score(TITAN, artifact) == 0.4
    assert suggest._type_score(TITAN, instant) == 0.0


def test_curve_score_falls_off_with_distance():
    assert suggest._curve_score(TITAN, card("same", cmc=6.0)) == 1.0
    assert suggest._curve_score(TITAN, card("near", cmc=7.0)) > 0.6
    assert suggest._curve_score(TITAN, card("far", cmc=9.0)) == 0.0
    assert suggest._curve_score(TITAN, card("cheap", cmc=1.0)) == 0.0


def test_keyword_score_is_an_overlap_not_a_contains():
    both = card("both", keywords=("Trample",))
    extra = card("extra", keywords=("Trample", "Flying", "Haste"))
    none = card("none", keywords=("Flying",))
    assert suggest._keyword_score(TITAN, both) == 1.0
    # Sharing trample while carrying two unrelated keywords is a weaker match
    # than sharing trample and nothing else.
    assert 0 < suggest._keyword_score(TITAN, extra) < 1.0
    assert suggest._keyword_score(TITAN, none) == 0.0


def test_keyword_score_is_neutral_when_the_target_has_none():
    plain = card("plain", text="Draw a card.")
    assert suggest._keyword_score(plain, card("x", keywords=("Flying",))) == 0.0


def test_text_score_measures_the_targets_vocabulary_not_the_candidates():
    """Asymmetric on purpose: the question is how much of what the old card did
    the new one also does, so a wordy card is not rewarded for being wordy."""
    tokens = suggest._tokens(TITAN.oracle_text)
    covers = card("covers", text="Search your library for two land cards and put "
                                 "them onto the battlefield, then shuffle.")
    incidental = card("incidental", text="Draw three cards.")
    disjoint = card("disjoint", text="Counter target spell unless its controller "
                                     "pays {3}.")
    assert suggest._text_score(tokens, covers) > 0.5
    # Not zero, and pretending otherwise would be wrong: "cards" is genuinely a
    # word both cards use. Magic's vocabulary is small, so incidental overlap is
    # the floor rather than the exception -- what matters is that it stays an
    # order of magnitude below a real match.
    assert 0 < suggest._text_score(tokens, incidental) < 0.15
    assert suggest._text_score(tokens, disjoint) == 0.0
    assert suggest._text_score(set(), covers) == 0.0


def test_popularity_rewards_a_better_rank_and_ignores_none():
    unranked = suggest._popularity(card("unranked", rank=None))
    staple = suggest._popularity(card("staple", rank=50))
    obscure = suggest._popularity(card("obscure", rank=20_000))
    assert unranked == 0.0
    assert staple > obscure >= 0.0
    assert staple <= 1.0


def test_a_banned_card_is_itself_unranked():
    """Not a quirk to work around: EDHREC does not rank what nobody may play,
    which is a useful reminder that popularity is not quality."""
    assert suggest._popularity(TITAN) == 0.0


# ------------------------------------------------------------------ ranking

def test_the_deck_rationale_feeds_the_text_comparison():
    """`why` says what the slot was for, which oracle text often does not --
    'ramp and threat in one card' appears on no card in Magic."""
    ramp = card("Ramper", text="When this creature enters, ramp your mana.")
    plain = card("Vanilla", text="Trample.")
    with_why = suggest.score(TITAN, ramp, why="Ramp and threat in one card.")
    without = suggest.score(TITAN, ramp)
    assert with_why.score > without.score
    assert suggest.score(TITAN, plain, why="Ramp and threat.").score < with_why.score


def test_rank_never_suggests_the_card_being_replaced():
    pool = [TITAN, card("Other", text="Search your library for a land card.")]
    names = [c.name for c in suggest.rank(TITAN, pool)]
    assert "Primeval Titan" not in names


def test_rank_honours_the_exclusion_list():
    """Suggesting a card the deck already runs is worse than suggesting
    nothing: it looks like a recommendation and is a no-op."""
    pool = [card("Already In Deck"), card("Fresh Face")]
    names = [c.name for c in
             suggest.rank(TITAN, pool, exclude=frozenset({"already in deck"}))]
    assert names == ["Fresh Face"]


def test_rank_respects_the_limit_and_orders_by_score():
    pool = [
        card("Close", text="Search your library for two land cards.",
             keywords=("Trample",)),
        card("Middling", type_line="Artifact", cmc=5.0),
        card("Distant", type_line="Instant", cmc=1.0, text="Counter target spell."),
    ]
    ranked = suggest.rank(TITAN, pool, limit=2)
    assert [c.name for c in ranked] == ["Close", "Middling"]
    assert ranked[0].score > ranked[1].score


def test_ties_break_by_name_so_the_shortlist_is_stable():
    """A list that reshuffles between two identical runs is one nobody trusts,
    and it would make the CLI output impossible to diff."""
    pool = [card("Zebra"), card("Antelope"), card("Mongoose")]
    first = [c.name for c in suggest.rank(TITAN, pool)]
    second = [c.name for c in suggest.rank(TITAN, list(reversed(pool)))]
    assert first == second == ["Antelope", "Mongoose", "Zebra"]


def test_every_candidate_explains_itself():
    """A score with no reason is a number to argue with rather than act on."""
    match = card("Twin", text="Search your library for two land cards.",
                 keywords=("Trample",), rank=300)
    result = suggest.score(TITAN, match)
    joined = " | ".join(result.reasons)
    assert "same card type (Creature)" in joined
    assert "same mana value (6)" in joined
    assert "shares trample" in joined
    assert "text:" in joined
    assert "EDHREC rank 300" in joined


def test_scores_stay_inside_their_documented_range():
    perfect = suggest.score(TITAN, TITAN)
    assert 0.0 <= perfect.score <= 1.10


if __name__ == "__main__":
    import pytest
    sys.exit(pytest.main([__file__, "-q"]))
