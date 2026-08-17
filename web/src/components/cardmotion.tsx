/**
 * A commander's painting, brought to life when its derivative exists
 * (ADR 32) and exactly the still it always was when it does not.
 *
 * One status fetch, no polling: derivatives are made by a dev-machine run
 * and pushed to the instance, never generated on request, so the answer
 * cannot change while the page is open. `ready: false` — or any error at
 * all — renders the still, which is the page as it was before the tier
 * existed. The decoration must never be the reason a page looks broken.
 *
 * When a derivative is ready the presentation ladder is:
 *   depth map + WebGL        -> ParallaxArt (tilts toward the pointer)
 *   loop files, no GL/depth  -> VideoBackdrop in `art` mode (the baked path)
 *   anything less            -> the still
 */

import { useCardMotion } from '../lib/motion'
import { ParallaxArt } from './parallax'
import { VideoBackdrop } from './videofx'

export function CommanderMotion({
  oracleId,
  art,
  still,
  className = '',
}: {
  oracleId: string | null | undefined
  /** The crop the page is showing — the deck's chosen printing when it has
   *  one. The server matches derivatives against it, so a swapped painting
   *  falls back to the (correct) still rather than breathing as the old
   *  printing. */
  art?: string | null
  /** The existing still presentation, rendered verbatim as the floor. */
  still: React.ReactNode
  className?: string
}) {
  const motion = useCardMotion(oracleId, ['depth-drift', 'slow-pan'], art)

  if (!motion?.ready || !motion.urls) return <>{still}</>
  const { webm, mp4, poster, depth } = motion.urls

  if (depth && poster) {
    return (
      <div className={className}>
        <ParallaxArt artSrc={poster} depthSrc={depth}
                     fallback={
                       <VideoBackdrop webmSrc={webm} mp4Src={mp4}
                                      poster={poster} mode="art"
                                      className="deck-hero-video"
                                      fallback={still} />
                     } />
      </div>
    )
  }
  return (
    <div className={className}>
      <VideoBackdrop webmSrc={webm} mp4Src={mp4} poster={poster} mode="art"
                     className="deck-hero-video" fallback={still} />
    </div>
  )
}
