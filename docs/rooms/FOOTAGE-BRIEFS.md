# The rooms' footage briefs

The Seedance briefs each interview room's loop was, or will be, rendered
from. Three are rendered and shipped (tavern, campfire, hut — see each
directory's `PROVENANCE.md` under `web/src/assets/`); three wait on credits
(therapist, scientist, chef). The cutting recipe — seam search, half-speed
stretch, encodes, the numpy audio bed — is the séance room's, recorded in
`web/src/assets/seance/PROVENANCE.md` and applied in the three new records.

Two lessons from the first renders, folded into the briefs below: "seamless
loop, last frame matching first" does nothing, the steady-state paragraph is
what loops; and a one-shot prompt's second markers are not honoured, so the
quiet state must be the *ending* and the last event the one whose colour may
linger.

## Render settings, every clip

- 16:9, 720p. 15 s. 24 fps.
- Audio on. The video ships muted; the track is stripped, looped and played behind the table-sound switch.
- Loops are text-to-video. The witch's three reaction clips are image-to-video from the chosen idle loop's still frame.
- Two or three takes per prompt. We pick by the seam (any two frames 12 apart that match), by luma (mean ~50, p90 ~115), and by whether the audio is a clean bed.

## Therapist — "Talk it through"

```
A quiet consulting room at dusk, photographed from a client's low armchair. Directly across, an empty worn brown leather armchair with a soft cushion, angled slightly toward the camera. Beside it a tall floor lamp with a warm linen shade, the only light in the room, amber on the leather. Behind the chair a tall sash window, deep blue twilight outside, steady rain running down the glass, a sheer white curtain lifting gently in a draught. A small side table with a plain tissue box, a glass of water and a cup of tea with steam rising. A potted fern in the corner, bookshelves in shadow with unlabelled spines. No clock anywhere. Photo-real, cinematic, fine film grain, very dark room with a single warm practical light and a cool blue window. The only motion is rain on the glass, the curtain breathing and the steam. The scene is in a steady state for the entire clip: the lighting, the amount of steam and the position of every object are the same at the end as at the beginning. Nothing builds, fades, appears, disappears or changes shape. No events happen; the motion is continuous and repetitive. The camera is locked off: no push-in, no zoom, no handheld sway, no rack focus. No people, no text, no logos. Audio: quiet continuous ambient room tone only, soft rain on glass and a faint lamp hum, steady from start to finish. No music, no voices, no footsteps, no sudden sounds, nothing rhythmic, nothing that happens once.
```

Quiet zone: the dark wall above the chair, left of the lamp.

## Scientist — "Study me"

```
Inside a naturalist's canvas field tent at night, in a rainforest, photographed from a seat at a wooden folding table. On the table, turned toward the camera, an open leather field notebook with illegible pencil sketches of insects and leaves, a pencil laid across the page. A brass hurricane lantern on the table is the only light, warm amber, flame flickering. Around it: glass specimen jars with beetles and leaves, a brass microscope, a coiled tape measure, a large fossilised tooth used as a paperweight. Two or three pale moths circle the lantern without ever landing. The tent flap behind is half open on a teal-green jungle night, rain pattering on the canvas, a thin mist outside. Photo-real, cinematic, fine film grain, very dark with a single warm practical light and a cool green exterior. The only motion is the lantern flame, the circling moths, rain on canvas and the flap lifting slightly. The scene is in a steady state for the entire clip: the lighting, the mist and the position of every object are the same at the end as at the beginning. Nothing builds, fades, appears, disappears or changes shape. No events happen; the motion is continuous and repetitive. The camera is locked off: no push-in, no zoom, no handheld sway, no rack focus. No people, no readable text, no logos. Audio: quiet continuous ambient room tone only, rain pattering on canvas, the lantern's soft hiss and distant insects, steady from start to finish. No music, no voices, no footsteps, no sudden sounds, nothing rhythmic, nothing that happens once.
```

Quiet zone: the plain canvas wall on the left.

## Chef — "Cook for me"

```
A restaurant kitchen pass an hour before service, photographed from a counter stool. In the foreground a scrubbed steel and copper counter with one empty white plate set for the viewer, a wooden board with chopped herbs and a chef's knife, a small bowl of flour. Behind the pass the kitchen is dim: a cast-iron range with low blue gas flames, a tall stockpot steaming, copper pans hanging above, a ball of dough proofing under a linen cloth, a rustic pie cooling on a rack. The only light is the orange heat lamps over the pass and the glow of the range. Photo-real, cinematic, fine film grain, very dark with warm amber practical light. The only motion is steam from the stockpot, the gas flames and faint heat shimmer under the lamps. The scene is in a steady state for the entire clip: the lighting, the amount of steam and the position of every object are the same at the end as at the beginning. Nothing builds, fades, appears, disappears or changes shape. No events happen; the motion is continuous and repetitive. The camera is locked off: no push-in, no zoom, no handheld sway, no rack focus. No people, no readable text, no logos. Audio: quiet continuous ambient room tone only, a stockpot at a low simmer, gas flames and faint far-off kitchen clatter, steady from start to finish. No music, no voices, no footsteps, no sudden sounds, nothing rhythmic, nothing that happens once.
```

Quiet zone: the dark extractor hood and the depth of the kitchen above the pass.

## Storyteller — "Trade stories"

```
A night campfire in a snowy northern clearing, photographed from a seat on the ground on one side of the fire. The campfire is large and prominent, just below centre frame, a stack of birch logs burning low and steady with glowing embers and a few sparks rising, not a roaring blaze. Across the fire an empty log bench with a heavy fur cloak thrown over it and a carved drinking horn set down beside it. Beyond, a dark pine treeline under a star-filled sky, and above the trees a slow green and violet aurora. Light snow drifting. A ring of flat grey stones around the fire. The campfire is the only light, warm orange on the snow and the bench. Photo-real, cinematic, fine film grain, very dark outside the firelight. The only motion is the steady fire, the rising sparks, the slow shimmer of the aurora and drifting snow. The scene is in a steady state for the entire clip: the size of the fire, the brightness of the aurora and the position of every object are the same at the end as at the beginning. No log shifts or collapses. Nothing builds, fades, appears, disappears or changes shape. No events happen; the motion is continuous and repetitive. The camera is locked off: no push-in, no zoom, no handheld sway, no rack focus. No people, no text, no logos. Audio: quiet continuous ambient room tone only, the fire's soft crackle and a low wind through pines, steady from start to finish. No music, no voices, no footsteps, no sudden sounds, nothing rhythmic, nothing that happens once.
```

Quiet zone: the night sky on the side away from the aurora.

## Barkeep — "Pull up a stool"

```
A medieval tavern at the quiet hour before the evening rush, photographed from a stool at the bar. In the foreground a thick dark oak bar top, worn smooth, with a pewter tankard and a clear glass set down beside a folded white polishing cloth, and a stubby candle in a bottle. Behind the bar, shelves of dark glass bottles and clay jugs lit from below by candlelight, wooden casks with brass taps, a row of pewter mugs hanging from hooks. To one side a stone hearth with a low fire. A small leaded window shows blue dusk and rain. Low timber beams, candle sconces, the whole room in candle and firelight only. Photo-real, cinematic, fine film grain, very dark with warm amber practical light and a cool blue window. The only motion is the candle flames, the hearth glow, rain on the window and faint drifting dust in the light. The scene is in a steady state for the entire clip: the lighting and the position of every object are the same at the end as at the beginning. Nothing builds, fades, appears, disappears or changes shape. No events happen; the motion is continuous and repetitive. The camera is locked off: no push-in, no zoom, no handheld sway, no rack focus. No people, no readable text, no logos, no signs. Audio: quiet continuous ambient room tone only, a hearth crackling, rain on the window and the faint flutter of candle flames, steady from start to finish. No music, no voices, no footsteps, no sudden sounds, nothing rhythmic, nothing that happens once.
```

Quiet zone: the dark beams above the bottle shelf.

## Witch — "Brew me something", idle loop

```
The inside of a witch's hut at night, photographed from just inside the doorway at standing height. Centre stage, dead centre of frame, a large black iron cauldron on a tripod over a low wood fire, seen from slightly above so its round open mouth is visible. The brew inside is a dark murky umber, barely lit, simmering with slow fat bubbles and thick pale steam rising. Red embers glow beneath the pot and are the room's main light, warm on the iron and the earth floor. Around the walls in shadow: bundles of dried herbs hanging from the beams, shelves of glass jars and bottles, a wicker cage, a stool with a wooden ladle leaning against the pot. A small window shows a green-tinged moonlit wood. Photo-real, cinematic, fine film grain, very dark with one warm practical light under the pot. The only motion is the simmering surface, the steam and the embers breathing. The scene is in a steady state for the entire clip: the colour of the brew, the amount of steam, the glow of the embers and the position of every object are the same at the end as at the beginning. Nothing builds, fades, appears, disappears or changes shape. No events happen; the motion is continuous and repetitive. The camera is locked off: no push-in, no zoom, no handheld sway, no rack focus. No people, no text, no logos. Audio: quiet continuous ambient room tone only, slow thick bubbling and embers ticking, steady from start to finish. No music, no voices, no footsteps, no sudden sounds, nothing rhythmic, nothing that happens once.
```

Quiet zone: the dark beams and herb bundles above the pot. Reduced-motion still: a frame with the steam thinnest over the mouth; the mouth's four anchors get re-measured against it.

## Witch, three reactions — image-to-video from the idle still

Pick the idle take first and export one still from it. Each reaction starts from that image and must return to it. Same 15 s; we trim.

**The Base**

```
Continue this exact scene with the camera perfectly still. For the first two seconds nothing changes. Then a handful of dried rose petals and small berries falls from above into the centre of the cauldron. The brew swallows them with a soft splash, the steam turns briefly rose-pink and lit from within, and a warm golden glow blooms up out of the pot and plays across the herb bundles and jars on the walls. The glow fades, the steam returns to pale, and the brew settles back to its dark murky simmer. The last three seconds look identical to the first frame: same brew colour, same steam, same ember glow, every object in the same place. No camera movement, no zoom, no push-in. No people, no text. Audio: the soft bubbling continues throughout; a gentle splash and a brief hiss of steam as the petals go in, then only the bubbling again. No music, no voices.
```

**The Heat**

```
Continue this exact scene with the camera perfectly still. For the first two seconds nothing changes. Then a pinch of dark powder and a dried red chilli fall from above into the centre of the cauldron. The fire beneath flares up, orange flame licking the sides of the pot, a burst of sparks and a column of steam shooting up, the whole hut flashing hot orange with hard shadows thrown across the walls. The flare dies back to embers, the sparks drift out, and the brew settles back to its dark murky simmer. The last three seconds look identical to the first frame: same brew colour, same steam, same ember glow, every object in the same place. No camera movement, no zoom, no push-in. No people, no text. Audio: the soft bubbling continues throughout; a sharp sizzle and a whoosh of flame as the powder goes in, then only the bubbling again. No music, no voices.
```

**The Binding**

```
Continue this exact scene with the camera perfectly still. For the first two seconds nothing changes. Then a knot of twisted roots and a sprig of green leaves falls from above into the centre of the cauldron. The surface ripples outward, the steam turns a luminous green and rises in a slow spiral, and an eerie green light climbs the walls while the shadows of the hut lean inward toward the pot. The green light sinks back down into the brew, the steam returns to pale, and the brew settles back to its dark murky simmer. The last three seconds look identical to the first frame: same brew colour, same steam, same ember glow, every object in the same place. No camera movement, no zoom, no push-in. No people, no text. Audio: the soft bubbling continues throughout; a low splash and a long slow hiss of steam as the roots go in, then only the bubbling again. No music, no voices.
```

## What each render owes before it ships

Per directory, the séance's provenance block: the prompt, the date, the licence paragraph re-checked rather than inherited, the per-clip content judgement, and the exact ffmpeg seam and encode parameters. `go/cmd/mtglab/mediaprovenance_test.go` reads that index.
