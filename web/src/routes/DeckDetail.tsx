import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  api,
  type Card,
  type DeckDetail as Deck,
  type DeckStats,
  type ValidationReport,
} from '../lib/api'
import { categoryLabel, identityName, percent } from '../lib/mtg'
import {
  Badge, CardArt, CardHover, Caveat, ColorPips, ErrorNote, ManaCost, Select,
  Spinner, StatTile,
} from '../components/ui'
import {
  CategoryCoverage, ColorNeedsChart, CurveChart, DataTable,
} from '../components/charts'

type Tab = 'cards' | 'stats' | 'validation' | 'notes'

export default function DeckDetail() {
  const { slug = '' } = useParams()
  const [deck, setDeck] = useState<Deck | null>(null)
  const [stats, setStats] = useState<DeckStats | null>(null)
  const [report, setReport] = useState<ValidationReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('cards')
  const [groupBy, setGroupBy] = useState('category')

  useEffect(() => {
    setDeck(null)
    setError(null)
    Promise.all([api.deck(slug), api.stats(slug), api.validate(slug)])
      .then(([d, s, v]) => {
        setDeck(d)
        setStats(s)
        setReport(v)
      })
      .catch((e) => setError(String(e.message ?? e)))
  }, [slug])

  const groups = useMemo(() => {
    if (!deck) return []
    const out = new Map<string, Card[]>()
    for (const card of deck.cards) {
      const key =
        groupBy === 'type'
          ? (card.type_line?.split(' // ')[0].split('—')[0].trim().split(' ').pop() ?? 'Unknown')
          : groupBy === 'mv'
            ? `MV ${card.cmc ?? 0}`
            : card.category
      if (!out.has(key)) out.set(key, [])
      out.get(key)!.push(card)
    }
    return [...out.entries()].sort((a, b) => b[1].length - a[1].length)
  }, [deck, groupBy])

  if (error) {
    return (
      <div className="space-y-4">
        <ErrorNote>{error}</ErrorNote>
        <Link to="/" className="text-sm underline" style={{ color: 'var(--series-1)' }}>
          ← Back to the library
        </Link>
      </div>
    )
  }
  if (!deck || !stats || !report) return <Spinner label="Loading deck…" />

  const tabs: { id: Tab; label: string }[] = [
    { id: 'cards', label: `The ${deck.total_cards}` },
    { id: 'stats', label: 'Stats' },
    { id: 'validation', label: 'Validation' },
    { id: 'notes', label: 'Notes' },
  ]

  return (
    <div className="space-y-6">
      {/* hero */}
      <div className="card-surface overflow-hidden rounded-xl">
        <div className="relative">
          <CardArt src={deck.commander_card?.art_crop} alt={deck.commander[0] ?? ''}
                   ratio="aspect-[1200/260]" className="rounded-none" />
        </div>
        <div className="flex flex-wrap items-end justify-between gap-4 p-5">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-semibold tracking-tight">{deck.name}</h1>
              {deck.bracket && <Badge>Bracket {deck.bracket}</Badge>}
              {report.ok
                ? <Badge tone="good">valid</Badge>
                : <Badge tone="critical">{report.errors.length} error(s)</Badge>}
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-sm"
                 style={{ color: 'var(--text-secondary)' }}>
              <ColorPips identity={deck.color_identity} />
              <span>{identityName(deck.color_identity)}</span>
              <span aria-hidden>·</span>
              <span>{deck.commander.join(', ')}</span>
              {deck.companion && (
                <>
                  <span aria-hidden>·</span>
                  <span>companion: {deck.companion}</span>
                </>
              )}
            </div>
          </div>
          <Link to={`/simulate?deck=${deck.slug}`}
                className="rounded-lg px-4 py-2 text-sm font-medium"
                style={{ background: 'var(--series-1)', color: '#fff' }}>
            Simulate this deck
          </Link>
        </div>
      </div>

      {deck.strategy && (
        <p className="max-w-3xl text-sm leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          {deck.strategy}
        </p>
      )}

      {/* tabs */}
      <div className="flex gap-1 border-b" style={{ borderColor: 'var(--hairline)' }}>
        {tabs.map((t) => (
          <button key={t.id} onClick={() => setTab(t.id)}
                  className="px-3 py-2 text-sm font-medium transition"
                  style={{
                    color: tab === t.id ? 'var(--text-primary)' : 'var(--text-muted)',
                    borderBottom: `2px solid ${tab === t.id ? 'var(--series-1)' : 'transparent'}`,
                  }}>
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'cards' && (
        <div className="space-y-5">
          <div className="flex items-end justify-between gap-4">
            <Select label="Group by" value={groupBy} onChange={setGroupBy}
                    options={[
                      { value: 'category', label: 'Category' },
                      { value: 'type', label: 'Card type' },
                      { value: 'mv', label: 'Mana value' },
                    ]} />
            {!deck.corpus_available && (
              <span className="text-xs" style={{ color: 'var(--status-warning)' }}>
                No corpus — card text and art unavailable.
              </span>
            )}
          </div>

          {groups.map(([key, cards]) => (
            <section key={key} className="space-y-2">
              <h3 className="flex items-baseline gap-2 text-sm font-semibold">
                {groupBy === 'category' ? categoryLabel(key) : key}
                <span className="tabular text-xs font-normal"
                      style={{ color: 'var(--text-muted)' }}>
                  {cards.reduce((n, c) => n + c.qty, 0)}
                </span>
              </h3>
              <ul className="space-y-1">
                {cards.map((card) => (
                  <li key={card.name}
                      className="card-surface flex gap-3 rounded-lg px-3 py-2">
                    <CardHover card={card}>
                      <span className="cursor-help text-sm font-medium underline decoration-dotted underline-offset-2">
                        {card.qty > 1 && <span className="tabular mr-1">{card.qty}×</span>}
                        {card.name}
                      </span>
                    </CardHover>
                    <ManaCost cost={card.mana_cost} />
                    <p className="ml-auto max-w-[62%] text-right text-xs leading-relaxed"
                       style={{ color: 'var(--text-secondary)' }}>
                      {card.why}
                    </p>
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      )}

      {tab === 'stats' && (
        <div className="space-y-6">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <StatTile label="Cards" value={String(stats.total_cards)} />
            <StatTile label="Lands" value={String(stats.land_count)} />
            <StatTile label="Average MV" value={stats.curve.average_mv.toFixed(2)}
                      hint="nonland cards only" />
            <StatTile label="Nonland cards" value={String(stats.curve.nonland_cards)} />
          </div>

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="text-sm font-semibold">Mana curve</h3>
            <Caveat>Nonland cards by mana value. Lands are excluded.</Caveat>
            <CurveChart buckets={stats.curve.buckets} />
          </section>

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="text-sm font-semibold">Colored pips vs sources</h3>
            <Caveat>
              Identity says a card is legal here; pips say whether you can actually
              cast it. Hybrid pips count toward every color that can pay them.
            </Caveat>
            <ColorNeedsChart needs={stats.colors} />
            <DataTable
              columns={[
                { key: 'color', label: 'Color' },
                { key: 'pips', label: 'Pips required' },
                { key: 'cards', label: 'Cards needing it' },
                { key: 'sources', label: 'Sources' },
                { key: 'sources_per_pip', label: 'Sources / pip' },
              ]}
              rows={stats.colors}
            />
          </section>

          <section className="card-surface space-y-2 rounded-xl p-5">
            <h3 className="text-sm font-semibold">Category coverage</h3>
            <Caveat>
              Targets are conventional deckbuilding guidance, not computed truth —
              amber means outside the usual range, which is a prompt to think, not a
              failure.
            </Caveat>
            <CategoryCoverage rows={stats.categories} />
          </section>
        </div>
      )}

      {tab === 'validation' && (
        <div className="space-y-3">
          {report.ok && report.warnings.length === 0 && (
            <div className="card-surface rounded-lg px-4 py-6 text-sm"
                 style={{ color: 'var(--status-good)' }}>
              No issues. Every card is legal in this commander's identity, carries a
              rationale, and the deck is the right size.
            </div>
          )}
          {report.errors.map((issue, i) => (
            <div key={i} className="rounded-lg px-4 py-3 text-sm"
                 style={{
                   background: 'color-mix(in srgb, var(--status-critical) 10%, transparent)',
                   border: '1px solid color-mix(in srgb, var(--status-critical) 35%, transparent)',
                 }}>
              <Badge tone="critical">{issue.code}</Badge>{' '}
              {issue.card && <strong>{issue.card}: </strong>}
              {issue.message}
            </div>
          ))}
          {report.warnings.map((issue, i) => (
            <div key={i} className="rounded-lg px-4 py-3 text-sm"
                 style={{
                   background: 'color-mix(in srgb, var(--status-warning) 12%, transparent)',
                   border: '1px solid color-mix(in srgb, var(--status-warning) 35%, transparent)',
                 }}>
              <Badge tone="warning">{issue.code}</Badge>{' '}
              {issue.card && <strong>{issue.card}: </strong>}
              {issue.message}
            </div>
          ))}
        </div>
      )}

      {tab === 'notes' && (
        <div className="grid gap-4 md:grid-cols-2">
          {Object.entries(deck.notes).length === 0 && (
            <p className="text-sm" style={{ color: 'var(--text-muted)' }}>
              No notes recorded.
            </p>
          )}
          {Object.entries(deck.notes).map(([key, value]) => (
            <section key={key} className="card-surface rounded-xl p-4">
              <h3 className="text-xs font-semibold uppercase tracking-wide"
                  style={{ color: 'var(--text-muted)' }}>
                {key.replace(/_/g, ' ')}
              </h3>
              <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed"
                 style={{ color: 'var(--text-secondary)' }}>
                {value}
              </p>
            </section>
          ))}
        </div>
      )}
    </div>
  )
}

export { percent }
