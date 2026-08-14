/**
 * The stance dial — [ADR 15](../../../docs/adr/0015-claude-surfaces-are-modes-with-capabilities.md)'s
 * "user's dial", and the last thing that ADR listed as unbuilt on this side.
 *
 * Four decisions worth stating, because each one had an easier alternative:
 *
 * **The presets are served, never hardcoded.** `/api/claude` sends the roster
 * with a `blurb` and an `available` flag per entry, and `available` is a fact
 * about *this deployment* — `MTGLAB_CLAUDE_STANCE_CEILING` can cap what any
 * user may select, and an unreadable ceiling fails closed to `off`. A list
 * written into this file would offer levels the instance refuses, which is the
 * failure the ceiling exists to prevent, moved into the UI.
 *
 * **"Follow the deck" is first and is the default.** See `lib/stance.ts`: the
 * per-deck default is real behaviour, not a fallback, and this is the only
 * control that can give it back once it has been overridden.
 *
 * **The axes are shown but not editable.** `stance.py` has three independent
 * axes and four presets over them, and the module says why the presets exist:
 * "most people will never touch the axes". So the axes render as a readout of
 * what the pin actually bought — with each axis's own question, which is
 * served for exactly this — and the dial stays a four-way choice. Three
 * dropdowns would be a truthful interface to a decision nobody is asking to
 * make at that resolution.
 *
 * **The `never` line is always visible, and is never conditional on the pin.**
 * It is served (`status.never`) rather than written here, and ADR 15's rule is
 * that no stance can widen it. A sentence that appeared only at the top of the
 * dial would read as a warning about `collaborator`; sitting under every
 * position, it reads as what it is — the one thing the dial does not control.
 */

import type { ClaudeStatus } from '../lib/api'
import { isCapped, type StancePin } from '../lib/stance'

/** The always-present first position. Not a preset — see `lib/stance.ts`. */
const FOLLOW = {
  name: null,
  label: 'Follow the deck',
  blurb: 'No preference. A deck being considered opens wider than one that is '
       + 'already sleeved, and a deck that does not exist yet opens widest.',
} as const

export function StanceDial({ status, pin, onPin }: {
  status: ClaudeStatus
  pin: StancePin
  onPin: (pin: StancePin) => void
}) {
  const capped = isCapped(pin, status)

  return (
    <fieldset className="space-y-2 border-t pt-2"
              style={{ borderColor: 'var(--hairline)' }}>
      <legend className="text-[11px] font-medium"
              style={{ color: 'var(--text-primary)' }}>
        How much should Claude do?
      </legend>

      <div className="space-y-1">
        <Option
          selected={pin === null}
          available
          label={FOLLOW.label}
          blurb={FOLLOW.blurb}
          onSelect={() => onPin(FOLLOW.name)}
        />
        {status.presets.map((preset) => (
          <Option
            key={preset.name}
            selected={pin === preset.name}
            available={preset.available}
            label={preset.name}
            blurb={preset.blurb}
            onSelect={() => onPin(preset.name)}
          />
        ))}
      </div>

      {/* Only when it is true, and phrased as the instance's decision rather
          than the user's mistake: they picked something legitimate and an
          operator capped it. `presets[].available` is the server saying so. */}
      {capped && (
        <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          This instance caps the stance below that, so the setting above is
          narrowed to what is shown below.
        </p>
      )}

      {/* What actually applied, straight from the server — never recomputed
          here. `status.stance` is the resolved-and-clamped answer. */}
      <dl className="space-y-1">
        {status.stance.axes.map((axis) => (
          <div key={axis.axis} className="flex gap-2 text-[11px]">
            <dt className="w-32 shrink-0" style={{ color: 'var(--text-muted)' }}>
              {axis.question}
            </dt>
            <dd style={{ color: 'var(--text-primary)' }}>
              <span className="font-mono">{axis.level}</span>
              <span className="ml-1" style={{ color: 'var(--text-muted)' }}>
                — {axis.means}
              </span>
            </dd>
          </div>
        ))}
      </dl>

      <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
        {status.never}
      </p>
    </fieldset>
  )
}

/**
 * One position on the dial.
 *
 * A real radio rather than a styled button, so the group is one tab stop and
 * arrow keys move within it — which is what a four-way exclusive choice is
 * supposed to do, and what a row of buttons silently would not.
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
        // is still worth showing, because "the operator capped this" and "this
        // does not exist" are different facts and only one of them is true.
        disabled={!available}
        onChange={onSelect}
        className="mt-0.5"
      />
      <span className="text-[11px]">
        <span style={{ color: 'var(--text-primary)' }}>{label}</span>
        {!available && (
          <span className="ml-1" style={{ color: 'var(--text-muted)' }}>
            (capped by this instance)
          </span>
        )}
        <span className="block" style={{ color: 'var(--text-muted)' }}>{blurb}</span>
      </span>
    </label>
  )
}
