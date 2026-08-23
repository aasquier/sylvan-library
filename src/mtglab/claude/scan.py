"""Claude reads a photographed card, and reads it as a camera would.

The seventh mode, and the smallest. It exists for the cards the browser's own
reader cannot do -- and there are a lot of them, because **cards printed
before mid-2015 carry no collector number on the face at all**. The
bottom-left info line arrived with the Magic Origins frame, so every dual
land, every Ravnica shock, every Innistrad flip card reads nothing at all
down there. Those are exactly the deep cuts this library is full of.

**What this mode does not do is name the card.**

That is the whole design, and it is what keeps ADR 14 intact. Naming a card
from a photograph has a right answer, so by ADR 14 it belongs to
deterministic Python -- but *transcribing pixels into text* has no Python
implementation available offline, which is why the reader exists at all. So
the boundary is drawn between the two: Claude returns **what is printed on
the card**, verbatim, and `cards/identify.py` decides what card that is.

Concretely, it fills the same `Sighting` the WebAssembly reader fills, and
everything downstream is unchanged:

* the corner block goes to `identify.from_corner`, which finds a set code only
  if it is one of the pool's real 986 -- so a misread corner fails the same
  way here as it does from OCR, rather than being trusted because a smarter
  reader produced it;
* the title goes to `by_title`, which **offers a shortlist and resolves
  nothing**, however confident the transcription looks. The measurement in
  `identify.py` says right and wrong answers score in overlapping ranges, and
  a better reader does not change that -- it changes how often the right name
  is *in* the list.

So this mode is a better camera, not a better judge. It has no tools, it
never sees a deck, and its answer is two short strings.

**The photograph does leave the browser, and only when asked.** That is the
one thing here the local tier does not do, so it is never automatic: a
capture is sent because somebody pressed a button on that specific card,
having been told what pressing it does.
"""

from __future__ import annotations

import base64
import binascii
from typing import Any

from mtglab.claude import modes
from mtglab.claude import stance as stance_mod

#: What the model may be handed. Anything else is refused before a request is
#: built -- the API accepts these four and a wrong label is a 400 that costs a
#: round trip to learn.
MEDIA_TYPES = frozenset({"image/jpeg", "image/png", "image/webp", "image/gif"})

#: The largest capture accepted, in decoded bytes. The API's own ceiling is
#: 5MB; this sits under it so an oversized frame is refused here, with a
#: sentence somebody can act on, rather than by the platform.
MAX_BYTES = 4 * 1024 * 1024

RESPONSE_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {
        "title": {
            "type": "string",
            "description": (
                "The card's name exactly as printed at the top, or an empty "
                "string if it cannot be read. Never guess a name from the art "
                "or from the rules text."),
        },
        "corner": {
            "type": "string",
            "description": (
                "The small block at the bottom left, transcribed as printed, "
                "one line per line. Typically a collector number and rarity, "
                "then a set code, a language and the artist. An empty string "
                "if the card has no such block or it cannot be read. Older "
                "cards genuinely do not have one."),
        },
    },
    "required": ["title", "corner"],
    #: No field for a card name, a set the model believes it is, or a
    #: confidence. There is nowhere to put a judgement, which is the point --
    #: see the module docstring.
    "additionalProperties": False,
}

INSTRUCTIONS = """\
You are reading a photograph of a single Magic: the Gathering card and \
transcribing the text printed on it. You are acting as a camera, not as an \
expert.

Transcribe two things, exactly as they appear on the card:

1. The card's name, printed in the title bar across the top.
2. The small block at the bottom left. On cards printed since 2015 this holds \
a collector number and rarity on one line, and a set code, a language code \
and the artist's name on the next. Transcribe the lines as printed, in order.

Rules that matter more than being helpful:

* **Transcribe, never identify.** Report the characters you can see. Do not \
correct a name to the card you believe it is, do not expand an abbreviation, \
and do not supply a set code you think is right for a card you recognise. \
Something else checks these against a card database; a plausible correction \
is worse than a faithful misreading, because it cannot be caught.
* **An unreadable field is an empty string.** Glare, a thumb, motion blur, a \
sleeve, a crop that cut the line off -- all of these mean empty. So does a \
card that simply has no bottom-left block: cards printed before mid-2015 do \
not have one, and inventing something there is the worst thing you can do.
* **Do not describe the picture.** Not the art, not the frame, not the \
condition, not what the card does. Two strings is the entire answer.
"""

#: No tools, no deck, no search. The only input is the picture.
MODE = modes.Mode(
    name="scan",
    purpose="transcribe the text printed on a photographed card",
    instructions=INSTRUCTIONS,
    tool_names=(),
    response_schema=RESPONSE_SCHEMA,
    # Low deliberately, and it is not a cost saving. This is a transcription
    # with no reasoning in it, and the higher levels are what make a model
    # reach for context and infer -- which is the one behaviour this mode is
    # written to prevent.
    effort="low",
    # Thinking and answer share this ceiling on Sonnet 5, and thinking is on
    # by default there. The answer is two short strings; the headroom is for
    # the thinking, not the text.
    max_tokens=2048,
)


