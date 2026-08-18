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

/** A single cricket chirp: a short train of pulses on one high carrier —
 *  which is very nearly what a chirp is, a wing scraped a few times fast.
 *  Each cricket keeps its own carrier so the chorus has voices. */
function chirp(c: AudioContext, at: number, carrier: number,
               pulses: number, gain: number): void {
  if (!master) return
  for (let i = 0; i < pulses; i++) {
    const t = at + i * 0.055
    const osc = c.createOscillator()
    osc.type = 'sine'
    osc.frequency.value = carrier
    const g = c.createGain()
    g.gain.setValueAtTime(0.0001, t)
    g.gain.exponentialRampToValueAtTime(gain, t + 0.008)
    g.gain.exponentialRampToValueAtTime(0.0001, t + 0.042)
    osc.connect(g)
    g.connect(master)
    osc.start(t)
    osc.stop(t + 0.05)
  }
}

/** A frog's croak: a low pulse-train growl, rounder and rarer than the
 *  crickets it answers. */
function croak(c: AudioContext, at: number, gain: number): void {
  if (!master) return
  const osc = c.createOscillator()
  osc.type = 'sawtooth'
  osc.frequency.setValueAtTime(85, at)
  osc.frequency.linearRampToValueAtTime(70, at + 0.18)
  const trem = c.createOscillator()
  trem.frequency.value = 22
  const tremGain = c.createGain()
  tremGain.gain.value = 0.5
  const g = c.createGain()
  g.gain.setValueAtTime(0.0001, at)
  g.gain.exponentialRampToValueAtTime(gain, at + 0.03)
  g.gain.exponentialRampToValueAtTime(0.0001, at + 0.22)
  const filter = c.createBiquadFilter()
  filter.type = 'lowpass'
  filter.frequency.value = 420
  trem.connect(tremGain)
  tremGain.connect(g.gain)
  osc.connect(filter)
  filter.connect(g)
  g.connect(master)
  osc.start(at)
  osc.stop(at + 0.25)
  trem.start(at)
  trem.stop(at + 0.25)
}

/** The signature call (Aaron: "where is the... whooo, whooo?"): two
 *  long, low, mournful notes, each swelling in, wavering with a slow
 *  vibrato, and dying away — the sound everybody means by "owl". */
function hoot(c: AudioContext, at: number, gain: number): void {
  if (!master) return
  for (const offset of [0, 1.35]) {
    const osc = c.createOscillator()
    osc.type = 'sine'
    osc.frequency.setValueAtTime(315, at + offset)
    osc.frequency.exponentialRampToValueAtTime(282, at + offset + 0.95)
    // The waver: a slow vibrato riding the pitch.
    const vib = c.createOscillator()
    vib.frequency.value = 5.2
    const vibGain = c.createGain()
    vibGain.gain.value = 6
    vib.connect(vibGain)
    vibGain.connect(osc.frequency)
    const g = c.createGain()
    g.gain.setValueAtTime(0.0001, at + offset)
    g.gain.exponentialRampToValueAtTime(gain, at + offset + 0.22)
    g.gain.setValueAtTime(gain, at + offset + 0.55)
    g.gain.exponentialRampToValueAtTime(0.0001, at + offset + 1.0)
    osc.connect(g)
    g.connect(master)
    osc.start(at + offset)
    osc.stop(at + offset + 1.05)
    vib.start(at + offset)
    vib.stop(at + offset + 1.05)
  }
}

/** One drip off the roots into the pool: a fast falling plink and its
 *  fainter echo. */
function drip(c: AudioContext, at: number, gain: number): void {
  if (!master) return
  for (const [offset, share] of [[0, 1], [0.15, 0.35]] as const) {
    const osc = c.createOscillator()
    osc.type = 'sine'
    osc.frequency.setValueAtTime(1250 + Math.random() * 300, at + offset)
    osc.frequency.exponentialRampToValueAtTime(640, at + offset + 0.07)
    const g = c.createGain()
    g.gain.setValueAtTime(gain * share, at + offset)
    g.gain.exponentialRampToValueAtTime(0.0001, at + offset + 0.09)
    osc.connect(g)
    g.connect(master)
    osc.start(at + offset)
    osc.stop(at + offset + 0.1)
  }
}

let swampTimer: number | null = null
let rainSource: AudioBufferSourceNode | null = null
let rainGain: GainNode | null = null
let rainLfo: OscillatorNode | null = null

/** Soft rain on the canopy: a looped noise bed low-passed to a hush,
 *  its level breathing under a slow LFO so it never reads as a fan. */
