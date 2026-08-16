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
 * visibly loop. The one source of variety that is not a table — the leaves
 * the canopy sheds when the header is clicked — derives its jitter
 * arithmetically from a counter, for the same reason.
 *
 * Punch list 2026-08-15, items 4 and 10, both land here: the vine grew
 * detail, a twist and an interaction; the weather brightened and stopped
 * hiding by theme; pages fall among the leaves, because this forest grew
 * through a library; and `SceneBackdrop` is the room itself — each page's
 * own painting washed across the whole screen, with sunlight through a
 * window that was never quite cleaned.
 */

import { useEffect, useId, useRef, useState } from 'react'

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
  { left: '12%', dur: 41, delay: 0, sway: 90, size: 13, opacity: 0.55 },
  { left: '34%', dur: 53, delay: 17, sway: -70, size: 10, opacity: 0.45 },
  { left: '61%', dur: 47, delay: 8, sway: 110, size: 15, opacity: 0.5 },
  { left: '83%', dur: 59, delay: 29, sway: -90, size: 11, opacity: 0.45 },
  { left: '48%', dur: 37, delay: 40, sway: 60, size: 9, opacity: 0.4 },
  { left: '22%', dur: 44, delay: 33, sway: 75, size: 12, opacity: 0.45 },
  { left: '72%', dur: 57, delay: 21, sway: -60, size: 10, opacity: 0.4 },
]

/** Loose pages, fallen out of some book upstairs. Same contract as the
 *  leaves — a constant table, periods that never coincide — but far fewer
 *  and slower: paper is lighter than it looks and rarer than leaves. */
