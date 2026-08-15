/**
 * Research — the questions the pool cannot answer, with the pages that can.
 * [ADR 26](../../../docs/adr/0026-research-answers-about-magic-not-about-your-deck.md).
 *
 * The app's first screen that is a question box, and the reason it is its own
 * page rather than a panel on a deck is the same reason it is its own ADR:
 * **this surface cannot see a deck.** Not "does not"; cannot. There is no
 * owner and no slug in the request, no deck tool in the mode, and no
 * `DeckSource` anywhere in the call. A panel sitting on the deck page would
 * say the opposite of that with its position, before a word of the answer
 * loaded.
 *
 * Four things this page has to get right, all of them ADR 26 rendered:
 *
 * **The two kinds of card fact stay visibly apart.** A card the pool has shows
 * the pool's own text. A card it has not — anything spoiled since the last
 * `data refresh`, which is a third of why this surface exists — shows a name
 * and a sentence saying so. Merging them would produce exactly the blended
 * paragraph ADR 19 rejected, one source further out.
 *
 * **Every finding carries its citations, as links.** A number that does
 * nothing asks the reader to take the citation on trust, which is the one
 * thing this feature may not do. The dossier's markers, and the same
 * reasoning.
 *
 * **The dropped counts render.** A number that climbs is a prompt that has
 * started inventing, and nobody checks a number they cannot see.
 * `cards_unresolved` is stated separately because it is the one that is *not*
 * a fault.
 *
 * **The waiting is honest.** This runs as a background job because the
 * dossier's synchronous version broke deployed at 236 seconds — a spinner and
 * then Safari's `Load failed`, no status code, no access-log line. The elapsed
 * counter here exists so minutes of nothing reads as work rather than as a
 * wedge.
 */

import { useEffect, useRef, useState } from 'react'

import { StanceReadout } from '../components/stance'
import {
  Badge, CardHover, ErrorNote, ManaCost, PageMasthead, Spinner,
} from '../components/ui'

/**
 * Novijen, Heart of Progress, Martina Pilcerova (the Commander 2021
 * printing) — the Simic guildhall, a laboratory the size of a district and
 * grown rather than built. The one masthead that steps outside the Mystical
 * Archive cycle (`CardSearch.tsx` argues the cycle), deliberately: this page
 * was renamed the Laboratory, and Simic art is what a laboratory looks like
 * in Magic. Looked up on Scryfall, not recalled; hotlinked and credited per
 * `PageMasthead`'s rules.
 */
const NOVIJEN_ART =
  'https://cards.scryfall.io/art_crop/front/9/a/9a1e15e7-4ba6-41ad-b27b-aee2d037b6a7.jpg'
import {
  api, errorMessage, followJob, hasResearch,
  type ClaudeStatus, type ResearchBody, type ResearchCard, type ResearchReport,
} from '../lib/api'
import { effectivePin, fetchClaudeStatus, useStance } from '../lib/stance'

/** A small Simic flask beside the nameplate. Decorative, so `aria-hidden`;
 *  the bubbles rise in CSS (`.flask-bubble`, index.css) and hold still under
 *  `prefers-reduced-motion`. */
function Flask() {
  return (
    <svg aria-hidden viewBox="0 0 24 24" className="h-6 w-6 shrink-0"
         style={{ color: 'var(--series-2)' }}>
      <path d="M9 3h6M10 3v5.2L4.8 18a2.4 2.4 0 0 0 2.1 3.6h10.2a2.4 2.4 0 0 0 2.1-3.6L14 8.2V3"
            fill="none" stroke="currentColor" strokeWidth="1.5"
            strokeLinecap="round" strokeLinejoin="round" />
      <path d="M7.4 15.5h9.2" stroke="currentColor" strokeWidth="1.2" />
      <circle className="flask-bubble" cx="10.5" cy="18" r="0.9" fill="currentColor" />
      <circle className="flask-bubble flask-bubble-2" cx="13.5" cy="18.8" r="0.7"
              fill="currentColor" />
      <circle className="flask-bubble flask-bubble-3" cx="12" cy="19.4" r="0.5"
              fill="currentColor" />
    </svg>
  )
}

