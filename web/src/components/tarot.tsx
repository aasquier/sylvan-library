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
import { deal as dealSound, flip as flipSound, riffle, shimmer }
  from '../lib/tablesounds'
import { useTableSound } from '../lib/prefs'
import { PERSONA_ART } from '../lib/personart'
import wispsMp4Url from '../assets/ambience/wisps-loop.mp4'
import wispsWebmUrl from '../assets/ambience/wisps-loop.webm'
import { ThemeInterview } from './theme'
import { CardArt } from './ui'
import { VideoBackdrop } from './videofx'

/** Mana over the felt: the wisps loop (ADR 31, seed 1909 — the Rider
 *  printing's year) drifting across the table at screen-blend opacity.
 *  Ambience mode, so reduced motion or the pref removes it entirely and
 *  the felt is exactly the table it always was. */
function TableWisps() {
  return <VideoBackdrop webmSrc={wispsWebmUrl} mp4Src={wispsMp4Url}
                        mode="ambience" className="tarot-wisps" />
}

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

/** The trumps' numerals, as the 1909 deck prints them. Index is the trump's
 *  own number, so Flubs (printed after The Fool) wears 0. */
const ROMAN = [
  '0', 'I', 'II', 'III', 'IV', 'V', 'VI', 'VII', 'VIII', 'IX', 'X', 'XI',
  'XII', 'XIII', 'XIV', 'XV', 'XVI', 'XVII', 'XVIII', 'XIX', 'XX', 'XXI',
]

/**
 * Where the character stands in each crossover's landscape crop, as the
 * horizontal centre of the portrait window the frame cuts from it. Measured
 * by eye against the art (the wheel's rule: fitted against the artwork,
 * checked with a render), because the plate must be *filled* the way
 * Smith's illustrations fill theirs — and each of the three turns out to
 * carry its trump's own iconography in that window: Flubs walks a cliff
 * edge with bindle and white rose, Massimo raises the wand over a table
 * holding cup and sword, Homer carries the lantern.
 */
const RWS_FOCUS: Record<string, string> = {
  'mtg-flubs-the-fool': '26%',
  'mtg-massimo-the-magician': '42%',
  'mtg-homer-the-hermit': '52%',
  // The echoes (deep scan 2026-08-16): each window centred on what makes
  // the painting its trump — Galina's throne, Apatzec's radiant seat, the
  // chariot's body, Gelon's wheel, the burning tower, the imprisoned moon,
  // the world tree's trunk.
  'mtg-empress-galina': '45%',
  'mtg-emperor-apatzec': '50%',
  // Both cats and the chariot's carved face — the window that says
  // "drawn by two cats" at a glance.
  'mtg-esikas-chariot': '40%',
  'mtg-wheel-of-fortune': '52%',
  'mtg-imprisoned-in-the-moon': '70%',
  'mtg-the-world-tree': '42%',
  // The second and third dives (2026-08-16): every trump answered, and the
  // suits opened. Same rule as always — the window holds what makes the
  // painting its original.
  'mtg-blind-seer': '50%',
  'mtg-orzhov-pontiff': '62%',
  'mtg-kynaios-and-tiro': '62%',
  'mtg-lion-umbra': '45%',
  'mtg-balance': '38%',
  'mtg-hanged-executioner': '48%',
  'mtg-pale-rider': '45%',
  'mtg-alchemists-apprentice': '45%',
  'mtg-master-of-cruelties': '45%',
  'mtg-stone-rain': '38%',
  'mtg-starfield-of-nyx': '38%',
  'mtg-approach-second-sun': '30%',
  'mtg-angelic-renewal': '40%',
  'mtg-wand-of-the-worldsoul': '55%',
  'mtg-chart-a-course': '55%',
  'mtg-tragic-poet': '60%',
  'mtg-everflowing-chalice': '50%',
  'mtg-sword-truth-justice': '42%',
  'mtg-sram-senior-edificer': '45%',
  'mtg-dragons-hoard': '50%',
  'mtg-smothering-tithe': '42%',
  'mtg-alms-collector': '40%',
  'mtg-king-macar': '45%',
}

/**
 * A Magic crossover, dressed as the trump it is printed after (overhaul
 * item 3, 2026-08-16). The old render matted a landscape art crop onto the
 * parchment and it read as exactly that — a photo letterboxed into a card.
 *
 * This draws the 1909 frame instead: ivory ground, black-ruled plate, the
 * roman numeral in its band, the name hand-set below in the Fell Types
 * (`assets/fonts/PROVENANCE.md`). The art cover-fills the plate through the
 * per-card focus above — a deliberate portrait window onto the character,
 * where the first cut of this frame letterboxed the whole crop and shipped
 * a tiny picture floating in blur. A muted-watercolour filter sits the
 * modern art with the ninety-year-old scans beside it. No derivative is
 * committed anywhere; the image is the Scryfall URL the credit line
 * already covers, and the "edit" exists only as CSS at render time.
 *
 * The reversal is on this wrapper rather than an inner element, because for
 * the 78 the picture *is* the card face and here the frame is — a reversed
 * physical card is upside down frame, caption and all.
 */
