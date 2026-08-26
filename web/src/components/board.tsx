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
  type CSSProperties, type ReactNode, createContext, useContext, useEffect,
  useRef, useState,
} from 'react'
import { createPortal } from 'react-dom'

import type { ColiseumZone, ForgeBoard } from '../lib/api'
import { CardSheet } from './ui'
import { CrownGlyph, HandFanGlyph, HornGlyph, ThroneGlyph } from './glyphs'
import { KeywordMarks } from './keywords'
import { ManaProduced } from './manasymbol'
import { COLOR_VAR, producedColors, producedName } from '../lib/mtg'
import aegisArt from '../assets/coliseum/aegis.webp'
import ensisArt from '../assets/coliseum/ensis.webp'
import mementoArt from '../assets/coliseum/memento.webp'
import { type BoardCard, type BoardSide, type BoardStack, fightingStats,
  foldBoard, sameCard, stackRow } from '../lib/board'
import { drawableKeywords } from '../lib/keywords'
import { tokenSigil } from '../lib/tokens'
import { stepToTurn } from '../lib/theater'
import { beatDelay, type Speed, type StagedBeat } from '../lib/reel'
import { CenterStage } from './stage'

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
 * than inventing a second one. The beat being read is the beat being drawn.
 * Nothing accumulates and nothing piles up, because there is only ever one
 * mark: a new one replaces the old one on the spot.
 *
 * What a mark no longer does is *end* when its beat does. See [MARK_LIFE].
 */
type Mark = 'attacks' | 'blocks' | 'dies'

/** A legend's name as a player says it: the part before the title.
 *
 *  "Thrasios, Triton Hero" is *Thrasios* at a table, and on a tile this size
 *  it has to be — the whole string is either three-point type or an ellipsis
 *  that eats the half doing the work. The full name stays in the `title` and
 *  in the accessible label, so nothing is lost, only folded.
 *
 *  Magic writes a legend's title two ways and this reads both:
 *  `Kaheera, the Orphanguard` cuts at the comma, and `Tymna the Weaver` cuts
 *  at "the" — but **only when "the" follows the first word**, which is the
 *  whole guard. `Jhoira of the Ghitu` also contains " the " and cutting on it
 *  would leave "Jhoira of", so the pattern is anchored rather than searched.
 *  Anything else is drawn whole, which is right: a name with no title in it
 *  is already short. */
function calledBy(name: string): string {
  const [head] = name.split(',')
  const short = head?.trim() || name
  return /^\S+ the /.exec(short) ? short.split(' ')[0] ?? short : short
}

function markOf(kind: string): Mark | null {
  return kind === 'attack' ? 'attacks'
    : kind === 'block' ? 'blocks'
    : kind === 'dies' ? 'dies'
    : null
}

/**
 * How long a mark is *seen*, in milliseconds.
 *
 * **A mark is timed by the mark, and it used to be timed by the beat.** That
 * is the whole of this fix. Every mark was a pure function of the beat —
 * raised when the sentence was spoken, gone the instant the next one landed —
 * which is a lovely property and had one consequence nobody could read off the
 * stylesheet. A beat at watching pace is 480ms, so a 900ms animation was being
 * cut off *before it was half told*: what a person saw was a shield swinging
 * in and then disappearing, never the block landing, never the settle, never
 * the skull's long hold. Lengthening the CSS changed nothing at all, because
 * the CSS was never what ended it (Aaron, 2026-08-26: the marks "need to
 * linger at least 30% longer").
 *
 * So each mark names its own length, and this number is both the animation's
 * duration and the element's lifetime. It reaches the stylesheet as a custom
 * property on the stage rather than being written down twice, so the two can
 * never drift: whatever is here is what a person watches.
 *
 * The invariant that is kept is **one mark at a time**. A new mark still
 * replaces the old one immediately — two marks a beat apart on two cards is a
 * board asking to be read in two places at once. All that has changed is that
 * a *silent* beat no longer takes a mark down with it.
 */
const MARK_LIFE: Record<Mark, number> = {
  /* 900 -> 1250. The commonest beat in the game, so it stays the shortest of
     the three even after the lift; a red lamp that outstays the others turns a
     combat into a traffic light. */
  attacks: 1250,
  /* 1000 -> 1800, the largest lift of the three and the one Aaron named twice:
     the shield is an interception, and an interception that is over in a
     second was never seen to intercept anything. */
  blocks: 1800,
  /* 1500 -> 2000. The one beat in a game somebody might want a moment to
     register, which was already the argument for its hold; it now gets the
     hold it was written for. */
  dies: 2000,
}

/** The most beats a mark may outlive its own.
 *
 *  A cap rather than a scale, because the pace is a reading speed and a mark
 *  is a fact about the game: slowing the room down is not a reason to hold a
 *  skull for six seconds, and speeding it up is not a reason to make one
 *  invisible. At watching pace and slower this never binds — five beats at
 *  480ms is longer than the longest mark — and it exists for the fast end,
 *  where 150ms a beat would otherwise leave a skull sitting on a board that
 *  moved on thirteen beats ago. */
const MARK_BEATS = 5
/** ...and the floor under it, so no pace can clip a mark to a flicker. */
const MARK_FLOOR = 640
/** A breath past the animation, so the element is taken away *after* it has
 *  faded out rather than popping from under its own last frame. */
const MARK_TAIL = 90

function markLife(mark: Mark, speed: Speed): number {
  const beat = beatDelay(speed)
  // Paused is not a slow pace, it is the absence of one: nothing is draining,
  // so there is nothing for the mark to outlive and nothing to cap it against.
  if (beat === 0) return MARK_LIFE[mark]
  return Math.max(MARK_FLOOR, Math.min(MARK_LIFE[mark], beat * MARK_BEATS))
}

/** A mark on a card: what happened to it, and which beat said so. */
interface Struckdown { card: string; mark: Mark; key: string }

/**
 * Hold a mark for its own length rather than for its beat's.
 *
 * Three things this must not do, and each is a line of it:
 *
 * - **It must not pile marks up.** It holds one value, so it cannot. A mark
 *   that arrives while another is up replaces it, on the spot.
 * - **It must not leak a timer.** Every timeout is cleared by the effect that
 *   set it — on the next beat, on a speed change, and on unmount.
 * - **It must not carry a mark across a game.** Choosing another game of the
 *   same match keeps this component mounted, and a shield held over from game
 *   one could land on a same-named creature in game two. The hold is dropped
 *   at the boundary rather than left to time out across it.
 *
 * And one thing the caller must do, which is why this takes no decision about
 * it: while the transport is **paused** nothing is draining, so there is
 * nothing to outlive. `MatchBoard` reads the beat's own mark there instead, so
 * a scrub lands on the board its beat describes and never on the one before —
 * which is the property the pure-function version had, kept exactly where it
 * still matters.
 *
 * **The mark is chosen during the render, and only the taking-away is a
 * timer.** Raising it from an effect would draw it one commit *after* the
 * sentence that raised it, and half a frame of drift between the words and the
 * picture is the one thing this room has been careful about from the start.
 * React re-runs a render that sets its own state before it commits anything,
 * so the beat and its mark still reach the screen together.
 */
