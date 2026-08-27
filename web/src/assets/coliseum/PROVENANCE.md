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

- **Source**: "Schild des Landgrafen Konrad von Thüringen fm810232.jpg", <https://commons.wikimedia.org/wiki/File:Schild_des_Landgrafen_Konrad_von_Th%C3%BCringen_fm810232.jpg>, found via Commons, through the category `Photographs of shields by Ludwig Bickell`, which is where the search finally landed after the Met's Arms and Armor department turned out to hold no heater at all -- its shields are discs, dhals and bouched targes, and a bouche's lobed outline is no more legible small than a circle is. The category was reached from `Heater shields`; every file in it was licence-checked and every one of them passes
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-27).
- **Transformations** (Pillow, scripted -- `animist build aegis.recipe.yaml`):
  - `crop`: frac_box=[0.1162, 0.1916, 0.9072, 0.9172].
  - `matte_backdrop`: tolerance=30, border=6, soft=26, enclosed='drop'.
  - `resize`: width=300.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: It is drawn at the moment a beat lands and lives for about a second, which makes it the worst possible thing to fetch from somebody else's CDN: a mark that arrives after the sentence it belongs to has been read is not a mark, it is a flicker on an unrelated card. It is also small enough that committing it costs less than the request would. And the licence here is a *photograph of a medieval object*, which is the category where somebody else's file quietly changes licence or vanishes -- a copy checked once and committed is a copy whose provenance stays true.
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

<!-- animist:begin ensis -->
## ensis.webp

- **Source**: "Hand-and-a-Half Sword, European or possibly British, 15th century", <https://www.metmuseum.org/art/collection/search/35888>, found via the Met Open Access API, looking for a full-length studio plate of a medieval sword shot vertically on an even ground -- of the two that qualify, this is the one whose steel is polished rather than corroded, which is what a mark drawn over card art at sixty pixels needs
.
- **Licence**: CC0. Confirmed through met at fetch time (2026-08-26).
- **Transformations** (Pillow, scripted -- `animist build ensis.recipe.yaml`):
  - `crop`: frac_box=[0.29, 0.04, 0.78, 0.955].
  - `resize`: height=620.
  - `matte_backdrop`: tolerance=32, soft=7, border=3, enclosed='keep'.
  - `resize`: height=300.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: The same argument the shield and the skull already make, with one addition that is specific to this beat. It is drawn on the most frequent event in a match and it lives for about a second, so a mark that arrives from somebody else's CDN after the beat has passed is not late -- it is wrong, landing on whatever the board has moved on to. At five kilobytes it is also cheaper to commit than to request.
<!-- animist:end ensis -->

<!-- animist:begin manica -->
## manica.webp

- **Source**: "Gauntlet for the Right Hand, from Tannenberg Castle, German, ca. 1380", <https://www.metmuseum.org/art/collection/search/23158>, found via the Met Open Access API, after searching for broken and fragmentary arms and armour returned essentially nothing usable -- the search that worked asked for pieces recovered from destroyed sites rather than for damage, which is where the objects that look defeated actually are
.
- **Licence**: CC0. Confirmed through met at fetch time (2026-08-26).
- **Transformations** (Pillow, scripted -- `animist build manica.recipe.yaml`):
  - `matte_neutral`: tolerance=13, soft=10.
  - `crop`: frac_box=[0.24, 0.2591, 0.7755, 0.8068].
  - `resize`: width=440.
  - Encoded WEBP, quality 78.

Why committed rather than hotlinked: It appears in the same breath as the wreath, at the end of a match, and the two are read against each other -- so a gauntlet that arrives from somebody else's CDN a beat after the crown has landed does not merely appear late, it breaks the comparison the panel exists to make. The licence argument is the skull's: a photograph of an ancient object is exactly the category where a third-party file quietly changes terms or moves, and a copy checked once and committed is a copy whose provenance stays true.
<!-- animist:end manica -->

<!-- animist:begin corona -->
## corona.webp

