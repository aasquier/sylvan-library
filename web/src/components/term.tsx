/**
 * The vocabulary, wherever a word turns up.
 *
 * Two surfaces over one table, which is the same arrangement the colors table
 * has with the wheel and the carousel. `/learn` renders every term at length;
 * `<Term>` and `<HelpTip>` render one of them at a sentence, next to the thing
 * being named. Neither holds a definition of its own — both look the key up in
 * what `/api/glossary` served, so the served glossary stays the one authority
 * and a term explained in two places cannot say two things.
 *
 * The fetch behind both is memoised at module scope in `lib/glossary.ts` — one
 * promise for the page, so a screen with a dozen marks on it costs one
 * request.
 *
 * A term whose key is missing renders as plain text with no affordance rather
 * than as a control that opens nothing. That is what makes it safe to mark up
 * a word before its entry is written.
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { Term as TermData } from '../lib/api'
import { useTerm } from '../lib/glossary'
import { type HintPlace, TIP_W, placeTip } from '../lib/hint'
import { categoryTerm } from '../lib/mtg'
import { ManaText } from './ui'

/** What the panel is assumed to be tall before it exists to be measured; the
 *  layout effect corrects it from the real box before anything is painted.
 *  Three lines of a definition at this width, which is the ordinary case. */
const TIP_GUESS_H = 76

/* ------------------------------------------------------------- the popover */

/**
 * Shared shell for both affordances: opens on hover, on focus and on click,
 * closes on Escape and on a click elsewhere.
 *
 * Click as well as hover because hover is not available on a touch screen, and
 * focus as well as click because a keyboard has no pointer. All three set the
 * same piece of state, so there is one open panel and one way to close it.
 *
 * **The panel is portalled and placed in viewport coordinates, and that is a
 * bug fix rather than a refactor.** It used to be an `absolute; left: 0;
 * w-64` child of the trigger, which is correct for a mark on the left of a
 * wide screen and, for a mark halfway across a phone, is 256 pixels of panel
 * starting past the middle of a 375-pixel window: measured on the deck page at
 * 375 with one category's help open, the **document was 34 pixels wider than
 * the window** and the page scrolled sideways. `lib/hint.ts` already held the
 * arithmetic that does not do that — it is the file that exists because a
 * suite cannot see layout — so this uses it (`placeTip`, below-first, the
 * other way up from the board's marks).
 *
 * Portalled rather than merely `position: fixed`, for the reason
 * `components/hint.tsx` writes out at length: a transformed ancestor is a
 * containing block for a fixed child, and these marks sit inside several.
 */
function Popover({ label, term, children, trigger = '' }: {
  label: string
  term: TermData
  children: React.ReactNode
  /** A class for the button itself. The two callers below want different
   *  hands: a word in a sentence underlines, a pip beside a label fills. Both
   *  need somewhere a `:hover` can actually reach, which an inline style is
   *  not (commandment 17, and the reason a hundred dull controls happened). */
  trigger?: string
}) {
  const [open, setOpen] = useState(false)
  const [pinned, setPinned] = useState(false)
  const [at, setAt] = useState<HintPlace | null>(null)
  const ref = useRef<HTMLSpanElement>(null)
  const panel = useRef<HTMLSpanElement>(null)

  // Measured before paint, from the real panel where there is one and from a
  // guess on the first frame — the shape `FieldHint` uses, and the reason it
  // never appears in the wrong place and then hops.
  useLayoutEffect(() => {
    if (!open || !ref.current) { setAt(null); return }
    const tall = panel.current?.getBoundingClientRect().height || TIP_GUESS_H
    setAt(placeTip(ref.current.getBoundingClientRect(),
      document.documentElement.clientWidth,
      document.documentElement.clientHeight,
      { w: TIP_W, h: tall }))
  }, [open, term.key])

  useEffect(() => {
    if (!pinned) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { setPinned(false); setOpen(false) }
    }
    const onClick = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) {
        setPinned(false)
        setOpen(false)
      }
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mousedown', onClick)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mousedown', onClick)
    }
  }, [pinned])

  return (
    <span ref={ref} className="relative inline-block">
      <button
        type="button"
        aria-label={`What is ${label}?`}
        aria-expanded={open}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => !pinned && setOpen(false)}
        onFocus={() => setOpen(true)}
        onBlur={() => !pinned && setOpen(false)}
        onClick={() => { setPinned((p) => !p); setOpen(true) }}
        className={`cursor-help align-baseline ${trigger}`}
        style={{ font: 'inherit', color: 'inherit', textAlign: 'inherit' }}
      >
        {children}
      </button>
      {open && createPortal(
        // `pointer-events-none` so moving the mouse toward the panel does not
        // count as leaving the trigger — and, now that the panel is portalled,
        // so that a click on it can never be read as a click *outside* the
        // trigger by the dismissal listener above.
        <span
          ref={panel}
          role="tooltip"
          className="pointer-events-none fixed z-50 block rounded-lg px-3 py-2 text-left text-xs leading-relaxed shadow-xl"
          style={{
            left: at ? at.left : -9999,
            top: at ? at.top : -9999,
            width: at ? at.width : TIP_W,
            background: 'var(--surface-1)',
            border: '1px solid var(--hairline)',
            color: 'var(--text-secondary)',
            whiteSpace: 'normal',
            // The panel used to be a descendant of the simulator's field
            // labels, which are `uppercase tracking-wide`, and a sentence of
            // help arrived SHOUTED AND LETTER-SPACED. It is a child of the
            // body now and inherits none of that — the resets stay because a
            // panel that states its own type is a panel that cannot be
            // re-broken by whatever it is portalled out of next.
            textTransform: 'none',
            letterSpacing: 'normal',
            fontWeight: 400,
          }}
        >
          <span className="block font-semibold"
                style={{ color: 'var(--text-primary)' }}>
            {term.term}
          </span>
          <span className="mt-0.5 block">
            <ManaText>{term.short}</ManaText>
          </span>
        </span>,
        document.body)}
    </span>
  )
}

