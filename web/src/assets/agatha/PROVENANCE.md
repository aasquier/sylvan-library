# agatha assets -- provenance

The rule here is the tarot deck's (`assets/tarot/PROVENANCE.md`): nothing ships in this directory whose licence was not checked per file, and every transformation applied is written down so the derivation is reproducible from the source.

<!-- animist:begin agatha-hut-plate -->
## agatha-hut-plate.webp

- **Source**: "The Magic Circle - John William Waterhouse.jpg", <https://commons.wikimedia.org/wiki/File:The_Magic_Circle_-_John_William_Waterhouse.jpg>, found via Commons, searching the painter and the painting by name -- which is the search that works for pictures and the one `campus.recipe.yaml` recorded. Three other subjects were fetched and proof-sheeted first.
.
- **Licence**: Public domain. Confirmed through the Wikimedia Commons API at fetch time (2026-09-12).
- **Transformations** (Pillow, scripted -- `animist build agatha-hut-plate.recipe.yaml`):
  - `crop`: frac_box=[0.0, 0.5574, 1.0, 0.9398].
  - `resize`: width=1280.
  - Encoded WEBP, quality 82.

Why committed rather than hotlinked: The room is staged on this painting: ingredients fall into its cauldron and the brew is drawn over its mouth, so it is the floor the whole composition stands on rather than decoration beside one. Scryfall art is hotlinked because the Fan Content Policy covers display and the credit line is part of the deal; this is a public-domain oil photographed by a museum, so committing the derived crop is clean -- and a room that goes blank when a third-party host moves a file is a room the app does not own.
<!-- animist:end agatha-hut-plate -->

<!-- authored: everything below is hand-written, outside the markers, and
     `animist build` will not rewrite it. It is the half no recipe can own. -->

## The credit, and where it renders

**John William Waterhouse, *The Magic Circle*, 1886. Tate, London.** Public
domain; the photograph is a faithful reproduction of a two-dimensional
public-domain work.

The painter and the painting are named **in the room itself**, under the
witch's opening line -- the same "The room wears ..." line the hotlinked
persona paintings carry (`web/src/components/theme.tsx`), extended here to name the
work and its year because this one is a whole painting rather than a card
crop. A picture is credited where it renders, and that rule does not soften
because the licence is easier.

## The plate is untreated, and the dusk is a stylesheet layer

The committed file is the crop and the resize and **nothing else**. No level,
no duotone, no darkening of any kind: the painting's own colour is intact,
which is what the room's green layer needs at runtime.

The painting is daylit and wants holding back -- mean luma 104 against the
seance room's 52 -- and it is held back in `web/src/index.css`, as
`.cauldron-dusk`: one `mix-blend-mode: multiply` radial of exactly the shape
`.seance-glass-dim` already is, turned inside out. Open over the cauldron,
closing to dusk at the frame's edges.

```css
.cauldron-dusk {
  position: absolute;
  inset: 0;
  pointer-events: none;
  mix-blend-mode: multiply;
  /* The five stops are a smoothstep written out; CSS interpolates a
     gradient linearly, and two stops alone put a visible shoulder across
     the sorceress. */
  background: radial-gradient(25% 68% at 70% 44%,
    rgba(0, 0, 0, 0)     55%,
    rgba(0, 0, 0, 0.106) 66.25%,
    rgba(0, 0, 0, 0.34)  77.5%,
    rgba(0, 0, 0, 0.574) 88.75%,
    rgba(0, 0, 0, 0.68)  100%);
}
```

A black multiply layer at alpha `a` composites to exactly `v * (1 - a)`, so
the stylesheet and a bake would be the same arithmetic -- which is *why* the
choice is free to be made on the right side of the line. `fabrica.recipe.yaml`
states that line for a tenebrist oil and it holds for a daylit one: *"a level
baked into the byte stream is a decision `index.css` can no longer argue
with"*. `velatio.recipe.yaml` is the nearer precedent still, being this exact
problem with the sign flipped -- the brightest picture in the coliseum, mean
152, answered with a tighter mask in the stylesheet rather than a level in the
file.

Every channel is scaled by the same factor, so **no hue moves**. Nothing is
added, nothing is keyed, nothing is sharpened or blurred: this is a shade laid
over a picture, which is the shape commandment 19 permits -- and the picture
underneath is ours in the first place, so the rule is being kept here out of
habit rather than obligation. Habit is the point.

### What it lands at, measured

