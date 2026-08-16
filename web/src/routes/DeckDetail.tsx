import {
  useCallback, useEffect, useMemo, useRef, useState, type ReactNode,
} from 'react'
import { Link, useParams } from 'react-router-dom'
import {
  api,
  errorMessage,
  type Card,
  type ClaudeStatus,
  type CommanderDossier,
  type DeckDetail as Deck,
  type DeckLogEntry,
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
import { ArtPicker, CardArtPicker } from '../components/artpicker'
import { CommanderDossierPanel } from '../components/dossier'
import { DeckReviewPanel } from '../components/review'
import { SwapComposer } from '../components/swap'
import { StanceReadout } from '../components/stance'
import { effectivePin, fetchClaudeStatus, useStance } from '../lib/stance'

type Tab = 'cards' | 'stats' | 'validation' | 'notes' | 'history'

/** What the action bar can do to a card (punch list item 9). */
type CardAction = 'ask' | 'argue' | 'why' | 'art' | 'entomb'

/** The bar's vocabulary: label, hint while armed, and whether it needs
 *  Claude. Entomb is the one destructive act and is styled apart. */
const CARD_ACTIONS: {
  key: CardAction
  label: string
  hint: string
  claude?: boolean
  danger?: boolean
}[] = [
  { key: 'why', label: 'Write why',
    hint: 'Pick a card to write or edit its rationale.' },
  // "Card art", not "Change art": the commander's own picker in the hero
  // already answers to that name, and two controls sharing it would make
  // either one unfindable — by a person or a test.
  { key: 'art', label: 'Card art',
    hint: 'Pick a card to choose which printing it shows.' },
  { key: 'ask', label: 'Ask Claude', claude: true,
    hint: 'Pick a card and Claude interviews you about its slot.' },
  { key: 'argue', label: 'Argue slot', claude: true,
    hint: 'Pick a card to hear the case against its slot.' },
  { key: 'entomb', label: 'Entomb', danger: true,
    hint: 'Pick a card to send to the graveyard — it will ask twice.' },
]

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

/**
 * A destructive control that arms on the first click and acts on the second.
 *
 * The confirmation structure ADR 27 chose over a dialog: one accidental click
 * does nothing except turn the button solid red for a few seconds, and a
 * deliberate deletion is still two quick clicks in place — no modal to dismiss
 * ten times while tuning a deck. The arm times out rather than staying
 * cocked, because a button left armed yesterday firing today is the mis-click
 * this exists to prevent.
 */
function ArmedButton({ children, armedLabel, title, onConfirm }: {
  children: ReactNode
  /** What the armed state says — name the consequence, not "are you sure". */
  armedLabel: string
  title?: string
  onConfirm: () => void
}) {
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    if (!armed) return
    const timer = setTimeout(() => setArmed(false), 4000)
    return () => clearTimeout(timer)
  }, [armed])
  return (
    <button
      type="button"
      title={title}
      aria-pressed={armed}
      className={'card-action card-action-danger rounded-md px-2 py-1 '
                 + `text-[11px] font-medium${armed ? ' armed' : ''}`}
      onClick={() => {
        if (armed) {
          setArmed(false)
          onConfirm()
        } else {
          setArmed(true)
        }
      }}
    >
      {armed ? armedLabel : children}
    </button>
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
  // What has been done to this deck (ADR 28). `null` is "not asked yet", which
  // the panel renders as loading; an empty array is the real answer for a deck
  // whose every edit predates the log, and those two must not look the same.
  const [history, setHistory] = useState<DeckLogEntry[] | null>(null)
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
  // The armed action, if any (punch list 2026-08-15 item 9). The four
  // per-row buttons became one bar above the 99: press an action, then pick
  // the card it applies to — which is the same interaction inverted, and it
  // gives every row back the width four buttons were eating. Escape or the
  // bar's own button cancels.
  const [action, setAction] = useState<CardAction | null>(null)
  // The card whose art picker is open (item 8) — set through the action bar.
  const [artFor, setArtFor] = useState<string | null>(null)
  // Entomb through the bar keeps ADR 27's two-step: the first pick arms the
  // row, the second within four seconds commits it.
  const [pendingEntomb, setPendingEntomb] = useState<string | null>(null)
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
    // Only if it has been looked at: an edit adds an entry, and a panel the
    // user is watching must not go stale behind them. Untouched, this stays
    // the lazy fetch the tab does on first open.
    if (historyFor.current) loadHistory()
  }

  async function saveRationale(name: string, why: string) {
    await api.setCardField(deckRef, name, 'why', why)
    await refresh()
    setEditing(null)
  }

  // Escape leaves the action mode from anywhere; the arm on a pending entomb
  // times out on its own, exactly as ArmedButton's does.
  useEffect(() => {
    if (!action) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setAction(null)
        setPendingEntomb(null)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [action])

  useEffect(() => {
    if (!pendingEntomb) return
    const timer = setTimeout(() => setPendingEntomb(null), 4000)
    return () => clearTimeout(timer)
  }, [pendingEntomb])

  /** The armed action lands on a card. Everything but entomb disarms the bar
   *  on the first pick; entomb keeps ADR 27's second click. */
  function actOn(name: string) {
    if (!action) return
    if (action === 'ask') {
      setAskNow(true)
      setEditing(name)
      setAction(null)
    } else if (action === 'why') {
      setAskNow(false)
      setEditing(editing === name ? null : name)
      setAction(null)
    } else if (action === 'argue') {
      setArguing(arguing === name ? null : name)
      setAction(null)
    } else if (action === 'art') {
      setArtFor(artFor === name ? null : name)
      setAction(null)
    } else if (pendingEntomb === name) {
      setPendingEntomb(null)
      setAction(null)
      void entombCard(name)
    } else {
      setPendingEntomb(name)
    }
  }

  /** Rows currently sinking into the graveyard — they get the send-off class
   *  and lose their pointer events while the server is asked. */
  const [leaving, setLeaving] = useState<Set<string>>(new Set())
  // Bulk entombment (ADR 27): a mode you enter, not checkboxes always there —
  // 99 permanent checkboxes would make every visit look like a form.
  const [selecting, setSelecting] = useState(false)
  const [chosen, setChosen] = useState<Set<string>>(new Set())

  /** The animation's length. Awaited alongside the API call so the row is
   *  never yanked out mid-sink: whichever finishes last ends the rite. */
  const RITE = 360

  async function entombCard(name: string) {
    setEditError(null)
    setLeaving((prev) => new Set(prev).add(name))
    const rite = new Promise((r) => setTimeout(r, RITE))
    try {
      await api.entombCard(deckRef, name)
      await rite
      await refresh()
    } catch (e) {
      setEditError(errorMessage(e))
    } finally {
      setLeaving((prev) => {
        const next = new Set(prev)
        next.delete(name)
        return next
      })
    }
  }

  async function entombChosen() {
    const names = [...chosen]
    if (!names.length) return
    setEditError(null)
    setLeaving((prev) => new Set([...prev, ...names]))
    const rite = new Promise((r) => setTimeout(r, RITE))
    try {
      // One request, one write, one gate verdict — and all-or-nothing on the
      // server, so a partial sweep can never read as a chosen deck state.
      await api.entombCards(deckRef, names)
      await rite
      await refresh()
      setChosen(new Set())
      setSelecting(false)
    } catch (e) {
      setEditError(errorMessage(e))
    } finally {
      setLeaving(new Set())
    }
  }

  async function returnCard(name: string) {
    setEditError(null)
    try {
      await api.returnCard(deckRef, name)
      await refresh()
    } catch (e) {
      setEditError(errorMessage(e))
    }
  }

  async function exileCard(name: string) {
    setEditError(null)
    try {
      await api.exileCard(deckRef, name)
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
    // The history belongs to *this* deck. Cleared here as well as keyed by
    // address below, so a slow fetch for the deck being left cannot land in
    // the panel of the deck being opened.
    setHistory(null)
    historyFor.current = null
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

  // The history (ADR 28), on the same terms as the shortlist above: lazily,
  // for the tab that shows it, keyed on the whole address because two owners
  // may both have a `goreclaw`.
  //
  // Its own ref rather than sharing `requested`, and that is not tidiness: one
  // ref holding one key would make opening Validation and then History serve
  // whichever was asked for second and cancel the other. `history` also has to
  // be re-fetched after an edit, which is a thing the shortlist never does.
  const historyFor = useRef<string | null>(null)
  const loadHistory = useCallback(() => {
    historyFor.current = `${owner}/${slug}`
    api.deckLog(deckRef).then((l) => setHistory(l.entries))
      .catch(() => setHistory([]))
  }, [deckRef, owner, slug])

  useEffect(() => {
    if (tab !== 'history' || historyFor.current === `${owner}/${slug}`) return
    loadHistory()
  }, [tab, owner, slug, loadHistory])

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
    { id: 'history', label: 'History' },
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
              {/* Bulk entombment is a mode, not a standing row of checkboxes:
                  entering it is the first deliberate step, the armed button
                  naming the count is the second, so a sweep is never one
                  stray click. */}
              <button
                type="button"
                onClick={() => {
                  setSelecting(!selecting)
                  setChosen(new Set())
                  setAction(null)
                  setPendingEntomb(null)
                }}
                className="card-action rounded-md px-2 py-1 text-[11px] font-medium"
              >
                {selecting ? 'Done selecting' : 'Bulk entomb…'}
              </button>
              {selecting && (
                chosen.size > 0
                  ? <ArmedButton
                      armedLabel={`Entomb ${chosen.size} — sure?`}
                      title="One write: the chosen cards go to the graveyard together"
                      onConfirm={() => void entombChosen()}>
                      Entomb {chosen.size} selected
                    </ArmedButton>
                  : <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
                      Tick the cards to entomb.
                    </span>
              )}
            </div>
          )}

          {/* The action bar (punch list item 9): the four per-row buttons,
              rolled up and inverted. Press an action, then pick the card it
              applies to — one bar instead of four buttons times 99 rows, and
              every row gets its width back. Escape or the pressed button
              itself cancels. */}
          {deck.writable && !selecting && (
            <div className="action-bar flex flex-wrap items-center gap-2 rounded-lg px-3 py-2">
              <span className="text-[10px] uppercase tracking-wide"
                    style={{ color: 'var(--text-muted)' }}>
                Card actions
              </span>
              {CARD_ACTIONS
                .filter((a) => !a.claude || claudeReady)
                .map((a) => (
                  <button
                    key={a.key}
                    type="button"
                    aria-pressed={action === a.key}
                    onClick={() => {
                      setAction(action === a.key ? null : a.key)
                      setPendingEntomb(null)
                    }}
                    className={`card-action${a.danger ? ' card-action-danger' : ''}`
                               + ' rounded-md px-2.5 py-1 text-[11px] font-medium'}
                  >
                    {a.label}
                  </button>
                ))}
              {action && (
                <span className="action-hint text-xs"
                      style={{ color: 'var(--text-secondary)' }}>
                  {CARD_ACTIONS.find((a) => a.key === action)?.hint}{' '}
                  <button type="button"
                          onClick={() => { setAction(null); setPendingEntomb(null) }}
                          className="underline"
                          style={{ color: 'var(--text-muted)' }}>
                    Cancel
                  </button>
                </span>
              )}
            </div>
          )}
          {/* The sweep. Gated exactly as the per-card argue is — it is the
              same mode, multiplied — and mounted as its own section because
              its selection list and results queue do not fit a button row. */}
          {deck.writable && claudeReady && (
            <DeckReviewPanel deck={deck} deckRef={deckRef} status={claude}
                             onChanged={() => void refresh()} />
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
                  <li key={card.name}
                      className={'card-surface rounded-lg p-2'
                                 + (leaving.has(card.name) ? ' entombing' : '')
                                 + (action && deck.writable ? ' action-pick' : '')
                                 + (pendingEntomb === card.name ? ' is-pending-entomb' : '')}>
                   {/* In action mode the whole row is the pick target — the
                       four buttons that used to sit here live in the action
                       bar above the list now (punch list item 9). */}
                   <div className="flex flex-wrap items-center gap-3"
                        {...(action && deck.writable
                          ? {
                              role: 'button' as const,
                              tabIndex: 0,
                              'aria-label': `${CARD_ACTIONS.find((a) => a.key === action)?.label}: ${card.name}`,
                              onClick: () => actOn(card.name),
                              onKeyDown: (e: React.KeyboardEvent) => {
                                if (e.key === 'Enter' || e.key === ' ') {
                                  e.preventDefault()
                                  actOn(card.name)
                                }
                              },
                            }
                          : {})}>
                    {selecting && deck.writable && (
                      <input
                        type="checkbox"
                        aria-label={`Choose ${card.name} for entombment`}
                        checked={chosen.has(card.name)}
                        onChange={() => setChosen((prev) => {
                          const next = new Set(prev)
                          if (next.has(card.name)) next.delete(card.name)
                          else next.add(card.name)
                          return next
                        })}
                      />
                    )}
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
                    {/* The four buttons that lived here are the action bar
                        now. What remains per row is state, not controls: the
                        armed entomb naming its second click, and a chip when
                        the card wears a chosen printing. */}
                    {pendingEntomb === card.name && (
                      <span className="shrink-0 rounded-md px-2 py-1 text-[11px] font-medium"
                            style={{ background: 'var(--status-critical)', color: '#fff' }}>
                        Click again to entomb
                      </span>
                    )}
                    {!!card.art && (
                      <span className="shrink-0 text-[10px] uppercase tracking-wide"
                            title="This deck shows a chosen printing for this card"
                            style={{ color: 'var(--text-muted)' }}>
                        ✦ chosen art
                      </span>
                    )}
                   </div>
                   {artFor === card.name && (
                     <CardArtPicker
                       deck={deckRef}
                       card={card.name}
                       onPicked={() => void refresh()}
                       onClose={() => setArtFor(null)} />
                   )}
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

          {/* The graveyard (ADR 27). Rendered for readers too — an entombed
              card is deck state, not a private undo buffer — but the two ways
              out of it are the owner's. Absent entirely while empty, which is
              its normal state. */}
          {deck.graveyard.length > 0 && (
            <section className="space-y-2 border-t pt-4"
                     style={{ borderColor: 'var(--hairline)' }}>
              <h3 className="flex items-baseline gap-2 text-sm font-semibold">
                <span aria-hidden>⚰</span> Graveyard
                <span className="tabular text-xs font-normal"
                      style={{ color: 'var(--text-muted)' }}>
                  {deck.graveyard.length}
                </span>
              </h3>
              <p className="max-w-2xl text-xs" style={{ color: 'var(--text-muted)' }}>
                Entombed from the 99, rationale and all. Return puts a card
                back exactly as it left; exile is for good.
              </p>
              <ul className="space-y-1">
                {deck.graveyard.map((card) => (
                  <li key={card.name} className="card-surface rounded-lg p-2"
                      style={{ opacity: 0.85 }}>
                    <div className="flex flex-wrap items-center gap-3">
                      {/* Desaturated by the wrapper, not the art component:
                          a dead card's art dims with the row. */}
                      <CardHover card={card}>
                        <span className="shrink-0" style={{ filter: 'grayscale(0.7)' }}>
                          <CardArt src={card.art_crop} alt={card.name}
                                   ratio="aspect-[626/457]"
                                   className="w-16 cursor-help" />
                        </span>
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
                          <span className="text-[10px] uppercase tracking-wide"
                                style={{ color: 'var(--text-muted)' }}>
                            {categoryLabel(card.category)}
                          </span>
                        </div>
                        {card.why && (
                          <p className="mt-0.5 text-xs leading-relaxed"
                             style={{ color: 'var(--text-muted)' }}>
                            <ManaText>{card.why}</ManaText>
                          </p>
                        )}
                      </div>
                      {deck.writable && (
                        <div className="flex shrink-0 items-center gap-2">
                          <button
                            onClick={() => void returnCard(card.name)}
                            title={`Return ${card.name} to the 99, rationale intact`}
                            className="card-action rounded-md px-2 py-1 text-[11px] font-medium">
                            Return
                          </button>
                          {/* The only permanent delete left — and it can only
                              ever act on a card already entombed, so exiling
                              is two deliberate steps by construction, plus
                              the arm. */}
                          <ArmedButton
                            title={`Exile ${card.name} — gone from the graveyard for good`}
                            armedLabel="Gone forever?"
                            onConfirm={() => void exileCard(card.name)}>
                            Exile
                          </ArmedButton>
                        </div>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          )}
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

      {tab === 'history' && <DeckHistory entries={history} />}
    </div>
  )
}

/**
 * What has been done to this deck, and by whom (ADR 28).
 *
 * The sentences are the server's — `decks/log.py:describe` renders them and
 * `mtglab decks log` prints the same strings. Nothing is composed here, which
 * is what stops the panel and the CLI drifting into two accounts of the same
 * edit. All this adds is the shape: a verb chip you can scan down, the actor,
 * and the time.
 *
 * Three states, kept apart, because collapsing the first two is how a panel
 * tells somebody their history is empty while it is still loading it:
 * `null` is not asked yet, `[]` is a deck whose every edit predates this log,
 * and anything else is the history.
 *
 * **No rationale appears here and none can**, because the server does not send
 * one. A `why` change is recorded as a `why` change; the text itself lives in
 * `deck.yaml`, which is the source of truth (rule 4).
 */
function DeckHistory({ entries }: { entries: DeckLogEntry[] | null }) {
  if (entries === null) {
    return <Spinner label="Reading the history…" />
  }
  if (entries.length === 0) {
    return (
      <div className="space-y-3">
        <Caveat>
          Every edit made through the app or the CLI is recorded here, with who
          made it. Nothing is written on your behalf — this is a record of what
          was done, not an opinion about it.
        </Caveat>
        <p className="card-surface rounded-xl px-4 py-6 text-sm"
           style={{ color: 'var(--text-muted)' }}>
          Nothing recorded yet. Edits made before this log existed are in git,
          not here.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <Caveat>
        Newest first. Card names, categories and which field changed — never
        the words of a rationale, which stay in <code>deck.yaml</code>.
      </Caveat>
      <ol className="card-surface divide-y rounded-xl"
          style={{ borderColor: 'var(--hairline)' }}>
        {entries.map((e) => (
          <li key={e.id}
              className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-4 py-2.5"
              style={{ borderColor: 'var(--hairline)' }}>
            <Badge tone={e.action === 'exile' ? 'critical' : 'neutral'}>
              {e.action}
            </Badge>
            <span className="min-w-0 flex-1 basis-64 text-sm">{e.summary}</span>
            <span className="text-xs" style={{ color: 'var(--text-secondary)' }}>
              {/* `null` is whoever is at the machine: the CLI, and the app with
                  auth off. An unnamed actor rather than an unknown one, so it
                  is named rather than left blank. */}
              {e.actor ?? 'you, locally'}
            </span>
            <time className="text-xs tabular-nums"
                  style={{ color: 'var(--text-muted)' }}
                  dateTime={e.created_at}>
              {logWhen(e.created_at)}
            </time>
          </li>
        ))}
      </ol>
    </div>
  )
}

/** An ISO-8601 UTC instant in the reader's own locale and zone.
 *
 * The server stores and sends UTC — string comparison is date comparison, and
 * a shared instance has readers in more than one zone. Turning it local is the
 * client's job precisely because the client is the only part that knows where
 * it is. An unparseable stamp is shown as it arrived rather than as
 * "Invalid Date": a row that cannot be dated still says what happened.
 */
function logWhen(stamp: string): string {
  const at = new Date(stamp)
  if (Number.isNaN(at.getTime())) return stamp
  return at.toLocaleString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