function rainStart(c: AudioContext): void {
  if (rainSource || !master) return
  const length = c.sampleRate * 2
  const buffer = c.createBuffer(1, length, c.sampleRate)
  const data = buffer.getChannelData(0)
  let last = 0
  for (let i = 0; i < length; i++) {
    // One-pole smoothed noise: pink-ish, which is what rain is.
    last = last * 0.94 + (Math.random() * 2 - 1) * 0.06
    data[i] = last * 6
  }
  rainSource = c.createBufferSource()
  rainSource.buffer = buffer
  rainSource.loop = true
  const filter = c.createBiquadFilter()
  filter.type = 'lowpass'
  filter.frequency.value = 1100
  rainGain = c.createGain()
  rainGain.gain.value = 0.05
  rainLfo = c.createOscillator()
  rainLfo.frequency.value = 0.11
  const lfoGain = c.createGain()
  lfoGain.gain.value = 0.02
  rainLfo.connect(lfoGain)
  lfoGain.connect(rainGain.gain)
  rainSource.connect(filter)
  filter.connect(rainGain)
  rainGain.connect(master)
  rainSource.start()
  rainLfo.start()
}

function rainStop(): void {
  try {
    rainSource?.stop()
    rainLfo?.stop()
  } catch { /* already stopped */ }
  rainSource = null
  rainGain = null
  rainLfo = null
}

/**
 * Thunder after the flash: a long low rumble that arrives late, the way
 * thunder does — the wheel's lightning effect passes the delay it wants,
 * so the sky and the sound stay one storm.
 */
export function thunderRoll(delaySeconds: number): void {
  play((c) => {
    if (!master) return
    const at = c.currentTime + delaySeconds
    // The CRACK first: a hard broadband snap and a low slam, the part
    // of thunder that makes shoulders jump (round three: "the sound
    // needs to be more prominent").
    burst(c, at, 0.1, 1100, 0.5, 0.35)
    burst(c, at + 0.04, 0.3, 420, 0.4, 0.3)
    thump(c, at + 0.05, 130, 42, 0.5, 0.7)
    // Then the long rumble rolling away.
    const dur = 3.4 + Math.random() * 1.4
    const length = Math.ceil(c.sampleRate * dur)
    const buffer = c.createBuffer(1, length, c.sampleRate)
    const data = buffer.getChannelData(0)
    let last = 0
    for (let i = 0; i < length; i++) {
      last = last * 0.985 + (Math.random() * 2 - 1) * 0.015
      // The rumble undulates: two slow beats against each other.
      const t = i / c.sampleRate
      const roll = 0.6 + 0.4 * Math.sin(t * 2.1) * Math.sin(t * 3.7)
      data[i] = last * 24 * roll
    }
    const source = c.createBufferSource()
    source.buffer = buffer
    const filter = c.createBiquadFilter()
    filter.type = 'lowpass'
    filter.frequency.setValueAtTime(190, at)
    filter.frequency.exponentialRampToValueAtTime(85, at + dur)
    const g = c.createGain()
    g.gain.setValueAtTime(0.0001, at + 0.08)
    g.gain.exponentialRampToValueAtTime(0.5, at + 0.4)
    g.gain.exponentialRampToValueAtTime(0.0001, at + dur)
    source.connect(filter)
    filter.connect(g)
    g.connect(master)
    source.start(at + 0.08)
  })
}

/**
 * The clearing's night chorus: soft rain on the canopy, crickets in loose
 * annexes of three or four, drips off the roots, a frog answering every
 * little while, and — rarely — the barn owl on the limb. Scheduled a
 * couple of seconds at a time from a timer rather than as one long
 * buffer, so switching the table sound off falls silent at the next
 * breath — and nothing at all runs, or exists, while the switch is off
 * or the wheel is off the page.
 */
export function swampStart(): void {
  if (swampTimer !== null) return
  const tick = () => {
    // A hidden tab's clearing holds its breath; browsers throttle these
    // timers anyway, and a chirp in a tab nobody is watching is noise.
    // The rain bed stops too — it is the one voice that would otherwise
    // keep playing into a tab nobody is looking at.
    if (document.hidden) {
      rainStop()
      return
    }
    play((c) => {
      rainStart(c)
      const t = c.currentTime
      // Two or three crickets, each with its own carrier and cadence.
      for (let v = 0; v < 3; v++) {
        if (Math.random() < 0.75) {
          const carrier = 3900 + v * 480 + Math.random() * 220
          chirp(c, t + Math.random() * 1.1,
                carrier, 3 + ((Math.random() * 3) | 0), 0.05)
        }
      }
      // Water finds its way down every few seconds.
      if (Math.random() < 0.45) drip(c, t + Math.random() * 1.6, 0.07)
      // The frog is occasional company, not a metronome.
      if (Math.random() < 0.16) croak(c, t + Math.random() * 1.4, 0.1)
      // And every little while, the owl says whooo — front of the mix,
      // because she is the voice of the place.
      if (Math.random() < 0.11) hoot(c, t + Math.random() * 0.8, 0.27)
    })
  }
  tick()
  swampTimer = window.setInterval(tick, 2000)
}

