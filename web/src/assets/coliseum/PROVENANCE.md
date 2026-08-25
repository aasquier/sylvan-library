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
