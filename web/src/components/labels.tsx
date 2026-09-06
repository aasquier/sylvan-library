import { useEffect, useState } from 'react'

import { api, errorMessage } from '../lib/api'
import type {
  ClaudeStatus, DeckDescriptionDraft, DeckDetail, DeckRef, ThemeVocabulary,
} from '../lib/api'
import { claudeCanAnswer } from '../lib/stance'
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
export function DeckLabels({ deck, deckRef, claude, onRefresh }: {
  deck: DeckDetail
  deckRef: DeckRef
  /** Whether Claude can answer here at all — null until known. An absent
   *  control is how ADR 15's "off is a real position" renders; a control that
   *  can only refuse is how it renders wrong. */
  claude?: ClaudeStatus | null
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
        <LabelEditor deck={deck} deckRef={deckRef} claude={claude}
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
function LabelEditor({ deck, deckRef, claude, onDone, onRefresh }: {
  deck: DeckDetail
  deckRef: DeckRef
  claude?: ClaudeStatus | null
  onDone: () => void
  onRefresh: () => void
}) {
  const [vocab, setVocab] = useState<ThemeVocabulary | null>(null)
  const [chosen, setChosen] = useState<string[]>(deck.themes ?? [])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  // What Claude read off the list, when it has been asked. `null` is "not
  // asked", which is a different thing from an empty answer and reads
  // differently on the screen.
  const [read, setRead] = useState<DeckDescriptionDraft | null>(null)
  const [asking, setAsking] = useState(false)

  /**
   * Ask Claude which of these words fit, and tick them.
   *
   * **It ticks, it does not save.** The chips it turns on are chips, sitting in
   * the same editor with the same save button somebody was already looking at,
   * and the row that appears says which ones it added so they can be unticked
   * one at a time. The pen stays where the panel above the description keeps
   * it: it drafts, you keep the pen.
   *
   * **Added to what is already chosen, never instead of it.** The obvious
   * implementation replaces the selection, and the obvious implementation
   * throws away the labels somebody chose by hand a moment ago -- for a
   * suggestion they have not read yet.
   *
   * No stance is sent, so the server resolves this deck's own default and
   * clamps it to the deployment's ceiling; what actually applied comes back in
   * the report, and `asked: false` with a reason is a real answer rather than
   * a failure (a dial set to stay silent costs nothing and says so).
   */
  async function ask() {
    try {
      setError(null)
      setAsking(true)
      const got = await api.describeDeck(deckRef, {})
      setRead(got)
      setChosen(prev => [...new Set([...prev, ...(got.themes ?? [])])])
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setAsking(false)
    }
  }

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
        category. They stay the deck&rsquo;s own words
        {claudeCanAnswer(claude)
          ? <>: Claude will read the list and tick what it recognises, and every
              one of them is yours to untick before you save.</>
          : <>, so nothing here is guessed from the decklist.</>}
      </p>

      {/* Offered only where it can be answered. An instance with no key, a
          build without the feature, or a dial set to stay silent are three
          ordinary states, and a button that refuses in all three tells a
          newcomer the site is broken when it is doing what it was told
          (ADR 15, commandment 2). */}
      {claudeCanAnswer(claude) && (
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <button type="button" onClick={() => { void ask() }} disabled={asking || saving}
                  className="btn btn-quiet btn-xs">
            {asking ? 'Reading the list…' : read ? 'Ask again' : 'Ask Claude which fit'}
          </button>
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
            It ticks boxes. Nothing is saved until you press save.
          </span>
        </div>
      )}

      {/* ADR 14 boundary 3: what Claude answered never shares a surface with
          the gate's output unlabelled, and the label names the system rather
          than anything that computes it (commandment 10). */}
      {read && !read.asked && (
        <p className="mt-2 text-xs" role="status" style={{ color: 'var(--text-muted)' }}>
          {read.reason}
        </p>
      )}
      {read?.asked && (
        <div className="mt-2 space-y-1 text-xs" role="status">
          <p style={{ color: 'var(--text-muted)' }}>
            {read.themes.length > 0
              ? <>Claude read the list and ticked{' '}
                  <span style={{ color: 'var(--text-secondary)' }}>
                    {read.themes.join(', ')}
                  </span>. Untick anything it got wrong.</>
              : <>Claude read the list and found nothing in this vocabulary that
                  fits. The words below are still yours to pick from.</>}
          </p>
          {(read.themes_dropped ?? []).length > 0 && (
            // Counted rather than swallowed: "it read four and could file two"
            // is the difference between a thin answer and a narrow vocabulary,
            // and only one of those is worth doing anything about.
            <p style={{ color: 'var(--text-muted)' }}>
              It also read{' '}
              <span style={{ color: 'var(--text-secondary)' }}>
                {(read.themes_dropped ?? []).join(', ')}
              </span>, which this library has no label for yet.
            </p>
          )}
        </div>
      )}

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
