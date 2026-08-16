/**
 * The Wheel of Fortune (punch list 2026-08-15 item 9).
 *
 * Daniel Gelon's Alpha painting, brought to life the only honest way: the
 * painting itself is the static scene — hotlinked from Scryfall and
 * credited, like every masthead — and over the painted wheel sits a *drawn*
 * wheel, planked and studded with the same four fates the painting carries
 * (cup, heart, sword, skull, clockwise from the top; a second skull watches
 * from the treetop, and it is the marker the spin lands under). The drawn
 * wheel is what turns. Nobody's scan of a 1993 card gets warped through a
 * rotation filter; the animation is geometry on top, in the painting's own
 * palette.
 *
 * The spin itself is the server's (`decks/wheel.py`): seeded dice over the
 * pool, a fate and a card in the deck's colours that answers to it. The
 * client's whole job is theatre — ask first, then decelerate onto the
 * answer, then show the card once the wheel has stopped. `answered_by:
 * "python"` renders in the caveat, because a fortune wheel is exactly where
 * somebody would otherwise assume Claude.
 *
 * The reveal waits on `transitionend`, with a timer as the fallback — a
 * hidden tab never paints the transition, and a spin whose result never
 * arrives is a broken toy. Reduced motion skips the theatre entirely: the
 * wheel jumps, the card shows.
 */

import { useEffect, useRef, useState } from 'react'
import { api, errorMessage, type DeckRef, type WheelSpin } from '../lib/api'
import { CardArt, CardHover, ManaCost } from './ui'

const WHEEL_ART =
  'https://cards.scryfall.io/art_crop/front/6/7/67b369c4-faa8-45c8-a1b9-98f228b69682.jpg'

/** Where each fate sits on the drawn wheel, in degrees clockwise from the
 *  top — the same order `wheel.SYMBOLS` serves, which is the contract. */
const ANGLES: Record<string, number> = {
  cup: 0, heart: 90, sword: 180, skull: 270,
}

/** The four fates, drawn small: a gold cup, a pink heart, a grey sword, an
 *  ivory skull — the painting's own props, simplified to read at 30px. */
function FateGlyph({ symbol }: { symbol: string }) {
  switch (symbol) {
    case 'cup':
      return (
        <g>
          <path d="M-7 -6 H 7 L 5 2 A 5 5 0 0 1 -5 2 Z" fill="#d9a72c" />
          <path d="M-3 5 H 3 L 4 8 H -4 Z" fill="#b8891f" />
          <ellipse cx="0" cy="-6" rx="7" ry="1.8" fill="#f0cf6e" />
        </g>
      )
    case 'heart':
      return (
        <path d="M0 8 C -9 1 -8 -6 -4 -7 C -1.5 -7.5 0 -5 0 -4
                 C 0 -5 1.5 -7.5 4 -7 C 8 -6 9 1 0 8 Z" fill="#d97a90" />
      )
    case 'sword':
      return (
        <g transform="rotate(35)">
          <path d="M-1.4 -9 H 1.4 L 1 4 H -1 Z" fill="#9aa2ad" />
          <path d="M-5 4 H 5 V 5.8 H -5 Z" fill="#6e5a2e" />
          <path d="M-1.2 5.8 H 1.2 L 1 9 H -1 Z" fill="#57431f" />
        </g>
      )
    default:
      return (
        <g>
          <ellipse cx="0" cy="-1.5" rx="6.5" ry="6" fill="#e8e0c8" />
          <path d="M-4 3.5 H 4 L 3 8 H -3 Z" fill="#e8e0c8" />
          <ellipse cx="-2.6" cy="-2" rx="1.7" ry="2.1" fill="#4a3d2a" />
          <ellipse cx="2.6" cy="-2" rx="1.7" ry="2.1" fill="#4a3d2a" />
          <path d="M-0.9 1.6 L 0 3 L 0.9 1.6 Z" fill="#4a3d2a" />
        </g>
      )
  }
}

/** The drawn wheel: planks, iron rim, hub, and the four fates. Rendered
 *  once; the spin is one CSS transform on the group. */
function DrawnWheel({ rotation, spinning }: {
  rotation: number
  spinning: boolean
}) {
  return (
    <svg viewBox="-50 -50 100 100" className="h-full w-full">
      <g className="wheel-disc"
         style={{
           transform: `rotate(${rotation}deg)`,
           transitionDuration: spinning ? undefined : '0ms',
         }}>
        <circle r="47" fill="#57431f" />
        <circle r="44" fill="#c0a878" />
        {/* Planks: chords across the face, the grain the painting shows. */}
        {[-33, -22, -11, 0, 11, 22, 33].map((y) => (
          <line key={y} x1={-Math.sqrt(44 * 44 - y * y)} y1={y}
                x2={Math.sqrt(44 * 44 - y * y)} y2={y}
                stroke="#8a6f45" strokeWidth="1.1" opacity="0.8" />
        ))}
        <circle r="44" fill="none" stroke="#6e5a2e" strokeWidth="2.5" />
        {/* Iron studs on the rim. */}
        {Array.from({ length: 12 }, (_, i) => {
          const a = (i * Math.PI * 2) / 12
          return <circle key={i} cx={Math.cos(a) * 40.5}
                         cy={Math.sin(a) * 40.5} r="1.3" fill="#4a3a22" />
        })}
        {/* The hub — the dark mounting the painting shows at centre. */}
        <circle r="4.5" fill="#3a2c16" />
        <circle r="2" fill="#241a0c" />
        {/* The four fates, upright relative to the wheel. */}
        {Object.entries(ANGLES).map(([symbol, angle]) => (
          <g key={symbol}
             transform={`rotate(${angle}) translate(0 -27) rotate(${-angle})`}>
            <FateGlyph symbol={symbol} />
          </g>
        ))}
      </g>
    </svg>
  )
}

