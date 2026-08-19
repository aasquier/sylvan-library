"""The seventh mode: Claude as a camera.

No test here sends a request. What is pinned is the shape of what would be
sent, the refusals that happen before anything is built, and — the one that
matters — **that the answer has nowhere to put a judgement**.

`test_the_schema_cannot_name_a_card` is this file's reason to exist. The
mode's whole claim is that it transcribes and the pool decides; the schema is
where that claim is enforced, exactly as `argue.py`'s schema has no `defence`
field. A `name`, a `card`, or a `confidence` added here would move the
deciding out of `cards/identify.py` and into a model, and nothing else in the
stack would notice.
"""

import json
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "src"))

from mtglab.claude import modes, scan


def turn(text: str, *, refused: bool = False) -> modes.Turn:
    return modes.Turn(mode="scan", model="test", stop_reason="end_turn",
                      text=text, tool_calls=[], input_tokens=0,
                      output_tokens=0, refused=refused)


# --------------------------------------------------- the shape of the answer

def test_the_schema_cannot_name_a_card():
    """The mode transcribes; the pool decides. The schema is where that is
    enforced rather than requested."""
    properties = scan.RESPONSE_SCHEMA["properties"]
    assert set(properties) == {"title", "corner"}
    assert scan.RESPONSE_SCHEMA["additionalProperties"] is False
    for forbidden in ("name", "card", "confidence", "set", "collector_number",
                      "oracle_id", "verdict", "certainty"):
        assert forbidden not in properties, forbidden


def test_the_mode_has_no_tools_and_no_deck(): 
    """It reads a picture. There is nothing to look up and nothing to see."""
    assert scan.MODE.tool_names == ()
    assert scan.MODE.server_tools == ()
    assert scan.MODE.may_write == ()


def test_the_instructions_forbid_identifying():
    """The prompt is the second line of defence behind the schema, and it is
    the one that stops a *plausible* correction — which the schema cannot see."""
    said = scan.MODE.instructions.lower()
    assert "transcribe" in said
    assert "do not correct" in said
    assert "empty string" in said


def test_effort_is_low_on_purpose():
    """Higher effort is what makes a model reach for context and infer, which
    is the single behaviour this mode exists to prevent."""
    assert scan.MODE.effort == "low"


# ------------------------------------------------------------- the refusals

@pytest.mark.parametrize("media_type", [
    "image/tiff", "application/pdf", "text/plain", "image/svg+xml", "", "jpeg",
])
def test_an_unreadable_format_is_refused_before_a_request(media_type):
    with pytest.raises(scan.ScanRefused, match="not an image"):
        scan.message(b"\xff\xd8\xff", media_type)


def test_an_oversized_capture_is_refused_with_a_number(): 
    """A person can act on 'photograph one card, closer'; they cannot act on
    a 400 from the platform."""
    with pytest.raises(scan.ScanRefused, match="over the"):
        scan.message(b"x" * (scan.MAX_BYTES + 1), "image/jpeg")


def test_an_empty_capture_is_refused():
    with pytest.raises(scan.ScanRefused, match="empty"):
        scan.message(b"", "image/jpeg")


def test_bytes_that_are_not_base64_are_refused():
    with pytest.raises(scan.ScanRefused, match="base64"):
        scan.message("not base64 at all!!", "image/jpeg")


# ------------------------------------------------------------- the request

def test_the_picture_comes_before_the_ask():
    """The documented ordering for vision requests, and the order the
    instruction reads in."""
    content = scan.message(b"\xff\xd8\xff\xe0", "image/jpeg")["content"]
    assert content[0]["type"] == "image"
    assert content[1]["type"] == "text"
    assert content[0]["source"]["media_type"] == "image/jpeg"


def test_raw_bytes_and_base64_reach_the_same_payload():
    import base64
    raw = b"\x89PNG\r\n\x1a\n"
    from_bytes = scan.message(raw, "image/png")["content"][0]["source"]["data"]
    from_text = scan.message(base64.b64encode(raw).decode(), "image/png")
    assert from_bytes == from_text["content"][0]["source"]["data"]


# ------------------------------------------------------------- the reading

def test_a_reading_is_the_sighting_the_pool_already_takes():
    """Same shape the browser's own reader produces, so both travel the same
    path and get the same scrutiny."""
    read = scan.sighting(turn(json.dumps(
        {"title": "Sol Ring", "corner": "0284/0281 U\nLTC EN Mike Burns"})))
    assert read == {"title": "Sol Ring",
                    "corner": "0284/0281 U\nLTC EN Mike Burns"}


def test_an_empty_field_is_dropped_rather_than_carried():
    """A pre-2015 card has no corner at all, and an empty string must not
    reach `from_corner` as though it were a failed read of something."""
    read = scan.sighting(turn(json.dumps({"title": "Black Lotus", "corner": ""})))
    assert read == {"title": "Black Lotus"}
    assert scan.sighting(turn(json.dumps({"title": "  ", "corner": "  "}))) == {}


