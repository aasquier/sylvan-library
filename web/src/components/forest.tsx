/**
 * The forest layer: the greenery the app is named after, drawn rather than
 * photographed.
 *
 * Everything here is inline SVG and CSS for the same reason `CardBack` and
 * `managlyphs.ts` are — no asset, no licence question, no bytes on the wire —
 * and all of it is decoration in the strict sense: `aria-hidden`, pointer
 * events off, and nothing a screen reader or a keyboard ever meets. The two
 * themes get different weather on purpose: **fireflies at night, drifting
 * leaves by day.** That split lives in `index.css` (`.firefly` and
 * `.leaf-fall` display-gate on the theme), and `prefers-reduced-motion`
 * removes the whole layer rather than freezing it — a firefly holding
 * perfectly still is not a firefly, it is a spot on the screen.
 *
 * The positions and timings are constant tables, not `Math.random()`: the
 * same forest on every render, nothing for a test to flake on, and the
 * numbers were picked so no two cycles share a period — the sky does not
 * visibly loop.
 */

import { useId } from 'react'

/* ------------------------------------------------------------ the ambience */

interface Firefly {
  left: string
  top: string
  dur: number
  delay: number
  /** Three waypoints of the wander, in px. */
  path: [number, number, number, number, number, number]
}

const FIREFLIES: Firefly[] = [
  { left: '6%', top: '70%', dur: 27, delay: 0, path: [50, -70, -20, -140, 35, -50] },
  { left: '16%', top: '38%', dur: 33, delay: 4, path: [-40, 60, 30, 110, -20, 40] },
  { left: '28%', top: '82%', dur: 24, delay: 9, path: [60, -50, 90, -110, 20, -60] },
  { left: '55%', top: '75%', dur: 31, delay: 2, path: [-55, -40, -90, -100, -30, -30] },
  { left: '68%', top: '30%', dur: 26, delay: 12, path: [45, 70, -25, 130, 15, 50] },
  { left: '81%', top: '64%', dur: 35, delay: 6, path: [-60, -45, -20, -120, -70, -35] },
  { left: '90%', top: '44%', dur: 23, delay: 15, path: [-45, 60, -80, 20, -30, 80] },
  { left: '43%', top: '55%', dur: 29, delay: 18, path: [35, -65, 70, -20, 25, -90] },
]

interface Leaf {
  left: string
  dur: number
  delay: number
  /** How far the fall drifts sideways, in px; sign is the wind's direction. */
  sway: number
  size: number
  opacity: number
}

const LEAVES: Leaf[] = [
  { left: '12%', dur: 41, delay: 0, sway: 90, size: 13, opacity: 0.4 },
  { left: '34%', dur: 53, delay: 17, sway: -70, size: 10, opacity: 0.3 },
  { left: '61%', dur: 47, delay: 8, sway: 110, size: 15, opacity: 0.35 },
  { left: '83%', dur: 59, delay: 29, sway: -90, size: 11, opacity: 0.3 },
  { left: '48%', dur: 37, delay: 40, sway: 60, size: 9, opacity: 0.25 },
]

/** One drawn leaf — a willow-ish blade with a midrib, on `currentColor`. */
function LeafShape({ size }: { size: number }) {
  return (
    <svg width={size} height={size * 1.5} viewBox="0 0 12 18" fill="none">
      <path d="M6 0 C 10.5 5 10.5 12 6 18 C 1.5 12 1.5 5 6 0 Z"
            fill="currentColor" />
      <path d="M6 2 L 6 16" stroke="currentColor" strokeWidth="0.7"
            opacity="0.6" />
    </svg>
  )
}

/**
 * The fixed full-viewport layer. Mounted once in the shell, behind the
 * content (`<main>` carries `relative z-10`, so anything with painted pixels
 * covers this — a firefly passing behind a card is *behind* it, which is
 * what makes the room read as having depth rather than an overlay).
 */
export function ForestAmbience() {
  return (
    <div className="forest-ambience" aria-hidden="true">
      {FIREFLIES.map((f, i) => (
        <span key={i} className="firefly" style={{
          left: f.left,
          top: f.top,
          '--dur': `${f.dur}s`,
          '--delay': `${f.delay}s`,
          '--fx1': `${f.path[0]}px`,
          '--fy1': `${f.path[1]}px`,
          '--fx2': `${f.path[2]}px`,
          '--fy2': `${f.path[3]}px`,
          '--fx3': `${f.path[4]}px`,
          '--fy3': `${f.path[5]}px`,
        } as React.CSSProperties} />
      ))}
      {LEAVES.map((l, i) => (
        <span key={i} className="leaf-fall" style={{
          left: l.left,
          '--dur': `${l.dur}s`,
          '--delay': `${l.delay}s`,
          '--sway': `${l.sway}px`,
          '--leaf-op': l.opacity,
        } as React.CSSProperties}>
          <LeafShape size={l.size} />
        </span>
      ))}
    </div>
  )
}

/* -------------------------------------------------------------- the canopy */

/**
 * The vine that drapes from the header — one tile of stem, leaves and
 * berries, repeated as an SVG pattern across whatever width the window has.
 * It hangs *below* the sticky bar (absolute, `top: 100%`) so the chrome
 * stays legible and the greenery reads as growing off it rather than
 * through it. The sway is on the `<svg>`, one element, transform-only.
 */
export function HeaderCanopy() {
  const id = useId().replace(/:/g, '')
  return (
    <div className="header-canopy" aria-hidden="true">
      <svg className="h-full w-full">
        <defs>
          <pattern id={`vine-${id}`} width="220" height="26"
                   patternUnits="userSpaceOnUse">
            <path d="M0 4 C 30 0, 52 12, 82 7 S 140 1, 168 9 S 206 12, 220 4"
                  fill="none" stroke="currentColor" strokeWidth="1.4"
                  opacity="0.75" />
            {/* Leaves alternate sides of the stem, each its own size and
                lean, because a vine that repeats too obviously is a wallpaper
                border rather than a plant. */}
            <path d="M26 6 q 6 1 8 9 q -8 -1 -8 -9" fill="currentColor"
                  opacity="0.8" />
            <path d="M58 9 q -2 -8 5 -12 q 3 8 -5 12" fill="currentColor"
                  opacity="0.65" />
            <path d="M104 6 q 7 2 8 11 q -9 -2 -8 -11" fill="currentColor"
                  opacity="0.85" />
            <path d="M136 5 q -1 -7 6 -10 q 2 7 -6 10" fill="currentColor"
                  opacity="0.6" />
            <path d="M186 9 q 6 1 7 10 q -8 -1 -7 -10" fill="currentColor"
                  opacity="0.75" />
            <circle cx="76" cy="8" r="1.6" fill="currentColor" opacity="0.7" />
            <circle cx="160" cy="7" r="1.3" fill="currentColor" opacity="0.6" />
            <circle cx="212" cy="6" r="1.5" fill="currentColor" opacity="0.65" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill={`url(#vine-${id})`} />
      </svg>
    </div>
  )
}
