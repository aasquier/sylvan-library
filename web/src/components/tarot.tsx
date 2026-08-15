/**
 * The table: pick a reader, deal three cards, turn them over (ADR 21).
 *
 * This is the create flow's fourth door and the only one that is allowed to be
 * theatre. ROADMAP item 3 put the requirement plainly before a line of it was
 * written — **a version of this that arrives sensible and dull has missed the
 * point** — so the deal is staggered, the backs are drawn rather than
 * borrowed, and turning a card is a card turning over.
 *
 * What the ceremony is wrapped around is not decoration. Three things:
 *
 * **A card is dealt *for* a slot.** `tarot.SPREAD`'s three positions are ADR
 * 20's first three slot kinds — taste, temperament, posture — so the reading
 * reuses the readiness instrument unchanged: the reader turns a card over,
 * asks about that slot with the picture in front of them, and the querent's
 * own words are still the only evidence there is. The cards colour the
 * questions. They are never mistaken for something anybody said.
 *
 * **The roster is fetched, not written here.** `/api/claude/personas` is free,
 * needs no key and no card pool, and answers before anyone has committed to
 * spending anything — so the first screen of the most expensive door in the
 * app costs nothing. Three more voices are on ADR 21's list and none of them
 * needs this file reopened.
 *
 * **A persona is fixed for a conversation.** The transcript is client-held and
 * resent whole every turn, so a voice swapped halfway leaves every earlier
 * answer speaking in the old one. Changing the reader remounts the interview
 * by its `key`, which is how that reads as a fact rather than as a warning
 * nobody sees.
 *
 * The pictures are the original 1909 Rider "Roses & Lilies" printing, public
 * domain in both the US and the UK, shipped as package data and served from
 * `/tarot/<key>.webp`. `assets/tarot/PROVENANCE.md` is the argument, and the
 * short version is that the 1971 recolouring everybody actually pictures is
 * still in copyright and is not this.
 */

import { useCallback, useEffect, useId, useRef, useState } from 'react'
import {
  api,
  type Persona,
  type PersonaRoster,
  type TarotDrawn,
  type TarotReading,
  type ThemeCommander,
} from '../lib/api'
import { ThemeInterview } from './theme'
import { CardArt } from './ui'

/** The table survives a reload, for the reason the transcript does: a reading
 *  is of one person on one evening, and re-dealing it would make it somebody
 *  else's. Only three integers' worth — the seed re-deals the rest. */
const TABLE = 'mtglab-tarot-table'

interface Table {
  /** Which reader was chosen, or null while nobody has sat down. */
  persona: string | null
  /** The spread's seed, or null for a reader who deals no cards. */
  seed: number | null
  /** Which of the three have been turned face up, by index. Stored rather
   *  than derived so a reload does not re-hide a card somebody has read. */
  turned: number[]
}

const NO_TABLE: Table = { persona: null, seed: null, turned: [] }

function loadTable(): Table {
  try {
    const raw = localStorage.getItem(TABLE)
    if (!raw) return NO_TABLE
    const p = JSON.parse(raw) as Partial<Table>
    return {
      persona: typeof p.persona === 'string' ? p.persona : null,
      seed: typeof p.seed === 'number' ? p.seed : null,
      turned: Array.isArray(p.turned)
        ? p.turned.filter((n) => typeof n === 'number')
        : [],
    }
  } catch {
    return NO_TABLE
  }
}

/* --------------------------------------------------------------- the cards */

/**
 * The back of a card, drawn.
 *
 * Drawn rather than photographed for the same reason `managlyphs.ts` draws the
 * mana symbols: the 78 faces are a public-domain scan with a provenance file
 * behind them, and a back lifted off the internet would be a 79th image with
 * no such argument attached. This one is geometry — an interlocking-ring
 * lattice on a deep field, which is what every card back has looked like since
 * the eighteenth century — so it owes nobody anything.
 *
 * It commits to one palette in both themes on purpose. A card back is a
 * physical object on a table, not a piece of UI chrome, and a card that
 * changed colour with the system theme would stop reading as one.
 */
