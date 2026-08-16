/**
 * The Wheel of Fortune (punch list 2026-08-15 item 9; rebuilt same day,
 * item 12 of the second list).
 *
 * Daniel Gelon's Alpha painting, and this time the painted wheel itself is
 * what turns. The first build drew an SVG wheel over the art and spun that;
 * it read as clip art sitting on a painting, because that is what it was.
 * Now the scene renders the painting twice from the same hotlinked crop:
 * once whole and still, and once clipped to the wheel's own circle — centre
 * measured at (65.54%, 44.35%) of the crop, diameter 54% of its width, fitted
 * numerically against the disc — with a feathered mask so the cut edge
 * blends into the rim's shadow. Rotating the clipped copy rotates the wheel
 * Gelon painted, planks, fates and all. No derivative is committed anywhere:
 * both copies are the same Scryfall URL the credit line already covers, and
 * the "edit" exists only as CSS at render time.
 *
 * The four fates sit where the painter put them, not on neat right angles —
 * cup 340°, heart 63°, sword 155°, skull 254°, measured clockwise from the
 * top of the crop — and the spin lands the chosen fate under the crowned
 * skull watching from the treetop, which sits at 0° and is the marker.
 *
 * The spin itself is the server's (`decks/wheel.py`): seeded dice over the
 * pool, a fate and a card in the deck's colours that answers to it. The
 * client's whole job is theatre — ask first, then decelerate onto the
 * answer, then show the card once the wheel has stopped. The theatre now
 * has a soundtrack to match: `wheelTurn` in `lib/tablesounds.ts` ratchets a
 * pawl over the studs on the same deceleration curve the CSS uses, and it
 * obeys the same switch every other table sound does. `answered_by:
 * "python"` renders in the caveat, because a fortune wheel is exactly where
 * somebody would otherwise assume Claude.
 *
 * The reveal waits on `transitionend`, with a timer as the fallback — a
 * hidden tab never paints the transition, and a spin whose result never
 * arrives is a broken toy. Reduced motion skips the theatre entirely: the
 * wheel jumps, the card shows, nothing clicks.
 */

import { useEffect, useRef, useState } from 'react'
import { api, errorMessage, type DeckRef, type WheelSpin } from '../lib/api'
import { wheelTurn } from '../lib/tablesounds'
import { CardArt, CardHover, ManaCost } from './ui'

const WHEEL_ART =
  'https://cards.scryfall.io/art_crop/front/6/7/67b369c4-faa8-45c8-a1b9-98f228b69682.jpg'

/** The wheel's circle within the art crop: centre as fractions of the crop's
 *  width and height, diameter as a fraction of width. Fitted against the
 *  painted disc (a grid search maximising plank-tan inside the circle vs the
 *  ring outside it), then checked by eye with an annotated render. */
const WHEEL_CX = 0.6554
const WHEEL_CY = 0.4435
/** Nudged out from the fitted 0.54 because the turning disc read as slightly
 *  undersized against the painting. The extra is a thin annulus of *not the
 *  disc* — planks and background, which do rotate along with it — so this can
 *  only go as far as the rim band below covers, and 0.556 is that limit at
 *  the scene's widths. Bigger than this wants a re-fit, not a bigger number. */
const WHEEL_DIAMETER = 0.556
/** The crop is 563x451; the cutout's percentage offsets depend on it. */
const ART_ASPECT = 563 / 451

/** Where each fate sits in the painting, in degrees clockwise from the top.
 *  Measured, not designed: Gelon spaced them like props, not like a clock
 *  face, and landing them honestly means using his angles. The order is
 *  still `wheel.SYMBOLS`'s — cup, heart, sword, skull, clockwise. */
const ANGLES: Record<string, number> = {
  cup: 340, heart: 63, sword: 155, skull: 254,
}

/** How long the CSS deceleration runs (`.wheel-disc` in index.css). The
 *  ratchet in `tablesounds` is scheduled against the same figure. */
const SPIN_MS = 3800

/**
 * The painted wheel, cut loose: the full crop absolutely positioned inside a
 * circle-masked box so the wheel's centre sits at the box's centre, rotated
 * as one piece. The box is sized so the crop always covers the circle at any
 * rotation — the nearest crop edge to the wheel's centre is farther away
 * than the radius, which was checked with the same measurements the mask
 * uses.
 */
function PaintedWheelCutout({ rotation, spinning, onDone }: {
  rotation: number
  spinning: boolean
  onDone: () => void
}) {
  // The cutout box is WHEEL_DIAMETER of the scene's width, square. Inside
  // it the crop renders at scene size again: width is 1/diameter of the
  // box, and the offsets put the wheel's centre at the box's centre.
  const imgW = 100 / WHEEL_DIAMETER
  const left = 50 - WHEEL_CX * imgW
  const top = 50 - (WHEEL_CY / ART_ASPECT) * imgW
  // The cutout and the rim are the same circle, so they take the same box.
  const geometry = {
    left: `${WHEEL_CX * 100}%`,
    top: `${WHEEL_CY * 100}%`,
    width: `${WHEEL_DIAMETER * 100}%`,
    aspectRatio: '1',
    transform: 'translate(-50%, -50%)',
  }
  return (
    <>
      <div className="wheel-cutout absolute" style={geometry}>
        <div className="wheel-disc absolute inset-0"
             onTransitionEnd={onDone}
             style={{
               transform: `rotate(${rotation}deg)`,
               transitionDuration: spinning ? undefined : '0ms',
             }}>
          <img src={WHEEL_ART} alt="" aria-hidden
               className="absolute max-w-none"
               style={{
                 width: `${imgW}%`,
                 left: `${left}%`,
                 top: `${top}%`,
               }} />
        </div>
      </div>
      {/* The rim goes over the seam, and outside the cutout so the cutout's
          own mask cannot clip it. It does not rotate: a rim belongs to the
          frame the wheel is mounted in, not to the wheel. */}
      <div className="wheel-rim absolute" aria-hidden style={geometry} />
    </>
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
      // The pawl ratchets over the twelve studs on the same curve the CSS
      // decelerates on; silent unless the table sound is switched on.
      wheelTurn(next - rotation, SPIN_MS)
      // The transition is 3.8s; the fallback only exists for tabs that
      // never paint (a hidden pane fires no transitionend).
      fallback.current = window.setTimeout(reveal, SPIN_MS + 400)
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
          <PaintedWheelCutout rotation={rotation} spinning={spinning}
                              onDone={reveal} />
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
        the wheel that turns is the painting&rsquo;s own, cut loose by a mask
        and put back exactly where it was.
      </p>
    </section>
  )
}
