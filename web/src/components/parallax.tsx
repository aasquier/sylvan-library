/**
 * The 2.5D player: art + depth map, warped live in WebGL1 (ADR 32's client
 * half). The painting tilts toward the pointer on fine-pointer devices and
 * drifts on a slow closed path everywhere else — no gyro, deliberately: the
 * iOS motion-permission prompt is a dialog, and a decoration must never ask
 * for anything.
 *
 * WebGL1 rather than 2 for the Safari-16.4 floor, and hand-rolled rather
 * than a library because two textures and one quad do not need one. Both
 * textures come from OUR origin (the derivative cache's poster and depth
 * files), never from a Scryfall hotlink — cross-origin pixels would taint
 * the GL context, and the poster is frame zero of the same derivation
 * anyway.
 *
 * The fallback chain is the component's real contract: no WebGL, a lost
 * context, a failed texture — each falls back to `fallback` (the video loop
 * or the still), and reduced motion or ambience-off never mounts GL at all.
 */

import { useEffect, useRef, useState } from 'react'
import { coverTopWindow, reducedMotion } from '../lib/motion'
import { useAmbience } from '../lib/prefs'

export const VERTEX_SHADER = `
attribute vec2 pos;
varying vec2 uv;
void main() {
  uv = pos * 0.5 + 0.5;
  gl_Position = vec4(pos, 0.0, 1.0);
}
`

// Depth displaces the sample point along the camera offset; a small zoom
// keeps the warp from ever sampling past the canvas edge. Lighter depth =
// nearer = moves more, matching `tools/cardmotion/depth.py`'s polarity.
//
// `uvScale`/`uvOffset` are the cover-crop window (see `coverTopWindow`):
// the canvas box and the painting have different aspects, so the shader
// samples only the slice of the texture the still's `object-fit: cover;
// object-position: center top` would show. Displacement happens in box
// space, then maps through the window, so amplitude means the same thing
// whatever the crop.
export const FRAGMENT_SHADER = `
precision mediump float;
varying vec2 uv;
uniform sampler2D art;
uniform sampler2D depth;
uniform vec2 offset;
uniform vec2 uvScale;
uniform vec2 uvOffset;
void main() {
  vec2 centred = (uv - 0.5) / 1.06 + 0.5;
  vec2 base = centred * uvScale + uvOffset;
  float d = texture2D(depth, base).r - 0.5;
  vec2 shifted = (centred + d * offset) * uvScale + uvOffset;
  gl_FragColor = texture2D(art, clamp(shifted, 0.0, 1.0));
}
`


type GL = WebGLRenderingContext

function compile(gl: GL, kind: number, source: string): WebGLShader | null {
  const shader = gl.createShader(kind)
  if (!shader) return null
  gl.shaderSource(shader, source)
  gl.compileShader(shader)
  return gl.getShaderParameter(shader, gl.COMPILE_STATUS) ? shader : null
}

function loadTexture(gl: GL, unit: number, image: HTMLImageElement): void {
  const texture = gl.createTexture()
  gl.activeTexture(gl.TEXTURE0 + unit)
  gl.bindTexture(gl.TEXTURE_2D, texture)
  // Image rows arrive top-first; GL's v axis points up. Without this flip
  // the painting renders upside-down.
  gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, true)
  // Non-power-of-two textures under WebGL1: clamp + linear, no mips.
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
  gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
  gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, image)
}

function fetchImage(src: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const image = new Image()
    image.onload = () => resolve(image)
    image.onerror = () => reject(new Error(`texture failed: ${src}`))
    image.src = src
  })
}

