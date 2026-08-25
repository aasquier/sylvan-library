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

/** A hand of cards, fanned: three outlines pivoting from one wrist. */
export function HandFanGlyph({ size = 14 }: { size?: number }) {
  const card = (
    <rect x="-2.7" y="-9.5" width="5.4" height="8" rx="0.9"
          fill="var(--page)" stroke="currentColor" strokeWidth="1.5" />
  )
  return (
    <svg width={size} height={size} viewBox="0 0 20 20" aria-hidden
         focusable="false" style={{ display: 'block' }}>
      {/* Painted back-to-front so each card overlaps the one before it the
          way a fan actually stacks; the page-colour fill is what keeps the
          outlines from reading as one pretzel where they cross. */}
      <g transform="translate(10 17)">
        <g transform="rotate(-24)">{card}</g>
        <g transform="rotate(24)">{card}</g>
        <g>{card}</g>
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
