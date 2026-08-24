# History

Why things are, never what is — and as of 2026-08-24, deliberately short.

The long-form build narrative that lived here (the phase-by-phase account,
and the landed sections of the original hosting guide, kept under their old
section numbers §§1–3 and §§6–7 for the ADRs that cite them) served its
purpose and now lives in git history:

```bash
git log --oneline -- docs/HISTORY.md
```

Any older revision renders with `git show <sha>:docs/HISTORY.md`. The
durable *why* — every decision with consequences — is `docs/adr/`, which is
immutable once accepted: supersede, never edit. Everything current lives in
`CLAUDE.md` (the rules), `ROADMAP.md` (direction), and `docs/HOSTING.md`
(the runbook).
