import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, type ImportResult } from '../lib/api'
import { Badge, ErrorNote, ManaText, Spinner, TextField } from '../components/ui'

/**
 * Paste a decklist, see exactly what it resolves to, then create it.
 *
 * The preview is not a description of what import would do — it is the same
 * request with `dry_run`, so the resolution, the gate and the deck.yaml on
 * screen are the real ones. What you approve is what gets written.
 *
 * The deck arrives as a draft with an empty `why` on every card, and nothing
 * here offers to fill one in. That is ADR 13, and it is the reason this page
 * ends on a count of the work still owed rather than on a success message.
 */

/** `Arahbo — Cats` -> `arahbo-cats`, matching the slug the API will accept. */
function slugify(name: string): string {
  return name
    .toLowerCase()
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export default function Import() {
  const navigate = useNavigate()
  const [text, setText] = useState('')
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [commander, setCommander] = useState('')
  const [companion, setCompanion] = useState('')
  const [bracket, setBracket] = useState('')
  const [status, setStatus] = useState('theoretical')
  const [preview, setPreview] = useState<ImportResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState<'preview' | 'create' | null>(null)
  const [showYaml, setShowYaml] = useState(false)

  // Typing a name fills the slug, until the slug is edited by hand.
  const [slugTouched, setSlugTouched] = useState(false)
  const effectiveSlug = slugTouched ? slug : slugify(name)

  function body(dryRun: boolean) {
    return {
      slug: effectiveSlug,
      text,
      name: name.trim(),
      // One field, split on commas, because a partner pair is two commanders
      // and asking for two boxes for a case most decks do not have is worse.
      commander: commander.split(',').map((c) => c.trim()).filter(Boolean),
      companion: companion.trim(),
      bracket: bracket ? Number(bracket) : null,
      status,
      dry_run: dryRun,
    }
  }

  async function run(dryRun: boolean) {
    setBusy(dryRun ? 'preview' : 'create')
    setError(null)
    try {
      const result = await api.importDeck(body(dryRun))
      setPreview(result)
      if (result.created) navigate(`/decks/${result.slug}`)
    } catch (e: any) {
      setError(String(e.message ?? e))
      if (!dryRun) setPreview(null)
    } finally {
      setBusy(null)
    }
  }

  const lines = text.split('\n').filter((l) => l.trim()).length
  const ready = text.trim().length > 0 && effectiveSlug.length > 0

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Import a decklist</h1>
          <p className="mt-1 max-w-2xl text-sm" style={{ color: 'var(--text-secondary)' }}>
            Paste an export from Moxfield, Archidekt, Arena or anywhere else.
            Names are resolved against the local corpus — anything that does not
            resolve is reported, never guessed.
          </p>
        </div>
        <Link to="/" className="text-sm underline" style={{ color: 'var(--series-1)' }}>
          ← Back to the library
        </Link>
      </header>

      <div className="grid gap-6 lg:grid-cols-[1.3fr_1fr]">
        <section className="space-y-3">
          <label className="flex flex-col gap-1">
            <span className="text-[11px] font-medium uppercase tracking-wide"
                  style={{ color: 'var(--text-muted)' }}>
              Decklist
            </span>
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value)}
              rows={18}
              spellCheck={false}
              aria-label="Decklist"
              placeholder={'1 Arahbo, Roar of the World (C17) 27 *CMDR*\n1 Sol Ring\n36 Forest'}
              className="rounded-md p-3 font-mono text-xs leading-relaxed outline-none focus:ring-2"
              style={{
                background: 'var(--surface-1)',
                color: 'var(--text-primary)',
                border: '1px solid var(--hairline)',
              }}
            />
          </label>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {lines} non-empty line{lines === 1 ? '' : 's'}
          </p>
        </section>

        <section className="space-y-3">
          <TextField label="Deck name" value={name} onChange={setName}
                     placeholder="Arahbo — Cats" />
          <TextField label="Slug" value={effectiveSlug}
                     onChange={(v) => { setSlugTouched(true); setSlug(v) }}
                     placeholder="arahbo-cats" />
          <TextField label="Commander" value={commander} onChange={setCommander}
                     placeholder="left blank, the list's own is used" />
          <TextField label="Companion" value={companion} onChange={setCompanion}
                     placeholder="optional; sits outside the 100" />
          <div className="flex flex-wrap gap-3">
            <TextField label="Bracket" value={bracket} onChange={setBracket}
                       placeholder="1–5" />
            <label className="flex min-w-48 flex-1 flex-col gap-1">
              <span className="text-[11px] font-medium uppercase tracking-wide"
                    style={{ color: 'var(--text-muted)' }}>
                Status
              </span>
              <select
                value={status}
                onChange={(e) => setStatus(e.target.value)}
                aria-label="Status"
                className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
                style={{
                  background: 'var(--surface-1)',
                  color: 'var(--text-primary)',
                  border: '1px solid var(--hairline)',
                }}
              >
                <option value="theoretical">Theory — a list I am considering</option>
                <option value="built">Built — the cards are sleeved up</option>
              </select>
            </label>
          </div>

          <div className="flex flex-wrap items-center gap-3 pt-1">
            <button
              onClick={() => run(true)}
              disabled={!ready || busy !== null}
              className="rounded-lg px-4 py-2 text-sm font-medium disabled:opacity-40"
              style={{ border: '1px solid var(--hairline)', color: 'var(--text-primary)' }}
            >
              {busy === 'preview' ? 'Resolving…' : 'Preview'}
            </button>
            <button
              onClick={() => run(false)}
              disabled={!ready || busy !== null}
              className="rounded-lg px-4 py-2 text-sm font-medium disabled:opacity-40"
              style={{ background: 'var(--series-1)', color: '#fff' }}
            >
              {busy === 'create' ? 'Importing…' : 'Import as draft'}
            </button>
          </div>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            Preview runs the same resolution and the same gate, and writes
            nothing.
          </p>
        </section>
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}
      {busy === 'preview' && !preview && <Spinner label="Resolving names…" />}
      {preview && !preview.created && <Preview result={preview}
                                               showYaml={showYaml}
                                               onToggleYaml={() => setShowYaml((v) => !v)} />}
    </div>
  )
}