const PAGES: Leaf[] = [
  { left: '27%', dur: 61, delay: 6, sway: 130, size: 15, opacity: 0.4 },
  { left: '56%', dur: 73, delay: 31, sway: -110, size: 12, opacity: 0.34 },
  { left: '79%', dur: 67, delay: 52, sway: 95, size: 14, opacity: 0.38 },
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

/** One loose page — parchment, a turned corner, and lines too small to read.
 *  The text is rules text from no card at all: five strokes of varying
 *  length, which is what a page looks like from across a room. */
function PageShape({ size }: { size: number }) {
  return (
    <svg width={size} height={size * 1.3} viewBox="0 0 14 18" fill="none">
      <path d="M1 0 H 10 L 13 3 V 18 H 1 Z" fill="#f0e4c2" />
      <path d="M10 0 L 10 3 L 13 3 Z" fill="#d9c89a" />
      {[5, 7.5, 10, 12.5, 15].map((y, i) => (
        <path key={y} d={`M3 ${y} H ${i === 4 ? 8 : 11}`} stroke="#8a7a55"
              strokeWidth="0.8" opacity="0.55" />
      ))}
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
      {/* Pages fall in both themes — paper catches whatever light there is —
          and tumble on a second axis the leaves do not use, because paper
          planes and flutters where a leaf glides. */}
      {PAGES.map((p, i) => (
        <span key={i} className="page-fall" style={{
          left: p.left,
          '--dur': `${p.dur}s`,
          '--delay': `${p.delay}s`,
          '--sway': `${p.sway}px`,
          '--leaf-op': p.opacity,
        } as React.CSSProperties}>
          <PageShape size={p.size} />
        </span>
      ))}
    </div>
  )
}

/* -------------------------------------------------------------- the canopy */

/** A leaf the canopy shed. Everything the animation needs, precomputed —
 *  the jitter is arithmetic on the id, not `Math.random()`, so a test can
 *  predict a burst exactly. */
interface ShedLeaf {
  id: number
  /** px from the canopy's left edge. */
  x: number
  drift: number
  rot: number
  size: number
  delay: number
}

/**
 * The vine that drapes from the header — one tile of stems, leaves, curling
 * tendrils and berries, repeated as an SVG pattern across whatever width the
 * window has. It hangs *below* the sticky bar (absolute, `top: 100%`) so the
 * chrome stays legible and the greenery reads as growing off it rather than
 * through it.
 *
 * Punch list 2026-08-15 item 4 rebuilt it. Three moves, none of them an
 * asset: **two stems that cross** rather than one polite wave, so the vine
 * reads as twisted growth; **an uneven underside** — tendrils of different
 * lengths spiralling off the stem, leaf pairs at their tips — so the bottom
 * edge is a plant's and not a wallpaper border's; and **berries with a gold
 * glint**, the same #c9a227 every drawn mark in this app commits to. The
 * whole canopy grows in on load (scaleY from the stem line, in CSS) and
 * sways for good; both stop under `prefers-reduced-motion`.
 *
 * And it answers to touch: **click the header and the vine sheds leaves**
 * from where you clicked. The canopy itself stays `pointer-events: none` —
 * it listens on its parent, the header, so the nav keeps working and the
 * leaves are a side effect of whatever you were doing anyway.
 */
export function HeaderCanopy() {
  const id = useId().replace(/:/g, '')
  const rootRef = useRef<HTMLDivElement>(null)
  const [shed, setShed] = useState<ShedLeaf[]>([])
  const counter = useRef(0)

  useEffect(() => {
    const root = rootRef.current
    const host = root?.parentElement
    if (!root || !host) return
    const onClick = (e: MouseEvent) => {
      if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
      const left = root.getBoundingClientRect().left
      const burst = Array.from({ length: 3 }, (_, i) => {
        const n = counter.current++
        return {
          id: n,
          x: e.clientX - left + (i - 1) * 16 + ((n * 7) % 11) - 5,
          drift: ((n * 13) % 49) - 24,
          rot: 140 + ((n * 31) % 160),
          size: 9 + ((n * 5) % 3) * 3,
          delay: i * 110,
        }
      })
      setShed((s) => [...s, ...burst])
    }
    host.addEventListener('click', onClick)
    return () => host.removeEventListener('click', onClick)
  }, [])

  return (
    <div ref={rootRef} className="header-canopy" aria-hidden="true">
      <svg className="h-full w-full">
        <defs>
          <pattern id={`vine-${id}`} width="260" height="44"
                   patternUnits="userSpaceOnUse">
            {/* Two stems, crossing twice: the twist. The second is thinner
                and dimmer, a stem seen behind a stem. */}
            <path d="M0 6 C 34 -2, 62 16, 104 9 S 178 0, 216 10 S 248 12, 260 5"
                  fill="none" stroke="currentColor" strokeWidth="1.6"
                  opacity="0.8" />
            <path d="M0 11 C 40 19, 70 1, 112 13 S 190 19, 226 6 S 250 3, 260 10"
                  fill="none" stroke="currentColor" strokeWidth="1"
                  opacity="0.45" />
            {/* Leaves alternate sides of the stems, each its own size and
                lean — a vine that repeats too obviously is a wallpaper
                border rather than a plant. Every leaf carries its vein. */}
            <path d="M22 7 q 7 1 9 11 q -9 -1 -9 -11" fill="currentColor"
                  opacity="0.85" />
            <path d="M23.5 8.5 q 4 3 6 8" stroke="currentColor"
                  strokeWidth="0.6" fill="none" opacity="0.5" />
            <path d="M52 10 q -2 -9 6 -13 q 3 9 -6 13" fill="currentColor"
                  opacity="0.65" />
            <path d="M53.5 8 q 2 -4 3.5 -8" stroke="currentColor"
                  strokeWidth="0.6" fill="none" opacity="0.45" />
            <path d="M98 8 q 8 2 9 13 q -10 -2 -9 -13" fill="currentColor"
                  opacity="0.9" />
            <path d="M99.5 10 q 4 4 6 9" stroke="currentColor"
                  strokeWidth="0.6" fill="none" opacity="0.5" />
            <path d="M132 7 q -1 -8 7 -11 q 2 8 -7 11" fill="currentColor"
                  opacity="0.6" />
            <path d="M168 12 q 6 1 8 11 q -9 -1 -8 -11" fill="currentColor"
                  opacity="0.75" />
            <path d="M169.5 13.5 q 3.5 3.5 5.5 8" stroke="currentColor"
                  strokeWidth="0.6" fill="none" opacity="0.5" />
            <path d="M206 8 q -2 -8 5 -12 q 3 8 -5 12" fill="currentColor"
                  opacity="0.7" />
            <path d="M238 9 q 7 2 8 12 q -9 -2 -8 -12" fill="currentColor"
                  opacity="0.8" />
            {/* Tendrils: spirals of unequal length hanging off the stem, a
                leaf pair at each tip. The uneven underside is the point. */}
            <path d="M64 10 c 1 8 -2 14 2 19 c 4 5 10 2 8 -3 c -1.5 -4 -7 -2 -5 2"
                  fill="none" stroke="currentColor" strokeWidth="1"
                  opacity="0.6" />
            <path d="M70 27 q 5 0 7 6 q -7 0 -7 -6" fill="currentColor"
                  opacity="0.55" />
            <path d="M148 11 c 0 10 -3 18 2 25 c 4 6 11 2 9 -4 c -2 -5 -8 -2 -6 2"
                  fill="none" stroke="currentColor" strokeWidth="1.1"
                  opacity="0.7" />
            <path d="M152 33 q -6 -1 -8 6 q 7 1 8 -6" fill="currentColor"
                  opacity="0.6" />
            <path d="M154 34 q 5 1 6 7 q -7 -1 -6 -7" fill="currentColor"
                  opacity="0.5" />
            <path d="M228 9 c 1 6 -1 11 2 15 c 3 4 8 1 6 -2 c -1.5 -3 -5 -1 -4 1.5"
                  fill="none" stroke="currentColor" strokeWidth="0.9"
                  opacity="0.55" />
            {/* Berries in clusters, each with the gold glint the drawn marks
                share. */}
            <circle cx="84" cy="10" r="1.9" fill="currentColor" opacity="0.8" />
            <circle cx="87.5" cy="12.5" r="1.5" fill="currentColor" opacity="0.65" />
            <circle cx="85" cy="14" r="1.2" fill="currentColor" opacity="0.5" />
            <circle cx="83.4" cy="9.4" r="0.6" fill="#c9a227" opacity="0.9" />
            <circle cx="190" cy="9" r="1.7" fill="currentColor" opacity="0.75" />
            <circle cx="193" cy="11" r="1.3" fill="currentColor" opacity="0.55" />
            <circle cx="189.4" cy="8.4" r="0.55" fill="#c9a227" opacity="0.85" />
            <circle cx="254" cy="8" r="1.5" fill="currentColor" opacity="0.65" />
            <circle cx="253.5" cy="7.5" r="0.5" fill="#c9a227" opacity="0.8" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill={`url(#vine-${id})`} />
      </svg>
      {shed.map((leaf) => (
        <span key={leaf.id} className="leaf-shed"
              onAnimationEnd={() =>
                setShed((s) => s.filter((l) => l.id !== leaf.id))}
              style={{
                left: `${leaf.x}px`,
                animationDelay: `${leaf.delay}ms`,
                '--drift': `${leaf.drift}px`,
                '--rot': `${leaf.rot}deg`,
              } as React.CSSProperties}>
          <LeafShape size={leaf.size} />
        </span>
      ))}
    </div>
  )
}

/**
 * The room behind a page (punch list 2026-08-15 item 10).
 *
 * Every masthead page already names one painting; this washes that same
 * painting across the whole viewport — blurred, dim, and fading out toward
 * the reading column — so the page stops being content floating on flat
 * black and starts being a room with the light off. Over it, one shaft of
 * sunlight from the upper corner, flickering slowly the way light does when
 * it has come through leaves and old glass, with dust hanging in it. The
 * art is the page's own masthead crop, so this adds no new source and no
 * new credit; the beam and the dust are CSS.
 *
 * Decorative in the strict sense: `aria-hidden`, no pointer events, and
 * behind everything with painted pixels. `prefers-reduced-motion` stills
 * the flicker and the dust but keeps the room.
 */
export function SceneBackdrop({ art }: { art: string }) {
  return (
    <div className="scene-backdrop" aria-hidden="true">
      <img src={art} alt="" className="scene-backdrop-art" />
      <div className="scene-sunbeam" />
      <div className="scene-sunbeam scene-sunbeam-b" />
      {[
        { left: '78%', top: '12%', dur: 19, delay: 0 },
        { left: '84%', top: '24%', dur: 23, delay: 7 },
        { left: '72%', top: '30%', dur: 29, delay: 13 },
        { left: '88%', top: '9%', dur: 17, delay: 4 },
      ].map((m, i) => (
        <span key={i} className="scene-mote" style={{
          left: m.left,
          top: m.top,
          '--dur': `${m.dur}s`,
          '--delay': `${m.delay}s`,
        } as React.CSSProperties} />
      ))}
    </div>
  )
}

/* ------------------------------------------------------------ the wordmark */

/**
 * The library's mark: a tree grown out of an open book.
 *
 * Drawn for the same reasons everything else in this file is — and because
 * the header wore a 🌳 emoji, which renders as whatever the platform's emoji
 * font thinks a tree is and reads as a placeholder on every one of them.
 * This is the identity the tab, the header and the sign-in screen share.
 *
 * It commits to its own palette in both themes, like the tarot card back:
 * a mark that changed colour with the theme would stop reading as a mark.
 */
export function LibraryMark({ size = 24 }: { size?: number }) {
  return (
    <svg viewBox="0 0 40 40" width={size} height={size} aria-hidden="true">
      {/* The canopy: overlapping crowns, lit from the upper left. */}
      <circle cx="20" cy="11.5" r="8" fill="#2f6b3f" />
      <circle cx="12" cy="16" r="6.5" fill="#285c36" />
      <circle cx="28" cy="16" r="6.5" fill="#285c36" />
      <circle cx="20" cy="17" r="7.5" fill="#357347" />
      <circle cx="16.5" cy="9.5" r="3.2" fill="#5c9c6c" opacity="0.9" />
      {/* The trunk, flaring where it meets the pages. */}
      <path d="M18.8 21 C18.8 24.5 17.8 27 16.2 28.8 L20 28.2 L23.8 28.8
               C22.2 27 21.2 24.5 21.2 21 Z" fill="#6e4e28" />
      {/* The open book: amber cover, parchment pages, a seam at the spine. */}
      <path d="M5.5 28.5 C10.5 27 15.5 27.5 20 29.8 C24.5 27.5 29.5 27
               34.5 28.5 L34.5 33.5 C29.5 32 24.5 32.5 20 34.8 C15.5 32.5
               10.5 32 5.5 33.5 Z" fill="#c9a227" />
      <path d="M7.5 29.3 C11.5 28.2 16 28.6 20 30.6 C24 28.6 28.5 28.2
               32.5 29.3 L32.5 32 C28.5 31 24 31.4 20 33.2 C16 31.4 11.5 31
               7.5 32 Z" fill="#f0e4c2" />
      <path d="M20 30.6 L20 33.2" stroke="#b39b62" strokeWidth="0.9" />
    </svg>
  )
}
