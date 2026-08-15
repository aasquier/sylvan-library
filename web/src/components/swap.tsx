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
 * The server re-checks everything anyway (`service.swap_card`: the incoming
 * card must exist, be Commander-legal, fit the identity, not already be in
 * the deck) — this component's job is composing the request, not vetting it.
 */
import { useState } from 'react'
import { api, errorMessage, type DeckRef } from '../lib/api'
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
                className="rounded-lg px-3 py-1.5 text-xs font-medium disabled:opacity-50"
                style={{ background: 'var(--series-1)', color: '#fff' }}>
          {busy ? 'Swapping…' : 'Apply swap'}
        </button>
        <button onClick={onCancel}
                className="text-xs underline"
                style={{ color: 'var(--text-muted)' }}>
          Cancel
        </button>
        <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
          Writes deck.yaml. Commit it — deck history is git history.
        </span>
      </div>
    </div>
  )
}
