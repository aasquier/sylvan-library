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
 * card's oracle text — a card pool fact, per rule 1 — next to the box, so the
 * thinking happens against what the card actually says rather than against
 * what someone remembers it saying. When the assistant modes arrive, their
 * questions belong in that same column, for the same reason and under the same
 * rule: they may ask, they may argue, they may not type into the field.
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  api, type Card, type ClaudeStatus, type DeckRef, type EditResult,
  type InterviewReport, type SlotArgumentReport,
} from '../lib/api'
import { CATEGORY_LABELS, categoryLabel } from '../lib/mtg'
import { presetLabel } from '../lib/claudecopy'
import { effectivePin, fetchClaudeStatus, useStance } from '../lib/stance'
import { CardArt, CardHover, ErrorNote, ManaCost, ManaText, Select } from '../components/ui'
import { SwapComposer } from './swap'

const CATEGORIES = Object.keys(CATEGORY_LABELS).filter((k) => k !== 'commander')

const inputStyle: React.CSSProperties = {
  background: 'var(--surface-1)',
  color: 'var(--text-primary)',
  border: '1px solid var(--hairline)',
}

function PrimaryButton({ children, ...rest }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button {...rest} className="btn btn-primary btn-accent-1 btn-sm">
      {children}
    </button>
  )
}

