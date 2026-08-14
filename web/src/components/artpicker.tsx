import { useEffect, useState } from 'react'
import { api, errorMessage, type DeckRef, type Printing } from '../lib/api'
import { Spinner } from './ui'

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

  async function pick(id: string) {
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
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setSaving('')
    }
  }

  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="rounded-lg px-3 py-1.5 text-xs font-medium"
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
              No printings found. Without a card pool this list is empty — run
              <code className="mx-1">mtglab data refresh</code>.
            </p>
          )}
          {printings && printings.length > 0 && (
            <>
              <p className="mb-2 text-xs" style={{ color: 'var(--text-muted)' }}>
                {printings.length} printings, newest first. Digital-only ones
                are left out. The choice is saved to <code>deck.yaml</code>, so
                it travels with the deck.
              </p>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-6">
                {printings.map((printing) => {
                  const on = printing.id === selected
                  return (
                    <button
                      key={printing.id}
                      type="button"
                      onClick={() => void pick(printing.id)}
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
