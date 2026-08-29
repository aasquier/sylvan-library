/**
 * Ancestral Recall — deal yourself an opening hand, and deal it again.
 *
 * This exists for commandment 2 and for no other reason. Somebody who has
 * never played Magic has no picture of what seven cards off the top look
 * like: how often it is three lands and four spells, how often it is one land
 * and a fistful of six-drops, how often you would want to throw it back. That
 * is not learnable from a probability table. It is learnable by dealing hands
 * until the shape of them stops being a surprise, which costs nothing and is
 * exactly what a person can do at a kitchen table before they ever sit down
 * to a game.
 *
 * # Why Ancestral Recall, and what it beat
 *
 * Aaron asked the question and left the answer open: *"something famous for
 * card draw, like what is the most card draw-y card in all of magic?"* The
 * consensus answer to that question is Ancestral Recall, and it wins here for
 * a second reason that matters more: **the name is the feature**. To recall
 * is to have the memory of a thing you did not live through, which is what
 * this hands a newcomer — a hundred opening hands they have not played yet.
 * The Vintage Masters printing is the one used, Ryan Pancoast's: colossal
 * ancestors standing out of the sea with one small figure climbing toward
 * them by lantern-light. A real painting, at a real painter's hand
 * (commandment 5).
 *
 * Three cards were weighed and put down. **Timetwister** is the better
 * *mechanical* match — shuffle everything back and draw seven is literally
 * the redeal button — but it is a Wheel of Fortune in a different frame, and
 * this same page already has a Wheel; two wheel effects side by side read as
 * one joke told twice. **Brainstorm** is the better *conceptual* match, draw
 * three and put two back being the card about hand quality, but Christopher
 * Rush's Ice Age painting is a grimacing face, and a grimace is not the
 * greeting a newcomer's first opening hand should get. **Serum Powder** is
 * the literal redeal and nobody outside Legacy has heard of it; the folded
 * state has to be recognisable at 136 pixels.
 *
 * The art is hotlinked from Scryfall, like every other painting in this app,
 * and no derivative is committed anywhere (rule 5, ADR 6). The credit line
 * under the panel is the licence made visible, not decoration.
 *
 * # The shuffle is the server's, and there is no seed anywhere near a person
 *
 * `POST /api/decks/{owner}/{slug}/opening-hand` shuffles and turns over
 * seven; `go/internal/sim/opening` argues the whole of it. No seed crosses
 * the wire in either direction and `Math.random` is never reached for here:
 * a seed is machinery, and machinery is never rendered at somebody who came
 * for cards (commandment 10).
 *
 * A dealt hand lives in state and is never re-fetched by a render, so
 * nothing but a press of Deal ever changes what is on the table. The fold
 * does not survive a reload, for the Wheel's reason: this is an amusement you
 * go to, use and are done with, and coming back to a deck page should show
 * you the deck.
 *
 * # It counts. It does not advise.
 *
 * ADR 14: deterministic code decides, Claude advises. Keep-or-mulligan is an
 * *opinion* and this panel deliberately has none — no verdict, no score, no
 * traffic light. What it renders is arithmetic somebody could redo with the
 * seven cards face up: how many are lands, which of the commander's colours
 * those lands can pay for, the earliest turn a spell here could be cast, how
 * many are reachable by turn three. The one line of conventional guidance is
 * labelled as convention. The conclusion is the reader's, which is the whole
 * exercise: a toy that said "mulligan this" would teach a newcomer to ask it
 * again instead of teaching them to read a hand.
 */

import { useEffect, useRef, useState } from 'react'
import {
  api, errorMessage, type DealtCard, type DeckRef, type OpeningHand,
} from '../lib/api'
import { useTableSound } from '../lib/prefs'
import { deal as dealSound, riffle } from '../lib/tablesounds'
import { HandFanGlyph } from './glyphs'
import { CardHover, ManaCost, ManaText } from './ui'

/** Ancestral Recall, Vintage Masters, Ryan Pancoast — the whole card, for the
 *  folded state. Hotlinked, never committed. */
const RECALL_CARD =
  'https://cards.scryfall.io/normal/front/2/3/2398892d-28e9-4009-81ec-0d544af79d2b.jpg'

/** How long between one card landing and the next, in milliseconds. The
 *  stylesheet staggers `.hand-card` on the same figure through `--deal-i`,
 *  and the table sound is scheduled against it, so a card's sound and its
 *  landing are one event rather than two that drift. */
const DEAL_STEP = 90

/** A dealt card's caption: what this card is waiting on.
 *
 *  Three sentences, because a card in an opening hand is in exactly three
 *  states and a newcomer needs all three named. A land is played rather than
 *  cast and has no turn at all; a spell either has an earliest turn off the
 *  lands sitting beside it, or it has none — and "none" is the common,
 *  important case that a blank would hide. */
