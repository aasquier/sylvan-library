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
 *
 * **Which keywords get a sign is `lib/keywords.ts`'s list**, and the map at the
 * foot of this file is typed against it — so a word on that list with no
 * drawing here fails the typecheck, and a drawing here for a word nobody listed
 * fails too. A set of names and a set of pictures that are supposed to be the
 * same set is exactly the pair that quietly stops being the same set.
 */

import type { ReactElement } from 'react'

import { type DrawnKeyword, drawableKeywords } from '../lib/keywords'

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

/** Deathtouch: a snake, with the drop coming off its fang.
 *
 *  **A fang alone was not enough** (Aaron, 2026-08-25) — a lone tooth with a
 *  drop under it is a dentist, and the keyword is not about a tooth, it is
 *  about the thing the tooth belongs to. A serpent is what every player already
 *  reads as *one touch and you are gone*.
 *
 *  Laid out so nothing has to share room with anything: the head takes the top
 *  left, the body coils away to the right, and the whole left edge below the
 *  snout is left clear for the fang and the drop — which are the two shapes
 *  that carry the meaning and the two that must not be crowded. */
const Deathtouch: Glyph = () => (
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
 */
export function KeywordMarks({ keywords }: { keywords: string[] }) {
  const drawn = drawableKeywords(keywords)
  if (drawn.length === 0) return null
  const shown = drawn.slice(0, MOST)
  const rest = drawn.length - shown.length
  return (
    <span className="field-keywords" title={drawn.join(', ')}>
      {shown.map((key) => {
        const Mark = MARKS[key]
        return (
          <span key={key} className="field-keyword" title={key}>
            <svg viewBox="0 0 20 20" aria-hidden focusable="false">
              <Mark />
            </svg>
          </span>
        )
      })}
      {rest > 0 && (
        <span className="field-keyword field-keyword-more tabular">
          +{rest}
        </span>
      )}
    </span>
  )
}
