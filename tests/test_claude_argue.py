"""The slot argument: everything about it that does not need a model.

No network, no key, no tokens. The one call this feature makes is deliberately
not made here, for the same reason the interview's is not.

What is here is the part that has to hold whatever the model says, and this
mode needs more of it than the interview did. The interview's guard is that
every item must be a question, which is cheap to check because the failure --
a declarative sentence -- is the opposite of the format. This mode's items are
*all* declarative, so the guard moved: the schema has no field for a reason to
keep the card, every charge must cite a fact or be dropped, and every
alternative is judged by deterministic Python against the pool, the ban list
and the deck's colour identity rather than by the model that named it.

That last one is the CLAUDE.md cautionary tale made executable. *Ajani, Nacatl
Pariah* is in `tiny_pool` precisely because its front face is white and its
back face is red, so a model proposing it for a green deck is the error this
project actually made. Here it is proposed, and dropped.

The boundary that outranks all of it -- nothing under `src/mtglab/claude/` may
name a write function -- lives in `test_claude_boundary.py` and picked this
module up automatically the moment it existed.
"""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import tiny_pool
from mtglab import config
from mtglab.claude import argue, client, interview, modes, stance, tools
from mtglab.decks.source import MemoryDeckSource


@pytest.fixture
def pool(tmp_path):
    """A real DuckDB pool with 21 real cards, built in about a second.

    Needed because `resolve_alternatives` is not a string filter -- it asks the
    pool what each named card actually is, which is the whole point of it.
    """
    with config.use_paths(data_dir=tmp_path / "data"):
        yield tiny_pool.build(config.DB_PATH)


@pytest.fixture
def source():
    """Mono-green, Goreclaw at the helm, running one banned card."""
    return MemoryDeckSource([tiny_pool.mono_green_deck()])


@pytest.fixture
def no_network(monkeypatch):
    """Make any attempt to reach the API a test failure rather than a bill."""
    def refuse():
        raise AssertionError("a Claude call was made; this test forbids one")
    monkeypatch.setattr(client, "connect", refuse)


def turn_returning(payload_json: str, **kw) -> modes.Turn:
    return modes.Turn(
        mode=argue.SLOT_ARGUMENT.name,
        model="claude-sonnet-5-test",
        stop_reason=kw.pop("stop_reason", "end_turn"),
        text=payload_json, tool_calls=kw.pop("tool_calls", []),
        input_tokens=100, output_tokens=50, **kw)


# --------------------------------------------------------------- the mode

def test_the_mode_declares_no_write_capability():
    """ADR 15's table, as an assertion rather than a document."""
    assert argue.SLOT_ARGUMENT.may_write == ()


def test_the_tool_set_is_inside_the_read_only_registry():
    assert set(argue.SLOT_ARGUMENT.tool_names) <= set(tools.READ_ONLY)


def test_the_tool_set_is_adr_15s_row_for_this_mode():
    """get_cards, search_cards, suggest, analyze, validate -- and no deck read.

    `get_deck` is absent deliberately: `brief()` already hands over the deck,
    the category and the siblings' rationales, so the only thing a deck read
    would add is the other 88 cards' prose. Pinned because adding it later
    should be a decision somebody argues, not a convenience somebody reaches
    for while debugging.
    """
    assert set(argue.SLOT_ARGUMENT.tool_names) == {
        "get_cards", "search_cards", "suggest_replacements",
        "deck_stats", "validate_deck"}


def test_this_mode_may_shop_where_the_interview_may_not():
    """The one real difference in the two per-card modes' capabilities.

    An interview that offered a replacement would have stopped interviewing;
    an argument that could not name one would be making a case with no
    alternative, which is half an argument.
    """
    argue_tools = set(argue.SLOT_ARGUMENT.tool_names)
    interview_tools = set(interview.RATIONALE_INTERVIEW.tool_names)
    assert {"search_cards", "suggest_replacements"} <= argue_tools
    assert not {"search_cards", "suggest_replacements"} & interview_tools


