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

/**
 * Ludevic, Necro-Alchemist, Aaron Miller (Commander 2016) — Magic's
 * canonical mad scientist, hired as the Laboratory's keeper (punch list
 * 2026-08-15 item 7). Looked up on Scryfall, not recalled; hotlinked with
 * the artist credited, per the persona tiles' rules.
 */
const LUDEVIC_ART =
  'https://cards.scryfall.io/art_crop/front/f/3/f3e7a886-2593-4e6e-b9da-d7cb417cba08.jpg'

/**
 * The bench (punch list item 7): racks of glassware, a burner, a condenser
 * mid-drip, jars on a shelf, notes the keeper never files. Inline SVG and
 * CSS like every other drawn thing in this app — no asset, no licence — and
 * all of it `aria-hidden`: it is the room the question box sits in, not
 * information.
 *
 * `busy` is the fun part: while a question is out being researched the
 * bench *cooks* — `--lab-speed` drops and every bubble, drip and flame
 * animation runs faster, so minutes of waiting read as the lab working
 * rather than as a stuck page. The speed lives in one custom property so
 * the busy state is one class, not eleven animation overrides.
 */
function LabBench({ busy }: { busy: boolean }) {
  return (
    <div className={`lab-bench${busy ? ' is-cooking' : ''}`} aria-hidden="true">
      <svg viewBox="0 0 900 190" className="h-auto w-full" fill="none">
        {/* The back shelf, and what lives on it. */}
        <rect x="500" y="52" width="380" height="5" rx="1" fill="#6e4e28" />
        <rect x="497" y="57" width="8" height="14" fill="#5c3f1e" />
        <rect x="875" y="57" width="8" height="14" fill="#5c3f1e" />
        {/* Specimen jars: something teal, something that floats, something
            best not asked about. */}
        <g opacity="0.9">
          <rect x="520" y="24" width="26" height="28" rx="3" fill="#1d3a3a" />
          <rect x="522" y="30" width="22" height="20" rx="2" fill="#2e8f7f"
                opacity="0.55" />
          <ellipse cx="533" cy="40" rx="6" ry="4" fill="#0f2424" opacity="0.8" />
          <rect x="518" y="20" width="30" height="5" rx="2" fill="#8a6a33" />
        </g>
        <g opacity="0.9">
          <rect x="560" y="30" width="20" height="22" rx="3" fill="#241d3a" />
          <rect x="562" y="34" width="16" height="16" rx="2" fill="#8f79e8"
                opacity="0.45" />
          <circle className="lab-float" cx="570" cy="42" r="3" fill="#3a2f5c" />
          <rect x="558" y="26" width="24" height="5" rx="2" fill="#8a6a33" />
        </g>
        <g opacity="0.9">
          <rect x="596" y="26" width="24" height="26" rx="3" fill="#3a2e1d" />
          <rect x="598" y="32" width="20" height="18" rx="2" fill="#c9a227"
                opacity="0.4" />
          <path d="M602 44 q 4 -6 8 0 q 4 6 8 0" stroke="#8a6a33"
                strokeWidth="1.2" opacity="0.8" />
          <rect x="594" y="22" width="28" height="5" rx="2" fill="#8a6a33" />
        </g>
        {/* Books leaning at the shelf's end — it is still a library. */}
        <rect x="800" y="22" width="9" height="30" rx="1" fill="#7a3b2e" />
        <rect x="811" y="25" width="8" height="27" rx="1" fill="#2e5c46"
              transform="rotate(6 815 52)" />
        <rect x="823" y="24" width="9" height="28" rx="1" fill="#3a4d7a"
              transform="rotate(11 827 52)" />

        {/* The bench itself. */}
        <rect x="0" y="168" width="900" height="7" rx="1" fill="#6e4e28" />
        <rect x="0" y="175" width="900" height="12" fill="#4a3212" />

        {/* The burner, its flame, and the beaker it worries. */}
        <g>
          <rect x="96" y="158" width="30" height="10" rx="2" fill="#55606e" />
          <rect x="108" y="146" width="6" height="14" fill="#55606e" />
          <path className="lab-flame"
                d="M111 146 C 106 138 108 130 111 124 C 114 130 116 138 111 146 Z"
                fill="#f0a05a" />
          <path className="lab-flame lab-flame-2"
                d="M111 144 C 108.5 139 109.5 133 111 129 C 112.5 133 113.5 139 111 144 Z"
                fill="#5aa0f0" opacity="0.85" />
          {/* Tripod and beaker. */}
          <path d="M92 168 L 104 120 M 130 168 L 118 120 M 96 132 H 126"
                stroke="#55606e" strokeWidth="2.5" strokeLinecap="round" />
          <path d="M92 92 H 130 L 127 120 H 95 Z" fill="#cfe4ea" opacity="0.3" />
          <rect x="95" y="103" width="32" height="16" fill="#2e8f7f"
                opacity="0.75" />
          <ellipse cx="111" cy="103" rx="16" ry="3" fill="#3aa08c" />
          <circle className="lab-bubble" cx="104" cy="114" r="2" fill="#bfeadf" />
          <circle className="lab-bubble lab-bubble-2" cx="114" cy="116" r="1.5"
                  fill="#bfeadf" />
          <circle className="lab-bubble lab-bubble-3" cx="109" cy="112" r="1.2"
                  fill="#bfeadf" />
          {/* Steam, once the bubbles have done their work. */}
          <circle className="lab-steam" cx="106" cy="88" r="4" fill="#cfe4ea" />
          <circle className="lab-steam lab-steam-2" cx="116" cy="84" r="3"
                  fill="#cfe4ea" />
        </g>

        {/* The erlenmeyer, mid-thought. */}
        <g>
          <path d="M232 96 H 248 L 249 112 L 266 160 A 6 6 0 0 1 260 168
                   H 220 A 6 6 0 0 1 214 160 L 231 112 Z"
                fill="#cfe4ea" opacity="0.28" />
          <path d="M224 138 L 256 138 L 264 160 A 5 5 0 0 1 259 166
                   H 221 A 5 5 0 0 1 216 160 Z" fill="#8f79e8" opacity="0.7" />
          <ellipse cx="240" cy="138" rx="16" ry="3" fill="#a99af0" />
          <circle className="lab-bubble lab-bubble-2" cx="234" cy="152" r="2"
                  fill="#d5ccf7" />
          <circle className="lab-bubble lab-bubble-4" cx="246" cy="156" r="1.4"
                  fill="#d5ccf7" />
          <rect x="230" y="90" width="20" height="6" rx="2" fill="#8a6a33" />
        </g>

        {/* The retort and condenser: a round flask over a stand, a tube
            sloping down through a water jacket, and a drip that will not
            stop. The drip is the whole joke of a condenser. */}
        <g>
          <circle cx="360" cy="132" r="26" fill="#cfe4ea" opacity="0.28" />
          <path d="M338 143 A 26 26 0 0 0 382 143 Z" fill="#3aa08c"
                opacity="0.8" />
          <ellipse cx="360" cy="143" rx="21" ry="3.5" fill="#4db8a2" />
          <rect x="354" y="94" width="12" height="16" rx="2" fill="#cfe4ea"
                opacity="0.5" />
          <path d="M344 168 L 352 154 M 376 168 L 368 154 M 346 160 H 374"
                stroke="#55606e" strokeWidth="2.2" strokeLinecap="round" />
          <circle className="lab-bubble lab-bubble-3" cx="354" cy="150" r="2"
                  fill="#bfeadf" />
          <circle className="lab-bubble" cx="366" cy="152" r="1.5" fill="#bfeadf" />
          {/* The tube out of the neck, through the jacket, to the beaker. */}
          <path d="M366 96 C 390 84 410 92 424 110 C 434 122 440 132 444 142"
                stroke="#cfe4ea" strokeWidth="3.5" opacity="0.7" />
          <rect x="398" y="88" width="34" height="14" rx="7" fill="#5aa0f0"
                opacity="0.35" transform="rotate(32 415 95)" />
          {/* The receiving beaker, and the drip. */}
          <path d="M432 144 H 460 L 458 168 H 434 Z" fill="#cfe4ea"
                opacity="0.3" />
          <rect x="434" y="156" width="23" height="10" fill="#6fae7f"
                opacity="0.7" />
          <circle className="lab-drip" cx="444" cy="144" r="1.8" fill="#bfeadf" />
        </g>

        {/* The test tube rack: four verdicts pending. */}
        <g>
          <rect x="656" y="140" width="92" height="6" rx="2" fill="#6e4e28" />
          <rect x="656" y="162" width="92" height="6" rx="2" fill="#6e4e28" />
          <rect x="660" y="146" width="4" height="16" fill="#5c3f1e" />
          <rect x="740" y="146" width="4" height="16" fill="#5c3f1e" />
          {[
            [672, '#3aa08c', 26],
            [692, '#8f79e8', 34],
            [712, '#c9a227', 20],
            [732, '#c75e5e', 30],
          ].map(([x, color, depth]) => (
            <g key={String(x)}>
              <rect x={Number(x) - 5} y={112} width="10" height="54" rx="5"
                    fill="#cfe4ea" opacity="0.28" />
              <rect x={Number(x) - 5} y={166 - Number(depth)} width="10"
                    height={Number(depth)} rx="4" fill={String(color)}
                    opacity="0.8" />
            </g>
          ))}
          <circle className="lab-bubble lab-bubble-4" cx="692" cy="150" r="1.6"
                  fill="#d5ccf7" />
          <circle className="lab-bubble lab-bubble-2" cx="732" cy="152" r="1.4"
                  fill="#f0c0c0" />
        </g>

        {/* Notes strewn where notes get strewn. The writing is five strokes
            nobody can read, which is faithful to the keeper's hand. */}
        <g transform="rotate(-7 790 158)">
          <rect x="768" y="146" width="44" height="30" rx="2" fill="#f0e4c2" />
          {[153, 158, 163, 168].map((y, i) => (
            <path key={y} d={`M774 ${y} H ${i === 3 ? 794 : 806}`}
                  stroke="#8a7a55" strokeWidth="1" opacity="0.6" />
          ))}
        </g>
        <g transform="rotate(5 842 162)">
          <rect x="820" y="150" width="44" height="28" rx="2" fill="#e8dcb8" />
          {[157, 162, 167, 172].map((y, i) => (
            <path key={y} d={`M826 ${y} H ${i === 2 ? 844 : 858}`}
                  stroke="#8a7a55" strokeWidth="1" opacity="0.6" />
          ))}
          <circle cx="852" cy="171" r="3" fill="#3aa08c" opacity="0.4" />
        </g>
      </svg>
    </div>
  )
}

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

      {/* The room (punch list item 7): the bench, and the keeper who works
          it. The bench cooks while a question is out. */}
      <div className="flex flex-col gap-4 lg:flex-row lg:items-stretch">
        <div className="card-surface min-w-0 flex-1 overflow-hidden rounded-xl">
          <LabBench busy={busy} />
        </div>
        <aside className="card-surface flex w-full shrink-0 flex-col overflow-hidden rounded-xl lg:w-60">
          <img src={LUDEVIC_ART}
               alt="Ludevic, Necro-Alchemist, painted by Aaron Miller: a
                    wild-eyed alchemist at work by lamplight."
               className="aspect-[626/457] w-full object-cover" />
          <div className="flex flex-1 flex-col gap-1 px-4 py-3">
            <p className="text-[10px] uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              The keeper
            </p>
            <p className="text-sm font-medium">Ludevic, Necro-Alchemist</p>
            <p className="text-xs leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              Magic&rsquo;s own mad scientist runs this room. He reads
              everything, cites his pages, and cannot see your decks —
              which, given what he does with donated material, is probably
              for the best.
            </p>
            <p className="mt-auto pt-1 text-[10px]"
               style={{ color: 'var(--text-muted)' }}>
              Art by Aaron Miller
            </p>
          </div>
        </aside>
      </div>

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
        <section className="flex flex-col gap-3">
          <h2 className="text-xs font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            From the keeper&rsquo;s notes
          </h2>
          {/* The strewn notes the punch list asked for, made load-bearing:
              each example question is a parchment slip dropped at its own
              angle (`.lab-note`, index.css), and picking one up asks it. */}
          <ul className="grid gap-3 sm:grid-cols-2">
            {EXAMPLES.map((example) => (
              <li key={example} className="lab-note-slot">
                <button type="button"
                        onClick={() => { setQuestion(example); void ask(example) }}
                        className="lab-note w-full px-4 py-3 text-left text-sm leading-relaxed">
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
