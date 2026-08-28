/**
 * A keyword's sign, for the corner of a card on the battlefield.
 *
 * **The problem these solve is that a board is unreadable at fifty-eight
 * pixels.** A creature's painting tells you it is a Dragon; it does not tell
 * you the Dragon flies, and whether the Dragon flies is the whole question
 * when the other player has ground blockers. Every digital client answers this
 * the same way — Arena puts a small sign in the card's corner (Aaron,
 * 2026-08-25: *"first strike is a single sword, double strike two swords,
 * flying is a wing, lifelink a heart, vigilance a castle, trample a stylized
 * dinosaur footprint"*), and it is the right answer because the alternative is
 * hovering forty cards one at a time to find the one that can block.
 *
 * **Drawn, not photographed, and that is a size argument rather than a taste
 * one** — the same call `CrossedSwordsGlyph` makes one file over. These play
 * at ten pixels. A museum plate at ten pixels is a smudge; a silhouette is
 * legible. Commandment 5 forbids clip art, not drawing.
 *
 * So every mark here is built to survive being ten pixels tall, and that
 * governs the drawing more than any other consideration:
 *
 * - **One shape, filled, no strokes thinner than about 1.5 units** in a 20-unit
 *   box. A hairline at this scale is a grey suggestion.
 * - **The silhouette carries the meaning.** Interior detail is gone by 12px, so
 *   nothing may depend on it: the wing reads as a wing from its outline alone,
 *   and the feather notches are a bonus at the sizes where they survive.
 * - **No two marks share a silhouette.** A shield and a dome are the same blob
 *   small, so indestructible takes the shield and hexproof takes a dome *with a
 *   ground line under it* — the line is what separates them, and it is thick.
 *   Deathtouch's skull is the third member of that family and the hardest: by
 *   outline it is both of the others. It is separated by what it has that they
 *   have not, which is *holes* — see the argument on the drawing itself. When
 *   a fourth blob wants to join, that is the move to reach for; a fourth
 *   outline is not available.
 *
 * **Which keywords get a sign is `lib/keywords.ts`'s list**, and the map at the
 * foot of this file is typed against it — so a word on that list with no
 * drawing here fails the typecheck, and a drawing here for a word nobody listed
 * fails too. A set of names and a set of pictures that are supposed to be the
 * same set is exactly the pair that quietly stops being the same set.
 *
 * **A keyword something else is giving the creature is struck the other way
 * round**, and the distinction is in the *plate* rather than in the drawing.
 * The rule above — no two marks share a silhouette — is exactly what forbids
 * the obvious answer here: there are fifteen drawings and each one is already
 * the only shape it is allowed to be, so a granted vigilance cannot be a
 * *different castle*. What is free is the ground the castle stands on, so the
 * plate and the ink change places: printed marks are brass on a dark chip,
 * granted ones a dark mark on a brass chip. That is a swap of *value*, not of
 * colour — it survives a greyscale print, a colour-blind reader and both
 * themes, which no hue would — and it is the largest difference available in
 * a thirteen-pixel square. `.field-keyword.is-granted` is the two lines of
 * stylesheet that do it. **Nothing names the giver**: Forge carries no source
 * for a granted keyword, so there is nothing to name and inventing one would
 * be the board making a reading of the game (Aaron, 2026-08-27: *"we don't
 * need to say who granted the ability if it is not traceable"*).
 *
 * **Each mark is a button, and that is not decoration either.** A drawing this
 * size cannot carry its own meaning to somebody meeting Magic here, so every
 * one of them explains itself in a panel — see `components/hint.tsx` for why
 * the `title` attribute it replaces was never an answer. The drawing is still
 * ten pixels; the *target* under it is not, because a thirteen-pixel chip is
 * not something a thumb can hit.
 */

import type { ReactElement } from 'react'

import { type DrawnKeyword, drawableKeywords, keywordSaid, keywordWords }
  from '../lib/keywords'
import { FieldHint } from './hint'

/** One mark, drawn on `currentColor` in a 20-unit box like the rest of the
 *  house's glyphs. */
type Glyph = () => ReactElement

