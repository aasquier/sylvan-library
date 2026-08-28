/**
 * The room's colours, and the one property of them that is the whole point:
 * **no card image survives into what renders.**
 *
 * `SceneBackdrop` used to wash the page's masthead painting across the
 * viewport as a second `<img>` at `blur(11px)`. Scryfall's imagery guidelines
 * open their list with *"Do not blur, sharpen, desaturate, or color-shift card
 * images"*, and unlike every other finding in ADR 48's sweep there was no
 * layer to move the blur onto — the blurring *was* the effect. So the picture
 * left and its palette stayed.
 *
 * A rendering test can only see that the element is absent (`forest.test.tsx`
 * holds that). This is the other half: the value the sampler actually
 * produces, with a canvas standing in, checked for being a colour field and
 * nothing else.
 */

import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import { useArtWash } from './artwash'

const ART = 'https://cards.scryfall.io/art_crop/front/f/e/fed-cafe.jpg'

/** A painting that is unmistakably four colours in four corners, so a wash
 *  built from the wrong cells is visibly the wrong wash rather than a shade
 *  off. Filled per pixel because the sampler averages whole cells. */
function quadrants(w: number, h: number): Uint8ClampedArray {
  const px = new Uint8ClampedArray(w * h * 4)
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = (y * w + x) * 4
      const left = x < w / 2
      const top = y < h / 2
      px[i] = top ? (left ? 200 : 20) : (left ? 20 : 200)
      px[i + 1] = top ? 20 : 200
      px[i + 2] = left ? 20 : 200
      px[i + 3] = 255
    }
  }
  return px
}

/** Enough of a browser for the sampler: an `Image` that loads on assignment
 *  and a 2D context that hands back a known picture. jsdom has neither — which
 *  is exactly why the no-canvas path is the one `forest.test.tsx` exercises. */
function stubCanvas() {
  const realImage = globalThis.Image
  const realGetContext = HTMLCanvasElement.prototype.getContext

  class Loading {
    crossOrigin = ''
    onload: (() => void) | null = null
    onerror: (() => void) | null = null
    set src(_url: string) {
      queueMicrotask(() => this.onload?.())
    }
  }
  globalThis.Image = Loading as unknown as typeof Image

  HTMLCanvasElement.prototype.getContext = function (this: HTMLCanvasElement) {
    return {
      drawImage: () => {},
      getImageData: (_x: number, _y: number, w: number, h: number) =>
        ({ data: quadrants(w, h), width: w, height: h }),
    }
  } as unknown as typeof realGetContext

  return () => {
    globalThis.Image = realImage
    HTMLCanvasElement.prototype.getContext = realGetContext
  }
}

let restore: (() => void) | null = null
afterEach(() => {
  restore?.()
  restore = null
  vi.restoreAllMocks()
})

it('builds a colour field out of the painting and never the painting', async () => {
  restore = stubCanvas()

  const { result } = renderHook(() => useArtWash(ART))
  await waitFor(() => expect(result.current).not.toBeNull())

  const wash = result.current as string
  // **The assertion the whole change exists for.** Whatever else this string
  // is, it must not reach for the image: no `url()`, and not the art's own
  // address anywhere in it. A wash that quietly went back to referencing the
  // painting would render identically to one that did not.
  expect(wash).not.toContain('url(')
  expect(wash).not.toContain(ART)
  expect(wash).not.toContain('scryfall')

  // Nine cells, nine lobes of light, and a flat floor colour after them so no
  // corner of the viewport is ever bare.
  expect(wash.match(/radial-gradient\(/g)).toHaveLength(9)
  expect(wash.match(/rgb\(/g)).toHaveLength(10)

  // The corners of the source survive as the corners of the wash: the sampler
  // averages cells in place rather than blending the picture into mush. Top
  // left is the red quadrant, top right the dark one.
  const colours = [...wash.matchAll(/rgb\((\d+) (\d+) (\d+)\)/g)]
    .map((m) => m.slice(1).map(Number))
  expect(colours[0]?.[0]).toBeGreaterThan(150)
  expect(colours[2]?.[0]).toBeLessThan(60)
})

it('answers a second asking from the first read', async () => {
  restore = stubCanvas()

  const first = renderHook(() => useArtWash(ART))
  await waitFor(() => expect(first.result.current).not.toBeNull())

  // A cache hit is available on the very first render rather than after an
  // effect: every mastheaded route mounts a backdrop, and going back to a page
  // must not decode its painting again — nor flash an unwashed room while it
  // does.
  const again = renderHook(() => useArtWash(ART))
  expect(again.result.current).toBe(first.result.current)
})

it('is null for no art at all, without asking anything', () => {
  const { result } = renderHook(() => useArtWash(undefined))
  expect(result.current).toBeNull()
})
