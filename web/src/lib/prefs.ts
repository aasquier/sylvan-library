/**
 * The person-preferences that are not the stance: ambience and table sound,
 * one store, every reader subscribed (second punch list of 2026-08-15,
 * item 9).
 *
 * The shape is `lib/stance.ts`'s, and for the same reason: the readers are
 * not in one tree. The ambience switch lives in the settings gear while
 * `ForestAmbience` and `SceneBackdrop` render at opposite ends of the shell,
 * so a `useState` copy in each would leave the weather running until
 * something happened to remount it. The value lives outside React; every
 * hook subscribes to the same store; `storage` events keep a second tab
 * honest.
 *
 * **Ambience defaults on, and "on" is the absence of the key** — the app's
 * character is the default and the preference is the opt-out, mirroring how
 * `mtglab-stance` stores only a deviation. Table sound is the opposite
 * (`lib/tablesounds.ts` stores only "1"), and that asymmetry is each
 * feature's right default: a silent page is the one that never surprises
 * anybody at work; an empty sky is just less.
 */

import { useCallback, useSyncExternalStore } from 'react'
import { setSound, soundOn, wake } from './tablesounds'

const AMBIENCE_KEY = 'mtglab-ambience'
const SOUND_KEY = 'mtglab-table-sound'

const listeners = new Set<() => void>()

function notify(): void {
  listeners.forEach((l) => l())
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  // Fired by *other* tabs only — the case a single-tab store silently
  // gets wrong.
  const onStorage = (e: StorageEvent) => {
    if (e.key === AMBIENCE_KEY || e.key === SOUND_KEY) notify()
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(fn)
    window.removeEventListener('storage', onStorage)
  }
}

export function ambienceOn(): boolean {
  try {
    return localStorage.getItem(AMBIENCE_KEY) !== '0'
  } catch {
    // Private browsing: no preference survives, so the default holds.
    return true
  }
}

/** The ambience switch: fireflies, leaves, pages, and the rooms. */
export function useAmbience(): [boolean, (on: boolean) => void] {
  const on = useSyncExternalStore(subscribe, ambienceOn, () => true)
  const set = useCallback((next: boolean) => {
    try {
      if (next) localStorage.removeItem(AMBIENCE_KEY)
      else localStorage.setItem(AMBIENCE_KEY, '0')
    } catch { /* the toggle just does not persist */ }
    notify()
  }, [])
  return [on, set]
}

/**
 * The table-sound switch, wrapping `lib/tablesounds.ts`'s own key so the
 * gear and the tarot table flip one preference. Turning it on `wake()`s
 * inside the caller's click — the same guaranteed-gesture contract the
 * table's toggle has always honoured, and the reason "on" is something you
 * hear rather than a label you take on faith.
 */
export function useTableSound(): [boolean, (on: boolean) => void] {
  const on = useSyncExternalStore(subscribe, soundOn, () => false)
  const set = useCallback((next: boolean) => {
    setSound(next)
    if (next) wake()
    notify()
  }, [])
  return [on, set]
}