function Waiting({ card }: { card: DealtCard }) {
  if (card.is_land) {
    return (
      <span className="hand-mark is-land"
            title="A land is played, not cast — one a turn, for free.">
        Land
      </span>
    )
  }
  if (card.turn === null) {
    return (
      <span className="hand-mark is-far"
            title={`The lands in this hand never pay for ${card.name}.`}>
        Out of reach
      </span>
    )
  }
  return (
    <span className="hand-mark"
          title={`Playing one land a turn from this hand, ${card.name} `
                 + `could be cast on turn ${card.turn}.`}>
      Turn {card.turn}
    </span>
  )
}

/** One colour the commander needs, and whether a land here makes it. Written
 *  as a mana symbol through `ManaText`, so it is the pip a player already
 *  knows rather than a letter. */
function ColorNote({ color, covered }: { color: string; covered: boolean }) {
  return (
    <span className={`hand-color${covered ? ' is-covered' : ''}`}
          title={covered
            ? `A land in this hand makes ${color}.`
            : `No land in this hand makes ${color}.`}>
      <ManaText size={13}>{`{${color}}`}</ManaText>
      <span>{covered ? 'covered' : 'missing'}</span>
    </span>
  )
}

/** One card on the table: the printing, and what it is waiting on. */
function TableCard({ card, index }: { card: DealtCard; index: number }) {
  // A dealt hand is not a spreadsheet. Each card sits at its own small angle,
  // alternating either side of true, so the row reads as cards somebody put
  // down rather than as a row of thumbnails. The angle is data rather than
  // state, which is why it rides in as a custom property: a `:hover` never
  // needs to reach it, and the stylesheet owns every rule that does.
  const tilt = ((index % 2 === 0 ? -1 : 1) * (1.4 + (index % 3) * 0.9)).toFixed(2)
  return (
    <figure className="hand-card"
            style={{ '--deal-i': index, '--tilt': `${tilt}deg` } as React.CSSProperties}>
      <CardHover card={{ name: card.name, image: card.image }}>
        {card.image
          ? <img src={card.image} alt={card.name} className="hand-card-face" />
          : (
            <span className="hand-card-face is-blank">
              {card.name}
            </span>
            )}
      </CardHover>
      <figcaption className="hand-card-foot">
        <span className="hand-card-name">{card.name}</span>
        <span className="hand-card-line">
          <ManaCost cost={card.mana_cost} size={12} />
          <Waiting card={card} />
        </span>
      </figcaption>
    </figure>
  )
}

/** The face-down deck, waiting to be dealt from. Drawn in the panel's own
 *  materials — three plates, gold-ruled, squared up the way a deck sits on a
 *  table — because the alternative is a hole where the cards will be, and a
 *  panel that just sits there is the thing commandment 6 forbids. */
function FacedownDeck() {
  return (
    <div className="hand-deck" aria-hidden="true">
      <i className="hand-deck-plate is-back" />
      <i className="hand-deck-plate is-mid" />
      <i className="hand-deck-plate is-top" />
    </div>
  )
}

