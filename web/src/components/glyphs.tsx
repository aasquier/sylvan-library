/**
 * Small action glyphs, drawn on `currentColor` like every mark in the app.
 *
 * Two verbs the app keeps using deserved their signs: *run it again* (the
 * Simulator's fresh shuffle, the dossier rewritten, a review re-swept) and
 * *shuffle the cards* (the tarot table). Drawn rather than borrowed from an
 * icon set — commandment 5 — and always beside a label, never instead of
 * one: an icon-only button asks a newcomer to already know the app.
 *
 * Conventions from `GearGlyph`: one small `<svg aria-hidden>`, currentColor
 * ink so the button's own text colour styles the mark, checked at the size
 * buttons actually use (14px).
 */

/** The replay sign: an almost-closed circle, arrowhead at the open end. */
export function ReplayGlyph({ size = 14 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      <path
        d="M10 3.2 a6.8 6.8 0 1 1 -6.4 4.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.2"
        strokeLinecap="round"
      />
      {/* The arrowhead sits at the arc's open end and points along its
          travel, which is what makes the circle read as motion. */}
      <path d="M1.2 4.6 L7.4 4.2 L3.9 9.4 Z" fill="currentColor" />
    </svg>
  )
}

/**
 * A hand of cards, fanned: three outlines pivoting from one wrist.
 *
 * **The fill is a variable now, because this mark left the page.** It was
 * drawn only on the tarot table, where `var(--page)` is exactly the ground
 * behind it; the Coliseum's hand plate is a dark wash over sand at both
 * themes, so a light-theme `--page` painted three white cards onto black.
 * `--glyph-ground` lets a surface say what its own ground is, and falls back
 * to the page for every caller that never had to think about it.
 *
 * **And it opens.** The mark sits on a control that spreads a hand, so it
 * spreads too when the hand reaches for it (commandment 17) — the angle is a
 * prop rather than a hover rule, because the control is already holding that
 * state for the tray and two sources for one gesture is how they drift apart.
 * The base angle is unchanged, so every caller that does not ask for the wider
 * fan is drawn exactly as it always was.
 */
export function HandFanGlyph({ size = 14, open = false }: {
  size?: number
  /** Spread wider: the fan being opened, rather than held. */
  open?: boolean
}) {
  const card = (
    <rect x="-2.7" y="-9.5" width="5.4" height="8" rx="0.9"
          fill="var(--glyph-ground, var(--page))" stroke="currentColor"
          strokeWidth="1.5" />
  )
  // Thirty-six rather than anything wider: the outermost corner of a card at
  // that angle lands 1.3 units inside the viewBox, stroke included, and an SVG
  // root clips to its own box — a fan that opened any further would be a fan
  // with its corners cut off.
  const turn = open ? 36 : 24
  // The rotation stays a presentation attribute rather than moving into CSS,
  // and that is the whole reason this is a prop. An attribute transform pivots
  // about the element's own local origin — the wrist — where a CSS `transform`
  // would pivot about the viewBox corner unless `transform-box` and
  // `transform-origin` were both restated, which is three chances to move a
  // mark that two other screens already draw correctly.
  const at = (deg: number) => (
    <g className="glyph-fan" transform={`rotate(${deg})`}>{card}</g>
  )
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      {/* Painted back-to-front so each card overlaps the one before it the
          way a fan actually stacks; the ground-colour fill is what keeps the
          outlines from reading as one pretzel where they cross. */}
      <g transform="translate(10 17)">
        {at(-turn)}
        {at(turn)}
        {at(0)}
      </g>
    </svg>
  )
}

/** Crossed swords in saltire: the sign the Coliseum's gate wears.
 *
 *  Drawn rather than photographed, and that is a size argument rather than a
 *  taste one. The wheel's clash is a real Met plate because it plays at 380px,
 *  where a museum photograph earns every one of them; this mark plays at 15px
 *  inside a button, where a matted photograph is a smudge and a silhouette is
 *  legible. Commandment 5 forbids clip art, not drawing — `ReplayGlyph` above
 *  settled that, and this file is where the house keeps its own marks.
 *
 *  What makes it read as *swords* rather than as an X is the crossguard: two
 *  bars and two pommels, which is the whole difference between heraldry and a
 *  multiplication sign. One sword is drawn once and mounted twice at ±45°,
 *  because a matched pair is what a pair of gladiators is — and it is mounted
 *  so the blades cross just above the guards, which is where every crossed
 *  pair since the Romans has crossed.
 *
 *  Each sword carries its own class and its own wrapper so a button can move
 *  it — `transform-box: fill-box` makes that wrapper's own centre the pivot,
 *  which is the crossing point, so a rotation opens the pair rather than
 *  swinging it. `.btn-arena` in `index.css` does exactly that on hover;
 *  nothing here animates on its own. */
