import { useEffect, useState } from 'react'

import { api, errorMessage } from '../lib/api'
import type { DeckDetail, DeckRef, ThemeVocabulary } from '../lib/api'
import { Badge, ErrorNote } from './ui'

/**
 * What a deck says it is, and the control that says it.
 *
 * ADR 37's declared `themes` are the deck's identity — several per deck, from
 * a hand-curated vocabulary that grows only by somebody reading and editing
 * `model.THEMES`. The `archetype` the rating boards group by is a **reading**
 * of them, never a second declaration, which is why nothing here writes it.
 *
 * This is the control the migration exposed as missing: labels could only be
 * applied from the CLI, so relabelling the deployed library meant
 * `fly ssh console` — exactly the laptop coupling the 2026-08-21 volume ruling
 * ended. The library lives on the volume; the editor for it belongs in the app.
 *
 * **The archetype is not predicted while you edit.** It is a readout of what
 * the server resolved, the same rule the stance dial follows: a second copy of
 * the worst-piloted-wins reading, living in TypeScript, would disagree with
 * the served one silently and nobody would learn which was right. So editing
 * names the class words as class words — that is a fact about the vocabulary,
 * served in `archetypes` — and the deck shows its archetype again once saved.
 */
export function DeckLabels({ deck, deckRef, onRefresh }: {
  deck: DeckDetail
  deckRef: DeckRef
  onRefresh: () => void
}) {
  const [editing, setEditing] = useState(false)

  // Read through a default rather than off the deck. The wire always carries
  // `themes`, so the type is honest — but a *deploy* changes both halves and
  // the browser is the half that lies: a freshly-served bundle can put this
  // question to a server that has not restarted yet, and `.length` on an
  // absent list would take the whole deck page down over a label line. This
  // is what `as unknown as Deck` in the page's own tests hid until it ran.
  const themes = deck.themes ?? []

  // An unlabelled deck somebody else owns says nothing: "this deck has no
  // themes" is not a fact worth a line on a deck the reader cannot label.
  if (themes.length === 0 && !deck.writable) return null

  return (
    <div className="mt-3">
      {editing ? (
        <LabelEditor deck={deck} deckRef={deckRef}
                     onDone={() => setEditing(false)}
                     onRefresh={onRefresh} />
      ) : (
        <div className="flex flex-wrap items-center gap-2 text-sm"
             style={{ color: 'var(--text-muted)' }}>
          {themes.length > 0 ? (
            <>
              <span>Themes</span>
              {themes.map(t => (
                <Badge key={t}>{t}</Badge>
              ))}
              {deck.archetype && (
                <span className="text-xs">
                  — the boards read this as{' '}
                  <span style={{ color: 'var(--text-secondary)' }}>
                    {deck.archetype}
                  </span>
                </span>
              )}
            </>
          ) : (
            <span>No themes declared.</span>
          )}
          {deck.writable && (
            <button type="button" className="btn btn-ghost btn-xs"
                    onClick={() => setEditing(true)}>
              {themes.length > 0 ? 'Change themes' : 'Declare themes'}
            </button>
          )}
        </div>
      )}
    </div>
  )
}

/**
 * The vocabulary as toggles.
 *
 * Class words are shown first and marked, because ticking one is the only
 * label choice here that carries a consequence beyond identity: it decides
 * which rating board this deck's Forge results may share.
 */
function LabelEditor({ deck, deckRef, onDone, onRefresh }: {
  deck: DeckDetail
  deckRef: DeckRef
  onDone: () => void
  onRefresh: () => void
}) {
  const [vocab, setVocab] = useState<ThemeVocabulary | null>(null)
  const [chosen, setChosen] = useState<string[]>(deck.themes ?? [])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let live = true
    api.themes()
      .then(v => { if (live) setVocab(v) })
      .catch(e => { if (live) setError(errorMessage(e)) })
    return () => { live = false }
  }, [])

  function toggle(theme: string) {
    setChosen(prev => prev.includes(theme)
      ? prev.filter(t => t !== theme)
      : [...prev, theme])
  }

  async function save() {
    try {
      setError(null)
      setSaving(true)
      // Sorted, so a deck.yaml diff shows what changed rather than what was
      // clicked in what order.
      await api.setDeckField(deckRef, 'themes', [...chosen].sort())
      onDone()
      onRefresh()
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setSaving(false)
    }
  }

  if (error && !vocab) return <ErrorNote>{error}</ErrorNote>
  if (!vocab) {
    return (
      <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
        Fetching the vocabulary…
      </p>
    )
  }

  // `archetypes` is used as the *order* here, not just as a membership test.
  // The server sends it best-Forge-piloted first, and that gradient is the
  // whole reason the class exists — rendering these four alphabetically threw
  // away the one thing their order was carrying, and left the sentence below
  // ("the hardest of them to pilot") pointing at nothing on screen.
  const classWords = vocab.archetypes
  const rest = vocab.themes.filter(t => !vocab.archetypes.includes(t))
  const chosenClass = classWords.filter(t => chosen.includes(t))

  return (
    <div className="rounded-lg p-3"
         style={{ border: '1px solid var(--hairline)' }}>
      <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
        What is this deck about?
      </p>
      <p className="mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
        Pick as many as are true — themes are an identity, not a single
        category. They are the deck&rsquo;s own words, so nothing here is
        guessed from the decklist.
      </p>

      <p className="mt-3 text-xs font-medium"
         style={{ color: 'var(--text-secondary)' }}>
        How it plays
      </p>
      <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
        These also decide which rating board the deck&rsquo;s games are
        grouped into, because the engine pilots them differently — easiest
        first, hardest last. Declare every one that is true; the board reads
        the hardest of them to pilot.
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        {classWords.map(t => (
          <ThemeChip key={t} theme={t} on={chosen.includes(t)}
                     onToggle={() => toggle(t)} />
        ))}
      </div>

      <p className="mt-4 text-xs font-medium"
         style={{ color: 'var(--text-secondary)' }}>
        What it is about
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        {rest.map(t => (
          <ThemeChip key={t} theme={t} on={chosen.includes(t)}
                     onToggle={() => toggle(t)} />
        ))}
      </div>

      <p className="mt-3 text-xs" style={{ color: 'var(--text-muted)' }}>
        {chosen.length === 0
          ? 'Nothing declared — saving now would clear this deck’s labels.'
          : `${chosen.length} declared.`}
        {chosenClass.length > 0 && (
          <> Its board will be read from {chosenClass.join(', ')}.</>
        )}
      </p>

      {error && <div className="mt-2"><ErrorNote>{error}</ErrorNote></div>}

      <div className="mt-3 flex flex-wrap gap-2">
        <button type="button" className="btn btn-primary btn-sm"
                disabled={saving} onClick={save}>
          {saving ? 'Saving…' : 'Save themes'}
        </button>
        <button type="button" className="btn btn-ghost btn-sm"
                disabled={saving} onClick={onDone}>
          Cancel
        </button>
      </div>
    </div>
  )
}

function ThemeChip({ theme, on, onToggle }: {
  theme: string
  on: boolean
  onToggle: () => void
}) {
  return (
    <button type="button" aria-pressed={on} onClick={onToggle}
            className={`chip-toggle rounded-full px-3 py-1 text-xs${on ? ' is-on' : ''}`}>
      {theme}
    </button>
  )
}
