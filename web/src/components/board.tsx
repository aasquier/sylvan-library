/**
 * The board: two Commander decks, drawn on the floor of an arena.
 *
 * Until now a Tier 3 match was an *account* — a column of sentences saying
 * what had happened. This is the thing that happened: card art for what is in
 * play, life totals, lands in a row of their own, a graveyard in a stack, and
 * both hands. It moves as the game moves.
 *
 * Five decisions worth keeping.
 *
 * **It draws; it does not decide.** Every judgement about the game was made in
 * Go against a recorded match — which zone a card is in, land row or
 * battlefield row, that the stack is not a zone and the library is never sent
 * at all. `go/internal/sim/tier3/board.go` argues all of it and
 * `lib/board.ts` applies the deltas. What is left here is layout, and that is
 * on purpose: a browser that had to know any Magic to draw this would be a
 * second place for those rulings to rot.
 *
 * **The two seats face each other across a seam**, the way two people sit at a
 * table. The far player's rows run outward from the middle — battlefield
 * nearest the seam, then lands, then their hand at the top edge — and the near
 * player's mirror it. So the creatures that are about to fight are next to
 * each other, which is where a game actually happens.
 *
 * **Hands are shown and the library never is.** Aaron's ruling, and the line
 * is worth stating because Forge reports both: there is no human at this table
 * so nothing is hidden from the engine, and showing a hand is a *broadcast* —
 * the poker hole-card camera. Showing the library would be showing the
 * answers. The library is dropped in Go so it cannot arrive here by accident.
 *
 * **A tapped permanent is turned**, because that is what tapped *is*. No
 * badge, no dimming, no icon: the card leans the way it would on a table, and
 * it turns rather than jumping there. Forty-five degrees rather than ninety —
 * a full quarter-turn reads correctly and costs a card's whole height in
 * width, which on a board of forty permanents is the difference between a row
 * and two rows. Half a turn is unmistakably *turned* and stays inside its own
 * slot (Aaron, 2026-08-25: *"more compact use of space in general"*). What it
 * costs is that a card's corners no longer line up with its slot's, which is
 * what `.field-card-arm` is for.
 *
 * **Nothing here is a plain `<img>` on a plain `<div>`** (commandment 17's
 * spirit one layer down). A card that arrives grows into place, a card that
 * dies is dealt out, a life total that changes takes the hit visibly. The
 * motion is CSS and switches off under `prefers-reduced-motion` beside the
 * rules it turns off, the way the rest of this project does it.
 */

import {
  type CSSProperties, createContext, useContext, useEffect, useRef, useState,
} from 'react'
import { createPortal } from 'react-dom'

import type { ColiseumZone, ForgeBoard } from '../lib/api'
import { CardSheet } from './ui'
import { ThroneGlyph } from './glyphs'
import { KeywordMarks } from './keywords'
import aegisArt from '../assets/coliseum/aegis.webp'
import mementoArt from '../assets/coliseum/memento.webp'
import { type BoardCard, type BoardSide, fightingStats, foldBoard, stackRow }
  from '../lib/board'
import { drawableKeywords } from '../lib/keywords'
import type { Speed, StagedBeat } from '../lib/reel'

/** One card on the field.
 *
 * `key` is the card's Forge instance id, which is what lets React move the
 * same element between zones instead of destroying and rebuilding it — a
 * creature that dies has to be the same DOM node in the graveyard or the
 * animation has nothing to animate.
 */
/**
 * Which way a counter cuts.
 *
 * **The sign is on the kind, not on the number.** `n` is how many counters of
 * that kind are on the card and it is a count — it is never negative. A single
 * -1/-1 arrives as `{kind: '-1/-1', n: 1}`, and reading the sign off `n` drew
 * it as a cheerful green `+1`, which is the exact opposite of the news.
 *
 * Three answers rather than two, because most counters are neither: charge,
 * loyalty, quest and stun counters are not good or bad, they are just counters,
 * and colouring them green would be the board having an opinion it has no
 * basis for.
 */
function counterSign(kind: string): 'up' | 'down' | 'flat' {
  if (kind.startsWith('-')) return 'down'
  if (kind.startsWith('+')) return 'up'
  return 'flat'
}

/**
 * What the beat that just landed did, as a mark on the board.
 *
 * **One mark at a time, and it is the sentence's own.** The room drains beats
 * at reading pace and the board is folded to exactly the same count, so the
 * picture moves when the sentence is spoken — the marks ride that clock rather
 * than inventing a second one. The beat being read is the beat being drawn,
 * and when the next one arrives this one is over. Nothing accumulates, nothing
 * has to be timed out, and a scrub backwards is a mark that simply was not
 * there, because the mark is a *function of the beat* and not a thing that
 * happened once.
 */
type Mark = 'attacks' | 'blocks' | 'dies'

/** Whether a beat's card and a board card are the same card.
 *
 *  Not `===`, and the reason is written down one layer below: Forge names a
 *  **face**, never Scryfall's combined `A // B` (`events.go`, and
 *  `docs/FORGE.md`'s fourth fact). The board's names come from the scribe and
 *  can carry the combined spelling, so a transforming creature would attack,
 *  block and die without a single mark ever landing on it — silently, and only
 *  on the decks that play them. Comparing the front face costs one split. */
function sameCard(onBoard: string, inBeat: string): boolean {
  return onBoard === inBeat || onBoard.split(' // ')[0] === inBeat
}

function markOf(kind: string): Mark | null {
  return kind === 'attack' ? 'attacks'
    : kind === 'block' ? 'blocks'
    : kind === 'dies' ? 'dies'
    : null
}

/** The card the current beat is about, and what happened to it.
 *
 *  A context rather than four more props: `FieldCard` is drawn in the rows, in
 *  a hand and inside every tray, and threading a mark down three levels to
 *  reach all of them would be the same fact written four times. `key` is the
 *  beat's own identity, and it is what makes the second attack by the same
 *  creature animate again rather than sitting there already-animated. */
const Struck = createContext<{ card: string; mark: Mark; key: string } | null>(
  null)