function useHeldMark(card: string | null, mark: Mark | null, key: string,
  speed: Speed, game: number): Struckdown | null {
  const [held, setHeld] = useState<Struckdown | null>(null)
  const [wasGame, setWasGame] = useState(game)
  // Null rather than `key`, and never the empty string: a board that mounts
  // with a beat already in hand — every scrub, every game chosen from the
  // tabs, every reload of a finished match — has to raise that beat's mark
  // rather than wait for the next one. `''` is a key `beat?.key ?? ''` can
  // really produce, so the sentinel has to be something a key cannot be.
  const [wasKey, setWasKey] = useState<string | null>(null)
  const raised = card && mark ? { card, mark, key } : null
  if (game !== wasGame) {
    setWasGame(game)
    setWasKey(key)
    setHeld(raised)
  } else if (key !== wasKey) {
    setWasKey(key)
    // A beat with nothing to mark leaves whatever is up alone — that silence
    // is the whole of what changed here.
    if (raised) setHeld(raised)
  }
  useEffect(() => {
    if (!held) return
    // Changing pace part-way through a mark re-times it from that moment,
    // which is the answer somebody asking for a different pace wants.
    const done = window.setTimeout(() => setHeld(null),
      markLife(held.mark, speed) + MARK_TAIL)
    return () => { window.clearTimeout(done) }
  }, [held, speed])
  return held
}

/** The card the current beat is about, and what happened to it.
 *
 *  A context rather than four more props: `FieldCard` is drawn in the rows, in
 *  a hand and inside every tray, and threading a mark down three levels to
 *  reach all of them would be the same fact written four times. `key` is the
 *  beat's own identity, and it is what makes the second attack by the same
 *  creature animate again rather than sitting there already-animated. */
const Struck = createContext<Struckdown | null>(null)

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
/** The narrowest a preview is worth *stepping aside* for.
 *
 *  **A floor on the decision, not on the panel.** It used to be a floor on the
 *  width itself — "never draw one narrower than this" — which is a rule that
 *  can only be kept by hanging the panel off the edge of the screen, and on a
 *  narrow window it did exactly that. What is actually being decided is which
 *  is the lesser evil when an open tray leaves no room beside it: a whole card
 *  drawn on top of the tray, or a legible one next to it. Wider than this,
 *  beside; narrower, on top. Twice the size of the cards inside a tray, so
 *  "beside" always means *bigger than what you were already looking at*. */
const PEEK_MIN_W = 160
const PEEK_RATIO = 680 / 488
const PEEK_GAP = 10
const PEEK_EDGE = 8

/** A rectangle of free space, in viewport coordinates. */
interface Clear { x0: number; x1: number; y0: number; y1: number }

/** What a card is to the deck that brought it, when it is anything special. */
type Leads = 'commander' | 'companion'

/**
 * Which cards on the sand lead the deck they came from.
 *
 * **The command zone can only say a commander is *home*.** Once it is cast it
 * stands in the creature row like any other body, and until now nothing said
 * which of forty permanents was the one the whole deck is built around (Aaron,
 * 2026-08-26). In Commander that is the single most load-bearing card on the
 * table: it is what the removal is pointed at, what the tax is counted for,
 * and what the other player is playing around.
 *
 * A context rather than a prop, for `Struck`'s reason — `FieldCard` is drawn
 * in five rows through two wrappers, and threading one boolean down all of
 * them would be the same fact written five times. Provided by `FieldSide`, so
 * it covers **the battlefield and nothing else**: a crown in a hand fan would
 * be a mark drawn on the neighbouring card, which is the fault `inPlay`
 * already documents one field down.
 *
 * The answer comes from Go — `forgeBoardSeat.Commanders` and `.Companion`,
 * matched to board ids on the server and read off `BoardSide` here. A browser
 * that decided which card was the commander would be a second place for the
 * companion rules to rot.
 */
const Crowned = createContext<ReadonlyMap<number, Leads>>(new Map())

/** Every card on one side that wears a mark, by board id. */
function crownedOn(side: BoardSide): ReadonlyMap<number, Leads> {
  const out = new Map<number, Leads>()
  for (const c of side.thrones) out.set(c.id, 'commander')
  if (side.companion) out.set(side.companion.id, 'companion')
  return out
}

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
  const doc = document.documentElement
  // **A hidden tab measures zero and every sum below inherits the lie.** A
  // background tab reports `clientWidth: 0` — the whole document does, `vw`
  // included — and a panel sized against that comes out negative, which is a
  // card drawn inside out. Nobody is looking at a hidden tab; they are looking
  // the instant it comes back, and the preview must not be the thing that
  // arrives broken. So a zero reads as "the room the panel wants" rather than
  // as a room with nothing in it.
  const wide = doc.clientWidth > 0 ? doc.clientWidth : PEEK_W + 2 * PEEK_EDGE
  const tall = doc.clientHeight > 0 ? doc.clientHeight
    : PEEK_W * PEEK_RATIO + 2 * PEEK_EDGE
  // The artist line under the painting is part of what has to fit.
  const chrome = card.artist ? 18 : 0
  const whole: Clear = { x0: PEEK_EDGE, x1: wide - PEEK_EDGE,
    y0: PEEK_EDGE, y1: tall - PEEK_EDGE }
  /** The widest panel a clearing can hold — capped at the size it wants, and
   *  bounded by the *height* as well, which is the half the old placement
   *  never asked. A card is taller than it is wide, so a short clearing is a
   *  narrower panel and not a clipped one. */
  const fit = (c: Clear) => Math.max(0, Math.min(PEEK_W, c.x1 - c.x0,
    (c.y1 - c.y0 - chrome) / PEEK_RATIO))

  // **Four ways round an open tray, not two.**
  //
  // The old rule tried the right flank, then the left, and gave up — so a tray
  // whose left edge was under about 318px offered neither, and the preview
  // fell through to the ordinary placement and landed *on the tray it came out
  // of*. On a phone that is every tray there is, and the hands sit in the left
  // column at every width above 62rem, which is why Aaron only ever saw it on
  // the left (2026-08-26: "full hand previews look clipped when they are on
  // the lefthand side").
  //
  // Above and below are the two that were missing, and on a narrow viewport
  // they are the only two: a 375px screen has no flank wide enough for
  // anything, and plenty of page over and under a 268px panel.
  const rooms: Clear[] = avoid ? [
    { ...whole, x0: avoid.right + PEEK_GAP },
    { ...whole, x1: avoid.left - PEEK_GAP },
    { ...whole, y0: avoid.bottom + PEEK_GAP },
    { ...whole, y1: avoid.top - PEEK_GAP },
  ] : []
  // The side with the most room, ties going to the earlier one — which puts
  // the right flank first, then the left, then under, then over. That order is
  // the reading order for a panel that opens downward.
  let best: Clear | undefined
  for (const c of rooms) if (!best || fit(c) > fit(best)) best = c
  // **A sliver beside the tray is worse than a whole card over it.** Below the
  // floor there is no placement worth having, and the honest answer is the one
  // this has always given in that corner: cover the tray rather than hang the
  // panel off the screen.
  const clear = best && fit(best) >= PEEK_MIN_W ? best : whole
  const width = Math.max(1, fit(clear))
  const height = width * PEEK_RATIO + chrome
  /** Centred on the card, then pushed inside the clearing. `Math.max` on the
   *  far edge rather than a bare subtraction, so a clearing smaller than the
   *  panel still starts at its own near edge instead of before it. */
  const into = (lo: number, hi: number, want: number, size: number) =>
    Math.min(Math.max(want, lo), Math.max(lo, hi - size))
  const x = into(clear.x0, clear.x1, at.left + at.width / 2 - width / 2, width)
  if (clear !== whole) return draw(x, into(clear.y0, clear.y1,
    at.top + at.height / 2 - height / 2, height))
  // Nothing to step around: above the card by preference, below it when there
  // is no room above — which is most of the far player's half, and every tray
  // that opened downward. Never *on* the card, which is the one place the
  // answer is already showing.
  const above = at.top - PEEK_GAP - height
  return draw(x, above >= clear.y0 ? above
    : into(clear.y0, clear.y1, at.bottom + PEEK_GAP, height))

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

