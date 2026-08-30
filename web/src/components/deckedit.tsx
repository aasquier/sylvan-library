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
  api, type Card, type CardOffer, type ClaudeStatus, type DeckDescriptionDraft,
  type DeckRef, type EditResult, type InterviewReport, type SlotArgumentReport,
} from '../lib/api'
import { CATEGORY_LABELS, categoryLabel } from '../lib/mtg'
import { presetLabel } from '../lib/claudecopy'
import { effectivePin, fetchClaudeStatus, useStance } from '../lib/stance'
import { CardArt, CardHover, ErrorNote, ManaCost, ManaText, Select } from '../components/ui'
import { CardFinder } from './cardfinder'
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
    return <ClaudeUnavailable />
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
 * The one unavailable state there is: a server with no key.
 *
 * There used to be a second — Claude not present at all — and it was
 * unreachable, because the client is linked into the binary and the dial's
 * `installed` is a constant (`TestInstalledIsAConstantBecauseTheSDKIsLinkedIn`
 * holds it). An arm nobody can reach is copy nobody proofreads, and this one
 * had gone false: it told the reader to run an installer this server has no
 * use for. The first sentence is for anybody; the `code` line is for whoever
 * runs the server, which on a laptop is the same person.
 */
function ClaudeUnavailable({ className = '' }: { className?: string }) {
  return (
    <p className={`border-t pt-2 ${className}`}
       style={{ borderColor: 'var(--hairline)', color: 'var(--text-muted)' }}>
      Claude is here but has no key to call with.
      <span className="mt-0.5 block text-[10px]">
        Set <code>ANTHROPIC_API_KEY</code> — see <code>.env.example</code>.
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
    return <ClaudeUnavailable className="mt-2" />
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

/**
 * Add a card to the 99 or the swap board.
 *
 * The name is *found* rather than recalled — `CardFinder` holds that argument,
 * and the sentence it replaced ("Exact name — checked against the pool") is
 * quoted there as the specification of what was wrong.
 *
 * Two things follow here, and both are the same rule read in opposite
 * directions. **The category may be filled in, once, and only from a card pool
 * fact**: a land is filed under `land`, which is `CardRecord.IsLand` and the
 * importer's own inference, right about the double-faced cards a type line is
 * wrong about. **The rationale may never be filled in at all**, by this or by
 * anything else — the textarea takes the user's keystrokes and nothing else
 * (rule 4, ADR 8, ADR 11), and the finder has no path into it. The card's own
 * rules text renders beside the box for the same reason it does in the
 * rationale editor: so the thinking happens against what the card says.
 */
export function AddCardForm({ deck, stage, identity, onDone }: {
  deck: DeckRef
  stage: string
  /** The deck's colour identity, so a card outside it is marked while it is
   *  being chosen instead of refused after a rationale has been written. */
  identity: string[]
  onDone: (result: EditResult) => void
}) {
  const [open, setOpen] = useState(false)
  const [card, setCard] = useState<CardOffer | null>(null)
  const [category, setCategory] = useState('ramp')
  // Whether the person has filed this card themselves. Until they do, picking
  // a land re-files it — and picking anything else leaves their last choice
  // alone, because "ramp" was as good a guess as any and losing a deliberate
  // one on every keystroke would be worse than not guessing.
  const [filed, setFiled] = useState(false)
  const [why, setWhy] = useState('')
  const [qty, setQty] = useState(1)
  const [to, setTo] = useState('cards')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // A draft owes its rationales rather than refusing work while the thinking is
  // still to come (ADR 13), so the field is optional there and required here.
  const rationaleRequired = stage !== 'draft'

  function pick(chosen: CardOffer | null) {
    setCard(chosen)
    setError(null)
    if (chosen && !filed) setCategory(chosen.is_land ? 'land' : 'ramp')
  }

  async function submit() {
    if (!card) return
    setBusy(true)
    setError(null)
    try {
      // **Two routes, because starting a swap board is a shape change.**
      // `POST .../cards` refuses a deck file with no `swap_board:` block --
      // an edit changes what a deck says, never what shape it has (ADR 12) --
      // so a deck that has never kept a board answered this form with a 422
      // and the panel had no way to ask for one. `addToBoard` is the route
      // that may open it, and it takes a deck that already has one just the
      // same, so this does not have to know which kind it is holding.
      const body = { name: card.name, category, why: why.trim(), qty }
      const result = to === 'swap_board'
        ? await api.addToBoard(deck, body)
        : await api.addCard(deck, { ...body, to })
      onDone(result)
      setCard(null)
      setWhy('')
      setQty(1)
      setFiled(false)
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
    <div className="card-surface w-full space-y-3 rounded-lg p-4">
      {/* The finder gets the full width rather than a quarter of the grid: it
          carries a painting and a list, and a card squeezed into a 25% column
          is the thing this change exists to stop. */}
      <CardFinder value={card} onChange={pick} identity={identity} />

      <div className="grid gap-3 sm:grid-cols-3">
        <Select label="Category" value={category}
                onChange={(v) => { setCategory(v); setFiled(true) }}
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
      <div className="flex flex-wrap items-center gap-3">
        <PrimaryButton onClick={submit}
                       disabled={busy || !card || (rationaleRequired && !why.trim())}>
          {busy ? 'Adding…' : 'Add card'}
        </PrimaryButton>
        <QuietButton onClick={() => { setOpen(false); setError(null) }} disabled={busy}>
          Cancel
        </QuietButton>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          {card
            ? 'Nothing is written until you press Add — and the rationale is yours to write.'
            : 'Every card is checked against the library, and every card carries a reason.'}
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

/**
 * `/api/claude` for one deck: what this instance has, and what the dial says.
 *
 * **Three answers rather than two**, which is the whole reason this is a hook
 * and not two lines inlined. `undefined` is "not asked yet", `null` is "the
 * question itself failed", and a status is a status. The panels below render
 * nothing for the first and *say so* for the second — because a panel that
 * vanishes when the call fails is a fallback that reads as a fact, and what it
 * would be claiming is "this instance has no Claude", which nobody checked.
 *
 * Cheap and local: this endpoint reaches no network of its own, it reports
 * what the environment has and what the dial resolved. It is asked only when
 * `enabled`, so a deck somebody is merely reading pays nothing for it.
 */
function useClaudeStatus(deck: DeckRef, enabled: boolean) {
  const [status, setStatus] = useState<ClaudeStatus | null | undefined>(undefined)
  // Read rather than set: the dial lives up on the deck page and this
  // subscribes to the same store, so moving it and then asking asks with the
  // stance that was just chosen.
  const [pin, setPin] = useStance()
  useEffect(() => {
    if (!enabled) return
    let live = true
    // `owner` too: the route takes its deck as a query parameter rather than a
    // path segment, so the URL alone does not say whose it is.
    fetchClaudeStatus({ slug: deck.slug, owner: deck.owner }, pin, () => setPin(null))
      .then((s) => { if (live) setStatus(s) })
      .catch(() => { if (live) setStatus(null) })
    return () => { live = false }
  }, [deck, enabled, pin, setPin])
  return { status, pin }
}

/** Whether a status permits an ask at all: present, keyed, and not switched
 *  off. The same three-part answer the dossier button and the deck page use —
 *  a control that appears and then refuses is worse than one that is honestly
 *  absent (ADR 15). */
function claudeCanAnswer(status: ClaudeStatus | null | undefined): boolean {
  return !!status?.installed && !!status.configured
    && status.stance.axes[0]?.level !== 'off'
}

/**
 * Claude's draft of the deck's description — beside the box, never into it.
 *
 * The mode is `deck-description`, the same one the import intake runs. There it
 * writes: an imported deck is seconds old, its description is empty by
 * construction, and the intake skips the step outright for a deck that already
 * says something. **Here the field may already hold a paragraph its owner
 * wrote**, so the route this panel calls writes nothing at all and this panel
 * is where that difference is kept honest:
 *
 * * The draft renders *alongside* the box. Nothing lands in the textarea
 *   until somebody presses a button, and the button's label says what pressing
 *   it will do — "Use this draft" over an empty box, "Replace what you wrote"
 *   over a full one, and no button at all once the box already holds this
 *   draft. No control here is ambiguous about whose words lose.
 * * A replacement is one click from being undone, and the draft stays on
 *   screen after it is used, so neither version is ever the one you cannot get
 *   back.
 * * Nothing reaches the deck file until **Save description**, which is the
 *   same button and the same call a person's own typing goes through.
 *
 * This is ADR 8 and ADR 11's principle — no surface writes for you unasked —
 * applied to a field they do not name. The deck's `strategy` is not a card's
 * `why`, and this panel does not pretend it is: there is no `why_by`-style mark
 * on the far side, because ADR 41's mark is dropped the first time a person
 * edits a drafted sentence and here that edit has already had its chance before
 * anything is saved. `claude.DescriptionNever` is the server saying the same
 * thing in the payload, and it renders at the bottom of the panel.
 */
function DescriptionAssist({ deck, status, pin, current, autoAsk, onUse }: {
  deck: DeckRef
  status: ClaudeStatus | null | undefined
  pin: string | null
  /** What is in the box right now, so the control can name what it will cost. */
  current: string
  /** Opened by a control that already said "ask", so asking again here would
   *  be a second click for a decision somebody has made. */
  autoAsk?: boolean
  onUse: (draft: string) => void
}) {
  const [draft, setDraft] = useState<DeckDescriptionDraft | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const askIt = useCallback(async () => {
    setBusy(true)
    setError(null)
    try {
      // The dial's pin, or nothing — in which case the server resolves this
      // deck's own default from its status and clamps it to the deployment's
      // ceiling. What actually applied comes back in the report.
      setDraft(await api.describeDeck(deck, { stance: effectivePin(pin, status ?? null) }))
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }, [deck, pin, status])

  // Fired once, on the deck, rather than once per mount: re-rendering must not
  // buy a second call.
  const asked = useRef('')
  useEffect(() => {
    if (!autoAsk || !claudeCanAnswer(status)) return
    if (asked.current === deck.slug) return
    asked.current = deck.slug
    void askIt()
  }, [autoAsk, status, deck.slug, askIt])

  if (status === undefined) return null
  if (status === null) {
    return (
      <p className="border-t pt-2 text-xs"
         style={{ borderColor: 'var(--hairline)', color: 'var(--text-muted)' }}>
        Claude could not be reached just now. The description is still yours to
        write, and the box above works either way.
      </p>
    )
  }
  if (!status.installed || !status.configured) return <ClaudeUnavailable className="text-xs" />
  if (status.stance.axes[0]?.level === 'off') {
    // A real position, not a fault: somebody set the dial to stay silent. Say
    // where the dial is rather than showing a button that would refuse.
    return (
      <p className="border-t pt-2 text-xs"
         style={{ borderColor: 'var(--hairline)', color: 'var(--text-muted)' }}>
        Claude is set to stay silent for this deck. The dial at the top of the
        page will let it help you draft this.
      </p>
    )
  }

  const replacing = current.trim().length > 0
  // Whether the box already holds exactly this draft — true the moment it is
  // used, false again the first time somebody types.
  const inBox = !!draft?.strategy && current.trim() === draft.strategy.trim()

  return (
    <div className="space-y-2 border-t pt-2 text-xs"
         style={{ borderColor: 'var(--hairline)' }}>
      <div className="flex flex-wrap items-center gap-2">
        <button onClick={askIt} disabled={busy} className="btn btn-quiet btn-xs">
          {busy ? 'Reading your list…' : draft ? 'Ask again' : 'Ask Claude for a draft'}
        </button>
        <span style={{ color: 'var(--text-muted)' }}>
          It drafts. You keep the pen — nothing changes up there unless you say so.
        </span>
      </div>

      {error && <ErrorNote>{error}</ErrorNote>}

      {/* A live region, because the answer arrives ten to twenty seconds after
          the button and a sighted person watches it appear. `role="status"` is
          an implicit polite announcement, which is what `Spinner` and
          `ErrorNote` carry for the same reason — commandment 2's "shut out"
          includes shut out by a screen reader. The error is deliberately
          outside it: `ErrorNote` is its own region and nesting two would have
          one swallow the other. */}
      <div role="status" className="space-y-2">
      {draft && !draft.asked && (
        <p style={{ color: 'var(--text-muted)' }}>{draft.reason}</p>
      )}

      {draft?.asked && !draft.strategy && (
        <p style={{ color: 'var(--text-muted)' }}>
          {draft.reason || 'Nothing usable came back. Ask again, or write your own.'}
        </p>
      )}

      {draft?.asked && draft.strategy && (
        <div className="draft-panel space-y-2 rounded-lg p-3">
          {/* ADR 14 boundary 3: the gate's output is reproducible and this is
              not, so they never share a surface without a label. It names the
              system, never what computes it (commandment 10). */}
          <p className="text-[11px] uppercase tracking-wide"
             style={{ color: 'var(--text-muted)' }}>
            A draft by Claude, not the gate
            {draft.stance.preset ? ` · ${presetLabel(draft.stance.preset)}` : null}
          </p>
          <p className="whitespace-pre-wrap text-sm leading-relaxed"
             style={{ color: 'var(--text-primary)' }}>
            <ManaText>{draft.strategy}</ManaText>
          </p>
          {draft.fact && (
            <p style={{ color: 'var(--text-muted)' }}>
              Read off your list: {draft.fact}
            </p>
          )}
          {draft.themes.length > 0 && (
            <p style={{ color: 'var(--text-muted)' }}>
              It also read these as the deck’s themes:{' '}
              <span style={{ color: 'var(--text-secondary)' }}>
                {draft.themes.join(', ')}
              </span>
              . Shown here only — this box writes the description.
            </p>
          )}
          <div className="flex flex-wrap items-center gap-2">
            {/* Three states, not two. The label has to name the cost before it
                is paid — over an empty box this takes a draft, over a full one
                it takes a paragraph somebody wrote — and once the draft *is*
                the box, "replace what you wrote" is a sentence about nothing.
                Said rather than disabled: a greyed control reads as "this is
                broken" where the truth is "this is already done". */}
            {inBox ? (
              <span style={{ color: 'var(--text-secondary)' }}>
                This draft is in the box above, and yours to edit.
              </span>
            ) : (
              <button onClick={() => onUse(draft.strategy)}
                      className="btn btn-primary btn-accent-2 btn-xs">
                {replacing ? 'Replace what you wrote' : 'Use this draft'}
              </button>
            )}
            <span style={{ color: 'var(--text-muted)' }}>{draft.never}</span>
          </div>
        </div>
      )}
      </div>
    </div>
  )
}

/**
 * The deck's own description — the paragraph the shelf, this page and the
 * generated primer all show.
 *
 * **The empty state is the point of this component, not an afterthought.**
 * Until 2026-08-29 nothing in the app could write `strategy` at all, and a
 * deck without one rendered *nothing* here — so the field with the widest
 * reach in the whole library was the one field with no way in. An imported
 * deck is exactly the deck that has no description, which made the silence
 * worst precisely where somebody had just arrived.
 *
 * So a writable deck with no description says so and offers the pen. A deck
 * somebody else owns simply shows nothing, because an empty invitation to
 * edit a deck you cannot edit is furniture.
 *
 * **And it offers help writing one**, which is the second half of the same
 * hole: the mode that says what a deck is trying to do has existed since ADR
 * 41 and could be reached from exactly one screen — the import intake, which a
 * deck passes through once and never again. `DescriptionAssist` above is that
 * mode asked from here, and everything it does it does through this
 * component's own `text`, so the only thing that ever reaches the deck file is
 * whatever is in the box when somebody presses save.
 */
export function StrategyEditor({ deck, value, writable, onDone }: {
  deck: DeckRef
  /** The deck's current description, or "" when it has none. */
  value: string
  writable: boolean
  onDone: (result: EditResult) => void
}) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(value)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  // What a draft displaced, so a replacement is never the one thing on this
  // screen you cannot get back. Null means nothing has been displaced.
  const [displaced, setDisplaced] = useState<string | null>(null)
  // Set by the empty state's "Ask Claude for a draft", which opens the editor
  // *and* asks: the decision was made by the click that opened it, and making
  // somebody press a second button for it is a hole to stare into.
  const [askOnOpen, setAskOnOpen] = useState(false)
  // Asked only where the answer is needed: a deck this person can write, and
  // either open in the editor or missing a description entirely — those are
  // the two places a control depends on it. A deck being read costs nothing,
  // and one with a description pays only when somebody picks up the pen.
  const { status, pin } = useClaudeStatus(deck, writable && (editing || !value))

  function open(from: string, ask: boolean) {
    setText(from)
    setDisplaced(null)
    setAskOnOpen(ask)
    setEditing(true)
  }

  /** Take a draft into the box, remembering what it displaced. */
  function useDraft(drafted: string) {
    setDisplaced(text.trim() ? text : null)
    setText(drafted)
  }

  async function save() {
    if (!text.trim()) return
    setBusy(true)
    setError(null)
    try {
      onDone(await api.setDeckField(deck, 'strategy', text.trim()))
      setEditing(false)
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy(false)
    }
  }

  if (editing) {
    return (
      <div className="max-w-3xl space-y-2">
        <label className="flex flex-col gap-1">
          <span className="text-[11px] font-medium uppercase tracking-wide"
                style={{ color: 'var(--text-muted)' }}>
            What this deck is trying to do
          </span>
          <textarea value={text} onChange={(e) => setText(e.target.value)} rows={5}
                    aria-label="What this deck is trying to do"
                    placeholder="Golgari Food aristocrats. Gyome turns every nontoken creature into a meal…"
                    className="w-full rounded-md px-2 py-1.5 text-sm leading-relaxed outline-none focus:ring-2"
                    style={inputStyle} />
        </label>
        <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
          A few sentences, for somebody who has never seen the list. It shows
          on your shelf, at the top of this page, and in the printed primer.
        </p>
        {displaced !== null && (
          // The undo for a replacement, and it is only ever offered when there
          // is something to put back. Nothing has been saved at this point —
          // the swap happened in this box and nowhere else — but "you can get
          // it back" is worth more said than inferred.
          <div className="flex flex-wrap items-center gap-2 text-xs">
            <button onClick={() => { setText(displaced); setDisplaced(null) }}
                    className="btn btn-quiet btn-xs">
              Put your own words back
            </button>
            <span style={{ color: 'var(--text-muted)' }}>
              Your paragraph is safe until you save this one.
            </span>
          </div>
        )}
        {error && <ErrorNote>{error}</ErrorNote>}
        <div className="flex items-center gap-3">
          <PrimaryButton onClick={save} disabled={busy || !text.trim()}>
            {busy ? 'Saving…' : 'Save description'}
          </PrimaryButton>
          <QuietButton onClick={() => setEditing(false)} disabled={busy}>Cancel</QuietButton>
        </div>
        {writable && (
          <DescriptionAssist deck={deck} status={status} pin={pin} current={text}
                             autoAsk={askOnOpen} onUse={useDraft} />
        )}
      </div>
    )
  }

  if (!value) {
    // Nothing to read, and nothing anybody else can do about it.
    if (!writable) return null
    return (
      <div className="max-w-3xl">
        <p className="text-sm leading-relaxed" style={{ color: 'var(--text-muted)' }}>
          This deck does not say what it is trying to do yet.
        </p>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <button onClick={() => open('', false)} className="btn btn-ghost btn-xs">
            Describe this deck
          </button>
          {/* Offered only where it would work. The blank deck is where a
              newcomer is most likely to want the help and least likely to go
              looking for it, so the way in is next to the pen rather than
              behind it. */}
          {claudeCanAnswer(status) && (
            <button onClick={() => open('', true)} className="btn btn-quiet btn-xs">
              Ask Claude for a draft
            </button>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-3xl">
      <p className="whitespace-pre-wrap text-sm leading-relaxed"
         style={{ color: 'var(--text-secondary)' }}>
        <ManaText>{value}</ManaText>
      </p>
      {writable && (
        <button onClick={() => open(value, false)}
                className="btn btn-ghost btn-xs mt-2">
          Edit description
        </button>
      )}
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
