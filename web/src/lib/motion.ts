/**
 * The motion tier's client plumbing (ADR 32): the status hook and the
 * reduced-motion read. In `lib/` beside `prefs.ts` rather than in the
 * component files, because oxlint's fast-refresh rule is right — these are
 * not components, and two components need each of them.
 */

import { useEffect, useState } from 'react'

export function reducedMotion(): boolean {
  // `?.` because jsdom has no matchMedia at all.
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false
}

export interface MotionStatus {
  ready: boolean
  effect: string
  fingerprint?: string
  urls?: { webm?: string; mp4?: string; poster?: string; depth?: string }
}

/**
 * One status pass, no polling: derivatives are made by a dev-machine run
 * and pushed to the instance, never generated on request, so the answer
 * cannot change while the page is open. `effects` is a preference ladder --
 * the first ready one wins, so a card with only the no-model `slow-pan`
 * still moves when `depth-drift` was never built for it. Any failure at
 * all reads as "no motion", because the decoration must never be the
 * reason a page looks broken.
 */
export function useCardMotion(oracleId: string | null | undefined,
                              effects: readonly string[],
                              art?: string | null): MotionStatus | null {
  const [status, setStatus] = useState<MotionStatus | null>(null)
  const ladder = effects.join(',')

  useEffect(() => {
    if (!oracleId) return
    let cancelled = false
    // Which painting this page is showing. The server matches derivatives
    // against it, so a deck that picked a printing gets that printing's
    // loop or the still -- never the default painting breathing over a
    // swapped one.
    const chosen = art ? `?art=${encodeURIComponent(art)}` : ''
    void (async () => {
      for (const effect of ladder.split(',')) {
        try {
          const resp = await fetch(
            `/api/art/motion/${oracleId}/${effect}${chosen}`)
          if (!resp.ok) continue
          const body = (await resp.json()) as MotionStatus
          if (body.ready) {
            if (!cancelled) setStatus(body)
            return
          }
        } catch {
          return // the instance is unreachable; asking again would be noise
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [oracleId, ladder, art])

  return status
}

/**
 * The window of texture the shader may sample: `object-fit: cover;
 * object-position: center top`, in GL texture coordinates (v = 1 is the
 * painting's top — textures are uploaded with UNPACK_FLIP_Y_WEBGL, so
 * image rows and GL's v axis agree).
 *
 * A band wider than the painting shows the full width and the TOP slice
 * of the height — the hero band's contract, argued at its call site: the
 * bottom of card art is ground and robes, the top is the subject's head.
 * A box taller than the painting shows full height, centred horizontally.
 */
export function coverTopWindow(
  texWidth: number, texHeight: number,
  boxWidth: number, boxHeight: number,
): { scale: [number, number]; offset: [number, number] } {
  const texAspect = texWidth / texHeight
  const boxAspect = boxWidth / boxHeight
  if (boxAspect >= texAspect) {
    const sliceHeight = texAspect / boxAspect
    return { scale: [1, sliceHeight], offset: [0, 1 - sliceHeight] }
  }
  const sliceWidth = boxAspect / texAspect
  return { scale: [sliceWidth, 1], offset: [(1 - sliceWidth) / 2, 0] }
}
