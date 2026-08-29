import { useEffect, useState } from 'react'
import { type ClaudeStatus, type IntakeSheet } from '../lib/api'
import { fetchClaudeStatus, useStance } from '../lib/stance'

/**
 * The intake sheet: what a deck may be asked to have done to it as it lands
 * (ADR 41).
 *
 * Five toggles, **all off by default**, chosen per import rather than
 * remembered. That is not a UI preference — ADR 41 makes it the first of two
 * gates, and a sheet that remembered its last state would be a standing
 * permission rather than an answer about this deck.
 *
 * ## Why drafting is sometimes not here at all
 *
 * Four of these were always allowed and simply never built. The first — Claude
 * drafting the reasons — is the one that needed a decision, and it carries a
 * second gate: the user's stance has to permit a write. Below that stance the
 * control is **absent, not disabled**, and there is a sentence saying where the
 * setting is.
 *
 * A disabled toggle would be the wrong answer twice over. It reads as "this is
 * broken" rather than "you turned this off", and a control nobody can reach is
 * a control that owes an explanation more than it owes a place in the row.
 *
 * ## Why each one says what it does rather than what it is called
 *
 * The audience for an import screen is somebody who has just pasted their one
 * deck (commandment 2). "Commander dossier" is a name; "who your commander is
 * and where they come from" is what they get. Every label here is the second
 * kind, and the shortest true version of it.
 */

/** One row of the sheet. `writes` is what it changes in the deck file, in
 *  words, or null when it changes nothing there. */
const ACTIONS: {
  key: keyof IntakeSheet
  title: string
  blurb: string
  writes: string | null
}[] = [
  {
    key: 'categories',
    title: 'Sort the cards',
    blurb: 'Files every card under what it is doing — ramp, removal, a win '
      + 'condition — instead of leaving them all under Utility.',
    writes: 'the category on each card',
  },
  {
    key: 'rationales',
    title: 'Draft the reasons',
    blurb: 'Writes a first pass at why each card is in the deck. They are '
      + 'marked as Claude’s in the file until you rewrite them, and cards you '
      + 'already wrote a reason for are left alone.',
    writes: 'a why on the blank cards',
  },
  {
    key: 'description',
    title: 'Describe the deck',
    blurb: 'A short paragraph on what the deck is trying to do and what it is '
      + 'bad at, plus the handful of words that label it.',
    writes: 'the deck’s game plan note and its themes',
  },
  {
    key: 'dossier',
    title: 'Read up on your commander',
    blurb: 'Who they are in Magic’s story, what kind of deck they usually '
      + 'lead, and who else you might have built instead.',
    writes: null,
  },
  {
    key: 'argue',
    title: 'Argue with every card',
    blurb: 'Makes the case against each card holding its slot. Only against — '
      + 'the case for a card is a reason, and reasons are yours to write.',
    writes: null,
  },
]

export function IntakeChoices({ value, onChange, slug, owner }: {
  value: IntakeSheet
  /** A functional update, and deliberately not a plain value: two chips
   *  toggled inside one frame would otherwise both read the same stale
   *  `value` prop and the first would be lost. Pass a `useState` setter. */
  onChange: (update: (prev: IntakeSheet) => IntakeSheet) => void
  /** The deck this will run against, so the stance is the deck's own. Empty
   *  before a slug is chosen, which is fine: the dial answers without one. */
  slug?: string
  owner?: string
}) {
  const [pin, setPin] = useStance()
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [asked, setAsked] = useState(false)

  useEffect(() => {
    let live = true
    void (async () => {
      try {
        // **`surface: 'intake'` is load-bearing, not decoration.** This screen
        // has no deck yet -- the deck it is about does not exist until the
        // button is pressed -- and without a surface the dial answers with the
        // no-deck default, which is `off`. That stands the whole sheet down
        // for every user on the one page it belongs to, which is exactly what
        // it did until somebody loaded the page.
        const got = await fetchClaudeStatus(
          { slug, owner, surface: 'intake' }, pin, () => setPin(null))
        if (live) setStatus(got)
      } catch {
        // A dial that will not answer is not an error on this screen. The
        // sheet still works — the server decides what it will do — and the
        // one control that depends on the answer stays hidden, which is the
        // safe direction for a control gated on a permission.
        if (live) setStatus(null)
      } finally {
        if (live) setAsked(true)
      }
    })()
    return () => { live = false }
  }, [slug, owner, pin, setPin])

  // **A dial that could not be read is not a dial that said no.** These three
  // used to fall back to `false`, so any failure -- a 404, a dropped request,
  // a 500 -- rendered as "Claude is turned off for this deck", which is a
  // statement about somebody's settings made by something that could not read
  // their settings. `unread` keeps the two apart.
  const unread = asked && status === null
  const configured = status?.configured ?? false
  const mayWrite = status?.stance.may_write ?? false
  const allowsCalls = status?.stance.allows_calls ?? false

  // Nothing here can run without a credential and a stance that speaks, so
  // the whole sheet stands down rather than offering five controls that would
  // each fail the same way. Only when the dial actually SAID so.
  if (asked && !unread && (!configured || !allowsCalls)) {
    return (
      <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        Your deck will land exactly as you pasted it. Claude is turned off for
        this deck, so nothing will be added to it — the gate, the simulator and
        every other part of the site work the same either way.
      </p>
    )
  }

  // Drafting stays shut when the answer is unknown -- closed is the safe
  // direction for a control gated on a permission -- but the four that were
  // never gated are still offered, because the server decides what it will do
  // and a sheet that hid them would be guessing in the other direction.
  const shown = ACTIONS.filter((a) => a.key !== 'rationales' || mayWrite)

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        {shown.map((action) => {
          const on = value[action.key] === true
          return (
            <button
              key={action.key}
              type="button"
              aria-pressed={on}
              onClick={() =>
                onChange((prev) => ({ ...prev, [action.key]: prev[action.key] !== true }))}
              className={`chip-toggle rounded-full px-3 py-1.5 text-xs font-medium${
                on ? ' is-on' : ''}`}>
              {action.title}
            </button>
          )
        })}
      </div>

      {/* The blurbs sit under the row rather than in a tooltip, because a
          tooltip is hover-only and half this audience is on a phone. Only the
          chosen ones are described: five paragraphs nobody asked for is how a
          simple choice starts reading like a form. */}
      {shown.some((a) => value[a.key]) && (
        <ul className="space-y-2 text-xs leading-relaxed"
            style={{ color: 'var(--text-secondary)' }}>
          {shown.filter((a) => value[a.key]).map((action) => (
            <li key={action.key}>
              <strong style={{ color: 'var(--text-primary)' }}>{action.title}.</strong>{' '}
              {action.blurb}
              {action.writes && (
                <>
                  {' '}
                  <span style={{ color: 'var(--text-muted)' }}>
                    Changes {action.writes}; you can edit or undo any of it on
                    the deck page.
                  </span>
                </>
              )}
            </li>
          ))}
        </ul>
      )}

      {/* Said in full rather than behind a mark or a tooltip. This sentence
          explains why a control the user may have been told about is not on
          their screen, and hiding that behind a hover is hiding it from every
          phone and every keyboard. */}
      {asked && !mayWrite && !unread && (
        <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
          Claude will not draft the reasons for your cards here, because your
          settings say it may not change anything. That is the write setting on
          the stance dial, in the settings panel — its lowest position is
          “nothing; it talks, you type”, and that is the one you are on.
          Everything else on this list still works.
        </p>
      )}
    </div>
  )
}
