# 45. The scribe listens for four more things, and the board still never guesses

**Status:** Accepted · **Decided:** 2026-08-26 · Extends
[ADR 42](0042-a-scribe-rides-forges-event-bus.md) (a scribe rides Forge's event
bus) and finishes the three things
[ADR 44](0044-the-board-holds-state-and-never-holds-a-guess.md) left named and
undone. Corrects one factual claim in ADR 44's Consequences; supersedes neither.

## Context

ADR 44 ended with a list of three events Forge's bus carries that the scribe did
not listen for, all of them additive and none blocked by anything decided there.
Aaron then asked for five things in one sitting, and every one of them was on
that list or beside it:

- **The mana pool.** *"It would be nice if we could show the mana pool as things
  tap into it before it is drained to cast things… The regular icons for each
  symbol would suffice."*
- **A Treasure tapped and then sacrificed.** *"They must tap to sacrifice and
  they go into the ether."* A cracked Treasure raised no beat at all: `dies` is
  creatures and planeswalkers by rule 700.4, and a token leaving the battlefield
  is rewritten to `gone`, so the whole transaction folded silently into the next
  step. A fetchland is the same shape and this format is full of them.
- **Kaheera's vigilance.** *"Some cards like Kaheera give vigilance or another
  effect to other cards, we currently are not representing that symbolically…
  it would be nice if a hover showed the vigilance badge for instance, and cited
  the source being Kaheera."*
- **Eminence.** *"It can be used on the battlefield or from the command zone…
  It should just visually indicate that an ability is being used."*
- **Populate.** *"It really is making a clone, or splitting one thing into two."*

Four of the five were unanswerable from what the pipe carried. The fifth —
keyword attribution — is unanswerable from Forge.

**Everything below was settled against Forge 2.0.14's bytecode and then against
a real match played on this laptop**, because ADR 42's own rule is that a
listener added to this bus counts before it assumes, and because this project
has twice been burned by two encodings whose wrong reading looks identical. The
match was a Kaheera-commander deck against a copy of itself, built to put every
one of the five on the table at once: Beasts that gain vigilance they do not
print, a Wooded Foothills that taps and sacrifices itself, Growing Ranks
populating a Centaur Token, and Sol Ring making colourless mana.

## The four things Forge would not say, and the two traps

**`GameEventCombatUpdate` is not the end-of-combat signal, and ADR 44 says it
is.** That is the one claim in ADR 44 that is wrong, and it is wrong in the way
that costs the most: a listener built on it compiles, subscribes, and sits
silent forever. A reference scan across all of Forge's classes finds it
constructed in exactly two places — `InputAttack` and `InputBlock`, the *human*
declare-attackers and declare-blockers handlers, which raise it on every click
so a person's own screen can keep up. Nothing in `forge.game.**` posts it. **In
a headless AI match it never fires once.** The engine's own signal is
`GameEventCombatEnded`, raised from `PhaseHandler.onPhaseEnd()` while
`inCombat()`, and it fired 46 times in a 46-turn recorded game.

**Colourless mana lives at byte 32, not byte 0.** Forge has two constant sets
for the same six things: `MagicColor` calls colourless `0` and `ManaAtom` calls
it `32`. `PlayerView.updateMana` fills its map by iterating `ManaAtom.MANATYPES`,
and `getMana(byte)` is a plain map lookup with no masking — so
`getMana(MagicColor.COLORLESS)` asks for a key that is not there and returns
zero forever. The two readings are indistinguishable in the data: a pool with no
colourless mana and a pool asked the wrong question both render empty.
`Mana.isColorless()` is literally `color == 32`, which settled it. The recorded
match shows `"pool":"CC"` off a Sol Ring, which is the proof it is right.

**`GameEventCardDestroyed` is a record with no components at all** — a bare
signal, like `GameEventTokenCreated`. It cannot say which card was destroyed.
`GameEventCardSacrificed` carries a `CardView` and can.

**Keyword attribution does not exist at Forge's view layer.** The model has it:
`KeywordInterface` carries `isIntrinsic()` and `getStatic()`, and the granting
card is `getStatic().getHostCard()`. The **view** does not: `KeywordView` is a
four-field record — original, keyword, title, reminder text — and carries no
host, no source, and not even the intrinsic flag. `KeywordCollection.getView()`
maps every entry through a projection that keeps those four. So a listener on
the event bus, which is handed `CardView`s, cannot answer *why* a card has
vigilance at any price.

## Decision

**1. The pool is read off the view, and the whole pool crosses every time.**
`GameEventManaPool` carries a mode and the set of colours that moved, and the
set is **null** on the mode that empties the pool — `ManaPool`'s clear path
passes `aconst_null` where its other two pass a set. A reader adding and
subtracting the reported colours would therefore drift on exactly the event that
matters most. Sending the totals costs six lookups and cannot drift. The amounts
are fresh: all three sites that raise this event call `Player.updateManaForView()`
on the line before they construct it.

