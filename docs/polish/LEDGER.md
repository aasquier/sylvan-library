# The Polish Ledger

The memory of the recurring polish pass (`.claude/skills/polish/`). One
section per color; each run updates its color's section **on the branch that
did the work**. Queued findings wait on Aaron and are not re-litigated;
deferred items name the trigger that revives them; measurements are recorded
even when healthy, because today's healthy number is next quarter's baseline.

Facet-to-color map and the run protocol live in the skill. This file holds
state, never checklists.

---

## White — Law & Protection

*Licensing/free-use (triple-checked) · security & isolation · testing discipline*

- **Last run:** 2026-08-16 (first White run, during the skill's own eval).
- **Fixed and landed:** pre-auth path traversal in the SPA catch-all
  (`api/app.py`), which served files resolved from `WEB_DIST / full_path`
  with no containment — an arbitrary out-of-tree read reachable without a
  session. Fixed by serving root files from a name→path dict keyed on the
  request, so no user input reaches the filesystem call. Mutation-verified
  test. Closed the two open CodeQL `py/path-injection` alerts. Landed as
  [#126](https://github.com/aasquier/sylvan-library/pull/126). The CodeQL
  fight (four commits) is written up in `references/white.md`.
- **Queued for Aaron:**
  1. **ReDoS cluster — 6 open CodeQL `py/polynomial-redos` warnings.**
     `auth/users.py` (`EMAIL_RE`, on the unauthenticated claim path) and five
     in `decks/decklist.py` (`_MARKER`, `_BRACKET`, `_PRINTING`, `_QTY`,
     `_HEADER`). Polynomial, low-impact; worst case is slow parsing on a long
     crafted input, never a wrong answer. Not fixed because anchoring six
     patterns is behaviour-sensitive on a load-bearing parser. Suggested
     direction: a max-length bound on the pasted decklist and the email
     *before* the regex runs, whose value is Aaron's call — cheaper and safer
     than rewriting the patterns. Wants a per-pattern test.
  2. **Package licence undeclared in `pyproject.toml`.** `[project]` has no
     `license` key, so the wheel metadata reads as UNKNOWN despite the MIT
     `LICENSE`. One-line change, but the correct form is
     setuptools-version-sensitive and only the `image` CI job can fully verify
     it — so it is queued (land it alone on a watchable branch), not bundled.
- **Deferred:** `pytest-xdist` for parallel tests — a new dev dependency, so
  Aaron's call. Trigger to pick up: a run gathers evidence the suite is
  parallel-safe (shared `data/` state, `config.use_paths` discipline).
- **Measurements (2026-08-16):**
  - `animist verify`: both recipes held (tarot, ambience).
  - Committed media: 78 tarot (`RWS1909`, 1909 Rider PD), 4 ambience ivy (CC0,
    recipe-verified). No hand-placed binary bypassing the pipeline; no Wizards
    art under `git ls-files`. Licence gate: CC0/PD only, no `--force`.
  - No monetization surface; Fan Content + Scryfall attributions present.
  - Dependency licences swept 2026-08-16: Python deps MIT/BSD/Apache/MPL/PSF/
    CC0; npm direct deps MIT except TypeScript (Apache-2.0). No AGPL.
  - Security: cookies `HttpOnly; Secure; SameSite=Lax`; Argon2id at OWASP
    minimum; tokens via URL fragment; email omitted from `User.as_dict()`
    (2 sanctioned callers); SQL parameterized.
  - Testing: full suite ~1920 passed / 2 skipped in ~155s locally; skip gate
    at 2; `data/` not dirtied.

## Blue — Craft & Knowledge

*Python craft · TypeScript/React craft · Claude-first docs & memory*

- **Last run:** never
- **Queued for Aaron:** —
- **Deferred:** —
- **Measurements:** —

## Black — Ruthless Efficiency

*Claude API spend · static assets · performance*

- **Last run:** never
- **Queued for Aaron:** —
- **Deferred:** —
- **Measurements:** —

## Red — Speed & Alarum

*CI/CD · alerting & self-healing*

- **Last run:** never
- **Queued for Aaron:** —
- **Deferred:** —
- **Measurements:** —

## Green — Growth & Resilience

*Browser & mobile compatibility · cloud resource watch · scalability*

- **Last run:** never
- **Queued for Aaron:** —
- **Deferred:** —
- **Measurements:** —