export function ParallaxArt({
  artSrc,
  depthSrc,
  className = '',
  amplitude = 0.014,
  anchor = 'top',
  fallback = null,
}: {
  artSrc: string
  depthSrc: string
  className?: string
  /** Max UV displacement at full tilt — texture-space, not pixels. */
  amplitude?: number
  /** Which band of a painting taller than the box survives the cover
   *  crop: `top` is the hero band's contract (heads sit high), `center`
   *  is the gallery's (it replaces a centre-cropped still). */
  anchor?: 'top' | 'center'
  fallback?: React.ReactNode
}) {
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const [failed, setFailed] = useState(false)
  const [ambience] = useAmbience()
  const [still] = useState(() => reducedMotion())

  const unwelcome = still || !ambience

  useEffect(() => {
    if (unwelcome || failed) return
    const canvas = canvasRef.current
    if (!canvas) return
    const gl = canvas.getContext('webgl')
    if (!gl) {
      setFailed(true)
      return
    }

    let raf = 0
    let disposed = false
    let resize: ResizeObserver | null = null
    // Pointer position in [-1, 1], eased toward with a lerp so the painting
    // settles rather than snaps; the sine path takes over when idle.
    const target = { x: 0, y: 0, active: false }
    const eased = { x: 0, y: 0 }

    const fine = window.matchMedia?.('(pointer: fine)').matches ?? false
    const onMove = (event: PointerEvent) => {
      const box = canvas.getBoundingClientRect()
      if (box.width === 0 || box.height === 0) return
      target.x = ((event.clientX - box.left) / box.width) * 2 - 1
      target.y = ((event.clientY - box.top) / box.height) * 2 - 1
      target.active = true
    }
    const onLeave = () => {
      target.active = false
    }

    void (async () => {
      let art: HTMLImageElement
      let depth: HTMLImageElement
      try {
        ;[art, depth] = await Promise.all([fetchImage(artSrc),
                                           fetchImage(depthSrc)])
      } catch {
        if (!disposed) setFailed(true)
        return
      }
      if (disposed) return

      const vertex = compile(gl, gl.VERTEX_SHADER, VERTEX_SHADER)
      const fragment = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER)
      const program = gl.createProgram()
      if (!vertex || !fragment || !program) {
        setFailed(true)
        return
      }
      gl.attachShader(program, vertex)
      gl.attachShader(program, fragment)
      gl.linkProgram(program)
      if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
        setFailed(true)
        return
      }
      gl.useProgram(program)

      const quad = gl.createBuffer()
      gl.bindBuffer(gl.ARRAY_BUFFER, quad)
      gl.bufferData(gl.ARRAY_BUFFER,
                    new Float32Array([-1, -1, 3, -1, -1, 3]), gl.STATIC_DRAW)
      const pos = gl.getAttribLocation(program, 'pos')
      gl.enableVertexAttribArray(pos)
      gl.vertexAttribPointer(pos, 2, gl.FLOAT, false, 0, 0)

      loadTexture(gl, 0, art)
      loadTexture(gl, 1, depth)
      gl.uniform1i(gl.getUniformLocation(program, 'art'), 0)
      gl.uniform1i(gl.getUniformLocation(program, 'depth'), 1)
      const offsetLoc = gl.getUniformLocation(program, 'offset')
      const uvScaleLoc = gl.getUniformLocation(program, 'uvScale')
      const uvOffsetLoc = gl.getUniformLocation(program, 'uvOffset')

      // The buffer, the viewport and the crop window all follow the box,
      // so a rotated phone or a dragged window re-crops instead of
      // stretching a stale buffer.
      const fit = () => {
        const dpr = Math.min(window.devicePixelRatio || 1, 2)
        canvas.width = Math.max(1, Math.round(canvas.clientWidth * dpr))
        canvas.height = Math.max(1, Math.round(canvas.clientHeight * dpr))
        gl.viewport(0, 0, canvas.width, canvas.height)
        const window_ = coverTopWindow(art.naturalWidth, art.naturalHeight,
                                       canvas.width, canvas.height, anchor)
        gl.uniform2f(uvScaleLoc, window_.scale[0], window_.scale[1])
        gl.uniform2f(uvOffsetLoc, window_.offset[0], window_.offset[1])
      }
      fit()
      if (typeof ResizeObserver !== 'undefined') {
        resize = new ResizeObserver(fit)
        resize.observe(canvas)
      }

      if (fine) {
        canvas.addEventListener('pointermove', onMove)
        canvas.addEventListener('pointerleave', onLeave)
      }

      const started = performance.now()
      const draw = (now: number) => {
        if (disposed) return
        const t = (now - started) / 1000
        // Idle (or coarse-pointer) drift: the same closed ellipse the
        // precomputed loops run, so the two presentations rhyme.
        const idleX = Math.cos(t * 0.45)
        const idleY = 0.6 * Math.sin(t * 0.45)
        const wantX = target.active ? target.x : idleX
        const wantY = target.active ? target.y : idleY
        eased.x += (wantX - eased.x) * 0.06
        eased.y += (wantY - eased.y) * 0.06
        gl.uniform2f(offsetLoc, eased.x * amplitude, eased.y * amplitude)
        gl.drawArrays(gl.TRIANGLES, 0, 3)
        raf = requestAnimationFrame(draw)
      }
      raf = requestAnimationFrame(draw)
    })()

    return () => {
      disposed = true
      cancelAnimationFrame(raf)
      resize?.disconnect()
      canvas.removeEventListener('pointermove', onMove)
      canvas.removeEventListener('pointerleave', onLeave)
    }
  }, [artSrc, depthSrc, amplitude, anchor, unwelcome, failed])

  if (unwelcome || failed) return <>{fallback}</>
  return (
    <canvas ref={canvasRef} className={className} aria-hidden
            style={{ pointerEvents: 'auto', display: 'block',
                     width: '100%', height: '100%' }} />
  )
}