/** Flying: a falcon head on, wings swept back into scythes.
 *
 * **Four drawings died to get here, and the post-mortem is the useful part**
 * (Aaron, 2026-08-25, across four rounds: a notched wing, separated feathers,
 * a talaria, then a head-on bird that was *"on the right track"* but *"I don't
 * look at that and think bird"*).
 *
 * The first three all made the same mistake in different costumes: they asked
 * a **wing** to mean *flying*. A wing cannot, at this size. A tapered shape
 * with a stem is a leaf, a petal, a flame or a feather depending entirely on
 * detail that does not survive half a pixel per unit — so every fix was more
 * detail, and every fix blurred back into the same ambiguous blade.
 *
 * The fourth failed differently and more usefully: a whole bird, but drawn as
 * wings plus a lump, and **a lump with wings is a paper plane**. What a plane
 * has not got is a *tail* and a *beak*, and of those the tail is the valuable
 * one — a fan is a large, low-frequency shape, which is exactly the kind of
 * feature that survives when fine ones cannot.
 *
 * So: a body, a beaked head, a forked tail, and wings **swept back into
 * scythes**. The sweep is the load-bearing choice. Nothing built by people has
 * that outline — an aircraft's wing is straight or delta, never concave along
 * the trailing edge — so the silhouette reads as *alive* before it resolves
 * into *bird*, and it keeps reading that way at ten pixels, where the tail and
 * the beak have already gone.
 *
 * The general rule this file learned, worth keeping: **at ten pixels, choose
 * the subject whose whole outline is unmistakable, not the subject that is
 * literally the thing you mean.** A wing is what flying *is*; a falcon is what
 * flying *looks like*.
 */
const Flying: Glyph = () => (
  <>
    {/* The wings. Tip forward and up, trailing edge scooped back — the
        concave sweep no aircraft has. */}
    <path d="M9.2 6.6 C7 4.7 4.2 3 1.2 2.1 C2.6 5.4 4.6 8.3 7 10.5
             C8 11.3 9.2 11.1 9.2 9.9 Z"
          fill="currentColor" />
    <path d="M10.8 6.6 C13 4.7 15.8 3 18.8 2.1 C17.4 5.4 15.4 8.3 13 10.5
             C12 11.3 10.8 11.1 10.8 9.9 Z"
          fill="currentColor" />
    {/* The body, and the forked tail behind it. */}
    <path d="M9 6 C8.8 8.6 9 11 9.3 13.2 L10.7 13.2 C11 11 11.2 8.6 11 6 Z"
          fill="currentColor" />
    <path d="M9.3 12.8 L8.6 17.6 L10 16.5 L11.4 17.6 L10.7 12.8 Z"
          fill="currentColor" />
    {/* Head and beak. Both are gone by ten pixels; they are what makes the
        mark right at twenty and forty, where somebody actually looks at it. */}
    <circle cx="10" cy="4.8" r="1.6" fill="currentColor" />
    <path d="M9 3.9 L10 2.1 L11 3.9 Z" fill="currentColor" />
  </>
)

/** One sword, point up. The crossguard is the half that makes it a sword
 *  rather than a stroke, and it is drawn heavier than a real one — the same
 *  lesson `CrossedSwordsGlyph` records. */
function bladeAt(x: number, tilt: number) {
  return (
    <g transform={`translate(${x} 10) rotate(${tilt})`}>
      <path d="M-1.5 -1 L1.5 -1 L1.5 -6.6 L0 -9.4 L-1.5 -6.6 Z"
            fill="currentColor" />
      <rect x="-4" y="-0.9" width="8" height="2.1" rx="0.8"
            fill="currentColor" />
      <rect x="-1" y="0.9" width="2" height="6.2" rx="0.9"
            fill="currentColor" />
      <circle cx="0" cy="8" r="1.5" fill="currentColor" />
    </g>
  )
}

const FirstStrike: Glyph = () => bladeAt(10, 0)

/** Double strike: two of them, splayed. Not the Coliseum's crossed pair —
 *  that mark is the gate's and means *a match*, and two marks meaning two
 *  things must not look alike. */
const DoubleStrike: Glyph = () => (
  <>
    {bladeAt(6.6, -13)}
    {bladeAt(13.4, 13)}
  </>
)