/** The paintings the board's own zones are dressed in, by zone key.
 *
 *  A context rather than a prop threaded through four levels, for `Struck`'s
 *  reason: the rail is drawn twice, once per seat, and both rails want the
 *  same four pictures. Empty is a legible state — the room answers before any
 *  match is asked for, and a zone with no painting is the brass tile it has
 *  always been. */
const Dressing = createContext<Record<string, ColiseumZone>>({})

/** How wide the card held up on hover is drawn, and how much room it needs.
 *
 *  A Scryfall `normal` face is 488x680, so the height follows the width; the
 *  artist line under it is the rest. Both are needed *before* the element
 *  exists, because the placement below decides where to put it rather than
 *  putting it somewhere and correcting. */
const PEEK_W = 300
/** The narrowest a held-up card is allowed to be. Narrower than the smallest
 *  phone this room supports, so it only ever binds on a viewport that is
 *  lying — see the floor's argument in `FieldPeek`. */
const PEEK_MIN_W = 160
const PEEK_RATIO = 680 / 488
const PEEK_GAP = 10
const PEEK_EDGE = 8

/**
 * The one card held up off the board, drawn in the body rather than in the row.
 *
 * **Three separate bugs, one cause.** The preview used to be a sibling `span`
 * inside each card, absolutely positioned and centred on it, and that shape
 * fails three ways at once (Aaron, 2026-08-25):
 *
 * - a card near the left edge had its preview *"clipped by the black border"* —
 *   the field clips its own overflow, and a 300px panel centred on a card 30px
 *   from the edge hangs 120px into the wall;
 * - opening a hand or a graveyard and then hovering a card inside it put a
 *   preview *"in conflict"* with the tray it opened from, because the tray is a
 *   scrolling box and the preview was inside it;
 * - and the cards in those trays are 42 pixels wide, where *"I can't even make
 *   out the printing"*.
 *
 * All three are the same fact: a preview parented to the thing it previews
 * inherits that thing's clipping, its scrolling and its stacking. So it is
 * **portalled to the body and placed in viewport coordinates**, measured from
 * the card's own rectangle — which is also the only way to clamp it, because
 * clamping needs to know where the edges are and CSS centring never does.
 *
 * `position: fixed` alone would not have done it. The board's cards sit inside
 * several transformed ancestors and are *literally mid-rotation*, and a
 * transform makes a new containing block for fixed children — the same trap
 * `CardSheet` documents one file over.
 */
function FieldPeek({ card, at, avoid }: {
  card: BoardCard
  at: DOMRect
  /** The opened tray this card is sitting in, when it is sitting in one.
   *
   *  A hand or a graveyard spread out is a panel somebody opened *in order to
   *  look at it*, and dropping a 300px card into the middle of it covers the
   *  thing they opened (Aaron, 2026-08-25: the full-hand view "conflicts with
   *  the individual hover preview on each card"). Given the panel's rectangle
   *  the preview can step out beside it instead of onto it — so the pile stays
   *  readable and the one card being asked about stands next to it, which is
   *  what picking a card out of a pile looks like. */
  avoid: DOMRect | null
}) {
  const room = document.documentElement
  // **A floor, because a viewport can measure zero.** A background or hidden
  // tab reports `clientWidth: 0` — the whole document does, `vw` included —
  // and the shrink-to-fit above then hands back a *negative* width, which is a
  // card drawn inside out. Nobody is looking at a hidden tab, but they are
  // looking the instant it comes back, and the preview must not be the thing
  // that arrives broken.
  const width = Math.max(PEEK_MIN_W,
    Math.min(PEEK_W, room.clientWidth - 2 * PEEK_EDGE))
  const height = width * PEEK_RATIO + (card.artist ? 18 : 0)
  const fits = (x: number) => x >= PEEK_EDGE
    && x + width <= room.clientWidth - PEEK_EDGE
  const clamp = (v: number, max: number) =>
    Math.min(Math.max(v, PEEK_EDGE), Math.max(max, PEEK_EDGE))

  // Beside the panel when there is one and either flank has room; otherwise
  // the ordinary placement, which will land on top of it — better a covered
  // tray than a preview half off the screen.
  const beside = avoid
    ? [avoid.right + PEEK_GAP, avoid.left - PEEK_GAP - width].find(fits)
    : undefined
  if (beside !== undefined) {
    return draw(beside,
      clamp(at.top + at.height / 2 - height / 2,
        room.clientHeight - height - PEEK_EDGE))
  }
  // Above the card by preference, below it when there is no room above —
  // which is most of the far player's half, and every tray that opened
  // downward.
  const above = at.top - PEEK_GAP - height
  return draw(
    clamp(at.left + at.width / 2 - width / 2,
      room.clientWidth - width - PEEK_EDGE),
    above >= PEEK_EDGE ? above
      : clamp(at.bottom + PEEK_GAP, room.clientHeight - height - PEEK_EDGE))

  function draw(left: number, top: number) {
    return createPortal(
      <span className="field-peek" aria-hidden="true"
            style={{ left, top, width }}>
        <img src={card.image} alt="" draggable={false} />
        {card.artist && (
          <span className="field-peek-artist">art by {card.artist}</span>
        )}
      </span>,
      document.body)
  }
}

