import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import {
  api, deckUrl, errorMessage, followJob, type Correction, type ImportResult,
  type IntakeSheet, type Job,
} from '../lib/api'
import { IntakeChoices } from '../components/intake'
import { runIntake } from '../lib/intake'
import CameraDoor from '../components/camera'
import {
  Badge, ErrorNote, ManaText, PageMasthead, Spinner, TextField,
} from '../components/ui'

/**
 * Meticulous Archive, Sam Burley, Murders at Karlov Manor (2024) — a reading
 * room with the lamps lit, ladders against the shelves and scholars already at
 * the tables: the place a list becomes part of a collection, which is what this
 * page does.
 *
 * It leaves the Mystical Archive cycle the other mastheads share (see
 * `CardSearch.tsx` for why that cycle) and the reason is Aaron's, on
 * 2026-08-28: Magic has painted a great many libraries and this site is named
 * for one, so the room where decks arrive should be a library rather than a
 * spell about searching. Sylvan Library itself was not available to it — the
 * shelf and Learn already wear two different printings of it, and a third
 * would read as the same page twice.
 *
 * `PageMasthead` carries the hotlink-and-credit rules; the credit below is
 * commandment 19's half of them and is not optional.
 */
const METICULOUS_ARCHIVE_ART =
  'https://cards.scryfall.io/art_crop/front/6/5/652236c2-84ef-45e4-b5fc-ed6170bc3d6c.jpg'

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

/**
 * What the intake is doing, while the import waits on it.
 *
 * The percentage is the job's own -- five steps, one tick each -- and the
 * label says which action rather than "working", because the actions take
 * wildly different times and somebody watching a bar that has sat at 40% for
 * two minutes deserves to know it is the one that reads the whole web.
 */
