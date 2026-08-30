/**
 * The settings gear (second punch list of 2026-08-15, item 9): one header
 * control for every preference this app keeps about *you*.
 *
 * Before this the header carried the Claude menu and the theme button side by
 * side, the table-sound switch lived only on the tarot table, and the
 * ambience — fireflies, falling leaves, the room behind a page — could not be
 * turned off at all. Now the gear opens one panel with four rows:
 *
 * - **Theme**, the same toggle the logged-out screens keep as a bare button.
 * - **Ambience**, new (`lib/prefs.ts`): off stops the weather and the rooms
 *   in one switch, for readers who want a quiet page. Distinct from
 *   `prefers-reduced-motion`, which is the *system* saying so and removes
 *   motion unconditionally; this is the person saying so, and it removes the
 *   layers entirely.
 * - **Table sound**, the same `mtglab-table-sound` key the tarot table
 *   toggles — one preference, two doors, no drift, because both go through
 *   `lib/tablesounds.ts`. Turning it on here `wake()`s inside the click, so
 *   the switch is heard the same way the table's is.
 * - **Claude**, the stance slider, unchanged in every property the menu
 *   version had (served presets, "follow the deck" first and default, capped
 *   detents peek, the never-sentence always visible). Self-gating: the row
 *   exists only when this instance has Claude installed and configured — but
 *   the *gear* renders regardless, because the other three rows are about
 *   this person, not about any feature's availability.
 */

import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ClaudeStatus } from '../lib/api'
import { CLEARING_HINT, ClearingNote } from './clearing'
import { presetLabel } from '../lib/claudecopy'
import { useClearing } from '../lib/fullscreen'
import { useAmbience, useTableSound } from '../lib/prefs'
import { fetchClaudeStatus, useStance } from '../lib/stance'

const FOLLOW_BLURB =
  'No preference. A deck being considered opens wider than one that is '
  + 'already sleeved, and a deck that does not exist yet opens widest.'

/** The gear itself: eight teeth and a hub, drawn on `currentColor` like
 *  every other mark in the header. */
function GearGlyph() {
  const teeth = Array.from({ length: 8 }, (_, i) => {
    const a = (i * Math.PI) / 4
    return (
      <rect key={i} x="-1.6" y="-9.4" width="3.2" height="4"
            rx="0.8" fill="currentColor"
            transform={`rotate(${(a * 180) / Math.PI})`} />
    )
  })
  return (
    <svg width="16" height="16" viewBox="-10 -10 20 20" aria-hidden>
      <g>{teeth}</g>
      <circle r="6.2" fill="currentColor" />
      <circle r="2.6" fill="var(--page)" />
    </svg>
  )
}

/** One labelled on/off row. A real `role="switch"` so a screen reader hears
 *  a switch, not a button that happens to say "on". */
function Toggle({ label, hint, on, onChange }: {
  label: string
  hint: string
  on: boolean
  onChange: (next: boolean) => void
}) {
  return (
    <button type="button" role="switch" aria-checked={on}
            onClick={() => onChange(!on)}
            className="menu-row flex w-full items-center gap-3 rounded-md px-1.5 py-1.5 text-left">
      <span className="min-w-0 flex-1">
        <span className="block text-[12px] font-medium"
              style={{ color: 'var(--text-primary)' }}>{label}</span>
        <span className="block text-[11px]"
              style={{ color: 'var(--text-muted)' }}>{hint}</span>
      </span>
      <span aria-hidden
            className="relative inline-block h-4 w-7 shrink-0 rounded-full transition-colors"
            style={{ background: on ? 'var(--series-1)' : 'var(--gridline)' }}>
        <span className="absolute top-0.5 h-3 w-3 rounded-full bg-white transition-all"
              style={{ left: on ? '14px' : '2px' }} />
      </span>
    </button>
  )
}

export function SettingsMenu({ theme, onToggleTheme }: {
  theme: 'light' | 'dark'
  onToggleTheme: () => void
}) {
  const [pin, setPin] = useStance()
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [open, setOpen] = useState(false)
  const [ambience, setAmbience] = useAmbience()
  const [sound, setSound] = useTableSound()
  const clearing = useClearing()
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

  const claude = Boolean(status?.installed && status?.configured)

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        aria-label="Settings"
        title="Settings — theme, ambience, sound, and Claude"
        className="btn btn-quiet btn-sm whitespace-nowrap gap-1.5"
      >
        <GearGlyph />
        {claude && (
          <span className="hidden text-xs sm:inline">
            Claude · {presetLabel(pin)}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-80 rounded-lg p-3 shadow-xl"
             style={{ background: 'var(--surface-1)',
                      border: '1px solid var(--hairline)' }}>
          <Toggle label="Dark mode"
                  hint="The forest at night, or by day."
                  on={theme === 'dark'} onChange={onToggleTheme} />
          <Toggle label="Ambience"
                  hint="Fireflies, falling leaves, and the room behind a page."
                  on={ambience} onChange={setAmbience} />
          <Toggle label="Table sound"
                  hint="Shuffles, deals, and the wheel — synthesised, never recorded."
                  on={sound} onChange={setSound} />
          {/* Where a browser will fill the screen on request, the header's own
              control does it in one tap and this is the row that explains it.
              Where it will not — every iPhone — the switch would be a switch
              that does nothing, so the note takes its place and points at the
              home screen, which gets there properly. Nothing at all when the
              app was already launched from one. `lib/fullscreen.ts` argues the
              split; `components/clearing.tsx` owns the copy. */}
          {clearing.offered && (
            <Toggle label="Clear the table" hint={CLEARING_HINT}
                    on={clearing.on} onChange={clearing.toggle} />
          )}
          {!clearing.offered && !clearing.homescreen && <ClearingNote />}
          {/* The door to the decks' own settings.
              **A real destination, so a real link** (commandment 20): every
              other row in this panel changes something here and is a button
              for that reason; this one leaves. It closes the panel on the way
              out, because a popup left standing over the page you just asked
              for is a popup you have to dismiss before you can read it. */}
          <div className="mt-2 border-t pt-2" style={{ borderColor: 'var(--hairline)' }}>
            <Link to="/settings" onClick={() => setOpen(false)}
                  className="menu-row flex w-full items-center gap-3 rounded-md px-1.5 py-1.5 text-left">
              <span className="min-w-0 flex-1">
                <span className="block text-[12px] font-medium"
                      style={{ color: 'var(--text-primary)' }}>Your decks</span>
                <span className="block text-[11px]"
                      style={{ color: 'var(--text-muted)' }}>
                  Who can read each one, and the arena after dark.
                </span>
              </span>
              <span aria-hidden style={{ color: 'var(--text-muted)' }}>›</span>
            </Link>
          </div>
          {claude && status && (
            <div className="mt-2 border-t pt-2"
                 style={{ borderColor: 'var(--hairline)' }}>
              <StancePanel status={status} pin={pin} setPin={setPin} />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/**
 * The stance slider (first 2026-08-15 punch list, item 2): one track from
 * quiet to busy, now a section of the settings panel rather than its own
 * header menu.
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
interface Stop {
  name: string | null
  label: string
  blurb: string
  available: boolean
}

export function StancePanel({ status, pin, setPin }: {
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
    <div>
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