export function OpeningHandDeal({ deckRef }: { deckRef: DeckRef }) {
  const [open, setOpen] = useState(false)
  const [hand, setHand] = useState<OpeningHand | null>(null)
  const [dealing, setDealing] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [sound] = useTableSound()
  // Every deal gets a fresh key so React rebuilds the cards rather than
  // reusing them — without it the second hand swaps its pictures in place and
  // no card is ever dealt again, because the elements never re-mount and the
  // deal animation never re-runs.
  const [dealtAt, setDealtAt] = useState(0)
  const timers = useRef<number[]>([])

  useEffect(() => () => {
    for (const id of timers.current) window.clearTimeout(id)
    timers.current = []
  }, [])

  async function draw() {
    if (dealing) return
    setError(null)
    setDealing(true)
    // The shuffle is heard before the answer arrives, because that is the
    // order it happens in at a table: the cards go through your hands and
    // then they come off the top.
    if (sound) riffle()
    try {
      const result = await api.dealOpeningHand(deckRef)
      setHand(result)
      setDealtAt(Date.now())
      if (sound && result.cards.length > 0) {
        for (const id of timers.current) window.clearTimeout(id)
        timers.current = result.cards.map((_, i) =>
          window.setTimeout(dealSound, 120 + i * DEAL_STEP))
      }
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setDealing(false)
    }
  }

  // Folded, this is the card it came from and nothing else — no request has
  // been made, no pictures are being loaded, and one click opens the table.
  if (!open) {
    return (
      <button type="button" onClick={() => setOpen(true)} aria-expanded={false}
              className="hand-folded"
              title="Ancestral Recall — deal yourself an opening hand">
        <img src={RECALL_CARD}
             alt="Ancestral Recall, the card — deal yourself an opening hand" />
      </button>
    )
  }

  const reading = hand?.reading

  return (
    <section className="card-surface hand-frame space-y-3 rounded-xl p-5">
      <div className="flex flex-wrap items-baseline gap-2">
        <h3 className="text-sm font-semibold">Ancestral Recall</h3>
        <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
          Seven cards off a real shuffle of this deck. Deal as many as you
          like — nothing here is written down.
        </span>
        <button type="button" onClick={() => setOpen(false)} aria-expanded
                className="btn btn-ghost btn-xs ml-auto"
                title="Fold the table away into its card">
          Fold away
        </button>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <button type="button" onClick={() => void draw()} disabled={dealing}
                className={`btn btn-deal${dealing ? ' is-dealing' : ''}`}>
          <HandFanGlyph size={15} open={dealing} />
          {dealing ? 'Shuffling…' : hand ? 'Deal again' : 'Deal a hand'}
        </button>
        {hand?.pool_available && hand.deck_size !== undefined && (
          <span className="text-[11px]" style={{ color: 'var(--text-muted)' }}>
            Shuffled from {hand.deck_size} cards
            {hand.unresolved_count
              ? ` — ${hand.unresolved_count} the library could not find were left out`
              : ''}
          </span>
        )}
      </div>

      {error && (
        <p className="text-xs" style={{ color: 'var(--status-critical)' }}>
          {error}
        </p>
      )}

      {hand && !hand.pool_available && (
        <p className="text-xs" style={{ color: 'var(--text-muted)' }}>
          {hand.message}
        </p>
      )}

      {!hand && !error && (
        <div className="hand-empty">
          <FacedownDeck />
          <p className="text-xs leading-relaxed" style={{ color: 'var(--text-muted)' }}>
            Every game of Magic starts here: seven cards, face up, and one
            decision. Deal a few and you will start to feel which sevens you
            would keep.
          </p>
        </div>
      )}

      {/* A deal is a page changing under somebody who cannot see it change.
          One short spoken sentence per deal — the count and the shape, not
          the seven names and not the caveat, which would be read out again
          every single press. */}
      <span className="sr-only" role="status">
        {hand?.reading
          ? `Dealt ${hand.cards.length} cards: ${hand.reading.lands} `
            + `land${hand.reading.lands === 1 ? '' : 's'} and `
            + `${hand.reading.spells} spell${hand.reading.spells === 1 ? '' : 's'}.`
          : ''}
      </span>

      {hand?.cards.length ? (
        <div className="hand-table" key={dealtAt}>
          {hand.cards.map((card, i) => (
            <TableCard key={`${dealtAt}-${i}-${card.name}`} card={card} index={i} />
          ))}
        </div>
      ) : null}

      {reading && (
        <div className="hand-reading">
          {hand?.commander && (
            <CardHover card={{ name: hand.commander.name, image: hand.commander.image }}>
              <span className="hand-commander">
                {hand.commander.image && (
                  <img src={hand.commander.image} alt={hand.commander.name} />
                )}
                <span>Command zone — never in your hand, always available</span>
              </span>
            </CardHover>
          )}
          <ul className="hand-facts">
            <li>
              <strong>{reading.lands}</strong> land{reading.lands === 1 ? '' : 's'},{' '}
              <strong>{reading.spells}</strong> spell{reading.spells === 1 ? '' : 's'}
            </li>
            {(reading.colors_covered.length > 0 || reading.colors_missing.length > 0) && (
              <li className="hand-colors">
                {reading.colors_covered.map((c) => (
                  <ColorNote key={`have-${c}`} color={c} covered />
                ))}
                {reading.colors_missing.map((c) => (
                  <ColorNote key={`want-${c}`} color={c} covered={false} />
                ))}
              </li>
            )}
            <li>
              {reading.first_spell_turn === null
                ? 'No spell here casts off these lands alone.'
                : <>First spell on <strong>turn {reading.first_spell_turn}</strong></>}
            </li>
            <li>
              <strong>{reading.castable_by_horizon}</strong> of {hand?.cards.length}{' '}
              castable by turn {reading.horizon}
            </li>
          </ul>
          <p className="text-[11px] leading-relaxed" style={{ color: 'var(--text-muted)' }}>
            Players usually like two to four lands in an opening seven. That is
            convention rather than arithmetic, and it is not a verdict on this
            hand — the call is yours, and dealing again costs nothing.
          </p>
          <p className="text-[11px] leading-relaxed" style={{ color: 'var(--text-muted)' }}>
            {hand?.caveat}
          </p>
        </div>
      )}

      {/* `toy-credit` pins the licence line to the bottom edge of the frame:
          this panel stands beside the Wheel's now, the two are never the same
          height, and credits that end level are what makes them read as a
          pair rather than as two things that happened to land next to each
          other. */}
      <p className="toy-credit text-[11px]" style={{ color: 'var(--text-muted)' }}>
        <em>Ancestral Recall</em> by Ryan Pancoast, Vintage Masters — one
        blue mana for three cards, and the reason anybody says
        &ldquo;card advantage&rdquo; at all.
      </p>
    </section>
  )
}