function CrossoverFace({ card }: { card: TarotDrawn }) {
  const style = { '--rws-focus': RWS_FOCUS[card.key] ?? '50%' } as
    React.CSSProperties
  return (
    <div className={`tarot-rws${card.reversed ? ' is-reversed' : ''}`}
         style={style}>
      <div className="tarot-rws-plate">
        {/* Only the trumps wear a numeral band — the 1909 pips carry their
            number in the picture, and a Magic minor follows its original. */}
        {card.arcana === 'major' && (
          <p className="tarot-rws-numeral">{ROMAN[card.number] ?? ''}</p>
        )}
        <div className="tarot-rws-canvas">
          <img src={card.image} alt={card.face_name}
               className="tarot-rws-art" />
        </div>
      </div>
      <p className="tarot-rws-name">{card.face_name}</p>
    </div>
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
    ? `${card.position}: ${card.face_name}${card.reversed ? ', reversed' : ''}`
    : `${card.position}: face down`
  const inner = (
    <div className={`tarot-card${faceUp ? ' is-face-up' : ''}`}>
      <div className="tarot-face tarot-face-back">
        <CardBack />
      </div>
      <div className="tarot-face tarot-face-front">
        {/* A Magic crossover wears the 1909 frame (`CrossoverFace`): the crop
            is landscape, a vertical slice of it would hide exactly the
            character the wink depends on, and a letterboxed photo on
            parchment is not a tarot card. */}
        {card.after
          ? <CrossoverFace card={card} />
          : <img src={card.image} alt={card.name}
                 className={card.reversed ? 'is-reversed' : ''} />}
      </div>
    </div>
  )
  // Dealt with a stagger, so three cards land one after another rather than
  // appearing at once. The delay is a custom property because the keyframes
  // are shared and only the offset differs — and each card settles with its
  // own slight rotation, because a machine deals parallel and a hand does
  // not. Fixed per slot rather than random: the same table on a reload.
  const style = {
    '--deal-delay': `${index * 150}ms`,
    '--settle-rot': `${SETTLE_ROT[index] ?? 0}deg`,
  } as React.CSSProperties

  return (
    <div className={`tarot-slot${small ? ' is-small' : ''}`} style={style}>
      {onTurn && !faceUp
        ? (
          <button onClick={onTurn} className="tarot-hinge" aria-label={`Turn over ${card.position}`}>
            {inner}
          </button>
          )
        : <div className="tarot-hinge" role="img" aria-label={label}>{inner}</div>}
      <p className="tarot-caption mt-2 text-center text-[11px] uppercase tracking-wide"
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
            {card.face_name}
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
          {/* The crossover's provenance (item 13): which original it
              answers, and whose painting this is — hotlinked art is
              credited wherever it renders, the persona tiles' rule. */}
          {card.after && (
            <p className="text-center text-[10px]"
               style={{ color: 'var(--text-muted)' }}>
              after {card.after} · art by {card.artist}
            </p>
          )}
          {/* Why this card holds its slot (Aaron, 2026-08-16): the
              resonance with its original, in checkable facts — a power of
              0, a fourth of his name. A fun fact, not a meaning; the
              reading stays the reader's. Hidden in the folded strip,
              where a paragraph per card would bury the conversation. */}
          {card.note && !small && (
            <p className="tarot-note mx-auto mt-1 max-w-[26rem] text-center text-[11px] italic leading-relaxed"
               style={{ color: 'var(--text-secondary)' }}>
              {card.note}
            </p>
          )}
        </>
      )}
    </div>
  )
}

/** How far off square each dealt card lands, in reading order. */
const SETTLE_ROT = [-2.2, 1.6, -1.2]

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

/**
 * The reader's crystal ball — a real one this time.
 *
 * Inline SVG like `CardBack`, so it costs no asset and owes nobody a licence:
 * the glass, the fog and the brass are all geometry. Three layers of blurred
 * fog turn against each other inside a clip of the sphere (`.crystal-fog-*`),
 * glints twinkle on their own delays, the gleam still breathes, and an aura
 * pools on the felt underneath. Everything holds still under
 * `prefers-reduced-motion` — the fog is simply weather, arrested.
 *
 * `vision` is the trick the ball was bought for: the card most recently
 * turned over surfaces inside the glass, under the fog, the way a fortune
 * arrives — already there once the mist thins. Whole, since the punch list
 * (2026-08-15 item 11) — the old crop filled the glass with a detail nobody
 * could name. A reversed card appears upside down in the glass too; the
 * ball does not editorialise.
 *
 * The same punch list called the ball clip art, and the answer is optics
 * rather than assets: a fresnel ring (glass darkens at its own silhouette),
 * one hard specular hotspot, a contact shadow under a properly turned
 * cradle — lathework is a stack of profiles, each catching its own light —
 * and behind the whole thing, slow smoke with a light flickering in it
 * (`.crystal-haze`), so the ball sits in a séance and not on a sticker.
 *
 * Decorative and marked so. The spread announces every card by name; the
 * ball repeating it to a screen reader would be saying everything twice.
 */
