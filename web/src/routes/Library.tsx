import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, deckUrl, type DeckTile, type Health } from '../lib/api'
import { identityName } from '../lib/mtg'
import {
  Badge, CardArt, ColorPips, ErrorNote, ManaText, PageMasthead, Select,
  Spinner,
} from '../components/ui'

/**
 * The word that confirms a deletion. Mirrors `service.DELETE_WORD`, which also
 * still accepts the slug; this dialog asks for the shorter of the two.
 *
 * Magic's own retired templating for destroying something that cannot
 * regenerate — and the right verb because the obvious alternative is wrong:
 * the deck moves to `decks/.trash/`, so "exile", which in Magic means gone for
 * good, would promise something harsher than what happens.
 */
const DELETE_WORD = 'bury'

/**
 * Confirm a deletion by typing a word.
 *
 * Not a yes/no. An "Are you sure?" is answered the same way by someone who
 * read the dialog and someone who clicked through it, and the deck this is
 * most likely to be aimed at by mistake is a draft imported minutes ago that
 * git has never seen. It says where the deck goes, too, because a deletion
 * someone is nervous about is one they should be able to see is reversible
 * before they commit to it.
 *
 * It used to ask for the slug, and that is worth recording because it failed
 * in a way that looked like a broken button. The label was styled
 * `uppercase`, so `ishai-ojutai-dragonspeaker` rendered as
 * `ISHAI-OJUTAI-DRAGONSPEAKER`; the comparison was case-sensitive against the
 * lowercase slug; and typing exactly what was on screen left the button
 * disabled with nothing on screen explaining why. **A field whose contents
 * must be retyped verbatim can never carry a `text-transform`** — that is the
 * general rule the bug is an instance of, and it is why the label below is not
 * uppercased even though every other small label in the app is.
 *
 * Both fixes are here rather than only one: the word is short enough to retype
 * without copying, *and* the match ignores case and surrounding space, so the
 * near side cannot refuse something the server would accept.
 */
