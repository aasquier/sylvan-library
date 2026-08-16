# 32. Card-art motion is derived at runtime, and never committed

**Status:** Accepted · **Decided:** 2026-08-16 · Refines rule 3 of
[ADR 29](0029-an-asset-is-committed-only-with-a-recipe.md) without
superseding it.

## Context

The motion phase's ambition is "pictures brought to life", and the pictures
people actually look at in this app are Wizards' card paintings. ADR 29
rule 3 says: *"Wizards' art is animated at runtime only, never baked into a
committed file."* When that sentence was written, "runtime" meant CSS — the
wheel's clip-and-rotate, the backdrop's blur — because the only alternative
imagined was a committed sprite sheet or video, which rules 5 and ADR 6
already forbid.

Depth parallax broke the dichotomy. The effect needs a depth map inferred
by a model and a loop encoded by ffmpeg — neither can happen in a browser
frame budget, and neither output may land in git. But there is a third
place, and every heavy thing this project owns already lives there: the
gitignored `data/` tree, the volume when deployed. The pool is downloaded
there, `app.db` lives there, the sim cache and the dossier cache grow
there. None of it is committed, none of it ships in the image, and all of
it is the app's working data.

The licensing ground was re-checked rather than inherited, this session,
against the primary sources. The Fan Content Policy permits free fan
content built on Wizards' IP (the required disclaimer already renders in
the app's footer) and forbids *"verbatim copying and reposting"* — bulk
redistribution, which a public git repository would be and a private
server-side cache is not. Scryfall's imagery guidelines constrain the
*kind* of presentation: no distortion, blur, desaturation or colour-shift
of card imagery, and artist attribution rendered in the interface. The
community norm agrees with the repo half: Forge, XMage and Cockatrice all
deliberately ship no card images and fetch at runtime.

## Options considered

**Client-side only (CSS/WebGL on the hotlinked still).** The status quo,
kept for everything it can do. Rejected as the *whole* answer because the
marquee effect cannot be computed in the client: depth inference is a model
and a loop is an encoder.

**Commit derivatives with a Scryfall provider in the animist.** Rejected
without much argument needed: it is the exact thing ADR 29 built the
pipeline to structurally refuse, and the refusal was right. A committed
derivative is redistribution.

**Generate on the instance, on demand.** Deferred, not designed out. The
deployed machine is a 1GB shared CPU whose comment already records that
512MB was too tight without torch anywhere near it; on-demand generation
would also put ffmpeg and Pillow in the image for a job that runs a handful
of times ever. The route layer is shaped so a job-backed generation path
could appear behind the same status endpoint; v1 ships without it.

**Derive on the dev machine, cache on the volume, serve from the app.**
Chosen. Generation is a documented run — the pool's own pattern — and the
app's only new capability is serving files off its own data tree.

## Decision

- `src/mtglab/cardmotion/` derives card-art motion using the animist's pure
  functions as a library. The animist's *recipes, provenance, licence gate
  and committed outputs* remain closed to Wizards' art; nothing here adds a
  provider, and nothing here writes into an asset directory.
- Derivatives live under `data/cache/cardmotion/<key>/`, keyed like the sim
  cache (ADR 18): the inputs (`oracle_id` and the exact art URL) plus the
  effect's fingerprint (its parameters and a code version). Regeneration is
  a new key; stale entries age out by never being found.
- **The effect vocabulary is bounded by Scryfall's guidelines**: motion and
  parallax (`depth-drift`, `slow-pan`), never distortion, blur or
  colour-shift of the artwork. Adding an effect means re-reading that list.
- **`attribution.json` is written at build time from the pool row** and
  served with every derivative, so the interface can credit the artist
  without the pool being reachable, and the FCP notice travels inside the
  cache entry itself.
- **Torch never enters the container.** The depth model is the `depth`
  extra — CPU torch, pinned exact, weights cached under `data/cache/models/`
  — and is the one extra deliberately *not* vendored into `dev`: no test
  may import it, and the suite drives everything through a `DepthModel`
  Protocol and fakes. Determinism stance: pinned weights, CPU inference;
  bit-drift across torch builds is acceptable because nothing byte-verifies
  a derivative.
- The serving half is two GET routes (`/api/art/motion/...` status and
  file), classified **shared** in the isolation suite: a derivative is
  keyed on a public painting, nothing per-account, behind auth like the
  deck pages it decorates. `ready: false` is a complete answer — the client
  keeps showing the still it already has. Long-lived HTTP caching is safe
  because URLs carry the fingerprint as a version stamp.
- Deployment is `fly ssh sftp put` into `/data/cache/cardmotion/`,
  documented in HOSTING beside the pool seeding it imitates.

## Consequences

- The image grows by zero bytes and the instance runs nothing new — the
   236-second-dossier lesson consulted in advance: there is no long request
  here because there is no request-time work at all.
- A fresh instance (or a cleared cache) simply shows stills everywhere,
  which is the app as it was yesterday — the derivative tier degrades to
  its own absence.
- The dev machine is a single point of generation. Accepted: it already is
  one for the pool refresh and every deploy-adjacent runbook.
- git history stays clean of Wizards' pixels forever, which keeps ADR 30's
  "git holds the tool, never the toybox" true without exceptions — and
  keeps this repository the kind of citizen the Fan Content Policy's
  "verbatim copying" clause is aimed away from.
- The one rule that must survive future convenience: **no code path may
  write a card-art derivative into a committed directory.** The animist's
  glob-driven `verify` would fail on an unrecipe'd file, which is the
  structural tripwire, and this sentence is the intent behind it.
