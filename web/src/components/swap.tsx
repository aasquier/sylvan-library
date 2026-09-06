/**
 * The swap composer: one card out, one card in, and the sentence that makes
 * it legal.
 *
 * Extracted from the validation tab so the slot argument can offer the same
 * ending — a suggestion should be one click from becoming the card of record,
 * wherever the suggestion came from. What must not change with the extraction
 * is the shape rule 4 gives it (ADR 8, ADR 11): the `why` box opens empty,
 * its placeholder is a question rather than a draft, and the button stays
 * disabled until a human has written something. The deterministic shortlist
 * and Claude's alternatives are different advisers, but the write path is the
 * same and so is the requirement.
 *
 * The server re-checks everything anyway (the swap route: the incoming card
 * must exist, be Commander-legal, fit the identity, not already be among the
 * 99 — a card standing on the deck's own swap board is not a duplicate any
 * more, it is promoted off the board by the route's second door) — this
 * component's job is composing the request, not vetting it.
 */
import { useState } from 'react'
import { api, errorMessage, type CardOffer, type DeckRef } from '../lib/api'
import { CardFinder } from './cardfinder'
import { ErrorNote } from './ui'

export function SwapComposer({ deck, out, into, onDone, onCancel }: {
  deck: DeckRef
  /** The card leaving the deck. */
  out: string
  /** The card taking its slot — named by the caller, never inferred. */
  into: string
  /** Called after the server accepted the swap. */
  onDone: () => void | Promise<void>
  onCancel: () => void
}) {
  const [why, setWhy] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function apply() {
    if (!why.trim()) return
    setBusy(true)
    setError(null)
    try {
      await api.swapCard(deck, { out, into, why: why.trim() })
      await onDone()
    } catch (e) {
      setError(errorMessage(e))
      setBusy(false)
    }
  }

  return (
    <div className="space-y-2 rounded-lg p-3" style={{ background: 'var(--surface-1)' }}>
      <p className="text-xs font-medium">
        Swap {out} → {into}
      </p>
      {/* Rule 4: every card carries a rationale, and one written by the tool
          is exactly the empty justification that rule exists to prevent. So
          the button stays disabled until a human writes one. */}
      <textarea
        value={why}
        onChange={(e) => setWhy(e.target.value)}
        rows={3}
        placeholder="Why does this card earn the slot? Required — the gate will not accept a card without a rationale."
        className="w-full rounded-md px-2 py-1.5 text-xs outline-none focus:ring-2"
        style={{ background: 'var(--surface-2, var(--gridline))',
                 color: 'var(--text-primary)',
                 border: '1px solid var(--hairline)' }}
      />
      {error && <ErrorNote>{error}</ErrorNote>}
      <div className="flex items-center gap-2">
        <button onClick={() => void apply()}
                disabled={!why.trim() || busy}
                className="btn btn-primary btn-accent-1 btn-sm">
          {busy ? 'Swapping…' : 'Apply swap'}
        </button>
        <button onClick={onCancel}
                className="btn btn-ghost btn-xs">
          Cancel
        </button>
        <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
          Writes deck.yaml. The History tab records the swap.
        </span>
      </div>
    </div>
  )
}

/**
 * The straight swap, started from the card rather than from a suggestion.
 *
 * `SwapComposer` above has always needed both names handed to it — its two
 * callers are the banned-card shortlist and the slot argument, where the
 * replacement is the very thing being offered. This wraps it for the deck
 * page's action bar, where only the *outgoing* card is known: the finder
 * chooses the incoming one, and the moment it is chosen the composer above
 * takes over, contract intact. Wrapping rather than loosening, deliberately:
 * a `SwapComposer` whose `into` could be empty would let a caller compose a
 * swap with no card in it.
 *
 * One sentence changes with the second door (the promotion): a picked card
 * that is standing on this deck's own swap board is not refused as a
 * duplicate any more — the server lifts it off the board and entombs the
 * outgoing card with its rationale. The composer says so quietly when it
 * happens, because "already in this deck" was the sentence this used to end
 * in and a person who has met it deserves to hear the rule changed.
 */
export function ReplaceComposer({ deck, out, identity, board, onDone, onCancel }: {
  deck: DeckRef
  /** The card leaving the 99 — named by the row the action bar picked. */
  out: string
  /** The deck's colour identity, so the finder marks an outside card at the
   *  moment it is chosen rather than after a rationale has been written. */
  identity: string[]
  /** The names standing on the deck's swap board, so the composer can say
   *  out loud when the picked card will be lifted from there. */
  board: string[]
  onDone: () => void | Promise<void>
  onCancel: () => void
}) {
  const [into, setInto] = useState<CardOffer | null>(null)
  const fromBoard = into !== null
    && board.some((n) => n.toLowerCase() === into.name.toLowerCase())

  return (
    <div className="mt-2 space-y-3 rounded-lg p-3"
         style={{ background: 'var(--surface-1)' }}>
      <CardFinder value={into} onChange={setInto} identity={identity}
                  label={`Card to swap in for ${out}`} />
      {fromBoard && (
        <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
          {into.name} is waiting on this deck&rsquo;s swap board — it will be
          lifted from there, and {out} goes to the graveyard with its reason
          kept.
        </p>
      )}
      {into !== null
        ? <SwapComposer deck={deck} out={out} into={into.name}
                        onDone={onDone} onCancel={onCancel} />
        : (
          <button type="button" onClick={onCancel} className="btn btn-ghost btn-xs">
            Cancel
          </button>
        )}
    </div>
  )
}