function DeleteDialog({ deck, onCancel, onDeleted }: {
  deck: DeckTile
  onCancel: () => void
  onDeleted: (movedTo: string) => void
}) {
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const answer = typed.trim().toLowerCase()
  const matches = answer === DELETE_WORD || answer === deck.slug.toLowerCase()

  async function remove() {
    if (!matches) return
    setBusy(true)
    setError(null)
    try {
      // The normalised answer, not the raw keystrokes: it is the value this
      // dialog actually validated, and sending anything else would let the two
      // sides disagree about what was confirmed.
      const result = await api.deleteDeck(deck, answer)
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
          {/* Deliberately not `uppercase` — see the component's docstring.
              The word is inside a `<code>` so it is unmistakably the literal
              to retype rather than an English verb in a sentence. */}
          <span className="text-xs tracking-wide"
                style={{ color: 'var(--text-muted)' }}>
            Type <code style={{ color: 'var(--text-primary)' }}>{DELETE_WORD}</code>
            {' '}to confirm
          </span>
          <input
            autoFocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') onCancel()
              if (e.key === 'Enter' && matches) void remove()
            }}
            placeholder={DELETE_WORD}
            aria-label={`Type ${DELETE_WORD} to confirm deleting ${deck.name}`}
            className="mt-1 w-full rounded-md px-3 py-2 font-mono text-sm"
            style={{ background: 'var(--page)', color: 'var(--text-primary)',
                     border: '1px solid var(--hairline)' }}
          />
          {/* A disabled button with no stated reason is what made the old
              version read as broken rather than as refusing. */}
          {!matches && (
            <span className="mt-1 block text-xs" style={{ color: 'var(--text-muted)' }}>
              {typed.trim()
                ? <>That is not the word. Type <code>{DELETE_WORD}</code>, or the
                    deck&rsquo;s slug <code>{deck.slug}</code>.</>
                : <>Or type the deck&rsquo;s slug, <code>{deck.slug}</code>.</>}
            </span>
          )}
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
 * Which is itself a deliberate choice. It cannot come from the pool: `art_crop`
 * hangs off the oracle card, so the pool only knows Scryfall's *default*
 * printing — a different painting by a different artist. Picking this one means
 * naming this printing. The `?<timestamp>` cache-buster Scryfall appends is
 * dropped; the bare URL serves the same bytes and does not rot when they
 * re-stamp it.
 */
const SYLVAN_LIBRARY_ART =
  'https://cards.scryfall.io/art_crop/front/3/0/3003d481-1b52-4aa9-bbdc-e948fbc8d49d.jpg'

/**
 * The library's own nameplate, and the one place the app is named after
 * anything.
 *
 * It exists because the painting above had been in the codebase for weeks and
 * nobody had ever seen it: `FirstRun` is the only thing that rendered it, and
 * `FirstRun` only renders on an **empty** library, which no instance with
 * decks on it ever is. A piece of identity reachable exclusively by deleting
 * all your work is not identity.
 *
 * **Beside the title rather than behind it**, and that is the interesting
 * decision. The obvious masthead is a full-bleed band, and this is the third
 * time in this project that the obvious one has been wrong for the same
 * measured reason: `art_crop` is 616x452, about 1.36:1, and a band across a
 * 1230px page is nearer 3:1, so `object-fit: cover` keeps **44% of the
 * painting's height** and throws the rest away from the top and bottom
 * equally. On this painting that lands on bare wall texture — the sky goes,
 * the path goes, and the three human figures that give the canyon its scale go
 * with it.
 *
 * Branch 1 hit this on the deck hero and answered it by showing the commander
 * as a whole card next to the band rather than as a second crop. Same answer
 * here, for the same reason, and it costs nothing: shown at its own ratio the
 * painting is entirely visible, which is the whole point of putting it on the
 * page.
 */
function LibraryMasthead({ decks, health }: {
  decks: number
  health: Health | null
}) {
  return (
    <PageMasthead
      art={SYLVAN_LIBRARY_ART}
      alt="Sylvan Library, painted by Yeong-Hao Han: a canyon of mossy
           trunks riddled with alcoves, with three tiny figures on the path
           below for scale."
      title="Deck library"
      credit={<>
        <em>Sylvan Library</em> by Yeong-Hao Han, Commander&rsquo;s Arsenal —
        the card this project is named after.
      </>}>
      <p>
        {decks} deck{decks === 1 ? '' : 's'} ·{' '}
        {health?.pool
          ? `${health.oracle_cards.toLocaleString()} cards in the local pool`
          : 'no card pool yet — run `mtglab data refresh`'}
      </p>
    </PageMasthead>
  )
}

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
          Names resolve against the local pool, the gate checks legality and
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

/**
 * One deck on the shelf.
 *
 * Extracted when the browse tab arrived, because the same tile now renders in
 * two places and the alternative was the whole card duplicated per tab.
 *
 * Two things it says that it did not have to before decks had owners: whose it
 * is, when it is not yours; and whether it is private, when it is. Both are
 * only shown in the case that carries information — a tile on a laptop, where
 * one person owns everything and shares it with nobody, is unchanged.
 */
function DeckCard({ deck, onDelete, heading: Heading = 'h2' }: {
  deck: DeckTile
  onDelete: (deck: DeckTile) => void
  /** `h3` inside the browse tab, where the owner's username is the `h2` these
   *  decks sit under. A flat shelf keeps `h2`, which is what it is: a list of
   *  decks under the page's own `h1`. */
  heading?: 'h2' | 'h3'
}) {
  return (
    // `group/card` is on this wrapper rather than on the Link, because the
    // delete button has to sit outside the Link — inside it, a click would
    // navigate — and still react to hovering the card.
    <div className="group/card relative">
      {/* Muted until the card is hovered or the button is focused: deleting a
          deck should be reachable without being the thing your eye lands on. */}
      {deck.writable && (
        <button
          onClick={() => onDelete(deck)}
          title={`Delete ${deck.name}`}
          aria-label={`Delete ${deck.name}`}
          className="absolute right-2 top-2 z-10 rounded-md px-2 py-1 text-[11px] opacity-0 transition focus:opacity-100 group-hover/card:opacity-100"
          style={{ background: 'var(--surface-1)', color: 'var(--text-muted)',
                   border: '1px solid var(--hairline)' }}
        >
          Delete
        </button>
      )}
      <Link to={deckUrl(deck)}
            className="card-surface block overflow-hidden rounded-xl transition hover:-translate-y-0.5 hover:shadow-lg">
        {/* 626/457 is `art_crop`'s own shape, so the tile shows the whole
            painting instead of a band cut out of its middle. It was 626/300,
            which threw away a third of the height and took it off the top and
            bottom equally — on a commander drawn head-up, off the head. A
            taller tile is the cost, and it is the right one on a shelf whose
            whole job is recognition. */}
        <CardArt src={deck.art_crop} alt={deck.commander[0] ?? deck.name}
                 ratio="aspect-[626/457]" className="rounded-none" />
        <div className="space-y-2 p-4">
          <div className="flex items-start justify-between gap-2">
            <Heading className="font-semibold leading-tight">{deck.name}</Heading>
            <div className="flex shrink-0 items-center gap-1">
              {/* Only on somebody else's deck. On your own it would be a label
                  reading "yours" on every tile, which is not information. */}
              {!deck.writable && <Badge>{deck.owner}</Badge>}
              {/* And only on your own, for the mirror-image reason: a private
                  deck of somebody else's is not in this response at all, so
                  the absence of this badge elsewhere says nothing. */}
              {deck.writable && !deck.shared && <Badge>private</Badge>}
              {/* null is "the pool was missing, so the gate never ran" --
                  which is not the same as passing. Rendering it like a clean
                  deck throws away the distinction the list endpoint carries
                  these counts to preserve. */}
              {/* A theoretical deck is a list, not a box of cards. Worth
                  saying on the shelf, because the difference decides whether
                  you can sit down and play it. */}
              {deck.status === 'theoretical' && <Badge>theory</Badge>}
              {/* Orthogonal to that: has anyone reasoned about it? A draft
                  renders like a curated deck unless the shelf says so, which
                  hides the distinction the stage exists to draw. The count is
                  the prompt (ADR 13). */}
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
  )
}

function DeckGrid({ decks, onDelete, heading }: {
  decks: DeckTile[]
  onDelete: (deck: DeckTile) => void
  heading?: 'h2' | 'h3'
}) {
  return (
    <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
      {decks.map((deck) => (
        <DeckCard key={`${deck.owner}/${deck.slug}`} deck={deck}
                  onDelete={onDelete} heading={heading} />
      ))}
    </div>
  )
}

/** Which shelf is being looked at. See the `split` memo below. */
type Shelf = 'mine' | 'players'

export default function Library() {
  const [decks, setDecks] = useState<DeckTile[] | null>(null)
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [bracket, setBracket] = useState('all')
  const [color, setColor] = useState('all')
  const [sort, setSort] = useState('name')
  const [status, setStatus] = useState('all')
  const [stage, setStage] = useState('all')
  const [shelf, setShelf] = useState<Shelf>('mine')
  const [deleting, setDeleting] = useState<DeckTile | null>(null)
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

  /**
   * The two shelves ADR 22 asks for: yours, and everybody else's.
   *
   * Other players' decks are "a tab somebody opts into rather than something
   * in the way", and the maintainer's showcase is "always visible" — so the
   * default shelf is what you can write plus the six, and the browse tab is
   * the remainder.
   *
   * Both tests come from the server. `writable` is the caller's own decks and
   * `showcase` is the curated six's owner; neither is a comparison this client
   * could make, because it is never told who the maintainer is.
   */
  const [mine, players] = useMemo(() => {
    const list = decks ?? []
    return [
      list.filter((d) => d.writable || d.showcase),
      list.filter((d) => !d.writable && !d.showcase),
    ]
  }, [decks])

  const shown = useMemo(() => {
    let list = shelf === 'mine' ? mine : players
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
  }, [mine, players, shelf, bracket, color, status, stage, sort])

  /** The browse shelf, grouped under the username it belongs to.
   *
   * Alphabetical by owner, and by whatever `sort` says within each — the deck
   * controls keep working inside a group rather than being overridden by it.
   */
  const byOwner = useMemo(() => {
    const out = new Map<string, DeckTile[]>()
    for (const deck of shown) {
      if (!out.has(deck.owner)) out.set(deck.owner, [])
      out.get(deck.owner)!.push(deck)
    }
    return [...out.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [shown])

  if (error) return <ErrorNote>Could not load decks: {error}</ErrorNote>
  if (!decks) return <Spinner label="Loading decks…" />

  const brackets = [...new Set(decks.map((d) => d.bracket).filter(Boolean))].sort()

  return (
    <div className="space-y-6">
      {/* The nameplate carries the title and the counts now, so the row below
          is only the controls.

          The plain heading on an empty library is not a stylistic choice, it
          is the page keeping its `h1`. `FirstRun` is this same painting at
          full size, so showing the nameplate too would introduce the app
          twice — but dropping the nameplate silently dropped the only
          top-level heading with it, which is what the first version of this
          did. */}
      {mine.length > 0 ? (
        <LibraryMasthead decks={mine.length} health={health} />
      ) : (
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Deck library</h1>
          <p className="mt-1 text-sm" style={{ color: 'var(--text-secondary)' }}>
            {health?.pool
              ? `${health.oracle_cards.toLocaleString()} cards in the local pool`
              : 'no card pool yet — run `mtglab data refresh`'}
          </p>
        </div>
      )}

      {/* Offered only when there is somebody to browse. On a laptop, and on an
          instance where nobody else has shared anything, this is one tab
          labelling itself — which is exactly the "something in the way" ADR 22
          asked the browse view not to be. */}
      {players.length > 0 && (
        <div role="tablist" aria-label="Whose decks"
             className="flex gap-1 border-b" style={{ borderColor: 'var(--hairline)' }}>
          {([
            ['mine', 'My decks', mine.length],
            ['players', 'Other players', players.length],
          ] as const).map(([key, label, count]) => (
            <button key={key} role="tab" aria-selected={shelf === key}
                    onClick={() => setShelf(key)}
                    className="-mb-px border-b-2 px-3 py-2 text-sm font-medium transition"
                    style={{
                      borderColor: shelf === key ? 'var(--series-1)' : 'transparent',
                      color: shelf === key ? 'var(--text-primary)' : 'var(--text-muted)',
                    }}>
              {label}
              <span className="ml-1.5 text-xs tabular" style={{ color: 'var(--text-muted)' }}>
                {count}
              </span>
            </button>
          ))}
        </div>
      )}

      <header className="flex flex-wrap items-end justify-between gap-4">
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

      {shelf === 'mine' && mine.length === 0 ? (
        <FirstRun />
      ) : shown.length === 0 ? (
        <div className="card-surface rounded-lg px-4 py-8 text-center text-sm"
             style={{ color: 'var(--text-secondary)' }}>
          No decks match those filters.
        </div>
      ) : shelf === 'mine' ? (
        <DeckGrid decks={shown} onDelete={setDeleting} />
      ) : (
        // Grouped under the username, which is what ADR 22 asked browsing to
        // be organised by — and the path shape gives it for free, since the
        // owner is already half of every deck's address.
        <div className="space-y-8">
          {byOwner.map(([owner, group]) => (
            <section key={owner} className="space-y-4">
              <h2 className="flex items-baseline gap-2 text-lg font-semibold tracking-tight">
                {owner}
                <span className="text-xs font-normal" style={{ color: 'var(--text-muted)' }}>
                  {group.length} deck{group.length === 1 ? '' : 's'} shared
                </span>
              </h2>
              <DeckGrid decks={group} onDelete={setDeleting} heading="h3" />
            </section>
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
            //
            // Matched on the whole address, not the slug: two owners may have
            // a `goreclaw` and only one of them was deleted.
            setDecks((current) => (current ?? []).filter(
              (d) => d.slug !== deleting.slug || d.owner !== deleting.owner))
            setDeleted({ name: deleting.name, movedTo })
            setDeleting(null)
          }}
        />
      )}
    </div>
  )
}