def test_a_refusal_reads_as_nothing_legible():
    assert scan.sighting(turn("", refused=True)) == {}


@pytest.mark.parametrize("text", ["", "not json", "[]", "null", '{"title": 7}'])
def test_an_unusable_answer_reads_as_nothing_rather_than_raising(text):
    """A response schema makes these close to impossible, and 'close to' is
    why the branch exists — a truncated answer must read as an unreadable
    card, not as a crash mid-import."""
    assert scan.sighting(turn(text)) == {}


def test_a_stance_resolves_rather_than_refusing_the_call():
    """There is no deck here, and `resolve(None, None)` would land on `off` —
    the bug `/api/claude`'s `surface` parameter was added for."""
    assert scan.stance_for().allows_calls


# ------------------------------------------------------------ the job body
#
# `plan_scan` refuses in the request and the *work* happens in the closure it
# hands back — and until 2026-08-19 nothing exercised that closure at all
# (`api/scanruns.py` sat at 61%, the least-covered module in the app, on the
# newest paid surface). What the tests above pin is the mode; what these pin
# is ADR 34's actual claim, which lives in the closure: the transcription goes
# through `identify` exactly as the browser's reader's does, so the pool still
# names the card. That is checked against `tiny_pool`'s real printings rather
# than a fake `identify`, because a faked pool would agree with any wrong
# answer the closure passed it.


@pytest.fixture
def shot(tmp_path, monkeypatch):
    """A planned scan against a real pool, with no key and no network.

    `require` is the only thing between a test and an SDK check; `converse`
    is replaced per test, because what each one is about is the answer.
    """
    import tiny_pool
    from mtglab import config
    from mtglab.api import scanruns
    from mtglab.claude import client as claude_client

    monkeypatch.setattr(claude_client, "require", lambda: None)
    with config.use_paths(data_dir=tmp_path / "data"):
        tiny_pool.build(config.DB_PATH)
        yield scanruns.plan_scan(image=b"\xff\xd8\xff\xe0 a photograph")


def run(plan):
    """The closure, driven the way a worker drives it."""
    return plan.run(lambda *a, **k: None)


def test_the_pool_names_the_card_the_transcription_only_described(shot,
                                                                 monkeypatch):
    """ADR 34 in one assertion. Claude returns a set code and a number and no
    card name — the schema has no field for one — and the answer that comes
    back names Sol Ring because the *pool* resolved that printing."""
    from mtglab.claude import scan as scanmode

    monkeypatch.setattr(modes, "converse", lambda *a, **k: turn(json.dumps(
        {"title": "", "corner": "0284/0281 U\nLTC EN Mike Burns"})))
    assert "name" not in scanmode.RESPONSE_SCHEMA["properties"]

    body = run(shot)
    assert body["reading"]["via"] == "printing"
    assert body["reading"]["resolved"]["name"] == "Sol Ring"
    assert body["transcribed"]["corner"].startswith("0284/0281")


def test_an_unreadable_capture_is_an_answer_and_not_a_failure(shot,
                                                              monkeypatch):
    """A card the model could not read comes back as `reading: None` — the
    page says so. Raising here would present a blurry photograph as a broken
    feature."""
    monkeypatch.setattr(modes, "converse",
                        lambda *a, **k: turn("", refused=True))
    body = run(shot)
    assert body["reading"] is None
    assert body["transcribed"] == {}


def test_an_exhausted_mode_becomes_the_failure_the_page_reads(shot,
                                                              monkeypatch):
    """`ModeExhausted` is translated rather than escaping, so the job's error
    field holds a sentence instead of the SDK's own words."""
    from mtglab.api.service import ClaudeFailed
    from mtglab.claude.modes import ModeExhausted

    def exhausted(*a, **k):
        raise ModeExhausted("the conversation ran out of turns")

    monkeypatch.setattr(modes, "converse", exhausted)
    with pytest.raises(ClaudeFailed, match="ran out of turns"):
        run(shot)


def test_an_sdk_failure_is_explained_rather_than_raised_raw(shot, monkeypatch):
    """The broad `except` is broad on purpose: everything landing there is the
    SDK, and `explain` is what turns a 401 into a sentence about the key."""
    from mtglab.api.service import ClaudeFailed

    def boom(*a, **k):
        raise RuntimeError("authentication_error: invalid x-api-key")

    monkeypatch.setattr(modes, "converse", boom)
    monkeypatch.setattr("mtglab.claude.client.explain",
                        lambda exc: "your key may have expired")
    with pytest.raises(ClaudeFailed, match="may have expired"):
        run(shot)
