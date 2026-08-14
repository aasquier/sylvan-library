/**
 * The stance, client side: one pin, held in the browser.
 *
 * [ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)
 * splits a Claude surface into a **mode** and a **stance**, and calls the
 * stance "the user's dial over it". The dial is this. Everything it does is
 * decide what to put in one optional `stance` field on four requests; the
 * server owns every other part of the decision, which is the property worth
 * protecting.
 *
 * **`null` is a position, not an absence.** It means *let the surface use its
 * own default*, and that default is not one value — `stance.default_for` reads
 * `status: built | theoretical` off the deck, so Goreclaw and Tivit open wider
 * than Arahbo and Gyome without anybody configuring it, and `theme._stance`
 * defaults to `SECOND_OPINION` because a deck that does not exist yet is as
 * theoretical as a deck gets. A dial whose lowest position was `off` rather
 * than "follow the deck" would throw all of that away the first time somebody
 * touched it, and there would be no way back: "I have not chosen" is not a
 * preset and cannot be spelled as one.
 *
 * **Nothing here computes what applies.** The pin is a *request*; the server
 * resolves and clamps it, and `/api/claude`'s `stance` field is the answer.
 * Re-deriving the clamp here would be a second implementation of
 * `stance.clamp` that could disagree with the one that matters, and it would
 * disagree silently — the UI would say `collaborator` while the instance ran
 * `consultant`. So the dial sends the pin and renders what comes back.
 */

import { useCallback, useSyncExternalStore } from 'react'
import { ApiError, api, type ClaudeStatus } from './api'

/** Mirrors `mtglab-theme`, which is the only other preference this app keeps. */
const KEY = 'mtglab-stance'

/** A pinned preset name, or `null` for "whatever the surface would do anyway". */
export type StancePin = string | null

function read(): StancePin {
  try {
    return localStorage.getItem(KEY) || null
  } catch {
    // Private browsing, or storage disabled. An unavailable preference is not
    // an error worth surfacing — it just means every session starts unpinned.
    return null
  }
}

// ------------------------------------------------------------- the store
//
// `useState` in the hook would give every caller its *own* copy, and the
// callers are not in one tree: the dial renders on the deck page, and
// `deckedit`'s interview panel reads the pin from inside a modal several
// levels away. Change the dial with `useState` and the interview keeps the
// old pin until something happens to remount it — a stale preference that
// looks exactly like a preference that was never set.
//
// So the pin lives outside React and every caller subscribes to the same
// value. `storage` is in the listener set too, which makes a second tab
// correct for free rather than as an extra feature.

let current: StancePin = read()
const listeners = new Set<() => void>()

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  // Fired by *other* tabs only, which is exactly the case a single-tab
  // implementation gets wrong and never notices.
  const onStorage = (e: StorageEvent) => {
    if (e.key !== KEY) return
    current = read()
    listeners.forEach((l) => l())
  }
  window.addEventListener('storage', onStorage)
  return () => {
    listeners.delete(fn)
    window.removeEventListener('storage', onStorage)
  }
}

/** The pin, and a setter that persists it and tells every other reader. */
export function useStance(): [StancePin, (pin: StancePin) => void] {
  const pin = useSyncExternalStore(subscribe, () => current, () => null)

  const set = useCallback((next: StancePin) => {
    current = next
    try {
      if (next === null) localStorage.removeItem(KEY)
      else localStorage.setItem(KEY, next)
    } catch { /* see `read` */ }
    listeners.forEach((l) => l())
  }, [])

  return [pin, set]
}

/**
 * The pin, if this deployment still recognises it.
 *
 * A stored preset name outlives the roster that produced it, and the two ways
 * it can go stale fail very differently:
 *
 * * **Renamed or removed.** `Stance.from_obj` raises on an unknown preset and
 *   the route answers 422, so a name this build no longer serves would break
 *   *every* Claude call until somebody cleared their browser storage. Dropping
 *   it to `null` costs a preference and keeps the feature.
 * * **Capped by the operator.** `MTGLAB_CLAUDE_STANCE_CEILING` can narrow at
 *   any time and the server clamps rather than refusing, so a pin above the
 *   ceiling is not an error — it is a request that quietly gets less than it
 *   asked for. That one is kept and sent; `presets[].available` is how the
 *   dial says so out loud, which is what that field is for.
 *
 * Returns `undefined` rather than `null` when there is nothing to send, so it
 * drops straight into a request body without a conditional at each call site.
 */
export function effectivePin(pin: StancePin, status: ClaudeStatus | null): string | undefined {
  if (pin === null) return undefined
  if (!status) return pin           // Nothing to check against; let the server rule.
  return status.presets.some((p) => p.name === pin) ? pin : undefined
}

/** Whether a pin names a preset this deployment will honour unclamped. */
export function isCapped(pin: StancePin, status: ClaudeStatus | null): boolean {
  if (pin === null || !status) return false
  const preset = status.presets.find((p) => p.name === pin)
  return preset ? !preset.available : false
}

/**
 * `/api/claude`, asked with the pin, and healing itself if the pin is refused.
 *
 * `effectivePin` cannot save this call, and that is the whole reason this
 * function exists: it validates against the roster, and **this is the request
 * that fetches the roster.** A preset renamed between builds therefore 422s
 * here first, before anything has a list to check it against.
 *
 * Left alone the failure is permanent and silent in the worst way. The status
 * fetch is what every Claude panel gates on, so a stored name this build no
 * longer serves does not produce an error message — it makes the dial and both
 * panels *disappear*, including the dial that is the only control able to
 * clear the pin. The user is locked out of the feature by a preference they
 * cannot see, and the fix is to know to clear browser storage.
 *
 * So a 422 is read as what it is — the server rejecting the pin — and the pin
 * is dropped and the call retried bare. One wasted request, once, on the build
 * that renames a preset; the alternative is a dead feature.
 */
export async function fetchClaudeStatus(
  params: { slug?: string; owner?: string; surface?: string },
  pin: StancePin,
  clearPin: () => void,
): Promise<ClaudeStatus> {
  try {
    return await api.claudeStatus({ ...params, stance: pin ?? undefined })
  } catch (e) {
    // Only a 422, and only when a pin was actually sent. Anything else is a
    // real failure and belongs to the caller: swallowing a 500 here would
    // turn "the instance is broken" into "you have no Claude surface".
    if (pin === null || !(e instanceof ApiError) || e.status !== 422) throw e
    clearPin()
    return api.claudeStatus(params)
  }
}