function FieldCard({ card, size, count, inPlay = false }: {
  card: BoardCard
  size: 'normal' | 'small'
  /** How many identical cards this one stands for. See `stackRow`. */
  count: number
  /** Whether this card is standing on the battlefield, as opposed to being
   *  held, buried, exiled or waiting in the command zone.
   *
   *  **Only the loupe reads it, and only the loupe should.** Power and
   *  toughness on this board are what a creature is fighting at *now* — the
   *  live figures, counters and anthems included — and that is a question the
   *  battlefield asks and nowhere else does. A card in a hand has printed
   *  numbers and no fight to have them in.
   *
   *  It is also a real fault rather than a nicety. The hand is a fan overlapped
   *  to the 27px strip carrying each card's name, so a card's bottom-right
   *  corner is *under the next card* — and a loupe pinned there is a set of
   *  numbers drawn on somebody else's painting, belonging to a card you cannot
   *  see. Measured on a live board before it was believed. */
  inPlay?: boolean
}) {
  const [held, setHeld] = useState(false)
  // Where this card is standing, the moment a pointer or the keyboard found
  // it — and null the rest of the time, which is what keeps exactly one
  // preview on the page. Measured rather than remembered: a card in a tray
  // that has just scrolled, or one mid-rotation, is not where it was.
  const box = useRef<HTMLDivElement>(null)
  const [at, setAt] = useState<{ card: DOMRect; tray: DOMRect | null } | null>(
    null)
  // Which kind of hand last touched this. Touch browsers fire `mouseenter`
  // synthetically after a tap, so without this a tap would open the sheet and
  // arm a hover preview behind it — `CardHover` learned the same thing.
  const coarse = useRef(false)
  const show = () => {
    if (coarse.current || !card.image || !box.current) return
    // The panel this card is sitting in, if any. Asked of the DOM rather than
    // passed down: the same `FieldCard` is drawn on the sand, in a fan and in
    // four kinds of tray, and threading "are you in a tray" through all of
    // them would be a prop that exists to restate what the tree already says.
    const tray = box.current.closest('.field-tray')
    setAt({
      card: box.current.getBoundingClientRect(),
      tray: tray ? tray.getBoundingClientRect() : null,
    })
  }
  const hide = () => setAt(null)
  // A preview placed in viewport coordinates is wrong the moment the page
  // moves under it, and the room scrolls while a match is playing. Registered
  // only while one is open, so a board of forty cards costs zero listeners at
  // rest rather than forty.
  const showing = at !== null
  useEffect(() => {
    if (!showing) return
    const clear = () => setAt(null)
    window.addEventListener('scroll', clear, true)
    return () => window.removeEventListener('scroll', clear, true)
  }, [showing])
  const stats = inPlay ? fightingStats(card) : null
  const struck = useContext(Struck)
  // Matched on Forge's own spelling, which is what both ends of this carry.
  // Two copies of one name is a token or a basic; marking both is a better
  // wrong answer than marking neither, and in a singleton format it is rare.
  const mark = struck && sameCard(card.name, struck.card) ? struck : null
  // `!== 0` rather than `> 0`: a -1/-1 counter is a counter, and the pile of
  // them on a creature that is about to die is exactly the thing somebody is
  // reading the board to find.
  const counters = card.counters.filter((c) => c.n !== 0)
  // A token's painting is a *chosen* printing (the earliest, which is the
  // original), so the painter is worth naming where a person can find them.
  const title = [
    count > 1 ? `${count} × ${card.name}` : card.name,
    stats,
    counters.map((c) => `${c.n} ${c.kind}`).join(', '),
    card.tapped ? 'tapped' : '',
    inPlay ? drawableKeywords(card.keywords).join(', ') : '',
    card.artist ? `art by ${card.artist}` : '',
  ].filter(Boolean).join(' · ')

  return (
    <div className={`field-card field-card-${size}${card.tapped ? ' is-tapped' : ''}`
                    + (count > 1 ? ' is-stacked' : '')
                    // Still standing, but only for this beat. Every way of
                    // leaving play gets it — died, bounced, exiled, ceased to
                    // exist — because what it draws is *departure*. Which
                    // departure it was is the mark's business.
                    + (card.leaving ? ' is-leaving' : '')
                    + (mark ? ` is-${mark.mark}` : '')}
         ref={box} title={title} tabIndex={card.image ? 0 : -1}
         onPointerDown={(e) => { coarse.current = e.pointerType !== 'mouse' }}
         onPointerUp={(e) => {
           // **The preview is `:hover` and keyboard focus, and a phone is
           // neither.** Forty cards on a floor, none of them readable at forty
           // pixels, and the one mechanism that made them readable needed a
           // pointer — so on a touch screen the whole board was a mosaic. The
           // sheet is the same answer the card lists got: held up, centred,
           // and free of every box on the way out.
           if (e.pointerType === 'mouse' || !card.image) return
           hide()
           setHeld(true)
         }}
         // **The card, readable.** A permanent on this board is fifty-eight
         // pixels of painting — enough to know a Forest from a Dragon and
         // nowhere near enough to read one. These four lift the whole face
         // out at a size a person can actually read; `FieldPeek` decides
         // where it goes and why it is not drawn here.
         onMouseEnter={show} onMouseLeave={hide}
         onFocus={show} onBlur={hide}>
      {/* The pile behind it. Two leaves is enough to read as depth and few
          enough not to fatten the row — a real stack of nine Forests does not
          look nine cards thick from across a table either. */}
      {count > 1 && <span className="field-card-pile" aria-hidden="true" />}
      <div className="field-card-turn">
        {card.image ? (
          <img className="field-card-art" src={card.image} alt={card.name}
               loading="lazy" draggable={false} />
        ) : (
          // No painting is a legible state, not a hole: the pool may not have
          // been refreshed, and a match is worth watching either way.
          <span className="field-card-plate">{card.name}</span>
        )}
        {/* The gold edge belongs to the card, so it turns with the card. */}
        {card.token && <span className="field-card-token" aria-hidden="true" />}
      </div>
      {/* **The arm: everything written in the card's corners.**

          A card's furniture belongs to the card's *corners*, and until now it
          belonged to the slot's — three chips pinned to a box that never
          turned, while the card inside it did. At ninety degrees that was
          survivable, because a card turned ninety degrees still fills the
          corners of its own slot. At forty-five (Aaron, 2026-08-25: *"make
          sure you get any overlays correct"*) it is not: the card's corners
          swing a fifth of its width clear of the slot's, so a count pinned
          top-right of the box floats over the sand, and the counters pinned
          bottom-left sit on the neighbour.

          So the arm turns with the card and each thing on it turns back. The
          furniture rides the corner it names and stays upright to be read,
          which is exactly what a player does with a tapped card: turn the
          card, not your head. Each piece pivots about its own anchor corner,
          so counter-rotating moves it nowhere. */}
      <div className="field-card-arm" aria-hidden="true">
        {/* **What the card does, in the one corner that was free.**
            A painting at fifty-eight pixels tells you this is a Dragon. It
            does not tell you the Dragon flies, and whether it flies is the
            whole question when the other side has ground blockers — so the
            board made you hover forty cards one at a time to find the one
            that could block (Aaron, 2026-08-25, on Arena's keyword icons).

            Only on the battlefield, for `inPlay`'s reason one field up: these
            are facts about a fight, and a card in a hand is not in one. */}
        {inPlay && <KeywordMarks keywords={card.keywords} />}
        {/* **The loupe.** Power and toughness were a black tab printed over
            the corner of the painting at all times — legible, and permanently
            in the way of the one part of a card everybody already looks at.
            The glass replaced the tab, and then hid until hovered, which
            traded one fault for its opposite: a board of forty creatures with
            no numbers on it at all unless you went hunting one at a time
            (Aaron, 2026-08-25: *"what I meant is that it always appeared"*).

            So it is always there and it never turns. It sits where a card's
            own power/toughness box sits, magnifies the painting under it, and
            carries the *current* figures on the glass in crisp type — current
            rather than printed, because a 2/2 with three +1/+1 counters is a
            5/5 and the printed box would be a lie told very clearly. Upright
            through the whole rotation, because the one thing a magnifier is
            for is reading: *"the magnifying glass should always be upright
            and oriented so the viewer can read it"*. */}
        {stats && (
          <span className="field-card-lens"
                style={card.image
                  ? ({ '--lens-art': `url(${card.image})` } as CSSProperties)
                  : undefined}>
            <span className="field-card-lens-glass" />
            <span className="field-card-lens-pt tabular">{stats}</span>
          </span>
        )}
        {count > 1 && (
          <span className="field-card-count tabular">{count}<span
            className="field-card-times">×</span></span>
        )}
        {/* Counters, one chip each rather than one sum. A creature carrying
            three +1/+1 and two -1/-1 was drawn as a "1", which is arithmetic
            the board should not be doing on somebody's behalf — the two kinds
            annihilate as a state-based action, and until they do they are two
            different things on the card. Green for what is being added and red
            for what is being taken away, which is the one colour convention
            every player already has, and brass for the ones that are neither.
            The count carries the sign in type as well, for anybody who does
            not separate those two hues. */}
        {counters.length > 0 && (
          <span className="field-card-counters">
            {counters.map((c) => {
              const way = counterSign(c.kind)
              return (
                <span key={c.kind} title={`${c.n} ${c.kind}`}
                      className={`field-counter tabular is-${way}`}>
                  {way === 'down' ? '-' : way === 'up' ? '+' : ''}{c.n}
                </span>
              )
            })}
          </span>
        )}
      </div>
      {/* **The marks.** Keyed on the beat so the same creature attacking twice
          plays twice — without it React keeps the element and the animation,
          having already run, never runs again.

          Attacking is light and motion rather than an object, and that is a
          choice about *frequency*: a creature is declared an attacker several
          times a turn, and hanging a photograph on the most common beat in the
          game would turn the board into a slideshow. The rare, decisive beats
          get the objects — the shield when something steps in front, the
          Pompeii skull when something dies. */}
      {mark && (
        <span key={mark.key} aria-hidden="true"
              className={`field-mark field-mark-${mark.mark}`}>
          {mark.mark === 'blocks' && (
            <img src={aegisArt} alt="" draggable={false} />
          )}
          {mark.mark === 'dies' && (
            <img src={mementoArt} alt="" draggable={false} />
          )}
        </span>
      )}
      {at && card.image && (
        <FieldPeek card={card} at={at.card} avoid={at.tray} />
      )}
      {held && card.image && (
        <CardSheet name={card.name} image={card.image}
                   onClose={() => setHeld(false)} />
      )}
    </div>
  )
}