- **Source**: "Gold funerary wreath, Roman, 1st-2nd century CE", <https://www.metmuseum.org/art/collection/search/254968>, found via the Met Open Access API, searching the Greek and Roman department for gold wreaths and keeping the ones photographed whole on a plain ground, then choosing between them on silhouette -- an arc that opens downward can crown something, and a flat band cannot
.
- **Licence**: CC0. Confirmed through met at fetch time (2026-08-26).
- **Transformations** (Pillow, scripted -- `animist build corona.recipe.yaml`):
  - `crop`: frac_box=[0.118, 0.168, 0.849, 0.851].
  - `resize`: width=720.
  - `matte_backdrop`: tolerance=18, soft=10, border=3, enclosed='drop'.
  - Encoded WEBP, quality 80.

Why committed rather than hotlinked: It is the largest thing on the screen at the moment it appears and it appears exactly once, at the end of a match a user has just spent minutes watching. An image that resolves late from somebody else's CDN would pop in after the sentence it is illustrating, which is worse here than anywhere else on the board -- everywhere else a late picture is a decoration arriving, and here it is the punchline arriving after the joke. The licence is also a photograph of an ancient work, which is the category where a third-party file most quietly changes terms.
<!-- animist:end corona -->

<!-- animist:begin aurum -->
## aurum.webp

- **Source**: "Goblet, Avar, 700s, gold", <https://www.metmuseum.org/art/collection/search/464121>, found via the Met Open Access API, searching for gold coins, gold cups and gold hoards and then keeping the plates where the object is alone, face-on and lit from one side -- of the gold in the results this is the only one that is both unmistakably a vessel and bright enough to read as gold rather than as brass
.
- **Licence**: CC0. Confirmed through met at fetch time (2026-08-26).
- **Transformations** (Pillow, scripted -- `animist build aurum.recipe.yaml`):
  - `matte_neutral`: tolerance=16, soft=12.
  - `crop`: frac_box=[0.1853, 0.1059, 0.8176, 0.9676].
  - `resize`: height=200.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: A board can hold a dozen Treasures at once and every one of them draws this single file, so it belongs in the bundle rather than at the end of a dozen requests to somebody else's CDN. It is drawn the instant a token arrives, which is the worst possible moment for a picture to be late: a Treasure that fades in after the beat has passed is not a Treasure, it is a flicker. At ten kilobytes it is also cheaper to commit than to ask for.
<!-- animist:end aurum -->
<!-- animist:begin ferculum -->
## ferculum.webp

- **Source**: "Osias Beert (der Ältere) (1580 - 1623) - Still Life with Cherries and Strawberries in Porcelain Bowls - 60.2 - Gemäldegalerie.jpg", <https://commons.wikimedia.org/wiki/File:Osias_Beert_(der_%C3%84ltere)_(1580_-_1623)_-_Still_Life_with_Cherries_and_Strawberries_in_Porcelain_Bowls_-_60.2_-_Gem%C3%A4ldegalerie.jpg>, found via Commons, searching Flemish and Dutch still life for a dish of food seen from above with clear ground all round it, then filtered to public domain and CC0 -- of the ones that qualify this is the only bowl whose whole rim is visible and whose contents are a strong colour rather than another shade of the porcelain
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-26).
- **Transformations** (Pillow, scripted -- `animist build ferculum.recipe.yaml`):
  - `crop`: frac_box=[0.3015, 0.6002, 0.623, 0.7864].
  - `resize`: width=200, height=200.
  - `mask_circle`: cx=0.5, cy=0.5, r=0.497, feather=1.5.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: The same argument the gold and the glass beside it make: one file drawn by every Food on the board, needed the instant a token arrives. And this one is a crop -- a hotlink would have to fetch a 4000-pixel painting to draw forty pixels of it, every time, which is the whole cost of the picture spent on the part nobody sees.
<!-- animist:end ferculum -->
<!-- animist:begin lens -->
## lens.webp

