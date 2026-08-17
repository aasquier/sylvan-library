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
                              effects: readonly string[]): MotionStatus | null {
  const [status, setStatus] = useState<MotionStatus | null>(null)
  const ladder = effects.join(',')

  useEffect(() => {
    if (!oracleId) return
    let cancelled = false
    void (async () => {
      for (const effect of ladder.split(',')) {
        try {
          const resp = await fetch(`/api/art/motion/${oracleId}/${effect}`)
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
  }, [oracleId, ladder])

  return status
}