/**
 * A word in prose, with its definition one hover away.
 *
 * The dotted underline is the affordance and it is only drawn once the term
 * has resolved — marking a word up before its entry exists is meant to be
 * free, so an unknown key degrades to the words themselves.
 */
export function Term({ name, children }: {
  name: string
  children: React.ReactNode
}) {
  const term = useTerm(name)
  if (!term) return <>{children}</>
  return (
    <Popover label={term.term} term={term} trigger="term-trigger">
      <span className="term-word">{children}</span>
    </Popover>
  )
}

/**
 * A help affordance beside a control, rather than inside a sentence.
 *
 * The simulator's parameters are the case this exists for: "Min mana pieces"
 * is a label with no sentence around it to underline, so the mark goes next to
 * it. Same table, same popover, different attachment.
 */
export function HelpTip({ name }: { name: string }) {
  const term = useTerm(name)
  if (!term) return null
  return (
    // **The smallest control in the app was the beginner's one.** Measured on
    // the deployed Coliseum: 18x17px, no focus ring, and its border and ink
    // written inline where no `:hover` could ever reach them — in 44 places.
    // WCAG's own floor for a target is 24x24 (2.5.8) and it missed that by a
    // third. The mark itself grows a little, which a help affordance can
    // afford to do, and `.help-pip` reaches past it for the rest of the
    // target, so nothing around it moves.
    <Popover label={term.term} term={term} trigger="help-pip">
      <span aria-hidden
            className="pip-mark ml-1 inline-flex h-[1.1rem] w-[1.1rem]
                       items-center justify-center rounded-full
                       text-[0.62rem] font-bold leading-none">
        ?
      </span>
    </Popover>
  )
}

/**
 * The same mark, beside the name of a shelf in the 99.
 *
 * "Interaction", "Ramp", "Payoffs" — the deck model's own vocabulary, and a
 * newcomer's first deck page shows them a dozen of these words with nothing
 * offered (Aaron, 2026-09-27: *"it would be nice if there was help text for
 * the genres, like interaction"*). Every one of them now has an entry, and
 * `lib/mtg.ts`'s `categoryTerm` is the one place a category's key becomes a
 * glossary key.
 *
 * A separate component rather than a computed key written out at each call
 * site, for the reason `glossarykeys_test.go` exists: a key the sweep cannot
 * read is a key that fails silently, so there is exactly one such key in the
 * whole tree and it lives here, where that guard's exemption can name it and
 * `categorywords_test.go` can hold the whole category list to the served
 * table behind it.
 *
 * It renders beside the shelf's name rather than under it, because a group
 * header on this page is already a **fold** — a `<button>` — and a control
 * inside a control is not markup a browser will honour.
 */
export function CategoryHelp({ category }: { category: string }) {
  return <HelpTip name={categoryTerm(category)} />
}
