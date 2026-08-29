/**
 * The card finder: type a few letters, see the card, choose it.
 *
 * ## What this replaces, and what was wrong with it
 *
 * A plain text box whose placeholder read *"Exact name — checked against the
 * pool"*. That sentence is the whole problem in six words: it asks a beginner
 * to already know something the library knows perfectly well, and it puts the
 * checking **after** the typing. Four things went wrong every time:
 *
 *  1. **A misspelling answered with nothing.** The card search behind that box
 *     is a substring test, so `Sol Rng` matched no row — and an empty result
 *     is indistinguishable from "we do not have that card". Somebody adding
 *     their first card cannot tell those two apart, and there was nothing on
 *     the screen that would help them.
 *  2. **You never saw the card.** Every other surface in this app shows the
 *     painting; the one place you commit a card to your deck showed a text
 *     field.
 *  3. **The refusals arrived last.** Banned, or outside the commander's
 *     colours, are facts about the card that the library knows the instant the
 *     name resolves — but the write path is where they were said, which is
 *     *after* a category, a quantity and a whole rationale had been typed.
 *  4. **Nothing carried over.** The refusal was a red sentence; the box still
 *     held the same wrong name, and you were guessing again with no more help
 *     than the first time.
 *
 * So: the name is *found* rather than recalled, the card is shown while it is
 * being chosen rather than after, and the two refusals are marks on the card
 * before a word of rationale is owed.
 *
 * ## What it does not do
 *
 * **It never writes a rationale** (rule 4, ADR 8, ADR 11). ADR 41 opened one
 * door elsewhere -- an import may ask for drafts, marked as drafts -- and this
 * is not it: a card being added by hand is somebody at a keyboard with the
 * field in front of them, and drafting into it would be answering a question
 * they are in the middle of answering.
 * It fills exactly one field on the user's behalf and only when the card pool
 * says so outright: a land is filed under `land`, which is the importer's own
 * rule and a card pool fact rather than an opinion. Every other category is an
 * opinion, and `why` is nothing but the user's keystrokes.
 *
 * **It does not hide anything.** A banned card is offered and marked; a card
 * outside the deck's identity is offered and marked. Filtering either one out
 * would leave somebody retyping a name that was right all along — the same
 * argument as "an invalid deck is simulated, not refused". The refusal itself
 * still comes from the server, which is the one implementation of the rule.
 *
 * ## The interaction
 *
 * A real combobox, and the keyboard reaches all of it: type, `ArrowDown` and
 * `ArrowUp` through the list, `Enter` to choose, `Escape` to close and again
 * to clear. The **preview tracks the active row**, so arrowing down the list
 * flips through the paintings — which is the part that makes this teach rather
 * than merely work.
 *
 * Layout is a **container query**, not a viewport breakpoint (`.finder-*` in
 * `index.css`). This component sits in a form that is one column on a phone
 * and three on a desk, so "how wide is the screen" is the wrong question and
 * guessing at it with `sm:` is how this repo has lost three rounds to one
 * phone. Narrow: the card sits under its list. Wide: beside it.
 */
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'

import { api, type CardOffer } from '../lib/api'
import { cardWarning } from '../lib/cardoffer'
import { CardHover, ManaCost, ManaText } from './ui'

/** How long a keystroke waits before it becomes a question for the library.
 *  Long enough that typing a name is one request rather than fourteen, short
 *  enough that the list feels like it is keeping up. */
const SETTLE_MS = 220

/** Under this, a query is one or two letters and every card is "near" it. */
const MIN_QUERY = 2

/** How many rows the list offers. Eight is what a phone shows without the
 *  chosen row scrolling out from under the thumb reaching for it. */
const OFFERS = 8

/** What one row's `via` says out loud, for the rows that need saying. The
 *  literal tiers say nothing: finding `Lightning Bolt` inside `bolt` is not
 *  news. A guess is news, and is labelled once, above the first of them. */
const GUESSED = 'near'

export interface CardFinderProps {
  /** The chosen card, or null. Held by the parent so the form can read the
   *  name, the type line and the identity off it. */
  value: CardOffer | null
  onChange: (card: CardOffer | null) => void
  /** The deck's colour identity, so a card outside it is marked at the moment
   *  it is chosen rather than at the moment it is refused. Empty is a
   *  colourless commander, which is a real deck and not a missing value. */
  identity: string[]
  /** The field's label, and the id the label points at. */
  label?: string
}