def test_the_scope_axis_changes_the_prompt_and_the_tool_set_never_moves():
    """A stance may widen what a mode does, never what it may do (ADR 15)."""
    mode = argue.SLOT_ARGUMENT
    prompts = {s: mode.system(stance.Stance("on-request", s, "none"))
               for s in stance.SCOPE}
    assert len(set(prompts.values())) == 3, "scope must change the prompt"
    for text in prompts.values():
        assert mode.instructions in text


# ------------------------------------------------------------- the schema

#: Words a field holding the case *for* the card would plausibly be called.
#: Checked by name because the guard is that no such field exists, and a guard
#: that only knew one spelling of the thing it forbids is not much of one.
IN_FAVOUR = ("defence", "defense", "in_favour", "in_favor", "keep", "verdict",
             "rationale", "why", "justification", "summary", "counterpoint",
             "strengths", "recommendation", "draft", "suggestion")


def test_the_schema_has_nowhere_to_put_a_case_for_the_card():
    """The structural half of ADR 25.

    The one-direction rule is not a tone the prompt asks for. A model that
    wanted to explain why this card earns its slot has no field to put it in,
    and `additionalProperties: false` means it cannot add one at run time.
    """
    schema = argue.RESPONSE_SCHEMA
    assert schema["additionalProperties"] is False
    assert set(schema["properties"]) == {"charges", "alternatives"}

    item = schema["properties"]["charges"]["items"]
    assert item["additionalProperties"] is False
    for banned in IN_FAVOUR:
        assert banned not in item["properties"], f"charges may not carry {banned}"


def test_an_alternative_is_a_bare_name_with_no_room_for_prose():
    """A replacement *with a reason attached* is a rationale in waiting.

    So the alternatives array holds strings. Where a deterministic scorer
    exists, `suggest_replacements` already returns why each candidate scored
    well; where one does not, the card's own oracle text renders beside it.
    Either way the sentence is not the model's to write.
    """
    items = argue.RESPONSE_SCHEMA["properties"]["alternatives"]["items"]
    assert items["type"] == "string"
    assert "properties" not in items


def test_every_charge_must_carry_a_citation():
    assert set(argue.RESPONSE_SCHEMA["properties"]["charges"]["items"]
               ["required"]) == {"claim", "ground", "fact", "strength"}


# ------------------------------------------------------- reading the answer

def test_a_charge_with_no_fact_is_dropped_and_counted():
    """`only_charges` is this mode's `only_questions`, and the predicate moved.

    There the failure is a sentence that is not a question. Here every item is
    a declarative sentence by design, so the thing that separates an argument
    from an opinion is whether it cites anything.
    """
    kept, dropped = argue.only_charges([
        {"claim": "Nine cards already sit at three mana.", "ground": "count",
         "fact": "curve: 9 cards at MV 3", "strength": "serious"},
        {"claim": "It just feels win-more.", "ground": "ceiling",
         "fact": "   ", "strength": "decisive"},
        {"claim": "", "ground": "cost", "fact": "MV 7", "strength": "minor"},
        "not a dict at all",
    ])
    assert len(kept) == 1
    assert dropped == 3
    assert kept[0]["fact"] == "curve: 9 cards at MV 3"


def test_an_unrecognised_ground_or_strength_is_relabelled_not_dropped():
    """A labelling miss should not cost a cited argument.

    The enum exists so a UI can group the case. Throwing away a charge that
    cited a real fact because its tag was spelled oddly would protect nothing
    and lose the useful half.
    """
    kept, dropped = argue.only_charges([
        {"claim": "Too slow.", "ground": "vibes", "fact": "MV 7",
         "strength": "catastrophic"},
    ])
    assert dropped == 0
    assert kept[0]["ground"] in argue.GROUNDS
    assert kept[0]["strength"] in argue.STRENGTHS


# ------------------------------------------------------- the alternatives