/**
 * One card, at the one size this board draws cards.
 *
 * **There used to be two, and the small one was unreadable.** Lands,
 * artifacts, enchantments and planeswalkers were drawn at 42x59 and only
 * creatures at 58x81, and the board draws the *whole card face* rather than an
 * art crop — so at forty-two pixels the printed type under the painting was a
 * grey smear that reads as a rendering fault rather than as small text (Aaron,
 * 2026-08-26: *"creatures up front are big enough their visible text isn't
 * distracting... all cards should be at least the size we have been using on
 * creatures so the text doesn't look funny"*).
 *
 * It cost four rows per seat about twenty-two pixels each, which is real on a
 * phone and is the trade he asked for. Nothing else had to move: identical
 * cards already collapse into one stack (`stackRow`), so the rows that hold
 * many of one thing — lands, a dozen Treasures — were never the rows paying
 * for the width.
 */
function FieldCard({ card, count, inPlay = false }: {
  card: BoardCard
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
  // Only on the sand: see `Crowned`. A commander waiting at home already has a
  // throne of its own, and a card in a fan is overlapped to a 27px strip that
  // belongs to the card in front of it.
  const leads = useContext(Crowned).get(card.id) ?? null
  const struck = useContext(Struck)
  // Matched on Forge's own spelling, which is what both ends of this carry.
  // Two copies of one name is a token or a basic; marking both is a better
  // wrong answer than marking neither, and in a singleton format it is rare.
  const mark = struck && sameCard(card.name, struck.card) ? struck : null
  // `!== 0` rather than `> 0`: a -1/-1 counter is a counter, and the pile of
  // them on a creature that is about to die is exactly the thing somebody is
  // reading the board to find.
  const counters = card.counters.filter((c) => c.n !== 0)
  // **What a turned permanent taps for**, and only while it is turned.
  //
  // Derived entirely from state a fold already settled — tapped, and what the
  // printing produces — so it can never accumulate, leak between cards, or
  // survive a scrub back past the tap. There is nothing here to clean up
  // because there is nothing here that is remembered.
  //
  // Not on a card in a hand, for `inPlay`'s reason: a card that is not on the
  // battlefield is not tapped for anything.
  const makes = inPlay && card.tapped ? producedColors(card.makes) : []
  // A token's painting is a *chosen* printing (the earliest, which is the
  // original), so the painter is worth naming where a person can find them.
  const title = [
    count > 1 ? `${count} × ${card.name}` : card.name,
    // Said in words as well as drawn, because the crown is a picture and a
    // picture is a thing you have to already know (commandment 2).
    inPlay && leads === 'commander' ? 'the commander'
      : inPlay && leads === 'companion' ? 'the companion' : '',
    stats,
    counters.map((c) => `${c.n} ${c.kind}`).join(', '),
    card.tapped ? 'tapped' : '',
    // **"taps for", not "made".** The bead is a fact about the printing shown
    // while the permanent is turned — see `BoardCard.makes`. A creature can be
    // turned because it attacked, and this sentence stays true when it was.
    makes.length ? `taps for ${producedName(makes)}` : '',
    inPlay ? drawableKeywords(card.keywords).join(', ') : '',
    card.artist ? `art by ${card.artist}` : '',
  ].filter(Boolean).join(' · ')

  return (
    <div className={`field-card${card.tapped ? ' is-tapped' : ''}`
                    + (count > 1 ? ' is-stacked' : '')
                    + (inPlay && leads ? ` is-${leads}` : '')
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
      {/* **The pile behind it, and it is made of the card it is a pile of.**

          It used to be two blank grey leaves offset up and to the left, which
          says *something is behind this* and nothing about what — and what is
          behind it is the whole reason a stack exists (Aaron, 2026-08-26:
          *"stacks of cards... should look stacked on the deck with some
          effect, but they should also have a stacked effect on hover, or a
          fanned hand look"*). Every card in a stack is identical by
          construction — `stackRow` merges only what is indistinguishable in
          play — so a leaf wearing the same painting is not a decoration
          standing in for the pile, it is a truthful picture of the next card
          down.

          At rest they sit a couple of degrees out of true, the way a pile
          somebody has been drawing off does. Hover or focus fans them, which
          is what a thumb does to a pile you are counting.

          **The arc is fixed and the density is what grows.** Two leaves for a
          pair, four for a pile of four or more — and both fans reach exactly
          as far, because the outermost leaf is what the arena's wall measures
          against (`.field-card-leaf` does that arithmetic). A dozen Treasures
          fanned twelve deep would cover the rest of the row to say a number
          that is already written on the card; four edges in the same sweep is
          what a thicker pile looks like under a thumb anyway. */}
      {count > 1 && (
        <span className="field-card-pile" aria-hidden="true"
              style={card.image
                ? ({ '--leaf-art': `url(${card.image})` } as CSSProperties)
                : undefined}>
          {(count >= 4 ? [-1, -0.42, 0.42, 1] : [-1, 1]).map((leaf) => (
            <span key={leaf} className="field-card-leaf"
                  style={{ '--leaf': leaf } as CSSProperties} />
          ))}
        </span>
      )}
      <div className="field-card-turn">
        {card.image ? (
          <img className="field-card-art" src={card.image} alt={card.name}
               loading="lazy" draggable={false} />
        ) : (
          // No painting is a legible state, not a hole: the pool may not have
          // been refreshed, and a match is worth watching either way.
          <span className="field-card-plate">{card.name}</span>
        )}
        {/* The gold edge belongs to the card, so it turns with the card — and
            so does the material on it, for the same reason: light lies on gold
            at whatever angle the gold is sitting. `lib/tokens.ts` decides
            which material, or none. */}
        {card.token && <span className={tokenSigil(card.name, card.types)}
                             aria-hidden="true" />}
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
        {/* **The crown, sitting on the top edge of the painting.**

            Aaron offered three: oversized, a golden aura, or a crown touching
            the top of the art. It is the last two together and deliberately
            not the first — every card on this board is now one size (see
            above), and the one exception to that would land on the row that
            can least afford it while saying, in the same gesture, something
            about the card's *stature* rather than its role. A Llanowar Elves
            commander is not a bigger card.

            So: a standing gold rim on the card, and a crown on its brow. Both
            are steady, and that is what keeps them clear of the attack, block
            and death marks a beat away — those flash and are gone, this is
            simply true for as long as the card is standing there. Panache and
            martial prowess: the metal is the same brass the rest of this room
            is trimmed in, and the mark is a drawn one rather than a photograph
            at fourteen pixels.

            A companion gets the horn it already wears in the command zone,
            in the vine green that zone gives it — the same two signs in the
            same two colours, wherever the card happens to be standing. */}
        {inPlay && leads && (
          <span className={`field-card-crown is-${leads}`}>
            {leads === 'commander' ? <CrownGlyph /> : <HornGlyph size={14} />}
          </span>
        )}
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
        {/* **The bead: what this turned permanent taps for.**

            A board of forty cards showed a Llanowar Elves lying sideways and
            said nothing about why (Aaron, 2026-08-26: *"I would like a mana
            symbol to appear in mana dorks tapped for mana"*). A tapped card is
            the most legible state on this table and the least informative one:
            turned is turned, whether it swung, blocked or paid for something.

            **What it claims, exactly.** This permanent is turned, and this
            permanent taps for {'{'}G{'}'}. It does *not* claim this activation
            filled anybody's pool — nothing on the wire can say that, and ADR
            44 is clear that the board holds state and never a guess. The
            source of a mana is erased before the board ever sees it: the mana
            event carries a seat and a pool, the tap event carries a card, and
            no key joins them. So the honest sentence is the one about the
            printing, which is true whatever the card was turned for — and on
            a real board a turned mana creature nearly always did just tap for
            it, which is why the mark is worth drawing at all.

            The right edge, because it is the only side of a 58px card that is
            still free, and because it makes a system out of what was going to
            be a leftover: **the left edge is what the card does in a fight**
            (the keyword marks), **the right edge is what it pays for.** Clear
            of the count above it and the loupe below it, at every count of
            both. */}
        {makes.length > 0 && (
          <span className="field-card-bead"
                title={`taps for ${producedName(makes)}`}
                // The aura takes the mana's own colour when there is one to
                // take. Three or more has no colour to be, so it falls to the
                // brass this room is trimmed in rather than picking a winner
                // out of five. A custom property and not a `style` on the glow
                // itself, so the `:hover` and the breath can still reach it.
                style={makes.length <= 2 && makes[0]
                  ? ({ '--bead-glow': COLOR_VAR[makes[0]] } as CSSProperties)
                  : undefined}>
            <span className="field-card-bead-mark">
              <ManaProduced colors={makes} size={15} />
            </span>
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

          All three beats carry a real object now. Attacking used to be light
          and motion alone, on the argument that it is the most frequent event
          in the game and a photograph on it would make a slideshow of the
          board; what that actually produced was a shield for the creature that
          stepped in front and a coloured glow for the one that swung, which
          reads as the block being the decisive half. The sword answers the
          frequency worry by being fast rather than by being absent — it falls
          across the card and is gone. The stylesheet's marks block carries the
          argument in full. */}
      {mark && (
        <span key={mark.key} aria-hidden="true"
              className={`field-mark field-mark-${mark.mark}`}>
          {mark.mark === 'attacks' && (
            <img src={ensisArt} alt="" draggable={false} />
          )}
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
        // **The count travels with the card into the sheet**, because the fan
        // behind a stack is a hover and a phone has no hover to give. The
        // sheet is the whole of what a touch user gets from a card, so it is
        // the one place "there are twelve of these" has to be sayable there.
        //
        // **And so does what the card is carrying.** `FieldGeared` tucks a
        // creature's swords and Auras under it as corners, which says *that*
        // it is carrying something and never *what* — the reading is a hover,
        // and a phone has none. So the sheet takes the whole assemblage and
        // riffles through it, the creature first. A card with nothing on it
        // passes an empty list and the sheet stays exactly one card.
        <CardSheet name={count > 1 ? `${count} × ${card.name}` : card.name}
                   image={card.image} worn={card.attachments}
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
function FieldRow({ label, cards, empty }: {
  label: string
  cards: BoardCard[]
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
        <FieldGeared key={stack.card.id} stack={stack} />
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
function FieldPile({ label, cards, short, zone, seat: kind, solo,
                    badge, receiving }: {
  label: string
  cards: BoardCard[]
  short: string
  /** Which of the board's own zones this is, which is how it finds its
   *  painting in `Dressing`. */
  zone: 'command' | 'graveyard' | 'exile'
  /** Which seat of the command zone this is, when it is one of them.
   *
   *  **The command zone is the one region whose *emptiness* is news**, and
   *  there are two ways for one of its seats to be empty. A commander is out
   *  on the sand, and its chair says so. A companion has been bought into a
   *  hand for {3} — a departure no other card in the zone can make — and a
   *  horn says that instead. A blank tile would say neither. */
  seat?: 'throne' | 'companion'
  /** A seat holds one named card, so it does not carry a count.
   *
   *  "1" beside a commander is a number nobody asked for, and "0" beside an
   *  empty chair is worse: the chair has already said it. A graveyard's count
   *  is the whole point of a graveyard and keeps it. */
  solo?: boolean
  /** A chip pinned to this tile's bottom-right corner, where a Magic card
   *  keeps the number that says what it is worth right now.
   *
   *  Only the command zone uses it, for the commander tax — but it is a
   *  general corner rather than a tax-shaped hole, because it is the *card's*
   *  corner and the loupe one row up already proves what belongs there. */
  badge?: ReactNode
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
  // **Who is holding this tray open, in three states rather than two.**
  //
  // `null` — nobody has said. A pointer that has hover to give, and the
  // keyboard, decide it in CSS.
  // `true` — held open by a tap or a keypress, for the pointer that has no
  // hover to give.
  // `false` — **shut on purpose**, which is a different thing from "not
  // opened" and has to be, because on a touch screen `:hover` *latches* on
  // the tapped element and does not let go until the next tap lands
  // somewhere else. A boolean could only stop asserting `is-open`, and the
  // latched hover went on holding the panel up: the tray had no way to shut
  // (Aaron, 2026-08-26, on the live site: *"when I click in a graveyard or
  // command zone and it expands, it is awkward to get it to collapse again.
  // A touch outside the zone or on the same zone itself should easily
  // collapse it."*). `is-shut` is that third state, and every rule that
  // opens the tray steps aside for it.
  const [open, setOpen] = useState<boolean | null>(null)
  /** The zone entire — the pile and the tray that opens off it — which is
   *  what "outside" is measured against. */
  const zoneBox = useRef<HTMLDivElement>(null)
  /** Shut it, and mean it. `false` rather than `null`, so a latched `:hover`
   *  and a standing keyboard focus both stand down. */
  const shut = () => setOpen(false)
  // **The two ways out of an open tray that a person tries without being
  // told**, and neither existed: a tap anywhere else, and Escape.
  //
  // Registered only while this tray is the one being held open, so a band of
  // six zones costs no listeners at rest — `FieldCard`'s scroll listener
  // keeps the same discipline one component up, and for the same reason.
  //
  // **It is also the whole of the one-tray-at-a-time rule.** Opening the
  // graveyard while the command zone is spread lands a pointer outside the
  // command zone, so the command zone shuts itself on the way past. Nothing
  // has to know about anything else, and there is no shared "which one is
  // open" for two panels to disagree about.
  const holding = open === true
  useEffect(() => {
    if (!holding) return
    const away = (e: PointerEvent) => {
      const hit = e.target as Element | null
      if (!hit || !zoneBox.current) return
      // Inside the zone is not outside it. **This is the trap**: a document
      // listener that skips this test shuts the graveyard the instant
      // somebody reaches for a card in the graveyard.
      if (zoneBox.current.contains(hit)) return
      // A card lifted out of *this* tray is drawn in a dialog portalled to
      // the body, so it is outside this wrapper by construction and is not a
      // dismissal. Shutting the tray behind it would mean putting a card
      // down and finding the pile you took it from had closed itself.
      if (hit.closest('[role="dialog"]')) return
      shut()
    }
    // Escape from wherever the keyboard happens to be. A tap opens this tray
    // without focusing anything, so the wrapper's own `onKeyDown` — which
    // only ever sees keys pressed *inside* the zone — cannot be the whole
    // answer for a tray somebody opened with a finger and then reached for
    // the keyboard to dismiss.
    const key = (e: KeyboardEvent) => { if (e.key === 'Escape') shut() }
    document.addEventListener('pointerdown', away, true)
    document.addEventListener('keydown', key)
    return () => {
      document.removeEventListener('pointerdown', away, true)
      document.removeEventListener('keydown', key)
    }
  }, [holding])
  // **The skull used to land here, and no longer does.** Forge reports a death
  // and the zone change on one line, so by the time the room said "X dies" the
  // card was already in this pile and there was no instant at which the board
  // held a dead creature standing. The grave was the only surface the mark
  // could reach — a headstone rather than a death. `foldBoard` holds the dying
  // card in its own row for the length of its beat now, so the skull lands on
  // the card, which is where Aaron asked for it and where it belongs.
  const top = cards[cards.length - 1]
  const empty = kind && !top ? kind : null
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
  // **A seat says what it is, in words a first game can follow.**
  //
  // The command zone was the one place on this board saying things nobody
  // could act on. Aaron, 2026-08-26: *"hovering on the command zone pops up
  // some things I don't understand, like 'Olinda the Oblivious (99)'s effect?
  // I don't get that."* Two separate faults met there — a Forge EFFECT card
  // leaking past a filter on the server, which is being fixed where it is
  // made, and this: a zone drawn as a *pile* answers "how many, what is on
  // top", and neither question is one anybody asks the command zone.
  //
  // His ruling settles what it may say at all: *"at most it should just be two
  // slots for partners, one for a singular commander, or a second companion
  // devoted slot for Kaheera, et al. Those are the only combinations possible
  // in that zone."* So a seat names its occupant and where that occupant is,
  // and that is the whole vocabulary. The zone's own catch-all is drawn only
  // when the server named no seats at all, and even then it counts rather than
  // naming — the pile it is standing in for is exactly the pile whose contents
  // could not be trusted.
  const seated = kind
    ? `${label} — the ${kind === 'throne' ? 'commander' : 'companion'}, `
    : ''
  const title = [
    top && kind ? `${seated}waiting in the command zone`
      : empty === 'throne' ? `${seated}out on the battlefield`
      : empty === 'companion' ? `${seated}already called into hand`
      : top && solo ? label
      : zone === 'command' ? `${label}: ${cards.length}`
      : top ? `${label}: ${cards.length}, ${top.name} on top`
      : `${label}: empty`,
    // Named because somebody painted it, and because rule 9 says so.
    dressed ? `${dressed.card}, art by ${dressed.art.artist}` : '',
  ].filter(Boolean).join(' · ')
  return (
    // The seat travels out to the wrapper as well as the tile, because how
    // much of the zone a place takes is a property of its *slot* — and the
    // slot is the flex item. A commander's chair takes a full share; the
    // companion's takes less than one, which is what "off to the side" is
    // in a row of flex items.
    <div className={`field-pile-wrap${kind ? ` field-seat-wrap-${kind}` : ''}`}
         ref={zoneBox}
         // A mouse arriving is the one signal that says the hand on this
         // machine has hover to give after all — so forget that a tap once
         // shut this zone, and let `:hover` do its job again. Nothing else
         // clears `is-shut`, because nothing else should: on a phone it has
         // to outlive every latched hover there is.
         onPointerEnter={(e) => {
           if (e.pointerType === 'mouse') setOpen(null)
         }}
         // Escape, from anywhere inside the zone — the tile, or a card down
         // in the open tray. React's `onKeyDown` is the bubbling kind, so
         // one handler on the wrapper covers both without the tray needing
         // to know it is being watched.
         onKeyDown={(e) => { if (e.key === 'Escape') shut() }}
         // Focus has left the zone altogether, so the shutting is spent:
         // arriving back here by keyboard should open it the way a first
         // arrival does, not find a panel that remembers being dismissed.
         onBlur={(e) => {
           if (!e.currentTarget.contains(e.relatedTarget)) {
             setOpen((was) => (was === false ? null : was))
           }
         }}>
    <div className={`field-pile${cards.length === 0 ? ' is-empty' : ''}`
                    + (empty ? ' is-vacant' : '')
                    + (kind ? ` field-seat field-seat-${kind}` : '')}
         title={title}
         aria-label={solo ? title : `${label}: ${cards.length}`}
         tabIndex={cards.length > 0 ? 0 : -1}
         // **The thing you opened is the thing that closes it.** A tap
         // toggles, from any of the three states — which is what a person
         // tries first, and until now the second tap did nothing they could
         // see.
         onPointerUp={(e) => {
           if (e.pointerType === 'mouse' || cards.length === 0) return
           setOpen((was) => was !== true)
         }}
         // The keyboard's half of the same toggle. Focus alone opens the
         // tray (see `.field-pile:focus-visible` in the stylesheet), so this
         // is what gives somebody who pressed Escape a way back in without
         // having to tab away and return.
         onKeyDown={(e) => {
           if (cards.length === 0) return
           if (e.key !== 'Enter' && e.key !== ' ') return
           e.preventDefault()   // Space scrolls the page otherwise.
           setOpen((was) => was !== true)
         }}>
      {dressed && (
        <img className="field-pile-ground" src={dressed.art.url} alt=""
             loading="lazy" draggable={false} />
      )}
      {top && top.image ? (
        <img className="field-pile-art" src={top.image} alt="" loading="lazy"
             draggable={false} />
      ) : null}
      {/* **An empty command zone means the card is somewhere else**, which
          is the opposite of an empty graveyard and was drawn the same way.
          The mark says which seat it is and where its occupant went: a chair
          with nobody in it, or a horn that has been blown. */}
      {empty && (
        <span className="field-pile-throne">
          {empty === 'throne' ? <ThroneGlyph /> : <HornGlyph />}
        </span>
      )}
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
      {badge}
      {!solo && <span className="field-pile-n tabular">{cards.length}</span>}
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
      <div className={`field-tray${open === true ? ' is-open' : ''}`
                      + (open === false ? ' is-shut' : '')}
           role="group" aria-label={`${label}, all ${cards.length}`}>
        <span className="field-tray-head">
          {label}<span className="field-tray-n tabular">{cards.length}</span>
        </span>
        <div className="field-tray-cards">
          {cards.map((c) => (
            <FieldCard key={c.id} card={c} count={1} />
          ))}
        </div>
      </div>
    )}
    </div>
  )
}

/**
 * A permanent and whatever is attached to it, drawn as one thing.
 *
 * **Because they are one thing.** An Aura or an Equipment on the battlefield
 * used to stand in the artifacts or enchantments row, which is a list of loose
 * objects with no way to tell which creature across the seam was carrying
 * which — and this is Commander, where suiting one creature up is a whole way
 * to win a game. Forge has always known; its *log* never said, which is the
 * same hole ADR 42 was written about.
 *
 * So they tuck under, stepped down and to the right, the way you slide a sword
 * under the creature holding it at a real table. What each one shows is a
 * corner — not enough to read, and it is not meant to be read: what the board
 * has to say at this size is **that** this creature is carrying something and
 * *how many* somethings, and both of those are legible from the steps alone.
 * Reading them is the loupe's job, and every tucked card keeps its own hover
 * preview.
 *
 * The wrapper reserves the overhang as margin rather than letting it lie over
 * the next card in the row. A stack that overlapped its neighbour would be
 * saying the neighbour is part of it.
 */
function FieldGeared({ stack }: {
  stack: BoardStack
}) {
  const { card, count } = stack
  if (card.attachments.length === 0) {
    return <FieldCard card={card} count={count} inPlay />
  }
  const worn = card.attachments.map((a) => a.name).join(', ')
  return (
    <div className="field-geared"
         style={{ '--gear': card.attachments.length } as CSSProperties}
         role="group" aria-label={`${card.name}, carrying ${worn}`}>
      {/* First in the DOM so the host paints over them: both are positioned,
          neither carries a z-index, and among positioned elements at auto the
          later one wins. A z-index here would be a stacking context fighting
          the loupe and the card preview, which are the two things on this
          board that have to escape one. */}
      {card.attachments.map((a, i) => (
        <span className="field-gear" key={a.id}
              style={{ '--i': i + 1 } as CSSProperties}>
          <FieldCard card={a} count={1} inPlay />
        </span>
      ))}
      <FieldCard card={card} count={count} inPlay />
    </div>
  )
}

/**
 * How wide the companion's slot is, against a commander's chair at one.
 *
 * Under one because the companion is not one of them, and far enough under to
 * be read as a side rather than as a fourth chair — but not so far that the
 * horn stops being legible on a phone, where the whole rail gives its tiles
 * about 55px and this slot gets two thirds of one.
 *
 * Handed to the stylesheet as `--aside` rather than written down in both
 * places: the group's total share is arithmetic here and the slot's share is a
 * flex rule there, and the two have to be the same number or the zone's frame
 * fits its contents at exactly one commander count.
 */
const COMPANION_SHARE = 0.72

/** The stone rail one player's name, life and closed zones are carved into. */
function FieldRail({ side, facing, name }: {
  side: BoardSide
  facing: 'far' | 'near'
  /** Whose zones these are. The nameplate came off the old grey bar and back
   *  onto *this*, which is the difference between a label floating on a strip
   *  and a heading over the thing it names. */
  name: string
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
  // **Two generic for each previous cast from the zone, per chair.**
  //
  // This used to be one number for the whole zone — the dearer of a pairing's
  // two — because there was one pile forty pixels wide and no way to say which
  // commander the price belonged to. There are chairs now, and a price on the
  // chair is both truer and shorter: Thrasios costs nothing to recast and
  // Tymna costs four, which is two facts a pairing's pilot actually uses and
  // one number could never carry.
  //
  // A companion is never priced, and now it structurally cannot be: the badge
  // rides its own seat, and the companion's seat is not one of these. That was
  // a real bug once — a Kaheera bought into a hand read as a commander being
  // cast — and this is the shape that stops it recurring.
  const taxOn = (c: BoardCard) => 2 * c.casts
  // The zone-wide figure, for the fallback pile below and nothing else.
  const tax = 2 * side.commanders.reduce((n, c) => Math.max(n, c.casts), 0)
  const price = (n: number) => n > 0 ? (
    <span className="field-tax tabular"
          title={`Commander tax: it costs ${n} more to cast this from the `
            + 'command zone'}>
      +{n}
    </span>
  ) : null
  // **Nothing stands in this zone that is not a seat.**
  //
  // The catch-all pile used to draw alongside the seats whenever the zone was
  // holding anything else, and what it actually drew was a Forge EFFECT card —
  // an internal object with a name like "X's effect" that means nothing to
  // anybody at the table (Aaron, 2026-08-26). The server is closing that leak
  // at its source; this is the other half, and it is a design ruling rather
  // than a patch: *"at most it should just be two slots for partners, one for
  // a singular commander, or a second companion devoted slot for Kaheera, et
  // al. Those are the only combinations possible in that zone."*
  //
  // So the pile survives only as the fallback for a board whose seats the
  // server never named — a mid-deploy skew, or an older worker — where it is
  // the only thing that can say the zone is occupied at all.
  const unseated = side.thrones.length === 0
  // How many shares of the rail the command zone takes: one per chair, and
  // never fewer than one, so a board with no seats named still draws a tile
  // the size the old single pile was.
  //
  // **The companion is not a chair and no longer takes a chair's share.** It
  // had an equal tile beside the thrones, edged in its own colour, and an
  // equal box beside a box is a second zone however it is labelled (Aaron,
  // 2026-08-26: *"companions shouldn't be in their own mini zone, they should
  // just be in the main command zone to the side"*). It is in this zone — it
  // really does sit there — so it belongs inside the zone's own frame, at the
  // side, in a slot narrower than the places the commanders keep.
  const chairs = Math.max(1, side.thrones.length + (unseated ? 1 : 0))
  const seats = chairs + (side.companion ? COMPANION_SHARE : 0)
  return (
    <div className={`field-rail field-rail-${facing}`}>
      <span className="field-rail-totals">
        {/* **The command zone is a row of seats, not a pile.**
            A pile answers "how many", and the command zone is never asked
            that — it is asked *who is home*. One pile could not say it for a
            pairing at all: two commanders stacked into one tile showed the
            top one and hid the other, and the one you could not see was as
            likely as not the one that mattered. So each commander gets a
            chair of its own, in the order `deck.yaml` names them, and a
            companion gets a narrower slot at the side of the same zone — it
            sits in here, and it is not one of them.

            The group takes a share of the rail per chair it holds, so a deck
            with one commander is drawn exactly as it was before this and a
            pairing does not squeeze the graveyard to make room.

            **And the group is the zone.** The frame is here now rather than
            on each tile, so what a player sees is one command zone with
            places inside it — the chairs, and the companion at the side —
            instead of two or three separate boxes that have to be read as
            belonging together. */}
        <span className="field-command"
              style={{ '--seats': seats,
                       '--aside': COMPANION_SHARE } as CSSProperties}>
          {/* The label is the card's own name now rather than a sentence
              about the zone — the seat phrases the rest of it, because the
              seat is the thing that knows whether its occupant is home. */}
          {side.thrones.map((c) => (
            <FieldPile key={c.id} label={c.name}
                       short={calledBy(c.name)} zone="command"
                       cards={c.zone === 'command' ? [c] : []}
                       badge={price(taxOn(c))}
                       seat="throne" solo />
          ))}
          {side.companion && (
            <FieldPile key={side.companion.id} zone="command"
                       label={side.companion.name}
                       short={calledBy(side.companion.name)}
                       cards={side.companion.zone === 'command'
                         ? [side.companion] : []}
                       seat="companion" solo />
          )}
          {/* The whole zone, for a board shaped before the server named the
              seats — the one case left where nothing knows which commander a
              price belongs to, so it carries the dearer of them. See
              `unseated` above. */}
          {unseated && (
            <FieldPile label="Command zone" short="Command Zone"
                       cards={side.command} zone="command" seat="throne"
                       badge={price(tax)} />
          )}
        </span>
        <FieldPile label="Graveyard" short="Graveyard" cards={side.graveyard}
                   zone="graveyard"
                   receiving={holding && struck ? struck.key : null} />
        <FieldPile label="Exile" short="Exile" cards={side.exile}
                   zone="exile" />
        {/* **Whose places these are, over the number everybody is watching.**

            The name was a heading on a line of its own across the top of the
            panel, and a line holding one short string and nothing else reads
            as a border somebody put a word in (Aaron, 2026-08-26: *"it would
            be a better use of space if the commander name was stacked above
            the life total, then the zones in general would not have a weird
            top border with only the commander name"*). He is right about the
            space and righter about the pairing: a player's name and a player's
            life are one fact — *how this player is doing* — and they belong in
            one column, at the end of the row, where a scoreboard puts them.

            `title` keeps the deck's full name for anybody who wants it, which
            is what the heading was doing for the names too long to draw. */}
        <span className="field-rail-who">
          <span className="field-rail-name" title={side.name}>{name}</span>
          <LifeTotal life={side.life} />
        </span>
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
        {/* **The sign says which pile of cards this is.**

            Four plates on this board open four different things, and this one
            was a deck's name over a row of card backs with nothing to say that
            what it holds is a *hand* (Aaron, 2026-08-26: *"we need a hand icon
            to show that is what we are showing people with the cards in
            hand — maybe a stylized fanned out set of cards like a magic or
            poker hand?"*). The mark was already drawn, for the tarot table's
            own hand of cards; it had simply never been asked to work here.

            It fans wider while the hand is open, which is commandment 17 in
            its smallest possible form: the control answers the hand that
            reaches for it, and it answers by doing the thing it is a picture
            of. Beside a label rather than instead of one — an icon-only
            control asks a newcomer to already know the app. */}
        <span className="field-hand-mark" aria-hidden="true">
          <HandFanGlyph size={13} open={open} />
        </span>
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
              <FieldCard key={card.id} card={card} count={1} />
            ))}
          </div>
        </div>
      )}
      <div className="field-hand-fan"
           aria-label={`Hand: ${side.hand.length}`}>
        {side.hand.length === 0 ? (
          <span className="field-hand-empty">an empty hand</span>
        ) : side.hand.map((card) => (
          <FieldCard key={card.id} card={card} count={1} />
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
function FieldSide({ side, facing, active }: {
  side: BoardSide
  facing: 'far' | 'near'
  /** Whether it is this seat's turn. Neither half is lit before the first
   *  turn has begun, which is a true thing to say about that moment rather
   *  than a gap to fill. */
  active: boolean
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
    <FieldRow key="land" label="Lands and mana" cards={side.land}
              empty="no lands yet" />,
    <FieldRow key="ench" label="Enchantments" cards={side.enchantments} />,
    <FieldRow key="arti" label="Artifacts" cards={side.artifacts} />,
    <FieldRow key="walk" label="Planeswalkers" cards={side.walkers} />,
    <FieldRow key="crea" label="Creatures" cards={side.creatures}
              empty="no creatures" />,
  ]
  return (
    // **The crowns are provided here and nowhere higher**, because a card is
    // only this side's commander — the same context around both seats would
    // mark one player's Tymna in the other player's row. This is also the
    // narrowest scope that covers the battlefield and no hand or tray, which
    // is the scope the mark is for. See `Crowned`.
    <Crowned.Provider value={crownedOn(side)}>
    <div className={`field-side field-side-${facing}${active ? ' is-active' : ''}`}>
      {/* **Whose turn it is, as light rather than as a border.**
          A Magic table is lit from wherever the game currently is, and the
          half that is not on turn is simply a little further from the torches
          — so the active side gets a low sun coming across the sand from the
          seam, and the other side goes quietly into shadow. The light lies on
          the *floor*, under every card: a scrim over the cards would dim the
          art on the half a person is most likely to be reading, which is the
          exact opposite of the point.

          It is a state and not an entrance. A match is about a hundred and
          thirty beats and the turn changes constantly; an animation that
          restarted at every turn would be a strobe by turn six. What moves is
          a seven-second swell and a warm pool drifting across the sand —
          slower than anything else on the board, so it is felt rather than
          watched. */}
      <span className="field-side-lit" aria-hidden="true" />
      <div className="field-rows">
        {facing === 'far' ? rows : [...rows].reverse()}
      </div>
    </div>
    </Crowned.Provider>
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
function FieldTransport({ speed, setSpeed, at, of, seek, turns = [],
  games, playing: onGame, chooseGame }: {
  speed: Speed
  setSpeed: (s: Speed) => void
  at: number
  of: number
  seek: (to: number) => void
  /** Where each turn begins, as a count of beats told — so seeking to one puts
   *  the turn's own announcement on the board as the last thing said.
   *
   *  **One player's turn, not a round.** Forge alternates seats and prints a
   *  turn line for each, so consecutive marks are one player's turn apart.
   *  That is the unit somebody studying a game wants (Aaron, 2026-08-26:
   *  "a player's turn at a time, not a full two player turn"), and this
   *  project has been bitten once already by Forge's two different turn
   *  numbers — `lib/theater.ts` carries that argument in full. */
  turns?: number[]
  /** Every game the match has finished, in order. One while it is still being
   *  played; all of them once it is over. */
  games: number[]
  /** Which one is on the field. */
  playing: number
  chooseGame: (game: number) => void
}) {
  const running = speed !== 'paused'

  // Where a turn step lands is `stepToTurn`'s rule, in `lib/theater.ts` beside
  // the rest of the turn arithmetic — a component is a poor place for a rule
  // worth pinning with a test.
  const step = (to: number) => { setSpeed('paused'); seek(to) }

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
      {/* **Two units, and the controls say which is which.** The inner pair
          walks a beat — one line of the account, one change on the board. The
          outer pair walks a whole turn, and wears the word so nobody has to
          discover the difference by pressing it. That asymmetry is the label:
          a glyph alone means "the small step", a glyph beside "Turn" means the
          big one.

          The turn pair only appears once the account has raised a turn to step
          to. A control that cannot do anything is not a control (the same rule
          the game chips above follow), and before the first turn line lands
          there is nothing for these to reach. */}
      <div className="field-transport-buttons">
        {turns.length > 0 && (
          <button type="button" className="btn btn-sm field-step field-turn"
                  onClick={() => step(stepToTurn(turns, at, of, 'back'))}
                  disabled={at <= 0} aria-label="Back one turn"
                  title="Back one turn">
            <span aria-hidden="true">◀</span>
            <span className="field-turn-word">Turn</span>
          </button>
        )}
        <button type="button" className="btn btn-sm field-step"
                onClick={() => step(at - 1)}
                disabled={at <= 0} aria-label="Back one beat"
                title="Back one beat">
          <span aria-hidden="true">◀◀</span>
        </button>
        <button type="button"
                className={`btn btn-sm${running ? ' is-on' : ''}`}
                onClick={() => setSpeed(running ? 'paused' : 'play')}
                aria-label={running ? 'Pause' : 'Play'}>
          <span aria-hidden="true">{running ? '❙❙' : '▶'}</span>
        </button>
        <button type="button" className="btn btn-sm field-step"
                onClick={() => step(at + 1)}
                disabled={at >= of} aria-label="Forward one beat"
                title="Forward one beat">
          <span aria-hidden="true">▶▶</span>
        </button>
        {turns.length > 0 && (
          <button type="button" className="btn btn-sm field-step field-turn"
                  onClick={() => step(stepToTurn(turns, at, of, 'on'))}
                  disabled={at >= of} aria-label="Forward one turn"
                  title="Forward one turn">
            <span className="field-turn-word">Turn</span>
            <span aria-hidden="true">▶</span>
          </button>
        )}
      </div>

      {/* Places rather than actions, so `.chip-toggle` rather than `.btn` —
          a speed is a setting you are *in*, not a thing you do.

          **"Study", not "Slow".** The slow setting was twice too fast to
          follow (Aaron: "it moves quicker than the mind can keep up with"),
          and the fix took it to three seconds a beat — six and a half minutes
          for a game. That is not a slower way of watching; it is reading the
          game a line at a time, which is what somebody meeting their first
          Commander match actually needs (commandment 2). A control whose name
          undersells it by that much is a control nobody presses. */}
      <div className="field-speeds" role="group" aria-label="Speed">
        {(['study', 'play', 'fast'] as const).map((s) => (
          <button key={s} type="button"
                  className={`chip-toggle field-speed${speed === s ? ' is-active' : ''}`}
                  aria-pressed={speed === s}
                  onClick={() => setSpeed(s)}>
            {s === 'study' ? 'Study' : s === 'play' ? 'Watch' : 'Fast'}
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
  speed, setSpeed, of, seek, turns, games, playing, chooseGame, zones = [] }: {
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
  /** Where each of this bout's turns begins, for stepping a turn at a time.
   *  See [FieldTransport] — these are player-turns, not rounds. */
  turns?: number[]
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
  const live = mark && beat?.card
    ? { card: beat.card, mark, key: beat.key }
    : null
  const held = useHeldMark(live?.card ?? null, live ? mark : null,
    beat?.key ?? '', speed, game)
  // **Paused, the mark is the beat's again.** Nothing is draining, so there is
  // nothing for a mark to outlive — and stepping or scrubbing pauses first
  // (both controls call `setSpeed('paused')`), which is exactly where holding
  // a mark past its beat would be wrong: a scrub must land on the board its
  // beat describes and not on the one before it. See `useHeldMark`.
  const struck = speed === 'paused' ? live : held

  // How long each mark is watched, handed to the stylesheet rather than
  // written down there a second time. It rides `.field-stage` because that is
  // the one box containing both the arena and the zones beside it, and the
  // ghost rising off a graveyard is timed by the same death that put it there.
  const lives = {
    '--mark-life-attacks': `${markLife('attacks', speed)}ms`,
    '--mark-life-blocks': `${markLife('blocks', speed)}ms`,
    '--mark-life-dies': `${markLife('dies', speed)}ms`,
  } as CSSProperties

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
              + 'tale of the tape below is the whole of what it saw.'}
        </p>
      </section>
    )
  }

  // **Whose turn it is was already on the wire.** Forge's turn event names the
  // seat and `foldBoard` carries it as `active`; it was simply never drawn.
  // Zero until the first turn begins, and then neither half is lit — which is
  // the truth about that moment rather than a hole to fill.
  const lit = far.seat === state.active ? 'far'
    : near.seat === state.active ? 'near'
    : null
  const farName = name(far.slug, far.name)
  const nearName = name(near.slug, near.name)
  const onTurn = lit === 'far' ? farName : lit === 'near' ? nearName : null

  // Which seat the beat being read belongs to. `Coliseum` stages a beat's
  // `who` through the same shelf this `name` reads, so the two strings are
  // comparable — and this is only ever a *preference*: the centre stage uses
  // it to pick the caster's own copy of a card both seats happen to run, and a
  // miss is a null, which is the other copy of the same picture.
  const casting = beat?.who === farName ? far.seat
    : beat?.who === nearName ? near.seat
    : null

  return (
    <Dressing.Provider value={dressing}>
    <Struck.Provider value={struck}>
    {/* **The stage: the arena, and the places beside it.**
        The zones were a column *inside* the field, which put them on the sand
        — and took their width out of the battlefield, so the arena came out
        scaled awkwardly (Aaron, 2026-08-25). They are not part of the arena.
        A graveyard is not somewhere you stand; it is somewhere cards go, and
        it belongs off the floor entirely. So the field keeps its own box at
        its own size, and the zones stand outside it in their own column. */}
    <div className="field-stage" style={lives}>
    <section className={`field${lit ? ` is-${lit}-on` : ''}`}
             aria-label="The battlefield">
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

      <FieldSide side={far} facing="far" active={lit === 'far'} />

      {/* The seam: in the real building, the trench the lifts came up through.
          Here it is where the turn is announced and where the two
          battlefields meet. */}
      <div className="field-seam">
        <span className="field-seam-rule" aria-hidden="true" />
        <span className="field-seam-turn tabular">
          {/* **The light says which half; this says it in a second way.**
              A warm wash on sand is a beautiful signal and a soft one, and
              somebody at their first game should not have to learn to read it
              — so the trench also points, up or down, at whoever is on turn.
              A mark rather than a name: a deck name in here is an overflow on
              a phone, and the name is right beside each seat already. The
              whole fact goes to a screen reader below. */}
          {lit && (
            <span className={`field-seam-on is-${lit}`} aria-hidden="true">
              {lit === 'far' ? '▲' : '▼'}
            </span>
          )}
          {state.turn > 0 ? `Turn ${state.turn}` : 'Before the first turn'}
          {game > 0 && <span className="field-seam-game">Game {game}</span>}
          {onTurn && <span className="sr-only">, {onTurn} is on turn</span>}
        </span>
        <span className="field-seam-rule" aria-hidden="true" />
      </div>

      <FieldSide side={near} facing="near" active={lit === 'near'} />

      <FieldHand side={near} facing="near" name={nearName} />

      {/* **Everything that is cast, and everything that dies.** The board can
          only draw what stays, and half of Commander never stays — an instant
          is cast, resolves and is in a graveyard inside one beat, and the sand
          had nothing to say about any of it (Aaron, 2026-08-26). So the middle
          of the arena is its own surface: see `stage.tsx`, which owns every
          decision about what goes there and for how long.

          Inside the field rather than over the whole stage, on purpose — it is
          clipped to the sand, it is centred on the arena rather than on the
          arena plus a column of graveyards, and it inherits the marks' own
          `--mark-life-dies` from `.field-stage`, which is how a death drawn
          here and the skull on the card in its row stay one event. */}
      <CenterStage board={board} beat={beat ?? null} speed={speed} game={game}
                   dies={markLife('dies', speed)} seat={casting} />

      <FieldTransport speed={speed} setSpeed={setSpeed} at={shown} of={of}
                      seek={seek} turns={turns} games={games} playing={playing}
                      chooseGame={chooseGame} />
    </section>
    <aside className="field-zones" aria-label="Zones off the battlefield">
      <FieldRail side={far} facing="far" name={name(far.slug, far.name)} />
      <FieldRail side={near} facing="near" name={name(near.slug, near.name)} />
    </aside>
    </div>
    </Struck.Provider>
    </Dressing.Provider>
  )
}