function CardBack() {
  const id = useId().replace(/:/g, '')
  return (
    <svg viewBox="0 0 200 348" className="h-full w-full" aria-hidden="true">
      <defs>
        <pattern id={`lattice-${id}`} width="26" height="26"
                 patternUnits="userSpaceOnUse">
          {/* Rings on a half-drop grid, so the tile has no visible seam and
              the eye reads a weave rather than a checkerboard. */}
          <circle cx="13" cy="13" r="9.5" fill="none"
                  stroke="#c9a227" strokeWidth="0.9" opacity="0.55" />
          <circle cx="0" cy="0" r="9.5" fill="none"
                  stroke="#c9a227" strokeWidth="0.9" opacity="0.55" />
          <circle cx="26" cy="0" r="9.5" fill="none"
                  stroke="#c9a227" strokeWidth="0.9" opacity="0.55" />
          <circle cx="0" cy="26" r="9.5" fill="none"
                  stroke="#c9a227" strokeWidth="0.9" opacity="0.55" />
          <circle cx="26" cy="26" r="9.5" fill="none"
                  stroke="#c9a227" strokeWidth="0.9" opacity="0.55" />
          <circle cx="13" cy="13" r="1.6" fill="#c9a227" opacity="0.7" />
        </pattern>
        <radialGradient id={`glow-${id}`} cx="50%" cy="46%" r="62%">
          <stop offset="0%" stopColor="#2c3f6b" />
          <stop offset="100%" stopColor="#141d33" />
        </radialGradient>
      </defs>

      <rect width="200" height="348" rx="9" fill={`url(#glow-${id})`} />
      <rect x="7" y="7" width="186" height="334" rx="5"
            fill={`url(#lattice-${id})`} />
      <rect x="7" y="7" width="186" height="334" rx="5" fill="none"
            stroke="#c9a227" strokeWidth="1.4" opacity="0.85" />
      <rect x="12.5" y="12.5" width="175" height="323" rx="3" fill="none"
            stroke="#c9a227" strokeWidth="0.7" opacity="0.5" />

      {/* A sun with alternating straight and wavy rays — Smith drew one on
          nearly every card in the deck that needed a sky. */}
      <g transform="translate(100 174)">
        <circle r="34" fill="#141d33" opacity="0.9" />
        <circle r="34" fill="none" stroke="#c9a227" strokeWidth="1.1"
                opacity="0.85" />
        {Array.from({ length: 16 }, (_, i) => {
          const a = (i * Math.PI * 2) / 16
          const inner = 36
          const outer = i % 2 === 0 ? 52 : 45
          return (
            <line key={i}
                  x1={Math.cos(a) * inner} y1={Math.sin(a) * inner}
                  x2={Math.cos(a) * outer} y2={Math.sin(a) * outer}
                  stroke="#c9a227" strokeWidth={i % 2 === 0 ? 1.5 : 0.9}
                  opacity="0.8" strokeLinecap="round" />
          )
        })}
        <circle r="17" fill="none" stroke="#c9a227" strokeWidth="0.9"
                opacity="0.7" />
        <circle r="6" fill="#c9a227" opacity="0.85" />
      </g>
    </svg>
  )
}

/**
 * One card in its place: a back, a face, and a hinge between them.
 *
 * The reversal lives on the `<img>` and not on the face, which is the one
 * detail here that is easy to get wrong and invisible when you do. Both
 * transforms are `rotate`s on the same element otherwise, so a reversed card
 * would spend the flip un-reversing itself and land the right way up.
 */