def test_an_off_colour_alternative_is_dropped(pool):
    """The CLAUDE.md error, executable.

    *Ajani, Nacatl Pariah* is {1}{W} on its front face and red on its back, so
    its colour identity is {R}{W}. Proposed for a mono-green deck it is
    illegal, and a model reasoning from the mana cost would not know that.
    Python reads the pool's own `color_identity` field and drops it -- rule 2,
    enforced rather than requested.

    Note which name comes back. Asked by its front face, the card is reported
    under the pool's full `A // B` spelling, because *the back face is the
    whole point* -- "Ajani, Nacatl Pariah is off-colour in green" reads as a
    mistake until you can see the Avenger half.
    """
    kept, dropped = argue.resolve_alternatives(
        ["Ajani, Nacatl Pariah", "Craterhoof Behemoth"], identity=["G"])
    assert [c["name"] for c in kept] == ["Craterhoof Behemoth"]
    assert dropped["off_colour"] == ["Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"]


def test_a_partly_overlapping_identity_is_still_off_colour(pool):
    """Etali is {G}{R}. Green is not enough -- identity is a subset test."""
    kept, dropped = argue.resolve_alternatives(
        ["Etali, Primal Conqueror"], identity=["G"])
    assert kept == []
    assert dropped["off_colour"] == [
        "Etali, Primal Conqueror // Etali, Primal Sickness"]


def test_a_colourless_alternative_survives_any_identity(pool):
    kept, _ = argue.resolve_alternatives(["Sol Ring"], identity=["G"])
    assert [c["name"] for c in kept] == ["Sol Ring"]


def test_a_banned_alternative_is_dropped(pool):
    """The gate's own answer, applied before anybody sees the suggestion.

    A mode arguing that a card should be cut, and offering a banned card as
    the replacement, would be the tool undoing its own gate.
    """
    kept, dropped = argue.resolve_alternatives(
        ["Primeval Titan", "Emrakul, the Aeons Torn", "Terastodon"],
        identity=["G"])
    assert [c["name"] for c in kept] == ["Terastodon"]
    assert set(dropped["banned"]) == {"Primeval Titan",
                                      "Emrakul, the Aeons Torn"}


def test_an_invented_card_is_dropped_and_named(pool):
    """Named, not merely counted: "you made that up" and "that is off-colour"
    are different failures and only one of them is about the deck."""
    kept, dropped = argue.resolve_alternatives(
        ["Blossoming Nonesuch", "Terastodon"], identity=["G"])
    assert [c["name"] for c in kept] == ["Terastodon"]
    assert dropped["not_in_pool"] == ["Blossoming Nonesuch"]


def test_alternatives_are_deduplicated_case_insensitively(pool):
    kept, _ = argue.resolve_alternatives(
        ["Terastodon", "terastodon", "  TERASTODON  "], identity=["G"])
    assert len(kept) == 1


def test_alternatives_are_capped(pool):
    names = ["Terastodon", "Craterhoof Behemoth", "Woodfall Primus",
             "Cultivator Colossus", "Regal Behemoth",
             "Goreclaw, Terror of Qal Sisma", "Sol Ring", "Llanowar Reborn"]
    kept, _ = argue.resolve_alternatives(names, identity=["G"])
    assert len(kept) <= argue.MAX_ALTERNATIVES


def test_no_names_means_no_pool_lookup_and_no_drops():
    """Deliberately takes no `pool` fixture: an empty list must not need one."""
    kept, dropped = argue.resolve_alternatives([], identity=["G"])
    assert kept == []
    assert all(v == [] for v in dropped.values())


# ------------------------------------------------------------------- ask()

def test_at_stance_off_no_call_is_made_and_it_says_so(pool, source, no_network):
    """`off` is a real position (ADR 15), and it must not read as "nothing to
    say about this card" -- the two are opposite and look identical from an
    empty list."""
    report = argue.ask("mono-green", "Vorinclex, Voice of Hunger",
                       requested="off", source=source)
    assert report["asked"] is False
    assert report["charges"] == []
    assert "no call was made" in report["reason"]


