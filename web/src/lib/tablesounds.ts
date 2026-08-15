/**
 * Card-table sounds, synthesised.
 *
 * Web Audio, not audio files: the repo ships no recordings, pays no licence
 * questions and adds nothing to the bundle — a riffle is fourteen short
 * bursts of band-passed noise, which is very nearly what a riffle *is*.
 *
 * Two rules, both structural:
 *
 * - **Off by default, and nothing exists until it is on.** No `AudioContext`
 *   is constructed until a play function runs with sound enabled, so the
 *   default experience allocates nothing and asks the browser for nothing.
 *   The toggle is a click, which also satisfies the autoplay policy — the
 *   context is first created inside a user gesture.
 * - **The preference is its own key**, not a field on the tarot table's
 *   stash: it is a preference about this person, like `mtglab-theme` and
 *   `mtglab-stance`, and it should survive "different reader" resetting the
 *   table.
 */

const KEY = 'mtglab-table-sound'

let ctx: AudioContext | null = null

export function soundOn(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}

export function setSound(on: boolean): void {
  try {
    if (on) localStorage.setItem(KEY, '1')
    else localStorage.removeItem(KEY)
  } catch { /* private browsing: the toggle just does not persist */ }
}

function context(): AudioContext | null {
  if (!soundOn()) return null
  if (typeof AudioContext === 'undefined') return null
  ctx ??= new AudioContext()
  if (ctx.state === 'suspended') void ctx.resume()
  return ctx
}

/** One short burst of band-passed noise — the sound of card stock moving. */
function burst(c: AudioContext, at: number, dur: number,
               freq: number, gain: number): void {
  const length = Math.max(1, Math.ceil(c.sampleRate * dur))
  const buffer = c.createBuffer(1, length, c.sampleRate)
  const data = buffer.getChannelData(0)
  for (let i = 0; i < length; i++) {
    data[i] = (Math.random() * 2 - 1) * (1 - i / length)
  }
  const source = c.createBufferSource()
  source.buffer = buffer
  const filter = c.createBiquadFilter()
  filter.type = 'bandpass'
  filter.frequency.value = freq
  filter.Q.value = 0.8
  const g = c.createGain()
  g.gain.value = gain
  source.connect(filter)
  filter.connect(g)
  g.connect(c.destination)
  source.start(at)
}

/** The shuffle: a run of quick snaps, slightly irregular, like thumbs. */
export function riffle(): void {
  const c = context()
  if (!c) return
  const t = c.currentTime
  for (let i = 0; i < 14; i++) {
    burst(c, t + i * 0.045 + Math.random() * 0.012, 0.03,
          2000 + Math.random() * 900, 0.1)
  }
}

/** A card leaving the deck and hitting felt. */
export function deal(): void {
  const c = context()
  if (!c) return
  const t = c.currentTime
  burst(c, t, 0.06, 1500, 0.16)
  burst(c, t + 0.07, 0.04, 500, 0.1)
}

/** A card turned over: a swish, then the soft settle. */
export function flip(): void {
  const c = context()
  if (!c) return
  const t = c.currentTime
  burst(c, t, 0.05, 1900, 0.18)
  burst(c, t + 0.06, 0.035, 800, 0.12)
}
