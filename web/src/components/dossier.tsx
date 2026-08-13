import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ApiError, api, errorMessage, followJob, hasDossier,
  type DossierBody, type DossierReport, type DossierSection, type Job,
} from '../lib/api'
import { CardHover, ManaCost, Spinner } from './ui'

/**
 * Where a run's id is parked so a reload can reattach to it.
 *
 * Keyed by slug because two deck pages are two runs, and the failure this
 * fixes was precisely that a reload lost the connection to work that was still
 * happening — the dossier was written and cached while the page showed
 * `Load failed`.
 */
const jobKey = (slug: string) => `mtglab-dossier-job:${slug}`

/**
 * The commander dossier, and the seams it is required to show.
 *
 * [ADR 19](../../../docs/adr/0019-the-dossier-cites-three-sources.md) says
 * three sources with different jurisdictions, and it says the page has to make
 * clear which is which. That is not decoration here — it is the acceptance
 * criterion, because a dossier rendered as one confident voice has erased the
 * only thing that makes it trustworthy. So:
 *
 * * The counted strip above this (`CommanderFacts`, every number a DuckDB
 *   query) stays exactly where it was and is not merged in.
 * * Everything in this panel is labelled as Claude's writing, once at the top
 *   where it cannot be missed rather than per paragraph where it becomes
 *   wallpaper.
 * * A claim that came from the web keeps its link, inline, as a numbered
 *   citation that scrolls to the source. The sources are listed with their
 *   real titles and real URLs — every one of which the server has already
 *   checked against the pages the search actually fetched.
 * * A rival is a real card, hoverable, showing its corpus text. That is the
 *   half of rule 1 that does not depend on the model complying: a first run
 *   described a rival as making Food tokens when it makes Soldiers, and the
 *   card being right there is what lets a reader catch it.
 *
 * Collapsed by default. The maintainer's note was that the deck page must not
 * get busy, and four paragraphs of prose above the 99 would bury the deck.
 */
