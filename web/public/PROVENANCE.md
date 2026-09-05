# web/public -- provenance

The home-screen icons. The tarot deck's rule (`assets/tarot/PROVENANCE.md`)
applies here as everywhere: nothing ships in this directory whose origin is
not written down beside it.

## apple-touch-icon.png, icon-192.png, icon-512.png, icon-maskable-512.png

- **Source**: this project's own mark -- the tree over an open book, drawn as
  the inline SVG favicon in `web/index.html` (a dozen circles and paths,
  authored for this repository in #380). No upstream file, no third party,
  nothing of Wizards': the mark is original and carries the repository's own
  MIT licence.
- **Why PNG copies of a drawn SVG exist at all**: iOS reads
  `apple-touch-icon` and not the manifest's `icons`, and accepts no SVG for
  it -- `web/index.html` says so where the tag is set. The manifest icons
  follow the same form so every surface installs the same mark.
- **Derivation**: the drawn mark rasterized at 180 (apple-touch), 192 and
  512 square, over the app's parchment ground; `icon-maskable-512.png` is
  the same mark shrunk into the maskable safe zone so a round or squircle
  crop cannot cut the canopy. The rasterizer used on 2026-08-28 was not
  recorded at the time, which is why this file says so plainly: to
  regenerate, render the SVG in `web/index.html` at those sizes and lay it
  over the ground -- the drawn mark is the reference, and visual identity
  with it is the check.
- **Outside the animist gate, deliberately**: the pipeline exists to hold
  third-party sources to their licences, and there is no third party here.
  What holds this directory instead is the accounting sweep
  (`go/cmd/mtglab/mediaprovenance_test.go`): a picture with no record beside
  it fails the suite, which is how these four got this file.

The copies under `web_dist/` are the build's own, byte-identical; the build
carries what `web/public` holds.