export function CrossedSwordsGlyph({ size = 17 }: { size?: number }) {
  const sword = (
    <>
      {/* Blade: parallel edges most of the way, then a taper to the point. */}
      <path d="M -1.55 -3.1 L 1.55 -3.1 L 1.55 -10.8 L 0 -14 L -1.55 -10.8 Z"
            fill="currentColor" />
      {/* Crossguard, grip and pommel — the half that makes it a sword rather
          than a stroke, and the half that disappears first. Drawn heavier
          than the proportions of a real blade would give it: at seventeen
          pixels a historically honest guard is one pixel tall and the mark
          collapses into a multiplication sign. The bar is what carries it. */}
      <rect x="-4.5" y="-4.1" width="9" height="2.3" rx="0.9"
            fill="currentColor" />
      <rect x="-1.1" y="-2.4" width="2.2" height="4.3" rx="0.9"
            fill="currentColor" />
      <circle cx="0" cy="2.8" r="1.7" fill="currentColor" />
    </>
  )
  // 4.8 is the sword's own midpoint: it spans -14 (point) to 4.35 (pommel),
  // so sliding it down by half that puts its centre on the crossing and the
  // pair balances — points reaching as far as pommels, which is what keeps a
  // saltire from looking like it is falling over.
  const mounted = (deg: number) => (
    <g transform={`rotate(${deg}) translate(0 4.8)`}>{sword}</g>
  )
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      <g transform="translate(10 10)">
        <g className="glyph-sword glyph-sword-a">{mounted(-45)}</g>
        <g className="glyph-sword glyph-sword-b">{mounted(45)}</g>
      </g>
    </svg>
  )
}

/**
 * An empty throne: the command zone with nobody home.
 *
 * A command zone that is empty because the commander is *out on the field* is
 * a different fact from one that is empty because nothing has happened yet,
 * and a blank slot says neither. A chair says both at once — it is theirs, and
 * there is nobody in it.
 *
 * Hollow rather than filled, because the emptiness is the whole message: a
 * solid silhouette at this size reads as a tombstone, which is the wrong news
 * entirely in a room that is about to start using tombstones for something
 * else. The back is crenellated rather than arched for the crossed swords'
 * reason — at eighteen pixels an arch is two grey pixels and a rectangle, and
 * the notches are the part that survives.
 */
export function ThroneGlyph({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      <g fill="none" stroke="currentColor" strokeWidth="1.4"
         strokeLinejoin="round" strokeLinecap="round">
        {/* The high back and its three merlons. */}
        <path d="M6.6 11.6 V4.4 H8 V2.6 H9.3 V4.4 H10.7 V2.6 H12 V4.4
                 H13.4 V11.6" />
        {/* The seat, and the arms that end on it. */}
        <path d="M4.6 11.6 H15.4 V14.2 H4.6 Z" />
        {/* Planted, so it reads as furniture rather than a card. */}
        <path d="M6 14.2 V17.2" />
        <path d="M14 14.2 V17.2" />
      </g>
    </svg>
  )
}

/**
 * A hunting horn: the companion, called away.
 *
 * The command zone's other empty seat. A companion begins in the zone beside
 * the commander and leaves it in the one way nothing else does — its owner
 * pays {3} and it goes to their *hand*, which is not a departure any other
 * card in that zone can make. So the mark is not a chair with nobody in it,
 * it is the thing you blow to call somebody who then comes.
 *
 * Hollow and stroked to match [ThroneGlyph], and one closed shape because two
 * would be two grey smudges: the horn narrows to a mouthpiece at the bottom
 * left and flares to a bell at the top right, which is the whole silhouette
 * and survives being eighteen pixels tall. The bell's rim is the one extra
 * stroke, because without it the shape reads as a claw.
 */