function IntakeProgress({ job }: { job: Job }) {
  const pct = Math.max(0, Math.min(100, Math.round(job.percent)))
  return (
    <div className="space-y-2 rounded-lg px-4 py-3"
         style={{ background: 'var(--gridline)' }}>
      <p className="text-sm" style={{ color: 'var(--text-primary)' }}>
        Settling the deck in… {job.done} of {job.total} done.
      </p>
      <div className="h-1.5 w-full overflow-hidden rounded-full"
           style={{ background: 'var(--surface-1)' }}
           role="progressbar" aria-valuenow={pct} aria-valuemin={0}
           aria-valuemax={100} aria-label="Intake progress">
        <div className="h-full rounded-full transition-[width] duration-300"
             style={{ width: `${pct}%`, background: 'var(--series-1)' }} />
      </div>
      <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
        Your deck is already saved. This is the extra work you asked for, and
        the deck page opens as soon as it is finished.
      </p>
    </div>
  )
}

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
  // The intake sheet (ADR 41), empty by default and per import: a sheet that
  // remembered its last state would be a standing permission rather than an
  // answer about this deck.
  const [sheet, setSheet] = useState<IntakeSheet>({})
  // The stance the sheet asked the dial with, handed up by the sheet itself.
  //
  // **One value has to decide both halves of ADR 41's second gate.** The sheet
  // shows the drafting toggle because the dial said this stance may write; the
  // server refuses the drafting unless the stance it is *sent* says the same.
  // This page used to send none, so the server resolved the deck's own default
  // — `consultant`, which writes nothing — and refused work the sheet had just
  // offered. Reading the pin again here would be this page answering a
  // question the sheet has already asked, which is the same bug one refactor
  // later; the sheet reports what it asked with and this carries it through.
  const [sheetStance, setSheetStance] = useState<string | undefined>(undefined)
  const [intake, setIntake] = useState<Job | null>(null)

  // Typing a name fills the slug, until the slug is edited by hand.
  const [slugTouched, setSlugTouched] = useState(false)
  const effectiveSlug = slugTouched ? slug : slugify(name)

  function body(dryRun: boolean, override?: string) {
    return {
      slug: effectiveSlug,
      // `override` rather than the state, and only where one is passed: a
      // correction sets the text and previews in the same handler, and a
      // `setState` is not visible to the call that follows it.
      text: override ?? text,
      name: name.trim(),
      // Sent whole, and NOT split on commas.
      //
      // It used to be split here, on the reasoning that a partner pair is two
      // commanders and two boxes for a rare case is worse than one. Both
      // halves of that were right and the conclusion was still wrong: a comma
      // is part of a legendary creature's name far more often than it
      // separates two of them, and every deck in this library is led by one
      // -- Arahbo, Roar of the World; Gyome, Master Chef; Tivit, Seller of
      // Secrets. Typing any of them into this box produced two commanders,
      // neither of them a card, and a deck that reported `unknown-card` twice
      // for the two halves of one legend.
      //
      // The client cannot tell the readings apart: it takes a card pool to
      // know whether "Arahbo, Roar of the World" is one card or two. So it
      // sends what was typed and `deckimport.commanderReading` decides by
      // looking both up -- a pairing still works, and now it works because
      // the parts are cards rather than because a comma was present.
      commander: commander.trim() ? [commander.trim()] : [],
      companion: companion.trim(),
      bracket: bracket ? Number(bracket) : null,
      status,
      dry_run: dryRun,
    }
  }

  /**
   * Accept one spelling, or every obvious one at once.
   *
   * The pasted list is the truth this page works from -- the preview is the
   * real import with `dry_run`, and the deck that gets written is built from
   * whatever is in the box. So a correction edits the box and previews again,
   * rather than being carried alongside as a promise to apply later. Nothing
   * about the resolution, the gate or the deck.yaml on screen is a
   * description of what would happen; it is what happened, and that stays
   * true only if there is one source for it.
   *
   * Rewritten by whole line rather than by substring, and that matters: a
   * decklist line is a quantity and a name, and a bare replace of "Cultivate"
   * inside "1 Cultivator Colossus" would quietly corrupt a card that was
   * already right. The written name has to be the whole of what follows the
   * count.
   */
  function applyFixes(fixes: { written: string; chosen: string }[]) {
    const by = new Map(fixes.map((f) => [f.written.toLowerCase(), f.chosen]))
    const next = text.split('\n').map((line) => {
      // The count, whatever separator followed it, and the rest of the line.
      const m = /^(\s*(?:\d+\s*[xX]?\s+|[xX]\s*\d+\s+)?)(.*?)(\s*)$/.exec(line)
      if (!m) return line
      const [, lead, name, trail] = m
      const chosen = by.get((name ?? '').toLowerCase())
      return chosen === undefined ? line : `${lead}${chosen}${trail}`
    }).join('\n')
    setText(next)
    // Preview again from the corrected list, so the count, the gate and the
    // YAML on screen are about what is now in the box.
    void runWith(next, true)
  }

  const run = (dryRun: boolean) => runWith(undefined, dryRun)

  async function runWith(override: string | undefined, dryRun: boolean) {
    setBusy(dryRun ? 'preview' : 'create')
    setError(null)
    try {
      const result = await api.importDeck(body(dryRun, override))
      setPreview(result)
      if (result.created) {
        // The intake runs HERE rather than on the deck page, and the deck
        // page is not opened until it is done. Navigating first would drop
        // somebody onto a deck that is about to rewrite itself underneath
        // them — ninety-nine rationales appearing one reload at a time, with
        // nothing on screen saying why.
        const started = await runIntake(
          { owner: result.owner, slug: result.slug }, sheet, sheetStance)
        if (started) {
          setIntake(started)
          await followJob(started.id, setIntake, 400, started).promise
        }
        // `result.owner` rather than an assumption about whose library it
        // landed in: the server chooses the tier, and the deck's address
        // needs the owner segment (ADR 22).
        navigate(deckUrl(result))
      }
    } catch (e) {
      setError(errorMessage(e))
      if (!dryRun) setPreview(null)
    } finally {
      setBusy(null)
    }
  }

  const lines = text.split('\n').filter((l) => l.trim()).length
  const ready = text.trim().length > 0 && effectiveSlug.length > 0

  return (
    <div className="space-y-6">
      <PageMasthead
        art={METICULOUS_ARCHIVE_ART}
        alt="Meticulous Archive, painted by Sam Burley: a vast vaulted reading
             room, lamps burning along its length, ladders leaning against the
             shelves and scholars bent over their tables."
        title="Import a decklist"
        credit={<>
          <em>Meticulous Archive</em> by Sam Burley, Murders at Karlov Manor —
          the room a list walks into.
        </>}>
        <p className="max-w-2xl">
          Paste an export from Moxfield, Archidekt, Arena or anywhere else.
          Names are resolved against the local pool — anything that does not
          resolve is reported, never guessed. Add a quoted reason to any line
          and it becomes that card&rsquo;s <code>why</code>.{' '}
          <Link to="/" className="underline" style={{ color: 'var(--series-1)' }}>
            Back to the library
          </Link>
        </p>
      </PageMasthead>

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
              placeholder={'1 Arahbo, Roar of the World (C17) 27 *CMDR*\n'
                + '1 Sol Ring "fast mana, and it never gets cut"\n'
                + '36 Forest'}
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

          {/* The house extension, taught by showing it rather than by
              describing it. Every export this page reads has a column for the
              printing and none for the reasoning, which is the one thing a
              deck file here requires — so the reason goes in quotes at the end
              of the line, and lands in that card's `why` verbatim.

              Always open rather than behind a disclosure: it is the only place
              the format is written down, and somebody who does not know it
              exists will never press a control to find out (commandment 2). */}
          <div className="space-y-2 rounded-lg px-4 py-3"
               style={{ background: 'var(--gridline)',
                        color: 'var(--text-secondary)' }}>
            <p className="text-xs leading-relaxed">
              <strong style={{ color: 'var(--text-primary)' }}>
                Say why a card is in the deck, while you paste it.
              </strong>{' '}
              Put the reason in quotes at the end of any line and it becomes
              that card&rsquo;s <code>why</code> — your words, exactly as you
              wrote them. Every other column stays as your export wrote it,
              so a list with no quotes at all still reads perfectly.
            </p>
            <pre className="overflow-x-auto rounded-md p-3 font-mono text-[11px] leading-relaxed"
                 style={{ background: 'var(--surface-1)',
                          border: '1px solid var(--hairline)',
                          color: 'var(--text-muted)' }}>
1 Acidic Slime (ZNC) 59{' '}
              <span style={{ color: 'var(--series-1)' }}>
                &quot;Deathtouch body that kills artifacts too&quot;
              </span>
            </pre>
            <p className="text-xs leading-relaxed">
              Cards you leave unquoted are counted, never invented — nothing
              here writes a reason on your behalf.
            </p>
          </div>

          {/* The deck that exists nowhere online. Every other import path
              here is text, so a deck that lives in a box on a table has
              nothing to paste from — and that is exactly the deck a
              first-time player owns (commandment 2). The camera is the door
              for it; the apps below stay named because ours reads one card
              at a time and a whole 99 is faster through theirs. */}
          <div className="space-y-3 rounded-lg px-4 py-3"
               style={{ background: 'var(--gridline)',
                        color: 'var(--text-secondary)' }}>
            <p className="text-xs leading-relaxed">
              <strong style={{ color: 'var(--text-primary)' }}>
                Only have the cards?
              </strong>{' '}
              A deck sleeved up on the table has no list to copy. Photograph
              them one at a time and the pool will name them — a card's set
              code and collector number are enough to look it up exactly, and
              when the corner will not read, you choose from the closest
              names in the pool.
            </p>

            <CameraDoor onCards={(lines) => {
              // Appended to the box rather than imported: what the camera
              // produces is a decklist like any other, and it goes through
              // the same preview, the same gate and the same draft (ADR 13).
              setText((current) => {
                const before = current.trimEnd()
                return `${before ? `${before}\n` : ''}${lines.join('\n')}\n`
              })
            }} />

            <p className="text-xs leading-relaxed">
              For a whole deck at once, a free scanner app is quicker, and
              every one of them exports a format this page already reads —{' '}
              <em>Dragon Shield MTG Scanner</em> writes plain text and asks
              nothing of you, <em>ManaBox</em> writes the Arena format, and{' '}
              <em>Delver Lens</em> is the most accurate on Android, though
              its free export stops at 100 cards a session: one Commander
              deck exactly.
            </p>
          </div>
        </section>

        <section className="space-y-3">
          <TextField label="Deck name" value={name} onChange={setName}
                     placeholder="Arahbo — Cats" />
          <TextField label="Slug" value={effectiveSlug}
                     onChange={(v) => { setSlugTouched(true); setSlug(v) }}
                     placeholder="arahbo-cats" />
          <TextField label="Commander" value={commander} onChange={setCommander}
                     placeholder="blank uses the list's own; a pair is A + B" />
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
              className="btn btn-quiet"
            >
              {busy === 'preview' ? 'Resolving…' : 'Preview'}
            </button>
            <button
              onClick={() => run(false)}
              disabled={!ready || busy !== null}
              className="btn btn-primary btn-accent-1"
            >
              {busy === 'create' ? 'Importing…' : 'Import as draft'}
            </button>
          </div>
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            Preview runs the same resolution and the same gate, and writes
            nothing.
          </p>

          {/* The intake sheet (ADR 41). Under the buttons rather than above
              them: the deck lands whatever is ticked here, and a row of
              optional extras between somebody and the button they came for
              would read as a form standing in the way. */}
          <div className="space-y-3 rounded-lg px-4 py-3"
               style={{ background: 'var(--gridline)',
                        color: 'var(--text-secondary)' }}>
            <p className="text-xs leading-relaxed">
              <strong style={{ color: 'var(--text-primary)' }}>
                Want a hand with it as it lands?
              </strong>{' '}
              All of these are optional and off unless you turn them on, and
              your deck arrives exactly as you pasted it either way.
            </p>
            {/* **No slug, deliberately.** This screen is about a deck that
                does not exist yet, and `effectiveSlug` is what the deck WILL
                be called — asking the dial about it 404s, which this
                component then rendered as "Claude is turned off for this
                deck". The `intake` surface exists precisely so the dial can
                answer without a deck; passing a name that is not one yet
                undoes that. */}
            {/* `onStance` is a `useState` setter, so its identity is stable —
                the sheet holds it in a ref rather than in the dependency list
                that drives the dial, but a stable callback is the right thing
                to hand it regardless. */}
            <IntakeChoices value={sheet} onChange={setSheet}
                           onStance={setSheetStance} />
          </div>
        </section>
      </div>

      {/* What the intake is doing, while the page waits for it. A job with a
          label and a percentage rather than a bare spinner: five actions over
          ninety-nine cards is minutes, and a spinner that long reads as a
          page that has died. */}
      {intake && intake.status !== 'done' && (
        <IntakeProgress job={intake} />
      )}

      {error && <ErrorNote>{error}</ErrorNote>}
      {busy === 'preview' && !preview && <Spinner label="Resolving names…" />}
      {preview && !preview.created && <Preview result={preview}
                                               showYaml={showYaml}
                                               onToggleYaml={() => setShowYaml((v) => !v)}
                                               onFix={applyFixes} />}
    </div>
  )
}