export function CardFinder({ value, onChange, identity, label = 'Card' }: CardFinderProps) {
  const [typed, setTyped] = useState('')
  // **The answer carries the question it answers**, so "have we got results
  // for what is in the box right now" is derived during render rather than
  // cleared from an effect. Clearing it in the effect meant a stale list
  // survived one paint after a backspace — long enough to be clicked.
  const [answer, setAnswer] = useState<{ asked: string; cards: CardOffer[] } | null>(null)
  const [active, setActive] = useState(0)
  const [open, setOpen] = useState(false)
  const inputId = useId()
  const listId = useId()
  const box = useRef<HTMLDivElement>(null)
  const list = useRef<HTMLUListElement>(null)

  const query = typed.trim()
  const asked = query.length >= MIN_QUERY
  const offers = asked && answer?.asked === query ? answer.cards : null
  const asking = asked && offers === null

  // **Debounced, and ordered.** The timer stops a request per keystroke; the
  // token stops a slow answer to an old question landing on top of a fast
  // answer to the current one, which on a shaky connection is how a list ends
  // up showing the results for four letters ago.
  const token = useRef(0)
  useEffect(() => {
    if (!asked) return
    const mine = ++token.current
    const timer = setTimeout(() => {
      api.suggestCards(query, OFFERS)
        .then((r) => {
          if (token.current !== mine) return
          setAnswer({ asked: query, cards: r.cards })
          setActive(0)
        })
        .catch(() => {
          if (token.current !== mine) return
          setAnswer({ asked: query, cards: [] })
          setActive(0)
        })
    }, SETTLE_MS)
    return () => clearTimeout(timer)
  }, [query, asked])

  // A click anywhere else puts the list down. Hung only while it is up, so a
  // deck page with a closed finder on it costs nothing.
  useEffect(() => {
    if (!open) return
    const away = (e: PointerEvent) => {
      const hit = e.target as Node | null
      if (hit && box.current?.contains(hit)) return
      setOpen(false)
    }
    document.addEventListener('pointerdown', away, true)
    return () => document.removeEventListener('pointerdown', away, true)
  }, [open])

  const rows = offers ?? []
  const showing = open && asked
  // Clamped during render rather than reset from an effect: a shorter list
  // can arrive while the cursor is on row six, and one paint with the cursor
  // past the end is one paint with no card in the panel.
  const at = rows.length === 0 ? 0 : Math.min(active, rows.length - 1)
  // Where the guessed rows start, so the list can say so once instead of
  // labelling every row. The server orders literal hits ahead of guesses, so
  // this is a boundary rather than a filter.
  const firstGuess = rows.findIndex((c) => c.via === GUESSED)

  // What the card panel shows: the row being arrowed over while the list is
  // up, and the chosen card once it is down. Arrowing through a list of
  // paintings is the whole point of the panel.
  const shown = showing ? (rows[at] ?? value) : value

  const choose = useCallback((card: CardOffer) => {
    onChange(card)
    setTyped(card.name)
    setOpen(false)
  }, [onChange])

  const clear = useCallback(() => {
    onChange(null)
    setTyped('')
    setAnswer(null)
    setOpen(false)
  }, [onChange])

  // Keep the active row in view when the keyboard moves it past the fold.
  //
  // Feature-tested rather than called: this is the one line in the component
  // that is pure enhancement — the list is fully usable if it never scrolls
  // itself — and it must not be the line that throws. The test rig has no
  // layout and no `scrollIntoView` at all, which is exactly the kind of
  // rig-versus-browser difference that presents as a product bug.
  useEffect(() => {
    if (!showing) return
    const row = list.current?.children[at]
    if (row instanceof HTMLElement && typeof row.scrollIntoView === 'function') {
      row.scrollIntoView({ block: 'nearest' })
    }
  }, [at, showing])

  function onKey(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Escape') {
      // Two presses, two different jobs: put the list down, then give the
      // field back. A single Escape that cleared the box would throw away a
      // name somebody had nearly finished typing.
      if (showing) setOpen(false)
      else if (typed) clear()
      return
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (!showing) { setOpen(true); return }
      if (rows.length === 0) return
      // Wraps, so the last row's Down reaches the first: a list of eight is
      // short enough that the far end is where somebody actually wanted to
      // be, and a cursor that just stops there says nothing at all.
      const step = e.key === 'ArrowDown' ? 1 : -1
      setActive((at + step + rows.length) % rows.length)
      return
    }
    // Home and End are deliberately NOT taken. They belong to the text box —
    // somebody halfway through `Sakura-Tribe Elder` pressing Home means the
    // start of what they typed, and a list that stole it would be a combobox
    // that had forgotten it is also an input.
    if (e.key === 'Enter') {
      const pick = rows[at]
      if (showing && pick) {
        e.preventDefault()
        choose(pick)
      }
      return
    }
    if (e.key === 'Tab' && showing) setOpen(false)
  }

  const warning = useMemo(
    () => (shown ? cardWarning(shown, identity) : ''), [shown, identity])
  const chosen = value !== null && value.name === typed

  return (
    <div className="finder" ref={box}>
      <label htmlFor={inputId}
             className="mb-1 block text-[11px] font-medium uppercase tracking-wide"
             style={{ color: 'var(--text-muted)' }}>
        {label}
      </label>
      <div className="finder-field">
        <input
          id={inputId}
          className="finder-input"
          value={typed}
          role="combobox"
          aria-expanded={showing}
          aria-controls={listId}
          aria-autocomplete="list"
          aria-activedescendant={showing && rows[at] ? `${listId}-${at}` : undefined}
          autoComplete="off"
          spellCheck={false}
          placeholder="Start typing a card name…"
          onChange={(e) => { setTyped(e.target.value); setOpen(true); if (chosen) onChange(null) }}
          onFocus={() => setOpen(true)}
          onKeyDown={onKey}
        />
        {typed && (
          <button type="button" onClick={clear} className="finder-clear"
                  aria-label="Clear the card name">
            <span aria-hidden="true">×</span>
          </button>
        )}
      </div>

      {/* How many cards came back, for the hand that cannot see the list.
          A combobox's own rows are announced one at a time by
          `aria-activedescendant`, which never says how many there are — so
          "nothing came back" would otherwise be a silence, which is exactly
          the state the old box left everybody in. */}
      <span className="sr-only" role="status">
        {showing && offers !== null
          ? (rows.length === 0
              ? 'No cards match what you typed.'
              : `${rows.length} card${rows.length === 1 ? '' : 's'} to choose from.`)
          : ''}
      </span>

      {/* Every state this field can be in says something, because "nothing
          happened" is the state the old box left somebody in. */}
      {!asked && !chosen && (
        <p className="finder-note">
          A part of the name is enough — and a name spelled nearly right is
          enough too.
        </p>
      )}

      <div className="finder-body">
        <div className="finder-cols">
          <div className="finder-results">
            {showing && (
              <ul className="finder-list" id={listId} role="listbox" ref={list}
                  aria-label="Cards matching what you typed">
                {rows.map((card, i) => (
                  <li key={card.name} id={`${listId}-${i}`} role="option"
                      aria-selected={i === at}
                      className={`finder-row${i === at ? ' is-active' : ''}`}
                      // Pointer, not click: the row has to win the race
                      // against the outside-click listener that closes the
                      // list, and `pointerdown` fires first.
                      onPointerDown={(e) => { e.preventDefault(); choose(card) }}
                      onMouseEnter={() => setActive(i)}>
                    <span className="finder-row-name">
                      {/* A heading for a run of rows, so it is drawn once and
                          hidden from the accessible name — a screen reader
                          reads one option at a time and would hear the
                          heading as part of the first guess's name. Each
                          guessed row carries the same fact for itself. */}
                      {i === firstGuess && (
                        <span className="finder-guess" aria-hidden="true">did you mean</span>
                      )}
                      {card.name}
                      {card.via === GUESSED && (
                        <span className="sr-only">, the closest name in the library</span>
                      )}
                    </span>
                    <span className="finder-row-type">{card.type_line}</span>
                    <span className="finder-row-cost">
                      <ManaCost cost={card.mana_cost} size={13} />
                    </span>
                  </li>
                ))}
              </ul>
            )}
            {showing && rows.length === 0 && !asking && (
              <p className="finder-note">
                No card in the library is spelled anything like that. Try a
                word from the middle of the name — or look the card up on the
                Card search page, which reads rules text as well as names.
              </p>
            )}
            {showing && asking && (
              <p className="finder-note">Looking through the library…</p>
            )}

            {/* The two facts the old flow only ever said after a rationale
                had been written. */}
            {shown && warning && (
              <p className="finder-warning" role="status">{warning}</p>
            )}
            {shown && !warning && chosen && (
              <p className="finder-ok" role="status">
                Legal in Commander, and inside your commander&rsquo;s colours.
              </p>
            )}
          </div>

          {/* Second in the DOM, and second on the screen at every width: on a
              phone the list has to be the thing under the typing, and the
              card follows it. See the container query in index.css. */}
          {shown && (
            <div className="finder-preview">
              {/* The painting whole, hot-linked, and never filtered, cropped
                  or dimmed (ADR 6, ADR 32, commandment 9). Tapping or
                  hovering it lifts the same card bigger. */}
              <CardHover card={{ name: shown.name, image: shown.image }}
                         className="finder-cardwrap">
                {shown.image
                  ? <img src={shown.image} alt={shown.name} loading="lazy"
                         className="finder-card" />
                  : <span className="finder-cardless">{shown.name}</span>}
              </CardHover>
              {shown.artist && (
                <p className="finder-credit">art by {shown.artist}</p>
              )}
            </div>
          )}

          {/* Last, and in its own grid area, so the card comes before it on a
              phone — the picture is the point — and the two of them sit side
              by side where there is room to put a column beside a column. */}
          {shown?.oracle_text && (
            <p className="finder-rules">
              <ManaText size={12}>{shown.oracle_text}</ManaText>
            </p>
          )}
        </div>
      </div>
    </div>
  )
}
