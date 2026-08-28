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

<!-- animist:begin crypta -->
## crypta.webp

- **Source**: "Roma. Colombari di Vigna Codini Gang, GD010218.jpg", <https://commons.wikimedia.org/wiki/File:Roma._Colombari_di_Vigna_Codini_Gang,_GD010218.jpg>, found via Commons, searching `columbarium Rome niches` and then `Vigna Codini columbarium`, both restricted to bitmaps and filtered to Public domain and CC0 -- an unrestricted search returns almost nothing but scanned nineteenth-century guidebooks, which is the trap `lens.recipe.yaml` records. Searches for catacombs and for tufa galleries returned nothing free and large enough at all. The Met's Open Access API was 403-ing every request from this machine on the day of the build, so it was never asked; the two Rijksmuseum albumen prints of the same chambers are CC0 and are album pages with two small prints to a sheet.
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-27).
- **Transformations** (Pillow, scripted -- `animist build crypta.recipe.yaml`):
  - `crop`: frac_box=[0.235, 0.3, 0.752, 0.868].
  - `resize`: width=520.
  - Encoded WEBP, quality 78.

Why committed rather than hotlinked: The same argument as the road and the skull beside it. This is drawn on the beat that says a creature died and it is gone two seconds later, so fetching it from somebody else's host would mean a vault that opens after the card has already gone down into it. The second reason is the licence: a scanned glass lantern slide of unknown authorship, re-hosted by a university library and then by Commons, is squarely the category where somebody else's copy quietly changes terms or moves. A copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end crypta -->

<!-- animist:begin campus -->
## campus.webp

- **Source**: "La Bataille d'Issus - Jan Brueghel l'Ancien - Musée du Louvre Peintures INV 1094 ; MR 596.jpg", <https://commons.wikimedia.org/wiki/File:La_Bataille_d%27Issus_-_Jan_Brueghel_l%27Ancien_-_Mus%C3%A9e_du_Louvre_Peintures_INV_1094_;_MR_596.jpg>, found via Commons, searching `Brueghel Battle of Issus` after `Battle of the Milvian Bridge Giulio Romano` and `Battle of Zama` had been fetched and rejected on sight. Every unrestricted phrasing -- `battle in a valley landscape painting armies`, `bird's eye battle`, `Roman battle landscape` -- returns nothing but scanned art-history books and auction catalogues, which is the trap `lens.recipe.yaml` records; the searches that work are the ones that name a painter and a battle. This is the Louvre's own panel at 11022x6994, and the only one of the four candidates that is both a valley and a landscape band -- the Altdorfer is a valley in a portrait panel, and the other two are friezes.
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-27).
- **Transformations** (Pillow, scripted -- `animist build campus.recipe.yaml`):
  - `crop`: frac_box=[0.0, 0.26, 1.0, 0.9].
  - `resize`: width=720.
  - Encoded WEBP, quality 78.

Why committed rather than hotlinked: The same argument as the road and the vault beside it. This is drawn on the beat that says a permanent arrived uncast and it is gone about a second and a half later, so fetching it from somebody else's host would mean a battlefield that opens after the creature standing on it has already faded. The licence is the second reason: this is a PD-Art photographic reproduction uploaded by an individual contributor, which is the category where somebody else's copy quietly moves or changes terms -- a copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end campus -->

<!-- animist:begin fabrica -->
## fabrica.webp

- **Source**: "Joseph Wright - An Iron Forge - Google Art Project.jpg", <https://commons.wikimedia.org/wiki/File:Joseph_Wright_-_An_Iron_Forge_-_Google_Art_Project.jpg>, found via Commons, searching the painter and the painting by name -- which is the search that works here and the one `campus.recipe.yaml` recorded. Every unrestricted phrasing (`blacksmith anvil sparks`, `forge hammer glowing iron`, `smithy interior night`) returns modern photographs under CC BY-SA and scanned trade catalogues. Three candidates were fetched and cropped with a card stood in them before this one was chosen: Wright's own `A Blacksmith's Shop` (Yale, 4688x5975, rejected on the crop -- see the header), Velazquez's `La Fragua de Vulcano` (Prado, 2952x2293, a frieze with a god in it), and this. The Google Art Project scan is 2801x2572, which is nearly square and the reason a wide band comes out of it without throwing most of the painting away.
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-27).
- **Transformations** (Pillow, scripted -- `animist build fabrica.recipe.yaml`):
  - `crop`: frac_box=[0.22, 0.4, 1.0, 0.92].
  - `resize`: width=660.
  - Encoded WEBP, quality 76.