Luma is ITU-R 601 (`PIL.Image.convert("L")`), the scale the coliseum recipe
headers quote. The house's own room scenes, measured 2026-09-12:

| plate | mean | p90 |
| --- | --- | --- |
| `seance/seance-room-still.webp` (the room analogue, 16:9) | 52.2 | 115.0 |
| `coliseum/fabrica.webp` (the forge) | 48.4 | 110.4 |
| `coliseum/templum.webp` (the colonnade) | 60.5 | 114.0 |
| `coliseum/ossarium.webp` | 64.0 | 128.0 |
| `coliseum/campus.webp` (the valley) | 71.7 | 133.0 |
| `coliseum/crypta.webp` (the vault) | 88.2 | 137.0 |
| `coliseum/via.webp` (the road) | 127.6 | 186.0 |
| `coliseum/velatio.webp` (the fresco -- its recipe calls this a cost) | 153.1 | 215.0 |

The dark cluster is the target and the seance still is its centre: **mean
48-64, p90 110-128.**

| | mean | p50 | p90 | p99 |
| --- | --- | --- | --- | --- |
| Waterhouse source, whole plate | 99.0 | -- | 139.0 | 190.1 |
| `agatha-hut-plate.webp`, as committed | 104.4 | 110.2 | 143.3 | 194.2 |
| the same plate under `.cauldron-dusk` | **54.1** | 39.8 | **116.1** | 179.6 |

**A flat multiply could not have done this**, and the reason is a fact about
the painting. Waterhouse's histogram is narrow -- p90/mean is 1.37, against
the seance still's 2.21 -- because the whole canvas is lit daylight-even. A
uniform scale preserves that ratio exactly, so the one that lands the mean at
55 lands the p90 at **75.5**: well under every shipped scene, a room with no
sparkle in it at all. The gradient is what pulls the two numbers apart, and it
can only do that because **62% of the plate's bright decile already falls
inside the dusk's own ellipse** (33% inside its fully-open core). The rest of
that decile is her dress and the flowers, and those are what go to dusk.

## The hotspot, and how it was measured

The cauldron's mouth, as percentages of the 1280x720 frame -- the same
contract as `--ball-x`/`-y`/`-d`/`-dy` in `web/src/index.css`, and the only
four numbers in the room that mean anything:

```css
--pot-x: 66.0%;    /* centre of the mouth, across the frame */
--pot-y: 36.0%;    /* centre of the mouth, down the frame */
--pot-d: 23.0%;    /* the mouth's MAJOR axis, over the frame's width */
--pot-dy: 6.54%;   /* the mouth's MINOR axis, over the frame's height */
```

**`--pot-dy` does not mean what `--ball-dy` means, and the difference is
real.** The seance's sphere is a circle, so its two numbers are one circle
read against two axes and `--ball-dy` is exactly `--ball-d * 16/9` (22.9 ->
40.7, which checks). A cauldron seen from a standing person's height is an
**ellipse**, 0.16 as tall as it is wide. The same two CSS properties still
describe it -- a `width`/`height` pair is a width and a height -- but the
identity is gone. Both readings were recorded because they answer different
questions: **40.9%** is the circle that hangs over the pot, which is what a
glow or a rising column of steam wants; **6.54%** is the mouth's own opening,
which is what anything that lands *in* the brew wants.

Measured, not eyeballed, and it took three passes because the obvious two fail
on this painting:

1. A warmth-times-luma heat map (`fabrica`'s own statistic) finds the glow but
   not the pot: the whole canvas is warm ochre, so its bounding boxes run edge
   to edge.
2. A saturation threshold fails for the same reason in reverse -- the ground
   measures *more* saturated (0.571) than the brew (0.541).
3. What worked: the rim region contrast-stretched to its 2nd/70th percentile
   and gridded at 0.01 of the source, which makes the shadowed left end of the
   rim visible, and the edges read off that grid -- then confirmed by drawing
   the resulting ellipse back onto the plate, where it lands on the brass.

The mouth in source fractions of the 2000x2942 painting: **x 0.545 to 0.775,
y 0.6825 to 0.7075.** Centre (0.660, 0.695); major axis 460 source pixels. The
ember bed's centroid, found by the same heat statistic restricted to the
finished frame, is at (67.8%, 52.6%) -- which is what the dusk's hold is
centred between.

**Re-measure all four against the first frame of any footage that replaces
this plate, and nothing else.** Every effect in the room is anchored to them,
so the splash lands in the pot at every window width, and the folded strip is
the same composition cropped to a band around the same four numbers rather
than a second measurement.
