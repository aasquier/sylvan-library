# Notices and attribution

## Wizards of the Coast

mtg-lab is unofficial Fan Content permitted under the Wizards of the Coast Fan
Content Policy. Not approved or endorsed by Wizards. Portions of the materials
used are property of Wizards of the Coast. ©Wizards of the Coast LLC.

Magic: The Gathering, Wizards of the Coast, and their logos are trademarks of
Wizards of the Coast LLC in the United States and other countries.

**The Fan Content Policy permits noncommercial use only.** This project must
stay free. No paid tiers, no sponsorship of the tool, no donations tied to it,
no advertising against it. If you fork it, that constraint travels with you.

## Scryfall

Card data and images are provided by [Scryfall](https://scryfall.com), free of
charge, for the purpose of building Magic software and community content. Per
their guidelines:

- Do not imply Scryfall has endorsed this project.
- Do not put Scryfall data behind payment, subscription, or a survey.
- Do not redistribute their bulk data as your own dataset. **This repository
  contains no card data.** `mtglab data refresh` downloads it at runtime and
  `data/` is gitignored.
- Use the daily bulk files rather than hammering the API card by card. The
  ingest in `cards/db.py` makes one request per bulk file per day.

Card images are hotlinked or cached locally at runtime and are never committed.
Artwork remains the property of its artists and Wizards of the Coast.

## Forge

The optional Tier 3 bridge shells out to [Forge](https://github.com/Card-Forge/forge)
as a separate process. It is not bundled, linked, or redistributed here, and
its own license applies to it. You install it yourself.

**Forge is licensed GPL-3.0** — verified against the `LICENSE.txt` in its own
distribution, not assumed. Three consequences, none of which bind this
repository today but all of which bind a hosted instance that ships it:

1. **This project stays MIT.** `sim/tier3/run.py` starts `forge.jar` as a
   separate process and reads its stdout; nothing links to it and nothing
   imports it. The FSF treats "pipes, sockets and command-line arguments" as
   the communication mechanisms normally used *between two separate programs*,
   which is exactly this boundary.
2. **The GPL is triggered by distributing binaries, not by charging for them.**
   Its requirements are identical whether software is sold or given away, so
   this project being noncommercial — which the Fan Content Policy above
   requires — neither creates nor removes any obligation. The two rule sets are
   independent, and both are satisfied: the GPL permits noncommercial
   distribution, and Wizards requires it.
3. **Forge is GPL-3.0, not AGPL-3.0**, and the difference decides the hosted
   case. Running GPL software as a network service is not distribution, so a
   hosted instance that lets people *use* Forge over the web incurs no
   source-sharing obligation. The AGPL is the licence that would have changed
   that, and Forge is not under it.

Where an obligation *would* attach is **publishing a container image with Forge
inside it**, which is distribution in the ordinary way. `docs/HOSTING.md` §7
records the practical route around that, and it is the same one already used
for the card pool: keep it on the volume, not in the image.

Nothing here is legal advice.

## imageio-ffmpeg and its bundled ffmpeg

The animist's video encoders (ADR 31) run through
[imageio-ffmpeg](https://github.com/imageio/imageio-ffmpeg), which is
**BSD-2-Clause** and whose wheel bundles a static **ffmpeg** binary. That
binary is a **GPL build** (it links x264, among others), and the argument for
why that binds nothing here is Forge's argument verbatim, one section up:

1. It is a **build-time subprocess on the dev machine and CI** — `mtglab
   animist build` starts it as a separate process and feeds it frames over a
   pipe, the boundary the FSF names as the one between two separate programs.
   Nothing links it, and this project stays MIT.
2. It is **never in the container image and never served to users**. The
   `animist` extra is not installed by the image's `.[api,claude]`, so the
   only thing distributed to anybody is the encoded WebM/MP4 output — and an
   encoder's licence does not attach to the files it encodes.
3. If a future change ever did put ffmpeg in a distributed image, the Forge
   section's rule applies: that is distribution, and it would need the same
   deliberate handling. Do not do it casually.

## Depth-Anything V2 (planned)

The depth-parallax phase (ADR 32) uses Depth-Anything-V2-Small, released under
**Apache-2.0**, via CPU torch on the dev machine only. Model weights are
downloaded to the gitignored cache at build time and are never committed,
never in the image, and never served.

## Prices

Price data comes from the Scryfall printings feed. TCGplayer's developer API is
closed to new applicants; this project does not scrape TCGplayer and does not
automate purchases.
