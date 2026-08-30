/**
 * The bulk edit: paste the 99 back in, reasons and all, and see exactly what
 * that would do before it happens.
 *
 * **Two screens, and the second one is the point.** A dialog that says *this
 * will overwrite things* is a warning. A dialog that says *these four cards go
 * to the graveyard, these six reasons are replaced, and here are the sentences
 * leaving and arriving* is a decision. So the paste is previewed against the
 * server — the same request with `dry_run`, the same code, the same plan the
 * confirm will apply, which is the shape the import page argued — and the
 * confirmation is that plan rather than a sentence about it.
 *
 * **Nothing is deleted, and the copy says so where it matters.** A card the
 * list leaves out goes to the deck's graveyard with the reason it was played
 * for, and one control on the deck page brings it back (ADR 27). That is the
 * thing a newcomer needs to read before they press anything, so it is written
 * at the top, again next to the list of burials, and again on the button.
 *
 * **The unresolved lines sit directly above the burials, on purpose.** A name
 * the library could not read is also a name the list did not match, so
 * somewhere in that list of cards going to the graveyard is the card the
 * misspelt line was meant to be. Putting the two apart would be technically
 * complete and practically silent.
 *
 * The server owns every refusal: it re-reads the deck, re-resolves the names
 * and re-works the plan on the confirm, and will not apply one whose deck has
 * moved since. This component's job is composing the request and rendering the
 * answer, never vetting it.
 */
import { useId, useState } from 'react'
import {
  api, errorMessage, isBulkPreview,
  type BulkPlan, type BulkPreview, type BulkReading, type DeckRef, type EditResult,
} from '../lib/api'
import { ErrorNote } from './ui'

/** What to say when the answer is not the shape it should be. Unreachable
 *  through this app; here because "nothing happened and nothing was said" is
 *  the failure mode this repo keeps writing, and a fallback that renders as
 *  silence is worse than one that admits it does not know. */
const WRONG_ANSWER = 'Something came back that this page could not read, so '
  + 'nothing was changed. Try again in a moment.'

/** The example that teaches the format by showing it, as the import page does:
 *  a quantity, whatever printing column the export wrote, and the reason in
 *  quotes at the end. */
const PLACEHOLDER = [
  '1 Sol Ring (LTC) 284 "Two mana on turn one, and it always has been."',
  '1 Cultivate "Fixes and ramps in one card."',
  '36 Forest',
].join('\n')