/** Kept short deliberately. Every one is a question the deck page cannot
 *  answer and the pool does not hold — which is the whole boundary, shown
 *  rather than explained. */
const EXAMPLES = [
  'What did the last banned-and-restricted announcement change for Commander?',
  'How does a "dies" trigger interact with a creature that is exiled by its own ability?',
  'What is the current consensus on Game Changers in bracket 3 pods?',
  'What was previewed in the most recent Commander precon cycle?',
]

export default function Research() {
  const [question, setQuestion] = useState('')
  const [report, setReport] = useState<ResearchReport | null>(null)
  const [status, setStatus] = useState<ClaudeStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [elapsed, setElapsed] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [pin, setPin] = useStance()
  const run = useRef<{ cancel: () => void } | null>(null)

  // `surface: 'research'` and no slug. Without it `/api/claude` resolves
  // through `stance.resolve(None, None)` and answers `off`, while
  // `research.stance_for` is about to run the search at `second-opinion` — the
  // exact fault the create flow shipped once, in the only other place it could
  // happen. A dial that misreports the stance it governs is worse than no dial.
  useEffect(() => {
    fetchClaudeStatus({ surface: 'research' }, pin, () => setPin(null))
      .then(setStatus).catch(() => setStatus(null))
  }, [pin, setPin])

  // Minutes of a still spinner is indistinguishable from a wedge. Ticking is
  // the cheapest honest signal, and it is why `converse` reports turns at all.
  useEffect(() => {
    if (!busy) return
    const started = Date.now()
    setElapsed(0)
    const id = window.setInterval(
      () => setElapsed(Math.round((Date.now() - started) / 1000)), 1000)
    return () => window.clearInterval(id)
  }, [busy])

  useEffect(() => () => run.current?.cancel(), [])

  async function ask(text: string) {
    const asked = text.trim()
    if (!asked || busy) return
    setBusy(true)
    setError(null)
    setReport(null)
    run.current?.cancel()
    try {
      const job = await api.research(
        { question: asked, stance: effectivePin(pin, status) })
      // `initial` is what keeps the cheap case cheap: a stance of `off` comes
      // back already `done` and this resolves without a single poll. 2s
      // otherwise — this is minutes of searching, not a chat turn, so polling
      // at 400ms would be noise.
      const following = followJob(job.id, () => {}, 2000, job)
      run.current = { cancel: following.cancel }
      setReport((await following.promise).result as ResearchReport)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  const unavailable = status && !(status.installed && status.configured)

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-6">
      <PageMasthead
        art={NOVIJEN_ART}
        alt="Novijen, Heart of Progress, painted by Martina Pilcerova: the
             Simic guildhall rising over Ravnica, a laboratory grown into a
             city district."
        title={<span className="inline-flex items-center gap-2">
          Laboratory <Flask />
        </span>}
        credit={<>
          <em>Novijen, Heart of Progress</em> by Martina Pilcerova — the
          Simic guildhall, a laboratory the size of a district.
        </>}>
        <p className="max-w-2xl leading-relaxed">
          Ask about the things this tool cannot compute: the meta, what a ruling
          means in practice, a card spoiled since the last data refresh. Every
          answer comes back with the pages it was read from.
        </p>
        {/* Said on the screen rather than only in an ADR, and said before the
            first question rather than in an apology afterwards. */}
        <p className="mt-2 max-w-2xl leading-relaxed"
           style={{ color: 'var(--text-muted)' }}>
          It cannot see your decks. That is deliberate — this surface has no
          access to your library at all, so ask about the card and the format,
          and let the deck page&rsquo;s gate, simulator and slot argument answer
          the rest.
        </p>
      </PageMasthead>

      <form className="card-surface flex flex-col gap-3 rounded-xl px-5 py-4"
            onSubmit={(e) => { e.preventDefault(); void ask(question) }}>
        <label htmlFor="research-question" className="text-xs font-medium"
               style={{ color: 'var(--text-primary)' }}>
          Your question
        </label>
        <textarea
          id="research-question"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              void ask(question)
            }
          }}
          rows={3}
          placeholder="Ask about Magic — not about a deck."
          className="w-full resize-y rounded-lg px-3 py-2 text-sm"
          style={{
            background: 'var(--page)',
            border: '1px solid var(--hairline)',
            color: 'var(--text-primary)',
          }}
        />
        <div className="flex flex-wrap items-center gap-3">
          <button type="submit" disabled={busy || !question.trim() || !!unavailable}
                  className="rounded-lg px-4 py-2 text-sm font-medium transition disabled:opacity-40"
                  style={{ background: 'var(--series-2)', color: 'var(--page)' }}>
            {busy ? 'Searching…' : 'Ask'}
          </button>
          {busy && (
            <Spinner label={`reading pages… ${elapsed}s`} />
          )}
          {unavailable && (
            <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
              {status?.installed
                ? 'Claude has no key to call with on this server.'
                : 'Claude isn’t installed on this server.'}
            </span>
          )}
        </div>
        {status && <StanceReadout status={status} pin={pin} />}
      </form>

      {!report && !busy && (
        <section className="flex flex-col gap-2">
          <h2 className="text-xs font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            Things worth asking
          </h2>
          <ul className="flex flex-col gap-1">
            {EXAMPLES.map((example) => (
              <li key={example}>
                <button type="button"
                        onClick={() => { setQuestion(example); void ask(example) }}
                        className="text-left text-sm underline-offset-2 hover:underline"
                        style={{ color: 'var(--series-2)' }}>
                  {example}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}

      {error && <ErrorNote>{error}</ErrorNote>}

      {report && !hasResearch(report) && (
        // A refused answer, and it says why. ADR 26 refuses rather than
        // rendering an unsourced paragraph, so this branch is a feature and
        // reads as one.
        <div className="card-surface rounded-xl px-5 py-4 text-sm"
             style={{ color: 'var(--text-secondary)' }}>
          {report.reason || 'Nothing came back.'}
        </div>
      )}

      {report && hasResearch(report) && (
        <Answer body={report.research} report={report} />
      )}
    </div>
  )
}

/** How sure the pages are that they agree — never how sure the answer is that
 *  it is right. Rendered because a mode with no way to say "people disagree"
 *  writes consensus that is not there. */
function Confidence({ level }: { level: string }) {
  const tone = level === 'settled' ? 'good' : level === 'thin' ? 'warning' : 'neutral'
  const says = {
    settled: 'the pages agree',
    contested: 'informed people disagree',
    thin: 'little was found',
  }[level] ?? level
  return <Badge tone={tone}>{level} — {says}</Badge>
}

function Cites({ ids, sources }: {
  ids: string[]
  sources: ResearchBody['sources']
}) {
  // The dossier's markers and the dossier's reasoning: a citation that does
  // nothing asks the reader to take it on trust. An id with no matching source
  // renders as nothing, which is the instrument rather than a convenience —
  // though it should not happen here, because the server already dropped any
  // finding left citing only those.
  const shown = ids.flatMap((id) => {
    const at = sources.findIndex((s) => s.id === id)
    const source = sources[at]
    return source ? [{ id, at, source }] : []
  })
  if (!shown.length) return null
  return (
    <>
      {shown.map(({ id, at, source }) => (
        <a key={id} href={`#src-${id}`} title={source.title}
           className="ml-1 align-super text-[10px] font-medium no-underline"
           style={{ color: 'var(--series-2)' }}>
          [{at + 1}]
        </a>
      ))}
    </>
  )
}

/**
 * One named card, rendered as one of two visibly different things.
 *
 * This component is ADR 26's third decision and nothing else. A card the pool
 * has is a rule-1 fact and gets the pool's own text and the usual hover. A
 * card the pool lacks gets a name and a sentence saying the tool has not
 * ingested it — because a card spoiled since the last refresh is real, is what
 * a third of this surface's questions are about, and is *not* something the
 * pool can vouch for.
 */
function NamedCard({ card }: { card: ResearchCard }) {
  if (!card.in_pool) {
    return (
      <li className="rounded-lg px-3 py-2 text-sm"
          style={{
            border: '1px dashed var(--hairline)',
            color: 'var(--text-secondary)',
          }}>
        <span className="font-medium" style={{ color: 'var(--text-primary)' }}>
          {card.name}
        </span>
        <p className="mt-1 text-xs leading-relaxed"
           style={{ color: 'var(--text-muted)' }}>
          Not in the card pool. Everything said about this card above comes from
          a cited page, not from a card lookup — run{' '}
          <code>mtglab data refresh</code> once it is in Scryfall&rsquo;s bulk
          data.
        </p>
      </li>
    )
  }
  return (
    <li className="rounded-lg px-3 py-2 text-sm"
        style={{ border: '1px solid var(--hairline)' }}>
      <div className="flex flex-wrap items-center gap-2">
        <CardHover card={{ name: card.name, image: card.image }}>
          <span className="font-medium" style={{ color: 'var(--text-primary)' }}>
            {card.name}
          </span>
        </CardHover>
        {card.mana_cost && <ManaCost cost={card.mana_cost} size={13} />}
        {card.legal_commander === false && (
          <Badge tone="critical">banned in Commander</Badge>
        )}
      </div>
      {card.type_line && (
        <p className="mt-0.5 text-xs" style={{ color: 'var(--text-muted)' }}>
          {card.type_line}
        </p>
      )}
      {card.oracle_text && (
        <p className="mt-1 whitespace-pre-line text-xs leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          {card.oracle_text}
        </p>
      )}
    </li>
  )
}

function Answer({ body, report }: {
  body: ResearchBody
  report: ResearchReport
}) {
  return (
    <div className="flex flex-col gap-6">
      <section className="card-surface flex flex-col gap-3 rounded-xl px-5 py-4">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="text-xs font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            Answer
          </h2>
          <Confidence level={body.confidence} />
        </div>
        <p className="text-sm leading-relaxed"
           style={{ color: 'var(--text-primary)' }}>
          {body.answer}
        </p>
      </section>

      {body.findings.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="text-xs font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            What the pages said
          </h2>
          <ul className="flex flex-col gap-2">
            {body.findings.map((finding, i) => (
              <li key={i} className="text-sm leading-relaxed"
                  style={{ color: 'var(--text-secondary)' }}>
                {finding.claim}
                <Cites ids={finding.source_ids} sources={body.sources} />
              </li>
            ))}
          </ul>
        </section>
      )}

      {body.cards.length > 0 && (
        <section className="flex flex-col gap-2">
          <h2 className="text-xs font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            Cards named
          </h2>
          <ul className="flex flex-col gap-2">
            {body.cards.map((card) => (
              <NamedCard key={card.name} card={card} />
            ))}
          </ul>
        </section>
      )}

      <section>
        <h2 className="text-xs font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
          Sources — {body.searched} pages read, {body.sources.length} cited
        </h2>
        <ol className="mt-2 flex flex-col gap-1">
          {body.sources.map((source, i) => (
            <li key={source.id} id={`src-${source.id}`} className="text-xs">
              <span style={{ color: 'var(--text-muted)' }}>[{i + 1}]</span>{' '}
              <a href={source.url} target="_blank" rel="noreferrer noopener"
                 style={{ color: 'var(--series-2)' }}>
                {source.title || source.url}
              </a>
            </li>
          ))}
        </ol>

        {(body.sources_dropped > 0 || body.findings_dropped > 0) && (
          // Shown, not logged. A number that climbs means the model has started
          // inventing citations; silence here means everything it cited, it had
          // read.
          <p className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
            Discarded before you saw it: {body.sources_dropped} cited page(s)
            the search never returned, {body.findings_dropped} finding(s) left
            citing nothing.
          </p>
        )}
        {body.cards_unresolved > 0 && (
          // Deliberately a separate sentence from the line above. This count is
          // not a fault — for a spoiler question the right value is above zero
          // — and filing it with the dropped counts would read as one.
          <p className="mt-1 text-xs" style={{ color: 'var(--text-muted)' }}>
            {body.cards_unresolved} named card(s) are not in the pool yet, which
            is expected for anything spoiled since the last refresh.
          </p>
        )}
        <p className="mt-2 text-xs leading-relaxed"
           style={{ color: 'var(--text-muted)' }}>
          {report.never
            ?? 'This is Claude’s reading of the cited pages, not the '
               + 'tool’s own answer. It has not seen any of your decks.'}
        </p>
      </section>
    </div>
  )
}
