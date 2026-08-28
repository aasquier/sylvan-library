/**
 * A creature's gear, fanned out under the hand that reached for it.
 *
 * **The stack opens on a hover now, and riffles under the pointer.** What was
 * there before was a modal: the tucked corners under a creature said *there is
 * something here*, and finding out what it was meant clicking, reading a sheet
 * that covered the board, stepping it with a rail, and closing it again.
 * Aaron, 2026-08-28: *"Still not in love with the equipment carousel when I
 * click on a creature with auras or equipment. It requires some awkward
 * clicking. I had envisioned a hover opens the stack of them into an actual
 * live carousel you can thumb left or right through easily."*
 *
 * So: **the pointer is the thumb.** Hovering the creature fans its cards out
 * above it, overlapped, each showing its left third. Whichever one the pointer
 * is over lifts out of the fan and is read. Sliding across riffles them, with
 * no click anywhere in the gesture — which is exactly what a hand does with a
 * fan of cards it is holding.
 *
 * ## The three things that make this harder than it sounds
 *
 * **1. The fan cannot live where the creature lives.** A geared creature sits
 * in `.field-row`, which is a scroll container — and `overflow-x: auto` implies
 * `overflow-y: auto`, so anything parented to the card is clipped to a lane
 * about eighty pixels tall. That is the trap that has now eaten the card
 * preview, the zone trays and the keyword panels. This portals to the body and
 * places itself in viewport coordinates, for the same reason and by the same
 * arithmetic (`lib/gearfan.ts`).
 *
 * **2. Hover has to survive the journey from the creature to the fan.** The
 * two are in different trees, so there is no CSS answer at all, and a naive
 * `onMouseLeave` closes the fan the instant the pointer sets off toward it.
 * The open state is React's, and both the trigger and the fan hold it up:
 * leaving either one *schedules* a close, and entering either one cancels it.
 * The delay is what carries the pointer across the ten-pixel gap between them.
 *
 * The board's own reason this was a click before is written into `FieldCard`:
 * *"nothing you have to keep hovering to keep alive can be stepped through"*.
 * That was true of a thing you step through with a **rail**. It is not true of
 * one you step through by moving the pointer, because the pointer never has to
 * leave.
 *
 * **3. A phone has no hover, and this must not become mouse-only** — the fault
 * this room has now found four times. A tap pins the fan open, a tap on a card
 * raises that card, and a tap anywhere else puts it down. The keyboard gets
 * the same fan on focus, with the arrow keys for the thumb.
 */

import { type CSSProperties, useCallback, useEffect, useId, useRef,
  useState } from 'react'
import { createPortal } from 'react-dom'

import { FAN_LIFT, FAN_SLICE, type FanPlace, fanHeight, placeFan }
  from '../lib/gearfan'

/** One card in the fan: what to draw, and what to say about it. */
export interface FannedCard {
  id: number
  name: string
  image: string
  /** What this card is attached to, or null for the creature itself. The
   *  caption says it, because a fan of four pictures does not otherwise
   *  distinguish the host from the things riding on it. */
  on: string | null
}

/** How long the pointer has to cross the gap between the creature and its
 *  fan, and to cross between two cards without the fan flickering.
 *
 *  120ms is above the time a deliberate move takes and below the time a
 *  *decision* takes, so a pointer travelling to the fan keeps it and a pointer
 *  travelling past it does not. */
const LINGER = 120

