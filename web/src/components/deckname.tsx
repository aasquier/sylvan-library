/**
 * The deck's name, read at the top of its page and written there too.
 *
 * Aaron, 2026-08-29: *"users should be able to edit their deck names after
 * they are imported. I don't think that is currently possible."* He was
 * right. `name` was not in the editor's `SettableDeckFields`, so a deck wore
 * whatever the import form was given, forever — and a first import is
 * precisely where somebody types a placeholder and means to fix it later.
 *
 * **A deck has two identities and this component only touches one.** The name
 * is what a person reads; the deck's *address* is what a link points at, and
 * they are not the same thing. Renaming leaves the address alone on purpose:
 * a deck whose address moved is a deck whose every shared link, saved page
 * and history entry now names something else, which is the same reason a deck
 * coming back out of the crypt is refused rather than quietly renamed. So the
 * pen changes the title and says, in the editor where the question actually
 * occurs to somebody, that the link they have shared still works.
 *
 * The shape is `PilotLine`'s, one floor up: shown as content, edited in
 * place, saved on Enter or the button, cancelled on Escape. The controls are
 * the shared `.btn` family, so hover, focus and press are all answered
 * already (commandment 17), and the empty-name case is refused in the control
 * as well as at the server — a deck with a blank name does not render blank,
 * it renders its address, which would look like a name somebody chose.
 */
import { useEffect, useRef, useState } from 'react'

import { api, errorMessage, type DeckRef } from '../lib/api'
import { QuillGlyph } from './glyphs'

/**
 * The longest name the library will hold, and the same number the server
 * enforces (`deckedit.DeckNameMax`, where the measurement lives: the longest
 * name a legendary creature prints under, plus the theme people append).
 *
 * Held here as well so the count under the box is the real one. It is
 * deliberately *not* a `maxLength` on the input: a hard stop silently eats
 * the tail of a pasted name and leaves somebody looking at a name they did
 * not type with nothing saying why.
 */
export const DECK_NAME_MAX = 80

/** Show the count only once it is information — near the cap, or past it. */
const COUNT_FROM = DECK_NAME_MAX - 15

export function DeckNameHeading({ name, writable, deckRef, onRenamed }: {
  /** The deck's current name, as the server serves it. */
  name: string
  /** Whether this reader may write this deck. A reader gets the heading and
   *  nothing else — an invitation to rename a deck you cannot rename is
   *  furniture. */
  writable: boolean
  deckRef: DeckRef
  /** Re-read the deck, so the shelf's copy and this page's agree. */
  onRenamed: () => void
}) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(name)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const box = useRef<HTMLInputElement>(null)

  // Focus and select on open: the name is already there, and the common edit
  // is replacing it rather than appending to it.
  useEffect(() => {
    if (editing) box.current?.select()
  }, [editing])

  const trimmed = text.trim().replace(/\s+/g, ' ')
  const tooLong = [...trimmed].length > DECK_NAME_MAX
  const unchanged = trimmed === name
  const ready = trimmed.length > 0 && !tooLong && !unchanged

  function open() {
    setText(name)
    setError(null)
    setEditing(true)
  }

  async function save() {
    if (!ready) return
    setBusy(true)
    setError(null)
    try {
      await api.setDeckField(deckRef, 'name', trimmed)
      setEditing(false)
      onRenamed()
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  if (!editing) {
    return (
      <>
        <h1 className="text-2xl font-semibold tracking-tight">{name}</h1>
        {writable && (
          <button type="button" onClick={open}
                  className="btn btn-quiet btn-xs deck-rename-pen"
                  aria-label={`Rename ${name}`}>
            <QuillGlyph />
            Rename
          </button>
        )}
      </>
    )
  }

  // `w-full` and **no max-width on this box**, so the row this sits in
  // actually wraps: the heading's badges are its flex siblings, and a wrapper
  // capped at `max-w-2xl` resolves `w-full` to the cap and leaves room beside
  // it, which put "theory / draft / valid" up the editor's right-hand side.
  // The cap belongs on the field, which is what is being sized.
  return (
    <div className="deck-rename w-full">
      <label className="flex max-w-2xl flex-col gap-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          What this deck is called
        </span>
        <input ref={box} value={text} autoFocus
               onChange={(e) => setText(e.target.value)}
               onKeyDown={(e) => {
                 if (e.key === 'Enter') void save()
                 if (e.key === 'Escape') setEditing(false)
               }}
               aria-label="Deck name"
               aria-invalid={tooLong || undefined}
               placeholder="Gyome, Master Chef — Food"
               className="deck-rename-box w-full rounded-md px-2.5 py-1.5" />
      </label>

      <p className="mt-1.5 max-w-2xl text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
        Only what it is called. Where this deck lives does not move, so a link
        you have already shared still opens it.
      </p>

      {trimmed.length >= COUNT_FROM && (
        <p className="mt-1 text-xs tabular"
           style={{ color: tooLong ? 'var(--status-critical)' : 'var(--text-muted)' }}>
          {[...trimmed].length} of {DECK_NAME_MAX} characters
          {tooLong && <> — what the deck is <em>doing</em> belongs in its
            description, which has room for it.</>}
        </p>
      )}

      {error && (
        <p role="alert" className="mt-1.5 text-xs"
           style={{ color: 'var(--status-critical)' }}>{error}</p>
      )}

      <div className="mt-2 flex flex-wrap items-center gap-2">
        <button type="button" onClick={() => void save()}
                disabled={busy || !ready}
                className="btn btn-primary btn-accent-1 btn-sm">
          {busy ? 'Saving…' : 'Save name'}
        </button>
        <button type="button" onClick={() => setEditing(false)} disabled={busy}
                className="btn btn-ghost btn-xs">
          Cancel
        </button>
        {/* Why the button is off, said out loud. A disabled control with no
            reason beside it is a control that reads as broken. */}
        {!busy && !ready && !tooLong && (
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {trimmed.length === 0
              ? 'A deck needs a name.'
              : 'That is the name it already has.'}
          </span>
        )}
      </div>
    </div>
  )
}
