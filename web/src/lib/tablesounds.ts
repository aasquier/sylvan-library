/**
 * Card-table sounds, synthesised.
 *
 * Web Audio, not audio files: the repo ships no recordings, pays no licence
 * questions and adds nothing to the bundle — a riffle is a run of short
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
 *
 * Two lessons from the first version, which shipped and was never heard:
 *
 * - **Level is a correctness property.** The old bursts peaked at −20 dBFS
 *   for thirty milliseconds — measured offline, below a laptop's own fan.
 *   Every voice now runs through a master gain into a compressor, and the
 *   bursts are shaped with real envelopes at levels a speaker can say.
 * - **The toggle proves itself.** `wake()` runs inside the toggle's own
 *   click: it constructs and resumes the context in a guaranteed gesture
 *   (Safari refuses a context born in a `setTimeout`, and the deal lands in
 *   one) and answers with two soft taps, so "on" is something you hear
 *   rather than a label you take on faith.
 */

const KEY = 'mtglab-table-sound'

let ctx: AudioContext | null = null
let master: GainNode | null = null

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
  if (!ctx) {
    ctx = new AudioContext()
    // Master bus: one place to set the room's volume, and a compressor so
    // fourteen overlapping riffle snaps sum to "cards" rather than clipping.
    master = ctx.createGain()
    master.gain.value = 0.9
    const glue = ctx.createDynamicsCompressor()
    glue.threshold.value = -18
    glue.knee.value = 12
    glue.ratio.value = 5
    glue.attack.value = 0.002
    glue.release.value = 0.12
    master.connect(glue)
    glue.connect(ctx.destination)
  }
  if (ctx.state === 'suspended') void ctx.resume()
  return ctx
}

/**
 * Run a voice against a context that is actually producing sound.
 *
 * A suspended context accepts every schedule and plays none of it — the
 * failure mode that is silent by construction. Inside a gesture `resume()`
 * settles fast; scheduled after it, the sound still lands within the beat
 * it was meant for.
 */
function play(voice: (c: AudioContext) => void): void {
  const c = context()
  if (!c) return
  if (c.state === 'running') {
    voice(c)
    return
  }
  c.resume().then(() => {
    if (c.state === 'running') voice(c)
  }).catch(() => { /* the policy said no; stay silent rather than throw */ })
}

/** One burst of band-passed noise with an exponential decay — the sound of
 *  card stock moving. `gain` here is a real level, pre-compressor. */
function burst(c: AudioContext, at: number, dur: number,
               freq: number, gain: number, q = 0.9): void {
  if (!master) return
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
  filter.Q.value = q
  const g = c.createGain()
  g.gain.setValueAtTime(gain, at)
  g.gain.exponentialRampToValueAtTime(Math.max(gain * 0.02, 0.001), at + dur)
  source.connect(filter)
  filter.connect(g)
  g.connect(master)
  source.start(at)
}

/** The felt answering: a soft low thump, pitch falling as the energy goes. */
function thump(c: AudioContext, at: number, from: number, to: number,
               dur: number, gain: number): void {
  if (!master) return
  const osc = c.createOscillator()
  osc.type = 'sine'
  osc.frequency.setValueAtTime(from, at)
  osc.frequency.exponentialRampToValueAtTime(to, at + dur)
  const g = c.createGain()
  g.gain.setValueAtTime(gain, at)
  g.gain.exponentialRampToValueAtTime(0.001, at + dur)
  osc.connect(g)
  g.connect(master)
  osc.start(at)
  osc.stop(at + dur)
}

/** The shuffle: a run of quick snaps that accelerates like thumbs letting a
 *  deck go, then the square-up tap on the table. */
export function riffle(): void {
  play((c) => {
    const t = c.currentTime
    let at = t
    for (let i = 0; i < 16; i++) {
      // A real riffle speeds up as the thumbs run out of cards.
      at += 0.055 - i * 0.0022 + Math.random() * 0.01
      burst(c, at, 0.035, 1800 + Math.random() * 1400, 0.5)
      burst(c, at + 0.004, 0.05, 350 + Math.random() * 150, 0.22)
    }
    // Squaring the deck: two taps, wood under felt.
    thump(c, at + 0.22, 200, 90, 0.09, 0.5)
    thump(c, at + 0.34, 180, 80, 0.1, 0.6)
  })
}

