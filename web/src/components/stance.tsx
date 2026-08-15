/**
 * The stance readout — what a Claude panel says about the setting it obeys.
 *
 * This used to be the dial itself, repeated as a fieldset on three screens.
 * The pin was always one global value, so the control moved to the header
 * (`StanceMenu`) and each panel keeps this single line instead: the resolved
 * position, with the full answer one hover away.
 *
 * Two properties survive the move unchanged, because they were never about
 * layout:
 *
 * - **The axes are a readout of the server's resolved answer.** `status.stance`
 *   is what `/api/claude` said after resolving and clamping the pin; nothing
 *   here recomputes it. A second implementation of `stance.clamp` in
 *   TypeScript would disagree silently — the UI saying `collaborator` while
 *   the instance ran `consultant`.
 * - **The `never` sentence is served and always present.** ADR 15's rule is
 *   that no stance can widen it, so it does not depend on the pin.
 *
 * When a pin is being narrowed by the deployment ceiling, the line says so —
 * phrased as the instance's decision rather than the user's mistake, because
 * they picked something legitimate and an operator capped it.
 */

import { useEffect, useRef, useState } from 'react'
import type { ClaudeStatus } from '../lib/api'
import { levelLabel, presetLabel } from '../lib/claudecopy'
import { isCapped, type StancePin } from '../lib/stance'

export function StanceReadout({ status, pin }: {
  status: ClaudeStatus
  pin: StancePin
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

  const capped = isCapped(pin, status)
  // The resolved answer leads; the pin only appears when it bought less than
  // it asked for, or when following the deck (the position worth naming
  // because it is the one the menu can give back).
  const resolved = presetLabel(status.stance.preset)
  const line = pin === null
    ? `${resolved} · following the deck`
    : capped
      ? `${resolved} · limited from ${presetLabel(pin)}`
      : resolved

  return (
    <span ref={ref} className="relative inline-block text-[11px]">
      <button
        type="button"
        aria-label="What is Claude allowed to do here?"
        aria-expanded={open}
        onMouseEnter={() => setOpen(true)}
        onMouseLeave={() => !pinned && setOpen(false)}
        onFocus={() => setOpen(true)}
        onBlur={() => !pinned && setOpen(false)}
        onClick={() => { setPinned((p) => !p); setOpen(true) }}
        className="cursor-help"
        style={{ color: 'var(--text-muted)' }}
      >
        Claude:{' '}
        <span style={{ color: 'var(--text-secondary)',
                       borderBottom: '1px dotted var(--text-muted)' }}>
          {line}
        </span>
      </button>

      {open && (
        <span
          role="tooltip"
          className="pointer-events-none absolute bottom-full left-0 z-50 mb-1 block w-72 rounded-lg px-3 py-2 text-left leading-relaxed shadow-xl"
          style={{
            background: 'var(--surface-1)',
            border: '1px solid var(--hairline)',
            color: 'var(--text-secondary)',
            whiteSpace: 'normal',
          }}
        >
          {/* What actually applied, straight from the server — never
              recomputed here. */}
          <span className="block space-y-1">
            {status.stance.axes.map((axis) => (
              <span key={axis.axis} className="block">
                <span style={{ color: 'var(--text-muted)' }}>{axis.question}</span>{' '}
                <span style={{ color: 'var(--text-primary)' }}>
                  {levelLabel(axis.level)}
                </span>
                <span style={{ color: 'var(--text-muted)' }}> — {axis.means}</span>
              </span>
            ))}
          </span>
          {capped && (
            <span className="mt-1 block" style={{ color: 'var(--text-muted)' }}>
              This server limits Claude below your setting, so the narrower
              answer above is what applies.
            </span>
          )}
          <span className="mt-1 block" style={{ color: 'var(--text-muted)' }}>
            {status.never}
          </span>
          <span className="mt-1 block" style={{ color: 'var(--text-muted)' }}>
            Change it from the Claude menu in the header.
          </span>
        </span>
      )}
    </span>
  )
}
