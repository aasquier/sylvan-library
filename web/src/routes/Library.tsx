import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type DeckSummary, type Health } from '../lib/api'
import { identityName } from '../lib/mtg'
import {
  Badge, CardArt, ColorPips, ErrorNote, ManaText, Select, Spinner,
} from '../components/ui'

/**
 * Sylvan Library, Yeong-Hao Han, Commander's Arsenal (2012) — the card this
 * project is named after.
 *
 * Hotlinked rather than committed: rule 5 and
 * [ADR 6](docs/adr/0006-never-redistribute-scryfall-bulk-data.md) say never
 * redistribute Scryfall's data, and a card image checked into a public repo is
 * exactly that. Every other card image in the app is a hotlink to this CDN
 * too; the only difference here is that the URL is a constant.
 *
 * Which is itself a deliberate choice. It cannot come from the corpus: `art_crop`
 * hangs off the oracle card, so the corpus only knows Scryfall's *default*
 * printing — a different painting by a different artist. Picking this one means
 * naming this printing. The `?<timestamp>` cache-buster Scryfall appends is
 * dropped; the bare URL serves the same bytes and does not rot when they
 * re-stamp it.
 */
const SYLVAN_LIBRARY_ART =
  'https://cards.scryfall.io/art_crop/front/3/0/3003d481-1b52-4aa9-bbdc-e948fbc8d49d.jpg'

/**
 * What someone sees before they own anything.
 *
 * Distinct from "no decks match those filters", which is a dead end you get
 * out of by changing a filter. An empty library is a beginning, and until
 * import existed there was nothing to offer here but a shrug.
 */
function FirstRun() {
  return (
    <section className="card-surface relative overflow-hidden rounded-xl">
      {/* `hero-art` and `hero-scrim` live in index.css because the two themes
          need opposite treatment — dark mode brightens the art rather than
          dimming it, or a dark painting under a near-black scrim is mud. The
          reasoning is written out there. */}
      <img src={SYLVAN_LIBRARY_ART} alt="" aria-hidden className="hero-art" />
      <div className="hero-scrim" />
      <div className="relative px-6 py-14 sm:px-10 sm:py-20">
        <h2 className="max-w-lg text-2xl font-semibold tracking-tight">
          Nothing on the shelf yet.
        </h2>
        <p className="mt-2 max-w-xl text-sm leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          Paste a decklist from Moxfield, Archidekt, Arena or anywhere else.
          Names resolve against the local corpus, the gate checks legality and
          colour identity immediately, and the deck arrives as a draft — with a
          count of the cards whose slot you have not argued for yet.
        </p>
        <Link to="/import"
              className="mt-6 inline-block rounded-lg px-4 py-2 text-sm font-medium"
              style={{ background: 'var(--series-1)', color: '#fff' }}>
          Import a decklist
        </Link>
        <p className="mt-8 text-[11px]" style={{ color: 'var(--text-muted)' }}>
          Art: <em>Sylvan Library</em> by Yeong-Hao Han, Commander&rsquo;s Arsenal.
        </p>
      </div>
    </section>
  )
}

export default function Library() {
  const [decks, setDecks] = useState<DeckSummary[] | null>(null)
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [bracket, setBracket] = useState('all')
  const [color, setColor] = useState('all')
  const [sort, setSort] = useState('name')
  const [status, setStatus] = useState('all')
  const [stage, setStage] = useState('all')

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
    if (status !== 'all') list = list.filter((d) => d.status === status)
    if (stage !== 'all') list = list.filter((d) => d.stage === stage)
    return [...list].sort((a, b) =>
      sort === 'bracket'
        ? (b.bracket ?? 0) - (a.bracket ?? 0)
        : sort === 'size'
          ? b.total_cards - a.total_cards
          : a.name.localeCompare(b.name),
    )
  }, [decks, bracket, color, status, stage, sort])

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
          <Select label="Status" value={status} onChange={setStatus}
                  options={[{ value: 'all', label: 'Built and theory' },
                    { value: 'built', label: 'Built' },
                    { value: 'theoretical', label: 'Theory' }]} />
          <Select label="Stage" value={stage} onChange={setStage}
                  options={[{ value: 'all', label: 'Draft and curated' },
                    { value: 'curated', label: 'Curated' },
                    { value: 'draft', label: 'Draft' }]} />
          <Select label="Sort" value={sort} onChange={setSort}
                  options={[
                    { value: 'name', label: 'Name' },
                    { value: 'bracket', label: 'Bracket' },
                    { value: 'size', label: 'Card count' },
                  ]} />
          <Link to="/import"
                className="h-9 rounded-lg px-4 text-sm font-medium leading-9"
                style={{ background: 'var(--series-1)', color: '#fff' }}>
            Import a decklist
          </Link>
        </div>
      </header>

      {decks.length === 0 ? (
        <FirstRun />
      ) : shown.length === 0 ? (
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
                    {/* A theoretical deck is a list, not a box of cards.
                        Worth saying on the shelf, because the difference
                        decides whether you can sit down and play it. */}
                    {deck.status === 'theoretical' && <Badge>theory</Badge>}
                    {/* Orthogonal to that: has anyone reasoned about it?
                        A draft renders like a curated deck unless the shelf
                        says so, which hides the distinction the stage exists
                        to draw. The count is the prompt (ADR 13). */}
                    {deck.stage === 'draft' && (
                      <Badge tone="warning">
                        draft · {deck.needs_rationale}
                      </Badge>
                    )}
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
                    <ManaText>{deck.strategy}</ManaText>
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
