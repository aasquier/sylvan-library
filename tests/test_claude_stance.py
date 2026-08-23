"""The stance dial: three axes, named presets, and a deployment ceiling.

No key, no network, no SDK. That is not incidental — ADR 15 says *off is a real
position*, and the decision not to call must not itself depend on a call. Every
assertion here runs on a base install.
"""

from __future__ import annotations

import dataclasses

import pytest

from mtglab.claude import stance as st


class FakeDeck:
    def __init__(self, status: str):
        self.status = status


@pytest.fixture(autouse=True)
def _no_ceiling(monkeypatch):
    """Most tests want the default ceiling, not whatever the machine exports."""
    monkeypatch.delenv(st.CEILING_ENV, raising=False)


# ----------------------------------------------------------------- the axes

def test_the_three_axes_are_ordered_least_to_most():
    """Index is the level, which is what makes clamping a min() rather than a
    table of special cases."""
    assert st.INITIATIVE[0] == "off"
    assert st.WRITE[0] == "none"
    for axis in st.AXES:
        assert len(set(st.LEVELS[axis])) == len(st.LEVELS[axis])


def test_a_bad_level_is_refused_with_the_valid_ones_named():
    with pytest.raises(ValueError, match="not a initiative level"):
        st.Stance(initiative="whenever")
    with pytest.raises(ValueError, match="not a write level"):
        st.Stance(write="anything")


def test_at_least_compares_along_one_axis():
    s = st.Stance("volunteers", "adjacent", "none")
    assert s.at_least("initiative", "on-request")
    assert not s.at_least("initiative", "interjects")
    assert s.at_least("scope", "adjacent")
    assert not s.at_least("write", "proposes")


# ------------------------------------------------------------- off is real

def test_off_makes_no_calls():
    """The single most important line in the module."""
    assert st.OFF.allows_calls is False
    assert st.OFF.may_write is False


def test_every_other_preset_allows_calls():
    for name, s in st.PRESETS.items():
        if name == "off":
            continue
        assert s.allows_calls, name


def test_no_preset_reaches_the_top_of_the_write_axis():
    """`applies` exists on the axis, but nothing hands it out by name.

    ADR 15 allows it; a preset that quietly selected it would mean somebody got
    autonomous edits by picking a friendly-sounding word off a menu.
    """
    assert all(s.write != "applies" for s in st.PRESETS.values())


# --------------------------------------------------------------- defaulting

def test_a_built_deck_defaults_narrower_than_a_theoretical_one():
    """The default comes from a field the deck already has. Arahbo is sleeved
    cardboard; Goreclaw is a list under consideration."""
    built = st.default_for(FakeDeck("built"))
    theoretical = st.default_for(FakeDeck("theoretical"))
    assert built == st.CONSULTANT
    assert theoretical == st.SECOND_OPINION
    assert theoretical.at_least("initiative", "volunteers")
    assert not built.at_least("initiative", "volunteers")


def test_no_default_can_write():
    for status in ("built", "theoretical", "", "nonsense"):
        assert not st.default_for(FakeDeck(status)).may_write


def test_an_unknown_status_defaults_to_the_narrower_stance():
    """Absent means `theoretical` in the deck model, but a stance default is a
    permission — so an unreadable status gets the quieter answer, not the
    wider one."""
    assert st.default_for(FakeDeck("")) == st.CONSULTANT
    assert st.default_for(object()) == st.CONSULTANT


# ----------------------------------------------------------------- parsing

def test_a_partial_mapping_fills_from_off_not_from_the_top():
    """A half-written request can only ever be quieter than intended."""
    s = st.Stance.from_obj({"initiative": "interjects"})
    assert s.initiative == "interjects"
    assert s.scope == "flagged"
    assert s.write == "none"


def test_none_is_off_rather_than_a_default_that_talks():
    assert st.Stance.from_obj(None) == st.OFF


def test_an_unknown_axis_is_refused():
    with pytest.raises(ValueError, match="not stance axes"):
        st.Stance.from_obj({"enthusiasm": "high"})


def test_a_preset_name_parses():
    assert st.Stance.from_obj("collaborator") == st.COLLABORATOR
    with pytest.raises(ValueError, match="not a stance preset"):
        st.Stance.from_obj("chatty")


# ----------------------------------------------------------- the ceiling

def test_the_ceiling_defaults_to_the_top_preset(monkeypatch):
    """A local run on your own key should not need configuring."""
    assert st.ceiling() == st.COLLABORATOR


