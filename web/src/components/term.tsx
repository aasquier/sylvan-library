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

import { useEffect, useRef, useState } from 'react'
import type { Term as TermData } from '../lib/api'
import { useTerm } from '../lib/glossary'
import { ManaText } from './ui'

/* ------------------------------------------------------------- the popover */

/**
 * Shared shell for both affordances: opens on hover, on focus and on click,
 * closes on Escape and on a click elsewhere.
 *
 * Click as well as hover because hover is not available on a touch screen, and
 * focus as well as click because a keyboard has no pointer. All three set the
 * same piece of state, so there is one open panel and one way to close it.
 */
function Popover({ label, term, children }: {
  label: string
  term: TermData
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(false)
  const [pinned, setPinned] = useState(false)
  const ref = useRef<HTMLSpanElement>(null)

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
        className="cursor-help align-baseline"
        style={{ font: 'inherit', color: 'inherit', textAlign: 'inherit' }}
      >
        {children}
      </button>
      {open && (
        // Fixed width and z-50 so it is never clipped by the card it sits in,
        // and `pointer-events-none` so moving the mouse toward it does not
        // count as leaving the trigger.
        <span
          role="tooltip"
          className="pointer-events-none absolute left-0 top-full z-50 mt-1 block w-64 rounded-lg px-3 py-2 text-left text-xs leading-relaxed shadow-xl"
          style={{
            background: 'var(--surface-1)',
            border: '1px solid var(--hairline)',
            color: 'var(--text-secondary)',
            whiteSpace: 'normal',
            // The simulator's field labels are `uppercase tracking-wide`, and
            // the panel is a descendant of the label — so without this a
            // sentence of help arrived SHOUTED AND LETTER-SPACED. Reset
            // rather than avoided, because sitting inside the label is what
            // puts the mark next to the word it explains.
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
        </span>
      )}
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
    <Popover label={term.term} term={term}>
      <span style={{
        borderBottom: '1px dotted var(--text-muted)',
        textDecoration: 'none',
      }}>
        {children}
      </span>
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
    <Popover label={term.term} term={term}>
      <span
        aria-hidden
        className="ml-1 inline-flex h-3.5 w-3.5 items-center justify-center rounded-full text-[9px] font-bold leading-none"
        style={{
          border: '1px solid var(--text-muted)',
          color: 'var(--text-muted)',
        }}
      >
        ?
      </span>
    </Popover>
  )
}
