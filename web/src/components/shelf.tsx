/**
 * The shelves (second punch list of 2026-08-15, item 7): one fact at a time
 * from `lore.py`'s volumes, under the Library masthead.
 *
 * The first shelf strip borrowed one combination story from the colour
 * taxonomy; this is the grown version. The facts are checked-in prose served
 * by `/api/lore` — history, rules that changed, the painters, table talk,
 * curiosities — each with a longer paragraph behind **Tell me more**, so the
 * strip is a door and never a wall of text. Cards a fact names arrive
 * *resolved through the pool* with their own cost and text (or dropped and
 * counted server-side), so the one place trivia touches card facts is the
 * place card facts always come from.
 *
 * Walking the shelf follows the whisper's rule: **Another** advances a
 * shuffled order by a coprime stride rather than re-rolling, because a
 * re-roll repeats often enough to feel broken; one random offset per mount
 * gives each visit a different opening fact and the same one for the life
 * of the page. Decorative in the strict sense — any failure to load renders
 * nothing at all.
 */

import { useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { useLore } from '../lib/lore'
import { CardHover, ManaCost } from './ui'

/** A stride coprime to any plausible shelf size (89 is prime, and the shelf
 *  is nowhere near a multiple of it) — every fact shows before any repeats. */
const STRIDE = 89

export function TheShelves() {
  const lore = useLore()
  const [step, setStep] = useState(0)
  const [open, setOpen] = useState(false)
  const offset = useRef(Math.floor(Math.random() * 997))

  const facts = lore?.facts ?? []
  if (facts.length === 0) return null

  const fact = facts[(offset.current + step * STRIDE) % facts.length]
  if (!fact) return null
  const volume = lore?.volumes.find((v) => v.key === fact.volume)

  return (
    <aside className="shelf-fact card-surface rounded-xl px-4 py-3">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
        <span className="text-[10px] uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          From the shelves
        </span>
        {volume && (
          <span className="rounded-full px-2 py-0.5 text-[10px] font-medium"
                title={volume.blurb}
                style={{ background: 'var(--gridline)',
                         color: 'var(--text-secondary)' }}>
            {volume.label}
          </span>
        )}
      </div>

      <p className="mt-1.5 text-sm leading-relaxed"
         style={{ color: 'var(--text-secondary)' }}>
        {fact.fact}
      </p>

      {open && (
        <p className="mt-2 text-xs leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          {fact.more}
        </p>
      )}

      {open && fact.cards.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1">
          {fact.cards.map((card) => (
            <CardHover key={card.name} card={card}>
              <span className="inline-flex cursor-help items-baseline gap-1.5 text-xs underline decoration-dotted"
                    style={{ color: 'var(--text-primary)' }}>
                {card.name}
                {card.mana_cost && <ManaCost cost={card.mana_cost} size={12} />}
              </span>
            </CardHover>
          ))}
        </div>
      )}

      <div className="mt-2 flex flex-wrap items-center gap-3 text-xs">
        <button type="button" onClick={() => setOpen((o) => !o)}
                className="btn btn-ghost btn-xs font-medium"
                style={{ color: 'var(--series-1)' }}>
          {open ? 'Enough' : 'Tell me more'}
        </button>
        <button type="button"
                onClick={() => { setOpen(false); setStep((n) => n + 1) }}
                className="btn btn-ghost btn-xs">
          Another
        </button>
        {/* `words` opens the vocabulary tab; a `colors` link lands on the
            combination itself. The Learn page owns its own anchors. */}
        {fact.learn && (
          <Link to={fact.learn.tab === 'colors'
                      ? `/learn?c=${fact.learn.key}`
                      : '/learn?tab=words'}
                className="ml-auto whitespace-nowrap underline"
                style={{ color: 'var(--series-1)' }}>
            In the Learn room →
          </Link>
        )}
      </div>
    </aside>
  )
}
