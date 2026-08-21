# The Go migration

The plan and the ledger for moving the **served backend** from Python to Go —
decided with Aaron 2026-08-21, superseding the "compiled backend, deferred"
position in `docs/ENGINEERING.md` §1 (that section answered a *different*
question; see PLAN.md §1).

Read in this order:

1. **[PLAN.md](PLAN.md)** — scope, the honest case, the strangler mechanism,
   the phases, the risk register, and the decisions Aaron owns. **Status:
   ratified 2026-08-21** — the seven §11 rulings are recorded inline and
   ADR 38 is in `docs/adr/`. **Phases 0–2 are done** (the contract harness,
   then the Go front door in front of both runtimes); **§10 holds the port
   board**, which is the frontier a fresh session reads first.
2. **[BASELINE.md](BASELINE.md)** — the Python side measured, 2026-08-21.
   The numbers the finished Go backend is compared against. **Append-only**:
   a re-measurement gets a new dated block, never an edit, the same rule
   `docs/polish/LEDGER.md` lives by.

Two rules for this folder:

- **The ADR is not here.** ADR 38 is in `docs/adr/` (landed with the
  Phase 1 branch, per the no-doc-only-PR rule); PLAN.md's Appendix A is the
  record of its draft. The Go code is in `go/` at the repository root.
- **The comparison is the deliverable.** Aaron asked for Python-vs-Go numbers
  when the port is done. BASELINE.md is the "before"; PLAN.md appendix B is
  the empty "after" table waiting to be filled. A migration that cannot show
  its comparison table is not finished (Phase 8 exit gate).
