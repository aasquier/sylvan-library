/**
 * Video as decoration: the `<video>` element wearing the forest layer's rules.
 *
 * Two modes, and the difference is what removal means (web/README):
 *
 * - `ambience` — the loop is weather. Reduced motion or the ambience pref
 *   removes it entirely; "frozen weather is a smudge", so there is no poster
 *   fallback, just nothing.
 * - `art` — the loop is a painting brought to life. Reduced motion falls back
 *   to the still image, which is the page as it always was, never a
 *   freeze-frame of a video that failed to play.
 *
 * WebM first, MP4 second: both play everywhere ≥ the Safari 16.4 floor, so
 * source order just means the smaller file wins where both are understood.
 * The element pauses itself off-screen — a background tab or a scrolled-away
 * hero should not keep a decoder warm (the rAF-dt-clamp discipline, video-
 * shaped).
 */

import { useEffect, useRef, useState } from 'react'
import { reducedMotion } from '../lib/motion'
import { useAmbience } from '../lib/prefs'

export function VideoBackdrop({
  webmSrc,
  mp4Src,
  poster,
  mode,
  className = '',
  fallback = null,
}: {
  webmSrc?: string
  mp4Src?: string
  poster?: string
  mode: 'ambience' | 'art'
  className?: string
  /** What `art` mode shows when motion is unwelcome: the still, as JSX. */
  fallback?: React.ReactNode
}) {
  const [ambience] = useAmbience()
  const videoRef = useRef<HTMLVideoElement>(null)
  // Read once per mount: a live change of the OS setting re-renders on the
  // next navigation, which is the same deal the canvas layers offer.
  const [still] = useState(() => reducedMotion())

  const unwelcome = still || !ambience

  useEffect(() => {
    const video = videoRef.current
    if (!video || typeof IntersectionObserver === 'undefined') return
    const observer = new IntersectionObserver((entries) => {
      for (const entry of entries) {
        if (!entry) continue
        if (entry.isIntersecting) void video.play().catch(() => undefined)
        else video.pause()
      }
    })
    observer.observe(video)
    return () => observer.disconnect()
  }, [unwelcome])

  if (unwelcome) return mode === 'art' ? <>{fallback}</> : null
  if (!webmSrc && !mp4Src) return mode === 'art' ? <>{fallback}</> : null

  return (
    <video
      ref={videoRef}
      className={className}
      autoPlay
      muted
      loop
      playsInline
      preload="metadata"
      poster={poster}
      aria-hidden
      // Decorative, never load-bearing: if the sources fail the element
      // shows its poster (art) or nothing visible (ambience), and no error
      // reaches the user.
      style={{ pointerEvents: 'none' }}
    >
      {webmSrc && <source src={webmSrc} type="video/webm" />}
      {mp4Src && <source src={mp4Src} type="video/mp4" />}
    </video>
  )
}
