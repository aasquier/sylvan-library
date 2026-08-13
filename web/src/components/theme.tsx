/**
 * The theme interview — the third door into the create flow (ADR 20).
 *
 * The other two doors both open onto the same question: which of the 32 colour
 * combinations do you want? Somebody who has never played cannot answer that,
 * so this one asks about *them* instead — a film, a period, their sign, how
 * they are at game night — and translates. That works because the colour pie
 * is a personality taxonomy before it is a set of mechanics.
 *
 * Three things this component renders that are not decoration:
 *
 * **Every reading shows the words it rests on.** A slot chip carries the
 * user's own quote, because the server threw away any reading it could not
 * find in the transcript and the point of that check is lost if the result is
 * invisible. Somebody should be able to see the interview being held to what
 * they actually said.
 *
 * **A reading and a fact are styled differently and labelled differently.**
 * "Dune sounds like Golgari to me" is an interpretation and cannot be wrong;
 * "Golgari is Ravnica's guild of death and rebirth" is a claim and can be.
 * Merging them into one confident paragraph is the failure ADR 19 named, and
 * the separation survives all the way to here or it did not happen.
 *
 * **The proposal is a proposal.** Picking a commander fills in the create form
 * that already exists — it does not make a deck. Nothing under
 * `src/mtglab/claude/` can reach a write path, and this is the UI telling the
 * same truth: the deck is made by the person whose deck it is.
 */

import { useCallback, useEffect, useRef, useState } from 'react'
import {
  api,
  type ClaudeStatus,
  type ThemeCombination,
  type ThemeCommander,
  type ThemeProposal,
  type ThemeReport,
  type ThemeSlot,
  type ThemeTurn,
} from '../lib/api'
import { COLOR_VAR } from '../lib/mtg'
import { CardHover, ColorRing } from './ui'

/** Held here so a closed tab does not cost ten minutes of somebody's thinking.
 *  The server stores nothing (ADR 20), which is also why the most personal
 *  thing this app handles never reaches its disk. */
const SAVED = 'mtglab-theme-conversation'

const SLOT_LABELS: Record<string, string> = {
  taste: 'What you love',
  temperament: 'How you are',
  posture: 'At the table',
  anchor: 'Already a favourite',
}

interface Saved {
  transcript: ThemeTurn[]
  slots: ThemeSlot[]
}

function load(): Saved {
  try {
    const raw = localStorage.getItem(SAVED)
    if (!raw) return { transcript: [], slots: [] }
    const parsed = JSON.parse(raw) as Partial<Saved>
    return {
      transcript: Array.isArray(parsed.transcript) ? parsed.transcript : [],
      slots: Array.isArray(parsed.slots) ? parsed.slots : [],
    }
  } catch {
    // A corrupted stash is not worth an error message. Start again.
    return { transcript: [], slots: [] }
  }
}

/* ------------------------------------------------------------- the pieces */

function Chip({ slot }: { slot: ThemeSlot }) {
  return (
    <div className="rounded-lg px-3 py-2"
         style={{ border: '1px solid var(--hairline)', background: 'var(--page)' }}>
      <p className="text-[10px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        {SLOT_LABELS[slot.kind] ?? slot.kind}
      </p>
      <p className="text-sm" style={{ color: 'var(--text-primary)' }}>{slot.value}</p>
      {/* The check, made visible. The server dropped anything it could not
          find in the transcript, and showing the quote is how somebody can
          tell that this is a reading of them rather than about them. */}
      <p className="mt-0.5 text-[11px] italic" style={{ color: 'var(--text-muted)' }}>
        because you said “{slot.quote}”
      </p>
    </div>
  )
}

