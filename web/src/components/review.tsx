/**
 * The deck review: the slot argument (ADR 25), swept over a selection.
 *
 * One Claude call per selected card, which is why everything about this panel
 * is shaped around consent and cost: nothing runs on mount, the selection is
 * explicit (spells preselected, lands left out until asked for), the button
 * states the count, and the run is a background job with a progress bar —
 * one job, sequential, so a sweep never occupies the whole NET lane.
 *
 * The job id is kept per deck in localStorage, so a reload reattaches to the
 * run already in flight rather than paying for a second one (the server also
 * dedupes an identical selection in flight, which covers the second tab).
 *
 * The results queue renders each card through `SlotArgumentBody` — the same
 * component the per-card panel uses — so the rules arrive with it: a charge
 * cites a fact or was dropped, an alternative was pool-checked or was named
 * in the dropped lines, and the only way any of it changes the deck is the
 * swap composer, with a why the user typed (ADR 11).
 */
import { useEffect, useRef, useState } from 'react'
import {
  ApiError, api, errorMessage, followJob,
  type DeckDetail, type DeckRef, type DeckReviewResult,
} from '../lib/api'
import { categoryLabel } from '../lib/mtg'
import { effectivePin, useStance } from '../lib/stance'
import type { ClaudeStatus } from '../lib/api'
import { ErrorNote } from './ui'
import { SlotArgumentBody } from './deckedit'

/** Where a running sweep's job id lives, per deck, so a reload reattaches. */
function storageKey(ref: DeckRef): string {
  return `mtglab-deck-review:${ref.owner}/${ref.slug}`
}

