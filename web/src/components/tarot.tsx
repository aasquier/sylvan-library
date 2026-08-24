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
import { HandFanGlyph } from './glyphs'
import { reducedMotion } from '../lib/motion'
import { PERSONA_ART } from '../lib/personart'
import backUrl from '../assets/seance/tarot-back.webp'
import roomMp4Url from '../assets/seance/seance-room-loop.mp4'
import roomStillUrl from '../assets/seance/seance-room-still.webp'
import roomWebmUrl from '../assets/seance/seance-room-loop.webm'
import { ThemeInterview } from './theme'
import { CardArt, Spinner } from './ui'
import { VideoBackdrop } from './videofx'

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
  /** Whether the querent has taken the reading — knocked on the glass and
   *  moved on to the conversation. Before this the table LINGERS with all
   *  three cards up (Aaron, 2026-08-17): the spread is the event, and
   *  folding it away the moment the last card landed stole the one moment
   *  somebody might want to sit with. Stored so a reload lands where the
   *  querent actually is. */
  read: boolean
}

const NO_TABLE: Table = { persona: null, seed: null, turned: [], read: false }

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
      // A stash from before the linger existed has no `read`; those tables
      // were already in conversation, so landing them back on the felt with
      // the knock hint costs one double-click and loses nothing.
      read: p.read === true,
    }
  } catch {
    return NO_TABLE
  }
}

/* --------------------------------------------------------------- the cards */

/**
 * The back of a card.
 *
 * A painted Magic back (Aaron, 2026-08-17), replacing a drawn one. The drawn
 * version was geometry — an interlocking-ring lattice with the colour wheel
 * on it — chosen so the deck owed nobody anything, and it was fine. This is
 * better, and the reason is commandment 3 rather than craft: the back of the
 * deck is the one surface a querent looks at for the whole deal, and a
 * planeswalker's crown on it says *what game this is* before a single card
 * is turned.
 *
 * It is `cover`, and `index.css` says why: the plate is a Magic card's
 * proportions and the 1909 scans are a tarot card's, so one of them has to
 * give a tenth away, and a cropped margin beats two black bands.
 *
 * It commits to one palette in both themes on purpose. A card back is a
 * physical object on a table, not a piece of UI chrome, and a card that
 * changed colour with the system theme would stop reading as one.
 */
function CardBack() {
  return <img className="tarot-back" src={backUrl} alt="" aria-hidden="true" />
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
  } as React.CSSProperties

  return (
    <div className={`tarot-slot${small ? ' is-small' : ''}`} style={style}>
      {/* The card, and the place printed on the cloth under it, in a box of
          their own.

          The place used to take its size from the whole slot, which is the
          same box as the card only while the legend is absolutely
          positioned. On a touch screen it is not -- `(hover: hover)` keeps
          those lines in flow, because a phone has no pointer to rest and a
          newcomer must still be able to read what the cards are. So the slot
          grew by however many lines that card's legend ran to, and the gold
          frame grew with it: three cards carrying two, three and four lines
          printed three different-sized places on the cloth, and they crossed
          both each other and their neighbours. An `inset` off this box
          cannot disagree with the card it frames. Only on the felt: in the
          folded strip the cards are context for the conversation, and a
          marked position with a card already in it is a label for something
          nobody is about to do. */}
      <div className="tarot-seat">
        {!small && <span className="tarot-place" aria-hidden="true" />}
        {onTurn && !faceUp
          ? (
            <button onClick={onTurn} className="tarot-hinge" aria-label={`Turn over ${card.position}`}>
              {inner}
            </button>
            )
          : <div className="tarot-hinge" role="img" aria-label={label}>{inner}</div>}
      </div>
      <div className="tarot-legend">
      <p className="tarot-caption mt-2 text-center text-[11px] uppercase tracking-wide">
        {card.position}
      </p>
      {/* Only once it is face up. A name under a face-down card would be the
          spread explaining itself before it has been turned over, which is a
          form with candles on it. */}
      {faceUp && (
        <>
          <p className="tarot-legend-name text-center text-xs">
            {card.face_name}
          </p>
          {/* Its own line rather than a suffix. At 96px "Nine of Swords ·
              reversed" wraps wherever it likes, and the word that says which
              way up the picture is should not be the orphan. */}
          {card.reversed && (
            <p className="tarot-legend-aside text-center text-[11px] italic">
              reversed
            </p>
          )}
          {/* The crossover's provenance (item 13): which original it
              answers, and whose painting this is — hotlinked art is
              credited wherever it renders, the persona tiles' rule. */}
          {card.after && (
            <p className="tarot-legend-aside text-center text-[10px]">
              after {card.after} · art by {card.artist}
            </p>
          )}
          {/* Why this card holds its slot (Aaron, 2026-08-16): the
              resonance with its original, in checkable facts — a power of
              0, a fourth of his name. A fun fact, not a meaning; the
              reading stays the reader's. Hidden in the folded strip,
              where a paragraph per card would bury the conversation. */}
          {card.note && !small && (
            <p className="tarot-note tarot-legend-name mx-auto mt-1 max-w-[26rem] text-center text-[11px] italic leading-relaxed">
              {card.note}
            </p>
          )}
        </>
      )}
      </div>
    </div>
  )
}