/** Deathtouch: a skull.
 *
 *  **A skull, and deliberately not a skull and crossbones** (Aaron, 2026-08-27,
 *  who offered both). The crossbones are the poison pictogram — the label on a
 *  bottle, the flag over a hold — and this board now has a keyword that
 *  genuinely means poison standing directly underneath. `Bilious Skulldweller`
 *  is a real card with deathtouch *and* toxic, so the two marks stack in one
 *  corner, and a pair where one is a viper and the other is the international
 *  sign for venom is a pair that says the same thing twice. Stripped of the
 *  bones the skull stops meaning *poison* and goes back to meaning *death*,
 *  which is the rule: any damage at all from this creature is lethal.
 *
 *  **Three holes are what make it a skull rather than a shield**, and that is
 *  the whole drawing. A skull's gross outline — broad at the top, narrowing at
 *  the bottom — is `Indestructible`'s heater shield, and a dome is `Hexproof`;
 *  the file's own rule says no two marks may share a silhouette, and by
 *  outline alone this one would share two. What no shield has is *voids*: two
 *  large sockets and a nasal aperture, punched clean through, so the mark is a
 *  mass with dark inside it rather than a solid one. They are drawn enormous by
 *  anatomical standards for the same reason the falcon's wings are swept — a
 *  big, low-frequency feature is the kind that is still there at ten pixels,
 *  where a correct one is a grey suggestion. The second discriminator is the
 *  foot: a shield tapers to a point and a skull ends in a jaw, so the bottom
 *  edge is a flat scallop of teeth rather than a tip.
 *
 *  **The sockets are holes, not dots**, by `evenodd` — the same mechanism and
 *  the same reason as the viper's eye below. On a card's corner they show the
 *  chip; anywhere else they show whatever is behind, which is what a hole in a
 *  bone actually does.
 *
 *  **It shares a subject with `assets/coliseum/memento.webp` and not a shape,
 *  and the sharing is the point.** The stage draws that Pompeian mosaic over a
 *  creature that has died. This is a skull too, and a player who has seen the
 *  one will read the other instantly — the room means *death* by *skull*, on
 *  the stage and in a corner, which is a vocabulary rather than a collision.
 *  Nothing about them can be confused: the memento is deliberately uncut, so
 *  its silhouette is a rectangle of Roman floor, it is photographic where this
 *  is a cut line, it covers the whole card where this is ten pixels of its
 *  corner, and it lives for a second where this stands as long as the creature
 *  does. One says *something died here*; the other says *this is what kills
 *  it*. */
const Deathtouch: Glyph = () => (
  <path d="M10 1.7
           C14.6 1.7 17.3 4.5 17.3 8.7
           C17.3 11.1 16.5 12.6 15.5 13.2
           C14.6 13.6 13.9 13.9 13.85 14.8
           C13.85 15.5 13.8 16.2 13.8 16.7
           C13.3 18.6 11.77 18.6 11.27 16.7
           C10.77 18.6 9.23 18.6 8.73 16.7
           C8.23 18.6 6.7 18.6 6.2 16.7
           C6.2 16.2 6.15 15.5 6.15 14.8
           C6.1 13.9 5.4 13.6 4.5 13.2
           C3.5 12.6 2.7 11.1 2.7 8.7
           C2.7 4.5 5.4 1.7 10 1.7 Z
           M11.6 7.1 L15.1 6.9
           C15.55 6.9 15.65 7.4 15.5 8.1
           C15.2 10 14.5 11.35 13.4 11.35
           C12.25 11.35 11.5 10.3 11.45 8.7
           C11.42 7.9 11.45 7.35 11.6 7.1 Z
           M8.4 7.1 L4.9 6.9
           C4.45 6.9 4.35 7.4 4.5 8.1
           C4.8 10 5.5 11.35 6.6 11.35
           C7.75 11.35 8.5 10.3 8.55 8.7
           C8.58 7.9 8.55 7.35 8.4 7.1 Z
           M10 10.6
           C10.3 11.7 11.05 12.9 11.28 13.55
           C11.5 14.2 11.05 14.5 10 14.5
           C8.95 14.5 8.5 14.2 8.72 13.55
           C8.95 12.9 9.7 11.7 10 10.6 Z"
        fill="currentColor" fillRule="evenodd" />
)