export function DeckReviewPanel({ deck, deckRef, status, onChanged }: {
  deck: DeckDetail
  deckRef: DeckRef
  /** The resolved Claude status, for the pin check — the caller already
   *  gates mounting this on Claude being reachable. */
  status: ClaudeStatus | null
  /** Called after a swap landed, so the page refreshes the 99. */
  onChanged: () => void
}) {
  const [open, setOpen] = useState(false)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [result, setResult] = useState<DeckReviewResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [pin] = useStance()
  const poller = useRef<{ cancel: () => void } | null>(null)

  const spells = deck.cards.filter((c) => c.category !== 'land')

  // Reattach on mount: a job id in storage is a sweep this deck paid for and
  // may still be running. Follow it rather than forgetting it; a 404 means
  // the server restarted and the run died with it (`api/jobs.py`).
  useEffect(() => {
    const stored = localStorage.getItem(storageKey(deckRef))
    if (!stored) return
    setOpen(true)
    follow(stored)
    return () => poller.current?.cancel()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount only
  }, [])

  function follow(id: string) {
    poller.current?.cancel()
    const run = followJob(id, (job) => {
      setProgress({ done: job.done, total: job.total })
    }, 2000)
    poller.current = run
    run.promise
      .then((job) => {
        setResult(job.result as DeckReviewResult)
        setProgress(null)
        localStorage.removeItem(storageKey(deckRef))
      })
      .catch((e) => {
        setProgress(null)
        localStorage.removeItem(storageKey(deckRef))
        setError(e instanceof ApiError && e.status === 404
          ? 'That run is gone — the server restarted while it was working. '
            + 'Start it again when you are ready.'
          : errorMessage(e))
      })
  }

  async function start() {
    const cards = deck.cards.filter((c) => picked.has(c.name)).map((c) => c.name)
    if (cards.length === 0) return
    setError(null)
    setResult(null)
    try {
      const job = await api.argueDeck(deckRef, {
        cards, stance: effectivePin(pin, status),
      })
      if (job.status === 'done') {
        // Born finished — a stance of off, answered without a poll.
        setResult(job.result as DeckReviewResult)
        return
      }
      localStorage.setItem(storageKey(deckRef), job.id)
      setProgress({ done: job.done, total: job.total })
      follow(job.id)
    } catch (e) {
      setError(errorMessage(e))
    }
  }

  function toggle(name: string) {
    setPicked((p) => {
      const next = new Set(p)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  function pickAll(names: string[]) {
    setPicked(new Set(names))
  }

  if (!open) {
    return (
      <button
        onClick={() => {
          setOpen(true)
          // Spells preselected: the one-press flow is "review the deck", and
          // arguing 60 basic lands is spend with nothing to say. Lands stay
          // selectable below.
          setPicked(new Set(spells.map((c) => c.name)))
        }}
        className="rounded-lg px-3 py-1.5 text-xs font-medium"
        style={{ background: 'var(--gridline)', color: 'var(--text-primary)' }}>
        Review with Claude
      </button>
    )
  }

  const running = progress !== null
  const count = picked.size

  // Selection list, grouped the way the page's default view groups.
  const groups = new Map<string, typeof deck.cards>()
  for (const card of deck.cards) {
    const bucket = groups.get(card.category) ?? []
    bucket.push(card)
    groups.set(card.category, bucket)
  }

  return (
    <section className="card-surface space-y-3 rounded-xl p-4 text-xs">
      <div className="flex flex-wrap items-center gap-3">
        <strong className="text-sm" style={{ color: 'var(--text-primary)' }}>
          Review with Claude
        </strong>
        <span style={{ color: 'var(--text-muted)' }}>
          The case against each selected slot — one Claude conversation per
          card, so a big selection takes minutes. It runs in the background;
          you can leave and come back.
        </span>
        <button onClick={() => { poller.current?.cancel(); setOpen(false) }}
                className="ml-auto text-[11px]"
                style={{ color: 'var(--text-muted)' }}>
          Close
        </button>
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}

      {!running && !result && (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <button onClick={() => pickAll(spells.map((c) => c.name))}
                    className="rounded-md px-2 py-1"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              All spells
            </button>
            <button onClick={() => pickAll(deck.cards.map((c) => c.name))}
                    className="rounded-md px-2 py-1"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              Everything
            </button>
            <button onClick={() => setPicked(new Set())}
                    className="rounded-md px-2 py-1"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              None
            </button>
          </div>

          <div className="max-h-64 space-y-2 overflow-y-auto rounded-md p-2"
               style={{ background: 'var(--surface-1)' }}>
            {[...groups.entries()].map(([category, cards]) => {
              const names = cards.map((c) => c.name)
              const allIn = names.every((n) => picked.has(n))
              return (
                <div key={category}>
                  <label className="flex cursor-pointer items-center gap-2 font-medium">
                    <input type="checkbox" checked={allIn}
                           onChange={() => setPicked((p) => {
                             const next = new Set(p)
                             if (allIn) names.forEach((n) => next.delete(n))
                             else names.forEach((n) => next.add(n))
                             return next
                           })} />
                    {categoryLabel(category)}
                    <span style={{ color: 'var(--text-muted)' }}>{cards.length}</span>
                  </label>
                  <div className="ml-5 mt-1 flex flex-wrap gap-x-4 gap-y-1">
                    {cards.map((c) => (
                      <label key={c.name}
                             className="flex cursor-pointer items-center gap-1.5">
                        <input type="checkbox" checked={picked.has(c.name)}
                               onChange={() => toggle(c.name)} />
                        <span style={{ color: 'var(--text-secondary)' }}>{c.name}</span>
                      </label>
                    ))}
                  </div>
                </div>
              )
            })}
          </div>

          <div className="flex flex-wrap items-center gap-3">
            <button onClick={() => void start()} disabled={count === 0}
                    className="rounded-lg px-3 py-1.5 font-medium disabled:opacity-50"
                    style={{ background: 'var(--series-1)', color: '#fff' }}>
              Argue {count} slot{count === 1 ? '' : 's'}
            </button>
            {/* The cost, stated before the click rather than discovered after:
                the count is the number of paid conversations. */}
            <span style={{ color: 'var(--text-muted)' }}>
              {count === 0
                ? 'Nothing selected.'
                : `${count} card${count === 1 ? '' : 's'} → ${count} Claude `
                  + `conversation${count === 1 ? '' : 's'}, one at a time.`}
            </span>
          </div>
        </>
      )}

      {running && progress && (
        <div className="space-y-1">
          <div className="h-2 w-full overflow-hidden rounded-full"
               style={{ background: 'var(--gridline)' }}>
            <div className="h-full rounded-full transition-all"
                 style={{
                   width: progress.total
                     ? `${Math.round((progress.done / progress.total) * 100)}%`
                     : '0%',
                   background: 'var(--series-1)',
                 }} />
          </div>
          <p style={{ color: 'var(--text-muted)' }}>
            {progress.done} of {progress.total} slots argued — this carries on
            if you reload or close the tab.
          </p>
        </div>
      )}

      {result && !result.asked && (
        <p style={{ color: 'var(--text-muted)' }}>{result.reason}</p>
      )}

      {result?.asked && (
        <div className="space-y-3">
          {result.reports.map((report) => (
            <div key={report.card} className="space-y-2 border-t pt-3"
                 style={{ borderColor: 'var(--hairline)' }}>
              <strong className="text-sm" style={{ color: 'var(--text-primary)' }}>
                {report.card}
              </strong>
              <SlotArgumentBody deck={deckRef} report={report}
                                writable={deck.writable} onSwapped={onChanged} />
            </div>
          ))}
          {Object.entries(result.errors).length > 0 && (
            <div className="border-t pt-3" style={{ borderColor: 'var(--hairline)' }}>
              {Object.entries(result.errors).map(([card, message]) => (
                <p key={card} style={{ color: 'var(--status-warning)' }}>
                  {card}: {message}
                </p>
              ))}
            </div>
          )}
          <button onClick={() => { setResult(null) }}
                  className="rounded-md px-2 py-1"
                  style={{ border: '1px solid var(--hairline)',
                           color: 'var(--text-secondary)' }}>
            Pick cards again
          </button>
        </div>
      )}
    </section>
  )
}