function CrystalBall({ vision }: {
  vision?: { image: string; reversed: boolean } | null
}) {
  const id = useId().replace(/:/g, '')
  return (
    <div className="crystal-ball" aria-hidden="true">
      {/* The room behind the glass: drifting smoke, and a light somewhere
          in it that will not hold steady. Behind the svg in paint order. */}
      <div className="crystal-haze">
        <span className="crystal-backlight" />
        <span className="crystal-haze-puff crystal-haze-a" />
        <span className="crystal-haze-puff crystal-haze-b" />
        <span className="crystal-haze-puff crystal-haze-c" />
      </div>
      <svg viewBox="0 0 120 150" className="h-full w-full">
        <defs>
          {/* The depths: a nebula rather than a flat gradient. The base is
              near-black indigo; two coloured pools drift through it below
              (`.crystal-nebula-*`) so the interior reads as space with
              weather in it, not as a tinted circle. */}
          <radialGradient id={`${id}-depth`} cx="50%" cy="44%" r="70%">
            <stop offset="0%" stopColor="#312566" />
            <stop offset="45%" stopColor="#1d1548" />
            <stop offset="100%" stopColor="#080520" />
          </radialGradient>
          <radialGradient id={`${id}-nebula-violet`} cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="rgba(147,98,255,0.55)" />
            <stop offset="100%" stopColor="rgba(147,98,255,0)" />
          </radialGradient>
          <radialGradient id={`${id}-nebula-teal`} cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="rgba(64,201,198,0.4)" />
            <stop offset="100%" stopColor="rgba(64,201,198,0)" />
          </radialGradient>
          <radialGradient id={`${id}-sheen`} cx="34%" cy="26%" r="60%">
            <stop offset="0%" stopColor="rgba(255,255,255,0.55)" />
            <stop offset="34%" stopColor="rgba(255,255,255,0.13)" />
            <stop offset="62%" stopColor="rgba(255,255,255,0)" />
          </radialGradient>
          {/* Light gathering at the bottom of the sphere, the way it does in
              real glass: a caustic pool, faintly violet. */}
          <radialGradient id={`${id}-caustic`} cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="rgba(196,170,255,0.4)" />
            <stop offset="70%" stopColor="rgba(196,170,255,0.08)" />
            <stop offset="100%" stopColor="rgba(196,170,255,0)" />
          </radialGradient>
          <radialGradient id={`${id}-fog`} cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="rgba(224,212,255,0.9)" />
            <stop offset="55%" stopColor="rgba(168,142,255,0.35)" />
            <stop offset="100%" stopColor="rgba(168,142,255,0)" />
          </radialGradient>
          <linearGradient id={`${id}-rim`} x1="0" y1="0" x2="0.6" y2="1">
            <stop offset="0%" stopColor="rgba(255,255,255,0.75)" />
            <stop offset="45%" stopColor="rgba(190,170,255,0.18)" />
            <stop offset="100%" stopColor="rgba(120,90,220,0.45)" />
          </linearGradient>
          {/* Brass, with the stops a photograph of brass actually has: a
              bright turned band, a fast fall to shadow, a bounce of warm
              reflected light off the felt near the foot. The lip runs the
              gradient horizontally, because a lathe leaves its light across
              the turning, not down it. */}
          <linearGradient id={`${id}-brass`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#f6e3a0" />
            <stop offset="18%" stopColor="#d4a94e" />
            <stop offset="38%" stopColor="#8f6526" />
            <stop offset="62%" stopColor="#5c3f16" />
            <stop offset="84%" stopColor="#3e2a0e" />
            <stop offset="100%" stopColor="#6b4d1e" />
          </linearGradient>
          <linearGradient id={`${id}-brass-lip`} x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="#6b4d1e" />
            <stop offset="22%" stopColor="#f8e6a8" />
            <stop offset="40%" stopColor="#c9a227" />
            <stop offset="58%" stopColor="#8f6526" />
            <stop offset="78%" stopColor="#e8c76a" />
            <stop offset="100%" stopColor="#5c3f16" />
          </linearGradient>
          {/* The vertical streak a window leaves on polished metal. */}
          <linearGradient id={`${id}-brass-streak`} x1="0" y1="0" x2="1" y2="0">
            <stop offset="0%" stopColor="rgba(255,244,200,0)" />
            <stop offset="45%" stopColor="rgba(255,244,200,0.55)" />
            <stop offset="55%" stopColor="rgba(255,244,200,0.55)" />
            <stop offset="100%" stopColor="rgba(255,244,200,0)" />
          </linearGradient>
          {/* Fresnel: real glass goes dark and reflective at its silhouette.
              Transparent through the body, a deep violet ring at the edge —
              this one gradient is most of the difference between "sphere"
              and "circle with a border". */}
          <radialGradient id={`${id}-fresnel`} cx="50%" cy="46%" r="54%">
            <stop offset="0%" stopColor="rgba(8,5,32,0)" />
            <stop offset="78%" stopColor="rgba(8,5,32,0)" />
            <stop offset="93%" stopColor="rgba(24,14,64,0.42)" />
            <stop offset="100%" stopColor="rgba(5,3,24,0.75)" />
          </radialGradient>
          <radialGradient id={`${id}-hotspot`} cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="rgba(255,255,255,0.95)" />
            <stop offset="40%" stopColor="rgba(255,255,255,0.35)" />
            <stop offset="100%" stopColor="rgba(255,255,255,0)" />
          </radialGradient>
          <radialGradient id={`${id}-shadow`} cx="50%" cy="50%" r="50%">
            <stop offset="0%" stopColor="rgba(4,2,18,0.6)" />
            <stop offset="70%" stopColor="rgba(4,2,18,0.25)" />
            <stop offset="100%" stopColor="rgba(4,2,18,0)" />
          </radialGradient>
          <clipPath id={`${id}-glass`}>
            <circle cx="60" cy="62" r="44" />
          </clipPath>
          {/* What turns flat ellipses into weather: static turbulence tears
              the fog's edges into wisps as each layer drifts through it. The
              noise itself never animates (no SMIL — reduced motion must be
              able to arrest everything from CSS); the *layers* move, and the
              displacement makes their edges boil as they go. */}
          <filter id={`${id}-wisp`} x="-60%" y="-60%" width="220%" height="220%">
            <feTurbulence type="fractalNoise" baseFrequency="0.055 0.09"
                          numOctaves="3" seed="7" result="noise" />
            <feDisplacementMap in="SourceGraphic" in2="noise" scale="26"
                               xChannelSelector="R" yChannelSelector="G" />
            <feGaussianBlur stdDeviation="2.6" />
          </filter>
          <filter id={`${id}-soft`} x="-40%" y="-40%" width="180%" height="180%">
            <feGaussianBlur stdDeviation="5" />
          </filter>
        </defs>

        {/* What the ball does to the table: its violet light, and under the
            stand the shadow the light cannot argue away. Shadow first, so
            everything sits *on* something — the missing contact shadow was
            half of what read as clip art. */}
        <ellipse cx="60" cy="144" rx="34" ry="6" fill={`url(#${id}-shadow)`} />
        <ellipse className="crystal-aura" cx="60" cy="130" rx="46" ry="10"
                 fill="#8f79e8" />

        {/* The stand: a turned brass cradle. Real lathework is a stack of
            profiles — a cushion ring the sphere sits in, a waisted ogee
            body, a beaded band, a flared foot — and each course carries the
            lip gradient across it so every turning catches its own light. */}
        {/* The body, waisted then flared. */}
        <path d="M36 116 C 36 108 84 108 84 116 C 84 124 76 126 74 130
                 L 78 140 C 78 148 42 148 42 140 L 46 130
                 C 44 126 36 124 36 116 Z"
              fill={`url(#${id}-brass)`} />
        {/* The cushion ring the sphere rests in, seen edge-on. */}
        <ellipse cx="60" cy="112" rx="26" ry="7" fill={`url(#${id}-brass-lip)`} />
        <ellipse cx="60" cy="111" rx="21" ry="5" fill="#1a1030" />
        <ellipse cx="60" cy="110.4" rx="21" ry="5" fill="none"
                 stroke="#f8e6a8" strokeWidth="0.7" opacity="0.7" />
        {/* Engraved turnings on the body. */}
        <path d="M42 122 C 50 126 70 126 78 122" stroke="#2c1d0a"
              strokeWidth="1" fill="none" opacity="0.55" />
        <path d="M43 120.6 C 51 124.6 69 124.6 77 120.6" stroke="#f3dc96"
              strokeWidth="0.7" fill="none" opacity="0.6" />
        <path d="M45 132 C 52 135 68 135 75 132" stroke="#2c1d0a"
              strokeWidth="0.9" fill="none" opacity="0.5" />
        {/* The window's streak down the polished body. */}
        <path d="M50 112 L 48 146 L 56 146 L 56 112 Z"
              fill={`url(#${id}-brass-streak)`} opacity="0.5" />
        {/* The beaded band above the foot. */}
        {[46, 52.5, 59, 65.5, 72].map((x) => (
          <g key={x}>
            <circle cx={x + 1} cy="137.5" r="1.6" fill="#e8c76a"
                    opacity="0.9" />
            <circle cx={x + 0.5} cy="137" r="0.55" fill="#fff7dd"
                    opacity="0.9" />
          </g>
        ))}
        {/* The flared foot, its own turning and its own light. */}
        <path d="M42 140 C 42 138 78 138 78 140 L 82 144
                 C 84 147 76 149 60 149 C 44 149 36 147 38 144 Z"
              fill={`url(#${id}-brass)`} />
        <path d="M38.5 144.5 C 44 147.5 76 147.5 81.5 144.5" stroke="#f3dc96"
              strokeWidth="0.8" fill="none" opacity="0.55" />
        {/* Three stones: amethyst, opal, jade — each with its own glint. */}
        <circle cx="60" cy="128" r="3.4" fill="#7d4b8f" />
        <circle cx="60" cy="128" r="3.4" fill="none" stroke="#2c1d0a"
                strokeWidth="0.5" opacity="0.6" />
        <circle cx="59" cy="127" r="1" fill="#e6c8f2" opacity="0.9" />
        <circle cx="46" cy="126" r="2.4" fill="#cfd8e8" />
        <circle cx="45.4" cy="125.4" r="0.8" fill="#fff" opacity="0.95" />
        <circle cx="74" cy="126" r="2.4" fill="#2e6f6a" />
        <circle cx="73.4" cy="125.4" r="0.8" fill="#bff0e2" opacity="0.9" />

        {/* The glass, and everything it holds. */}
        <circle cx="60" cy="62" r="44" fill={`url(#${id}-depth)`} />
        <g clipPath={`url(#${id}-glass)`}>
          {/* The nebula: two pools of colour turning slowly against each
              other, far below the fog. */}
          <g className="crystal-nebula crystal-nebula-a">
            <ellipse cx="44" cy="50" rx="30" ry="20"
                     fill={`url(#${id}-nebula-violet)`} />
          </g>
          <g className="crystal-nebula crystal-nebula-b">
            <ellipse cx="76" cy="76" rx="26" ry="18"
                     fill={`url(#${id}-nebula-teal)`} />
          </g>
          {/* A drift of stars, deeper than the smoke. */}
          <g className="crystal-stars" fill="#fff">
            {[[34, 44, 0.7], [50, 30, 0.5], [72, 38, 0.6], [86, 60, 0.45],
              [78, 84, 0.55], [52, 92, 0.5], [30, 72, 0.45], [63, 58, 0.8],
              [42, 62, 0.4], [70, 68, 0.5]].map(([x, y, r]) => (
                <circle key={`${x}-${y}`} cx={x} cy={y} r={r} opacity="0.7" />
            ))}
          </g>

          {vision && (
            // Keyed on the image so a new turn arrives as a new vision. Two
            // players in the arrival: the smoke billows up first
            // (`.crystal-vision-smoke`), and the card surfaces through it,
            // unblurring as the smoke thins — the fortune is simply *there*
            // once the mist parts. A reversed card appears upside down; the
            // ball does not editorialise.
            <g key={vision.image}>
              <g transform={vision.reversed ? 'rotate(180 60 62)' : undefined}>
                {/* `meet`, not `slice`, and a modest footprint (punch list
                    2026-08-15 item 11): the old crop filled the glass and
                    showed a fist-sized detail of a card nobody could name.
                    The whole picture floats in the sphere now, its corners
                    just kissing the glass — a vision of the card, not a
                    zoom into it. */}
                <image className="crystal-vision" href={vision.image}
                       x="35.5" y="20" width="49" height="84"
                       preserveAspectRatio="xMidYMid meet" />
              </g>
              <g className="crystal-vision-smoke" filter={`url(#${id}-wisp)`}>
                <ellipse cx="60" cy="70" rx="34" ry="22" fill={`url(#${id}-fog)`} />
                <ellipse cx="48" cy="52" rx="22" ry="14" fill={`url(#${id}-fog)`} />
                <ellipse cx="74" cy="58" rx="20" ry="13" fill={`url(#${id}-fog)`} />
              </g>
            </g>
          )}

          {/* The standing weather: three layers of torn smoke drifting at
              incommensurate speeds, so the glass never visibly loops. */}
          <g className="crystal-fog crystal-fog-a" filter={`url(#${id}-wisp)`}>
            <ellipse cx="42" cy="52" rx="27" ry="13" fill={`url(#${id}-fog)`} />
            <ellipse cx="76" cy="76" rx="23" ry="11" fill={`url(#${id}-fog)`} />
          </g>
          <g className="crystal-fog crystal-fog-b" filter={`url(#${id}-wisp)`}>
            <ellipse cx="68" cy="42" rx="21" ry="10" fill={`url(#${id}-fog)`} />
            <ellipse cx="46" cy="84" rx="25" ry="11" fill={`url(#${id}-fog)`} />
          </g>
          <g className="crystal-fog crystal-fog-c" filter={`url(#${id}-soft)`}>
            <ellipse cx="60" cy="64" rx="31" ry="16" fill={`url(#${id}-fog)`} />
          </g>

          <g fill="#fff">
            <circle className="crystal-spark" cx="44" cy="46" r="1.4" />
            <circle className="crystal-spark crystal-spark-2" cx="77" cy="58" r="1" />
            <circle className="crystal-spark crystal-spark-3" cx="55" cy="86" r="1.2" />
            <circle className="crystal-spark crystal-spark-4" cx="69" cy="33" r="0.9" />
            <circle className="crystal-spark crystal-spark-5" cx="36" cy="66" r="1.1" />
          </g>

          {/* Glasswork, over everything inside: the caustic pool at the foot
              of the sphere, the sheen, then the fresnel — the darkening a
              sphere of glass does at its own silhouette, which is most of
              what separates a ball from a circle. Last, the hotspot: one
              small hard reflection of the room's light, blurred just enough
              to sit *on* the glass rather than in it. */}
          <ellipse cx="60" cy="94" rx="26" ry="10" fill={`url(#${id}-caustic)`} />
          <circle cx="60" cy="62" r="44" fill={`url(#${id}-sheen)`} />
          <circle cx="60" cy="62" r="44" fill={`url(#${id}-fresnel)`} />
          <ellipse cx="44" cy="35" rx="7" ry="4.5" fill={`url(#${id}-hotspot)`}
                   transform="rotate(-28 44 35)" />
          <circle cx="47.5" cy="32.5" r="1.3" fill="#fff" opacity="0.95" />
        </g>

        {/* The rim: a gradient stroke so the sphere's edge catches light at
            the top and holds shadowed violet at the base — one flat stroke
            was most of what read as clip art. */}
        <circle cx="60" cy="62" r="44" fill="none"
                stroke={`url(#${id}-rim)`} strokeWidth="1.6" />
        <circle cx="60" cy="62" r="42.2" fill="none"
                stroke="rgba(255,255,255,0.08)" strokeWidth="0.8" />
        {/* The gleam that breathes, and its small answer across the glass. */}
        <path className="crystal-gleam" d="M29 46 A 35 35 0 0 1 50 23"
              fill="none" stroke="rgba(255,255,255,0.92)" strokeWidth="3"
              strokeLinecap="round" />
        <path d="M83 90 A 34 34 0 0 1 74 98" fill="none"
              stroke="rgba(255,255,255,0.28)" strokeWidth="2"
              strokeLinecap="round" />
      </svg>
    </div>
  )
}

/**
 * The sound toggle. Off by default and structural about it — until this is
 * switched on, `lib/tablesounds` constructs no AudioContext at all. The
 * first switch-on is necessarily a click, which is also what the browser's
 * autoplay policy wants to see — and `wake()` spends that click twice over:
 * it constructs and resumes the context inside the one gesture every
 * browser accepts, and it answers with two soft taps, so a working toggle
 * is heard working. A silent "on" is indistinguishable from a broken one,
 * which is exactly what the first version shipped as.
 */
function SoundToggle() {
  // Through `lib/prefs.ts` rather than a local `useState`: the settings gear
  // flips the same preference, and two controls over one key must watch one
  // store or the one not clicked lies until remounted.
  const [on, setOn] = useTableSound()
  return (
    <button type="button"
            onClick={() => setOn(!on)}
            aria-pressed={on}
            title={on
              ? 'Table sounds are on'
              : 'Turn on table sounds — the shuffle, the deal, the flip'}
            className="rounded-md px-3 py-1.5 text-sm"
            style={{ border: '1px solid var(--hairline)',
                     color: on ? 'var(--text-secondary)' : 'var(--text-muted)' }}>
      {on ? '♪ Sound on' : '♪ Sound off'}
    </button>
  )
}

/* -------------------------------------------------------------- the reader */

/*
 * The paintings themselves moved to `lib/personart.ts` (punch list item 8):
 * the interview's rooms need them too, and this file already imports
 * `theme.tsx`, so a table in either component would be an import cycle.
 *
 * `plain` is deliberately absent from that table: it is the tile with no
 * costume, and a borrowed painting would make it one of seven characters
 * rather than the exit from character. It is not artless any more, though
 * (punch list 2026-08-15 item 1) — it wears `ClaudeMark` below, drawn rather
 * than painted, for the same reason the card back and the library's tree are
 * drawn: the one voice that is nobody in particular still deserves a face,
 * and the honest face for it is a mark rather than somebody else's portrait.
 */

/**
 * Claude's own tile art: a spark of warm light on the same night the card
 * backs are printed on.
 *
 * Chosen by Claude, since the tile is Claude (item 1 asked). Not a figure —
 * every costumed tile is a painting of somebody, and the point of this one
 * is that there is nobody between you and the conversation. A light source
 * works where a portrait would lie: warm against the indigo, radiating, with
 * the concentric rings a voice makes. The palette deliberately shares the
 * card back's night so the grid reads as one table, and the spark is the
 * warm terracotta none of the paintings use, so the one drawn tile still
 * reads as its own kind of thing.
 */
function ClaudeMark() {
  const id = useId().replace(/:/g, '')
  return (
    <svg viewBox="0 0 626 457" className="h-full w-full" aria-hidden="true"
         preserveAspectRatio="xMidYMid slice">
      <defs>
        <radialGradient id={`${id}-night`} cx="50%" cy="42%" r="75%">
          <stop offset="0%" stopColor="#2c3f6b" />
          <stop offset="60%" stopColor="#1b2647" />
          <stop offset="100%" stopColor="#101830" />
        </radialGradient>
        <radialGradient id={`${id}-halo`} cx="50%" cy="50%" r="50%">
          <stop offset="0%" stopColor="rgba(255,236,210,0.95)" />
          <stop offset="30%" stopColor="rgba(240,166,122,0.55)" />
          <stop offset="70%" stopColor="rgba(218,119,86,0.18)" />
          <stop offset="100%" stopColor="rgba(218,119,86,0)" />
        </radialGradient>
      </defs>
      <rect width="626" height="457" fill={`url(#${id}-night)`} />
      {/* A scatter of far stars, the card back's sky continued. */}
      <g fill="#f0e4c2">
        {[[62, 70, 1.6], [140, 330, 1.2], [210, 96, 1.1], [318, 40, 1.4],
          [430, 88, 1.2], [538, 150, 1.6], [566, 330, 1.1], [468, 396, 1.3],
          [96, 210, 1.0], [246, 402, 1.2], [388, 372, 1.0], [520, 244, 1.0],
        ].map(([x, y, r]) => (
          <circle key={`${x}-${y}`} cx={x} cy={y} r={r} opacity="0.55" />
        ))}
      </g>
      {/* The rings a voice makes: concentric, fading as they travel. */}
      {[74, 118, 166, 220].map((r, i) => (
        <circle key={r} cx="313" cy="222" r={r} fill="none"
                stroke="#f0a67a" strokeWidth={1.6 - i * 0.3}
                opacity={0.34 - i * 0.07} />
      ))}
      <circle cx="313" cy="222" r="150" fill={`url(#${id}-halo)`} />
      {/* The spark: rays alternating long and short, warm on the night. */}
      <g transform="translate(313 222)">
        {Array.from({ length: 12 }, (_, i) => {
          const a = (i * Math.PI * 2) / 12 - Math.PI / 2
          const inner = 26
          const outer = i % 2 === 0 ? 74 : 50
          return (
            <line key={i}
                  x1={Math.cos(a) * inner} y1={Math.sin(a) * inner}
                  x2={Math.cos(a) * outer} y2={Math.sin(a) * outer}
                  stroke={i % 2 === 0 ? '#f7d9b8' : '#e8956d'}
                  strokeWidth={i % 2 === 0 ? 7 : 4.5}
                  strokeLinecap="round" opacity="0.92" />
          )
        })}
        <circle r="19" fill="#fbe8d0" />
        <circle r="9" fill="#fff6ea" />
      </g>
    </svg>
  )
}

function ReaderPanel({ persona, onPick }: {
  persona: Persona
  onPick: () => void
}) {
  const art = PERSONA_ART[persona.key]
  return (
    // No `art-fade` on the wrapper: that class belongs to the `<img>` inside
    // `CardArt`, which is the element that gets `.loaded` back. On the wrapper
    // it is an opacity-0 that nothing ever lifts, and every tile shipped as a
    // black rectangle with a caption — the exact bug the punch list reported.
    <button onClick={onPick}
            className="reader-tile card-surface flex flex-col overflow-hidden rounded-xl text-center">
      {art && (
        <CardArt src={art.art} alt="" ratio="aspect-[626/457]" eager
                 className="reader-tile-art w-full rounded-none" />
      )}
      {/* The tile with no costume wears Claude's own mark — drawn, so it
          needs no credit line and owes nobody a licence (item 1). */}
      {!art && persona.key === 'plain' && (
        <span className="reader-tile-art block w-full aspect-[626/457]"
              aria-hidden="true">
          <ClaudeMark />
        </span>
      )}
      <span className="flex flex-1 flex-col items-center gap-2 px-5 py-4">
        <span className="text-base font-medium">{persona.label}</span>
        <span className="text-xs leading-relaxed"
              style={{ color: 'var(--text-secondary)' }}>
          {persona.blurb}
        </span>
        {/* Only the reader who deals gets a footer. Six tiles all reciting
            "No cards — just the questions" said nothing six times; the blurb
            already says who each voice is, and the one fact worth a line of
            its own is that the fortune-teller's table has cards on it. */}
        {persona.deals && (
          <span className="mt-auto text-[11px] uppercase tracking-wide"
                style={{ color: 'var(--series-1)' }}>
            ✦ Three cards, dealt for you
          </span>
        )}
        {art && (
          <span className={`text-[10px]${persona.deals ? '' : ' mt-auto'}`}
                style={{ color: 'var(--text-muted)' }}>
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
    riffle()
    try {
      const dealt = await api.tarotReading()
      later(() => {
        setReading(dealt)
        setTable({ persona: persona.key, seed: dealt.seed, turned: [] })
        setShuffling(false)
        dealSound()
      }, 1100)
    } catch (e) {
      setShuffling(false)
      setError(String((e as Error).message ?? e))
    }
  }, [])

  const turn = (i: number) => {
    turnedHere.current = true
    // The sound outside the updater: React may re-run an updater, and a card
    // must not flip twice in the ear when it flips once on the table. The
    // shimmer rides under the flip — the ball noticing what was turned.
    if (!table.turned.includes(i)) { flipSound(); shimmer() }
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
          <span className="ml-auto flex items-center gap-2">
            <SoundToggle />
            <button onClick={onLeave}
                    className="rounded-md px-3 py-1.5 text-sm"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-secondary)' }}>
              ← Pick colours myself
            </button>
          </span>
        </div>

        {error && (
          <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
        )}

        {shuffling
          ? (
            <div className="tarot-table-felt relative flex flex-col items-center gap-4 py-10">
              <TableWisps />
              <CrystalBall />
              <ShufflingDeck />
              <p className="text-sm tracking-wide" style={{ color: 'var(--tarot-felt-text)' }}>
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
  // What the ball is showing: the card most recently turned face up. Indexed
  // through `turned` rather than tracked separately, so a table restored from
  // a stash scries its last card too.
  const lastTurnedIndex = table.turned[table.turned.length - 1]
  const lastTurned = lastTurnedIndex === undefined
    ? null
    : cards[lastTurnedIndex] ?? null
  // Every voice frames its own table. The dealing reader talks about the
  // cards; everyone else introduces themselves with the same words their
  // tile used — so the screen the conversation opens on belongs to the voice
  // that was picked, rather than to one shared paragraph that read like the
  // interview's script.
  const intro = chosen.deals
    ? {
        title: 'The cards are out',
        blurb: 'Three cards for three places. The questions are still about '
             + 'you — the pictures are only there to make them harder to '
             + 'answer politely.',
      }
    : {
        title: chosen.label,
        blurb: `${chosen.blurb} None of the questions are about Magic.`,
      }

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
        <span className="ml-auto flex items-center gap-2">
          <SoundToggle />
          <button onClick={leaveTable}
                  className="rounded-md px-3 py-1.5 text-sm"
                  style={{ border: '1px solid var(--hairline)',
                           color: 'var(--text-muted)' }}>
            Different reader
          </button>
        </span>
      </div>

      {error && (
        <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
      )}

      {/* The spread. Centred and large while it is the event — on the felt,
          with the reader's crystal ball at the table's edge; a strip against
          the left margin once the conversation is, where it sits over the
          column the questions are in rather than floating in the middle of
          the page with nothing under it. The felt goes when the table folds:
          a green rectangle beside a chat column is furniture, not ceremony. */}
      {cards.length > 0 && (
        <div className={dealing ? 'tarot-table-felt relative px-4 py-8' : ''}>
          {dealing && <TableWisps />}
          {dealing && (
            /* `lg`, not `sm`. The ball is positioned against the felt's right
               edge while the spread is centred in the whole felt, so the two
               close on each other as the page narrows — at `sm` it sat on top
               of the third card for every width from 640px to about a
               thousand, which is most laptops in a half-width window. The
               arithmetic is in `index.css` beside `.crystal-ball`, which
               shrinks it with the viewport for the widths where it does fit.
               Below that it is absent rather than squeezed: no ball is a
               plainer table, and a ball across the cards is a broken one. */
            <div className="absolute right-5 top-1/2 hidden -translate-y-1/2 lg:block">
              <CrystalBall vision={lastTurned && lastTurned.image
                ? { image: lastTurned.image, reversed: lastTurned.reversed }
                : null} />
            </div>
          )}
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
