import { useEffect, useRef, useState } from 'react'
import { api, errorMessage, type DeckRef, type Printing } from '../lib/api'
import { Spinner } from './ui'

/**
 * Click-versus-double-click, disambiguated (punch list 2026-08-15 item 3).
 *
 * A double-click is two clicks, and a naive handler pair would fire the
 * single-click path twice first — which here means "apply" then "toggle back
 * to the default", the worst possible reading of an eager gesture. So the
 * single click waits one beat before applying; the double-click cancels the
 * pending beat and does its own thing. 250ms is under anybody's double-click
 * threshold and invisible next to the save's round trip.
 */
function useClickOrDouble(single: (id: string) => void,
                          double: (id: string) => void) {
  const timer = useRef<number | null>(null)
  useEffect(() => () => {
    if (timer.current) window.clearTimeout(timer.current)
  }, [])
  return {
    click(id: string) {
      if (timer.current) window.clearTimeout(timer.current)
      timer.current = window.setTimeout(() => {
        timer.current = null
        single(id)
      }, 250)
    },
    doubleClick(id: string) {
      if (timer.current) {
        window.clearTimeout(timer.current)
        timer.current = null
      }
      double(id)
    },
  }
}

/**
 * The same choice for any card in the 99 (punch list 2026-08-15 item 8).
 *
 * Rendered under the card's row by the deck page's action bar rather than as
 * a per-row toggle — 99 "change art" buttons would be 98 too many. Writes
 * through `set_card_field(field='art')`, which pool-checks the id the same
 * way the commander's picker does, and picking the printing already showing
 * clears the choice back to the default, same contract as below.
 */
