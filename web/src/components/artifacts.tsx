import { useCallback, useEffect, useState } from 'react'

import { api, errorMessage } from '../lib/api'
import type { DeckArtifacts, DeckDetail, DeckRef } from '../lib/api'
import { Badge, Caveat, ErrorNote, Spinner } from './ui'

/**
 * The five deliverables (rule 3), read and rebuilt from the deck page.
 *
 * The hosted half of `mtglab decks build`, and it exists because the ruling
 * that made `mtglab ui` a development harness turned "the CLI can do it" into
 * a gap. Until 2026-08-21 rebuilding a deployed deck's artifacts meant opening
 * a shell on the instance — the laptop coupling the volume ruling ended — and
 * the page had been *describing* these files in four places without being able
 * to produce or open one.
 *
 * **`baseline` is the reason this is a panel and not a button.** Every
 * artifact on the volume was eight days older than its deck when this was
 * built, and nothing in the app could say so. The server compares the stored
 * snapshot against the deck; this renders that answer and never recomputes it,
 * the same readout rule the labels editor and the stance dial follow.
 */

/** What each file is for, in the module docstring's own numbering. Copy, not
 *  data: these five are fixed by rule 3, and a served vocabulary would be
 *  ceremony for a list that changes when the rule does. */
const BLURBS: Record<string, string> = {
  'primer-quick.md': 'One page. Enough to get somebody playing the deck.',
  'primer-advanced.md': 'Lines, sequencing, matchups and how it loses.',
  'decklist-annotated.md': 'The 99, each with the reason it holds its slot.',
  'moxfield.txt': 'Bulk import — paste it straight into a deckbuilder.',
  'swaps.md': 'What changed since the last build, and what it costs.',
}

const BASELINE: Record<DeckArtifacts['baseline'], {
  tone: 'good' | 'warning'; label: string; note: string
}> = {
  current: {
    tone: 'good', label: 'Current',
    note: 'These were built from the deck exactly as it stands.',
  },
  different: {
    tone: 'warning', label: 'Out of date',
    note: 'The deck has changed since these were built. Rebuild to catch them up — '
      + 'the swap list will say what moved.',
  },
  unknown: {
    tone: 'warning', label: 'Unknown',
    note: 'These were built before the deck started keeping a baseline, so nothing '
      + 'can say whether they still match. A rebuild settles it.',
  },
}

export function DeckArtifactsPanel({ deck, deckRef }: {
  deck: DeckDetail
  deckRef: DeckRef
}) {
  const [state, setState] = useState<DeckArtifacts | null>(null)
  const [error, setError] = useState('')
  const [building, setBuilding] = useState(false)
  const [open, setOpen] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setState(await api.deckArtifacts(deckRef))
      setError('')
    } catch (e) {
      setError(errorMessage(e))
    }
  }, [deckRef])

  useEffect(() => { void load() }, [load])

  async function build(force: boolean) {
    setBuilding(true)
    setError('')
    try {
      setState(await api.buildArtifacts(deckRef, force))
      setOpen(null)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBuilding(false)
    }
  }

  if (!state && !error) return <Spinner label="Reading the shelf…" />

  // ADR 13, said where somebody would go looking for it. The deck page's own
  // draft banner offers the promotion; this says why the shelf is bare.
  if (state && !state.buildable) {
    return (
      <Caveat>
        A draft's artifacts stay shut. They are the shareable surface, and a
        primer that quietly omits the argument for a card reads exactly like one
        that had it — so the way out is to write the rationales and promote the
        deck, never a flag.
      </Caveat>
    )
  }

  const built = state?.artifacts ?? []
  const baseline = state ? BASELINE[state.baseline] : null

  return (
    <div className="space-y-4">
      <Caveat>
        The five deliverables, generated from <code>deck.yaml</code> and never
        written by hand. Rebuilding is cheap and safe: the deck itself is not
        touched, and the notes that carry the thinking live in the deck file, so
        nothing you wrote can be lost by regenerating.
      </Caveat>

      <div className="flex flex-wrap items-center gap-3">
        {baseline && built.length > 0 && (
          <>
            <Badge tone={baseline.tone}>{baseline.label}</Badge>
            <span className="text-sm" style={{ color: 'var(--text-secondary)' }}>
              {baseline.note}
            </span>
          </>
        )}
        {built.length === 0 && (
          <span className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            This deck has never been built.
          </span>
        )}
      </div>

      {deck.writable && (
        <div className="flex flex-wrap items-center gap-2">
          <button onClick={() => { void build(false) }} disabled={building}
                  className="btn btn-primary btn-accent-1 btn-sm">
            {building ? 'Building…' : built.length ? 'Rebuild' : 'Build the five'}
          </button>
          {/* Offered only once the gate has actually refused, so the flag is
              an answer to something rather than a standing invitation to
              ignore the gate. */}
          {error && error.includes('gate') && (
            <button onClick={() => { void build(true) }} disabled={building}
                    className="btn btn-quiet btn-sm">
              Build anyway
            </button>
          )}
        </div>
      )}

      {error && <ErrorNote>{error}</ErrorNote>}

      {built.length > 0 && (
        <div className="grid gap-3 md:grid-cols-2">
          {built.map(a => (
            <ArtifactCard key={a.name} artifact={a} deckRef={deckRef}
                          open={open === a.name}
                          onToggle={() => setOpen(open === a.name ? null : a.name)} />
          ))}
        </div>
      )}
    </div>
  )
}

function ArtifactCard({ artifact, deckRef, open, onToggle }: {
  artifact: DeckArtifacts['artifacts'][number]
  deckRef: DeckRef
  open: boolean
  onToggle: () => void
}) {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    if (!open || text !== null) return
    let live = true
    void (async () => {
      try {
        const got = await api.deckArtifact(deckRef, artifact.name)
        if (live) setText(got.text)
      } catch (e) {
        if (live) setError(errorMessage(e))
      }
    })()
    return () => { live = false }
  }, [open, text, deckRef, artifact.name])

  async function copy() {
    if (text === null) return
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1600)
    } catch {
      // A clipboard the browser will not hand over is not an error worth a
      // red box — the text is on screen and can be selected.
      setCopied(false)
    }
  }

  return (
    <section className="card-surface rounded-xl p-4">
      <button onClick={onToggle}
              className="strip-tab flex w-full items-baseline justify-between gap-3
                         rounded-lg px-2 py-1 text-left">
        <span className="font-mono text-sm font-semibold">{artifact.name}</span>
        <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
          {formatSize(artifact.size)} · {open ? 'hide' : 'read'}
        </span>
      </button>

      <p className="mt-2 text-xs" style={{ color: 'var(--text-secondary)' }}>
        {BLURBS[artifact.name] ?? 'A generated deliverable.'}
      </p>

      {open && (
        <div className="mt-3">
          {error && <ErrorNote>{error}</ErrorNote>}
          {!error && text === null && <Spinner label="Fetching…" />}
          {text !== null && (
            <>
              <div className="mb-2 flex justify-end">
                <button onClick={() => { void copy() }} className="btn btn-quiet btn-xs">
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
              <pre className="max-h-96 overflow-auto rounded-lg p-3 text-xs leading-relaxed"
                   style={{ background: 'var(--gridline)', color: 'var(--text-primary)' }}>
                {text}
              </pre>
            </>
          )}
        </div>
      )}
    </section>
  )
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  return `${(bytes / 1024).toFixed(1)} kB`
}
