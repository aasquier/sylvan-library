/**
 * The colour wheel, drawn — and drawn because the shape *is* the lesson.
 *
 * Five vertices in WUBRG order round a circle. Every other fact about how
 * Magic's colours relate to each other falls out of that one arrangement:
 *
 * - the **perimeter** joins colours that sit next to each other, and those
 *   five edges are the five allied guilds;
 * - the **chords** join colours two apart, and those five lines are the five
 *   enemy guilds — and they are what draws the star, which is why this shape
 *   is a pentagram rather than a pentagon.
 *
 * Ten lines, ten guilds, no leftovers. That is checkable rather than asserted:
 * `tests/test_colors.py` runs the same adjacency rule over `colors.py` and
 * fails if the perimeter and the chords are not exactly the guild tier.
 *
 * **Nothing here is a second copy of the taxonomy.** Every name, tagline and
 * blurb is looked up in the `Combination[]` the page already fetched from
 * `/api/colors`, so `colors.py` stays the one authority on what a slot is
 * called — the rule CLAUDE.md states for any new copy of that table is
 * satisfied by there not being one. What *is* written down here is the
 * geometry, and geometry is not data: the classification is computed from
 * WUBRG adjacency, so the diagram cannot disagree with itself about which
 * guilds are allied.
 *
 * Drawn in SVG rather than shipped as an image, per the branch's own rule: no
 * new binary assets and no licensing question. A diagram is the one thing that
 * loses nothing by being drawn.
 */

import { useState } from 'react'
import type { Combination } from '../lib/api'
import { COLOR_NAMES, COLOR_VAR, WUBRG } from '../lib/mtg'
import { GLYPH_PATH } from '../lib/managlyphs'

/* ---------------------------------------------------------------- geometry */

const CX = 200
const CY = 200
/** Vertex circle centres. Leaves room for r=26 discs inside a 400 box. */
const R = 140
const VERTEX_R = 26

/**
 * Vertex `i` of a five-pointed wheel, clockwise from the top.
 *
 * -90° puts White at twelve o'clock and +72° steps round the five, so reading
 * the diagram clockwise from the top spells WUBRG. Anti-clockwise would be a
 * mirror image that still joins the right pairs — the allied and enemy sets
 * are symmetric — but it would spell the order backwards, and the order is the
 * thing being taught.
 *
 * Takes its geometry as arguments so the full diagram and the tier badges are
 * literally the same wheel at two sizes rather than two drawings that happen
 * to agree.
 */
function vertexOn(i: number, r: number, cx: number, cy: number): { x: number; y: number } {
  const radians = ((-90 + i * 72) * Math.PI) / 180
  return { x: cx + r * Math.cos(radians), y: cy + r * Math.sin(radians) }
}

const vertex = (i: number) => vertexOn(i, R, CX, CY)

/** Adjacent on the wheel is allied; two apart is enemy. */
function isAllied(a: number, b: number): boolean {
  const gap = Math.abs(a - b)
  return gap === 1 || gap === 4
}

/** Canonical WUBRG key for two colours, matching `colors.key_for`. */
function pairKey(a: string, b: string): string {
  return WUBRG.filter((c) => c === a || c === b).join('')
}

interface Edge {
  key: string
  from: number
  to: number
  allied: boolean
}

/**
 * The ten lines.
 *
 * Adjacent in WUBRG order is allied; two apart is enemy. Both directions past
 * that are the same pair seen from the other end — three apart is two apart
 * read backwards — so stepping `i+1` and `i+2` over five vertices enumerates
 * each guild exactly once.
 */
const EDGES: Edge[] = Array.from({ length: 5 }, (_, i) => i).flatMap((i) => [
  { key: pairKey(WUBRG[i], WUBRG[(i + 1) % 5]), from: i, to: (i + 1) % 5, allied: true },
  { key: pairKey(WUBRG[i], WUBRG[(i + 2) % 5]), from: i, to: (i + 2) % 5, allied: false },
])

/* ---------------------------------------------------------- tier badges */

/**
 * One representative member of each tier, as indices into WUBRG.
 *
 * Not a copy of the taxonomy — no names, no membership, nothing a caller could
 * read a fact off. It is a *shape*: which vertices a tier of this kind lights
 * up, chosen so the badge draws the definition `TIER_BLURBS` states in words.
 *
 * The two three-colour tiers are the pair worth getting right, because they
 * are the ones people confuse and the badge can settle it at a glance:
 *
 * - a **shard** is a colour and both its neighbours, so G-W-U — an unbroken
 *   arc, and its three lines come out two solid and one dashed;
 * - a **wedge** is a colour and both its opposites, so W-B-R — a span across
 *   the middle, two dashed and one solid.
 *
 * Same three dots, same triangle count, opposite texture. That is the whole
 * difference between the two tiers, drawn.
 */
const TIER_SHAPE: Record<string, number[]> = {
  colorless: [],
  mono: [0],
  guild: [0, 1],
  shard: [4, 0, 1],
  wedge: [0, 2, 3],
  quad: [0, 1, 2, 3],
  five: [0, 1, 2, 3, 4],
}

