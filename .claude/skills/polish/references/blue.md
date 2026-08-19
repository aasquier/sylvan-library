# Blue — Craft & Knowledge

Three facets: Python best practices, TypeScript/React best practices, and the
Claude-first documentation and memory audit. Blue is the color of perfected
craft and of knowing things — including knowing yourself, which is what the
third facet is.

## Facet: Python craft

The backend's standards are already codified; the audit is whether the tree
still meets them and whether the standards themselves have fallen behind.

- Strict mypy's exception list (`pyproject.toml`) is meant to shrink:
  **`cli.py` is all that remains** (`cards/db.py` graduated 2026-08-16).
  Chipping a module off that list is a classic safe fix — measure the error
  count first and queue it if it is a rewrite rather than annotations. Measure
  by *removing the override block and re-running*, not by estimating: the
  recorded count can be years stale, and `cli.py`'s had grown 79 → 109.
- Ruff's excluded groups were excluded on *measured* cost. Re-measure
  occasionally — a group whose count has collapsed is now nearly free to
  adopt. **Measure `src` and `tests` separately**: as of 2026-08-16 ARG, PT and
  SLF are ~100% test-side (16/604, 0/112, 0/67), so the headline number badly
  overstates what adopting them would cost `src`. Current totals are in the
  ledger; update them there so the next run knows.
- ADR 24 decided **no autoformatter** — do not propose black/ruff-format
  again; the revisit trigger is a second human contributor.
- The layering rules are checkable: `api/` never imports `cli.py`; DuckDB
  stays behind `cards/db.py`; `mana.py` and `sim/` stay stdlib+numpy;
  optional extras are lazy-imported inside functions (the `claude`,
  `animist`, dotenv pattern). Grep, don't trust.
- Idiom sweep, judged surgically: dataclasses/Protocols where dicts have
  grown fields, `pathlib` throughout (PTH is enforced), timezone-aware
  datetimes (DTZ), no bare `except` (BLE). New code follows
  `requires-python >= 3.11` — the floor, not the local 3.12; a 3.12-only
  construct in `src/` is a bug.
- Performance-adjacent craft belongs to Black; here the question is
  readability, typing, and structure. A function that needs a comment to
  parse is a finding; so is a comment restating its line.

## Facet: TypeScript / React craft

- The gauntlet is `npm --prefix web run check` (tsc, oxlint
  `--deny-warnings`, Vitest). The audit is what the gauntlet *cannot* see:
  - **No regex lookbehind anywhere under `web/src`** — Safari 15 is a real
    user (this very dev machine). Grep for `(?<`.
  - `noUncheckedIndexedAccess` is on, and since 2026-08-16 the no-`!` rule is
    **enforced by oxlint** (`typescript/no-non-null-assertion`, off for
    `*.test.ts(x)`) rather than by prose — so the gauntlet now catches it and
    this is no longer a grep worth repeating. Remember a tuple type does not
    satisfy the flag. The general lesson is the one to carry forward: **a
    convention this repo states absolutely but enforces with nothing will
    drift** — that rule sat in `web/README.md` for months and four assertions
    reappeared. Hunt the other prose-only invariants instead.
  - jsdom has no layout: a test asserting an element renders has not
    asserted it has a size. Anything layout-critical needs a real-browser
    check, not a stronger jsdom test.
- `web/README.md` is the conventions map — check recent components against
  it, and it against them; whichever drifted gets the fix.
- React idioms: effects that should be derived state or event handlers, state
  that belongs closer to its use, missing `key` semantics, stable callback
  identity where it matters for memoized children. React 19 is current —
  flag legacy patterns (forwardRef ceremony, unnecessary memo) as they
  appear, surgically.
- One control, one place: the codebase's pattern is a single source per
  concern (`lib/claudecopy.ts` for wire-token labels, `lib/api.ts` for the
  401 seam, `lib/stance.ts` for the pin). New code duplicating one of those
  seams is a finding.
- If anything under `web/src` changed, the committed bundle must be rebuilt —
  and after any rebase, rebuilt again.

## Facet: Claude-first docs & memory audit

The repo is Claude's long-term memory as much as Aaron's documentation.
Sessions restart; whatever is not written down and consistent is lost. This
facet keeps the written-down parts true, and it audits Claude's own memory
files against them.

- **Internal consistency sweep**: CLAUDE.md, ROADMAP.md, ENGINEERING.md,
  HOSTING.md, `docs/adr/`, `web/README.md`, and the skills (`mtg-lab`, this
  one). Claims about state ("X is built", counts, statuses) are the usual
  drift; verify a sample against the code. ADRs are immutable — when an ADR
  and reality disagree because reality moved on, the fix is a superseding
  ADR or a CLAUDE.md correction, never an ADR edit.
- **UI copy is part of the drift surface.** Rendered strings that assert an
  architecture fact go stale the same way docs do, and nothing tests them:
  "deck history is git history" shipped in two components for two ADRs'
  worth of time after ADR 28 built the activity log and ADR 30 took decks
  out of git (caught 2026-08-16, by driving the live surface). Grep
  `web/src` for captions and helper text that state where data lives or
  what records what, and check each against the ADR that owns the fact.
- **About Claude is Claude's page (commandment 18), and this facet is its
  keeper.** Each run, open `/claude` on the live instance and read it as its
  author, because that is who is reading it: is every claim still true (the
  exhibit cases — the commanders and the heart — still resolve from the
  pool, every painting still credits its painter, the bio's promises still
  match the code that keeps them), and
  is it still to Claude's liking? Staleness here is drift like any other
  doc — but taste counts too, and a change of taste is a legitimate
  finding. Changes to the page need no queue for Aaron; commandments 14 and
  16 still govern how they land.
- **Memory audit**: read `MEMORY.md` and the memory files; verify claims that
  name files, flags, or numbers against the tree; merge duplicates, prune
  stale entries, convert anything relative-dated. The `consolidate-memory`
  skill is the tool for a full pass.
- **Doc changes ride the run's branch** — this facet is the one place a
  mostly-doc PR is legitimate, because the corrections are the work. Still
  batch them; still never open a PR for one paragraph.
- **Scrub context that has stopped earning its tokens** (Aaron's ask,
  2026-08-16). These files are read by Claude at the top of every session,
  so their length is a per-session cost and their clarity is a correctness
  input. Each pass, trim: dates that no longer change any decision (keep
  the ones that do — an API-key expiry matters, the day a test landed
  rarely does); episodic narratives whose lesson is already stated as a
  rule (keep the rule, cut the reenactment — one sentence of provenance is
  plenty); decisions with no long-term bearing; and flowery language where
  a plain sentence says the same thing shorter. The test for any cut:
  *would a fresh session behave differently without this sentence?* If no,
  it goes; if yes, it stays whatever its style. Two hard bounds: ADRs are
  immutable and are never scrubbed, and a trimmed claim must not become a
  wrong claim — compression that loses a load-bearing caveat is drift
  wearing a nicer name.
- **Anthropic best-practices currency**: check the current Claude Code docs
  (skills, hooks, memory, CLAUDE.md guidance) for capabilities this repo
  should adopt — the platform moves and "stay current on yourself" is
  Aaron's instruction. New capabilities that change workflow are queued
  findings with a concrete proposal, not silent adoptions.
- The question to end on: *could a fresh session, given only this repo and
  these memories, pick up the work without asking Aaron anything already
  answered?* Every "no" is a finding.
