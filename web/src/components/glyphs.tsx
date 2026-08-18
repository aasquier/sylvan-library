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
