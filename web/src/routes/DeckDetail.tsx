import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  api,
  errorMessage,
  type Card,
  type ClaudeStatus,
  type CommanderDossier,
  type DeckDetail as Deck,
  type DeckRef,
  type DeckStats,
  type Suggestions,
  type ValidationReport,
} from '../lib/api'
import { categoryLabel, identityName } from '../lib/mtg'
import {
  Badge, CardArt, CardHover, Caveat, ColorRing, ErrorNote, ManaCost, ManaText,
  Select, Spinner, StatTile,
} from '../components/ui'
import {
  CategoryCoverage, ColorNeedsChart, CurveChart, DataTable,
} from '../components/charts'
import {
  AddCardForm, AddNoteForm, NoteEditor, RationaleEditor, SlotArgumentPanel,
} from '../components/deckedit'
import { ArtPicker } from '../components/artpicker'
import { CommanderDossierPanel } from '../components/dossier'
import { SwapComposer } from '../components/swap'
import { StanceReadout } from '../components/stance'
import { effectivePin, fetchClaudeStatus, useStance } from '../lib/stance'

type Tab = 'cards' | 'stats' | 'validation' | 'notes'

/**
 * Put this deck on display to the other accounts on this instance, or take it
 * off (ADR 22).
 *
 * Shown only to the owner, and it is the only control in the app that changes
 * who can see something — so it says what it will *do* rather than what the
 * deck currently *is*, and the current state is the sentence underneath. A
 * button labelled with a state is the one people click expecting to select it.
 *
 * Nothing here is a permission model: sharing makes a deck readable by anybody
 * signed in to this instance and by nobody else. The wording avoids "public"
 * for exactly that reason — there are no unlisted links and nothing outside the
 * auth boundary, which ADR 22 rejected on purpose.
 */