/** A card leaving the deck and hitting felt. */
export function deal(): void {
  play((c) => {
    const t = c.currentTime
    burst(c, t, 0.07, 1600, 0.55)
    burst(c, t + 0.01, 0.09, 500, 0.35)
    thump(c, t + 0.06, 160, 70, 0.09, 0.5)
  })
}

/** A card turned over: a swish, then the soft settle. */
export function flip(): void {
  play((c) => {
    const t = c.currentTime
    burst(c, t, 0.09, 2200, 0.5, 0.6)
    burst(c, t + 0.07, 0.06, 900, 0.4)
    thump(c, t + 0.12, 150, 75, 0.08, 0.35)
  })
}

/** The crystal ball noticing: three soft partials of the same chord, lit one
 *  after another, with a breath of air over the top. Quiet on purpose — it
 *  plays under `flip`, not instead of it. */
export function shimmer(): void {
  play((c) => {
    if (!master) return
    const t = c.currentTime
    const partials: [number, number, number][] =
      [[660, 0.10, 0.05], [990, 0.07, 0.16], [1320, 0.05, 0.3]]
    for (const [freq, gain, offset] of partials) {
      const osc = c.createOscillator()
      osc.type = 'sine'
      osc.frequency.value = freq
      const g = c.createGain()
      g.gain.setValueAtTime(0.0001, t + offset)
      g.gain.exponentialRampToValueAtTime(gain, t + offset + 0.09)
      g.gain.exponentialRampToValueAtTime(0.001, t + offset + 1.1)
      osc.connect(g)
      g.connect(master)
      osc.start(t + offset)
      osc.stop(t + offset + 1.2)
    }
    burst(c, t + 0.05, 0.5, 5200, 0.05, 0.4)
  })
}

/**
 * The Wheel of Fortune's own curve, evaluated rather than approximated: the
 * CSS spins on `cubic-bezier(0.12, 0.55, 0.08, 1)` (`.wheel-disc` in
 * index.css), and a ratchet clicking on any other curve would drift audibly
 * out of step with the planks by the second turn. Given a fraction of the
 * rotation completed, this answers the fraction of the duration elapsed —
 * the inverse of what a timing function usually does, because the clicks
 * are placed in angle and scheduled in time.
 */
function wheelEase(rotationFrac: number): number {
  const y = (s: number) =>
    3 * (1 - s) * (1 - s) * s * 0.55 + 3 * (1 - s) * s * s * 1 + s ** 3
  const x = (s: number) =>
    3 * (1 - s) * (1 - s) * s * 0.12 + 3 * (1 - s) * s * s * 0.08 + s ** 3
  let lo = 0
  let hi = 1
  for (let i = 0; i < 24; i++) {
    const mid = (lo + hi) / 2
    if (y(mid) < rotationFrac) lo = mid
    else hi = mid
  }
  return x((lo + hi) / 2)
}

/**
 * The wheel turning: a pawl ratcheting over the studs, one click per 30° of
 * rotation, spaced on the same deceleration the CSS runs — a blur of clicks
 * at the start, then slower, softer knocks as the last fate crawls under
 * the marker, and the pawl's final settle once the wheel has answered.
 * The pitch wanders arithmetically, not randomly, in the house style.
 */
export function wheelTurn(degrees: number, durMs: number): void {
  const total = Math.abs(degrees)
  if (total < 30) return
  play((c) => {
    const t0 = c.currentTime
    const dur = durMs / 1000
    const clicks = Math.floor(total / 30)
    for (let k = 1; k <= clicks; k++) {
      const at = t0 + wheelEase((k * 30) / total) * dur
      // Wood on wood, easing off as the wheel does.
      const level = 0.42 - 0.2 * (k / clicks)
      burst(c, at, 0.022, 1900 + ((k * 37) % 5) * 110, level, 1.6)
      burst(c, at + 0.004, 0.045, 380 + ((k * 13) % 3) * 40, level * 0.55)
    }
    // The pawl drops into the last notch: the wheel has answered.
    thump(c, t0 + dur + 0.04, 170, 72, 0.12, 0.55)
  })
}

/**
 * Called from the toggle's own click when sound turns on. Constructs and
 * resumes the context inside a guaranteed user gesture — after this, a deal
 * scheduled from a timer finds a context already running — and answers with
 * two soft taps so the switch is heard flipping.
 */
export function wake(): void {
  play((c) => {
    const t = c.currentTime
    burst(c, t, 0.05, 1500, 0.4)
    thump(c, t + 0.09, 190, 85, 0.09, 0.5)
  })
}