function QuietButton({ children, ...rest }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button {...rest} className="btn btn-ghost btn-xs">
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
  // Read rather than set: the dial itself lives on the deck page, and this
  // panel opens inside a modal over it. One store, so moving the dial and
  // then opening this asks with the stance the user just chose.
  const [pin, setPin] = useStance()

  // Cheap and local: this endpoint reaches no network, it only reports what
  // the environment has and what the dial says.
  useEffect(() => {
    let live = true
    // `owner` too: this route takes its deck as a query parameter rather
    // than a path segment, so the URL alone does not say whose it is.
    fetchClaudeStatus({ slug: deck.slug, owner: deck.owner }, pin, () => setPin(null))
      .then((s) => { if (live) setStatus(s) })
      .catch(() => { if (live) setStatus(null) })
    return () => { live = false }
  }, [deck, pin, setPin])

  // A new card is a new interview. Without this the questions about the last
  // card linger beside the next one's empty box, which is the most misleading
  // thing this panel could do -- and an effect cleared them one render *after*
  // that had already happened, so the misleading frame was shipped and then
  // withdrawn. Adjusting during render is React's own answer to "reset state
  // when a prop changes" and closes the window rather than shortening it. The
  // rest of the panel's state survives on purpose: the stance readout is
  // about the deck and the dial, neither of which changed.
  const [interviewing, setInterviewing] = useState(card)
  if (interviewing !== card) {
    setInterviewing(card)
    setReport(null)
    setError(null)
  }

  const askIt = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      // The dial's pin, or nothing at all — in which case the server resolves
      // the deck's own default from its `status`. Either way it clamps to the
      // deployment ceiling, and what actually applied comes back in the report
      // and is shown below.
      setReport(await api.interview(deck, { card, stance: effectivePin(pin, status) }))
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }, [deck, card, pin, status])

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
  if (!status.installed || !status.configured) {
    return <ClaudeUnavailable installed={status.installed} />
  }

  return (
    <div className="space-y-2 border-t pt-2" style={{ borderColor: 'var(--hairline)' }}>
      <div className="flex items-center gap-2">
        <button
          onClick={askIt}
          disabled={busy}
          className="btn btn-quiet btn-xs"
        >
          {busy ? 'Asking…' : report ? 'Ask again' : 'Ask for questions'}
        </button>
        <span style={{ color: 'var(--text-muted)' }}>
          It asks. You answer. The why stays yours.
        </span>
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}

      {report && !report.asked && (
        <p style={{ color: 'var(--text-muted)' }}>{report.reason}</p>
      )}

      {report?.asked && (
        <div className="space-y-2">
          {/* ADR 14 boundary 3: the gate's output is reproducible and this is
              not, so they never share a surface without a label. The label
              names the system, not the model id — which system answered is
              the fact a reader needs; the checkpoint string is not. */}
          <p style={{ color: 'var(--text-muted)' }}>
            Claude, not the gate
            {report.stance.preset ? ` · ${presetLabel(report.stance.preset)}` : null}
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

/** How a charge's weight reads. Three, matching `argue.STRENGTHS`. */
const STRENGTH_COLOUR: Record<string, string> = {
  // `--status-critical`/`--status-serious` are the defined tokens; an earlier
  // `--status-error` here named nothing and decisive charges rendered in the
  // inherited colour.
  decisive: 'var(--status-critical)',
  serious: 'var(--status-serious)',
  minor: 'var(--text-muted)',
}

/**
 * The two unavailable states, kept apart — collapsing them tells somebody
 * their key is missing when they simply have not installed the extra. The
 * first sentence is for anybody; the `code` line is for whoever runs the
 * server, which on a laptop is the same person.
 */
function ClaudeUnavailable({ installed, className = '' }: {
  installed: boolean
  className?: string
}) {
  return (
    <p className={`border-t pt-2 ${className}`}
       style={{ borderColor: 'var(--hairline)', color: 'var(--text-muted)' }}>
      {installed
        ? 'Claude is installed here but has no key to call with.'
        : 'Claude isn’t available on this server.'}
      <span className="mt-0.5 block text-[10px]">
        {installed
          ? <>Set <code>ANTHROPIC_API_KEY</code> — see <code>.env.example</code>.</>
          : <><code>pip install -e &quot;.[claude]&quot;</code> adds it.</>}
      </span>
    </p>
  )
}

/**
 * The slot argument (ADR 25): the case against one card, and only against it.
 *
 * This panel renders a one-sided argument and **says so**, which is the whole
 * of its presentational responsibility. The mode cannot produce the other side
 * — its response schema has no field for one — so there is nothing here to
 * suppress; what there is to do is make sure a user reading a one-sided case
 * knows that is what they are reading, because the failure this design makes
 * possible is somebody taking it for a verdict.
 *
 * Note what is absent, and that it is a *different* absence from the
 * interview's. There is no control that writes anything, same as there. There
 * is also no "and here is why it is good" section, because a balanced version
 * of this feature would hand back a finished rationale grounded in the user's
 * own deck — and a UI that merely declined to render it would not be a guard,
 * since the CLI renders the same payload.
 *
 * The alternatives are rendered as **cards, not as recommendations**. Each one
 * has already been through the pool, the ban list and the deck's colour
 * identity on the server; what is shown beside a name is the card's own oracle
 * text, because that is better evidence than a sentence about it and it is the
 * sentence nobody is allowed to write.
 */
export function SlotArgumentPanel({ deck, card, onClose, writable = false, onSwapped }: {
  deck: DeckRef
  card: string
  onClose: () => void
  /** Whether to offer "Use this card" on an alternative. The alternatives
   *  are analysis and render either way; only the swap is gated. */
  writable?: boolean
  /** Called after a swap landed — the argued card is gone, so the caller
   *  should refresh the deck and close this panel. */
  onSwapped?: () => void
}) {
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [report, setReport] = useState<SlotArgumentReport | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [pin, setPin] = useStance()

  useEffect(() => {
    let live = true
    fetchClaudeStatus({ slug: deck.slug, owner: deck.owner }, pin, () => setPin(null))
      .then((s) => { if (live) setStatus(s) })
      .catch(() => { if (live) setStatus(null) })
    return () => { live = false }
  }, [deck, pin, setPin])

  const askIt = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      setReport(await api.argue(deck, { card, stance: effectivePin(pin, status) }))
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }, [deck, card, pin, status])

  // Opened by a control that already said "argue this slot", so asking again
  // here would be a second click for a decision already made. Guarded on the
  // card name rather than on mount, exactly as the interview is.
  const asked = useRef('')
  useEffect(() => {
    if (!status?.installed || !status.configured) return
    if (asked.current === card) return
    asked.current = card
    void askIt()
  }, [status, card, askIt])

  if (!status) return null
  if (!status.installed || !status.configured) {
    return <ClaudeUnavailable installed={status.installed} className="mt-2" />
  }

  return (
    <div className="mt-2 space-y-2 border-t pt-2 text-xs"
         style={{ borderColor: 'var(--hairline)' }}>
      <div className="flex items-center gap-2">
        <strong style={{ color: 'var(--text-primary)' }}>The case against</strong>
        <button onClick={askIt} disabled={busy}
                className="btn btn-quiet btn-xs">
          {busy ? 'Arguing…' : report ? 'Argue again' : 'Argue'}
        </button>
        <button onClick={onClose} className="btn btn-ghost btn-xs ml-auto">Close</button>
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}

      {report && !report.asked && (
        <p style={{ color: 'var(--text-muted)' }}>{report.reason}</p>
      )}

      {report?.asked && (
        <SlotArgumentBody deck={deck} report={report}
                          writable={writable} onSwapped={onSwapped} />
      )}
    </div>
  )
}

/**
 * One argued slot's answer, rendered. Shared by the per-card panel above and
 * the deck review's queue — same payload, same rules, and in particular the
 * same absence: nothing here writes or pre-fills a rationale, and the only
 * way an alternative becomes the card of record is the swap composer, with a
 * why the user typed (ADR 11).
 */
