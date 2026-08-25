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
import { type BoardCard, type BoardSide, fightingStats, foldBoard }
  from '../lib/board'

/** One card on the field.
 *
 * `key` is the card's Forge instance id, which is what lets React move the
 * same element between zones instead of destroying and rebuilding it — a
 * creature that dies has to be the same DOM node in the graveyard or the
 * animation has nothing to animate.
 */
function FieldCard({ card, size }: { card: BoardCard; size: 'normal' | 'small' }) {
  const stats = fightingStats(card)
  const counters = card.counters.filter((c) => c.n > 0)
  // A token's painting is a *chosen* printing (the earliest, which is the
  // original), so the painter is worth naming where a person can find them.
  const title = [
    card.name,
    stats,
    counters.map((c) => `${c.n} ${c.kind}`).join(', '),
    card.tapped ? 'tapped' : '',
    card.artist ? `art by ${card.artist}` : '',
  ].filter(Boolean).join(' · ')

  return (
    <div className={`field-card field-card-${size}${card.tapped ? ' is-tapped' : ''}`}
         title={title}>
      <div className="field-card-turn">
        {card.image ? (
          <img className="field-card-art" src={card.image} alt={card.name}
               loading="lazy" draggable={false} />
        ) : (
          // No painting is a legible state, not a hole: the pool may not have
          // been refreshed, and a match is worth watching either way.
          <span className="field-card-plate">{card.name}</span>
        )}
        {card.token && <span className="field-card-token" aria-hidden="true" />}
        {stats && <span className="field-card-stats tabular">{stats}</span>}
        {counters.length > 0 && (
          <span className="field-card-counters tabular">
            {counters.map((c) => c.n).reduce((a, b) => a + b, 0)}
          </span>
        )}
      </div>
    </div>
  )
}

/** A row of cards, with a name and a count when it is empty. */
function FieldRow({ label, cards, size = 'normal', empty }: {
  label: string
  cards: BoardCard[]
  size?: 'normal' | 'small'
  empty?: string
}) {
  return (
    <div className="field-row" aria-label={`${label}: ${cards.length}`}>
      {cards.length === 0 ? (
        <span className="field-row-empty">{empty ?? label}</span>
      ) : cards.map((card) => (
        <FieldCard key={card.id} card={card} size={size} />
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

/** The stone rail one player's name and totals are carved into. */
function FieldRail({ side, name }: { side: BoardSide; name: string }) {
  return (
    <div className="field-rail">
      <span className="field-rail-name" title={side.name}>{name}</span>
      <span className="field-rail-totals">
        <LifeTotal life={side.life} />
        <span className="field-rail-tally tabular"
              title={`${side.graveyard.length} in the graveyard`}>
          <span className="field-rail-tally-label">GY</span>
          {side.graveyard.length}
        </span>
        {side.exile.length > 0 && (
          <span className="field-rail-tally tabular"
                title={`${side.exile.length} exiled`}>
            <span className="field-rail-tally-label">EX</span>
            {side.exile.length}
          </span>
        )}
      </span>
    </div>
  )
}

/**
 * One player's half of the field.
 *
 * `facing` is which edge of the table they sit at, and it does one thing:
 * reverses the row order. The far player's battlefield is nearest the seam and
 * their hand is at the outer edge, which is exactly how it looks from the
 * other side of a real table.
 */
function FieldSide({ side, name, facing }: {
  side: BoardSide
  name: string
  facing: 'far' | 'near'
}) {
  const rows = (
    <>
      <FieldRow label="Hand" cards={side.hand} size="small"
                empty="an empty hand" />
      <FieldRow label="Lands" cards={side.land} size="small"
                empty="no lands yet" />
      <FieldRow label="Battlefield" cards={side.battlefield}
                empty="nothing on the battlefield" />
    </>
  )
  return (
    <div className={`field-side field-side-${facing}`}>
      {facing === 'far' && <FieldRail side={side} name={name} />}
      <div className="field-rows">
        {facing === 'far' ? rows : (
          <>
            <FieldRow label="Battlefield" cards={side.battlefield}
                      empty="nothing on the battlefield" />
            <FieldRow label="Lands" cards={side.land} size="small"
                      empty="no lands yet" />
            <FieldRow label="Hand" cards={side.hand} size="small"
                      empty="an empty hand" />
          </>
        )}
      </div>
      {facing === 'near' && <FieldRail side={side} name={name} />}
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
export function MatchBoard({ board, shown, game, name, running }: {
  board: ForgeBoard | null
  shown: number
  game: number
  /** Turns a seat's slug into whatever the room calls that deck. Passed in
   *  because only the room has the shelf. */
  name: (slug: string | null, fallback: string) => string
  running: boolean
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
    </section>
  )
}
