# 12. Decks are edited by surgical operations over text

**Status:** Accepted · **Decided:** 2026-08-11 · **Recorded:** 2026-08-11

Generalises the mechanism introduced for one operation in
[ADR 11](0011-the-api-may-apply-a-swap.md).

## Context

`decks/edit.py` exists because a swap had to be applied without rewriting the
file. It does exactly one thing: replace one card. Adding a card, removing one,
moving it to another category, changing a quantity and editing a note are all
coming — a deck the user can create and import is a deck they will want to
change — and the shape of that code gets decided by whichever operation is
written second, unless it is decided now.

The constraint that produced the current design has not changed and is worth
restating, because it is unusual and it is the reason this ADR is not simply
"use a YAML library":

**`deck.yaml` is the source of truth, its history is git history, and
`swaps.md` is a diff of it.** So the *size* of an edit is part of its
correctness. An edit that reflows the file destroys the record it exists to
produce.

Measured on the real deck files:

| Approach | Changed lines for a no-op round trip |
| --- | --- |
| `Deck.dump()`, pyyaml | **829** on Goreclaw, and all 8 comments deleted |
| ruamel.yaml round-trip | **6–132**, varying by file |
| Text-surgical | **0** by construction; 2–5 for a real swap |

ruamel preserves comments, which pyyaml cannot, and was the obvious answer
until it was measured. It still reflows folded scalars to its own width, and
these decks were hand-wrapped at several.

## Options considered

**Parse, mutate, dump.** Rejected on the measurements above.

**Adopt ruamel.yaml anyway and accept the churn.** Rejected. 6–132 lines of
unrelated diff per edit makes `swaps.md` useless, which is one of the few
genuinely novel things this project does.

**A structured editor that regenerates only changed regions from the parse
tree.** This is what ruamel is, and it still does not preserve hand-wrapping.
Building a better one is a YAML project, not a Magic project.

**Text-surgical operations, each verifying itself.** Chosen.

## Decision

Editing is a set of small operations with a single shape:

```python
def operation(text: str, *, ...) -> str        # pure, text in, text out
```

Five rules govern all of them.

1. **Touch only the lines that belong to the thing being changed.** Everything
   else — comments, blank lines, other cards, folded scalars, key order —
   survives byte for byte.
2. **Verify the result before returning it.** Parse before and after, and raise
   rather than return if anything other than the intended target moved.
   Silently corrupting the source of truth is the one failure this code must
   not have, so it is checked rather than argued.
3. **Never author a rationale.** No operation may invent, template or infer a
   `why`. It comes from a human or the operation is refused. This is what keeps
   [rule 4](../../CLAUDE.md) and [ADR 8](0008-the-gate-blocks.md) meaningful now
   that something other than a person can edit a deck.
4. **Pure, so composition is free.** Operations take and return text and never
   touch the disk, so a refactor of twenty cards composes twenty calls in memory
   and writes once. There is no partially-applied edit to recover from, and no
   transaction machinery to build.
5. **Persistence belongs to `DeckSource`.** `read_text` and `write_text` are the
   only I/O, so the same operations serve a file today and a database row later
   ([ADR 4](0004-two-embedded-databases.md)).

The operation set, as it is needed rather than up front:

| Operation | Status |
| --- | --- |
| `replace_card` | built |
| `add_card` | planned |
| `remove_card` | planned |
| `set_card_field` — category, qty | planned |
| `set_note` — deck-level prose | planned |

**Creating and importing are not editors.** They write a whole file where none
existed, so there is no formatting to preserve and no reason to be surgical.
They go through the ordinary dumper, and are covered by
[ADR 13](0013-an-imported-deck-is-a-draft.md).

## Consequences

- Every edit stays reviewable as a diff, so `swaps.md` keeps working and
  `git log -p decks/<slug>/deck.yaml` remains a readable record of how a deck
  got to where it is.
- Hand-written formatting in the deck files is now load-bearing. That is a real
  cost: the tests in `tests/test_edit.py` — including one that edits all six
  real deck files and asserts the diff stays small — are what keep it true, and
  they will need extending with every new operation.
- Operations are pure, so they test without a database, without HTTP and
  without a filesystem. That is why the swap refusals run on a fresh clone.
- The verification step means an operation can fail *after* doing the work. That
  is deliberate: a refused edit changes nothing, and a corrupt one would change
  everything.
- Text surgery is more code than `yaml.dump`, and it can be defeated by YAML
  this project does not currently use — anchors, aliases, flow mappings, multiple
  documents. If a deck file ever needs those, this decision needs revisiting
  rather than patching.
