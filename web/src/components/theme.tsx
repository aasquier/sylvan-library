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
  ApiError,
  api,
  followJob,
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
  /** A proposal in flight, as a job id. Kept for the same reason the
   *  transcript is: the run takes minutes and costs real money, so a reload
   *  should reattach to the one already going rather than start a second. */
  job: string | null
  /** And the answer once it lands, which is the same argument one step on —
   *  four minutes of waiting should not be undone by a refresh. */
  proposal: ThemeProposal | null
  /** Who was speaking, and which three cards were on the table.
   *
   *  Stashed because **a persona is fixed for a conversation** (ADR 21): the
   *  transcript is resent whole every turn, so a voice swapped halfway leaves
   *  every earlier answer speaking in the old one. Restoring a conversation
   *  under a different reader is the same fault with a reload in the middle,
   *  so a stash whose persona or seed does not match what the door is offering
   *  is discarded rather than adopted. */
  persona: string
  seed: number | null
}

const EMPTY: Saved = {
  transcript: [], slots: [], job: null, proposal: null,
  persona: 'plain', seed: null,
}

function load(persona: string, seed: number | null): Saved {
  const empty: Saved = { ...EMPTY, persona, seed }
  try {
    const raw = localStorage.getItem(SAVED)
    if (!raw) return empty
    const parsed = JSON.parse(raw) as Partial<Saved>
    // A conversation belongs to one reader and one spread. Anything else is
    // somebody else's conversation wearing this one's costume.
    const was = typeof parsed.persona === 'string' ? parsed.persona : 'plain'
    const dealt = typeof parsed.seed === 'number' ? parsed.seed : null
    if (was !== persona || dealt !== seed) return empty
    return {
      transcript: Array.isArray(parsed.transcript) ? parsed.transcript : [],
      slots: Array.isArray(parsed.slots) ? parsed.slots : [],
      job: typeof parsed.job === 'string' ? parsed.job : null,
      proposal: parsed.proposal ?? null,
      persona, seed,
    }
  } catch {
    // A corrupted stash is not worth an error message. Start again.
    return empty
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

export function ThemeInterview({
  onPick, onLeave, persona = 'plain', seed = null, intro, leaveLabel,
}: {
  onPick: (key: string, card: ThemeCommander) => void
  onLeave: () => void
  /** Which voice is asking (ADR 21). Fixed for the life of the conversation —
   *  the tarot door remounts this component when the reader changes, which is
   *  what makes "changing it restarts" a fact rather than a request. */
  persona?: string
  /** The spread's seed, when the reader was dealt one. Re-deals the identical
   *  three cards server-side, so the conversation and the table can never
   *  disagree about what is face up. */
  seed?: number | null
  /** The reader's own framing, when somebody else is setting the scene. */
  intro?: { title: string; blurb: string }
  leaveLabel?: string
}) {
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [saved, setSaved] = useState<Saved>(() => load(persona, seed))
  const { transcript, slots, proposal } = saved
  const [report, setReport] = useState<ThemeReport | null>(null)
  const [answer, setAnswer] = useState('')
  const [budget, setBudget] = useState('')
  const [busy, setBusy] = useState<'' | 'asking' | 'proposing'>('')
  const [elapsed, setElapsed] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const box = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    api.claudeStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

  useEffect(() => {
    localStorage.setItem(SAVED, JSON.stringify(saved))
  }, [saved])

  // The turn is a background job too now, for the reason `api.themeAsk` gives:
  // measured at 4.3–37.7s with one at 133.8s, against a transport ceiling
  // nobody has measured and that is known only to be at or below 236s.
  //
  // Unlike the proposal the id is **not** saved. That one persists it because
  // a reload inside four minutes would otherwise pay twice; a turn is seconds,
  // and persisting it here would put a second claimant on the transcript
  // alongside the auto-ask effect below — two paths that could both decide
  // what the pending question is. A reload mid-turn re-asks, which is what it
  // did when this was a plain POST.
  const asker = useRef<{ cancel: () => void } | null>(null)

  // Stable, so the opening effect below can depend on it honestly rather than
  // being told to ignore it.
  const send = useCallback(async (next: ThemeTurn[], carried: ThemeSlot[]) => {
    setBusy('asking')
    setError(null)
    asker.current?.cancel()
    try {
      const job = await api.themeAsk({
        transcript: next, slots: carried,
        persona, seed: seed ?? undefined,
      })
      // `initial` is what keeps the cheap case cheap: stance `off` and a
      // finished conversation come back already `done`, and this resolves with
      // no poll at all. 400ms otherwise — this is somebody waiting on a
      // question, not a four-minute proposal, so the 2s the proposal polls at
      // would be most of the latency on a fast turn.
      const run = followJob(job.id, () => {}, 400, job)
      asker.current = { cancel: run.cancel }
      const got = (await run.promise).result as ThemeReport
      setReport(got)
      setSaved((s) => ({
        ...s,
        transcript: got.question
          ? [...next, { role: 'assistant', text: got.question }]
          : next,
        slots: got.slots,
      }))
    } catch (e) {
      setError(e instanceof ApiError && e.status === 404
        // Same sentence the proposal shows, and the same cause: jobs live in
        // the server's memory and die with it (`api/jobs.py`).
        ? 'That question is gone — the server restarted while it was working. Say something and it will ask again.'
        : String((e as Error).message ?? e))
    } finally {
      setBusy('')
      box.current?.focus()
    }
  }, [persona, seed])

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
    // Not while a proposal is in flight either. The transcript almost always
    // ends on a question by then so this rarely fires, but a tab restored
    // mid-run can end on an answer, and asking a fresh question underneath a
    // running proposal is confusing rather than helpful.
    if (saved.job) return
    if (transcript[transcript.length - 1]?.role === 'assistant') return
    if (awaited.current === transcript.length) return
    awaited.current = transcript.length
    void send(transcript, slots)
  }, [status, busy, transcript, slots, saved.job, send])

  function answerIt() {
    const said = answer.trim()
    if (!said || busy) return
    setAnswer('')
    // Record the answer and stop. The effect above notices there is no
    // question pending and fetches one — which keeps "who asks for the next
    // question" in exactly one place rather than two that can both fire.
    setSaved((s) => ({
      ...s, transcript: [...s.transcript, { role: 'user', text: said }],
    }))
  }

  // The proposal is a background job now (`api/themeruns.py`), because it was
  // measured at 226 seconds and a four-minute POST does not survive a hosted
  // proxy. So this is the simulator's shape: submit, poll, read the result off
  // the job. What it adds is that the id is *saved* — a reload reattaches to
  // the run in flight rather than paying for a second one.
  const poller = useRef<{ cancel: () => void } | null>(null)
  const followed = useRef<string | null>(null)

  const follow = useCallback((id: string) => {
    setBusy('proposing')
    setError(null)
    poller.current?.cancel()
    // How old the *run* is, not how long this tab has been watching it. They
    // differ exactly when it matters: reattaching after a reload showed 0s
    // against a job already a minute in, which is the confusion a clock was
    // put there to remove. The job's own `created_at` is the answer, and the
    // local start is only the guess held until the first poll corrects it.
    const started = { at: Date.now() }
    setElapsed(0)
    // Seconds rather than a percentage bar. The job reports its turn out of a
    // ceiling it usually does not reach, so a bar would sit at 38% and then
    // jump; an honest clock is more use to somebody deciding whether to wait.
    const clock = setInterval(
      () => setElapsed(Math.round((Date.now() - started.at) / 1000)), 1000)
    // Two seconds, not the 400ms default: this runs for minutes, and a poll
    // every 400ms would be six hundred requests to watch one job.
    const run = followJob(id, (job) => {
      const born = Date.parse(job.created_at)
      if (!Number.isNaN(born)) started.at = born
    }, 2000)
    poller.current = { cancel: () => { run.cancel(); clearInterval(clock) } }
    run.promise
      .then((job) => setSaved((s) => (
        { ...s, proposal: job.result as ThemeProposal, job: null })))
      .catch((e) => {
        setError(e instanceof ApiError && e.status === 404
          // Jobs live in the server's memory and die with it (`api/jobs.py`).
          // Say so rather than showing a bare 404 for something the person
          // never asked to look up.
          ? 'That run is gone — the server restarted while it was working. Ask again when you are ready.'
          : String((e as Error).message ?? e))
        setSaved((s) => ({ ...s, job: null }))
      })
      .finally(() => { clearInterval(clock); setBusy('') })
  }, [])

  // One place decides to follow a job, and it is this — the same argument as
  // the auto-ask effect above. `proposeIt` records the id and stops; a
  // restored tab arrives with the id already in state and lands here too, so
  // there is no second path that could double-poll.
  useEffect(() => {
    if (!saved.job || followed.current === saved.job) return
    followed.current = saved.job
    follow(saved.job)
  }, [saved.job, follow])

  // Both pollers. The tarot door remounts this component when the reader
  // changes (ADR 21 — a persona is fixed for a conversation), so an unmount
  // here is a live turn somebody has walked away from, not just a closing tab.
  useEffect(() => () => {
    poller.current?.cancel()
    asker.current?.cancel()
  }, [])

  async function proposeIt() {
    setBusy('proposing')
    setError(null)
    setElapsed(0)
    try {
      const job = await api.themePropose({
        transcript, slots,
        budget: budget ? Number(budget) : undefined,
        persona, seed: seed ?? undefined,
      })
      setSaved((s) => ({ ...s, job: job.id }))
    } catch (e) {
      // 409 below the floor, 503 with no key, 422 for a transcript the server
      // will not take — all still answered by the POST itself, which is why
      // they read as sentences here rather than as a job that failed.
      setError(String((e as Error).message ?? e))
      setBusy('')
    }
  }

  function startOver() {
    localStorage.removeItem(SAVED)
    poller.current?.cancel()
    // The turn in flight goes too, or its question lands in the conversation
    // that was just cleared — the empty transcript would gain an assistant
    // turn nobody asked for and the auto-ask effect would never fire.
    asker.current?.cancel()
    awaited.current = -1
    followed.current = null
    // Starting over keeps the reader and the cards. Those were chosen on the
    // way in and are the door's to change, not this button's — "start over"
    // means these answers, not this table.
    setSaved({ ...EMPTY, persona, seed })
    setReport(null)
    setError(null)
    setBusy('')
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
          {leaveLabel ?? '← Pick colours myself'}
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
            {intro?.title ?? 'Let’s work out what you want'}
          </h2>
          <p className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            {intro?.blurb ?? 'No Magic knowledge needed — the questions are '
              + 'about you. Magic’s five colours started life as five '
              + 'philosophies, so this is less of a detour than it sounds.'}
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
            {leaveLabel ?? '← Pick colours myself'}
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
                {busy === 'proposing'
                  ? `Reading around… ${elapsed}s`
                  : 'Suggest my colours'}
              </button>
              <p className="mt-1 text-[11px]" style={{ color: 'var(--text-muted)' }}>
                {/* Measured at three to four minutes: it reads a dozen-odd
                    pages and checks every legend against the pool. Saying so
                    is cheaper than a spinner somebody assumes has hung — and
                    the clock on the button is cheaper still, because a number
                    that moves is the difference between slow and stuck. */}
                {busy === 'proposing'
                  ? 'A few minutes — it reads around and checks every card. It carries on if you reload or close the tab.'
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
            <button onClick={() => setSaved((s) => ({ ...s, proposal: null }))}
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