function FactNote({ fact }: { fact: NonNullable<ThemeReport['fact']> }) {
  return (
    <aside className="rounded-lg px-4 py-3"
           style={{ background: 'var(--gridline)' }}>
      <p className="text-[10px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        While you are here
      </p>
      <p className="mt-1 text-sm leading-relaxed"
         style={{ color: 'var(--text-secondary)' }}>{fact.text}</p>
      <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
        {fact.url
          ? <a href={fact.url} target="_blank" rel="noreferrer noopener"
               className="underline">{fact.source}</a>
          : 'From this tool’s own colour reference data.'}
      </p>
    </aside>
  )
}

function CommanderTile({ card, onPick }: {
  card: ThemeCommander
  onPick: () => void
}) {
  return (
    <CardHover card={card} className="block">
      <button onClick={onPick}
              className="card-surface block w-full overflow-hidden rounded-xl text-left transition hover:opacity-90">
        {card.art_crop && (
          <img src={card.art_crop} alt="" loading="lazy"
               className="h-20 w-full object-cover" />
        )}
        <div className="px-3 py-2">
          <p className="text-sm font-medium">{card.name}</p>
          <p className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            {card.type_line}
          </p>
          {card.prose && (
            <p className="mt-1 text-xs leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>{card.prose}</p>
          )}
        </div>
      </button>
    </CardHover>
  )
}

function CombinationPanel({ combo, rank, sources, onPick }: {
  combo: ThemeCombination
  rank: number
  sources: ThemeProposal['sources']
  onPick: (card: ThemeCommander) => void
}) {
  const cited = sources.filter((s) => combo.source_ids.includes(s.id))
  return (
    <article
      className="card-surface rounded-xl px-5 py-5"
      style={{
        backgroundImage: combo.colors.length
          ? `linear-gradient(135deg, ${combo.colors
              .map((c, i) => `color-mix(in srgb, ${COLOR_VAR[c]} 18%, transparent) ${
                (i / Math.max(combo.colors.length - 1, 1)) * 100}%`)
              .join(', ')})`
          : 'none',
      }}
    >
      <div className="flex flex-wrap items-center gap-3">
        <ColorRing colors={combo.colors} size={26} />
        <div>
          <h3 className="text-xl font-semibold tracking-tight">{combo.name}</h3>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {rank === 0 ? 'The one I would build' : 'Worth a look for contrast'}
            {' · '}{combo.tagline}
          </p>
        </div>
      </div>

      {/* Two paragraphs, two labels, and they are never merged. One of them
          can be wrong and the other cannot. */}
      {combo.reading && (
        <div className="mt-4 border-l-2 pl-3" style={{ borderColor: 'var(--series-1)' }}>
          <p className="text-[10px] uppercase tracking-wide"
             style={{ color: 'var(--text-muted)' }}>
            Claude reading you — an interpretation, not a finding
          </p>
          <p className="mt-1 text-sm leading-relaxed"
             style={{ color: 'var(--text-primary)' }}>{combo.reading}</p>
        </div>
      )}

      {combo.grounding && (
        <div className="mt-3 border-l-2 pl-3" style={{ borderColor: 'var(--baseline)' }}>
          <p className="text-[10px] uppercase tracking-wide"
             style={{ color: 'var(--text-muted)' }}>
            What is actually true about these colours
          </p>
          <p className="mt-1 text-sm leading-relaxed"
             style={{ color: 'var(--text-secondary)' }}>{combo.grounding}</p>
          {cited.length > 0 && (
            <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
              {cited.map((s, i) => (
                <span key={s.id}>
                  {i > 0 && ' · '}
                  <a href={s.url} target="_blank" rel="noreferrer noopener"
                     className="underline">{s.title}</a>
                </span>
              ))}
            </p>
          )}
        </div>
      )}

      <p className="mt-4 text-[10px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        Legends who lead exactly these colours
      </p>
      <div className="mt-2 grid gap-3 sm:grid-cols-3">
        {combo.commanders.map((c) => (
          <CommanderTile key={c.name} card={c} onPick={() => onPick(c)} />
        ))}
      </div>
    </article>
  )
}

/* --------------------------------------------------------------- the page */

