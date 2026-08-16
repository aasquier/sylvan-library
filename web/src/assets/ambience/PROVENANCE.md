# Ambience assets — provenance

The rule here is the tarot deck's (`src/mtglab/assets/tarot/PROVENANCE.md`):
nothing ships in this directory whose licence was not checked per file, and
every transformation applied is written down so the derivation is
reproducible from the source.

## ivy-canopy.webp, ivy-sprig-1.webp, ivy-sprig-2.webp, ivy-sprig-3.webp

- **Source**: "Free ivy leaves cascading gray" — rawpixel,
  <https://www.rawpixel.com/image/5909908/image-texture-public-domain-leaves>,
  found via Openverse with a `license=cc0` filter.
- **Licence**: CC0 1.0 (public domain dedication). Confirmed on the source
  page and in the Openverse record at fetch time (2026-08-15). CC0 requires
  no attribution; the source is named here anyway, because provenance is for
  the next maintainer, not for the licence.
- **Transformations** (Pillow, scripted):
  - Cropped to the cascading foliage band at the top of the photograph.
  - Matted to alpha by green-dominance segmentation (`G − max(R, B)`,
    softly thresholded), with the sparse lower-wall growth faded out below
    42% of frame height.
  - Alpha feathered (1.2px Gaussian) and re-steepened, so leaf edges stay
    leaf-shaped.
  - Mirror-tiled horizontally (image + its flip) so `repeat-x` is seamless
    at any viewport width. 2048×250.
  - The three sprigs are crops of the same matte, chosen by alpha-density
    scoring (dense centre, sparse border): self-contained clusters that
    read as something a vine would drop.
  - Encoded WebP, quality 82.

Why committed rather than hotlinked: Scryfall art is hotlinked because the
Fan Content Policy covers *display* and the credit line is part of the deal;
this photograph is CC0, so committing the derived asset is clean, and a
decoration that breaks when a third-party CDN moves is a decoration the app
does not own.
