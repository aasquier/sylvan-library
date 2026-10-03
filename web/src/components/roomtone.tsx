/**
 * A room's own sound, behind the table-sound switch.
 *
 * `lib/tablesounds.ts` synthesises the table's clicks and riffles and argues
 * why it ships no recordings; this is the one exception and it is argued
 * here. A room that is footage has a sound the footage came with — rain on
 * the tavern window, the fire's crackle — and a bed the render made is as
 * much Aaron's as the picture is, so the licence question the synth avoids
 * does not arise. What it costs is bytes, which is why each bed is a short
 * AAC loop rather than the clip's whole track.
 *
 * **Through the synth's graph first, the element second.** An `<audio loop>`
 * cannot loop AAC without a gap — the codec's own padding plays at every
 * turn, and a three-second simmer stuttered on Aaron's seventh walk — so the
 * bed is a decoded buffer looped sample-accurately by `tablesounds.bedStart`,
 * and the element below is only what plays where there is no Web Audio at
 * all (and in jsdom, which is why the tests still see it).
 *
 * Two rules, both the synth's own:
 *
 * - **Off unless the switch is on**, read live: flipping the switch while
 *   the room is open starts or stops the bed without a reload.
 * - **The browser decides when sound may start, and the room does not
 *   argue.** `play()` is refused until the page has had a gesture; the first
 *   refusal arms a one-shot listener and the next click or key plays the bed.
 *   No error reaches anybody, because a silent room is not a broken one.
 *
 * Pauses on unmount — the bed belongs to the room, not the site — and the
 * element is `preload="none"` so a room with the switch off fetches nothing.
 */

import { useEffect, useRef } from 'react'
import { useTableSound } from '../lib/prefs'
import { bedStart, bedStop } from '../lib/tablesounds'

/** `play()` returns a promise in browsers and nothing in jsdom; both are
 *  handled, and the refusal is the only outcome anybody acts on. */
function attempt(el: HTMLAudioElement, onRefused: () => void): void {
  let p: Promise<void> | undefined
  try {
    p = el.play() as Promise<void> | undefined
  } catch {
    onRefused()
    return
  }
  if (p && typeof p.catch === 'function') p.catch(onRefused)
}

export function RoomTone({ src, volume = 1 }: { src: string; volume?: number }) {
  const [on] = useTableSound()
  const ref = useRef<HTMLAudioElement>(null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    if (!on) {
      bedStop()
      try { el.pause() } catch { /* jsdom */ }
      return
    }
    // The graph, when there is one: gapless, and its own gesture handling.
    if (bedStart(src, volume)) {
      return () => bedStop()
    }
    el.volume = volume
    let armed = false
    const disarm = () => {
      window.removeEventListener('pointerdown', onGesture)
      window.removeEventListener('keydown', onGesture)
    }
    const onGesture = () => {
      disarm()
      attempt(el, () => undefined)
    }
    attempt(el, () => {
      if (armed) return
      armed = true
      window.addEventListener('pointerdown', onGesture)
      window.addEventListener('keydown', onGesture)
    })
    return () => {
      disarm()
      try { el.pause() } catch { /* jsdom */ }
    }
  }, [on, volume, src])

  return <audio ref={ref} src={src} loop preload="none" aria-hidden="true" />
}
