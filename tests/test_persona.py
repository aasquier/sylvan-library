"""A persona is a voice, and these are the tests that keep it only a voice.

The risk this file exists for: a persona is a system prompt, and a system
prompt is the one place where "just add a sentence" can quietly undo a rule
that took an ADR to establish. So the checks below are mostly about what a
persona *cannot* do.
"""

from __future__ import annotations

import pytest

from mtglab.claude import persona as persona_mod
from mtglab.claude import theme


def test_an_absent_persona_is_the_plain_one():
    """Every client written before personas existed sends nothing."""
    assert persona_mod.get(None).key == persona_mod.DEFAULT
    assert persona_mod.get(None) is persona_mod.PLAIN


def test_an_unknown_persona_is_refused_and_says_what_there_is():
    with pytest.raises(persona_mod.UnknownPersona) as exc:
        persona_mod.get("necromancer")
    assert "fortune-teller" in str(exc.value)


@pytest.mark.parametrize("bad", [3, [], {}, True])
def test_a_persona_that_is_not_a_string_is_refused(bad):
    with pytest.raises(persona_mod.UnknownPersona):
        persona_mod.get(bad)


def test_the_plain_persona_leaves_the_existing_modes_untouched():
    """The strongest claim in this file: adding personas moved nothing.

    Byte-identical, not merely equivalent — the system block is what `converse`
    caches, so a stray newline here would be a silent cache miss on every
    conversation that never asked for a persona at all.
    """
    assert theme.CONVERSATION_MODES["plain"] is theme.THEME_CONVERSATION
    assert theme.PROPOSAL_MODES["plain"] is theme.THEME_PROPOSAL


def test_a_voice_is_appended_to_the_contract_never_substituted_for_it():
    """The rules that make the interview work are not a persona's to soften."""
    base = theme.THEME_CONVERSATION.instructions
    dressed = theme.CONVERSATION_MODES["fortune-teller"].instructions
    assert dressed.startswith(base), "the voice replaced the instructions"
    assert persona_mod.FORTUNE_TELLER.voice in dressed


@pytest.mark.parametrize("rule", [
    "Do not ask them about Magic",
    "Ask **one** question at a time",
    "carries a quote of their own words",
    "never tell them what you have concluded",
])
def test_every_persona_still_carries_the_interviews_rules(rule):
    for key, mode in theme.CONVERSATION_MODES.items():
        assert rule in mode.instructions, f"{key} lost: {rule}"


def test_no_persona_may_write_anything():
    """ADR 15, checked per persona rather than per mode."""
    for mode in (*theme.CONVERSATION_MODES.values(),
                 *theme.PROPOSAL_MODES.values()):
        assert mode.may_write == ()


def test_a_persona_does_not_widen_the_tool_set():
    """A voice may not reach further than the mode it is speaking for."""
    for key, mode in theme.CONVERSATION_MODES.items():
        assert mode.tool_names == theme.THEME_CONVERSATION.tool_names, key
    for key, mode in theme.PROPOSAL_MODES.items():
        assert mode.tool_names == theme.THEME_PROPOSAL.tool_names, key


def test_the_roster_never_ships_a_prompt():
    for row in persona_mod.as_dicts():
        assert set(row) == {"key", "label", "blurb", "deals"}


def test_only_the_fortune_teller_is_dealt_cards():
    deals = {p["key"] for p in persona_mod.as_dicts() if p["deals"]}
    assert deals == {"fortune-teller"}


def test_the_spread_reaches_the_model_as_a_message_not_the_prompt():
    """A cache decision, pinned.

    The instructions are byte-stable per persona so `converse` can cache them.
    A spread differs every reading, so it belongs after the breakpoint — if it
    ever drifts into the system prompt, every first turn becomes a full-price
    cache miss and nothing else would notice.
    """
    reading = tarot_reading()
    frame = theme._frame_for(reading)
    # The *positions* are static and naming them in the voice is fine — it is
    # how the reader knows what order to work in. The *cards* are what differ
    # every reading, and they are what must not be baked into a cached block.
    assert "The Root" in frame
    for card in (d.card.name for d in reading.cards):
        assert card in frame
        for key, mode in theme.CONVERSATION_MODES.items():
            assert card not in mode.instructions, f"{key} has {card} baked in"


def test_no_reading_means_the_frame_is_unchanged():
    assert theme._frame_for(None) == theme._FRAME


def test_a_plain_conversation_is_dealt_nothing_even_with_a_seed():
    """The seed is ignored rather than obeyed: only a reader reads cards."""
    assert theme._reading_for(persona_mod.PLAIN, 1909) is None
    assert theme._reading_for(persona_mod.FORTUNE_TELLER, 1909) is not None


def test_an_unusable_seed_is_refused_as_a_malformed_request():
    with pytest.raises(theme.TranscriptRejected):
        theme._reading_for(persona_mod.FORTUNE_TELLER, "the moon")


def tarot_reading():
    from mtglab import tarot
    return tarot.deal(1909)
