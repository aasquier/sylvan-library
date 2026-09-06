# 50. The crossing's working documents live in git history

**Status:** Accepted · **Decided:** 2026-09-05 with Aaron (the daybreak
queue's 2026-08-23 item, answered yes) · **Recorded:** 2026-09-05 ·
Supersedes the `docs/go-migration/` pointers in
[ADR 38](0038-the-served-backend-is-rewritten-in-go.md) — the pointers only,
never the decision, which stands whole.

## Context

ADR 38's header sends its reader to `docs/go-migration/` as home of "the
plan, the measured baseline and the seven rulings", and its Consequences
lean on the same documents by bare name — PLAN Appendix B and PLAN §8, and
`BASELINE.md` as the retirement comparison's "before" column. (The reading
guide in this directory's `README.md` carried one more link; an index is
not immutable, so that one is simply repaired.) The directory is gone. When
the crossing ended, the zero-trace ruling that retired the interpreter —
#272, "The interpreter departs", 2026-08-23 — deliberately removed the
working documents with it, for the same reason `docs/HISTORY.md` was cut
short the next day: a working document that outlives its work sits in the
tree claiming to be current forever. ADRs are immutable once accepted, so
the dead pointers cannot be repaired in place, and every future reader of
ADR 38 hits them.

## Options considered

**Restore the directory as an appendix.** Rejected: the crossing is over, and
a restored plan would stand beside the tree wearing a currency it lost the
day the migration closed. The zero-trace ruling made this trade knowingly.

**Accept the dead link.** Rejected: an accepted record pointing at nothing is
uncheckable exactly where it says "here is the evidence", and this directory
exists to be re-verified.

**A short superseding note — this file.** Accepted, as the smallest honest
form: only the pointers needed a forwarding address.

## Decision

The working documents — `README.md`, `PLAN.md`, `BASELINE.md` — were removed
deliberately, not lost. Their content is git history, readable at any
revision before the removal:

```bash
git show 90158d6^:docs/go-migration/PLAN.md   # likewise README.md, BASELINE.md
```

Everything durable in them was already in ADR 38 itself when they were
deleted — the options, the toolchain, the equivalence gates and their
consequences — which is what let the directory go.

## Consequences

- ADR 38's cites resolve here rather than nowhere, and the index in
  `README.md` points both ways.
- The long-form reasoning is one `git show` away. A reader who needs more
  than ADR 38 records is reading history, which is where a finished
  migration's plan belongs.