/**
 * A row of cards, identical ones stacked.
 *
 * Nine Forests is one pile with a nine on it, which is how they sit on a real
 * table and the only way a row of them fits on a phone. `stackRow` decides
 * what "identical" means, and the answer is *identical in play* — a tapped
 * Forest and an untapped one are two piles, because the difference between
 * them is the thing somebody is looking at the board to find out.
 *
 * The key is the first card's id rather than the stack's position: a pile that
 * grows keeps its element, so its count animates instead of the row rebuilding
 * itself every time a land comes down.
 */
function FieldRow({ label, cards, size = 'normal', empty }: {
  label: string
  cards: BoardCard[]
  size?: 'normal' | 'small'
  /** What to say when the row is empty — and **whether to draw it at all**.
   *
   *  Without one the row simply is not there. Splitting the old single
   *  "permanents" row into artifacts, enchantments and planeswalkers is worth
   *  doing because it is how a player sorts a table, and it is only worth
   *  doing if the rows that hold nothing cost nothing: a deck with no
   *  planeswalkers would otherwise pay a labelled empty row all game, twice,
   *  once per seat. Creatures and lands pass a line instead, because those two
   *  are the spine of a board and an empty one in the early turns is the news
   *  rather than the absence of it. */
  empty?: string
}) {
  const stacks = stackRow(cards)
  if (stacks.length === 0 && !empty) return null
  return (
    <div className="field-row" aria-label={`${label}: ${cards.length}`}>
      {stacks.length === 0 ? (
        <span className="field-row-empty">{empty ?? label}</span>
      ) : stacks.map((stack) => (
        // The only four rows a permanent actually stands in: creatures,
        // artifacts and enchantments, and lands. Everything else that draws a
        // `FieldCard` is a card somebody is holding or has already lost.
        <FieldCard key={stack.card.id} card={stack.card} size={size}
                   count={stack.count} inPlay />
      ))}
    </div>
  )
}

/** Commander's starting life, which is what the ring below is a fraction of.
 *
 *  A constant rather than a reading, because this room plays exactly one
 *  format and the *starting* total is gone by the time a board is folded —
 *  `BoardSide.life` is the current one. If the Coliseum ever runs a format
 *  that starts anywhere else, this is the line that has to learn it. */
const STARTING_LIFE = 40

