/**
 * A room that is footage, and the two things jsdom can hold about it: what
 * is mounted, and whether anything was asked to make a sound.
 */
import { cleanup, fireEvent, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { roomFootage } from '../lib/roomfootage'
import { RoomPlate } from './roomplate'
import { RoomTone } from './roomtone'

const FOOTAGE = roomFootage('barkeep')!

beforeEach(() => {
  localStorage.clear()
  vi.spyOn(HTMLMediaElement.prototype, 'play').mockImplementation(
    () => Promise.resolve())
  vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(
    () => undefined)
})
afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('RoomPlate', () => {
  it('mounts the loop on the body with the still as its poster, a veil, and the bed', () => {
    render(<RoomPlate footage={FOOTAGE} />)
    const plate = document.body.querySelector('.room-plate')!
    expect(plate).not.toBeNull()
    expect(plate.getAttribute('aria-hidden')).toBe('true')
    const film = plate.querySelector<HTMLVideoElement>('video.room-plate-film')!
    expect(film).not.toBeNull()
    expect(film.getAttribute('poster')).toBe(FOOTAGE.still)
    expect([...film.querySelectorAll('source')].map((s) => s.getAttribute('src')))
      .toEqual([FOOTAGE.webm, FOOTAGE.mp4])
    expect(plate.querySelector('.room-plate-veil')).not.toBeNull()
    expect(plate.querySelector('audio')?.getAttribute('src')).toBe(FOOTAGE.tone)
  })

  it('asks the forest to step out while it is open, and lets it back in after', () => {
    const { unmount } = render(<RoomPlate footage={FOOTAGE} />)
    expect(document.body.classList.contains('in-footage-room')).toBe(true)
    unmount()
    expect(document.body.classList.contains('in-footage-room')).toBe(false)
  })

  it('is the still when ambience is off: a room removed is a void', () => {
    localStorage.setItem('mtglab-ambience', '0')
    render(<RoomPlate footage={FOOTAGE} />)
    const plate = document.body.querySelector('.room-plate')!
    expect(plate.querySelector('video')).toBeNull()
    expect(plate.querySelector('img.room-plate-film')?.getAttribute('src'))
      .toBe(FOOTAGE.still)
  })
})

describe('RoomTone', () => {
  it('asks for nothing while the switch is off', () => {
    render(<RoomTone src={FOOTAGE.tone} />)
    const el = document.querySelector('audio')!
    expect(el.getAttribute('preload')).toBe('none')
    expect(el.loop).toBe(true)
    expect(HTMLMediaElement.prototype.play).not.toHaveBeenCalled()
  })

  it('plays when the switch is on, and stops on the way out', () => {
    localStorage.setItem('mtglab-table-sound', '1')
    const { unmount } = render(<RoomTone src={FOOTAGE.tone} />)
    expect(HTMLMediaElement.prototype.play).toHaveBeenCalledTimes(1)
    unmount()
    expect(HTMLMediaElement.prototype.pause).toHaveBeenCalled()
  })

  it('waits for a gesture when the browser refuses, and tries once more on it',
     async () => {
    localStorage.setItem('mtglab-table-sound', '1')
    const play = vi.mocked(HTMLMediaElement.prototype.play)
    play.mockImplementationOnce(() => Promise.reject(new Error('NotAllowed')))
    render(<RoomTone src={FOOTAGE.tone} />)
    expect(play).toHaveBeenCalledTimes(1)
    // The refusal lands on a microtask; let it.
    await Promise.resolve()
    await Promise.resolve()
    fireEvent.pointerDown(window)
    expect(play).toHaveBeenCalledTimes(2)
    // One shot: a second gesture does not pile up plays.
    fireEvent.pointerDown(window)
    expect(play).toHaveBeenCalledTimes(2)
  })
})