export function GearFan({ cards, label, children }: {
  /** The host and everything on it, host first — the order `BoardCard`'s own
   *  `attachments` is built in, which is the order they were attached. */
  cards: FannedCard[]
  /** What the group is called, for a reader: the creature and its gear. */
  label: string
  /** The tucked stack as the board already draws it. The fan is raised *by*
   *  this and drawn somewhere else entirely. */
  children: React.ReactNode
}) {
  const id = useId()
  const host = useRef<HTMLDivElement>(null)
  const fan = useRef<HTMLDivElement>(null)
  const [at, setAt] = useState<FanPlace | null>(null)
  /** Which card is out of the fan and being read. Zero is the creature, which
   *  is the right thing to open on: somebody who hovered a creature was
   *  looking at the creature. */
  const [up, setUp] = useState(0)
  /** Held open by a tap rather than by a pointer that is still there. */
  const [pinned, setPinned] = useState(false)
  /** Which hand last touched this. A touch browser fires a synthetic
   *  `mouseenter` after a tap, so without this the tap's own phantom hover
   *  would re-open a fan the same tap had just pinned shut — `FieldCard` and
   *  `FieldHint` both learned this, one component up and one across. */
  const coarse = useRef(false)
  /** The pending close. A ref because it is not drawn, and because a
   *  re-render mid-journey would otherwise lose the timer and strand the fan
   *  open for the rest of the match. */
  const closing = useRef<ReturnType<typeof setTimeout> | null>(null)

  const open = at !== null

  const raise = useCallback(() => {
    if (!host.current || cards.length < 2) return
    const doc = document.documentElement
    setAt(placeFan(host.current.getBoundingClientRect(),
      doc.clientWidth, doc.clientHeight, cards.length))
  }, [cards.length])

  const lower = useCallback(() => {
    setAt(null)
    setPinned(false)
    setUp(0)
  }, [])

  /** Stay open a moment longer, because the pointer may be on its way here. */
  const hold = useCallback(() => {
    if (closing.current) { clearTimeout(closing.current); closing.current = null }
  }, [])

  const leave = useCallback(() => {
    if (pinned) return
    hold()
    closing.current = setTimeout(() => { closing.current = null; lower() }, LINGER)
  }, [pinned, hold, lower])

  // Nothing outlives the component. A fan whose creature was scrubbed off the
  // board while a close was pending would otherwise set state on a corpse.
  useEffect(() => () => { if (closing.current) clearTimeout(closing.current) }, [])

  // **A fan placed in viewport coordinates is wrong the moment the page moves
  // under it**, and this room scrolls while a match is playing. Hung only
  // while one is open, so a board of forty creatures costs nothing at rest —
  // the same discipline `FieldHint` keeps for its panel and `FieldPile` for
  // its tray.
  useEffect(() => {
    if (!open) return
    const away = (e: PointerEvent) => {
      const hit = e.target as Element | null
      // Inside the creature or inside the fan is not outside them. **This is
      // the trap**: the tap that pins the fan is the group's own handler, and
      // a document listener without this test would close it first and let the
      // same tap immediately re-open it.
      if (hit && (host.current?.contains(hit) || fan.current?.contains(hit))) {
        return
      }
      lower()
    }
    const key = (e: KeyboardEvent) => { if (e.key === 'Escape') lower() }
    document.addEventListener('pointerdown', away, true)
    document.addEventListener('keydown', key)
    window.addEventListener('scroll', lower, true)
    window.addEventListener('resize', lower)
    return () => {
      document.removeEventListener('pointerdown', away, true)
      document.removeEventListener('keydown', key)
      window.removeEventListener('scroll', lower, true)
      window.removeEventListener('resize', lower)
    }
  }, [open, lower])

  const step = useCallback((d: number) => {
    setUp((n) => Math.max(0, Math.min(cards.length - 1, n + d)))
  }, [cards.length])

  // Nothing to fan: a creature carrying nothing is drawn by the board exactly
  // as it always was, with no handlers and no portal.
  if (cards.length < 2) return <>{children}</>

  const read = cards[up] ?? cards[0] as FannedCard

  return (
    <>
      {/* **No tab stop of its own.** Every card inside is already focusable and
          `focusin` bubbles, so tabbing to a geared creature raises its fan
          without adding a second stop to the order — and the ring a keyboard
          user sees is the card's own. `aria-expanded` is gone with the tab
          stop it belonged to: it is not a thing a `role="group"` may carry,
          and what a reader actually needs is the caption below, which is a
          live region and says the card's name as it changes. */}
      <div ref={host} className={`field-fanned${open ? ' is-open' : ''}`}
           role="group" aria-label={label}
           onPointerDown={(e) => { coarse.current = e.pointerType !== 'mouse' }}
           onPointerEnter={(e) => {
             if (e.pointerType !== 'mouse') return
             hold()
             raise()
           }}
           onPointerLeave={leave}
           onClick={(e) => {
             // A tap is the touch hand's way in, and the only way in that
             // *stays*. It deliberately does not stop propagation: the card
             // underneath still lifts its own sheet, which is the deep look
             // and is still worth having.
             if (!coarse.current) return
             e.stopPropagation()
             if (pinned) { lower(); return }
             setPinned(true)
             raise()
           }}
           onFocus={() => { hold(); raise() }}
           onBlur={() => leave()}
           onKeyDown={(e) => {
             if (e.key === 'Escape' && open) { e.stopPropagation(); lower(); return }
             if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
             // The arrows are the keyboard's thumb. Stopped and prevented:
             // the row underneath scrolls on an arrow key, and riffling a fan
             // must not also slide the lane behind it.
             e.preventDefault()
             e.stopPropagation()
             if (!open) raise()
             step(e.key === 'ArrowRight' ? 1 : -1)
           }}>
        {children}
      </div>
      {open && createPortal(
        <div ref={fan} id={id} className="field-fan"
             style={{
               left: at.left,
               top: at.top,
               width: at.width,
               height: fanHeight(at.cardW),
               '--fan-card': `${at.cardW}px`,
               '--fan-slice': `${at.cardW * FAN_SLICE}px`,
               '--fan-lift': `${FAN_LIFT}px`,
             } as CSSProperties}
             onPointerEnter={hold}
             onPointerLeave={leave}>
          {cards.map((c, i) => (
            <span key={c.id}
                  className={`field-fan-card${i === up ? ' is-up' : ''}`}
                  style={{
                    '--i': i,
                    '--z': i === up ? cards.length + 1 : i,
                  } as CSSProperties}
                  // **The pointer is the thumb, and this is the whole of it.**
                  // No click, no rail, no drag: crossing a card raises it.
                  onPointerEnter={(e) => {
                    if (e.pointerType !== 'mouse') return
                    setUp(i)
                  }}
                  // And a tap, for the hand that has no pointer to cross with.
                  onClick={(e) => { e.stopPropagation(); setUp(i) }}
                  aria-hidden="true">
              <img src={c.image} alt="" draggable={false} />
            </span>
          ))}
          {/* Said in words as well as drawn, and said again every time the
              raised card changes. The picture is the answer for somebody who
              can see it; the name is the answer for everybody else, and the
              *"on Bronzehide Lion"* line is the one fact the pictures cannot
              carry — which of these four is the creature and which are riding
              on it.

              **One element, visible and announced**, which is `CardSheet`'s
              own arrangement two files over. A hidden duplicate beside a
              visible caption is two sentences to keep in step, and the pair
              stops being in step the first time one of them is edited. The
              cards themselves are `aria-hidden`: they are pictures with no
              text, and a reader given four unlabelled images has been given
              nothing. */}
          <span className="field-fan-say" aria-live="polite">
            <span className="field-fan-name">{read.name}</span>
            {read.on && <span className="field-fan-on">on {read.on}</span>}
          </span>
        </div>,
        document.body)}
    </>
  )
}
