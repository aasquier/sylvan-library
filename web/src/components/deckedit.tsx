/**
 * The editing surfaces for a deck: rationales, cards, notes.
 *
 * One rule shapes all of them, and it is the reason this file is worth reading
 * before changing: **nothing here may author a rationale.** No generate button,
 * no suggested text, no placeholder that is really a draft, no "tidy this up".
 * The `why` field takes the user's keystrokes and nothing else, because a
 * rationale written by the tool is precisely the empty justification that rule
 * 4 exists to prevent (ADR 8, ADR 12 rule 3).
 *
 * What the editor may do is make the rationale easier to *write*: it puts the
 * card's oracle text — a corpus fact, per rule 1 — next to the box, so the
 * thinking happens against what the card actually says rather than against
 * what someone remembers it saying. When the assistant modes arrive, their
 * questions belong in that same column, for the same reason and under the same
 * rule: they may ask, they may argue, they may not type into the field.
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  api, type Card, type ClaudeStatus, type DeckRef, type EditResult,
  type InterviewReport,
} from '../lib/api'
import { CATEGORY_LABELS, categoryLabel } from '../lib/mtg'
import { ErrorNote, ManaText, Select } from '../components/ui'

const CATEGORIES = Object.keys(CATEGORY_LABELS).filter((k) => k !== 'commander')

const inputStyle: React.CSSProperties = {
  background: 'var(--surface-1)',
  color: 'var(--text-primary)',
  border: '1px solid var(--hairline)',
}

function PrimaryButton({ children, ...rest }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      {...rest}
      className="rounded-lg px-3 py-1.5 text-xs font-medium disabled:opacity-50"
      style={{ background: 'var(--series-1)', color: '#fff' }}
    >
      {children}
    </button>
  )
}

function QuietButton({ children, ...rest }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button {...rest} className="text-xs underline disabled:opacity-50"
            style={{ color: 'var(--text-muted)' }}>
      {children}
    </button>
  )
}

/**
 * The rationale interview, in the column beside the box rather than in it.
 *
 * This component may render questions and may not render anything else. It has
 * no control that puts text into the textarea — no copy button, no "use this",
 * no click-to-insert — and that absence is the feature, not an omission to fix
 * later. The server side is built the same way: the mode's response schema has
 * no field for a rationale, and anything coming back that does not end in a
 * question mark is dropped before it reaches here.
 *
 * Three states worth keeping apart, because collapsing them tells someone
 * their key is missing when they simply have not installed the extra:
 * not installed, not configured, and nothing asked yet.
 */