function TarotCard({ card, faceUp, onTurn, index, small }: {
  card: TarotDrawn
  faceUp: boolean
  onTurn?: () => void
  index: number
  small?: boolean
}) {
  const label = faceUp
    ? `${card.position}: ${card.name}${card.reversed ? ', reversed' : ''}`
    : `${card.position}: face down`
  const inner = (
    <div className={`tarot-card${faceUp ? ' is-face-up' : ''}`}>
      <div className="tarot-face tarot-face-back">
        <CardBack />
      </div>
      <div className="tarot-face tarot-face-front">
        <img src={card.image} alt={card.name}
             className={card.reversed ? 'is-reversed' : ''} />
      </div>
    </div>
  )
  // Dealt with a stagger, so three cards land one after another rather than
  // appearing at once. The delay is a custom property because the keyframes
  // are shared and only the offset differs.
  const style = { '--deal-delay': `${index * 150}ms` } as React.CSSProperties

  return (
    <div className={`tarot-slot${small ? ' is-small' : ''}`} style={style}>
      {onTurn && !faceUp
        ? (
          <button onClick={onTurn} className="tarot-hinge" aria-label={`Turn over ${card.position}`}>
            {inner}
          </button>
          )
        : <div className="tarot-hinge" role="img" aria-label={label}>{inner}</div>}
      <p className="mt-2 text-center text-[11px] uppercase tracking-wide"
         style={{ color: 'var(--text-muted)' }}>
        {card.position}
      </p>
      {/* Only once it is face up. A name under a face-down card would be the
          spread explaining itself before it has been turned over, which is a
          form with candles on it. */}
      {faceUp && (
        <>
          <p className="text-center text-xs"
             style={{ color: 'var(--text-secondary)' }}>
            {card.name}
          </p>
          {/* Its own line rather than a suffix. At 96px "Nine of Swords ·
              reversed" wraps wherever it likes, and the word that says which
              way up the picture is should not be the orphan. */}
          {card.reversed && (
            <p className="text-center text-[11px] italic"
               style={{ color: 'var(--text-muted)' }}>
              reversed
            </p>
          )}
        </>
      )}
    </div>
  )
}

/** The three places, laid out. `small` is the strip the conversation runs
 *  under, once the ceremony is over and the cards are context rather than
 *  the event. */
function Spread({ cards, turned, onTurn, small }: {
  cards: TarotDrawn[]
  turned: number[]
  onTurn?: (i: number) => void
  small?: boolean
}) {
  return (
    <div className={`tarot-spread${small ? ' is-small' : ''}`}>
      {cards.map((card, i) => (
        <TarotCard key={card.key} card={card} index={i} small={small}
                   faceUp={turned.includes(i)}
                   onTurn={onTurn ? () => onTurn(i) : undefined} />
      ))}
    </div>
  )
}

/** The deck, stacked, while it is being shuffled. Three backs offset by a
 *  couple of degrees is enough to read as a pile of cards. */
function ShufflingDeck() {
  return (
    <div className="tarot-deck" aria-hidden="true">
      {[0, 1, 2].map((i) => (
        <div key={i} className="tarot-deck-card" style={
          { '--deck-index': i } as React.CSSProperties}>
          <CardBack />
        </div>
      ))}
    </div>
  )
}

/* -------------------------------------------------------------- the reader */

/**
 * Each costumed voice gets a painting, and the paintings are Scryfall art
 * crops — hotlinked with the artist credited on the tile, never committed
 * (rule 5, ADR 6), exactly as the page mastheads do it. Looked up on
 * Scryfall rather than recalled (rule 1 covers art too); artist and set are
 * what Scryfall reports for the default printing:
 *
 * - fortune-teller — *The Deck of Many Things*, Volkan Baǵa, Adventures in
 *   the Forgotten Realms: a spread of cards nobody should trust.
 * - therapist — *Alandra, Sky Dreamer*, Caroline Gariba, Murders at Karlov
 *   Manor Commander: somebody paid to sit with what you dream.
 * - scientist — *Rukarumel, Biologist*, Fariba Khamseh, Commander Masters:
 *   a field scientist delighted by her specimen.
 * - chef — *Gyome, Master Chef*, Steve Prescott: the house chef, and a nod
 *   to the deck he leads in this library.
 * - storyteller — *Birgi, God of Storytelling*, Eric Deschamps, Kaldheim.
 * - barkeep — *Edgewall Innkeeper*, Matt Stewart, Throne of Eldraine.
 *
 * `plain` is deliberately absent: "just talk to me" is the tile with no
 * costume, and a stand-in painting would make it one of seven characters
 * rather than the exit from character.
 */
