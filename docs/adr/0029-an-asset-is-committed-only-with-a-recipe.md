# An asset is committed only with a recipe, and the licence gate has no override

**Status:** Accepted — decided and built 2026-08-16.

## Context

Commandments 5 and 6 want photo-real, living imagery, and ROADMAP item 12 had
already decided the method: no third-party animation software, real images —
Scryfall hotlinks for Wizards' art, CC0 photography committed as assets when a
thing must be ours — and "scripted Pillow" as the pipeline.

Two problems had accumulated under that decision by the time this ADR was
written:

1. **The scripts were prose.** Both `PROVENANCE.md` files described a
   pipeline — the tarot fetch with its `RWS1909` filename guard, the ivy
   matte with its `G − max(R, B)` segmentation — and neither script was ever
   committed. Pillow was not even a declared dependency. The reproducibility
   the provenance files claimed ("every transformation applied is written
   down so the derivation is reproducible") was a claim no test could check
   and no future session could run.
2. **The licence check was manual.** "Checked per file through the Commons
   API" was true the day it was done and enforced by nothing afterwards. The
   next asset — added in some later styling session, possibly in a hurry —
   would be checked exactly as carefully as that session happened to be.

Meanwhile the styling roadmap is long (the photo-real pass, and later phases:
motion textures, animated formats, depth parallax), so every future asset
multiplies both problems.

## Options considered

- **Keep ad-hoc scripts, run and discarded.** The status quo. Rejected: it
  is the status quo precisely because scripts written in a session die with
  the session; the two PROVENANCE files are the evidence.
- **A `scripts/` directory at the repo root.** The conventional answer, and
  rejected on measured grounds: `ruff` runs on `src tests`, mypy on
  `src/mtglab`, coverage on `src/mtglab` — a top-level directory is unlinted,
  untyped and uncovered by default, and would ship into the container unless
  `.dockerignore` learned about it. Tooling that enforces a legal boundary
  should not live in the one place the quality gates cannot see.
- **A package module behind an extra** — `src/mtglab/animist/`, Pillow in an
  `animist` extra (and in `dev`, so the boundary tests cannot skip), PIL
  imported inside functions the way `claude/` treats its SDK. Chosen: strict
  mypy from the day it is written, the 95% coverage floor applies, and the
  deployed app never notices the package exists.
- On verification: **byte-reproducible regeneration in CI** (ADR 9's bundle
  discipline) was considered and rejected for assets — WebP output is not
  byte-stable across libwebp versions, so the check would fail on the next
  encoder release with nothing wrong. What is stable is the *contract*:
  dimensions, byte budgets, set counts, metadata absence.

## Decision

**An asset is committed only with a recipe and provenance.** A
`*.recipe.yaml` beside the assets records the source, the licence
expectation, every transformation with its parameters, and an `expect` block
stating the checkable contract. `mtglab animist verify` holds the committed
files to it, and `tests/test_animist_recipes_repo.py` runs that verification
in the suite, on every checkout — the reproducibility claim is now a test.

**The licence gate blocks, and there is no override.** `mtglab animist`
confirms every file's licence through the provider's API (Commons
`extmetadata`, the Openverse record) at fetch time, against a deliberately
tiny allowlist — public domain and CC0 only, because a committed asset is
redistribution and even an attribution obligation is an obligation a
decoration should not carry. A refused source is refused before a byte of it
is transformed, there is no `--force`, and the refusal is a sentence naming
what was found and what would have passed. The gate's pass is a dated
`LicenceConfirmation` that the tool writes into `PROVENANCE.md` itself.

**Wizards' art is animated at runtime only, never baked into a committed
file.** The Wheel of Fortune spins as CSS over a hotlink; that is the
pattern. A sprite sheet, video loop or displacement map derived from a card
painting is a committed derivative of Wizards' art, which rule 5 and ADR 6
already forbid in their own domains; this ADR extends the same line to
everything the pipeline can produce. The pipeline has no provider that
accepts a Scryfall URL, deliberately.

**The prose is never invented.** `why_committed` — the sentence saying why
an asset is committed rather than hotlinked — is required by the schema,
flows verbatim into `PROVENANCE.md`, and has no default. ADR 8's spirit,
applied to pictures: the tool refuses an empty rationale rather than
writing one.

## Consequences

- Pillow is a real dependency at last (`animist` extra, and `dev`), and the
  ROADMAP 12 sentence "scripted Pillow (a dev-only dependency)" is finally
  true in `pyproject.toml` rather than aspirationally.
- The two founding pipelines are reconstructed as committed recipes
  (`web/src/assets/ambience/ambience.recipe.yaml`,
  `src/mtglab/assets/tarot/tarot.recipe.yaml`). Parameters that did not
  survive their sessions — the ivy crop boxes — are honestly absent with a
  comment, not plausibly invented; `expect` pins what is checkable. Neither
  recipe re-fetches: the committed assets are the record.
- `verify` reads and never writes, and checks contract rather than bytes, so
  an encoder upgrade does not break CI but a hand-replaced asset does — the
  metadata check doubles as a tamper tell, since the pipeline strips
  EXIF/XMP/ICC and CI's secret scan cannot read binaries.
- Later phases (procedural textures, animated WebP/APNG, video loops, depth
  maps) are registry entries and format-table rows, not schema changes; each
  arrives under this ADR's rules or argues a new one.
- The cache of originals lives under `data/cache/animist/`, gitignored like
  every other regenerable download (ADR 6's shape).