**2. The pool is the one sequence on this wire.** Every other thing a
[`BoardStep`] carries is a state, folded last-write-wins. A pool is not: it fills
and empties several times between two beats, so a step holding only the value it
ends on holds an empty pool nearly every time. **Measured before this was a
sequence: ten pool changes reached the browser and nine of them were empty** —
a truthful answer to a question nobody asked, and precisely the opposite of what
was requested. So a step carries every value the pool took, in order. A consumer
wanting the resting state reads the last entry, which is what a fold does anyway.

**3. Sacrifice is the only word the board has for how a permanent left, and
that is where it stops.** `BoardChange.Fate` is `sacrificed` or nothing. A
destruction cannot be attributed to a card and a combat death is not announced
at all, so the board says what Forge said and stays quiet about the rest rather
than reading a word off the circumstances. A creature sacrificed raises both
`sacrificed` and `dies`, which is correct: it was sacrificed and it did die.

**4. The live keyword set crosses; the *granted* subset is worked out in Go,
one layer up.** The scribe sends `CardStateView.getKeywords()` — the recomputed
set, granted keywords included — as `original()` strings. `internal/sim/tier3`
cannot say which of them were granted, because it does not know what any card
was printed with. `internal/api` does: it already resolves every card's printing
to paint the board. So the subtraction happens there, and the answer is a
separate field.

The two lists speak different dialects — Forge writes `Ward:2` and
`Protection from red`, Scryfall writes `Ward` and `Protection` — so a live
keyword counts as printed when the oracle name is a prefix of it **at a word
boundary**. Compared as plain strings, a card that prints Ward 2 would wear a
badge saying something gave it to them, which is a small confident lie about a
card the player can read.

**5. The board says *that* a keyword was granted and never *by what*, and there
will be no field for it.** This is ADR 44's counter-attribution ruling, arrived
at independently for keywords and landing in the same place. The only way to
name a granter from what the bus carries is to blame whatever resolved most
recently, which is inference wearing a fact's clothes. **The copy that renders
this must not imply an agent.** Aaron asked for the source to be cited; the
honest answer is that Forge erases it at the view boundary, and it is his call
whether reaching into the model layer is worth reopening ADR 42's division for.

**6. Abilities go on the wire, and this does not reopen the stack.** The scribe
returned on anything that was not a spell, so eminence — a triggered ability
whose source sits in the command zone and never moves — was invisible from end
to end. `board.go`'s first ruling drops Stack *zone* events because they never
balance (52 in against 14 out in one game), and that is untouched. An ability
cast is a different event about a different subject: it fires once, nothing
accumulates, and nothing waits for it to come off anything. Counted before it
was added, as ADR 42 asks: **ten in a forty-six-turn game**, against 46 lands
and 548 zone movements. The source's zone rides along, because it is the whole
of what makes eminence legible.

**7. A copied token names the card whose ability made it, and not what it was
copied from.** `CardView.getCloneOrigin()` is on the view and is populated, and
its presence *is* the copy — a token minted fresh carries nothing. But it is not
the parent: a Centaur Token populated by Growing Ranks names **Growing Ranks**,
because `TokenEffectBase` hands `sa.getHostCard()` to `setCloneOrigin`. The
permanent that was copied is `Card.getCopiedPermanent()`, which is model-only.
A whole-jar scan finds `setCloneOrigin` called from one game effect and
otherwise only from the AI's own state-copying machinery, so there is no second
meaning waiting in another card.

**8. Combat ends when Forge says it ends, and the turn boundary stands in only
until it does.** One rule with a stated precedence rather than two that can
disagree. A worker image built before the scribe learned `GameEventCombatEnded`
sends no such line at all, and a board that dropped the turn fallback would
leave those matches marked as attacking forever. The latch is per game.

**9. A beat carries the board id of the card it is about.** A name cannot answer
*which one*: two Egg Tokens are one string between them and so are two copies of
a commander. Zero on the prose path, which has no ids to give.

## Consequences

**Four of the five asks are answered and the fifth is answered honestly.** The
badge can say *this creature has vigilance and its printing does not*. It cannot
say *Kaheera gave it*, and that gap is now written down with the bytecode that
makes it a gap rather than an oversight.

**The scribe reads Forge's model nowhere, and that boundary held under
pressure.** Two of the five asks — the granting card, and the permanent a token
was copied from — are answerable from the model and not from the view. Both were
left unanswered rather than reaching across, because ADR 42's division is what
keeps the pipe small and the worker replaceable. That is a cost, and it is the
cost that was chosen.

**Every new beat kind needs a sentence.** `web/src/lib/theater.ts` renders an
unrecognised kind as its raw name, so `sacrificed` and `ability` will read as
debug text until somebody writes their copy — which is a commandment 2 and 10
problem, and the same one `enters` and `attach` already have.

**A keyword that changes nothing else is still invisible.** The live set rides
every line naming a card, and Forge raises those lines for zone changes, taps,
stats and combat. A grant that moved no numbers and touched no card otherwise
would not be reported until something else mentioned that card. Nothing observed
in a real match hits this, and it is a real hole rather than a theoretical one.

**`GameEventCardDestroyed` and `GameEventTokenCreated` remain unsubscribed**,
and should stay that way: both are bare signals, and a listener on either would
be able to say only that *something* happened.