/** Toxic: a snake, with the venom coming off its fang.
 *
 *  **This drawing was deathtouch's first, and it was always really about
 *  poison** (Aaron, 2026-08-27: *"lets move our current deathtouch icon to be
 *  the new symbol for when something has the toxic keyword"*). It earned the
 *  move rather than being handed it. The argument that produced it holds
 *  exactly as written — *a fang alone was not enough* (Aaron, 2026-08-25), a
 *  lone tooth with a drop under it is a dentist, and the keyword is not about a
 *  tooth but about the thing the tooth belongs to — and the drop at the end of
 *  that fang is the part that was always straining under the old word. Nothing
 *  drips off deathtouch; deathtouch is an instant, and the creature is simply
 *  gone. Toxic is the keyword that *leaves something behind* — a poison counter
 *  that stays on a player for the rest of the game — and a falling drop is that
 *  sentence said as a picture.
 *
 *  Laid out so nothing has to share room with anything: the head takes the top
 *  left, the body coils away to the right, and the whole left edge below the
 *  snout is left clear for the fang and the drop — which are the two shapes
 *  that carry the meaning and the two that must not be crowded. */
const Toxic: Glyph = () => (
  <>
    {/* The body, coiling away behind the head. A stroke rather than a filled
        outline: at this size a snake is a thick line that bends, and the bend
        is the whole of what says snake. */}
    <path d="M10.4 6 C14.6 5.6 17.2 8 16.8 10.8 C16.4 13.6 13 14.2 12 16
             C11.4 17.1 11.8 17.6 12.8 17.9"
          stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"
          fill="none" />
    {/* The head: a wedge, snout to the left. Wider at the back than the front,
        which is the one proportion that separates a snake's head from a worm's
        end.

        **The eye is a hole, not a black dot**, and the difference matters
        because this mark has two grounds: a dark chip on the board and
        whatever a proof sheet or a future surface puts behind it. A dot
        painted black is black on both; a hole is *the ground*, which is what
        an eye punched out of a silhouette actually is. `evenodd` with the
        circle as a second subpath is the whole mechanism. */}
    <path d="M8.8 2.6 C10.6 2.6 11.6 3.8 11.6 5.2 C11.6 6.7 10.6 7.7 8.8 7.7
             C6.4 7.7 3.2 6.7 3.2 5.15 C3.2 3.6 6.4 2.6 8.8 2.6 Z
             M7.3 4.6 A1.35 1.35 0 1 0 4.6 4.6 A1.35 1.35 0 1 0 7.3 4.6 Z"
          fill="currentColor" fillRule="evenodd" />
    {/* The fang, and the drop falling off it. */}
    <path d="M3.6 6.3 L5.5 6.9 L4.3 9.9 Z" fill="currentColor" />
    <path d="M4.3 11.3 C5.6 12.8 6.2 13.7 6.2 14.5 A1.9 1.9 0 0 1 2.4 14.5
             C2.4 13.7 3 12.8 4.3 11.3 Z"
          fill="currentColor" />
  </>
)

/** Lifelink: a heart. The one mark here that needed no argument. */
const Lifelink: Glyph = () => (
  <path d="M10 17.4 C4.4 13.4 1.6 10.4 1.6 7.4 A4.4 4.4 0 0 1 10 5.5
           A4.4 4.4 0 0 1 18.4 7.4 C18.4 10.4 15.6 13.4 10 17.4 Z"
        fill="currentColor" />
)

/** Vigilance: a crenellated tower — the castle that does not lie down. */
const Vigilance: Glyph = () => (
  <path d="M3 6.4 L3 3.2 L6 3.2 L6 5 L8.5 5 L8.5 3.2 L11.5 3.2 L11.5 5
           L14 5 L14 3.2 L17 3.2 L17 6.4 L15.4 7.6 L15.4 17 L4.6 17
           L4.6 7.6 Z"
        fill="currentColor" />
)

/** Trample: a three-toed footprint, pressed. Aaron's own image, and the right
 *  one — a footprint is *something having gone through*, which is what the
 *  keyword does. */
