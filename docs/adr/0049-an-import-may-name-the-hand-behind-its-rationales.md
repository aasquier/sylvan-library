# 49. An import may name the hand behind its rationales

**Status:** Accepted · **Decided:** 2026-08-29 with Aaron · Extends
[ADR 41](0041-the-intake-is-asked-for-and-the-hand-is-named.md)'s mark to
reasons that arrive *with* the paste, rather than being drafted after it.

## Context

ADR 41 recorded the failure this exists to close: a rationale composed by a
model and delivered through the import's quoted column is indistinguishable
from the owner's own words, and will read as theirs in six months. When that
ADR landed, the only door for a drafted `why` was `deckedit.DraftRationale`,
which writes `why_by: claude` as it drafts — so the mark and the drafting
were one motion, and a paste could not need the mark because a paste was, by
definition, a person's writing.

Then the `/scry` intake council was built: a Claude Code skill that takes one
of Aaron's Moxfield lists, convenes six adversarial research personas, and
hands back a full 99 in the import grammar — every card's `why` already
written, in the drafting voice ADR 41 prescribes (what the card does in this
deck, never a case that it deserves the slot). Aaron reviews the contested
calls and the final blob, and pastes it himself. The sentences are
co-authored at best; unmarked, the file would claim a person wrote every one
of them. `why_by` is not a field anybody types and the grammar has no slot
for it, so without a change the honest path was to discard the council's
researched rationales and have the site's intake redraft ninety-nine worse
ones, marked.

## Decision

One optional field on `POST /api/decks/import`: `why_by`.

- Its only legal value is `deckedit.DraftedBy` (`"claude"`). Anything else is
  refused with a 422 before the parse — one hand ever drafts, so one value is
  ever legal, and any other name is a claim the deck file has no way to
  record.
- When set, every card whose `why` actually rode the paste — in the 99 and on
  the swap board alike — is written `why_by: claude`. A card whose reason is
  still owed takes no mark: a mark on an empty `why` would claim authorship
  of a sentence nobody wrote.
- A reason on the commander's own line still lands in `notes.command_zone`,
  where no mark can travel; when the paste declared a hand, the import report
  says out loud that the sentence was drafted rather than hand-written.
- The import page carries the declaration as a per-import toggle, off by
  default and never remembered, beside the quoted-column explainer. Absent
  and empty mean the same thing on the wire; the page omits the field rather
  than asking the question on every ordinary import.

## What does not move

- **`DraftRationale` remains the only door that *drafts*.** This field never
  writes a word — it names the hand behind words the paste already carries.
  ADR 41's two gates guard drafting, and nothing here drafts, so nothing here
  needs them.
- **The mark's exit is unchanged.** `SetCardField` drops `why_by` the moment
  a person rewrites the sentence, exactly as ADR 41 built it, and the mark
  still satisfies promotion — a filled `why` is a filled `why` (Aaron,
  2026-08-28).
- **ADR 25 is untouched.** The council's rationales are written in the
  descriptive voice, and the slot argument still has no field for a defence.
- **The bulk-edit door** (`POST .../bulk`) does not take the field yet. A
  re-list of an existing deck through the council — the tune-up — is real and
  future, and it should get this field the day it exists rather than a
  speculative one now.
