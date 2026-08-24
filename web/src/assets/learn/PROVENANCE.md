# learn assets -- provenance

The rule here is the tarot deck's (`assets/tarot/PROVENANCE.md`): nothing ships in this directory whose licence was not checked per file, and every transformation applied is written down so the derivation is reproducible from the source.

<!-- animist:begin bookworm -->
## bookworm-loop.mp4, bookworm-loop.webm, bookworm-still.webp

- **Source**: "Carl Spitzweg - "The Bookworm".jpg", <https://commons.wikimedia.org/wiki/File:Carl_Spitzweg_-_%22The_Bookworm%22.jpg>, found via Commons search; the Museum Georg Schäfer original, not the Grohmann or Schweinfurt variants.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-08-16).
- **Transformations** (Pillow, scripted -- `mtglab animist build bookworm.recipe.yaml`):
  - `resize`: width=576, height=1072.
  - `ken_burns`: frames=192, fps=16, zoom_from=1.02, zoom_to=1.12, pan_from=[0.5, 0.6], pan_to=[0.5, 0.4], bounce=True.
  - Encoded WEBM, crf 40.
  - `resize`: width=576, height=1072.
  - `ken_burns`: frames=192, fps=16, zoom_from=1.02, zoom_to=1.12, pan_from=[0.5, 0.6], pan_to=[0.5, 0.4], bounce=True.
  - Encoded MP4, crf 30.
  - `resize`: width=576, height=1072.
  - `ken_burns`: frames=192, fps=16, zoom_from=1.02, zoom_to=1.12, pan_from=[0.5, 0.6], pan_to=[0.5, 0.4], bounce=True.
  - Encoded WEBP, quality 72.

Why committed rather than hotlinked: Public domain -- Spitzweg died in 1885, and Commons records the file as such (the gate confirms it at fetch time, per file, through the API). Committed rather than hotlinked for the same reason as the tarot: a page decoration that breaks when a third-party CDN moves is a decoration the app does not own.
<!-- animist:end bookworm -->
