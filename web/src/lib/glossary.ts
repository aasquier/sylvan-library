/**
 * The vocabulary, fetched once per page load.
 *
 * Separate from `components/term.tsx` because that file exports components and
 * this one exports hooks, which is the split the lint rule wants and the split
 * the rest of `lib/` already follows.
 *
 * **Memoised at module scope rather than lifted into a provider.** A tooltip
 * may be asked for from any screen and several times on one of them, so the
 * natural shape is one shared promise rather than a context threaded through
 * `App` for 12 kB of fixed prose. The table is reference data — it cannot go
 * stale within a session, so there is nothing for a cache to get wrong.
 */

import { useEffect, useState } from 'react'
import { api, type Glossary, type Term } from './api'

let pending: Promise<Glossary> | null = null
let loaded: Glossary | null = null

/** The whole glossary, or `null` until it arrives. */
export function useGlossary(): Glossary | null {
  const [value, setValue] = useState<Glossary | null>(loaded)
  useEffect(() => {
    if (loaded) return
    let live = true
    pending ??= api.glossary()
    pending
      .then((g) => {
        loaded = g
        if (live) setValue(g)
      })
      // A missing glossary costs tooltips and nothing else, so it is not worth
      // an error state on a screen that is about something else. Clearing the
      // promise lets the next mount retry.
      .catch(() => { pending = null })
    return () => { live = false }
  }, [])
  return value
}

/** One entry by key: `null` while loading, and `null` when there is no such
 *  term — which is what lets a word be marked up before its entry exists. */
export function useTerm(key: string): Term | null {
  const glossary = useGlossary()
  return glossary?.terms.find((t) => t.key === key) ?? null
}

/** Test seam: drop the module-level cache between cases. */
export function resetGlossaryCache(): void {
  pending = null
  loaded = null
}
