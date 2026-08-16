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

/** One detent on the slider. The first is always "follow the deck"; the rest
 *  are the server's presets in the served order, which is already the
 *  quiet-to-busy order the track needs. */
interface Stop {
  name: string | null
  label: string
  blurb: string
  available: boolean
}

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
        <StancePanel status={status} pin={pin} setPin={setPin} />
      )}
    </div>
  )
}

/**
 * The slider (punch list 2026-08-15 item 2): one track from quiet to busy.
 *
 * The old panel was five radios, which read as five unrelated choices; the
 * whole point of the presets is that they are *ordered* — each one does
 * strictly more than the one before it — and a slider is the control that
 * says so without a paragraph. Far left is "follow the deck", the auto
 * position; far right is the stance where Claude helps with everything it is
 * ever allowed to. A real `<input type="range">`, so it is one tab stop and
 * arrow keys walk the detents.
 *
 * A capped preset is still shown at its place on the track, still labelled —
 * sliding onto it *peeks*: the readout names it, says the operator limited
 * it, and pins nothing, so the setting that actually holds is the last
 * available stop touched. The capped stops are always the busy end of the
 * track, because the cap is a ceiling, so peeking there reads as feeling the
 * deployment's limit rather than as a dead zone in the middle.
 */
function StancePanel({ status, pin, setPin }: {
  status: ClaudeStatus
  pin: string | null
  setPin: (p: string | null) => void
}) {
  const follow: Stop = {
    name: null, label: presetLabel(null), blurb: FOLLOW_BLURB,
    available: true,
  }
  const stops: Stop[] = [
    follow,
    ...status.presets.map((preset) => ({
      name: preset.name as string | null,
      label: presetLabel(preset.name),
      blurb: preset.blurb,
      available: preset.available,
    })),
  ]
  const pinnedAt = Math.max(0, stops.findIndex((s) => s.name === pin))
  // A refused detent being looked at, or null when the thumb sits where the
  // pin is. Peeking is what keeps "shown, disabled, and labelled" true on a
  // control that cannot disable one notch of itself.
  const [peek, setPeek] = useState<number | null>(null)
  const shown = stops[peek ?? pinnedAt] ?? follow

  function slide(index: number) {
    const stop = stops[index]
    if (!stop) return
    if (!stop.available) {
      setPeek(index)
      return
    }
    setPeek(null)
    setPin(stop.name)
  }

  return (
    <div className="absolute right-0 top-full z-50 mt-1 w-80 rounded-lg p-3 shadow-xl"
         style={{ background: 'var(--surface-1)',
                  border: '1px solid var(--hairline)' }}>
      <p className="mb-2 text-[11px] font-medium"
         style={{ color: 'var(--text-primary)' }}>
        How much should Claude do?
      </p>
      <input
        type="range"
        min={0}
        max={stops.length - 1}
        step={1}
        value={peek ?? pinnedAt}
        aria-label="How much should Claude do?"
        aria-valuetext={shown.label}
        onChange={(e) => slide(Number(e.target.value))}
        className="w-full"
        style={{ accentColor: 'var(--series-1)' }}
      />
      <div className="flex justify-between text-[10px]"
           style={{ color: 'var(--text-muted)' }}>
        <span>Follow the deck</span>
        <span>Helps with everything</span>
      </div>
      {/* The readout: which detent, and what it means. Fixed place under the
          track so sliding reads as scrubbing through the positions. */}
      <p className="mt-2 min-h-10 text-[11px]">
        <span style={{ color: 'var(--text-primary)' }}>{shown.label}</span>
        {!shown.available && (
          <span className="ml-1" style={{ color: 'var(--text-muted)' }}>
            (limited on this server)
          </span>
        )}
        <span className="block" style={{ color: 'var(--text-muted)' }}>
          {shown.blurb}
        </span>
      </p>
      <p className="mt-2 border-t pt-2 text-[11px]"
         style={{ borderColor: 'var(--hairline)', color: 'var(--text-muted)' }}>
        {status.never}
      </p>
    </div>
  )
}
