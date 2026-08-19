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
import { coverTopWindow, reducedMotion } from '../lib/motion'
import { CommanderMotion } from './cardmotion'
import { FRAGMENT_SHADER, ParallaxArt } from './parallax'
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

it('honours a caller-supplied effect ladder', async () => {
  // The gallery's flat watercolour asks for `breath` and must never be
  // offered a pan — the ladder is the page's to choose.
  const spy = vi.fn((_url: string) => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ ready: false, effect: 'breath' }),
  }))
  vi.stubGlobal('fetch', spy)
  render(
    <CommanderMotion oracleId="abc" still={STILL} effects={['breath']} />)
  await waitFor(() => expect(spy).toHaveBeenCalled())
  expect(String(spy.mock.calls[0]?.[0])).toContain('/api/art/motion/abc/breath')
  expect(spy.mock.calls.map((c) => String(c[0])).join(' '))
    .not.toContain('depth-drift')
})

it('anchors the loop to the centre when the caller says so', async () => {
  // The Island bug: the hero band's `center top` traded a portrait
  // painting's subject for its sky. `position="center"` is the gallery's
  // framing — the same band its centre-cropped still showed.
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({
      ready: true,
      effect: 'breath',
      fingerprint: 'f00',
      urls: { webm: '/a/loop.webm?v=f00', mp4: '/a/loop.mp4?v=f00',
              poster: '/a/poster.webp?v=f00' },
    }),
  })))
  const { container } = render(
    <CommanderMotion oracleId="abc" still={STILL} effects={['breath']}
                     position="center" />)
  await waitFor(() =>
    expect(container.querySelector('video')).not.toBeNull())
  expect(container.querySelector('video')?.className)
    .toContain('motion-art-center')
})

it('coverTopWindow centres the band only when asked', () => {
  // A 3:4 portrait in a wide box: `top` pins the window to the painting's
  // top (v = 1), `center` halves the leftover.
  const top = coverTopWindow(600, 800, 626, 457, 'top')
  const centred = coverTopWindow(600, 800, 626, 457, 'center')
  expect(top.scale).toEqual(centred.scale)
  expect(centred.offset[1]).toBeCloseTo(top.offset[1] / 2)
})

it('asks the server about the painting the page is showing', async () => {
  // The Gyome/Trostani bug: without the art in the question, a deck that
  // swapped printings was served the old painting's loop. The still (the
  // correct painting) is the floor a mismatch must land on.
  const spy = vi.fn((_url: string) => Promise.resolve({
    ok: true,
    json: () => Promise.resolve({ ready: false, effect: 'depth-drift' }),
  }))
  vi.stubGlobal('fetch', spy)
  render(
    <CommanderMotion oracleId="abc" still={STILL}
                     art="https://x/art_crop/chosen.jpg?9" />)
  await waitFor(() => expect(spy).toHaveBeenCalled())
  const asked = String(spy.mock.calls[0]?.[0])
  expect(asked).toContain('/api/art/motion/abc/depth-drift')
  expect(asked).toContain(
    `art=${encodeURIComponent('https://x/art_crop/chosen.jpg?9')}`)
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

/* The crop window is the one part of the GL path jsdom can check: pure
 * numbers in, pure numbers out. What it pins is that the canvas frames the
 * painting exactly as the still's `object-fit: cover; object-position:
 * center top` does — the bug these cases came from was the full painting
 * squashed into the band, upside-down. */

it('a band wider than the painting shows the full width and the TOP slice', () => {
  // The deck hero: a 4:1 band over a ~1.37:1 art crop.
  const { scale, offset } = coverTopWindow(1200, 875, 2440, 610)
  expect(scale[0]).toBe(1)
  expect(offset[0]).toBe(0)
  const slice = (1200 / 875) / (2440 / 610)
  expect(scale[1]).toBeCloseTo(slice, 10)
  // Top-anchored in GL coordinates: the window ends at v = 1, the top row.
  expect(offset[1] + scale[1]).toBeCloseTo(1, 10)
})

it('a box taller than the painting shows full height, centred', () => {
  const { scale, offset } = coverTopWindow(1200, 600, 500, 500)
  expect(scale).toEqual([0.5, 1])
  expect(offset).toEqual([0.25, 0])
})

it('a box matching the painting is the identity window', () => {
  const { scale, offset } = coverTopWindow(1200, 875, 2400, 1750)
  expect(scale).toEqual([1, 1])
  expect(offset).toEqual([0, 0])
})

it('the shader samples art and depth through the same window', () => {
  // A refactor that drops the uniforms fails here rather than silently
  // rendering the stretched full texture again.
  expect(FRAGMENT_SHADER).toContain('uvScale')
  expect(FRAGMENT_SHADER).toContain('uvOffset')
  expect(FRAGMENT_SHADER).toContain('texture2D(depth, base)')
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
