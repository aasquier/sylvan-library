/** The OCR engine, kept behind a door that only opens when the camera does.
 *
 * Tesseract is about six megabytes of WebAssembly and trained data. None of
 * it is in the bundle and none of it is in git: the server fetches the
 * three files once, pins each by SHA-256, and serves them from `/api/ocr`
 * (the arrangement ADR 33 already uses for the mana symbols). The `import()`
 * below is what keeps the entry chunk free of even the small part — nobody
 * who never opens a viewfinder pays for one.
 *
 * ## Two reads per card, not one
 *
 * The corner and the title want opposite settings. The corner is a known,
 * tiny alphabet where restricting the character set is the single biggest
 * accuracy win available; the title is a line of ordinary words where the
 * same restriction would mangle it. So the worker's parameters are set
 * between the two `recognize` calls rather than two workers being spawned —
 * a second worker means a second four-megabyte core in a phone's memory.
 *
 * Nothing here decides what a card is. It produces the `Sighting` that
 * `/api/cards/identify` reads, and the pool does the deciding.
 */

import type { PSM, Worker } from 'tesseract.js'
import {
  CORNER, CORNER_ALPHABET, TITLE, cleanTitle, pixels, type Region,
} from './cardframe'

/** What one capture thought it saw — the wire shape `/api/cards/identify`
 *  takes. Every field is optional and independently unreliable. */
export interface Sighting {
  /** The bottom-left block verbatim, newlines and all. Sent as text rather
   *  than picked apart here: a real capture returns `LTCENLIK`, and finding
   *  `LTC` inside it is a question about which set codes exist — which the
   *  pool answers and a browser cannot. */
  corner?: string
  title?: string
}

/** Tesseract page-segmentation modes, as the literal strings the enum holds
 *  (`PSM.SINGLE_LINE` is `'7'`). Written out and cast rather than imported as
 *  a value, because importing the enum for two strings would pull the whole
 *  module into the entry chunk and undo the lazy load below.
 *  `cardframe.test.ts` has no way to catch a drift here; the type does. */
const SINGLE_LINE = '7' as PSM
const SINGLE_BLOCK = '6' as PSM

/** Upscale factor for a crop before it is read.
 *
 * A collector number is about four millimetres tall, which even on a good
 * phone camera lands as a handful of pixels per glyph — below what the
 * engine can resolve. Enlarging before recognition costs milliseconds and
 * is the difference between a read and a blank.
 */
const MAGNIFY = 3

let pending: Promise<Worker> | null = null

/** The one worker, created on first use and shared.
 *
 * Held at module scope rather than in component state because a capture is
 * a rapid sequence — a 99 goes past at several cards a minute — and paying
 * the load once per card would make the door unusable.
 */
async function engine(): Promise<Worker> {
  if (!pending) {
    pending = (async () => {
      const { createWorker } = await import('tesseract.js')
      return createWorker('eng', 1, {
        // Every path is ours. The defaults point at a CDN, which would put
        // a third party in the request path of a feature whose entire
        // privacy claim is that the photograph never leaves the browser.
        workerPath: '/api/ocr/worker.min.js',
        corePath: '/api/ocr/tesseract-core-simd-lstm.wasm.js',
        langPath: '/api/ocr',
        gzip: true,
      })
    })().catch((error: unknown) => {
      // A failed load must not be remembered, or one flaky fetch disables
      // the camera for the rest of the session.
      pending = null
      throw error
    })
  }
  return pending
}

/** Warm the engine up. Called when the viewfinder opens so the six
 *  megabytes arrive while somebody is still lining up their first card. */
export function warm(): void {
  void engine().catch(() => { /* reported when a capture actually needs it */ })
}

/** Let go of the engine and its memory. */
export async function rest(): Promise<void> {
  const worker = pending
  pending = null
  if (worker) await (await worker).terminate()
}

/** Cut one region out of a frame, enlarged, on its own canvas. */
function cut(frame: CanvasImageSource, region: Region,
             width: number, height: number): HTMLCanvasElement {
  const box = pixels(region, width, height)
  const canvas = document.createElement('canvas')
  canvas.width = box.width * MAGNIFY
  canvas.height = box.height * MAGNIFY
  const context = canvas.getContext('2d')
  if (context) {
    context.imageSmoothingQuality = 'high'
    context.drawImage(frame, box.left, box.top, box.width, box.height,
                      0, 0, canvas.width, canvas.height)
  }
  return canvas
}

/**
 * Read one captured card.
 *
 * Both regions are always read, even when the corner succeeds. It costs a
 * second and it buys the case that matters: a corner that reads *plausibly
 * but wrongly* is the one failure this whole design fears, and having the
 * title beside it is what lets a person see the two disagree.
 */
export async function read(frame: CanvasImageSource,
                           width: number, height: number): Promise<Sighting> {
  const worker = await engine()

  await worker.setParameters({
    tessedit_pageseg_mode: SINGLE_BLOCK,
    tessedit_char_whitelist: CORNER_ALPHABET,
  })
  const corner =
    (await worker.recognize(cut(frame, CORNER, width, height))).data.text

  await worker.setParameters({
    tessedit_pageseg_mode: SINGLE_LINE,
    // Cleared, not narrowed: a card name is ordinary words, and the corner's
    // alphabet would turn every lower-case letter into a capital.
    tessedit_char_whitelist: '',
  })
  const title = cleanTitle(
    (await worker.recognize(cut(frame, TITLE, width, height))).data.text)

  return {
    ...(corner.trim() ? { corner } : {}),
    ...(title ? { title } : {}),
  }
}
