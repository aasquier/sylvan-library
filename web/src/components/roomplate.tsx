/**
 * A room that is footage: the loop across the whole viewport, a veil for the
 * words to sit on, and the room's own sound.
 *
 * The painting's rooms (`forest.tsx`'s `SceneBackdrop`) wash a palette over
 * the page and hang the picture in the margins; this room IS the picture. So
 * there is less here, on purpose — Aaron's ruling on the first three clips
 * was that the footage carries the room and nothing much should be laid on
 * top of it. One veil, darker at the edges and the foot, is the whole
 * treatment: a shade laid over the picture (commandment 19's shape, kept out
 * of habit — the picture is ours), never a level in the file.
 *
 * Portalled onto `document.body` for `SceneBackdrop`'s reason, verbatim: the
 * routed page animates a `transform`, and a transformed ancestor is the
 * containing block for every `position: fixed` descendant, so rendered in
 * place this would size itself to a section rather than to the window.
 *
 * `art` mode, so reduced motion and the ambience switch both fall back to the
 * still: a room removed is a conversation held in a void, and the still is
 * the picture the room always was.
 */

import { createPortal } from 'react-dom'
import type { RoomFootage } from '../lib/roomfootage'
import { RoomTone } from './roomtone'
import { VideoBackdrop } from './videofx'

export function RoomPlate({ footage }: { footage: RoomFootage }) {
  if (typeof document === 'undefined') return null
  return createPortal(
    <div className="room-plate" aria-hidden="true">
      <VideoBackdrop webmSrc={footage.webm} mp4Src={footage.mp4}
                     poster={footage.still} mode="art"
                     className="room-plate-film"
                     fallback={<img className="room-plate-film"
                                    src={footage.still} alt="" />} />
      <span className="room-plate-veil" />
      <RoomTone src={footage.tone} />
    </div>,
    document.body,
  )
}
