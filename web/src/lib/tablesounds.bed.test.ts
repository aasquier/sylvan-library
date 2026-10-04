/**
 * The beds: a room's own sound looped through the graph, gaplessly. The one
 * property jsdom can hold is the loop's shape — that it loops at all, that
 * its points skip the decoder's silence, and that a stop stops it — against a
 * fake context whose decoder answers with a buffer we shaped.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

let started: Array<{ loop: boolean; loopStart: number; loopEnd: number; offset: number }> = []
let stopped = 0
let fetched: string[] = []

class FakeParam { value = 0 }
class FakeNode {
  gain = new FakeParam()
  threshold = new FakeParam(); knee = new FakeParam(); ratio = new FakeParam()
  attack = new FakeParam(); release = new FakeParam()
  buffer: unknown = null
  loop = false
  loopStart = 0
  loopEnd = 0
  connect() { return this }
  disconnect() {}
  start(_when: number, offset = 0) {
    started.push({ loop: this.loop, loopStart: this.loopStart, loopEnd: this.loopEnd, offset })
  }
  stop() { stopped++ }
}
/** A one-second buffer at 1000 Hz with 100 ms of decoder silence at each end. */
function shapedBuffer() {
  const data = new Float32Array(1000)
  for (let i = 100; i < 900; i++) data[i] = 0.2
  return { sampleRate: 1000, length: 1000, getChannelData: () => data }
}
class FakeAudioContext {
  state = 'running'
  sampleRate = 1000
  currentTime = 0
  destination = {}
  resume() { return Promise.resolve() }
  createGain() { return new FakeNode() }
  createDynamicsCompressor() { return new FakeNode() }
  createBufferSource() { return new FakeNode() }
  decodeAudioData() { return Promise.resolve(shapedBuffer()) }
}

async function fresh() {
  vi.resetModules()
  return import('./tablesounds')
}
const settle = () => new Promise((r) => setTimeout(r, 0))

beforeEach(() => {
  started = []
  stopped = 0
  fetched = []
  localStorage.clear()
  vi.stubGlobal('AudioContext', FakeAudioContext)
  vi.stubGlobal('fetch', (url: string) => {
    fetched.push(url)
    return Promise.resolve({ arrayBuffer: () => Promise.resolve(new ArrayBuffer(8)) })
  })
})
afterEach(() => vi.unstubAllGlobals())

describe('the beds', () => {
  it('answer false while the switch is off, and fetch nothing', async () => {
    const snd = await fresh()
    expect(snd.bedStart('/hut.m4a')).toBe(false)
    expect(fetched).toEqual([])
  })

  it('loop the decoded buffer between its first and last real samples', async () => {
    localStorage.setItem('mtglab-table-sound', '1')
    const snd = await fresh()
    expect(snd.bedStart('/hut.m4a')).toBe(true)
    await settle(); await settle(); await settle()
    expect(fetched).toEqual(['/hut.m4a'])
    expect(started).toHaveLength(1)
    const bed = started[0]!
    expect(bed.loop).toBe(true)
    // 100 ms of silence at each end of a one-second buffer: the loop runs
    // from 0.1 s to 0.9 s and the playhead starts at the loop's start, so
    // the decoder's padding is never heard, at the join or the start.
    expect(bed.loopStart).toBeCloseTo(0.1, 5)
    expect(bed.loopEnd).toBeCloseTo(0.9, 5)
    expect(bed.offset).toBeCloseTo(0.1, 5)
  })

  it('stop when told, and a second bed replaces the first', async () => {
    localStorage.setItem('mtglab-table-sound', '1')
    const snd = await fresh()
    snd.bedStart('/hut.m4a')
    await settle(); await settle(); await settle()
    snd.bedStart('/tavern.m4a')
    await settle(); await settle(); await settle()
    expect(stopped).toBe(1)
    expect(started).toHaveLength(2)
    snd.bedStop()
    expect(stopped).toBe(2)
    // Decoded once per URL: coming back to a room costs no second fetch.
    snd.bedStart('/hut.m4a')
    await settle(); await settle(); await settle()
    expect(fetched).toEqual(['/hut.m4a', '/tavern.m4a'])
  })
})
