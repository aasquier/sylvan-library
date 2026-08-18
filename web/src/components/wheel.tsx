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
import { useTableSound } from '../lib/prefs'
import {
  fateLand, swampStart, swampStop, thunderRoll, wheelTurn,
} from '../lib/tablesounds'
import beetleUrl from '../assets/wheel/wheel-beetle.webp'
import cocUrl from '../assets/wheel/wheel-coin-obverse.webp'
import corUrl from '../assets/wheel/wheel-coin-reverse.webp'
import crocUrl from '../assets/wheel/wheel-croc.webp'
import lanternUrl from '../assets/wheel/wheel-lantern.webp'
import owlUrl from '../assets/wheel/wheel-owl.webp'
import shade1Url from '../assets/wheel/wheel-shade-1.webp'
import shade2Url from '../assets/wheel/wheel-shade-2.webp'
import shade3Url from '../assets/wheel/wheel-shade-3.webp'
import sparksUrl from '../assets/wheel/wheel-sparks.webp'
import swordUrl from '../assets/wheel/wheel-sword.webp'
import { CardArt, CardHover, ManaCost } from './ui'

const WHEEL_ART =
  'https://cards.scryfall.io/art_crop/front/6/7/67b369c4-faa8-45c8-a1b9-98f228b69682.jpg'

/** The whole card, for the folded state: the wheel collapses into the
 *  very card it came from, which is the only icon it could honestly
 *  have. Hotlinked like the art crop, never committed. */
const WHEEL_CARD =
  'https://cards.scryfall.io/normal/front/6/7/67b369c4-faa8-45c8-a1b9-98f228b69682.jpg'

/** Folded or unfolded survives a reload; open is the absence of the key,
 *  the same shape as every other preference here. */
const OPEN_KEY = 'mtglab-wheel-open'

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

/** The red-cloaked figure's home in the crop (fractions of the scene) —
 *  the ellipse the living-copy mask feathers around, and the feet the
 *  breathe pivots on. Measured off the painting, like everything else
 *  in this file. */
const FIGURE = { cx: 0.16, cy: 0.55, rx: 0.155, ry: 0.33, feet: '16% 84%' }

/** The crowned skull in the treetop — the wheel's clicker. Pivots where
 *  the bone meets the branch. */
const CLICKER = { cx: 0.645, cy: 0.055, rx: 0.075, ry: 0.085 }

/** The figure's outstretched arm, shoulder to fingertips. Its tip grazes
 *  the disc's edge, so this copy renders BELOW the wheel cutout — during
 *  a spin the moving planks paint over the overlap, and a frozen smudge
 *  of wheel never rides the hand. Pivot at the shoulder. Trimmed on the
 *  left (0.315/0.105 → 0.33/0.09) so its moving feather stays off the
 *  chest where the lantern hangs — pixels sliding under a stationary
 *  object was most of what read as the lantern "interacting funny". */
const ARM = { cx: 0.33, cy: 0.5, rx: 0.09, ry: 0.07 }

/** The great branch arcing over the figure's head. A tree this old does
 *  not sway; it CREAKS — a fraction of a degree on a long cycle, pivoted
 *  where the limb leaves the trunk. Below the figure copy, because the
 *  painter put the hood in front of the limb. */
const BRANCH = { cx: 0.22, cy: 0.13, rx: 0.3, ry: 0.17 }

/** The pale forest curtain behind the figure. It stirs rather than sways —
 *  a whole stand of dead wood leaning a breath together, pivoted at the
 *  ground so the roots stay planted. */
const FOREST = { cx: 0.14, cy: 0.3, rx: 0.17, ry: 0.34 }

/** The pool along the painting's bottom edge — the blue-grey wash Gelon
 *  ran under the roots. A masked copy breathes sideways by a pixel or two,
 *  which is what standing water does under moving air. */
const POOL = { cx: 0.5, cy: 0.96, rx: 0.56, ry: 0.08 }

const ellipseMask = (f: { cx: number; cy: number; rx: number; ry: number }) =>
  `radial-gradient(ellipse ${f.rx * 100}% ${f.ry * 100}% at `
  + `${f.cx * 100}% ${f.cy * 100}%, #000 55%, rgba(0,0,0,0.6) 78%, `
  + 'transparent 98%)'