/**
 * The room, photographed (Aaron, 2026-08-17).
 *
 * This replaced a composite the previous pass built out of parts — a drawn
 * gradient room, a CC0 candle plate mirrored into two racks, three smoke
 * loops and a museum bronze holding a smoky-quartz sphere. All of it was
 * doing one job: convincing you that a séance table was there. A single
 * photographed table does that job outright, and every seam the composite
 * had to manage — the rack's mirror line, the sphere against its stand, the
 * horizon where black met felt — is not managed here. It is absent.
 *
 * Three things about the arrangement, each a choice rather than a default.
 *
 * **Nothing is cropped, so nothing drifts.** The room keeps the footage's
 * own 16:9 and the ball's position is written down once, in `index.css`, as
 * a percentage of that frame. `object-fit: cover` on a box whose aspect is
 * the video's own is a no-op — which is the point: the vision has to land
 * inside the glass at every window width, and a crop that moves with the
 * viewport would move the glass out from under it.
 *
 * **The card is screened INTO the glass, over a dim.** The footage's sphere
 * is lit from within, and a picture screened onto a near-white ball adds
 * nothing you can see. So a soft dark disc multiplies the interior down
 * first and the card is screened on top of that — which is the same order
 * the old composite used (depths, vision, then the shell's specular arc
 * back over), minus the two photographs it needed to fake the glass, since
 * this glass is real and its highlights are already in the footage.
 *
 * **`art` mode, not `ambience`.** The distinction is `videofx.tsx`'s and it
 * decides what reduced motion means: weather is removed, a painting falls
 * back to its still. A room the whole ceremony is staged in is the second
 * kind — remove it and the cards are dealt onto nothing — so the still is
 * the floor and the loop is what plays over it.
 */
function SeanceRoom({ children, vision, onKnock }: {
  children?: React.ReactNode
  /** The card most recently turned, surfacing inside the glass — the way a
   *  fortune arrives, already there once the mist thins. A reversed card
   *  hangs upside down in the glass too; the ball does not editorialise. */
  vision?: { image: string; reversed: boolean } | null
  /** Two raps on the glass, while the spread lingers: the ceremonial way
   *  to take the reading. Purely additive — the visible button beside the
   *  spread is the accessible door, so this carries no key handling and
   *  no focus of its own. */
  onKnock?: () => void
}) {
  // Read once per mount, the way `VideoBackdrop` does: a live change of the
  // OS setting lands on the next navigation.
  const [still] = useState(() => reducedMotion())
  return (
    <div className={`seance-room${onKnock ? ' is-knockable' : ''}`}
         onDoubleClick={onKnock}
         title={onKnock ? 'Knock twice on the glass to begin the reading'
                        : undefined}>
      <VideoBackdrop webmSrc={roomWebmUrl} mp4Src={roomMp4Url}
                     mode="art" className="seance-room-plate"
                     poster={roomStillUrl}
                     fallback={<img className="seance-room-plate"
                                    src={roomStillUrl} alt="" aria-hidden />} />
      {/* Siblings rather than children of a wrapper, and deliberately: a
          `mix-blend-mode` blends against its stacking context, so a box
          around these two would blend them with each other and hand the
          video a flat composite. Two absolutely-positioned spans over the
          plate each see the plate as their backdrop. */}
      <span className="seance-glass-dim" aria-hidden="true" />
      {vision && (
        <img className={`seance-vision${vision.reversed ? ' is-reversed' : ''}`}
             src={vision.image} alt="" aria-hidden="true" />
      )}
      {/* What makes the picture read as suspended IN something rather than
          shown on a screen: turbulence displacing its own pixels, so the
          edges wander and the whole card breathes the way an image does
          through moving glass. Width/height zero because this is a
          definition, not a drawing; the seed is 1848, the year the
          spiritualist craze began, as everywhere else in this room.
          The animation is dropped rather than stilled under reduced
          motion, because SMIL cannot hear a media query. */}
      <svg width="0" height="0" style={{ position: 'absolute' }}
           aria-hidden="true">
        <filter id="seance-ripple" x="-20%" y="-20%" width="140%"
                height="140%">
          <feTurbulence type="fractalNoise" baseFrequency="0.014 0.022"
                        numOctaves="2" seed="1848" result="n">
            {!still && (
              <animate attributeName="baseFrequency"
                       values="0.014 0.022;0.019 0.03;0.012 0.018;0.014 0.022"
                       dur="11s" repeatCount="indefinite" />
            )}
          </feTurbulence>
          <feDisplacementMap in="SourceGraphic" in2="n" scale="15"
                             xChannelSelector="R" yChannelSelector="G" />
        </filter>
      </svg>
      {children}
    </div>
  )
}