/** A life total, drawn as the thing everyone at the table is actually
 *  watching.
 *
 * **It was a number in a stone bar** (Aaron, 2026-08-25: *"mega basic, like
 * whiteclaw basic"*), and he is right twice over. Once on looks: 1.28rem of
 * bold type is not a treatment, it is a default. And once on *information* —
 * a bare "23" makes you do the arithmetic that matters, because what a player
 * reads off a life total is not the integer, it is **how much is left**.
 *
 * So it is a ring that drains. The arc is life over forty, the figure sits in
 * the middle, and the whole thing warms from brass to blood as it goes — three
 * ways of saying one fact, for the same reason the counters carry a sign as
 * well as a colour. You can see a player is in trouble from across the room
 * without reading a digit.
 *
 * The flash on change stays: a total that changed silently is a total nobody
 * notices changing. */
function LifeTotal({ life }: { life: number }) {
  const previous = useRef(life)
  const [hit, setHit] = useState<'up' | 'down' | null>(null)
  useEffect(() => {
    if (life === previous.current) return
    const direction = life > previous.current ? 'up' : 'down'
    previous.current = life
    setHit(direction)
    const id = window.setTimeout(() => setHit(null), 700)
    return () => window.clearTimeout(id)
  }, [life])
  // Clamped both ways: a player on a lifegain deck goes past forty and the
  // ring simply reads full, and a dead player reads empty rather than negative.
  const left = Math.max(0, Math.min(1, life / STARTING_LIFE))
  // The mix is computed here rather than in CSS because a `calc()` inside
  // `color-mix()`'s percentage is the one part of that function browsers still
  // disagree about, and this is a colour nobody should have to debug.
  const spent = `${Math.round((1 - left) * 100)}%`
  return (
    <span className={`field-life${hit ? ` is-${hit}` : ''}`}
          style={{ '--life-left': left, '--life-spent': spent } as CSSProperties}
          title={`${life} life`}>
      <svg viewBox="0 0 48 48" aria-hidden="true" focusable="false">
        <circle className="field-life-track" cx="24" cy="24" r="20" />
        <circle className="field-life-arc" cx="24" cy="24" r="20" />
      </svg>
      <span className="field-life-n tabular">{life}</span>
    </span>
  )
}

/**
 * A closed zone drawn as the pile it is: the top card, with a count.
 *
 * The graveyard and exile were numbers on the rail, which is what a scoreboard
 * does and not what a table does — you can see somebody's graveyard from
 * across a table, and the *top* card of it is the one that matters, because
 * that is the one everything in Magic reaches for. The command zone is here
 * for the same reason: in Commander it is where the game's most important card
 * waits, and a number cannot say which commander is home and which is out.
 */
function FieldPile({ label, cards, short, zone, throne, receiving }: {
  label: string
  cards: BoardCard[]
  short: string
  /** Which of the board's own zones this is, which is how it finds its
   *  painting in `Dressing`. */
  zone: 'command' | 'graveyard' | 'exile'
  /** The command zone, which is the one pile whose *emptiness* is news. */
  throne?: boolean
  /** The beat's key when *this* seat's grave is the one receiving a death, and
   *  null otherwise.
   *
   *  **Decided by the rail, not here**, and it has to be: the card whose death
   *  is being announced is held on the *battlefield* for that beat now, so a
   *  graveyard cannot answer "did I just get it?" by looking at what it holds —
   *  it holds nothing yet. The rail can see the side that is holding the body.
   *  Without this both graves raised a ghost for one death. */
  receiving?: string | null
}) {
  // Held open by a tap. Hover and keyboard focus open it in CSS; this is for
  // the pointer that has no hover to give.
  const [open, setOpen] = useState(false)
  // **The skull used to land here, and no longer does.** Forge reports a death
  // and the zone change on one line, so by the time the room said "X dies" the
  // card was already in this pile and there was no instant at which the board
  // held a dead creature standing. The grave was the only surface the mark
  // could reach — a headstone rather than a death. `foldBoard` holds the dying
  // card in its own row for the length of its beat now, so the skull lands on
  // the card, which is where Aaron asked for it and where it belongs.
  const top = cards[cards.length - 1]
  const seat = throne && !top
  // **The zone, dressed.** These three were three-letter labels on a 26px
  // tile, which is what a scoreboard does and not what a table does — a player
  // knows the graveyard, exile and the command zone by sight (Aaron,
  // 2026-08-25: *"icons to represent the graveyard and exile"*, and the
  // command zone *"its own area of interest"*). The painting is Magic's own,
  // pinned to a printing in checked-in prose, and it sits *under* the pile's
  // top card rather than instead of it: a graveyard with cards in it still
  // shows what is on top, and the ground says which graveyard it is.
  const dressing = useContext(Dressing)
  const dressed = dressing[zone]
  const ghost = dressing.ghost
  // Keyed on the beat so a second death raises a second ghost rather than
  // reusing a finished animation — `Struck`'s own trick, one level out.
  const arriving = receiving ?? null
  const title = [
    top ? `${label}: ${cards.length}, ${top.name} on top`
      : seat ? `${label}: empty — out on the battlefield`
      : `${label}: empty`,
    // Named because somebody painted it, and because rule 9 says so.
    dressed ? `${dressed.card}, art by ${dressed.art.artist}` : '',
  ].filter(Boolean).join(' · ')
  return (
    <div className="field-pile-wrap">
    <div className={`field-pile${cards.length === 0 ? ' is-empty' : ''}`
                    + (seat ? ' is-throne' : '')}
         title={title} aria-label={`${label}: ${cards.length}`}
         tabIndex={cards.length > 0 ? 0 : -1}
         onPointerUp={(e) => {
           if (e.pointerType === 'mouse' || cards.length === 0) return
           setOpen((was) => !was)
         }}>
      {dressed && (
        <img className="field-pile-ground" src={dressed.art.url} alt=""
             loading="lazy" draggable={false} />
      )}
      {top && top.image ? (
        <img className="field-pile-art" src={top.image} alt="" loading="lazy"
             draggable={false} />
      ) : null}
      {/* **An empty command zone means the commander is on the table**, which
          is the opposite of an empty graveyard and was drawn the same way.
          The chair says which: theirs, and nobody in it. */}
      {seat && <span className="field-pile-throne"><ThroneGlyph /></span>}
      {/* **The ghost, rising off the grave.** Two beats, two pictures: the
          skull lands on the creature *as it dies*, held on the sand for that
          beat, and this rises from the zone that received it. Magic's own
          spectre rather than a photograph — everything photographic that reads
          as a ghost is pale and low-contrast, and this mark lives about a
          second. */}
      {arriving && ghost && (
        <span key={arriving} className="field-pile-ghost" aria-hidden="true">
          <img src={ghost.art.url} alt="" draggable={false} />
        </span>
      )}
      <span className="field-pile-label">{short}</span>
      <span className="field-pile-n tabular">{cards.length}</span>
    </div>
    {/* **The pile, opened out.** A closed zone drawn as its top card answers
        one question — what is on top — and a graveyard is asked a different
        one all game: *what is in there*. You can pick up somebody's graveyard
        at a real table and look through it, and it is public information in
        every format, so there was never a reason this could not be read.

        It spills onto the sand rather than out of the frame: the field clips
        its own overflow, so a tray hung outside the rail would be cut in half.
        Down from the far player's rail and up from the near player's, which
        is the only direction each has room in.

        Hover keeps it open across the gap because the tray is inside the same
        wrapper as the pile — the pointer never leaves the hover target on its
        way in, which is the trap that makes most hover panels unusable. */}
    {cards.length > 0 && (
      <div className={`field-tray${open ? ' is-open' : ''}`}
           role="group" aria-label={`${label}, all ${cards.length}`}>
        <span className="field-tray-head">
          {label}<span className="field-tray-n tabular">{cards.length}</span>
        </span>
        <div className="field-tray-cards">
          {cards.map((c) => (
            <FieldCard key={c.id} card={c} size="small" count={1} />
          ))}
        </div>
      </div>
    )}
    </div>
  )
}