export function BulkEditPanel({ deck, onDone }: {
  deck: DeckRef
  /** Called once the server has written the deck, with its own answer. */
  onDone: (result: EditResult) => void | Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [preview, setPreview] = useState<BulkPreview | null>(null)
  const [busy, setBusy] = useState<'looking' | 'applying' | null>(null)
  const [error, setError] = useState<string | null>(null)
  const boxId = useId()

  function reset() {
    setOpen(false)
    setText('')
    setPreview(null)
    setError(null)
  }

  async function look() {
    setBusy('looking')
    setError(null)
    // Cleared rather than left standing: a plan on screen that describes the
    // previous paste is the one thing worse than no plan at all.
    setPreview(null)
    try {
      const result = await api.bulkEdit(deck, { text, dry_run: true })
      // The `else` is unreachable through this app and says so anyway. A
      // branch that quietly does nothing renders as a button that does
      // nothing, which is the shape this repo has shipped most often.
      if (isBulkPreview(result)) setPreview(result)
      else setError(WRONG_ANSWER)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(null)
    }
  }

  async function apply() {
    if (!preview) return
    setBusy('applying')
    setError(null)
    try {
      const result = await api.bulkEdit(deck, { text, basis: preview.plan.basis })
      if (isBulkPreview(result)) {
        setError(WRONG_ANSWER)
        return
      }
      await onDone(result)
      reset()
    } catch (e) {
      // The deck moved under the plan, or the pool answered differently. Drop
      // the plan with the error: the sentence tells them to look again, and a
      // stale plan still on screen would let them try the same thing twice.
      setError(errorMessage(e))
      setPreview(null)
    } finally {
      setBusy(null)
    }
  }

  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)}
              className="btn btn-quiet btn-sm">
        ⇄ Rewrite the 99 from a list
      </button>
    )
  }

  const lines = text.split('\n').filter((l) => l.trim()).length

  return (
    <div className="card-surface w-full space-y-4 rounded-lg p-4">
      <div>
        <h3 className="text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>
          Rewrite the 99 from a list
        </h3>
        <p className="mt-1 max-w-2xl text-xs leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          Paste your whole deck — quantity, card name, and the reason for each
          card in quotes at the end of its line. Nothing happens until you have
          seen exactly what it would change and said yes.{' '}
          <strong style={{ color: 'var(--text-primary)' }}>
            No card is ever deleted.
          </strong>{' '}
          A card your list leaves out goes to this deck&rsquo;s graveyard,
          keeping its reason, and one click on the deck page brings it back.
        </p>
      </div>

      <label className="flex flex-col gap-1" htmlFor={boxId}>
        <span className="text-[11px] font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          The 99
        </span>
        <textarea
          id={boxId}
          value={text}
          onChange={(e) => { setText(e.target.value); setPreview(null) }}
          rows={12}
          spellCheck={false}
          placeholder={PLACEHOLDER}
          className="bulk-box rounded-md p-3 font-mono text-xs leading-relaxed outline-none focus:ring-2"
        />
      </label>

      {/* Both facts on one left-aligned line rather than pushed to opposite
          ends. `justify-between` put the right-hand sentence under the
          forest's floating sprout at the window widths this Mac can actually
          show, and a rule about your own words is not a thing to read through
          a decoration. */}
      <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
        {lines} non-empty line{lines === 1 ? '' : 's'}
        {' · '}
        A line with no quoted reason keeps the reason that card already has.
      </p>

      {error && <ErrorNote>{error}</ErrorNote>}

      <div className="flex flex-wrap items-center gap-3">
        <button type="button" onClick={() => void look()}
                disabled={!text.trim() || busy !== null}
                className="btn btn-primary btn-accent-1 btn-sm">
          {busy === 'looking' ? 'Reading your list…' : 'Show me what would change'}
        </button>
        <button type="button" onClick={reset} disabled={busy !== null}
                className="btn btn-quiet btn-sm">
          Close
        </button>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          This reads your list and writes nothing.
        </span>
      </div>

      {preview && (
        <BulkPlanView preview={preview} busy={busy === 'applying'}
                      onApply={() => void apply()} />
      )}
    </div>
  )
}