const PERSONA_ART: Record<string, { art: string; credit: string }> = {
  'fortune-teller': {
    art: 'https://cards.scryfall.io/art_crop/front/f/e/feddbdc6-0757-43cb-bb41-dc83c6cf42ea.jpg',
    credit: 'Volkan Baǵa',
  },
  therapist: {
    art: 'https://cards.scryfall.io/art_crop/front/5/4/54bf48d4-e350-4ca7-87da-ce04fefd4610.jpg',
    credit: 'Caroline Gariba',
  },
  scientist: {
    art: 'https://cards.scryfall.io/art_crop/front/0/b/0b2f7397-9d75-4667-8872-e58a39512583.jpg',
    credit: 'Fariba Khamseh',
  },
  chef: {
    art: 'https://cards.scryfall.io/art_crop/front/8/2/8279d421-dd86-49d1-93f7-65f6046c542d.jpg',
    credit: 'Steve Prescott',
  },
  storyteller: {
    art: 'https://cards.scryfall.io/art_crop/front/4/4/44657ab1-0a6a-4a5f-9688-86f239083821.jpg',
    credit: 'Eric Deschamps',
  },
  barkeep: {
    art: 'https://cards.scryfall.io/art_crop/front/7/c/7c5d0560-f9e6-4c70-8cce-cae61e4e74bc.jpg',
    credit: 'Matt Stewart',
  },
}

function ReaderPanel({ persona, onPick }: {
  persona: Persona
  onPick: () => void
}) {
  const art = PERSONA_ART[persona.key]
  return (
    <button onClick={onPick}
            className="card-surface flex flex-col overflow-hidden rounded-xl text-center transition hover:opacity-90">
      {art && (
        <CardArt src={art.art} alt="" ratio="aspect-[626/457]"
                 className="art-fade w-full rounded-none" />
      )}
      <span className="flex flex-1 flex-col items-center gap-2 px-5 py-4">
        <span className="text-base font-medium">{persona.label}</span>
        <span className="text-xs leading-relaxed"
              style={{ color: 'var(--text-secondary)' }}>
          {persona.blurb}
        </span>
        <span className="mt-auto text-[11px] uppercase tracking-wide"
              style={{ color: 'var(--text-muted)' }}>
          {persona.deals ? 'Three cards' : 'No cards — just the questions'}
        </span>
        {art && (
          <span className="text-[10px]" style={{ color: 'var(--text-muted)' }}>
            Art by {art.credit}
          </span>
        )}
      </span>
    </button>
  )
}

/* --------------------------------------------------------------- the table */

