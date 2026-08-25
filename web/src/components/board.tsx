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
 * **A tapped permanent is turned ninety degrees**, because that is what tapped
 * *is*. No badge, no dimming, no icon: the card lies on its side the way it
 * would on a table, and it turns rather than jumping there.
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

import type { ForgeBoard } from '../lib/api'
import { CardSheet } from './ui'
import { ThroneGlyph } from './glyphs'
import aegisArt from '../assets/coliseum/aegis.webp'
import mementoArt from '../assets/coliseum/memento.webp'
import { type BoardCard, type BoardSide, fightingStats, foldBoard, stackRow }
  from '../lib/board'
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

function FieldCard({ card, size, count }: {
  card: BoardCard
  size: 'normal' | 'small'
  /** How many identical cards this one stands for. See `stackRow`. */
  count: number
}) {
  const [held, setHeld] = useState(false)
  // The loupe, held open by a tap. Hover and focus open it in CSS; this is
  // only for the pointer that has no hover to give.
  const [peeking, setPeeking] = useState(false)
  const stats = fightingStats(card)
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
    card.artist ? `art by ${card.artist}` : '',
  ].filter(Boolean).join(' · ')

  return (
    <div className={`field-card field-card-${size}${card.tapped ? ' is-tapped' : ''}`
                    + (count > 1 ? ' is-stacked' : '')
                    + (mark ? ` is-${mark.mark}` : '')}
         title={title} tabIndex={card.image ? 0 : -1}
         onPointerUp={(e) => {
           // **The peek below is `:hover` and `:focus-visible`, and a phone is
           // neither.** Forty cards on a floor, none of them readable at forty
           // pixels, and the one mechanism that made them readable needed a
           // pointer — so on a touch screen the whole board was a mosaic. The
           // sheet is the same answer the card lists got: held up, centred,
           // and free of the field's own `overflow: hidden`, which is what
           // clips a peek opening near an edge.
           if (e.pointerType === 'mouse' || !card.image) return
           setHeld(true)
         }}>
      {/* **The card, readable.** A permanent on this board is forty pixels of
          painting — enough to know a Forest from a Dragon and nowhere near
          enough to read one. Hovering lifts the whole face out at a size a
          person can actually read, which is what every Magic client does and
          what the rest of this app already does through `CardHover`.

          Drawn as a sibling rather than in a portal, and only on hover or
          keyboard focus: a board holds forty of these, and forty always-mounted
          previews is forty more images than the page needs. `tabIndex` is what
          gives it to the keyboard — a hover-only affordance is one nobody
          without a mouse ever gets. */}
      {card.image && (
        <span className="field-card-peek" aria-hidden="true">
          <img src={card.image} alt="" loading="lazy" draggable={false} />
          {card.artist && (
            <span className="field-card-peek-artist">art by {card.artist}</span>
          )}
        </span>
      )}
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
      {/* **Outside the part that turns.** A tapped permanent lies on its side
          and its numbers must not: a sideways "19/19" is unreadable, and by
          the late turns most of a board is tapped. These sit on the outer box,
          which never rotates. */}
      {/* **The loupe.** Power and toughness were a black tab printed over the
          corner of the painting at all times — legible, and permanently in
          the way of the one part of a card everybody already looks at. A
          board holds forty of them, so forty little black tabs sat on forty
          paintings whether anybody wanted a number or not (Aaron, 2026-08-25:
          *"a magnifying glass effect instead when hovering in that corner"*).

          So the numbers are behind glass. The lens sits where a card's own
          power/toughness box sits, magnifies the painting under it, and puts
          the *current* figures on the glass in crisp type — current rather
          than printed, because a 2/2 with three +1/+1 counters is a 5/5 and
          the printed box would be a lie told very clearly.

          Hover, focus **and** tap all open it. A hover-only reading
          affordance does not exist on a phone, which this room has now
          learned twice. */}
      {stats && (
        <span className={`field-card-lens${peeking ? ' is-open' : ''}`}
              aria-hidden="true"
              style={card.image
                ? ({ '--lens-art': `url(${card.image})` } as CSSProperties)
                : undefined}
              onPointerUp={(e) => {
                // The corner is its own gesture: a tap here is *what are its
                // numbers*, and a tap anywhere else on the card is still
                // "hold it up", which is a different question.
                if (e.pointerType === 'mouse') return
                e.stopPropagation()
                setPeeking((was) => !was)
              }}>
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
          The count carries the sign in type as well, for anybody who does not
          separate those two hues. */}
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
  empty?: string
}) {
  const stacks = stackRow(cards)
  return (
    <div className="field-row" aria-label={`${label}: ${cards.length}`}>
      {stacks.length === 0 ? (
        <span className="field-row-empty">{empty ?? label}</span>
      ) : stacks.map((stack) => (
        <FieldCard key={stack.card.id} card={stack.card} size={size}
                   count={stack.count} />
      ))}
    </div>
  )
}

