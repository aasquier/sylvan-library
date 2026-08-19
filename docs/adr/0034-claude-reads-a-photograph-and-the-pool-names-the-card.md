# 34. Claude reads a photograph, and the pool names the card

**Status:** Accepted · **Decided:** 2026-08-19 with Aaron · Rides on ADR 14
(Python decides, Claude advises) and ADR 15 (a surface is a mode plus a
stance); the fallback half of the camera door, whose deterministic tier is
`cards/identify.py`.

## Context

Every way into this library was text. A deck that exists only as a stack of
cards on a table — the newcomer's deck, the one commandment 2 is about — had
nowhere to be typed from. The camera door answered that with a reader that
runs entirely in the browser: WebAssembly OCR against crops of the card's
title bar and its bottom-left corner, resolved through the pool.

That tier has a hard floor, and it is not a tuning problem. **Cards printed
before mid-2015 carry no collector number on the face at all** — the
bottom-left info line arrived with the Magic Origins frame. Every dual land,
every Ravnica shockland, every Innistrad flip card reads nothing whatsoever
down there, so those cards fall to the title tier, which by measurement
*offers a shortlist and never resolves*. Those cards are also exactly what
this library is full of: the working style calls for deep cuts from old
Magic, and four of the six curated decks are built on them.

Reading pixels is also the one job in this project with no deterministic
Python answer available. The gate, the solver and the simulator all have
right answers computable offline; "what characters are in this photograph"
does not, which is why an OCR engine was fetched at all.

Aaron chose the shape directly, asked as a question about the reader rather
than about Claude: *local first, Claude as the fallback.*

## The decision

**Claude transcribes what is printed on the card. `cards/identify.py`
decides what card that is.**

The mode returns two strings — the title bar as printed, and the bottom-left
block as printed — and nothing else. It fills the same `Sighting` the
WebAssembly reader fills, and everything downstream is unchanged:

* the corner text goes through `identify.from_corner`, which finds a set code
  only if it is one of the pool's real 986;
* the title goes through `by_title`, which offers five candidates and
  resolves nothing, however confident the transcription looks.

So the model is a **better camera, not a better judge**. A card read by
Claude gets exactly the scrutiny a card read by Tesseract gets.

## Why this keeps ADR 14 intact

ADR 14 says anything with a right answer belongs in deterministic Python.
Naming a card from a photograph plainly has a right answer, so a mode that
returned `{"card": "Sol Ring"}` would be ADR 14 broken, whatever it scored.

The boundary is drawn one step earlier instead. *Transcription* has a right
answer too, but no offline Python implementation — that is the gap the OCR
engine already fills, and a model filling the same gap better is a swap of
readers, not a move of the decision. **The decision never leaves Python**:
the set code is validated against the pool, the name is never resolved from a
similarity, and the person confirms every card either way.

Two things enforce that rather than request it:

* **The response schema has no field for a card.** `title` and `corner`,
  `additionalProperties: false`. There is nowhere to put a name, a set, or a
  confidence — the same technique ADR 25 used to make a balanced argument
  impossible to express.
* **The prompt forbids correction explicitly**, because the schema cannot see
  the difference between a faithful transcription and a plausible one. A
  model that silently corrects `LTCENLIK` to a set code it recognises would
  pass the schema and defeat the design.

## Options considered

**Let Claude name the card.** Fewer moving parts, and it would be right more
often than a shortlist. Rejected: it moves the deciding into the model,
makes rule 1 unenforceable at the point it matters, and produces a wrong
answer nobody can see — the resolved card and the evidence would come from
the same place.

**Claude only, no local tier.** Simpler, and better on hard cards. Rejected
because it makes every capture cost money and sends every photograph off the
device, including the great majority the browser can read for nothing.

**Local only, and accept the floor.** Free forever, nothing leaves the
browser. Rejected because the floor lands precisely on this library's own
decks, and telling a first-time player their cards are too old is
commandment 2 failing.

## Consequences

**The photograph leaves the browser, and only on purpose.** This is the one
route in the app that receives an image. It is never automatic: the local
tier sends two short strings, and a capture is sent because somebody pressed
a button on that card having been told what the button does. The image is
not written to disk and not logged.

**It costs money, per card, and only on the cards that needed it.** A
capture at the guide's size is roughly 800 image tokens; at Sonnet 5's rate
that is about two tenths of a cent per card. A whole 99 falling back would
be about twenty cents — the ceiling, not the expectation, since the tier
only fires where the local reader failed.

**It is a background job from its first commit, and its duration is
unmeasured.** ADR 20's lesson has cost three incidents, twice because a
docstring said "it is a few seconds". A vision call at `low` effort ought to
be quick; "ought to be" is not a measurement, and the failure mode is a
transport error with no status code.

**`effort` is `low`, and not to save money.** Higher effort is what makes a
model gather context and infer, which is the single behaviour a transcriber
must not have.

**The stance is `consultant` — the narrowest preset that is not `off`.** The
two other deckless surfaces default to `second-opinion` because volunteering
is their feature; here it is the failure mode.

**What is transcribed rides back beside what was resolved**, so the page can
show both. A wrong reading next to the words it came from is a mistake
somebody can catch; a wrong reading alone is one they cannot.