function InterviewPanel({ deck, card, askNow = false }: {
  deck: DeckRef
  card: string
  /** Opened from a control that already said "ask Claude", so asking again
   *  here would be a second click for a decision already made. */
  askNow?: boolean
}) {
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [report, setReport] = useState<InterviewReport | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Cheap and local: this endpoint reaches no network, it only reports what
  // the environment has and what the dial says.
  useEffect(() => {
    let live = true
    // `owner` too: this route takes its deck as a query parameter rather
    // than a path segment, so the URL alone does not say whose it is.
    api.claudeStatus({ slug: deck.slug, owner: deck.owner })
      .then((s) => { if (live) setStatus(s) })
      .catch(() => { if (live) setStatus(null) })
    return () => { live = false }
  }, [deck])

  // A new card is a new interview. Without this the questions about the last
  // card linger beside the next one's empty box, which is the most misleading
  // thing this panel could do.
  useEffect(() => { setReport(null); setError(null) }, [card])

  const askIt = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      // No stance sent: the server resolves the deck's own default from its
      // `status` and clamps it to the deployment ceiling. What actually
      // applied comes back in the report and is shown below.
      setReport(await api.interview(deck, { card }))
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }, [deck, card])

  // Opened by somebody who clicked "Ask Claude" on the card itself. Firing
  // once per card rather than once per mount: the guard is the card name, so
  // switching cards re-asks and re-rendering does not.
  const asked = useRef('')
  useEffect(() => {
    if (!askNow || !status?.installed || !status.configured) return
    if (asked.current === card) return
    asked.current = card
    void askIt()
  }, [askNow, status, card, askIt])

  if (!status) return null
  if (!status.installed) {
    return (
      <p className="border-t pt-2" style={{ borderColor: 'var(--hairline)',
                                            color: 'var(--text-muted)' }}>
        Claude is not installed. <code>pip install -e &quot;.[claude]&quot;</code>
      </p>
    )
  }
  if (!status.configured) {
    return (
      <p className="border-t pt-2" style={{ borderColor: 'var(--hairline)',
                                            color: 'var(--text-muted)' }}>
        No <code>ANTHROPIC_API_KEY</code> set — see <code>.env.example</code>.
      </p>
    )
  }

  return (
    <div className="space-y-2 border-t pt-2" style={{ borderColor: 'var(--hairline)' }}>
      <div className="flex items-center gap-2">
        <button
          onClick={askIt}
          disabled={busy}
          className="rounded-md px-2 py-1 text-[11px] font-medium disabled:opacity-50"
          style={{ border: '1px solid var(--hairline)',
                   color: 'var(--text-primary)' }}
        >
          {busy ? 'Asking…' : report ? 'Ask again' : 'Ask for questions'}
        </button>
        <span style={{ color: 'var(--text-muted)' }}>
          It asks. You answer.
        </span>
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}

      {report && !report.asked && (
        <p style={{ color: 'var(--text-muted)' }}>{report.reason}</p>
      )}

      {report?.asked && (
        <div className="space-y-2">
          {/* ADR 14 boundary 3: the gate's output is reproducible and this is
              not, so they never share a surface without a label. */}
          <p style={{ color: 'var(--text-muted)' }}>
            Claude — <span className="font-mono">{report.model}</span>, not the gate
            {report.stance.preset ? ` · ${report.stance.preset}` : null}
          </p>
          {report.questions.length === 0 && (
            <p style={{ color: 'var(--text-muted)' }}>
              {report.reason || 'Nothing usable came back.'}
            </p>
          )}
          <ol className="space-y-2">
            {report.questions.map((q) => (
              <li key={q.question}>
                <span className="mr-1 text-[10px] uppercase tracking-wide"
                      style={{ color: 'var(--text-muted)' }}>{q.angle}</span>
                <span style={{ color: 'var(--text-primary)' }}>{q.question}</span>
                {q.fact && (
                  <span className="mt-0.5 block text-[10px]"
                        style={{ color: 'var(--text-muted)' }}>{q.fact}</span>
                )}
              </li>
            ))}
          </ol>
          {report.questions_dropped > 0 && (
            <p style={{ color: 'var(--status-warning)' }}>
              {report.questions_dropped} answer
              {report.questions_dropped === 1 ? ' was' : 's were'} not a question
              and {report.questions_dropped === 1 ? 'was' : 'were'} dropped.
            </p>
          )}
          <p style={{ color: 'var(--text-muted)' }}>{report.never}</p>
        </div>
      )}
    </div>
  )
}

/**
 * Write or rewrite one card's `why`.
 *
 * The placeholder is a question, deliberately. A placeholder containing a
 * plausible rationale is a first draft the tool wrote, and the difference
 * between that and a generate button is one keystroke.
 */
