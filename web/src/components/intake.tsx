import { useEffect, useRef, useState } from 'react'
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
 * ## Why the sheet hands its stance back up
 *
 * This component is the only thing that talks to the dial, and the dial's
 * answer is what decides whether the drafting toggle is on the screen at all.
 * The submit is a *different* request from a different file, and the server
 * resolves the stance for it all over again — so unless one value reaches
 * both, the sheet and the server hold two opinions about one permission.
 *
 * **They did, from the day ADR 41 landed.** The import page sent no stance at
 * all, the server fell back to the deck's own default (`consultant`, which
 * writes nothing), and every user whose dial permitted a write was offered the
 * toggle here and refused by the server the moment they used it. Aaron hit it
 * on 2026-08-29; nothing had ever drafted a rationale from this screen.
 *
 * So `onStance` reports the value this sheet asked the dial with, and the
 * submit sends that one. Reported rather than read a second time on purpose: a
 * parent that fetched the pin again would be answering a question this
 * component has already asked, which is the same bug wearing a different file
 * name and one refactor away from diverging again. `runIntake` takes the
 * stance as a **required** parameter for the same reason — a call site can no
 * longer forget it without the typechecker saying so.
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
    blurb: 'The paragraph that shows on your shelf and at the top of the '
      + 'deck: what it is trying to do, how it wins, and one honest line on '
      + 'what it is bad at.',
    writes: 'the deck’s description and its themes',
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

export function IntakeChoices({ value, onChange, onStance, slug, owner, running }: {
  value: IntakeSheet
  /** A functional update, and deliberately not a plain value: two chips
   *  toggled inside one frame would otherwise both read the same stale
   *  `value` prop and the first would be lost. Pass a `useState` setter. */
  onChange: (update: (prev: IntakeSheet) => IntakeSheet) => void
  /** Told the stance this sheet asked the dial with, so the submit can carry
   *  the same one. `undefined` is a position — "no pin, let the surface
   *  default" — and not an absence. See the note above on why this travels
   *  upward instead of being read again by whoever submits. */
  onStance?: (stance: string | undefined) => void
  /** The deck this will run against, so the stance is the deck's own. Empty
   *  before a slug is chosen, which is fine: the dial answers without one. */
  slug?: string
  owner?: string
  /**
   * The sheet has been submitted and the work is running.
   *
   * **A chip toggled now changes nothing, and said nothing about it.** The
   * request left when the button was pressed; the run is on the server and
   * reads none of this. So a chip that still answers the hand is telling a
   * small lie for the whole two or three minutes an intake takes — press
   * "Draft the reasons" off while it is drafting and it goes grey, exactly as
   * though it had been called off, and eighty-four rationales arrive anyway.
   *
   * Locked rather than hidden: what was asked for is the most useful thing on
   * the screen while it is being done, and a sheet that vanished at the moment
   * it started would take that away.
   */
  running?: boolean
}) {
  const [pin, setPin] = useStance()
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [asked, setAsked] = useState(false)

  // **The callback in a ref, and kept out of the effect below's dependencies.**
  // It is reported from inside the effect that asks the dial, so a caller who
  // passes an inline arrow would change its identity on every render and turn
  // one question to the dial into an unbounded stream of them. A doc comment
  // asking for a stable callback is not a mechanism; this is.
  const report = useRef(onStance)
  useEffect(() => { report.current = onStance })

  useEffect(() => {
    // **Reported as the question is asked, not once the answer arrives.** This
    // is the value the dial is being asked with, and the submit has to carry
    // the same one whatever comes back — including when nothing does, because
    // the four ungated actions still run and the user's dial still applies to
    // them. `fetchClaudeStatus` may drop a pin this build no longer serves,
    // which changes `pin` and re-runs this effect, so the parent is corrected
    // by the same path that healed it.
    report.current?.(pin ?? undefined)
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
              disabled={running}
              // Said rather than merely shown: a disabled control explains
              // itself to a pointer that hovers it and to a reader that lands
              // on it, and "why can I not press this" is the whole question a
              // locked sheet raises.
              title={running
                ? 'Asked for already — this is running now and cannot be changed'
                : undefined}
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
        <ul className="intake-told">
          {shown.filter((a) => value[a.key]).map((action) => (
            <li key={action.key}>
              <span className="intake-told-name">{action.title}</span>
              <p className="intake-told-what">{action.blurb}</p>
              {action.writes && (
                <p className="intake-told-writes">
                  <b>Writes</b>
                  {action.writes} — yours to edit or undo on the deck page.
                </p>
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
