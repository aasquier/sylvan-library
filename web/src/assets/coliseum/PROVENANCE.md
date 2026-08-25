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