Why committed rather than hotlinked: The three scenes beside it, exactly. This draws on the beat an artifact arrives and is gone about a second later, so fetching it from somebody else's host would mean a forge that opens after the card standing in it has faded. And the licence is the second reason: this is a PD-Art photographic reproduction on Commons, which is the category where somebody else's copy quietly moves or changes terms -- a copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end fabrica -->

<!-- animist:begin templum -->
## templum.webp

- **Source**: "Piranesi-17003.jpg", <https://commons.wikimedia.org/wiki/File:Piranesi-17003.jpg>, found via Commons, searching the printmaker and the subject by name -- which is the search that works for pictures and the one `campus.recipe.yaml` recorded. Unrestricted phrasings (`Roman temple interior`, `colonnade ruins`, `sacred precinct`) return modern photographs under CC BY-SA and scanned guidebooks. Reached after the curse tablet, Panini's Pantheon and the Carceri had each been fetched, cropped to a band with a card stood in it, and rejected on sight -- see the header for all three.
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-27).
- **Transformations** (Pillow, scripted -- `animist build templum.recipe.yaml`):
  - `crop`: frac_box=[0.045, 0.12, 0.955, 0.8].
  - `resize`: width=700.
  - Encoded WEBP, quality 80.

Why committed rather than hotlinked: The four scenes beside it, exactly. This draws on the beat an enchantment arrives and is gone about a second later, so fetching it from somebody else's host would mean a colonnade that opens after the card standing in it has faded. And the licence is the second reason: this is a PD-Art photographic reproduction on Commons, which is the category where somebody else's copy quietly moves or changes terms -- a copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end templum -->

<!-- animist:begin velatio -->
## velatio.webp

- **Source**: "Aldobrandini wedding.JPG", <https://commons.wikimedia.org/wiki/File:Aldobrandini_wedding.JPG>, found via Commons, searching `Aldobrandini Wedding fresco` -- naming the work, which is the search that works here and the one `campus.recipe.yaml` recorded. Unrestricted phrasings (`Roman bride veiled fresco`, `Roman wedding painting`) return scanned Victorian handbooks almost exclusively, which is `lens.recipe.yaml`'s trap. Two other veiling subjects were considered and not fetched: the Villa of the Mysteries frieze, whose good wide photographs on Commons are all CC BY-SA rather than public domain, and the Pompeian `Nozze di Ercole`, which is a fragment.
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-27).
- **Transformations** (Pillow, scripted -- `animist build velatio.recipe.yaml`):
  - `crop`: frac_box=[0.16, 0.08, 0.94, 0.92].
  - `resize`: width=700.
  - Encoded WEBP, quality 78.

Why committed rather than hotlinked: The five scenes beside it, exactly. This draws on the beat an Aura goes onto a creature and is gone about a second later, so fetching it from somebody else's host would mean a rite that opens after the card in it has faded. And the licence is the second reason: this is a PD-Art photographic reproduction on Commons, which is the category where somebody else's copy quietly moves or changes terms -- a copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end velatio -->

<!-- animist:begin certamen -->
## certamen.webp

- **Source**: "Ave Caesar Morituri te Salutant (Gérôme) 01.jpg", <https://commons.wikimedia.org/wiki/File:Ave_Caesar_Morituri_te_Salutant_(G%C3%A9r%C3%B4me)_01.jpg>, found via Commons, searching `Ave Caesar Morituri te Salutant` by name -- the search that works here, as it was for the valley and the veiling. The unrestricted phrasings (`Roman amphitheatre interior`, `amphitheatre cavea arena floor`, `arena from the stands`) return CC BY-SA photographs of tourist sites and scanned nineteenth-century guidebooks and nothing else, which is `lens.recipe.yaml`'s trap for the third time. Twelve candidates were fetched and measured against the layout's own card footprint before this was chosen: nine amphitheatre photographs and three Piranesi plates. The two that got closest were rejected for stated reasons -- `Amphitheatre (Pula), interior 96` (CC0, structurally the best photograph, and it carries a modern steel safety railing and visitors in bright shirts that only grow if you crop closer) and Piranesi's `Veduta dell'Anfiteatro Flavio` (public domain, the cleanest register and the darkest of the three finalists, and it has no arena floor for the blocking rank to stand on). The painting is Yale University Art Gallery object 9187; the Commons file is the gallery's own reproduction at 3000x1881.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-28) -- `LicenseShortName` and `UsageTerms` both "Public domain". Gérôme died in 1904, and the photograph is a faithful reproduction of a two-dimensional public-domain work.
- **Transformations** (Pillow, scripted -- `animist build certamen.recipe.yaml`):
  - `crop`: frac_box=[0.38, 0.14, 1.0, 0.92]. The window puts the painting's dead gladiators out of frame, drops the attacker's card onto the packed cavea rather than the pale velarium, and leaves the blocking rank clean sand. The crop is narrower than the band, so the height is taken from its middle.
  - `resize`: width=880.
  - Encoded WEBP, quality 84.