export function SlotArgumentBody({ deck, report, writable = false, onSwapped }: {
  deck: DeckRef
  report: SlotArgumentReport
  writable?: boolean
  onSwapped?: () => void
}) {
  // The alternative being swapped in, if any. The composer owns the why.
  const [swapInto, setSwapInto] = useState<string | null>(null)

  const dropped = report.alternatives_dropped
  const droppedLines: [string, string[]][] = dropped ? [
    ['not in the card pool', dropped.not_in_pool],
    ['banned in Commander', dropped.banned],
    ["outside the deck's colour identity", dropped.off_colour],
    ['already in this deck', dropped.already_in_deck ?? []],
  ] : []

  return (
        <div className="space-y-2">
          {/* ADR 14 boundary 3, and this mode needs it more than the interview
              does: questions are never mistaken for a verdict, and a reasoned
              case against a card reads exactly like one. */}
          <p style={{ color: 'var(--text-muted)' }}>
            Claude, not the gate
            {report.stance.preset ? ` · ${presetLabel(report.stance.preset)}` : null}
          </p>
          {report.charges.length === 0 && (
            <p style={{ color: 'var(--text-muted)' }}>
              {report.reason || 'Nothing usable came back.'}
            </p>
          )}
          <ol className="space-y-2">
            {report.charges.map((c) => (
              <li key={c.claim}>
                <span className="mr-1 text-[10px] uppercase tracking-wide"
                      style={{ color: STRENGTH_COLOUR[c.strength] ?? 'var(--text-muted)' }}>
                  {c.strength}
                </span>
                <span className="mr-1 text-[10px] uppercase tracking-wide"
                      style={{ color: 'var(--text-muted)' }}>{c.ground}</span>
                <span style={{ color: 'var(--text-primary)' }}>{c.claim}</span>
                <span className="mt-0.5 block text-[10px]"
                      style={{ color: 'var(--text-muted)' }}>{c.fact}</span>
              </li>
            ))}
          </ol>
          {report.charges_dropped > 0 && (
            <p style={{ color: 'var(--status-warning)' }}>
              {report.charges_dropped} charge
              {report.charges_dropped === 1 ? '' : 's'} cited nothing and
              {report.charges_dropped === 1 ? ' was' : ' were'} dropped.
            </p>
          )}

          {report.alternatives.length > 0 && (
            <div className="space-y-2">
              <p style={{ color: 'var(--text-secondary)' }}>
                Could do the job instead — checked against the pool, the ban
                list and this deck&apos;s colour identity:
              </p>
              <ul className="space-y-1">
                {report.alternatives.map((a) => (
                  <li key={a.name}
                      className="flex flex-wrap items-center gap-3 rounded-lg p-2"
                      style={{ background: 'var(--surface-1)' }}>
                    {/* Same affordance as the validation shortlist: the art is
                        what you recognise, the full card is what you read
                        before taking a suggestion. */}
                    {a.art_crop && (
                      <CardHover card={{ name: a.name, image: a.image }}>
                        <CardArt src={a.art_crop} alt={a.name}
                                 ratio="aspect-[626/457]"
                                 className="w-16 shrink-0 cursor-help" />
                      </CardHover>
                    )}
                    <div className="min-w-0 flex-1 basis-52">
                      <div className="flex flex-wrap items-baseline gap-2">
                        <span className="text-sm font-medium"
                              style={{ color: 'var(--text-primary)' }}>{a.name}</span>
                        {a.mana_cost && <ManaCost cost={a.mana_cost} />}
                      </div>
                      {a.oracle_text && (
                        <p className="mt-0.5 text-[10px] leading-relaxed"
                           style={{ color: 'var(--text-muted)' }}>
                          <ManaText size={10}>{a.oracle_text}</ManaText>
                        </p>
                      )}
                    </div>
                    {/* The argument is one-sided by design (ADR 25); the swap
                        is not part of the argument. The card of record still
                        changes only through the composer below, with a why the
                        user wrote — the button names the card, nothing more. */}
                    {writable && (
                      <button
                        onClick={() => setSwapInto(swapInto === a.name ? null : a.name)}
                        className="btn btn-quiet btn-sm shrink-0">
                        Use this card
                      </button>
                    )}
                  </li>
                ))}
              </ul>
              {swapInto && (
                <SwapComposer
                  deck={deck}
                  out={report.card}
                  into={swapInto}
                  onDone={() => { setSwapInto(null); onSwapped?.() }}
                  onCancel={() => setSwapInto(null)}
                />
              )}
            </div>
          )}
          {/* Named rather than silently shortened. Which filter removed a card
              is the informative part: one of these is a fact about the model
              and two are facts about the deck. */}
          {droppedLines.filter(([, names]) => names.length > 0).map(([why, names]) => (
            <p key={why} style={{ color: 'var(--text-muted)' }}>
              Dropped, {why}: {names.join(', ')}
            </p>
          ))}
          {dropped && dropped.no_pool.length > 0 && (
            <p style={{ color: 'var(--text-muted)' }}>
              No card pool loaded, so no alternative could be checked.
            </p>
          )}
          <p style={{ color: 'var(--text-muted)' }}>{report.never}</p>
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
            Writes deck.yaml — the History tab records the edit.
          </span>
        </div>
      </div>

      {/* Rule 1, made useful: the card's actual text, from the pool, next to
          the box you are arguing in — and, under it, the questions. Both are
          in this column and neither may reach the box: the pool states
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
              No card text in the pool for this card.
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
              className="btn btn-quiet btn-sm">
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
                 placeholder="Exact name — checked against the pool"
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
          Legality and colour identity are checked against the pool before anything is written.
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
                  className="btn btn-ghost btn-xs mt-2">
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
              className="btn btn-quiet btn-sm">
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
