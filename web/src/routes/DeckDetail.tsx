import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  api,
  type Card,
  type DeckDetail as Deck,
  type DeckStats,
  type Suggestions,
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
  const [suggestions, setSuggestions] = useState<Suggestions | null>(null)
  const requested = useRef<string | null>(null)
  // The card the user is proposing to swap in, and the rationale they are
  // writing for it. Null means no swap is being composed.
  const [swapping, setSwapping] = useState<{ out: string; into: string } | null>(null)
  const [swapWhy, setSwapWhy] = useState('')
  const [swapError, setSwapError] = useState<string | null>(null)
  const [swapBusy, setSwapBusy] = useState(false)

  async function applySwap() {
    if (!swapping || !swapWhy.trim()) return
    setSwapBusy(true)
    setSwapError(null)
    try {
      await api.swapCard(slug, { ...swapping, why: swapWhy.trim() })
      // Re-read everything the swap invalidates: the list, the gate, and the
      // shortlist, which should now be empty for this card.
      const [d, v] = await Promise.all([api.deck(slug), api.validate(slug)])
      setDeck(d)
      setReport(v)
      requested.current = null
      setSuggestions(null)
      setSwapping(null)
      setSwapWhy('')
    } catch (e: any) {
      setSwapError(String(e.message ?? e))
    } finally {
      setSwapBusy(false)
    }
  }

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
    setSuggestions(null)
    requested.current = null
  }, [slug])

  // Lazily, and only for the tab that shows them: building a shortlist means a
  // pool query per offending card, which is real work to do on a page most
  // visits never scroll to. A deck that passes the gate returns immediately.
  //
  // The ref, not the state, is what makes this one request. Guarding on
  // `suggestions` looks equivalent and is not: it stays null until the response
  // lands, so switching tabs twice in that window fires the query again.
  useEffect(() => {
    if (tab !== 'validation' || requested.current === slug) return
    requested.current = slug
    api.suggestions(slug).then(setSuggestions).catch(() => setSuggestions(null))
  }, [tab, slug])

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
                      className="card-surface flex items-center gap-3 rounded-lg p-2">
                    {/* The art crop, not the full card: at this size a whole
                        card scan is an unreadable smudge, while the art alone
                        is what the eye actually recognises a card by. Hover
                        still gives the full card for the text. */}
                    <CardHover card={card}>
                      <CardArt src={card.art_crop} alt={card.name}
                               ratio="aspect-[626/457]"
                               className="w-16 shrink-0 cursor-help" />
                    </CardHover>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline gap-2">
                        <CardHover card={card}>
                          <span className="cursor-help text-sm font-medium">
                            {card.qty > 1 && <span className="tabular mr-1">{card.qty}×</span>}
                            {card.name}
                          </span>
                        </CardHover>
                        <ManaCost cost={card.mana_cost} />
                      </div>
                      <p className="mt-0.5 text-xs leading-relaxed"
                         style={{ color: 'var(--text-secondary)' }}>
                        {card.why}
                      </p>
                    </div>
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
          {report.errors.map((issue, i) => {
            const shortlist = suggestions?.targets.find((t) => t.card === issue.card)
            return (
              <div key={i} className="space-y-3 rounded-lg px-4 py-3 text-sm"
                   style={{
                     background: 'color-mix(in srgb, var(--status-critical) 10%, transparent)',
                     border: '1px solid color-mix(in srgb, var(--status-critical) 35%, transparent)',
                   }}>
                <div>
                  <Badge tone="critical">{issue.code}</Badge>{' '}
                  {issue.card && <strong>{issue.card}: </strong>}
                  {issue.message}
                </div>

                {shortlist && shortlist.candidates.length > 0 && (
                  <div className="space-y-2">
                    <p className="text-xs" style={{ color: 'var(--text-secondary)' }}>
                      Legal cards that most resemble it, by type, mana value,
                      keywords and oracle text — with EDHREC rank only as a
                      tiebreak. <strong>A shortlist to argue with, not a
                      recommendation:</strong> the choice is yours, and nothing
                      here edits the deck.
                    </p>
                    <ul className="space-y-1">
                      {shortlist.candidates.map((c) => (
                        <li key={c.name}
                            className="flex items-center gap-3 rounded-lg p-2"
                            style={{ background: 'var(--surface-1)' }}>
                          <CardArt src={c.art_crop} alt={c.name}
                                   ratio="aspect-[626/457]" className="w-16 shrink-0" />
                          <div className="min-w-0 flex-1">
                            <div className="flex items-baseline gap-2">
                              <span className="text-sm font-medium">{c.name}</span>
                              <ManaCost cost={c.mana_cost} />
                            </div>
                            <p className="mt-0.5 text-xs leading-relaxed"
                               style={{ color: 'var(--text-muted)' }}>
                              {c.reasons.join(' · ')}
                            </p>
                          </div>
                          <button
                            onClick={() => {
                              setSwapping({ out: shortlist.card, into: c.name })
                              setSwapWhy('')
                              setSwapError(null)
                            }}
                            className="shrink-0 rounded-lg px-3 py-1.5 text-xs font-medium"
                            style={{ background: 'var(--gridline)',
                                     color: 'var(--text-primary)' }}>
                            Use this card
                          </button>
                        </li>
                      ))}
                    </ul>

                    {swapping?.out === shortlist.card && (
                      <div className="space-y-2 rounded-lg p-3"
                           style={{ background: 'var(--surface-1)' }}>
                        <p className="text-xs font-medium">
                          Swap {swapping.out} → {swapping.into}
                        </p>
                        {/* Rule 4: every card carries a rationale, and one
                            written by the tool is exactly the empty
                            justification that rule exists to prevent. So the
                            button stays disabled until a human writes one. */}
                        <textarea
                          value={swapWhy}
                          onChange={(e) => setSwapWhy(e.target.value)}
                          rows={3}
                          placeholder="Why does this card earn the slot? Required — the gate will not accept a card without a rationale."
                          className="w-full rounded-md px-2 py-1.5 text-xs outline-none focus:ring-2"
                          style={{ background: 'var(--surface-2, var(--gridline))',
                                   color: 'var(--text-primary)',
                                   border: '1px solid var(--hairline)' }}
                        />
                        {swapError && <ErrorNote>{swapError}</ErrorNote>}
                        <div className="flex items-center gap-2">
                          <button onClick={applySwap}
                                  disabled={!swapWhy.trim() || swapBusy}
                                  className="rounded-lg px-3 py-1.5 text-xs font-medium disabled:opacity-50"
                                  style={{ background: 'var(--series-1)', color: '#fff' }}>
                            {swapBusy ? 'Swapping…' : 'Apply swap'}
                          </button>
                          <button onClick={() => setSwapping(null)}
                                  className="text-xs underline"
                                  style={{ color: 'var(--text-muted)' }}>
                            Cancel
                          </button>
                          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
                            Writes deck.yaml. Commit it — deck history is git history.
                          </span>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )
          })}
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