/** The stone rail one player's name, life and closed zones are carved into. */
function FieldRail({ side, facing }: {
  side: BoardSide
  facing: 'far' | 'near'
}) {
  // **Whose grave is about to receive the body.** The dying card is held on
  // the sand for the beat that announces it, so no graveyard can answer this
  // by looking at what it holds — it holds nothing yet. The side that is
  // holding the body is the side whose grave it is bound for, and a rail is
  // the first thing up the tree that knows which side it is drawing. Without
  // this, one death raised a ghost over both players' graves.
  const struck = useContext(Struck)
  const holding = struck?.mark === 'dies' && [side.creatures, side.walkers,
    side.artifacts, side.enchantments, side.land].some((row) =>
    row.some((c) => c.leaving && sameCard(c.name, struck.card)))
  // Two generic for each previous cast from the zone. With partners this
  // reports the dearer of the two: there is one pile and forty pixels of it,
  // and the expensive commander is the one whose price changes a decision.
  const tax = 2 * side.commanders.reduce((n, c) => Math.max(n, c.casts), 0)
  return (
    <div className={`field-rail field-rail-${facing}`}>
      {/* **The nameplate is gone, and the stone bar with it.** The name was
          said twice — the hand beside this rail already carries it, and a
          board that tells you the same thing in two places has spent the room
          twice (Aaron, 2026-08-25). The bar it sat in was a grey strip with a
          hatch on it, which is furniture pretending to be material: the sand
          is the material, and three lit places standing on it is a table.
          `title` keeps the full deck name reachable for anybody who wants it. */}
      <span className="field-rail-totals" title={side.name}>
        <FieldPile label="Command zone" short="Command Zone"
                   cards={side.command} zone="command" throne />
        {/* **Beside the pile rather than on it.** Inside, the chip covered
            the zone's own three-letter label — the first draft read "CI +4",
            which is a worse pile than one with no tax on it at all. Twenty-six
            pixels does not hold two facts, so the price stands next to the
            thing it is the price of. */}
        {tax > 0 && (
          <span className="field-tax tabular"
                title={`Commander tax: +${tax} to cast from the command zone`}>
            +{tax}
          </span>
        )}
        <FieldPile label="Graveyard" short="Graveyard" cards={side.graveyard}
                   zone="graveyard"
                   receiving={holding && struck ? struck.key : null} />
        <FieldPile label="Exile" short="Exile" cards={side.exile}
                   zone="exile" />
        <LifeTotal life={side.life} />
      </span>
    </div>
  )
}

/**
 * A hand, held at the side of the table rather than laid out on it.
 *
 * **A hand is not on the battlefield, and it used to be drawn as though it
 * were** — a full-width row in the same stack as lands and creatures, one per
 * seat. Eight rows for two players, two of them cards nobody has played yet,
 * and the field itself squeezed for the room (Aaron, 2026-08-25: *"maybe it
 * isn't in the field but is to the side to give more room for cards"*). He is
 * describing a real table: your hand is in your hand, off to one side, and the
 * sand is for what has been committed to it.
 *
 * So the cards overlap the way cards in a hand overlap, each showing the strip
 * that carries its name, and the whole hand costs one narrow column instead of
 * a row across the field. Below `--field-wide` the column has nowhere to go
 * and the fan turns on its side into a strip, which is the same gesture read
 * across instead of down.
 *
 * Not `stackRow`: two copies of the same card in a hand are two cards, and a
 * hand is small enough that seeing seven of them is the point. On the
 * battlefield stacking nine Forests into one is a mercy; here it would be a
 * lie about how many cards somebody is holding.
 */