/**
 * The wheel at badge size, with one tier's shape lit.
 *
 * Unlit vertices stay on as faint dots rather than disappearing, so all seven
 * badges are the same figure with different parts picked out — which is what
 * makes them comparable at a glance. A badge that dropped the unlit dots would
 * be seven unrelated shapes.
 */
export function TierGlyph({ tier, size = 26 }: { tier: string; size?: number }) {
  const lit = TIER_SHAPE[tier] ?? []
  const box = 44
  const r = 15
  const c = box / 2
  const pairs = lit.flatMap((a, i) => lit.slice(i + 1).map((b) => [a, b] as const))

  return (
    <svg width={size} height={size} viewBox={`0 0 ${box} ${box}`}
         aria-hidden focusable="false" className="shrink-0">
      {pairs.map(([a, b]) => {
        const p = vertexOn(a, r, c, c)
        const q = vertexOn(b, r, c, c)
        return (
          <line key={`${a}-${b}`} x1={p.x} y1={p.y} x2={q.x} y2={q.y}
                stroke="currentColor" strokeWidth={1.6} strokeLinecap="round"
                strokeDasharray={isAllied(a, b) ? undefined : '2.5 2.5'}
                opacity={0.55} />
        )
      })}
      {WUBRG.map((code, i) => {
        const p = vertexOn(i, r, c, c)
        const on = lit.includes(i)
        return (
          <circle key={code} cx={p.x} cy={p.y} r={on ? 5 : 2}
                  fill={on ? COLOR_VAR[code] : 'currentColor'}
                  opacity={on ? 1 : 0.3} />
        )
      })}
      {/* Colourless lights nothing on the wheel, because it is not a point on
          it. The centre dot says "an identity, but none of these". */}
      {tier === 'colorless' && (
        <circle cx={c} cy={c} r={5.5} fill="var(--mtg-c)" />
      )}
    </svg>
  )
}

/* --------------------------------------------------------------- the shape */

export interface PentagramProps {
  /** The full taxonomy from `/api/colors`. The only source of names here. */
  combinations: Combination[]
  /** A vertex or a line was chosen. */
  onPick: (combo: Combination) => void
  /** Key of the combination to draw as already selected, if any. */
  selected?: string | null
}

