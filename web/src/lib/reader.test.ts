/** Every byte of the OCR engine comes from us.
 *
 * `ocr.py` fetches the three engine files once, **pins each by SHA-256**, and
 * serves them from `/api/ocr`. That pin is the whole security argument for
 * running six megabytes of somebody else's compiler output in a visitor's
 * browser -- and it has exactly one bypass: tesseract.js's own defaults point
 * at a CDN, so a `createWorker` call that forgot an option would fetch
 * unpinned WebAssembly from a third party, leak every visitor's IP to it, and
 * do both silently, because the feature would still work.
 *
 * `reader.ts` says all of this in a comment. A comment is not a guard, which
 * is what this file is for.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const created: Array<Record<string, unknown>> = []

vi.mock('tesseract.js', () => ({
  createWorker: (_lang: string, _oem: number, options: Record<string, unknown>) => {
    created.push(options)
    return Promise.resolve({ terminate: () => Promise.resolve() })
  },
}))

beforeEach(() => {
  created.length = 0
})

// The worker is module-scoped and shared on purpose (a 99 goes past at
// several cards a minute), so each test has to put it back.
afterEach(async () => {
  const { rest } = await import('./reader')
  await rest()
})

describe('where the engine is fetched from', () => {
  it('names a first-party path for every file the worker loads', async () => {
    const { warm } = await import('./reader')
    warm()
    // `warm` is deliberately fire-and-forget; let its promise settle.
    await new Promise((resolve) => { setTimeout(resolve, 0) })

    expect(created).toHaveLength(1)
    // All three, by name. Leaving any one unset is what falls through to the
    // CDN, and the three fail differently: the worker script, the wasm core,
    // and the trained data.
    for (const key of ['workerPath', 'corePath', 'langPath']) {
      const value = created[0]?.[key]
      expect(typeof value, `${key} must be set`).toBe('string')
      expect(String(value), `${key} must be ours`).toMatch(/^\/api\/ocr(\/|$)/)
    }
  })

  it('loads the engine once and shares it', async () => {
    const { warm } = await import('./reader')
    warm()
    warm()
    await new Promise((resolve) => { setTimeout(resolve, 0) })
    expect(created).toHaveLength(1)
  })
})