const Trample: Glyph = () => (
  <>
    <path d="M10 8.4 C13.4 8.4 15.8 11 15.8 14 C15.8 16.4 13.4 18 10 18
             C6.6 18 4.2 16.4 4.2 14 C4.2 11 6.6 8.4 10 8.4 Z"
          fill="currentColor" />
    <path d="M4.6 3.4 C6.1 3.4 7 4.7 7 6.2 C7 7.7 6.1 8.8 4.6 8.8
             C3.1 8.8 2.2 7.7 2.2 6.2 C2.2 4.7 3.1 3.4 4.6 3.4 Z"
          fill="currentColor" />
    <path d="M10 1.6 C11.5 1.6 12.4 2.9 12.4 4.5 C12.4 6.1 11.5 7.2 10 7.2
             C8.5 7.2 7.6 6.1 7.6 4.5 C7.6 2.9 8.5 1.6 10 1.6 Z"
          fill="currentColor" />
    <path d="M15.4 3.4 C16.9 3.4 17.8 4.7 17.8 6.2 C17.8 7.7 16.9 8.8 15.4 8.8
             C13.9 8.8 13 7.7 13 6.2 C13 4.7 13.9 3.4 15.4 3.4 Z"
          fill="currentColor" />
  </>
)

/** Haste: a bolt. */
const Haste: Glyph = () => (
  <path d="M11.6 1.4 L4.2 11.2 L8.8 11.2 L7.4 18.6 L15.6 8.2 L10.6 8.2 Z"
        fill="currentColor" />
)

/** Menace: two arrowheads closing on one thing, which is the rule said as a
 *  picture — it takes two to block. */
const Menace: Glyph = () => (
  <>
    <path d="M1.2 10 L6.4 5.4 L6.4 8.4 L8.6 8.4 L8.6 11.6 L6.4 11.6 L6.4 14.6 Z"
          fill="currentColor" />
    <path d="M18.8 10 L13.6 14.6 L13.6 11.6 L11.4 11.6 L11.4 8.4 L13.6 8.4
             L13.6 5.4 Z"
          fill="currentColor" />
  </>
)

/** Reach: a web in the corner, which is where a spider waits for the thing
 *  that thought it was safe up there. */
const Reach: Glyph = () => (
  <>
    <path d="M2.6 2.6 L17.4 2.6 M2.6 2.6 L2.6 17.4 M2.6 2.6 L14.2 14.2
             M2.6 2.6 L8.4 16.2 M2.6 2.6 L16.2 8.4"
          stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"
          fill="none" />
    <path d="M8.6 2.6 A6 6 0 0 1 2.6 8.6 M14.6 2.6 A12 12 0 0 1 2.6 14.6"
          stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"
          fill="none" />
  </>
)

/** Hexproof: a dome over a ground line. The line is doing real work — a dome
 *  without it is a shield, and indestructible has the shield. */
const Hexproof: Glyph = () => (
  <>
    <path d="M2.4 13.4 A7.6 7.6 0 0 1 17.6 13.4 L14.6 13.4
             A4.6 4.6 0 0 0 5.4 13.4 Z"
          fill="currentColor" />
    <rect x="1.8" y="15.4" width="16.4" height="2.6" rx="1.3"
          fill="currentColor" />
  </>
)

/** Indestructible: a shield, solid and heavy. */
const Indestructible: Glyph = () => (
  <path d="M10 1.8 L17.4 4.4 C17.4 11.4 14.6 16.2 10 18.4
           C5.4 16.2 2.6 11.4 2.6 4.4 Z"
        fill="currentColor" />
)

/** Ward: a sigil — a diamond inside a diamond, which is what a rune drawn to
 *  keep something out looks like in every tradition that has one. */
const Ward: Glyph = () => (
  <path d="M10 1.4 L18.6 10 L10 18.6 L1.4 10 Z
           M10 6.2 L5.8 10 L10 13.8 L14.2 10 Z"
        fill="currentColor" fillRule="evenodd" />
)

/** Defender: a wall, coursed the way a wall is coursed. */
const Defender: Glyph = () => (
  <path d="M2 4.4 H18 V8.2 H2 Z
           M2 9.4 H8.6 V13.2 H2 Z  M9.8 9.4 H18 V13.2 H9.8 Z
           M2 14.4 H12 V18.2 H2 Z  M13.2 14.4 H18 V18.2 H13.2 Z"
        fill="currentColor" />
)

/**
 * Every keyword the board has a sign for, by Scryfall's spelling lowercased.
 *
 * `Record<DrawnKeyword, Glyph>` is doing real work: it is total, so this map
 * must cover `DRAWN_KEYWORDS` exactly — no missing drawing, no orphan one, and
 * no index below that can come back undefined.
 */
