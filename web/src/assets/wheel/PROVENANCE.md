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

<!-- animist:begin menagerie -->
## wheel-croc.webp, wheel-owl.webp

### from `lurker`
- **Source**: "Free crocodile white background image", <https://www.rawpixel.com/image/5914013/image-background-public-domain-eye>, found via Openverse with a license=cc0 filter, searching `crocodile white background`; a real photograph on seamless white -- the one background the flood matte cannot get wrong
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build menagerie.recipe.yaml`):
  - `crop`: frac_box=[0.08, 0.1, 0.97, 0.55].
  - `matte_backdrop`: tolerance=16, soft=10, border=2, enclosed='drop'.
  - `resize`: width=300.
  - Encoded WEBP, quality 84.

### from `watcher`
- **Source**: "Owl Eyes", <https://stocksnap.io/photo/owl-eyes-E1CMK2AIID>, found via Openverse with a license=cc0 filter, searching `owl portrait dark`; a barn owl photographed pale against near-dark, which is the contrast the flood matte wants -- a great horned owl on a branch was tried first and its dark plumage shredded, being within tolerance of its own background everywhere
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build menagerie.recipe.yaml`):
  - `crop`: frac_box=[0.115, 0.055, 0.5, 0.79].
  - `matte_backdrop`: tolerance=14, soft=12, border=2, enclosed='drop'.
  - `resize`: width=190.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: Both plates are CC0 -- the crocodile by rawpixel's dedication, the owl by StockSnap's -- so the matted cutouts are clean to commit, and the beetle's rule holds for the whole menagerie; a creature that vanishes when a CDN moves is a haunting nobody asked for.
<!-- animist:end menagerie -->
<!-- animist:begin fates -->
## wheel-coin-obverse.webp, wheel-coin-reverse.webp, wheel-sparks.webp, wheel-sword.webp

### from `blade`
- **Source**: "Two-Handed Sword, ca. 1400-1450", <https://www.metmuseum.org/art/collection/search/35388>, found via the Met Open Access API, searching `sword` -- the one European two-hander in the results with isPublicDomain set and a full studio plate, and its date sits inside the century the painting wears
.
- **Licence**: CC0. Confirmed through met at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build fates.recipe.yaml`):
  - `crop`: frac_box=[0.22, 0.015, 0.78, 0.985].
  - `matte_backdrop`: tolerance=18, soft=10, border=2, enclosed='drop'.
  - `resize`: height=380.
  - Encoded WEBP, quality 80.

### from `heads`
- **Source**: "Tetradrachm: Head of Athena, r. (obverse) 449-440 BCE", <https://www.flickr.com/photos/clevelandart/25279094634>, found via Openverse with a license=cc0 filter, searching `tetradrachm owl athena`; the Cleveland Museum of Art's open-access dedication, photographed dead-on with the relief lit
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build fates.recipe.yaml`):
  - `matte_backdrop`: tolerance=16, soft=10, border=2, enclosed='keep'.
  - `duotone`: shadow='#2a1a06', mid='#b8862e', light='#ffeab0'.
  - `resize`: width=128.
  - Encoded WEBP, quality 86.

### from `tails`
- **Source**: "Tetradrachm: Owl, standing, r., crescent moon and olive branch above (reverse) 449-440 BCE
", <https://www.flickr.com/photos/clevelandart/24677217172>, found via the same search and the same collection, same decade of striking -- the matched reverse, and the owl on it answers the owl on the limb
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build fates.recipe.yaml`):
  - `matte_backdrop`: tolerance=16, soft=10, border=2, enclosed='keep'.
  - `duotone`: shadow='#2a1a06', mid='#b8862e', light='#ffeab0'.
  - `resize`: width=128.
  - Encoded WEBP, quality 86.

### from `tinder`
- **Source**: "Free burning sparkler black background", <https://www.rawpixel.com/search?page=1&q=burning%20sparkler>, found via Openverse with a license=cc0 filter, searching `sparks black background`; a real spark shower on true black, which screen blend composites the same way the shades' smoke goes -- the black vanishes and only fire survives, the sparkler's own stick included
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build fates.recipe.yaml`):
  - `crop`: frac_box=[0.08, 0.04, 0.88, 0.96].
  - `resize`: width=320.
  - Encoded WEBP, quality 82.

Why committed rather than hotlinked: CC0 on both plates, so the cutouts are clean to commit; and a fate whose gold evaporates when a CDN moves would be a poor omen for a fortune wheel.
<!-- animist:end fates -->
<!-- animist:begin lantern -->
## wheel-lantern.webp

- **Source**: "Burning candle lantern night", <https://www.rawpixel.com/image/6039984/photo-image-light-public-domain-free>, found via Openverse with a license=cc0 filter, searching `candle lantern`; the one plate in the results that is a real candle burning in a real hand-lantern rather than an electric fixture, and the star-pierced panes answer the Hermit's own star
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build lantern.recipe.yaml`):
  - `crop`: frac_box=[0.1, 0.0, 0.545, 0.95].
  - `resize`: width=150.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: CC0 by rawpixel's dedication, so the cutout is clean to commit, and the beetle's rule holds again: a lantern that goes dark when a CDN moves would be the wrong kind of omen.
<!-- animist:end lantern -->

<!-- animist:begin shades -->
## wheel-shade-1.webp, wheel-shade-2.webp, wheel-shade-3.webp

### from `breath`
- **Source**: "Smoke black background", <https://www.rawpixel.com/search?page=1&q=smoke%20black%20background>, found via Openverse with a license=cc0 filter, searching `smoke black background`; a single rising column that folds at its crown like a veiled figure
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build shades.recipe.yaml`):
  - `crop`: frac_box=[0.35, 0.02, 0.75, 0.98].
  - `duotone`: shadow='#000000', mid='#4d7a58', light='#d8f0dc'.
  - `resize`: width=160.
  - Encoded WEBP, quality 82.

### from `shade`
- **Source**: "Smoke black background", <https://www.rawpixel.com/search?page=1&q=smoke%20black%20background>, found via the same Openverse search; dense smoke whose central swirl reads as a head over a trailing body, which is as much ghost as any photograph honestly holds
.
- **Licence**: cc0. Confirmed through the Openverse record API at fetch time (2026-08-17).
- **Transformations** (Pillow, scripted -- `mtglab animist build shades.recipe.yaml`):
  - `crop`: frac_box=[0.3, 0.0, 0.78, 0.95].
  - `duotone`: shadow='#000000', mid='#4d7a58', light='#d8f0dc'.
  - `resize`: width=180.
  - Encoded WEBP, quality 82.
  - `crop`: frac_box=[0.0, 0.12, 0.32, 1.0].
  - `duotone`: shadow='#000000', mid='#4d7a58', light='#d8f0dc'.
  - `resize`: width=150.
  - Encoded WEBP, quality 82.

Why committed rather than hotlinked: Both plates are CC0 by rawpixel's dedication, so the graded crops are clean to commit; and the dead should not fail to rise because a CDN moved.
<!-- animist:end shades -->
