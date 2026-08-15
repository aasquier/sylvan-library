/**
 * The stance, as a header menu — the user-setting shape of what used to be a
 * fieldset repeated on three screens.
 *
 * The pin was always one global value (`lib/stance.ts`), so the control moved
 * to the one global surface and the panels that obey it now carry a readout
 * (`StanceReadout`) instead of a second dial. Everything the dial promised
 * still holds here:
 *
 * - **The presets are served, never hardcoded.** The roster, the blurbs and
 *   the `available` flags all come from `/api/claude`; this file adds only the
 *   human labels (`lib/claudecopy.ts`).
 * - **"Follow the deck" is first and is the default** — `null` is a position,
 *   and this menu is the only control that can give it back.
 * - **A capped preset is shown, disabled, and labelled.** "The operator
 *   limited this" and "this does not exist" are different facts.
 * - **The `never` sentence is always visible** at the foot of the menu,
 *   unconditional on the pin.
 *
 * Self-gating: it fetches its own status (no deck, no surface — the roster
 * and the two availability flags are all it reads) and renders nothing at all
 * when Claude is not installed and configured, because a menu over a feature
 * that does not exist here would be a control over nothing.
 */

import { useEffect, useRef, useState } from 'react'
import type { ClaudeStatus } from '../lib/api'
import { presetLabel } from '../lib/claudecopy'
import { fetchClaudeStatus, useStance } from '../lib/stance'

const FOLLOW_BLURB =
  'No preference. A deck being considered opens wider than one that is '
  + 'already sleeved, and a deck that does not exist yet opens widest.'

export function StanceMenu() {
  const [pin, setPin] = useStance()
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let live = true
    fetchClaudeStatus({}, pin, () => setPin(null))
      .then((s) => { if (live) setStatus(s) })
      .catch(() => { if (live) setStatus(null) })
    return () => { live = false }
  }, [pin, setPin])

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    const onClick = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false)
    }
    window.addEventListener('keydown', onKey)
    window.addEventListener('mousedown', onClick)
    return () => {
      window.removeEventListener('keydown', onKey)
      window.removeEventListener('mousedown', onClick)
    }
  }, [open])

  if (!status?.installed || !status.configured) return null

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        className="whitespace-nowrap rounded-md px-2 py-1.5 text-sm"
        style={{ color: 'var(--text-secondary)', border: '1px solid var(--hairline)' }}
      >
        Claude<span className="hidden sm:inline"> · {presetLabel(pin)}</span>
        <span aria-hidden className="ml-1 text-[9px]">▾</span>
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-72 rounded-lg p-3 shadow-xl"
             style={{ background: 'var(--surface-1)',
                      border: '1px solid var(--hairline)' }}>
          <p className="mb-2 text-[11px] font-medium"
             style={{ color: 'var(--text-primary)' }}>
            How much should Claude do?
          </p>
          <div className="space-y-1.5">
            <Option
              selected={pin === null}
              available
              label={presetLabel(null)}
              blurb={FOLLOW_BLURB}
              onSelect={() => { setPin(null); setOpen(false) }}
            />
            {status.presets.map((preset) => (
              <Option
                key={preset.name}
                selected={pin === preset.name}
                available={preset.available}
                label={presetLabel(preset.name)}
                blurb={preset.blurb}
                onSelect={() => { setPin(preset.name); setOpen(false) }}
              />
            ))}
          </div>
          <p className="mt-2 border-t pt-2 text-[11px]"
             style={{ borderColor: 'var(--hairline)', color: 'var(--text-muted)' }}>
            {status.never}
          </p>
        </div>
      )}
    </div>
  )
}

/**
 * One position. A real radio rather than a styled button, so the group is one
 * tab stop and arrow keys move within it — which is what a five-way exclusive
 * choice is supposed to do, and what a column of buttons silently would not.
 */
function Option({ selected, available, label, blurb, onSelect }: {
  selected: boolean
  available: boolean
  label: string
  blurb: string
  onSelect: () => void
}) {
  return (
    <label className={`flex gap-2 ${available ? 'cursor-pointer' : 'opacity-50'}`}>
      <input
        type="radio"
        name="claude-stance"
        checked={selected}
        // Disabled rather than hidden: a level this deployment will not honour
        // is still worth showing, because "the operator limited this" and
        // "this does not exist" are different facts and only one is true.
        disabled={!available}
        onChange={onSelect}
        className="mt-0.5"
      />
      <span className="text-[11px]">
        <span style={{ color: 'var(--text-primary)' }}>{label}</span>
        {!available && (
          <span className="ml-1" style={{ color: 'var(--text-muted)' }}>
            (limited on this server)
          </span>
        )}
        <span className="block" style={{ color: 'var(--text-muted)' }}>{blurb}</span>
      </span>
    </label>
  )
}