export function HornGlyph({ size = 18 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      <g fill="none" stroke="currentColor" strokeWidth="1.4"
         strokeLinejoin="round" strokeLinecap="round">
        <path d="M4.6 15.8 C3.9 9.8 8.4 4.6 16 4.2 L15.1 10.4
                 C10.7 10.9 8.1 13.2 7.1 16.6 Z" />
        {/* The rim, so the wide end reads as an opening. */}
        <path d="M16 4.2 L15.1 10.4" />
      </g>
    </svg>
  )
}

/**
 * A crown: the commander, out on the battlefield.
 *
 * The command zone can say a commander is *home*; nothing on the sand said
 * which of forty permanents was the one the whole deck is built around (Aaron,
 * 2026-08-26). This is that card's mark, and it rides the top edge of the
 * painting the way a crown sits on a head.
 *
 * **Filled, where the throne and the horn are hollow, and the difference is
 * the message.** Those two are drawn for a seat with *nobody in it* — an
 * outline is an absence. A crown is an object that is present, on a card that
 * is present, and at fourteen pixels over an arbitrary photograph a hollow one
 * is four grey hairlines. Solid metal is what survives.
 *
 * Two pieces with a gap between them — the points and the circlet — because
 * that gap is the entire difference between a crown and a spiky blob at this
 * size. The three finials are what keep the points from reading as a saw.
 */
export function CrownGlyph({ size = 14 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      {/* The points, rising from a base that the circlet sits under. */}
      <path d="M3.4 12.4 L2.4 4.8 L7 8.8 L10 3.2 L13 8.8 L17.6 4.8
               L16.6 12.4 Z" fill="currentColor" />
      {/* Finials, so each point ends in something rather than stopping. */}
      <circle cx="2.4" cy="4.4" r="1.35" fill="currentColor" />
      <circle cx="10" cy="2.8" r="1.5" fill="currentColor" />
      <circle cx="17.6" cy="4.4" r="1.35" fill="currentColor" />
      {/* The circlet, a hair below the points so the gap reads. */}
      <rect x="3.1" y="13.5" width="13.8" height="3.4" rx="1.2"
            fill="currentColor" />
    </svg>
  )
}

/**
 * A goldfish: the opponent the Simulator has always played against.
 *
 * The deck page offers two rooms a deck can be taken to, and the marks on those
 * two controls are the whole of how a newcomer tells them apart before reading
 * either label (commandment 2). One is [CrossedSwordsGlyph], which is a fight.
 * This is the other one, and the joke is the oldest one in the game: playing a
 * deck with nobody on the other side of the table is *goldfishing*, because for
 * all the resistance you meet you might as well be playing against a goldfish.
 * The Simulator's own nameplate already draws him — a fish idling in a glass
 * bowl on a brass stand, waiting out your ten thousand shuffles — so the button
 * that leads there wears the tenant of the room.
 *
 * **Filled, and that is [CrownGlyph]'s argument rather than a taste.** At
 * fifteen pixels a hollow outline is four grey hairlines; solid is what
 * survives. Three shapes and nothing else: a body, a forked tail and a dorsal
 * fin. The fourth thing a fish needs is an eye, and an eye cannot be *drawn*
 * here — a dot of some assumed background colour is a hole punched in the wrong
 * paint the moment the plate under it changes. So the eye is a real hole:
 * body and pupil are one path under `evenodd`, and what shows through it is
 * whatever the button is actually made of.
 *
 * The tail carries its own class so a button can move it — `.glyph-fin`, the
 * same mechanism `.glyph-sword` uses, with `fill-box` putting the pivot where
 * the tail meets the body so it *swishes* rather than swinging bodily.
 * `.btn-goldfish` in `index.css` does that on hover; nothing here animates on
 * its own.
 */
