import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
/**
 * The Opponent: a goldfish idling in a glass bowl across a candlelit card
 * table — the goldfish this page's own masthead copy has always named, given
 * its seat. An authored loop like the About Claude masthead (the séance
 * room's second cousin): Claude wrote the scene, Aaron conjured it, and
 * `PROVENANCE.md` beside the files holds the licence judgement and the exact
 * encode. This page left the Mystical Archive masthead cycle for it
 * deliberately — the cycle's story is in `CardSearch.tsx` — and the still is
 * the poster, the reduced-motion floor and the room's backdrop wash.
 */
import goldfishMp4 from '../assets/simulator/goldfish-loop.mp4'
import goldfishStill from '../assets/simulator/goldfish-still.webp'
import goldfishWebm from '../assets/simulator/goldfish-loop.webm'
import {
  api, errorMessage, followJob, type DeckTile, type Job,
  type LandResult, type ManaResult, type PolicyResult, type ShelfResult,
  type ValidationReport,
} from '../lib/api'
import { ReplayGlyph } from '../components/glyphs'
import { percent } from '../lib/mtg'
import {
  Badge, Caveat, ErrorNote, NumberField, PageMasthead, Select, Spinner,
  StatTile,
} from '../components/ui'
import {
  ByTurnChart, CommanderCurve, LandSweepChart, LandTradeoffChart, WastedManaChart,
} from '../components/lazycharts'
import { DataTable } from '../components/datatable'
import {
  ClosedForm, DeckCaution, DeckVerdict, ManaCurvePanel, PolicyReport,
} from '../components/closedform'
import { HelpTip, Term } from '../components/term'
import { Link } from 'react-router-dom'

/**
 * The four simulations this screen runs, and they are all arithmetic.
 *
 * **Real games are not here, deliberately** (Aaron's call, 2026-08-25). Tier 3
 * used to be a fifth option in this list, which made one screen answer two
 * unlike questions: "what does ten thousand shuffles say about my mana" is a
 * number you read, and "who wins" is a match you *watch*, for minutes. They
 * wanted different rooms and one of them wanted a stage. The Forge moved
 * whole to `/coliseum`, which was built to be where a match is watched; this
 * screen kept the goldfish and got shorter.
 */
type Mode = 'mana' | 'lands' | 'shelf' | 'policy'

/**
 * Every control and every figure on this screen, keyed to the served glossary.
 *
 * This screen was the clearest case for the glossary existing: its parameters
 * are words and numbers with no meaning attached — "Min mana pieces" is three
 * words that name a rule inside `KeepRule` and say nothing about it, and
 * "Deployment spread" is a number nobody can act on without being told what
 * flat means.
 *
 * The keys are string names into the glossary table the server owns.
 * TypeScript cannot check a string against a served table, so a renamed
 * glossary key would not fail anywhere — the popover would just stop
 * opening.
 */
const help = (key: string) => <HelpTip name={key} />


/** The seed a run gets unless you ask for another. Matches
 * `simruns.DEFAULT_SEED`, so the app and the CLI describe the same sample. */
const DEFAULT_SEED = 7

/** A seed nobody would pick twice. "New sample" is the deliberate act of
 * asking for a different draw; everything else is reproducible on purpose. */
const newSeed = () => Math.floor(Math.random() * 1_000_000) + 1

/** How old a cached result is, in words rather than an ISO timestamp. */
function ago(iso: string): string {
  const seconds = Math.max(0, (Date.now() - Date.parse(iso)) / 1000)
  if (!Number.isFinite(seconds)) return 'earlier'
  if (seconds < 90) return 'moments ago'
  if (seconds < 3600) return `${Math.round(seconds / 60)} min ago`
  if (seconds < 86400) return `${Math.round(seconds / 3600)} h ago`
  return `${Math.round(seconds / 86400)} d ago`
}

/** Where a number came from: this run, or a previous identical one.
 *
 * Shown rather than hidden. The results are cached on the deck's *compiled*
 * content, so a hit means nothing about the deck or the parameters changed —
 * but a figure that cannot say how old it is cannot be read honestly, which is
 * the same reason every Tier 1 result ships with its caveat.
 */
function Provenance({ seed, cached, computed_at }: {
  seed: number; cached: boolean; computed_at: string | null
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 text-xs"
         style={{ color: 'var(--text-secondary)' }}>
      <Badge>shuffle {seed}</Badge>
      {cached && computed_at
        ? <span>Cached — computed {ago(computed_at)}. Same deck, same
            parameters, same numbers.</span>
        : <span>Computed just now.</span>}
    </div>
  )
}

