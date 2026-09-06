# 51. A Rulebreaker clause is read from the card, and its one choice is read from the deck

**Status:** Accepted · **Decided:** 2026-09-06 · An instance of
[ADR 7](0007-card-facts-come-from-the-corpus.md) (card facts come from the
pool) and [ADR 8](0008-the-gate-blocks.md) (an unevaluated rule warns rather
than passing), applied to the first mechanic that changes what a legal
Commander deck *is*.

## Context

Mystery Booster Commander Edition (`mbc`, 2026-11-09) prints **Rulebreaker** on
eight commanders. The clause is a deckbuilding permission, and it is the first
printed text that moves the boundary the gate has enforced since it was
written:

    Rulebreaker — A deck with this commander can have Angel cards of any
    color identity and any basic land cards.

Under that commander, a red-white Angel and a Swamp are legal in a mono-white
deck. `internal/gate` did not know the word, so it reported each of them as a
`color-identity` error — a legal deck told, by name, card by card, that it was
illegal. That is commandment 2's exact failure mode, and it blocked a real
import.

Seven of the eight clauses widen colour identity for a named group of cards.
One (Whtz, the Bibliophile) removes the maximum deck size instead. One
(Tolabow, Loch Rascal) widens the identity of the deck's instants and
sorceries by **one colour of the deckbuilder's choosing**.

## Options considered

**A table of the eight card names.** Fastest, and wrong the day a ninth is
spoiled — wrong *silently*, which is the part that disqualifies it. It also
duplicates a fact that is already printed on the card, and this repo has had
five separate copied facts rot.

**Parse the clause off the card's oracle text.** More work, and it can fail on
a sentence nobody has seen. But the failure is detectable, and ADR 8 already
says what to do with a rule that could not be evaluated: say so.

**For Tolabow: a `rulebreaker_color:` key in the deck file.** The obvious
design — the clause says "of your choice", so store the choice. It would reach
the deck model, the emitter, the edit engine, the wire, and the browser, for
one card; and it would be a second copy of a fact, to be kept in step with the
cards by hand.

**For Tolabow: derive the choice from the deck.** The deck is legal exactly
when *some* legal choice exists — that is, when the off-identity colours its
instants and sorceries actually use number one or none. Nothing in the game
ever asks which colour was picked: it changes no cost, no ability, no zone.

## Decision

**The clause is read from the card, never from a list of names.** `gate`
parses the three printed sentence shapes and records the *grammar*, not the
cards. The eight names appear in this repo only in a test, and that test says
where they came from and how to re-derive them.

**A clause that will not parse is reported, and widens nothing.** ADR 8, at
the sharpest edge it has had: an unreadable clause emits a `rulebreaker-unread`
warning naming the commander and quoting the sentence, and the identity check
then runs unwidened. The player gets the *old* wrong answer with a sentence
attached saying that is what happened — never a quiet pass, and never a quiet
refusal with no reason.

**Tolabow's colour is derived, not stored.** No deck-file key. The gate reads
back the off-identity colours the deck's instants and sorceries reach; one or
none is legal, two or more is a `rulebreaker-color-choice` error naming both
colours and the cards that reach them. This is not a heuristic — it is the
same question the rule asks, answered exactly.

**The identity error quotes the clause it failed.** A card the clause does not
cover gets the sentence it did not satisfy, printed, so the next question —
"then which cards *does* it cover" — is answered on the same line.

**One check, both doors.** `api.playableCard` reads the same clauses. A gate
that passes a card while the write path refuses it is the worse half of the
bug: a wrong report can be read past, and a refused write cannot.

## Consequences

- The eight cards are covered without being named in the code. A ninth that
  fits the grammar works on the day it is spoiled; one that does not says so.
- `deck-size` becomes a floor rather than a number under Whtz. The singleton
  rule is untouched and still applies to all 200 of them.
- A Rulebreaker clause deliberately does **not** widen the companion check.
  Every clause says what "a deck with this commander can have", and a companion
  is not in the deck — it waits outside the hundred. Moot in fact, since no
  companion is an Angel, an Aura, a Phyrexian, an Equipment, a land or a
  seven-drop, and written down because a silence is not an answer.
- Advisory surfaces still narrow by the commander's printed identity:
  `suggest.ReplacementsFor`, `claude.argue`'s candidate filter and the Wheel
  will not offer an off-colour Angel to a Seluma deck. None of them is a gate
  and none reports an error; widening them is follow-up work, not this ADR.
- **A `rulebreaker_color:` key would supersede this.** If a future clause ever
  makes the choice observable in play, deriving it stops being equivalent and
  the field becomes right. Until then it is a second copy of a derivable fact.