/**
 * A living copy: the same hotlinked crop, masked to one painted thing, so
 * a CSS transform moves that thing and nothing else. The mask feathers
 * wide and the motion is small, which is what keeps the seam invisible —
 * the copy's edges land on pixels that barely move. Same argument as the
 * wheel cutout: no derivative exists anywhere, the "edit" is CSS at
 * render time.
 */
function LivingCopy({ region, className, style }: {
  region: { cx: number; cy: number; rx: number; ry: number }
  className: string
  style?: React.CSSProperties
}) {
  const mask = ellipseMask(region)
  return (
    <img src={WHEEL_ART} alt="" aria-hidden
         className={`absolute inset-0 w-full ${className}`}
         style={{
           WebkitMaskImage: mask,
           maskImage: mask,
           ...style,
         } as React.CSSProperties} />
  )
}

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
        <div className={`wheel-disc absolute inset-0${spinning ? ' is-spinning' : ''}`}
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
          {/* (The twelve visible studs were tried and ditched — Aaron:
              "distracting". The pawl still clicks over them in the
              sound; the eye takes the painter's word for it.) */}
        </div>
      </div>
      {/* The rim goes over the seam, and outside the cutout so the cutout's
          own mask cannot clip it. It does not rotate: a rim belongs to the
          frame the wheel is mounted in, not to the wheel — but light does
          walk it, and walks faster while the planks turn. */}
      <div className={`wheel-rim absolute${spinning ? ' is-spinning' : ''}`}
           aria-hidden style={geometry} />
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
  const landed = useRef<WheelSpin | null>(null)
  const [sound] = useTableSound()
  const [open, setOpen] = useState(() => {
    try {
      return localStorage.getItem(OPEN_KEY) !== '0'
    } catch {
      return true
    }
  })

  function toggleOpen() {
    setOpen((was) => {
      const next = !was
      try {
        if (next) localStorage.removeItem(OPEN_KEY)
        else localStorage.setItem(OPEN_KEY, '0')
      } catch { /* private browsing: the fold just does not persist */ }
      return next
    })
  }

  useEffect(() => () => {
    if (fallback.current) window.clearTimeout(fallback.current)
  }, [])

  // The clearing sings while you can hear it: rain, crickets, drips, the
  // odd frog and the odder owl — behind the same switch as every other
  // table sound, and torn down the moment the wheel leaves the page or
  // the switch flips.
  useEffect(() => {
    if (!sound || !open) return
    swampStart()
    return () => swampStop()
  }, [sound, open])

  // The storm: every half-minute or so, lightning somewhere past the
  // trees — the flash renders at once, and the thunder is handed its
  // distance so the sky and the sound stay one storm. Reduced motion
  // gets neither; a flash is exactly what that setting asks not to see.
  const [bolt, setBolt] = useState(0)
  useEffect(() => {
    if (!open) return
    const still =
      window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
    if (still) return
    let alive = true
    let timer = 0
    const schedule = () => {
      timer = window.setTimeout(() => {
        if (!alive) return
        if (!document.hidden) {
          setBolt(Date.now())
          thunderRoll(1.1 + Math.random() * 1.7)
        }
        schedule()
      }, 24000 + Math.random() * 26000)
    }
    schedule()
    return () => {
      alive = false
      window.clearTimeout(timer)
    }
  }, [open])

  function reveal() {
    if (fallback.current) window.clearTimeout(fallback.current)
    fallback.current = null
    setSpinning(false)
    setRevealed(true)
    // The fate's own voice, once — the ref carries the fresh spin past the
    // stale closure a fallback timer holds, and clearing it keeps the
    // transitionend that can trail the timer from sounding it twice.
    // The face rides along, because a broken heart and an offered hilt
    // do not sound like their whole and violent twins.
    const s = landed.current
    landed.current = null
    if (s?.symbol) {
      fateLand(s.symbol,
               s.coin ?? s.heart_face ?? s.sword_face ?? s.skull_face)
    }
  }

  async function turn() {
    if (spinning) return
    setError(null)
    setRevealed(false)
    try {
      const result = await api.wheelSpin(deckRef)
      setSpin(result)
      landed.current = result
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
        reveal()
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

  // Folded, the wheel is the card it came from — nothing else renders,
  // no ambience plays, no storm schedules. One click unfolds the
  // clearing again.
  if (!open) {
    return (
      <button type="button" onClick={toggleOpen} aria-expanded={false}
              className="wheel-folded"
              title="Unfold the Wheel of Fortune">
        <img src={WHEEL_CARD}
             alt="Wheel of Fortune, the card — unfold the wheel" />
      </button>
    )
  }

  return (
    <section className="card-surface wheel-frame space-y-3 rounded-xl p-5">
      <div className="flex flex-wrap items-baseline gap-2">
        <h3 className="text-sm font-semibold">The Wheel of Fortune</h3>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          Spin for a card in this deck&rsquo;s colours. The dice decide;
          nobody opines.
        </span>
        <button type="button" onClick={toggleOpen} aria-expanded
                className="wheel-fold-btn ml-auto"
                title="Fold the wheel away into its card">
          Fold away
        </button>
      </div>

      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="wheel-scene relative w-full max-w-sm shrink-0 overflow-hidden rounded-lg">
          <img src={WHEEL_ART}
               alt="Wheel of Fortune, painted by Daniel Gelon for Limited
                    Edition Alpha: a red-cloaked figure spins a plank wheel
                    mounted on a great tree."
               className="block w-full" />
          {/* The light through the trees: two cold shafts falling from the
              upper left across the pale forest curtain, swelling and dying
              on long uneven cycles — sun through a dead wood on a moving
              sky. Under the living copies, so the figure and the clicker
              catch it. */}
          <span className="wheel-ray" aria-hidden="true" />
          <span className="wheel-ray is-second" aria-hidden="true" />
          {/* Moonlight from the upper-right corner: one silver shaft over
              the trunk, guttering as clouds cross a moon the frame never
              shows. Colder than the rays on purpose — two lights from two
              skies is what makes the clearing feel deep. */}
          <span className="wheel-moon" aria-hidden="true" />
          <span className="wheel-moonglow" aria-hidden="true" />
          {/* The dead wood stirs: the whole pale curtain leans a breath
              together, planted at the roots. */}
          <LivingCopy region={FOREST} className="wheel-forest"
                      style={{ transformOrigin: '14% 88%' }} />
          {/* The pool breathes sideways by a pixel — standing water under
              moving air — and a glint walks it. */}
          <LivingCopy region={POOL} className="wheel-pool"
                      style={{ transformOrigin: '50% 100%' }} />
          <span className="wheel-pool-glint" aria-hidden="true" />
          {/* The tree creaks: the limb over the figure's head leans a
              fraction of a degree on a seventeen-second cycle, pivoted
              where it leaves the trunk. */}
          <LivingCopy region={BRANCH} className="wheel-branch"
                      style={{ transformOrigin: '46% 8%' }} />
          {/* The barn owl (menagerie.recipe.yaml) keeps the limb over
              the figure's head, doing what owls do: nothing, superbly.
              She breathes, settles her weight once in a while, and her
              eyes catch the moonlight; when she has something to say,
              the ambience says who. */}
          <span className="wheel-owl" aria-hidden="true">
            <img src={owlUrl} alt="" className="wheel-owl-body" />
            <span className="wheel-owl-eyes" />
          </span>
          {/* The figure lives: breath from the feet up, and on a long cycle
              a slow lean toward the wheel it keeps spinning. */}
          <LivingCopy region={FIGURE} className="wheel-figure"
                      style={{ transformOrigin: FIGURE.feet }} />
          {/* The idle hand has a job now: the Hermit's own star-pierced
              candle-lantern (lantern.recipe.yaml). The anchor runs the
              figure's exact breathe animation, so the lantern rides
              the chest it hangs on instead of hovering while painted
              pixels slide beneath it; its own sway swings inside
              that. */}
          <span className="wheel-lantern-anchor" aria-hidden="true">
            <span className="wheel-lantern-rig">
              {/* The chain (Aaron's idea): a hand's length of links from
                  the grip below the cowl down to the lantern's loop —
                  which hangs the lantern clear of the robe's folds and
                  gives the sway a thing to swing FROM. */}
              <span className="wheel-chain" />
              <span className="wheel-lantern-light" />
              <img src={lanternUrl} alt="" className="wheel-lantern" />
            </span>
          </span>
          {/* The hand hovers at the rim — a small tremor at rest, and the
              PUSH when the wheel is thrown. Below the cutout on purpose;
              see ARM. */}
          <LivingCopy region={ARM}
                      className={`wheel-arm${spinning ? ' is-pushing' : ''}`}
                      style={{ transformOrigin: '24% 50%' }} />
          <PaintedWheelCutout rotation={rotation} spinning={spinning}
                              onDone={reveal} />
          {/* The clicker, above the disc: the crowned skull rocks on its
              branch — a slow watchful tilt at rest, a decaying flap while
              the planks rattle under it. */}
          <LivingCopy region={CLICKER}
                      className={`wheel-clicker${spinning ? ' is-flapping' : ''}`}
                      style={{ transformOrigin: '64.5% 2%' }} />
          {/* The knot-hole in the right trunk is a doorway to somewhere
              dark, and different things take turns looking out of it:
              three pairs of eyes — amber, green, a cold pale blue —
              each with its own height, its own spacing, and its own
              way of blinking. Nobody is ever sure what lives in there,
              which is the point. */}
          <span className="wheel-hollow" aria-hidden="true">
            <span className="wheel-hollow-eyes is-first" />
            <span className="wheel-hollow-eyes is-second" />
            <span className="wheel-hollow-eyes is-third" />
          </span>
          {/* The pool has a tenant too: a crocodile in the painting's own
              blue-grey (menagerie.recipe.yaml), who surfaces by inches,
              watches a long while, and sinks without a sound. The rig
              clips him at the waterline. */}
          <span className="wheel-croc-rig" aria-hidden="true">
            <span className="wheel-croc-body">
              <img src={crocUrl} alt="" className="wheel-croc" />
              {/* His eye takes the light (Aaron, round four) — the one
                  glint that says the log is looking back. */}
              <span className="wheel-croc-eye" />
            </span>
          </span>
          {/* Something lives among the roots (critter.recipe.yaml): a
              photographed Carabus, CC0 and matted through the animist,
              moving the way a ground beetle actually moves — parked for
              long stretches, then a quick dart to the next dark spot.
              The wrapper walks the ground; the img inside turns to face
              each run. */}
          <span className="wheel-critter" aria-hidden="true">
            <img src={beetleUrl} alt="" />
          </span>
          {/* Lightning past the trees: mounted per bolt so the flash
              runs once and unmounts with the next; the thunder was
              scheduled the same instant, further away. */}
          {bolt > 0 && (
            <span key={bolt} className="wheel-lightning" aria-hidden="true" />
          )}
          {/* What the landed fate does to the room, once, at the reveal —
              each wash under its own photographed centrepiece: the cup
              spins real hoard gold, the heart is Pinson's wax beating in
              a pocket of gloom, the sword strikes sparks and bleeds, the
              skull raises the dead from every side. */}
          {revealed && spin?.symbol && (
            <span key={`${spin.seed}-${spin.symbol}`}
                  className={`wheel-fatefx is-${spin.symbol}${
                    spin.coin === 'tails' ? ' is-tails' : ''}${
                    spin.heart_face === 'broken' ? ' is-broken' : ''}`}
                  aria-hidden="true" />
          )}
          {revealed && spin?.symbol === 'cup' && (
            <span key={`coin-${spin.seed}`} className="wheel-coinspin"
                  aria-hidden="true">
              {/* The turn ends on the face the dice actually landed:
                  Athena for heads, her owl for tails. */}
              <span className={`wheel-coin3d${
                spin.coin === 'tails' ? ' is-tails' : ''}`}>
                <img src={cocUrl} alt="" className="wheel-coin-face" />
                <img src={corUrl} alt="" className="wheel-coin-face is-back" />
              </span>
            </span>
          )}
          {revealed && spin?.symbol === 'heart' && (
            <span key={`heart-${spin.seed}`} className="wheel-heartfx"
                  aria-hidden="true">
              {/* Gelon's own painted heart, cut from the crop by
                  background arithmetic and made luminous — and when the
                  fate lands broken, it renders as two halves torn on a
                  jagged line, drifting apart as they beat. */}
              {spin.heart_face === 'broken'
                ? (
                  <>
                    <span className="wheel-heart-bloom is-half is-left"
                          style={{ backgroundImage: `url(${WHEEL_ART})` }} />
                    <span className="wheel-heart-bloom is-half is-right"
                          style={{ backgroundImage: `url(${WHEEL_ART})` }} />
                  </>
                  )
                : (
                  <span className="wheel-heart-bloom"
                        style={{ backgroundImage: `url(${WHEEL_ART})` }} />
                  )}
            </span>
          )}
          {revealed && spin?.symbol === 'skull' && (
            <span key={`ghosts-${spin.seed}`}
                  className={`wheel-ghosts${
                    spin.skull_face === 'buried' ? ' is-buried' : ''}`}
                  aria-hidden="true">
              {/* Real smoke, graded grave-green (shades.recipe.yaml) —
                  screen blend erases the black around it, so what rises
                  is only the smoke itself. */}
              <img src={shade1Url} alt="" className="wheel-ghost is-g1" />
              <img src={shade2Url} alt="" className="wheel-ghost is-g2" />
              <img src={shade3Url} alt="" className="wheel-ghost is-g3" />
              <img src={shade2Url} alt="" className="wheel-ghost is-g4" />
              <img src={shade1Url} alt="" className="wheel-ghost is-g5" />
            </span>
          )}
          {revealed && spin?.symbol === 'sword' && spin.sword_face === 'hilt' && (
            <span key={`offer-${spin.seed}`} className="wheel-offer"
                  aria-hidden="true">
              {/* The peaceful landing: one sword descends hilt-first —
                  the old sign of fealty — gleams, and is gone. No
                  sparks, no blood; an offer, not a blow. */}
              <img src={swordUrl} alt="" className="wheel-offer-blade" />
              <i className="wheel-offer-gleam" />
            </span>
          )}
          {revealed && spin?.symbol === 'sword' && spin.sword_face !== 'hilt' && (
            <span key={`sword-${spin.seed}`} className="wheel-strike"
                  aria-hidden="true">
              {/* The Met's two-hander twice over — a matched pair
                  swinging in from the wings to meet over the wheel,
                  sparks at the bind, and then the wheel bleeds. */}
              <img src={swordUrl} alt="" className="wheel-clash-blade is-left" />
              <img src={swordUrl} alt="" className="wheel-clash-blade is-right" />
              <i className="wheel-strike-flash" />
              {/* Real fire (fates.recipe.yaml): a photographed spark
                  shower, twice, mirrored and staggered — screen blend
                  eats its black and leaves only the burning. */}
              <img src={sparksUrl} alt="" className="wheel-sparkburst is-a" />
              <img src={sparksUrl} alt="" className="wheel-sparkburst is-b" />
              <i className="wheel-spark is-s1" />
              <i className="wheel-spark is-s2" />
              <i className="wheel-spark is-s3" />
              <i className="wheel-spark is-s4" />
              <i className="wheel-spark is-s5" />
              <i className="wheel-spark is-s6" />
              <i className="wheel-spark is-s7" />
              <i className="wheel-spark is-s8" />
              <i className="wheel-spark is-s9" />
              <i className="wheel-spark is-s10" />
              <i className="wheel-spark is-s11" />
              <i className="wheel-spark is-s12" />
              <i className="wheel-blood is-b1" />
              <i className="wheel-blood is-b2" />
              <i className="wheel-blood is-b3" />
            </span>
          )}
        </div>

        <div className="min-w-0 flex-1 space-y-2">
          <button type="button" onClick={() => void turn()} disabled={spinning}
                  className={`wheel-spin-btn${spinning ? ' is-turning' : ''}`}>
            <span className="wheel-spin-btn-glyph" aria-hidden="true" />
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
                {spin.caveat}
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