export default function Simulator() {
  const [params, setParams] = useSearchParams()
  const [decks, setDecks] = useState<DeckTile[]>([])
  const [slug, setSlug] = useState(params.get('deck') ?? '')
  // Whose deck, as its own parameter rather than folded into `deck` (ADR 22).
  // Two parameters, so a bookmark from before owners existed still names a
  // deck: absent, the server reads `owner` as the caller's own library, which
  // is what `?deck=goreclaw` always meant.
  const [owner, setOwner] = useState(params.get('owner') ?? '')
  const [mode, setMode] = useState<Mode>('mana')
  const [games, setGames] = useState(20000)
  const [minLands, setMinLands] = useState(2)
  const [maxLands, setMaxLands] = useState(5)
  const [minPieces, setMinPieces] = useState(3)
  const [low, setLow] = useState(30)
  const [high, setHigh] = useState(40)
  const [seed, setSeed] = useState(DEFAULT_SEED)

  const [job, setJob] = useState<Job | null>(null)
  const [mana, setMana] = useState<ManaResult | null>(null)
  const [lands, setLands] = useState<LandResult | null>(null)
  // Tier 1.5. The shelf is not a job -- it answers in the request -- so it has
  // no `Job` beside it and its own small in-flight flag instead.
  const [shelf, setShelf] = useState<ShelfResult | null>(null)
  const [shelfBusy, setShelfBusy] = useState(false)
  const [policy, setPolicy] = useState<PolicyResult | null>(null)
  // Its own control rather than sharing the mana run's
  // twenty thousand and silently clamping it. A field reading 20,000 above a
  // report saying "2,000 games each" is a screen contradicting itself, and
  // the honest fix is a control that cannot be set to a number the run will
  // not honour.
  const [policyGames, setPolicyGames] = useState(2000)
  const [target, setTarget] = useState(90)
  // The mana curve's two dials (Aaron's ruling, 2026-08-21). `targetMana` is
  // the one that makes the advice mean anything: at the turn number lands
  // always win, and above it they cannot help at all.
  const [targetTurn, setTargetTurn] = useState(4)
  const [targetMana, setTargetMana] = useState(4)
  const [error, setError] = useState<string | null>(null)
  /**
   * A chosen deck's own gate report, by address.
   *
   * Fetched only for a deck the shelf has *already* said has errors, so the
   * common case -- every deck valid -- costs nothing. `DeckTile.errors` is a
   * count and this is what the count is about, which is the difference
   * between "something is wrong with this deck" and "Sol Rng is not a card".
   */
  const [checks, setChecks] = useState<Record<string, ValidationReport>>({})
  /**
   * Submitted, and not yet a job (punch list item 6).
   *
   * `running` used to be `job.status`, which is only true once the POST has
   * come back. A button that stays live and unchanged for the length of a
   * round trip has nothing on screen saying the run began, and the honest
   * reading of that is that the click missed. This is the gap between the
   * press and the job.
   *
   * The gap was worst on the Forge, whose POST had a JVM starting behind it —
   * that mode lives in the Coliseum now and took its own word for the wait
   * with it. The gap is smaller here and it is still real.
   */
  const [submitting, setSubmitting] = useState(false)
  const cancelRef = useRef<null | (() => void)>(null)

  useEffect(() => {
    api.decks().then((d) => {
      setDecks(d)
      // `d[0]` rather than `d.length &&` -- the same test, but one that carries
      // the deck along with the answer instead of leaving the read to a second
      // lookup the checker has to trust.
      const first = d[0]
      if (!slug && first) {
        setSlug(first.slug)
        setOwner(first.owner)
      } else if (!owner && d.length) {
        // A bookmarked `?deck=` with no owner: resolve it the same way
        // `DeckRedirect` does, by taking the first library that has it. The
        // list is the caller's own decks first, then the showcase, then
        // everybody else's, so first-match is the precedence a person wants.
        setOwner(d.find((deck) => deck.slug === slug)?.owner ?? '')
      }
    }).catch((e) => setError(errorMessage(e)))
    // Cancel any in-flight poll when the screen unmounts.
    return () => cancelRef.current?.()
  }, [])                                     // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (slug) setParams(owner ? { deck: slug, owner } : { deck: slug },
                        { replace: true })
  }, [slug, owner, setParams])

  /** The shelf's own record of a deck in one of the two seats, or null while
   *  the shelf is still loading. Matched on owner as well as slug, because an
   *  address is both (ADR 22) and two libraries may hold the same slug — an
   *  empty owner is the pre-owner bookmark shape and matches on slug alone,
   *  exactly as the resolution above does. */
  const deckOf = (s: string, o: string) =>
    decks.find((d) => d.slug === s && (!o || d.owner === o)) ?? null

  /** A deck's address, which is owner and slug together (ADR 22). */
  const address = (s: string, o: string) => `${o}/${s}`

  // Ask the gate about the deck in the seat, and only when the shelf has
  // already said there is something to ask about. Cached by address, because
  // switching back and forth between two decks should not re-ask the same
  // question, and because a run does not change the answer.
  useEffect(() => {
    if (!slug) return
    let live = true
    const key = address(slug, owner)
    const tile = deckOf(slug, owner)
    // `errors` is null when the pool was unavailable and the gate never
    // ran, which is not a pass and must not render as one -- but it is
    // also not something this screen can diagnose, so it asks nothing.
    if (tile?.errors && !checks[key]) {
      api.validate({ owner: tile.owner, slug: tile.slug })
        .then((report) => { if (live) setChecks((c) => ({ ...c, [key]: report })) })
        .catch(() => undefined)   // a caution nobody can fetch is not an error
    }
    return () => { live = false }
  }, [slug, owner, decks, checks])   // eslint-disable-line react-hooks/exhaustive-deps

  async function run(withSeed = seed) {
    if (!slug) return
    cancelRef.current?.()
    setSubmitting(true)
    setError(null)
    // Cleared before the next run rather than after the last one, so a
    // second run reads as running instead of inheriting the finished job
    // that made the first one stop saying so.
    setJob(null)
    setMana(null)
    setLands(null)
    setShelf(null)
    setPolicy(null)
    const payload = {
      slug, owner, games, min_lands: minLands, max_lands: maxLands,
      min_pieces: minPieces, seed: withSeed,
    }

    // The closed form is the one mode with no job to follow: it is 40ms of
    // arithmetic and the route answers in the request. It gets its own branch
    // rather than a fake job, because pretending it is one would mean a
    // spinner, a poll and a job id for a call that has already returned.
    if (mode === 'shelf') {
      setShelfBusy(true)
      try {
        setShelf(await api.simShelf({
          slug, owner, target: target / 100,
          target_turn: targetTurn, target_mana: targetMana,
        }))
      } catch (e) {
        setError(errorMessage(e))
      } finally {
        setShelfBusy(false)
        setSubmitting(false)
      }
      return
    }

    try {
      const submitted =
        mode === 'mana'
          ? await api.simMana({ ...payload, turns: 12 })
          : mode === 'lands'
            ? await api.simLands({ ...payload, low, high, games: Math.min(games, 25000) })
            : await api.simPolicy({
                  slug, owner, seed: withSeed,
                  // Its own field, capped at 2,000: this multiplies by the
                  // size of the grid, so the mana run's 20,000 would be
                  // thirty-three simulations of 20,000 games. At 2,000 the
                  // sampling error on deployment is already well under the
                  // threshold the verdict is reported at, so a bigger sample
                  // would buy precision the answer throws away.
                  games: policyGames,
                })
      setJob(submitted)
      // The submitted job is handed on: results are cached server-side, so it
      // can already be `done` and there is nothing to poll for.
      const follower = followJob(submitted.id, setJob, undefined, submitted)
      cancelRef.current = follower.cancel
      const finished = await follower.promise
      if (mode === 'mana') setMana(finished.result as ManaResult)
      else if (mode === 'lands') setLands(finished.result as LandResult)
      else setPolicy(finished.result as PolicyResult)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setSubmitting(false)
    }
  }

  /** Re-run the same deck against a different shuffle.
   *
   * The seed is fixed by default so the numbers on a deck are reproducible and
   * cost nothing to look at twice. Wanting a second sample is legitimate and
   * cheap to ask for — it is just no longer what happens by accident. */
  function resample() {
    const next = newSeed()
    setSeed(next)
    void run(next)
  }

  const running = submitting || shelfBusy
    || job?.status === 'queued' || job?.status === 'running'

  return (
    <div className="space-y-6">
      <PageMasthead
        art={goldfishStill}
        video={{ webm: goldfishWebm, mp4: goldfishMp4 }}
        alt="A candlelit study table dressed in green felt: a hand of seven
             face-down cards fanned on the near felt, and across the table a
             goldfish idling in a glass bowl on a brass stand — the opponent,
             waiting. Hourglasses run on the shelves behind it."
        title="Simulator"
        credit={<>
          The opponent&rsquo;s seat, taken at last: the goldfish this page has
          always played against, waiting out your ten thousand shuffles. The
          scene is ours outright — Claude wrote it, Aaron conjured it — so for
          once there is no painter to name.
        </>}>
        <p>
          <Term name="tier-1">Tier 1</Term> Monte Carlo: shuffle, draw, pay
          costs, repeat. It is a <Term name="goldfish">goldfish</Term> — nobody
          is playing against you — so it answers questions about mana and no
          others. For an opponent who fights back,
          {' '}<Term name="tier-3">the Forge</Term> plays real games in the
          {' '}<Link to="/coliseum" className="underline decoration-dotted"
                     style={{ color: 'var(--series-1)' }}>Coliseum</Link>.
        </p>
      </PageMasthead>

      <div className="card-surface flex flex-wrap items-end gap-3 rounded-xl p-4">
        {/* The value is the deck's whole address, because a slug is unique
            per owner rather than globally now and two people may both have a
            `goreclaw` on this list. */}
        <Select label="Deck" value={owner ? `${owner}/${slug}` : slug}
                onChange={(v) => {
                  const cut = v.indexOf('/')
                  setOwner(cut < 0 ? '' : v.slice(0, cut))
                  setSlug(cut < 0 ? v : v.slice(cut + 1))
                }}
                options={decks.map((d) => ({
                  value: `${d.owner}/${d.slug}`,
                  // The pilot tag rides along (second punch list, item 10),
                  // so a household comparing decks can tell whose is whose
                  // without leaving the page.
                  label: (d.writable ? d.name : `${d.name} — ${d.owner}`)
                    + (d.pilot ? ` (${d.pilot})` : ''),
                }))} />
        <Select label="Simulation" value={mode}
                onChange={(v) => setMode(v as Mode)}
                options={[
                  { value: 'mana', label: 'Mana & consistency' },
                  { value: 'lands', label: 'Land count sweep' },
                  { value: 'shelf', label: 'The closed form' },
                  { value: 'policy', label: 'Mulligan policy search' },
                ]} />
        {mode === 'policy' ? (
          <NumberField label="Games per rule" value={policyGames}
                       onChange={setPolicyGames} min={200} max={2000} step={200}
                       help={help('sim.games')} />
        ) : (
          <NumberField label="Games" value={games} onChange={setGames}
                       min={100} max={200000} step={1000} help={help('sim.games')} />
        )}
        {mode === 'lands' && (
          <>
            <NumberField label="From lands" value={low} onChange={setLow} min={20} max={59}
                         help={help('sim.land_range')} />
            <NumberField label="To lands" value={high} onChange={setHigh} min={21} max={60}
                         help={help('sim.land_range')} />
          </>
        )}
        {mode === 'shelf' && (
          <>
            <NumberField label="Consistency %" value={target} onChange={setTarget}
                         min={50} max={99} help={help('sim.target')} />
            <NumberField label="Target turn" value={targetTurn}
                         onChange={setTargetTurn} min={1} max={10}
                         help={help('sim.target_turn')} />
            <NumberField label="Mana wanted" value={targetMana}
                         onChange={setTargetMana} min={1} max={12}
                         help={help('sim.target_mana')} />
          </>
        )}
        {mode === 'mana' && (
          <>
            <NumberField label="Keep min lands" value={minLands} onChange={setMinLands}
                         min={0} max={7} help={help('sim.min_lands')} />
            <NumberField label="Keep max lands" value={maxLands} onChange={setMaxLands}
                         min={1} max={7} help={help('sim.max_lands')} />
            <NumberField label="Min mana pieces" value={minPieces} onChange={setMinPieces}
                         min={0} max={7} help={help('sim.min_pieces')} />
          </>
        )}
        <NumberField label="Shuffle" value={seed} onChange={setSeed}
                     min={1} max={999999} help={help('sim.seed')} />
        {/* Pressed is a state the button has to be able to show, and until
            now it could not: `running` came from the job and the job comes
            from the POST, so a button clicked stayed live and unchanged until
            the round trip returned (punch list item 6). `submitting` closes
            that gap from the click rather than from the job. */}
        <button onClick={() => run()} disabled={running || !slug}
                className="btn btn-primary btn-accent-1">
          {running ? 'Running…' : 'Run simulation'}
        </button>
        <button onClick={resample} disabled={running || !slug}
                title="Run the same deck against a different shuffle"
                className="btn btn-quiet">
          <ReplayGlyph />
          New sample
        </button>
      </div>

      {/* What a run will leave out, before it is paid for. */}
      <DeckCaution report={checks[address(slug, owner)]}
                   name={deckOf(slug, owner)?.name ?? slug} />

      {error && <ErrorNote>Simulation failed: {error}</ErrorNote>}

      {running && job && (
        <div className="card-surface space-y-2 rounded-xl p-4">
          <div className="flex items-center justify-between text-sm">
            <Spinner label={job.label || 'Running…'} />
            <span className="tabular" style={{ color: 'var(--text-secondary)' }}>
              {job.percent}%
            </span>
          </div>
          <div className="h-2 overflow-hidden rounded-full" style={{ background: 'var(--gridline)' }}>
            <div className="h-full rounded-full transition-all"
                 style={{ width: `${job.percent}%`, background: 'var(--series-1)' }} />
          </div>
        </div>
      )}

      {/* Tier 1.5 renders as its own section rather than folded into the mana
          screen. The numbers answer a different question -- "would the mana be
          there", not "did you cast it" -- and stacking two true figures that
          answer different questions in one column is how a screen lies with
          correct numbers. The Karsten shelf's own doc carries the measurement. */}
      {shelf && (
        <section className="card-surface space-y-3 rounded-xl p-5">
          <DeckVerdict check={shelf.deck_check} />
          {shelf.mana_curve && <ManaCurvePanel curve={shelf.mana_curve} />}
          <ClosedForm shelf={shelf} />
          <Caveat>{shelf.caveat}</Caveat>
        </section>
      )}

      {policy && (
        <section className="card-surface space-y-3 rounded-xl p-5">
          <DeckVerdict check={policy.deck_check} />
          <PolicyReport policy={policy} />
          <Provenance seed={policy.seed} cached={policy.cached}
                      computed_at={policy.computed_at} />
          <Caveat>{policy.caveat}</Caveat>
        </section>
      )}

      {mana && (
        <div className="space-y-6">
          {/* The same rule as the closed form's: a number about an illegal
              deck is honest only when it says so, and Tier 1's figures are
              no more exempt than Tier 1.5's. */}
          <DeckVerdict check={mana.deck_check} />
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <StatTile label="Mulligan rate" value={percent(mana.mulligan_rate)}
                      hint={`avg ${mana.avg_mulligans.toFixed(2)} per game`}
                      help={help('stat.mulligan_rate')} />
            <StatTile label="Median commander turn"
                      value={mana.median_commander_turn?.toString() ?? '—'}
                      help={help('stat.median_commander_turn')} />
            <StatTile label="Never cast by T12"
                      value={percent(mana.never_cast_commander)}
                      tone={mana.never_cast_commander > 0.05 ? 'warning' : 'good'}
                      help={help('stat.never_cast_commander')} />
            <StatTile label="Color-only blocks"
                      value={mana.color_screw_rate.toFixed(2)} hint="turns per game"
                      help={help('stat.color_screw_rate')} />
            {/* The texture pair (second punch list, item 11): how the games
                actually feel — when the deck starts moving, and how often
                it just sits there. */}
            <StatTile label="First spell"
                      value={mana.median_first_spell_turn != null
                        ? `T${mana.median_first_spell_turn}` : '—'}
                      hint="median turn"
                      help={help('stat.median_first_spell')} />
            <StatTile label="Stalled turns"
                      value={mana.stalled_turns.toFixed(2)}
                      hint="castless with a spell in hand, per game"
                      tone={mana.stalled_turns > 2 ? 'warning' : undefined}
                      help={help('stat.stalled_turns')} />
          </div>

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="text-sm font-semibold">Commander on board</h3>
            <CommanderCurve rows={mana.by_turn} />
          </section>

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="text-sm font-semibold">Board development by turn</h3>
            <ByTurnChart rows={mana.by_turn} />
            <DataTable
              columns={[
                { key: 'turn', label: 'Turn' },
                { key: 'lands', label: 'Lands' },
                { key: 'mana', label: 'Mana' },
                { key: 'spells', label: 'Spells' },
                { key: 'unused', label: 'Wasted' },
                { key: 'missed_drop', label: 'No land', format: (v) => percent(v, 0) },
                { key: 'commander_down', label: 'Commander', format: (v) => percent(v, 0) },
              ]}
              rows={mana.by_turn}
            />
          </section>

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="flex items-center text-sm font-semibold">
              When each card comes online{help('stat.card_timing')}
            </h3>
            <Caveat>
              Worst first: never-cast cards lead, then the latest medians. Low
              cast rates are normal — a specific card in a 99 is only drawn in
              a minority of games — so read the gap between a card&rsquo;s
              cost and its median turn, not the rate alone.
            </Caveat>
            <div className="max-h-96 overflow-y-auto">
              <DataTable
                columns={[
                  { key: 'name', label: 'Card' },
                  { key: 'mv', label: 'MV' },
                  { key: 'median_turn', label: 'Median turn',
                    format: (v) => (v == null ? 'never' : `T${v}`) },
                  { key: 'cast_rate', label: 'Cast in', format: (v) => percent(v, 0) },
                  { key: 'by_t8', label: 'Down by T8', format: (v) => percent(v, 0) },
                ]}
                rows={mana.card_timings}
              />
            </div>
          </section>

          <Provenance seed={mana.seed} cached={mana.cached}
                      computed_at={mana.computed_at} />
          <Caveat>{mana.caveat}</Caveat>
        </div>
      )}

      {lands && (
        <div className="space-y-6">
          <DeckVerdict check={lands.deck_check} />
          <div className="grid gap-3 sm:grid-cols-3">
            <StatTile label="Deployment spread"
                      value={lands.deployment_spread.toFixed(2)}
                      hint="spells through T8, best minus worst"
                      tone={lands.flat ? 'warning' : undefined}
                      help={help('stat.deployment_spread')} />
            <StatTile label="Highest deployment" value={`${lands.argmax_lands} lands`}
                      help={help('stat.argmax_lands')} />
            <StatTile label="Games per count" value={lands.games.toLocaleString()}
                      help={help('sim.games')} />
          </div>

          {lands.flat && (
            <div className="rounded-lg px-4 py-3 text-sm"
                 style={{
                   background: 'color-mix(in srgb, var(--status-warning) 12%, transparent)',
                   border: '1px solid color-mix(in srgb, var(--status-warning) 35%, transparent)',
                 }}>
              <Badge tone="warning">flat</Badge>{' '}
              Deployment varies by only {lands.deployment_spread.toFixed(2)} spells
              across this whole range, which is within noise. There is no peak to
              pick here — {lands.argmax_lands} is the arithmetic maximum, not a
              meaningful recommendation. Decide on the measures that do move:
              mulligan rate and wasted mana, below.
            </div>
          )}

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="flex items-center text-sm font-semibold">
              Spells deployed through T8{help('stat.spells_through_t8')}
            </h3>
            <Caveat>
              The decision metric. The shaded band spans the full range of results —
              if the line stays inside it, the differences are noise.
            </Caveat>
            <LandSweepChart rows={lands.rows} flat={lands.flat} />
          </section>

          <div className="grid gap-5 lg:grid-cols-2">
            <section className="card-surface space-y-2 rounded-xl p-5">
              <h3 className="text-sm font-semibold">What you buy with more lands</h3>
              <LandTradeoffChart rows={lands.rows} />
            </section>
            <section className="card-surface space-y-2 rounded-xl p-5">
              <h3 className="flex items-center text-sm font-semibold">
                What it costs you{help('stat.wasted_through_t8')}
              </h3>
              <WastedManaChart rows={lands.rows} />
            </section>
          </div>

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="text-sm font-semibold">All results</h3>
            <DataTable
              columns={[
                { key: 'lands', label: 'Lands' },
                { key: 'commander_by_t5', label: 'Commander by T5', format: (v) => percent(v) },
                { key: 'spells_through_t8', label: 'Spells thru T8' },
                { key: 'wasted_through_t8', label: 'Wasted thru T8' },
                { key: 'mulligan_rate', label: 'Mulligan rate', format: (v) => percent(v) },
              ]}
              rows={lands.rows}
            />
          </section>

          <Provenance seed={lands.seed} cached={lands.cached}
                      computed_at={lands.computed_at} />
          <Caveat>{lands.caveat}</Caveat>
        </div>
      )}

    </div>
  )
}