def test_a_card_the_deck_does_not_run_is_its_own_error(pool, source, no_network):
    """A 422 rather than a 404: the deck is fine, the question is not."""
    with pytest.raises(argue.CardNotInDeck):
        argue.ask("mono-green", "Black Lotus", requested="consultant",
                  source=source)


def test_the_report_says_which_system_answered(pool, source, monkeypatch):
    """ADR 14's third boundary as a field. This mode needs it more than the
    interview does: questions are never mistaken for a verdict, and a reasoned
    case against a card reads exactly like one."""
    monkeypatch.setattr(argue, "converse", lambda *a, **k: turn_returning(
        '{"charges": [], "alternatives": []}'))
    report = argue.ask("mono-green", "Vorinclex, Voice of Hunger",
                       requested="consultant", source=source)
    assert report["answered_by"] == "claude"
    assert report["mode"] == "slot-argument"


def test_a_whole_answer_is_read_filtered_and_labelled(pool, source, monkeypatch):
    """The end-to-end shape, with the model's half stubbed.

    One cited charge and one uncited, one legal alternative and one that is the
    project's own recorded mistake. What comes back is the charge that cited
    something and the card that is actually castable.
    """
    monkeypatch.setattr(argue, "converse", lambda *a, **k: turn_returning("""
    {"charges": [
       {"claim": "Regal Behemoth already doubles your mana for two less.",
        "ground": "redundancy",
        "fact": "Regal Behemoth's rationale claims the same job at MV 6.",
        "strength": "serious"},
       {"claim": "It is simply not very good.",
        "ground": "ceiling", "fact": "", "strength": "decisive"}],
     "alternatives": ["Craterhoof Behemoth", "Ajani, Nacatl Pariah",
                      "Primeval Titan"]}
    """))
    report = argue.ask("mono-green", "Vorinclex, Voice of Hunger",
                       requested="consultant", source=source)

    assert [c["ground"] for c in report["charges"]] == ["redundancy"]
    assert report["charges_dropped"] == 1
    assert [c["name"] for c in report["alternatives"]] == ["Craterhoof Behemoth"]
    assert report["alternatives_dropped"]["off_colour"] == [
        "Ajani, Nacatl Pariah // Ajani, Nacatl Avenger"]
    assert report["alternatives_dropped"]["banned"] == ["Primeval Titan"]
    assert "yours to write" in report["never"]


def test_a_banned_card_in_the_deck_can_still_be_argued_about(pool, source,
                                                             monkeypatch):
    """The hole in rule 1, closed here too.

    `search_cards` filters to Commander-legal, so a banned card is invisible to
    it; `brief()` goes through `cards_named`, which filters on nothing. This
    project deliberately runs two banned cards, so the mode most likely to be
    pointed at one must be able to read it.
    """
    monkeypatch.setattr(argue, "converse", lambda *a, **k: turn_returning(
        '{"charges": [], "alternatives": []}'))
    facts = argue.brief("mono-green", "Primeval Titan", source=source)
    assert facts["card"]["in_pool"] is True
    assert facts["card"]["pool"]["legal_commander"] is False
    assert facts["card"]["pool"]["oracle_text"]
    assert any(i["severity"] == "error" for i in facts["gate"]["about_this_card"])


def test_an_unparseable_answer_reports_rather_than_raises(pool, source,
                                                          monkeypatch):
    monkeypatch.setattr(argue, "converse", lambda *a, **k: turn_returning(
        "not json at all", stop_reason="max_tokens"))
    report = argue.ask("mono-green", "Vorinclex, Voice of Hunger",
                       requested="consultant", source=source)
    assert report["charges"] == []
    assert "max_tokens" in report["reason"]


def test_a_refusal_is_reported_as_one(pool, source, monkeypatch):
    monkeypatch.setattr(argue, "converse", lambda *a, **k: turn_returning(
        "", stop_reason="refusal", refused=True))
    report = argue.ask("mono-green", "Vorinclex, Voice of Hunger",
                       requested="consultant", source=source)
    assert report["asked"] is True
    assert report["charges"] == []
    assert "declined" in report["reason"]
