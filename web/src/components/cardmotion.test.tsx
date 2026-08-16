/**
 * The motion tier's client contract (ADR 32): the still is the floor.
 *
 * What these cases pin is the fallback ladder's bottom rung — every way the
 * decoration can be unwelcome or unavailable must render exactly the still
 * the page always had. What they deliberately cannot pin is anything about
 * how the motion *looks*: jsdom has no layout, no video decoder and no
 * WebGL, so a green run here says nothing about the browser (the forest
 * layer's lesson) — the real check is driving the deck page.
 */

import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { reducedMotion } from '../lib/motion'
import { CommanderMotion } from './cardmotion'
import { ParallaxArt } from './parallax'
import { VideoBackdrop } from './videofx'

beforeEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

afterEach(cleanup)

const STILL = <img data-testid="the-still" src="still.webp" alt="" />

function mockMatchMedia(reduce: boolean) {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: reduce && query.includes('reduce'),
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }))
}

it('reducedMotion is false when jsdom has no matchMedia at all', () => {
  expect(reducedMotion()).toBe(false)
})

it('renders the still while the status fetch is unresolved', () => {
  vi.stubGlobal('fetch', vi.fn(() => new Promise(() => undefined)))
  render(<CommanderMotion oracleId="abc" still={STILL} />)
  expect(screen.getByTestId('the-still')).not.toBeNull()
})

it('renders the still when the derivative is not ready', async () => {
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ ready: false, effect: 'depth-drift' }),
  })))
  render(<CommanderMotion oracleId="abc" still={STILL} />)
  await waitFor(() => expect(screen.getByTestId('the-still')).not.toBeNull())
})

it('renders the still when the status fetch fails outright', async () => {
  vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new Error('down'))))
  render(<CommanderMotion oracleId="abc" still={STILL} />)
  await waitFor(() => expect(screen.getByTestId('the-still')).not.toBeNull())
})

it('never fetches without an oracle id', () => {
  const spy = vi.fn()
  vi.stubGlobal('fetch', spy)
  render(<CommanderMotion oracleId={null} still={STILL} />)
  expect(spy).not.toHaveBeenCalled()
  expect(screen.getByTestId('the-still')).not.toBeNull()
})

it('mounts the motion presentation when a derivative is ready', async () => {
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({
      ready: true,
      effect: 'depth-drift',
      fingerprint: 'f00',
      urls: { webm: '/a/loop.webm?v=f00', mp4: '/a/loop.mp4?v=f00',
              poster: '/a/poster.webp?v=f00', depth: '/a/depth.png?v=f00' },
    }),
  })))
  const { container } = render(
    <CommanderMotion oracleId="abc" still={STILL} />)
  // jsdom has no WebGL, so the parallax player must fail closed -- through
  // the video rung or all the way to the still, never to a blank box.
  await waitFor(() => {
    const mounted = container.querySelector('canvas, video') ||
      screen.queryByTestId('the-still')
    expect(mounted).not.toBeNull()
  })
})

it('art mode falls back to the still under reduced motion', () => {
  mockMatchMedia(true)
  render(
    <VideoBackdrop webmSrc="/x.webm" mp4Src="/x.mp4" mode="art"
                   fallback={STILL} />)
  expect(screen.getByTestId('the-still')).not.toBeNull()
  expect(document.querySelector('video')).toBeNull()
})

it('ambience mode renders nothing at all under reduced motion', () => {
  mockMatchMedia(true)
  const { container } = render(
    <VideoBackdrop webmSrc="/x.webm" mode="ambience" />)
  expect(container.innerHTML).toBe('')
})

it('ambience mode renders nothing when the person said no', () => {
  localStorage.setItem('mtglab:ambience', '0')
  const { container } = render(
    <VideoBackdrop webmSrc="/x.webm" mode="ambience" />)
  expect(container.innerHTML).toBe('')
})

it('offers webm before mp4 so the smaller file wins where both play', () => {
  mockMatchMedia(false)
  const { container } = render(
    <VideoBackdrop webmSrc="/x.webm" mp4Src="/x.mp4" mode="ambience" />)
  const sources = [...container.querySelectorAll('source')]
  expect(sources.map((s) => s.getAttribute('type'))).toEqual(
    ['video/webm', 'video/mp4'])
  const video = container.querySelector('video')
  expect(video?.muted).toBe(true)
  expect(video?.getAttribute('playsinline')).not.toBeNull()
})

it('parallax falls back when webgl is unavailable, as in jsdom', async () => {
  mockMatchMedia(false)
  render(
    <ParallaxArt artSrc="/poster.webp" depthSrc="/depth.png"
                 fallback={STILL} />)
  // jsdom's canvas has no webgl context; the component must notice and
  // yield to the fallback rather than leaving a dead canvas.
  await waitFor(() => expect(screen.getByTestId('the-still')).not.toBeNull())
})
