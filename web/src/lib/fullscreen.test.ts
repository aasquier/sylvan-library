/**
 * The one thing this file exists to hold still: **a phone that cannot fill the
 * screen must be told so, not handed a control that does nothing.**
 *
 * Every other case here is a spelling. Safari answered only the `webkit` names
 * until 16.4 and still answers them, so a build that asks in one dialect and
 * listens in the other enters fullscreen and then insists it did not — and
 * that failure is invisible to a suite that stubs only the modern names,
 * because the modern names are what this Mac has.
 *
 * jsdom implements none of it, which is the right starting point: the default
 * environment here is an honest model of the iPhone.
 */

import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { canClear, cleared, standing, toggleCleared, useClearing } from './fullscreen'

/** Install a property on an object for one test. */
const undo: (() => void)[] = []

function put(target: object, name: string, value: unknown): void {
  const had = Object.getOwnPropertyDescriptor(target, name)
  Object.defineProperty(target, name, { configurable: true, writable: true, value })
  undo.push(() => {
    if (had) Object.defineProperty(target, name, had)
    else delete (target as Record<string, unknown>)[name]
  })
}

/** A browser that answers the modern names. */
function modern(): { ask: ReturnType<typeof vi.fn>; quit: ReturnType<typeof vi.fn> } {
  const ask = vi.fn(() => Promise.resolve())
  const quit = vi.fn(() => Promise.resolve())
  put(document, 'fullscreenEnabled', true)
  put(document.documentElement, 'requestFullscreen', ask)
  put(document, 'exitFullscreen', quit)
  return { ask, quit }
}

/** A browser that answers only the prefixed ones — Safari before 16.4. */
function prefixed(): { ask: ReturnType<typeof vi.fn>; quit: ReturnType<typeof vi.fn> } {
  // Returns undefined rather than a promise, which the old spelling does and
  // which is the reason `toggleCleared` branches instead of using `??`.
  const ask = vi.fn(() => undefined)
  const quit = vi.fn(() => undefined)
  put(document, 'webkitFullscreenEnabled', true)
  put(document.documentElement, 'webkitRequestFullscreen', ask)
  put(document, 'webkitExitFullscreen', quit)
  return { ask, quit }
}

afterEach(() => {
  undo.splice(0).reverse().forEach((f) => f())
  vi.unstubAllGlobals()
})

describe('whether the screen can be filled at all', () => {
  it('says no on a device with no such call — the iPhone, and the whole reason for the home-screen half', () => {
    expect(canClear()).toBe(false)
  })

  it('says yes to a browser with the modern names', () => {
    modern()
    expect(canClear()).toBe(true)
  })

  it('says yes to a browser with only the prefixed ones', () => {
    prefixed()
    expect(canClear()).toBe(true)
  })

  it('says no when the call exists but the document is not permitted it', () => {
    // A frame without the permission: the method is there and rejects on
    // every press. `fullscreenEnabled` is the flag that knows.
    put(document, 'fullscreenEnabled', false)
    put(document.documentElement, 'requestFullscreen', vi.fn())
    expect(canClear()).toBe(false)
  })

  it('says no when it is permitted but nothing can be asked', () => {
    put(document, 'fullscreenEnabled', true)
    expect(canClear()).toBe(false)
  })
})

describe('whether the screen is filled right now', () => {
  it('is false with nothing fullscreen', () => {
    expect(cleared()).toBe(false)
  })

  it('reads the modern name', () => {
    put(document, 'fullscreenElement', document.body)
    expect(cleared()).toBe(true)
  })

  it('reads the prefixed one, which is the only one Safari set for years', () => {
    put(document, 'webkitFullscreenElement', document.body)
    expect(cleared()).toBe(true)
  })
})

