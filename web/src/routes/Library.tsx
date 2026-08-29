import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  api, deckUrl, type DeckTile, type EntombedDeck, type Health,
} from '../lib/api'
import { identityName } from '../lib/mtg'
import { TheShelves } from '../components/shelf'
import {
  Badge, CardArt, ColorPips, ErrorNote, ManaText, PageMasthead,
  Select, Spinner,
} from '../components/ui'

/**
 * The word that confirms a deletion. Mirrors `service.DELETE_WORD`, which also
 * still accepts the slug; this dialog asks for the shorter of the two.
 *
 * Magic's own retired templating for destroying something that cannot
 * regenerate — and the right verb because the obvious alternative is wrong:
 * the deck goes to the crypt and can be raised again, so "exile", which in
 * Magic means gone for good, would promise something harsher than what happens.
 */
const DELETE_WORD = 'bury'

/**
 * When a deck was entombed, in words rather than in a timestamp.
 *
 * **Null is not a date.** The server sends null when nothing recorded the
 * burial, and this returns null in turn so the row can say nothing rather than
 * say "just now" — which is the shape of bug this repo makes most often and
 * would make here in the one place a player looks to check their deck survived.
 */
function entombedWhen(at: string | null): string | null {
  if (!at) return null
  const then = new Date(at)
  if (Number.isNaN(then.getTime())) return null
  const minutes = Math.round((Date.now() - then.getTime()) / 60000)
  if (minutes < 1) return 'moments ago'
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'} ago`
  const hours = Math.round(minutes / 60)
  if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`
  const days = Math.round(hours / 24)
  if (days < 30) return `${days} day${days === 1 ? '' : 's'} ago`
  return then.toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })
}

/**
 * Confirm a deletion by typing a word.
 *
 * Not a yes/no. An "Are you sure?" is answered the same way by someone who
 * read the dialog and someone who clicked through it, and the deck this is
 * most likely to be aimed at by mistake is a draft imported minutes ago that
 * nothing else has a copy of. It says the deck is recoverable, too, because a
 * deletion someone is nervous about is one they should be able to see is
 * survivable before they commit to it.
 *
 * **What it must never say is where the deck goes.** It said
 * `decks/.trash/` and "reversible from the shell" until 2026-08-29 — a
 * filesystem path and an instruction to open a terminal, shown to a player
 * (commandment 10). The fix was not to delete that sentence: it was the only
 * recovery anybody had. It was to build the crypt the sentence was standing in
 * for, and then describe *that* instead.
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
  onDeleted: (cryptId: string) => void
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
      // The handle, or the empty string when the server could not read the
      // crypt back in that instant. The notice treats those differently on
      // purpose: one offers a button, the other says where to look.
      onDeleted(result.recoverable ? result.crypt_id : '')
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
           aria-label={`Entomb ${deck.name}`}
           onClick={(e) => e.stopPropagation()}>
        <h2 className="text-lg font-semibold tracking-tight">Entomb {deck.name}?</h2>
        <p className="mt-2 text-sm" style={{ color: 'var(--text-secondary)' }}>
          {deck.total_cards} cards · {deck.stage} · {deck.status}
        </p>
        <p className="mt-3 text-sm leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
          Entombed, not exiled. The deck goes to your crypt whole — every
          rationale, every artifact — and you can raise it again from there
          whenever you like.
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
            aria-label={`Type ${DELETE_WORD} to confirm entombing ${deck.name}`}
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
                  className="btn btn-danger-solid">
            {busy ? 'Entombing…' : 'Entomb this deck'}
          </button>
          <button onClick={onCancel} disabled={busy}
                  className="btn btn-quiet">
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
        <em>Sylvan Library</em> by Yeong-Hao Han, Commander&rsquo;s Arsenal.
      </>}>
      <p>
        {decks} deck{decks === 1 ? '' : 's'} ·{' '}
        {health?.pool
          ? `${health.oracle_cards.toLocaleString()} cards in the local pool`
          : 'no card pool yet — the library awaits its first stocking'}
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
      {/* `hero-art`, `hero-lift` and `hero-scrim` live in index.css because
          the two themes need opposite treatment — dark mode lifts the art
          rather than dimming it, or a dark painting under a near-black scrim
          is mud. The reasoning is written out there.

          **Three elements in this order and the order is the whole rule.**
          The lift used to be `filter: brightness(1.3)` on the painting, which
          is Scryfall's "color-shift" reaching Yeong-Hao Han's brushwork; it is
          a screened sheet of light over the art now, and it has to sit above
          the painting and below the scrim, which is exactly what writing it
          second says. */}
      <img src={SYLVAN_LIBRARY_ART} alt="" aria-hidden className="hero-art" />
      <div className="art-lift hero-lift" aria-hidden />
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
function DeckCard({ deck, onDelete, heading: Heading = 'h2', index = 0 }: {
  deck: DeckTile
  onDelete: (deck: DeckTile) => void
  /** `h3` inside the browse tab, where the owner's username is the `h2` these
   *  decks sit under. A flat shelf keeps `h2`, which is what it is: a list of
   *  decks under the page's own `h1`. */
  heading?: 'h2' | 'h3'
  /** Position in the grid, for the entrance stagger — the shelf fills the
   *  way a hand lays cards out, not all at once. */
  index?: number
}) {
  return (
    // `group/card` is on this wrapper rather than on the Link, because the
    // delete button has to sit outside the Link — inside it, a click would
    // navigate — and still react to hovering the card.
    <div className="group/card tile-enter relative"
         style={{ '--tile-index': Math.min(index, 11) } as React.CSSProperties}>
      {/* Muted until the card is hovered or the button is focused: deleting a
          deck should be reachable without being the thing your eye lands on. */}
      {deck.writable && (
        <button
          onClick={() => onDelete(deck)}
          title={`Entomb ${deck.name} — the whole deck goes to your crypt, and can be raised again`}
          aria-label={`Entomb ${deck.name}`}
          className="btn btn-danger btn-xs absolute right-2 top-2 z-10 opacity-0 focus:opacity-100 group-hover/card:opacity-100"
          style={{ background: 'var(--surface-1)' }}
        >
          Entomb
        </button>
      )}
      <Link to={deckUrl(deck)}
            className="deck-tile card-surface block overflow-hidden rounded-xl">
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
              {/* Whose hands it belongs in, when the household tagged it.
                  Plain text, no emoji — a glyph here would render as
                  whatever the platform thinks a pilot is. */}
              {deck.pilot && <Badge>pilot · {deck.pilot}</Badge>}
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
      {decks.map((deck, i) => (
        <DeckCard key={`${deck.owner}/${deck.slug}`} deck={deck} index={i}
                  onDelete={onDelete} heading={heading} />
      ))}
    </div>
  )
}

/**
 * The crypt: every deck this player has entombed, and the way back out.
 *
 * ADR 27 gave the *card* level both halves — entomb, and a graveyard to
 * return from. The deck level had only the first, and what stood in for the
 * second was a line of copy naming the folder the deck had been moved to,
 * with the suggestion that the player go and fetch it from a shell. This is
 * the missing half, and the leak is closed by having something true to say
 * instead rather than by deleting the sentence.
 *
 * Modelled on the deck page's graveyard section deliberately: same glyph,
 * same verb, same shape of row. A player who has entombed a card has already
 * been taught how this ends.
 */
function TheCrypt({ entombed, error, busy, failed, onReturn }: {
  /** Null means *not known* — the crypt could not be read — which is why
   *  `error` exists rather than an empty list standing in for it. */
  entombed: EntombedDeck[] | null
  error: string | null
  busy: string | null
  failed: string | null
  onReturn: (id: string) => void
}) {
  if (entombed === null) {
    return (
      <ErrorNote>
        Your crypt could not be read just now, so this list may not be
        everything. Nothing has been lost — try again in a moment.
        {error ? ` (${error})` : ''}
      </ErrorNote>
    )
  }
  return (
    <section className="space-y-3">
      {/* The tab says "Crypt" and this says it again, which is deliberate: the
          tab is a control, and a reader arriving by keyboard or by screen
          reader needs the page's outline to name the room they are in. Same
          reason the browse shelf keeps a heading per owner. */}
      <h2 className="flex items-baseline gap-2 text-lg font-semibold tracking-tight">
        <span aria-hidden>⚰</span> The crypt
      </h2>
      <p className="max-w-2xl text-sm leading-relaxed" style={{ color: 'var(--text-secondary)' }}>
        Decks you entombed. Nothing here was erased — return one and it comes
        back whole, every rationale and every artifact with it. A deck always
        comes back under its own name.
      </p>
      {failed && <ErrorNote>{failed}</ErrorNote>}
      {entombed.length === 0 ? (
        <div className="card-surface rounded-lg px-4 py-8 text-center text-sm"
             style={{ color: 'var(--text-secondary)' }}>
          Nothing rests here. Every deck you have made is still on the shelf.
        </div>
      ) : (
        <ul className="space-y-2">
          {entombed.map((entry, i) => {
            const when = entombedWhen(entry.entombed_at)
            return (
              <li key={entry.id}
                  className="card-surface tile-enter flex flex-wrap items-center gap-3 rounded-lg px-4 py-3"
                  style={{ '--tile-index': Math.min(i, 11) } as React.CSSProperties}>
                <span aria-hidden className="text-lg leading-none"
                      style={{ color: 'var(--text-muted)' }}>⚰</span>
                <div className="min-w-0 flex-1 basis-52">
                  <div className="text-sm font-medium">{entry.name}</div>
                  <div className="mt-0.5 text-xs" style={{ color: 'var(--text-muted)' }}>
                    {/* **Every clause here is conditional, and that is the
                        point.** Each one is a fact the server sent, and each
                        one is omitted rather than guessed when the server had
                        no answer — `when` is null for a burial nothing
                        recorded, and a count of zero means the deck file could
                        not be read. "0 cards" beside a deck somebody knows had
                        99 is the worst sentence available on the one screen
                        they came to for reassurance, and a made-up "just now"
                        is the second worst. */}
                    {entry.commander.length > 0 && <>{entry.commander.join(' & ')} · </>}
                    {entry.total_cards > 0
                      && <>{entry.total_cards} card{entry.total_cards === 1 ? '' : 's'} · </>}
                    entombed{when ? ` ${when}` : ', and still here'}
                  </div>
                </div>
                <button
                  onClick={() => onReturn(entry.id)}
                  disabled={busy !== null}
                  title={`Return ${entry.name} to your library, exactly as it was`}
                  className="btn btn-quiet btn-sm">
                  {busy === entry.id ? 'Returning…' : 'Return'}
                </button>
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}

/** Which shelf is being looked at. See the `split` memo below. */
type Shelf = 'mine' | 'players' | 'crypt'

export default function Library() {
  const [decks, setDecks] = useState<DeckTile[] | null>(null)
  const [health, setHealth] = useState<Health | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [bracket, setBracket] = useState('all')
  const [color, setColor] = useState('all')
  const [sort, setSort] = useState('name')
  const [status, setStatus] = useState('all')
  const [stage, setStage] = useState('all')
  const [pilot, setPilot] = useState('all')
  const [shelf, setShelf] = useState<Shelf>('mine')
  const [deleting, setDeleting] = useState<DeckTile | null>(null)
  // Kept after the dialog closes: "it is in your crypt, here is the way back"
  // is the sentence that makes a deletion feel survivable, and it is useless
  // if it flashes past. `cryptId` is empty when the server could not hand one
  // over, and the notice says something different in that case rather than
  // offering a button that cannot work.
  const [deleted, setDeleted] = useState<{ name: string; cryptId: string } | null>(null)
  /** The crypt. **Null is "not known", not "empty"** — a failed read must not
   *  render as a player having nothing buried, which is the whole shape of
   *  this repo's most-repeated bug. */
  const [crypt, setCrypt] = useState<EntombedDeck[] | null>(null)
  const [cryptError, setCryptError] = useState<string | null>(null)
  const [returning, setReturning] = useState<string | null>(null)
  const [returnError, setReturnError] = useState<string | null>(null)

  /** Read the crypt. Its own call rather than part of the shelf's payload:
   *  the crypt is a place you visit, and a shelf that waited on it would be
   *  slower for everybody who has never deleted anything. */
  const loadCrypt = () =>
    api.entombed()
      .then((c) => {
        setCrypt(c.entombed)
        setCryptError(null)
      })
      .catch((e) => {
        setCrypt(null)
        setCryptError(String(e.message ?? e))
      })

  useEffect(() => {
    Promise.all([api.decks(), api.health()])
      .then(([d, h]) => {
        setDecks(d)
        setHealth(h)
      })
      .catch((e) => setError(String(e.message ?? e)))
    // Once, on arrival, and deliberately not in the `Promise.all` above: a
    // crypt that will not answer must not take the shelf down with it, and the
    // two failures are told apart on screen. Every later read is asked for by
    // something the player did.
    void loadCrypt()
  }, [])

  /** Raise one deck, then re-read both lists from the server.
   *
   * Re-fetched rather than patched in from the response: a deck tile carries
   * the gate's counts, its art and its colours, and reconstructing one here
   * would be this page inventing facts the shelf is the authority on. */
  async function returnDeck(id: string) {
    setReturning(id)
    setReturnError(null)
    try {
      await api.returnEntombed(id)
      setDeleted((current) => (current && current.cryptId === id ? null : current))
      const [tiles] = await Promise.all([api.decks(), loadCrypt()])
      setDecks(tiles)
    } catch (e) {
      setReturnError(String((e as Error).message ?? e))
      // Both lists are re-read even on a refusal, and the shelf is the one
      // that matters: the commonest refusal is "a deck of that name is already
      // on your shelf", and a sentence pointing at a deck the shelf is not
      // showing is a sentence nobody can act on.
      void loadCrypt()
      // Best effort, and the swallow is argued: a shelf that fails to refresh
      // stays exactly as it was, which is stale rather than false — and
      // replacing the refusal above with a fetch error would throw away the
      // one answer the player actually asked for.
      api.decks().then(setDecks).catch(() => undefined)
    } finally {
      setReturning(null)
    }
  }

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
    // The crypt is not a shelf of tiles and is rendered by itself; `shown` is
    // still computed so the filter row keeps its options while the crypt is
    // open, and so switching back does not blink.
    let list = shelf === 'players' ? players : mine
    if (bracket !== 'all') list = list.filter((d) => String(d.bracket) === bracket)
    if (color !== 'all') list = list.filter((d) => d.color_identity.includes(color))
    if (status !== 'all') list = list.filter((d) => d.status === status)
    if (stage !== 'all') list = list.filter((d) => d.stage === stage)
    // 'none' is a real answer — the untagged decks — not the filter off.
    if (pilot !== 'all') {
      list = list.filter((d) => (pilot === 'none' ? !d.pilot : d.pilot === pilot))
    }
    return [...list].sort((a, b) =>
      sort === 'bracket'
        ? (b.bracket ?? 0) - (a.bracket ?? 0)
        : sort === 'size'
          ? b.total_cards - a.total_cards
          : a.name.localeCompare(b.name),
    )
  }, [mine, players, shelf, bracket, color, status, stage, pilot, sort])

  /** The browse shelf, grouped under the username it belongs to.
   *
   * Alphabetical by owner, and by whatever `sort` says within each — the deck
   * controls keep working inside a group rather than being overridden by it.
   */
  const byOwner = useMemo(() => {
    const out = new Map<string, DeckTile[]>()
    for (const deck of shown) {
      const bucket = out.get(deck.owner) ?? []
      bucket.push(deck)
      out.set(deck.owner, bucket)
    }
    return [...out.entries()].sort((a, b) => a[0].localeCompare(b[0]))
  }, [shown])

  if (error) return <ErrorNote>Could not load decks: {error}</ErrorNote>
  if (!decks) return <Spinner label="Loading decks…" />

  const brackets = [...new Set(decks.map((d) => d.bracket).filter(Boolean))].sort()
  // The household's names, straight off the decks (second punch list,
  // item 10): the filter exists only once somebody has tagged a pilot, so a
  // single-player library never grows a control about nobody.
  const pilots = [...new Set(decks.map((d) => d.pilot).filter(Boolean))].sort()

  /**
   * The strip's tabs, built rather than listed, because two of the three are
   * conditional and for the same reason: a tab about nobody is exactly the
   * "something in the way" ADR 22 asked the browse view not to be.
   *
   * The crypt appears when there is something in it — or when the crypt could
   * not be read, which is a *different* answer and gets a tab with **no
   * count**: a zero there would be a claim that nothing is buried, made by a
   * page that does not know.
   *
   * **And whenever the player is standing in it**, empty or not, which is not
   * symmetry for its own sake: returning the last buried deck empties the
   * crypt, and without this the strip vanished under somebody who was still
   * looking at the room — leaving "nothing rests here" on screen with no way
   * back to the shelf. A tab you are standing on is never hidden.
   */
  const tabs: [Shelf, string, number | null][] = [['mine', 'My decks', mine.length]]
  if (players.length > 0) tabs.push(['players', 'Other players', players.length])
  if (crypt === null) tabs.push(['crypt', 'Crypt', null])
  else if (crypt.length > 0 || shelf === 'crypt') tabs.push(['crypt', 'Crypt', crypt.length])

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
              : 'no card pool yet — the library awaits its first stocking'}
          </p>
        </div>
      )}

      {mine.length > 0 && <TheShelves />}

      {/* Offered only when there is more than one place to be. On a laptop
          nobody has shared with and nobody has deleted from, a strip here
          would be one tab labelling itself — exactly the "something in the
          way" ADR 22 asked the browse view not to be.

          `flex-wrap` for the reason `DeckDetail`'s strip carries it: the day a
          third shelf or a longer word arrives they wrap onto a second row
          instead of hanging off the right and making the whole page scroll
          sideways. That day is here — the crypt is the third — and the one
          word is why nothing had to change for it. */}
      {tabs.length > 1 && (
        <div role="tablist" aria-label="Whose decks"
             className="flex flex-wrap gap-1 border-b" style={{ borderColor: 'var(--hairline)' }}>
          {tabs.map(([key, label, count]) => (
            <button key={key} role="tab" aria-selected={shelf === key}
                    onClick={() => setShelf(key)}
                    className={`strip-tab -mb-px border-b-2 px-3 py-2 text-sm font-medium${
                      shelf === key ? ' is-active' : ''}`}>
              {label}
              {/* No count on a crypt nobody could read: a number here would be
                  a claim about how many decks are buried, and the honest
                  answer is that we do not know. */}
              {count !== null && (
                <span className="ml-1.5 text-xs tabular" style={{ color: 'var(--text-muted)' }}>
                  {count}
                </span>
              )}
            </button>
          ))}
        </div>
      )}

      {/* Absent over the crypt, and that is not tidiness: a row of controls
          that does nothing to what is on screen is a control lying about its
          own reach. Bracket and colour filter *tiles*, and the crypt shows
          none. */}
      {shelf !== 'crypt' && (
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
          {pilots.length > 0 && (
            <Select label="Pilot" value={pilot} onChange={setPilot}
                    options={[{ value: 'all', label: 'Everyone' },
                      ...pilots.map((p) => ({ value: p, label: p })),
                      { value: 'none', label: 'Untagged' }]} />
          )}
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
      )}

      {/* The moment after a deletion, and the whole reason the leak existed:
          somebody who has just buried a deck wants to know it is not gone. It
          used to be told the path. It is told the place now — and handed the
          way back, so the commonest recovery is one click and never a
          conversation about folders. */}
      {deleted && (
        <div role="status"
             className="card-surface flex flex-wrap items-center gap-2 rounded-lg px-4 py-3 text-sm"
             style={{ color: 'var(--text-secondary)' }}>
          <span aria-hidden style={{ color: 'var(--text-muted)' }}>⚰</span>
          <span>
            <strong>{deleted.name}</strong> rests in your crypt — entombed, not
            erased. {deleted.cryptId
              ? 'Bring it back whenever you like.'
              : 'You can raise it again from the Crypt tab.'}
          </span>
          {/* Rendered here only when the crypt itself is not on screen, so a
              refusal is said once rather than in two places at once. */}
          {returnError && shelf !== 'crypt' && (
            <span className="basis-full"><ErrorNote>{returnError}</ErrorNote></span>
          )}
          <span className="ml-auto flex items-center gap-2">
            {deleted.cryptId && (
              <button
                onClick={() => void returnDeck(deleted.cryptId)}
                disabled={returning !== null}
                title={`Return ${deleted.name} to your library, exactly as it was`}
                className="btn btn-quiet btn-xs">
                {returning === deleted.cryptId ? 'Returning…' : 'Return it'}
              </button>
            )}
            <button onClick={() => { setDeleted(null); setReturnError(null) }}
                    className="btn btn-ghost btn-xs">
              Dismiss
            </button>
          </span>
        </div>
      )}

      {shelf === 'crypt' ? (
        <TheCrypt entombed={crypt} error={cryptError} busy={returning}
                  failed={returnError} onReturn={(id) => void returnDeck(id)} />
      ) : shelf === 'mine' && mine.length === 0 ? (
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
          onDeleted={(cryptId) => {
            // Drop it from the list here rather than re-fetching: the server
            // has already confirmed, and a round trip would leave the deck on
            // screen for a beat after it was gone.
            //
            // Matched on the whole address, not the slug: two owners may have
            // a `goreclaw` and only one of them was deleted.
            setDecks((current) => (current ?? []).filter(
              (d) => d.slug !== deleting.slug || d.owner !== deleting.owner))
            setDeleted({ name: deleting.name, cryptId })
            setDeleting(null)
            // The crypt just gained an entry, and the tab that shows it is
            // built from this list.
            void loadCrypt()
          }}
        />
      )}
    </div>
  )
}