/** A life total that takes the hit visibly.
 *
 * The number is the fact and the flash is the news — a total that changed
 * silently is a total nobody notices changing, and life is the one number in
 * Commander everybody is actually tracking. */
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
  return (
    <span className={`field-life tabular${hit ? ` is-${hit}` : ''}`}>
      {life}
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
function FieldPile({ label, cards, short, throne }: {
  label: string
  cards: BoardCard[]
  short: string
  /** The command zone, which is the one pile whose *emptiness* is news. */
  throne?: boolean
}) {
  // Held open by a tap. Hover and keyboard focus open it in CSS; this is for
  // the pointer that has no hover to give.
  const [open, setOpen] = useState(false)
  const struck = useContext(Struck)
  // **The skull lands on the grave, and it has to.** By the time the sentence
  // "X dies" is read, the card is already in the graveyard — Forge reports the
  // death and the zone change on one line, so the step that tells the beat is
  // the step that moves the card, and there is no instant at which the board
  // holds a dead creature still standing. Marking the pile it went into is not
  // a consolation for that; it is where a headstone goes.
  const buried = struck?.mark === 'dies'
    && cards.some((c) => sameCard(c.name, struck.card))
  const top = cards[cards.length - 1]
  const seat = throne && !top
  const title = top ? `${label}: ${cards.length}, ${top.name} on top`
    : seat ? `${label}: empty — out on the battlefield`
    : `${label}: empty`
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
      {top && top.image ? (
        <img className="field-pile-art" src={top.image} alt="" loading="lazy"
             draggable={false} />
      ) : null}
      {/* **An empty command zone means the commander is on the table**, which
          is the opposite of an empty graveyard and was drawn the same way.
          The chair says which: theirs, and nobody in it. */}
      {seat && <span className="field-pile-throne"><ThroneGlyph /></span>}
      {buried && struck && (
        <span key={struck.key} className="field-pile-buried" aria-hidden="true">
          <img src={mementoArt} alt="" draggable={false} />
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
function FieldRail({ side, name }: { side: BoardSide; name: string }) {
  // Two generic for each previous cast from the zone. With partners this
  // reports the dearer of the two: there is one pile and forty pixels of it,
  // and the expensive commander is the one whose price changes a decision.
  const tax = 2 * side.commanders.reduce((n, c) => Math.max(n, c.casts), 0)
  return (
    <div className="field-rail">
      <span className="field-rail-name" title={side.name}>{name}</span>
      <span className="field-rail-totals">
        <FieldPile label="Command zone" short="CMD" cards={side.command}
                   throne />
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
        <FieldPile label="Graveyard" short="GY" cards={side.graveyard} />
        <FieldPile label="Exile" short="EX" cards={side.exile} />
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
  const [spread, setSpread] = useState(false)
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
          kind of panel instead of four with two. */}
      <span className="field-hand-label"
            onPointerUp={(e) => {
              if (e.pointerType === 'mouse' || side.hand.length === 0) return
              setSpread((was) => !was)
            }}>
        {name}<span className="field-hand-n tabular">{side.hand.length}</span>
      </span>
      {side.hand.length > 0 && (
        <div className={`field-tray field-hand-tray${spread ? ' is-open' : ''}`}
             role="group" aria-label={`Hand, all ${side.hand.length}`}>
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
function FieldSide({ side, name, facing }: {
  side: BoardSide
  name: string
  facing: 'far' | 'near'
}) {
  // Outermost first. The near player's side is the same list reversed, so the
  // two creature rows finish up either side of the seam.
  const rows = [
    <FieldRow key="land" label="Lands" cards={side.land} size="small"
              empty="no lands yet" />,
    <FieldRow key="perm" label="Artifacts and enchantments"
              cards={side.permanents} size="small" empty="—" />,
    <FieldRow key="crea" label="Creatures" cards={side.creatures}
              empty="no creatures" />,
  ]
  return (
    <div className={`field-side field-side-${facing}`}>
      {facing === 'far' && <FieldRail side={side} name={name} />}
      <div className="field-rows">
        {facing === 'far' ? rows : [...rows].reverse()}
      </div>
      {facing === 'near' && <FieldRail side={side} name={name} />}
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
  speed, setSpeed, of, seek, games, playing, chooseGame }: {
  board: ForgeBoard | null
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

      <FieldSide side={far} facing="far"
                 name={name(far.slug, far.name)} />

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

      <FieldSide side={near} facing="near"
                 name={name(near.slug, near.name)} />

      <FieldHand side={near} facing="near"
                 name={name(near.slug, near.name)} />


      <FieldTransport speed={speed} setSpeed={setSpeed} at={shown} of={of}
                      seek={seek} games={games} playing={playing}
                      chooseGame={chooseGame} />
    </section>
    </Struck.Provider>
  )
}