export function TarotTable({ onPick, onLeave }: {
  onPick: (key: string, card: ThemeCommander) => void
  onLeave: () => void
}) {
  const [roster, setRoster] = useState<PersonaRoster | null>(null)
  const [table, setTable] = useState<Table>(loadTable)
  const [reading, setReading] = useState<TarotReading | null>(null)
  const [shuffling, setShuffling] = useState(false)
  const [settled, setSettled] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const timers = useRef<number[]>([])
  // Whether the last card was turned here, in front of somebody, or arrived
  // already face up from a stash. It decides whether the reveal gets its beat
  // — see the settle effect below.
  const turnedHere = useRef(false)

  useEffect(() => {
    api.personas().then(setRoster).catch((e) => setError(String(e)))
  }, [])

  useEffect(() => {
    localStorage.setItem(TABLE, JSON.stringify(table))
  }, [table])

  // Every stagger this component sets up, cancelled on the way out — a card
  // turning over into an unmounted tree is a warning in the console and a
  // state update nobody wanted.
  useEffect(() => () => { timers.current.forEach(clearTimeout) }, [])
  const later = (fn: () => void, ms: number) => {
    timers.current.push(window.setTimeout(fn, ms))
  }

  // Re-deal from the seed after a reload. Deterministic server-side, so the
  // cards that come back are the cards that were on the table — which is the
  // whole reason a spread costs one integer to remember.
  useEffect(() => {
    if (table.seed === null || reading?.seed === table.seed) return
    let live = true
    api.tarotReading(table.seed)
      .then((r) => { if (live) setReading(r) })
      .catch((e) => { if (live) setError(String(e)) })
    return () => { live = false }
  }, [table.seed, reading])

  const cards = reading?.cards ?? []
  const allTurned = cards.length > 0 && table.turned.length >= cards.length

  // The reveal gets to land before the table folds itself away.
  //
  // Without this the spread shrank to its strip the instant the *last* turn
  // was recorded — which is 840ms into a stagger whose third card is still
  // half way through a 760ms flip. So the climax of the whole door happened
  // during a resize, and the one card somebody was actually waiting to see
  // turned over at a third of the size it started at. The wait is skipped
  // when the cards arrived already face up from a stash, because then there
  // is no reveal to land: they were read on a previous visit.
  useEffect(() => {
    if (!allTurned) { setSettled(false); return }
    if (!turnedHere.current) { setSettled(true); return }
    const t = window.setTimeout(() => setSettled(true), 1600)
    return () => clearTimeout(t)
  }, [allTurned])

  const chooseReader = useCallback(async (persona: Persona) => {
    setError(null)
    if (!persona.deals) {
      setReading(null)
      setTable({ persona: persona.key, seed: null, turned: [] })
      return
    }
    // The shuffle is a beat rather than a spinner. `/api/tarot/reading` is a
    // dict lookup and answers instantly, and a reading that arrived the moment
    // you sat down would not feel like one — this is the door where that
    // matters, and ADR 21 says so in as many words.
    setShuffling(true)
    try {
      const dealt = await api.tarotReading()
      later(() => {
        setReading(dealt)
        setTable({ persona: persona.key, seed: dealt.seed, turned: [] })
        setShuffling(false)
      }, 1100)
    } catch (e) {
      setShuffling(false)
      setError(String((e as Error).message ?? e))
    }
  }, [])

  const turn = (i: number) => {
    turnedHere.current = true
    setTable((t) => (t.turned.includes(i) ? t : { ...t, turned: [...t.turned, i] }))
  }

  const turnAll = () => {
    if (!reading) return
    reading.cards.forEach((_, i) => later(() => turn(i), i * 420))
  }

  const leaveTable = () => {
    timers.current.forEach(clearTimeout)
    timers.current = []
    setReading(null)
    setTable(NO_TABLE)
    setError(null)
  }

  if (error && !roster) {
    return (
      <div className="card-surface rounded-xl px-6 py-8">
        <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
        <button onClick={onLeave} className="mt-3 rounded-md px-3 py-1.5 text-sm"
                style={{ border: '1px solid var(--hairline)',
                         color: 'var(--text-secondary)' }}>
          ← Pick colours myself
        </button>
      </div>
    )
  }
  if (!roster) {
    return <p className="text-sm" style={{ color: 'var(--text-muted)' }}>Loading…</p>
  }

  const chosen = roster.personas.find((p) => p.key === table.persona) ?? null

  /* -------------------------------------------------- nobody has sat down */

  if (!chosen || shuffling) {
    return (
      <section className="space-y-6">
        <div className="flex flex-wrap items-start gap-3">
          <div>
            <h2 className="text-xl font-semibold tracking-tight">
              Who do you want across the table?
            </h2>
            <p className="mt-1 max-w-2xl text-sm"
               style={{ color: 'var(--text-secondary)' }}>
              Whoever you pick, the questions are about you and never about
              Magic — what you already love, how you are when a plan comes
              apart, who you become with other people in the room. Only the
              voice changes. The fortune-teller asks with three cards face up
              on the table.
            </p>
          </div>
          <button onClick={onLeave}
                  className="ml-auto rounded-md px-3 py-1.5 text-sm"
                  style={{ border: '1px solid var(--hairline)',
                           color: 'var(--text-secondary)' }}>
            ← Pick colours myself
          </button>
        </div>

        {error && (
          <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
        )}

        {shuffling
          ? (
            <div className="flex flex-col items-center gap-4 py-10">
              <ShufflingDeck />
              <p className="text-sm tracking-wide" style={{ color: 'var(--text-muted)' }}>
                Shuffling, and cutting the deck…
              </p>
            </div>
            )
          : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {roster.personas.map((p) => (
                <ReaderPanel key={p.key} persona={p}
                             onPick={() => void chooseReader(p)} />
              ))}
            </div>
            )}
      </section>
    )
  }

  /* ------------------------------------------- a reader, and maybe a deal */

  const dealing = chosen.deals && !(allTurned && settled)
  const intro = chosen.deals
    ? {
        title: 'The cards are out',
        blurb: 'Three cards for three places. The questions are still about '
             + 'you — the pictures are only there to make them harder to '
             + 'answer politely.',
      }
    : undefined

  return (
    <section className="space-y-6">
      {/* One heading, and only while the cards are the event. Once the
          conversation starts the reader supplies its own (`intro` below), and
          two headings a paragraph apart were one heading too many. */}
      <div className="flex flex-wrap items-start gap-3">
        {dealing
          ? (
            <div>
              <h2 className="text-xl font-semibold tracking-tight">
                Three cards, face down
              </h2>
              <p className="mt-1 max-w-2xl text-sm"
                 style={{ color: 'var(--text-secondary)' }}>
                Dealt for three places — the root, the turning, the table. Turn
                them over when you are ready.
              </p>
            </div>
            )
          : cards.length > 0 && (
            <p className="text-[10px] uppercase tracking-wide"
               style={{ color: 'var(--text-muted)' }}>
              Your spread
            </p>
            )}
        <button onClick={leaveTable}
                className="ml-auto rounded-md px-3 py-1.5 text-sm"
                style={{ border: '1px solid var(--hairline)',
                         color: 'var(--text-muted)' }}>
          Different reader
        </button>
      </div>

      {error && (
        <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
      )}

      {/* The spread. Centred and large while it is the event; a strip against
          the left margin once the conversation is, where it sits over the
          column the questions are in rather than floating in the middle of
          the page with nothing under it. */}
      {cards.length > 0 && (
        <div className={dealing ? 'py-4' : ''}>
          <Spread cards={cards} turned={table.turned} small={!dealing}
                  onTurn={dealing ? turn : undefined} />
        </div>
      )}

      {dealing && (
        <div className="flex flex-wrap items-center justify-center gap-3">
          <button onClick={turnAll} disabled={allTurned}
                  className="rounded-md px-4 py-2 text-sm font-medium disabled:opacity-50"
                  style={{ background: 'var(--series-1)', color: '#fff' }}>
            Turn them over
          </button>
          <span className="text-xs" style={{ color: 'var(--text-muted)' }}>
            or turn them one at a time — {table.turned.length} of {cards.length}
          </span>
        </div>
      )}

      {/* Keyed on the reader and the spread, so choosing differently remounts
          rather than reuses. A persona is fixed for a conversation (ADR 21)
          and this is where that is enforced rather than asked for. */}
      {!dealing && (
        <ThemeInterview key={`${chosen.key}:${table.seed ?? 'none'}`}
                        persona={chosen.key} seed={table.seed}
                        intro={intro} onPick={onPick} onLeave={onLeave} />
      )}
    </section>
  )
}
