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

import { useEffect, useRef, useState } from 'react'

import type { ForgeBoard } from '../lib/api'
import { CardSheet } from './ui'
import { type BoardCard, type BoardSide, fightingStats, foldBoard, stackRow }
  from '../lib/board'
import type { Speed } from '../lib/reel'

/** One card on the field.
 *
 * `key` is the card's Forge instance id, which is what lets React move the
 * same element between zones instead of destroying and rebuilding it — a
 * creature that dies has to be the same DOM node in the graveyard or the
 * animation has nothing to animate.
 */
function FieldCard({ card, size, count }: {
  card: BoardCard
  size: 'normal' | 'small'
  /** How many identical cards this one stands for. See `stackRow`. */
  count: number
}) {
  const [held, setHeld] = useState(false)
  const stats = fightingStats(card)
  const counters = card.counters.filter((c) => c.n > 0)
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
                    + (count > 1 ? ' is-stacked' : '')}
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
      {stats && <span className="field-card-stats tabular">{stats}</span>}
      {count > 1 && (
        <span className="field-card-count tabular">{count}<span
          className="field-card-times">×</span></span>
      )}
      {counters.length > 0 && (
        <span className="field-card-counters tabular">
          {counters.map((c) => c.n).reduce((a, b) => a + b, 0)}
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
function FieldPile({ label, cards, short }: {
  label: string
  cards: BoardCard[]
  short: string
}) {
  const top = cards[cards.length - 1]
  return (
    <div className={`field-pile${cards.length === 0 ? ' is-empty' : ''}`}
         title={top ? `${label}: ${cards.length}, ${top.name} on top`
                    : `${label}: empty`}
         aria-label={`${label}: ${cards.length}`}>
      {top && top.image ? (
        <img className="field-pile-art" src={top.image} alt="" loading="lazy"
             draggable={false} />
      ) : null}
      <span className="field-pile-label">{short}</span>
      <span className="field-pile-n tabular">{cards.length}</span>
    </div>
  )
}

/** The stone rail one player's name, life and closed zones are carved into. */
function FieldRail({ side, name }: { side: BoardSide; name: string }) {
  return (
    <div className="field-rail">
      <span className="field-rail-name" title={side.name}>{name}</span>
      <span className="field-rail-totals">
        <FieldPile label="Command zone" short="CMD" cards={side.command} />
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
  return (
    <div className={`field-hand field-hand-${facing}`}>
      <span className="field-hand-label">
        {name}<span className="field-hand-n tabular">{side.hand.length}</span>
      </span>
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
export function MatchBoard({ board, shown, game, name, running,
  speed, setSpeed, of, seek, games, playing, chooseGame }: {
  board: ForgeBoard | null
  shown: number
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
    <section className="field" aria-label="The battlefield">
      {/* The arena floor: sand, and the dust that never quite settles. */}
      <div className="field-floor" aria-hidden="true">
        <span className="field-dust field-dust-1" />
        <span className="field-dust field-dust-2" />
        <span className="field-dust field-dust-3" />
      </div>

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

      {/* Both hands, held at the side of the table. Far above near, the same
          way the seats are, so a hand stays with the person holding it. */}
      <div className="field-hands">
        <FieldHand side={far} facing="far" name={name(far.slug, far.name)} />
        <FieldHand side={near} facing="near"
                   name={name(near.slug, near.name)} />
      </div>

      <FieldTransport speed={speed} setSpeed={setSpeed} at={shown} of={of}
                      seek={seek} games={games} playing={playing}
                      chooseGame={chooseGame} />
    </section>
  )
}