/**
 * The names that did not resolve, and what the pool thinks they nearly are.
 *
 * Aaron's complaint was that the import is "way too strict on spelling cards
 * in a printed list". It is, and deliberately: `deckimport`'s first rule is
 * that **nothing is guessed** -- a name the pool does not know is kept exactly
 * as written so the list stays the size it was pasted, and it is reported
 * rather than silently dropped, because a quietly-shortened deck is the worst
 * thing this page could do. That rule is not the problem. The problem was
 * that being told "Sol Rng is not a card" and nothing else leaves the whole
 * job to the person, one name at a time, in a textarea.
 *
 * So the strictness stays and a shortlist arrives beside it. The server
 * measures the similarity and never applies it; pressing a name rewrites the
 * pasted list, which re-runs the same preview against the same gate. What you
 * approve is still exactly what gets written -- the list in the box is the
 * truth, and this edits the box.
 *
 * The distinction the copy has to make, and the reason none of this is
 * automatic: **a miss is not always a misspelling.** A card spoiled since the
 * last pool refresh is absent from a pool that is otherwise perfect, and the
 * nearest name to it will be a real card that is not it. Nobody but the
 * person holding the list can tell those apart.
 */
/**
 * The misspellings that were read, and what they were read as.
 *
 * Aaron's ruling, 2026-08-24: "we should do the fuzzy matching on the
 * backend, not allow misspelled things in." The deck already holds the real
 * card by the time this renders -- `deckimport.Respell` installs the record
 * under the written name before `BuildDeck` ever runs, so the count, the
 * category, the colour identity and the gate are all about the actual card.
 *
 * Which is the argument for doing it this way rather than offering a
 * shortlist: a misspelled `Rhystic Studdy` used to be an `unknown-card` and
 * nothing else. Read as Rhystic Study, it is a blue card, and a Selesnya deck
 * holding it now fails colour identity -- a real problem the old behaviour
 * hid behind a spelling complaint.
 *
 * So this is the saying-so, and it is deliberately loud and deliberately
 * above the errors. A correction changed what the deck contains, and the one
 * thing that would make it wrong to do at all is doing it quietly.
 */