export function CardArtPicker({ deck, card, onPicked, onClose }: {
  deck: DeckRef
  card: string
  onPicked: () => void
  onClose: () => void
}) {
  const [printings, setPrintings] = useState<Printing[] | null>(null)
  const [selected, setSelected] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState('')

  useEffect(() => {
    let live = true
    api.printings(deck, card)
      .then((list) => {
        if (!live) return
        setPrintings(list.printings)
        setSelected(list.selected)
      })
      .catch((err) => { if (live) setError(errorMessage(err)) })
    return () => { live = false }
  }, [deck, card])

  async function pick(id: string, thenClose = false) {
    const next = id === selected ? '' : id
    setSaving(id)
    setError(null)
    try {
      await api.setCardField(deck, card, 'art', next)
      setSelected(next)
      onPicked()
      // Close only on a successful save: a double-click that failed must
      // leave the error where the person who made it is looking.
      if (thenClose) onClose()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving('')
    }
  }

  const gesture = useClickOrDouble(
    (id) => void pick(id),
    (id) => void pick(id, true),
  )

  return (
    <div className="mt-2 rounded-lg px-3 py-3"
         style={{ border: '1px solid var(--hairline)', background: 'var(--page)' }}>
      <div className="flex items-center gap-2">
        <p className="text-xs font-medium">Art for {card}</p>
        <button type="button" onClick={onClose}
                className="ml-auto rounded-md px-2 py-1 text-[11px]"
                style={{ border: '1px solid var(--hairline)',
                         color: 'var(--text-muted)' }}>
          Close
        </button>
      </div>
      {error && (
        <p className="mt-2 text-xs" style={{ color: 'var(--status-critical)' }}>
          {error}
        </p>
      )}
      {!printings && !error && <Spinner label="finding printings…" />}
      {printings && printings.length === 0 && (
        <p className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
          No printings found. The library&rsquo;s card pool has not been
          stocked yet.
        </p>
      )}
      {printings && printings.length > 0 && (
        <>
          <p className="mb-2 mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
            {printings.length} printings, newest first. The choice is saved to{' '}
            <code>deck.yaml</code>; picking the one showing resets to the
            default. Double-click a painting to pick it and close this.
          </p>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-6">
            {printings.map((printing) => {
              const on = printing.id === selected
              return (
                <button
                  key={printing.id}
                  type="button"
                  onClick={() => gesture.click(printing.id)}
                  onDoubleClick={() => gesture.doubleClick(printing.id)}
                  disabled={!!saving}
                  title={`${printing.set_name ?? printing.set_code} · #${printing.collector_number ?? '?'}`}
                  className="overflow-hidden rounded-lg text-left"
                  style={{
                    outline: on ? '2px solid var(--series-1)' : '1px solid var(--hairline)',
                    outlineOffset: on ? '1px' : '0',
                    opacity: saving && saving !== printing.id ? 0.5 : 1,
                    background: 'var(--page)',
                  }}
                >
                  {printing.art_crop
                    ? <img src={printing.art_crop} alt="" loading="lazy"
                           className="aspect-[626/457] w-full object-cover" />
                    : <span className="block aspect-[626/457] w-full" />}
                  <span className="flex items-baseline justify-between gap-1 px-1.5 py-1">
                    <span className="text-[10px] font-medium">
                      {printing.set_code}
                    </span>
                    <span className="text-[10px]" style={{ color: 'var(--text-muted)' }}>
                      {printing.released_at?.slice(0, 4)}
                    </span>
                  </span>
                  {on && (
                    <span className="block px-1.5 pb-1 text-[10px]"
                          style={{ color: 'var(--series-1)' }}>
                      showing · click to reset
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}

/**
 * Choose which printing's art a deck shows for its commander.
 *
 * The choice is a **deck** property, not a viewer preference — it lives in
 * `deck.yaml` and travels with the deck through git, the same as every other
 * decision about it (ADR 1). So this writes through the ordinary deck-field
 * edit, the server refuses an id that is not a printing of *this* commander,
 * and the change shows up in `git diff` as one line.
 *
 * Only non-digital printings are offered. An Arena-only painting is not
 * something you can put in a sleeve, and offering one would be offering
 * something that does not exist as a card.
 *
 * Fetched when opened rather than with the deck: Goreclaw has twelve
 * printings and most visits never open this.
 */
export function ArtPicker({ deck, onPicked }: {
  deck: DeckRef
  onPicked: () => void
}) {
  const [open, setOpen] = useState(false)
  const [printings, setPrintings] = useState<Printing[] | null>(null)
  const [selected, setSelected] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState('')

  useEffect(() => {
    if (!open || printings) return
    let live = true
    api.printings(deck)
      .then((list) => {
        if (!live) return
        setPrintings(list.printings)
        setSelected(list.selected)
      })
      .catch((err) => { if (live) setError(errorMessage(err)) })
    return () => { live = false }
  }, [open, printings, deck])

  async function pick(id: string, thenClose = false) {
    // Picking the one already showing clears the choice instead of rewriting
    // it — that is how somebody gets back to the default printing without
    // having to know which one it was.
    const next = id === selected ? '' : id
    setSaving(id)
    setError(null)
    try {
      await api.setDeckField(deck, 'commander_art', next)
      setSelected(next)
      onPicked()
      // The one-gesture swap (item 3): double-click applies and folds the
      // picker away — but only on success, so an error is seen where it
      // happened.
      if (thenClose) setOpen(false)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving('')
    }
  }

  const gesture = useClickOrDouble(
    (id) => void pick(id),
    (id) => void pick(id, true),
  )

  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        // Sized to match the hero action row it sits in — see the share
        // toggle's note; three button idioms in one row was the defect.
        className="rounded-lg px-4 py-2 text-sm font-medium"
        style={{ border: '1px solid var(--hairline)', background: 'var(--page)',
                 color: 'var(--text-secondary)' }}
        aria-expanded={open}
      >
        {open ? 'Close art picker' : 'Change art'}
      </button>

      {open && (
        <div className="mt-3">
          {error && (
            <p className="mb-2 text-xs" style={{ color: 'var(--status-critical)' }}>
              {error}
            </p>
          )}
          {!printings && !error && <Spinner label="finding printings…" />}
          {printings && printings.length === 0 && (
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              No printings found. The library&rsquo;s card pool has not been
              stocked yet.
            </p>
          )}
          {printings && printings.length > 0 && (
            <>
              <p className="mb-2 text-xs" style={{ color: 'var(--text-muted)' }}>
                {printings.length} printings, newest first. Digital-only ones
                are left out. The choice is saved to <code>deck.yaml</code>, so
                it travels with the deck. Double-click a painting to swap and
                close the picker in one go.
              </p>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-6">
                {printings.map((printing) => {
                  const on = printing.id === selected
                  return (
                    <button
                      key={printing.id}
                      type="button"
                      onClick={() => gesture.click(printing.id)}
                      onDoubleClick={() => gesture.doubleClick(printing.id)}
                      disabled={!!saving}
                      title={`${printing.set_name ?? printing.set_code} · #${printing.collector_number ?? '?'}`}
                      className="overflow-hidden rounded-lg text-left"
                      style={{
                        // The selected one is ringed rather than merely
                        // brighter: "which am I showing" has to survive a
                        // greyscale screenshot and a colourblind reader.
                        outline: on ? '2px solid var(--series-1)' : '1px solid var(--hairline)',
                        outlineOffset: on ? '1px' : '0',
                        opacity: saving && saving !== printing.id ? 0.5 : 1,
                        background: 'var(--page)',
                      }}
                    >
                      {printing.art_crop
                        ? <img src={printing.art_crop} alt="" loading="lazy"
                               className="aspect-[626/457] w-full object-cover" />
                        : <span className="block aspect-[626/457] w-full" />}
                      <span className="flex items-baseline justify-between gap-1 px-1.5 py-1">
                        <span className="text-[10px] font-medium">
                          {printing.set_code}
                        </span>
                        <span className="text-[10px]" style={{ color: 'var(--text-muted)' }}>
                          {printing.released_at?.slice(0, 4)}
                        </span>
                      </span>
                      {on && (
                        <span className="block px-1.5 pb-1 text-[10px]"
                              style={{ color: 'var(--series-1)' }}>
                          showing · click to reset
                        </span>
                      )}
                    </button>
                  )
                })}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )
}