Why committed rather than hotlinked: The six scenes beside it, exactly. This draws on the beat a creature is blocked and is gone about two seconds later, so fetching it from somebody else's host would mean an arena that opens after the fight standing in it has faded. And the licence is the second reason: this is a PD-Art photographic reproduction on Commons, which is the category where somebody else's copy quietly moves or changes terms -- a copy confirmed once and committed is a copy whose provenance stays true.
<!-- animist:end certamen -->

<!-- animist:begin ossarium -->
## ossarium.webp

- **Source**: "Beinhaus Kutna Hora 1990.JPG", <https://commons.wikimedia.org/wiki/File:Beinhaus_Kutna_Hora_1990.JPG>, found via Commons after two rounds — the first searched Paris by name, as Aaron asked, and returned three free files of which none was usable; the second widened to `ossuaire`, `Beinhaus`, `Capuchin crypt Rome bones` and `Kutna Hora ossuary interior`. Nine free candidates in all, four fetched and measured against the bout's own card footprint. Paris itself is the arena backdrop's licence wall again: its catacombs are a tourist site, so the good modern photographs are CC BY-SA, and what is public domain is either a painting of eighteenth-century gentlemen in tricorn hats visiting (the "no non-Roman figures at size" rule exactly) or the famous inscription, which is French text on a flat wall rather than a place. Rome's Capuchin Crypt is nearer the room's own city and its free plate is a bright nineteenth-century print of robed skeletons standing up, at mean 159 far outside the range.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-28) — `LicenseShortName` and `UsageTerms` both "Public domain". An own-work release by the uploader rather than a term expiry.
- **Transformations** (Pillow, scripted — `animist build ossarium.recipe.yaml`):
  - `crop`: frac_box=[0.0, 0.0, 1.0, 1.0]. The whole plate; it is 1.59 against a band of 1.804, so the height comes from its middle, and there is nothing at the top or bottom this picture's middle does not also have.
  - `resize`: width=880, matching `certamen.webp` so the scene does not jump when a fight resolves.
  - Encoded WEBP, quality 80. Measured at the bout's band with the cards stood in it: mean 64, p90 128 — in range as it stands, needing neither a blend nor a hold-back.

Why committed rather than hotlinked: The seven scenes beside it, exactly — it is drawn on the beat that says a fight settled badly and is gone about two seconds later. The licence is the sharper reason here: this is an individual contributor's own photograph released into the public domain, which `via.recipe.yaml` records as precisely the category where a third party's copy quietly changes terms or disappears.
<!-- animist:end ossarium -->

<!-- animist:begin triumphus -->
## triumphus.webp

- **Source**: "Giovanni Battista Piranesi, Arco di Costantino, 1748, NGA 125667.jpg", <https://commons.wikimedia.org/wiki/File:Giovanni_Battista_Piranesi,_Arco_di_Costantino,_1748,_NGA_125667.jpg>, found via Commons, searching the painter by name after an unrestricted hunt for triumphs and arches produced only cassone panels and centred monuments — the third time this directory has recorded that naming the painter is the search that works. Forty-nine free wide candidates across seven phrasings; five fetched and measured. What the others failed on: every photograph of a triumphal arch puts the arch dead centre, which is where the attacker's card stands, and the wide Roman-triumph paintings (Apollonio di Giovanni's `Triumph of Lucius Aemilius Paullus`, the Marradi Master's `A Roman Triumph`) are quattrocento cassone panels — the same fault `campus.recipe.yaml` rejected the `Battle of Zama` for, one of them with its gilt frame in the scan.
- **Licence**: CC0. Confirmed through the Wikimedia Commons API at fetch time (2026-08-28) — `UsageTerms` is "Creative Commons Zero, Public Domain Dedication". Donated to Commons by the National Gallery of Art: an institution's own scan under an explicit dedication, which is the strongest provenance any plate in this directory has.
- **Transformations** (Pillow, scripted — `animist build triumphus.recipe.yaml`):
  - `crop`: frac_box=[0.0, 0.16, 0.70, 0.90]. Left of the arch and below the sky, so the middle of the frame is ruins and depth rather than a monument, and the arch sits at the right where the mask fades.
  - `resize`: width=880, matching `certamen.webp` and `ossarium.webp`.
  - Encoded WEBP, quality 68 — swept 60 to 82, and the curve is a straight line with no knee: an etching is a quarter of a million hand-cut lines. 68 is where the stylesheet's blend has already crushed the finest hatching.
  - **The stylesheet blends it**, as it does `templum`: `background-blend-mode: soft-light` over a dark ground. Raw the plate is mean 121 / p90 203 — line on white paper, the `Carceri` fault. Blended it lands at mean 37 / p90 68. The asset itself is untouched, which is every recipe's rule here.