export function RationaleEditor({
  deck, card, onSave, onCancel, askNow = false,
}: {
  deck: DeckRef
  card: Card
  onSave: (why: string) => Promise<void>
  onCancel: () => void
  /** True when opened from the deck page's "Ask Claude", which is the control
   *  that exists because nothing on that page said this was here. */
  askNow?: boolean
}) {
  const [why, setWhy] = useState(card.why)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const box = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    box.current?.focus()
  }, [])

  async function save() {
    if (!why.trim()) return
    setBusy(true)
    setError(null)
    try {
      await onSave(why.trim())
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="mt-2 grid gap-3 rounded-lg p-3 md:grid-cols-[1fr_minmax(0,18rem)]"
         style={{ background: 'var(--surface-1)' }}>
      <div className="space-y-2">
        <label className="block text-[11px] font-medium uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
          Why {card.name} earns its slot
        </label>
        <textarea
          ref={box}
          value={why}
          onChange={(e) => setWhy(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') onCancel()
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) void save()
          }}
          rows={4}
          placeholder="What does this card do that the deck needs? What would you cut it for?"
          className="w-full rounded-md px-2 py-1.5 text-xs outline-none focus:ring-2"
          style={inputStyle}
        />
        {error && <ErrorNote>{error}</ErrorNote>}
        <div className="flex items-center gap-3">
          <PrimaryButton onClick={save} disabled={!why.trim() || busy}>
            {busy ? 'Saving…' : 'Save rationale'}
          </PrimaryButton>
          <QuietButton onClick={onCancel} disabled={busy}>Cancel</QuietButton>
          <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            Writes deck.yaml — commit it, deck history is git history.
          </span>
        </div>
      </div>

      {/* Rule 1, made useful: the card's actual text, from the corpus, next to
          the box you are arguing in — and, under it, the questions. Both are
          in this column and neither may reach the box: the corpus states
          facts, the interview asks things, and the sentence that lands in
          deck.yaml is typed on the left. */}
      <aside className="space-y-2 rounded-md p-2 text-[11px] leading-relaxed"
             style={{ background: 'var(--surface-2, var(--gridline))',
                      color: 'var(--text-secondary)' }}>
        <div className="font-medium" style={{ color: 'var(--text-primary)' }}>
          {card.type_line ?? card.name}
        </div>
        {card.oracle_text
          ? <p className="whitespace-pre-wrap"><ManaText size={11}>{card.oracle_text}</ManaText></p>
          : <p style={{ color: 'var(--text-muted)' }}>
              No corpus text for this card.
            </p>}
        <InterviewPanel deck={deck} card={card.name} askNow={askNow} />
      </aside>
    </div>
  )
}

/** Add a card to the 99 or the swap board. */
export function AddCardForm({ deck, stage, onDone }: {
  deck: DeckRef
  stage: string
  onDone: (result: EditResult) => void
}) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [category, setCategory] = useState('ramp')
  const [why, setWhy] = useState('')
  const [qty, setQty] = useState(1)
  const [to, setTo] = useState('cards')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // A draft owes its rationales rather than refusing work while the thinking is
  // still to come (ADR 13), so the field is optional there and required here.
  const rationaleRequired = stage !== 'draft'

  async function submit() {
    setBusy(true)
    setError(null)
    try {
      const result = await api.addCard(deck, {
        name: name.trim(), category, why: why.trim(), qty, to,
      })
      onDone(result)
      setName('')
      setWhy('')
      setQty(1)
      setOpen(false)
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }

  if (!open) {
    return (
      <button onClick={() => setOpen(true)}
              className="rounded-lg px-3 py-1.5 text-xs font-medium"
              style={{ background: 'var(--gridline)', color: 'var(--text-primary)' }}>
        + Add a card
      </button>
    )
  }

  return (
    <div className="card-surface space-y-3 rounded-lg p-4">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <label className="flex flex-col gap-1">
          <span className="text-[11px] font-medium uppercase tracking-wide"
                style={{ color: 'var(--text-muted)' }}>Card name</span>
          <input value={name} onChange={(e) => setName(e.target.value)}
                 placeholder="Exact name — checked against the corpus"
                 className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
                 style={inputStyle} />
        </label>
        <Select label="Category" value={category} onChange={setCategory}
                options={CATEGORIES.map((c) => ({ value: c, label: categoryLabel(c) }))} />
        <label className="flex flex-col gap-1">
          <span className="text-[11px] font-medium uppercase tracking-wide"
                style={{ color: 'var(--text-muted)' }}>Quantity</span>
          <input type="number" min={1} value={qty}
                 onChange={(e) => setQty(Math.max(1, Number(e.target.value) || 1))}
                 className="h-9 rounded-md px-2 text-sm outline-none focus:ring-2"
                 style={inputStyle} />
        </label>
        <Select label="Into" value={to} onChange={setTo}
                options={[{ value: 'cards', label: 'The 99' },
                          { value: 'swap_board', label: 'Swap board' }]} />
      </div>

      <label className="block space-y-1">
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          Why it earns the slot{rationaleRequired ? '' : ' (optional in a draft)'}
        </span>
        <textarea value={why} onChange={(e) => setWhy(e.target.value)} rows={2}
                  placeholder="What does this card do that the deck needs?"
                  className="w-full rounded-md px-2 py-1.5 text-xs outline-none focus:ring-2"
                  style={inputStyle} />
      </label>

      {error && <ErrorNote>{error}</ErrorNote>}
      <div className="flex items-center gap-3">
        <PrimaryButton onClick={submit}
                       disabled={busy || !name.trim() || (rationaleRequired && !why.trim())}>
          {busy ? 'Adding…' : 'Add card'}
        </PrimaryButton>
        <QuietButton onClick={() => { setOpen(false); setError(null) }} disabled={busy}>
          Cancel
        </QuietButton>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          Legality and colour identity are checked against the corpus before anything is written.
        </span>
      </div>
    </div>
  )
}

