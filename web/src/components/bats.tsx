/**
 * A flurry of bats (second punch list of 2026-08-15, item 8): canvas
 * silhouettes crossing a band of painting, for the keeper's dossier.
 *
 * Not clip art, and the reasoning is worth a sentence: a bat at dusk *is* a
 * silhouette — a black flicker with a wingbeat — so silhouettes drawn at
 * low alpha over a real painting read as the thing itself, where a detailed
 * cartoon bat would read as a sticker. Each bat is a body and two wings
 * drawn with quadratic curves whose control points swing on a sine — the
 * flap — following a wobbling path across the band and gone. A flurry runs
 * when asked (the panel opening) and occasionally after, never constantly:
 * bats startle, they do not hover.
 *
 * Same three exits as the vapour: `prefers-reduced-motion`, the ambience
 * switch, and a canvas without a 2d context all render a quiet nothing.
 */

import { useEffect, useRef } from 'react'
import { useAmbience } from '../lib/prefs'

interface Bat {
  born: number
  life: number
  fromLeft: boolean
  y0: number
  wobble: number
  wobbleFreq: number
  flapFreq: number
  size: number
  speed: number
}

function drawBat(ctx: CanvasRenderingContext2D, x: number, y: number,
                 size: number, flap: number): void {
  // flap in [-1, 1]: wingtips swing above and below the body line.
  const tip = flap * size * 0.55
  const mid = flap * size * 0.2
  ctx.beginPath()
  // Left wing: out to the tip, back to the body through a scalloped edge.
  ctx.moveTo(x, y)
  ctx.quadraticCurveTo(x - size * 0.5, y - mid - size * 0.3, x - size, y - tip)
  ctx.quadraticCurveTo(x - size * 0.55, y + size * 0.18 - mid, x - size * 0.3, y + size * 0.08)
  ctx.quadraticCurveTo(x - size * 0.15, y + size * 0.16, x, y + size * 0.1)
  // Right wing, mirrored.
  ctx.quadraticCurveTo(x + size * 0.15, y + size * 0.16, x + size * 0.3, y + size * 0.08)
  ctx.quadraticCurveTo(x + size * 0.55, y + size * 0.18 - mid, x + size, y - tip)
  ctx.quadraticCurveTo(x + size * 0.5, y - mid - size * 0.3, x, y)
  ctx.closePath()
  ctx.fill()
}

export function BatFlurry({ trigger, className = '' }: {
  /** Increment to send a flurry across; each change releases one. */
  trigger: number
  className?: string
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const batsRef = useRef<Bat[]>([])
  const [ambience] = useAmbience()

  useEffect(() => {
    if (!ambience || trigger === 0) return
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
    const now = performance.now()
    const count = 4 + Math.floor(Math.random() * 4)
    const fromLeft = Math.random() < 0.5
    for (let i = 0; i < count; i++) {
      batsRef.current.push({
        born: now + i * 140 + Math.random() * 120,
        life: 2600 + Math.random() * 1400,
        fromLeft,
        y0: 0.15 + Math.random() * 0.5,
        wobble: 8 + Math.random() * 18,
        wobbleFreq: 3 + Math.random() * 3,
        flapFreq: 9 + Math.random() * 5,
        size: 7 + Math.random() * 7,
        speed: 0.9 + Math.random() * 0.4,
      })
    }
  }, [trigger, ambience])

  useEffect(() => {
    if (!ambience) return
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
    const canvas = canvasRef.current
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

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

    let raf = 0
    const frame = (now: number) => {
      raf = requestAnimationFrame(frame)
      if (w === 0 || h === 0) return
      ctx.clearRect(0, 0, w, h)
      const bats = batsRef.current
      for (let i = bats.length - 1; i >= 0; i--) {
        const bat = bats[i]
        if (!bat) continue
        const t = (now - bat.born) / bat.life
        if (t >= 1) {
          bats.splice(i, 1)
          continue
        }
        if (t < 0) continue
        const progress = t * bat.speed
        const x = bat.fromLeft ? progress * (w + 60) - 30
          : w + 30 - progress * (w + 60)
        const y = bat.y0 * h + Math.sin(t * bat.wobbleFreq * Math.PI * 2)
          * bat.wobble
        const flap = Math.sin(t * bat.flapFreq * Math.PI * 2)
        // Fading in and out at the edges keeps the entrance from popping.
        const alpha = Math.min(1, t / 0.12, (1 - t) / 0.15)
        ctx.fillStyle = `rgba(8, 10, 12, ${0.72 * alpha})`
        drawBat(ctx, x, y, bat.size, flap)
      }
    }
    raf = requestAnimationFrame(frame)
    return () => {
      cancelAnimationFrame(raf)
      ro.disconnect()
    }
  }, [ambience])

  return (
    <canvas ref={canvasRef} aria-hidden="true"
            className={`pointer-events-none ${className}`} />
  )
}
