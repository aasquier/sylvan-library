/**
 * The shelves, fetched once per page load — `lib/glossary.ts`'s shape, for
 * the same reasons: the facts are reference data that cannot go stale within
 * a session, and more than one screen may someday walk past the shelf.
 */

import { useEffect, useState } from 'react'
import { api, type LoreShelves } from './api'

let pending: Promise<LoreShelves> | null = null
let loaded: LoreShelves | null = null

/** The whole shelf, or `null` until it arrives. Failure renders nothing —
 *  trivia is never worth an error state. */
export function useLore(): LoreShelves | null {
  const [value, setValue] = useState<LoreShelves | null>(loaded)
  useEffect(() => {
    if (loaded) return
    let live = true
    try {
      pending ??= api.lore()
      pending
        .then((l) => {
          loaded = l
          if (live) setValue(l)
        })
        .catch(() => { pending = null })
    } catch {
      // An api mock without the method, or a throw before the promise —
      // same answer as a rejection: no shelf today.
    }
    return () => { live = false }
  }, [])
  return value
}

/** Test seam: drop the module-level cache between cases. */
export function resetLoreCache(): void {
  pending = null
  loaded = null
}
