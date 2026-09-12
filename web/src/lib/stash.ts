/**
 * State that survives a reload, because losing it would cost somebody their
 * evening.
 *
 * Two screens keep one: the interview holds ten minutes of thinking
 * (`components/theme.tsx`, and the server stores none of it on purpose — ADR
 * 20), and the table holds which three cards are out and which have been
 * turned (`components/tarot.tsx`, three integers, because the seed re-deals
 * the rest). Both wrote the same three lines, and the second room to be built
 * would have written them a third time.
 *
 * Two details worth keeping rather than rediscovering:
 *
 * **The load is lazy and runs once.** It reads `localStorage` in a state
 * initialiser, so a stash is parsed on mount and never again — which is what
 * lets the interview's loader compare the stash against the persona and seed
 * it was mounted with and discard somebody else's conversation.
 *
 * **The write is an effect, not part of the setter.** It follows the render
 * rather than leading it, which is the one thing the table's test had to learn
 * the hard way (#95, the suite's only flake): reading `localStorage` the
 * instant a name appears on screen reads it *before* this has run. Wait for
 * the observable, however cheap the render looks.
 *
 * There is deliberately no `try`/`catch` around the write. Private browsing
 * throws here, and it threw here before this hook existed; swallowing it now
 * would be a behaviour change smuggled in under a refactor.
 */

import { useEffect, useState, type Dispatch, type SetStateAction } from 'react'

export function useStashed<T>(
  key: string, load: () => T,
): [T, Dispatch<SetStateAction<T>>] {
  const [value, setValue] = useState<T>(load)
  useEffect(() => {
    localStorage.setItem(key, JSON.stringify(value))
  }, [key, value])
  return [value, setValue]
}