def test_a_deployment_can_cap_what_anyone_selects(monkeypatch):
    monkeypatch.setenv(st.CEILING_ENV, "consultant")
    effective = st.resolve("collaborator")
    assert effective == st.CONSULTANT
    assert not effective.may_write


def test_a_ceiling_of_off_silences_the_deployment(monkeypatch):
    """Whatever any user has saved."""
    monkeypatch.setenv(st.CEILING_ENV, "off")
    assert st.resolve("collaborator").allows_calls is False
    assert st.resolve({"initiative": "interjects"}).allows_calls is False


def test_an_unreadable_ceiling_fails_closed(monkeypatch):
    """A typo in a deployment variable should cost a feature, not open one."""
    monkeypatch.setenv(st.CEILING_ENV, "unlimited")
    assert st.ceiling() == st.OFF
    assert st.resolve("collaborator").allows_calls is False


def test_the_ceiling_is_read_per_call_not_at_import(monkeypatch):
    """An operator lowering the cap should not need a restart."""
    assert st.ceiling() == st.COLLABORATOR
    monkeypatch.setenv(st.CEILING_ENV, "off")
    assert st.ceiling() == st.OFF


def test_clamping_is_per_axis():
    """So "may speak freely but may never write" is expressible — the shape a
    hosted instance most plausibly wants."""
    limit = st.Stance("interjects", "rethink", "none")
    asked = st.Stance("volunteers", "rethink", "applies")
    got = st.clamp(asked, limit)
    assert got.initiative == "volunteers"   # under the cap, untouched
    assert got.scope == "rethink"
    assert got.write == "none"              # capped


def test_clamping_never_raises_a_level():
    """A ceiling is a maximum, not a setting."""
    limit = st.COLLABORATOR
    assert st.clamp(st.OFF, limit) == st.OFF


def test_resolve_falls_back_to_the_decks_default():
    assert st.resolve(None, deck=FakeDeck("theoretical")) == st.SECOND_OPINION
    assert st.resolve(None, deck=FakeDeck("built")) == st.CONSULTANT


def test_resolve_with_no_deck_and_no_request_is_off():
    assert st.resolve() == st.OFF


# ---------------------------------------------------------------- describe

def test_describe_names_the_preset_when_there_is_one():
    body = st.describe(st.CONSULTANT)
    assert body["preset"] == "consultant"
    assert body["allows_calls"] is True
    assert [a["axis"] for a in body["axes"]] == list(st.AXES)


def test_describe_has_no_preset_for_a_custom_stance():
    body = st.describe(st.Stance("interjects", "flagged", "none"))
    assert body["preset"] is None


def test_every_level_on_every_axis_has_a_meaning():
    """The UI renders these; a missing one would be a blank radio button."""
    for axis in st.AXES:
        for level in st.LEVELS[axis]:
            assert st.LEVEL_MEANINGS[(axis, level)].strip()


def test_every_preset_has_a_blurb():
    assert set(st.PRESET_BLURBS) == set(st.PRESETS)


def test_a_stance_cannot_be_mutated_by_the_mode_it_is_handed_to():
    """A mode that could edit its own dial would be widening it."""
    with pytest.raises(dataclasses.FrozenInstanceError):
        st.CONSULTANT.write = "applies"  # type: ignore[misc]


# ------------------------------------------------------------- rejections

@pytest.mark.parametrize("requested", [
    "emperor",                       # not a preset
    {"scope": "everywhere"},         # not a level
    {"tempo": "fast"},               # not an axis
    7,                               # not a shape that can hold one
    ["off"],
])
def test_an_unreadable_stance_is_a_stance_rejected(requested):
    """One name for every spelling of "that is not a stance".

    `resolve()` is the one function callers use, and every way it can refuse
    a request now comes out as `StanceRejected` -- still a `ValueError`, so
    nothing that caught one stops catching it, and still carrying the same
    sentence. What the name buys is the service layer's re-raise tuple:
    without it a malformed stance fell into `except Exception` there and was
    answered 502 as a failed call, while the route's own 422 branch sat dead.
    """
    with pytest.raises(st.StanceRejected) as rejected:
        st.resolve(requested)
    assert isinstance(rejected.value, ValueError)
    # The sentence is `from_obj`'s own, unchanged: only the name moved.
    with pytest.raises(ValueError) as raw:
        st.Stance.from_obj(requested)
    assert str(rejected.value) == str(raw.value)


def test_a_readable_stance_is_never_a_rejection():
    assert st.resolve("consultant") == st.CONSULTANT
    assert st.resolve({"initiative": "on-request"}) == st.Stance(
        "on-request", "flagged", "none")
