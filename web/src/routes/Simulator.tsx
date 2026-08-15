import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  api, errorMessage, followJob, type DeckTile, type Job, type LandResult,
  type ManaResult,
} from '../lib/api'
import { percent } from '../lib/mtg'
import {
  Badge, Caveat, ErrorNote, NumberField, PageMasthead, Select, Spinner,
  StatTile,
} from '../components/ui'
import {
  ByTurnChart, CommanderCurve, DataTable, LandSweepChart, LandTradeoffChart,
  WastedManaChart,
} from '../components/charts'
import { HelpTip, Term } from '../components/term'

type Mode = 'mana' | 'lands'

/**
 * Every control and every figure on this screen, keyed to `glossary.py`.
 *
 * This screen was the clearest case for the glossary existing: its parameters
 * are words and numbers with no meaning attached — "Min mana pieces" is three
 * words that name a rule inside `KeepRule` and say nothing about it, and
 * "Deployment spread" is a number nobody can act on without being told what
 * flat means.
 *
 * The keys are pinned from the Python side by `SIMULATOR_KEYS` in
 * `tests/test_glossary.py`, because TypeScript cannot check a string against a
 * table in another language and a renamed key would otherwise just stop
 * opening.
 */
const help = (key: string) => <HelpTip name={key} />

/**
 * Strategic Planning, Robbie Trevino, Strixhaven Mystical Archive (2021) —
 * look at the top three, keep the one the plan needs, which is a goldfish run
 * in two sentences of rules text. Part of the Mystical Archive cycle the page
 * mastheads share; see `CardSearch.tsx` for why that cycle, and `PageMasthead`
 * for the hotlink-and-credit rules.
 */
const STRATEGIC_PLANNING_ART =
  'https://cards.scryfall.io/art_crop/front/f/d/fd7b2ee8-9f2f-4624-a788-15b88e6a2d50.jpg'

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
      <Badge>seed {seed}</Badge>
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
  const [error, setError] = useState<string | null>(null)
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

  async function run(withSeed = seed) {
    if (!slug) return
    cancelRef.current?.()
    setError(null)
    setMana(null)
    setLands(null)
    try {
      const payload = {
        slug, owner, games, min_lands: minLands, max_lands: maxLands,
        min_pieces: minPieces, seed: withSeed,
      }
      const submitted =
        mode === 'mana'
          ? await api.simMana({ ...payload, turns: 12 })
          : await api.simLands({ ...payload, low, high, games: Math.min(games, 25000) })
      setJob(submitted)
      // The submitted job is handed on: results are cached server-side, so it
      // can already be `done` and there is nothing to poll for.
      const follower = followJob(submitted.id, setJob, undefined, submitted)
      cancelRef.current = follower.cancel
      const finished = await follower.promise
      if (mode === 'mana') setMana(finished.result as ManaResult)
      else setLands(finished.result as LandResult)
    } catch (e) {
      setError(errorMessage(e))
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

  const running = job?.status === 'queued' || job?.status === 'running'

  return (
    <div className="space-y-6">
      <PageMasthead
        art={STRATEGIC_PLANNING_ART}
        alt="Strategic Planning, painted by Robbie Trevino: open hands over
             three glowing cards laid in a triangle, sight-lines drawn between
             them toward an eye at the apex."
        title="Simulator"
        credit={<>
          <em>Strategic Planning</em> by Robbie Trevino, Strixhaven Mystical
          Archive — look at the top three, keep what the plan needs.
        </>}>
        <p>
          <Term name="tier-1">Tier 1</Term> Monte Carlo: shuffle, draw, pay
          costs, repeat. It is a <Term name="goldfish">goldfish</Term> — nobody
          is playing against you — so it answers questions about mana and no
          others.
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
                  label: d.writable ? d.name : `${d.name} — ${d.owner}`,
                }))} />
        <Select label="Simulation" value={mode}
                onChange={(v) => setMode(v as Mode)}
                options={[
                  { value: 'mana', label: 'Mana & consistency' },
                  { value: 'lands', label: 'Land count sweep' },
                ]} />
        <NumberField label="Games" value={games} onChange={setGames}
                     min={100} max={200000} step={1000} help={help('sim.games')} />
        {mode === 'lands' ? (
          <>
            <NumberField label="From lands" value={low} onChange={setLow} min={20} max={59}
                         help={help('sim.land_range')} />
            <NumberField label="To lands" value={high} onChange={setHigh} min={21} max={60}
                         help={help('sim.land_range')} />
          </>
        ) : (
          <>
            <NumberField label="Keep min lands" value={minLands} onChange={setMinLands}
                         min={0} max={7} help={help('sim.min_lands')} />
            <NumberField label="Keep max lands" value={maxLands} onChange={setMaxLands}
                         min={1} max={7} help={help('sim.max_lands')} />
            <NumberField label="Min mana pieces" value={minPieces} onChange={setMinPieces}
                         min={0} max={7} help={help('sim.min_pieces')} />
          </>
        )}
        <NumberField label="Seed" value={seed} onChange={setSeed}
                     min={1} max={999999} help={help('sim.seed')} />
        <button onClick={() => run()} disabled={running || !slug}
                className="h-9 rounded-lg px-4 text-sm font-medium disabled:opacity-50"
                style={{ background: 'var(--series-1)', color: '#fff' }}>
          {running ? 'Running…' : 'Run simulation'}
        </button>
        <button onClick={resample} disabled={running || !slug}
                title="Run the same deck against a different shuffle"
                className="h-9 rounded-lg border px-4 text-sm font-medium disabled:opacity-50"
                style={{ borderColor: 'var(--gridline)' }}>
          New sample
        </button>
      </div>

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

      {mana && (
        <div className="space-y-6">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
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
                { key: 'commander_down', label: 'Commander', format: (v) => percent(v, 0) },
              ]}
              rows={mana.by_turn}
            />
          </section>

          <Provenance seed={mana.seed} cached={mana.cached}
                      computed_at={mana.computed_at} />
          <Caveat>{mana.caveat}</Caveat>
        </div>
      )}

      {lands && (
        <div className="space-y-6">
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