function ShareToggle({ deck, deckRef, onChanged }: {
  deck: Deck
  deckRef: DeckRef
  onChanged: () => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function toggle() {
    setBusy(true)
    setError(null)
    try {
      await api.setShared(deckRef, !deck.shared)
      onChanged()
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <span className="flex flex-col gap-1">
      {/* Same size and weight as its row siblings — this button sits between
          the simulate link and the art picker, and it used to be the odd one
          out in all three of padding, weight and vertical position. */}
      <button onClick={() => void toggle()} disabled={busy}
              className="rounded-lg px-4 py-2 text-sm font-medium disabled:opacity-40"
              style={{ border: '1px solid var(--hairline)',
                       background: 'var(--page)',
                       color: 'var(--text-secondary)' }}>
        {busy ? 'Saving…' : deck.shared ? 'Make private' : 'Share this deck'}
      </button>
      <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
        {deck.shared
          ? 'Anyone signed in here can read it.'
          : 'Only you can see it.'}
      </span>
      {error && <span className="text-xs" style={{ color: 'var(--status-critical)' }}>
        {error}
      </span>}
    </span>
  )
}

/**
 * The deck's header: who leads it, what that card says, and who they are.
 *
 * Three shapes were tried here and the middle one is why the third looks like
 * this. `art_crop` is 626x457 — about 1.37:1 — so a full-bleed band across a
 * wide page can only show a horizontal slice of the painting, and the first
 * version's 1200/260 band kept under a third of the height out of the middle:
 * on a commander drawn head-up, the head was what got cropped. The second
 * version reacted by throwing the band away and blurring it into a colour
 * wash, which fixed the cropping by removing the picture — sterile, and it
 * gave up the thing that made the page feel like a deck rather than a
 * spreadsheet.
 *
 * So: the band is back and it is the centrepiece, framed at `center 22%`
 * because card art is composed with its subject high far more often than low,
 * and tall enough (1200/300) to keep a real slice rather than a stripe. The
 * card itself is no longer *in* the band — it sits beside the text underneath,
 * whole and unclipped, which is what it was colliding with before.
 *
 * The text is the other half. A page that named its commander and showed
 * nothing else made the reader hover a thumbnail to find out what the card
 * does. The identity leads, lettered and large, because it is the fact that
 * governs the other 99; then the printed cost, type line and stats; then the
 * oracle text; then flavour, which belongs to a printing and which four of
 * these six commanders do not have.
 *
 * Everything below the fold is `CommanderDossier`, and every number in it was
 * counted over the pool by `service.commander_dossier`. That is rule 1 in
 * the place it would have been easiest to ignore: "one of eight legendary
 * Trolls" is exactly the sentence a model writes fluently and wrongly.
 */
function DeckHero({ deck, deckRef, report, dossier, claude, onRefresh }: {
  deck: Deck
  /** The deck's address. Taken from the URL rather than rebuilt from `deck`,
   *  so every call this page makes is aimed at the deck the reader asked for
   *  even if the payload's own `owner` ever disagreed. */
  deckRef: DeckRef
  report: ValidationReport
  dossier: CommanderDossier | null
  /** Null until known. `stance.axes[0].level === 'off'` is what decides
   *  whether the dossier offers a button at all — ADR 15's "off is a real
   *  position" rendered as an absent control rather than a refusing one. */
  claude: ClaudeStatus | null
  onRefresh: () => void
}) {
  // Read here rather than passed down: the pin lives outside React (see
  // `lib/stance.ts`), so this and the fetch in the parent see one value
  // without the prop having to exist.
  const [pin] = useStance()
  const card = deck.commander_card
  const stats = card?.power != null && card?.toughness != null
    ? `${card.power}/${card.toughness}`
    : null

  return (
    <div className="card-surface overflow-hidden rounded-xl">
      {/* Decorative, and marked so: the commander's name is already the
          heading and the card's own alt text below. */}
      {card?.art_crop && (
        <div className="deck-hero-band relative" aria-hidden>
          {/* `top`, not a percentage. The band is 4:1 over a 1.37:1 crop, so
              it can only show about a third of the painting's height and the
              only question is which third. Centring took it out of the
              middle and decapitated head-up compositions; 22% still shaved
              the tops of heads. Anchoring to the top cannot clip upward at
              all — it gives up the bottom instead, which on card art is
              ground, robes and negative space far more often than it is the
              subject. */}
          <CardArt src={card.art_crop} alt="" ratio="aspect-[1200/300]"
                   className="rounded-none" position="center top" />
          <div className="deck-hero-scrim" />
        </div>
      )}

      <div className="relative flex flex-col gap-6 p-5 sm:flex-row sm:p-6">
        {card?.image && (
          <CardHover card={card}>
            {/* Beside the text, not lifted into the band. Overlapping the art
                was what "the card preview is clipped by the hero art" meant,
                and there is no negative margin here for that reason. */}
            <img src={card.image} alt={card.name}
                 className="w-40 shrink-0 cursor-help self-start rounded-xl sm:w-44"
                 style={{ boxShadow: '0 14px 40px rgba(0,0,0,0.45)' }} />
          </CardHover>
        )}

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">{deck.name}</h1>
            {deck.bracket && <Badge>Bracket {deck.bracket}</Badge>}
            {deck.status === 'theoretical' && <Badge>theory</Badge>}
            {deck.stage === 'draft' && <Badge tone="warning">draft</Badge>}
            {report.ok
              ? <Badge tone="good">valid</Badge>
              : <Badge tone="critical">{report.errors.length} error(s)</Badge>}
          </div>

          {/* Whose deck this is, and only when it is not the reader's. On your
              own deck it would be a line telling you your own username. */}
          {!deck.writable && (
            <p className="mt-2 text-sm" style={{ color: 'var(--text-muted)' }}>
              Shared by <span style={{ color: 'var(--text-secondary)' }}>{deck.owner}</span>
              {' '}— you can read this deck, not change it.
            </p>
          )}

          {/* The identity, loud — it constrains all 99 other cards and it used
              to be a row of 12px dots. The mana cost deliberately sits on the
              card's line below instead of next to this: both are rows of
              coloured pips, and side by side they asked the reader to tell
              "this deck may play white" from "this costs two white" by pip
              size alone. */}
          <div className="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2">
            <ColorRing colors={deck.color_identity} size={28} />
            <span className="text-lg font-medium">
              {identityName(deck.color_identity)}
            </span>
            <span className="text-xs uppercase tracking-wide"
                  style={{ color: 'var(--text-muted)' }}>
              colour identity
            </span>
          </div>

          <p className="mt-2 flex flex-wrap items-center gap-x-2 text-sm"
             style={{ color: 'var(--text-secondary)' }}>
            <span className="font-medium" style={{ color: 'var(--text-primary)' }}>
              {deck.commander.join(' & ')}
            </span>
            {card?.mana_cost && <ManaCost cost={card.mana_cost} size={16} />}
            {card?.type_line && <span>{card.type_line}</span>}
            {stats && <span className="tabular">{stats}</span>}
            {deck.companion && <span>· companion: {deck.companion}</span>}
          </p>

          {card?.oracle_text && (
            <p className="mt-3 max-w-2xl whitespace-pre-line text-sm leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              <ManaText>{card.oracle_text}</ManaText>
            </p>
          )}

          {card?.flavor_text && (
            <p className="mt-3 max-w-2xl border-l-2 pl-3 text-sm italic leading-relaxed"
               style={{ borderColor: 'var(--baseline)', color: 'var(--text-muted)' }}>
              {card.flavor_text}
            </p>
          )}

          {/* `items-start`, not `items-center`: the share toggle is a button
              with a status caption underneath, and centring the pair made its
              button ride higher than every sibling. Top-aligned, the buttons
              share a line and the caption hangs below on its own. */}
          <div className="mt-5 flex flex-wrap items-start gap-3">
            <Link to={`/simulate?owner=${encodeURIComponent(deckRef.owner)}`
                     + `&deck=${encodeURIComponent(deckRef.slug)}`}
                  className="rounded-lg px-4 py-2 text-sm font-medium"
                  style={{ background: 'var(--series-1)', color: '#fff' }}>
              Simulate this deck
            </Link>
            {/* Every control below is hidden rather than disabled when the
                deck is not this viewer's to change. Disabled would be honest
                about the shape of the app but would offer a row of dead
                buttons to somebody who is only ever going to read; the
                server refuses either way. */}
            {deck.writable && <ShareToggle deck={deck} deckRef={deckRef}
                                           onChanged={onRefresh} />}
            {deck.writable && <ArtPicker deck={deckRef} onPicked={onRefresh} />}
            {card?.artist && (
              <span className="self-center text-xs" style={{ color: 'var(--text-muted)' }}>
                {/* The artist belongs to the printing being shown, so when a
                    deck has picked one, name it. Otherwise two decks on the
                    same commander credit the same painter for different
                    paintings. */}
                Art by {card.artist}
                {card.printing?.set_name && ` · ${card.printing.set_name}`}
              </span>
            )}
          </div>
        </div>
      </div>

      {dossier && <CommanderFacts dossier={dossier} />}

      {/* Below the counted strip, deliberately and permanently. The order on
          the page is the order of authority: what was counted, then what was
          read and written. ADR 19's "the page shows the seams" is this
          adjacency plus the label the panel carries. */}
      {/* Gated on the deck's own commander list, not on `commander_card`.
          The dossier is about the character, and the deck knows its name
          without a card pool — on a fresh clone the panel should still say what
          it is rather than vanish along with the card row. */}
      {deck.commander[0] && (
        <CommanderDossierPanel
          deck={deckRef}
          commander={deck.commander[0]}
          stance={effectivePin(pin, claude)}
          canGenerate={
            !!claude?.installed && !!claude?.configured
            && claude.stance.axes[0]?.level !== 'off'
          }
        />
      )}

      {/* Under the dossier rather than over it, and only where Claude is
          actually reachable. The control itself lives in the header now
          (`StanceMenu`); this line says what that setting resolved to for
          *this* deck, which the header cannot know. */}
      {claude?.installed && claude.configured && (
        <div className="px-5 pb-4 sm:px-6">
          <StanceReadout status={claude} pin={pin} />
        </div>
      )}
    </div>
  )
}

/**
 * The strip of pool facts under the header.
 *
 * Renders nothing at all when it has nothing to say — a deck on a clone with
 * no card pool, or a commander whose type line has no subtypes and whose name
 * matches no other card. An empty "About" heading is worse than no heading.
 */
function CommanderFacts({ dossier }: { dossier: CommanderDossier }) {
  const { subtypes, other_cards: others, printings } = dossier
  const first = printings?.first_released
    ? new Date(printings.first_released).getUTCFullYear()
    : null
  if (!subtypes.length && !others.length && !first) return null

  return (
    <div className="border-t px-5 py-4 sm:px-6"
         style={{ borderColor: 'var(--hairline)' }}>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-2">
        <span className="text-xs font-medium uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          About this commander
        </span>

        {/* Read as a sentence rather than as a stat block: "a Troll, one of 8
            legendary ones" is a fact you can feel the size of, where "Troll:
            8" is a number you have to interpret. */}
        {subtypes.map((s) => (
          <span key={s.name} className="text-sm"
                style={{ color: 'var(--text-secondary)' }}>
            <span className="font-medium" style={{ color: 'var(--text-primary)' }}>
              {s.name}
            </span>
            <span style={{ color: 'var(--text-muted)' }}>
              {' '}— {s.legendary.toLocaleString()} legendary,{' '}
              {s.total.toLocaleString()} in all
            </span>
          </span>
        ))}

        {first && (
          <span className="text-sm" style={{ color: 'var(--text-secondary)' }}>
            First printed{' '}
            <span className="font-medium" style={{ color: 'var(--text-primary)' }}>
              {first}
            </span>
            {printings?.first_set && (
              <span style={{ color: 'var(--text-muted)' }}> in {printings.first_set}</span>
            )}
            {printings && printings.count > 1 && (
              <span style={{ color: 'var(--text-muted)' }}>
                {' '}· {printings.count} printings
              </span>
            )}
          </span>
        )}
      </div>

      {others.length > 0 && (
        <div className="mt-3">
          <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
            {/* Deliberately hedged. The match is on the name, so this finds
                the character's other cards and also anything that merely
                shares the word — offered to look at, not asserted. */}
            Other cards carrying this name — the same character, printed again
            with different mechanics:
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            {others.map((o) => (
              <CardHover key={o.name} card={{ name: o.name, image: o.image }}>
                <span className="flex cursor-help items-center gap-2 rounded-lg px-2 py-1.5 text-xs"
                      style={{ border: '1px solid var(--hairline)',
                               background: 'var(--page)' }}>
                  {o.art_crop && (
                    <img src={o.art_crop} alt="" loading="lazy"
                         className="h-7 w-12 rounded object-cover" />
                  )}
                  <span>
                    <span className="font-medium">{o.name}</span>
                    {o.mana_cost && (
                      <span className="ml-1 align-middle">
                        <ManaCost cost={o.mana_cost} size={12} />
                      </span>
                    )}
                  </span>
                </span>
              </CardHover>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

export default function DeckDetail() {
  const { owner = '', slug = '' } = useParams()
  // Memoised because it is a dependency of the effects below, and a fresh
  // object literal every render would re-run all of them on every keystroke
  // anywhere on the page — including the one that costs a card pool query.
  const deckRef = useMemo<DeckRef>(() => ({ owner, slug }), [owner, slug])
  const [deck, setDeck] = useState<Deck | null>(null)
  const [stats, setStats] = useState<DeckStats | null>(null)
  const [report, setReport] = useState<ValidationReport | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('cards')
  const [groupBy, setGroupBy] = useState('category')
  const [suggestions, setSuggestions] = useState<Suggestions | null>(null)
  // Fetched alongside the deck rather than with it, and never awaited with
  // it: the panel it fills is decorative and runs several extra pool
  // queries, so a slow one must not hold up the 99. A failure is silent for
  // the same reason — the deck is still perfectly usable without trivia.
  const [dossier, setDossier] = useState<CommanderDossier | null>(null)
  // Whether a Claude surface exists on this instance at all, and whether the
  // stance permits a call. Fetched once per deck; reaches no network itself.
  const [claude, setClaude] = useState<ClaudeStatus | null>(null)
  // The dial itself is in `DeckHero`, which subscribes to the same store. This
  // copy exists so the status fetch below can depend on the pin — and so it can
  // clear one this build no longer serves.
  const [pin, setPinFor] = useStance()
  const requested = useRef<string | null>(null)
  // The card the user is proposing to swap in. Null means no swap is being
  // composed; the composer itself (`SwapComposer`) owns the rationale text.
  const [swapping, setSwapping] = useState<{ out: string; into: string } | null>(null)
  // The card whose rationale is being written, and whether the list is filtered
  // down to the ones a draft still owes.
  const [editing, setEditing] = useState<string | null>(null)
  // Whether the editor about to open should ask Claude straight away. Set by
  // the "Ask Claude" control and cleared by "Write why", so opening the editor
  // the ordinary way still costs nothing.
  const [askNow, setAskNow] = useState(false)
  // The card whose slot is being argued (ADR 25). Its own state rather than a
  // mode of `editing`, because the two answer different questions and are
  // useful at the same time: the argument says whether to keep the card, the
  // editor is where you say why you did.
  const [arguing, setArguing] = useState<string | null>(null)
  const [onlyUnjustified, setOnlyUnjustified] = useState(false)
  // Two error slots, not one: a refused removal belongs next to the cards and a
  // refused promotion belongs next to the button that asked for it. Sharing one
  // renders the same sentence in both places.
  const [editError, setEditError] = useState<string | null>(null)
  const [promoteError, setPromoteError] = useState<string | null>(null)
  const [promoting, setPromoting] = useState(false)

  /** Re-read everything an edit invalidates: the list, the stats and the gate.
   *
   * Every write returns the gate's verdict already, but the page shows more
   * than the verdict — category counts, the curve, the shortlist — and a page
   * that displayed a stale curve next to a fresh gate would be worse than one
   * that took a second round trip. */
  async function refresh() {
    const [d, s, v] = await Promise.all([
      api.deck(deckRef), api.stats(deckRef), api.validate(deckRef),
    ])
    setDeck(d)
    setStats(s)
    setReport(v)
    requested.current = null
    setSuggestions(null)
  }

  async function saveRationale(name: string, why: string) {
    await api.setCardField(deckRef, name, 'why', why)
    await refresh()
    setEditing(null)
  }

  async function removeCard(name: string) {
    setEditError(null)
    try {
      await api.removeCard(deckRef, name)
      await refresh()
    } catch (e) {
      setEditError(errorMessage(e))
    }
  }

  async function promote() {
    setPromoteError(null)
    setPromoting(true)
    try {
      await api.setDeckField(deckRef, 'stage', 'curated')
      await refresh()
    } catch (e) {
      setPromoteError(errorMessage(e))
    } finally {
      setPromoting(false)
    }
  }

  useEffect(() => {
    setDeck(null)
    setError(null)
    Promise.all([api.deck(deckRef), api.stats(deckRef), api.validate(deckRef)])
      .then(([d, s, v]) => {
        setDeck(d)
        setStats(s)
        setReport(v)
      })
      .catch((e) => setError(errorMessage(e)))
    setDossier(null)
    api.commander(deckRef).then(setDossier).catch(() => setDossier(null))
    setSuggestions(null)
    requested.current = null
  }, [deckRef, owner, slug])

  // Its own effect, and keyed on the pin as well as the deck, because the
  // answer depends on both: `stance` in the response is the *resolved* stance,
  // and re-asking is how the dial's readout shows what a pin actually bought
  // after the deployment ceiling has had its say. Nothing here recomputes that
  // — the clamp has one implementation and it is `stance.clamp`.
  useEffect(() => {
    setClaude(null)
    // `owner` alongside `slug`: this route takes its deck as a query parameter
    // rather than a path segment, so the URL says nothing about whose it is.
    fetchClaudeStatus({ slug, owner }, pin, () => setPinFor(null))
      .then(setClaude).catch(() => setClaude(null))
  }, [owner, slug, pin, setPinFor])

  // Lazily, and only for the tab that shows them: building a shortlist means a
  // pool query per offending card, which is real work to do on a page most
  // visits never scroll to. A deck that passes the gate returns immediately.
  //
  // The ref, not the state, is what makes this one request. Guarding on
  // `suggestions` looks equivalent and is not: it stays null until the response
  // lands, so switching tabs twice in that window fires the query again.
  useEffect(() => {
    // Keyed on the whole address, not the slug: two owners may both have a
    // `goreclaw`, and a ref that only remembered the slug would serve one
    // person's shortlist on the other's deck.
    const key = `${owner}/${slug}`
    if (tab !== 'validation' || requested.current === key) return
    requested.current = key
    api.suggestions(deckRef).then(setSuggestions).catch(() => setSuggestions(null))
  }, [tab, deckRef, owner, slug])

  const groups = useMemo(() => {
    if (!deck) return []
    const out = new Map<string, Card[]>()
    const shown = onlyUnjustified
      ? deck.cards.filter((c) => !c.why.trim())
      : deck.cards
    for (const card of shown) {
      const key =
        groupBy === 'type'
          // Front face, before the em dash, last word: "Legendary Creature —
          // Cat Warrior" groups as "Creature". Every step can come back empty
          // on a card with no type line, so the chain is optional throughout
          // and lands on 'Unknown' rather than asserting a shape the pool does
          // not promise.
          ? (card.type_line?.split(' // ')[0]?.split('—')[0]?.trim()
              .split(' ').pop() || 'Unknown')
          : groupBy === 'mv'
            ? `MV ${card.cmc ?? 0}`
            : card.category
      if (!out.has(key)) out.set(key, [])
      out.get(key)!.push(card)
    }
    return [...out.entries()].sort((a, b) => b[1].length - a[1].length)
  }, [deck, groupBy, onlyUnjustified])

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

  // The same three-part answer the dossier button uses: installed, configured,
  // and not switched off. A control that appears and then refuses is worse
  // than one that is honestly absent (ADR 15).
  const claudeReady = !!claude?.installed && !!claude.configured
    && claude.stance.axes[0]?.level !== 'off'

  return (
    <div className="space-y-6">
      <DeckHero deck={deck} deckRef={deckRef} report={report} dossier={dossier}
                claude={claude}
                onRefresh={() => { void refresh() }} />

      {/* A draft is a to-do list with a number on it (ADR 13), so the number
          leads. The cards themselves are marked below, but the count is what
          says how far off `curated` is — and the artifacts stay shut until
          then, which is worth saying where someone would go looking. */}
      {deck.stage === 'draft' && (
        <div className="rounded-lg px-4 py-3 text-sm"
             style={{
               background: 'color-mix(in srgb, var(--status-warning) 12%, transparent)',
               border: '1px solid color-mix(in srgb, var(--status-warning) 40%, transparent)',
             }}>
          {deck.cards.length === 0 ? (
            <>
              {/* A brand-new deck from the create flow. Without this branch it
                  falls into "every card carries a rationale, nothing is
                  outstanding" and offers promotion — vacuously true of a deck
                  with no cards, and the server now refuses it. Say what is
                  actually true instead. */}
              <strong>Draft — this deck is empty.</strong>{' '}
              <span style={{ color: 'var(--text-secondary)' }}>
                Its commander and colour identity are set. Add the 99 next, from
                a decklist or a card at a time; each card will want a reason for
                its slot before this can become curated.
              </span>
              <div className="mt-2 flex flex-wrap gap-2">
                <Link to="/import"
                      className="rounded-lg px-3 py-1.5 text-xs font-medium"
                      style={{ background: 'var(--gridline)', color: 'var(--text-primary)' }}>
                  Paste a decklist
                </Link>
              </div>
            </>
          ) : deck.needs_rationale > 0 ? (
            <>
              <strong>
                Draft — {deck.needs_rationale} of {deck.cards.length} cards still
                need a <code>why</code>.
              </strong>{' '}
              <span style={{ color: 'var(--text-secondary)' }}>
                Its legality, colour identity and size are already checked. Write
                the rationales below and this becomes a promotion you can make
                here — the gate refuses it while any card is blank, and artifacts
                stay blocked until it lands.
              </span>
              <div className="mt-2">
                <button
                  onClick={() => { setTab('cards'); setOnlyUnjustified(true) }}
                  className="rounded-lg px-3 py-1.5 text-xs font-medium"
                  style={{ background: 'var(--gridline)', color: 'var(--text-primary)' }}>
                  Show the {deck.needs_rationale} that need one
                </button>
              </div>
            </>
          ) : (
            <>
              {/* The work is done and the deck does not know it yet. This is
                  the last step of an import, and until `set_deck_field` it was
                  the last thing in the lifecycle that needed a text editor. */}
              <strong>Draft — every card carries a rationale.</strong>{' '}
              <span style={{ color: 'var(--text-secondary)' }}>
                Nothing is outstanding. Promoting marks it curated and unblocks
                the five artifacts.
              </span>
              {deck.writable && (
                <div className="mt-2">
                  <button onClick={promote} disabled={promoting}
                          className="rounded-lg px-3 py-1.5 text-xs font-medium disabled:opacity-50"
                          style={{ background: 'var(--series-1)', color: '#fff' }}>
                    {promoting ? 'Promoting…' : 'Promote to curated'}
                  </button>
                </div>
              )}
            </>
          )}
          {promoteError && (
            <div className="mt-2"><ErrorNote>{promoteError}</ErrorNote></div>
          )}
        </div>
      )}

      {deck.strategy && (
        <p className="max-w-3xl text-sm leading-relaxed"
           style={{ color: 'var(--text-secondary)' }}>
          <ManaText>{deck.strategy}</ManaText>
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
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div className="flex flex-wrap items-end gap-4">
              <Select label="Group by" value={groupBy} onChange={setGroupBy}
                      options={[
                        { value: 'category', label: 'Category' },
                        { value: 'type', label: 'Card type' },
                        { value: 'mv', label: 'Mana value' },
                      ]} />
              {deck.needs_rationale > 0 && (
                <label className="flex items-center gap-2 pb-2 text-xs"
                       style={{ color: 'var(--text-secondary)' }}>
                  <input type="checkbox" checked={onlyUnjustified}
                         onChange={(e) => setOnlyUnjustified(e.target.checked)} />
                  Only the {deck.needs_rationale} needing a rationale
                </label>
              )}
            </div>
            {!deck.pool_available && (
              <span className="text-xs" style={{ color: 'var(--status-warning)' }}>
                No card pool — card text and art unavailable.
              </span>
            )}
          </div>

          {/* Announcing the feature, once, where the work is. The interview has
              existed since ADR 15 and the deck page never mentioned it — which
              is the finding that moved this whole branch to the front of the
              queue, so the fix is a sentence rather than a redesign. It says
              what the rule is at the same time, because the first question
              anybody asks about this is whether it writes the answer. */}
          {/* Also gated on `writable`: announcing a feature to somebody who
              cannot reach it is worse than not announcing it, and the whole
              point of this paragraph was that the interview was invisible. */}
          {deck.writable && claudeReady && (
            <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
              Stuck on a <code>why</code>? <strong>Ask Claude</strong> on any card
              and it will interview you about that slot — pool text, the gate’s
              verdict and the neighbours it competes with — to help you write the
              rationale. It asks; you answer. No setting lets it write the
              rationale for you.
            </p>
          )}

          {deck.writable && (
            <div className="flex flex-wrap items-center gap-3">
              <AddCardForm deck={deckRef} stage={deck.stage} onDone={() => void refresh()} />
            </div>
          )}
          {editError && <ErrorNote>{editError}</ErrorNote>}

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
                  <li key={card.name} className="card-surface rounded-lg p-2">
                   {/* `flex-wrap` plus the text column's `basis-52`: on a
                       phone the four action buttons cannot share a line with
                       the rationale, and without the wrap they crushed it to
                       a word per line and pushed Remove off-screen. The
                       basis is what makes the wrap deterministic — the text
                       claims 13rem before the buttons may have the rest. */}
                   <div className="flex flex-wrap items-center gap-3">
                    {/* The art crop, not the full card: at this size a whole
                        card scan is an unreadable smudge, while the art alone
                        is what the eye actually recognises a card by. Hover
                        still gives the full card for the text. */}
                    <CardHover card={card}>
                      <CardArt src={card.art_crop} alt={card.name}
                               ratio="aspect-[626/457]"
                               className="w-16 shrink-0 cursor-help" />
                    </CardHover>
                    <div className="min-w-0 flex-1 basis-52">
                      <div className="flex flex-wrap items-baseline gap-2">
                        <CardHover card={card}>
                          <span className="cursor-help text-sm font-medium">
                            {card.qty > 1 && <span className="tabular mr-1">{card.qty}×</span>}
                            {card.name}
                          </span>
                        </CardHover>
                        <ManaCost cost={card.mana_cost} />
                      </div>
                      {card.why ? (
                        <p className="mt-0.5 text-xs leading-relaxed"
                           style={{ color: 'var(--text-secondary)' }}>
                          <ManaText>{card.why}</ManaText>
                        </p>
                      ) : (
                        /* Where the draft's outstanding work actually is. The
                           gate reports the count; this is the per-card list,
                           read off the deck rather than off the report. */
                        <p className="mt-0.5 text-xs italic leading-relaxed"
                           style={{ color: 'var(--status-warning)' }}>
                          no rationale yet
                        </p>
                      )}
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {/* The discoverability fix, and the smallest honest
                          version of it. The rationale interview has worked
                          since ADR 15 and was reachable only by opening the
                          editor and finding a second button inside it, so
                          nothing on this page said it existed. This says it,
                          per card, where the work actually is — and it opens
                          the editor already asking, because clicking a control
                          that says "ask" and then being shown another one that
                          says "ask" is the same problem one layer down. */}
                      {/* The interview is gated with the writes even though it
                          only asks questions: its entire purpose is to help
                          somebody write a `why`, and offering that to a
                          reader who cannot save one would spend a Claude call
                          on a dead end. */}
                      {deck.writable && claudeReady && (
                        <button
                          onClick={() => {
                            setAskNow(true)
                            setEditing(card.name)
                          }}
                          title={`Claude interviews you about ${card.name}'s slot`
                                 + ' to help you write its why — it asks, you write'}
                          className="rounded-md px-2 py-1 text-[11px]"
                          style={{ border: '1px solid var(--hairline)',
                                   color: 'var(--text-secondary)' }}>
                          Ask Claude
                        </button>
                      )}
                      {/* Gated with the writes for the same reason the
                          interview is, and it is a spend argument rather than
                          a privacy one: arguing a slot on a deck you cannot
                          edit spends a call to reach a conclusion you cannot
                          act on. */}
                      {deck.writable && claudeReady && (
                        <button
                          onClick={() => setArguing(
                            arguing === card.name ? null : card.name)}
                          title={`The case against ${card.name}'s slot`}
                          className="rounded-md px-2 py-1 text-[11px]"
                          style={{ border: '1px solid var(--hairline)',
                                   color: 'var(--text-secondary)' }}>
                          Argue slot
                        </button>
                      )}
                      {deck.writable && (
                        <>
                          <button
                            onClick={() => {
                              setAskNow(false)
                              setEditing(editing === card.name ? null : card.name)
                            }}
                            className="rounded-md px-2 py-1 text-[11px] font-medium"
                            style={{ background: 'var(--gridline)',
                                     color: 'var(--text-primary)' }}>
                            {card.why ? 'Edit why' : 'Write why'}
                          </button>
                          <button
                            onClick={() => removeCard(card.name)}
                            title={`Remove ${card.name} from the deck`}
                            className="rounded-md px-2 py-1 text-[11px]"
                            style={{ color: 'var(--text-muted)' }}>
                            Remove
                          </button>
                        </>
                      )}
                    </div>
                   </div>
                   {arguing === card.name && (
                     <SlotArgumentPanel
                       deck={deckRef}
                       card={card.name}
                       writable={deck.writable}
                       onSwapped={() => { setArguing(null); void refresh() }}
                       onClose={() => setArguing(null)} />
                   )}
                   {editing === card.name && (
                     <RationaleEditor
                       deck={deckRef}
                       card={card}
                       askNow={askNow}
                       onSave={(why) => saveRationale(card.name, why)}
                       onCancel={() => setEditing(null)} />
                   )}
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
                  <ManaText>{issue.message}</ManaText>
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
                            className="flex flex-wrap items-center gap-3 rounded-lg p-2"
                            style={{ background: 'var(--surface-1)' }}>
                          {/* Same affordance as the decklist: the art is what
                              you recognise, the full card is what you need to
                              read before accepting a suggestion. */}
                          <CardHover card={{ name: c.name, image: c.image }}>
                            <CardArt src={c.art_crop} alt={c.name}
                                     ratio="aspect-[626/457]"
                                     className="w-16 shrink-0 cursor-help" />
                          </CardHover>
                          <div className="min-w-0 flex-1 basis-52">
                            <div className="flex flex-wrap items-baseline gap-2">
                              <span className="text-sm font-medium">{c.name}</span>
                              <ManaCost cost={c.mana_cost} />
                            </div>
                            <p className="mt-0.5 text-xs leading-relaxed"
                               style={{ color: 'var(--text-muted)' }}>
                              <ManaText>{c.reasons.join(' · ')}</ManaText>
                            </p>
                          </div>
                          {/* The shortlist itself stays visible — it is
                              analysis, and reading why a card is a candidate
                              is worth having without the power to act on it.
                              Only the swap is taken away. */}
                          {deck.writable && (
                            <button
                              onClick={() => setSwapping({ out: shortlist.card, into: c.name })}
                              className="shrink-0 rounded-lg px-3 py-1.5 text-xs font-medium"
                              style={{ background: 'var(--gridline)',
                                       color: 'var(--text-primary)' }}>
                              Use this card
                            </button>
                          )}
                        </li>
                      ))}
                    </ul>

                    {swapping?.out === shortlist.card && (
                      <SwapComposer
                        deck={deckRef}
                        out={swapping.out}
                        into={swapping.into}
                        onDone={async () => { await refresh(); setSwapping(null) }}
                        onCancel={() => setSwapping(null)}
                      />
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
              <ManaText>{issue.message}</ManaText>
            </div>
          ))}
        </div>
      )}

      {tab === 'notes' && (
        <div className="space-y-4">
          <Caveat>
            The deck's thinking, kept in <code>deck.yaml</code> rather than in an
            artifact — which is why regenerating the five deliverables cannot lose
            it. The advanced primer reads these keys directly.
          </Caveat>
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
                <NoteEditor deck={deckRef} noteKey={key} value={value}
                            writable={deck.writable}
                            onDone={() => void refresh()} />
              </section>
            ))}
          </div>
          {deck.writable && (
            <AddNoteForm deck={deckRef} existing={Object.keys(deck.notes)}
                         onDone={() => void refresh()} />
          )}
        </div>
      )}
    </div>
  )
}

