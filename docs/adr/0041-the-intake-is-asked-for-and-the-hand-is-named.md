# 41. The intake is asked for, and the hand that wrote it is named

**Status:** Accepted · **Decided:** 2026-08-28 · **Recorded:** 2026-08-28

Narrows [ADR 15](0015-claude-surfaces-are-modes-with-capabilities.md), whose
title is *no stance may write a rationale*, in one case: an imported deck,
where the person doing the importing asks for it, and their stance permits a
write at all. It leaves ADR 15's mechanism entirely intact — see *What does
not move* — and it changes what
[ADR 13](0013-an-imported-deck-is-a-draft.md) means by a draft that is ready.

This ADR was drafted on 2026-08-24, discarded on 2026-08-25 while clearing
stale files off `main`, and rebuilt from memory today. The design below is the
one that was drafted. The question it left open is answered at the bottom, and
the answer is not the one the draft recommended.

## Context

An imported deck arrives empty of everything except its cards. Ninety-nine
rationales owed, no themes, no strategy, no dossier, and every card that is not
a land filed under `Utility` because
[ADR 13](0013-an-imported-deck-is-a-draft.md) leaves the filing to a human on
the grounds that a guessed category ends up asserted in a generated primer.

That is the correct state for a list nobody has thought about. It is a
punishing state for the person who just pasted one, and the punishment lands
hardest on exactly the person commandment 2 is about — somebody importing the
one deck they own, from the one app they use, to see what this site does with
it. What this site does with it, today, is show them a list of ninety-nine
things they owe.

