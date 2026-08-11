import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type DeckSummary, type Health } from '../lib/api'
import { identityName } from '../lib/mtg'
import { Badge, CardArt, ColorPips, ErrorNote, Select, Spinner } from '../components/ui'

export default function Library() {
  const [decks, setDecks] = useState<DeckSummary[] | null>(null)
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [bracket, setBracket] = useState('all')
  const [color, setColor] = useState('all')
  const [sort, setSort] = useState('name')

  useEffect(() => {
    Promise.all([api.decks(), api.health()])
      .then(([d, h]) => {
        setDecks(d)
        setHealth(h)
      })
      .catch((e) => setError(String(e.message ?? e)))
  }, [])

  const shown = useMemo(() => {
    let list = decks ?? []
    if (bracket !== 'all') list = list.filter((d) => String(d.bracket) === bracket)
    if (color !== 'all') list = list.filter((d) => d.color_identity.includes(color))
    return [...list].sort((a, b) =>
      sort === 'bracket'
        ? (b.bracket ?? 0) - (a.bracket ?? 0)
        : sort === 'size'
          ? b.total_cards - a.total_cards
          : a.name.localeCompare(b.name),
    )
  }, [decks, bracket, color, sort])

  if (error) return <ErrorNote>Could not load decks: {error}</ErrorNote>
  if (!decks) return <Spinner label="Loading decks…" />

  const brackets = [...new Set(decks.map((d) => d.bracket).filter(Boolean))].sort()

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Deck library</h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>
            {decks.length} deck{decks.length === 1 ? '' : 's'} ·{' '}
            {health?.corpus
              ? `${health.oracle_cards.toLocaleString()} cards in the local corpus`
              : 'no corpus yet — run `mtglab data refresh`'}
          </p>
        </div>

        <div className="flex flex-wrap items-end gap-3">
          <Select label="Bracket" value={bracket} onChange={setBracket}
                  options={[{ value: 'all', label: 'All brackets' },
                    ...brackets.map((b) => ({ value: String(b), label: `Bracket ${b}` }))]} />
          <Select label="Color" value={color} onChange={setColor}
                  options={[{ value: 'all', label: 'Any color' },
                    ...['W', 'U', 'B', 'R', 'G'].map((c) => ({ value: c, label: c }))]} />
          <Select label="Sort" value={sort} onChange={setSort}
                  options={[
                    { value: 'name', label: 'Name' },
                    { value: 'bracket', label: 'Bracket' },
                    { value: 'size', label: 'Card count' },
                  ]} />
        </div>
      </header>

      {shown.length === 0 ? (
        <div className="card-surface rounded-lg px-4 py-8 text-center text-sm"
             style={{ color: 'var(--text-secondary)' }}>
          No decks match those filters.
        </div>
      ) : (
        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {shown.map((deck) => (
            <Link key={deck.slug} to={`/decks/${deck.slug}`}
                  className="card-surface group overflow-hidden rounded-xl transition hover:-translate-y-0.5 hover:shadow-lg">
              <CardArt src={deck.art_crop} alt={deck.commander[0] ?? deck.name}
                       ratio="aspect-[626/300]" className="rounded-none" />
              <div className="space-y-2 p-4">
                <div className="flex items-start justify-between gap-2">
                  <h2 className="font-semibold leading-tight">{deck.name}</h2>
                  <div className="flex shrink-0 items-center gap-1">
                    {/* null is "the corpus was missing, so the gate never
                        ran" -- which is not the same as passing. Rendering it
                        like a clean deck throws away the distinction the list
                        endpoint carries these counts to preserve. */}
                    {deck.errors === null && <Badge tone="warning">not checked</Badge>}
                    {deck.errors !== null && deck.errors > 0 && (
                      <Badge tone="critical">
                        {deck.errors} error{deck.errors === 1 ? '' : 's'}
                      </Badge>
                    )}
                    {deck.bracket && <Badge>B{deck.bracket}</Badge>}
                  </div>
                </div>
                <div className="flex items-center gap-2 text-xs"
                     style={{ color: 'var(--text-secondary)' }}>
                  <ColorPips identity={deck.color_identity} />
                  <span>{identityName(deck.color_identity)}</span>
                  <span aria-hidden>·</span>
                  <span className="tabular">{deck.total_cards} cards</span>
                  <span aria-hidden>·</span>
                  <span className="tabular">{deck.land_count} lands</span>
                </div>
                {deck.strategy && (
                  <p className="line-clamp-3 text-xs leading-relaxed"
                     style={{ color: 'var(--text-muted)' }}>
                    {deck.strategy}
                  </p>
                )}
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
