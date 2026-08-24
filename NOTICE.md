# Notices and attribution

Third-party licence texts live in `licenses/`. Everything below either points
at one of them or explains why none is needed.

**Read the distinction this file turns on before adding a section.** Almost
nothing here is redistributed by this project: Forge is a separate process you
install yourself, ffmpeg is a build-time subprocess that never reaches the
image, Depth-Anything's weights are never served, and Scryfall's bulk data is
downloaded at runtime into a gitignored directory. The one exception is the
card reader — Tesseract — whose bytes this app serves to every browser that
opens the camera, and whose licences therefore have to travel with them. That
section, and the fonts', are the two that carry an actual obligation.

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

## Tesseract — the card reader, and the only code this project redistributes

The camera door reads a card in the browser. `go/internal/shelves/shelves.go` downloads the
engine once and pins every file by SHA-256 against the manifest in
`internal/reference`, and `/api/ocr/{name}` serves it (ADR 33's
arrangement applied to somebody else's compiler output). Nothing is hotlinked
and nothing enters git — but the bytes do reach every visitor, and *that* is
distribution in the ordinary way, unlike every other section in this file.

Verified 2026-08-19 from the packages themselves rather than from a summary:

| Served as | From | Licence |
|---|---|---|
| `tesseract-core-simd-lstm.wasm.js` | `tesseract.js-core@6.1.2` | Apache-2.0 (`LICENSE` in the package) |
| `worker.min.js` | `tesseract.js@7.0.0` | Apache-2.0 (`LICENSE.md` in the package) |
| `eng.traineddata.gz` | `tessdata_fast` 4.0.0 | Apache-2.0 |

The trained data was checked by its bytes and not by its URL: it is fetched
from a mirror, and gunzipped it hashes to the same git blob as
`tesseract-ocr/tessdata_fast@4.0.0`'s own `eng.traineddata`
(`bbef4675053b5b468cdb477053e28b1c698ba08e`, 4,113,088 bytes).

Three consequences, each with its discharge:

1. **`worker.min.js` names a notice file, and we now serve it.** Its first
   line is `/*! For license information please see worker.min.js.LICENSE.txt */`
   — a pointer *relative to wherever the script is served from*. Served from
   our origin without that file beside it, the pointer was a 404 for every
   recipient. It is now a fourth row on the shelf, answering at
   `/api/ocr/worker.min.js.LICENSE.txt`, and it holds the MIT and BSD-3-Clause
   notices for the buffer, ieee754, regenerator-runtime and zlib.js code
   bundled inside the worker — licences whose single condition is that the
   copyright notice travels with the copy.
2. **The Apache-2.0 text is at `licenses/Apache-2.0.txt`.** It covers the three
   files above and one more that is easy to miss: Vite bundles the
   main-thread half of `tesseract.js` into
   `web_dist/assets/reader.js`, which *is* committed, and the
   minifier drops the legal comments on the way. The licence has to be carried
   here because it is no longer carried there.
3. **Nothing about the reader renders, and that is correct rather than a
   dodge.** Apache-2.0 §4(d) — the clause that would require attribution
   *inside a display* — attaches only when the upstream work ships a `NOTICE`
   file. Neither package does: both `tesseract.js@7.0.0/NOTICE` and
   `tesseract.js-core@6.1.2/NOTICE` were requested and both 404. So §4(a) (a
   copy of the licence reaches recipients) and §4(b)-(c) (notices are retained
   in the copies) are the whole obligation, and commandment 10 is untouched.
   If a future version of either package gains a `NOTICE` file, this paragraph
   stops being true and the two commandments have to be squared. Re-check on
   any version bump rather than inheriting this.

## Fonts

Four woff2 faces are committed under `web/src/assets/fonts/`, all under the
**SIL Open Font License 1.1**, whose text is at `licenses/OFL-1.1.txt` with
both copyright statements at its head.
`web/src/assets/fonts/PROVENANCE.md` argues the choice of each face.

Verified 2026-08-19, three ways: against Google Fonts' own `OFL.txt` and
`METADATA.pb` for each family; against the licence bodies being identical
across the two foundries; and — the one that actually decides it — against the
binaries. Each woff2 carries its copyright and reserved font name in the
`name` table (nameID 0) and the OFL's URL beside it (nameID 14), which is the
"machine-readable metadata fields within binary files" the OFL's second
condition names. The notice therefore travels inside the font; the licence
text now travels beside it.

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
inside it**, which is distribution in the ordinary way. The hosted worker
(ADR 35, `Dockerfile.forge`) builds exactly such an image — and stays on the
right side of that line because the image is never published: it is pushed
only to the app's **private, org-scoped registry** (`registry.fly.io`) as a
deployment step, readable by nobody but this app's own account. GPLv3 §0 ties
"convey" to propagation "that enables other parties to make or receive
copies", and a private registry used to move a binary onto one's own server
enables no other party anything — the same argument that lets anyone `scp` a
GPL binary to their own host. What must therefore never happen is pushing
that image to a public registry, or making the Fly registry tag pullable by
others; if the image is ever published, pin the exact Forge version and ship
the corresponding source alongside it.

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