/** The plan, and the confirmation. */
function BulkPlanView({ preview, busy, onApply }: {
  preview: BulkPreview
  busy: boolean
  onApply: () => void
}) {
  const plan = preview.plan
  const unresolved = preview.unknown.length + preview.unreadable.length

  return (
    <div className="bulk-plan space-y-4 rounded-lg p-4">
      <div>
        <h4 className="text-sm font-semibold" style={{ color: 'var(--text-primary)' }}>
          What this would do
        </h4>
        <p className="mt-1 text-xs" style={{ color: 'var(--text-secondary)' }}>
          Nothing below has happened yet.
        </p>
      </div>

      <Tallies plan={plan} />

      {plan.blocked.length > 0 && (
        <Section tone="critical" title={
          plan.blocked.length === 1
            ? 'One card needs a reason before any of this can happen'
            : `${plan.blocked.length} cards need a reason before any of this can happen`
        }>
          <p className="mb-2 text-xs" style={{ color: 'var(--text-secondary)' }}>
            This deck is curated, so every card in it carries a reason for its
            slot. Add one in quotes at the end of each line below and look
            again — the rest of your list is fine.
          </p>
          <ul className="space-y-1">
            {plan.blocked.map((b) => (
              <li key={b.name} className="text-xs">
                <strong style={{ color: 'var(--text-primary)' }}>{b.name}</strong>{' '}
                <span style={{ color: 'var(--text-secondary)' }}>{b.reason}.</span>
              </li>
            ))}
          </ul>
        </Section>
      )}

      <Reading reading={preview} />

      {plan.add.length > 0 && (
        <Section tone="good" title={count(plan.add.length, 'card joins', 'cards join') + ' the deck'}>
          <ul className="space-y-1.5">
            {plan.add.map((c) => (
              <li key={c.name} className="text-xs">
                <strong style={{ color: 'var(--text-primary)' }}>{c.name}</strong>
                {c.qty > 1 && (
                  <span style={{ color: 'var(--text-muted)' }}> &times;{c.qty}</span>
                )}{' '}
                <span style={{ color: 'var(--text-muted)' }}>filed as {c.category}</span>
                {c.why
                  ? <div className="mt-0.5" style={{ color: 'var(--text-secondary)' }}>
                      &ldquo;{c.why}&rdquo;
                    </div>
                  : <div className="mt-0.5" style={{ color: 'var(--text-muted)' }}>
                      No reason yet — this deck is a draft, so it is owed rather
                      than required.
                    </div>}
              </li>
            ))}
          </ul>
        </Section>
      )}

      {plan.rewrite.length > 0 && (
        <Section tone="series-4"
                 title={count(plan.rewrite.length, 'reason is', 'reasons are') + ' replaced'}>
          <ul className="space-y-2">
            {plan.rewrite.map((r) => (
              <li key={r.name} className="text-xs">
                <strong style={{ color: 'var(--text-primary)' }}>{r.name}</strong>
                {r.was_drafted && (
                  <span className="ml-1.5 text-[10px] uppercase tracking-wide"
                        style={{ color: 'var(--text-muted)' }}>
                    Claude drafted the old one
                  </span>
                )}
                <div className="mt-0.5 bulk-was">&ldquo;{r.was || '(nothing yet)'}&rdquo;</div>
                <div className="mt-0.5" style={{ color: 'var(--text-primary)' }}>
                  &ldquo;{r.why}&rdquo;
                </div>
              </li>
            ))}
          </ul>
        </Section>
      )}

      {plan.requantify.length > 0 && (
        <Section tone="series-1"
                 title={count(plan.requantify.length, 'quantity changes', 'quantities change')}>
          <ul className="space-y-1">
            {plan.requantify.map((q) => (
              <li key={q.name} className="text-xs">
                <strong style={{ color: 'var(--text-primary)' }}>{q.name}</strong>{' '}
                <span style={{ color: 'var(--text-secondary)' }}>
                  {q.was} &rarr; {q.qty}
                </span>
              </li>
            ))}
          </ul>
        </Section>
      )}

      {plan.entomb.length > 0 && (
        <Section tone="serious"
                 title={count(plan.entomb.length, 'card goes', 'cards go') + ' to the graveyard'}>
          <p className="mb-2 text-xs" style={{ color: 'var(--text-secondary)' }}>
            Your list does not name {plan.entomb.length === 1 ? 'this card' : 'these cards'}.
            {' '}They keep the reason they were played for and wait in this
            deck&rsquo;s graveyard; bringing one back is one click on this page.
            {unresolved > 0 && (
              <>
                {' '}
                <strong style={{ color: 'var(--text-primary)' }}>
                  {count(unresolved,
                    'line above was not read as a card',
                    'lines above were not read as cards')}.
                </strong>{' '}
                If one of them was meant to be a card in this list, fix the
                spelling and look again rather than burying it.
              </>
            )}
          </p>
          <ul className="flex flex-wrap gap-1.5">
            {plan.entomb.map((name) => (
              <li key={name} className="bulk-tomb-chip">{name}</li>
            ))}
          </ul>
        </Section>
      )}

      {plan.left.length > 0 && (
        <Section tone="muted" title={count(plan.left.length, 'line is', 'lines are') + ' left as it is'}>
          <ul className="space-y-1">
            {plan.left.map((l) => (
              <li key={l.name} className="text-xs" style={{ color: 'var(--text-secondary)' }}>
                <strong style={{ color: 'var(--text-primary)' }}>{l.name}</strong> {l.reason}.
              </li>
            ))}
          </ul>
        </Section>
      )}

      {plan.unchanged.length > 0 && (
        <details className="bulk-details">
          <summary className="bulk-summary">
            {/* Three words agree with the count, not two. The noun and the
                trailing clause were already conditional and the verb was not,
                so a single unchanged card read "1 card stay exactly as it is"
                — on the one panel whose whole job is to be read carefully
                before 78 cards are buried. */}
            {plan.unchanged.length} card{plan.unchanged.length === 1 ? '' : 's'}{' '}
            {plan.unchanged.length === 1 ? 'stays' : 'stay'} exactly as{' '}
            {plan.unchanged.length === 1 ? 'it is' : 'they are'}
          </summary>
          <ul className="mt-2 flex flex-wrap gap-1.5">
            {plan.unchanged.map((name) => (
              <li key={name} className="bulk-quiet-chip">{name}</li>
            ))}
          </ul>
        </details>
      )}

      <div className="flex flex-wrap items-center gap-3 pt-1">
        <button
          type="button"
          onClick={onApply}
          disabled={busy || !preview.ready}
          className="btn btn-primary btn-accent-2 btn-sm"
        >
          {busy ? 'Rewriting the deck…' : applyLabel(plan)}
        </button>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          {preview.ready
            ? 'The History tab records this as one entry, and the graveyard keeps what leaves.'
            : plan.blocked.length > 0
              ? 'Fix the lines above and look again.'
              : 'Your list already matches this deck, so there is nothing to change.'}
        </span>
      </div>
    </div>
  )
}