- **Source**: "Magnifying Glass MET ADA5941.jpg", <https://commons.wikimedia.org/wiki/File:Magnifying_Glass_MET_ADA5941.jpg>, found via Commons, searching magnifying glasses restricted to bitmaps -- an unrestricted search returns almost nothing but scanned books -- and then filtered to public domain and CC0 only, after the Met's own Open Access API turned up no plate of a magnifier by itself
.
- **Licence**: CC0. Confirmed through the Wikimedia Commons API at fetch time (2026-08-26).
- **Transformations** (Pillow, scripted -- `animist build lens.recipe.yaml`):
  - `matte_neutral`: tolerance=44, soft=12.
  - `crop`: frac_box=[0.2529, 0.1482, 0.7842, 0.741].
  - `resize`: height=210.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: The same argument the gold beside it makes: a dozen tokens on one board draw one file, and it is drawn the moment the token arrives rather than a request later. There is a second reason particular to this one -- it is a photograph of an object held by a museum, re-hosted by a third party, and that is precisely the category where somebody else's copy quietly changes licence or disappears. A copy checked once and committed is a copy whose provenance stays true.
<!-- animist:end lens -->

<!-- animist:begin via -->
## via.webp

- **Source**: "Appia antica 2-7-05 048.jpg", <https://commons.wikimedia.org/wiki/File:Appia_antica_2-7-05_048.jpg>, found via Commons, searching `Appian Way Via Appia road` and then `Via Appia Antica`, filtering to Public domain and CC0 and discarding everything that was not a photograph -- the search returns far more engravings, maps and scanned books than pictures. Of the photographs, most are of the tombs rather than of the road; this is one of the few taken *along* it, standing on the paving and looking down its length.
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-26).
- **Transformations** (Pillow, scripted -- `animist build via.recipe.yaml`):
  - `crop`: frac_box=[0.0, 0.26, 1.0, 1.0].
  - `resize`: width=560.
  - Encoded WEBP, quality 74.

Why committed rather than hotlinked: The same argument as the skull and the shield beside it. This is drawn on the beat that says a permanent was exiled and it is gone about a second and a half later, so fetching it from somebody else's host would mean a road that arrives after the card has already left down it. It is also a photograph released into the public domain by an individual rather than by an institution, which is the category where a file most quietly changes licence or disappears — a copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end via -->

<!-- animist:begin cippus -->
## cippus.webp

- **Source**: "Marble funerary altar of Cominia Tyche MET DP138727.jpg", <https://commons.wikimedia.org/wiki/File:Marble_funerary_altar_of_Cominia_Tyche_MET_DP138727.jpg>, found via Commons, searching `Roman funerary altar MET` and `Roman grave marker MET` and keeping only CC0 and public-domain results -- the plain search for Roman steles is mostly CC BY-SA, an attribution obligation a decoration must not carry. Of the free ones this is the only Roman grave marker photographed square-on, whole, on a clean sweep and with nothing else in frame; the other funerary altars are all three-quarter views, which read as a box on a table rather than as a stone standing up. The object is the Met's 38.27, Roman, ca. A.D. 90-100.
.
- **Licence**: CC0. Confirmed through the Wikimedia Commons API at fetch time (2026-08-27).
- **Transformations** (Pillow, scripted -- `animist build cippus.recipe.yaml`):
  - `crop`: frac_box=[0.233, 0.204, 0.79, 0.902].
  - `matte_backdrop`: tolerance=28, soft=20, border=2.
  - `resize`: width=360.
  - Encoded WEBP, quality 80.

Why committed rather than hotlinked: It arrives in the same breath as the wreath, at the end of a match, and the two are read against each other -- a stone that turns up a beat after the crown has landed does not merely appear late, it breaks the comparison the panel exists to make. That is the gauntlet's argument and it survives the object being swapped. The second reason is the licence: a photograph of an ancient object re-hosted by a third party is precisely the category where somebody else's copy quietly changes terms or moves, and a copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end cippus -->