const MARKS: Record<DrawnKeyword, Glyph> = {
  flying: Flying,
  'first strike': FirstStrike,
  'double strike': DoubleStrike,
  deathtouch: Deathtouch,
  toxic: Toxic,
  lifelink: Lifelink,
  vigilance: Vigilance,
  trample: Trample,
  haste: Haste,
  menace: Menace,
  reach: Reach,
  hexproof: Hexproof,
  indestructible: Indestructible,
  ward: Ward,
  defender: Defender,
}

/** How many marks a card's corner will carry before it stops being a corner
 *  and starts being a column. Four is about a third of a card's height at the
 *  size these draw, and a creature with five drawn keywords is rare enough
 *  that the tooltip and the held-up card can carry the rest. */
const MOST = 4

/**
 * A card's keywords, stacked in its corner.
 *
 * Upper-left, which is the one corner of a card this board had left: the count
 * is upper-right, the counters lower-left and the loupe lower-right. It rides
 * `.field-card-arm` with the rest of them, so it turns with the card and each
 * mark turns back to stay level.
 *
 * **`granted` changes the plate a mark stands on and nothing else** — see the
 * argument at the top of this file. In particular it does not change the
 * *order*: a mark keeps its place in the card's own list whether it was
 * printed or lent, because a creature gains and loses granted keywords all
 * game and a band that regrouped itself every time something entered play
 * would move the mark a player was reading.
 *
 * **Every mark says what it means, and it stopped being a `title` to do it.**
 * The sentence used to hang off the attribute — which draws on hover, and on
 * nothing else. A phone never saw one; the keyboard never saw one either,
 * because no browser renders `title` on focus; and the marks were not
 * focusable in the first place, so a keyboard could not even reach the thing
 * whose tooltip it was not going to get. That made the one explanation this
 * board offers a newcomer available to a mouse and to nobody else, which is
 * commandment 2 failing quietly (Aaron, 2026-08-28, for the third time in four
 * days). `FieldHint` is the answer and carries the whole argument; each mark
 * is a button now, and hover, focus and a tap all raise the same panel.
 *
 * Aaron asked for the sentence first on a *granted* keyword, where a player
 * has nowhere else to look — lifting the card face out shows a Bronzehide Lion
 * with no vigilance printed on it — and every mark gets one, because a viper is
 * a viper whoever put it there.
 *
 * **The band's own `title` is gone with them.** It carried the scannable list
 * because it was the whole accessible account of a row of `aria-hidden`
 * pictures; the pictures now sit inside buttons that are named, so the list is
 * said by the marks themselves. The card one level up still carries it in
 * prose for anybody reading the card rather than the corner.
 */
export function KeywordMarks(
  { keywords, granted = [], onHint, onHintShut }: {
    keywords: string[]
    granted?: string[]
    /** Passed through to every mark: the card these are standing on uses it to
     *  put its own hover preview down while a panel is up. */
    onHint?: () => void
    onHintShut?: () => void
  },
) {
  const drawn = drawableKeywords(keywords)
  if (drawn.length === 0) return null
  const lent = new Set(granted.map((k) => k.toLowerCase()))
  const shown = drawn.slice(0, MOST)
  const spare = drawn.slice(MOST)
  return (
    <span className="field-keywords">
      {shown.map((key) => {
        const Mark = MARKS[key]
        const said = keywordSaid(key, lent.has(key))
        return (
          <FieldHint key={key} name={said.name} says={said.says}
                     note={said.note} onOpen={onHint} onShut={onHintShut}
                     className={`field-keyword${
                       lent.has(key) ? ' is-granted' : ''}`}>
            <svg viewBox="0 0 20 20" aria-hidden focusable="false">
              <Mark />
            </svg>
          </FieldHint>
        )
      })}
      {/* **The overflow chip explains itself too**, which it could not while
          it was a bare count. Four marks is where a column of chips stops
          being a corner, and the fifth keyword was simply gone — a player
          could see that *something* was not being drawn and had no way at all
          to learn what. It is the same panel as its neighbours, saying the
          words it is standing in for; `keywordWords` writes the lending in,
          so a hidden granted keyword still says it was lent. */}
      {spare.length > 0 && (
        <FieldHint name={`${spare.length} more`}
                   says={keywordWords(spare, granted).join(', ')}
                   onOpen={onHint} onShut={onHintShut}
                   className="field-keyword field-keyword-more tabular">
          +{spare.length}
        </FieldHint>
      )}
    </span>
  )
}