/* Nothing rotates any more (Aaron, 2026-08-18: "still not all oriented in
   the same manner"). Two rotations used to compose here — a fan turning
   each place to face the ball, and a per-card wobble so the deal read as a
   hand rather than a machine. Both were arguments about a drawn table. On a
   photographed one the perspective is already in the footage, and cards
   lying at three different angles in front of it read as three cards
   somebody knocked, not as a spread. The custom properties they set are
   gone, and so are the `rotate()` calls in `index.css` that read them. */

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
            className="btn btn-felt btn-sm"
            style={{ opacity: on ? 1 : 0.75 }}>
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

export function TarotTable({ onPick, onLeave, onCeremony }: {
  onPick: (key: string, card: ThemeCommander) => void
  onLeave: () => void
  /** Fires with `true` while the deal is the event — the shuffle and the
   *  face-down/lingering spread — so the page around the table can clear
   *  its chrome and give the room the viewport (Aaron's item 6: the whole
   *  table in one screen). Always fired `false` on unmount. */
  onCeremony?: (active: boolean) => void
}) {
  const [roster, setRoster] = useState<PersonaRoster | null>(null)
  const [table, setTable] = useState<Table>(loadTable)
  const [reading, setReading] = useState<TarotReading | null>(null)
  const [shuffling, setShuffling] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const timers = useRef<number[]>([])
  // Whether the last card was turned here, in front of somebody, or arrived
  // already face up from a stash. It decides whether the reveal gets its beat
  // — see the settle below. State rather than a ref because `settled` is read
  // during render and is derived from this; turning a card re-renders the
  // table anyway, so it costs nothing.
  const [turnedHere, setTurnedHere] = useState(false)

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
  //
  // Only the *waiting* is state; `settled` itself is arithmetic over it, so
  // the stashed spread folds in the render it arrives in and never a beat
  // later. The cleanup re-arms the wait, because a fresh deal un-completes
  // the spread and the next reveal has earned the same beat this one got.
  const [waited, setWaited] = useState(false)
  useEffect(() => {
    if (!allTurned || !turnedHere) return
    const t = window.setTimeout(() => setWaited(true), 1600)
    return () => { clearTimeout(t); setWaited(false) }
  }, [allTurned, turnedHere])
  const settled = allTurned && (!turnedHere || waited)

  // Tell the page when the deal is the event. Computed here rather than
  // reusing `dealing` below because hooks cannot follow the early returns,
  // and the two must agree: this is `dealing || shuffling` by another
  // route. Cleanup fires `false` so leaving the door restores the chrome.
  // `shuffling` stands alone because only the dealing reader ever shuffles,
  // and during her shuffle `table.persona` is not yet written.
  const ceremonyActive = shuffling || (Boolean(
    roster?.personas.find((p) => p.key === table.persona)?.deals)
    && !(allTurned && settled && table.read))
  useEffect(() => {
    onCeremony?.(ceremonyActive)
  }, [ceremonyActive, onCeremony])
  useEffect(() => () => { onCeremony?.(false) }, [onCeremony])

  const chooseReader = useCallback(async (persona: Persona) => {
    setError(null)
    if (!persona.deals) {
      setReading(null)
      setTable({ persona: persona.key, seed: null, turned: [], read: false })
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
        setTable({ persona: persona.key, seed: dealt.seed, turned: [], read: false })
        setShuffling(false)
        dealSound()
      }, 1100)
    } catch (e) {
      setShuffling(false)
      setError(String((e as Error).message ?? e))
    }
  }, [])

  const turn = (i: number) => {
    setTurnedHere(true)
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
        <button onClick={onLeave} className="btn btn-quiet btn-sm mt-3">
          ← Pick colours myself
        </button>
      </div>
    )
  }
  if (!roster) {
    return <Spinner label="Gathering the readers…" />
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
                    className="btn btn-felt btn-sm">
              ← Pick colours myself
            </button>
          </span>
        </div>

        {error && (
          <p className="text-sm" style={{ color: 'var(--status-critical)' }}>{error}</p>
        )}

        {shuffling
          ? (
            <div className="tarot-table-felt relative flex flex-col items-center gap-4 pb-10">
              <SeanceRoom />
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

  // The linger (Aaron's item 4): all three cards up is not the end of the
  // ceremony — it is the moment the querent gets to sit with the spread.
  // The table folds only once they knock on the glass (`table.read`).
  const dealing = chosen.deals && !(allTurned && settled && table.read)
  const lingering = chosen.deals && allTurned && settled && !table.read
  const takeReading = () => setTable((t) => ({ ...t, read: true }))
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
      {/* While dealing there is no heading and no outer control row at all:
          the felt explains itself (the places are printed on the cloth) and
          the controls ride ON the room as overlays, because every line of
          chrome above the felt is a line the ceremony cannot fit in one
          screen (item 6). Once the conversation is the event the plain
          header row returns. */}
      {!dealing && (
        <div className="flex flex-wrap items-start gap-3">
          {cards.length > 0 && (
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
                      className="btn btn-felt btn-sm">
                <HandFanGlyph />
                {shuffling ? 'Shuffling…' : 'Shuffle again'}
              </button>
            )}
            <button onClick={leaveTable}
                    className="btn btn-felt btn-sm">
              Different reader
            </button>
          </span>
        </div>
      )}

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
        <div className={dealing ? 'tarot-table-felt tarot-ceremony relative px-4 pb-2' : ''}>
          {/* No weather layer over the felt (Aaron's item 9): a loop across
              the whole table read as haze on the picture frame, cards
              included. The room's own air is in the footage, where it
              belongs, and the felt stays a table. */}
          {dealing && (
            /* The controls ride the room's dark upper corners — the one
               part of the composition with nothing in it. Left: the act
               (turn, or take the reading). Right: the table's own
               housekeeping. */
            <div className="tarot-ceremony-controls">
              <span className="flex items-center gap-3">
                {!allTurned && (
                  <>
                    <button onClick={turnAll}
                            className="btn btn-primary btn-accent-1">
                      Turn them over
                    </button>
                    <span className="text-xs" style={{ color: 'var(--tarot-felt-text)' }}>
                      or one at a time — {table.turned.length} of {cards.length}
                    </span>
                  </>
                )}
                {lingering && (
                  <>
                    <button onClick={takeReading}
                            className="btn btn-primary btn-accent-1">
                      Begin the reading
                    </button>
                    <span className="text-xs" style={{ color: 'var(--tarot-felt-text)' }}>
                      sit as long as you like — or knock twice on the glass
                    </span>
                  </>
                )}
              </span>
              <span className="flex items-center gap-2">
                <SoundToggle />
                <button onClick={() => { void chooseReader(chosen) }}
                        disabled={shuffling}
                        className="btn btn-felt btn-sm">
                  <HandFanGlyph />
                  {shuffling ? 'Shuffling…' : 'Shuffle again'}
                </button>
                <button onClick={leaveTable}
                        className="btn btn-felt btn-sm">
                  Different reader
                </button>
              </span>
            </div>
          )}
          {dealing && (
            /* Above the spread and centred, which is where a crystal ball
               on a séance table goes — and which deletes a problem rather
               than managing one. Pinned to the felt's right edge it closed
               on the centred spread as the page narrowed, sat across the
               third card everywhere below about a thousand pixels, and had
               to be shrunk and then hidden below `lg` to cope. Standing it
               over the cards means nothing is beside it, so it shows at
               every width and can be the size the thing deserves.

               The knock (item 4): while the spread lingers, two raps on the
               glass take the reading. Double-click only ever PROCEEDS — it
               is decorated with a visible invitation below, so the gesture
               is a flourish, never the only door. */
            <SeanceRoom onKnock={lingering ? takeReading : undefined}
                        vision={lastTurned && lastTurned.image
                          ? { image: lastTurned.image,
                              reversed: lastTurned.reversed }
                          : null} />
          )}
          <Spread cards={cards} turned={table.turned} small={!dealing}
                  onTurn={dealing ? turn : undefined} />
        </div>
      )}

      {/* The act controls (turn them over; begin the reading) live in the
          overlay above — commandment 2 note: the knock is a flourish, and
          the visible "Begin the reading" button beside it is the plain
          door, so the gesture is never the only way forward. */}

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