[PR #391](https://github.com/aasquier/sylvan-library/pull/391) took the first
bite out of that: a pasted line may now carry its own quoted reason, and that
reason becomes the card's `why` verbatim. It narrows nothing, because the
words are the person's own. It also only helps somebody who has already
written ninety-nine sentences somewhere, which is not the person above.

So the question is what the site may offer to do for them, and by whose hand.

## What is being asked for

Five things, all of them off by default, chosen on the import screen before
the deck is created. Aaron's list, in his words, with what each one actually is:

| | Asked for | What it is | Status before this ADR |
|---|---|---|---|
| 1 | Claude assisted descriptions of cards | drafts the `why` on every blank card | **forbidden** by ADR 15 rule 1 |
| 2 | Dividing cards into their macro categories | files each card under one of the thirteen | allowed; nothing does it |
| 3 | Claude arguing macro categories or best-in-slots | the existing slot-argument sweep, run at intake | allowed, [ADR 25](0025-argue-a-slot-argues-one-direction.md) |
| 4 | Dossier built on intake for commander | the existing commander dossier | allowed, [ADR 19](0019-the-dossier-cites-three-sources.md) |
| 5 | Deck description | themes and strategy from the deck's own contents | allowed |

Four of the five are inside today's rules and are only unbuilt. **The first one
is the whole decision**, and the table above is the reason this ADR exists
rather than a pull request: a menu where four items are plumbing and one is a
rule change should not have the rule change decided by whoever builds the menu.

## The line, and where it actually is

ADR 15 states the rule as an assertion about code:

> no code path passes a model response into `set_card_field(field="why")`

`internal/claude/boundary_test.go` is that assertion. It is worth being precise
about what it proves, because the claim is repo-wide and the enforcement is
not: it loads `internal/claude/...` and fails if **that tree** imports the
write engine, transitively, or names any function on the write surface. It
resolves identifiers through the type checker, so an alias or an intermediary
is not a way past it. Within its scope it is airtight.

Its scope is the Claude tree. A caller outside that tree — the API, a job —
has always been able to take a model's answer and write it, and nothing in the
code would object. What stopped it was that nothing asked for it.

This matters because it means there are two different things that could be
changed, and only one of them should be:

- **The model surface.** Give a mode `may_write: ["why"]`. `NewMode` refuses
  this outright, with an error that says changing it needs a new ADR
  superseding 15. Doing it would mean Claude writes decks.
- **The caller.** Leave every mode read-only, and let the intake — ordinary Go,
  outside the Claude tree, with the user's instruction in hand — write what a
  mode drafted.

**This ADR changes the caller and not the surface.** Claude still only ever
answers. The thing that writes a deck is the same `deckedit` call that a person
clicking *Save* uses, and the sentence it writes came back in a JSON field
rather than from a text box.

That is not a loophole being exploited; it is the distinction ADR 15 was
drawing all along. ADR 15's slope was *authorship* — the user's words, lightly
edited, then not the user's words. What it was protecting against was a mode
that quietly tidies a rationale nobody noticed it touched. A field the user
asked to have drafted, marked as drafted, in a deck that says so, is not that.

## What does not move

Everything ADR 15 built stays exactly as it is, and this is the load-bearing
half of the decision:

- **Every mode keeps `may_write: []`**, and `NewMode` keeps refusing anything
  else. There is no mode that writes.
- **`boundary_test.go` is not touched, weakened, or scoped down.** The Claude
  tree still cannot reach `deckedit`, `library`'s writers or `decklog.Record`,
  directly or through six intermediaries.
- **The interview keeps returning only questions.** `rationale-interview` is
  untouched: it is the surface for somebody who wants to write their own, and
  drafting is a different surface with a different name.
- **[ADR 25](0025-argue-a-slot-argues-one-direction.md) is untouched.** The
  slot argument still has no field for a defence. A drafted `why` and a case
  *for* a card are different objects and stay different: one is a description
  of what a card is doing in this deck, the other is an argument that it
  deserves the slot, and merging them is how the asymmetry ADR 25 exists to
  protect would be lost by the back door.

## The two gates

A rationale is drafted only when both of these are true, and they are
deliberately different kinds of permission:

1. **The user asked, for this deck, on this import.** A toggle, off by default,
   on the screen where the deck is created. Not a preference, not a default,
   not a setting that persists into the next import.
2. **Their stance's write axis is above `none`.** This axis has existed since
   ADR 15 with exactly the three levels this needs — `none`, `proposes`,
   `applies` — and **no mode has ever used one**. It was built for this and has
   been sitting empty. The `claude` seat is `Collaborator`, which is
   `proposes`: a batch to approve in one go, not a write that already happened.

Gate 2 is what makes the toggle honest. Somebody at stance `Off` does not get
a toggle that quietly does nothing; they get no toggle, because their stance
already answered the question.

## The mark

**Every field a model drafted is written `why_by: claude` in the deck file, and
stays marked until a person rewrites it.**

The deck file is the truth. A rationale whose author is unrecorded is a
rationale that will be read as the owner's in six months, by the owner. The
mark is not a warning label and not a disclaimer — it is the provenance of a
sentence, kept where the sentence is.

It comes off the moment the text changes. Editing a drafted rationale is
adopting it, and the person who edited it is its author from that point.

## The question the draft left open

> Does a Claude-drafted rationale satisfy promotion to `curated`?

The draft recommended **no**, and the argument was: `curated` is the one claim
this library makes about thinking, and a deck that promotes itself in three
minutes makes the word mean *the fields are non-empty*.

**Aaron answered yes, on 2026-08-28: a filled `why` is a filled `why`.**

The cost is real and is recorded here rather than argued away. `curated` now
means every card has a reason attached to it, and not that a person formed one.
The `why_by: claude` mark is what carries the difference, so the distinction
survives — it moves from the gate, where it would have been enforced, to the
deck file, where it is stated. Anybody reading the deck can see which sentences
were drafted; the gate no longer stops the promotion over it.

Two things follow, and both are consequences of the answer rather than
softenings of it. The mark is now the *only* thing carrying that distinction,
which raises it from a nicety to a load-bearing invariant — a drafted field
that loses its mark is a real defect, and gets a test rather than a comment.
And the deck page shows the count, because a claim visible only inside a YAML
file is not visible.

## Options considered

**Refuse the whole idea and keep rule 1 absolute.** Coherent, and it was the
status quo for a reason. Rejected because the thing it protects — that a
rationale is somebody's actual thinking — is not protected by leaving
ninety-nine of them blank. An empty field is not a more honest field. The
person who imports a deck and never writes a `why` does not end up with a
deck of considered choices; they end up with a draft they abandoned, which is
what the surface produces today.

**Let the mode write, with `may_write: ["why"]`.** Simpler in the code by some
distance: no separate write step, no caller-side stitching. Rejected because it
makes the assertion in `boundary_test.go` false, and that test is worth more
than the convenience — it is the one guard here written *before* the thing it
guards, and its whole value is that it never had an exception in it.

**Draft into a review queue and never into the file.** The user reads
ninety-nine drafts and accepts them one at a time. This is what stance
`proposes` means, and it is genuinely better for a careful user. Rejected as
the *only* option because ninety-nine one-at-a-time confirmations is a worse
experience than the blank fields it replaces, and because the stance axis
already expresses the difference: `proposes` gets the queue, `applies` gets the
write. Building only the queue would leave `applies` meaning nothing, again.

**A deterministic first pass — file categories by type line, draft a `why`
from oracle text.** No model, no rule change, no cost. Rejected because it is
the worst of both: a sentence assembled from a template is not a rationale and
reads as one, and it would carry no mark distinguishing it from a person's
because there would be nobody to name. ADR 13's objection to guessed categories
is exactly this, and it applies with more force to prose.

## Decision

1. The import screen offers five intake actions, all off by default, chosen
   per import.
2. Actions 2 to 5 are built against today's rules and need nothing from this
   ADR beyond being built.
3. Action 1 — drafting a `why` — is permitted when the user asks for it **and**
   their stance's write axis is above `none`, and not otherwise.
4. No mode gains write scope. `may_write` stays empty everywhere, `NewMode`
   keeps refusing, and `boundary_test.go` keeps passing unchanged. The intake
   writes; Claude answers.
5. Every drafted field is marked `why_by: claude` in the deck file and stays
   marked until its text changes.
6. A drafted rationale satisfies promotion to `curated` (Aaron, 2026-08-28).
   The mark, not the gate, carries the difference.

## Consequences

- **`deck.CardEntry` gains `why_by`**, so the emitter, the round trip and the
  deck wire all carry it. Existing deck files have no such key and are
  unaffected: absent means a person wrote it, which is true of every rationale
  in the library today.
- **[ADR 28](0028-the-activity-log-records-edits-and-never-rationales.md) gains
  an edit kind** for the intake write. It records that a draft happened and for
  which cards, and — unchanged, and the reason ADR 28 exists — never the text.
- **The About Claude page becomes false.** It says *"I never write your reasons
  for you"*, and after this it does. That page is Claude's room under
  commandment 18, so Claude rewrites the line rather than anybody correcting it
  on Claude's behalf.
- **A new failure mode exists that did not before**: a deck full of confident
  sentences nobody has read. The mark, the count on the deck page and the
  interview all point the same way — at a person eventually replacing them —
  and none of them forces it. That is the trade this ADR makes, stated plainly
  so that a later ADR reversing it has something to argue with.
