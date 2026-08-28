/**
 * A mark on the board that can say what it means — to a mouse, to a keyboard,
 * and to a thumb.
 *
 * **This exists because `title` is not an affordance, and this room has now
 * found that out three separate times.** The attribute renders as a browser
 * tooltip on hover and nowhere else: a phone never sees one, a keyboard never
 * sees one — no browser shows `title` on focus — and a screen reader's
 * treatment of it is a per-vendor lottery. So every sentence this board has
 * ever hung on a `title` was readable by exactly one of the three hands that
 * arrive here, and it was always the hand that needed it least. Aaron, three
 * times over four days and most recently 2026-08-28: *"the commander dial's
 * sentence and the keyword help text are both `title` tooltips — no phone,
 * shaky keyboard. Third time this has come up. Making the card's corner marks
 * focusable with a small panel is the change."*
 *
 * So: **the marks become focusable, and what they had to say becomes a small
 * panel.** Hover raises it, focus raises it, and a tap pins it up until
 * something puts it down — all three hands, the same sentence, drawn in this
 * room's materials rather than in the operating system's.
 *
 * ## The three things that make this harder than a tooltip
 *
 * **1. The panel cannot live where the mark lives.** A keyword mark sits on a
 * 58-pixel card inside `.field-row`, which is a scroll container — and
 * `overflow-x: auto` implies `overflow-y: auto`, so a panel parented to the
 * mark is clipped to a lane about forty pixels tall. That is the trap that
 * once ate the card preview whole. `FieldPeek` answered it by portalling to
 * the body and placing itself in viewport coordinates, and this answers it the
 * same way. `position: fixed` alone would not do: these cards sit inside
 * transformed ancestors — a tapped one is literally mid-rotation — and a
 * transform is a containing block for fixed children.
 *
 * **2. Hover is not a state a portalled panel can be styled by.** Trigger and
 * panel are in different trees, so `:hover` on one cannot reach the other and
 * there is no CSS answer at all. The open state is held in React instead,
 * which turns out to be the better shape anyway: it is the only way a *tap*
 * can pin the panel open, and the only way anything else on the board can be
 * told to stand down while it is up (see `onOpen`).
 *
 * **3. A thirteen-pixel drawing is not a thirteen-pixel target.** The marks are
 * drawn at 13px because that is what fits in the corner of a card, and nothing
 * anywhere thinks 13px is a thing a finger can hit. The drawing stays the size
 * it is and the *target* grows underneath it — `.field-hint` in the stylesheet
 * pushes the hit box out sideways, over the painting where there is room, and
 * only to the row's own pitch vertically so that a mark can never steal the
 * tap belonging to the mark below it.
 */