export function WheelOfFortune({ deckRef }: { deckRef: DeckRef }) {
  const [spin, setSpin] = useState<WheelSpin | null>(null)
  const [rotation, setRotation] = useState(0)
  const [spinning, setSpinning] = useState(false)
  const [revealed, setRevealed] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fallback = useRef<number | null>(null)

  useEffect(() => () => {
    if (fallback.current) window.clearTimeout(fallback.current)
  }, [])

  function reveal() {
    if (fallback.current) window.clearTimeout(fallback.current)
    fallback.current = null
    setSpinning(false)
    setRevealed(true)
  }

  async function turn() {
    if (spinning) return
    setError(null)
    setRevealed(false)
    try {
      const result = await api.wheelSpin(deckRef)
      setSpin(result)
      if (!result.pool_available || !result.symbol) {
        setRevealed(true)
        return
      }
      // Optional-chained for jsdom, which has no matchMedia at all.
      const still =
        window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
      const target = ANGLES[result.symbol] ?? 0
      // Land the fate under the treetop skull: the wheel turns so the
      // chosen symbol ends at the top. Four full turns of theatre first,
      // always forward from wherever the last spin left it.
      const current = ((rotation % 360) + 360) % 360
      const next = rotation + 4 * 360 + ((360 - target - current + 720) % 360)
      if (still) {
        setRotation(next)
        setRevealed(true)
        return
      }
      setSpinning(true)
      setRotation(next)
      // The transition is 3.8s; the fallback only exists for tabs that
      // never paint (a hidden pane fires no transitionend).
      fallback.current = window.setTimeout(reveal, 4200)
    } catch (e) {
      setError(errorMessage(e))
    }
  }

  return (
    <section className="card-surface space-y-3 rounded-xl p-5">
      <div className="flex flex-wrap items-baseline gap-2">
        <h3 className="text-sm font-semibold">The Wheel of Fortune</h3>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          Spin for a card in this deck&rsquo;s colours. Python rolls; nobody
          opines.
        </span>
      </div>

      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="wheel-scene relative w-full max-w-sm shrink-0 overflow-hidden rounded-lg">
          <img src={WHEEL_ART}
               alt="Wheel of Fortune, painted by Daniel Gelon for Limited
                    Edition Alpha: a red-cloaked figure spins a plank wheel
                    mounted on a great tree."
               className="block w-full" />
          {/* The drawn wheel sits exactly over the painted one: centre at
              69% / 47% of the crop, diameter just over half its width. */}
          <div className="absolute"
               onTransitionEnd={reveal}
               style={{ left: '69%', top: '47%', width: '53%',
                        aspectRatio: '1', transform: 'translate(-50%, -50%)' }}>
            <DrawnWheel rotation={rotation} spinning={spinning} />
          </div>
        </div>

        <div className="min-w-0 flex-1 space-y-2">
          <button type="button" onClick={() => void turn()} disabled={spinning}
                  className="rounded-lg px-4 py-2 text-sm font-medium disabled:opacity-50"
                  style={{ background: 'var(--series-1)', color: '#fff' }}>
            {spinning ? 'The wheel turns…' : 'Spin the wheel'}
          </button>

          {error && (
            <p className="text-xs" style={{ color: 'var(--status-critical)' }}>
              {error}
            </p>
          )}

          {spin && !spin.pool_available && revealed && (
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              {spin.message}
            </p>
          )}

          {spin && revealed && spin.symbol && (
            <div className="space-y-2">
              <p className="text-sm font-medium">
                {spin.label}
                <span className="ml-2 text-xs font-normal"
                      style={{ color: 'var(--text-secondary)' }}>
                  {spin.meaning}
                </span>
              </p>
              {spin.card
                ? (
                  <div className="flex flex-wrap items-center gap-3 rounded-lg p-2"
                       style={{ background: 'var(--surface-1)' }}>
                    <CardHover card={{ name: spin.card.name,
                                       image: spin.card.image }}>
                      <CardArt src={spin.card.art_crop} alt={spin.card.name}
                               ratio="aspect-[626/457]"
                               className="w-20 shrink-0 cursor-help" />
                    </CardHover>
                    <div className="min-w-0 flex-1 basis-48">
                      <div className="flex flex-wrap items-baseline gap-2">
                        <span className="text-sm font-medium">
                          {spin.card.name}
                        </span>
                        <ManaCost cost={spin.card.mana_cost} />
                      </div>
                      <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
                        {spin.card.type_line}
                      </p>
                      <p className="mt-1 whitespace-pre-line text-xs leading-relaxed"
                         style={{ color: 'var(--text-secondary)' }}>
                        {spin.card.oracle_text}
                      </p>
                    </div>
                  </div>
                  )
                : (
                  <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
                    {spin.reason}
                  </p>
                  )}
              <p className="text-[11px] leading-relaxed"
                 style={{ color: 'var(--text-muted)' }}>
                {spin.caveat} Seed {spin.seed} — the same seed re-spins the
                same fate.
              </p>
            </div>
          )}
        </div>
      </div>

      <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
        <em>Wheel of Fortune</em> by Daniel Gelon, Limited Edition Alpha —
        the drawn wheel over it turns so the painting doesn&rsquo;t have to.
      </p>
    </section>
  )
}
