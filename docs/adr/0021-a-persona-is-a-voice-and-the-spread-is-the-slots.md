# 21. A persona is a voice, and a tarot spread is the slots wearing pictures

**Status:** Accepted · **Decided:** 2026-08-13 · **Recorded:** 2026-08-13

Extends [ADR 15](0015-claude-surfaces-are-modes-with-capabilities.md), which
says a surface is a **mode** plus a **stance**, and
[ADR 20](0020-the-theme-interview-reads-a-person.md), whose readiness
instrument this has to leave standing. It adds a third thing a surface has —
a **persona** — and argues that it is genuinely a third thing rather than a
fourth stance axis.

## Context

The theme interview was play-tested the day after it shipped, and the note that
came back was not a bug:

> "I don't want to get too locked into our question battery. Books, art, tv,
> movies, star signs, all good stuff, but we almost want personas."

Alongside it, a second idea: a tarot reading as a door of its own — deal a
hand, let Claude be the oracle, interpret the spread into a colour identity and
a commander. Explicitly *fun*: "animations, crystal balls, etc."

The first thing to establish was whether the complaint was even accurate, and
it half was. `SLOT_KINDS` and `SLOT_QUESTIONS` are fixed, but they are a
*taxonomy* handed to the model, not a script: every sentence a user reads is
generated. So there is no question battery to unlock. What is fixed is the
register — one warm, curious interviewer, and only that one.

That is the real finding, and it makes the feature cheap: **the thing to vary
is the voice, and the voice is already separate from everything that makes the
interview work.**

## Decision

### A persona is a voice, and it is not a stance

`stance.py` has three axes and they are all about *how much the model does*:
initiative, scope, write autonomy. A persona is about **who it sounds like**,
which is orthogonal. Adding a fourth axis would have made one dial mean two
different kinds of thing, and "set the interviewer to collaborator and the
fortune teller to second-opinion" is a sentence nobody should have to parse.

So `claude/persona.py` is its own concept with its own field on the wire,
travelling exactly as `stance` does. It inherits ADR 15's constraint verbatim:
**a persona may not widen what a mode does.** Same tools, same write scope
(none), same schema.

### The voice is appended to the contract, never substituted for it

`CONVERSATION_INSTRUCTIONS` carries the rules the whole feature rests on — ask
about them and never about Magic, one question at a time, every slot carries a
quote of their own words, never propose. A persona is appended after it.

This is the load-bearing structural choice, because a system prompt is the one
place where "just add a sentence" can quietly undo a rule that took an ADR to
establish. A parametrised test asserts each of those rules still appears in
*every* persona's prompt, so the commit that writes a persona clever enough to
talk its way out of one fails.

`CONVERSATION_MODES["plain"] is THEME_CONVERSATION` — identity, not equality.
Adding personas moved nothing at all for anyone who does not ask for one.

### The spread's positions are the slot kinds

This is the decision that makes the tarot door possible rather than a rewrite.

ADR 20's readiness is a count of **grounded slots**: the model claims a slot,
attaches a quote, and Python checks that quote against what the person actually
typed. Three surviving slots opens the proposal. The obvious way to build a
tarot door — deal three cards, read them, propose — destroys that instrument,
because **a card is not something the querent said.** A reading built that way
would either need a second readiness rule or would let the model report a
preference nobody expressed, which is the exact failure ADR 20 exists to catch.

So the cards do not replace the conversation. `tarot.SPREAD` has three
positions and they *are* `SLOT_KINDS[:3]` — taste, temperament, posture — with
`len(SPREAD) == FLOOR`. A card is dealt **for** a slot. The reader turns it
over, asks about that slot with the picture in front of them, and the answer
grounds it exactly as before.

**The cards colour the questions. The querent's own words remain the only
evidence there is.** A test pins the coupling, because its failure mode is
silent: if the two drift apart, a reading can never reach the floor and the
proposal button simply never lights up.

### Python shuffles; the reader reads

`tarot.py` is stdlib, holds all 78 cards, and holds **no card's meaning**.
There is deliberately no `meaning` field. Writing 78 upright-and-reversed
interpretations into a Python file would be inventing an authority this project
does not have, and would freeze every reading into the same one.

This is [ADR 14](0014-python-decides-claude-advises.md) with candles on it: the
deck, the shuffle, the reversals and the positions have right answers and are
tested without a network; what a spread *means* has no right answer and is the
half Claude is for.

The deal is seeded and returns its seed, so the client carries one integer and
a reload deals the same three cards — the same stateless trick the transcript
uses, and the reason a reading needs no table either.

### Read the person, never the future

The persona is instructed to interpret the querent rather than predict
anything, and this is a design position rather than a disclaimer bolted on.
A real reader is doing close attention to somebody in front of them; the cards
are a mirror that makes a person talk about themselves. That happens to be
exactly what this feature needs, so honesty and usefulness point the same way.

The prompt also forbids breaking frame to explain that it is only a bit.
Winking at the reader would be worse than either committing or not building it:
**a version of this that arrives sensible and dull has missed the point**, and
ROADMAP item 3 said so before a line was written.

### The art is the 1909 printing, and that is not a detail

The original Rider "Roses & Lilies" deck of 1909 is public domain in the US
(published before 1929) and the UK (sources disagree over whether the clock
runs from Waite's death in 1942 or Smith's in 1951; both expired). **US Games
Systems' 1971 recolouring — the deck everybody actually pictures — is still in
copyright.**

Every one of the 78 files carries `RWS1909` in its upstream filename, and the
licence tag was checked per file through the Commons API rather than read off a
category page. The 1909 printing is also visibly different: aged stock with
paper grain, muted uneven colour, and a serif title with a trailing full stop.
`src/mtglab/assets/tarot/PROVENANCE.md` records the argument next to the files.

They ship as `package-data` rather than through `web/public`, because the
latter would store 4.6MB twice in git — once there and once in the committed
bundle Vite writes.

## Consequences

- **Personas are fixed for a conversation.** The transcript is client-held and
  resent whole every turn, so switching halfway leaves the history speaking in
  the old voice. The selector belongs on the door; changing it restarts.
- **Each persona is its own prompt-cache entry.** `converse` caches the system
  block because it is byte-stable per mode. This is why a dealt spread travels
  in the frame *message* — a spread in the system prompt would make every first
  turn a full-price miss, and nothing else would ever notice.
- **The `image` CI job became load-bearing for assets.** Because the art is
  package-data, the *installed* package is the only place the glob can be
  wrong, and the container build is the only place that is visible. It now
  counts the cards.
- **Three voices from the play-test list are still unbuilt** — scientist,
  therapist/confessor, storyteller. Adding one is a `Persona` and a prompt;
  nothing else moves. That was the point.
- **This does not make a deck.** Like ADR 20's proposal, the output is a
  suggestion; the existing create route is what writes anything. No mode may
  write, and `tests/test_claude_boundary.py` still fails the commit that tries.