function Preview({ result, showYaml, onToggleYaml }: {
  result: ImportResult
  showYaml: boolean
  onToggleYaml: () => void
}) {
  return (
    <section className="card-surface space-y-4 rounded-xl p-5">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-semibold">{result.name}</h2>
        <Badge tone="warning">draft</Badge>
        {result.ok
          ? <Badge tone="good">valid</Badge>
          : <Badge tone="critical">{result.errors.length} error(s)</Badge>}
      </div>

      <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm"
           style={{ color: 'var(--text-secondary)' }}>
        <span className="tabular">{result.total_cards} cards in the 99</span>
        <span className="tabular">{result.land_count} lands</span>
        <span>{result.commander.join(', ') || 'no commander'}</span>
        {result.companion && <span>companion: {result.companion}</span>}
        {result.swap_board.length > 0 && (
          <span className="tabular">{result.swap_board.length} on the swap board</span>
        )}
      </div>

      {/* The facts, checked on day one. A banned card is an error the moment
          the list arrives, which is the half of ADR 13 that does not wait. */}
      {result.errors.length > 0 && (
        <ul className="space-y-1 text-sm">
          {result.errors.map((issue, i) => (
            <li key={i} style={{ color: 'var(--status-critical)' }}>
              <span className="font-medium">{issue.code}</span>
              {issue.card && <> · {issue.card}</>} — <ManaText>{issue.message}</ManaText>
            </li>
          ))}
        </ul>
      )}

      {result.unknown.length > 0 && (
        <div className="space-y-1">
          <h3 className="text-sm font-semibold">
            {result.unknown.length} name{result.unknown.length === 1 ? '' : 's'} the
            corpus does not know
          </h3>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            Kept exactly as written so the deck stays the size you pasted.
            Nothing was guessed — fix the spelling here, or in deck.yaml after
            importing.
          </p>
          <ul className="font-mono text-xs" style={{ color: 'var(--status-warning)' }}>
            {result.unknown.map((n) => <li key={n}>{n}</li>)}
          </ul>
        </div>
      )}

      {result.unreadable.length > 0 && (
        <div className="space-y-1">
          <h3 className="text-sm font-semibold">
            {result.unreadable.length} line(s) could not be read
          </h3>
          <ul className="font-mono text-xs" style={{ color: 'var(--status-warning)' }}>
            {result.unreadable.map((l) => <li key={l.line}>line {l.line}: {l.text}</li>)}
          </ul>
        </div>
      )}

      {result.notes.length > 0 && (
        <ul className="space-y-1 text-xs" style={{ color: 'var(--text-secondary)' }}>
          {result.notes.map((note, i) => <li key={i}>{note}</li>)}
        </ul>
      )}

      <div className="rounded-lg px-4 py-3 text-sm"
           style={{ background: 'var(--gridline)', color: 'var(--text-secondary)' }}>
        <strong style={{ color: 'var(--text-primary)' }}>
          {result.needs_rationale} card{result.needs_rationale === 1 ? '' : 's'} will
          need a <code>why</code>.
        </strong>{' '}
        An imported deck is a draft: its legality, colour identity and size are
        checked immediately, and the reasoning is counted rather than invented.
        Write those rationales, promote it to curated, and the five artifacts
        unlock.
      </div>

      <button onClick={onToggleYaml} className="text-xs underline"
              style={{ color: 'var(--series-1)' }}>
        {showYaml ? 'Hide' : 'Show'} the deck.yaml this writes
      </button>
      {showYaml && (
        <pre className="max-h-96 overflow-auto rounded-md p-3 font-mono text-[11px] leading-relaxed"
             style={{ background: 'var(--surface-1)', border: '1px solid var(--hairline)' }}>
          {result.yaml}
        </pre>
      )}
    </section>
  )
}
