/**
 * The one property that matters more than how it sounds: **off means
 * nothing exists.** No AudioContext until the preference is on and a play
 * function actually runs — the default experience allocates nothing, and the
 * first construction happens inside the click that enabled it.
 *
 * The second property earned its place by shipping broken: a **suspended
 * context accepts every schedule and plays none of it**, so the voices must
 * wait for `resume()` rather than firing into the void.
 *
 * The module holds its context for the life of the page, so each test gets a
 * fresh copy of the module (`vi.resetModules`) rather than sharing one
 * context and arguing about who constructed it.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

let constructed = 0
let started = 0
let resumed = 0
/** What state a newly constructed fake context is born in. Safari says
 *  'suspended'; Chrome inside a gesture says 'running'. */
let bornIn = 'running'

class FakeParam {
  value = 0
  setValueAtTime(v: number) { this.value = v; return this }
  exponentialRampToValueAtTime() { return this }
  linearRampToValueAtTime() { return this }
}

class FakeNode {
  gain = new FakeParam()
  frequency = new FakeParam()
  Q = new FakeParam()
  threshold = new FakeParam()
  knee = new FakeParam()
  ratio = new FakeParam()
  attack = new FakeParam()
  release = new FakeParam()
  type = ''
  buffer: unknown = null
  connect() { return this }
  start() { started++ }
  stop() {}
}

class FakeAudioContext {
  state = bornIn
  sampleRate = 48000
  currentTime = 0
  destination = {}
  constructor() { constructed++ }
  // Real resume is asynchronous — the state flips when the promise settles,
  // not when the call returns. A synchronous fake here would hide exactly
  // the ordering bug the suspended test exists to catch.
  resume() {
    resumed++
    return Promise.resolve().then(() => { this.state = 'running' })
  }
  createBuffer(_ch: number, length: number) {
    return { getChannelData: () => new Float32Array(length) }
  }
  createBufferSource() { return new FakeNode() }
  createBiquadFilter() { return new FakeNode() }
  createGain() { return new FakeNode() }
  createOscillator() { return new FakeNode() }
  createDynamicsCompressor() { return new FakeNode() }
}

type Sounds = typeof import('./tablesounds')

async function fresh(): Promise<Sounds> {
  vi.resetModules()
  return import('./tablesounds')
}

beforeEach(() => {
  constructed = 0
  started = 0
  resumed = 0
  bornIn = 'running'
  localStorage.clear()
  vi.stubGlobal('AudioContext', FakeAudioContext)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('tablesounds', () => {
  it('is off by default and constructs nothing while it stays off', async () => {
    const snd = await fresh()
    expect(snd.soundOn()).toBe(false)
    snd.riffle()
    snd.deal()
    snd.flip()
    snd.shimmer()
    snd.wheelTurn(1620, 3800)
    snd.wake()
    expect(constructed).toBe(0)
    expect(started).toBe(0)
  })

  it('wheelTurn clicks once per notch and settles, or stays quiet under one', async () => {
    const snd = await fresh()
    snd.setSound(true)
    // 4.5 turns = 54 notches; each click is two bursts, the settle is one
    // oscillator. Placement in time is the bezier's business; the count is
    // this test's.
    snd.wheelTurn(1620, 3800)
    expect(started).toBe(54 * 2 + 1)
    started = 0
    // Under one notch there is nothing to ratchet over — silence, not a
    // lone floating click.
    snd.wheelTurn(15, 3800)
    expect(started).toBe(0)
  })

  it('persists the preference under its own key, not the table stash', async () => {
    const snd = await fresh()
    snd.setSound(true)
    expect(localStorage.getItem('mtglab-table-sound')).toBe('1')
    expect(snd.soundOn()).toBe(true)
    snd.setSound(false)
    expect(localStorage.getItem('mtglab-table-sound')).toBeNull()
  })

  it('constructs one context once enabled, and reuses it', async () => {
    const snd = await fresh()
    snd.setSound(true)
    snd.riffle()
    snd.flip()
    snd.deal()
    expect(constructed).toBe(1)
  })

  it('wake() constructs the context and makes noise — the toggle is heard', async () => {
    const snd = await fresh()
    snd.setSound(true)
    snd.wake()
    expect(constructed).toBe(1)
    expect(started).toBeGreaterThan(0)
  })

  it('a context born suspended is resumed before anything is scheduled on it', async () => {
    bornIn = 'suspended'
    const snd = await fresh()
    snd.setSound(true)
    snd.deal()
    // Nothing may start while the context is suspended — a schedule made
    // there is accepted and never heard, which is the bug this pins.
    expect(started).toBe(0)
    await Promise.resolve()
    await Promise.resolve()
    expect(resumed).toBeGreaterThan(0)
    expect(started).toBeGreaterThan(0)
  })
})