export function GoldfishGlyph({ size = 15 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      {/* The tail, forked. Drawn first so the body's edge covers the join —
          a fish is one animal, not a triangle parked next to an oval. */}
      <path className="glyph-fin"
            d="M 8.6 10 L 2.2 5.6 L 3.9 10 L 2.2 14.4 Z" fill="currentColor" />
      {/* The dorsal fin, its base buried in the body for the same reason. */}
      <path d="M 8.4 6.9 L 11.6 3.5 L 13.4 7.4 Z" fill="currentColor" />
      {/* Body and eye as one path: two arcs make the ellipse, two more make
          the pupil, and `evenodd` turns the inner ring into a hole. */}
      <path fillRule="evenodd" fill="currentColor"
            d="M 5.6 10 A 6 4.3 0 0 1 17.6 10 A 6 4.3 0 0 1 5.6 10 Z
               M 15.5 8.7 A 1.05 1.05 0 1 1 13.4 8.7 A 1.05 1.05 0 1 1 15.5 8.7 Z" />
    </svg>
  )
}

/**
 * A strongbox: what a commander costs to call back.
 *
 * Commander tax was a bare red chip of digits, and a number alone is a fact
 * with no thing attached to it — Aaron, 2026-08-27: *"it would be nice if it
 * was a stylized treasure chest or back of coins or something. Something for
 * indicative of a tax."* A Roman *arca* is the object that answer wants: a
 * bound chest with a heavy lock, standing on the floor where the money was
 * kept, which is furniture this room already has a floor for.
 *
 * **Drawn rather than photographed, and that is a measurement rather than a
 * preference.** This mark renders at fifteen pixels in a seat that is
 * seventy-two wide — measured on a live board, not guessed — and everything
 * this project has learned about that size band says a photograph loses
 * there: the shield next door had to be re-cut twice before it stopped
 * reading as an emblem, and `CrownGlyph` above says it plainly, that at
 * fourteen pixels over an arbitrary painting solid metal is what survives.
 * The two glyphs it stands beside on this very tile — the throne and the
 * horn — are drawn too, so a photograph here would also be a second visual
 * language inside one seventy-two pixel tile.
 *
 * **Filled, like the crown and unlike the throne**, and for the crown's
 * reason: the hollow ones are drawn for a seat with *nobody in it*, where an
 * outline is an absence. A price is not an absence. It is a thing you owe.
 *
 * **Everything legible here is a gap.** Inside one solid shape at this size
 * no detail can be drawn *on* the box, so the seam between lid and body is a
 * band of nothing, and the hasp is the one place that band is interrupted —
 * which is the whole difference between a chest and a loaf. The feet are what
 * keep it standing on a floor rather than floating over one, the same trick
 * [ThroneGlyph] uses to stop reading as a card.
 */
export function StrongboxGlyph({ size = 15 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      {/* The barrel lid, a true half-ellipse rather than a rounded corner. A
          flat top is a crate; the dome is the whole silhouette, and it is what
          a person recognises before any detail arrives.

          **And it overhangs.** The lid is wider than the body by a unit on
          each side, which is how a chest is actually built and — at this size
          the only reason the two read as two parts rather than as one lump. */}
      <path d="M1.6 9 V8.2 C1.6 5.4 4.6 4 10 4 C15.4 4 18.4 5.4 18.4 8.2 V9 Z"
            fill="currentColor" />
      {/* The body, 2 units under the lid: a pixel and a half at the size this
          renders, which is the narrowest a seam can be and still survive being
          anti-aliased away.

          **Taller than the lid, which is the proportion that settles what this
          is.** A dome deeper than its box is a loaf; a box deeper than its
          dome is a chest. Two attempts had it the other way round and both
          read as food. */}
      <path d="M2.8 11 H17.2 V16.4 H2.8 Z" fill="currentColor" />
      {/* Feet. Two, out at the corners so the base reads as wide as the lid —
          they are what keep the box standing on the floor of the seat rather
          than floating over it, which is [ThroneGlyph]'s trick. */}
      <rect x="3.2" y="16.4" width="3.2" height="1.6" fill="currentColor" />
      <rect x="13.6" y="16.4" width="3.2" height="1.6" fill="currentColor" />
      {/* The hasp, bridging lid and body across the seam. It is invisible
          where it lies on metal and visible where it crosses the gap, so what
          the eye gets is a seam broken in the middle — which is exactly how a
          real one looks, and the only detail a solid shape this size can
          hold. */}
      <rect x="8.4" y="7.6" width="3.2" height="5.4" rx="0.5"
            fill="currentColor" />
    </svg>
  )
}
