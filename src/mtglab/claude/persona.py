"""Who the theme interview sounds like.

A **persona** is a voice. It is not a stance and it is deliberately not modelled
as one: `stance.py`'s three axes are about *how much the model does* --
initiative, scope, write autonomy -- and folding "who does it sound like" into
that would make one dial mean two things. A persona may not widen what a mode
does, exactly as a stance may not (ADR 15). It changes register and nothing
else.

That is what makes this cheap, and it is the whole design. `theme.py`'s
`CONVERSATION_INSTRUCTIONS` stays in front of every persona's voice rather than
being replaced by it, so the rules that make the interview work -- one question
at a time, ask about them and never about Magic, every slot carries a quote of
their own words, never propose -- are not a persona's to soften. A voice is
appended; a contract is not.

Three consequences worth knowing before adding a fourth voice:

* **Persona is fixed for a conversation.** The transcript is client-held and
  resent whole every turn, so switching halfway leaves the history speaking in
  the old voice. The selector belongs on the door and changing it restarts.
* **Each persona is its own prompt-cache entry.** `converse` caches the system
  block because it is byte-stable per mode; N personas is N blocks. Irrelevant
  at this scale, and the reason the dealt cards go in the *frame message*
  rather than in here -- a spread in the system prompt would bust the cache on
  every single reading.
* **A dull persona is a bug.** ROADMAP item 3 puts it plainly: the register is
  the requirement, not decoration. A fortune teller that sounds like the plain
  interview with tarot nouns sprinkled on it has missed the point entirely.
"""

from __future__ import annotations

from dataclasses import dataclass

#: What the plain interview has always sounded like. Its `voice` is empty on
#: purpose: `CONVERSATION_INSTRUCTIONS` already ends with a paragraph about how
#: to write, and the default persona is that paragraph. Anything here would be
#: a second opinion about the same thing.
PLAIN_VOICE = ""

FORTUNE_TELLER_VOICE = """
## Your voice

You are reading tarot for this person. Not as a bit, and not as a joke you are
both above -- the way a good reader actually works. You have a table, a cloth,
and three cards already face up between you.

**Read the person, never the future.** This matters and it is not a
disclaimer: a real reader is doing cold, close attention to who is in front of
them, and the cards are a mirror to make somebody talk about themselves. You do
not know what will happen to them. You are not pretending to. What you have is
a picture on a card and a person who reacts to it, and the reaction is the
reading. Never tell them what a card *means* will happen. Ask them what they
see, what it reminds them of, whether it feels like them.

You have been dealt three cards for three places, and you know which is which.
Work the spread in order: The Root, then The Turning, then The Table. Turn each
one over in the conversation before moving on. Let the picture do the asking --
"the woman on this card is pouring water back into the river she took it from;
is that you, or is that the opposite of you?" is a question about *them*
wearing a card as a costume, and it is the shape you want.

A reversed card is not a bad card. It is the same picture with the light coming
from somewhere else, and it is your licence to ask the harder version of the
question.

Be a little theatrical. Pauses, small ceremony, a moment of taking somebody
seriously that they were not expecting. Warmth over spookiness -- you are
delighted by this person, not ominous about them. Do not be arch, do not wink
at the reader, and never break the frame to explain that this is only a bit.

What does not change, no matter how good the theatre gets: one question at a
time; every slot still carries a quote of their own words, copied exactly; you
still know nothing about any Magic card; and you never say what they should
build. The cards colour the questions. Their answers are still the only
evidence there is.
""".strip()


@dataclass(frozen=True)
class Persona:
    """A voice the interview can adopt."""

    key: str
    #: What the door calls it.
    label: str
    #: One line under the label. Written for somebody who has never played.
    blurb: str
    #: Appended after `CONVERSATION_INSTRUCTIONS`, never in place of it.
    voice: str
    #: Whether this persona is dealt a spread before it starts. Only the
    #: fortune teller is, and `tarot.py` owns the deal.
    deals: bool = False


PLAIN = Persona(
    key="plain",
    label="Just talk to me",
    blurb="A few questions about you, and a suggestion at the end.",
    voice=PLAIN_VOICE,
)

FORTUNE_TELLER = Persona(
    key="fortune-teller",
    label="Read my cards",
    blurb="Three cards, and somebody paying you close attention.",
    voice=FORTUNE_TELLER_VOICE,
    deals=True,
)

PERSONAS: dict[str, Persona] = {p.key: p for p in (PLAIN, FORTUNE_TELLER)}

#: What an absent or unreadable value becomes. The plain interview, because
#: that is what every existing client sends today -- nothing.
DEFAULT = PLAIN.key


class UnknownPersona(ValueError):
    """A persona nobody has written. A 422, and it names what there is.

    Its own exception for the reason `TranscriptRejected` has one: the caller's
    answer is to send a different string, not to retry, and certainly not to
    read it as the model failing.
    """


def get(requested: object) -> Persona:
    """The persona for a request, refusing an unknown one rather than guessing.

    `None` is the default and not an error -- every client written before this
    existed sends exactly that, and they should keep working unchanged.
    """
    if requested is None:
        return PERSONAS[DEFAULT]
    if not isinstance(requested, str) or requested not in PERSONAS:
        raise UnknownPersona(
            f"no persona {requested!r}; there is "
            f"{', '.join(repr(k) for k in PERSONAS)}")
    return PERSONAS[requested]


def as_dicts() -> list[dict[str, object]]:
    """The roster, for the door to render. No prompts: `voice` stays server-side.

    Not because it is secret -- the repository is public and anybody can read
    it -- but because a client that received the prompt would eventually send
    one back, and "the persona is chosen from a fixed set" is a property worth
    keeping structural.
    """
    return [{"key": p.key, "label": p.label, "blurb": p.blurb, "deals": p.deals}
            for p in PERSONAS.values()]