/** Edit the deck-level prose the advanced primer reads directly. */
export function NoteEditor({ deck, noteKey, value, onDone, writable = true }: {
  deck: DeckRef
  noteKey: string
  value: string
  onDone: (result: EditResult) => void
  /** Whether to offer the Edit control. The note's *prose* is shown either
   *  way — this component renders the deck's thinking, not just a button, and
   *  hiding it from a reader would hide content rather than an affordance.
   *  Defaults true so every existing caller is unchanged. */
  writable?: boolean
}) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(value)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function save() {
    if (!text.trim()) return
    setBusy(true)
    setError(null)
    try {
      onDone(await api.setNote(deck, noteKey, text.trim()))
      setEditing(false)
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }

  if (!editing) {
    return (
      <>
        <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          <ManaText>{value}</ManaText>
        </p>
        {writable && (
          <button onClick={() => { setText(value); setEditing(true) }}
                  className="mt-2 text-xs underline" style={{ color: 'var(--text-muted)' }}>
            Edit
          </button>
        )}
      </>
    )
  }

  return (
    <div className="mt-2 space-y-2">
      <textarea value={text} onChange={(e) => setText(e.target.value)} rows={8}
                className="w-full rounded-md px-2 py-1.5 text-xs leading-relaxed outline-none focus:ring-2"
                style={inputStyle} />
      {error && <ErrorNote>{error}</ErrorNote>}
      <div className="flex items-center gap-3">
        <PrimaryButton onClick={save} disabled={busy || !text.trim()}>
          {busy ? 'Saving…' : 'Save note'}
        </PrimaryButton>
        <QuietButton onClick={() => setEditing(false)} disabled={busy}>Cancel</QuietButton>
      </div>
    </div>
  )
}

/** Start a note the deck does not have yet. */
export function AddNoteForm({ deck, existing, onDone }: {
  deck: DeckRef
  existing: string[]
  onDone: (result: EditResult) => void
}) {
  const [open, setOpen] = useState(false)
  const [key, setKey] = useState('')
  const [text, setText] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const clash = existing.includes(key.trim())

  async function save() {
    setBusy(true)
    setError(null)
    try {
      onDone(await api.setNote(deck, key.trim(), text.trim()))
      setKey('')
      setText('')
      setOpen(false)
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }

  if (!open) {
    return (
      <button onClick={() => setOpen(true)}
              className="rounded-lg px-3 py-1.5 text-xs font-medium"
              style={{ background: 'var(--gridline)', color: 'var(--text-primary)' }}>
        + Add a note
      </button>
    )
  }

  return (
    <section className="card-surface space-y-2 rounded-xl p-4">
      <input value={key} onChange={(e) => setKey(e.target.value)}
             placeholder="Key, e.g. mulligan"
             className="h-9 w-full rounded-md px-2 text-sm outline-none focus:ring-2"
             style={inputStyle} />
      <textarea value={text} onChange={(e) => setText(e.target.value)} rows={6}
                placeholder="The thinking that only a conversation produces."
                className="w-full rounded-md px-2 py-1.5 text-xs leading-relaxed outline-none focus:ring-2"
                style={inputStyle} />
      {clash && (
        <p className="text-xs" style={{ color: 'var(--status-warning)' }}>
          A note called {key.trim()} already exists — saving replaces it.
        </p>
      )}
      {error && <ErrorNote>{error}</ErrorNote>}
      <div className="flex items-center gap-3">
        <PrimaryButton onClick={save} disabled={busy || !key.trim() || !text.trim()}>
          {busy ? 'Saving…' : 'Save note'}
        </PrimaryButton>
        <QuietButton onClick={() => { setOpen(false); setError(null) }} disabled={busy}>
          Cancel
        </QuietButton>
      </div>
    </section>
  )
}