function NamesRead({ read }: { read: Correction[] }) {
  return (
    <div className="rounded-lg px-4 py-3 text-sm"
         style={{
           background: 'color-mix(in srgb, var(--series-1) 10%, transparent)',
           borderLeft: '2px solid var(--series-1)',
         }}>
      <strong style={{ color: 'var(--text-primary)' }}>
        {read.length} name{read.length === 1 ? ' was' : 's were'} read as the
        card {read.length === 1 ? 'it is' : 'they are'} nearest to.
      </strong>{' '}
      <span style={{ color: 'var(--text-secondary)' }}>
        The deck below holds the real card — its cost, its colours and its
        legality are all the real card’s. If one of these is not what you
        meant, correct it in the list above and preview again.
      </span>
      <ul className="mt-2 space-y-0.5 text-xs">
        {read.map((c) => (
          <li key={c.written} className="flex flex-wrap items-baseline gap-2">
            <span className="font-mono" style={{ color: 'var(--text-muted)' }}>
              {c.written}
            </span>
            <span aria-hidden style={{ color: 'var(--text-muted)' }}>→</span>
            <span style={{ color: 'var(--text-primary)' }}>{c.read}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function UnknownNames({ result, onFix }: {
  result: ImportResult
  onFix: (fixes: { written: string; chosen: string }[]) => void
}) {
  const shortlists = result.did_you_mean ?? []
  const byName = new Map(shortlists.map((s) => [s.written, s]))
  const bare = result.unknown.filter((n) => !byName.has(n))

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <h3 className="text-sm font-semibold">
          {result.unknown.length} name{result.unknown.length === 1 ? '' : 's'} the
          pool does not know
        </h3>
      </div>
      <p className="max-w-2xl text-xs" style={{ color: 'var(--text-muted)' }}>
        These are the ones no single card was clearly enough — the obvious
        misspellings have already been read above. Each is kept exactly as
        written, so the deck stays the size you pasted, and nothing below has
        been applied: pressing a name rewrites it in the list above and
        previews again. Bear in mind that a name the pool does not know is not
        always a misspelling — a card printed since this pool was last
        refreshed is missing from it, and the closest match to that card’s
        name will be a different card.
      </p>
      <ul className="space-y-1.5">
        {shortlists.map((s) => (
          <li key={s.written} className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs"
                  style={{ color: 'var(--status-warning)' }}>
              {s.written}
            </span>
            <span aria-hidden style={{ color: 'var(--text-muted)' }}>→</span>
            {s.candidates.map((c) => (
              <button key={c.name} type="button"
                      onClick={() => onFix([{ written: s.written, chosen: c.name }])}
                      title={`Rewrite “${s.written}” as “${c.name}” in the list above`}
                      className="chip-toggle text-xs">
                {c.name}
              </button>
            ))}
          </li>
        ))}
        {bare.map((n) => (
          <li key={n} className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-xs"
                  style={{ color: 'var(--status-warning)' }}>{n}</span>
            <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
              nothing in the pool is close to this one
            </span>
          </li>
        ))}
      </ul>
      {result.did_you_mean_skipped > 0 && (
        <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
          {result.did_you_mean_skipped} more went unchecked — a list with this
          many unknown names is usually a pool that needs refreshing rather
          than {result.did_you_mean_skipped} typos.
        </p>
      )}
    </div>
  )
}

function Preview({ result, showYaml, onToggleYaml, onFix }: {
  result: ImportResult
  showYaml: boolean
  onToggleYaml: () => void
  /** Accept a spelling: `written` becomes `chosen` in the pasted list. */
  onFix: (fixes: { written: string; chosen: string }[]) => void
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

      {result.read.length > 0 && <NamesRead read={result.read} />}

      {result.unknown.length > 0 && (
        <UnknownNames result={result} onFix={onFix} />
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

      {/* What is owed, and — the half that was missing until the quoted column
          existed — what arrived. A person who wrote sixty reasons into their
          paste and is answered with "39 cards will need a why" has been told
          only the discouraging half of a good outcome (commandment 2). */}
      <div className="rounded-lg px-4 py-3 text-sm"
           style={{ background: 'var(--gridline)', color: 'var(--text-secondary)' }}>
        {result.rationales > 0 && (
          <>
            <strong style={{ color: 'var(--series-1)' }}>
              {result.rationales} card{result.rationales === 1 ? '' : 's'} arrived
              with your reason already written.
            </strong>{' '}
          </>
        )}
        {result.needs_rationale > 0 ? (
          <>
            <strong style={{ color: 'var(--text-primary)' }}>
              {result.needs_rationale} card{result.needs_rationale === 1 ? '' : 's'}{' '}
              still need{result.needs_rationale === 1 ? 's' : ''} a <code>why</code>.
            </strong>{' '}
            An imported deck is a draft: its legality, colour identity and size
            are checked immediately, and the reasoning is counted rather than
            invented. Write the rest, promote it to curated, and the five
            artifacts unlock.
          </>
        ) : (
          <>
            <strong style={{ color: 'var(--text-primary)' }}>
              Nothing is owed.
            </strong>{' '}
            It still lands as a draft, so its legality, colour identity and size
            are reported rather than refused — promote it to curated from the
            deck page whenever you are happy with it, and the five artifacts
            unlock.
          </>
        )}
      </div>

      <button onClick={onToggleYaml} aria-expanded={showYaml}
              className="btn btn-ghost btn-ghost-accent btn-xs">
        {showYaml ? 'Hide' : 'Show'} the file this writes
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
