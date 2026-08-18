# wheel assets -- provenance

The rule here is the tarot deck's (`src/mtglab/assets/tarot/PROVENANCE.md`): nothing ships in this directory whose licence was not checked per file, and every transformation applied is written down so the derivation is reproducible from the source.

<!-- animist:begin critter -->
## wheel-beetle.webp

- **Source**: "Ground Beetle (Carabidae, Carabus finitimus (Haldeman))", <https://www.flickr.com/photos/131104726@N02/33310032166>, found via Openverse with a license=cc0 filter, searching `ground beetle`; the plate is the Insects Unlocked project's specimen photograph (University of Texas at Austin), public domain by their own dedication
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build critter.recipe.yaml`):
  - `crop`: frac_box=[0.08, 0.005, 0.92, 0.9].
  - `matte_backdrop`: tolerance=24, soft=22, border=2, enclosed='drop'.
  - `resize`: width=160.
  - Encoded WEBP, quality 86.

Why committed rather than hotlinked: CC0 from the University of Texas "Insects Unlocked" project, so the matted cutout is clean to commit -- and a critter that vanishes when a CDN moves is a haunting nobody asked for. A few kilobytes of beetle.
<!-- animist:end critter -->