export function ThemeInterview({ onPick, onLeave }: {
  onPick: (key: string, card: ThemeCommander) => void
  onLeave: () => void
}) {
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [{ transcript, slots }, setSaved] = useState<Saved>(load)
  const [report, setReport] = useState<ThemeReport | null>(null)
  const [proposal, setProposal] = useState<ThemeProposal | null>(null)
  const [answer, setAnswer] = useState('')
  const [budget, setBudget] = useState('')
  const [busy, setBusy] = useState<'' | 'asking' | 'proposing'>('')
  const [error, setError] = useState<string | null>(null)
  const box = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    api.claudeStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

  useEffect(() => {
    localStorage.setItem(SAVED, JSON.stringify({ transcript, slots }))
  }, [transcript, slots])

  // Stable, so the opening effect below can depend on it honestly rather than
  // being told to ignore it.
  const send = useCallback(async (next: ThemeTurn[], carried: ThemeSlot[]) => {
    setBusy('asking')
    setError(null)
    try {
      const got = await api.themeAsk({ transcript: next, slots: carried })
      setReport(got)
      setSaved({
        transcript: got.question
          ? [...next, { role: 'assistant', text: got.question }]
          : next,
        slots: got.slots,
      })
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy('')
      box.current?.focus()
    }
  }, [])

  // Fetch a question whenever there isn't one pending. That covers the opening
  // turn, and it also covers the case a plain `length > 0` guard got wrong: a
  // conversation restored from a previous tab that ended on *your answer* has
  // an outstanding question nobody ever asked for, and would otherwise sit on
  // "Starting…" with no way forward but answering twice.
  //
  // The ref does the work the dependency array cannot: this must fire once per
  // pending question, and every value it reads changes as the conversation
  // runs. Re-running and returning immediately is cheap; suppressing the lint
  // and being wrong later is not.
  const awaited = useRef(-1)
  useEffect(() => {
    if (!status?.installed || !status.configured || busy) return
    if (transcript[transcript.length - 1]?.role === 'assistant') return
    if (awaited.current === transcript.length) return
    awaited.current = transcript.length
    void send(transcript, slots)
  }, [status, busy, transcript, slots, send])

  function answerIt() {
    const said = answer.trim()
    if (!said || busy) return
    setAnswer('')
    // Record the answer and stop. The effect above notices there is no
    // question pending and fetches one — which keeps "who asks for the next
    // question" in exactly one place rather than two that can both fire.
    setSaved({ transcript: [...transcript, { role: 'user', text: said }], slots })
  }

  async function proposeIt() {
    setBusy('proposing')
    setError(null)
    try {
      setProposal(await api.themePropose({
        transcript, slots,
        budget: budget ? Number(budget) : undefined,
      }))
    } catch (e) {
      setError(String((e as Error).message ?? e))
    } finally {
      setBusy('')
    }
  }

  function startOver() {
    localStorage.removeItem(SAVED)
    awaited.current = -1
    setSaved({ transcript: [], slots: [] })
    setReport(null)
    setProposal(null)
    setError(null)
  }

  if (!status) {
    return <p className="text-sm" style={{ color: 'var(--text-muted)' }}>Loading…</p>
  }
  // Three states kept apart, because collapsing them tells somebody their key
  // is missing when they have simply not installed the extra.
  if (!status.installed || !status.configured) {
    return (
      <div className="card-surface rounded-xl px-6 py-8">
        <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
          {status.installed
            ? <>This one needs an <code>ANTHROPIC_API_KEY</code> — see <code>.env.example</code>.</>
            : <>This one needs the Claude extra: <code>pip install -e &quot;.[claude]&quot;</code></>}
        </p>
        <button onClick={onLeave} className="mt-3 rounded-md px-3 py-1.5 text-sm"
                style={{ border: '1px solid var(--hairline)',
                         color: 'var(--text-secondary)' }}>
          ← Pick colours myself
        </button>
      </div>
    )
  }

  const grounded = report?.grounded ?? slots.length
  const floor = report?.floor ?? 3
  // A conversation restored from a previous tab has no report yet, and every
  // one of these has to come off the transcript instead — otherwise coming
  // back to it shows a stranger's blank screen with your own answers above it.
  const ready = report?.may_propose ?? (new Set(slots.map((s) => s.kind)).size >= floor)
  const last = transcript[transcript.length - 1]
  const question = report?.question
    || (last?.role === 'assistant' ? last.text : '')
  const spent = report?.exchanges
    ?? transcript.filter((t) => t.role === 'user').length
  const ceiling = report?.max_exchanges ?? 10

  return (
    <section className="space-y-5">
      <div className="flex flex-wrap items-center gap-3">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">
            Let’s work out what you want
          </h2>
          <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            No Magic knowledge needed — the questions are about you. Magic’s
            five colours started life as five philosophies, so this is less of
            a detour than it sounds.
          </p>
        </div>
        <div className="ml-auto flex items-center gap-2">
          {transcript.length > 0 && (
            <button onClick={startOver} className="rounded-md px-3 py-1.5 text-sm"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-muted)' }}>
              Start over
            </button>
          )}
          <button onClick={onLeave} className="rounded-md px-3 py-1.5 text-sm"
                  style={{ border: '1px solid var(--hairline)',
                           color: 'var(--text-secondary)' }}>
            ← Pick colours myself
          </button>
        </div>
      </div>

      {error && (
        <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
      )}

      {!proposal && (
        <div className="grid gap-5 lg:grid-cols-[1fr_18rem]">
          <div className="space-y-4">
            {/* What has been said, so the conversation reads as one. */}
            {transcript.length > 1 && (
              <ol className="space-y-3">
                {transcript.slice(0, -1).map((t, i) => (
                  <li key={`${i}-${t.text.slice(0, 12)}`}
                      className={t.role === 'user' ? 'text-right' : ''}>
                    <span className="inline-block max-w-[85%] rounded-xl px-3 py-2 text-sm"
                          style={t.role === 'user'
                            ? { background: 'var(--series-1)', color: '#fff' }
                            : { border: '1px solid var(--hairline)',
                                color: 'var(--text-secondary)' }}>
                      {t.text}
                    </span>
                  </li>
                ))}
              </ol>
            )}

            {report?.fact && <FactNote fact={report.fact} />}

            <div className="card-surface rounded-xl px-5 py-4">
              <p className="text-base leading-relaxed"
                 style={{ color: 'var(--text-primary)' }}>
                {busy === 'asking'
                  ? 'Thinking…'
                  : question || report?.reason || 'Starting…'}
              </p>
              <textarea
                ref={box}
                value={answer}
                onChange={(e) => setAnswer(e.target.value)}
                onKeyDown={(e) => {
                  // Enter sends, shift-enter is a newline. A conversation is
                  // mostly one-liners and reaching for a button each time
                  // makes it feel like a form, which is the thing it is not.
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    answerIt()
                  }
                }}
                rows={2}
                placeholder="However much or little you like…"
                className="mt-3 w-full rounded-md px-3 py-2 text-sm"
                style={{ background: 'var(--page)', color: 'var(--text-primary)',
                         border: '1px solid var(--hairline)' }}
              />
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <button onClick={answerIt} disabled={!answer.trim() || !!busy}
                        className="rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
                        style={{ background: 'var(--series-1)', color: '#fff' }}>
                  Answer
                </button>
                <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
                  {spent} of {ceiling} questions
                </span>
              </div>
            </div>
          </div>

          <aside className="space-y-3">
            <p className="text-[10px] uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              What it has picked up
            </p>
            {slots.length === 0 && (
              <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
                Nothing yet — it only counts things you have actually said.
              </p>
            )}
            {slots.map((s) => <Chip key={s.kind} slot={s} />)}

            {/* A number that climbs is a model inventing preferences, which is
                exactly the failure the grounding check exists for. Rendered
                rather than logged, for the same reason the interview shows
                how many of its answers were not questions. */}
            {!!report?.slots_dropped && (
              <p className="text-xs" style={{ color: 'var(--status-warning)' }}>
                {report.slots_dropped} reading
                {report.slots_dropped === 1 ? '' : 's'} did not match anything
                you said, and {report.slots_dropped === 1 ? 'was' : 'were'} dropped.
              </p>
            )}

            <div className="border-t pt-3" style={{ borderColor: 'var(--hairline)' }}>
              <label className="block">
                <span className="text-[10px] uppercase tracking-wide"
                      style={{ color: 'var(--text-muted)' }}>
                  Budget for the deck (optional)
                </span>
                <input value={budget} inputMode="decimal"
                       onChange={(e) => setBudget(e.target.value.replace(/[^\d.]/g, ''))}
                       placeholder="150"
                       className="mt-1 w-full rounded-md px-3 py-1.5 text-sm"
                       style={{ background: 'var(--page)', color: 'var(--text-primary)',
                                border: '1px solid var(--hairline)' }} />
              </label>
              <button onClick={proposeIt} disabled={!ready || !!busy}
                      className="mt-3 w-full rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
                      style={{ background: ready ? 'var(--series-2)' : 'transparent',
                               color: ready ? '#fff' : 'var(--text-muted)',
                               border: ready ? 'none' : '1px solid var(--hairline)' }}>
                {busy === 'proposing' ? 'Reading around…' : 'Suggest my colours'}
              </button>
              <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
                {/* Measured at three to four minutes: it reads a dozen-odd
                    pages and checks every legend against the corpus. Saying so
                    is cheaper than a spinner somebody assumes has hung. */}
                {busy === 'proposing'
                  ? 'This one takes a few minutes — it reads around and checks every card.'
                  : ready
                    ? 'Ready whenever you are — or keep talking.'
                    : `${grounded} of ${floor} things known so far.`}
              </p>
            </div>
          </aside>
        </div>
      )}

      {proposal && (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
              Pick a commander to carry on — you will name the deck next, and
              nothing is created until you say so.
            </p>
            <button onClick={() => setProposal(null)}
                    className="ml-auto rounded-md px-3 py-1.5 text-sm"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              ← Keep talking
            </button>
          </div>

          {proposal.combinations.length === 0 && (
            <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
              {proposal.reason || 'Nothing usable came back.'}
            </p>
          )}

          {proposal.combinations.map((combo, i) => (
            <CombinationPanel key={combo.key} combo={combo} rank={i}
                              sources={proposal.sources}
                              onPick={(card) => onPick(combo.key, card)} />
          ))}

          {/* ADR 14's third boundary, at the bottom of the thing it applies
              to. The counts sit here rather than in a log because a number
              nobody can see is a number nobody checks. */}
          {proposal.combinations.length > 0 && (
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              {proposal.never} Written by{' '}
              <span className="font-mono">{proposal.model}</span> over{' '}
              {proposal.searched} page{proposal.searched === 1 ? '' : 's'}
              {proposal.commanders_dropped > 0 &&
                ` · ${proposal.commanders_dropped} named card${
                  proposal.commanders_dropped === 1 ? '' : 's'} did not resolve and ${
                  proposal.commanders_dropped === 1 ? 'was' : 'were'} dropped`}
              {proposal.combinations_dropped > 0 &&
                ` · ${proposal.combinations_dropped} further suggestion${
                  proposal.combinations_dropped === 1 ? '' : 's'} lost every
                  commander to that check`}
              {proposal.sources_dropped > 0 &&
                ` · ${proposal.sources_dropped} citation${
                  proposal.sources_dropped === 1 ? '' : 's'} were not among the pages read`}
            </p>
          )}
        </div>
      )}
    </section>
  )
}