export function swampStop(): void {
  if (swampTimer !== null) {
    window.clearInterval(swampTimer)
    swampTimer = null
  }
  rainStop()
}

/**
 * What each fate sounds like when it lands, played at the reveal alongside
 * its effect: gold rings, the heart beats, the steel strikes and keens,
 * the grave exhales. All synthesised, like every other sound the table
 * makes, and all behind the same switch.
 */
export function fateLand(symbol: string, face?: string): void {
  play((c) => {
    const t = c.currentTime
    if (symbol === 'cup') {
      // One sound and one only (round eight): the pool takes the coin.
      // The toss is silent theatre; the PLOP is the point — a big
      // round bloop as the water swallows it, the splash over it,
      // droplets falling back — timed to the instant the coin drops
      // from the air at the end of its arc.
      if (master) {
        const at = t + 4.5
        // The bloop: a sine that dips and then rises as the cavity
        // closes — the shape of every stone ever dropped in a pond.
        const osc = c.createOscillator()
        osc.type = 'sine'
        osc.frequency.setValueAtTime(400, at)
        osc.frequency.exponentialRampToValueAtTime(160, at + 0.055)
        osc.frequency.exponentialRampToValueAtTime(640, at + 0.19)
        const g = c.createGain()
        g.gain.setValueAtTime(0.62, at)
        g.gain.exponentialRampToValueAtTime(0.0001, at + 0.28)
        osc.connect(g)
        g.connect(master)
        osc.start(at)
        osc.stop(at + 0.3)
        // The splash and the spray.
        burst(c, at + 0.01, 0.18, 2000, 0.45, 0.4)
        burst(c, at + 0.03, 0.08, 4600, 0.25, 0.3)
        thump(c, at + 0.02, 150, 70, 0.12, 0.4)
        // Droplets falling back in.
        drip(c, at + 0.42, 0.14)
        drip(c, at + 0.72, 0.08)
      }
    } else if (symbol === 'heart') {
      if (face === 'broken') {
        // The break: a dry crack, two beats that still try, and a
        // third that gets out its lub and cannot finish.
        burst(c, t + 0.25, 0.05, 620, 0.4, 0.5)
        thump(c, t + 0.26, 180, 60, 0.14, 0.55)
        for (let beat = 0; beat < 2; beat++) {
          const at = t + 0.6 + beat * 1.05
          thump(c, at, 95, 42, 0.16, 0.55)
          thump(c, at + 0.24, 72, 36, 0.13, 0.36)
        }
        thump(c, t + 0.6 + 2 * 1.05, 88, 38, 0.2, 0.45)
      } else {
        // Three bars of lub-dub, the dub softer and lower, as the
        // painted heart on screen pumps on the same period.
        for (let beat = 0; beat < 3; beat++) {
          const at = t + 0.3 + beat * 1.0
          thump(c, at, 95, 42, 0.16, 0.6)
          thump(c, at + 0.22, 75, 38, 0.14, 0.42)
        }
      }
    } else if (symbol === 'sword' && face === 'hilt') {
      // The offer: steel drawn slowly — one soft ring, held out — and
      // a low bow of assent underneath. No crack, no scrape: nothing
      // here is fighting.
      burst(c, t + 0.2, 0.4, 1400, 0.1, 0.3)
      if (master) {
        for (const [freq, share] of
             [[2417, 0.2], [3251, 0.14], [4108, 0.08]] as const) {
          const osc = c.createOscillator()
          osc.type = 'sine'
          osc.frequency.value = freq
          const g = c.createGain()
          g.gain.setValueAtTime(0.0001, t + 0.5)
          g.gain.exponentialRampToValueAtTime(share, t + 0.75)
          g.gain.exponentialRampToValueAtTime(0.0001, t + 2.2)
          osc.connect(g)
          g.connect(master)
          osc.start(t + 0.5)
          osc.stop(t + 2.3)
        }
      }
      thump(c, t + 1.6, 140, 65, 0.14, 0.5)
    } else if (symbol === 'sword') {
      // Two whooshes as the blades come — one each — then the bind:
      // a broadband CRACK, a chest-thump under it, a cluster of
      // inharmonic steel partials rung all at once, and the scrape of
      // edge sliding on edge as they hold. No pure tones anywhere:
      // pure tones are what read as a toy (round six).
      burst(c, t + 0.08, 0.22, 650, 0.18, 0.22)
      burst(c, t + 0.2, 0.2, 800, 0.16, 0.22)
      // The crack and the weight of it.
      burst(c, t + 0.44, 0.012, 2600, 0.85, 0.2)
      burst(c, t + 0.45, 0.06, 4600, 0.5, 0.3)
      thump(c, t + 0.44, 220, 50, 0.34, 0.7)
      // Steel rings at inharmonic partials, struck together, fast decay.
      if (master) {
        for (const [freq, share, dur] of [
          [2417, 0.32, 0.5], [3251, 0.26, 0.42], [4108, 0.2, 0.34],
          [5530, 0.14, 0.26], [6841, 0.09, 0.2],
        ] as const) {
          const osc = c.createOscillator()
          osc.type = 'sine'
          osc.frequency.value = freq * (0.99 + Math.random() * 0.02)
          const g = c.createGain()
          g.gain.setValueAtTime(share, t + 0.45)
          g.gain.exponentialRampToValueAtTime(0.0001, t + 0.45 + dur)
          osc.connect(g)
          g.connect(master)
          osc.start(t + 0.45)
          osc.stop(t + 0.45 + dur + 0.05)
        }
        // The scrape: filtered noise sliding down as the edges grind.
        const dur = 0.55
        const length = Math.ceil(c.sampleRate * dur)
        const buffer = c.createBuffer(1, length, c.sampleRate)
        const data = buffer.getChannelData(0)
        for (let i = 0; i < length; i++) {
          data[i] = (Math.random() * 2 - 1) * (1 - i / length)
        }
        const src = c.createBufferSource()
        src.buffer = buffer
        const filter = c.createBiquadFilter()
        filter.type = 'bandpass'
        filter.Q.value = 6
        filter.frequency.setValueAtTime(2400, t + 0.5)
        filter.frequency.exponentialRampToValueAtTime(600, t + 0.5 + dur)
        const g = c.createGain()
        g.gain.setValueAtTime(0.16, t + 0.5)
        g.gain.exponentialRampToValueAtTime(0.0001, t + 0.5 + dur)
        src.connect(filter)
        filter.connect(g)
        g.connect(master)
        src.start(t + 0.5)
      }
    } else if (symbol === 'skull' && face === 'buried') {
      // The grave takes: earth turned over — three low falls of soil,
      // each duller than the last, and the air pressed out of the
      // ground under them.
      for (const [offset, gain] of
           [[0.3, 0.4], [0.9, 0.3], [1.5, 0.22]] as const) {
        burst(c, t + offset, 0.12, 350, gain, 0.25)
        thump(c, t + offset + 0.01, 110, 45, gain * 0.8, 0.6)
      }
      burst(c, t + 0.5, 1.8, 500, 0.07, 0.2)
    } else if (symbol === 'skull') {
      // The dead rise wailing (round three: "more ghosty"): breath
      // first, then two voices a fourth apart gliding down out of
      // tune with each other, each shivering with its own tremolo —
      // a chord no living throat sings — and long cold air after.
      burst(c, t + 0.15, 2.4, 850, 0.13, 0.3)
      burst(c, t + 1.8, 1.8, 620, 0.09, 0.3)
      if (master) {
        const voices: [number, number, number, number][] = [
          // [start Hz, end Hz, onset, length]
          [233, 168, 0.3, 2.6],
          [311, 224, 0.7, 2.6],
        ]
        for (const [from, to, onset, len] of voices) {
          const osc = c.createOscillator()
          osc.type = 'sine'
          osc.frequency.setValueAtTime(from, t + onset)
          osc.frequency.linearRampToValueAtTime(to, t + onset + len)
          const trem = c.createOscillator()
          trem.frequency.value = 6.3
          const tremGain = c.createGain()
          tremGain.gain.value = 0.035
          const g = c.createGain()
          g.gain.setValueAtTime(0.0001, t + onset)
          g.gain.exponentialRampToValueAtTime(0.1, t + onset + 0.6)
          g.gain.exponentialRampToValueAtTime(0.0001, t + onset + len + 0.4)
          trem.connect(tremGain)
          tremGain.connect(g.gain)
          osc.connect(g)
          g.connect(master)
          osc.start(t + onset)
          osc.stop(t + onset + len + 0.5)
          trem.start(t + onset)
          trem.stop(t + onset + len + 0.5)
        }
      }
    }
  })
}
