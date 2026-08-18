/**
 * One small drawn sign per card category — the 13 canonical names from
 * `decks/model.py`, the UI's own `commander`, and a spark for anything a
 * hand-edited deck file invents (the gate only *warns* on an unknown
 * category, so the fallback is load-bearing, not decorative).
 *
 * Drawn rather than borrowed (commandment 5), on `currentColor` like every
 * mark in the app, and built as bold silhouettes because they live in the
 * 99's section headers at ~15px. Where a verb already has a sign the sign
 * is reused as a *drawing* — recursion is the replay circle, card
 * advantage is the fanned hand — redrawn in this box rather than nested as
 * a second `<svg>`, so each glyph stays one element with one coordinate
 * space.
 */

import type { ReactNode } from 'react'

const stroke = {
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 1.8,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
} as const

const GLYPHS: Record<string, ReactNode> = {
  // Two peaks and a horizon: the land the deck stands on.
  land: (
    <path d="M1.5 16.5 L7.5 6.5 L11 12 L13.5 8.5 L18.5 16.5 Z"
          fill="currentColor" />
  ),
  // A sprout, two leaves off one stem: mana ahead of schedule.
  ramp: (
    <g {...stroke}>
      <path d="M10 17.5 V9" />
      <path d="M10 12 C9.5 9 6.5 8 4.5 8.5 C5 11.5 8 12.5 10 12" />
      <path d="M10 9 C10.5 6 13.5 5 15.5 5.5 C15 8.5 12 9.5 10 9" />
    </g>
  ),
  // Three cards fanned from one wrist: more in hand than you started with.
  'card-advantage': (
    <g transform="translate(10 17.5)">
      <g transform="rotate(-24)">
        <rect x="-2.6" y="-9.8" width="5.2" height="7.8" rx="0.9"
              fill="var(--page)" stroke="currentColor" strokeWidth="1.5" />
      </g>
      <g transform="rotate(24)">
        <rect x="-2.6" y="-9.8" width="5.2" height="7.8" rx="0.9"
              fill="var(--page)" stroke="currentColor" strokeWidth="1.5" />
      </g>
      <rect x="-2.6" y="-9.8" width="5.2" height="7.8" rx="0.9"
            fill="var(--page)" stroke="currentColor" strokeWidth="1.5" />
    </g>
  ),
  // The library under a lens.
  tutor: (
    <g {...stroke}>
      <circle cx="8.5" cy="8.5" r="5" />
      <path d="M12.5 12.5 L17 17" strokeWidth="2.2" />
    </g>
  ),
  // A bolt: the answer, mid-flight.
  interaction: (
    <path d="M11.5 1.5 L4.5 11 H9 L7.5 18.5 L15.5 8 H10.5 Z"
          fill="currentColor" />
  ),
  // The shield.
  protection: (
    <path d="M10 1.8 L16.2 4 V9.8 C16.2 13.8 13.4 16.4 10 18.2
             C6.6 16.4 3.8 13.8 3.8 9.8 V4 Z"
          fill="currentColor" />
  ),
  // A sword, point up: the clock the deck puts on the table.
  threat: (
    <path d="M10 1.5 L12 4 L11.2 11.5 H8.8 L8 4 Z
             M5.5 12.8 H14.5 V14.6 H11.2 V18.5 H8.8 V14.6 H5.5 Z"
          fill="currentColor" />
  ),
  // A gear: the machine that turns every upkeep.
  engine: (
    <g fill="currentColor">
      {Array.from({ length: 6 }, (_, i) => (
        <rect key={i} x="-1.5" y="-9.3" width="3" height="3.6" rx="0.7"
              transform={`translate(10 10) rotate(${i * 60})`} />
      ))}
      <circle cx="10" cy="10" r="5.6" />
      <circle cx="10" cy="10" r="2.3" fill="var(--page)" />
    </g>
  ),
  // The dagger, point down: something is always worth more dead.
  'sac-outlet': (
    <path d="M10 18.5 L8.4 14.8 L9 7 H11 L11.6 14.8 Z
             M6.2 5.2 H13.8 V7 H6.2 Z M9 1.5 H11 V5.2 H9 Z"
          fill="currentColor" />
  ),
  // A crown: what the deck is owed when its plan works.
  payoff: (
    <path d="M3 15.5 V6.5 L7.2 9.8 L10 3.5 L12.8 9.8 L17 6.5 V15.5 Z"
          fill="currentColor" />
  ),
  // The replay circle: back from the graveyard, again.
  recursion: (
    <g>
      <path d="M10 3.2 a6.8 6.8 0 1 1 -6.4 4.5" {...stroke}
            strokeWidth="2" />
      <path d="M1.2 4.6 L7.4 4.2 L3.9 9.4 Z" fill="currentColor" />
    </g>
  ),
  // The trophy.
  'win-con': (
    <g fill="currentColor">
      <path d="M6 2.5 H14 V8 C14 11 12.2 12.8 10 12.8 C7.8 12.8 6 11 6 8 Z" />
      <path d="M8.7 12.8 H11.3 V15 H8.7 Z M5.5 15 H14.5 V17.5 H5.5 Z" />
      <path d="M3.2 4 H6 V6 H4.8 C4.8 7.5 5.4 8.6 6.4 9.2 L5.6 10.6
               C4 9.6 3.2 7.9 3.2 6 Z M16.8 4 H14 V6 H15.2 C15.2 7.5
               14.6 8.6 13.6 9.2 L14.4 10.6 C16 9.6 16.8 7.9 16.8 6 Z" />
    </g>
  ),
  // A wrench: the odd jobs.
  utility: (
    <path d="M16.6 5.8 A4.4 4.4 0 0 1 10.5 10.6 L5.7 15.4 A1.7 1.7 0 0 1
             3.3 13 L8.1 8.2 A4.4 4.4 0 0 1 12.9 2.1 L10.9 4.6 L11.6 7.1
             L14.1 7.8 Z"
          fill="currentColor" />
  ),
  // The helm at the head of the table.
  commander: (
    <g fill="currentColor">
      <path d="M4.5 16.5 V10 C4.5 6 7 3.5 10 3.5 C13 3.5 15.5 6 15.5 10
               V16.5 H12.5 V12.5 H7.5 V16.5 Z" />
      <path d="M9.2 1 H10.8 V3.5 H9.2 Z" />
    </g>
  ),
}

// A four-point spark for a category no glyph knows — visible, never wrong.
const FALLBACK: ReactNode = (
  <path d="M10 1.8 L11.9 8.1 L18.2 10 L11.9 11.9 L10 18.2 L8.1 11.9
           L1.8 10 L8.1 8.1 Z"
        fill="currentColor" />
)

/** One category's sign, inked on `currentColor` at header size. */
export function CategoryGlyph({ category, size = 15 }: {
  category: string
  size?: number
}) {
  return (
    <svg viewBox="0 0 20 20" width={size} height={size} aria-hidden
         focusable="false" style={{ display: 'block' }}>
      {GLYPHS[category] ?? FALLBACK}
    </svg>
  )
}
