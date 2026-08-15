/**
 * The one property that matters more than how it sounds: **off means
 * nothing exists.** No AudioContext until the preference is on and a play
 * function actually runs — the default experience allocates nothing, and the
 * first construction happens inside the click that enabled it.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { deal, flip, riffle, setSound, soundOn } from './tablesounds'

let constructed = 0

class FakeAudioContext {
  state = 'running'
  sampleRate = 48000
  currentTime = 0
  destination = {}
  constructor() { constructed++ }
  resume() { return Promise.resolve() }
  createBuffer(_ch: number, length: number) {
    return { getChannelData: () => new Float32Array(length) }
  }
  createBufferSource() {
    return { buffer: null, connect: () => {}, start: () => {} }
  }
  createBiquadFilter() {
    return { type: '', frequency: { value: 0 }, Q: { value: 0 }, connect: () => {} }
  }
  createGain() {
    return { gain: { value: 0 }, connect: () => {} }
  }
}

beforeEach(() => {
  constructed = 0
  localStorage.clear()
  vi.stubGlobal('AudioContext', FakeAudioContext)
})

afterEach(() => {
  // The module holds its context across calls; turning sound off between
  // tests keeps one test's context from muddying the next one's count.
  setSound(false)
  vi.unstubAllGlobals()
})

describe('tablesounds', () => {
  it('is off by default and constructs nothing while it stays off', () => {
    expect(soundOn()).toBe(false)
    riffle()
    deal()
    flip()
    expect(constructed).toBe(0)
  })

  it('persists the preference under its own key, not the table stash', () => {
    setSound(true)
    expect(localStorage.getItem('mtglab-table-sound')).toBe('1')
    expect(soundOn()).toBe(true)
    setSound(false)
    expect(localStorage.getItem('mtglab-table-sound')).toBeNull()
  })

  it('constructs one context once enabled, and reuses it', () => {
    setSound(true)
    riffle()
    flip()
    deal()
    expect(constructed).toBeLessThanOrEqual(1)
    expect(constructed).toBeGreaterThan(0)
  })
})
