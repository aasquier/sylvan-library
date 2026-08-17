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
import candleMp4Url from '../assets/seance/candle-glow-loop.mp4'
import candleWebmUrl from '../assets/seance/candle-glow-loop.webm'
import standUrl from '../assets/seance/crystal-fish.webp'
import shellUrl from '../assets/seance/crystal-shell-sepia.webp'
import candlesUrl from '../assets/seance/seance-candles.webp'
import smokeMp4Url from '../assets/seance/seance-smoke-loop.mp4'
import smokeWebmUrl from '../assets/seance/seance-smoke-loop.webm'
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
  'mtg-orzhov-pontiff': '62%',
  'mtg-lion-umbra': '45%',
  'mtg-balance': '38%',
  'mtg-approach-second-sun': '30%',
  'mtg-angelic-renewal': '40%',
  'mtg-wand-of-the-worldsoul': '55%',
  'mtg-tragic-poet': '60%',
  'mtg-everflowing-chalice': '50%',
  'mtg-sword-truth-justice': '42%',
  'mtg-sram-senior-edificer': '45%',
  'mtg-dragons-hoard': '50%',
  'mtg-smothering-tithe': '42%',
  'mtg-alms-collector': '40%',
  'mtg-king-macar': '45%',
  // Aaron's verdicts (2026-08-17). Eighteen cards arrived with no window
  // between them and every one defaulted to dead centre, which is where
  // True Love's Kiss keeps the knight's dark pauldron and loses both
  // faces. Each of these was picked off a rendered strip of the same crop
  // at 20/35/50/65/80, judged at the size the felt actually draws it.
  'mtg-willow-priestess': '70%',        // her face and the cat, not the faerie
  'mtg-true-loves-kiss': '30%',         // both of them in the window
  'mtg-suspension-field': '45%',        // the hanging figure, arms open
  'mtg-murderous-rider': '38%',         // the ink showcase: rider and skull horse
  'mtg-chalice-of-life': '50%',         // the cup, base to rim
  'mtg-asmodeus-the-archfiend': '60%',  // horns and throne
  'mtg-command-tower': '60%',           // the struck crown of the tower
  'mtg-ephara': '45%',                  // the urn and what pours from it
  'mtg-expedition-map': '30%',          // the compass rose
  'mtg-goblin-gathering': '30%',        // two goblins and the fire
  'mtg-young-pyromancer': '62%',        // the face *and* the flame in hand
  'mtg-hellrider': '45%',               // the devil on the horse
  'mtg-rite-of-harmony': '55%',         // all three of the ring
  'mtg-happily-ever-after': '28%',      // the two crowned figures
  'mtg-thassa': '30%',                  // her face, before the bident
  'mtg-curse-of-the-pierced-heart': '25%',  // the heart, not the screamer
  'mtg-startled-awake': '42%',          // the figure sitting up in bed
  'mtg-murder': '58%',                  // the blade already through
  // Round two of the minors (2026-08-17): seventeen more, every one picked
  // off a rendered strip at 20/35/50/65/80 before it was written down.
  'mtg-darling-of-the-masses': '32%',   // her, and the petals falling
  'mtg-high-ground': '30%',             // the defenders, the drop below
  'mtg-hail-of-arrows': '28%',          // the sky full of shafts
  'mtg-burdened-stoneback': '55%',      // the load, and the face under it
  'mtg-queen-marchesa': '50%',          // enthroned, straight to camera
  'mtg-wedding-announcement': '22%',    // both of them, not just the groom
  'mtg-forsake-the-worldly': '40%',     // the figure, arms open, leaving
  'mtg-golden-wish': '50%',             // the hand and what it holds
  'mtg-svyelun': '40%',                 // her, and the whirlpool under her
  'mtg-winters-rest': '80%',            // the head and the roses, not the feet
  'mtg-rescue-from-the-underworld': '22%',  // the ferryman and his lamp
  'mtg-pale-rider-of-trostad': '45%',   // the pale horse at full gallop
  'mtg-chromatic-star': '35%',          // the five points of it
  'mtg-harvest-season': '50%',          // the seedling in the hand
  'mtg-argivian-blacksmith': '40%',     // the hammer at the top of its swing
  'mtg-soraya-the-falconer': '30%',     // her face and the bird together
  'mtg-inheritance': '40%',             // both generations in the window
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
    '--arc-rot': `${ARC_ROT[index] ?? 0}deg`,
    '--arc-drop': `${ARC_DROP[index] ?? 0}px`,
  } as React.CSSProperties

  return (
    <div className={`tarot-slot${small ? ' is-small' : ''}`} style={style}>
      {/* The place printed on the cloth, under the card that fills it. Only
          on the felt: in the folded strip the cards are context for the
          conversation, and a marked position with a card already in it is a
          label for something nobody is about to do. */}
      {!small && <span className="tarot-place" aria-hidden="true" />}
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

/**
 * The room the ball stands in (Aaron's composition, 2026-08-17).
 *
 * The felt used to be a green rectangle with a crystal ball on it. It is a
 * *room* now, and the whole thing is one horizontal line: **black above,
 * green below, and the line between them is the back edge of the table.**
 * A votive rack burns along that line on either side, and the fish stands in
 * the gap with its base bridging the two racks — which is what stops the
 * mirror reading as a mirror, because the seam where the two halves would
 * meet is behind a foot of bronze.
 *
 * Three things make it work and each was a choice.
 *
 * **The stage is exactly as tall as the ball**, so the horizon is not a
 * number anybody has to keep in step: `.crystal-ball`'s box ends where the
 * carp's own foot ends (the stand occupies 17.98%–100% of it), so putting
 * the dark on `inset: 0` of this wrapper puts the table's back edge under the
 * fish automatically, at every width and every clamp of the ball's size.
 *
 * **The candles are screened, not matted.** The plate is candles on a black
 * ground, and black is already transparent under `screen` — so the ground
 * goes exactly, and the flames' halos and the light bleeding off the wax
 * survive as photographed rather than as whatever a matte's edge decided.
 * `matte_backdrop` exists for the opposite case, which is the bronze: a
 * studio grey *lighter* than its subject.
 *
 * **One file, mirrored.** Two crops of one photograph read as two different
 * racks and invite you to compare them; one rack seen from both ends of the
 * same table is what a room actually looks like.
 *
 * The dark is `ambience` weather like everything else here: reduced motion or
 * the ambience pref leaves the room lit and still rather than frozen mid
 * flicker.
 */
function SeanceRoom({ children }: { children: React.ReactNode }) {
  return (
    <div className="seance-stage">
      <div className="seance-dark" aria-hidden="true">
        <VideoBackdrop webmSrc={smokeWebmUrl} mp4Src={smokeMp4Url}
                       mode="ambience" className="seance-haze" />
      </div>
      {/* Two elements per rack, and it is not decoration: the inner edge
          needs a horizontal fade and the bottom needs a vertical one, which
          is two masks on one box. `mask-composite` is exactly the sort of
          thing WebKit 17.4 disagrees about, so each mask gets its own
          element — the span fades the edge that faces the fish, the img
          fades the wax into the table. */}
      <span className="seance-rack is-left" aria-hidden="true">
        <img src={candlesUrl} alt="" />
      </span>
      <span className="seance-rack is-right" aria-hidden="true">
        <img src={candlesUrl} alt="" />
      </span>
      {/* The light the rack puts back on the felt, in front of the dark so it
          washes over the table's edge rather than stopping at it. */}
      <span className="seance-spill" aria-hidden="true" />
      {children}
    </div>
  )
}

/** How far off square each dealt card lands, in reading order. */
const SETTLE_ROT = [-2.2, 1.6, -1.2]

/** The arc, in reading order: the three places are laid out *around the ball*
 *  rather than in a row, so they radiate from the thing they are being read
 *  under. `ARC_ROT` turns each place to face the centre and `ARC_DROP` sets
 *  it on the curve — the outer two ride high, the middle one sits lower,
 *  which is the shape a hand lays three cards in front of somebody.
 *
 *  These are the *place's* numbers and `SETTLE_ROT` is the *card's*: the
 *  printed position is exact and the card that lands in it is a hair off,
 *  because the cloth was printed and the deal was not. */
const ARC_ROT = [-9, 0, 9]
const ARC_DROP = [0, 26, 0]

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
 * The reader's crystal ball — a photograph now, both halves of it.
 *
 * The glass was the first half (PR 2): a CC0 smoky-quartz sphere, fetched,
 * licence-gated, matted and committed through the animist
 * (`assets/seance/crystal.recipe.yaml`, ADR 29) rather than hand-placed,
 * because the thing a real sphere has that geometry does not is *dirt* —
 * veils, fractures, a bloom of internal cloud that no gradient stack
 * proposes.
 *
 * **The stand is the second half, and it had to be.** It was inline SVG, and
 * it was good SVG — a turned foot, a bead course, a cushion ring split in
 * two so the near side occluded the glass. Asked whether it looked
 * photo-real, the answer was no, and the argument for keeping it ("a cradle
 * is turned geometry, which is what SVG is good at") was a rationalisation
 * for not doing the harder half. A photographed sphere on a drawn stand is
 * *worse* than an all-drawn ball, because the sphere sets a standard of
 * realism the stand cannot meet and the eye goes straight to the seam.
 *
 * So the stand is the Met's own crystal ball on a bronze stand in the shape
 * of a fish — a carp leaping through waves, the sphere held in a spray of
 * foam; museum open access, CC0. The decisive property is that **sphere and
 * stand come from ONE photograph**: one light, one shadow, one set of
 * material responses, and no compositing mismatch to manage. It also deletes
 * the problem three passes went into — the ball does not have to be made to
 * *sit in* the foam, because in the photograph it already does.
 *
 * **The composite is why it is five layers and not one picture.** The card
 * has to be *inside* the glass, so the stack is: our own candle and our own
 * smoke behind (two ADR 31 procedural loops, seeds 1848 and 1909 — the
 * spiritualist craze and the printing); the depths; the vision; the
 * photographed glass twice (`.crystal-shell-body` carrying it over the card
 * at `soft-light`, `.crystal-shell-light` crushed to its specular arc and
 * screened back on); and then the bronze **over all of it**, so the foam
 * closes in FRONT of the ball and it reads as held rather than balanced.
 * Flatten any of that and the card is in front of a ball instead of in one.
 *
 * **The geometry is the recipe's, not this file's**, and it is not to be
 * re-guessed here: in the 700×950 stand the foam closes at y=290, its centre
 * is x=529, and the ball is drawn at r=265 with its centre 0.88 radii
 * *above* that claw line — Aaron's number off a rendered board, because at
 * 0.62 the claws crossed the ball's belly and it read as impaled. Those four
 * numbers are ground into the percentages in `index.css`, where the
 * arithmetic that turns them into a box is written out.
 *
 * `vision` is the trick the ball was bought for: the card most recently
 * turned surfaces inside the glass, the way a fortune arrives — already
 * there once the mist thins. Whole rather than cropped, and a reversed card
 * hangs upside down in the glass too; the ball does not editorialise.
 *
 * Decorative and marked so. The spread announces every card by name; the
 * ball repeating it to a screen reader would be saying everything twice.
 */
function CrystalBall({ vision }: {
  vision?: { image: string; reversed: boolean } | null
}) {
  return (
    <div className="crystal-ball" aria-hidden="true">
      {/* The room behind the glass. Both are `ambience` mode, so reduced
          motion or the ambience pref removes them outright rather than
          freezing them — frozen weather is a smudge. */}
      <div className="crystal-room">
        <VideoBackdrop webmSrc={candleWebmUrl} mp4Src={candleMp4Url}
                       mode="ambience" className="crystal-candle" />
        <VideoBackdrop webmSrc={smokeWebmUrl} mp4Src={smokeMp4Url}
                       mode="ambience" className="crystal-smoke" />
      </div>

      {/* What the ball does to the table: its light on the felt. The stand's
          own cast shadow is in the photograph — the matte's `soft` ramp was
          built to keep it — so this is the glow and nothing else, and there
          is no drawn contact shadow to disagree with the real one. */}
      <span className="crystal-aura" />

      {/* The glass, and everything it holds. Absolutely positioned and it
          deliberately overhangs the stand's box on three sides, which is
          free precisely because it is a layer rather than a sibling. */}
      <div className="crystal-orb">
        <span className="crystal-depths" />
        {vision && (
          <img className={`crystal-vision${vision.reversed ? ' is-reversed' : ''}`}
               src={vision.image} alt="" />
        )}
        <img className="crystal-shell-body" src={shellUrl} alt="" />
        <img className="crystal-shell-light" src={shellUrl} alt="" />
      </div>

      {/* The bronze, last, so the foam closes in front of the glass. This one
          ordering is what turns "a ball balanced on a fish" into "a ball held
          by one" — the same argument the drawn cradle's split made, except
          that here the occluding edge is photographed rather than drawn. */}
      <img className="crystal-stand" src={standUrl} alt="" />
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
          {/* A spread costs one integer to remember, and the consequence
              nobody saw until Aaron went looking for Magic cards on the
              instance: coming back to the table re-deals the *stashed*
              seed, so the cards never change. A new deal only ever
              happened inside `chooseReader`, which meant the way to
              reshuffle was to leave and pick the same reader again —
              behind a button that says "Different reader", which is the
              one thing you do not want. This is that path, named for what
              it does. Only for a reader who deals; the plain voices have
              no cards to shuffle. */}
          {chosen.deals && (
            <button onClick={() => { void chooseReader(chosen) }}
                    disabled={shuffling}
                    className="rounded-md px-3 py-1.5 text-sm disabled:opacity-50"
                    style={{ border: '1px solid var(--hairline)',
                             color: 'var(--text-muted)' }}>
              {shuffling ? 'Shuffling…' : 'Shuffle again'}
            </button>
          )}
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
          under the reader's crystal ball; a strip against the left margin
          once the conversation is, where it sits over the column the
          questions are in rather than floating in the middle of the page
          with nothing under it. The felt goes when the table folds: a green
          rectangle beside a chat column is furniture, not ceremony. */}
      {cards.length > 0 && (
        <div className={dealing ? 'tarot-table-felt relative px-4 py-8' : ''}>
          {dealing && <TableWisps />}
          {dealing && (
            /* Above the spread and centred, which is where a crystal ball
               on a séance table goes — and which deletes a problem rather
               than managing one. Pinned to the felt's right edge it closed
               on the centred spread as the page narrowed, sat across the
               third card everywhere below about a thousand pixels, and had
               to be shrunk and then hidden below `lg` to cope. Standing it
               over the cards means nothing is beside it, so it shows at
               every width and can be the size the thing deserves. */
            <SeanceRoom>
              <CrystalBall vision={lastTurned && lastTurned.image
                ? { image: lastTurned.image, reversed: lastTurned.reversed }
                : null} />
            </SeanceRoom>
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