export function CommanderDossierPanel({
  slug, commander, canGenerate, onLoaded,
}: {
  slug: string
  commander: string
  /** False at stance `off` — ADR 15 says off means no calls, so the button
   *  that would make one is absent rather than present and refusing. */
  canGenerate: boolean
  onLoaded?: (report: DossierReport) => void
}) {
  const [report, setReport] = useState<DossierReport | null>(null)
  const [open, setOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  // Read from storage on mount, so a tab that comes back finds the run it left.
  const [jobId, setJobId] = useState<string | null>(
    () => localStorage.getItem(jobKey(slug)))
  const [elapsed, setElapsed] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [fetched, setFetched] = useState(false)
  const poller = useRef<{ cancel: () => void } | null>(null)
  const followed = useRef<string | null>(null)

  const busy = submitting || jobId !== null

  // The free half. Reads a stored row and reaches no network beyond this app,
  // so opening the panel can ask for it without costing anything.
  async function load() {
    if (fetched) return
    setFetched(true)
    try {
      const got = await api.dossier(slug)
      setReport(got)
      onLoaded?.(got)
    } catch {
      // Silent: an absent dossier is the normal state and the deck is
      // perfectly usable without one. Only *writing* one reports failure.
    }
  }

  const settle = useCallback((job: Job) => {
    const got = job.result as DossierReport
    setReport(got)
    onLoaded?.(got)
    // A run that finishes into a collapsed panel has produced nothing anybody
    // can see. This matters on the reattach path, where the tab was reloaded
    // and the panel came back closed.
    setOpen(true)
    // A report with no dossier in it is the mode declining and saying why —
    // no source survived checking, the stance is off. That is a sentence to
    // show, not an error to swallow.
    if (!hasDossier(got) && got.reason) setError(got.reason)
  }, [onLoaded])

  const follow = useCallback((id: string) => {
    poller.current?.cancel()
    // How old the *run* is, not how long this tab has been watching it — the
    // two differ exactly when it matters, which is after a reload.
    const started = { at: Date.now() }
    setElapsed(0)
    const clock = setInterval(
      () => setElapsed(Math.round((Date.now() - started.at) / 1000)), 1000)
    // Two seconds, not the 400ms default: this runs for minutes, and polling
    // four times a second would be six hundred requests to watch one job.
    const run = followJob(id, (job) => {
      const born = Date.parse(job.created_at)
      if (!Number.isNaN(born)) started.at = born
    }, 2000)
    poller.current = { cancel: () => { run.cancel(); clearInterval(clock) } }
    run.promise
      .then(settle)
      .catch((e) => {
        setError(e instanceof ApiError && e.status === 404
          // Jobs live in the server's memory and die with it (`api/jobs.py`).
          // Say that rather than showing a bare 404 for something nobody asked
          // to look up.
          ? 'That run is gone — the server restarted while it was working. Ask again.'
          : errorMessage(e))
      })
      .finally(() => {
        clearInterval(clock)
        localStorage.removeItem(jobKey(slug))
        setJobId(null)
      })
  }, [slug, settle])

  // One place decides to follow a job: a fresh submission and a restored tab
  // both arrive here with the id in state, so there is no second path that
  // could double-poll.
  useEffect(() => {
    if (!jobId || followed.current === jobId) return
    followed.current = jobId
    follow(jobId)
  }, [jobId, follow])

  useEffect(() => () => poller.current?.cancel(), [])

  async function write(refresh: boolean) {
    setSubmitting(true)
    setError(null)
    try {
      const job = await api.writeDossier(slug, { refresh })
      if (job.status === 'done') {
        // A stored dossier, or a stance of `off`: a job born finished, so
        // there is nothing to poll for and no spinner to earn.
        settle(job)
        return
      }
      localStorage.setItem(jobKey(slug), job.id)
      setJobId(job.id)
    } catch (err) {
      // 422 with no commander, 503 with no key — both still answered by the
      // POST itself, which is why they read as sentences rather than as a job
      // that failed.
      setError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const body = hasDossier(report) ? report.dossier : null

  return (
    <div className="border-t" style={{ borderColor: 'var(--hairline)' }}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 px-5 py-3 sm:px-6">
        <button
          type="button"
          onClick={() => { setOpen(!open); void load() }}
          className="flex items-center gap-2 text-xs font-medium uppercase tracking-wide"
          style={{ color: 'var(--text-muted)' }}
          aria-expanded={open}
        >
          <span aria-hidden style={{
            display: 'inline-block',
            transition: 'transform 150ms',
            transform: open ? 'rotate(90deg)' : 'none',
          }}>▸</span>
          Who is {commander.split(',')[0]}?
        </button>

        {/* The label is on the outside of the collapsed panel deliberately.
            Somebody who never opens it should still know what is in there and
            who wrote it. */}
        <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide"
              style={{ background: 'color-mix(in srgb, var(--series-2) 18%, transparent)',
                       color: 'var(--series-2)' }}>
          Claude, with sources
        </span>

        {report?.generated_at && (
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
            written {report.generated_at.slice(0, 10)}
          </span>
        )}

        <span className="flex-1" />

        {canGenerate && (
          <button
            type="button"
            onClick={() => { setOpen(true); void write(!!body) }}
            disabled={busy}
            className="rounded-lg px-3 py-1.5 text-xs font-medium"
            style={{
              border: '1px solid var(--hairline)',
              background: 'var(--page)',
              opacity: busy ? 0.6 : 1,
              color: 'var(--text-secondary)',
            }}
          >
            {busy ? 'searching…' : body ? 'Write it again' : 'Write the dossier'}
          </button>
        )}
      </div>

      {open && (
        <div className="px-5 pb-5 sm:px-6">
          {busy && !body && (
            // A clock rather than a bar. This takes minutes, the job reports a
            // turn out of a ceiling it usually does not reach, and seconds are
            // what somebody deciding whether to wait actually wants.
            <Spinner label={elapsed
              ? `reading the web and the corpus… ${elapsed}s`
              : 'reading the web and the corpus…'} />
          )}
          {error && (
            <p className="text-sm" style={{ color: 'var(--status-critical)' }}>
              {error}
            </p>
          )}
          {!busy && !body && !error && (
            <p className="max-w-2xl text-sm" style={{ color: 'var(--text-muted)' }}>
              {canGenerate
                ? 'Nothing written yet. The corpus facts above are counted; '
                  + 'this is the part that needs reading around — who the '
                  + 'character is, what archetype they define, and who their '
                  + 'rivals are. It costs one call and a few searches.'
                : 'No dossier stored, and the Claude stance is off — so '
                  + 'nothing here will call out. Everything else on this page '
                  + 'works unchanged.'}
            </p>
          )}
          {body && <DossierBodyView body={body} report={report} />}
        </div>
      )}
    </div>
  )
}

/** A passage plus the numbered citations it rests on. */
function Passage({ title, section, sources, extra }: {
  title: string
  section: DossierSection
  sources: DossierBody['sources']
  extra?: string
}) {
  if (!section.prose) return null
  return (
    <section className="max-w-3xl">
      <h3 className="text-xs font-medium uppercase tracking-wide"
          style={{ color: 'var(--text-muted)' }}>
        {title}
        {extra && (
          <span className="ml-2 normal-case tracking-normal"
                style={{ color: 'var(--text-primary)' }}>
            {extra}
          </span>
        )}
      </h3>
      <p className="mt-1.5 text-sm leading-relaxed"
         style={{ color: 'var(--text-secondary)' }}>
        {section.prose}
        <Cites ids={section.source_ids} sources={sources} />
      </p>
    </section>
  )
}

/**
 * The citation markers, and the reason they are links rather than superscripts.
 *
 * A number that does nothing asks the reader to take the citation on trust,
 * which is the thing this feature is not allowed to do. Each one jumps to the
 * source below and carries the page's real title in its tooltip.
 */
function Cites({ ids, sources }: { ids: string[]; sources: DossierBody['sources'] }) {
  const shown = ids
    .map((id) => ({ id, at: sources.findIndex((s) => s.id === id) }))
    .filter((x) => x.at >= 0)
  if (!shown.length) return null
  return (
    <>
      {shown.map(({ id, at }) => (
        <a key={id} href={`#src-${id}`} title={sources[at].title}
           className="ml-1 align-super text-[10px] font-medium no-underline"
           style={{ color: 'var(--series-2)' }}>
          [{at + 1}]
        </a>
      ))}
    </>
  )
}

function DossierBodyView({ body, report }: {
  body: DossierBody
  report: DossierReport | null
}) {
  return (
    <div className="flex flex-col gap-5">
      <Passage title="Who they are" section={body.who} sources={body.sources} />
      <Passage title="The archetype" section={body.archetype}
               sources={body.sources} extra={body.archetype.name} />

      {body.rivals.length > 0 && (
        <section className="max-w-3xl">
          <h3 className="text-xs font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
            Rivals
          </h3>
          <div className="mt-2 flex flex-col gap-3">
            {body.rivals.map((rival) => (
              <div key={rival.name} className="flex items-start gap-3">
                {/* Hoverable, showing the real card. A reader who doubts a
                    sentence about a rival can check it without leaving. */}
                <CardHover card={{ name: rival.name, image: rival.image }}>
                  <span className="shrink-0 cursor-help">
                    {rival.art_crop
                      ? <img src={rival.art_crop} alt="" loading="lazy"
                             className="h-12 w-20 rounded object-cover" />
                      : <span className="flex h-12 w-20 items-center justify-center rounded text-[10px]"
                              style={{ border: '1px solid var(--hairline)',
                                       color: 'var(--text-muted)' }}>
                          no art
                        </span>}
                  </span>
                </CardHover>
                <div className="min-w-0">
                  <p className="flex flex-wrap items-center gap-x-2 text-sm font-medium">
                    {rival.name}
                    {rival.mana_cost && <ManaCost cost={rival.mana_cost} size={13} />}
                    {rival.legal_commander === false && (
                      <span className="text-[10px] uppercase"
                            style={{ color: 'var(--status-critical)' }}>
                        banned
                      </span>
                    )}
                  </p>
                  <p className="text-sm leading-relaxed"
                     style={{ color: 'var(--text-secondary)' }}>
                    {rival.prose}
                    <Cites ids={rival.source_ids} sources={body.sources} />
                  </p>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      <Passage title="Where they sit in Magic's history" section={body.standing}
               sources={body.sources} />

      <section className="max-w-3xl">
        <h3 className="text-xs font-medium uppercase tracking-wide"
            style={{ color: 'var(--text-muted)' }}>
          Sources — {body.searched} pages read, {body.sources.length} cited
        </h3>
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
        {(body.sources_dropped > 0 || body.rivals_dropped > 0) && (
          // Shown, not logged. A number that climbs means the model has
          // started inventing citations, and nobody checks what they cannot
          // see. Silence here means everything it cited, it had read.
          <p className="mt-2 text-xs" style={{ color: 'var(--text-muted)' }}>
            Discarded before you saw it: {body.sources_dropped} cited page(s)
            the search never returned, {body.rivals_dropped} rival(s) not in
            the corpus.
          </p>
        )}
        <p className="mt-2 text-xs leading-relaxed"
           style={{ color: 'var(--text-muted)' }}>
          {report?.never
            ?? 'This is Claude\'s writing over cited pages. The card facts '
               + 'above it are the corpus\'s.'}
        </p>
      </section>
    </div>
  )
}
