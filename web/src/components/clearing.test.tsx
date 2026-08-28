/**
 * The control, and the one thing it must never be: a button that does nothing.
 *
 * Aaron asked for this from a phone and can only check it on one, so the first
 * test here is the whole lane — on a device with no way to fill the screen,
 * the header offers no control at all and the settings panel says how to get
 * the same thing from the home screen instead.
 *
 * The rest is the toggle behaving like a toggle: it reads its state off the
 * document rather than off its own memory, because Escape and F11 leave
 * fullscreen without telling anybody, and its key does not steal a letter from
 * somebody typing.
 */

import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ClearingButton, ClearingNote } from './clearing'

const undo: (() => void)[] = []

function put(target: object, name: string, value: unknown): void {
  const had = Object.getOwnPropertyDescriptor(target, name)
  Object.defineProperty(target, name, { configurable: true, writable: true, value })
  undo.push(() => {
    if (had) Object.defineProperty(target, name, had)
    else delete (target as Record<string, unknown>)[name]
  })
}

/** A browser that will fill the screen when asked. */
function willFill(): ReturnType<typeof vi.fn> {
  const ask = vi.fn(() => Promise.resolve())
  put(document, 'fullscreenEnabled', true)
  put(document.documentElement, 'requestFullscreen', ask)
  put(document, 'exitFullscreen', vi.fn(() => Promise.resolve()))
  return ask
}

/** Report the screen as filled, the way the browser would. */
function report(on: boolean): void {
  put(document, 'fullscreenElement', on ? document.documentElement : null)
  fireEvent(document, new Event('fullscreenchange'))
}

afterEach(() => {
  cleanup()
  undo.splice(0).reverse().forEach((f) => f())
})

describe('the header control', () => {
  it('is not there at all on a device that cannot fill the screen', () => {
    // jsdom implements no fullscreen, which makes it an honest iPhone.
    render(<ClearingButton />)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('does not answer its key there either, so F stays an ordinary letter', () => {
    render(<ClearingButton />)
    const stopped = fireEvent.keyDown(document.body, { key: 'f' })
    // Nothing called `preventDefault`, so the event ran its course.
    expect(stopped).toBe(true)
  })

  it('appears where the screen can be filled, named once and not renamed', () => {
    willFill()
    render(<ClearingButton />)
    const btn = screen.getByRole('button', { name: 'Clear the table' })
    expect(btn.getAttribute('aria-pressed')).toBe('false')
  })

  it('asks for the screen when pressed', () => {
    const ask = willFill()
    render(<ClearingButton />)
    fireEvent.click(screen.getByRole('button', { name: 'Clear the table' }))
    expect(ask).toHaveBeenCalledTimes(1)
  })

  it('reports pressed only once the document says so — never on the click alone', () => {
    willFill()
    render(<ClearingButton />)
    const btn = screen.getByRole('button', { name: 'Clear the table' })
    fireEvent.click(btn)
    // The request is out but nothing has happened to the screen yet. A control
    // that flipped here would be lying every time an engine refused.
    expect(btn.getAttribute('aria-pressed')).toBe('false')
    report(true)
    expect(btn.getAttribute('aria-pressed')).toBe('true')
  })

  it('notices being left by Escape, which never comes through the control', () => {
    willFill()
    render(<ClearingButton />)
    const btn = screen.getByRole('button', { name: 'Clear the table' })
    report(true)
    expect(btn.getAttribute('aria-pressed')).toBe('true')
    report(false)
    expect(btn.getAttribute('aria-pressed')).toBe('false')
  })

  it('keeps the same name in both states, so a screen reader hears one control', () => {
    willFill()
    render(<ClearingButton />)
    report(true)
    expect(screen.getByRole('button', { name: 'Clear the table' })).toBeTruthy()
  })
})

describe('the key', () => {
  it('does it without the pointer', () => {
    const ask = willFill()
    render(<ClearingButton />)
    fireEvent.keyDown(document.body, { key: 'f' })
    expect(ask).toHaveBeenCalledTimes(1)
  })

  it('does it with Shift held, because that is still the same letter', () => {
    const ask = willFill()
    render(<ClearingButton />)
    fireEvent.keyDown(document.body, { key: 'F', shiftKey: true })
    expect(ask).toHaveBeenCalledTimes(1)
  })

  it('stays out of the way of somebody typing in a field', () => {
    const ask = willFill()
    render(<><ClearingButton /><input aria-label="Card name" /></>)
    fireEvent.keyDown(screen.getByLabelText('Card name'), { key: 'f' })
    expect(ask).not.toHaveBeenCalled()
  })

  it('stays out of the way of a rationale box, which is where most typing here happens', () => {
    const ask = willFill()
    render(<><ClearingButton /><textarea aria-label="Why" /></>)
    fireEvent.keyDown(screen.getByLabelText('Why'), { key: 'f' })
    expect(ask).not.toHaveBeenCalled()
  })

  it('stays out of the way of anything editable in place', () => {
    const ask = willFill()
    render(<><ClearingButton /><div contentEditable data-testid="note" /></>)
    const note = screen.getByTestId('note')
    // jsdom parses the attribute but does not compute `isContentEditable`.
    Object.defineProperty(note, 'isContentEditable', { configurable: true, value: true })
    fireEvent.keyDown(note, { key: 'f' })
    expect(ask).not.toHaveBeenCalled()
  })

  it('leaves a shortcut belonging to the browser alone', () => {
    const ask = willFill()
    render(<ClearingButton />)
    fireEvent.keyDown(document.body, { key: 'f', metaKey: true })
    fireEvent.keyDown(document.body, { key: 'f', ctrlKey: true })
    fireEvent.keyDown(document.body, { key: 'f', altKey: true })
    expect(ask).not.toHaveBeenCalled()
  })

  it('ignores a held key, so leaning on F is one trip and not forty', () => {
    const ask = willFill()
    render(<ClearingButton />)
    fireEvent.keyDown(document.body, { key: 'f', repeat: true })
    expect(ask).not.toHaveBeenCalled()
  })

  it('stops answering once the control is gone', () => {
    const ask = willFill()
    render(<ClearingButton />).unmount()
    fireEvent.keyDown(document.body, { key: 'f' })
    expect(ask).not.toHaveBeenCalled()
  })
})

describe('what a phone is told instead', () => {
  it('names the two taps that get there, and no technology at all', () => {
    render(<ClearingNote />)
    const said = document.body.textContent ?? ''
    expect(said).toContain('Add to Home Screen')
    expect(said).toContain('no tabs')
    // Commandment 10: the copy says what happens, never what does it.
    expect(said).not.toMatch(/safari|browser|fullscreen|app|install/i)
  })
})