/** The headline: four numbers, so the shape of the edit is legible before a
 *  single list is read. Only the ones that are not zero — a row of noughts
 *  reads as a form rather than as an answer. */
function Tallies({ plan }: { plan: BulkPlan }) {
  const tiles: { n: number; label: string; tone: string }[] = [
    { n: plan.add.length, label: 'joining', tone: 'var(--status-good)' },
    {
      n: plan.rewrite.length,
      label: plan.rewrite.length === 1 ? 'reason rewritten' : 'reasons rewritten',
      tone: 'var(--series-4)',
    },
    {
      n: plan.requantify.length,
      label: plan.requantify.length === 1 ? 'quantity' : 'quantities',
      tone: 'var(--series-1)',
    },
    { n: plan.entomb.length, label: 'to the graveyard', tone: 'var(--status-serious)' },
  ].filter((t) => t.n > 0)
  if (tiles.length === 0) {
    return (
      <p className="text-xs" style={{ color: 'var(--text-secondary)' }}>
        Nothing at all — your list says the same thing the deck already says.
      </p>
    )
  }
  return (
    <ul className="flex flex-wrap gap-2">
      {tiles.map((t) => (
        <li key={t.label} className="bulk-tally" style={{ '--tally': t.tone } as React.CSSProperties}>
          <span className="bulk-tally-n">{t.n}</span>
          <span className="bulk-tally-label">{t.label}</span>
        </li>
      ))}
    </ul>
  )
}

/** What the list was read as: corrections made, names that could not be read,
 *  lines that are not cards, and lines under a heading outside the 99.
 *
 *  Above the plan's own sections rather than below them, because every one of
 *  these changes how the plan should be read. */