#: The narrowest preset that still permits a call. Deliberately not
#: `second-opinion`, which the two other deckless surfaces use: those are
#: conversations where volunteering is the feature, and this is a
#: transcription where it is the failure mode.
DEFAULT_PRESET = "consultant"


def stance_for(requested: Any = None) -> stance_mod.Stance:
    """This mode's stance, and the reason it takes one at all.

    There is no deck to derive a default from, and `stance.resolve(None, None)`
    is `off` -- the right answer for "no idea what this is about" and the
    wrong one for a button somebody just pressed on a photograph. **Public for
    the reason `theme.stance_for` and `research.stance_for` are**, and against
    the same bug: `/api/claude` would render `off` for a surface that was
    about to run.

    **And it did, for three months.** This function was written with that
    sentence in its docstring and `service._SURFACE_DEFAULTS` was never
    extended to name `scan`, so `?surface=scan` answered `off` from ADR 34
    landing (#180) until 2026-08-23 -- a guard that described itself
    accurately and was never wired up. Found by the Go port, which had to ask
    what the dial answered rather than what it meant to. Nothing in the app
    sends `surface=scan`, which is exactly how it survived: a docstring is not
    a test, and an unused parameter is not a caller.

    Nothing here is meaningfully steerable in any case -- a stance widens what
    a mode *does*, and there is no more or less forward way to read a
    collector number off a picture. It resolves so the call happens, not so it
    changes, which is why it takes the narrowest preset that is not `off`.
    """
    if requested is None:
        return stance_mod.clamp(stance_mod.PRESETS[DEFAULT_PRESET],
                                stance_mod.ceiling())
    return stance_mod.resolve(requested)


class ScanRefused(Exception):
    """The capture was refused before any request was built."""


def _payload(image: bytes | str, media_type: str) -> str:
    """Validate a capture and return it base64-encoded.

    Refuses here rather than at the API for the usual reason: a 400 from the
    platform arrives after a round trip and says nothing a person can act on.
    """
    if media_type not in MEDIA_TYPES:
        raise ScanRefused(
            f"{media_type!r} is not an image this reads. "
            f"Expected one of: {', '.join(sorted(MEDIA_TYPES))}.")
    if isinstance(image, str):
        try:
            raw = base64.b64decode(image, validate=True)
        except (binascii.Error, ValueError) as exc:
            raise ScanRefused("the capture was not valid base64") from exc
    elif isinstance(image, (bytes, bytearray)):
        raw = bytes(image)
    else:
        # **Refused here since 2026-08-23, and it was a 500 before that.** The
        # `else` took whatever it was handed, so a list or an object reached
        # `len()` and raised an uncaught `TypeError` -- an internal error for a
        # request that is plainly malformed. Found by the Go port on the same
        # day the theme proposal's identical `float(budget)` wart was ruled,
        # and ruled with it: a bad field is a 422 in this project, and the
        # sentence it answers with is one we wrote.
        raise ScanRefused("the capture must be a base64 string")
    if not raw:
        raise ScanRefused("the capture was empty")
    if len(raw) > MAX_BYTES:
        raise ScanRefused(
            f"the capture is {len(raw) // 1024}KB, over the "
            f"{MAX_BYTES // 1024}KB limit. Photograph one card, closer.")
    return base64.b64encode(raw).decode("ascii")


def message(image: bytes | str, media_type: str = "image/jpeg") -> dict[str, Any]:
    """The one user message: the picture, then the ask.

    The image block goes **first**. That is the documented ordering for vision
    requests, and it is also the order the instruction reads in -- the model
    is told what to do with a picture it has already been shown.
    """
    return {
        "role": "user",
        "content": [
            {"type": "image",
             "source": {"type": "base64", "media_type": media_type,
                        "data": _payload(image, media_type)}},
            {"type": "text",
             "text": "Transcribe this card's name and its bottom-left block."},
        ],
    }


def sighting(turn: modes.Turn) -> dict[str, str]:
    """A finished turn as the `Sighting` `/api/cards/identify` already takes.

    The deliberate absence here is any resolution step. What comes back is
    what the reader saw, in the same shape the browser's reader produces, and
    the pool does the rest -- so a card named by Claude and a card named by
    WebAssembly travel exactly the same path and get exactly the same
    scrutiny.
    """
    import json

    if turn.refused:
        return {}
    try:
        read = json.loads(turn.text or "{}")
    except json.JSONDecodeError:
        # A response schema makes this close to impossible, and "close to" is
        # why the branch exists: a truncated answer is still valid JSON's
        # opposite, and it must read as "nothing legible" rather than raise.
        return {}
    if not isinstance(read, dict):
        return {}
    out = {}
    for field_name in ("title", "corner"):
        value = read.get(field_name)
        if isinstance(value, str) and value.strip():
            out[field_name] = value.strip()
    return out