function FieldHand({ side, name, facing }: {
  side: BoardSide
  name: string
  facing: 'far' | 'near'
}) {
  // **The nameplate is the handle, and the fan is not.**
  //
  // The whole hand used to be the hover target, so running the pointer along
  // the fan to read one card sprang the entire hand open underneath it — two
  // panels answering one gesture, fighting over the same patch of sand (Aaron,
  // 2026-08-25: the full-hand view "conflicts with the individual hover
  // preview on each card"). They are two different questions: *what is this
  // one card* is the fan's, and the preview answers it; *show me the whole
  // hand* is the nameplate's.
  //
  // Held in state rather than in `:hover`, because the tray opens across a gap
  // and past the fan — a CSS hover group would have to be the whole hand
  // again, which is the bug. The delay on the way out is what lets the pointer
  // cross that gap.
  const [spread, setSpread] = useState(false)
  const [over, setOver] = useState(false)
  const leaving = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(leaving.current), [])
  const enter = () => { window.clearTimeout(leaving.current); setOver(true) }
  const leave = () => {
    window.clearTimeout(leaving.current)
    leaving.current = window.setTimeout(() => setOver(false), 130)
  }
  const open = (over || spread) && side.hand.length > 0
  return (
    <div className={`field-hand field-hand-${facing}`
                    + (spread ? ' is-spread' : '')}>
      {/* **The hand opens out too, and it opens the same way the piles do.**
          Seven cards overlapped to the 27px strip that carries a name is how a
          hand is *held*; it is not how one is read.

          Spreading the fan in place was the first answer and the geometry
          refuses it: on a wide screen the fan is a *column* inside a 112px
          rail, so sliding seven cards apart needs 400 pixels of a space that
          has 250, and the accordion becomes a scrollbar in a gutter. A tray
          onto the sand has the whole arena to open into, and it means all four
          zones — hand, graveyard, exile, command — answer one gesture with one
          kind of panel instead of four with two.

          A real button, because it is a real disclosure: it has to be
          reachable by keyboard (the tray is `visibility: hidden` until it
          opens, so nothing inside it can be tabbed to first) and it has to say
          whether it is open. A `span` did neither. */}
      <button type="button"
              className={`field-hand-label${open ? ' is-open' : ''}`}
              aria-expanded={open} disabled={side.hand.length === 0}
              onMouseEnter={enter} onMouseLeave={leave}
              onFocus={enter} onBlur={leave}
              onClick={() => setSpread((was) => !was)}>
        {name}<span className="field-hand-n tabular">{side.hand.length}</span>
      </button>
      {side.hand.length > 0 && (
        <div className={`field-tray field-hand-tray${open ? ' is-open' : ''}`}
             role="group" aria-label={`Hand, all ${side.hand.length}`}
             onMouseEnter={enter} onMouseLeave={leave}
             onFocus={enter} onBlur={leave}>
          <span className="field-tray-head">
            Hand<span className="field-tray-n tabular">{side.hand.length}</span>
          </span>
          <div className="field-tray-cards">
            {side.hand.map((card) => (
              <FieldCard key={card.id} card={card} size="small" count={1} />
            ))}
          </div>
        </div>
      )}
      <div className="field-hand-fan"
           aria-label={`Hand: ${side.hand.length}`}>
        {side.hand.length === 0 ? (
          <span className="field-hand-empty">an empty hand</span>
        ) : side.hand.map((card) => (
          <FieldCard key={card.id} card={card} size="small" count={1} />
        ))}
      </div>
    </div>
  )
}

/**
 * One player's half of the field.
 *
 * `facing` is which edge of the table they sit at, and it does one thing:
 * reverses the row order. The far player's battlefield is nearest the seam and
 * their lands are at the outer edge, which is exactly how it looks from the
 * other side of a real table. The hand is no longer among these rows — it is
 * held at the side (`FieldHand` above).
 */
function FieldSide({ side, facing }: {
  side: BoardSide
  facing: 'far' | 'near'
}) {
  // **Outermost first**, and the near player's side is the same list reversed,
  // so the two creature rows finish up either side of the seam.
  //
  // Five rows where there were three. Two of them draw only when they hold
  // something, and that is the difference between organising a board and
  // padding it: artifacts and enchantments come and go, planeswalkers may
  // never appear at all, and a permanently empty row labelled "planeswalkers"
  // is furniture that costs a phone a third of its board. Creatures and lands
  // always draw — those two are the spine, and an empty one early is news.
  const rows = [
    <FieldRow key="land" label="Lands and mana" cards={side.land} size="small"
              empty="no lands yet" />,
    <FieldRow key="ench" label="Enchantments" cards={side.enchantments}
              size="small" />,
    <FieldRow key="arti" label="Artifacts" cards={side.artifacts}
              size="small" />,
    <FieldRow key="walk" label="Planeswalkers" cards={side.walkers}
              size="small" />,
    <FieldRow key="crea" label="Creatures" cards={side.creatures}
              empty="no creatures" />,
  ]
  return (
    <div className={`field-side field-side-${facing}`}>
      <div className="field-rows">
        {facing === 'far' ? rows : [...rows].reverse()}
      </div>
    </div>
  )
}

/**
 * The transport: watch it at a speed a person can follow, or walk it by hand.
 *
 * **The Forge is not slowed down for this and never waits for it.** It plays
 * its games flat out and the results land when they land; these buttons govern
 * only how fast the *room* reads them back. A match is a measurement and
 * watching one is a performance, and pacing the measurement to suit the
 * performance would be the wrong trade in both directions.
 *
 * Stepping and scrubbing are possible at all because the board is a **pure
 * fold over a count** — the board after n beats needs nothing but n — so
 * backwards costs exactly what forwards costs. The controls are a second way
 * to set that number, not a second engine.
 */