Why committed rather than hotlinked: The seven scenes beside it — drawn on the beat that says a fight settled well and gone about two seconds later, so a hotlinked copy would open after the creature walking through it had faded.
<!-- animist:end triumphus -->

<!-- animist:begin arva -->
## arva.webp

- **Source**: "Lucas van Uden - Panoramic landscape with shepherds and peasants.jpg", <https://commons.wikimedia.org/wiki/File:Lucas_van_Uden_-_Panoramic_landscape_with_shepherds_and_peasants.jpg>, found via Commons across six phrasings that named painters — `Philips Koninck panoramic landscape`, `Ruisdael panoramic landscape fields`, `Hobbema landscape countryside` and others. The fourth time this directory has recorded that naming the painter is the search that works. Fourteen free wide candidates; six fetched and measured with a card stood in them. The five rejected all failed the same way: panoramic landscape is a genre of *drawings*, and line on paper arrives as a lamp behind a card — `Panoramic landscape near Bergen` (Met, CC0) is pencil at mean 203, `An Extensive Panoramic Landscape with a Windmill` (Met, CC0) red chalk at 193, `A Panoramic Landscape with a Herdsman and His Flock` (Met, CC0) brown wash at 171, Cuyp's `Panoramic Landscape along the Rhine` (NGA, CC0) a pale wash at 168. The one other painting, `Rural Scene` (Nationalmuseum), is a grisaille packed with horses, carts and foreground figures — an incident, where this beat needs somewhere merely *being*.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-28) — `LicenseShortName` and `UsageTerms` both "Public domain". Van Uden died in 1672, so the term has certainly expired, and the photograph is a faithful reproduction of a two-dimensional public-domain work. **The provenance is weaker than the museum plates beside it and that is worth saying**: the Commons file credits an auction house rather than an institution, so there is no CC0 dedication behind it the way there is for `triumphus` — only PD-Art over an expired term. Sound, and exactly the category `via.recipe.yaml` flags as most likely to move or change terms.
- **Transformations** (Pillow, scripted — `animist build arva.recipe.yaml`):
  - `crop`: frac_box=[0.42, 0.0, 1.0, 1.0]. Uncropped, the card lands squarely on a village steeple — a single subject, and the one shape in the picture that could be argued as iconography. From x 0.42 rightward it sits at the far left where the mask is already fading, and the card gets open receding fields instead. Four windows were cut and judged with a card in them.
  - `resize`: width=880, matching the four scenes it stands beside.
  - Encoded WEBP, quality 80 — 37 KB, the smallest picture in this directory, because a hazy Flemish distance has no hard edge for a codec to spend bytes on. Convenient rather than lucky, since this is the plate drawn most often. Measured at mean 116, and it ships **held back to 0.72** besides, further than `velatio`'s 0.82.
  - **On the steeple**: `campus.recipe.yaml` set the no-Christian-iconography refusal turning down Giulio Romano for angels and a cross-standard, and Panini's Pantheon for a frieze reading *LAUS EIUS IN ECCLESIA*. Those are images and words of the faith. A village steeple on a Flemish horizon is a building, and here it is at the rim at some twenty pixels. Kept, and recorded so the judgement is on the record rather than assumed.

Why committed rather than hotlinked: The eight scenes beside it, and more sharply than any of them — this is the most frequently drawn picture in the room, eleven times a game, so a copy fetched from somebody else's host would be the one asset whose latency a player actually learns. At 37 KB the request would also cost more than the bytes.
<!-- animist:end arva -->
