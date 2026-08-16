# 31. Motion is seeded, and a procedural source is its own declaration

**Status:** Accepted · **Decided:** 2026-08-16

## Context

ADR 29 built the animist for stills and named its own later phases:
"procedural textures, animated WebP/APNG, video loops, depth maps are
registry entries and format-table rows, not schema changes; each arrives
under this ADR's rules or argues a new one." Most of the motion phase is
exactly that — `awebp`, `apng`, `webm` and `mp4` are four rows in
`encode.py`'s table, and `expect.frames` / `expect.fps` are additive fields
`verify` checks when present. Two things, however, genuinely strained the
schema, and this record is the argument for how they were resolved.

**First: `ops.py` promises "nothing stochastic lives here."** Procedural
noise — the whole point of generating backgrounds instead of fetching them —
is stochastic on its face, and a naive implementation (`np.random` seeded
from the clock) would break the promise that lets `verify` hold a committed
file to its recipe.

**Second: the schema requires every output to draw from a declared source,
and every source to be fetched and licence-gated.** A generated texture has
no upstream file, nothing to download, and no licence to confirm. The
"≥1 source" rule was written so no asset could appear from nowhere; a
procedural asset *is* from nowhere, on purpose.

## Options considered

**Let procedural outputs skip the `sources` block.** Rejected. It makes
"where did this come from" have two answer shapes, provenance rendering
grows a special case, and the licence gate acquires a class of asset it
never sees — the first exception to "the gate runs on everything" would not
be the last.

**A second pipeline beside the animist for generated assets.** Rejected for
ADR 29's own reason: two pipelines is two provenance formats, two verify
commands, and a question ("which one does this asset use?") nobody should
have to answer.

**A `procedural` provider whose declaration is the source.** Chosen. The
source block stays mandatory; what changes is what it contains — a `seed`
instead of an upstream identifier, and the one licence a thing with no
upstream can carry, `ours-generated`. The gate still runs and still returns
a dated confirmation; what it records is that there was nothing to ask
anybody, which is itself a fact about where the asset came from.

For the stochasticity: **every stochastic op takes an explicit integer seed
and is a pure function of it.** `ops.py`'s sentence was protecting
determinism, not banning randomness — a seeded generator is exactly as
deterministic as `resize`, just with one more parameter. The loader refuses
a recipe whose stochastic op has no seed to inherit, so the derivation a
recipe records is always the complete one.

## Decision

- `motion.py` owns a second shape, `FrameSequence`, and two new protocols
  over it: a **generator** (`params -> frames`; only `spectral_noise` today)
  and a **motion op** (`frames -> frames`; `advect`, `color_ramp`,
  `ken_burns`). Still ops are not duplicated — `run.py` lifts them over a
  sequence frame by frame. The registries get schema copies in `recipe.py`
  (`KNOWN_GENERATOR_OPS`, `KNOWN_MOTION_OPS`, `KNOWN_SEEDED_OPS`), pinned
  equal by test, because the schema module deliberately cannot import the
  implementations.
- **Loop-perfection is by construction, not post-processing.**
  `spectral_noise` is synthesised in the frequency domain over integer cycle
  counts, so it is periodic in x, y and t; `advect`'s flow field uses the
  same machinery, so periodicity survives it. There is no crossfade op to
  hide a seam because there is no seam.
- A `procedural` source requires `seed` and `licence: ours-generated`; a
  procedural output must start with a generator op and be a `file:` output;
  a generator op on a fetched source is refused (it would discard the image
  the gate just confirmed). A seeded op inherits the procedural source's
  seed unless its params carry their own.
- The format table gains `awebp` and `apng` (Pillow, no new dependency) and
  `webm` (VP9) / `mp4` (H.264) through **imageio-ffmpeg**, whose bundled
  ffmpeg is a build-time subprocess — dev machine and CI only, never the
  image; NOTICE.md carries the licence argument. Video rate control is
  `crf`, and the loader requires exactly the knob the format reads. The
  frame rate is deliberately **not** an encode field: it belongs to the op
  that created the timeline, and the encoder reads it off the sequence.
- Dual-format shipping (a `webm` and its `mp4` twin, a loop and its poster)
  is two outputs with the same source and ops; `run.build` memoises the
  derivation on that pair so generation runs once. No schema change.
- Video files cannot be opened by Pillow, so `verify` and `has_metadata`
  grew format-aware branches that read through the same bundled ffmpeg that
  wrote the file — encode-time check and verify-time check still cannot
  disagree. `-map_metadata -1` strips at encode; the check dumps ffmetadata
  and holds it to a structural allowlist (a muxer's own encoder tag is
  container furniture, the same ruling as WebP's `background`/`loop`).

## Consequences

- `verify`'s promise is unchanged in kind: contract, not bytes. A libvpx or
  x264 release re-encoding differently never fails CI; a committed loop
  whose frame count, rate, dimensions or budget drift from the recipe does.
- The suite encodes real video — tiny sequences, sub-second — so
  `imageio-ffmpeg` is vendored into `dev` beside Pillow, and the pinned
  skip count stays at 2. The container installs neither; zero image growth.
- `mtglab animist measure` gains a second axis: for a video output the
  sweep is over `crf` rather than width, and the knee finder is shared
  because its arithmetic never asked what the x-axis meant.
- A recipe is now the complete record of a generated asset: seed, ops,
  parameters, budgets. Deleting every committed loop and rebuilding from
  recipes alone must reproduce them up to encoder drift — which is the same
  sentence ADR 29 could already say about the ivy, now true of assets that
  never had an upstream at all.
- Depth maps — the one later phase 29 named that this ADR does not build —
  arrive with ADR 32's runtime tier, where the model that infers them lives
  behind its own extra and never enters this registry's purity contract.