function FieldTransport({ speed, setSpeed, at, of, seek,
  games, playing: onGame, chooseGame }: {
  speed: Speed
  setSpeed: (s: Speed) => void
  at: number
  of: number
  seek: (to: number) => void
  /** Every game the match has finished, in order. One while it is still being
   *  played; all of them once it is over. */
  games: number[]
  /** Which one is on the field. */
  playing: number
  chooseGame: (game: number) => void
}) {
  const running = speed !== 'paused'
  return (
    <div className="field-transport">
      {/* Which game. A place rather than an action, so `.strip-tab`'s
          relatives rather than `.btn` — and only once there is more than one
          to choose between, because a lone tab labelled "Game 1" is a control
          that cannot do anything. */}
      {games.length > 1 && (
        <div className="field-games" role="group" aria-label="Which game">
          {games.map((n) => (
            <button key={n} type="button"
                    className={`chip-toggle field-game${n === onGame ? ' is-active' : ''}`}
                    aria-pressed={n === onGame}
                    onClick={() => chooseGame(n)}>
              {n}
            </button>
          ))}
        </div>
      )}
      <div className="field-transport-buttons">
        <button type="button" className="btn btn-sm field-step"
                onClick={() => { setSpeed('paused'); seek(at - 1) }}
                disabled={at <= 0} aria-label="Back one beat">
          <span aria-hidden="true">◀◀</span>
        </button>
        <button type="button"
                className={`btn btn-sm${running ? ' is-on' : ''}`}
                onClick={() => setSpeed(running ? 'paused' : 'play')}
                aria-label={running ? 'Pause' : 'Play'}>
          <span aria-hidden="true">{running ? '❙❙' : '▶'}</span>
        </button>
        <button type="button" className="btn btn-sm field-step"
                onClick={() => { setSpeed('paused'); seek(at + 1) }}
                disabled={at >= of} aria-label="Forward one beat">
          <span aria-hidden="true">▶▶</span>
        </button>
      </div>

      {/* Places rather than actions, so `.chip-toggle` rather than `.btn` —
          a speed is a setting you are *in*, not a thing you do. */}
      <div className="field-speeds" role="group" aria-label="Speed">
        {(['slow', 'play', 'fast'] as const).map((s) => (
          <button key={s} type="button"
                  className={`chip-toggle field-speed${speed === s ? ' is-active' : ''}`}
                  aria-pressed={speed === s}
                  onClick={() => setSpeed(s)}>
            {s === 'slow' ? 'Slow' : s === 'play' ? 'Watch' : 'Fast'}
          </button>
        ))}
      </div>

      <label className="field-scrub">
        <span className="sr-only">Scrub through the game</span>
        <input type="range" min={0} max={Math.max(of, 1)} value={at}
               onChange={(e) => { setSpeed('paused'); seek(Number(e.target.value)) }} />
      </label>
      <span className="field-scrub-at tabular">{at}/{of}</span>
    </div>
  )
}

/**
 * The field.
 *
 * `shown` is how many beats the room has told, and the board is folded to
 * exactly that many steps — the server builds one step per beat, so the
 * picture moves when the sentence is spoken and there is one clock rather than
 * two to keep in step.
 */
export function MatchBoard({ board, shown, game, name, running, beat,
  speed, setSpeed, of, seek, games, playing, chooseGame, zones = [] }: {
  board: ForgeBoard | null
  /** The paintings the board's own zones wear, from `/api/coliseum`. Checked-in
   *  prose, so it arrives before any match does; empty is a legible state and
   *  draws the brass tiles the rail has always had. */
  zones?: ColiseumZone[]
  shown: number
  /** The beat the room has just spoken, which is the one the board marks.
   *  Null before a game starts, and while the account is silent. */
  beat?: StagedBeat | null
  game: number
  /** Turns a seat's slug into whatever the room calls that deck. Passed in
   *  because only the room has the shelf. */
  name: (slug: string | null, fallback: string) => string
  running: boolean
  speed: Speed
  setSpeed: (s: Speed) => void
  /** How many beats this game has in total, told and untold. */
  of: number
  seek: (to: number) => void
  games: number[]
  playing: number
  chooseGame: (game: number) => void
}) {
  const dressing: Record<string, ColiseumZone> = {}
  for (const z of zones) dressing[z.key] = z
  const state = foldBoard(board, shown)
  const far = state.sides[0]
  const near = state.sides[1]
  // One mark, belonging to one beat. No `useMemo`: the compiler does that,
  // and identity is not what governs replay here anyway — every mark is keyed
  // on `beat.key`, so a fresh object with the same key reconciles onto the
  // same element and does *not* restart an animation that is already running.
  const mark = beat ? markOf(beat.kind) : null
  const struck = mark && beat?.card
    ? { card: beat.card, mark, key: beat.key }
    : null

  if (!board || !far || !near) {
    return (
      <section className="field field-quiet" aria-label="The battlefield">
        <div className="field-floor" aria-hidden="true" />
        <p className="field-waiting">
          {running
            ? 'The gates are open and the field is still empty. The first '
              + 'cards are dealt when the first game begins.'
            : 'No battlefield was drawn for this match. It was played by a '
              + 'worker that reports the result but not the board, so the '
              + 'account beside this is the whole of what it saw.'}
        </p>
      </section>
    )
  }

  return (
    <Dressing.Provider value={dressing}>
    <Struck.Provider value={struck}>
    <section className="field" aria-label="The battlefield">
      {/* The arena floor: sand, and the dust that never quite settles. */}
      <div className="field-floor" aria-hidden="true">
        <span className="field-dust field-dust-1" />
        <span className="field-dust field-dust-2" />
        <span className="field-dust field-dust-3" />
      </div>

      {/* **A hand belongs to the player holding it, not to the furniture.**
          Both used to live in one rail, which is right on a wide screen — the
          rail runs the height of the table and each hand sits beside its own
          seat. On a phone that rail has nowhere to go and becomes a strip at
          the foot, and there both hands ended up under the near player's
          half: the far player's cards stacked below the near player's own,
          two seats away from the person holding them (Aaron, 2026-08-25, from
          his phone). Now each hand is its own grid area and travels with its
          seat at every width — above the far half, below the near one, which
          is where the two players' hands actually are. */}
      <FieldHand side={far} facing="far" name={name(far.slug, far.name)} />

      <FieldSide side={far} facing="far" />
      <FieldRail side={far} facing="far" />

      {/* The seam: in the real building, the trench the lifts came up through.
          Here it is where the turn is announced and where the two
          battlefields meet. */}
      <div className="field-seam">
        <span className="field-seam-rule" aria-hidden="true" />
        <span className="field-seam-turn tabular">
          {state.turn > 0 ? `Turn ${state.turn}` : 'Before the first turn'}
          {game > 0 && <span className="field-seam-game">Game {game}</span>}
        </span>
        <span className="field-seam-rule" aria-hidden="true" />
      </div>

      <FieldRail side={near} facing="near" />
      <FieldSide side={near} facing="near" />

      <FieldHand side={near} facing="near"
                 name={name(near.slug, near.name)} />


      <FieldTransport speed={speed} setSpeed={setSpeed} at={shown} of={of}
                      seek={seek} games={games} playing={playing}
                      chooseGame={chooseGame} />
    </section>
    </Struck.Provider>
    </Dressing.Provider>
  )
}
