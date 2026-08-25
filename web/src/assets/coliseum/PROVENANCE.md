# coliseum assets -- provenance

The rule here is the tarot deck's (`assets/tarot/PROVENANCE.md`): nothing ships in this directory whose licence was not checked per file, and every transformation applied is written down so the derivation is reproducible from the source.

<!-- animist:begin harena -->
## harena.webp

- **Source**: "Coarse yellow sand.jpg", <https://commons.wikimedia.org/wiki/File:Coarse_yellow_sand.jpg>, found via Commons Category:Sand textures, filtered to public domain and CC0 only, after Category:Colosseum and Category:Hypogeum of the Colosseum returned nothing usable under those two licences
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-24).
- **Transformations** (Pillow, scripted -- `animist build harena.recipe.yaml`):
  - `crop`: frac_box=[0.17, 0.09, 0.93, 0.93].
  - `levels`: out_black='#3a2c17'.
  - `resize`: width=860.
  - Encoded WEBP, quality 70.

Why committed rather than hotlinked: Two reasons, and the first is the licence. The floor under a match has to be public domain end to end, and Wikimedia's Colosseum photography is almost entirely CC BY-SA -- an attribution obligation a decoration must not carry, which the gate refuses and is right to. This plate is public domain outright. The second is that a battlefield must not go blank when somebody else's CDN moves: the sand is the ground every card in the room stands on, and forty pieces of hotlinked card art on a missing floor is a worse page than no board at all.
<!-- animist:end harena -->

<!-- animist:begin secutor -->
## secutor.webp

- **Source**: "Terracotta statuette of a gladiator, 1st-2nd century CE", <https://www.metmuseum.org/art/collection/search/248365>, found via the Met Open Access API, searching the Greek and Roman Art department for `gladiator` -- the one figure in the results with a full studio plate, an unambiguous secutor's helmet, and no second object in frame
.
- **Licence**: CC0. Confirmed through met at fetch time (2026-08-25).
- **Transformations** (Pillow, scripted -- `animist build secutor.recipe.yaml`):
  - `matte_neutral`: tolerance=14, soft=16.
  - `crop`: frac_box=[0.3458, 0.07, 0.7242, 0.92].
  - `resize`: height=448.
  - Encoded WEBP, quality 82.

Why committed rather than hotlinked: The licence is clean and the figure is small, but the real reason is that he stands in a slot that is empty by construction. He fills the slack between a short slide and the controls pinned under it — space that exists because the painting beside him is tall, and that varies with every one of thirteen slides. A decoration that is sometimes 224 pixels tall and sometimes 80 has to be a local file: a figure that resolves from somebody else's CDN would pop in at a different size on every slide, and the one place a reader's eye is already moving is exactly where a late-arriving image is most obvious.
<!-- animist:end secutor -->

<!-- animist:begin aegis -->
## aegis.webp

- **Source**: "Shield, Turkish or Mamluk, late 15th century", <https://www.metmuseum.org/art/collection/search/24296>, found via the Met Open Access API, searching `shield`, `buckler` and `scutum` and keeping only the public-domain results whose objectName is actually a shield -- nine of them, of which this is the one that is a clean circle on an even ground
.
- **Licence**: CC0. Confirmed through met at fetch time (2026-08-25).
- **Transformations** (Pillow, scripted -- `animist build aegis.recipe.yaml`):
  - `matte_backdrop`: tolerance=20, soft=10, border=2, enclosed='keep'.
  - `crop`: frac_box=[0.1933, 0.0889, 0.8067, 0.8933].
  - `resize`: width=220.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: It is drawn at the moment a beat lands and lives for about a second, which makes it the worst possible thing to fetch from somebody else's CDN: a mark that arrives after the sentence it belongs to has been read is not a mark, it is a flicker on an unrelated card. It is also small enough that committing it costs less than the request would.
<!-- animist:end aegis -->

<!-- animist:begin memento -->
## memento.webp

- **Source**: "Skull mosaic MAN Naples Inv 109982.jpg", <https://commons.wikimedia.org/wiki/File:Skull_mosaic_MAN_Naples_Inv_109982.jpg>, found via Commons, searching `memento mori mosaic Pompeii skull`, then filtering the results by licence -- the gladiator steles found alongside it are mostly CC BY-SA and one is Attribution-only, obligations a decoration must not carry
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-25).
- **Transformations** (Pillow, scripted -- `animist build memento.recipe.yaml`):
  - `crop`: frac_box=[0.28, 0.235, 0.72, 0.665].
  - `resize`: width=200.
  - Encoded WEBP, quality 82.

Why committed rather than hotlinked: The same reason as the shield beside it: this is drawn on the beat that says a creature died and it is gone about a second later, so it has to be in the bundle rather than a request away. There is a second reason here, though — the licence is a *photograph of an ancient work*, and that is exactly the category where somebody else's file can quietly change licence or vanish. A copy checked once and committed is a copy whose provenance stays true.
<!-- animist:end memento -->
