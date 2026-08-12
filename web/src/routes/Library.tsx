import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, type DeckSummary, type Health } from '../lib/api'
import { identityName } from '../lib/mtg'
import {
  Badge, CardArt, ColorPips, ErrorNote, ManaText, Select, Spinner,
} from '../components/ui'

/**
 * Confirm a deletion by typing the slug.
 *
 * Not a yes/no. The server refuses anything but the slug itself, and this is
 * the same check on the near side rather than a softer one: an "Are you sure?"
 * is answered the same way by someone who read the dialog and someone who
 * clicked through it, and the deck this is most likely to be aimed at by
 * mistake is a draft imported minutes ago that git has never seen.
 *
 * It says where the deck goes, because a deletion someone is nervous about is
 * one they should be able to see is reversible before they commit to it.
 */
function DeleteDialog({ deck, onCancel, onDeleted }: {
  deck: DeckSummary
  onCancel: () => void
  onDeleted: (movedTo: string) => void
}) {
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const matches = typed.trim() === deck.slug

  async function remove() {
    if (!matches) return
    setBusy(true)
    setError(null)
    try {
      const result = await api.deleteDeck(deck.slug, typed.trim())
      onDeleted(result.moved_to)
    } catch (e) {
      setError(String((e as Error).message ?? e))
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4"
         style={{ background: 'rgba(0,0,0,0.6)' }}
         onClick={onCancel}>
      <div className="card-surface w-full max-w-md rounded-xl p-6"
           role="dialog" aria-modal="true"
           aria-label={`Delete ${deck.name}`}
           onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold tracking-tight">Delete {deck.name}?</h2>
        <p className="mt-2 text-sm" style={{ color: 'var(--text-secondary)' }}>
          {deck.total_cards} cards · {deck.stage} · {deck.status}
        </p>
        <p className="mt-3 text-sm leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
          The deck moves to <code>decks/.trash/</code> rather than being erased,
          so this is reversible from the shell. Its artifacts go with it.
        </p>
        <label className="mt-4 block">
          <span className="text-xs uppercase tracking-wide"
                style={{ color: 'var(--text-muted)' }}>
            Type <code>{deck.slug}</code> to confirm
          </span>
          <input
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') onCancel()
              if (e.key === 'Enter' && matches) void remove()
            }}
            className="mt-1 w-full rounded-md px-3 py-2 font-mono text-sm"
            style={{ background: 'var(--page)', color: 'var(--text-primary)',
                     border: '1px solid var(--hairline)' }}
          />
        </label>
        {error && <div className="mt-3"><ErrorNote>{error}</ErrorNote></div>}
        <div className="mt-5 flex items-center gap-3">
          <button onClick={remove} disabled={!matches || busy}
                  className="rounded-lg px-4 py-2 text-sm font-medium disabled:opacity-40"
                  style={{ background: 'var(--status-critical)', color: '#fff' }}>
            {busy ? 'Deleting…' : 'Delete this deck'}
          </button>
          <button onClick={onCancel} disabled={busy}
                  className="rounded-lg px-3 py-2 text-sm"
                  style={{ border: '1px solid var(--hairline)',
                           color: 'var(--text-secondary)' }}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  )
}

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
  const [deleting, setDeleting] = useState<DeckSummary | null>(null)
  // Kept after the dialog closes: "it is in .trash/" is the sentence that
  // makes a deletion feel survivable, and it is useless if it flashes past.
  const [deleted, setDeleted] = useState<{ name: string; movedTo: string } | null>(null)

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

      {deleted && (
        <div className="card-surface flex flex-wrap items-center gap-2 rounded-lg px-4 py-3 text-sm"
             style={{ color: 'var(--text-secondary)' }}>
          <span>
            Deleted <strong>{deleted.name}</strong>. It moved to{' '}
            <code>{deleted.movedTo}</code> — not gone, aside.
          </span>
          <button onClick={() => setDeleted(null)}
                  className="ml-auto text-xs underline"
                  style={{ color: 'var(--text-muted)' }}>
            Dismiss
          </button>
        </div>
      )}

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
            // `group/card` is on this wrapper rather than on the Link, because
            // the delete button has to sit outside the Link — inside it, a
            // click would navigate — and still react to hovering the card.
            <div key={deck.slug} className="group/card relative">
            {/* Muted until the card is hovered or the button is focused:
                deleting a deck should be reachable without being the thing
                your eye lands on. */}
            <button
              onClick={() => setDeleting(deck)}
              title={`Delete ${deck.name}`}
              aria-label={`Delete ${deck.name}`}
              className="absolute right-2 top-2 z-10 rounded-md px-2 py-1 text-[11px] opacity-0 transition focus:opacity-100 group-hover/card:opacity-100"
              style={{ background: 'var(--surface-1)', color: 'var(--text-muted)',
                       border: '1px solid var(--hairline)' }}
            >
              Delete
            </button>
            <Link to={`/decks/${deck.slug}`}
                  className="card-surface block overflow-hidden rounded-xl transition hover:-translate-y-0.5 hover:shadow-lg">
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
            </div>
          ))}
        </div>
      )}

      {deleting && (
        <DeleteDialog
          deck={deleting}
          onCancel={() => setDeleting(null)}
          onDeleted={(movedTo) => {
            // Drop it from the list here rather than re-fetching: the server
            // has already confirmed, and a round trip would leave the deck on
            // screen for a beat after it was gone.
            setDecks((current) => (current ?? []).filter(
              (d) => d.slug !== deleting.slug))
            setDeleted({ name: deleting.name, movedTo })
            setDeleting(null)
          }}
        />
      )}
    </div>
  )
}