export function ColorPentagram({ combinations, onPick, selected }: PentagramProps) {
  // One hover/focus channel for both, so tabbing through the diagram reads the
  // same caption a mouse would surface. `null` means "nothing under the
  // pointer", which is a different caption from any combination.
  const [active, setActive] = useState<string | null>(null)

  const byKey = new Map(combinations.map((c) => [c.key, c]))
  const shown = active ?? selected ?? null
  const captioned = shown ? byKey.get(shown) ?? null : null

  /** Shared by both kinds of target: a click, Enter and Space all pick. */
  const handlers = (combo: Combination | undefined) => ({
    role: 'button',
    tabIndex: combo ? 0 : -1,
    'aria-label': combo ? `${combo.name} — ${combo.tagline}` : undefined,
    style: { cursor: combo ? 'pointer' : 'default', outline: 'none' } as const,
    onClick: () => combo && onPick(combo),
    onKeyDown: (e: React.KeyboardEvent) => {
      if (!combo) return
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        onPick(combo)
      }
    },
    onMouseEnter: () => combo && setActive(combo.key),
    onMouseLeave: () => setActive(null),
    onFocus: () => combo && setActive(combo.key),
    onBlur: () => setActive(null),
  })

  return (
    // Centred rather than top-aligned: the diagram is square and the caption
    // beside it is four short lines, so aligning their tops left a third of
    // the panel empty under the text.
    <div className="flex flex-col items-center gap-4 sm:flex-row sm:items-center sm:gap-8">
      <svg
        viewBox="0 0 400 400"
        className="pentagram w-full max-w-[300px] shrink-0"
        role="group"
        aria-label="The colour wheel: five colours, ten guilds"
      >
        <defs>
          {/* A guild line is a gradient between the two colours it joins, so
              the line for Azorius literally runs white into blue. Oriented
              along the line rather than across the box, or a chord and an
              edge with the same colours would blend in different directions. */}
          {EDGES.map((e) => {
            const a = vertex(e.from)
            const b = vertex(e.to)
            return (
              <linearGradient
                key={e.key}
                id={`pent-${e.key}`}
                gradientUnits="userSpaceOnUse"
                x1={a.x} y1={a.y} x2={b.x} y2={b.y}
              >
                <stop offset="0%" stopColor={COLOR_VAR[WUBRG[e.from]]} />
                <stop offset="100%" stopColor={COLOR_VAR[WUBRG[e.to]]} />
              </linearGradient>
            )
          })}
        </defs>

        {/* Chords first, then the perimeter, then the discs: later elements
            paint over earlier ones, and the star crossing the middle must not
            be drawn on top of the colours it connects. */}
        {[...EDGES].sort((a, b) => Number(a.allied) - Number(b.allied)).map((e) => {
          const combo = byKey.get(e.key)
          const a = vertex(e.from)
          const b = vertex(e.to)
          const on = shown === e.key
          return (
            <g key={e.key} {...handlers(combo)} className="pentagram-edge">
              <line
                x1={a.x} y1={a.y} x2={b.x} y2={b.y}
                stroke={`url(#pent-${e.key})`}
                strokeWidth={on ? 9 : 4}
                strokeLinecap="round"
                // An enemy line is dashed. The allied/enemy split is the one
                // fact the colours themselves cannot carry — both kinds join
                // the same five discs — so it needs a mark of its own that
                // survives being read without colour.
                strokeDasharray={e.allied ? undefined : '10 8'}
                // The five discs are pale to begin with, and a gradient
                // between two of them is paler still. Much under this and the
                // ten lines stop reading as colour at all and become grey
                // scaffolding between the vertices.
                opacity={on ? 1 : 0.78}
              />
              {/* A 4px line is a 4px hit target. This one is invisible and
                  wide enough to actually point at, which is why the visible
                  line above sets `pointer-events: none` in the stylesheet. */}
              <line
                x1={a.x} y1={a.y} x2={b.x} y2={b.y}
                stroke="transparent" strokeWidth={20} strokeLinecap="round"
              />
            </g>
          )
        })}

        {WUBRG.map((code, i) => {
          const combo = byKey.get(code)
          const p = vertex(i)
          const on = shown === code
          return (
            <g key={code} {...handlers(combo)} className="pentagram-vertex">
              {/* The halo is the focus and hover indicator. Drawn under the
                  disc so it reads as a glow rather than a border, and sized
                  in the same units so it cannot drift from the disc. */}
              <circle
                cx={p.x} cy={p.y} r={VERTEX_R + (on ? 7 : 0)}
                fill={COLOR_VAR[code]}
                opacity={on ? 0.45 : 0}
              />
              <circle
                cx={p.x} cy={p.y} r={VERTEX_R}
                fill={COLOR_VAR[code]}
                stroke="var(--hairline)"
                strokeWidth={1}
              />
              {/* The colour's own icon, at the size the wheel gives it. Drawn
                  from the same paths as every pip in the app, so the disc a
                  vertex shows here is the disc a cost shows on a card — the
                  diagram teaches a mark the rest of the app then uses. */}
              <g transform={`translate(${p.x - 18} ${p.y - 18}) scale(0.36)`}>
                <path d={GLYPH_PATH[code].d} fill="#141414"
                      fillRule={GLYPH_PATH[code].evenOdd ? 'evenodd' : 'nonzero'} />
              </g>
            </g>
          )
        })}
      </svg>

      <div className="min-w-0 flex-1">
        {/* The caption is a fixed slot rather than something that appears on
            hover: a panel that grows when the pointer lands on a line would
            shove the diagram up the page and out from under the pointer. */}
        <div className="min-h-[7.5rem]">
          {captioned ? (
            <>
              <div className="flex flex-wrap items-baseline gap-2">
                <h3 className="text-xl font-semibold tracking-tight">{captioned.name}</h3>
                <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
                  {captioned.colors.length === 1
                    ? COLOR_NAMES[captioned.colors[0]]
                    : EDGES.find((e) => e.key === captioned.key)?.allied
                      ? 'allied pair — neighbours on the wheel'
                      : 'enemy pair — opposite on the wheel'}
                </span>
              </div>
              <p className="mt-1 text-sm leading-relaxed"
                 style={{ color: 'var(--text-secondary)' }}>
                {captioned.tagline}
              </p>
              {/* Says what the click does rather than what it is near. A
                  guild sits in a different tier from the diagram it was
                  clicked on, and a control that silently moves the tier
                  selector under you should say so first. */}
              <p className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
                {captioned.tier === 'mono'
                  ? `Click to read ${captioned.name} below.`
                  : `Click to cross to the guilds, at ${captioned.name}.`}
              </p>
            </>
          ) : (
            <p className="text-sm leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
              Five colours, and every pair of them has a name. Point at a disc
              for a mono-colour deck, or at a line for the guild that joins
              those two.
            </p>
          )}
        </div>

        <dl className="mt-2 space-y-1.5 text-xs" style={{ color: 'var(--text-muted)' }}>
          <div className="flex items-center gap-2">
            <svg width="34" height="8" aria-hidden className="shrink-0">
              <line x1="2" y1="4" x2="32" y2="4" strokeWidth="3"
                    strokeLinecap="round" stroke="var(--baseline)" />
            </svg>
            <dt className="sr-only">Solid lines</dt>
            <dd>The pentagon — five pairs of neighbours, the allied guilds.</dd>
          </div>
          <div className="flex items-center gap-2">
            <svg width="34" height="8" aria-hidden className="shrink-0">
              <line x1="2" y1="4" x2="32" y2="4" strokeWidth="3"
                    strokeLinecap="round" strokeDasharray="7 6"
                    stroke="var(--baseline)" />
            </svg>
            <dt className="sr-only">Dashed lines</dt>
            <dd>The star inside — five pairs of opposites, the enemy guilds.</dd>
          </div>
        </dl>
      </div>
    </div>
  )
}