describe('asking for it', () => {
  it('asks the document element and never a subtree, so the portalled fixed layers survive', () => {
    const { ask } = modern()
    toggleCleared()
    expect(ask).toHaveBeenCalledTimes(1)
    expect(ask.mock.instances[0]).toBe(document.documentElement)
  })

  it('gives it back when it is already filled', () => {
    const { ask, quit } = modern()
    put(document, 'fullscreenElement', document.documentElement)
    toggleCleared()
    expect(quit).toHaveBeenCalledTimes(1)
    expect(ask).not.toHaveBeenCalled()
  })

  it('uses the prefixed call where that is the only one', () => {
    const { ask } = prefixed()
    toggleCleared()
    expect(ask).toHaveBeenCalledTimes(1)
  })

  it('asks ONCE on a browser that has both spellings', () => {
    const both = modern()
    const old = prefixed()
    toggleCleared()
    expect(both.ask).toHaveBeenCalledTimes(1)
    expect(old.ask).not.toHaveBeenCalled()
  })

  it('asks once even when the modern call returns nothing to chain on', () => {
    // **This is the case the branch exists for**, and the one the test above
    // cannot see. `modern?.() ?? old?.()` only short-circuits on a value, and
    // `requestFullscreen` has not always returned a promise — where it does
    // not, a nullish chain falls straight through and fires the prefixed call
    // as well. Two requests going out, and on the way back two exits.
    put(document, 'fullscreenEnabled', true)
    const modernVoid = vi.fn(() => undefined)
    put(document.documentElement, 'requestFullscreen', modernVoid)
    const old = prefixed()
    toggleCleared()
    expect(modernVoid).toHaveBeenCalledTimes(1)
    expect(old.ask).not.toHaveBeenCalled()
  })

  it('gives it back once on a browser with both, for the same reason', () => {
    put(document, 'fullscreenElement', document.documentElement)
    put(document, 'fullscreenEnabled', true)
    const modernVoid = vi.fn(() => undefined)
    put(document, 'exitFullscreen', modernVoid)
    const old = prefixed()
    toggleCleared()
    expect(modernVoid).toHaveBeenCalledTimes(1)
    expect(old.quit).not.toHaveBeenCalled()
  })

  it('does nothing at all where there is no call to make', () => {
    expect(() => toggleCleared()).not.toThrow()
  })

  it('swallows a refusal rather than leaving an unhandled rejection', async () => {
    put(document, 'fullscreenEnabled', true)
    put(document.documentElement, 'requestFullscreen',
      vi.fn(() => Promise.reject(new Error('not from a gesture'))))
    expect(() => toggleCleared()).not.toThrow()
    await Promise.resolve()
  })

  it('survives an engine that throws instead of rejecting', () => {
    put(document, 'fullscreenEnabled', true)
    put(document.documentElement, 'requestFullscreen', vi.fn(() => { throw new Error('no') }))
    expect(() => toggleCleared()).not.toThrow()
  })
})

describe('whether the app was launched from a home screen', () => {
  it('is false in a plain browser with no media query support at all', () => {
    expect(standing()).toBe(false)
  })

  it('reads the flag iOS has always set', () => {
    put(navigator, 'standalone', true)
    expect(standing()).toBe(true)
  })

  it('reads the modern display mode', () => {
    vi.stubGlobal('matchMedia', (media: string) =>
      ({ matches: media.includes('standalone'), media }))
    expect(standing()).toBe(true)
  })

  it('is false for a browser tab, which reports its own display mode', () => {
    vi.stubGlobal('matchMedia', (media: string) =>
      ({ matches: media.includes('browser'), media }))
    expect(standing()).toBe(false)
  })
})

describe('the hook', () => {
  it('offers nothing where the call does not exist', () => {
    const { result } = renderHook(() => useClearing())
    expect(result.current.offered).toBe(false)
    expect(result.current.on).toBe(false)
  })

  it('offers nothing when the chrome is already gone', () => {
    modern()
    put(navigator, 'standalone', true)
    const { result } = renderHook(() => useClearing())
    expect(result.current.homescreen).toBe(true)
    expect(result.current.offered).toBe(false)
  })

  it('follows the document rather than the press, so Escape and F11 move it too', () => {
    modern()
    const { result } = renderHook(() => useClearing())
    expect(result.current.on).toBe(false)
    // Nobody pressed the control; the browser simply reports a change, which
    // is exactly what leaving with Escape looks like from in here.
    put(document, 'fullscreenElement', document.documentElement)
    act(() => { document.dispatchEvent(new Event('fullscreenchange')) })
    expect(result.current.on).toBe(true)
  })

  it('follows the prefixed event as well, which is the only one older Safari fires', () => {
    prefixed()
    const { result } = renderHook(() => useClearing())
    put(document, 'webkitFullscreenElement', document.documentElement)
    act(() => { document.dispatchEvent(new Event('webkitfullscreenchange')) })
    expect(result.current.on).toBe(true)
  })

  it('lets go of both listeners when it unmounts', () => {
    const off = vi.spyOn(document, 'removeEventListener')
    renderHook(() => useClearing()).unmount()
    const names = off.mock.calls.map((c) => c[0])
    expect(names).toContain('fullscreenchange')
    expect(names).toContain('webkitfullscreenchange')
  })
})
