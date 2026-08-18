/**
 * The library whispers: a sprout in the corner of every page that offers one
 * glossary entry at a time.
 *
 * The facts are `glossary.py`'s — checked-in prose, served free by
 * `/api/glossary` with no card pool and no key — which is what makes this
 * safe to hang on every page: it can never say anything the repo did not
 * write down, and it costs one cached fetch the tooltips were already
 * paying for (`useGlossary` is memoised at module scope).
 *
 * Deliberately a door and never an interruption. It does not open itself,
 * ever — an uninvited pop-up is the one way to make people hate a thing
 * that is supposed to be a gift. The sprout bobs; that is the whole
 * invitation. Esc and clicking elsewhere both close it.
 */

import { useEffect, useRef, useState } from 'react'
import { useGlossary } from '../lib/glossary'
import { ManaText } from './ui'

/** The sprout itself: two leaves off a stem, drawn on `currentColor`. */
function Sprout() {
  return (
    <svg width="22" height="22" viewBox="0 0 22 22" fill="none" aria-hidden>
      <path d="M11 20 C 11 15 11 12 11 9" stroke="currentColor"
            strokeWidth="1.6" strokeLinecap="round" />
      <path d="M11 12 C 6 12 3.5 9 3 5 C 8 5 10.5 8 11 12 Z"
            fill="currentColor" opacity="0.85" />
      <path d="M11 9 C 15.5 9 18 6.5 18.5 3 C 14 3 11.5 5.5 11 9 Z"
            fill="currentColor" />
    </svg>
  )
}

export function LibraryWhisper() {
  const glossary = useGlossary()
  const [open, setOpen] = useState(false)
  // Advanced by "another", so each click walks the shuffled order rather
  // than re-rolling — a re-roll repeats itself often enough to feel broken.
  const [step, setStep] = useState(0)
  // One random offset per mount: a different first whisper each visit, the
  // same one for the life of the page.
  const offset = useRef(Math.floor(Math.random() * 997))
  const panel = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    const onClick = (e: MouseEvent) => {
      if (panel.current && !panel.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('keydown', onKey)
    document.addEventListener('mousedown', onClick)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('mousedown', onClick)
    }
  }, [open])

  const terms = glossary?.terms ?? []
  // Nothing to whisper until the glossary lands; a button that opens onto
  // "loading…" is a promise made before checking it can be kept.
  if (terms.length === 0) return null

  // A coprime stride visits every term before any repeats. 89 is prime and
  // the glossary is nowhere near a multiple of it.
  const term = terms[(offset.current + step * 89) % terms.length]
  const section = glossary?.sections.find((s) => s.key === term?.section)

  return (
    <div ref={panel} className="library-whisper">
      {open && term && (
        <div role="note" aria-label="A whisper from the library"
             className="whisper-bubble card-surface rounded-xl p-4 shadow-xl">
          <p className="text-[10px] font-medium uppercase tracking-wide"
             style={{ color: 'var(--vine)' }}>
            The library whispers{section ? ` · ${section.label}` : ''}
          </p>
          <p className="mt-1.5 text-sm font-semibold">{term.term}</p>
          <p className="mt-1 text-xs leading-relaxed"
             style={{ color: 'var(--text-secondary)' }}>
            <ManaText>{term.short}</ManaText>
          </p>
          <div className="mt-3 flex items-center gap-3">
            <button onClick={() => setStep((n) => n + 1)}
                    className="btn btn-quiet btn-xs">
              Another leaf
            </button>
            <button onClick={() => setOpen(false)}
                    className="btn btn-ghost btn-xs ml-auto">
              Close
            </button>
          </div>
        </div>
      )}
      <button type="button"
              onClick={() => setOpen((o) => !o)}
              aria-expanded={open}
              title="A whisper from the library — one term at a time"
              aria-label="A whisper from the library"
              className="whisper-sprout card-surface flex h-11 w-11 items-center
                         justify-center rounded-full shadow-lg">
        <Sprout />
      </button>
    </div>
  )
}
