/**
 * Volumetric vapour over a painting (second punch list of 2026-08-15,
 * items 4 and 5): a canvas particle layer that puts *steam* over real art,
 * where the first Laboratory bench put vector circles over a drawing.
 *
 * The design premise of the photo-real pass is that the paintings are the
 * material — Magic's own art, hotlinked and credited, is what this app has
 * licence-clean access to at painting quality — and what code adds on top
 * has to live up to them. A crisp-edged SVG circle cannot; a few hundred
 * soft sprites breathing upward can. Each particle is a radial-gradient
 * blob (pre-rendered once, tinted per source), drawn with `lighter`
 * compositing so overlapping vapour brightens the way lit steam does,
 * rising with sinusoidal sway, growing and thinning as it climbs.
 *
 * Sources are given in fractions of the host box, so the caller measures
 * them once against the painting (the same discipline as the wheel's
 * circle) and the layer holds them at any size. `busy` is the Laboratory's
 * "cooking" state: spawn rate and climb speed go up, so minutes of research
 * read as the lab working harder rather than as a stuck page.
 *
 * Three ways this layer politely does not exist: `prefers-reduced-motion`
 * (the painting alone is the reduced experience), the ambience switch
 * (`lib/prefs.ts` — a person's no-thank-you outranks the theatre), and a
 * canvas whose 2d context is unavailable (jsdom, ancient browsers), where
 * it renders an empty element and schedules nothing.
 */

import { useEffect, useRef } from 'react'
import { useAmbience } from '../lib/prefs'

export interface VaporSource {
  /** Fraction of the host's width. */
  x: number
  /** Fraction of the host's height. */
  y: number
  /** Cool is lit steam (teal-white); warm is candle-smoke (amber-grey). */
  hue?: 'cool' | 'warm'
  /** Relative plume size; 1 is a flask, 2 a vat. */
  size?: number
}

interface Particle {
  sx: number
  x: number
  y: number
  born: number
  life: number
  drift: number
  sway: number
  scale: number
  source: number
}

/** One blob sprite per hue, drawn once and stamped thousands of times. */
function makeSprite(hue: 'cool' | 'warm'): HTMLCanvasElement | null {
  const c = document.createElement('canvas')
  c.width = 64
  c.height = 64
  const ctx = c.getContext('2d')
  if (!ctx) return null
  const g = ctx.createRadialGradient(32, 32, 2, 32, 32, 30)
  if (hue === 'cool') {
    g.addColorStop(0, 'rgba(210, 240, 245, 0.55)')
    g.addColorStop(0.5, 'rgba(160, 210, 220, 0.18)')
    g.addColorStop(1, 'rgba(140, 200, 210, 0)')
  } else {
    g.addColorStop(0, 'rgba(235, 215, 175, 0.4)')
    g.addColorStop(0.5, 'rgba(180, 160, 130, 0.13)')
    g.addColorStop(1, 'rgba(150, 140, 120, 0)')
  }
  ctx.fillStyle = g
  ctx.fillRect(0, 0, 64, 64)
  return c
}

export function VaporLayer({ sources, busy = false, className = '' }: {
  sources: VaporSource[]
  busy?: boolean
  className?: string
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const busyRef = useRef(busy)
  busyRef.current = busy
  const [ambience] = useAmbience()

  useEffect(() => {
    if (!ambience) return
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    const sprites = {
      cool: makeSprite('cool'),
      warm: makeSprite('warm'),
    }

    let w = 0
    let h = 0
    const dpr = Math.min(window.devicePixelRatio || 1, 2)
    const resize = () => {
      const rect = canvas.getBoundingClientRect()
      w = rect.width
      h = rect.height
      canvas.width = Math.max(1, Math.round(w * dpr))
      canvas.height = Math.max(1, Math.round(h * dpr))
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    }
    resize()
    const ro = new ResizeObserver(resize)
    ro.observe(canvas)

    const particles: Particle[] = []
    // Fractional spawn accumulator per source, so slow rates stay smooth.
    const debt = sources.map(() => 0)
    let last = performance.now()
    let raf = 0

    const frame = (now: number) => {
      raf = requestAnimationFrame(frame)
      // A tab that slept for a minute owes no minute of particles.
      const dt = Math.min((now - last) / 1000, 0.1)
      last = now
      if (w === 0 || h === 0) return

      const cooking = busyRef.current
      sources.forEach((s, i) => {
        const size = s.size ?? 1
        const rate = (cooking ? 7 : 2.2) * Math.sqrt(size)
        debt[i] = (debt[i] ?? 0) + rate * dt
        while ((debt[i] ?? 0) >= 1) {
          debt[i] = (debt[i] ?? 0) - 1
          particles.push({
            sx: s.x * w + (Math.random() - 0.5) * 14 * size,
            x: 0,
            y: s.y * h,
            born: now,
            life: 3200 + Math.random() * 2600,
            drift: (Math.random() - 0.5) * 8,
            sway: 6 + Math.random() * 10,
            scale: (0.35 + Math.random() * 0.5) * size,
            source: i,
          })
        }
      })

      ctx.clearRect(0, 0, w, h)
      ctx.globalCompositeOperation = 'lighter'
      const speed = cooking ? 1.7 : 1
      for (let i = particles.length - 1; i >= 0; i--) {
        const p = particles[i]
        if (!p) continue
        const age = (now - p.born) / p.life
        if (age >= 1) {
          particles.splice(i, 1)
          continue
        }
        const src = sources[p.source]
        const sprite = sprites[src?.hue ?? 'cool']
        if (!sprite) continue
        // Rise, sway, swell, thin.
        const rise = age * (h * 0.32) * speed * (0.6 + p.scale * 0.5)
        const x = p.sx + Math.sin(age * 5 + p.drift) * p.sway * age + p.drift * age * 6
        const y = p.y - rise
        const grow = (0.5 + age * 1.6) * p.scale
        const alpha = age < 0.15 ? age / 0.15 : 1 - (age - 0.15) / 0.85
        const px = 64 * grow
        ctx.globalAlpha = alpha * 0.55
        ctx.drawImage(sprite, x - px / 2, y - px / 2, px, px)
      }
      ctx.globalAlpha = 1
      ctx.globalCompositeOperation = 'source-over'
    }
    raf = requestAnimationFrame(frame)

    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
    }
  }, [sources, ambience])

  return (
    <canvas ref={canvasRef} aria-hidden="true"
            className={`pointer-events-none ${className}`} />
  )
}
