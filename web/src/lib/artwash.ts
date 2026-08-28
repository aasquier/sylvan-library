/**
 * The colours a painting is made of, without the painting.
 *
 * `SceneBackdrop` washes the page's own masthead art across the whole
 * viewport, and until ADR 48 it did that the obvious way: the same `<img>`
 * again, `blur(11px)`, dimmed and masked. Scryfall's imagery guidelines name
 * that in the first verb of the clause — *"Do not blur, sharpen, desaturate,
 * or color-shift card images"* (fetched 2026-08-28) — and unlike every other
 * finding in that sweep, the blur here **was** the effect. There was no layer
 * to move it to, because the thing being altered was the whole point.
 *
 * So the picture leaves and its colours stay. The art is read once into a
 * 24x18 canvas, averaged down to a 3x3 grid, and handed back as nine swatches
 * the stylesheet lays out as overlapping radial gradients. What renders is a
 * colour field in the painting's own palette: the room keeps the connection to
 * the page it belongs to, and there is no card image on screen to violate
 * anything. It is also simply better — the sharp-clone problem the old rule's
 * comment records, where a blurred copy bleeding out from behind the crisp
 * masthead "read as a rendering mistake", cannot happen to nine colours.
 *
 * **Nothing is rehosted and nothing new is fetched.** The sampling reads the
 * same hot-linked image the masthead is already loading, in the browser, from
 * cache. `cards.scryfall.io` answers with `access-control-allow-origin: *`,
 * which is what makes `crossOrigin = 'anonymous'` work and the canvas readable
 * rather than tainted — checked against the live headers 2026-08-28. If that
 * ever stops being true the read throws, this returns null, and the room falls
 * back to the procedural loop already rendering under it. A missing wash is a
 * quieter page, never a broken one.
 *
 * Deterministic by construction: one image in, the same nine colours out,
 * every time. No sampling positions are chosen, nothing is seeded, and there
 * is no arithmetic here a golden could disagree with.
 */

import { useEffect, useState } from 'react'

/** The grid the wash is built from. Three by three is chosen against what it
 *  has to do rather than for tidiness: it is enough to keep a painting's
 *  light where the painting had it — sky above, ground below — and far too
 *  coarse for anything to be recognisable in. Nine colours are a palette. */
const ACROSS = 3
const DOWN = 3

/** What the image is decoded into before averaging. Eight pixels per cell
 *  each way, which is plenty to average and small enough that the decode is
 *  not worth measuring. */
const W = ACROSS * 8
const H = DOWN * 6

/** One answer per URL, for the life of the tab. Every mastheaded route mounts
 *  a backdrop, and going back to a page must not decode its painting again. */
const known = new Map<string, string | null>()

/** In flight, so two components asking for the same art at the same moment
 *  make one decode between them rather than one each. */
const asking = new Map<string, Promise<string | null>>()

function averageGrid(px: Uint8ClampedArray): string[] {
  const cw = W / ACROSS
  const ch = H / DOWN
  const out: string[] = []
  for (let gy = 0; gy < DOWN; gy++) {
    for (let gx = 0; gx < ACROSS; gx++) {
      let r = 0
      let g = 0
      let b = 0
      let n = 0
      for (let y = gy * ch; y < (gy + 1) * ch; y++) {
        for (let x = gx * cw; x < (gx + 1) * cw; x++) {
          // `?? 0` for the type checker rather than for the arithmetic: the
          // indices are inside a buffer this function sized itself, so the
          // fallback is unreachable. Alpha is skipped — the canvas was opaque
          // before the image was drawn into it.
          const i = (y * W + x) * 4
          r += px[i] ?? 0
          g += px[i + 1] ?? 0
          b += px[i + 2] ?? 0
          n++
        }
      }
      out.push(`rgb(${Math.round(r / n)} ${Math.round(g / n)} ${Math.round(b / n)})`)
    }
  }
  return out
}

/** The nine swatches as one CSS `background`: a lobe of light per cell, laid
 *  where that cell was, over a flat fill of the middle one so no corner of the
 *  viewport is ever bare. Generous radii, because the lobes have to overlap
 *  into each other — nine hard circles would be a colour chart, and what this
 *  is standing in for is a painting at eleven pixels of blur. */
function washFrom(swatches: string[]): string {
  const layers = swatches.map((colour, i) => {
    const x = ((i % ACROSS) + 0.5) * (100 / ACROSS)
    const y = (Math.floor(i / ACROSS) + 0.5) * (100 / DOWN)
    return `radial-gradient(58% 52% at ${x.toFixed(1)}% ${y.toFixed(1)}%, `
      + `${colour} 0%, transparent 100%)`
  })
  // The centre cell as the floor under all of them.
  return `${layers.join(', ')}, ${swatches[4]}`
}

function sample(art: string): Promise<string | null> {
  return new Promise((settle) => {
    // Guarded because jsdom has no canvas at all and would throw on the
    // constructor: the suite must read "no wash", not "the room crashed".
    if (typeof document === 'undefined') return settle(null)
    const img = new Image()
    // What makes the pixels readable instead of tainting the canvas. Set
    // before `src`, or the request goes out without the CORS mode and the
    // attribute is ignored.
    img.crossOrigin = 'anonymous'
    img.onerror = () => settle(null)
    img.onload = () => {
      try {
        const canvas = document.createElement('canvas')
        canvas.width = W
        canvas.height = H
        const ctx = canvas.getContext('2d', { willReadFrequently: true })
        if (!ctx) return settle(null)
        ctx.drawImage(img, 0, 0, W, H)
        settle(washFrom(averageGrid(ctx.getImageData(0, 0, W, H).data)))
      } catch {
        // A tainted canvas throws here rather than earlier. Same answer as a
        // failed load: the room does without.
        settle(null)
      }
    }
    img.src = art
  })
}

/**
 * The wash for one painting, as a CSS `background` value, or null while it is
 * unknown and forever if it cannot be read.
 *
 * Null is a first-class answer and the caller renders nothing for it — the
 * procedural mood loop underneath is the room on its own, which is what every
 * page looked like before backdrops existed.
 */
export function useArtWash(art: string | undefined): string | null {
  // **The answer is read during render and the state only exists to schedule
  // one.** A `setWash` on a cache hit would be a second render to arrive at
  // the value the first one already had — the cascade `set-state-in-effect`
  // is named after. So `known` is consulted below, and the effect's only job
  // is to fill it and then say so.
  const [done, setDone] = useState<{ art: string; wash: string | null } | null>(null)

  useEffect(() => {
    if (!art || known.has(art)) return
    let live = true
    let inflight = asking.get(art)
    if (!inflight) {
      inflight = sample(art).then((got) => {
        known.set(art, got)
        asking.delete(art)
        return got
      })
      asking.set(art, inflight)
    }
    void inflight.then((got) => { if (live) setDone({ art, wash: got }) })
    return () => { live = false }
  }, [art])

  if (!art) return null
  const cached = known.get(art)
  if (cached !== undefined) return cached
  // Only reachable in the frame between the promise settling and the cache
  // being consulted again; `art` is checked because a route change can land a
  // stale answer on a new painting.
  return done && done.art === art ? done.wash : null
}