function Reading({ reading }: { reading: BulkReading }) {
  const { read, unknown, did_you_mean, unreadable, outside, notes } = reading
  if (!read.length && !unknown.length && !unreadable.length
      && !outside.length && !notes.length) {
    return null
  }
  const shortlist = new Map(did_you_mean.map((d) => [d.written, d.candidates]))
  return (
    <div className="space-y-3">
      {read.length > 0 && (
        <Section tone="series-1" title={count(read.length, 'spelling was', 'spellings were') + ' read for you'}>
          <ul className="space-y-1">
            {read.map((c) => (
              <li key={c.written} className="text-xs" style={{ color: 'var(--text-secondary)' }}>
                <span className="bulk-was">{c.written}</span> &rarr;{' '}
                <strong style={{ color: 'var(--text-primary)' }}>{c.read}</strong>
              </li>
            ))}
          </ul>
        </Section>
      )}

      {unknown.length > 0 && (
        <Section tone="warning"
                 title={count(unknown.length, 'name is', 'names are') + ' not a card the library knows'}>
          <p className="mb-2 text-xs" style={{ color: 'var(--text-secondary)' }}>
            Nothing was guessed for {unknown.length === 1 ? 'it' : 'them'}. Fix
            the spelling in your list and look again.
          </p>
          <ul className="space-y-1.5">
            {unknown.map((name) => (
              <li key={name} className="text-xs">
                <strong style={{ color: 'var(--text-primary)' }}>{name}</strong>
                {(shortlist.get(name) ?? []).length > 0 && (
                  <span style={{ color: 'var(--text-secondary)' }}>
                    {' '}— did you mean{' '}
                    {(shortlist.get(name) ?? []).map((c) => c.name).join(', ')}?
                  </span>
                )}
              </li>
            ))}
          </ul>
        </Section>
      )}

      {unreadable.length > 0 && (
        <Section tone="warning"
                 title={count(unreadable.length, 'line could not be read', 'lines could not be read')}>
          <ul className="space-y-1">
            {unreadable.map((l) => (
              <li key={l.line} className="text-xs" style={{ color: 'var(--text-secondary)' }}>
                <span style={{ color: 'var(--text-muted)' }}>line {l.line}</span>{' '}
                <span className="bulk-was">{l.text}</span>
              </li>
            ))}
          </ul>
        </Section>
      )}

      {outside.length > 0 && (
        <Section tone="muted"
                 title={count(outside.length, 'line sits', 'lines sit') + ' outside the 99'}>
          <p className="mb-2 text-xs" style={{ color: 'var(--text-secondary)' }}>
            {outside.length === 1 ? 'It was' : 'They were'} under a heading for
            the command zone or a sideboard, so this edit left{' '}
            {outside.length === 1 ? 'it' : 'them'} alone — it works on the 99.
          </p>
          <ul className="flex flex-wrap gap-1.5">
            {outside.map((l) => (
              <li key={l.line} className="bulk-quiet-chip">{l.text}</li>
            ))}
          </ul>
        </Section>
      )}

      {notes.length > 0 && (
        <Section tone="muted" title="Worth knowing">
          <ul className="space-y-1">
            {notes.map((n) => (
              <li key={n} className="text-xs" style={{ color: 'var(--text-secondary)' }}>{n}</li>
            ))}
          </ul>
        </Section>
      )}
    </div>
  )
}

const TONES: Record<string, string> = {
  good: 'var(--status-good)',
  warning: 'var(--status-warning)',
  serious: 'var(--status-serious)',
  critical: 'var(--status-critical)',
  'series-1': 'var(--series-1)',
  'series-4': 'var(--series-4)',
  muted: 'var(--text-muted)',
}

/** One part of the plan, under a rule in its own colour. */
function Section({ tone, title, children }: {
  tone: keyof typeof TONES | string
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="bulk-section"
             style={{ '--rule': TONES[tone] ?? 'var(--hairline)' } as React.CSSProperties}>
      <h5 className="bulk-section-title">{title}</h5>
      {children}
    </section>
  )
}

/** `1 card goes` / `4 cards go`, with both halves spelled out rather than an
 *  `s` appended — English does not conjugate that way and the panel would say
 *  "1 cards go" or "4 reason is" forever. */
function count(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`
}

/** The button names the consequence rather than saying "confirm". */
function applyLabel(plan: BulkPlan): string {
  const parts: string[] = []
  if (plan.add.length) parts.push(`add ${plan.add.length}`)
  if (plan.rewrite.length) parts.push(`rewrite ${plan.rewrite.length}`)
  if (plan.requantify.length) parts.push(`renumber ${plan.requantify.length}`)
  if (plan.entomb.length) parts.push(`bury ${plan.entomb.length}`)
  return parts.length ? `Yes — ${parts.join(', ')}` : 'Nothing to change'
}