import { type CSSProperties, type ReactNode, useCallback, useEffect, useId,
  useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'

import { HINT_W, type HintPlace, placeHint } from '../lib/hint'

/** What the panel is assumed to be tall before it exists to be measured. Two
 *  lines and a word, which is the ordinary case; the layout effect below
 *  corrects it from the real box before anything is painted. */
const HINT_GUESS_H = 64

/**
 * One mark, and the sentence it is willing to say.
 *
 * The trigger is a real `<button>` rather than a `<span tabindex>`, which buys
 * three things no attribute would: it is in the tab order, Enter and Space
 * work without being written, and a screen reader announces it as something
 * that can be operated instead of as loose text.
 *
 * **Every event on it is stopped from bubbling, and that is load-bearing.**
 * These marks stand inside `.field-card`, which has its own handlers for
 * pointer-up, key-down, focus and blur — so without this a tap on a mark would
 * also lift the whole card into a sheet, and Enter would do both at once. The
 * card keeps every one of those behaviours for the ninety-five per cent of
 * itself that is not a mark.
 */
export function FieldHint({ name, says, note, className = '', style, children,
  onOpen, onShut }: {
  /** What the mark is called: the button's accessible name, and the panel's
   *  first line. */
  name: string
  /** What it means, in one plain sentence. */
  says: string
  /** A second, quieter line — "granted; not printed on this card", which is a
   *  fact about *this* card rather than about the keyword. */
  note?: string
  className?: string
  /** The trigger's own custom properties. The dials in the trench are drawn
   *  from `--bead-full` and friends, which are computed in React because a
   *  `calc()` inside `color-mix()`'s percentage is the one corner of that
   *  function browsers still disagree about. */
  style?: CSSProperties
  children: ReactNode
  /** Called when the panel goes up. A card uses it to stand its own hover
   *  preview down: that preview is a 300px card face placed against the same
   *  card, so without this the two panels arrive on top of each other. */
  onOpen?: () => void
  /** Called when it comes back down, so whatever stood aside can return. */
  onShut?: () => void
}) {
  const id = useId()
  const mark = useRef<HTMLButtonElement>(null)
  const panel = useRef<HTMLSpanElement>(null)
  const [at, setAt] = useState<HintPlace | null>(null)
  /** Held up by a tap or a press rather than by a pointer that is still
   *  there. A pinned panel survives the pointer leaving and is dismissed the
   *  way a zone tray is: a tap elsewhere, or Escape. */
  const [pinned, setPinned] = useState(false)
  /** Which hand last touched this. A touch browser fires a synthetic
   *  `mouseenter` after a tap, so without this the tap's own phantom hover
   *  would re-raise a panel that the same tap had just pinned shut —
   *  `FieldCard` learned the identical lesson one component up. */
  const coarse = useRef(false)

  const open = at !== null
  const raise = useCallback(() => {
    if (!mark.current) return
    const doc = document.documentElement
    const tall = panel.current?.getBoundingClientRect().height
    setAt(placeHint(mark.current.getBoundingClientRect(),
      doc.clientWidth, doc.clientHeight,
      { w: HINT_W, h: tall && tall > 0 ? tall : HINT_GUESS_H }))
  }, [])
  const lower = useCallback(() => { setAt(null); setPinned(false) }, [])

  // **Measured after the panel exists, rather than guessed before it.** The
  // first commit draws it from `HINT_GUESS_H`; this runs before paint and puts
  // it where three lines of *this* sentence actually reach, so nothing is ever
  // seen in the wrong place. `open` in the deps and not `at`, which would be a
  // loop — the only thing that has to re-measure is the panel arriving.
  useLayoutEffect(() => { if (open) raise() }, [open, raise])

  // Tell whoever asked. An effect rather than calls inside the handlers, so
  // the pair cannot fall out of step with the state that is actually
  // rendered: an `onShut` that was called on a path which then failed to
  // close is what leaves a card's preview muted for the rest of the match.
  useEffect(() => {
    if (!open) return
    onOpen?.()
    return () => onShut?.()
    // Deliberately not depending on the callbacks. They are rebuilt on every
    // render by the card that owns them, and depending on them would fire the
    // pair again on every beat of the reel.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  // **A panel placed in viewport coordinates is wrong the moment the page
  // moves under it**, and this room scrolls while a match is playing. Hung
  // only while one is up, so a board of forty cards wearing a hundred and
  // sixty marks costs nothing at rest — the same discipline `FieldCard` keeps
  // for its preview and `FieldPile` for its tray.
  useEffect(() => {
    if (!open) return
    const away = (e: PointerEvent) => {
      const hit = e.target as Element | null
      // Inside this mark is not outside it. **This is the trap**: the tap that
      // closes a pinned panel is the button's own `onClick`, and a document
      // listener that skipped this test would shut it first and let the click
      // immediately re-open it.
      if (hit && mark.current?.contains(hit)) return
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

  return (
    <>
      <button type="button" ref={mark}
              className={`field-hint ${className}`.trim()}
              style={style}
              aria-label={name}
              aria-describedby={open ? id : undefined}
              aria-expanded={open}
              onPointerDown={(e) => {
                coarse.current = e.pointerType !== 'mouse'
                e.stopPropagation()
              }}
              onPointerUp={(e) => e.stopPropagation()}
              onClick={(e) => {
                e.stopPropagation()
                if (pinned) { lower(); return }
                setPinned(true)
                raise()
              }}
              onMouseEnter={() => { if (!coarse.current) raise() }}
              onMouseLeave={() => { if (!pinned) setAt(null) }}
              // **Stopped, like the rest.** `onFocus`/`onBlur` are `focusin`
              // and `focusout`, which bubble — so without this, reaching a
              // mark with the keyboard also arms the card's own hover preview
              // underneath the panel, and leaving it disarms things the mark
              // never armed. The card is not being focused; one of its chips
              // is.
              onFocus={(e) => { e.stopPropagation(); raise() }}
              onBlur={(e) => { e.stopPropagation(); if (!pinned) lower() }}
              onKeyDown={(e) => {
                // Escape puts the panel down without putting the card down.
                // The card above answers Escape too — it closes its sheet, and
                // the pile under it closes its tray — and one press must not
                // mean all three.
                if (e.key === 'Escape' && open) {
                  e.stopPropagation()
                  lower()
                  return
                }
                if (e.key === 'Enter' || e.key === ' ') e.stopPropagation()
              }}>
        {children}
      </button>
      {open && createPortal(
        <span ref={panel} id={id} role="tooltip"
              className={`field-hint-panel${at.under ? ' is-under' : ''}`}
              style={{ left: at.left, top: at.top, width: at.width }}>
          <span className="field-hint-name">{name}</span>
          <span className="field-hint-says">{says}</span>
          {note && <span className="field-hint-note">{note}</span>}
        </span>,
        document.body)}
    </>
  )
}
